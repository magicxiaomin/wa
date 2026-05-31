# REVIEW_WAVE5 · Claude Code 审核（PR #3 / feat/wave5-sdk-module）

> 审核对象：PR #3 / 分支 `feat/wave5-sdk-module`，HEAD `a8f9a05`。
> 权威需求：`SPEC_WAVE5.md` / `ACCEPTANCE_WAVE5.md` / `SDK_API.md` /
> `SPEC_WAVE4.md` / `ACCEPTANCE_WAVE4.md` / `CLAUDE_REVIEW_HANDOFF_WAVE4_MVP.md` / `TRACE_SCHEMA.md`。
> 定位：个人本地研究，单手机、单账号、本地运行、不上云、不远程、不规模化。

## 审核方法
- 本地 clone（`--filter=blob:none`），将 `bridge/` 包提取后用 Go 1.26.3 实际编译：`go build ./bridge/` → 退出码 0。
- Android 侧逐文件读源码 + gradle/manifest/AIDL；本机不跑 Android Gradle，`assembleRelease/assembleDebug` 以脚本与配置正确性 + 作者已声明的本地 BUILD SUCCESSFUL 为准。

---

## Verdict: **APPROVE**

模块拆分干净、Wave 5 新 API 全链路打通、gomobile 边界与跨进程/线程模型正确、本地研究红线全部守住。无阻断性问题。仅有文档与打包健壮性层面的非阻断项。

---

## 1. Blocking issues
无。

## 2. Non-blocking issues

1. **【文档 bug，建议合并前顺手改】`SDK_API.md` 的 `sendTextMulti` 返回示例字段名过时。**
   文档（`SDK_API.md` ~242 行）写返回 `{"jid_suffix":"1234", ...}`，但 Go 实际返回完整 `jid`（Wave 4 已从 `JIDSuffix`→`JID`；`client.go` `multiSendResult{ JID: target }`，`MainActivity.kt` 读 `jid`）。集成方按文档解析 `jid_suffix` 会拿到空串。改为 `jid` 即可。

2. **AAR 打包是"手工拆包"而非直接消费 `wamobile.aar`。**
   `build_debug_go126.sh` 把 gomobile 产物 `wamobile.aar` `unzip` 出 `classes.jar`→`wa-sdk/libs/wamobile-classes.jar`（`api files(...)`）+ `libgojni.so`→`wa-sdk/src/main/jniLibs/`。能工作且 `:wa-sdk:assembleRelease` 产出单一自洽 AAR。隐患：(a) 两个中间产物被提交进 git（`wamobile-classes.jar` 12KB、`libgojni.so` 31MB），而 `.gitignore` 又忽略了 `wa-sdk/libs/wamobile.aar` 本体——源 AAR 不入库、拆出来的碎片入库，只改 Go 不重跑脚本会导致碎片与源漂移；(b) 直接用 AGP 的 AAR 依赖更不易漂移。研究项目可接受，建议在脚本头注释"这些碎片是生成物，改 Go 必须重跑脚本"。

3. **`ExportSessionDebug` 路径约束只在 Android（FGS）层，Go 层不约束。**
   `BridgeForegroundService.privateFilesPath()` 把 `exportTrace/exportSessionDebug` 限制在 app `filesDir`（canonical + 前缀校验，写得好）。但 Go 的 `Client.ExportSessionDebug(path)` 接受任意路径——直连 AAR 而不经 SDK Service 的集成方拿不到这层防护。建议 `SDK_API.md` 注明"该约束由 SDK Service 层提供，直连 AAR 的调用方需自行约束路径"。

4. **`.gitignore` 未显式忽略 session-debug 导出目录。**
   实测 `session-debug.json` 只写 self_jid + db 路径/大小/存在性（不含 db 内容），trace 走既有 MVP raw 口径。`.gitignore` 已含 `wa-session/`、`trace.json`、`outputs/`，但没有 `session-debug/`。建议补 `session-debug/` 防手滑提交敏感导出。

