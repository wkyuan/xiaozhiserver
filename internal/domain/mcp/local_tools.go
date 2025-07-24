package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	log "xiaozhi-esp32-server-golang/logger"
)

// LocalToolRegistry 本地工具注册表
type LocalToolRegistry struct {
	tools map[string]tool.InvokableTool
}

var localRegistry *LocalToolRegistry

func init() {
	localRegistry = &LocalToolRegistry{
		tools: make(map[string]tool.InvokableTool),
	}

	// 注册本地工具
	registerLocalTools()
}

// registerLocalTools 注册所有本地工具
func registerLocalTools() {
	// 注册退出对话工具
	exitTool := &ExitChatTool{}
	localRegistry.tools["exit_chat"] = exitTool

	log.Info("已注册本地MCP工具: exit_chat")
}

// GetLocalTools 获取所有本地工具
func GetLocalTools() map[string]tool.InvokableTool {
	return localRegistry.tools
}

// CheckToolShouldReturnToLLM 检查工具是否需要回传结果给LLM
// 默认为true，只有实现了LocalToolInfo接口且ShouldReturnToLLM返回false的工具才不回传
func CheckToolShouldReturnToLLM(tool tool.InvokableTool) bool {
	if localTool, ok := tool.(LocalToolInfo); ok {
		return localTool.ShouldReturnToLLM()
	}
	// 默认回传给LLM
	return true
}

// LocalToolInfo 本地工具信息，扩展原有的tool.InvokableTool接口
type LocalToolInfo interface {
	tool.InvokableTool
	// ShouldReturnToLLM 判断工具执行结果是否需要回传给LLM
	ShouldReturnToLLM() bool
}

// ExitChatTool 退出对话工具
type ExitChatTool struct{}

// Info 获取工具信息，实现BaseTool接口
func (t *ExitChatTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name:        "exit_chat",
		Desc:        "结束当前对话会话，优雅地断开与用户的连接。当用户明确表示要退出、结束对话或不再需要服务时使用此工具。",
		ParamsOneOf: &schema.ParamsOneOf{},
	}, nil
}

// ShouldReturnToLLM 退出对话工具不需要回传结果给LLM，因为会话已经结束
func (t *ExitChatTool) ShouldReturnToLLM() bool {
	return false
}

// InvokableRun 执行工具，实现InvokableTool接口
func (t *ExitChatTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	log.Infof("执行退出对话工具，参数: %s", argumentsInJSON)

	// 解析参数
	var args struct {
		DeviceID string `json:"device_id,omitempty"`
		Reason   string `json:"reason,omitempty"`
	}

	if argumentsInJSON != "" && argumentsInJSON != "{}" {
		if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
			return "", fmt.Errorf("解析工具参数失败: %v", err)
		}
	}

	// 使用提供的device_id或从上下文中获取
	deviceID := args.DeviceID
	if deviceID == "" {
		// 尝试从上下文中获取device_id
		if deviceIDFromCtx := ctx.Value("device_id"); deviceIDFromCtx != nil {
			if deviceIDStr, ok := deviceIDFromCtx.(string); ok {
				deviceID = deviceIDStr
			}
		}
	}

	if deviceID == "" {
		return "", fmt.Errorf("设备ID不能为空，无法确定要关闭哪个设备的对话")
	}

	// 通过ChatManager注册表关闭对话
	// 这里需要导入chat包，但为了避免循环依赖，我们使用函数指针
	if exitChatFunc != nil {
		err := exitChatFunc(deviceID)
		if err != nil {
			log.Errorf("关闭设备 %s 对话失败: %v", deviceID, err)
			return "", fmt.Errorf("关闭对话失败: %v", err)
		}
	} else {
		return "", fmt.Errorf("退出对话功能未初始化")
	}

	reason := args.Reason
	if reason == "" {
		reason = "用户请求退出对话"
	}

	log.Infof("设备 %s 对话已退出，原因: %s", deviceID, reason)

	result := map[string]interface{}{
		"success":   true,
		"device_id": deviceID,
		"reason":    reason,
		"message":   "对话已成功退出",
	}

	resultJSON, _ := json.Marshal(result)
	return string(resultJSON), nil
}

// 使用函数指针避免循环依赖
var exitChatFunc func(deviceID string) error

// SetExitChatFunc 设置退出对话函数，由chat包调用
func SetExitChatFunc(f func(deviceID string) error) {
	exitChatFunc = f
	log.Info("已设置退出对话函数")
}
