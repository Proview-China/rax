package sdk

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/kernel"
	contextports "github.com/Proview-China/rax/ExecutionRuntime/context-engine/ports"
)

const ContextEngineeringSDKContractVersionV1 = "praxis.context.engineering-sdk/v1"

const (
	hardEngineeringPromptFragmentsV1 = uint32(64)
	hardEngineeringOutcomesV1        = uint32(64)
	hardEngineeringNestedRefsV1      = uint32(32768)
	hardEngineeringEvidenceRefsV1    = uint32(512)
	hardEngineeringDiagnosticsV1     = uint32(1024)
	hardEngineeringCanonicalBytesV1  = uint64(32 * 1024 * 1024)
	hardEngineeringWireBytesV1       = uint64(48 * 1024 * 1024)
)

type ContextEngineeringOperationV1 string

const (
	EngineeringValidatePromptAssetV1 ContextEngineeringOperationV1 = "validate_prompt_asset"
	EngineeringPreviewPromptV1       ContextEngineeringOperationV1 = "preview_prompt_candidates"
	EngineeringPrepareEvaluationV1   ContextEngineeringOperationV1 = "prepare_context_evaluation"
	EngineeringAdmitEvaluationV1     ContextEngineeringOperationV1 = "admit_context_evaluation"
	EngineeringBuildFeedbackV1       ContextEngineeringOperationV1 = "build_feedback_candidate"
)

func (op ContextEngineeringOperationV1) Validate() error {
	switch op {
	case EngineeringValidatePromptAssetV1, EngineeringPreviewPromptV1, EngineeringPrepareEvaluationV1, EngineeringAdmitEvaluationV1, EngineeringBuildFeedbackV1:
		return nil
	default:
		return fmt.Errorf("%w: context engineering operation", contract.ErrUnsupported)
	}
}

type ContextEngineeringLimitsV1 struct {
	MaxPromptFragments uint32 `json:"max_prompt_fragments"`
	MaxOutcomes        uint32 `json:"max_outcomes"`
	MaxNestedRefs      uint32 `json:"max_nested_refs"`
	MaxEvidenceRefs    uint32 `json:"max_evidence_refs"`
	MaxDiagnostics     uint32 `json:"max_diagnostics"`
	MaxCanonicalBytes  uint64 `json:"max_canonical_bytes"`
	MaxWireBytes       uint64 `json:"max_wire_bytes"`
}

func DefaultContextEngineeringLimitsV1() ContextEngineeringLimitsV1 {
	return ContextEngineeringLimitsV1{
		MaxPromptFragments: hardEngineeringPromptFragmentsV1, MaxOutcomes: hardEngineeringOutcomesV1,
		MaxNestedRefs: hardEngineeringNestedRefsV1, MaxEvidenceRefs: hardEngineeringEvidenceRefsV1,
		MaxDiagnostics: hardEngineeringDiagnosticsV1, MaxCanonicalBytes: hardEngineeringCanonicalBytesV1,
		MaxWireBytes: hardEngineeringWireBytesV1,
	}
}

func (v ContextEngineeringLimitsV1) Validate() error {
	if v.MaxPromptFragments == 0 || v.MaxPromptFragments > hardEngineeringPromptFragmentsV1 || v.MaxOutcomes == 0 || v.MaxOutcomes > hardEngineeringOutcomesV1 || v.MaxNestedRefs == 0 || v.MaxNestedRefs > hardEngineeringNestedRefsV1 || v.MaxEvidenceRefs == 0 || v.MaxEvidenceRefs > hardEngineeringEvidenceRefsV1 || v.MaxDiagnostics == 0 || v.MaxDiagnostics > hardEngineeringDiagnosticsV1 || v.MaxCanonicalBytes == 0 || v.MaxCanonicalBytes > hardEngineeringCanonicalBytesV1 || v.MaxWireBytes == 0 || v.MaxWireBytes > hardEngineeringWireBytesV1 {
		return fmt.Errorf("%w: context engineering limits", contract.ErrLimitExceeded)
	}
	return nil
}

