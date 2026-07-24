# Context Offline SDK V1设计候选

时间：2026-07-16 21:04（Asia/Shanghai）

状态：`design_candidate / review_pending`。该SDK未获用户确认、未冻结、未实现；CTX-D09 A层Owner-local与本组件B层fixture已完成/二审YES，Application公共Port、G6B跨模块fixture与production C层继续NO-GO。

## 核对与短审结论

live `ExecutionRuntime/context-engine`没有`sdk`包。此前资产只有validate/compile/preview/diff/inspect能力名，缺少完整DTO、错误闭集、limits、codec、deep-copy与cancellation合同。独立短审判P1后，本轮只修订Context design/plan/memory候选，没有修改ExecutionRuntime代码。

## 候选合同

- 仅四个typed入口：`ValidateRecipeV1 / CompileFrameV1 / PreviewFrameV1 / InspectFrameExactV1`；每个都有独立完整Request/Response DTO及共同Request/Response Meta；
- typed Go入口不声称检测duplicate JSON key；候选另提供四个`Decode*RequestV1`与四个`Encode*ResponseV1` codec，Decode才递归拒绝unknown/trailing/duplicate key；
- hard maxima核心值：recipes=1、candidates=512、input items=1024、单item=4 MiB、input raw=24 MiB；Compile-derived generated<=52 MiB/output<=76 MiB；68 MiB/4 items与100 MiB/1028 items只是independent global guards；wire按operation/方向为Validate 48/48、Compile 48/144、Preview 144/48、Inspect 144/48 MiB；旧32/40 MiB及统一48 MiB request描述已废止；
- error code闭集：`invalid_argument / limit_exceeded / not_found / conflict / expired / unauthorized / unsupported / canceled / deadline_exceeded / internal_failure`；纯离线路径不返回unknown/unavailable；
- `OfflineContentBundleV1`只能经constructor建立，构造时deep-copy并校验Ref/Length/Digest/limits；所有getter与Response再次deep-copy，输入或返回slice/map/bytes均no-alias；
- cancel检查点覆盖validate前、candidate循环、content装载循环、sort/admission、budget、各render阶段、InspectFrame、seal/deep-copy及return前；cancel/deadline保真且返回零成功产物；
- Compile只使用单次调用内ephemeral workspace并返回`Authoritative=false` bundle，不写Owner Store/CAS、Generation current、DomainResult或Settlement；
- Preview不返回正文；Inspect只证明离线bundle内部exact，不证明Owner currentness。
- Offline SDK与G6B首切面均保持`MemorySources=0 / KnowledgeSources=0 / ContinuitySources=0`；Memory/Knowledge public contextsource Reader调用数为0；
- live Memory/Knowledge Owner已分别拥有`MemoryContextSourceCurrentReaderV1`、`KnowledgeContextSourceCurrentReaderV1`及Owner DTO。未来Context只能消费Owner唯一public Reader；V1无法无损承载新需求时，由对应Owner发布additive V2或唯一无损facade，Context不创建第二套平行nominal/DTO、不依赖Owner concrete Store/internal；
- `knowledge_reference`仍是Context Owner需单独确认的设计候选，未冻结；它不因Knowledge Reader V1已存在而自动成为Offline SDK/G6B的合法Source kind。

## 早期短审前置（已由21:49第四短审、22:06第五短审依次超越）

- 候选已补exact Go structs/tags、Operation四值闭集、presence/nil/empty规则和Request/Result/Report/Compile/Preview/Inspection digest等式，但仍待联合复审；
- 预算三分且按operation复核：24 MiB input的Compile派生上界为52 MiB generated/76 MiB output，68/100只是global guard；100 MiB raw的base64加4 MiB结构约137.333 MiB，只用于证明144 MiB global wire cap相容；
- Compile需call-scoped ephemeral workspace的`Begin/Seal/Export/Abort/Destroy`状态机；第五短审进一步冻结为构造后/Begin前先`defer Destroy()`、`Destroy(new)`合法，Begin成功后再`defer Abort()`。所有路径最终destroyed，partial Put永不可达，成功时只Export深拷贝sealed snapshot。旧Compile/Inspect wrapper不承诺cancel，streaming renderer必须golden逐字节等于旧renderer，禁止`goroutine+select`假取消；
- 64并发只验证small fixture；max-size只验证1/2/4/8并发并记录资源，不宣称SLA；
- public bundle改为unexported storage，constructor/getter/response全量deep-copy；所有嵌套slice/map/bytes进clone表与test；
- live `kernel.Compile/InspectFrame/ReferenceStore`无context，SDK代码NO-GO。必须先在Context Owner内抽取唯一context-aware staged kernel/store，旧API只作wrapper；SDK不得复制第二sort/admission/budget/render/Inspect算法；
- effective-required content missing只在SDK边界映射`not_found`+零Response；optional missing复用live `content_unavailable` Residual，不生成Fragment/不消耗token/不读Owner Store；共享wrapper不得改写Owner语义。

## 实施门禁

当前没有SDK实现或SDK专属测试。当前最新结论为第五短审`NO-GO，P0=2/P1=2`；须先获得独立复审YES，之后才可在Context独占目录实现；仍不得解锁Application Port、G6B跨模块fixture、production root、Capability、Harness Continuation或Turn推进。

diff、replay、transport-neutral API、CLI、refresh/parent-current快捷入口、真实Artifact Resolver与任何写命令均不在本候选V1。

## 既有模块回归记录

下列结果来自设计候选产生前的既有Context模块回归，只证明无代码回归，不代表SDK测试已执行：

- `go test -count=100 ./...`：PASS；
- `go test -race -count=20 ./...`：PASS；
- `go test -count=1 ./...`：PASS；
- `go test -race -count=1 ./...`：PASS；
- `go vet ./...`：PASS。
