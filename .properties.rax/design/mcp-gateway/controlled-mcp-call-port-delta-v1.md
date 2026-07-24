# 受治理MCP tools/call actual-point公共Port Delta V1

## 1. 当前裁决

状态：actual-point授权与正式Observation切片已实现并通过 owner-local 软件门。Runtime public
`controlled_operation_physical_execution_v3.go`、Tool
`MCPExecutionCommandFactV1`/current Reader、Prepared-domain-command Association、
Runtime create-once Association Gateway/V3授权签发器、official SDK physical executor、
create-once admission及Receipt→Evidence→Observation已落盘；这不代表
production root 或端到端 Operation Settlement 已完成。

live Runtime `ControlledOperationProviderRequestV2`与Prepared V2只绑定Payload Schema/Digest/Revision、Provider Binding和Attempt；kernel私有actual-point Transport Request不携Tool Owner canonical command，也不能被Tool包实现。仅凭相同PayloadDigest无法证明Tool、MCP Server、Connection Epoch、Snapshot Tool和JSON-RPC Request ID，V2包装或从Runtime私有类型复制字段均不可接受。

## 2. 用例与Owner

| 对象/动作 | 语义Owner | 非Owner |
|---|---|---|
| MCP Tool映射、canonical call command、Connection/Snapshot current、Protocol Receipt | Tool/MCP | Runtime、Application、Harness |
| Operation/Effect/Prepared/Attempt、actual-point authorization、统一NotAfter | Runtime | Tool/MCP |
| BindingV2到Operation准备与跨域提交顺序 | Application Coordinator | Tool/MCP、Runtime不反向编排 |
| public neutral physical execution Port装配 | 宿主composition root | Runtime/Tool均不import对方实现 |
| Review/Authority/Budget/Fence/Scope | 各治理Owner/Runtime Gateway | Tool/MCP不得写入或推断 |

## 3. Tool Owner exact对象

### 3.1 `MCPExecutionCommandRefV1`

字段：`ID, Revision, Digest`。`Revision=1`；ID由`BindingV2 Ref + Prepared Ref + Attempt Ref + Connection Ref/Epoch + Snapshot Ref + Snapshot Tool ObjectDigest` canonical派生，不包含Checked/Expires等fresh字段。

### 3.2 `MCPExecutionCommandFactV1`

必需字段：

- `ContractVersion, Ref`；
- `BindingCurrentRefV2, CandidateV3 Ref, InputContractCurrentRefV1`；
- `CapabilityRef, ToolRef`，且Tool Mechanism必须为`mcp`；
- `MCPServerRef, MCPConnectionRef, ConnectionEpoch, MCPCapabilitySnapshotV2 Ref`；
- `SnapshotToolName, SnapshotToolObjectDigest, SnapshotInputSchemaDigest`；
- `Method="tools/call", JSONRPCRequestID`；
- `ParamsSchema, ParamsDigest, ParamsRevision, Params canonical bytes`；
- Runtime public `Operation, OperationDigest, EffectID/Revision, IntentDigest, Prepared Ref, Attempt Ref, ProviderBinding`；
- `CreatedUnixNano, NotAfterUnixNano, Digest`。

硬门：Params bytes/digest/schema/revision必须等于CandidateV3 Payload与Prepared Payload；Tool/Capability/InputSchema必须等于BindingV2 closure；Connection必须绑定同Tenant/Identity/Run且Server/Snapshot/Epoch exact；Snapshot Tool必须唯一匹配Tool mapping，不能按name/latest弱选；Provider Binding必须等于BindingV2 Route Provider。Fact只是领域Intent，不授执行权。

### 3.3 Store与Reader

```go
type MCPExecutionCommandStoreV1 interface {
    CreateMCPExecutionCommandV1(context.Context, MCPExecutionCommandFactV1) (MCPExecutionCommandFactV1, error)
    InspectMCPExecutionCommandV1(context.Context, MCPExecutionCommandRefV1) (MCPExecutionCommandFactV1, error)
}

type MCPExecutionCommandCurrentReaderV1 interface {
    InspectCurrentMCPExecutionCommandV1(context.Context, MCPExecutionCommandRefV1) (MCPExecutionCommandCurrentProjectionV1, error)
}
```

