# Runtime Review Verdict V2与条件审核门禁完成

## 事件

2026-07-14，Runtime P0.3完成Review Verdict V2公共合同、权威事实状态机、Governance Gateway、条件满足事实、派发投影、测试Fact Owner与Conformance。

## 已落地

- 无环的Subject→Policy→Candidate→Effect ReviewBinding两阶段绑定；
- Case、Verdict、Condition Satisfaction独立CAS事实；
- Create/Decide/Satisfy只通过ReviewGovernanceGateway复读current事实；
- conditional Satisfaction精确进入Permit，Issue/Begin/实际执行点三次复读；
- Satisfaction不在同一Verdict内替换；失效后必须创建新Effect revision与新Review/Permit链；
- 自动local/remote Reviewer必须绑定独立已settled Effect；
- self-review默认拒绝，显式Policy才可允许；
- operation_not_required必须由显式Policy Fact证明；
- 执行点新增Authority、Review、Budget、Policy Reader，禁止信任调用方自由Current；
- canonical nil/empty与SandboxLease值语义在持久round-trip中稳定。

## 验证

在`ExecutionRuntime/runtime`实际通过：

```text
go test -count=1 ./...
go test -count=1 -race ./...
go vet ./...
go test ./tests/ports -run='^$' -fuzz=FuzzReviewEvidenceCanonicalSetV2 -fuzztime=2s
git diff --check
```

## 边界与下一步

内存fake不代表生产后端或一致快照SLA。P0.4将实现Evidence Ledger V2与Timeline单主，并通过显式Adapter接入本阶段稳定Evidence Ref/Digest；不得把Review投影或Reviewer Observation提升为Ledger权威事实。
