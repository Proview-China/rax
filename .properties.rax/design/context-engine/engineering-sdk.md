# Context Engineering SDK V1

状态：用户已确认“Prompt开发入口 + 可插拔Evaluator/Policy”及“API+CLI五入口”方向；Owner-local typed/strict Go首切面已实现并通过分层、高重复及race/vet验证，transport-neutral API与stdin/stdout CLI进入本轮实现。既有`ContextOfflineSDKV1`六operation闭集保持不变；远程Judge、production发布、Capability及跨Owner root仍NO-GO。

## 1. 目标与非目标

`ContextEngineeringSDKV1`补齐两条Owner-local开发链：

```text
PromptAsset -> validate -> preview exact ContextCandidates

exact ContextOutcomes -> prepare EvaluationInput
  -> pluggable Evaluator returns Observation
  -> Context admit/verify -> ContextEvaluationFact
  -> build -> ContextFeedbackCandidateFact
```

它不是production发布面，不注册Capability，不调用Harness/Model Provider，不推进Turn，不写Runtime Outcome/Settlement，不把cache usage、模型自评或单次任务成功自动升级为“better”。`publish/rollback/revoke`继续由CTX-D07阻塞。

## 2. 版本与operation闭集

```go
const ContextEngineeringSDKContractVersionV1 = "praxis.context.engineering-sdk/v1"

type ContextEngineeringOperationV1 string

const (
    EngineeringValidatePromptAssetV1 ContextEngineeringOperationV1 = "validate_prompt_asset"
    EngineeringPreviewPromptV1       ContextEngineeringOperationV1 = "preview_prompt_candidates"
    EngineeringPrepareEvaluationV1   ContextEngineeringOperationV1 = "prepare_context_evaluation"
    EngineeringAdmitEvaluationV1     ContextEngineeringOperationV1 = "admit_context_evaluation"
    EngineeringBuildFeedbackV1       ContextEngineeringOperationV1 = "build_feedback_candidate"
)
```

不得把这五项追加进`OfflineSDKOperationV1`或改变既有六入口canonical/wire行为。用户已确认用独立`ContextEngineeringAPIV1`包装同一typed SDK/strict codec，并在既有`context`命令增加五个stdin/stdout只读命令：`prompt validate|preview`、`evaluation prepare|admit`、`feedback build`。API/CLI不新增server、listener、auth、Store、Capability、Provider调用、Prompt发布或production root。

## 3. 共同Envelope、limits与错误

```go
type ContextEngineeringLimitsV1 struct {
    MaxPromptFragments uint32 `json:"max_prompt_fragments"` // 1..64
    MaxOutcomes        uint32 `json:"max_outcomes"`         // 1..64
    MaxNestedRefs      uint32 `json:"max_nested_refs"`      // 1..32768
    MaxEvidenceRefs    uint32 `json:"max_evidence_refs"`    // 1..512
    MaxDiagnostics     uint32 `json:"max_diagnostics"`      // 1..1024
    MaxCanonicalBytes  uint64 `json:"max_canonical_bytes"`  // 1..32 MiB
    MaxWireBytes       uint64 `json:"max_wire_bytes"`       // 1..48 MiB
}

type ContextEngineeringRequestMetaV1 struct {
    ContractVersion string                     `json:"contract_version"`
    RequestID       string                     `json:"request_id"`
    Operation       ContextEngineeringOperationV1 `json:"operation"`
    Limits          ContextEngineeringLimitsV1 `json:"limits"`
    RequestDigest   Digest                     `json:"request_digest"`
}

type ContextEngineeringResponseMetaV1 struct {
    ContractVersion string                     `json:"contract_version"`
    RequestID       string                     `json:"request_id"`
    Operation       ContextEngineeringOperationV1 `json:"operation"`
    RequestDigest   Digest                     `json:"request_digest"`
    ResultDigest    Digest                     `json:"result_digest"`
}
```

请求在任何deep-copy、canonical marshal或Evaluator调用前执行结构计数预检；nested refs包含Outcome内Tool/Task/User Evidence与所有顶层refs。超过调用方limit或hard max返回`limit_exceeded`且Response零值。所有输入和返回均deep-copy/no-alias。

typed error闭集固定为：`invalid_argument / limit_exceeded / conflict / expired / unauthorized / not_found / unavailable / unknown / canceled / deadline_exceeded / unsupported / internal_failure`。`context.Canceled`与`context.DeadlineExceeded`保留`errors.Is`；Evaluator的Unknown/Unavailable不得降格为确定性失败。除Prompt Validate的确定性`Valid=false`诊断外，任何error返回零Response。

