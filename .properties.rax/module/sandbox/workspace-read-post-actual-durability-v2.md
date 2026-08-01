# Workspace Read Post-Actual Durability V2 Module

## 当前状态

状态：`implementation-complete / local-full-gates-green / independent-p0-p1-green / owner-local / uncommitted`。

本模块已经落地 additive V20 的公共历史合同与Sandbox-internal写能力：

- `WorkspaceReadExecutionQualificationV2`：actual point前的create-once S1资格历史；
- `WorkspaceReadPhysicalJournalLookupV2`：从Qualification确定性派生、无current/authority的
  restart exact lookup；
- `WorkspaceReadPhysicalJournalRefV2`：started revision 1或completed revision 2的neutral
  execute journal坐标；
- `WorkspaceReadTerminalFactV2`：每个origin Attempt一个`observed|indeterminate`历史终态；
- public Terminal exact/origin readers；
- `internal/owner/workspaceread` Qualification nominal write capability；
- Kernel私有Unix IPC actual-point桥与私有journal evidence/Terminal nominal capability；
- SQLite V20 append-only Qualification/Terminal repository与Rust journal restart Inspect。

当前实现已完成，全Sandbox ordinary/race/vet、Rust test/fmt/clippy、diff-check与
8-handle natural-clock高重复门禁均通过；独立终审结论为P0=0、P1=0、无实质P2。
publish-ready之前只剩最新main rebase后复验与PR/CI门禁。

## 关键闭包

- Runtime Attempt与Sandbox origin Attempt不是同一ID；两者通过existing
  AdmissionAttemptBinding digest显式关联。
- Admission V1/Runtime receipt逐ID/revision/digest/stable key闭合。
- Qualification封存exact Association、Workspace Lease digest、sealed current query digest、
  expected Runtime current digest、actual request/payload digest，但不伪称已获得Rust S2 Projection。
- Journal lookup只含RuntimeAttemptID/request/payload/execute，不携过期current。
- Observed必须Completed + exact Provider Observation/Receipt + 可重算S2 proof；其稳定closure逐轴
  等于Qualification S1。
- Completed后/S2前崩溃只能Indeterminate；Indeterminate类型没有content字段。
- Indeterminate stage由journal state与error class确定性推导；public caller不能自签Owner能力。
- Qualification到期后仍可exact Inspect，但不能恢复Dispatch authority。
- public ports没有裸Fact writer；其他Owner不能导入Sandbox internal capability。
- public `dataplaneadapter.Client.Dispatch`拒绝`workspace.read`；生产构造器只接收Client并在
  Kernel内部构造不可替换的actual point，fake注入仅存在于`_test.go`。

## 文件入口

- `ExecutionRuntime/sandbox/contract/workspace_read_post_actual_v2.go`
- `ExecutionRuntime/sandbox/ports/workspace_read_post_actual_v2.go`
- `ExecutionRuntime/sandbox/internal/owner/workspaceread/post_actual_v2.go`
- `ExecutionRuntime/sandbox/kernel/workspace_read_actual_point_v2.go`
- `ExecutionRuntime/sandbox/kernel/workspace_read_terminal_authority_v2.go`
- `ExecutionRuntime/sandbox/storage/sqlite/workspace_read_post_actual_v20.go`
- `ExecutionRuntime/sandbox/contract/workspace_read_post_actual_v2_test.go`

## 已验证

- focused contract/kernel/SQLite ordinary/race（含S1/S2逐轴splice、deterministic stage、
  public Client raw dispatch拒绝与8-handle natural-clock单winner）；
- 8-handle自然时钟 ordinary×20与race×5；
- full Sandbox `go test -count=1 ./...`与`go test -race -count=1 ./...`；
- `go vet ./...`；
- Rust `cargo test --all-targets`、`cargo fmt --check`、
  `cargo clippy --all-targets -- -D warnings`；
- `git diff --check`。

## NO-GO

本模块不生成ToolResult，不刷新Context，不触发下一Model Turn，
不改变V1/V2 wire/digest，不宣称production。
