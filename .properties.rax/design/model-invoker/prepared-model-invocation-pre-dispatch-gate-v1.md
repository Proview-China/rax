# PreparedModelInvocation / PreDispatch Gate V1

## 1. 状态与目标

- Owner：`Model Invoker`
- 状态：Surface第六审；Model本地合同已基本闭合，Runtime asset candidate及Registry Exact Reader合同已落盘；当前external P0为1项，仍不允许写Go
- 性质：additive、provider-neutral、tool-owner-neutral
- 目标：在完整统一请求和实际工具注入都已确定后，建立一个不可变的准备事实、一个短时current投影和一个公共Commit Gate；任何模型外部调度都必须在取得并验证current ACK后才能发生。

本合同解决的是“这次Model Invocation准备了什么，以及它是否已通过pre-dispatch提交门”。它不创建Tool Surface、PendingAction、ActionCandidate、Effect、Evidence或Runtime Settlement，也不把Gate ACK解释为最终执行授权。

## 2. Live基线与必须修正的顺序

当前live链存在多处早于正式推理调用的外部行为：

- `execution.Runtime.Start`先调用`Adapter.Preflight`；Direct Preflight会调用`Backend.Resolve`；Claude、Qwen、Codex App Server和ACP Preflight会启动子进程并initialize；
- 根`modelinvoker.Invoker.prepare`会调用`Provider.Capabilities`，随后才进入`Provider.Invoke`或`Provider.Stream`；
- Route Gateway当前在真正Invoker前可能解析secret、获取pool lease并构造Provider；
- Direct首次调用和每次tool-result continuation都会再次调用`Backend.Invoke/OpenStream`；
- retry会重复调用`Provider.Invoke`；Realtime会直接调用`Provider.Open`。

因此V1固定为两个阶段：

```text
Acquire Sealed Authority Inputs
  - full Registry candidate Ref -> Registry Owner Exact Reader -> verified clone
  - full Profile/Route/Capability snapshot refs -> respective public Exact Readers
  - no Provider/Backend/secret/pool/session/process/model dispatch
        |
        v
PurePrepare / Map / Seal
  - 完整UnifiedExecutionRequest已Validate并Digest
  - 只读前一步已verified并pin的Route/Profile/Capability/Registry snapshot clones
  - 映射provider-visible Tools/ToolChoice/ParallelToolCalls/hosted-tool options
  - 严格零Provider、Backend、secret、pool、session、process、network调用
        |
        v
Ensure Historical Fact + Ensure Current Projection
        |
        v
public CommitGate -> current ACK -> Model exact/current validation
        |
        v
外部preflight/discovery/resolve/secret/pool/process/open/invoke/stream
```

Authority snapshot exact读取发生在PurePrepare之前，只能通过各Owner公共Reader取得sealed clone；它不是provider discovery。任何现有实现无法在PurePrepare阶段完成的工作都不能偷偷保留在Gate前；它必须被拆成纯映射和Gate后的外部执行两部分。

## 3. 三类公共数据

### 3.1 PreparedModelInvocationRefV1

```text
PreparedModelInvocationRefV1
  ContractVersion        = praxis.model-invoker.prepared-model-invocation/v1
  ID                     stable nominal ID
  Revision               = 1
  Digest                 exact Historical Fact digest
  InvocationID           exact UnifiedExecutionRequest.ExecutionID
  InvocationDigest       exact terminal-compatible invocation digest
  UnifiedRequestDigest   exact UnifiedExecutionRequest.Digest()
```

V1中`InvocationDigest == UnifiedRequestDigest`是硬不变量。这样它能与live `ToolCallCandidateObservationRefV1.InvocationID/InvocationDigest`做exact父Invocation回链；不得另造一个不同含义的InvocationDigest。

`ID`由domain-separated identity digest对`ContractVersion + InvocationID + InvocationDigest`派生。它不包含Fact Digest，因此不存在ID/Digest闭环。

`PreparedModelInvocationRefV1`是Harness、Tool和Model唯一允许复用的Historical Ref nominal；三方不得另建只含ID/digest、InvocationID/digest或latest pointer的缩水DTO。

### 3.2 PreparedModelInvocationFactV1

```text
PreparedModelInvocationFactV1
  ContractVersion
  ID
  Revision
  InvocationID
  InvocationDigest
  UnifiedRequestDigest
  RequestToolsDigest
  PreparedPlanDigest
  RouteDigest
  ProfileDigest
  ActualToolSurfaceDigest
  ActualProviderInjectionDigest
  CapabilitySnapshotRef
  RegistrySnapshotRef    runtimeports.RegistrySnapshotRefV1
  CreatedUnixNano
  NotAfterUnixNano
  Digest
```

