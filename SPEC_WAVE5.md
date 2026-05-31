# SPEC_WAVE5 · Android SDK 模块化与接口健壮性研究

Wave 5 的目标是把本地 Android 能力封装成可被其他 Android app / 模块集成的 SDK，并用 sample app 验证公开 API 的一致性和健壮性。

`SPEC_WAVE4.md` 中的定位与红线继续生效：单手机、单账号、本地、不上云、不规模化、显式业务 API 优先、raw trace 仅本机。

## 定位

个人兴趣研究项目。当前 UX / 客户端不是正式聊天产品，而是 **SDK API 的一致性与健壮性验证台**。

示例 app 只能通过 SDK 公开入口调用能力，用来证明外部集成方能按公开契约正常使用这些接口。

## SDK 模块结构

```text
Go bridge ──gomobile──> wamobile.aar
      │
:wa-sdk  (Android library module → 可集成 AAR)
   - 封装 wamobile.aar
   - 持有 AIDL 契约（IBridgeService / IBridgeCallback）
   - 持有 BridgeForegroundService（:wa_bridge 独立进程，dataSync）
   - 对外暴露 WaBridgeClient + 事件回调
      │
:sample-app  (SDK 验证台)
   - 只通过 WaBridgeClient 调用公开 API
   - 每个 SDK API 一个按钮 + 原始结果展示
```

## 公开业务 API

核心能力必须有显式 Go wrapper / AIDL / SDK 方法，不依赖动态字符串分发：

```go
GetSelfIdentity() (string, error)
GetContacts() (string, error)
GetGroups() (string, error)
GetUserInfo(jidsJson string) (string, error)
GetProfilePictureInfo(jid string) (string, error)
SendText(to string, text string, clientMsgId string) error
SendTextMulti(toJidsJson string, text string, clientMsgId string) (string, error)
MarkRead(chatJid string, messageIdsJson string, senderJid string) error
SendPresence(state string) error
SubscribePresence(jid string) error
ExportTrace(path string) error
ExportSessionDebug(path string) error
```

每个导出 Go 方法必须：

- 只使用 gomobile 支持的基础类型；复杂入参/返回值使用 JSON string。
- 不把内部 `types.JID`、`events.*`、proto 类型跨过 gomobile 边界。
- 使用 `recoverAsError` 作为 panic 边界。
- 需要连接、冷却或 risk-stop 判定时复用现有 gate。
- 按 `TRACE_SCHEMA.md` MVP raw 口径记录本机 trace/debug。

## `:wa-sdk` 要求

- Android library module，能 `assembleRelease` 产出可集成 AAR。
- 持有 AIDL 契约、`BridgeForegroundService` 与 `wamobile.aar` 封装。
- Manifest 声明 `:wa_bridge` ForegroundService 与所需权限，集成方 merge 即用。
- 对外暴露 `WaBridgeClient`：
  - `bind()` / `unbind()`。
  - 与 AIDL 一一对应的业务方法。
  - 事件 listener：`onEvent(eventType, payloadJson)`。
  - 回调必须由 SDK 内部切回 `Looper.getMainLooper()`。
  - 错误统一包装为 SDK 自己的异常，不直接抛原始 `RemoteException`。

## `:sample-app` 要求

- 仅依赖 `:wa-sdk`。
- 只通过 `WaBridgeClient` 调用能力。
- 不直接引用 `wamobile.aar`、不直接写 AIDL stub、不直接 new 内部 Service。
- 覆盖登录/恢复/清除 session、状态、事件流和所有公开业务方法。
- UI 职责是验证 SDK API，不追求正式聊天体验。

## 不做范围

- 不做 Cloudflare、VPS、远程触发、Web 控制台。
- 不做队列、定时、调度、模板、批量任务、对象存储。
- 不做多账号、多手机、多租户、后台隐藏群发。
- 不做媒体消息，除非后续单独立项。
- 不实现 `InvokeAPI` 作为核心业务唯一入口。
- 不引入任何 trace/session/debug 的网络上传或外发路径。
- 不把 SDK 做成多账号管理框架。

## 交付物

- `wamobile.aar` 与 `wa-sdk-release.aar`。
- `IBridgeService` / `IBridgeCallback` AIDL。
- `:wa_bridge` ForegroundService 与 `WaBridgeClient`。
- `:sample-app` SDK 验证台。
- `SDK_API.md` API 文档。
- `ACCEPTANCE_WAVE5.md` 回归基线。

## 配套文档

- `SDK_API.md`
- `ACCEPTANCE_WAVE5.md`
- `SPEC_WAVE4.md`
- `ACCEPTANCE_WAVE4.md`
- `TRACE_SCHEMA.md`
- `GOMOBILE_CONSTRAINTS.md`
- `ANDROID_PITFALLS.md`
- `KNOWN_PITFALLS.md`
