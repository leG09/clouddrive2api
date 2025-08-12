package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
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
		exclude    = flag.String("exclude", "", "排除的目录路径，多个路径用逗号分隔")
		verbose    = flag.Bool("verbose", false, "显示详细信息")
	)

	// 自定义 -h 帮助信息
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "CloudDrive CLI\n\n")
		fmt.Fprintf(os.Stderr, "用法:\n  %s [flags]\n\n", os.Args[0])
		fmt.Fprintln(os.Stderr, "命令:")
		fmt.Fprintln(os.Stderr, "  list          列出云存储")
		fmt.Fprintln(os.Stderr, "  refresh-all   刷新所有目录（支持 -exclude）")
		fmt.Fprintln(os.Stderr, "  refresh-dir   刷新指定目录（需 -path；可用逗号分隔多个路径）")
		fmt.Fprintln(os.Stderr, "  list-dir      列出目录内容（可选 -path，默认 /）")
		fmt.Fprintln(os.Stderr, "  upload        上传文件（需 -file，使用默认上传目录）")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "参数:")
		flag.PrintDefaults()
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "说明:")
		fmt.Fprintln(os.Stderr, "  -server 支持 'host:port' 或 'http(s)://host:port'，程序会自动处理协议前缀")
		fmt.Fprintln(os.Stderr, "  -exclude 为逗号分隔的路径列表，例如 '/tmp,/系统文件夹'")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "示例:")
		fmt.Fprintln(os.Stderr, "  "+os.Args[0]+" -command=refresh-all -exclude='/tmp,/temp'")
		fmt.Fprintln(os.Stderr, "  "+os.Args[0]+" -command=refresh-dir -path='/我的网盘/电影,/我的网盘/剧集'")
		fmt.Fprintln(os.Stderr, "  "+os.Args[0]+" -command=list-dir -path='/'")
	}
	flag.Parse()

	// 检查未知命令
	if *command != "list" && *command != "refresh-all" && *command != "refresh-dir" && *command != "list-dir" && *command != "upload" {
		fmt.Println("未知命令:", *command)
		fmt.Println("可用命令: list, refresh-all, refresh-dir, list-dir, upload")
		fmt.Println("示例:")
		fmt.Println("  ./clouddrive-cli -command=refresh-all -exclude='/tmp,/temp'")
		fmt.Println("  ./clouddrive-cli -command=refresh-all -exclude='/系统文件夹' -verbose")
		os.Exit(1)
	}

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

		// 解析排除路径
		var excludePaths []string
		if *exclude != "" {
			excludePaths = strings.Split(*exclude, ",")
			// 清理路径（去除空格）
			for i, path := range excludePaths {
				excludePaths[i] = strings.TrimSpace(path)
			}
			if *verbose {
				fmt.Printf("排除的目录: %v\n", excludePaths)
			}
		}

		err = client.RefreshAllDirectories(excludePaths)
		if err != nil {
			log.Fatalf("刷新所有目录失败: %v", err)
		}
		fmt.Println("所有目录刷新完成!")

	case "refresh-dir":
		// 刷新指定目录（支持多个，以逗号分隔）
		if *path == "" {
			log.Fatal("请指定要刷新的目录路径 (-path)，多个用逗号分隔")
		}
		pathList := strings.Split(*path, ",")
		for _, p := range pathList {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			fmt.Printf("开始刷新目录: %s\n", p)
			err = client.RefreshSpecificDirectory(p)
			if err != nil {
				fmt.Printf("刷新目录失败 %s: %v\n", p, err)
				continue
			}
			fmt.Printf("目录 %s 刷新完成!\n", p)
		}

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
		// 这里不应该到达，因为已经在前面检查了命令
		fmt.Println("未知命令:", *command)
		os.Exit(1)
	}

	duration := time.Since(startTime)
	if *verbose {
		fmt.Printf("总耗时: %v\n", duration)
	}
}