字段语义固定如下：

| 字段 | 权威内容 |
|---|---|
| `UnifiedRequestDigest` | Model对完整`UnifiedExecutionRequest`重算的digest；覆盖Input、Instructions、有序Tools、schema、ToolPolicy、Budget、ProviderOptions等全部序列化字段 |
| `RequestToolsDigest` | domain-separated canonical body `{ordered Tools, ToolPolicy}`；不得排序Tools，不得重写schema，不得只按name摘要 |
| `PreparedPlanDigest` | 已sealed `PreparedExecutionPlan.Digest` |
| `RouteDigest` | Model-owned exact Route/RouteFingerprint canonical digest，不含明文secret |
| `ProfileDigest` | Model Profile Compiler对完整Effective Profile的digest；不得用`ProfileKeyDigest`冒充 |
| `ActualToolSurfaceDigest` | 严格使用Tool公共`ComputeExpectedInjectionDigest(entries)`canonical合同计算，只覆盖canonical ordered entries中的`ModelName + InputSchemaRef + DescriptionDigest + EffectKinds`；只允许与`ToolSurfaceManifest.ExpectedInjectionDigest`exact比较 |
| `ActualProviderInjectionDigest` | Model-owned richer canonical body，覆盖映射后Model准备提交给provider/harness的有序Tools、ToolChoice、ParallelToolCalls、hosted/builtin tool options和provider tool extensions；不与Tool或Context expected digest比较 |
| `CapabilitySnapshotRef` | Model-owned、不可变、已sealed的能力快照exact ref；PurePrepare只能读取该快照，不能实时调用Provider |
| `RegistrySnapshotRef` | 直接使用已落盘Runtime asset candidate中的唯一neutral `runtimeports.RegistrySnapshotRefV1{Owner,ContractVersion,ID,Revision,Digest}`；Authority为`Ref.Owner`，Model只携带和比较；public Go nominal尚未实现 |
| `CreatedUnixNano` | Model clock观察到的UTC创建时间 |
| `NotAfterUnixNano` | 此准备事实允许产生current资格的绝对上界；历史事实过期后仍可读，但不能再提交dispatch |

`ActualToolSurfaceDigest`的canonical Owner是Tool。Model不得重写算法、改变domain/version/discriminator、重新排序或把provider-only字段混进去；实现必须通过获批的Tool公共canonical合同/窄Port取得exact结果，不能import Tool internal/implementation。`ActualToolSurfaceDigest == ToolSurfaceManifest.ExpectedInjectionDigest`是Surface Binding的唯一digest等式。

`ActualProviderInjectionDigest`中的`Actual`只表示“PurePrepare已完成映射的实际目标provider request形状”，不表示provider/harness已经执行或已经产生Observed Actual Manifest。它是sealed target digest；Gate后的任何观测只能验证或拒绝，不能回写Historical Fact。

`ProfileDigest`和Runtime Registry public Go是live前置Delta：当前Plan没有完整ProfileDigest；Runtime asset candidate已经冻结唯一neutral Registry Ref、`RegistrySnapshotExactReaderV1`及Assembly Current Ref/Projection/Reader，但这些public Go nominal/Reader均尚未实现。Tool Surface仍只有`RegistrySnapshotDigest`。Model/Harness/Tool不得重算、猜测、只传裸digest或用latest替代。

若Harness可能在Model提交内容之外添加原生工具，PurePrepare必须持有可验证的sealed Harness tool-surface capability/snapshot；缺失时不能形成current资格。Gate后的Preflight只能重新观测并校验Model provider-tool映射，不能把未知原生工具补写回Historical Fact；发现额外工具必须在发送prompt前fail closed并释放process/session。

Context的`ExpectedInjectionManifest -> Harness ActualInjectionManifest -> InjectionConformanceFact`是Frame/field注入链，和Tool Surface Binding是两条独立因果链。Context Manifest、ProviderActualInjectionObservation或Context Conformance不得进入`ActualToolSurfaceDigest`硬等式，也不得与`ActualProviderInjectionDigest`直接比较。

Fact Digest使用`core.CanonicalJSONDigest`等价的严格、domain-separated canonical算法，覆盖除`Digest`外的全部字段。Fact不内嵌Ref；Ref在Fact seal完成后复制Fact身份与Digest。任何非法UTF-8、重复JSON key、非canonical schema、零digest或空exact ref都拒绝。

