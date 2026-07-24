package sdk

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
)

func cloneValidateRequestV1(value ValidateRecipeRequestV1) ValidateRecipeRequestV1 {
	value.Recipe = cloneRecipeV1(value.Recipe)
	value.Candidates = cloneCandidatesV1(value.Candidates)
	return value
}

func cloneCompileRequestV1(value CompileFrameRequestV1) CompileFrameRequestV1 {
	value.Recipe = cloneRecipeV1(value.Recipe)
	value.Candidates = cloneCandidatesV1(value.Candidates)
	value.ParentFrame = cloneFactRefPtrV1(value.ParentFrame)
	value.InputBundle = OfflineContentBundleV1{items: value.InputBundle.Items(), contentSetDigest: value.InputBundle.contentSetDigest}
	return value
}

func clonePreviewRequestV1(value PreviewFrameRequestV1) PreviewFrameRequestV1 {
	value.Compiled = cloneCompiledV1(value.Compiled)
	return value
}

func cloneInspectRequestV1(value InspectFrameExactRequestV1) InspectFrameExactRequestV1 {
	value.Manifest = cloneManifestV1(value.Manifest)
	value.Frame = cloneFrameV1(value.Frame)
	value.ContentBundle = OfflineContentBundleV1{items: value.ContentBundle.Items(), contentSetDigest: value.ContentBundle.contentSetDigest}
	return value
}

func cloneBundleContextV1(ctx context.Context, value OfflineContentBundleV1) (OfflineContentBundleV1, error) {
	if value.items == nil {
		return OfflineContentBundleV1{}, nil
	}
	items := make([]OfflineContentItemV1, len(value.items))
	for i := range value.items {
		cloned, err := cloneContextBytesV1(ctx, value.items[i].Bytes)
		if err != nil {
			return OfflineContentBundleV1{}, err
		}
		items[i] = OfflineContentItemV1{Ref: value.items[i].Ref, Bytes: cloned}
	}
	return OfflineContentBundleV1{items: items, contentSetDigest: value.contentSetDigest}, nil
}

func cloneCompileRequestContextV1(ctx context.Context, value CompileFrameRequestV1) (CompileFrameRequestV1, error) {
	value.Recipe = cloneRecipeV1(value.Recipe)
	value.Candidates = cloneCandidatesV1(value.Candidates)
	value.ParentFrame = cloneFactRefPtrV1(value.ParentFrame)
	bundle, err := cloneBundleContextV1(ctx, value.InputBundle)
	if err != nil {
		return CompileFrameRequestV1{}, err
	}
	value.InputBundle = bundle
	return value, nil
}

func cloneCompiledContextV1(ctx context.Context, value CompiledBundleV1) (CompiledBundleV1, error) {
	value.Manifest = cloneManifestV1(value.Manifest)
	value.Frame = cloneFrameV1(value.Frame)
	bundle, err := cloneBundleContextV1(ctx, value.ContentBundle)
	if err != nil {
		return CompiledBundleV1{}, err
	}
	value.ContentBundle = bundle
	value.ResidualCandidateRefs = cloneSliceV1(value.ResidualCandidateRefs)
	return value, nil
}

func cloneCompileResponseContextV1(ctx context.Context, value CompileFrameResponseV1) (CompileFrameResponseV1, error) {
	compiled, err := cloneCompiledContextV1(ctx, value.Compiled)
	if err != nil {
		return CompileFrameResponseV1{}, err
	}
	value.Compiled = compiled
	value.Diagnostics = cloneDiagnosticsV1(value.Diagnostics)
	return value, nil
}

func clonePreviewRequestContextV1(ctx context.Context, value PreviewFrameRequestV1) (PreviewFrameRequestV1, error) {
	compiled, err := cloneCompiledContextV1(ctx, value.Compiled)
	if err != nil {
		return PreviewFrameRequestV1{}, err
	}
	value.Compiled = compiled
	return value, nil
}

func cloneInspectRequestContextV1(ctx context.Context, value InspectFrameExactRequestV1) (InspectFrameExactRequestV1, error) {
	value.Manifest = cloneManifestV1(value.Manifest)
	value.Frame = cloneFrameV1(value.Frame)
	bundle, err := cloneBundleContextV1(ctx, value.ContentBundle)
	if err != nil {
		return InspectFrameExactRequestV1{}, err
	}
	value.ContentBundle = bundle
	return value, nil
}

