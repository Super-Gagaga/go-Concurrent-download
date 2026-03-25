package downloader

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/yourusername/go-concurrent-download/pkg/gocd"
)

// StateFileVersion 状态文件版本
const StateFileVersion = 1

// DownloadState 下载状态（用于持久化）
type DownloadState struct {
	Version     int                    `json:"version"`      // 状态文件版本
	TaskID      string                 `json:"task_id"`      // 任务ID
	URL         string                 `json:"url"`          // 下载URL
	FilePath    string                 `json:"file_path"`    // 目标文件路径
	TotalSize   int64                  `json:"total_size"`   // 总文件大小
	Downloaded  int64                  `json:"downloaded"`   // 已下载字节数
	SupportRange bool                  `json:"support_range"` // 是否支持Range请求
	ChunkSize   int64                  `json:"chunk_size"`   // 分片大小
	Chunks      []ChunkState           `json:"chunks"`       // 分片状态
	Config      gocd.DownloadConfig    `json:"config"`       // 下载配置
	StartTime   time.Time              `json:"start_time"`   // 开始时间
	LastUpdate  time.Time              `json:"last_update"`  // 最后更新时间
	Status      string                 `json:"status"`       // 任务状态
	Error       string                 `json:"error,omitempty"` // 错误信息
}

// ChunkState 分片状态（用于持久化）
type ChunkState struct {
	Index      int    `json:"index"`       // 分片索引
	Start      int64  `json:"start"`       // 起始位置
	End        int64  `json:"end"`         // 结束位置
	Size       int64  `json:"size"`        // 分片大小
	Downloaded int64  `json:"downloaded"`  // 已下载字节数
	Completed  bool   `json:"completed"`   // 是否完成
	Error      string `json:"error,omitempty"` // 错误信息
	FilePath   string `json:"file_path"`   // 临时文件路径
}

// StateManager 状态管理器
type StateManager struct {
	stateDir string // 状态文件目录
}

// NewStateManager 创建新的状态管理器
func NewStateManager(stateDir string) (*StateManager, error) {
	if stateDir == "" {
		// 使用默认目录
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, gocd.NewDownloadError(err, "", "failed to get user home directory")
		}
		stateDir = filepath.Join(homeDir, ".gocd", "states")
	}

	// 创建目录
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		return nil, gocd.NewDownloadError(err, "", fmt.Sprintf("failed to create state directory: %s", stateDir))
	}

	return &StateManager{
		stateDir: stateDir,
	}, nil
}

// Save 保存下载状态
func (sm *StateManager) Save(task *DownloadTask) error {
	if task == nil {
		return gocd.ErrInvalidConfig
	}

	// 转换为持久化状态
	state := sm.taskToState(task)

	// 获取状态文件路径
	stateFilePath := sm.getStateFilePath(task.ID)

	// 创建临时文件
	tempFilePath := stateFilePath + ".tmp"
	file, err := os.Create(tempFilePath)
	if err != nil {
		return gocd.NewDownloadError(err, task.URL, "failed to create state file")
	}
	defer file.Close()

	// 编码为JSON
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(state); err != nil {
		return gocd.NewDownloadError(err, task.URL, "failed to encode state")
	}

	// 原子性重命名
	if err := os.Rename(tempFilePath, stateFilePath); err != nil {
		return gocd.NewDownloadError(err, task.URL, "failed to save state file")
	}

	return nil
}

// Load 加载下载状态
func (sm *StateManager) Load(taskID string) (*DownloadTask, error) {
	// 获取状态文件路径
	stateFilePath := sm.getStateFilePath(taskID)

	// 检查文件是否存在
	if _, err := os.Stat(stateFilePath); os.IsNotExist(err) {
		return nil, gocd.ErrTaskNotFound
	}

	// 读取文件
	file, err := os.Open(stateFilePath)
	if err != nil {
		return nil, gocd.NewDownloadError(err, "", fmt.Sprintf("failed to open state file: %s", stateFilePath))
	}
	defer file.Close()

	// 解码JSON
	var state DownloadState
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&state); err != nil {
		return nil, gocd.NewDownloadError(err, "", "failed to decode state")
	}

	// 检查版本
	if state.Version != StateFileVersion {
		return nil, gocd.NewDownloadError(gocd.ErrResumeFailed, "", fmt.Sprintf("unsupported state version: %d", state.Version))
	}

	// 转换为任务
	task, err := sm.stateToTask(&state)
	if err != nil {
		return nil, err
	}

	return task, nil
}

// Delete 删除状态文件
func (sm *StateManager) Delete(taskID string) error {
	stateFilePath := sm.getStateFilePath(taskID)
	if err := os.Remove(stateFilePath); err != nil && !os.IsNotExist(err) {
		return gocd.NewDownloadError(err, "", fmt.Sprintf("failed to delete state file: %s", stateFilePath))
	}
	return nil
}

// List 列出所有状态文件
func (sm *StateManager) List() ([]string, error) {
	// 读取目录
	files, err := os.ReadDir(sm.stateDir)
	if err != nil {
		return nil, gocd.NewDownloadError(err, "", "failed to read state directory")
	}

	var taskIDs []string
	for _, file := range files {
		if filepath.Ext(file.Name()) == ".json" {
			taskID := file.Name()[:len(file.Name())-5] // 移除.json扩展名
			taskIDs = append(taskIDs, taskID)
		}
	}

	return taskIDs, nil
}

