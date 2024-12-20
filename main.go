package main

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
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

// 获取文件的 MD5 值
func getFileMD5(filePath string) (string, error) {
	data, err := os.ReadFile(filePath) // 使用 os.ReadFile 代替 ioutil.ReadFile
	if err != nil {
		return "", err
	}
	hash := md5.Sum(data)
	return hex.EncodeToString(hash[:]), nil
}

// modifyFileContent 修改文件内容，随机加入数据，包括时间戳和随机数
func modifyFileContent(filePath string, r *rand.Rand) ([]byte, error) {
	// 读取文件内容
	data, err := os.ReadFile(filePath) // 使用 os.ReadFile 代替 ioutil.ReadFile
	if err != nil {
		return nil, err
	}

	// 获取当前时间戳
	currentTime := time.Now().Format("2006-01-02 15:04:05")

	// 随机数据
	randomData := fmt.Sprintf("\n# 随机数据: %d", r.Intn(10000))

	// 注入当前时间和随机数据到文件内容中
	modifiedData := fmt.Sprintf("\n# 当前时间: %s\n# 随机数据: %s\n", currentTime, randomData)

	// 将注入的内容和原数据合并
	modifiedDataBytes := []byte(modifiedData)
	data = append(data, modifiedDataBytes...)

	// 如果需要加入更复杂的内容，可以使用下面的方式
	// 比如拼接一个 JSON 格式的随机数据（作为示例）
	jsonData := fmt.Sprintf(`{"time": "%s", "random": %d}`, currentTime, r.Intn(10000))
	jsonBytes := []byte("\n" + jsonData)
	data = append(data, jsonBytes...)

	// 返回修改后的文件数据
	return data, nil
}

// cropImage 用于裁剪图像，裁剪掉四个边各 10 个像素
func cropImage(imgData []byte, cropWidth, cropHeight int) (image.Image, error) {
	// 解码图像
	img, _, err := image.Decode(bytes.NewReader(imgData))
	if err != nil {
		return nil, fmt.Errorf("解码图像时出错: %v", err)
	}

	// 获取图像的宽度和高度
	imgWidth := img.Bounds().Max.X
	imgHeight := img.Bounds().Max.Y

	// 计算裁剪区域
	cropRect := image.Rect(cropWidth, cropHeight, imgWidth-cropWidth, imgHeight-cropWidth)

	// 确保裁剪区域不会超出原图范围
	if cropRect.Dx() <= 0 || cropRect.Dy() <= 0 {
		return nil, fmt.Errorf("裁剪区域无效，可能导致宽度或高度为负值")
	}

	// 裁剪并返回裁剪后的图像
	croppedImg := img.(interface {
		SubImage(r image.Rectangle) image.Image
	}).SubImage(cropRect)

	return croppedImg, nil
}

// saveImage 根据文件类型保存裁剪后的图像
func saveImage(img image.Image, dstFile *os.File, format string) error {
	switch strings.ToLower(format) {
	case "jpeg", "jpg":
		// 使用JPEG格式保存图像，设置质量为80
		err := jpeg.Encode(dstFile, img, &jpeg.Options{Quality: 80})
		if err != nil {
			return fmt.Errorf("保存JPEG图像时出错: %v", err)
		}
	case "png":
		// 使用PNG格式保存图像，设置最高压缩级别
		encoder := png.Encoder{CompressionLevel: png.BestCompression} // 设置最高压缩级别
		err := encoder.Encode(dstFile, img)
		if err != nil {
			return fmt.Errorf("保存PNG图像时出错: %v", err)
		}
	default:
		return fmt.Errorf("不支持的图像格式: %s", format)
	}
	return nil
}

// createNewFolderAndSaveFile 创建文件夹并保存裁剪后的图像
func createNewFolderAndSaveFile(srcPath, destPath string, modifiedData []byte) (string, error) {
	// 创建目标文件夹
	err := os.MkdirAll(destPath, os.ModePerm)
	if err != nil {
		return "", err
	}

	cropWidth := rand.Intn(20) + 1  // 生成 1 到 20 之间的随机数
	cropHeight := rand.Intn(20) + 1 // 生成 1 到 20 之间的随机数

	// 调用裁剪函数
	croppedImg, err := cropImage(modifiedData, cropWidth, cropHeight)
	if err != nil {
		return "", err
	}

	// 生成目标文件路径
	newFilePath := filepath.Join(destPath, filepath.Base(srcPath))

	// 保存裁剪后的图像为PNG或JPEG
	dstFile, err := os.Create(newFilePath)
	if err != nil {
		return "", err
	}

	// 获取文件扩展名
	ext := filepath.Ext(newFilePath)
	ext = ext[1:] // 去掉扩展名的点

	// 根据文件类型保存图像
	err = saveImage(croppedImg, dstFile, ext)
	if err != nil {
		return "", err
	}

	return newFilePath, nil
}

