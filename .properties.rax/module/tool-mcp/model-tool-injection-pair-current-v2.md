# Model Tool Injection Pair Current V2

## 作用

本模块把 Tool Owner 保存的 exact Model Tool Injection material、compiled tools 和 current Tool Surface，转换成 Model `InvocationMaterialToolPairExactReaderV2` 可消费的 owner-neutral pair projection。它只证明输入工具材料的来源、当前性与真实请求字节一致，不授权 Provider 调用或 Tool execution。

## 组成

1. `contract/model_tool_injection_pair_current_v2.go`
   - 保留 Material/Surface 两个 exact source。
   - 分开保存 stored 与 actual compiled digest，并要求两者严格相等。
   - 保存由 Model 官方 helper 从真实 request tools 计算出的 `RequestToolsDigest`。
2. `surface/model_tool_injection_pair_current_v2.go`
   - 从同一 closure reader 成对读取 compiled/material。
   - 读取 exact current Surface，执行 S1/S2 全量复读与最小 TTL 收口。
   - 使用真实 `RouteCall.Request.Tools`，不接收 caller digest，不处理 ToolChoice。
3. `modelinvokeradapter/invocation_material_tool_pair_v2.go`
   - request-scoped 冻结真实 tools bytes。
   - 实现 Model paired-reader 公开接口。
   - caller digest 只能与实际 bytes 自算结果比较，不能替代 authoritative Tool projection。

## 边界

- Owner、Kind、ID、revision、digest 从 Tool authoritative projection 无损映射；adapter 不复制 Tool Kind literal。
- Material source、Surface source、Expected injection、Compiled Tools、Request Tools 与 pair Projection digest 是不可替代的独立语义角色；只有 stored/actual Compiled Tools 作为同一角色的两次观察必须相等。
- Model `CompiledToolsDigest` 已能承载 Tool 的 actual compiled digest，因此没有修改 Model 合同。
- 不导入 Model internal/provider、Harness、Sandbox、Runtime adapter 或 SQLite。
- ToolChoice 仍归 Model；静态 import conformance 与实际注入的 connected spy 共同证明 adapter 没有 Provider 或 Tool execution 面。
- production Provider 链仍为 NO-GO，本模块只是 owner-local/cross-owner read adapter。

## 验证

- `go test -count=100 ./modelinvokeradapter`
- `go test -race -count=20 ./modelinvokeradapter`
- `go test ./...`
- `go test -race ./...`
- `go vet ./...`

以上命令均于 2026-07-31 本地回审前通过。
