# Runtime Kernel、三维状态与持续协调

## 1. 定位

Runtime Kernel是一个具体`AgentInstanceID + instance_epoch`的生命周期所有者。它执行已经线性化的期望状态，协调Binding Saga、Harness、Effect和清理，不定义Agent目标或相邻模块语义。

## 2. 三维状态

单线状态机无法表达失联、Fencing和清理不确定性。实例状态必须由三个正交维度组成。

### LifecyclePhase

```text
pending -> admitted -> preflighting -> activating
        -> provisioning -> binding -> starting
        -> ready -> running -> stopping -> terminal
```

Pause与Checkpoint是可选Capability；未声明支持时不出现在可用迁移中。

### ExecutionCertainty

```text
confirmed
unknown
lost
fenced
```

### CleanupStatus

```text
not_required
pending
complete
failed
indeterminate
```

合法组合示例：

```text
stopping + lost + pending
terminal + fenced + indeterminate
terminal + confirmed + complete
```

`fenced`只承诺旧执行面不能获得新的授权Effect；不承诺物理进程死亡。`terminal`只表示Runtime不再推进该具体Instance；不等于Cleanup Complete或Remote Continuation结束。

### 可判定合法组合谓词

状态写入前必须同时满足下列谓词，而不是只比对示例：

1. `LifecyclePhase`只能沿正常主链前进；任一非terminal阶段可因Stop/Fence/失败跳到`stopping`，尚未产生任何清理义务时可直接到`terminal`；`terminal`对该Instance不可逆。
2. `pending/admitted/preflighting`只能是`certainty=confirmed`且`cleanup=not_required`；这些阶段不得已经持有reserved/active外部Lease。Preflight成功或confirmed-not-applied必须不留下Runtime清理义务。
3. Preflight Probe若响应丢失、可能已变更外部状态或需要清理，状态写入必须原子跳到`stopping + unknown + pending/indeterminate`，不得停留为`preflighting + unknown`。除此之外，`certainty=unknown`允许`activating..terminal`；`certainty=lost`只允许`provisioning..terminal`，表示最后已知Lifecycle未被证实改变。
4. 已提交的`certainty=fenced`必须同时处于`stopping`或`terminal`；Fence更新不能留下`ready/running + fenced`组合。
5. 只有`certainty=confirmed`且Identity/Sandbox Lease、Authority和强制事实有效时才能推进Lifecycle或派发一般业务Effect。`unknown/lost/fenced`只能派发下文“受限恢复Effect白名单”中的动作；白名单动作不能推进或复活Lifecycle。
6. `cleanup=not_required`要求从未创建待清理资源、挂载、凭据路径或本地Continuation；首个此类义务出现时单调转为`pending`。
7. `cleanup=complete`只允许`phase=terminal`且所有声明的本地清理义务都有权威Receipt/Inspect；RemoteContinuation与Provider Retention仍在独立维度报告。
8. `cleanup=failed/indeterminate`只允许`stopping/terminal`；授权重试可回到`pending`。若complete后出现证明遗漏义务的权威证据，必须产生`cleanup_completion_retracted`并转为indeterminate，保留原完成声明历史。
9. `terminal + cleanup=pending/failed/indeterminate`合法；terminal不等待清理完成。`terminal + cleanup=not_required`仅适用于从未产生清理义务的失败或拒绝。
10. Effect UnknownOutcome只改变`EffectSettlement`；只有它同时使实例执行事实不可判断时才把`ExecutionCertainty`变为unknown，不能机械折叠两个维度。

### 状态迁移判定表

