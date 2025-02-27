package check

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bestruirui/mihomo-check/check/platfrom"
	"github.com/bestruirui/mihomo-check/config"
	proxyutils "github.com/bestruirui/mihomo-check/proxy"
	"github.com/metacubex/mihomo/adapter"
	"github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/log"
)

// Result 存储节点检测结果
type Result struct {
	Proxy      map[string]any
	Openai     bool
	Youtube    bool
	Netflix    bool
	Google     bool
	Cloudflare bool
	Disney     bool
}

// ProxyChecker 处理代理检测的主要结构体
type ProxyChecker struct {
	results     []Result
	proxyCount  int
	threadCount int
	progress    int32
	available   int32
	resultChan  chan Result
	tasks       chan map[string]any
}

// NewProxyChecker 创建新的检测器实例
func NewProxyChecker(proxies []map[string]any) *ProxyChecker {
	proxyCount := len(proxies)
	threadCount := config.GlobalConfig.Concurrent
	if proxyCount < threadCount {
		threadCount = proxyCount
	}

	return &ProxyChecker{
		results:     make([]Result, 0),
		proxyCount:  proxyCount,
		threadCount: threadCount,
		resultChan:  make(chan Result),
		tasks:       make(chan map[string]any, proxyCount),
	}
}

// Check 执行代理检测的主函数
func Check() ([]Result, error) {
	proxyutils.ResetRenameCounter()

	proxies, err := proxyutils.GetProxies()
	if err != nil {
		return nil, fmt.Errorf("获取节点失败: %w", err)
	}

	log.Infoln("共获取到 %v 个节点", len(proxies))
	proxies = proxyutils.DeduplicateProxies(proxies)
	log.Infoln("去重后共 %v 个节点", len(proxies))

	checker := NewProxyChecker(proxies)
	return checker.run(proxies)
}

// Run 运行检测流程
func (pc *ProxyChecker) run(proxies []map[string]any) ([]Result, error) {
	log.Infoln("开始检测节点")
	done := make(chan bool)
	if config.GlobalConfig.PrintProgress {
		go pc.showProgress(done)
	}
	// todo: 替换掉日志库
	log.SetLevel(log.ERROR)
	var wg sync.WaitGroup
	// 启动工作线程
	for i := 0; i < pc.threadCount; i++ {
		wg.Add(1)
		go pc.worker(&wg)
	}

	// 发送任务
	go pc.distributeProxies(proxies)

	// 收集结果 - 添加一个 WaitGroup 来等待结果收集完成
	var collectWg sync.WaitGroup
	collectWg.Add(1)
	go func() {
		pc.collectResults()
		collectWg.Done()
	}()

	wg.Wait()
	close(pc.resultChan)

	// 等待结果收集完成
	collectWg.Wait()
	// 等待进度条显示完成
	time.Sleep(100 * time.Millisecond)

	if config.GlobalConfig.PrintProgress {
		done <- true
	}
	log.SetLevel(log.INFO)
	log.Infoln("共 %v 个可用节点", len(pc.results))
	return pc.results, nil
}

// worker 处理单个代理检测的工作线程
func (pc *ProxyChecker) worker(wg *sync.WaitGroup) {
	defer wg.Done()
	for proxy := range pc.tasks {
		if result := pc.checkProxy(proxy); result != nil {
			pc.resultChan <- *result
		}
		pc.incrementProgress()
	}
}

// checkProxy 检测单个代理
func (pc *ProxyChecker) checkProxy(proxy map[string]any) *Result {
	httpClient := CreateClient(proxy)
	if httpClient == nil {
		return nil
	}
	log.Debugln("开始检测代理: %v", proxy["name"])
	cloudflare, err := platfrom.CheckCloudflare(httpClient)
	if err != nil || !cloudflare {
		return nil
	}

	google, err := platfrom.CheckGoogle(httpClient)
	if err != nil || !google {
		return nil
	}
	var speed int
	if config.GlobalConfig.SpeedTestUrl != "" {
		speed, err = platfrom.CheckSpeed(httpClient)
		if err != nil || speed < config.GlobalConfig.MinSpeed {
			return nil
		}
	}
	// 执行其他平台检测
	openai, _ := platfrom.CheckOpenai(httpClient)
	youtube, _ := platfrom.CheckYoutube(httpClient)
	netflix, _ := platfrom.CheckNetflix(httpClient)
	disney, _ := platfrom.CheckDisney(httpClient)

	// 更新代理名称
	pc.updateProxyName(proxy, httpClient, speed)
	pc.incrementAvailable()

	return &Result{
		Proxy:      proxy,
		Cloudflare: cloudflare,
		Google:     google,
		Openai:     openai,
		Youtube:    youtube,
		Netflix:    netflix,
		Disney:     disney,
	}
}

// updateProxyName 更新代理名称
func (pc *ProxyChecker) updateProxyName(proxy map[string]any, client *http.Client, speed int) {
	// country := proxyutils.GetProxyCountry(client)
	// if country == "" {
	// 	country = "未识别"
	// }

	// 获取速度
	if config.GlobalConfig.SpeedTestUrl != "" {
		var speedStr string
		if speed < 1024 {
			speedStr = fmt.Sprintf("%dKB/s", speed)
		} else {
			speedStr = fmt.Sprintf("%.1fMB/s", float64(speed)/1024)
		}
		proxy["name"] = (proxy["name"]).(string) + " | ⬇️ " + speedStr
	}
}

// showProgress 显示进度条
func (pc *ProxyChecker) showProgress(done chan bool) {
	for {
		select {
		case <-done:
			fmt.Println()
			return
		default:
			current := atomic.LoadInt32(&pc.progress)
			available := atomic.LoadInt32(&pc.available)

			// if 0/0 = NaN ,shoule panic
			percent := float64(current) / float64(pc.proxyCount) * 100
			fmt.Printf("\r进度: [%-50s] %.1f%% (%d/%d) 可用: %d",
				strings.Repeat("=", int(percent/2))+">",
				percent,
				current,
				pc.proxyCount,
				available)
			time.Sleep(100 * time.Millisecond)
		}
	}
}

// 辅助方法
func (pc *ProxyChecker) incrementProgress() {
	atomic.AddInt32(&pc.progress, 1)
}

func (pc *ProxyChecker) incrementAvailable() {
	atomic.AddInt32(&pc.available, 1)
}

// distributeProxies 分发代理任务
func (pc *ProxyChecker) distributeProxies(proxies []map[string]any) {
	for _, proxy := range proxies {
		pc.tasks <- proxy
	}
	close(pc.tasks)
}

// collectResults 收集检测结果
func (pc *ProxyChecker) collectResults() {
	for result := range pc.resultChan {
		pc.results = append(pc.results, result)
	}
}

func CreateClient(mapping map[string]any) *http.Client {
	proxy, err := adapter.ParseProxy(mapping)
	if err != nil {
		return nil
	}

	return &http.Client{
		Timeout: time.Duration(config.GlobalConfig.Timeout) * time.Millisecond,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				host, port, err := net.SplitHostPort(addr)
				if err != nil {
					return nil, err
				}
				var u16Port uint16
				if port, err := strconv.ParseUint(port, 10, 16); err == nil {
					u16Port = uint16(port)
				}
				return proxy.DialContext(ctx, &constant.Metadata{
					Host:    host,
					DstPort: u16Port,
				})
			},
			// 设置连接超时
			IdleConnTimeout: time.Duration(config.GlobalConfig.Timeout) * time.Millisecond,
			// 关闭keepalive
			DisableKeepAlives: true,
		},
	}
}
