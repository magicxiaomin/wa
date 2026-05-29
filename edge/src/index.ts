export interface Env {
  PHONE_RELAY: DurableObjectNamespace;
  RELAY_TOKEN?: string;
  RELAY_TIMEOUT_MS?: string;
}

type ErrorCode =
  | "server_misconfigured"
  | "unauthorized"
  | "not_found"
  | "invalid_request"
  | "too_many_recipients"
  | "group_not_allowed"
  | "rate_limited"
  | "phone_offline"
  | "timeout"
  | "phone_error";

interface PendingRequest {
  resolve: (frame: PhoneResponseFrame) => void;
  timeout: ReturnType<typeof setTimeout>;
}

interface PhoneResponseFrame {
  type: "response";
  request_id: string;
  ok: boolean;
  result_json?: string;
  error_code?: string;
}

interface SendRequestBody {
  to_jids?: unknown;
  text?: unknown;
  client_msg_id?: unknown;
}

const MAX_TOKEN_LENGTH_MIN = 32;
const MAX_SENDS_PER_MINUTE = 10;
const MAX_RECIPIENTS = 3;
const DEFAULT_TIMEOUT_MS = 30_000;

export default {
  async fetch(request: Request, env: Env): Promise<Response> {
    let token: string;
    try {
      token = requireRelayToken(env);
    } catch {
      return errorResponse("server_misconfigured", 500);
    }

    const url = new URL(request.url);
    if (url.pathname === "/") {
      return new Response(indexFallbackHtml(), {
        headers: { "content-type": "text/html; charset=utf-8" }
      });
    }
    if (url.pathname !== "/healthz" && !isAuthorized(request, token)) {
      return errorResponse("unauthorized", 401);
    }
    if (!["/healthz", "/ws", "/contacts", "/send"].includes(url.pathname)) {
      return errorResponse("not_found", 404);
    }

    const id = env.PHONE_RELAY.idFromName("phone");
    const stub = env.PHONE_RELAY.get(id);
    return stub.fetch(request);
  }
};

export class PhoneRelayDurableObject {
  private phoneSocket: WebSocket | null = null;
  private pending = new Map<string, PendingRequest>();
  private rateWindowStart = 0;
  private rateCount = 0;

  constructor(
    private readonly state: DurableObjectState,
    private readonly env: Env
  ) {}

  async fetch(request: Request): Promise<Response> {
    const url = new URL(request.url);
    switch (url.pathname) {
      case "/healthz":
        return jsonResponse({ phone_online: this.isPhoneOnline() });
      case "/ws":
        return this.acceptPhoneWebSocket(request);
      case "/contacts":
        if (request.method !== "GET") {
          return errorResponse("invalid_request", 405);
        }
        return this.forwardContacts();
      case "/send":
        if (request.method !== "POST") {
          return errorResponse("invalid_request", 405);
        }
        return this.forwardSend(request);
      default:
        return errorResponse("not_found", 404);
    }
  }

  private acceptPhoneWebSocket(request: Request): Response {
    if (request.headers.get("Upgrade") !== "websocket") {
      return errorResponse("invalid_request", 400);
    }

    const pair = new WebSocketPair();
    const [client, server] = Object.values(pair) as [WebSocket, WebSocket];
    server.accept();
    this.replacePhoneSocket(server);
    safeLog("phone_ws_connected", {});
    return new Response(null, { status: 101, webSocket: client });
  }

  private async forwardContacts(): Promise<Response> {
    const frame = await this.sendPhoneCommand({ type: "contacts" });
    return responseFromPhoneFrame(frame);
  }

  private async forwardSend(request: Request): Promise<Response> {
    const body = await parseSendRequest(request);
    if ("error" in body) {
      return errorResponse(body.error, 400);
    }
    const toJids = body.to_jids;
    if (toJids.length === 0 || toJids.length > MAX_RECIPIENTS) {
      return errorResponse("too_many_recipients", 400);
    }
    if (toJids.some((jid) => jid.endsWith("@g.us"))) {
      return errorResponse("group_not_allowed", 400);
    }
    if (!this.takeSendRateToken()) {
      return errorResponse("rate_limited", 429);
    }

    const frame = await this.sendPhoneCommand({
      type: "send",
      to_jids: toJids,
      text: body.text,
      client_msg_id: body.client_msg_id
    });
    return responseFromPhoneFrame(frame);
  }

  private replacePhoneSocket(socket: WebSocket): void {
    if (this.phoneSocket) {
      try {
        this.phoneSocket.close(1012, "replaced");
      } catch {
        // Best effort only; a closed socket will be discarded below.
      }
    }
    this.phoneSocket = socket;
    socket.addEventListener("message", (event) => this.handlePhoneMessage(event));
    socket.addEventListener("close", () => this.handlePhoneClose(socket));
    socket.addEventListener("error", () => this.handlePhoneClose(socket));
  }

  private handlePhoneClose(socket: WebSocket): void {
    if (this.phoneSocket === socket) {
      this.phoneSocket = null;
      this.rejectAllPending("phone_offline");
      safeLog("phone_ws_disconnected", {});
    }
  }

