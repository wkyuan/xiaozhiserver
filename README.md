# 🚀 xiaozhi-esp32-server-golang

> **Xiaozhi AI Backend for ESP32 Devices**

---

## 项目简介 | Project Overview

xiaozhi-esp32-server-golang 是一款高性能、全流式的 AI 后端服务，专为物联网与智能语音场景设计。项目基于 Go 语言开发，集成了 ASR（自动语音识别）、LLM（大语言模型）、TTS（语音合成）等核心能力，支持大规模并发与多协议接入，助力智能终端与边缘设备的 AI 语音交互。

---

## ✨ 主要特性 | Key Features

- 🚀 **端到端全流式 AI 语音链路**：ASR（自动语音识别）、LLM（大语言模型）、TTS（语音合成）全流程流式处理，极致低延迟，适配实时语音交互场景。
- 🧩 **主逻辑代码梳理与优化**：对主流程代码结构进行系统性梳理与重构，提升可读性、可维护性与扩展性。
- 🛠️ **Transport 接口层抽象**：将 WebSocket、MQTT、UDP 等协议统一抽象为 Transport 接口层，灵活注入主逻辑，便于协议扩展与切换。
- 📬 **LLM/TTS 消息队列化处理**：LLM 与 TTS 处理流程采用消息队列方式，支持异步处理与新业务逻辑的灵活注入。
- 🔗 **多协议高并发接入**：内置 WebSocket、MQTT、UDP 等多种协议服务器，支持大规模设备并发接入与消息推送。
- ♻️ **高效资源池与连接复用**：外部资源连接池机制，显著降低响应耗时，提升系统吞吐能力。
- 🧠 **多引擎 AI 能力集成，基于 Eino 框架**：项目基于 Eino 框架开发，支持 FunASR、Eino LLM、OpenAI、Ollama、Doubao、EdgeTTS、CosyVoice 等多种主流 AI 引擎，灵活切换与扩展。
- 🛡️ **模块化与可扩展架构**：各核心能力（VAD/ASR/LLM/TTS/MCP/视觉）均为独立模块，便于定制、扩展和集成更多 AI 服务。
- 📦 **一键 Docker 部署 & 跨平台支持**：官方 Dockerfile，支持主流 Linux 发行版与本地编译，快速落地部署。
- 📊 **高性能与低资源占用**：Golang 原生高并发架构，基于 Eino 框架优化，适配边缘设备与云端，资源占用低，稳定性强。
- 🔒 **安全与权限体系（规划中）**：预留用户认证、权限管理接口，便于后续集成企业级安全体系。
- 👀 **视觉与多模态能力拓展**：支持视觉模型接入，满足多模态智能终端需求。

---

## 🚀 快速开始 | Quick Start

1. **Docker 一键部署**  
   [查看 Docker 快速开始文档 »](doc/docker.md)
2. **本地编译与运行（优化版）**

   1. **安装依赖**
      - 安装 Go 1.20+（建议与 Dockerfile 中一致的版本）
      - 安装 Opus 相关依赖：
        ```bash
        sudo apt-get update
        sudo apt-get install -y libopus0 libopusfile-dev
        ```
      - 安装 ONNX Runtime：
        ```bash
        wget https://github.com/microsoft/onnxruntime/releases/download/v1.21.0/onnxruntime-linux-x64-1.21.0.tgz
        tar -xzf onnxruntime-linux-x64-1.21.0.tgz
        sudo mkdir -p /usr/local/include/onnxruntime
        sudo cp -r onnxruntime-linux-x64-1.21.0/include/* /usr/local/include/onnxruntime/
        sudo cp -r onnxruntime-linux-x64-1.21.0/lib/* /usr/local/lib/
        sudo ldconfig
        ```
      - 设置环境变量（可写入 `~/.bashrc` 或 `~/.zshrc`）：
        ```bash
        export ONNXRUNTIME_DIR=/usr/local
        export CGO_CFLAGS="-I${ONNXRUNTIME_DIR}/include/onnxruntime"
        export CGO_LDFLAGS="-L${ONNXRUNTIME_DIR}/lib -lonnxruntime"
        ```

   2. **部署 FunASR 服务**
      - 参考 [FunASR 官方文档](https://github.com/modelscope/FunASR/blob/main/runtime/docs/SDK_advanced_guide_online_zh.md) 部署并启动服务。

   3. **编译服务**

      - 编译：
        ```bash
        go build -o xiaozhi_server ./cmd/server/
        ```

   4. **准备配置文件**
      - 复制或编辑 `config/config.json`，根据实际环境调整参数。
      - 详细配置说明请参考 [配置文档](doc/config.md)

   5. **启动服务**
      ```bash
      ./xiaozhi_server -c config/config.json
      ```

   ### 📚 相关文档 | Related Docs
   - [WebSocket 服务器与 OTA 配置说明 »](doc/websocket_server.md)
   - [MQTT+UDP 服务器配置流程 »](doc/mqtt_udp.md)
   - [MQTT UDP 协议与数据流程 »](doc/mqtt_udp_protocol.md)
   - [Vision 视觉识别 »](doc/vision.md)
   - [mcp 架构 »](doc/mcp.md)

   ---

   > ⚠️ 推荐在 Ubuntu 22.04 环境下操作，确保依赖一致。
   > 若遇到 ONNX Runtime 相关的 CGO 编译问题，请检查环境变量和依赖路径。
   > 日志和配置目录建议与 Docker 保持一致（`logs/`、`config/`）。

---

## 🧩 模块架构 | Module Overview

| 模块      | 功能简介                       | 技术栈/说明                |
|-----------|-------------------------------|----------------------------|
| VAD       | 声音活动检测（Silero VAD）    | Silero VAD, Webrtc vad                    |
| ASR       | 语音识别（FunASR对接）        | FunASR          |
| LLM       | 大语言模型（OpenAI兼容接口）  | Eino框架兼容的 LLM, openai, ollama       |
| TTS       | 语音合成（多引擎支持）        | Doubao, EdgeTTS, CosyVoice |
| MCP       | 多协议接入 | 支持全局MCP、MCP接入点、端侧MCP Server）       |
| 视觉      | 视觉处理相关能力                                    |  支持 doubao, aliyun 视觉模型      |

---

## 📈 性能与测试 | Performance & Testing

- [延迟测试报告](doc/delay_test.md)
- 高并发场景下稳定运行，资源占用低

---

## 🛠️ TODO & 规划
- [x] 完善 Docker 化部署
- [ ] 用户认证与权限体系
- [ ] 集成更多云厂商 ASR 服务
- [ ] Web 用户界面
- [ ] LLM 记忆体增强


---

## 🤝 贡献 | Contributing

欢迎提交 Issue、PR 或建议！如有合作意向请联系作者。

---

## 📄 License

本项目遵循 MIT License。

---

## 📬 联系方式 | Contact
交流群二维码

![微信图片_20250714093831(1)](https://github.com/user-attachments/assets/68364ce3-d507-4030-910e-fe1585fe5055)



个人微信：hackers365

![个人二维码_0618(1)](https://github.com/user-attachments/assets/6b8d3d11-7bf5-4fa4-a73e-5109019dab85)

---

> © 2024 xiaozhi-esp32-server-golang. All rights reserved.