type ContextEngineeringRequestMetaV1 struct {
	ContractVersion string                        `json:"contract_version"`
	RequestID       string                        `json:"request_id"`
	Operation       ContextEngineeringOperationV1 `json:"operation"`
	Limits          ContextEngineeringLimitsV1    `json:"limits"`
	RequestDigest   contract.Digest               `json:"request_digest"`
}

type ContextEngineeringResponseMetaV1 struct {
	ContractVersion string                        `json:"contract_version"`
	RequestID       string                        `json:"request_id"`
	Operation       ContextEngineeringOperationV1 `json:"operation"`
	RequestDigest   contract.Digest               `json:"request_digest"`
	ResultDigest    contract.Digest               `json:"result_digest"`
}

type ContextEngineeringDiagnosticV1 struct {
	Code       string `json:"code"`
	Severity   string `json:"severity"`
	ObjectKind string `json:"object_kind"`
	ObjectID   string `json:"object_id"`
	FieldPath  string `json:"field_path"`
	Message    string `json:"message"`
}

type ValidatePromptAssetEngineeringRequestV1 struct {
	Meta  ContextEngineeringRequestMetaV1 `json:"meta"`
	Asset contract.PromptAssetV1          `json:"asset"`
}

type ValidatePromptAssetEngineeringResponseV1 struct {
	Meta        ContextEngineeringResponseMetaV1 `json:"meta"`
	Valid       bool                             `json:"valid"`
	AssetRef    *contract.PromptAssetRefV1       `json:"asset_ref,omitempty"`
	Diagnostics []ContextEngineeringDiagnosticV1 `json:"diagnostics"`
	limits      ContextEngineeringLimitsV1
}

type PreviewPromptCandidatesEngineeringRequestV1 struct {
	Meta  ContextEngineeringRequestMetaV1         `json:"meta"`
	Asset contract.PromptAssetV1                  `json:"asset"`
	Build contract.BuildPromptCandidatesRequestV1 `json:"build"`
}

type PreviewPromptCandidatesEngineeringResponseV1 struct {
	Meta       ContextEngineeringResponseMetaV1 `json:"meta"`
	Candidates contract.PromptCandidateSetV1    `json:"candidates"`
	limits     ContextEngineeringLimitsV1
}

type PrepareContextEvaluationRequestV1 struct {
	Meta               ContextEngineeringRequestMetaV1 `json:"meta"`
	EvaluationID       string                          `json:"evaluation_id"`
	EvaluatorRef       contract.ContextEvaluatorRefV1  `json:"evaluator_ref"`
	Outcomes           []contract.ContextOutcomeFactV1 `json:"outcomes"`
	BaselineRecipeRef  contract.FactRef                `json:"baseline_recipe_ref"`
	CandidateRecipeRef contract.FactRef                `json:"candidate_recipe_ref"`
	PolicyRef          contract.FactRef                `json:"policy_ref"`
	CheckedUnixNano    int64                           `json:"checked_unix_nano"`
	NotAfterUnixNano   int64                           `json:"not_after_unix_nano"`
}

type PrepareContextEvaluationResponseV1 struct {
	Meta   ContextEngineeringResponseMetaV1  `json:"meta"`
	Input  contract.ContextEvaluationInputV1 `json:"input"`
	limits ContextEngineeringLimitsV1
}

type AdmitContextEvaluationRequestV1 struct {
	Meta        ContextEngineeringRequestMetaV1         `json:"meta"`
	Preparation PrepareContextEvaluationRequestV1       `json:"preparation"`
	Input       contract.ContextEvaluationInputV1       `json:"input"`
	Observation contract.ContextEvaluationObservationV1 `json:"observation"`
}

