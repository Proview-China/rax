# Review Engine v1 可实施计划

## 1. 状态、范围与约束

- 状态：**Review owner-local/reference/test 非完整生产闭环最终独立复审 YES（P0/P1/P2=0）；Praxis production integration 仍 NO-GO**。
- REV-D11当前真值：Review-owned external-current aggregate与显式依赖注入组合已写Go并验收；真实五类Owner production adapter/certification及宿主root仍由对应Owner/host关闭，Review不提供替代写口或假current。
- 当前实现独占落点：`ExecutionRuntime/review/**`；允许实现Review自有SQLite/API/SDK/CLI/平台协议Adapter，不创建跨组件production composition root。
- 默认语言：Go。当前无经基准证明的计算稠密热点，不规划 Rust。
- 只依赖 Review 自身合同与 Runtime/Application/Harness/Model Invoker 的公开版本化 Port；禁止导入任何 `internal`、Runtime foundation/fakes/kernel 实现或 Harness kernel/fakes。
- 用户已选择SQLite WAL与HTTP/JSON+SSE，并要求Slack/Linear/Jira协议Adapter；仍不预选消息队列、跨组件进程拓扑或SLA。
- Review 只判定，不 Dispatch、不 Commit、不写 Runtime Outcome/Binding/Policy/Trust。

## 2. 实施前置依赖

### 2.1 必须联合关闭的 Review Port Delta

| Delta | 阶段门禁 | Owner/参与方 |
|---|---|---|
| REV-D1 通用 Target | Artifact/Outcome/Detached Target 前 | Review + 各 Target Owner + Runtime |
| REV-D2 正交 Route/Delivery/Profile | Router/Assignment 前 | Review + Policy + Application |
| REV-D3 独立 PolicyNotRequired | Bypass 发布前 | Review + Policy + Runtime |
| REV-D4 Attestation/Finding/Trace | Human/Auto Adapter 前 | Review + Evidence/Identity |
| REV-D5 Resolution 映射 | Gate/SDK 前 | Review + Application/Harness + 管理线 |
| REV-D6 Invocation/Assignment/Unknown | Auto/Human 外部 Effect 前 | Review + Runtime + Model Invoker |
| REV-D7 Operation Review Current Projection V4 | 只读 adapter 已实现；生产 source/后续授权链另审 | Runtime Owner + Review |
| REV-D8 持久 Review Gate | Inline/Detached 集成前 | Application + Harness + Review |
| REV-D9 Behavior Feedback | 正式反馈资产应用前 | 管理线 + Policy/Context/Organization |
| REV-D10 Evidence 精确引用 | Verdict 发布前 | Evidence Owner + Review + Runtime |
| REV-D11 Verdict Decide production current Reader | Review aggregate/Decision组合已实现；真实Owner adapters/certification与host root仍关闭production | Binding + Evidence + Policy/Authority/Scope Owners + Review |
| REV-D12 Condition admissibility current Reader | Review exact Condition set、Human/Auto/V4/V5投影已实现；Policy tuple current与host root未闭合 | Runtime public ports + Policy/Binding/Authority + Review + host composition |
| REV-D13 Result Bundle Current Grounding V2 | Review contract/atomic Store/full aggregate/conformance已实现并独立复审0/0/0；真实source Owner adapters/certification与typed root未闭合 | Artifact/Environment/Validation Scope/Context/Evidence/Binding Owners + Review + host composition |
| REV-D14 Detached Delivery V1 | 独立ReviewRun/外部Thread production前；当前仅Review Binding/Closure候选资产 | Runtime Run + Application + Human delivery + Harness + Review + Agent Host |

### 2.2 公共装配硬依赖

以下由 Harness 接线方向统一，不允许 Review 自建替代品：

- Agent Assembler 最终输出与 ResolvedAgentPlan；
- Assembly SDK / CompiledHarnessGraph；
- namespaced/versioned Slot/Phase 合并、排序、冲突与 Digest 规则；
- Binding V2 到 RuntimeProviderBinding 的映射；
- Checkpoint、Action Gateway、per-turn refresh 接线。

