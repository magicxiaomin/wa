# TRACE_SCHEMA · trace 字段定义与脱敏

## 每条 trace 事件的结构
```json
{
  "ts": "2026-05-25T10:30:00.123Z",   // ISO8601 带毫秒
  "seq": 42,                          // 单调递增序号
  "event": "message_sent",            // 见 API_CONTRACT 事件类型
  "state": "connected",               // 当时的连接状态
  "data": { }                         // 事件特定数据（已脱敏）
}
```

## 各事件的 data 字段示例
- `qr_generated`: `{ "qr_len": 220 }` （记长度即可，**不记二维码内容本身到导出文件**，或仅运行时打印终端）
- `connected`: `{ "jid_suffix": "...3000" }` （只记号码后四位）
- `message_send_start`: `{ "clientMsgId": "uuid", "to_suffix": "...3000" }`
- `message_sent`: `{ "clientMsgId": "uuid", "server_msg_id": "3EB0...", "latency_ms": 320 }`
- `message_ack`: `{ "server_msg_id": "3EB0...", "ack_level": 2, "latency_ms": 1500 }`
- `message_failed`: `{ "clientMsgId": "uuid", "error_code": "...", "error": "短描述" }`
- `disconnected`: `{ "reason": "...", "will_reconnect": true }`
- `error`: `{ "where": "...", "message": "..." }`
- `remote_relay_started`: `{ "url_host": "worker.example.com" }`（只记 host，不记 token）
- `remote_relay_connected`: `{ "url_host": "worker.example.com" }`
- `remote_relay_disconnected`: `{ "url_host": "worker.example.com" }`
- `remote_relay_error`: `{ "where": "remoteRelay", "code": "timeout" }`
- `remote_relay_stopped`: `{}`

## 脱敏要求（导出 trace.json 前必须执行）
**绝对不能出现在 trace.json 里：**
- ❌ session key / auth credentials / 任何密钥
- ❌ 消息正文明文（连测试消息内容也不记，最多记长度或 hash）
- ❌ 完整电话号码（只记后 4 位 + 国家码，如 `86...3000`）
- ❌ 二维码 / pairing code 的完整内容
- ❌ 远程 relay token / 带 token 的 URL

**可以记录：**
- 事件类型、时间戳、序号、连接状态
- 服务器返回的 message ID（这是消息标识，非内容）
- 错误码 / 错误短描述（确认不含敏感数据）
- 各种延迟（ack 延迟、连接耗时）
- 号码后四位（用于关联，不泄露完整号码）

## 额外建议记录的事件（增强可观测性）
- `network_changed`（PoC 桌面可选；Android 阶段用 ConnectivityManager）
- `reconnect_attempt`（第几次重连）
- `qr_expired`（二维码过期刷新）
- `keepalive`（心跳，可选）
