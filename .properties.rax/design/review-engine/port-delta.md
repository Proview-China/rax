# Review Engine 结构化 Port Delta

## 1. 使用原则

- Runtime P0.1-P0.6、Application Coordinator 与 Operation V3 已闭合；Harness 已有 Governed 运行合同，但公共装配仍有下文所列联合依赖。以下 Delta 只描述 Review 定稿相对 live schema 的增量，不把旧 P0 缺口当总阻塞。
- Delta 必须由相应 Owner 串行评审合入；Review 组件不得私建兼容接口、复制共享类型或绕过装配。
- 本设计不需要 pre-run 权威 Evidence，因此**不提出 pre-run Evidence Delta**。

## REV-D1：通用 Review Target

| 项 | 内容 |
|---|---|
| 用例 | 审核 Intent、Action、Effect、Artifact、WorkState、Outcome、Result Bundle，而不把非 Effect 对象伪装成 EffectIntent |
| 当前缺口 | `ReviewCandidateV2` 强制 IntentID、RunID、Provider、ExecutionScope，天然面向 Effect/Run |
| 语义 Owner | Review Target 语义由 Review Owner；具体 Artifact/Outcome 内容仍由原领域 Owner |
| 输入 | TargetKind、TargetID/Revision/Digest、Payload Schema/Digest/Revision、SourceRun/ReviewRun、Scope、Policy、Authority、Evidence、ContextFrame |
| 输出 | canonical `ReviewTargetRef` / Review Candidate；Effect Target 可无损映射到现有 `ReviewCandidateV2` |
| 不变量 | Target immutable；任一 Revision/Digest/Scope/Policy/Evidence 漂移产生新 Target/Case；不复制原领域 payload |
| Effect/Recovery | 创建 Target 本身是 Fact Candidate；外部物化按对应 Effect 治理；create reply 丢失按 Target ID/Digest Inspect |
| 反例 | 用虚构 Effect 包装 PPT/报告；使用“当前工作区”动态指针；Target 改后沿用旧 Verdict |
| 兼容影响 | 加法版本；现有 Effect Review 保持 V2；Runtime Adapter 需联合批准后发布 |

## REV-D2：Route、Delivery、Profile/Rubric 正交化

| 项 | 内容 |
|---|---|
| 用例 | 同一 Human/Auto Reviewer 可 Inline 或 Detached；Bypass 独立表达；Rubric 不与拓扑混合 |
| 当前缺口 | `ReviewInvocationModeV2` 只有 human/automatic_local/automatic_remote，混合 Route 与 Locality，缺 Delivery/Profile |
| 语义 Owner | Review Policy Decision 的执行语义属于 Review；五档矩阵与 Policy 发布 Owner 待管理线裁决 |
| 输入 | ReviewRoute、ReviewDelivery、ReviewerLocality、ReviewProfileRef、RubricRef、OutputSchemaRef |
| 输出 | 路由后的 Assignment/BypassDecision 与装配要求 |
| 不变量 | Human/Auto/Bypass 与 Inline/Detached 正交；五档 Profile 不是五套 Engine；Locality 不扩张 Authority |
| Effect/Recovery | Delivery/Locality 决定是否需要外部 Effect；未知 delivery/invocation 只 Inspect |
| 反例 | `automatic_remote` 同时隐含异步、权限和 Rubric；用 yolo 名称跳过其他治理 |
| 兼容影响 | 保留 V2 InvocationMode 作为 Effect Review 投影；丰富语义进入新版本/组件合同 |

## REV-D3：Bypass / OperationNotRequired 独立事实