Review 只声明：`review.gate` Port，`action.review`、`subagent.completion.validate`、`run.completion.validate` Gate，以及 `residual.detected` Observer。

### 2.3 管理线门禁

实现对应能力前必须冻结：

1. v1 五档 Profile 的精确 Human/Auto/Bypass 路由矩阵与不可 Bypass 清单；
2. Rubric payload/version/schema/revoke 已冻结归 Review Owner；Policy 只选择 exact ref/适用条件。业务域 publisher 资格仍需由宿主管理策略配置，不得建立第二 Owner；
3. Request Changes、Escalate Human、Insufficient Evidence、Conditional 的 Case/Gate 映射；
4. Human 自审、委托、多签、quorum、职责分离和 Authority 撤销；
5. Round/token/time/cost/重复 finding/重复拒绝的终止阈值与升级目标；
6. Behavior Feedback 正式资产 Owner、Admission、TTL、申诉、隔离和后续应用；
7. 外部 Adapter 脱敏、通知责任、身份映射与 Lease 冲突规则。

## 3. 依赖 DAG

```text
管理线语义裁决 ───────────────┐
Review Port Delta ─────────────┼─> 领域合同与纯状态机
Runtime 公开 V2/V3 合同 ──────┘          │
                                         ├─> Human / Auto / Bypass
Model Invoker public RouteID ────────────┘          │
                                                    ├─> Review API / Go SDK / CLI Adapter
Harness 公共装配 + Binding V2 ──────────────────────┤
Application 持久 Gate + Runtime current projection ─┴─> 生产集成与系统验收
```

任何上游未关闭时，只推进不依赖该项的纯领域包；不得用私有 Port、fixture 手工授权或 legacy boolean 越过门禁。

## 4. 文件级落点与 Wave 1 状态

以下只描述 Review 独占范围。`implemented` 表示 reference/test implementation 已落地，不等于生产接线完成：

