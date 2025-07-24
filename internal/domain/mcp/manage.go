package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/spf13/viper"

	log "xiaozhi-esp32-server-golang/logger"
)

// MCPServerConfig MCP服务器配置
type MCPServerConfig struct {
	Name    string `json:"name" mapstructure:"name"`
	SSEUrl  string `json:"sse_url" mapstructure:"sse_url"`
	Enabled bool   `json:"enabled" mapstructure:"enabled"`
}

// GlobalMCPManager 全局MCP管理器
type GlobalMCPManager struct {
	servers       map[string]*MCPServerConnection
	tools         map[string]tool.InvokableTool
	mu            sync.RWMutex
	ctx           context.Context
	cancel        context.CancelFunc
	reconnectConf ReconnectConfig
	httpClient    *http.Client
}

// ReconnectConfig 重连配置
type ReconnectConfig struct {
	Interval    time.Duration
	MaxAttempts int
}

// MCPServerConnection MCP服务器连接
type MCPServerConnection struct {
	config     MCPServerConfig
	client     *client.Client
	tools      map[string]tool.InvokableTool
	connected  bool
	mu         sync.RWMutex
	lastError  error
	retryCount int
	lastPing   time.Time
}

var (
	globalManager *GlobalMCPManager
	once          sync.Once
)

// GetGlobalMCPManager 获取全局MCP管理器单例
func GetGlobalMCPManager() *GlobalMCPManager {
	once.Do(func() {
		ctx, cancel := context.WithCancel(context.Background())
		globalManager = &GlobalMCPManager{
			servers: make(map[string]*MCPServerConnection),
			tools:   make(map[string]tool.InvokableTool),
			ctx:     ctx,
			cancel:  cancel,
			reconnectConf: ReconnectConfig{
				Interval:    time.Duration(viper.GetInt("mcp.global.reconnect_interval")) * time.Second,
				MaxAttempts: viper.GetInt("mcp.global.max_reconnect_attempts"),
			},
			httpClient: &http.Client{
				Timeout: 600 * time.Second,
			},
		}
	})
	return globalManager
}

// Start 启动全局MCP管理器
func (g *GlobalMCPManager) Start() error {
	// 首先检查配置
	CheckMCPConfig()

	// 首先注册本地工具
	g.registerLocalTools()

	if !viper.GetBool("mcp.global.enabled") {
		log.Info("全局MCP管理器已禁用，但本地工具已注册")
		return nil
	}

	var serverConfigs []MCPServerConfig
	if err := viper.UnmarshalKey("mcp.global.servers", &serverConfigs); err != nil {
		log.Errorf("解析MCP服务器配置失败: %v", err)
		return fmt.Errorf("解析MCP服务器配置失败: %v", err)
	}

	log.Infof("从配置中读取到 %d 个MCP服务器配置", len(serverConfigs))

	// 详细记录每个服务器配置
	for i, config := range serverConfigs {
		log.Infof("MCP服务器[%d]: Name=%s, SSEUrl=%s, Enabled=%v",
			i+1, config.Name, config.SSEUrl, config.Enabled)
	}

	// 连接启用的服务器
	connectedCount := 0
	for _, config := range serverConfigs {
		if config.Enabled {
			if err := g.connectToServer(config); err != nil {
				log.Errorf("连接到MCP服务器 %s 失败: %v", config.Name, err)
				// 继续尝试连接其他服务器，而不是直接返回错误
			} else {
				connectedCount++
			}
		} else {
			log.Infof("MCP服务器 %s 已禁用，跳过连接", config.Name)
		}
	}

	log.Infof("成功连接了 %d 个MCP服务器", connectedCount)

	// 启动监控goroutine
	go g.monitorConnections()

	log.Info("全局MCP管理器已启动")
	return nil
}

// Stop 停止全局MCP管理器
func (g *GlobalMCPManager) Stop() error {
	g.cancel()

	g.mu.Lock()
	defer g.mu.Unlock()

	for name, conn := range g.servers {
		if err := conn.disconnect(); err != nil {
			log.Errorf("断开MCP服务器 %s 连接失败: %v", name, err)
		}
	}

	g.servers = make(map[string]*MCPServerConnection)
	g.tools = make(map[string]tool.InvokableTool)

	log.Info("全局MCP管理器已停止")
	return nil
}

