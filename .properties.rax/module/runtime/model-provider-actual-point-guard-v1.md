# Runtime Model Provider Actual-Point Guard V1

该模块把 Model Owner 的 boundary current ref 与 Runtime 的 Run、control、Effect、V4 Permit/Review/Governance、Fence current 水位在物理 Provider 调用前重新闭合。

代码：

- `runtime/ports/model_provider_actual_point_guard_v1.go`
- `runtime/control/model_dispatch_control_current_v1.go`
- `runtime/kernel/model_provider_actual_point_guard_v1.go`
- `runtime/conformance/model_provider_actual_point_guard_v1.go`

边界：

- Runtime 不持有 Model Prepared/Boundary Fact；
- Runtime 不调用 Provider；
- Runtime 不把 Context cancel 升级为 Run cancel；
- Runtime 不签发 Permit、Review Authorization 或 Enforcement；
- Runtime cancel/current 生产实现与跨 Owner Adapter 尚缺，因此 production NO-GO。
- Boundary projection 的 Prepared/current/ACK 证明仅依赖 Model Owner reader/ref digest；真实 Model Adapter 与 no-bypass 尚未接入。
