# Model PreDispatch Assembly Owner-current V1裁决

## 1. 状态与唯一方案

中央M2裁决唯一采用`A2+B1+C2`：

- **A2**：Harness Assembly Owner拥有完整Verified Assembly OwnerCurrent Store/Reader；
- **B1**：复用Runtime公开`GenerationBindingAssociationCurrentReaderV1`及其canonical Gateway；Gateway每次Inspect真实重建BindingSet current，不新增BindingSet Reader；
- **C2**：复用Tool public contract的完整`ToolSurfaceManifestCurrentReaderV1`，current语义与唯一Repository只归Tool Owner。

该Delta对应M2 Go已完成Owner-local实现/测试并通过独立代码审计；它只闭合Assembly Owner-current，不证明Model actual-point no-bypass，也不解锁Tool V2 Consumer、Capability、system G6A或production root。

## 2. Owner与无环DAG

| 能力 | type/semantic Owner | Harness M2注入能力 | 禁止 |
|---|---|---|---|
| Verified Assembly OwnerCurrent | Harness Assembly Owner | `ModelPreDispatchVerifiedAssemblyOwnerCurrentReaderV1` | Runtime/Tool复制、caller raw refs冒充Owner current |
| Generation-Binding Association current | Runtime Owner | `runtimeports.GenerationBindingAssociationCurrentReaderV1` | 接收GovernancePort、另建BindingSet Reader、信历史Fact不走Gateway |
| Handoff current | Harness Handoff Owner | 既有窄Reader | 用A2内嵌Handoff替代独立current复读 |
| ToolSurface Manifest current | Tool Owner | Tool public `ToolSurfaceManifestCurrentReaderV1` | Harness定义Tool DTO/Repo、接收Tool Writer、直接import Tool实现 |
| Registry exact | Registry Owner | `runtimeports.RegistrySnapshotExactReaderV1` | digest-only、latest、扫描或Harness重签 |

```text
Runtime ports <- Runtime canonical Association Gateway
Harness A2 Store/Reader -> Harness M2 publisher
Tool public contract <- Tool unique Surface Repository
host test composition -> injects narrow readers only
```

Harness不新增Runtime或Tool nominal，不形成Harness↔Tool module SCC。

## 3. A2：Harness完整Verified Assembly OwnerCurrent

### 3.1 完整对象

```go
const ModelPreDispatchVerifiedAssemblyOwnerCurrentContractVersionV1 =
    "praxis.harness.model-predispatch-verified-assembly-owner-current/v1"

type ModelPreDispatchVerifiedAssemblyOwnerCurrentRefV1 struct {
    ID       string        `json:"id"`
    Revision core.Revision `json:"revision"`
    Digest   core.Digest   `json:"digest"`
}

type ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1 struct {
    ContractVersion  string                                                 `json:"contract_version"`
    Ref              ModelPreDispatchVerifiedAssemblyOwnerCurrentRefV1      `json:"ref"`
    Compile          assemblycontract.CompileResultV1                       `json:"compile"`
    CompileDigest    core.Digest                                            `json:"compile_digest"`
    Conformance      assemblycontract.AssemblyBindingConformanceV1          `json:"conformance"`
    CheckedUnixNano  int64                                                  `json:"checked_unix_nano"`
    ExpiresUnixNano  int64                                                  `json:"expires_unix_nano"`
    ProjectionDigest core.Digest                                            `json:"projection_digest"`
}

type ModelPreDispatchVerifiedAssemblyOwnerCurrentReaderV1 interface {
    InspectCurrentModelPreDispatchVerifiedAssemblyOwnerV1(
        context.Context,
        ModelPreDispatchVerifiedAssemblyOwnerCurrentRefV1,
    ) (ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1, error)
    modelPreDispatchVerifiedAssemblyOwnerCurrentReaderV1()
}

type ModelPreDispatchVerifiedAssemblyOwnerCurrentStoreV1 interface {
    ModelPreDispatchVerifiedAssemblyOwnerCurrentReaderV1
    EnsureModelPreDispatchVerifiedAssemblyOwnerCurrentV1(
        context.Context,
        ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1,
    ) (ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1, error)
    CompareAndSwapModelPreDispatchVerifiedAssemblyOwnerCurrentV1(
        context.Context,
        ModelPreDispatchVerifiedAssemblyOwnerCurrentRefV1,
        ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1,
    ) (ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1, error)
    InspectHistoricalModelPreDispatchVerifiedAssemblyOwnerV1(
        context.Context,
        ModelPreDispatchVerifiedAssemblyOwnerCurrentRefV1,
    ) (ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1, error)
}
```

