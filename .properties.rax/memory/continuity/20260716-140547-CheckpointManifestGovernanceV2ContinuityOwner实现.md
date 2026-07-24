# Checkpoint Manifest Governance V2 Continuity Owner实现

时间：2026-07-16 14:05:47 CST

状态：Checkpoint-first V2联合设计终审YES后，Continuity Owner的Manifest/Seal最小垂直切面已实现并通过全门；跨Owner Checkpoint集成与Restore继续`NOT READY`，Provider调用为`0`。

## 本次落地

- 新增approved `CheckpointManifestGovernancePortV2`及独立Manifest Repository/Reader；
- 新增V2 `ExactFactRef`，跨Owner引用至少绑定contract/schema、Owner Binding、ID、revision、digest和scope，不复制Runtime Outcome、Review Verdict或Participant状态语义；
- 新增`CheckpointManifestFactV2` create-once/CAS、immutable revision history、Owner current Inspect与exact historical Inspect；
- 新增canonical required Participant set与frozen-ref-set digest重算，换ref、revision、digest或closure时Fail Closed；
- 新增已Begin Attempt的opaque Settlement closure规则：缺Settlement时必须保留exact Inspection+Residual，且只能进入diagnostic finalization；Unknown不猜测、不重派；
- 新增`diagnostic_partial/diagnostic_indeterminate/rejected` Owner finalization与terminal状态机；
- 新增immutable revision 1 `CheckpointManifestSealFactV2` Repository/Reader：only-current `verified_candidate`可Seal，exact绑定Manifest revision/digest、Attempt、Barrier、EffectCut、frozen-set和Participant closures；
- same-ID/same-canonical内容幂等，changed content Conflict；manifest/seal idempotency key禁止lost reply后改ID重建；Seal没有CAS/delete/TTL/current Restore字段；
- 新增线程安全内存reference backend、深拷贝/no-alias、typed-nil拒绝、lost-reply fault fake及unit/whitebox/blackbox/fault/conformance/并发测试；
- `conformance.Wave1Manifest`新增`continuity/checkpoint-manifest-governance-v2`，同时继续把checkpoint capture、restore execute、remote backend及公共Adapter列为unsupported。

## Owner边界与NO-GO

- Continuity只写自身Manifest/Seal Fact，不协调Runtime Barrier，不写`CheckpointConsistencyFact`或Attempt Finalization；
- Continuity不执行Participant Prepare/Commit/Abort，不捕获Snapshot，不调用Harness/Sandbox/Provider；
- Manifest或Seal不等于Runtime consistent，也不提供Restore current资格；Partial/Indeterminate/Rejected只作诊断；
- Restore Plan仍为historical/reference-only纯验证；没有`RestoreGovernancePortV2`运行面、Stage、Activate或外部世界回滚；
- SQLite/RocksDB仍只有SPI，无production driver；remote blob/purge/archive、SDK/CLI/API和生产SLA继续NO-GO；
- legacy Checkpoint/Restore Port不得包装、补默认值或升权为V2。

## 实际验证

工作目录：`ExecutionRuntime/continuity`

```bash
go test -count=100 ./contract ./domain ./fakes ./storage/memory ./tests/blackbox ./tests/fault ./tests/conformance
go test -race -count=20 ./domain ./fakes ./storage/memory ./tests/fault ./tests/conformance
go test -count=1 -shuffle=on ./...
go test -race -count=1 ./...
go vet ./...
gofmt -l .
go list -deps ./...
rg -n '"github\.com/Proview-China/rax/ExecutionRuntime/(runtime|harness|application|sandbox|context-engine|tool-mcp|model-invoker|memory-knowledge|review)/' --glob '*.go' .
```

结果：target `count=100`全部PASS；定向`race -count=20`全部PASS；full ordinary、full race、vet、go list全部PASS；`gofmt -l`与禁止跨Owner import扫描均无输出。Markdown/链接/diff/范围轻门在资产写入后单独执行。

本次未修改Runtime、Harness、Application、Sandbox、Context、Tool、Model Invoker、生产后端、根配置或全局索引；未stage、未commit。
