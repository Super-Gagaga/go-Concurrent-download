package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/Super-Gagaga/go-Concurrent-download/pkg/gocd"
)

var (
	version = "0.1.0"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "download", "dl":
		downloadCommand(os.Args[2:])
	case "batch":
		batchCommand(os.Args[2:])
	case "resume":
		resumeCommand(os.Args[2:])
	case "list":
		listCommand(os.Args[2:])
	case "status":
		statusCommand(os.Args[2:])
	case "version", "v":
		printVersion()
	case "help", "h", "-h", "--help":
		printUsage()
	default:
		fmt.Printf("未知命令: %s\n\n", command)
		printUsage()
		os.Exit(1)
	}
}

// downloadCommand 处理下载命令
func downloadCommand(args []string) {
	downloadCmd := flag.NewFlagSet("download", flag.ExitOnError)
	output := downloadCmd.String("output", "", "输出文件路径")
	concurrency := downloadCmd.Int("concurrency", 4, "并发数")
	retryCount := downloadCmd.Int("retry", 3, "重试次数")
	retryInterval := downloadCmd.Duration("retry-interval", time.Second, "重试间隔")
	timeout := downloadCmd.Duration("timeout", 0, "超时时间（0表示无限制）")
	checksum := downloadCmd.String("checksum", "", "校验和值")
	checksumType := downloadCmd.String("checksum-type", "md5", "校验和类型：md5, sha1, sha256, sha512")
	disableResume := downloadCmd.Bool("disable-resume", false, "禁用断点续传")
	quiet := downloadCmd.Bool("quiet", false, "安静模式，不显示进度")
	rateLimit := downloadCmd.Int64("rate-limit", 0, "下载速度限制（字节/秒）")

	if err := downloadCmd.Parse(args); err != nil {
		log.Fatal(err)
	}

	if downloadCmd.NArg() < 1 {
		fmt.Println("错误: 需要指定下载URL")
		fmt.Println("用法: gocd download [选项] <URL>")
		downloadCmd.PrintDefaults()
		os.Exit(1)
	}

	url := downloadCmd.Arg(0)
	outputPath := *output
	if outputPath == "" {
		outputPath = extractFilenameFromURL(url)
	}

	fmt.Printf("开始下载: %s -> %s\n", url, outputPath)
	fmt.Printf("并发数: %d, 重试次数: %d\n", *concurrency, *retryCount)

	// 创建配置
	config := gocd.DownloadConfig{
		Concurrency:    *concurrency,
		RetryCount:     *retryCount,
		RetryInterval:  *retryInterval,
		Timeout:        *timeout,
		EnableResume:   !*disableResume,
		Checksum:       *checksum,
		ChecksumType:   *checksumType,
		RateLimit:      *rateLimit,
	}

	// 设置进度回调
	if !*quiet {
		config.ProgressFunc = func(status gocd.ProgressStatus) {
			if status.IsCompleted {
				fmt.Printf("\r下载完成! 文件: %s, 大小: %s, 耗时: %v\n",
					status.FileName,
					formatBytes(status.TotalBytes),
					time.Since(status.StartTime).Round(time.Second))
				return
			}
			if status.Error != nil {
				fmt.Printf("\r下载错误: %v\n", status.Error)
				return
			}
			fmt.Printf("\r进度: %6.2f%% | 速度: %8s/s | 剩余: %v",
				status.Percentage,
				formatBytes(status.Speed),
				formatDuration(status.RemainingTime))
		}
	}

	// 创建上下文，支持取消
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_ = ctx // 防止未使用变量错误

	// 处理信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\n接收到终止信号，正在停止下载...")
		cancel()
	}()

	// 开始下载
	startTime := time.Now()
	err := gocd.DownloadWithConfig(url, outputPath, config)
	elapsed := time.Since(startTime)

	if err != nil {
		log.Printf("下载失败: %v", err)
		os.Exit(1)
	}

	if !*quiet {
		fmt.Printf("\n下载成功! 耗时: %v\n", elapsed.Round(time.Second))
	}
}

