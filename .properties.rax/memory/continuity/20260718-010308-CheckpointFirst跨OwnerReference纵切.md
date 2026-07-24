# Checkpoint-first跨Owner Reference纵切

时间：2026-07-18 01:03 CST

## 结论

用户授权的最小Runtime Delta与Checkpoint-first跨Owner reference纵切已闭合：Harness Gate经Application协调Runtime原子Attempt/Barrier与EffectCut，两个测试Participant提交exact closure，Continuity聚合Manifest并创建immutable Seal，Runtime双读后提交唯一Consistency，最后以terminal Attempt释放Gate。

该结论只覆盖reference组合与公开合同；全链`ProviderCalls=0`，不解锁生产Checkpoint root、真实Snapshot capture/Participant Provider、Restore、production Backend或SLA。

## 实现事实

- Harness Gate绑定EffectCut所引用的前驱Attempt，并要求当前collection Attempt为紧邻revision；重放只能返回可证明的单调successor，禁止rev1复活与ABA。
- Application `CheckpointCoordinatorV1`只通过公开Port编排，不写Runtime或Participant事实；Runtime/Participant/Manifest/Seal/Consistency mutation丢回包均Inspect原identity。
- Harness与Sandbox Participant Adapter都校验exact Attempt+Participant，Observation不能冒充Owner Fact。
- Continuity Manifest create/CAS/Seal lost reply均以exact history/current Inspect恢复；changed content为Conflict。
- Runtime Participant closure与current projection精确绑定collection Attempt；terminal Attempt是更高revision，terminal Inspect由immutable Consistency回指历史collection Attempt，拒绝跨Attempt splice。
- partial第二Participant不形成Seal或Consistency，Gate保持bound/fenced；Partial只作诊断。

## 验证证据

- Harness Gate ordinary `count=100`：PASS。
- Runtime跨Attempt splice定向ordinary `count=100`：PASS。
- Checkpoint integration ordinary `count=20`：PASS。
- Harness Gate与integration定向race `count=20`：PASS。
- Runtime、Application、Harness、Sandbox、Continuity分别执行`go test ./...`：全部PASS。
- 上述五模块分别执行`go test -race ./...`：全部PASS。
- 上述五模块分别执行`go vet ./...`：全部PASS。

组合测试覆盖happy、Runtime Attempt create lost reply、Participant commit lost reply、Manifest create/CAS/Seal lost reply、Runtime Consistency lost reply、partial、progressed replay/no-ABA和跨Attempt换包。

## 仍然unsupported

- 生产Participant phase bridge、真实Snapshot Provider/capture、production composition root；
- Restore Intent/Eligibility/Attempt/Stage/Activate及任何外部世界回滚；
- 真实remote blob/purge/archive、production Backend选择与SLA；
- 用测试fixture、legacy Port或ControlCapabilities声明冒充生产能力。

## 入口

- [Continuity实现入口](../../../ExecutionRuntime/continuity/README.md)
- [Continuity设计入口](../../design/continuity/README.md)
- [跨模块映射](../../design/continuity/integration-mapping.md)
- [验收合同](../../design/continuity/acceptance.md)
- [实施计划](../../plan/continuity/README.md)
- [模块入口](../../module/continuity/README.md)
