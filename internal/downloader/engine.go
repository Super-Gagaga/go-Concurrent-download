package downloader

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/Super-Gagaga/go-Concurrent-download/internal/httpclient"
	"github.com/Super-Gagaga/go-Concurrent-download/pkg/gocd"
)

// ConcurrentEngine 并发下载引擎
type ConcurrentEngine struct {
	// 配置
	config gocd.DownloadConfig

	// HTTP客户端
	clientPool httpclient.ClientPool

	// 任务管理
	tasks     map[string]*DownloadTask
	tasksLock sync.RWMutex

	// 工作池
	workerPool chan struct{}

	// 控制通道
	stopChan chan struct{}
	stopped  bool

	// 进度监控
	progressTicker *time.Ticker
	progressStop   chan struct{}
}

// NewConcurrentEngine 创建新的并发下载引擎
func NewConcurrentEngine(config gocd.DownloadConfig) (*ConcurrentEngine, error) {
	// 创建HTTP客户端池
	clientConfig := httpclient.ClientConfig{
		ConnectTimeout: config.ConnectTimeout,
		ReadTimeout:    config.ReadTimeout,
		UserAgent:      config.UserAgent,
		Headers:        config.Headers,
	}

	clientPool := httpclient.NewSimpleClientPool(clientConfig)

	// 创建工作池（限制并发数）
	workerPool := make(chan struct{}, config.Concurrency)
	for i := 0; i < config.Concurrency; i++ {
		workerPool <- struct{}{}
	}

	engine := &ConcurrentEngine{
		config:      config,
		clientPool:  clientPool,
		tasks:       make(map[string]*DownloadTask),
		workerPool:  workerPool,
		stopChan:    make(chan struct{}),
		progressStop: make(chan struct{}),
	}

	// 启动进度监控
	if config.ProgressFunc != nil {
		engine.startProgressMonitor()
	}

	return engine, nil
}

// Download 下载文件
func (e *ConcurrentEngine) Download(ctx context.Context, url, filePath string) (string, error) {
	// 创建下载任务
	task, err := NewDownloadTask(url, filePath, e.config)
	if err != nil {
		return "", err
	}

	// 获取HTTP客户端
	client, err := e.clientPool.GetClient()
	if err != nil {
		return "", gocd.NewDownloadError(err, url, "failed to get HTTP client")
	}
	defer e.clientPool.ReturnClient(client)

	// 初始化任务（获取文件信息）
	if err := task.Init(client); err != nil {
		return "", err
	}

	// 创建临时目录
	if err := os.MkdirAll(task.GetTempDir(), 0755); err != nil {
		return "", gocd.NewDownloadError(err, url, "failed to create temp directory")
	}

	// 保存任务
	e.tasksLock.Lock()
	e.tasks[task.ID] = task
	e.tasksLock.Unlock()

	// 设置任务状态为运行中
	task.SetStatus(TaskStatusRunning)

	// 开始下载
	go e.downloadTask(ctx, task)

	return task.ID, nil
}

// downloadTask 下载任务的主逻辑
func (e *ConcurrentEngine) downloadTask(ctx context.Context, task *DownloadTask) {
	defer func() {
		// 任务结束，清理资源
		e.tasksLock.Lock()
		delete(e.tasks, task.ID)
		e.tasksLock.Unlock()
	}()

	// 检查是否支持Range请求
	if !task.SupportRange {
		// 不支持Range，使用单连接下载
		if err := e.downloadSingle(ctx, task); err != nil {
			task.Error = err
			task.SetStatus(TaskStatusFailed)
			return
		}
		task.SetStatus(TaskStatusCompleted)
		return
	}

	// 支持Range，使用并发分片下载
	if err := e.downloadConcurrent(ctx, task); err != nil {
		task.Error = err
		task.SetStatus(TaskStatusFailed)
		return
	}

	// 合并分片文件
	if err := e.mergeChunks(task); err != nil {
		task.Error = err
		task.SetStatus(TaskStatusFailed)
		return
	}

	// 清理临时文件
	e.cleanupTempFiles(task)

	task.SetStatus(TaskStatusCompleted)
}

