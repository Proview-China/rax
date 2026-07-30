# Module: Governed Model Turn V3 Provider Boundary Current

## Current truth

该模块是 Model Invoker 的 owner-local pre-provider history 与 Runtime read adapter。它保存“边界事实已经由 Model Owner 精确封存”，不表示 Runtime 已授权，也不表示 Provider 已被调用。

## Public surface

- `GovernedModelTurnProviderBoundaryRefV3`
- `GovernedModelTurnProviderBoundaryFactV3`
- `GovernedModelTurnProviderBoundaryPersistenceResultV3`
- `GovernedModelTurnProviderBoundaryPersistenceDispositionV3`
- `GovernedModelTurnProviderBoundaryRepositoryV3`
- `BuildGovernedModelTurnProviderBoundaryFactV3`
- `EnsureOrInspectGovernedModelTurnProviderBoundaryFactV3`
- `runtimeadapter.ModelProviderBoundaryCurrentAdapterV1`

## Storage

- SQLite V5：`governed_model_turn_v3_provider_boundary_history`
- create-once、history-only、无 current table
- canonical JSON 与全部关键 exact coordinate 分列复算
- existing ledger 只验证、不修复；fresh migration 要求 V5-owned objects 为零

## Production status

Owner-local reference executable；Provider dispatch、Runtime actual-point guard、routegateway 与 Harness production composition 仍为 NO-GO。

`created / existing / recovered_unknown` 只描述本地持久化结果。只有未来同调用栈内的 `created` 可以进入后续 S3/guard；它本身仍不授权 Provider。
