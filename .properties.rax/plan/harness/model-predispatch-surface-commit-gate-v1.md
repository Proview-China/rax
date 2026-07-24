# Model PreDispatch Surface Commit Gate V1实施计划

## 1. 状态

- 当前：中央唯一采用的`A2+B1+C2` Owner-current、Runtime neutral Current Reader、Harness concrete Model Gate与同实例ACK create-once Repository均已实现，达到`owner-local implementation_software_test_yes`并通过对应独立代码审计；
- 实施：Owner-local M2/Gate不再重开；当前工作转向Model actual-point全路径强制guard，以及Harness Assembly专用required Capability、Factory/Binding和no-bypass Conformance。A2继续由Harness保存完整CompileResult+Conformance，B1只复用Runtime canonical Association Gateway，C2只复用Tool public完整Manifest窄Reader；
- 系统边界：Application P2不变；Model actual-point、Tool V2 Consumer、system G6A、Capability与production root继续`NO-GO`；
- 设计事实源：[Gate V1设计](../../design/harness/assembly/model-predispatch-surface-commit-gate-v1.md)与[测试矩阵](../../design/harness/assembly/model-predispatch-surface-commit-gate-v1-test-matrix.md)。

## 2. 预期产物

联合设计和依赖门全部通过后，后续Harness实施候选为：

```text
ExecutionRuntime/harness/assemblyadapter/model_predispatch_assembly_current_v1.go
ExecutionRuntime/harness/modelinvokeradapter/prepared_model_invocation_ack_repository_v1.go
ExecutionRuntime/harness/modelinvokeradapter/predispatch_surface_commit_gate_v1.go
ExecutionRuntime/harness/tests/modelinvokeradapter/prepared_model_invocation_ack_repository_v1_test.go
ExecutionRuntime/harness/tests/assemblyintegration/model_predispatch_assembly_current_v1_test.go
ExecutionRuntime/harness/tests/assemblyintegration/model_predispatch_surface_commit_gate_v1_test.go
ExecutionRuntime/harness/tests/conformance/model_predispatch_surface_commit_gate_v1_test.go
```

Model Prepared Fact/Current、ACK public nominal/canonical、Gate调用与两阶段actual-point由Model Owner实现；Harness/host实现Gate并拥有Model ACK同实例create-once Repository；Tool Surface Current及Binding Ensure/Inspect由Tool Owner实现。Harness不复制类型或创建替代Store。

Runtime Owner已落盘并实现neutral `ModelPreDispatchAssemblyCurrentRefV1/ProjectionV1/ReaderV1`与`RegistrySnapshotExactReaderV1`，最终公共composite继续无损复用。B1在M2 integration/acceptance/production中必须由Runtime concrete `GenerationBindingAssociationGatewayV1`提供live窄Reader；生产资格只由Association exact Ref、Conformance分离BindingSet字段、sealed ProviderCandidate→Runtime BindingSet member exact映射、Provider package/type identity与assembly lineage证明，association path不使用`Conformance.Binding`。Runtime public conformance只验Fact shape且`ProductionClaimEligible=false`，仅限Reader接口单测。不得新增Runtime BindingSet Reader。A2只属于Harness，C2只属于Tool public contract；本计划不授权Harness修改Runtime或Tool。

## 3. P0：联合公共合同

