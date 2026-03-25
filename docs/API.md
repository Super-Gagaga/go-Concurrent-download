# API 文档

## 概述

Go 并发文件下载工具提供了简洁易用的 API，支持并发下载、断点续传、进度监控等功能。本文档详细介绍了所有可用的接口和配置选项。

## 快速开始

### 导入包

```go
import "github.com/Super-Gagaga/go-Concurrent-download/pkg/gocd"
```

### 最简单的下载

```go
err := gocd.Download("https://example.com/file.zip", "./file.zip")
if err != nil {
    log.Fatal(err)
}
```

### 带配置的下载

```go
config := gocd.DownloadConfig{
    Concurrency: 8,
    RetryCount:  3,
    ProgressFunc: func(status gocd.ProgressStatus) {
        fmt.Printf("进度: %.2f%%\n", status.Percentage)
    },
}

err := gocd.DownloadWithConfig("https://example.com/file.zip", "./file.zip", config)
```

## 核心接口

### Downloader 接口

`Downloader` 是核心接口，提供了完整的下载控制功能：

```go
type Downloader interface {
    // 下载单个文件
    Download(url, destPath string, options ...DownloadOption) error

    // 使用配置下载文件
    DownloadWithConfig(url, destPath string, config DownloadConfig) error

    // 批量下载多个文件
    DownloadBatch(urls []string, destDir string, options ...DownloadOption) []DownloadResult

    // 暂停下载任务
    Pause(taskID string) error

    // 恢复下载任务
    Resume(taskID string) error

    // 取消下载任务
    Cancel(taskID string) error

    // 获取下载状态
    GetStatus(taskID string) (DownloadStatus, error)

    // 列出所有任务
    ListTasks() []DownloadStatus

    // 等待所有任务完成
    Wait() error
}
```

### 创建下载器实例

```go
// 使用默认配置
downloader := gocd.NewDownloader()

// 使用自定义配置
downloader := gocd.NewDownloader(
    gocd.WithConcurrency(8),
    gocd.WithRetryCount(5),
    gocd.WithProgressFunc(myProgressFunc),
)
```

## 配置系统

### DownloadConfig 结构

`DownloadConfig` 包含了所有的下载配置选项：

```go
type DownloadConfig struct {
    // 并发设置
    Concurrency    int           // 并发数，默认4
    MaxConnections int           // 最大连接数（兼容性别名）

    // 重试设置
    RetryCount    int           // 重试次数，默认3
    RetryInterval time.Duration // 重试间隔，默认1秒

    // 超时设置
    Timeout        time.Duration // 总超时时间，默认0（无限制）
    ConnectTimeout time.Duration // 连接超时，默认30秒
    ReadTimeout    time.Duration // 读取超时，默认30秒

    // 缓冲区设置
    BufferSize int // 缓冲区大小（字节），默认32KB

    // 进度回调
    ProgressFunc ProgressFunc // 进度回调函数

    // 断点续传
    EnableResume bool   // 启用断点续传，默认true
    StateFile    string // 状态文件路径

    // 校验和验证
    Checksum     string // 校验和值
    ChecksumType string // 校验和类型：md5, sha1, sha256, sha512

    // HTTP 头部
    Headers map[string]string // 自定义HTTP头部

    // 用户代理
    UserAgent string // User-Agent头部，默认使用库标识

    // 限速
    RateLimit int64 // 下载速度限制（字节/秒），默认0（无限制）

    // 分片设置
    MinChunkSize int64 // 最小分片大小，默认1MB
    MaxChunkSize int64 // 最大分片大小，默认10MB

    // 临时文件
    TempDir string // 临时文件目录
}
```

### 配置选项函数

为了方便配置，提供了一系列选项函数：

```go
// 设置并发数
WithConcurrency(n int)

// 设置重试次数
WithRetryCount(n int)

// 设置重试间隔
WithRetryInterval(d time.Duration)

// 设置进度回调函数
WithProgressFunc(f ProgressFunc)

// 设置校验和
WithChecksum(checksum, checksumType string)

// 设置HTTP头部
WithHeaders(headers map[string]string)

// 设置User-Agent
WithUserAgent(ua string)

// 设置下载速度限制（字节/秒）
WithRateLimit(limit int64)

// 设置超时时间
WithTimeout(timeout time.Duration)
```

使用示例：

```go
downloader := gocd.NewDownloader(
    gocd.WithConcurrency(8),
    gocd.WithRetryCount(5),
    gocd.WithProgressFunc(myProgressFunc),
    gocd.WithHeaders(map[string]string{
        "Authorization": "Bearer token",
    }),
)
```

## 进度监控

### ProgressFunc 类型

进度回调函数的类型定义：

```go
type ProgressFunc func(status ProgressStatus)
```

### ProgressStatus 结构

进度状态包含下载的详细信息：

