# wa · WhatsApp 本地 Bridge — PoC 阶段

> 本仓库当前处于**第一波：桌面 Go PoC**。目标是用最低成本验证
> `whatsmeow` + 测试号能否连接、发消息、恢复 session —— 一票决定项目是否进入 Android 阶段。

## 给执行 agent（Codex）的入口

**请先读 [`SPEC.md`](./SPEC.md)，它是本任务的宪法。**

完整阅读顺序：
1. [`SPEC.md`](./SPEC.md) — 目标、范围、非目标、分阶段（**先读这个**）
2. [`API_CONTRACT.md`](./API_CONTRACT.md) — 要实现的接口与事件类型
3. [`GOMOBILE_CONSTRAINTS.md`](./GOMOBILE_CONSTRAINTS.md) — 类型约束（PoC 就遵守，省后续返工）
4. [`KNOWN_PITFALLS.md`](./KNOWN_PITFALLS.md) — 已知坑（**逐条对照**，项目特有知识）
5. [`ACCEPTANCE.md`](./ACCEPTANCE.md) — 验收 checklist
6. [`TRACE_SCHEMA.md`](./TRACE_SCHEMA.md) — trace 字段与脱敏要求

启动指令见 [`00_如何使用.md`](./00_如何使用.md) 末尾的"可直接粘给 Codex 的启动 prompt"。

## 范围红线（重要）

- 本阶段**只做桌面 Go PoC**。
- **禁止**写任何 Android / gomobile / AAR / Kotlin 代码——那是后续阶段。
- **禁止**群发 / 多账号 / 收消息 / 后台静默 / 风控规避。
- `SendTextForTest` 必须内置白名单 + 发送计数上限（防滥用）。

## 技术栈（锁定）

Go + [`go.mau.fi/whatsmeow`](https://pkg.go.dev/go.mau.fi/whatsmeow)，命令行程序，本地 session 持久化。
不使用 c-shared/JNA、不使用 Baileys、不使用 Cloud API、不使用 WebView 注入（理由见 SPEC）。

## Step 1 运行

```powershell
go run ./cmd/wa-poc -data-dir ./wa-session -device-name wa-desktop-poc
```

启动后看到 `bridge_started` 表示 wrapper 和本地 session store 初始化完成。全新 session 下应随后看到 `qr_generated`，终端会打印可扫码的二维码；下方也会保留 QR payload 文本作为兜底。用测试小号在 WhatsApp 的 Linked Devices 里扫码后，看到 `connected` 表示 Step 1 跑通。

程序退出时会尝试导出 `trace.json`。Step 1 的 trace 只记录二维码长度等脱敏信息，不写入 QR 完整内容。

## Step 2 运行

先确认测试接收号码在 [`bridge/client.go`](./bridge/client.go) 的 `allowedTestNumbers` 硬编码白名单内。号码必须是完整国家码格式，不带 `+`、空格或横线。

```powershell
go run ./cmd/wa-poc `
  -data-dir ./wa-session `
  -device-name wa-desktop-poc `
  -send-to 15551234567 `
  -text "hello from wa poc"
```

已有 session 时，程序应直接进入 `connected` / `session_restored`，随后输出 `message_send_start` 和 `message_sent`。`message_sent` 的 payload 里应包含 `clientMsgId` 和服务器返回的 `server_msg_id`。

发送 trace 不记录消息正文和完整手机号，只记录文本长度、号码后四位、服务器 message ID 和延迟。

## 合规提醒

whatsmeow 是非官方库，连接个人号的 Linked Device，违反 WhatsApp 条款、有封号风险。
**PoC 必须用一个不重要的测试小号，绝不用主力号。**
