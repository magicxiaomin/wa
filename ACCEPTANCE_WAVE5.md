# ACCEPTANCE_WAVE5 · 当前回归基线（SDK 模块化 · 接口健壮性研究）

本文件记录当前主线应持续满足的回归基线。设计以 `SPEC_WAVE5.md` 为准，`SPEC_WAVE4.md` 的本地研究边界继续生效。

## 推荐业务 API

- [x] `GetSelfIdentity()` 返回 self_jid/jid_server/state/is_logged_in/is_connected/has_session_db/device_name 的 JSON。
- [x] `GetUserInfo(jidsJson)` 入参为 JSON 数组，返回每个 jid 的 user info JSON。
- [x] `GetProfilePictureInfo(jid)` 返回头像信息 JSON；拿不到时返回明确空结构和原因。
- [x] `MarkRead(chatJid, messageIdsJson, senderJid)` 已暴露。
- [x] `SendPresence(state)` 支持 `available` / `unavailable`。
- [x] `SubscribePresence(jid)` 已暴露。
- [x] `ExportSessionDebug(path)` 仅写本机私有目录，无网络上传，产物不提交 git。
- [x] 公开 Go 方法有 `recoverAsError` panic 边界。
- [x] 导出签名只用基础类型，复杂入参/返回值用 JSON string，不漏内部协议类型。
- [x] 每个核心方法有对应 AIDL 方法和 SDK 公开方法，不靠动态字符串分发暴露。
- [x] Go 单测覆盖主要 happy path、错误路径和 JSON 形状。

## SDK 模块化

- [x] `:wa-sdk` 是 Android library module，可 `assembleRelease` 产出可集成 AAR。
- [x] AIDL 契约与 `BridgeForegroundService` 位于 `:wa-sdk`。
- [x] `:wa_bridge` 独立进程使用 dataSync ForegroundService。
- [x] `WaBridgeClient` 暴露 bind/unbind、业务方法和事件 listener。
- [x] SDK 回调保证在 Main Looper 触发。
- [x] SDK 定义自己的异常类型，不直接向集成方抛原始 `RemoteException`。
- [x] SDK Manifest 声明 ForegroundService 与权限，集成方 merge 即用。
- [x] `:sample-app` 仅依赖 `:wa-sdk`，只通过 `WaBridgeClient` 调用。
- [x] `:sample-app` 不直接引用 `wamobile.aar`、不直接写 AIDL stub、不直接 new 内部 Service。
- [x] `:sample-app` 提供 SDK API 验证台：按钮、原始结果/错误展示、状态/事件流展示。

## 构建与交付物

- [x] `go test ./bridge` 通过。
- [x] `:sample-app` assembleDebug 通过。
- [x] `:wa-sdk` assembleRelease 通过并产出 AAR。
- [x] `wamobile.aar` 与 `wa-sdk-release.aar` 已纳入交付物。
- [x] AIDL、Android Service 封装、示例 App、`SDK_API.md` 均已提供。
- [x] Go 代码变化后通过 `android/build_debug_go126.sh` 重编 AAR 和拆包碎片。

## 安全 / 不变量

- [x] 无云端、远程触发、队列、调度、对象存储。
- [x] 单手机、单账号；无多账号、多手机、多租户、后台隐藏群发。
- [x] 所有导出 Go 方法和事件 goroutine 有 panic recover。
- [x] 跨边界只用基础类型 + JSON string。
- [x] 跨进程回调最终回到 Main Looper。
- [x] 核心业务能力都有显式 wrapper/AIDL/SDK 方法，不靠动态字符串分发。
- [x] raw trace / session debug 仅本机私有目录，无自动上传/外发；debug bundle 不进 git。

## 真机回归基线

- [x] `bind()` → session 恢复 → `connected`。
- [x] `getSelfIdentity()` 返回正确 self_jid/jid_server/device_name/has_session_db。
- [x] `getContacts()` 能返回联系人列表。
- [x] `getGroups()` 能返回群列表。
- [x] `sendText()` 能发送 1:1 文本并收到发送/ack 事件。
- [x] `sendText()` 能向单个群发送文本并收到发送/ack 事件。
- [x] `getUserInfo()` 返回 `jid/status/lid/devices` 等字段。
- [x] `getProfilePictureInfo()` 对无头像账号返回 `found=false` 和明确原因，不崩。
- [x] `sendPresence()` / `subscribePresence()` 能返回 requested。
- [x] `exportTrace()` / `exportSessionDebug()` 在 filesDir 内成功落盘。
- [x] 杀主进程后 `:wa_bridge` 仍可保持；重开 app 后无双 client / db lock 明显迹象。

## 不阻塞通过的已知项

- [ ] 收群消息内容：当前 wrapper 仍过滤群消息接收事件，后续按需立项。
- [ ] 媒体消息接口：不在当前范围。
- [ ] 真机长跑 / 国产 ROM 保活：延续观察。
- [ ] 第三方真实集成工程联调：当前用 `:sample-app` 自证，外部工程联调延后。
