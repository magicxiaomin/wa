# GOMOBILE_CONSTRAINTS · 类型约束（PoC 阶段就遵守）

本阶段是桌面 PoC，**不会真的用 gomobile**。但后续 Android 阶段会用 `gomobile bind` 把同一个 wrapper 编成 AAR。gomobile 对跨语言边界的类型有严格限制。**如果 PoC 阶段就遵守这些约束，后续 Android 化几乎零返工；否则要重设计接口。**

## gomobile bind 能跨边界传递的类型（白名单）
- 基本类型：`int`, `int64`, `float64`, `bool`, `string`
- `[]byte`
- 在你自己的 package 里定义的 `struct` 指针
- 在你自己的 package 里定义的 `interface`（用于回调）
- 错误：`error`

## **不能**直接跨边界的类型（会导致 bind 失败或不可用）
- `map`（任何 map）
- `chan`（channel）
- 复杂 slice（除 `[]byte` 外的 slice of struct）
- 函数类型（除非包装成 interface 的方法）
- **第三方包的类型**——尤其是 whatsmeow 的 `types.JID`、`events.*`、protobuf 生成的 `waProto.*` 等。这些**绝对不能**出现在导出方法的签名里。

## 因此的设计铁律
1. **所有复杂数据（事件 payload、消息对象、状态详情）一律序列化成 JSON 字符串跨边界**。这就是 API_CONTRACT 里回调用 `payloadJSON string` 的原因。
2. 导出方法的参数和返回值，只用上面白名单里的类型。
3. whatsmeow 的内部类型（JID、events、proto）只在 wrapper **内部**使用，绝不暴露到导出接口。在 wrapper 内部把它们转成 string / JSON。
4. 回调用 interface 实现：定义一个 Go interface（如 `EventCallback`，含一个方法），Android 侧 Kotlin 实现它。PoC 阶段桌面可以用普通函数，但**接口形态要预留**，注释说明 Android 化时改成 interface。

## 一句话
**把 wrapper 想象成一道墙：墙内是 whatsmeow 的复杂 Go 世界，墙外（导出接口）只有 string / 基本类型 / JSON。** 这道墙现在就建好，Android 化时直接搬。
