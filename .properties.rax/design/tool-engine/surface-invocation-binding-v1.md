# Tool SurfaceInvocationBinding V1

状态：**Runtime ports唯一Assembly Current/RegistrySnapshot public Go nominal与Reader已落盘；Tool P4-1 Owner-local实现已通过独立software test。** 该结论不是系统YES；Harness M2、运行时Gate与全Provider路径证明闭合前，system/production继续hard-block。

## 1. Owner与非Owner

- Tool Owner：`ToolSurfaceInvocationBindingV1`、Ack、唯一create-once Repository、canonical、TTL、Inspect与错误分类。
- Model Owner：唯一public `modelcontract.PreparedModelInvocation*V1`、neutral `SurfaceBindingRefV1`与PreDispatch Gate时点。Tool直接消费Prepared nominal，不定义影子DTO、Kind翻译或缩水Ref。
- Harness/宿主Owner：只实现Model PreDispatch Gate与全Provider路径wiring；Gate内部通过注入的Tool Writer调用`Ensure`，不构造Tool Fact/private Commit，也不另建Repository。
- Application：不改Request、Port或状态机，不参与Surface选择。
- Binding/Ack只证明Tool Fact已提交且current，不授Authority、Review、Fence、Permit、Budget、Scope或Provider执行权。

## 2. Tool-owned nominal与Model public exact引用

以下Tool类型位于未来`tool-mcp/contract/surface_invocation_binding_v1.go`。Invocation/Binding nominal由Tool拥有；Prepared/Registry字段直接依赖Model public contract nominal，只允许`contract`依赖，不依赖Model实现/internal。Assembly方向禁止Tool反向import Harness，也禁止Tool neutral echo；Tool与Harness共同直接依赖Runtime ports唯一`ModelPreDispatchAssemblyCurrentRefV1/ProjectionV1`。该nominal的语义/发布Owner仍是Harness，Runtime ports只是无环公共合同落点。

