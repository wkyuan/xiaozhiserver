# 🚦 数据流程

1. **调用 OTA 接口**
   - 获取 **MQTT**、**WebSocket** 地址

2. **连接 MQTT**
   - 发送 `hello` 消息，获取：
     - 🎵 `audio_params`
     - 🌐 UDP 服务器地址
     - 🔑 `aes_key`
     - 🧩 `nonce`

3. **连接 UDP 服务器**
   - 进行语音数据的发送与接收

---

# 🛠️ 服务端流程

| 步骤 | 说明 |
| :--- | :--- |
| 1. MQTT 服务 | 生成 `aes_key`、`nonce`，并与 `device_id`、`client_id` 关联 |
| 2. MQTT 消息监听 | 收到 `type: listen, state: start` 时，初始化 `clientState` 结构，状态为 `start` |
| 3. UDP 服务 | 收到包后解析 `nonce`，查找对应 `clientState`，填充远程地址，状态为 `recv` |
| 4. 停止接收 | 收到 `type: listen, state: stop` 或自动检测无声音时，停止接收 |

---

# 🔗 关联关系

- OTA 验证 **MAC 地址** 和 **clientId**，并关联到 **uid**
- OTA 下发的 **MQTT 地址** 和 **mqtt_clientId** 关联 **MAC 地址** 和 **clientId**
- 通过 **MQTT 连接** 可解析出 **clientId** 和 **MAC 地址**
- 通过 **MQTT hello 消息** 可关联到 `aes_key`、`nonce`
- 通过 **UDP 音频消息** 可关联到 `nonce`

---

> **说明：**
> - `clientState` 结构用于维护每个客户端的会话状态和资源。
> - `nonce` 是客户端与服务端之间的唯一标识，用于安全关联和数据路由。
