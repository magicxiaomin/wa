# SPEC_WAVE4 · 本地 Android 接口研究边界

本文件定义当前项目的本地研究边界。它是 `SPEC_WAVE5.md` 的前置约束：Wave 5 在此基础上把 Android 能力封装成可集成 SDK。

## 定位

这是个人兴趣研究项目，只在本机和小范围测试环境中使用，不商业化、不发布、不规模化。

研究重点是：在 Android 真机上稳定、透明地调用 Go IM 协议客户端能力，并观察完整接口输入输出。

## 当前本地能力

- 扫码登录与 session 恢复。
- 获取联系人。
- 获取已加入群列表。
- 发送 1:1 文本。
- 向多个联系人发送同一文本，用于接口行为研究。
- 向单个群发送文本。
- 接收 1:1 文本消息。
- 导出 MVP research raw trace/debug。
- 暴露当前登录账号身份信息，用于验证 self JID、连接状态和 session 持有状态。

## 不做范围

- 不做 Cloudflare、VPS、远程触发、Web 控制台。
- 不做队列、任务、调度、模板、批量任务。
- 不做多账号、多手机、多租户。
- 不做媒体消息。
- 不做远程发群接口。
- 不做风控规避。
- 不做任何 trace/session/debug bundle 的自动上传或外发。
- 不把研究用 trace/debug 文件上传到服务器或第三方。

## 架构边界

```text
Android sample-app（主进程）
  - SDK API 验证台
  - 只通过 WaBridgeClient 调用公开接口
        │
        │ AIDL
        ▼
:wa-sdk / BridgeForegroundService（:wa_bridge 独立进程）
  - ForegroundService dataSync
  - 持有 Go Client
  - 跨进程事件回调切回 Main Looper
        │
        │ gomobile
        ▼
wamobile.aar / bridge.Client（Go wrapper）
  - Start / Connect / GetContacts / GetGroups
  - SendText / SendTextMulti
  - GetSelfIdentity / ExportTrace / ExportSessionDebug
  - MVP research raw trace/debug
        │
        ▼
Go IM 协议客户端
  - SQLite session
```

## MVP Research Raw Trace / Debug

当前目标是研究 Android 接口真实行为，而不是发布产品。因此 raw trace/debug 允许记录完整研究材料：

- `self_jid` / `UserIDString()` / 当前登录账号 JID。
- 完整联系人 JID、群 JID、PN/LID 映射相关字段。
- 消息正文、收发目标、server message ID、receipt 信息。
- QR code / pairing code。
- session/auth credentials、device store、sqlite session 调试材料。

这些字段是登录凭证或可识别个人身份的信息。它们只用于本机 MVP 研究，不适合作为公开日志、issue 附件、review 附件或对外证明材料。

## 账号身份验证语义

- `UserIDString()` / self JID 可作为当前账号在协议客户端体系内的账号 ID。
- `GetState() == connected` 表示本机当前持有一个可用 linked-device session。
- `self JID + connected` 可作为本机可信的账号持有状态证明。
- `self JID + session/auth credentials` 可证明本机持有该账号的 linked-device 登录凭证，但 credentials 是钥匙，不是适合外部公开验证的证书。
- 如需对外可核验控制权，应使用 nonce 消息验证：生成随机 nonce，由当前账号发给测试联系人或等待对方回复同一 nonce。

## 业务 API 暴露方式

业务能力优先通过常规 Go wrapper / AIDL / SDK 方法暴露，而不是通过动态字符串分发入口暴露。

推荐形态：

```go
GetSelfIdentity() (string, error)
GetContacts() (string, error)
GetGroups() (string, error)
GetUserInfo(jidsJson string) (string, error)
GetProfilePictureInfo(jid string) (string, error)
SendText(to string, text string, clientMsgId string) error
SendTextMulti(toJidsJson string, text string, clientMsgId string) (string, error)
MarkRead(chatJid string, messageIdsJson string, senderJid string) error
SendPresence(state string) error
SubscribePresence(jid string) error
ExportTrace(path string) error
ExportSessionDebug(path string) error
```

对应 AIDL / SDK 方法应保持同名或近似同名，复杂入参和返回值继续使用 JSON string。

`InvokeAPI(name, inputJSON)` 如果后续保留，只能作为开发调试/实验台辅助入口，不能成为核心业务 API 的唯一入口。

## 当前验收基线

- 无云端、远程触发、Web 控制台、队列、调度、多账号。
- Android UI 不再限制联系人多选最多 3 个。
- `SendTextMulti` 返回完整 `jid` 和错误信息，方便研究。
- `TRACE_SCHEMA.md` 明确 MVP research raw trace/debug 会包含敏感研究材料。
- 业务 API 暴露方式以常规 wrapper / AIDL / SDK 方法为主。
- Android build 与 Go wrapper 单测通过。
- 既有本地能力不被破坏。
