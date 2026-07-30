# Model Provider Actual-Point Guard V1

状态：Owner-local implementation，等待主理人独立审计；production NO-GO。

## 目标

Runtime 在 Model Provider 物理调用紧前提供只读 current guard。它不调用 Provider、不写 Model/Harness 事实、不自动授权，也不把 Go `context.Context` 取消解释为 Runtime Run cancel。

首切面仅接受：

- Run-scoped Model Turn；
- EffectKind `praxis.harness/model-turn`；
- Provider capability `praxis.model/invoke`；
- V4 Permit 状态 `begun`；
- instance Fence，精确绑定 Instance、Sandbox Lease/epoch、ExecutionScope。

## 公共合同

- `ports.ModelProviderBoundaryCurrentRefV1`
- `ports.ModelProviderBoundaryCurrentProjectionV1`
- `ports.ModelProviderBoundaryCurrentReaderV1`
- `ports.InspectCurrentModelProviderActualPointRequestV1`
- `ports.ModelProviderActualPointCurrentProjectionV1`
- `ports.ModelProviderActualPointGuardV1`

返回值没有 `Allowed` 布尔字段。成功返回的 sealed projection 仅代表检查时点 current；Provider 调用和失败恢复仍属于后续 Model/Harness Adapter。

Request 与 projection 均绑定 exact `Verifier`，其语义复用 V4 `OperationDispatchPermitV3.EnforcementPoint`。公开 `Projection.ValidateAgainst(request, now)` 会重算 request digest，并逐字段校验 Operation、Effect、Attempt、Permit、Admission、Review、Fence、Model boundary 与 Verifier。

## 读取顺序

1. Model boundary current；
2. Run lifecycle；
3. Runtime cancel/fence/revoke desired/current；
4. Effect `dispatch_intent`；
5. V4 Permit/Auth/Admission/V3 governance current；
6. exact instance Fence；
7. S2 按相同顺序复读权威水位；
8. fresh clock 计算所有 TTL 的最小 `NotAfter` 并 seal。

S1/S2 比较权威 ref、revision、digest、state、TTL；合法变化的 `CheckedUnixNano` 不作为事实漂移。每份 current projection 的 Checked 时点不得来自未来。

## Unknown 与取消

- 入口、S1/S2 之间和 seal 前检查 `ctx.Err()`；取消返回零 projection。
- `context.Context` 取消不生成 Runtime cancel 事实。
- guard 后取消、调用回包丢失或结果不确定时，只能 Inspect Model 原 Attempt；不得重派。

## 当前 NO-GO

Runtime已提供owner-local `ModelDispatchControlCurrentReaderV1` adapter，从Run、Desired State与唯一LastCommand双读派生短TTL current projection；缺失、Unavailable、Indeterminate一律fail closed。当前仍没有生产SQLite Run/Command Owner或composition root，因此该adapter不构成production current实现。

Model/Harness/Provider 尚未接入本 guard，conformance 只核 Owner-local wiring metadata，`ProductionEligible=false`。64 并发 Provider boundary winner、所有 direct/stream/continuation/realtime/raw 路径的真实 no-bypass 仍需跨 Owner Adapter 黑盒证明。

`ModelProviderBoundaryCurrentProjectionV1` 对 Prepared/current/ACK 坐标的证明，目前仅来自 Model Owner reader 返回的 exact ref 与 canonical digest。Runtime 尚未接入真实 Model Adapter，也尚未证明所有 Provider 路径都经过该 reader；因此这些字段不能单独形成 production no-bypass 证明。
