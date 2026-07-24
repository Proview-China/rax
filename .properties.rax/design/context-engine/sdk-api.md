# Context Engineering SDK、CLI与API边界

状态：第七资产短审与后续独立软件验收均为YES；`ExecutionRuntime/context-engine/sdk`及唯一context-aware staged kernel已落地并通过target100、race20、full ordinary/race、vet与max-size验证。该YES只覆盖Owner-local Offline SDK，不授予production Capability。

## 1. Go SDK

V1 Owner-local design只规划一个Go包`context-engine/sdk`，合同名为`ContextOfflineSDKV1`。它是Context Owner-local library，不是Application/Harness公共Port，不注册Capability，不需要production composition root。当前有六个typed入口；`CompareRecipesV1`只作结构diff，`InspectCachePlanV1`只检查provider-neutral计划/currentness/经济性，两者均不作生产事实或外部动作：

```go
ValidateRecipeV1(ctx, ValidateRecipeRequestV1) (ValidateRecipeResponseV1, error)
CompareRecipesV1(ctx, CompareRecipesRequestV1) (CompareRecipesResponseV1, error)
CompileFrameV1(ctx, CompileFrameRequestV1) (CompileFrameResponseV1, error)
PreviewFrameV1(ctx, PreviewFrameRequestV1) (PreviewFrameResponseV1, error)
InspectFrameExactV1(ctx, InspectFrameExactRequestV1) (InspectFrameExactResponseV1, error)
InspectCachePlanV1(ctx, InspectCachePlanRequestV1) (InspectCachePlanResponseV1, error)
```

### 1.1 共同DTO与hard limits（第六审YES）

`OfflineRequestMetaV1`完整字段：`ContractVersion="praxis.context.offline-sdk/v1"`、`RequestID`、`Operation`、`Limits OfflineSDKLimitsV1`、`RequestDigest`。`RequestDigest` seal除自身外完整请求。`OfflineResponseMetaV1`完整字段：`ContractVersion`、`RequestID`、`Operation`、`RequestDigest`、`ResultDigest`。

`OfflineSDKLimitsV1`完整字段为`MaxRecipes`、`MaxCandidates`、`MaxInputContentItems`、`MaxInputContentItemBytes`、`MaxInputRawBytes`、`MaxGeneratedContentItems`、`MaxGeneratedRawBytes`、`MaxOutputContentItems`、`MaxOutputRawBytes`、`MaxTotalTokens`、`MaxDiagnostics`、`MaxDiagnosticMessageBytes`、`MaxNonContentWireBytes`、`MaxWireRequestBytes`、`MaxWireResponseBytes`。各字段必须大于0且不超过以下hard maxima；调用方可请求更小值，实际限制等于请求值，不能自动放宽：

| 限制 | hard maximum |
|---|---:|
| Recipes per request | 通常1；`compare_recipes`精确2 |
| Candidates per request | 512 |
| Input content items | 1,024 |
| Bytes per input content item | 4,194,304（4 MiB） |
| Aggregate input raw bytes | 25,165,824（24 MiB） |
| Generated content item reserve | 4（Stable/SemiStable/Dynamic/Rendered） |
| Aggregate generated raw bytes global guard | 71,303,168（68 MiB） |
| Output content items（input + generated） | 1,028 |
| Aggregate output raw bytes global guard | 104,857,600（100 MiB） |
| Total token estimate / Recipe budget | 1,048,576 |
| Diagnostics per response | 1,024 |
| Bytes per diagnostic message | 4,096 |
| Non-content canonical wire bytes | 4,194,304（4 MiB） |
| Wire request/response bytes | 按下表的Operation方向hard cap |

三类预算分开计算：①raw input只算解码后输入正文；②raw output算输入正文深拷贝加最多4个生成region/rendered item；③wire按实际canonical JSON字节精确计算。Wire content使用最大64 KiB的base64 string chunk，每chunk最多解码48 KiB；`ceil(raw/3)*4 + chunk delimiters + non-content canonical JSON`必须不超过对应wire上限。Input raw固定24 MiB；Validate/Compile request cap为48 MiB，Preview/Inspect request cap为144 MiB；旧40 MiB和统一48 MiB request描述全部废止。

live `renderRegions`先把`fragment.Content []byte`作为JSON bytes编码为base64，full Rendered再复用三个region JSON。第四短审冻结的Compile操作派生上界为：24 MiB input时`generated <= 52 MiB`、`input + generated <= 76 MiB`。68/100 MiB仅是完全独立的global guard，不是使用者可将派生值放宽至该值的Compile配额。任一中间值在分配前超出operation-derived cap、请求限额、global guard或checked arithmetic时立即`limit_exceeded`且零Response；不允许边生成边截断正文。不存在SemiStable时其指针为nil，但其预留item不得被其他未定义输出占用。

Wire cap按Operation和方向分开，请求内`Limits.MaxWireRequestBytes/MaxWireResponseBytes`不得超过该格：

| Operation | request hard cap | response hard cap | 原因 |
|---|---:|---:|---|
| `validate_recipe` | 48 MiB | 48 MiB | 不携带bundle bytes |
| `compare_recipes` | 48 MiB | 48 MiB | 仅携带两个Recipe与结构diff |
| `compile_frame` | 48 MiB | 144 MiB | request携24 MiB input；response携最多76 MiB Compile output |
| `preview_frame` | 144 MiB | 48 MiB | request嵌入完整CompiledBundle；response无正文 |
| `inspect_frame_exact` | 144 MiB | 48 MiB | request嵌入完整bundle；response无正文 |
| `inspect_cache_plan` | 48 MiB | 48 MiB | 仅携CachePlan/Profile与只读经济性投影 |

100 MiB global raw output如被其他未来operation使用，其base64约133.333 MiB，加4 MiB non-content约137.333 MiB，仍必须通过144 MiB实际wire计数；这个算术只证明global cap相容，不放宽Compile的52/76 MiB派生上界。

所有计数使用checked arithmetic；溢出等同`limit_exceeded`。Diagnostics规范排序；超过请求上限时保留前`max-1`项并以唯一`diagnostics_truncated`作为最后一项，不能静默截断。

`OfflineDiagnosticV1`字段：`Code`、`Severity=info|warning|error`、`ObjectKind`、`ObjectID`、`FieldPath`、`Message`。排序键固定为`Severity + Code + ObjectKind + ObjectID + FieldPath + Message`。

`OfflineContentItemV1`字段：`Ref ContentRef`、`Bytes []byte`。`OfflineContentBundleV1`字段/逻辑视图：规范排序的`Items`与`ContentSetDigest`。对外只能通过`NewOfflineContentBundleV1(items, limits)`构造；构造时逐项要求live `ContentRef.Validate()`通过、`Ref.Length>0`、`len(Bytes)>0`且Length/Digest exact，再deep-copy并拒绝重复Ref。全局ContentRef合同不修改，也不建立SDK shadow nominal。`Items()`、lookup及所有Response必须再次deep-copy，调用方修改输入或返回slice不得改变SDK内部、其他返回或Digest；SDK不保留跨调用alias或引用。

