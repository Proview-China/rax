# Workspace Read Admission Attempt Binding V2

## 目标

该 additive Sandbox Owner 合同封闭一条历史恢复坐标：

```text
exact Runtime OperationDispatchAttemptRefV3
  -> exact WorkspaceRead Admission
  -> original WorkspaceReadAttemptRefV1
```

V1 wire 与 digest 不修改。V2 只证明既有事实的历史因果关系，不授予 current、
执行、重试或恢复物理读取的资格。

## Owner 与公开边界

Sandbox 是 Reservation、WorkspaceRead Command、Admission、original Attempt、
V1 binding 与 V2 reverse binding 的唯一 Owner。Runtime Attempt 仅作为完整
exact 外部坐标被引用，Runtime 不复制或签发第二份 Sandbox 事实。

公开接口只有只读 Reader：

```go
InspectWorkspaceReadAdmissionForRuntimeAttemptV2(
    context.Context,
    runtimeports.OperationDispatchAttemptRefV3,
) (WorkspaceReadAdmissionAttemptBindingV2, error)
```

请求必须携带完整 Runtime Attempt，包括 Delegation presence/ID/revision/digest。
禁止 AttemptID-only、stable-key、current 或 caller-provided snapshot 查询。
Tool、Runtime、Application、Harness 与 Console 均没有 V2 writer。

唯一 writer 由
`ExecutionRuntime/sandbox/internal/owner/workspaceread.AuthorizedReservationV2`
这一 Go internal nominal capability 保护。只有 Sandbox kernel 在 S1 current
closure 成功后才能构造；SQLite writer不接受 caller 自签的完整 V2 Fact。
`SealWorkspaceReadAdmissionAttemptBindingV2`只是纯 canonical helper，不赋权。

## V2 对象与闭包

`WorkspaceReadAdmissionAttemptBindingV2`封存：

- `ContractVersion / TypeURL`；
- 完整 `OperationDispatchAttemptRefV3`；
- 完整 V1 Admission-to-Attempt binding；
- `AuthorizationDigest`；
- `Association`、`DomainCommand`；
- immutable `WorkspaceReadCommandV1`；
- `AdmissionReceipt`；
- original `WorkspaceReadAttemptRefV1`；
- V2 canonical `Digest`。

创建路径必须证明：

- Runtime Attempt 的 operation/effect/intent/permit/attempt/delegation 全轴与
  同一 `ControlledOperationPhysicalExecutionAuthorizationV3`完全一致；
- `AuthorizationDigest == authorization.DigestV3()`；
- Command 的 operation/effect/intent/attempt轴与 Runtime Attempt一致；
- V1 DispatchDigest由完整 Runtime Attempt重算，不能遗漏Permit或Delegation；
- Association、DomainCommand、Command、Admission与original Attempt全部精确；
- Reservation 的WorkspaceView、RequestDigest、PayloadDigest分别等于Command的
  WorkspaceView、Meta.Digest、SourceToolPayloadDigest；
- Command payload schema/revision/digest与Authorization prepared payload一致。

V1 Reservation、origin/current Attempt、V1 binding与V2 binding在同一 Sandbox
SQLite事务中create-once。相同Runtime Attempt的相同请求幂等；同Runtime Attempt
异body、或同Admission/original Attempt被第二Runtime Attempt绑定均Conflict。

## 历史读取、损坏与并发

Reader以完整Runtime Attempt digest定位唯一V2 winner，并在同一只读事务中严格
解码、重编码和逐列重算V2、V1 binding、Command、Reservation与origin Attempt。
V2行存在后，任何被引用行缺失均是Owner存储损坏，必须返回Conflict，不能降格为
NotFound。origin与Reservation各自的`stable_digest`列分别和自身strict-decoded
body中的`StableKeyDigest`比较，禁止协同篡改。

SQLite v18迁移对V2 table、三个unique index、PK autoindex、ledger与namespace
做严格shape验证；`index_list.seq`、`index_xinfo` key/aux/cid/name/desc和字面
`BINARY`均须精确。ledger存在时不得silent repair；旧schema出现任意partial V2
namespace对象时Fail Closed。

V2是append-only历史事实，不以TTL作为执行资格。即使关联执行事实已经过期，
仍可用同一exact Runtime Attempt读取历史binding并继续Inspect original Attempt；
不得重新执行物理read。lost reply、restart与UnknownOutcome只能沿原坐标Inspect。

## 兼容与 NO-GO

- V1合同、wire、digest和兼容入口不变；
- actual physical path必须走完整V2 authorization→internal capability路径；
  V1-only owner store不能成为旁路；
- public Reader读取计数为零写、Provider调用为零；
- 本切片不修改Runtime、Tool、Application、Harness、Host或Console；
- Tool full composition、跨Owner product root及production readiness继续NO-GO。
