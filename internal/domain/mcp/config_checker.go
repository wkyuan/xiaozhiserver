package mcp

import (
	"fmt"
	"strings"

	log "xiaozhi-esp32-server-golang/logger"

	"github.com/spf13/viper"
)

// CheckMCPConfig 检查MCP配置并报告潜在问题
func CheckMCPConfig() {
	log.Info("=== MCP配置检查 ===")

	// 检查全局启用状态
	globalEnabled := viper.GetBool("mcp.global.enabled")
	log.Infof("全局MCP启用状态: %v", globalEnabled)

	if !globalEnabled {
		log.Info("全局MCP已禁用，配置检查完成")
		return
	}

	// 检查重连配置
	reconnectInterval := viper.GetInt("mcp.global.reconnect_interval")
	maxAttempts := viper.GetInt("mcp.global.max_reconnect_attempts")
	log.Infof("重连配置: 间隔=%d秒, 最大尝试次数=%d", reconnectInterval, maxAttempts)

	// 检查服务器配置
	var serverConfigs []MCPServerConfig
	if err := viper.UnmarshalKey("mcp.global.servers", &serverConfigs); err != nil {
		log.Errorf("❌ 解析MCP服务器配置失败: %v", err)
		return
	}

	if len(serverConfigs) == 0 {
		log.Warn("⚠️  未配置任何MCP服务器")
		return
	}

	log.Infof("共配置了 %d 个MCP服务器:", len(serverConfigs))

	enabledCount := 0
	problemCount := 0

	for i, config := range serverConfigs {
		status := "✅"
		issues := []string{}

		// 检查名称
		if config.Name == "" {
			status = "❌"
			issues = append(issues, "名称为空")
			problemCount++
		}

		// 检查SSE URL
		if config.SSEUrl == "" {
			status = "❌"
			issues = append(issues, "SSE URL为空")
			problemCount++
		} else {
			// 检查URL格式
			if !strings.HasPrefix(config.SSEUrl, "http://") && !strings.HasPrefix(config.SSEUrl, "https://") {
				status = "⚠️"
				issues = append(issues, "SSE URL格式可能不正确")
			}
		}

		// 检查启用状态
		if config.Enabled {
			enabledCount++
		}

		// 输出检查结果
		issueStr := ""
		if len(issues) > 0 {
			issueStr = fmt.Sprintf(" - 问题: %s", strings.Join(issues, ", "))
		}

		log.Infof("  [%d] %s %s (URL: %s, 启用: %v)%s",
			i+1, status, config.Name, config.SSEUrl, config.Enabled, issueStr)
	}

	// 总结
	log.Infof("配置检查完成: %d个服务器已启用, %d个存在问题", enabledCount, problemCount)

	if problemCount > 0 {
		log.Warn("⚠️  发现配置问题，请检查上述错误并修复")
	}

	// 检查设备MCP配置
	checkDeviceMCPConfig()

	log.Info("=== MCP配置检查完成 ===")
}

// checkDeviceMCPConfig 检查设备MCP配置
func checkDeviceMCPConfig() {
	log.Info("--- 设备MCP配置检查 ---")

	deviceEnabled := viper.GetBool("mcp.device.enabled")
	log.Infof("设备MCP启用状态: %v", deviceEnabled)

	if !deviceEnabled {
		log.Info("设备MCP已禁用")
		return
	}

	websocketPath := viper.GetString("mcp.device.websocket_path")
	maxConnections := viper.GetInt("mcp.device.max_connections_per_device")

	log.Infof("WebSocket路径: %s", websocketPath)
	log.Infof("每设备最大连接数: %d", maxConnections)

	// 检查路径格式
	if websocketPath == "" {
		log.Warn("⚠️  WebSocket路径为空")
	} else if !strings.HasPrefix(websocketPath, "/") {
		log.Warn("⚠️  WebSocket路径应以'/'开头")
	}

	if maxConnections <= 0 {
		log.Warn("⚠️  每设备最大连接数应大于0")
	}
}
