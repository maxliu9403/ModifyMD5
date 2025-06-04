package main

import (
	"bufio"
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

func runProcess(parentFolder string, modifyTimes int, r *rand.Rand) {
	dirImages := make(map[string][]string)

	// 第一步：遍历并按文件夹分组
	err := filepath.Walk(parentFolder, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if strings.Contains(path, "modified") {
				return filepath.SkipDir
			}
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".jpg" && ext != ".jpeg" && ext != ".png" {
			return nil
		}

		dir := filepath.Dir(path)
		dirImages[dir] = append(dirImages[dir], path)
		return nil
	})
	if err != nil {
		log.Fatalf("遍历文件时出错: %v", err)
	}

	// 第二步：按目录处理文件，并在每个目录结束后输出日志
	for dir, files := range dirImages {
		modifiedDir := filepath.Join(dir, "modified")
		if err := deleteFolderIfExist(modifiedDir); err != nil {
			log.Printf("准备 modified 文件夹失败: %v", err)
		}

		for _, path := range files {
			originalMD5, err := getFileMD5(path)
			if err != nil {
				log.Printf("获取 MD5 错误: %v", err)
				continue
			}

			randomModifications := r.Intn(modifyTimes) + 1
			modifiedData, err := modifyFileContent(path, r)
			if err != nil {
				log.Printf("修改文件失败: %v", err)
				continue
			}

			var tempFiles []string
			for i := 0; i < randomModifications-1; i++ {
				tempPath := filepath.Join(os.TempDir(), fmt.Sprintf("temp_%d", i))
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

			newFilePath, err := createNewFolderAndSaveFile(path, modifiedDir, modifiedData)
			if err != nil {
				log.Printf("保存新文件失败: %v", err)
				continue
			}

			modifiedMD5, err := getFileMD5(newFilePath)
			if err != nil {
				log.Printf("获取新 MD5 错误: %v", err)
			}

			rel, _ := filepath.Rel(parentFolder, path)
			fmt.Printf("文件: %s，原始MD5：%s，修改后MD5：%s\n", rel, originalMD5, modifiedMD5)

			for _, temp := range tempFiles {
				_ = os.Remove(temp)
			}
		}

		relDir, _ := filepath.Rel(parentFolder, dir)
		fmt.Printf("✅ 已完成目录处理: %s\n", relDir)
	}
}

func main() {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	scanner := bufio.NewScanner(os.Stdin)

	for {
		var parentFolder string
		var modifyTimes int
		flagSet := flag.NewFlagSet("image-modifier", flag.ContinueOnError)
		autoOnce := flagSet.Bool("once", false, "只执行一次后退出（非交互）")
		pathPtr := flagSet.String("path", "", "处理路径（可选）")
		timesPtr := flagSet.Int("times", 0, "每个文件最大修改次数（可选）")
		flagSet.Parse(os.Args[1:])

		if *pathPtr != "" && *timesPtr > 0 {
			parentFolder = *pathPtr
			modifyTimes = *timesPtr
			fmt.Printf("使用命令行参数: 路径 = %s，最大修改次数 = %d\n", parentFolder, modifyTimes)
		} else {
			fmt.Print("请输入处理路径: ")
			scanner.Scan()
			parentFolder = scanner.Text()
			fmt.Print("请输入每个文件最大修改次数: ")
			scanner.Scan()
			fmt.Sscanf(scanner.Text(), "%d", &modifyTimes)
		}

		runProcess(parentFolder, modifyTimes, r)

		if *autoOnce {
			break
		}

		fmt.Print("是否继续执行？(y/n): ")
		scanner.Scan()
		answer := strings.TrimSpace(scanner.Text())
		if strings.ToLower(answer) != "y" {
			fmt.Println("已退出程序。")
			break
		}
	}
}
