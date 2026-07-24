# Controlled Operation Provider强类型Assembly Route V2实施计划

## 1. 状态

- 对应设计：[Controlled Operation Provider Route V2](../../design/harness/assembly/controlled-operation-provider-route-v2.md)；
- Runtime公共Route合同第三轮独立审计最终`YES`；Harness Route V2第八独立短审最终`YES(P0/P1/P2=0)`，Route模块门已解除；
- 当前仍不修改Runtime/Tool/Application，不建立cross-module fixture或production root；
- 不修改`AssemblyInputV1`、通用`RouteBindings`、HookFace或既有digest。

## 2. 预期产物

本轮已经实现并通过第八独立短审：additive Route Declaration及nominal Ref、required registered extension严格解码、closed matrix/canonical、确定性多Declaration merge、结构化Conflict、全图no-bypass、post-binding Conformance、Owner-backed exact artifact Readers、Runtime-facing current Reader Adapter、S1/S2跨读取时钟单调性，以及对应unit/whitebox/fault/64并发/race测试。该YES只覆盖Route，不把Owner-local完整合同冒充cross-module fixture或production root。

不产出Runtime Governance/Entry/internal Runner、Tool ProviderTransport/Boundary/Inspect实现、Application Coordinator、production Backend/RPC/root、Capability启用、Continuation或Turn推进。Runtime internal Runner不得进入Harness公共合同或fixture注入面。

## 3. 依赖顺序

```text
Route design/plan联合YES
-> Runtime V2 Governance/Entry及private actual-point实现联合YES
-> Runtime-facing RouteCurrentProjection/Reader公共合同联合YES
-> Tool Prepared/Boundary Reader + ProviderTransport/Inspect Adapter联合YES
-> Harness Route contract/compiler/conformance隔离实现与验收
-> G6A test-only cross-module fixture -> G6A PASS
-> G6B test-only fixture与完整验收
-> production root真实接线/no-bypass Conformance
-> production Capability/Continuation/Turn GO
-> G7 Checkpoint
```

Runtime V2或Tool Adapter代码未落时，只能实现/测试Harness Owner-local纯合同与编译器；不得运行或宣称G6A cross-module fixture闭合。

## 4. 实施阶段

### P0：联合合同冻结

- [x] Runtime确认Governance Port由Tool Adapter消费，Entry FactPort不外泄，Authorization不出公共Result，internal Runner不进入Harness公共合同/fixture；
- [x] Runtime确认Request同时绑定exact CurrentRef/DeclarationRef/ConformanceRef；Gateway只调用真实Go接口`InspectCurrentControlledOperationProviderRouteV2(context.Context, ControlledOperationProviderRouteCurrentRefV2, OperationScopeEvidenceApplicabilityMatrixKeyV3) (ControlledOperationProviderRouteCurrentProjectionV2, error)`并fresh比较完整Projection；
- [x] Runtime ports只定义DeclarationRef/ConformanceRef/CurrentRef/Projection/Reader/MatrixKeyV3六个中立类型，不定义Harness Declaration/Conformance Fact或canonical/digest；
- [x] Tool确认exact ProviderTransport、独立actual Provider、Prepared/Boundary/ProviderInspect Adapter与ProviderBinding Capability；ProviderInspect必须read-only/no-execute；
- [x] Application确认Coordinator仍只持`SingleCallToolActionPortV1`与既有只读Settlement面；
- [x] Harness确认Route对象、required extension、merge/conflict与no-bypass；
- [x] 宿主确认test fixture与production root严格分离。

退出条件：联合`YES`。否则不实施。

Runtime Owner必须先在其独占路径冻结公共代码（Harness不得代写）：

```text
ExecutionRuntime/runtime/ports/controlled_operation_provider_route_current_v2.go
```

该文件只含六个中立类型中的Route Ref/Projection/Reader及其Validate/canonical；不得包含Harness Declaration/Conformance事实struct或Store。

### P1：公共Route合同

候选文件，仅在后续实现授权范围内创建：

```text
ExecutionRuntime/harness/assemblycontract/controlled_operation_provider_route_v2.go
ExecutionRuntime/harness/tests/assembly/controlled_operation_provider_route_v2_test.go
```

- [x] 固定`2.0.0`、canonical domain/type discriminator和nominal Ref；
- [x] Harness Declaration/Conformance保持唯一事实canonical定义；Runtime/Tool只持runtimeports Ref/current Projection，不复制事实struct；
- [x] 冻结DeclarationRef/ConformanceRef exact Go字段、core.Revision/core.Digest类型、snake_case JSON tags、严格Validate及same-ID drift Conflict；ConformanceID按`Derive(RouteID, GenerationArtifactRefV1, BindingSetID)`生成，Declaration/Conformance同ID immutable create-once；
- [x] Harness Declaration/Conformance facts分别产出runtimeports DeclarationRef/ConformanceRef；两者都不本地重定义/alias，ConformanceRef不重复嵌Generation/Handoff全量；
- [x] 只接受closed G6A matrix与两项封闭Policy；
- [x] required extension严格解码，复算payload/declaration digest，并绑定Governance Catalog、Publisher Manifest Kind/Locality/2.x/Capability/extension digest；
- [x] Declaration加入ToolAdapter、ProviderTransport、独立Provider Endpoint/Candidate与ProviderInspect Reader，closed role增加`provider`；
- [x] 证明不新增`AssemblyInputV1`字段、不改canonical算法，Declaration只经Manifest digest间接约束Input digest；通用Route ref不能满足V2。