strict codec必须递归拒绝unknown field、duplicate key、trailing document与错误null/presence；typed Go入口不声称检查JSON duplicate key。所有request/response canonical body置空自身digest后计算，domain/version/operation discriminator必须进入digest。

## 4. Prompt DTO

```go
type ValidatePromptAssetEngineeringRequestV1 struct {
    Meta   ContextEngineeringRequestMetaV1 `json:"meta"`
    Asset  PromptAssetV1                    `json:"asset"`
}

type ValidatePromptAssetEngineeringResponseV1 struct {
    Meta     ContextEngineeringResponseMetaV1 `json:"meta"`
    Valid    bool                             `json:"valid"`
    AssetRef *PromptAssetRefV1                `json:"asset_ref,omitempty"`
    Diagnostics []ContextEngineeringDiagnosticV1 `json:"diagnostics"`
}

type PreviewPromptCandidatesEngineeringRequestV1 struct {
    Meta    ContextEngineeringRequestMetaV1 `json:"meta"`
    Asset   PromptAssetV1                    `json:"asset"`
    Build   BuildPromptCandidatesRequestV1   `json:"build"`
}

type PreviewPromptCandidatesEngineeringResponseV1 struct {
    Meta       ContextEngineeringResponseMetaV1 `json:"meta"`
    Candidates PromptCandidateSetV1              `json:"candidates"`
}
```

Validate成功时`AssetRef`必须exact等于完整Asset digest；失败时`Valid=false`、`AssetRef=nil`且只返回有界结构诊断。Preview要求`Build.PromptAssetRef` exact等于输入Asset，Authority、RenderCompatibility与TTL通过后才投影；不写Prompt Store/lifecycle，不决定Frame Region、最终Role/顺序/cache placement，不返回Provider message。

## 5. Evaluator nominal、Input与Observation

```go
type ContextEvaluatorRefV1 struct {
    ID       string `json:"id"`
    Revision uint64 `json:"revision"`
    Digest   Digest `json:"digest"`
}

type ContextEvaluationInputV1 struct {
    ContractVersion    string                `json:"contract_version"`
    EvaluationID      string                `json:"evaluation_id"`
    EvaluatorRef       ContextEvaluatorRefV1 `json:"evaluator_ref"`
    OutcomeRefs        []FactRef             `json:"outcome_refs"`
    BaselineRecipeRef  FactRef               `json:"baseline_recipe_ref"`
    CandidateRecipeRef FactRef               `json:"candidate_recipe_ref"`
    PolicyRef          FactRef               `json:"policy_ref"`
    CheckedUnixNano    int64                 `json:"checked_unix_nano"`
    ExpiresUnixNano    int64                 `json:"expires_unix_nano"`
    InputDigest        Digest                `json:"input_digest"`
}

type ContextEvaluationObservationV1 struct {
    ContractVersion string                         `json:"contract_version"`
    EvaluatorRef    ContextEvaluatorRefV1          `json:"evaluator_ref"`
    InputDigest     Digest                         `json:"input_digest"`
    OutcomeRefs     []FactRef                      `json:"outcome_refs"`
    PolicyRef       FactRef                        `json:"policy_ref"`
    QualityScorePPM uint32                         `json:"quality_score_ppm"`
    EconomicScorePPM uint32                        `json:"economic_score_ppm"`
    RiskScorePPM    uint32                         `json:"risk_score_ppm"`
    Disposition     ContextEvaluationDispositionV1 `json:"disposition"`
    Evidence        []EvidenceRef                  `json:"evidence"`
    ObservedUnixNano int64                         `json:"observed_unix_nano"`
    ExpiresUnixNano int64                          `json:"expires_unix_nano"`
    ObservationDigest Digest                       `json:"observation_digest"`
}

type ContextEvaluatorV1 interface {
    RefV1() ContextEvaluatorRefV1
    EvaluateContextV1(context.Context, ContextEvaluationInputV1) (ContextEvaluationObservationV1, error)
}
```

Evaluator是可插拔计算Port，输出永远只是Observation。Context Owner只有在exact验证EvaluatorRef/InputDigest/OutcomeRefs/PolicyRef、分数范围、Evidence、时间窗口和ObservationDigest后，才能构造`ContextEvaluationFactV1`。本地规则、回放器或人工评分Adapter均可实现该Port；远程/model Judge必须由未来受治理Adapter在Operation Admission/Permit/Begin/actual-point Enforcement/Inspect后提供Observation，首切面不实现网络或Effect。