| 项 | 内容 |
|---|---|
| 用例 | Policy 明确不需要阻塞式 Review，但必须审计且不得伪造 Reviewer 接受 |
| 当前缺口 | `ReviewVerdictFactV2.Validate` 要求 `policy_not_required => accepted`，与定稿语义冲突；dispatch 投影才转为 operation_not_required |
| 语义 Owner | Review Owner 记录 Bypass Decision；Policy Owner 决定是否 not-required |
| 输入 | exact Target、PolicyDecisionRef/Digest/Revision、Scope、Authority、Risk、Profile、TTL |
| 输出 | 独立 `OperationNotRequired` 当前事实/投影；无 Reviewer Attestation |
| 不变量 | 不等于 accepted；不绕过 Authority/Budget/Fence/Scope/Sandbox/Evidence；Policy/Target 漂移立即失效 |
| Effect/Recovery | 纯 Fact/CAS；回包丢失 Inspect exact decision；无 Reviewer invocation Effect |
| 反例 | 创建虚假 human accepted；把 not-required 当全局 Permit |
| 兼容影响 | 需要 Runtime Owner 裁决 vNext 表达；旧 V2 只能作为明确标注的过渡投影，不能污染外部 SDK 语义 |

## REV-D4：完整 Attestation、Finding、Trace 与 Reviewer Identity

| 项 | 内容 |
|---|---|
| 用例 | 支持重复 Webhook、并发 Reviewer、迟到响应、Human Accountability、高信号 Finding 和审计 |
| 当前缺口 | `ReviewAttestationObservationV2` 无 ID/Revision/Digest/幂等键、Reviewer Identity、Reason、Finding、TTL；FactPort 无 Attestation create/inspect |
| 语义 Owner | Review Verdict Owner |
| 输入 | Case/Round/Target、Reviewer Identity/Authority/Binding、Resolution、Reason/Finding/Condition、Evidence、Source sequence、TTL、idempotency key |
| 输出 | create-once Attestation Fact、Finding refs、Trace refs；Verdict 只引用其 digest/evidence |
| 不变量 | Observation 不自动升级；同 key 同 digest 幂等，换内容冲突；Authority/Binding/Lease 当前前才可 Decide |
| Effect/Recovery | 外部响应摄取先 Observation；Resolve 丢回包 Inspect Attestation/Case/Verdict；不盲重复 Decide |
| 反例 | Slack 评论“同意”直接成为 Verdict；自由文本 condition 直接授权 |
| 兼容影响 | Review-owned 新 Port；Runtime 只消费最终 current projection，不承载 SDK 工作流 |

## REV-D5：领域 Resolution 与流程 Decision 分离

| 项 | 内容 |
|---|---|
| 用例 | 表达 Accept、Request Changes、Escalate Human、Reject、Insufficient Evidence、Conditional，同时保持 Gate allow/deny/ask/defer 独立 |
| 当前缺口 | V2 Verdict 只有 accepted/rejected/conditional/expired/revoked；缺非授权领域结果和 richer Case 状态 |
| 语义 Owner | Review Owner；Gate 投影由 Application/Harness 接线消费 |
| 输入 | Attestation Resolution、Policy、Round/Case state、Reason、Findings、Conditions |
| 输出 | Review Resolution Fact、Case transition、PhaseDecision projection |
| 不变量 | EscalateHuman/InsufficientEvidence/RequestChanges 不授权执行；RequestChanges 不直接修改 Target；deny 不被低权限 allow 覆盖 |
| Effect/Recovery | 状态 CAS；丢回包 Inspect；Case terminal/superseded 迁移保持历史不可变 |
| 反例 | 把 insufficient evidence 映射 accepted；Reviewer 与工作 Agent 无限聊天 |
| 兼容影响 | 加法状态/投影；具体 Case 终态映射需管理线裁决后冻结 |

## REV-D6：Reviewer Invocation、Assignment 与 UnknownOutcome

| 项 | 内容 |
|---|---|
| 用例 | Human/Auto/Remote、Inline/Detached 均可分配、领取、调用、终止和恢复 |
| 当前缺口 | V2 只要求 automatic 绑定 InvocationEffect；缺 Assignment/Lease、ContextFrame、fork lineage、终止预算；外部 Human Delivery/轮询 Effect 未表达 |
| 语义 Owner | Review Owner 拥有 Assignment/Round；Runtime Effect/Settlement Owner 仍拥有调用治理事实 |
| 输入 | Case/Round、Reviewer Identity/Authority/Binding、RouteID、ContextFrame、Rubric、只读 Capability、Budget、TTL、attempt id |
| 输出 | Assignment/Lease、Invocation Receipt/Observation/Settlement refs、Attestation |
| 不变量 | 自动调用必须 exact settled 后才能用于 Decide；Human Lease 不扩张 Authority；Begin 后只 Inspect 原 attempt |
| Effect/Recovery | reviewer-invocation、readonly-exploration、external-delivery、attachment-fetch、remote-cancel、inspection；Unknown 全部 Inspect-only |
| 反例 | 模型 completed 直接成为 Verdict；超时后新建第二 Reviewer attempt；从 Raw Provider event 推断成功 |
| 兼容影响 | 复用 Operation V3；不导入 Model Invoker internal；Context Reference 不闭合 Route Fail Closed/Residual |

