# HostDeploymentCurrentV2

## 状态

本设计是 Agent Host 的单节点、Owner-local Deployment Current 合同；它不是 Deployment Controller、生产 Composition Root 或 Production Eligibility 证明。

## 所有权

- Builder 拥有 `AgentPackageSelectionCurrentV1` 与 `VerifiedAgentPackageClosureV1`。
- Runtime 与服务 Owner 分别拥有 Resource/Service Current。
- Agent Host 只拥有 `HostDeploymentCurrentV2`。
- Host Current 只持久化 Builder 的 exact `AgentPackageSelectionCurrentRefV1`；不得复制 `PackageRef`、`PublicationRef` 或 `ClosureDigest`。

## 发布边界

```text
exact Selection + current Selection
  -> LoadVerified closure
  -> exact Resource/Service current S1
  -> exact Resource/Service current S2
  -> fresh seal
  -> append history + current CAS
```

TTL 是 Bootstrap、请求、Selection、Resource 与 Service 有效期的最小值。`now == expiry` 必须 Fail Closed。

Unknown mutation只能 Inspect 原 desired exact Ref；历史存在后还必须确认 raw current pointer 仍指向该 exact Ref。已被替代的历史不是 Current。

## 持久化

SQLite schema v7 使用 append-only history 与 current pointer CAS，开启 WAL、foreign keys 与 `synchronous=FULL`。打开数据库时通过 PRAGMA、STRICT、约束行为探针、额外 index/trigger 枚举证明物理结构，不能只信 schema ledger 或 SQL 字符串。

## NO-GO

- 不提供 Component Factory 或 HostV3 注入。
- 不拥有 Builder、Runtime、Provider、Application 或 Harness 事实。
- 不声称多节点、HA、远程持久化或生产可用。
