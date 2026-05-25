# 00_如何使用这个交接包（给你自己看）

## 这是什么
这是交给 Codex 的**第一波**任务包，范围只有一个：桌面 Go PoC。
目的：在投入任何 Android 工程前，用半天验证 whatsmeow + 你的号能不能连上、发消息、恢复 session。

## 文件清单
- `SPEC.md` — 主指令（Codex 的宪法）。范围、目标、非目标、分阶段。
- `API_CONTRACT.md` — 要实现的接口与事件类型。
- `GOMOBILE_CONSTRAINTS.md` — 类型约束（PoC 就遵守，省后续返工）。
- `KNOWN_PITFALLS.md` — 已知坑（最值钱，逐条对照）。
- `ACCEPTANCE.md` — 验收 checklist。
- `TRACE_SCHEMA.md` — trace 字段与脱敏。

## 怎么交给 Codex
1. 把整个 `codex-handoff-poc/` 目录作为上下文提供给 Codex。
2. 用下面的"启动 prompt"作为第一条指令。
3. 让它先读完所有 .md，再开始按 SPEC 的 Step 1→4 推进，每步停下来给你看。

## 你自己要准备的
- 一个**不重要的测试 WhatsApp 小号**（绝不用主力号）
- 一个白名单接收号码（可以是你另一个号 / 朋友的，先打好招呼）
- Go 开发环境（Codex 会用，但你本地也要能跑来验收）

## 关键提醒
- 让 Codex **只做 PoC**。如果它开始写 Android / gomobile / Kotlin，拉回来——那是第二波。
- PoC 跑通后，用 `ACCEPTANCE.md` 末尾的"项目级判定"三问决定要不要进 Android 阶段。
- 这一波通过，我再帮你做第二波（Android 工程）的交接包。

---

# 可直接粘给 Codex 的启动 prompt

```
你是一名资深 Go 工程师。本仓库根目录有一套规格文档，
请先完整阅读以下文件，再开始工作：
SPEC.md, API_CONTRACT.md, GOMOBILE_CONSTRAINTS.md, KNOWN_PITFALLS.md,
ACCEPTANCE.md, TRACE_SCHEMA.md。

你的任务范围严格限定为 SPEC.md 描述的"桌面 Go PoC"。
绝对不要写任何 Android / gomobile / AAR / Kotlin 代码——那是后续阶段。

要求：
1. 用 go.mau.fi/whatsmeow 实现一个命令行 Go 程序。
2. 严格遵守 API_CONTRACT.md 的接口形态和事件类型。
3. 严格遵守 GOMOBILE_CONSTRAINTS.md（导出接口只用基本类型/JSON，
   whatsmeow 内部类型不外泄）。
4. 逐条对照 KNOWN_PITFALLS.md，特别是：
   - 第1条：事件处理器和 QR channel 必须在 Connect 之前挂载（否则拿不到二维码）
   - 第3条：每个导出方法和事件 goroutine 必须 panic recover
   - 第5条：评估用 modernc.org/sqlite 纯 Go 驱动避开 cgo
5. SendTextForTest 必须内置白名单校验 + 发送计数上限（防滥用，见 SPEC 第3节）。
6. trace 导出严格按 TRACE_SCHEMA.md 脱敏。

请按 SPEC.md 第4节的 Step 1 → Step 2 → Step 3 → Step 4 顺序推进，
每完成一个 Step 停下来，说明这一步实现了什么、如何验证、对应 ACCEPTANCE 的哪几条。
先从 Step 1 开始：whatsmeow 初始化 + session store + 正确挂载事件 + 生成二维码。
```
