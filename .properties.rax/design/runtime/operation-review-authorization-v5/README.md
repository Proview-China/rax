# Operation Review Authorization V5

## 1. 冻结范围

本增量只解决两个已经由用户确认、且 V4 无法无损表达的 Review 治理输入：

1. Human enterprise multi-sign V2 的 K-of-N、角色、veto 与 distinct reviewer quorum；
2. 独立 `operation_not_required`，它来自 current Policy + Review-owned Bypass Decision，不能伪装成 accepted Verdict。

V5 是 additive contract。V4 单 Reviewer 历史、Reader、Fact 与 Gateway 不修改、不迁移、不 type-pun。V5 仍只形成 Runtime Authorization Fact；它不是 Permit、Begin、Dispatch、Provider 调用或 Operation Settlement。

## 2. Owner 与依赖边界

| 对象 | 唯一 Owner | V5 只做什么 |
|---|---|---|
| Target/Case/Panel/Assignment/Attestation/Quorum/Verdict/BypassDecision | Review Owner | exact read-only projection |
| PolicyNotRequired 与 quorum policy 决策 | Policy Owner | exact current S1/S2；不由 Review/Runtime补签 |
| Identity/Role/Delegation/Responsibility | Organization/Authority Owner | exact current S1/S2；Runtime不复制组织事实 |
| Binding/Scope/Evidence | 各原 Owner | exact current S1/S2、最短 TTL |
| EffectIntent/Fence/Governance/Authorization Fact | Runtime Owner | current交叉验证、create-once/CAS、失效 |
| wait/resume/PhaseDecision | Application/Harness Owner | 只消费 Runtime current Authorization；不聚合票数 |

import 方向必须无 SCC：Review adapter只依赖 Review public contract + `runtime/ports`；Runtime只依赖 `runtime/core` + `runtime/ports`；Application/Harness只依赖公共 Port；`agent-host` 最后注入实现。

## 3. 公共中立合同

### 3.1 exact refs

Runtime neutral ref统一为强类型 `{TenantID, ID, Revision, Digest, ExpiresUnixNano}`。以下类型名称独立，禁止互相转换或用同一个字符串承载：

- `OperationReviewCaseRefV5`
- `OperationReviewPanelRefV5`
- `OperationReviewQuorumDecisionRefV5`
- `OperationReviewVerdictRefV5`
- `OperationReviewBypassDecisionRefV5`

这些 ref 不授予 Authority；它们只是 Owner fact 的 exact coordinate。

### 3.2 `OperationReviewQuorumCurrentProjectionV5`

字段必须包含：

```text
ContractVersion
Operation
IntentID / IntentRevision / IntentDigest
PayloadSchema / PayloadDigest / PayloadRevision
Target {Ref, Revision, Digest}
Case / Panel / QuorumDecision / Verdict exact refs
QuorumPolicy exact ref
ReviewerSetDigest
AcceptCount / Threshold
SatisfiedRoleCounts[]
ReviewerAuthorityRefs[]
BindingRefs[]
ScopeRef
DecisionEvidence[] / EvidenceDigest
Basis = accepted_quorum | conditional_quorum_satisfied
Satisfaction(optional, only conditional)
Current / CurrentnessDigest / ProjectionDigest
CheckedUnixNano / ExpiresUnixNano
```

Review runtimeadapter必须从同一 Review-owned consistent snapshot exact复读 Case、Panel、全部计入 Assignment/Attestation、QuorumDecision、Verdict、Finding/Trace；随后对 Policy、Organization/Authority、Binding、Scope、Evidence执行 S1 -> exact Ref reread -> S2。`AcceptCount >= Threshold`、每个必需Role满足、无veto、distinct identity、production禁自审必须由 Review Owner 已验证且 Runtime neutral projection可重验。Runtime不重新计票，也不接受caller给出的布尔 `quorum=true`。

### 3.3 `OperationReviewPolicyNotRequiredCurrentProjectionV5`

字段必须包含：

```text
ContractVersion
Operation
IntentID / IntentRevision / IntentDigest
PayloadSchema / PayloadDigest / PayloadRevision
Target {Ref, Revision, Digest}
Case exact ref
BypassDecision exact ref
PolicyCurrentProjection full exact ref
PolicyDecisionRef
ScopeRef / ActorAuthorityRef
Current / CurrentnessDigest / ProjectionDigest
CheckedUnixNano / ExpiresUnixNano
```

Review Owner的 BypassDecision 必须 exact 绑定 Target、Case、Policy current projection、Scope、Actor Authority、Profile、Risk、Decision=`operation_not_required` 和 TTL；不得含 Reviewer、Assignment、Attestation、Verdict 或 Satisfaction。Policy Owner S1/S2 必须证明同一 Target/Run/Scope/Policy 当前且 `OperationNotRequired=true`。Operation Binding由Runtime Governance current独立复读，不为Bypass虚构Reviewer Binding。Policy撤销、Target/Case漂移或任一TTL crossing立即失败关闭。

### 3.4 reader 与 union

