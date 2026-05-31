# GOMOBILE_CONSTRAINTS · Android AAR 边界类型约束

本项目使用 `gomobile bind` 将 Go wrapper 编译成 Android AAR。gomobile 对跨语言边界的类型有严格限制，公开 API 需要保持稳定、简单、可绑定。

## gomobile bind 可跨边界传递的类型

- 基本类型：`int`, `int64`, `float64`, `bool`, `string`
- `[]byte`
- 在当前 package 中定义的 `struct` 指针
- 在当前 package 中定义的 `interface`（用于回调）
- `error`

## 不能直接跨边界的类型

- `map`
- `chan`
- 复杂 slice（除 `[]byte` 外的 slice of struct）
- 函数类型（除非包装成 interface 的方法）
- 第三方包的类型，尤其是内部 JID、event、protobuf 生成类型等。

这些类型不能出现在导出方法签名里，否则会导致 bind 失败或生成的 Android API 难以使用。

## API 设计规则

1. 复杂数据（事件 payload、消息对象、状态详情）一律序列化成 JSON string 跨边界。
2. 导出方法的参数和返回值只使用 gomobile 支持的基础类型。
3. 内部协议类型只在 Go wrapper 内部使用，导出前转换成 string / JSON。
4. 回调使用 Go interface 暴露，Android/Kotlin 侧实现该 interface。
5. Kotlin SDK 再把底层 AAR/API 包装成面向集成方的 `WaBridgeClient`。

## 一句话

Go wrapper 是边界层：边界内可以使用复杂 Go 类型，边界外只暴露基础类型、string 和 JSON。
