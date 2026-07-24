# Review Engine 领域合同与状态机

## 1. 合同分层

Review 设计分为三层，禁止把三层合并成万能对象：

1. **Review 领域事实层**：Request、Target、Case、Round、Assignment、Attestation、Finding、Condition、Verdict、Trace、BehaviorFeedbackCandidate；
2. **Runtime 当前性投影层**：复用 `ReviewCandidateV2`、`ReviewCaseFactV2`、`ReviewVerdictFactV2`、`ConditionSatisfactionFactV2`、`DispatchReviewFactV2`、`OperationReviewAuthorizationV3`；
3. **Harness/Application 流程层**：复用 Harness 接线的 `review.gate` Slot、`action.review`、`subagent.completion.validate`、`run.completion.validate` Phase Point，以及 `PhaseDecision/PhaseReceipt`。Review 不定义公共 Phase 枚举。

领域事实不能直接变成 Runtime Permit；PhaseDecision 也不能替代 ReviewVerdict。

## 2. 必须复用的共享类型

未来 Go 实现必须直接复用 Runtime 公开合同，不复制以下类型：

- `core.Digest`、`core.Revision`、`core.Epoch`、`core.AgentRunID`、`core.ExecutionScope`；
- `ports.SchemaRefV2`、`ports.OpaquePayloadV2`、`ports.ProviderBindingRefV2`；
- `ports.AuthorityBindingRefV2`、`ports.ExecutionScopeBindingRefV2`；
- `ports.ReviewPolicyBindingRefV2`、`ports.ReviewComponentBindingRefV2`、`ports.ReviewEvidenceRefV2`；
- `ports.ReviewCandidateV2`、`ports.ReviewCaseFactV2`、`ports.ReviewVerdictFactV2`、`ports.ConditionSatisfactionFactV2`；
- `ports.OperationReviewBindingRefV3`、`ports.OperationReviewAuthorizationV3`；
- `ports.EvidenceRecordRefV2`、Binding V2 Manifest/Capability/Conformance 类型。

Review 领域只增加 live 共享合同没有表达的语义，并通过经联合批准的版本化 Port 投影到 Runtime/Application。

## 3. 统一字段规则

每个可持久化 Review 对象至少携带：

| 字段组 | 必要内容 | 不变量 |
|---|---|---|
| 合同身份 | ContractVersion、ID、Revision、Digest、CreatedAt、UpdatedAt | ID 不复用；Revision 单调；Digest 使用 canonical schema/version domain |
| 租户与执行 | Tenant、Identity+Epoch、Lineage+PlanDigest、Instance+Epoch、SandboxLease+Epoch（适用时）、SourceRunID | 跨租户同本地 ID 不冲突；旧 Epoch 只能成为迟到 Evidence |
| Target | TargetKind、TargetID、TargetRevision、TargetDigest、PayloadSchema/Digest/Revision | 任一漂移产生 Superseded，不沿用 Verdict |
| 治理 | Policy Ref/Digest/Revision、Actor Authority、Reviewer Authority、CurrentScope、Risk、ActionScopeDigest | 只绑定并复读；Review 不签发 Authority、Budget、Fence 或 Policy |
| Evidence | EvidenceSetDigest、精确 ledger record refs、ContextFrame Ref/Digest | Observation 不自动升级；Evidence 变更产生新 Review 输入版本 |
| 时间 | RequestedAt、Deadline、ExpiresAt | Verdict TTL 不超过 Target、Policy、Authority、Binding、Scope、Evidence 的最短当前性上界 |
| 因果 | CausationID、CorrelationID、SourceID/Epoch/Sequence、IdempotencyKey | 同源同序同内容幂等，换内容冲突 |

## 4. 核心对象

### 4.1 ReviewTargetSnapshot

Review Target 是冻结对象，不是“当前状态”的动态指针。

必需字段：

- `TargetKind`：Intent、Action、Effect、Artifact、WorkState、Outcome；
- `TargetID + TargetRevision + TargetDigest`；
- `PayloadSchema + PayloadDigest + PayloadRevision`；
- `ExecutionScope + ActionScopeDigest + SourceRunID`；
- `IntentID/IntentRevision/SubjectDigest`：Effect/Operation 类 Target 必需；
- `ArtifactRef/ArtifactRevision/ArtifactDigest`：Artifact/Outcome 类 Target 必需；
- `EvidenceSetDigest + ReviewerContextFrameRef/Digest`；
- `CurrentScope、ActorAuthority、Policy、SubjectProvider/Owner Binding`。

