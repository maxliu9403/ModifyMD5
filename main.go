package main

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"flag"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func getFileMD5(filePath string) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	hash := md5.Sum(data)
	return hex.EncodeToString(hash[:]), nil
}

func modifyFileContent(filePath string, r *rand.Rand) ([]byte, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	currentTime := time.Now().Format("2006-01-02 15:04:05")
	randomData := fmt.Sprintf("\n# 随机数据: %d", r.Intn(10000))
	modifiedData := fmt.Sprintf("\n# 当前时间: %s\n# 随机数据: %s\n", currentTime, randomData)

	data = append(data, []byte(modifiedData)...)
	jsonData := fmt.Sprintf(`{"time": "%s", "random": %d}`, currentTime, r.Intn(10000))
	data = append(data, []byte("\n"+jsonData)...)

	return data, nil
}

func cropImage(imgData []byte, cropWidth, cropHeight int) (image.Image, error) {
	img, _, err := image.Decode(bytes.NewReader(imgData))
	if err != nil {
		return nil, fmt.Errorf("解码图像时出错: %v", err)
	}

	imgWidth := img.Bounds().Max.X
	imgHeight := img.Bounds().Max.Y
	cropRect := image.Rect(cropWidth, cropHeight, imgWidth-cropWidth, imgHeight-cropHeight)

	if cropRect.Dx() <= 0 || cropRect.Dy() <= 0 {
		return nil, fmt.Errorf("裁剪区域无效")
	}

	croppedImg := img.(interface {
		SubImage(r image.Rectangle) image.Image
	}).SubImage(cropRect)

	return croppedImg, nil
}

func saveImage(img image.Image, dstFile *os.File, format string) error {
	switch strings.ToLower(format) {
	case "jpeg", "jpg":
		return jpeg.Encode(dstFile, img, &jpeg.Options{Quality: 80})
	case "png":
		encoder := png.Encoder{CompressionLevel: png.BestCompression}
		return encoder.Encode(dstFile, img)
	default:
		return fmt.Errorf("不支持的图像格式: %s", format)
	}
}

func createNewFolderAndSaveFile(srcPath, destPath string, modifiedData []byte) (string, error) {
	err := os.MkdirAll(destPath, os.ModePerm)
	if err != nil {
		return "", err
	}

	cropWidth := rand.Intn(20) + 1
	cropHeight := rand.Intn(20) + 1

	croppedImg, err := cropImage(modifiedData, cropWidth, cropHeight)
	if err != nil {
		return "", err
	}

	newFilePath := filepath.Join(destPath, filepath.Base(srcPath))
	dstFile, err := os.Create(newFilePath)
	if err != nil {
		return "", err
	}
	defer dstFile.Close()

	ext := strings.TrimPrefix(filepath.Ext(newFilePath), ".")
	err = saveImage(croppedImg, dstFile, ext)
	return newFilePath, err
}

func deleteFolderIfExist(modifiedFolder string) error {
	_, err := os.Stat(modifiedFolder)
	if err == nil {
		if err := os.RemoveAll(modifiedFolder); err != nil {
			return fmt.Errorf("删除文件夹失败: %v", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("检查文件夹失败: %v", err)
	}

	if err := os.Mkdir(modifiedFolder, os.ModePerm); err != nil {
		return fmt.Errorf("创建文件夹失败: %v", err)
	}
	return nil
}

func modifyFilesInFolder(folderPath string, modifyTimes int, r *rand.Rand) error {
	modifiedFolder := filepath.Join(folderPath, "modified")
	if err := deleteFolderIfExist(modifiedFolder); err != nil {
		log.Fatalf("准备保存文件夹失败: %v", err)
	}

	var tempFiles []string

	err := filepath.Walk(folderPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || path == folderPath || strings.Contains(path, "modified") {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".jpg" && ext != ".jpeg" && ext != ".png" {
			return nil
		}

		originalMD5, err := getFileMD5(path)
		if err != nil {
			log.Printf("获取 MD5 错误: %v", err)
			return nil
		}

		randomModifications := r.Intn(modifyTimes) + 1
		modifiedData, err := modifyFileContent(path, r)
		if err != nil {
			log.Printf("修改文件失败: %v", err)
			return nil
		}

		for i := 0; i < randomModifications-1; i++ {
			tempPath := filepath.Join(modifiedFolder, fmt.Sprintf("temp_%d", i))
			if err := os.WriteFile(tempPath, modifiedData, 0644); err != nil {
				log.Printf("写临时文件失败: %v", err)
				continue
			}
			tempFiles = append(tempFiles, tempPath)
			modifiedData, err = modifyFileContent(tempPath, r)
			if err != nil {
				log.Printf("二次修改失败: %v", err)
				continue
			}
		}

		newFilePath, err := createNewFolderAndSaveFile(path, modifiedFolder, modifiedData)
		if err != nil {
			log.Printf("保存新文件失败: %v", err)
			return nil
		}

		modifiedMD5, err := getFileMD5(newFilePath)
		if err != nil {
			log.Printf("获取新 MD5 错误: %v", err)
		}

		relativePath := filepath.Join(filepath.Base(filepath.Dir(path)), filepath.Base(path))
		fmt.Printf("文件: %s，原始MD5：%s，修改后MD5：%s\n", relativePath, originalMD5, modifiedMD5)
		return nil
	})

	for _, temp := range tempFiles {
		_ = os.Remove(temp)
	}
	return err
}

func main() {
	// 使用 UnixNano 时间戳作为随机数种子，创建一个新的随机数生成器
	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	// 命令行参数定义
	pathPtr := flag.String("path", "", "处理路径（可选）")
	timesPtr := flag.Int("times", 0, "每个文件最大修改次数（可选）")
	flag.Parse()

	var parentFolder string
	var modifyTimes int

	// 判断是否提供了命令行参数
	if *pathPtr != "" && *timesPtr > 0 {
		parentFolder = *pathPtr
		modifyTimes = *timesPtr
		fmt.Printf("使用命令行参数: 路径 = %s，最大修改次数 = %d\n", parentFolder, modifyTimes)
	} else {
		fmt.Print("请输入处理路径: ")
		fmt.Scanln(&parentFolder)

		fmt.Print("请输入每个文件最大修改次数: ")
		fmt.Scanln(&modifyTimes)
	}

	for {
		// 遍历父文件夹中的所有子文件夹
		err := filepath.Walk(parentFolder, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if path == parentFolder || strings.Contains(path, "modified") {
				return nil
			}

			if info.IsDir() {
				fmt.Printf("===============正在处理文件夹=====================: %s\n", path)
				err := modifyFilesInFolder(path, modifyTimes, r)
				if err != nil {
					log.Printf("处理文件夹 %s 时出错: %v", path, err)
				}
			}
			return nil
		})

		if err != nil {
			log.Fatalf("遍历文件树时出错: %v", err)
		}

		// 如果是通过命令行传参启动，则运行一次后直接退出
		if *pathPtr != "" && *timesPtr > 0 {
			break
		}

		// 否则进入交互模式
		var userInput string
		fmt.Print("是否继续执行下一个操作？(yes/no): ")
		fmt.Scanln(&userInput)
		if strings.ToLower(userInput) != "yes" {
			fmt.Println("程序已退出.")
			break
		}

		// 重新输入新参数
		fmt.Print("请输入处理路径: ")
		fmt.Scanln(&parentFolder)
		fmt.Print("请输入每个文件最大修改次数: ")
		fmt.Scanln(&modifyTimes)
	}
}