### P2：确定性merge与no-bypass编译

候选文件：

```text
ExecutionRuntime/harness/assemblycompiler/controlled_operation_provider_route_v2.go
ExecutionRuntime/harness/tests/assembly/controlled_operation_provider_route_merge_v2_test.go
```

- [x] 按matrix稳定排序并幂等折叠exact duplicate；
- [x] 同key换内容Conflict，禁止priority/first-wins/partial merge；
- [x] 交叉校验PortSpec、Manifest、Artifact、Capability和Provider candidate；
- [x] 按`ProviderRef/Candidate/Module/ComponentManifest/Artifact/Capability/Port/ConflictDomain/Binding`分别归一ProviderTransport与actual Provider身份；
- [x] Compile Result从真实Manifest/Module/PortSpec/Candidate重建两项post identity，PortSpecDigest、ConflictDomain和Binding漂移均Fail Closed；
- [x] 扫描全部Candidates、PortSpecs、Slots、Factories output、Dependencies、Hook/Phase、其他Route与sealed真实注入边；
- [x] Candidate ID/Module/Component/Port alias仍识别为对应Transport/Provider并拒绝两层旁路；
- [x] 联合扫描V1/V2 active route；相同、别名或同matrix双激活均Conflict，不允许newer-wins；
- [x] V1 RouteBinding只经Harness Owner-current Reader按exact Ref执行S1/S2复读，恢复左右normalized identity/Binding；调用方Fact直传、缺Reader或过期proof Fail Closed；
- [x] 所有alias扫描路径统一产出sealed `provider_alias_conflict`，V1/V2统一产出sealed `active_route_version_conflict`；`errors.As`和Conflict digest可验证；
- [x] Conflict按closed code/phase强制provenance：prebinding alias/V1只含AssemblyInputDigest，postbinding active-route含AssemblyInput/Graph/Wiring三项；
- [x] closed AliasSurface逐Kind强制唯一坐标形状并复算canonical digest；Dependency用Ref/PortSpecRef分别绑定From/To，完整source由同一Conflict的AssemblyInputDigest绑定；缺失/额外/Kind错位字段及空identity回退全部拒绝；
- [x] Legacy Fact closed state `active|inactive|revoked`的call-time current exact absence proof通过第八独立短审：fresh clock、`checked<=now<expires`、显式`nowS2>=nowS1`、S1/S2 Fact exact、Record-derived RouteBindingRef、目标non-active binding覆盖与同matrix/alias identity active V1扫描均已闭合；
- [x] Port kind生产路径正例进入closed AliasSurface测试，并逐字段绑定`Ref==PortSpecRef+Capability`；
- [x] 产出Harness纯编译`ControlledOperationProviderRouteConflictV2/ConflictCode/ConflictDigest`，ConflictSide包含TransportBinding/ProviderBinding，不产半Graph、不扩大Runtime API。

### P3：post-binding Conformance

候选文件：

```text
ExecutionRuntime/harness/assemblyfacts/controlled_operation_provider_route_conformance_v2.go
ExecutionRuntime/harness/assemblyfacts/controlled_operation_provider_route_current_v2.go
ExecutionRuntime/harness/assemblyadapter/controlled_operation_provider_route_v2.go
ExecutionRuntime/harness/tests/assemblyfacts/controlled_operation_provider_route_v2_test.go
ExecutionRuntime/harness/tests/assemblyintegration/controlled_operation_provider_route_v2_test.go
```

