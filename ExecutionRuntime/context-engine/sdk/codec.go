package sdk

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/kernel"
)

const (
	rawChunkBytesV1  = 48 * 1024
	wireChunkBytesV1 = 64 * 1024
)

type offlineContentItemWireV1 struct {
	Ref          contract.ContentRef `json:"ref"`
	Base64Chunks []string            `json:"base64_chunks"`
}

type offlineContentBundleWireV1 struct {
	Items            []offlineContentItemWireV1 `json:"items"`
	ContentSetDigest contract.Digest            `json:"content_set_digest"`
}

type compileFrameRequestWireV1 struct {
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
	InputBundle       offlineContentBundleWireV1  `json:"input_bundle"`
}

type compiledBundleWireV1 struct {
	Manifest              contract.ContextManifest   `json:"manifest"`
	Frame                 contract.ContextFrame      `json:"frame"`
	ContentBundle         offlineContentBundleWireV1 `json:"content_bundle"`
	ResidualCandidateRefs []contract.FactRef         `json:"residual_candidate_refs"`
	Authoritative         *bool                      `json:"authoritative"`
	CompileDigest         contract.Digest            `json:"compile_digest"`
}

type previewFrameRequestWireV1 struct {
	Meta                  OfflineRequestMetaV1 `json:"meta"`
	Compiled              compiledBundleWireV1 `json:"compiled"`
	ExpectedCompileDigest contract.Digest      `json:"expected_compile_digest"`
	CheckedUnixNano       int64                `json:"checked_unix_nano"`
}

type inspectFrameExactRequestWireV1 struct {
	Meta                  OfflineRequestMetaV1       `json:"meta"`
	Manifest              contract.ContextManifest   `json:"manifest"`
	Frame                 contract.ContextFrame      `json:"frame"`
	ContentBundle         offlineContentBundleWireV1 `json:"content_bundle"`
	ExpectedManifestRef   contract.FactRef           `json:"expected_manifest_ref"`
	ExpectedFrameRef      contract.FactRef           `json:"expected_frame_ref"`
	ExpectedCompileDigest contract.Digest            `json:"expected_compile_digest"`
	CheckedUnixNano       int64                      `json:"checked_unix_nano"`
}

type compileFrameResponseWireV1 struct {
	Meta        OfflineResponseMetaV1 `json:"meta"`
	Compiled    compiledBundleWireV1  `json:"compiled"`
	Diagnostics []OfflineDiagnosticV1 `json:"diagnostics"`
}

func DecodeValidateRecipeRequestV1(ctx context.Context, payload []byte) (ValidateRecipeRequestV1, error) {
	var request ValidateRecipeRequestV1
	if err := decodeStrictV1(ctx, OfflineValidateRecipeV1, payload, hardWire48MiBV1, &request); err != nil {
		return ValidateRecipeRequestV1{}, err
	}
	if err := validateRequestMetaV1(request.Meta, OfflineValidateRecipeV1); err != nil {
		return ValidateRecipeRequestV1{}, err
	}
	if request.Candidates == nil {
		return ValidateRecipeRequestV1{}, sdkErrorV1(OfflineErrorInvalidArgumentV1, OfflineValidateRecipeV1, "candidates", "candidates cannot be null", contract.ErrInvalid)
	}
	if uint64(len(payload)) > request.Meta.Limits.MaxWireRequestBytes {
		return ValidateRecipeRequestV1{}, sdkErrorV1(OfflineErrorLimitExceededV1, OfflineValidateRecipeV1, "payload", "request wire limit exceeded", contract.ErrLimitExceeded)
	}
	if err := validateWireAccountingV1(payload, 0, request.Meta.Limits, OfflineValidateRecipeV1); err != nil {
		return ValidateRecipeRequestV1{}, err
	}
	return cloneValidateRequestV1(request), nil
}

func DecodeCompileFrameRequestV1(ctx context.Context, payload []byte) (CompileFrameRequestV1, error) {
	var wire compileFrameRequestWireV1
	if err := decodeStrictV1(ctx, OfflineCompileFrameV1, payload, hardWire48MiBV1, &wire); err != nil {
		return CompileFrameRequestV1{}, err
	}
	if err := validateRequestMetaV1(wire.Meta, OfflineCompileFrameV1); err != nil {
		return CompileFrameRequestV1{}, err
	}
	if wire.Candidates == nil {
		return CompileFrameRequestV1{}, sdkErrorV1(OfflineErrorInvalidArgumentV1, OfflineCompileFrameV1, "candidates", "candidates cannot be null", contract.ErrInvalid)
	}
	if uint64(len(payload)) > wire.Meta.Limits.MaxWireRequestBytes {
		return CompileFrameRequestV1{}, sdkErrorV1(OfflineErrorLimitExceededV1, OfflineCompileFrameV1, "payload", "request wire limit exceeded", contract.ErrLimitExceeded)
	}
	if err := validateWireAccountingV1(payload, wireBundleContentCharsV1(wire.InputBundle), wire.Meta.Limits, OfflineCompileFrameV1); err != nil {
		return CompileFrameRequestV1{}, err
	}
	bundle, err := decodeBundleWireV1(ctx, OfflineCompileFrameV1, wire.InputBundle, wire.Meta.Limits)
	if err != nil {
		return CompileFrameRequestV1{}, err
	}
	request := CompileFrameRequestV1{
		Meta: wire.Meta, AttemptID: wire.AttemptID, ManifestID: wire.ManifestID, FrameID: wire.FrameID,
		GenerationID: wire.GenerationID, GenerationOrdinal: wire.GenerationOrdinal, Recipe: wire.Recipe,
		Execution: wire.Execution, Candidates: wire.Candidates, ParentFrame: wire.ParentFrame,
		CreatedUnixNano: wire.CreatedUnixNano, ExpiresUnixNano: wire.ExpiresUnixNano, InputBundle: bundle,
	}
	cloned, cloneErr := cloneCompileRequestContextV1(ctx, request)
	if cloneErr != nil {
		return CompileFrameRequestV1{}, mapErrorV1(OfflineCompileFrameV1, "request", cloneErr)
	}
	return cloned, nil
}

func DecodePreviewFrameRequestV1(ctx context.Context, payload []byte) (PreviewFrameRequestV1, error) {
	var wire previewFrameRequestWireV1
	if err := decodeStrictV1(ctx, OfflinePreviewFrameV1, payload, hardWire144MiBV1, &wire); err != nil {
		return PreviewFrameRequestV1{}, err
	}
	if err := validateRequestMetaV1(wire.Meta, OfflinePreviewFrameV1); err != nil {
		return PreviewFrameRequestV1{}, err
	}
	if uint64(len(payload)) > wire.Meta.Limits.MaxWireRequestBytes {
		return PreviewFrameRequestV1{}, sdkErrorV1(OfflineErrorLimitExceededV1, OfflinePreviewFrameV1, "payload", "request wire limit exceeded", contract.ErrLimitExceeded)
	}
	if err := validateWireAccountingV1(payload, wireBundleContentCharsV1(wire.Compiled.ContentBundle), wire.Meta.Limits, OfflinePreviewFrameV1); err != nil {
		return PreviewFrameRequestV1{}, err
	}
	compiled, err := compiledFromWireV1(ctx, OfflinePreviewFrameV1, wire.Compiled, wire.Meta.Limits)
	if err != nil {
		return PreviewFrameRequestV1{}, err
	}
	return PreviewFrameRequestV1{Meta: wire.Meta, Compiled: compiled, ExpectedCompileDigest: wire.ExpectedCompileDigest, CheckedUnixNano: wire.CheckedUnixNano}, nil
}

