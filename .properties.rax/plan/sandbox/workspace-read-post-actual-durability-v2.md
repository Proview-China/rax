# Workspace Read Post-Actual Durability V2 Plan

## 预期产物

完成后将得到一个 additive V20 Sandbox Owner闭环：actual point前有create-once
Qualification，actual point后有append-only Terminal，重启或TTL crossing后仍能exact
Inspect原结果，且不会重读文件。V1/V2保持兼容，Tool仍只消费Sandbox历史证据。

## 阶段A：合同与端口

- [x] `WorkspaceReadExecutionQualificationRefV2/FactV2`：Validate/Seal/稳定ID。
- [x] `WorkspaceReadPhysicalJournalLookupV2/RefV2`：neutral exact journal lookup/坐标。
- [x] `WorkspaceReadTerminalRefV2/FactV2`：outcome闭集
  `observed|indeterminate`、Validate/Seal/稳定ID。
- [x] Observed携完整可重算S2 proof；Indeterminate携error-class确定性推导的evidence且无content。
- [x] public exact terminal reader；Owner-private Qualification/Terminal repository ports。
- [x] Qualification封存AdmissionAttemptBinding digest，显式关联不同的Runtime/Sandbox
  Attempt坐标；禁止ID相等假设。
- [x] Qualification封存exact Association与Workspace Lease digest；S2 proof逐轴回证S1。
- [x] public caller-sealed proof不能直接取得Owner terminal capability；Indeterminate stage不能自选。
- [x] contract tests覆盖identity、splice、Outcome互斥、Unknown无content、TTL后shape可读。

## 阶段B：SQLite V20

- [x] additive v19→v20 migration；无silent repair。
- [x] qualification history与terminal history，均append-only且无current pointer。
- [x] full exact axes、canonical JSON、row digest、FK/UNIQUE与strict physical schema验证。
- [x] commit Unknown只Inspect exact Ref；8 handles/restart/64并发单winner。
- [x] old v19 facts可读但绝不回填V20；旧Inspection只可兼容报告indeterminate且不恢复执行。

## 阶段C：Kernel 与 Data Plane

- [x] actual point前用Go已验证S1 closure、sealed CurrentQueryV2 digest、expected Runtime
  current digest与actual request digest create/inspect Qualification；不得预填Rust S2
  Projection。
- [x] Rust journal发布neutral exact Ref/record digest；Kernel私有IPC桥形成内部evidence capability，
  contract shape校验不能代替source proof；公开Client/ActualPoint无physical旁路。
- [x] IPC dispatch维持ValidateCurrent；Inspect改为shape-only+exact request/payload/attempt。
- [x] Rust Completed + kernel outcome S2成功 + exact Observation/完整S2 proof→Observed；Completed后S2前
  崩溃以及Started无Complete/S2不确定均→Indeterminate且无content。
- [x] terminal persist使用immutable exact closure，不重新ValidateCurrent。
- [x] legacy public writers继续Fail Closed。

## 阶段D：黑盒与发布门

- [x] S2恰在expiry：Unknown历史可重启读取，physical read=1。
- [x] S2在expiry前成功、exact Observation已形成、Terminal commit晚于expiry：Observed
  历史可读。
- [x] Rust Completed后、Go S2前崩溃：重启Inspect journal并落Indeterminate且无content，
  不重读。
- [x] Rust Started无Complete：落Indeterminate，不重读。
- [x] 64并发、lost commit reply、Observed/Unknown冲突、逐轴splice。
- [x] Unknown表/JSON不含content；v19→v20/schema drift/8 handles。
- [x] focused ordinary×100、race×20、full ordinary/race、vet、Rust test/clippy、diff-check。
- [x] 独立P0/P1审计通过后更新`.properties.rax/module`与粗粒度memory。

## 合并约束

V20作为PR #79的必需P0闭包，不得让仅含v19修复的PR先合并。任何实现若通过延长V1
TTL、重新Seal Attempt、重发physical read或把Unknown写成Failed来规避V20，均判Fail
Closed。完成V20也只表示Sandbox owner-local结果持久性闭合，后续Tool typed handoff仍是
独立切片。