type AdmitContextEvaluationResponseV1 struct {
	Meta          ContextEngineeringResponseMetaV1 `json:"meta"`
	Evaluation    contract.ContextEvaluationFactV1 `json:"evaluation"`
	EvaluationRef contract.FactRef                 `json:"evaluation_ref"`
	limits        ContextEngineeringLimitsV1
}

type BuildContextFeedbackRequestV1 struct {
	Meta                ContextEngineeringRequestMetaV1  `json:"meta"`
	FeedbackCandidateID string                           `json:"feedback_candidate_id"`
	Outcomes            []contract.ContextOutcomeFactV1  `json:"outcomes"`
	Evaluation          contract.ContextEvaluationFactV1 `json:"evaluation"`
	ChangeDigest        contract.Digest                  `json:"change_digest"`
	Evidence            []contract.EvidenceRef           `json:"evidence"`
	CreatedUnixNano     int64                            `json:"created_unix_nano"`
	NotAfterUnixNano    int64                            `json:"not_after_unix_nano"`
}

type BuildContextFeedbackResponseV1 struct {
	Meta        ContextEngineeringResponseMetaV1        `json:"meta"`
	Feedback    contract.ContextFeedbackCandidateFactV1 `json:"feedback"`
	FeedbackRef contract.FactRef                        `json:"feedback_ref"`
	limits      ContextEngineeringLimitsV1
}

func SealValidatePromptAssetEngineeringRequestV1(ctx context.Context, request ValidatePromptAssetEngineeringRequestV1) (ValidatePromptAssetEngineeringRequestV1, error) {
	return sealEngineeringRequestV1(ctx, EngineeringValidatePromptAssetV1, request, func(v *ValidatePromptAssetEngineeringRequestV1) *ContextEngineeringRequestMetaV1 { return &v.Meta })
}

func SealPreviewPromptCandidatesEngineeringRequestV1(ctx context.Context, request PreviewPromptCandidatesEngineeringRequestV1) (PreviewPromptCandidatesEngineeringRequestV1, error) {
	return sealEngineeringRequestV1(ctx, EngineeringPreviewPromptV1, request, func(v *PreviewPromptCandidatesEngineeringRequestV1) *ContextEngineeringRequestMetaV1 { return &v.Meta })
}

func SealPrepareContextEvaluationRequestV1(ctx context.Context, request PrepareContextEvaluationRequestV1) (PrepareContextEvaluationRequestV1, error) {
	return sealEngineeringRequestV1(ctx, EngineeringPrepareEvaluationV1, request, func(v *PrepareContextEvaluationRequestV1) *ContextEngineeringRequestMetaV1 { return &v.Meta })
}

func SealAdmitContextEvaluationRequestV1(ctx context.Context, request AdmitContextEvaluationRequestV1) (AdmitContextEvaluationRequestV1, error) {
	return sealEngineeringRequestV1(ctx, EngineeringAdmitEvaluationV1, request, func(v *AdmitContextEvaluationRequestV1) *ContextEngineeringRequestMetaV1 { return &v.Meta })
}

func SealBuildContextFeedbackRequestV1(ctx context.Context, request BuildContextFeedbackRequestV1) (BuildContextFeedbackRequestV1, error) {
	return sealEngineeringRequestV1(ctx, EngineeringBuildFeedbackV1, request, func(v *BuildContextFeedbackRequestV1) *ContextEngineeringRequestMetaV1 { return &v.Meta })
}

func ValidatePromptAssetEngineeringV1(ctx context.Context, request ValidatePromptAssetEngineeringRequestV1) (ValidatePromptAssetEngineeringResponseV1, error) {
	const op = EngineeringValidatePromptAssetV1
	if err := validateEngineeringRequestV1(ctx, op, request, request.Meta); err != nil {
		return ValidatePromptAssetEngineeringResponseV1{}, err
	}
	response := ValidatePromptAssetEngineeringResponseV1{Meta: engineeringResponseMetaV1(request.Meta), Diagnostics: []ContextEngineeringDiagnosticV1{}}
	ref, err := request.Asset.RefV1()
	if err != nil {
		response.Diagnostics = []ContextEngineeringDiagnosticV1{{Code: "invalid_prompt_asset", Severity: "error", ObjectKind: "prompt_asset", ObjectID: request.Asset.ID, FieldPath: "asset", Message: "prompt asset contract is invalid"}}
	} else {
		response.Valid = true
		response.AssetRef = &ref
	}
	return sealEngineeringResponseV1(ctx, op, response, request.Meta.Limits)
}

