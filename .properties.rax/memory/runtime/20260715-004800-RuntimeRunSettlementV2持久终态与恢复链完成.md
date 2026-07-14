# Runtime Run Settlement V2持久终态与恢复链完成

## 事件

2026-07-15，Runtime P0.5完成Run Settlement V2公共合同、持久事实、分段Run Effect索引、Closure Attempt恢复、Decision+Run原子提交、Termination Progress/Report及Conformance首切面的代码与测试收口。

## 已落地事实

- Run与Settlement Plan在任何Run内dispatch前原子创建；Plan固定8个Runtime治理barrier，并允许多个自定义组件以同Kind、不同Requirement ID共同参与；
- Run Effect write-ahead与索引登记同Owner原子，分段链通过10,000引用、跨tenant同Run/Effect ID隔离与Freeze竞态验证；
- P0.2 Gateway可通过分区Adapter治理原子登记的Run Effect，Permit exact replay幂等、换内容Conflict；
- Claim Association与Ledger Record在P0.5重新精确核验，Claim不选择Outcome；
- Execution Inspection逐字段绑定stable Runtime session、endpoint、scope/epoch、run/binding、source sequence、payload和Evidence；
- Participant必须引用精确Evidence，correlation和causation绑定Participant ID/revision；
- unknown、failed、not_applied三类Disposition使用独立Policy模式，execution unknown/lost有独立结果矩阵；
- Closure使用append-only attempt链与current pointer，TTL续租/重启可创建新attempt，旧attempt Decision拒绝；
- Decision与terminal Run由同一Owner原子提交；终态重放只Inspect exact Decision/Progress/Closure；
- Fact Owner从full Closure重建并校验Execution/Claim/Resolution/Outcome provenance，且从terminal Run+Decision+Progress重建Report，raw Port伪造逐字段拒绝；
- cleanup/residual/provider retention通过独立Progress推进；永久unknown可持久交付且重复reconcile不假增长，Termination Report仅完全闭合后create-once且不改Outcome；
- 自定义Participant/Backend Conformance不会自授Binding、Dispatch、Outcome、production durability或SLA。

## 保留限制

- Foundation的持久Run准备与Runtime Settlement已可用，但现有直接Close/Fence/Release仍是restricted compatibility，不是可恢复外部Effect路径；
- P0.6仍需用Operation Effect、Application Step Journal和Host/Data Provider双Binding bridge闭合start/cancel/close/release；
- Runtime+Harness+6+1执行基座尚不能解锁，必须等P0.6与最终组合门禁完成；
- 未选择生产数据库、RPC、Scheduler、签名、进程拓扑或SLA。
