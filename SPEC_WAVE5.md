# SPEC_WAVE5 · 第五波交接（SDK 模块化 · 接口健壮性研究）

> 前置：Wave 4 MVP Research Mode 已审通过（分支 `feat/wave4-cloudflare-relay`，HEAD `6fbca60`，verdict APPROVE）。
> Wave 4 的定位与红线（单手机、单账号、本地、不上云、不规模化、显式业务 API 优先、raw trace 仅本机）
> 在本波**继续全部生效**。本文件在其之上增加两件事。

---

## 0. 定位（决定一切取舍）

个人兴趣研究项目。Wave 5 的研究主题是：
**"把 whatsmeow 的 Android 能力封装成可被其他 Android app / 模块集成的 SDK 模块"这件事是否可行、接口是否健壮、是否够用。**

因此本波的 UX / 客户端不再是"自用工具"，而是 **SDK API 的一致性与健壮性验证台**：
示例 app 只通过 SDK 公开入口调用，用来证明外部集成方能按公开契约正常使用这些能力。

仍不商业化、不发布、不规模化。遇到不确定取舍，永远选更简单 + 更严格那个。

---

## 1. 本波目标（两块）

### A. 补齐推荐业务 API
为 SPEC_WAVE4 §6 推荐但尚未实现的能力，新增**显式** Go wrapper 方法 + 对应 AIDL 方法 + SDK 公开 API：

```go
GetSelfIdentity() (string, error)
GetUserInfo(jidsJson string) (string, error)
GetProfilePictureInfo(jid string) (string, error)
MarkRead(chatJid string, messageIdsJson string, senderJid string) error
SendPresence(state string) error
SubscribePresence(jid string) error
ExportSessionDebug(path string) error
```

不实现 `InvokeAPI` / 任何动态字符串分发作为核心业务入口。

### B. SDK 模块化重构
把当前单 `:app` 拆成：
- `:wa-sdk`（Android library module，产出可被外部工程依赖的 AAR）
- `:sample-app`（验证台，只走 `:wa-sdk` 公开 API）

---

## 2. 目标模块结构

```
whatsmeow(Go) ──gomobile──> wamobile.aar          （底层，不变）
      │
:wa-sdk  (Android library module → 可集成 AAR)
   - 封装 wamobile.aar
   - 持有 AIDL 契约（IBridgeService / IBridgeCallback）
   - 持有 BridgeForegroundService（:wa_bridge 独立进程，dataSync）
   - 对外暴露干净 Kotlin 客户端 API（WaBridgeClient）+ 事件回调
      │  被依赖
:sample-app  (示例 / 健壮性验证台)
   - 只通过 WaBridgeClient 调用；不直接引用 wamobile.aar、不直接写 AIDL stub
   - 每个 SDK API 一个按钮 + 原始结果展示
```

原则：**示例 app 只能走 SDK 公开入口**——这本身就是对"SDK 是否自洽、够用"的验证。

---

## 3. 不做范围（延续 Wave 4）

- 不做 Cloudflare / VPS / 远程触发 / Web 控制台。
- 不做队列 / 定时 / 调度 / 模板 / 批量 / 对象存储。
- 不做多账号 / 多手机 / 多租户 / 后台隐藏群发。
- 不做媒体消息（除非后续单独立项）。
- 不实现 InvokeAPI 作为核心业务唯一入口。
- 不引入任何 trace/session/debug 的网络上传或外发路径。
- 不把 SDK 做成多账号管理框架。

---

## 4. 阶段 A：推荐业务 API 实现细则

每个新方法必须：

1. **gomobile 类型约束**：导出签名只用 `string` / `int` / `bool` / `[]byte`；复杂入参与返回值
   一律用 JSON string；**绝不把 whatsmeow 内部 `types.JID` / `types.UserInfo` / `events.*` /
   proto 类型漏过 gomobile 边界**。
2. **panic 边界**：和现有导出方法一致，`defer c.recoverAsError("MethodName", &err)`。
3. **gate 复用**：需要连接 / 冷却 / risk-stop 判定的，复用现有 `checkSendGate` /
   `activeOperationRemainingLocked` / `riskRemainingLocked` 习惯，不另造一套。
4. **trace**：按 `TRACE_SCHEMA.md` MVP raw 口径记录（可含完整字段，仅本机）。
5. **JSON 形状文档化**：在 `SDK_API.md` 写明每个方法入参/返回 JSON 结构。

各方法语义建议：
- `GetSelfIdentity` → `{self_jid, jid_server, state, is_logged_in, is_connected, has_session_db, device_name}`。
- `GetUserInfo(jidsJson)` → 入参 JSON 数组，返回每个 jid 的 user info JSON（status / picture id / verified name 等可用字段）。
- `GetProfilePictureInfo(jid)` → 头像 url / id / type 的 JSON（拿不到返回明确空结构 + 原因）。
- `MarkRead(chatJid, messageIdsJson, senderJid)` → 标记已读，messageIdsJson 为数组。
- `SendPresence(state)` → `available` / `unavailable`。
- `SubscribePresence(jid)` → 订阅某 jid 的 presence。
- `ExportSessionDebug(path)` → 只写本机私有目录，导出当前 trace + session db 文件信息 +
  device store / credential 调试字段；**禁止任何网络上传，禁止提交进 git**。

