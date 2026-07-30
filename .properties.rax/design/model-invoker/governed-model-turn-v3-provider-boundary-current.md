# Governed Model Turn V3 Provider Boundary Current

## 冻结目标

本切片只建立 Model Invoker Owner 在物理 Provider 调用前的不可变边界事实，以及面向 Runtime 的只读 current 投影。它不授权调用 Provider，也不拥有 Runtime guard、routegateway 或 Harness 的执行语义。

## Owner 与对象

- `GovernedModelTurnProviderBoundaryRefV3` 的稳定 ID 只由 original `GovernedModelTurnAttemptRefV3` 派生。
- `GovernedModelTurnProviderBoundaryFactV3` 完整封存 Turn exact Ref、Turn TTL、verified ACK/Dispatch Receipt、Runtime actual-point request、Provider binding 和 sealed Runtime boundary。
- `GovernedModelTurnProviderBoundaryPersistenceResultV3` 只报告本次本地 create-once 结果：`created`、`existing` 或 `recovered_unknown`。它不是 Permit、Authorization 或 Provider 调用权。
- SQLite 只保存 create-once history，不建立可回拨 current pointer。
- V5 ledger 已存在时，启动只能验证现存物理对象，禁止通过 `IF NOT EXISTS` 修复缺失表或索引；无 V5 ledger 时只有全部 V5 对象均不存在才允许 fresh/V4→V5 原子迁移，partial V5 必须 Fail Closed。
- `ModelProviderBoundaryCurrentAdapterV1` 只把 exact Model Fact 投影成既有 Runtime public contract；它没有 writer。

## 精确闭包

```text
GovernedModelTurnV3 winner
+ Prepared historical/current
+ exact ACK
+ verified Dispatch Receipt
+ Runtime actual-point request
+ expected Provider binding
→ immutable Model Provider Boundary Fact
→ read-only Runtime current projection
```

有效期严格取 Turn、Prepared Current、Material、ACK、Runtime request 的最小上界；`now == expires`、clock rollback、splice、Unknown commit 和 physical schema drift 全部 Fail Closed。

只有未来 routegateway 在同一调用栈内收到已知 `created`，才可以继续执行 S3 与 Runtime guard；`existing` 和 `recovered_unknown` 必须保持 Provider call count 为零。即使是 `created`，也仍需后续治理，不能单独授权调用。

## NO-GO

- 不暴露生产 `CrossBoundary` 或 `Dispatch` Port。
- 不调用 Provider、routegateway、Runtime guard、Harness、Context 或 Tool。
- 不调用 `prepareAt` / `invokePrepared`。
- 不创建 fake lowerer、fake Provider 或生产 authorizer。
- Context authoritative lowering 与 Harness exact closure 未闭合前，Provider call count 必须为零。