  private handlePhoneMessage(event: MessageEvent): void {
    const text = typeof event.data === "string" ? event.data : "";
    let frame: PhoneResponseFrame;
    try {
      frame = JSON.parse(text) as PhoneResponseFrame;
    } catch {
      safeLog("phone_frame_invalid", {});
      return;
    }
    if (frame.type !== "response" || !frame.request_id) {
      safeLog("phone_frame_ignored", {});
      return;
    }
    const pending = this.pending.get(frame.request_id);
    if (!pending) {
      safeLog("phone_response_orphan", { request_id: shortId(frame.request_id) });
      return;
    }
    clearTimeout(pending.timeout);
    this.pending.delete(frame.request_id);
    safeLog("phone_response", {
      request_id: shortId(frame.request_id),
      ok: frame.ok,
      error_code: frame.error_code || ""
    });
    pending.resolve(frame);
  }

  private async sendPhoneCommand(command: Record<string, unknown>): Promise<PhoneResponseFrame> {
    if (!this.isPhoneOnline() || !this.phoneSocket) {
      return { type: "response", request_id: "", ok: false, error_code: "phone_offline" };
    }

    const requestId = crypto.randomUUID();
    const timeoutMs = timeoutFromEnv(this.env);
    const response = new Promise<PhoneResponseFrame>((resolve) => {
      const timeout = setTimeout(() => {
        this.pending.delete(requestId);
        safeLog("phone_request_timeout", { request_id: shortId(requestId) });
        resolve({ type: "response", request_id: requestId, ok: false, error_code: "timeout" });
      }, timeoutMs);
      this.pending.set(requestId, { resolve, timeout });
    });

    try {
      this.phoneSocket.send(JSON.stringify({ ...command, request_id: requestId }));
      safeLog("phone_request_sent", {
        request_id: shortId(requestId),
        type: String(command.type || ""),
        recipient_count: Array.isArray(command.to_jids) ? command.to_jids.length : 0
      });
    } catch {
      const pending = this.pending.get(requestId);
      if (pending) {
        clearTimeout(pending.timeout);
      }
      this.pending.delete(requestId);
      return { type: "response", request_id: requestId, ok: false, error_code: "phone_offline" };
    }
    return response;
  }

  private rejectAllPending(errorCode: ErrorCode): void {
    for (const [requestId, pending] of this.pending) {
      clearTimeout(pending.timeout);
      pending.resolve({ type: "response", request_id: requestId, ok: false, error_code: errorCode });
    }
    this.pending.clear();
  }

  private isPhoneOnline(): boolean {
    return this.phoneSocket !== null;
  }

  private takeSendRateToken(): boolean {
    const now = Date.now();
    if (now - this.rateWindowStart >= 60_000) {
      this.rateWindowStart = now;
      this.rateCount = 0;
    }
    if (this.rateCount >= MAX_SENDS_PER_MINUTE) {
      safeLog("send_rate_limited", {});
      return false;
    }
    this.rateCount++;
    return true;
  }
}

function requireRelayToken(env: Env): string {
  const token = env.RELAY_TOKEN || "";
  if (token.length < MAX_TOKEN_LENGTH_MIN) {
    throw new Error("RELAY_TOKEN must be at least 32 characters");
  }
  return token;
}

function isAuthorized(request: Request, token: string): boolean {
  const header = request.headers.get("Authorization") || "";
  return header === `Bearer ${token}`;
}

async function parseSendRequest(request: Request): Promise<
  | { to_jids: string[]; text: string; client_msg_id: string }
  | { error: ErrorCode }
> {
  let raw: SendRequestBody;
  try {
    raw = (await request.json()) as SendRequestBody;
  } catch {
    return { error: "invalid_request" };
  }
  if (!Array.isArray(raw.to_jids) || typeof raw.text !== "string" || typeof raw.client_msg_id !== "string") {
    return { error: "invalid_request" };
  }
  const toJids = raw.to_jids.map((item) => (typeof item === "string" ? item.trim() : "")).filter(Boolean);
  if (toJids.length !== raw.to_jids.length || raw.text.trim() === "" || raw.client_msg_id.trim() === "") {
    return { error: "invalid_request" };
  }
  return { to_jids: toJids, text: raw.text, client_msg_id: raw.client_msg_id };
}

function responseFromPhoneFrame(frame: PhoneResponseFrame): Response {
  if (frame.ok) {
    return new Response(frame.result_json || "[]", {
      status: 200,
      headers: { "content-type": "application/json; charset=utf-8" }
    });
  }
  const code = (frame.error_code || "phone_error") as ErrorCode;
  const status = code === "phone_offline" ? 503 : code === "timeout" ? 504 : 502;
  return errorResponse(code, status);
}

function errorResponse(error: ErrorCode, status: number): Response {
  return jsonResponse({ error }, status);
}

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "content-type": "application/json; charset=utf-8" }
  });
}

function timeoutFromEnv(env: Env): number {
  const parsed = Number(env.RELAY_TIMEOUT_MS || "");
  return Number.isFinite(parsed) && parsed > 0 ? parsed : DEFAULT_TIMEOUT_MS;
}

function safeLog(event: string, data: Record<string, unknown>): void {
  console.log(JSON.stringify({ ts: new Date().toISOString(), event, ...data }));
}

function shortId(requestId: string): string {
  return requestId.slice(0, 8);
}

function indexFallbackHtml(): string {
  return `<!doctype html>
<html lang="zh-CN">
<head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>WA Relay</title></head>
<body><p>WA Relay Worker is running. Deploy edge/static as the Pages frontend.</p></body>
</html>`;
}