## REV-D7：Operation Review Current Projection V4（Wave 1 已实现只读 adapter）

| 项 | 内容 |
|---|---|
| 用例 | Runtime 通过 `OperationReviewCurrentReaderV4` 从 Review 当前事实重建只读 current projection，供后续治理链独立消费 |
| 当前状态 | `ExecutionRuntime/review/runtimeadapter.ReaderV4` 已实现 exact Target/Case/Verdict/Decision chain、Policy/Authority/Scope/Binding/Evidence/TTL 验证；不创建 Runtime Authorization Fact、Permit、Begin 或 Provider 权限 |
| 语义 Owner | Review Owner 提供领域 current facts；Runtime Owner 保持后续 Authorization/Permit/Begin 语义；Review 仍只判定 |
| 输入 | exact `OperationEffectIntentV3` 与单次线性化 `CurrentFactSnapshotV4` |
| 输出 | sealed `OperationReviewCurrentProjectionV4`；basis 仅允许 accepted 或 conditional_satisfied |
| 不变量 | Case 必须是 `Verdict.CaseRevision+1` 的原子 resolved revision；绑定 exact Target/Intent/Payload/Scope/Actor+Reviewer Authority/Policy/Evidence/TTL；rejected/expired/revoked/superseded/pending/drift Fail Closed |
| Effect/Recovery | 纯 exact Inspect；lost reply 仅以 `context.WithoutCancel` 重读同一 request，随后 fresh clock 再验 TTL/rollback |
| 反例 | accepted Verdict 伪造 operation_not_required；Attestation 直接授权；Inspect/retry 跨 TTL 仍返回 current；Harness boolean/fixture 授权 |
| 兼容影响 | 加法只读 adapter；不改 Runtime public contract，不改变 legacy ReviewPort；生产 current source 仍受 REV-D11 阻塞 |

## REV-D8：Application Coordinator 持久 Review Gate

| 项 | 内容 |
|---|---|
| 用例 | `action.review`、`subagent.completion.validate`、`run.completion.validate` 支持 Inline/Detached 持久等待与恢复 |
| 当前缺口 | Harness Session 不拥有 Review 状态；`PhaseDecision`/waiting 投影尚需统一装配接线 |
| 语义 Owner | Application Coordinator 拥有跨域编排；Review Owner 拥有 Case/Verdict；Harness 只运行编译后的 Gate |
| 输入 | Harness 已定义的 namespaced PhasePointRef、PhaseContext、frozen Target、Review Port binding |
| 输出 | PhaseDecision allow/deny/ask/defer、PhaseReceipt、持久 wait/resume ref |
| 不变量 | 不新增 Harness `waiting_review`；恢复先 Inspect Review currentness；Target 漂移重新 Review；PhaseReceipt 不是 Verdict |
| Effect/Recovery | 等待为持久编排事实；通知/Reviewer 调用另走 Effect；恢复只 Inspect 原 Case/attempt |
| 反例 | 各组件发明自己的 Hook enum；异步 Handler 在动作执行后撤销；Review 直接改 Harness Session |
| 兼容影响 | 依赖 Harness 接线统一 Assembly SDK/CompiledGraph/Slot/Phase 合并；Review 只提交 Contribution |

## REV-D9：Behavior Feedback Candidate 与后续应用

