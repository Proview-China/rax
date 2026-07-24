# Prompt Upstream Provenance V1

状态：Owner-local离线合同、seal/verify核与unit/blackbox/fault/conformance已实现，target100/race20/full ordinary/race/vet通过；不抓取网络、不裁决许可证、不发布Prompt、不绑定production Route。来源等级与审计清单见[官方上游审计](prompt-upstream-audit.md)。

## 1. Owner与边界

Context Owner拥有`PromptUpstreamProvenanceV1`、离线Verify Report及其exact digest。上游仓库只提供候选bytes；Model Invoker拥有Model/Profile/Route；T3Code只是未来消费端；Application/Harness不拥有Prompt来源或变换事实。

本合同回答“这些Prompt片段从哪来、经过什么确定性变换、stable/semi/dynamic如何闭包”，不回答“哪个线上模型当前应使用它”。后者必须依赖Model Invoker未来公开的nominal exact Profile ref/current reader；不得在Context复制`ModelFamily`或`RouteID`类型。

## 2. exact Go合同

```go
const PromptUpstreamProvenanceContractVersionV1 = "praxis.context.prompt-upstream-provenance/v1"

type PromptUpstreamSourceClassV1 string

const (
    PromptSourceOfficialCodingAgentV1 PromptUpstreamSourceClassV1 = "official_coding_agent_prompt"
    PromptSourceOfficialSDKPresetV1   PromptUpstreamSourceClassV1 = "official_agent_sdk_preset_reference"
    PromptSourceOfficialModelTemplateV1 PromptUpstreamSourceClassV1 = "official_model_chat_template"
    PromptSourceOfficialPolicyPrefixV1 PromptUpstreamSourceClassV1 = "official_policy_prefix"
    PromptSourceComparativeOpenSourceV1 PromptUpstreamSourceClassV1 = "comparative_open_source"
)

type PromptUpstreamRangeV1 struct {
    Start uint64 `json:"start"`
    End   uint64 `json:"end"` // exclusive; Start < End <= artifact ByteLength
    Digest Digest `json:"digest"`
}

type PromptUpstreamArtifactV1 struct {
    ID             string                  `json:"id"`
    Repository     string                  `json:"repository"`
    Commit         string                  `json:"commit"` // lowercase 40-hex
    Path           string                  `json:"path"`
    MediaType      string                  `json:"media_type"`
    ByteLength     uint64                  `json:"byte_length"`
    ContentDigest  Digest                  `json:"content_digest"`
    ExtractedRanges []PromptUpstreamRangeV1 `json:"extracted_ranges"`
}

type PromptUpstreamLicenseV1 struct {
    SPDXID          string        `json:"spdx_id"`
    Repository      string        `json:"repository"`
    Commit          string        `json:"commit"`
    Path            string        `json:"path"`
    ByteLength      uint64        `json:"byte_length"`
    ContentDigest   Digest        `json:"content_digest"`
    ReviewEvidence  []EvidenceRef `json:"review_evidence"`
}

type PromptTransformKindV1 string
const (
    PromptTransformExtractV1      PromptTransformKindV1 = "extract"
    PromptTransformRemoveClaimsV1 PromptTransformKindV1 = "remove_host_claims"
    PromptTransformParameterizeV1 PromptTransformKindV1 = "parameterize"
    PromptTransformSplitClosureV1 PromptTransformKindV1 = "split_closure"
    PromptTransformCanonicalizeV1 PromptTransformKindV1 = "canonicalize"
)

type PromptTransformStepV1 struct {
    ID            string                `json:"id"`
    Revision      uint64                `json:"revision"`
    Kind          PromptTransformKindV1 `json:"kind"`
    InputDigest   Digest                `json:"input_digest"`
    RulesDigest   Digest                `json:"rules_digest"`
    ToolDigest    Digest                `json:"tool_digest"`
    OutputDigest  Digest                `json:"output_digest"`
}

type PromptClosureManifestV1 struct {
    Stable          []ContentRef `json:"stable"`
    SemiStable      []ContentRef `json:"semi_stable"`
    DynamicTemplate []ContentRef `json:"dynamic_template"`
    StableDigest    Digest       `json:"stable_digest"`
    SemiStableDigest Digest      `json:"semi_stable_digest"`
    DynamicDigest   Digest       `json:"dynamic_digest"`
    ClosureDigest   Digest       `json:"closure_digest"`
}

type PromptUpstreamProvenanceV1 struct {
    ContractVersion  string                      `json:"contract_version"`
    ID               string                      `json:"id"`
    Revision         uint64                      `json:"revision"`
    SourceClass      PromptUpstreamSourceClassV1 `json:"source_class"`
    SourceProduct    string                      `json:"source_product"`
    Artifacts        []PromptUpstreamArtifactV1  `json:"artifacts"`
    License          PromptUpstreamLicenseV1     `json:"license"`
    SourceSetDigest  Digest                      `json:"source_set_digest"`
    TransformChain   []PromptTransformStepV1     `json:"transform_chain"`
    GeneratedContent []ContentRef                `json:"generated_content"`
    GeneratedSetDigest Digest                    `json:"generated_set_digest"`
    Closure          PromptClosureManifestV1     `json:"closure"`
    Evidence         []EvidenceRef               `json:"evidence"`
    CreatedUnixNano  int64                       `json:"created_unix_nano"`
    ExpiresUnixNano  int64                       `json:"expires_unix_nano"`
    ProvenanceDigest Digest                      `json:"provenance_digest"`
}

type PromptUpstreamProvenanceRefV1 struct {
    ID       string `json:"id"`
    Revision uint64 `json:"revision"`
    Digest   Digest `json:"digest"`
}
```

