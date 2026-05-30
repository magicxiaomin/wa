# SPEC_WAVE4 · 第四波交接（Android whatsmeow 接口研究 · 去云端 · MVP Research）

> 前置：Wave 3 已合并——Android 真机能扫码登录、获取联系人、收发 1:1 文本消息、发给多个联系人、发到 1 个群。
>
> 本文件取代此前 Cloudflare Workers / Durable Objects 远程触发设计。项目定位调整为：
> **单手机、本地 Android、whatsmeow Android 接口研究工具**。
>
> 本文件也取代 Wave 3 中“最多 3 人”和“trace 必须脱敏”的产品化约束。Wave 4 是 MVP 研究模式，
> 用于验证 whatsmeow Android 接口有效性，不发布、不交给普通用户使用。

---

## 0. 定位

这是个人兴趣研究项目，只在小范围本地研究环境中使用，不商业化、不规模化、不发布。

Wave 4 不再做云端远程触发，不部署 Cloudflare / VPS / Pages / Worker，不做 Web 控制台。
研究重点回到 Android 真机上如何稳定、透明地调用 whatsmeow 能力，并观察完整接口输入输出。

---

## 1. 本波目标

只保留并整理 Android 本地能力：

1. 扫码登录 / session 恢复。
2. 获取联系人。
3. 获取已加入群。
4. 发送 1:1 文本。
5. 发给多个联系人同一条文本，用于研究 `SendTextMulti` 行为，不再设置 3 人上限。
6. 发到 1 个群。
7. 收 1:1 文本消息。
8. 导出 MVP research raw trace/debug，方便研究 whatsmeow 在 Android 上的行为。
9. 暴露当前登录账号身份信息，用于验证 `UserIDString()`、连接状态、session 持有状态。

---

## 2. 不做范围

- 不做 Cloudflare Workers / Durable Objects / Pages。
- 不做 VPS。
- 不做远程触发发送。
- 不做浏览器控制台。
- 不做队列、任务、调度、模板、批量发送。
- 不做多账号、多手机、多租户。
- 不做媒体消息。
- 不做远程发群接口。
- 不做风控规避。
- 不做任何自动外发 trace/session/debug bundle 的功能。
- 不把研究用 trace/debug 文件上传到服务器或第三方。

---

## 3. 架构

```
Android MainActivity（主进程）
  - 本地 UI
  - QR / contacts / groups / send
  - 联系人多选，不设置 3 人上限
  - 群列表单选
        │ AIDL
        ▼
BridgeForegroundService（:wa_bridge 独立进程）
  - ForegroundService dataSync
  - 持有 Go Client
  - 跨进程事件回调
        │ gomobile
        ▼
wamobile.aar / bridge.Client（Go wrapper）
  - Start / Connect / GetContacts / GetGroups
  - SendText / SendTextMulti
  - ClearSession / SafetyStatus / ExportTrace
  - MVP research raw trace/debug
  - 可暴露 self JID / UserIDString / session debug 信息
        │
        ▼
whatsmeow
  - WhatsApp Web 协议
  - SQLite session
        │
        ▼
WhatsApp
```

---

## 4. MVP Research 边界

本波不再把 3 人上限和 trace 脱敏作为验收要求。原因：

- 当前目标是研究 whatsmeow Android 接口的真实行为，而不是发布产品。
- 研究需要看到完整 JID、完整号码、消息正文、QR/pairing code、session/auth 相关调试材料。
- `SendTextMulti` 要尽量接近底层接口行为，不能因为 wrapper 的产品化限制影响接口验证。
- trace/debug 文件只保存在本机研究环境，不上传、不分享、不进入云端。

仍然保留的边界：

- 不提供 Cloudflare / VPS / Web 控制台 / 远程触发入口。
- 不提供队列、定时、模板、批量任务。
- 群发送仍是本地 UI 选 1 个群即时发送，不做多群任务。
- 不做多账号、多手机、多租户。

