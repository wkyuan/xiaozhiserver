package xiaozhi

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/go-audio/audio"
	"github.com/go-audio/wav"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"gopkg.in/hraban/opus.v2"

	"xiaozhi-esp32-server-golang/internal/util/workqueue"
)

func OpusToWav(opusData [][]byte, sampleRate int, channels int, fileName string) ([][]int16, error) {
	opusDecoder, err := opus.NewDecoder(sampleRate, channels)
	if err != nil {
		return nil, fmt.Errorf("创建Opus解码器失败: %v", err)
	}

	wavOut, err := os.Create(fileName)
	if err != nil {
		return nil, fmt.Errorf("创建WAV文件失败: %v", err)
	}

	pcmDataList := make([][]int16, 0)
	pcmBuffer := make([]int16, 4096)

	wavEncoder := wav.NewEncoder(wavOut, sampleRate, 16, channels, 1)
	wavBuffer := audio.IntBuffer{
		Format: &audio.Format{
			NumChannels: channels, // 使用传入的通道数
			SampleRate:  sampleRate,
		},
		SourceBitDepth: 16,
		Data:           make([]int, 4096),
	}

	for _, frame := range opusData {
		n, err := opusDecoder.Decode(frame, pcmBuffer)
		if err != nil {
			return nil, fmt.Errorf("解码失败: %v", err)
		}
		copyData := make([]int16, len(pcmBuffer[:n]))
		copy(copyData, pcmBuffer[:n])
		pcmDataList = append(pcmDataList, copyData)

		//fmt.Println("pcmData len: ", len(copyData))

		// 将PCM数据转换为int格式
		for i := 0; i < len(copyData); i++ {
			wavBuffer.Data = append(wavBuffer.Data, int(copyData[i]))
		}
	}

	// 写入WAV文件
	err = wavEncoder.Write(&wavBuffer)
	if err != nil {
		return nil, fmt.Errorf("写入WAV文件失败: %v", err)
	}

	wavEncoder.Close()

	return pcmDataList, nil
}

func initLog() error {
	// 使用标准输出而不是文件
	logrus.SetOutput(os.Stdout)

	// 禁用默认的调用者报告，使用自定义的caller字段
	logrus.SetReportCaller(false)
	logrus.SetFormatter(&logrus.TextFormatter{
		TimestampFormat: "2006-01-02 15:04:05.000", //时间格式化，添加毫秒
		ForceColors:     true,                      // 启用颜色输出
	})
	logLevel, _ := logrus.ParseLevel(viper.GetString("log.level"))
	if logLevel == 0 {
		logLevel = logrus.DebugLevel // 默认设置为Debug级别
	}
	logrus.SetLevel(logLevel)
	return nil
}

func TestTextToSpeechStream(t *testing.T) {
	//初始化log日志输出至标准输出
	//initLog()
	provider := NewXiaozhiProvider(map[string]interface{}{
		"server_addr": "wss://api.tenclass.net/xiaozhi/v1/",
		"device_id":   "ba:8f:17:de:94:94",
	})

	textList := []string{
		"你好，小智TTS单元测试",
		"讲个笑话",
		"今天天气怎么样",
		"你叫什么名字",
		"你今年几岁",
		"你住在哪里",
		"你喜欢吃什么",
		"你最喜欢什么颜色",
		"你最喜欢什么食物",
		"你最喜欢什么动物",
	}

	workqueue.ParallelizeUntil(context.Background(), 3, len(textList), func(piece int) {
		text := textList[piece]
		fmt.Println("开始 speech text: ", text)
		ch, err := provider.TextToSpeechStream(context.Background(), text)
		if err != nil {
			fmt.Println("TextToSpeechStream 连接失败: ", err)
			return
		}
		opusDataList := [][]byte{}
		for frame := range ch {
			opusDataList = append(opusDataList, frame)
			if len(frame) == 0 {
				t.Error("收到空音频帧")
			}
		}
		fmt.Printf("text: %s, 收到 %d 个音频帧\n", text, len(opusDataList))
	})

	/*
		for _, text := range textList {
			fmt.Println("开始 speech text: ", text)
			ch, err := provider.TextToSpeechStream(context.Background(), text)
			if err != nil {
				fmt.Println("TextToSpeechStream 连接失败: ", err)
				return
			}
			opusDataList := [][]byte{}
			for frame := range ch {
				opusDataList = append(opusDataList, frame)
				if len(frame) == 0 {
					t.Error("收到空音频帧")
				}
			}
			//OpusToWav(opusDataList, 24000, 1, "output_24000.wav")
		}*/

}
