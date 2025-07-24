package doubao

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	log "xiaozhi-esp32-server-golang/logger"

	"xiaozhi-esp32-server-golang/internal/domain/tts/common"

	"github.com/gorilla/websocket"
)

// 枚举消息类型
var (
	enumMessageType = map[byte]string{
		11: "audio-only server response",
		12: "frontend server response",
		15: "error message from server",
	}
	enumMessageTypeSpecificFlags = map[byte]string{
		0: "no sequence number",
		1: "sequence number > 0",
		2: "last message from server (seq < 0)",
		3: "sequence number < 0",
	}
	enumMessageSerializationMethods = map[byte]string{
		0:  "no serialization",
		1:  "JSON",
		15: "custom type",
	}
	enumMessageCompression = map[byte]string{
		0:  "no compression",
		1:  "gzip",
		15: "custom compression method",
	}
)

// 常量定义
const (
	optQuery   string = "query"  // 一次性合成
	optSubmit  string = "submit" // 流式合成
	wsScheme   string = "wss"
	wsPath     string = "/api/v1/tts/ws_binary"
	headerSize int    = 4 // 默认头部大小(字节)
)

// 默认二进制协议头
// version: b0001 (4 bits)
// header size: b0001 (4 bits)
// message type: b0001 (Full client request) (4bits)
// message type specific flags: b0000 (none) (4bits)
// message serialization method: b0001 (JSON) (4 bits)
// message compression: b0001 (gzip) (4bits)
// reserved data: 0x00 (1 byte)
var defaultHeader = []byte{0x11, 0x10, 0x11, 0x00}

// 全局WebSocket客户端，实现连接池
var (
	wsClientMap     = make(map[string]*WSConnWrapper)
	wsClientMapLock sync.Mutex
	// 全局WebSocket Dialer，配置更大的缓冲区以避免slice bounds out of range错误
	wsDialer = websocket.Dialer{
		ReadBufferSize:   16384, // 16KB 读取缓冲区
		WriteBufferSize:  16384, // 16KB 写入缓冲区
		HandshakeTimeout: 45 * time.Second,
	}
)

// WSConnWrapper WebSocket连接包装器，带有最后活跃时间
type WSConnWrapper struct {
	Conn         *websocket.Conn
	LastActiveAt time.Time
}

// 合成响应结构
type synResp struct {
	Audio  []byte
	IsLast bool
}

// DoubaoWSProvider 读伴WebSocket TTS提供者
type DoubaoWSProvider struct {
	AppID       string
	AccessToken string
	Cluster     string
	Voice       string
	WSHost      string
	WSURL       *url.URL
	Header      http.Header
	UseStream   bool // 是否使用流式合成
	// 音频片段处理回调函数，仅在流式模式下使用
	OnAudioChunk func(chunkData []byte, isLast bool) error
}

// NewDoubaoWSProvider 创建新的读伴WebSocket TTS提供者
func NewDoubaoWSProvider(config map[string]interface{}) *DoubaoWSProvider {
	appID, _ := config["appid"].(string)
	accessToken, _ := config["access_token"].(string)
	cluster, _ := config["cluster"].(string)
	voice, _ := config["voice"].(string)
	wsHost, _ := config["ws_host"].(string)
	useStream, _ := config["use_stream"].(bool)

	// 如果没有指定WebSocket主机，使用默认值
	if wsHost == "" {
		wsHost = "openspeech.bytedance.com"
	}

	// 构建WebSocket URL
	wsURL := url.URL{Scheme: wsScheme, Host: wsHost, Path: wsPath}

	// 检查令牌
	if accessToken == "" {
		log.Error("TTS WebSocket 访问令牌不能为空")
	}

	// 创建HTTP头
	header := http.Header{}
	header.Add("Authorization", fmt.Sprintf("Bearer;%s", accessToken))

	return &DoubaoWSProvider{
		AppID:       appID,
		AccessToken: accessToken,
		Cluster:     cluster,
		Voice:       voice,
		WSHost:      wsHost,
		WSURL:       &wsURL,
		Header:      header,
		UseStream:   useStream,
	}
}