本波对以下字段允许原始导出：

- `self_jid` / `UserIDString()` / 当前登录账号 JID。
- 完整联系人 JID、群 JID、PN/LID 映射相关字段。
- 消息正文、收发目标、server message ID、receipt 信息。
- QR code / pairing code。
- session/auth credentials、device store、sqlite session 调试材料。

注意：这些字段是登录凭证或可识别个人身份的信息。它们只用于本机 MVP 研究，不适合作为公开日志、
issue 附件、review 附件或对外证明材料。

## 5. 账号身份验证语义

本波需要支持验证当前登录账号身份：

- `UserIDString()` / self JID 可作为当前 WhatsApp 账号在 whatsmeow 体系内的唯一账号 ID。
- `GetState() == connected` 表示本机当前持有一个可用 linked-device session。
- `self JID + connected` 可作为本机可信的账号持有状态证明。
- `self JID + session/auth credentials` 可证明本机持有该账号的 linked-device 登录凭证，但这些 credentials 是钥匙，不是适合外部公开验证的证书。
- 如需对外可核验控制权，应使用 nonce 消息验证：生成随机 nonce，由当前账号发给测试联系人或等待对方回复同一 nonce。

建议新增或整理接口：

- `GetSelfIdentity() string`
  - 返回 `self_jid`、`jid_server`、`state`、`is_logged_in`、`is_connected`、`has_session_db`、`device_name` 等 JSON 字段。
- `ExportSessionDebug(path string)`
  - 研究模式导出当前 trace、session db 文件信息、device store / credential 调试信息。
  - 只写本地私有目录，不做网络上传。

---

## 6. Phase 划分

### Phase 5：去云端化

- 删除 `edge/` Cloudflare Worker / Pages / mock phone。
- 删除 Go Remote Relay WS client。
- 删除 Android Remote Trigger UI / AIDL / Service 接口。
- 删除远程 relay trace schema。
- 确认 `go.mod` 不再因远程 relay 引入额外依赖。

### Phase 6：Android 本地接口研究整理

- 保留并验证现有 Android 本地入口：
  - `GetContacts`
  - `GetGroups`
  - `SendText`
  - `SendTextMulti`
  - `ExportTrace`
  - `SafetyStatus`
- 增加或整理身份验证入口：
  - `GetSelfIdentity`
  - 可选 `ExportSessionDebug`
- UI 保持简单，不做产品化聊天体验。
- 确认 UI 侧不再限制 3 人。
- 确认 `SendTextMulti` 结果返回完整 `jid` 和错误原文，方便研究。
- 确认 raw trace/debug 可包含完整业务字段和认证调试材料。

### Phase 7：文档与验收

- 更新验收文档，删除 Cloudflare 相关验收项。
- 输出当前 Android whatsmeow 接口研究架构说明。
- 给 reviewer 明确：本波不包含云端、远程触发、Web 控制台。

---

## 7. 验收要求

- 代码中不存在 `edge/` 云端实现。
- Android UI 中不存在 Remote Trigger 开关、URL 输入、token 输入。
- AIDL 中不存在远程 relay 方法。
- Go wrapper 中不存在 Remote Relay WS client。
- Android UI 不再限制联系人多选最多 3 个。
- Go wrapper 不再把 `SendTextMulti` 返回结果脱敏成后四位。
- `TRACE_SCHEMA.md` 明确 MVP research raw trace/debug 会包含敏感研究材料。
- 本地 Android build 通过。
- Go wrapper 单测通过。
- 现有 Wave 3 本地能力不被破坏。

---

## 8. 配套文档

- `ACCEPTANCE_WAVE4.md`
- `SPEC_WAVE3.md`
- `KNOWN_PITFALLS.md`
- `GOMOBILE_CONSTRAINTS.md`
- `ANDROID_PITFALLS.md`
- `TRACE_SCHEMA.md`
