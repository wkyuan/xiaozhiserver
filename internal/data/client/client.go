package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"sync"

	"xiaozhi-esp32-server-golang/internal/domain/asr"
	utypes "xiaozhi-esp32-server-golang/internal/domain/config/types"
	"xiaozhi-esp32-server-golang/internal/domain/llm"
	llm_common "xiaozhi-esp32-server-golang/internal/domain/llm/common"
	"xiaozhi-esp32-server-golang/internal/domain/tts"

	. "xiaozhi-esp32-server-golang/internal/data/audio"

	log "xiaozhi-esp32-server-golang/logger"

	"github.com/cloudwego/eino/schema"
	"github.com/spf13/viper"
)

// Dialogue 表示对话历史
type Dialogue struct {
	Messages []schema.Message
}

const (
	ClientStatusInit       = "init"
	ClientStatusListening  = "listening"
	ClientStatusListenStop = "listenStop"
	ClientStatusLLMStart   = "llmStart"
	ClientStatusTTSStart   = "ttsStart"
)

type SendAudioData func(audioData []byte) error

// ClientState 表示客户端状态
type ClientState struct {
	// 对话历史
	Dialogue *Dialogue
	// 打断状态
	Abort bool
	// 拾音模式
	ListenMode string
	// 设备ID
	DeviceID string
	// 会话ID
	SessionID string

	//设备配置
	DeviceConfig utypes.UConfig

	Vad
	Asr
	Llm

	// TTS 提供者
	TTSProvider tts.TTSProvider

	// 上下文控制
	Ctx    context.Context
	Cancel context.CancelFunc

	//prompt, 系统提示词
	SystemPrompt string

	InputAudioFormat  AudioFormat //输入音频格式
	OutputAudioFormat AudioFormat //输出音频格式

	// opus接收的音频数据缓冲区
	OpusAudioBuffer chan []byte

	// pcm接收的音频数据缓冲区
	AsrAudioBuffer *AsrAudioBuffer

	VoiceStatus
	SessionCtx Ctx

	UdpSendAudioData SendAudioData //发送音频数据
	Statistic        Statistic     //耗时统计
	MqttLastActiveTs int64         //最后活跃时间
	VadLastActiveTs  int64         //vad最后活跃时间, 超过 60s && 没有在tts则断开连接

	Status string //状态 listening, llmStart, ttsStart

	IsTtsStart        bool //是否tts开始
	IsWelcomeSpeaking bool //是否已经欢迎语
}

func (c *ClientState) SetTtsStart(isStart bool) {
	c.IsTtsStart = isStart
}

func (c *ClientState) GetTtsStart() bool {
	return c.IsTtsStart
}

func (c *ClientState) GetMaxIdleDuration() int64 {
	maxIdleDuration := viper.GetInt64("chat.max_idle_duration")
	if maxIdleDuration == 0 {
		maxIdleDuration = 20000
	}
	return maxIdleDuration
}

func (c *ClientState) UpdateLastActiveTs() {
	c.MqttLastActiveTs = time.Now().Unix()
}

func (c *ClientState) IsActive() bool {
	diff := time.Now().Unix() - c.MqttLastActiveTs
	return c.MqttLastActiveTs > 0 && diff <= ClientActiveTs
}

func (c *ClientState) SetStatus(status string) {
	c.Status = status
}

func (c *ClientState) GetStatus() string {
	return c.Status
}

func (s *ClientState) ResetSessionCtx() {
	s.SessionCtx.Lock()
	defer s.SessionCtx.Unlock()
	if s.SessionCtx.Ctx == nil {
		s.SessionCtx.Ctx, s.SessionCtx.Cancel = context.WithCancel(s.Ctx)
	}
}

func (s *ClientState) CancelSessionCtx() {
	s.SessionCtx.Lock()
	defer s.SessionCtx.Unlock()
	if s.SessionCtx.Ctx != nil {
		s.SessionCtx.Cancel()
		s.SessionCtx.Ctx = nil
	}
}

func (s *ClientState) GetSessionCtx() context.Context {
	s.SessionCtx.Lock()
	defer s.SessionCtx.Unlock()
	if s.SessionCtx.Ctx == nil {
		s.SessionCtx.Ctx, s.SessionCtx.Cancel = context.WithCancel(s.Ctx)
	}
	return s.SessionCtx.Ctx
}

