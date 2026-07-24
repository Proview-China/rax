# Review Human Enterprise Multi-Sign V2

## 1. 冻结状态

- 最高业务输入：`tmp.document/Review.md`。
- 用户于 2026-07-17 明确选择：**租户 Policy 必填的 K-of-N、多角色约束、具备 veto 权的 Reviewer Reject 硬否决、只接受 Authority Owner 当前显式 Delegation、生产环境禁止自审**。
- 本文件只扩展 Human Route；Auto Route、Bypass、Runtime Permit/Begin、Provider 执行与 Run Outcome 的 Owner 不变。
- 现有单 Assignment/单 Attestation `V1` 保留历史读取和 reference/test 兼容；它不能宣称满足企业多签生产语义。Human production root 必须使用 V2。

## 2. Owner 与非 Owner

| 对象 | 唯一 Owner | Review 的权限 |
|---|---|---|
| Panel、Panel Assignment、Human Attestation、Quorum Decision、Verdict V2、Trace | Review Verdict Owner | create-once、append-only history、current index、CAS、撤销/过期/替代 |
| Quorum/角色/veto/自审/委托允许规则 | 租户 Policy Owner | 只 exact 复读 sealed current Policy；无 Policy Fail Closed |
| 人类 Identity、Role Authority、Delegation、撤销 | Organization/Authority Owner | 只 exact 复读；不签发、不延长、不缓存为真值 |
| Candidate 作者/发起者责任坐标 | Target/Organization Owner | 只 exact 复读，用于生产禁自审；不从显示名或请求文本推断 |
| Reviewer Binding/Capability | Runtime Binding Owner | 只 exact 复读 authoritative current closure |
| Scope/Evidence current | Scope/Evidence Owner | 只读；不由 Review 补签 digest |
| wait/resume、PhaseDecision | Application/Harness Owner | Review 只输出领域状态和只读 current projection |

## 3. V2 领域对象

所有对象都携带 `ContractVersion`、`TenantID`、`ID`、`Revision`、`Digest`、`CreatedUnixNano`、`UpdatedUnixNano`；所有 exact ref 必须是具名强类型，禁止用字符串或 V1 类型互换。

### 3.1 `HumanQuorumPolicyBindingV2`

这是 Policy Owner Fact 的 exact ref，不是 Review Policy 副本：

```text
Ref, Revision, Digest, TenantID, Domain, CheckedUnixNano, ExpiresUnixNano
```

Policy current projection必须给出并 seal：

```text
AcceptThreshold(K > 0)
MaximumPanelSize(N >= K)
RoleRequirements[] {Role, Minimum > 0}
RejectVetoRoles[]
DelegationRequired=true
ProductionSelfReviewAllowed=false
MaxPanelDuration
MaxVoteTTL
```

`RoleRequirements`与`RejectVetoRoles` canonical 排序且去重。Role minimum 总和不得大于 N；任何未知字段、过期或 terminal Policy 都 Fail Closed。

### 3.2 `HumanReviewPanelV2`

```text
Case exact ref
Target exact ref
Round exact ref
QuorumPolicy exact ref
ResponsibilitySubject exact ref
State
AssignmentRefs[]
AcceptThreshold
RoleRequirements[]
RejectVetoRoles[]
ExpiresUnixNano
```

Panel ID 由 `(Tenant, Case exact, Round exact, QuorumPolicy exact)` 稳定派生；revision 严格 `+1`。Policy 配置快照只用于证明 Panel canonical 内容，S1/S2 仍必须复读同一 Policy current ref，不能把 Panel 内副本当 current。

状态：

```text
proposed -> open -> quorum_satisfied -> deciding -> decided
                  |-> vetoed
                  |-> waiting_revision
                  |-> waiting_evidence
                  |-> waiting_higher_authority
any nonterminal -> revoked | expired | superseded | indeterminate
```

纯时间到期只让 current 校验失败；显式发布 `expired` revision 才改变 current index，历史 revision 永不重写。

### 3.3 `HumanPanelAssignmentV2`

每个 Assignment 精确绑定 Panel、Case、Round、Target、Reviewer Identity、Role Authority、Reviewer Binding、可否 veto、Lease 和 TTL。委托时还必须绑定：

```text
DelegatorIdentity
DelegateIdentity
DelegationFact exact ref
DelegatedRole
DelegationScopeDigest
```

直接授权不允许伪造空 Delegation 为“当前委托”。同一个实际人类 Identity 在一个 Panel 中最多计一票，即使其持有多个角色或多个委托；角色要求可以由该票覆盖多个已验证角色，但总票数仍只增加一。

