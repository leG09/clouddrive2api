package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/ge-fei-fan/clouddrive2api"
)

func main() {
	// 命令行参数
	var (
		serverAddr = flag.String("server", "", "CloudDrive服务器地址")
		username   = flag.String("username", "", "用户名")
		password   = flag.String("password", "", "密码")
		command    = flag.String("command", "list", "命令: list, refresh-all, refresh-dir, upload")
		path       = flag.String("path", "", "目录路径")
		file       = flag.String("file", "", "文件路径 (用于上传)")
		verbose    = flag.Bool("verbose", false, "显示详细信息")
	)
	flag.Parse()

	// 创建客户端
	client := clouddrive2api.NewClient(*serverAddr, *username, *password, "/离线下载", "/上传文件夹")
	defer client.Close()

	// 登录
	if *verbose {
		fmt.Println("正在登录CloudDrive...")
	}
	err := client.Login()
	if err != nil {
		log.Fatalf("登录失败: %v", err)
	}
	if *verbose {
		fmt.Println("登录成功!")
	}

	startTime := time.Now()

	switch *command {
	case "list":
		// 列出云存储
		fmt.Println("获取云存储列表...")
		cloudAPIs, err := client.GetAllCloudApis()
		if err != nil {
			log.Fatalf("获取云存储列表失败: %v", err)
		}

		fmt.Printf("发现 %d 个云存储:\n", len(cloudAPIs.Apis))
		for i, api := range cloudAPIs.Apis {
			fmt.Printf("%d. %s (用户: %s)\n", i+1, api.Name, api.UserName)
		}

	case "refresh-all":
		// 刷新所有目录
		fmt.Println("开始刷新所有目录...")
		err = client.RefreshAllDirectories()
		if err != nil {
			log.Fatalf("刷新所有目录失败: %v", err)
		}
		fmt.Println("所有目录刷新完成!")

	case "refresh-dir":
		// 刷新指定目录
		if *path == "" {
			log.Fatal("请指定要刷新的目录路径 (-path)")
		}
		fmt.Printf("开始刷新目录: %s\n", *path)
		err = client.RefreshSpecificDirectory(*path)
		if err != nil {
			log.Fatalf("刷新目录失败: %v", err)
		}
		fmt.Printf("目录 %s 刷新完成!\n", *path)

	case "list-dir":
		// 列出目录内容
		if *path == "" {
			*path = "/"
		}
		fmt.Printf("正在获取目录内容: %s\n", *path)
		files, err := client.ListDirectoryContents(*path, true)
		if err != nil {
			log.Fatalf("获取目录内容失败: %v", err)
		}

		fmt.Printf("目录 %s 包含 %d 个文件/文件夹:\n", *path, len(files))
		for i, file := range files {
			fileType := "文件"
			if file.IsDirectory {
				fileType = "目录"
			}
			fmt.Printf("%d. %s (%s) - 大小: %d 字节\n",
				i+1, file.Name, fileType, file.Size)
		}

	case "upload":
		// 上传文件
		if *file == "" {
			log.Fatal("请指定要上传的文件路径 (-file)")
		}
		if *path == "" {
			*path = "/"
		}
		fmt.Printf("正在上传文件: %s 到目录: %s\n", *file, *path)
		err = client.Upload(*file, "")
		if err != nil {
			log.Fatalf("上传文件失败: %v", err)
		}
		fmt.Println("文件上传完成!")

	default:
		fmt.Println("未知命令:", *command)
		fmt.Println("可用命令: list, refresh-all, refresh-dir, list-dir, upload")
		os.Exit(1)
	}

	duration := time.Since(startTime)
	if *verbose {
		fmt.Printf("总耗时: %v\n", duration)
	}
}