## 6. Evaluation与Feedback DTO

```go
type PrepareContextEvaluationRequestV1 struct {
    Meta               ContextEngineeringRequestMetaV1 `json:"meta"`
    EvaluationID       string                           `json:"evaluation_id"`
    EvaluatorRef       ContextEvaluatorRefV1            `json:"evaluator_ref"`
    Outcomes           []ContextOutcomeFactV1           `json:"outcomes"`
    BaselineRecipeRef  FactRef                          `json:"baseline_recipe_ref"`
    CandidateRecipeRef FactRef                          `json:"candidate_recipe_ref"`
    PolicyRef          FactRef                          `json:"policy_ref"`
    CheckedUnixNano    int64                            `json:"checked_unix_nano"`
    NotAfterUnixNano   int64                            `json:"not_after_unix_nano"`
}

type PrepareContextEvaluationResponseV1 struct {
    Meta  ContextEngineeringResponseMetaV1 `json:"meta"`
    Input ContextEvaluationInputV1          `json:"input"`
}

type AdmitContextEvaluationRequestV1 struct {
    Meta        ContextEngineeringRequestMetaV1 `json:"meta"`
    Preparation PrepareContextEvaluationRequestV1 `json:"preparation"`
    Input       ContextEvaluationInputV1          `json:"input"`
    Observation ContextEvaluationObservationV1    `json:"observation"`
}

type AdmitContextEvaluationResponseV1 struct {
    Meta          ContextEngineeringResponseMetaV1 `json:"meta"`
    Evaluation    ContextEvaluationFactV1           `json:"evaluation"`
    EvaluationRef FactRef                           `json:"evaluation_ref"`
}

type BuildContextFeedbackRequestV1 struct {
    Meta                ContextEngineeringRequestMetaV1 `json:"meta"`
    FeedbackCandidateID string                           `json:"feedback_candidate_id"`
    Outcomes            []ContextOutcomeFactV1           `json:"outcomes"`
    Evaluation          ContextEvaluationFactV1          `json:"evaluation"`
    ChangeDigest        Digest                           `json:"change_digest"`
    Evidence            []EvidenceRef                    `json:"evidence"`
    CreatedUnixNano     int64                            `json:"created_unix_nano"`
    NotAfterUnixNano    int64                            `json:"not_after_unix_nano"`
}

type BuildContextFeedbackResponseV1 struct {
    Meta        ContextEngineeringResponseMetaV1 `json:"meta"`
    Feedback    ContextFeedbackCandidateFactV1   `json:"feedback"`
    FeedbackRef FactRef                          `json:"feedback_ref"`
}
```

Prepare按完整Outcome对象重算exact refs并规范排序；每个Outcome必须绑定同一Policy，Recipe必须等于Baseline或Candidate，且两边至少各有一项。Baseline与Candidate必须是不同exact ref。Input expiry取请求上界与全部Outcome expiry最小值，`checked >= expires`拒绝。

Admit必须重新执行相同Preparation（S2逻辑）、得到逐字段相同Input，再核验Observation。Evaluation的Created取Observed，Expires取Input/Observation最小值；Observation不成为FactRef，也不能被普通FactRef type-pun。任一Outcome/Input/Policy/Evaluator/TTL漂移返回Conflict/Expired并产生零Evaluation。

Build Feedback重新核验完整Outcome集合与Evaluation exact digest，固定输出`State=evaluated`，BaseRecipe取Evaluation Baseline、OutcomeRefs/风险分数无损继承；Feedback expiry取请求、Evaluation和全部Outcome最小值。它不修改Recipe/Prompt current，不自动进入Review或published。

## 7. 调用顺序与Unknown

```text
Validate/Preview Prompt: request -> preflight -> validate exact asset -> pure projection -> response

Evaluation:
prepare(S1 exact outcomes) -> canonical Input
  -> Evaluator Observation
  -> admit(S2 exact outcomes + exact Input/Observation)
  -> ContextEvaluationFact
  -> build Feedback Candidate
```

Evaluator返回`unknown/unavailable/canceled/deadline`时零Evaluation/Feedback；是否重试由调用方按Evaluator自身Effect语义决定。远程Evaluator Begin后丢回包只能Inspect原外部attempt，不能直接重跑；Context Admit只接受已Inspect得到的exact Observation。Owner-local typed SDK自身无外部Effect、Settlement或Cleanup。

