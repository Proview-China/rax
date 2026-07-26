# Context Frame Consumption、Local Cache与Compression Evidence V1

状态：Design Candidate。合并确认后才允许进入Context Owner-local Go实现；本文件不授权production composition、Harness Continuation、Model/Provider调用、远程服务或生产持久化。

本设计只冻结Context Owner输出immutable provider-neutral `ContextFrame`供下游消费的首个纵切。Context不定义Model Projection DTO/Port：provider-specific Projection及Projection Cache归Model Invoker，API Prefix/KV Cache归Provider Adapter或本地推理后端。

## 1. Owner与非Owner

| 对象 | Owner | 边界 |
|---|---|---|
| `ContextFragment`、`ContextManifest`、immutable provider-neutral `ContextFrame` | Context | Frame是Context语义权威；下游不得反写 |
| Fragment Cache、Frame Cache | Context | 仅加速exact materialization，不替代Owner current |
| `ContextFrameConsumptionDescriptorV1`、cache hints/fingerprint/invalidation reasons | Context | 下游消费描述，不是cache hit或执行资格 |
| `StructuralValueEvaluationV1`、`CompressionEvidenceV1` | Context | advisory，不是正确性证明、Verdict、Authority或Runtime Evidence |
| provider-specific Model Projection及Projection Cache | Model Invoker | Context不得定义第二套Model DTO/Port |
| API Prefix/KV Cache与opaque handle | Provider Adapter/本地推理后端 | Context只给hint/fingerprint，不认定hit |
| Tool/MCP执行结果和settled资格 | Tool Owner | Context只消费批准公共投影中的exact ref/revision/digest/TTL |

依赖方向：

```text
Tool Owner settled ToolResult exact projection
-> 宿主/Application中立映射
-> Context ToolResult Fragment
-> incremental immutable ContextFrame
-> ContextFrameConsumptionDescriptorV1
-> Model Invoker自行生成provider-specific Projection
-> Provider Adapter自行管理Prefix/KV Cache
```

Context不得import Tool/MCP、Application、Harness或Model实现包，也不得复制其Owner nominal。本Design PR不新增跨Owner公共Port。

## 2. Immutable Frame消费边界

`ContextFragment`、`ContextManifest`和`ContextFrame`继续复用live contract，不建立V2副本：

- Fragment只持有admitted Candidate exact ref、kind、region、position、ContentRef与tokens；
- Manifest exact绑定Recipe、Execution、Generation、Parent Frame、Admission Decisions、规范排序Fragments、分区token totals与SourceSetDigest；
- Frame exact绑定Manifest、Parent、Generation、StablePrefix/SemiStable/DynamicTail/Rendered ContentRef与SourceSetDigest；
- Frame冻结后不可变。下游materialization、render、cache miss、eviction、压缩评分均不得修改Frame或延长其TTL。

### 2.1 `ContextFrameConsumptionDescriptorV1`

该对象是Context输出给下游的只读materialization descriptor，不是Model Projection、Provider Request或Capability。

```go
type ContextFrameConsumptionDescriptorRefV1 struct {
    ID       string
    Revision uint64
    Digest   Digest
}

type ContextFrameConsumptionRequestV1 struct {
    ContractVersion       string
    DescriptorID          string
    FrameRef              FactRef
    ManifestRef           FactRef
    GenerationRef         FactRef
    TenantScopeDigest     Digest
    AgentInstanceRef      FactRef
    RunID                 string
    RunScopeDigest        Digest
    RendererVersion       string
    ModelFamily           string
    ModelProfileRef       FactRef
    ToolSchemaDigest      Digest
    PromptAssetRefs       []PromptAssetRefV1
    RecipeRef             FactRef
    DisclosureClass       DisclosureClassV1
    CheckedUnixNano       int64
    NotAfterUnixNano      int64
    RequestDigest         Digest
}

type ContextFrameConsumptionDescriptorV1 struct {
    ContractVersion       string
    ID                    string
    Revision              uint64
    FrameRef              FactRef
    ManifestRef           FactRef
    GenerationRef         FactRef
    StablePrefix          ContentRef
    SemiStable            *ContentRef
    DynamicTail           ContentRef
    Rendered              ContentRef
    FragmentRefs          []FactRef
    TenantScopeDigest     Digest
    AgentInstanceRef      FactRef
    RunID                 string
    RunScopeDigest        Digest
    RendererVersion       string
    ModelFamily           string
    ModelProfileRef       FactRef
    ToolSchemaDigest      Digest
    PromptAssetRefs       []PromptAssetRefV1
    RecipeRef             FactRef
    DisclosureClass       DisclosureClassV1
    CacheHint             ContextCacheHintV1
    CheckedUnixNano       int64
    ExpiresUnixNano       int64
    Digest                Digest
}
```