func DecodeInspectFrameExactRequestV1(ctx context.Context, payload []byte) (InspectFrameExactRequestV1, error) {
	var wire inspectFrameExactRequestWireV1
	if err := decodeStrictV1(ctx, OfflineInspectFrameExactV1, payload, hardWire144MiBV1, &wire); err != nil {
		return InspectFrameExactRequestV1{}, err
	}
	if err := validateRequestMetaV1(wire.Meta, OfflineInspectFrameExactV1); err != nil {
		return InspectFrameExactRequestV1{}, err
	}
	if uint64(len(payload)) > wire.Meta.Limits.MaxWireRequestBytes {
		return InspectFrameExactRequestV1{}, sdkErrorV1(OfflineErrorLimitExceededV1, OfflineInspectFrameExactV1, "payload", "request wire limit exceeded", contract.ErrLimitExceeded)
	}
	if err := validateWireAccountingV1(payload, wireBundleContentCharsV1(wire.ContentBundle), wire.Meta.Limits, OfflineInspectFrameExactV1); err != nil {
		return InspectFrameExactRequestV1{}, err
	}
	bundle, err := decodeBundleWireV1(ctx, OfflineInspectFrameExactV1, wire.ContentBundle, wire.Meta.Limits)
	if err != nil {
		return InspectFrameExactRequestV1{}, err
	}
	return InspectFrameExactRequestV1{Meta: wire.Meta, Manifest: wire.Manifest, Frame: wire.Frame, ContentBundle: bundle, ExpectedManifestRef: wire.ExpectedManifestRef, ExpectedFrameRef: wire.ExpectedFrameRef, ExpectedCompileDigest: wire.ExpectedCompileDigest, CheckedUnixNano: wire.CheckedUnixNano}, nil
}

func EncodeValidateRecipeResponseV1(ctx context.Context, response ValidateRecipeResponseV1) ([]byte, error) {
	if err := validateValidateResponseV1(ctx, response); err != nil {
		return nil, err
	}
	return encodeResponseV1(ctx, OfflineValidateRecipeV1, response, response.Meta, response.limits)
}

func EncodeCompileFrameResponseV1(ctx context.Context, response CompileFrameResponseV1) ([]byte, error) {
	if err := validateCompileResponseV1(ctx, response); err != nil {
		return nil, err
	}
	return encodeCompileResponseStreamingV1(ctx, response)
}

func EncodePreviewFrameResponseV1(ctx context.Context, response PreviewFrameResponseV1) ([]byte, error) {
	if err := validatePreviewResponseV1(ctx, response); err != nil {
		return nil, err
	}
	return encodeResponseV1(ctx, OfflinePreviewFrameV1, response, response.Meta, response.limits)
}

func EncodeInspectFrameExactResponseV1(ctx context.Context, response InspectFrameExactResponseV1) ([]byte, error) {
	if err := validateInspectResponseV1(ctx, response); err != nil {
		return nil, err
	}
	return encodeResponseV1(ctx, OfflineInspectFrameExactV1, response, response.Meta, response.limits)
}

func validateResponseEnvelopeV1(meta OfflineResponseMetaV1, limits OfflineSDKLimitsV1, op OfflineSDKOperationV1) error {
	if meta.ContractVersion != OfflineSDKContractVersionV1 || meta.Operation != op || meta.RequestID == "" || meta.RequestDigest.Validate() != nil || meta.ResultDigest.Validate() != nil {
		return sdkErrorV1(OfflineErrorInvalidArgumentV1, op, "meta", "invalid response metadata", contract.ErrInvalid)
	}
	if err := limits.validate(op); err != nil {
		return sdkErrorV1(OfflineErrorLimitExceededV1, op, "response.limits", err.Error(), contract.ErrLimitExceeded)
	}
	return nil
}

func validateDiagnosticsV1(values []OfflineDiagnosticV1, limits OfflineSDKLimitsV1, op OfflineSDKOperationV1) error {
	if values == nil {
		return sdkErrorV1(OfflineErrorInvalidArgumentV1, op, "diagnostics", "diagnostics must be present", contract.ErrInvalid)
	}
	if len(values) > int(limits.MaxDiagnostics) {
		return sdkErrorV1(OfflineErrorLimitExceededV1, op, "diagnostics", "diagnostic limit exceeded", contract.ErrLimitExceeded)
	}
	for index, value := range values {
		if value.Code == "" || value.ObjectKind == "" || value.ObjectID == "" || value.FieldPath == "" || (value.Severity != "info" && value.Severity != "warning" && value.Severity != "error") {
			return sdkErrorV1(OfflineErrorInvalidArgumentV1, op, fmt.Sprintf("diagnostics[%d]", index), "invalid diagnostic", contract.ErrInvalid)
		}
		if uint64(len(value.Message)) > uint64(limits.MaxDiagnosticMessageBytes) {
			return sdkErrorV1(OfflineErrorLimitExceededV1, op, fmt.Sprintf("diagnostics[%d].message", index), "diagnostic message limit exceeded", contract.ErrLimitExceeded)
		}
	}
	if !reflect.DeepEqual(values, canonicalDiagnosticsV1(values, limits)) {
		return sdkErrorV1(OfflineErrorConflictV1, op, "diagnostics", "diagnostics are not canonical", contract.ErrConflict)
	}
	return nil
}

func validateValidateResponseV1(ctx context.Context, response ValidateRecipeResponseV1) error {
	const op = OfflineValidateRecipeV1
	if err := validateContextV1(ctx, op); err != nil {
		return err
	}
	if err := validateResponseEnvelopeV1(response.Meta, response.limits, op); err != nil {
		return err
	}
	if response.CandidateRefs == nil || response.Diagnostics == nil {
		return sdkErrorV1(OfflineErrorInvalidArgumentV1, op, "response", "response slices must be present", contract.ErrInvalid)
	}
	if err := validateDiagnosticsV1(response.Diagnostics, response.limits, op); err != nil {
		return err
	}
	if response.RecipeRef != nil && response.RecipeRef.Validate() != nil {
		return sdkErrorV1(OfflineErrorInvalidArgumentV1, op, "recipe_ref", "invalid recipe reference", contract.ErrInvalid)
	}
	for index, ref := range response.CandidateRefs {
		if ref.Validate() != nil {
			return sdkErrorV1(OfflineErrorInvalidArgumentV1, op, fmt.Sprintf("candidate_refs[%d]", index), "invalid candidate reference", contract.ErrInvalid)
		}
	}
	report, err := canonicalDigestV1("validate-report", struct {
		Valid         bool                  `json:"valid"`
		RecipeRef     *contract.FactRef     `json:"recipe_ref,omitempty"`
		CandidateRefs []contract.FactRef    `json:"candidate_refs"`
		Diagnostics   []OfflineDiagnosticV1 `json:"diagnostics"`
	}{response.Valid, response.RecipeRef, response.CandidateRefs, response.Diagnostics}, ctx)
	if err != nil {
		return mapErrorV1(op, "report_digest", err)
	}
	if report != response.ReportDigest {
		return sdkErrorV1(OfflineErrorConflictV1, op, "report_digest", "report digest mismatch", contract.ErrConflict)
	}
	want, err := validateResponseResultDigestV1("validate-recipe-response", &response.Meta, validateResponsePrivateV1(response), ctx)
	if err != nil {
		return mapErrorV1(op, "meta.result_digest", err)
	}
	if want != response.Meta.ResultDigest {
		return sdkErrorV1(OfflineErrorConflictV1, op, "meta.result_digest", "result digest mismatch", contract.ErrConflict)
	}
	return nil
}

