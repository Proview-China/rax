# Context Engineering与Cache验收合同

状态：规划内无需production composition root的Owner-local/B-cross/reference-only切片均已YES，包括Wave 1离线核、`CTX-D10`、`CTX-D09`、Application公共Port Adapter、Memory/Knowledge B-cross fixture、Offline/Engineering SDK、Compaction/Generation、Outcome、Prompt Provenance、durable Reviewer Context、Restore Context materialization与Component Release候选。full ordinary、target100、race20、full race及vet均通过；production C层未启用。

## 1. 设计与边界验收

- `ContextRecipe / ContextManifest / ContextFrame / ContextOutcome`分离；
- Owner/非Owner、公共/私有Port、DAG、Slot/Phase贡献、Effect、Run Requirement全部明确；
- domain/kernel只依赖自身合同；Runtime Adapter只依赖`runtime/core`与`runtime/ports`；
- 不依赖Harness私有Port、Model Invoker internal/厂商SDK/Raw事件；
- 不修改或重造Runtime Operation V3、Review、Evidence、Binding、Run Outcome；
- V1明确不产生pre-run Runtime Evidence。

## 2. Effect/Review/Fence/Unknown矩阵

| 动作 | Effect | Review | Fence/Currentness | Unknown恢复 |
|---|---|---|---|---|
| 本地Recipe validate/compare/compile/preview与Frame inspect | 否；非权威诊断 | 否 | 本地资产Digest | 不写事实 |
| G6A ParentFrame current读取 | 否；严格只读 | 否；不授Evidence资格 | exact Source/Frame/Manifest/Generation、scope、S1/S2、短租约 | NotFound/Unavailable/Conflict/漂移均Fail Closed；零Evidence Issue/Tool watermark/Provider |
| G6B N=1本地Refresh/Apply | 否；本首切面无远程Source/Provider Effect | 不新增Review；只消费G6A exact-current输入 | settled Tool exact chain、pending DomainResult、S2、原子ApplySettlement+Generation current CAS | S2失败current pointer不可见；Unknown/lost reply只Inspect原Attempt |
| Run内本地Frame编译/内容寻址读 | 否 | 按Admission政策，不授予执行权限 | Scope/Authority/Source currentness | Create/CAS丢回包Inspect exact Attempt |
| 本地Frame/Manifest State Plane CAS | 领域事实写；不伪装Runtime Effect | 由发布/运行政策决定 | Owner/Scope/ExpectedRevision | Inspect exact ID，换内容冲突 |
| 远程Source/Reference解析 | `context/remote-source-resolve` | 明确required/not-required | Operation V3 Permit+Fence+执行点复验 | Begin后只Inspect原attempt |
| Model请求内Provider Cache read | 被父Model Operation覆盖 | 跟随父Operation | 父Permit/Fence | 跟随Model attempt Inspect |
| 独立Provider Cache query/create/write/refresh | 对应`context/provider-cache-*` | 明确required/not-required | Permit/Fence/Binding/TTL | Begin后只Inspect；usage不是hit |
| Cache invalidate/delete | 对应Cleanup Effect | 明确required/not-required | Permit/Fence/Owner currentness | Inspect/Settlement，Residual显式 |
| 本地Compaction | 无外部Effect | Recipe policy | Source Frame/Generation currentness | CAS丢回包Inspect |
| 远程/计费Compaction或Evaluation | `context/remote-evaluation` | 明确required/not-required | Permit/Fence/Budget/Disclosure | Begin后只Inspect |
| Recipe/Prompt publish/rollback/revoke | `context/recipe-release`等 | 必须有Verdict或显式not-required | admin/custom subject currentness；CTX-D07待裁决 | Inspect Effect与Release CAS |

所有真实外部Effect严格复用Runtime治理链。CTX-D09本地迁移不是外部Effect，且`CTX-D09-R1`已冻结为零Runtime Settlement；只走`pending DomainResult → S2 → atomic ApplySettlement+Generation current CAS`。