Reader通过unexported marker做package-sealed；只有`ExecutionRuntime/harness/assemblyadapter`内的Harness Owner实现可满足。外部fixture、caller或其他Owner不能实现自签Reader再注入Publisher；测试必须使用本package concrete Store。

Store是Harness Owner唯一线性化点：immutable `(ID,revision)` history、stable ID current index、full Ref CAS、禁止ABA。初次Ensure只接受revision 1；same canonical幂等，same ID/revision换内容Conflict；successor只允许expected revision+1。丢回复只Inspect同一完整Ref，不重建、重Seal或换时间。

### 3.2 完整lineage

`Compile.Generation/Manifest/Graph/Handoff`必须全部非nil。live只有Handoff提供`Validate()`，Conformance提供`Validate(nowUnixNano)`；A2不得调用或宣称不存在的Manifest/Graph/Generation Validate。固定校验为：

- 以`assemblycontract.GenerationDigestV1(*Compile.Generation)`重算并exact等于`Generation.Digest`，同时硬校验ContractVersion、GenerationID、Revision、CompilerVersion、Created、State、Input/Manifest/Graph/Diagnostic/Residual及Previous/Evidence字段闭合；
- 以`assemblycontract.ManifestDigestV1(*Compile.Manifest)`重算并exact等于`Manifest.Digest`，同时硬校验ContractVersion、Input/Catalog、Plan、Policy、CurrentFacts/RouteBindings及所有descriptor/contribution/dependency/factory/provider/residual集合；
- 以`assemblycontract.GraphDigestV1(*Compile.Graph)`重算并exact等于`Graph.Digest`，同时硬校验ContractVersion、Input/Catalog、DependencyOrder、Slots/Phases、PortSpecRefs与FactoryRefs；
- 调用`Compile.Handoff.Validate()`；
- 调用`Conformance.Validate(nowUnixNano)`，禁止只重算digest而忽略Current/Observed/Expires。

上述各项通过后，A2还必须逐字段证明：

1. Generation只把ID/revision/digest/input/manifest/graph无损映射到本链使用的Runtime Generation exact坐标；本裁决不从Generation映射或推导Catalog；
2. `Generation.ManifestDigest==Manifest.Digest`、`Generation.GraphDigest==Graph.Digest`；
3. Handoff的GenerationRef、ManifestDigest、GraphDigest、CatalogDigest与上述完整对象exact；
4. Manifest `InputDigest`与Generation/Graph/Handoff exact；Catalog只由Manifest、Graph、Handoff三方`CatalogDigest` exact闭合；
5. ToolSurface唯一来自`Manifest.Plan.ToolSurface`完整`ObjectRefV1`；Profile唯一来自`Manifest.Plan.Profile.Digest`；
6. Conformance的HandoffRef、GenerationRef、Input/Manifest/Graph/Catalog、BindingSet ID/revision/digests与上述对象及其Association exact；
7. Conformance必须`Current=true`，`CheckedUnixNano==Conformance.ObservedUnixNano`，`ExpiresUnixNano==Conformance.ExpiresUnixNano`；
8. Diagnostics、Residuals、ComponentManifests、ProviderCandidates及所有nested canonical内容完整保留，不能只保存摘要。

A2只证明Harness verified compile/conformance lineage；它不替代B1 Runtime current、Handoff current或C2 Tool current。

### Live implementation residual: Handoff current exact lookup

当前只有`ModelPreDispatchAssemblyHandoffCurrentReaderV1`消费接口，尚无可作为生产接线证据的Owner backend。M2请求携带的exact Handoff坐标是`GenerationID + "/handoff"`、Generation revision与Handoff digest；`assemblypublication.OwnerStoreV2`却只能按`ScopeRef`读取current，或按完整`AssemblyPublicationRefV2`读取history。该Handoff坐标不携`ScopeRef`、`InputDigest`或Publication digest，无法无损调用现有Store。

因此本轮明确禁止：