## 8. 文件与依赖DAG

已实现独占落点：

- `contract/evaluator.go`：nominal Evaluator ref、Input、Observation；
- `ports/evaluator.go`：Context-owned plugin Port；
- `kernel/evaluator.go`：Prepare/Admit/Feedback唯一核；
- `kernel/prompt.go`：抽取Store Service与SDK共用的纯Candidate projector，不复制算法；
- `sdk/engineering.go`、`engineering_codec.go`、`engineering_errors.go`：typed/strict入口、Envelope、limits、clone/cancel；
- `engineeringapi/service.go`：五typed方法与五JSON dispatch的transport-neutral薄封装；
- `cmd/context/main.go`：保留既有六个Offline命令，并增加五个Engineering stdin/stdout命令；
- `internal/testkit/engineering.go`及contract/kernel/sdk/blackbox/failure/conformance tests。

```text
sdk -> contract + ports + kernel
engineeringapi -> sdk
cmd/context -> engineeringapi + offlineapi + sdk
kernel -> contract + ports
ports -> contract
reference tests -> sdk/kernel + Context-only fakes
```

禁止导入Application/Harness/Model Invoker/Tool/Memory/Knowledge/Continuity实现或私有Port。API/CLI只能调用既有SDK入口：JSON dispatch必须先走对应strict decoder，typed与JSON结果/digest必须一致；CLI成功只向stdout输出一个canonical response，错误只向stderr输出typed error且不得泄露request/content/secret。首切面不新增API server、listener、production root、Provider/remote Evaluator Adapter或CLI写命令。

## 9. 模型专属预埋提示词的上游来源链

用户已确认策略：不同Model Family的预埋提示词优先借鉴其官方开源coding agent实现，而不是由Context Owner凭印象重新编造。该策略不代表直接复制任意文本，也不把厂商产品内部角色、Provider message或隐藏系统提示写入Context核心。

每次导入前必须形成可审核的上游来源记录，至少绑定：官方项目/仓库、不可变commit、文件路径与提取范围、许可证标识及归属要求、原文digest、规范化/删改/参数化变换的ID+revision+digest、生成的exact `ContentRef`、适用的Model Profile exact refs和Evidence。PromptAsset仍是Context Owner的权威资产；上游仓库内容只作为候选来源，不能继承Authority、Review、published或production资格。

七家来源与T3Code兼容范围已由用户确认，详见[模型专属 Coding Agent Prompt 官方上游审计](prompt-upstream-audit.md)。Codex、Gemini CLI、Kimi Code与Grok Build属于可进入候选变换的官方Coding Agent明文；Claude Agent SDK只提供`claude_code` preset引用而无正文；DeepSeek与MiniMax只作为模型template/profile证据；OpenCode仍只作B级工程对照。Context-owned exact DTO、canonical/closure规则、离线Verify与Model Invoker Port Delta见[Prompt Upstream Provenance V1](prompt-provenance.md)。网络抓取属于远程Source Effect；没有exact来源或license闭包时Fail Closed，不得把“来自官方”作为无证据标签。

## 10. 硬反例与验收门

- 普通FactRef/RecipeRef type-pun为PromptAssetRef或EvaluatorRef；
- Prompt Asset/Build ref、Authority、RenderCompatibility、TTL漂移仍返回Candidate；
- Outcome same-ID换revision/digest、乱序/重复、Policy或Recipe不闭合、只含单边样本仍Prepare成功；
- EvaluatorRef/InputDigest/OutcomeRefs/Policy/Evidence/ObservationDigest任一漂移仍Admit；
- cache usage、模型自评、一次成功或Observation直接升级为Evaluation Fact；
- Admit S2发生Outcome/TTL漂移后仍产生Evaluation；
- cancel/deadline/Unknown/Unavailable返回partial Evaluation/Feedback；
- Feedback自行改OutcomeRefs、risk、BaseRecipe或延长TTL；
- remote Judge未过治理Effect直接从SDK联网；
- API/CLI绕过strict codec、改变operation闭集/canonical digest，或错误时输出partial response/请求正文；
- 64并发同输入输出不确定、共享slice alias、strict codec递归duplicate key通过。

软件验收已执行：unit/whitebox/blackbox/fault/conformance、target100、race20、full ordinary/race/vet均PASS；gofmt、diff/import-boundary随本轮资产收口复核。该YES只覆盖Owner-local Go与本组件tests，不授权transport server、production root、远程Evaluator、Prompt发布、Capability、Harness Continuation或Turn推进。