```go
const ToolSurfaceInvocationBindingContractVersionV1 = "praxis.tool.surface-invocation-binding/v1"
const ToolSurfaceInvocationBindingAckKindV1 runtimeports.NamespacedNameV2 = "praxis.tool/surface-invocation-binding-ack-v1"

type ToolSurfaceInvocationCoordinateV1 struct {
    InvocationID     string      `json:"invocation_id"`
    InvocationDigest core.Digest `json:"invocation_digest"`
}

type ToolSurfaceInvocationBindingSubjectV1 struct {
    Invocation                ToolSurfaceInvocationCoordinateV1          `json:"invocation"`
    PreparedFactRef           modelcontract.PreparedModelInvocationRefV1 `json:"prepared_fact_ref"`
    PreparedHistoricalFact    modelcontract.PreparedModelInvocationFactV1 `json:"prepared_historical_fact"`
    PreparedCurrentRef        modelcontract.PreparedModelInvocationCurrentRefV1 `json:"prepared_current_ref"`
    PreparedCurrent           modelcontract.PreparedModelInvocationCurrentProjectionV1 `json:"prepared_current"`
    SurfaceCurrent            ToolSurfaceManifestCurrentProjectionV1     `json:"surface_current"`
    AssemblyCurrentRef        runtimeports.ModelPreDispatchAssemblyCurrentRefV1 `json:"assembly_current_ref"`
    AssemblyRegistrySnapshot  runtimeports.RegistrySnapshotRefV1         `json:"assembly_registry_snapshot"`
    AssemblyCurrent           runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1 `json:"assembly_current"`
    ToolExpectedInjectionDigest core.Digest                               `json:"tool_expected_injection_digest"`
    ModelActualToolSurfaceDigest core.Digest                              `json:"model_actual_tool_surface_digest"`
    ModelActualProviderInjectionDigest core.Digest                        `json:"model_actual_provider_injection_digest"`
    ProfileDigest             core.Digest                                 `json:"profile_digest"`
    RequestedNotAfterUnixNano int64                                       `json:"requested_not_after_unix_nano"`
    Digest                    core.Digest                                 `json:"digest"`
}

type ToolSurfaceInvocationBindingRefV1 struct {
    Owner           core.OwnerRef `json:"owner"`
    ContractVersion string        `json:"contract_version"`
    ID              string        `json:"id"`
    Revision        core.Revision `json:"revision"`
    Digest          core.Digest   `json:"digest"`
}

type ToolSurfaceInvocationBindingV1 struct {
    ContractVersion string                                `json:"contract_version"`
    Ref             ToolSurfaceInvocationBindingRefV1    `json:"ref"`
    Subject         ToolSurfaceInvocationBindingSubjectV1 `json:"subject"`
    CreatedUnixNano int64                                 `json:"created_unix_nano"`
    NotAfterUnixNano int64                                `json:"not_after_unix_nano"`
    Digest          core.Digest                           `json:"digest"`
}

type ToolSurfaceInvocationBindingAckRefV1 struct {
    Kind       runtimeports.NamespacedNameV2     `json:"kind"`
    ID         string                            `json:"id"`
    Revision   core.Revision                     `json:"revision"`
    Digest     core.Digest                       `json:"digest"`
    BindingRef ToolSurfaceInvocationBindingRefV1 `json:"binding_ref"`
}

type ToolSurfaceInvocationBindingAckV1 struct {
    ContractVersion string                             `json:"contract_version"`
    Ref             ToolSurfaceInvocationBindingAckRefV1 `json:"ref"`
    BindingRef      ToolSurfaceInvocationBindingRefV1 `json:"binding_ref"`
    Invocation      ToolSurfaceInvocationCoordinateV1 `json:"invocation"`
    PreparedFactRef modelcontract.PreparedModelInvocationRefV1 `json:"prepared_fact_ref"`
    PreparedCurrentRef modelcontract.PreparedModelInvocationCurrentRefV1 `json:"prepared_current_ref"`
    CheckedUnixNano int64                              `json:"checked_unix_nano"`
    NotAfterUnixNano int64                             `json:"not_after_unix_nano"`
    Digest          core.Digest                        `json:"digest"`
}

type ToolSurfaceInvocationBindingEnsureRequestV1 struct {
    Invocation                ToolSurfaceInvocationCoordinateV1          `json:"invocation"`
    PreparedFactRef           modelcontract.PreparedModelInvocationRefV1 `json:"prepared_fact_ref"`
    PreparedHistoricalFact    modelcontract.PreparedModelInvocationFactV1 `json:"prepared_historical_fact"`
    PreparedCurrentRef        modelcontract.PreparedModelInvocationCurrentRefV1 `json:"prepared_current_ref"`
    PreparedCurrent           modelcontract.PreparedModelInvocationCurrentProjectionV1 `json:"prepared_current"`
    SurfaceCurrent            ToolSurfaceManifestCurrentProjectionV1     `json:"surface_current"`
    AssemblyCurrentRef        runtimeports.ModelPreDispatchAssemblyCurrentRefV1 `json:"assembly_current_ref"`
    AssemblyRegistrySnapshot  runtimeports.RegistrySnapshotRefV1         `json:"assembly_registry_snapshot"`
    AssemblyCurrent           runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1 `json:"assembly_current"`
    RequestedNotAfterUnixNano int64                                       `json:"requested_not_after_unix_nano"`
}
```

Prepared historical Fact与current资格严格分层：Binding直接无损保存Model public `PreparedModelInvocationRefV1 + PreparedModelInvocationFactV1 + PreparedModelInvocationCurrentRefV1 + PreparedModelInvocationCurrentProjectionV1`，不创建Tool第二套Prepared/Registry nominal。Historical Fact提供RequestTools/PreparedPlan/Route/Profile、CapabilitySnapshot Ref、完整`runtimeports.RegistrySnapshotRefV1`、两个Actual digest与Historical NotAfter；Tool只把Current的Ref/PreparedRef/Checked/Expires/Digest视为current资格，public Current中的任何历史echo都必须回扣Historical且不成为新Authority。Historical retention不能替代current资格窗口，但Historical NotAfter是绝对上界，Current Expires与Binding NotAfter都不得超过它。

Assembly采用Harness sealed composite current watermark；Harness内部负责独立复读Generation/Handoff/BindingSet并输出共同最小expiry。Tool Binding直接嵌入Runtime ports唯一`ModelPreDispatchAssemblyCurrentProjectionV1`，完整保留Ref闭包、Generation/Handoff/BindingSet、Assembly Manifest/Conformance/Watermark、ToolSurface、Profile、完整Registry Ref、Semantic/Currentness、Checked/Expires与ProjectionDigest。不存在Tool echo、Kind转换、mapping、重签Ref或重算digest。