```go
type ProgressStatus struct {
    TaskID         string        // 任务ID
    URL            string        // 下载URL
    FileName       string        // 文件名
    TotalBytes     int64         // 总字节数
    Downloaded     int64         // 已下载字节数
    Percentage     float64       // 百分比（0-100）
    Speed          int64         // 下载速度（字节/秒）
    RemainingTime  time.Duration // 剩余时间
    IsCompleted    bool          // 是否完成
    Error          error         // 错误信息
    StartTime      time.Time     // 开始时间
    LastUpdateTime time.Time     // 最后更新时间
}
```

### 进度回调示例

```go
progressFunc := func(status gocd.ProgressStatus) {
    if status.IsCompleted {
        fmt.Printf("下载完成: %s\n", status.FileName)
        return
    }
    if status.Error != nil {
        fmt.Printf("下载错误: %v\n", status.Error)
        return
    }

    fmt.Printf("\r进度: %6.2f%% | 速度: %8s/s | 剩余: %v",
        status.Percentage,
        formatBytes(status.Speed),
        formatDuration(status.RemainingTime),
    )
}
```

## 批量下载

### DownloadBatch 方法

`DownloadBatch` 方法支持批量下载多个文件：

```go
urls := []string{
    "https://example.com/file1.zip",
    "https://example.com/file2.zip",
    "https://example.com/file3.zip",
}

results := downloader.DownloadBatch(urls, "./downloads",
    gocd.WithConcurrency(4),
    gocd.WithRetryCount(2),
)

for _, result := range results {
    if result.Success {
        fmt.Printf("下载成功: %s\n", result.URL)
    } else {
        fmt.Printf("下载失败: %s (%v)\n", result.URL, result.Error)
    }
}
```

### DownloadResult 结构

批量下载的结果：

```go
type DownloadResult struct {
    TaskID     string        // 任务ID
    URL        string        // 下载URL
    FilePath   string        // 文件路径
    Success    bool          // 是否成功
    Error      error         // 错误信息
    Size       int64         // 文件大小
    Duration   time.Duration // 下载耗时
    Checksum   string        // 计算出的校验和
}
```

## 任务管理

### 任务控制

下载器提供了完整的任务控制功能：

```go
// 开始下载任务
taskID, err := downloader.StartDownload(url, destPath, options...)

// 暂停任务
err := downloader.Pause(taskID)

// 恢复任务
err := downloader.Resume(taskID)

// 取消任务
err := downloader.Cancel(taskID)

// 获取任务状态
status, err := downloader.GetStatus(taskID)

// 列出所有任务
tasks := downloader.ListTasks()

// 等待所有任务完成
err := downloader.Wait()
```

### DownloadStatus 结构

任务状态的详细信息：

```go
type DownloadStatus struct {
    TaskID      string        // 任务ID
    URL         string        // 下载URL
    FilePath    string        // 文件路径
    TotalBytes  int64         // 总字节数
    Downloaded  int64         // 已下载字节数
    Percentage  float64       // 百分比
    Speed       int64         // 当前速度
    Remaining   time.Duration // 剩余时间
    IsPaused    bool          // 是否暂停
    IsCompleted bool          // 是否完成
    IsFailed    bool          // 是否失败
    Error       string        // 错误信息
    StartTime   time.Time     // 开始时间
}
```

## 错误处理

### 错误类型

库定义了一系列错误类型：

```go
// 通用错误
ErrInvalidURL      = errors.New("invalid URL")
ErrInvalidPath     = errors.New("invalid file path")
ErrInvalidConfig   = errors.New("invalid configuration")

// 网络错误
ErrNetworkError    = errors.New("network error")
ErrHTTPError       = errors.New("HTTP error")
ErrTimeout         = errors.New("timeout")

// 下载错误
ErrDownloadFailed  = errors.New("download failed")
ErrResumeFailed    = errors.New("resume failed")
ErrChecksumMismatch = errors.New("checksum mismatch")

// 状态错误
ErrTaskNotFound    = errors.New("task not found")
ErrTaskAlreadyRunning = errors.New("task already running")
```

### DownloadError 类型

`DownloadError` 提供了更详细的错误信息：

```go
type DownloadError struct {
    Err     error  // 原始错误
    Message string // 错误消息
    URL     string // 相关URL
    Code    int    // HTTP状态码或错误码
}
```

### 错误检查函数

```go
// 判断错误是否可重试
IsRetryableError(err error) bool

// 判断错误是否致命（不应重试）
IsFatalError(err error) bool
```

使用示例：

```go
err := downloader.Download(url, destPath)
if err != nil {
    if gocd.IsRetryableError(err) {
        fmt.Println("可重试错误，正在重试...")
        // 重试逻辑
    } else if gocd.IsFatalError(err) {
        fmt.Println("致命错误，停止重试")
        // 处理致命错误
    } else {
        fmt.Printf("其他错误: %v\n", err)
    }
}
```

