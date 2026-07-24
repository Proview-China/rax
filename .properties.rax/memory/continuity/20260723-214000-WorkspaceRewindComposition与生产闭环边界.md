# Workspace Rewind Composition与生产闭环边界

时间：2026-07-23 21:40 Asia/Shanghai

## 本轮完成

- Continuity `RewindPlanFactV2`既有exact Plan/current/history/CAS/TTL/lost-reply边界保持不变。
- Sandbox Owner新增并闭合`WorkspaceRewindCompositionPortV1`：只接受exact Workspace View、keep/drop ChangeSet refs、稳定Request/Idempotency及planned ChangeSet ID。
- Owner双读并结构化组合后，只创建新的`staged` ChangeSet与immutable revision-1 Composition Fact；历史Fact在TTL过期后仍可exact Inspect。
- SQLite事务内重验current View、Tenant/Scope/Base/FileScope、keep/drop exact refs、planned ChangeSet内容和共同TTL。
- same-request exact replay幂等；同Request换内容Conflict；create回包丢失只Inspect原Composition；64并发单赢家；Repository直写不能绕过Owner closure。
- Sandbox SDK、API action、Reconcile Inspect与host root已接入该Owner Port；该动作本身无文件副作用。

## 实际验证

在`ExecutionRuntime/sandbox`：

```text
go test ./storage/sqlite -run WorkspaceRewind -count=100                  PASS
go test -race ./storage/sqlite -run WorkspaceRewind -count=20            PASS
go test ./...                                                             PASS
go test -race ./...                                                       PASS
go vet ./...                                                              PASS
```

在`ExecutionRuntime/continuity`：

```text
go test ./...                                                             PASS
go test -race ./...                                                       PASS
go vet ./...                                                              PASS
```

## 仍阻止完整生产闭环

1. Application/Assembler尚未把Composition exact ref接入既有`praxis.sandbox/workspace-commit`的Admission、Review/Authorization、Permit/Fence、Begin、双重Enforcement、Evidence、DomainResult与Runtime Settlement链。
2. Runtime Run Settlement公开Participant Port当前只提供exact Inspect；组件Owner create/CAS及trusted Assembler Requirement映射没有冻结公共写合同，Continuity不得私建替代接口。
3. 首版Required Checkpoint Participant虽冻结为Runtime、Harness、Context、Sandbox、Memory-Knowledge，但Context与Memory-Knowledge的生产Participant Snapshot/Coverage/current合同尚未全部冻结并装配。
4. production trusted Assembler/current Readers、root credential/deployment attestation、跨Owner系统Conformance仍缺失。
5. remote blob/purge/archive/KMS/SLA、Compactor/Indexer/Consolidator算法与执行合同未获设计冻结，不在本轮实现；Partial Checkpoint仍只诊断，Rewind/Restore不宣称外部世界回滚。

结论：本轮完成了规划内已有明确合同的最后一个Owner-local Rewind缺口；当前候选仍为`reference_only`，不能标记完整production GO。
