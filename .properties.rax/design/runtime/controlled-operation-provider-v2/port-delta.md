# Controlled Operation Provider V2 Port Delta

## 1. Runtime公共只读/治理面

第四轮联合Review `YES`后已落地文件：

```text
ExecutionRuntime/runtime/ports/controlled_operation_provider_v2.go
```

只增加：

| 类型 | 用途 | 可见性/Owner |
|---|---|---|
| `ControlledOperationProviderRouteDeclarationRefV2` | Harness唯一canonical pre-binding事实的中立坐标 | Go type由Runtime `ports`唯一拥有；Harness拥有事实语义 |
| `ControlledOperationProviderRouteConformanceRefV2` | Harness post-binding conformance事实的中立坐标 | Go type由Runtime `ports`唯一拥有；Harness拥有事实语义 |
| `ControlledOperationProviderRouteCurrentRefV2` | `{CurrentID,Revision,DeclarationRef,ConformanceRef,MatrixDigest,Watermark,Digest}` | Digest排除自身；exact绑定current身份 |
| `ControlledOperationProviderRouteCurrentProjectionV2` | Generation/Handoff/BindingSet/active-route scan/七个Binding current闭包 | Runtime additive只读投影；不含Runner |
| `ControlledOperationProviderRouteCurrentReaderV2` | 下方真实Go签名 | Runtime `ports`唯一接口；Harness Adapter实现 |
| `ControlledOperationPreparedSemanticSnapshotV2` | Request稳定Prepared语义 | 不含fresh Checked time |
| `ControlledOperationPreparedCurrentProjectionV2` | 完整Prepared current proof | 含Delegation/Prepared/Persisted Enforcement |
| `ControlledOperationPreparedCurrentReaderV2` | fresh Prepared current | 复用既有Prepared Owner |
| `ControlledOperationProviderRequestV2` | Tool Adapter提交的exact request | Tool Adapter专用调用面 |
| `ControlledOperationProviderEntryRefV2` | derived stable Entry历史坐标 | Runtime Owner生成 |
| `ControlledOperationProviderResultV2` | entered/unknown/observed/rejected_no_effect结果 | 不含Authorization/Outcome |
| `ControlledOperationProviderPortV2` | Tool Adapter持有的Governance Port | Enter + Inspect，不暴露FactPort |
| `ControlledProviderInspectPortV2` | 按Entry原Prepared/Attempt只读Inspect | Gateway注入，caller不可换坐标 |

Application继续只依赖既有`SingleCallToolActionPortV1`，不注入V2 Governance或FactPort。

Runtime/Harness边界的五个V2新增中立Route公共类型唯一位于`github.com/Proview-China/rax/ExecutionRuntime/runtime/ports`：DeclarationRef、ConformanceRef、CurrentRef、CurrentProjection、CurrentReader；另复用既有MatrixKeyV3。Harness不重定义任何struct/interface/alias，只实现Reader。前两个Ref固定为：

```go
type ControlledOperationProviderRouteDeclarationRefV2 struct {
	RouteID              string        `json:"route_id"`
	Revision             core.Revision `json:"revision"`
	PublisherComponentID string        `json:"publisher_component_id"`
	DeclarationDigest    core.Digest   `json:"declaration_digest"`
}

type ControlledOperationProviderRouteConformanceRefV2 struct {
	ConformanceID     string                                            `json:"conformance_id"`
	Revision          core.Revision                                     `json:"revision"`
	DeclarationRef    ControlledOperationProviderRouteDeclarationRefV2 `json:"declaration_ref"`
	ConformanceDigest core.Digest                                       `json:"conformance_digest"`
}

type ControlledOperationProviderRouteCurrentRefV2 struct {
	CurrentID      string                                            `json:"current_id"`
	Revision       core.Revision                                     `json:"revision"`
	DeclarationRef ControlledOperationProviderRouteDeclarationRefV2 `json:"declaration_ref"`
	ConformanceRef ControlledOperationProviderRouteConformanceRefV2 `json:"conformance_ref"`
	MatrixDigest   core.Digest                                       `json:"matrix_digest"`
	Watermark      core.Digest                                       `json:"watermark"`
	Digest         core.Digest                                       `json:"digest"`
}
```