- 以进程内`Handoff -> scope/publication` sidecar、可逆字符串猜测或全表扫描补定位；
- 把A2内嵌Handoff当成独立current复读结果；
- 为了让测试通过而注入静态Reader并宣称production wiring。

后续必须由Harness Assembly Publication/Handoff Owner先冻结并实现单一权威exact索引/窄Reader：索引与Publication current在同一Owner事务内更新；输入绑定完整Handoff exact Ref，输出同时证明对应historical bundle与当前Publication，执行S1/Owner reread/S2、fresh clock和最短TTL；unknown、多重映射、superseded Ref、digest漂移、clock rollback与TTL crossing全部Fail Closed。该Delta闭合前，M2 durable Store只可标记Owner-local backend，不可作为完整production current闭包。

### 3.3 canonical与deep clone

canonical必须使用以下三个具名input，不允许anonymous struct、共用domain或discriminator alias：

```go
type ModelPreDispatchVerifiedAssemblyOwnerCurrentIdentityCanonicalV1 struct {
    ContractVersion string `json:"contract_version"`
    GenerationID    string `json:"generation_id"`
    HandoffID       string `json:"handoff_id"`
}

type ModelPreDispatchVerifiedAssemblyOwnerCurrentCompileCanonicalV1 struct {
    Compile assemblycontract.CompileResultV1 `json:"compile"`
}

type ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionCanonicalV1 struct {
    ContractVersion string                                                 `json:"contract_version"`
    Ref             ModelPreDispatchVerifiedAssemblyOwnerCurrentRefV1      `json:"ref"`
    Compile         assemblycontract.CompileResultV1                       `json:"compile"`
    CompileDigest   core.Digest                                            `json:"compile_digest"`
    Conformance     assemblycontract.AssemblyBindingConformanceV1          `json:"conformance"`
    CheckedUnixNano int64                                                  `json:"checked_unix_nano"`
    ExpiresUnixNano int64                                                  `json:"expires_unix_nano"`
}
```

| 产物 | domain | version | discriminator | input/own-digest规则 |
|---|---|---|---|---|
| stable ID digest | `praxis.harness.model-predispatch-verified-assembly-owner-current-id/v1` | `praxis.harness.model-predispatch-verified-assembly-owner-current/v1` | `ModelPreDispatchVerifiedAssemblyOwnerCurrentIdentityV1` | 完整`IdentityCanonicalV1`；ID=`"mpva-owner-current:v1:" + digest` |
| `CompileDigest` | `praxis.harness.model-predispatch-verified-assembly-owner-current-compile/v1` | 同上 | `ModelPreDispatchVerifiedAssemblyOwnerCurrentCompileV1` | 完整deep-cloned `CompileResultV1`；保留内嵌sealed digest，不做nil/empty、排序或字段投影改写 |
| `ProjectionDigest` | `praxis.harness.model-predispatch-verified-assembly-owner-current-projection/v1` | 同上 | `ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1` | 完整`ProjectionCanonicalV1`；`Ref.Digest=""`，`ProjectionDigest`不在input中 |

literal golden固定为：

- stable input=`{ContractVersion:"praxis.harness.model-predispatch-verified-assembly-owner-current/v1",GenerationID:"golden-generation",HandoffID:"golden-generation/handoff"}`；digest=`sha256:c05c7aab1acd177e819a951287120e5cbab0859d8c3eee9d8478cdab6a45f68c`；ID=`mpva-owner-current:v1:sha256:c05c7aab1acd177e819a951287120e5cbab0859d8c3eee9d8478cdab6a45f68c`；
- Compile canonical-only input=`CompileResultV1{Generation:nil,Manifest:nil,Graph:nil,Handoff:nil,Diagnostics:nil,Residuals:nil}`；digest=`sha256:a833f14f767cd6083cfde17198423cdf4cd0cfdb323a40fe95db69ed0465b455`；该向量只锁canonical frame，不是可发布Compile；
- Projection canonical-only input=`{ContractVersion:<V1>,Ref:{ID:<上述golden ID>,Revision:1,Digest:""},Compile:<上述zero Compile>,CompileDigest:<上述compile golden>,Conformance:zero AssemblyBindingConformanceV1,CheckedUnixNano:1,ExpiresUnixNano:2}`；digest=`sha256:93c8ffb4f5aeb21685f1a1eee9c32b156b4d28b1ddff9772e6869e468a8013e9`；该向量只锁canonical frame，发布仍必须先通过全部lineage/current验证。

