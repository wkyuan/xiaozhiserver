package chat

import (
	"sync"
	log "xiaozhi-esp32-server-golang/logger"
)

// ChatManagerRegistry 全局ChatManager注册表
type ChatManagerRegistry struct {
	managers map[string]*ChatManager
	mutex    sync.RWMutex
}

var (
	globalRegistry *ChatManagerRegistry
	registryOnce   sync.Once
)

// GetChatManagerRegistry 获取全局ChatManager注册表单例
func GetChatManagerRegistry() *ChatManagerRegistry {
	registryOnce.Do(func() {
		globalRegistry = &ChatManagerRegistry{
			managers: make(map[string]*ChatManager),
		}
	})
	return globalRegistry
}

// RegisterChatManager 注册ChatManager
func (r *ChatManagerRegistry) RegisterChatManager(deviceID string, manager *ChatManager) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	// 如果已存在，先关闭旧的
	if existingManager, exists := r.managers[deviceID]; exists {
		log.Warnf("设备 %s 已存在ChatManager，将关闭旧连接", deviceID)
		go existingManager.Close()
	}

	r.managers[deviceID] = manager
	log.Infof("注册ChatManager，设备ID: %s", deviceID)
}

// UnregisterChatManager 注销ChatManager
func (r *ChatManagerRegistry) UnregisterChatManager(deviceID string) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if _, exists := r.managers[deviceID]; exists {
		delete(r.managers, deviceID)
		log.Infof("注销ChatManager，设备ID: %s", deviceID)
	}
}

// GetChatManager 根据设备ID获取ChatManager
func (r *ChatManagerRegistry) GetChatManager(deviceID string) (*ChatManager, bool) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	manager, exists := r.managers[deviceID]
	return manager, exists
}

// GetAllDeviceIDs 获取所有已注册的设备ID
func (r *ChatManagerRegistry) GetAllDeviceIDs() []string {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	deviceIDs := make([]string, 0, len(r.managers))
	for deviceID := range r.managers {
		deviceIDs = append(deviceIDs, deviceID)
	}
	return deviceIDs
}

// GetManagerCount 获取当前注册的ChatManager数量
func (r *ChatManagerRegistry) GetManagerCount() int {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	return len(r.managers)
}

// CloseChatManager 根据设备ID关闭ChatManager
func (r *ChatManagerRegistry) CloseChatManager(deviceID string) error {
	r.mutex.RLock()
	manager, exists := r.managers[deviceID]
	r.mutex.RUnlock()

	if !exists {
		log.Warnf("设备 %s 的ChatManager不存在", deviceID)
		return nil
	}

	log.Infof("通过注册表关闭设备 %s 的ChatManager", deviceID)
	return manager.Close()
}