Offline SDK首切面的跨Owner Source基数固定为`MemorySources=0 / KnowledgeSources=0 / ContinuitySources=0`，不调用任何Memory/Knowledge/Continuity Reader。调用方提供的离线`ContextCandidate`/`OfflineContentBundleV1`只是本请求的非权威材料，不得解释为某个Owner的current projection。未单独获得Context Owner候选确认前，`knowledge_reference`不是已冻结的Context Source kind，不进入V1 Admission/Compile。

### 1.2 JSON codec（第六审YES）

六个typed Go入口本身**不声称**检测duplicate JSON key。只有显式codec入口提供该保证：

```go
DecodeValidateRecipeRequestV1(ctx, payload []byte) (ValidateRecipeRequestV1, error)
DecodeCompareRecipesRequestV1(ctx, payload []byte) (CompareRecipesRequestV1, error)
DecodeCompileFrameRequestV1(ctx, payload []byte) (CompileFrameRequestV1, error)
DecodePreviewFrameRequestV1(ctx, payload []byte) (PreviewFrameRequestV1, error)
DecodeInspectFrameExactRequestV1(ctx, payload []byte) (InspectFrameExactRequestV1, error)
DecodeInspectCachePlanRequestV1(ctx, payload []byte) (InspectCachePlanRequestV1, error)
EncodeValidateRecipeResponseV1(ctx, response ValidateRecipeResponseV1) ([]byte, error)
EncodeCompareRecipesResponseV1(ctx, response CompareRecipesResponseV1) ([]byte, error)
EncodeCompileFrameResponseV1(ctx, response CompileFrameResponseV1) ([]byte, error)
EncodePreviewFrameResponseV1(ctx, response PreviewFrameResponseV1) ([]byte, error)
EncodeInspectFrameExactResponseV1(ctx, response InspectFrameExactResponseV1) ([]byte, error)
EncodeInspectCachePlanResponseV1(ctx, response InspectCachePlanResponseV1) ([]byte, error)
```

Decode先检查operation/方向对应的48或144 MiB wire hard cap，再只流式定位并解码`meta`，在复制payload或反序列化DTO前执行request-specific `MaxWireRequestBytes`。随后context-aware递归strict token scan先累计array/base64 chunk数量、单item raw、aggregate raw与non-content wire，任何超限在`json.Decoder.Decode`物化`[]string`前拒绝；scanner同时拒绝trailing document和任意嵌套duplicate key，最终strict Decode再拒绝unknown field。Wire bundle的private DTO使用`base64_chunks`，单个string token最大64 KiB，scanner至少每64 KiB和每token检查`ctx.Err()`；通过有界扫描后才逐chunk解码、归并并经`NewOfflineContentBundleV1`构造，禁止反序列化绕过deep-copy/limits。Encode只接受已Validate的Response，使用context-aware canonical writer按operation/方向cap输出JSON并复验ResultDigest；codec同样deep-copy输入payload且不保留alias。

strict scanner与base64 chunk decode是唯一wire管线；不允许再对整个48/144 MiB payload做一次无context的`json.Unmarshal`。最大不可中断单元是一个64 KiB wire token或48 KiB raw chunk；这是work bound，不宣称wall-clock SLA。

base64 primitive只有一个规范编码：Go `encoding/base64.StdEncoding`、标准`+/`字母表、必要时`=`padding、无空白/换行。纯primitive允许并测试`encode([]byte{})=[]`与`decode([]string{})=[]byte{}`，但该结果没有ContentRef、不能进入`offlineContentItemWireV1`或`OfflineContentBundleV1`。ContentItem正长度时每个非最后chunk必须恰好解码49,152 bytes（48 KiB）且wire string恰好65,536 bytes（64 KiB），最后chunk解码1..49,152 bytes。禁止empty string chunk、URL alphabet、raw/no-padding base64、非最后短chunk、可用更少chunk表达的冗余分块、解码后Length/Digest不exact。Items先按ContentRef规范排序，chunks保持原始byte顺序；Encode必须产生唯一形式，Decode必须拒绝其他等价表示。

base64 codec必须有独立于renderer golden的规范矩阵。primitive正例固定使用确定性bytes序列`byte(i % 251)`，覆盖raw长度`0/1/2`、`48 KiB-1/exact/+1`、`96 KiB`双chunk；其中0字节只断言primitive `[]↔[]`，不得构造或断言ContentRef/ContentItem。1字节`AA==`、2字节`AAE=`，48 KiB+1与96 KiB分别形成exact的2 chunks，并逐字节断言round-trip、chunk数/长度、padding与正长度ContentRef。标准alphabet另用`[]byte{0xfb,0xff}`固定`+/8=`，拒绝URL形式`-_8=`；1/2字节分别拒绝raw/no-padding的`AA`/`AAE`。其余反例拒绝whitespace、empty string chunk、短non-final、redundant chunk、错误padding及其他非规范表示。额外硬反例：以零长度ContentRef或空bytes构造OfflineContentItem/Bundle必须`invalid_argument`且零Bundle；全部失败返回零产物。

### 1.3 六入口exact Go DTO（四个既有入口保持第六审YES规范；Compare/Cache Inspect为additive只读Delta）

以下struct、tag、指针与slice presence是第六审通过的唯一Owner-local设计，不再以prose推断字段。其中`contract.*`指live Context Owner值对象；不引入跨Owner DTO。

