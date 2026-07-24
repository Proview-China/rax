package sdk

import (
	"context"
	"errors"
	"fmt"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/kernel"
)

func ValidateRecipeV1(ctx context.Context, request ValidateRecipeRequestV1) (ValidateRecipeResponseV1, error) {
	const op = OfflineValidateRecipeV1
	if err := validateContextV1(ctx, op); err != nil {
		return ValidateRecipeResponseV1{}, err
	}
	if err := validateRequestMetaV1(request.Meta, op); err != nil {
		return ValidateRecipeResponseV1{}, err
	}
	if err := preflightValidateRecipeRequestV1(request); err != nil {
		return ValidateRecipeResponseV1{}, err
	}
	request = cloneValidateRequestV1(request)
	if err := validateRecipeRequestDigestV1(request, ctx); err != nil {
		return ValidateRecipeResponseV1{}, err
	}
	diagnostics := make([]OfflineDiagnosticV1, 0)
	valid := true
	var recipeRef *contract.FactRef
	if err := request.Recipe.Validate(); err != nil {
		valid = false
		diagnostics = append(diagnostics, diagnosticV1("invalid_recipe", "error", "recipe", request.Recipe.ID, "recipe", "recipe validation failed"))
	} else {
		digest, err := digestJSONContextV1(ctx, request.Recipe)
		if err != nil {
			return ValidateRecipeResponseV1{}, mapErrorV1(op, "recipe", err)
		}
		ref := contract.FactRef{ID: request.Recipe.ID, Revision: request.Recipe.Revision, Digest: digest}
		recipeRef = &ref
	}
	refs := make([]contract.FactRef, 0, len(request.Candidates))
	for index, candidate := range request.Candidates {
		if err := validateContextV1(ctx, op); err != nil {
			return ValidateRecipeResponseV1{}, err
		}
		if candidate.Kind == contract.FragmentKind("knowledge_reference") {
			return ValidateRecipeResponseV1{}, sdkErrorV1(OfflineErrorUnsupportedV1, op, fmt.Sprintf("candidates[%d].kind", index), "knowledge_reference is not part of offline sdk v1", contract.ErrUnsupported)
		}
		if err := candidate.Validate(); err != nil {
			valid = false
			diagnostics = append(diagnostics, diagnosticV1("invalid_candidate", "error", "candidate", candidate.ID, fmt.Sprintf("candidates[%d]", index), "candidate validation failed"))
			continue
		}
		digest, err := digestJSONContextV1(ctx, candidate)
		if err != nil {
			return ValidateRecipeResponseV1{}, mapErrorV1(op, fmt.Sprintf("candidates[%d]", index), err)
		}
		refs = append(refs, contract.FactRef{ID: candidate.ID, Revision: candidate.Revision, Digest: digest})
	}
	diagnostics = canonicalDiagnosticsV1(diagnostics, request.Meta.Limits)
	reportDigest, err := canonicalDigestV1("validate-report", struct {
		Valid         bool                  `json:"valid"`
		RecipeRef     *contract.FactRef     `json:"recipe_ref,omitempty"`
		CandidateRefs []contract.FactRef    `json:"candidate_refs"`
		Diagnostics   []OfflineDiagnosticV1 `json:"diagnostics"`
	}{valid, recipeRef, refs, diagnostics}, ctx)
	if err != nil {
		return ValidateRecipeResponseV1{}, mapErrorV1(op, "report_digest", err)
	}
	response := ValidateRecipeResponseV1{
		Meta:  OfflineResponseMetaV1{ContractVersion: OfflineSDKContractVersionV1, RequestID: request.Meta.RequestID, Operation: op, RequestDigest: request.Meta.RequestDigest},
		Valid: valid, RecipeRef: recipeRef, CandidateRefs: refs, Diagnostics: diagnostics, ReportDigest: reportDigest, limits: request.Meta.Limits,
	}
	resultDigest, err := validateResponseResultDigestV1("validate-recipe-response", &response.Meta, validateResponsePrivateV1(response), ctx)
	if err != nil {
		return ValidateRecipeResponseV1{}, mapErrorV1(op, "meta.result_digest", err)
	}
	response.Meta.ResultDigest = resultDigest
	if err := validateContextV1(ctx, op); err != nil {
		return ValidateRecipeResponseV1{}, err
	}
	return cloneValidateResponseV1(response), nil
}

