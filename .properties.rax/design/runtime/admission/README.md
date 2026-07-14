# StaticAdmission、BoundedPreflight、Activation与BindingSaga

## 1. 三阶段边界

### StaticAdmission（静态接纳）

纯本地、确定性、无外部Effect：

- Schema、版本和摘要；
- Plan不可变性与秘密明文检测；
- Profile/Route/Capability静态组合；
- Authority引用结构、Policy冲突和Binding DAG；
- Required/Optional与允许Residual；
- Extension Schema与静态信任证据。

失败不得分配Lease、访问Provider或生成账单。

### BoundedPreflight（有界预检）

Static Admission通过后先创建proposed Lineage/Instance身份，Preflight全部关联该proposed Instance；它仍不授予执行权。

只允许声明过的有界探测：

```text
probe_type
credential_scope
network_target
max_requests / max_duration
possible_charge / possible_mutation
cleanup
evidence TTL / digest
```

允许读取元数据、受控健康和Harness Manifest。成功或confirmed-not-applied的Probe不得留下Runtime清理义务。若声明的Probe可能产生有限变更，必须可查询/补偿并在结果未知或清理残留时原子把proposed Instance转为`stopping + unknown + pending/indeterminate`；不得形成ActivationSnapshot。发送用户Prompt、创建预期长期资源、触发Tool、产生业务写入或不可忽略费用的探测属于Activation，不得伪装为Preflight。

### Activation

在第一项Activation外部Effect前，proposed身份已经承载并完成Bounded Preflight；随后生成不可变`ActivationSnapshot`并重新验证live facts：

```text
authority_epoch
entitlement_digest + expiry
credential_ref state + expiry
route/model/harness digests
capability evidence digest + TTL
extension trust/revocation state
sandbox provider capability attestation（非具体Lease）
policy digest
pricing evidence
budget policy digest / requested executable cap
provider quota/health evidence
proposed_lineage_id
proposed_instance_id / proposed_instance_epoch
sandbox_requirement_digest / requested_effect_domains
```

任一强制事实漂移、过期或撤销时Activation失败。Snapshot绑定proposed Instance身份和Sandbox需求摘要，**不绑定尚未产生的IdentityExecutionLease ID或SandboxLease ID**。

## 2. 无循环对象建立协议

唯一顺序如下：

| 顺序 | 动作 | 成功状态 | 此时允许的权力 | 失败恢复 |
|---|---|---|---|---|
| 1 | 创建/解析Lineage记录 | `lineage=proposed` | 无执行权 | 标记`abandoned` |
| 2 | 预留新Instance ID与epoch | `instance=proposed` | 无Lease、无业务Effect权 | 标记`aborted`，ID永不复用 |
| 3 | 执行Bounded Preflight | `instance=preflighting` | 只允许声明的Probe Effect | Probe必须结算；未知/残留则停止，不得进入Snapshot |
| 4 | 形成ActivationSnapshot | `snapshot=frozen` | 只允许提交同一attempt的申请 | 漂移则失效，回收proposed对象 |
| 5 | 申请IdentityExecutionLease | `identity_lease=reserved` | 排他占位；仅允许同一attempt的激活/清理Effect | revoke/release/expiry；结果未知则禁止竞争Lineage激活 |
| 6 | 以EffectIntent取得适用Budget Reservation | `budget_reservation=reserved`或明确not_required | 只覆盖声明的Activation cap | Unknown保留cap占额；不足或无可执行cap则停止 |
| 7 | 以EffectIntent申请Sandbox | `sandbox_lease=reserved_quarantined` | 不能运行Harness、不能取得冲突域 | Fence并Release；未知则保持Quarantine和Residual |
| 8 | 最终重验并线性化ActivationCommit | `identity_lease=active`、`lineage=active`、`instance=provisioning`并绑定Budget/Sandbox引用 | 可按Fence签发后续Binding Effect | 提交失败则不得激活Sandbox；清理reserved Lease/Reservation |
| 9 | Sandbox Provider确认激活 | `sandbox_lease=active` | 只允许Plan内、Fence有效的Binding/Run Effect | 失败进入stopping/cleanup；不能回退复用Instance ID |

