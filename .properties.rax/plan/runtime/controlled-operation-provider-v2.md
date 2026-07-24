# Runtime Controlled Operation Provider V2实施计划

## 1. 状态与范围

- 对应设计：[Controlled Operation Provider V2](../../design/runtime/controlled-operation-provider-v2/README.md)；
- 第四轮联合Review与第三轮独立代码审计均为`YES`；Runtime V2纵向切面已完成并冻结；
- 本计划只规划Runtime additive V2，不改V1、Evidence V3、Dispatch V4.0、Enforcement 4.1、Settlement V4或AssemblyInput V1；
- Application继续只调用`SingleCallToolActionPortV1`；Tool Application Adapter持V2 Governance Port；Application与Tool均不持Entry FactPort；
- 当前不产出production root、Backend、SLA或物理exactly-once声明。

## 2. 固定产出

Runtime已完成：

1. Runtime `ports`唯一拥有的Declaration/Conformance nominal refs与`RouteCurrentRef/Projection/Reader`；Harness拥有事实语义和Route Current发布；
2. Tool Adapter-facing `ControlledOperationProviderRequestV2/PortV2`；
3. Prepared stable snapshot与完整fresh current projection；
4. Runtime-derived stable Entry identity；
5. `CreateEnteredResultV2{created|existing, opaqueClaim}`内部Owner语义；
6. kernel内部`ControlledOperationProviderRunnerV2`与不可外泄Authorization；
7. unified NotAfter与Provider原子execute admission合同；
8. read-only `ControlledProviderInspectPortV2`；
9. 完整`ControlledOperationProviderResultV2` Validate/canonical；
10. public-only Conformance与零Provider反例。

## 3. P0联合合同冻结

- [x] Application确认不直接调用V2；
- [x] Tool确认Application Adapter持Governance Port但不持FactPort/raw Provider；
- [x] Harness确认pre-binding Declaration、post-binding Conformance与Route Current发布是唯一fact semantic Owner；Runtime `ports`只唯一拥有跨包nominal Go类型；
- [x] Harness Adapter导入Runtime `ports`并实现Reader，不重定义DeclarationRef/ConformanceRef/CurrentRef/Projection/Reader/Matrix；Runtime不定义Harness事实struct/canonical/digest；
- [x] Harness确认Runtime RouteCurrent Projection完整携Conformance、Generation/Handoff、BindingSet、active-route proof及Tool/Gateway/ProviderTransport/Readers/Provider公开Binding；Declaration独立声明closed `provider` role；
- [x] Route Current Owner冻结`CurrentID=Derive(RouteID, MatrixDigest)`、Revision单调CAS和Generation/Handoff/BindingSet/ActiveRoute/七Binding/WiringInventory canonical Watermark；
- [x] post-binding Conformance成功后才sealed publish；重复/lost reply Inspect幂等、changed-content/CAS冲突Conflict、旧Ref/revoke/expire/unavailable Fail Closed；
- [x] kernel Runner不进入Declaration、Conformance、Projection或任何public ref；
- [x] 不使用`RouteBindings[]ObjectRef`，不新增AssemblyInput V1字段或改变digest算法；Declaration通过Manifest digest间接受现有Input摘要约束；
- [x] Runtime确认Entry ID由stable key派生，caller无自由ID；
- [x] Runtime确认只有`created + opaqueClaim`同call stack可构Authorization并调用Runner；
- [x] Prepared Owner确认Projection含Delegation、Prepared与Persisted Enforcement完整语义；
- [x] Evidence Owner确认Evidence Policy与Applicability Policy分离及Handoff→Qualification→Scope闭包；
- [x] Provider Owner确认fresh NotAfter check与irreversible admission同stable key原子；
- [x] 所有Owner接受“缩窄并审计TOCTOU”，不声称跨Owner撤销窗口消失。

退出条件已满足：第四轮联合`YES`，第三轮独立代码审计`YES`（P0/P1/P2=0）。Runtime代码现保持冻结。

## 4. 实施波次

### COP2-P1：public types

