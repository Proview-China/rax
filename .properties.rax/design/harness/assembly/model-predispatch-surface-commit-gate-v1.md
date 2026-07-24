# Model PreDispatch Surface Commit Gate V1设计

## 1. 状态与目标

- 状态：中央唯一裁决的`A2+B1+C2` Owner-current、Runtime neutral Current Reader、Harness concrete Model Gate与同实例ACK create-once Repository均已实现并达到`owner-local implementation_software_test_yes`，对应独立代码审计为`YES(P0/P1/P2=0)`；Model actual-point全路径no-bypass、专用Assembly Capability/Conformance、Tool V2 Consumer、system G6A与production root仍未闭合；
- 合同版本：`praxis.harness/model-predispatch-surface-commit-gate/v1`；
- 目标：把tool-bearing Model调用拆为“纯本地Preparation”和“ACK后actual-point”两阶段，并由Harness/host唯一Gate把Model Prepared current、Assembly current、Tool Surface current create-once绑定；
- 硬边界：Binding/Ack只证明因果绑定，不授Provider进入权。每次Provider attempt、`Preflight/Resolve/Capabilities/Open/Invoke/Stream`及direct continuation首调前，都必须重新Inspect exact Binding/Ack与全部Owner current；
- 非目标：不修改Application P2，不让Harness创建或Seal Tool Fact，不复制Model/Tool DTO，不选择生产Backend、RPC、SLA或production root。

live `ModelTurnPort.Invoke`仍是Harness Kernel到Model执行面的私有边界。Assembly Catalog中的`model.request.prepare`与`model.dispatch.before`只是编译期HookFace/Phase声明，没有运行时dispatcher、ACK或actual-point拦截语义，不能冒充本Gate。

## 2. 三Owner与唯一职责

| 语义 | 唯一Owner | Harness可做 | Harness禁止做 |
|---|---|---|---|
| Prepared Model Invocation Fact与Current Projection | Model Owner | 以完整Prepared Ref和Current Ref分别调用公开exact Reader | 按Prepared查latest Current、伪造Owner对象、把NotAfter当Retention |
| Assembly post-binding current | Harness Assembly Owner | A2完整保存CompileResult+Conformance并经B1/Handoff/C2/Registry复读后发布sealed composite | 信caller snapshot、只信ObjectRef格式、私建BindingSet Reader |
| PreDispatch Commit次序 | Harness/host | 唯一Gate；Owner current exact复读；调用Tool公开Ensure | 建第二Tool仓、Seal Tool Fact/Ack、授Provider权限 |
| Tool Surface及Invocation Binding | Tool Owner | 消费其公开Ensure/Reader结果 | 代替Tool生成Created/NotAfter/Digest |
| Provider actual-point | Model Owner | Harness只提供Gate与ACK验证能力 | 把一次ACK解释为永久或单次进入权 |
| Application G6A | Application Owner | 保持既有neutral P2合同不变 | 增加Surface、Gate、Writer或Tool Owner事实 |

依赖DAG固定为：

```text
Model pure preparation
  -> Model public Prepared Fact/Current Reader + Gate interface
Harness Gate adapter
  -> Harness Assembly Current Reader
  -> Tool Surface Current Reader
  -> Tool Binding Ensure/Inspect Port
Model actual-point guard
  -> exact Binding/Ack + Prepared/Assembly/Surface current reread
Application P2 -> unchanged
```

Gate接口及ACK中立类型可由Model公共包拥有，因为Model是调用者；Gate实现与宿主接线次序由Harness拥有。Model不得反向import Harness；Harness只import Model/Tool公开contract/ports。

## 3. 两阶段时序

### 3.1 Phase A：纯本地Preparation

```text
Harness Loop -> ModelTurnPort.Invoke
  -> validate immutable request
  -> read sealed registry capability snapshot
  -> pure Map/Prepare/Seal request tools and tool/provider injection digests
  -> Model Owner create-once Prepared Fact
  -> Model Owner publish Prepared Current Projection
```

Phase A必须满足：

1. `RequestToolsDigest`覆盖完整最终Tools集合；
2. Historical Fact封存Plan/RequestTools/Route/Profile、完整Registry Snapshot Ref与NotAfter；Current封存两个actual digest及current窗口；
3. 不调用registry/provider/backend的动态方法，不进入`Resolve/Capabilities/Preflight/Open/Invoke/Stream`，不创建远程session；
4. 若Capabilities影响工具映射，只能来自进入Prepared digest的sealed registry capability snapshot；
5. 若只能调用Provider `Capabilities`取得能力，则该调用后移到ACK之后，并且只能验证预先sealed映射。任何会改变Tools、ActualToolSurface或ActualProviderInjection的结果都Fail Closed，不能用同Invocation重Prepare。

### 3.2 Phase B：Gate Commit与actual-point

```text
Model actual-point
  -> Harness Gate reads Prepared historical Fact exact
  -> Harness Gate reads the explicitly supplied Prepared Current exact
  -> Harness Gate reads Assembly Current exact
  -> Harness Gate reads Tool Surface Current exact
  -> exact compare ToolSurface.ExpectedInjectionDigest == Model ActualToolSurfaceDigest
  -> Harness calls Tool-owned EnsureToolSurfaceInvocationBindingV1
  -> Tool Owner fresh clock + Seal + create-once Fact/Ack
  -> lost reply only Inspect same Invocation
  -> Harness validates returned Fact/Ack against request
  -> immediately before each actual-point attempt:
       Inspect exact Binding/Ack
       reread Prepared/Assembly/Surface current
       validate common window and ActualToolSurfaceDigest unchanged
  -> only then call Provider/Backend
```

