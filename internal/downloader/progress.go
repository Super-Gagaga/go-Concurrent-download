package downloader

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/yourusername/go-concurrent-download/pkg/gocd"
)

// ProgressMonitor 进度监视器
type ProgressMonitor struct {
	// 配置
	updateInterval time.Duration // 更新间隔

	// 状态存储
	history     []ProgressHistory
	historyLock sync.RWMutex
	maxHistory  int // 最大历史记录数

	// 回调函数
	callback gocd.ProgressFunc

	// 控制
	stopChan chan struct{}
	running  bool
}

// ProgressHistory 进度历史记录
type ProgressHistory struct {
	Timestamp time.Time           // 时间戳
	Progress  gocd.ProgressStatus // 进度状态
}

// NewProgressMonitor 创建新的进度监视器
func NewProgressMonitor(updateInterval time.Duration, callback gocd.ProgressFunc) *ProgressMonitor {
	return &ProgressMonitor{
		updateInterval: updateInterval,
		maxHistory:     1000, // 保存最近1000个记录
		callback:       callback,
		stopChan:       make(chan struct{}),
		history:        make([]ProgressHistory, 0, 1000),
	}
}

// Start 启动进度监视器
func (pm *ProgressMonitor) Start() {
	if pm.running {
		return
	}

	pm.running = true
	go pm.monitorLoop()
}

// Stop 停止进度监视器
func (pm *ProgressMonitor) Stop() {
	if !pm.running {
		return
	}

	pm.running = false
	close(pm.stopChan)
}

// Update 更新进度
func (pm *ProgressMonitor) Update(progress gocd.ProgressStatus) {
	// 保存历史记录
	pm.historyLock.Lock()
	defer pm.historyLock.Unlock()

	history := ProgressHistory{
		Timestamp: time.Now(),
		Progress:  progress,
	}

	pm.history = append(pm.history, history)

	// 限制历史记录大小
	if len(pm.history) > pm.maxHistory {
		pm.history = pm.history[1:]
	}

	// 调用回调函数
	if pm.callback != nil {
		pm.callback(progress)
	}
}

// GetHistory 获取历史记录
func (pm *ProgressMonitor) GetHistory() []ProgressHistory {
	pm.historyLock.RLock()
	defer pm.historyLock.RUnlock()

	// 返回副本
	history := make([]ProgressHistory, len(pm.history))
	copy(history, pm.history)
	return history
}

// GetRecentHistory 获取最近的历史记录
func (pm *ProgressMonitor) GetRecentHistory(count int) []ProgressHistory {
	pm.historyLock.RLock()
	defer pm.historyLock.RUnlock()

	if count <= 0 || len(pm.history) == 0 {
		return []ProgressHistory{}
	}

	if count > len(pm.history) {
		count = len(pm.history)
	}

	start := len(pm.history) - count
	history := make([]ProgressHistory, count)
	copy(history, pm.history[start:])
	return history
}

// CalculateAverageSpeed 计算平均下载速度
func (pm *ProgressMonitor) CalculateAverageSpeed(duration time.Duration) int64 {
	pm.historyLock.RLock()
	defer pm.historyLock.RUnlock()

	if len(pm.history) < 2 {
		return 0
	}

	// 获取指定时间范围内的记录
	cutoff := time.Now().Add(-duration)
	var startHistory, endHistory ProgressHistory
	startIdx := -1

	for i := len(pm.history) - 1; i >= 0; i-- {
		if pm.history[i].Timestamp.After(cutoff) {
			endHistory = pm.history[i]
			startIdx = i
		} else {
			break
		}
	}

	if startIdx > 0 {
		startHistory = pm.history[0]
	} else if len(pm.history) > 1 {
		startHistory = pm.history[0]
	} else {
		return 0
	}

	// 计算平均速度
	timeDiff := endHistory.Timestamp.Sub(startHistory.Timestamp).Seconds()
	if timeDiff <= 0 {
		return 0
	}

	dataDiff := endHistory.Progress.Downloaded - startHistory.Progress.Downloaded
	return int64(float64(dataDiff) / timeDiff)
}

// EstimateRemainingTime 估计剩余时间
func (pm *ProgressMonitor) EstimateRemainingTime(currentProgress gocd.ProgressStatus) time.Duration {
	if currentProgress.Speed <= 0 || currentProgress.TotalBytes <= 0 {
		return 0
	}

	remainingBytes := currentProgress.TotalBytes - currentProgress.Downloaded
	if remainingBytes <= 0 {
		return 0
	}

	seconds := float64(remainingBytes) / float64(currentProgress.Speed)
	return time.Duration(seconds) * time.Second
}

// FormatProgress 格式化进度输出
func (pm *ProgressMonitor) FormatProgress(progress gocd.ProgressStatus, format string) string {
	switch format {
	case "text":
		return pm.formatText(progress)
	case "json":
		return pm.formatJSON(progress)
	case "bar":
		return pm.formatProgressBar(progress)
	default:
		return pm.formatText(progress)
	}
}

