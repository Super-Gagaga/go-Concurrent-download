package httpclient

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Super-Gagaga/go-Concurrent-download/pkg/gocd"
)

// TestNewClient 测试创建客户端
func TestNewClient(t *testing.T) {
	config := DefaultClientConfig()
	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	if client == nil {
		t.Error("Client should not be nil")
	}
}

// TestClient_Head 测试HEAD请求
func TestClient_Head(t *testing.T) {
	// 创建测试服务器
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "HEAD" {
			t.Errorf("Expected HEAD request, got %s", r.Method)
		}
		w.Header().Set("Content-Length", "1024")
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := DefaultClientConfig()
	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()
	info, err := client.Head(ctx, server.URL)
	if err != nil {
		t.Fatalf("HEAD request failed: %v", err)
	}

	if info.ContentLength != 1024 {
		t.Errorf("Expected content length 1024, got %d", info.ContentLength)
	}
	if !info.AcceptRanges {
		t.Error("Expected Accept-Ranges to be true")
	}
	if info.ContentType != "application/octet-stream" {
		t.Errorf("Expected content type 'application/octet-stream', got '%s'", info.ContentType)
	}
	if !info.SupportRange {
		t.Error("Expected SupportRange to be true")
	}
}

// TestClient_GetRange 测试范围请求
func TestClient_GetRange(t *testing.T) {
	content := "Hello, World! This is a test file content for range requests."

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("Expected GET request, got %s", r.Method)
		}

		rangeHeader := r.Header.Get("Range")
		if rangeHeader == "" {
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(content)))
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(content))
			return
		}

		// 解析Range头部
		if !strings.HasPrefix(rangeHeader, "bytes=") {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		rangeStr := strings.TrimPrefix(rangeHeader, "bytes=")
		var start, end int
		if n, _ := fmt.Sscanf(rangeStr, "%d-%d", &start, &end); n != 2 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if start < 0 || end >= len(content) || start > end {
			w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
			return
		}

		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(content)))
		w.Header().Set("Content-Length", fmt.Sprintf("%d", end-start+1))
		w.WriteHeader(http.StatusPartialContent)
		w.Write([]byte(content[start:end+1]))
	}))
	defer server.Close()

	config := DefaultClientConfig()
	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	// 测试有效范围
	reader, length, err := client.GetRange(ctx, server.URL, 0, 12)
	if err != nil {
		t.Fatalf("GetRange failed: %v", err)
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("Failed to read response: %v", err)
	}

	expected := "Hello, World!"
	if string(data) != expected {
		t.Errorf("Expected '%s', got '%s'", expected, string(data))
	}
	if length != int64(len(expected)) {
		t.Errorf("Expected length %d, got %d", len(expected), length)
	}
}

// TestClient_GetRange_NotSupported 测试不支持Range的情况
func TestClient_GetRange_NotSupported(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 服务器不支持Range，返回整个文件
		w.Header().Set("Content-Length", "100")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("full content"))
	}))
	defer server.Close()

	config := DefaultClientConfig()
	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()
	reader, length, err := client.GetRange(ctx, server.URL, 0, 50)
	if err != nil {
		t.Fatalf("GetRange failed: %v", err)
	}
	defer reader.Close()

	// 应该返回整个文件
	if length != 100 {
		t.Errorf("Expected length 100, got %d", length)
	}
}

// TestClient_GetWithRetry 测试带重试的GET请求
func TestClient_GetWithRetry(t *testing.T) {
	retryCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		retryCount++
		if retryCount < 3 {
			// 前两次返回服务器错误
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Length", "12")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Hello World!"))
	}))
	defer server.Close()

	config := DefaultClientConfig()
	config.RetryCount = 5
	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	reader, length, err := client.GetWithRetry(ctx, server.URL, config.RetryCount, config.RetryInterval)
	if err != nil {
		t.Fatalf("GetWithRetry failed: %v", err)
	}
	defer reader.Close()

	if retryCount != 3 {
		t.Errorf("Expected 3 retries, got %d", retryCount)
	}
	if length != 12 {
		t.Errorf("Expected length 12, got %d", length)
	}
}

// TestClient_Close 测试关闭客户端
func TestClient_Close(t *testing.T) {
	config := DefaultClientConfig()
	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	if err := client.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}
}

// TestNewDownloadError 测试错误处理
func TestNewDownloadError(t *testing.T) {
	err := gocd.NewDownloadError(gocd.ErrNetworkError, "http://example.com", "test error")
	if err == nil {
		t.Error("Expected error, got nil")
	}

	if !gocd.IsRetryableError(gocd.ErrNetworkError) {
		t.Error("ErrNetworkError should be retryable")
	}

	if gocd.IsRetryableError(gocd.ErrInvalidURL) {
		t.Error("ErrInvalidURL should not be retryable")
	}
}