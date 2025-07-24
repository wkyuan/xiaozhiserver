package chat

import (
	"encoding/json"

	. "xiaozhi-esp32-server-golang/internal/data/client"
	"xiaozhi-esp32-server-golang/internal/domain/mcp"
	log "xiaozhi-esp32-server-golang/logger"

	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/spf13/viper"
)

type McpTransport struct {
	Client          *ClientState
	ServerTransport *ServerTransport
}

func (c *McpTransport) SendMcpMsg(payload []byte) error {
	//如果是initialize请求，则注入vision
	var request transport.JSONRPCRequest
	err := json.Unmarshal(payload, &request)
	if err == nil {
		if request.Method == "initialize" {
			if origInitParams, ok := request.Params.(map[string]interface{}); ok {
				b, err := json.Marshal(origInitParams)
				if err != nil {
					return err
				}

				var initParams mcp.InitializeParams
				err = json.Unmarshal(b, &initParams)
				if err != nil {
					return err
				}
				initParams.Capabilities["vision"] = mcp.Vision{
					Url:   viper.GetString("vision.vision_url"),
					Token: "1234567890",
				}
				request.Params = initParams
			}
			payload, _ = json.Marshal(request)
		}
	}

	return c.ServerTransport.SendMcpMsg(payload)
}

func (c *McpTransport) RecvMcpMsg(timeOut int) ([]byte, error) {
	return c.ServerTransport.RecvMcpMsg(timeOut)
}

func initMcp(clientState *ClientState, serverTransport *ServerTransport) {
	mcpClientSession := mcp.GetDeviceMcpClient(clientState.DeviceID)
	if mcpClientSession == nil {
		mcpClientSession = mcp.NewDeviceMCPSession(clientState.DeviceID)
		mcp.AddDeviceMcpClient(clientState.DeviceID, mcpClientSession)
	}

	// 创建IotOverMcp客户端
	mcpTransport := &McpTransport{
		Client:          clientState,
		ServerTransport: serverTransport,
	}
	iotOverMcpClient := mcp.NewIotOverMcpClient(clientState.DeviceID, mcpTransport)
	if iotOverMcpClient == nil {
		log.Errorf("创建IotOverMcp客户端失败")
		serverTransport.transport.Close()
		return
	}
	mcpClientSession.SetIotOverMcp(iotOverMcpClient)
}