func validateResponseResultDigestV1(discriminator string, meta *OfflineResponseMetaV1, body any, contexts ...context.Context) (contract.Digest, error) {
	copyMeta := *meta
	copyMeta.ResultDigest = ""
	return canonicalDigestV1(discriminator, struct {
		Meta OfflineResponseMetaV1 `json:"meta"`
		Body any                   `json:"body"`
	}{copyMeta, body}, contexts...)
}

type compiledClosureProjectionV1 struct {
	Manifest              contract.ContextManifest `json:"manifest"`
	Frame                 contract.ContextFrame    `json:"frame"`
	ContentBundle         offlineBundleClosureV1   `json:"content_bundle"`
	ResidualCandidateRefs []contract.FactRef       `json:"residual_candidate_refs"`
	Authoritative         bool                     `json:"authoritative"`
	CompileDigest         contract.Digest          `json:"compile_digest"`
}

func compiledProjectionV1(value CompiledBundleV1) compiledClosureProjectionV1 {
	return compiledClosureProjectionV1{
		Manifest: cloneManifestV1(value.Manifest), Frame: cloneFrameV1(value.Frame), ContentBundle: value.ContentBundle.closureV1(),
		ResidualCandidateRefs: cloneSliceV1(value.ResidualCandidateRefs), Authoritative: value.Authoritative, CompileDigest: value.CompileDigest,
	}
}

func responsePrivateV1(value CompileFrameResponseV1) any {
	return struct {
		Compiled    compiledClosureProjectionV1 `json:"compiled"`
		Diagnostics []OfflineDiagnosticV1       `json:"diagnostics"`
	}{compiledProjectionV1(value.Compiled), cloneDiagnosticsV1(value.Diagnostics)}
}

func validateResponsePrivateV1(value ValidateRecipeResponseV1) any {
	return struct {
		Valid         bool                  `json:"valid"`
		RecipeRef     *contract.FactRef     `json:"recipe_ref,omitempty"`
		CandidateRefs []contract.FactRef    `json:"candidate_refs"`
		Diagnostics   []OfflineDiagnosticV1 `json:"diagnostics"`
		ReportDigest  contract.Digest       `json:"report_digest"`
	}{value.Valid, cloneFactRefPtrV1(value.RecipeRef), cloneSliceV1(value.CandidateRefs), cloneDiagnosticsV1(value.Diagnostics), value.ReportDigest}
}

func previewResponsePrivateV1(value PreviewFrameResponseV1) any {
	copy := clonePreviewResponseV1(value)
	copy.Meta = OfflineResponseMetaV1{}
	return struct {
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
	}{copy.AdmissionDecisions, copy.Fragments, copy.StableTokens, copy.SemiStableTokens, copy.DynamicTokens, copy.TotalTokens,
		copy.StablePrefixRef, copy.SemiStableRef, copy.DynamicTailRef, copy.RenderedRef, copy.SourceSetDigest, copy.RecipeRef,
		copy.ManifestRef, copy.FrameRef, copy.ExpiresUnixNano, copy.Diagnostics, copy.PreviewDigest}
}

func inspectResponsePrivateV1(value InspectFrameExactResponseV1) any {
	return struct {
		Exact            bool                  `json:"exact"`
		ManifestRef      contract.FactRef      `json:"manifest_ref"`
		FrameRef         contract.FactRef      `json:"frame_ref"`
		ContentSetDigest contract.Digest       `json:"content_set_digest"`
		CheckedUnixNano  int64                 `json:"checked_unix_nano"`
		ExpiresUnixNano  int64                 `json:"expires_unix_nano"`
		Diagnostics      []OfflineDiagnosticV1 `json:"diagnostics"`
		InspectionDigest contract.Digest       `json:"inspection_digest"`
	}{value.Exact, value.ManifestRef, value.FrameRef, value.ContentSetDigest, value.CheckedUnixNano, value.ExpiresUnixNano, cloneDiagnosticsV1(value.Diagnostics), value.InspectionDigest}
}

