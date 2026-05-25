# ANDROID_PITFALLS · Android / gomobile 特有坑（逐条对照）

第一波的 KNOWN_PITFALLS 仍然全部有效（事件挂载、panic recover、cgo、context 等）。
本文件是 Android 集成阶段**新增**的坑。

---

## 1. 【最致命】回调跨进程 + 跨线程的双重边界
事件从 whatsmeow goroutine 出来，要到达 UI，经过两道边界：
- 边界1（跨线程）：whatsmeow goroutine → 不是主线程
- 边界2（跨进程）：:wa_bridge 进程 → 主进程
路径：whatsmeow goroutine → wrapper 回调 → AIDL 跨进程 → 主进程 → `Looper.getMainLooper()` 切主线程 → 更新 UI。
**任何一段省了切换都会崩或丢事件。** 这是本波最容易出错、最难排查的地方，务必一开始就设计对。

## 2. gomobile bind 的类型限制（再次强调，Android 阶段会真碰到）
- 导出方法签名只能用 string/int/bool/[]byte/自定义interface。
- 复杂数据（联系人列表、消息对象）一律 JSON 字符串。
- whatsmeow 的 types.JID / events.* / proto 类型**绝不能**出现在导出签名里。
- 回调必须用 Go interface（Kotlin 实现），不能用 Go func 类型。

## 3. cgo + sqlite → 优先 modernc.org/sqlite
- whatsmeow 默认 sqlite 驱动 mattn/go-sqlite3 依赖 cgo，Android 交叉编译要配 NDK 的 C 工具链，复杂且增大体积。
- **优先用 modernc.org/sqlite（纯 Go，无 cgo）**，让 gomobile 编译大幅简化。
- 第一波若已用纯 Go 驱动，这里直接延续。

## 4. ForegroundService 在 Android 14/15 的限制（收消息常驻必须面对）
- Android 14（API 34）：前台服务必须声明 `foregroundServiceType`。本场景用 `dataSync`。
- `dataSync` 类型有**总运行时长限制**（系统会限制），长期常驻可能被系统约束。需测试在你的目标机型上的实际行为。
- Android 15：对 dataSync 后台启动进一步收紧。
- 启动前台服务的时机有限制（不能随意从后台启动）。

## 5. POST_NOTIFICATIONS 权限（Android 13+）
- 前台服务的常驻通知，在 Android 13（API 33）+ 需要运行时申请 `POST_NOTIFICATIONS` 权限。
- 没授权 → 通知不显示 → 前台服务存活率下降。UI 要引导用户授权。

## 6. 独立进程的初始化陷阱
- `:wa_bridge` 是独立进程，它有自己的 Application.onCreate。
- 如果你在 Application 里做了主进程才该做的初始化，Bridge 进程会重复执行或出错。
- 要用 `getProcessName()` 判断当前进程，分别初始化。

## 7. 国产 ROM 的激进杀进程
- 小米/华为/OPPO/vivo 等的省电策略远超原生 Android，会杀前台服务、冻结后台、断网。
- 收消息依赖常驻连接，在这些 ROM 上可能不稳。
- 本波先在原生 Android（如 Pixel 模拟器/真机）验证通路；ROM 适配留作已知风险，不在本波深入解决。

## 8. session 文件路径
- 用 `context.getFilesDir()` 的私有目录存 session。
- 注意 Bridge 进程和主进程访问的是同一个 app 的私有目录（同 app 不同进程共享 filesDir），路径要一致。

## 9. 收消息会触发历史同步洪流
- 首次登录时 whatsmeow 可能同步大量历史消息/联系人，事件会瞬间涌入。
- 回调和 UI 要能扛住突发批量事件（别每条都同步刷 UI，做批处理/节流）。
- `syncFullHistory` 建议设 false，减少首次洪流。

## 10. AAR 体积与 ABI
- 含 whatsmeow + sqlite 的 AAR 约十几 MB，正常。
- 先只编 arm64-v8a（真机基本都是）。需要模拟器测试再加 x86_64。

## 11. gomobile 工具链本身的坑
- gomobile 维护不算活跃，对 Go 版本、NDK 版本有兼容性要求，可能要试几个版本组合。
- `gomobile init` 和 `gomobile bind` 的环境（ANDROID_NDK_HOME、Go 版本）要配对。
- 这是本波最可能卡时间的工程环节，预留 buffer。