Projection字段逐字冻结为`ContractVersion string`、上述三个exact Ref/Generation、扁平Handoff ID/revision/digest、BindingSet五水位、扁平ActiveRoute ID/revision/digest、七个`ProviderBindingRefV2`、Checked/Expires与ProjectionDigest；所有字段使用snake_case JSON tag，完整Go struct以[合同](./contracts.md)为唯一实施清单。Generation只复用`GenerationArtifactRefV1`，不得新增`GenerationRef`；Handoff/ActiveRoute不得新增未定义Ref类型。

版本/对象类型由外层canonical domain/type discriminator固定；`ContractVersion`固定`ControlledOperationProviderRouteCurrentContractVersionV2 = "2.0.0"`；禁止自由map。`ConformanceID = Derive(RouteID, GenerationRef, BindingSetID)`，Revision由Harness Conformance Fact Owner单调CAS；完整Generation/Handoff只在事实和Projection中。Reader真实接口为：

```go
type ControlledOperationProviderRouteCurrentReaderV2 interface {
	InspectCurrentControlledOperationProviderRouteV2(
		context.Context,
		ControlledOperationProviderRouteCurrentRefV2,
		OperationScopeEvidenceApplicabilityMatrixKeyV3,
	) (ControlledOperationProviderRouteCurrentProjectionV2, error)
}
```

`error`仅使用现有`core.DomainError`：Invalid=`core.ErrorInvalidArgument`、NotFound=`core.ErrorNotFound`、Conflict=`core.ErrorConflict`、Expired=`core.ErrorPreconditionFailed`、Unavailable=`core.ErrorUnavailable`、Unknown=`core.ErrorIndeterminate`。reason复用既有`ReasonBindingExpired/ReasonBindingDrift/ReasonClockRegression`等；实现使用`core.NewError/HasCategory/HasReason`，不新增平行错误体系。

## 2. Runtime内部唯一面

已落地内部文件：

```text
ExecutionRuntime/runtime/control/controlled_operation_provider_v2.go
ExecutionRuntime/runtime/kernel/controlled_operation_provider_gateway_v2.go
ExecutionRuntime/runtime/fakes/controlled_operation_provider_store_v2.go
```

内部类型：

- `ControlledOperationProviderEntryFactPortV2`；
- `CreateEnteredResultV2{Fact, created|existing, opaqueClaim}`；
- 非持久、不可复制的opaque claim；
- 非公共、同call-stack Authorization；
- `ControlledOperationProviderRunnerV2`；
- raw Provider seam，仅Runner持有。

Authorization、opaque claim与raw Provider不得进入`ports`公共Result或Tool/Application对象图。Runner接口不能被普通Application或Tool Adapter注入。

## 3. exact Reader复用

| 事实 | 复用/已落地Reader | 禁止 |
|---|---|---|
| Runtime Effect/Intent | 既有Operation Effect current | 第二Effect Store |
| Harness Route | canonical DeclarationRef + ConformanceRef + Runtime RouteCurrent Projection | Runtime第二Declaration或只信静态Declaration |
| Generation/Handoff/BindingSet | RouteCurrent内exact refs + 各Owner current | 只信Conformance内嵌水位 |
| 七个公开Binding | `ProviderBindingCurrentnessPortV2` | 漏ToolAdapter/Gateway/ProviderTransport/PreparedReader/BoundaryReader/ProviderInspect/Provider任一Binding |
| V1/V2 active route | Harness active-route scan current proof | 静态声明互斥或Runtime自行猜测 |
| Prepared | 既有Prepared Governance current经窄Projection | 丢失Persisted Enforcement或信caller Checked time |
| Evidence Policy | exact V3 Policy current Reader | 与Applicability Policy合并 |
| Applicability Policy | exact V3 Applicability Policy current Reader | 从request自由Profile |
| execute Enforcement | 既有4.1 current Reader | V2 Enforcement sidecar |
| execute Handoff/Qualification/Scope | 既有Evidence V3 current/read-only闭包 | 只比Handoff ref |
| Tool Boundary | `OperationProviderBoundaryCurrentReaderV1` | Runtime写Tool Watermark |
| Provider Observation | `ControlledProviderInspectPortV2` | raw return升级Observation |

