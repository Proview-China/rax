# Review Engine 治理链、外部接口与恢复

## 1. 总链路

Review 不重造 Runtime/Application 已闭合的 Operation V3 链。所有会越过进程、网络、模型、外部平台或执行点的动作保持以下固定顺序：

```text
领域 Intent / Reservation
  -> Admission
  -> Permit
  -> Begin
  -> Delegation / Prepare
  -> Enforcement
  -> Execute 或 Inspect原attempt
  -> Observation / Evidence
  -> 领域 Owner Inspect / CAS / Settlement
```

Begin 后丢回包只允许 Inspect 原 attempt。新的 idempotency key、Reviewer fork、Webhook delivery 或表面改写不能代替恢复。

## 2. Review 主流程

```text
Target Owner形成冻结Candidate
  -> Application Coordinator提交ReviewRequest Candidate
  -> Review Admission复读Run/Scope/Policy/Authority/Binding/TTL
  -> Review Owner CreateCase(create-once)
  -> Policy Route: Human | Auto | Bypass
      Human -> Assignment/Lease -> Human Attestation Observation
      Auto  -> Reviewer Invocation Effect治理链 -> Reviewer Attestation Observation
      Bypass -> current PolicyNotRequired Decision（无Reviewer Attestation）
  -> Review Owner Inspect当前Target/Policy/Authority/Evidence/Attestation
  -> Decide/CAS唯一Verdict或非授权Resolution
  -> Runtime current projection
  -> Application/Harness PhaseDecision: allow | deny | ask | defer
  -> Host Gateway在真实Operation前重读Review+Authority+Budget+Scope+Fence
  -> Permit/Begin/Provider实际执行点二次门禁
  -> Observation/Evidence -> 对应领域DomainResultFact -> Runtime Operation Settlement -> 领域ApplySettlement
```

Review 完成不表示动作已发生、Artifact 已 Commit、Run 已完成或 Outcome 已成立。

## 3. Target Admission

Admission 只接受：

- immutable Target ID/Revision/Digest；
- 当前 Source Run 或已 Admission 的独立 ReviewRun；
- 当前 ExecutionScope、Actor Authority、Policy、Binding、Risk、ActionScope；
- canonical Evidence set 与 Reviewer Context Frame ref；
- 明确 Delivery、Route constraint、Profile/Rubric 和 TTL；
- 唯一 Review Owner 与 Reviewer Capability binding。

拒绝：

- “审核当前状态”式动态指针；
- 未建立 Run 就要求产生权威 Evidence/Verdict；
- 只携带 Harness `ReviewRequired` 布尔值；
- Model Tool Call、stream/completed/cache usage、Provider 状态直接充当事实；
- Raw/Native Provider Event、Model Invoker internal 类型或厂商 SDK 对象；
- 缺少 Context Reference 且 Route 无法物化 Review Frame，却仍宣称通用支持。

## 4. Human Gate 流程

1. Review Owner 创建 Case 与 Human Assignment；
2. 外部 Delivery 如需网络/平台写入，先建立 `review/external-delivery` Effect；
3. Adapter 只发送脱敏 Envelope/Deep Link，并记录 Receipt/Observation；
4. 人类领取 Assignment Lease，Authority Owner 仍是权限事实源；
5. CLI/UI/SDK/Webhook 提交 Attestation，带 expected Case Revision 与幂等键；
6. 回包丢失时 Inspect 精确 Attestation ID/幂等键，禁止重复 Resolve；
7. Review Owner 复读 Case、Target、Identity/Authority、Policy、Evidence、Lease 与 TTL；
8. CAS Verdict；并发 Reviewer 只有一个当前 Revision 获胜，迟到回复成为历史 Observation；
9. Application Coordinator Inspect 当前 Case/Verdict 后恢复原 Gate。

外部 Ticket/Message 状态不是 Verdict；`approve` CLI 命令的真实语义是“提交 Human Attestation”，不是直接写 Verdict。

## 5. Auto Reviewer 流程

