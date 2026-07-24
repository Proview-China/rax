package sdk

import (
	"context"
	"fmt"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/kernel"
)

// CompareRecipesRequestV1 is an owner-local, read-only structural comparison.
// It does not make quality, compatibility, review, or publication decisions.
type CompareRecipesRequestV1 struct {
	Meta            OfflineRequestMetaV1   `json:"meta"`
	BaseRecipe      contract.ContextRecipe `json:"base_recipe"`
	CandidateRecipe contract.ContextRecipe `json:"candidate_recipe"`
	CheckedUnixNano int64                  `json:"checked_unix_nano"`
	ExpiresUnixNano int64                  `json:"expires_unix_nano"`
}

type CompareRecipesResponseV1 struct {
	Meta        OfflineResponseMetaV1              `json:"meta"`
	Comparison  contract.ContextRecipeComparisonV1 `json:"comparison"`
	Diagnostics []OfflineDiagnosticV1              `json:"diagnostics"`
	limits      OfflineSDKLimitsV1
}

func cloneRecipeComparisonV1(value contract.ContextRecipeComparisonV1) contract.ContextRecipeComparisonV1 {
	value.Changes = cloneSliceV1(value.Changes)
	for index := range value.Changes {
		if value.Changes[index].BeforeDigest != nil {
			digest := *value.Changes[index].BeforeDigest
			value.Changes[index].BeforeDigest = &digest
		}
		if value.Changes[index].AfterDigest != nil {
			digest := *value.Changes[index].AfterDigest
			value.Changes[index].AfterDigest = &digest
		}
	}
	return value
}

func cloneCompareRequestV1(value CompareRecipesRequestV1) CompareRecipesRequestV1 {
	value.BaseRecipe = cloneRecipeV1(value.BaseRecipe)
	value.CandidateRecipe = cloneRecipeV1(value.CandidateRecipe)
	return value
}

func cloneCompareResponseV1(value CompareRecipesResponseV1) CompareRecipesResponseV1 {
	value.Comparison = cloneRecipeComparisonV1(value.Comparison)
	value.Diagnostics = cloneDiagnosticsV1(value.Diagnostics)
	return value
}

func preflightCompareRecipesRequestV1(request CompareRecipesRequestV1) error {
	const op = OfflineCompareRecipesV1
	if request.BaseRecipe.Rules == nil || request.CandidateRecipe.Rules == nil {
		return sdkErrorV1(OfflineErrorInvalidArgumentV1, op, "request", "recipe rules must be present", contract.ErrInvalid)
	}
	if request.Meta.Limits.MaxRecipes < 2 {
		return sdkErrorV1(OfflineErrorLimitExceededV1, op, "meta.limits.max_recipes", "compare requires capacity for two recipes", contract.ErrLimitExceeded)
	}
	if len(request.BaseRecipe.Rules) > 64 || len(request.CandidateRecipe.Rules) > 64 {
		return sdkErrorV1(OfflineErrorLimitExceededV1, op, "recipe.rules", "recipe rule limit exceeded", contract.ErrLimitExceeded)
	}
	for path, recipe := range map[string]contract.ContextRecipe{"base_recipe": request.BaseRecipe, "candidate_recipe": request.CandidateRecipe} {
		if recipe.Budget.TotalTokens > request.Meta.Limits.MaxTotalTokens {
			return sdkErrorV1(OfflineErrorLimitExceededV1, op, path+".budget.total_tokens", "token limit exceeded", contract.ErrLimitExceeded)
		}
	}
	if request.CheckedUnixNano <= 0 || request.ExpiresUnixNano <= request.CheckedUnixNano {
		return sdkErrorV1(OfflineErrorInvalidArgumentV1, op, "lifetime", "invalid comparison lifetime", contract.ErrInvalid)
	}
	return nil
}

