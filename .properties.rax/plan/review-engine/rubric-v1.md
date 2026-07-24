# Review Rubric V1 实施计划

## 状态

- Owner：Review Owner。
- 当前：实现、owner test与最终独立复审完成，`P0/P1/P2=0`；不声明跨组件 production root GO。
- Policy 边界：只选择 exact Rubric ref/适用条件，不拥有 Rubric payload 或撤销。

## 文件级落点

| 阶段 | 文件 | 产物/验收 |
|---|---|---|
| 合同 | `contract/rubric.go`、`contract/rubric_test.go` | 七 kind、criteria/rules/output schema/capability/termination、canonical digest/currentness |
| Port | `ports/rubric.go`、`ports/store.go` | Owner-only Publish/Revoke、historical exact、current Reader；Admission compound current check |
| reference Store | `memory/rubric_v1.go`、`memory/snapshot_rubric_v1.go`、`memory/{store,snapshot}.go` | append-only history/current full-ref/highest atomic CAS、deep clone、optional snapshot |
| durable Store | `storage/sqlite/rubric_v1.go` | 复用 tenant snapshot/generation CAS；restart/integrity/64 concurrency |
| admission/service | `service/rubric_v1.go`、`service/service.go`、`caseengine/engine.go` | Publish/Revoke lost reply exact recovery；Request Rubric S1/S2 + actual-point recheck |
| conformance | `conformance/rubric_v1.go` | memory/SQLite reusable suite |
| hard negatives | `tests/rubric_store_v1_test.go`、`service/rubric_admission_test.go` | RUB-01..28 |
| 文档同步 | Review design/plan/module/memory、`ExecutionRuntime/review/README.md` | Owner/非Owner、NO-GO、真实测试证据一致 |

## 顺序与依赖

1. Review Owner 发布 active Rubric revision；
2. Policy Owner 选择 exact `{ID,Revision,Digest}` 与适用条件；
3. Request 提交 exact ref；
4. Service S1/S2 + Store actual-point current recheck；
5. Reviewer Context 只读取 exact Rubric 和其允许的只读 capability；
6. supersede/revoke/TTL 漂移后旧 Request/Round Fail Closed。

不依赖 Runtime/Harness/Organization 实现包，不修改 Runtime Port。未来 production root 只注入 Review `RubricStoreV1/RubricCurrentReaderV1`，不得创建第二 Rubric Owner。

## 验收门

- targeted ordinary100、race20；
- full ordinary/race/vet；
- gofmt、Review 范围 diff/stale/import 扫描；
- restart/integrity、64 memory/SQLite concurrency、lost reply、S1/S2 drift、TTL/clock rollback；
- 资产状态只有真实命令全部通过后才能写 owner-local YES；production root 由联合线单独裁决。

## 实际门禁（2026-07-18）

- `go test -count=100 -run 'Rubric' ./...`：PASS，command wall 16.239s；
- `go test -race -count=20 -run 'Rubric' ./...`：PASS，command wall 25.897s；
- `go test ./...`：PASS；
- `go test -race ./...`：PASS；
- `go vet ./...`：PASS；
- gofmt、Review production import、asset link/trailing whitespace、`git diff --check`：PASS。

以上只证明 Review-owned Rubric 合同/Store/SQLite/Admission/conformance；不证明宿主 root、真实 Reviewer Provider、Runtime Authorization 或 SLA。