Binding/Ack不授进入权、不计数attempt，也不替代Authority、Permit、Enforcement或Settlement。每个retry、stream open、session open和continuation都执行新的current复读；跨TTL立即Fail Closed。

direct continuation允许Input/State/ToolResult变化，但Tools集合、Surface、`ActualToolSurfaceDigest`、`ActualProviderInjectionDigest`、Profile、RegistrySnapshot及Assembly水位必须不变。任一actual digest变化时当前session拒绝，并要求新的Invocation epoch；不得刷新旧Binding TTL或覆盖原Fact。

## 4. Model公开完整nominal依赖

Harness不得定义、alias、投影或重新序列化任何`PreparedModel*`镜像。唯一事实源是Model Owner公开的完整nominal及其公开窄Reader：

- `PreparedModelInvocationRefV1`：完整携带`ContractVersion/ID/Revision/Digest/InvocationID/InvocationDigest/UnifiedRequestDigest`；
- `PreparedModelInvocationFactV1`：完整携带`ContractVersion/ID/Revision/InvocationID/InvocationDigest/UnifiedRequestDigest/RequestToolsDigest/PreparedPlanDigest/RouteDigest/ProfileDigest/ActualToolSurfaceDigest/ActualProviderInjectionDigest/CapabilitySnapshotRef/RegistrySnapshotRef/CreatedUnixNano/NotAfterUnixNano/Digest`；其中Registry字段直接使用候选`runtimeports.RegistrySnapshotRefV1`；
- `runtimeports.RegistrySnapshotRefV1`：完整携带`Owner core.OwnerRef/ContractVersion/ID/Revision/Digest`，五项缺一均不是exact ref；Harness不得定义私有Registry Ref、string Owner或Model/Tool alias；
- `PreparedModelInvocationCurrentRefV1`：完整携带`ContractVersion/ID/Revision/Digest/Prepared/CheckedUnixNano/ExpiresUnixNano/NotAfterUnixNano`，其中`Prepared`是完整`PreparedModelInvocationRefV1`；
- `PreparedModelInvocationCurrentProjectionV1`：完整携带Model公开的`ContractVersion/ID/Revision/Digest/Prepared/CapabilitySnapshotRef/RegistrySnapshotRef/ActualToolSurfaceDigest/ActualProviderInjectionDigest/CheckedUnixNano/ExpiresUnixNano/NotAfterUnixNano`；
- `PreparedModelInvocationReaderV1`：`InspectExactPreparedModelInvocationV1(ctx, complete PreparedModelInvocationRefV1) -> complete PreparedModelInvocationFactV1`；
- `PreparedModelInvocationCurrentReaderV1`：`InspectExactPreparedModelInvocationCurrentV1(ctx, complete PreparedModelInvocationCurrentRefV1) -> complete PreparedModelInvocationCurrentProjectionV1`。

Harness Gate实现只接收上述两个Model公开Reader和显式`PreparedModelInvocationRefV1 + PreparedModelInvocationCurrentRefV1`。Harness资产与未来代码不得再声明任何本地`PreparedModel*`结构体、alias、Historical Projection、Plan/Tools/Route/Profile/Registry镜像或缩水Current Ref。Model公开Fact中的Plan/RequestTools/Route/Profile是各自完整canonical digest字段，不由Harness伪造成无Owner Ref。

Gate实现必须对Model公开接口做compile-time exact assertion，方法集不得包一层Harness Request/Writer，也不得保留旧长方法名：

```text
Commit(ctx, modelcontract.PreparedModelInvocationRefV1, modelcontract.PreparedModelInvocationCurrentRefV1)
  -> modelcontract.PreparedModelInvocationCommitAckV1
InspectExactAck(ctx, modelcontract.PreparedModelInvocationCommitAckRefV1)
  -> modelcontract.PreparedModelInvocationCommitAckV1
```

`runtimeports.RegistrySnapshotRefV1`虽然由Model Fact/Current无损携带，但Authority始终是其`Owner core.OwnerRef`所指Registry Owner；Runtime ports只拥有neutral type，Model是carrier，Harness/Tool只能exact比较与转交，不得签发、解释或据裸digest补全该Ref。

Prepared Fact是immutable historical事实；其`NotAfterUnixNano`是本Invocation资格的绝对上界，不是Repository retention。Fact在NotAfter之后仍可通过公开Reader历史Inspect，但不能Commit或进入actual-point。Current `ExpiresUnixNano/NotAfterUnixNano`和Tool Binding `NotAfterUnixNano`都必须小于等于Historical `NotAfterUnixNano`。Gate必须由调用方同时提供完整Prepared Ref与完整Current Ref；禁止按Prepared ID读取latest Current，wrong current revision/digest、Prepared回链或时间字段一律Conflict。

### 4.1 Harness/host-owned Model ACK Repository

Harness/host Gate implementation Owner拥有Model ACK的create-once Repository；ACK canonical与类型仍完全复用Model公开`PreparedModelInvocationCommitAckV1/RefV1`。同一concrete实例必须同时提供：

```text
EnsureAck(ctx, complete Model ACK) -> complete Model ACK clone
InspectExactAck(ctx, complete Model AckRef) -> complete Model ACK clone
internal InspectByPreparedCurrent(ctx, complete PreparedRef, complete CurrentRef) -> complete Model ACK clone
```

`InspectByPreparedCurrent`的stable key是对完整`PreparedRef + CurrentRef`的domain-separated canonical digest；不含Surface Binding、Checked/Expires或repository clock，因此与Model ACK ID的唯一性一致。它只作Gate lost-reply/重入恢复，不向Model、Tool、Application或插件发布第三个公共Gate方法。`EnsureAck`、`InspectExactAck`和internal lookup必须落在同一Repository实例、同一锁域与同一create-once索引；Repository原子维护`by_ack_id`、`by_prepared_current`及`by_prepared_ref`三个索引。`by_prepared_ref`保证一个Prepared epoch最多关联一个Current/ACK；同Prepared换Current即Conflict。禁止Writer/Reader分仓、cache冒充索引或第二ACK truth。