| 路径 | 责任 | 依赖门禁 |
|---|---|---|
| `ExecutionRuntime/review/go.mod` | 独立 Go module 边界 | implemented |
| `ExecutionRuntime/review/README.md`、`doc.go` | Owner/非 Owner、公开入口、限制 | implemented，production unsupported 已标注 |
| `ExecutionRuntime/review/contract/*.go` | Review 领域对象、精确绑定、Validate、Digest、Decision current snapshot | implemented |
| `ExecutionRuntime/review/ports/*.go` | Review-owned Store/Decision current Reader Port | implemented；REV-D11 production adapter 未闭合 |
| `ExecutionRuntime/review/caseengine/*.go` | Case/Round/Assignment/Attestation 状态机与 CAS 命令 | implemented |
| `ExecutionRuntime/review/verdictowner/*.go` | Owner exact Inspect、Decide/CAS、撤销/过期/替代、reply-loss recovery | implemented，依赖注入的 production current Reader unsupported |
| `ExecutionRuntime/review/policyrouter/*.go` | Route/Delivery/Profile/Bypass 决策消费 | 管理线矩阵 + REV-D2/D3 |
| `ExecutionRuntime/review/reviewer/human/*.go` | Envelope、Assignment、Attestation Adapter | REV-D4/D6 + Identity/Authority |
| `ExecutionRuntime/review/reviewer/auto/*.go` | ContextFrame、RouteID 调用、终止器、Observation | REV-D6 + Model Invoker public union |
| `ExecutionRuntime/review/runtimeadapter/*.go` | rich facts -> Runtime V4 只读 current projection | implemented；不创建 Authorization/Permit/Begin；operation_not_required unsupported |
| `ExecutionRuntime/review/applicationadapter/*.go` | 持久 wait/resume 与 PhaseDecision 投影 | REV-D5/D8 + 公共装配 |
| `ExecutionRuntime/review/sdk/go/*.go` | transport-neutral Go SDK | 外部 API schema 冻结 |
| `ExecutionRuntime/review/cmd/praxis-review/*.go` | CLI Adapter；是否并入全局 CLI 另行评审 | SDK + CLI 命令归属裁决 |
| `ExecutionRuntime/review/conformance/*.go` | reusable Store Target+Case(+Trace) create/replay、Case CAS 与 exact Case history 子集 | implemented；Owner/lost-reply/settlement 属普通测试覆盖，不宣称 reusable conformance |
| `ExecutionRuntime/review/conformance/external_current_reader_v1.go` | REV-D11 immutable projection identity/current-index/history、exact Ref/Subject、fixed Checked/Expires/ProjectionDigest、ValidateCurrent、S1/S2 full compare、TTL、closed errors、lost-reply/clock rollback reusable suite | 仅设计；Owner公共Port冻结并获Review纯测试实现授权后才可创建 |
| `ExecutionRuntime/review/conformance/external_current_reader_v1_benchmark_test.go` | REV-D11 Evidence规模、聚合、并发、失败路径Go基线 | 仅设计；不设SLA、不预选Rust/拓扑 |
| `ExecutionRuntime/review/internal/testkit/*.go` | 仅测试 Fake、故障注入、确定性时钟 | implemented，仅测试，不对生产导出 |
| `ExecutionRuntime/review/policyrouter/*.go` | 基于Profile/risk/effect/policy的安全Route；不按Tool ID授权限 | implemented |
| `ExecutionRuntime/review/storage/sqlite/*.go` | SQLite WAL、migration、tenant snapshot generation CAS、重启恢复 | implemented；单机，无HA/SLA声明 |
| `ExecutionRuntime/review/service/*.go` | Submit/Inspect/List/Watch/Claim/Attest/Cancel/Finding/Behavior Candidate facade | implemented；Decide仍要求external current Reader |
| `ExecutionRuntime/review/api/http/*.go` | strict HTTP/JSON、SSE、鉴权、typed errors | implemented；真实Webhook admission root仍NO-GO |
| `ExecutionRuntime/review/sdk/go/*.go` | HTTP Go SDK | implemented |
| `ExecutionRuntime/review/cmd/{praxis-review,review-service}/*.go` | SDK-only CLI与Review独立服务进程 | implemented；不并入根CLI/root |
| `ExecutionRuntime/review/platform/{contract,slack,linear,jira}/*.go` | sealed delivery intent、验签/时间窗、immutable Envelope binding、Observation映射 | implemented协议面；禁止直接网络Dispatch/写Verdict |
| `ExecutionRuntime/review/contract/{auto_reviewer_v1,reviewer,verdict}.go`及Condition helper | V1 `Conditions,omitempty`、canonical exact set、legacy/strict分层 | Review-owned Condition V2已实现；production Policy tuple/root仍NO-GO |
| `ExecutionRuntime/review/autoattestation`、`service`、`verdictowner`、`runtimeadapter` | Auto/Human/Verdict exact set无损链、REV-D12 S1/S2、legacy零授权 | REV-D12及单独Go授权前NO-GO |
| `ExecutionRuntime/review/tests`、`conformance` | CND-01..61与Policy publisher/Owner consumer conformance | 仅设计；不得以fake替代Policy/Binding/Authority生产证据 |
| `ExecutionRuntime/review/contract/result_v2.go`、`ports/result_grounding_v2.go`、Store/SQLite/Verdict Owner | typed Bundle/Claim、Context exact intent/criteria、三stable identity、Validation Scope unique Owner association、external+Owner Binding S1/S2、exact typed router、true min TTL、bounded recovery、legacy隔离；Bundle无current index | REV-D13及单独Go授权前NO-GO；计划见`result-bundle-current-grounding-v2.md` |
| `ExecutionRuntime/review/tests`、未来Owner conformance/root integration | RB-REV/RB-OWN/RB-ROOT矩阵 | 仅设计；不得以Sandbox/Continuity type-pun或fake消除门禁 |

若仓库已有统一 SDK/CLI 归属要求，上述 `sdk/go` 与 `cmd` 路径在实现前服从联合评审；不得越权修改其他目录。

## 5. 分阶段实施

### 阶段 0：联合合同冻结

工作：逐项裁决 REV-D1-D10、管理线问题和公共装配对象；形成版本化 schema、Owner 与兼容结论。

验收：