func CompileFrameV1(ctx context.Context, request CompileFrameRequestV1) (CompileFrameResponseV1, error) {
	const op = OfflineCompileFrameV1
	if err := validateContextV1(ctx, op); err != nil {
		return CompileFrameResponseV1{}, err
	}
	if err := validateRequestMetaV1(request.Meta, op); err != nil {
		return CompileFrameResponseV1{}, err
	}
	if err := preflightCompileFrameRequestV1(request); err != nil {
		return CompileFrameResponseV1{}, err
	}
	request, cloneErr := cloneCompileRequestContextV1(ctx, request)
	if cloneErr != nil {
		return CompileFrameResponseV1{}, mapErrorV1(op, "request", cloneErr)
	}
	if err := request.InputBundle.validateContextV1(ctx, request.Meta.Limits); err != nil {
		return CompileFrameResponseV1{}, withOperationV1(err, op)
	}
	if err := validateInputRawLimitsV1(request.InputBundle, request.Meta.Limits); err != nil {
		return CompileFrameResponseV1{}, err
	}
	if err := validateCompileRequestDigestV1(request, ctx); err != nil {
		return CompileFrameResponseV1{}, err
	}
	if err := preflightRequiredContentV1(request.Recipe, request.Candidates, request.InputBundle); err != nil {
		return CompileFrameResponseV1{}, mapErrorV1(op, "input_bundle", err)
	}
	workspace, err := newOfflineWorkspaceV1(ctx, request.InputBundle)
	if err != nil {
		return CompileFrameResponseV1{}, mapErrorV1(op, "workspace", err)
	}
	defer workspace.Destroy()
	workLimits := compileWorkLimitsV1(request.Meta.Limits)
	if err := workspace.Begin(ctx, workLimits); err != nil {
		return CompileFrameResponseV1{}, mapErrorV1(op, "workspace.begin", err)
	}
	defer workspace.Abort()
	result, err := kernel.CompileStagedV1(ctx, workspace, kernel.CompileRequest{
		AttemptID: request.AttemptID, ManifestID: request.ManifestID, FrameID: request.FrameID,
		GenerationID: request.GenerationID, Generation: request.GenerationOrdinal,
		Recipe: request.Recipe, Execution: request.Execution, Candidates: request.Candidates,
		ParentFrame: request.ParentFrame, CreatedUnixNano: request.CreatedUnixNano, ExpiresUnixNano: request.ExpiresUnixNano,
	}, workLimits)
	if err != nil {
		return CompileFrameResponseV1{}, mapKernelCompileErrorV1(op, err)
	}
	seal, err := workspace.Seal(ctx)
	if err != nil {
		return CompileFrameResponseV1{}, mapErrorV1(op, "workspace.seal", err)
	}
	bundle, err := workspace.Export(ctx, seal, request.Meta.Limits)
	if err != nil {
		return CompileFrameResponseV1{}, withOperationV1(err, op)
	}
	residuals := residualCandidateRefsV1(result.Manifest)
	compiled := CompiledBundleV1{Manifest: result.Manifest, Frame: result.Frame, ContentBundle: bundle, ResidualCandidateRefs: residuals, Authoritative: false}
	compiled.CompileDigest, err = compileDigestV1(compiled, ctx)
	if err != nil {
		return CompileFrameResponseV1{}, mapErrorV1(op, "compiled.compile_digest", err)
	}
	response := CompileFrameResponseV1{
		Meta:     OfflineResponseMetaV1{ContractVersion: OfflineSDKContractVersionV1, RequestID: request.Meta.RequestID, Operation: op, RequestDigest: request.Meta.RequestDigest},
		Compiled: compiled, Diagnostics: []OfflineDiagnosticV1{}, limits: request.Meta.Limits,
	}
	resultDigest, err := validateResponseResultDigestV1("compile-frame-response", &response.Meta, responsePrivateV1(response), ctx)
	if err != nil {
		return CompileFrameResponseV1{}, mapErrorV1(op, "meta.result_digest", err)
	}
	response.Meta.ResultDigest = resultDigest
	if err := validateContextV1(ctx, op); err != nil {
		return CompileFrameResponseV1{}, err
	}
	clonedResponse, cloneErr := cloneCompileResponseContextV1(ctx, response)
	if cloneErr != nil {
		return CompileFrameResponseV1{}, mapErrorV1(op, "response", cloneErr)
	}
	return clonedResponse, nil
}

