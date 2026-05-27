# ACCEPTANCE_ANDROID · 第二波验收 checklist

按 Phase 分组，每个 Phase 的门都通过才进下一个。

## 2026-05-27 Android 真机实测记录

设备：Android 真机 `N0WR2G0009`

账号：`18205924392`，连接后 UI 显示 `jid_suffix: ...4392`

构建工具链：
- AAR 使用 Go `1.26.3` + gomobile 重编。
- sqlite 驱动使用 `modernc.org/sqlite` 纯 Go 驱动。
- Go `1.25.0` 构建的 AAR 曾在发送时触发 `fatal error: bulkBarrierPreWrite: unaligned arguments`，已通过 Go `1.26.3` 重编规避。

已通过：
- AAR 编译、Android 工程编译、真机加载 `.so`。
- `:wa_bridge` 独立进程 + ForegroundService 启动。
- session 恢复成功，无需重新扫码，UI 显示 `connected {"jid_suffix":"...4392"}`。
- GetContacts 成功，UI 显示 `Contacts: 29`。
- 联系人列表默认显示多行并可滚动。
- 选择 `Robert 2 / 85255804693@s.whatsapp.net` 后发送 1 对 1 文本成功。
- 发送结果：`SENT c25ad4ee 3EB00D2071B5DCCAF9B62F`。
- 收到发送 ACK：`ack_level=1`，延迟约 `4151ms`。
- 对方回复实时显示成功：`message_received`，正文 `收到`，底部 UI 显示 `IN ...9165: 收到`。
- Disconnect 真机验证通过，UI 显示 `disconnected {"reason":"manual_disconnect","will_reconnect":false}`，服务进程未崩溃。
- Export Trace 真机验证通过，生成 `/data/user/0/com.magicxiaomin.wa/files/wa-trace.json`，已确认连接/断开事件脱敏，不含 session key、消息正文、完整号码。
- 同一进程内 lifecycle trace 已覆盖连接/session 恢复/断开/重连/发送/ACK：
  `message_send_start -> message_sent -> message_ack`，发送正文 `trace_test_1030` 未进入 trace，目标号码未完整进入 trace。
- 本轮 trace 发送结果：`SENT b6f1d6e6 3EB076ED716F6A0D9707DA`，ACK `ack_level=1`，延迟约 `2677ms`。
- 联系人 UI 去重已修复并真机验证：后端返回中 `Robert 2` 曾重复显示导致 `Contacts: 30`；安装去重修复后 UI 回到 `Contacts: 29`，`Robert 2` 只显示一次。
- 后台 5 分钟短测通过：主进程与 `:wa_bridge` 均存活，`BridgeForegroundService` 仍为 foreground service，`isForeground=true`，通知 ongoing，未见 bridge 崩溃/断连日志。
- 锁屏 2 分钟短测通过：主进程与 `:wa_bridge` 均存活，未见 bridge 崩溃/断连日志。
- 同一进程内 `message_received` trace 补齐完成：收到新消息正文 `测试`，UI 实时显示；导出的 trace 包含 `message_received`，仅记录 `from_suffix`、`server_msg_id`、`text_len`、`ts`，未记录正文和完整号码。
- 当前未生成 `risk-stop.json`。

未完成 / 暂未执行：
- 同一进程内 trace 的收消息脱敏已补齐；若需要“连接 + 发 + 收 + 断开”严格全部位于同一份 trace，需要再跑一次包含全部动作的完整脚本化回归。
- Go 普通 panic recover 已有单元测试覆盖；Go runtime fatal 无法 recover，本次通过升级 Go 工具链规避。
- ClearSession 后重新扫码登录尚未在 Go `1.26.3` AAR 上重跑。该项会破坏当前 session，需要单独确认后执行。
- 后台/锁屏已做短测；10/30/60 分钟长期保活、后台/锁屏状态下实时收消息仍未专项验证。

当前判定：
- 第二波核心功能链路已通过：`扫码/恢复 session → 看到联系人 → 给指定联系人发消息他收到 → 他回复我看到`。
- Phase F 还剩 ClearSession 重扫回归、严格单文件全动作 lifecycle trace 回归、长期保活专项。

## Phase A（桌面扩展 wrapper）
- [ ] 桌面 wrapper 新增 GetContacts()，能返回联系人 JSON 列表
- [ ] 桌面能接收到别人发来的 1 对 1 文本消息（message_received 事件含发送方+正文）
- [ ] 收消息路径区分：给 UI 的回调含正文，写 trace 的不含正文（脱敏）
- [ ] 桌面收发联系人消息全通

## Phase B（AAR 编译）
- [ ] gomobile bind 成功产出 wamobile.aar（arm64-v8a）
- [ ] 用纯 Go sqlite（modernc.org/sqlite）避开 cgo（或说明为何没用）
- [ ] Android 工程引入 AAR 编译通过
- [ ] 真机能加载 .so，调 GetState() 等简单方法成功返回

## Phase C（BridgeService）
- [ ] :wa_bridge 独立进程启动，ForegroundService 常驻（含通知）
- [ ] POST_NOTIFICATIONS 权限正确申请，通知可见
- [ ] 真机扫码登录成功，进入 connected
- [ ] 重启 app 后 session 恢复，无需重新扫码

## Phase D（IPC）
- [ ] AIDL 接口定义，主进程能调用 Bridge 进程
- [ ] 事件能从 Bridge 进程经 IPC 回传到主进程
- [ ] 回调正确切主线程（无线程崩溃）
- [ ] UI 能拿到连接状态和联系人列表

## Phase E（收发 + UI）
- [ ] 联系人列表页能显示 GetContacts 结果
- [ ] 能选一个联系人发文本，对方实际收到，UI 显示 server_msg_id/状态
- [ ] 对方回复时，UI 能实时显示收到的消息
- [ ] 首次登录的历史同步洪流不导致 UI 卡死/崩溃

## Phase F（收尾）
- [ ] trace 覆盖连接/收/发全生命周期，且脱敏（不含 session key / 正文 / 全号码）
- [ ] Disconnect 能释放连接
- [ ] ClearSession 后能重新扫码登录
- [ ] Go panic 被 recover，不导致 app 崩溃

## 本波最终判定
- [ ] 在一台 Android 真机上：扫码登录 → 看到联系人 → 给指定联系人发消息他收到 → 他回复我看到。
      这条完整链路通了 = 第二波成功。
- [ ] 明确范围：本波**不评判发信体验好不好**，只评判通路是否打通。

## 不阻塞通过的已知风险（记录但不在本波解决）
- 国产 ROM 杀进程导致收消息不稳（留待后续 ROM 适配）
- 封号风险（继续用测试小号观察）
- dataSync 前台服务长期常驻的系统限制（记录实测行为）
