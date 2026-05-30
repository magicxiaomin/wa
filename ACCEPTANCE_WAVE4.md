# ACCEPTANCE_WAVE4 · 第四波验收（Android whatsmeow 接口研究 · 去云端 · MVP Research）

> 设计以 `SPEC_WAVE4.md` 为准。本波移除 Cloudflare / 远程触发，项目回到单手机 Android 本地接口研究。
> 本波是 MVP research mode，不发布、不交给普通用户使用。
>
> 注意：Wave 4 明确取代 Wave 3 中“最多 3 人”和“trace 必须脱敏”的约束。
> Claude review 不应把移除 3 人限制、记录完整业务字段、记录 session/auth/QR 调试材料判为回归；
> 它们是本波需求。

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
- [ ] `SendTextMulti` 返回结果包含完整 `jid`，不再只返回 `jid_suffix`。
- [ ] `SendTextMulti` 单个目标失败时返回错误原文，方便定位底层接口失败原因。
- [ ] 本地发到 1 个群入口保留。
- [ ] `ExportTrace` / `SafetyStatus` 保留。
- [ ] UI 联系人多选不再限制最多 3 个。
- [ ] 群列表仍为单选。
- [ ] 可获取当前登录账号身份：至少能拿到 `UserIDString()` / self JID 与当前连接状态。
- [ ] `GetState() == connected` 时，文档语义明确为“本机持有 self JID 对应账号的可用 linked-device session”。
- [ ] 业务相关 whatsmeow 能力优先通过常规 Go wrapper / AIDL 方法暴露。
- [ ] `InvokeAPI(name, inputJSON)` 不是本波必需项；如果后续保留，只能作为调试辅助入口，不能成为唯一业务入口。
- [ ] 新增常规 API 仍遵守 gomobile 类型约束：复杂入参/返回值用 JSON string，不外泄 whatsmeow 内部类型。

## Phase 6B：MVP Research Raw Trace / Debug

- [ ] `TRACE_SCHEMA.md` 标明 trace/debug 是研究模式原始导出，不适合发布或分享。
- [ ] 导出 trace 可包含完整 JID、完整号码、消息正文、错误原文。
- [ ] 导出 trace 可包含 QR / pairing code，用于研究扫码与关联流程。
- [ ] 研究模式允许导出 session/auth/device store 调试材料到本机私有目录。
- [ ] 不存在任何自动上传 trace/session/debug bundle 的代码路径。
- [ ] 不存在把 trace/session/debug bundle 发往 Cloudflare、VPS、第三方服务的代码路径。

## Phase 7：构建与文档

- [ ] `go test ./bridge` 通过。
- [ ] `./android/build_debug_go126.sh` 通过。
- [ ] 文档明确本波不做 Cloudflare / VPS / 远程触发 / Web 控制台。
- [ ] reviewer 能从文档判断项目定位为“whatsmeow Android 接口研究”。
- [ ] reviewer 能从文档判断：3 人上限移除和 trace 原始导出是有意需求，不是安全回归。
- [ ] reviewer 能从文档判断：业务 API 不要求通过动态 `InvokeAPI` 暴露，常规方法是首选。

## 最终判定

- [ ] 项目不包含任何云端 relay / 远程触发发送能力。
- [ ] Android 本地 whatsmeow 接口研究能力保留。
- [ ] 范围守住：无云端、无队列、无调度、无远程群发、无多账号。
- [ ] MVP 研究模式生效：无 3 人上限；trace/debug 可原始导出完整研究材料。
- [ ] raw trace/debug 只落本机，不上传、不外发、不作为公开日志。

## 不阻塞通过的已知项

- 真机端到端送达可按需另行验证。
- UI 仍是研究工具级别，不做产品化聊天体验。
- 国产 ROM 后台限制、电量消耗、长时间保活仍是后续观察项。
- session/auth credentials 是登录钥匙，不是适合对外公开验证身份的证书；如需外部可核验账号控制权，另做 nonce 消息验证。
