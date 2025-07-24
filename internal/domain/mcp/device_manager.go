package mcp

import (
	"context"
	"fmt"
	"time"

	"xiaozhi-esp32-server-golang/logger"

	"github.com/cloudwego/eino/components/tool"
	"github.com/gorilla/websocket"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	cmap "github.com/orcaman/concurrent-map/v2"
)

type McpClientPool struct {
	device2McpClient cmap.ConcurrentMap[string, *DeviceMcpSession]
}

var mcpClientPool *McpClientPool

func init() {
	mcpClientPool = &McpClientPool{
		device2McpClient: cmap.New[*DeviceMcpSession](),
	}
	go mcpClientPool.checkOffline()
}

func (p *McpClientPool) GetMcpClient(deviceID string) *DeviceMcpSession {
	client, ok := p.device2McpClient.Get(deviceID)
	if !ok {
		return nil
	}
	return client
}

func (p *McpClientPool) RemoveMcpClient(deviceID string) {
	if client, ok := p.device2McpClient.Get(deviceID); ok {
		// 取消上下文，停止相关的goroutine
		if client.cancel != nil {
			client.cancel()
		}
		logger.Infof("设备 %s MCP客户端已移除并取消上下文", deviceID)
	}
	p.device2McpClient.Remove(deviceID)
}

func (p *McpClientPool) AddMcpClient(deviceID string, client *DeviceMcpSession) {
	p.device2McpClient.Set(deviceID, client)
}

func (p *McpClientPool) GetToolByDeviceId(deviceId string, toolsName string) (tool.InvokableTool, bool) {
	client := p.GetMcpClient(deviceId)
	if client == nil {
		return nil, false
	}
	return client.GetToolByName(toolsName)
}

func (p *McpClientPool) GetAllToolsByDeviceId(deviceId string) (map[string]tool.InvokableTool, error) {
	client := p.GetMcpClient(deviceId)
	if client == nil {
		return nil, fmt.Errorf("client not found")
	}
	return client.GetTools(), nil
}

func (p *McpClientPool) checkOffline() {
	ticker := time.NewTicker(30 * time.Second) // 每30秒检查一次离线状态
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			for _, client := range p.device2McpClient.Items() {
				// 检查空指针
				if client.wsEndPointMcp != nil && time.Since(client.wsEndPointMcp.lastPing) > 2*time.Minute {
					client.wsEndPointMcp.connected = false
					client.wsEndPointMcp.cancel()
				}
				if client.iotOverMcp != nil && time.Since(client.iotOverMcp.lastPing) > 2*time.Minute {
					client.iotOverMcp.connected = false
					client.iotOverMcp.cancel()
				}

				// 检查是否两个连接都已断开
				wsDisconnected := client.wsEndPointMcp == nil || !client.wsEndPointMcp.connected
				iotDisconnected := client.iotOverMcp == nil || !client.iotOverMcp.connected

				if wsDisconnected && iotDisconnected {
					logger.Infof("设备 %s 所有MCP连接都已断开，从池中移除", client.deviceID)
					p.RemoveMcpClient(client.deviceID)
				}
			}
		}
	}
}

// DeviceMcpSession 代表一个设备的MCP会话，聚合了多种MCP连接
type DeviceMcpSession struct {
	deviceID      string
	Ctx           context.Context
	cancel        context.CancelFunc
	wsEndPointMcp *McpClientInstance
	iotOverMcp    *McpClientInstance
}

func (dcs *DeviceMcpSession) SetWsEndPointMcp(mcpClient *McpClientInstance) {
	dcs.wsEndPointMcp = mcpClient
	dcs.refreshToolsAndPing()
}

func (dcs *DeviceMcpSession) SetIotOverMcp(mcpClient *McpClientInstance) {
	dcs.iotOverMcp = mcpClient
	dcs.refreshToolsAndPing()
}

// McpClientInstance 代表一个具体的MCP客户端连接
type McpClientInstance struct {
	serverName string
	mcpClient  *client.Client // 是从ws endpoint连上来的mcp server
	tools      map[string]tool.InvokableTool
	serverInfo *mcp.InitializeResult
	lastPing   time.Time
	ctx        context.Context
	cancel     context.CancelFunc
	connected  bool
	conn       ConnInterface
}

// NewDeviceMCPClient 创建新的MCP客户端
func NewDeviceMCPSession(deviceID string) *DeviceMcpSession {
	ctx, cancel := context.WithCancel(context.Background())

	deviceMcpClient := &DeviceMcpSession{
		deviceID: deviceID,
		Ctx:      ctx,
		cancel:   cancel,
	}

	go deviceMcpClient.refreshToolsAndPing()

	return deviceMcpClient
}

func NewWsEndPointMcpClient(ctx context.Context, deviceID string, conn *websocket.Conn) *McpClientInstance {
	ctx, cancel := context.WithCancel(ctx)

	wsTransport, err := NewWebsocketTransport(conn)
	if err != nil {
		logger.Errorf("创建MCP客户端失败: %v", err)
		return nil
	}
	mcpClient := client.NewClient(wsTransport)

	wsEndPointMcp := &McpClientInstance{
		serverName: fmt.Sprintf("ws_endpoint_mcp_%s", deviceID),
		mcpClient:  mcpClient,
		tools:      make(map[string]tool.InvokableTool),
		ctx:        ctx,
		cancel:     cancel,
		connected:  true,
		lastPing:   time.Now(),
	}
	wsTransport.SetNotificationHandler(wsEndPointMcp.handleJSONRPCNotification)

	wsEndPointMcp.sendInitlize(ctx)
	wsEndPointMcp.mcpClient.Start(ctx)
	return wsEndPointMcp
}

