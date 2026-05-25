# API_CONTRACT · Wrapper 接口与事件契约

即使本阶段是桌面 PoC，也请**按这个接口来组织代码**（实现成一个 Go package / struct）。原因：后续 Android 阶段会把同一个 wrapper 用 gomobile 编译，接口稳定能最大化复用、避免返工。

但注意：**PoC 阶段不要为这个接口做多余的抽象层或框架**。就是一个简单的 struct + 方法 + 一个回调函数。保持朴素。

---

## 要实现的方法

```go
// 创建客户端。callback 用于异步上报所有事件（见下方事件类型）。
// dataDir: session / store 的本地存放目录
// deviceName: 在 WhatsApp"已连接的设备"里显示的名字
NewClient(callback EventCallback, dataDir string, deviceName string) (*Client, error)

Start() error            // 初始化 store、挂载事件处理器（不建立网络连接）
Stop() error             // 优雅停止：取消 context、关闭连接、释放 goroutine
Connect() error          // 建立到 WhatsApp 的连接（已有 session 则恢复，无则触发 QR）
Disconnect() error       // 断开网络连接，但保留 session
RequestPairing() error   // （可选）请求 pairing code 方式登录，替代扫码
GetState() string        // 返回当前连接状态（与事件里的状态枚举一致）

// 发送测试消息。必须内部校验：to 在白名单内 + 未超发送计数上限。
// clientMsgId: 调用方生成的幂等 ID，用于在事件回调里关联这条消息的后续状态。
SendTextForTest(to string, text string, clientMsgId string) error

ExportTrace(path string) error  // 导出 trace.json（导出前脱敏）
ClearSession() error            // 清除本地 session，下次需重新扫码
```

## 回调签名

```go
// 所有事件通过这一个回调上报。payload 是 JSON 字符串（见 GOMOBILE_CONSTRAINTS 为何用 JSON）。
type EventCallback func(eventType string, payloadJSON string)
```

⚠️ **回调线程注意**：whatsmeow 事件在其内部 goroutine 触发，因此 callback **不在主 goroutine / 不在 Android 主线程**。PoC 阶段桌面程序影响小，但请在代码注释里**明确标注这一点**，因为 Android 阶段 Kotlin 侧必须切回主线程才能更新 UI，否则崩溃。

---

## 事件类型（eventType 取值）

连接生命周期：
- `bridge_started`
- `qr_generated`（payload 含二维码内容）
- `pairing_code_generated`（payload 含 pairing code）
- `paired`
- `connecting`
- `connected`
- `disconnected`（payload 含原因 / 是否会重连）
- `session_restored`（重启后用旧 session 成功恢复）
- `session_invalid`（session 失效，需重新登录）

发送生命周期：
- `message_send_start`（payload 含 clientMsgId）
- `message_sent`（payload 含 clientMsgId + 服务器返回的 message ID）
- `message_ack`（送达/已读回执——**尽力记录，拿不到不算失败**）
- `message_failed`（payload 含 clientMsgId + 错误原因）

其它：
- `error`（payload 含 where + message）

---

## 状态枚举（GetState 返回值，与事件对齐）
`initializing` / `waiting_qr` / `connecting` / `connected` / `disconnected` / `logged_out`

---

## 设计约束
- 每个导出方法内部都要有 panic recover（见 KNOWN_PITFALLS 第 3 条）
- `SendTextForTest` 的白名单与计数校验在方法内部完成，调用方无法绕过
- `GetState()` 的返回值必须来自同一个状态机，不要在多处各自维护状态