## 3. 安全 / 边界（全部通过）

- 无 Cloudflare/VPS/relay/远程触发/Web 控制台（全代码 sweep 0 命中）。
- 无队列/定时/cron/调度/对象存储。
- 无多账号/多手机/多租户（单 client、单 session 目录）。
- 无 InvokeAPI / 动态字符串分发作核心入口（全部显式 AIDL+SDK 方法）。
- 无网络上传/第三方外发（bridge 无 net/http/okhttp/upload；ExportSessionDebug 仅本地 os.WriteFile）。
- gomobile 边界正确：导出签名只 `string`/`error`，复杂值走 JSON；`types.JID`/`UserInfo`/`events.*` 在 adapter 内转字符串，不跨边界。
- 7 个新导出方法全有 `recoverAsError`；事件 goroutine（consumeQR/handleWAEvent）有 recover。
- AIDL 回调经 `WaBridgeClient` 切回 Main Looper；不向集成方抛 `RemoteException`，统一包成 `WaBridgeException`。
- 文件导出有路径穿越防护（`privateFilesPath`）。
- `sample-app` 0 处碰 wamobile/AIDL/Service，仅用 `WaBridgeClient`（模块隔离达标）。
- 新读 API 走 `queryAdapter()`（risk-stop + 操作冷却 + connected 校验），安全闸门继承。

**收群消息缺口**：`handleMessage` 仍过滤 `message.Info.IsGroup`，主号发群不触发 `message_received`。属文档允许的已知缺口（SPEC_WAVE5 未要求收群消息），不阻断本轮，建议作为下一轮（按需）任务。

## 4. 测试

`bridge` 包 34 个测试函数；Wave 5 全部新 API 均有测试（含 happy path、bad-JSON、空参校验、`ExportSessionDebug` 本地落盘、`fakeWAAdapter` 实现完整接口）。`go build ./bridge/` 通过。

## 5. 真机回归建议 checklist

- [ ] `bind()` → 首登扫码 → `connected`；杀前台 App 再回来，`getSelfIdentity()` 仍 `is_connected=true`（`:wa_bridge` 进程存活）
- [ ] `getSelfIdentity` 返回正确 self_jid/jid_server/device_name/has_session_db
- [ ] `getUserInfo(["对方jid"])` 返回 status/verified_name/devices；传无效 jid，单条 `found=false`+`error`，不影响其余
- [ ] `getProfilePictureInfo` 有头像/无头像两种（无头像 `found=false`+原因，不崩）
- [ ] `markRead`：对一条 1:1 消息标已读，发出 `mark_read_success`；对方端显示已读
- [ ] `sendPresence(available/unavailable)` 不报错；`subscribePresence(对方)` 后收到 `presence_update`
- [ ] `sendTextMulti` 多收件人：确认返回 JSON 字段是 **`jid`**（验证文档修正项）
- [ ] `exportTrace`/`exportSessionDebug`：filesDir 内成功；**filesDir 外被 SDK 拒绝**（验证 `privateFilesPath`）
- [ ] 回调线程：在 listener 里直接 `setText` 不抛 `CalledFromWrongThreadException`（验证 Main Looper）
- [ ] `clearSession` 后 `getState` 回到未连接、可重新扫码登录
- [ ] 进程被系统杀后重连，不出现双 client（`ensureClient` 竞态）
- [ ] 第三方集成冒烟：空 App 只 `implementation(wa-sdk-release.aar)` + merge manifest，能 bind + getState（验证 AAR 自洽、manifest 合并）

## 6. 合并建议

**建议合并。** 无必须先修的阻断项。合并前强烈建议顺手改 **Non-blocking #1**（`SDK_API.md` 的 `sendTextMulti` 字段 `jid_suffix`→`jid`，避免误导集成方）。其余 #2/#3/#4 + 收群消息缺口可并入下一轮。