### 3.3 PreparedModelInvocationCurrentProjectionV1

Historical Fact只表达“曾准备过什么”，不表达“现在还能否提交”。短时资格使用独立不可变Current Projection：

```text
PreparedModelInvocationCurrentProjectionV1
  ContractVersion        = praxis.model-invoker.prepared-model-invocation-current/v1
  ID
  Revision               = 1
  Digest
  Prepared               complete PreparedModelInvocationRefV1
  CapabilitySnapshotRef  exact copy
  RegistrySnapshotRef    exact copy
  ActualToolSurfaceDigest       exact copy
  ActualProviderInjectionDigest exact copy
  CheckedUnixNano
  ExpiresUnixNano
  NotAfterUnixNano
```

- Current ID只从完整Prepared Ref派生；Checked/Expires不能参与identity，避免换时间创建第二Current。Current Digest覆盖除自身`Digest`外的全部字段。
- 必须满足`Created <= Checked < Expires <= NotAfter`，同时不超过context deadline和Runtime policy上界。
- Historical Fact的`NotAfter`只是current资格的绝对上界，不是Fact retention TTL。Current是一次检查结果，不修改Historical Fact，不延长Fact的`NotAfter`，不伪造Tool/Runtime领域TTL。
- 一个Invocation epoch只允许create-once一个session-level Current Projection，并由Gate create-once一个Surface Binding/ACK。retry、Stream/Open和Direct continuation不得刷新Current、延长TTL或创建第二Binding。
- Current Reader只接受完整Ref并返回deep clone，严格重算Prepared Ref、Current ID和Digest；禁止按ID/latest/name弱读取。

`PreparedModelInvocationCurrentRefV1`固定为`ContractVersion + ID + Revision + Digest + complete PreparedRef + CheckedUnixNano + ExpiresUnixNano + NotAfterUnixNano`。Ref的`ID/Revision/Digest`逐字段复制Current Projection；Ref不另算内容digest。Exact Reader必须重算Projection ID/Digest并验证完整Prepared Ref与时间字段。

Registry exact Ref不再由Model定义。Runtime asset candidate已经冻结Ref与Exact Reader合同；下列Ref/Reader都尚无public Go实现：

```go
// 已落盘asset candidate；package runtimeports
type RegistrySnapshotRefV1 struct {
    Owner           core.OwnerRef
    ContractVersion string
    ID              string
    Revision        core.Revision
    Digest          core.Digest
}

type RegistrySnapshotExactReaderV1 interface {
    InspectExactRegistrySnapshotV1(
        context.Context,
        RegistrySnapshotRefV1,
    ) (RegistrySnapshotRefV1, error)
}
```

唯一公共类型名固定为`runtimeports.RegistrySnapshotRefV1`，五字段不可变。Model资产不得提前创建同名本地struct、type alias、wrapper或wire mirror。`runtimeports.RegistrySnapshotRefV1`的Authority始终是`Owner`指向的Registry Owner；Runtime只拥有neutral nominal，Model只是无损carrier，Historical/Current Repository不能创建、修正或刷新该Ref。

取得顺序固定为：

1. 在PurePrepare之前，Model接收来自已解析Plan/Assembly的完整candidate Ref；candidate不得是裸digest、ID或latest selector；
2. Model调用Runtime ports公共`RegistrySnapshotExactReaderV1`的Registry Owner实现；Reader必须在Authority Repository中按完整Ref exact Inspect、严格校验Owner/Version/ID/Revision/Digest并返回deep clone；
3. 返回Ref必须与candidate逐字段exact；NotFound、Unavailable、Indeterminate、owner/version/revision/digest drift全部fail closed；
4. Model把该verified Ref pin为PurePrepare的sealed输入；PurePrepare本身只消费已pin clone，不再访问Registry；
5. Historical Fact和Current Projection逐字段无损携带同一Ref，Model Reader对Harness/Tool返回clone。

Model Prepared Fact/Current、Harness Assembly composite和Tool Binding都必须直接使用同一个Runtime ports concrete nominal，禁止alias、复制或投影成Owner-local Ref。Harness/Tool仍通过`PreparedModelInvocationReaderV1`与`PreparedModelInvocationCurrentReaderV1`读取Model carrier中的完整Ref，禁止调用方payload、私有copy或裸digest替代。跨Owner关系固定为：