- [x] 新增`ports/controlled_operation_provider_v2.go`；
- [x] `github.com/Proview-China/rax/ExecutionRuntime/runtime/ports`唯一冻结五个V2新增中立类型：DeclarationRef、ConformanceRef、CurrentRef、CurrentProjection、CurrentReader，并复用既有MatrixKeyV3；Harness只实现Reader且不重定义/alias；
- [x] DeclarationRef固定`{RouteID string, Revision core.Revision, PublisherComponentID string, DeclarationDigest core.Digest}`；
- [x] ConformanceRef固定`{ConformanceID string, Revision core.Revision, DeclarationRef ControlledOperationProviderRouteDeclarationRefV2, ConformanceDigest core.Digest}`；
- [x] 两Ref外层domain/type discriminator固定版本与kind；无自由map；空/零/非法digest拒绝，same ID任一字段漂移Conflict；
- [x] `ConformanceID=Derive(RouteID, GenerationRef, BindingSetID)`且Revision由Harness Conformance Fact Owner单调CAS；Generation/Handoff全量不重复进入Ref；
- [x] 四个DTO全部使用snake_case JSON tag与真实类型：Revision=`core.Revision`、Digest=`core.Digest`、Generation=`GenerationArtifactRefV1`、七Binding=`ProviderBindingRefV2`；
- [x] CurrentProjection的Handoff与ActiveRoute扁平为exact ID/revision/digest；禁止未定义`GenerationRef/HandoffRef/ActiveRouteCurrentRef`占位；
- [x] `ContractVersion`固定`ControlledOperationProviderRouteCurrentContractVersionV2 = "2.0.0"`；canonical domain/ObjectKind由类型固定，unknown field/free map拒绝；
- [x] Handoff/ActiveRoute same-ID drift、Checked/Expires非法及七Binding field-role Capability type-pun全部Fail Closed；
- [x] Request携exact RouteCurrent、Declaration与Conformance refs；
- [x] Reader真实Go签名固定为：

```go
type ControlledOperationProviderRouteCurrentReaderV2 interface {
	InspectCurrentControlledOperationProviderRouteV2(
		context.Context,
		ControlledOperationProviderRouteCurrentRefV2,
		OperationScopeEvidenceApplicabilityMatrixKeyV3,
	) (ControlledOperationProviderRouteCurrentProjectionV2, error)
}
```
- [x] CurrentRef固定七字段：CurrentID、Revision、DeclarationRef、ConformanceRef、MatrixDigest、Watermark、Digest；
- [x] Ref Digest与ProjectionDigest分别canonical且无循环摘要；
- [x] Projection Validate逐字段闭合Ref的Declaration/Conformance/Matrix/Watermark；
- [x] 冻结Prepared semantic snapshot/current Projection；
- [x] 冻结Request、Entry Ref、Inspect Port、Governance Port与Result；
- [x] Result status/error组合closed且不含Authorization/Outcome/DomainResult；
- [x] V1/V2 type-pun、nil/empty、all-ref drift在backend读取前拒绝。
- [x] same CurrentID任一字段漂移为Conflict；只有ID从未存在返回NotFound。
- [x] closed error复用`core.DomainError`：Invalid/NotFound/Conflict/Expired/Unavailable/Unknown分别映射`ErrorInvalidArgument/ErrorNotFound/ErrorConflict/ErrorPreconditionFailed/ErrorUnavailable/ErrorIndeterminate`；
- [x] 实现只用`core.NewError/HasCategory/HasReason`与既有binding/clock reasons，不新增`ErrExpired/ErrUnknown`。

### COP2-P2：Entry Owner

- [x] Derive Entry ID：OperationDigest+Effect/Attempt+Prepared ID/digest；
- [x] Boundary/current revision/Checked/NotAfter不进入identity；
- [x] create-once返回`created|existing`；
- [x] 只有created生成不可复制、不可持久opaque claim；
- [x] 同stable key换caller ID或changed content Conflict；
- [x] 64并发只一个claim；
- [x] `entered -> unknown|observed|rejected_no_effect`与`unknown -> observed|rejected_no_effect`单调CAS；
- [x] `rejected_no_effect`必须有可验证admission receipt、无Observation，且同stable key不可重新execute。

### COP2-P3：Gateway current闭包

- [x] 验证Tool Adapter caller、Harness Declaration/Conformance与Runtime RouteCurrent exact关系；
- [x] fresh复读Generation/Handoff、BindingSet current及Tool Adapter/Gateway/Provider Transport/Prepared Reader/Boundary Reader/Provider Inspect/Provider全部Binding；
- [x] 七个Binding逐项current并进入TTL最小值；ProviderTransport与actual Provider Binding分别验证，不得合并；
- [x] Declaration的ProviderTransport与closed `provider` nominal endpoint分别post-bind；no-bypass扫描两层aliases/真实注入，二者raw句柄均不公开；
- [x] fresh复读Harness active-route scan，必须证明V2唯一active且V1未激活；
- [x] fresh复读Effect/Intent与完整Prepared projection；
- [x] 分开复读Evidence Policy、Applicability Policy；Profile只来自Applicability；
- [x] 复读execute Enforcement、Handoff、Qualification、full Scope及Boundary；
- [x] 验证`ProviderBinding.Capability == EffectKind`；
- [x] Unified NotAfter取Intent、Evidence/Applicability Policy、RouteCurrent、七个Binding、Prepared、Boundary、Enforcement、Handoff/Qualification、caller deadline最小值；
- [x] 不引入无Owner/Reader的上游policy expiry；未来Operation Policy仅作exact current门禁，TTL未冻结前不参与NotAfter；
- [x] clock zero/rollback/expiry全部零Entry、零Provider。

