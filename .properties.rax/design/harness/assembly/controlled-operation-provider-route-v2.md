# Controlled Operation Provider强类型Assembly Route V2设计

## 1. 状态与目标

- 状态：Runtime公共Route合同已完成第三轮独立审计并获联合`YES`；Harness Route V2第八独立短审已获`YES(P0/P1/P2=0)`，Route模块门解除；该结论不包含cross-module fixture、system G6A或生产能力；
- 合同：`praxis.harness/controlled-operation-provider-route/v2`，SemVer `2.0.0`；
- 目标：证明G6A真实Tool Provider只有一条可装配路线，raw Provider、通用Route引用、HookFace或其他Slot均不能旁路Runtime `ControlledOperationProviderPortV2` actual-point Gateway；
- 兼容：不新增`AssemblyInputV1`字段、不修改其canonical算法，也不修改`AssemblyManifestV1`、`AssemblyHandoffV1`既有字段/Validate/digest；Declaration payload进入required `ComponentManifestV2` extension，其digest经Manifest集合被现有`AssemblyInputV1` digest间接约束。不得把`RouteBindings []ObjectRefV1`静默升级为本合同。

本Delta只拥有Assembly Declaration/Conformance/Route Current事实语义、确定性合并、冲突诊断与post-binding发布。Runtime仍拥有Entry Journal/Governance及kernel-internal Runner；Tool仍拥有Boundary、Provider Transport与Provider Observation；actual Provider仍由其领域Owner拥有。Application仍只通过`SingleCallToolActionPortV1`编排。Runtime internal Runner不进入任何Harness公共对象、role enum、Binding或fixture注入面。

## 2. 唯一允许矩阵

| 维度 | 唯一值 |
|---|---|
| `OperationScopeKind` | `run` |
| `EffectKind` | `praxis.tool/execute` |
| `PolicyProfile` | `praxis.tool/single-call-action-v1` |
| cardinality | `one_active_binding` |
| bypass policy | `no_raw_provider_bypass` |

`N>1`、admin、custom、MCP server lifecycle、Context、Checkpoint及其他EffectKind全部Fail Closed；不得用默认值、前缀匹配或自定义扩展扩大首版矩阵。

## 3. 唯一canonical定义与三层协议

pre-binding编译时尚不存在本次Generation对应的Runtime `BindingSetV2`。把未来BindingSet、Handoff或Conformance强塞进声明会形成`Compile -> Binding -> Compile`循环。

`ControlledOperationProviderRouteDeclarationV2`是pre-binding Route语义的唯一canonical定义。Runtime、Tool、Application和composition root不得复制、扩表或重新seal另一份Declaration；它们只能持有exact Declaration Ref/Digest或消费下述current projection。

完整协议由三个不同权威等级的对象构成：

1. `ControlledOperationProviderRouteDeclarationV2`：pre-binding静态声明，随required registered extension进入`ComponentManifestV2`并被现有Manifest/Input摘要间接绑定；
2. `ControlledOperationProviderRouteConformanceV2`：post-binding只读报告，把Declaration精确关联到Generation、Handoff、BindingSet与`AssemblyBindingConformanceV1`；
3. Runtime-facing `ControlledOperationProviderRouteCurrentProjectionV2`：从前两者及live Binding currentness生成的短租约投影；Runtime Gateway只能通过`ControlledOperationProviderRouteCurrentReaderV2` fresh读取它。

Declaration不是Binding，Conformance不是Permit、Authorization、Entry Fact或生产启用资格。二者缺一，路线都不可执行。

## 4. Declaration公共形状

```text
ControlledOperationProviderRouteDeclarationV2 {
  ContractVersion
  RouteID
  Revision
  PublisherComponent
  Matrix { OperationScopeKind, EffectKind, PolicyProfile }
  ApplicationToolPort       ControlledOperationProviderRoutePortRefV2
  ToolAdapter               ControlledOperationProviderRouteEndpointV2
  RuntimeGovernancePort     ControlledOperationProviderRoutePortRefV2
  Gateway                   ControlledOperationProviderRouteEndpointV2
  ProviderTransport         ControlledOperationProviderRouteEndpointV2
  Provider                  ControlledOperationProviderRouteEndpointV2
  PreparedCurrentReader     ControlledOperationProviderRouteReaderRefV2
  BoundaryCurrentReader     ControlledOperationProviderRouteReaderRefV2
  ProviderInspectReader     ControlledOperationProviderRouteReaderRefV2
  ActiveBindingPolicy       one_active_binding
  BypassPolicy              no_raw_provider_bypass
  DeclarationDigest
}
```