```go
type OfflineSDKOperationV1 string

const (
    OfflineValidateRecipeV1    OfflineSDKOperationV1 = "validate_recipe"
    OfflineCompareRecipesV1    OfflineSDKOperationV1 = "compare_recipes"
    OfflineCompileFrameV1      OfflineSDKOperationV1 = "compile_frame"
    OfflinePreviewFrameV1      OfflineSDKOperationV1 = "preview_frame"
    OfflineInspectFrameExactV1 OfflineSDKOperationV1 = "inspect_frame_exact"
    OfflineInspectCachePlanV1  OfflineSDKOperationV1 = "inspect_cache_plan"
)

type CompareRecipesRequestV1 struct {
    Meta              OfflineRequestMetaV1   `json:"meta"`
    BaseRecipe        contract.ContextRecipe `json:"base_recipe"`
    CandidateRecipe   contract.ContextRecipe `json:"candidate_recipe"`
    CheckedUnixNano   int64                  `json:"checked_unix_nano"`
    ExpiresUnixNano   int64                  `json:"expires_unix_nano"`
}

type CompareRecipesResponseV1 struct {
    Meta        OfflineResponseMetaV1              `json:"meta"`
    Comparison contract.ContextRecipeComparisonV1 `json:"comparison"`
    Diagnostics []OfflineDiagnosticV1              `json:"diagnostics"`
}

type InspectCachePlanRequestV1 struct {
    Meta                 OfflineRequestMetaV1          `json:"meta"`
    CachePlan            contract.CachePlan            `json:"cache_plan"`
    ProviderCacheProfile contract.ProviderCacheProfile `json:"provider_cache_profile"`
    CheckedUnixNano      int64                         `json:"checked_unix_nano"`
}

type InspectCachePlanResponseV1 struct {
    Meta               OfflineResponseMetaV1          `json:"meta"`
    Current            bool                           `json:"current"`
    PlanRef            contract.FactRef               `json:"plan_ref"`
    PartitionDigest    contract.Digest                `json:"partition_digest"`
    KeyDigest          contract.Digest                `json:"key_digest"`
    ProviderProfileRef contract.FactRef               `json:"provider_profile_ref"`
    EconomicDecision   contract.CacheEconomicDecision `json:"economic_decision"`
    CheckedUnixNano    int64                          `json:"checked_unix_nano"`
    ExpiresUnixNano    int64                          `json:"expires_unix_nano"`
    Diagnostics        []OfflineDiagnosticV1          `json:"diagnostics"`
    InspectionDigest   contract.Digest                `json:"inspection_digest"`
}

type OfflineSDKLimitsV1 struct {
    MaxRecipes                  uint32 `json:"max_recipes"`
    MaxCandidates               uint32 `json:"max_candidates"`
    MaxInputContentItems        uint32 `json:"max_input_content_items"`
    MaxInputContentItemBytes    uint64 `json:"max_input_content_item_bytes"`
    MaxInputRawBytes            uint64 `json:"max_input_raw_bytes"`
    MaxGeneratedContentItems    uint32 `json:"max_generated_content_items"`
    MaxGeneratedRawBytes        uint64 `json:"max_generated_raw_bytes"`
    MaxOutputContentItems       uint32 `json:"max_output_content_items"`
    MaxOutputRawBytes           uint64 `json:"max_output_raw_bytes"`
    MaxTotalTokens              uint64 `json:"max_total_tokens"`
    MaxDiagnostics              uint32 `json:"max_diagnostics"`
    MaxDiagnosticMessageBytes   uint32 `json:"max_diagnostic_message_bytes"`
    MaxNonContentWireBytes      uint64 `json:"max_non_content_wire_bytes"`
    MaxWireRequestBytes         uint64 `json:"max_wire_request_bytes"`
    MaxWireResponseBytes        uint64 `json:"max_wire_response_bytes"`
}

type OfflineRequestMetaV1 struct {
    ContractVersion string                `json:"contract_version"`
    RequestID       string                `json:"request_id"`
    Operation       OfflineSDKOperationV1 `json:"operation"`
    Limits          OfflineSDKLimitsV1    `json:"limits"`
    RequestDigest   contract.Digest       `json:"request_digest"`
}

type OfflineResponseMetaV1 struct {
    ContractVersion string                `json:"contract_version"`
    RequestID       string                `json:"request_id"`
    Operation       OfflineSDKOperationV1 `json:"operation"`
    RequestDigest   contract.Digest       `json:"request_digest"`
    ResultDigest    contract.Digest       `json:"result_digest"`
}

type OfflineDiagnosticV1 struct {
    Code       string `json:"code"`
    Severity   string `json:"severity"`
    ObjectKind string `json:"object_kind"`
    ObjectID   string `json:"object_id"`
    FieldPath  string `json:"field_path"`
    Message    string `json:"message"`
}

type OfflineContentItemV1 struct {
    Ref   contract.ContentRef `json:"ref"`
    Bytes []byte              `json:"-"`
}

// No exported fields and no direct json.Marshal/json.Unmarshal path.
type OfflineContentBundleV1 struct {
    items            []OfflineContentItemV1
    contentSetDigest contract.Digest
}

// Private codec projection; each base64 string is <=64 KiB wire and
// decodes to <=48 KiB raw. It is the only JSON representation of a bundle.
type offlineContentItemWireV1 struct {
    Ref          contract.ContentRef `json:"ref"`
    Base64Chunks []string            `json:"base64_chunks"`
}

type offlineContentBundleWireV1 struct {
    Items            []offlineContentItemWireV1 `json:"items"`
    ContentSetDigest contract.Digest            `json:"content_set_digest"`
}

type ValidateRecipeRequestV1 struct {
    Meta       OfflineRequestMetaV1       `json:"meta"`
    Recipe     contract.ContextRecipe     `json:"recipe"`
    Candidates []contract.ContextCandidate `json:"candidates"`
}

type ValidateRecipeResponseV1 struct {
    Meta          OfflineResponseMetaV1 `json:"meta"`
    Valid         bool                  `json:"valid"`
    RecipeRef     *contract.FactRef     `json:"recipe_ref,omitempty"`
    CandidateRefs []contract.FactRef    `json:"candidate_refs"`
    Diagnostics   []OfflineDiagnosticV1 `json:"diagnostics"`
    ReportDigest  contract.Digest       `json:"report_digest"`
}

type CompileFrameRequestV1 struct {
    Meta              OfflineRequestMetaV1        `json:"meta"`
    AttemptID         string                      `json:"attempt_id"`
    ManifestID        string                      `json:"manifest_id"`
    FrameID           string                      `json:"frame_id"`
    GenerationID      string                      `json:"generation_id"`
    GenerationOrdinal uint64                      `json:"generation_ordinal"`
    Recipe            contract.ContextRecipe      `json:"recipe"`
    Execution         contract.ExecutionBinding   `json:"execution"`
    Candidates        []contract.ContextCandidate `json:"candidates"`
    ParentFrame       *contract.FactRef            `json:"parent_frame,omitempty"`
    CreatedUnixNano   int64                       `json:"created_unix_nano"`
    ExpiresUnixNano   int64                       `json:"expires_unix_nano"`
    InputBundle       OfflineContentBundleV1      `json:"input_bundle"`
}

type CompiledBundleV1 struct {
    Manifest             contract.ContextManifest `json:"manifest"`
    Frame                contract.ContextFrame    `json:"frame"`
    ContentBundle        OfflineContentBundleV1   `json:"content_bundle"`
    ResidualCandidateRefs []contract.FactRef       `json:"residual_candidate_refs"`
    Authoritative        bool                     `json:"authoritative"`
    CompileDigest        contract.Digest          `json:"compile_digest"`
}

type CompileFrameResponseV1 struct {
    Meta        OfflineResponseMetaV1 `json:"meta"`
    Compiled    CompiledBundleV1      `json:"compiled"`
    Diagnostics []OfflineDiagnosticV1 `json:"diagnostics"`
}

type PreviewFrameRequestV1 struct {
    Meta                  OfflineRequestMetaV1 `json:"meta"`
    Compiled              CompiledBundleV1      `json:"compiled"`
    ExpectedCompileDigest contract.Digest       `json:"expected_compile_digest"`
    CheckedUnixNano       int64                 `json:"checked_unix_nano"`
}

type FragmentPreviewV1 struct {
    Position     uint32                `json:"position"`
    CandidateRef contract.FactRef      `json:"candidate_ref"`
    Kind         contract.FragmentKind `json:"kind"`
    Region       contract.FrameRegion  `json:"region"`
    ContentRef   contract.ContentRef   `json:"content_ref"`
    Tokens       uint64                `json:"tokens"`
}

type PreviewFrameResponseV1 struct {
    Meta               OfflineResponseMetaV1       `json:"meta"`
    AdmissionDecisions []contract.AdmissionDecision `json:"admission_decisions"`
    Fragments          []FragmentPreviewV1          `json:"fragments"`
    StableTokens       uint64                       `json:"stable_tokens"`
    SemiStableTokens   uint64                       `json:"semi_stable_tokens"`
    DynamicTokens      uint64                       `json:"dynamic_tokens"`
    TotalTokens        uint64                       `json:"total_tokens"`
    StablePrefixRef    contract.ContentRef          `json:"stable_prefix_ref"`
    SemiStableRef      *contract.ContentRef         `json:"semi_stable_ref,omitempty"`
    DynamicTailRef     contract.ContentRef          `json:"dynamic_tail_ref"`
    RenderedRef        contract.ContentRef          `json:"rendered_ref"`
    SourceSetDigest    contract.Digest              `json:"source_set_digest"`
    RecipeRef          contract.FactRef             `json:"recipe_ref"`
    ManifestRef        contract.FactRef             `json:"manifest_ref"`
    FrameRef           contract.FactRef             `json:"frame_ref"`
    ExpiresUnixNano    int64                        `json:"expires_unix_nano"`
    Diagnostics        []OfflineDiagnosticV1        `json:"diagnostics"`
    PreviewDigest      contract.Digest              `json:"preview_digest"`
}

type InspectFrameExactRequestV1 struct {
    Meta                  OfflineRequestMetaV1     `json:"meta"`
    Manifest              contract.ContextManifest `json:"manifest"`
    Frame                 contract.ContextFrame    `json:"frame"`
    ContentBundle         OfflineContentBundleV1   `json:"content_bundle"`
    ExpectedManifestRef   contract.FactRef          `json:"expected_manifest_ref"`
    ExpectedFrameRef      contract.FactRef          `json:"expected_frame_ref"`
    ExpectedCompileDigest contract.Digest           `json:"expected_compile_digest"`
    CheckedUnixNano       int64                     `json:"checked_unix_nano"`
}

type InspectFrameExactResponseV1 struct {
    Meta             OfflineResponseMetaV1 `json:"meta"`
    Exact            bool                  `json:"exact"`
    ManifestRef      contract.FactRef      `json:"manifest_ref"`
    FrameRef         contract.FactRef      `json:"frame_ref"`
    ContentSetDigest contract.Digest       `json:"content_set_digest"`
    CheckedUnixNano  int64                 `json:"checked_unix_nano"`
    ExpiresUnixNano  int64                 `json:"expires_unix_nano"`
    Diagnostics      []OfflineDiagnosticV1 `json:"diagnostics"`
    InspectionDigest contract.Digest       `json:"inspection_digest"`
}
```

