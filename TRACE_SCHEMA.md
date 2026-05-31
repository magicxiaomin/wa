# TRACE_SCHEMA · Trace / Debug 字段定义

本文件定义 SDK 内部 trace 与 session debug 导出的数据形态。trace/debug 用于本机排查、接口验证和回归分析。

## 1. Trace 事件结构

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
- `data`：事件业务数据。

## 2. Trace / Debug 可能包含的内容

trace/debug 可能包含以下字段，使用者需要按敏感调试材料处理：

- 当前登录账号 JID / `UserIDString()`。
- 联系人 JID、群 JID、PN JID、LID JID。
- 手机号或 JID user 部分。
- 消息正文。
- 收发目标、server message ID、receipt / ack 信息。
- QR code 内容。
- pairing code 内容（如果后续实现）。
- session/auth credentials、device store、sqlite session 调试材料。
- 底层错误原文，包括 sqlite / network / protocol 错误。

这些字段可能包含可识别个人身份的信息，或者等同于 linked-device session 的登录钥匙。导出的 trace/debug bundle 不应提交到 git、issue、review 附件或外部服务。

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
  "error": "raw send error",
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

### `self_identity`

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

## 4. Session / Auth Debug Bundle

`ExportSessionDebug(path)` 在 App 私有目录导出：

- 当前 trace。
- `whatsmeow.db` 文件信息或副本。
- device store / identity / credential 调试字段。
- 当前 `self_jid`、连接状态、session 文件状态。

约束：

- 不把 debug bundle 提交到 git。
- 不把 debug bundle 作为公开日志或 issue 附件。
- 直连底层 AAR、不经 SDK Service 的调用方，需要自行约束导出路径。

## 5. 长度与容量

- trace recorder 使用环形缓冲，默认最多保留 `5000` 条事件。
- trace 可能包含大字段；如后续验证媒体接口，需要单独设计媒体数据截断策略。
