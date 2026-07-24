# Review Engine 可评审设计

## 1. 状态与授权

- 当前阶段：**Review owner-local/reference/test 非完整生产闭环最终独立复审 YES（P0/P1/P2=0）**。
- 当前结论：**Review-owned 单机服务、Decision/Auto/Human/Bypass/Runtime只读组合、Result Grounding V2与reusable测试已完成；Praxis production integration仍 NO-GO**。真实外部 Owner adapter/certification、宿主 composition root、公共 Gate 与 Provider Effect 不在本轮完成面。
- 最高业务输入：`tmp.document/Review.md`。
- live 技术基线：Review 已实现只读 `OperationReviewCurrentReaderV4` 投影，但不创建 Runtime Authorization Fact；Harness/Application/Model Invoker 接线与生产 current source 仍是发布门禁。本设计复用现有基座，不重造 Runtime 治理链，也不私建 Harness 装配替代品。
- REV-D11固定状态：**Review-owned aggregate P0/P1/P2=0；五类外部Owner production adapter/root NO-GO**。Review已实现只读聚合与真实Reader注入边界，但不拥有或伪造外部Owner current。
- Condition V2 Review-owned实现与资产最终独立复审YES（P0/P1/P2=0）：Human/Auto Attestation、Verdict与Runtime V4/V5只读投影携带完整`[]runtimeports.ReviewConditionV2`；legacy digest-only零授权。Policy Owner exact tuple-decision Reader与宿主root未关闭前，production Conditional保持NO-GO。
- Result Bundle Current Grounding V2 owner-local实现与最终独立复审已完成（P0/P1/P2=0）：V2使用单向`Bundle -> exact Request/Target`解除摘要环，只能随Request/Target/Case复合admission；Context/Artifact/Environment/Validation Scope/Evidence/Owner Binding执行S1/S2、full Owner route、逐Owner双时钟、真实min TTL、lost-read exact恢复与deep clone。真实Owner production adapters/certification/root仍NO-GO。
- Detached Delivery V1已冻结Review-owned候选：[`detached-delivery-v1.md`](detached-delivery-v1.md)与[`detached-delivery-v1-test-matrix.md`](detached-delivery-v1-test-matrix.md)。Review只拥有immutable DeliveryBinding/Closure；Runtime Run、Application coordination、Harness Phase与Human delivery事实继续归各Owner，四组外部P0及Host root关闭前Go/production NO-GO。
- 2026-07-17 新授权只覆盖 Review 独占目录。Review 可实现自身 SQLite/API/SDK/CLI/平台协议 Adapter，但不得把平台回包直接升级为 Verdict、直接执行外部网络 Effect，或跨目录补齐五类 Owner/root。
- 设计语言：Go 为默认实现语言。当前没有经基准证明的计算稠密热点，因此不规划 Rust、FFI 或独立 Rust 进程。

## 2. 设计目标

Review Engine 是从 Intent、Action、Effect、Artifact 到 Outcome 之间的判断层。它统一承载 Human、Auto 与 Bypass 三类路由，并把 Reviewer 的原始响应转化为可追溯、可过期、可撤销、可 CAS 的正式 Verdict。

本设计必须同时满足：

1. Review 只判定，不 Dispatch、不 Commit、不修改 Runtime Outcome；
2. Human、Auto、Remote、CLI、Webhook 回包都只是 Attestation/Observation；
3. 正式 Verdict 由 Review Verdict Owner 复读当前 Target、Policy、Reviewer Identity/Authority、Scope 与 Evidence 后 CAS；
4. Verdict 精确绑定 Case、Target/Intent/Payload Revision 与 Digest、Policy、Authority、Scope、Conditions、Evidence 和 TTL；
5. Target、Policy、Authority、Scope、Evidence 或 TTL 漂移后，旧 Verdict 与 Permit 均不得复活；
6. Bypass/YOLO 是显式 `operation_not_required`，不是伪造的 accepted Verdict，也不绕过 Authority、Budget、Fence、Scope、Sandbox、Evidence 或 Effect Gateway；
7. 同步/异步只是 Delivery，Human/Auto/Bypass 是 Route，Rubric/Profile 是判断方法，四者不得混成模式枚举；
8. Auto Reviewer 是去工作身份化、只读、一次性的 Reviewer fork，不能继承工作 Agent 的写入、Dispatch、Commit、Spawn 或隐藏推理；
9. 外部系统只能作为 Adapter，不能成为 Review 事实权威；
10. UnknownOutcome 只 Inspect 原 attempt，禁止盲重派。

