# KNOWN_PITFALLS · 已知坑清单（务必逐条对照）

这些是本项目特有的、踩过或预判到的坑。训练数据里大概率没有这些项目特定知识。**请逐条对照你的实现。**

---

## 1. 【最致命】whatsmeow 事件必须在 Connect 之前正确挂载，否则拿不到二维码
whatsmeow 的二维码、连接状态、发送回执**全部通过事件机制异步产生**，不是函数返回值。
- 二维码：通过 QR channel（`client.GetQRChannel(ctx)`）或 connect 前订阅，登录前必须先拿到这个 channel 并起 goroutine 消费它。
- 其它事件：通过 `client.AddEventHandler(handler)` 注册。
- **顺序很重要**：先 `AddEventHandler` 和拿 QR channel，**再** `Connect()`。反了就收不到二维码，表现为"连接了但永远没有码、无法登录"。
- 这是之前一版方案的致命错误（只 Connect 不挂事件），务必避免。

## 2. 全新设备 vs 已有 session 的分支
- `container.GetFirstDevice()` 在**全新安装**时返回一个空 device（需要走扫码登录）；在**已登录过**时返回带凭证的 device（直接恢复）。
- 代码要正确处理两种情况：device.ID == nil → 新登录走 QR；device.ID != nil → 直接 Connect 恢复。
- 这是"session 恢复"验收能否通过的关键。

## 3. 【必做】Go panic 会杀掉整个进程，必须 recover
- Go 里未捕获的 panic 会**终止整个程序**。在 Android 阶段，这意味着**连 Android app 一起闪退**。
- wrapper 的每个导出方法、以及事件处理 goroutine 内部，都要有 `defer func(){ recover() }()`，把 panic 转成 `error` 事件上报，而不是让它炸掉进程。
- PoC 阶段就要做对，这是验收项。

## 4. 回调不在主线程/主 goroutine
- whatsmeow 事件在它内部的 goroutine 触发。你的 callback 会在那个 goroutine 里被调用。
- PoC（桌面）：注意并发安全（trace 写入要加锁）。
- 注释里必须标明这一点，提醒 Android 阶段：Kotlin 收到回调后必须 `runOnUiThread` / 切主线程，否则更新 UI 会崩。

## 5. cgo + sqlite 的坑 —— 建议用纯 Go sqlite 避开
- whatsmeow 的 session store 常用 `mattn/go-sqlite3`，它**依赖 cgo**。cgo 会让后续 Android 交叉编译显著复杂（要配 NDK 的 C 编译器）、并增大体积。
- **建议**：评估用纯 Go 的 sqlite 驱动 `modernc.org/sqlite`（无 cgo），whatsmeow 的 sqlstore 支持指定驱动。若纯 Go 驱动能跑通，后续 Android 化省大量麻烦。
- PoC 阶段就用纯 Go 驱动验证可行性，是高价值的提前探路。

## 6. context cancellation 与优雅退出
- `Connect()` 用的 context 要可取消；`Stop()`/`Disconnect()` 时取消它，确保 whatsmeow 的后台 goroutine 退出，避免泄漏。
- 程序退出前要优雅关闭连接，否则 session 可能处于不一致状态。

## 7. 二维码会过期，要处理刷新
- QR channel 会陆续吐出多个二维码（旧的过期、推新的）。消费 QR channel 的 goroutine 要循环处理，每次更新展示，而不是只读第一个。

## 8. whatsmeow 版本与 API 漂移
- whatsmeow 更新较频繁，API（事件类型、proto 路径、store 接口）可能与网上旧示例不一致。**以你 `go get` 到的实际版本的 godoc 为准**，不要照抄旧博客。
- `go.mau.fi/whatsmeow` 是正确的模块路径（不是 GitHub 镜像路径）。

## 9. 发送消息的 JID 构造
- 个人聊天 JID 形如 `<国家码><号码>@s.whatsapp.net`，号码不带 + 不带空格。
- 发送前最好用 `client.IsOnWhatsApp([]string{...})` 校验号码是否注册，用返回的规范 JID，而不是手拼。
- PoC 白名单里的号码请用完整国家码格式。

## 10. 合规与封号
- whatsmeow 是非官方库，连接的是个人号的 Linked Device，违反 WhatsApp 条款，有封号风险。
- **PoC 必须用一个不重要的测试小号**，绝不用主力号。
- 这也是为什么 PoC 要先跑——万一这个用法会被快速封号，要在投入 Android 工程前就发现。