Binding Subject与Ensure Request同时直接嵌入live `runtimeports.ModelPreDispatchAssemblyCurrentRefV1`和`runtimeports.RegistrySnapshotRefV1`，并硬门`AssemblyCurrentRef == AssemblyCurrent.Ref`、`AssemblyRegistrySnapshot == AssemblyCurrent.RegistrySnapshot`。Model Prepared Historical/Current内的Registry字段也必须直接使用同一`runtimeports.RegistrySnapshotRefV1`，与`AssemblyRegistrySnapshot`做整体exact equality；禁止任何旧Model私有Registry nominal、alias、type-pun或JSON重编码。Authority始终是Registry Owner；Tool不定义fallback。

## 3. Canonical与stable ID

1. Invocation coordinate canonical：domain=`praxis.tool`、version=本合同version、discriminator=`ToolSurfaceInvocationCoordinateV1`，覆盖InvocationID/Digest。
2. Subject canonical：domain=`praxis.tool`、version=本合同version、discriminator=`ToolSurfaceInvocationBindingSubjectV1`，覆盖Subject除自身Digest外的全部字段；nil slice/空值规范固定，不做语义重写。
3. Binding canonical：domain=`praxis.tool`、version=本合同version、discriminator=`ToolSurfaceInvocationBindingV1`，覆盖ContractVersion、Ref（保留Owner/ContractVersion/ID/Revision并将Ref.Digest置空）、完整Subject、Created/NotAfter，排除Binding.Digest；computed digest必须同时写入`Ref.Digest`与`Binding.Digest`。
4. ID只由完整Invocation coordinate digest派生：`tool-surface-invocation-v1-`加64位hex，无colon且不超过96字符；Owner固定为Tool Owner，ContractVersion固定为本合同version，Revision固定1。Surface、Injection或watermark不进入ID，因此同Invocation变化只能Conflict，不能产生第二Fact。该五元组可与Model public neutral `SurfaceBindingRefV1{Owner,ContractVersion,ID,Revision,Digest}`无损对应。
5. Ensure request canonical使用discriminator=`ToolSurfaceInvocationBindingEnsureRequestV1`，覆盖完整exact coordinates与RequestedNotAfter，不含Owner clock派生的Created/NotAfter。same canonical重投以该request与winner Fact逐字段证明幂等，不比较重投时的新clock样本。
6. Ack canonical独立使用domain=`praxis.tool`、version=本合同version、discriminator=`ToolSurfaceInvocationBindingAckV1`。计算body覆盖Ack的全部字段，但必须先将`Ack.Ref.Digest`置空并排除顶层`Ack.Digest`；computed digest一次性同时写入`Ack.Ref.Digest`与`Ack.Digest`，严禁将任一computed digest回流计算第二次。Ack Ref Kind固定、Revision=1，ID只由完整Binding Ref+Prepared Fact Ref+Prepared Current Ref派生。Ack Ref/BindingRef/Invocation/Prepared refs/NotAfter必须逐字段回扣Binding。Ack只是Invocation→Surface历史因果Fact的提交回执，不是第二Truth Owner，也不授Provider执行权。

硬门：`PreparedCurrentRef.Prepared == PreparedFactRef`，Current Projection的Prepared Ref与Historical Fact/Ref逐字段exact，且Current `ExpiresUnixNano/NotAfterUnixNano`均不得超过Historical `NotAfterUnixNano`。Tool用现有public `ComputeExpectedInjectionDigest` canonical重算Surface Manifest的expected digest，并强制`SurfaceCurrent.Manifest.ExpectedInjectionDigest == ToolExpectedInjectionDigest == ModelActualToolSurfaceDigest`；`ModelActualProviderInjectionDigest`必须保存并Validate，但不与Tool expected强等。Context Frame/field注入链完全在本Binding合同之外，不能与Model Actual ToolSurface混义。

