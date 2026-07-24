# Declarative Agent 三项公共 P0 候选冻结

## 事件

live调查确认AgentDefinition、Agent Assembler和HostV2 Start/Inspect参考纵切已经可编译并通过普通测试与vet，但仓库仍不能由一份配置经非测试入口直接启动完整Agent。

本次形成三个待用户审核的候选资产：

- Host-owned Cleanup Closure V2：Start绑定唯一CleanupPlan与完整coverage，Stop调用方PlanRef只作expected；
- Application Agent Activation V2：把Preflight到Ready八步改为typed Owner facts/current与unknown inspect-only；
- Declarative Composition Root V1：冻结唯一CLI/service、bootstrap与AgentDefinition分层、factory/readers注入和系统完成门。

## 当前真值

- AgentDefinition/Assembler/HostV2库级闭环：YES；
- 七组件production release/current/executable factory：NO；
- 真实Activation V2、Stop Closure、production root/CLI：NO；
- `AgentDefinitionV1`继续严格要求production，不因本候选放宽；
- 三份设计及计划均为待用户审核，未授权Go实现。