## 3. 唯一 Owner 与非 Owner

| 对象/动作 | 唯一 Owner | Review Engine 的关系 |
|---|---|---|
| ReviewRequest、ReviewCase、Round、Assignment、Attestation、Finding、Condition、Verdict、Trace | Review Verdict Owner | 拥有状态机、Inspect、CAS、撤销、过期、替代和审计语义 |
| Reviewer Identity、Authority、Accountability | Organization/Authority Owner | 只绑定并复读，不自行签发或延长 |
| Rubric Definition、criteria/rules、输出 Schema、只读能力、终止 ceiling、版本与撤销 | Review Owner | 持久化 append-only history/current；Policy 只选择 exact ref 与适用条件 |
| Review Profile 的业务路由规则、五档矩阵与适用条件 | Policy Owner | 只选择 Review-owned exact Rubric ref，不复制或重定义 Rubric payload |
| Run、Runtime Outcome | Runtime Owner | Review 只提供 Participant/领域事实引用，不写 Runtime 状态 |
| 持久等待/恢复与跨域编排 | Application Coordinator | Review 提供 Case/Verdict currentness，不新增 Harness 等待状态 |
| PhaseContext、PhaseDecision、PhaseReceipt 与合并 | Harness 公共接线/编译装配 Owner | Review 只贡献版本化 Gate/Port/Observer，不定义公共枚举或合并规则 |
| Operation Permit、Begin、Fence、Budget、Scope 双重门禁 | Runtime Host Governance Gateway 与实际执行点 | Review 只提供当前 Verdict 投影所需事实 |
| Harness Session、Candidate、PendingAction、Completion Claim | Harness | 只消费冻结 Target；不依赖 Harness 私有 Port 或内部实现 |
| Effect Observation、Evidence Ledger、Settlement | 各领域 Owner、Evidence Owner、Settlement Owner | Review Attestation 可引用 Evidence，但不能把 Provider 回包升级为事实 |
| Artifact、Memory、Knowledge、Context、Cache 正式 Commit | 各自领域 Owner | Review 只判断 Candidate，不 Commit |
| BehaviorFeedbackCandidate | Review Verdict Owner | 只产生带 provenance 的候选；正式资产 Admission 与应用规则待管理线裁决 |

## 4. 四个正交维度

| 维度 | v1 候选值 | 冻结语义 |
|---|---|---|
| Target | Intent、Action、Effect、Artifact、WorkState、Outcome | 必须有稳定 ID、Revision、Digest、Schema、Scope、Evidence 与来源 Run |
| Delivery | Inline、Detached | 只描述等待/交付方式，不改变 Reviewer 权限与 Verdict 语义 |
| Route | Human、Auto、Bypass | Bypass 仍需正式策略事实和审计记录 |
| Profile/Rubric | intent-safety、code-change、work-state、artifact-quality、outcome-acceptance 等 | 不使用万能 Prompt；每类有独立 Rubric、输出 Schema、工具范围与终止条件 |

`bapr`、`yolo`、`permissive`、`standard`、`restricted` 只是 Policy Router 的输入。精确 Human/Auto/Bypass 矩阵尚未冻结，不得从名称推导生产安全承诺。

## 5. Human Gate

