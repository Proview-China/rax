# Continuity P4 Component Release/readiness

时间：2026-07-18 01:01 +08:00

## live裁决

- SQLite WAL/FULL schema v8真实覆盖Checkpoint Manifest/Seal、Timeline、Artifact、History、RestorePlan等owner-local metadata/current/history/CAS。
- RocksDB build-tag实现真实ContentStore Chunk边界，但不授remote provider或deployment资格。
- Checkpoint-first跨Owner链当前仍是reference/test纵切，`ProviderCalls=0`；真实Participant capture、remote blob、Restore execute、physical cleanup/purge/archive与production root不存在。
- `conformance.Wave1Manifest`强制restricted、reference-only、无Production SLA。

因此本轮发布`reference_only ComponentReleaseV1`；没有production promotion API，强改production由Assembler conformance/residual校验失败关闭。

## 实现

- 新增`ExecutionRuntime/continuity/releasecandidate`：完整Manifest/Module/Capability/Port/Factory descriptor、effect/settlement/cleanup owners、artifact、candidate certification、evidence、TTL与readiness proof boundary。
- publisher只依赖Agent Assembler公共publisher/reader ports；Ensure回包indeterminate时只用同一exact ref Inspect，不重派mutation。
- Host只消费Factory descriptor；无Host、Assembler repository、Harness/Application实现反向import。
- production P0固定为：durable/current deployment attestation、remote blob provider、真实Participant capture、Restore execute、cleanup/purge/archive conformance、production composition root。

## 验证

在`ExecutionRuntime/continuity`实际通过：

```text
go test ./releasecandidate -count=100
go test -race ./releasecandidate -count=20
go list -deps ./releasecandidate | rg 'ExecutionRuntime/(agent-host|host|application|model-invoker)|agent-assembler/(repository|resolver)|harness/(kernel|internal|ports)'
go test ./...
go test -race ./...
go vet ./...
go test -tags continuity_rocksdb ./storage/rocksdb
go test -race -tags continuity_rocksdb ./storage/rocksdb
go vet -tags continuity_rocksdb ./storage/rocksdb
```

import命令无输出；release package未导入所列实现包。仓库范围`git diff --check`、新文件显式`--no-index --check`及trailing-whitespace扫描均通过。

覆盖：publisher lost reply exact recovery、64并发确定性、TTL crossing、clock regression、artifact/published/proof/conformance drift、typed nil、production fail-closed、公共类型复用与Factory-descriptor-only。

未运行真实remote Provider、Participant capture、Restore execute、cleanup/purge/archive或deployment root测试，因为这些production能力不存在；不得记录为通过。