// formatText 格式化文本输出
func (pm *ProgressMonitor) formatText(progress gocd.ProgressStatus) string {
	var builder strings.Builder

	builder.WriteString(fmt.Sprintf("任务: %s\n", progress.TaskID))
	builder.WriteString(fmt.Sprintf("文件: %s\n", progress.FileName))
	builder.WriteString(fmt.Sprintf("进度: %.2f%%\n", progress.Percentage))
	builder.WriteString(fmt.Sprintf("大小: %s / %s\n",
		formatBytes(progress.Downloaded),
		formatBytes(progress.TotalBytes)))
	builder.WriteString(fmt.Sprintf("速度: %s/s\n", formatBytes(progress.Speed)))

	if progress.RemainingTime > 0 {
		builder.WriteString(fmt.Sprintf("剩余: %s\n", formatDuration(progress.RemainingTime)))
	}

	if progress.IsCompleted {
		builder.WriteString("状态: 已完成\n")
	} else if progress.Error != nil {
		builder.WriteString(fmt.Sprintf("错误: %v\n", progress.Error))
	} else {
		builder.WriteString("状态: 下载中\n")
	}

	return builder.String()
}

// formatJSON 格式化JSON输出
func (pm *ProgressMonitor) formatJSON(progress gocd.ProgressStatus) string {
	// 简化实现，实际应使用json.Marshal
	return fmt.Sprintf(`{
	"task_id": "%s",
	"file_name": "%s",
	"percentage": %.2f,
	"downloaded": %d,
	"total": %d,
	"speed": %d,
	"remaining_time": %.0f,
	"completed": %v,
	"error": %v
}`,
		progress.TaskID,
		progress.FileName,
		progress.Percentage,
		progress.Downloaded,
		progress.TotalBytes,
		progress.Speed,
		progress.RemainingTime.Seconds(),
		progress.IsCompleted,
		progress.Error != nil,
	)
}

// formatProgressBar 格式化进度条
func (pm *ProgressMonitor) formatProgressBar(progress gocd.ProgressStatus) string {
	const width = 50 // 进度条宽度
	completed := int(float64(width) * progress.Percentage / 100)

	var barBuilder strings.Builder
	barBuilder.WriteString("[")

	for i := 0; i < width; i++ {
		if i < completed {
			barBuilder.WriteString("=")
		} else if i == completed {
			barBuilder.WriteString(">")
		} else {
			barBuilder.WriteString(" ")
		}
	}

	barBuilder.WriteString("]")

	return fmt.Sprintf("%s %.2f%% | %s/s | %s",
		barBuilder.String(),
		progress.Percentage,
		formatBytes(progress.Speed),
		formatDuration(progress.RemainingTime))
}

// monitorLoop 监控循环
func (pm *ProgressMonitor) monitorLoop() {
	ticker := time.NewTicker(pm.updateInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// 定期清理旧的历史记录
			pm.cleanupOldHistory()
		case <-pm.stopChan:
			return
		}
	}
}

// cleanupOldHistory 清理旧的历史记录
func (pm *ProgressMonitor) cleanupOldHistory() {
	pm.historyLock.Lock()
	defer pm.historyLock.Unlock()

	// 保留最近1小时的数据
	cutoff := time.Now().Add(-1 * time.Hour)
	startIdx := 0

	for i, history := range pm.history {
		if history.Timestamp.After(cutoff) {
			startIdx = i
			break
		}
	}

	if startIdx > 0 {
		pm.history = pm.history[startIdx:]
	}
}

// formatBytes 格式化字节显示
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// formatDuration 格式化时间显示
func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	if d < time.Hour {
		minutes := int(d.Minutes())
		seconds := int(d.Seconds()) % 60
		return fmt.Sprintf("%dm%ds", minutes, seconds)
	}
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	return fmt.Sprintf("%dh%dm", hours, minutes)
}

// ProgressTracker 进度跟踪器（用于单个任务）
type ProgressTracker struct {
	taskID     string
	monitor    *ProgressMonitor
	lastUpdate time.Time
	minInterval time.Duration // 最小更新间隔
}

// NewProgressTracker 创建新的进度跟踪器
func NewProgressTracker(taskID string, monitor *ProgressMonitor, minInterval time.Duration) *ProgressTracker {
	return &ProgressTracker{
		taskID:     taskID,
		monitor:    monitor,
		minInterval: minInterval,
	}
}

// Update 更新进度（带频率限制）
func (pt *ProgressTracker) Update(progress gocd.ProgressStatus) {
	now := time.Now()
	if now.Sub(pt.lastUpdate) < pt.minInterval && !progress.IsCompleted && progress.Error == nil {
		return
	}

	pt.lastUpdate = now
	pt.monitor.Update(progress)
}