# Claude Code 审核报告 — 2026-05-27

> 评审范围：基于 `CLAUDE_REVIEW_HANDOFF.md` 列出的"高优先级问题 1–10"，覆盖
> Android 第二波 PoC 的 bridge service、Go wrapper、IPC、trace 与前台服务设计。
> 审核者：Claude Opus 4.7 (1M context)。审核基线 commit：当前 main 分支 HEAD。

---

## 整体判断

核心链路（QR/session → contacts → send → receive）已通，trace 脱敏单测覆盖到位。
**但要继续做下去，三件事必须先修**：

1. `IBridgeCallback.onEvent` **改成 `oneway`**——避免事件 goroutine 被主进程 UI 反压。
2. `MainActivity` 里 ad-hoc `Thread` 改 executor + 节流——消除事件洪流时的线程/binder 压力。
3. `ClearSession` 之后客户端状态没复原——修 `Stop()` 不清 `c.wa`、Kotlin Service 不清 `client = null` 的问题；否则"清完再扫码"永远过不了。

其它属于交付前打磨。

---

## 高优先级问题逐条答复

### 1. `BridgeForegroundService.emit()` 跨边界安全性 — ⚠️ 有问题，需要改

事件路径：whatsmeow goroutine → Go `Client.emit` → gomobile JNI → Kotlin
`EventCallback.onEvent` → `RemoteCallbackList.beginBroadcast` → 对每个注册的
`IBridgeCallback` 调用 `onEvent`。

**核心隐患**：`android/app/src/main/aidl/com/magicxiaomin/wa/bridge/IBridgeCallback.aidl`
的 `onEvent` **没有声明 `oneway`**。这意味着：

- 每条事件从 `:wa_bridge` 进程向主进程派发时，binder 调用是**同步**的，会阻塞调用线程直到主进程返回。
- 这条调用线程就是 whatsmeow 的事件 goroutine（gomobile 的 JNI 调用是同步的）。
- 一旦主进程主线程繁忙（例如 `handleEvent` 里 `setImageBitmap(makeQr(qr))` 这种 720×720 逐像素操作），whatsmeow 的事件循环就被压住，进而压住网络读循环。
- 历史同步洪流场景下尤其明显。

**修复**：

```aidl
// IBridgeCallback.aidl
interface IBridgeCallback {
    oneway void onEvent(String eventType, String payloadJson);
}
```

oneway 后 binder 在 bridge 进程侧异步排队、kernel 缓冲；牺牲极端拥塞时事件丢失的可能，
换来事件 goroutine 不阻塞。这是这条链路应有的语义。

附带：`BridgeForegroundService.kt:126` 的 `runCatching { ... }` 直接吞掉异常，
建议至少 `Log.w`，否则中间一段坏了不会有任何信号。

---

### 2. `MainActivity.updateSafetyControls()` 的 `Thread` — ⚠️ 该改成 executor

`MainActivity.kt:346` 每次都 `Thread { ... }.start()`，并且：

- `handleEvent` 末尾每次都调用一次。
- `runBridgeCall`、`sendMessage`、`loadContacts` 完成后也各调一次。
- `runBridgeCall` 本身也是 `Thread { ... }.start()`，`sendMessage` 里还有一个独立的 `Thread { service?.sendText(...) }.start()`。

历史同步、ack 风暴等场景下会瞬时创建几十甚至上百个一次性 Thread，每个还要做一次
binder roundtrip 拿 `safetyStatus`。PoC 用没事，但已经不是"能不能跑"，是"长时间跑会不会被 OOM 或卡爆"。

**建议**：单一线程的 `Executors.newSingleThreadExecutor()`（或 `lifecycleScope + Dispatchers.IO`）
替换所有 ad-hoc `Thread`，并把"安全状态刷新"做成节流（例如 200ms 合并多次请求）。

---

### 3. JID/LID 去重 — ⚠️ UI 侧不够，应在 Go 侧合并

当前实现：

- `whatsmeow_adapter.go:245` `preferredContactJID` 把 LID → PN（如果 `LIDs.GetPNForLID` 成功），否则原样返回。
- `MainActivity.kt:270` UI 用 `seenJids` set 按返回的 JID 字符串去重。