func (p *DoubaoWSProvider) TextToSpeech(ctx context.Context, text string, sampleRate int, channels int, frameDuration int) ([][]byte, error) {
	return nil, nil
}

// TextToSpeech 将文本转换为语音，返回音频帧数据和错误
func (p *DoubaoWSProvider) TextToSpeechStream(ctx context.Context, text string, sampleRate int, channels int, frameDuration int) (outputOpusChan chan []byte, err error) {
	var operation string
	if p.UseStream {
		operation = optSubmit // 流式合成
	} else {
		operation = optQuery // 一次性合成
	}

	startTs := time.Now().UnixMilli()

	// 准备请求数据
	input := p.setupInput(text, p.Voice, operation, sampleRate, channels, frameDuration)

	// 获取或创建WebSocket连接
	conn, err := p.getWSConnection()
	if err != nil {
		return nil, fmt.Errorf("获取WebSocket连接失败: %v", err)
	}

	// 压缩输入
	compressedInput := gzipCompress(input)
	payloadSize := len(compressedInput)

	// 准备payload大小数据
	payloadArr := make([]byte, 4)
	binary.BigEndian.PutUint32(payloadArr, uint32(payloadSize))

	// 组装完整的请求
	clientRequest := make([]byte, len(defaultHeader))
	copy(clientRequest, defaultHeader)
	clientRequest = append(clientRequest, payloadArr...)
	clientRequest = append(clientRequest, compressedInput...)

	// 发送请求
	err = conn.WriteMessage(websocket.BinaryMessage, clientRequest)
	if err != nil {
		// 连接可能已关闭，移除并重试
		p.removeWSConnection()
		return nil, fmt.Errorf("发送WebSocket消息失败: %v", err)
	}

	// 设置读取超时
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))

	pipeReader, pipeWriter := io.Pipe()

	outputOpusChan = make(chan []byte, 1000)

	go func() {
		mp3Decoder, err := common.CreateAudioDecoder(ctx, pipeReader, outputOpusChan, frameDuration, "mp3")
		if err != nil {
			log.Errorf("创建MP3解码器失败: %v", err)
			close(outputOpusChan)
			return
		}
		err = mp3Decoder.Run(startTs)
		if err != nil {
			log.Errorf("MP3解码器运行失败: %v", err)
			return
		}
	}()
	go func() {
		defer func() {
			pipeReader.Close()
			pipeWriter.Close()
		}()
		// 流式合成
		chunkCount := 0
		//var allAudio []byte
		for {
			select {
			case <-ctx.Done():
				log.Debugf("DoubaoWs TextToSpeechStream context done, exit")
				return
			default:
				_, message, err := conn.ReadMessage()
				if err != nil {
					p.removeWSConnection()
					log.Errorf("读取WebSocket消息失败: %v", err)

					return
				}

				resp, err := parseResponse(message)
				if err != nil {
					p.removeWSConnection()
					log.Errorf("解析响应失败: %v", err)
					return
				}

				if len(resp.Audio) > 0 {
					chunkCount++
					// 存储用于最终返回
					//allAudio = append(allAudio, resp.Audio...)
					pipeWriter.Write(resp.Audio)
				}

				if resp.IsLast {
					log.Debugf("收到最后一个音频片段，共%d个片段", chunkCount)
					//将allAudio写到文件中
					//saveAudioToTmp(allAudio, "mp3")
					return
				}

				// 重置读取超时
				conn.SetReadDeadline(time.Now().Add(5 * time.Second))
			}
		}
	}()

	return outputOpusChan, nil
}

// GetVoiceInfo 获取语音信息
func (p *DoubaoWSProvider) GetVoiceInfo() map[string]interface{} {
	return map[string]interface{}{
		"voice": p.Voice,
		"type":  "doubao_ws",
	}
}