```text
Historical.RegistrySnapshotRef
  == Current.RegistrySnapshotRef
  == HarnessAssemblyComposite.RegistrySnapshotRef
  == ToolBinding.RegistrySnapshotRef

Historical.RegistrySnapshotRef.Digest
  == ToolSurfaceCurrent.Manifest.RegistrySnapshotDigest
```

live `ToolSurfaceManifest`只有`RegistrySnapshotDigest`，所以它本身不能提供完整身份。Harness/Tool在提交Gate时必须同时exact读取Model Historical/Current和Tool Surface Current Manifest，验证上述Digest等式；完整Ref由Model Prepared Historical/Current无损携带Runtime nominal。任何实现都不能从Tool Surface的裸digest反向猜Owner、Version、ID或Revision，也不能创建三方私有Registry Ref DTO。

Assembly composite的Runtime asset candidate已经落盘并冻结唯一neutral Ref/Projection/Reader；对应public Go nominal/Reader尚未实现。未来类型必须位于`ExecutionRuntime/runtime/ports`，Harness负责发布/实现但不拥有type，Tool直接引用Runtime concrete nominal，Model无需import Harness或Tool。Model Prepared Fact不内嵌Harness composite；CommitGate实现按Runtime neutral composite完成跨Ownercurrent校验，Model只消费ACK。

每次真实dispatch边界前另生成纯数据`PreparedModelInvocationDispatchValidationReceiptV1`：

```text
ContractVersion = praxis.model-invoker.prepared-model-invocation-dispatch-validation/v1
ID / Revision=1 / Digest
PreparedRef / CurrentRef / AckRef
DispatchSequence        positive, monotonic within Invocation epoch
BoundaryKind
ProviderAttemptOrdinal  scoped to this DispatchSequence
AttemptRequestDigest    full mapped request for this boundary
ActualToolSurfaceDigest
ActualProviderInjectionDigest
CheckedUnixNano
```

Receipt ID从`PreparedRef + DispatchSequence`派生，Digest覆盖除自身`Digest`外的全部字段。Receipt只记录本次边界已经exact Inspect同一个Prepared Current和同一个ACK，并在紧邻调用前通过clock/current/两个digest校验；它不是下一边界的guard输入，不能复用、不能持久化新的Binding、不能刷新TTL，不是Authority、Permit或Provider执行权。

## 4. 公共Port

设计冻结的最小公共形状如下，精确Go落点须在实现计划的P0合同审查中确定：

```go
type PreparedModelInvocationRepositoryV1 interface {
    EnsurePreparedModelInvocationV1(
        context.Context,
        PreparedModelInvocationFactV1,
    ) (PreparedModelInvocationFactV1, error)
}

type PreparedModelInvocationReaderV1 interface {
    InspectExactPreparedModelInvocationV1(
        context.Context,
        PreparedModelInvocationRefV1,
    ) (PreparedModelInvocationFactV1, error)
}

type PreparedModelInvocationCurrentReaderV1 interface {
    InspectExactPreparedModelInvocationCurrentV1(
        context.Context,
        PreparedModelInvocationCurrentRefV1,
    ) (PreparedModelInvocationCurrentProjectionV1, error)
}

type PreparedModelInvocationCurrentRepositoryV1 interface {
    PreparedModelInvocationCurrentReaderV1
    EnsurePreparedModelInvocationCurrentV1(
        context.Context,
        PreparedModelInvocationCurrentProjectionV1,
    ) (PreparedModelInvocationCurrentProjectionV1, error)
}

type PreparedModelInvocationCommitGateV1 interface {
    Commit(
        context.Context,
        PreparedModelInvocationRefV1,
        PreparedModelInvocationCurrentRefV1,
    ) (PreparedModelInvocationCommitAckV1, error)
    InspectExactAck(
        context.Context,
        PreparedModelInvocationCommitAckRefV1,
    ) (PreparedModelInvocationCommitAckV1, error)
}
```

这是Model、Harness、Tool三Owner唯一允许依赖的Gate接口与exact方法集：

```text
Commit(ctx, complete PreparedRef, complete CurrentRef) -> complete ACK
InspectExactAck(ctx, complete AckRef) -> complete ACK clone
```

Harness只能实现该接口并注入Model，不能重命名方法、调整参数顺序、删除CurrentRef、接受缩水DTO、返回私有ACK或增加另一套可被生产调用的Gate签名。Tool只通过Gate实现内部的公共Surface/Binding Port参与，不直接获得Commit调用权。

