# Review Engine Production Service V1

## 1. 已确认决策与结论

本切片由用户在 2026-07-17 明确确认：

1. 实现始终限制在 Review 独占目录；Runtime、Harness、Application、Model Invoker、Context、Continuity、Organization、Policy、Evidence 与其他 Owner 只提交 Port Delta，不跨目录补接口；
2. 五档 Profile 采用安全基线 v1，但 Router 按 capability/risk/effect/policy 属性判断，不维护未来 Tool ID allowlist，也不从 Tool 名称推导权限；
3. Review State Plane 首个真实持久实现采用 SQLite WAL；定位为单机、单 writer-domain 的 production Backend，不声明多节点一致性、高可用或 SLA；
4. 对外入口采用 HTTP/JSON、SSE Watch、Go SDK 与 CLI；实现 Slack、Linear、Jira 的签名/时间窗协议 Adapter，但 production Webhook identity/Authority admission root 尚未闭合。平台回包仍只是 Observation，不能直接写 Verdict；
5. Review-owned production service 可以启动、持久化、重启恢复并服务 Human/API 流程；五类 external-current Owner Reader 与宿主 composition root 未闭合前，Verdict Decide、Auto Provider、外部通知 Dispatch、Runtime Gate 继续 Fail Closed/NO-GO。

因此存在两个独立发布轴：

- `review-service`：Review 自有 SQLite/API/SDK/CLI/Adapter 软件闭环；
- `praxis-production-integration`：五类 current Owner、Runtime/Application/Harness/Model/Context/Continuity 接线与宿主 root。

第一个轴完成不能消除第二个轴的 P0，也不能生成 Runtime Authorization、Permit、Begin 或任何外部 Effect 成功事实。

## 2. Owner 与非 Owner

Review Owner 拥有 Request Candidate、Target、Case、Round、Assignment、Attestation、Finding、Condition、Condition Satisfaction、Verdict、Trace、Result Bundle 与 Behavior Feedback Candidate；拥有 HTTP 请求校验、幂等、分页/SSE Observer、平台 Observation 规范化以及 SQLite schema/migration/WAL 和 Review facts/history/current/index/idempotency/event cursor。

Review Owner 不拥有 Policy、Actor/Reviewer Authority、Scope、Binding、Evidence current Fact，Runtime Run/Outcome/Authorization/Permit/Begin/Fence/Settlement，Harness PhaseDecision/Session，Model Provider 调用、Credential、外部平台事实，Context Frame、Timeline sequence、Artifact/Memory/Knowledge Commit、Behavior Feedback 正式 Admission 或宿主 production composition root。

## 3. 安全基线 Router V1

### 3.1 输入

Router 只消费已验证、带版本和摘要的 Policy 输入：

- `Profile=restricted|standard|permissive|yolo|bapr`；
- `Risk=low|medium|high|critical`；
- `EffectClass=observe_only|reversible|persistent|irreversible`；
- `Environment=development|test|staging|production`；
- `HumanRequired`、`BypassAllowed`、`EvidenceSufficient`；
- Policy/Target/Authority/Scope exact refs 与 TTL。

Router 不接收 Tool 名称 allowlist，不自行授 Tool 权限。未来 Tool 只提交同样的 capability/risk/effect 声明即可进入矩阵；权限仍由 Authority/Binding/Sandbox/Gateway 独立判断。

### 3.2 输出矩阵

硬规则先于 Profile：

1. `HumanRequired=true`、`Risk=critical`、`EffectClass=irreversible`，或证据不足且策略不允许纯观察降级：Human；
2. Policy/Authority/Scope/Target/TTL 缺失或 unknown：Fail Closed，不产生 Route；
3. Bypass 只产生 `operation_not_required` 候选；不产生 accepted Verdict。

| Profile | low | medium | high | critical |
|---|---|---|---|---|
| restricted | Auto；persistent 为 Human | Human | Human | Human |
| standard | Auto；仅 observe-only 且显式 BypassAllowed 可 Bypass | Auto | Human | Human |
| permissive | 显式 BypassAllowed 可 Bypass，否则 Auto | Auto | Auto；irreversible 为 Human | Human |
| yolo | 显式 BypassAllowed 且非 HumanRequired/irreversible 才 Bypass，否则 Auto/Human | 同左 | Auto；irreversible 为 Human | Human |
| bapr | 仅 development/test 合法；显式 BypassAllowed 可 Bypass，否则 Auto | Auto | Auto；irreversible 为 Human | Human |

`bapr` 在 staging/production Fail Closed。任何 Router 输出只选择 Review Route，不授 Authority、Binding、Sandbox、Budget 或 Effect Dispatch。

## 4. Review-owned 新合同

### 4.1 Request 与 Delivery