// 获取WebSocket连接(连接池实现)
func (p *DoubaoWSProvider) getWSConnection() (*websocket.Conn, error) {
	wsClientMapLock.Lock()
	defer wsClientMapLock.Unlock()

	connKey := fmt.Sprintf("%s_%s", p.WSHost, p.AccessToken)
	timeoutDuration := 30 * time.Second

	// 检查是否已有连接
	if wrapper, ok := wsClientMap[connKey]; ok {
		// 验证连接是否有效
		err := wrapper.Conn.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(1*time.Second))

		// 检查连接是否超时
		isTimeout := time.Since(wrapper.LastActiveAt) > timeoutDuration

		if err == nil && !isTimeout {
			// 更新最后活跃时间
			wrapper.LastActiveAt = time.Now()
			log.Debugf("doubao websocket复用现有连接")
			return wrapper.Conn, nil
		}

		// 连接已失效或超时，关闭并移除
		reason := "失效"
		if isTimeout {
			reason = "超时(30秒)"
		}
		log.Warnf("WebSocket连接已%s，正在关闭并创建新连接", reason)
		wrapper.Conn.Close()
		delete(wsClientMap, connKey)
	}

	// 使用全局配置的Dialer创建新连接
	conn, _, err := wsDialer.Dial(p.WSURL.String(), p.Header)
	if err != nil {
		return nil, err
	}

	// 设置消息读取限制，防止过大的消息
	conn.SetReadLimit(1024 * 1024) // 1MB 最大消息大小

	// 设置保持连接
	conn.SetPingHandler(func(appData string) error {
		return conn.WriteControl(websocket.PongMessage, []byte(appData), time.Now().Add(1*time.Second))
	})

	// 将连接添加到池中
	wsClientMap[connKey] = &WSConnWrapper{
		Conn:         conn,
		LastActiveAt: time.Now(),
	}

	log.Debugf("创建新的doubao WebSocket连接，使用全局Dialer配置(读取缓冲区: 16KB，最大消息: 1MB)")
	return conn, nil
}

// 从连接池中移除连接
func (p *DoubaoWSProvider) removeWSConnection() {
	wsClientMapLock.Lock()
	defer wsClientMapLock.Unlock()

	connKey := fmt.Sprintf("%s_%s", p.WSHost, p.AccessToken)

	if wrapper, ok := wsClientMap[connKey]; ok {
		wrapper.Conn.Close()
		delete(wsClientMap, connKey)
	}
}

// 设置请求输入
func (p *DoubaoWSProvider) setupInput(text, voiceType, opt string, sampleRate int, channels int, frameDuration int) []byte {
	// 生成请求ID
	reqID := generateUUID()

	// 构建请求参数
	params := make(map[string]map[string]interface{})

	// 应用信息
	params["app"] = make(map[string]interface{})
	params["app"]["appid"] = p.AppID
	params["app"]["token"] = p.AccessToken
	params["app"]["cluster"] = p.Cluster

	// 用户信息
	params["user"] = make(map[string]interface{})
	params["user"]["uid"] = "uid"

	// 音频参数
	params["audio"] = make(map[string]interface{})
	params["audio"]["voice_type"] = voiceType
	params["audio"]["encoding"] = "mp3"
	params["audio"]["rate"] = sampleRate
	params["audio"]["speed_ratio"] = 1.0
	params["audio"]["volume_ratio"] = 1.0
	params["audio"]["pitch_ratio"] = 1.0

	// 请求信息
	params["request"] = make(map[string]interface{})
	params["request"]["reqid"] = reqID
	params["request"]["text"] = text
	params["request"]["text_type"] = "plain"
	params["request"]["operation"] = opt

	// 序列化为JSON
	resBytes, _ := json.Marshal(params)
	return resBytes
}

// gzip压缩
func gzipCompress(input []byte) []byte {
	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	w.Write(input)
	w.Close()
	return b.Bytes()
}

// gzip解压缩
func gzipDecompress(input []byte) []byte {
	b := bytes.NewBuffer(input)
	r, _ := gzip.NewReader(b)
	out, _ := io.ReadAll(r)
	r.Close()
	return out
}

