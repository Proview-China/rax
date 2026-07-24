package sdk

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
)

const canonicalDomainV1 = "praxis.context.offline-sdk"

func canonicalDigestV1(discriminator string, body any, contexts ...context.Context) (contract.Digest, error) {
	ctx := context.Background()
	if len(contexts) > 0 {
		ctx = contexts[0]
	}
	hash := sha256.New()
	err := writeJSONContextV1(ctx, hash, struct {
		Domain        string `json:"domain"`
		Version       string `json:"version"`
		Discriminator string `json:"discriminator"`
		Body          any    `json:"body"`
	}{Domain: canonicalDomainV1, Version: "v1", Discriminator: discriminator, Body: body})
	if err != nil {
		return "", err
	}
	return contract.Digest("sha256:" + hex.EncodeToString(hash.Sum(nil))), nil
}

func validateRequestMetaV1(meta OfflineRequestMetaV1, expected OfflineSDKOperationV1) error {
	if err := validateRequestMetaBaseV1(meta, expected); err != nil {
		return err
	}
	if meta.RequestDigest.Validate() != nil {
		return sdkErrorV1(OfflineErrorInvalidArgumentV1, expected, "meta.request_digest", "invalid request digest", contract.ErrInvalid)
	}
	return nil
}

func validateRequestMetaBaseV1(meta OfflineRequestMetaV1, expected OfflineSDKOperationV1) error {
	if meta.ContractVersion != OfflineSDKContractVersionV1 || meta.RequestID == "" {
		return sdkErrorV1(OfflineErrorInvalidArgumentV1, expected, "meta", "invalid request metadata", contract.ErrInvalid)
	}
	if _, _, ok := wireCapsV1(meta.Operation); !ok {
		return sdkErrorV1(OfflineErrorUnsupportedV1, expected, "meta.operation", "unsupported operation", contract.ErrUnsupported)
	}
	if meta.Operation != expected {
		return sdkErrorV1(OfflineErrorInvalidArgumentV1, expected, "meta.operation", "operation does not match entrypoint", contract.ErrInvalid)
	}
	if err := meta.Limits.validate(expected); err != nil {
		return sdkErrorV1(OfflineErrorLimitExceededV1, expected, "meta.limits", err.Error(), contract.ErrLimitExceeded)
	}
	return nil
}

func validateRecipeRequestDigestValueV1(request ValidateRecipeRequestV1, contexts ...context.Context) (contract.Digest, error) {
	meta := request.Meta
	meta.RequestDigest = ""
	return canonicalDigestV1("validate-recipe-request", struct {
		Meta       OfflineRequestMetaV1        `json:"meta"`
		Recipe     contract.ContextRecipe      `json:"recipe"`
		Candidates []contract.ContextCandidate `json:"candidates"`
	}{meta, cloneRecipeV1(request.Recipe), cloneCandidatesV1(request.Candidates)}, contexts...)
}

func validateRecipeRequestDigestV1(request ValidateRecipeRequestV1, contexts ...context.Context) error {
	digest, err := validateRecipeRequestDigestValueV1(request, contexts...)
	if err != nil {
		return mapErrorV1(OfflineValidateRecipeV1, "meta.request_digest", err)
	}
	if digest != request.Meta.RequestDigest {
		return sdkErrorV1(OfflineErrorConflictV1, OfflineValidateRecipeV1, "meta.request_digest", "request digest mismatch", contract.ErrConflict)
	}
	return nil
}

func compileRequestDigestV1(request CompileFrameRequestV1, contexts ...context.Context) (contract.Digest, error) {
	meta := request.Meta
	meta.RequestDigest = ""
	return canonicalDigestV1("compile-frame-request", struct {
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
		InputBundle       offlineBundleClosureV1      `json:"input_bundle"`
	}{meta, request.AttemptID, request.ManifestID, request.FrameID, request.GenerationID, request.GenerationOrdinal,
		cloneRecipeV1(request.Recipe), request.Execution, cloneCandidatesV1(request.Candidates), cloneFactRefPtrV1(request.ParentFrame),
		request.CreatedUnixNano, request.ExpiresUnixNano, request.InputBundle.closureV1()}, contexts...)
}

func validateCompileRequestDigestV1(request CompileFrameRequestV1, contexts ...context.Context) error {
	digest, err := compileRequestDigestV1(request, contexts...)
	if err != nil {
		return mapErrorV1(OfflineCompileFrameV1, "meta.request_digest", err)
	}
	if digest != request.Meta.RequestDigest {
		return sdkErrorV1(OfflineErrorConflictV1, OfflineCompileFrameV1, "meta.request_digest", "request digest mismatch", contract.ErrConflict)
	}
	return nil
}

func compileDigestV1(compiled CompiledBundleV1, contexts ...context.Context) (contract.Digest, error) {
	return canonicalDigestV1("compiled-bundle", struct {
		Manifest              contract.ContextManifest `json:"manifest"`
		Frame                 contract.ContextFrame    `json:"frame"`
		Bundle                offlineBundleClosureV1   `json:"bundle_closure"`
		ResidualCandidateRefs []contract.FactRef       `json:"residual_candidate_refs"`
		Authoritative         bool                     `json:"authoritative"`
	}{cloneManifestV1(compiled.Manifest), cloneFrameV1(compiled.Frame), compiled.ContentBundle.closureV1(), cloneSliceV1(compiled.ResidualCandidateRefs), false}, contexts...)
}

func canonicalDiagnosticsV1(values []OfflineDiagnosticV1, limits OfflineSDKLimitsV1) []OfflineDiagnosticV1 {
	result := cloneSliceV1(values)
	sort.Slice(result, func(i, j int) bool {
		left, right := result[i], result[j]
		return fmt.Sprintf("%s\x00%s\x00%s\x00%s\x00%s\x00%s", left.Severity, left.Code, left.ObjectKind, left.ObjectID, left.FieldPath, left.Message) <
			fmt.Sprintf("%s\x00%s\x00%s\x00%s\x00%s\x00%s", right.Severity, right.Code, right.ObjectKind, right.ObjectID, right.FieldPath, right.Message)
	})
	if len(result) <= int(limits.MaxDiagnostics) {
		return result
	}
	keep := int(limits.MaxDiagnostics) - 1
	result = result[:keep]
	return append(result, OfflineDiagnosticV1{Code: "diagnostics_truncated", Severity: "warning", ObjectKind: "request", ObjectID: "diagnostics", FieldPath: "diagnostics", Message: "diagnostics truncated by request limit"})
}