问题：

- 如果 `GetPNForLID` 失败，同一个人会同时以 `xxx@s.whatsapp.net` 和 `yyy@lid` 出现，UI 去重抓不到（两个 JID 字符串不同）。验收记录里 "Robert 2" 出现两次正是这一类。
- `GetContacts` 在主 map 为空时才回退到 DB（`whatsmeow_adapter.go:136`），不会合并两边。

**建议**：在 Go `GetContacts` 末尾用 canonical key（resolved PN 失败时也可用 `User` 字符串）
做最后一道去重，UI 只做展示。这是协议层应该解决的，不应每个客户端都自己写一遍。

---

### 4. trace 里的 `server_msg_id` — ✅ 可以接受

`server_msg_id` 是 WhatsApp 服务端分配的消息标识（如 `3EB00D2071B5DCCAF9B62F`），
不含密钥、不含正文。它会泄露"发了/收了一条消息"的事实和顺序，但 trace 本身就在记
send/ack 事件——脱敏后没它就没法把 `message_sent` 和 `message_ack` 对上，丢失可观测性收益大于风险。
**保留原样**。如果将来对外披露 trace 才考虑 hash。

---

### 5. panic recover 覆盖度 — ✅ 基本完整

逐项检查：

- 所有导出方法（`Start/Stop/Connect/Disconnect/SendText/SendTextForTest/GetContacts/ResolveJID/SafetyStatus/ExportTrace/ClearSession/GetState/RequestPairing`）都用 `defer c.recoverAsError(...)` 或局部 recover ✓
- 事件 goroutine（`consumeQR`、`handleWAEvent`、`reconnectAfterManualLogin`）都有 recover ✓
- `emit()` 在调用 Kotlin 回调外面也包了 recover ✓ —— 这点很重要，否则 JNI 抛上来的异常会炸 Go 进程

漏的小点：

- `handleMessage` / `handleReceipt` 没有自己的 recover，但它们的调用者 `handleWAEvent` 有，OK。
- `traceRecorder.add` 不需要。

如 handoff 所说，Go runtime fatal（`bulkBarrierPreWrite`）不能被 recover；这个用
Go 1.26.3 工具链规避，是正确的处置。**panic 路径上没看到漏网**。

---

### 6. `ClearSession` 守护 — ⚠️ 缺确认对话框，且有功能性 bug

UI 侧只长按、没确认 dialog——对一个会清空账号的破坏性操作，单步触发太弱。**至少加一个 AlertDialog**。

**更要紧的是隐藏 bug**：`Client.ClearSession`（`bridge/client.go:476`）调用 `Stop()`，
但 `Stop()` 不会清 `c.wa` 与 `c.started`，只把 `c.cancel = nil`。结果：

- `c.wa` 仍然指向一个已 `Disconnect()` + `Close()` 的 adapter。
- 下次 `Connect()` 进 `connectionParts()` 时 `c.cancel == nil` → 返回 `"client is not started"`。
- 同时 Kotlin 侧 `BridgeForegroundService.kt:19` 的 `private var client: Client?` 也没被重置，`ensureClient` 直接返回那个死客户端。

结论：**当前 ClearSession 之后想再扫码登录，必须杀掉 `:wa_bridge` 进程**。
这正是 ACCEPTANCE 文档说"延迟"的那项，但根因写出来值得修：

1. `ClearSession` 末尾把 `c.wa`、`c.started`、`c.hadSession` 都清零（或者更彻底地，重新从零构造 adapter）。
2. Kotlin Service 在 `clearSession()` AIDL 调用后把 `client = null`。
3. UI 同时清空 contacts / conversation / selectedJid。

---

### 7. `RemoteCallbackList` 错误处理 — ⚠️ 可以更宽容，目前过于安静

`BridgeForegroundService.kt:122` 的 `runCatching { callbacks.getBroadcastItem(i).onEvent(...) }`
把异常完全吞掉。死亡的 client 会被 `RemoteCallbackList` 通过 DeathRecipient 自动清理，
这部分没问题；但同步抛出的 `RemoteException`、`RuntimeException` 没有 log，
没法看出主进程到底有没有接到事件。**至少加 `Log.w(TAG, "callback delivery failed", it)`**。