func compareRequestDigestV1(request CompareRecipesRequestV1, contexts ...context.Context) (contract.Digest, error) {
	meta := request.Meta
	meta.RequestDigest = ""
	return canonicalDigestV1("compare-recipes-request", struct {
		Meta            OfflineRequestMetaV1   `json:"meta"`
		BaseRecipe      contract.ContextRecipe `json:"base_recipe"`
		CandidateRecipe contract.ContextRecipe `json:"candidate_recipe"`
		CheckedUnixNano int64                  `json:"checked_unix_nano"`
		ExpiresUnixNano int64                  `json:"expires_unix_nano"`
	}{meta, cloneRecipeV1(request.BaseRecipe), cloneRecipeV1(request.CandidateRecipe), request.CheckedUnixNano, request.ExpiresUnixNano}, contexts...)
}

func validateCompareRequestDigestV1(request CompareRecipesRequestV1, contexts ...context.Context) error {
	digest, err := compareRequestDigestV1(request, contexts...)
	if err != nil {
		return mapErrorV1(OfflineCompareRecipesV1, "meta.request_digest", err)
	}
	if digest != request.Meta.RequestDigest {
		return sdkErrorV1(OfflineErrorConflictV1, OfflineCompareRecipesV1, "meta.request_digest", "request digest mismatch", contract.ErrConflict)
	}
	return nil
}

func compareResponsePrivateV1(value CompareRecipesResponseV1) any {
	return struct {
		Comparison  contract.ContextRecipeComparisonV1 `json:"comparison"`
		Diagnostics []OfflineDiagnosticV1              `json:"diagnostics"`
	}{cloneRecipeComparisonV1(value.Comparison), cloneDiagnosticsV1(value.Diagnostics)}
}

func SealCompareRecipesRequestV1(ctx context.Context, request CompareRecipesRequestV1) (CompareRecipesRequestV1, error) {
	const op = OfflineCompareRecipesV1
	if err := validateSealContextAndMetaV1(ctx, request.Meta, op); err != nil {
		return CompareRecipesRequestV1{}, err
	}
	if err := preflightCompareRecipesRequestV1(request); err != nil {
		return CompareRecipesRequestV1{}, err
	}
	request = cloneCompareRequestV1(request)
	digest, err := compareRequestDigestV1(request, ctx)
	if err != nil {
		return CompareRecipesRequestV1{}, mapErrorV1(op, "meta.request_digest", err)
	}
	if err := acceptOrSetRequestDigestV1(&request.Meta, digest, op); err != nil {
		return CompareRecipesRequestV1{}, err
	}
	return request, validateContextV1(ctx, op)
}

func EncodeCompareRecipesRequestV1(ctx context.Context, request CompareRecipesRequestV1) ([]byte, error) {
	sealed, err := SealCompareRecipesRequestV1(ctx, request)
	if err != nil {
		return nil, err
	}
	return encodeBoundedRequestV1(ctx, OfflineCompareRecipesV1, sealed.Meta, func(buffer *boundedCodecBufferV1) (uint64, error) {
		return 0, buffer.writeJSON(sealed)
	})
}

func DecodeCompareRecipesRequestV1(ctx context.Context, payload []byte) (CompareRecipesRequestV1, error) {
	var request CompareRecipesRequestV1
	if err := decodeStrictV1(ctx, OfflineCompareRecipesV1, payload, hardWire48MiBV1, &request); err != nil {
		return CompareRecipesRequestV1{}, err
	}
	if err := validateRequestMetaV1(request.Meta, OfflineCompareRecipesV1); err != nil {
		return CompareRecipesRequestV1{}, err
	}
	if uint64(len(payload)) > request.Meta.Limits.MaxWireRequestBytes {
		return CompareRecipesRequestV1{}, sdkErrorV1(OfflineErrorLimitExceededV1, OfflineCompareRecipesV1, "payload", "request wire limit exceeded", contract.ErrLimitExceeded)
	}
	if err := validateWireAccountingV1(payload, 0, request.Meta.Limits, OfflineCompareRecipesV1); err != nil {
		return CompareRecipesRequestV1{}, err
	}
	if err := preflightCompareRecipesRequestV1(request); err != nil {
		return CompareRecipesRequestV1{}, err
	}
	return cloneCompareRequestV1(request), nil
}