// downloadConcurrent 并发分片下载
func (e *ConcurrentEngine) downloadConcurrent(ctx context.Context, task *DownloadTask) error {
	var wg sync.WaitGroup
	errChan := make(chan error, len(task.Chunks))

	// 下载所有分片
	for _, chunk := range task.Chunks {
		// 等待工作池中的空闲worker
		select {
		case <-e.workerPool:
		case <-ctx.Done():
			return ctx.Err()
		case <-e.stopChan:
			return gocd.ErrTaskCancelled
		}

		wg.Add(1)
		go func(chunk *Chunk) {
			defer wg.Done()
			defer func() { e.workerPool <- struct{}{} }()

			// 检查任务状态
			if task.GetStatus() == TaskStatusCancelled || task.GetStatus() == TaskStatusPaused {
				return
			}

			// 下载分片
			if err := e.downloadChunk(ctx, task, chunk); err != nil {
				errChan <- err
			}
		}(chunk)
	}

	// 等待所有分片完成
	wg.Wait()

	// 检查错误
	select {
	case err := <-errChan:
		return err
	default:
		return nil
	}
}

// downloadChunk 下载单个分片
func (e *ConcurrentEngine) downloadChunk(ctx context.Context, task *DownloadTask, chunk *Chunk) error {
	// 如果分片已经完成，跳过
	if chunk.Completed {
		return nil
	}

	// 获取HTTP客户端
	client, err := e.clientPool.GetClient()
	if err != nil {
		return gocd.NewDownloadError(err, task.URL, "failed to get HTTP client")
	}
	defer e.clientPool.ReturnClient(client)

	// 检查控制信号
	if paused, cancelled := task.WaitForSignal(); paused {
		// 等待恢复信号
		for !task.CheckResume() {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-e.stopChan:
				return gocd.ErrTaskCancelled
			case <-time.After(100 * time.Millisecond):
			}
		}
	} else if cancelled {
		return gocd.ErrTaskCancelled
	}

	// 创建临时文件
	tempFilePath := task.GetTempFilePath(chunk.Index)
	file, err := os.Create(tempFilePath)
	if err != nil {
		return gocd.NewDownloadError(err, task.URL, fmt.Sprintf("failed to create temp file for chunk %d", chunk.Index))
	}
	defer file.Close()

	chunk.FilePath = tempFilePath

	// 下载分片数据
	reader, contentLength, err := client.GetRange(ctx, task.URL, chunk.Start, chunk.End)
	if err != nil {
		return gocd.NewDownloadError(err, task.URL, fmt.Sprintf("failed to download chunk %d", chunk.Index))
	}
	defer reader.Close()

	// 写入文件
	buffer := make([]byte, e.config.BufferSize)
	var totalWritten int64

	for {
		// 检查控制信号
		if paused, cancelled := task.WaitForSignal(); paused {
			reader.Close()
			// 等待恢复信号
			for !task.CheckResume() {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-e.stopChan:
					return gocd.ErrTaskCancelled
				case <-time.After(100 * time.Millisecond):
				}
			}
			// 恢复后需要重新获取连接
			return e.downloadChunk(ctx, task, chunk)
		} else if cancelled {
			reader.Close()
			return gocd.ErrTaskCancelled
		}

		n, err := reader.Read(buffer)
		if n > 0 {
			if _, writeErr := file.Write(buffer[:n]); writeErr != nil {
				return gocd.NewDownloadError(writeErr, task.URL, fmt.Sprintf("failed to write chunk %d", chunk.Index))
			}
			totalWritten += int64(n)

			// 更新分片进度
			task.UpdateChunk(chunk.Index, totalWritten, nil)
		}

		if err != nil {
			if err == io.EOF {
				break
			}
			return gocd.NewDownloadError(err, task.URL, fmt.Sprintf("failed to read chunk %d", chunk.Index))
		}
	}

	// 验证下载大小
	if contentLength > 0 && totalWritten != contentLength {
		return gocd.NewDownloadError(gocd.ErrIncompleteDownload, task.URL,
			fmt.Sprintf("chunk %d incomplete: expected %d, got %d", chunk.Index, contentLength, totalWritten))
	}

	// 标记分片完成
	chunk.Completed = true
	task.UpdateChunk(chunk.Index, chunk.Size, nil)

	return nil
}