- 每个 Delta 有 Owner、输入输出、不变量、Effect/Recovery、反例和兼容结论；
- 明确 v1 支持/不支持的 Target、Route、Delivery、Profile 与 Resolution；
- 明确无 pre-run Evidence；
- 无 Review 私建 Runtime/Harness 兼容接口。

### 阶段 1：合同与纯领域状态机

工作：实现 contract、canonical digest、Validate、Case/Round/Assignment/Attestation/Finding/Condition/Verdict/Trace 状态机。

验收：单元测试覆盖合法迁移和全部反例；Target immutable；Revision 单调；同源同序换内容冲突；终态授权不复活。

### 阶段 2：State Plane Owner、Inspect/CAS 与恢复

工作：实现 create-once Request/Case/Attestation、expected-revision CAS、Decide、Expire/Revoke/Supersede、Watch、Cleanup/Residual 索引；存储仅以 Port 抽象，不预选生产 Backend。

验收：并发 Decide 单一胜者；reply-loss Inspect 能恢复；重复事件幂等；Store unknown Fail Closed；Fake 只用于测试。

### 阶段 3：Human Gate

工作：实现 Envelope、Assignment Lease、Claim、Attestation、迟到响应和外部 Adapter 边界；UI/平台不在组件内实现。

验收：身份/Authority/Case/Target/TTL 当前性复读；外部评论不能直接写 Verdict；进程重启恢复同一 Case；脱敏字段有明确策略。

### 阶段 4：Auto Reviewer fork

工作：实现只读 ContextFrame、Rubric/Schema、公开 RouteID 调用、Invocation Effect/Receipt/Observation/Settlement 关联、终止预算和 Human 升级。

验收：无工作 Agent persona/写权/Dispatch/Commit/Spawn；Model completed 只是 Observation；ContextReference 不可物化时 Fail Closed 或记录批准的 Residual；Unknown 只 Inspect 原 attempt。

### 阶段 5：Bypass 与 Policy Router

工作：消费已发布 Policy，产生独立 PolicyNotRequired 事实；实现 Human/Auto/Bypass 与 Inline/Detached 正交路由。

验收：无虚假 accepted；不可 Bypass 清单生效；Policy/Target/Authority/Scope/TTL 漂移立即失效；其他 Gateway 门禁仍全量执行。

### 阶段 6：Review API、Go SDK 与 CLI Adapter

工作：提供 Submit/Get/List/Watch/Claim/Attach/Attest/RequestChanges/Cancel；CLI 只调用 SDK。

验收：无 direct WriteVerdict；transport-neutral；幂等/冲突/分页/Watch 重复语义明确；不预选生产 RPC 或外部平台。

### 阶段 7：Runtime/Application/Harness 集成

工作：在 REV-D7/D8 和公共装配关闭后，发布 Review Manifest/Contribution、Operation V3 current projection、Application wait/resume 与 Harness Gate 接线。

验收：

- 不依赖 Harness 私有 Port；不新增 Harness Review 状态；
- 不手工构造生产 `OperationReviewAuthorizationV3`；
- pending/reject/expired/revoked/drift/unsatisfied 均无法触达 Execute；
- Provider Prepare/Execute 二次复读 Review/Fence/Authority/Scope/Budget；
- Review PhaseReceipt 不替代 Verdict，Verdict 不替代 Runtime Settlement/Outcome。

### 阶段 8：硬化、Conformance 与交付资产

工作：完成单元/白盒/黑盒/故障注入/Conformance/race/vet/集成/系统测试，回读公开文档，并在用户授权下同步 module/memory。

验收：测试证据可复查；所有未实现能力明确 unsupported；无跨组件实现导入；无生产 SLA/Fake 声称；module/memory 同步必须另有授权。

### 阶段 9：Review-owned production service（2026-07-17已授权）

工作：按`production-service-v1.md`实现安全Router、Review-owned补充合同、SQLite WAL Backend、HTTP/JSON+SSE、Go SDK/CLI、Slack/Linear/Jira协议Adapter和对应Conformance/benchmark。