`CompareRecipesV1`要求`Limits.MaxRecipes=2`，两个Recipe及其`Rules`都必须present/non-null并通过live `ContextRecipe.Validate`。`CheckedUnixNano`必须同时落在两个Recipe有效期内，`ExpiresUnixNano`不得超过任一Recipe真实expiry；不得伪造或延长比较报告TTL。输出`Changes`始终present，完全相同编码`[]`；最多80项，按`FieldPath`唯一升序。每项只暴露`added|removed|modified|reordered`与before/after digest，不回传字段正文。Request/Comparison/Result三个digest分别seal自身排除关系，任一漂移零Response。

`InspectCachePlanV1`只接受调用方构造的provider-neutral `CachePlan`与exact `ProviderCacheProfile`，执行`Plan.ValidateCurrent(profile, checked)`、Partition/Key digest及`CompareCacheEconomics`。返回的`Current=true`只表示该离线输入闭包在Checked时自洽；不证明Provider cache存在或命中。Expires取Plan/Profile严格最小值，Provider Profile过期为typed `expired`。输出不含CacheEntry、CacheAccessFact或usage→hit推断，不调用Provider、不预热、不写缓存。

Operation是上述六值闭集，并必须与入口一一对应；任一其他值为`unsupported`。除`RecipeRef`、`ParentFrame`和`SemiStableRef`外没有optional field；只有这三个指针nil时用`omitempty`编码为缺席。所有其他键（包括`valid=false`、`authoritative=false`、`exact=false`）必须出现。所有slice必须非nil；零项规范编码为`[]`，`null`、键缺席或typed请求中的nil slice均为`invalid_argument`。Strict codec使用每个struct的required-key bitmap检查presence，不用Go零值猜测是否缺键。

递归presence表固定如下；`required`表示键必须出现，不代表零值必然合法：

| Object | required keys | optional/conditional keys |
|---|---|---|
| request meta | `contract_version,request_id,operation,limits,request_digest` | 无 |
| response meta | `contract_version,request_id,operation,request_digest,result_digest` | 无 |
| limits | 上述15个`max_*`键全部 | 无 |
| Validate request | `meta,recipe,candidates` | 无；`candidates=[]`允许 |
| Validate response | `meta,valid,candidate_refs,diagnostics,report_digest` | `recipe_ref` |
| Compare request | `meta,base_recipe,candidate_recipe,checked_unix_nano,expires_unix_nano` | 无；两个Recipe都不得null |
| Compare response | `meta,comparison,diagnostics` | 无；`comparison.changes=[]`允许但不得null |
| Cache Inspect request | `meta,cache_plan,provider_cache_profile,checked_unix_nano` | 无；Plan/Profile都不得null |
| Cache Inspect response | `meta,current,plan_ref,partition_digest,key_digest,provider_profile_ref,economic_decision,checked_unix_nano,expires_unix_nano,diagnostics,inspection_digest` | 无；`current`必须true但不等于hit |
| Compile request | `meta,attempt_id,manifest_id,frame_id,generation_id,generation_ordinal,recipe,execution,candidates,created_unix_nano,expires_unix_nano,input_bundle` | `parent_frame` |
| Compiled bundle | `manifest,frame,content_bundle,residual_candidate_refs,authoritative,compile_digest` | 无；`authoritative` must be present and false |
| Preview request | `meta,compiled,expected_compile_digest,checked_unix_nano` | 无 |
| Preview response | 除`semi_stable_ref`外上述struct所有tag | `semi_stable_ref` |
| Inspect request | `meta,manifest,frame,content_bundle,expected_manifest_ref,expected_frame_ref,expected_compile_digest,checked_unix_nano` | 无 |
| Inspect response | `meta,exact,manifest_ref,frame_ref,content_set_digest,checked_unix_nano,expires_unix_nano,diagnostics,inspection_digest` | 无 |
| private bundle wire | `items,content_set_digest` | 无；`items=[]`仅在operation语义允许时可用 |
| private content item wire | `ref,base64_chunks` | 无；Ref必须通过live Validate且`Length>0`，chunks必须non-nil且非空；primitive零字节无wire item表示 |

