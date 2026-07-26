package kernel

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
)

type StructuralValueEvaluatorV1 interface {
	EvaluateStructuralValueV1(context.Context, contract.StructuralValueEvaluationRequestV1, string) (contract.StructuralValueEvaluationV1, error)
}

type DeterministicStructuralValueEvaluatorV1 struct{}

func (DeterministicStructuralValueEvaluatorV1) EvaluateStructuralValueV1(ctx context.Context, request contract.StructuralValueEvaluationRequestV1, evaluationID string) (contract.StructuralValueEvaluationV1, error) {
	if err := checkContextV1(ctx); err != nil {
		return contract.StructuralValueEvaluationV1{}, err
	}
	if request.Validate() != nil || strings.TrimSpace(evaluationID) == "" {
		return contract.StructuralValueEvaluationV1{}, fmt.Errorf("%w: structural evaluator request", contract.ErrInvalid)
	}
	retained := make(map[contract.FactRef]struct{}, len(request.RetainedAnchorRefs))
	for _, ref := range request.RetainedAnchorRefs {
		retained[ref] = struct{}{}
	}
	preserved := uint64(0)
	for _, ref := range request.RequiredAnchorRefs {
		if _, ok := retained[ref]; ok {
			preserved++
		}
	}
	coverage := uint64(contract.StructuralScorePPMMaxV1)
	if len(request.RequiredAnchorRefs) > 0 {
		coverage = preserved * uint64(contract.StructuralScorePPMMaxV1) / uint64(len(request.RequiredAnchorRefs))
	}
	savings := (request.SourceTokenCount - request.CandidateTokenCount) * uint64(contract.StructuralScorePPMMaxV1) / request.SourceTokenCount
	value := savings * coverage / uint64(contract.StructuralScorePPMMaxV1)
	evaluation, err := contract.SealStructuralValueEvaluationV1(contract.StructuralValueEvaluationV1{
		ID:                 evaluationID,
		RequestDigest:      request.RequestDigest,
		EvaluatorRef:       request.EvaluatorRef,
		StructuralValuePPM: uint32(value),
		CoveragePPM:        uint32(coverage),
		LossPPM:            uint32(uint64(contract.StructuralScorePPMMaxV1) - coverage),
		PreservedRequired:  preserved == uint64(len(request.RequiredAnchorRefs)),
		Limitations:        []string{"deterministic_heuristic_only"},
		CheckedUnixNano:    request.CheckedUnixNano,
		ExpiresUnixNano:    request.NotAfterUnixNano,
	})
	if err != nil {
		return contract.StructuralValueEvaluationV1{}, err
	}
	return evaluation, checkContextV1(ctx)
}

type CompressionInvariantInputV1 struct {
	SourceFrame        contract.ContextFrame
	SourceGeneration   contract.ContextGeneration
	CandidateOutputRef contract.ContentRef
	CandidateFrameRef  *contract.FactRef
	RequiredAnchorRefs []contract.FactRef
	RetainedAnchorRefs []contract.FactRef
	DeltaRefs          []contract.FactRef
	ProtectedRefs      []contract.FactRef
	SourceTokens       uint64
	CandidateTokens    uint64
	CheckedUnixNano    int64
	ExpiresUnixNano    int64
}

type CompressionPreparationRequestV1 struct {
	EvaluationID     string
	EvidenceID       string
	EvaluatorRef     contract.ContextEvaluatorRefV1
	EvaluatorVersion string
	Invariant        CompressionInvariantInputV1
}

type CompressionPreparationResultV1 struct {
	Evaluation contract.StructuralValueEvaluationV1
	Evidence   contract.CompressionEvidenceV1
}

