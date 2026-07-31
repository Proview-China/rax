# Workspace Read Execution Command V1模块说明

## 用途

该模块在effect前，把一次Tool Request的Claim/ExecutionState/Binding/Candidate与Runtime的
Effect/Prepared/full Attempt封成一个Tool-owned immutable command。Sandbox后续只能引用该
exact command，不能继续引用generic Tool object。

## 公开对象

- `WorkspaceReadExecutionCommandRefV1`
- `WorkspaceReadExecutionCommandV1`
- `WorkspaceReadExecutionCommandCurrentV1`
- `WorkspaceReadExecutionCommandExactReaderV1`
- `WorkspaceReadExecutionCommandAttemptReaderV1`
- `WorkspaceReadExecutionCommandCurrentReaderV1`

create request、Owner producer及raw repository writer均位于Tool Go `internal`边界，不是跨Owner
公开写Port。

## 持久化

- 单表create-once；
- command ID主键，以及RequestKey、Tool ExecutionAttempt、Runtime Attempt三条外部因果轴唯一；
- full Attempt canonical digest覆盖Delegation和Permit；
- lost reply/restart只Inspect；
- 不执行外部effect。

历史Fact保存稳定语义与稳定absolute TTL。Registry、Runtime Effect、Runtime Prepared的fresh
Checked/Expires/ProjectionDigest只进入Current；Binding/Input是create-once固定winner，保留exact
Ref与稳定body，但当前只有InMemory Store，因此重启后的fresh Current仍是NO-GO。

## 边界

本模块不是Runtime authorization、Sandbox admission或Provider transport。它只建立Tool Owner
的唯一因果事实。Payload Source/Material、Classification和Context投影不在本模块内。

设计入口：[workspace-read-execution-command-v1.md](../../design/tool-engine/workspace-read-execution-command-v1.md)

计划入口：[workspace-read-execution-command-v1.md](../../plan/tool-mcp/workspace-read-execution-command-v1.md)
