# Review Engine 验收合同

## 1. 当前验收结论

本文冻结设计、Wave 1 reference/test实现与Review-owned production service的验收口径。SQLite WAL、HTTP/SSE、SDK/CLI和平台协议Adapter已经落地；五类production current Reader、真实Provider/identity admission、跨Owner adapter与composition root仍须关闭对应Port Delta和公共装配依赖。

当前验收结论：**Review owner-local/reference/test 非完整生产闭环最终独立复审 YES（P0/P1/P2=0）；Praxis production integration 仍 NO-GO**。真实外部Owner adapter/certification、宿主root、公共Gate与Provider Effect未验收。

## 2. 设计完整性验收

| 编号 | 验收项 | 通过条件 |
|---|---|---|
| D-01 | Owner 边界 | Review 只拥有 Request/Case/Round/Assignment/Attestation/Finding/Condition/Verdict/Trace 与 BehaviorFeedbackCandidate，不写 Runtime Outcome、Binding、Policy、Trust 或其他领域事实 |
| D-02 | 判定边界 | Review 不 Dispatch、不 Commit、不执行 Tool/Memory/Knowledge/Artifact 写入 |
| D-03 | 正交建模 | Target、Delivery、Route、Profile/Rubric 独立表达，Human/Auto/Bypass 不与 Inline/Detached 混成单枚举 |
| D-03A | Rubric Owner | Rubric payload/version/schema/revoke归Review Owner；Policy只选择exact ref；七类baseline无万能Prompt/写权限 |
| D-03B | Rubric Current | append-only history/full-ref current/highest同事务；Request Admission S1/S2+Store actual-point重验，漂移/TTL/rollback零写 |
| D-04 | 精确绑定 | Verdict 绑定 exact Case、Target/Candidate/Intent/Payload revision+digest、Policy、Authority、Scope、Evidence、Conditions 与 TTL |
| D-05 | 当前性 | Target/Policy/Authority/Scope/Binding/Evidence/TTL 任一漂移均 Fail Closed 并重新 Review |
| D-06 | Observation 边界 | Human/Auto/Remote/Provider 输出仅为 Attestation/Observation，由 Review Owner 独立 Inspect 并 CAS Verdict |
| D-07 | Bypass | `operation_not_required` 是显式策略事实，不伪造 accepted Verdict，且不绕过其他治理门禁 |
| D-08 | Auto fork | 使用只读、去工作身份化 ContextFrame；无 Dispatch/Commit/Spawn 权；具备预算和终止条件 |
| D-09 | UnknownOutcome | Begin 后丢回包只 Inspect 原 attempt；不得盲重派 Reviewer、重复 Decide 或生成替代 allow |
| D-10 | pre-run Evidence | 不产生 pre-run 权威 Evidence；权威事实必须绑定已存在的父 Run 或已 Admission 的 ReviewRun |
| D-11 | Harness 边界 | 仅引用接线线的 namespaced/versioned Slot/Phase 对象；不依赖 Harness 私有 ContextPort/ModelTurnPort/EventCandidatePort |
| D-12 | Model Invoker 边界 | 只使用公开 RouteID + routegateway/execution union；stream/completed/cache/provider 状态不能成为 Verdict |
| D-13 | Auto settlement | 只接受 stored `State=applied` ApplySettlement，并 exact 复读 ReviewerInvocationResult；Tenant/Case/Round/Assignment/Target/Attempt/ResultDigest 任一漂移 Fail Closed |
| D-14 | Owner currentness | Decide 不信 caller current；单次 snapshot 复读全部 Owner current facts，fresh clock 重验并取最短 TTL |
| D-15 | 历史与恢复 | current index 与 append-only history 分离；staged failure 零历史泄漏；mutation reply-loss 只 exact Inspect 原对象 |
| D-16 | Case 创建入口 | Store public shape 只允许原子 `CreateTargetCaseV1`；不得存在 Case-only create 绕过 Target 唯一索引 |
| D-17 | Decide 时钟区间 | current Inspect 前 non-zero baseline、返回后 fresh now；`now < baseline` 时 zero Verdict/CAS，即使 now 仍晚于 facts |
| D-18 | Target history | 同 TargetID 的 revision/digest append-only；same revision换digest、revision rollback均 zero write，旧 exact history仍可读 |
| D-19 | Create 原子性 | Target+Case+optional Requested Trace同锁发布；Trace冲突在首个写前失败，lost reply exact Inspect原三对象且不重调 mutation |
| D-20 | Reviewer drift | ReviewerID、ReviewerAuthority、ReviewerBinding任一漂移均在Decide actual point zero Verdict/CAS |
| D-21 | TTL 独立性 | Policy、ActorAuthority、ReviewerAuthority、Scope、Binding及每项Evidence分别成为唯一最短TTL时，Verdict expiry精确等于该值；actual-point过期zero write |
| D-22 | REV-D11 exact refs | Binding使用`ReviewComponentBindingRefV2`六字段full ref；Evidence保留`ReviewEvidenceRefV2`三字段并由Evidence Owner返回full `EvidenceOwnerFactRefV2`；Owner projection另含immutable ProjectionID/Revision、exact Subject/Checked/Expires/ProjectionDigest，Review不派生、猜测或补签 |
| D-23 | REV-D11 S1/S2 | canonical request经S1全量Owner Inspect并保存完整sealed projection后，以同一组exact projection/ref/subject requests执行S2、调用`ValidateCurrent`并逐字段比较；任一字段漂移整批zero projection |
| D-24 | REV-D11 min TTL | external projection精确取Policy、ActorAuthority、ReviewerAuthority、Scope、Binding、每项Evidence的最短正TTL；`now >= expires`即过期 |
| D-25 | REV-D11 closed errors | 只返回冻结Category/Reason组合；Unknown不降NotFound、Owner错误不吞掉、缺Reader为CapabilityUnavailable |
| D-26 | REV-D11 recovery/clock | lost reply至多一次Inspect同一canonical request；恢复不受原ctx取消影响；baseline/S1/S2/recovery/final任一clock rollback均Fail Closed |
| D-27 | REV-D11 production边界 | reusable conformance/benchmark fixture无Owner写口且不冒充生产；公共Owner Port与composition未闭合前production Adapter/root保持NO-GO |
| D-28 | REV-D11 projection不可变 | 状态变化create-once新projection并CAS current index；Checked/Expires/ProjectionDigest固定；exact Inspect返回同一deep clone；fresh now只ValidateCurrent，不重封或刷新projection |
| D-29 | REV-D11 S1 Ref来源 | S1仅以stored facts派生的full ExactSource+ExactSubject线性化resolve Owner current index取得完整ProjectionRef；S2只按该Ref复读并验证index；禁止caller/stored预带ref、by-name/latest与ABA |
| D-30 | Condition exact set | Conditional production对象必须携带非空、按ID严格递增且逐字段有效的`[]runtimeports.ReviewConditionV2`；digest只由`DigestReviewConditionsV2`证明 |
| D-31 | Condition legacy隔离 | digest-only Conditional只允许historical exact Inspect/低层兼容；Service、Auto Owner、Verdict Owner、Runtime Adapter均不得让其产生新Verdict或Authorization |
| D-32 | Condition admissibility | Policy Owner明确允许每个exact Schema/SatisfactionOwner/Capability/Authority tuple；Review不得从Policy Active、名字或Rubric允许Conditional推导 |
| D-33 | Condition S1/S2 | REV-D12对Policy、每项SatisfactionOwner Binding、每项Condition Authority执行full-ref S1/S2；不追随新ref，不接受ABA、drift、Unknown或Unavailable |
| D-34 | Condition scope/TTL | 每项ScopeDigest等于Target ActionScope；Verdict TTL进一步取每项Condition及admissibility projection最短TTL，actual-point crossing/rollback zero CAS |
| D-35 | Satisfaction边界 | Review不创建/CAS `ConditionSatisfactionFactV2`，Provider/Human自报不升级；Runtime只有在独立Owner current Satisfaction成立时才能授权 |
| D-36 | Result Bundle V2 exact | Bundle精确绑定Request/Target、typed Artifact Owner/source+Anchor、Environment、Context Envelope/source、Validation Scope与逐Claim full Evidence；Claim可直接定位且无弱字符串fallback |
| D-37 | Anchor语义Owner | Artifact Owner复读exact revision并验证locator；Continuity relation、Sandbox snapshot、UI路径或latest对象不能冒充generic Artifact/Anchor current |
| D-38 | Result Grounding S1/S2 | Context/Evidence复用live Reader；OriginalIntent/AcceptanceCriteria exact匹配Context materials；三具名ProjectionIdentityInput冻结stable ID；Validation Scope Owner-neutral identity只允许一个Owner association；全部source、association及Owner Binding保存S1 full Ref，S2只按同Ref复读并验证index，aggregate完整封入association，拒绝ABA/漂移 |
| D-39 | Result Grounding min TTL | Bundle、Request、Target、Context及全部source、全部Artifact、Environment、Validation Scope Owner association/Scope、每项Evidence、每个external Owner Binding分别作为唯一最短TTL时，aggregate expiry精确等于该值；actual-point crossing zero CAS |
| D-40 | Result Grounding recovery/root | lost S1形成新cut不冒充恢复；lost S2只重读same Ref；`0<ReadRecoveryTimeout<=2s`并按cut TTL/deadline裁剪；router返回nominal sealed resolved-route，aggregate封入Declaration/full Owner、RouteRef、ReaderBindingRef/adapter digest proof；独立required catalog且全部public error方法closed；Unknown、clock rollback、cross-tenant、unknown kind、Reader/root缺失全部Fail Closed |
| D-41 | Result Bundle legacy隔离 | V1可historical Inspect，但string Anchor/digest-only Environment/Scope不能进入production Verdict、Runtime current或Authorization，禁止自动升级 |
| D-42 | Result Bundle current语义 | V2 Bundle只允许create-once atomic append与immutable history/exact Inspect；无Bundle current index或replacement CAS，currentness只由Decide read cut与TTL证明 |
| D-43 | Detached Owner边界 | Review只拥有DeliveryBinding/Closure；不得复制Runtime Run、Application coordination、Harness waiting或把外部Thread评论当Verdict |
| D-44 | Detached exact recovery | parent/child lineage、Phase、Waiting、endpoint、Case/Verdict全exact S1/S2；min TTL、rollback、ABA、lost reply只Inspect，Closure不授父Run恢复权 |