// connectToServer 连接到MCP服务器
func (g *GlobalMCPManager) connectToServer(config MCPServerConfig) error {
	// 验证配置
	if config.Name == "" {
		return fmt.Errorf("MCP服务器名称不能为空")
	}

	if config.SSEUrl == "" {
		log.Warnf("MCP服务器 %s 的SSE URL为空，跳过连接", config.Name)
		return fmt.Errorf("MCP服务器 %s 的SSE URL为空", config.Name)
	}

	log.Infof("正在连接MCP服务器: %s (URL: %s)", config.Name, config.SSEUrl)

	conn := &MCPServerConnection{
		config: config,
		tools:  make(map[string]tool.InvokableTool),
	}

	// 创建 SSE 传输层
	sseTransport, err := transport.NewSSE(config.SSEUrl)
	if err != nil {
		return fmt.Errorf("创建SSE传输层失败: %v", err)
	}

	// 使用 client.NewClient 创建 MCP 客户端
	mcpClient := client.NewClient(sseTransport)

	conn.client = mcpClient

	// 连接到服务器
	if err := conn.connect(); err != nil {
		return fmt.Errorf("连接MCP服务器失败: %v", err)
	}

	g.mu.Lock()
	g.servers[config.Name] = conn
	g.mu.Unlock()

	log.Infof("已连接到MCP服务器: %s", config.Name)
	return nil
}

// connect 连接到MCP服务器
func (conn *MCPServerConnection) connect() error {
	// 使用背景上下文，不设置超时，让SSE连接长期保持
	ctx := context.Background()

	// 如果client为空，重新创建client
	if conn.client == nil {
		sseTransport, err := transport.NewSSE(conn.config.SSEUrl)
		if err != nil {
			return fmt.Errorf("创建SSE传输层失败: %v", err)
		}
		conn.client = client.NewClient(sseTransport)
	}

	log.Infof("开始连接MCP服务器: %s, SSE URL: %s", conn.config.Name, conn.config.SSEUrl)

	// 启动客户端
	if err := conn.client.Start(ctx); err != nil {
		log.Errorf("启动MCP客户端失败，服务器: %s, 错误: %v", conn.config.Name, err)
		return fmt.Errorf("启动客户端失败: %v", err)
	}

	log.Infof("MCP客户端启动成功: %s", conn.config.Name)

	// 初始化客户端
	initRequest := mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			ClientInfo: mcp.Implementation{
				Name:    "xiaozhi-esp32-server",
				Version: "1.0.0",
			},
			Capabilities: mcp.ClientCapabilities{
				Experimental: make(map[string]any),
			},
		},
	}

	log.Infof("正在初始化MCP服务器: %s", conn.config.Name)
	initResult, err := conn.client.Initialize(ctx, initRequest)
	if err != nil {
		log.Errorf("初始化MCP服务器失败，服务器: %s, 错误: %v", conn.config.Name, err)
		return fmt.Errorf("初始化失败: %v", err)
	}

	log.Infof("MCP服务器初始化成功: %s, 结果: %+v", conn.config.Name, initResult)

	// 获取工具列表
	if err := conn.refreshTools(ctx); err != nil {
		log.Errorf("获取工具列表失败: %v", err)
		// 不直接返回错误，因为工具列表获取失败不应该阻止连接建立
	}

	conn.mu.Lock()
	conn.connected = true
	conn.lastError = nil
	conn.retryCount = 0
	conn.mu.Unlock()

	log.Infof("MCP服务器连接建立完成: %s", conn.config.Name)
	return nil
}

// refreshTools 刷新工具列表
func (conn *MCPServerConnection) refreshTools(ctx context.Context) error {
	// 获取工具列表
	listRequest := mcp.ListToolsRequest{}
	toolsResult, err := conn.client.ListTools(ctx, listRequest)
	if err != nil {
		return fmt.Errorf("获取工具列表失败: %v", err)
	}

	conn.mu.Lock()
	defer conn.mu.Unlock()

	conn.tools = ConvertMcpToolListToInvokableToolList(toolsResult.Tools, conn.config.Name, conn.client)

	// 更新全局工具列表
	globalManager.updateGlobalTools(conn.config.Name, conn.tools)

	log.Infof("MCP服务器 %s 工具列表已更新，共 %d 个工具", conn.config.Name, len(conn.tools))
	return nil
}

func ConvertMcpToolListToInvokableToolList(tools []mcp.Tool, serverName string, client *client.Client) map[string]tool.InvokableTool {
	invokeTools := make(map[string]tool.InvokableTool)
	for _, tool := range tools {
		// 转换InputSchema类型
		var inputSchema map[string]interface{}
		// 通过JSON序列化和反序列化来转换类型
		if schemaBytes, err := json.Marshal(tool.InputSchema); err == nil {
			json.Unmarshal(schemaBytes, &inputSchema)
		}

		mcpToolInstance := &mcpTool{
			name:        tool.Name,
			description: tool.Description,
			inputSchema: inputSchema,
			serverName:  serverName,
			client:      client,
		}
		invokeTools[tool.Name] = mcpToolInstance
	}
	return invokeTools
}