所有Ref均为nominal类型，不能用一个通用`ObjectRefV1`互换：

- `ControlledOperationProviderRoutePortRefV2`：PortSpec ID/digest、Owner Capability、Request/Response Schema与Contract版本；
- `ControlledOperationProviderRouteEndpointV2`：Role、Component ID、Component Manifest digest、Artifact digest、Capability、Contract版本、Locality、exact ProviderBindingCandidate ID/digest；
- `ControlledOperationProviderRouteReaderRefV2`：Role、Component/Manifest/Artifact、Capability、PortSpec ID/digest、Request/Projection Schema与read-only标志；
- `ControlledOperationProviderRouteDeclarationRefV2`：exact `RouteID string / Revision core.Revision / PublisherComponentID string / DeclarationDigest core.Digest`，由Runtime ports唯一拥有、Harness Declaration Fact产出。
- `ControlledOperationProviderRouteConformanceRefV2`：exact `ConformanceID string / Revision core.Revision / DeclarationRef / ConformanceDigest core.Digest`，由Runtime ports唯一拥有、Harness Conformance Fact产出。

其中PortRef、Endpoint与ReaderRef只属于Harness Declaration/Conformance事实内部Schema，不跨Runtime/Harness边界；跨边界公共类型严格限定为第9节六项runtimeports类型。

角色是封闭枚举：`application_tool_port|tool_adapter|runtime_governance|runtime_gateway|provider_transport|provider|prepared_current_reader|boundary_current_reader|provider_inspect_reader`。同一Ref不得跨角色重封；Gateway、ToolAdapter、ProviderTransport与Provider必须分别绑定自己的Component/Manifest/Artifact/Capability/Candidate，不能只比较Component ID。`provider_inspect_reader`必须声明read-only/no-execute，不能与ProviderTransport、Provider或Boundary Reader互换。不得声明`runtime_runner`角色。

`ProviderTransport`与`Provider`是两项独立nominal Declaration：前者是Tool侧受控transport adapter endpoint/candidate，后者是transport背后的actual Provider endpoint/candidate。它们可以在同一组件中实现，但Ref、role、candidate和post-binding proof仍不得折叠或互换；两者都只声明身份/Binding expectation，不暴露raw调用句柄。

## 5. required registered extension

Declaration只能通过已注册required `GovernanceExtensionV2`进入装配：

```text
Key      = praxis.harness/controlled-operation-provider-route-v2
Required = true
Schema   = praxis.harness/controlled-operation-provider-route@2.0.0
Payload.ContentDigest = DeclarationDigest
```

规则：

1. Governance Catalog必须显式注册Key、Schema、Publisher Kind/Category/Capability/Locality/Conformance；Publisher Manifest必须是Host Control Plane上的canonical `2.x`版本，且Catalog digest、Manifest binding digest、完整Capability集合和extension digest全部进入verified compile result；未知Schema、optional声明、空Capability、`Inline='x'`伪payload或缺失注册一律Fail Closed；
2. Payload必须按V2严格解码和canonical复算，不能只信`ContentDigest`；
3. Route extension进入`ComponentManifestV2.Extensions`；Declaration payload digest进入Component Manifest digest，Manifest集合再被现有`AssemblyInputV1` digest约束。这里不新增`AssemblyInputV1`字段，也不改变其canonical算法；
4. 不读取`AssemblyInputV1.RouteBindings`来补字段、选择Provider Transport或降级；通用Route ref即使摘要正确也不能满足本合同；但V1 active-route记录仍必须进入第8.2节跨版本冲突扫描；
5. 现有`AssemblyHandoffV1.RequiredExtension`及Generation-Binding协议保持原义。本Route由独立V2 Conformance对象关联，不覆盖或冒充现有Generation governance extension。

## 6. 结构交叉校验