Repository语义固定为：同stable key+同canonical ACK幂等返回deep clone；同key换SurfaceBindingRef、GateImplementationRef、时间或任一内容返回Conflict；authoritative NotFound只由本实例索引给出。Unavailable、Indeterminate、timeout或decode error只能Inspect原stable key/原AckRef，不能当NotFound、不能盲Ensure、不能换时间重Seal。

Tool Binding Repository与Model ACK Repository是两个Owner、两个线性化域，跨仓不宣称原子。`Commit`或中断重入按以下顺序恢复：

1. 完整Prepared+Current通过intrinsic Validate后，第一项Owner调用必须是ACK Repository `InspectByPreparedCurrent`；若存在，先逐字段验证Prepared/Current与stored canonical，再调用Model公开Prepared Current Reader复读同一exact Current，最后取fresh Harness Owner clock验证stored ACK与Prepared Current均满足`checked <= now < expires/not_after`后返回deep clone；该命中路径零Tool、零Ensure、零重Seal，但绝不允许零clock；
2. 只有该同实例Repository对`by_prepared_current`和`by_prepared_ref`共同证明authoritative never-created，才继续重读Model Historical/Current并形成同一Tool Invocation coordinate；`by_prepared_ref`已存在其他Current时直接Conflict；
3. 调用Tool公开按Invocation recovery Reader恢复winner；仅Tool authoritative NotFound时才允许同请求`EnsureToolSurfaceInvocationBindingV1`，回包不确定后仍只Inspect原Invocation；
4. Tool winner exact后执行Assembly/Surface/Prepared S2；此后才取fresh Harness Owner clock，用Tool winner的Model-neutral SurfaceBindingRef与Harness GateImplementationRef Seal Model ACK并`EnsureAck`；
5. ACK Ensure回包不确定只以同stable key/原AckRef恢复；只有winner Tool Binding与winner Model ACK逐字段exact、时间仍current时，`Commit`才返回Model ACK。不得先取新clock/Seal候选再查ACK Repository。

## 5. Harness Assembly Current合同

### 5.1 Tool Surface与Context Injection边界

live `assemblycontract.ObjectRefV1{ID,Revision,Digest}`的`Validate()`只验证引用形状。C2只把A2 `Manifest.Plan.ToolSurface`的exact ID/Revision/Digest无损映射为Tool public `ToolSurfaceManifestCurrentRefV1`，补Tool current合同版本后调用`InspectExactToolSurfaceManifestCurrentV1`；调用方不预知ProjectionDigest，后者只在返回的完整`ToolSurfaceManifestCurrentProjectionV1`中独立重算/Validate。Tool Owner的Ensure shape必须无损保持`ToolSurfaceManifestCurrentEnsureRequestV1{ContractVersion,Manifest,ExpectedCurrent full Ref}`：create revision 1要求`ExpectedCurrent`严格零值；successor只允许当前full Ref的revision+1 CAS；current推进后重投旧revision必须Conflict/PreconditionFailed，不得回退或ABA。Harness不获得该Ensure能力。

- `ToolSurfaceManifest.ExpectedInjectionDigest`是工具表expected canonical，只能与Model `ActualToolSurfaceDigest`比较；
- Model `ActualProviderInjectionDigest`可包含ToolChoice、Parallel、hosted tools与extensions，只作为额外exact事实保存和复读，不要求与Tool expected相等；
- Context `ExpectedInjectionManifest`描述Frame/field注入，继续走独立Injection Conformance链；它不进入Tool Binding等式、Assembly composite TTL或本Gate Reader依赖；
- 本桥不新增Context Reader Delta，也不把Assembly Plan的Context ExpectedInjection ObjectRef解释为Tool expected。

### 5.2 Memory/Knowledge Delta 10/11接线审查

Harness Owner结论是“复用公开Owner Reader，不形成第二套DTO”，并保持Delta 10/11原Owner边界：

1. live唯一入口分别是`ExecutionRuntime/memory-knowledge/memory/contextsource.MemoryContextSourceCurrentReaderV1`与`ExecutionRuntime/memory-knowledge/knowledge/contextsource.KnowledgeContextSourceCurrentReaderV1`；非零Memory/Knowledge Source只能由Context Owner Adapter接收这两个公开能力，Harness不得接入、复制、alias或定义等价Reader/DTO；
2. Context Owner负责`InspectAttempt -> S1 InspectForTurn/ReadContentExact -> Context Candidate/Fragment/pending DomainResult -> S2 -> atomic Apply/Generation CAS`，并且是`SourceTurn -> TargetTurn` TransitionProof的唯一Owner；Application只协调调用顺序和exact refs，不创建、签发或解释Proof；
3. `SourceTurn=T`的完整exact ref必须来自Session/Turn Owner公开Reader；Harness、Memory、Knowledge和Application都不得按ID、ordinal或payload补造。Memory/Knowledge仍只拥有自身source/current/content事实；
4. Harness只消费Context Owner已settled、已current Inspect的final `Target/ContextTurn=T+1 Frame` exact ref；不接收SourceTurn、TransitionProof、Memory/Knowledge source ref、content、citation或retrieval payload，也不直接调用两个Owner Reader；
5. 首个G6B继续固定`MemorySources=0`、`KnowledgeSources=0`且两个Reader调用数均为0，状态仍为P0。未来非零Source只能由Memory/Knowledge Owner对各自live contextsource V1做additive演进，或发布经联合审定且替代旧入口的唯一facade；拒绝并存的第二套neutral DTO、第二current或Harness兼容桥，并须先通过Context联合Conformance。

