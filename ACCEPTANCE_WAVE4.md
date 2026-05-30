# ACCEPTANCE_WAVE4 · 第四波验收（Android whatsmeow 接口研究 · 去云端）

> 设计以 `SPEC_WAVE4.md` 为准。本波移除 Cloudflare / 远程触发，项目回到单手机 Android 本地接口研究。

## Phase 5：去云端化

- [ ] 仓库中不存在 `edge/` Cloudflare Worker / Pages / mock phone 实现。
- [ ] Go wrapper 中不存在 Remote Relay WS client。
- [ ] Android AIDL 中不存在 `startRemoteRelay` / `stopRemoteRelay` / `getRemoteRelayStatus`。
- [ ] Android UI 中不存在 Remote Trigger 开关、URL 输入、token 输入。
- [ ] `TRACE_SCHEMA.md` 中不存在远程 relay 事件字段。
- [ ] `go.mod` 不再因远程 relay 引入额外 websocket 依赖。

## Phase 6：Android 本地接口研究

- [ ] `GetContacts` 本地入口保留。
- [ ] `GetGroups` 本地入口保留。
- [ ] `SendText` 本地 1:1 文本发送入口保留。
- [ ] `SendTextMulti` 本地接口研究入口保留，wrapper 不再强制 3 人上限。
- [ ] 本地发到 1 个群入口保留。
- [ ] `ExportTrace` / `SafetyStatus` 保留。
- [ ] UI 联系人多选最多 3 个，选满后拒绝继续选择。
- [ ] 群列表仍为单选。

## Phase 7：构建与文档

- [ ] `go test ./bridge` 通过。
- [ ] `./android/build_debug_go126.sh` 通过。
- [ ] 文档明确本波不做 Cloudflare / VPS / 远程触发 / Web 控制台。
- [ ] reviewer 能从文档判断项目定位为“whatsmeow Android 接口研究”。

## 最终判定

- [ ] 项目不包含任何云端 relay / 远程触发发送能力。
- [ ] Android 本地 whatsmeow 接口研究能力保留。
- [ ] 范围守住：无云端、无队列、无调度、无远程群发、无多账号。
- [ ] 安全限制转到交互层：UI 最多 3 人，wrapper 保留节流、risk-stop、trace 脱敏。

## 不阻塞通过的已知项

- 真机端到端送达可按需另行验证。
- UI 仍是研究工具级别，不做产品化聊天体验。
- 国产 ROM 后台限制、电量消耗、长时间保活仍是后续观察项。
