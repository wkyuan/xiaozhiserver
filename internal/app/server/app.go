package server

import (
	"xiaozhi-esp32-server-golang/internal/app/mqtt_server"
	"xiaozhi-esp32-server-golang/internal/app/server/chat"
	"xiaozhi-esp32-server-golang/internal/app/server/mqtt_udp"
	"xiaozhi-esp32-server-golang/internal/app/server/types"
	"xiaozhi-esp32-server-golang/internal/app/server/websocket"
	"xiaozhi-esp32-server-golang/internal/domain/mcp"
	log "xiaozhi-esp32-server-golang/logger"

	"github.com/spf13/viper"
)

// App 统一管理所有协议服务和 ChatManager

type App struct {
	wsServer       *websocket.WebSocketServer
	mqttUdpAdapter *mqtt_udp.MqttUdpAdapter
}

func NewApp() *App {
	var err error
	app := &App{}

	// 设置退出对话函数
	mcp.SetExitChatFunc(func(deviceID string) error {
		registry := chat.GetChatManagerRegistry()
		return registry.CloseChatManager(deviceID)
	})

	app.wsServer = app.newWebSocketServer()
	app.mqttUdpAdapter, err = app.newMqttUdpAdapter()
	if err != nil {
		log.Errorf("newMqttUdpAdapter err: %+v", err)
		return nil
	}
	return app
}

func (a *App) Run() {
	go a.wsServer.Start()
	if a.mqttUdpAdapter != nil {
		go a.mqttUdpAdapter.Start()
	}
	if viper.GetBool("mqtt_server.enable") {
		go func() {
			err := a.startMqttServer()
			if err != nil {
				log.Errorf("startMqttServer err: %+v", err)
			}
		}()
	}
	select {} // 阻塞主线程
}

func (app *App) newMqttUdpAdapter() (*mqtt_udp.MqttUdpAdapter, error) {
	isEnableUdp := viper.GetBool("mqtt.enable")
	if !isEnableUdp {
		return nil, nil
	}
	mqttConfig := mqtt_udp.MqttConfig{
		Broker:   viper.GetString("mqtt.broker"),
		Type:     viper.GetString("mqtt.type"),
		Port:     viper.GetInt("mqtt.port"),
		ClientID: viper.GetString("mqtt.client_id"),
		Username: viper.GetString("mqtt.username"),
		Password: viper.GetString("mqtt.password"),
	}

	udpServer, err := app.newUdpServer()
	if err != nil {
		return nil, err
	}

	return mqtt_udp.NewMqttUdpAdapter(
		&mqttConfig,
		mqtt_udp.WithUdpServer(udpServer),
		mqtt_udp.WithOnNewConnection(app.OnNewConnection),
	), nil
}

func (app *App) newUdpServer() (*mqtt_udp.UdpServer, error) {
	udpPort := viper.GetInt("udp.listen_port")
	externalHost := viper.GetString("udp.external_host")
	externalPort := viper.GetInt("udp.external_port")

	udpServer := mqtt_udp.NewUDPServer(udpPort, externalHost, externalPort)
	err := udpServer.Start()
	if err != nil {
		log.Fatalf("udpServer.Start err: %+v", err)
		return nil, err
	}
	return udpServer, nil
}

func (app *App) newWebSocketServer() *websocket.WebSocketServer {
	port := viper.GetInt("websocket.port")
	return websocket.NewWebSocketServer(port, websocket.WithOnNewConnection(app.OnNewConnection))
}

func (app *App) startMqttServer() error {
	return mqtt_server.StartMqttServer()
}

// 所有协议新连接都走这里
func (a *App) OnNewConnection(transport types.IConn) {
	deviceID := transport.GetDeviceID()

	//need delete
	chatManager, err := chat.NewChatManager(deviceID, transport)
	if err != nil {
		log.Errorf("创建chatManager失败: %v", err)
		return
	}

	// 注册ChatManager到全局注册表
	registry := chat.GetChatManagerRegistry()
	registry.RegisterChatManager(deviceID, chatManager)

	// 设置连接关闭时的清理回调
	transport.OnClose(func(deviceId string) {
		registry.UnregisterChatManager(deviceId)
	})

	go chatManager.Start()
}