func previewRequestDigestV1(request PreviewFrameRequestV1, contexts ...context.Context) (contract.Digest, error) {
	meta := request.Meta
	meta.RequestDigest = ""
	return canonicalDigestV1("preview-frame-request", struct {
		Meta                  OfflineRequestMetaV1        `json:"meta"`
		Compiled              compiledClosureProjectionV1 `json:"compiled"`
		ExpectedCompileDigest contract.Digest             `json:"expected_compile_digest"`
		CheckedUnixNano       int64                       `json:"checked_unix_nano"`
	}{meta, compiledProjectionV1(request.Compiled), request.ExpectedCompileDigest, request.CheckedUnixNano}, contexts...)
}

func validatePreviewRequestDigestV1(request PreviewFrameRequestV1, contexts ...context.Context) error {
	digest, err := previewRequestDigestV1(request, contexts...)
	if err != nil {
		return mapErrorV1(OfflinePreviewFrameV1, "meta.request_digest", err)
	}
	if digest != request.Meta.RequestDigest {
		return sdkErrorV1(OfflineErrorConflictV1, OfflinePreviewFrameV1, "meta.request_digest", "request digest mismatch", contract.ErrConflict)
	}
	return nil
}

func inspectRequestDigestV1(request InspectFrameExactRequestV1, contexts ...context.Context) (contract.Digest, error) {
	meta := request.Meta
	meta.RequestDigest = ""
	return canonicalDigestV1("inspect-frame-exact-request", struct {
		Meta                  OfflineRequestMetaV1     `json:"meta"`
		Manifest              contract.ContextManifest `json:"manifest"`
		Frame                 contract.ContextFrame    `json:"frame"`
		ContentBundle         offlineBundleClosureV1   `json:"content_bundle"`
		ExpectedManifestRef   contract.FactRef         `json:"expected_manifest_ref"`
		ExpectedFrameRef      contract.FactRef         `json:"expected_frame_ref"`
		ExpectedCompileDigest contract.Digest          `json:"expected_compile_digest"`
		CheckedUnixNano       int64                    `json:"checked_unix_nano"`
	}{meta, cloneManifestV1(request.Manifest), cloneFrameV1(request.Frame), request.ContentBundle.closureV1(), request.ExpectedManifestRef, request.ExpectedFrameRef, request.ExpectedCompileDigest, request.CheckedUnixNano}, contexts...)
}

func validateInspectRequestDigestV1(request InspectFrameExactRequestV1, contexts ...context.Context) error {
	digest, err := inspectRequestDigestV1(request, contexts...)
	if err != nil {
		return mapErrorV1(OfflineInspectFrameExactV1, "meta.request_digest", err)
	}
	if digest != request.Meta.RequestDigest {
		return sdkErrorV1(OfflineErrorConflictV1, OfflineInspectFrameExactV1, "meta.request_digest", "request digest mismatch", contract.ErrConflict)
	}
	return nil
}

func previewDigestV1(response PreviewFrameResponseV1, contexts ...context.Context) (contract.Digest, error) {
	return canonicalDigestV1("frame-preview", struct {
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
	}{response.AdmissionDecisions, response.Fragments, response.StableTokens, response.SemiStableTokens, response.DynamicTokens, response.TotalTokens,
		response.StablePrefixRef, cloneContentRefPtrV1(response.SemiStableRef), response.DynamicTailRef, response.RenderedRef,
		response.SourceSetDigest, response.RecipeRef, response.ManifestRef, response.FrameRef, response.ExpiresUnixNano, response.Diagnostics}, contexts...)
}

func inspectionDigestV1(response InspectFrameExactResponseV1, contexts ...context.Context) (contract.Digest, error) {
	return canonicalDigestV1("frame-inspection", struct {
		Exact            bool                  `json:"exact"`
		ManifestRef      contract.FactRef      `json:"manifest_ref"`
		FrameRef         contract.FactRef      `json:"frame_ref"`
		ContentSetDigest contract.Digest       `json:"content_set_digest"`
		CheckedUnixNano  int64                 `json:"checked_unix_nano"`
		ExpiresUnixNano  int64                 `json:"expires_unix_nano"`
		Diagnostics      []OfflineDiagnosticV1 `json:"diagnostics"`
	}{response.Exact, response.ManifestRef, response.FrameRef, response.ContentSetDigest, response.CheckedUnixNano, response.ExpiresUnixNano, response.Diagnostics}, contexts...)
}
