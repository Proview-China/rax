# TurnContinuation V1实施计划

## 当前切片

- 冻结Application-neutral `TurnContinuationV1`合同和Harness-owned `ActiveContextRef` CAS Port；
- 复用既有Tool result ref与Context refresh result，不新增平行DTO；
- 用`continuation_pending`阻断旧Context上的下一Model Turn；
- 用预冻结Context prepare Attempt exact四元组阻断same-ID digest splice；
- 只co-seal两个independently sealed exact refs，不证明Context prepare内容来自SettledToolResult payload；给定两端ref后只证明continuation CAS/recovery；
- 补unit、fault、currentness、black-box lost-reply和race测试；
- 同步最小design/plan/memory资产。

## 明确保留

1. Harness durable Begin/Commit/Inspect owner实现与现有Interaction Loop接线；
2. Application TurnContinuation coordinator及Context prepare先seal、Begin后调用的实际排序；
   其中必须补SettledToolResult payload到Context prepare内容的真实派生/绑定证据；
3. `Tool=1 / Memory=0 / Knowledge=0`的Context Owner语义；
4. Context永久失败后的Failure/Run termination状态；当前只能fail-closed + Inspect原Attempt，不能开启下一Turn；
5. Sandbox `workspace.read`窄Port、Coding Execution actual-point和production host plane；#23已加固Unix socket回归测试；
6. next Model predispatch全路径no-bypass、system fixture与production root；
7. Console/TS ViewModel/Event，直到Runtime/Review/Timeline/Sandbox正文分别冻结。

## 验证

```text
go test ./contract
go test ./tests -run TestTurnContinuationBlackBox -count=1
go test -race ./contract ./tests -run TurnContinuation -count=1
go vet ./contract ./ports ./tests
git diff --check
```

基线已包含#24的Activation conformance修复；`origin/main@13f63712`上的Application full ordinary/race/vet、Context full ordinary/vet、Harness相关ordinary/race/vet均已通过。

这些结果只验收continuation公共合同与CAS/recovery反例，不声称完整真实Loop已接线。
