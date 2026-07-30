# AgentExecutionAvailabilityCurrentReaderV1

## 状态

本设计冻结 Agent Host Owner 的 owner-local/reference 切片。它新增
`HostStartPackageSelectionBindingV1`，但不修改既有 `HostStartClaimV1`、
`HostStartClaimInputV3` 或旧 `ClaimOrInspectHostStartV3`。Availability Reader
只返回 Runtime 已有 `AgentExecutionAvailabilityProjectionV1`。

## Host-owned immutable association

每个 exact Claim 最多有一份 revision=1 的
`HostStartPackageSelectionBindingV1`：

- Ref 绑定 HostID、StartID、exact ClaimRef、expiry 与 digest；
- Body 绑定 InputV3 BindingDigest、exact DeploymentV2 Ref、与该 DeploymentV2
  完全相同的 PackageSelection Ref、同一 Selection current 的 Verified Closure
  digest；
- TTL 严格等于 Claim/InputV3、DeploymentV2、Selection 的最小值；
- BindingRef digest 与 BindingDigest 相同，create-once 后不可替换。

新复合 Host port 在同一 SQLite transaction 内原子创建 ClaimV1、InputV3 sidecar
与 Binding。已由旧 V3 port 创建但缺 Binding 的 Claim 不允许补写升级。mutation
返回 Unknown 时，只能用原 BindingRef exact Inspect 一次，不重试 mutation、不选择
另一 DeploymentV2/Selection。

SQLite schema v8 新增 STRICT association 表，以 HostID+StartID 为主键，并以
DeploymentV2 history 的完整 Deployment/Selection exact coordinate 为外键。
迁移保留 v7 proof，另记录并物理校验 v8 DDL、列、索引、外键、STRICT 与 trigger。

## Availability exact 闭包

构造配置不再含 `DeploymentV2Ref` 或 `BindingRef`。每次读取：

1. 读取 Availability 对应的 SystemReady Current/Fact、exact Claim 与 InputV3；
2. 用已经读取的 exact ClaimRef 调用 Binding ForClaim reader；
3. 从 Binding 取得 DeploymentV2 Ref，再读 DeploymentV1/V2；
4. 从 Binding 取得 Selection Ref，复读 Selection exact/current 与 Verified Closure；
5. 复读 SystemReady 全部 owner current、Component current、CleanupClosure；
6. 用 `<=1s` exact request 读取 HostV3 Inspect；
7. 对上述完整水印执行 S1/S2，fresh clock 封口。

Ready TTL 必须是全 Owner 闭包最小值；Binding、Selection、Closure 也进入水印与 TTL。
任何 splice、drift、过期、`now==expiry`、clock rollback、typed nil、unavailable 或
cancel 均 Fail Closed。Reader 不重签、不缓存、不创建第二 Availability 事实。

## NO-GO

本切片只有 owner-local/reference 实现。HostV3 production Start composition 尚未
冻结并调用新复合端口；旧 V3 Claim 不具备本证明。Runtime actual-point guard 也尚未
接入 Host availability input。故 production composition 仍为 NO-GO，不得宣称
Availability 已闭合。