验收：SQLite migration/WAL/generation CAS与重启/故障反例通过；HTTP strict JSON、tenant隔离、幂等、分页、SSE恢复通过；CLI只依赖SDK，平台raw输入只形成Observation且outbound只形成DeliveryIntent；Router不按Tool ID授权；service YES与external current/root NO-GO分别同步。

### REV-D11 production external current Reader 切片

Review-owned asset已完成最终复核，以下内容保持冻结：

1. Review-owned request/projection继续复用live `DecisionExternalCurrentReaderV1` seam；
2. Binding full六字段ref与Evidence full三字段ref/OwnerFact全字段逐项核验；每个Owner public projection还必须有immutable ProjectionID/Revision、exact Ref/Subject、fixed Checked/Expires/ProjectionDigest与`ValidateCurrent`；
3. 冻结`Owner状态变化create-once/seal+CAS current index -> baseline -> S1按full ExactSource+ExactSubject线性化resolve完整ProjectionRef并exact Inspect -> S2只按保存Ref复读并验证index -> fresh now ValidateCurrent -> min TTL -> aggregate seal`；禁止caller/stored预带ref、by-name/latest、追随新Ref与ABA；
4. 冻结closed Category/Reason、lost reply单次exact recovery、ctx取消隔离、clock rollback与TTL crossing；
5. 冻结reusable conformance fixture必须只读、无Owner write Port，benchmark只测Go聚合成本且无SLA。

Binding候选历史保留在 [`rev-d11-binding-authoritative-current-port-delta-v1.md`](../../design/review-engine/rev-d11-binding-authoritative-current-port-delta-v1.md)：Runtime Binding Owner public nominal包含Review消费Reader、Owner-only首建/CAS Publisher及host consumer association Reader。Review-owned aggregate已经实现并通过最终独立复审；BIND-01..28仍是实际Runtime Binding production Adapter/certification与host root的外部门禁，不构成跨Owner实现授权。

本轮未创建上述Go suite/benchmark，也未修改任何Go。Binding、Evidence、Policy、Authority、Scope五类production exact-current公共合同与production root仍NO-GO。停止扩设计，等待联合实施门；任何实现仍须另行授权。

#### live依赖关闭清单（不构成实施授权）

精确字段、Reader与SHA-256快照以 [REV-D11外部Owner live依赖盘点](../../design/review-engine/rev-d11-external-owner-live-inventory.md) 为准。关闭顺序只用于判断准入，不扩展REV-D11语义：

| 依赖 | 当前可复用 | 关闭动作 | 准入证据 | 状态 |
|---|---|---|---|---|
| Binding Owner | full `ReviewComponentBindingRefV2`；Provider consumer current模式；Review第四候选exact shape | Runtime Owner冻结Reader+Owner-only Publisher+association；atomic publish；S1/S2复读closure/association/consumer；禁止Raw Fact public | BIND-01..28 ordinary100/race20/full/race/vet+双独立0/0/0+指纹回读 | NO-GO / P0=1；第三候选NO 2/3/1，第四候选READY |
| Evidence Owner | full `ReviewEvidenceRefV2`与`EvidenceOwnerFactRefV2` | 冻结applicability映射、nominal sealed projection和exact Reader | N项/跨域/TTL/ABA/lost-reply conformance+独立0/0/0+指纹回读 | NO-GO |
| Policy Owner | `ReviewPolicyFactV2`与`ValidateCurrent` | 冻结Target/Run/Scope exact Subject、sealed current projection和exact Reader | drift/revoke/TTL/history conformance+独立0/0/0+指纹回读 | NO-GO |
| Authority Owner | authority Fact/Binding；V3 governance ref模式 | 分别冻结Actor/Reviewer role-aware nominal projection和exact Reader | role/type-pun/跨域/TTL conformance+独立0/0/0+指纹回读 | NO-GO |
| Scope Owner | execution-scope Fact/Binding与`ValidateCurrent` | 冻结Review-decision scope projection、exact Subject/index/Inspect | scope/run/capability/TTL/history conformance+独立0/0/0+指纹回读 | NO-GO |
| Review conformance/root | Review aggregate seam；Store conformance子集 | 五Owner关闭后另行授权reusable suite；最后由composition Owner注入只读capability | Review targeted100/race20/full/race/vet/import/root integration+独立0/0/0+用户授权 | NO-GO |

