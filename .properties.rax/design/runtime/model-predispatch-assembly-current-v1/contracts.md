# Model Pre-Dispatch Assembly Current V1合同

状态：**Runtime中立Go合同已实现并通过双独立代码审计YES（P0/P1/P2=0/0/0）及全门；Harness事实发布与相邻Owner适配仍未授权**。

## 1. Contract Version与canonical域

- `ModelPreDispatchAssemblyCurrentContractVersionV1 = "1.0.0"`；
- canonical域固定为`praxis.runtime.model-predispatch-assembly-current/v1`；
- `RegistrySnapshotRefV1`使用独立域`praxis.runtime.registry-snapshot-ref/v1`；
- 所有ID、revision、digest和时间字段均为closed字段，不接受自由`map`、ObjectRef、Kind或alias。

## 2. Registry exact ref

```go
type RegistrySnapshotRefV1 struct {
    Owner           core.OwnerRef `json:"owner"`
    ContractVersion string        `json:"contract_version"`
    ID              string        `json:"id"`
    Revision        core.Revision `json:"revision"`
    Digest          core.Digest   `json:"digest"`
}
```

`Owner/ContractVersion/ID/Revision/Digest`五组坐标必须完整有效。该类型只表达Registry Owner签发的exact ref；Runtime、Harness、Model和Tool均不得根据裸digest补全、重签或改变Owner。三方旧Registry局部命名必须在后续各Owner适配中收敛到此唯一neutral类型，不能保留第二alias。

### 2.1 唯一Registry exact Reader

```go
type RegistrySnapshotExactReaderV1 interface {
    InspectExactRegistrySnapshotV1(
        context.Context,
        RegistrySnapshotRefV1,
    ) (RegistrySnapshotRefV1, error)
}
```

第二参数本身就是唯一closed request：必须携完整`Owner/ContractVersion/ID/Revision/Digest`，不得新增ID-only、latest、name、裸digest或自由selector请求。Reader实现Owner由`request.Owner`确定；composition必须把调用路由到该Authority Owner的Repository，并在任何返回前验证repository authority与`request.Owner` exact相同。错误Owner、跨Owner同ID或无法证明Authority均Fail Closed。

读取顺序固定为：request Validate → typed-nil/dependency preflight → Authority Owner historical exact read → stored Ref Validate → stored/request逐字段exact → Owner current pointer读取 → current exact比较 → deep clone return。返回值必须是stored Ref的deep clone，禁止返回内部别名、修正request或按current pointer重写字段。

historical/current语义分离如下：

- Authority Repository可保留旧revision作为historical immutable record；historical存在不表示可用于pre-dispatch；
- 本唯一public Reader是**current exact Reader**：旧historical Ref即使仍可由Owner内部审计读取，也必须因不等Owner current pointer返回`PreconditionFailed`，不返回旧Ref；
- current pointer exact相等仍须完整Ref historical读取成功；current index、latest或裸ID不能替代immutable record；
- Reader只返回Ref clone，不返回Registry内容、mutation、publish、CAS或current pointer写能力。

closed errors固定为：request/returned shape非法=`InvalidArgument`；Authority Owner下ID或revision不存在=`NotFound`；same ID或跨Owner的version/revision/digest/body漂移=`Conflict`；历史存在但已非current、revoked或不再可用=`PreconditionFailed`；Repository不可用=`Unavailable`；current/authority无法判定=`Indeterminate`。不得把Unavailable/Indeterminate降级为NotFound。

## 3. Assembly field-specific refs

```go
type ModelPreDispatchAssemblyExactRefV1 struct {
    ID       string        `json:"id"`
    Revision core.Revision `json:"revision"`
    Digest   core.Digest   `json:"digest"`
}

type ModelPreDispatchAssemblyBindingSetRefV1 struct {
    ID                string        `json:"id"`
    Revision          core.Revision `json:"revision"`
    Digest            core.Digest   `json:"digest"`
    SemanticDigest    core.Digest   `json:"semantic_digest"`
    CurrentnessDigest core.Digest   `json:"currentness_digest"`
    ProjectionDigest  core.Digest   `json:"projection_digest"`
    ExpiresUnixNano   int64         `json:"expires_unix_nano"`
}
```

`ExactRef`只用于字段位置已经封闭的Handoff、Manifest、Conformance和ToolSurface，不能作为自由type-pun容器。BindingSet必须携完整事实、semantic、currentness、projection与TTL水位。

## 4. Current Ref与Projection

