# 本地MCP工具实现

## 概述

本实现为 xiaozhi-esp32-server 项目增加了本地MCP工具支持，特别是实现了用于处理用户退出对话的本地工具。这使得LLM能够在用户明确表示要退出对话时，优雅地关闭会话连接。

## 🎯 核心功能

### 1. 本地工具注册系统
- **LocalToolRegistry**: 本地工具注册表，管理所有本地MCP工具
- **自动注册**: 在系统初始化时自动注册本地工具到全局MCP管理器
- **工具发现**: LLM可以像使用远程MCP工具一样使用本地工具

### 2. ChatManager全局注册表
- **ChatManagerRegistry**: 全局ChatManager注册表，维护设备ID到ChatManager的映射
- **生命周期管理**: 自动注册新连接，清理断开连接
- **并发安全**: 使用读写锁保证并发访问安全

### 3. 退出对话工具
- **ExitChatTool**: 实现了 `tool.InvokableTool` 和 `LocalToolInfo` 接口的本地工具
- **参数支持**: 支持设备ID和退出原因参数
- **优雅关闭**: 通过ChatManager注册表找到对应设备并优雅关闭连接
- **不回传结果**: 标记为不回传结果给LLM，执行后直接结束会话

### 4. 工具回传策略
- **默认行为**: 工具执行结果回传给LLM进行后续处理
- **标记不回传**: 实现 `LocalToolInfo` 接口的工具可以标记为不回传
- **智能处理**: 系统自动根据工具标记决定是否继续LLM处理

## 🏗️ 架构设计

```
┌─────────────────────────────────────────────────────────────┐
│                    LLM 工具调用系统                          │
│   当用户说"退出对话"时，LLM 自动调用 exit_chat 工具            │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                  全局 MCP 管理器                             │
│  • 外部MCP服务器工具: filesystem_read_file                   │
│  • 本地工具: local_exit_chat                                │
│  • 统一调用接口                                              │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                  本地工具注册表                               │
│  • ExitChatTool: 退出对话工具                                │
│  • 可扩展: 支持添加更多本地工具                               │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                ChatManager 注册表                           │
│  • 设备ID → ChatManager 映射                                │
│  • 生命周期管理                                              │
│  • 优雅关闭接口                                              │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                  具体设备 ChatManager                        │
│  • 管理设备连接                                              │
│  • 控制对话会话                                              │
│  • 资源清理                                                  │
└─────────────────────────────────────────────────────────────┘
```

## 📋 实现文件

### 核心文件
- `local_tools.go` - 本地工具实现和注册
- `manager_registry.go` - ChatManager全局注册表
- `local_tools_example.go` - 使用示例和演示

### 修改文件
- `manage.go` - 全局MCP管理器增加本地工具支持
- `app.go` - 应用启动时设置退出函数和注册表
- `chat.go` - ChatManager增加注册表集成

## 🚀 使用流程

### 1. 系统初始化
```go
// 1. 应用启动时自动设置退出函数
mcp.SetExitChatFunc(func(deviceID string) error {
    registry := chat.GetChatManagerRegistry()
    return registry.CloseChatManager(deviceID)
})

// 2. 全局MCP管理器启动时自动注册本地工具
globalManager.registerLocalTools()
```

### 2. 设备连接
```go
// 1. 新设备连接时创建ChatManager
chatManager, err := chat.NewChatManager(deviceID, transport)

// 2. 注册到全局注册表
registry := chat.GetChatManagerRegistry()
registry.RegisterChatManager(deviceID, chatManager)
```

### 3. LLM工具调用
```go
// 1. LLM获取可用工具（包含本地工具）
mcpTools, err := mcp.GetToolsByDeviceId(deviceID)

// 2. 用户说"退出对话"时，LLM调用工具
{
  "name": "local_exit_chat",
  "arguments": {
    "device_id": "02:4A:7D:E3:89:BF",
    "reason": "用户主动请求退出对话"
  }
}

// 3. 工具执行，优雅关闭连接
result := exitTool.InvokableRun(ctx, argumentsJSON)
```

### 4. 连接清理
```go
// 1. 设备断开时自动从注册表移除
registry.UnregisterChatManager(deviceID)

// 2. 资源自动清理
chatManager.Close()
```

## 🔧 扩展指南

### 添加新的本地工具

1. **实现工具结构体**：
```go
type MyLocalTool struct{}

func (t *MyLocalTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
    return &schema.ToolInfo{
        Name: "my_tool",
        Desc: "我的本地工具描述",
        ParamsOneOf: &schema.ParamsOneOf{
            // 参数定义
        },
    }, nil
}

func (t *MyLocalTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
    // 工具实现逻辑
    return "执行结果", nil
}
```