Tool Surface Manifest Current必须携完整Manifest。其exact Ref的ID/Revision/Digest无损等于Manifest ID/Revision/Digest，ProjectionDigest独立返回；Harness可直接用Plan中的Manifest坐标调用`InspectExactToolSurfaceManifestCurrentV1`，禁止latest或猜ProjectionDigest。Registry exact Ref的Authority是Registry Owner；Model Historical Fact只是`runtimeports.RegistrySnapshotRefV1`的无损carrier。该Ref完整携`Owner + ContractVersion + ID + Revision + Digest`，Tool只通过Model public Prepared Reader取得并无损保存在Binding，不定义Tool Ref/Reader或第二仓。硬门为`PreparedHistoricalFact.RegistrySnapshotRef.Digest == SurfaceCurrent.Manifest.RegistrySnapshotDigest`，且Prepared Current、Assembly Current与Binding中同Ref必须整体exact。V1没有独立Registry current资格；Surface Current以sealed Manifest与expiry承担该Surface/RegistrySnapshot组合的资格，每个边界复读同一Surface Current并交叉完整Registry Ref，禁止裸digest查latest。未来若Registry Owner发布独立current，只能另立additive Delta并纳入边界复读。CapabilitySnapshot Ref同样直接来自public Historical Fact，不降格为单digest。

每个Invocation只允许一个Binding。continuation/retry可复用同Invocation的前提是RequestTools、Plan、Route/Profile、ActualToolSurface、ActualProviderInjection及全部canonical坐标不变；任一变化必须由Model创建新Invocation epoch/coordinate。旧Invocation上变化返回Conflict，不能覆盖、续租或再建第二Fact。

## 4. 唯一Repository与窄能力

```go
type ToolSurfaceInvocationBindingWriterV1 interface {
    EnsureToolSurfaceInvocationBindingV1(
        context.Context,
        ToolSurfaceInvocationBindingEnsureRequestV1,
    ) (ToolSurfaceInvocationBindingV1, ToolSurfaceInvocationBindingAckV1, error)
}

type ToolSurfaceInvocationBindingReaderV1 interface {
    InspectToolSurfaceInvocationBindingByInvocationV1(
        context.Context,
        ToolSurfaceInvocationCoordinateV1,
    ) (ToolSurfaceInvocationBindingV1, ToolSurfaceInvocationBindingAckV1, error)
    InspectExactToolSurfaceInvocationBindingV1(
        context.Context,
        ToolSurfaceInvocationBindingRefV1,
    ) (ToolSurfaceInvocationBindingV1, ToolSurfaceInvocationBindingAckV1, error)
}

// Tool internal/owner private nominal；Harness/Application adapter不能构造或调用。
type toolSurfaceInvocationBindingCommitRequestV1 struct {
    EnsureRequest ToolSurfaceInvocationBindingEnsureRequestV1
    Subject       ToolSurfaceInvocationBindingSubjectV1
}

// 由同一Repository concrete实现；不是第二Store。
type toolSurfaceInvocationBindingCommitterV1 interface {
    commitCreateOnceToolSurfaceInvocationBindingV1(
        context.Context,
        toolSurfaceInvocationBindingCommitRequestV1,
    ) (ToolSurfaceInvocationBindingV1, ToolSurfaceInvocationBindingAckV1, error)
}

type ToolSurfaceInvocationBindingRepositoryV1 interface {
    ToolSurfaceInvocationBindingWriterV1
    ToolSurfaceInvocationBindingReaderV1
}
```

- 唯一Repository实例同时维护`by_binding_id`与`by_invocation_coordinate`两个唯一索引，并在一个线性化事务中写入Binding、exact Ack及两个索引；禁止Writer/Reader各自建仓、禁止第二事实仓、禁止用cache冒充索引。
- public Writer只接受`EnsureRequest`，Harness不得提交完整Subject、Created/NotAfter、Binding Ref/Digest或已Seal Tool Fact。Tool Owner先Validate request和exact currents，再以Owner clock生成Created/NotAfter、构造private CommitRequest、Seal并在唯一Repository内线性化create-once。
- private CommitRequest与Committer只能落在Tool `internal/owner/surfacebinding`（最终目录名以仓内internal惯例为准），不得落入`applicationadapter`语义层。Tool public adapter只做public nominal→EnsureRequest逐字段映射；同一Repository concrete同时实现commit与两个Inspect，不得注入第二Commit Store。首次create原子写Binding/Ack及两个索引；同canonical request返回winner Binding/Ack；same Invocation任一stable/current coordinate或RequestedNotAfter不同均Conflict，零覆盖、零新ID。Owner派生Created/NotAfter/Ack Checked不参与重投request相等比较。
- Ensure回包丢失只允许以原Invocation调用`Inspect...ByInvocation`，再由winner Fact重建并Validate同一Ack；不得再次create、重新选择Surface或进入Provider。
- `Inspect...ByInvocation`只用完整Invocation coordinate查唯一索引并返回同一winner Binding/Ack；`InspectExact`只接收完整Binding Ref，在唯一Repository中返回同一immutable Binding与其Tool Ack deep-copy。Reader内部必须重算Binding/Ack canonical，验证Ack Ref与Ack顶层Digest、Ack.BindingRef、Binding.Ref全部exact；调用方不需要预知AckRef。actual-point可从Model public Ack仅取`SurfaceBindingRefV1`，无损转为Tool BindingRef后exact Inspect。两种Inspect均不刷新时间。
- constructor拒绝nil/typed-nil clock/store；nil context在任何clock/index访问前InvalidArgument。Unavailable、Indeterminate、timeout、decode error不得转换为NotFound；只有唯一Repository对exact key给出的authoritative NotFound才是NotFound。

