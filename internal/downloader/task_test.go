package downloader

import (
	"strings"
	"testing"
	"time"

	"github.com/Super-Gagaga/go-Concurrent-download/pkg/gocd"
)

// TestNewDownloadTask 测试创建下载任务
func TestNewDownloadTask(t *testing.T) {
	config := gocd.DefaultConfig()
	task, err := NewDownloadTask("http://example.com/file.zip", "/tmp/file.zip", config)
	if err != nil {
		t.Fatalf("Failed to create task: %v", err)
	}

	if task.ID == "" {
		t.Error("Task ID should not be empty")
	}
	if task.URL != "http://example.com/file.zip" {
		t.Errorf("Expected URL 'http://example.com/file.zip', got '%s'", task.URL)
	}
	if task.FilePath == "" {
		t.Error("File path should not be empty")
	}
	// 在Windows上路径分隔符可能被转换，只检查是否包含文件名
	if !strings.Contains(task.FilePath, "file.zip") {
		t.Errorf("File path should contain 'file.zip', got '%s'", task.FilePath)
	}
	if task.GetStatus() != TaskStatusPending {
		t.Errorf("Expected status Pending, got %v", task.GetStatus())
	}
}

// TestTaskStatusString 测试任务状态字符串表示
func TestTaskStatusString(t *testing.T) {
	testCases := []struct {
		status TaskStatus
		expect string
	}{
		{TaskStatusPending, "pending"},
		{TaskStatusRunning, "running"},
		{TaskStatusPaused, "paused"},
		{TaskStatusCompleted, "completed"},
		{TaskStatusFailed, "failed"},
		{TaskStatusCancelled, "cancelled"},
		{TaskStatus(100), "unknown"},
	}

	for _, tc := range testCases {
		result := tc.status.String()
		if result != tc.expect {
			t.Errorf("Status %v: expected '%s', got '%s'", tc.status, tc.expect, result)
		}
	}
}

// TestTaskControl 测试任务控制（暂停、恢复、取消）
func TestTaskControl(t *testing.T) {
	config := gocd.DefaultConfig()
	task, err := NewDownloadTask("http://example.com/file.zip", "/tmp/file.zip", config)
	if err != nil {
		t.Fatalf("Failed to create task: %v", err)
	}

	// 测试暂停和恢复
	task.SetStatus(TaskStatusRunning)
	task.Pause()
	if task.GetStatus() != TaskStatusPaused {
		t.Error("Task should be paused")
	}

	task.Resume()
	if task.GetStatus() != TaskStatusRunning {
		t.Error("Task should be running after resume")
	}

	// 测试取消
	task.Cancel()
	if task.GetStatus() != TaskStatusCancelled {
		t.Error("Task should be cancelled")
	}
}

// TestGetProgress 测试获取进度
func TestGetProgress(t *testing.T) {
	config := gocd.DefaultConfig()
	task, err := NewDownloadTask("http://example.com/file.zip", "/tmp/file.zip", config)
	if err != nil {
		t.Fatalf("Failed to create task: %v", err)
	}

	// 模拟一些数据
	task.TotalSize = 1000
	task.StartTime = time.Now().Add(-10 * time.Second)

	// 创建分片并设置下载进度
	task.Chunks = []*Chunk{
		{Index: 0, Size: 500, Downloaded: 300, Completed: false},
		{Index: 1, Size: 500, Downloaded: 200, Completed: false},
	}
	task.ChunkCount = 2

	progress := task.GetProgress()

	if progress.TotalBytes != 1000 {
		t.Errorf("Expected total bytes 1000, got %d", progress.TotalBytes)
	}
	// Downloaded应该是所有分片下载量的总和：300 + 200 = 500
	if progress.Downloaded != 500 {
		t.Errorf("Expected downloaded 500, got %d", progress.Downloaded)
	}
	if progress.Percentage != 50.0 {
		t.Errorf("Expected percentage 50.0, got %.2f", progress.Percentage)
	}
	// 有StartTime且Downloaded>0，速度应该大于0
	if progress.Speed <= 0 {
		t.Error("Speed should be greater than 0")
	}

	// 测试完成的情况
	task.Chunks[0].Downloaded = 500
	task.Chunks[0].Completed = true
	task.Chunks[1].Downloaded = 500
	task.Chunks[1].Completed = true
	task.SetStatus(TaskStatusCompleted) // 设置任务状态为已完成

	progress = task.GetProgress()
	if progress.Percentage != 100.0 {
		t.Errorf("Expected percentage 100.0 for completed task, got %.2f", progress.Percentage)
	}
	if !progress.IsCompleted {
		t.Error("Progress should show as completed")
	}
}

// TestCalculateOptimalChunkSize 测试计算最优分片大小
func TestCalculateOptimalChunkSize(t *testing.T) {
	testCases := []struct {
		totalSize    int64
		concurrency  int
		minChunkSize int64
		maxChunkSize int64
		expect       int64
	}{
		{1000, 4, 100, 1000, 250},   // 正常情况
		{1000, 10, 100, 1000, 100},  // 并发数多，使用最小分片
		{10000, 2, 100, 1000, 1000}, // 文件大，使用最大分片
		{0, 4, 100, 1000, 100},      // 文件大小为0
	}

	for i, tc := range testCases {
		result := calculateOptimalChunkSize(tc.totalSize, tc.concurrency, tc.minChunkSize, tc.maxChunkSize)
		if result != tc.expect {
			t.Errorf("Test case %d: expected %d, got %d", i, tc.expect, result)
		}
	}
}

// TestTaskGetChunk 测试获取分片
func TestTaskGetChunk(t *testing.T) {
	config := gocd.DefaultConfig()
	task, err := NewDownloadTask("http://example.com/file.zip", "/tmp/file.zip", config)
	if err != nil {
		t.Fatalf("Failed to create task: %v", err)
	}

	// 初始化分片
	task.TotalSize = 1000
	task.ChunkSize = 100
	task.ChunkCount = 10
	task.Chunks = make([]*Chunk, 10)
	for i := 0; i < 10; i++ {
		task.Chunks[i] = &Chunk{
			Index: i,
			Start: int64(i * 100),
			End:   int64((i+1)*100 - 1),
			Size:  100,
		}
	}

	// 测试获取有效分片
	chunk, err := task.GetChunk(5)
	if err != nil {
		t.Fatalf("Failed to get chunk: %v", err)
	}
	if chunk.Index != 5 {
		t.Errorf("Expected chunk index 5, got %d", chunk.Index)
	}

	// 测试获取无效分片
	_, err = task.GetChunk(15)
	if err == nil {
		t.Error("Expected error for invalid chunk index")
	}
}

// TestUpdateChunk 测试更新分片状态
func TestUpdateChunk(t *testing.T) {
	config := gocd.DefaultConfig()
	task, err := NewDownloadTask("http://example.com/file.zip", "/tmp/file.zip", config)
	if err != nil {
		t.Fatalf("Failed to create task: %v", err)
	}

	// 初始化分片
	task.Chunks = []*Chunk{
		{Index: 0, Size: 100},
		{Index: 1, Size: 100},
	}

	// 更新分片
	task.UpdateChunk(0, 50, nil)
	task.UpdateChunk(1, 100, nil)

	if task.Chunks[0].Downloaded != 50 {
		t.Errorf("Expected chunk 0 downloaded 50, got %d", task.Chunks[0].Downloaded)
	}
	if !task.Chunks[1].Completed {
		t.Error("Chunk 1 should be completed")
	}
}