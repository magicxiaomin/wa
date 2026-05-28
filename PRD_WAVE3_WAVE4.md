# PRD_WAVE3_WAVE4

仓库 magicxiaomin/wa，继续 Wave 3 + Wave 4。

## 先做一次性准备

1. `git fetch origin && git checkout main && git pull --ff-only origin main`
2. 新建分支：`git checkout -b feat/wave3-wave4`
3. 阅读：`SPEC_WAVE3.md`、`ACCEPTANCE_WAVE3.md`、`KNOWN_PITFALLS.md`、`GOMOBILE_CONSTRAINTS.md`、`ANDROID_PITFALLS.md`、`TRACE_SCHEMA.md`、`bridge/client.go`、`bridge/whatsmeow_adapter.go`。
4. 把 PRD 原文保存为仓库根目录 `PRD_WAVE3_WAVE4.md`，单独 commit：`docs: add wave3+wave4 PRD`。后续每个 Phase 以该 PRD 为准。
5. 另存一份 Wave 4 验收骨架 `ACCEPTANCE_WAVE4.md`（按 PRD 的 Phase 5-7 + 8 条安全红线列 checklist），同一 commit。

## 本次只做 Phase 1，做完停下汇报，不要碰 Phase 2+

目标：Go wrapper 新增 `SendTextMulti`，含 ≤3 硬校验，规避 PRD 陷阱 A。

## 实现要点

- 重构发送内核：把“门检查”(risk/freshLinkSendDelay/connected/operation/进程内配额) 与“底层单条发送+发事件+记 sentAt”拆成两个内部函数，供单发和多发复用。
- `SendTextMulti(toJidsJson, text, clientMsgId) (string, error)`：
  - 解析 JSON 数组；数量必须 1-3，>3 返回 `"exceeds max 3 recipients"`，一条不发。
  - 每个 JID 必须是个人（`@s.whatsapp.net` 或 `@lid`）；含 `@g.us`/其他 → 整体拒绝。
  - 门检查做一次；`sendCount+N > maxSendsPerRun` → 整体拒绝。
  - 通过后循环发送，每条之间随机节流 uniform `[3s,8s]`（用包级可覆盖变量以便测试设 0）；每条派生 id `"<clientMsgId>#<i>"`，单独 `sendCount++`/记 `sentAt`/发事件。
  - 单条失败不影响其余；返回结果 JSON 数组，元素含 `jid_suffix`（后四位）/`ok`/`server_msg_id`/`error`。
  - 返回里禁止出现完整 JID、完整号码、消息正文。
- 不动 trace 字段结构；不改 `TRACE_SCHEMA.md`（如需新事件先在汇报里提出，等确认）。

## 测试

`bridge/client_test.go`，用 `fakeWAAdapter`：

- 发 2 人、3 人全部成功，各有 `server_msg_id`。
- 发 4 人被拒，`fakeAdapter.sendCalls == 0`（一条不发）。
- 含一个 `@g.us` 的输入被拒，一条不发。
- 第 2 个收件人不会因 `operation_backoff` 失败（验证陷阱 A 已规避；测试里把节流变量设 0）。
- 单条失败（让 fake 对某 jid 返回 err）其余仍成功，结果列表逐条标注。
- 返回 JSON 不含完整号码/正文。

## 构建/测试命令

```sh
GOCACHE=$HOME/.cache/codex-wa-tools/go-cache \
GOPATH=$HOME/.local/share/codex-wa-tools/go-path \
GOPROXY=https://goproxy.cn,direct GOSUMDB=off \
$HOME/.local/share/codex-wa-tools/go1.26.3/bin/go test ./bridge
```

## Phase 1 不要做

- 不碰 Android/Kotlin/AIDL/UI（留 Phase 3）。
- 不实现 `GetGroups`/`SendToGroup`（留 Phase 2）。
- 不重编/提交 AAR（Phase 1 只是 Go 逻辑+单测；UI 接入时再重编）。
- 不引入调度/队列/模板。
- 不扩大 `allowedTestNumbers`、不移除任何发送限制、不把正文写进 trace。

## DISPUTE

判断某要求不对：不改代码，在最后一个 commit body 写 `DISPUTE: <点> <理由>`。

## 完成汇报

停下，输出：

- 实现了什么（文件 + 关键函数）
- 如何验证（测试名 + go test 结果）
- 对应 `ACCEPTANCE_WAVE3` 哪些条目
- 风险/遗留
- 终行：`PHASE_1_COMPLETE branch=feat/wave3-wave4 commits=<n> tests=<pass|fail> disputes=<n>`

等确认后再给 Phase 2 提示词。