`ModelFamily/ModelProfileRef`只作为下游消费条件和cache fingerprint维度，不定义Model消息、role、Projection或Provider协议。

### 2.2 Presence、排序与Digest

- Frame/Manifest/Generation与所有ContentRef必须exact并通过完整`InspectFrame`。
- `FragmentRefs`必须与Manifest admitted Fragments一一对应且顺序一致。
- `PromptAssetRefs`按`ID,Revision,Digest`排序且唯一；nil规范化为非nil empty。
- `SemiStable=nil`表示Frame没有该区域；非nil必须Length大于0。
- `Created/Checked < Expires <= NotAfter`；等于边界即不可消费。
- Request Digest使用domain=`praxis.context/frame-consumption-request-v1`，seal版本、operation discriminator及置空自身Digest后的完整Request。
- Descriptor Digest使用domain=`praxis.context/frame-consumption-v1`，seal除自身Digest置空外的完整Descriptor。
- Descriptor Ref Digest必须等于完整Descriptor Digest，禁止只seal ID、FrameRef、Rendered或Fingerprint。

生成算法固定为S1 exact读取Frame/Manifest/Generation/ContentRefs并完整Inspect，构造Descriptor后执行S2相同Owner-current复读。任一ref、digest、current pointer、scope、renderer、model profile、tool schema、prompt/recipe、disclosure或TTL漂移时零Descriptor、零Cache写。

## 3. Context Cache Hint与Fingerprint

```go
type ContextCacheHintV1 struct {
    StableEligible        bool
    SemiStableEligible    bool
    Fingerprint           Digest
    InvalidationReasons   []ContextCacheInvalidationReasonV1
    ExpiresUnixNano       int64
}
```

`InvalidationReasons`闭集为：

```text
frame_changed
manifest_changed
generation_changed
fragment_changed
renderer_changed
model_profile_changed
tool_schema_changed
prompt_revision_changed
recipe_revision_changed
disclosure_changed
scope_changed
ttl_expired
owner_current_unknown
```

Fingerprint必须seal：

```text
tenant scope + agent exact ref + run ID/scope
+ Frame/Manifest/Generation exact refs
+ Stable/SemiStable/DynamicTail ContentRefs
+ renderer version + model family/profile exact ref
+ tool schema digest + prompt exact refs + recipe exact ref
+ disclosure class + cache key version
```

Context可以说明哪些区域稳定及为什么失效，但：

- 不生成Provider cache handle；
- 不读取或写入Provider Prefix/KV Cache；
- 不把Provider usage/cached token自报解释为hit；
- hint/fingerprint相同也不授予Provider复用资格；
- miss、eviction和restart不改变Frame真值。

## 4. Fragment Cache与Frame Cache

### 4.1 Key闭包

```go
type ContextLocalCacheKeyClosureV1 struct {
    ContractVersion        string
    Layer                  ContextCacheLayerV1
    TenantScopeDigest      Digest
    AgentInstanceRef       FactRef
    RunID                  string
    RunScopeDigest         Digest
    FrameRef               FactRef
    ManifestRef            FactRef
    GenerationRef          FactRef
    FragmentRef            *FactRef
    RendererVersion        string
    ModelFamily            string
    ModelProfileRef        FactRef
    ToolSchemaDigest       Digest
    PromptAssetRefs        []PromptAssetRefV1
    RecipeRef              FactRef
    DisclosureClass        DisclosureClassV1
    InvalidationGeneration uint64
    KeyVersion             string
    Digest                 Digest
}
```

Fragment Cache V1仍绑定消费它的exact Frame，禁止通过省略Frame/scope维度进行跨Frame复用。Frame Cache value是已完整Inspect的immutable Frame/Manifest/Generation/ContentRef闭包，不替代权威metadata/current store。

### 4.2 TTL

```text
Expires = min(
  request.NotAfter,
  Frame/Manifest/Generation current window,
  every Fragment Source current window,
  renderer current window,
  Model Profile current window,
  Tool Schema current window,
  Prompt/Recipe current window,
  Disclosure/Authority current window,
  cache policy upper bound
)
```

