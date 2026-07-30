# Workspace Read Exact Handoff V1

## 目标

该 additive 切片只由 Sandbox Owner 持有，闭合两处接缝：

1. `WorkspaceReadPhysicalExecutorV1` 从自身读取的 exact Owner current 构造 additive `WorkspaceReadCurrentQueryV2`，并把它交给 Rust Data Plane 的 execute 请求；
2. Sandbox 在创建 original WorkspaceRead Attempt 时，同事务保存 Runtime AdmissionReceipt 到 original Attempt 的不可变 exact binding，供后续 Tool Owner 通过完整 receipt 检索。

不修改 Runtime、Tool、Harness、Application、Host 或 Console，不新增页面合同。

## Actual-point current

caller 只能传入 sealed Physical Authorization。Sandbox kernel 自行读取 Association、WorkspaceReadCommand、WorkspaceView 与 Runtime current，并据此封存 Query。StableKey、AuthorizationDigest、Association、DomainCommand、Command、WorkspaceView、Scope、Path、Checked 与 Expires 均来自已复读事实，不接受 caller 选择 mutable current。

V2 保留 V1 的 Runtime/Association/Command/Workspace closure，并额外绑定 Sandbox Reservation、original Attempt 与 Sandbox AdmissionReceipt。S1/S2 都复读 Base V1、Reservation 和 Attempt；Attempt 必须仍为 `started`，Reservation 必须指向同一 Attempt，且 stable/request/payload/admission/TTL 必须逐项一致。

Go actual-point adapter 只把 V2 Query 放入 execute Dispatch；prepare 继续使用其 prepare phase exact Runtime current。Rust 在 `openat2/pread` 前通过 UDS 回读 Go V2 Reader。V1-only execute 会在 Owner/Provider 调用前拒绝；Reservation 或 Attempt 漂移返回可信 `effect_not_started / physical_read_count=0`，不会被误记为已跨 actual point。

## Admission 到 original Attempt

`WorkspaceReadAdmissionAttemptBindingV1` 至少封存：

- 完整 exact Runtime AdmissionReceipt；
- original `WorkspaceReadAttemptRefV1`；
- WorkspaceReadCommand exact ref；
- AuthorizationDigest 与 StableKeyDigest；
- Prepared Association exact ref；
- DomainCommand exact ref；
- Created、Expires 与 Digest。

StableKey 与 AuthorizationDigest 只是被封存的历史证明，不是 lookup key。唯一公开读取入口为：

`InspectWorkspaceReadAttemptForAdmissionV1(ctx, exactAdmissionReceipt)`

SQLite 以完整 AdmissionReceipt identity 建 exact 索引，并在保存 Reservation、Attempt origin/current 的同一事务写入 binding。Attempt current 后续推进、Unknown 或过期不改变 original AttemptRef；历史读取只返回 original ref，不恢复执行、不宣告 current。

V2 current 另使用两个互相独立的 Sandbox Reader：

- `InspectWorkspaceReadReservationExactV1`
- `InspectWorkspaceReadAttemptCurrentV1`

它们不扩展 broad OwnerStore；前者读取 exact immutable Reservation，后者以 original AttemptRef 定位 latest current，并保留 original/current 分离。

## Unknown 与 Owner 边界

lost reply 只能使用完整 AdmissionReceipt 读取 original AttemptRef，再调用现有 exact Attempt Inspect。Tool 不得通过 stable key、ID 命名规则或 digest 推导 Attempt。Sandbox 不生成 Tool DomainResult、Settlement 或 ToolResult。