上述Model Prepared Fact/Ref/Current/Readers和`runtimeports.RegistrySnapshotRefV1`共同组成三Owner共享的完整public contract。Registry Ref与Assembly composite只由Runtime ports定义；Harness/Tool只获得两个Model窄Reader capability，不获得Historical/Current publish口，也不得复制一套私有wire schema。

`PreparedModelInvocationCommitAckV1`固定为：

```text
ContractVersion = praxis.model-invoker.prepared-model-invocation-commit-ack/v1
ID / Revision=1 / Digest
PreparedRef       complete PreparedModelInvocationRefV1
CurrentRef        complete PreparedModelInvocationCurrentRefV1
GateImplementationRef
SurfaceBindingRef complete neutral ref: Owner + ContractVersion + ID + Revision + Digest
CheckedUnixNano
ExpiresUnixNano
NotAfterUnixNano
```

Model neutral Ref的唯一prototype为：

```go
type PreparedModelInvocationSurfaceBindingRefV1 struct {
    Owner           core.OwnerRef `json:"owner"`
    ContractVersion string        `json:"contract_version"`
    ID              string        `json:"id"`
    Revision        core.Revision `json:"revision"`
    Digest          core.Digest   `json:"digest"`
}
```

它必须与Tool Owner公共`ToolSurfaceInvocationBindingRefV1`在字段名、字段类型、JSON tag、顺序语义和Validate约束上逐字段同形：

```text
ModelAck.SurfaceBindingRef.Owner           == ToolBindingRef.Owner
ModelAck.SurfaceBindingRef.ContractVersion == ToolBindingRef.ContractVersion
ModelAck.SurfaceBindingRef.ID              == ToolBindingRef.ID
ModelAck.SurfaceBindingRef.Revision        == ToolBindingRef.Revision
ModelAck.SurfaceBindingRef.Digest          == ToolBindingRef.Digest
```

live Tool候选Ref仍是`Kind + ID + Revision + Digest`，与本冻结合同不一致；这是Tool Owner必须版本化修正的P0 Port Delta。Model不得接收Kind版Ref、做Kind→Owner/ContractVersion推断或维护转换表。

- ACK ID只从完整Prepared Ref + Current Ref派生，确保一个Invocation epoch不能靠更换Binding创建第二ACK；
- ACK Digest使用严格domain-separated canonical body覆盖除自身`Digest`外全部字段；ACK不内嵌AckRef，不形成digest闭环；
- `PreparedModelInvocationCommitAckRefV1`完整包含`ContractVersion + ID + Revision + Digest + PreparedRef + CurrentRef + SurfaceBindingRef + Checked/Expires/NotAfter`，其身份和Digest逐字段复制ACK；
- `Ack.PreparedRef == Current.PreparedRef == Fact.Ref`；
- `Ack.Checked >= Current.Checked`、`Ack.Checked < Ack.Expires <= Current.Expires`；
- `Ack.NotAfter == Current.NotAfter == Fact.NotAfter`，Gate绝不能扩大current窗口；
- Surface Binding Ref必须能由Tool公共Exact Reader按Owner/Contract/ID/Revision/Digest读取；裸BindingDigest不构成跨Owner坐标。
- Model ACK不携带、预计算或要求`ToolSurfaceInvocationBindingAckRefV1`。Tool Ack身份与Repository恢复属于Tool Owner，不进入Model ACK canonical body或AckRef。

actual-point复读固定为单Ref输入：

```text
InspectExactToolSurfaceInvocationBindingV1(
  ctx,
  complete ToolSurfaceInvocationBindingRefV1
) -> complete ToolSurfaceInvocationBindingV1,
     complete ToolSurfaceInvocationBindingAckV1,
     error
```

Harness/host从Model ACK取得neutral SurfaceBindingRef，逐字段转换为同形Tool BindingRef，并以该Ref作为唯一业务参数调用Tool exact Reader。Reader从同一Tool create-once Repository返回Binding + ToolAck deep clones；调用方不得提供ToolAckRef、Invocation coordinate、latest selector或私有request DTO。返回后必须验证`Binding.Ref == input Ref`、`ToolAck.BindingRef == input Ref`以及Binding/ToolAck canonical/current；任一漂移、过期、Unavailable或Indeterminate均使Provider调用数为0。

