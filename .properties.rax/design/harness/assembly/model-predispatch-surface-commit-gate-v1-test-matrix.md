# Model PreDispatch Surface Commit Gate V1测试矩阵

状态：Harness M2 `A2+B1+C2`与concrete Gate/ACK Repository均已达到`owner-local implementation_software_test_yes`并通过对应独立代码审计。MPG-01~MPG-22及Owner-local M2/Gate反例已有实现证据；MPG-23~MPG-38的Model actual-point全路径no-bypass、专用Assembly Capability/Conformance、system G6A及production root仍未完成，不得由Owner-local绿测替代。

| ID | 层级 | 场景 | 必须结果 |
|---|---|---|---|
| MPG-01 | unit | Model公开完整Prepared Fact/Ref与Current Ref/Projection canonical | Fact含Plan/RequestTools/Route/Profile canonical digest、完整Registry exact Ref与Historical NotAfter；Current含完整Prepared回链、双actual digest及Checked/Expires/NotAfter |
| MPG-02 | unit | Historical NotAfter已过但Fact仍可读 | 历史Inspect成功；Current/Tool不得超过该上界；Commit和Provider attempt拒绝 |
| MPG-03 | unit | Retention不可读与资格expiry | 两者都Fail Closed但错误语义不混同，不伪造TTL |
| MPG-04 | unit | Harness single Runtime-neutral composite watermark | Projection含Generation/Handoff/BindingSet exact refs、Profile、完整Registry Ref及组合Semantic/Currentness/Watermark；A2/B1/C2不产生第二composite，Tool完整保存不拆解 |
| MPG-05 | unit | composite Assembly窗口 | 严格A2→B1→Handoff→C2→Registry读取；Checked=`max(A,B.Binding.Issued,Handoff,C)`，Expires=`min(A,B.Fact,Handoff,C)` |
| MPG-06 | whitebox | A2完整CompileResult/Conformance读取Profile与ToolSurface | exact Ref等于sealed Manifest Plan；完整Surface再经C2 Tool Owner Reader复读 |
| MPG-07 | whitebox | 只给格式有效ObjectRef、自签Projection或raw AssemblyInput | 零Projection、零Tool Ensure、零Provider |
| MPG-08 | unit | Tool Surface expected与Model ActualToolSurface不等 | Conflict，零Commit、零Provider；不与Context Expected比较 |
| MPG-09 | unit | Historical PreparedPlan/RequestTools/Route/Profile digest或完整Registry Ref漂移 | Conflict，零Commit、零Provider |
| MPG-10 | unit | composite内任一exact ref、Semantic/Currentness/Watermark漂移或ABA | Conflict，零Ensure、零Provider |
| MPG-11 | boundary | Harness尝试传完整Tool Fact/Created/NotAfter/Digest | public Writer不可表达；编译/合同测试拒绝 |
| MPG-12 | unit | Tool Owner公开Ensure request | Harness调用`EnsureToolSurfaceInvocationBindingV1`；Tool Clock生成Created；NotAfter只取显式Owner/caller上界的min；Fact/Ack由Tool Seal |
| MPG-13 | fault | Tool Ensure已提交但回包丢失 | 只Inspect同Invocation；exact恢复Fact/Ack；不换ID、不重选Surface |
| MPG-14 | fault | Commit/Inspect Unavailable、Indeterminate或NotFound | Fail Closed；不盲重提；Provider 0 |
| MPG-15 | concurrency | 64 Gate实例同Invocation同请求 | Tool单一Fact；同Ack幂等恢复；不声称Ack授一次进入权 |
| MPG-16 | concurrency | 64 Gate实例同Invocation不同Surface/Injection/watermark | 单一内容胜出，其余Conflict；冲突调用Provider 0 |
| MPG-17 | time | S1/S2/attempt前clock rollback | `ReasonClockRegression`或等价existing reason；Provider 0 |
| MPG-18 | time | stored ACK命中后ACK或Prepared Current已过期，或复读途中跨TTL | lookup仍必须第一步；随后复读同一Prepared Current并取fresh Owner clock，任一不满足`checked <= now < expires/not_after`即PreconditionFailed；Tool/Ensure/重Seal调用数0 |
| MPG-19 | blackbox | Phase A调用registry/provider/backend动态方法 | 测试失败；Provider/Backend call count必须0 |
| MPG-20 | blackbox | sealed registry capability snapshot完成纯映射 | Historical fields与Current双actual digest稳定进入各自canonical |
| MPG-21 | blackbox | ACK后Provider Capabilities与snapshot不同但不改变Tools或双actual digest | 可记录Observation并继续，仍需其余current有效 |
| MPG-22 | blackbox | ACK后Capabilities会改变Tools或任一actual digest | 拒绝当前Invocation；不得重Seal或覆盖Binding |
| MPG-23 | actual-point | `execution.Runtime.Start→Adapter.Preflight` | Preflight前Inspect exact Ack与全部current |
| MPG-24 | actual-point | direct `Preflight→Backend.Resolve` | Resolve前Inspect；ACK前Resolve count=0 |
| MPG-25 | actual-point | direct `Open/Invoke/Stream` | 每次attempt分别Inspect；一次Ack不是永久进入权 |
| MPG-26 | actual-point | generic `Invoker.prepare→Capabilities` | Capabilities在ACK后；其结果不得改变sealed Tools或双actual digest |
| MPG-27 | actual-point | routegateway `Capabilities/Invoke/Stream` | 每个Provider lease actual call前统一guard |
| MPG-28 | actual-point | operation/composite Provider路径 | 子Provider Capabilities/Invoke/Stream前统一guard |
| MPG-29 | continuation | direct continuation Input/State变化、Tools与双actual不变 | 重新Inspect exact current后可继续 |
| MPG-30 | continuation | continuation Tools/Surface/任一actual digest变化 | 当前session拒绝，要求新Invocation epoch |
| MPG-31 | realtime | native websocket `Open/DialContext` | Open前Inspect；ACK/current过期时dial count=0 |
| MPG-32 | conformance | hosted/remote/provider adapter任一raw路径 | 缺同Ack guard即Generation rejected |
| MPG-33 | conformance | 只有`model.dispatch.before` HookFace | rejected；compile-time声明不能冒充runtime Gate |
| MPG-34 | compatibility | legacy ModelTurnPort/Loop canonical | 字段和digest不变；旧Generation不自动获得Gate能力 |
| MPG-35 | boundary | Application V2/P3/PendingAction扫描 | 不新增Surface、Writer、Gate字段或Owner import |
| MPG-36 | boundary | Harness复制Model/Tool DTO或import internal/store | import/conformance测试失败 |
| MPG-37 | recovery | 重启后同Ack/Binding | Inspect exact historical Fact，再复读全部current；不刷新TTL |
| MPG-38 | negative-system | 任一公共Delta或no-bypass路径未闭合 | Tool P4、system G6A、Capability、production root全部NO-GO |
| MPG-39 | boundary | Context ExpectedInjection被接入Surface Binding等式或TTL | 测试失败；Context Injection Conformance保持独立 |
| MPG-40 | unit | 只给Prepared Ref、按Prepared查latest Current | 接口无法表达或Fail Closed；必须显式exact Current Ref |
| MPG-41 | unit | Current/Tool NotAfter超过Historical NotAfter | PreconditionFailed，零Commit、零Provider |
| MPG-42 | contract | Harness定义HistoricalProjection、Plan/Tools/Route/Profile/Registry镜像或缩水CurrentRef | import/schema/conformance拒绝；只允许Model公开完整Fact/Ref/Reader nominal |
| MPG-43 | unit | Registry ref缺Owner/ContractVersion/ID/Revision/Digest任一项 | Invalid/Conflict，零Commit、零Provider；不得用裸digest或latest补齐 |
| MPG-44 | unit | Current Ref缺ContractVersion、Prepared、Checked、Expires或NotAfter任一公开字段 | Invalid/Conflict，零Owner后续读取、零Commit、零Provider |
| MPG-45 | conformance | Harness发布第二Surface composite current或Tool拆解后重签watermark | rejected；一个Harness `ModelPreDispatchAssemblyCurrentProjectionV1`是唯一composite current |
| MPG-46 | contract | request注入不能追溯到Owner current/Fact或caller的匿名NotAfter上界 | public合同无法表达并拒绝 |
| MPG-47 | boundary | Harness/Application定义或消费第二套Memory/Knowledge neutral DTO/current | rejected；Context Adapter只能接收live Memory/Knowledge contextsource V1 Reader或其唯一替代facade |
| MPG-48 | boundary | 首个G6B开启Memory/Knowledge Source或Harness直连Reader/body/citation | Fail Closed；Sources=0、两Reader调用数0，Harness只消费Context current exact Frame |
| MPG-49 | contract | Harness Gate方法集或Tool调用名漂移 | compile-time拒绝；Gate只实现Model短名`Commit + InspectExactAck`，Tool只调用公开Ensure+Inspect |
| MPG-50 | unit | composite canonical循环或摘要字段回流 | Watermark排除Watermark、Ref.Digest、Ref.ProjectionDigest与Projection.ProjectionDigest；Projection包含已算Watermark且清空三项own digest |
| MPG-51 | unit | same ID+same revision换内容 | Conflict；不得覆盖current index或产生ABA |
| MPG-52 | unit | expected revision+1 CAS合法推进 | immutable新revision写入与current index替换同一线性点；旧revision历史可读但ValidateCurrent失败 |
| MPG-53 | unit | Currentness遗漏A2、B1 canonical Gateway、Handoff、C2或Registry任一轮输入/输出digest，或Semantic遗漏Profile/完整Registry Ref | Seal/Validate失败，零Tool Ensure、零Provider |
| MPG-54 | boundary | Harness接收Memory/Knowledge SourceTurn、TransitionProof或自行补Turn | rejected；SourceTurn exact ref来自Session/Turn Owner Reader，Context唯一拥有Proof，Application只协调，Harness只消费final T+1 current Frame |
| MPG-55 | unit | ACK Repository同Prepared+Current stable key同canonical重投 | create-once幂等返回同ACK deep clone；记录数1 |
| MPG-56 | unit | ACK同stable key换SurfaceBindingRef/GateImplementationRef/Checked/Expires | Conflict；不覆盖、不新建第二ACK |
| MPG-57 | fault | Tool Ensure成功后进程中断、Model ACK未写 | 重入第一步先InspectByPreparedCurrent得到authoritative never-created，再恢复Tool winner、S2、fresh clock并Ensure同canonical ACK；不宣称跨仓原子 |
| MPG-58 | fault | Tool Ensure或ACK Ensure回包丢失 | Commit重入始终先InspectByPreparedCurrent；之后分别只Inspect原Invocation和原Prepared+Current stable key/AckRef；Unavailable/Indeterminate不盲写 |
| MPG-59 | conformance | ACK Ensure/Inspect/internal lookup由不同实例或cache实现 | rejected；三能力必须同一concrete、锁域及`by_ack_id/by_prepared_current/by_prepared_ref`索引 |
| MPG-60 | boundary | `InspectExactAck`触发clock、Tool Ensure或写入 | 测试失败；只读exact clone，零其他调用 |
| MPG-61 | contract | Tool Exact Reader要求BindingRef+AckRef双参、Kind或Invocation | rejected；唯一参数是Model ACK公开neutral SurfaceBindingRef |
| MPG-62 | unit | neutral SurfaceBindingRef Owner/Contract/ID/Revision/Digest漂移 | Conflict；Harness只做无损映射，不按Kind猜源 |
| MPG-63 | import | Tool直接import Harness assemblycontract | 当前DAG rejected：Harness Gate将依赖Tool，反向import形成SCC；Harness与Tool只依赖Runtime ports neutral type |
| MPG-64 | unit | Tool Binding直接嵌入Runtime public composite Projection | 完整Generation/Handoff/BindingSet/Manifest/Conformance/Watermark/ToolSurface/Profile/Registry/Semantic/Currentness/time/ProjectionDigest exact，无转换、无第二current |
| MPG-65 | contract | Tool定义neutral echo/alias/Kind或遗漏/修改Runtime composite任一字段 | import/schema/conformance拒绝，零Model ACK、零Provider |
| MPG-66 | recovery | `InspectByPreparedCurrent`已存在fresh stored ACK | 第一项Owner调用先命中；随后Prepared Current exact Reader与fresh Owner clock必须各执行并验证ACK+Current freshness；成功返回stored deep clone，Tool Reader/Ensure、Assembly/Surface Reader、ACK Ensure与重Seal调用数均0 |
| MPG-67 | conflict | 同PreparedRef换CurrentRef创建ACK | `by_prepared_ref`命中不同Current，Conflict；单epoch记录仍1 |
| MPG-68 | fault | ACK lookup Unavailable/Indeterminate或只有单索引NotFound | Fail Closed；不得取clock、Seal或Ensure；只有同实例两个stable索引共同证明never-created才继续 |
| MPG-69 | contract | Harness/Model/Tool定义私有Registry Ref、string Owner或任一旧Model Registry nominal | schema/import拒绝；只允许`runtimeports.RegistrySnapshotRefV1`且Owner为完整`core.OwnerRef` |
| MPG-70 | contract | Current Ref遗漏或splice ToolSurface/Profile/Registry/Semantic/Currentness/Checked/Expires/ProjectionDigest任一字段 | Validate/ValidateAgainst Conflict，零ACK、零Tool；Ref与Projection必须逐字段exact闭合 |
| MPG-71 | canonical | Current Ref/Projection将自身Digest或ProjectionDigest回流canonical body，或三份最终digest不相等 | Seal/Validate拒绝；计算时清空Projection own digest及Ref两项own digest，最终`Ref.Digest=Ref.ProjectionDigest=Projection.ProjectionDigest` |
| MPG-72 | whitebox | raw caller Manifest/Conformance/ToolSurface/Profile均shape合法但不是A2完整CompileResult/Conformance输出 | A2 Reader交叉失败；零B/Handoff/C/Registry调用、零CAS，不能直接Seal caller值 |
| MPG-73 | integration | Association仍active但某BindingSet成员Grant内容或TTL已漂移 | 真实Runtime `GenerationBindingAssociationGatewayV1`本次Inspect重建完整BindingSet后拒绝；禁止独立BindingSet Reader或static Fact fake |
| MPG-74 | whitebox | A2 Manifest Plan ToolSurface exact，但C2 Reader缺失、过期或返回另一完整Manifest | Fail Closed；caller Ref不授current资格，零Registry/CAS |
| MPG-75 | fault | CAS UnknownOutcome且Owner TTL很长、caller无deadline | recovery仍被独立host hard cap终止；只Inspect同一next Ref，不无限等待 |
| MPG-76 | unit | stable ID发生A→B→A并试图CAS回旧revision/watermark | 真实CAS拒绝ABA；旧历史可读但不得重获current |
| MPG-77 | concurrency | 64个不同内容竞争同一stable ID/revision | 恰一winner，其余Conflict；current/history无覆盖 |
| MPG-78 | cancellation | Current Reader在blocking clock返回后、return前ctx取消 | 返回取消，零错误current成功；post-clock复查可观测 |
| MPG-79 | tamper | verified Compile/Manifest/Conformance各自Validate成功但跨Owner拼接 | exact lineage交叉拒绝，后续ToolSurface Reader/CAS调用数0 |
| MPG-80 | contract | Handoff current Seal输入错误非零ContractVersion | Invalid/Fail Closed；不得覆盖为当前版本后通过 |
| MPG-81 | deep-clone | 修改A2输入或Ensure/CAS/Historical/Current返回中的Compile pointer、Manifest/Graph slices、Diagnostics/Residuals、Conformance pointer/slices | Harness A2 Store history/current逐字段不变，无alias |
| MPG-82 | conformance | static/cache/self-built Reader返回shape-valid Fact并通过Runtime public conformance | 仍拒绝M2 integration/acceptance/production composition；conformance必须报`ProductionClaimEligible=false`且仅用于窄Reader接口单测，不得作为Gateway身份证明 |
| MPG-83 | contract | C2把`ToolSurfaceManifestCurrentRepositoryV1`/Ensure注入Harness，或Reader只返回digest/echo而非完整`ToolSurfaceManifestCurrentProjectionV1` | compile/schema拒绝；Harness只持Tool public窄Reader并只调用`InspectExactToolSurfaceManifestCurrentV1`，唯一Repo仍归Tool Owner |
| MPG-84 | call-order | A2/B1/Handoff/C2/Registry任一步并行、换序、漏读或S1/S2混用 | call-order oracle拒绝，零composite CAS |
| MPG-85 | integration | 真实多Owner fixture | Conformance Association exact Ref、六个分离BindingSet字段、sealed ProviderCandidate→Runtime BindingSet目标member full Ref映射、完整assembly lineage与实际Runtime Gateway package/type identity全部exact；association path Conformance Binding/Capability/Schema严格为空 |
| MPG-86 | whitebox | A2实现尝试调用不存在的Manifest/Graph/Generation Validate，或只校验已有Digest字段 | 编译/测试拒绝；必须分别重算`ManifestDigestV1/GraphDigestV1/GenerationDigestV1`并命中字段硬门，Handoff/Conformance才调用live Validate |
| MPG-87 | boundary | 外部包实现A2 Reader并返回自签OwnerCurrent | compile-time失败；unexported marker使Reader package-sealed，Publisher只接Harness concrete Store视图 |
| MPG-88 | contract | C2 lookup要求caller提供ProjectionDigest，或用ProjectionDigest代替Manifest digest | 接口/测试拒绝；lookup仅用Manifest Plan exact ID/Revision/Digest，ProjectionDigest只在返回后独立验证 |
| MPG-89 | lineage | A2把Catalog当成Generation映射字段或从Generation单边推导 | 拒绝；Generation链只映射ID/revision/digest/input/manifest/graph，Catalog必须由Manifest/Graph/Handoff三方exact闭合 |
| MPG-90 | contract | C2 create revision 1携带非零`ExpectedCurrent` | Invalid/Conflict且零history/current写；create只允许strict-zero `ExpectedCurrent` + authoritative current NotFound |
| MPG-91 | unit | C2 successor缺失/替换`ExpectedCurrent` full Ref，或Manifest revision不是current+1 | Conflict/PreconditionFailed且零写；CAS必须从当前full ID/Revision/Digest精确推进一个revision |
| MPG-92 | recovery | rev2已current后重投旧rev1，即使rev1 history winner仍存在 | Conflict/PreconditionFailed；不得幂等返回rev1、回退current index或ABA |
| MPG-93 | integration | Reader通过public conformance，但Association Ref、Conformance分离BindingSet字段、ProviderCandidate→Runtime member映射、Provider package/type identity或assembly lineage任一缺失/漂移 | composition在零M2 Reader调用前Fail Closed；不得以Fact shape、接口实现、conformance report或伪造`Conformance.Binding`补证 |
| MPG-94 | golden | A2 stable ID/CompileDigest/ProjectionDigest canonical literals | 具名`ModelPreDispatchVerifiedAssemblyOwnerCurrentIdentityCanonicalV1/CompileCanonicalV1/ProjectionCanonicalV1`分别使用固定`...-id/v1`/`...-compile/v1`/`...-projection/v1` domain、V1 contract version和exact discriminator `ModelPreDispatchVerifiedAssemblyOwnerCurrentIdentityV1`/`ModelPreDispatchVerifiedAssemblyOwnerCurrentCompileV1`/`ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1`；literal完整digest exact为`sha256:c05c7aab1acd177e819a951287120e5cbab0859d8c3eee9d8478cdab6a45f68c`/`sha256:a833f14f767cd6083cfde17198423cdf4cd0cfdb323a40fe95db69ed0465b455`/`sha256:93c8ffb4f5aeb21685f1a1eee9c32b156b4d28b1ddff9772e6869e468a8013e9`；expected不得由被测helper现算 |
| MPG-95 | lineage | A2 HandoffRef使用非`Generation.ID+"/handoff"`的ID、不同revision、Generation digest或caller digest | 在A2 history/current写入前Conflict；只允许`ID=GenerationRef.ID+"/handoff", Revision=GenerationRef.Revision, Digest=Handoff.Digest`并exact等于Conformance.HandoffRef |
| MPG-96 | contract | Inventory完整但actual-point carrier缺ACK full Ref、boundary kind、sequence、attempt ordinal或request digest | public合同无法表达并Fail Closed；禁止context value、裸string、latest ACK和全局表补齐 |
| MPG-97 | no-bypass | direct Preflight在Gate Commit前调用`Backend.Resolve` | rejected；Backend调用数0，不能在Resolve后补Inspect |
| MPG-98 | observation | ACK/current复读成功但没有Model Owner Receipt create-once sink/reader，或Harness生成sequence/ordinal | rejected；Receipt只由Model attempt Owner发布Observation且可exact复读 |
| MPG-99 | compatibility | 只落Inventory nominal/Reader便启用actual-point或production Conformance | rejected；Inventory、guard carrier、Receipt sink/reader与纯Preparation四门必须同时闭合 |

## 机械门

未来实现至少执行：

```text
targeted ordinary -count=100
targeted race -count=20
full go test ./...
full go test -race ./...
go vet ./...
gofmt -l
import boundary / zero-network / provider-call counter / git diff --check
```

fixture必须分别拥有Model Historical/Current Repository、Harness Assembly Current Owner、Tool Surface/Binding Repository；Harness只注入Model公开完整Reader nominal，不定义镜像。Context ExpectedInjection不参与本桥；Memory/Knowledge Sources固定0且Reader调用数0。不能由一个fake扮演多个Owner。Clock必须可推进、回拨和在调用边界跨TTL。
