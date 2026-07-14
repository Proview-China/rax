# Review Engine 设计骨架

## 设计状态

- 当前阶段：设计草案。
- 当前授权：仅允许共同讨论和完善设计，不存在实现授权。
- 本文件不是冻结合同，也不代表已经创建 Review Engine 模块。

## 作用

Review Engine 是 Praxis 的判断引擎。
它依据明确的规则、证据和当前快照，对计划、动作、效果或状态作出可追溯判断。
它回答“是否满足条件、是否允许继续、证据是否充分”，但不负责采取控制动作。

## 核心输入

- `ReviewRequest`：待判断对象、判断目的、请求方和关联身份。
- `SubjectSnapshot`：计划、实例、AgentRun、动作或效果的不可变快照。
- `ReviewPolicyRef`：版本化判断规则和适用范围。
- `EvidenceRef[]`：事件、Effect、测试、审批、签名或外部证明。
- `OrganizationSnapshotRef`：相关 Identity、Authority 与 Accountability 绑定。

## 核心输出

- `ReviewDecision`：通过、拒绝、条件通过、证据不足或需要人工判断。
- `DecisionReason[]`：逐条规则的命中情况与理由。
- `RequiredCondition[]`：继续前必须满足的条件，不等同于执行命令。
- `ReviewTrace`：策略版本、输入摘要、判断过程和决策身份。

## 本引擎拥有

- 判断规则的解析与确定性求值。
- 证据充分性和规则符合性的判断。
- 对过期、冲突或缺失证据的明确报告。

## 本引擎不拥有

- 不拥有Agent、Harness、AgentRun或Sandbox的生命周期。
- 不提出启动、暂停、取消、终止等控制意图。
- 不执行工具、MCP、网络、文件或进程动作。
- 不读取、推断或干预 AI 的隐藏推理过程。
- 不绕过 Management Engine 或 Runtime 直接产生现实副作用。

## 与 Runtime 和 Management 的关系

```text
Review Engine  ->  产生 ReviewDecision
Management     ->  根据目标、状态和判断提出 ControlIntent
Runtime        ->  验证意图、权限、时序与当前状态后执行或拒绝
```

Runtime 是现实执行状态的最终验证者，不得把 ReviewDecision 当作已经执行。
Management Engine 可以引用判断结果，但不得把判断结果伪装成 Runtime 回执。
Review Engine 只消费 Runtime 的快照和证据，不直接进入 Runtime 内部修改状态。

## 依赖

- 版本化 Review Policy 与规则解析合同。
- RuntimeEvent、外部效果、AgentRun与状态快照合同。
- Organization Engine 提供的身份、职权和问责快照。
- 统一 Evidence、时间、摘要和签名表达。

## 待共同决定

- v1 的判断种类、终态枚举和条件通过语义。
- 同步判断、异步判断和人工判断之间的边界。
- 决策有效期、状态漂移后的自动失效规则。
- 多条规则冲突时的优先级、合并和拒绝策略。
- Review Policy 的作者、发布者和版本撤销权限。

## 进入 Plan 阶段的门槛

- 冻结 ReviewRequest、ReviewDecision 和 ReviewTrace 的 v1 候选语义。
- 完成 Review、Management、Runtime 三方责任矩阵并消除重叠。
- 明确判断结果不会直接触发副作用的强制门禁。
- 为每个判断终态准备正例、反例、过期和状态漂移验收用例。
- 获得用户对范围、产物和实施顺序的明确审核与授权。