## 高级用法

### 断点续传

```go
// 启用断点续传
downloader := gocd.NewDownloader(
    gocd.WithResume(true),
    gocd.WithStateFile("./download-state.json"),
)

// 开始下载（如果中断，可以重新运行继续下载）
err := downloader.Download(url, destPath)
```

### 校验和验证

```go
// 下载时验证校验和
err := gocd.DownloadWithConfig(url, destPath, gocd.DownloadConfig{
    Checksum:     "a1b2c3d4e5f67890",
    ChecksumType: "md5",
})

if errors.Is(err, gocd.ErrChecksumMismatch) {
    fmt.Println("文件校验失败，可能已损坏")
}
```

### 限速下载

```go
// 限制下载速度为 1MB/s
downloader := gocd.NewDownloader(
    gocd.WithRateLimit(1024 * 1024), // 1MB/s
)
```

### 自定义 HTTP 头部

```go
downloader := gocd.NewDownloader(
    gocd.WithHeaders(map[string]string{
        "Authorization": "Bearer your-token",
        "User-Agent":    "MyApp/1.0",
        "Referer":       "https://example.com",
    }),
)
```

## 工具函数

### 快速下载函数

库提供了一些快速下载函数，适用于简单场景：

```go
// 快速下载单个文件
err := gocd.Download(url, destPath)

// 快速下载带配置
err := gocd.DownloadWithConfig(url, destPath, config)

// 快速批量下载
results := gocd.DownloadBatch(urls, destDir, options...)
```

### 辅助函数

```go
// 格式化字节显示
func formatBytes(bytes int64) string {
    const unit = 1024
    if bytes < unit {
        return fmt.Sprintf("%d B", bytes)
    }
    div, exp := int64(unit), 0
    for n := bytes / unit; n >= unit; n /= unit {
        div *= unit
        exp++
    }
    return fmt.Sprintf("%.1f %ciB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// 格式化时间显示
func formatDuration(d time.Duration) string {
    if d < time.Minute {
        return fmt.Sprintf("%.0fs", d.Seconds())
    }
    if d < time.Hour {
        return fmt.Sprintf("%.0fm%.0fs", d.Minutes(), d.Seconds()%60)
    }
    return fmt.Sprintf("%.0fh%.0fm", d.Hours(), d.Minutes()%60)
}
```

## 最佳实践

### 1. 合理设置并发数

```go
// 根据网络条件和服务器限制设置
downloader := gocd.NewDownloader(
    gocd.WithConcurrency(4), // 一般4-8个并发比较合适
)
```

### 2. 使用进度回调

```go
downloader := gocd.NewDownloader(
    gocd.WithProgressFunc(func(status gocd.ProgressStatus) {
        // 更新UI或记录日志
        log.Printf("进度: %.2f%%, 速度: %s/s",
            status.Percentage,
            formatBytes(status.Speed),
        )
    }),
)
```

### 3. 处理错误和重试

```go
err := downloader.Download(url, destPath)
if err != nil {
    if gocd.IsRetryableError(err) {
        // 实现重试逻辑
        for i := 0; i < maxRetries; i++ {
            time.Sleep(retryInterval)
            err = downloader.Download(url, destPath)
            if err == nil {
                break
            }
        }
    }
    // 处理其他错误
}
```

### 4. 使用断点续传

```go
// 对于大文件下载，始终启用断点续传
downloader := gocd.NewDownloader(
    gocd.WithResume(true),
    gocd.WithStateFile(fmt.Sprintf("./%s.state", filename)),
)
```

## 常见问题

### Q: 如何下载需要认证的文件？
A: 使用 `WithHeaders` 设置认证头部：

```go
downloader := gocd.NewDownloader(
    gocd.WithHeaders(map[string]string{
        "Authorization": "Basic " + base64.StdEncoding.EncodeToString([]byte("user:pass")),
    }),
)
```

### Q: 如何限制下载速度？
A: 使用 `WithRateLimit`：

```go
// 限制为 500KB/s
downloader := gocd.NewDownloader(
    gocd.WithRateLimit(500 * 1024),
)
```

### Q: 如何验证文件完整性？
A: 使用校验和验证：

```go
err := gocd.DownloadWithConfig(url, destPath, gocd.DownloadConfig{
    Checksum:     "expected-md5-or-sha",
    ChecksumType: "md5", // 或 "sha256"
})
```

### Q: 如何恢复中断的下载？
A: 启用断点续传并使用相同的状态文件：

```go
downloader := gocd.NewDownloader(
    gocd.WithResume(true),
    gocd.WithStateFile("./download.state"),
)
// 重新运行下载，会自动从断点继续
err := downloader.Download(url, destPath)
```

---

**注意**: 本文档基于 API 版本 0.1.0，API 可能会有变动。请参考代码注释获取最新信息。