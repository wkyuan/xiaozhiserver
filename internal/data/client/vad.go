package client

import (
	"fmt"
	"sync"
	"sync/atomic"
	"xiaozhi-esp32-server-golang/internal/domain/vad"
	vad_inter "xiaozhi-esp32-server-golang/internal/domain/vad/inter"
)

type Vad struct {
	lock sync.RWMutex
	// VAD 提供者
	VadProvider vad_inter.VAD

	IdleDuration int64 // 空闲时间, 单位: ms
}

func (v *Vad) AddIdleDuration(idleDuration int64) int64 {
	return atomic.AddInt64(&v.IdleDuration, idleDuration)
}

func (v *Vad) GetIdleDuration() int64 {
	return atomic.LoadInt64(&v.IdleDuration)
}

func (v *Vad) ResetIdleDuration() {
	atomic.StoreInt64(&v.IdleDuration, 0)
}

func (v *Vad) Init(provider string, config map[string]interface{}) error {
	v.lock.Lock()
	defer v.lock.Unlock()
	vadProvider, err := vad.AcquireVAD(provider, config)
	if err != nil {
		return fmt.Errorf("创建 VAD 提供者失败: %v", err)
	}

	vadProvider.Reset()
	v.VadProvider = vadProvider
	return nil
}

func (v *Vad) Reset() error {
	v.lock.Lock()
	defer v.lock.Unlock()
	if v.VadProvider != nil {
		vad.ReleaseVAD(v.VadProvider) //释放vad实例资源
		v.VadProvider = nil           //置nil
	}
	v.ResetIdleDuration()
	return nil
}
