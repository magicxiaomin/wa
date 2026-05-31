# Android SDK API

本文件说明 `:wa-sdk` 对外暴露的 Kotlin SDK。集成方只依赖 `WaBridgeClient`，不直接访问底层 AAR、AIDL Stub 或内部 Service。

## 交付物

- 底层协议 AAR：`android/wa-sdk/libs/wamobile.aar`
- 可集成 SDK AAR：`android/wa-sdk/build/outputs/aar/wa-sdk-release.aar`
- SDK 客户端：`android/wa-sdk/src/main/java/com/magicxiaomin/wa/sdk/WaBridgeClient.kt`
- AIDL 契约：`android/wa-sdk/src/main/aidl/com/magicxiaomin/wa/bridge/`
- 示例 App：`android/sample-app`

## 最小集成

1. 在 Android 工程中引入 `wa-sdk-release.aar`。
2. 合并 SDK Manifest。SDK 已声明前台服务、独立 `:wa_bridge` 进程和所需权限。
3. Android 13+ 由 App 申请 `POST_NOTIFICATIONS`。
4. 创建并绑定客户端：

```kotlin
val client = WaBridgeClient(context, deviceName = "MyDevice")
client.setEventListener { eventType, payloadJson ->
    // 这里已在 Main Looper，可直接更新 UI。
}
client.bind()
```

5. 登录、取状态、调用业务 API；退出页面时调用 `client.unbind()`。

## 线程与错误

- `WaBridgeClient.setEventListener` 的回调总是在 Main Looper 触发。
- SDK 方法是同步调用。耗时操作建议由集成方放到后台线程。
- 跨进程错误会包装成 `WaBridgeException`，不会直接向集成方抛 `RemoteException`。
- JSON 入参必须是合法 JSON string；复杂返回值也都以 JSON string 返回。

## 事件

事件统一形态：

```json
{
  "eventType": "message_received",
  "payloadJson": "{}"
}
```

常见 `eventType`：

- `bridge_started`
- `qr_generated`
- `connected`
- `session_restored`
- `message_received`
- `message_sent`
- `message_failed`
- `message_ack`
- `presence_update`
- `error`

### 发送与已读事件 payload

`message_sent` 表示底层已接受发送请求，并返回可用于后续回执关联的 `server_msg_id`：

```json
{
  "clientMsgId": "client-msg-id",
  "server_msg_id": "3EB0...",
  "latency_ms": 320,
  "recipient_jid": "123@s.im.net",
  "recipient_server": "s.im.net",
  "used_lid": false
}
```

`message_ack` 表示同一个 `server_msg_id` 收到送达/已读/播放回执：

```json
{
  "server_msg_id": "3EB0...",
  "ack_level": 2,
  "latency_ms": 1500
}
```

`ack_level` 取值：

- `1`：已送达或默认 receipt。
- `2`：已读。
- `3`：已播放，主要用于语音等可播放消息。

集成方可以用 `message_sent.server_msg_id` 建立本地发送记录映射，再用 `message_ack.server_msg_id` 找回对应消息；当 `ack_level >= 2` 时可视为已读或更高等级回执。

## API

### bind / unbind

```kotlin
client.bind()
client.unbind()
```

启动并绑定 SDK 内部前台服务。`bind()` 会按需启动独立进程服务。

### connectBridge

```kotlin
client.connectBridge()
```

开始连接协议端。首次登录会通过事件返回二维码：

```json
{"qr":"...","qr_len":123,"timeout_seconds":20}
```

### disconnectBridge

```kotlin
client.disconnectBridge()
```

断开当前连接，不删除本地会话。

### clearSession

```kotlin
client.clearSession()
```

停止连接并删除本地会话目录。调用后需要重新连接并重新完成登录。

### getState

```kotlin
val state = client.getState()
```

返回字符串状态，例如 `initializing`、`waiting_qr`、`connected`、`disconnected`。

### getSafetyStatus

```kotlin
val json = client.getSafetyStatus()
```

返回运行保护状态 JSON：

```json
{
  "risk_stopped": false,
  "risk_retry_after_seconds": 0,
  "fresh_contacts_retry_after_seconds": 0,
  "fresh_send_retry_after_seconds": 0,
  "operation_retry_after_seconds": 0
}
```

