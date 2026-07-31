# Workspace Read Execution Command V1实施计划

## 文件

- `contract/workspace_read_execution_command_v1.go`
- `ports/workspace_read_execution_command_v1.go`
- `internal/owner/workspacereadcommand/producer_v1.go`
- `internal/owner/workspacereadcommandrepo/repository_v1.go`
- `storage/sqlite/workspace_read_execution_command_v1.go`
- focused contract、Owner与SQLite tests
- nominal corepack handoff在上述门禁闭合并rebase正式Sandbox V2后另行接线

## 顺序

- [x] 冻结Request/Claim/State/Binding/Candidate与Runtime Attempt唯一因果边。
- [x] 实现nominal Ref、immutable Fact和full Attempt canonical digest。
- [x] 实现Tool-internal Owner producer、public exact/reverse/current readers。
- [x] 实现Tool-owned S1/S2 stable-semantic producer，要求start_committed且外部effect=0。
- [x] 实现独立SQLite ledger、create-once、三条外部因果轴UNIQUE及历史Inspect。
- [x] 完成contract、Owner、SQLite真实时钟、restart/lost-reply和64并发门禁。
- [ ] 实现additive typed `WorkspaceReadSandboxRequestV2`/adapter：Start读取Command exact/current并做
  current S1/S2/S3，recovery只读exact历史；封堵generic V1 production旁路。
- [x] 补跨Request/Action/Binding/Attempt splice、Permit/Delegation逐轴splice。
- [x] 补64并发single winner与same Attempt/different command Conflict。
- [x] 跑focused ordinary×100、race×20、full ordinary/race、vet、diff与import门禁。
- [x] 以ready PR #78发布owner-local Command切片；PR尚未合并。

## 边界

- 所有改动保持owner-local/reference durability；
- 不改既有Runtime或Sandbox事实；
- 不落Payload Source/Material或Classification；
- 不创建production Host/Provider；
- Binding/Input只有InMemory固定winner，restart后的fresh Current不得声称可用；
- owner-local Command已全门禁并发布ready PR #78；additive typed Tool→Sandbox V2 handoff、
  Payload Source/Material与Classification仍pending/NO-GO。