| 迁移/触发 | 前置条件 | 成功写入 | 失败落点 |
|---|---|---|---|
| pending→admitted | Static Admission通过、无外部资源 | admitted/confirmed/not_required | terminal/confirmed/not_required + admission failure |
| admitted→preflighting | Probe已声明预算、幂等/查询、可能变更与清理边界 | preflighting/confirmed/not_required | confirmed拒绝且未发生→terminal/confirmed/not_required；结果未知/可能残留→stopping/unknown/pending或indeterminate |
| preflighting→activating | proposed对象和Snapshot有效 | activating/confirmed/not_required | terminal/confirmed/not_required |
| activating内预留Lease | write-ahead Intent与reserved权限有效 | activating/confirmed/pending | stopping + confirmed/unknown + pending/indeterminate |
| activating→provisioning | ActivationCommit成功、实际LeaseRef已绑定 | provisioning/confirmed/pending | stopping + confirmed/unknown + pending |
| provisioning→binding→starting | 上一步Required事实已独立验证 | 下一phase/confirmed/pending | stopping + confirmed/unknown + pending |
| starting→ready | 满足第3节全部Ready条件 | ready/confirmed/pending | 保持starting等待，或stopping + 相应certainty |
| ready→running | 无活跃Run、命令有效、全部Lease/Fence在线重验 | running/confirmed/pending | ready保持不变并记录TypedError |
| 任意外部阶段→lost | Inspect不可达且无法证明死亡 | phase不变/lost/原cleanup或pending | 不允许推进，等待Fence/Inspect |
| 任意非terminal→fenced | 安全命令线性化或Fence事实成立 | stopping/fenced/pending或indeterminate | 本地Fence失败仍记录stopping/unknown并升级隔离 |
| unknown/lost→confirmed | 权威领域事实或独立Inspect覆盖全部相关冲突域，且没有证据缺口 | phase保持原值/confirmed/独立cleanup值 | 保持原certainty，继续Quarantine并记录缺口 |
| stopping→terminal | Runtime不再推进该Instance，ExecutionOutcome已形成 | terminal/原certainty/独立cleanup值 | 保持stopping并记录证据缺口 |

迟到事件必须按Instance/Lease/Effect epoch关联原对象，不能执行状态迁移；重建永远创建新Instance ID和更高epoch，不存在terminal→pending或fenced→running。

### 受限恢复Effect白名单

`unknown/lost/fenced`不是“绝对禁止一切新Effect”，而是禁止扩大业务能力，只允许以下可判定集合：

1. **Inspect**：只读查询领域权威事实或取得独立Attestation；若查询本身是外部Effect，也必须有新的Intent、Fence、Authority和适用Budget；
2. **原Intent结算**：查询或接收原`effect_intent_id/revision`的Receipt、UnknownOutcome对账与Settlement，不创建替代业务效果；
3. **Emergency Safety**：本地Fence、网络隔离、凭据访问路径撤销或同等单调安全动作；事实源不可达时可先执行，恢复后必须补写证据；
4. **Cleanup/Release**：只在不会扩大权限或冲突效果域、具有新的EffectIntent、当前Fence/Authority、适用Budget与Write-ahead Evidence时，允许释放Lease/Reservation、删除临时资源或执行受权清理；结果未知时保持cleanup pending/indeterminate，不盲重试；
5. **Compensation**：只有原Effect已经由权威Receipt/Inspect确定、补偿动作有独立授权且满足第4项全部约束时允许。原Effect仍为UnknownOutcome时禁止用补偿猜测原效果。

白名单判定失败返回`recovery_effect_not_permitted`，不改变三维状态。Budget Release等“释放”仍是新Effect，不能因为名称安全而跳过Intent、Fence、Authority或Evidence。

### ExecutionCertainty收敛

- 非terminal Instance的`unknown/lost`只有在权威事实或独立Inspect覆盖导致不确定的全部Effect、Lease与冲突域后，才可原地回到`confirmed`；Lifecycle保持原阶段，再由正常迁移规则决定是否继续或停止。
- `fenced`是已经线性化的安全事实，不因进程重新可达而撤销；要恢复服务只能创建满足ReplacementPermit的新Instance，旧Instance继续`stopping/terminal + fenced`。
- terminal Instance不得因Inspect收敛而复活Lifecycle。若终止后能够证明执行事实，可把certainty从`unknown/lost`收敛为`confirmed`并补充Settlement/Cleanup；若仍无法证明，则保持terminal+unknown/lost，只结算各个独立Effect。
- 任一Inspect只覆盖其声明的事实范围；部分收敛不能把Instance整体标记confirmed。

## 3. Ready与终止

Harness的Ready只是Observation。Instance进入Ready至少需要：