func PreviewFrameV1(ctx context.Context, request PreviewFrameRequestV1) (PreviewFrameResponseV1, error) {
	const op = OfflinePreviewFrameV1
	if err := validateContextV1(ctx, op); err != nil {
		return PreviewFrameResponseV1{}, err
	}
	if err := validateRequestMetaV1(request.Meta, op); err != nil {
		return PreviewFrameResponseV1{}, err
	}
	if err := preflightPreviewFrameRequestV1(request); err != nil {
		return PreviewFrameResponseV1{}, err
	}
	request, cloneErr := clonePreviewRequestContextV1(ctx, request)
	if cloneErr != nil {
		return PreviewFrameResponseV1{}, mapErrorV1(op, "request", cloneErr)
	}
	if err := request.Compiled.ContentBundle.validateContextV1(ctx, request.Meta.Limits); err != nil {
		return PreviewFrameResponseV1{}, withOperationV1(err, op)
	}
	if err := validateCompiledRawLimitsV1(request.Compiled.ContentBundle, request.Meta.Limits, op); err != nil {
		return PreviewFrameResponseV1{}, err
	}
	if request.Compiled.Authoritative {
		return PreviewFrameResponseV1{}, sdkErrorV1(OfflineErrorConflictV1, op, "compiled.authoritative", "offline compiled bundle must be non-authoritative", contract.ErrConflict)
	}
	digest, err := compileDigestV1(request.Compiled, ctx)
	if err != nil {
		return PreviewFrameResponseV1{}, mapErrorV1(op, "expected_compile_digest", err)
	}
	if digest != request.Compiled.CompileDigest || digest != request.ExpectedCompileDigest {
		return PreviewFrameResponseV1{}, sdkErrorV1(OfflineErrorConflictV1, op, "expected_compile_digest", "compile digest mismatch", contract.ErrConflict)
	}
	if err := validatePreviewRequestDigestV1(request, ctx); err != nil {
		return PreviewFrameResponseV1{}, err
	}
	if request.CheckedUnixNano < request.Compiled.Frame.CreatedUnixNano || request.CheckedUnixNano >= request.Compiled.Frame.ExpiresUnixNano {
		return PreviewFrameResponseV1{}, sdkErrorV1(OfflineErrorExpiredV1, op, "checked_unix_nano", "frame is outside its lifetime", contract.ErrExpired)
	}
	if err := kernel.InspectFrameStagedV1(ctx, bundleStoreV1{request.Compiled.ContentBundle}, request.Compiled.Manifest, request.Compiled.Frame, inspectWorkLimitsV1(request.Meta.Limits)); err != nil {
		return PreviewFrameResponseV1{}, mapErrorV1(op, "compiled", err)
	}
	fragments := make([]FragmentPreviewV1, len(request.Compiled.Manifest.Fragments))
	for i, fragment := range request.Compiled.Manifest.Fragments {
		fragments[i] = FragmentPreviewV1{Position: fragment.Position, CandidateRef: fragment.CandidateRef, Kind: fragment.Kind, Region: fragment.Region, ContentRef: fragment.Content, Tokens: fragment.Tokens}
	}
	manifestDigest, err := digestJSONContextV1(ctx, request.Compiled.Manifest)
	if err != nil {
		return PreviewFrameResponseV1{}, mapErrorV1(op, "manifest", err)
	}
	frameDigest, err := digestJSONContextV1(ctx, request.Compiled.Frame)
	if err != nil {
		return PreviewFrameResponseV1{}, mapErrorV1(op, "frame", err)
	}
	response := PreviewFrameResponseV1{
		Meta:               OfflineResponseMetaV1{ContractVersion: OfflineSDKContractVersionV1, RequestID: request.Meta.RequestID, Operation: op, RequestDigest: request.Meta.RequestDigest},
		AdmissionDecisions: append([]contract.AdmissionDecision(nil), request.Compiled.Manifest.Decisions...), Fragments: fragments,
		StableTokens: request.Compiled.Manifest.StableTokens, SemiStableTokens: request.Compiled.Manifest.SemiStableTokens,
		DynamicTokens: request.Compiled.Manifest.DynamicTokens, TotalTokens: request.Compiled.Manifest.TotalTokens,
		StablePrefixRef: request.Compiled.Frame.StablePrefix, SemiStableRef: cloneContentRefPtrV1(request.Compiled.Frame.SemiStable),
		DynamicTailRef: request.Compiled.Frame.DynamicTail, RenderedRef: request.Compiled.Frame.Rendered,
		SourceSetDigest: request.Compiled.Frame.SourceSetDigest, RecipeRef: request.Compiled.Manifest.RecipeRef,
		ManifestRef:     contract.FactRef{ID: request.Compiled.Manifest.ID, Revision: request.Compiled.Manifest.Revision, Digest: manifestDigest},
		FrameRef:        contract.FactRef{ID: request.Compiled.Frame.ID, Revision: request.Compiled.Frame.Revision, Digest: frameDigest},
		ExpiresUnixNano: request.Compiled.Frame.ExpiresUnixNano, Diagnostics: []OfflineDiagnosticV1{}, limits: request.Meta.Limits,
	}
	response.PreviewDigest, err = previewDigestV1(response, ctx)
	if err != nil {
		return PreviewFrameResponseV1{}, mapErrorV1(op, "preview_digest", err)
	}
	response.Meta.ResultDigest, err = validateResponseResultDigestV1("preview-frame-response", &response.Meta, previewResponsePrivateV1(response), ctx)
	if err != nil {
		return PreviewFrameResponseV1{}, mapErrorV1(op, "meta.result_digest", err)
	}
	if err := validateContextV1(ctx, op); err != nil {
		return PreviewFrameResponseV1{}, err
	}
	return clonePreviewResponseV1(response), nil
}