`ReviewRequestV1` 是 create-once Candidate，包含 Fact identity、IdempotencyKey、exact Target ID/Revision/Digest、`Delivery=inline|detached`、requested Profile、Rubric Ref、Requester Ref、可选 Result Bundle Ref 与 TTL。非空 Result Bundle 必须随 Submit 提交并与 Request+Target+Case+Requested Trace 在同一 Owner 事务创建；缺失、换 ref 或 payload drift 零写。Submit API 不暴露 direct Verdict 写口。

### 4.2 Condition

`ReviewConditionV1` 与 `ConditionSatisfactionFactV1` 都绑定 Tenant/Case/Target exact identity。Satisfaction 只引用 Evidence exact refs；Conditional Verdict 只有全部 required Condition 当前且未过期时才能进入 Runtime 下游投影。Review 不创建 Evidence。

### 4.3 Human Envelope

`HumanReviewEnvelopeV1` 绑定 Case/Target exact refs、原始 Intent 摘要、影响范围、Diff/Artifact anchors、风险、Evidence、限制、允许的 Resolution 与 TTL。平台 Adapter 只获得脱敏 Envelope；修改任一字段产生新 digest。

### 4.4 Result Bundle

`ReviewResultBundleV1` 包含原始任务/验收摘要、Artifact exact refs、Claim、Claim→Evidence 映射、环境、验证范围、限制和未覆盖项。截图、录屏、测试和日志只作为带 source/revision/digest 的 Evidence ref。

### 4.5 Behavior Feedback Candidate

Review 只创建 `BehaviorFeedbackCandidateV1`，绑定 Target/Case/Verdict/Findings/Policy/Reviewer provenance 与 TTL。它不能直接修改 Policy、Context、Memory、Knowledge、Organization 或工作 Agent 行为；正式 Admission/Application 继续由相应 Owner Port Delta 管理。

## 5. SQLite WAL Backend

### 5.1 技术边界

- Go `database/sql` + CGo-free `modernc.org/sqlite`，版本固定在 Review `go.mod`，不修改 workspace/root；
- 启动执行 schema version/migration，强制读回 `journal_mode=WAL`、`foreign_keys=ON` 和 bounded busy timeout；
- 一个 SQLite 文件承载 Review Owner facts；路径、文件权限和备份由宿主配置；
- 每个 mutation 在单一 SQL transaction 中完成 validate/stage/history/current/index/idempotency/event cursor；失败零部分写；
- lost commit reply 不重放不同 mutation，只以既有 idempotency/exact refs Inspect；
- migration 失败不开放 writer，不自动 destructive downgrade；
- ctx cancel/deadline、busy/IO/commit unknown 保留 typed closed errors，不伪装 NotFound。

### 5.2 数据模型

首版以 tenant-scoped canonical owner snapshot + monotonic generation 实现同一 Owner 事务，避免把 SQLite row layout 变成公共合同：

```text
review_owner_state(
  tenant_id TEXT PRIMARY KEY,
  generation INTEGER NOT NULL,
  canonical_json BLOB NOT NULL,
  digest TEXT NOT NULL,
  updated_unix_nano INTEGER NOT NULL
)
review_schema(version INTEGER PRIMARY KEY, digest TEXT NOT NULL, applied_unix_nano INTEGER NOT NULL)
```

Snapshot 内含全部 exact history/current/index/idempotency/trace。每次 mutation：transaction 内读 generation+snapshot → Review state machine 全量 validate/stage → canonical seal → `UPDATE ... WHERE generation=?` → commit。不同 tenant 逻辑隔离；SQLite 物理 writer 串行不被表述为跨租户语义冲突。

该布局是单机 v1 的实现选择，可在保持 `ports.StoreV1` 与 Conformance 不变时迁移为细粒度表或其他 Backend。Snapshot 不是 API 响应、Evidence 或 Authority。

### 5.3 current/read 与多进程

- 每次 public read从已提交snapshot读取，不信进程内缓存；
- mutation使用generation CAS检测其他连接/进程写入；冲突返回typed Conflict，调用方重新Inspect；
- WAL只提供SQLite事务与读写并发，不声明分布式线性化、HA或exactly-once transport；
- `PRAGMA integrity_check`、migration、重启恢复、截断/损坏文件、disk-full/readonly/busy进入故障矩阵。

## 6. HTTP/JSON、SSE、SDK 与 CLI

### 6.1 API

路径固定在 `/v1/reviews`：