Runtime `ReviewCandidateV2` 当前只自然表达 Effect/Run Target。Artifact、Outcome 和通用 Detached Target 需要 Port Delta，不允许组件把它们伪装成虚构 Effect。

### 4.2 ReviewRequest

入口可以来自 Runtime Hook、Agent Action、人类、CLI、SDK 或外部系统，但必须归一为同一 Request：

- Request ID/Revision/Digest/IdempotencyKey；
- TargetSnapshot；
- Delivery：Inline/Detached；
- Requested Route 或 Route Constraint；
- ReviewProfileRef/RubricRef/OutputSchemaRef；
- Requester Identity/Authority；
- Deadline、Requested Verdict TTL、Budget Constraint；
- Attachments/Evidence refs；
- Source OperationSubject/OperationScope Ref（精确父Run或独立ReviewRun）；不得用它冒充RunStartRequirement或RunSettlementRequirement。

Request 只是候选。Admission 前不得形成权威 Evidence、Verdict 或执行资格。

`RubricRef` 在 live V1 中是 `ExactResourceRefV1{ID,Revision,Digest}`。Rubric Definition、criteria/rules、输出 Schema、只读 capability、终止 ceiling、append-only history/current 与 revoke/supersede 归 Review Owner；Policy 只选 exact ref。Admission 必须以 fresh baseline/fresh clock 做同一 exact ref 的 S1/S2，并在 Target+Case 复合 Store mutation 的锁内再次验证 current full ref/highest/history 与 `Request.Expires <= Rubric.Expires`。任何 missing、drift、revoke、expiry、clock rollback 都是 admission 零写。

### 4.3 ReviewCase

Case 是可分配、可等待、可恢复、可过期的审核实例：

- Case ID 与 Target 一一绑定；
- Target 新 Revision 默认创建新 Case，并把旧 Case 标记 Superseded；
- 一个 Case 可以拥有多个 Round，但同一 Round 只绑定一个 Target Revision；
- Case 只有一个当前 Revision，所有写入通过 expected revision + CAS；
- 当前 Verdict 与历史 Verdict 分离，旧 Fact 永不覆盖。

### 4.4 ReviewRound

Round 表达一次离散 Reviewer 判断：

- Round ID/Revision、Case Revision、Target Digest；
- Reviewer Assignment、Context Frame、Rubric、Budget；
- Finding 集、Attestation、Termination reason；
- 前一 Round/新 Candidate 的因果引用。

Reviewer 输出后实例终止。Request Changes 由工作域形成新 Candidate，不延长同一 Reviewer 自由对话。

### 4.5 ReviewerAssignment 与 Lease

- Assignment 绑定 Reviewer Identity、Authority、Binding、Route、Locality、Capability、Case/Round、TTL；
- Lease 只解决并发领取，不能扩张 Reviewer Authority；
- Lease 过期、Authority 撤销、Binding 漂移后，迟到回复仅作为历史 Observation；
- 多签/quorum 规则等待管理线裁决，v1 不预设。

### 4.6 ReviewAttestation

Attestation 是 Reviewer 的结构化 Observation，不是 Verdict。至少包含：

- Attestation ID/Revision/Digest/IdempotencyKey；
- Case/Round/Target/ContextFrame 精确绑定；
- Reviewer Identity、Authority、Binding、Source/Epoch/Sequence；
- ProposedResolution、ReasonCodes、FindingRefs、Condition proposals；
- Evidence refs/digest、ObservedAt、ExpiresAt；
- 对 Auto/Remote Reviewer：Invocation Effect、Attempt、Observation、Settlement 引用。

Auto Attestation 额外要求：只接受 Review Store 中 `State=applied` 的 exact ApplySettlement；再由 Review Owner exact 复读该 ApplySettlement 绑定的 `ReviewerInvocationResultFact`。该 Result 的 Tenant、Case、CaseRevision、Round ID/Revision/Digest、Assignment ID/Revision/Digest、Target ID/Revision/Digest、AttemptID 与 ResultDigest 必须和当前 Case、Assignment、Attestation 全部一致。`not_applied`、`failed`、缺失或任一交叉漂移都不得进入 Verdict。

重复 Webhook 或 CLI 提交只有在同幂等键、同 Digest 时幂等；换内容必须冲突。

### 4.7 ReviewFinding

Finding 必须离散、可操作、可定位、有 Evidence：

- Finding ID、Category、Priority、Target Location/Anchor；
- Claim、Impact、Evidence refs；
- Introduced/Observed Revision；
- Confidence 只作 Observation，不替代 Evidence；
- Status：open、addressed-candidate、superseded、dismissed-with-authority。

Review 不直接修复 Finding。

