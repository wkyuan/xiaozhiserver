package main

import (
	"flag"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"xiaozhi-esp32-server-golang/internal/app/server"
	log "xiaozhi-esp32-server-golang/logger"

	"github.com/spf13/viper"
)

func main() {
	// 解析命令行参数
	configFile := flag.String("c", "config/config.json", "配置文件路径")
	flag.Parse()

	if *configFile == "" {
		fmt.Println("配置文件路径不能为空")
		return
	}

	err := Init(*configFile)
	if err != nil {
		return
	}

	// 根据配置启动pprof服务
	if viper.GetBool("server.pprof.enable") {
		pprofPort := viper.GetInt("server.pprof.port")
		go func() {
			log.Infof("启动pprof服务，端口: %d", pprofPort)
			if err := http.ListenAndServe(fmt.Sprintf(":%d", pprofPort), nil); err != nil {
				log.Errorf("pprof服务启动失败: %v", err)
			}
		}()
		log.Infof("pprof地址: http://localhost:%d/debug/pprof/", pprofPort)
	} else {
		log.Info("pprof服务已禁用")
	}

	// 创建服务器
	appInstance := server.NewApp()
	appInstance.Run()

	// 阻塞监听退出信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	log.Info("服务器已启动，按 Ctrl+C 退出")
	<-quit

	log.Info("正在关闭服务器...")
	// TODO: 在这里添加清理资源的代码
	log.Info("服务器已关闭")
}
