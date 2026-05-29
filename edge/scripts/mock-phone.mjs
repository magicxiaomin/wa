#!/usr/bin/env node

import WebSocket from "ws";

const wsUrl = process.env.RELAY_WS_URL;
const token = process.env.RELAY_TOKEN;

if (!wsUrl || !token || token.length < 32) {
  console.error("Set RELAY_WS_URL and RELAY_TOKEN (>=32 chars).");
  process.exit(1);
}

const socket = new WebSocket(wsUrl, {
  headers: { Authorization: `Bearer ${token}` }
});

socket.addEventListener("open", () => {
  console.log(JSON.stringify({ event: "mock_phone_connected" }));
});

socket.addEventListener("message", (event) => {
  const frame = JSON.parse(String(event.data));
  const requestId = frame.request_id;
  if (frame.type === "contacts") {
    socket.send(JSON.stringify({
      type: "response",
      request_id: requestId,
      ok: true,
      result_json: JSON.stringify([
        { jid: "15550000001@s.whatsapp.net", name: "Mock One" },
        { jid: "15550000002@s.whatsapp.net", name: "Mock Two" }
      ])
    }));
    console.log(JSON.stringify({ event: "mock_contacts", request_id: shortId(requestId) }));
    return;
  }
  if (frame.type === "send") {
    const count = Array.isArray(frame.to_jids) ? frame.to_jids.length : 0;
    const result = Array.from({ length: count }, (_, index) => ({
      jid_suffix: `...000${index + 1}`,
      ok: true,
      server_msg_id: `mock-server-${Date.now()}-${index}`
    }));
    socket.send(JSON.stringify({
      type: "response",
      request_id: requestId,
      ok: true,
      result_json: JSON.stringify(result)
    }));
    console.log(JSON.stringify({ event: "mock_send", request_id: shortId(requestId), recipient_count: count }));
  }
});

socket.addEventListener("close", () => {
  console.log(JSON.stringify({ event: "mock_phone_disconnected" }));
});

function shortId(value) {
  return String(value || "").slice(0, 8);
}