### 4.8 ReviewCondition 与 ConditionSatisfaction

- Conditional Verdict 必须包含 canonical Condition 集；
- 每个 Condition 绑定 Schema、ConstraintDigest、Satisfaction Owner、Authority、Scope、TTL；
- Satisfaction 由对应领域 Owner 独立 Inspect/CAS，Review 只验证精确 Proof；
- 所有 Condition 都有 current Satisfaction 时，Runtime 才能生成允许执行的投影；
- Condition 过期/撤销后旧 Permit 不复活。

直接复用 Runtime V2 Condition/Satisfaction 事实，不另建私有兼容类型。

Condition exact-set的兼容、Owner-local严格校验与production current准入详见`condition-v2.md`：Auto output、`AttestationV1`、`VerdictV1`未来只做`Conditions,omitempty`加法；历史digest-only Conditional仅允许exact historical Inspect，production Service/Owner/Adapter必须Fail Closed。Policy是否允许某个Schema/SatisfactionOwner/Capability/Authority组合不能由Review猜测，必须消费REV-D12 `ConditionAdmissibilityCurrentReaderV2`，并在Verdict CAS前完成Policy/Binding/Authority S1/S2与最短TTL校验。

### 4.8.1 ReviewResultBundle Current Grounding V2

live `ReviewResultBundleV1`只负责create-once结构与历史存储；string Anchor、`EnvironmentDigest`和`ValidationScopeDigest`不得参与production Verdict。V2必须绑定exact Request/Target、typed Artifact Owner/source revision、typed Anchor、Environment exact ref、Reviewer Context Envelope及全部source、Validation Scope exact ref和逐Claim full `ReviewEvidenceRefV2`。

Artifact Anchor定位语义归对应Artifact Owner；Review只校验canonical wrapper。OriginalIntent与AcceptanceCriteria必须引用Context Envelope中的exact instruction material sources，禁止caller digest。Context直接复用`ReviewerContextCurrentReaderV1`，Evidence直接复用`ReviewEvidenceApplicabilityCurrentReaderV1`。Artifact、Environment与Validation Scope分别由REV-D13 nominal exact-current Reader证明，并逐项复读live Review Binding current；各Owner的Publisher只在Owner control path。Continuity ArtifactRelation和Sandbox Snapshot不得type-pun为generic Artifact current。

Verdict前执行`baseline -> S1 current-index resolve -> exact Inspect -> S2 same ref/index unchanged -> fresh now -> true min TTL -> aggregate seal`。三类source使用具名stable identity；Validation Scope先按Owner-neutral source identity复读唯一Owner association；typed router绑定full Owner Binding、sealed ReaderBindingRef与独立required catalog。aggregate必须完整封入Scope association projection，以及每个source的nominal resolved-route Proof（Declaration/full Owner、RouteRef、ReaderBindingRef/adapter digest）和Owner Binding closure；Reader capability本身不进入digest。具名exact Grounding Request、只读StoredFacts Reader、完整Dependencies与Reader方法/constructor不得使用注释占位或`any`。lost-read恢复时限必须`0<timeout<=2s`并按cut TTL/deadline裁剪。TTL精确取Bundle、Request、Target、Context Envelope与全部source、全部Artifact、Environment、Validation Scope association/Scope、全部Evidence及Owner Binding的最短正值；Unknown、ABA、clock rollback、TTL crossing或缺root整批零Verdict/CAS。Bundle自身只允许create-once atomic append与exact history，无current index或replacement CAS。完整合同见`result-bundle-current-grounding-v2.md`。

### 4.9 ReviewVerdict

正式 Verdict 至少绑定：

- CaseID + CaseRevision；
- TargetID + TargetRevision + TargetDigest；
- Candidate/Intent/Payload Revision + Digest；
- Policy Ref/Digest/Revision；
- Reviewer Identity + Reviewer Authority + Binding；
- ExecutionScope + ActionScopeDigest + CurrentScope；
- Decision、ReasonCodes、FindingDigest、EvidenceDigest；
- ConditionsDigest 与 Satisfaction（适用时）；
- CreatedAt、UpdatedAt、ExpiresAt、Revision、InvalidationReason。

Review Owner 在同一一致性域内通过 Review-owned exact current Reader 独立复读以上事实，并在 Inspect 后使用 fresh clock 取 Target、Case、Round、Assignment lease/expiry、Attestation、Policy、Authority、Scope、Binding、Evidence 的最短 TTL。Provider/Human/Auto Attestation 不得直接成为 Verdict；caller supplied current 不具权威性。

