package httpclient

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/Super-Gagaga/go-Concurrent-download/pkg/gocd"
)

// HTTPClient HTTP客户端接口
type HTTPClient interface {
	// Do 执行HTTP请求
	Do(req *http.Request) (*http.Response, error)

	// Head 发送HEAD请求，获取文件信息
	Head(ctx context.Context, url string) (*FileInfo, error)

	// GetRange 获取文件范围数据
	GetRange(ctx context.Context, url string, start, end int64) (io.ReadCloser, int64, error)

	// GetWithRetry 带重试的GET请求
	GetWithRetry(ctx context.Context, url string, retryCount int, retryInterval time.Duration) (io.ReadCloser, int64, error)

	// Close 关闭客户端
	Close() error
}

// FileInfo 文件信息
type FileInfo struct {
	URL               string
	ContentLength     int64
	AcceptRanges      bool
	LastModified      time.Time
	ETag              string
	ContentType       string
	SupportRange      bool // 服务器是否支持Range请求
}

// ClientConfig 客户端配置
type ClientConfig struct {
	// 超时设置
	ConnectTimeout time.Duration
	ReadTimeout    time.Duration
	IdleTimeout    time.Duration

	// 连接池设置
	MaxIdleConns        int
	MaxIdleConnsPerHost int
	MaxConnsPerHost     int

	// 重试设置
	RetryCount    int
	RetryInterval time.Duration

	// 用户代理
	UserAgent string

	// 自定义头部
	Headers map[string]string

	// 代理设置
	ProxyURL string
}

// DefaultClientConfig 返回默认客户端配置
func DefaultClientConfig() ClientConfig {
	return ClientConfig{
		ConnectTimeout:       30 * time.Second,
		ReadTimeout:          30 * time.Second,
		IdleTimeout:          90 * time.Second,
		MaxIdleConns:         100,
		MaxIdleConnsPerHost:  10,
		MaxConnsPerHost:      0, // 0表示无限制
		RetryCount:           3,
		RetryInterval:        time.Second,
		UserAgent:           "GoConcurrentDownload/0.1.0",
		Headers:             make(map[string]string),
	}
}

// httpClientImpl HTTP客户端实现
type httpClientImpl struct {
	client    *http.Client
	config    ClientConfig
	transport *http.Transport
}

// NewClient 创建新的HTTP客户端
func NewClient(config ClientConfig) (HTTPClient, error) {
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   config.ConnectTimeout,
			KeepAlive: 30 * time.Second,
			DualStack: true,
		}).DialContext,
		MaxIdleConns:        config.MaxIdleConns,
		MaxIdleConnsPerHost: config.MaxIdleConnsPerHost,
		MaxConnsPerHost:     config.MaxConnsPerHost,
		IdleConnTimeout:     config.IdleTimeout,
		TLSHandshakeTimeout: 10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	// 设置代理
	if config.ProxyURL != "" {
		proxyURL, err := url.Parse(config.ProxyURL)
		if err != nil {
			return nil, gocd.NewDownloadError(err, "", fmt.Sprintf("invalid proxy URL: %s", config.ProxyURL))
		}
		transport.Proxy = http.ProxyURL(proxyURL)
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   config.ReadTimeout,
	}

	return &httpClientImpl{
		client:    client,
		config:    config,
		transport: transport,
	}, nil
}

// Do 执行HTTP请求
func (c *httpClientImpl) Do(req *http.Request) (*http.Response, error) {
	// 设置用户代理
	if c.config.UserAgent != "" && req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", c.config.UserAgent)
	}

	// 设置自定义头部
	for k, v := range c.config.Headers {
		req.Header.Set(k, v)
	}

	return c.client.Do(req)
}

