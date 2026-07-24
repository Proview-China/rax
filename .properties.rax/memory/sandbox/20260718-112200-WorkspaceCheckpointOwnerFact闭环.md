# Workspace Checkpoint Owner Fact闭环

时间：2026-07-18 11:22（Asia/Shanghai）

## 当前事实

- 新增`WorkspaceCheckpointCoverageFactV2`与`WorkspaceCheckpointParticipantFactV2`；只有
  Residual为空才能形成`complete/prepared` owner-local事实，声明的process/network/secret/device
  排除项不被伪装为已恢复内容。
- `WorkspaceCheckpointPreparationCurrentProjectionV2`由Owner Reader派生Snapshot Aggregate exact
  current；caller Request不能注入该current ref。
- Controller执行S1/read -> seal candidate -> S2 exact digest -> create-once commit；S2漂移与
  Partial均零写，lost reply只Inspect原identity。
- Store以`TenantID + ScopeDigest + AttemptID + ParticipantID`及结构化Fact key隔离；同ID跨Tenant
  独立，错误Scope读取拒绝。
- Application Adapter复读Prepared Bundle、Artifact Fact和expected Aggregate current后才发布公共
  Participant/Snapshot/Coverage candidate；Sandbox raw SHA-256 digest映射到Runtime时使用
  `sha256:<hex>`，不再产生无效`core.Digest`。

## 实测

```text
go test ./contract ./kernel ./applicationadapter ./tests -run 'WorkspaceCheckpoint' -count=100  PASS
go test -race ./contract ./kernel ./applicationadapter ./tests -run 'WorkspaceCheckpoint' -count=20  PASS
go test ./...  PASS
go test -race ./...  PASS
go vet ./...  PASS
```

Sandbox模块因Agent Assembler release repository的live SQLite依赖补齐了模块图`go.sum`与精确
indirect require；白盒门继续逐文件禁止Sandbox production包直接导入这些外部包。

## 仍未解锁

Evidence/Settlement Lifecycle、真实Provider、production composition root、Restore、Retention/Purge、
remote backend与SLA仍为NO-GO；`FeatureCheckpointParticipant=false`、`FeatureCheckpointRestore=false`。