// downloadSingle 单连接下载（用于不支持Range的服务器）
func (e *ConcurrentEngine) downloadSingle(ctx context.Context, task *DownloadTask) error {
	// 获取HTTP客户端
	client, err := e.clientPool.GetClient()
	if err != nil {
		return gocd.NewDownloadError(err, task.URL, "failed to get HTTP client")
	}
	defer e.clientPool.ReturnClient(client)

	// 创建目标文件
	file, err := os.Create(task.FilePath)
	if err != nil {
		return gocd.NewDownloadError(err, task.URL, "failed to create target file")
	}
	defer file.Close()

	// 下载文件
	reader, contentLength, err := client.GetWithRetry(ctx, task.URL, e.config.RetryCount, e.config.RetryInterval)
	if err != nil {
		return gocd.NewDownloadError(err, task.URL, "failed to download file")
	}
	defer reader.Close()

	// 写入文件
	buffer := make([]byte, e.config.BufferSize)
	var totalWritten int64

	for {
		// 检查控制信号
		if paused, cancelled := task.WaitForSignal(); paused {
			reader.Close()
			// 等待恢复信号
			for !task.CheckResume() {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-e.stopChan:
					return gocd.ErrTaskCancelled
				case <-time.After(100 * time.Millisecond):
				}
			}
			// 恢复后需要重新获取连接
			return e.downloadSingle(ctx, task)
		} else if cancelled {
			reader.Close()
			return gocd.ErrTaskCancelled
		}

		n, err := reader.Read(buffer)
		if n > 0 {
			if _, writeErr := file.Write(buffer[:n]); writeErr != nil {
				return gocd.NewDownloadError(writeErr, task.URL, "failed to write file")
			}
			totalWritten += int64(n)

			// 更新任务进度
			task.UpdateChunk(0, totalWritten, nil)
		}

		if err != nil {
			if err == io.EOF {
				break
			}
			return gocd.NewDownloadError(err, task.URL, "failed to read file")
		}
	}

	// 验证下载大小
	if contentLength > 0 && totalWritten != contentLength {
		return gocd.NewDownloadError(gocd.ErrIncompleteDownload, task.URL,
			fmt.Sprintf("file incomplete: expected %d, got %d", contentLength, totalWritten))
	}

	return nil
}

// mergeChunks 合并分片文件
func (e *ConcurrentEngine) mergeChunks(task *DownloadTask) error {
	// 创建目标文件
	file, err := os.Create(task.FilePath)
	if err != nil {
		return gocd.NewDownloadError(err, task.URL, "failed to create target file")
	}
	defer file.Close()

	// 按顺序合并所有分片
	for i, chunk := range task.Chunks {
		if !chunk.Completed {
			return gocd.NewDownloadError(gocd.ErrIncompleteDownload, task.URL,
				fmt.Sprintf("chunk %d not completed", i))
		}

		chunkFile, err := os.Open(chunk.FilePath)
		if err != nil {
			return gocd.NewDownloadError(err, task.URL, fmt.Sprintf("failed to open chunk %d", i))
		}

		if _, err := io.Copy(file, chunkFile); err != nil {
			chunkFile.Close()
			return gocd.NewDownloadError(err, task.URL, fmt.Sprintf("failed to merge chunk %d", i))
		}

		chunkFile.Close()
	}

	return nil
}

// cleanupTempFiles 清理临时文件
func (e *ConcurrentEngine) cleanupTempFiles(task *DownloadTask) {
	for _, chunk := range task.Chunks {
		if chunk.FilePath != "" {
			os.Remove(chunk.FilePath)
		}
	}

	// 删除临时目录（如果为空）
	if tempDir := task.GetTempDir(); tempDir != "" {
		os.Remove(tempDir)
	}
}

