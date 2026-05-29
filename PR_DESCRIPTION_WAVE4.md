# Wave 4 PR 描述

## 本轮范围

一次性实现 `SPEC_WAVE4.md` 的 Phase 5/6/7：

1. Cloudflare Workers + Durable Objects relay，单手机、单 DO（`phone`）、无队列/无存储。
2. 手机端主动连出 `wss://.../ws`，收到远程指令后复用 wrapper 的 `SendTextMulti` / `GetContacts`。
3. `edge/static/` 极简网页：输 token、拉联系人、最多选 3 人、立即发送、显示逐个结果。

## 主要改动

- `edge/`
  - `src/index.ts`：实现 `/healthz`、`/ws`、`/contacts`、`/send` 和 `PhoneRelayDurableObject`。
  - `test/relay.test.ts`：mock phone WebSocket 覆盖未授权、冗余校验、离线、超时清理、限流、成功路径。
  - `static/`：Pages 静态页面。
  - `scripts/mock-phone.mjs`：本地 mock 手机 WS client。
  - `README.md`：secret、部署、Pages、mock 测试说明。

- `bridge/remote_relay.go`
  - 新增 `StartRemoteRelay` / `StopRemoteRelay` / `RemoteRelayStatus`。
  - 手机主动连出 WS，token 走 `Authorization: Bearer`。
  - 指令 `send` 只调用 `SendTextMulti`；指令 `contacts` 只调用 `GetContacts`。
  - 指数退避重连；事件和 trace 只记录 host、错误码等元信息。

- Android
  - AIDL 增加远程 relay 启停/状态接口。
  - `BridgeForegroundService` 转发到 Go wrapper。
  - `MainActivity` 增加远程触发 URL、token 输入和开关。
  - 已用 Go 1.26.3 重编 `wamobile.aar`。

## 自检结果

- Phase 5
  - [x] 无 token / 错 token → 401
  - [x] `/send` >3 → 400；`@g.us` → 400
  - [x] 超过 10 次/分钟 → 429
  - [x] 手机 WS 未连接 → 503
  - [x] 手机不回 → 504，且 late response 不影响后续请求
  - [x] mock 手机 WS 正常路径 → 200
  - [x] token <32 → Worker 返回 `server_misconfigured`，手机端也拒绝启动
  - [x] 未使用 KV/D1/Queues/R2/Cron/DO storage API
  - [x] 日志只含 request_id 短前缀、类型、数量、错误码；不含 token/号码/正文

- Phase 6
  - [x] 手机端实现主动连出 `wss://.../ws`
  - [x] 指数退避重连
  - [x] `send` → `SendTextMulti`
  - [x] `contacts` → `GetContacts`
  - [x] wrapper 硬上限、节流、risk-stop 不绕过
  - [x] UI 有远程触发开关；关闭后停止连接
  - [x] 手机不开监听端口

- Phase 7
  - [x] 页面可输 token、拉联系人、多选 ≤3、发送并展示逐项结果
  - [x] 页面离线错误显示“手机离线，稍后再试”
  - [x] Android 真机安装启动，远程触发 UI 出现
  - [ ] 未做真实 Cloudflare 域名 + 真机 WhatsApp 端到端送达验收

## 测试

```sh
GOCACHE=$HOME/.cache/codex-wa-tools/go-cache \
GOPATH=$HOME/.local/share/codex-wa-tools/go-path \
GOPROXY=https://goproxy.cn,direct \
GOSUMDB=off \
$HOME/.local/share/codex-wa-tools/go1.26.3/bin/go test ./bridge
```

结果：通过。

```sh
cd edge
npm run check
npm test
```

结果：通过。说明：本地 Workers runtime 最新支持的 compatibility date 是 `2025-09-06`，测试时对 `2026-05-28` 有 fallback warning。

```sh
./android/build_debug_go126.sh
```

结果：通过。

真机基础验证：

- 设备：`XT2453_2`
- `adb install -r android/app/build/outputs/apk/debug/app-debug.apk`：成功
- UI dump 确认远程触发 URL/token/开关可见

## 安全红线对照

1. 强 token：Worker secret 注入；无默认值；<32 拒绝。
2. HTTPS/WSS：手机端只接受 `wss://.../ws`。
3. 限流：DO 内存计数 10/min。
4. 无任务/队列/调度/存储：没有 `/schedule`、任务表、补发、KV/D1/Queues/R2/Cron，也未调用 DO storage。
5. 3 人上限/节流/risk-stop：远程只调用现有 wrapper。
6. 边缘不发群：`@g.us` 在 `/send` 直接 400。
7. 日志脱敏：不记 token、完整号码、正文。
8. 失败错误码明确：401/400/429/503/504 等。
9. 单手机单账号：固定 DO name `phone`，新手机 WS 替换旧连接，不做多租户。

## 已知未做

- 真实 Cloudflare 部署、自定义域名、Pages 绑定和真机 WhatsApp 送达，需要 reviewer/owner 提供域名和 32+ token 后回归。
- `npm audit` 报告 dev dependency 链上 2 moderate / 2 high，未自动 `audit fix --force`，避免引入破坏性升级；运行时代码不打包 `node_modules`。
