# AgentPackage Selection Current V1 模块说明

## 作用

该模块把已经持久提交并由 Loader 完整验证的 AgentPackage + Harness Publication 闭包，发布为一个可被下游按 exact ref 读取的当前选择。

## 组成

- `VerifiedAgentPackageClosureV1`：完整 verified value proof；
- `AgentPackageSelectionCurrentV1`：名义 current projection；
- `ServiceV1`：强制先 fresh exact load，再派生 current；
- `SelectionSQLiteV1`：append-only history、current CAS、exact/current Readers。

## 使用约束

调用方只提供 `SelectionID`、Package exact ref、expected current 与有效期。Publication ref 和 Closure digest 由 Service 从刚验证的 closure 内部产生。Current 过期后 current Reader Fail Closed；exact history 仍只作为审计与恢复事实。

## 不包含

不包含 Host、StartV3、Runtime、Factory 构造、Provider 调用、Activation、ProductionEligible、SystemReady 或 Console API。