1. Application Tool Port精确指向Application公开`SingleCallToolActionPortV1`的PortSpec；它不携Tool Boundary、Entry Authorization、Provider Transport句柄或Runtime internal Runner；
2. ToolAdapter Endpoint精确实现Application Tool Port，并且是Runtime Governance Port的唯一非Runtime消费者；Application Coordinator不直接获得Governance Port；
3. Runtime Governance Port精确指向未来`ControlledOperationProviderPortV2`的Enter/Inspect合同；
4. Gateway Endpoint位于受信Host Control Plane并提供Runtime Governance Capability；
5. ProviderTransport Endpoint精确提供受控Tool transport能力，其ProviderBindingCandidate、Manifest、Artifact、Capability与Runtime V2 Request预期完全一致；
6. Provider Endpoint精确声明actual Provider及独立Candidate；post-binding `ProviderBinding`必须从该Candidate解析，并与Prepared/Request预期完全一致；不得从ProviderTransportBinding反推；
7. Prepared Reader、Boundary Reader与ProviderInspect Reader都是read-only Port，不能暴露Create/CAS/Commit/Execute；ProviderInspect只按原Prepared/Attempt读取Observation或unknown；
8. sealed wiring inventory必须完整证明`Application Tool Port → ToolAdapter → Runtime Governance Port → Gateway → ProviderTransport → actual Provider`五段链；Runtime kernel-internal Runner仍不进入Harness公共类型，只是Gateway内部执行点。Application Tool Port、Tool协调层、Slot、HookFace或PhaseContribution不得取得Transport或Provider raw句柄；
9. ProviderTransport与Provider Candidate可进入Runtime Handoff候选集合，但不得被其他可执行声明或实际注入边暴露；
10. Route声明不得携Secret、Credential值、Provider payload bytes、Entry Fact、Permit或Settlement。

## 7. canonical、merge与conflict

### 7.1 canonical

- canonical domain、ContractVersion和type discriminator固定；
- 禁止自由`map`、unknown field、重复JSON key、尾随文档、浮点时间和隐式默认；
- 名称使用严格namespaced值；Revision非零；Digest必须合法；
- nominal Ref按固定字段顺序编码；集合按`role + component + port/candidate ID`稳定排序并拒绝重复；nil slice归一为空集合；
- Matrix、两项封闭Policy、全部Port、ToolAdapter/Gateway/ProviderTransport/Provider Endpoint、三类Reader及Publisher均进入Declaration digest；self digest不参与自身计算；
- 首版不接受自由Annotation或optional extension。

### 7.2 merge

- merge key固定为`OperationScopeKind + EffectKind + PolicyProfile`；
- 同Publisher、Route ID、Revision、完整Declaration digest的重复输入只可幂等折叠一次；
- 不支持优先级、first-wins、last-wins、按Component ID猜测或部分字段拼接；
- required字段不能由另一Declaration补齐，每个Declaration必须独立完整Validate。

### 7.3 conflict

以下任一情况产生结构化Conflict且不产出可执行Graph：

- 同merge key出现不同Route ID、Revision、Publisher或Digest；
- 同Route ID换Matrix、Port、Reader、Endpoint、Manifest、Artifact、Capability、Candidate或Policy；
- 多个active Gateway、ProviderTransport或Provider Binding；
- 规范化ProviderTransport或actual Provider身份被另一个Route、Port、Slot、Factory、Dependency、Hook/Phase或实际注入边暴露为raw入口；Candidate ID不同也不得逃避；
- Gateway/ProviderTransport/Provider/Reader角色type-pun，或Application Tool Port与Runtime Governance Port互换；
- required extension缺失、重复、optional、未注册或payload canonical不一致。

跨版本active-route冲突必须产出领域专用、不可执行的`ControlledOperationProviderRouteConflictV2`诊断，至少绑定`ConflictCode`、closed `Phase`、closed Matrix、左右Route version/ref、左右规范化ProviderTransport/Provider身份、可用时左右TransportBinding/ProviderBinding与Conflict digest。prebinding alias/V1 conflict只携`AssemblyInputDigest`，严禁伪写Graph/Wiring provenance；postbinding active-route conflict同时精确携`AssemblyInputDigest/GraphDigest/WiringInventoryDigest`。Validate按ConflictCode+Phase强制上述provenance。V1 `RouteBinding`必须由Harness Owner-current Reader按exact Ref执行S1/S2复读，恢复右侧完整规范化身份、Binding与短租约；调用方直传Fact不得替代Owner复读。V2 pre-binding侧尚无Binding时必须显式标为`v2-prebinding`，不得伪造Binding。`ConflictCode=active_route_version_conflict`时不得降级为warning、幂等命中或newer-wins；诊断只解释拒绝原因，不授Binding、Entry或执行资格。

