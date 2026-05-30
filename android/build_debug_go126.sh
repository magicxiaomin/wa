#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TOOLS_DIR="$HOME/.local/share/codex-wa-tools"

export PATH="$TOOLS_DIR/go1.26.3/bin:$TOOLS_DIR/go-tools:$TOOLS_DIR/go-path/bin:$HOME/Library/Android/sdk/platform-tools:$PATH"
export JAVA_HOME="$TOOLS_DIR/jdk17/Contents/Home"
export ANDROID_HOME="$HOME/Library/Android/sdk"
export ANDROID_SDK_ROOT="$HOME/Library/Android/sdk"
export ANDROID_NDK_HOME="$HOME/Library/Android/sdk/ndk/27.2.12479018"
export GOCACHE="$HOME/.cache/codex-wa-tools/go-cache"
export GOPATH="$TOOLS_DIR/go-path"
export GOPROXY="https://goproxy.cn,direct"
export GOSUMDB="off"

GO_VERSION="$("$TOOLS_DIR/go1.26.3/bin/go" version)"
case "$GO_VERSION" in
  *"go1.26.3"*) ;;
  *)
    echo "error: expected Go 1.26.3, got: $GO_VERSION" >&2
    exit 1
    ;;
esac

"$TOOLS_DIR/go-tools/gomobile" bind -target=android/arm64 -androidapi 24 -o "$ROOT_DIR/android/wa-sdk/libs/wamobile.aar" "$ROOT_DIR/bridge"
cp "$ROOT_DIR/android/wa-sdk/libs/wamobile.aar" "$ROOT_DIR/android/libs/wamobile.aar"

TMP_AAR_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_AAR_DIR"' EXIT
unzip -q "$ROOT_DIR/android/wa-sdk/libs/wamobile.aar" classes.jar jni/arm64-v8a/libgojni.so -d "$TMP_AAR_DIR"
cp "$TMP_AAR_DIR/classes.jar" "$ROOT_DIR/android/wa-sdk/libs/wamobile-classes.jar"
mkdir -p "$ROOT_DIR/android/wa-sdk/src/main/jniLibs/arm64-v8a"
cp "$TMP_AAR_DIR/jni/arm64-v8a/libgojni.so" "$ROOT_DIR/android/wa-sdk/src/main/jniLibs/arm64-v8a/libgojni.so"

(
  cd "$ROOT_DIR/android"
  "$TOOLS_DIR/gradle-8.10.2/bin/gradle" --no-daemon :wa-sdk:assembleRelease :sample-app:assembleDebug
)

cp "$ROOT_DIR/android/wa-sdk/build/outputs/aar/wa-sdk-release.aar" "$ROOT_DIR/android/libs/wa-sdk-release.aar"