1. Context Owner 物化冻结 `ReviewerContextFrame`；无法可靠物化时 Fail Closed 或显式 Residual；
2. Review Owner 创建 Auto Assignment/Round，绑定 RouteID、Rubric、Schema、只读能力、预算与终止上限；
3. Reviewer 模型调用按 Operation V3 建立 `review/reviewer-invocation` Effect；
4. 使用 Model Invoker 公开 `RouteID + routegateway/execution union`；
5. Begin 后调用失败、超时或取消一律 Inspect 原 attempt；
6. stream、completed、usage、cache、provider 状态只作为 Observation；
7. Review Owner独立Inspect后形成`ReviewerInvocationResultFact`；Runtime Settlement Owner再基于精确Observation/Inspect引用提交Operation Settlement，Review只ApplySettlement形成领域settled投影；
8. 只有 Review Store exact 复读到 `State=applied` 的 ApplySettlement，且再 exact 复读其 ReviewerInvocationResultFact 并证明 Tenant/Case/Round/Assignment/Target/Attempt/ResultDigest 全链一致时，结构化输出才可成为 Reviewer Attestation Observation；`not_applied`/`failed` 与任一交叉漂移均 Fail Closed；
9. Review Owner通过只读exact current Reader形成单次snapshot：S1以stored facts派生的full ExactSource+ExactSubject线性化resolve各Owner current index，取得完整ProjectionRef后exact Inspect并保存immutable projection；S2只按该exact Ref复读并原子验证index未漂移，不重新resolve current/latest；fresh clock只调用`ValidateCurrent`，不重封Checked/Expires/Digest；
10. Reviewer 实例终止，不保留工作 Agent 身份或写入能力。

只读探索不是治理豁免。若探索会联网、披露数据、消费费用或调用外部工具，必须分别走受治理 Effect；本地纯读取也必须受 Sandbox Scope 与 Rubric allowlist 约束。

REV-D11 的lost-reply恢复只允许对同一canonical Owner request做至多一次read-only Inspect，恢复context不受原ctx取消影响；不能换ref、调用mutation、把Unknown降NotFound或复用旧current。Binding/Evidence等Owner公共Reader与production composition仍未闭合，故该流程当前只具Review侧冻结设计，不构成production Adapter授权。

## 6. Bypass / YOLO 流程

```text
ReviewRequest
  -> current Policy明确operation_not_required
  -> Review Owner记录BypassDecision/Trace（无Reviewer Attestation）
  -> Runtime生成独立not-required当前投影
  -> Host Gateway继续合取Authority/Budget/Scope/Fence/Binding/Policy
```

Bypass 不允许：

- 生成虚假 accepted Human/Auto Verdict；
- 绕过 EffectIntent/Admission/Permit/Begin；
- 跳过 Evidence、Sandbox、Budget、Authority、Scope 或执行点二次门禁；
- 在 Policy 撤销、Target 漂移或 TTL 过期后复用旧 Permit。

## 7. Inline 与 Detached

### Inline

- 绑定父 Run 与当前 PhaseContext；
- Application Coordinator 持久记录 waiting/defer，不阻塞线程；
- 重启后 Inspect 原 Case/Verdict/PhaseReceipt；
- Target 改变时 Supersede 原 Case，不能恢复旧 Verdict。

### Detached

- 先建立受治理 ReviewRun，再产生权威 Review Evidence；
- ReviewRun 与 SourceRun、Case、Target 精确关联，但 Runtime 仍拥有 Run 生命周期；
- Human/Auto 都可 Detached；
- 外部 Watch/通知是 Observer，不具备延迟撤销已执行动作的权限。

## 8. 全部规划 Effect

下表是 Review v1 可能产生的全部 Effect 类别。具体 namespaced `EffectKind`、Provider Binding 和策略阈值由装配/Policy 冻结；组件不新增 Runtime 枚举。