Projection返回同一immutable Fact、BindingV2/Connection/Snapshot exact current closure、`Checked/Expires/ProjectionDigest`。S1创建前与actual-point读取均复读exact refs；expiry取BindingV2、Connection、Snapshot、Prepared、caller deadline的共同最小值。NotFound才允许首次create；Unavailable/Indeterminate/lost create reply只按派生Ref Inspect，不重新准备或换Request ID。

## 4. Runtime additive中立Delta

### 4.1 Prepared与领域命令关联

Runtime需发布中立`OperationDomainCommandRefV1{Owner, Kind, ID, Revision, Digest}`及additive Prepared-domain-command Association/current Reader。Association至少exact绑定：

- `OperationDigest, EffectID/Revision, IntentDigest`；
- `PreparedProviderAttemptRefV2, OperationDispatchAttemptRefV3, ProviderBindingRefV2`；
- `PayloadSchema/Digest/Revision`；
- `DomainCommandRef`；
- `Checked/Expires/Digest`。

不能修改V2 canonical后假装兼容；采用additive V3 Prepared/Request，或独立versioned Association并由Gateway强制读取。Association不解释MCP字段，只证明同一Prepared/Attempt绑定同一领域命令。

### 4.2 public actual-point授权与执行Port

Runtime需把kernel私有Transport seam替换或补充为`runtime/ports` public neutral V3：

```go
type ControlledOperationPhysicalExecutionAuthorizationV3 struct {
    StableKeyDigest  core.Digest
    UnifiedNotAfterUnixNano int64
    ProviderTransport ProviderBindingRefV2
    Provider          ProviderBindingRefV2
    Operation         OperationSubjectV3
    OperationDigest   core.Digest
    EffectKind        EffectKindV2
    Prepared          PreparedProviderAttemptRefV2
    Attempt           OperationDispatchAttemptRefV3
    ExecuteEnforcement OperationDispatchEnforcementPhaseRefV4
    ExecuteEvidenceHandoff OperationScopeEvidenceProviderHandoffRefV3
    Boundary          OperationProviderBoundaryRefV1
    DomainCommand     OperationDomainCommandRefV1
    AuthorizationDigest core.Digest
}

type ControlledOperationPhysicalExecutionPortV3 interface {
    ExecuteControlledOperationPhysicalV3(context.Context, ControlledOperationPhysicalExecutionAuthorizationV3) (ControlledOperationProviderAdmissionReceiptRefV2, error)
}
```

live Runtime名为`ControlledOperationPhysicalExecutionAuthorizationV3`与
`ControlledOperationPhysicalExecutionPortV3`，并额外包含
`OperationScopeDigest`与Prepared-domain-command `Association`。Runtime在调用前完成
自身Gateway current闭包，Tool actual executor仍必须在物理写入入口用自身fresh
clock验证`UnifiedNotAfter`、exact DomainCommand current、ProviderTransport/Provider
与Prepared/Attempt。验证后再转调V1或把Tool Reader结果交回Runtime执行都不能闭合
actual point。

live还提供`PreparedDomainCommandAssociationPortV1`与
`ControlledOperationPhysicalAuthorizationPortV3`：前者由Runtime Owner按stable
Prepared/Attempt/DomainCommand ID create-once持久关联并只返回sealed current Projection；
后者复读V2 Route/七Binding/Effect/Prepared/Policy/Enforcement/Handoff/Evidence/Boundary
完整current闭包与该Association，以fresh S1/S2 clock签发统一NotAfter授权。两者都不调用
Provider；过期Association不续租，lost create reply只按派生ID Inspect winner。

### 4.3 Receipt到正式Observation的已实现additive公共Delta

Tool physical executor只产出`MCPProtocolReceiptV1`和Runtime admission receipt；二者
都是外部观察，不是`EvidenceRecordRefV2`或正式
`ProviderAttemptObservationRefV2`。现已在live Runtime additive实现：

- `EvidenceGovernancePortV2.AppendGoverned`：重验Evidence Source、Authority、Scope、
  Policy并写入Ledger；
- `OperationObservationGovernancePortV3.RecordGovernedProviderObservationV3`：重验
  Intent/Permit/Fence/Attempt及exact Evidence后提交Provider Observation；
