package sdk

import "github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"

func cloneSliceV1[T any](values []T) []T {
	if values == nil {
		return nil
	}
	result := make([]T, len(values))
	copy(result, values)
	return result
}

func cloneFactRefPtrV1(value *contract.FactRef) *contract.FactRef {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneContentRefPtrV1(value *contract.ContentRef) *contract.ContentRef {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneRecipeV1(value contract.ContextRecipe) contract.ContextRecipe {
	value.Rules = cloneSliceV1(value.Rules)
	return value
}

func cloneCandidatesV1(values []contract.ContextCandidate) []contract.ContextCandidate {
	return cloneSliceV1(values)
}

func cloneManifestV1(value contract.ContextManifest) contract.ContextManifest {
	value.ParentFrame = cloneFactRefPtrV1(value.ParentFrame)
	value.Decisions = cloneSliceV1(value.Decisions)
	value.Fragments = cloneSliceV1(value.Fragments)
	return value
}

func cloneFrameV1(value contract.ContextFrame) contract.ContextFrame {
	value.ParentFrame = cloneFactRefPtrV1(value.ParentFrame)
	value.SemiStable = cloneContentRefPtrV1(value.SemiStable)
	return value
}

func cloneCompiledV1(value CompiledBundleV1) CompiledBundleV1 {
	value.Manifest = cloneManifestV1(value.Manifest)
	value.Frame = cloneFrameV1(value.Frame)
	value.ContentBundle = OfflineContentBundleV1{items: value.ContentBundle.Items(), contentSetDigest: value.ContentBundle.contentSetDigest}
	value.ResidualCandidateRefs = cloneSliceV1(value.ResidualCandidateRefs)
	return value
}

func cloneDiagnosticsV1(values []OfflineDiagnosticV1) []OfflineDiagnosticV1 {
	return cloneSliceV1(values)
}

func cloneValidateResponseV1(value ValidateRecipeResponseV1) ValidateRecipeResponseV1 {
	value.RecipeRef = cloneFactRefPtrV1(value.RecipeRef)
	value.CandidateRefs = cloneSliceV1(value.CandidateRefs)
	value.Diagnostics = cloneDiagnosticsV1(value.Diagnostics)
	return value
}

func cloneCompileResponseV1(value CompileFrameResponseV1) CompileFrameResponseV1 {
	value.Compiled = cloneCompiledV1(value.Compiled)
	value.Diagnostics = cloneDiagnosticsV1(value.Diagnostics)
	return value
}

func clonePreviewResponseV1(value PreviewFrameResponseV1) PreviewFrameResponseV1 {
	value.AdmissionDecisions = cloneSliceV1(value.AdmissionDecisions)
	value.Fragments = cloneSliceV1(value.Fragments)
	value.SemiStableRef = cloneContentRefPtrV1(value.SemiStableRef)
	value.Diagnostics = cloneDiagnosticsV1(value.Diagnostics)
	return value
}

func cloneInspectResponseV1(value InspectFrameExactResponseV1) InspectFrameExactResponseV1 {
	value.Diagnostics = cloneDiagnosticsV1(value.Diagnostics)
	return value
}