本节只记录Harness接线审查，不发布Memory/Knowledge/Context接口，也不改变本Surface Binding等式或TTL闭包。

### 5.3 single composite current watermark

中央唯一采用`A2+B1+C2`。A2由Harness Assembly Owner在单一Store中完整保存并复读`CompileResultV1 + AssemblyBindingConformanceV1`；B1在M2集成、验收与生产中只允许注入Runtime concrete `GenerationBindingAssociationGatewayV1`提供的窄Reader，由该Gateway每次真实重建BindingSet current并复读Generation/Activation。该注入资格由sealed Handoff的exact `ProviderBindingCandidateV1`身份、`AssemblyBindingConformanceV1.Association` exact Ref、Conformance分离的BindingSet ID/revision/digest/semantic/currentness/projection字段、Provider candidate到Runtime BindingSet目标member full Ref的exact映射与完整assembly lineage共同证明，并要求实际Provider package/type identity exact是Runtime kernel Gateway。association path中Conformance `Binding/CapabilityDigest/SchemaDigests`必须为空，禁止伪造Provider Binding。Runtime public conformance只验Fact shape且`ProductionClaimEligible=false`，仅限接口单测，绝不承担fixture/集成/生产身份证明。随后独立复读Handoff current；C2只调用Tool public完整`ToolSurfaceManifestCurrentReaderV1`并由Tool唯一Repository回答；最后读取Registry exact。公共Runtime-neutral composite保持不变，Tool仍完整保存同一Projection，不自行解释或签发Owner TTL。完整合同见[Owner-current裁决](../port-deltas/model-predispatch-assembly-owner-current-v1.md)。

A2 Reader必须以unexported marker package-sealed，外部包不能自签Owner current。A2对完整CompileResult的校验固定为：Generation/Manifest/Graph分别调用`GenerationDigestV1/ManifestDigestV1/GraphDigestV1`重算并执行字段硬门；Handoff调用`Validate()`；Conformance调用`Validate(nowUnixNano)`。不得笼统调用不存在的Manifest/Graph/Generation Validate，也不得只看四个顶层digest。Store对完整nested pointer/slice/bytes做输入、保存、Historical/Current返回双向deep clone。

A2 canonical三元组固定为：

- stable ID使用domain `praxis.harness.model-predispatch-verified-assembly-owner-current-id/v1`、version `praxis.harness.model-predispatch-verified-assembly-owner-current/v1`、discriminator `ModelPreDispatchVerifiedAssemblyOwnerCurrentIdentityV1`与具名input `ModelPreDispatchVerifiedAssemblyOwnerCurrentIdentityCanonicalV1`；
- CompileDigest使用domain `praxis.harness.model-predispatch-verified-assembly-owner-current-compile/v1`、同一V1 version、discriminator `ModelPreDispatchVerifiedAssemblyOwnerCurrentCompileV1`与具名input `ModelPreDispatchVerifiedAssemblyOwnerCurrentCompileCanonicalV1`；
- ProjectionDigest使用domain `praxis.harness.model-predispatch-verified-assembly-owner-current-projection/v1`、同一V1 version、discriminator `ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1`与具名input `ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionCanonicalV1`。

三者不共用anonymous body。literal golden为ID digest `sha256:c05c7aab1acd177e819a951287120e5cbab0859d8c3eee9d8478cdab6a45f68c`、Compile digest `sha256:a833f14f767cd6083cfde17198423cdf4cd0cfdb323a40fe95db69ed0465b455`、Projection digest `sha256:93c8ffb4f5aeb21685f1a1eee9c32b156b4d28b1ddff9772e6869e468a8013e9`；完整golden input与own-digest排除规则以[Owner-current裁决§3.3](../port-deltas/model-predispatch-assembly-owner-current-v1.md)为唯一真值。

A2 HandoffRef必须以live规则现场构造并与Conformance逐字段exact：`ID=GenerationRef.ID+"/handoff"`、`Revision=GenerationRef.Revision`、`Digest=Compile.Handoff.Digest`。禁止caller自报、只比digest或用Generation digest代替Handoff digest。

### 5.4 Runtime ports唯一neutral Go nominal Port Delta

Surface DAG裁决拒绝Tool直接import Harness `assemblycontract`。唯一跨Harness/Tool的Go nominal必须新增到`ExecutionRuntime/runtime/ports`：type owner是Runtime ports；semantic、publisher、revision/current index与Reader implementation Owner仍是Harness。Runtime不得发布、CAS或解释Assembly语义，Tool不得复制/echo或按Kind猜源。