// disconnect 断开连接
func (conn *MCPServerConnection) disconnect() error {
	conn.mu.Lock()
	defer conn.mu.Unlock()

	if conn.client != nil {
		// 关闭客户端
		if err := conn.client.Close(); err != nil {
			log.Errorf("关闭MCP客户端失败: %v", err)
		}
		conn.client = nil
	}

	conn.connected = false
	conn.tools = make(map[string]tool.InvokableTool)

	return nil
}

// updateGlobalTools 更新全局工具列表
func (g *GlobalMCPManager) updateGlobalTools(serverName string, tools map[string]tool.InvokableTool) {
	g.mu.Lock()
	defer g.mu.Unlock()

	// 移除该服务器的旧工具
	for name, mcpToolInterface := range g.tools {
		if mt, ok := mcpToolInterface.(*mcpTool); ok && mt.serverName == serverName {
			delete(g.tools, name)
		}
	}

	// 添加新工具
	for name, mcpToolInterface := range tools {
		g.tools[fmt.Sprintf("%s_%s", serverName, name)] = mcpToolInterface
	}
}

// registerLocalTools 注册本地工具
func (g *GlobalMCPManager) registerLocalTools() {
	g.mu.Lock()
	defer g.mu.Unlock()

	// 获取所有本地工具
	localTools := GetLocalTools()

	// 添加本地工具到全局工具列表
	for name, tool := range localTools {
		g.tools[fmt.Sprintf("local_%s", name)] = tool
		log.Infof("已注册本地工具: local_%s", name)
	}

	log.Infof("已注册 %d 个本地工具", len(localTools))
}

// GetAllTools 获取所有可用工具
func (g *GlobalMCPManager) GetAllTools() map[string]tool.InvokableTool {
	g.mu.RLock()
	defer g.mu.RUnlock()

	result := make(map[string]tool.InvokableTool)
	for name, mcpToolInterface := range g.tools {
		result[name] = mcpToolInterface
	}
	return result
}

// GetToolByName 根据名称获取工具
func (g *GlobalMCPManager) GetToolByName(name string) (tool.InvokableTool, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	// 首先直接查找工具名（支持本地工具如 local_exit_chat）
	if mcpToolInterface, exists := g.tools[name]; exists {
		return mcpToolInterface, true
	}

	// 然后尝试查找远程服务器的工具（格式：serverName_toolName）
	for _, conn := range g.servers {
		serverToolName := fmt.Sprintf("%s_%s", conn.config.Name, name)
		if mcpToolInterface, exists := g.tools[serverToolName]; exists {
			return mcpToolInterface, true
		}
	}

	return nil, false
}

// isSessionClosedError 判断是否为session closed错误
func isSessionClosedError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "session closed")
}

// monitorConnections 监控连接状态
func (g *GlobalMCPManager) monitorConnections() {
	ticker := time.NewTicker(g.reconnectConf.Interval)
	pingTicker := time.NewTicker(30 * time.Second) // 每30秒ping一次
	defer ticker.Stop()
	defer pingTicker.Stop()

	for {
		select {
		case <-g.ctx.Done():
			return
		case <-ticker.C:
			g.checkAndReconnect()

			// 定时健康检查
			g.mu.RLock()
			for name, conn := range g.servers {
				go func(name string, conn *MCPServerConnection) {
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()
					if err := conn.refreshTools(ctx); err != nil {
						if isSessionClosedError(err) {
							log.Warnf("MCP服务器 %s 健康检查失败(session closed): %v", name, err)
							conn.mu.Lock()
							conn.connected = false
							conn.lastError = err
							conn.mu.Unlock()
						} else {
							log.Debugf("MCP服务器 %s 健康检查失败(非session closed错误): %v", name, err)
						}
					}
				}(name, conn)
			}
			g.mu.RUnlock()
		case <-pingTicker.C:
			// 执行ping检测
			g.mu.RLock()
			for name, conn := range g.servers {
				go func(name string, conn *MCPServerConnection) {
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()

					if err := conn.ping(ctx); err != nil {
						log.Warnf("MCP服务器 %s ping失败: %v", name, err)
						if isSessionClosedError(err) {
							conn.mu.Lock()
							conn.connected = false
							conn.lastError = err
							conn.mu.Unlock()
						}
					} else {
						log.Debugf("MCP服务器 %s ping成功", name)
					}
				}(name, conn)
			}
			g.mu.RUnlock()
		}
	}
}