live nested Context Owner类型不复制为SDK shadow DTO。Codec递归使用live concrete type的公开JSON tags与Validate：`FactRef/ContentRef/ExecutionBinding/ContextRecipe/ContextCandidate/ContextFragment`的所有非`omitempty`键必须出现；`ContextManifest.ParentFrame`、`ContextFrame.ParentFrame/SemiStable`是指针optional；`AdmissionDecision.Region`仅在`admitted`时必须出现，其他disposition时必须缺席。任一live Owner tag/schema变化必须使codec schema-conformance测试失败并重新评审，禁止SDK保留一份不会自动漂移报错的Owner DTO副本。

`OfflineContentBundleV1`仅能由`NewOfflineContentBundleV1`构造，对外只提供`Items() []OfflineContentItemV1`、`Lookup(ref) ([]byte, bool)`与`ContentSetDigest() contract.Digest`；三者均返回clone。需全量clone的嵌套字段为：所有bundle item bytes、Candidates、Recipe.Rules、Manifest.Decisions/Fragments、Generation retained/open refs（如后续纳入）、ResidualCandidateRefs、AdmissionDecisions、Fragments、CandidateRefs、Diagnostics；任一未来slice/map/bytes字段加入DTO必须同时加入clone表与test。Wire private item为`{ref, base64_chunks}`，不是第二套公共DTO；仅codec内部可见。

Pointer/slice clone表是闭集：

| 对象 | 必须clone |
|---|---|
| Validate request/response | `Recipe.Rules`、`Candidates`及每个candidate内未来可变字段、`RecipeRef` pointee、`CandidateRefs`、`Diagnostics` |
| Compare request/response | 两个`Recipe.Rules`、`Comparison.Changes`、每项`BeforeDigest/AfterDigest` pointee、`Diagnostics` |
| Cache Inspect request/response | DTO无pointer/slice/map/bytes；仅`Diagnostics`必须clone，输入按值复制 |
| Compile request | `Recipe.Rules`、`Candidates`、`ParentFrame` pointee、`InputBundle.items`及每个`Bytes` |
| Compiled bundle | `Manifest.ParentFrame` pointee、`Manifest.Decisions/Fragments`、`Frame.ParentFrame/SemiStable` pointees、`ContentBundle.items/Bytes`、`ResidualCandidateRefs` |
| Preview request/response | 完整Compiled bundle clone、`AdmissionDecisions`、`Fragments`、`SemiStableRef` pointee、`Diagnostics` |
| Inspect request/response | `Manifest.ParentFrame`、`Manifest.Decisions/Fragments`、`Frame.ParentFrame/SemiStable`、`ContentBundle.items/Bytes`、`Diagnostics` |
| private wire DTO | `Items`、每个`Base64Chunks`及string backing bytes在decode后不保留payload alias |

当前DTO无map字段；任一新增pointer/slice/map/bytes字段在clone表和no-alias测试落地前不得合并。

#### 1.3.1 Canonical与digest等式

所有SDK digest使用同一`CanonicalDigest(domain, version, discriminator, privateBody)`，其常量固定为：

```text
domain  = "praxis.context.offline-sdk"
version = "v1"
request discriminators =
  "validate-recipe-request" | "compare-recipes-request" | "compile-frame-request" |
  "preview-frame-request" | "inspect-frame-exact-request" |
  "inspect-cache-plan-request"
response discriminators =
  "validate-recipe-response" | "compare-recipes-response" | "compile-frame-response" |
  "preview-frame-response" | "inspect-frame-exact-response" |
  "inspect-cache-plan-response"
subordinate discriminators =
  "content-set" | "validate-report" | "compiled-bundle" |
  "frame-preview" | "frame-inspection" | "cache-plan-inspection"
```

`privateBody`是SDK内部规范投影，不是新公共DTO。它与上述exact Go DTO字段一一对应，但用`offlineBundleClosureV1{SortedContentRefs []ContentRef, ContentSetDigest}`替代任何`OfflineContentBundleV1`；ContentRefs按`Ref + Digest + Length`规范排序，constructor已先校验每个Ref的Length/Digest等于clone bytes。禁止对含未导出bundle字段的public request/response直接`DigestJSON/json.Marshal`，也禁止把wire chunk分割方式变成domain digest语义。

- `RequestDigest = CanonicalDigest(domain, version, request discriminator, private request body with RequestDigest omitted)`；private body包含Meta其余字段与该operation全部输入；
- `ContentSetDigest = CanonicalDigest(domain, version, "content-set", {sorted_content_refs})`；
- `ReportDigest = CanonicalDigest(domain, version, "validate-report", {valid, recipe_ref, candidate_refs, diagnostics})`，不包含Meta和自身；
- `ComparisonDigest`复用Context `ContextRecipeComparisonV1.DigestValue`，清空自身后seal两个exact Recipe refs、规范changes与Checked/Expires；
- `CompileDigest = CanonicalDigest(domain, version, "compiled-bundle", {manifest, frame, bundle_closure, residual_candidate_refs, authoritative:false})`，不包含Response Meta、Diagnostics和自身；
- `PreviewDigest = CanonicalDigest(domain, version, "frame-preview", {admission_decisions ... diagnostics})`，不包含Meta和自身；
- `InspectionDigest = CanonicalDigest(domain, version, "frame-inspection", {exact ... diagnostics})`，不包含Meta和自身；
- Cache `InspectionDigest = CanonicalDigest(domain, version, "cache-plan-inspection", {current, exact refs/digests, economics, checked/expires, diagnostics})`，不包含Meta和自身；
- `ResultDigest = CanonicalDigest(domain, version, response discriminator, private full response body with ResultDigest omitted)`；此时下层Report/Comparison/Compile/Preview/Inspection digest已填充，不得一并清空。Response Meta的RequestDigest必须exact等于已验证请求。