func PreviewPromptCandidatesEngineeringV1(ctx context.Context, request PreviewPromptCandidatesEngineeringRequestV1) (PreviewPromptCandidatesEngineeringResponseV1, error) {
	const op = EngineeringPreviewPromptV1
	if err := validateEngineeringRequestV1(ctx, op, request, request.Meta); err != nil {
		return PreviewPromptCandidatesEngineeringResponseV1{}, err
	}
	set, err := kernel.ProjectPromptCandidatesV1(ctx, request.Asset, request.Build)
	if err != nil {
		return PreviewPromptCandidatesEngineeringResponseV1{}, mapEngineeringErrorV1(op, "build", err)
	}
	response := PreviewPromptCandidatesEngineeringResponseV1{Meta: engineeringResponseMetaV1(request.Meta), Candidates: set}
	return sealEngineeringResponseV1(ctx, op, response, request.Meta.Limits)
}

func PrepareContextEvaluationV1(ctx context.Context, request PrepareContextEvaluationRequestV1) (PrepareContextEvaluationResponseV1, error) {
	const op = EngineeringPrepareEvaluationV1
	if err := validateEngineeringRequestV1(ctx, op, request, request.Meta); err != nil {
		return PrepareContextEvaluationResponseV1{}, err
	}
	input, err := kernel.PrepareContextEvaluationInputV1(ctx, request.EvaluationID, request.EvaluatorRef, request.Outcomes, request.BaselineRecipeRef, request.CandidateRecipeRef, request.PolicyRef, request.CheckedUnixNano, request.NotAfterUnixNano)
	if err != nil {
		return PrepareContextEvaluationResponseV1{}, mapEngineeringErrorV1(op, "outcomes", err)
	}
	response := PrepareContextEvaluationResponseV1{Meta: engineeringResponseMetaV1(request.Meta), Input: input}
	return sealEngineeringResponseV1(ctx, op, response, request.Meta.Limits)
}

func AdmitContextEvaluationV1(ctx context.Context, request AdmitContextEvaluationRequestV1) (AdmitContextEvaluationResponseV1, error) {
	const op = EngineeringAdmitEvaluationV1
	if err := validateEngineeringRequestV1(ctx, op, request, request.Meta); err != nil {
		return AdmitContextEvaluationResponseV1{}, err
	}
	if err := validateEngineeringRequestV1(ctx, EngineeringPrepareEvaluationV1, request.Preparation, request.Preparation.Meta); err != nil {
		return AdmitContextEvaluationResponseV1{}, mapEngineeringErrorV1(op, "preparation", err)
	}
	prepared, err := kernel.PrepareContextEvaluationInputV1(ctx, request.Preparation.EvaluationID, request.Preparation.EvaluatorRef, request.Preparation.Outcomes, request.Preparation.BaselineRecipeRef, request.Preparation.CandidateRecipeRef, request.Preparation.PolicyRef, request.Preparation.CheckedUnixNano, request.Preparation.NotAfterUnixNano)
	if err != nil {
		return AdmitContextEvaluationResponseV1{}, mapEngineeringErrorV1(op, "preparation", err)
	}
	if prepared.InputDigest != request.Input.InputDigest {
		return AdmitContextEvaluationResponseV1{}, mapEngineeringErrorV1(op, "input", fmt.Errorf("%w: prepared input drift", contract.ErrConflict))
	}
	evaluation, ref, err := kernel.AdmitContextEvaluationObservationV1(ctx, request.Preparation.Outcomes, request.Input, request.Observation)
	if err != nil {
		return AdmitContextEvaluationResponseV1{}, mapEngineeringErrorV1(op, "observation", err)
	}
	response := AdmitContextEvaluationResponseV1{Meta: engineeringResponseMetaV1(request.Meta), Evaluation: evaluation, EvaluationRef: ref}
	return sealEngineeringResponseV1(ctx, op, response, request.Meta.Limits)
}

