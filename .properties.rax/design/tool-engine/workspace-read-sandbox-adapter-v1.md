# workspace.read → Sandbox Adapter V1 Design Delta

## 状态

- `owner_local_implementation_under_independent_audit`
- 只覆盖Tool Owner `workspace.read`到Sandbox public Port的接线与只读恢复。
- CorePack `Executable=false`；没有production composition root、Artifact backend或完整ToolResult/Context接线。
- Sandbox依赖固定为`WorkspaceReadExecutionPortV2`，V1 inspection不得降级使用。

## Owner边界

- Tool拥有：sealed `WorkspaceReadSandboxRequestV1`、Tool输出物化、Runtime Admission关系的窄只读投影、Adapter恢复策略。
- Sandbox拥有：Command、Reservation、Attempt、Admission→Attempt binding、`WorkspaceReadInspectionEnvelopeV2`及实际Rust `openat2/pread`。
- Runtime拥有：`OperationDispatchAttemptRefV3`、physical authorization、`ControlledOperationProviderAdmissionReceiptRefV2`。
- Tool不创建Runtime/Sandbox事实，不直接文件IO，不从稳定key推导Sandbox Attempt。

## Tool-side additive read seam

Runtime live public ports尚无按原`OperationDispatchAttemptRefV3`精确读取Admission Receipt的接口，因此Tool仅冻结可注入关系Reader：

```go
type WorkspaceReadRuntimeAdmissionInspectionV1 struct {
    ContractVersion string
    Attempt         runtimeports.OperationDispatchAttemptRefV3
    Admission       runtimeports.ControlledOperationProviderAdmissionReceiptRefV2
    CheckedUnixNano int64
    ExpiresUnixNano int64
    Digest          runtimecore.Digest
}

type WorkspaceReadRuntimeAdmissionCurrentReaderV1 interface {
    InspectWorkspaceReadAdmissionForRuntimeAttemptV1(
        context.Context,
        runtimeports.OperationDispatchAttemptRefV3,
    ) (WorkspaceReadRuntimeAdmissionInspectionV1, error)
}
```

约束：

1. `Attempt`必须与请求exact全字段相同；只比`AttemptID`非法。
2. `Admission`必须valid、`Admitted=true`、`NoEffect=false`，且`StableKeyDigest`回扣历史authorization。
3. `Checked < Expires`，TTL最多30秒；fresh clock回拨或过期Fail Closed。
4. Projection digest使用`praxis.tool-mcp.workspace-read-runtime-admission`、
   `praxis.tool-mcp/workspace-read-runtime-admission-inspection/v1`和
   `WorkspaceReadRuntimeAdmissionInspectionV1` canonical body。
5. 该Projection不授执行权、不复制Runtime事实。当前仅有
   `UnsupportedWorkspaceReadRuntimeAdmissionCurrentReaderV1`零调用Fail Closed实现和test fixture；
   Runtime/宿主真实Reader未落，因此production recovery保持NO-GO。

Sandbox public Port已经提供TTL过期后仍可读取immutable historical Command的窄Reader：

```go
type sandboxports.WorkspaceReadCommandExactReaderV1 interface {
    InspectWorkspaceReadCommandExactV1(
        context.Context,
        sandboxcontract.Ref,
    ) (sandboxcontract.WorkspaceReadCommandV1, error)
}
```

恢复必须以`binding.Command`调用该Reader，再执行`Command.ValidateShape`并exact核对
`Meta.Ref`、`SourceToolCommand`、Payload Schema/Digest/Revision、WorkspaceView、
RelativePath、Range及Runtime authorization坐标。Runtime坐标包括按Sandbox既有
`praxis.sandbox.workspace-read/1.0.0/OperationDispatchAttemptRefV3` canonical重算的
`DispatchDigest`，以及`Operation.ExecutionScope.Identity.TenantID`。历史读取不校验
Command TTL，也不续权。

从terminal Projection读取Reservation后，Tool只回扣当前输入可证明的TTL关系：

- `UnifiedNotAfter == authorization.UnifiedNotAfterUnixNano`；
- `RuntimeEnforcementExpires <= authorization.ExecuteEnforcement.ExpiresUnixNano`；前者真实来源是
  Runtime current Enforcement projection，Tool没有该Reader，不能伪造exact equality；
- Command RequestedNotAfter与Command Meta expiry逐字段exact；
- `EffectiveExpires == Reservation.Meta.Expires == Binding.Expires`；
- `Reservation.RequestDigest == Command.Meta.Digest`；
- `Reservation.AttemptID == Binding.Attempt.ID`；
- Projection Admission的Checked/Expires分别等于Binding Created/Expires；
- Attempt expiry保持`min(Reservation expiry, Admission expiry)`。

