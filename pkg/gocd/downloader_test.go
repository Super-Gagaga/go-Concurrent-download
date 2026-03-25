package gocd

import (
	"testing"
	"time"
)

// TestDefaultConfig 测试默认配置
func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config.Concurrency != 4 {
		t.Errorf("Expected Concurrency 4, got %d", config.Concurrency)
	}
	if config.RetryCount != 3 {
		t.Errorf("Expected RetryCount 3, got %d", config.RetryCount)
	}
	if config.BufferSize != 32*1024 {
		t.Errorf("Expected BufferSize 32KB, got %d", config.BufferSize)
	}
	if !config.EnableResume {
		t.Error("Expected EnableResume true")
	}
	if config.UserAgent != "GoConcurrentDownload/0.1.0" {
		t.Errorf("Expected UserAgent 'GoConcurrentDownload/0.1.0', got '%s'", config.UserAgent)
	}
}

// TestWithConcurrency 测试并发数选项
func TestWithConcurrency(t *testing.T) {
	config := DefaultConfig()
	WithConcurrency(8)(&config)

	if config.Concurrency != 8 {
		t.Errorf("Expected Concurrency 8, got %d", config.Concurrency)
	}
	if config.MaxConnections != 8 {
		t.Errorf("Expected MaxConnections 8, got %d", config.MaxConnections)
	}
}

// TestWithRetryCount 测试重试次数选项
func TestWithRetryCount(t *testing.T) {
	config := DefaultConfig()
	WithRetryCount(5)(&config)

	if config.RetryCount != 5 {
		t.Errorf("Expected RetryCount 5, got %d", config.RetryCount)
	}
}

// TestWithProgressFunc 测试进度回调选项
func TestWithProgressFunc(t *testing.T) {
	config := DefaultConfig()
	called := false
	progressFunc := func(status ProgressStatus) {
		called = true
	}
	WithProgressFunc(progressFunc)(&config)

	if config.ProgressFunc == nil {
		t.Error("Expected ProgressFunc not nil")
	}

	// 测试调用
	if config.ProgressFunc != nil {
		config.ProgressFunc(ProgressStatus{})
		if !called {
			t.Error("ProgressFunc should have been called")
		}
	}
}

// TestWithChecksum 测试校验和选项
func TestWithChecksum(t *testing.T) {
	config := DefaultConfig()
	WithChecksum("abc123", "md5")(&config)

	if config.Checksum != "abc123" {
		t.Errorf("Expected Checksum 'abc123', got '%s'", config.Checksum)
	}
	if config.ChecksumType != "md5" {
		t.Errorf("Expected ChecksumType 'md5', got '%s'", config.ChecksumType)
	}
}

// TestWithHeaders 测试HTTP头部选项
func TestWithHeaders(t *testing.T) {
	config := DefaultConfig()
	headers := map[string]string{
		"Authorization": "Bearer token",
		"User-Agent":    "TestAgent",
	}
	WithHeaders(headers)(&config)

	if config.Headers["Authorization"] != "Bearer token" {
		t.Errorf("Expected Authorization header 'Bearer token', got '%s'", config.Headers["Authorization"])
	}
	if config.Headers["User-Agent"] != "TestAgent" {
		t.Errorf("Expected User-Agent header 'TestAgent', got '%s'", config.Headers["User-Agent"])
	}
}

// TestWithUserAgent 测试用户代理选项
func TestWithUserAgent(t *testing.T) {
	config := DefaultConfig()
	WithUserAgent("CustomAgent/1.0")(&config)

	if config.UserAgent != "CustomAgent/1.0" {
		t.Errorf("Expected UserAgent 'CustomAgent/1.0', got '%s'", config.UserAgent)
	}
}

// TestWithRateLimit 测试速率限制选项
func TestWithRateLimit(t *testing.T) {
	config := DefaultConfig()
	WithRateLimit(1024 * 1024)(&config) // 1MB/s

	if config.RateLimit != 1024*1024 {
		t.Errorf("Expected RateLimit 1048576, got %d", config.RateLimit)
	}
}

// TestWithTimeout 测试超时选项
func TestWithTimeout(t *testing.T) {
	config := DefaultConfig()
	WithTimeout(30 * time.Second)(&config)

	if config.Timeout != 30*time.Second {
		t.Errorf("Expected Timeout 30s, got %v", config.Timeout)
	}
}

// TestNewDownloader 测试创建下载器
func TestNewDownloader(t *testing.T) {
	downloader := NewDownloader()
	if downloader == nil {
		t.Error("Expected downloader not nil")
	}

	// 测试带选项的创建
	downloader2 := NewDownloader(
		WithConcurrency(8),
		WithRetryCount(5),
	)
	if downloader2 == nil {
		t.Error("Expected downloader2 not nil")
	}
}

// TestErrorTypes 测试错误类型
func TestErrorTypes(t *testing.T) {
	if ErrInvalidURL.Error() != "invalid URL" {
		t.Errorf("Expected ErrInvalidURL 'invalid URL', got '%s'", ErrInvalidURL.Error())
	}
	if ErrNetworkError.Error() != "network error" {
		t.Errorf("Expected ErrNetworkError 'network error', got '%s'", ErrNetworkError.Error())
	}
	if ErrDownloadFailed.Error() != "download failed" {
		t.Errorf("Expected ErrDownloadFailed 'download failed', got '%s'", ErrDownloadFailed.Error())
	}
}

// TestIsRetryableError 测试可重试错误判断
func TestIsRetryableError(t *testing.T) {
	if !IsRetryableError(ErrNetworkError) {
		t.Error("ErrNetworkError should be retryable")
	}
	if !IsRetryableError(ErrTimeout) {
		t.Error("ErrTimeout should be retryable")
	}
	if IsRetryableError(ErrInvalidURL) {
		t.Error("ErrInvalidURL should not be retryable")
	}
	if IsRetryableError(ErrPermissionDenied) {
		t.Error("ErrPermissionDenied should not be retryable")
	}
}

// TestIsFatalError 测试致命错误判断
func TestIsFatalError(t *testing.T) {
	if !IsFatalError(ErrInvalidURL) {
		t.Error("ErrInvalidURL should be fatal")
	}
	if !IsFatalError(ErrPermissionDenied) {
		t.Error("ErrPermissionDenied should be fatal")
	}
	if !IsFatalError(ErrChecksumMismatch) {
		t.Error("ErrChecksumMismatch should be fatal")
	}
	if IsFatalError(ErrNetworkError) {
		t.Error("ErrNetworkError should not be fatal")
	}
}

// TestNewDownloadError 测试创建下载错误
func TestNewDownloadError(t *testing.T) {
	err := NewDownloadError(ErrNetworkError, "http://example.com", "connection failed")
	if err == nil {
		t.Error("Expected error not nil")
	}

	if err.URL != "http://example.com" {
		t.Errorf("Expected URL 'http://example.com', got '%s'", err.URL)
	}
	if err.Message != "connection failed" {
		t.Errorf("Expected message 'connection failed', got '%s'", err.Message)
	}
}

// TestNewHTTPError 测试创建HTTP错误
func TestNewHTTPError(t *testing.T) {
	err := NewHTTPError(404, "http://example.com/file")
	if err == nil {
		t.Error("Expected error not nil")
	}

	if err.Code != 404 {
		t.Errorf("Expected code 404, got %d", err.Code)
	}
	if err.URL != "http://example.com/file" {
		t.Errorf("Expected URL 'http://example.com/file', got '%s'", err.URL)
	}
}