func validateCompileResponseV1(ctx context.Context, response CompileFrameResponseV1) error {
	const op = OfflineCompileFrameV1
	if err := validateContextV1(ctx, op); err != nil {
		return err
	}
	if err := validateResponseEnvelopeV1(response.Meta, response.limits, op); err != nil {
		return err
	}
	if response.Diagnostics == nil || response.Compiled.ResidualCandidateRefs == nil || response.Compiled.Authoritative {
		return sdkErrorV1(OfflineErrorInvalidArgumentV1, op, "response", "invalid compile response shape", contract.ErrInvalid)
	}
	if err := validateDiagnosticsV1(response.Diagnostics, response.limits, op); err != nil {
		return err
	}
	if response.Compiled.Manifest.Validate() != nil || response.Compiled.Frame.Validate() != nil {
		return sdkErrorV1(OfflineErrorInvalidArgumentV1, op, "compiled", "invalid manifest or frame", contract.ErrInvalid)
	}
	if err := response.Compiled.ContentBundle.validateContextV1(ctx, response.limits); err != nil {
		return withOperationV1(err, op)
	}
	if err := validateCompiledRawLimitsV1(response.Compiled.ContentBundle, response.limits, op); err != nil {
		return err
	}
	for index, ref := range response.Compiled.ResidualCandidateRefs {
		if ref.Validate() != nil {
			return sdkErrorV1(OfflineErrorInvalidArgumentV1, op, fmt.Sprintf("compiled.residual_candidate_refs[%d]", index), "invalid residual reference", contract.ErrInvalid)
		}
	}
	if !reflect.DeepEqual(response.Compiled.ResidualCandidateRefs, residualCandidateRefsV1(response.Compiled.Manifest)) {
		return sdkErrorV1(OfflineErrorConflictV1, op, "compiled.residual_candidate_refs", "residual closure mismatch", contract.ErrConflict)
	}
	if err := kernel.InspectFrameStagedV1(ctx, bundleStoreV1{response.Compiled.ContentBundle}, response.Compiled.Manifest, response.Compiled.Frame, inspectWorkLimitsV1(response.limits)); err != nil {
		return mapErrorV1(op, "compiled", err)
	}
	wantCompile, err := compileDigestV1(response.Compiled, ctx)
	if err != nil {
		return mapErrorV1(op, "compiled.compile_digest", err)
	}
	if wantCompile != response.Compiled.CompileDigest {
		return sdkErrorV1(OfflineErrorConflictV1, op, "compiled.compile_digest", "compile digest mismatch", contract.ErrConflict)
	}
	want, err := validateResponseResultDigestV1("compile-frame-response", &response.Meta, responsePrivateV1(response), ctx)
	if err != nil {
		return mapErrorV1(op, "meta.result_digest", err)
	}
	if want != response.Meta.ResultDigest {
		return sdkErrorV1(OfflineErrorConflictV1, op, "meta.result_digest", "result digest mismatch", contract.ErrConflict)
	}
	return nil
}

func validatePreviewResponseV1(ctx context.Context, response PreviewFrameResponseV1) error {
	const op = OfflinePreviewFrameV1
	if err := validateContextV1(ctx, op); err != nil {
		return err
	}
	if err := validateResponseEnvelopeV1(response.Meta, response.limits, op); err != nil {
		return err
	}
	if response.AdmissionDecisions == nil || response.Fragments == nil || response.Diagnostics == nil {
		return sdkErrorV1(OfflineErrorInvalidArgumentV1, op, "response", "response slices must be present", contract.ErrInvalid)
	}
	if err := validateDiagnosticsV1(response.Diagnostics, response.limits, op); err != nil {
		return err
	}
	for index, decision := range response.AdmissionDecisions {
		if decision.Validate() != nil {
			return sdkErrorV1(OfflineErrorInvalidArgumentV1, op, fmt.Sprintf("admission_decisions[%d]", index), "invalid admission decision", contract.ErrInvalid)
		}
	}
	for index, fragment := range response.Fragments {
		if fragment.Position == 0 || fragment.CandidateRef.Validate() != nil || fragment.Region == "" || fragment.ContentRef.Validate() != nil || fragment.Tokens == 0 {
			return sdkErrorV1(OfflineErrorInvalidArgumentV1, op, fmt.Sprintf("fragments[%d]", index), "invalid fragment preview", contract.ErrInvalid)
		}
	}
	if response.StablePrefixRef.Validate() != nil || response.DynamicTailRef.Validate() != nil || response.RenderedRef.Validate() != nil || response.RecipeRef.Validate() != nil || response.ManifestRef.Validate() != nil || response.FrameRef.Validate() != nil || response.SourceSetDigest.Validate() != nil || response.ExpiresUnixNano <= 0 {
		return sdkErrorV1(OfflineErrorInvalidArgumentV1, op, "response", "invalid preview closure", contract.ErrInvalid)
	}
	if response.SemiStableRef != nil && response.SemiStableRef.Validate() != nil {
		return sdkErrorV1(OfflineErrorInvalidArgumentV1, op, "semi_stable_ref", "invalid semi-stable reference", contract.ErrInvalid)
	}
	if response.StableTokens > response.TotalTokens || response.SemiStableTokens > response.TotalTokens-response.StableTokens || response.DynamicTokens != response.TotalTokens-response.StableTokens-response.SemiStableTokens || response.TotalTokens > response.limits.MaxTotalTokens {
		return sdkErrorV1(OfflineErrorLimitExceededV1, op, "total_tokens", "preview token limit or total mismatch", contract.ErrLimitExceeded)
	}
	wantPreview, err := previewDigestV1(response, ctx)
	if err != nil {
		return mapErrorV1(op, "preview_digest", err)
	}
	if wantPreview != response.PreviewDigest {
		return sdkErrorV1(OfflineErrorConflictV1, op, "preview_digest", "preview digest mismatch", contract.ErrConflict)
	}
	want, err := validateResponseResultDigestV1("preview-frame-response", &response.Meta, previewResponsePrivateV1(response), ctx)
	if err != nil {
		return mapErrorV1(op, "meta.result_digest", err)
	}
	if want != response.Meta.ResultDigest {
		return sdkErrorV1(OfflineErrorConflictV1, op, "meta.result_digest", "result digest mismatch", contract.ErrConflict)
	}
	return nil
}