- Harness身份、版本和Manifest与ActivationSnapshot一致；
- Sandbox Provider独立Inspect确认进程/端点存在；
- Required Binding全部处于`applied + independently_verified`；
- IdentityExecutionLease、Fence、Authority、Budget和强制live facts仍有效；
- 没有Required步骤处于unknown effect；
- Harness Conformance不低于Plan要求。

终止报告必须拆分：

```text
execution_outcome
execution_certainty
cleanup_status
effect_settlement
remote_continuations_status
provider_data_retention_status
```

无法证明进程停止时返回`termination_indeterminate`，禁止把“已发Stop”写成Terminated事实。

## 4. Reconcile

每轮Reconcile：

1. 读取目标Instance、expected revision和Desired State；
2. 读取Identity Lease、Fence、Lease、Binding、Harness、Effect和远程状态；
3. 拒绝旧Instance、旧epoch、旧命令和迟到Observation；
4. 计算唯一安全的下一动作；
5. 对外部Effect先持久化Intent；
6. 执行动作并回读权威或独立状态；
7. 更新三维状态、Saga、Effect和证据关联；
8. 无法证明结果时进入unknown/indeterminate，而不是猜测成功或失败；
9. 仅在Policy允许时退避重试，禁止叠加未知Provider重试。

### 4.1 24×7逻辑监督

持续运行实例的监督参数必须由部署显式给出，Runtime不内置生产SLA。每份`SupervisionRecord`绑定精确的Policy Digest；间隔、续租提前量、退避上限或失败预算变化时，旧记录必须拒绝继续执行，经过显式迁移后才能采用新策略。

监督决策遵守以下顺序和不变量：

1. Identity Lease到期立即Fence并停止，进入续租窗口时续租优先于普通健康检查；
2. 普通检查按稳定身份散列分散时间，瞬时故障采用有界指数退避，避免集中唤醒和无界重试；
3. 失败预算耗尽或状态未知时进入Quarantine，只能凭权威Inspect结果恢复；
4. 旧Observation、旧Lease revision和迟到健康结果不能覆盖新事实；
5. Fenced实例或已过期Identity Lease不能被迟到成功结果复活；
6. 调用方必须先以revision CAS持久化新的监督记录，再派发决策对应的外部动作。

当前切面只冻结上述纯逻辑合同和确定性测试，不选择生产Scheduler、Clock、事实后端、进程拓扑或SLA。

## 5. 重建

重建创建同Lineage下新的Instance ID和更高epoch，但必须先取得[ReplacementPermit](../safety/README.md)。旧Instance保持历史终态，不能被覆盖。若Plan Digest变化，则必须创建新Lineage。

## 6. 失败与迟到结果

| 情况 | 结果 |
|---|---|
| Harness启动迟到但Instance已Fenced | 记录迟到Observation，拒绝Ready并请求清理 |
| Stop发送后节点失联 | stopping + lost + pending/indeterminate |
| Provider返回旧epoch Receipt | 只关联旧Effect，不改变新Instance状态 |
| Cleanup失败 | 保留原始失败，追加cleanup failed，不覆盖原因 |
| Event缺口 | 标记Evidence Gap；安全状态不能仅依赖缺失流 |

## 7. 最低反例

- `STATE-01`：节点断电后不得直接标记Cleanup Complete；
- `STATE-02`：Harness签名Ready但Sandbox Inspect找不到进程，Instance不得Ready；
- `STATE-03`：旧Instance迟到Ready不能复活终态；
- `STATE-04`：Stop超时且Remote Batch仍活动，只能报告本地/远程分维度状态；
- `STATE-05`：同Plan重建必须产生新Instance ID和更高epoch。
- `STATE-06`：状态写入`running + fenced`必须被验证器拒绝并改为stopping/fenced；
- `STATE-07`：terminal + cleanup pending合法，不能为满足单线终态而伪造complete；
- `STATE-08`：旧epoch迟到事件试图把terminal改回ready，必须只保存为迟到证据。
- `STATE-09`：Preflight Probe响应丢失且可能创建远程对象，必须原子进入stopping/unknown/indeterminate，不得保留preflighting/confirmed或进入Snapshot。
