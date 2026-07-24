# Context Engine模块说明

状态：规划内无需production composition root的Owner-local/B-cross/reference-only切片均已完成软件验证：CTX-D09/CTX-D10、Application Adapter、Memory/Knowledge B-cross、Offline/Engineering SDK与API/CLI、Compaction/Generation、Outcome、Recipe/PromptAsset pre-release、Prompt Provenance、durable Reviewer Context、Restore Context materialization及Component Release候选。target100、race20、full ordinary/race/vet通过。PromptAsset仍只为Owner-local Service；production Recipe/Prompt发布仍等待CTX-D07，C层production root、Capability、Harness Continuation与Turn推进未启用。

## 1. 作用

Context Engine把版本化Source、Recipe和Artifact输入编译为不可变Manifest/Frame，并负责stable prefix、token预算、provider-neutral Cache规划、Injection离线一致性，以及CTX-D09 N=1 Owner-local Refresh。它不拥有Runtime Outcome、Tool事实、Harness Continuation或Provider执行。

## 2. 当前组成

| 实现位置 | 作用 |
|---|---|
| `contract` | Source/Recipe/PromptAsset/Recipe结构Comparison/Artifact/Cache/Injection/Frame/Generation、CTX-D10 current及CTX-D09 Refresh/Apply/Inspect合同 |
| `ports` | Context Owner只读metadata/current reader与Owner-local refresh store；Application公共Port由独立Adapter实现 |
| `kernel` | 编译、Frame完整Inspect、Cache/Injection离线核、ParentFrame current reader及CTX-D09三段Service |
| `contract/compaction.go`、`kernel/compaction.go` | Owner-local exact Compaction Summary/Plan与确定性候选Generation Prepare；候选明确不可见 |
| `contract/outcome.go`、`kernel/outcome.go`、`outcomestore` | Outcome/Evaluation/Feedback Candidate不可变事实、exact链校验与线程安全Put-once/Inspect参考Store |
| `contract/release.go`、`kernel/release.go`、`releasestore` | Recipe不可变注册与pre-release lifecycle-head CAS；production动作显式unsupported |
| `contract/prompt*.go`、`kernel/prompt.go`、`promptstore` | PromptAsset不可变Put/exact Inspect、distinct ref、Candidate确定性投影与独立pre-release lifecycle；production动作显式unsupported |
| `contract/prompt_provenance.go`、`kernel/prompt_provenance.go` | 官方上游repo/commit/path/license bytes/range与transform/closure exact离线核；32MiB有界、chunked cancel、deep-copy/no-alias；不联网、不绑定Model Route |
| `contract/evaluator.go`、`ports/evaluator.go`、`kernel/evaluator.go`、`sdk/engineering*.go` | 独立Engineering SDK五入口、Evaluator Observation exact Admission、Evaluation/Feedback Fact构造、strict codec/limits/deep-copy/cancel；不联网、不发布 |
| `engineeringapi` | Engineering五typed方法与strict JSON dispatch；无Store/listener/auth/root语义 |
| `sdk` | 六个Owner-local Offline typed入口（含只读Recipe结构Compare与Cache Plan Inspect）、strict codec/base64、canonical digest、deep-copy bundle与ephemeral workspace；Cache Inspect不生成Plan、不声明hit；不注册Capability |
| `offlineapi` | 六typed方法与严格JSON dispatch的Owner-local transport-neutral只读面；无Store/listener/auth/root语义 |
| `cmd/context` | stdin/stdout十一命令CLI：既有Offline六命令加`prompt validate|preview`、`evaluation prepare|admit`、`feedback build`；typed错误退出码，错误零stdout |
| `refstore` | 内容寻址内存参考实现 |
| `refreshstore` | pending不可见、local ApplySettlement+Generation current expected-CAS原子提交的进程内参考Store |
| `runtimeadapter` | 仅实现Runtime公共`OperationScopeEvidenceApplicabilityCurrentReaderV3`的Context Kind投影 |
| `applicationadapter` | 实现Application公共三段Refresh与Memory/Knowledge Owner Source映射；只依赖Application公共contract/ports，不是production root |
| `reviewadapter`、`reviewcontextstore` | 实现Review公共Reviewer Context Publisher/Current Reader；Memory reference与SQLite WAL单节点durable repository，不声明HA/备份/SLA/root |
| `restorestore`、`runtimeadapter/restore_materialization_v1.go` | Restore Context新Generation/Frame的Owner-local materialization与公共只读投影；不执行Runtime Activation或Provider |
| `releasecandidate` | 公共`ComponentReleaseV1` reference-only builder/readiness及lost-reply exact publisher；Factory仅descriptor |
| `internal/testkit` | 线程安全metadata/content故障Fake；只用于测试 |
| `tests` | 黑盒、故障注入、Offline SDK边界与Conformance反例 |