未知上界不得伪造；Cache不得延长任何Owner TTL或因访问自动续期。`checked >= expires`为miss并重新走Owner-current materialization。

### 4.3 并发、失效与no-alias

Entry状态闭集为`current | expired | invalidated`：

- 同Key、同Value Digest并发Put幂等；
- 同Key、不同Value Digest为`conflict`，禁止last-write-wins；
- Get返回deep copy/no-alias，并在返回前复验Key、Value Digest、TTL和InvalidationGeneration；
- Invalidate只单调增加generation；旧generation全部miss；
- eviction只删除派生Entry，不发布领域Fact；
- 64并发相同输入只能观察一个canonical value；
- cancel/Unknown/lost reply后只Inspect原Key/Attempt，不换ID盲写。

## 5. settled ToolResult到Incremental Frame

MCP调用在Context边界只作为Tool Owner已经settled的ToolResult；Context不定义第二套MCP Result。

```go
type ToolResultFragmentMaterializationRequestV1 struct {
    ContractVersion       string
    ParentFrameRef        FactRef
    ToolResultRef         FactRef
    ToolDomainResultRef   FactRef
    ToolApplyRef          FactRef
    ToolCurrentRef        FactRef
    AssociationRef        FactRef
    ExecutionScopeDigest  Digest
    RunID                 string
    SourceTurn            uint32
    ActionID              string
    AttemptID             string
    InlineContent         *ContentRef
    ArtifactRef           *ArtifactAnchor
    BoundedSummary        *ContentRef
    CheckedUnixNano       int64
    NotAfterUnixNano      int64
    RequestDigest         Digest
}
```

规则：

- Tool exact refs必须绑定同一Execution/Action/Attempt/Result，且Revision/Digest/TTL current；
- `InlineContent`只允许`<= min(Recipe.ToolInlineMaxBytes, 64 KiB)`且仍受token预算约束；
- 大正文必须走exact `ArtifactAnchor{Owner,Version/Digest,Range}`和有界摘要；Frame不得复制原始大对象；
- Inline与Artifact分支exact-one；大结果缺Artifact或摘要时Fail Closed；
- Receipt、Observation、Provider response、未settled Result、raw/unbounded output在任何Cache/Frame写前拒绝；
- 结果只能形成`tool_result`或`artifact_reference` Fragment，不能形成instruction、Authority、Tool Schema或Review Verdict；
- Parent Frame不可变；新增Fragment只进入incremental child Frame，StablePrefix/SemiStable exact refs保持，动态内容进入DynamicTail。

## 6. `StructuralValueEvaluatorV1`

Evaluator为可插拔、非权威、advisory组件。V1默认是确定性启发式，不联网、不调用小模型；本PR明确不实现Epiplexity或其他小模型Evaluator。

```go
type StructuralValueEvaluationRequestV1 struct {
    ContractVersion       string
    SourceFrameRef        FactRef
    CandidateOutputRef    ContentRef
    RequiredAnchorRefs    []FactRef
    RetainedAnchorRefs    []FactRef
    SourceTokenCount      uint64
    CandidateTokenCount   uint64
    EvaluatorRef          FactRef
    CheckedUnixNano       int64
    NotAfterUnixNano      int64
    RequestDigest         Digest
}

type StructuralValueEvaluationV1 struct {
    ContractVersion       string
    ID                    string
    Revision              uint64
    RequestDigest         Digest
    EvaluatorRef          FactRef
    StructuralValuePPM    uint32
    CoveragePPM           uint32
    LossPPM               uint32
    PreservedRequired     bool
    Limitations           []string
    CheckedUnixNano       int64
    ExpiresUnixNano       int64
    Digest                Digest
}
```

PPM范围为`0..1_000_000`。默认启发式只使用冻结输入：required anchor exact retention、结构化section/identifier保留、source/candidate token比、重复移除与Delta闭包。同输入必须逐字节同输出。

评分不得删除required anchor、权限/安全规则、Tool Schema、用户明确纠正、未完成Effect或不可压缩引用。Evaluator Unknown/Unavailable/cancel/deadline时只能回退既有安全确定性压缩，或Fail Closed并保留原Frame。

## 7. `CompressionEvidenceV1`

