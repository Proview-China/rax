# workspace.read → Sandbox Adapter V1模块说明

## 状态

`owner_local_reference_reaudit_pending`

本切片把CorePack严格`workspace.read`请求接到Sandbox public
`WorkspaceReadExecutionPortV2`。它复用Sandbox Owner的Command、Reservation、Attempt、
Observation和V2 Inspection Envelope，不复制Sandbox DTO、不直接读取文件，也不生成Runtime
Settlement或ToolResult。CorePack继续`Executable=false`。

## 组成

- `corepack/workspace_sandbox_adapter_v1.go`：正常Start-or-Inspect及独立bounded inspect-only恢复；
- `contract/workspace_read_runtime_admission_v1.go`：Tool-neutral Runtime
  Attempt→Admission inspection projection；
- `internal/testkit/workspace_read_sandbox_adapter_v1.go`：owner-local fixture；
- `tests/blackbox|fault|conformance/workspace_read_sandbox_adapter_v1_test.go`：边界、故障与导入门。

## 已闭合

- request、Runtime authorization及Command `ExpectedFileRef`入口封存；
- Sandbox Command current S1/S2后actual-entry fresh clock、current和ctx门；
- zero、畸形、non-admitted、no-effect或错StableKey Admission只从原Runtime Attempt恢复；
- Runtime Attempt→Admission→Sandbox binding→historical Command→V2 terminal Envelope exact链；
- Reservation TTL可证明轴、StableKey、RequestDigest、AttemptID和Admission/Binding时间关系；
- Observation Reservation、Attempt、WorkspaceView、File ID/revision及可选ExpectedFile exact关系；
- Unknown/lost reply只Inspect原Attempt，恢复不续执行权；
- 大于256KiB inline结果Fail Closed，等待Tool Artifact能力。

最终返修后的focused blackbox/fault ordinary×100及race×20通过；该结果等待独立复审，
不得写成production ready。

## 仍为NO-GO

- Runtime/宿主真实Attempt→Admission production Reader与composition root；
- Association、WorkspaceView、WorkspaceLease expiry的Tool独立current证明；
- 大结果Artifact writer/reader与包含WorkspaceView exact identity的正式Tool输出；
- production backend、SLA、Provider能力启用及Context/Harness接线。

Runtime Admission的`UnsupportedWorkspaceReadRuntimeAdmissionCurrentReaderV1`仅为零调用
Fail Closed placeholder。无Tool重复Sandbox Command Reader或对应Unsupported包装。

设计入口：[workspace-read-sandbox-adapter-v1.md](../../design/tool-engine/workspace-read-sandbox-adapter-v1.md)

实施计划：[workspace-read-sandbox-adapter-v1.md](../../plan/tool-mcp/workspace-read-sandbox-adapter-v1.md)
