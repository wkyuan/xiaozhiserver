package main

import (
	"context"
	"fmt"
	"log"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func main() {
	//endPoint := "wss://api.xiaozhi.me/mcp/?token=eyJhbGciOiJFUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1c2VySWQiOjE0NDQzNSwiYWdlbnRJZCI6MzUxNjQsImVuZHBvaW50SWQiOiJhZ2VudF8zNTE2NCIsInB1cnBvc2UiOiJtY3AtZW5kcG9pbnQiLCJpYXQiOjE3NDk1NDk2MzR9.nPMAHaYyRrxQGqHnzFk-SqLDb61p3YGJqRsQ3TZZEqPxQgef0jg_fTLiZsTNVI34VaNOaOobvKnl55VoIuYx7w"
	endPoint := "ws://localhost:8989/xiaozhi/mcp/shijingbo"
	s := server.NewMCPServer("mcp_websocket_server", "1.0.0")

	// Add tool
	tool := mcp.NewTool("hello_world",
		mcp.WithDescription("Say hello to someone"),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Name of the person to greet"),
		),
	)

	// 新增查询天气工具
	weatherTool := mcp.NewTool("query_weather",
		mcp.WithDescription("查询天气"),
	)

	// Add tool handler
	s.AddTool(tool, helloHandler)
	// 注册查询天气工具
	s.AddTool(weatherTool, queryWeatherHandler)

	transport, err := NewWebSocketServerTransport(endPoint, WithWebSocketServerOptionMcpServer(s))
	if err != nil {
		log.Fatalf("Failed to create websocket server transport: %v", err)
	}
	transport.Run()
}

func helloHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name, err := request.RequireString("name")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Hello, %s!", name)), nil
}

// 查询天气 handler
func queryWeatherHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return mcp.NewToolResultText("天气晴朗 20度 北风3级"), nil
}