```go
type CompressionEvidenceV1 struct {
    ContractVersion       string
    ID                    string
    Revision              uint64
    SourceFrameRef        FactRef
    SourceGenerationRef   FactRef
    CandidateOutputRef    ContentRef
    CandidateFrameRef     *FactRef
    RetainedAnchorRefs    []FactRef
    DeltaRefs             []FactRef
    RequiredAnchorRefs    []FactRef
    EvaluationRef         FactRef
    EvaluatorIdentity     string
    EvaluatorVersion      string
    SourceTokens          uint64
    CandidateTokens       uint64
    CoveragePPM           uint32
    LossPPM               uint32
    Limitations           []string
    InvariantGateDigest   Digest
    CheckedUnixNano       int64
    ExpiresUnixNano       int64
    Digest                Digest
}
```

Evidence只说明候选如何生成和评估，不是正确性证明、Review Verdict、Runtime Evidence或Authority。

确定性Invariant Gate在Evaluator前后各执行一次：

1. Source Frame/Generation exact-current且未过期；
2. required anchors逐项exact保留；
3. 权限、安全、Tool Schema、用户纠正、open effects和不可压缩引用未删除或降格；
4. Retained Anchors与Delta链可复算、无环且未超Recipe上限；
5. Candidate ContentRef bytes/digest/length一致；
6. candidate token/byte预算未超限；
7. Evaluation/Evidence绑定同一Source、Candidate和Evaluator版本；
8. S2复读无current、TTL、scope或disclosure漂移。

Gate失败不得生成current child Frame/Generation；最多保留非current诊断。Evaluator高分不能绕过Gate。

## 8. 错误闭集

| Error | 条件 | 恢复 |
|---|---|---|
| `invalid_argument` | presence、闭集、排序、Digest或exact-one非法 | 修正请求；零写 |
| `unauthorized` | tenant/agent/run/disclosure/authority不匹配 | 重新取得Owner current |
| `not_found` | exact Frame/Content/Tool/Artifact不存在 | exact reread；不猜测 |
| `expired` | 任一依赖TTL到期/crossing | 重新派生；不延寿 |
| `conflict` | 同Key异Digest、scope/current/anchor/delta漂移 | Inspect current并使用新Attempt |
| `unavailable` | Owner Reader/Store确定不可用 | 保留原Frame后重试读取 |
| `indeterminate` | Unknown、cancel、deadline或lost reply | 只Inspect原Attempt/Key |
| `limit_exceeded` | bytes/token/items/delta深度超限 | 安全降级，否则Fail Closed |
| `unsupported` | Provider DTO/cache handle、Epiplexity或跨FrameCache复用 | 不执行 |

Cache miss/eviction不是领域错误。任何错误均不得修改原Frame或触发Provider/Harness调用。

## 9. 硬反例

- 相同FrameID但Revision/Digest不同仍命中Cache；
- cache key遗漏tenant/agent/run、Frame exact ref、renderer、model、tool schema、prompt/recipe或disclosure；
- Entry TTL长于任一Owner current window；
- eviction后改变Frame Digest；
- Provider cached tokens自报被认定为Context hit；
- Context定义Model Projection DTO、Projection Cache或Provider KV handle；
- Receipt、Observation、未settled ToolResult或MCP raw response进入Frame；
- 大Tool正文以内联字符串绕过ArtifactRef、摘要和64 KiB上限；
- Evaluator高分删除required anchor、安全规则、Tool Schema或open effect；
- Evaluator失败后自动接受候选摘要；
- Compression Evidence冒充正确性证明、Verdict、Authority或Runtime Evidence；
- 同Key异Value并发last-write-wins；
- cancel/lost reply后换ID重写Cache或child Generation。

以上全部Fail Closed或退回原Frame，且零Provider调用、零Harness Continuation、零Runtime Settlement。

## 10. 后续Implementation PR独占范围

Design PR合并后仅允许：

- `contract`：Consumption Descriptor、Cache Hint/Key、Tool materialization、Evaluation/Evidence；
- `kernel`：exact descriptor、ToolResult incremental Frame、Invariant Gate、默认确定性Evaluator；
- `fragmentcache`、`framecache`：线程安全Memory reference implementation；
- unit/whitebox/blackbox/fault/conformance/race测试。

继续NO-GO：

- Model Projection/Projection Cache DTO或实现；
- Application/Harness/Tool/Model/Provider生产Adapter；
- Provider Prefix/KV Cache；
- Epiplexity/远程Evaluator；
- production State Plane、Capability、Continuation、Turn推进、远程服务和SLA。
