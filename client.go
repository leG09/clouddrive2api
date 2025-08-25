package clouddrive2api

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"net/url"

	"github.com/ge-fei-fan/clouddrive2api/clouddrive"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/emptypb"
)

const DEFAULT_BUFFER_SIZE = 8192 // 默认缓冲区大小

type Client struct {
	addr              string
	conn              *grpc.ClientConn
	cd                clouddrive.CloudDriveFileSrvClient
	contextWithHeader context.Context
	username          string
	password          string
	OfflineFolder     string
	UploadFolder      string
}

func NewClient(addr, username, password, offlineFolder, uploadFolder string) *Client {
	c := Client{
		addr:              addr,
		conn:              nil,
		cd:                nil,
		contextWithHeader: nil,
		username:          username,
		password:          password,
		OfflineFolder:     offlineFolder,
		UploadFolder:      uploadFolder,
	}
	return &c
}

func (c *Client) Close() {
	if c.conn != nil {
		_ = c.conn.Close()
	}
}

func (c *Client) Login() error {
	// 兼容传入 http(s):// 前缀或包含路径的地址
	dialAddr := strings.TrimSpace(c.addr)
	if strings.HasPrefix(dialAddr, "http://") || strings.HasPrefix(dialAddr, "https://") {
		if u, err := url.Parse(dialAddr); err == nil {
			if u.Host != "" {
				dialAddr = u.Host
			}
		}
	}

	conn, err := grpc.Dial(dialAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}
	c.conn = conn
	c.cd = clouddrive.NewCloudDriveFileSrvClient(c.conn)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer func() {
		cancel()
	}()
	r, err := c.cd.GetToken(ctx, &clouddrive.GetTokenRequest{UserName: c.username, Password: c.password})
	if err != nil {
		return err
	}
	header := metadata.New(map[string]string{
		"authorization": "Bearer " + r.GetToken(),
	})
	c.contextWithHeader = metadata.NewOutgoingContext(context.Background(), header)
	return nil
}

func (c *Client) Set115Cookie(ck string) error {
	res, err := c.cd.APILogin115Editthiscookie(c.contextWithHeader, &clouddrive.Login115EditthiscookieRequest{EditThiscookieString: ck})
	if err != nil {
		return err
	}
	if !res.Success {
		return errors.New(res.ErrorMessage)
	}
	return nil
}

func (c *Client) Get115QrCode(platformString string) (string, error) {
	res, err := c.cd.APILogin115QRCode(c.contextWithHeader, &clouddrive.Login115QrCodeRequest{PlatformString: &platformString})
	if err != nil {
		return "", err
	}
	msg, err := res.Recv()
	if err != nil {
		return "", err
	}
	return msg.GetMessage(), nil
}

func (c *Client) AddOfflineFiles(url string) ([]string, error) {
	res, err := c.cd.AddOfflineFiles(c.contextWithHeader, &clouddrive.AddOfflineFileRequest{Urls: url, ToFolder: c.OfflineFolder})
	if err != nil {
		return nil, err
	}

	return res.GetResultFilePaths(), nil
}
func (c *Client) ListAllOfflineFiles(cloudName, cloudAccountId string, page uint32) ([]*clouddrive.OfflineFile, error) {
	res, err := c.cd.ListAllOfflineFiles(c.contextWithHeader, &clouddrive.OfflineFileListAllRequest{
		CloudName:      cloudName,
		CloudAccountId: cloudAccountId,
		Page:           page,
	})
	if err != nil {
		return nil, err
	}
	return res.GetOfflineFiles(), nil
}

func (c *Client) Upload(filePath, fileName string) error {
	return c.UploadToPath(filePath, fileName, c.UploadFolder)
}

func (c *Client) UploadToPath(filePath, fileName, targetPath string) error {
	var createFileResult *clouddrive.CreateFileResult
	var file *os.File
	if fileName == "" {
		fileName = filepath.Base(filePath)
	}
	defer func() {
		if file != nil {
			_ = file.Close()
		}
		if createFileResult != nil {
			_, _ = c.cd.CloseFile(c.contextWithHeader, &clouddrive.CloseFileRequest{FileHandle: createFileResult.FileHandle})
		}
	}()
	createFileResult, err := c.cd.CreateFile(c.contextWithHeader, &clouddrive.CreateFileRequest{ParentPath: targetPath, FileName: fileName})
	if err != nil {
		return err
	}
	// 打开文件
	file, err = os.Open(filePath)
	if err != nil {
		return err
	}
	// 如果传入了文件对象
	if file != nil {
		offset := uint64(0)
		// 循环读取文件内容并写入到云端文件
		for {
			reader := bufio.NewReader(file)
			data := make([]byte, DEFAULT_BUFFER_SIZE)
			n, err := reader.Read(data)
			if err != nil && err != io.EOF {
				return err
			}
			if n == 0 {
				break
			}
			// 将文件内容写入到云端文件
			_, err = c.cd.WriteToFile(c.contextWithHeader, &clouddrive.WriteFileRequest{FileHandle: createFileResult.FileHandle, StartPos: offset, Length: uint64(n), Buffer: data[:n], CloseFile: false})
			if err != nil {
				return err
			}
			//fmt.Println(res.GetBytesWritten())
			offset += uint64(n)
		}
	}
	return nil
}

