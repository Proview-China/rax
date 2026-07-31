# Tool Owner Durable SQLite V2 模块说明

该子模块为 Tool/MCP Owner 保存 single-call claim、execution marker 和 Action V2 authoritative facts。

入口分为三层：

- `action.FactStoreV2`：context-aware 事实读写合同；
- `applicationadapter`：claim/marker/entry lease typed contract 与 restart-safe 状态分派；
- `storage/sqlite`：STRICT schema、immutable history、narrow head、CAS、WAL 与故障恢复。

安全语义：

- durable marker 在任何可能执行之前提交；
- durable entry lease 跨 flow/跨 store 串行化外部入口；lease 过期接管仍绑定原 exact downstream attempt；
- holder 可在退出前提交 owner-safe handoff：`handoff_inspect` 只能 exact Inspect，`handoff_start_or_inspect` 只能由 Start caller 在同一 `start_committed` attempt 上消费；
- 紧贴 StartOrInspect/Inspect 外部入口前 fresh 复读 input、lease、execution current、TTL 与 phase；取消、漂移、clock regression 或 expiry 时外部调用为零；
- 外部入口后的 clock/marker/state 失败必须先 durable handoff 到 exact Inspect，不能留下仍允许 Start 的 lease；
- handoff/execution advance 丢回包只复读预封 next immutable history；同 phase 的后续 revision 不得伪装成原 successor，防止 ABA；
- live caller 不受任意 owner-local wall-clock 恢复窗截断；transport 已取消时才进入有界 `WithoutCancel` exact-Inspect；
- unknown 后只 Inspect；只有同一 exact attempt 的纯 Inspect NotFound 且 marker 仍为 `start_committed` 时，才允许 create-once StartOrInspect 恢复，绝不生成新 attempt；
- Runtime settlement 与 association 必须完成 S1/S2 exact 复读后才允许 Tool Apply；InMemory 的 S2 后 Tool mutation gate 不等待，也不持锁调用外部 Reader；
- Reservation 与 Apply 使用不可回退的 Tool Owner clock，并在 S1、SQLite IMMEDIATE 写锁后 S2 与 commit 前 S3 验证 authoritative TTL/current，锁等待跨 expiry 时零写；
- malformed Runtime inspection 在 InMemory/SQLite 两种 FactStore 后端都 fail closed 且零写；
- Apply、Result、head 原子提交，reply 丢失后按 exact ref 恢复；head 读取逐项验证完整 immutable predecessor closure；
- schema、JSON、digest、index、FK、trigger 或 history 发生漂移时 fail closed。
- 有 ledger 的数据库只允许纯验证，禁止 `CREATE IF NOT EXISTS` 静默修复缺失 Owner 对象。
- base V1 与本切片 V2 都遵守同一 no-repair 规则。
- `single_call_application_result_v2` 是 Application close seam；本模块的 constructors、namespace 与接口均明确排除它。

本模块只证明 owner-local 单机软件实现，不是 production readiness 证明。
