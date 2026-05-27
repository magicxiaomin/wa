# Claude review round 1 PR 描述

## 判定汇总表

| 编号 | 判定 | 一句话理由 | 涉及文件:行号 |
| --- | --- | --- | --- |
| 1 | confirmed | `IBridgeCallback.onEvent` 原本不是 `oneway`，且 callback 投递失败没有日志。 | `android/app/src/main/aidl/com/magicxiaomin/wa/bridge/IBridgeCallback.aidl:4`; `android/app/src/main/java/com/magicxiaomin/wa/bridge/BridgeForegroundService.kt:126` |
| 2 | confirmed | `updateSafetyControls()` 每次事件都会新建 `Thread`，高频事件下会放大线程和 Binder 压力。 | `android/app/src/main/java/com/magicxiaomin/wa/MainActivity.kt:356` |
| 3 | confirmed | UI 只按字符串 JID 去重，Go 层没有做 PN/LID 语义归并，联系人仍可能重复。 | `android/app/src/main/java/com/magicxiaomin/wa/MainActivity.kt:257`; `bridge/whatsmeow_adapter.go:126` |
| 4 | false_positive | `server_msg_id` 是当前 trace schema 允许的关联字段，不包含正文、session key 或完整号码。 | `bridge/trace.go:104`; `bridge/trace.go:122`; `TRACE_SCHEMA.md` |
| 5 | false_positive | 当前导出方法和事件 goroutine 已有 recover；Go runtime fatal 类崩溃本身不可由 recover 兜住。 | `bridge/client.go:128`; `bridge/client.go:509`; `bridge/client.go:541`; `bridge/client.go:708` |
| 6 | confirmed | `ClearSession()` 原本只删目录和部分安全状态，Go client、Kotlin service、UI 本地状态没有完整复位。 | `bridge/client.go:476`; `android/app/src/main/java/com/magicxiaomin/wa/bridge/BridgeForegroundService.kt:70`; `android/app/src/main/java/com/magicxiaomin/wa/MainActivity.kt:157` |
| 7 | confirmed | 与问题 1 同源，`RemoteCallbackList` 投递异常原本被吞掉，排查跨进程回调问题困难。 | `android/app/src/main/java/com/magicxiaomin/wa/bridge/BridgeForegroundService.kt:126` |
| 8 | confirmed | 风险控制 PoC 可用，但 `sendCount` 仅内存、风险识别依赖文本、`sentAt` 未清理都是真实遗留点。 | `bridge/client.go:86`; `bridge/client.go:681`; `bridge/client.go:814` |
| 9 | needs_more_info | manifest 和 service 类型声明正确，但 dataSync 长时间常驻、国产 ROM 保活仍需要真机长跑证据。 | `android/app/src/main/AndroidManifest.xml:2`; `android/app/src/main/AndroidManifest.xml:21`; `android/app/src/main/java/com/magicxiaomin/wa/bridge/BridgeForegroundService.kt:141` |
| 10 | confirmed | `syncFullHistory=false` 已正确，但 trace 缺少历史同步/洪流指标，后续长跑时不易量化 UI 压力。 | `bridge/whatsmeow_adapter.go:57`; `bridge/client.go:609`; `bridge/trace.go:95` |
| A | confirmed | `c.sentAt` 发送时写入，原本收到对应 receipt 后不删除。 | `bridge/client.go:381`; `bridge/client.go:681` |
| B | confirmed | `connectionParts()` 覆盖 `c.cancel` 前没有取消旧 context，可能留下旧连接上下文。 | `bridge/client.go:495` |
| C | confirmed | `traceRecorder.events` 原本无上限，长跑或事件洪流会持续增长。 | `bridge/trace.go:20`; `bridge/trace.go:29` |
| D | confirmed | 默认 trace 对部分事件只保留很少字段，长跑排查信息不足。 | `bridge/trace.go:95` |
| E | confirmed | `sanitizeTraceString()` 命中敏感关键词会整字段替换，可能误伤可用错误信息。 | `bridge/trace.go:147` |
| F | confirmed | QR bitmap 在主线程逐点绘制，低端机或频繁刷新时可能卡 UI。 | `android/app/src/main/java/com/magicxiaomin/wa/MainActivity.kt:223`; `android/app/src/main/java/com/magicxiaomin/wa/MainActivity.kt:390` |
| G | confirmed | UI status 直接展示实时 JSON，实时回调允许正文，但状态栏展示完整 JSON 有隐私暴露风险。 | `android/app/src/main/java/com/magicxiaomin/wa/MainActivity.kt:223` |
| H | false_positive | `SendTextForTest` 是桌面/单测测试入口，Android 正式发送走 `SendText`，白名单没有被扩大。 | `bridge/client.go:63`; `bridge/client.go:281`; `bridge/client.go:293` |
| I | confirmed | `sendText`/`exportTrace` AIDL 仍是同步调用，长操作可能占用 Binder 调用线程。 | `android/app/src/main/aidl/com/magicxiaomin/wa/bridge/IBridgeService.aidl:14` |
| J | confirmed | Contact/PushName/BusinessName 每次都发 `contacts_synced`，高频同步时会放大 UI 刷新压力。 | `bridge/client.go:609` |