func InspectFrameExactV1(ctx context.Context, request InspectFrameExactRequestV1) (InspectFrameExactResponseV1, error) {
	const op = OfflineInspectFrameExactV1
	if err := validateContextV1(ctx, op); err != nil {
		return InspectFrameExactResponseV1{}, err
	}
	if err := validateRequestMetaV1(request.Meta, op); err != nil {
		return InspectFrameExactResponseV1{}, err
	}
	if err := preflightInspectFrameExactRequestV1(request); err != nil {
		return InspectFrameExactResponseV1{}, err
	}
	request, cloneErr := cloneInspectRequestContextV1(ctx, request)
	if cloneErr != nil {
		return InspectFrameExactResponseV1{}, mapErrorV1(op, "request", cloneErr)
	}
	if err := request.ContentBundle.validateContextV1(ctx, request.Meta.Limits); err != nil {
		return InspectFrameExactResponseV1{}, withOperationV1(err, op)
	}
	if err := validateCompiledRawLimitsV1(request.ContentBundle, request.Meta.Limits, op); err != nil {
		return InspectFrameExactResponseV1{}, err
	}
	if err := validateInspectRequestDigestV1(request, ctx); err != nil {
		return InspectFrameExactResponseV1{}, err
	}
	manifestDigest, err := digestJSONContextV1(ctx, request.Manifest)
	if err != nil {
		return InspectFrameExactResponseV1{}, mapErrorV1(op, "manifest", err)
	}
	frameDigest, err := digestJSONContextV1(ctx, request.Frame)
	if err != nil {
		return InspectFrameExactResponseV1{}, mapErrorV1(op, "frame", err)
	}
	manifestRef := contract.FactRef{ID: request.Manifest.ID, Revision: request.Manifest.Revision, Digest: manifestDigest}
	frameRef := contract.FactRef{ID: request.Frame.ID, Revision: request.Frame.Revision, Digest: frameDigest}
	if manifestRef != request.ExpectedManifestRef || frameRef != request.ExpectedFrameRef {
		return InspectFrameExactResponseV1{}, sdkErrorV1(OfflineErrorConflictV1, op, "expected_refs", "manifest or frame ref mismatch", contract.ErrConflict)
	}
	rebuiltCompile := CompiledBundleV1{Manifest: request.Manifest, Frame: request.Frame, ContentBundle: request.ContentBundle, ResidualCandidateRefs: residualCandidateRefsV1(request.Manifest), Authoritative: false}
	rebuiltCompile.CompileDigest, err = compileDigestV1(rebuiltCompile, ctx)
	if err != nil {
		return InspectFrameExactResponseV1{}, mapErrorV1(op, "expected_compile_digest", err)
	}
	if rebuiltCompile.CompileDigest != request.ExpectedCompileDigest {
		return InspectFrameExactResponseV1{}, sdkErrorV1(OfflineErrorConflictV1, op, "expected_compile_digest", "compile digest mismatch", contract.ErrConflict)
	}
	if request.CheckedUnixNano < request.Frame.CreatedUnixNano || request.CheckedUnixNano >= request.Frame.ExpiresUnixNano {
		return InspectFrameExactResponseV1{}, sdkErrorV1(OfflineErrorExpiredV1, op, "checked_unix_nano", "frame is outside its lifetime", contract.ErrExpired)
	}
	if err := kernel.InspectFrameStagedV1(ctx, bundleStoreV1{request.ContentBundle}, request.Manifest, request.Frame, inspectWorkLimitsV1(request.Meta.Limits)); err != nil {
		return InspectFrameExactResponseV1{}, mapErrorV1(op, "frame", err)
	}
	response := InspectFrameExactResponseV1{
		Meta:  OfflineResponseMetaV1{ContractVersion: OfflineSDKContractVersionV1, RequestID: request.Meta.RequestID, Operation: op, RequestDigest: request.Meta.RequestDigest},
		Exact: true, ManifestRef: manifestRef, FrameRef: frameRef, ContentSetDigest: request.ContentBundle.ContentSetDigest(),
		CheckedUnixNano: request.CheckedUnixNano, ExpiresUnixNano: request.Frame.ExpiresUnixNano, Diagnostics: []OfflineDiagnosticV1{}, limits: request.Meta.Limits,
	}
	response.InspectionDigest, err = inspectionDigestV1(response, ctx)
	if err != nil {
		return InspectFrameExactResponseV1{}, mapErrorV1(op, "inspection_digest", err)
	}
	response.Meta.ResultDigest, err = validateResponseResultDigestV1("inspect-frame-exact-response", &response.Meta, inspectResponsePrivateV1(response), ctx)
	if err != nil {
		return InspectFrameExactResponseV1{}, mapErrorV1(op, "meta.result_digest", err)
	}
	if err := validateContextV1(ctx, op); err != nil {
		return InspectFrameExactResponseV1{}, err
	}
	return cloneInspectResponseV1(response), nil
}