```go
type ModelPreDispatchAssemblyExactRefV1 struct {
    ID       string        `json:"id"`
    Revision core.Revision `json:"revision"`
    Digest   core.Digest   `json:"digest"`
}

type ModelPreDispatchAssemblyBindingSetRefV1 struct {
    ID                    string        `json:"id"`
    Revision              core.Revision `json:"revision"`
    Digest                core.Digest   `json:"digest"`
    SemanticDigest        core.Digest   `json:"semantic_digest"`
    CurrentnessDigest     core.Digest   `json:"currentness_digest"`
    ProjectionDigest      core.Digest   `json:"projection_digest"`
    ExpiresUnixNano       int64         `json:"expires_unix_nano"`
}

type RegistrySnapshotRefV1 struct {
    Owner           core.OwnerRef `json:"owner"`
    ContractVersion string        `json:"contract_version"`
    ID              string        `json:"id"`
    Revision        core.Revision `json:"revision"`
    Digest          core.Digest   `json:"digest"`
}

type ModelPreDispatchAssemblyCurrentRefV1 struct {
    ContractVersion string                                        `json:"contract_version"`
    ID              string                                        `json:"id"`
    Revision        core.Revision                                 `json:"revision"`
    Digest          core.Digest                                   `json:"digest"`
    Generation      GenerationArtifactRefV1                       `json:"generation"`
    Handoff         ModelPreDispatchAssemblyExactRefV1             `json:"handoff"`
    BindingSet      ModelPreDispatchAssemblyBindingSetRefV1        `json:"binding_set"`
    Manifest        ModelPreDispatchAssemblyExactRefV1             `json:"manifest"`
    Conformance     ModelPreDispatchAssemblyExactRefV1             `json:"conformance"`
    WatermarkDigest core.Digest                                   `json:"watermark_digest"`
    ToolSurface       ModelPreDispatchAssemblyExactRefV1             `json:"tool_surface"`
    ProfileDigest     core.Digest                                   `json:"profile_digest"`
    RegistrySnapshot  RegistrySnapshotRefV1                          `json:"registry_snapshot"`
    SemanticDigest    core.Digest                                   `json:"semantic_digest"`
    CurrentnessDigest core.Digest                                   `json:"currentness_digest"`
    CheckedUnixNano   int64                                         `json:"checked_unix_nano"`
    ExpiresUnixNano   int64                                         `json:"expires_unix_nano"`
    ProjectionDigest  core.Digest                                   `json:"projection_digest"`
}

type ModelPreDispatchAssemblyCurrentProjectionV1 struct {
    ContractVersion   string                                        `json:"contract_version"`
    Ref               ModelPreDispatchAssemblyCurrentRefV1          `json:"ref"`
    Generation        GenerationArtifactRefV1                       `json:"generation"`
    Handoff           ModelPreDispatchAssemblyExactRefV1             `json:"handoff"`
    BindingSet        ModelPreDispatchAssemblyBindingSetRefV1        `json:"binding_set"`
    Manifest          ModelPreDispatchAssemblyExactRefV1             `json:"manifest"`
    Conformance       ModelPreDispatchAssemblyExactRefV1             `json:"conformance"`
    ToolSurface       ModelPreDispatchAssemblyExactRefV1             `json:"tool_surface"`
    ProfileDigest     core.Digest                                   `json:"profile_digest"`
    RegistrySnapshot  RegistrySnapshotRefV1                          `json:"registry_snapshot"`
    SemanticDigest    core.Digest                                   `json:"semantic_digest"`
    CurrentnessDigest core.Digest                                   `json:"currentness_digest"`
    CheckedUnixNano   int64                                         `json:"checked_unix_nano"`
    ExpiresUnixNano   int64                                         `json:"expires_unix_nano"`
    ProjectionDigest  core.Digest                                   `json:"projection_digest"`
}

type ModelPreDispatchAssemblyCurrentReaderV1 interface {
    InspectCurrentModelPreDispatchAssemblyV1(
        context.Context,
        ModelPreDispatchAssemblyCurrentRefV1,
    ) (ModelPreDispatchAssemblyCurrentProjectionV1, error)
}
```

Runtime ports类型只承载中立exact坐标：完整Generation直接复用现有`GenerationArtifactRefV1`；Handoff/BindingSet/Manifest/Conformance/ToolSurface均是closed field-specific nominal，不含Kind；Registry统一使用`runtimeports.RegistrySnapshotRefV1`并完整保留`Owner core.OwnerRef/ContractVersion/ID/Revision/Digest`。Harness Reader返回上述类型，Tool Binding直接嵌入同一Ref/Projection，不再定义Tool neutral echo、别名或转换DTO。

ID只由稳定subject派生：`ContractVersion + Generation.ID + Handoff.ID + BindingSet.ID + Manifest.ID + Conformance.ID + ToolSurface.ID + ProfileDigest + RegistrySnapshot.{Owner,ContractVersion,ID}`；不含Revision、current digest或时间。Revision由Harness Owner单调CAS。canonical必须使用以下无环body，禁止把已计算摘要回流进自己的输入：

1. `SemanticDigest`覆盖ProfileDigest、完整`runtimeports.RegistrySnapshotRefV1{Owner core.OwnerRef,ContractVersion,ID,Revision,Digest}`、ToolSurface Ref、Generation/Handoff/BindingSet exact coordinates、Manifest/Conformance；
2. `CurrentnessDigest`精确覆盖A2 OwnerCurrent、B1 Runtime concrete Association Gateway、Handoff current、C2 ToolSurface Manifest current与Registry exact的完整expected输入和两轮返回digest。B1返回Fact中的Candidate.Binding只有在本次concrete Gateway已真实重建并exact验证BindingSet/Generation/Activation后才可进入闭包；不新增独立BindingSet Reader。Registry不伪造TTL；不接受conformance-equivalent、static、cache、self-built Reader、caller snapshot、latest或Harness私有跨Owner Reader；
3. `WatermarkDigest = domain-separated digest(ContractVersion, ID, Revision, Generation, Handoff, BindingSet, Manifest, Conformance, ToolSurface, ProfileDigest, complete RegistrySnapshot, SemanticDigest, CurrentnessDigest, CheckedUnixNano, ExpiresUnixNano)`；计算时`WatermarkDigest`、`Ref.Digest`、`Ref.ProjectionDigest`及`Projection.ProjectionDigest`均不进入body；
4. Current Ref必须逐字段重复闭合完整`Generation/Handoff/BindingSet/Manifest/Conformance/Watermark/ToolSurface/Profile/Registry/Semantic/Currentness/Checked/Expires/ProjectionDigest`，并与Projection同名字段exact相等；只给ID/Revision/Digest的缩水Ref无效；
5. `ProjectionDigest`等于完整Projection canonical digest；计算时`Projection.ProjectionDigest`、`Projection.Ref.Digest`与`Projection.Ref.ProjectionDigest`全部置空，但包含已经计算完成的`WatermarkDigest`及其余closure字段；最终`Ref.Digest = Ref.ProjectionDigest = Projection.ProjectionDigest`，own digest不得回流任何自身canonical body。

