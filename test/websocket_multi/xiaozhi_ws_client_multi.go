package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"

	"xiaozhi-esp32-server-golang/internal/domain/tts"

	"github.com/gorilla/websocket"
)

// 消息类型常量
const (
	MessageTypeHello  = "hello"
	MessageTypeListen = "listen"
	MessageTypeAbort  = "abort"
	MessageTypeIot    = "iot"
)

// 消息状态常量
const (
	MessageStateStart   = "start"
	MessageStateStop    = "stop"
	MessageStateDetect  = "detect"
	MessageStateSuccess = "success"
	MessageStateError   = "error"
	MessageStateAbort   = "abort"
)

// ClientMessage 表示客户端消息
type ClientMessage struct {
	Type        string       `json:"type"`
	DeviceID    string       `json:"device_id"`
	Text        string       `json:"text,omitempty"`
	Mode        string       `json:"mode,omitempty"`
	State       string       `json:"state,omitempty"`
	Token       string       `json:"token,omitempty"`
	DeviceMac   string       `json:"device_mac,omitempty"`
	Version     int          `json:"version,omitempty"`
	Transport   string       `json:"transport,omitempty"`
	AudioParams *AudioFormat `json:"audio_params,omitempty"`
}

// ServerMessage 表示服务器消息
type ServerMessage struct {
	Type        string       `json:"type"`
	Text        string       `json:"text,omitempty"`
	State       string       `json:"state,omitempty"`
	SessionID   string       `json:"session_id,omitempty"`
	Transport   string       `json:"transport,omitempty"`
	AudioFormat *AudioFormat `json:"audio_format,omitempty"`
}

// AudioFormat 表示音频格式
type AudioFormat struct {
	SampleRate    int    `json:"sample_rate"`
	Channels      int    `json:"channels"`
	FrameDuration int    `json:"frame_duration"`
	Format        string `json:"format"`
}

// Opus编码常量
const (
	// Opus编码的采样率
	SampleRate = 16000
	// 音频通道数
	Channels = 1
	// 每帧持续时间(毫秒)
	FrameDurationMs = 60
	// PCM缓冲区大小 = 采样率 * 通道数 * 帧持续时间(秒)
	PCMBufferSize = SampleRate * Channels * FrameDurationMs / 1000
)

type WsClient struct {
	DeviceId          string
	ClientId          string
	Token             string
	ServerAddr        string
	AudioFile         string
	Conn              *websocket.Conn
	avgResponseMs     int64
	firstRecvFrame    bool
	detectStartTs     int64
	index             int
	audioOpusDataChan chan AudioOpusData
}

var lock sync.RWMutex
var totalRequest int64
var avgResponseMs int64

func main() {
	// 解析命令行参数
	serverAddr := flag.String("server", "ws://localhost:8989/xiaozhi/v1/", "服务器地址")
	//audioFile := flag.String("audio", "../test.wav", "音频文件路径")
	clientCount := flag.Int("count", 10, "客户端数量")
	chatText := flag.String("text", "你好", "聊天内容, 多句以逗号分隔会依次发送")
	deviceId := flag.String("device", "", "设备ID")
	flag.Parse()

	fmt.Printf("运行小智客户端\n服务器: %s\n客户端数量: %d\n发送内容: %s\n",
		*serverAddr, *clientCount, *chatText)

	maxClient := 1
	if clientCount != nil {
		maxClient = *clientCount
	}

	//生成音频数据
	textList := strings.Split(*chatText, ",")
	audioOpusDataList, err := genAudioOpusDataList(textList)
	if err != nil {
		fmt.Printf("生成音频数据失败: %v\n", err)
		return
	}

	// 运行客户端
	for i := 0; i < maxClient; i++ {
		go func() {
			client := &WsClient{
				ServerAddr:        *serverAddr,
				index:             i,
				audioOpusDataChan: make(chan AudioOpusData),
				DeviceId:          *deviceId,
			}

			if err := client.runClient(audioOpusDataList); err != nil {
				log.Fatalf("客户端运行失败: %v", err)
			}
		}()
	}

	go func() {
		for {
			time.Sleep(2 * time.Second)
			lock.Lock()
			fmt.Printf("请求%d次, 平均响应时间: %d 毫秒\n", totalRequest, avgResponseMs)
			lock.Unlock()
		}
	}()

	//阻塞
	select {}
}