- stable ID仅由上述Identity canonical派生，不含revision、digest或时间；
- `CompileDigest`只使用上述Compile canonical；
- `ProjectionDigest`只使用上述Projection canonical；
- 最终`Ref.Digest==ProjectionDigest`；同ID/revision任一nested内容变化均Conflict；
- Store输入、history/current保存、Ensure/CAS返回及Historical/Current Inspect返回都必须deep clone。

HandoffRef exact映射不得由caller提供或另行派生：`ID=Compile.Handoff.GenerationRef.ID+"/handoff"`、`Revision=Compile.Handoff.GenerationRef.Revision`、`Digest=Compile.Handoff.Digest`，且必须逐字段exact等于`Conformance.HandoffRef`。由于`Compile.Handoff.GenerationRef`又必须exact等于`Conformance.GenerationRef`，因此该规则等价于`HandoffRef.ID=Conformance.GenerationRef.ID+"/handoff"`、revision相等、digest来自完整Handoff。任一ID后缀、revision或digest错配都在A2 Store写入前Fail Closed。

deep clone闭集至少包含Compile四个pointer、Generation PreviousGenerationRef/EvidenceRefs、Manifest全部slice与其nested slice/bytes/pointer、Graph slices、Diagnostics/Residuals、Conformance Association/GovernanceExtension pointer、SchemaDigests/Diagnostics及所有Runtime manifest nested集合。调用方修改输入或返回值不能污染Store。

## 4. B1：Runtime canonical Association Gateway

Harness M2构造器只接收：

```go
runtimeports.GenerationBindingAssociationCurrentReaderV1
```

禁止接收`GenerationBindingAssociationGovernancePortV1`。M2集成、验收、production composition与canonical fixture中的`GenerationBindingAssociationCurrentReaderV1`必须由Runtime concrete `GenerationBindingAssociationGatewayV1`提供；不得用conformance-equivalent、自建、static、cache或wrapper Reader替代。该注入资格不由Runtime public conformance证明：集成/生产composition必须同时持有并验证已sealed Handoff中的exact `ProviderBindingCandidateV1`身份、`AssemblyBindingConformanceV1.Association` exact Ref、Conformance分离的`BindingSetID/BindingSetRevision/BindingSetDigest/BindingSetSemanticDigest/BindingSetCurrentnessDigest/BindingSetProjectionDigest`字段，以及连接Handoff/Association/Generation/BindingSet/Capability/Schema的完整assembly lineage。association path中`AssemblyBindingConformanceV1.Binding/CapabilityDigest/SchemaDigests`必须保持严格空值，不得为证明Gateway而伪造Provider Binding。composition还必须将sealed Provider candidate的`CandidateID/ModuleRef/SlotRef/PortSpecRef/ProviderRef/Digest`与Runtime concrete Gateway重建的BindingSet中目标member full `ProviderBindingRefV2`及其Component Manifest/Artifact/Capability做exact映射，并要求实际Provider的Go package/type identity exact等于Runtime kernel `GenerationBindingAssociationGatewayV1`。任一身份、Association、BindingSet字段、member mapping或assembly lineage缺失/漂移均拒绝组装。仅Reader接口自身的isolated unit test可以使用通过公开conformance的等价实现；`ProductionClaimEligible=false`必须保留并视为“不具生产证明力”，该测试不构成M2 fixture、集成或生产资格。concrete Gateway在每次Inspect中：

1. 读取并Validate authoritative Association Fact；
2. 复读Generation current；
3. 调用Runtime `BuildGenerationBindingSetCurrentProjectionV1`从完整Binding facts真实重建BindingSet current；
4. 复读Activation current；
5. exact比较Candidate内Generation/BindingSet/Activation projection digests并验证共同expiry。

因此M2不定义任何额外BindingSet Reader。Association Fact中的Candidate.Binding只有经过本次Gateway current Inspect成功后才可使用；直接Fact store、static fake或历史快照均不符合B1。

Harness再逐字段证明A2 Conformance.Association等于B1 Fact Ref，A2 Generation等于B1 Candidate.Generation.Generation，A2 Conformance的完整BindingSet字段等于B1 Candidate.Binding。S1/S2返回Fact必须完整exact；BindingSet换代、撤销、member drift、Generation/Activation漂移由Gateway先Fail Closed。