func BuildContextFeedbackEngineeringV1(ctx context.Context, request BuildContextFeedbackRequestV1) (BuildContextFeedbackResponseV1, error) {
	const op = EngineeringBuildFeedbackV1
	if err := validateEngineeringRequestV1(ctx, op, request, request.Meta); err != nil {
		return BuildContextFeedbackResponseV1{}, err
	}
	feedback, ref, err := kernel.BuildContextFeedbackCandidateV1(ctx, request.FeedbackCandidateID, request.Outcomes, request.Evaluation, request.ChangeDigest, request.Evidence, request.CreatedUnixNano, request.NotAfterUnixNano)
	if err != nil {
		return BuildContextFeedbackResponseV1{}, mapEngineeringErrorV1(op, "feedback", err)
	}
	response := BuildContextFeedbackResponseV1{Meta: engineeringResponseMetaV1(request.Meta), Feedback: feedback, FeedbackRef: ref}
	return sealEngineeringResponseV1(ctx, op, response, request.Meta.Limits)
}

// EvaluateContextWithV1 is a local developer convenience. A remote evaluator
// still requires an external governed adapter; this function does not provide
// an Effect, Permit, Settlement, retry, or production execution boundary.
func EvaluateContextWithV1(ctx context.Context, evaluator contextports.ContextEvaluatorV1, preparation PrepareContextEvaluationRequestV1, admitMeta ContextEngineeringRequestMetaV1) (AdmitContextEvaluationResponseV1, error) {
	if evaluator == nil {
		return AdmitContextEvaluationResponseV1{}, mapEngineeringErrorV1(EngineeringAdmitEvaluationV1, "evaluator", contract.ErrInvalid)
	}
	prepared, err := PrepareContextEvaluationV1(ctx, preparation)
	if err != nil {
		return AdmitContextEvaluationResponseV1{}, err
	}
	if evaluator.RefV1() != prepared.Input.EvaluatorRef {
		return AdmitContextEvaluationResponseV1{}, mapEngineeringErrorV1(EngineeringAdmitEvaluationV1, "evaluator", contract.ErrConflict)
	}
	observation, err := evaluator.EvaluateContextV1(ctx, prepared.Input)
	if err != nil {
		return AdmitContextEvaluationResponseV1{}, mapEngineeringErrorV1(EngineeringAdmitEvaluationV1, "evaluator", err)
	}
	request, err := SealAdmitContextEvaluationRequestV1(ctx, AdmitContextEvaluationRequestV1{Meta: admitMeta, Preparation: preparation, Input: prepared.Input, Observation: observation})
	if err != nil {
		return AdmitContextEvaluationResponseV1{}, err
	}
	return AdmitContextEvaluationV1(ctx, request)
}

func engineeringResponseMetaV1(request ContextEngineeringRequestMetaV1) ContextEngineeringResponseMetaV1 {
	return ContextEngineeringResponseMetaV1{ContractVersion: ContextEngineeringSDKContractVersionV1, RequestID: request.RequestID, Operation: request.Operation, RequestDigest: request.RequestDigest}
}