如果按问题 1 把 `IBridgeCallback.onEvent` 改成 `oneway`，调用基本不会同步抛异常，
这条就退化成偶发的 RemoteException，更应有 log。

---

### 8. fresh-link / risk-stop 节流 — ⚠️ PoC 够用，两个语义缺陷需要记

时序常量看起来合理（联系人 2min、发消息 10min、单操作 5s、rate-limit 30min、临时封 24h、单进程最多 5 条）。但：

- **`sendCount` 只在内存里**（`bridge/client.go:86`），进程一重启就清零。`maxSendsPerRun=5` 是把"防误发"做成了"每次启动 5 条"。短期 PoC 没事，但任何把它当作真实"日上限"用的下游都会失望。要么持久化到磁盘按日窗口，要么 docstring 显式声明 "per-process not per-day"。
- **`enterRiskStopIfNeeded` 的关键词匹配**（`bridge/client.go:814`）依赖错误字符串里出现 `463`/`rate-overlimit`/`429` 等子串。whatsmeow 错误信息变化时会有静默假阴性。建议同时监听 `events.TemporaryBan`（已经在做 ✓）和 `events.ConnectFailure` 的 `Reason` 枚举值，少依赖文本匹配。
- `freshLinkedAt` 仅在 `PairSuccess` 事件触发时写入磁盘。如果在 pair 与磁盘写之间进程崩溃（很罕见），就丢了 cooldown。可接受。
- `riskUntil` 持久化在 `risk-stop.json` ✓，重启可恢复 ✓。
- **`c.sentAt` map 永不清理**（`bridge/client.go:382` 写，`bridge/client.go:680` 读）。长时间运行（30/60 分钟保活测试）会持续增长。低优先级，但 10/60min 测试做完前最好顺手加一个写完发 ack 后 `delete(c.sentAt, serverID)`，或者按时间 TTL（≥7 天的扔掉）。

---

### 9. 前台服务 / 通知 (Android 13/14/15) — ✅ 声明完整，⚠️ dataSync 长跑限制需自测

声明面：

- `POST_NOTIFICATIONS` ✓ + 运行时申请 ✓（`MainActivity.kt:399`）
- `FOREGROUND_SERVICE` + `FOREGROUND_SERVICE_DATA_SYNC` ✓
- `foregroundServiceType="dataSync"` ✓
- `startForeground(id, notif, FOREGROUND_SERVICE_TYPE_DATA_SYNC)` 在 API ≥29 用 3 参版本 ✓
- `startForegroundService` → `onCreate` 立即 `startForeground` ✓（不会触发 5 秒 ANR）

