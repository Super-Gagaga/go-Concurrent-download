package httpclient

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/yourusername/go-concurrent-download/pkg/gocd"
)

// ClientPool 客户端连接池
type ClientPool interface {
	// GetClient 获取一个客户端
	GetClient() (HTTPClient, error)

	// ReturnClient 返回客户端到连接池
	ReturnClient(client HTTPClient)

	// Close 关闭连接池
	Close() error

	// Stats 获取连接池统计信息
	Stats() PoolStats
}

// PoolConfig 连接池配置
type PoolConfig struct {
	// 连接池大小
	MaxSize int // 最大客户端数，0表示无限制

	// 客户端配置
	ClientConfig ClientConfig

	// 超时设置
	AcquireTimeout time.Duration // 获取客户端超时时间
}

// DefaultPoolConfig 返回默认连接池配置
func DefaultPoolConfig() PoolConfig {
	return PoolConfig{
		MaxSize:        10,
		ClientConfig:   DefaultClientConfig(),
		AcquireTimeout: 30 * time.Second,
	}
}

// PoolStats 连接池统计信息
type PoolStats struct {
	TotalClients   int // 总客户端数
	ActiveClients  int // 活跃客户端数
	IdleClients    int // 空闲客户端数
	MaxSize        int // 最大客户端数
	AcquireCount   int // 获取次数
	ReturnCount    int // 返回次数
	CreateCount    int // 创建次数
	DestroyCount   int // 销毁次数
	AcquireTimeout int // 获取超时次数
}

// clientPoolImpl 客户端连接池实现
type clientPoolImpl struct {
	config     PoolConfig
	clients    chan HTTPClient
	created    map[HTTPClient]bool
	stats      PoolStats
	statsMutex sync.RWMutex
	mutex      sync.Mutex
	closed     bool
}

// NewClientPool 创建新的客户端连接池
func NewClientPool(config PoolConfig) (ClientPool, error) {
	if config.MaxSize <= 0 {
		return nil, gocd.NewDownloadError(gocd.ErrInvalidConfig, "", "pool max size must be greater than 0")
	}

	pool := &clientPoolImpl{
		config:  config,
		clients: make(chan HTTPClient, config.MaxSize),
		created: make(map[HTTPClient]bool),
		stats: PoolStats{
			MaxSize: config.MaxSize,
		},
	}

	// 预先创建一些客户端
	for i := 0; i < min(2, config.MaxSize); i++ {
		client, err := NewClient(config.ClientConfig)
		if err != nil {
			pool.Close()
			return nil, fmt.Errorf("failed to create initial client: %w", err)
		}
		pool.clients <- client
		pool.created[client] = true
		pool.stats.TotalClients++
		pool.stats.IdleClients++
		pool.stats.CreateCount++
	}

	return pool, nil
}

// GetClient 获取一个客户端
func (p *clientPoolImpl) GetClient() (HTTPClient, error) {
	ctx, cancel := context.WithTimeout(context.Background(), p.config.AcquireTimeout)
	defer cancel()

	select {
	case client := <-p.clients:
		p.statsMutex.Lock()
		p.stats.IdleClients--
		p.stats.ActiveClients++
		p.stats.AcquireCount++
		p.statsMutex.Unlock()
		return client, nil

	case <-ctx.Done():
		p.statsMutex.Lock()
		p.stats.AcquireTimeout++
		p.statsMutex.Unlock()

		// 超时，尝试创建新客户端（如果未达到最大限制）
		p.mutex.Lock()
		defer p.mutex.Unlock()

		if p.closed {
			return nil, gocd.NewDownloadError(gocd.ErrNetworkError, "", "pool is closed")
		}

		if p.stats.TotalClients < p.config.MaxSize || p.config.MaxSize == 0 {
			client, err := NewClient(p.config.ClientConfig)
			if err != nil {
				return nil, fmt.Errorf("failed to create new client: %w", err)
			}

			p.created[client] = true
			p.statsMutex.Lock()
			p.stats.TotalClients++
			p.stats.ActiveClients++
			p.stats.CreateCount++
			p.stats.AcquireCount++
			p.statsMutex.Unlock()

			return client, nil
		}

		return nil, gocd.NewDownloadError(gocd.ErrTimeout, "", "timeout waiting for available client")
	}
}

