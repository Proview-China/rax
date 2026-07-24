# Harness Model PreDispatch Commit Gate V1设计冻结

时间：2026-07-16 20:56（Asia/Shanghai）

## 事件

用户接受Model Preparation、Harness唯一Commit Gate、Tool InvocationSurfaceBinding三Owner边界，并授权Harness先形成设计、计划和测试资产。本次已冻结Harness Owner设计，尚未实现Go。

## 冻结决定

- Model Owner发布immutable PreparedInvocation Ref/Reader，并在任何Provider/Backend首调前调用公共Gate；
- Harness/host实现唯一Gate与Assembly接线，只做Owner exact复读、逐字段比较和Tool writer调用；
- Tool Owner单独持有Surface Current与InvocationSurfaceBinding create-once Repository；
- ACK前Provider调用数必须为0；lost writer reply只Inspect同Invocation；
- Assembly ToolSurface与ExpectedInjectionManifest只从verified、sealed `AssemblyManifestV1.Plan`读取，并绑定Generation/Handoff/BindingSet current；
- live `model.dispatch.before` HookFace只作编译声明，不是runtime Gate；
- ModelTurnPort/Loop V1字段保持不变，通过受治理ModelTurn实现的构造注入与Assembly Conformance新增能力；
- Application P2不增加Surface字段或Owner依赖。

## 当前状态

- Harness设计：已冻结，等待Model/Tool/Harness联合设计Review；
- Harness Go：NO-GO；
- Tool P4/system G6A/Capability/production root：继续NO-GO；
- 未选择生产Backend、RPC、数据库、拓扑或SLA。

## 资产

- [设计](../../design/harness/assembly/model-predispatch-surface-commit-gate-v1.md)
- [测试矩阵](../../design/harness/assembly/model-predispatch-surface-commit-gate-v1-test-matrix.md)
- [实施计划](../../plan/harness/model-predispatch-surface-commit-gate-v1.md)