任一Owner public文件指纹变化、缺nominal类型、缺linearizable current index、缺immutable projection、缺closed errors或缺独立审计时，回到资产盘点，不进入Go实现。不得用现有weak Reader、`memory`、`internal/testkit`、Runtime ReaderV4或私有兼容接口补洞。

独立复核后的双轴计数不得合并：上表对应production准入`P0=6`，不降低Review-owned盘点/冻结资产`P0/P1/P2=0`结论；也不得用资产0/0/0消除六项production P0。原五Owner盘点九个live指纹与Binding第三候选十个live输入均已复核一致，仓内没有可替代这些缺口的第二production仓。固定状态继续为：**Review-owned asset P0/P1/P2=0；五类Owner/production NO-GO**。

### Wave 1 修复收口（本轮）

- Auto Attestation：stored ApplySettlement 必须 `applied`，并 exact 复读 ReviewerInvocationResult 全链；
- Verdict currentness：单次 Review-owned snapshot、fresh clock、全部输入最短 TTL；
- 唯一性与历史：Target-to-Case create-once、显式 supersede、Case/Verdict append-only revision history；
- exact binding：Round/Assignment/Attestation/Finding/Trace/Verdict 全链绑定 Target，Verdict 绑定 Reviewer；
- recovery：Attestation/Decide/Invalidate lost reply 只 exact Inspect；复合写 staged failure 零 current/history 泄漏；
- Store shape：删除 public/concrete Case-only create，Case 创建只允许 `CreateTargetCaseV1` 原子维护 Target 唯一/current 索引；
- Target history：same revision换digest与revision rollback均在写前Conflict，旧 exact Target不可覆盖；新 revision严格递增；
- Create 原子性：Target+Case+optional Requested Trace同锁、同线性化点发布；Trace冲突三者零写，lost reply exact Inspect原Target/Case/Trace；
- Decide clock：在 Decision current Reader 前后取 baseline/fresh now，读期间回拨一律在 Verdict/CAS 前失败关闭；
- P2 currentness：Auto cross-tenant和Reviewer三类漂移zero write；Policy/三类治理ref/Binding/每项Evidence独立最短TTL与actual-point expiry逐项测试；
- Runtime read-only：V4 projection 的 Inspect/retry TTL crossing 与 clock rollback Fail Closed，`operation_not_required` 不由 accepted Verdict 伪造；
- production 残余：REV-D11五类current Reader、Application/Harness/Model/Context/Continuity adapters、真实外部Provider/Webhook identity admission与production composition root保持 unsupported；SQLite WAL Backend已实现，不再列为残余。
- RunID 已由 Target+Policy exact 绑定；本轮不新增 TurnID，Turn 语义没有获得额外实现结论。

## 6. 固定外部动作顺序

所有有外部影响的 Reviewer 调用、通知、拉取、轮询和清理必须实现为：

