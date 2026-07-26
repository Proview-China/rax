package contract

import (
	"fmt"
	"sort"
)

const StructuralScorePPMMaxV1 = 1_000_000

type StructuralValueEvaluationRequestV1 struct {
	ContractVersion     string                `json:"contract_version"`
	SourceFrameRef      FactRef               `json:"source_frame_ref"`
	CandidateOutputRef  ContentRef            `json:"candidate_output_ref"`
	RequiredAnchorRefs  []FactRef             `json:"required_anchor_refs"`
	RetainedAnchorRefs  []FactRef             `json:"retained_anchor_refs"`
	SourceTokenCount    uint64                `json:"source_token_count"`
	CandidateTokenCount uint64                `json:"candidate_token_count"`
	EvaluatorRef        ContextEvaluatorRefV1 `json:"evaluator_ref"`
	CheckedUnixNano     int64                 `json:"checked_unix_nano"`
	NotAfterUnixNano    int64                 `json:"not_after_unix_nano"`
	RequestDigest       Digest                `json:"request_digest"`
}

func (r StructuralValueEvaluationRequestV1) digestValue() (Digest, error) {
	copy := r
	copy.RequestDigest = ""
	return digestDomainV1("praxis.context/structural-value-request-v1", copy)
}

func (r StructuralValueEvaluationRequestV1) Validate() error {
	if ValidateContract(r.ContractVersion) != nil || r.SourceFrameRef.Validate() != nil || r.CandidateOutputRef.Validate() != nil || r.EvaluatorRef.Validate() != nil || r.SourceTokenCount == 0 || r.CandidateTokenCount == 0 || r.CandidateTokenCount >= r.SourceTokenCount || validateTimes(r.CheckedUnixNano, r.NotAfterUnixNano) != nil || r.RequestDigest.Validate() != nil {
		return fmt.Errorf("%w: structural value request", ErrInvalid)
	}
	if r.RequiredAnchorRefs == nil || r.RetainedAnchorRefs == nil || len(r.RequiredAnchorRefs) > MaxFrameConsumptionRefsV1 || len(r.RetainedAnchorRefs) > MaxFrameConsumptionRefsV1 || !canonicalFactRefsV1(r.RequiredAnchorRefs) || !canonicalFactRefsV1(r.RetainedAnchorRefs) {
		return fmt.Errorf("%w: structural value references", ErrConflict)
	}
	want, err := r.digestValue()
	if err != nil || want != r.RequestDigest {
		return fmt.Errorf("%w: structural value request digest", ErrConflict)
	}
	return nil
}

func SealStructuralValueEvaluationRequestV1(r StructuralValueEvaluationRequestV1) (StructuralValueEvaluationRequestV1, error) {
	r.ContractVersion = Version
	r.RequiredAnchorRefs = canonicalizeCompressionFactRefsV1(r.RequiredAnchorRefs)
	r.RetainedAnchorRefs = canonicalizeCompressionFactRefsV1(r.RetainedAnchorRefs)
	r.RequestDigest = ""
	digest, err := r.digestValue()
	if err != nil {
		return StructuralValueEvaluationRequestV1{}, err
	}
	r.RequestDigest = digest
	return r, r.Validate()
}

type StructuralValueEvaluationV1 struct {
	ContractVersion    string                `json:"contract_version"`
	ID                 string                `json:"evaluation_id"`
	Revision           uint64                `json:"revision"`
	RequestDigest      Digest                `json:"request_digest"`
	EvaluatorRef       ContextEvaluatorRefV1 `json:"evaluator_ref"`
	StructuralValuePPM uint32                `json:"structural_value_ppm"`
	CoveragePPM        uint32                `json:"coverage_ppm"`
	LossPPM            uint32                `json:"loss_ppm"`
	PreservedRequired  bool                  `json:"preserved_required"`
	Limitations        []string              `json:"limitations"`
	CheckedUnixNano    int64                 `json:"checked_unix_nano"`
	ExpiresUnixNano    int64                 `json:"expires_unix_nano"`
	Digest             Digest                `json:"digest"`
}

func (v StructuralValueEvaluationV1) digestValue() (Digest, error) {
	copy := v
	copy.Digest = ""
	return digestDomainV1("praxis.context/structural-value-evaluation-v1", copy)
}

func (v StructuralValueEvaluationV1) Validate() error {
	if ValidateContract(v.ContractVersion) != nil || validateID(v.ID) != nil || v.Revision != 1 || v.RequestDigest.Validate() != nil || v.EvaluatorRef.Validate() != nil || v.StructuralValuePPM > StructuralScorePPMMaxV1 || v.CoveragePPM > StructuralScorePPMMaxV1 || v.LossPPM > StructuralScorePPMMaxV1 || v.CoveragePPM+v.LossPPM != StructuralScorePPMMaxV1 || v.Limitations == nil || len(v.Limitations) > 64 || !canonicalStringsV1(v.Limitations) || validateTimes(v.CheckedUnixNano, v.ExpiresUnixNano) != nil || v.Digest.Validate() != nil {
		return fmt.Errorf("%w: structural value evaluation", ErrInvalid)
	}
	want, err := v.digestValue()
	if err != nil || want != v.Digest {
		return fmt.Errorf("%w: structural value evaluation digest", ErrConflict)
	}
	return nil
}