- [ ] 只无损复用Model公开完整`PreparedModelInvocationFactV1/RefV1/ReaderV1`与完整Current Ref/Projection/Reader；删除Harness HistoricalProjection及Plan/Tools/Route/Profile/Registry/Current缩水镜像；
- [ ] Gate输入显式携完整Prepared Ref+Current Ref；Current Ref包含ContractVersion/Prepared/Checked/Expires/NotAfter完整回链；
- [ ] Historical Fact携Plan/RequestTools/Route/Profile canonical digest、Owner/ContractVersion/ID/Revision/Digest完整Registry exact Ref、双actual digest与NotAfter；
- [ ] Model冻结纯Preparation→Prepared publish→Gate→actual-point两阶段，不在Phase A调用Provider/Backend；
- [ ] Harness Gate只exact实现Model公开短方法`Commit + InspectExactAck`；清除旧长方法名且不定义本地Gate Request/Writer；
- [ ] Harness/host ACK Repository同一实例实现Owner-local `EnsureAck + InspectExactAck + internal InspectByPreparedCurrent`，stable key只绑定完整Prepared+Current；
- [ ] 直接注入Tool公开Writer/Reader并调用`EnsureToolSurfaceInvocationBindingV1`，由Tool Clock生成Created/NotAfter并Seal Fact/Ack；
- [ ] Tool Binding/Ack明确不授Provider进入权，每次attempt前仍需Inspect current；
- [ ] A2：Harness-owned `ModelPreDispatchVerifiedAssemblyOwnerCurrentStoreV1/ReaderV1`完整保存、deep clone并验证`CompileResultV1 + AssemblyBindingConformanceV1`，使用immutable history、stable current index与full-Ref CAS；Reader以unexported marker package-sealed；
- [ ] B1：构造器静态能力只接Runtime窄`GenerationBindingAssociationCurrentReaderV1`，但M2 composition必须注入concrete `GenerationBindingAssociationGatewayV1`；组装时逐字段验证sealed `ProviderBindingCandidateV1`身份、`AssemblyBindingConformanceV1.Association` exact Ref、Conformance分离的BindingSet ID/revision/digest/semantic/currentness/projection字段、Provider candidate→Runtime BindingSet目标member full Ref映射、完整assembly lineage及Runtime Gateway package/type identity；association path的Conformance Binding/Capability/Schema保持严格空值。Gateway每次Inspect重建BindingSet current并复读Generation/Activation。public conformance只用于接口单测，`ProductionClaimEligible=false`，禁止进入fixture/integration/production资格判定；禁止新增BindingSet Reader或注入GovernancePort；
- [ ] C2：构造器只接Tool public `ToolSurfaceManifestCurrentReaderV1`并调用`InspectExactToolSurfaceManifestCurrentV1`；Tool唯一`ToolSurfaceManifestCurrentRepositoryV1`通过`ToolSurfaceManifestCurrentEnsureRequestV1{ContractVersion,Manifest,ExpectedCurrent full Ref}`保存完整Manifest；rev1要求ExpectedCurrent严格零值，successor仅允许current+1 full-Ref CAS，旧revision重投Conflict/PreconditionFailed；Harness不得获得Ensure或复制Tool类型；
- [ ] 最终继续发布Runtime-neutral `ModelPreDispatchAssemblyCurrentRefV1/ProjectionV1/ReaderV1`；A2/B1/C2不会形成第二composite；
- [ ] Runtime ports additive冻结唯一neutral Assembly Current Ref/Projection/Reader；type owner=Runtime ports，semantic/publisher/current Owner=Harness，Tool直接嵌入且无echo/Kind；
- [ ] Registry唯一使用Runtime ports候选`RegistrySnapshotRefV1`，Owner为完整`core.OwnerRef`；删除Harness私有Ref与旧Model名；
- [ ] Context ExpectedInjection明确退出Surface Binding等式和TTL，保留独立Injection Conformance；
- [ ] Memory/Knowledge Delta10/11已撤第二套DTO但G6B仍P0；Context唯一拥有TransitionProof，Application只协调，SourceTurn exact ref来自Session/Turn Owner Reader；Harness只消费final TargetTurn current Frame且不接SourceTurn/Proof；首个G6B Sources=0/Reader calls=0；
- [ ] 三Owner逐字段mapping与共同最小TTL冻结；
- [ ] NotAfter共同上界只来自显式Owner Fact/current与caller deadline；拒绝匿名上界字段；
- [ ] public错误分类、lost reply、same-ID Conflict、clock rollback闭合。

## 4. P1：Harness Assembly current

