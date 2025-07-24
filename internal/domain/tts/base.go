package tts

import (
	"context"
	"fmt"

	"xiaozhi-esp32-server-golang/constants"
	"xiaozhi-esp32-server-golang/internal/domain/tts/cosyvoice"
	"xiaozhi-esp32-server-golang/internal/domain/tts/doubao"
	"xiaozhi-esp32-server-golang/internal/domain/tts/edge"
	"xiaozhi-esp32-server-golang/internal/domain/tts/edge_offline"
	"xiaozhi-esp32-server-golang/internal/domain/tts/xiaozhi"
)

// 基础TTS提供者接口（不含Context方法）
type BaseTTSProvider interface {
	TextToSpeech(ctx context.Context, text string, sampleRate int, channels int, frameDuration int) ([][]byte, error)
	TextToSpeechStream(ctx context.Context, text string, sampleRate int, channels int, frameDuration int) (outputChan chan []byte, err error)
}

// 完整TTS提供者接口（包含Context方法）
type TTSProvider interface {
	BaseTTSProvider
}

// GetTTSProvider 获取一个完整的TTS提供者（支持Context）
func GetTTSProvider(providerName string, config map[string]interface{}) (TTSProvider, error) {
	var baseProvider BaseTTSProvider

	switch providerName {
	case constants.TtsTypeDoubao:
		baseProvider = doubao.NewDoubaoTTSProvider(config)
	case constants.TtsTypeDoubaoWS:
		baseProvider = doubao.NewDoubaoWSProvider(config)
	case constants.TtsTypeCosyvoice:
		baseProvider = cosyvoice.NewCosyVoiceTTSProvider(config)
	case constants.TtsTypeEdge:
		baseProvider = edge.NewEdgeTTSProvider(config)
	case constants.TtsTypeEdgeOffline:
		baseProvider = edge_offline.NewEdgeOfflineTTSProvider(config)
	case constants.TtsTypeXiaozhi:
		baseProvider = xiaozhi.NewXiaozhiProvider(config)
	default:
		return nil, fmt.Errorf("不支持的TTS提供者: %s", providerName)
	}

	// 使用适配器包装基础提供者，转换为完整的TTSProvider
	provider := &ContextTTSAdapter{baseProvider}
	return provider, nil
}

// ContextTTSAdapter 是一个适配器，为基础TTS提供者添加Context支持
type ContextTTSAdapter struct {
	Provider BaseTTSProvider
}

// TextToSpeech 代理到原始提供者
func (a *ContextTTSAdapter) TextToSpeech(ctx context.Context, text string, sampleRate int, channels int, frameDuration int) ([][]byte, error) {
	return a.Provider.TextToSpeech(ctx, text, sampleRate, channels, frameDuration)
}

// TextToSpeechStream 代理到原始提供者
func (a *ContextTTSAdapter) TextToSpeechStream(ctx context.Context, text string, sampleRate int, channels int, frameDuration int) (outputChan chan []byte, err error) {
	return a.Provider.TextToSpeechStream(ctx, text, sampleRate, channels, frameDuration)
}

// TextToSpeechWithContext 使用Context版本的文本转语音
func (a *ContextTTSAdapter) TextToSpeechWithContext(ctx context.Context, text string, sampleRate int, channels int, frameDuration int) ([][]byte, error) {
	// 检查提供者是否直接支持Context版本
	if provider, ok := a.Provider.(interface {
		TextToSpeechWithContext(ctx context.Context, text string, sampleRate int, channels int, frameDuration int) ([][]byte, error)
	}); ok {
		// 提供者直接支持Context版本
		return provider.TextToSpeechWithContext(ctx, text, sampleRate, channels, frameDuration)
	}

	// 否则使用标准版本，并通过goroutine和channel实现上下文控制
	resultChan := make(chan struct {
		frames [][]byte
		err    error
	})

	go func() {
		frames, err := a.Provider.TextToSpeech(ctx, text, sampleRate, channels, frameDuration)
		select {
		case <-ctx.Done():
			// 上下文已取消，不发送结果
			return
		case resultChan <- struct {
			frames [][]byte
			err    error
		}{frames, err}:
			// 结果已发送
		}
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case result := <-resultChan:
		return result.frames, result.err
	}
}

// TextToSpeechStreamWithContext 使用Context版本的流式文本转语音
func (a *ContextTTSAdapter) TextToSpeechStreamWithContext(ctx context.Context, text string, sampleRate int, channels int, frameDuration int) (outputChan chan []byte, cancelFunc func(), err error) {
	// 检查提供者是否直接支持Context版本
	if provider, ok := a.Provider.(interface {
		TextToSpeechStreamWithContext(ctx context.Context, text string, sampleRate int, channels int, frameDuration int) (chan []byte, func(), error)
	}); ok {
		// 提供者直接支持Context版本
		return provider.TextToSpeechStreamWithContext(ctx, text, sampleRate, channels, frameDuration)
	}

	// 否则使用标准版本，但创建一个包装器来处理上下文取消
	streamChan, err := a.Provider.TextToSpeechStream(ctx, text, sampleRate, channels, frameDuration)
	if err != nil {
		return nil, nil, err
	}

	// 创建一个新的输出通道，用于转发和处理取消
	outputChan = make(chan []byte, 10)

	// 创建一个goroutine来转发数据并监听上下文取消
	go func() {
		defer close(outputChan)

		for {
			select {
			case <-ctx.Done():
				// 上下文已取消，调用原始取消函数并退出
				cancelFunc()
				return
			case frame, ok := <-streamChan:
				if !ok {
					// 原始通道已关闭
					return
				}
				// 转发数据
				select {
				case <-ctx.Done():
					// 上下文已取消
					cancelFunc()
					return
				case outputChan <- frame:
					// 成功转发数据
				}
			}
		}
	}()

	return outputChan, cancelFunc, nil
}