`Checked=max(all checked)`、`Expires=min(all expires)`。same ID+same revision换任一内容一律Conflict；合法推进只允许expected revision+1的CAS，在同一线性点原子写入immutable新revision并替换current index。旧revision/ref仍保留为历史审计记录，但公开`InspectCurrentModelPreDispatchAssemblyV1`/`ValidateCurrent`对旧Ref返回PreconditionFailed；不得把历史可读解释为仍current，也不得ABA回指旧revision。

## 6. Tool Ensure与逐字段映射

### 6.1 Owner-correct公开Writer

Harness只构造Tool公开`ToolSurfaceInvocationBindingEnsureRequestV1`中的exact坐标和上界，不提交完整Tool Fact，也不定义Harness CommitRequest/Commit Writer：

```go
toolcontract.ToolSurfaceInvocationBindingWriterV1.
    EnsureToolSurfaceInvocationBindingV1(
        ctx,
        toolcontract.ToolSurfaceInvocationBindingEnsureRequestV1,
    ) -> (toolcontract.ToolSurfaceInvocationBindingV1, toolcontract.ToolSurfaceInvocationBindingAckV1)
```

Tool Repository使用自己的可信Clock生成`CreatedUnixNano`，只对request中逐字段可追溯到Owner current/Fact或caller deadline的显式上界取min，生成`NotAfterUnixNano`并Seal Fact/Ack，在Invocation与Binding ID两个索引上原子create-once。Harness只调用Tool公开`ValidateAgainst(request, now)`；不得调用Tool Seal函数或覆写Owner时间。回包丢失只Inspect同Invocation coordinate；Unavailable/Indeterminate/NotFound不允许盲重提或进入Provider。

Tool公开Ensure request/Fact不得携Context `ExpectedInjectionManifest`作为Tool等式；必须无损携完整Prepared Ref、Prepared Historical Fact、Prepared Current Ref/Projection、Historical NotAfter、完整Registry Snapshot Ref、`ActualToolSurfaceDigest`、`ActualProviderInjectionDigest`和完整Harness composite Projection。现有Tool候选若仍使用单一`ActualInjectionDigest`、单Registry digest或Context-named Expected ref，继续视为联合Port Delta，Harness不得本地翻译掩盖。

Tool公开Exact Reader必须以Model ACK中的公开中立`SurfaceBindingRef` nominal作为唯一参数，按`Owner + ContractVersion + ID + Revision + Digest`单参读取winner Binding；不得要求调用方同时提供Tool BindingRef/AckRef、Kind、Invocation或私有key。Harness只把Tool winner逐字段无损映射为该Model neutral Ref：Owner固定为Tool Binding Owner、ContractVersion固定为Tool公开Binding合同、ID/Revision/Digest逐字段复制；不得从Kind猜Owner/Contract，也不得用Owner/Contract反推或替换ID/Digest。Tool public adapter收到neutral Ref后先exact验证Owner/Contract，再由Tool Owner内部索引读取；Model/Harness不获得Tool Store能力。

### 6.2 composite无损映射与包DAG裁决

Tool直接import Harness `assemblycontract`会与未来`harness -> tool-mcp` Gate依赖形成module SCC，因此不批准。替代方案不是Tool echo，而是第5.4节Runtime ports唯一neutral nominal：`harness -> runtime/ports`与`tool-mcp -> runtime/ports`保持DAG，Runtime只拥有type、不拥有语义或发布权。

Harness Publisher/Reader直接产生并返回`runtimeports.ModelPreDispatchAssemblyCurrentRefV1/ProjectionV1`；Tool Binding直接嵌入同一完整Projection，不定义Tool Ref/Projection/Kind/alias。映射必须逐字段无损覆盖完整Generation、Handoff、BindingSet、Manifest、Conformance、Watermark、ToolSurface、Profile、Registry、Semantic、Currentness、Checked、Expires与ProjectionDigest；Tool `ValidateAgainst`直接验证该Runtime public Projection及原Harness Reader current结果。漏字段、Kind转换、generic ObjectRef、裸digest或map顺序映射均Conflict。

### 6.3 逐字段mapping表

| Model/Harness源 | Tool request目标 | exact规则 |
|---|---|---|
| Model完整`PreparedModelInvocationRefV1` | `PreparedRef`与`Invocation` | ContractVersion/ID/Revision/Digest/InvocationID/InvocationDigest/UnifiedRequestDigest全字段无损映射 |
| Historical Fact `PreparedPlanDigest/RequestToolsDigest/RouteDigest/ProfileDigest` | Tool Prepared historical coordinate | 使用Model Fact公开字段逐字节映射；不得伪造成Harness Ref、不得从Current补签 |
| Historical完整`runtimeports.RegistrySnapshotRefV1` | Tool Binding直接嵌入同一RegistrySnapshot exact ref | `Owner core.OwnerRef/ContractVersion/ID/Revision/Digest`无损复用，不降级为单Digest或string Owner |
| Historical `NotAfter` | Tool资格绝对上界 | Current Expires和Tool NotAfter都不得超过 |
| Model完整`PreparedModelInvocationCurrentRefV1` | Tool Prepared Current exact ref | ContractVersion/ID/Revision/Digest/Prepared/Checked/Expires/NotAfter逐字段无损映射；禁止latest lookup |
| Current `ActualToolSurfaceDigest` | Tool Binding actual tool surface digest | exact等于Tool Surface expected digest |
| Current `ActualProviderInjectionDigest` | Tool Binding provider actual digest | 原样保存，不与Tool expected或Context expected比较 |
| Current `Checked/Expires` | Tool Prepared Current窗口 | exact；Expires参与min且不超过Historical NotAfter |
| Assembly `ToolSurface` | `AssemblyCurrent.AssemblyToolSurface` | ID/Revision/Digest exact；Tool Surface Reader复读 |
| Assembly `ProfileDigest` | Assembly/Subject profile | exact等于Prepared/Surface |
| Surface完整RegistrySnapshot Ref | Tool Binding registry snapshot | Owner/ContractVersion/ID/Revision/Digest exact等于Historical RegistrySnapshot Ref |
| Runtime ports Assembly composite Ref/Projection | Tool Binding直接嵌入同一public Projection | 零转换；全部冻结字段逐字段exact；Tool不得定义echo/Kind、拆分、漏字段或重签currentness |
| Gate fresh caller上界 | `RequestedNotAfterUnixNano` | 只允许缩短；Harness不提供Created/Fact NotAfter |