- `OperationProviderReceiptRefV1/ProjectionV1/ReaderV1`与
  `OperationProviderReceiptObservationPortV1`，负责串联exact Receipt、Evidence append和
  Observation记录。

`OperationProviderReceiptObservationPortV1`由Runtime治理面拥有，输入只接受exact
Intent/Permit/Fence/Attempt、Tool-owned domain Receipt Ref与专属Evidence Source Registration
Ref；输出只返回Runtime正式Observation Ref。V1采用每Prepared Attempt一个专属Source：

1. Source初始Revision=1且`NextSourceSequence=1`，唯一sequence固定为1；lost reply只按该
   exact source key Inspect，不重新分配；
2. 调用现有Evidence Gateway取得真实`EvidenceRecordRefV2`，不得接收Tool自报Evidence；
3. 再调用现有Observation Gateway并返回其正式Ref；
4. append成功而Observation回包丢失时只Inspect同source key/delegation/prepared attempt；
5. Unavailable/Indeterminate不当作NotFound，禁止重分配sequence或重发Provider调用。

该协调Port与Tool MCP Receipt只读adapter已完成owner-local/reference-test软件门。真实
MCP DomainResult与Settlement接线、production Source provisioning/root仍保持NO-GO。

## 5. 精确顺序

```text
BindingV2 current
  -> Tool create/Inspect Action/Reservation
  -> Runtime Intent/Admission/Permit/Begin/Prepare
  -> Tool Ensure MCPExecutionCommand Fact
  -> Application submit Prepared <-> DomainCommand Association
  -> Runtime prepare Enforcement/Evidence
  -> Runtime execute Enforcement/Evidence/Boundary
  -> Runtime V3 actual-point authorization
  -> Tool physical executor InspectCurrent exact command + fresh clock + create-once admission
  -> official MCP SDK CallTool on exact Session
  -> Tool MCPProtocolReceipt Observation
  -> Runtime receipt-observation coordinator appends Evidence with owner sequence
  -> Runtime Observation Gateway obtains ProviderAttemptObservationRefV2
  -> Tool DomainResult -> Runtime Settlement V4 -> Tool Apply/ToolResult
```

MCP SDK回包、Error、Task或Progress都只是Observation；不得直接成为DomainResult、Settlement或ToolResult。

## 6. Effect、恢复与反例

- Effect固定`praxis.tool/execute`、OperationScope=`run`、Profile=`praxis.tool/single-call-action-v1`；N大于1、batch、custom effect继续NO-GO；
- physical admission持久成功即视为“可能已调用”；崩溃或丢Provider回包只Inspect同StableKey/Prepared/Attempt/RequestID，不重新CallTool；
- admission明确`rejected_no_effect`必须有可验证NoEffect Receipt且没有Provider Observation；
- Command/Association/authorization任一NotFound、Unavailable、Indeterminate、过期、clock rollback、same ID换digest、跨Attempt、换Connection Epoch、换Snapshot Tool、换Provider/Transport时Provider调用为0；
- PayloadDigest相同但Tool/Server/Connection/RequestID不同必须Conflict，证明不能只靠Prepared V2 Payload字段执行；
- V1/V2 alias、Runtime kernel私有type复制、raw official ClientSession/Transport注入、SDK/CLI直连Provider全部拒绝。

## 7. 兼容与实施门

- V1/V2现有公开合同保持不变并Fail Closed；不在旧类型追加字段改变canonical；
- Runtime Association create-once Gateway、V3 actual-point authorization issuer与Tool official
  SDK physical executor已完成owner-local软件测试；
- Runtime-owned 4.3协调Port已完成owner-local软件门，Tool不得伪造`EvidenceRecordRefV2`；
- production必须在执行前由宿主/Application持久创建每Attempt专属Evidence Source并保存exact Registration Ref；当前无production root；
- production真实Provider路径仍等待宿主root、持久State Plane/Session、Credential与系统验收；
- 生产stdio/Streamable HTTP、Credential Broker、Session Repository、持久State Plane与SLA另行启用，不由本Delta预选；
- 本Delta不触发pre-run Evidence；它只覆盖Run内single-call execute。Connect/Discover等Admin Effect仍需独立Operation治理。
