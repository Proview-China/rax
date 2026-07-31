# Tool Owner Durable SQLite V2 计划

状态：实现与软件测试完成；production NO-GO。

## 完成项

- [x] `FactStoreV2` context-aware Port 与旧 `StoreV2` 兼容 adapter。
- [x] 五类 Tool fact、action head、claim、execution history/head SQLite schema。
- [x] deterministic claim/marker create-once、exact replay、CAS/ABA 与 lost-reply Inspect。
- [x] claim → marker → state dispatch；marker attempt 强类型下传。
- [x] Tool-owned durable entry lease；64 个独立 flow/多 SQLite store 只允许一个 StartOrInspect 或 Inspect，过期 holder 以新 revision 接管同一 exact attempt。
- [x] owner-safe durable handoff：`handoff_inspect` 强制 exact Inspect，`handoff_start_or_inspect` 只允许 Start caller 在同一 `start_committed` attempt 上恢复。
- [x] handoff/execution commit-unknown 只复读预封 next immutable history；同 phase 后续 revision 的 ABA replay 必须 Conflict。
- [x] actual-entry fresh gate：紧贴外部 seam 复读 input、lease、execution current、TTL 与 phase；取消、漂移、clock regression、expiry 均保持外部调用零次。
- [x] 外部入口后的 clock/marker/state 失败先持久化 Inspect handoff，避免活跃 lease stranded 或错误重启。
- [x] live caller 等待服从 caller context 与 owner clock TTL；transport 取消后的 `WithoutCancel` 恢复严格有界且只 Inspect exact attempt。
- [x] Runtime settlement/association S1/S2 fresh reader 门；InMemory 在 S2 后使用无等待 Tool mutation gate，reader drift/锁竞争均零写且不持锁回调。
- [x] Reservation/Apply 使用 Tool Owner clock，并在 S1、IMMEDIATE 写锁后 S2、commit 前 S3 验证 TTL/current；锁等待跨 expiry 时零事实写入。
- [x] malformed Runtime inspection 在 InMemory 与 SQLite backend 间保持相同 InvalidArgument/InvalidCanonicalForm 与零写语义。
- [x] ApplySettlement + ToolResult + head 单事务。
- [x] action head 读取时逐项复核五类 immutable predecessor closure；post-commit caller cancellation 两种 backend 均返回 Indeterminate 且 exact 事实可恢复。
- [x] strict JSON、row/head/ledger/schema physical proof、corruption/splice fail closed；ledger 存在时 migration 永不执行静默 repair。
- [x] base V1 schema 同样使用 ledger-first no-repair 与完整 physical proof；缺表、额外列/index/trigger、collation 和伪造 ledger 均 fail closed。
- [x] durable constructors、8 次 restart 与返回方法集均排除 `single_call_application_result_v2`；该 Application close seam 不属于本切片。
- [x] 每阶段 restart、64 并发、fault injection、ordinary/race/vet 验证。

## 产物

- `ExecutionRuntime/tool-mcp/action`：durable fact Port。
- `ExecutionRuntime/tool-mcp/applicationadapter`：typed claim/marker store 与 durable owner flow。
- `ExecutionRuntime/tool-mcp/storage/sqlite`：Tool-owned SQLite 实现、schema proof 和专项测试。

## 未授权

production composition root、跨 Owner 接线、HA/SLA、Provider/Credential/网络 backend 与 `CoordinationStoreV1` durability 均保持 NO-GO。
