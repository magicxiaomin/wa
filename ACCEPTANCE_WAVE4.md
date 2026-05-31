# ACCEPTANCE_WAVE4 · 当前本地研究边界基线

本文件记录 `SPEC_WAVE4.md` 对当前主线的本地研究边界要求。Wave 5 在此基础上提供 SDK 模块化封装。

## 本地能力

- [x] `GetContacts` 本地入口保留。
- [x] `GetGroups` 本地入口保留。
- [x] `SendText` 本地 1:1 文本发送入口保留。
- [x] `SendTextMulti` 本地接口研究入口保留。
- [x] `SendTextMulti` 返回结果包含完整 `jid`。
- [x] `SendTextMulti` 单个目标失败时返回错误原文，方便定位底层接口失败原因。
- [x] 本地发到 1 个群入口保留。
- [x] `ExportTrace` / `SafetyStatus` 保留。
- [x] 群列表仍为单选发送入口。
- [x] 可获取当前登录账号身份：self JID、连接状态、session db 状态。
- [x] `GetState() == connected` 的语义是“本机持有 self JID 对应账号的可用 linked-device session”。
- [x] 业务能力优先通过常规 Go wrapper / AIDL / SDK 方法暴露。
- [x] 核心能力不依赖动态 `InvokeAPI` 字符串分发。
- [x] 常规 API 遵守 gomobile 类型约束：复杂入参/返回值用 JSON string，不外泄内部协议类型。

## MVP Research Raw Trace / Debug

- [x] `TRACE_SCHEMA.md` 标明 trace/debug 是研究模式原始导出，不适合发布或分享。
- [x] 导出 trace 可包含完整 JID、完整号码、消息正文、错误原文。
- [x] 导出 trace 可包含 QR / pairing code，用于研究扫码与关联流程。
- [x] 研究模式允许导出 session/auth/device store 调试材料到本机私有目录。
- [x] 不存在任何自动上传 trace/session/debug bundle 的代码路径。

## 构建与文档

- [x] `go test ./bridge` 通过。
- [x] `./android/build_debug_go126.sh` 通过。
- [x] 文档明确当前项目不做云端、远程触发、Web 控制台。
- [x] 文档明确项目定位为 Android 本地接口研究与 SDK 验证。
- [x] 文档明确 raw trace/debug 原始导出是研究需求，不是发布口径。
- [x] 文档明确业务 API 首选常规方法，不要求通过动态入口暴露。

## 最终判定

- [x] 项目不包含任何云端 relay / 远程触发发送能力。
- [x] Android 本地接口研究能力保留。
- [x] 范围守住：无云端、无队列、无调度、无远程群发、无多账号。
- [x] MVP 研究模式生效：trace/debug 可原始导出完整研究材料。
- [x] raw trace/debug 只落本机，不上传、不外发、不作为公开日志。

## 不阻塞通过的已知项

- [ ] 收群消息内容：当前未支持。
- [ ] 媒体消息：当前未支持。
- [ ] UI 仍是研究工具级别，不做产品化聊天体验。
- [ ] 国产 ROM 后台限制、电量消耗、长时间保活仍是后续观察项。
- [ ] session/auth credentials 是登录钥匙，不是适合对外公开验证身份的证书；如需外部可核验账号控制权，另做 nonce 消息验证。