`provider_alias_conflict`另必须携closed `AliasSurfaceV2{Kind,Ref,ModuleRef,PortSpecRef,Capability,CanonicalDigest}`。Kind只允许`candidate|port|slot|factory|dependency|phase`，并按Kind冻结唯一坐标形状：Candidate=`Ref+Module+Port`且无Capability；Port=`Ref==PortSpecRef+Capability`且无Module；Slot=`ContributionRef+Module+Port+Capability`；Factory=`FactoryRef+Module+Capability`且无Port；Dependency=`FromRef+ToRef(置于PortSpecRef)+Capability`且无Module；Phase=`ContributionRef+Module+HookFaceRef(置于PortSpecRef)+PhaseCapability`。完整offending object由同一Conflict的exact `AssemblyInputDigest`绑定，AliasSurface在该输入内唯一定位来源；字段缺失、额外字段或Kind互换均由Seal/Validate拒绝。Candidate若能完整归一，可同时把右侧normalized identity替换为Candidate identity；其他surface不得用空identity回退为protected identity来掩盖来源。

## 8. post-binding Conformance

```text
ControlledOperationProviderRouteConformanceV2 {
  ContractVersion
  Declaration              exact DeclarationRefV2
  RequiredExtension        exact key/schema/content digest
  AssemblyInputDigest
  ManifestDigest
  GraphDigest
  Generation                  GenerationArtifactRefV1
  HandoffID                   string
  HandoffRevision             core.Revision
  HandoffDigest               core.Digest
  BindingSetID/Revision/Digest/SemanticDigest/CurrentnessDigest
  AssemblyBindingConformanceRef/Digest
  ActiveRouteID               string
  ActiveRouteRevision         core.Revision
  ActiveRouteDigest           core.Digest
  ToolAdapterBinding       exact ProviderBindingRefV2
  GatewayBinding           exact ProviderBindingRefV2
  ProviderTransportBinding exact ProviderBindingRefV2
  PreparedReaderBinding    exact ProviderBindingRefV2
  BoundaryReaderBinding    exact ProviderBindingRefV2
  ProviderInspectBinding   exact ProviderBindingRefV2
  ProviderBinding          exact ProviderBindingRefV2
  WiringInventoryRef/Digest exact test|production injection inventory
  CheckedUnixNano
  ExpiresUnixNano
  Status                   conformant | rejected | expired
  ConformanceDigest
}
```

`ProviderTransportBinding`绑定Tool侧Provider Transport endpoint/adapter本身；`ProviderBinding`是从Declaration中独立`Provider` Endpoint/Candidate post-binding解析出的actual Provider current proof。两者是不同nominal角色，不能因Component或Capability相同而折叠或type-pun；Prepared/Request必须逐字段精确绑定后者。

Conformance只能由Harness-owned Builder接受`CompileDigest/BindingSetID/ActiveRouteID/Revision`四项exact lookup key，再通过`ControlledOperationProviderRouteConformanceInputsReaderV2`读取verified compile result和live exact Assembly artifacts/current Readers；InputsReader与其OwnerSource都含assemblyadapter包内未导出sealing method，外包stub不能经`FromOwner`绕回Builder。OwnerSource只返回Compile、Generation-Binding Association、ActiveRoute、WiringInventory与七Binding的stable exact refs；verified Compile、ActiveRoute current和WiringInventory分别由独立exact Reader读取，Runtime Generation-Binding Association及七Binding继续由各自公共current Reader复读。Harness-local Owner Artifact Store只做create-once exact refs/内容索引，不签发Runtime currentness。调用方不得提交完整Snapshot。Builder必须重新验证extension内完整Declaration、Governance Catalog digest、Publisher Manifest、Assembly Input/Manifest/Graph/Generation/Handoff lineage、完整`AssemblyBindingConformanceV1`、BindingSet/ActiveRoute/wiring与七Binding。Compile Result必须从真实Manifest/Module/PortSpec/Candidate重建ProviderTransport与Provider规范化身份，PortSpecDigest、ConflictDomain或Binding任一漂移均在零Conformance写入前拒绝。任何自签`Compile/Generation/Handoff/BindingSet/currentness`、`Inline='x'`或空Publisher Manifest同样拒绝。TTL取ActiveRoute current、七个Binding current（ToolAdapter、Gateway、ProviderTransport、Prepared、Boundary、ProviderInspect、ProviderBinding）及其他全部适用期的最小值，时钟回拨、恰好到期或任一ref漂移均拒绝。它不改变BindingSet，不授Capability、Permit或Provider调用权。

