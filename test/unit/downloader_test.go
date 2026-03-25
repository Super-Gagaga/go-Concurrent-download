package unit

import (
	"testing"

	"github.com/yourusername/go-concurrent-download/pkg/gocd"
)

func TestDefaultConfig(t *testing.T) {
	config := gocd.DefaultConfig()

	if config.Concurrency != 4 {
		t.Errorf("默认并发数应为4，实际为%d", config.Concurrency)
	}

	if config.RetryCount != 3 {
		t.Errorf("默认重试次数应为3，实际为%d", config.RetryCount)
	}

	if config.BufferSize != 32*1024 {
		t.Errorf("默认缓冲区大小应为32KB，实际为%d", config.BufferSize)
	}

	if !config.EnableResume {
		t.Error("默认应启用断点续传")
	}
}

func TestDownloadOptionFunctions(t *testing.T) {
	config := gocd.DefaultConfig()

	// 测试并发数选项
	gocd.WithConcurrency(8)(&config)
	if config.Concurrency != 8 {
		t.Errorf("WithConcurrency 应设置并发数为8，实际为%d", config.Concurrency)
	}

	// 测试重试次数选项
	gocd.WithRetryCount(5)(&config)
	if config.RetryCount != 5 {
		t.Errorf("WithRetryCount 应设置重试次数为5，实际为%d", config.RetryCount)
	}

	// 测试进度回调选项
	called := false
	progressFunc := func(status gocd.ProgressStatus) {
		called = true
	}
	gocd.WithProgressFunc(progressFunc)(&config)
	if config.ProgressFunc == nil {
		t.Error("WithProgressFunc 应设置进度回调函数")
	}
}

func TestErrorTypes(t *testing.T) {
	// 测试错误类型
	err := gocd.ErrInvalidURL
	if err == nil {
		t.Error("ErrInvalidURL 不应为nil")
	}

	if err.Error() != "invalid URL" {
		t.Errorf("ErrInvalidURL 错误消息应为'invalid URL'，实际为'%s'", err.Error())
	}

	// 测试可重试错误判断
	if gocd.IsRetryableError(gocd.ErrNetworkError) != true {
		t.Error("ErrNetworkError 应被识别为可重试错误")
	}

	if gocd.IsRetryableError(gocd.ErrInvalidURL) != false {
		t.Error("ErrInvalidURL 不应被识别为可重试错误")
	}
}

func TestNewDownloader(t *testing.T) {
	downloader := gocd.NewDownloader()
	if downloader == nil {
		t.Error("NewDownloader 应返回非nil的下载器")
	}

	// 测试使用选项创建下载器
	downloader2 := gocd.NewDownloader(
		gocd.WithConcurrency(10),
		gocd.WithRetryCount(2),
	)
	if downloader2 == nil {
		t.Error("带选项的NewDownloader 应返回非nil的下载器")
	}
}

// 测试占位符，实际实现后需要更多测试
func TestDownloadFunctionPlaceholder(t *testing.T) {
	// 目前只是占位符测试
	t.Log("下载功能测试占位符 - 实际实现后需要完整测试")
}