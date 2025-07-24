package websocket

import (
	"io"
	"net/http"
	"strings"
	"xiaozhi-esp32-server-golang/internal/app/server/chat"
	log "xiaozhi-esp32-server-golang/logger"

	"github.com/spf13/viper"
)

// handleVisionAPI 处理图片识别API
func (s *WebSocketServer) handleVisionAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "仅支持POST请求", http.StatusMethodNotAllowed)
		return
	}

	//从header头部获取Device-Id和Client-Id
	deviceId := r.Header.Get("Device-Id")
	clientId := r.Header.Get("Client-Id")
	_ = clientId
	if deviceId == "" {
		log.Errorf("缺少Device-Id")
		http.Error(w, "缺少Device-Id", http.StatusBadRequest)
		return
	}

	if viper.GetBool("vision.enable_auth") {

		//从header Authorization中获取Bearer token
		authToken := r.Header.Get("Authorization")
		if authToken == "" {
			log.Errorf("缺少Authorization")
			http.Error(w, "缺少Authorization", http.StatusBadRequest)
			return
		}
		authToken = strings.TrimPrefix(authToken, "Bearer ")

		err := chat.VisvionAuth(authToken)
		if err != nil {
			log.Errorf("图片识别认证失败: %v", err)
			http.Error(w, "图片识别认证失败", http.StatusUnauthorized)
			return
		}
	}

	// 解析 multipart 表单，最大 10MB
	question := r.FormValue("question")
	if question == "" {
		http.Error(w, "缺少question参数", http.StatusBadRequest)
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "缺少file参数或文件读取失败", http.StatusBadRequest)
		return
	}
	defer file.Close()

	fileBytes, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "文件读取失败", http.StatusInternalServerError)
		return
	}

	file.Close()

	result, err := chat.HandleVllm(deviceId, fileBytes, question)
	if err != nil {
		log.Errorf("图片识别失败: %v", err)
		http.Error(w, "图片识别失败", http.StatusInternalServerError)
		return
	}

	// TODO: 调用llm进行图片识别，输出识别内容
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(result))
}
