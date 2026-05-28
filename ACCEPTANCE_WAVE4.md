# ACCEPTANCE_WAVE4 · 第四波验收骨架

> Wave 4 以 `PRD_WAVE3_WAVE4.md` 后续 Phase 5-7 为准。本文件先列验收骨架，具体条目随 Phase 提示补全。

## Phase 5

- [ ] 按 PRD 的 Phase 5 目标实现。
- [ ] 通过对应 Go/Android 自动化或手动验证。
- [ ] 不突破安全红线。

## Phase 6

- [ ] 按 PRD 的 Phase 6 目标实现。
- [ ] 通过对应 Go/Android 自动化或手动验证。
- [ ] 不突破安全红线。

## Phase 7

- [ ] 按 PRD 的 Phase 7 目标实现。
- [ ] 通过对应 Go/Android 自动化或手动验证。
- [ ] 不突破安全红线。

## 8 条安全红线

- [ ] 单次个人多人发送目标数硬上限 3，wrapper 层不可绕过。
- [ ] 群发送一次只允许 1 个群，不做多群群发。
- [ ] 保留已有 risk-stop。
- [ ] 保留已有发送频率/计数限制。
- [ ] 不扩大 `allowedTestNumbers`。
- [ ] 不做多账号。
- [ ] 不做媒体消息或模板化批量发送。
- [ ] trace 不记录 session key、完整号码、完整 JID 或消息正文。