| 项 | 内容 |
|---|---|
| 用例 | 将 Review Trace/Finding/后验结果转成可审计行为反馈候选，并在未来影响路由/Profile/Context |
| 当前缺口 | 无正式 subject key、Admission、Owner、TTL、撤销/申诉、跨租户隔离和应用合同 |
| 语义 Owner | Review 只拥有 Candidate；正式资产与应用 Owner 需 Management/Organization/Policy/Context 联合裁决 |
| 输入 | Subject、Case/Target/Outcome、Finding、Evidence、Policy、Route/Rubric/Model binding、时间与局限 |
| 输出 | `BehaviorFeedbackCandidate`；未来正式 Asset/Policy input 另有 Admission/CAS |
| 不变量 | Candidate 不自行改变 Authority、Route、Profile 或 Context；禁止跨租户聚合和自强化无证据评分 |
| Effect/Recovery | Candidate 写入是领域 Fact；远程分析按 Effect；正式 Commit 由最终 Owner 决定 |
| 反例 | 一次 Auto Review 拒绝直接降低模型全局信任；Review 自己写 Memory/Knowledge/Policy |
| 兼容影响 | 当前仅设计 Candidate；正式 Port 等管理线裁决，不阻塞 Review 核心 Case/Verdict 评审 |

## REV-D10：Evidence 精确引用

| 项 | 内容 |
|---|---|
| 用例 | Verdict/Attestation 能审计 source identity/epoch/sequence、ledger scope/sequence 与 trust classification |
| 当前缺口 | `ReviewEvidenceRefV2` 只有 ref/classification/digest，不能从类型上证明 Evidence Ledger 坐标与 Owner Fact provenance |
| 语义 Owner | Evidence Owner 拥有 ledger；Review 只引用 |
| 输入 | `EvidenceRecordRefV2`、record digest、classification/trust、owner fact ref（适用时） |
| 输出 | canonical Review Evidence binding |
| 不变量 | Observation/Attestation/Owner Fact 不互相升级；tombstone/expiry/currentness可检查；同 source sequence 换内容冲突 |
| Effect/Recovery | Evidence append/inspect 复用 Evidence V2；reply-loss 按 exact source key Inspect |
| 反例 | 自由字符串 evidence ref；截图未绑定 Artifact Revision；Provider usage 当权威预算事实 |
| 兼容影响 | 加法引用；现有 ReviewEvidenceRef 可作兼容显示层，正式 Verdict 应使用可验证 ledger ref |

## REV-D11：Verdict Decide 的生产 external current Reader

**Review 侧状态：FROZEN FOR JOINT REVIEW；production Port/root：OPEN / NO-GO。**完整 Review 侧合同见 `rev-d11-external-current-reader-v1.md`，conformance/benchmark 设计见 `rev-d11-external-current-reader-v1-test-matrix.md`。

| 项 | 内容 |
|---|---|
| 用例 | Verdict Owner 在 Decide/CAS 前，不信 caller current，而是从 Policy、Authority、Scope、Binding、Evidence 各 Owner 的公开事实形成一个可重验、最短 TTL 的只读 snapshot |
| 当前缺口 | Review 已定义 `DecisionExternalCurrentReaderV1` seam 与reference/test reader，但live `ReviewerBindingCurrentV1`/`DecisionEvidenceCurrentV1`只有Current+Expires，缺Owner-sealed Checked/ProjectionDigest；公共合同也未闭合`ReviewComponentBindingRefV2` exact current Inspect及`ReviewEvidenceRefV2`到authoritative `EvidenceOwnerFactRefV2` applicability/current无损映射。Review不得复制共享类型、补签Owner digest或用fake冒充生产事实 |
| 语义 Owner | Binding Owner 与 Evidence Owner 分别拥有自身 current facts；Policy/Authority/Scope 仍由各公开 Owner；Review Owner 只聚合 exact read model并决定 Verdict |
| 输入 | exact stored Target、Assignment、Attestation与去重后的`ReviewEvidenceRefV2`集；Review从中派生每个Owner的full ExactSource+ExactSubject，S1据此查询Owner线性化current index取得完整ProjectionRef；caller/stored facts不得预带ref，禁止by-name/latest；时点只来自注入的fresh monotonic clock |
| 输出 | 只读 `DecisionExternalCurrentProjectionV1`：current Policy、ActorAuthority、ReviewerAuthority、Scope、Binding、逐 Evidence OwnerFact/current TTL；不得输出授权或新 Fact |
| 不变量 | Owner current projection只在状态变更时create-once/sealed且current index CAS单调前进；S1按full ExactSource+ExactSubject线性化resolve完整ProjectionRef并exact Inspect；S2只按S1保存的Ref复读并验证index未漂移，不重新resolve；ProjectionID/Revision、exact Ref/Subject、Checked/Expires、ProjectionDigest与payload固定；snapshot TTL精确等于全部输入最短正TTL；无Reader、NotFound、Unknown、漂移、过期、clock rollback均Fail Closed |
| Effect/Recovery | 只读 exact Inspect；lost reply 至多一次重读同一 canonical request，使用不受原ctx取消影响且受宿主批准边界约束的恢复context，不调用任何 mutation；恢复前后fresh clock，无法证明S1/S2一致时返回closed Indeterminate/Unavailable，不缓存旧 current |
| 反例 | caller 提交 `Current=true`；用 `internal/testkit.ExternalCurrentReader` 进生产；把 ReviewEvidence digest 当 Evidence Owner Fact；一个 Owner 过期后仍取其他 Owner 的较长 TTL；Unknown 时生成默认 current |
| 兼容影响 | 版本化加法 Port；不修改 Runtime/Harness/Application；在 Owner 公共合同与生产 composition root 联合冻结前，Review Verdict production path 保持 unsupported，reference/conformance 可继续使用 test reader |