### 8.1 no-bypass全图扫描

Provider Transport不能只按Candidate ID去重。Compiler先把所有候选归一为预期身份，post-binding Conformance再闭合为完整身份元组：

```text
NormalizedProviderTransportIdentityV2 =
  ProviderRef / CandidateID / ModuleRef / ComponentManifestRef /
  ArtifactDigest / Capability / PortSpecRef+PortDigest+ConflictDomain /
  exact ProviderBindingRefV2

NormalizedActualProviderIdentityV2 =
  ProviderRef / CandidateID / ModuleRef / ComponentManifestRef /
  ArtifactDigest / Capability / PortSpecRef+PortDigest+ConflictDomain /
  exact ProviderBindingRefV2
```

pre-binding阶段分别从ProviderTransport Candidate与Provider Candidate形成两个不可执行expectation；post-binding阶段必须分别替换为exact `ProviderTransportBinding`与`ProviderBinding`并重扫。不同Candidate ID、别名Component或重复Module只要归一身份相同，均视为对应层的alias，不是第二条合法路线；Transport alias与actual Provider alias两组都必须闭合。

扫描范围必须覆盖全部`ProviderBindingCandidates`、`PortSpecs`、`SlotContributions`、`ModuleFactoryDescriptorV1.OutputCapability`、`Dependencies`、HookFace/PhaseContribution、其他Route，以及test/production composition提供的sealed真实注入边清单。所有Candidate/Port/Slot/Factory/Dependency/Hook/Phase alias路径统一返回sealed `provider_alias_conflict`和可由`errors.As`读取的`ControlledOperationProviderRouteConflictErrorV2`，不得退回自由字符串Conflict。production root若不能枚举全部可调用Provider Transport与actual Provider句柄及消费者，Conformance必须Fail Closed。仅扫描Manifest Candidate集合不足以满足`no_raw_provider_bypass`。

### 8.2 V1/V2 active-route联合扫描

`AssemblyInputV1.RouteBindings`与V1 Provider Candidate不能满足V2 Declaration，但必须作为潜在旁路进入联合扫描。Harness-local Legacy Fact使用closed state `active|inactive|revoked`并绑定sealed WiringInventory、`CheckedUnixNano/ExpiresUnixNano`及从稳定Record identity确定性派生的exact `RouteBindingRef`。Compile只接受Harness Owner-current Reader；每个Ref以fresh clock执行S1/S2，要求两次分别满足`checked <= now < expires`、`nowS2 >= nowS1`且两次Fact完全一致。即使S1/S2分别都在同一租约窗口内，只要S2时钟早于S1也必须返回`ReasonClockRegression`。调用方直传Fact、无Reader、自报`Record.Version+RouteID`或不相关RouteBindingRef均不能替代current证明。Scanner按Route version、closed matrix及规范化Provider Transport/actual Provider双身份建立active-route version index：

- V1与V2同时指向相同或别名Transport/Provider身份时，即使内容完全一致也不是跨版本幂等，必须结构化Conflict；
- V1 route以不同Candidate ID、Module别名、Port别名或BindingSet ref暴露同Transport或Provider，仍Conflict；
- 同matrix下V1/V2分别激活两个不同Transport也Conflict，不能用priority、newer-wins或V2 shadow V1；
- `active`必须在sealed WiringInventory中存在exact active record并产生结构化Conflict；`inactive/revoked`必须让同一sealed inventory覆盖目标binding的exact non-active record，并扫描拒绝同matrix或Transport/Provider identity/Binding任一别名相同的其他active V1 route，不能用一个non-active记录掩盖另一个active记录；
- proof过期、`now<checked`、S1/S2分别在窗口内但`nowS2<nowS1`、S1/S2 Fact漂移、RouteBindingRef与Record不对应、目标binding未被inventory覆盖、同identity另有active V1、state/Active标志或inventory digest任一漂移均Fail Closed；
- Conflict时不产出current Projection、不创建Entry、不调用Provider Transport。

## 9. Runtime-facing current projection/Reader

Runtime公共Ports后续以additive合同冻结中立投影；Runtime不import Harness实现，也不复制完整Declaration。

