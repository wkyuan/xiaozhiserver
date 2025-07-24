package silero_vad

import (
	"errors"
	"fmt"
	"sync"
	log "xiaozhi-esp32-server-golang/logger"

	. "xiaozhi-esp32-server-golang/internal/domain/vad/inter"

	"github.com/streamer45/silero-vad-go/speech"
)

// VAD默认配置
var defaultVADConfig = map[string]interface{}{
	"threshold":               0.5,
	"min_silence_duration_ms": int64(100),
	"sample_rate":             16000,
	"channels":                1,
	"speech_pad_ms":           60,
}

// 资源池默认配置
var defaultPoolConfig = struct {
	// 池大小
	MaxSize int
	// 获取超时时间（毫秒）
	AcquireTimeout int64
}{
	MaxSize:        10,
	AcquireTimeout: 3000, // 3秒
}

// 全局变量和初始化
var (
	// 全局解码器实例池
	opusDecoderMap sync.Map
	// 全局VAD检测器实例池
	vadDetectorMap sync.Map
	// 全局初始化锁
	initMutex sync.Mutex
	// 初始化标志
	initialized = false
	// 全局VAD资源池实例
	globalVADResourcePool *VADResourcePool

	once sync.Once
)

// InitVADFromConfig 从配置文件初始化VAD模块
func InitVADFromConfig(config map[string]interface{}) error {
	var modelPath string
	// 从viper获取模型路径
	if rawModelPath, ok := config["model_path"]; ok {
		tModelPath, ok := rawModelPath.(string)
		if ok {
			modelPath = tModelPath
		}
	}

	// 获取其他可选配置
	if rawThreshold, ok := config["threshold"]; ok {
		threshold, ok := rawThreshold.(float64)
		if ok && threshold > 0 {
			globalVADResourcePool.defaultConfig["threshold"] = threshold
		}
	}

	if rawSilenceMs, ok := config["min_silence_duration_ms"]; ok {
		silenceMs, ok := rawSilenceMs.(int64)
		if ok && silenceMs > 0 {
			globalVADResourcePool.defaultConfig["min_silence_duration_ms"] = silenceMs
		}
	}

	if rawSampleRate, ok := config["sample_rate"]; ok {
		sampleRate, ok := rawSampleRate.(int)
		if ok && sampleRate > 0 {
			globalVADResourcePool.defaultConfig["sample_rate"] = sampleRate
		}
	}

	if rawChannels, ok := config["channels"]; ok {
		channels, ok := rawChannels.(int)
		if ok && channels > 0 {
			globalVADResourcePool.defaultConfig["channels"] = channels
		}
	}

	// VAD资源池特有配置
	if rawPoolSize, ok := config["pool_size"]; ok {
		poolSize, ok := rawPoolSize.(int)
		if ok && poolSize > 0 {
			globalVADResourcePool.maxSize = poolSize
		}
	}

	if rawTimeout, ok := config["acquire_timeout_ms"]; ok {
		timeout, ok := rawTimeout.(int64)
		if ok && timeout > 0 {
			globalVADResourcePool.acquireTimeout = timeout
		}
	}

	// 设置模型路径并完成初始化
	return initVADResourcePool(modelPath)
}

// 内部方法：初始化VAD资源池
func initVADResourcePool(modelPath string) error {
	if modelPath == "" {
		return errors.New("模型路径不能为空")
	}

	initMutex.Lock()
	defer initMutex.Unlock()

	// 已经初始化过，检查模型路径是否变更
	if globalVADResourcePool.initialized {
		currentPath, ok := globalVADResourcePool.defaultConfig["model_path"].(string)
		if ok && currentPath == modelPath {
			return nil // 模型路径未变，无需重复初始化
		}
		log.Infof("VAD资源池模型路径变更，重新初始化: %s", modelPath)
	}

	// 设置模型路径
	globalVADResourcePool.defaultConfig["model_path"] = modelPath

	// 初始化资源池
	err := globalVADResourcePool.initialize()
	if err != nil {
		return fmt.Errorf("初始化VAD资源池失败: %v", err)
	}

	globalVADResourcePool.initialized = true
	log.Infof("VAD资源池初始化完成，模型路径: %s，池大小: %d", modelPath, globalVADResourcePool.maxSize)
	return nil
}

