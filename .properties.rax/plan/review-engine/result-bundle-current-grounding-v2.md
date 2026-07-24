# Review Result Bundle Current Grounding V2 实施计划

## 1. 状态

- 设计：`../../design/review-engine/result-bundle-current-grounding-v2.md`。
- 测试矩阵：`../../design/review-engine/result-bundle-current-grounding-v2-test-matrix.md`。
- 当前结果：**Review owner-local Go、测试与最终独立复审完成（P0/P1/P2=0）**。
- Owner-local V1 structure/store：YES。
- V2 production：**NO-GO**，等待Artifact/Environment/Validation Scope公共exact-current Readers、typed router与trusted root。
- 默认Go；无benchmark证据，不规划Rust。

## 2. 依赖DAG与无SCC import

```text
Artifact Owner Reader ---------+
Environment Owner Reader ------+
Validation Scope Owner Reader -+--> host typed-owner router --+
Context Owner live Reader ------------------------------------+--> Review Grounding aggregate
Evidence Owner live Reader -----------------------------------+            |
Review Request/Target/Bundle Store ----------------------------+            v
                                                               Verdict Owner CAS input
```

- Review只依赖自身公开包与Runtime public `core/ports`；不import Sandbox/Continuity/Context实现包。
- Owner公共Port不import Review实现包；source adapter由各Owner实现。
- composition root最后注入，缺任一required Reader时构造失败。

## 3. 分阶段落点

| 阶段 | 文件级候选 | 产物 | 前置 |
|---|---|---|---|
| G0 联合冻结 | Runtime public ports、Artifact/Environment/Validation Scope Owner资产 | 三组nominal refs/projections/readers/Owner-only publishers；Validation Scope Owner-neutral identity+association current；Owner Binding closure；逐方法closed errors；conformance | 本设计双审 |
| G1 Review contract | `contract/result_v2.go`、canonical helper与tests | **完成**：V2 Bundle/Claim/Artifact binding、单向exact Request/Target、deep clone、legacy隔离 | G0 public types |
| G2 Store | `ports/store.go`、`memory`、`storage/sqlite` | **完成**：无孤儿写口；Bundle V2与Request/Target/Case原子提交、immutable exact history/snapshot | G1 |
| G3 current consumer | `resultgrounding/reader_v2.go`、`conformance/result_grounding_v2.go` | **完成**：stored+Context+Binding+Evidence+typed router S1/S2、full route Proof、逐Owner时钟、minTTL、closed errors | G0-G2 |
| G4 Verdict接入 | `verdictowner`、`decisioncurrent` | Decide实际点fresh clock复读aggregate，零漂移CAS | G3、REV-D11/REV-D12 |
| G5 SDK/API/UI | `service`、`api/http`、`sdk/go`、CLI | Claim→Artifact Anchor→Evidence可导航；明确legacy状态 | G2-G4 |
| G6 production root | host composition Owner | real typed Readers/root、integration/system proof | 全Owner conformance |

Runtime/Harness/Application/其他Owner的文件只由相应Owner修改；Review不跨目录补洞。

## 4. 阶段验收

### G0 公共合同

- 三类ref nominal独立；三个具名ProjectionIdentityInput及literal golden冻结稳定ID边界；Artifact locator语义归Artifact Owner；closed kind/version router无fallback。
- projection immutable create-once、history/highest/current full-ref/PublishReceipt原子CAS；historical与current解耦；publish lost reply只Inspect exact PublishRef。
- Validation Scope以Owner-neutral`Kind/TenantID/ID`建立唯一Owner association current；换Owner必须CAS新revision，同source双Owner Conflict。
- typed router declaration绑定full Owner Binding与sealed ReaderBindingRef，Resolve返回含Declaration/full Owner、RouteRef、ReaderBindingRef/adapter digest和typed Reader的nominal sealed对象；独立required catalog与bindings一一对应；禁止Go interface identity比较。
- Resolve/InspectCurrent/InspectHistorical、Create/CAS/InspectPublish、Validate/ValidateCurrent、全部Derive/Digest/Seal、resolved-route、Grounding Request/StoredFacts/Dependencies、constructor/read与deep clone的逐方法closed errors及lost reply语义冻结。
- Sandbox/Continuity source adapter不扩张Owner，不制造generic Artifact current。

### G1-G2 Review本域

- literal JSON/golden digest稳定；all claims exact绑定Artifact/Anchor/full Evidence；OriginalIntent/AcceptanceCriteria只引用Context Envelope exact instruction materials，不接受caller digest。
- Bundle+Request+Target+Case可按现有compound mutation扩展或另行联合裁决；任何staged冲突零写，禁止事务外补Trace/Bundle。
- V1历史可读但production zero authorization；V2 history不可覆盖。

### G3-G4 current与Verdict

- baseline→S1 resolve/index→exact Inspect→S2 same ref/index unchanged→fresh now→minTTL→aggregate seal；aggregate完整包含Validation Scope association与全部nominal route Proof/Owner Binding closure。
- TTL精确纳入Bundle/Request/Target/Context/all Context sources/all Artifact/Environment/Scope/all Evidence及每个external Owner Binding/consumer/grant closure。
- lost S1开始新cut；lost S2只重读same ref；唯一`ReadRecoveryTimeoutNanos`满足`0<timeout<=2s`并按cut TTL/caller deadline裁剪，`WithoutCancel`后立即`WithTimeout`；Unknown、clock rollback、TTL crossing、ABA整批零Verdict/CAS。
- grounding projection不是Evidence、Authority、Permit、Artifact Commit或Runtime Outcome。

### G5-G6 交付与production

- UI/API从Claim可定位exact Artifact Anchor、Evidence、Environment、Validation Scope与Limitations/Uncovered。
- production root使用真实Owner Reader；缺Reader/unknown kind/root未装配立即Fail Closed。
- 黑盒验证旧截图/旧revision、cross-tenant、source drift、Provider observation不能误Accept。

## 5. 测试门

按`result-bundle-current-grounding-v2-test-matrix.md`执行：

1. unit/canonical/golden/deep-clone；
2. whitebox S1/S2/current index/ABA/minTTL；
3. blackbox cross-owner/cross-tenant/legacy/root missing；
4. fault lost read、ctx cancel、Unknown、clock rollback、TTL crossing、staged zero write；
5. Artifact/Environment/Validation Scope reusable Owner reader+publisher+Binding current conformance，Validation Scope association conformance，typed required-catalog/resolved-route router与全部public error方法conformance；
6. targeted ordinary100、race20、full ordinary/race/vet；
7. import scan、gofmt、diff-check、links/stale。

只有实际执行后才能记录PASS；本轮资产冻结不运行Go门。

## 6. 兼容、迁移与回退

- V1不删除、不覆盖、不批量升级；API明确`v1_legacy`。
- 新生产Result Review只接受V2；缺public Reader时Fail Closed，不退回digest-only。
- 回退只停用V2 producer/consumer capability，不删除V2历史、不恢复旧Verdict/Permit。
- source adapter故障不切换到结构相似Owner；由root移除对应kind并让请求明确失败。

## 7. 完成定义

Owner-local Go完成：G1-G4代码与RB-REV/RB-FAULT/RB-ERR、target100/race20/full/race/vet、独立复审全绿。

production完成：G0三Owner真实public contracts+conformance、G5/G6真实root与integration/system proof、REV-D11/REV-D12共同满足。任何一项未闭合都只能报告“Review V2 owner-local完成；production Result Grounding NO-GO”。