## 3. 状态机不变量

### 3.1 Case 与 Round

- Case 的 Target Snapshot 创建后不可原位修改；Target 漂移只能 Supersede 并创建新 Candidate/Case。
- 同一 Case Revision 的状态改变必须以 expected revision 执行 CAS。
- 同一 exact Target 只能经原子 Target+Case create-once 入口绑定 Case；public/concrete Store 均不得暴露 Case-only create。
- 同一 Target revision 不得换 digest；新 revision 严格递增。非空 Requested Trace 与 Target/Case 原子发布，冲突时三者零写。
- Assignment Lease 只解决并发领取，不创建或扩大 Reviewer Authority。
- 每个 Auto Reviewer 实例在产生一次结构化 Attestation 或终止原因后结束。
- Request Changes 由 Target Owner 产生新 Candidate Revision，不允许 Reviewer 直接修改 Target。
- Expired、Revoked、Superseded、Cancelled 的旧 Verdict 只保留审计价值，不能恢复授权。

### 3.2 Verdict

- Accepted 只有在所有 exact binding 当前时才可投影为 allow。
- Conditional 只有所有 Condition Satisfaction 由相应 Owner 独立 Inspect/CAS 且当前时才可投影为 allow。
- Rejected、Request Changes、Escalate Human、Insufficient Evidence、Expired、Revoked、Superseded 一律不授权执行。
- Bypass 必须有 current PolicyNotRequired 事实；不得创建虚假 Reviewer Attestation。
- 并发 Decide 只能有一个 CAS 胜者；输家 Inspect 当前 Fact 后返回冲突或幂等成功。

