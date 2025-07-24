package user_config

import (
	"fmt"

	userconfig_redis "xiaozhi-esp32-server-golang/internal/domain/config/redis"
)

// Config 用户配置提供者配置结构
type Config struct {
	Type       string                 `json:"type"`       // 存储类型: "redis", "memory", "file"
	Parameters map[string]interface{} `json:"parameters"` // 存储相关配置参数
}

func GetProvider() (UserConfigProvider, error) {
	provider, err := GetUserConfigProvider("redis", map[string]interface{}{})
	if err != nil {
		return nil, err
	}
	return provider, nil
}

// GetUserConfigProvider 创建用户配置提供者
// 根据传入的存储类型和配置参数创建对应的提供者实例
// providerType: 提供者类型，支持 "redis", "memory", "file"
// config: 提供者配置参数
// 返回UserConfigProvider接口，支持完整的CRUD操作
func GetUserConfigProvider(providerType string, config map[string]interface{}) (UserConfigProvider, error) {
	if config == nil {
		config = make(map[string]interface{})
	}

	switch providerType {
	case "redis":
		// 创建Redis用户配置提供者
		provider, err := userconfig_redis.NewRedisUserConfigProvider(config)
		if err != nil {
			return nil, fmt.Errorf("创建Redis用户配置提供者失败: %v", err)
		}
		return provider, nil
	case "file":
		// TODO: 创建文件用户配置提供者
		return nil, fmt.Errorf("文件用户配置提供者暂未实现")
	default:
		return nil, fmt.Errorf("不支持的用户配置提供者: %s", providerType)
	}
}
