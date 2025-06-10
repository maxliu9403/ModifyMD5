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
	"io"
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

// 定义压缩错误类型
var (
	ErrUnsupportedFormat  = fmt.Errorf("不支持的图像格式")
	ErrTargetSizeTooSmall = fmt.Errorf("目标大小过小，无法压缩")
)

// 增强版 saveImage，支持指定目标大小（单位KB）
func saveImage(img image.Image, dstFile *os.File, format string, targetKB int) error {
	format = strings.ToLower(format)

	// 不指定大小时使用默认压缩
	if targetKB <= 0 {
		return saveDefault(img, dstFile, format)
	}

	targetBytes := int64(targetKB) * 1024 // 转换为字节

	switch format {
	case "jpeg", "jpg", "png":
		return compressJPEG(img, dstFile, targetBytes)
	default:
		return fmt.Errorf("%w: %s", ErrUnsupportedFormat, format)
	}
}

// 默认保存方式（无目标大小）
func saveDefault(img image.Image, dstFile *os.File, format string) error {
	switch format {
	case "jpeg", "jpg", "png":
		return jpeg.Encode(dstFile, img, &jpeg.Options{Quality: 80})
	//case "png":
	//	encoder := png.Encoder{CompressionLevel: png.BestCompression}
	//	return encoder.Encode(dstFile, img)
	default:
		return fmt.Errorf("%w: %s", ErrUnsupportedFormat, format)
	}
}

// JPEG压缩（带大小控制）
func compressJPEG(img image.Image, dstFile *os.File, targetBytes int64) error {
	// 边界检查
	minBuf := bytes.NewBuffer(nil)
	if err := jpeg.Encode(minBuf, img, &jpeg.Options{Quality: 1}); err != nil {
		return err
	}
	if int64(minBuf.Len()) > targetBytes {
		return fmt.Errorf("%w: 最低质量图像仍大于目标大小", ErrTargetSizeTooSmall)
	}

	// 二分法查找最佳质量参数
	low, high := 1, 100
	var bestBuf *bytes.Buffer

	for low <= high {
		mid := (low + high) / 2
		buf := bytes.NewBuffer(nil)

		if err := jpeg.Encode(buf, img, &jpeg.Options{Quality: mid}); err != nil {
			return err
		}

		size := int64(buf.Len())

		if size <= targetBytes {

			bestBuf = buf
			low = mid + 1 // 尝试更高质量
		} else {
			high = mid - 1 // 质量过高，需要降低
		}
	}

	// 写入最佳结果
	if bestBuf != nil {
		_, err := io.Copy(dstFile, bestBuf)
		return err
	}

	// 保底：使用最低质量
	_, err := io.Copy(dstFile, minBuf)
	return err
}

// PNG压缩（带大小控制）
func compressPNG(img image.Image, dstFile *os.File, targetBytes int64) error {
	// 尝试最高压缩级别
	buf := bytes.NewBuffer(nil)
	encoder := png.Encoder{CompressionLevel: png.BestCompression}
	if err := encoder.Encode(buf, img); err != nil {
		return err
	}

	// 检查是否满足大小要求
	if int64(buf.Len()) <= targetBytes {
		_, err := io.Copy(dstFile, buf)
		return err
	}

	// PNG无法进一步压缩，返回错误但仍保存
	_, _ = io.Copy(dstFile, buf)
	return fmt.Errorf("PNG压缩后仍超出目标大小 (%.2fKB > %.2fKB)",
		float64(buf.Len())/1024,
		float64(targetBytes)/1024)
}

func createNewFolderAndSaveFile(srcPath, destPath string, modifiedData []byte, targetKB int) (string, error) {
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
	err = saveImage(croppedImg, dstFile, ext, targetKB)
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

func runProcess(parentFolder string, modifyTimes, targetKB int, r *rand.Rand) {
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

			newFilePath, err := createNewFolderAndSaveFile(path, modifiedDir, modifiedData, targetKB)
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
		var targetKB, modifyTimes int
		var parentFolder string
		flagSet := flag.NewFlagSet("image-modifier", flag.ContinueOnError)
		autoOnce := flagSet.Bool("once", false, "只执行一次后退出（非交互）")
		pathPtr := flagSet.String("path", "", "处理路径（可选）")
		timesPtr := flagSet.Int("times", 0, "每个文件最大修改次数（可选）")
		targetKBPtr := flagSet.Int("targetKB", 300, "压缩图片文件的大小，单位KB，默认300KB（可选）")
		err := flagSet.Parse(os.Args[1:])
		if err != nil {
			return
		}

		if *pathPtr != "" && *timesPtr > 0 && *targetKBPtr > 0 {
			parentFolder = *pathPtr
			modifyTimes = *timesPtr
			targetKB = *targetKBPtr
			fmt.Printf("使用命令行参数: 路径 = %s，最大修改次数 = %d，压缩图片大小 = %d\n", parentFolder, modifyTimes, targetKBPtr)
		} else {
			fmt.Print("请输入处理路径: ")
			scanner.Scan()
			parentFolder = scanner.Text()
			fmt.Print("请输入每个文件最大修改次数：")
			scanner.Scan()
			fmt.Sscanf(scanner.Text(), "%d", &modifyTimes)
			fmt.Print("请输入压缩图片的大小需0KB，默认压缩到80%: ")
			scanner.Scan()
			fmt.Sscanf(scanner.Text(), "%d", &targetKB)
		}

		runProcess(parentFolder, modifyTimes, targetKB, r)

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
