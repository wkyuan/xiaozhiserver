package silero_vad

import (
	"errors"
	"fmt"
	"sync"
	"time"

	. "xiaozhi-esp32-server-golang/internal/domain/vad/inter"

	log "xiaozhi-esp32-server-golang/logger"
)

// VADResourcePool VAD资源池管理，不与会话ID绑定
type VADResourcePool struct {
	// 可用的VAD实例队列
	availableVADs chan VAD
	// 已分配的VAD实例映射，用于跟踪和管理
	allocatedVADs sync.Map
	// 池大小配置
	maxSize int
	// 获取VAD超时时间（毫秒）
	acquireTimeout int64
	// 默认VAD配置
	defaultConfig map[string]interface{}
	// 互斥锁，用于初始化和重置操作
	mu sync.Mutex
	// 是否已初始化标志
	initialized bool
}

// initialize 初始化VAD资源池
func (p *VADResourcePool) initialize() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// 已经初始化过，先关闭现有资源
	if p.availableVADs != nil {
		close(p.availableVADs)
		p.availableVADs = nil

		// 释放所有已分配的VAD实例
		p.allocatedVADs.Range(func(key, value interface{}) bool {
			if sileroVAD, ok := value.(*SileroVAD); ok {
				sileroVAD.Close()
			}
			p.allocatedVADs.Delete(key)
			return true
		})
	}

	// 创建资源队列
	p.availableVADs = make(chan VAD, p.maxSize)

	// 预创建VAD实例
	for i := 0; i < p.maxSize; i++ {
		vadInstance, err := CreateVAD(p.defaultConfig)
		if err != nil {
			// 关闭已创建的实例
			for j := 0; j < i; j++ {
				vad := <-p.availableVADs
				if sileroVAD, ok := vad.(*SileroVAD); ok {
					sileroVAD.Close()
				}
			}
			close(p.availableVADs)
			p.availableVADs = nil

			return fmt.Errorf("预创建VAD实例失败: %v", err)
		}

		// 放入可用队列
		p.availableVADs <- vadInstance
	}

	log.Infof("VAD资源池初始化完成，创建了 %d 个VAD实例", p.maxSize)
	return nil
}

// AcquireVAD 从资源池获取一个VAD实例
func (p *VADResourcePool) AcquireVAD() (VAD, error) {
	if !p.initialized {
		return nil, errors.New("VAD资源池未初始化")
	}

	// 设置超时
	timeout := time.After(time.Duration(p.acquireTimeout) * time.Millisecond)

	log.Debugf("获取VAD实例, 当前可用: %d/%d", len(p.availableVADs), p.maxSize)

	// 尝试从池中获取一个VAD实例
	select {
	case vad := <-p.availableVADs:
		if vad == nil {
			return nil, errors.New("VAD资源池已关闭")
		}

		// 标记为已分配
		p.allocatedVADs.Store(vad, time.Now())

		log.Debugf("从VAD资源池获取了一个VAD实例，当前可用: %d/%d", len(p.availableVADs), p.maxSize)
		return vad, nil

	case <-timeout:
		return nil, fmt.Errorf("获取VAD实例超时，当前资源池已满载运行（%d/%d）", p.maxSize, p.maxSize)
	}
}

// ReleaseVAD 释放VAD实例回资源池
func (p *VADResourcePool) ReleaseVAD(vad VAD) {
	if vad == nil || !p.initialized {
		return
	}

	log.Debugf("释放VAD实例: %v, 当前可用: %d/%d", vad, len(p.availableVADs), p.maxSize)

	// 检查是否是从此池分配的实例
	if _, exists := p.allocatedVADs.Load(vad); exists {
		// 从已分配映射中删除
		p.allocatedVADs.Delete(vad)

		// 如果资源池已关闭，直接销毁实例
		if p.availableVADs == nil {
			if sileroVAD, ok := vad.(*SileroVAD); ok {
				sileroVAD.Close()
			}
			return
		}

		// 尝试放回资源池，如果满了就丢弃
		select {
		case p.availableVADs <- vad:
			log.Debugf("VAD实例已归还资源池，当前可用: %d/%d", len(p.availableVADs), p.maxSize)
		default:
			// 资源池满了，直接关闭实例
			if sileroVAD, ok := vad.(*SileroVAD); ok {
				sileroVAD.Close()
			}
			log.Warn("VAD资源池已满，多余实例已销毁")
		}
	} else {
		log.Warn("尝试释放非此资源池管理的VAD实例")
	}
}