func CompareRecipesV1(ctx context.Context, request CompareRecipesRequestV1) (CompareRecipesResponseV1, error) {
	const op = OfflineCompareRecipesV1
	if err := validateContextV1(ctx, op); err != nil {
		return CompareRecipesResponseV1{}, err
	}
	if err := validateRequestMetaV1(request.Meta, op); err != nil {
		return CompareRecipesResponseV1{}, err
	}
	if err := preflightCompareRecipesRequestV1(request); err != nil {
		return CompareRecipesResponseV1{}, err
	}
	request = cloneCompareRequestV1(request)
	if err := validateCompareRequestDigestV1(request, ctx); err != nil {
		return CompareRecipesResponseV1{}, err
	}
	comparison, err := kernel.CompareContextRecipesV1(ctx, request.BaseRecipe, request.CandidateRecipe, request.CheckedUnixNano, request.ExpiresUnixNano)
	if err != nil {
		return CompareRecipesResponseV1{}, mapErrorV1(op, "comparison", err)
	}
	if len(comparison.Changes) > 80 {
		return CompareRecipesResponseV1{}, sdkErrorV1(OfflineErrorLimitExceededV1, op, "comparison.changes", "comparison change limit exceeded", contract.ErrLimitExceeded)
	}
	response := CompareRecipesResponseV1{
		Meta:       OfflineResponseMetaV1{ContractVersion: OfflineSDKContractVersionV1, RequestID: request.Meta.RequestID, Operation: op, RequestDigest: request.Meta.RequestDigest},
		Comparison: comparison, Diagnostics: []OfflineDiagnosticV1{}, limits: request.Meta.Limits,
	}
	response.Meta.ResultDigest, err = validateResponseResultDigestV1("compare-recipes-response", &response.Meta, compareResponsePrivateV1(response), ctx)
	if err != nil {
		return CompareRecipesResponseV1{}, mapErrorV1(op, "meta.result_digest", err)
	}
	if err := validateContextV1(ctx, op); err != nil {
		return CompareRecipesResponseV1{}, err
	}
	return cloneCompareResponseV1(response), nil
}

func validateCompareResponseV1(ctx context.Context, response CompareRecipesResponseV1) error {
	const op = OfflineCompareRecipesV1
	if err := validateContextV1(ctx, op); err != nil {
		return err
	}
	if err := validateResponseEnvelopeV1(response.Meta, response.limits, op); err != nil {
		return err
	}
	if err := validateDiagnosticsV1(response.Diagnostics, response.limits, op); err != nil {
		return err
	}
	if err := response.Comparison.Validate(); err != nil {
		return mapErrorV1(op, "comparison", err)
	}
	if err := ensureCompareResponseBoundV1(response); err != nil {
		return mapErrorV1(op, "comparison.changes", err)
	}
	digest, err := response.Comparison.DigestValue()
	if err != nil {
		return mapErrorV1(op, "comparison.comparison_digest", err)
	}
	if digest != response.Comparison.ComparisonDigest {
		return sdkErrorV1(OfflineErrorConflictV1, op, "comparison.comparison_digest", "comparison digest mismatch", contract.ErrConflict)
	}
	want, err := validateResponseResultDigestV1("compare-recipes-response", &response.Meta, compareResponsePrivateV1(response), ctx)
	if err != nil {
		return mapErrorV1(op, "meta.result_digest", err)
	}
	if want != response.Meta.ResultDigest {
		return sdkErrorV1(OfflineErrorConflictV1, op, "meta.result_digest", "result digest mismatch", contract.ErrConflict)
	}
	return nil
}

func EncodeCompareRecipesResponseV1(ctx context.Context, response CompareRecipesResponseV1) ([]byte, error) {
	if err := validateCompareResponseV1(ctx, response); err != nil {
		return nil, err
	}
	return encodeResponseV1(ctx, OfflineCompareRecipesV1, response, response.Meta, response.limits)
}

func ensureCompareResponseBoundV1(response CompareRecipesResponseV1) error {
	if len(response.Comparison.Changes) > 80 {
		return fmt.Errorf("%w: comparison change bound", contract.ErrLimitExceeded)
	}
	return nil
}