Decide 必须在调用 `InspectDecisionCurrentV1` 前取得非零 baseline，在全部读取返回后再次读取 fresh now；`now < baseline` 时必须在构造 Verdict 和调用 Store CAS 前返回 ClockRegression。仅验证 now 晚于历史 facts 不能替代这一读区间回拨检查。

production external current部分按REV-D11冻结为：Owner只在状态变化时create-once/seal immutable projection并CAS subject-current index单调前进。S1唯一合法Ref来源是以stored Review facts派生的full ExactSource+ExactSubject线性化resolve Owner current index，取得完整ProjectionRef后exact Inspect；caller/stored facts预带ref及by-name/latest均禁止。S2只按S1保存的exact Ref复读并原子验证index仍指向它，不重新resolve。ProjectionID/Revision、Ref/Subject、Checked/Expires、ProjectionDigest与payload固定，historical exact Inspect恒返回同一deep clone；fresh now只ValidateCurrent。最后取全部Owner最短TTL并seal aggregate projection。任何漂移、过期、NotFound、Unknown、ABA或clock rollback整批Fail Closed。

### 4.10 ReviewTrace

Trace 是不可变审计索引，引用而不复制其他 Owner 的事实：

- Requested、Admitted、Routed、Assigned、Started、FindingObserved、AttestationRecorded、VerdictRecorded、Expired、Escalated、Superseded、Resolved；
- Source/Epoch/Sequence、Causation/Correlation、Evidence record refs；
- ContextFrame、Rubric、Model Route、Effect Attempt、Settlement、PhaseReceipt refs；
- 不保存隐藏 Chain-of-Thought。

### 4.11 BehaviorFeedbackCandidate

Review 可以从已结算的 Trace/Finding/后验结果产生行为反馈候选：

- Subject 可以是 Reviewer Route、Model Route、Agent/Profile、Rubric 或 Adapter Binding；
- 必须绑定租户、来源 Case/Target/Outcome、Evidence、Policy、时间和局限；
- Candidate 不能自行改变未来 Route、Authority、Profile 或 Context；
- 正式资产 Owner、Admission、CAS、TTL、撤销、申诉和应用规则等待管理线裁决。

## 5. 状态机

### 5.1 ReviewCase 领域状态

```text
requested
  -> admitted
  -> routed
  -> waiting_reviewer
  -> reviewing
  -> attested
  -> deciding
  -> resolved

reviewing|attested|deciding
  -> waiting_revision
  -> waiting_human
  -> waiting_evidence

waiting_revision -> superseded（旧Target）或routed（精确新Target Revision）
waiting_human -> routed
waiting_evidence -> deciding或routed

任一非终态 -> expired | revoked | superseded | cancelled | indeterminate
indeterminate -> Inspect同一事实/attempt -> 恢复原状态或fail_closed/escalated
```

Case create 的唯一公开 Store 入口是复合 `CreateTargetCaseV1{Target, Case, optional Trace}`。禁止公开 Case-only create：它无法维护 exact Target identity、Target-to-Case 唯一索引与 current Target-ID 索引，会绕过显式 supersede-and-create。非空 Requested Trace 必须与 Target+Case 在同一锁、同一线性化点一次发布；所有 history/index/Trace source-sequence 冲突都必须在首个写入前完成检查，不允许先写 Target/Case 再事务外 AppendTrace，也不以失败后删除模拟原子性。

同一 TargetID 的 `(Revision -> Digest)` history 为 append-only：已存在 revision 换 digest 必须 Conflict，旧 exact Target 永不覆盖；新 Target revision 必须严格大于全部 current/history revision。当前设计允许严格向前的 revision gap，但拒绝 rollback。

Wave 1 的 RunID 已由 Target 与 current Policy exact 绑定；本轮不增加 TurnID，也不宣称尚未设计确认的 Turn 语义。

约束：

- `requested` 仅是候选，不产生权威 Evidence；
- `admitted` 必须满足 Run Requirement、Scope、Policy、Authority、Binding、TTL；
- `resolved` 必须通过唯一Decide/CAS，并且只能承载Accept、Conditional Acceptance、Reject或精确PolicyNotRequired；Request Changes、Escalate Human、Insufficient Evidence只终止当前Round，不直接关闭Case；
- Target 漂移只能到 `superseded`，不能修改 Case 内 Target；
- `expired/revoked/superseded/cancelled` 均终止当前授权；
- 回包丢失先 Inspect，不能再次 Resolve 或 Dispatch Reviewer。

Runtime V2 当前把所有中间态投影为 `pending`，把正式决定投影为 `decided`，把失效投影为 `expired/revoked`。领域细化状态在联合批准前不得直接写入 Runtime Port。

