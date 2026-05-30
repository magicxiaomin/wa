# Claude Review Handoff · Wave 4 MVP Research Mode

## Review Target

Branch: `feat/wave4-cloudflare-relay`

Primary requirements:

- `SPEC_WAVE4.md`
- `ACCEPTANCE_WAVE4.md`
- `TRACE_SCHEMA.md`

These three files are authoritative for the current Wave 4 review.

Older documents such as `SPEC_WAVE3.md`, `ACCEPTANCE_WAVE3.md`, `SPEC_ANDROID.md`,
`ACCEPTANCE_ANDROID.md`, and old PR descriptions may still mention:

- max 3 recipients
- trace redaction
- no message body / no full phone / no session key in trace

Those are historical constraints from earlier waves. They are superseded by Wave 4 MVP research mode.

## Current Product Direction

The project is now a local Android whatsmeow interface research tool:

- single phone
- local Android UI
- local `:wa_bridge` ForegroundService
- gomobile `wamobile.aar`
- Go wrapper around whatsmeow
- no Cloudflare
- no VPS
- no browser console
- no remote trigger
- no queue
- no scheduler
- no cloud storage

The goal is to verify whatsmeow Android interfaces one by one from the UI.

## Intentional Requirement Changes

The following are intentional and should not be flagged as regressions:

1. `SendTextMulti` no longer has a hard 3-recipient limit.
2. Android UI no longer blocks selecting more than 3 contacts.
3. `SendTextMulti` result JSON may include full `jid`.
4. Error JSON may include raw error text.
5. trace/debug may include:
   - full phone numbers
   - full JIDs
   - message text
   - QR code
   - pairing code
   - session/auth/device store debug material
6. `UserIDString()` / self JID is treated as the current WhatsApp account ID in the whatsmeow session.
7. `self JID + GetState() == connected` is treated as local proof that this device currently holds a valid linked-device session for that account.

## What Should Still Be Flagged

Please flag any of the following:

- Any Cloudflare / VPS / remote relay / remote trigger code path.
- Any queue, task scheduler, cron, durable storage, or delayed send mechanism.
- Any automatic upload or network export of trace/session/debug bundles.
- Any multi-account or multi-phone orchestration.
- Any hidden background mass-send flow not initiated locally from the Android UI.
- Any gomobile-exported API that leaks Go internal complex types directly instead of JSON/string/int/bool/[]byte-compatible values.
- Any Android callback path that updates UI off the main Looper.
- Any exported Go method or event goroutine without panic recovery.

## Identity Review Notes

`UserIDString()` is expected to identify the current WhatsApp account inside whatsmeow.

Device/session-specific credentials are different:

- self JID identifies the account
- session/auth credentials identify and authorize this linked device session
- connected state proves the local session is currently usable

For external verification, a future nonce-message flow is preferred:

1. generate nonce
2. send nonce to a known test contact from the current account
3. verify receipt/reply with the same nonce

Nonce verification is not required for this Wave 4 document update unless explicitly implemented later.

## Expected Review Summary

A good review should answer:

- Does the code match the new Wave 4 MVP research requirements?
- Are all cloud/remote relay pieces removed?
- Is raw trace/debug local-only?
- Are old max-3 and redaction assumptions fully removed from current docs/code paths?
- Is the self identity model documented and technically consistent with whatsmeow?
