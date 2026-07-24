# Runtime Controlled Operation Provider V2设计

## 1. 状态与目标

- 状态：第四轮联合Review与第三轮独立代码审计均为`YES`；Runtime V2纵向切面已实现并冻结；
- 版本：`praxis.runtime/controlled-operation-provider/v2`，SemVer `2.0.0`；
- 目标：缩窄、线性化并可审计化Tool检查与actual Provider不可逆execute admission之间的TOCTOU；不宣称消除跨Owner撤销窗口，不宣称Runtime Fact与外部Provider副作用分布式原子或物理exactly-once；
- 兼容：V1保持fixture-only，字段、Validate、canonical和digest不变。V1与V2在Assembly/production composition中互斥，V1不得通过sidecar升权。

本资产记录已落地的Runtime V2 Delta；本次实现未改Application、Tool、Harness或production root。

## 2. 调用与Owner边界

```text
Application Coordinator
-> existing SingleCallToolActionPortV1
-> Tool Application Adapter
-> ControlledOperationProviderPortV2 (Runtime Governance)
-> Runtime Gateway internal ControlledOperationProviderRunnerV2
-> raw Provider
```

- Application不直接调用V2 Governance Port；
- Tool Application Adapter提交V2 Request并持Governance Port，但Application与Tool均不得取得Entry FactPort；
- raw Provider只能由Runtime内部Runner持有，不能注入Application、Tool协调层或公共Result；
- Runtime不写Tool Boundary Watermark，不Consume Evidence，不创建Tool DomainResult或Operation Settlement；
- Runner只返回raw admission receipt/unknown给Gateway。raw return不等于`ProviderAttemptObservationV2`，Runtime不得据此伪造Observation。

## 3. Harness canonical Route与Runtime current投影

Harness Assembly是pre-binding `ControlledOperationProviderRouteDeclarationV2`、post-binding Conformance与Route Current事实的唯一语义Owner。为避免Go import cycle，所有跨Owner中立nominal DTO/Ref/Projection/Reader都只在`github.com/Proview-China/rax/ExecutionRuntime/runtime/ports`定义；Harness Assembly Adapter导入并实现这些`ports`合同，不得重定义同名Go struct。即：type Owner是Runtime ports，fact semantic Owner与发布Owner仍是Harness Assembly；Runtime不得定义Harness Declaration/Conformance事实struct、canonical或digest。

Runtime/Harness边界由Runtime `ports`唯一拥有五个V2新增中立Route公共类型，并复用一个既有Matrix类型：

```text
ControlledOperationProviderRouteDeclarationRefV2
ControlledOperationProviderRouteConformanceRefV2
ControlledOperationProviderRouteCurrentRefV2
ControlledOperationProviderRouteCurrentProjectionV2
ControlledOperationProviderRouteCurrentReaderV2
OperationScopeEvidenceApplicabilityMatrixKeyV3   // existing
```

