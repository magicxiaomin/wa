# SDK_DESIGN · Android SDK 模块与接口设计

本文件描述当前 Android SDK 的模块结构、公开接口边界和回归关注点。SDK 的目标是让外部 Android app 只通过稳定的 Kotlin 客户端调用能力，而不需要理解底层 Go、AIDL 或 Service 细节。

## 模块结构

```text
Go bridge ──gomobile──> wamobile.aar
      │
:wa-sdk  (Android library module -> 可集成 AAR)
   - 封装 wamobile.aar
   - 持有 AIDL 契约（IBridgeService / IBridgeCallback）
   - 持有 BridgeForegroundService（:wa_bridge 独立进程，dataSync）
   - 对外暴露 WaBridgeClient + 事件回调
      │
:sample-app  (SDK API 验证台)
   - 只通过 WaBridgeClient 调用公开 API
   - 用按钮、原始结果和事件流验证接口行为
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
- 不把内部 JID、event、proto 类型跨过 gomobile 边界。
- 使用 `recoverAsError` 作为 panic 边界。
- 按 `TRACE_SCHEMA.md` 记录本机 trace/debug。

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
- UI 职责是验证 SDK API 行为，不承担正式聊天产品体验。
- UI 文案跟随系统语言，默认中文，英文系统使用英文资源。

## 交付物

- `wamobile.aar` 与 `wa-sdk-release.aar`。
- `IBridgeService` / `IBridgeCallback` AIDL。
- `:wa_bridge` ForegroundService 与 `WaBridgeClient`。
- `:sample-app` SDK 验证台。
- `SDK_API.md` API 文档。
- `SDK_REGRESSION_CHECKLIST.md` 回归基线。

## 配套文档

- `SDK_API.md`
- `SDK_REGRESSION_CHECKLIST.md`
- `TRACE_SCHEMA.md`
- `GOMOBILE_CONSTRAINTS.md`
- `ANDROID_PITFALLS.md`
- `KNOWN_PITFALLS.md`