func validateContextV1(ctx context.Context, op OfflineSDKOperationV1) error {
	if ctx == nil {
		return sdkErrorV1(OfflineErrorInvalidArgumentV1, op, "context", "nil context", contract.ErrInvalid)
	}
	return mapErrorV1(op, "context", ctx.Err())
}

func diagnosticV1(code, severity, kind, id, path, message string) OfflineDiagnosticV1 {
	return OfflineDiagnosticV1{Code: code, Severity: severity, ObjectKind: kind, ObjectID: id, FieldPath: path, Message: message}
}

func withOperationV1(err error, op OfflineSDKOperationV1) error {
	var sdkErr *OfflineSDKErrorV1
	if errors.As(err, &sdkErr) {
		copy := *sdkErr
		copy.Operation = op
		return &copy
	}
	return mapErrorV1(op, "", err)
}

func mapKernelCompileErrorV1(op OfflineSDKOperationV1, err error) error {
	if errors.Is(err, contract.ErrUnknown) {
		return sdkErrorV1(OfflineErrorInternalFailureV1, op, "compile", "unexpected owner helper outcome", err)
	}
	return mapErrorV1(op, "compile", err)
}

func compileWorkLimitsV1(limits OfflineSDKLimitsV1) kernel.CompileWorkLimitsV1 {
	generated := limits.MaxGeneratedRawBytes
	if generated > hardMaxCompileGeneratedBytesV1 {
		generated = hardMaxCompileGeneratedBytesV1
	}
	output := limits.MaxOutputRawBytes
	if output > hardMaxCompileOutputRawBytesV1 {
		output = hardMaxCompileOutputRawBytesV1
	}
	return kernel.CompileWorkLimitsV1{
		MaxCandidates: limits.MaxCandidates, MaxInputContentItems: limits.MaxInputContentItems,
		MaxInputContentItemBytes: limits.MaxInputContentItemBytes, MaxInputRawBytes: limits.MaxInputRawBytes,
		MaxGeneratedContentItems: limits.MaxGeneratedContentItems, MaxGeneratedRawBytes: generated,
		MaxOutputContentItems: limits.MaxOutputContentItems, MaxOutputRawBytes: output,
		MaxTotalTokens: limits.MaxTotalTokens, StreamChunkBytes: kernel.StagedStreamChunkBytesV1, CloneChunkBytes: kernel.StagedCloneChunkBytesV1,
	}
}