// Head 发送HEAD请求获取文件信息
func (c *httpClientImpl) Head(ctx context.Context, url string) (*FileInfo, error) {
	req, err := http.NewRequestWithContext(ctx, "HEAD", url, nil)
	if err != nil {
		return nil, gocd.NewDownloadError(err, url, "failed to create HEAD request")
	}

	resp, err := c.Do(req)
	if err != nil {
		return nil, gocd.NewDownloadError(err, url, "HEAD request failed")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, gocd.NewHTTPError(resp.StatusCode, url)
	}

	// 解析文件信息
	info := &FileInfo{
		URL:           url,
		ContentLength: resp.ContentLength,
		AcceptRanges:  resp.Header.Get("Accept-Ranges") == "bytes",
		LastModified:  parseTime(resp.Header.Get("Last-Modified")),
		ETag:          resp.Header.Get("ETag"),
		ContentType:   resp.Header.Get("Content-Type"),
	}

	// 检查服务器是否支持Range请求
	info.SupportRange = info.AcceptRanges || resp.Header.Get("Content-Range") != ""

	return info, nil
}

// GetRange 获取文件范围数据
func (c *httpClientImpl) GetRange(ctx context.Context, url string, start, end int64) (io.ReadCloser, int64, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, 0, gocd.NewDownloadError(err, url, "failed to create range request")
	}

	// 设置Range头部
	rangeHeader := fmt.Sprintf("bytes=%d-%d", start, end)
	if end == -1 {
		rangeHeader = fmt.Sprintf("bytes=%d-", start)
	}
	req.Header.Set("Range", rangeHeader)

	resp, err := c.Do(req)
	if err != nil {
		return nil, 0, gocd.NewDownloadError(err, url, fmt.Sprintf("range request failed: %s", rangeHeader))
	}

	// 检查响应状态
	if resp.StatusCode == http.StatusOK {
		// 服务器可能不支持Range请求，返回了整个文件
		return resp.Body, resp.ContentLength, nil
	} else if resp.StatusCode == http.StatusPartialContent {
		// Range请求成功
		return resp.Body, resp.ContentLength, nil
	} else if resp.StatusCode == http.StatusRequestedRangeNotSatisfiable {
		// 请求的范围无效
		resp.Body.Close()
		return nil, 0, gocd.NewDownloadError(gocd.ErrServerNotSupportRange, url, "range not satisfiable")
	} else {
		// 其他错误
		resp.Body.Close()
		return nil, 0, gocd.NewHTTPError(resp.StatusCode, url)
	}
}

// GetWithRetry 带重试的GET请求
func (c *httpClientImpl) GetWithRetry(ctx context.Context, url string, retryCount int, retryInterval time.Duration) (io.ReadCloser, int64, error) {
	var lastErr error

	for i := 0; i <= retryCount; i++ {
		if i > 0 {
			// 不是第一次尝试，等待重试间隔
			select {
			case <-ctx.Done():
				return nil, 0, ctx.Err()
			case <-time.After(retryInterval):
			}
		}

		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			lastErr = gocd.NewDownloadError(err, url, "failed to create GET request")
			continue
		}

		resp, err := c.Do(req)
		if err != nil {
			lastErr = gocd.NewDownloadError(err, url, "GET request failed")
			if gocd.IsRetryableError(err) && i < retryCount {
				continue
			}
			return nil, 0, lastErr
		}

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return resp.Body, resp.ContentLength, nil
		}

		resp.Body.Close()
		lastErr = gocd.NewHTTPError(resp.StatusCode, url)

		// 检查是否应该重试
		if resp.StatusCode >= 500 && resp.StatusCode < 600 || resp.StatusCode == 429 {
			// 服务器错误或Too Many Requests，可以重试
			if i < retryCount {
				continue
			}
		}
		// 客户端错误，不重试
		break
	}

	return nil, 0, lastErr
}

// Close 关闭客户端
func (c *httpClientImpl) Close() error {
	c.transport.CloseIdleConnections()
	return nil
}

// parseTime 解析时间字符串
func parseTime(timeStr string) time.Time {
	if timeStr == "" {
		return time.Time{}
	}
	t, err := http.ParseTime(timeStr)
	if err != nil {
		return time.Time{}
	}
	return t
}