func sealEngineeringRequestV1[T any](ctx context.Context, op ContextEngineeringOperationV1, request T, meta func(*T) *ContextEngineeringRequestMetaV1) (T, error) {
	var zero T
	if err := engineeringContextErrV1(ctx); err != nil {
		return zero, mapEngineeringErrorV1(op, "context", err)
	}
	m := meta(&request)
	m.ContractVersion = ContextEngineeringSDKContractVersionV1
	m.Operation = op
	if err := validateEngineeringMetaBaseV1(*m, op); err != nil {
		return zero, err
	}
	prior := m.RequestDigest
	m.RequestDigest = ""
	if err := engineeringStructuralPreflightV1(op, request, m.Limits); err != nil {
		return zero, err
	}
	digest, err := engineeringCanonicalDigestV1(ctx, "request:"+string(op), request, m.Limits.MaxCanonicalBytes)
	if err != nil {
		return zero, mapEngineeringErrorV1(op, "meta.request_digest", err)
	}
	if prior != "" && prior != digest {
		return zero, mapEngineeringErrorV1(op, "meta.request_digest", contract.ErrConflict)
	}
	m.RequestDigest = digest
	cloned, err := cloneEngineeringValueV1(ctx, request, m.Limits.MaxCanonicalBytes)
	if err != nil {
		return zero, mapEngineeringErrorV1(op, "request", err)
	}
	return cloned, nil
}

func validateEngineeringRequestV1(ctx context.Context, op ContextEngineeringOperationV1, request any, meta ContextEngineeringRequestMetaV1) error {
	if err := engineeringContextErrV1(ctx); err != nil {
		return mapEngineeringErrorV1(op, "context", err)
	}
	if err := validateEngineeringMetaV1(meta, op); err != nil {
		return err
	}
	if err := engineeringStructuralPreflightV1(op, request, meta.Limits); err != nil {
		return err
	}
	digest, err := engineeringRequestDigestV1(ctx, op, request, meta.Limits.MaxCanonicalBytes)
	if err != nil {
		return mapEngineeringErrorV1(op, "meta.request_digest", err)
	}
	if digest != meta.RequestDigest {
		return mapEngineeringErrorV1(op, "meta.request_digest", contract.ErrConflict)
	}
	return nil
}

func engineeringRequestDigestV1(ctx context.Context, op ContextEngineeringOperationV1, request any, max uint64) (contract.Digest, error) {
	switch value := request.(type) {
	case ValidatePromptAssetEngineeringRequestV1:
		value.Meta.RequestDigest = ""
		return engineeringCanonicalDigestV1(ctx, "request:"+string(op), value, max)
	case PreviewPromptCandidatesEngineeringRequestV1:
		value.Meta.RequestDigest = ""
		return engineeringCanonicalDigestV1(ctx, "request:"+string(op), value, max)
	case PrepareContextEvaluationRequestV1:
		value.Meta.RequestDigest = ""
		return engineeringCanonicalDigestV1(ctx, "request:"+string(op), value, max)
	case AdmitContextEvaluationRequestV1:
		value.Meta.RequestDigest = ""
		return engineeringCanonicalDigestV1(ctx, "request:"+string(op), value, max)
	case BuildContextFeedbackRequestV1:
		value.Meta.RequestDigest = ""
		return engineeringCanonicalDigestV1(ctx, "request:"+string(op), value, max)
	default:
		return "", contract.ErrInvalid
	}
}

func sealEngineeringResponseV1[T any](ctx context.Context, op ContextEngineeringOperationV1, response T, limits ContextEngineeringLimitsV1) (T, error) {
	var zero T
	digest, err := engineeringCanonicalDigestV1(ctx, "response:"+string(op), response, limits.MaxCanonicalBytes)
	if err != nil {
		return zero, mapEngineeringErrorV1(op, "meta.result_digest", err)
	}
	if !setEngineeringResponseDigestV1(&response, digest) {
		return zero, mapEngineeringErrorV1(op, "response", contract.ErrInvalid)
	}
	cloned, err := cloneEngineeringValueV1(ctx, response, limits.MaxCanonicalBytes)
	if err != nil {
		return zero, mapEngineeringErrorV1(op, "response", err)
	}
	setEngineeringResponseLimitsV1(&cloned, limits)
	return cloned, nil
}