// GetActiveCount 获取当前活跃（被分配）的VAD实例数量
func (p *VADResourcePool) GetActiveCount() int {
	count := 0
	p.allocatedVADs.Range(func(_, _ interface{}) bool {
		count++
		return true
	})
	return count
}

// GetAvailableCount 获取当前可用的VAD实例数量
func (p *VADResourcePool) GetAvailableCount() int {
	if p.availableVADs == nil {
		return 0
	}
	return len(p.availableVADs)
}

// Resize 调整资源池大小
func (p *VADResourcePool) Resize(newSize int) error {
	if newSize <= 0 {
		return errors.New("资源池大小必须大于0")
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	currentSize := p.maxSize

	// 如果新大小小于当前大小，需要减少实例数量
	if newSize < currentSize {
		// 更新大小配置
		p.maxSize = newSize

		// 计算需要释放的实例数量
		toRemove := currentSize - newSize
		for i := 0; i < toRemove; i++ {
			// 尝试从可用队列中取出实例并关闭
			select {
			case vad := <-p.availableVADs:
				if sileroVAD, ok := vad.(*SileroVAD); ok {
					sileroVAD.Close()
				}
			default:
				// 没有更多可用实例了，退出循环
				break
			}
		}

		log.Infof("VAD资源池大小已调整：%d -> %d", currentSize, newSize)
		return nil
	}

	// 如果新大小大于当前大小，需要增加实例数量
	if newSize > currentSize {
		// 计算需要增加的实例数量
		toAdd := newSize - currentSize

		// 创建新的VAD实例
		for i := 0; i < toAdd; i++ {
			vadInstance, err := CreateVAD(p.defaultConfig)
			if err != nil {
				// 有错误发生，更新大小为当前已成功创建的实例数
				actualNewSize := currentSize + i
				p.maxSize = actualNewSize

				log.Errorf("无法创建全部请求的VAD实例，资源池大小已调整为: %d", actualNewSize)
				return fmt.Errorf("创建新VAD实例失败: %v", err)
			}

			// 放入可用队列
			select {
			case p.availableVADs <- vadInstance:
				// 成功放入队列
			default:
				// 队列已满，直接关闭实例
				if sileroVAD, ok := vadInstance.(*SileroVAD); ok {
					sileroVAD.Close()
				}
				log.Warn("无法将新创建的VAD实例放入可用队列，实例已销毁")
			}
		}

		// 更新大小配置
		p.maxSize = newSize

		log.Infof("VAD资源池大小已调整：%d -> %d", currentSize, newSize)
		return nil
	}

	// 大小相同，无需调整
	return nil
}

// Close 关闭资源池，释放所有资源
func (p *VADResourcePool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.availableVADs != nil {
		// 关闭可用队列
		close(p.availableVADs)

		// 释放所有可用的VAD实例
		for vad := range p.availableVADs {
			if sileroVAD, ok := vad.(*SileroVAD); ok {
				sileroVAD.Close()
			}
		}

		p.availableVADs = nil
	}

	// 释放所有已分配的VAD实例
	p.allocatedVADs.Range(func(key, _ interface{}) bool {
		vad := key.(VAD)
		if sileroVAD, ok := vad.(*SileroVAD); ok {
			sileroVAD.Close()
		}
		p.allocatedVADs.Delete(key)
		return true
	})

	p.initialized = false
	log.Info("VAD资源池已关闭，所有资源已释放")
}

// GetVADResourcePool 获取全局VAD资源池实例
/*func GetVADResourcePool() (*VADResourcePool, error) {
	if globalVADResourcePool == nil || !globalVADResourcePool.initialized {
		// 尝试自动初始化
		if err := InitVADFromConfig(); err != nil {
			return nil, errors.New("VAD资源池未完全初始化，请在配置文件中设置 " + ConfigKeyVADModelPath)
		}
	}
	return globalVADResourcePool, nil
}
*/