```go
type ModelPreDispatchAssemblyCurrentRefV1 struct {
    ContractVersion string                                   `json:"contract_version"`
    ID              string                                   `json:"id"`
    Revision        core.Revision                            `json:"revision"`
    Digest          core.Digest                              `json:"digest"`
    Generation      GenerationArtifactRefV1                  `json:"generation"`
    Handoff         ModelPreDispatchAssemblyExactRefV1       `json:"handoff"`
    BindingSet      ModelPreDispatchAssemblyBindingSetRefV1  `json:"binding_set"`
    Manifest        ModelPreDispatchAssemblyExactRefV1       `json:"manifest"`
    Conformance     ModelPreDispatchAssemblyExactRefV1       `json:"conformance"`
    WatermarkDigest core.Digest                              `json:"watermark_digest"`
    ToolSurface       ModelPreDispatchAssemblyExactRefV1     `json:"tool_surface"`
    ProfileDigest     core.Digest                            `json:"profile_digest"`
    RegistrySnapshot  RegistrySnapshotRefV1                  `json:"registry_snapshot"`
    SemanticDigest    core.Digest                            `json:"semantic_digest"`
    CurrentnessDigest core.Digest                            `json:"currentness_digest"`
    CheckedUnixNano   int64                                  `json:"checked_unix_nano"`
    ExpiresUnixNano   int64                                  `json:"expires_unix_nano"`
    ProjectionDigest  core.Digest                            `json:"projection_digest"`
}

type ModelPreDispatchAssemblyCurrentProjectionV1 struct {
    ContractVersion   string                                  `json:"contract_version"`
    Ref               ModelPreDispatchAssemblyCurrentRefV1    `json:"ref"`
    Generation        GenerationArtifactRefV1                 `json:"generation"`
    Handoff           ModelPreDispatchAssemblyExactRefV1       `json:"handoff"`
    BindingSet        ModelPreDispatchAssemblyBindingSetRefV1  `json:"binding_set"`
    Manifest          ModelPreDispatchAssemblyExactRefV1       `json:"manifest"`
    Conformance       ModelPreDispatchAssemblyExactRefV1       `json:"conformance"`
    ToolSurface       ModelPreDispatchAssemblyExactRefV1       `json:"tool_surface"`
    ProfileDigest     core.Digest                              `json:"profile_digest"`
    RegistrySnapshot  RegistrySnapshotRefV1                    `json:"registry_snapshot"`
    SemanticDigest    core.Digest                              `json:"semantic_digest"`
    CurrentnessDigest core.Digest                              `json:"currentness_digest"`
    CheckedUnixNano   int64                                    `json:"checked_unix_nano"`
    ExpiresUnixNano   int64                                    `json:"expires_unix_nano"`
    ProjectionDigest  core.Digest                              `json:"projection_digest"`
}
```

Projection必须逐字段证明`Ref.Generation/Handoff/BindingSet/Manifest/Conformance/ToolSurface/RegistrySnapshot`与外层对应字段exact相等，并要求Ref与Projection的`ProfileDigest/SemanticDigest/CurrentnessDigest/CheckedUnixNano/ExpiresUnixNano/ProjectionDigest`全部exact一致。`CheckedUnixNano > 0 && CheckedUnixNano < ExpiresUnixNano`；caller不能提供、延长或重封TTL。

## 5. ID、revision与摘要

- `ID`只由稳定subject派生：contract version、Generation/Handoff/BindingSet/Manifest/Conformance/ToolSurface稳定ID、ProfileDigest、Registry Snapshot的Owner/ContractVersion/ID；不得含revision、current digest或时间；
- Revision由Harness Assembly Owner单调CAS；same ID+same revision任一内容漂移为Conflict；禁止ABA；
- `SemanticDigest`覆盖完整Registry exact ref、Profile、ToolSurface、Generation/Handoff/BindingSet/Manifest/Conformance语义坐标；
- `CurrentnessDigest`覆盖Generation、Handoff与BindingSet Owner current Reader的exact输入/输出水位；
- `WatermarkDigest`覆盖Ref主体、完整Projection current坐标、Semantic/Currentness与严格TTL，但排除自身、Ref.Digest及ProjectionDigest；
- `ProjectionDigest`覆盖完整Projection canonical body，计算时同时清空`Projection.ProjectionDigest`、`Projection.Ref.Digest`与`Projection.Ref.ProjectionDigest`；最终这三者exact等于同一完成摘要，不形成循环摘要；

所有摘要计算均遵守own-ref/digest排除：正在计算的字段及其自引用外壳必须置空，已经完成的下游稳定摘要才可进入上一层body。禁止把`Ref.Digest`、`Ref.ProjectionDigest`、`Projection.ProjectionDigest`或正在计算的`WatermarkDigest`回灌进自身；Seal不得覆盖非零但错误的caller派生字段，错误非零值必须Conflict。

## 6. historical/current与错误

- 旧Ref可作为历史审计坐标，但单方法Reader只提供current读取；旧revision、revoked/expired/currentness漂移均Fail Closed；
- absent ID=`NotFound`；same-ID同revision换body/digest、type-pun或request-return exact漂移=`Conflict`；结构有效的historical exact Ref已非Owner current、过期、撤销或current pointer合法推进=`PreconditionFailed`；Reader不可用=`Unavailable`；无法判定=`Indeterminate`；
- nil/typed-nil Reader或任何Owner current依赖在读取前Fail Closed；
- Reader结果必须先`Validate()`，再与exact request Ref逐字段比较。

## 7. 禁止项

- Runtime不得实现Harness Store、publisher、semantic assembler或current index；
- Tool不得定义第二份Ref/Projection、alias、echo DTO或重新计算摘要；
- 本Projection不是Runtime Effect、Provider Permit、PreparedAttempt、Enforcement或Surface invocation授权；
- compile HookFace、Manifest presence或Tool Surface存在均不得替代真实Provider gate。
- Registry Reader不授Registry内容读取、publisher、CAS或mutation；verified Registry Ref也不授Provider、Prepared、Permit或Enforcement权力。
