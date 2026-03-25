package downloader

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/Super-Gagaga/go-Concurrent-download/internal/httpclient"
	"github.com/Super-Gagaga/go-Concurrent-download/pkg/gocd"
)

// TaskStatus 任务状态
type TaskStatus int

const (
	TaskStatusPending   TaskStatus = iota // 等待中
	TaskStatusRunning                     // 运行中
	TaskStatusPaused                      // 已暂停
	TaskStatusCompleted                   // 已完成
	TaskStatusFailed                      // 已失败
	TaskStatusCancelled                   // 已取消
)

// String 返回任务状态的字符串表示
func (s TaskStatus) String() string {
	switch s {
	case TaskStatusPending:
		return "pending"
	case TaskStatusRunning:
		return "running"
	case TaskStatusPaused:
		return "paused"
	case TaskStatusCompleted:
		return "completed"
	case TaskStatusFailed:
		return "failed"
	case TaskStatusCancelled:
		return "cancelled"
	default:
		return "unknown"
	}
}

// Chunk 下载分片
type Chunk struct {
	Index      int    // 分片索引
	Start      int64  // 起始位置
	End        int64  // 结束位置（包含）
	Size       int64  // 分片大小
	Downloaded int64  // 已下载字节数
	Completed  bool   // 是否完成
	Error      error  // 错误信息
	FilePath   string // 临时文件路径
}

// DownloadTask 下载任务
type DownloadTask struct {
	// 基本信息
	ID       string // 任务ID
	URL      string // 下载URL
	FilePath string // 目标文件路径

	// 配置信息
	Config gocd.DownloadConfig

	// 文件信息
	FileInfo *httpclient.FileInfo // 文件元信息
	TotalSize int64               // 总大小

	// 分片信息
	Chunks      []*Chunk // 所有分片
	ChunkSize   int64    // 分片大小
	ChunkCount  int      // 分片数量
	SupportRange bool    // 是否支持Range请求

	// 状态信息
	Status        TaskStatus // 任务状态
	StartTime     time.Time  // 开始时间
	EndTime       time.Time  // 结束时间
	Downloaded    int64      // 总已下载字节数
	LastUpdateTime time.Time // 最后更新时间

	// 控制通道
	pauseChan  chan struct{} // 暂停通道
	resumeChan chan struct{} // 恢复通道
	cancelChan chan struct{} // 取消通道

	// 错误信息
	Error error // 任务错误

	// 临时文件目录
	tempDir string

	// 互斥锁
	mutex sync.RWMutex
}

// NewDownloadTask 创建新的下载任务
func NewDownloadTask(url, filePath string, config gocd.DownloadConfig) (*DownloadTask, error) {
	if url == "" {
		return nil, gocd.ErrInvalidURL
	}
	if filePath == "" {
		return nil, gocd.ErrInvalidPath
	}

	// 生成任务ID
	taskID := generateTaskID(url)

	task := &DownloadTask{
		ID:         taskID,
		URL:        url,
		FilePath:   filepath.Clean(filePath),
		Config:     config,
		Status:     TaskStatusPending,
		pauseChan:  make(chan struct{}, 1),
		resumeChan: make(chan struct{}, 1),
		cancelChan: make(chan struct{}, 1),
		tempDir:    config.TempDir,
	}

	if task.tempDir == "" {
		task.tempDir = filepath.Join(filepath.Dir(filePath), ".download_tmp")
	}

	return task, nil
}

// Init 初始化任务，获取文件信息
func (t *DownloadTask) Init(client httpclient.HTTPClient) error {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	if t.Status != TaskStatusPending {
		return fmt.Errorf("task already initialized")
	}

	// 获取文件信息
	ctx, cancel := context.WithTimeout(context.Background(), t.Config.ConnectTimeout)
	defer cancel()

	fileInfo, err := client.Head(ctx, t.URL)
	if err != nil {
		return gocd.NewDownloadError(err, t.URL, "failed to get file info")
	}

	t.FileInfo = fileInfo
	t.TotalSize = fileInfo.ContentLength
	t.SupportRange = fileInfo.SupportRange

	// 计算分片策略
	if err := t.calculateChunks(); err != nil {
		return err
	}

	t.Status = TaskStatusPending
	return nil
}

// calculateChunks 计算分片策略
func (t *DownloadTask) calculateChunks() error {
	if t.TotalSize <= 0 {
		// 不知道文件大小，使用单个分片
		t.ChunkCount = 1
		t.ChunkSize = t.TotalSize
		t.Chunks = []*Chunk{
			{
				Index: 0,
				Start: 0,
				End:   -1, // 表示到文件末尾
				Size:  t.TotalSize,
			},
		}
		return nil
	}

	// 计算分片大小
	t.ChunkSize = calculateOptimalChunkSize(t.TotalSize, t.Config.Concurrency, t.Config.MinChunkSize, t.Config.MaxChunkSize)

	// 计算分片数量
	t.ChunkCount = int((t.TotalSize + t.ChunkSize - 1) / t.ChunkSize) // 向上取整

	// 创建分片
	t.Chunks = make([]*Chunk, t.ChunkCount)
	for i := 0; i < t.ChunkCount; i++ {
		start := int64(i) * t.ChunkSize
		end := start + t.ChunkSize - 1
		if i == t.ChunkCount-1 {
			// 最后一个分片
			end = t.TotalSize - 1
		}

		t.Chunks[i] = &Chunk{
			Index: i,
			Start: start,
			End:   end,
			Size:  end - start + 1,
		}
	}

	return nil
}

