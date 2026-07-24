# Agent Host H4 Production Lifecycle V2 实施计划候选

## 1. 状态

- 状态：设计与本计划已获用户确认；P0公共合同开始实施，其余项仍未开始。
- 前置设计：[H4 Production Lifecycle V2](../../design/agent-host/h4-production-lifecycle-v2.md)。
- 目标：不改变 HostV1 的前提下，新增唯一 HostV2 production root，把 Runtime/Application/Harness/6+1 的真实治理链接成可恢复整体。

## 2. 预期产物

```text
ExecutionRuntime/agent-host/
|-- contract/v2_*.go
|-- ports/v2_*.go
|-- owneradapter/{assembly_current,binding,activation,readiness}_v2.go
|-- composition/root_v2.go
|-- lifecycle/host_v2.go
|-- cmd/praxis-agent/
`-- tests/system/
```

Owner 公共 Delta 分别落在 Runtime、Application、Harness 与各组件目录，由对应 Owner 独占实现；Host 只实现组合和中立 adapter。

## 3. 波次

### P0 合同

- [ ] 冻结跨HostV1/V2共用的HostStart Admission/Claim与唯一production facade；
- [ ] 冻结 HostV2 contract/version/conflict domain；
- [x] 冻结 Harness Assembly Artifact atomic publish + Historical/Current Reader；
- [ ] 冻结 Runtime Binding Admission Governance Port；
- [ ] 冻结ResourceBindingSet并纳入Binding request/fact/result/TTL；
- [ ] 冻结 Application Production Start Coordination Port/Fact；
- [ ] 冻结中立 Component Production Current 与 SystemReady Fact Port；
- [ ] 冻结Generation Association/Application result/Sandbox active/Execution ready完整readiness闭包；
- [ ] 冻结SystemReady immutable Fact + renewable Current/supervision失效语义；
- [ ] 冻结Runtime中立AgentExecutionAvailability ref/current reader与epoch fence actual-point门；
- [ ] 冻结typed ControlAdapterFactoryV2/Conformance与Cleanup dependency class；
- [ ] 冻结CleanupPlan/Node/Barrier/Attempt typed DAG和逐节点恢复；
- [ ] 区分pre-binding DeploymentReadiness与post-activation ComponentProductionCurrent；
- [ ] 冻结Organization Release与Human Multi-Sign Review Release variant的显式required dependency；
- [ ] 完成 V1/V2 no-conversion/import DAG/typed-nil/canonical测试。

### P1 Owner 实现

- [x] Harness immutable artifact atomic publisher/store/historical/current readers；
- [ ] Runtime Binding admission start-or-inspect；
- [ ] Application activation/stop write-ahead coordinator；
- [ ] 七组件 production current adapters；
- [ ] Runtime/Harness/Application production current adapters；
- [ ] executable factory adapters 和 cleanup inspectors。

### P2 HostV2

- [x] V2 journal 与状态机；
- [ ] outcome_unknown/reconciliation_required只Inspect收敛状态机；
- [ ] shared HostStart Claim永久唯一索引/续期/压缩tombstone；
- [x] Definition/Assembler/Harness H3 adapter复用；
- [ ] Binding -> construct control -> Activation -> Generation Association；
- [ ] S1/actual-point/S2 readiness；
- [ ] SystemReady Current续检、过期/revoke退出Ready与新Run阻断；
- [ ] stop/reconcile/residual聚合；
- [ ] CLI只调用同一 HostV2 API。

所有Owner调用均须先在HostV2 Journal写完整step key、`AttemptID + RequestDigest + exact inputs + predecessor`，返回后再CAS exact result；unknown/lost reply只Inspect原attempt。Stop按typed Cleanup DAG执行，Sandbox release不得越过仍需lease的cleanup。Production CLI/API只调用shared Admission后的HostV2 facade；V1 governed wrapper仅用于兼容冲突测试并固定reference-only。

### P3 系统验收

- [ ] 单元/白盒/黑盒/故障/Conformance；
- [ ] ordinary100/race20/full ordinary/race/vet；
- [ ] YAML AgentDefinition -> sealed plan -> full 6+1 production Start；
- [ ] model -> governed tool/MCP -> context refresh -> memory/knowledge -> checkpoint/restore；
- [ ] crash/restart/TTL/clock/64并发/cleanup residual；
- [ ] V1先/V2后、V2先/V1后与64并发共享HostStart Claim；
- [ ] artifact publish lost reply、SystemReady全current S1/S2 splice与zero-effect factory conformance；
- [ ] staged artifact不可见、Ready依赖过期后退出、Claim过期不复用、cleanup barrier DAG故障矩阵；
- [ ] production backend smoke，无 fake/internal/testkit/raw provider import。

## 4. 完成门

只有 P0-P3 全部通过并且七个 required release 都由对应 Owner 发布 current production proof，才可把 Agent Host 标记为 `SYSTEM_READY`。单独 Release candidate、test-only fixture、ModuleFactoryDescriptor 或一次普通测试通过均不构成 production GO。