### COP2-P4：internal Runner与Provider admission

- [x] Gateway只在未中断的`created + opaqueClaim`call stack构内部Authorization；
- [x] Authorization不进入Fact、Result、日志、sidecar或Tool/Application返回；
- [x] raw Provider只能由Runner持有；
- [x] Provider以stable key原子执行fresh NotAfter check+irreversible admission；
- [x] raw receipt/return不升级Observation；
- [x] 可验证no-effect receipt才收口`rejected_no_effect`；不能证明则unknown；
- [x] Runner不持Evidence、DomainResult、Settlement或Continuation写Port；
- [x] lost create reply即使Inspect看到entered也零Runner。

### COP2-P5：Inspect恢复

- [x] Gateway只从Entry取原Prepared/Attempt调用`ControlledProviderInspectPortV2`；
- [x] caller不能替换Inspect坐标；
- [x] exact Provider State Plane Observation才能收口observed；
- [x] 不可证明则unknown；unknown不得重Execute；
- [x] lost CAS reply只Inspect，Runner调用数不增加。

### COP2-P6：Conformance与隔离fixture

- [x] Application direct V2、非Tool caller、Route object-ref、raw Provider bypass全部零Provider；
- [x] Authorization泄漏、序列化、重放、跨Attempt全部拒绝；
- [x] 64并发仅一个created/claim/Runner；
- [x] wrong Binding/Prepared/Policy/Handoff/TTL/rollback全部零Entry或零Provider；
- [x] stale Conformance/Generation/Handoff/Binding与缺Provider Inspect Binding全部零Entry、零Provider；
- [x] public route出现Runner binding、active-route proof缺失/过期或V1/V2同时active全部零Entry、零Provider；
- [x] Runner推进Evidence/DomainResult/Settlement调用数0；
- [x] V1/V2 route互斥；V1保持fixture-only；
- [x] `ProductionClaimEligible=false`。

## 5. Tool/Harness后续接线（本计划不实施）

- Tool Application Adapter：实现既有SingleCall入口，内部调用V2 Governance；
- Tool/Provider Owner：提供Prepared current Adapter、raw Provider transport与只读Inspect；
- Harness Assembly Owner：保留唯一canonical Declaration/Conformance事实与Route Current Store/发布，导入Runtime nominal refs并实现RouteCurrent Reader Adapter与active-route scan current proof；
- 宿主composition root：保证raw Provider没有Runner旁路；
- 各Owner另行设计、评审与实现，Runtime不跨改。

## 6. 实际代码门禁

```text
go test -count=100 <targeted packages>
go test -race -count=20 <targeted packages>
go test -count=1 -shuffle=on ./...
go test -count=1 -race ./...
go vet ./...
gofmt -l .
git diff --check -- .
```

上述targeted ordinary count100、race count20、full shuffle ordinary/race、vet、gofmt与diff-check均已通过；中央full ordinary/race亦通过。

## 7. 完成条件

- Application不直接持V2 Port，Tool Adapter不持Entry FactPort/raw Provider；
- typed Route、stable Entry identity、created opaque claim与internal Runner闭合；
- Runtime `ports`是全部Route nominal Go类型唯一Owner，Harness是Declaration/Conformance/RouteCurrent事实语义Owner；
- Route Current确定性ID、Revision CAS、完整Watermark、sealed publish与closed `core.DomainError`映射闭合；
- ProviderTransport与closed `provider` endpoint分别声明、post-bind、current验证和no-bypass扫描；
- Prepared、两Policy、Handoff/Qualification/Scope和Capability exact；
- NotAfter由全部current TTL最小值形成，Provider admission自行原子检查；
- Result不泄漏Authorization且不冒充Observation/Outcome/DomainResult；
- lost reply/unknown只Inspect原Prepared/Attempt；
- V1不改；Runtime以Harness active-route scan current proof执行V1/V2互斥；
- 仍明确跨Owner撤销窗口只被缩窄、线性化和审计化。


## 8. 完成记录

- public Route类型冻结为五个V2新增类型加既有MatrixKeyV3，唯一Reader签名与Projection字段/JSON tag由反射及AST测试锁定；
- Prepared与PreparedSemanticSnapshot使用legacy V3 Permit水位；Request execute Attempt、Boundary与execute Enforcement使用V4 Permit Fact水位；
- Entry create/lost-create/CAS恢复按immutable request与stable key判定，同请求的合法progressed revision只Inspect，不恢复claim、不重Runner；
- Entry Fact闭合Route七Binding、BindingSet digest/semantic digest及Entered时刻的Checked/Issued/Expires/NotAfter；
- reference store、fake transport与Conformance不代表production backend/root、持久性、availability、SLA或物理exactly-once。
