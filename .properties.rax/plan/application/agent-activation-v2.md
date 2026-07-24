# Application Agent Activation V2 实施计划候选

## 1. 状态

- 状态：候选，待用户设计审核；所有实现项未授权。
- 独立设计反审：`YES（P0/P1=0）`；实施门仍关闭。
- 设计：[Agent Activation V2](../../design/application/agent-activation-v2.md)。

## 2. 候选产物

```text
ExecutionRuntime/application/
|-- contract/agent_activation_v2.go
|-- ports/agent_activation_v2.go
|-- storage/sqlite/agent_activation_v2.go
|-- agent_activation_coordinator_v2.go
|-- conformance/agent_activation_v2.go
`-- tests/...
```

Runtime、Sandbox、Harness 的窄Reader、stable Gateway与adapter由各Owner单独审核和实现；Application不复制其事实类型，也不在`application/runtimeadapter`中反向包装Owner实现。Composition Root只向Application Coordinator注入public StepPort。

## 3. 波次

### P0 合同与联合审核

- [ ] 冻结 V2 envelope、pre-allocation ActivationSubject、result/ref/canonical/closed role union；
- [ ] 冻结Application V1/V2 VersionClaim与初始Fact同一原子线性点；
- [ ] 冻结 V1/V2 version claim 冲突域；
- [ ] Runtime中立nominal refs、窄Reader与Commit Gateway Delta联合审核；
- [ ] Sandbox两个stable Gateway与三个Reader Delta联合审核；
- [ ] Harness两个stable Gateway与三个Reader Delta联合审核；
- [ ] 冻结八步 Intent/Fence 与 unknown inspect-only 时序。

### P1 Application State Plane

- [ ] Coordination immutable history/current/step event SQLite；
- [ ] create-once/CAS/lost-reply/clock/TTL；
- [ ] 每步 stable AttemptID 和 predecessor exact；
- [ ] intent_recorded -> invocation_recorded正常CAS独占Start权；
- [ ] 64 并发只线性化一个 start token。

### P2 Owner adapters

- [ ] Preflight；
- [ ] Snapshot；
- [ ] Identity/Budget；
- [ ] Sandbox Allocate；
- [ ] Activation Commit；
- [ ] Sandbox Activate；
- [ ] Execution Open；
- [ ] Ready Inspect。

每个adapter落在对应Owner包，只依赖Application public request和本Owner public Port，不导入其他Owner实现包。

### P3 Host 与系统验收

- [ ] production Host Service V3 Binding -> Closure -> Control -> Activation V2；
- [ ] HostV2 reference conformance保持兼容，但不计production完成；
- [ ] crash/restart/lost reply/unknown/TTL/clock rollback；
- [ ] Commit 前后副作用顺序；
- [ ] 64 Host/Coordinator；
- [ ] ordinary100/race20/full ordinary/race/vet/gofmt/import/diff。

## 4. 完成门

P0-P3、Owner联合审计与production current后端全部通过前，不得用V1 fake或generic Observation声明Agent Activation可用于生产。