## 3. 对象与状态机验收

- 所有Fact具有Contract/Schema Version、ID、Revision、Digest、Owner、Scope、Authority和TTL；
- 同ID同Digest幂等；同ID换内容冲突；CAS只允许revision+1；
- Required Source漂移/不可用使Frame失败；Optional降级留下原因和Residual；
- Manifest冻结后不添加来源；Frame冻结后不改变渲染；
- Compaction产生新Generation并保留因果/Anchor/Open Effect；
- Artifact diff绑定base/target；冲突时重新物化；
- Cache Plan不等于Entry，Receipt/Usage不等于hit；
- Expected/Actual不可观察时为unknown，不推断matched；
- Prompt反馈不自动发布。
- G6A必须提供settled `ToolResultV2`、current V4 Inspection与verified Association；缺一或绑定漂移时G6B不得启动；
- G6A exact输入、CTX-D10与`CTX-D09-R1`均已冻结；A层Context Owner-local kernel/store与本组件B层fixture已完成并通过二轮独立复审，两者均不以production root为前提；
- Application已发布三段`ContextTurnRefreshPortV1`及Memory/Knowledge中立Owner Source Reader；Context Owner Adapter只依赖Application公共`contract/ports`，Application与Harness不import Context实现；
- 三段必须按`核验settled Tool exact chain → Refresh/pending DomainResult → Apply内S2 fresh复读 → 原子ApplySettlement+expected Generation current CAS`执行；`Inspect`只读恢复原Attempt；
- Apply DTO和本地状态机必须为零Runtime Settlement；不得扩展/使用V4、引入additive Runtime settlement或复用上游Tool settlement；
- S2失败、TTL crossing或Owner-current漂移时，最多保留pending/diagnostic fact；ApplySettlement成功与Generation current pointer必须原子可见，任何单边可见均验收失败；
- 首切面Source基数必须为`Tool=1 / Memory=0..1 / Knowledge=0..1 / Continuity=0`；Context不构造Continuation、不推进Turn；
- Memory/Knowledge必须经各Owner唯一public V2 Reader与Application中立Port执行S1/S2、exact content observation和association校验；Context平行Owner nominal、Owner concrete Store/internal依赖、正文总量超过每Owner64KiB或错误被降格均为Fail；`knowledge_reference`只能是受限DynamicTail材料；
- Source Turn exact ref必须来自具名Session/Turn Owner Reader，其`Ordinal=T`必须exact等于settled Tool/ExpectedCurrent `uint32` Turn；Target T+1只能由Context childExecution生成。Transition proof唯一Owner为Context，Application只编排不 mint；Memory/Knowledge自行`+1`、补造Session/Turn或产生proof时Fail Closed；
- 必须按`pending Frame/Generation seal → Context proof → S2 → atomic Apply/CAS → publish`顺序；stable closure不含phase/time/self-digest，fresh observation必须含phase、Checked/Expires与current projection digests；S2漂移后proof不授予publish资格；
- RefreshAttempt/Frame/Generation ID确定性；S1/S2复读、expected Generation CAS及lost reply exact Inspect均通过；
- 父Frame不可变，StablePrefix/SemiStable逐项复用exact `ContentRef{Ref,Digest,Length}`，只有DynamicTail变化；PrefixDigest必须seal完整StablePrefix，stable cache identity不得使用随DynamicTail变化的完整Manifest SourceSetDigest；
- Source Reader只允许类型化Tool exact链、有界summary或exact Artifact `{Owner,Version,Digest,Range}`；raw内容、Provider receipt/Observation不得直接注入；
- 压缩后未被新Generation精确保留的旧Anchor失效；
- Frame/Generation/Cache TTL严格等于请求NotAfter与ToolResult、ParentFrame/Manifest/Generation、Recipe、Binding/Authority、Cache/Profile、Artifact currentness的最短有效期；`checked >= expires`拒绝；
- Expected过期、Actual Observation缺失或Route/Attempt/Frame/sequence/fidelity漂移绝不matched。

