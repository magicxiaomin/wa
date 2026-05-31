# wa · Android SDK 接口模块

`wa` 是一个 Android SDK 模块，用来把 Go IM 协议客户端能力封装成可被 Android app 集成的 Kotlin API。

当前主线已经完成 SDK 模块化：

- `:wa-sdk`：Android library module，封装 AIDL、`:wa_bridge` 前台服务、`wamobile.aar`，对外暴露 `WaBridgeClient`。
- `:sample-app`：SDK 验证台，只通过 `WaBridgeClient` 调用能力，不直接访问 AIDL、内部 Service 或底层 AAR。
- `bridge/`：Go wrapper，通过 gomobile 编译成 Android AAR，所有复杂入参/返回值使用 JSON string。

## 当前能力

- 扫码登录与 session 恢复。
- 获取联系人与已加入群列表。
- 发送 1:1 文本消息。
- 向多个联系人发送同一文本。
- 向单个群发送文本。
- 接收 1:1 文本消息。
- 查询当前登录账号身份信息。
- 查询用户信息、头像信息、presence，标记已读。
- 导出本机 raw trace 与 session debug 文件。

## 已知限制

- 不支持接收群消息内容；当前 wrapper 会过滤群消息接收事件。
- 不做媒体消息。
- 不把 trace/session/debug bundle 自动上传或外发。
- sample app 是 SDK API 验证台，不是正式聊天产品 UI。

## 快速构建

本仓库固定使用 Go 1.26.3、gomobile、Android Gradle Plugin 与本机 Android SDK/NDK 工具链。工具链路径见 [`ANDROID_BUILD_TOOLCHAIN.md`](./ANDROID_BUILD_TOOLCHAIN.md)。

```bash
./android/build_debug_go126.sh
```

脚本会完成：

1. 用 Go 1.26.3 重编 `wamobile.aar`。
2. 同步 `android/libs/wamobile.aar`。
3. 拆出 `android/wa-sdk/libs/wamobile-classes.jar` 与 `android/wa-sdk/src/main/jniLibs/arm64-v8a/libgojni.so`。
4. 构建 `:wa-sdk:assembleRelease`。
5. 构建 `:sample-app:assembleDebug`。
6. 输出 `android/libs/wa-sdk-release.aar`。

安装验证台：

```bash
adb install -r android/sample-app/build/outputs/apk/debug/sample-app-debug.apk
```

## SDK 集成入口

外部 Android app 集成时只依赖 `wa-sdk-release.aar`，并通过 `WaBridgeClient` 调用公开 API。

核心文件：

- [`SDK_API.md`](./SDK_API.md)：SDK 方法、JSON 形状、线程语义、错误模型、最小集成步骤。
- [`android/wa-sdk/src/main/java/com/magicxiaomin/wa/sdk/WaBridgeClient.kt`](./android/wa-sdk/src/main/java/com/magicxiaomin/wa/sdk/WaBridgeClient.kt)：SDK Kotlin 客户端。
- [`android/wa-sdk/src/main/AndroidManifest.xml`](./android/wa-sdk/src/main/AndroidManifest.xml)：SDK Manifest，声明 `:wa_bridge` 前台服务和权限。

最小调用流程：

```kotlin
val client = WaBridgeClient(context, "WA-Android")
client.setEventListener { eventType, payloadJson ->
    // 回调已由 SDK 切回 Main Looper
}
client.bind()
client.connectBridge()
val identity = client.getSelfIdentity()
val contacts = client.getContacts()
```

SDK 方法是同步 AIDL 调用。集成方应把耗时调用放到后台线程；事件回调由 SDK 保证回到主线程。

## 文档入口

- [`SDK_API.md`](./SDK_API.md)：集成方 API 手册。
- [`SDK_DESIGN.md`](./SDK_DESIGN.md)：SDK 模块结构、公开接口边界和交付物。
- [`SDK_REGRESSION_CHECKLIST.md`](./SDK_REGRESSION_CHECKLIST.md)：SDK 回归基线。
- [`TRACE_SCHEMA.md`](./TRACE_SCHEMA.md)：trace/debug 字段定义。
- [`GOMOBILE_CONSTRAINTS.md`](./GOMOBILE_CONSTRAINTS.md)：gomobile 边界。
- [`ANDROID_PITFALLS.md`](./ANDROID_PITFALLS.md)：Android 真机坑清单。
- [`KNOWN_PITFALLS.md`](./KNOWN_PITFALLS.md)：Go wrapper / 协议客户端坑清单。

## 调试文件注意事项

trace/debug 可能包含完整 JID、消息正文、QR/pairing code、session/auth 调试材料。这些文件只应保存在 App 私有目录，不应提交到 git、issue、review 附件或外部服务。
