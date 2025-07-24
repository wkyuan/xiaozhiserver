package mcp

import (
	"context"
	"encoding/json"
	"fmt"
)

// ExampleUsage 展示如何使用本地MCP工具
func ExampleUsage() {
	fmt.Println("=== 本地MCP工具使用示例 ===")

	// 1. 获取所有本地工具
	localTools := GetLocalTools()
	fmt.Printf("可用的本地工具: %d 个\n", len(localTools))

	for name, tool := range localTools {
		// 获取工具信息
		if info, err := tool.Info(context.Background()); err == nil {
			fmt.Printf("- %s: %s\n", name, info.Desc)
		}
	}

	// 2. 获取退出对话工具
	exitTool, exists := localTools["exit_chat"]
	if !exists {
		fmt.Println("❌ exit_chat 工具不存在")
		return
	}

	fmt.Println("\n=== 退出对话工具详情 ===")
	toolInfo, err := exitTool.Info(context.Background())
	if err != nil {
		fmt.Printf("❌ 获取工具信息失败: %v\n", err)
		return
	}

	fmt.Printf("工具名称: %s\n", toolInfo.Name)
	fmt.Printf("工具描述: %s\n", toolInfo.Desc)

	// 3. 模拟工具调用（仅用于演示，实际使用中设备ID应该存在）
	fmt.Println("\n=== 模拟工具调用 ===")

	// 构建调用参数
	args := map[string]interface{}{
		"device_id": "demo_device_123",
		"reason":    "用户请求退出对话",
	}

	argsJSON, _ := json.Marshal(args)

	// 调用工具（这会失败，因为demo_device_123不存在）
	result, err := exitTool.InvokableRun(context.Background(), string(argsJSON))
	if err != nil {
		fmt.Printf("⚠️ 工具调用失败（预期行为）: %v\n", err)
	} else {
		fmt.Printf("✅ 工具调用成功: %s\n", result)
	}

	fmt.Println("\n=== 工具回传策略 ===")
	fmt.Printf("exit_chat 工具回传策略: %v\n", CheckToolShouldReturnToLLM(exitTool))
	fmt.Println("- exit_chat: 不回传 (因为会话已结束)")
	fmt.Println("- 其他工具: 默认回传给LLM")

	fmt.Println("\n=== 如何在LLM中使用 ===")
	fmt.Println("当用户说 '退出对话'、'结束聊天' 等类似话语时，")
	fmt.Println("LLM可以自动调用 exit_chat 工具来优雅地结束对话。")
	fmt.Println("此工具执行后不会回传结果给LLM，直接结束会话。")

	fmt.Println("\n工具调用示例:")
	fmt.Printf(`{
  "name": "exit_chat",
  "arguments": {
    "device_id": "02:4A:7D:E3:89:BF",
    "reason": "用户主动请求退出对话"
  }
}`)
	fmt.Println()
}

// ShowGlobalMCPTools 展示全局MCP管理器中的所有工具
func ShowGlobalMCPTools() {
	fmt.Println("\n=== 全局MCP工具列表 ===")

	globalManager := GetGlobalMCPManager()
	allTools := globalManager.GetAllTools()

	fmt.Printf("总共 %d 个工具:\n", len(allTools))

	localCount := 0
	remoteCount := 0

	for name, tool := range allTools {
		toolInfo, err := tool.Info(context.Background())
		if err != nil {
			fmt.Printf("- %s: (无法获取信息)\n", name)
			continue
		}

		if len(name) >= 6 && name[:6] == "local_" {
			localCount++
			fmt.Printf("- 🏠 %s: %s\n", name, toolInfo.Desc)
		} else {
			remoteCount++
			fmt.Printf("- 🌐 %s: %s\n", name, toolInfo.Desc)
		}
	}

	fmt.Printf("\n统计: 本地工具 %d 个, 远程工具 %d 个\n", localCount, remoteCount)
}

// DemoExitChatFlow 演示退出对话的完整流程
func DemoExitChatFlow(deviceID string) {
	fmt.Printf("\n=== 演示设备 %s 退出对话流程 ===\n", deviceID)

	// 1. 获取退出工具
	globalManager := GetGlobalMCPManager()
	tool, exists := globalManager.GetToolByName("exit_chat")
	if !exists {
		fmt.Println("❌ 未找到 exit_chat 工具")
		return
	}

	// 2. 构建参数
	args := map[string]interface{}{
		"device_id": deviceID,
		"reason":    "演示退出对话功能",
	}

	argsJSON, _ := json.Marshal(args)

	// 3. 执行工具
	fmt.Printf("🔧 调用工具: local_exit_chat\n")
	fmt.Printf("📋 参数: %s\n", string(argsJSON))

	result, err := tool.InvokableRun(context.Background(), string(argsJSON))
	if err != nil {
		fmt.Printf("❌ 执行失败: %v\n", err)
	} else {
		fmt.Printf("✅ 执行成功: %s\n", result)
	}
}