### 5.2 Reviewer Round 状态

```text
prepared -> admitted -> dispatched_or_delivered -> observed
        -> inspected -> attested -> terminated

dispatched_or_delivered -> unknown_outcome -> inspect_original_attempt
inspect_original_attempt -> observed | still_unknown | failed_closed
```

Begin 后只 Inspect 原 attempt；不得新建相同语义的第二 attempt 规避 UnknownOutcome。

### 5.3 领域 Resolution 与 Gate Decision

| 领域 Resolution | Case/Route 行为 | Gate 投影 | 是否允许执行 |
|---|---|---|---|
| Accept | 当前 Round 结束，Owner 可 CAS accepted Verdict | allow | 仅在全部当前性门禁成立时 |
| Conditional Acceptance | CAS conditional Verdict，等待独立 Satisfaction | defer，满足后 allow | Satisfaction 当前前不允许 |
| Request Changes | 当前Round终止，Case进入waiting_revision；Target Owner产生精确新Candidate Revision后重新路由 | deny/defer | 不允许 |
| Escalate Human | Auto Round终止，Case进入waiting_human并转Human route | ask/defer | 不允许 |
| Reject | CAS rejected Verdict | deny | 不允许 |
| Insufficient Evidence | 当前Round终止，Case进入waiting_evidence；补充Evidence后重新Decide/Route，或按Policy转Human | defer/ask | 不允许 |
| Operation Not Required | 无 Reviewer Attestation，记录策略 not-required | allow 的独立依据 | 仍需其余 Runtime 门禁 |

Gate 的 `allow/deny/ask/defer` 是流程投影，不是领域 Verdict 枚举。当前 Runtime V2 缺少部分领域 Resolution，详见 Port Delta。

### 5.4 终止与熔断

- Auto Reviewer 每轮输出一次结构化结果即终止；
- 动作前 timeout、parse failure、unavailable、unknown 默认 Fail Closed；
- 纯结果观察是否允许降级，只能由显式 Policy 决定；
- max round/token/time/cost、重复 Finding、重复拒绝、循环争议达到边界后停止并升级 Human；
- 具体阈值及升级目标等待管理线裁决，不在组件中写死。

## 6. Slot / Phase Contribution 声明

Review 只引用 `tmp.document/Harness接线.md` 已定义对象：

| 公共对象 | Review 角色 | Contribution 类型 | 输入 | 输出/限制 |
|---|---|---|---|---|
| Slot `review.gate` | Owner | Port/Gate | frozen Target、Policy、Scope、Evidence refs | Case/Verdict 当前性引用；不 Dispatch |
| Phase `action.review` | 审核动作候选 | Gate | 已完成 `action.admission` 的 Candidate | allow/deny/ask/defer PhaseDecision + PhaseReceipt |
| Phase `subagent.completion.validate` | 审核子 Agent 结果 | Gate | 冻结 Result Bundle/Artifact/Evidence | 不写 Harness Completion Claim |
| Phase `run.completion.validate` | 审核最终结果 | Gate | 冻结 Outcome/Artifact/Grounding Candidate | 不写 Runtime Outcome |
| Phase `residual.detected` | Review residual 来源 | Observer | Review unknown/timeout/未结算引用 | 只报告 Observation/Residual |

Review 不声明新 SlotID、PhaseID 或合并优先级；实际 `SlotContribution`、`PhaseContribution`、顺序与 Digest 必须由 Harness Assembly Compiler 的统一对象生成。

## 7. 依赖 DAG

```text
Agent Definition/Profile/Policy
        -> Agent Assembler ResolvedAgentPlan
        -> Harness Assembly SDK / CompiledHarnessGraph
        -> Binding V2 + Runtime Activation
        -> Application Coordinator Phase/Gate调用
        -> Review Request/Case/Reviewer/Attestation/Verdict
        -> Runtime Review Current Projection
        -> Operation Governance Permit/Begin/Execution/Settlement
```

Review 的直接公共依赖：

- Runtime core/ports：Scope、Binding、Effect、Evidence、Review V2、Operation V3；
- Application Coordinator 公共 Gate/等待/恢复面；
- Harness Assembly 的 `review.gate`、PhaseContribution、PhaseContext/Decision/Receipt；
- Organization/Authority、Profile/Policy、Context Frame、Evidence、Continuity、Budget、Sandbox、Artifact 公共 Port。

公共装配硬依赖：Agent Assembler 最终输出、Assembly SDK/CompiledGraph、Slot/Phase 合并规则、Binding V2 映射、Checkpoint/Action Gateway/per-turn refresh 接线。Review 只声明贡献与需求，不私建替代品。
