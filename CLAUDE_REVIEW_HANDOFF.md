# Claude review handoff

## Review goal

Audit the Android second-wave PoC for this repository:

1. `gomobile` AAR integration of the whatsmeow wrapper.
2. Android real-device flow for QR/session restore, contacts, one-to-one text send, and one-to-one text receive.
3. Safety and privacy controls: send throttling, risk stop, foreground bridge process, panic handling, trace redaction.

The current implementation is a PoC. Do not judge UI polish as a blocker unless it affects functional correctness, safety, or testability.

## Current status

The main Android chain has passed real-device testing:

```text
session restore / QR login capability
-> GetContacts
-> select Robert 2 / 85255804693@s.whatsapp.net
-> send 1:1 text
-> remote side received it
-> remote side replied
-> Android UI received message_received
```

Test device: `N0WR2G0009`

Active test account suffix: `...4392` for phone `18205924392`

See `ACCEPTANCE_ANDROID.md` for the detailed real-device evidence.

## Build requirements

Use the pinned Go 1.26.3 Android build path. Do not use Go 1.25.x for AAR builds.

```sh
./android/build_debug_go126.sh
```

Then install while preserving app data/session:

```sh
$HOME/Library/Android/sdk/platform-tools/adb install -r android/app/build/outputs/apk/debug/app-debug.apk
```

Details are in `ANDROID_BUILD_TOOLCHAIN.md`.

Reason: a Go 1.25.0-built AAR crashed in the Android `:wa_bridge` process during send:

```text
fatal error: bulkBarrierPreWrite: unaligned arguments
```

Rebuilding the AAR with Go 1.26.3 removed that crash in subsequent real-device send/receive testing.

## Test commands

Go tests:

```sh
GOCACHE=$HOME/.cache/codex-wa-tools/go-cache \
GOPATH=$HOME/.local/share/codex-wa-tools/go-path \
GOPROXY=https://goproxy.cn,direct \
GOSUMDB=off \
$HOME/.local/share/codex-wa-tools/go1.26.3/bin/go test ./...
```

Android build:

```sh
./android/build_debug_go126.sh
```

Useful real-device checks:

```sh
$HOME/Library/Android/sdk/platform-tools/adb shell ps -A | rg 'com\.magicxiaomin\.wa'
$HOME/Library/Android/sdk/platform-tools/adb shell run-as com.magicxiaomin.wa cat files/wa-session/risk-stop.json 2>/dev/null || true
$HOME/Library/Android/sdk/platform-tools/adb shell run-as com.magicxiaomin.wa cat files/wa-trace.json
```

## Files to review first

Android:

- `android/app/src/main/AndroidManifest.xml`
- `android/app/src/main/java/com/magicxiaomin/wa/MainActivity.kt`
- `android/app/src/main/java/com/magicxiaomin/wa/WaApp.kt`
- `android/app/src/main/java/com/magicxiaomin/wa/bridge/BridgeForegroundService.kt`
- `android/app/src/main/aidl/com/magicxiaomin/wa/bridge/IBridgeService.aidl`
- `android/app/src/main/aidl/com/magicxiaomin/wa/bridge/IBridgeCallback.aidl`

Go wrapper:

- `bridge/client.go`
- `bridge/whatsmeow_adapter.go`
- `bridge/trace.go`
- `bridge/client_test.go`
- `bridge/trace_test.go`

Docs:

- `SPEC_ANDROID.md`
- `API_CONTRACT_ANDROID.md`
- `ANDROID_PITFALLS.md`
- `GOMOBILE_CONSTRAINTS.md`
- `TRACE_SCHEMA.md`
- `ACCEPTANCE_ANDROID.md`
- `ANDROID_BUILD_TOOLCHAIN.md`

## High-priority review questions

1. Does `BridgeForegroundService.emit()` safely cross the whatsmeow goroutine -> Go callback -> Android service -> AIDL callback boundary, or can it block binder/service threads under load?
2. Does `MainActivity.updateSafetyControls()` create too many ad hoc `Thread`s under frequent events? Should it use a single executor/coroutines?
3. Is the UI-side JID-only contact de-duplication enough, or should the Go contact resolver merge LID/JID aliases before returning contacts?
4. Is `server_msg_id` acceptable in trace under the privacy policy, or should it be hashed/truncated?
5. Are panic recover boundaries complete for exported Go methods and event goroutines? Note: Go runtime fatal errors cannot be recovered; Go 1.26.3 is the mitigation.
6. Is `ClearSession` sufficiently guarded? It currently requires long press but has no confirmation dialog.
7. Is `RemoteCallbackList` error handling adequate? Failed callback deliveries are currently swallowed.
8. Are the fresh-link send/contact cooldowns and risk-stop behavior adequate for avoiding immediate new-device programmatic behavior?
9. Does foreground service setup satisfy Android 13+ notification and Android 14/15 foreground service requirements?
10. Should `syncFullHistory=false` / history flood throttling be further validated or surfaced in logs?

## Known remaining work

These are not blockers for the current PoC pass:

- `ClearSession -> QR login again` regression is intentionally deferred to preserve the active session.
- Long-duration background/lock-screen testing remains: 10/30/60 minute runs, including receive while backgrounded/locked.
- UI is intentionally utilitarian; product-level messaging UI/history/error UX is future work.
- A strict single exported trace containing every action in one file can be re-run, though individual trace redaction for send/receive has already been verified.

## Safety notes

- Scope is one account, one-to-one text only.
- No group send, media, multi-account, or broadcast behavior.
- Sending keeps wrapper-side rate/count/risk controls.
- Trace must not contain session keys, full phone numbers, or message bodies.
- UI realtime callbacks may contain message body by design.
