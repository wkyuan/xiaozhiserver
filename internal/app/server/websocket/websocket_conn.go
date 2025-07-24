package websocket

import (
	"context"
	"encoding/binary"
	"errors"
	"sync"
	"time"
	"xiaozhi-esp32-server-golang/internal/app/server/types"
	log "xiaozhi-esp32-server-golang/logger"

	"github.com/gorilla/websocket"
)

// WebSocketConn 实现 types.IConn 接口，适配 WebSocket 连接
type WebSocketConn struct {
	ctx    context.Context
	cancel context.CancelFunc

	onCloseCbList []func(deviceId string)

	conn     *websocket.Conn
	deviceID string

	isMqttUdpBridge bool
	recvCmdChan     chan []byte
	recvAudioChan   chan []byte

	// 连接状态标记
	isClosed bool
	sync.RWMutex
}

// NewWebSocketConn 创建一个新的 WebSocketConn 实例
func NewWebSocketConn(conn *websocket.Conn, deviceID string, isMqttUdpBridge bool) *WebSocketConn {
	ctx, cancel := context.WithCancel(context.Background())
	instance := &WebSocketConn{
		ctx:             ctx,
		cancel:          cancel,
		conn:            conn,
		deviceID:        deviceID,
		isMqttUdpBridge: isMqttUdpBridge,
		recvCmdChan:     make(chan []byte, 100),
		recvAudioChan:   make(chan []byte, 100),
	}

	go func() {
		for {
			select {
			case <-instance.ctx.Done():
				return
			default:
				instance.conn.SetReadDeadline(time.Now().Add(120 * time.Second))
				msgType, audio, err := instance.conn.ReadMessage()
				if err != nil {
					log.Errorf("read message error: %v", err)
					for _, cb := range instance.onCloseCbList {
						cb(instance.deviceID) //通知注册方退出
					}
					return
				}

				if msgType == websocket.TextMessage {
					select {
					case instance.recvCmdChan <- audio:
					default:
						log.Errorf("recv cmd channel is full")
					}
				} else if msgType == websocket.BinaryMessage {
					if instance.isMqttUdpBridge {
						audio = instance.tryUnpackUdpBridgeAudioPacket(audio)
					}
					select {
					case instance.recvAudioChan <- audio:
					default:
						log.Errorf("recv audio channel is full")
					}
				}
			}
		}
	}()

	return instance
}

// 适配mqtt udp bridge的数据格式
// 前8个字节为0, 12-16字节为音频数据长度, 16字节后为音频数据
func (c *WebSocketConn) tryUnpackUdpBridgeAudioPacket(buffer []byte) []byte {
	if len(buffer) < 16 {
		return buffer
	}
	// 检查前8字节是否全为0
	for i := 0; i < 8; i++ {
		if buffer[i] != 0 {
			return buffer
		}
	}
	dataLen := binary.BigEndian.Uint32(buffer[12:16])
	if int(dataLen) != len(buffer)-16 {
		return buffer
	}
	audioData := buffer[16:]
	return audioData
}

func (w *WebSocketConn) SendCmd(msg []byte) error {
	w.Lock()
	defer w.Unlock()

	// 检查连接是否已关闭
	if w.isClosed {
		return errors.New("connection is closed")
	}

	err := w.conn.WriteMessage(websocket.TextMessage, msg)
	if err != nil {
		log.Errorf("send cmd error: %v", err)
		return err
	}
	return nil
}

func (w *WebSocketConn) SendAudio(audio []byte) error {
	w.Lock()
	defer w.Unlock()

	// 检查连接是否已关闭
	if w.isClosed {
		return errors.New("connection is closed")
	}

	err := w.conn.WriteMessage(websocket.BinaryMessage, audio)
	if err != nil {
		log.Errorf("send audio error: %v", err)
		return err
	}
	return nil
}

func (w *WebSocketConn) RecvCmd(timeout int) ([]byte, error) {
	for {
		select {
		case msg := <-w.recvCmdChan:
			return msg, nil
		case <-time.After(time.Duration(timeout) * time.Second):
			return nil, errors.New("timeout")
		}
	}
}

func (w *WebSocketConn) RecvAudio(timeout int) ([]byte, error) {
	for {
		select {
		case audio := <-w.recvAudioChan:
			return audio, nil
		case <-time.After(time.Duration(timeout) * time.Second):
			return nil, errors.New("timeout")
		}
	}
}

func (w *WebSocketConn) Close() error {
	w.Lock()
	defer w.Unlock()

	// 设置关闭标记
	if w.isClosed {
		return nil // 已经关闭，避免重复关闭
	}
	w.isClosed = true

	w.cancel()
	w.conn.Close()
	close(w.recvCmdChan)
	close(w.recvAudioChan)

	// 调用关闭回调
	for _, cb := range w.onCloseCbList {
		if cb != nil {
			cb(w.deviceID)
		}
	}

	return nil
}

func (w *WebSocketConn) OnClose(cb func(deviceId string)) {
	w.onCloseCbList = append(w.onCloseCbList, cb)
}

func (w *WebSocketConn) GetDeviceID() string {
	return w.deviceID
}

func (w *WebSocketConn) GetTransportType() string {
	return types.TransportTypeWebsocket
}

func (w *WebSocketConn) GetData(key string) (interface{}, error) {
	return nil, errors.New("not implemented")
}

func (w *WebSocketConn) CloseAudioChannel() error {
	return nil
}

// IsClosed 检查连接是否已关闭
func (w *WebSocketConn) IsClosed() bool {
	w.RLock()
	defer w.RUnlock()
	return w.isClosed
}