所有规范排序在digest前完成；任一own-digest未置空、下层digest缺失、nil/empty不规范或等式不成立均为`conflict`，零Response。

#### 1.3.2 Missing分支

`selected_required_rule`必须与live唯一compiler算法一致，步骤固定为：①先用live `StableSortCandidates(recipe)`对所有candidate排序；②按排序结果遍历，对每个`rule.Required=true`的kind，第一个命中该rule的candidate成为唯一`selected_required_rule=true`，即使它稍后因missing/expiry/authority失败也不得改选第二个；③`effective_required = candidate.Required || selected_required_rule`；④遍历结束仍没有候选命中的required rule为`not_found`。SDK不得自己实现另一个“挑第一个可用候选”算法。

错误分类固定：`effective_required=true`的合法非零ContentRef缺失或required rule无candidate在**Offline SDK边界**映射为`not_found`；required candidate expired→`expired`；required candidate execution/authority/trust不匹配→`unauthorized`；required candidate无recipe rule、required budget不足或digest/binding冲突→`conflict`；零长度Ref、空ContentItem bytes及结构/tag/presence错误→`invalid_argument`。`effective_required=false`的合法Ref缺失必须复用live Owner语义，产生唯一`AdmissionResidual(reason="content_unavailable")`。Required空内容若没有合法非零ContentRef必须Fail Closed，不能借primitive空编码或空item绕过。共享staged helper与旧compatibility wrapper必须原样保留Owner错误/Residual语义；`not_found`只允许由SDK边界对合法Ref的确定性required-missing作typed映射。

### 1.4 typed error闭集

所有失败使用`OfflineSDKErrorV1{Code, Operation, FieldPath, Message}`；Code闭集为：

```text
invalid_argument
limit_exceeded
not_found
conflict
expired
unauthorized
unsupported
canceled
deadline_exceeded
internal_failure
```

纯离线SDK不返回`unknown`或`unavailable`。除`ValidateRecipeV1`可诊断的`Valid=false`报告外，任何error必须返回对应Response零值，不能同时返回partial/stale成功产物。内部错误不得包含Secret或正文。

确定性映射：Envelope/codec/contract `ErrInvalid`→`invalid_argument`；任一hard/request limit或算术溢出→`limit_exceeded`；effective-required bundle缺项由SDK封闭bundle预检或SDK边界映射为`not_found`；digest/length/duplicate/binding drift→`conflict`；时间窗口→`expired`；Authority/Trust拒绝→`unauthorized`；未纳入V1的mode/operation→`unsupported`；context两类错误映射同名code并保留`errors.Is`；闭集之外的不可预期内部错误→`internal_failure`。当前内核/Owner helper的错误与`content_unavailable` Residual在共享路径中必须保持原样；SDK不得为获得`not_found`而修改共享wrapper，只能在SDK边界基于exact required ContentRef作确定性分类。

`knowledge_reference`在未经独立候选确认时按未纳入V1的Source kind处理：返回`unsupported`且零Response，不得隐式降级为普通ContentRef。

### 1.5 cancel、deep-copy与阶段边界

当前live `kernel.Compile`、`kernel.InspectFrame`与`kernel.ReferenceStore`均不接受`context.Context`，因此不能直接支撑上述取消承诺。SDK实现前必须先在Context Owner内联合评审并落地唯一共享的context-aware staged kernel/store：

```go
type CompileWorkLimitsV1 struct {
    MaxCandidates            uint32
    MaxInputContentItems     uint32
    MaxInputContentItemBytes uint64
    MaxInputRawBytes         uint64 // <=24 MiB
    MaxGeneratedContentItems uint32 // <=4
    MaxGeneratedRawBytes     uint64 // Compile-derived <=52 MiB; global <=68 MiB
    MaxOutputContentItems    uint32 // <=1028
    MaxOutputRawBytes        uint64 // Compile-derived <=76 MiB; global <=100 MiB
    MaxTotalTokens           uint64
    StreamChunkBytes         uint32 // exactly 64 KiB in V1
    CloneChunkBytes          uint32 // exactly 64 KiB in V1
}

type InspectWorkLimitsV1 struct {
    MaxFragments        uint32 // <=512
    MaxContentItems     uint32 // <=1028
    MaxContentItemBytes uint64 // <=operation MaxOutputRawBytes
    MaxRawBytes         uint64 // <=76 MiB for V1 compiled bundle
    StreamChunkBytes    uint32 // exactly 64 KiB in V1
    CloneChunkBytes     uint32 // exactly 64 KiB in V1
}

type ContextAwareReferenceStoreV1 interface {
    GetContextV1(context.Context, contract.ContentRef) ([]byte, error)
    PutContextV1(context.Context, []byte) (contract.ContentRef, error)
}

type offlineWorkspaceV1 interface {
    ContextAwareReferenceStoreV1
    Begin(context.Context, CompileWorkLimitsV1) error
    Seal(context.Context) (offlineWorkspaceSealV1, error)
    Export(context.Context, offlineWorkspaceSealV1) (OfflineContentBundleV1, error)
    Abort() error
    Destroy() error
}

func CompileStagedV1(
    context.Context,
    ContextAwareReferenceStoreV1,
    CompileRequest,
    CompileWorkLimitsV1,
) (CompileResult, error)

func InspectFrameStagedV1(
    context.Context,
    ContextAwareReferenceStoreV1,
    contract.ContextManifest,
    contract.ContextFrame,
    InspectWorkLimitsV1,
) error
```

`CompileStagedV1`必须抽取并复用当前Compile的唯一sort/admission/budget/render/Inspect helper；旧`Compile/InspectFrame`只能成为该共享实现的compatibility wrapper，SDK不得复制第二套算法。`ContextAwareReferenceStoreV1`为Context Owner内部编译缝隙，不是新跨组件public Port；Offline SDK的ephemeral store实现它，不暴露写口。

每次Compile必须先构造call-scoped `offlineWorkspaceV1`，**构造返回后、调用`Begin`前**立即注册不使用request ctx的`defer Destroy()`；`Begin`成功后再注册`defer Abort()`。状态机唯一为`new --Begin(success)--> open --Seal--> sealed --Export--> exported --Destroy--> destroyed`、`new --Begin(error)--> new --Destroy--> destroyed`或`open|sealed --Abort--> aborted --Destroy--> destroyed`。`Destroy()`在`new`合法且幂等，必须释放constructor或失败Begin留下的全部私有资源；对open/sealed先执行等价Abort再销毁。`Abort()`无context参数、幂等，在open/sealed清理全部staging bytes/ref/index，在aborted/exported/destroyed重复调用无害。因此Begin失败、cancel/deadline或成功Export的所有路径最终都到destroyed，且已cancel ctx不能阻止清理。

