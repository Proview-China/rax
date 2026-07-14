# Runtime Control Plane、命令线性化与CAP边界

## 1. 定位

Control Plane保存期望状态、Instance/Lineage注册、Command Log和Fence协调。Human、Management和Policy没有隐含优先级，所有控制权来自明确Authority Scope、目标、前置条件和线性化顺序。

## 2. 命令线性化

单Instance命令的线性化点是：

```text
validate actor/authority/target
+ compare expected_revision
+ persist CommandRecord
+ advance aggregate_revision
+ persist desired-state/outbox intent
```

任一部分不能持久化时，命令不得标记Accepted。长操作返回OperationRef，执行结果通过Watch或Query取得。

## 3. 安全支配关系

```text
revoke / fence > approve / start / resume / provide_input
stop_instance > start / resume / provide_input
cancel_run > provide_input
deny(effect_intent_revision) > 尚未派发的approve(effect_intent_revision)
identity_stop > rebuild
```

- Stop一旦线性化，该具体Instance不可恢复；恢复创建新Instance；
- 更高revision安全命令到达后，已接受但未执行的推进命令变为`superseded`或`invalidated`；
- 已派发Effect不能被后到Deny伪装为未发生，只能继续结算；
- Approve绑定Effect Intent Revision、参数摘要、目标、期限和Fence；Tool/MCP类可附带ActionRef，但不产生通用推进权；
- Rebuild必须等待ReplacementPermit。

## 4. 命令结果

```text
accepted
rejected
executing
completed
superseded
invalidated
indeterminate
```

典型失败：

```text
stale_instance_epoch
revision_conflict
authority_revoked
precondition_failed
linearizable_authority_unavailable
evidence_unavailable
replacement_permit_missing
```

## 5. 网络分区与CAP

必须由唯一线性化事实源决定：

- IdentityExecutionLease；
- Command Log和Desired State revision；
- Fence/Authority epoch；
- Budget Reservation；
- 新Effect Intent；
- 正式Artifact/Memory Commit Intent。

无法访问该事实源时：

| 操作 | 行为 |
|---|---|
| 查询 | 可返回带watermark和`stale=true`的投影 |
| Start/Resume/Rebuild | fail closed |
| 新Approve/Provide Input | fail closed |
| 新Lease或续租 | fail closed |
| 新授权、新Effect Intent | fail closed |
| 分区前已经持久化并授权的Effect | 仅当Intent声明`leased_offline`、Token/Fence尚在期限内且Risk Policy允许时才可派发/继续；`online_strict`失去事实源即fail closed。已在Provider内执行的操作只允许结算，不等于允许新增步骤 |
| 本地Fence/Kill/断网 | 允许紧急执行，随后补证 |

高可用不能替代一致性。少数分区不得续租、递增epoch或接受推进命令。

## 6. Emergency Safety

Fence、Kill、断网和撤销本地挂载可以在权威证据面暂时不可用时先执行，因为它们只收紧权限。动作必须写本地受限Emergency Journal；恢复后补入权威证据。无法记录时仍以停止危险为优先，但必须产生`evidence_gap`，不得声称审计完整。

## 7. 最低反例

- `CMD-01`：Resume已接受但未执行，随后Stop线性化，Resume必须Superseded；
- `CMD-02`：Approve后参数Digest改变，旧审批必须失效；
- `CMD-03`：两个网络分区同时启动同Identity，只允许能访问线性化事实源的一侧成功；
- `CMD-04`：证据Intent无法持久化时不得接受新的模型调用或Tool动作；
- `CMD-05`：Deny晚于已派发Effect时不能报告“未发生”。
- `CMD-06`：`online_strict`模型请求已持久化但派发前失去Authority事实源，最终边界必须拒绝；
- `CMD-07`：`leased_offline` Intent的Token已过期，即使分区前获批也不得继续派发新步骤。
