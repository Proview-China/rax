# Sandbox Owner SQLite持久State Plane

时间：2026-07-18 11:59:54（Asia/Shanghai）

状态：`implementation_software_test_yes / single-node-durable-owner-store / deployment-pending`

本轮在`ExecutionRuntime/sandbox/storage/sqlite/**`落地Sandbox Owner单节点持久State Plane：

- WAL、`synchronous=FULL`、foreign key、busy timeout与immediate写事务；
- 生命周期Reservation/Observation/Inspection/DomainResult；
- Environment Projection append-only history/current与opaque Settlement→DomainResult exact binding；
- Sandbox DomainResult→Runtime public exact ref的create-once binding；
- Application lifecycle plan/result；
- SnapshotArtifact reserved→available的Reservation/Fact/Entry/Envelope/current index原子CAS；
- source epoch/sequence ordering、重启恢复、lost-reply Inspect、no-ABA与64路不同内容单赢家。

Owner边界不变：该库只保存Sandbox拥有的事实和引用，不复制或伪造Runtime、Retention/Legal Hold、
Continuity、Review、Management或Provider权威事实；这些current输入仍由对应Owner公共Reader注入。

实际验证：

```text
go test -count=1 -shuffle=on ./...                         PASS
go test -count=100 -shuffle=on ./storage/sqlite            PASS
go test -race -count=20 -shuffle=on ./storage/sqlite       PASS
go test -count=100 -shuffle=on ./...                       PASS
go test -race -count=20 -shuffle=on ./...                  PASS
go vet ./...                                               PASS
gofmt -l .                                                 PASS（零输出）
```

仍未关闭：Checkpoint Phase Apply closure、Restore、Snapshot retention/purge/cleanup、真实Workspace
commit、Host/MicroVM/Remote、公共SDK/CLI/API、外部Owner readiness、宿主部署认证、分布式State Plane与SLA。
