# Review Engine 模块说明

## 当前状态

- Wave 1 reference/test implementation：最终独立复审 **YES（P0/P1/P2=0）**。
- Review-owned非完整生产闭环：单机服务、Decision/Auto/Human/Bypass、Runtime只读组合、Result Grounding V2与测试已完成，最终独立复审 **YES（P0/P1/P2=0）**。
- Review-owned Rubric V1：七类 baseline、Store/SQLite/Admission/conformance 已完成，纳入本轮最终独立复审 **YES（P0/P1/P2=0）**。
- Praxis production integration：**NO-GO**。真实外部Owner adapter/certification、宿主 composition root、公共Gate与Provider Effect尚未闭合。
- 2026-07-18 production-proof live 核验：11 项 proof 仍全部缺少可独立验证的 production current certification；Decision、Verdict、SQLite durable store、Human owner-local software 已实现但不能升权。`releasecandidate` 已发布机器可读 blocker inventory，继续固定 `reference_only`。

这三个状态互不替代：SQLite/API 软件闭环不授 Runtime Authorization、Permit、Begin 或外部 Effect 执行权。

## 模块作用

Review Engine 是判断 Owner。它维护 Request、Target、Case、Round、Assignment、Attestation、Finding、Condition、Verdict、Trace、Result Bundle 与 Behavior Feedback Candidate；它不 Dispatch、不 Commit、不写 Runtime Outcome，也不执行 Tool、Memory、Provider 或跨域反馈应用。

## 已实现组成

| 包 | 责任 |
|---|---|
| `contract` | Review 领域事实、精确引用、canonical digest、currentness、七类 Rubric baseline 与 Result/Feedback 合同 |
| `ports` | Review Store、exact Inspect、CAS、query 与只读 external-current seam |
| `caseengine` | Target+Case+optional Trace+Result Bundle 原子 admission、状态迁移、Assignment/Attestation 与 lost-reply recovery |
| `verdictowner` | 双时钟 current snapshot、最短 TTL、Decide/CAS、Expire/Revoke/Supersede 与 exact recovery |
| `reviewer`、`policyrouter` | 只读 Reviewer Context、纯终止判断和基于风险/Effect/Policy 属性的安全路由 |
| `memory` | 并发安全 reference Store、Rubric/Case/Verdict append-only history/current index/highest revision 与 canonical snapshot |
| `storage/sqlite` | 单机 SQLite WAL production persistence、schema、generation CAS、重启恢复与完整性检查 |
| `service`、`api/http` | Review-owned facade、严格 JSON、鉴权/租户隔离、sealed cursor、SSE Watch |
| `sdk/go`、`cmd/*` | Go SDK、`praxis-review` CLI 与可启动的 `review-service` |
| `platform/{slack,linear,jira}` | 官方签名/时间窗校验、脱敏 Envelope exact binding、Observation/DeliveryIntent 协议面；不联网 |
| `runtimeadapter` | 只读 `OperationReviewCurrentReaderV4` 投影；不创建 Runtime 授权事实 |
| `production` | 显式依赖注入的Review-owned组合；不持有外部Owner写口、不替代宿主root |
| `resultgrounding` | Result Bundle V2全链exact-current S1/S2、full Owner router proof、min TTL与sealed aggregate |
| `releasecandidate` | 声明式 `ComponentReleaseV1` assembly-candidate；固定 `reference_only`，只暴露 Host factory descriptor |
| `conformance`、`tests` | reusable Store/Service 合同、unit/whitebox/blackbox/fault/runtime-integration、并发与 benchmark |

## 核心不变量

1. Review 只判定；Provider、Human、Auto 与外部平台输入先是 Observation/Attestation，正式 Verdict 只能由 Review Owner exact Inspect 后 CAS。
2. Auto Attestation 只接受 stored `State=applied` ApplySettlement，并 exact 复读 Reviewer Invocation Result；跨 Tenant/Case/Round/Assignment/Target/Attempt/ResultDigest 全部 Fail Closed。
3. Verdict Owner 不信 caller current；读取前后使用 baseline/fresh clock，重验全部 Review facts 与 external-current facts，TTL 取真实最短值。
4. 同一 Target identity 只有一个原子 Case 创建入口；revision 严格递增，历史 append-only，same revision 换 digest 与 staged failure 均零写。
5. 所有 mutation 的 lost reply 只 Inspect 原 canonical 对象，不重派、不换 idempotency、不重新 Decide。
6. Result Bundle 与 Request/Target/Case/Trace 同事务落盘；Behavior Feedback 只是带 Case/Target/Verdict/Policy/Reviewer/Finding provenance 的候选。
7. 平台 Envelope 必须绑定 Tenant、Envelope、Case、Target 的 exact immutable coordinate；平台回包不得升级为 Verdict，DeliveryIntent 不执行网络。
8. accepted Verdict 不能伪造 `operation_not_required`；Bypass/YOLO 的正式 Policy Fact 尚未闭合时保持 unsupported。
9. Rubric 的 criteria/rules/输出 Schema/只读 capability/终止 ceiling/version/revoke 归 Review Owner；Policy 只选 exact ref。Request Admission 对同一 Rubric 做 S1/S2 与 Store actual-point current 重验，漂移/撤销/TTL/clock rollback 全部零 Case 写入。

## 已完成的软件边界

- 单机 SQLite WAL 持久化、跨连接 generation CAS、重启恢复；
- SQLite public read/mutation 对 typed-nil Store 与 nil context 失败关闭；64 个独立 Store/连接共享同库时，CAS 仅一个 winner，历史与 current 均保持精确；
- 鉴权 HTTP/JSON、SSE、Go SDK、CLI 与服务进程；
- Human submit/inspect/list/watch/claim/attest/cancel/finding/feedback-candidate；
- Slack/Linear/Jira 协议解析、签名校验、重放时间窗和 outbound DeliveryIntent；
- 安全 Router、Result Bundle、Behavior Feedback Candidate；
- 内存 reference backend、SQLite backend、Service 与 Runtime Reader 的测试矩阵。

## 当前 NO-GO

- REV-D11 五类 production exact-current Reader：Binding、Evidence、Policy、Authority、Scope；
- production Verdict Decide 与宿主 composition root；
- Runtime/Application/Harness/Model/Context/Continuity production wiring 与公共 Review Gate；
- 真实 Auto Reviewer 模型调用、外部通知/Webhook Provider、平台 credential/identity admission root；
- Bypass/YOLO 独立 `operation_not_required` Policy Fact 与 Runtime 授权投影；
- Behavior Feedback 正式 Admission/Application、分布式 HA、备份策略与 SLA。

`memory.Store`、`internal/testkit` 和平台协议 fixture 不得冒充上述生产能力。

`releasecandidate` 也不得冒充 production：它只证明 Review descriptor 自洽，不证明 Decision/Verdict/Policy/Evidence/Authority/Scope external-current、remote Effect、Human intervention、cleanup 或宿主 composition root 已闭合。

## 资产与验证入口

- 设计：[README.md](../../design/review-engine/README.md)
- Production Service：[production-service-v1.md](../../design/review-engine/production-service-v1.md)
- 验收：[acceptance.md](../../design/review-engine/acceptance.md)
- Port Delta：[port-delta.md](../../design/review-engine/port-delta.md)
- 计划：[review-engine-v1.md](../../plan/review-engine/review-engine-v1.md)
- 代码：[ExecutionRuntime/review/README.md](../../../ExecutionRuntime/review/README.md)

本目录的验证命令与最后一次真实结果记录在最新 `memory/review-engine` 事件中；未实际运行的生产联调、Provider、HA 或 SLA 不作通过声明。
