package sdk

import (
	"fmt"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
)

const OfflineSDKContractVersionV1 = "praxis.context.offline-sdk/v1"

type OfflineSDKOperationV1 string

const (
	OfflineValidateRecipeV1    OfflineSDKOperationV1 = "validate_recipe"
	OfflineCompareRecipesV1    OfflineSDKOperationV1 = "compare_recipes"
	OfflineCompileFrameV1      OfflineSDKOperationV1 = "compile_frame"
	OfflinePreviewFrameV1      OfflineSDKOperationV1 = "preview_frame"
	OfflineInspectFrameExactV1 OfflineSDKOperationV1 = "inspect_frame_exact"
	OfflineInspectCachePlanV1  OfflineSDKOperationV1 = "inspect_cache_plan"
)

const (
	hardMaxCandidatesV1             = uint32(512)
	hardMaxInputContentItemsV1      = uint32(1024)
	hardMaxInputContentItemBytesV1  = uint64(4 * 1024 * 1024)
	hardMaxInputRawBytesV1          = uint64(24 * 1024 * 1024)
	hardMaxGeneratedContentItemsV1  = uint32(4)
	hardMaxGeneratedRawBytesV1      = uint64(68 * 1024 * 1024)
	hardMaxCompileGeneratedBytesV1  = uint64(52 * 1024 * 1024)
	hardMaxOutputContentItemsV1     = uint32(1028)
	hardMaxOutputRawBytesV1         = uint64(100 * 1024 * 1024)
	hardMaxCompileOutputRawBytesV1  = uint64(76 * 1024 * 1024)
	hardMaxTotalTokensV1            = uint64(1024 * 1024)
	hardMaxDiagnosticsV1            = uint32(1024)
	hardMaxDiagnosticMessageBytesV1 = uint32(4096)
	hardMaxNonContentWireBytesV1    = uint64(4 * 1024 * 1024)
	hardWire48MiBV1                 = uint64(48 * 1024 * 1024)
	hardWire144MiBV1                = uint64(144 * 1024 * 1024)
)

type OfflineSDKLimitsV1 struct {
	MaxRecipes                uint32 `json:"max_recipes"`
	MaxCandidates             uint32 `json:"max_candidates"`
	MaxInputContentItems      uint32 `json:"max_input_content_items"`
	MaxInputContentItemBytes  uint64 `json:"max_input_content_item_bytes"`
	MaxInputRawBytes          uint64 `json:"max_input_raw_bytes"`
	MaxGeneratedContentItems  uint32 `json:"max_generated_content_items"`
	MaxGeneratedRawBytes      uint64 `json:"max_generated_raw_bytes"`
	MaxOutputContentItems     uint32 `json:"max_output_content_items"`
	MaxOutputRawBytes         uint64 `json:"max_output_raw_bytes"`
	MaxTotalTokens            uint64 `json:"max_total_tokens"`
	MaxDiagnostics            uint32 `json:"max_diagnostics"`
	MaxDiagnosticMessageBytes uint32 `json:"max_diagnostic_message_bytes"`
	MaxNonContentWireBytes    uint64 `json:"max_non_content_wire_bytes"`
	MaxWireRequestBytes       uint64 `json:"max_wire_request_bytes"`
	MaxWireResponseBytes      uint64 `json:"max_wire_response_bytes"`
}

func (l OfflineSDKLimitsV1) validate(operation OfflineSDKOperationV1) error {
	maxRecipes := uint32(1)
	if operation == OfflineCompareRecipesV1 {
		maxRecipes = 2
	}
	if l.MaxRecipes == 0 || l.MaxRecipes > maxRecipes ||
		l.MaxCandidates == 0 || l.MaxCandidates > hardMaxCandidatesV1 ||
		l.MaxInputContentItems == 0 || l.MaxInputContentItems > hardMaxInputContentItemsV1 ||
		l.MaxInputContentItemBytes == 0 || l.MaxInputContentItemBytes > hardMaxInputContentItemBytesV1 ||
		l.MaxInputRawBytes == 0 || l.MaxInputRawBytes > hardMaxInputRawBytesV1 ||
		l.MaxGeneratedContentItems == 0 || l.MaxGeneratedContentItems > hardMaxGeneratedContentItemsV1 ||
		l.MaxGeneratedRawBytes == 0 || l.MaxGeneratedRawBytes > hardMaxGeneratedRawBytesV1 ||
		l.MaxOutputContentItems == 0 || l.MaxOutputContentItems > hardMaxOutputContentItemsV1 ||
		l.MaxOutputRawBytes == 0 || l.MaxOutputRawBytes > hardMaxOutputRawBytesV1 ||
		l.MaxTotalTokens == 0 || l.MaxTotalTokens > hardMaxTotalTokensV1 ||
		l.MaxDiagnostics == 0 || l.MaxDiagnostics > hardMaxDiagnosticsV1 ||
		l.MaxDiagnosticMessageBytes == 0 || l.MaxDiagnosticMessageBytes > hardMaxDiagnosticMessageBytesV1 ||
		l.MaxNonContentWireBytes == 0 || l.MaxNonContentWireBytes > hardMaxNonContentWireBytesV1 ||
		l.MaxWireRequestBytes == 0 || l.MaxWireResponseBytes == 0 {
		return fmt.Errorf("limits outside v1 hard maxima")
	}
	reqCap, respCap, ok := wireCapsV1(operation)
	if !ok {
		return fmt.Errorf("unsupported operation")
	}
	if l.MaxWireRequestBytes > reqCap || l.MaxWireResponseBytes > respCap {
		return fmt.Errorf("wire limits outside operation hard caps")
	}
	return nil
}