所有`PutContextV1`只写workspace私有staging namespace，任何部分Put、cancel、deadline、limit或Inspect失败后都不能通过bundle getter/Ref访问。`Seal`只冻结staged index并返回一次性seal token，不导出bytes；`Export`只接受当前workspace的exact seal token，在全部Compile+Inspect+canonical digest+limits通过后深拷贝一个response bundle snapshot并进入exported。该Seal/Export不是Owner Store commit或production CAS。

旧`Compile/InspectFrame`作为compatibility wrapper不承诺cancel；只有新staged API可做SDK的取消合同。禁止用`goroutine + select`在外层提前返回来冒充取消，因为后台编译/写入仍会继续。Streaming renderer必须对所有确定性fixture产生与live旧`renderRegions`逐字节相同的Stable/SemiStable/Dynamic/Rendered golden及ContentRef/Digest；不得借streaming改变JSON、base64、顺序或换行。

Streaming renderer golden矩阵至少覆盖：三region分别空/单fragment/多fragment，SemiStable缺席，三region混合，ASCII/UTF-8/binary bytes，48 KiB raw和64 KiB rendered边界的`-1/exact/+1`，4 MiB单item，candidate乱序/重复内容与24 MiB aggregate。每个用例比较Stable/SemiStable/Dynamic/Rendered bytes、Length、ContentRef/Digest、Manifest/Frame digest；任一字节差异都是P0。该矩阵不能替代前述独立base64 codec golden。

唯一staged实现的取消检查点是闭集：Decode的payload copy前/每token/每base64 chunk/构造bundle前；SDK入口与每个validate前；workspace Begin前后；每个candidate及每个content item/chunk；stable sort前后；admission每迭代；budget前后；Stable/SemiStable/Dynamic/full render每64 KiB输出；每次Get/Put的每64 KiB clone；Inspect每个ref/content item/chunk；canonical body每64 KiB；Seal前后；Export前后；Response clone/digest前后；最终return立即前。不可中断工作上界固定为一个64 KiB wire/render/canonical chunk、一个48 KiB raw decode chunk或一次最多512 candidates的live `sort.SliceStable`调用；**不声明comparison公式上界**。实现评审必须用live comparator对已排序、逆序、全相等、大量重复、交错key和确定性seed乱序fixture实测comparisons与耗时，并以同Go版本实测最大值建立保守防回归阈值；阈值与证据入测试资产前不得承诺取消延迟或SLA。任一实现若退化为整个48/144 MiB payload、76 MiB Compile output或全量render的单次无context操作，Conformance直接Fail。

`context.Canceled`映射`canceled`，`context.DeadlineExceeded`映射`deadline_exceeded`并保留`errors.Is`。取消发生在任一检查点时，销毁ephemeral workspace并返回零Response；已经计算出的Manifest/Frame/bundle/diagnostics不得逃逸。所有请求的slice/map/bytes在进入异步或循环阶段前deep-copy；所有Response在seal后再deep-copy交付，保证no-alias。

取消fault矩阵对constructor后/Begin前取消、Begin各分配点错误、以及后续每一类检查点注入cancel/deadline和Put/Get/Seal/Export错误，断言`Destroy(new)`被调用、零Response、workspace最终destroyed、partial Ref/bytes不可达、无后台goroutine。Benchmark只分两组：small fixture可跑64并发/race20；max-size 24 MiB input/52 MiB generated/76 MiB output/wire边界只跑1/2/4/8并发，记录`ns/op, B/op, allocs/op, peak heap/RSS, cancel-to-return`；stable sort另外记录上述对抗fixture的comparisons与耗时，只作短审证据，不宣称production SLA。

### 1.6 Owner与范围边界

- V1 Owner-local核心不包含diff、replay、parent-current、refresh、artifact resolver、Cache Store/Provider操作、evaluation或写命令；已实现的`InspectCachePlanV1`仅检查调用者给定的provider-neutral Plan/Profile闭包，不创建Plan、Entry、Access Fact或Effect；
- 不暴露`kernel.ReferenceStore`、`refstore.Memory`或其他可写Store句柄；ephemeral workspace/Fake不宣称Backend/State Plane/SLA；
- context-aware staged kernel/store已按单独授权落地为实现候选；只允许在Context Owner内保持唯一算法路径，不更改Owner语义、不新建跨模块Port；
- 不导入Runtime/Application/Harness/Tool/Memory/Knowledge/Continuity/Model Invoker实现或私有Port；只依赖Context自身`contract/kernel`与Go标准库；
- live Memory/Knowledge Owner已分别发布唯一的`MemoryContextSourceCurrentReaderV1`与`KnowledgeContextSourceCurrentReaderV1`及其Owner DTO。Offline SDK V1对二者的依赖和调用数均为0；未来若Context确需消费Memory/Knowledge source，联合评审只能要求对应Owner发布additive V2或一个唯一、无损的public facade，Context不定义、复制或依赖第二套平行Owner nominal/DTO，也不依赖Owner concrete Store/internal实现；
- SDK无Effect、Review、Fence、Run Requirement、pre-run Evidence或Settlement；输出不能注册Capability或推进Turn；
- Owner-local design与Go独立软件验收均为YES；该状态只覆盖离线SDK/Ingress，不影响production C层持续NO-GO。

## 2. CLI

### 2.1 `ContextOfflineIngressV1` Owner-local只读首切面

Owner-local开发者入口当前复用六个只读operation：`context recipe validate`、`context recipe compare`、`context recipe compile`、`context recipe preview`、`context frame inspect`、`context cache inspect`。Recipe compare不返回`better/compatible/publish`建议；Cache inspect不返回hit、Entry current事实或Provider状态。命令从stdin读取一个严格JSON request document，stdout只输出成功response JSON，stderr只输出结构化typed error；不读任意文件、不启动listener、不持有Store、不注册Capability。publish/rollback/revoke、cache写、远程评测、replay与跨Owner source均不属于本切面。

SDK必须公开每个request的`Seal*RequestV1`与`Encode*RequestV1`：Seal在canonical body中将自身`RequestDigest`置空后计算exact digest，输入已有digest时只接受空值或exact相等；Encode先Seal/Validate，再使用与Decode相同的operation-specific wire cap、48 KiB base64 chunk与context-aware bounded writer。调用者不得复制私有canonical或bundle wire算法。`WireCapsV1`只公开冻结的六组hard cap，不授予更大request limit。

`offlineapi.ContextOfflineAPIV1`提供六个typed方法和一个`ExecuteJSON(ctx, operation, payload)`。JSON路径必须严格执行对应SDK `Decode -> typed operation -> Encode response`，因此递归duplicate key、unknown field、trailing document、presence、limits、cancel/deadline和no-alias语义均只有一条实现路径。API没有transport、auth、Watch、CAS或Owner current语义。

