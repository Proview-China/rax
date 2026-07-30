# Harness Exact Model Turn Adapter V2

## 1. 当前结论

本切面把 Model Owner 已发布的 `InvocationMaterialV2` 与
`GovernedModelTurnPortV3` 接入 Harness，但只形成 Harness 自有的
`attempt_bound -> outcome_bound` 引用事实。

它不调用 Provider、不执行 Tool、不创建 PendingAction、不推进 Turn，也不写
Context、Tool 或 Model Owner 事实。

当前实现是 **owner-local 代码候选，等待独立终审**。由于仓库尚无把 Context
与 Tool 的生产事实库装配为 Model 中立 Pair Reader 的 production
composition adapter，本切面不得宣称 production root 已闭合。

流程图见 [exact-model-turn-adapter-v2.drawio](exact-model-turn-adapter-v2.drawio)。

## 2. 公共输入和输出

- 输入：`bridgecontract.ModelTurnExactEnvelopeV2`
  - 精确绑定 `InvocationMaterialRefV2`；
  - 精确绑定 `GovernedModelTurnCommandV3`；
  - `RequestedNotAfterUnixNano` 只能缩短所有 Owner current 窗口。
- 输出：`bridgecontract.ModelTurnDispatchFactV2`
  - `attempt_bound`：Harness 已持久绑定唯一 Model attempt；
  - `outcome_bound`：Harness 已持久绑定该 attempt 的精确 Model V3 outcome ref。

持久化 Port 是
`ports.ModelTurnDispatchRepositoryV2`，执行入口是
`ports.ExactModelTurnPortV2`。

## 3. 调用顺序

1. 严格校验 Envelope 并派生确定性 DispatchRef。
2. 从 SQLite sidecar 精确 Inspect；不存在时 create-once 写入
   `attempt_bound`。
3. S1 依次读取：
   - Model `InvocationMaterialV2`；
   - ContextFrame + ContextMaterial Pair；
   - ToolInjectionMaterial + ToolSurface Pair。
4. 调用 Model Owner 的 `StartOrInspectGovernedModelTurnV3`。该 V3 首切面只产生
   Model owner-local `prepared` outcome，不包含 Provider 调用。
5. S2 重读完整四源闭包；任一 Ref、摘要、current projection 或 TTL 漂移均
   fail closed。
6. 使用新鲜时钟检查共同最短 TTL，CAS 写入 `outcome_bound`；无论 Bind
   正常返回还是 Unknown 后 exact Inspect 恢复，返回前都必须再次取得 fresh
   trusted clock，拒绝 clock regression、`now == common expiry` 和不再 current
   的 Outcome。
7. create/bind 回包丢失只通过精确 Inspect 恢复；恢复 context 忽略 caller
   cancel，但必须由 Envelope/S1/共同最短 TTL 的 exact deadline 截断，不换
   ID、不盲重派。

## 4. 所有权与依赖

```text
Harness bridgecontract/ports
  -> Model 公共 neutral refs/ports
  -> Runtime core

Harness modelinvokeradapter
  -> Harness bridgecontract/ports
  -> Model 公共 neutral refs/ports
  -> Runtime core
```

禁止依赖 Context、Tool、Application 的实现包，禁止依赖 Harness
`kernel/loop.go` 或旧 `ModelTurnPort`。

四源的 Owner/Kind 由 Model 公共 lineage 合同校验；Harness 生产代码不得写死
这些字符串。

## 5. 持久化不变量

- Dispatch ID 由完整 Envelope 与 Model attempt 确定性派生。
- SQLite 行保存完整 canonical Fact，并在每次读取时交叉校验索引列。
- 状态只允许 `attempt_bound -> outcome_bound`，不能回退或覆盖。
- 相同 ID、不同 canonical 内容必须 Conflict。
- fresh 数据库只在三个 owner schema 对象全部不存在时创建；一旦 V2 ledger
  已存在，Open 必须先验真现有物理对象，禁止用 `IF NOT EXISTS` 静默修复被删
  的表或索引。无 ledger 的 partial schema 同样 fail closed。
- 数据库启用 WAL、foreign keys、`synchronous=FULL`；schema ledger 版本集合必须
  精确为 `{2}`，并在每次 Open 时复核完整 DDL、`table_xinfo`、
  `index_list/index_xinfo`、无额外 trigger 以及回滚约束 probe。
- 本实现只声明单节点 durable sidecar，不声明 HA、远程后端或 SLA。

## 6. 反例和验收

测试必须覆盖：

- Envelope/DispatchRef 摘要漂移与非 canonical JSON；
- 四个 source role 分别在 S2 漂移；
- `now == expiry`、调用途中 TTL crossing、clock rollback；
- typed-nil、cancel；
- 三条 Unknown 恢复 Inspect 均等待 `ctx.Done()`，且 deadline 精确绑定各自 TTL；
- outcome Bind 已落盘后 clock rollback 或 `now == common expiry` 均 fail closed，
  不向调用方返回 `outcome_bound`；
- attempt create 与 outcome bind 回包丢失；
- 进程重启后从 `attempt_bound` 和 `outcome_bound` 恢复；
- 64 个独立 Adapter 共享同一 SQLite sidecar 的唯一 winner；
- 预创建无 PK、额外列、错误同名 index、弱化 CHECK 或额外 ledger version
  的数据库在 reopen 时 fail closed；
- 正确 ledger 下缺失表/索引、以及无 ledger 的 partial schema 在 reopen 时
  fail closed，且 Open 不得自行修复；
- 全路径 Provider/Tool/PendingAction/Turn 能力为零。

代码入口：

- [model_turn_dispatch_v2.go](../../../../ExecutionRuntime/harness/bridgecontract/model_turn_dispatch_v2.go)
- [model_turn_exact_v2.go](../../../../ExecutionRuntime/harness/ports/model_turn_exact_v2.go)
- [exact_model_turn_v2.go](../../../../ExecutionRuntime/harness/modelinvokeradapter/exact_model_turn_v2.go)
- [sqlite_model_turn_dispatch_v2.go](../../../../ExecutionRuntime/harness/modelinvokeradapter/sqlite_model_turn_dispatch_v2.go)