Runtime/Harness边界由Go包`ExecutionRuntime/runtime/ports`唯一拥有且仅拥有以下六个中立Route公共类型：

1. `ControlledOperationProviderRouteDeclarationRefV2`；
2. `ControlledOperationProviderRouteConformanceRefV2`；
3. `ControlledOperationProviderRouteCurrentRefV2`；
4. `ControlledOperationProviderRouteCurrentProjectionV2`；
5. `ControlledOperationProviderRouteCurrentReaderV2`；
6. 已存在的`OperationScopeEvidenceApplicabilityMatrixKeyV3`。

但Runtime ports不得定义或拥有Harness `ControlledOperationProviderRouteDeclarationV2`、`ControlledOperationProviderRouteConformanceV2`事实struct及其canonical/digest，也不得import Harness。Harness Assembly import Runtime ports的中立类型，拥有Declaration/Conformance事实生成、严格Validate、canonical与digest；这就是`type Owner != fact semantic Owner`。Runtime代码Owner不因此成为Assembly事实Owner，Harness事实Owner也不得复制公共类型。

六个公共类型中的两个Ref、CurrentRef、Projection与Reader真实Go形状冻结为：

```go
package ports

const ControlledOperationProviderRouteCurrentContractVersionV2 = "2.0.0"

type ControlledOperationProviderRouteDeclarationRefV2 struct {
	RouteID              string        `json:"route_id"`
	Revision             core.Revision `json:"revision"`
	PublisherComponentID string        `json:"publisher_component_id"`
	DeclarationDigest    core.Digest   `json:"declaration_digest"`
}

type ControlledOperationProviderRouteConformanceRefV2 struct {
	ConformanceID     string                                          `json:"conformance_id"`
	Revision          core.Revision                                   `json:"revision"`
	DeclarationRef    ControlledOperationProviderRouteDeclarationRefV2 `json:"declaration_ref"`
	ConformanceDigest core.Digest                                     `json:"conformance_digest"`
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

type ControlledOperationProviderRouteCurrentReaderV2 interface {
	InspectCurrentControlledOperationProviderRouteV2(
		context.Context,
		ControlledOperationProviderRouteCurrentRefV2,
		OperationScopeEvidenceApplicabilityMatrixKeyV3,
	) (ControlledOperationProviderRouteCurrentProjectionV2, error)
}
```

`ContractVersion`必须等于`ControlledOperationProviderRouteCurrentContractVersionV2`。canonical domain固定为`praxis.runtime.controlled-operation-provider-route-current`，ObjectKind分别固定为`ControlledOperationProviderRouteDeclarationRefV2`、`ControlledOperationProviderRouteConformanceRefV2`、`ControlledOperationProviderRouteCurrentRefV2`与`ControlledOperationProviderRouteCurrentProjectionV2`；Ref不重复携ContractVersion/ObjectKind字段，不增加自由`map`或optional metadata。所有字段进入各自ObjectKind canonical；self digest在自身计算时清零，ProjectionDigest独立计算。

`Validate`要求全部string/digest非空、Revision非零、`Generation.Validate()`成功、Handoff/ActiveRoute flattened ID/revision/digest完整、BindingSet五项水位完整、七个`ProviderBindingRefV2.Validate()`成功、`CheckedUnixNano > 0`且`ExpiresUnixNano > CheckedUnixNano`。Handoff或ActiveRoute same-ID任一revision/digest漂移均为`core.ErrorConflict`；过期、TTL crossing或Binding currentness漂移为`core.ErrorPreconditionFailed`并携精确既有Reason。

live `ProviderBindingRefV2`包含BindingSet ID/revision、Component、Manifest、Artifact与namespaced Capability，但不含自由Role字段。首版只有在Declaration为七个Projection字段分别冻结互不复用的role-specific Capability，并由Harness Conformance Fact把`Role + PortSpec ID/digest + Capability`一对一映射到对应Binding时，才允许复用该类型；Projection字段位置提供nominal role，Capability提供可验证区分。Prepared、Boundary、ProviderInspect或其他角色若复用Capability、映射不唯一或PortSpec无法current复读，Conformance/Current publish必须Fail Closed，不得靠字段互换强行type-pun。该约束满足时无需新增第七个公共类型；若Runtime联合Review拒绝此映射充分性，只能新增版本化公共Delta，禁止静默扩充`ProviderBindingRefV2`或当前Projection digest。

