# 统一Effect、Budget、远程状态与正式提交

## 1. Effect定义

任何越出纯本地、无披露、无费用、无持久化计算边界的操作都是Effect。

| 类别 | 典型操作 |
|---|---|
| `data_disclosure` | 向模型发送Context、网络外发、上传文件 |
| `cost_consumption` | 模型Token、GPU、付费Tool、Sandbox资源 |
| `resource_lifecycle` | Sandbox Allocate、挂载、远程Sidecar |
| `external_mutation` | Tool/MCP、文件、数据库、外部API写入 |
| `provider_continuation` | Session、Background Response、Batch、Async Operation |
| `hosted_execution` | Hosted Tool、Code Interpreter、远程Browser |
| `cache_state` | Provider Prompt Cache创建、远程查询/命中、写入、删除、保留；本地纯查找按下述规则处理 |
| `formal_commit` | Artifact发布、Memory正式提交 |
| `credential_operation` | Token签发、刷新、代理签名、Provider登录 |
| `safety_control` | Fence、Revoke、Kill、Cancel、断网 |

Invoker、Provider SDK、Official Harness和Hosted Tool不能因为不走ActionPort而绕过Effect治理。

## 2. EffectIntent与结算

除紧急收紧权限动作外，派发前必须持久化：

```text
effect_intent_id
effect_kind / risk_class
canonical_payload_digest
target / conflict_effect_domain
authorization / approval
execution_fence
budget_reservation_ref（适用时）
idempotency_class
provider/route/operation expectations
evidence classification
```

统一Fence和审批主键是`effect_intent_id + effect_intent_revision + canonical_payload_digest`。Tool/MCP等Action类Effect可以附带`action_ref`，但不能用Action ID代替非Action Effect身份。

状态：

```text
not_dispatched
dispatched
confirmed_applied
confirmed_not_applied
unknown_outcome
compensated
compensation_unknown
reconciliation_required
```

超时不等于失败。Runtime不得在不知道Provider是否执行时盲目重试或宣称Exactly Once。

## 3. 幂等能力

| 等级 | 超时处理 |
|---|---|
| Provider强幂等键 | 只能使用同一Intent和幂等键重试 |
| 可查询/可对账 | 先Inspect真实状态 |
| 非幂等且不可查询 | 禁止自动重试，进入unknown outcome |
| 不可信 | Admission拒绝 |

## 4. 最终授权

入口授权不能替代最终效果边界授权。Effect Authorization绑定actor、Identity、Instance/Lease/Fence epoch、Capability、Effect Intent ID/revision、参数摘要、目标、Policy、Approval、期限和Budget；Action引用仅在适用时附加。参数、工具Schema、目标或Revision变化使旧审批失效。Authority撤销后，未派发Effect拒绝；已派发Effect继续追踪Receipt。

## 5. BudgetAuthorityPort

Budget是必要能力，不预设新模块：

```text
DescribeBudgetPolicy
Reserve
Commit
Release
Reconcile
InspectReservation
WatchBudgetState
```

它可由Management、Organization/Authority、Provider Accounting或未来获批实现承载。绑定中必须指定唯一事实所有者。Runtime不拥有价格、币种、套餐和财务语义。

规则：

- 派发前按最坏上界原子Reserve；
- Receipt后Commit真实Usage并Release剩额；
- UnknownOutcome保留最坏占额；
- Provider迟报Usage进入Reconcile；
- 实际超额形成Budget Debt并阻断新计费Effect；
- Hard Budget不足时拒绝；Soft Budget只告警/审批；
- Reservation无法原子化时不能宣称硬预算。

Hard Budget只有在Effect存在可强制执行的有限最坏上界时成立，例如最大Token、时长、条目、字节、GPU时间、请求数或Provider可执行cap；所有可能产生费用的子Effect也必须被该上界覆盖。若stream、batch、后台任务或Provider计费无法设置/证明有限cap，只能拒绝Hard Budget请求，或经显式Policy降级为Soft Budget并重新审批；Runtime不得用历史均值冒充上界。UnknownOutcome按已承诺的cap保留占额，直到Receipt或对账收敛。

