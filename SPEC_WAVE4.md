# SPEC_WAVE4 · 第四波交接（远程触发发送 / Cloudflare Workers + Durable Objects · 单手机）

> 前置：Wave 3 已合并——Android 真机能扫码登录、收发 1:1 文本、发给最多 3 个联系人、发到群。
> 本波在仓库 magicxiaomin/wa 工作。
>
> **本文档为 Wave 4 的最终定稿，取代 `PRD_WAVE3_WAVE4.md` 中旧的（基于 VPS 的）Wave 4 设计部分。**
> 凡两者冲突，以本文件为准。Wave 3 部分仍以 `SPEC_WAVE3.md` 为准。

---

## 0. 定位（决定一切取舍）

个人兴趣研究项目，3-5 人小圈子用，明确不做大、不商业化、不规模化。一切设计以
"简单、够用、防止滑向群发/规模化"为准。遇到不确定的取舍，永远选更简单 + 更严格那个。

---

## 1. 本波目标（就一件事）

人不在手机边时，也能从一个网页远程触发"发给某个（最多 3 个）联系人一条文本"。
但 **手机始终是 WhatsApp 协议的唯一持有方，Cloudflare 边缘只是无状态转发器。**

**关键语义：远程触发是"立即发送一次"，不是"下任务"。**
每个 HTTP 请求 = 当下一次决定，转发给手机、等结果、回响应，执行完即一切结束、什么都不留。

**不做**：VPS、多手机/多账号/多租户、发群远程接口、媒体消息、任何队列/任务/调度/存储、
手机离线时的补发/排队。

---

## 2. 部署形态：方案 B（无 VPS，全在 Cloudflare 边缘）

```
浏览器（前端 Pages）
  │ POST /send  Bearer <token>   { to_jids:[≤3], text, client_msg_id }
  │ GET  /contacts  Bearer <token>
  ▼
Cloudflare Worker（无状态路由 + token 校验）
  │ 路由到固定 ID 的单例 DO（"phone"）
  ▼
Durable Object "phone"（单实例）
  • 持有手机的 WebSocket（唯一一条）
  • 内存 map<request_id, resolver>（在途请求关联表）
  • 限流计数（内存）
  ✗ 不开 DO 持久存储 / Queues / KV / D1 / Cron
  │ wss://域名/ws （手机主动连出，断线重连）
  ▼
手机 :wa_bridge 内的 WS client
  → 调已有的 SendTextMulti / GetContacts（≤3 上限、节流、risk-stop 全在这生效）
  → whatsmeow → WhatsApp
```

要点：
- **手机主动连出**到边缘（手机在 NAT/移动网后，不开任何监听端口）；边缘是 WS server，
  绝不反向连手机。
- **边缘无状态、不落盘**：Durable Object 只当"内存协调器 + WS 持有者"，重启/休眠唤醒后
  不持有任何 WhatsApp 状态。
- **单手机、单账号**：只有一个 DO（`idFromName("phone")`），只接一条手机 WS，新连接替换旧的。

---

## 3. 端点定义（全部"立即请求-响应"，无存储、无队列）

| 端点 | 鉴权 | 说明 |
|---|---|---|
| `GET /` | 无 | Pages 提供前端页面 |
| `GET /healthz` | 无 | 返回 200 + 手机 WS 是否在线；不泄露账号信息 |
| `GET /ws` | token | 手机作为 client 连入；同一时刻只接 1 条手机连接，新替旧 |
| `GET /contacts` | Bearer token | 经 WS 中继到手机 `GetContacts`，返回 `[{jid,name}]`；即时透传，**不在边缘缓存/落盘** |
| `POST /send` | Bearer token | 请求体 `{ to_jids:["jid1",...], text, client_msg_id }`；中继到手机 `SendTextMulti`，同步等结果再回 HTTP 响应 |

`/send` 处理流程（DO 内）：
1. 冗余校验：`to_jids` 数量 >3 → 400；任一为 `@g.us` → 400。
2. 限流：每分钟最多 10 次 → 超出 429。
3. 手机 WS 未连 → 503。
4. 生成 `request_id`，在 `map` 里挂 resolver，经 WS 推送指令帧。
5. `await` 该 resolver，带约 30s 超时。手机回帧（按 `request_id`）→ resolve →
   **删除 map 条目** → 作为 HTTP 响应返回逐个收件人结果。
6. 超时 → 504 + 删除 map 条目，**不重投、不缓存**。

---

## 4. 构建顺序（三个 Phase）