## 4. Harness Route接线依赖

Harness canonical Declaration/Conformance与Route Current发布语义保持唯一；Runtime只拥有跨包nominal Go类型。Harness后续Adapter只需：

- 把exact DeclarationRef、ConformanceRef、Generation/Handoff、BindingSet、active-route proof与Tool Adapter/Gateway/Provider Transport/Prepared Reader/Boundary Reader/Provider Inspect/Provider bindings投影到Runtime `RouteCurrentProjectionV2`；
- Harness Adapter只通过已冻结真实Go方法接收exact CurrentRef与typed Matrix，返回typed Projection并逐字段匹配两者；同CurrentID任一ref字段漂移为Conflict，只有ID不存在为NotFound；
- `CurrentID = Derive(RouteID, MatrixDigest)`；Route Current Owner单调CAS Revision，并把Generation/Handoff/BindingSet/ActiveRoute/七Binding/WiringInventory current摘要封入Watermark；
- post-binding Conformance成功后才sealed publish；同内容重复publish幂等，lost reply只Inspect，changed-content或CAS并发Conflict；旧Ref/revoke/expiry/unavailable Fail Closed；
- Tool Adapter与Runtime Gateway只接收composition注入的exact CurrentRef，不提供自由discovery；
- Ref Digest与ProjectionDigest使用两个无环canonical；Projection Validate逐字段闭合Ref的Declaration/Conformance/Matrix/Watermark；
- 不公开、不枚举也不绑定kernel内部Runner；Runtime Runner只持exact Provider Transport；
- 每次Reader调用复读post-binding Conformance与底层current水位；
- 缺Provider Inspect Binding、stale Conformance/Generation/Handoff/Binding时Fail Closed；
- active-route scan不能证明V2唯一active或发现V1 active时Fail Closed；
- 七个Binding分别current并进入TTL最小值；ProviderTransport是调用通道，Provider是Prepared/Request绑定的actual authority，不能合并；
- Declaration分别包含ProviderTransport与closed role `provider`的nominal endpoint/candidate；post-binding分别解析为ProviderTransportBinding和ProviderBinding；no-bypass扫描二者aliases与真实注入，raw句柄均不公开；
- 不新增`RouteBindings[]ObjectRef`；
- 不新增`AssemblyInputV1`字段或改变digest算法。Declaration required extension经Component Manifest digest间接受现有AssemblyInput摘要约束；
- 不建立production root；fixture显式注入Reader。

## 5. Tool Adapter Delta

Tool后续只需：

1. Application Adapter实现`SingleCallToolActionPortV1`既有入口，并在内部持Runtime Governance Port；
2. 提交含Boundary/Prepared stable proof的V2 Request；
3. 不取得Entry FactPort、Authorization、opaque claim或raw Provider；
4. Tool Provider transport由宿主装配给Runtime内部Runner，不暴露给Application Adapter；
5. unknown时只通过Gateway Inspect，不能自行重调raw Provider；
6. Observation后才进入Evidence Consumption、Tool DomainResult与Settlement既有Owner链。

## 6. 已完成实施路径

Runtime按以下顺序完成纵向切面：

1. `ports/controlled_operation_provider_v2.go`：typed route、Prepared stable/current、Request/Result/Inspect/Governance contracts；
2. `control/controlled_operation_provider_v2.go`：Entry Fact、derived ID、transition、create result；
3. `kernel/controlled_operation_provider_gateway_v2.go`：caller/route/current/TTL、opaque claim、internal Runner、Inspect恢复；
4. `fakes/controlled_operation_provider_store_v2.go`：同锁create/CAS/故障注入；
5. `conformance/controlled_operation_provider_v2.go`：public-only Tool Adapter/Governance conformance，raw Provider bypass负例；
6. `tests/{ports,control,fakes,kernel}/controlled_operation_provider_v2_test.go`。

不得修改V1、Evidence V3、Dispatch V4.0、Enforcement 4.1、Settlement V4、AssemblyInput V1、Application、Tool或Harness代码。
