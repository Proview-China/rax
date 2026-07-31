# workspace.read → Sandbox Adapter V1实施计划

## 当前状态

- Sandbox #56 V2 exact inspection envelope以stacked dependency参与编译。
- Tool owner-local实现正在独立审计，未提交。
- CorePack `Executable=false`；Sandbox public historical exact Command Reader已落盘并被Tool
  直接消费，但production root、Runtime Attempt→Admission真实Reader、
  大于256KiB Artifact物化及WorkspaceView exact输出仍NO-GO。

## 文件落点

- `ExecutionRuntime/tool-mcp/corepack/workspace_sandbox_adapter_v1.go`
- `ExecutionRuntime/tool-mcp/internal/testkit/workspace_read_sandbox_adapter_v1.go`
- `ExecutionRuntime/tool-mcp/tests/blackbox/workspace_read_sandbox_adapter_v1_test.go`
- `ExecutionRuntime/tool-mcp/tests/fault/workspace_read_sandbox_adapter_v1_test.go`
- `ExecutionRuntime/tool-mcp/tests/conformance/workspace_read_sandbox_adapter_v1_test.go`

## 验收

- [x] Constructor只接受`WorkspaceReadExecutionPortV2`，typed-nil/5..30秒恢复窗门禁。
- [x] `RequestedOriginAttemptRef == binding.Attempt`全Ref exact。
- [x] Adapter只消费`Envelope.CurrentProjection`。
- [x] 正常入口physical effect前fresh S1/S2及actual-entry第三时钟重验request、Command与
  authorization current；S2后ctx取消、clock回拨、`now==expiry`均零Execute。
- [x] Execute返回zero/畸形/non-admitted/no-effect/错StableKey Admission统一转原Runtime
  Attempt inspect-only recovery，不把不可信receipt送入binding lookup。
- [x] bounded inspect-only recovery不受caller cancel或上游执行TTL阻断。
- [x] recovery先以`binding.Command`消费Sandbox历史exact Command，逐字段绑定
  `SourceCommand`、payload、workspace/range与Runtime authorization；其中Dispatch Attempt
  canonical digest与TenantID必须独立exact，TTL过期只读shape、不续权。
- [x] Reservation TTL只闭合可证明关系：Unified exact、Runtime Enforcement不超过已知
  authorization上界、Command requested/meta expiry exact、Effective=Reservation=Binding；
  Reservation RequestDigest=Command digest、AttemptID=Binding Attempt，以及Admission
  Checked/Expires=Binding Created/Expires；
  Association/View/Lease expiry不脑补，继续作为reference production NO-GO。
- [x] 已验证Command传入Tool输出物化；非nil`ExpectedFileRef`与Observation File完整SameRef。
- [x] SourceCommand重Seal splice、历史Reader unavailable/typed-nil、legacy未Seal事实均
  零Execute/零Tool输出；未Seal按Conflict分类，正常Start只走Command Current S1/S2且历史Reader调用数为0。
- [x] 合法重SealCommand仅漂移`DispatchDigest`或`TenantID`均
  `Conflict, historical=1, Inspect=0, Execute=0, physical-read=0`。
- [x] TTL/Reservation级联重Seal十组反例（含StableKey三方exact）、ExpectedFile digest自洽
  Envelope反例、四级NotFound及
  same-ref未Seal Command body均Fail Closed且零Execute/physical read。
- [x] Command Current/Historical Reader结果对唯一指针`ExpectedFileRef`做Owner-local deep clone；
  同步alias mutation和并发mutator race反例通过。
- [x] request schema直接调用`runtimeports.SchemaRefV2.Validate()`；uppercase namespace、
  非canonical SemVer/media type在Seal阶段Fail Closed。
- [x] lost reply/restart、wrong origin、Admission splice、Envelope expiry反例。
- [x] 64并发recovery零Execute/零physical read。
- [x] real Rust门只证明相邻Sandbox actual-point，不冒充Tool production composition。
- [x] Tool构造器直接消费`sandboxports.WorkspaceReadCommandExactReaderV1`；无Tool重复
  Sandbox Command Reader、对应Unsupported包装或Sandbox DTO复制。Runtime Admission的
  零调用Unsupported placeholder单独保留且不冒充production实现。
- [ ] Runtime/宿主实现真实Attempt→Admission exact Reader。
- [ ] production Artifact writer、WorkspaceView exact output与composition root。