func setEngineeringResponseDigestV1[T any](response *T, digest contract.Digest) bool {
	switch value := any(response).(type) {
	case *ValidatePromptAssetEngineeringResponseV1:
		value.Meta.ResultDigest = digest
	case *PreviewPromptCandidatesEngineeringResponseV1:
		value.Meta.ResultDigest = digest
	case *PrepareContextEvaluationResponseV1:
		value.Meta.ResultDigest = digest
	case *AdmitContextEvaluationResponseV1:
		value.Meta.ResultDigest = digest
	case *BuildContextFeedbackResponseV1:
		value.Meta.ResultDigest = digest
	default:
		return false
	}
	return true
}

func setEngineeringResponseLimitsV1[T any](response *T, limits ContextEngineeringLimitsV1) {
	switch value := any(response).(type) {
	case *ValidatePromptAssetEngineeringResponseV1:
		value.limits = limits
	case *PreviewPromptCandidatesEngineeringResponseV1:
		value.limits = limits
	case *PrepareContextEvaluationResponseV1:
		value.limits = limits
	case *AdmitContextEvaluationResponseV1:
		value.limits = limits
	case *BuildContextFeedbackResponseV1:
		value.limits = limits
	}
}

func validateEngineeringMetaBaseV1(meta ContextEngineeringRequestMetaV1, op ContextEngineeringOperationV1) error {
	if meta.ContractVersion != ContextEngineeringSDKContractVersionV1 || !engineeringIDV1(meta.RequestID) || meta.Operation != op {
		return engineeringErrorV1(EngineeringErrorInvalidArgumentV1, op, "meta", "invalid request metadata", contract.ErrInvalid)
	}
	if err := op.Validate(); err != nil {
		return mapEngineeringErrorV1(op, "meta.operation", err)
	}
	if err := meta.Limits.Validate(); err != nil {
		return mapEngineeringErrorV1(op, "meta.limits", err)
	}
	return nil
}

func validateEngineeringMetaV1(meta ContextEngineeringRequestMetaV1, op ContextEngineeringOperationV1) error {
	if err := validateEngineeringMetaBaseV1(meta, op); err != nil {
		return err
	}
	if meta.RequestDigest.Validate() != nil {
		return mapEngineeringErrorV1(op, "meta.request_digest", contract.ErrInvalid)
	}
	return nil
}

func engineeringStructuralPreflightV1(op ContextEngineeringOperationV1, request any, limits ContextEngineeringLimitsV1) error {
	var promptFragments, outcomes, nestedRefs, evidence int
	switch value := request.(type) {
	case ValidatePromptAssetEngineeringRequestV1:
		promptFragments, evidence = len(value.Asset.Fragments), len(value.Asset.Evidence)
		nestedRefs = len(value.Asset.RenderCompatibility) + evidence
	case PreviewPromptCandidatesEngineeringRequestV1:
		promptFragments, evidence = len(value.Asset.Fragments), len(value.Asset.Evidence)
		nestedRefs = len(value.Asset.RenderCompatibility) + evidence + 2
		if promptFragments == 0 {
			return mapEngineeringErrorV1(op, "asset.fragments", contract.ErrInvalid)
		}
	case PrepareContextEvaluationRequestV1:
		outcomes = len(value.Outcomes)
		nestedRefs, evidence = engineeringOutcomeCountsV1(value.Outcomes)
		nestedRefs += 4
		if outcomes == 0 {
			return mapEngineeringErrorV1(op, "outcomes", contract.ErrInvalid)
		}
	case AdmitContextEvaluationRequestV1:
		outcomes = len(value.Preparation.Outcomes)
		nestedRefs, evidence = engineeringOutcomeCountsV1(value.Preparation.Outcomes)
		nestedRefs += len(value.Input.OutcomeRefs) + len(value.Observation.OutcomeRefs) + len(value.Observation.Evidence) + 12
		evidence += len(value.Observation.Evidence)
		if outcomes == 0 || len(value.Input.OutcomeRefs) == 0 || len(value.Observation.OutcomeRefs) == 0 || len(value.Observation.Evidence) == 0 {
			return mapEngineeringErrorV1(op, "evaluation", contract.ErrInvalid)
		}
	case BuildContextFeedbackRequestV1:
		outcomes = len(value.Outcomes)
		nestedRefs, evidence = engineeringOutcomeCountsV1(value.Outcomes)
		nestedRefs += len(value.Evaluation.OutcomeRefs) + len(value.Evaluation.Evidence) + len(value.Evidence) + 7
		evidence += len(value.Evaluation.Evidence) + len(value.Evidence)
		if outcomes == 0 || len(value.Evaluation.OutcomeRefs) == 0 || len(value.Evidence) == 0 {
			return mapEngineeringErrorV1(op, "feedback", contract.ErrInvalid)
		}
	default:
		return mapEngineeringErrorV1(op, "request", contract.ErrInvalid)
	}
	if promptFragments > int(limits.MaxPromptFragments) || outcomes > int(limits.MaxOutcomes) || nestedRefs > int(limits.MaxNestedRefs) || evidence > int(limits.MaxEvidenceRefs) {
		return mapEngineeringErrorV1(op, "request", contract.ErrLimitExceeded)
	}
	return nil
}

