# Agent Host Cleanup Closure V2 实施计划候选

## 1. 状态

- 状态：候选，待用户设计审核；所有实施项未授权。
- 独立设计反审：`YES（P0/P1=0）`；实施门仍关闭。
- 设计：[Cleanup Closure V2](../../design/agent-host/cleanup-closure-v2.md)。

## 2. 候选代码范围

```text
ExecutionRuntime/agent-host/
|-- contract/cleanup_closure_v2.go
|-- contract/cleanup_dispatch_v3.go
|-- ports/cleanup_closure_v2.go
|-- storage/sqlite/cleanup_closure_v2.go
|-- lifecycle/host_v2.go
|-- lifecycle/stop_v2.go
`-- tests/...
```

不得修改 Runtime、Application、Harness、组件 Owner Fact；不得静默改变既有 V2 digest。

## 3. 实施波次

### P0 合同

- [ ] 冻结 Ref、Assembly、Binding、Control、Coverage、Fact JSON/canonical/digest；
- [ ] 冻结 deterministic ID、create-once、exact Inspect 与错误 closed set；
- [ ] 冻结 embedded Plan 与同仓 Reader；
- [ ] 冻结 CleanupPlanTemplate current与sealed cleanup route registry；
- [ ] 冻结V3 typed cleanup dispatch envelope与target Readers；
- [ ] 冻结NodeKind+Host phase到constructed_target/attempt_inspect/not_constructed的required/forbidden矩阵；
- [ ] 冻结 Start Journal operation kind 和 Stop expected-only 语义；
- [ ] 冻结 legacy missing closure Fail Closed。

### P1 State Plane

- [ ] SQLite schema/version/foreign keys/row digest/strict decode；
- [ ] Ensure/Inspect/by-start/Plan Reader；
- [ ] lost reply、restart、ABA、typed-nil、deep-copy；
- [ ] 64 个 Store handle 对同一 DB 只线性化一个 Fact。

### P2 Host 接线

- [ ] Binding 后构造完整 Control requests；
- [ ] Closure candidate coverage 验证；
- [ ] Journal intent -> Ensure/Inspect -> result_recorded；
- [ ] 所有后续Host Journal operation Inputs绑定同一ClosureRef，不改Owner request digest；
- [ ] Stop 从 Closure 取唯一 Plan，执行 typed registry；
- [ ] Closure result_recorded后的partial Start各phase均可进入Stop；
- [ ] partial Start按Journal phase只携已存在target；unknown attempt只Inspect，未构造节点只记not-required；
- [ ] historical Closure配合fresh cleanup authorization/fence执行；
- [ ] residual/unknown 只进入 residual 或 indeterminate。

### P3 验收

- [ ] 12 个设计硬反例逐项测试；
- [ ] unit/whitebox/blackbox/fault/conformance；
- [ ] `go test -count=100` targeted；
- [ ] `go test -race -count=20` targeted；
- [ ] full ordinary/race/vet/gofmt/import/diff；
- [ ] Start -> crash -> restart -> Stop exact closure system test。
- [ ] alternate DB、three-write-point crash、expired historical truth、target splice tests。

## 4. 完成门

P0-P3 和独立代码审计全部通过前，`HostV2.StopV2` 仍是 NO-GO；不得用 caller Plan 或 test fixture 宣称安全关闭。
