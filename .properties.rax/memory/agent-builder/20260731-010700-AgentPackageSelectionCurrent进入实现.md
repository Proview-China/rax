# AgentPackage Selection Current 进入实现

## 当前事实

基于 `origin/main@a9481b2e35ab`，Agent Builder 已有 AgentPackage SQLite exact store 与 Loader 对 committed Harness Publication 的 exact 重读能力。

本次冻结并实现 `VerifiedAgentPackageClosureV1 + AgentPackageSelectionCurrentV1`：Selection 使用 append-only history + current CAS；Unknown 只 Inspect；Service 不接受调用方自报 PublicationRef/ClosureDigest。

## 边界

该结果只闭合 Agent Builder Owner-local nominal selection。Agent Host、StartV3、Runtime、Factory、ProductionEligible 与完整 production composition 仍为 NO-GO。