Harness Declaration Fact负责产出exact runtimeports DeclarationRef；Harness Conformance Fact负责产出exact runtimeports ConformanceRef。`ConformanceID = deterministic Derive(RouteID, GenerationArtifactRefV1, BindingSetID)`，Revision只由Harness Conformance Fact Owner单调CAS；ConformanceRef不重复嵌入Generation/Handoff全量，完整事实留在Harness Conformance Fact与CurrentProjection。

Harness Assembly Adapter只导入Runtime ports并实现Reader接口；Harness不得重新声明、type alias、复制或包装上述六个类型。Harness侧只允许编译期断言其Adapter满足`ports.ControlledOperationProviderRouteCurrentReaderV2`。

### 9.1 Route Current权威发布

Harness Assembly Route Current Store/Adapter是Route Current Fact唯一Owner。Current Fact是post-binding产物，不进入、也不修改`AssemblyInputV1`任何字段或digest：

1. `CurrentID = deterministic Derive(RouteID, MatrixDigest)`；同Route与closed Matrix始终导出同一ID，调用方不得自选ID；
2. Declaration与Conformance同ID为immutable create-once：exact replay幂等，换任一内容直接Conflict；Current `Revision`只由该Store以单调CAS推进，调用方不得覆盖、跳号或按latest自由发现；
3. `Watermark`直接覆盖ConformanceRef、Generation、Handoff、BindingSet全水位、ActiveRoute与七个Binding current；WiringInventory由Harness Conformance Fact摘要绑定并经ConformanceRef间接进入Watermark。任一维度漂移都必须产生新Revision/Ref；
4. 只有post-binding Conformance成功且上述current事实全部fresh时，Owner才sealed publish exact Projection与Ref；publish不授Entry、Permit、Provider调用或production启用资格；
5. composition只把exact CurrentRef注入Tool Adapter与Runtime Gateway。两者不得List、ResolveLatest、按RouteID discovery或自行构造Ref；Reader只接受exact Ref key。

Conformance或Current publish回包丢失后只Inspect原预期Ref；same canonical重复publish幂等，same ID换任一字段Conflict。Current只接受fresh Checked水位、同BindingSet严格更高revision或新Generation/BindingSet的单调CAS，并持久拒绝任何已见Watermark/Conformance回流；因此`A→B→A`即使重新seal也Conflict。64个不同内容并发只允许一个Revision线性化。旧Ref遇到同ID更高Revision返回Conflict；对应Conformance/ActiveRoute/Binding revoked或expired返回`core.ErrorPreconditionFailed`；Owner/Reader不可访问返回`core.ErrorUnavailable`，全部Fail Closed。

`ControlledOperationProviderRouteCurrentRefV2`是nominal exact Ref，形状只能是上述七个字段，不得增删或由DeclarationRef、ConformanceRef、flattened ActiveRoute坐标重封。`Digest`是`CurrentID/Revision/DeclarationRef/ConformanceRef/MatrixDigest/Watermark`六字段按固定顺序canonical后的摘要，计算时排除自身`Digest`；Projection另有独立`ProjectionDigest`，Ref不得包含或反向绑定它，禁止循环摘要。

Projection `Validate`必须逐字段证明：`Ref.DeclarationRef == Projection.DeclarationRef`、`Ref.ConformanceRef == Projection.ConformanceRef`、`Ref.MatrixDigest`等于closed Matrix canonical digest、`Ref.Watermark`与Projection中的ActiveRoute、完整BindingSet及七个Binding current事实一致；然后独立复算`Ref.Digest`与`ProjectionDigest`。任何字段只靠同ID、历史值或调用方快照匹配都不够。

Reader由Harness Assembly Adapter实现，只持Declaration/Conformance、ActiveRoute及Binding current只读面。它以exact Ref为key执行S1读取、完整exact比较、fresh clock S2复读；Checked不得晚于S2，Expires取Declaration适用期、Conformance、Generation/Handoff、BindingSet、ActiveRoute current及七个Binding current TTL最小值。同`CurrentID`但Revision、Digest或任一Ref字段不同必须返回Conflict；只有`CurrentID`根本不存在才返回NotFound。Reader unavailable、clock rollback、TTL crossing、V1/V2双激活或任一Binding漂移均Fail Closed。