测试：`bridge/*_test.go` 为每个新方法补单测（happy path + 错误路径 + JSON 形状）。

---

## 5. 阶段 B：SDK 模块细则

### B1. `:wa-sdk`（library module）
- 迁入 AIDL 契约、`BridgeForegroundService`（`:wa_bridge` 独立进程，dataSync 类型）、对
  `wamobile.aar` 的依赖与封装。
- 暴露 `WaBridgeClient`：
  - 生命周期：`bind(context)` / `unbind()`，封装 bindService + AIDL 连接管理与断连重试。
  - 业务方法：与 AIDL 一一对应（getSelfIdentity / getContacts / getGroups / getUserInfo /
    getProfilePictureInfo / sendText / sendTextMulti / markRead / sendPresence /
    subscribePresence / exportTrace / exportSessionDebug / clearSession / getState /
    getSafetyStatus）。
  - 事件：listener 接口 `onEvent(eventType, payloadJson)`（或强类型回调）；**集成方拿到的
    回调必须保证在主线程触发**——SDK 内部完成 `Looper.getMainLooper()` 切换，集成方无需自己
    处理跨进程线程切换。
  - 错误模型：定义 SDK 自己的异常 / 结果类型；不把原始 `RemoteException` 直接抛给集成方，
    包装后保留可读信息。
- Manifest：SDK 模块声明自己的 `:wa_bridge` ForegroundService 与所需权限，集成方 merge 即用。
- 能 `assembleRelease` 产出可被外部工程依赖的 `.aar`。

### B2. `:sample-app`（验证台）
- 依赖 `:wa-sdk`，**只**通过 `WaBridgeClient` 公开 API 调用；不直接引用 `wamobile.aar`、
  不直接写 AIDL stub、不直接 new 内部 Service 类。
- 每个 SDK API 一个按钮，点击显示原始返回 JSON / 错误；覆盖全部业务方法 + 登录/恢复/清除
  session + 状态/事件流展示（用于确认回调在主线程、事件不丢）。
- UI 不追求产品化聊天体验，职责是**证明 SDK API 可被外部按公开契约正常调用**。

### B3. 构建
- 更新 `android/build_debug_go126.sh`（或新增脚本）适配多模块：仍用 Go 1.26.3 重编
  `wamobile.aar`；能 `:sample-app` assembleDebug + `:wa-sdk` assembleRelease。
- AAR 随 Go 代码变化重编并提交（gomobile 产物非字节可复现，按功能验证为准，不做 SHA 门禁）。

---

## 6. 阶段 C：交付物（六类，缺一不可）

1. **AAR**：`wamobile.aar`（底层）+ `:wa-sdk` 产出的可集成 AAR。
2. **AIDL**：`IBridgeService` / `IBridgeCallback`（含新增方法）。
3. **Android Service 封装**：`:wa_bridge` ForegroundService + `WaBridgeClient`（绑定 / 线程 / 错误封装）。
4. **示例 App**：`:sample-app` 验证台。
5. **API 文档**：`SDK_API.md`——逐方法写用途、入参/返回 JSON 形状、线程语义（回调在主线程）、
   错误码/异常、前置条件、最小集成步骤（bind → 收事件 → 调用 → unbind）。
6. **验收 checklist**：`ACCEPTANCE_WAVE5.md`。

---

## 7. 安全 / 不变量（逐条遵守）

- 继续无云端、无远程、无队列/调度/存储。
- 单手机、单账号；不做多账号/多手机/多租户/后台隐藏群发。
- 所有导出 Go 方法 + 事件 goroutine 都有 panic recover。
- 跨边界只用基础类型 + JSON string，不漏 whatsmeow 内部类型。
- 跨进程回调最终回到 Main Looper 再交给集成方/UI。
- 核心业务能力必须有显式 wrapper/AIDL/SDK 方法；不得仅靠动态字符串分发暴露。
- raw trace / session debug 只落本机私有目录，禁止任何自动上传或外发；debug bundle 不提交 git。

---

## 8. 配套文档

- `ACCEPTANCE_WAVE5.md`
- `SDK_API.md`（本波产出）
- `SPEC_WAVE4.md` / `ACCEPTANCE_WAVE4.md` / `CLAUDE_REVIEW_HANDOFF_WAVE4_MVP.md` / `TRACE_SCHEMA.md`（继续生效）
- `GOMOBILE_CONSTRAINTS.md` / `ANDROID_PITFALLS.md` / `KNOWN_PITFALLS.md`（继续生效）
