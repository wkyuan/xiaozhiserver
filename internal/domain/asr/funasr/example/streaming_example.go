package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"xiaozhi-esp32-server-golang/internal/domain/asr/funasr"
)

func main() {
	// 获取当前工作目录
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Printf("获取当前工作目录失败: %v\n", err)
		return
	}

	// 计算配置文件路径
	configPath := filepath.Join(cwd, "config", "config.json")

	// 尝试多个可能的路径
	possiblePaths := []string{
		configPath,
		filepath.Join(cwd, "xiaozhi-esp32-server-golang", "config", "config.json"),
		filepath.Join(cwd, "..", "..", "..", "..", "config", "config.json"),
	}

	var finalConfigPath string
	for _, path := range possiblePaths {
		if _, err := os.Stat(path); err == nil {
			finalConfigPath = path
			break
		}
	}

	if finalConfigPath == "" {
		fmt.Println("未找到配置文件，将使用默认配置")
	} else {
		fmt.Printf("使用配置文件: %s\n", finalConfigPath)
	}

	// 使用配置创建ASR实例
	asr := funasr.NewFunASRClient(finalConfigPath)
	defer asr.Close()

	// 示例音频文件路径
	audioFilePath := "test.wav"

	// 检查音频文件是否存在
	if _, err := os.Stat(audioFilePath); os.IsNotExist(err) {
		fmt.Printf("音频文件 %s 不存在\n", audioFilePath)
		fmt.Println("请提供有效的音频文件路径")
		return
	}

	// 执行流式识别
	result, err := asr.Recognize(audioFilePath)
	if err != nil {
		fmt.Printf("识别失败: %v\n", err)
		return
	}

	// 格式化并打印结果
	fmt.Println("识别结果:")
	fmt.Println(strings.Repeat("-", 40))
	fmt.Println(result)
	fmt.Println(strings.Repeat("-", 40))
}
