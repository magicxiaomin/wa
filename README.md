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

## 合规提醒

whatsmeow 是非官方库，连接个人号的 Linked Device，违反 WhatsApp 条款、有封号风险。
**PoC 必须用一个不重要的测试小号，绝不用主力号。**