`ActivationCommit`是已有Control Plane事实的一次逻辑提交，不是新模块，也不承诺与Budget/Sandbox Provider跨系统原子提交。Preflight Probe按其受限Effect合同记录；步骤5以后每个外部动作都有统一EffectIntent。reserved Identity Lease已排除双主，但不授权业务Run；Budget Reservation只覆盖可执行cap；reserved Sandbox Lease必须被网络和凭据边界强制Quarantine。

恢复时根据最后一个权威状态继续：`proposed`可安全废弃，`reserved`必须Inspect后释放，结果未知则Fence对应冲突域并等待对账；`active`以后只能走正常Stop/Cleanup或ReplacementPermit，不能回退为proposed。

## 3. BindingSaga（绑定长事务）

跨Sandbox、MCP、Secret、Provider和Harness不承诺原子Rollback。每一步持久化：

```text
not_started
applying
applied
independently_verified
compensating
compensated
compensation_failed
unknown_effect
```

流程：

1. 持久记录Step Intent；
2. 执行并获取Receipt或Observation；
3. 独立Inspect可验证的实际状态；
4. Required步骤全部验证后才继续；
5. 失败时按依赖图逆序发起Compensation Intent；
6. Compensation失败或未知时保留残留并Fence相关Effect Domain；
7. Required步骤处于unknown effect时Instance不得Ready。

## 4. CompensationIntent

Compensation是新的外部Effect，不是免费的回滚：

```text
compensation_id
original_effect_intent_id
original_receipt_ref
compensation_action_digest
authority / fence
budget_reservation
idempotency_class
expiry
```

只有原审批显式包含精确、有界的补偿动作时才能复用，否则必须重新授权。不可逆、影响第三方权利、状态已漂移或补偿风险更高时禁止自动补偿。补偿成功不删除原效果历史。

原Effect仍是`UnknownOutcome`时禁止自动发起Compensation；必须先用Receipt、领域权威查询或独立Inspect确定原效果。处于`unknown/lost/fenced`的Instance只能按[Kernel受限恢复Effect白名单](../kernel/README.md#受限恢复effect白名单)派发Inspect、结算、Emergency Safety以及具有新Intent/Fence/Authority/适用Budget的Cleanup/Release；这些恢复Effect不得扩大权限或复活Lifecycle。

## 5. Live Fact漂移

| 漂移 | 处理 |
|---|---|
| Authority、Entitlement、Trust撤销 | Fence对应能力 |
| Route/Model/Harness身份变化 | 新Plan和Lineage |
| 强制安全能力消失 | Fence或Stop |
| Optional能力消失 | 仅Plan允许时Degraded |
| Pricing变化导致Reservation不足 | 阻止新计费Effect |
| 可观察性低于Harness准入等级 | Fence或降级到不允许Effect的模式 |

## 6. 失败结果

```text
static_admission_rejected
preflight_effect_budget_exceeded
preflight_evidence_expired
activation_fact_drift
activation_attempt_conflict
identity_lease_reservation_unknown
sandbox_lease_reservation_unknown
activation_commit_failed
binding_unknown_effect
compensation_authorization_missing
compensation_unknown
residual_not_allowed
```

## 7. 最低反例

- `ADM-01`：所谓能力探测发送真实Prompt，必须分类为Activation；
- `ADM-02`：Preflight后扩展Trust被撤销，Activation必须失败；
- `ADM-04`：Snapshot尚未产生真实Lease却要求绑定Lease ID，Schema必须拒绝该循环引用；
- `ADM-05`：Sandbox reserved成功但ActivationCommit失败，Lease必须保持Quarantined并进入清理，不得启动Harness；
- `ADM-06`：Preflight Probe响应丢失且可能留下远程对象，必须停止并进入unknown/cleanup，不得继续Snapshot。
- `SAGA-01`：远程MCP Attach成功但响应丢失，不得盲目重试或宣称回滚；
- `SAGA-02`：发送邮件后删除本地记录不能作为已发送邮件的补偿；
- `SAGA-03`：Memory绑定成功、Harness绑定失败且Memory释放未知，Instance不得Ready。
