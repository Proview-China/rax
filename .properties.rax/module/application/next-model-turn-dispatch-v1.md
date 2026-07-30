# Application Next Model Turn Dispatch V1 模块候选

## 作用

该模块把existing Eligibility的stable DerivedDispatchRef封存为可重启恢复、无执行权的Harness `attempt_bound` sidecar。

## 组成

- `application/contract/next_model_turn_dispatch_v1.go`：Eligibility-only request、Inspect request与sidecar current；
- `application/ports/next_model_turn_dispatch_v1.go`：StartOrInspect与Inspect；
- `application/conformance/next_model_turn_dispatch_v1.go`：import boundary、replay、Inspect与64并发；
- `harness/applicationadapter/next_model_turn_dispatch_binding_v1.go`：Continuation S1/S2、fresh clock、最小TTL与bounded mutation context；
- `harness/applicationadapter/sqlite_next_model_turn_dispatch_v1.go`：append-only history/current、fresh-only schema migration、exact物理闭集验真与lost-reply恢复；
- 对应Application/Harness测试。

测试会从实际production source自动提取imports并执行Owner allowlist，不以手工imports或默认零字段冒充Model/Runtime guard/Provider/Harness dispatch零调用证据；空库64 mixed-payload首创race验证仅产生一个durable winner/current。

## 输入与输出

输入只包含existing Eligibility request/projection、RequestedNotAfter与canonical RequestDigest。Eligibility内部已绑定Continuation、ActiveContext、Run/Session/TargetTurn和future Runtime actual-point exact request coordinate。

输出只包含Application DerivedDispatchRef、RequestDigest、`attempt_bound`、checked upper bound、排他NotAfter与canonical fact digest。

DerivedDispatch stable ID只由Eligibility request digest按公开算法派生；Continuation Attempt与全部ref digest必须结构完整。SQLite打开不会修复partial/weak对象，history/current必须逐字段相等，任何非`attempt_bound`事实均拒绝。

## 明确不存在的内容

Application public contract与SQLite均不包含：

- Model Prepared/current/InvocationMaterial/Authorization/SourceLineage；
- Context/Tool Owner或Kind literal；
- RouteCall、Tool digest、Provider ordinal；
- Harness Envelope、Model AttemptRef、Harness DispatchRef或outcome；
- guard projection、Provider结果或Console合同。

## 当前限制

SQLite是单节点owner-local sidecar，不声明HA或production SLA。未来Harness V2必须显式关联自己的exact Model envelope；当前未合并V2、真实Model outcome、Runtime guard/Provider同栈no-bypass与production root均未完成，因此Application production dispatch继续 HARD-BLOCKED。
