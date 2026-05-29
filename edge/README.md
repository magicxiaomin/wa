# Wave 4 Cloudflare Relay

This edge relay is intentionally small: one Worker routes to one Durable Object named `phone`. The Durable Object keeps only in-memory state: the current phone WebSocket, in-flight request resolvers, and a per-minute send counter. It does not use KV, D1, Queues, R2, Cron, or Durable Object storage APIs.

## Configure

Install dependencies:

```sh
cd edge
npm install
```

Set a strong token as a Worker secret. It must be at least 32 characters.

```sh
npx wrangler secret put RELAY_TOKEN
```

Do not put `RELAY_TOKEN` in `wrangler.toml`.

## Deploy Worker

```sh
cd edge
npm run deploy
```

The phone connects to:

```text
wss://<worker-domain>/ws
```

with:

```text
Authorization: Bearer <RELAY_TOKEN>
```

## Deploy Pages frontend

Deploy `edge/static/` as a Cloudflare Pages site, or copy it to your preferred static hosting under the same Worker origin. The page calls:

- `GET /contacts`
- `POST /send`

Both use `Authorization: Bearer <token>`.

## Local mock phone

Run Worker dev, then start the mock phone client:

```sh
cd edge
npm run dev

RELAY_WS_URL=wss://<worker-domain>/ws \
RELAY_TOKEN=<same-token> \
node scripts/mock-phone.mjs
```

The mock logs only event type, short request id, and recipient count. It never logs token, full recipient JIDs, or message text.

## Tests

```sh
cd edge
npm test
```

The test suite covers unauthorized access, redundant `/send` validation, offline phone errors, timeout cleanup, rate limiting, and a mock phone WebSocket success path.