// runClient 运行小智客户端
func (w *WsClient) runClient(audioOpusDataList []AudioOpusData) error {
	if len(audioOpusDataList) == 0 {
		fmt.Printf("音频数据列表为空\n")
		return fmt.Errorf("音频数据列表为空")
	}

	fmt.Printf("%d 客户端开始运行\n", w.index)

	if w.DeviceId == "" {
		w.DeviceId = genDeviceId()
	}
	w.ClientId = genClientId()

	// 设置HTTP头
	header := http.Header{}
	header.Set("Device-Id", w.DeviceId)
	header.Set("Content-Type", "application/json")
	header.Set("Authorization", "Bearer "+w.Token)
	header.Set("Protocol-Version", "1")
	header.Set("Client-Id", w.ClientId)

	var err error
	// 连接WebSocket服务器
	w.Conn, _, err = websocket.DefaultDialer.Dial(w.ServerAddr, header)
	if err != nil {
		return fmt.Errorf("连接失败: %v", err)
	}
	defer w.Conn.Close()

	fmt.Printf("%d 客户端已连接到服务器: %s\n", w.index, w.ServerAddr)

	// 设置消息处理
	done := make(chan struct{})

	audioDataIndex := 0
	// 启动一个协程来处理从服务器接收的消息
	go func() {
		defer close(done)
		//var recvInterval int64
		for {
			messageType, message, err := w.Conn.ReadMessage()
			if err != nil {
				fmt.Printf("读取消息失败: %v\n", err)
				return
			}

			if messageType == websocket.TextMessage {
				var serverMsg ServerMessage
				if err := json.Unmarshal(message, &serverMsg); err != nil {
					fmt.Printf("解析消息失败: %v\n", err)
					continue
				}

				fmt.Printf("收到消息: %+v\n", serverMsg)

				if serverMsg.Type == "hello" {
					if audioDataIndex >= len(audioOpusDataList) {
						audioDataIndex = 0
					}
					w.audioOpusDataChan <- audioOpusDataList[audioDataIndex]
					audioDataIndex++

				}
				if serverMsg.Type == "tts" && serverMsg.State == "stop" {
					if audioDataIndex >= len(audioOpusDataList) {
						audioDataIndex = 0
					}
					w.audioOpusDataChan <- audioOpusDataList[audioDataIndex]
					audioDataIndex++
				}
			} else if messageType == websocket.BinaryMessage {
				if !w.firstRecvFrame {
					w.firstRecvFrame = true
					diffMs := time.Now().UnixMilli() - w.detectStartTs
					//fmt.Printf("%d 客户端 首帧到达时间: %d 毫秒\n", w.index, diffMs)
					lock.Lock()
					totalRequest++
					if avgResponseMs == 0 {
						avgResponseMs = diffMs
					} else {
						avgResponseMs = (avgResponseMs + diffMs) / 2
					}
					lock.Unlock()
					//os.WriteFile("ws_output_first_frame.wav", message, 0644)
				}
			}
		}
	}()

	w.sendHello()

	// 读取并发送音频文件（使用Opus编码)
	w.sendAudioDataToServer()

	return nil
}

func (w *WsClient) sendHello() error {
	// 发送hello消息
	helloMsg := ClientMessage{
		Type:      MessageTypeHello,
		DeviceID:  w.DeviceId,
		Transport: "websocket",
		Version:   1,
		AudioParams: &AudioFormat{
			SampleRate:    SampleRate,
			Channels:      Channels,
			FrameDuration: FrameDurationMs,
			Format:        "opus",
		},
	}

	if err := sendJSONMessage(w.Conn, helloMsg); err != nil {
		return fmt.Errorf("发送hello消息失败: %v", err)
	}
	return nil
}

func (w *WsClient) sendListenStart() error {
	// 发送listen start消息
	listenStartMsg := ClientMessage{
		Type:     MessageTypeListen,
		DeviceID: w.DeviceId,
		State:    MessageStateStart,
		Mode:     "manual",
	}

	if err := sendJSONMessage(w.Conn, listenStartMsg); err != nil {
		return fmt.Errorf("发送listen start消息失败: %v", err)
	}
	return nil
}