### 3.4 `HumanAttestationV2`

Attestation 是人类原始、结构化、已验身份的判断，不是 Verdict。它精确绑定 Panel/Assignment/Case/Round/Target/Policy/ResponsibilitySubject、Reviewer Identity/Authority/Delegation/Binding、Evidence、Finding、Conditions、ObservedAt、TTL 和 IdempotencyKey。

允许的 Resolution 继续复用领域语义：Accept、Conditional Acceptance、Request Changes、Escalate Human、Reject、Insufficient Evidence。平台评论、Webhook、CLI 输入先成为 Observation；只有 identity/authority/delegation/admission root 验证成功后才能构造 Attestation。

### 3.5 `HumanQuorumDecisionV2`

Quorum Decision 是 Review Owner 对当前 Panel 的一次原子聚合：

```text
Panel exact ref
Policy exact ref
AcceptedAttestationRefs[]
OtherAttestationRefs[]
DistinctReviewerIdentityRefs[]
SatisfiedRoleCounts[]
AcceptCount
Threshold
Resolution
ConditionsDigest
EvidenceSetDigest
ReviewerSetDigest
CheckedUnixNano
ExpiresUnixNano
```

计票规则：

1. 仅当前、未过期、exact 绑定、身份不同的 Human Attestation 可计票；Auto 不计入 Human quorum。
2. `Accept`与`Conditional Acceptance`计入 accept；只要任一计入票是 Conditional，最终接受结果必须保留合并后的 Conditions，不能降为无条件 Accept。
3. 具备 current veto 权的 Assignment 产生 `Reject` 时立即形成 `vetoed`；无 veto 权的 Reject 只保留在审计集合，不增加 accept。
4. `Request Changes`立即转 `waiting_revision`；`Insufficient Evidence`立即转 `waiting_evidence`；`Escalate Human`转 `waiting_higher_authority`并要求新 Panel revision/更高角色 Policy，不复用旧票静默放行。
5. 只有 `AcceptCount >= K` 且每个必需 Role 的 distinct-current count 达标才 `quorum_satisfied`。
6. 同一 IdempotencyKey 同 canonical Attestation 幂等；同 key 换内容、同 Reviewer 对同 Panel 投不同内容、迟到票、重复平台事件均 Conflict 或仅历史 Observation，不能改变 current。

### 3.6 `VerdictV2`

Verdict V2 不再声称只有一个 Reviewer。它绑定 `QuorumDecisionRef`、`ReviewerSetDigest`、全部计入 Attestation refs、Policy、Target、Scope、Binding closure set、Evidence set、Conditions 和最短 TTL。V1 的单一 `ReviewerID/ReviewerBinding`不得被填成 panel、群组或 synthetic identity。

Runtime 需要版本化 Review Verdict/Authorization current 投影来表达 V2 quorum basis；在该 Runtime Delta 关闭前，Human V2 Verdict 不得进入 Permit/Begin。

## 4. 原子状态机与线性化

### 4.1 Panel 创建

```text
Policy exact current S1
-> Authority/Responsibility/Binding exact current S1
-> 构造 Panel + N Assignments
-> 同一 Review Store 事务全量 validate/stage
-> create Panel history/current + Assignment history/index + Trace
-> lost reply: exact Inspect 原 Panel/Assignments/Trace，禁止重建另一 Panel
```

### 4.2 计票

```text
baseline clock
-> exact Inspect Panel current
-> S1 Policy/Identity/Authority/Delegation/Responsibility/Binding/Scope/Evidence
-> validate self-review forbidden
-> S2 same exact refs + current index unchanged
-> fresh clock / rollback / TTL
-> atomic append Attestation + Panel revision/current + QuorumDecision(optional) + Case transition(optional) + Trace
```

没有达到终止条件时，Case 保持 `reviewing`，Panel revision推进并记录票；不能用 Case 的单一 `CurrentAssignment` 表达 V2 Panel。达到等待、veto 或 quorum 条件时，Case 与 Panel 必须在同一 Review Store transaction 全有全无地推进。禁止先写 Attestation 再在事务外更新 Case。

### 4.3 Verdict

Verdict Owner 在一个 Review-owned snapshot 中 exact 读取 Target/Case/Round/Panel/Assignments/Attestations/QuorumDecision/Findings，然后对所有外部 Owner 做 S1 -> same exact Ref S2。CAS 必须原子写 `VerdictV2 + Case resolved + Panel decided + Trace`；任何漂移或到期 zero write。

## 5. Currentness 与最短 TTL

