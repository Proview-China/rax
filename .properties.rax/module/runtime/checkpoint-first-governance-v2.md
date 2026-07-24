# Runtime Checkpoint-first Governance V2模块说明

状态：**Checkpoint-first V2纵切及Continuity ManifestSeal 2.1.0 exact Reader reference链完成**。新增链已通过ordinary100/race20/full ordinary/race/vet；该结论不构成Harness/Application/Participant生产装配、Restore、Provider、root或SLA资格声明。

最新Delta与验证真值见[Checkpoint ManifestSeal V2.1 Exact Reader Delta](../../memory/runtime/20260717-233200-CheckpointManifestSealV2.1ExactReaderDelta.md)。

## 1. 作用与Owner

本切面把Checkpoint第一波收敛为Runtime唯一拥有的线性化状态机：原子创建`CheckpointAttempt+BarrierLease`，冻结不可变`EffectCut`，并以`Consistency+CloseBarrier`或Runtime派生的非成功终态`Finalize+CloseBarrier`结束Attempt。它不实现Restore，也不接触真实Provider。

Runtime不拥有Diagnostics、Residuals、Continuity ManifestSeal或Participant DomainResult。相关Owner分别create-once Seal/Fact，Runtime只保存typed exact ref、在最终CAS前复读current，并将historical Fact与terminal-current Projection分离。

## 2. 公共合同

- `ports/checkpoint_governance_v2.go`：Attempt、Barrier Policy/current、原子bundle与Governance Port；
- `ports/checkpoint_closure_v2.go`：EffectCut、Finalization Cut、两个Owner Seal、Runtime Closure、Consistency/Finalize与terminal-current Projection；
- `ports/checkpoint_participant_reservation_v2.go`：prepare/commit/abort Reservation与`prepared -> commit XOR abort`中立合同；
- `ports/checkpoint_manifest_seal_v2.go`：Continuity immutable ManifestSeal完整Owner/Scope exact lookup、Participant closure结构化映射、canonical digest与只读projection；
- `ports/checkpoint_restore_evidence_v1.go`：Checkpoint专用Qualification/Handoff/Consumption；
- `ports/operation_checkpoint_settlement_v5.go`：Participant DomainResult到Runtime Settlement V5的四对象终态闭包。

## 3. 原子与恢复边界

Reference Fact Owner用单锁/copy-on-write边界证明：

1. Attempt与Barrier同时出现或同时不存在；
2. EffectCut与Attempt revision同事务；
3. Finalization Cut/Closure分别与Attempt history绑定；
4. Consistency与closed Barrier同时发布；
5. non-success Finalize与closed Barrier同时发布；
6. Settlement V5的Settlement、Association、shared Guard、Projection全有或全无。

mutation回包丢失只使用`context.WithoutCancel`按deterministic ref Inspect；同canonical幂等，changed-content、revision rewind、兄弟分支或ABA均Conflict。historical终态可持续审计，但Owner Seal漂移、Unavailable或Indeterminate会使terminal-current Projection fail closed。

## 4. Evidence与Settlement

Evidence V1仅允许checkpoint prepare/commit/abort。Issue Qualification不推进source cursor；Handoff不调用Provider；Consumption通过真实Evidence Ledger Record Owner Reader按Ref与Source执行S1/S2 exact复读，任一record/chain/source/payload/scope漂移零cursor。只有`consumed_current`可进入Settlement V5，`consumed_observation`只保留审计且不能升级终态。

Settlement V5精确绑定Checkpoint Attempt/phase、typed Participant Fact/DomainResult、Evidence/Handoff、Dispatch Attempt、execute Enforcement与Owner。它扩展现有`OperationEffectStoreV3`同一实例/同一锁，并与V3/V4共享`(TenantID, EffectID)` terminal guard；任一版本先胜，其他版本零sidecar失败。

## 5. 验证

Owner实际执行：

```text
go test -count=100 ./tests/ports ./tests/control ./tests/fakes -run Checkpoint
go test -race -count=20 ./tests/ports ./tests/control ./tests/fakes -run Checkpoint
go test -count=1 -shuffle=on ./...
go test -race -count=1 ./...
go vet ./...
gofmt -l <本纵切Go文件>
git diff --check -- .
```

覆盖atomic staged failure、lost reply、64并发、Consistency/Finalize、Owner Seal current漂移、Evidence record/source/chain S1/S2与cursor、observation拒绝、V3/V4/V5 guard、DomainResult二次current复读和clock rollback。最终Owner targeted ordinary `count=100` PASS（tests/fakes 172.053s），targeted race `count=20` PASS（tests/fakes 347.009s）；中央与独立审查full ordinary/race/vet/gofmt/diff均PASS。public Conformance固定`ProviderCalls=0`、`ProductionClaimEligible=false`。

## 6. 限制

当前只有内存reference store、fixture Owner与Conformance；不声明进程崩溃耐久、production persistence、availability、SLA或物理exactly-once。Runtime↔Continuity exact Seal Reader Adapter已实现，但没有Harness/Application/Participant production composition root、Checkpoint capture或Provider。Restore的Eligibility、Attempt、新Instance/high Epoch/new Lease/Fence、Activate/Abort及Evidence/Settlement current rows全部unsupported，必须另行联合Review与授权。

第三次独立代码终审结论为YES（P0/P1/P2=0）。最终返修闭合EffectCut outer及V4 nested Attempt、V5 fail-closed、Consistency完整Attempt/Barrier inventory与canonical回值、真实Run current revision、逐revision create recovery lineage、Permit/Policy/source/schema/payload/ledger Evidence current闭包、Handoff/Consumption历史/当前恢复及terminal-current typed-nil preflight。

设计入口：[Checkpoint/Restore Governance V2](../../design/runtime/checkpoint-restore-governance-v2/README.md)。
