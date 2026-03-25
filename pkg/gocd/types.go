package gocd

import (
	"time"
)

// DownloadConfig 下载配置
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

// ProgressFunc 进度回调函数类型
type ProgressFunc func(status ProgressStatus)

// ProgressStatus 进度状态
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

// DownloadResult 下载结果
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

// DownloadStatus 下载状态（用于查询）
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

// DownloadOption 下载选项函数类型
type DownloadOption func(*DownloadConfig)

// DefaultConfig 返回默认配置
func DefaultConfig() DownloadConfig {
	return DownloadConfig{
		Concurrency:    4,
		MaxConnections: 4,
		RetryCount:     3,
		RetryInterval:  time.Second,
		ConnectTimeout: 30 * time.Second,
		ReadTimeout:    30 * time.Second,
		BufferSize:     32 * 1024, // 32KB
		EnableResume:   true,
		UserAgent:      "GoConcurrentDownload/0.1.0",
		MinChunkSize:   1 * 1024 * 1024,  // 1MB
		MaxChunkSize:   10 * 1024 * 1024, // 10MB
		Headers:        make(map[string]string),
	}
}