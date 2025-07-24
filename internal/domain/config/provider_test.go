package user_config

import (
	"context"
	"testing"

	"xiaozhi-esp32-server-golang/internal/domain/user_config/types"
)

func TestMemoryProvider(t *testing.T) {
	ctx := context.Background()

	// 创建内存provider
	config := map[string]interface{}{
		"max_entries": 10,
	}

	provider, err := GetUserConfigProvider("memory", config)
	if err != nil {
		t.Fatalf("创建内存provider失败: %v", err)
	}
	defer provider.Close()

	userID := "test_user_123"

	// 测试设置配置
	userConfig := types.UConfig{
		SystemPrompt: "测试系统提示",
		Llm: types.LlmConfig{
			Type: "openai",
		},
		Tts: types.TtsConfig{
			Type: "edge",
		},
		Asr: types.AsrConfig{
			Type: "funasr",
		},
	}

	err = provider.SetUserConfig(ctx, userID, userConfig)
	if err != nil {
		t.Fatalf("设置用户配置失败: %v", err)
	}

	// 测试获取配置
	retrievedConfig, err := provider.GetUserConfig(ctx, userID)
	if err != nil {
		t.Fatalf("获取用户配置失败: %v", err)
	}

	// 验证配置内容
	if retrievedConfig.SystemPrompt != userConfig.SystemPrompt {
		t.Errorf("系统提示不匹配，期望: %s, 实际: %s", userConfig.SystemPrompt, retrievedConfig.SystemPrompt)
	}

	if retrievedConfig.Llm.Type != userConfig.Llm.Type {
		t.Errorf("LLM类型不匹配，期望: %s, 实际: %s", userConfig.Llm.Type, retrievedConfig.Llm.Type)
	}

	// 测试删除配置
	err = provider.DeleteUserConfig(ctx, userID)
	if err != nil {
		t.Fatalf("删除用户配置失败: %v", err)
	}

	// 验证配置已删除
	emptyConfig, err := provider.GetUserConfig(ctx, userID)
	if err != nil {
		t.Fatalf("删除后获取配置失败: %v", err)
	}

	if emptyConfig.SystemPrompt != "" {
		t.Errorf("配置删除后应为空，但系统提示仍为: %s", emptyConfig.SystemPrompt)
	}
}

func TestProviderAdapter(t *testing.T) {
	ctx := context.Background()

	// 创建内存provider
	provider, err := GetUserConfigProvider("memory", map[string]interface{}{
		"max_entries": 5,
	})
	if err != nil {
		t.Fatalf("创建内存provider失败: %v", err)
	}
	defer provider.Close()

	// 设置一个配置
	userID := "adapter_test_user"
	userConfig := types.UConfig{
		SystemPrompt: "适配器测试",
		Llm: types.LlmConfig{
			Type: "ollama",
		},
	}

	err = provider.SetUserConfig(ctx, userID, userConfig)
	if err != nil {
		t.Fatalf("设置配置失败: %v", err)
	}

	// 使用适配器获取配置
	adapter := NewUserConfigAdapter(provider)
	retrievedConfig, err := adapter.GetUserConfig(ctx, userID)
	if err != nil {
		t.Fatalf("通过适配器获取配置失败: %v", err)
	}

	if retrievedConfig.SystemPrompt != userConfig.SystemPrompt {
		t.Errorf("适配器获取的配置不匹配，期望: %s, 实际: %s", userConfig.SystemPrompt, retrievedConfig.SystemPrompt)
	}
}

func TestDefaultConfig(t *testing.T) {
	// 测试Redis默认配置
	redisConfig := DefaultConfig("redis")
	if redisConfig["host"] != "localhost" {
		t.Errorf("Redis默认host配置错误，期望: localhost, 实际: %v", redisConfig["host"])
	}

	// 测试Memory默认配置
	memoryConfig := DefaultConfig("memory")
	if memoryConfig["max_entries"] != 1000 {
		t.Errorf("Memory默认max_entries配置错误，期望: 1000, 实际: %v", memoryConfig["max_entries"])
	}

	// 测试不支持的类型
	unknownConfig := DefaultConfig("unknown")
	if len(unknownConfig) != 0 {
		t.Errorf("未知类型应返回空配置，实际: %v", unknownConfig)
	}
}

func TestValidateConfig(t *testing.T) {
	// 测试有效的Redis配置
	validRedisConfig := map[string]interface{}{
		"host": "localhost",
		"port": 6379,
	}
	err := ValidateConfig("redis", validRedisConfig)
	if err != nil {
		t.Errorf("有效Redis配置验证失败: %v", err)
	}

	// 测试无效的Redis配置（缺少host）
	invalidRedisConfig := map[string]interface{}{
		"port": 6379,
	}
	err = ValidateConfig("redis", invalidRedisConfig)
	if err == nil {
		t.Error("缺少host的Redis配置应该验证失败")
	}

	// 测试Memory配置（无需验证）
	err = ValidateConfig("memory", map[string]interface{}{})
	if err != nil {
		t.Errorf("Memory配置验证失败: %v", err)
	}
}
