package webrtc_vad

import "xiaozhi-esp32-server-golang/internal/util"

func getPoolConfigFromMap(config map[string]interface{}) *util.PoolConfig {
	poolConfig := util.DefaultConfig()
	if config["pool_min_size"] != nil {
		if minSize, ok := config["pool_min_size"].(int); ok {
			poolConfig.MinSize = minSize
		}
	}
	if config["pool_max_size"] != nil {
		if maxSize, ok := config["pool_max_size"].(int); ok {
			poolConfig.MaxSize = maxSize
		}
	}
	if config["pool_max_idle"] != nil {
		if maxIdle, ok := config["pool_max_idle"].(int); ok {
			poolConfig.MaxIdle = maxIdle
		}
	}
	return poolConfig
}

func getVadConfigFromMap(config map[string]interface{}) WebRTCVADConfig {
	sampleRate := DefaultSampleRate
	mode := DefaultMode

	if val, ok := config["vad_sample_rate"]; ok {
		if sampleRate, ok := val.(int); ok {
			sampleRate = sampleRate
		}
	}
	if val, ok := config["vad_mode"]; ok {
		if mode, ok := val.(int); ok {
			mode = mode
		}
	}
	return WebRTCVADConfig{
		SampleRate: sampleRate,
		Mode:       mode,
	}
}