### 3.1 CTX-D10 G6A ParentFrame只读验收

- nominal coordinate必须为distinct Context类型；方案A固定`ID=FrameID`、`Revision=Frame Revision`，Digest seal exact Frame/Manifest/Generation+ordinal、ExecutionScope/Run/Session/Turn、Parent binding、Recipe与Authority；禁止只哈希Frame ID；
- `ID=FrameID`仅作Owner metadata index首个查询key，不假设跨Tenant/ExecutionScope全局唯一；Owner必须按完整Source四元组解析唯一sealed binding；
- metadata reader必须按完整`Frame/Manifest/Generation FactRef{ID,Revision,Digest}`和预期ExecutionScopeDigest exact读取，不得`FrameByID`后接受任意current值；
- 取回Frame必须重新计算ExecutionScopeDigest并与请求、sealed binding和Runtime公共projection一致；同ID换Revision/Digest、跨Tenant/scope歧义或多结果均Conflict/Fail Closed；
- S1和S2均复读Source binding、Frame/Manifest/Generation、Generation current pointer、Recipe/Authority上界及ReferenceStore，并完整执行`kernel.InspectFrame`；ReferenceStore内容改变不得成功；
- TTL必须等于请求上界、`checked+30s` cap及所有实际可读领域current上界的最小值；边界时刻和任何上界不可读均不得返回`Current=true`；cap不构成SLA；
- Runtime adapter只接收公共四元ref并调用Owner Reader；不得在构造参数保存Frame/Manifest/Generation、sealed binding map或快照冒充current；
- 公共ref本身不授Evidence资格；NotFound、Unavailable、Conflict、漂移、缺Kind router均产生零Evidence Issue、零Tool watermark、零Provider调用；
- CTX-D10不新增RunStart/RunSettlement Requirement；只响应Runtime既有Action矩阵中Context=`required`的读取，activation Context=`forbidden`时不得注册或调用该Kind；
- CTX-D10 Reader/Adapter/Fake已实现并通过隔离验证；仍不宣称production backend/root/SLA，也不自动创建ContextTurnRefresh、DomainResult、Settlement、Apply、Generation CAS、新Frame或Continuation。

## 4. Run Settlement、Cleanup与Residual

- `context/frame-closure` completion requirement证明Run引用的Frame Attempt已达到领域终态；
- `context/effect-cleanup` termination_report requirement证明Context/Cache Effect已Settlement/Cleanup；无操作时必须精确policy `operation_not_required`；
- Context只返回Participant Fact，不选择Runtime Outcome；
- Unknown remote effect、Remote Residual和Cleanup分别记录；
- 官方Harness opaque prompt/hidden compaction/unobservable cache只能是Residual或unknown；
- Authority/Secret/Workspace/强制Instruction/Tool Surface漂移不得作为Residual放行。
- Continuity只事后投影Context Owner facts；Context不创建Timeline sequence、不直接写RocksDB，RocksDB SPI不计为生产State Plane。

## 5. Conformance

| 等级 | 条件 |
|---|---|
| `fully_controlled` | exact Frame物化、Expected/Actual可比较、所有Effect可Fence/Inspect/Settle |
| `restricted_controlled` | 官方Harness存在已声明Residual，但强制边界仍可验证 |
| `contained_observe_only` | 只能观察Actual/Cache Usage，不能保证注入控制或引用物化 |
| `rejected` | ContextReference不可物化且Required、持久Effect不可Fence/Inspect、Secret暴露或未知网络 |

Fake、内存Store和离线Provider只证明合同，不授予生产Conformance、Backend或SLA。

## 6. 性能与Cache目标