func validateInspectResponseV1(ctx context.Context, response InspectFrameExactResponseV1) error {
	const op = OfflineInspectFrameExactV1
	if err := validateContextV1(ctx, op); err != nil {
		return err
	}
	if err := validateResponseEnvelopeV1(response.Meta, response.limits, op); err != nil {
		return err
	}
	if response.Diagnostics == nil {
		return sdkErrorV1(OfflineErrorInvalidArgumentV1, op, "response.diagnostics", "diagnostics must be present", contract.ErrInvalid)
	}
	if err := validateDiagnosticsV1(response.Diagnostics, response.limits, op); err != nil {
		return err
	}
	if !response.Exact || response.ManifestRef.Validate() != nil || response.FrameRef.Validate() != nil || response.ContentSetDigest.Validate() != nil || response.CheckedUnixNano <= 0 || response.ExpiresUnixNano <= response.CheckedUnixNano {
		return sdkErrorV1(OfflineErrorInvalidArgumentV1, op, "response", "invalid inspection closure", contract.ErrInvalid)
	}
	wantInspection, err := inspectionDigestV1(response, ctx)
	if err != nil {
		return mapErrorV1(op, "inspection_digest", err)
	}
	if wantInspection != response.InspectionDigest {
		return sdkErrorV1(OfflineErrorConflictV1, op, "inspection_digest", "inspection digest mismatch", contract.ErrConflict)
	}
	want, err := validateResponseResultDigestV1("inspect-frame-exact-response", &response.Meta, inspectResponsePrivateV1(response), ctx)
	if err != nil {
		return mapErrorV1(op, "meta.result_digest", err)
	}
	if want != response.Meta.ResultDigest {
		return sdkErrorV1(OfflineErrorConflictV1, op, "meta.result_digest", "result digest mismatch", contract.ErrConflict)
	}
	return nil
}

func encodeResponseV1(ctx context.Context, op OfflineSDKOperationV1, value any, meta OfflineResponseMetaV1, limits OfflineSDKLimitsV1) ([]byte, error) {
	if err := validateContextV1(ctx, op); err != nil {
		return nil, err
	}
	if err := validateResponseEnvelopeV1(meta, limits, op); err != nil {
		return nil, err
	}
	buffer := &boundedCodecBufferV1{ctx: ctx, max: limits.MaxWireResponseBytes}
	if err := buffer.writeJSON(value); err != nil {
		return nil, mapErrorV1(op, "response", err)
	}
	payload := buffer.buf.Bytes()
	if uint64(len(payload)) > limits.MaxWireResponseBytes {
		return nil, sdkErrorV1(OfflineErrorLimitExceededV1, op, "response", "response wire limit exceeded", contract.ErrLimitExceeded)
	}
	if uint64(len(payload)) > limits.MaxNonContentWireBytes {
		return nil, sdkErrorV1(OfflineErrorLimitExceededV1, op, "response", "non-content wire limit exceeded", contract.ErrLimitExceeded)
	}
	if err := validateContextV1(ctx, op); err != nil {
		return nil, err
	}
	return cloneCodecBytesV1(ctx, payload)
}

func decodeStrictV1(ctx context.Context, op OfflineSDKOperationV1, payload []byte, hardCap uint64, target any) error {
	meta, err := preflightWireMetaV1(ctx, payload, op, hardCap)
	if err != nil {
		return err
	}
	copyPayload, err := cloneCodecBytesV1(ctx, payload)
	if err != nil {
		return mapErrorV1(op, "payload", err)
	}
	scan, err := scanDuplicateKeysContextV1(ctx, copyPayload, op, meta.Limits)
	if err != nil {
		if ctx.Err() != nil {
			return mapErrorV1(op, "payload", ctx.Err())
		}
		if errors.Is(err, contract.ErrInvalid) || errors.Is(err, contract.ErrLimitExceeded) {
			return mapErrorV1(op, "payload", err)
		}
		return mapErrorV1(op, "payload", fmt.Errorf("%w: strict token scan: %v", contract.ErrInvalid, err))
	}
	if err := validateRequiredPresenceV1(op, scan.presence); err != nil {
		return sdkErrorV1(OfflineErrorInvalidArgumentV1, op, "payload", err.Error(), contract.ErrInvalid)
	}
	if err := validateWireAccountingV1(copyPayload, scan.contentChars, meta.Limits, op); err != nil {
		return err
	}
	reader := &contextChunkReaderV1{ctx: ctx, reader: bytes.NewReader(copyPayload)}
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		if ctx.Err() != nil {
			return mapErrorV1(op, "payload", ctx.Err())
		}
		return mapErrorV1(op, "payload", fmt.Errorf("%w: strict json: %v", contract.ErrInvalid, err))
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if ctx.Err() != nil {
			return mapErrorV1(op, "payload", ctx.Err())
		}
		return sdkErrorV1(OfflineErrorInvalidArgumentV1, op, "payload", "trailing json document", contract.ErrInvalid)
	}
	if err := validateDecodedPresenceV1(op, scan.presence, target); err != nil {
		return sdkErrorV1(OfflineErrorInvalidArgumentV1, op, "payload", err.Error(), contract.ErrInvalid)
	}
	return validateContextV1(ctx, op)
}

func encodeCompileResponseStreamingV1(ctx context.Context, response CompileFrameResponseV1) ([]byte, error) {
	const op = OfflineCompileFrameV1
	if err := validateContextV1(ctx, op); err != nil {
		return nil, err
	}
	buffer := &boundedCodecBufferV1{ctx: ctx, max: response.limits.MaxWireResponseBytes}
	if err := buffer.writeLiteral(`{"meta":`); err != nil {
		return nil, mapErrorV1(op, "response", err)
	}
	if err := buffer.writeJSON(response.Meta); err != nil {
		return nil, mapErrorV1(op, "response.meta", err)
	}
	if err := buffer.writeLiteral(`,"compiled":{"manifest":`); err != nil {
		return nil, mapErrorV1(op, "response", err)
	}
	if err := buffer.writeJSON(response.Compiled.Manifest); err != nil {
		return nil, mapErrorV1(op, "response.compiled.manifest", err)
	}
	if err := buffer.writeLiteral(`,"frame":`); err != nil {
		return nil, mapErrorV1(op, "response", err)
	}
	if err := buffer.writeJSON(response.Compiled.Frame); err != nil {
		return nil, mapErrorV1(op, "response.compiled.frame", err)
	}
	if err := buffer.writeLiteral(`,"content_bundle":{"items":[`); err != nil {
		return nil, mapErrorV1(op, "response", err)
	}
	for index, item := range response.Compiled.ContentBundle.items {
		if index > 0 {
			if err := buffer.writeLiteral(`,`); err != nil {
				return nil, mapErrorV1(op, "response", err)
			}
		}
		if err := buffer.writeLiteral(`{"ref":`); err != nil {
			return nil, mapErrorV1(op, "response", err)
		}
		if err := buffer.writeJSON(item.Ref); err != nil {
			return nil, mapErrorV1(op, "response.compiled.content_bundle.items.ref", err)
		}
		if err := buffer.writeLiteral(`,"base64_chunks":[`); err != nil {
			return nil, mapErrorV1(op, "response", err)
		}
		for offset := 0; offset < len(item.Bytes); offset += rawChunkBytesV1 {
			if err := ctx.Err(); err != nil {
				return nil, mapErrorV1(op, "response.compiled.content_bundle.items.base64_chunks", err)
			}
			if offset > 0 {
				if err := buffer.writeLiteral(`,`); err != nil {
					return nil, mapErrorV1(op, "response", err)
				}
			}
			end := offset + rawChunkBytesV1
			if end > len(item.Bytes) {
				end = len(item.Bytes)
			}
			chunk := base64.StdEncoding.EncodeToString(item.Bytes[offset:end])
			if err := buffer.writeJSON(chunk); err != nil {
				return nil, mapErrorV1(op, "response.compiled.content_bundle.items.base64_chunks", err)
			}
		}
		if err := buffer.writeLiteral(`]}`); err != nil {
			return nil, mapErrorV1(op, "response", err)
		}
	}
	if err := buffer.writeLiteral(`],"content_set_digest":`); err != nil {
		return nil, mapErrorV1(op, "response", err)
	}
	if err := buffer.writeJSON(response.Compiled.ContentBundle.ContentSetDigest()); err != nil {
		return nil, mapErrorV1(op, "response.compiled.content_bundle.content_set_digest", err)
	}
	if err := buffer.writeLiteral(`},"residual_candidate_refs":`); err != nil {
		return nil, mapErrorV1(op, "response", err)
	}
	if err := buffer.writeJSON(response.Compiled.ResidualCandidateRefs); err != nil {
		return nil, mapErrorV1(op, "response.compiled.residual_candidate_refs", err)
	}
	if err := buffer.writeLiteral(`,"authoritative":false,"compile_digest":`); err != nil {
		return nil, mapErrorV1(op, "response", err)
	}
	if err := buffer.writeJSON(response.Compiled.CompileDigest); err != nil {
		return nil, mapErrorV1(op, "response.compiled.compile_digest", err)
	}
	if err := buffer.writeLiteral(`},"diagnostics":`); err != nil {
		return nil, mapErrorV1(op, "response", err)
	}
	if err := buffer.writeJSON(response.Diagnostics); err != nil {
		return nil, mapErrorV1(op, "response.diagnostics", err)
	}
	if err := buffer.writeLiteral(`}`); err != nil {
		return nil, mapErrorV1(op, "response", err)
	}
	payload := buffer.buf.Bytes()
	var contentChars uint64
	for _, item := range response.Compiled.ContentBundle.items {
		encoded := ((item.Ref.Length + 2) / 3) * 4
		if ^uint64(0)-contentChars < encoded {
			return nil, sdkErrorV1(OfflineErrorLimitExceededV1, op, "response", "wire accounting overflow", contract.ErrLimitExceeded)
		}
		contentChars += encoded
	}
	if uint64(len(payload)) > response.limits.MaxWireResponseBytes {
		return nil, sdkErrorV1(OfflineErrorLimitExceededV1, op, "response", "response wire limit exceeded", contract.ErrLimitExceeded)
	}
	if uint64(len(payload)) < contentChars || uint64(len(payload))-contentChars > response.limits.MaxNonContentWireBytes {
		return nil, sdkErrorV1(OfflineErrorLimitExceededV1, op, "response", "non-content wire limit exceeded", contract.ErrLimitExceeded)
	}
	return cloneCodecBytesV1(ctx, payload)
}

