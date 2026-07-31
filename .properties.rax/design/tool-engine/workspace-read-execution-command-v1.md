# Workspace Read Execution Command V1 Design Delta

## 状态

- `owner-local/reference-durability-ready-pr-78`
- 唯一Owner为`ExecutionRuntime/tool-mcp`。
- Contract、Owner Producer、SQLite create-once/exact/reverse/current及其真实时钟、
  restart/lost-reply、64并发、splice和ordinary/race/vet/diff/import门禁均已通过，并以
  ready PR #78发布；PR尚未合并。
- 本切片只在真实effect前建立Tool Request与Runtime Attempt的不可变因果边，不执行
  Provider、Sandbox或Tool effect。
- BindingCurrentV2与ToolInputContractCurrentV1当前只有进程内create-once Store；命令历史可
  持久恢复，但重启后无法fresh复读这些上游时必须Unavailable，不能声明跨源production
  durability。

## 问题

现有两条权威链彼此独立：

```text
RequestKey → Claim → ExecutionState → BindingV2 → CandidateV3

Runtime Attempt → WorkspaceReadCommand → Admission → Sandbox Attempt
```

`WorkspaceReadCommand.SourceToolCommand`当前只是generic Tool Ref。两个Tool、payload和scope
完全相同但Request/Action不同的并发调用，可以把另一条Runtime Attempt拼入Sandbox Command。
payload digest、AttemptID字符串或Tool ID都不能证明这两条链属于同一次执行。

## 事实

`WorkspaceReadExecutionCommandV1`是Tool-owned、revision=1、create-once的nominal事实，必须绑定：

- `SingleCallToolActionInspectKeyV2`与Claim exact Ref；
- exact `start_committed` ExecutionState snapshot、ExecutionInputDigest和Tool-local
  ExecutionAttemptID；
- BindingCurrentRefV2、Candidate exact Ref、InputContract Current及Candidate Closure Digest；
- Tool与Tool Descriptor Current exact Ref；
- Runtime OperationSubjectV3及OperationDigest；
- PreparedProviderAttemptRefV2及完整OperationDispatchAttemptRefV3；
- payload Schema/Digest/Revision；
- Created、NotAfter及immutable Ref/Digest。

Tool execution attempt与Runtime dispatch attempt是两个不同Owner的nominal坐标。V1不以底层
string恰好相等或不等推导语义。

## 创建

Tool internal Owner producer的create request只允许：

```text
RequestKey
Expected Binding Ref
Expected Action Ref
Expected OperationSubject
Expected Prepared Ref
Expected full Runtime Attempt
RequestedNotAfter
```

Producer自行读取：

- Tool Owner Claim Store V2；
- Tool Owner ExecutionState Store V2；
- BindingV2 exact/current reader；
- Runtime Effect Current；
- Runtime Prepared Current。

Runtime Effect必须以Target、Review Candidate Digest/Revision、payload、scope、owner/provider、
idempotency及Operation exact绑定Candidate。Prepared Current必须返回完整Attempt，逐字段等于
expected Attempt。S1/S2分别在依赖读取完成后取Owner时钟并验证current；两轮只比较稳定语义：

- Claim、ExecutionState、Binding与Input create-once winner；
- Candidate、Descriptor body、Tool Registry stable Source/Object；
- Runtime Effect Intent/IntentDigest/FactRevision/`dispatch_intent`；
- Runtime Prepared semantic snapshot及exact Ref。

Registry、Runtime Effect与Runtime Prepared的fresh Checked/Expires/ProjectionDigest允许读时重签，
不得进入历史Fact identity；它们只进入Command Current。Binding/Input是固定winner，可保留
exact Ref、稳定body以及winner固定Checked/Expires进入历史闭包，但Current Reader仍需fresh复验。

创建时机固定为：

```text
fresh Claim/Binding/Candidate/start_committed
+ fresh Runtime Effect/Prepared/Attempt
→ create WorkspaceReadExecutionCommand once
→ seal Sandbox WorkspaceReadCommand
→ Runtime association/authorization
→ physical effect
```

创建命令时外部effect计数必须为0。

## 持久化与恢复

SQLite使用独立schema ledger和单一immutable表，不修改既有Tool Owner Claim/Execution schema。
command ID为主键；以下三条外部因果轴分别UNIQUE，四轴必须映射同一命令：

- RequestKey canonical digest；
- Tool-local ExecutionAttemptID；
- full Runtime Attempt canonical digest。

Runtime Attempt digest覆盖完整Permit及Delegation；禁止只使用AttemptID或Go指针相等。

并发候选的`CreatedUnixNano`取各自Tool Owner真实commit时钟，Fact digest覆盖Created，但command
ID与stable closure comparison排除Created和Ref Digest；SQLite只选择一个winner。其他任一稳定
语义漂移都Conflict。

`NotAfter`只取request/state/binding/candidate/input、Effect Intent、Prepared Fact等稳定absolute
上界；fresh Registry/Effect/Prepared TTL只限制Command Current，不改写历史Fact。

create回包不确定时只按预派生exact Ref Inspect。restart后exact Ref及full Attempt reverse lookup
必须返回同一canonical body。相同Attempt对应不同命令一律Conflict。

Claim、ExecutionState、Binding和Candidate完整body只在Owner-private S1/S2 snapshot中读取，
不复制进命令事实。命令保存它们的nominal exact refs、source digest及必要的denormalized
坐标，SQLite读取时逐列与canonical body交叉校验。

## Sandbox handoff

PR #78只闭合owner-local Command，不包含该接线。后续additive typed
`WorkspaceReadSandboxRequestV2`/adapter必须先读取Command exact/current；Start执行前做current
S1/S2/S3，recovery只读exact历史，再唯一映射为Sandbox wire中的opaque raw-lowercase Ref。
legacy `WorkspaceReadSandboxRequestV1`保持reference/test-only，不得进入production composition。
Sandbox Command必须逐字段满足：

```text
SourceToolCommand == exact Tool command Ref
SourceToolPayloadSchema == command.PayloadSchema
SourceToolPayloadDigest == command.PayloadDigest
SourceToolPayloadRevision == command.PayloadRevision
```

该转换只生成request envelope，不创建Sandbox/Runtime事实，也不执行effect。

## NO-GO

- 不写Classification、Payload Source/Material、Context、Application、Runtime或Sandbox事实；
- 不新增Provider、FFI或production composition；
- 不把本切片称为完整workspace.read生产链；
- raw create writer保持Go `internal`；公开面只提供typed exact、full RuntimeAttempt reverse和
  current reader；
- Runtime Effect/Prepared Current Reader当前没有非测试实现，production composition仍NO-GO；
- D0 Payload Source仍等待Sandbox V2 terminal handoff与原始canonical output capture。
