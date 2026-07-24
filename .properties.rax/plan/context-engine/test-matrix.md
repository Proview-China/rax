# Context Engine v1测试矩阵

状态：规划内无需production composition root的Owner-local/B-cross/reference-only矩阵均已执行通过，包括Wave 1离线核、`CTX-D10`、`CTX-D09`、Offline/Engineering SDK、Compaction/Generation、Outcome、Recipe/PromptAsset pre-release、Prompt Provenance、durable Reviewer Context、Restore Context materialization与Component Release候选。定向ordinary100/race20及full ordinary/race/vet均通过；production C层不在本组件实现范围且未执行。

## 已执行：CTX-D09本地A/B-local

| 组 | 已执行覆盖 | 实际结论 |
|---|---|---|
| 合同/确定性 | N=1基数、三段Prepare/Apply/Inspect、deterministic Attempt/Manifest/Frame/Generation ID | `Tool=1 / Memory=0..1 / Knowledge=0..1 / Continuity=0`；相同输入同ID/Digest |
| Owner Reader唯一性 | Memory/Knowledge public V2 Reader与Application中立Port；import/nominal扫描 | 无Context平行Owner nominal/DTO；无Owner concrete Store/internal import；S1/S2 exact association |
| Turn双坐标 | Source Turn Reader ref/Ordinal T、settled Tool/ExpectedCurrent T、Context childExecution T+1、Context-owned proof | T全等且属同Run/Session；Application/Memory/Knowledge均不 mint proof、不`+1` |
| 同backend current/CAS | CTX-D10权威current reader与Apply CAS同一Owner backend/锁域、S2后barrier漂移 | 漂移必Conflict；pending子Frame/Manifest/Generation仍不可见 |
| 原子可见性 | pending DomainResult、S2 fresh reread、atomic local ApplySettlement+expected Generation current CAS | pending不current；仅CAS赢家可见；Runtime settlement调用0 |
| Unknown恢复 | lost reply、重复Reserve/Apply、Inspect-only、Owner Inspect错误 | Unknown后只Inspect；Unavailable/Conflict/cancel/deadline不继续CAS |
| currentness | TTL crossing、clock rollback、Parent/Tool/current pointer漂移 | Fail Closed；旧current保持，零Harness/Provider副作用 |
| Frame/cache | 父Frame冻结、Stable/SemiStable exact ref复用、DynamicTail追加、PrefixDigest/cache identity | prefix漂移改变key或拒绝；DynamicTail不改变stable key |
| 并发 | 64个竞争Attempt、ordinary100、race20 | 单一current赢家；无竞态、无stale success |
| 全量轻重门 | full ordinary、full race、vet、gofmt、diff/import-boundary | 均PASS；第二轮独立复审YES |

## 未执行：G6B跨模块与C层

| 组 | 未执行项 | 门禁 |
|---|---|---|
| Application公共合同 | `ContextTurnRefreshPortV1`、`SettledActionContextSourceCurrentReaderV1`公共DTO/Port | 须由Application Owner发布并联合评审；本组件中立接口不得冒充 |
| G6B跨模块fixture（已执行） | 手工注入Application公共Port、Memory/Knowledge V2 Adapter到Context Owner-local合同映射 | exact Frame含ToolResult+MemoryRecall+KnowledgeReference；lost reply Inspect；ordinary100/race20 PASS；不冒充production root |
| Cross-owner Turn mapping | 具名Session/Turn Owner Reader exact ref→Source T；Context childExecution→Target T+1；pending seal→proof→S2→Apply/CAS→publish | 待公共Port/root后执行；任一Owner自增Turn、补造Session或抢占proof Owner均Fail Closed |
| Production composition | 真实Application/Context/Harness装配、Capability注册 | production root完成并通过G6B验收前NO-GO |
| Harness推进 | Continuation(new exact FrameRef)、Turn推进 | G6B验收前NO-GO |
| 真实Route/State Plane | exact materialization、生产State Plane/backend | 不由本组件fixture宣称或替代 |

## 已执行：ContextOfflineSDKV1（独立软件验收YES）

