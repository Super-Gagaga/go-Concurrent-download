package httpclient

import (
	"context"
	"errors"
	"io"
	"math"
	"net/http"
	"time"

	"github.com/Super-Gagaga/go-Concurrent-download/pkg/gocd"
)

// RetryConfig 重试配置
type RetryConfig struct {
	MaxRetries      int           // 最大重试次数
	BaseDelay       time.Duration // 基础延迟
	MaxDelay        time.Duration // 最大延迟
	BackoffFactor   float64       // 退避因子
	JitterFactor    float64       // 抖动因子（0-1）
	RetryableStatus []int         // 可重试的状态码
}

// DefaultRetryConfig 返回默认重试配置
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:      3,
		BaseDelay:       1 * time.Second,
		MaxDelay:        30 * time.Second,
		BackoffFactor:   2.0,
		JitterFactor:    0.1,
		RetryableStatus: []int{429, 500, 502, 503, 504},
	}
}

// RetryableFunc 可重试的函数类型
type RetryableFunc func() (interface{}, error)

// Retry 执行带重试的函数
func Retry(ctx context.Context, config RetryConfig, fn RetryableFunc) (interface{}, error) {
	var lastErr error

	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		if attempt > 0 {
			// 计算延迟时间
			delay := calculateDelay(config, attempt)

			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}

		result, err := fn()
		if err == nil {
			return result, nil
		}

		lastErr = err

		// 检查是否应该重试
		if !shouldRetry(err, config, attempt) {
			return nil, err
		}
	}

	return nil, lastErr
}

// RetryHTTP 执行带重试的HTTP请求
func RetryHTTP(ctx context.Context, config RetryConfig, client *http.Client, req *http.Request) (*http.Response, error) {
	var lastResp *http.Response
	var lastErr error

	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		if attempt > 0 {
			// 计算延迟时间
			delay := calculateDelay(config, attempt)

			select {
			case <-ctx.Done():
				if lastResp != nil {
					lastResp.Body.Close()
				}
				return nil, ctx.Err()
			case <-time.After(delay):
			}

			// 重试前可能需要重新创建请求体
			if req.Body != nil && req.GetBody != nil {
				if body, err := req.GetBody(); err == nil {
					req.Body = body
				}
			}
		}

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			if gocd.IsRetryableError(err) && attempt < config.MaxRetries {
				continue
			}
			return nil, err
		}

		// 检查状态码
		if isSuccessStatus(resp.StatusCode) {
			return resp, nil
		}

		lastResp = resp
		lastErr = gocd.NewHTTPError(resp.StatusCode, req.URL.String())

		// 检查是否应该重试
		if shouldRetryHTTP(resp.StatusCode, config, attempt) {
			resp.Body.Close()
			continue
		}

		// 不应该重试，返回错误
		return resp, nil
	}

	if lastResp != nil {
		lastResp.Body.Close()
	}
	return nil, lastErr
}

// RetryRangeDownload 带重试的范围下载
func RetryRangeDownload(ctx context.Context, config RetryConfig, client HTTPClient, url string, start, end int64) (io.ReadCloser, int64, error) {
	var lastErr error

	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		if attempt > 0 {
			// 计算延迟时间
			delay := calculateDelay(config, attempt)

			select {
			case <-ctx.Done():
				return nil, 0, ctx.Err()
			case <-time.After(delay):
			}
		}

		reader, contentLength, err := client.GetRange(ctx, url, start, end)
		if err == nil {
			return reader, contentLength, nil
		}

		lastErr = err

		// 检查是否应该重试
		if !shouldRetry(err, config, attempt) {
			return nil, 0, err
		}
	}

	return nil, 0, lastErr
}

// calculateDelay 计算重试延迟时间（指数退避 + 抖动）
func calculateDelay(config RetryConfig, attempt int) time.Duration {
	if attempt == 0 {
		return 0
	}

	// 指数退避
	delay := float64(config.BaseDelay) * math.Pow(config.BackoffFactor, float64(attempt-1))

	// 限制最大延迟
	if delay > float64(config.MaxDelay) {
		delay = float64(config.MaxDelay)
	}

	// 添加抖动
	if config.JitterFactor > 0 {
		jitter := delay * config.JitterFactor
		delay = delay - jitter/2 + (float64(time.Now().UnixNano()) * jitter / math.MaxInt64)
	}

	return time.Duration(delay)
}

// shouldRetry 检查错误是否应该重试
func shouldRetry(err error, config RetryConfig, attempt int) bool {
	if attempt >= config.MaxRetries {
		return false
	}

	if err == nil {
		return false
	}

	// 使用库的错误检查函数
	if gocd.IsFatalError(err) {
		return false
	}

	// 检查是否为HTTP错误
	var downloadErr *gocd.DownloadError
	if errors.As(err, &downloadErr) {
		return shouldRetryHTTP(downloadErr.Code, config, attempt)
	}

	// 默认情况下，网络错误可以重试
	return gocd.IsRetryableError(err)
}

// shouldRetryHTTP 检查HTTP状态码是否应该重试
func shouldRetryHTTP(statusCode int, config RetryConfig, attempt int) bool {
	if attempt >= config.MaxRetries {
		return false
	}

	// 检查是否在可重试状态码列表中
	for _, code := range config.RetryableStatus {
		if statusCode == code {
			return true
		}
	}

	// 5xx服务器错误可以重试
	if statusCode >= 500 && statusCode < 600 {
		return true
	}

	// 429 Too Many Requests 可以重试
	if statusCode == 429 {
		return true
	}

	return false
}

// isSuccessStatus 检查是否为成功状态码
func isSuccessStatus(statusCode int) bool {
	return statusCode >= 200 && statusCode < 300
}