func wireBundleContentCharsV1(bundle offlineContentBundleWireV1) uint64 {
	var total uint64
	for _, item := range bundle.Items {
		for _, chunk := range item.Base64Chunks {
			if ^uint64(0)-total < uint64(len(chunk)) {
				return ^uint64(0)
			}
			total += uint64(len(chunk))
		}
	}
	return total
}

func validateWireAccountingV1(payload []byte, contentChars uint64, limits OfflineSDKLimitsV1, op OfflineSDKOperationV1) error {
	if contentChars == ^uint64(0) || uint64(len(payload)) < contentChars || uint64(len(payload))-contentChars > limits.MaxNonContentWireBytes {
		return sdkErrorV1(OfflineErrorLimitExceededV1, op, "payload", "non-content wire limit exceeded", contract.ErrLimitExceeded)
	}
	return nil
}

type boundedCodecBufferV1 struct {
	ctx context.Context
	max uint64
	buf bytes.Buffer
}

func (b *boundedCodecBufferV1) writeLiteral(value string) error {
	return b.writeBytes([]byte(value))
}

func (b *boundedCodecBufferV1) writeJSON(value any) error {
	return writeJSONContextV1(b.ctx, b, value)
}

func (b *boundedCodecBufferV1) Write(value []byte) (int, error) {
	if err := b.writeBytes(value); err != nil {
		return 0, err
	}
	return len(value), nil
}

func (b *boundedCodecBufferV1) writeBytes(value []byte) error {
	for offset := 0; offset < len(value); offset += wireChunkBytesV1 {
		if err := b.ctx.Err(); err != nil {
			return err
		}
		end := offset + wireChunkBytesV1
		if end > len(value) {
			end = len(value)
		}
		if uint64(end-offset) > b.max || uint64(b.buf.Len()) > b.max-uint64(end-offset) {
			return contract.ErrLimitExceeded
		}
		if _, err := b.buf.Write(value[offset:end]); err != nil {
			return err
		}
	}
	return b.ctx.Err()
}

func cloneCodecBytesV1(ctx context.Context, value []byte) ([]byte, error) {
	if ctx == nil {
		return nil, contract.ErrInvalid
	}
	result := make([]byte, len(value))
	for offset := 0; offset < len(value); offset += wireChunkBytesV1 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		end := offset + wireChunkBytesV1
		if end > len(value) {
			end = len(value)
		}
		copy(result[offset:end], value[offset:end])
	}
	return result, ctx.Err()
}

type contextChunkReaderV1 struct {
	ctx    context.Context
	reader *bytes.Reader
}

func (r *contextChunkReaderV1) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	if len(p) > wireChunkBytesV1 {
		p = p[:wireChunkBytesV1]
	}
	return r.reader.Read(p)
}

type strictJSONScanV1 struct {
	presence     map[string][]map[string]struct{}
	arrayCounts  map[string]uint64
	limits       OfflineSDKLimitsV1
	op           OfflineSDKOperationV1
	contentChars uint64
	rawBytes     uint64
}

func scanDuplicateKeysContextV1(ctx context.Context, payload []byte, op OfflineSDKOperationV1, limits OfflineSDKLimitsV1) (strictJSONScanV1, error) {
	decoder := json.NewDecoder(&contextChunkReaderV1{ctx: ctx, reader: bytes.NewReader(payload)})
	decoder.UseNumber()
	scan := strictJSONScanV1{presence: make(map[string][]map[string]struct{}), arrayCounts: make(map[string]uint64), limits: limits, op: op}
	if err := scanJSONValueContextV1(ctx, decoder, "$", &scan); err != nil {
		return strictJSONScanV1{}, err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return strictJSONScanV1{}, fmt.Errorf("%w: trailing json", contract.ErrInvalid)
	}
	return scan, ctx.Err()
}

func scanJSONValueContextV1(ctx context.Context, decoder *json.Decoder, path string, scan *strictJSONScanV1) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		if token == nil && (strings.HasSuffix(path, ".rules") || strings.HasSuffix(path, ".decisions") || strings.HasSuffix(path, ".fragments")) {
			return fmt.Errorf("%w: required array %s cannot be null", contract.ErrInvalid, path)
		}
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]struct{}{}
		scan.presence[path] = append(scan.presence[path], seen)
		for decoder.More() {
			if err := ctx.Err(); err != nil {
				return err
			}
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return fmt.Errorf("object key is not a string")
			}
			if _, exists := seen[key]; exists {
				return fmt.Errorf("duplicate key %q", key)
			}
			seen[key] = struct{}{}
			if err := scanJSONValueContextV1(ctx, decoder, path+"."+key, scan); err != nil {
				return err
			}
		}
		_, err = decoder.Token()
		return err
	case '[':
		if strings.HasSuffix(path, ".base64_chunks") {
			return scanBase64ChunkArrayV1(ctx, decoder, path, scan)
		}
		count := uint64(0)
		maxCount := scanArrayMaxV1(path, scan)
		for decoder.More() {
			count++
			if maxCount > 0 && count > maxCount {
				return fmt.Errorf("%w: array count limit %s", contract.ErrLimitExceeded, path)
			}
			if err := scanJSONValueContextV1(ctx, decoder, path+"[]", scan); err != nil {
				return err
			}
		}
		scan.arrayCounts[path] += count
		_, err = decoder.Token()
		return err
	default:
		return fmt.Errorf("unexpected delimiter")
	}
}