```go
type OperationReviewCurrentRequestV5 struct {
    Intent OperationEffectIntentV3 `json:"intent"`
    Basis  OperationReviewAuthorizationBasisV5 `json:"basis"`
}

type OperationReviewCurrentProjectionV5 struct {
    ContractVersion string `json:"contract_version"`
    Basis OperationReviewAuthorizationBasisV5 `json:"basis"`
    Quorum *OperationReviewQuorumCurrentProjectionV5 `json:"quorum,omitempty"`
    PolicyNotRequired *OperationReviewPolicyNotRequiredCurrentProjectionV5 `json:"policy_not_required,omitempty"`
    ProjectionDigest core.Digest `json:"projection_digest"`
    ExpiresUnixNano int64 `json:"expires_unix_nano"`
}

type OperationReviewCurrentReaderV5 interface {
    InspectOperationReviewCurrentV5(context.Context, OperationReviewCurrentRequestV5) (OperationReviewCurrentProjectionV5, error)
}
```

union恰好一个分支非nil，并与Basis一致。Reader是只读接口；没有发布、CAS、Fact写口。

## 4. Runtime Authorization Fact

`OperationReviewAuthorizationFactV5`复用 V4 的 Intent binding、Runtime Governance binding、Fence、requested TTL和terminal状态语义，但其 Review 字段只能是 sealed V5 union projection。Fact expiry精确取：

```text
created + requested TTL
Intent expiry
V5 projection expiry
全部 Review/Policy/Organization/Authority/Binding/Scope/Evidence exact refs expiry
Runtime Governance snapshot expiry
Fence expiry
```

的最短正值。任何一个输入无正TTL、过期或无法exact复读都zero write。

FactPort与Gateway是 additive V5：create-once、historical exact Inspect、current Inspect、terminal CAS。相同 AuthorizationID + same canonical request幂等；相同ID换Effect/Basis/projection/digest为Conflict。

## 5. 唯一执行顺序

```text
Operation Effect Intent accepted/dispatch_intent
  -> Runtime Governance current S1
  -> Review V5 current Reader S1/S2
  -> fresh clock + rollback/TTL检查
  -> Runtime Governance current S2
  -> create-once Authorization V5
  -> lost create reply: exact Inspect同AuthorizationID，绝不重调Review mutation
  -> Host Gateway Inspect current V5 Authorization
  -> Permit -> Begin -> actual-point Review/Fence/Authority/Budget/Scope二次门禁
```

Review V5不Dispatch、不Commit、不改变Effect/Run状态。Provider边界后的 unknown 继续只Inspect原 attempt。

## 6. Currentness 与失败关闭

- 每次Gateway调用先取非零 baseline；所有Reader返回后再取 fresh now；`now < baseline`失败关闭。
- current Inspect必须重新读取原 EffectIntent、V5 projection、Runtime Governance snapshot与Fence关联；不得只信 Authorization内快照。
- quorum中的任一Assignment lease、Attestation、Policy/Role/Delegation/Responsibility/Binding/Scope/Evidence漂移都使V5失效。
- not-required中的Policy、BypassDecision、Target/Case/Scope/Binding/Actor Authority漂移都使V5失效。
- TTL crossing、ctx cancel/deadline、Unavailable、Indeterminate都不能产生Authorization。
- create reply unknown只用 `context.WithoutCancel` exact Inspect同ID；Inspect reply unknown只重读同一exact request一次，之后仍unknown。

closed categories：`InvalidArgument`、`NotFound`、`Conflict`、`PreconditionFailed`、`Forbidden`、`Indeterminate`、`Unavailable`。普通/权威 NotFound都不授新执行权；只有caller显式重新发起同canonical create request时才由create-once语义处理。

## 7. 兼容与迁移

- V4/V5 Fact、Reader、Gateway、SQLite rows与current index分开；不能把V2 group ID塞进V4 ReviewerID。
- V5可在验证当前后投影共享的非Review治理输入，但不得降级投影成V4/V3单Reviewer Authorization。
- V4继续用于已冻结的single Reviewer accepted/conditional path；V5用于enterprise quorum和独立not-required。
- Host route必须按版本显式选择，V4/V5同一Operation/Effect只能有一个current Review Authorization；shared guard由Runtime Owner线性化。
- 当前不声明HA/SLA；SQLite WAL是单节点production backend，未来迁移保持public ports不变。

## 8. 最小反例

1. V1单签伪装quorum；group ID伪装ReviewerID。
2. K满足但必需Role不足；同Identity重复计票；veto Reject仍accepted。
3. Assignment/Attestation/Delegation/Responsibility任一漂移或过期。
4. Bypass含Reviewer/Attestation/Verdict；accepted Verdict伪造not-required。
5. Policy `OperationNotRequired=false`、Policy撤销、PolicyDecisionRef漂移。
6. Target/Case/Intent/Payload/Scope/Binding/Authority/Evidence任一漂移。
7. S1/S2 ABA、TTL crossing、clock rollback。
8. lost reply后换AuthorizationID、Basis或重调mutation。
9. V4和V5并发争用同一Effect，只有一个current authorization。
10. pending/rejected/revoked/expired/superseded/unknown projection到达Permit。

## 9. 发布门

V5 Owner-local代码必须通过 unit、whitebox、blackbox、fault、conformance、SQLite restart、64并发、ordinary100、race20、full ordinary/race/vet、gofmt、diff/import扫描。只有 Review V2 current Reader、BypassDecision、Policy/Organization/Binding/Scope/Evidence production readers、Runtime SQLite V5与`agent-host` composition root全部闭合后，production Gate才可GO。
