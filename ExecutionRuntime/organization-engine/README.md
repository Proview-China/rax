# Organization Engine

本模块是 Review 企业多签所需的最小 Organization current Owner。它拥有 tenant-qualified Human Identity、Role Grant、Delegation 和 Responsibility 的不可变事实、append-only history、full-ref current CAS，以及 Review Eligibility 的 S1/S2 current projection。

## 已实现

- Go 1.25 contract：具名 exact refs、stable ID、canonical digest、revision `+1`、active/terminal/TTL；
- memory reference store：仅供测试和 conformance；
- single-node SQLite WAL State Plane：真实 fact history/current/projection tables、schema digest、integrity check、restart recovery；
- `ResolveCurrentReviewEligibilityV1` / `InspectCurrentReviewEligibilityV1`：Identity、全部 required Role、Delegation、Responsibility 与两侧 Identity current closure，min TTL、S1/S2、clock rollback、deep clone；
- production self-review fail closed；explicit Delegator/Delegate/Role/Scope binding；
- reusable Store/Reader conformance、unit/whitebox/blackbox/fault/concurrency 测试。
- 声明式Component Release：reference-only、exact SQLite standalone与production readiness合同；Publisher lost-reply恢复、TTL/clock和64并发门；Factory仅descriptor。

## 不拥有/不支持

- 不签发 Runtime Authority，不形成 Review Attestation/Verdict，不计 K-of-N，不授 Evidence；
- 不提供 production composition root、网络 API、UI、远程 Provider、HA 或 SLA；
- 不提供可执行Factory或真实production readiness；Release descriptor不能被当作constructor；
- Organization exact refs 只是真实 Owner fact 坐标，不单独构成执行授权；
- Review 的 Policy、Runtime Authority/Binding/Evidence/Scope、Authorization V5 与 host root 仍需对应 Owner 关闭。

## 验证

在本目录运行：

```text
go test ./...
go test -race ./...
go vet ./...
go test -count=100 ./contract ./current ./memory ./storage/sqlite
go test -race -count=20 ./contract ./current ./memory ./storage/sqlite
go test -count=100 ./release
go test -race -count=20 ./release
```
