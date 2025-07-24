package mcp

import (
	"context"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGlobalMCPManager_Singleton(t *testing.T) {
	// 测试单例模式
	manager1 := GetGlobalMCPManager()
	manager2 := GetGlobalMCPManager()

	assert.Equal(t, manager1, manager2, "应该返回同一个实例")
}

func TestDeviceMCPManager_Singleton(t *testing.T) {
	// 测试单例模式
	manager1 := GetDeviceMCPManager()
	manager2 := GetDeviceMCPManager()

	assert.Equal(t, manager1, manager2, "应该返回同一个实例")
}

func TestGlobalMCPManager_StartStop(t *testing.T) {
	// 设置测试配置
	viper.Set("mcp.global.enabled", false)

	manager := GetGlobalMCPManager()

	// 测试启动（禁用状态）
	err := manager.Start()
	assert.NoError(t, err)

	// 测试停止
	err = manager.Stop()
	assert.NoError(t, err)
}

func TestMCPTool_Info(t *testing.T) {
	tool := &mcpTool{
		name:        "test_tool",
		description: "测试工具",
		inputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"query": map[string]interface{}{
					"type": "string",
				},
			},
		},
		serverName: "test_server",
		client:     nil, // 测试中不需要真实客户端
	}

	info, err := tool.Info(context.Background())
	require.NoError(t, err)

	assert.Equal(t, "test_tool", info.Name)
	assert.Equal(t, "测试工具", info.Desc)
}

func TestMCPTool_InvokableRun(t *testing.T) {
	tool := &mcpTool{
		name:        "test_tool",
		description: "测试工具",
		inputSchema: map[string]interface{}{},
		serverName:  "test_server",
		client:      nil, // 测试中不需要真实客户端
	}

	// 这个测试会失败，因为客户端为nil
	// 但可以验证方法签名和基本逻辑
	_, err := tool.InvokableRun(context.Background(), `{"query": "test"}`)
	assert.Error(t, err)                         // 预期会有错误，因为客户端为nil
	assert.Contains(t, err.Error(), "调用MCP工具失败") // 验证错误消息包含预期文本
}

func TestDeviceMCPManager_GetDeviceTools(t *testing.T) {
	manager := GetDeviceMCPManager()

	// 测试获取不存在设备的工具
	tools := manager.GetDeviceTools("non_existent_device")
	assert.Empty(t, tools)
}

func TestGlobalMCPManager_GetAllTools(t *testing.T) {
	manager := GetGlobalMCPManager()

	// 测试获取所有工具（初始状态应该为空）
	tools := manager.GetAllTools()
	assert.NotNil(t, tools)
}

func TestGlobalMCPManager_GetToolByName(t *testing.T) {
	manager := GetGlobalMCPManager()

	// 测试获取不存在的工具
	tool, exists := manager.GetToolByName("non_existent_tool")
	assert.False(t, exists)
	assert.Nil(t, tool)
}

func TestMCPServerConfig_Structure(t *testing.T) {
	config := MCPServerConfig{
		Name:    "test_server",
		SSEUrl:  "http://localhost:3001/sse",
		Enabled: true,
	}

	assert.Equal(t, "test_server", config.Name)
	assert.Equal(t, "http://localhost:3001/sse", config.SSEUrl)
	assert.True(t, config.Enabled)
}

func TestReconnectConfig_Structure(t *testing.T) {
	config := ReconnectConfig{
		Interval:    5 * time.Second,
		MaxAttempts: 10,
	}

	assert.Equal(t, 5*time.Second, config.Interval)
	assert.Equal(t, 10, config.MaxAttempts)
}

// TestMCPGoStructures 测试 mcp-go 库结构体的使用
func TestMCPGoStructures(t *testing.T) {
	t.Run("InitializeRequest", func(t *testing.T) {
		initRequest := mcp.InitializeRequest{
			Request: mcp.Request{
				Method: string(mcp.MethodInitialize),
			},
			Params: mcp.InitializeParams{
				ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
				ClientInfo: mcp.Implementation{
					Name:    "test-client",
					Version: "1.0.0",
				},
				Capabilities: mcp.ClientCapabilities{
					Experimental: make(map[string]any),
				},
			},
		}

		assert.Equal(t, string(mcp.MethodInitialize), initRequest.Request.Method)
		assert.Equal(t, "test-client", initRequest.Params.ClientInfo.Name)
	})

	t.Run("JSONRPCRequest", func(t *testing.T) {
		request := mcp.JSONRPCRequest{
			JSONRPC: mcp.JSONRPC_VERSION,
			ID:      mcp.NewRequestId(1),
			Request: mcp.Request{
				Method: string(mcp.MethodToolsList),
			},
		}

		assert.Equal(t, mcp.JSONRPC_VERSION, request.JSONRPC)
		assert.Equal(t, string(mcp.MethodToolsList), request.Request.Method)
	})

	t.Run("Tool", func(t *testing.T) {
		tool := mcp.NewTool(
			"test-tool",
			mcp.WithDescription("A test tool"),
		)

		assert.Equal(t, "test-tool", tool.Name)
		assert.Equal(t, "A test tool", tool.Description)
	})
}

// 创建测试工具
func TestMCPTool_InvokableRun_NewTool(t *testing.T) {
	testTool := &mcpTool{
		name:        "test_tool",
		description: "测试工具",
		inputSchema: map[string]interface{}{
			"type": "object",
		},
		serverName: "test_server",
		client:     nil, // 测试中不需要真实客户端
	}

	// 这个测试会失败，因为没有真实的MCP服务器
	// 但可以验证方法签名和基本逻辑
	_, err := testTool.InvokableRun(context.Background(), `{"query": "test"}`)
	assert.Error(t, err) // 预期会有网络错误
}