2. **注册工具**：
```go
func registerLocalTools() {
    // 注册现有工具
    exitTool := &ExitChatTool{}
    localRegistry.tools["exit_chat"] = exitTool
    
    // 注册新工具
    myTool := &MyLocalTool{}
    localRegistry.tools["my_tool"] = myTool
    
    log.Info("已注册本地工具: exit_chat, my_tool")
}
```

### 工具参数处理模式

```go
// 解析JSON参数的标准模式
var args struct {
    DeviceID string `json:"device_id"`
    Param1   string `json:"param1,omitempty"`
    Param2   int    `json:"param2,omitempty"`
}

if argumentsInJSON != "" {
    if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
        return "", fmt.Errorf("解析参数失败: %v", err)
    }
}

// 参数验证
if args.DeviceID == "" {
    return "", fmt.Errorf("设备ID不能为空")
}
```

## 📖 创建自定义工具

### 普通工具（回传结果）
```go
type MyTool struct{}

func (t *MyTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
    return &schema.ToolInfo{
        Name: "my_tool",
        Desc: "我的工具，执行结果会回传给LLM",
    }, nil
}

func (t *MyTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
    // 工具逻辑
    return "处理完成", nil
}
```

### 不回传工具（执行后不继续LLM处理）
```go
type MyExitTool struct{}

func (t *MyExitTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
    return &schema.ToolInfo{
        Name: "my_exit_tool", 
        Desc: "退出工具，执行后不回传结果",
    }, nil
}

// 实现LocalToolInfo接口，标记为不回传
func (t *MyExitTool) ShouldReturnToLLM() bool {
    return false
}

func (t *MyExitTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
    // 执行退出逻辑
    return "已退出", nil
}
```

## 📊 监控和调试

### 查看注册的工具
```go
// 查看所有本地工具
localTools := mcp.GetLocalTools()
for name, tool := range localTools {
    info, _ := tool.Info(context.Background())
    fmt.Printf("本地工具: %s - %s\n", name, info.Desc)
}

// 查看全局工具（包含本地和远程）
globalManager := mcp.GetGlobalMCPManager()
allTools := globalManager.GetAllTools()
fmt.Printf("总工具数: %d\n", len(allTools))

// 检查工具回传策略
for name, tool := range allTools {
    shouldReturn := mcp.ShouldReturnToolResultToLLM(tool)
    fmt.Printf("工具 %s 回传策略: %v\n", name, shouldReturn)
}
```

### 查看连接状态
```go
// 查看所有活跃连接
registry := chat.GetChatManagerRegistry()
deviceIDs := registry.GetAllDeviceIDs()
fmt.Printf("活跃设备: %v\n", deviceIDs)

// 查看连接数量
count := registry.GetManagerCount()
fmt.Printf("连接数量: %d\n", count)
```

### 日志监控
系统会自动记录以下关键事件：
- 本地工具注册：`已注册本地工具: local_exit_chat`
- ChatManager注册：`注册ChatManager，设备ID: xxx`
- 工具调用：`执行退出对话工具，参数: {...}`
- 连接关闭：`通过注册表关闭设备 xxx 的ChatManager`

## 🚨 注意事项

### 1. 线程安全
- ChatManagerRegistry 使用读写锁保证并发安全
- LocalToolRegistry 在初始化后只读，无需额外保护
- 工具调用过程中的状态修改需要注意同步

### 2. 资源清理
- 设备断开连接时会自动从注册表移除
- ChatManager关闭时会清理所有相关资源
- 避免内存泄漏和僵尸连接

### 3. 错误处理
- 工具调用失败不会影响整个系统
- 设备不存在时工具调用会优雅失败
- 所有错误都有详细的日志记录

### 4. 扩展考虑
- 本地工具应该是轻量级的，避免耗时操作
- 复杂操作应该异步执行，工具快速返回
- 考虑工具的幂等性和可重试性

## 🎉 总结

通过这个实现，我们成功地：

1. **增强了LLM的能力** - LLM现在可以主动管理对话会话
2. **统一了工具调用接口** - 本地工具和远程工具使用相同的调用方式
3. **提供了扩展框架** - 可以轻松添加更多本地工具
4. **保证了系统稳定性** - 完整的错误处理和资源管理
5. **智能回传策略** - 支持工具结果的差异化处理，提升用户体验

用户现在可以通过自然语言（如"退出对话"、"结束聊天"）来控制对话会话，LLM会智能地调用对应的本地工具来优雅地关闭连接。系统支持两种工具类型：
- **回传工具**：执行后将结果返回给LLM进行后续处理（默认行为）
- **不回传工具**：执行后直接结束，不继续LLM处理（如退出对话工具） 