| 方法 | 路径 | 语义 |
|---|---|---|
| POST | `/v1/reviews` | Submit sealed Request/Target/Case/Trace；Admission/create-once |
| GET | `/v1/reviews/{tenant}/{case}` | Inspect current Case 与 exact Target/Verdict refs |
| GET | `/v1/reviews` | tenant-required、状态过滤、sealed cursor 分页 |
| GET | `/v1/reviews/{tenant}/{case}/events` | at-least-once Trace page |
| GET | `/v1/reviews/{tenant}/{case}/watch` | SSE；`Last-Event-ID`/cursor恢复 |
| POST | `/v1/reviews/{tenant}/{case}/claim` | expected-revision Assignment Lease CAS |
| POST | `/v1/reviews/{tenant}/{case}/attestations` | 记录 Human/Auto/Adapter Observation，不direct Verdict |
| POST | `/v1/reviews/{tenant}/{case}/cancel` | Owner CAS cancel intent，不取消 Runtime Run |
| POST | `/v1/reviews/{tenant}/{case}/findings` | 当前 claimed Reviewer Lease 下创建 exact Finding；不修改 Candidate |
| POST | `/v1/reviews/{tenant}/{case}/behavior-feedback-candidates` | 创建 exact provenance 候选；不执行正式反馈 Admission/Application |

严格 JSON拒绝body超限、未知字段、顶层/嵌套 duplicate key和trailing token。错误返回稳定`category/reason/message/request_id`；message不用于客户端分支。鉴权由宿主传入经过验证的`Principal`；无production Authenticator时server构造失败。

### 6.2 SDK/CLI

- Go SDK只依赖HTTP public DTO，支持Submit/Get/List/Watch/Claim/Attest/Cancel；
- CLI命令`list/show/watch/approve/deny/request-changes/cancel`全部调用SDK；approve/deny/request-changes生成Attestation request；
- token/secret只从进程环境或宿主Secret provider读取，不写入argv、日志、Trace或SQLite snapshot。

## 7. Slack、Linear、Jira Adapter

三类Adapter具有同一窄合同：

```text
HumanReviewEnvelope -> PlatformDeliveryIntent
Platform raw webhook -> signature/timestamp validation
                     -> exact-bound PlatformReviewObservation
                     -> host identity/Authority admission (当前 NO-GO)
                     -> Review Attestation
```

- Adapter只负责canonical request/response映射、签名验证、source event ID/sequence、identity handle、Case/Target envelope ref和幂等；入站解析必须携带宿主从已持久 Delivery 映射出的 immutable `EnvelopeBindingV1{Tenant,Envelope ID/Digest,Revision,Case exact,Target exact}`，raw provider payload不能另选Case/Target；
- raw payload、按钮、评论、Issue/Ticket status永远不能成为Verdict；
- outbound网络、Credential和provider call必须经宿主governed Effect；Review Adapter只生成sealed DeliveryIntent并解析exact Observation，不直接`http.Client.Do`；
- inbound webhook只有验签、timestamp/replay、tenant mapping和Authority Reader全部成功才提交Attestation；
- Slack、Linear、Jira source identities nominal独立，禁止type-pun；
- 平台API变化通过独立adapter contract version迁移，不改Review Case/Verdict历史。

真实账号/凭据未提供时，只能完成官方协议fixture、签名golden、mock provider和恶意payload测试，不能声称真实租户联调通过。

## 8. Effect、恢复与残余

| 动作 | Effect | 实际执行点 | Unknown 恢复 |
|---|---|---|---|
| HTTP Submit/Get/List/Watch | 入站服务/只读或领域mutation | Review Owner | mutation lost reply exact Inspect；read可同canonical恢复 |
| SQLite commit | Review State Plane mutation | SQLite transaction commit | generation/idempotency/exact ref Inspect |
| 平台通知 | external-human-delivery | 宿主Gateway+Provider | Begin后只Inspect原delivery attempt |
| 平台webhook | inbound Observation | Adapter验签已实现；identity/Authority admission root未闭合 | 当前只形成exact-bound Observation；production admission前Fail Closed |
| Auto Reviewer | reviewer-invocation | Model Invoker governed route | Begin后只Inspect原attempt；无公共route/root时unsupported |

## 9. Import DAG

```text
contract <- ports <- caseengine/verdictowner/policyrouter
contract <- storage/sqlite (implements ports.StoreV1)
contract <- service <- api/http <- sdk/go <- cmd/praxis-review
contract <- platform/{slack,linear,jira}
runtime/core + runtime/ports <- runtimeadapter only
```

Review production package不得导入Runtime `control/kernel/foundation/fakes`、Harness/Application/Model/Context/Continuity实现包。平台包不得依赖HTTP server或SQLite concrete。CLI不得导入Store/Owner写口。

## 10. 验收与发布真值

Review service可标记YES需要SQLite、HTTP、SDK/CLI、三平台Adapter、安全Router、Conformance/benchmark、ordinary/race/vet/import和独立Review全部通过。当前代码与Owner自测完成后只进入“awaiting independent review”，不能自行标记最终YES。

即使service YES，REV-D11五类current Reader、跨组件adapter/root、actual outbound Provider、Auto Model、external tenant credential联调、Operation Authorization/Permit/Begin和Behavior Feedback正式Admission/Application仍保持NO-GO。