func PrepareCompressionEvidenceV1(ctx context.Context, evaluator StructuralValueEvaluatorV1, request CompressionPreparationRequestV1) (CompressionPreparationResultV1, error) {
	if err := checkContextV1(ctx); err != nil {
		return CompressionPreparationResultV1{}, err
	}
	if evaluator == nil || request.EvaluatorRef.Validate() != nil || strings.TrimSpace(request.EvaluationID) == "" || strings.TrimSpace(request.EvidenceID) == "" || strings.TrimSpace(request.EvaluatorVersion) == "" {
		return CompressionPreparationResultV1{}, fmt.Errorf("%w: compression preparation", contract.ErrInvalid)
	}
	gateDigest, requiredClosure, err := evaluateCompressionInvariantV1(request.Invariant)
	if err != nil {
		return CompressionPreparationResultV1{}, err
	}
	frameDigest, err := request.Invariant.SourceFrame.DigestValue()
	if err != nil {
		return CompressionPreparationResultV1{}, err
	}
	frameRef := contract.FactRef{ID: request.Invariant.SourceFrame.ID, Revision: request.Invariant.SourceFrame.Revision, Digest: frameDigest}
	evaluationRequest, err := contract.SealStructuralValueEvaluationRequestV1(contract.StructuralValueEvaluationRequestV1{
		SourceFrameRef:      frameRef,
		CandidateOutputRef:  request.Invariant.CandidateOutputRef,
		RequiredAnchorRefs:  requiredClosure,
		RetainedAnchorRefs:  request.Invariant.RetainedAnchorRefs,
		SourceTokenCount:    request.Invariant.SourceTokens,
		CandidateTokenCount: request.Invariant.CandidateTokens,
		EvaluatorRef:        request.EvaluatorRef,
		CheckedUnixNano:     request.Invariant.CheckedUnixNano,
		NotAfterUnixNano:    request.Invariant.ExpiresUnixNano,
	})
	if err != nil {
		return CompressionPreparationResultV1{}, err
	}
	evaluation, err := evaluator.EvaluateStructuralValueV1(ctx, evaluationRequest, request.EvaluationID)
	if err != nil {
		return CompressionPreparationResultV1{}, err
	}
	if evaluation.Validate() != nil || evaluation.RequestDigest != evaluationRequest.RequestDigest || evaluation.EvaluatorRef != request.EvaluatorRef || evaluation.CheckedUnixNano != request.Invariant.CheckedUnixNano || evaluation.ExpiresUnixNano > request.Invariant.ExpiresUnixNano || !evaluation.PreservedRequired {
		return CompressionPreparationResultV1{}, fmt.Errorf("%w: advisory evaluator binding", contract.ErrConflict)
	}
	secondGate, secondRequired, err := evaluateCompressionInvariantV1(request.Invariant)
	if err != nil {
		return CompressionPreparationResultV1{}, err
	}
	if secondGate != gateDigest || !sameFactRefSliceV1(requiredClosure, secondRequired) {
		return CompressionPreparationResultV1{}, fmt.Errorf("%w: compression invariant drift", contract.ErrConflict)
	}
	generationDigest, err := contract.DigestJSON(request.Invariant.SourceGeneration)
	if err != nil {
		return CompressionPreparationResultV1{}, err
	}
	evidence, err := contract.SealCompressionEvidenceV1(contract.CompressionEvidenceV1{
		ID:                  request.EvidenceID,
		SourceFrameRef:      frameRef,
		SourceGenerationRef: contract.FactRef{ID: request.Invariant.SourceGeneration.ID, Revision: request.Invariant.SourceGeneration.Revision, Digest: generationDigest},
		CandidateOutputRef:  request.Invariant.CandidateOutputRef,
		CandidateFrameRef:   cloneFactRefPointerV1(request.Invariant.CandidateFrameRef),
		RetainedAnchorRefs:  request.Invariant.RetainedAnchorRefs,
		DeltaRefs:           request.Invariant.DeltaRefs,
		RequiredAnchorRefs:  requiredClosure,
		EvaluationRef:       contract.FactRef{ID: evaluation.ID, Revision: evaluation.Revision, Digest: evaluation.Digest},
		EvaluatorRef:        request.EvaluatorRef,
		EvaluatorVersion:    request.EvaluatorVersion,
		SourceTokens:        request.Invariant.SourceTokens,
		CandidateTokens:     request.Invariant.CandidateTokens,
		CoveragePPM:         evaluation.CoveragePPM,
		LossPPM:             evaluation.LossPPM,
		Limitations:         append([]string{}, evaluation.Limitations...),
		InvariantGateDigest: gateDigest,
		CheckedUnixNano:     request.Invariant.CheckedUnixNano,
		ExpiresUnixNano:     minInt64V1(request.Invariant.ExpiresUnixNano, evaluation.ExpiresUnixNano),
	})
	if err != nil {
		return CompressionPreparationResultV1{}, err
	}
	return CompressionPreparationResultV1{Evaluation: evaluation, Evidence: evidence}, checkContextV1(ctx)
}

