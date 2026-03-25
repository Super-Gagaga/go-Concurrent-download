package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/yourusername/go-concurrent-download/pkg/gocd"
)

func main() {
	fmt.Println("Go 并发文件下载工具 - 高级示例")
	fmt.Println("================================")

	// 示例1: 带上下文和取消的下载
	fmt.Println("\n1. 带上下文和取消的下载")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 设置信号处理
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\n接收到终止信号，正在停止下载...")
		cancel()
	}()

	downloader := gocd.NewDownloader(
		gocd.WithConcurrency(4),
		gocd.WithRetryCount(3),
		gocd.WithProgressFunc(func(status gocd.ProgressStatus) {
			if status.IsCompleted {
				fmt.Printf("\r下载完成: %s (%.2f%%, %s/s)\n",
					status.FileName, status.Percentage,
					formatBytes(status.Speed))
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
		}),
	)

	// 在goroutine中启动下载
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := downloader.Download(
			"https://releases.ubuntu.com/22.04.3/ubuntu-22.04.3-desktop-amd64.iso",
			"./ubuntu.iso",
		)
		if err != nil {
			if ctx.Err() != nil {
				fmt.Println("\n下载被取消")
			} else {
				log.Printf("下载失败: %v", err)
			}
		}
	}()

	// 模拟在5秒后取消
	go func() {
		time.Sleep(5 * time.Second)
		fmt.Println("\n模拟5秒后取消下载...")
		cancel()
	}()

	wg.Wait()

	// 示例2: 批量下载带错误处理
	fmt.Println("\n2. 批量下载带错误处理")
	urls := []string{
		"https://download.docker.com/linux/static/stable/x86_64/docker-24.0.6.tgz",
		"https://nodejs.org/dist/v20.9.0/node-v20.9.0.tar.gz",
		"https://example.com/non-existent-file.zip", // 这个会失败
	}

	results := gocd.DownloadBatch(urls, "./downloads",
		gocd.WithConcurrency(2),
		gocd.WithRetryCount(2),
		gocd.WithProgressFunc(func(status gocd.ProgressStatus) {
			if status.IsCompleted {
				fmt.Printf("\r%s: 完成\n", status.FileName)
			}
		}),
	)

	fmt.Println("\n批量下载结果:")
	successCount := 0
	for _, result := range results {
		if result.Success {
			successCount++
			fmt.Printf("  ✓ %s: 成功 (%s)\n",
				result.FilePath,
				formatBytes(result.Size))
		} else {
			fmt.Printf("  ✗ %s: 失败 (%v)\n",
				result.URL,
				result.Error)
		}
	}
	fmt.Printf("成功: %d/%d\n", successCount, len(urls))

	// 示例3: 断点续传示例
	fmt.Println("\n3. 断点续传示例")
	downloader2 := gocd.NewDownloader(
		gocd.WithConcurrency(8),
		gocd.WithRetryCount(5),
		gocd.WithResume(true),
		gocd.WithStateFile("./download.state"),
		gocd.WithProgressFunc(func(status gocd.ProgressStatus) {
			if status.IsCompleted {
				fmt.Printf("\r下载完成! 可以重新运行程序继续下载\n")
			} else if status.Error != nil {
				fmt.Printf("\r错误: %v\n", status.Error)
			} else {
				fmt.Printf("\r进度: %.2f%% (已下载: %s)",
					status.Percentage,
					formatBytes(status.Downloaded))
			}
		}),
	)

	fmt.Println("开始大文件下载（支持断点续传）...")
	fmt.Println("按 Ctrl+C 中断，然后重新运行程序继续下载")

	err := downloader2.Download(
		"https://releases.ubuntu.com/22.04.3/ubuntu-22.04.3-desktop-amd64.iso",
		"./ubuntu-resume.iso",
	)
	if err != nil {
		log.Printf("下载失败: %v", err)
	}

	fmt.Println("\n所有高级示例演示完毕!")
	fmt.Println("请查看下载的文件:")
	fmt.Println("  - ./ubuntu.iso (可能不完整，因为被取消了)")
	fmt.Println("  - ./downloads/ 目录下的文件")
	fmt.Println("  - ./ubuntu-resume.iso (断点续传示例)")
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