package sdk

import (
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
)

func preflightValidateRecipeRequestV1(request ValidateRecipeRequestV1) error {
	const op = OfflineValidateRecipeV1
	if request.Candidates == nil || request.Recipe.Rules == nil {
		return sdkErrorV1(OfflineErrorInvalidArgumentV1, op, "request", "required slices must be present", contract.ErrInvalid)
	}
	if len(request.Candidates) > int(request.Meta.Limits.MaxCandidates) {
		return sdkErrorV1(OfflineErrorLimitExceededV1, op, "candidates", "candidate limit exceeded", contract.ErrLimitExceeded)
	}
	if len(request.Recipe.Rules) > 64 {
		return sdkErrorV1(OfflineErrorLimitExceededV1, op, "recipe.rules", "recipe rule limit exceeded", contract.ErrLimitExceeded)
	}
	if request.Recipe.Budget.TotalTokens > request.Meta.Limits.MaxTotalTokens {
		return sdkErrorV1(OfflineErrorLimitExceededV1, op, "recipe.budget.total_tokens", "token limit exceeded", contract.ErrLimitExceeded)
	}
	return nil
}

func preflightCompileFrameRequestV1(request CompileFrameRequestV1) error {
	const op = OfflineCompileFrameV1
	if err := preflightValidateRecipeRequestV1(ValidateRecipeRequestV1{Meta: request.Meta, Recipe: request.Recipe, Candidates: request.Candidates}); err != nil {
		return withOperationV1(err, op)
	}
	return preflightBundleV1(request.InputBundle, request.Meta.Limits.MaxInputContentItems, request.Meta.Limits.MaxInputContentItemBytes, request.Meta.Limits.MaxInputRawBytes, op, "input_bundle")
}

func preflightPreviewFrameRequestV1(request PreviewFrameRequestV1) error {
	const op = OfflinePreviewFrameV1
	if request.Compiled.Manifest.Decisions == nil || request.Compiled.Manifest.Fragments == nil || request.Compiled.ResidualCandidateRefs == nil {
		return sdkErrorV1(OfflineErrorInvalidArgumentV1, op, "compiled", "required compiled slices must be present", contract.ErrInvalid)
	}
	if len(request.Compiled.Manifest.Decisions) > int(request.Meta.Limits.MaxCandidates) || len(request.Compiled.Manifest.Fragments) > int(request.Meta.Limits.MaxCandidates) || len(request.Compiled.ResidualCandidateRefs) > int(request.Meta.Limits.MaxCandidates) {
		return sdkErrorV1(OfflineErrorLimitExceededV1, op, "compiled", "compiled candidate limit exceeded", contract.ErrLimitExceeded)
	}
	if request.Compiled.Manifest.TotalTokens > request.Meta.Limits.MaxTotalTokens {
		return sdkErrorV1(OfflineErrorLimitExceededV1, op, "compiled.manifest.total_tokens", "token limit exceeded", contract.ErrLimitExceeded)
	}
	maxRaw := request.Meta.Limits.MaxOutputRawBytes
	if maxRaw > hardMaxCompileOutputRawBytesV1 {
		maxRaw = hardMaxCompileOutputRawBytesV1
	}
	return preflightBundleV1(request.Compiled.ContentBundle, request.Meta.Limits.MaxOutputContentItems, maxRaw, maxRaw, op, "compiled.content_bundle")
}

func preflightInspectFrameExactRequestV1(request InspectFrameExactRequestV1) error {
	const op = OfflineInspectFrameExactV1
	if request.Manifest.Decisions == nil || request.Manifest.Fragments == nil {
		return sdkErrorV1(OfflineErrorInvalidArgumentV1, op, "manifest", "required manifest slices must be present", contract.ErrInvalid)
	}
	if len(request.Manifest.Decisions) > int(request.Meta.Limits.MaxCandidates) || len(request.Manifest.Fragments) > int(request.Meta.Limits.MaxCandidates) {
		return sdkErrorV1(OfflineErrorLimitExceededV1, op, "manifest", "manifest candidate limit exceeded", contract.ErrLimitExceeded)
	}
	if request.Manifest.TotalTokens > request.Meta.Limits.MaxTotalTokens {
		return sdkErrorV1(OfflineErrorLimitExceededV1, op, "manifest.total_tokens", "token limit exceeded", contract.ErrLimitExceeded)
	}
	maxRaw := request.Meta.Limits.MaxOutputRawBytes
	if maxRaw > hardMaxCompileOutputRawBytesV1 {
		maxRaw = hardMaxCompileOutputRawBytesV1
	}
	return preflightBundleV1(request.ContentBundle, request.Meta.Limits.MaxOutputContentItems, maxRaw, maxRaw, op, "content_bundle")
}

func preflightBundleV1(bundle OfflineContentBundleV1, maxItems uint32, maxItemBytes, maxRawBytes uint64, op OfflineSDKOperationV1, path string) error {
	if bundle.items == nil {
		return sdkErrorV1(OfflineErrorInvalidArgumentV1, op, path, "content items must be present", contract.ErrInvalid)
	}
	if len(bundle.items) > int(maxItems) {
		return sdkErrorV1(OfflineErrorLimitExceededV1, op, path, "content item limit exceeded", contract.ErrLimitExceeded)
	}
	var total uint64
	for index := range bundle.items {
		length := uint64(len(bundle.items[index].Bytes))
		if length == 0 || bundle.items[index].Ref.Length != length {
			return sdkErrorV1(OfflineErrorInvalidArgumentV1, op, path, "content length mismatch", contract.ErrInvalid)
		}
		if length > maxItemBytes || length > maxRawBytes || total > maxRawBytes-length {
			return sdkErrorV1(OfflineErrorLimitExceededV1, op, path, "content byte limit exceeded", contract.ErrLimitExceeded)
		}
		total += length
	}
	return nil
}