func (w *WsClient) sendListenStop() error {
	// 发送listen start消息
	listenStartMsg := ClientMessage{
		Type:     MessageTypeListen,
		DeviceID: w.DeviceId,
		State:    MessageStateStop,
		Mode:     "manual",
	}

	if err := sendJSONMessage(w.Conn, listenStartMsg); err != nil {
		return fmt.Errorf("发送listen stop消息失败: %v", err)
	}

	w.detectStartTs = time.Now().UnixMilli()
	w.firstRecvFrame = false

	return nil
}

func (w *WsClient) sendListenDetect(text string) error {
	// 发送listen start消息
	listenStartMsg := ClientMessage{
		Type:     MessageTypeListen,
		DeviceID: w.DeviceId,
		State:    MessageStateDetect,
		Text:     text,
	}

	if err := sendJSONMessage(w.Conn, listenStartMsg); err != nil {
		return fmt.Errorf("发送listen detect消息失败: %v", err)
	}
	return nil
}

// 发送JSON消息
func sendJSONMessage(conn *websocket.Conn, msg interface{}) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	//fmt.Printf("发送消息: %s\n", string(data))
	return conn.WriteMessage(websocket.TextMessage, data)
}

func genDeviceId() string {
	//return "f2:99:72:0a:bf:30"
	//return "30:ed:a0:1f:4c:bc" //java
	//生成mac地址格式的字符串
	rand.Seed(time.Now().UnixNano())
	mac := make([]byte, 6)
	rand.Read(mac)
	return fmt.Sprintf("%02X:%02X:%02X:%02X:%02X:%02X", mac[0], mac[1], mac[2], mac[3], mac[4], mac[5])
}

func genClientId() string {
	return "e4b0c442-98fc-4e1b-8c3d-6a5b6a5b6a6e"
}

type AudioOpusData struct {
	OpusData [][]byte
	Duration int
}

func genAudioOpusDataList(textList []string) ([]AudioOpusData, error) {
	cosyVoiceConfig := map[string]interface{}{
		"api_url":        "https://tts.linkerai.cn/tts",
		"spk_id":         "OUeAo1mhq6IBExi",
		"frame_duration": FrameDurationMs,
		"target_sr":      SampleRate,
		"audio_format":   "mp3",
		"instruct_text":  "你好",
	}
	_ = cosyVoiceConfig
	/**
		    "edge": {
	      "voice": "zh-CN-XiaoxiaoNeural",
	      "rate": "+0%",
	      "volume": "+0%",
	      "pitch": "+0Hz",
	      "connect_timeout": 10,
	      "receive_timeout": 60
	    }
	*/
	edgeConfig := map[string]interface{}{
		"voice":           "zh-CN-XiaoxiaoNeural",
		"rate":            "+0%",
		"volume":          "+0%",
		"pitch":           "+0Hz",
		"connect_timeout": 10,
		"receive_timeout": 60,
	}
	_ = edgeConfig
	//调用tts服务生成语音
	ttsProvider, err := tts.GetTTSProvider("cosyvoice", cosyVoiceConfig)
	if err != nil {
		return nil, fmt.Errorf("获取tts服务失败: %v", err)
	}

	ret := []AudioOpusData{}
	for _, text := range textList {
		audioData := AudioOpusData{}
		var audioChan chan []byte

		for i := 0; i < 3; i++ {
			audioChan, err = ttsProvider.TextToSpeechStream(context.Background(), text, SampleRate, 1, FrameDurationMs)
			if err != nil {
				fmt.Printf("生成语音失败: %v\n", err)
				continue
			}
			break
		}

		for perOpusData := range audioChan {
			audioData.OpusData = append(audioData.OpusData, perOpusData)
		}
		ret = append(ret, audioData)
	}

	return ret, nil
}

// 调用tts服务生成语音, 并编码至opus发送至服务端
func (w *WsClient) sendAudioDataToServer() error {
	for {
		audioOpusData := <-w.audioOpusDataChan
		w.sendListenStart()
		for _, opusData := range audioOpusData.OpusData {
			fmt.Printf("发送Opus帧: %d\n", len(opusData))
			if err := w.Conn.WriteMessage(websocket.BinaryMessage, opusData); err != nil {
				return fmt.Errorf("发送Opus帧失败: %v", err)
			}
			time.Sleep(FrameDurationMs * time.Millisecond)
		}
		w.sendListenStop()
	}

	return nil
}