// 删除文件夹及其内容（如果已存在）
func deleteFolderIfExist(modifiedFolder string) error {
	// 检查文件夹是否存在
	_, err := os.Stat(modifiedFolder)
	if err == nil { // 文件夹存在
		// 删除文件夹及其内容
		err := os.RemoveAll(modifiedFolder)
		if err != nil {
			return fmt.Errorf("删除文件夹 %s 时出错: %v", modifiedFolder, err)
		}
		fmt.Printf("已删除文件夹: %s\n", modifiedFolder)
	} else if os.IsNotExist(err) {
		// 如果文件夹不存在，则不需要删除
		fmt.Printf("文件夹 %s 不存在，无需删除\n", modifiedFolder)
	} else {
		return fmt.Errorf("检查文件夹 %s 时出错: %v", modifiedFolder, err)
	}

	// 创建新的文件夹
	err = os.Mkdir(modifiedFolder, os.ModePerm)
	if err != nil {
		return fmt.Errorf("创建文件夹 %s 时出错: %v", modifiedFolder, err)
	}
	fmt.Printf("已重新创建文件夹: %s\n", modifiedFolder)
	return nil
}

// 随机设置修改次数
func modifyFilesInFolder(folderPath string, modifyTimes int, r *rand.Rand) error {
	// 为每个子文件夹创建一个新的子文件夹来保存修改后的图片
	modifiedFolder := filepath.Join(folderPath, "modified")
	err := deleteFolderIfExist(modifiedFolder)
	if err != nil {
		log.Fatalf("删除和创建保存更新文件的文件夹失败: %v", err)
	}
	// 临时文件列表，用于存储临时文件路径
	var tempFiles []string

	// 遍历子文件夹中的所有文件
	err = filepath.Walk(folderPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// 跳过父文件夹、modify 文件夹及其子文件夹
		if path == folderPath || strings.Contains(path, "modified") {
			return nil
		}

		// 只处理图片文件
		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".jpg" || ext == ".png" || ext == ".jpeg" {
			// 获取修改前的 MD5
			originalMD5, err := getFileMD5(path)
			if err != nil {
				log.Printf("获取文件 %s 的 MD5 时出错: %v", path, err)
				return nil
			}

			// 随机决定修改次数
			randomModifications := r.Intn(modifyTimes) + 1

			// 最后的修改结果
			var modifiedData []byte
			var newFilePath string
			var modifiedMD5 string

			// 基于原始文件进行第一次修改
			modifiedData, err = modifyFileContent(path, r)
			if err != nil {
				log.Printf("修改文件 %s 时出错: %v", path, err)
				return nil
			}

			// 每次修改后都计算 MD5，但只保留最后一次修改后的文件
			for i := 0; i < randomModifications-1; i++ {
				// 将修改后的文件作为下一次修改的基础
				tempFilePath := filepath.Join(modifiedFolder, fmt.Sprintf("temp_%d", i))
				err = os.WriteFile(tempFilePath, modifiedData, 0644)
				if err != nil {
					log.Printf("保存临时文件 %s 时出错: %v", tempFilePath, err)
					continue
				}

				// 将临时文件路径添加到列表中
				tempFiles = append(tempFiles, tempFilePath)

				// 使用修改后的文件进行下一次修改
				modifiedData, err = modifyFileContent(tempFilePath, r)
				if err != nil {
					log.Printf("修改文件 %s 时出错: %v", tempFilePath, err)
					continue
				}
			}

			// 保存最后一次修改的文件到 `modified` 文件夹
			newFilePath, err = createNewFolderAndSaveFile(path, modifiedFolder, modifiedData)
			if err != nil {
				log.Printf("保存修改后的文件 %s 时出错: %v", path, err)
			}

			// 获取最终修改后的 MD5
			modifiedMD5, err = getFileMD5(newFilePath)
			if err != nil {
				log.Printf("获取修改后文件 %s 的 MD5 时出错: %v", path, err)
			}

			relativePath := filepath.Join(filepath.Base(filepath.Dir(path)), filepath.Base(path))
			// 输出最终的 MD5 对比日志（只输出最后一次修改后的结果）
			fmt.Printf("文件: %s，原始MD5：%s，修改后MD5：%s\n", relativePath, originalMD5, modifiedMD5)
		}

		return nil
	})

	// 删除所有临时文件
	for _, tempFile := range tempFiles {
		err := os.RemoveAll(tempFile)
		if err != nil {
			log.Printf("删除临时文件 %s 时出错: %v", tempFile, err)
		}
	}

	return err
}

func main() {
	// 使用 UnixNano 时间戳作为随机数种子，创建一个新的随机数生成器
	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	for {
		// 输入父文件夹路径
		var parentFolder string
		fmt.Print("请输入处理路径: ")
		fmt.Scanln(&parentFolder)

		// 输入随机修改次数的最大值
		var modifyTimes int
		fmt.Print("请输入每个文件最大修改次数: ")
		fmt.Scanln(&modifyTimes)

		// 遍历父文件夹中的所有子文件夹
		err := filepath.Walk(parentFolder, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			// 跳过父文件夹本身和 modify 文件夹
			if path == parentFolder || strings.Contains(path, "modified") {
				return nil
			}

			// 只处理子文件夹
			if info.IsDir() {
				fmt.Printf("===============正在处理文件夹=====================: %s\n", path)

				// 修改文件
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

		// 询问用户是否继续操作
		var userInput string
		fmt.Print("是否继续执行下一个操作？(yes/no): ")
		fmt.Scanln(&userInput)

		// 如果用户输入 'no' 或其他终止命令，则退出循环
		if strings.ToLower(userInput) != "yes" {
			fmt.Println("程序已退出.")
			break
		}
	}
}
