# Praxis Runtime 设计入口

## 1. 当前状态

- 模块：`runtime`；
- 阶段：仓库Runtime设计资产已通过独立文件复审，恢复“具备正式Plan用户审核条件”；
- 最近更新：2026-07-14；
- 已成立：对话中的修正合同已经通过概念反审；
- 尚未成立：Runtime V1 Plan尚未获得用户批准，12项技术、后端与验收指标决策仍未确认；
- 当前授权：允许用户正式审核Runtime V1 Plan候选；
- 禁止事项：没有实现授权，不得创建Runtime实现目录、选择生产后端、接入账号或运行外部集成。

## 2. 核心定位

Praxis Runtime把不可变`ResolvedAgentPlan`转化为受隔离、可观察、可控制且失败结果真实的执行实例。它拥有实例生命周期、期望状态、协调、命令线性化和证据关联，不拥有模型、Prompt、工具、记忆、审核、预算或Sandbox后端的领域语义。

Runtime治理执行，不干涉AI内部认知。它不得读取隐藏Chain-of-Thought，不得静默改写Prompt、模型输出、Review意见或用户原意。

## 3. 已冻结的核心不变量

1. `AgentIdentity`持久；V1一个Identity只允许一个持有执行权的活跃`InstanceLineage`；
2. 一个Lineage固定绑定一个`ResolvedAgentPlanDigest`，Plan、Profile、Harness、Route或强制Sandbox语义变化创建新Lineage；
3. 重建创建新的`AgentInstanceID`和更高`instance_epoch`，旧Instance永不复活；
4. V1一个Instance独占一个`SandboxLease`，且最多一个活跃Run；
5. 生命周期状态、执行确定性和清理状态是三个正交维度；
6. 所有越出纯本地无副作用计算边界的操作都进入统一Effect合同；
7. 旧执行面的迟到事件过滤不等于Fencing；最终效果边界必须校验Fence、Authority和Capability；
8. 离线撤销只能承诺策略规定的最大延迟，未配置撤销策略时禁用离线效果；
9. 已暴露明文Secret无法证明从进程中消失；长期可重用明文不得交给受治理Harness；
10. 跨系统绑定采用持久Binding Saga和Compensation，不承诺原子Rollback；
11. 不承诺Exactly Once；超时后的真实效果可以是`unknown_outcome`；
12. Harness签名只证明来源，不证明Ready、Cleaned或Effect结论真实；安全状态需要独立Inspect或领域权威事实；
13. 无法访问线性化事实源时，推进、续租、新授权和新Effect fail closed；
14. 可披露数据、产生费用、创建资源或持久状态的Effect必须先持久化权威Intent；
15. Sandbox释放不代表Provider Session、Batch、Hosted Tool、Prompt Cache或远程Sidecar已经结束；
16. Runtime执行结束不等于Effect已结算、Artifact已发布或Task/Goal成功。

## 4. 设计资产

- [总体架构与所有权](./architecture.md)
- [Identity、Lineage、Instance、Run与结果对象](./concepts/README.md)
- [Runtime Kernel、三维状态与协调](./kernel/README.md)
- [Control Plane、命令线性化与CAP边界](./control-plane/README.md)
- [Admission、Activation与Binding Saga](./admission/README.md)
- [Fence、Secret、Sandbox与安全替换](./safety/README.md)
- [统一Effect、Budget、远程状态与正式提交](./effects/README.md)
- [事件、事实可信度与Write-ahead Evidence](./evidence/README.md)
- [Checkpoint、恢复与Cache安全分区](./continuity/README.md)
- [Runtime端口与组件合同](./contracts/README.md)
- [扩展体系与供应链](./extensions/README.md)
- [Application Facade、REPL、API与SDK](./interfaces/README.md)
- [Profile、Model Route与Runtime交接](./profile-assembly/README.md)
- [Sandbox与Placement摘要](./sandbox/README.md)
- [反例验收矩阵](./scenarios/README.md)
- [原始图与中文图解](./grounding/README.md)
- [设计门禁](./acceptance/README.md)

## 5. Runtime拥有与不拥有

| Runtime拥有 | Runtime不拥有 |
|---|---|
| Static Admission结果、Instance/Run执行记录 | Agent Definition与Profile内容 |
| Desired State、Command Log和Reconcile | 模型Route、Prompt与Context语义 |
| Instance、Lineage运行关联与Fence协调 | Tool/MCP业务动作和Provider实现 |
| Binding Saga、迟到结果和清理协调 | Budget价格、额度与结算真相 |
| EffectIntent协调、向绑定Intent Port提交、事件因果关联 | EffectIntent语义事实所有权；每个Effect由Plan绑定的唯一Intent所有者持有 |
| Execution Outcome | Task/Goal Outcome |
| Port绑定和Capability验证 | Sandbox、Harness、Cache和扩展内部实现 |

## 6. 相邻模块交接原则

相邻模块可以尚未实现，但面向Runtime的合同必须明确所有者、输入、输出、不变量、失败、迟到结果和漂移处理。Runtime使用fake/stub验证自身，不复制相邻引擎内部逻辑。

## 7. 当前明确不做

- 不决定语言、目录、进程拓扑、RPC、数据库或生产SLA；
- 不选择Docker、Kubernetes、Firecracker或其他首个真实Sandbox后端；
- 不设计多Identity副本、并发Run、跨Run Session或跨集群迁移；
- 不允许跨租户、跨Authority或跨Harness隐式缓存复用；
- 不实现不可逆效果的自动补偿或Exactly Once；
- 不修改并行推进的`model-invoker`内部实现。

## 8. 复审规则

本轮设计资产已经独立审核线程逐文件检查通过。该结论只恢复Plan用户审核资格，不等于Plan获批，也不自动授权实现。