| Effect 用途 | 典型 Runtime Effect 类 | Conflict Domain 要求 | Operation Subject / Scope | Settlement/Cleanup/Residual |
|---|---|---|---|---|
| Auto Reviewer 模型调用 | data disclosure + cost consumption + provider continuation/hosted execution | tenant-stable；Case+Round+Assignment+Route 的唯一业务域 | Parent Run 或独立 ReviewRun | 必须 exact Settlement；远程保留按 Provider residual 报告 |
| Reviewer 只读远程探索 | data disclosure / hosted execution | tenant-stable；Case+Round+Target scope | 同上 | 只读不等于无 Effect；未知只 Inspect，远程 residual 显式报告 |
| 外部 Human Envelope/通知 | external mutation + data disclosure | tenant-stable；Case+Adapter+消息幂等键 | Parent Run 或 ReviewRun | at-least-once Receipt；必要时无补偿，只 Inspect；脱敏残留报告 |
| 外部 Attachment 拉取/验证 | data disclosure / resource lifecycle | tenant-stable；Case+Attachment ref | ReviewRun 必需 | 内容先 Observation；正式 Evidence 由 Evidence Owner Admission |
| Webhook/外部响应摄取 | 外部输入 Observation；若主动轮询则 data disclosure/cost | tenant-stable；Case+Adapter+external event id | ReviewRun 或当前父 Run | 重复事件幂等；回包未知 Inspect exact attestation |
| Remote cancel/cleanup | safety control / resource lifecycle | 原 invocation conflict domain | 原 ReviewRun | 只撤销 Reviewer 资源，不改变外部世界历史；Cleanup/Residual 分开 |
| Unknown reviewer attempt Inspect | inspection Effect，绑定原 attempt | 与原 attempt 冲突域一致 | 原 Run | 只能 Inspect；Inspection 自身也有 Settlement，禁止递归盲重派 |

Review Case/Attestation/Verdict 的 State Plane CAS 是领域事实写入，不是外部 Provider Effect；但它仍必须使用 create-once/CAS、幂等和 reply-loss Inspect。

## 9. Effect / Review / Fence / Unknown 矩阵

| 场景 | Review 事实 | Host Gate | 执行点 Fence | Unknown 恢复 |
|---|---|---|---|---|
| Human pending | Case pending，无 Verdict | deny/defer | 不得签发 Permit | Inspect Case/Attestation |
| Auto invocation 未 Settlement | Attestation 不可用于 Decide | deny/defer | 不得签发 Permit | Inspect 原 invocation attempt |
| Accepted current | current Verdict + Evidence | 合取 Authority/Budget/Scope/Policy | 实际点重读 ReviewAuthorization + Fence | Permit/Execute unknown 只 Inspect |
| Conditional 未满足 | conditional Verdict，无 current Satisfaction | deny/defer | 不得执行 | Inspect Satisfaction journal |
| Conditional 已满足 | exact Verdict + Satisfaction | 可进入其余门禁 | 重读 Satisfaction/TTL/Fence | 任一漂移失败关闭 |
| Bypass current | explicit PolicyNotRequired | 仍检查全部非Review门禁 | 仍检查 Permit/Fence | Policy/TTL unknown失败关闭 |
| Rejected/expired/revoked/superseded | 历史 Verdict 仅审计 | deny | 不得执行 | 不创建替代 allow |
| Target/Authority/Policy/Scope 漂移 | 旧 Fact 历史保留 | deny并重新Review | 旧 Permit 无效 | 创建新 Candidate/Case，不重放旧动作 |

## 10. Receipt、Settlement、Cleanup、Residual

- Provider/Human/Adapter Receipt 只证明收到或观察到，不能证明审核结论；
- Attestation 先进入 Evidence/Review Owner Inspect，再 CAS Verdict；
- Auto Reviewer Invocation 的 Settlement Owner 只结算该调用是否应用/未应用/失败，不选择 Review Verdict；
- Review 本身不写 Runtime Outcome；Application 聚齐 Review、Effect、Participant、Cleanup Settlement 后由 Runtime Owner 收口；
- Reviewer 远程会话、临时 Snapshot、外部平台消息、未完成 Attachment、未知 webhook 等分别产生 Cleanup/Residual；
- Cleanup complete 必须有独立 Evidence，Residual 不得被成功 Verdict 掩盖；
- Fake 只能验证状态机，不能宣称生产 Backend、SLA、外部幂等或平台清理能力。

