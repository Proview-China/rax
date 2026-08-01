# Workspace Read Command Publication V2 Plan

状态：`implementation-software-test-yes / owner-local / uncommitted`。

## 阶段与文件落点

### P1：合同与消费Port

- `ExecutionRuntime/sandbox/contract/workspace_read_command_publication_v2.go`
  - public nominal Source Ref/Projection、Publication、OwnerCurrent；
  - stable semantic与transient current TTL分层；
  - canonical seal、shape/current、clone/no-alias。
- `ExecutionRuntime/sandbox/ports/workspace_read_command_publication_v2.go`
  - 只放窄Source Reader、Publication/OwnerCurrent Reader与coordinate-only
    Ensure interfaces。
- `ExecutionRuntime/sandbox/{contract,ports}/**/*test.go`
  - canonical inline bytes、ref payload、oversize、half-open TTL、typed-nil；
  - Delegation greater/equal/lower/ID/digest；
  - Workspace full-body、scope/path与ObservedRevision S1/S2漂移；
  - Command/Publication同一initial commitNow、stable refs与三事实/fresh join；
  - initial/next OwnerCurrent、derived TTL与expired predecessor refresh。

验收：Sandbox对`tool-mcp` import为零；public面无raw writer或stable lookup。

### P2：kernel S1/S2与owner-private capability

- `ExecutionRuntime/sandbox/internal/owner/workspacereadcommand/**`
  - 只允许Sandbox内部构造的authorized publication envelope；
  - clone/error传播与import-boundary。
- `ExecutionRuntime/sandbox/kernel/workspace_read_command_publication_v2.go`
  - Source、Runtime Effect/Prepared、Workspace S1/S2；
  - 从Prepared snapshot绑定full Runtime Attempt；
  - fresh-clock与TTL最小上界；
  - deterministic refs；
  - commit Unknown只bounded exact Inspect。
  - public current资格固定执行stored
    `ValidateWorkspaceReadCommandOwnerClosureV2`，再执行fresh
    `ValidateWorkspaceReadCommandOwnerFreshClosureV2`。
- `ExecutionRuntime/sandbox/ports/workspace_read_v1.go`
  - 从public `WorkspaceReadOwnerStoreV1`移除raw Command create；
  - 保留physical Attempt现有mutation合同。
- `ExecutionRuntime/sandbox/storage/sqlite/workspace_read_v1.go`
  - 删除或私有化concrete `(*Store).CreateWorkspaceReadCommandV1`；
  - legacy `InspectWorkspaceReadCommandCurrentV1`只有在V2
    Publication+OwnerCurrent current闭包存在时才可返回；v18/raw-only
    Command必须Fail Closed。

验收：缺任一current/轴漂移/typed-nil/clock rollback均零writer、零physical、
零Provider、零Runtime write。

kernel Workspace依赖只包含`InspectWorkspaceViewCurrentV1`，不注入broad
`WorkspaceCurrentReaderV1`。

### P3：SQLite v19

- `ExecutionRuntime/sandbox/storage/sqlite/store.go`
  - schemaVersion 19；
  - `BEGIN IMMEDIATE`内重新读取version并枚举namespace；
  - existing v19零DDL strict verify；旧版只有namespace全空才exact CREATE；
  - physical shape/probe通过后ledger+user_version同事务推进；
  - v18 legacy零回填、partial namespace Fail Closed。
- `ExecutionRuntime/sandbox/storage/sqlite/workspace_read_command_publication_v2.go`
  - Command body+seal、Publication、OwnerCurrent同事务create-once；
  - current history/pointer CAS；
  - exact historical/current Inspect；
  - commit Unknown三路closure。

Ensure事实事务禁止执行DDL、推进`user_version`或写schema ledger。
- 禁止把v19 DDL直接追加到通用`schemaStatements`后用
  `CREATE IF NOT EXISTS`修复缺对象。
- raw SQLite只暴露stored primitive；public current Reader在kernel完成三事实join
  与fresh四Reader join。
- `ExecutionRuntime/sandbox/storage/sqlite/**/*test.go`
  - missing/extra/partial/shape/DDL/index/ledger/tamper/restart/8 handles。

验收：无silent repair；V18 historical exact可读，但V1/V2 current、Ensure及
physical资格全部为零。

### P4：矩阵与全门

- unit：seal/validate/clone、canonical bytes、TTL equality/rollback；
- whitebox：四Reader S1/S2逐轴drift、owner capability、三路partial；
- blackbox：public Ensure→SQLite winner→OwnerCurrent exact Inspect；
- fault：lost reply、restart、reader unavailable、commit Unknown；
- concurrency：64同Source单winner、同Attempt异Source Conflict、8 handles；
- compatibility：legacy v18、V1 historical Reader、zero backfill；
- import/scope：Sandbox零`tool-mcp`，Runtime/Tool/Harness/Application零修改。

实际门禁（2026-08-01 live worktree）：

```text
focused ordinary / concurrency / fault PASS
focused race PASS
go test ./... PASS
go test -race ./... PASS
go vet ./... PASS
go mod verify PASS
gofmt check PASS
git diff --check PASS
import/scope/no-bypass scan PASS
```

## 依赖与阻塞

- 复用Runtime public Effect/Prepared current合同；Runtime Attempt只取Prepared
  snapshot，不新增Reader。
- neutral Source exact coordinate封`settlement` Owner、固定Kind、
  ID/revision/digest；Owner须在Effect.Intent.Owners exact集合中，并与
  Effect/Prepared Provider逐轴闭合；
  Source Ref digest只接受raw lowercase 64 hex，payload digest只接受
  `sha256:` lowercase。
- Publication只存stable semantic closure、absolute NotAfter和create-time
  watermarks；transient outer current proof只存OwnerCurrent。
- stable watermarks只计算`EffectiveCreatedLower`；实际Command/Publication
  Created取owner commitNow并从stable ID/digest/equality排除，winner body保留
  真实commit time。
- 初次三路原子；refresh只CAS同一Publication的新OwnerCurrent，且expiry不得越过
  Command/Publication语义NotAfter或fresh current上界。
- initial固定rev1且Command/Publication/OwnerCurrent共享commitNow；next固定
  predecessor revision+1、Created沿用、Checked单调，expiry由合同精确派生。
- expired predecessor允许在fresh S1/S2后CAS刷新，但stale/non-pointer
  predecessor必须Conflict。
- 复用Sandbox窄Workspace View current Inspect方法。
- 本PR定义neutral Source消费Port；Tool adapter后续单独实现。
- production Runtime current reader、Tool adapter、Application composition、
  physical Provider/backend仍NO-GO，不属于本计划。

## 交付结论门

全部测试已通过，独立P0/P1审计在current-truth同步后清零，因此当前状态为
`implementation-software-test-yes / owner-local`。仍保持uncommitted，不stage、
不发布；这不代表workspace.read cross-owner production seam或CorePack
Executable已经闭合。