- Human Envelope 必须显示冻结 Target、原始 Intent、影响范围、关键 Diff、风险、Evidence、限制和精确问题。
- 领取/分配使用 Reviewer Assignment + Lease；Lease 不授予超出 Organization Authority 的审核权。
- 人类通过 UI、CLI、SDK、Webhook 或外部系统提交的是 Attestation；Review Owner 验证身份、Authority、Case/Target Revision、Digest、TTL、Schema 与幂等键后才能 CAS Verdict。
- Inline Human Gate 是持久等待状态，不是阻塞线程；服务重启后继续 Inspect 同一 Case。
- Detached Human Review 可以在外部系统异步完成，但外部评论、Ticket 状态或消息反应永远不能直接成为 Verdict。

## 6. Auto Reviewer fork

- fork 输入是冻结的 `ReviewerContextFrame`：原始人类 Intent、稳定规则、已确认决策、Target/Diff、Evidence、风险、Rubric 和已知限制。
- 必须移除工作 Agent 的 persona、自我叙述、执行计划、写入权、Dispatch/Commit 权、Spawn 权和无关工具。
- 默认只读；任何模型调用、远程检索、外部读取或只读验证仍按 Effect 治理。
- Reviewer 单轮输出结构化 Attestation、Finding、Condition 或升级原因后终止；修订由工作域形成新 Candidate Revision，再根据 Policy 进入新 Round/Case。
- 终止边界必须包含最大 Round、token、时间、费用、重复 Finding、重复拒绝和循环争议；达到边界后停止并升级 Human，不能让两个 Agent 无限对话。
- Context Reference 物化不是所有 Model Route 都已闭合。Route 无法可靠物化所需 Context 时必须 Fail Closed，或在只允许观察的显式 Policy 下记录 Residual；不得宣称通用 Route 支持。
- Model 调用只通过公开 `RouteID + routegateway/execution union`，不得导入 `model-invoker/internal`、厂商 SDK 或 Raw/Native 事件。

## 7. Bypass / YOLO

Bypass 只表示当前 Review 操作经策略判定为不需要阻塞式 Reviewer：

- 必须持久化 Policy Decision、Target、Scope、Authority、Risk、Profile、原因、Revision、Digest 与 TTL；
- 不创建虚假 Reviewer Attestation；
- 不把 `operation_not_required` 伪装为 accepted Verdict；
- 后续 Host Gateway 仍独立复读 Authority、Review not-required 投影、Budget、Scope、Fence、Binding 与 Policy；
- 旧策略撤销、Target 漂移或 TTL 到期后，旧 Permit 不能继续使用。

Runtime V2 当前将 `policy_not_required` 内部编码为 accepted + basis，再投影为 `operation_not_required`。该实现与业务定稿存在语义差异，必须通过 Port Delta 联合裁决，组件不得私建兼容接口掩盖差异。

## 8. 与 live Runtime/Harness 的关系

### 已直接复用

- Runtime `ReviewCandidateV2`、`ReviewCaseFactV2`、`ReviewVerdictFactV2`、`ConditionSatisfactionFactV2`；
- `ReviewVerdictFactPortV2` 的 Create/Inspect/CAS/Decide 原子事实面；
- `ReviewGovernanceGatewayV2` 对 Effect Review 的 currentness 复读与 CAS；
- `DispatchReviewFactV2`、`OperationReviewAuthorizationV3`、Operation V3 Permit/Begin/执行点二次门禁；
- Runtime Effect/Evidence/Binding/Authority/Budget/Scope/Fence/Settlement 公开合同；
- Harness Governed Candidate、Session CAS、Reservation、Binding、Observation 与 Settlement 关联。

### 明确不做

- 不新增 Harness `waiting_review` 状态；Inline/Detached 等待属于 Application Coordinator 的持久编排投影；
- 不依赖 `harness/ports.ContextPort`、`ModelTurnPort`、`EventCandidatePort`，这些是 Harness 私有装配缝隙；
- 不依赖 Runtime foundation/fakes/kernel 内部、Harness kernel/fakes/internal 或 Model Invoker internal；
- 不把 legacy `ActionRequest.ReviewRequired` 布尔值升级为 Admission、Verdict 或 Authorization；
- 不手工构造 `OperationReviewAuthorizationV3` 作为生产授权。