// startProgressMonitor 启动进度监控
func (e *ConcurrentEngine) startProgressMonitor() {
	e.progressTicker = time.NewTicker(500 * time.Millisecond)

	go func() {
		for {
			select {
			case <-e.progressTicker.C:
				e.reportProgress()
			case <-e.progressStop:
				return
			case <-e.stopChan:
				return
			}
		}
	}()
}

// reportProgress 报告进度
func (e *ConcurrentEngine) reportProgress() {
	e.tasksLock.RLock()
	defer e.tasksLock.RUnlock()

	for _, task := range e.tasks {
		if e.config.ProgressFunc != nil {
			progress := task.GetProgress()
			e.config.ProgressFunc(progress)
		}
	}
}

// Stop 停止引擎
func (e *ConcurrentEngine) Stop() {
	if e.stopped {
		return
	}

	e.stopped = true
	close(e.stopChan)

	// 停止进度监控
	if e.progressTicker != nil {
		e.progressTicker.Stop()
		close(e.progressStop)
	}

	// 关闭客户端池
	if e.clientPool != nil {
		e.clientPool.Close()
	}

	// 取消所有任务
	e.tasksLock.Lock()
	for _, task := range e.tasks {
		task.Cancel()
	}
	e.tasks = make(map[string]*DownloadTask)
	e.tasksLock.Unlock()
}

// GetTask 获取任务
func (e *ConcurrentEngine) GetTask(taskID string) (*DownloadTask, error) {
	e.tasksLock.RLock()
	defer e.tasksLock.RUnlock()

	task, exists := e.tasks[taskID]
	if !exists {
		return nil, gocd.ErrTaskNotFound
	}
	return task, nil
}

// GetTaskStatus 获取任务状态
func (e *ConcurrentEngine) GetTaskStatus(taskID string) (gocd.DownloadStatus, error) {
	task, err := e.GetTask(taskID)
	if err != nil {
		return gocd.DownloadStatus{}, err
	}

	progress := task.GetProgress()

	return gocd.DownloadStatus{
		TaskID:      task.ID,
		URL:         task.URL,
		FilePath:    task.FilePath,
		TotalBytes:  progress.TotalBytes,
		Downloaded:  progress.Downloaded,
		Percentage:  progress.Percentage,
		Speed:       progress.Speed,
		Remaining:   progress.RemainingTime,
		IsPaused:    task.GetStatus() == TaskStatusPaused,
		IsCompleted: task.GetStatus() == TaskStatusCompleted,
		IsFailed:    task.GetStatus() == TaskStatusFailed,
		Error:       "",
		StartTime:   task.StartTime,
	}, nil
}

// PauseTask 暂停任务
func (e *ConcurrentEngine) PauseTask(taskID string) error {
	task, err := e.GetTask(taskID)
	if err != nil {
		return err
	}

	if task.GetStatus() != TaskStatusRunning {
		return gocd.ErrTaskNotRunning
	}

	task.Pause()
	return nil
}

// ResumeTask 恢复任务
func (e *ConcurrentEngine) ResumeTask(taskID string) error {
	task, err := e.GetTask(taskID)
	if err != nil {
		return err
	}

	if task.GetStatus() != TaskStatusPaused {
		return gocd.ErrTaskPaused
	}

	task.Resume()
	return nil
}

// CancelTask 取消任务
func (e *ConcurrentEngine) CancelTask(taskID string) error {
	task, err := e.GetTask(taskID)
	if err != nil {
		return err
	}

	task.Cancel()
	return nil
}

// ListTasks 列出所有任务
func (e *ConcurrentEngine) ListTasks() []gocd.DownloadStatus {
	e.tasksLock.RLock()
	defer e.tasksLock.RUnlock()

	var statuses []gocd.DownloadStatus
	for _, task := range e.tasks {
		status, err := e.GetTaskStatus(task.ID)
		if err == nil {
			statuses = append(statuses, status)
		}
	}
	return statuses
}