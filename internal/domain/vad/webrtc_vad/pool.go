package webrtc_vad

import (
	"fmt"
	"time"
	"xiaozhi-esp32-server-golang/internal/domain/vad/inter"
	"xiaozhi-esp32-server-golang/internal/util"
)

// WebRTCVADPool WebRTC VAD 资源池管理器
type WebRTCVADPool struct {
	pool *util.ResourcePool
}

// NewWebRTCVADPool 创建WebRTC VAD资源池
func NewWebRTCVADPool(config WebRTCVADConfig, poolConfig *util.PoolConfig) (*WebRTCVADPool, error) {
	if poolConfig == nil {
		poolConfig = util.DefaultConfig()
		// 为VAD设置合适的默认值
		poolConfig.MaxSize = 5
		poolConfig.MinSize = 1
		poolConfig.MaxIdle = 3
		poolConfig.IdleTimeout = 2 * time.Minute
	}

	factory := NewWebRTCVADFactory(config)
	pool, err := util.NewResourcePool(poolConfig, factory)
	if err != nil {
		return nil, fmt.Errorf("failed to create WebRTC VAD pool: %w", err)
	}

	return &WebRTCVADPool{
		pool: pool,
	}, nil
}

// AcquireVAD 获取VAD实例
func (p *WebRTCVADPool) AcquireVAD() (inter.VAD, error) {
	resource, err := p.pool.Acquire()
	if err != nil {
		return nil, err
	}

	vad, ok := resource.(*WebRTCVAD)
	if !ok {
		p.pool.Release(resource)
		return nil, fmt.Errorf("invalid resource type")
	}

	return vad, nil
}

// ReleaseVAD 释放VAD实例
func (p *WebRTCVADPool) ReleaseVAD(vad inter.VAD) error {
	webrtcVAD, ok := vad.(*WebRTCVAD)
	if !ok {
		return fmt.Errorf("invalid VAD type")
	}

	return p.pool.Release(webrtcVAD)
}

// Close 关闭资源池
func (p *WebRTCVADPool) Close() error {
	return p.pool.Close()
}

// Stats 获取资源池统计信息
func (p *WebRTCVADPool) Stats() map[string]interface{} {
	return p.pool.Stats()
}
