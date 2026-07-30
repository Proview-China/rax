# TurnContinuation V1实施计划

## 当前切片

- 冻结Application-neutral `TurnContinuationV1`合同和Harness-owned `ActiveContextRef` CAS Port；
- 在Harness `applicationadapter`落成`SQLiteTurnContinuationStoreV1`：WAL+FULL synchronous、Attempt create-once、exact pending/ActiveContext CAS、canonical row/index anti-splice与restart recovery；
- 复用既有Tool result ref与Context refresh result，不新增平行DTO；
- 用`continuation_pending`阻断旧Context上的下一Model Turn；
- 用预冻结Context prepare Attempt exact四元组阻断same-ID digest splice；
- 只co-seal两个independently sealed exact refs，不证明Context prepare内容来自SettledToolResult payload；给定两端ref后只证明continuation CAS/recovery；
- 补unit、fault、currentness、black-box lost-reply、restart、并发单winner、writer-wait TTL TOCTOU、schema drift和race测试；
- 同步最小design/plan/memory资产。

## 明确保留

1. Harness durable Begin/Commit/Inspect与现有Interaction Loop的composition-root接线；本PR只有Owner-local store，不宣称Loop已接通；
2. Application TurnContinuation coordinator及Context prepare先seal、Begin后调用的实际排序；
   其中必须补SettledToolResult payload到Context prepare内容的真实派生/绑定证据；
3. Context永久失败后的Failure/Run termination状态；当前只能fail-closed + Inspect原Attempt，不能开启下一Turn；
4. Sandbox `workspace.read`窄Port、Coding Execution actual-point和production host plane；#23已加固Unix socket回归测试；
5. next Model predispatch全路径no-bypass、system fixture与production root；
6. Console/TS ViewModel/Event，直到Runtime/Review/Timeline/Sandbox正文分别冻结。

`Tool=1 / Memory=0 / Knowledge=0`已由上游PR #32在`origin/main@f6014b93`闭合Application contract/coordinator与Context adapter delta；本PR只消费该基线，不修改或重复认领Context实现。

## 验证

```text
go test ./contract
go test ./tests -run TestTurnContinuationBlackBox -count=1
go test -race ./contract ./tests -run TurnContinuation -count=1
go vet ./contract ./ports ./tests
cd ../harness
go test ./applicationadapter ./tests/applicationadapter -count=1
go test -race ./applicationadapter ./tests/applicationadapter -count=1
go vet ./applicationadapter ./tests/applicationadapter
git diff --check
```

当前分支已rebase到`origin/main@f6014b93`；本轮在该基线重跑Harness相关ordinary/race/vet/diff-check。Application full ordinary/race/vet与Context full ordinary/vet保留为前一`13f63712`基线证据，不声称本轮重跑。

2026-07-30新增Harness四项命令live PASS；覆盖create-once、persistent idempotency、exact CAS、lost reply、restart、concurrency、anti-splice、TTL/clock与deep-copy。这些结果只验收continuation公共合同与Harness Owner-local durable CAS/recovery，不声称Application coordinator、完整真实Loop、下一Model实际调用、Console/TS或production已接线。
