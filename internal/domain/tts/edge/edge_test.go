package edge

import (
	"context"
	"testing"
	"time"
)

func TestEdgeTTSProvider(t *testing.T) {

	config := map[string]interface{}{
		"voice":           "zh-CN-XiaoxiaoNeural",
		"rate":            "+0%",
		"volume":          "+0%",
		"pitch":           "+0Hz",
		"connect_timeout": 10,
		"receive_timeout": 60,
	}

	provider := NewEdgeTTSProvider(config)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	t.Run("TestTextToSpeech", func(t *testing.T) {
		frames, err := provider.TextToSpeech(ctx, "你好，EdgeTTS测试")
		if err != nil {
			t.Fatalf("TextToSpeech失败: %v", err)
		}
		if len(frames) == 0 {
			t.Error("未返回任何音频帧")
		}
	})

	t.Run("TestTextToSpeechStream", func(t *testing.T) {
		outputChan, err := provider.TextToSpeechStream(ctx, "你好，EdgeTTS流式测试")
		if err != nil {
			t.Fatalf("TextToSpeechStream失败: %v", err)
		}
		var receivedFrames [][]byte
		timeout := time.After(20 * time.Second)
	ReceiveLoop:
		for {
			select {
			case frame, ok := <-outputChan:
				if !ok {
					break ReceiveLoop
				}
				receivedFrames = append(receivedFrames, frame)
			case <-timeout:
				t.Error("接收音频帧超时")
				break ReceiveLoop
			}
		}
		if len(receivedFrames) == 0 {
			t.Error("未接收到任何音频帧")
		}
	})
}