Harness/host只负责实现并注入上述exact CommitGate；Model负责在每条真实dispatch路径调用`Commit/InspectExactAck`并验证ACK。Harness调用Gate时必须同时使用Model公共`PreparedModelInvocationReaderV1`和`PreparedModelInvocationCurrentReaderV1`，不能信任调用方payload副本。Gate实现组合Tool-owned Surface/Binding Exact Reader，但Model公共合同不import Tool internal/implementation或Harness/Application/Runtime私有DTO。

Tool Owner拥有Surface Binding、Tool Ack的create-once identity、Repository和单Ref Exact Reader；Harness/host Gate实现Owner拥有Model ACK的create-once Repository及`InspectExactAck`；Model只拥有SurfaceBindingRef/ACK nominal、canonical校验和dispatch guard，不拥有Tool Binding/Tool Ack或Model ACK存储。

Binding/ACK只证明`Invocation -> Surface`因果绑定已经存在，不授予Provider、Tool、Harness或Runtime执行权。实际外部调用仍需Runtime policy、scope、secret、network/process和Effect等各自Owner的授权。

## 5. create-once、恢复与时间

### 5.1 Historical Fact

- Repository对`ID`和`InvocationID + InvocationDigest`建立同一线性化域的双索引；
- 同key、同canonical内容返回幂等成功；
- 同ID或同Invocation坐标换任一内容返回`Conflict`；
- `Indeterminate`只能使用本地已sealed同一Fact对同一原子Ensure最多恢复一次；不得换ID、时间、snapshot或digest；
- Reader exact找到后必须canonical wire exact；Authoritative NotFound仅能由同Repository线性化证明从未创建。

### 5.2 Current与ACK

- Current和ACK都不能复用过期回执；
- Current Repository必须同时按Current ID和完整Prepared Ref建立同一线性化域唯一索引；同Prepared Ref换Checked/Expires/content为Conflict，保证一个Invocation epoch只有一个session-level Current；
- Gate lost reply只允许对完全相同Prepared Ref + Current Ref幂等重试一次；
- 一个Invocation epoch只能有一个Surface Binding/ACK；同Invocation换SurfaceBindingRef、Surface或snapshot一律Conflict，不能update in place；
- 每个Provider attempt、Stream/Open和continuation边界只exact Inspect/Validate同一个Current和ACK，并产生非权威Dispatch Validation Receipt；
- retry/continuation跨越Current或ACK TTL立即fail closed；不得在同Invocation内refresh、renew或创建第二Binding；
- `Unavailable`、普通/unknown NotFound、第二次Indeterminate、Conflict、digest drift、expired、clock rollback全部fail closed；
- 时钟检查序列必须满足`t0 seal <= t1 fact/current ensure <= t2 gate ack <= t3紧邻外部调用 < min(Current.Expires, Ack.Expires, Fact.NotAfter, context deadline)`；任一后读时间早于前次checked视为rollback；
- 不承诺transport exactly-once，只承诺canonical create-once和同一sealed输入的有界恢复。

## 6. PurePrepare与Capability因果边界

PurePrepare必须是可白盒证明的纯函数域：

- 允许：Request/Plan Validate、canonical digest、离线Catalog读取、sealed Profile/Route/Capability/Registry snapshot exact读取、provider request纯映射；
- 禁止：`Provider.Capabilities`、`Backend.Resolve`、secret resolver、adapter pool、factory创建、网络、文件/进程副作用、SDK initialize、session创建、`Invoke/Stream/Open`。

`CapabilitySnapshotRef`封存PurePrepare所使用的能力合同。Gate后允许再次调用`Provider.Capabilities`做运行时验证，但它只能验证、不能改变已经sealed的Tools、ToolChoice、ParallelToolCalls、ActualToolSurfaceDigest或ActualProviderInjectionDigest；任何差异都在推理调用前失败。

Capabilities分为：

| 分类 | Gate前规则 |
|---|---|
| `sealed_local_snapshot` | PurePrepare可exact读取 |
| `local_pure_evaluation` | `EvaluateCapabilities`等纯计算可执行 |
| `provider_method_local` | 仍不得Gate前调用，避免接口实现未来外呼 |
| `remote_or_process_discovery` | 只能Gate后执行，并受独立外部发现/权限合同约束 |

CommitGate不替代Runtime对secret、process、network或Effect的授权；它只证明Model Preparation与Surface绑定已完成。

## 7. 全路径no-bypass表

