# Workspace Read Post-Actual Durability V2

## 状态与目的

状态：`design-frozen / additive-v20 / owner-local / implementation-active`。

本设计解决唯一问题：`workspace.read` 已越过 Rust `openat2/pread` 实际点后，
上游 current/TTL 到期、S2 失败、进程崩溃或 SQLite 回包丢失时，Sandbox 必须仍能
持久证明 `observed` 或 `indeterminate`，且永不以新 Attempt 重读文件。

V1/V2 的 wire、digest、current 与 execution authority 全部保持不变。V20 新事实只
追加历史证据，不延长 TTL、不恢复执行资格、不把 Unknown 映射成失败或成功。

## Owner 与边界

- 唯一写 Owner：`ExecutionRuntime/sandbox`。
- Runtime 继续拥有 Operation/Permit/Authorization/Admission；V20 只封存 exact refs。
- Rust Data Plane 继续拥有物理 journal；Go V20 只消费 Sandbox-neutral exact journal ref。
- `workspace.read` 的不可逆 IPC dispatch 只存在于 Kernel 私有桥；公开
  `dataplaneadapter.Client.Dispatch` 对该Provider/Effect永久Fail Closed，公开生产构造器也不接收
  caller-supplied ActualPoint实现。
- Tool 只读 Sandbox terminal，不写 Qualification/Terminal，不把 Observation 直接升级成
  ToolResult。
- Context、Application、Harness、Host、Console 均不在本切片。

## 两类新增历史事实

### WorkspaceReadExecutionQualificationV2

这是 physical actual point 之前 create-once 的资格历史，不是 current authority。它唯一
绑定：

- exact origin `WorkspaceReadAttemptRefV1`；
- exact Sandbox Reservation；
- exact Runtime Admission receipt 与 full Runtime Attempt；
- existing `WorkspaceReadAdmissionAttemptBindingV2` digest，作为Runtime Attempt与Sandbox
  origin Attempt的显式因果桥；两者ID不要求相等；
- Authorization digest；
- exact Prepared Domain Command Association；
- exact Command、Command Publication、Command OwnerCurrent；
- exact WorkspaceView与其Lease digest；
- sealed `WorkspaceReadCurrentQueryV2` digest、Go侧预期 Runtime current digest与
  actual request digest；
- S1 checked 与自然最小 expiry。

Qualification在调用Data Plane actual point之前创建，只能封存Go已经验证完成的S1
closure、sealed query与expected Runtime current。它不声称已经取得Rust current-server的
S2 Projection；未来若要封存该Projection，必须先建立真实reader并在资格写入前读取，
不得由caller预填或从query推导。

Qualification ID 只由 origin Attempt 派生；同一 Attempt 的异 query、异current或异
authorization只能Conflict，不能产生兄弟资格。Qualification expiry只表示当时允许
跨越实际点的上界；其历史事实在到期后仍可exact Inspect，但不得再次用于Dispatch。

### WorkspaceReadTerminalFactV2

这是 actual point 之后 append-only、每个 origin Attempt 至多一个的历史终态。Outcome
闭集仅：

- `observed`：必须携完整可重算的`WorkspaceReadOutcomeS2ProofV2`，逐字段绑定
  Qualification/origin/Association/Publication/OwnerCurrent/Workspace/Lease/Runtime current、
  exact journal、Observation、ProviderReceipt与S2 checked；
- `indeterminate`：只携确定性`WorkspaceReadIndeterminateEvidenceV2`，其stage由journal state
  与error class推导，不允许caller选择；不携content、artifact、
  bytes或可重放payload。

Terminal固定绑定：

- exact Qualification ref 与 origin Attempt；
- full Runtime Attempt 与 actual request digest；
- Sandbox-neutral `WorkspaceReadPhysicalJournalRefV2`；
- outcome checked、recorded、Qualification expiry及fact digest。

Terminal ID只由origin Attempt派生。Observed与Indeterminate竞争同一唯一坐标；同body
幂等，异Outcome或异证据Conflict。Terminal是历史事实，不是current、Permit、
Authorization或调用权。

