# SPEC_WAVE4 · 第四波交接（Android whatsmeow 接口研究 · 去云端）

> 前置：Wave 3 已合并——Android 真机能扫码登录、获取联系人、收发 1:1 文本消息、发给最多 3 个联系人、发到 1 个群。
>
> 本文件取代此前 Cloudflare Workers / Durable Objects 远程触发设计。项目定位调整为：
> **单手机、本地 Android、whatsmeow Android 接口研究工具**。

---

## 0. 定位

这是个人兴趣研究项目，只在 3-5 人小圈子内使用，不商业化、不规模化。

Wave 4 不再做云端远程触发，不部署 Cloudflare / VPS / Pages / Worker，不做 Web 控制台。
研究重点回到 Android 真机上如何稳定、安全地调用 whatsmeow 能力。

---

## 1. 本波目标

只保留并整理 Android 本地能力：

1. 扫码登录 / session 恢复。
2. 获取联系人。
3. 获取已加入群。
4. 发送 1:1 文本。
5. 发给 1-3 个联系人同一条文本。
6. 发到 1 个群。
7. 收 1:1 文本消息。
8. 导出脱敏 trace，方便研究 whatsmeow 在 Android 上的行为。

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

---

## 3. 架构

```
Android MainActivity（主进程）
  - 本地 UI
  - QR / contacts / groups / send
  - 联系人多选 UI 限制最多 3 个
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
  - 3 人上限、节流、risk-stop、trace 脱敏
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

## 4. 安全边界

本波以 UI 限制为主要交互防线：

- 联系人多选最多 3 个。
- 选满 3 个后，UI 拒绝继续选择。
- 群发送只能选 1 个群。
- 不提供远程发送入口。

Go wrapper 改为更接近接口研究语义：

- `SendTextMulti` 不再强制 1-3 目标上限。
- `SendTextMulti` 不再强制拒绝 `@g.us`。
- 不再使用 wrapper 进程内发送计数上限。
- 保留发送节流，便于观察连续调用行为且避免瞬时请求。
- 保留 risk-stop。
- trace 不记录 session key、完整号码、完整 JID、消息正文。

原因：本波定位为 whatsmeow Android 接口研究，wrapper 尽量少改变底层能力；用户交互限制放在 UI。
账号状态保护和隐私保护仍保留在 wrapper。

---

## 5. Phase 划分

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
- UI 保持简单，不做产品化聊天体验。
- 确认 UI 侧 3 人限制仍在。

### Phase 7：文档与验收

- 更新验收文档，删除 Cloudflare 相关验收项。
- 输出当前 Android whatsmeow 接口研究架构说明。
- 给 reviewer 明确：本波不包含云端、远程触发、Web 控制台。

---

## 6. 验收要求

- 代码中不存在 `edge/` 云端实现。
- Android UI 中不存在 Remote Trigger 开关、URL 输入、token 输入。
- AIDL 中不存在远程 relay 方法。
- Go wrapper 中不存在 Remote Relay WS client。
- 本地 Android build 通过。
- Go wrapper 单测通过。
- 现有 Wave 3 本地能力不被破坏。

---

## 7. 配套文档

- `ACCEPTANCE_WAVE4.md`
- `SPEC_WAVE3.md`
- `KNOWN_PITFALLS.md`
- `GOMOBILE_CONSTRAINTS.md`
- `ANDROID_PITFALLS.md`
- `TRACE_SCHEMA.md`