| 路径 | Gate前允许 | 必须验证current ACK的位置 | Gate后仍需检查 | 失败时硬结果 |
|---|---|---|---|---|
| `model-invoker/execution.Runtime.Start` | Invocation clone/Validate、PurePrepare、seal/Ensure | `Adapter.Preflight`之前 | 重算两个tool digest；Context Frame conformance走独立链 | Preflight call count=0 |
| Direct `Preflight` | 纯`mapRequest`与离线Route选择 | `Backend.Resolve`之前 | Resolution Route/Model exact | Resolve count=0 |
| `model-invoker/execution/harness`六类Adapter | 纯map与静态config digest | process start/initialize之前 | 重新观测provider tool mapping；Context ActualInjectionManifest不参与Surface等式 | process/session/prompt count=0 |
| root `Invoker.prepare` | Validate、Protocol选择、sealed Capability snapshot、纯Evaluate | `Provider.Capabilities`之前 | Capabilities结果只能验证sealed snapshot | Capabilities/Invoke/Stream count=0 |
| `RouteInvoker` | 离线catalog/policy选择 | 向root Invoker移交前携带exact refs | root guard必须再次验证 | provider count=0 |
| `routegateway.Gateway` | 离线Route与provider request纯映射 | secret、pool lease、factory/provider创建之前 | root guard再次验证 | secret/pool/provider count=0 |
| `Adapter.Open` | 无新增映射 | Open入口复验同一Prepared/Current/ACK | Invocation/Plan/两个tool digest exact | Open count=0 |
| `Provider.Invoke` | 无 | 每次attempt紧邻调用前 | response identity与normalization | attempt count=0 |
| `Provider.Stream` | 无 | `Stream`紧邻调用前 | nil stream/identity/terminal | stream call count=0 |
| retry | backoff与ctx检查 | 每个attempt exact Inspect同一Current/ACK并生成本地Validation Receipt | 两个tool digest必须不变；跨TTL拒绝 | 下一attempt count=0 |
| Direct continuation | tool result已进入新Frame、纯构造next request | 每次`Backend.Invoke/OpenStream`前 | 同Invocation epoch内两个tool digest exact | continuation count=0 |
| Realtime `Open` | strict decode配置、证明或提取tool surface | `Provider.Open`之前 | ClientEvent不得偷偷改变surface | Open count=0 |
| Hosted/builtin tool route | 纯映射hosted tool options | 任何provider/hosted execution前 | hosted options只纳入ActualProviderInjectionDigest | hosted call count=0 |

Direct、RouteGateway和低层Invoker不得各自维护不同Gate算法；实现时必须收口到一个Model-owned canonical guard。低层公开API若没有Prepared Ref、Current Ref和Gate依赖，只能标为legacy/ungoverned且不得进入production composition root；最终目标是所有可达真实dispatch都无法绕过guard。

## 8. retry、continuation与Realtime epoch

- 同一Invocation只有一个session-level Surface Preparation、一个Current和一个create-once Binding/ACK。它只在`InvocationDigest + RequestToolsDigest + RouteDigest + ProfileDigest + ActualToolSurfaceDigest + ActualProviderInjectionDigest + Capability/Registry snapshot refs`全部不变且TTL未跨越时可被逐attempt复验；
- retry只改变attempt，不改变Prepared Historical Fact、Current或Binding；每个attempt都必须重新Inspect/Validate并产生新的非权威Validation Receipt；
- live Direct `nextRequest`只改变Input/State并保持session request中的Tools/ToolChoice/ParallelToolCalls，因此可以复用同一Invocation Surface；设计与实现仍必须在每次continuation重算ActualToolSurfaceDigest和ActualProviderInjectionDigest并exact比较；
- Direct continuation若改变工具surface、ToolChoice、ParallelToolCalls或provider映射，必须创建新的`InvocationID/InvocationDigest` epoch，重新PurePrepare、seal和Commit；禁止在同Invocation内更新Binding；
- Realtime `Configuration`当前是opaque bytes，默认不能证明无Tools。每个Realtime adapter必须提供strict、provider-specific的tool injection extractor，或提供canonical `NoToolSurfaceProofV1`；否则Realtime Open fail closed；
- Realtime ClientEvent若可能修改Tools/ToolChoice/hosted tools，必须新epoch并先Commit；无法证明不修改时拒绝发送；
- Batch、background、hosted tool、computer-use等只要能携带模型可见工具，都遵守同一规则。只有公开合同能证明`ToolSurfacePresence=none`的路径才可豁免。

## 9. 与ToolCall Observation的关系

`ToolCallCandidateObservationRefV1`只能通过相同`InvocationID/InvocationDigest`回链Prepared Historical Fact。它在terminal后才产生，不能证明：

