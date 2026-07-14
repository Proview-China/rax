# Runtime公共合同与最小可运行基座

## 1. 状态与授权

- 当前阶段：设计补全并获用户授权直接落地；
- 实现范围：`ExecutionRuntime/runtime`中的公共合同、组件注册与装配门禁、组件中立协调器、fake Port和完整闭环验收；
- 明确不做：任何真实Harness、Tool/MCP、Memory/Knowledge、Sandbox、Review、Context/Cache内部算法，生产数据库、RPC或真实外部集成。

## 2. 固定实施路径

```text
冻结公共合同
-> 最小可运行基座
-> 组件通过责任域Port接入
-> 每接一个组件运行完整闭环验收
```

基座不是“万能Runtime”。它只拥有执行身份、生命周期、命令、Fence、统一Effect协调、时间线关联、Checkpoint协调和Port装配门禁；每个组件继续拥有自己的领域事实与算法。

## 3. 公共合同闭合范围

### 3.1 对象与连续性

- Identity、Lineage、Instance、Run和SandboxLease引用；
- 三维实例状态与单活跃Run；
- `TimelinePoint`同时保存Ledger顺序、Source顺序、因果、观察时间和摄取时间；
- `CheckpointSet`保存参与者快照、Plan/Profile/Context摘要和Effect四类水位；
- Restore/Rewind只能从`consistent` Checkpoint创建新Instance ID和更高epoch，不能复活旧Instance或声称回滚外部世界。

### 3.2 组件装配

- `ResolvedAgentPlan`只保存版本固定的组件需求、依赖、能力、允许Residual和摘要；
- `ComponentRegistry`按组件ID注册具体Port Adapter，并验证Kind、版本、Digest、合同版本、能力证据TTL和依赖；
- Required组件缺失或能力未达到`bound`时拒绝；Optional组件只有Plan允许时才形成Residual；
- 组件Digest、Harness、Route、Context语义或工具可见面变化必须生成新Plan/Lineage。

### 3.3 责任域Port

公共Port分别覆盖：Assembly/Profile、Execution、Context、Tool、MCP、Memory、Knowledge、Asset、Organization、Review、Management、Sandbox、Budget、Evidence、Timeline和Checkpoint。可以共享公共Envelope和Effect合同，但不得用一个万能Provider接口吞并领域语义。

### 3.4 最小运行闭环

```text
Validate Plan/Registry
-> Static Admission
-> create durable ActivationAttempt(proposed)
-> Execution Preflight
-> freeze ActivationSnapshot(pre-allocation facts only)
-> reserve IdentityExecutionLease
-> write-ahead Sandbox Allocate Intent
-> reserve_quarantined SandboxLease
-> journal Sandbox reservation confirmed
-> atomic Activation Commit(Attempt + Instance binding + Identity Lease active)
-> write-ahead Sandbox Activate Intent
-> activate Sandbox and journal authoritative result
-> open Execution endpoint
-> independent Inspect Ready
-> Ready / single Run / event observation
-> optional consistent Checkpoint
-> Stop / Close / Fence / Release
-> independent cleanup evidence
-> terminal report
```

任何外部步骤回包丢失都进入unknown/indeterminate；不得用下一步成功反推上一步已经完成。

重启后先读取Activation Journal并调用确定性的`PlanRecovery`。`intent_recorded`或`unknown_outcome`只能进入对应Inspect；其中unknown还必须保持Quarantined，禁止直接重复派发。Foundation只返回唯一安全下一动作，不替代具体Port执行恢复Effect。

## 4. 每个组件的统一接入门禁

每接入一个组件，必须重复执行：

1. Descriptor、版本、Digest、Capability和证据TTL静态校验；
2. Required/Optional、依赖DAG和Residual装配校验；
3. 正常激活、运行、停止、清理黑盒闭环；
4. 取消、超时、迟到事件、旧epoch、Evidence不可用和组件失联故障注入；
5. Effect Intent/Fence/Receipt或只读无Effect证明；
6. Checkpoint参与能力存在时验证Barrier、Snapshot、Abort和Restore；不存在时必须明确`unsupported`；
7. Race、Vet和组件合同测试。

## 5. 最小基座完成条件

- 公共Schema和Port可编译并有非法输入反例；
- fake Environment、Execution、Evidence和Checkpoint参与者能够跑通完整闭环；
- 两种不同Conformance的fake Execution Adapter通过相同外部合同；
- Required组件缺失、Capability过期、Ready自报与独立Inspect冲突、Checkpoint partial和Cleanup不完整均被拒绝；
- Sandbox Allocate/Activate/Release已生效但回包丢失时，不伪报成功、不重复派发，并保留Journal或Cleanup未完成状态；
- `go test ./...`、`go test -race ./...`和`go vet ./...`通过；
- 说明、计划、验收、项目索引和memory同步。
