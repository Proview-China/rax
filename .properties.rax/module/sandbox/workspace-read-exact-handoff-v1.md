# Workspace Read Exact Handoff V1 Module

该模块包含两个 Sandbox-only 接口：

- kernel 到 Data Plane 的 additive `WorkspaceReadCurrentQueryV2` exact handoff；
- `InspectWorkspaceReadAttemptForAdmissionV1` 历史 exact Reader。

V2 由 Base V1、Reservation、original Attempt 与 Sandbox AdmissionReceipt 组成，Rust 必须在实际读取前通过 UDS 回读；V1-only execute 不可进入物理读取。它只绑定既有 Owner facts，不授予执行权限。Admission lookup 返回 original AttemptRef；执行结果仍由 `InspectBoundedWorkspaceReadV1` 读取。

## Owner-local 验收

- 公共链路已覆盖：`Executor → Go Adapter → Rust Data Plane → Go CurrentServer → V2 exact Reader → openat2/pread`。
- workspace-read Execute 的 V2 要求由 Go 构造器、Go CurrentServer 和 Rust 合同三处独立 Fail Closed。
- S1/S2 会复读 Base V1、Reservation、Attempt；SandboxProjection 的 revision、digest、expiry 三轴必须完全一致，Projection 到期时间必须严格等于各 exact fact 的自然最小 TTL。
- query 构造失败发生在 actual point 前，并形成确定性 failed/no-effect，不转为 Unknown。
- 64 个同 Attempt 并发请求最多形成一次物理读取；Reservation/Attempt 漂移的物理读取计数为零。

最终门禁：

- `go test ./...`
- `go vet ./...`
- `go test -race ./...`
- `cargo test --all-targets`
- `cargo clippy --all-targets -- -D warnings`
- `cargo fmt --check`
- 公共 UDS 链路定向黑盒
- `git diff --check`

以上门禁于 2026-07-31 全绿。本结论仅代表 Sandbox owner-local exact handoff 闭合，不代表 Tool DomainResult、Runtime Settlement、ToolResult、Host composition 或 Console 页面已经接线。
