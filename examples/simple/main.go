package main

import (
	"fmt"
	"log"
	"time"

	"github.com/Super-Gagaga/go-Concurrent-download/pkg/gocd"
)

func main() {
	fmt.Println("Go 并发文件下载工具 - 简单示例")
	fmt.Println("================================")

	// 示例1: 简单下载
	fmt.Println("\n1. 简单下载示例")
	err := gocd.Download(
		"https://releases.ubuntu.com/22.04.3/ubuntu-22.04.3-desktop-amd64.iso",
		"./ubuntu.iso",
	)
	if err != nil {
		log.Printf("下载失败: %v", err)
	} else {
		fmt.Println("下载完成!")
	}

	// 示例2: 带进度显示的下载
	fmt.Println("\n2. 带进度显示的下载示例")
	config := gocd.DownloadConfig{
		Concurrency: 8,
		RetryCount:  3,
		ProgressFunc: func(status gocd.ProgressStatus) {
			if status.IsCompleted {
				fmt.Printf("\r下载完成! 文件: %s\n", status.FileName)
				return
			}
			if status.Error != nil {
				fmt.Printf("\r下载错误: %v\n", status.Error)
				return
			}
			fmt.Printf("\r进度: %6.2f%% | 速度: %8s/s | 剩余: %v",
				status.Percentage,
				formatBytes(status.Speed),
				formatDuration(status.RemainingTime),
			)
		},
	}

	err = gocd.DownloadWithConfig(
		"https://download.docker.com/linux/static/stable/x86_64/docker-24.0.6.tgz",
		"./docker.tgz",
		config,
	)
	if err != nil {
		log.Printf("下载失败: %v", err)
	} else {
		fmt.Println("\n下载完成!")
	}

	// 示例3: 使用选项函数
	fmt.Println("\n3. 使用选项函数示例")
	downloader := gocd.NewDownloader(
		gocd.WithConcurrency(4),
		gocd.WithRetryCount(5),
		gocd.WithProgressFunc(func(status gocd.ProgressStatus) {
			fmt.Printf("\r进度: %.1f%%", status.Percentage)
		}),
	)
	err = downloader.Download(
		"https://nodejs.org/dist/v20.9.0/node-v20.9.0.tar.gz",
		"./nodejs.tar.gz",
	)
	if err != nil {
		log.Printf("下载失败: %v", err)
	} else {
		fmt.Println("\n下载完成!")
	}

	fmt.Println("\n所有示例执行完毕!")
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
	if d < time.Minute {
		return fmt.Sprintf("%.0fs", d.Seconds())
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