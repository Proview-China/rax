# Harness Application V3持久Domain接线完成

时间：2026-07-15 03:36（Asia/Shanghai）

## 已完成

- 新增严格`ModelTurnEffectEnvelopeV2`，把不可变Harness Candidate作为唯一Model Turn Effect payload；
- 新增`ModelTurnOperationBindingFactV3/Port`及线程安全CAS fake，持久关联Application attempt、Run、Session、Candidate、Provider、完整Delegation、Runtime水位与Settlement；
- 新增`ModelTurnDomainAdapterV3`，只接受配置的精确namespaced StepKind和Domain Adapter Binding，通过Application Domain Router接入；
- prepared→observed、prepared→unknown、observed/unknown→settled及pre-prepared unknown failed-only路径闭合；
- 所有不确定回包只Inspect Harness Session和Binding；不重复执行不确定外部动作；
- endpoint/session/host relay/provider/candidate/scope/runtime/application逐项交叉校验；
- Binding按状态从内部事实重算Application Basis，拒绝相同外部Ref/Basis下替换Observation、Unknown Authorization、Settlement或DomainResult；
- post-prepared unknown要求独立Inspect Effect+Settlement provenance；缺失和错换均失败关闭；
- 真实Application Coordinator+OperationDomainRouter+Harness Adapter跨模块黑盒闭环通过，Runtime公共Port使用确定性测试替身。

## 验证

- `go test -count=1 ./...`：通过；
- `go test -count=1 -race -shuffle=on ./...`：通过；
- `go vet ./...`：通过；
- `go test -count=20 ./...`：通过；
- 103个顶层测试函数、3个Fuzz入口；跨包语句覆盖率79.0%。

## 保留边界

- fake Store和集成Runtime Port double不具生产持久性、认证、SLA或dispatch权威；
- 未来用户组件必须注册自己的namespaced Domain Adapter和状态Owner，不能复用Harness Model Turn适配器；
- 生产Provider、Runtime Gateway、State Plane、RPC/Scheduler、Checkpoint/Restore仍由对应Owner后续实现。
