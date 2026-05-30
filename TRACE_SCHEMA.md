# TRACE_SCHEMA · MVP Research Raw Trace 字段定义

> 本文件适用于 Wave 4 之后的 Android whatsmeow MVP 研究模式。
>
> 重要定位：trace/debug 是本机研究材料，不是生产日志、不是公开审计日志、不是 issue 附件。
> 本模式允许记录完整业务字段和认证调试材料，以便验证 whatsmeow Android 接口的真实行为。

---

## 1. 每条 trace 事件的结构

```json
{
  "ts": "2026-05-25T10:30:00.123Z",
  "seq": 42,
  "event": "message_sent",
  "state": "connected",
  "data": {}
}
```

字段说明：

- `ts`：UTC ISO8601 时间戳，带毫秒。
- `seq`：本进程内单调递增序号。
- `event`：事件类型。
- `state`：事件发生时 wrapper 记录的连接状态。
- `data`：事件原始业务数据。MVP research mode 下不做后四位脱敏。

---

## 2. MVP Research Raw Trace 允许记录的内容

以下内容允许写入本机 trace/debug 文件：

- 完整 `self_jid` / `UserIDString()` / 当前登录账号 JID。
- 完整联系人 JID、群 JID、PN JID、LID JID。
- 完整手机号或 JID user 部分。
- 消息正文。
- 收发目标、server message ID、receipt / ack 信息。
- QR code 完整内容。
- pairing code 完整内容（如果后续实现）。
- session/auth credentials、device store、sqlite session 调试材料。
- 底层错误原文，包括 whatsmeow / sqlite / network / protocol 错误。

这些字段可能包含可识别个人身份的信息，或者等同于 linked-device session 的登录钥匙。

---

## 3. 事件 data 示例

### `qr_generated`

```json
{
  "qr": "2@...",
  "qr_len": 220,
  "timeout_seconds": 20
}
```

### `paired`

```json
{
  "jid": "8613800000000@s.whatsapp.net",
  "jid_suffix": "...0000"
}
```

### `connected` / `session_restored`

```json
{
  "jid": "8613800000000@s.whatsapp.net",
  "jid_suffix": "...0000"
}
```

### `message_send_start`

```json
{
  "clientMsgId": "android-uuid",
  "to": "8613900000000@s.whatsapp.net",
  "text": "test message",
  "text_len": 12
}
```

### `message_sent`

```json
{
  "clientMsgId": "android-uuid",
  "server_msg_id": "3EB0...",
  "latency_ms": 320,
  "recipient_jid": "8613900000000@lid",
  "recipient_server": "lid",
  "used_lid": true
}
```

### `message_ack`

```json
{
  "server_msg_id": "3EB0...",
  "ack_level": 2,
  "latency_ms": 1500
}
```

### `message_failed`

```json
{
  "clientMsgId": "android-uuid",
  "error_code": "send_failed",
  "error": "raw whatsmeow error including target/text if present",
  "recipient_jid": "8613900000000@s.whatsapp.net",
  "recipient_server": "s.whatsapp.net",
  "used_lid": false
}
```

### `message_received`

```json
{
  "from_jid": "8613900000000@s.whatsapp.net",
  "from_suffix": "...0000",
  "text": "incoming message",
  "text_len": 16,
  "server_msg_id": "3EB0...",
  "ts": "2026-05-25T10:30:00.123Z"
}
```

### `risk_stopped`

```json
{
  "where": "sendText",
  "reason": "raw server/protocol error",
  "retry_after_seconds": 86400
}
```

### `self_identity`（建议新增）

```json
{
  "self_jid": "8613800000000@s.whatsapp.net",
  "jid_server": "s.whatsapp.net",
  "state": "connected",
  "is_logged_in": true,
  "is_connected": true,
  "has_session_db": true,
  "device_name": "WA-Android"
}
```

---

## 4. Session / Auth Debug Bundle（建议新增）

如实现 `ExportSessionDebug(path)`，允许在本机私有目录导出：

- 当前 trace。
- `whatsmeow.db` 文件信息或副本。
- device store / identity / credential 调试字段。
- 当前 `self_jid`、连接状态、session 文件状态。

限制：

- 不自动上传。
- 不写入云端。
- 不作为普通导出按钮暴露给非研究用户。
- 不把 debug bundle 提交到 git。

---

## 5. 审计口径

Claude / reviewer 审计时请按以下口径判断：

- 记录完整号码、完整 JID、消息正文：**符合 Wave 4 MVP research 需求**。
- 记录 QR / pairing code：**符合 Wave 4 MVP research 需求**。
- 记录 session/auth/device store 调试材料到本机：**符合 Wave 4 MVP research 需求**。
- 自动上传 trace/session/debug bundle：**不符合需求，应判为问题**。
- 引入 Cloudflare / VPS / 远程触发 / 队列 / 调度：**不符合需求，应判为问题**。
- 把 raw trace/debug 用于公开发布或分享：**不符合项目定位，应判为文档/流程风险**。

---

## 6. 长度与容量

- trace recorder 继续使用环形缓冲，默认最多保留 `5000` 条事件。
- raw trace 可能包含大字段，后续如验证媒体接口，需要单独设计媒体数据截断策略。