## 5. TTL、freshness与currentness

Seal前按顺序读取并验证Prepared Current Projection、Surface Current与Harness sealed composite Current，再取Tool Owner fresh `created`。`NotAfter`一次性取以下最小值：

```text
PreparedHistoricalFact.NotAfterUnixNano
PreparedCurrent.ExpiresUnixNano
SurfaceCurrent.Expires
AssemblyComposite.Expires
RequestedNotAfter（必需且大于created）
caller deadline（若有）
```

必须满足Prepared Current、Surface与composite所有Checked `<= created < NotAfter`，且final fresh clock `>= created && < NotAfter`；clock rollback、TTL crossing或任何expiry缺失均零Ensure/Ack。V1只使用上表六项共同min，不冻结默认秒数或额外cap，也不把TTL当生产SLA。Prepared历史Fact retention不进入min，但Historical NotAfter强制进入min。Binding create-once后Created/NotAfter不可刷新、缩短、延长或续租；过期Fact仍可历史Inspect，但`ValidateCurrent(now)`返回PreconditionFailed。旧Invocation不得创建第二Binding或续租；恢复必须使用新Invocation epoch。

Binding/Ack不授Provider执行权。每次provider attempt、Stream、Open及continuation边界前必须复读：Model Prepared Current；Tool Binding与exact Ack；Tool Surface Current并重新核Registry Ref/Digest；Harness sealed Assembly composite。四组对象均按同一refs `InspectExact + ValidateCurrent`，任一TTL已过、drift、Unavailable或clock rollback均Fail Closed、Provider调用为0。跨TTL恢复只能创建新Invocation epoch，不能给旧Binding续命。

冻结签名：

```go
func (c ToolSurfaceInvocationCoordinateV1) Validate() error
func (r ToolSurfaceInvocationBindingRefV1) Validate() error
func (r ToolSurfaceInvocationBindingAckRefV1) Validate() error
func (s ToolSurfaceInvocationBindingSubjectV1) Validate() error
func (b ToolSurfaceInvocationBindingV1) Validate() error
func (b ToolSurfaceInvocationBindingV1) ValidateCurrent(now time.Time) error
func (b ToolSurfaceInvocationBindingV1) ComputeDigest() (core.Digest, error)
func SealToolSurfaceInvocationBindingV1(ToolSurfaceInvocationBindingV1) (ToolSurfaceInvocationBindingV1, error)
func (r ToolSurfaceInvocationBindingEnsureRequestV1) Validate() error
func (r ToolSurfaceInvocationBindingEnsureRequestV1) ValidateAgainst(ToolSurfaceInvocationBindingV1) error
func (a ToolSurfaceInvocationBindingAckV1) ValidateAgainst(ToolSurfaceInvocationBindingV1, time.Time) error
```

顺序固定为intrinsic Validate → canonical/ID/digest重算 → external exact重复字段交叉 → current/fresh clock → Repository Ensure。任一失败零Ack、零Provider。

## 6. 后续Tool S1