func (c *Client) GetSubFiles(path string, forceRefresh bool, checkExpires bool) (*clouddrive.SubFilesReply, error) {
	stream, err := c.cd.GetSubFiles(
		c.contextWithHeader,
		&clouddrive.ListSubFileRequest{Path: path, ForceRefresh: forceRefresh, CheckExpires: &checkExpires},
	)
	if err != nil {
		return nil, err
	}

	var all []*clouddrive.CloudDriveFile
	for {
		msg, recvErr := stream.Recv()
		if recvErr == io.EOF {
			break
		}
		if recvErr != nil {
			return nil, recvErr
		}
		if msg != nil && len(msg.SubFiles) > 0 {
			all = append(all, msg.SubFiles...)
		}
	}

	return &clouddrive.SubFilesReply{SubFiles: all}, nil
}

// RefreshAllDirectories 遍历并刷新所有目录
func (c *Client) RefreshAllDirectories(excludePaths []string) error {
	// 首先获取所有云API
	cloudAPIs, err := c.GetAllCloudApis()
	if err != nil {
		return err
	}

	// 遍历每个云API
	for _, api := range cloudAPIs.Apis {
		fmt.Printf("正在处理云存储: %s (用户: %s)\n", api.Name, api.UserName)

		// 从根目录开始遍历
		err := c.refreshDirectoryRecursively("/", api.Name, api.UserName, excludePaths)
		if err != nil {
			fmt.Printf("处理云存储 %s 时出错: %v\n", api.Name, err)
			continue
		}
	}

	return nil
}

// refreshDirectoryRecursively 递归刷新目录
func (c *Client) refreshDirectoryRecursively(path, cloudName, userName string, excludePaths []string) error {
	fmt.Printf("正在刷新目录: %s\n", path)

	// 检查当前路径是否在排除列表中
	for _, excludePath := range excludePaths {
		if path == excludePath {
			fmt.Printf("跳过排除的目录: %s\n", path)
			return nil
		}
	}

	// 获取子文件列表，强制刷新
	subFiles, err := c.GetSubFiles(path, true, true)
	if err != nil {
		return fmt.Errorf("获取目录 %s 的子文件失败: %v", path, err)
	}

	// 遍历子文件
	for _, file := range subFiles.SubFiles {
		// 如果是目录，递归处理
		if file.IsDirectory {
			fmt.Printf("发现子目录: %s\n", file.FullPathName)
			err := c.refreshDirectoryRecursively(file.FullPathName, cloudName, userName, excludePaths)
			if err != nil {
				fmt.Printf("刷新子目录 %s 时出错: %v\n", file.FullPathName, err)
				continue
			}
		}
	}

	return nil
}

// RefreshSpecificDirectory 刷新指定目录
func (c *Client) RefreshSpecificDirectory(path string) error {
	fmt.Printf("正在刷新指定目录: %s\n", path)

	// 获取子文件列表，强制刷新
	subFiles, err := c.GetSubFiles(path, true, true)
	if err != nil {
		return fmt.Errorf("获取目录 %s 的子文件失败: %v", path, err)
	}

	fmt.Printf("目录 %s 刷新完成，包含 %d 个文件/文件夹\n", path, len(subFiles.SubFiles))
	return nil
}

// GetAllCloudApis 获取所有云API
func (c *Client) GetAllCloudApis() (*clouddrive.CloudAPIList, error) {
	res, err := c.cd.GetAllCloudApis(c.contextWithHeader, &emptypb.Empty{})
	if err != nil {
		return nil, err
	}
	return res, nil
}

// GetDirectoryInfo 获取目录信息
func (c *Client) GetDirectoryInfo(path string) (*clouddrive.CloudDriveFile, error) {
	res, err := c.cd.FindFileByPath(c.contextWithHeader, &clouddrive.FindFileByPathRequest{
		ParentPath: filepath.Dir(path),
		Path:       filepath.Base(path),
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

// ListDirectoryContents 列出目录内容
func (c *Client) ListDirectoryContents(path string, forceRefresh bool) ([]*clouddrive.CloudDriveFile, error) {
	subFiles, err := c.GetSubFiles(path, forceRefresh, true)
	if err != nil {
		return nil, err
	}
	return subFiles.SubFiles, nil
}
