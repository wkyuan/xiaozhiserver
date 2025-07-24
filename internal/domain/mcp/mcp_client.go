package mcp

import (
	"fmt"
	log "xiaozhi-esp32-server-golang/logger"

	"github.com/cloudwego/eino/components/tool"
)

func GetToolByName(deviceId string, toolName string) (tool.InvokableTool, bool) {
	log.Infof("查找工具: %s (设备: %s)", toolName, deviceId)

	// 首先尝试查找原始工具名
	tool, ok := globalManager.GetToolByName(toolName)
	if ok {
		log.Infof("在全局管理器中找到工具: %s", toolName)
		return tool, ok
	}

	// 如果没找到，尝试查找带local_前缀的工具名
	localToolName := fmt.Sprintf("local_%s", toolName)
	tool, ok = globalManager.GetToolByName(localToolName)
	if ok {
		log.Infof("找到本地工具: %s", localToolName)
		return tool, ok
	}

	// 尝试从设备专属工具中查找
	tool, ok = mcpClientPool.GetToolByDeviceId(deviceId, toolName)
	if ok {
		log.Infof("从设备工具中找到: %s", toolName)
		return tool, true
	}

	log.Errorf("工具 %s 在所有位置都未找到", toolName)
	return nil, false
}

func GetDeviceMcpClient(deviceId string) *DeviceMcpSession {
	return mcpClientPool.GetMcpClient(deviceId)
}

func AddDeviceMcpClient(deviceId string, mcpClient *DeviceMcpSession) error {
	mcpClientPool.AddMcpClient(deviceId, mcpClient)
	return nil
}

func RemoveDeviceMcpClient(deviceId string) error {
	mcpClientPool.RemoveMcpClient(deviceId)
	return nil
}

// ShouldReturnToolResultToLLM 检查工具是否需要回传结果给LLM
func ShouldReturnToolResultToLLM(tool tool.InvokableTool) bool {
	return CheckToolShouldReturnToLLM(tool)
}

func GetToolsByDeviceId(deviceId string) (map[string]tool.InvokableTool, error) {
	retTools := make(map[string]tool.InvokableTool)
	//从全局管理器获取
	globalTools := globalManager.GetAllTools()
	for toolName, tool := range globalTools {
		retTools[toolName] = tool
	}

	//从MCP客户端池获取
	deviceTools, err := mcpClientPool.GetAllToolsByDeviceId(deviceId)
	if err != nil {
		log.Errorf("获取设备 %s 的工具失败: %v", deviceId, err)
		return retTools, nil
	}
	for toolName, tool := range deviceTools {
		retTools[toolName] = tool
	}
	return retTools, nil
}
