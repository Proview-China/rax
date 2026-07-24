# Review Engine

Review Engine 是 Praxis 的判断 Owner：它维护 Request、Target、Case、Round、Assignment、Attestation、Finding、Verdict、Trace、Result Bundle 和 Behavior Feedback Candidate；它不 Dispatch、不 Commit，也不写 Runtime Outcome、Authorization、Permit 或 Begin。

## 已实现

- Target/Case 的唯一原子创建入口、严格 revision 单调性、supersede 与 append-only exact history；
- Case/Round/Assignment/Attestation/Finding/Verdict 状态机，create-once、CAS、幂等、并发与 lost-reply exact Inspect；
- Human/Auto Attestation，其中 Auto 只接受已应用的 Domain ApplySettlement，并 exact 复读 Reviewer Invocation Result；
- Verdict Owner 的双时钟、完整 current snapshot、最短 TTL、撤销/过期/替代和 Runtime V4 只读投影；
- restricted/standard/permissive/yolo/bapr 安全 Router；Router 按风险、Effect、环境和已验证 Policy 属性工作，不按 Tool ID 授权；
- Result Bundle 的 Claim→Artifact→Evidence 精确绑定，以及和 Request/Target/Case/Trace 同事务持久化；
- Result Bundle V2 的单向 `Bundle -> exact Request/Target` 绑定、无孤儿写口的复合 admission、immutable exact history，以及 Context/Artifact/Environment/Validation Scope/Evidence/Owner Binding 全链 S1→S2、真实最短 TTL、逐 Owner 双时钟与 sealed aggregate；
- Review.md 十类留痕事件；`review_started`、`finding_observed`、`escalated`、`resolved` 与对应 Claim/Finding/Attestation/Decide 领域事实同事务全有全无；窄 `TraceEventReaderV2` 提供 exact Inspect 和深拷贝分页，HTTP/SDK/CLI 复用同一 Reader；
- production `Service`/HTTP/SDK、`caseengine.Engine`、Auto Attestation Owner 与 Verdict Owner 不允许 eventless 旁路：`StoreV1` 与 SQLite concrete Store 均不暴露 `CompareAndSwapCaseV1`、`CreateFindingV1` 或 `AppendTraceV1`；Claim/Attestation/Decide 只接受对应 exactly-one 原子事件闭包，memory 中的冲突注入与 Case 前置构造 seam 仅供 reference/test；
- Behavior Feedback Candidate 的 Case/Target/Verdict/Policy/Reviewer/Finding provenance 校验与持久化；正式反馈 Admission/Application 不属于 Review；
- Review-owned `RubricDefinitionV1`：action safety、code change、work state、artifact quality、outcome acceptance、legal compliance、finance control 七类结构化 baseline，独立 criteria/rules/Attestation输出Schema/只读capability/终止ceiling；append-only/current/revoke/supersede 与 Request Admission S1/S2；
- 单机 SQLite WAL Backend：migration、generation CAS、严格 snapshot、重启恢复、完整性检查和多连接并发；
- 鉴权 HTTP/JSON、sealed cursor 分页、SSE Watch、Go SDK、`praxis-review` CLI 和可启动的 `review-service`；
- Slack、Linear、Jira 官方签名/时间窗解析与脱敏 Envelope exact binding；平台回包始终只是 Observation，outbound 只生成 DeliveryIntent，不调用网络；
- reusable Store/Service conformance、unit/whitebox/blackbox/fault/runtime integration、race/vet 和 benchmark。

## 公开入口

- `contract`：Review 领域事实、canonical digest 与 currentness；
- `ports`：Review Owner Store、Rubric Owner-only publish/revoke/current Reader、exact Inspect/CAS 和 query；
- `caseengine`、`verdictowner`：Owner 状态机；
- `policyrouter`、`reviewer`：纯路由、只读 Reviewer Context 与终止判断；
- `storage/sqlite`：单机 production persistence；`memory` 仅为 reference/test；
- `service`、`api/http`、`sdk/go`、`cmd/*`：Review-owned 服务面；
- `platform/{slack,linear,jira}`：协议 Adapter，无 Provider 执行点；
- `runtimeadapter`：只读 `OperationReviewCurrentReaderV4`，不创建 Runtime 授权事实；
- `production`：Review-owned composition，要求宿主显式注入真实公开 Owner Reader/Auto invocation；不创建跨组件 host root；
- `conformance`：可复用 Store/Service、Auto Reviewer、Condition、Result Grounding 正反合同检查。
- `releasecandidate`：发布完整的声明式 `ComponentReleaseV1` assembly-candidate；固定 `reference_only`，Host 只得到 Factory descriptor，不得到反向 import 或隐式 root。

## 当前 NO-GO

- Binding、Evidence、Policy、Authority、Scope 与 Result Grounding 外部 Owner 的生产 adapter/certification，以及宿主 composition root；Review 只持公开 Reader，不伪造这些 Owner 的 current；
- 公共 Review Gate、Application/Harness/Model/Context 的 production composition；
- Continuity typed Trace projection/Reader 已有 owner-local/test 闭环；Runtime Evidence 的 tenant+full-ref exact admission与宿主注册仍未闭合，Review Trace 不伪造成 Evidence；
- Bypass/YOLO 的独立 `operation_not_required` Policy Fact 与 Runtime 授权投影；
- 真实 Auto Reviewer 模型调用、外部通知 Provider、Webhook identity/Authority admission root 与真实 Slack/Linear/Jira 租户联调；
- Behavior Feedback 正式 Admission/Application、跨组件 Commit、分布式 HA、备份策略或 SLA。

这些缺口不会被 test fake、平台评论、accepted Verdict 或 SQLite snapshot 冒充。Review-owned 单机服务完成不等于 Praxis production integration 完成。

`releasecandidate` 即使完成 Manifest/Module/Capability/Port/Factory/owners/artifact/candidate-certification/evidence/TTL 闭合，也不会把上述 NO-GO 提升为 production；强改 `support_mode=production` 必须失败关闭。

## 本模块验证

从本目录运行：

```bash
go test ./...
go test -race ./...
go vet ./...
go test -bench 'SQLite|Router|HTTP' -benchmem -count=3 ./...
```