### getSelfIdentity

```kotlin
val json = client.getSelfIdentity()
```

返回当前登录身份与连接状态：

```json
{
  "self_jid": "123@s.im.net",
  "jid_server": "s.im.net",
  "state": "connected",
  "is_logged_in": true,
  "is_connected": true,
  "has_session_db": true,
  "device_name": "MyDevice"
}
```

### getContacts

```kotlin
val json = client.getContacts()
```

返回联系人数组：

```json
[
  {"jid":"123@s.im.net","name":"Alice","short_name":"Alice","notify_name":"Alice"}
]
```

### getGroups

```kotlin
val json = client.getGroups()
```

返回已加入群数组：

```json
[
  {"jid":"123@g.us","name":"Study","participant_count":3}
]
```

### resolveJID

```kotlin
val jid = client.resolveJID("123456")
```

把号码或已有 JID 解析成可发送目标 JID。

### getUserInfo

```kotlin
val input = """["123@s.im.net","456@s.im.net"]"""
val json = client.getUserInfo(input)
```

返回数组，每个元素对应一个查询目标：

```json
[
  {
    "jid": "123@s.im.net",
    "status": "hello",
    "picture_id": "abc",
    "verified_name": "Alice",
    "found": true,
    "error": ""
  }
]
```

### getProfilePictureInfo

```kotlin
val json = client.getProfilePictureInfo("123@s.im.net")
```

返回头像信息；取不到时 `found=false` 并给出 `error`：

```json
{
  "jid": "123@s.im.net",
  "url": "https://example.invalid/avatar",
  "id": "avatar-id",
  "type": "preview",
  "found": true,
  "error": ""
}
```

### sendText

```kotlin
client.sendText("123@s.im.net", "hello", "client-msg-id")
```

向单个联系人或单个群发送文本。结果通过 `message_sent` / `message_failed` / `message_ack` 事件返回；其中 `message_sent.server_msg_id` 与后续 `message_ack.server_msg_id` 对应。

### sendTextMulti

```kotlin
val targets = """["123@s.im.net","456@s.im.net"]"""
val json = client.sendTextMulti(targets, "hello", "client-msg-id")
```

向多个联系人发送同一文本。返回逐个目标结果数组：

```json
[
  {"jid":"123@s.im.net","ok":true,"server_msg_id":"ABC","error":""},
  {"jid":"456@s.im.net","ok":false,"server_msg_id":"","error":"send failed"}
]
```

### markRead

```kotlin
client.markRead(
    chatJid = "123@s.im.net",
    messageIdsJson = """["MSG1","MSG2"]""",
    senderJid = "123@s.im.net"
)
```

把指定会话内的消息标记为已读。成功时会发出 `mark_read_success` 事件。

### sendPresence

```kotlin
client.sendPresence("available")
client.sendPresence("unavailable")
```

发送在线状态。只接受 `available` 或 `unavailable`。

### subscribePresence

```kotlin
client.subscribePresence("123@s.im.net")
```

订阅某个联系人在线状态。后续变化通过 `presence_update` 事件返回。

### exportTrace

```kotlin
client.exportTrace(context.filesDir.resolve("trace.json").absolutePath)
```

导出本机运行 trace 到 App 私有目录。不会上传网络。
路径必须位于当前 App 的 `filesDir` 下，否则 SDK 内部服务会拒绝。
`filesDir` 路径限制由 SDK Service 层（`privateFilesPath`）提供；直连 AAR、不经 SDK Service 的调用方需自行约束导出路径。

### exportSessionDebug

```kotlin
client.exportSessionDebug(context.filesDir.resolve("session-debug").absolutePath)
```

导出本机会话调试包，目录中包含：

- `trace.json`
- `session-debug.json`

该目录只用于本机研究和人工排查，不应提交到 git。
路径必须位于当前 App 的 `filesDir` 下，否则 SDK 内部服务会拒绝。
`filesDir` 路径限制由 SDK Service 层（`privateFilesPath`）提供；直连 AAR、不经 SDK Service 的调用方需自行约束导出路径。
