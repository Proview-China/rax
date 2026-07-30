# AgentExecutionAvailabilityCurrentReaderV1 模块

## 代码入口

- `contract/start_package_selection_binding_v1.go`：Host-owned immutable
  Start→DeploymentV2→Selection/Closure association；
- `ports/start_package_selection_binding_v1.go`：复合 mutation、public exact reader
  与 Claim-bound lookup；
- `journal/start_package_selection_binding_v1.go`：same-lock reference store 与
  Unknown exact-Inspect admission；
- `storage/sqlite/start_package_selection_binding_v1.go`：单 transaction 三对象写入
  与 exact readers；
- `storage/sqlite/start_package_selection_schema_v1.go`：schema v8 物理证明；
- `owneradapter/agent_execution_availability_current_v1.go`：只读完整闭包、S1/S2、
  fresh clock、状态/TTL Fail Closed 与 Runtime projection。

## 使用边界

Availability 构造方只能提供只读接口和 exact Availability Ref；不能注入
BindingRef、DeploymentV2Ref 或 SelectionRef。Reader 从 SystemReady Fact 取得
ClaimRef，再经 Host-owned ForClaim lookup 定位唯一 Binding。

新复合 mutation 只供冻结后的 Host Start owner pipeline 使用。旧
`ClaimOrInspectHostStartV3` 仍只证明 Claim+InputV3，不能被解释为已有 Selection
association，也不能事后补写升级。

## 当前结论

owner-local/reference software slice；production composition NO-GO。剩余硬阻塞是
HostV3 production Start 尚未接新复合端口，Runtime Model actual-point guard 尚未
冻结 Host availability public input。本模块不改 Console、Provider、Model、Harness、
Runtime Authority 或既有 V1/V3 合同。
