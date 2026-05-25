# SPEC · 桌面 Go PoC（第一波交接 · 唯一任务）

> 给执行 agent（Codex）的指令文档。请严格按本文件工作，不要超出范围。
> 本任务是整个项目的**地基验证**。它通过与否，一票决定后续所有 Android 开发是否进行。

---

## 0. 一句话目标

写一个**纯命令行 Go 程序**，用 `whatsmeow` 连接一个测试用 WhatsApp 账号，完成：扫码登录 → 发送 1 条测试消息 → 验证程序重启后能用已存 session 自动恢复（无需重新扫码）→ 全程输出结构化 trace。

**这不是 Android 项目。本阶段绝对不碰 Android / gomobile / AAR / Kotlin。** 只产出一个能在开发机（Linux/macOS）上 `go run` 的程序。

---

## 1. 为什么先做这个（背景，帮助你理解意图）

整个项目最终目标是做一个 Android 本地 WhatsApp bridge（用 gomobile 把 whatsmeow 编进 Android）。但那条路工程量大、且 whatsmeow+Android 无成熟先例。

在投入 Android 工程之前，必须先用最低成本验证一件唯一不确定的事：**whatsmeow 这个非官方库，能不能稳定连上号、发出消息、恢复 session。** 这件事和 Android 无关，用一个桌面 Go 程序半天就能验证。

**如果这个 PoC 跑不通（连不上 / 发不出 / 一连就被封 / session 恢复不了），整个项目应当停止，不进入 Android 阶段。** 所以你的产出必须让人能清楚判断"通了还是没通"。

---

## 2. 必须验证的 6 件事（逐条，对应验收）

1. whatsmeow 能在桌面 Go 程序里初始化、建立本地 session store
2. 能生成二维码（或 pairing code）用于 Linked Device 登录
3. 扫码后能进入 connected 状态
4. 能向白名单测试号码发送 1 条文本消息，并拿到 message ID（= 已提交服务器）
5. 全程能记录结构化 trace（连接生命周期 + 发送生命周期）
6. 程序重启后，能用已存的 session 自动重连，**无需重新扫码**

---

## 3. 范围边界（严格遵守）

### 必做
- 单账号、单条测试消息、发送目标限定在硬编码白名单
- 命令行交互即可（打印二维码到终端 / 打印状态 / 打印 trace）
- session 持久化到本地目录，支持重启恢复
- 结构化事件日志（trace）

### 禁止做（超出本阶段范围，做了算偏离任务）
- ❌ 任何 Android / gomobile / AAR / Kotlin 代码
- ❌ 群发、多账号、循环发送、联系人批量导入
- ❌ 收消息处理、自动回复、媒体消息、群组
- ❌ 后台静默、保活、风控规避
- ❌ 任何"为将来 Android 准备"的抽象层（保持简单，PoC 就是 PoC）
- ❌ Web 服务 / REST API / 数据库服务器（本地文件足够）

### 防滥用硬约束（必须写进代码，不可省）
- `SendTextForTest` 内部**必须校验目标号码在硬编码白名单内**，不在则拒绝并报错
- **必须有累计发送计数上限**（建议单次运行 ≤ 5 条），超过直接拒绝
- 这两条是为了从代码层面防止该 PoC 被改造成群发工具

---

## 4. 分阶段（本任务内部的推进顺序）

请按此顺序提交，每步可独立验证：

- **Step 1**：whatsmeow 初始化 + session store + 正确挂载事件处理器 + 生成并打印二维码。能扫码进入 connected。
- **Step 2**：在 connected 后，向白名单号码发 1 条消息，拿到 message ID，记录发送 trace。
- **Step 3**：session 恢复——重跑程序，不弹二维码，直接 connected。
- **Step 4**：trace 导出成 `trace.json`，含完整事件序列；导出前做脱敏。

---

## 5. 交付物

1. 可 `go run` 的 Go 程序（建议单文件或极少文件，保持简单）
2. `go.mod` / 依赖清单
3. 一个 `README.md`：如何运行、如何配置白名单、如何看 trace
4. 运行后能产出 `trace.json`
5. 一段"运行说明"：明确告诉使用者每一步看到什么算成功

---

## 6. 配套文档（必读，在同目录）

- `API_CONTRACT.md` — 要实现的 wrapper API 与事件类型（即使是 PoC，也按这个接口组织代码，方便后续复用）
- `GOMOBILE_CONSTRAINTS.md` — 类型约束（PoC 阶段就遵守，避免后续 Android 化时返工）
- `KNOWN_PITFALLS.md` — 已知坑清单（**务必逐条对照，这些是项目特有、训练数据里大概率没有的知识**）
- `ACCEPTANCE.md` — 验收 checklist
- `TRACE_SCHEMA.md` — trace 字段定义与脱敏要求

---

## 7. 最重要的一条

whatsmeow 的二维码、连接状态、发送结果**全部通过 Go 的事件机制（AddEventHandler / 事件 channel）异步产生**。一个常见的致命错误是：只调用 `Connect()` 却没正确挂载事件处理器，导致**拿不到二维码、根本无法登录**。

请确保事件处理器在 `Connect()` **之前**正确挂载，并处理至少：QR 事件、连接状态变化、消息发送回执。详见 `KNOWN_PITFALLS.md` 第 1 条。