### REV-D11 最小公共合同裁决点

1. **Binding Owner Delta**：live `ProviderBindingCurrentProjectionV2`虽有sealed digest但只接受`ProviderBindingRefV2`，不能type-pun为Reviewer Binding；live Review `ReviewerBindingCurrentV1`又缺ProjectionID/Revision、Checked和ProjectionDigest。需冻结以full `ReviewComponentBindingRefV2`+Reviewer/Assignment/Target exact subject为输入的immutable current projection、subject-current index、historical exact Inspect与`ValidateCurrent`；
2. **Evidence Owner Delta**：live `EvidenceOwnerFactCurrentV2`/Review `DecisionEvidenceCurrentV1`缺ProjectionID/Revision、Checked、ProjectionDigest，Reader只按弱Fact ID读取且没有full `ReviewEvidenceRefV2` applicability subject。需冻结full ReviewEvidenceRef+Target/Scope subject到authoritative full OwnerFact的immutable projection、current index、exact Inspect与`ValidateCurrent`；
3. **Policy Owner Delta**：live `ReviewPolicyFactV2`虽有Fact Digest/Revision/Expires和`ValidateCurrent`，但`ReviewPolicyFactReaderV2`只按string ref读取，没有Owner current projection identity、Checked、ProjectionDigest、Target/Run/Scope expected subject与current-index exact校验。需冻结immutable Policy current projection；不得把Fact Digest冒充ProjectionDigest；
4. **Authority Owner Delta**：live `OperationGovernanceFactRefV3`只是ref，`OperationGovernanceCurrentReaderV3`返回subject-wide snapshot，不提供Actor/Reviewer各自的immutable exact projection、Checked/ProjectionDigest与Role+Target+Assignment/Reviewer expected subject。需分别冻结Actor/Reviewer role-aware current projection；不得从同一snapshot位置或字符串ref反推；
5. **Scope Owner Delta**：live governance snapshot没有独立Scope exact projection identity、Checked/ProjectionDigest或Target/Run/Scope expected subject current-index合同。需冻结Scope Owner immutable current projection与`ValidateCurrent`；
6. **所有Owner共同规则**：必须提供以各自nominal full ExactSource+ExactSubject为key的线性化current-index resolve，返回完整ProjectionRef；状态变化create-once新projection并CAS index单调前进，旧Ref不得重新成为current；S1 resolve一次，S2只按已保存Ref exact复读并验证index；旧exact projection历史不可覆盖；Checked/Expires/ProjectionDigest固定；fresh now只ValidateCurrent，不重封；
7. **组合Owner**：注入上述Reader capability；Review侧已冻结S1全读+S2同exact projection复读的一致cut，不得持有其他Owner写口；
8. **生产装配**：不注入fake、Store mutation或全局mutable registry，并另行获得production Adapter/root授权。