- [ ] Harness实现Runtime public `ModelPreDispatchAssemblyCurrentReaderV1`并返回Runtime public Ref/Projection；不在assemblycontract重定义类型；
- [ ] A2完整lineage逐字段闭合Generation/Manifest/Graph/Handoff/Conformance、Diagnostics/Residuals及Manifest Plan Profile/ToolSurface；Generation/Manifest/Graph分别用`GenerationDigestV1/ManifestDigestV1/GraphDigestV1`重算并执行字段硬门，Handoff调用`Validate()`，Conformance调用`Validate(now)`；Generation只映射ID/revision/digest/input/manifest/graph，Catalog由Manifest/Graph/Handoff exact闭合；HandoffRef仅允许`ID=GenerationRef.ID+"/handoff",Revision=GenerationRef.Revision,Digest=Handoff.Digest`；raw caller字段只可作expected echo；
- [ ] A2 canonical分别冻结具名`ModelPreDispatchVerifiedAssemblyOwnerCurrentIdentityCanonicalV1/CompileCanonicalV1/ProjectionCanonicalV1`；domain分别为`praxis.harness.model-predispatch-verified-assembly-owner-current-id/v1`、`praxis.harness.model-predispatch-verified-assembly-owner-current-compile/v1`、`praxis.harness.model-predispatch-verified-assembly-owner-current-projection/v1`；version均为`praxis.harness.model-predispatch-verified-assembly-owner-current/v1`；discriminator分别为`ModelPreDispatchVerifiedAssemblyOwnerCurrentIdentityV1`、`ModelPreDispatchVerifiedAssemblyOwnerCurrentCompileV1`、`ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1`；literal golden digest固定为`sha256:c05c7aab1acd177e819a951287120e5cbab0859d8c3eee9d8478cdab6a45f68c`、`sha256:a833f14f767cd6083cfde17198423cdf4cd0cfdb323a40fe95db69ed0465b455`、`sha256:93c8ffb4f5aeb21685f1a1eee9c32b156b4d28b1ddff9772e6869e468a8013e9`；测试expected必须硬编码，不得调用被测helper现算；
- [ ] B1返回Association Fact后，A2 Conformance.Association/Generation/分离BindingSet字段与Fact逐字段exact；production/integration先以Association Ref、分离BindingSet字段、ProviderCandidate→Runtime member映射、Provider package/type identity与assembly lineage证明注入Runtime concrete Gateway，再由该Gateway的live Inspect真实重建BindingSet current；association path的Conformance Binding/Capability/Schema为空，public conformance不参与此证明；
- [ ] C2 lookup以A2 Manifest Plan ToolSurface exact ID/Revision/Digest构造`ToolSurfaceManifestCurrentRefV1`并补public contract version；调用方不预知ProjectionDigest，返回后才独立验证完整`ToolSurfaceManifestCurrentProjectionV1`/Manifest/Owner/Profile/entries/expected injection/expiry；无Reader或漂移即零CAS；
- [ ] composite携三项exact refs、Semantic/Currentness及共同Checked/Expires，Tool完整保存且不拆解；
- [ ] module DAG冻结为Harness Gate依赖Tool公开Port，Tool不得反向import Harness；Harness与Tool共同只依赖Runtime ports neutral type，Tool Binding直接嵌入且不形成第二current；
- [ ] 冻结无环canonical：Semantic覆盖Profile+完整Registry Ref；Currentness覆盖A2→B1→Handoff→C2→Registry两轮完整输入/输出digest；最终Runtime-neutral Watermark/Projection canonical保持不变；
- [ ] Current Ref与Projection同时完整闭合ToolSurface/Profile/Registry/Semantic/Currentness/Checked/Expires/ProjectionDigest；own digest全部排除canonical输入，最终三项digest exact相等；
- [ ] Checked/Expires严格等于Harness内部所有Owner current的共同窗口；
- [ ] ID/Revision/Watermark/CAS、lost publish、same-ID drift、真实ABA、64异内容、TTL/clock rollback、Reader post-clock cancel闭合；
- [ ] UnknownOutcome recovery除caller deadline与Owner TTL外还有独立host hard cap；不得只用`Expires-now`形成无界恢复；
- [ ] Handoff current Seal拒绝错误非零ContractVersion，不得静默覆盖；
- [ ] same ID+same revision drift=Conflict；仅expected revision+1 CAS可原子替换current index，旧revision历史可读但ValidateCurrent失败；
- [ ] 不改AssemblyInput、Manifest、Graph、Handoff、ModelTurnRequest/Result/Port既有摘要。

