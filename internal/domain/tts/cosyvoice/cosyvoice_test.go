package cosyvoice

import (
	"os"
	"testing"
	"time"
)

func TestCosyVoiceTTS(t *testing.T) {
	// 跳过实际的网络请求测试，除非设置了环境变量
	if os.Getenv("RUN_COSYVOICE_TEST") != "1" {
		t.Skip("跳过CosyVoice API测试，设置环境变量RUN_COSYVOICE_TEST=1以启用")
	}

	config := map[string]interface{}{
		"api_url":        "https://cosyvoice.com/tts",
		"spk_id":         "OUeAo1mhq6IBExi",
		"frame_duration": float64(60),
		"target_sr":      float64(16000),
		"audio_format":   "mp3",
		"instruct_text":  "你好",
	}

	provider := NewCosyVoiceTTSProvider(config)

	// 测试文本转语音
	t.Run("TestTextToSpeech", func(t *testing.T) {
		frames, err := provider.TextToSpeech("你会说四川话吗")
		if err != nil {
			t.Fatalf("TextToSpeech失败: %v", err)
		}

		if len(frames) == 0 {
			t.Error("未返回任何音频帧")
		}
	})

	// 测试流式文本转语音
	t.Run("TestTextToSpeechStream", func(t *testing.T) {
		outputChan, cancel, err := provider.TextToSpeechStream("你会说四川话吗")
		if err != nil {
			t.Fatalf("TextToSpeechStream失败: %v", err)
		}

		defer cancel()

		// 接收所有帧
		var receivedFrames [][]byte
		timeout := time.After(10 * time.Second)

	receiveLoop:
		for {
			select {
			case frame, ok := <-outputChan:
				if !ok {
					break receiveLoop
				}
				receivedFrames = append(receivedFrames, frame)
			case <-timeout:
				t.Error("接收音频帧超时")
				break receiveLoop
			}
		}

		if len(receivedFrames) == 0 {
			t.Error("未接收到任何音频帧")
		}
	})
}
