package chat

import (
	"encoding/json"
	"fmt"
	"time"
	types_conn "xiaozhi-esp32-server-golang/internal/app/server/types"
	types_audio "xiaozhi-esp32-server-golang/internal/data/audio"
	. "xiaozhi-esp32-server-golang/internal/data/client"
	. "xiaozhi-esp32-server-golang/internal/data/msg"
	log "xiaozhi-esp32-server-golang/logger"
)

// ServerTransport handles sending messages to the client via the transport layer
// (原ServerMsgService)
type ServerTransport struct {
	transport      types_conn.IConn
	clientState    *ClientState
	McpRecvMsgChan chan []byte
}

func NewServerTransport(transport types_conn.IConn, clientState *ClientState) *ServerTransport {
	return &ServerTransport{
		transport:      transport,
		clientState:    clientState,
		McpRecvMsgChan: make(chan []byte, 100),
	}
}

func (s *ServerTransport) SendTtsStart() error {
	msg := ServerMessage{
		Type:      ServerMessageTypeTts,
		State:     MessageStateStart,
		SessionID: s.clientState.SessionID,
	}
	bytes, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	err = s.transport.SendCmd(bytes)
	if err != nil {
		return err
	}
	s.clientState.SetTtsStart(true)
	return nil
}

func (s *ServerTransport) SendTtsStop() error {
	msg := ServerMessage{
		Type:      ServerMessageTypeTts,
		State:     MessageStateStop,
		SessionID: s.clientState.SessionID,
	}
	bytes, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	err = s.transport.SendCmd(bytes)
	if err != nil {
		return err
	}
	return nil
}

func (s *ServerTransport) SendHello(transportType string, audioFormat *types_audio.AudioFormat, udpConfig *UdpConfig) error {
	msg := ServerMessage{
		Type:        MessageTypeHello,
		Text:        "欢迎使用小智服务器",
		SessionID:   s.clientState.SessionID,
		Transport:   transportType,
		AudioFormat: audioFormat,
		Udp:         udpConfig,
	}
	bytes, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	err = s.transport.SendCmd(bytes)
	if err != nil {
		return err
	}
	return nil
}

func (s *ServerTransport) SendIot(msg *ClientMessage) error {
	resp := ServerMessage{
		Type:      ServerMessageTypeIot,
		Text:      msg.Text,
		SessionID: s.clientState.SessionID,
		State:     MessageStateSuccess,
	}
	bytes, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	err = s.transport.SendCmd(bytes)
	if err != nil {
		return err
	}
	return nil
}

func (s *ServerTransport) SendAsrResult(text string) error {
	resp := ServerMessage{
		Type:      ServerMessageTypeStt,
		Text:      text,
		SessionID: s.clientState.SessionID,
	}
	bytes, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	err = s.transport.SendCmd(bytes)
	if err != nil {
		return err
	}
	return nil
}

func (s *ServerTransport) SendSentenceStart(text string) error {
	response := ServerMessage{
		Type:      ServerMessageTypeTts,
		State:     MessageStateSentenceStart,
		Text:      text,
		SessionID: s.clientState.SessionID,
	}
	bytes, err := json.Marshal(response)
	if err != nil {
		return err
	}
	err = s.transport.SendCmd(bytes)
	if err != nil {
		return err
	}
	s.clientState.SetStatus(ClientStatusTTSStart)
	return nil
}

func (s *ServerTransport) SendSentenceEnd(text string) error {
	response := ServerMessage{
		Type:      ServerMessageTypeTts,
		State:     MessageStateSentenceEnd,
		Text:      text,
		SessionID: s.clientState.SessionID,
	}
	bytes, err := json.Marshal(response)
	if err != nil {
		return err
	}
	err = s.transport.SendCmd(bytes)
	if err != nil {
		return err
	}
	s.clientState.SetStatus(ClientStatusTTSStart)
	return nil
}

func (s *ServerTransport) SendCmd(cmdBytes []byte) error {
	return s.transport.SendCmd(cmdBytes)
}

func (s *ServerTransport) SendAudio(audio []byte) error {
	return s.transport.SendAudio(audio)
}

func (s *ServerTransport) GetTransportType() string {
	return s.transport.GetTransportType()
}

func (s *ServerTransport) GetData(key string) (interface{}, error) {
	return s.transport.GetData(key)
}

func (s *ServerTransport) SendMcpMsg(payload []byte) error {
	response := ServerMessage{
		Type:      MessageTypeMcp,
		SessionID: s.clientState.SessionID,
		PayLoad:   payload,
	}
	bytes, err := json.Marshal(response)
	if err != nil {
		return err
	}
	err = s.transport.SendCmd(bytes)
	if err != nil {
		// 如果是连接关闭错误，不记录为错误日志
		if err.Error() == "connection is closed" {
			log.Debugf("跳过发送MCP消息，连接已关闭")
			return err
		}
		log.Errorf("发送MCP消息失败: %v", err)
		return err
	}
	return nil
}

func (s *ServerTransport) RecvMcpMsg(timeOut int) ([]byte, error) {
	select {
	case msg := <-s.McpRecvMsgChan:
		return msg, nil
	case <-time.After(time.Duration(timeOut) * time.Millisecond):
		return nil, fmt.Errorf("mcp 接收消息超时")
	}
}

func (s *ServerTransport) Close() error {
	return s.transport.Close()
}

func (s *ServerTransport) RecvAudio(timeOut int) ([]byte, error) {
	return s.transport.RecvAudio(timeOut)
}

func (s *ServerTransport) RecvCmd(timeOut int) ([]byte, error) {
	return s.transport.RecvCmd(timeOut)
}