func wireCapsV1(operation OfflineSDKOperationV1) (uint64, uint64, bool) {
	switch operation {
	case OfflineValidateRecipeV1, OfflineCompareRecipesV1, OfflineInspectCachePlanV1:
		return hardWire48MiBV1, hardWire48MiBV1, true
	case OfflineCompileFrameV1:
		return hardWire48MiBV1, hardWire144MiBV1, true
	case OfflinePreviewFrameV1, OfflineInspectFrameExactV1:
		return hardWire144MiBV1, hardWire48MiBV1, true
	default:
		return 0, 0, false
	}
}

// WireCapsV1 returns the frozen hard request and response wire caps for an
// Offline SDK operation. Callers must still honor the smaller limits sealed in
// each request.
func WireCapsV1(operation OfflineSDKOperationV1) (requestBytes uint64, responseBytes uint64, ok bool) {
	return wireCapsV1(operation)
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

type OfflineContentBundleV1 struct {
	items            []OfflineContentItemV1
	contentSetDigest contract.Digest
}

type ValidateRecipeRequestV1 struct {
	Meta       OfflineRequestMetaV1        `json:"meta"`
	Recipe     contract.ContextRecipe      `json:"recipe"`
	Candidates []contract.ContextCandidate `json:"candidates"`
}

type ValidateRecipeResponseV1 struct {
	Meta          OfflineResponseMetaV1 `json:"meta"`
	Valid         bool                  `json:"valid"`
	RecipeRef     *contract.FactRef     `json:"recipe_ref,omitempty"`
	CandidateRefs []contract.FactRef    `json:"candidate_refs"`
	Diagnostics   []OfflineDiagnosticV1 `json:"diagnostics"`
	ReportDigest  contract.Digest       `json:"report_digest"`
	limits        OfflineSDKLimitsV1
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
	ParentFrame       *contract.FactRef           `json:"parent_frame,omitempty"`
	CreatedUnixNano   int64                       `json:"created_unix_nano"`
	ExpiresUnixNano   int64                       `json:"expires_unix_nano"`
	InputBundle       OfflineContentBundleV1      `json:"input_bundle"`
}

type CompiledBundleV1 struct {
	Manifest              contract.ContextManifest `json:"manifest"`
	Frame                 contract.ContextFrame    `json:"frame"`
	ContentBundle         OfflineContentBundleV1   `json:"content_bundle"`
	ResidualCandidateRefs []contract.FactRef       `json:"residual_candidate_refs"`
	Authoritative         bool                     `json:"authoritative"`
	CompileDigest         contract.Digest          `json:"compile_digest"`
}

type CompileFrameResponseV1 struct {
	Meta        OfflineResponseMetaV1 `json:"meta"`
	Compiled    CompiledBundleV1      `json:"compiled"`
	Diagnostics []OfflineDiagnosticV1 `json:"diagnostics"`
	limits      OfflineSDKLimitsV1
}

type PreviewFrameRequestV1 struct {
	Meta                  OfflineRequestMetaV1 `json:"meta"`
	Compiled              CompiledBundleV1     `json:"compiled"`
	ExpectedCompileDigest contract.Digest      `json:"expected_compile_digest"`
	CheckedUnixNano       int64                `json:"checked_unix_nano"`
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
	Meta               OfflineResponseMetaV1        `json:"meta"`
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
	limits             OfflineSDKLimitsV1
}

type InspectFrameExactRequestV1 struct {
	Meta                  OfflineRequestMetaV1     `json:"meta"`
	Manifest              contract.ContextManifest `json:"manifest"`
	Frame                 contract.ContextFrame    `json:"frame"`
	ContentBundle         OfflineContentBundleV1   `json:"content_bundle"`
	ExpectedManifestRef   contract.FactRef         `json:"expected_manifest_ref"`
	ExpectedFrameRef      contract.FactRef         `json:"expected_frame_ref"`
	ExpectedCompileDigest contract.Digest          `json:"expected_compile_digest"`
	CheckedUnixNano       int64                    `json:"checked_unix_nano"`
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
	limits           OfflineSDKLimitsV1
}