## 4. 外部治理链验收

任何 Reviewer 外部调用、通知、附件拉取、远程会话或主动轮询都必须保持：

```text
领域 Intent/Reservation
  -> Admission
  -> Permit
  -> Begin
  -> Delegation/Prepare
  -> Enforcement
  -> Execute/Inspect
  -> Observation/Evidence
  -> Review DomainResultFact
  -> Runtime Operation Settlement
  -> Review ApplySettlement
```

验收反例：

- Permit 前调用远程 Reviewer；
- Begin 后超时即创建新 attempt；
- Model completed 或 HTTP 200 直接写 Verdict；
- Provider 自报 usage 直接写 Budget Fact；
- Review Owner 代写 Runtime Settlement/Outcome；
- 外部消息删除成功被当成原调用成功。

## 5. Effect / Conflict Domain / Operation Subject与Scope验收

| Effect kind | Conflict Domain 最低要求 | Operation Subject / Scope | 关键验收 |
|---|---|---|---|
| Auto/Remote Reviewer invocation | tenant + Case + Round + Invocation attempt | 父 Run 或 ReviewRun | 双重门禁；输出仅 Observation；Unknown Inspect-only |
| 只读探索/验证/检索 | tenant + Case + ContextFrame + operation | ReviewRun；受限本地读取可用父 Run | 只读不等于无 Effect；Scope/Authority/Budget/Fence 当前 |
| Human Envelope/外部通知 | tenant + Case + Adapter + message idempotency key | 父 Run 或 ReviewRun | 脱敏；at-least-once Receipt；重复发送可识别 |
| Attachment 拉取/验证 | tenant + Case + Attachment ref | ReviewRun | 内容先 Observation；Evidence Owner Admission 后才可引用 |
| Webhook/主动轮询 | tenant + Case + Adapter + external event id | 父 Run 或 ReviewRun | source sequence 去重；身份和 Case currentness 复读 |
| Remote cancel/cleanup | 原 invocation conflict domain | 原 ReviewRun | 只控制资源；不改写历史；Cleanup 与 Residual 分离 |
| Unknown attempt Inspect | 与原 attempt 相同 | 原 Run | 只能 Inspect 原 attempt；Inspection 也需结算 |