`SourceProduct`只标识来源产品（如`openai-codex`、`gemini-cli`、`kimi-code`、`grok-build`），不是目标`ModelFamily`。SDK preset引用没有可审计正文时，`GeneratedContent`必须为空并使用独立reference-only validation分支；它不得伪装成imported PromptAsset。

## 3. canonical与闭包

- Artifact按`ID`排序且唯一；range按`Start/End`排序、不得交叠；原始bytes必须重算完整文件与每个range digest。
- `SourceSetDigest`覆盖domain/version及完整Artifact metadata，不覆盖调用时临时bytes容器。
- Transform按顺序执行；第一步`InputDigest == SourceSetDigest`，后续`InputDigest == previous.OutputDigest`，末步`OutputDigest == GeneratedSetDigest`。
- Generated ContentRef按`ID/Revision/Digest`规范排序且唯一，调用方提供bytes时必须校验`Length`与digest；零Length继续非法。
- stable/semi/dynamic三个集合各自规范排序、两两不相交、并集exact等于GeneratedContent；四个closure digest全部重算。
- Provenance digest置空自身字段后覆盖完整对象；所有slice返回deep-copy/no-alias。
- `official_agent_sdk_preset_reference`允许零GeneratedContent，但必须有exact Artifact、License、Evidence和非空Transform链末端reference digest；其他等级至少一项GeneratedContent。

## 4. 离线Verify入口

```go
type PromptUpstreamArtifactBytesV1 struct {
    ArtifactID string `json:"artifact_id"`
    Bytes      []byte `json:"bytes"`
}

type PromptGeneratedContentBytesV1 struct {
    Ref   ContentRef `json:"ref"`
    Bytes []byte     `json:"bytes"`
}

type VerifyPromptUpstreamProvenanceRequestV1 struct {
    Provenance       PromptUpstreamProvenanceV1       `json:"provenance"`
    ArtifactBytes    []PromptUpstreamArtifactBytesV1  `json:"artifact_bytes"`
    LicenseBytes     []byte                            `json:"license_bytes"`
    GeneratedBytes   []PromptGeneratedContentBytesV1  `json:"generated_bytes"`
    CheckedUnixNano  int64                            `json:"checked_unix_nano"`
    MaxInputBytes    uint64                           `json:"max_input_bytes"`
    RequestDigest    Digest                           `json:"request_digest"`
}

type PromptUpstreamVerificationReportV1 struct {
    ProvenanceRef    PromptUpstreamProvenanceRefV1 `json:"provenance_ref"`
    SourceSetDigest  Digest  `json:"source_set_digest"`
    GeneratedSetDigest Digest `json:"generated_set_digest"`
    ClosureDigest    Digest  `json:"closure_digest"`
    VerifiedArtifactIDs []string `json:"verified_artifact_ids"`
    VerifiedContentRefs []ContentRef `json:"verified_content_refs"`
    CheckedUnixNano  int64 `json:"checked_unix_nano"`
    ExpiresUnixNano  int64 `json:"expires_unix_nano"`
    ReportDigest     Digest `json:"report_digest"`
}
```

首版hard max：16 Artifacts、64 ranges、64 transform steps、64 generated items、Artifact+License+Generated总输入32MiB。License bytes必须重算length/digest，不能只相信metadata。任何limit、digest、range、chain、closure、TTL、cancel错误返回零Report。检查点至少位于preflight前、每个artifact/range、license、每个generated item、每个transform step、closure验证后和返回前。

`PromptUpstreamProvenanceRefV1`是Context-owned nominal exact ref；普通`FactRef`、PromptAssetRef或RecipeRef不得代入。Verify只证明调用方提交的bytes与sealed记录一致，不证明语义删改正确或许可证可用；`ReviewEvidence`与Prompt lifecycle仍必须独立审核。

## 5. Public Port Delta

Model Invoker Owner后续需提供additive public合同：

```go
type SemanticRouteProfileRefV1 struct { ID string; Version string; Digest string }
type SemanticRouteProfileCurrentReaderV1 interface {
    InspectCurrent(ctx context.Context, exact SemanticRouteProfileRefV1, checked time.Time) (CurrentProjectionV1, error)
}
```

projection至少无损包含exact profile ref、selection/route identity digest、model behavior profile ref/digest、harness stack digest、context mode、Checked/Expires/ProjectionDigest。Context只消费该公共ref/reader，不import Model Invoker internal或复制profile对象；Unavailable/Unknown/cancel/deadline Fail Closed。T3Code适配在宿主composition root消费同一公共ref。

## 6. 验收反例

- 同commit/path换bytes、同bytes换range、range越界/交叠；
- License digest或ReviewEvidence缺失；
- SDK preset无正文却伪造GeneratedContent；
- transform断链、跳步、末步不等于GeneratedSet；
- closure漏项、重复、跨层复用、digest漂移；
- zero-byte ContentRef或bytes length/digest漂移；
- cancel后返回partial report；
- 把SourceProduct当ModelFamily、按字符串路由、导入T3Code/Model Invoker实现类型；
- Provenance验证成功后直接published或ActualInjection matched。