// GetProgress 获取任务进度
func (t *DownloadTask) GetProgress() gocd.ProgressStatus {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	var downloaded int64
	var completedChunks int

	for _, chunk := range t.Chunks {
		downloaded += chunk.Downloaded
		if chunk.Completed {
			completedChunks++
		}
	}

	t.Downloaded = downloaded

	var percentage float64
	if t.TotalSize > 0 {
		percentage = float64(downloaded) / float64(t.TotalSize) * 100
	} else if completedChunks == t.ChunkCount {
		percentage = 100
	}

	var speed int64
	var remaining time.Duration

	if !t.StartTime.IsZero() {
		elapsed := time.Since(t.StartTime)
		if elapsed > 0 {
			speed = int64(float64(downloaded) / elapsed.Seconds())
		}

		if speed > 0 && t.TotalSize > 0 {
			remainingBytes := t.TotalSize - downloaded
			remaining = time.Duration(float64(remainingBytes)/float64(speed)) * time.Second
		}
	}

	return gocd.ProgressStatus{
		TaskID:         t.ID,
		URL:           t.URL,
		FileName:      filepath.Base(t.FilePath),
		TotalBytes:    t.TotalSize,
		Downloaded:    downloaded,
		Percentage:    percentage,
		Speed:         speed,
		RemainingTime: remaining,
		IsCompleted:   t.Status == TaskStatusCompleted,
		Error:         t.Error,
		StartTime:     t.StartTime,
		LastUpdateTime: time.Now(),
	}
}

// GetChunk 获取指定分片
func (t *DownloadTask) GetChunk(index int) (*Chunk, error) {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	if index < 0 || index >= len(t.Chunks) {
		return nil, fmt.Errorf("chunk index out of range")
	}
	return t.Chunks[index], nil
}

// UpdateChunk 更新分片状态
func (t *DownloadTask) UpdateChunk(index int, downloaded int64, err error) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	if index < 0 || index >= len(t.Chunks) {
		return
	}

	chunk := t.Chunks[index]
	chunk.Downloaded = downloaded

	if err != nil {
		chunk.Error = err
	} else if downloaded >= chunk.Size {
		chunk.Completed = true
		chunk.Downloaded = chunk.Size
	}

	t.LastUpdateTime = time.Now()
}

// Pause 暂停任务
func (t *DownloadTask) Pause() {
	t.mutex.Lock()
	if t.Status == TaskStatusRunning {
		t.Status = TaskStatusPaused
		select {
		case t.pauseChan <- struct{}{}:
		default:
		}
	}
	t.mutex.Unlock()
}

// Resume 恢复任务
func (t *DownloadTask) Resume() {
	t.mutex.Lock()
	if t.Status == TaskStatusPaused {
		t.Status = TaskStatusRunning
		select {
		case t.resumeChan <- struct{}{}:
		default:
		}
	}
	t.mutex.Unlock()
}

// Cancel 取消任务
func (t *DownloadTask) Cancel() {
	t.mutex.Lock()
	if t.Status == TaskStatusRunning || t.Status == TaskStatusPaused {
		t.Status = TaskStatusCancelled
		select {
		case t.cancelChan <- struct{}{}:
		default:
		}
	}
	t.mutex.Unlock()
}

// WaitForSignal 等待控制信号
func (t *DownloadTask) WaitForSignal() (paused, cancelled bool) {
	select {
	case <-t.pauseChan:
		return true, false
	case <-t.cancelChan:
		return false, true
	default:
		return false, false
	}
}

// CheckResume 检查是否有恢复信号
func (t *DownloadTask) CheckResume() bool {
	select {
	case <-t.resumeChan:
		return true
	default:
		return false
	}
}

// SetStatus 设置任务状态
func (t *DownloadTask) SetStatus(status TaskStatus) {
	t.mutex.Lock()
	t.Status = status
	if status == TaskStatusRunning && t.StartTime.IsZero() {
		t.StartTime = time.Now()
	}
	if status == TaskStatusCompleted || status == TaskStatusFailed || status == TaskStatusCancelled {
		t.EndTime = time.Now()
	}
	t.mutex.Unlock()
}

// GetStatus 获取任务状态
func (t *DownloadTask) GetStatus() TaskStatus {
	t.mutex.RLock()
	defer t.mutex.RUnlock()
	return t.Status
}

// GetTempFilePath 获取分片临时文件路径
func (t *DownloadTask) GetTempFilePath(chunkIndex int) string {
	return filepath.Join(t.tempDir, fmt.Sprintf("%s_chunk_%d.tmp", t.ID, chunkIndex))
}

// GetTempDir 获取临时目录
func (t *DownloadTask) GetTempDir() string {
	return t.tempDir
}

// generateTaskID 生成任务ID
func generateTaskID(url string) string {
	// 简化实现，实际应该使用更复杂的方法生成唯一ID
	return fmt.Sprintf("task_%x", time.Now().UnixNano())
}

// calculateOptimalChunkSize 计算最优分片大小
func calculateOptimalChunkSize(totalSize int64, concurrency int, minChunkSize, maxChunkSize int64) int64 {
	if totalSize <= 0 {
		return minChunkSize
	}

	// 根据总大小和并发数计算分片大小
	chunkSize := totalSize / int64(concurrency)

	// 确保在最小和最大分片大小之间
	if chunkSize < minChunkSize {
		chunkSize = minChunkSize
	}
	if chunkSize > maxChunkSize {
		chunkSize = maxChunkSize
	}

	// 确保分片大小是合理的
	if chunkSize > totalSize {
		chunkSize = totalSize
	}

	return chunkSize
}