## 5. P2：Gate与actual-point no-bypass

- [ ] Harness adapter实现Model公共Gate接口并compile-time assertion；
- [ ] Gate只接受Model公开完整Prepared/Current refs与Readers、Harness exact current、Tool公开Writer/Reader、同一ACK Repository实例和Clock，typed nil与nil context Fail Closed；
- [ ] S1按显式Prepared Ref和Current Ref分别复读Prepared Historical/Current，再读Assembly/Surface并调用Tool公开Ensure；
- [ ] 跨Tool/ACK两仓不宣称原子；Commit第一项Owner调用先`InspectByPreparedCurrent`。命中后复读同一Prepared Current并取fresh Owner clock验证ACK+Current freshness，成功才返回stored clone，零Tool/Ensure/重Seal但不得零clock；只有authoritative never-created才进入Tool恢复、S2、fresh clock Seal/Ensure ACK；
- [ ] Tool回包只ValidateAgainst且不由Harness Seal；Model ACK由Harness Owner fresh clock Seal并由同一ACK Repository Ensure；
- [ ] Tool Exact Reader只接受Model neutral SurfaceBindingRef单参；Harness无损映射Owner/Contract/ID/Revision/Digest，不按Kind猜源；
- [ ] S2复读全部current后才允许首次取fresh ACK Owner clock；禁止先Seal候选再恢复ACK；
- [ ] 每个Provider attempt、Open/Stream和continuation首调前再次Inspect exact Ack/Binding与全部current；
- [ ] Model Owner冻结并实现exact actual-point guard carrier，把完整ACK Ref、closed boundary kind、dispatch sequence、provider attempt ordinal及request digest无损带到每个actual point；禁止context value、裸string、latest ACK或全局表；
- [ ] Model Owner冻结并实现dispatch Receipt create-once sink/reader；Receipt只保存Model Observation，sequence/ordinal由Model attempt Owner产生，Harness不代发；
- [ ] direct Preflight在Gate前保持纯Preparation，`Backend.Resolve`/Capabilities/Provider调用数为0；不得在既有Resolve之后补Inspect冒充两阶段；
- [ ] direct continuation验证Tools/Surface/ActualToolSurface/ActualProviderInjection不变，变化要求新Invocation epoch；
- [ ] `Runtime.Start/Preflight`、direct Resolve/Open、generic/operation Capabilities、routegateway、composite、continuation、realtime和所有provider adapter无旁路；
- [ ] sealed registry capability snapshot用于Phase A；动态Capabilities若改变Injection则拒绝；
- [ ] 64并发同内容单Fact、不同内容Conflict；ACK不产生“唯一进入权”计数。

## 6. P3：Assembly wiring与Conformance

- [ ] 新增专用required Capability/PortSpec/Factory binding，不新增万能Hook；
- [ ] `model.dispatch.before`只作语义阶段，Conformance另外证明真实Gate与所有actual-point guard；
- [ ] 联合冻结并落盘[Model actual-point inventory Port Delta](../../design/harness/port-deltas/model-predispatch-actual-point-inventory-v1.md)所需Model公开exact Inventory nominal/Reader与closed Kind集合、ACK actual-point carrier及dispatch Receipt sink/reader；Harness不私建等价Reader；
- [ ] 扫描Provider candidates、Factory output、Slot、Phase、direct/hosted/remote/realtime/continuation及production inventory；
- [ ] 无Gate、raw Provider或缺attempt前current Inspect的路径不得进入受治理Generation；
- [ ] Route V2、P3 Assembler与Application P2无字段、Owner和import漂移；
- [ ] SourceTurn full exact ref只来自Session/Turn Owner Reader；Context唯一拥有TransitionProof，Application只协调；Harness不接SourceTurn/Proof，只消费final TargetTurn current Frame；
- [ ] owner-local与test-only fixture不冒充production root。