1. Application Input与terminal Model ToolCall Projection按既有Reader exact读取；terminal Projection只提供最终Observation及Invocation coordinate，不证明pre-dispatch injection。
2. 以terminal Projection的Invocation ID/Digest构造`ToolSurfaceInvocationCoordinateV1`，调用`Inspect...ByInvocation`；NotFound/Unavailable/Indeterminate/expired均Fail Closed。
3. terminal Projection的session Invocation ID/Digest必须逐字段映射为同一`ToolSurfaceInvocationCoordinateV1`；从Binding的Prepared Fact Ref与Prepared Current exact坐标调用Model public Reader复读，并验证terminal Invocation、历史Fact与Current Projection Invocation exact；禁止用terminal Projection替代Prepared Current。
4. 以Binding内`ToolSurfaceManifestCurrentRefV1`调用窄Reader的`InspectExactToolSurfaceManifestCurrentV1`；验证返回Ref仍等于Manifest ID/Revision/Digest并独立验证ProjectionDigest。从同一Binding保存的Model public Historical Fact取得完整Registry exact Ref，验证其Owner/ContractVersion/ID/Revision/Digest与public Current echo（若有）exact，且Digest等于Manifest.RegistrySnapshotDigest；再按既有合同读取Capability/Tool Registry Object Currents和InputContract。
5. 内部CandidateV3 builder只消费上述closure；随后完整S2、共同min及BindingV2 create-once。BindingV2 closure必须保存SurfaceInvocationBinding exact Ref/Fact；Provider Boundary前再次InspectExact并ValidateCurrent。

## 7. 公共nominal与联合对齐

- Model Prepared部分不做neutral copy或Go alias，而是直接引用Model public `PreparedModelInvocationRefV1`、`PreparedModelInvocationFactV1`、`PreparedModelInvocationCurrentRefV1`、`PreparedModelInvocationCurrentProjectionV1`；它们的Registry字段直接使用`runtimeports.RegistrySnapshotRefV1`。Tool只依赖Model public contract/ports与Runtime public ports，不依赖Model实现/internal。
- Tool `applicationadapter`按Model public Reader返回值原样传入EnsureRequest，不重建Ref、不替换Owner/ContractVersion、不重新hash。Prepared Historical是Model-owned历史Fact，其携带的Registry Ref仍以Ref.Owner所指Registry Owner为Authority；Model只是无损carrier。Current只授current资格，public Current中的Historical echo必须exact回扣Fact。缺字段或语义转换一律compile/design blocked。
- 直接复用Harness `assemblycontract` 会形成package cycle；Tool自建neutral echo又会导致有损映射。live Runtime ports已提供唯一`runtimeports.ModelPreDispatchAssemblyCurrentRefV1/ProjectionV1/ReaderV1`中立public Go nominal；Harness是语义/发布Owner，Tool只按exact Ref读取并直接嵌入Projection。Harness/Application不得import Tool实现，不构造Tool Fact/private Commit。Application不变。

三方公共nominal字段闭包如下；Tool/Harness都不进行映射：

| Owner public语义 | Binding直接字段 | 当前门 |
|---|---|---|
| Model public Prepared Fact Ref + Historical Fact | 同一public nominal原样入Binding | Historical含RequestTools/PreparedPlan/Route/Profile、CapabilitySnapshot、完整`runtimeports.RegistrySnapshotRefV1`、两个Actual digest、NotAfter |
| Model public Prepared Current Ref + Projection | 同一public nominal原样入Binding | Current Ref/PreparedRef/Checked/Expires/Digest授资格；Historical echo只做exact回扣 |
| Model terminal session Invocation ID/Digest | `ToolSurfaceInvocationCoordinateV1` | 只用于`InspectByInvocation`，必须与Prepared Fact/Current Invocation exact |
| Runtime ports public composite Ref | `runtimeports.ModelPreDispatchAssemblyCurrentRefV1` | ContractVersion/ID/Revision/Digest、完整Generation、Handoff、BindingSet、Manifest、Conformance、WatermarkDigest、ToolSurface、ProfileDigest、RegistrySnapshot、SemanticDigest、CurrentnessDigest、Checked/Expires与ProjectionDigest完整闭包 |
| Runtime ports public `ToolSurface` | `AssemblyCurrent.ToolSurface` | ID/Revision/Digest exact，无Kind字段或转换 |
| Runtime ports Profile + complete Registry Ref | `AssemblyCurrent.ProfileDigest/RegistrySnapshot` | ProfileDigest与Registry Owner/ContractVersion/ID/Revision/Digest完整保留 |
| Runtime ports Generation/Handoff/BindingSet | `AssemblyCurrent`同名坐标 | Generation完整Ref、Handoff `ModelPreDispatchAssemblyExactRefV1`、BindingSet `ModelPreDispatchAssemblyBindingSetRefV1`（含Semantic/Currentness/Projection digest与Expires）全部保留 |
| Runtime ports Semantic/Currentness/Checked/Expires/ProjectionDigest | `AssemblyCurrent`同名字段 | Tool不拆分TTL、不重签currentness、不重hash |
| Tool Surface Manifest ExpectedInjectionDigest | `ToolExpectedInjectionDigest` | 必须由public `ComputeExpectedInjectionDigest`重算并等于Model ActualToolSurfaceDigest |
| Model ActualProviderInjectionDigest | `ModelActualProviderInjectionDigest` | 完整保存但不得与Tool expected强等 |
| Runtime public `RegistrySnapshotRefV1` | 同一public nominal原样保存在Prepared Historical/Current、Assembly Current与Binding | Authority为Registry Owner；Owner/ContractVersion/ID/Revision/Digest整体exact，Digest必须等于Manifest.RegistrySnapshotDigest |

