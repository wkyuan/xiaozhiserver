package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"xiaozhi-esp32-server-golang/internal/app/server/auth"
	redisdb "xiaozhi-esp32-server-golang/internal/db/redis"

	rotatelogs "github.com/lestrrat-go/file-rotatelogs"
	logrus "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

func Init(configFile string) error {
	//init config
	err := initConfig(configFile)
	if err != nil {
		fmt.Printf("initConfig err: %+v", err)
		os.Exit(1)
		return err
	}

	//init log
	initLog()

	//init vad
	//initVad()

	//init redis
	initRedis()

	//init auth
	err = initAuthManager()
	if err != nil {
		fmt.Printf("initAuthManager err: %+v", err)
		os.Exit(1)
		return err
	}

	return nil
}

func initConfig(configFile string) error {
	basePath, file := filepath.Split(configFile)

	// 获取文件名和扩展名
	fileName, fileExt := func(file string) (string, string) {
		if pos := strings.LastIndex(file, "."); pos != -1 {
			return file[:pos], strings.ToLower(file[pos+1:])
		}
		return file, ""
	}(file)

	// 设置配置文件名(不带扩展名)
	viper.SetConfigName(fileName)
	viper.AddConfigPath(basePath)

	// 根据文件扩展名设置配置类型
	switch fileExt {
	case "json":
		viper.SetConfigType("json")
	case "yaml", "yml":
		viper.SetConfigType("yaml")
	default:
		return fmt.Errorf("unsupported config file type: %s", fileExt)
	}

	return viper.ReadInConfig()
}

func initLog() error {
	// 输出到文件
	binPath, _ := os.Executable()
	baseDir := filepath.Dir(binPath)
	logPath := fmt.Sprintf("%s/%s%s", baseDir, viper.GetString("log.path"), viper.GetString("log.file"))
	/* 日志轮转相关函数
	`WithLinkName` 为最新的日志建立软连接
	`WithRotationTime` 设置日志分割的时间，隔多久分割一次
	WithMaxAge 和 WithRotationCount二者只能设置一个
		`WithMaxAge` 设置文件清理前的最长保存时间
		`WithRotationCount` 设置文件清理前最多保存的个数
	*/
	// 下面配置日志每隔 1 分钟轮转一个新文件，保留最近 3 分钟的日志文件，多余的自动清理掉。
	writer, err := rotatelogs.New(
		logPath+".%Y%m%d",
		rotatelogs.WithLinkName(logPath),
		rotatelogs.WithRotationCount(uint(viper.GetInt("log.max_age"))),
		rotatelogs.WithRotationTime(time.Duration(86400)*time.Second),
	)
	if err != nil {
		fmt.Printf("init log error: %v\n", err)
		os.Exit(1)
		return err
	}

	// 根据配置决定输出目标
	if viper.GetBool("log.stdout") {
		// 同时输出到文件和标准输出
		multiWriter := io.MultiWriter(writer, os.Stdout)
		logrus.SetOutput(multiWriter)
		logrus.SetFormatter(&logrus.TextFormatter{
			TimestampFormat: "2006-01-02 15:04:05.000", //时间格式化，添加毫秒
			ForceColors:     true,                      // 标准输出启用颜色
		})
	} else {
		// 只输出到文件
		logrus.SetOutput(writer)
		logrus.SetFormatter(&logrus.TextFormatter{
			TimestampFormat: "2006-01-02 15:04:05.000", //时间格式化，添加毫秒
			ForceColors:     false,                     // 文件输出不启用颜色
		})
	}

	// 禁用默认的调用者报告，使用自定义的caller字段
	logrus.SetReportCaller(false)
	logLevel, _ := logrus.ParseLevel(viper.GetString("log.level"))
	logrus.SetLevel(logLevel)

	return nil
}

/*
	func initVad() error {
		err := vad.InitVAD()
		if err != nil {
			fmt.Printf("initVad error: %v\n", err)
			os.Exit(1)
			return err
		}
		return nil
	}
*/
func initRedis() error {
	// 初始化我们的统一Redis模块
	redisConfig := &redisdb.Config{
		Host:     viper.GetString("redis.host"),
		Port:     viper.GetInt("redis.port"),
		Password: viper.GetString("redis.password"),
		DB:       viper.GetInt("redis.db"),
	}

	err := redisdb.Init(redisConfig)
	if err != nil {
		fmt.Printf("init redis error: %v\n", err)
		return err
	}

	return nil
}

func initAuthManager() error {
	return auth.Init()
}
