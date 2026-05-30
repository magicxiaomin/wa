# API_CONTRACT_ANDROID · Android 接口扩展

> 历史说明：本文最初来自第二波 Android 接口扩展。Wave 4 MVP research mode 已经改变 trace/debug
> 语义：trace 可以记录完整 JID、完整号码、消息正文、QR/pairing code、session/auth 调试材料。
> 当前审计以 `SPEC_WAVE4.md`、`ACCEPTANCE_WAVE4.md`、`TRACE_SCHEMA.md` 为准。

延续第一波 API_CONTRACT，本波新增"收消息"和"取联系人"，并定义 Kotlin 侧如何对接。

## Go wrapper 新增方法

```go
// 获取联系人列表。返回 JSON 字符串（遵守 gomobile 类型约束，不直接返回 struct slice）。
// JSON 形如: [{"jid":"...@s.whatsapp.net","name":"张三"}, ...]
GetContacts() (string, error)

// 解析/校验一个号码是否在 WhatsApp 上，返回规范 JID（发消息前用）
ResolveJID(phone string) (string, error)
```

发送方法沿用第一波的 `SendTextForTest(to, text, clientMsgId)`，
本波可改名为 `SendText(to, text, clientMsgId)`（去掉 ForTest，因为这次是真实收发），
但**保留内部的频率/计数节流**。

## 新增事件类型（延续第一波事件枚举）

接收相关：
- `message_received` — 收到 1 对 1 文本消息。payload:
  `{"from_jid":"...","from_suffix":"...3000","text":"...","text_len":12,"server_msg_id":"...","ts":"..."}`
  Wave 4 MVP research mode 下，给 UI 的实时回调和导出的 trace/debug 都可以包含正文和完整 JID，
  用于研究 whatsmeow Android 接口行为。该 trace/debug 只用于本机研究，不发布、不外发。
- `contacts_synced` — 联系人同步完成/更新，UI 可重新拉取
- `receipt` — 送达/已读回执（尽力记录）

## Kotlin 侧对接（gomobile 生成的绑定）

gomobile bind 会生成一个 Java/Kotlin 可调用的包。回调通过 interface 实现：

```kotlin
// Go 侧定义的回调 interface，Kotlin 实现它
interface EventCallback {
    fun onEvent(eventType: String, payloadJson: String)
}

// 调用示例
val client = Wamobile.newClient(callbackImpl, dataDir, "WA-Android")
client.start()
client.connect()
val contactsJson = client.getContacts()  // 解析 JSON 成列表给 UI
```

⚠️ **跨线程铁律**：`onEvent` 在 whatsmeow 的 goroutine 里被调用，**不在 Android 主线程**。
Kotlin 实现里**必须**把 UI 更新切回主线程：
```kotlin
override fun onEvent(type: String, payload: String) {
    Handler(Looper.getMainLooper()).post {
        // 这里才能安全更新 UI / LiveData / StateFlow
    }
}
```
不切主线程直接更新 UI = 崩溃。这是 Android 阶段第一个会踩的坑。

## 跨进程注意（因为引擎在 :wa_bridge 进程）
- 上面的 wrapper 调用发生在 `:wa_bridge` 进程内。
- 主进程 UI 要拿到这些数据/事件，需经 AIDL IPC（见 SPEC Phase D）。
- 即：`onEvent` 先在 Bridge 进程收到 → 通过 AIDL 回调转发到主进程 → 再切主线程更新 UI。
- 跨进程传递的也只能是基本类型/String（AIDL 限制），所以继续用 JSON 字符串传 payload。