// 解析WebSocket响应
func parseResponse(res []byte) (resp synResp, err error) {
	// 解析头部
	protoVersion := res[0] >> 4
	headSizeByte := res[0] & 0x0f
	messageType := res[1] >> 4
	messageTypeSpecificFlags := res[1] & 0x0f
	serializationMethod := res[2] >> 4
	messageCompression := res[2] & 0x0f

	_ = protoVersion
	_ = serializationMethod
	/*
		log.Debugf("协议版本: %x - 版本 %d\n", protoVersion, protoVersion)
		log.Debugf("头部大小: %x - %d 字节\n", headSizeByte, headSizeByte*4)
		log.Debugf("消息类型: %x - %s\n", messageType, enumMessageType[messageType])
		log.Debugf("消息类型特定标志: %x - %s\n", messageTypeSpecificFlags, enumMessageTypeSpecificFlags[messageTypeSpecificFlags])
		log.Debugf("消息序列化方法: %x - %s\n", serializationMethod, enumMessageSerializationMethods[serializationMethod])
		log.Debugf("消息压缩: %x - %s\n", messageCompression, enumMessageCompression[messageCompression])
	*/

	// 计算头部大小（以字节为单位）
	headSizeBytes := int(headSizeByte) * 4

	// 分离payload
	payload := res[headSizeBytes:]

	// 根据消息类型处理
	if messageType == 0xb { // audio-only server response (11)
		// 检查是否有序列号
		if messageTypeSpecificFlags == 0 {
			// 无序列号，空payload
			return
		} else {
			// 有序列号，提取payload
			sequenceNumber := int32(binary.BigEndian.Uint32(payload[0:4]))
			payloadSize := int32(binary.BigEndian.Uint32(payload[4:8]))
			payload = payload[8:]

			_ = payloadSize
			//log.Debugf("序列号: %d", sequenceNumber)
			//log.Debugf("Payload大小: %d", payloadSize)

			resp.Audio = append(resp.Audio, payload...)

			// 检查是否为最后一个包
			if sequenceNumber < 0 {
				resp.IsLast = true
			}
		}
	} else if messageType == 0xf { // error message (15)
		// 解析错误信息
		code := int32(binary.BigEndian.Uint32(payload[0:4]))
		errMsg := payload[8:] // 错误消息从第8个字节开始

		// 如果是压缩的，解压缩
		if messageCompression == 1 {
			errMsg = gzipDecompress(errMsg)
		}

		log.Errorf("服务端错误 (代码: %d): %s", code, string(errMsg))
		err = fmt.Errorf("服务端错误 (代码: %d): %s", code, string(errMsg))
		return
	} else if messageType == 0xc { // frontend server response (12)
		// 解析前端消息
		msgSize := int32(binary.BigEndian.Uint32(payload[0:4]))
		payload = payload[4:]

		// 如果是压缩的，解压缩
		if messageCompression == 1 {
			payload = gzipDecompress(payload)
		}

		// 记录前端消息
		if os.Getenv("DEBUG") == "1" {
			log.Debugf("前端消息大小: %d", msgSize)
			log.Debugf("前端消息内容: %s", string(payload))
		}
	} else {
		// 未知消息类型
		log.Warnf("未知消息类型: %d", messageType)
		err = fmt.Errorf("未知消息类型: %d", messageType)
		return
	}

	return
}

// saveAudioToTmp 将音频数据保存到tmp目录
func saveAudioToTmp(audioData []byte, format string) error {
	// 确保tmp目录存在
	tmpDir := "tmp"
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		return fmt.Errorf("创建tmp目录失败: %v", err)
	}

	// 生成唯一文件名
	timestamp := time.Now().Format("20060102_150405")
	uuid := generateUUID()
	filename := filepath.Join(tmpDir, fmt.Sprintf("audio_%s_%s.%s", timestamp, uuid[:8], format))

	// 写入文件
	if err := os.WriteFile(filename, audioData, 0644); err != nil {
		return fmt.Errorf("写入音频文件失败: %v", err)
	}

	log.Debugf("音频文件已保存: %s", filename)
	return nil
}