`ExecutionRuntime/application/ports`已发布`ContextTurnRefreshPortV1`与`ContextOwnerSourceReaderV1`；本模块Adapter完成公共DTO到Owner-local合同的显式映射，并在B-cross fixture中手工注入Memory/Knowledge V2 Owner Adapters。该fixture不构成production root；宿主仍需在production composition root完成Capability、Harness Continuation与Turn推进接线。Application不反向import Context实现。

Continuity侧已经存在可承载Context exact Owner refs的`TimelineOwnerFactRefV1`与Checkpoint V2，不需要Context再造事件/恢复DTO；但typed Owner-current Reader/Router仍只存在于`continuity/runtimeadapter`。因此当前没有合法的Context→Continuity production Adapter：需由Continuity Owner先发布公共Port或唯一无损facade，Context再实现只读exact Frame/Generation/Outcome current Adapter。Context不会导入Continuity实现、缓存Owner snapshot、创建Evidence/Event、分配Timeline sequence或写SQLite/RocksDB。

## 3. CTX-D10当前闭环

`ContextParentFrameApplicabilitySourceCoordinateV1`固定`ID=FrameID`作为查询键，但Digest完整seal Frame/Manifest/Generation+ordinal、Scope/Run/Session/Turn、Parent binding、Recipe与Authority。Owner Reader按完整四元坐标解析binding，再以完整`FactRef{ID,Revision,Digest}`与scope读取Frame、Manifest和Generation；同ID换版本/摘要或跨Tenant/scope歧义均Fail Closed。

Reader执行：`S1 exact binding/metadata/content → InspectFrame → TTL最小值（cap<=30s）→ S2同集合复读`。TTL不会超过Frame、Manifest、Generation current pointer、Binding、Recipe或Authority的可读current上界。ReferenceStore内容、current pointer、scope或exact ref漂移时不返回current projection。

Runtime Adapter仅持有Owner Reader和Clock，只无损转换公共四元ref并返回公共current projection。它不缓存Frame/Manifest/Generation或binding snapshot，不创建Applicability Fact、Evidence、Tool watermark、Provider调用、DomainResult、Settlement、Apply、Generation CAS、新Frame或Continuation。

CTX-D10的错误分类保留未知性：`context.Canceled`、`context.DeadlineExceeded`与`ErrUnknown`统一映射为Runtime `ErrorIndeterminate`，只有真实`ErrUnavailable`映射为`ErrorUnavailable`；Conflict、Stale/Expired与NotFound继续按冻结合同分类。metadata Fake、Owner Reader和Adapter均优先检查并原样传播`ctx.Err()`，取消、超时或未知结果不会返回残留current projection。

## 4. CTX-D09 A层当前闭环

首切面严格为一个settled Tool action：`Tool=1 / Memory=0..1 / Knowledge=0..1 / Continuity=0`。Tool projection绑定ToolResult/DomainResult/Tool Apply/V4 Inspection/Association exact refs、同一Execution/Action/Attempt及有界ContentRef；Memory/Knowledge经Owner V2 Reader的S1/S2 exact projection与正文观察进入DynamicTail，每Owner聚合上限64KiB。Context不接收Receipt、Provider Observation或原始大输出，也不成为这些事实的Owner。

调用顺序为：`Refresh(S1) -> pending DomainResult/current不可见 -> Apply内Tool+Parent S2 fresh reread -> atomic Context ApplySettlement + expected Generation current CAS -> Inspect`。该本地迁移没有Runtime settlement字段、Port或调用。父Frame不变；子Frame exact复用StablePrefix/SemiStable，只追加DynamicTail。PrefixDigest和StableSourceSetDigest进入cache identity，DynamicTail不会改变stable key。

已实现并经独立复审确认：单一`ContextTurnRefreshOwnerBackendV1`同时实现CTX-D10 metadata/current readers与Refresh Store。Service内部只能用该backend构造ParentFrame Current Reader并执行最终CAS，不能注入第二套Reader或shadow Store。backend必须由exact Owner current state打开；`Reserve`在权威current缺失时NotFound，不得从请求种入。合法writer与Apply在同一锁域CAS，因此S2后barrier发生current漂移时，Apply最终CAS必为Conflict，且pending子Binding/Frame/Manifest/Generation均不可见。

状态机已实现`ErrInspectOnly`：已存在Attempt的重复Reserve及已Applied Attempt的重复Apply都同时满足`ErrInspectOnly/ErrConflict`，只允许Inspect原Attempt。lost-Apply-reply测试确认第二次Apply、第二次Reserve均拒绝，而Inspect返回第一次原子提交的exact applied Result。

