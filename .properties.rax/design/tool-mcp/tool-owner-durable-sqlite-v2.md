# Tool Owner Durable SQLite V2 设计

状态：`owner_local_implementation_candidate`

## 边界

本切片只拥有 Tool/MCP Owner 的单机 SQLite 状态：

- `tool_action_candidate_v2`
- `tool_action_reservation_v2`
- `tool_domain_result_v2`
- `tool_apply_settlement_v2`
- `tool_result_v2`
- `tool_action_head_v2`
- single-call claim、execution immutable history 与 narrow head
- execution entry lease immutable history 与 narrow head

它不拥有 Runtime Settlement、Application workflow、Sandbox、Context、Harness、Host、Provider、Credential、production composition root，也不把 `CoordinationStoreV1` 声明为 durable。

旧 `single_call_application_result_v2` 属于 Application close seam，不在本切片的 constructor、SQLite namespace、返回接口或 durable closure 中；`OpenOwnerClaimExecutionStoreV2` 与 `OpenActionFactStoreV2` 不得创建或依赖该表。

## 冻结不变量

1. claim 的时间来自 immutable Application request；同 key replay 必须与完整 canonical body/digest 相同。
2. execution marker 只有 `start_committed | inspect_only | settled`；Unknown 只有 `entry_outcome_unknown | inspection_indeterminate`。
3. marker rev1 必须先 durable commit，随后才把 marker 派生的 `ExecutionAttemptID` 传入下游 create-once seam。
4. `inspect_only` 永不 Start；纯 Inspect 的 NotFound 只有在 marker 仍为 `start_committed`、且绑定同一 exact create-once attempt 时，才证明该 attempt 尚未建立并允许 durable `handoff_start_or_inspect`；它绝不授权生成新 attempt。
5. `start_committed` 与 `inspect_only` 进入外部 seam 前必须先取得 Tool-owned durable entry lease；lease 精确绑定 request/input/attempt/owner-incarnation，只协调入口，不拥有外部 effect 状态。
6. 同一有效 lease 只有一个 holder；holder 可以在退出前提交 owner-safe handoff，也可以在 lease 到期后由新 revision 接管，二者都继续使用原 exact downstream attempt。
7. `handoff_inspect` 强制任何后继只 Inspect；`handoff_start_or_inspect` 只有 Start caller 且 marker 仍为 `start_committed` 时可消费，纯 Inspect 不得借此启动 effect。
8. 在 StartOrInspect 或 Inspect 的不可逆外部入口前，holder 必须 fresh 复读 input、lease、exact execution state/current、TTL 和合法 phase；取消、漂移、clock regression 或 `now == expires` 均保持外部调用零次。
9. 外部入口后的 clock/marker/state 错误必须先持久化 exact Inspect handoff，再返回错误，禁止留下仍允许 Start 的活跃 lease。
10. handoff 或 execution advance 的 commit outcome 未知时，只能复读预封 next 的 immutable history exact ref/revision/digest；current 已推进到后继 revision 也不能替代该证据。
11. handoff replay 只接受预封的唯一 successor exact body；相同 phase 的后续 revision 不构成幂等成功，防止 ABA。
12. live caller 等待 durable winner 时继承 caller context，并由注入 owner clock 判定 exact TTL；只有 transport 已取消时才使用短且有界的 `WithoutCancel` exact-Inspect 恢复窗。
13. Reservation 与 Apply 使用不可回退的 Tool Owner clock，而 caller `now` 只进入 immutable fact 时间；S1、取得 IMMEDIATE 写锁后的 S2 与 commit 前 S3 都 fresh 复读 TTL/current，锁等待跨 expiry 时事务回滚且事实零写。
14. Apply 前必须对 Runtime current settlement 与 exact Evidence association 做完整 S1/S2 复读；SQLite 在写锁内复读，InMemory 在 S2 后只允许无等待 `TryLock`，任何漂移或锁竞争都保持 Apply/Result 零写，且绝不持 Tool lock 调用外部 Reader。
15. ApplySettlement、ToolResult 与 settled head 在同一 SQLite transaction 中提交。
16. schema proof 使用 `table_xinfo`、`index_list/index_xinfo`、`foreign_key_list`、trigger 精确枚举、完整 DDL token 等价与非法 stage/lease phase 行为证明；弱 schema fail closed。
17. V1/V2 migration 都只有在“无 ledger 且零 Owner 对象”时创建；ledger 已存在时只验证，缺表、缺 index 或弱结构绝不静默修复。
18. action head 的每次读取都复核 candidate → reservation → domain result → apply → result 的完整 immutable predecessor closure；row/body/head/history/ledger 都执行 strict JSON、canonical digest、exact ref、revision/CAS 与 restart 复核。

## 明确未完成

- production backend/root、HA/SLA、跨 Owner readiness 与系统能力启用；
- Runtime/Application/Context/Harness 持久化；
- `CoordinationStoreV1` durability；
- 多节点共识、在线迁移和生产运维证明。
