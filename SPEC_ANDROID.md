# SPEC_ANDROID · 第二波交接（Android 集成）

> 前置：第一波桌面 Go PoC 已验证通过（能连接、能发消息、session 能恢复）。
> 本波目标：把已验证的 whatsmeow 核心**编进 Android**，实现获取联系人 + 收发指定消息。
> 仍在同一仓库 magicxiaomin/wa 工作，桌面 PoC 代码作为复用基础。

---

## 0. 一句话目标

在一台 Android 真机上，基于第一波的 whatsmeow wrapper（用 gomobile 编成 AAR），实现：
1. 扫码登录 + session 恢复（复用 PoC 已验证的逻辑）
2. **获取联系人列表**
3. **接收**指定联系人发来的消息
4. **发送**消息给指定联系人
5. 全程 trace

**本波明确不做**：发信体验优化（UI 做到能用即可，不追求好用）。

---

## 1. 与第一波的关键差异：这次要"收"消息

第一波是"只发不收"，引擎用完即断。**本波要接收消息，引擎必须保持在线**，因此：
- 引擎放在**独立进程 + ForegroundService** 里常驻（这次它是必要的，不是过度设计）。
- 需要处理 Android 后台限制、前台服务类型、通知权限。

这是本波架构比第一波重的根本原因。

---

## 2. 范围

### 必做
- gomobile bind 把 wrapper 编成 AAR，接入原生 Android 工程（Kotlin）
- 独立进程 `:wa_bridge` + ForegroundService 承载引擎
- 扫码登录 / session 恢复（复用 PoC 逻辑）
- 获取联系人：返回 `[{jid, name}]` 列表给 UI
- 接收消息：whatsmeow 收到消息 → 事件上报 → UI 能看到「谁发来什么」
- 发送消息：UI 选联系人 + 输入文本 → 发送 → 拿到 server_msg_id
- 最小可用 UI（能完成上述操作即可，**不做体验优化**）
- trace 记录连接 + 收 + 发的完整生命周期

### 禁止做（本波范围外）
- ❌ 发信体验优化（快捷查找、模板、预览等 —— 留到第三波）
- ❌ 群发 / 多账号 / 联系人批量导入
- ❌ 群组消息、媒体消息（本波只做 1对1 文本）
- ❌ 风控规避 / 静默发送
- ❌ ContentProvider 对外暴露

### 防滥用约束（延续第一波）
- 发送仍走 wrapper 内的校验；本波可放宽白名单（因为要真实收发联系人消息），
  但**保留发送频率/计数的基本节流**，防止演变成群发。

---

## 3. 推荐架构（这次的重型是必要的）

```
主进程（UI）
  ├─ Compose 界面：联系人列表 / 会话 / 输入发送 / 扫码
  ├─ ViewModel
  └─ Repository ──┐
                  │ IPC（AIDL Bound Service + 回调）
独立进程 :wa_bridge │
  ├─ ForegroundService（常驻，承载引擎，收消息必需）
  ├─ wamobile.aar（gomobile 编出的 whatsmeow wrapper）
  ├─ Connection State Machine
  ├─ 收消息 handler → 事件上报
  ├─ 发送 + 联系人查询
  └─ Room：联系人 / 消息 / session 元数据本地缓存
        ↓ 加密 WebSocket（whatsmeow 多设备协议）
  WhatsApp 服务器
```

---

## 4. 构建顺序（严格按此，每步有验收门）

### Phase A — Wrapper 扩展（先在桌面做，不碰 Android）
第一波的 wrapper 只有"发"。本波要先在**桌面**给它加"收"和"取联系人"，并验证：
- A1. 加 `GetContacts()` → 返回联系人 JSON 列表
- A2. 加接收消息的事件：whatsmeow 收到消息 → `message_received` 事件（含发送方 + 文本）
- A3. 桌面验证：能收到别人发来的消息、能列出联系人
- **门：桌面上收发联系人都通，再进 Android。**（这样 Android 阶段只解决"编进手机"，不掺杂协议逻辑）

### Phase B — gomobile 编译成 AAR
- B1. 把 wrapper 用 `gomobile bind` 编成 `wamobile.aar`（先 arm64-v8a）
- B2. 处理 cgo/sqlite（优先 modernc.org/sqlite 纯 Go 驱动，见坑清单）
- B3. 新建 Android Studio 工程，引入 AAR，能编译通过、真机能加载 .so
- **门：真机能加载 AAR，调一个简单方法（如 GetState）成功返回。**

### Phase C — BridgeService（独立进程 + 前台服务）
- C1. `:wa_bridge` 独立进程 + ForegroundService（dataSync 类型 + 通知权限）
- C2. 在 Service 里初始化 wrapper、挂事件回调
- C3. 复现 PoC：真机扫码登录 + session 恢复
- **门：真机扫码登录成功，重启 app 后 session 恢复。**

### Phase D — IPC + Repository
- D1. AIDL 定义主进程↔Bridge 进程的接口（发指令、查联系人）
- D2. 事件回传（Bridge → 主进程 → UI），注意跨进程 + 跨线程
- D3. Repository 封装，UI 只跟 Repository 打交道
- **门：UI 能通过 IPC 拿到连接状态和联系人列表。**

### Phase E — 收发消息 + 最小 UI
- E1. 联系人列表页（展示 GetContacts 结果）
- E2. 会话页：显示收到的消息 + 输入框发送
- E3. 收消息：`message_received` 事件 → 经 IPC → UI 实时显示
- E4. 发消息：选联系人 + 文本 → 发送 → 显示 server_msg_id / 状态
- **门：能选一个联系人，发消息他能收到，他回复我能看到。**

### Phase F — 收尾
- F1. trace 覆盖收/发/连接全生命周期
- F2. Disconnect / ClearSession
- F3. 按 ACCEPTANCE_ANDROID 全量验收

---

## 5. 配套文档（同目录，必读）
- `API_CONTRACT_ANDROID.md` — wrapper 新增方法/事件 + Kotlin 侧接口
- `ANDROID_PITFALLS.md` — Android/gomobile 特有坑（**逐条对照**）
- `ACCEPTANCE_ANDROID.md` — 本波验收
- 第一波的 `KNOWN_PITFALLS.md` / `GOMOBILE_CONSTRAINTS.md` / `TRACE_SCHEMA.md` 仍然有效，继续遵守。

---

## 6. 最重要的一条
**Phase A 必须在桌面完成并验证后，才进 Phase B。**
原因：把"加收消息/联系人功能"和"编进 Android"两件事分开，任何一步出问题都能立刻定位是协议逻辑问题还是 Android 集成问题。绝不要在 Android 环境里同时调试这两件事——那会让排查变成噩梦。
