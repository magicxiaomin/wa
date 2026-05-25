# ACCEPTANCE_ANDROID · 第二波验收 checklist

按 Phase 分组，每个 Phase 的门都通过才进下一个。

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