| 层级 | 重点用例 | 通过标准 |
|---|---|---|
| SDK DTO/codec单元 | 六入口完整Request/Response、共同Envelope、六Decode/Encode、Request/Result Digest | typed入口不声称duplicate-key检查；codec拒绝unknown/trailing/递归duplicate key |
| Recipe Compare离线核 | 两个exact Recipe、field-path规范排序、before/after digest、Comparison/Result digest、双Recipe TTL最小值 | 不输出better/compatible/publish；相同Recipe=`changes:[]`；null、digest漂移、MaxRecipes<2、TTL延长、cancel均零Response；64并发确定 |
| DTO/presence/digest | exact structs/tags、Operation六值闭集、required key、optional指针、nil/empty、own-digest置空与等式/排除集 | 键缺席/null/nil slice/非法Operation/digest环或漂移全部零Response |
| limits边界 | input 24 MiB/1024；Compile-derived generated 52 MiB/output 76 MiB；global 68 MiB/4、100 MiB/1028；operation wire 48/48、48/144、144/48、144/48 MiB | operation-derived/request/global/wire分账；max+1与算术溢出稳定`limit_exceeded`，零Response |
| deep-copy/no-alias | Bundle构造后改输入bytes、改Items返回、改Response嵌套slice/map | Digest与其他结果不变；无跨请求内容泄漏 |
| Validate白盒 | Recipe/Candidate结构、预算、TTL字段、候选乱序 | 不读正文/Owner current；报告排序与Digest确定 |
| Compile白盒 | immutable input bundle、ephemeral workspace、Admission/budget/render/Inspect | 输出`Authoritative=false`；Owner Store/CAS、Generation current、DomainResult、Settlement调用0 |
| Preview黑盒 | Admission、区域、token、ContentRef/Digest、expiry/Residual | 原始内容字节与Secret均不返回；不推断cache hit/Actual Injection/Route能力 |
| Inspect黑盒 | exact Manifest/Frame/bundle/CompileDigest | 只证明bundle内部可复现，不宣称Owner current |
| 故障注入 | missing content、同Ref换bytes、digest/length/binding漂移、expiry crossing | NotFound/Conflict/Expired；零stale report、零写入 |
| required/optional missing | effective-required ContentRef缺失；optional ContentRef缺失；共享helper/wrapper错误语义 | required只在SDK边界=`not_found`+零Response；optional复用唯一`content_unavailable` Residual+零Fragment/零token/零Owner Store read；共享Owner语义无改写 |
| cancel/deadline | context-aware strict token、64/48 KiB chunks、candidate/content、staged kernel/store、render/Inspect、seal/return取消 | typed canceled/deadline且`errors.Is`保真；零Response/零bundle；无整包无context路径、无第二套算法 |
| workspace状态/故障 | 构造后/Begin前、Begin每个失败点、Seal/Export/Abort/Destroy合法与非法转移；每个Put/render/Inspect/seal/export注入cancel/error；二次请求读前次Ref | Begin前已defer Destroy；`Destroy(new)`合法幂等；所有路径终态destroyed；partial Put/bytes/Ref不可达；零goroutine泄漏/后台继续写 |
| base64 primitive golden | raw 0/1/2、48 KiB-1/exact/+1、96 KiB双chunk；padding、URL/raw/no-padding/whitespace/empty string/short non-final/redundant chunk | 0只验证primitive `[]↔[]`且无ContentRef；正长度canonical Encode/Decode固定；非规范表示typed拒绝、零产物 |
| zero-byte item反例 | 零长度ContentRef、空bytes、`base64_chunks=[]` wire item、required空内容 | ContentItem/Bundle构造=`invalid_argument`且零Bundle；required无合法非零Ref时Fail Closed；不得修改全局ContentRef或建立第二nominal |
| renderer golden | staged streaming renderer与旧`renderRegions`对同fixture输出四region/full bytes | 逐字节、ContentRef、Digest相同；旧wrapper不声称cancel |
| stable sort工作界 | live `sort.SliceStable`对已排序/逆序/全相等/重复/交错/确定性乱序512-candidate fixture | 记录comparisons与耗时；同Go版本保守防回归阈值可复算；无`n*ceil(log2 n)`虚假承诺，不宣称取消SLA |
| Source=0 / Owner合同 | `MemorySources/KnowledgeSources/ContinuitySources=0`；Reader调用计数；注入`knowledge_reference` | Reader调用均0；未确认kind返回`unsupported`+零Response；无平行Owner nominal/DTO |
| Conformance | 尝试把报告当Fact/Evidence/Capability/Settlement/current | nominal type/边界拒绝；Effect/Runtime/Harness/Provider调用0 |
| ordinary100 | 六入口确定性、strict decode、错误分类、乱序输入 | 100轮逐字节一致、无flaky |
| race20/small 64并发 | small fixture下共享SDK实例或无状态函数并发 | 无竞态、无跨请求内容泄漏、输出一致 |
| max-size 1/2/4/8并发 | 24 MiB input、52 MiB generated、76 MiB output及各operation wire上限附近，记录`ns/op,B/op,allocs/op,peak heap/RSS,cancel-to-return` | 无OOM/竞态/泄漏；只作有界证据，不宣称SLA |
| full gates | 获单独Go授权后的全模块ordinary/race/vet/gofmt/import/links/diff | 全部实际PASS后才标记implemented；design YES不能替代实现验证 |

