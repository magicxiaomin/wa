# wa · Android SDK 接口研究模块

`wa` 是一个本地 Android SDK 研究项目，用来验证 Go IM 协议客户端在 Android 真机上的可集成性、接口健壮性和运行边界。项目当前定位是：单手机、单账号、本地运行、不上云、不做队列或调度、不做多账号框架。

当前主线已经完成 SDK 模块化：

- `:wa-sdk`：Android library module，封装 AIDL、`:wa_bridge` 前台服务、`wamobile.aar`，对外暴露 `WaBridgeClient`。
- `:sample-app`：SDK 验证台，只通过 `WaBridgeClient` 调用能力，不直接访问 AIDL、内部 Service 或底层 AAR。
- `bridge/`：Go wrapper，通过 gomobile 编译成 Android AAR，所有复杂入参/返回值使用 JSON string。

## 当前能力

- 扫码登录与 session 恢复。
- 获取联系人与已加入群列表。
- 发送 1:1 文本消息。
- 向多个联系人发送同一文本，用于本地接口研究。
- 向单个群发送文本。
- 接收 1:1 文本消息。
- 查询当前登录账号身份信息。
- 查询用户信息、头像信息、presence，标记已读。
- 导出本机 raw trace 与 session debug 文件。

## 已知限制

- 不支持接收群消息内容；当前 wrapper 会过滤群消息接收事件。
- 不做媒体消息。
- 不做多账号、多手机、多租户。
- 不做云端、远程触发、Web 控制台、队列、调度、对象存储。
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
- [`SPEC_WAVE5.md`](./SPEC_WAVE5.md)：当前 SDK 模块化设计与不变量。
- [`ACCEPTANCE_WAVE5.md`](./ACCEPTANCE_WAVE5.md)：当前回归基线。
- [`SPEC_WAVE4.md`](./SPEC_WAVE4.md)：本地研究红线与 raw trace/debug 边界。
- [`TRACE_SCHEMA.md`](./TRACE_SCHEMA.md)：raw trace 字段定义。
- [`GOMOBILE_CONSTRAINTS.md`](./GOMOBILE_CONSTRAINTS.md)：gomobile 边界。
- [`ANDROID_PITFALLS.md`](./ANDROID_PITFALLS.md)：Android 真机坑清单。
- [`KNOWN_PITFALLS.md`](./KNOWN_PITFALLS.md)：Go wrapper / 协议客户端坑清单。

## 研究边界

本项目只用于本机研究和接口验证。raw trace/debug 可能包含完整 JID、消息正文、QR/pairing code、session/auth 调试材料。这些文件只应保存在本机私有目录，不应提交到 git、issue、review 附件或外部服务。