Verdict/Quorum 的 expiry 必须精确取以下全部输入的最短正 TTL：Target、Case、Round、Panel、全部计入 Assignment lease/expiry、全部计入 Attestation、Quorum Policy、Responsibility Subject、每个 Reviewer Identity、Role Authority、Delegation、Reviewer Binding closure、Scope 与每项 Evidence。Reader 不得在每次读取时重封 Checked/Digest；Owner 状态改变时创建新 immutable projection revision。

每个 Reader 方法前后取 fresh clock；`now < baseline`、`now < projection.Checked`、S1/S2 crossing expiry、clock rollback 均 Fail Closed。UnknownOutcome 只 exact Inspect 原 read/mutation coordinate；无 expected ref 的 Resolve 丢回包只能开启全新 S1，不宣称恢复原结果。

## 6. Closed errors

| Category | 典型原因 | 处理 |
|---|---|---|
| InvalidArgument | shape/canonical/ref不完整 | zero write |
| NotFound | exact ID/revision确实不存在 | mutation不自动重投；Resolve可开启新S1 |
| Conflict | digest/revision/role/identity/type-pun/ABA | zero write |
| PreconditionFailed | TTL、current、quorum、self-review、delegation、veto | zero write或进入明确等待态 |
| Forbidden | Reviewer无当前Authority/Delegation、生产自审 | zero write |
| Indeterminate | ctx取消、deadline、lost reply、未知后端结果 | exact Inspect原对象/attempt |
| Unavailable | 已知后端不可用 | Fail Closed；不降级单签 |

## 7. Port Delta

1. **Policy Owner**：`HumanQuorumPolicyCurrentReaderV2`，提供上述完整 sealed Policy projection、history/current index和closed errors。
2. **Authority/Organization Owner**：role-aware Identity/Authority/Delegation current Reader；同一 snapshot 返回 Delegator、Delegate、Role、Scope、撤销与TTL。
3. **Target/Organization Owner**：`ReviewResponsibilitySubjectCurrentReaderV2`，无损证明作者/发起者 exact identity；无 Reader 时生产禁自审检查不能降级。
4. **Binding Owner**：对 Panel 每个 Assignment 复用 authoritative Review Binding current，不能用 Provider nominal type-pun。
5. **Runtime Owner**：版本化 `OperationReviewCurrentReaderV5/AuthorizationV5`，表达 VerdictV2 quorum basis；不改变 V4 单签历史语义。
6. **Application/Harness**：只消费 V5 current authorization/PhaseDecision，不解析票数、不自己聚合 quorum。

## 8. API/SDK/CLI

- 新增 Panel inspect、Assignment list/claim、Attestation submit、Quorum inspect/watch；保留 Submit/Get/List/Watch/Cancel。
- `approve/deny/request-changes` 必须携带 exact Panel+Assignment，CLI 不生成 Authority/Delegation。
- API 不提供 direct `WriteQuorumDecision`或`WriteVerdict`；两者仅由 Owner worker形成。
- Slack/Linear/Jira reaction/comment 仍只形成 Observation；同一外部 actor 的重复事件不能增加票数。

## 9. 最小反例矩阵

- K=2 但同一 Identity 两个 Assignment；必需角色缺失；N/K非法；role duplicate；无 Policy。
- veto Reviewer Reject；非 veto Reject；Reject 与最后一张 Accept 并发；64人同时投票只有一个 current revision。
- Delegation撤销/跨租户/跨scope/过期/换Delegate；同一Delegate代多个Delegator只计一票。
- Reviewer等于ResponsibilitySubject；责任Reader缺失；作者identity漂移；测试环境允许值不得污染production。
- S1/S2 Policy、Identity、Authority、Delegation、Binding、Scope、单个Evidence漂移或TTL crossing。
- Attestation lost reply exact Inspect；Panel CAS lost reply；Quorum/Verdict CAS lost reply；generic NotFound不得授重投。
- staged failure在Attestation/Panel/Quorum/Case/Trace任一点前零历史/current泄漏。
- 旧Panel历史在新revision、veto、过期、supersede后仍exact可读但不能当current。
- V1单签不得投影成V2 quorum；synthetic group ReviewerID/type-pun Fail Closed。

## 10. 发布结论

- Review-owned V2设计在用户已确认语义范围内可实施。
- 在 Policy、Authority/Delegation、Responsibility Subject 与 Runtime V5 Port Delta 未关闭前，Human multi-sign production path保持 **NO-GO**；不得用Review私有类型伪造外部Owner current。
- 单机 SQLite State Plane 是用户选择的 v1 production backend，明确无 HA/SLA；schema/transaction必须支持上述原子复合写，未来迁移保持公开Port不变。