不存在Assembly mapping adapter；Tool Writer、Reader、Binding与Harness Publisher直接使用同一live Runtime ports nominal。Conformance对每个字段做单字段漂移反例，任一缩水DTO、echo、alias、零值补齐或digest重算均零Ensure/零Provider。P4-1 Tool Owner-local Go与软件验收已闭合；Harness wiring与system/production仍hard-block。

联合冻结后进入P4 Go的精确门：

1. Model public Prepared historical Fact Ref、Prepared Current Projection/Reader与PreDispatchGate时点可编译且独立审计YES；
2. Runtime ports唯一Assembly Current Ref/Projection/Reader public Go已落盘；仍需C2资产复审、Harness/宿主使用同一nominal发布sealed composite，并证明每个Provider attempt/Stream/Open/continuation actual-point前都重新读Prepared Current、Binding/ToolAck、Surface Current、Assembly composite；
3. 本文Tool字段/接口与Model/Harness对齐通过独立终审；
4. import DAG、typed-nil/nil-context/error taxonomy、single Repository注入和no-production-root边界通过静态审计；
5. 用户明确解冻PD-TM-04 P4 Go。此前只允许资产评审。

## 8. 测试矩阵

| ID | 类别 | 正向/反向断言 |
|---|---|---|
| `SIB-V1-UNIT-01` | canonical | 同完整Binding Seal稳定；Binding Ref按Owner/ContractVersion/ID/Revision/Digest无损对应Model neutral SurfaceBindingRef；换任一字段Digest变化 |
| `SIB-V1-UNIT-02` | identity | ID只随Invocation coordinate变化；换Surface不生成第二ID而在Ensure Conflict |
| `SIB-V1-UNIT-03` | surface digest | public canonical重算Manifest expected并与Model ActualToolSurface exact；Provider Actual只保存不强等；任一混义拒绝 |
| `SIB-V1-UNIT-04` | prepared | Historical完整字段与Current Ref/窗口分层且互相回扣；用retention代替current、Current复制Historical或超过Historical NotAfter拒绝 |
| `SIB-V1-UNIT-05` | assembly | Tool/Harness直接使用Runtime ports唯一composite；Ref全闭包、ToolSurface、Profile、完整Registry Ref、Generation/Handoff/BindingSet、Manifest/Conformance/Watermark、Semantic/Currentness、时间和digest任一漂移拒绝 |
| `SIB-V1-UNIT-06` | no lossy echo | 任一Tool/Harness影子DTO、Kind转换、泛型Ref、JSON/reflection/alias、重hash或缺省值即拒绝 |
| `SIB-V1-UNIT-06A` | Ack canonical | 计算时仅将Ack.Ref.Digest置空并排除Ack.Digest；computed digest同时写入两处，二次回流Seal、两digest不等或Ack.BindingRef漂移均拒绝 |
| `SIB-V1-UNIT-07` | TTL | Historical NotAfter、Prepared Current、Surface、composite、必需requested、caller共同min；缺requested/expired/rollback/final crossing拒绝 |
| `SIB-V1-UNIT-07A` | snapshots | Model public Capability Ref与`runtimeports.RegistrySnapshotRefV1`完整exact；Registry Owner/ContractVersion/ID/Revision/Digest任一漂移、降格摘要或Manifest digest不回扣拒绝 |
| `SIB-V1-UNIT-08` | writer ownership | public Ensure只接受exact request；caller填Created/NotAfter/Fact/Digest无入口，Tool clock+private Commit生成winner |
| `SIB-V1-IDEMP-01` | idempotency | 同canonical重复Ensure返回同Binding Ref/Ack，记录数仍1 |
| `SIB-V1-CONFLICT-01` | conflict | 同Invocation换Prepared/Surface/任一Actual/Profile/RegistrySnapshot/composite/任一内嵌ref/requested均Conflict，记录数仍1 |
| `SIB-V1-CONFLICT-02` | invocation epoch | 同Invocation内RequestTools/Plan/Route/Profile/任一Actual digest变化拒绝；新Invocation coordinate才允许新Binding |
| `SIB-V1-FAULT-01` | lost reply | create已提交后丢回包，以Invocation Inspect恢复winner并重建同Ack；Ensure调用不增加 |
| `SIB-V1-FAULT-02` | errors | Unavailable/Indeterminate/timeout不转NotFound，不create、不Provider |
| `SIB-V1-READ-01` | exact read | `InspectExact(ctx, BindingRef)`单参返回同immutable Binding/Ack deep-copy；Reader内部验AckRef关系，调用方无需预知AckRef；Binding/Ack same ID换digest或交叉BindingRef漂移均Conflict |
| `SIB-V1-RACE-01` | same key | 64并发同canonical单create/单record/同Ack |
| `SIB-V1-RACE-02` | conflict | 64并发同Invocation不同canonical单赢家，其余Conflict，无第二仓记录 |
| `SIB-V1-RACE-03` | different key | 64不同Invocation可并行，禁止全局串行锁作为正确性证明 |
| `SIB-V1-CONF-01` | provider gate | 每次provider attempt/Stream/Open/continuation actual-point前均重读Prepared Current、`InspectExact(ctx, ModelAck.SurfaceBindingRef)`得Binding/ToolAck、Surface Current与Assembly composite；Ack单独存在不授执行权 |
| `SIB-V1-CONF-02` | terminal observation | 只有terminal Projection、没有Binding时零S1/零Candidate/零Provider |
| `SIB-V1-BOUNDARY-01` | reread | S1及每个actual boundary均按同refs InspectCurrent；expired/drift/clock rollback零后续写 |
| `SIB-V1-BOUNDARY-02` | cross TTL | Ack已存在但任一Prepared/composite/Binding current跨TTL时Provider=0；旧Invocation不可续命 |
| `SIB-V1-IMPORT-01` | boundary | Tool contract不导入Harness/Application/Model internal；无JSON/reflection/type alias兼容 |
| `SIB-V1-COMPILE-01` | compile boundary | Tool contract只允许`modelcontract.PreparedModelInvocation*V1` + `runtimeports.ModelPreDispatchAssemblyCurrent*V1/RegistrySnapshotRefV1`；旧Model Registry nominal、Tool Assembly DTO或Harness import导致compile/import-boundary失败 |
| `SIB-V1-SCHEMA-01` | schema | Prepared Historical/Current、Assembly Projection/Binding的Registry字段必须是同一`runtimeports.RegistrySnapshotRefV1`且整体exact；Owner/ContractVersion/ID/Revision/Digest任一漂移、alias/type-pun/JSON重编码均零Ensure/零Provider |
| `SIB-V1-C2-01` | method set | Harness M2只import Tool contract且构造器只接`ToolSurfaceManifestCurrentReaderV1`；Repository/Ensure不可见，所有M2路径EnsureCalls=0 |
| `SIB-V1-C2-02` | exact coordinate | Plan ToolSurface ObjectRef到Current Ref的ID/Revision/Digest golden逐字段相等；禁止prefix、latest或猜ProjectionDigest |
| `SIB-V1-C2-03` | digest split | wrong Manifest/Ref digest与wrong ProjectionDigest分别拒绝，互不替代、不改查另一对象 |
| `SIB-V1-C2-04` | safety | typed-nil、nil/canceled ctx、tamper、TTL crossing、deep-clone alias均Fail Closed且EnsureCalls=0 |

原`PD-TM04-N01..N26`保持26/26，不改编号、级别或原义；schema `PD-TM04-S-P0-01..08/P1-01..03/P2-01`保持12/12。本表是additive，不替换原矩阵。新增桥接关联：`N01/N06/N07/N13/N16/N21/N26`追加SurfaceInvocationBinding前置；`S-P0-04/06/07/08`追加Surface选择与持久closure；`S-P1-01/02/03`追加validator/TTL/nominal；`S-P2-01`追加“Assembly只直接复用Runtime ports唯一nominal，禁止Tool/Harness影子DTO”真值检查。
