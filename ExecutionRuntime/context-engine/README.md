# Praxis Context Engine

状态：Owner-local已完成的软件切面均通过验证。CTX-D09 A层Refresh、CTX-D10 Reader、Offline SDK、`ContextOfflineIngressV1`、`ContextEngineeringSDKV1`、Compaction/Generation、Outcome事实链、Recipe/PromptAsset pre-release及`PromptUpstreamProvenanceV1`离线核已经落地。Engineering五入口API/CLI和Prompt Provenance均通过target100/race20及full ordinary/race/vet。production发布、跨Owner root、Capability、Harness Continuation与Turn推进仍未启用。

Context Owner 已实现 Review public `ReviewerContextPublisherV1` / `ReviewerContextCurrentReaderV1`：Memory 仅是 reference store；SQLite WAL durable repository 使用 stable subject-derived ID、append-only history、highest/current full-ref CAS 单事务、strict row digest、重启恢复和 exact historical lost-reply recovery。`OpenDurableReviewerContextAdapterV1`只构造单节点/单 writer-domain Owner capability，不安装宿主 composition root，也不声明多节点 HA、备份、远程持久性或 SLA。current读取每次以 fresh clock验证固定的 Checked/Expires/Digest，不因时间前进重封projection。

`ContextCompactionV1` Owner-local闭环已实现：exact Source Frame Range、Compaction Summary、expected Generation current绑定、确定性候选Generation Prepare、S2 current复读、同一Owner backend锁域内的原子Generation current CAS、Inspect-only恢复，以及压缩后未保留Anchor必须重新物化。Prepare后候选Manifest/Frame/Generation不可见；只有Apply CAS成功才一起发布。全链不写Runtime Settlement、Continuity或任何外部Effect。

`ContextOutcomeV1` Owner-local事实链已实现：Outcome只关联Frame/Recipe/Generation与外部Owner exact refs，并记录token/cache/cost/latency等量化Observation；Evaluation逐项Inspect Outcome并校验Recipe/Policy/TTL；Feedback Candidate再exact绑定Evaluation。三类事实均Put-once/Inspect、同ID换内容Conflict，不包含Task/Runtime Outcome、Cache hit、Review Verdict或自动Recipe发布能力。

Recipe pre-release Owner-local生命周期已实现：不可变Recipe注册与`draft -> validated -> evaluated -> review_pending|rejected` lifecycle-head CAS，支持exact Inspect和64并发单后继。它不是production Recipe current binding；`publish/rollback/revoke`固定返回`ErrUnsupported`，等待CTX-D07对Run外Review/Operation语义的公共裁决。

PromptAsset Owner-local闭环已实现：资产直接内嵌规范化`instruction/example/policy`片段规格与exact `ContentRef`，不创建第二层PromptFragment Fact；`PromptAssetRefV1`与普通/Recipe FactRef nominal分离。Store提供不可变Put/exact Inspect，Service确定性投影`ContextCandidate`并维护独立pre-release lifecycle-head CAS。Prompt不拥有Provider message、Frame Region、最终顺序或cache placement；production `publish/rollback/revoke`固定返回`ErrUnsupported`并等待CTX-D07。

本模块实现Context Engineering的纯本地、Provider-neutral内核：

- Source/Candidate/Admission、Recipe、Fragment、Manifest、Frame与Generation；
- stable prefix、预算和确定性排序；
- Manifest/Frame exact reference重算Inspect；
- Artifact Anchor/Delta及压缩后Anchor保留证明纯模型；
- CachePlan、全Partition cache key、Provider Profile currentness、TTL、失效和离线经济性比较；大数成本使用精确乘除后饱和，write+keepalive不会回绕；
- `ContextRecipeComparisonV1`纯结构diff：绑定两个exact Recipe Ref并报告规范字段digest变化，不判断better/compatible/publish；
- PromptAsset不可变资产、distinct pre-release lifecycle、Role到Fragment Kind/Trust的确定性Candidate投影；
- `PromptUpstreamProvenanceV1`离线来源核：exact repo/commit/path/license/bytes/range、确定性transform chain、Stable/SemiStable/Dynamic closure与verification report；不联网、不自动裁决license、不绑定Model Route；
- `ContextEngineeringSDKV1`五个Owner-local typed/strict入口：Prompt validate/preview、Evaluation prepare/admit与Feedback build；Evaluator只返回Observation，Context exact S2核验后才生成Fact；
- `ContextEngineeringAPIV1`五typed方法与五JSON dispatch，以及`prompt validate|preview`、`evaluation prepare|admit`、`feedback build`五个stdin/stdout CLI；只复用Engineering SDK/strict codec，无server/listener/Store/Capability/root；
- Expected/Provider Actual/Harness Actual Injection合同、强类型Observation因果绑定及离线Conformance；
- CTX-D09 N=1 Owner-local Refresh：`Refresh -> pending DomainResult（current不可见）-> S2 fresh reread/TTL -> atomic local ApplySettlement + expected Generation current CAS -> Inspect`，零Runtime settlement；
- 线程安全的内存内容寻址reference store。
- CTX-D10 ParentFrame Applicability Current Reader：distinct四元Source Coordinate、完整subject seal、exact Frame/Manifest/Generation metadata readers、Generation current pointer、S1/S2完整`InspectFrame`和最长30秒的owner-TTL最小值；
- Runtime V3只读Context-kind adapter：只把`Kind/ID/Revision/Digest`交给Context Owner Reader并投影公共current结果，不缓存metadata/binding snapshot，不创建Fact或Evidence。
- Review ReviewerContext Owner port：Memory reference与SQLite WAL durable repository共享同一公开Review合同；Resolve按exact subject取current full ref，current Inspect原子核对index/highest/history，historical Inspect不借current，lost mutation reply只Inspect原exact历史对象。
- `ContextOfflineIngressV1`：六request公开Seal/streaming Encode、六typed/strict JSON API，以及`context recipe validate|compare|compile|preview`、`context frame inspect`、`context cache inspect`六条stdin/stdout CLI；Compare只返回结构field-path与before/after digest，不作质量、兼容或发布结论；Cache Inspect只核验调用者给定的provider-neutral Plan/Profile exact闭包、TTL/currentness与离线经济性，不生成Plan、不调用Provider、不声明cache hit；与Engineering五入口共同保持无Store、listener、Capability或写命令。
- `releasecandidate`：复用Agent Assembler公共合同发布`reference_only` ComponentRelease候选；Ensure未知回包只按同一exact ref Inspect，Factory仅descriptor。

