# Workspace Read Exact Current V1

## 边界

该切片由 Sandbox Owner 持有，只读聚合 Runtime 既有 exact current、Prepared Association、Sandbox WorkspaceReadCommand 与 WorkspaceView。它不新增 Runtime 业务类型、Gateway、Permit、写面或 Console 合同。

## 查询与证明

查询携带 sealed `ControlledOperationPhysicalExecutionAuthorizationV3`，并把 StableKey、Authorization、Association、DomainCommand、Command、WorkspaceView、FileScope 与逻辑路径封入 canonical digest。StableKey 与 Authorization 不能由 caller 自报后回显，必须与 sealed Authorization 的重算结果一致。

## S1/S2

Adapter 在 S1 与 S2 分别重读 Association、Command、WorkspaceView 和 Runtime Enforcement。Runtime 顶层 fresh `CheckedUnixNano/Digest` 允许变化；Dispatch、Sandbox projection、Journal/Phase、Lease/Fence、Provider、Review、Permit、Admission 与所有 Owner exact refs 必须保持语义一致。任何漂移 Fail Closed，且该 Adapter 没有写面或 Provider 调用能力。

## Digest

`ProjectionDigest`封住一次完整读取及其时间；`SemanticDigest`排除读取时间与 Runtime 顶层 fresh envelope digest，因此同一未变化 current closure 的并发读取具有同一语义摘要。

## 集成阻塞

现有 workspace-read actual-point caller 尚未把 Association exact ref 与 sealed Authorization 交给 `DispatchInput`。本切片提供显式 exact handoff 和 CurrentServer 消费路径，但不修改 kernel/storage 来伪装既有调用已经接线。