## 5. C2：Tool完整Surface Manifest current

Tool public contract须发布窄能力：

```go
type ToolSurfaceManifestCurrentReaderV1 interface {
    InspectExactToolSurfaceManifestCurrentV1(
        context.Context,
        ToolSurfaceManifestCurrentRefV1,
    ) (ToolSurfaceManifestCurrentProjectionV1, error)
}

type ToolSurfaceManifestCurrentEnsureRequestV1 struct {
    ContractVersion string                                  `json:"contract_version"`
    Manifest        ToolSurfaceManifest                     `json:"manifest"`
    ExpectedCurrent ToolSurfaceManifestCurrentRefV1         `json:"expected_current"`
}

type ToolSurfaceManifestCurrentRepositoryV1 interface {
    ToolSurfaceManifestCurrentReaderV1
    EnsureExactToolSurfaceManifestCurrentV1(
        context.Context,
        ToolSurfaceManifestCurrentEnsureRequestV1,
    ) (ToolSurfaceManifestCurrentProjectionV1, error)
}
```

Harness M2只注入`ToolSurfaceManifestCurrentReaderV1`，不能获得Ensure。Tool唯一`ToolSurfaceManifestCurrentRepositoryV1`通过完整`ToolSurfaceManifestCurrentEnsureRequestV1{ContractVersion,Manifest,ExpectedCurrent}`接收Manifest与current CAS前置：`Manifest.Revision==1`时`ExpectedCurrent`必须为严格零值，且只能在current authoritative NotFound时create；`Manifest.Revision>1`时`ExpectedCurrent`必须是同ID的完整current Ref，并满足`Manifest.Revision==ExpectedCurrent.Revision+1`，Repository在单一线性化点写immutable history并以full Ref CAS替换current index。same request仅在winner仍为current时幂等；current已推进后重投旧revision必须Conflict/PreconditionFailed，不得返回历史winner、回退current或ABA；lost reply只能Inspect原full Ref。

C2调用坐标只能由A2 `Compile.Manifest.Plan.ToolSurface`逐字段映射为`ToolSurfaceManifestCurrentRefV1{ContractVersion,ID,Revision,Digest}`；ID/Revision/Digest无损来自Plan，ContractVersion使用Tool public current合同常量。调用方不提供、也不需要预知`ProjectionDigest`。`ProjectionDigest`仅存在于Reader返回的`ToolSurfaceManifestCurrentProjectionV1`，由Harness在返回后独立重算/Validate，禁止把它混入lookup key或用它替换Manifest digest。

每次读取必须返回完整Manifest、Owner、Checked/Expires与ProjectionDigest。Harness逐字段验证：

- lookup Ref与返回Ref的ID/revision/digest均exact等于A2 `Manifest.Plan.ToolSurface`；返回ProjectionDigest另行验证，不参与lookup；
- Manifest `ID/Revision/Digest`等于Ref，Owner等于Projection.Owner；
- Manifest/ProfileDigest等于A2 `Manifest.Plan.Profile.Digest`；
- `ExpiresUnixNano==Manifest.ExpiresUnixNano`且`checked<=now<expires`；
- 完整entries、schemas、effects、expected injection、capability/registry digests与residuals均由Tool Validate/canonical覆盖。

caller裸ToolSurface Ref、Harness私有echo或只返回digest的Reader均无效。

## 6. 固定S1/S2与TTL

每一轮严格按以下顺序，禁止并行、换序或缓存绕过：

```text
A2 Verified Assembly OwnerCurrent
  -> B1 Runtime GenerationBindingAssociation Current Gateway
  -> Harness Handoff Current
  -> C2 ToolSurfaceManifest Current
  -> Runtime Registry Exact
```

流程为：request/context intrinsic validate → fresh `nowS1` → 完整S1 → fresh `nowS2`且`nowS2>=nowS1` → 完整S2 → 两轮逐字段exact → fresh final clock → TTL计算 → Seal composite → Harness full-Ref CAS。任一Reader unavailable/typed nil、返回漂移、clock rollback、TTL crossing或ctx取消均在CAS前Fail Closed。

TTL固定：