### REV-D11 Binding 第三候选：唯一公共窄 Authoritative Current Reader

Binding Owner Delta 的可实施候选已单独冻结在 [`rev-d11-binding-authoritative-current-port-delta-v1.md`](rev-d11-binding-authoritative-current-port-delta-v1.md)。该候选把公共面收敛为一个 Runtime Binding Owner nominal Reader interface；Raw `BindingSetFactV2`、`BindingFactV2`与full Grant snapshot保持Owner内部。`ResolveCurrent`与`InspectCurrent`每次都必须在同一Owner snapshot复读当前BindingSet、全部member BindingFact及Set/Fact两侧完整Grant，并与projection index/history/highestRevision交叉验证；仅验证projection index不合格。

第四候选补齐Reader+Owner-only首建/CAS Publisher、atomic receipt/history/highest/current闭包、bound host consumer association与consumer Binding/Capability S1/S2复读，并把二者纳入true min TTL；historical exact与坏current解耦，纯时间到期不发新revision。具名canonical inputs/literal golden及BIND-01..28均在主候选。候选审计轴作为历史保留；当前双轴真值是：**Review owner-local aggregate最终独立复审YES（P0/P1/P2=0）；真实Binding production Adapter/certification与external production root仍OPEN/NO-GO**。

## REV-D12：Condition exact-set admissibility current Reader

完整候选、S1/S2和测试oracle见[`condition-v2.md`](condition-v2.md)与[`condition-v2-test-matrix.md`](condition-v2-test-matrix.md)。

| 项 | 内容 |
|---|---|
| 用例 | Auto、单人Human、企业多签都把完整`[]ReviewConditionV2`无损带入Attestation/Verdict；Verdict Owner在CAS前证明每个Condition被当前Policy允许，SatisfactionOwner Binding及Condition Authority当前 |
| 当前缺口 | live已有`ReviewConditionV2`、Policy V2 current、Review Binding V1 current和Dispatch Authority V3 current，但`ReviewPolicyFactV2`不表达“允许哪些Condition Schema/SatisfactionOwner/Capability/Authority tuple”；Review不能从Active布尔或名字猜测 |
| 唯一Owner | 公共Port/neutral类型由Runtime public ports Owner冻结；Condition tuple admissibility语义由Policy Owner；Binding/Authority仍各自拥有current facts；trusted host adapter聚合；Review只消费 |
| 输入 | exact Tenant/Case/Round/Target/Assignment、Policy、ActorAuthority、Scope/CurrentScope/ActionScopeDigest、canonical完整Conditions与`DigestReviewConditionsV2` |
| 输出 | sealed只读`ReviewConditionAdmissibilityCurrentProjectionV2`：current Policy、逐Condition完整sealed Policy tuple-decision current projection（Ref/Subject/Policy/Allowed/State/Current/Checked/Expires/Digest）、Binding current、Authority current、Checked、true min Expires、ProjectionDigest；不输出Satisfaction/Authority/Permit |
| 不变量 | exact set按ID严格递增且无重复；每项ScopeDigest等于Target ActionScope；S1先Resolve并保存Policy decision/Binding/Authority全部full refs，S2只按原refs复读并校验各current index；Policy decision/Binding/Authority逐字段一致；Item TTL取Condition+PolicyDecision+Binding+Authority最短，aggregate再取Policy+全部Item最短；success仅表示全部admissible |
| Effect/Recovery | 只读；lost reply最多一次同canonical request的detached fresh retry，返回新current snapshot而不声称恢复旧结果；clock rollback、TTL crossing、Unknown、Unavailable整批Fail Closed；绝不重调Provider/Decide |
| closed errors | InvalidArgument、NotFound、Conflict、Forbidden、PreconditionFailed、Indeterminate、Unavailable；禁止Unknown降NotFound、Policy deny软成功或空Condition授权 |
| 反例 | digest-only Conditional进入Runtime授权；Policy active即允许任意Condition；ReviewerBinding冒充SatisfactionOwner；Actor/Reviewer role projection type-pun Condition Authority；S1/S2追随新ref；Provider自报成为Satisfaction |
| 兼容影响 | V1对象只增加`Conditions,omitempty`，旧无Condition JSON/digest可读；legacy digest-only仅historical/低层compat且零production授权；Runtime Owner未合入Reader和root前production Conditional NO-GO |