Harness不得复制、alias、包装或重新seal上述六个类型。前两个Ref的真实Go形状固定为：

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
```

二者的`ContractVersion`与`ObjectKind`由所属canonical domain/type discriminator固定，不增加字段或自由`map`。`Validate`要求string/digest非空、Revision非零且嵌套Ref完整；same ID任一其他字段漂移均Conflict。Declaration Fact产出DeclarationRef；Conformance Fact产出ConformanceRef。`ConformanceID = Derive(RouteID, GenerationRef, BindingSetID)`，Revision由Harness Conformance Fact Owner单调CAS；Generation/Handoff全量留在Conformance Fact与CurrentProjection，不重复塞入ConformanceRef。

post-binding current Projection至少精确绑定：

- Harness DeclarationRef与`ControlledOperationProviderRouteConformanceV2` exact Ref；
- Generation与Assembly Handoff；
- BindingSet ID/revision/digest/semantic digest/currentness digest；
- Tool Adapter、Runtime Gateway、Provider Transport、Prepared Reader、Boundary Reader、Provider Inspect及actual Provider Binding；
- Harness active-route scan current proof，证明V2为唯一active route且V1未激活；
- Checked/Expires与self digest。

公共Go接口冻结为真实签名：

```go
type ControlledOperationProviderRouteCurrentReaderV2 interface {
	InspectCurrentControlledOperationProviderRouteV2(
		context.Context,
		ControlledOperationProviderRouteCurrentRefV2,
		OperationScopeEvidenceApplicabilityMatrixKeyV3,
	) (ControlledOperationProviderRouteCurrentProjectionV2, error)
}
```

expected Matrix固定为`run + praxis.tool/execute + praxis.tool/single-call-action-v1`，Reader返回的Declaration/Conformance/Projection必须与该Matrix exact一致。Harness Assembly Adapter只实现此接口，不重定义Ref、Projection、Reader或Matrix类型。

Current与Projection真实形状固定为：

```go
type ControlledOperationProviderRouteCurrentRefV2 struct {
	CurrentID      string                                            `json:"current_id"`
	Revision       core.Revision                                     `json:"revision"`
	DeclarationRef ControlledOperationProviderRouteDeclarationRefV2 `json:"declaration_ref"`
	ConformanceRef ControlledOperationProviderRouteConformanceRefV2 `json:"conformance_ref"`
	MatrixDigest   core.Digest                                       `json:"matrix_digest"`
	Watermark      core.Digest                                       `json:"watermark"`
	Digest         core.Digest                                       `json:"digest"`
}