## Physical journal exact ref

`WorkspaceReadPhysicalJournalRefV2`不得导入Rust DTO，至少封存：

- `AttemptID`；
- `RequestDigest`；
- `PayloadDigest`；
- `Phase`；
- `State`；
- `Revision`；
- `RecordedUnixNano`；
- `RecordDigest`。

Observed只接受Completed journal并且Go S2已经成功、exact Observation与完整S2 proof均已
形成的证据；S2 proof的稳定坐标必须逐字段等于Qualification的S1 closure，仅有Completed
journal不能推出Observed。Indeterminate接受已越actual point但
S2未完成、无法证明Observation闭合或journal只到started/unknown的证据。journal
request/payload/attempt必须逐字段等于
Qualification/Runtime Attempt/actual request；不能只比较stable key。

## TTL 与 Inspect

1. Data Plane current adapter在`openat2/pread`前完成其内部V2 current/TTL S1/S2；
   `now == expires`时physical=0。本文后续所称kernel outcome S2发生在actual point之后，
   两者不是同一检查。
2. 一旦actual point已越过，terminal create不再要求Qualification、Attempt、Reservation、
   OwnerCurrent保持current；只验证其immutable exact形状与历史因果。
3. Observed的`OutcomeChecked`可晚于Qualification expiry；不能篡改V1时间或重Seal旧Attempt。
4. Rust/Go内部IPC必须分离：
   - `dispatch`继续`ValidateCurrent`；
   - `inspect`只做shape-only + exact Attempt/Request/Payload digest校验，允许读取TTL已过期的
     historical journal/evidence/result，但绝不恢复执行authority。
5. Unknown只Inspect原Attempt与原journal，永不换key、换Attempt或重读文件。

## 持久化与恢复

SQLite schema V20新增两张append-only Owner表：

- qualification history：origin Attempt与full Runtime Attempt/query digest唯一；
- terminal history：origin Attempt与Qualification唯一。

Terminal可以对Qualification exact Ref建立外键；Qualification不得反向外键到旧V1
current/legacy表，只保存其canonical exact refs/digests，避免append-only历史被旧可变namespace
绑死。不新增current pointer。create commit回包不确定时只Inspect预封exact Ref；旧v19事实
绝不回填V20 Qualification或Terminal。旧expired started只能由旧Inspection兼容报告
indeterminate，且绝不能据此恢复执行。Rust journal Completed后、Go S2前崩溃仍只能物化
Indeterminate且不得带content；只有S2已成功、exact Observation与S2 proof已形成，但
Terminal commit回包丢失或跨TTL时，才可在重启后Inspect/收敛为Observed。physical read
计数始终保持1。

## 不变量与硬门

- V1/V2 JSON、digest和public execution port零修改。
- public ports只暴露Terminal exact/origin readers；Qualification writer接收Sandbox
  `internal/owner/workspaceread` nominal capability；Terminal writer只接收Kernel私有journal
  evidence派生且私有字段封存的nominal capability，禁止裸Fact public writer。
- caller-sealed public S2 proof或Indeterminate evidence不能直接获得Owner写能力；journal
  evidence只能由Kernel私有Unix IPC桥在严格验证Rust响应后生成。
- Qualification create发生在actual point前；Terminal create发生在actual point后。
- pre-actual current/TTL漂移：physical=0、terminal=0。
- post-actual TTL crossing：必须产生Observed或Indeterminate历史事实，不得丢成普通Conflict。
- Unknown事实中不得出现content、file bytes、raw response或secret。
- 64并发同origin只形成一个Qualification和一个Terminal winner。
- lost reply/restart只Inspect exact winner；SQLite与Rust均不重复物理读取。
- Runtime/Tool/Application/Context事实写入计数为0。

## 明确NO-GO

本切片不生成ToolResult、Context Frame、下一Model Turn或Console合同；不宣称
cross-owner/production闭合。它只关闭Sandbox actual-point后结果不可持久化的P0。