```text
Domain Intent/Reservation
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

Begin 后不确定只能 Inspect 原 attempt。Review 组件不改写此顺序，也不把 Reviewer Observation、Model stream/completed/cache/provider 状态升级为 Verdict 或 Runtime Outcome。

## 7. Effect、Conflict Domain 与 Operation Subject / Scope

| Effect kind | Conflict Domain | Operation Subject / Scope | Unknown/Cleanup |
|---|---|---|---|
| reviewer-invocation | tenant + Case + Round + attempt | 父 Run 或 ReviewRun | Inspect original attempt；远程资源独立 Cleanup/Residual |
| readonly-exploration/validation | tenant + Case + ContextFrame + operation | ReviewRun；受限本地可父 Run | 只读仍经门禁；无法物化 Fail Closed/Residual |
| external-human-delivery | tenant + Case + Adapter + message key | 父 Run 或 ReviewRun | at-least-once；Inspect delivery；脱敏残留 |
| attachment-fetch | tenant + Case + attachment ref | ReviewRun | Observation 后由 Evidence Owner Admission |
| webhook-poll | tenant + Case + Adapter + event id | 当前父 Run 或 ReviewRun | source sequence 去重；Inspect exact event/attempt |
| remote-cancel/cleanup | 原 invocation conflict domain | 原 ReviewRun | 只控制资源，不改变历史 Verdict/Observation |
| unknown-attempt-inspect | 原 attempt conflict domain | 原 Run | 禁止衍生新 attempt；Inspection 自身结算 |

Review Case/Attestation/Verdict 的 State Plane create-once/CAS 使用领域 Fact Port；reply-loss 同样 Inspect，但不伪装成 Provider Effect 成功。

## 8. Settlement、Cleanup 与 Residual

- Reviewer invocation 的Runtime Operation Settlement只结算该调用是否已被权威处理；Review ApplySettlement决定领域结果是否应用，但两者都不直接替代Verdict CAS；
- Review Participant Settlement 引用 Case/Verdict/Trace，不写 Runtime Outcome；
- Application 聚齐 Review、Effect、Participant、Cleanup Settlement 后，请求 Runtime 收口；
- 远程会话、临时 Snapshot、Attachment、通知、Webhook 游标各自记录 Cleanup 与 Residual；
- Cleanup 成功需独立 Evidence；成功 Verdict 不能掩盖 Residual；
- ContextReference 无法物化时，执行类路径 Fail Closed；仅观察且策略允许时记录 Residual。

## 9. 测试与验证矩阵

| 层级 | 文件/套件计划 | 场景 | 命令口径 |
|---|---|---|---|
| 单元 | 各 package `*_test.go` | digest、Validate、状态迁移、TTL、Condition、Router、终止器 | `go test ./...` |
| 白盒 | `caseengine`、`verdictowner` internal tests | CAS 争抢、Lease、迟到响应、Supersede、幂等冲突、Watch 顺序 | `go test ./... -run 'Whitebox|CAS|Lease'` |
| 黑盒 | `review_test` external package | Human/Auto/Bypass、Inline/Detached、Conditional、API/SDK | `go test ./... -run Blackbox` |
| 故障注入 | `internal/testkit` + fault tests | Begin reply-loss、Store unknown、重复乱序、漂移、Context failure、Cleanup failure | `go test ./... -run Fault` |
| Conformance | `conformance` | reusable Store Target+Case(+Trace) create/replay、Case CAS、exact Case history | `go test ./... -run ConformanceMemoryStore` |
| REV-D11 Conformance（规划） | `conformance/external_current_reader_v1*` | S1 exact Source+Subject index resolve、S2 exact Ref复读/index校验、禁止latest/preload/ABA、immutable projection、fixed Checked/Expires/Digest、TTL、lost reply、rollback、64并发 | 当前不执行；公共Owner Port与纯Review实现授权关闭后使用`-run 'ExternalCurrent.*Conformance'` |
| REV-D11 Benchmark（规划） | `conformance/external_current_reader_v1_benchmark_test.go` | N=1/8/64/256、S1/S2、并发、lost reply、drift、TTL min | 当前不执行；未来`-bench ExternalCurrent -benchmem -count=3`，无SLA |
| Wave 1 普通测试覆盖 | `tests/repair_wave1_test.go`、`tests/conformance_wave1_test.go`、`tests/decision_ttl_test.go` | Owner、Round/Assignment/Attestation/Verdict/Invalidate、Settlement、history、lost-reply、逐Owner TTL | `go test ./... -run 'P0|P1|P2|Fault|ConformanceWave1'`；不是 reusable conformance suite |
| Race | 全 module | Case/Assignment/Attestation/Verdict/Watch 并发 | `go test -race ./...` |
| Vet | 全 module | API、锁、copy、格式化等静态问题 | `go vet ./...` |
| 集成 | 获批的公开 Runtime/Application/Harness 测试装配 | Review -> current auth -> Permit/Begin -> Execute -> Evidence/Settlement | 联合仓库命令由接线评审冻结 |
| 系统 | 真实可持久 Backend/Adapter 获批后 | 重启恢复、外部回调、Auto fork、Run 收口 | 环境命令与证据模板另行冻结 |

不得用 Fake 的通过结果宣称生产 Backend、SLA、外部平台幂等或清理能力。

### 9.1 必测正向闭环

```text
CreateCase
  -> Human/Auto Observation
  -> Review Owner Inspect + Decide/CAS
  -> current OperationReviewAuthorizationV3
  -> Operation Permit + Begin
  -> Provider Prepare/Enforcement/Execute
  -> Observation/Evidence Admission
  -> Review DomainResultFact
  -> Runtime Operation Settlement
  -> Review ApplySettlement
  -> Application 聚合 -> Runtime CompleteRun
