package mqtt_server

import (
	"crypto/tls"
	"errors"
	"fmt"

	mqttServer "github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/listeners"
	"github.com/spf13/viper"

	log "xiaozhi-esp32-server-golang/logger"
)

func StartMqttServer() error {
	Server := mqttServer.New(&mqttServer.Options{
		InlineClient: true,
	})

	err := Server.AddHook(&AuthHook{}, nil)
	if err != nil {
		log.Fatalf("添加 AuthHook 失败: %v", err)
		return err
	}

	// 添加设备钩子
	deviceHook := &DeviceHook{server: Server}
	err = Server.AddHook(deviceHook, nil)
	if err != nil {
		log.Fatalf("添加 DeviceHook 失败: %v", err)
		return err
	}

	// 启动周期性打印订阅主题的任务（每10秒打印一次）
	//deviceHook.StartPeriodicSubscriptionPrinter(10 * time.Second)
	enableTLS := viper.GetBool("mqtt_server.tls.enable")
	if enableTLS {
		pemFile := viper.GetString("mqtt_server.tls.pem")
		keyFile := viper.GetString("mqtt_server.tls.key")
		cert, err := tls.LoadX509KeyPair(pemFile, keyFile)

		if err != nil {
			log.Fatalf("加载证书失败: %v", err)
			return err
		}

		tlsConfig := &tls.Config{
			Certificates: []tls.Certificate{cert},
		}
		ssltcp := listeners.NewTCP(listeners.Config{
			ID:        "ssl",
			Address:   fmt.Sprintf(":%d", viper.GetInt("mqtt_server.tls.port")),
			TLSConfig: tlsConfig,
		})
		err = Server.AddListener(ssltcp)
		if err != nil {
			log.Fatal(err)
		}
	}

	host := viper.GetString("mqtt_server.listen_host")
	port := viper.GetInt("mqtt_server.listen_port")
	if port == 0 {
		log.Errorf("mqtt_server.port 配置错误，请检查配置文件")
		return errors.New("mqtt_server.port 配置错误，请检查配置文件")
	}

	// 使用配置中的端口号
	address := fmt.Sprintf("%s:%d", host, port)
	tcp := listeners.NewTCP(listeners.Config{
		Type:    "tcp",
		ID:      "t1",
		Address: address,
	})
	err = Server.AddListener(tcp)
	if err != nil {
		log.Fatalf("添加 TCP 监听失败: %v", err)
	}

	log.Infof("MQTT 服务器启动，监听 %s 地址...", address)

	err = Server.Serve()
	if err != nil {
		log.Fatalf("MQTT 服务器启动失败: %v", err)
		return err
	}
	return nil
}