独立软件验收实证：24 MiB fixture产出generated=`53,129,367` bytes（`<52 MiB`）、output=`78,295,191` bytes（`<76 MiB`）、wire=`104,407,083` bytes（`<144 MiB`）；1/2/4/8并发均PASS，最高VmHWM=`3,285,408 KiB`，mid-cancel=`807.57 µs`。full ordinary100、full race20与vet实际PASS；该YES只覆盖Owner-local软件边界。

## 完整覆盖矩阵（执行状态以上述分区为准）

| 层级 | 重点用例 | 通过标准 |
|---|---|---|
| 合同单元 | strict schema、SemVer、canonical Digest、limits、nil/empty、排序去重 | 非规范输入Fail Closed；Digest可复算 |
| 状态机白盒 | Frame/Recipe/Cache/Anchor/Generation合法与非法迁移 | revision单调；非法边100%覆盖 |
| Admission白盒 | Required/Optional、authority、freshness、sensitivity、dedupe、budget | Required漂移失败；Residual原因稳定 |
| Frame白盒 | Manifest freeze、内容寻址、Delta/Rebase、Replay | 同输入同Digest；冻结后不可变 |
| CTX-D10 Coordinate单元 | Kind、`ID=FrameID`、Frame Revision、完整sealed subject Digest | ID只作查询key；改变Frame/Manifest/Generation/ordinal/scope/run/session/turn/parent/recipe/authority任一字段必须改变Digest；禁止只哈希Frame ID |
| CTX-D10 nominal/type-pun | Context Source、普通FactRef、Application DTO、Runtime公共ref使用相同字符串值 | 类型不可互换；只允许公共router逐字段投影后由Context Owner验证 |
| CTX-D10 metadata白盒 | `ResolveExactSourceBinding`、Frame/Manifest/Generation exact FactRef+scope、Generation current pointer | 每次从Owner-style store复读；不得按FrameID任选current值或由Coordinate Digest猜Frame Digest |
| CTX-D10 scope冲突 | 同FrameID跨Tenant/ExecutionScope、同ID多结果、取回Frame scope digest与请求/公共projection不一致 | Conflict/Fail Closed；不得选择第一项或缓存旧绑定 |
| CTX-D10 S1/S2白盒 | Source binding、Frame/Manifest/Generation、current pointer、Recipe/Authority、ReferenceStore ContentRefs | 完整`InspectFrame`两次通过才Current；任一Revision/Digest/content/pointer漂移拒绝 |
| CTX-D10 TTL边界 | request not-after、checked+30s、Frame/Manifest/Generation/Recipe/Authority真实上界、边界时刻 | Expires严格取最小值；等于checked或检查中跨界不得Current；cap不延长TTL/SLA |
| CTX-D10 Runtime黑盒 | Runtime公共四元ref→Context Kind adapter→Owner Reader→公共projection | 四元组与scope/expiry无损；Adapter配置无metadata快照/binding map；Runtime不创建Context Fact |
| CTX-D10 故障注入 | Source binding/Frame/Manifest/Generation/current pointer NotFound/Unavailable、Kind router缺失、ReferenceStore changed content | 全部零Evidence Issue、零Tool watermark、零Provider；只有复读，不生成替代坐标 |
| CTX-D10 Conformance | public ref、Context projection与Runtime Evidence资格分层 | ref/projection均不自动授Evidence资格；Owner边界与required矩阵保持 |
| CTX-D10 Race | 线程安全Fake并发读、同ID换revision/digest、跨scope插入、pointer切换 | `go test -race`无竞态；无stale success、无非确定性任选项 |
| G6A输入合同 | settled ToolResultV2、current V4 Inspection、verified Association及同一Execution/Action/Attempt绑定 | 缺项、非current、未verified或绑定漂移Fail Closed；不要求中央端到端Gateway进入测试进程 |
| CTX-D09 N=1白盒 | 本组件三段合同、G6A exact fixture、Owner V2 sources、Source cardinality | Tool=1、Memory/Knowledge各<=1、Continuity=0；S1/S2/64KiB/Unknown/cancel错误保真；其他基数在CAS前拒绝 |
| 三段顺序/Owner | settled Tool exact chain、Refresh/pending DomainResult、Apply内S2、atomic ApplySettlement+Generation current CAS、Inspect | pending不current；S2失败current pointer不可见；Inspect严格只读 |
| settled-action fixture | exact ToolResult/DomainResult/Apply/V4 Inspection/Association、有界ContentRef、Checked/Expires/Digest | Receipt/Observation/raw/unbounded output、普通DTO type-pun、任一exact链漂移均零projection/零Context写 |
| Refresh确定性/CAS | 相同冻结输入并发创建、deterministic Attempt/Frame/Generation ID、expected generation CAS | 相同输入同ID/Digest；单一CAS赢家；换内容复用ID冲突 |
| S1/S2本地current复读 | Tool exact fixture、ParentFrame/Manifest/Generation、Recipe/cache identity在S1后漂移或TTL crossing | 最多保留pending/diagnostic fact；ApplySettlement不成功、current pointer不可见、零Harness；跨模块Session/PendingAction/Binding复读未执行 |
| 原子Apply/current CAS | ApplySettlement写入、expected Generation current CAS、并发赢家、单边故障 | 两者同一原子可见性边界；不得观察单边成功；失败保持旧current |
| Refresh lost reply/Unknown | reserve/freeze/DomainResult/Settlement返回/Apply/generation CAS各点Unknown、cancel、deadline | 写前零状态；写后只Inspect原ID；不产生第二Frame/Generation、不重跑Tool、不推进Turn |
| Frame区域稳定性 | 父/子Frame的StablePrefix/SemiStable/DynamicTail exact `ContentRef{Ref,Digest,Length}` | StablePrefix与SemiStable逐项不变；只允许DynamicTail追加；父Frame不可变 |
| Prefix/cache identity | PrefixDigest、StableSourceSetDigest、完整Manifest SourceSetDigest及key各维度 | DynamicTail改变不改变稳定key；stable ref/recipe/render/model/harness/toolschema/authority/isolation/provider/key-version任一漂移改变key或拒绝；不得用完整Manifest SourceSetDigest替代 |
| Artifact白盒 | unchanged/diff/full、base冲突、写后Inspect | 无currentness不得沿用Anchor |
| 压缩Anchor反例 | RetainedAnchorSet、旧Generation、摘要提及、base/target文件版本 | 未精确保留立即失效；diff冲突重新物化 |
| 大内容/token反例 | 大文件、历史Tool输出、bounded summary、exact artifact ref/version/digest/range、delta chain上限 | 不全量重灌；超预算拒绝/降级；链超限rebase |
| Cache白盒 | partition、TTL、invalidation、经济性、miss reason、大数乘除与write+keepalive溢出 | usage不升级hit；跨分区不命中；先除后饱和值精确且加法不回绕 |
| Injection白盒 | Expected/Actual、opaque/partial、强制字段漂移 | unknown不算matched；强制漂移拒绝 |
| Injection因果反例 | 空Observation、Route/Attempt/Frame/sequence/revision/digest/fidelity、Expected TTL | 只有complete且exact、Expected current才matched |
| Context/Tool Surface链分离 | Context Expected/Actual Conformance与Tool `ToolSurfaceManifest.ExpectedInjectionDigest`使用相同/不同字符串值 | nominal Owner与digest subject不可互换；Context链不调用Surface Gate，Surface Gate不消费Context Expected Manifest |
| Compaction白盒 | 因果、Anchor、Open Effect、不可压缩引用 | Summary不升级事实；新Generation可回放 |
| Compaction exact Prepare | exact Source Frame Range、expected current、Summary/RootFrame、Ordinal+1、规范Ref集合、取消 | 候选Generation确定性且不可见；漂移/过期/乱序/重复/取消零current写 |
| Compaction Anchor反例 | RetainedAnchor exact ID/Revision/Digest、遗漏/换包/过期、旧Generation摘要提及 | 只有exact保留且仍current的Anchor可继续diff；摘要文字不得续命 |
| Outcome合同/白盒 | Frame/Manifest/Recipe/Generation、Model/Tool/User exact refs、token/cache/cost/latency | 规范集合与Digest确定；usage不升级Cache hit；无Task/Runtime Outcome字段 |
| Evaluation/Feedback白盒 | Outcome集合、baseline/candidate Recipe、Policy、ppm分数、Change Digest、状态 | 空/乱序/重复/越界拒绝；单次成功/模型自评不能自动发布 |
| Outcome Store并发/故障 | Put-once、同ID同Digest、同ID换包、cancel、64并发 | exact重放；换包Conflict；cancel零写；单一不可变Fact |
| Recipe pre-release状态机 | immutable Recipe、draft/validated/evaluated/review_pending/rejected、presence | 非法跳转/缺Ref拒绝；历史Fact不变 |
| PromptAsset合同（已执行） | 1..64个规范化instruction/example/policy片段、exact ContentRef、ContentDigest、Owner/Authority/Sensitivity、RenderCompatibility、Evidence、distinct `PromptAssetRefV1` | role确定性映射Kind/Trust；片段/compat/evidence乱序重复、零Length、摘要漂移、未包含Evidence全部拒绝 |
| Prompt Candidate投影（已执行） | exact AssetRef、Execution/Authority、RenderCompatibility、Created/NotAfter、确定性ID/Idempotency | same request逐字节一致；Prompt不设置Region；asset/render/authority/TTL drift零Candidate；Unknown/Unavailable/cancel零产物；64并发一致 |
| Prompt pre-release（已执行） | distinct lifecycle、Put-once/Inspect、expected head CAS、lost reply、64并发 | 单一successor；PromptAsset ref nominal分离；publish/rollback/revoke unsupported且零production current |
| Engineering Prompt入口（已执行） | validate/preview exact Asset/Build、Authority、RenderCompatibility、TTL、limits、codec | 零Store/lifecycle写；不产生Provider message/Frame Region/cache placement；错误零Response |
| Evaluation Prepare（已执行） | 1..64 Outcomes、两侧Recipe、Policy、EvaluatorRef、checked/not-after、nested-ref/canonical limits | same-ID换包、单边样本、乱序重复、Policy/TTL漂移全部拒绝；Input逐字节确定 |
| Evaluator Admission（已执行） | exact Input/Observation、S2 Outcomes、score ppm、Evidence、ObservationDigest | Observation不直接成为Fact；任一binding漂移、Unknown/Unavailable/cancel/deadline零Evaluation |
| Feedback Build（已执行） | exact Evaluation/Outcomes、BaseRecipe、ChangeDigest、risk、TTL | 不改risk/outcomes/base；不自动Review/publish；返回deep-copy/no-alias |
| Engineering SDK Conformance（已执行） | typed/strict JSON、recursive duplicate key、32 MiB canonical/48 MiB wire、64并发 | Offline六operation不变；不联网、不调用Runtime/Harness/Provider、不注册Capability；target100/race20/full ordinary/race/vet PASS |
| Engineering API/CLI（待本轮执行） | 五typed API、五JSON dispatch、`prompt validate|preview`、`evaluation prepare|admit`、`feedback build`、typed退出码 | typed/JSON结果与SDK exact一致；request先过strict codec与48 MiB hard cap；成功仅stdout canonical response，失败零stdout且stderr不含request/content/secret；零server/listener/Store/Capability/Provider/root |
| 官方Prompt Provenance（已执行） | Codex/Gemini/Kimi/Grok官方Coding Agent明文、Claude SDK preset引用、DeepSeek/MiniMax模型template、OpenCode B级；repo/commit/path/range/license bytes/source/transform/closure digest | 同commit/path换bytes、license/range漂移、range越界/重叠、transform断链、closure漏项/跨层重复、zero-byte、cancel全部零Report；Claude无正文不得伪造GeneratedContent；deep-copy/no-alias与64并发确定；target100/race20/full ordinary/race/vet PASS；不自动取得Authority/Review/published |
| T3Code/Model Profile兼容（未执行） | Context exact PromptAsset/Provenance/closure refs + Model Invoker未来exact Profile ref/current projection -> host adapter -> T3 ProviderDriverKind/ModelSelection | Context不import T3Code或复制ModelFamily/Profile nominal；`promptInjectedValues`不成为current/actual；公共Profile reader缺失时production Fail Closed |
| Recipe lifecycle CAS | expected exact head、同ID换包、cancel、64并发后继 | 单一后继；lost reply Inspect head；不产生production current binding |
| Recipe结构diff | identical、ID/version/revision/owner/rule add/remove/change/reorder、budget/render/lifetime | 全字段变化规范排序；同输入同Digest；不产生better/compatible/publish结论 |
| Recipe publish NO-GO | publish/rollback/revoke、普通Review Ref、Run内Settlement type-pun | 全部unsupported；零current binding/零Effect；CTX-D07单列 |
| Offline SDK（已执行） | validate/compare/compile/preview/frame-inspect/cache-inspect | 不暴露Store绕过；Compare不作质量结论；Cache Inspect不生成Plan、不作hit结论；错误类型稳定；详见前述独立已执行矩阵 |
| Offline ingress request codec | 六request Seal/Encode、空或exact RequestDigest、bundle streaming、六组wire cap | Encode→Decode exact；digest漂移Conflict；cancel零payload；不复制canonical/bundle算法 |
| Offline API conformance | 六typed方法、六JSON dispatch、unsupported operation | typed结果与SDK一致；JSON严格性与SDK一致；零Store/Capability/transport/Provider调用 |
| Cache Inspect hard negatives | Plan/Profile expired、same ID换Revision/Digest、Partition/Key漂移、递归duplicate key、null Plan、response tamper、usage冒充hit、64并发 | 全部Fail Closed或逐字节确定；错误零Response；无`cache_hit`字段；Cache Store/Provider/Effect调用0 |
| CLI黑盒 | `recipe validate|compare|compile|preview`、`frame inspect`、`cache inspect`、typed退出码、stdin/stdout/stderr | 成功只输出response；Compare只含结构diff；Cache Inspect只含exact闭包/economics；错误不打印request/content/secret；写命令与未知命令usage拒绝 |
| 故障注入 | Source超时/换包、Store丢回包、CAS冲突、TTL到期、Authority/Fence漂移 | exact Inspect恢复；不盲重派 |
| Effect故障 | Permit过期、Prepare丢回包、Enforcement缺失、Execute unknown、Inspect冲突 | 严格Operation顺序；Begin后只Inspect |
| Settlement/Cleanup | observation≠fact、Owner CAS、cleanup residual、operation_not_required | Runtime Outcome不由Context选择 |
| Conformance | fully/restricted/observe-only/rejected四级 | capability与证据一致，Fake不授予生产级 |
| Race | 并发reserve/create/CAS/hit/invalidate/release | `go test -race`无竞态，单一赢家 |
| Vet | 全生产与测试包 | `go vet ./...`零错误 |
| ordinary100（已执行） | CTX-D10及CTX-D09 contract/kernel/store/fixture/fault/conformance定向包 | `go test -count=100`无flaky，时钟均注入 |
| race20（已执行） | CTX-D10及CTX-D09 Reserve/Inspect/Apply/Generation CAS并发定向包 | `go test -race -count=20`无竞态、无stale success/双赢家 |
| full gates（已执行） | 本模块全包 | `go test ./...`、`go test -race ./...`、`go vet ./...`全部通过 |
| Cache Inspect增量门（已执行） | `contract/sdk/kernel/offlineapi/cmd/context`的Plan/Profile currentness、strict codec、economics、API/CLI、usage≠hit、64并发 | `go test -count=100`与`go test -race -count=20`通过；随后full ordinary/race/vet通过；Provider/Store/Effect调用0 |
| G6B test-only cross-module（已执行） | 手工注入Application三段公共Port + Context Adapter + Memory/Knowledge V2 Adapters | Apply DTO无Runtime settlement/ref；exact Frame含三类来源；不得注册Capability、调用Harness/推进Turn或宣称production root |
| Harness接线（未执行） | 每Turn exact FrameRef/Digest、不可物化Reference | 私有Adapter无网络后门；Fail Closed/Residual正确 |
| G6A→G6B门禁 | 已证明G6A/CTX-D10/`CTX-D09-R1` A/B→Application三段DTO/Reader→Context Adapter | B-cross已完成；无Harness/Tool/Application实现import，production仍NO-GO |
| 依赖反转/root分层（B已执行/C未执行） | Context Adapter只依赖Application公共contract/ports；B-cross fixture手工注入，C层production root未注入 | Application/Harness不import Context实现；fixture不冒充真实集成；Context不构造Continuation |
| C层production启用门禁 | production root缺失或G6B验收前尝试capability enable、Harness Continuation、Turn CAS | 全部Fail Closed且无外部调用；只有production composition root与验收完成后才GO |
| 冻结范围 | N>1 Tool、通用refresh、Checkpoint、Continuity来源及Memory/Knowledge>1 | 明确CapabilityUnavailable；不得落入N=1路径 |
| Model Invoker接线（未执行） | RouteID/routegateway/公开union、Actual Manifest/cache usage | 无internal/native依赖；usage仅Observation |
| Continuity public Reader合同（未执行） | 复用`TimelineOwnerFactRefV1`/Checkpoint V2 exact refs；Continuity public typed Owner-current Reader/Router或唯一facade | 禁止依赖`continuity/runtimeadapter`、复制nominal或用快照冒充current；公共Port未冻结即NO-GO |
| Continuity typed Owner Reader conformance（未执行） | Frame/Generation/Outcome exact ref S1/S2、TTL、scope/current pointer drift、cancel/deadline/lost read | 漂移/未知零Event；Context只返回current projection，不写Evidence/Timeline/RocksDB、不分配sequence |
| Continuity/Artifact跨模块（未执行） | Context Apply后Owner refs投影、Anchor/diff、Generation/Rewind、RocksDB SPI边界 | Context不分配Timeline sequence/写RocksDB；SPI不宣称Backend/SLA；Rewind不回滚外部世界 |
| TTL最小值反例 | request NotAfter、Recipe、ParentFrame/Manifest/Generation、ToolResult、Binding/Authority、Cache/Profile、Artifact有效期边界 | Frame/Generation/Cache expiry等于严格最小值；`checked >= expires`不可消费，不伪造/延长TTL |
| G6B Runtime反例 | Apply DTO携带V4、additive Runtime settlement、Tool settlement或任意Runtime ref | 全部strict decode/validate拒绝；零current pointer、零Harness、零Runtime调用 |
| 系统Direct API（未执行） | Shadow→controlled switch、真实Route可选smoke | 需显式live开关/凭据；未跑不宣称 |
| 系统官方Harness（未执行） | opaque/partial/hidden compaction/residual | Conformance诚实，不补造事件 |

## 建议命令

```bash
go test -count=1 -shuffle=on ./...
go test -count=100 ./...
go test -race -count=20 ./...
go test -count=1 -race -shuffle=on ./...
go vet ./...
go test -coverpkg=./contract,./ports,./kernel,./applicationadapter,./runtimeadapter,./sdk,./api -coverprofile=/tmp/praxis-context-engine.cover ./...
```

集成和系统测试必须使用各Owner批准的公开Adapter；不得通过导入Harness/Model Invoker内部包让测试“通过”。