Association、WorkspaceView、WorkspaceLease的具体expiry只依赖Sandbox Owner sealed
`TTLClosure.ValidateShape`，Tool当前不可独立证明，继续作为reference production NO-GO缺口。
已验证的完整Command必须一直传到输出物化；若`ExpectedFileRef != nil`，Observation File必须
完整`SameRef`，仅核File ID/revision/path不足。
Tool不再定义重复的Sandbox Command Reader、兼容接口或对应Unsupported包装；构造器直接
消费Sandbox public Port，SQLite Store和Sandbox executor已实现该Reader。正常Start路径仍只
消费Command Current Reader的S1/S2，不得回退到历史Reader。Tool仍保留独立的
`UnsupportedWorkspaceReadRuntimeAdmissionCurrentReaderV1`作为Runtime Admission零调用
Fail Closed placeholder；它不是production实现。production recovery仍因Runtime
Attempt→Admission真实Reader及composition root缺失而NO-GO。

## 顺序

正常入口：

1. 冻结request与Runtime authorization指针；
2. fresh S1读Sandbox Command current并逐字段绑定payload/source/authorization；
3. exact S2重读、fresh clock、ValidateCurrent；
4. actual entry再次采样fresh clock，以已封存S2 Command、request和authorization重验current；
   clock回拨、任一窗口`now >= expiry`或`ctx.Err()!=nil`均零Execute；
5. 调用Sandbox V2 physical execution；返回Admission畸形、non-admitted、no-effect或StableKey漂移时，
   不使用该返回值，直接从原Runtime Attempt进入inspect-only recovery；
6. 用可信Admission读Sandbox Admission→Attempt binding；
7. `InspectBoundedWorkspaceReadV2(binding.Attempt)`；
8. 强制`Envelope.RequestedOriginAttemptRef == binding.Attempt`全Ref相等；
9. 只消费`Envelope.CurrentProjection`，校验Reservation/Observation/File closure后物化输出。

恢复入口：

1. 不调用Execute；使用独立`context.WithTimeout(context.WithoutCancel(ctx), 5..30s)`；
2. 原Runtime Attempt→Tool窄Reader→Admission；
3. Admission→Sandbox binding；
4. 以`binding.Command`读取Sandbox历史exact Command，绑定请求`SourceCommand`及完整payload/workspace/range；
5. 历史Command证明通过后才读取V2 terminal Envelope；
6. V2 Envelope fresh资格独立于已过期Tool `RequestedNotAfter`与Runtime `UnifiedNotAfter`；
7. Unknown/Unavailable只返回Inspect结果错误，不重发physical read。

## 硬反例

- origin Attempt同ID换revision/digest；
- Runtime Attempt任一exact字段漂移；
- Admission splice、binding splice；
- recovery请求替换`SourceCommand`，即使重Seal且语义payload相同也必须Conflict、零Execute、零Tool输出；
- 历史Command合法重Seal但仅替换`DispatchDigest`或`TenantID`必须Conflict；
  `historical=1, Inspect=0, Execute=0, physical-read=0`；
- Reservation TTL Closure级联重Seal但替换Unified、越过已知Enforcement上界、
  Command RequestedNotAfter、Command expiry或Effective/Binding关系必须Conflict；
- Reservation RequestDigest、AttemptID或Admission Checked/Expires与Binding关系漂移必须Conflict；
- Reservation、Binding、authorization StableKey必须三方exact；
- Command携exact ExpectedFileRef而自洽Envelope返回同ID/revision不同digest File时必须零Tool输出；
- Current/Historical Command Reader返回后必须Owner-local复制`ExpectedFileRef`，之后调用方同步或并发
  修改原指针都不得改变已封存证明或产生data race；
- S2后ctx取消、actual-entry clock回拨或精确越过任一expiry均零Execute；
- Execute返回zero、畸形、non-admitted、no-effect或错StableKey Admission必须只Inspect原Runtime
  Attempt，不得把返回值送入binding lookup或触发第二次Execute；
- Runtime Attempt、Admission binding、historical Command、terminal Attempt任一级NotFound均只停止在该读层级；
- 历史Command Reader unavailable、typed-nil或返回legacy未Seal事实必须Fail Closed；
  legacy未Seal按Conflict分类，正常Start不得回退该Reader；
- terminal Envelope过期或ProjectionDigest漂移；
- caller取消、上游TTL过期后恢复仍只Inspect；
- 64并发恢复必须`Execute=0, physical-read=0`；
- Adapter不得import `os`、`io/fs`、Sandbox实现包或使用`os.ReadFile`；
- V1 Port包装成V2、直接消费裸Projection、按ID判断Attempt均禁止。
