# ACCEPTANCE_WAVE5 · 第五波验收（SDK 模块化 · 接口健壮性研究）

> 设计以 `SPEC_WAVE5.md` 为准。Wave 4 MVP Research Mode 的定位与红线继续生效。
> 本波一次性实现阶段 A/B/C 后统一 review，不逐阶段汇报。

## 阶段 A：推荐业务 API（Go wrapper + AIDL + SDK）
- [ ] 新增 `GetSelfIdentity()`，返回 self_jid/jid_server/state/is_logged_in/is_connected/has_session_db/device_name 的 JSON
- [ ] 新增 `GetUserInfo(jidsJson)`，入参 JSON 数组，返回每个 jid 的 user info JSON
- [ ] 新增 `GetProfilePictureInfo(jid)`，返回头像信息 JSON（拿不到时返回明确空结构 + 原因）
- [ ] 新增 `MarkRead(chatJid, messageIdsJson, senderJid)`
- [ ] 新增 `SendPresence(state)`（available / unavailable）
- [ ] 新增 `SubscribePresence(jid)`
- [ ] 新增 `ExportSessionDebug(path)`，仅写本机私有目录，无网络上传，产物不提交 git
- [ ] 以上每个方法都有 `recoverAsError` panic 边界
- [ ] 以上每个方法导出签名只用基础类型，复杂入参/返回值用 JSON string，不漏 whatsmeow 内部类型
- [ ] 每个方法有对应 AIDL 方法 + SDK 公开方法（不靠动态字符串分发暴露）
- [ ] `bridge/*_test.go` 为每个新方法补单测（happy path + 错误路径 + JSON 形状）

## 阶段 B：SDK 模块化
- [ ] 新增 `:wa-sdk` Android library module，能 `assembleRelease` 产出可集成 AAR
- [ ] AIDL 契约、`BridgeForegroundService`（:wa_bridge 独立进程 dataSync）迁入 `:wa-sdk`
- [ ] `:wa-sdk` 暴露 `WaBridgeClient`：bind/unbind + 与 AIDL 一一对应的业务方法 + 事件 listener
- [ ] SDK 回调保证在主线程触发（SDK 内部完成 Looper 切换，集成方零成本）
- [ ] SDK 定义自己的错误/异常类型，不把原始 RemoteException 直接抛给集成方
- [ ] `:wa-sdk` Manifest 声明自己的 ForegroundService 与权限，集成方 merge 即用
- [ ] 新增 `:sample-app`，仅依赖 `:wa-sdk`，只通过 `WaBridgeClient` 调用
- [ ] `:sample-app` 不直接引用 `wamobile.aar`、不直接写 AIDL stub、不直接 new 内部 Service
- [ ] `:sample-app` 每个 SDK API 一个按钮 + 原始结果/错误展示 + 状态/事件流展示

## 阶段 C：交付物（六类）
- [ ] AAR：`wamobile.aar` + `:wa-sdk` 产出的可集成 AAR
- [ ] AIDL：`IBridgeService` / `IBridgeCallback`（含新增方法）
- [ ] Android Service 封装：ForegroundService + `WaBridgeClient`（绑定/线程/错误封装）
- [ ] 示例 App：`:sample-app` 验证台
- [ ] API 文档：`SDK_API.md`（逐方法用途/JSON 形状/线程语义/错误/前置条件/最小集成步骤）
- [ ] 验收 checklist：本文件

## 构建
- [ ] `go test ./bridge` 通过
- [ ] `:sample-app` assembleDebug 通过
- [ ] `:wa-sdk` assembleRelease 通过，产出 .aar
- [ ] AAR 随 Go 代码变化重编并提交（不做 SHA 门禁）

## 安全 / 不变量（延续，不可破）
- [ ] 无云端 / 无远程 / 无队列 / 无调度 / 无对象存储
- [ ] 单手机、单账号；无多账号 / 多手机 / 多租户 / 后台隐藏群发
- [ ] 所有导出 Go 方法 + 事件 goroutine 都有 panic recover
- [ ] 跨边界只用基础类型 + JSON string，不漏 whatsmeow 内部类型
- [ ] 跨进程回调最终回到 Main Looper
- [ ] 核心业务能力都有显式 wrapper/AIDL/SDK 方法，不靠动态字符串分发
- [ ] raw trace / session debug 仅本机私有目录，无自动上传/外发；debug bundle 不进 git

## 最终判定
- [ ] 项目已成为可被其他 Android app/模块集成的 SDK 模块（`:wa-sdk` 产出 AAR）
- [ ] 示例 app 仅通过 SDK 公开 API 即可完成全部业务能力调用（证明接口自洽、够用）
- [ ] 推荐业务 API 全部补齐并可被外部按公开契约调用
- [ ] 范围守住：无云端、无远程、无队列/调度、无多账号、无 InvokeAPI 作核心唯一入口

## 不阻塞通过的已知项
- 媒体消息接口：不在本波
- 真机长跑 / 国产 ROM 保活：延续观察
- 第三方真实集成工程联调：本波用 `:sample-app` 自证，外部工程联调延后
