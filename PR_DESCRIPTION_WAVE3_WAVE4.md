# Wave 3 + Wave 4 PR 描述

## 本轮范围

本分支实现 Wave 3 的两个小功能：

1. 发给最多 3 个联系人：`SendTextMulti`，wrapper 层硬校验目标数 `1-3`，超过 3 个整体拒绝且一条不发。
2. 发到 1 个群：`GetGroups` 获取已加入群列表，单个群 JID 复用单发 `SendText`。

Wave 4 只新增验收骨架文档，未实现 Phase 5-7 的能力。

## 主要改动

- `bridge/client.go`
  - 新增 `SendTextMulti(toJidsJson, text, clientMsgId)`。
  - 重构发送门检查与底层单条发送，单发/多发复用。
  - 多人发送保留 risk-stop、fresh-link 冷却、operation backoff、进程内发送上限和每条随机节流。
  - 新增 `GetGroups()`，返回 JSON 数组。

- `bridge/whatsmeow_adapter.go`
  - 接入 `GetJoinedGroups`。
  - 允许单个 `@g.us` 群 JID 通过单发路径。

- Android
  - AIDL 新增 `getGroups()` / `sendTextMulti(...)`。
  - `BridgeForegroundService` 转发到 Go wrapper。
  - `MainActivity` 联系人列表支持多选，UI 层最多 3 个；群列表单选；发送结果显示在日志区。
  - 已用 Go 1.26.3 重编 `wamobile.aar`。

## 安全边界

- wrapper 层强制 `SendTextMulti` 最多 3 个目标，`>3` 直接拒绝。
- UI 层也限制最多选择 3 个联系人。
- `SendTextMulti` 只允许个人 JID，含 `@g.us` 或其他 server 时整体拒绝。
- 群发送只支持 1 个群，走单发路径；没有多群群发。
- 未扩大 `allowedTestNumbers`。
- 未移除发送频率、计数、risk-stop 限制。
- trace 不记录消息正文、完整号码、session key。
- 未修改 `TRACE_SCHEMA.md`，本轮未新增 trace 字段结构。

## 已验证

Go 测试：

```sh
GOCACHE=$HOME/.cache/codex-wa-tools/go-cache \
GOPATH=$HOME/.local/share/codex-wa-tools/go-path \
GOPROXY=https://goproxy.cn,direct \
GOSUMDB=off \
$HOME/.local/share/codex-wa-tools/go1.26.3/bin/go test ./bridge
```

结果：通过。

Android 构建：

```sh
./android/build_debug_go126.sh
```

结果：通过，生成 debug APK。

真机基础验证：

- APK 已安装到设备 `XT2453_2`。
- 应用可启动。
- `:wa_bridge` 进程存在。
- `libgojni.so` 加载成功。
- UI dump 确认 `Get Contacts` / `Get Groups` / `Send` / 5 行高列表区域显示正常。

## 跳过的验收

按项目 owner 指令，本轮跳过 Phase 4 真实送达验收。

未验证项：

- 真机选 2-3 个联系人发送并确认对方收到。
- 真机选 1 个群发送并确认群成员收到。
- 导出 trace 后确认多人发送/群发送真实事件链完整。

当前设备状态：点击 `Connect / QR` 后进入 `qr_generated`，说明旧 session 未恢复，需要重新扫码。为避免继续扫码，本轮未执行真实发送。

## Review 重点

1. `SendTextMulti` 是否真正做到 wrapper 层硬上限，且超限时 adapter 未被调用。
2. 多人发送的门检查是否只做一次，避免第 2 个收件人被 operation backoff 误伤。
3. 单个收件人失败时，其余收件人是否继续发送，结果 JSON 是否逐条标注。
4. 返回 JSON 是否没有完整 JID、完整号码、消息正文。
5. 群发送是否只允许单个 `@g.us` 通过单发路径，没有多群能力。
6. Android UI 的 3 人限制是否与 wrapper 层限制一致。
7. AIDL 变更后 `build_debug_go126.sh` 是否可在 reviewer 机器复现通过。

## 已知风险

- Phase 4 真实送达未验，本分支不能声称端到端“已送达通过”。
- 首次扫码后仍需遵守新 linked device 行为冷却，避免立即程序化发送。
- 国产 ROM 后台杀进程、dataSync 长时间常驻限制、账号风控风险仍是既有风险。