func SealStructuralValueEvaluationV1(v StructuralValueEvaluationV1) (StructuralValueEvaluationV1, error) {
	v.ContractVersion = Version
	v.Revision = 1
	v.Limitations = canonicalizeStringsV1(v.Limitations)
	v.Digest = ""
	digest, err := v.digestValue()
	if err != nil {
		return StructuralValueEvaluationV1{}, err
	}
	v.Digest = digest
	return v, v.Validate()
}

type CompressionEvidenceV1 struct {
	ContractVersion     string                `json:"contract_version"`
	ID                  string                `json:"evidence_id"`
	Revision            uint64                `json:"revision"`
	SourceFrameRef      FactRef               `json:"source_frame_ref"`
	SourceGenerationRef FactRef               `json:"source_generation_ref"`
	CandidateOutputRef  ContentRef            `json:"candidate_output_ref"`
	CandidateFrameRef   *FactRef              `json:"candidate_frame_ref,omitempty"`
	RetainedAnchorRefs  []FactRef             `json:"retained_anchor_refs"`
	DeltaRefs           []FactRef             `json:"delta_refs"`
	RequiredAnchorRefs  []FactRef             `json:"required_anchor_refs"`
	EvaluationRef       FactRef               `json:"evaluation_ref"`
	EvaluatorRef        ContextEvaluatorRefV1 `json:"evaluator_ref"`
	EvaluatorVersion    string                `json:"evaluator_version"`
	SourceTokens        uint64                `json:"source_tokens"`
	CandidateTokens     uint64                `json:"candidate_tokens"`
	CoveragePPM         uint32                `json:"coverage_ppm"`
	LossPPM             uint32                `json:"loss_ppm"`
	Limitations         []string              `json:"limitations"`
	InvariantGateDigest Digest                `json:"invariant_gate_digest"`
	CheckedUnixNano     int64                 `json:"checked_unix_nano"`
	ExpiresUnixNano     int64                 `json:"expires_unix_nano"`
	Digest              Digest                `json:"digest"`
}

func (v CompressionEvidenceV1) digestValue() (Digest, error) {
	copy := v
	copy.Digest = ""
	return digestDomainV1("praxis.context/compression-evidence-v1", copy)
}

func (v CompressionEvidenceV1) Validate() error {
	if ValidateContract(v.ContractVersion) != nil || validateID(v.ID) != nil || v.Revision != 1 || v.SourceFrameRef.Validate() != nil || v.SourceGenerationRef.Validate() != nil || v.CandidateOutputRef.Validate() != nil || v.EvaluationRef.Validate() != nil || v.EvaluatorRef.Validate() != nil || validateID(v.EvaluatorVersion) != nil || v.SourceTokens == 0 || v.CandidateTokens == 0 || v.CandidateTokens >= v.SourceTokens || v.CoveragePPM > StructuralScorePPMMaxV1 || v.LossPPM > StructuralScorePPMMaxV1 || v.CoveragePPM+v.LossPPM != StructuralScorePPMMaxV1 || v.InvariantGateDigest.Validate() != nil || validateTimes(v.CheckedUnixNano, v.ExpiresUnixNano) != nil || v.Digest.Validate() != nil {
		return fmt.Errorf("%w: compression evidence", ErrInvalid)
	}
	if v.CandidateFrameRef != nil && v.CandidateFrameRef.Validate() != nil {
		return fmt.Errorf("%w: compression candidate frame", ErrInvalid)
	}
	for _, refs := range [][]FactRef{v.RetainedAnchorRefs, v.DeltaRefs, v.RequiredAnchorRefs} {
		if refs == nil || len(refs) > MaxFrameConsumptionRefsV1 || !canonicalFactRefsV1(refs) {
			return fmt.Errorf("%w: compression evidence references", ErrConflict)
		}
	}
	if !requiredRefsRetainedV1(v.RequiredAnchorRefs, v.RetainedAnchorRefs) || v.Limitations == nil || len(v.Limitations) > 64 || !canonicalStringsV1(v.Limitations) {
		return fmt.Errorf("%w: compression invariant closure", ErrConflict)
	}
	want, err := v.digestValue()
	if err != nil || want != v.Digest {
		return fmt.Errorf("%w: compression evidence digest", ErrConflict)
	}
	return nil
}

func SealCompressionEvidenceV1(v CompressionEvidenceV1) (CompressionEvidenceV1, error) {
	v.ContractVersion = Version
	v.Revision = 1
	v.RetainedAnchorRefs = canonicalizeCompressionFactRefsV1(v.RetainedAnchorRefs)
	v.DeltaRefs = canonicalizeCompressionFactRefsV1(v.DeltaRefs)
	v.RequiredAnchorRefs = canonicalizeCompressionFactRefsV1(v.RequiredAnchorRefs)
	v.Limitations = canonicalizeStringsV1(v.Limitations)
	v.Digest = ""
	digest, err := v.digestValue()
	if err != nil {
		return CompressionEvidenceV1{}, err
	}
	v.Digest = digest
	return v, v.Validate()
}

func requiredRefsRetainedV1(required, retained []FactRef) bool {
	have := make(map[FactRef]struct{}, len(retained))
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

func canonicalizeStringsV1(values []string) []string {
	result := append([]string{}, values...)
	sort.Strings(result)
	return result
}

func canonicalizeCompressionFactRefsV1(values []FactRef) []FactRef {
	result := append([]FactRef{}, values...)
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