// SileroVAD Silero VAD模型实现
type SileroVAD struct {
	detector         *speech.Detector
	vadThreshold     float32
	silenceThreshold int64 // 单位:毫秒
	sampleRate       int   // 采样率
	channels         int   // 通道数
	mu               sync.Mutex
}

// NewSileroVAD 创建SileroVAD实例
func NewSileroVAD(config map[string]interface{}) (*SileroVAD, error) {
	threshold, ok := config["threshold"].(float64)
	if !ok {
		threshold = 0.5 // 默认阈值
	}

	silenceMs, ok := config["min_silence_duration_ms"].(int64)
	if !ok {
		silenceMs = 800 // 默认500毫秒
	}

	sampleRate, ok := config["sample_rate"].(int)
	if !ok {
		sampleRate = 16000 // 默认采样率
	}

	channels, ok := config["channels"].(int)
	if !ok {
		channels = 1 // 默认单声道
	}

	speechPadMs, ok := config["speech_pad_ms"].(int)
	if !ok {
		speechPadMs = 30 // 默认语音前后填充
	}

	modelPath, ok := config["model_path"].(string)
	if !ok {
		return nil, errors.New("缺少模型路径配置")
	}

	// 创建语音检测器
	detector, err := speech.NewDetector(speech.DetectorConfig{
		ModelPath:            modelPath,
		SampleRate:           sampleRate,
		Threshold:            float32(threshold),
		MinSilenceDurationMs: int(silenceMs),
		SpeechPadMs:          speechPadMs,
		LogLevel:             speech.LogLevelWarn,
	})
	if err != nil {
		return nil, err
	}

	return &SileroVAD{
		detector:         detector,
		vadThreshold:     float32(threshold),
		silenceThreshold: silenceMs,
		sampleRate:       sampleRate,
		channels:         channels,
	}, nil
}

func (s *SileroVAD) IsVADExt(pcmData []float32, sampleRate int, frameSize int) (bool, error) {
	return s.IsVAD(pcmData)
}

// IsVAD 实现VAD接口的IsVAD方法
func (s *SileroVAD) IsVAD(pcmData []float32) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	segments, err := s.detector.Detect(pcmData)
	if err != nil {
		log.Errorf("检测失败: %s", err)
		return false, err
	}

	for _, s := range segments {
		log.Debugf("speech starts at %0.2fs", s.SpeechStartAt)
		if s.SpeechEndAt > 0 {
			log.Debugf("speech ends at %0.2fs", s.SpeechEndAt)
		}
	}

	return len(segments) > 0, nil
}

// Close 关闭并释放资源
func (s *SileroVAD) Close() error {
	if s.detector != nil {
		return s.detector.Destroy()
	}
	return nil
}

// createVADInstance 创建指定类型的VAD实例（内部实现）
func createVADInstance(config map[string]interface{}) (VAD, error) {
	return NewSileroVAD(config)
}

// CreateVAD 创建指定类型的VAD实例（公共API）
func CreateVAD(config map[string]interface{}) (VAD, error) {
	return createVADInstance(config)
}

func InitVadPool(config map[string]interface{}) {
	once.Do(func() {
		globalVADResourcePool = &VADResourcePool{
			maxSize:        defaultPoolConfig.MaxSize,
			acquireTimeout: defaultPoolConfig.AcquireTimeout,
			defaultConfig:  defaultVADConfig,
			initialized:    false, // 标记为未完全初始化，需要后续读取配置
		}
		InitVADFromConfig(config)
	})

}

// AcquireVAD 获取一个VAD实例
func AcquireVAD(config map[string]interface{}) (VAD, error) {
	if globalVADResourcePool == nil || !globalVADResourcePool.initialized {
		return nil, errors.New("VAD资源池尚未初始化")
	}

	return globalVADResourcePool.AcquireVAD()
}

// ReleaseVAD 释放一个VAD实例
func ReleaseVAD(vad VAD) error {
	if globalVADResourcePool != nil && globalVADResourcePool.initialized {
		globalVADResourcePool.ReleaseVAD(vad)
	}
	return nil
}

// Reset 重置VAD检测器状态
func (s *SileroVAD) Reset() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.detector.Reset()
}

// SetThreshold 设置VAD检测阈值
func (s *SileroVAD) SetThreshold(threshold float32) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.vadThreshold = threshold
	// 注意：silero-vad-go 库的 detector 没有直接提供 SetThreshold 方法
	// 只能修改实例的阈值，在下次检测时生效
}