func scanArrayMaxV1(path string, scan *strictJSONScanV1) uint64 {
	limits := scan.limits
	switch {
	case strings.HasSuffix(path, ".candidates"), strings.HasSuffix(path, ".decisions"), strings.HasSuffix(path, ".fragments"), strings.HasSuffix(path, ".residual_candidate_refs"):
		return uint64(limits.MaxCandidates)
	case strings.HasSuffix(path, ".rules"):
		return 64
	case strings.HasSuffix(path, ".items"):
		if scan.op == OfflineCompileFrameV1 {
			return uint64(limits.MaxInputContentItems)
		}
		return uint64(limits.MaxOutputContentItems)
	default:
		return 0
	}
}

func validateDecodedPresenceV1(op OfflineSDKOperationV1, presence map[string][]map[string]struct{}, target any) error {
	switch value := target.(type) {
	case *ValidateRecipeRequestV1:
		if value.Recipe.Rules == nil || value.Candidates == nil {
			return fmt.Errorf("required recipe rules and candidates cannot be null")
		}
	case *CompareRecipesRequestV1:
		if value.BaseRecipe.Rules == nil || value.CandidateRecipe.Rules == nil {
			return fmt.Errorf("required recipe rules cannot be null")
		}
	case *InspectCachePlanRequestV1:
		profileShapeNow := value.ProviderCacheProfile.ExpiresUnixNano - 1
		if value.CachePlan.Validate() != nil || profileShapeNow <= 0 || value.ProviderCacheProfile.Validate(profileShapeNow) != nil {
			return fmt.Errorf("required cache plan and provider profile cannot be null or malformed")
		}
	case *compileFrameRequestWireV1:
		if value.Recipe.Rules == nil || value.Candidates == nil || value.InputBundle.Items == nil {
			return fmt.Errorf("required recipe rules, candidates, and input items cannot be null")
		}
	case *previewFrameRequestWireV1:
		if value.Compiled.Authoritative == nil || value.Compiled.Manifest.Decisions == nil || value.Compiled.Manifest.Fragments == nil || value.Compiled.ResidualCandidateRefs == nil || value.Compiled.ContentBundle.Items == nil {
			return fmt.Errorf("required compiled values cannot be null")
		}
		if err := validateDecisionRegionPresenceV1("$.compiled.manifest.decisions[]", value.Compiled.Manifest.Decisions, presence); err != nil {
			return err
		}
	case *inspectFrameExactRequestWireV1:
		if value.Manifest.Decisions == nil || value.Manifest.Fragments == nil || value.ContentBundle.Items == nil {
			return fmt.Errorf("required manifest values and content items cannot be null")
		}
		if err := validateDecisionRegionPresenceV1("$.manifest.decisions[]", value.Manifest.Decisions, presence); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported decoded request type")
	}
	return nil
}

func validateDecisionRegionPresenceV1(path string, decisions []contract.AdmissionDecision, presence map[string][]map[string]struct{}) error {
	objects := presence[path]
	if len(objects) != len(decisions) {
		return fmt.Errorf("decision presence mismatch at %s", path)
	}
	for index := range decisions {
		_, hasRegion := objects[index]["region"]
		if decisions[index].Disposition == contract.AdmissionAdmitted && !hasRegion {
			return fmt.Errorf("admitted decision requires region at %s[%d]", path, index)
		}
		if decisions[index].Disposition != contract.AdmissionAdmitted && hasRegion {
			return fmt.Errorf("non-admitted decision forbids region at %s[%d]", path, index)
		}
	}
	return nil
}

func scanBase64ChunkArrayV1(ctx context.Context, decoder *json.Decoder, path string, scan *strictJSONScanV1) error {
	maxItem, maxRaw := scan.limits.MaxOutputRawBytes, scan.limits.MaxOutputRawBytes
	if scan.op == OfflineCompileFrameV1 {
		maxItem, maxRaw = scan.limits.MaxInputContentItemBytes, scan.limits.MaxInputRawBytes
	}
	maxChunks := (maxItem + rawChunkBytesV1 - 1) / rawChunkBytesV1
	scratch := make([]byte, rawChunkBytesV1)
	var itemRaw uint64
	count := uint64(0)
	for decoder.More() {
		if err := ctx.Err(); err != nil {
			return err
		}
		count++
		if count > maxChunks {
			return fmt.Errorf("%w: base64 chunk count %s", contract.ErrLimitExceeded, path)
		}
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		chunk, ok := token.(string)
		if !ok || chunk == "" || len(chunk) > wireChunkBytesV1 || strings.IndexAny(chunk, " \t\r\n") >= 0 {
			return fmt.Errorf("%w: invalid base64 chunk %s", contract.ErrInvalid, path)
		}
		decodedLen := base64.StdEncoding.DecodedLen(len(chunk))
		if decodedLen <= 0 || decodedLen > rawChunkBytesV1 {
			return fmt.Errorf("%w: base64 chunk size %s", contract.ErrLimitExceeded, path)
		}
		n, decodeErr := base64.StdEncoding.Strict().Decode(scratch[:decodedLen], []byte(chunk))
		if decodeErr != nil || n <= 0 || base64.StdEncoding.EncodeToString(scratch[:n]) != chunk {
			return fmt.Errorf("%w: non-canonical base64 chunk %s", contract.ErrInvalid, path)
		}
		if decoder.More() && (n != rawChunkBytesV1 || len(chunk) != wireChunkBytesV1) {
			return fmt.Errorf("%w: short non-final base64 chunk %s", contract.ErrInvalid, path)
		}
		if uint64(n) > maxItem-itemRaw || uint64(n) > maxRaw-scan.rawBytes {
			return fmt.Errorf("%w: base64 raw limit %s", contract.ErrLimitExceeded, path)
		}
		itemRaw += uint64(n)
		scan.rawBytes += uint64(n)
		if uint64(len(chunk)) > ^uint64(0)-scan.contentChars {
			return fmt.Errorf("%w: base64 wire overflow %s", contract.ErrLimitExceeded, path)
		}
		scan.contentChars += uint64(len(chunk))
	}
	_, err := decoder.Token()
	return err
}