// taskToState 将任务转换为持久化状态
func (sm *StateManager) taskToState(task *DownloadTask) *DownloadState {
	state := &DownloadState{
		Version:     StateFileVersion,
		TaskID:      task.ID,
		URL:         task.URL,
		FilePath:    task.FilePath,
		TotalSize:   task.TotalSize,
		Downloaded:  task.Downloaded,
		SupportRange: task.SupportRange,
		ChunkSize:   task.ChunkSize,
		Config:      task.Config,
		StartTime:   task.StartTime,
		LastUpdate:  time.Now(),
		Status:      task.GetStatus().String(),
	}

	if task.Error != nil {
		state.Error = task.Error.Error()
	}

	// 转换分片状态
	state.Chunks = make([]ChunkState, len(task.Chunks))
	for i, chunk := range task.Chunks {
		chunkState := ChunkState{
			Index:      chunk.Index,
			Start:      chunk.Start,
			End:        chunk.End,
			Size:       chunk.Size,
			Downloaded: chunk.Downloaded,
			Completed:  chunk.Completed,
			FilePath:   chunk.FilePath,
		}
		if chunk.Error != nil {
			chunkState.Error = chunk.Error.Error()
		}
		state.Chunks[i] = chunkState
	}

	return state
}

// stateToTask 将持久化状态转换为任务
func (sm *StateManager) stateToTask(state *DownloadState) (*DownloadTask, error) {
	// 创建任务
	task, err := NewDownloadTask(state.URL, state.FilePath, state.Config)
	if err != nil {
		return nil, err
	}

	// 设置任务属性
	task.ID = state.TaskID
	task.TotalSize = state.TotalSize
	task.Downloaded = state.Downloaded
	task.SupportRange = state.SupportRange
	task.ChunkSize = state.ChunkSize
	task.StartTime = state.StartTime
	task.LastUpdateTime = state.LastUpdate

	// 设置错误
	if state.Error != "" {
		task.Error = fmt.Errorf(state.Error)
	}

	// 设置任务状态
	switch state.Status {
	case "pending":
		task.SetStatus(TaskStatusPending)
	case "running":
		task.SetStatus(TaskStatusRunning)
	case "paused":
		task.SetStatus(TaskStatusPaused)
	case "completed":
		task.SetStatus(TaskStatusCompleted)
	case "failed":
		task.SetStatus(TaskStatusFailed)
	case "cancelled":
		task.SetStatus(TaskStatusCancelled)
	default:
		task.SetStatus(TaskStatusPending)
	}

	// 创建分片
	task.Chunks = make([]*Chunk, len(state.Chunks))
	for i, chunkState := range state.Chunks {
		chunk := &Chunk{
			Index:      chunkState.Index,
			Start:      chunkState.Start,
			End:        chunkState.End,
			Size:       chunkState.Size,
			Downloaded: chunkState.Downloaded,
			Completed:  chunkState.Completed,
			FilePath:   chunkState.FilePath,
		}
		if chunkState.Error != "" {
			chunk.Error = fmt.Errorf(chunkState.Error)
		}
		task.Chunks[i] = chunk
	}

	return task, nil
}

// getStateFilePath 获取状态文件路径
func (sm *StateManager) getStateFilePath(taskID string) string {
	return filepath.Join(sm.stateDir, taskID+".json")
}

// CleanupOldStates 清理旧的状态文件
func (sm *StateManager) CleanupOldStates(maxAge time.Duration) error {
	files, err := os.ReadDir(sm.stateDir)
	if err != nil {
		return err
	}

	now := time.Now()
	for _, file := range files {
		if filepath.Ext(file.Name()) != ".json" {
			continue
		}

		filePath := filepath.Join(sm.stateDir, file.Name())
		info, err := file.Info()
		if err != nil {
			continue
		}

		// 检查文件修改时间
		if now.Sub(info.ModTime()) > maxAge {
			// 读取文件检查任务状态
			content, err := os.ReadFile(filePath)
			if err == nil {
				var state DownloadState
				if json.Unmarshal(content, &state) == nil {
					// 只删除已完成、失败或取消的任务状态
					if state.Status == "completed" || state.Status == "failed" || state.Status == "cancelled" {
						os.Remove(filePath)
					}
				}
			}
		}
	}

	return nil
}

// SaveProgress 快速保存进度（轻量级保存，只更新关键信息）
func (sm *StateManager) SaveProgress(task *DownloadTask) error {
	if task == nil || !task.Config.EnableResume {
		return nil
	}

	// 创建简化的状态
	state := &DownloadState{
		Version:     StateFileVersion,
		TaskID:      task.ID,
		URL:         task.URL,
		FilePath:    task.FilePath,
		TotalSize:   task.TotalSize,
		Downloaded:  task.Downloaded,
		SupportRange: task.SupportRange,
		ChunkSize:   task.ChunkSize,
		Config:      task.Config,
		StartTime:   task.StartTime,
		LastUpdate:  time.Now(),
		Status:      task.GetStatus().String(),
	}

	// 只保存分片的关键信息
	state.Chunks = make([]ChunkState, len(task.Chunks))
	for i, chunk := range task.Chunks {
		state.Chunks[i] = ChunkState{
			Index:      chunk.Index,
			Downloaded: chunk.Downloaded,
			Completed:  chunk.Completed,
		}
	}

	// 保存到文件
	stateFilePath := sm.getStateFilePath(task.ID)
	tempFilePath := stateFilePath + ".tmp"

	file, err := os.Create(tempFilePath)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	if err := encoder.Encode(state); err != nil {
		return err
	}

	return os.Rename(tempFilePath, stateFilePath)
}