CLI退出码闭集为：`0=success`、`2=invalid_argument|limit_exceeded|unsupported`、`3=not_found|expired|conflict`、`4=unauthorized`、`5=canceled|deadline_exceeded`、`1=internal_failure或非typed错误`。错误JSON保持SDK code/operation/field_path/message；不得打印请求正文、Content bytes、Secret或内部cause。该退出码只覆盖Owner-local离线入口，不冒充Admission/Review/Settlement状态。

| 命令族 | 只读 | 受治理写入 |
|---|---|---|
| `context recipe` | 当前：validate、compare、compile、preview；候选：inspect | publish、rollback、revoke |
| `context frame` | 当前：inspect；候选：diff、replay、explain | prepare（创建Attempt/Frame事实） |
| `context parent-current` | inspect、explain-source、explain-expiry | 无；V1严格只读 |
| `context cache` | 当前：inspect；候选：plan、explain-miss、usage | warm、refresh、invalidate、delete |
| `context anchor` | inspect、diff、explain | rebase |
| `context evaluate` | report、compare | run（真实/远程评测可能是Effect） |

所有未来写命令先显示Candidate/Scope/Authority/Review/Budget摘要，调用获批治理API而非直接写Store。当前`ContextOfflineIngressV1`没有写命令；更广退出码需随对应治理合同另行冻结。

## 3. Transport-neutral API

`ContextOfflineIngressV1`的API只是进程内、transport-neutral的只读函数面，不等于下述未来Domain Fact/Compile Governance API，也不宣称production API server/root。

API分三类：

1. Domain Fact API：Create/Inspect/CAS/List，所有CAS必须ExpectedRevision；
2. Compile API：ReserveAttempt、SubmitCandidate、FreezeManifest、FreezeFrame、InspectAttempt；
3. Governance API：ProposeRelease/CacheOperation、AttachPermit、RecordObservation、Inspect/Settle。

G6A只读前置的Context-owned V1合同已经实现并通过隔离验证：

1. `ContextParentFrameCurrentReaderV1.InspectCurrent`：请求绑定exact Source/Frame/Manifest/Generation/ExecutionScopeDigest/Run/Session/Turn/checked time/not-after，执行S1→完整`InspectFrame`→S2，返回Context-owned current projection；
2. `ContextParentFrameApplicabilitySourceCoordinateV1.InspectExact`：按完整Kind/ID/Revision/Digest从Context Owner metadata index解析sealed subject；`ID=FrameID`只作首个查询key，禁止普通FactRef、Application DTO或Runtime公共ref type-pun；
3. metadata readers：`ResolveExactSourceBinding`后只按完整Frame/Manifest/Generation `FactRef{ID,Revision,Digest}`与预期scope读取；同ID换版本/摘要或跨Tenant/scope歧义返回Conflict。

Reader返回Unavailable/Stale/Conflict时API不得降级为`Current=false`成功响应或缓存旧projection；调用链必须在Evidence Issue、Tool watermark和Provider之前Fail Closed。

per-turn接线已包含Application版本化公共三段Port与Owner Source Reader。当前CTX-D09 Owner-local三段合同、kernel/store、Context Adapter及Memory/Knowledge B-cross fixture已经实现并验证；在production验收前保持root、Capability、Harness Continuation与Turn推进unavailable：

1. Application-owned `ContextTurnRefreshPortV1`：精确三段`RefreshContextTurnV1 / ApplyContextTurnRefreshV1 / InspectContextTurnRefreshV1`。Refresh核验settled Tool exact chain并只产生pending Context DomainResult；Apply先S2 fresh复读，再原子执行Context ApplySettlement+Generation current expected-CAS；Inspect只读恢复原Attempt；
2. Application-owned `ContextOwnerSourceReaderV1`：Memory/Knowledge各自唯一V2 Reader通过中立DTO返回S1/S2 exact projection与有界正文；Context不拥有其领域事实；Continuity计数固定0；
3. `ContextFrameInspectPortV1.InspectExact`：重算Manifest/SourceSet/稳定区段/Rendered refs，只读且禁止联网；
4. `InjectionConformanceInspectPortV1.Inspect`：接收Expected、Harness Actual及对应Provider Observations，返回Context Owner Conformance Fact。

`ContextTurnRefreshRequestV1`必须包含确定性Attempt/Frame/Generation IDs、G6A exact链、scope/run/session/turn、CTX-D10 ParentFrame/Manifest/Generation current投影、Recipe、stable source set/cache identity、source projection及NotAfter。Prepared响应只是pending，current pointer不可见。`CTX-D09-R1`已冻结：Apply DTO没有任何Runtime settlement/ref字段；禁止使用V4、additive Runtime settlement或Tool settlement。只有S2通过且原子Apply/CAS成功才是`applied_current`。

请求携带ContractVersion、SchemaRef、IdempotencyKey、Scope/Run/Turn/Attempt、Owner Binding和deadline。响应只返回不可变Ref/Fact/Observation，不返回内部可变对象。Watch/Page使用有界cursor。

## 4. Adapter边界

- `runtimeadapter`：实现既有Runtime `OperationScopeEvidenceApplicabilityCurrentReaderV3`的Context Kind路由；只接受公共ref四元组并交给Owner Reader解析/复读，无损返回公共current projection，不创建Applicability/Evidence Fact、不延长Expiry，也不注入metadata快照或sealed binding map；
- `applicationadapter`：Context Owner实现Application发布的三段Refresh公共Port，只依赖Application公共`contract/ports`，把Prepare/Apply/Inspect映射为领域调用；消费Tool owner-local current Reader及Application中立Memory/Knowledge projection，不拥有Runtime Settlement编排或Harness Continuation；
- `harnessadapter`：由Harness/集成Owner实现，把已物化Frame转为私有ContextSnapshot，并聚合Run/Turn/Frame/Route/Attempt/source sequence为ActualInjectionManifest；本组件不导入Harness私有Port，Adapter不判断Conformance；
- `modelinvokeradapter`：由Model Invoker/集成Owner实现ProviderCacheProfile、Frame渲染和Route级ProviderActualInjectionObservation；
- `sourceadapter`：各领域提供Candidate，不能调用Context内部Store。

分段门禁：`CTX-D09-R1`已冻结，A/B-local、Application公共Port Adapter与Memory/Knowledge B-cross fixture已完成；禁止私建Runtime settlement fallback。C层真实接线与能力启用只能由production composition root在系统验收后完成。

## 5. Rust裁决

V1没有证据证明候选去重、diff、canonical hashing或token估算已成为计算热点，因此不规划Rust、FFI或独立Rust服务。未来只有在代表性数据集的Go profile证明纯计算热点持续占CPU至少30%，且隔离基准显示Rust能提供至少2倍吞吐或40% CPU下降、同时可定义确定的超时/崩溃/回退语义后，才重新提交独立设计；该门槛不是本计划依赖。