Review State Plane 的 create-once/CAS 不被包装成 Provider 成功；reply-loss 必须按稳定 ID、Revision、Digest Inspect。

## 6. Slot / Phase Contribution 验收

| 接线对象 | Review 贡献种类 | 允许结果 | 禁止事项 |
|---|---|---|---|
| `review.gate` | Port | Owner Port + N Reviewer Provider contribution | Review 自创 Slot enum 或抢占 Assembler Owner |
| `action.review` | Gate | allow/deny/ask/defer 的版本化 PhaseDecision | 改 Context、直接调用网络、写 Runtime Fact |
| `subagent.completion.validate` | Gate | 对 exact Result revision/digest 判定 | 写 Completion Claim 或 Runtime Outcome |
| `run.completion.validate` | Gate | 对 exact Result Bundle/Grounding 判定 | 代替 Settlement/CompleteRun |
| `residual.detected` | Observer | 只报告 Review Residual | 清理资源、阻断流程或修改其他 Owner Fact |

公共 Slot/Phase 合并规则、CompiledGraph 与 Binding V2 映射未统一前，不验收任何 Review 私建替代对象。

## 7. API / SDK / CLI 验收

- `SubmitReview` 只创建 Request Candidate；Admission 后才 create-once Case。
- `approve/deny/request-changes` 只提交 Attestation/Intent；不存在对外 `WriteVerdict`。
- `Get/Inspect/Watch/List` 返回事实或 at-least-once Observer 流，不作为执行授权缓存。
- `ClaimReview` 使用 expected revision + Lease，且复读 Organization Authority。
- 重复 CLI/Webhook/SDK 提交同 idempotency key + 同 digest 返回同一结果；换 digest 返回冲突。
- transport-neutral；不预选 HTTP、gRPC、消息队列、生产 DB、RPC、进程拓扑或 SLA。
- 外部 Adapter 故障不能改变唯一 Review Owner。

## 8. 测试矩阵

