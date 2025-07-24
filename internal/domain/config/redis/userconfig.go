package redis_config

import (
	"context"
	"encoding/json"
	"fmt"

	log "xiaozhi-esp32-server-golang/logger"

	i_redis "xiaozhi-esp32-server-golang/internal/db/redis"
	"xiaozhi-esp32-server-golang/internal/domain/config/types"

	"github.com/redis/go-redis/v9"
	"github.com/spf13/viper"
)

// RedisUserConfigProvider Redis用户配置提供者
// 实现UserConfigProvider接口，支持Redis存储
type RedisUserConfigProvider struct {
	UserConfig
}

type UserConfig struct {
	redisInstance *redis.Client
	prefix        string
}

// NewRedisUserConfigProvider 创建Redis用户配置提供者
// config: 配置参数map，包含host, port, password, db, prefix等
func NewRedisUserConfigProvider(config interface{}) (*RedisUserConfigProvider, error) {
	provider := &RedisUserConfigProvider{
		UserConfig: UserConfig{
			redisInstance: i_redis.GetClient(),
			prefix:        viper.GetString("redis.key_prefix"),
		},
	}

	log.Log().Info("Redis用户配置提供者初始化成功")
	return provider, nil
}

func (u *UserConfig) GetUserConfig(ctx context.Context, userID string) (types.UConfig, error) {
	redisConfig := map[string]string{}

	if u.redisInstance != nil {
		key := u.GetUserConfigKey(userID)

		//hgetall 拿到所有的
		var err error
		redisConfig, err = u.redisInstance.HGetAll(ctx, key).Result()
		if err != nil {
			return types.UConfig{}, err
		}
	}

	ret := types.UConfig{}
	//将UserConfig转换成UConfig结构
	kv := map[string]string{
		"llm": "",
		"asr": "",
		"tts": "",
	}
	for k, _ := range kv {
		var redisPerConfig map[string]interface{}
		if rv, ok := redisConfig[k]; ok {
			if rv != "" {
				err := json.Unmarshal([]byte(rv), &redisPerConfig)
				if err != nil {
					log.Log().Errorf("redis config unmarshal error: %+v", err)
				}
			}
		}

		if k == "llm" {
			config, err := u.getLlmConfig(ctx, redisPerConfig)
			if err != nil {
				return types.UConfig{}, err
			}
			ret.Llm = config
		} else if k == "tts" {
			config, err := u.getTtsConfig(ctx, redisPerConfig)
			if err != nil {
				return types.UConfig{}, err
			}
			ret.Tts = config
		} else if k == "asr" {
			config, err := u.getAsrConfig(ctx, redisPerConfig)
			if err != nil {
				return types.UConfig{}, err
			}
			ret.Asr = config
		}
	}
	ret.Vad = u.getVadConfig(ctx)

	log.Log().Infof("userconfig: %+v", ret)
	return ret, nil
}

func (u *UserConfig) getVadConfig(ctx context.Context) types.VadConfig {
	provider := viper.GetString("vad.provider")
	return types.VadConfig{
		Provider: provider,
		Config:   viper.GetStringMap(fmt.Sprintf("vad.%s", provider)),
	}
}

func (u *UserConfig) getConfigByType(ctx context.Context, config map[string]interface{}, prefix string) (string, map[string]interface{}, error) {
	provider := viper.GetString(prefix + ".provider")
	if _, ok := config[provider]; !ok {
		if iProvider, ok := config["provider"].(string); ok {
			provider = iProvider
		}
	}

	commonConfig := viper.GetStringMap(prefix + "." + provider)

	for k, v := range config {
		if k == "provider" {
			continue
		}
		commonConfig[k] = v
	}
	return provider, commonConfig, nil
}

func (u *UserConfig) getLlmConfig(ctx context.Context, config map[string]interface{}) (types.LlmConfig, error) {
	provider, commonConfig, err := u.getConfigByType(ctx, config, "llm")
	if err != nil {
		return types.LlmConfig{}, err
	}
	return types.LlmConfig{
		Provider: provider,
		Config:   commonConfig,
	}, nil
}
func (u *UserConfig) getAsrConfig(ctx context.Context, config map[string]interface{}) (types.AsrConfig, error) {
	provider, commonConfig, err := u.getConfigByType(ctx, config, "asr")
	if err != nil {
		return types.AsrConfig{}, err
	}
	return types.AsrConfig{
		Provider: provider,
		Config:   commonConfig,
	}, nil
}
func (u *UserConfig) getTtsConfig(ctx context.Context, config map[string]interface{}) (types.TtsConfig, error) {
	provider, commonConfig, err := u.getConfigByType(ctx, config, "tts")
	if err != nil {
		return types.TtsConfig{}, err
	}
	return types.TtsConfig{
		Provider: provider,
		Config:   commonConfig,
	}, nil
}
func (u *UserConfig) GetUserConfigKey(deviceId string) string {
	return fmt.Sprintf("%s:userconfig:%s", u.prefix, deviceId)
}
