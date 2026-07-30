# Workspace Read Inspection Envelope V2 Plan

## 文件落点

- `ExecutionRuntime/sandbox/ports/workspace_read_inspection_v2.go`
  - additive Envelope、Seal/Validate/strict Decode、Reader/Execution Port V2；
- `ExecutionRuntime/sandbox/storage/sqlite/workspace_read_inspection_v2.go`
  - origin history 到 latest current 的单事务 exact closure；
- `ExecutionRuntime/sandbox/kernel/workspace_read_v1.go`
  - V2 Inspect 只读转发，V1不变；
- `ExecutionRuntime/sandbox/{storage/sqlite,kernel}/*inspection_v2_test.go`
  - owner closure、并发、故障与零 actual-point 反例。

## 验收

- origin rev1 到 observed/failed/indeterminate rev2；
- same ID 换 origin revision/digest拒绝；
- Reservation、Command、Admission、current 任一 splice拒绝；
- Observed Observation的Reservation/Command/WorkspaceView/File/path/bytes/digest/
  receipt任一自洽重Seal splice拒绝；
- terminal早于origin、Provider receipt早于S1/origin等因果时间倒退拒绝；
- SQLite Observation `observation_id`或`stable_digest`单列splice在restart后拒绝；
- restart/lost reply只Inspect original；
- 过期 historical terminal可Inspect但不可恢复执行；
- Envelope digest、strict nested JSON、TTL上界、`now==expiry`、clock rollback、
  typed-nil均Fail Closed；
- SQLite读取前后分别读取initial/fresh owner clock；started在fresh到达任一
  execution TTL边界、fresh回退或归零时返回零Envelope；
- terminal historical inspection在execution TTL后仍可读取，但Envelope只从
  fresh产生独立短TTL，不能恢复执行资格；
- 64并发 V2 Inspect不写Owner state且物理读取计数为零；
- V1 Inspect保持兼容。

验证命令：

- focused ordinary `-count=100`；
- focused race `-count=20`；
- `go test ./...`；
- `go test -race ./...`；
- `go vet ./...`；
- Rust `cargo test --all-targets`、`cargo clippy --all-targets -- -D warnings`、
  `cargo fmt --check`；
- gofmt、diff-check与import/scope扫描。

完成条件只代表 Sandbox owner-local V2 inspection closure ready。Tool full
composition和其他跨 Owner product root继续 NO-GO。