## 9. pre-run Evidence 裁决

本设计**不触发 pre-run Evidence Delta**：

- Inline Review 必须绑定已存在的父 Run；
- Detached Review 在产生权威 Attestation/Evidence/Verdict 前，必须先通过现有 Runtime/Application 链建立独立 ReviewRun；
- Run 建立前可以接收未 Admission 的 ReviewRequest Candidate，但不得生成权威 Evidence、Verdict 或执行授权；
- 因此所有权威 Evidence 都发生在父 Run 或独立 ReviewRun 已进入现有 Run-scoped Evidence V2 适用域之后；本设计不声称 live Evidence 已支持无 Run 的 OperationScope 记账。

## 10. Wave 1 live 实现同步

- Auto Attestation 只有在 Review Store exact 复读到 `State=applied` 的 `DomainApplySettlementFact`，并继续 exact 复读其 `ReviewerInvocationResultFact` 后才可记录；Tenant/Case/Round/Assignment/Target/Attempt/ResultDigest 任一漂移均在 Verdict 前失败关闭。
- Verdict Owner 不接受 caller supplied current；它消费 Review-owned `DecisionCurrentReaderV1` 单次不可变 snapshot，并以 fresh clock 重验 Target/Case/Round/Assignment/Attestation/Policy/Authority/Scope/Binding/Evidence 及最短 TTL。
- Target exact identity 到 Case 使用 create-once 唯一索引；同 Target 同 Case payload replay 幂等，换 Case ID 或 payload 冲突；新 Target revision 必须先显式 supersede，历史 Case/Verdict 保留 exact Inspect。
- Target history 的 revision/digest append-only且revision严格递增；Case创建只有复合Target+Case+optional Trace入口，非空Trace同锁发布且冲突时三者零写。
- Round、Assignment、Attestation、Finding、Trace、Verdict 均绑定 Target ID/Revision/Digest；Verdict 额外绑定 ReviewerID/ReviewerBinding。
- mutation reply-loss 只 exact Inspect 原 Attestation/Case/Verdict，不重放 Decide/Invalidate；reference Store 的复合写遇到 staged failure 会回滚 current 与新增 history revision。
- `runtimeadapter.ReaderV4` 在首次 Inspect 与 lost-reply exact retry 后各取 fresh clock，并对 TTL crossing 与 clock rollback 失败关闭；`operation_not_required` 因缺少独立 current PolicyNotRequired 事实而保持 unsupported。
- Verdict Decide 同样在 Decision current Reader 前后取 baseline/fresh clock；Policy、ActorAuthority、ReviewerAuthority、Scope、Binding及每项Evidence均独立参与最短TTL，actual-point过期或读期间回拨均zero Verdict/CAS。
- production `DecisionExternalCurrentReaderV1` 尚无可用 Binding/Evidence Owner 公共 exact-current 实现；`memory` 与 `internal/testkit` 只用于 reference/conformance，不能宣称生产 Backend、SLA 或 composition root。
- Review-owned production slice 已增加 SQLite WAL `storage/sqlite`、`service`、鉴权HTTP/SSE、Go SDK/CLI、安全Router和三平台协议Adapter；Request可把Result Bundle与Target+Case+Trace原子落盘，Behavior Feedback仅以exact provenance Candidate落盘。平台Observation携带宿主提供的immutable Envelope binding，但真实凭据、outbound Provider、identity/Authority admission和跨组件root仍NO-GO。

## 11. 设计资产