func inspectWorkLimitsV1(limits OfflineSDKLimitsV1) kernel.InspectWorkLimitsV1 {
	maxRaw := limits.MaxOutputRawBytes
	if maxRaw > hardMaxCompileOutputRawBytesV1 {
		maxRaw = hardMaxCompileOutputRawBytesV1
	}
	return kernel.InspectWorkLimitsV1{MaxFragments: limits.MaxCandidates, MaxContentItems: limits.MaxOutputContentItems, MaxContentItemBytes: maxRaw, MaxRawBytes: maxRaw, StreamChunkBytes: kernel.StagedStreamChunkBytesV1, CloneChunkBytes: kernel.StagedCloneChunkBytesV1}
}

func validateInputRawLimitsV1(bundle OfflineContentBundleV1, limits OfflineSDKLimitsV1) error {
	var total uint64
	for _, item := range bundle.items {
		if item.Ref.Length > limits.MaxInputContentItemBytes {
			return sdkErrorV1(OfflineErrorLimitExceededV1, OfflineCompileFrameV1, "input_bundle", "input content item limit exceeded", contract.ErrLimitExceeded)
		}
		if ^uint64(0)-total < item.Ref.Length {
			return sdkErrorV1(OfflineErrorLimitExceededV1, OfflineCompileFrameV1, "input_bundle", "input bytes overflow", contract.ErrLimitExceeded)
		}
		total += item.Ref.Length
	}
	if total > limits.MaxInputRawBytes || len(bundle.items) > int(limits.MaxInputContentItems) {
		return sdkErrorV1(OfflineErrorLimitExceededV1, OfflineCompileFrameV1, "input_bundle", "input bundle limit exceeded", contract.ErrLimitExceeded)
	}
	return nil
}

func validateCompiledRawLimitsV1(bundle OfflineContentBundleV1, limits OfflineSDKLimitsV1, op OfflineSDKOperationV1) error {
	maxRaw := limits.MaxOutputRawBytes
	if maxRaw > hardMaxCompileOutputRawBytesV1 {
		maxRaw = hardMaxCompileOutputRawBytesV1
	}
	var total uint64
	for _, item := range bundle.items {
		if item.Ref.Length > maxRaw || total > maxRaw-item.Ref.Length {
			return sdkErrorV1(OfflineErrorLimitExceededV1, op, "content_bundle", "compiled bundle raw limit exceeded", contract.ErrLimitExceeded)
		}
		total += item.Ref.Length
	}
	if len(bundle.items) > int(limits.MaxOutputContentItems) {
		return sdkErrorV1(OfflineErrorLimitExceededV1, op, "content_bundle", "compiled bundle item limit exceeded", contract.ErrLimitExceeded)
	}
	return nil
}

func preflightRequiredContentV1(recipe contract.ContextRecipe, candidates []contract.ContextCandidate, bundle OfflineContentBundleV1) error {
	sorted := contract.StableSortCandidates(candidates, recipe)
	selected := make(map[contract.FragmentKind]bool)
	for _, candidate := range sorted {
		rule, hasRule := recipe.Rule(candidate.Kind)
		required := candidate.Required
		if hasRule && rule.Required && !selected[candidate.Kind] {
			selected[candidate.Kind] = true
			required = true
		}
		if required && hasRule {
			if !bundle.containsV1(candidate.Content) {
				return fmt.Errorf("%w: required content", contract.ErrNotFound)
			}
		}
	}
	for _, rule := range recipe.Rules {
		if rule.Required && !selected[rule.Kind] {
			return fmt.Errorf("%w: required recipe kind", contract.ErrNotFound)
		}
	}
	return nil
}

func residualCandidateRefsV1(manifest contract.ContextManifest) []contract.FactRef {
	refs := make([]contract.FactRef, 0)
	for _, decision := range manifest.Decisions {
		if decision.Disposition == contract.AdmissionResidual {
			refs = append(refs, decision.CandidateRef)
		}
	}
	return refs
}