共同上界只包括Historical NotAfter、Prepared Current Expires/NotAfter、Surface Current Expires、Harness composite Assembly Current Expires与caller deadline的最小值。Assembly composite内部只封存Generation/Handoff/BindingSet的共同min。Harness Gate必须在Commit前和返回后使用fresh clock检查，不重用旧`now`。

## 7. live actual-point no-bypass表

| live路径 | 当前Gate前触达点 | 冻结改造 |
|---|---|---|
| `execution.Runtime.Start` | `registry.Resolve`后直接`Adapter.Preflight` | 先纯Preparation+Commit；Preflight前重新Inspect current |
| `execution/direct.Adapter.Preflight` | `Backend.Resolve` | Resolve移到ACK后；不得参与Phase A |
| `execution/direct.Adapter.Open` | `Backend.OpenStream`或后续Invoke | 每次Open/Invoke/Stream前Inspect同Ack与current |
| generic `Invoker.prepare` | `registry.Get`后`provider.Capabilities` | 使用sealed registry snapshot做Phase A；动态Capabilities在ACK后只校验，不改Tools或双actual digest |
| `routegateway.Gateway` | `Capabilities/Invoke/Stream` | 每次Provider lease actual call前统一guard |
| `operation.Invoker.prepare` | `provider.Capabilities` | 同generic规则；operation/composite子Provider也不得旁路 |
| direct continuation | `session.backend.OpenStream/Invoke` | 每次continuation首调前Inspect；验证Tools与双actual digest不变，否则新Invocation epoch |
| realtime `Provider.Open` | WebSocket `DialContext` | Open前Inspect；ACK/current过期则零dial |
| hosted/remote/provider adapters | 各Adapter `Preflight/Open/Invoke/Stream` | Conformance枚举所有底层入口并注入同一guard |

所有tool-bearing底层dispatch都必须拿到同一Invocation的exact Ack Ref作为输入或由受控session不可变持有；仅在上层构造governed ModelTurn不够。任何raw provider、direct backend、continuation或realtime路径没有guard都使Generation Conformance失败。

## 8. Harness Gate adapter

候选落点：

```text
ExecutionRuntime/harness/assemblyadapter/model_predispatch_assembly_current_v1.go
ExecutionRuntime/harness/modelinvokeradapter/prepared_model_invocation_ack_repository_v1.go
ExecutionRuntime/harness/modelinvokeradapter/predispatch_surface_commit_gate_v1.go
```

构造器只注入exact Assembly Current Ref、Model公开Prepared Reader、Model公开Prepared Exact Current Reader、Harness Assembly Current Reader、Tool Surface Current Reader、Tool公开Binding Writer/Reader、同一个Harness-owned ACK Repository实例与单调Clock。ACK Ensure/Exact Inspect/internal stable-key Inspect不得拆成不同注入项或不同concrete。typed nil、nil context、缺Reader、缺ACK Repository、缺Clock都在任何Owner调用前Fail Closed。

`Commit`步骤固定为：intrinsic Validate完整Prepared+Current → 第一项Owner调用是ACK Repository `InspectByPreparedCurrent`；命中则exact验证stored canonical、复读同一Prepared Current并用fresh Owner clock验证ACK+Current freshness，成功后返回clone，零Tool/Ensure/重Seal，过期即PreconditionFailed → authoritative never-created才读取Historical/Current与Assembly/Surface S1 → 比较Tool expected与ActualToolSurface → 优先按原Invocation恢复Tool winner，Tool authoritative NotFound才调用`EnsureToolSurfaceInvocationBindingV1` → lost reply只Inspect原Invocation → Tool Fact/Ack `ValidateAgainst` → S2按原refs复读 → 此后才取fresh Harness Owner clock Seal Model ACK并`EnsureAck` → Ensure丢回复只按原stable key/AckRef恢复 → 返回Model公开ACK。`InspectExactAck`只从同一ACK Repository按完整AckRef读取deep clone，不触发Tool Ensure、clock或任何写入。Provider实际调用点还必须再次执行独立的`InspectCurrentBeforeProviderAttempt`；Gate S2不能替代attempt前复读。

## 9. Assembly接线与兼容

1. `ModelTurnRequest/Result/Port` V1字段与digest不变；additive wiring位于受治理ModelTurn实现及底层actual-point guard；
2. 新专用Capability固定为`praxis.harness/model-predispatch-surface-commit-gate-v1`，`one_active_binding`，不得新增万能Hook；
3. `model.dispatch.before`只声明阶段；Conformance必须证明具体Gate PortSpec、Factory output、Binding、Ack guard和所有provider injection edge；
4. Route V2保持Tool actual-point链不变；本Gate不替代Runtime Permit/Enforcement、Tool Action Route或Settlement；
5. P3 Application Assembler、Application V2 Request/Input和PendingAction不增加Surface字段；Tool P4以后按Model Invocation Inspect Tool Binding；
6. 旧Generation不自动获得能力；未完成两阶段重构或存在任一旁路时Fail Closed；
7. test-only组合不冒充production root。

