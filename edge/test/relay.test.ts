import { env } from "cloudflare:test";
import { describe, expect, it } from "vitest";
import worker from "../src/index";

const TOKEN = "test-token-1234567890-1234567890";

function relayFetch(path: string, init: RequestInit = {}): Promise<Response> {
  return worker.fetch(new Request(`https://relay.test${path}`, init), env);
}

function authHeaders(extra: Record<string, string> = {}): Headers {
  return new Headers({ Authorization: `Bearer ${TOKEN}`, ...extra });
}

async function connectMockPhone(): Promise<WebSocket> {
  const response = await relayFetch("/ws", { headers: authHeaders({ Upgrade: "websocket" }) });
  expect(response.status).toBe(101);
  expect(response.webSocket).toBeTruthy();
  const socket = response.webSocket!;
  socket.accept();
  return socket;
}

function nextFrame(socket: WebSocket): Promise<Record<string, unknown>> {
  return new Promise((resolve) => {
    socket.addEventListener(
      "message",
      (event) => {
        resolve(JSON.parse(String(event.data)) as Record<string, unknown>);
      },
      { once: true }
    );
  });
}

function sendBody(toJids: string[], text = "hello"): string {
  return JSON.stringify({
    to_jids: toJids,
    text,
    client_msg_id: `web-${crypto.randomUUID()}`
  });
}

describe.sequential("Wave 4 relay", () => {
  it("rejects missing and wrong tokens", async () => {
    expect((await relayFetch("/contacts")).status).toBe(401);
    expect((await relayFetch("/contacts", { headers: { Authorization: "Bearer wrong" } })).status).toBe(401);
  });

  it("rejects >3 recipients and group JIDs before reaching the phone", async () => {
    const phone = await connectMockPhone();
    const overLimit = await relayFetch("/send", {
      method: "POST",
      headers: authHeaders({ "content-type": "application/json" }),
      body: sendBody([
        "15550000001@s.whatsapp.net",
        "15550000002@s.whatsapp.net",
        "15550000003@s.whatsapp.net",
        "15550000004@s.whatsapp.net"
      ])
    });
    expect(overLimit.status).toBe(400);
    expect(await overLimit.json()).toEqual({ error: "too_many_recipients" });

    const group = await relayFetch("/send", {
      method: "POST",
      headers: authHeaders({ "content-type": "application/json" }),
      body: sendBody(["120363000000000000@g.us"])
    });
    expect(group.status).toBe(400);
    expect(await group.json()).toEqual({ error: "group_not_allowed" });
    phone.close();
  });

  it("returns 503 when the phone is offline", async () => {
    const response = await relayFetch("/send", {
      method: "POST",
      headers: authHeaders({ "content-type": "application/json" }),
      body: sendBody(["15550000001@s.whatsapp.net"])
    });
    expect(response.status).toBe(503);
    expect(await response.json()).toEqual({ error: "phone_offline" });
  });

  it("relays contacts and send requests through the mock phone", async () => {
    const phone = await connectMockPhone();

    const contactsPromise = relayFetch("/contacts", { headers: authHeaders() });
    const contactsFrame = await nextFrame(phone);
    expect(contactsFrame.type).toBe("contacts");
    phone.send(JSON.stringify({
      type: "response",
      request_id: contactsFrame.request_id,
      ok: true,
      result_json: JSON.stringify([{ jid: "15550000001@s.whatsapp.net", name: "One" }])
    }));
    expect(await (await contactsPromise).json()).toEqual([{ jid: "15550000001@s.whatsapp.net", name: "One" }]);

    const sendPromise = relayFetch("/send", {
      method: "POST",
      headers: authHeaders({ "content-type": "application/json" }),
      body: sendBody(["15550000001@s.whatsapp.net", "15550000002@s.whatsapp.net"])
    });
    const sendFrame = await nextFrame(phone);
    expect(sendFrame.type).toBe("send");
    expect(sendFrame.to_jids).toHaveLength(2);
    phone.send(JSON.stringify({
      type: "response",
      request_id: sendFrame.request_id,
      ok: true,
      result_json: JSON.stringify([
        { jid_suffix: "...0001", ok: true, server_msg_id: "server-1" },
        { jid_suffix: "...0002", ok: true, server_msg_id: "server-2" }
      ])
    }));
    expect(await (await sendPromise).json()).toEqual([
      { jid_suffix: "...0001", ok: true, server_msg_id: "server-1" },
      { jid_suffix: "...0002", ok: true, server_msg_id: "server-2" }
    ]);
    phone.close();
  });

  it("times out without retaining stale request state or replaying", async () => {
    const phone = await connectMockPhone();
    const first = relayFetch("/send", {
      method: "POST",
      headers: authHeaders({ "content-type": "application/json" }),
      body: sendBody(["15550000001@s.whatsapp.net"])
    });
    const timedOutFrame = await nextFrame(phone);
    const timeoutResponse = await first;
    expect(timeoutResponse.status).toBe(504);
    expect(await timeoutResponse.json()).toEqual({ error: "timeout" });

    phone.send(JSON.stringify({
      type: "response",
      request_id: timedOutFrame.request_id,
      ok: true,
      result_json: "[]"
    }));

    const second = relayFetch("/send", {
      method: "POST",
      headers: authHeaders({ "content-type": "application/json" }),
      body: sendBody(["15550000001@s.whatsapp.net"])
    });
    const next = await nextFrame(phone);
    expect(next.request_id).not.toBe(timedOutFrame.request_id);
    phone.send(JSON.stringify({
      type: "response",
      request_id: next.request_id,
      ok: true,
      result_json: JSON.stringify([{ jid_suffix: "...0001", ok: true, server_msg_id: "server-after-timeout" }])
    }));
    expect(await (await second).json()).toEqual([{ jid_suffix: "...0001", ok: true, server_msg_id: "server-after-timeout" }]);
    phone.close();
  });

  it("rate limits send requests at 10 per minute", async () => {
    const phone = await connectMockPhone();
    for (let i = 0; i < 6; i++) {
      const promise = relayFetch("/send", {
        method: "POST",
        headers: authHeaders({ "content-type": "application/json" }),
        body: sendBody(["15550000001@s.whatsapp.net"])
      });
      const frame = await nextFrame(phone);
      phone.send(JSON.stringify({
        type: "response",
        request_id: frame.request_id,
        ok: true,
        result_json: "[]"
      }));
      expect((await promise).status).toBe(200);
    }
    const limited = await relayFetch("/send", {
      method: "POST",
      headers: authHeaders({ "content-type": "application/json" }),
      body: sendBody(["15550000001@s.whatsapp.net"])
    });
    expect(limited.status).toBe(429);
    expect(await limited.json()).toEqual({ error: "rate_limited" });
    phone.close();
  });
});