合同JSON入口使用递归严格解码：拒绝unknown field、trailing document及任意嵌套层级的duplicate key。Harness Actual Injection必须携带至少一个按source sequence规范排序且唯一的类型化Observation Ref；只有Expected Manifest处于`Created <= current < Expires`、完整fidelity并通过Execution/Route/Attempt/Frame/sequence/Digest逐项Inspect的Provider Observation才可能得到`matched`。

CTX-D09首切面严格固定一个settled Tool action：`Tool=1 / Memory=0..1 / Knowledge=0..1 / Continuity=0`。Tool只接受exact settled chain及有界ContentRef；Memory/Knowledge只经各Owner V2 Reader与Application中立Port完成S1/S2、exact content observation与稳定关联，正文每Owner聚合上限64KiB。它们分别作为受限`memory_recall`/`knowledge_reference`进入DynamicTail，不携Authority。Receipt、raw/unbounded output没有合同入口。父Frame不可变，StablePrefix/SemiStable exact `ContentRef`复用；TTL crossing、clock rollback或任一current漂移都保留pending且不发布current，lost reply只Inspect原Attempt。

独立复审确认`ContextTurnRefreshServiceV1`构造器已收紧为单一`ContextTurnRefreshOwnerBackendV1`：CTX-D10 S2 exact-current reads、pending store、合法current writer与Apply expected-CAS必须由同一backend/锁域提供，调用方无法再分开注入Reader与CAS Store。`Reserve`只接受backend中已存在且exact相等的权威current；空backend返回NotFound，禁止从请求种入shadow current。S2后到CAS前的权威漂移由最终CAS捕获为Conflict，pending子Frame/Manifest/Generation保持不可见。

状态机新增`ErrInspectOnly`。同一Attempt一旦存在，重复`Refresh/Reserve`不得重写；一旦Applied，重复Apply同时分类为`ErrInspectOnly`与`ErrConflict`。Unknown/lost reply后唯一恢复路径是exact Inspect，不能通过重放写方法取得prepared或applied Result。

Apply的预Inspect严格Fail Closed：只有`nil + pending`可继续Load/S2/CAS，`nil + applied`返回Inspect-only Conflict；NotFound直接返回NotFound，Conflict、Unavailable、Unknown、cancel/deadline及其他context错误均原样返回。任何非pending Inspect结果都不会调用Apply CAS。

Application已发布`ContextTurnRefreshPortV1`、`ContextOwnerSourceReaderV1`及协调DTO；`applicationadapter.ContextTurnRefreshAdapterV1`显式映射公共DTO与Owner-local合同，且只依赖Application公共`contract/ports`。Memory/Knowledge B-cross fixture已完成，但仍不是production composition root；Harness Continuation、Turn推进与Capability没有接线。

`refreshstore.Memory`是线程安全、进程内Owner backend参考实现：本地ApplySettlement、权威Generation current、合法writer CAS及新Frame/Manifest/Generation metadata在同一锁域原子可见，可供CTX-D10 Reader exact读取；它不是production State Plane root、持久化Backend或SLA。`internal/testkit`与`internal/testfixture`也只用于隔离测试。

当前仍不支持真实Model、Provider Cache、远程Source、远程评测、Prompt发布、通用per-turn Hook、Runtime写接口或Harness/Model Invoker production Adapter；不实现C层production composition root、Harness Continuation、Turn推进、N>1 Tool refresh或Continuity注入。Memory/Knowledge仅完成B-cross test-only各一项projection闭集。七家模型官方来源审计与Provenance离线核已完成；具体Prompt文本仍是pre-release candidate，production适用性等待Model Invoker exact Profile ref/current reader。

因此Component Release固定`reference_only`：完整descriptor与owner-local conformance不等于durable state/cache、Provider current、Harness injection/continuation、cleanup或deployment attestation；强改production必须失败关闭。

验证：

```bash
go test -count=1 -shuffle=on ./...
go test -count=1 ./kernel -run 'TestCompile|TestCache|TestInjection|TestSettlement|TestContextTurnRefresh'
go test -count=1 ./tests/blackbox ./tests/failure ./tests/conformance
go test -count=100 ./contract ./kernel ./internal/testkit ./tests/blackbox ./tests/failure ./tests/conformance
go test -race -count=20 ./contract ./kernel ./internal/testkit ./tests/blackbox ./tests/failure ./tests/conformance
go test -run 'ContextRefresh|ContextOwnerSource|LiveV2' -count=100 ./contract ./applicationadapter ./tests/integration
go test -race -run 'ContextRefresh|ContextOwnerSource|LiveV2' -count=20 ./contract ./applicationadapter ./tests/integration
go test -count=1 -race ./...
go vet ./...
```