| 测试层级 | 必测场景 | 通过标准 |
|---|---|---|
| 单元 | canonical digest、Validate、状态迁移、CAS、TTL、Route/Delivery/Profile 正交、Condition currentness | 决定性、无时间隐式依赖、正反例完整 |
| 白盒 | 并发 Decide、Lease 争抢、迟到 Attestation、Supersede、撤销、幂等冲突、Residual 聚合 | 单一线性化点；无旧授权复活 |
| 黑盒 | Human、Auto、Bypass、Inline、Detached、Conditional、Request Changes、外部 Adapter | 只通过公开组件 API/Port；不导入 internal/fake |
| 故障注入 | Begin 丢回包、CAS reply-loss、Store unavailable、事件重复/乱序、Authority/Policy/TTL 漂移、Context materialize 失败 | 只 Inspect 原 attempt；Fail Closed；Cleanup/Residual 可见 |
| Conformance | Store、Decision/Auto/Human/Bypass、Runtime只读投影、Condition V2、Result Grounding V2已形成可复用或黑盒conformance；REV-D11 aggregate覆盖exact Source/Subject、S1/S2、TTL、lost reply、rollback、deep clone与漂移 | 只验Review-owned聚合；真实Owner Adapter/root缺失必须Fail Closed，Fake不冒充生产能力 |
| Benchmark | 已有Router、HTTP Inspect与SQLite基准；REV-D11 N=1/8/64/256跨Owner聚合基准仍只保留设计 | 只建立Go owner-local成本基线；无production SLA、无Rust结论 |
| Race | Case/Assignment/Attestation/Verdict 并发创建、Inspect、CAS、Watch | `go test -race` 无数据竞争；CAS 结果一致 |
| Vet | 全组件与 adapter package | `go vet` 无新增问题 |
| 集成 | Review Facts -> current `OperationReviewCurrentProjectionV4` -> Runtime 后续治理链 | 当前只验收只读 projection；Permit/Begin/Provider 和 production composition 不属于 Wave 1 |
| 系统 | 持久异步等待、进程重启恢复、外部 Human 回调、Auto fork、Run 收口 | Application 恢复同一 Case；Runtime 只在全部 Settlement 后收口 |

## 9. 关键端到端验收序列

唯一允许宣称完整闭环的正向序列是：

```text
Create Request Candidate
  -> Admission + create-once Case
  -> Human/Auto Observation
  -> Review Owner Inspect + Decide/CAS
  -> Runtime current OperationReviewAuthorizationV3 projection
  -> Operation Permit + Begin
  -> Provider Prepare/Enforcement/Execute
  -> Observation + Evidence Admission
  -> Domain/Operation/Review Participant Settlement
  -> Application 聚合后 Runtime CompleteRun
```

至少覆盖以下反向序列：pending、rejected、expired、revoked、superseded、conditional-unsatisfied、Target drift、Authority drift、Policy drift、TTL expiry、Context residual、Provider reply-loss、Verdict CAS reply-loss、重复 Webhook、过期 Assignment、Bypass 撤销。

## 10. 性能与语言验收

- 默认 Go；当前没有可证明的计算稠密热点，不引入 Rust。
- v1 先记录 Case create/Inspect/CAS、digest、Watch fan-out、current projection 的基线，不预设生产 SLA。
- 只有 Go 基准证明某纯计算热点在代表性负载下不满足已批准目标，才可另提 Rust 设计；必须同时冻结 Go ABI/进程边界、序列化、取消、panic/crash、超时、UnknownOutcome 与回退语义。

## 11. 发布门禁

下列条件全部满足前，不得发布 Runtime/Application/Harness Adapter：

1. REV-D3、REV-D7 与相关 Runtime Owner 裁决关闭；
2. REV-D8 与 Application/Harness 接线联合验收；
3. Agent Assembler、Assembly SDK/CompiledGraph、Slot/Phase 合并、Binding V2、Checkpoint/Action Gateway/per-turn refresh 完成统一；
4. 管理线冻结 v1 Profile 路由、不可 Bypass 清单、Resolution 映射、Human Authority/多签规则与终止预算；
5. ContextReference 不闭合 Route 采用可验证 Fail Closed/Residual；
6. 上述测试矩阵按阶段通过且证据可复查。
