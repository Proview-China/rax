# Sandbox Data Plane IPC v1

该目录是 Go Control Plane 与 Rust Data Plane 的唯一 wire 合同入口。传输为 Unix domain
socket 上的 `uint32` big-endian 长度前缀加 strict JSON；单帧上限 4 MiB，未知字段、尾随数据、
typed-nil/presence 冲突和非授权 peer UID 均 fail closed。

合同版本固定为 `praxis.sandbox/data-plane-ipc/v1`。`phase` 只有 `prepare`、`execute`；
`effect_kind` 独立表达 Sandbox lifecycle 动作，禁止把动作冒充 Runtime Enforcement phase。
所有时间使用 Unix nanoseconds，`now == expires_unix_nano` 已过期。

摘要算法：

```text
sha256(contract_version || 0x00 || kind || 0x00 || canonical_json)
```

canonical JSON 使用 DTO 声明字段顺序；map key 按 UTF-8 词典序；摘要字段自身清空后参与对象
canonical。Go `encoding/json` 与 Rust `serde_json` 的当前实现由 `golden/provider-binding-v1.json`
双边测试锁定。

`dispatch-request-v1.schema.json` 只描述 wire shape，不授予执行资格。Rust 在 Provider
Prepare/Execute 前仍必须经 reverse current-reader 独立复读 Runtime V4 与 Sandbox Owner current。
Provider 返回值只能是 Observation/Receipt，不能成为 Runtime 或 Sandbox 权威 Fact。