- `architecture.drawio`：Owner、State Plane、Human/Auto/Bypass 与 Runtime/Harness 边界图；
- `contracts.md`：对象、版本字段、状态机、Decision、Condition 和当前性合同；
- `governance-flow.md`：Admission、Effect、Receipt、Inspect、CAS、Settlement、SDK/CLI/API 与失败恢复；
- `port-delta.md`：公共合同缺口及兼容/迁移影响；
- `rev-d11-external-current-reader-v1.md`：REV-D11 Review 侧冻结合同、Owner边界、一致性、TTL、错误与恢复；
- `rev-d11-binding-authoritative-current-port-delta-v1.md`：保留Runtime Binding Owner候选的历史审计演进；Review-owned aggregate已实现并通过最终独立复审，真实Binding production Adapter/certification与external production root仍未关闭；
- `rev-d11-external-current-reader-v1-test-matrix.md`：未来 reusable conformance 与 benchmark 设计及实现门禁；
- `rev-d11-external-owner-live-inventory.md`：五类Owner公开类型/Reader/root的live指纹、缺失exact字段、最小Delta与实现准入证据；仅为依赖盘点，全部外部项保持NO-GO；
- `production-service-v1.md`：已确认的安全Router、SQLite WAL、HTTP/SSE、SDK/CLI、Slack/Linear/Jira Adapter、Effect恢复与双轴发布合同；
- `production-service-v1-test-matrix.md`：SQLite、Router、HTTP/SDK/CLI、平台Adapter与组合门禁反例；
- `human-multisig-v2.md`：用户已冻结的企业多签 Human Route；租户 K-of-N、必需角色、veto Reject、显式 Delegation current 与 production 禁自审；
- `rubric-v1.md`：Review-owned 七类 Rubric baseline、append-only/current、Admission S1/S2、撤销/替代/TTL/lost-reply 合同；
- `rubric-v1-test-matrix.md`：Rubric unit/store/SQLite/concurrency/fault/admission reusable 验收矩阵；
- `condition-v2.md`：exact Condition set、V1 `omitempty`迁移、Owner-local strict validation及REV-D12 admissibility current Reader候选；
- `condition-v2-test-matrix.md`：Condition canonical、Auto/Human/Verdict无损链、多签union、Policy exact tuple publisher/current、S1/S2、TTL、legacy隔离与Satisfaction边界CND-01..61；
- `result-bundle-current-grounding-v2.md`：typed Result Bundle、Context exact intent/criteria、三类具名stable identity、Validation Scope Owner-neutral association、Artifact Anchor语义Owner、Context/Evidence复用、REV-D13三类Owner exact-current Reader/Publisher Port Delta、full Binding/ReaderBinding required-catalog router；aggregate完整封入Scope association与每个nominal resolved-route proof，冻结具名exact Reader request/dependencies/closed errors、Owner Binding S1/S2、true min TTL与bounded lost-read恢复；
- `detached-delivery-v1.md`：Detached Runtime Run/外部Thread的Review-owned Binding/Closure、唯一执行顺序、S1/S2、TTL、lost-reply与四Owner Port Delta；
- `result-bundle-current-grounding-v2-test-matrix.md`：canonical/type-pun、Owner current/history/ABA、逐输入min TTL、lost read、root与legacy负例；
- `acceptance.md`：设计与未来实现的验收矩阵；
- `.properties.rax/plan/review-engine/review-engine-v1.md`：文件级实施计划。

## 12. 当前待管理线裁决

1. 安全基线v1已冻结；后续按业务域覆盖精确不可Bypass清单与Profile调整，但未来Tool不得依赖静态ID allowlist；
2. Rubric payload/version/revoke 已冻结归 Review Owner；后续只需按业务域冻结谁有资格请求 Review Owner 发布新 revision，以及 Policy 适用条件，不得建立第二 Rubric Owner；
3. Request Changes、Escalate Human、Insufficient Evidence、Conditional 的 Case/流程/Verdict 映射；
4. Human 多签已冻结为租户 K-of-N + 必需角色、veto Reject硬否决、显式 current Delegation、production禁自审；具体角色集合、K/N与TTL由租户版本化 Policy 必填；
5. Review Round/token/time/cost/重复阈值和升级目标；
6. Behavior Feedback 的正式资产 Owner、Admission、TTL、申诉、跨租户隔离和影响后续 Policy/Context 的规则；
7. 外部 Adapter 的脱敏字段、通知责任、Reviewer Lease 冲突与平台身份映射。

这些问题不阻止本设计进入联合评审，但在对应实现阶段前必须获得明确裁决。
