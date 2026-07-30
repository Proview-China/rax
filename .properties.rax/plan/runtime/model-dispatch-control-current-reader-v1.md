# Model Dispatch Control Current Reader V1 Plan

状态：owner-local implementation software-test PASS，等待独立代码审计；production composition NO-GO。

- [x] 核对`AgentRunRecord`、`DesiredStateSnapshotV2`、`ApplicationCommandRecordV2`可证明闭集。
- [x] 新增Run与Command结构化窄Reader，不持有Fact写面。
- [x] S1/S2复读Run、Desired、唯一LastCommand及canonical watermark。
- [x] fresh clock、1秒最大TTL、clock rollback与context cancel Fail Closed。
- [x] dispatchable/cancel_requested/fenced/revoked/indeterminate闭集。
- [x] LastCommand缺失/错scope/错revision/错precondition、superseded/indeterminate反例。
- [x] typed-nil、Unavailable/Indeterminate、64并发读、零写入、import-boundary反例。
- [x] public owner-local conformance，固定`ProductionEligible=false`。
- [x] targeted ordinary count=100、race count=20、full shuffle/race、vet、gofmt、diff-check。
- [ ] 独立Runtime代码审计。
- [ ] 生产SQLite Run/Command Owner与composition root。
- [ ] Host availability、Model Boundary CAS winner与routegateway no-bypass跨Owner闭环。