## 7. 测试与验收

依照[测试矩阵](../../design/harness/assembly/model-predispatch-surface-commit-gate-v1-test-matrix.md)执行：

- 单元：historical/current分层、canonical、single composite watermark、TTL、Tool Ensure request、Tool Ack ValidateAgainst、Model ACK create-once；
- 白盒：Tool Surface Owner current、Assembly current CAS、Prepared双Ref双Reader、Gate S1/S2、ACK同实例三能力、每attempt复读；
- 黑盒：Phase A外部调用0、所有provider路径ACK前0、过期后0；
- 故障：Tool winner/Model ACK两个中断点、lost reply、Unavailable、Indeterminate、authoritative NotFound、TTL crossing、clock rollback；
- 并发：64同内容/不同内容、普通100轮、race20轮；
- Conformance：Gate短方法exact、ACK同实例三索引、Tool单参Exact Reader、Runtime neutral composite完整直嵌、无Tool echo/Kind、HookFace不等于runtime Gate、raw Provider bypass、continuation/realtime旁路；
- 边界：Application不变、Harness不Seal Tool Fact、不复制Model/Tool/Memory/Knowledge Owner DTO、不import internal/store；Memory/Knowledge Reader调用数0。

机械门：targeted ordinary100、race20、full ordinary/race/vet、gofmt、import、zero-network、provider-call counter、XML/links/diff-check。

## 8. 完成条件

- A2+B1+C2资产独立设计YES；Harness A2 Store/Reader、Runtime concrete Gateway的Association exact Ref + Conformance分离BindingSet字段 + sealed ProviderCandidate→Runtime BindingSet member exact composition proof、Tool C2完整Manifest Reader/唯一Repo与真实多Owner fixture落地、compile并审计YES；Runtime public conformance只作Reader接口单测证据，不是production proof；
- Harness按新合同返修M2，raw caller splice、真实ABA、64异内容、post-clock cancel、bounded recovery及wrong-version反例全部通过，并重新取得独立代码审计YES；
- Prepared Historical NotAfter是资格绝对上界而非Retention；Current/Tool窗口不得超过；
- Tool Owner单仓、单线性点生成Tool Fact/Ack；Harness/host单独拥有Model ACK同实例Repository，跨仓按Inspect/Ensure恢复而不宣称原子；
- Binding/Ack不授Provider进入权；每attempt均fresh current；
- live no-bypass表全部可执行验证；
- system G6A与production root仍需独立验收。

## 9. 风险与回退

- Model若无法先纯Prepare再Gate，设计Fail Closed，不降级Observer；
- Context ExpectedInjection不进入本桥；不得为Surface Binding私建Context Reader或强制摘要相等；
- Tool若仍要求Harness提交sealed Fact，保持P0阻断；
- Tool Exact Reader若仍要求双Ref/Kind/Invocation而非Model neutral SurfaceBindingRef单参，保持P0阻断；
- Runtime ports唯一neutral Ref/Projection/Reader未公开compile，或Tool仍定义echo/Kind/alias，保持P0阻断；
- Model若未发布完整Fact/Ref/Current/Reader nominal，或任一Owner要求Harness使用缩水镜像，保持P0阻断；
- Memory/Knowledge若要求并存第二套neutral DTO/current而非live V1 additive或唯一facade，非零Source保持阻断；
- 动态Capabilities若改变Tools或任一actual digest，拒绝当前Invocation，不覆写Binding；
- 公共字段变化必须走版本化联合Review，不静默扩大V1。