## confirmed 项最小改动计划

按审核报告“修复优先级建议（5 条）”排序：

| 优先级 | 项 | 最小变更方案 | 测试影响 | 是否需重新跑 `build_debug_go126.sh` 真机回归 |
| --- | --- | --- | --- | --- |
| 1 | 问题 1 / 7 | 修改 `IBridgeCallback.aidl` 为 `oneway`；`BridgeForegroundService.emit()` 对 callback 投递失败打 `Log.w`。 | Kotlin 难单测；用真机确认 `qr_generated` / `message_received` 仍能回主进程 UI。 | 是，AIDL 接口变更。 |
| 2 | 问题 6 | Go `ClearSession()` 清 `wa/started/hadSession/cancel/sentAt`；Kotlin service 清 `client=null`；UI 清 contacts/conversation/selectedJid/pending。 | 新增 `TestClearSessionResetsClientState`；真机需谨慎验证 clear 后重新扫码链路。 | 是，Go AAR 和 Android 行为都变更。 |
| 3 | 问题 2 | `MainActivity` 增加 single-thread executor 或 HandlerThread，合并/节流 safety 查询，避免每事件建线程。 | 增加手动回归：高频事件下 UI 不阻塞，按钮状态正确。 | 是，Android 逻辑变更。 |
| 4 | 额外 A / C | receipt 命中后删除 `sentAt`；`traceRecorder` 改 5000 条环形缓冲。 | 新增 `TestReceiptDeletesSentAtEntry`、`TestTraceRecorderCapsEvents`。 | 是，Go AAR 变更。 |
| 5 | 问题 8 | 风险识别从字符串匹配补充 whatsmeow 事件/枚举路径；持久化或更清晰地暴露 send limit 状态。 | 需要构造 ConnectFailure/TemporaryBan 单测和一次真机负向验证。 | 是，Go AAR 变更。 |

其余 confirmed 项最小计划：问题 3 在 Go 层增加联系人规范化/别名合并；问题 10/D/E 扩展 trace 安全字段并同步更新 `TRACE_SCHEMA.md` 与 `trace_test.go`；B 在覆盖 `c.cancel` 前取消旧 cancel；F 把 QR bitmap 生成移到后台；G 将 status 展示改为脱敏摘要；I 将耗时 AIDL 方法拆成异步事件或 `oneway`；J 对 `contacts_synced` 做去抖/合并。

## 本次修复范围

本轮只实施前三个指定修复点，控制爆炸半径：

1. 问题 1 / 7：`IBridgeCallback.onEvent` 改 `oneway`，callback 失败加 `Log.w`。
2. 问题 6：`ClearSession` 完整复位 Go/Kotlin/UI 三层状态。
3. 额外 A / C：receipt 后删除 `sentAt`；trace 改 5000 条环形缓冲。

未修改 `REVIEW_CLAUDE_2026-05-27.md`。未扩大 `allowedTestNumbers`，未移除发送频率限制，未把消息正文写进 trace。trace 字段结构没有新增/删除，仅改变内存保留上限，因此本轮未改 `TRACE_SCHEMA.md`。

## 真机回归 checklist

- [x] `GOPROXY=https://goproxy.cn,direct $HOME/.local/share/codex-wa-tools/go1.26.3/bin/go test ./bridge` 通过。
- [x] `./android/build_debug_go126.sh` 通过，已用 Go 1.26.3 重编 `wamobile.aar` 并生成 debug APK。
- [x] sqlite 驱动仍为 `modernc.org/sqlite`，未引入 cgo sqlite。
- [ ] 安装 `android/app/build/outputs/apk/debug/app-debug.apk` 到真机，保留现有 session 验证 `session_restored`。
- [ ] 真机验证 `message_received` 从 `:wa_bridge` 进程经 AIDL 回主进程，再切 `Looper.getMainLooper()` 更新 UI。
- [ ] 真机验证发送指定联系人 1 对 1 文本后收到 `message_sent` 和后续 receipt ack。
- [ ] 谨慎验证 Clear Session：确认 UI 清空、service client 释放、重新启动后需重新扫码；该项会破坏当前登录态，建议单独安排。

## 已知未做项与原因

- 问题 2 未做：需要引入 executor/节流，属于下一轮 UI 稳定性改动。
- 问题 3 未做：PN/LID 语义去重需要更多真实联系人样本验证。
- 问题 8 只修了 `sentAt`：风险识别枚举化和 sendCount 持久化留到下一轮。
- 问题 9 未做：需要真机长跑和国产 ROM 后台限制验证。
- 问题 10、D、E 未做：trace 字段增强会牵动 `TRACE_SCHEMA.md`，本轮不扩大 schema。
- B、F、G、I、J 未做：均为 confirmed，但不属于本轮前三个指定修复点。
