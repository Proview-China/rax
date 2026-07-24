# Continuity首版Participant、Rewind与部署裁决

- 时间：2026-07-18 17:18 CST
- 性质：管理线业务裁决；后续设计与实现的唯一范围输入

## 已确认

1. 首个Checkpoint Profile的Required Participant固定为Runtime、Harness、Context、Sandbox、Memory-Knowledge五类。Review与Tool只冻结各自Owner已形成的exact refs；Model Invoker不是Checkpoint Participant。
2. 首版Rewind只覆盖受治理的Workspace文件ChangeSet。Continuity只拥有exact Plan/current/history/CAS；Sandbox拥有Workspace View、ChangeSet、实际文件Effect与ApplySettlement。Application/Runtime复用既有治理链，不能由Continuity新造执行通道。
3. 邮件、交易、网络请求、远程数据库、Tool调用和remote blob等外部Effect不会被Rewind回滚；只继承为历史事实、Settlement、Residual或后续补偿候选。
4. 本机默认Backend为live SQLite+RocksDB实现，公共SPI继续Backend-neutral；只报告真实benchmark，不承诺SLA，不预选远程Provider、进程拓扑、KMS或企业容量目标。

## 实施门禁

- Rewind Plan Request只携stable identity、Scope和exact refs；不得携accepted Review、Permit/Fence、Provider binding、可信current或文件payload。
- Plan approval/submission不授执行资格。实际Workspace变化必须形成new ChangeSet/new Operation并经过Admission、Review/Authorization、Permit/Fence、Begin、actual-point Enforcement、Observation/Evidence、Runtime Settlement(ref only)与Sandbox ApplySettlement。
- Begin后丢回包只Inspect原Attempt；不得换Plan、ChangeSet或Operation identity重派。
- Partial Checkpoint只诊断；Rewind/Restore不宣称外部世界回滚；legacy Port不能包装升权。

## 当前结论

上述裁决解除Continuity owner-local Rewind Plan V2的设计歧义，可进入TDD。跨Ownerproduction root、Tool/remote补偿、远程存储/KMS和SLA仍不在本切面。