- `Checked=max(A.Checked, B.Candidate.Binding.IssuedUnixNano, Handoff.Checked, C.Checked)`；
- `Expires=min(A.Expires, B.Fact.ExpiresUnixNano, Handoff.Expires, C.Expires)`；B Fact expiry已由Runtime canonical覆盖Generation、BindingSet、Activation及Requested expiry；
- Registry Exact Reader没有公开TTL，不伪造窗口；
- final clock必须满足`checked<=final<expires`；
- composite CurrentnessDigest覆盖A/B/Handoff/C/Registry两轮完整expected输入与返回Ref/Projection/Fact digest，不能覆盖raw caller字段。

## 7. canonical Gateway composition proof与真实fixture

Harness M2验收fixture必须使用独立Owner concrete：

1. Harness concrete A2 Store/Reader，真实history/current/CAS/deep clone；
2. Runtime concrete `GenerationBindingAssociationGatewayV1`，连接真实测试Binding Fact Store、Generation current、Activation current和Owner clock；fixture启动时必须验证Provider package/type identity exact是Runtime concrete Gateway，`AssemblyBindingConformanceV1.Association`与Runtime Fact Ref exact，Conformance六个BindingSet字段与Gateway返回的`Candidate.Binding`逐字段exact，且sealed Handoff Provider candidate exact映射到Gateway重建BindingSet的目标Runtime member full Ref。association path中Conformance Binding/Capability/Schema仍严格为空；Provider candidate、Handoff、Association、Generation、BindingSet及Runtime member须属同一完整assembly lineage。`runtime/conformance.CheckGenerationBindingAssociationCurrentReaderV1`只可在分离的Reader接口单测中执行；其`ProductionClaimEligible=false`，不得被fixture、集成或生产组装用作Gateway身份证明；
3. Harness concrete Handoff current Store/Reader；
4. Tool Owner concrete唯一`ToolSurfaceManifestCurrentRepositoryV1`，Harness只以窄`ToolSurfaceManifestCurrentReaderV1`注入；
5. Runtime concrete Registry Exact Reader；
6. Harness concrete composite Store/Publisher。

禁止一个fake同时扮演多个Owner，禁止static/cache/self-built/conformance-equivalent Association Reader冒充M2 concrete Gateway，禁止Harness fixture复制Tool repository。C2 Tool public合同与唯一Repo未落地、compile并独立审计YES前，跨Owner真实fixture与M2代码终审保持BLOCKED。

## 8. 反例、兼容与恢复

- raw caller Manifest/Conformance/ToolSurface/Profile shape合法但A2 lineage不一致：零B/C/Registry调用、零CAS；
- A2 nested pointer/slice/bytes输入或返回值被修改：Store history/current不变；
- B1 Association仍active但BindingSet成员、Generation或Activation已漂移：Runtime Gateway拒绝；
- static Fact Reader不重建BindingSet却返回valid Fact，且可通过shape-only conformance：因不具exact Runtime Gateway Provider identity/Binding/assembly binding proof，composition在零M2调用前拒绝；
- conformance-equivalent Reader通过接口单测后被注入M2 integration/acceptance/production：composition以Association exact Ref、Conformance分离BindingSet字段、sealed ProviderCandidate→Runtime BindingSet member exact映射与Provider package/type identity拒绝；conformance结果不参与production判定；
- C2 full Manifest Ref正确但content、Owner、Profile、entries、ExpectedInjection或expiry漂移：Conflict；
- A→B→Handoff→C→Registry任一步换序、并行或缺调用：call-order oracle拒绝；
- S1/S2任一Owner valid drift、TTL crossing、clock rollback、post-clock cancel：零CAS；
- same ID/revision drift、真实A→B→A ABA、64异内容：单winner，其余Conflict；
- Handoff Seal错误非零ContractVersion：Invalid，禁止静默覆盖；
- UnknownOutcome recovery受caller deadline、Owner TTL和独立host hard cap共同限制，只Inspect同一next Ref。

本裁决不修改Runtime、Tool或现有M2 Go对象；A2为Harness additive Owner-local合同，B1完全复用live Runtime public Reader/Gateway，C2完全复用Tool public完整Manifest current与唯一Repo。完成资产短审后，仍须等C2 live公共合同/Repo及真实fixture前置YES，才允许返修M2 Go并重新提交独立代码审计。
