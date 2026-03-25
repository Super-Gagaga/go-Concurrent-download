package gocd

import (
	"errors"
	"fmt"
)

// 定义错误类型
var (
	// 通用错误
	ErrInvalidURL      = errors.New("invalid URL")
	ErrInvalidPath     = errors.New("invalid file path")
	ErrInvalidConfig   = errors.New("invalid configuration")
	ErrFileExists      = errors.New("file already exists")
	ErrPermissionDenied = errors.New("permission denied")

	// 网络错误
	ErrNetworkError    = errors.New("network error")
	ErrHTTPError       = errors.New("HTTP error")
	ErrTimeout         = errors.New("timeout")
	ErrServerNotSupportRange = errors.New("server does not support range requests")

	// 下载错误
	ErrDownloadFailed  = errors.New("download failed")
	ErrResumeFailed    = errors.New("resume failed")
	ErrChecksumMismatch = errors.New("checksum mismatch")
	ErrIncompleteDownload = errors.New("incomplete download")
	ErrChunkDownloadFailed = errors.New("chunk download failed")

	// 状态错误
	ErrTaskNotFound    = errors.New("task not found")
	ErrTaskAlreadyRunning = errors.New("task already running")
	ErrTaskNotRunning  = errors.New("task is not running")
	ErrTaskPaused      = errors.New("task is paused")
	ErrTaskCancelled   = errors.New("task cancelled")

	// 文件系统错误
	ErrDiskSpace       = errors.New("insufficient disk space")
	ErrIOError         = errors.New("I/O error")
	ErrTempFile        = errors.New("failed to create temporary file")
)

// DownloadError 下载错误，包含更多上下文信息
type DownloadError struct {
	Err     error  // 原始错误
	Message string // 错误消息
	URL     string // 相关URL
	Code    int    // HTTP状态码或错误码
}

func (e *DownloadError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("download error: %s (URL: %s): %v", e.Message, e.URL, e.Err)
	}
	return fmt.Sprintf("download error (URL: %s): %v", e.URL, e.Err)
}

func (e *DownloadError) Unwrap() error {
	return e.Err
}

// NewDownloadError 创建新的下载错误
func NewDownloadError(err error, url string, message string) *DownloadError {
	return &DownloadError{
		Err:     err,
		URL:     url,
		Message: message,
	}
}

// NewHTTPError 创建HTTP错误
func NewHTTPError(statusCode int, url string) *DownloadError {
	return &DownloadError{
		Err:     ErrHTTPError,
		URL:     url,
		Message: fmt.Sprintf("HTTP %d", statusCode),
		Code:    statusCode,
	}
}

// IsRetryableError 判断错误是否可重试
func IsRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// 网络错误、超时错误通常可重试
	if errors.Is(err, ErrNetworkError) ||
		errors.Is(err, ErrTimeout) ||
		errors.Is(err, ErrChunkDownloadFailed) {
		return true
	}

	// HTTP 5xx 错误可重试
	var downloadErr *DownloadError
	if errors.As(err, &downloadErr) {
		// HTTP 状态码 5xx 或 429（太多请求）可重试
		if downloadErr.Code >= 500 && downloadErr.Code < 600 {
			return true
		}
		if downloadErr.Code == 429 { // Too Many Requests
			return true
		}
	}

	return false
}

// IsFatalError 判断错误是否致命（不应重试）
func IsFatalError(err error) bool {
	if err == nil {
		return false
	}

	// 权限错误、校验和错误、无效URL等不应重试
	if errors.Is(err, ErrPermissionDenied) ||
		errors.Is(err, ErrChecksumMismatch) ||
		errors.Is(err, ErrInvalidURL) ||
		errors.Is(err, ErrInvalidPath) ||
		errors.Is(err, ErrDiskSpace) {
		return true
	}

	// HTTP 4xx 错误（除429外）通常不应重试
	var downloadErr *DownloadError
	if errors.As(err, &downloadErr) {
		if downloadErr.Code >= 400 && downloadErr.Code < 500 && downloadErr.Code != 429 {
			return true
		}
	}

	return false
}