func validateRequiredPresenceV1(op OfflineSDKOperationV1, presence map[string][]map[string]struct{}) error {
	commonMeta := []string{"contract_version", "request_id", "operation", "limits", "request_digest"}
	limits := []string{"max_recipes", "max_candidates", "max_input_content_items", "max_input_content_item_bytes", "max_input_raw_bytes", "max_generated_content_items", "max_generated_raw_bytes", "max_output_content_items", "max_output_raw_bytes", "max_total_tokens", "max_diagnostics", "max_diagnostic_message_bytes", "max_non_content_wire_bytes", "max_wire_request_bytes", "max_wire_response_bytes"}
	required := map[string][]string{"$.meta": commonMeta, "$.meta.limits": limits}
	switch op {
	case OfflineValidateRecipeV1:
		required["$"] = []string{"meta", "recipe", "candidates"}
	case OfflineCompareRecipesV1:
		required["$"] = []string{"meta", "base_recipe", "candidate_recipe", "checked_unix_nano", "expires_unix_nano"}
	case OfflineInspectCachePlanV1:
		required["$"] = []string{"meta", "cache_plan", "provider_cache_profile", "checked_unix_nano"}
	case OfflineCompileFrameV1:
		required["$"] = []string{"meta", "attempt_id", "manifest_id", "frame_id", "generation_id", "generation_ordinal", "recipe", "execution", "candidates", "created_unix_nano", "expires_unix_nano", "input_bundle"}
		required["$.input_bundle"] = []string{"items", "content_set_digest"}
		required["$.input_bundle.items[]"] = []string{"ref", "base64_chunks"}
	case OfflinePreviewFrameV1:
		required["$"] = []string{"meta", "compiled", "expected_compile_digest", "checked_unix_nano"}
		required["$.compiled"] = []string{"manifest", "frame", "content_bundle", "residual_candidate_refs", "authoritative", "compile_digest"}
		required["$.compiled.content_bundle"] = []string{"items", "content_set_digest"}
		required["$.compiled.content_bundle.items[]"] = []string{"ref", "base64_chunks"}
	case OfflineInspectFrameExactV1:
		required["$"] = []string{"meta", "manifest", "frame", "content_bundle", "expected_manifest_ref", "expected_frame_ref", "expected_compile_digest", "checked_unix_nano"}
		required["$.content_bundle"] = []string{"items", "content_set_digest"}
		required["$.content_bundle.items[]"] = []string{"ref", "base64_chunks"}
	default:
		return fmt.Errorf("unsupported operation")
	}
	for path, keys := range required {
		objects, exists := presence[path]
		if !exists || len(objects) == 0 {
			// Empty arrays have no element object and therefore no [] path.
			if strings.HasSuffix(path, "items[]") {
				continue
			}
			return fmt.Errorf("missing required object %s", path)
		}
		for _, seen := range objects {
			for _, key := range keys {
				if _, ok := seen[key]; !ok {
					return fmt.Errorf("missing required key %s.%s", path, key)
				}
			}
		}
	}
	for path, objects := range presence {
		keys := requiredNestedKeysV1(path)
		if len(keys) == 0 {
			continue
		}
		for _, seen := range objects {
			for _, key := range keys {
				if _, ok := seen[key]; !ok {
					return fmt.Errorf("missing required key %s.%s", path, key)
				}
			}
		}
	}
	return nil
}

func requiredNestedKeysV1(path string) []string {
	switch {
	case path == "$.recipe" || path == "$.base_recipe" || path == "$.candidate_recipe":
		return []string{"contract_version", "recipe_id", "semantic_version", "revision", "owner", "rules", "budget", "render_version", "created_unix_nano", "expires_unix_nano"}
	case path == "$.recipe.rules[]" || path == "$.base_recipe.rules[]" || path == "$.candidate_recipe.rules[]":
		return []string{"kind", "region", "required", "max_tokens", "degradation"}
	case path == "$.recipe.budget" || path == "$.base_recipe.budget" || path == "$.candidate_recipe.budget":
		return []string{"total_tokens", "stable_prefix_max", "semi_stable_max", "dynamic_tail_max"}
	case path == "$.cache_plan":
		return []string{"contract_version", "plan_id", "revision", "partition", "eligible_tokens", "predicted_reads", "read_cost_per_million", "write_cost_per_million", "keepalive_cost", "ttl_nanos", "created_unix_nano", "expires_unix_nano"}
	case path == "$.cache_plan.partition":
		return []string{"audit_scope_digest", "reuse_scope", "isolation_digest", "authority_digest", "sensitivity", "source_set_digest", "recipe_digest", "render_digest", "model_profile_digest", "harness_digest", "tool_schema_digest", "prefix_digest", "provider_profile_ref", "key_version"}
	case path == "$.provider_cache_profile":
		return []string{"contract_version", "profile_id", "revision", "provider", "route_id", "model", "request_control", "key_ownership", "ttl_control", "usage_observable", "capability_digest", "expires_unix_nano"}
	case strings.HasSuffix(path, ".candidates[]"):
		return []string{"contract_version", "candidate_id", "revision", "kind", "owner", "execution", "source_ref", "source_revision", "content", "trust", "sensitivity", "materialization_mode", "required", "token_estimate", "estimator_digest", "cache_stability", "evidence", "idempotency_key", "created_unix_nano", "expires_unix_nano"}
	case strings.HasSuffix(path, ".manifest") || path == "$.manifest":
		return []string{"contract_version", "manifest_id", "revision", "execution", "recipe_ref", "generation_id", "decisions", "fragments", "stable_tokens", "semi_stable_tokens", "dynamic_tokens", "total_tokens", "source_set_digest", "created_unix_nano", "expires_unix_nano"}
	case strings.HasSuffix(path, ".frame") || path == "$.frame":
		return []string{"contract_version", "frame_id", "revision", "execution", "manifest_ref", "generation_id", "generation", "stable_prefix", "dynamic_tail", "rendered", "source_set_digest", "created_unix_nano", "expires_unix_nano"}
	case strings.HasSuffix(path, ".decisions[]"):
		return []string{"candidate_ref", "disposition", "reason", "tokens"}
	case strings.HasSuffix(path, ".fragments[]"):
		return []string{"candidate_ref", "kind", "region", "position", "content", "tokens"}
	case strings.HasSuffix(path, ".owner"):
		return []string{"component_id", "binding_digest"}
	case strings.HasSuffix(path, ".execution"):
		return []string{"scope_digest", "run_id", "turn", "authority_digest"}
	case strings.HasSuffix(path, ".evidence"):
		return []string{"id", "digest"}
	case isFactRefPresencePathV1(path):
		return []string{"id", "revision", "digest"}
	case isContentRefPresencePathV1(path):
		return []string{"ref", "digest", "length"}
	default:
		return nil
	}
}

func isFactRefPresencePathV1(path string) bool {
	for _, suffix := range []string{".candidate_ref", ".recipe_ref", ".manifest_ref", ".frame_ref", ".parent_frame", ".expected_manifest_ref", ".expected_frame_ref", ".provider_profile_ref", ".residual_candidate_refs[]", ".candidate_refs[]"} {
		if strings.HasSuffix(path, suffix) {
			return true
		}
	}
	return false
}

func isContentRefPresencePathV1(path string) bool {
	for _, suffix := range []string{".content", ".stable_prefix", ".semi_stable", ".dynamic_tail", ".rendered", ".stable_prefix_ref", ".semi_stable_ref", ".dynamic_tail_ref", ".rendered_ref", ".content_ref", ".items[].ref"} {
		if strings.HasSuffix(path, suffix) {
			return true
		}
	}
	return false
}

func encodeBase64ChunksV1(ctx context.Context, value []byte) ([]string, error) {
	if ctx == nil {
		return nil, fmt.Errorf("%w: nil context", contract.ErrInvalid)
	}
	if len(value) == 0 {
		return []string{}, ctx.Err()
	}
	chunks := make([]string, 0, (len(value)+rawChunkBytesV1-1)/rawChunkBytesV1)
	for offset := 0; offset < len(value); offset += rawChunkBytesV1 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		end := offset + rawChunkBytesV1
		if end > len(value) {
			end = len(value)
		}
		chunks = append(chunks, base64.StdEncoding.EncodeToString(value[offset:end]))
	}
	return chunks, ctx.Err()
}