错误类别直接复用Runtime `core.DomainError`封闭集合，不创建Harness平行错误类型：

| 语义 | 现有Runtime类别 | 唯一语义 |
|---|---|
| invalid | `core.ErrorInvalidArgument` | 输入Ref/Matrix形状、canonical或必需字段非法，尚不能形成合法查询 |
| absent | `core.ErrorNotFound` | `CurrentID`在唯一Owner中不存在；不得用于same-ID内容不匹配 |
| conflict | `core.ErrorConflict` | same-ID revision/digest/ref/watermark漂移、type-pun、Projection逐字段不一致、V1/V2双激活或current mismatch |
| expired/stale | `core.ErrorPreconditionFailed` | exact Current/Conformance/ActiveRoute/Binding过期、TTL crossing、stale binding或clock regression；使用既有`ReasonBindingExpired`、`ReasonBindingDrift`、`ReasonClockRegression`等精确Reason |
| unavailable | `core.ErrorUnavailable` | 必需current Reader/Owner暂时不可访问，无法完成fresh读取 |
| indeterminate | `core.ErrorIndeterminate` | Owner可访问但outcome/current真实性无法收敛到上述确定结果；不得重派或猜测NotFound |

Adapter只用`core.NewError`构造错误；调用方只用`core.HasCategory`与`core.HasReason`判定。不得新增`ErrExpired`、`ErrUnknown`、自由字符串错误或把一个类别静默降级成另一个类别；恢复只按原exact CurrentRef继续Inspect。

Runtime `ControlledOperationProviderRequestV2`必须同时绑定exact CurrentRef、DeclarationRef与ConformanceRef；Gateway在Entry create前以exact CurrentRef调用该Reader，并对返回Projection的Current/Declaration/Conformance、ActiveRoute、完整BindingSet水位及七个Binding逐字段fresh比较。Request自带快照、Conformance历史存在或BindingSet历史存在均不能替代current Reader。ProviderInspect Binding缺失或带Execute能力、ProviderBinding与ProviderTransportBinding混用，或Request/Prepared未exact绑定ProviderBinding时，Entry与Provider均不得开始。

### 9.2 F03终态解释

Entry成功后，Runtime私有执行路径可以已经进入Provider Adapter方法；Harness不得承诺Adapter方法调用数为零。若跨TTL发生在不可逆admission之前，只有Runtime返回可验证、绑定原Entry/attempt/stable key的no-effect admission receipt，且明确`irreversibleAdmissions=0`、没有Provider Observation，才能把Entry收口为`rejected_no_effect`。同stable key后续只能Inspect该终态，不得重新执行。

receipt缺失、回包丢失、内容不匹配或无法验证时一律保持`unknown`；Harness Route不得从“没有看到Observation”推导no-effect，也不得把`rejected_no_effect`升级为Settlement或领域成功。

## 10. composition边界

```text
Application Coordinator
  -> SingleCallToolActionPortV1
  -> Tool Owner Application Adapter
  -> Runtime ControlledOperationProviderPortV2 Governance Port
  -> Runtime Gateway / Entry Journal
     [kernel-internal Runner不可见，持exact ProviderTransport]
  -> exact Tool ProviderTransport endpoint/binding
  -> exact actual Provider endpoint/binding
  -> ProviderInspect Reader only for recovery
```

- Harness Assembly拥有Declaration/Conformance/Route Current事实生成、CAS与只读发布，只验证声明、候选、Binding与wiring，不实例化Runtime/Tool/Provider实现，不持有Runtime internal Runner、Provider Transport或actual Provider raw句柄；
- G6A test-only fixture在Runtime V2、Tool Adapter及本Route Conformance全部联合`YES`后，才可手工注入上述Ports；fixture不是production root，不注册/激活Capability；
- production root仍等待G6B完整验收、真实wiring与no-bypass Conformance，不因fixture通过而GO；
- 不新增P3b万能Hook，不把Provider Transport挂到HookFace或通用`runtime.gateway` Slot作为可直接调用对象。

## 11. 相关资产

- [Harness Assembly入口](./README.md)
- [Port Delta索引](./port-deltas.md)
- [Fixture测试矩阵](./controlled-operation-provider-route-v2-test-matrix.md)
- [流程图](./controlled-operation-provider-route-v2.drawio)
- [实施计划](../../../plan/harness/controlled-operation-provider-route-v2.md)