// batchCommand 处理批量下载命令
func batchCommand(args []string) {
	batchCmd := flag.NewFlagSet("batch", flag.ExitOnError)
	file := batchCmd.String("file", "", "包含URL列表的文件")
	outputDir := batchCmd.String("output", "./downloads", "输出目录")

	if err := batchCmd.Parse(args); err != nil {
		log.Fatal(err)
	}

	if *file == "" {
		fmt.Println("错误: 需要指定URL列表文件")
		fmt.Println("用法: gocd batch --file=<文件路径> [选项]")
		batchCmd.PrintDefaults()
		os.Exit(1)
	}

	fmt.Printf("批量下载: 文件=%s, 输出目录=%s\n", *file, *outputDir)
	fmt.Println("批量下载功能开发中...")
}

// resumeCommand 处理恢复下载命令
func resumeCommand(args []string) {
	resumeCmd := flag.NewFlagSet("resume", flag.ExitOnError)
	taskID := resumeCmd.String("task", "", "任务ID")
	stateFile := resumeCmd.String("state", "", "状态文件路径")
	listTasks := resumeCmd.Bool("list", false, "列出所有可恢复的任务")

	if err := resumeCmd.Parse(args); err != nil {
		log.Fatal(err)
	}

	if *listTasks {
		fmt.Println("可恢复的任务列表:")
		fmt.Println("  (功能开发中...)")
		return
	}

	if *taskID == "" && *stateFile == "" {
		fmt.Println("错误: 需要指定任务ID或状态文件")
		fmt.Println("用法: gocd resume --task=<任务ID> 或 gocd resume --state=<状态文件>")
		resumeCmd.PrintDefaults()
		os.Exit(1)
	}

	fmt.Println("断点续传功能开发中...")
}

// listCommand 列出所有任务
func listCommand(args []string) {
	fmt.Println("当前任务列表:")
	fmt.Println("  (功能开发中...)")
}

// statusCommand 查看任务状态
func statusCommand(args []string) {
	statusCmd := flag.NewFlagSet("status", flag.ExitOnError)
	taskID := statusCmd.String("task", "", "任务ID")

	if err := statusCmd.Parse(args); err != nil {
		log.Fatal(err)
	}

	if *taskID == "" {
		fmt.Println("错误: 需要指定任务ID")
		fmt.Println("用法: gocd status --task=<任务ID>")
		statusCmd.PrintDefaults()
		os.Exit(1)
	}

	fmt.Printf("任务状态: %s\n", *taskID)
	fmt.Println("任务状态查询功能开发中...")
}

// extractFilenameFromURL 从URL提取文件名
func extractFilenameFromURL(url string) string {
	// 移除查询参数
	if idx := strings.Index(url, "?"); idx != -1 {
		url = url[:idx]
	}
	// 移除片段标识符
	if idx := strings.Index(url, "#"); idx != -1 {
		url = url[:idx]
	}
	// 获取最后一部分作为文件名
	filename := filepath.Base(url)
	if filename == "" || filename == "." || filename == "/" {
		filename = "downloaded_file"
	}
	return filename
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

func printVersion() {
	fmt.Printf("Go Concurrent Download Tool v%s\n", version)
	fmt.Println("高性能并发文件下载工具")
	fmt.Println("GitHub: https://github.com/Super-Gagaga/go-Concurrent-download")
}

func printUsage() {
	fmt.Println(`Go Concurrent Download Tool (gocd)
高性能并发文件下载工具

用法:
  gocd <命令> [参数]

命令:
  download, dl  下载单个文件
  batch         批量下载多个文件
  resume        恢复中断的下载
  list          列出所有任务
  status        查看任务状态
  version, v    显示版本信息
  help, h       显示帮助信息

示例:
  gocd download https://example.com/file.zip
  gocd download -concurrency=8 -retry=5 https://example.com/file.zip ./myfile.zip
  gocd batch --file=urls.txt --output=./downloads
  gocd resume --task=task_123456
  gocd status --task=task_123456

详细文档: https://github.com/Super-Gagaga/go-Concurrent-download`)
}