func decodeBase64ChunksV1(ctx context.Context, chunks []string) ([]byte, error) {
	if ctx == nil || chunks == nil {
		return nil, fmt.Errorf("%w: invalid base64 chunks", contract.ErrInvalid)
	}
	if len(chunks) == 0 {
		return []byte{}, ctx.Err()
	}
	maxChunks := int((hardMaxOutputRawBytesV1 + rawChunkBytesV1 - 1) / rawChunkBytesV1)
	if len(chunks) > maxChunks {
		return nil, fmt.Errorf("%w: base64 chunk count", contract.ErrLimitExceeded)
	}
	decodedLengths := make([]int, len(chunks))
	scratch := make([]byte, rawChunkBytesV1)
	var total uint64
	for index, chunk := range chunks {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if chunk == "" || len(chunk) > wireChunkBytesV1 || strings.IndexAny(chunk, " \t\r\n") >= 0 {
			return nil, fmt.Errorf("%w: non-canonical base64 chunk", contract.ErrInvalid)
		}
		decodedLen := base64.StdEncoding.DecodedLen(len(chunk))
		if decodedLen == 0 || decodedLen > rawChunkBytesV1 {
			return nil, fmt.Errorf("%w: decoded base64 size", contract.ErrLimitExceeded)
		}
		n, err := base64.StdEncoding.Strict().Decode(scratch[:decodedLen], []byte(chunk))
		if err != nil || n == 0 || n > rawChunkBytesV1 {
			return nil, fmt.Errorf("%w: invalid base64 chunk", contract.ErrInvalid)
		}
		decoded := scratch[:n]
		if index < len(chunks)-1 && (n != rawChunkBytesV1 || len(chunk) != wireChunkBytesV1) {
			return nil, fmt.Errorf("%w: short non-final base64 chunk", contract.ErrInvalid)
		}
		if base64.StdEncoding.EncodeToString(decoded) != chunk {
			return nil, fmt.Errorf("%w: non-canonical base64 encoding", contract.ErrInvalid)
		}
		if total > hardMaxOutputRawBytesV1-uint64(n) {
			return nil, fmt.Errorf("%w: decoded base64 size", contract.ErrLimitExceeded)
		}
		total += uint64(n)
		decodedLengths[index] = n
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	result := make([]byte, int(total))
	offset := 0
	for index, chunk := range chunks {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		n, err := base64.StdEncoding.Strict().Decode(result[offset:offset+decodedLengths[index]], []byte(chunk))
		if err != nil || n != decodedLengths[index] {
			return nil, fmt.Errorf("%w: base64 decode changed between validation and copy", contract.ErrConflict)
		}
		offset += n
	}
	return result, ctx.Err()
}

func bundleToWireV1(ctx context.Context, bundle OfflineContentBundleV1) (offlineContentBundleWireV1, error) {
	wire := offlineContentBundleWireV1{Items: make([]offlineContentItemWireV1, len(bundle.items)), ContentSetDigest: bundle.ContentSetDigest()}
	for i, item := range bundle.items {
		chunks, err := encodeBase64ChunksV1(ctx, item.Bytes)
		if err != nil {
			return offlineContentBundleWireV1{}, err
		}
		wire.Items[i] = offlineContentItemWireV1{Ref: item.Ref, Base64Chunks: chunks}
	}
	return wire, nil
}

func decodeBundleWireV1(ctx context.Context, op OfflineSDKOperationV1, wire offlineContentBundleWireV1, limits OfflineSDKLimitsV1) (OfflineContentBundleV1, error) {
	if wire.Items == nil {
		return OfflineContentBundleV1{}, sdkErrorV1(OfflineErrorInvalidArgumentV1, op, "content_bundle.items", "items must be present", contract.ErrInvalid)
	}
	items := make([]OfflineContentItemV1, len(wire.Items))
	for i, item := range wire.Items {
		if item.Ref.Validate() != nil || item.Ref.Length == 0 || item.Base64Chunks == nil || len(item.Base64Chunks) == 0 {
			return OfflineContentBundleV1{}, sdkErrorV1(OfflineErrorInvalidArgumentV1, op, fmt.Sprintf("content_bundle.items[%d]", i), "positive content item required", contract.ErrInvalid)
		}
		value, err := decodeBase64ChunksV1(ctx, item.Base64Chunks)
		if err != nil {
			return OfflineContentBundleV1{}, mapErrorV1(op, fmt.Sprintf("content_bundle.items[%d].base64_chunks", i), err)
		}
		items[i] = OfflineContentItemV1{Ref: item.Ref, Bytes: value}
	}
	bundle, err := newOfflineContentBundleContextV1(ctx, items, limits)
	if err != nil {
		return OfflineContentBundleV1{}, withOperationV1(err, op)
	}
	if bundle.ContentSetDigest() != wire.ContentSetDigest {
		return OfflineContentBundleV1{}, sdkErrorV1(OfflineErrorConflictV1, op, "content_bundle.content_set_digest", "content set digest mismatch", contract.ErrConflict)
	}
	return bundle, nil
}

func compiledFromWireV1(ctx context.Context, op OfflineSDKOperationV1, wire compiledBundleWireV1, limits OfflineSDKLimitsV1) (CompiledBundleV1, error) {
	bundle, err := decodeBundleWireV1(ctx, op, wire.ContentBundle, limits)
	if err != nil {
		return CompiledBundleV1{}, err
	}
	if wire.ResidualCandidateRefs == nil || wire.Authoritative == nil {
		return CompiledBundleV1{}, sdkErrorV1(OfflineErrorInvalidArgumentV1, op, "compiled.residual_candidate_refs", "residual refs must be present", contract.ErrInvalid)
	}
	return CompiledBundleV1{Manifest: wire.Manifest, Frame: wire.Frame, ContentBundle: bundle, ResidualCandidateRefs: cloneSliceV1(wire.ResidualCandidateRefs), Authoritative: *wire.Authoritative, CompileDigest: wire.CompileDigest}, nil
}

func compiledToWireV1(ctx context.Context, value CompiledBundleV1) (compiledBundleWireV1, error) {
	bundle, err := bundleToWireV1(ctx, value.ContentBundle)
	if err != nil {
		return compiledBundleWireV1{}, err
	}
	authoritative := value.Authoritative
	return compiledBundleWireV1{Manifest: cloneManifestV1(value.Manifest), Frame: cloneFrameV1(value.Frame), ContentBundle: bundle, ResidualCandidateRefs: cloneSliceV1(value.ResidualCandidateRefs), Authoritative: &authoritative, CompileDigest: value.CompileDigest}, nil
}

func compileResponseToWireV1(ctx context.Context, response CompileFrameResponseV1) (compileFrameResponseWireV1, error) {
	compiled, err := compiledToWireV1(ctx, response.Compiled)
	if err != nil {
		return compileFrameResponseWireV1{}, mapErrorV1(OfflineCompileFrameV1, "compiled.content_bundle", err)
	}
	return compileFrameResponseWireV1{Meta: response.Meta, Compiled: compiled, Diagnostics: cloneDiagnosticsV1(response.Diagnostics)}, nil
}