## REV-D13：Result Bundle Current Grounding V2

完整候选与测试oracle见[`result-bundle-current-grounding-v2.md`](result-bundle-current-grounding-v2.md)和[`result-bundle-current-grounding-v2-test-matrix.md`](result-bundle-current-grounding-v2-test-matrix.md)。

| 项 | 内容 |
|---|---|
| 用例 | 结果Review把每个Claim绑定到typed Artifact exact revision+Anchor、Environment、Reviewer Context、Validation Scope与full Evidence，并在Verdict实际点证明全部current |
| 当前缺口 | V1只有generic `ExactResourceRefV1`、string Anchor、Environment/Scope digest；Sandbox/Continuity对象均为领域nominal且不得type-pun。Context/Evidence已有live exact-current Reader，Artifact/Environment/Validation Scope没有统一Review applicability Port |
| 唯一Owner | Review拥有Bundle/Claim/aggregate；Artifact Owner拥有body/revision/locator语义；Environment、Validation Scope、Context、Evidence各自拥有current；host composition拥有typed router/root |
| 输入 | exact stored Request/Target/Bundle/Run/Scope；Context Owner exact OriginalIntent/AcceptanceCriteria sources；typed Artifact source+anchors、Environment、Context Envelope/source、Validation Scope、逐Claim full Evidence refs |
| 输出 | 只读`ResultBundleCurrentGroundingProjectionV2`；不输出Evidence、Authority、Permit、Commit或Runtime Outcome |
| 不变量 | 三类具名ProjectionIdentityInput冻结stable ID；Validation Scope Owner-neutral identity+唯一Owner association；external projection与PublishReceipt在Owner事务immutable create-once/history/highest/current full-ref原子CAS；S1 resolve+exact Inspect，S2 same ref+index unchanged；每个source Owner Binding同样S1/S2；typed router返回含Declaration/full Owner、RouteRef、ReaderBindingRef/adapter digest与typed Reader的nominal sealed resolved-route，独立required catalog；aggregate完整封入association projection、全部resolved-route Proof与Owner Binding closure；Bundle只允许create-once atomic append/exact history，无current index或replacement CAS；TTL取全部输入、association及Owner Binding最短值 |
| Effect/Recovery | external纯读；lost S1开始新cut，lost S2只Inspect same ref；`0<ReadRecoveryTimeout<=2s`且按cut TTL/deadline裁剪；Unknown/ABA/clock rollback/TTL crossing/root missing整批Fail Closed，不重做Artifact/验证/Provider动作 |
| 反例 | Continuity relation当Artifact；Sandbox snapshot当generic Artifact；V1 digest升级V2；same Validation Scope source双Owner；aggregate漏association；stable identity字段/tag漂移；stale locator；cross-owner/tenant；required route整项缺失；只返回Reader/Route ID、ReaderBinding或adapter digest漂移；Reader interface identity比较；exact Request/Dependencies仍为注释占位；unknown kind走default；Reader缺失时fixture接管 |
| 兼容影响 | V1只historical/显示、零production Verdict；Runtime public ports Owner需冻结三组nominal Reader+Owner-only Publisher，source adapters由各Owner实现，Review不得私建production兼容接口 |

## 2. 公共装配依赖（非 Review 私有 Delta）

以下由 Harness 接线线统一设计/实现，Review 只声明需求：

- Agent Assembler 最终输出与 ResolvedAgentPlan；
- Assembly SDK / CompiledHarnessGraph；
- Slot/Phase 合并、排序、冲突与 Digest 规则；
- Binding V2 映射与 RuntimeProviderBinding；
- Checkpoint、Action Gateway、per-turn refresh 接线。

在这些对象未统一前，Review 可以实现独立领域合同与纯状态机，但不得发布私有 Slot/Hook/Phase、Runtime Adapter 或 Harness Adapter。
