# INSTALL · AAR 直接集成

本文说明如何不登录 GitHub Packages，直接下载 GitHub Release 里的 `wa-sdk-release.aar` 并集成到 Android app。

## 1. 下载 AAR

从 GitHub Releases 下载：

```text
https://github.com/magicxiaomin/wa/releases
```

需要的文件：

```text
wa-sdk-release.aar
```

该 AAR 已包含 SDK Kotlin 客户端、AIDL/Service 封装、底层 Go bridge classes 和 `arm64-v8a` native library。

## 2. 放入集成方工程

推荐放到 app 模块：

```text
app/libs/wa-sdk-release.aar
```

然后在集成方 `app/build.gradle` 中加入：

```gradle
dependencies {
    implementation files("libs/wa-sdk-release.aar")
}
```

如果集成方工程不是 Kotlin 工程，可能还需要加入 Kotlin stdlib：

```gradle
dependencies {
    implementation "org.jetbrains.kotlin:kotlin-stdlib:2.0.21"
}
```

## 3. Android 配置

SDK AAR 自带 Manifest，会合并以下内容：

- `:wa_bridge` 前台服务
- dataSync foreground service type
- AIDL Service 相关声明
- 基础权限声明

Android 13+ 仍需要集成方在运行时申请通知权限：

```kotlin
if (Build.VERSION.SDK_INT >= 33) {
    requestPermissions(arrayOf(Manifest.permission.POST_NOTIFICATIONS), 100)
}
```

当前发布包包含 `arm64-v8a` native library。测试设备需要支持 `arm64-v8a`。

## 4. 最小调用

```kotlin
val client = WaBridgeClient(context, deviceName = "MyDevice")

client.setEventListener { eventType, payloadJson ->
    // 回调已在 Main Looper，可以直接更新 UI。
}

client.bind()
client.connectBridge()

val state = client.getState()
val identity = client.getSelfIdentity()
val contacts = client.getContacts()
```

退出页面或不再使用时：

```kotlin
client.unbind()
```

完整 API 见 [`SDK_API.md`](./SDK_API.md)。

## 5. 版本建议

Release tag 使用语义化版本，例如：

```text
v0.1.0
```

集成方应记录所使用的 Release tag，便于问题复现和回滚。