func NewIotOverMcpClient(deviceID string, conn ConnInterface) *McpClientInstance {
	ctx, cancel := context.WithCancel(context.Background())

	wsTransport, err := NewIotOverMcpTransport(conn)
	if err != nil {
		logger.Errorf("创建MCP客户端失败: %v", err)
		return nil
	}
	mcpClient := client.NewClient(wsTransport)

	iotOverMcp := &McpClientInstance{
		serverName: fmt.Sprintf("iot_over_mcp_%s", deviceID),
		mcpClient:  mcpClient,
		tools:      make(map[string]tool.InvokableTool),
		ctx:        ctx,
		cancel:     cancel,
		connected:  true,
		lastPing:   time.Now(),
	}
	wsTransport.SetNotificationHandler(iotOverMcp.handleJSONRPCNotification)
	iotOverMcp.sendInitlize(ctx)
	iotOverMcp.mcpClient.Start(ctx)

	return iotOverMcp
}

func (dc *DeviceMcpSession) refreshToolsAndPing() {
	tick := time.NewTicker(60 * time.Second)
	defer tick.Stop()

	pingTick := time.NewTicker(30 * time.Second)
	defer pingTick.Stop()

	findTools := func(mcpInstance *McpClientInstance) {
		if mcpInstance == nil {
			return
		}
		// 检查上下文是否已取消
		select {
		case <-mcpInstance.ctx.Done():
			return
		default:
		}

		tools, err := mcpInstance.mcpClient.ListTools(mcpInstance.ctx, mcp.ListToolsRequest{})
		if err != nil {
			logger.Errorf("获取工具列表失败: %v", err)
			return
		}
		mcpInstance.tools = ConvertMcpToolListToInvokableToolList(tools.Tools, mcpInstance.serverName, mcpInstance.mcpClient)
		logger.Infof("设备 %s 获取工具列表成功: %v", mcpInstance.serverName, mcpInstance.tools)
	}

	ping := func(mcpInstance *McpClientInstance) {
		if mcpInstance == nil {
			return
		}
		// 检查上下文是否已取消
		select {
		case <-mcpInstance.ctx.Done():
			return
		default:
		}

		err := mcpInstance.mcpClient.Ping(mcpInstance.ctx)
		if err == nil {
			mcpInstance.lastPing = time.Now()
		} else {
			logger.Debugf("设备 %s ping失败: %v", mcpInstance.serverName, err)
		}
	}

	findTools(dc.wsEndPointMcp)
	findTools(dc.iotOverMcp)
	for {
		select {
		case <-dc.Ctx.Done():
			logger.Infof("设备 %s MCP会话上下文已取消，退出刷新和ping循环", dc.deviceID)
			return
		case <-tick.C:
			findTools(dc.wsEndPointMcp)
			findTools(dc.iotOverMcp)
		case <-pingTick.C:
			ping(dc.wsEndPointMcp)
			ping(dc.iotOverMcp)
		}
	}
}

func (dc *McpClientInstance) sendInitlize(ctx context.Context) error {
	initRequest := mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			ClientInfo: mcp.Implementation{
				Name:    "mcp-go",
				Version: "0.1.0",
			},
			Capabilities: mcp.ClientCapabilities{},
		},
	}

	serverInfo, err := dc.mcpClient.Initialize(ctx, initRequest)
	if err != nil {
		fmt.Println("Failed to initialize: %v", err)
		return err
	}
	dc.serverInfo = serverInfo
	return nil
}

func (dc *McpClientInstance) findTools() (*mcp.ListToolsResult, error) {
	tools, err := dc.mcpClient.ListTools(dc.ctx, mcp.ListToolsRequest{})
	if err != nil {
		logger.Errorf("获取工具列表失败: %v", err)
		return nil, err
	}
	return tools, nil
}

// handleJSONRPCNotification 处理JSON-RPC通知
func (dc *McpClientInstance) handleJSONRPCNotification(notif mcp.JSONRPCNotification) {
	logger.Infof("收到MCP服务器通知: %s", notif.Method)
	return
}

// handleJSONRPCError 处理JSON-RPC错误
func (dc *McpClientInstance) handleJSONRPCError(errMsg mcp.JSONRPCError) error {
	logger.Errorf("收到MCP服务器错误: %+v", errMsg.Error)
	return nil
}

// GetTools 获取工具列表
func (dc *DeviceMcpSession) GetTools() map[string]tool.InvokableTool {
	tools := make(map[string]tool.InvokableTool)
	if dc.wsEndPointMcp != nil {
		tools = dc.wsEndPointMcp.tools
	}
	if dc.iotOverMcp != nil {
		for k, v := range dc.iotOverMcp.tools {
			tools[k] = v
		}
	}
	return tools
}

func (dc *DeviceMcpSession) GetToolByName(toolName string) (tool.InvokableTool, bool) {
	if dc.wsEndPointMcp != nil {
		if tool, ok := dc.wsEndPointMcp.tools[toolName]; ok {
			return tool, true
		}
	}
	if dc.iotOverMcp != nil {
		if tool, ok := dc.iotOverMcp.tools[toolName]; ok {
			return tool, true
		}
	}
	return nil, false
}