- [x] Conformance Builder只接受exact lookup key；InputsReader与OwnerSource均package sealed，OwnerSource只返回stable exact refs，verified Compile/ActiveRoute/Wiring分别由独立Reader复读；调用方或外包stub自签Snapshot零写拒绝；
- [x] 只读关联ActiveRoute current、BindingSet全水位、ToolAdapter/Gateway/ProviderTransport/Prepared/Boundary/ProviderInspect及独立ProviderBinding七个Binding current；
- [x] 新增Harness实现的`ControlledOperationProviderRouteCurrentReaderV2` Adapter，按exact CurrentRef返回Runtime-facing exact Projection；
- [x] DeclarationRef/ConformanceRef/CurrentRef/Projection/Reader/MatrixKeyV3六项只使用`ExecutionRuntime/runtime/ports`公开类型；Harness不得重定义struct、interface、alias或本地DTO，并以编译期接口断言验收；
- [x] Harness Route Current Store/Adapter作为Fact Owner：CurrentID=`Derive(RouteID, MatrixDigest)`、Revision单调CAS且拒绝`A→B→A`；Watermark直接覆盖ConformanceRef、Generation/Handoff/BindingSet/ActiveRoute/七Binding，WiringInventory经ConformanceRef间接绑定；Conformance成功后才sealed publish；
- [x] Owner-local Adapter只接受exact CurrentRef；不提供List/latest/discovery或调用方自造ID；Current仍是post-binding产物，不修改AssemblyInput；真实Tool/Runtime composition注入保持P4 fixture前置；
- [x] CurrentRef严格固定`CurrentID/Revision/DeclarationRef/ConformanceRef/MatrixDigest/Watermark/Digest`；Ref Digest排除自身且不含ProjectionDigest，Projection `Validate`逐字段核对Ref与current事实；
- [x] Projection严格复用`GenerationArtifactRefV1`、七个`ProviderBindingRefV2`，Handoff/ActiveRoute flattened为ID/core.Revision/core.Digest；全部字段使用冻结snake_case JSON tags并纳入ObjectKind canonical；
- [x] TTL取ActiveRoute current、七个Binding current及所有其他适用期最小值，clock rollback/expiry/drift拒绝；
- [x] ProviderTransportBinding只证明Transport Adapter，ProviderBinding从独立Provider Candidate post-binding exact投影并由Request/Prepared逐字段比较；两者禁止折叠或type-pun；
- [x] 七个Projection role分别绑定互不复用的namespaced Capability，Conformance一对一证明Role+PortSpec到Binding；
- [x] 同CurrentID换revision/digest/ref/watermark返回Conflict；仅ID不存在返回NotFound；
- [x] 错误封闭复用`core.ErrorInvalidArgument/ErrorNotFound/ErrorConflict/ErrorPreconditionFailed/ErrorUnavailable/ErrorIndeterminate`；expired/TTL/stale/clock分别使用既有`ReasonBindingExpired/ReasonBindingDrift/ReasonClockRegression`等精确Reason；
- [x] Adapter只用`core.NewError`，测试只用`core.HasCategory/HasReason`；禁止新增`ErrExpired/ErrUnknown`、Harness error enum、自由字符串或跨类降级；
- [x] `assemblycontract/compiler/adapter`只单向import Runtime core/ports；Runtime不得import Harness，编译/import边界测试证明无环；
- [x] 报告不改Binding，不授Permit/Authorization/Capability。

### P4：G6A test-only cross-module fixture

前置：Runtime V2、Tool Adapter及本Route P1-P3全部联合验收。

- [ ] 手工注入公共Ports/Owner Adapters，不使用production root；
- [ ] 覆盖lost Conformance/Current publish、same canonical重复publish、64并发不同内容CAS、旧Ref、revoked/expired/unavailable；
- [ ] 覆盖lost Entry create reply、两处TTL crossing、unknown、64并发和raw ProviderTransport/actual Provider bypass六类反例；
- [ ] F03允许Provider Adapter方法进入；exact no-effect admission receipt才收口`rejected_no_effect`，否则`unknown`；断言irreversible admissions为0而非Adapter calls为0；
- [ ] Application只见Tool Port，Tool Adapter只见Runtime Governance Port；
- [ ] fixture只向公共协调层注入Application/Runtime Ports与三类current Reader；ProviderTransport到actual Provider只在sealed私有接线中出现，不增加raw句柄注入面；
- [ ] Provider Observation不推进Evidence/DomainResult/Settlement/Continuation；
- [ ] `ProductionClaimEligible=false`，Capability/production Continuation/真实Turn调用数均为0。

### P5：production root残余

本计划不实现root。G6A/G6B完整验收后，宿主Owner另行设计、实现并验证真实wiring、Runtime私有路径对exact ProviderTransport及Transport对exact actual Provider的唯一持有关系、所有构造路径双层no-bypass及Binding/Generation/TTL漂移撤销；真实接线Conformance通过后才允许production Capability、Continuation与Turn推进。

## 5. 测试门禁

本轮实际执行targeted ordinary100/race20、full ordinary/race/vet、gofmt、diff/import/zero-network与资产链接/XML门禁；命令结果以本任务最终回传为准。详细反例见[Route V2测试矩阵](../../design/harness/assembly/controlled-operation-provider-route-v2-test-matrix.md)。P4 cross-module fixture和production root仍未执行。

## 6. 完成条件

- Declaration不依赖通用`RouteBindings`或万能Hook；
- `AssemblyInputV1`字段、Validate与digest保持不变；
- Runtime ports六个中立类型与Harness Declaration/Conformance/Current Fact Owner边界无import cycle；
- closed matrix、Current/Declaration/Conformance/ActiveRoute refs及ToolAdapter/Gateway/ProviderTransport/Prepared/Boundary/ProviderInspect/Provider七个Binding current全部exact；
- merge、Conflict、one-active与no-bypass可自动验证；
- Generation/Handoff/BindingSet/Conformance通过post-binding对象闭合，Runtime只消费fresh current Projection且无pre-binding/import循环；
- V1/V2 active route联合扫描、Transport/actual Provider两层alias candidate与真实注入旁路全部Fail Closed；
- 六类fixture反例通过仍不等于production eligible；
- `G6A -> G6B -> production root -> G7`顺序未被绕过。