⚠️ **dataSync 在 Android 14+ 有累计 6 小时/天的运行限制**
（[官方文档](https://developer.android.com/about/versions/14/changes/fgs-types-required#data-sync)）。
"长期常驻收消息"严格说不属于 dataSync 的语义；Android 13/14 没有 `messaging` 类型可选，
目前用 dataSync 是务实选择，但**10/30/60 分钟测试通过 ≠ 24 小时通过**——长跑时系统会
主动停服务，且 ROM 厂商的额外杀进程更早。这点已在 PITFALLS/ACCEPTANCE 提到，
建议长跑测试里专门记录系统主动停服务的时间点。

⚠️ 用户拒绝 `POST_NOTIFICATIONS` 时：foreground service 仍可启动，但没有可见通知，
部分 OEM ROM 会更快杀。`requestNotificationsIfNeeded()` 没有处理拒绝场景下的引导。低优先级。

---

### 10. `syncFullHistory=false` / 历史洪流 — ⚠️ 设置正确，但日志覆盖不全

`whatsmeow_adapter.go:61` 设置了 `ManualHistorySyncDownload = true` 和
`EmitAppStateEventsOnFullSync = false`，这能阻断 whatsmeow 自动下载历史。
**但 trace 没有任何针对"突发事件量"的指标**——没法事后判断有没有出现过 history flood。
如果想留下证据，加一条简单的批处理事件计数（每 5 秒一次窗口聚合 `EventContactsSynced` /
`EventMessageReceived` 计数），即可让 trace 回答"刚才到底涌进来多少事件"。

另外 `bridge/client.go:604` 把 `*events.Contact / PushName / BusinessName` 三种事件都
映射到 `EventContactsSynced`——任意一个联系人推送名变化都会触发一次。这本身没问题，
但加上前述 `updateSafetyControls()` 的每事件 `Thread` 开销，洪流时被放大。
问题 2 修了这里就跟着舒服。

---

## 额外发现（不在 10 题之内但建议处理）

| # | 位置 | 问题 | 严重度 |
|---|---|---|---|
| A | `bridge/client.go:382` / `bridge/client.go:680` | `c.sentAt` 只写不删，长跑内存泄漏 | 中 |
| B | `bridge/client.go:499` `connectionParts` | 直接覆盖 `c.cancel` 而不先 `cancel()` 旧的；快速二次 `Connect()` 会泄漏旧 ctx | 低 |
| C | `bridge/trace.go` | `events` slice 无上限，长跑内存增长；建议加环形缓冲或 N 条上限 | 中 |
| D | `bridge/trace.go:75` | `EventBridgeStarted / EventConnecting / EventSessionInvalid / qr_success` 都落到 default → 数据被全清；`session_invalid` 的 `reason` 是有用的 debug 信号，应单独加 allow | 低 |
| E | `bridge/trace.go:138` | `sanitizeTraceString` 用 `strings.Contains` 匹配 `auth/credential/session/token/secret/ key`，一旦命中**整个字段被替换成 `[redacted]`**。`connect_failure` 错误里只要带 "auth token" 就全部丢失，定位会变难。建议仅替换匹配到的关键词周围片段，或专门白名单错误码 | 低 |
| F | `android/app/src/main/java/com/magicxiaomin/wa/MainActivity.kt:387` `makeQr` | 720×720 用 `setPixel` 在主线程跑（518400 次调用），每次 QR 刷新都会卡 UI。改 `setPixels(IntArray, ...)` 单次提交即可 | 低 |
| G | `android/app/src/main/java/com/magicxiaomin/wa/MainActivity.kt:224` | `status.text = "$eventType\n$payloadJson"` —— 每条事件直接把 raw JSON 渲染到 status TextView。`message_received` 时这里会包含正文和完整 `from_jid`。功能上 OK（实时回调本来就允许含正文），但截屏/录屏给第三方看时容易意外泄露 | 低 |
| H | `bridge/client.go:281` `SendTextForTest` | 仍保留白名单 `allowedTestNumbers = {"15551234567"}` 占位号。生产 release 之前要清理或在 build flag 里去掉，免得有人误用 | 低 |
| I | `android/app/src/main/aidl/com/magicxiaomin/wa/bridge/IBridgeService.aidl` | `sendText` AIDL 不是 `oneway`，每次发送 binder 线程被同步占用 `sendTextTimeout=45s`。`MainActivity` 这边已经包了 Thread，但如果未来其他 client 在主线程直接调，会 ANR。建议把 `sendText` / `exportTrace` 等可能长耗时的 AIDL 方法也声明 `oneway`（结果通过 callback 回传） | 中 |
| J | `bridge/client.go:604` | `Contact / PushName / BusinessName` 三种事件都映射到 `EventContactsSynced`，对 UI 来说会被打成噪音。可以在 Go 侧做节流（200ms 合并） | 低 |

---

## 修复优先级建议（如果只挑 5 个做）

按 ROI 从高到低：

1. **AIDL `IBridgeCallback.onEvent` 改 `oneway`**（问题 1）—— 改一行 AIDL，回避所有"主线程慢导致 bridge 阻塞"的连锁问题。
2. **`ClearSession` 状态机修复**（问题 6）—— 让 ACCEPTANCE 里那项延迟项可以收尾。
3. **`MainActivity` 引入 single-thread executor + 节流**（问题 2）—— 长跑测试稳定性的基础。
4. **`c.sentAt` 加清理 + `trace.events` 加上限**（额外 A、C）—— 长跑必备。
5. **`enterRiskStop` 同时识别 `events.ConnectFailure.Reason` 枚举**（问题 8）—— 减少文本匹配假阴性。

其余项可以排在第三波或长跑测试期间穿插处理。