// ReturnClient 返回客户端到连接池
func (p *clientPoolImpl) ReturnClient(client HTTPClient) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if p.closed {
		// 连接池已关闭，直接销毁客户端
		client.Close()
		p.statsMutex.Lock()
		p.stats.DestroyCount++
		p.statsMutex.Unlock()
		return
	}

	// 检查客户端是否由本连接池创建
	if !p.created[client] {
		// 不是本连接池创建的客户端，直接关闭
		client.Close()
		return
	}

	select {
	case p.clients <- client:
		p.statsMutex.Lock()
		p.stats.IdleClients++
		p.stats.ActiveClients--
		p.stats.ReturnCount++
		p.statsMutex.Unlock()
	default:
		// 连接池已满，销毁客户端
		client.Close()
		p.statsMutex.Lock()
		delete(p.created, client)
		p.stats.TotalClients--
		p.stats.ActiveClients--
		p.stats.DestroyCount++
		p.statsMutex.Unlock()
	}
}

// Close 关闭连接池
func (p *clientPoolImpl) Close() error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if p.closed {
		return nil
	}

	p.closed = true
	close(p.clients)

	// 关闭所有客户端
	for client := range p.clients {
		client.Close()
		p.statsMutex.Lock()
		delete(p.created, client)
		p.stats.DestroyCount++
		p.statsMutex.Unlock()
	}

	// 关闭已创建但不在通道中的客户端
	for client := range p.created {
		client.Close()
		p.statsMutex.Lock()
		delete(p.created, client)
		p.stats.DestroyCount++
		p.statsMutex.Unlock()
	}

	p.created = make(map[HTTPClient]bool)
	return nil
}

// Stats 获取连接池统计信息
func (p *clientPoolImpl) Stats() PoolStats {
	p.statsMutex.RLock()
	defer p.statsMutex.RUnlock()

	stats := p.stats
	// 更新空闲客户端数（从通道长度计算）
	stats.IdleClients = len(p.clients)
	// 重新计算活跃客户端数
	stats.ActiveClients = stats.TotalClients - stats.IdleClients
	return stats
}

// min 返回两个整数中的较小值
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// SimpleClientPool 简单客户端池（无限制）
type simpleClientPool struct {
	config ClientConfig
}

// NewSimpleClientPool 创建简单客户端池（无限制大小）
func NewSimpleClientPool(config ClientConfig) ClientPool {
	return &simpleClientPool{
		config: config,
	}
}

// GetClient 获取客户端（每次都创建新的）
func (p *simpleClientPool) GetClient() (HTTPClient, error) {
	return NewClient(p.config)
}

// ReturnClient 返回客户端（直接关闭）
func (p *simpleClientPool) ReturnClient(client HTTPClient) {
	client.Close()
}

// Close 关闭连接池
func (p *simpleClientPool) Close() error {
	return nil
}

// Stats 获取统计信息
func (p *simpleClientPool) Stats() PoolStats {
	return PoolStats{
		MaxSize: 0, // 0表示无限制
	}
}

// RangeDownloader 范围下载器，专门用于分片下载
type RangeDownloader struct {
	pool    ClientPool
	config  ClientConfig
}

// NewRangeDownloader 创建新的范围下载器
func NewRangeDownloader(config ClientConfig) *RangeDownloader {
	return &RangeDownloader{
		pool:   NewSimpleClientPool(config),
		config: config,
	}
}

// DownloadRange 下载指定范围的数据
func (d *RangeDownloader) DownloadRange(ctx context.Context, url string, start, end int64) ([]byte, error) {
	client, err := d.pool.GetClient()
	if err != nil {
		return nil, err
	}
	defer d.pool.ReturnClient(client)

	reader, contentLength, err := client.GetRange(ctx, url, start, end)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	if contentLength <= 0 {
		contentLength = end - start + 1
		if end == -1 {
			// 不知道具体长度，使用合理的大小
			contentLength = 1024 * 1024 // 1MB
		}
	}

	data := make([]byte, contentLength)
	n, err := io.ReadFull(reader, data)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return nil, gocd.NewDownloadError(err, url, fmt.Sprintf("failed to read range %d-%d", start, end))
	}

	return data[:n], nil
}

// Close 关闭下载器
func (d *RangeDownloader) Close() error {
	return d.pool.Close()
}