- provider调用前是否已有Prepared Fact；
- 当时是否有current projection；
- Gate是否返回过current ACK；
- retry或continuation是否逐次复验。

因此任何“terminal Observation存在，所以pre-dispatch合规”的推断都必须拒绝。Observation仍是Provider输出历史事实，不升级PendingAction或dispatch资格。

## 10. 错误归一化

公共错误至少区分：

- `Invalid`：字段、canonical、typed-nil或时序无效；
- `Conflict`：同身份/坐标换内容；
- `AuthoritativeNotFound`：同一线性化Repository证明从未创建；
- `UnknownNotFound`：eventual/retention/未知缺失；
- `Unavailable`：Repository、Reader、Gate或snapshot reader不可用；
- `Indeterminate`：调用结果未知；
- `Drift`：Request/Plan/Route/Profile/ActualToolSurface/ActualProviderInjection/snapshot不匹配；
- `Expired`：Current、ACK或Fact上界失效；
- `ClockRegression`：时钟回退。

除同sealed输入的有界恢复外，上述错误全部在第一次外部调用前fail closed。

## 11. Owner与非目标

Model Invoker拥有：

- Prepared/Current identity、canonical codec、digest、Repository/Reader Port；
- PurePrepare、两个分离的tool digest和canonical dispatch guard；
- 全Model dispatch路径上的CommitGate调用与ACK验证；
- 每次dispatch attempt的非权威current validation receipt；
- terminal Observation到Prepared父Invocation坐标的exact回链。

Model Invoker不拥有：

- Tool Surface、Registry内容、Binding、PendingAction、ActionCandidate或Dispatch；
- Context Expected/ActualInjectionManifest、ProviderActualInjectionObservation与InjectionConformance Fact；
- Runtime Operation Settlement、Evidence Ledger或Effect；
- production持久化driver、composition root、retention SLA；
- secret material、provider pool或账号认证。

本设计只冻结Port和因果顺序，不宣称live代码、production backend或root已经存在。

## 12. 实施前Delta裁决

### P0

Model本地合同已基本闭合。跨Owner external P0只保留一项：实现已冻结候选对应的Runtime public Go nominal/Reader。

1. Model Profile Compiler公开并seal完整`ProfileDigest`；不得用ProfileKeyDigest替代。
2. Runtime Owner实现asset candidate已冻结的`runtimeports.RegistrySnapshotRefV1`、`RegistrySnapshotExactReaderV1`及Assembly Current Ref/Projection/Reader public Go nominal；Registry Authority仍是Ref.Owner，Model/Harness/Tool直接使用Runtime concrete types，不alias/复制。
3. 冻结ActualToolSurfaceDigest的Tool公共canonical Port和唯一等式；与Model richer provider digest、Context Frame链严格分离。
4. 将RouteGateway/Direct/model-invoker execution Harness映射拆成零外部PurePrepare和Gate后执行；尤其Gate必须早于`Adapter.Preflight`。
5. 冻结Prepared/Current/CurrentRef/ACK/AckRef/BindingRef canonical、clock、唯一索引和原子Repository合同。
6. 建立单一Model-owned guard，所有tool-bearing真实dispatch强制使用。
7. Tool BindingRef版本化为Owner/ContractVersion/ID/Revision/Digest同形Ref，并提供单Ref exact Reader返回Binding+ToolAck；Model Ack不携ToolAckRef。

### P1

1. 迁移root Invoker、RouteInvoker、RouteGateway、Direct首次调用/continuation、六类Harness、retry和Realtime到统一guard。
2. 为Capabilities提供sealed snapshot，并把provider方法调用后移到Gate后且禁止改变两个tool digest。
3. 为Realtime/opaque provider options提供strict tool extractor或NoToolSurfaceProof。
4. 提供线程安全内存reference store/conformance；production driver/root继续NO-GO。

### P2

1. 提供按路径分组的before-ACK外部调用计数与审计字段。
2. 补充observability、性能基线和长期retention适配，但不得改变V1身份。

## 13. 关联资产

- [测试矩阵](./prepared-model-invocation-pre-dispatch-gate-v1-test-matrix.md)
- [实施计划](../../plan/model-invoker/prepared-model-invocation-pre-dispatch-gate-v1.md)
- [ToolCall Projection发布与Exact Reader V1](./tool-call-observation-projection-publish-reader-v1.md)
- [Tool Call候选观测V1](./tool-call-candidate-observation-v1.md)
- [组件治理](../../properties/architecture/component-governance/README.md)