## 11. SDK / CLI / API 边界

### Review-owned API

| 能力 | 写入语义 | 并发/恢复 |
|---|---|---|
| SubmitReview | 创建 Request Candidate，Admission 后 create-once Case | idempotency key + Request Digest；丢回包 Inspect |
| GetReview / InspectVerdict | 只读当前与历史事实 | 不缓存成执行授权 |
| ListPendingReviews | 分页查询 | 快照/游标必须有 TTL，不承诺强实时 |
| WatchReview | 事件流 Observer | at-least-once；客户端按 source sequence 去重 |
| ClaimReview | CAS Assignment Lease | expected revision；Authority 不由 Lease 产生 |
| AttachEvidence | 提交 Evidence Candidate/Ref | Evidence Owner Admission 后才可引用 |
| SubmitAttestation | Human/Auto/Adapter Observation | create-once + expected Case Revision；丢回包 Inspect |
| RequestChanges | 非授权 Resolution/Attestation | 不能直接改 Target；由 Target Owner 新建 Revision |
| CancelReview | 提出取消意图 | Review Owner CAS；不直接取消 Runtime Run |

对外不暴露“直接写 Verdict”的通用 API。Owner 内部 `Decide` 仍必须经 currentness Inspect 和 CAS。

### CLI

规划命令：`praxis review list/show/watch/approve/deny/request-changes/attach/cancel`。其中 approve/deny/request-changes 都提交 Attestation 或 Intent，不直接写领域事实。

### SDK

- Go 为首个 SDK，导出类型化 Request/Case/Envelope/Assignment/Attestation/Finding/Verdict/Event；
- transport-neutral，HTTP/gRPC/消息队列均不在本设计中预选；
- SDK 不提供绕过 Admission、Review Owner、Runtime Gateway 的 direct dispatch；
- Linear/Jira/Slack 等仅实现 Adapter Port。

## 12. Slot / Phase / Port 映射

| Harness 接线对象 | Review 贡献 | 公共/私有 | 冲突检查 |
|---|---|---|---|
| `review.gate` | Owner Port + Human/Auto Reviewer Provider contributions | 公共装配对象 | 只能一个 Owner；Reviewer 为 N Provider，不获得 Slot Owner 权限 |
| `action.review` | Gate | 公共 PhaseContribution | deny > defer > ask > allow；不与低权限 Gate 宽松合并 |
| `subagent.completion.validate` | Gate | 公共 PhaseContribution | 绑定 Result revision/digest；不写 Completion Claim |
| `run.completion.validate` | Gate | 公共 PhaseContribution | 绑定 Result Bundle/Grounding；不写 Runtime Outcome |
| `residual.detected` | Observer | 公共 PhaseContribution | 只报告，不阻断或清理 |
| Harness ContextPort/ModelTurnPort/EventCandidatePort | 无贡献、无依赖 | Harness 私有 | Review 导入即失败 |

公共 Slot/Phase 版本、合并顺序、Binding V2 映射和 CompiledGraph 由 Harness 接线线统一实现。Review 只提供 Manifest/Capability/Dependency/Contribution 声明。

## 13. 兼容与迁移

1. legacy `ReviewPort` 与 `ActionRequest.ReviewRequired` 仅保留受限 Observation 兼容，不承载正式授权；
2. Effect Review 优先复用当前 `ReviewGovernanceGatewayV2`；
3. 通用 Artifact/Outcome/Detached Target 等待 Target Delta；
4. Operation V3 等待从 rich Review Facts 派生 current `OperationReviewAuthorizationV3` 的联合投影；不得在 fixture 外手工构造；
5. Runtime/Application/Harness 仍按现有 Operation V3 外部动作顺序；
6. Assembly/CompiledGraph/Slot/Phase 合并未统一前，只实现独立合同和纯领域状态机，不发布 Runtime/Harness Adapter；
7. 数据迁移必须保留旧 Case/Verdict 为历史，不能把 legacy boolean 或外部评论批量升级为 accepted Verdict。
