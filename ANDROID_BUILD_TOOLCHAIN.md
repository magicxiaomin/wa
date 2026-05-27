# Android build toolchain notes

## Required local toolchain

Use the pinned local Go toolchain for Android AAR builds:

- Go: `$HOME/.local/share/codex-wa-tools/go1.26.3`
- gomobile/gobind: `$HOME/.local/share/codex-wa-tools/go-tools`
- Gradle: `$HOME/.local/share/codex-wa-tools/gradle-8.10.2`
- JDK: `$HOME/.local/share/codex-wa-tools/jdk17`
- Android SDK: `$HOME/Library/Android/sdk`
- Android NDK: `$HOME/Library/Android/sdk/ndk/27.2.12479018`

Do not build the Android AAR with Go 1.25.x. On the tested Android arm64 device,
the Go 1.25.0-built gomobile AAR crashed in `:wa_bridge` during send with:

```text
fatal error: bulkBarrierPreWrite: unaligned arguments
```

Rebuilding `wamobile.aar` with Go 1.26.3 removed that runtime crash in the
subsequent real-device send and receive tests.

## One-command debug build

From the repository root:

```sh
./android/build_debug_go126.sh
```

The script:

1. Verifies that Go 1.26.3 is being used.
2. Rebuilds `android/app/libs/wamobile.aar` with gomobile for `android/arm64`.
3. Copies the AAR to `android/libs/wamobile.aar`.
4. Runs `:app:assembleDebug`.

Install while preserving app data/session:

```sh
$HOME/Library/Android/sdk/platform-tools/adb install -r android/app/build/outputs/apk/debug/app-debug.apk
```

## SQLite driver

The wrapper uses `modernc.org/sqlite` through `database/sql`, avoiding cgo and
the Android NDK C compiler path for sqlite.
