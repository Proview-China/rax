# Module: Governed Model Turn V3 Actual Boundary

## 作用

该模块是Model Invoker中V3 pre-provider facts到routegateway物理调用的窄接线。它复用公开Owner ports，不复制Context、Tool、Runtime或Harness语义。

## Public surface

- `InspectCurrentInvocationMaterialAuthorizationV2`
- `InspectCurrentInvocationMaterialAuthorizationClosureV3`
- `ValidateModelProviderActualPointRequestDraftV3`
- `GovernedModelTurnProviderBoundaryTurnAttemptReaderV3`
- `routegateway.GovernedModelTurnActualBoundaryDependenciesV3`
- `routegateway.GovernedModelTurnActualBoundaryCommandV3`
- `routegateway.GovernedModelTurnActualBoundaryResultV3`
- `routegateway.WithGovernedModelTurnActualBoundaryV3`
- `routegateway.Gateway.InvokeGovernedModelTurnActualBoundaryV3`

## Current status

Owner-local pre-invoke candidate。它以完整Context/Tool paired projection、完整ACK snapshot和已验证Turn body执行S1/S2；在adapter prepare后、Boundary写入前确定性Fail Closed。无credential exact current、实际Provider request canonical闭包与持久Provider Observation，Provider调用固定为0，production仍为NO-GO。
