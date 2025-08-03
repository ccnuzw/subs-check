# Android (NDK) 编译指南

本文档旨在说明如何将此目录下的 Go 项目编译为可供 Android 应用使用的 `.aar` 库文件。

## 编译前提

1.  **Go 环境**: 确保您已正确安装 Go 语言环境。
2.  **Android NDK**: `gomobile` 工具的必要依赖。通常 `gomobile init` 会自动下载安装。
3.  **JDK (Java Development Kit)**: 编译 Java 封装代码时需要。您需要知道其 `bin` 目录的路径。
4.  **`gomobile` 工具**: 用于编译的核心工具。

## 编译步骤

### 1. 准备 Go 源码包

`gomobile bind` 命令需要的是一个库包，而不是一个 `main` 包。在编译前，必须对 Go 源码进行以下修改：

-   所有 `.go` 文件中的包声明必须是库名称（例如 `package subslib`），而不是 `package main`。
-   必须移除或注释掉 `main.go` 文件中的 `func main()` 函数。
-   必须移除因 `main()` 函数被注释而产生的未使用的 `import` 语句，否则 Go 编译器会报错。

### 2. 安装 `gomobile` 及其依赖

如果您尚未安装 `gomobile`，请在 `subslib` 目录下执行以下命令：

```bash
# 安装 gomobile 工具
go install golang.org/x/mobile/cmd/gomobile@latest

# 初始化 gomobile (会自动下载 NDK 等)
gomobile init

# 获取 gomobile bind 命令所需的依赖包
go get golang.org/x/mobile/bind
```

### 3. 整理 Go 模块

为避免潜在的依赖问题，建议清理模块缓存并重新整理依赖。

```bash
# (可选) 清理模块缓存，有助于解决一些疑难杂症
go clean -modcache

# 在 subslib 目录下整理依赖
go mod tidy
```

### 4. 执行编译命令

最终的编译命令需要 Java 编译器 `javac`。为此，需要将 JDK 的 `bin` 目录临时添加到系统的 `PATH` 环境变量中。

**注意:** 以下命令必须在 `subslib` 目录中执行。请将 `D:\Android Studio\jbr\bin` 替换为您自己电脑上 JDK `bin` 目录的实际路径。

#### 适用于 Windows PowerShell:

```powershell
# 在当前会话中临时将 JDK 添加到 PATH，然后执行编译
$env:PATH += ";D:\Android Studio\jbr\bin"; gomobile bind -o ..\app\libs\subslib.aar -target=android .
```

#### 适用于 Windows 命令提示符 (cmd.exe):

```cmd
# 在当前会话中临时将 JDK 添加到 PATH，然后执行编译
set "PATH=%PATH%;D:\Android Studio\jbr\bin" && gomobile bind -o ..\app\libs\subslib.aar -target=android .
```

### 5. 查找输出文件

编译成功后，生成的 `subslib.aar` 文件会位于 `subslib` 目录的上级目录中的 `app/libs/` 路径下。之后，您便可以将这个 `.aar` 文件集成到您的 Android Studio 项目中。
