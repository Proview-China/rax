# AgentPackage Verified Closure + Selection Current V1

## 1. 冻结边界

本切片只闭合 Agent Builder Owner 的两个名义事实：

1. `VerifiedAgentPackageClosureV1`：由 Loader 对 exact `AgentPackageV1` 与 committed Harness `AssemblyPublicationBundleV2` 重新读取并复算；
2. `AgentPackageSelectionCurrentV1`：把一个 `SelectionID` 当前选择的 Package exact ref、Publication exact ref 与 Closure digest 持久发布。

Selection 只是 host-facing nominal package coordinate，不是 executable assembly、Factory、Activation、Runtime Binding、ProductionEligible 或 SystemReady。

## 2. Verified closure

公共 Reader 为：

```go
LoadVerifiedAgentPackageClosureV1(ctx, AgentPackageRefV1) (VerifiedAgentPackageClosureV1, error)
```

closure 必须重新验证 Package、Publication、Generation、Manifest、Graph、Handoff、Assembly Input digest、compiler version 与 frozen time 的完整 exact 链；任一 missing、uncommitted、splice、body drift 或 digest drift 均 Fail Closed。

## 3. Selection current

`AgentPackageSelectionCurrentRefV1` 只含：

- `SelectionID`
- `Revision`
- `Digest`
- `ExpiresUnixNano`

Current 保存 `Ref`、PackageRef、PublicationRef、ClosureDigest、Checked/Expires 与 ProjectionDigest；必须满足 `Ref.Digest == ProjectionDigest` 且 `Ref.ExpiresUnixNano == Current.ExpiresUnixNano`。

Owner ports 固定为：

```go
InspectAgentPackageSelectionCurrentV1(ctx, selectionID)
InspectAgentPackageSelectionExactV1(ctx, ref)
CompareAndSwapAgentPackageSelectionCurrentV1(ctx, expected, next)
```

create 使用全零 expected 与 revision 1；advance 必须绑定 exact predecessor 且 revision 只加 1。CAS 进入事务后先读取 next exact history：相同 expected + 相同 next 即使 current 已继续前进，也只幂等读回原结果；非零 expected 必须同时从 exact history 复读成功并证明是 next 的前一 revision，伪造 digest/expiry 不得借历史 next 返回成功。相同坐标异 body 或相同 expected + 不同 next 均冲突。history append-only，current pointer 在同一 SQLite 事务内 CAS。Pointer 自带 row digest，并且 current read/CAS 都要求 pointer revision 等于该 Selection history 的最大 revision，拒绝回拨到旧的合法 history。

Repository clock 只在取得进程锁并开始 serializable transaction 后采样；写入事务后、commit 前必须再次采样并拒绝 clock regression、`now == expires` 或 expired next/predecessor，等待锁不能发布已经过期的 current。

`RequestDigest` 只保护一次 sealed request 的 canonical 完整性，不是 command idempotency key。Service 不承诺把不同调用时刻派生出的不同 next 合并为同一命令结果；幂等边界仅是 Repository 的 same expected + same next CAS。

## 4. Unknown / Inspect

CAS 回包未知后，只 Inspect 原 next exact ref；不得重试 CAS、换 Package、换 SelectionID 或创建下一 revision。Current Reader 使用 Repository 自有 clock 拒绝过期；Exact Historical Reader 仍可读过期记录，但不授予任何激活资格。

## 5. 明确禁止

- 不修改或调用 Agent Host、StartV3、Runtime、Factory、Provider、Console；
- 不增加 Host/Console API；
- 不声明 ProductionEligible、Activation 或 SystemReady；
- 调用方不得提交 PublicationRef 或 ClosureDigest，Service 只能从刚验证的 closure 派生。
