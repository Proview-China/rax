# Management Engine 设计骨架

## 设计状态

- 当前阶段：设计草案。
- 当前授权：仅允许共同讨论和完善设计，不存在实现授权。
- 本文件不是冻结合同，也不代表已经创建 Management Engine 模块。

## 作用

Management Engine 是 Praxis 的管理与控制意图层。
它比较目标状态和已知现实状态，组织管理流程，并提出可审核的控制意图。
它回答“希望系统接下来发生什么”，但不直接执行任何现实动作。

## 核心输入

- `DesiredState`：AgentIdentity、AgentInstance、AgentRun或集群的目标状态。
- `ObservedStateRef`：Runtime 返回的当前状态、健康度和版本引用。
- `ManagementPolicyRef`：生命周期、恢复、预算和运维规则。
- `ReviewDecisionRef[]`：Review Engine 已产生的判断结果。
- `OrganizationSnapshotRef`：请求方身份、管理关系、职权与问责绑定。
- `OperatorRequest`：经过身份认证的人工管理请求。

## 核心输出

- `ControlIntent`：希望 Runtime 验证并执行的类型化控制意图。
- `DesiredStateRevision`：目标状态的版本化修订候选。
- `ReconciliationPlan`：从观察状态走向目标状态的有序意图集合。
- `IntentTrace`：意图来源、依据、关联判断、请求方和幂等身份。

## 本引擎拥有

- 目标状态和管理策略的解释。
- 生命周期管理意图的生成、排序和去重。
- 观察状态与目标状态之间的漂移分析。
- 控制意图的来源、理由、优先级和幂等身份。

## 本引擎不拥有

- 不拥有 Runtime 的真实状态机和执行权。
- 不直接启动、暂停、恢复、取消或终止任何对象。
- 不替代 Review Engine 作规则符合性和证据充分性判断。
- 不自行扩大请求方的 Authority。
- 不把发出的 ControlIntent 视为已经成功执行。

## 与 Review 和 Runtime 的关系

```text
Review Engine  ->  对事实、计划或动作作出判断
Management     ->  综合目标和判断，提出 ControlIntent
Runtime        ->  按当前事实重新验证，并执行、拒绝或延后
```

Runtime 必须验证 ControlIntent 的权限、前置条件、目标版本和幂等状态。
Runtime 返回的 `ControlResult` 才能改变 Management 已知的观察状态。
状态在意图产生后发生漂移时，Runtime 必须拒绝或要求重新协调。

## 依赖

- Runtime 暴露的状态快照、控制意图和结果合同。
- Review Engine 的判断引用合同。
- Organization Engine 的身份、管理关系、职权和问责合同。
- 统一事件、时间、版本、幂等和因果关联语义。

## 待共同决定

- v1 支持的 ControlIntent 种类与粒度。
- 自动协调与必须人工确认之间的边界。
- 多个管理者、策略或意图冲突时的仲裁顺序。
- 失败补偿、重试上限和 `indeterminate` 状态的管理方式。

## 进入 Plan 阶段的门槛

- 冻结 DesiredState、ControlIntent 和 ControlResult 的 v1 候选语义。
- 完成 Review、Management、Runtime 三方责任与时序图。
- 明确 Runtime 当前事实始终高于 Management 缓存状态。
- 准备正常、冲突、状态漂移、重复提交和部分失败验收用例。
- 获得用户对范围、产物和实施顺序的明确审核与授权。