- 先验证语义正确性，再评测Cache；
- 90%只作为`cache read tokens / cache-eligible prefix tokens`目标，不是全部input token或已承诺SLA；
- 必须记录Prefix变化位置、miss原因、read/write/dynamic token、成本和延迟；
- 无法观察的Provider cache保持unknown；
- 不做无经济证明的预热/续期请求。

## 7. 完成门槛

Wave 1离线核已通过合同、单元、白盒、黑盒、故障注入、Conformance、count100、race20、full race与vet；`CTX-D10`及`CTX-D09`本组件A层/B层fixture也已完成隔离验证并获二轮独立复审YES。这些事实不授予生产Backend/SLA、Application公共Port、G6B跨模块验收或per-turn启用资格。

分段裁决：CTX-D10、`CTX-D09-R1` A/B-local、Application公共Port Adapter及Memory/Knowledge B-cross fixture已完成；C层production Capability、真实跨模块composition、Harness Continuation与Turn推进继续NO-GO，只有production composition root完成且G6B验收通过后才GO。

## 8. ContextOfflineSDKV1验收（独立软件验收YES）

- Offline SDK operation当前严格为六个：`ValidateRecipeV1 / CompareRecipesV1 / CompileFrameV1 / PreviewFrameV1 / InspectFrameExactV1 / InspectCachePlanV1`；不存在质量评测/replay/refresh、Provider调用或Cache写operation。`CompareRecipesV1`只允许结构diff，不得输出`better`、兼容或发布结论；`InspectCachePlanV1`只核验调用者给定Plan/Profile，不生成Plan或hit；
- exact Go structs/tags、Operation六值闭集、required-key presence bitmap与optional指针严格符合设计；Compare的两个Recipe必须非null且`Rules`非null，`MaxRecipes`精确允许2；Cache Inspect的Plan/Profile必须非null且exact；既有optional field与非nil empty slice规则不变；
- Compare的Checked/Expires不得越过任一Recipe真实有效期；Changes必须present、field-path唯一排序且不超过80项，相同Recipe返回`[]`；任何正文泄露、`better/compatible/publish`结论或TTL延长均Fail；
- RequestDigest、ResultDigest、Report/Comparison/Compile/Preview/Inspection Digest按规定own-digest置空与排除集等式逐一重算；nil/empty、下层digest或等式漂移均零Response；
- SDK只依赖Context `contract/kernel`与标准库；不依赖Runtime/Application/Harness/其他Owner实现或私有Port；
- Compile仅产生请求内ephemeral、`Authoritative=false` bundle，Owner Store/CAS、Generation current、DomainResult、Settlement、Capability和外部调用计数均为0；
- Preview只返回结构、token、Admission与exact refs/digests，不返回任何原始正文；
- Inspect对missing content、digest/length漂移、Manifest/Frame/ref/binding漂移、expiry crossing全部Fail Closed；`Exact=true`不得被解释为Owner current；
- Cache Inspect必须在Plan与Profile真实有效期内核验；输出PlanRef、PartitionDigest、KeyDigest、ProviderProfileRef、EconomicDecision和最短Expires。过期、ref/digest/partition/key漂移、递归重复键、cancel、response tamper全部零Response；`usage/cached_tokens/Current=true`均不得解释为cache hit；Provider/Cache Store/Effect调用必须为0；
- typed Go入口不声称检测duplicate key；六个`Decode*RequestV1` codec递归拒绝unknown/trailing/duplicate key。Wire cap按operation/方向：Validate 48/48 MiB、Compare 48/48 MiB、Compile 48/144 MiB、Preview 144/48 MiB、Inspect Frame 144/48 MiB、Inspect Cache 48/48 MiB（request/response）；全部旧40 MiB或统一request=48描述视为stale；
- 三预算逐项生效：input raw=24 MiB/1024 items；Compile-derived generated<=52 MiB、output<=76 MiB；68 MiB/4 items与100 MiB/1028 items只是independent global guards；non-content wire=4 MiB。任一operation-derived/request/global/wire cap或算术溢出均`limit_exceeded`+零Response；
- `OfflineContentBundleV1`构造时deep-copy；所有请求入站和Response出站deep-copy/no-alias，修改输入/返回slice不改变其他结果或Digest；
- typed error code严格闭集；除Validate的`Valid=false`诊断外，任何error返回零Response。纯离线路径无Unknown/Unavailable；
- SDK不得直接调用无context的live `kernel.Compile/InspectFrame/ReferenceStore`；必须先以唯一共享helper落地context-aware staged kernel/store，旧API只作wrapper，SDK不复制算法。Cancel检查覆盖strict token/chunk、candidate/content、sort/admission/budget/render/Inspect/store clone、seal/return；任一无context整包处理直接Fail；
- workspace构造后、`Begin`前必须立即`defer Destroy()`；`Destroy(new)`合法、无ctx且幂等，Begin失败也必须最终destroyed。Begin成功后再`defer Abort()`；Partial Put、cancel、limit或Inspect失败的Ref/bytes全部不可达，只在成功Export后深拷贝sealed response snapshot。旧Compile/Inspect wrapper不承诺cancel；禁止`goroutine+select`假取消；
- streaming renderer的Stable/SemiStable/Dynamic/Rendered bytes、ContentRef与Digest必须与live旧renderer golden逐字节相同；
- 独立base64 primitive golden覆盖0/1/2、48 KiB-1/exact/+1和96 KiB双chunk；零字节只允许`[]↔[]` primitive round-trip，不构造ContentRef/ContentItem。URL/raw/no-padding/whitespace/empty string/short non-final/redundant chunk拒绝且零产物；
- `OfflineContentItemV1`/Bundle中的每个Ref必须通过live Validate且`Length>0`，bytes必须非空并与Ref exact；零长度Ref或空bytes构造必须`invalid_argument`、零Bundle。Required空内容没有合法非零Ref时Fail Closed，不能借primitive空编码绕过；
- required/effective-required content missing只在SDK边界返回`not_found`+零Response；optional missing复用live `AdmissionResidual(content_unavailable)`和规范`ResidualCandidateRefs`，零Fragment/零token/不读Owner Store；共享helper/wrapper改写Owner error/reason直接Fail；
- 512-candidate live `sort.SliceStable`只在调用前后检查取消，不声明comparison公式上界；必须对已排序/逆序/全相等/重复/交错/确定性乱序实测comparisons与耗时并建立同Go版本保守防回归阈值；
- Offline SDK的`MemorySources/KnowledgeSources/ContinuitySources`固定为0，Memory/Knowledge Reader调用数为0；`knowledge_reference`在单独候选确认前必须`unsupported`且零Response；
- 相同输入、候选乱序、map乱序、100轮及small fixture 64并发均逐字节确定；max-size fixture只跑1/2/4/8并发并采集内存/时间，不用于宣称SLA；race20/full race无共享状态竞态；
- Fake/ephemeral workspace不宣称production Backend、State Plane、SLA或Conformance等级。第二轮独立审计六项缺口已返修并经后续独立复审YES：typed入口在任何slice/bytes clone前执行零分配成功路径预检；wire先流式提取meta并执行request-specific wire cap，再以token scanner在DTO `[]string`物化前累计chunk/item/raw上界；canonical marshal/digest/response encode按64 KiB保留cancel/deadline；required slice null与Decision Region条件presence fail closed；Compile 24/52/76与global 68/100边界、renderer/workspace故障矩阵均已实跑。
- 最终实现实跑full ordinary `count=100`、full race `count=20`及`go vet ./...`均PASS；max-size fixture input=`25,165,824`、generated=`53,129,367`、output=`78,295,191`、wire=`104,407,083` bytes，1/2/4/8并发PASS，最高VmHWM=`3,285,408 KiB`，mid-cancel=`807.57 µs`。这些数字只作本机有界证据，不声明SLA；独立软件验收不解锁G6B或production C层。