## 6. Cache读取、命中与持久状态

- Context Engine内部本地查找若不发生外部披露、费用或Provider通信，不需要独立Runtime EffectIntent，但必须先重验Partition/Authority，并产生`CacheAccessEvidence`；
- Provider缓存命中若发生在模型请求内部，由父模型`EffectIntent`覆盖，Intent必须携带cache namespace/key digest、数据分级、Usage cap与Retention Policy；Receipt返回hit/miss、Usage和可知的Retention事实；
- 单独远程lookup只要发送标识/Context、产生费用或触碰Provider状态，就是独立或父子关联的Effect，必须Intent-before-dispatch；
- Provider Cache创建、写入、删除、延长Retention始终是Effect；读取导致Retention刷新时也按持久状态Effect结算；
- 所有命中在返回内容前重新鉴权并验证Tenant/Principal/Authority/Harness/Route/Tool分区；失败不得把缓存内容交给调用者。

## 7. RemoteContinuation

Provider侧长期状态统一记录：

```text
continuation_id / effect_intent_id
provider_operation_id
type: session | background_response | batch | hosted_tool | prompt_cache | remote_sidecar
instance_id / instance_epoch / fence
data_classification
budget_reservation
cancel_capability
expiry / retention
state
```

状态：`active`、`cancel_requested`、`cancelled`、`completed`、`expired`、`orphaned`、`unknown`、`retained_by_provider`。

Sandbox释放不推导RemoteContinuation结束。支持Cancel时发送独立Cancel Effect；不支持时撤销后续交互能力并追踪到完成或过期。Provider没有Operation ID时明确标记不可协调。旧Remote Operation可能产生冲突效果时阻塞ReplacementPermit。

## 8. Artifact与Memory正式提交

```text
transient
-> candidate_submitted
-> under_review
-> accepted | rejected
-> commit_dispatched
-> committed | commit_unknown
```

Candidate携带内容摘要、Identity/Lineage/Instance/Run、Plan/Profile/Context摘要、Authority、分类和来源证据。Harness只产生候选；Review产生Verdict；Asset Manager或Memory Engine决定并执行正式提交；Runtime只协调和关联。

Commit是正式Effect，必须绑定当前Fence、Authority、Candidate Digest、Review引用、幂等键和Receipt。Fence后的旧Instance Candidate拒绝；Fence前候选可保留供查看，但迟到Commit默认需要当前Authority重新采纳同一Candidate Digest。

## 9. 失败结果

```text
effect_intent_missing
effect_fence_stale
effect_authorization_missing
unknown_outcome
budget_reservation_conflict
budget_reconciliation_required
hard_budget_cap_unprovable
cache_read_authorization_stale
remote_continuation_outstanding
provider_retention_unknown
candidate_stale_epoch
commit_unknown
```

## 10. 最低反例

- `EFF-01`：模型请求外发Context但无Intent，必须拒绝；
- `EFF-02`：Hosted Tool不可观察且能持久写外部状态，Harness Route必须降级或拒绝；
- `EFF-03`：付款请求超时，禁止自动再次付款，进入unknown outcome；
- `BUD-01`：预算100、两个并发动作各Reserve 70，只能一个成功；
- `BUD-03`：无最大Token/时长的stream或无上限Batch请求Hard Budget，必须拒绝或显式降级Soft；
- `CACHE-05`：Provider cache lookup单独发送Key并计费，不能伪装成本地命中，必须有EffectIntent；
- `CACHE-06`：父模型请求命中Provider Cache但Authority已缩小，必须在内容返回前拒绝；
- `REMOTE-01`：Sandbox已释放但Provider Batch仍运行，不能报告完全清理；
- `REMOTE-02`：Prompt Cache无法删除，必须报告Provider保留状态；
- `COMMIT-01`：旧Instance重建后迟到提交Memory Candidate，默认拒绝；
- `COMMIT-02`：Commit回包丢失，必须查询原Intent，不能重复提交。