## 10. 恢复与硬反例

- Prepared historical仍可读但Current过期：Binding历史可Inspect，Commit与Provider attempt拒绝；
- Harness定义缩水Historical/Plan/Tools/Route/Profile/Registry/Current DTO，或仅凭裸digest重建Model对象：合同审查直接拒绝；
- Registry exact ref缺Owner/ContractVersion/ID/Revision/Digest任一项，或Current Ref缺Prepared/Checked/Expires/NotAfter任一项：零Commit、零Provider；
- 同时存在第二个Harness Surface composite current，或Tool拆分后重新签发Generation/Handoff/BindingSet currentness：Conflict；
- 在显式Owner/caller上界之外注入匿名NotAfter上界：合同无法表达并拒绝；
- Harness或Application另建Memory/Knowledge Reader/DTO、直连Owner Reader或接收source/body/citation：Conformance拒绝；
- Tool Ensure正常提交但回包丢失：只Inspect原Invocation；不重新提交不同请求；
- Tool winner已存在、Model ACK尚未Ensure时进程中断：重入第一项Owner调用仍先按完整Prepared+Current执行`InspectByPreparedCurrent`；仅authoritative never-created后才恢复同Tool winner并Ensure同canonical ACK，不要求跨仓回滚或原子事务；
- ACK Ensure回包丢失：只Inspect同stable key/原AckRef；same key换SurfaceBindingRef、GateImplementationRef或Owner时间Conflict；Unavailable/Indeterminate不盲写；
- 构造器分别注入ACK Writer/Reader、两个Repository实例或cache index：Unavailable/Conformance拒绝；
- Tool Exact Reader要求BindingRef+AckRef双参、Kind或Invocation，或Harness按Kind猜neutral SurfaceBindingRef：合同审查拒绝；
- Tool复制、别名化或重签Runtime neutral composite，或遗漏Manifest/Conformance/Watermark/Profile/完整Registry Ref/任一digest或时间字段：合同审查拒绝，零ACK、零Provider；
- 同Invocation不同Surface/Injection/watermark：Conflict，不能创建第二Fact；
- Ack有效但attempt前任一Owner current跨TTL：Provider=0；
- continuation只改Input/State且Tools与双actual digest不变：可在重新Inspect后继续；变化则拒绝并要求新Invocation epoch；
- caller提交已Seal Tool Fact、Created或NotAfter：接口层无法表达并拒绝；
- ToolSurface ObjectRef格式有效但Owner对象不存在、digest漂移或Reader不可用：零Commit、零Provider；
- caller直接提交shape合法的Manifest/Conformance/ToolSurface/Profile，未能由同一verified Compile/Manifest/Conformance Reader闭合：零Projection、零CAS；
- Association仍active但任一BindingSet成员Grant、member TTL、Generation或Activation已漂移：Runtime canonical Gateway重建current时拒绝，零Handoff/C2/Registry调用、零CAS；
- A2完整CompileResult/Conformance的任一nested pointer/slice/bytes被输入方或读取方修改：Store历史/current不变；
- recovery只取Owner TTL而没有独立host hard cap：合同测试拒绝；
- Handoff current Seal收到错误非零ContractVersion却覆盖为当前版本：合同测试拒绝；
- sealed capability snapshot与ACK后Provider Capabilities不一致且会改变Injection：拒绝当前Invocation；
- Binding/Ack不得升级为Provider进入权、Authority、Permit、Evidence、DomainFact或Settlement。

## 11. 实现门

Harness Go只能在以下条件全部满足后开始：

- Model Prepared Fact/Current Reader、两阶段actual-point Gate/Ack设计独立YES；
- Model公开actual-point inventory nominal/Reader与closed Kind集合尚未冻结；详见[actual-point inventory Port Delta](../port-deltas/model-predispatch-actual-point-inventory-v1.md)。在该Delta及Model actual-point代码审计YES前，Harness不得用Phase声明、静态源码扫描或自报清单冒充production no-bypass；
- Tool公开Writer/Reader与`EnsureToolSurfaceInvocationBindingV1`完成独立YES；
- Harness-owned Model ACK同实例Repository、create-once/recovery canonical与跨仓非原子恢复顺序完成独立YES；
- Prepared Fact/Current双Ref双Reader、Tool/Provider双actual digest与single composite Assembly watermark冻结；
- [Owner-current A2+B1+C2裁决](../port-deltas/model-predispatch-assembly-owner-current-v1.md)取得联合设计YES；Harness A2 Store/Reader、Runtime concrete Gateway的Association exact Ref + Conformance分离BindingSet字段 + sealed ProviderCandidate→Runtime BindingSet member exact composition proof、Tool C2完整Manifest窄Reader/唯一Repo及真实多Owner fixture分别落盘、compile并审计YES；Runtime public conformance仅作Reader接口单测，不承担production proof；
- 本Harness设计三Owner联合YES；
- live no-bypass表所有路径有可验证接线方案；
- 测试组合明确不冒充production root。

当前结论：M2 `A2+B1+C2`与Harness concrete Gate/ACK Repository已经完成Owner-local实现、测试及独立代码审计；该YES只证明Harness拥有的Current/Gate/恢复边界。live Model仍须在每个Provider attempt、Open/Stream、continuation与realtime actual-point前强制复读exact ACK/current，Harness Assembly仍须证明专用required Capability、Factory/Binding和完整actual-point inventory。上述两项以及Tool V2 Consumer、system G6A、Capability启用与production root继续`NO-GO`。