func evaluateCompressionInvariantV1(input CompressionInvariantInputV1) (contract.Digest, []contract.FactRef, error) {
	if input.SourceFrame.Validate() != nil || input.SourceGeneration.Validate() != nil || input.CandidateOutputRef.Validate() != nil || input.SourceTokens == 0 || input.CandidateTokens == 0 || input.CandidateTokens >= input.SourceTokens || input.CheckedUnixNano <= 0 || input.ExpiresUnixNano <= input.CheckedUnixNano {
		return "", nil, fmt.Errorf("%w: compression invariant input", contract.ErrInvalid)
	}
	frameDigest, err := input.SourceFrame.DigestValue()
	if err != nil {
		return "", nil, err
	}
	if input.SourceGeneration.RootFrame != (contract.FactRef{ID: input.SourceFrame.ID, Revision: input.SourceFrame.Revision, Digest: frameDigest}) {
		return "", nil, fmt.Errorf("%w: compression source binding", contract.ErrConflict)
	}
	for _, refs := range [][]contract.FactRef{input.RequiredAnchorRefs, input.RetainedAnchorRefs, input.DeltaRefs, input.ProtectedRefs} {
		if refs == nil || !canonicalFactRefsKernelV1(refs) {
			return "", nil, fmt.Errorf("%w: compression invariant references", contract.ErrConflict)
		}
	}
	required := mergeFactRefsV1(input.RequiredAnchorRefs, input.ProtectedRefs, input.SourceGeneration.OpenEffects)
	if !factRefsContainedV1(required, input.RetainedAnchorRefs) {
		return "", nil, fmt.Errorf("%w: required compression reference removed", contract.ErrConflict)
	}
	digest, err := contract.DigestJSON(struct {
		Domain       string
		Input        CompressionInvariantInputV1
		RequiredRefs []contract.FactRef
	}{"praxis.context/compression-invariant-v1", cloneInvariantInputV1(input), required})
	if err != nil {
		return "", nil, err
	}
	return digest, required, nil
}

func canonicalFactRefsKernelV1(values []contract.FactRef) bool {
	for index, value := range values {
		if value.Validate() != nil {
			return false
		}
		if index > 0 {
			previous := values[index-1]
			if previous.ID > value.ID || previous.ID == value.ID && (previous.Revision > value.Revision || previous.Revision == value.Revision && previous.Digest >= value.Digest) {
				return false
			}
		}
	}
	return true
}

func mergeFactRefsV1(groups ...[]contract.FactRef) []contract.FactRef {
	unique := make(map[contract.FactRef]struct{})
	for _, group := range groups {
		for _, ref := range group {
			unique[ref] = struct{}{}
		}
	}
	result := make([]contract.FactRef, 0, len(unique))
	for ref := range unique {
		result = append(result, ref)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].ID != result[j].ID {
			return result[i].ID < result[j].ID
		}
		if result[i].Revision != result[j].Revision {
			return result[i].Revision < result[j].Revision
		}
		return result[i].Digest < result[j].Digest
	})
	return result
}

func factRefsContainedV1(required, retained []contract.FactRef) bool {
	have := make(map[contract.FactRef]struct{}, len(retained))
	for _, ref := range retained {
		have[ref] = struct{}{}
	}
	for _, ref := range required {
		if _, ok := have[ref]; !ok {
			return false
		}
	}
	return true
}

func cloneInvariantInputV1(input CompressionInvariantInputV1) CompressionInvariantInputV1 {
	copy := input
	copy.CandidateFrameRef = cloneFactRefPointerV1(input.CandidateFrameRef)
	copy.RequiredAnchorRefs = append([]contract.FactRef{}, input.RequiredAnchorRefs...)
	copy.RetainedAnchorRefs = append([]contract.FactRef{}, input.RetainedAnchorRefs...)
	copy.DeltaRefs = append([]contract.FactRef{}, input.DeltaRefs...)
	copy.ProtectedRefs = append([]contract.FactRef{}, input.ProtectedRefs...)
	copy.SourceGeneration.RetainedAnchors = append([]contract.FactRef{}, input.SourceGeneration.RetainedAnchors...)
	copy.SourceGeneration.OpenEffects = append([]contract.FactRef{}, input.SourceGeneration.OpenEffects...)
	return copy
}

func cloneFactRefPointerV1(value *contract.FactRef) *contract.FactRef {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