func engineeringOutcomeCountsV1(outcomes []contract.ContextOutcomeFactV1) (refs int, evidence int) {
	for _, outcome := range outcomes {
		refs += 8 + len(outcome.ToolActionRefs) + len(outcome.TaskEvidenceRefs)
		if outcome.ModelSettlementRef != nil {
			refs++
		}
		if outcome.ActualInjectionManifestRef != nil {
			refs++
		}
		evidence += len(outcome.UserCorrectionEvidence)
		refs += len(outcome.UserCorrectionEvidence)
	}
	return refs, evidence
}

func engineeringCanonicalDigestV1(ctx context.Context, discriminator string, body any, max uint64) (contract.Digest, error) {
	writer := &engineeringHashWriterV1{ctx: ctx, hash: sha256.New(), max: max}
	err := writeJSONContextV1(ctx, writer, struct {
		Domain        string `json:"domain"`
		Version       string `json:"version"`
		Discriminator string `json:"discriminator"`
		Body          any    `json:"body"`
	}{Domain: "praxis.context.engineering-sdk", Version: "v1", Discriminator: discriminator, Body: body})
	if err != nil {
		return "", err
	}
	return contract.Digest("sha256:" + hex.EncodeToString(writer.hash.Sum(nil))), nil
}

type engineeringHashWriterV1 struct {
	ctx   context.Context
	hash  hash.Hash
	max   uint64
	count uint64
}

func (w *engineeringHashWriterV1) Write(payload []byte) (int, error) {
	if err := w.ctx.Err(); err != nil {
		return 0, err
	}
	if uint64(len(payload)) > w.max || w.count > w.max-uint64(len(payload)) {
		return 0, contract.ErrLimitExceeded
	}
	w.count += uint64(len(payload))
	return w.hash.Write(payload)
}

func cloneEngineeringValueV1[T any](ctx context.Context, value T, max uint64) (T, error) {
	var zero T
	buffer := &boundedCodecBufferV1{ctx: ctx, max: max}
	if err := buffer.writeJSON(value); err != nil {
		return zero, err
	}
	decoder := json.NewDecoder(&contextChunkReaderV1{ctx: ctx, reader: bytes.NewReader(buffer.buf.Bytes())})
	if err := decoder.Decode(&zero); err != nil {
		return zero, err
	}
	return zero, ctx.Err()
}

func engineeringIDV1(value string) bool {
	if strings.TrimSpace(value) != value || value == "" || len(value) > 256 {
		return false
	}
	for _, r := range value {
		if r < 0x21 || r > 0x7e {
			return false
		}
	}
	return true
}

func engineeringContextErrV1(ctx context.Context) error {
	if ctx == nil {
		return contract.ErrInvalid
	}
	return ctx.Err()
}