type ControlledOperationProviderRouteCurrentProjectionV2 struct {
	ContractVersion             string                                            `json:"contract_version"`
	Ref                         ControlledOperationProviderRouteCurrentRefV2     `json:"ref"`
	DeclarationRef              ControlledOperationProviderRouteDeclarationRefV2 `json:"declaration_ref"`
	ConformanceRef              ControlledOperationProviderRouteConformanceRefV2 `json:"conformance_ref"`
	Generation                  GenerationArtifactRefV1                           `json:"generation"`
	HandoffID                   string                                            `json:"handoff_id"`
	HandoffRevision             core.Revision                                     `json:"handoff_revision"`
	HandoffDigest               core.Digest                                       `json:"handoff_digest"`
	BindingSetID                string                                            `json:"binding_set_id"`
	BindingSetRevision          core.Revision                                     `json:"binding_set_revision"`
	BindingSetDigest            core.Digest                                       `json:"binding_set_digest"`
	BindingSetSemanticDigest    core.Digest                                       `json:"binding_set_semantic_digest"`
	BindingSetCurrentnessDigest core.Digest                                       `json:"binding_set_currentness_digest"`
	ActiveRouteID               string                                            `json:"active_route_id"`
	ActiveRouteRevision         core.Revision                                     `json:"active_route_revision"`
	ActiveRouteDigest           core.Digest                                       `json:"active_route_digest"`
	ToolAdapterBinding          ProviderBindingRefV2                              `json:"tool_adapter_binding"`
	GatewayBinding              ProviderBindingRefV2                              `json:"gateway_binding"`
	ProviderTransportBinding    ProviderBindingRefV2                              `json:"provider_transport_binding"`
	PreparedReaderBinding       ProviderBindingRefV2                              `json:"prepared_reader_binding"`
	BoundaryReaderBinding       ProviderBindingRefV2                              `json:"boundary_reader_binding"`
	ProviderInspectBinding      ProviderBindingRefV2                              `json:"provider_inspect_binding"`
	ProviderBinding             ProviderBindingRefV2                              `json:"provider_binding"`
	CheckedUnixNano             int64                                             `json:"checked_unix_nano"`
	ExpiresUnixNano             int64                                             `json:"expires_unix_nano"`
	ProjectionDigest            core.Digest                                       `json:"projection_digest"`
}
```

`ContractVersion`固定为`ControlledOperationProviderRouteCurrentContractVersionV2 = "2.0.0"`。Ref Digest覆盖清空自身Digest后的其余字段；ProjectionDigest覆盖清空自身后的全部字段，二者禁止循环。Generation复用`GenerationArtifactRefV1`，七个Binding复用`ProviderBindingRefV2`；Handoff与ActiveRoute只用扁平exact ID/revision/digest。全部字段严格Validate；同HandoffID/ActiveRouteID换revision或digest、TTL crossing或field-role Capability type-pun均Fail Closed。

Reader以exact Ref为key；同CurrentID换Revision或Digest等任一字段是Conflict，只有CurrentID不存在才是NotFound。

Route Current由Harness Assembly Route Current Store/Adapter权威发布：`CurrentID = Derive(RouteID, MatrixDigest)`，Revision由该Owner单调CAS；Watermark是Generation、Handoff、BindingSet、ActiveRoute、七个Binding及WiringInventory current水位的canonical digest。只有post-binding Conformance成功后才能seal并publish exact Projection/Ref。Tool Adapter和Runtime Gateway只接收composition注入的exact CurrentRef，不提供自由discovery。重复同内容publish幂等；lost publish reply只按exact Ref Inspect；同ID换内容/CAS竞争Conflict；旧Ref、revoke、expire或Owner unavailable均Fail Closed。CurrentRef是post-binding产物，不改变`AssemblyInputV1`字段或digest。

Reader返回`error`但不引入平行错误体系。closed语义映射现有`core.DomainError`：invalid→`core.ErrorInvalidArgument`；absent CurrentID→`core.ErrorNotFound`；same-ID drift、type-pun或current mismatch→`core.ErrorConflict`；expired、TTL crossing或stale binding→`core.ErrorPreconditionFailed`并使用既有`ReasonBindingExpired/ReasonBindingDrift/ReasonClockRegression`等reason；reader unavailable→`core.ErrorUnavailable`；current/outcome无法判定→`core.ErrorIndeterminate`。实现复用`core.NewError/HasCategory/HasReason`，不得新增`ErrExpired`、`ErrUnknown`或其他平行类别。

Request携exact RouteCurrent Ref、DeclarationRef与ConformanceRef。Gatewayfresh读取Projection并与Request三ref exact比较，再独立复读Generation/Handoff及七个Binding current；不能只信Declaration或Conformance内嵌字段。Conformance、Generation、Handoff、BindingSet或任一Binding漂移/过期/缺失都零Entry、零Provider。

Harness Declaration必须独立声明closed role `provider`的nominal Provider endpoint/candidate，并与`ProviderTransport`分开。post-binding分别解析七个Binding：ToolAdapter、Gateway、ProviderTransport、PreparedReader、BoundaryReader、ProviderInspect、Provider。`ProviderTransportBinding`是Tool侧受控transport adapter binding，也是Runner唯一可调用的通道；`ProviderBinding`是transport背后的actual data-plane Provider权威binding，并须与Prepared/Request exact。二者语义不同，必须同时current且逐字段闭合，不能合并或相互替代。二者只暴露typed ref/binding，raw句柄不进入Application或Tool协调层；no-bypass图扫描必须同时覆盖Transport和actual Provider的alias及真实注入。Provider role可声明，但不公开kernel Runner。

Declaration作为required registered extension进入`ComponentManifestV2.Extensions`，其digest经Component Manifest digest间接受现有`AssemblyInputV1` digest约束。Runtime不新增`AssemblyInputV1`字段、不改变其digest算法，也不读取`RouteBindings[]ObjectRef`补Route。

## 4. stable Entry identity与唯一调用权

Entry ID由Runtime确定性派生，caller不提交自由Entry ID：

```text
EntryStableKey = canonical(
  OperationDigest,
  EffectID + stable Attempt coordinates,
  Prepared.ID + Prepared.Digest
)
```

Boundary revision、current Checked time、Policy revision和`UnifiedNotAfter`等易变水位不得进入唯一execution identity；它们进入Entry Fact content并在同stable key下changed-content Conflict。caller换Entry ID、Attempt别名或Prepared别名二次调用必须零Provider。

Entry Owner create结果冻结为内部：

```text
CreateEnteredResultV2 {
  Fact
  Disposition   created | existing
  opaqueClaim   // only created; kernel-internal, non-copyable, non-persistable
}
```

只有真正赢得`absent -> entered`的同一调用得到`created + opaqueClaim`，并在同一call stack内构造内部Authorization。`existing`、duplicate、detached Inspect或lost-create-reply恢复都没有claim，只能Inspect。64并发同stable key最多一个claim、一个Runner调用。

## 5. exact current闭包

Request由Tool Adapter提交，携稳定语义与exact refs：

- Operation/Scope、Effect ID/revision/kind/Intent digest、Attempt；
- Provider Binding；
- Prepared exact ref及稳定语义快照；
- Tool Boundary；
- execute Enforcement 4.1与execute Evidence Handoff；
- exact Evidence Policy ref；
- exact Applicability Policy ref；
- Harness DeclarationRef、ConformanceRef与Runtime RouteCurrent Ref；
- caller deadline上界。

Gateway在任何Entry写前fresh复读：

1. Runtime Effect/Intent current；
2. RouteCurrent Projection，以及其Declaration/Conformance/Generation/Handoff/BindingSet闭包；
3. Tool Adapter、Gateway、Provider Transport、Prepared Reader、Boundary Reader、Provider Inspect与actual Provider Binding current；
4. Harness active-route scan current proof，证明V1/V2没有同时激活；
5. Prepared current完整投影：`Delegation + Prepared + PersistedOperationEnforcementRefV3`；
6. execute Enforcement 4.1；
7. execute Handoff及其Qualification、full Scope；
8. Evidence Policy current；
9. Applicability Policy current；
10. Tool Boundary current。

`ProviderBinding.Capability == EffectKind`，且EffectKind、Provider、Permit、Prepared、Payload与Runtime Effect/Intent current exact。Profile只来自Applicability Policy；Evidence Policy与Applicability Policy不得合并或互相替代。Handoff必须反查exact Qualification与Scope，Scope再闭合两份Policy与同Operation/Attempt/phase。

## 6. Prepared current语义

Prepared current projection必须覆盖live `PreparedExecutionGovernanceResultV2`全部权威语义：

- exact `ExecutionDelegationRefV2`；
- exact `PreparedProviderAttemptRefV2`；
- exact `PersistedOperationEnforcementRefV3`；
- Operation/Effect/Attempt/Permit/Provider/Payload逐字段关系；
- current checked/expires与projection digest。

Request不携fresh current projection的Checked时间，只携Prepared exact ref与稳定语义快照。Gateway把fresh Reader返回的完整Projection持久写入Entry。`CheckedUnixNano`仅表示该Reader完成本轮current验证的时间：不参与Entry ID，也不要求等于caller快照；必须非零、不回拨且早于Projection expiry。

## 7. Unified NotAfter与Provider admission

`UnifiedNotAfter`由Gatewayfresh计算并持久化，取以下最小值：

- Intent expiry；
- RouteCurrent Projection及七个Binding current expiry；
- Prepared current expiry；
- Evidence Policy与Applicability Policy expiry；
- Boundary、execute Enforcement、execute Handoff/Qualification expiry；
- caller deadline。

Runtime最后fresh clock必须严格早于该值。Runner在raw Provider方法入口传入内部Authorization，但不承诺“适配器方法零进入”。Provider必须以同一stable idempotency key原子执行：

```text
fresh NotAfter check + irreversible execute admission
```

Provider返回可验证admission receipt：能证明过期前未admit时为`rejected_no_effect`；不能证明是否admit时为`unknown`。raw receipt不等于Observation。Gateway只有经只读Inspect得到exact `ProviderAttemptObservationV2`后才能把Entry收为`observed`，否则收为`unknown`。

## 8. 状态与恢复

| Entry状态 | 含义 | 允许下一步 |
|---|---|---|
| absent | stable key尚无Entry | current全通过后create |
| entered | Entry已线性化，Provider可能被触达 | 同call stack持claim者调用Runner；其他路径只Inspect |
| unknown | 无法证明Provider结果 | 只以Entry内原Prepared/Attempt stable key Inspect |
| observed | Provider State Plane返回exact Observation | 只读返回；后续Owner链独立推进 |
| rejected_no_effect | Provider admission以同stable key可验证地证明未admit | 终态只读；无Observation且同stable key不可重新execute |

只允许`entered -> unknown | observed | rejected_no_effect`，以及`unknown -> observed | rejected_no_effect`；不得回到absent、不得换Prepared/Attempt Inspect、不得重调Provider。`rejected_no_effect`必须携可验证admission receipt；不能证明no-effect时只能`unknown`。

lost create reply时，即使detached Inspect看到exact entered，也不能恢复opaque claim，本次Provider调用数必须为0。lost Provider reply只调用只读`ControlledProviderInspectPortV2`；该Port从Entry读取原Prepared/Attempt stable key，不接受caller替换坐标。

## 9. Result与Authorization边界

公共`ControlledOperationProviderResultV2`只含Entry Ref、closed status/error、原Prepared/Attempt refs、可选admission receipt ref、可选exact Observation ref、inspection time与result digest：

- 不携Authorization或opaque claim；
- 不等于Outcome、DomainResult、Evidence Consumption或Settlement；
- raw Provider return不能填入Observation字段；
- `observed`必须有exact Observation且`error=none`；
- `entered`必须`error=inspection_required`；
- `unknown`必须是封闭unknown错误且不得携伪Observation。
- `rejected_no_effect`必须有可验证receipt、无Observation且`error=none`；它是Entry终态，同stable key不得重新execute。

Authorization是kernel内部同call-stack瞬时值，不进入公共Result、Tool/Application返回、Entry Fact、日志、sidecar或可重放序列化。Runner调用结束后即失效；重放、泄漏或跨Attempt使用均零Provider。

## 10. 不做事项

- 不修改V1、Evidence V3、Dispatch V4.0、Enforcement 4.1、Settlement V4或AssemblyInput V1 digest；
- 不增加Runtime Applicability/Boundary/Prepared第二Owner；
- 不让Application直接调用V2或持FactPort；
- 不让Runner推进Evidence、DomainResult、Settlement、Continuation或Turn；
- 不声明生产composition root、Backend、SLA或物理exactly-once。

## 11. 实现与验证记录

已落地代码位于`ExecutionRuntime/runtime/{ports,control,kernel,fakes,conformance,tests}`。最终实现保持Route公共边界为五个V2新增类型与既有MatrixKeyV3，未发布额外Route Reader；Entry按immutable request派生稳定ID，只有本次`absent -> entered`调用可获得kernel内部opaque claim。相同immutable request即使已由其他reconciler推进revision、current closure或终态，也只按原Entry Inspect，不恢复claim、不重调Provider；changed-content仍Conflict。

Prepared链保留双水位：Prepared与`PreparedSemanticSnapshot`绑定legacy V3 Permit revision/digest，Request的execute Attempt、Boundary与execute Enforcement绑定V4 Permit Fact current revision/digest。Entry Fact同时冻结Route七Binding与BindingSet digest/semantic digest，并证明所有fresh projection在`EnteredUnixNano`时有效；历史Fact过期后仍可验证其进入时真实性，但不重新授予执行资格。

Owner与中央实际通过：

```text
go test -count=1 ./tests/ports ./tests/fakes -run ControlledOperationProvider
go test -count=100 ./tests/ports ./tests/fakes -run ControlledOperationProvider
go test -race -count=20 ./tests/ports ./tests/fakes -run ControlledOperationProvider
go test -count=1 -shuffle=on ./...
go test -race -count=1 -shuffle=on ./...
go vet ./...
gofmt -l <本纵切相关Go文件>        # 无输出
git diff --check -- .              # PASS
```

第三轮独立审计最终结论：`YES`，P0/P1/P2均为0。reference store、fake transport与Conformance只证明合同和逻辑线性化，不声明生产持久后端、production composition root、availability、SLA或物理exactly-once。

## 12. 资产

- [合同](./contracts.md)
- [Port Delta](./port-delta.md)
- [测试矩阵](./test-matrix.md)
- [流程图](./controlled-operation-provider-v2.drawio)
- [实施计划](../../../plan/runtime/controlled-operation-provider-v2.md)