// checkAndReconnect 检查并重连断开的服务器
func (g *GlobalMCPManager) checkAndReconnect() {
	g.mu.RLock()
	servers := make(map[string]*MCPServerConnection)
	for name, conn := range g.servers {
		servers[name] = conn
	}
	g.mu.RUnlock()

	for name, conn := range servers {
		conn.mu.RLock()
		connected := conn.connected
		retryCount := conn.retryCount
		conn.mu.RUnlock()

		if !connected && retryCount < g.reconnectConf.MaxAttempts {
			log.Infof("尝试重连MCP服务器: %s (第%d次)", name, retryCount+1)

			conn.mu.Lock()
			conn.retryCount++
			conn.mu.Unlock()

			if _, err := g.reconnectServer(name); err != nil {
				log.Errorf("重连MCP服务器 %s 失败: %v", name, err)
				conn.mu.Lock()
				conn.lastError = err
				conn.mu.Unlock()
			}
		}
	}
}

// mcpTool MCP工具实现
type mcpTool struct {
	name        string
	description string
	inputSchema map[string]interface{}
	serverName  string
	client      *client.Client
}

// Info 获取工具信息，实现BaseTool接口
func (t *mcpTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	// 创建参数定义
	var paramsOneOf *schema.ParamsOneOf
	if t.inputSchema != nil {
		paramsOneOf = &schema.ParamsOneOf{
			// 简化的参数定义，实际使用中需要完善
		}
	}

	return &schema.ToolInfo{
		Name:        t.name,
		Desc:        t.description,
		ParamsOneOf: paramsOneOf,
	}, nil
}

// reconnectServer 重连服务器并返回新的client
func (g *GlobalMCPManager) reconnectServer(serverName string) (*client.Client, error) {
	g.mu.RLock()
	var conn *MCPServerConnection
	for _, c := range g.servers {
		if c.config.Name == serverName {
			conn = c
			break
		}
	}
	g.mu.RUnlock()

	if conn == nil {
		return nil, fmt.Errorf("未找到服务器连接: %s", serverName)
	}

	// 断开连接
	if err := conn.disconnect(); err != nil {
		log.Errorf("断开连接失败: %v", err)
	}

	// 等待一小段时间确保资源释放
	time.Sleep(time.Second)

	// 重新连接
	if err := conn.connect(); err != nil {
		return nil, fmt.Errorf("重连失败: %v", err)
	}

	return conn.client, nil
}

// InvokableRun 调用工具，实现InvokableTool接口
func (t *mcpTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	// 检查客户端是否可用
	if t.client == nil {
		return "", fmt.Errorf("调用MCP工具失败: MCP客户端未初始化")
	}

	// 解析参数
	var arguments map[string]interface{}
	if argumentsInJSON != "" {
		if err := json.Unmarshal([]byte(argumentsInJSON), &arguments); err != nil {
			return "", fmt.Errorf("解析工具参数失败: %v", err)
		}
	}

	// 准备调用请求
	callRequest := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      t.name,
			Arguments: arguments,
		},
	}

	// 第一次尝试调用
	result, err := t.client.CallTool(ctx, callRequest)
	if err != nil && isSessionClosedError(err) {
		log.Warnf("工具 %s 调用失败(session closed): %v，尝试重连后重试", t.name, err)

		// 重连并获取新的client
		newClient, err := GetGlobalMCPManager().reconnectServer(t.serverName)
		if err != nil {
			return "", fmt.Errorf("重连服务器失败: %v", err)
		}

		// 更新工具的client引用
		t.client = newClient

		// 重试调用
		result, err = t.client.CallTool(ctx, callRequest)
		if err != nil {
			return "", fmt.Errorf("重连后调用仍然失败: %v", err)
		}
	} else if err != nil {
		return "", fmt.Errorf("调用工具失败: %v", err)
	}

	// 处理结果
	if len(result.Content) > 0 {
		// 将结果转换为字符串
		if textContent, ok := result.Content[0].(mcp.TextContent); ok {
			return textContent.Text, nil
		}

		// 如果是其他类型的内容，尝试序列化为JSON
		contentBytes, err := json.Marshal(result.Content[0])
		if err != nil {
			return "", fmt.Errorf("序列化工具结果失败: %v", err)
		}
		return string(contentBytes), nil
	}

	return "", fmt.Errorf("工具调用未返回任何内容")
}

// ping 发送ping请求检测连接状态
func (conn *MCPServerConnection) ping(ctx context.Context) error {
	if conn.client == nil {
		return fmt.Errorf("client未初始化")
	}

	// 使用空的Ping请求作为ping
	err := conn.client.Ping(ctx)
	if err != nil {
		return fmt.Errorf("ping失败: %v", err)
	}

	conn.mu.Lock()
	conn.lastPing = time.Now()
	conn.mu.Unlock()

	return nil
}