### Phase 5：边缘 relay（Worker + Durable Object）
- 目录 `edge/`，TypeScript + Cloudflare Workers + Durable Objects，`wrangler` 部署。
- 实现上述全部端点 + DO 单实例 + 在途关联表 + 限流 + 冗余校验。
- **token** 从 Worker secret 读（如 `RELAY_TOKEN`）；为空或 <32 字符 → 部署/启动校验失败，
  无默认值、不硬编码。
- **TLS** 由 Cloudflare 自动提供。
- **脱敏日志**：只记元信息（时间戳、request_id、收件人数量、ack 概况、错误码）；
  不记 token / 号码 / 正文。错误响应对外只给错误码，不泄露内部细节。
- 提供 **mock 手机 WS client**，使 Phase 5 可独立端到端验收。
- `edge/README.md`：设 secret、`wrangler deploy`、配 Pages、配自定义域名。

### Phase 6：手机端 WS 客户端
- 在 `bridge/` 新增 WS client 模块 + 少量 Kotlin 接线。
- 手机作为 WS client 主动连出 `wss://域名/ws`，token 鉴权；断线指数退避重连。
- 收到指令帧按 type 分发：`send` → `SendTextMulti`；`contacts` → `GetContacts`；
  回结果帧带回相同 `request_id`。
- **不写任何新发送逻辑**：≤3 上限、节流、risk-stop、发送计数全部由 wrapper 自动继承。
- 在 `:wa_bridge` 前台服务里拉起 WS client；UI 加开关启停"远程触发"，关掉则不连边缘。
- 只连一个 DO、只承载当前这一个账号。

### Phase 7：前端 + 真机端到端验收
- `edge/static/` 放极简 HTML/JS，Cloudflare Pages 托管。
- 输 token → `GET /contacts` 拉联系人 → 多选（**最多 3，选满其余置灰/提示**）→ 输文本 →
  发送 → 显示每个收件人结果。
- 手机离线时显示明确"手机离线，稍后再试"，不补发、不排队。
- 真机端到端：浏览器触发 → 手机发出 → 收件人收到。
- trace + 边缘日志脱敏复核。

---

## 5. 四个实现陷阱（务必避开）

1. **别把 relay 写成队列/任务系统**。在途请求只能"建 → 等 → 删"，超时即 504，不缓存不重投。
   一旦出现"待发列表/重试队列/定时/批量"，立刻违反红线。
2. **别用 DO 的持久存储**（transactional storage / SQLite），也别引入 Queues/KV/D1/Cron。
   DO 只当内存协调器 + WS 持有者。这是方案 B 最容易手滑的点。
3. **WS 方向是手机 → 边缘**。手机主动连出；边缘绝不反向连手机，手机也不开监听口。
4. **3 人上限的权威实现只在手机 wrapper**。边缘的 ≤3 校验只是冗余早拦，不能放宽、
   不能成为唯一校验。

---

## 6. 安全红线（逐条遵守，不可破）

1. 强 token：≥32 字符随机串，Worker secret 注入，无默认值/硬编码。
2. HTTPS 强制（Cloudflare 自动 TLS）。
3. 限流：每分钟最多 10 次 send 请求。
4. **【最重要】禁止任务/队列/调度/存储**：边缘只有"立即发送一次"语义，执行完即结束，重启即空。
   绝不实现 `/schedule`、`/task`、`/queue`、`/template`、定时/批量发送。
5. 手机 wrapper 的 3 人上限 / 节流 / risk-stop 全部继承——远程触发不绕过任何一条。
6. 边缘只支持个人发送（≤3 人），**不开放任何发群接口**——发群只保留手机本地操作；
   `/send` 遇到 `@g.us` 直接拒。
7. 日志脱敏：不记 token 明文 / 完整号码 / 消息正文，只记元信息和时间戳。
8. 失败返回明确错误码，不泄露内部细节。
9. 单手机、单账号：只有一个 DO（"phone"）、只接一条手机 WS。不做多账号/多租户/广播。

---

## 7. 配套文档
- `ACCEPTANCE_WAVE4.md` — 本波验收（具体条目）。
- 前几波的 SPEC_WAVE3 / KNOWN_PITFALLS / GOMOBILE_CONSTRAINTS / ANDROID_PITFALLS /
  TRACE_SCHEMA 仍全部有效。
- `PRD_WAVE3_WAVE4.md` — Wave 3 部分仍参考；Wave 4 部分以本文件为准（旧 VPS 设计作废）。