type Ctx struct {
	sync.RWMutex
	Ctx    context.Context
	Cancel context.CancelFunc
}

func (s *ClientState) getLLMProvider() (llm.LLMProvider, error) {
	llmConfig := s.DeviceConfig.Llm
	llmType, ok := llmConfig.Config["type"]
	if !ok {
		log.Errorf("getLLMProvider err: not found llm type: %+v", llmConfig)
		return nil, fmt.Errorf("llm config type not found")
	}
	llmProvider, err := llm.GetLLMProvider(llmType.(string), llmConfig.Config)
	if err != nil {
		return nil, fmt.Errorf("创建 LLM 提供者失败: %v", err)
	}
	return llmProvider, nil
}

func (s *ClientState) InitLlm() error {
	ctx, cancel := context.WithCancel(s.Ctx)

	llmProvider, err := s.getLLMProvider()
	if err != nil {
		log.Errorf("创建 LLM 提供者失败: %v", err)
		return err
	}

	s.Llm = Llm{
		Ctx:         ctx,
		Cancel:      cancel,
		LLMProvider: llmProvider,
	}
	return nil
}

func (s *ClientState) InitAsr() error {
	asrConfig := s.DeviceConfig.Asr
	//初始化asr
	asrProvider, err := asr.NewAsrProvider(asrConfig.Provider, asrConfig.Config)
	if err != nil {
		log.Errorf("创建asr提供者失败: %v", err)
		return fmt.Errorf("创建asr提供者失败: %v", err)
	}
	ctx, cancel := context.WithCancel(s.Ctx)
	s.Asr = Asr{
		Ctx:             ctx,
		Cancel:          cancel,
		AsrProvider:     asrProvider,
		AsrAudioChannel: make(chan []float32, 100),
		AsrEnd:          make(chan bool, 1),
		AsrResult:       bytes.Buffer{},
	}
	return nil
}

func (c *ClientState) Destroy() {
	c.Asr.Stop()
	c.Vad.Reset()

	c.VoiceStatus.Reset()
	c.AsrAudioBuffer.ClearAsrAudioData()

	c.ResetSessionCtx()
	c.Statistic.Reset()
	c.SetStatus(ClientStatusInit)
	c.SetTtsStart(false)
}

func (c *ClientState) SetAsrPcmFrameSize(sampleRate int, channels int, perFrameDuration int) {
	c.AsrAudioBuffer.PcmFrameSize = sampleRate * channels * perFrameDuration / 1000
}

func (state *ClientState) OnManualStop() {
	state.OnVoiceSilence()
}

func (state *ClientState) OnVoiceSilence() {
	state.SetClientVoiceStop(true) //设置停止说话标志位, 此时收到的音频数据不会进vad
	//客户端停止说话
	state.Asr.Stop() //停止asr并获取结果，进行llm
	//释放vad
	state.Vad.Reset() //释放vad实例
	//asr统计
	state.SetStartAsrTs() //进行asr统计

	state.SetStatus(ClientStatusListenStop)
}

type Llm struct {
	Ctx    context.Context
	Cancel context.CancelFunc
	// LLM 提供者
	LLMProvider llm.LLMProvider
	//asr to text接收的通道
	LLmRecvChannel chan llm_common.LLMResponseStruct
}

// ClientMessage 表示客户端消息
type ClientMessage struct {
	Type        string          `json:"type"`
	DeviceID    string          `json:"device_id,omitempty"`
	SessionID   string          `json:"session_id,omitempty"`
	Text        string          `json:"text,omitempty"`
	Mode        string          `json:"mode,omitempty"`
	State       string          `json:"state,omitempty"`
	Token       string          `json:"token,omitempty"`
	DeviceMac   string          `json:"device_mac,omitempty"`
	Version     int             `json:"version,omitempty"`
	Transport   string          `json:"transport,omitempty"`
	Features    map[string]bool `json:"features,omitempty"`
	AudioParams *AudioFormat    `json:"audio_params,omitempty"`
	PayLoad     json.RawMessage `json:"payload,omitempty"`
}
