package edge_offline

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

// 模拟 TTS WebSocket 服务器
func mockTTSServer(t *testing.T) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 升级HTTP连接为WebSocket
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("升级WebSocket失败: %v", err)
			return
		}
		defer conn.Close()

		// 读取文本消息
		_, text, err := conn.ReadMessage()
		if err != nil {
			t.Errorf("读取文本消息失败: %v", err)
			return
		}

		// 模拟音频数据
		audioData := []byte("mock audio data for: " + string(text))

		// 发送二进制音频数据
		err = conn.WriteMessage(websocket.BinaryMessage, audioData)
		if err != nil {
			t.Errorf("发送音频数据失败: %v", err)
			return
		}

		// 正常关闭连接
		err = conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		if err != nil {
			t.Errorf("关闭WebSocket连接失败: %v", err)
			return
		}
	}))
}

func TestEdgeOfflineTTSProvider(t *testing.T) {
	// 启动模拟服务器
	server := mockTTSServer(t)
	defer server.Close()

	// 创建配置
	config := map[string]interface{}{
		"server_url": "ws" + server.URL[4:], // 将 http:// 替换为 ws://
		"timeout":    float64(5),            // 5秒超时
	}

	provider := NewEdgeOfflineTTSProvider(config)

	t.Run("TestTextToSpeech", func(t *testing.T) {
		ctx := context.Background()
		frames, err := provider.TextToSpeech(ctx, "测试文本", 16000, 1, 20)
		if err != nil {
			t.Fatalf("TextToSpeech失败: %v", err)
		}
		if len(frames) == 0 {
			t.Error("未返回任何音频帧")
		}
	})

	t.Run("TestTextToSpeechStream", func(t *testing.T) {
		ctx := context.Background()
		outputChan, err := provider.TextToSpeechStream(ctx, "测试文本", 16000, 1, 20)
		if err != nil {
			t.Fatalf("TextToSpeechStream失败: %v", err)
		}

		var receivedFrames [][]byte
		timeout := time.After(5 * time.Second)

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

	t.Run("TestInvalidServerURL", func(t *testing.T) {
		provider := NewEdgeOfflineTTSProvider(map[string]interface{}{
			"server_url": "ws://invalid-server:12345",
			"timeout":    float64(1),
		})

		ctx := context.Background()
		_, err := provider.TextToSpeech(ctx, "测试文本", 16000, 1, 20)
		if err == nil {
			t.Error("期望连接无效服务器时返回错误")
		}
	})

	t.Run("TestTimeout", func(t *testing.T) {
		// 创建一个会延迟响应的服务器
		slowServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				t.Errorf("升级WebSocket失败: %v", err)
				return
			}
			defer conn.Close()

			// 延迟2秒
			time.Sleep(2 * time.Second)
		}))
		defer slowServer.Close()

		provider := NewEdgeOfflineTTSProvider(map[string]interface{}{
			"server_url": "ws" + slowServer.URL[4:],
			"timeout":    float64(1), // 1秒超时
		})

		ctx := context.Background()
		_, err := provider.TextToSpeech(ctx, "测试文本", 16000, 1, 20)
		if err == nil {
			t.Error("期望超时时返回错误")
		}
	})
}