Apply预Inspect只接受`nil + pending`进入后续路径；`nil + applied`为Inspect-only Conflict，NotFound直接返回，其他Conflict/Unavailable/Unknown/context错误原样返回且CAS调用数必须为0。故障Fake已覆盖“Inspect失败但Load可成功”反例，证明实现不会绕过Owner Inspect继续Apply。

RefreshAttempt/Manifest/Frame/Generation ID由canonical request确定性派生。TTL取请求、Parent current、Tool current、Recipe及cache identity最小值；边界相等拒绝。S2 TTL crossing、clock rollback、Parent/current或Tool source漂移时，最多保留pending，Generation current不可见。Apply丢回包只Inspect原Attempt。64个竞争Attempt并发Apply只有一个expected-current CAS赢家。

`refreshstore.Memory`在单一锁域读取/更新权威Generation current，并提交local ApplySettlement和新Frame/Manifest/Generation metadata；提交前CTX-D10 Reader读不到子Source，提交后可按exact refs读取。它只是进程内参考Backend，不是production State Plane root、持久化Backend或SLA。

## 5. 测试与能力边界

既有CTX-D09/CTX-D10修复已通过第二轮独立只读复审。Offline SDK第二轮审计六类缺口已经返修：typed入口clone前有界预检、base64 chunk array在DTO物化前流式计数、request-specific wire limits、context-aware canonical marshal/digest、null/conditional presence，以及boundary/fault/max矩阵。最终hash已由独立复审执行target100、race20、full ordinary/race、vet、P0回归与max-size 1/2/4/8，全部PASS且无硬问题，因此标记`implementation_software_test_yes`；该结论不等于production/root GO。

Engineering SDK已执行Prompt validate/preview、Evaluation prepare/admit、Feedback build的unit/blackbox/fault/conformance，并通过定向普通100轮、race20轮及full ordinary/race/vet。Prompt Provenance增量同样通过target100/race20/full ordinary/race/vet，覆盖artifact/license/generated bytes漂移、range、transform chain、closure、TTL、mid-cancel、zero-byte、opaque SDK preset、no-alias和64并发。Evaluator与Provenance都不扩Offline SDK六operation，不提供远程Judge、联网抓取、Capability或production root。

`internal/testkit`和`internal/testfixture`不是生产Backend、State Plane root、持久化承诺或SLA。CTX-D10、CTX-D09 A/B-local、Application公共Port Adapter与Memory/Knowledge B-cross fixture均已完成；C层production composition root、Capability、Harness Continuation与Turn推进仍未实现且NO-GO。

Component release当前只到`reference_only assembly-candidate`。进程内refresh/cache/ref/outcome/release/prompt store、Offline SDK与test-only B-cross均不能替代durable state/cache、Source/Provider current、per-turn injection/continuation、cleanup或deployment证明。

## 6. 构建验证

```bash
cd ExecutionRuntime/context-engine
go test -count=1 ./...
go test -count=100 ./contract ./kernel ./internal/testkit ./tests/blackbox ./tests/failure ./tests/conformance
go test -race -count=20 ./contract ./kernel ./internal/testkit ./tests/blackbox ./tests/failure ./tests/conformance
go test -race -count=1 ./...
go vet ./...
go test -run 'ContextRefresh|ContextOwnerSource|LiveV2' -count=100 ./contract ./applicationadapter ./tests/integration
go test -race -run 'ContextRefresh|ContextOwnerSource|LiveV2' -count=20 ./contract ./applicationadapter ./tests/integration
PRAXIS_CONTEXT_MAX_SIZE=1 go test -count=1 -run 'TestOfflineSDKMaxSizeConcurrencyEvidenceV1|TestStagedRendererFourMiBGoldenAndExactLimitV1' -v ./sdk ./kernel
```

Compaction/Generation完整Owner-local闭环实际通过：`go test -count=100 ./contract ./kernel`、`go test -race -count=20 ./contract ./kernel`、`go test -count=1 ./...`、`go test -race -count=1 ./...`及`go vet ./...`。候选metadata在Prepare后不可见；S2、TTL和expected current均通过后，Manifest/Frame/Generation/current pointer在同一锁域原子发布。lost reply、重复Prepare/Apply只允许exact Inspect。

Outcome/Evaluation/Feedback Candidate事实链通过`go test -count=100 ./contract ./kernel ./outcomestore`与`go test -race -count=20 ./contract ./kernel ./outcomestore`，并纳入full ordinary/race/vet。Store为进程内参考实现，不是production State Plane；Provider cache usage Observation不会自动形成Cache hit。

Recipe pre-release lifecycle通过`go test -count=100 ./contract ./kernel ./releasestore`与`go test -race -count=20 ./contract ./kernel ./releasestore`，并纳入full ordinary/race/vet。lifecycle head不等于production current binding；publish/rollback/revoke为CTX-D07 NO-GO。