```

### 9.2 必测反例

- pending、rejected、expired、revoked、superseded、cancelled、indeterminate；
- conditional 未满足/过期/撤销；
- Candidate/Intent/Payload/Policy/Authority/Scope/Binding/Evidence/TTL 漂移；
- 并发 Decide、CAS reply-loss、重复 Webhook、迟到 Reviewer、过期 Lease；
- Auto completed 但未 Settlement、Provider 状态自报、cache usage 自报；
- ContextReference 无法物化、外部通知未知、Reviewer Begin 丢回包；
- Bypass 策略撤销、不可 Bypass Target、旧 Permit 在漂移后重放；
- Provider 实际执行点缺 Review/Fence/Authority/Scope/Budget 任一条件。

## 10. 性能计划与 Rust 门禁

- Go 基准关注 canonical digest、Case Inspect/CAS、current projection、Watch fan-out 与大 Finding 集验证；
- REV-D11 基准另覆盖Evidence N=1/8/64/256、S1/S2 exact reads/op、deep clone/排序/去重、并发、lost-reply与fail-closed路径；只测Review聚合成本，不模拟生产网络SLA；
- 阶段 1-2 建立代表性数据规模和 allocation/latency 基线，不设未经用户批准的生产 SLA；
- 只有基准证明某纯计算热点不满足已批准目标，才另提 Rust 计划；
- Rust 提案必须明确 Go API/ABI、序列化与所有权、FFI 或独立进程选择、取消/超时、panic/crash、UnknownOutcome、回退与部署兼容；
- 状态机、CAS、网络、外部 Adapter 和治理编排默认保留 Go，不因“高性能”泛化 Rust。

## 11. 兼容、迁移与回退

1. legacy `ReviewPort` 与 `ActionRequest.ReviewRequired` 仅保留 Observation 兼容，不批量升级为正式 Verdict；
2. Effect Review 复用现有 V2，rich Target 使用经批准的新版本；
3. 旧 `policy_not_required => accepted` 只可作为显式标记的过渡投影，外部 API 不暴露其伪 accepted 语义；
4. 旧 Case/Verdict 历史不可变，新 schema 以新 revision/version 写入；
5. Adapter 发布使用 capability/version negotiation；不支持能力 Fail Closed；
6. 回退只停用新 Adapter/Contribution，不删除历史 Fact、不恢复旧 Permit、不回滚其他组件；
7. 数据迁移先 dry-run/Inspect，再以 create-once/CAS 写新索引，并保留 Residual 报告。

## 12. 阶段完成定义

每阶段完成必须同时具备：

- 对应管理线和 Port Delta 已裁决；
- 文件级产物与公开边界回读一致；
- 该阶段测试实际执行并保存可复查结果；
- `go test`、相关 `go test -race`、`go vet` 无新增失败；
- 无跨组件 implementation import、共享类型复制、私有 Slot/Hook/Phase 或 Fake 生产承诺；
- Effect/Conflict Domain/Run Requirement/Unknown/Cleanup/Residual 全部有覆盖；
- 仅在用户明确授权后同步 `.properties.rax/module/review-engine/**` 与 `.properties.rax/memory/review-engine/**`。

Wave 1及本轮非完整生产闭环切片已通过最终独立复审（P0/P1/P2=0）。关闭真实外部Owner adapters/certification、Provider/identity admission、公共Gate和production composition root前，不得声称Praxis生产闭环、生产SLA或真实Reviewer/Provider能力。
