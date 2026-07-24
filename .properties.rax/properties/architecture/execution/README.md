# 核心执行设计域

## 1. 目标

把已解析的Agent装配计划转化为受隔离、可观测、可控制、可恢复的运行实例，并为未来多Agent调度和Sandbox集群管理提供稳定基础。

## 2. 当前模块

| 模块 | 职责 | 当前阶段 |
|---|---|---|
| [`runtime`](../../../design/runtime/README.md) | 实例接纳、状态机、协调、挂载、事件和控制 | 公共合同、Activation容灾、组件装配门禁与最小完整闭环已通过普通/Race/Vet验证 |
| [`sandbox`](../../../design/sandbox/README.md) | 隔离环境、资源租约、网络/文件/进程边界和集群供给 | 设计骨架 |

Scheduler暂作为Runtime Control Plane的内部设计部分研究；只有职责和规模证明需要独立模块时，才单独立项。

## 3. 执行主链

```text
ResolvedAgentPlan
  -> Static Admission
  -> proposed Lineage / proposed Instance
  -> Bounded Preflight（关联proposed Instance）
  -> ActivationSnapshot（只绑定proposed身份与Sandbox需求）
  -> reserve IdentityExecutionLease（排他、只允许激活/清理）
  -> reserve Budget cap（适用时）
  -> reserve_quarantined SandboxLease
  -> ActivationCommit（Identity/Lineage active；Instance provisioning）
  -> activate SandboxLease
  -> Binding Saga
  -> Independently Verify Ready
  -> Run / Unified Effect / Observe / Control
  -> Checkpoint / Fence / Reconcile RemoteContinuation
  -> Stop / Cleanup
  -> 有本地清理证据后Release SandboxLease
  -> RemoteContinuation单独结算/保留Quarantine
```

这条链不是一次性脚本。Runtime通过期望状态、可验证状态机和持续协调，让实际状态收敛到期望状态。

## 4. 共同边界

- V1一个AgentInstance独占一个SandboxLease；
- V1一个AgentIdentity只有一个活跃IdentityExecutionLease，一个Instance只有一个活跃Run；
- Runtime不写死Docker、Kubernetes或某一种Sandbox后端；
- Runtime不生成Prompt、不选择审核结论、不拥有记忆算法；
- Sandbox销毁不得删除AgentIdentity、正式Memory或已确认Asset；
- 所有推进命令、EffectIntent和高风险状态变更必须先持久化权威证据；证据不可写时推进操作fail closed；
- Harness自报只构成带来源的观察，Ready、Fenced、Cleaned和Effect结算必须依赖对应权威边界的独立证据；
- Sandbox释放不等于Provider Session、Batch、Background、Hosted Tool或Prompt Cache已清理；残留进入RemoteContinuation与UnknownOutcome跟踪；
- SandboxLease只在本地进程、网络、挂载、设备和Secret路径有充分清理证据后释放；它不必等待无冲突的远程状态结束。仍可能影响冲突域的远程残留继续占用该域并阻止相应ReplacementPermit；
- Sandbox、Harness和其他组件通过Provider/Adapter合同替换，公共接口不绑定官方实现；
- REPL、API和SDK只进入跨领域应用门面；门面不拥有Runtime内部事实，也不是新的万能Runtime模块；
- Runtime消费Context CachePlan和Model Profile结果，但不拥有缓存或模型语义。

## 5. 已冻结的概念合同

- AgentInstance重建使用新`instance_id`与更高`instance_epoch`；同一Lineage只绑定一个ResolvedPlanDigest；
- 生命周期必须同时表达`LifecyclePhase`、`ExecutionCertainty`与`CleanupStatus`，不能把已发Stop写成已终止；
- 所有外部效果统一进入Intent-before-dispatch、最终效果边界校验、Receipt/UnknownOutcome和必要预算结算；
- 分区时读取可降级为stale projection，但推进、授权、续租和新Effect无法访问唯一线性化事实源时一律fail closed；
- Runtime只权威决定ExecutionOutcome，不代替Review、Artifact、Task或Goal的结果所有者。

## 6. 后续仍待审核的实现选项

- 单实例Runtime Kernel与多实例Control Plane的进程边界；
- 语言扩展边界、协议、数据库和部署拓扑；
- 首个Sandbox/Harness/Provider后端及其Conformance目标；
- Checkpoint各Provider可贡献的状态范围与恢复支持等级；
- 调度器是否在V1出现，以及V1仅保留哪些最小Placement字段。
