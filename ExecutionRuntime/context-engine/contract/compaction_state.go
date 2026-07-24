package contract

import "fmt"

type ContextCompactionStatusV1 string

const (
	ContextCompactionPendingV1 ContextCompactionStatusV1 = "pending"
	ContextCompactionAppliedV1 ContextCompactionStatusV1 = "applied_current"
)

type ContextCompactionPendingRecordV1 struct {
	Plan        ContextCompactionPlanV1           `json:"plan"`
	Summary     ContextCompactionSummaryV1        `json:"summary"`
	Manifest    ContextManifest                   `json:"manifest"`
	Frame       ContextFrame                      `json:"frame"`
	Prepared    ContextCompactionPreparedV1       `json:"prepared"`
	NextCurrent ContextGenerationCurrentPointerV1 `json:"next_current"`
}

func (r ContextCompactionPendingRecordV1) Validate() error {
	if r.Plan.Validate() != nil || r.Summary.Validate() != nil || r.Manifest.Validate() != nil || r.Frame.Validate() != nil || r.Prepared.Validate() != nil || r.NextCurrent.Validate() != nil {
		return fmt.Errorf("%w: compaction pending record", ErrInvalid)
	}
	summaryDigest, _ := r.Summary.DigestValue()
	manifestDigest, _ := r.Manifest.DigestValue()
	frameDigest, _ := r.Frame.DigestValue()
	summaryRef := FactRef{ID: r.Summary.ID, Revision: r.Summary.Revision, Digest: summaryDigest}
	manifestRef := FactRef{ID: r.Manifest.ID, Revision: r.Manifest.Revision, Digest: manifestDigest}
	frameRef := FactRef{ID: r.Frame.ID, Revision: r.Frame.Revision, Digest: frameDigest}
	if r.Plan.SummaryRef != summaryRef || r.Plan.TargetRootFrameRef != frameRef || r.Prepared.SummaryRef != summaryRef || r.Prepared.Generation.RootFrame != frameRef || r.Prepared.GenerationRef != r.NextCurrent.GenerationRef || r.Frame.ManifestRef != manifestRef || r.Frame.GenerationID != r.Prepared.Generation.ID || r.Frame.Generation != r.Prepared.Generation.Ordinal {
		return fmt.Errorf("%w: compaction pending exact binding", ErrConflict)
	}
	if r.NextCurrent.ID != r.Plan.ExpectedCurrent.ID || r.NextCurrent.Revision != r.Plan.ExpectedCurrent.Revision+1 || r.NextCurrent.ExecutionScopeDigest != r.Plan.ExpectedCurrent.ExecutionScopeDigest || r.NextCurrent.RunID != r.Plan.ExpectedCurrent.RunID || r.NextCurrent.SessionRef != r.Plan.ExpectedCurrent.SessionRef || r.NextCurrent.Turn != r.Plan.ExpectedCurrent.Turn || r.NextCurrent.GenerationOrdinal != r.Plan.ExpectedCurrent.GenerationOrdinal+1 || r.NextCurrent.ExpiresUnixNano > r.Plan.ExpiresUnixNano {
		return fmt.Errorf("%w: compaction next current", ErrConflict)
	}
	foundSummary := false
	for _, fragment := range r.Manifest.Fragments {
		if fragment.Kind == FragmentCompactionSummary && fragment.Content == r.Summary.Summary {
			if foundSummary {
				return fmt.Errorf("%w: duplicate compaction summary fragment", ErrConflict)
			}
			foundSummary = true
		}
	}
	if !foundSummary {
		return fmt.Errorf("%w: missing compaction summary fragment", ErrConflict)
	}
	return nil
}

type ApplyContextCompactionRequestV1 struct {
	ContractVersion string                            `json:"contract_version"`
	PlanRef         FactRef                           `json:"plan_ref"`
	PreparedDigest  Digest                            `json:"prepared_digest"`
	ExpectedCurrent ContextGenerationCurrentPointerV1 `json:"expected_current"`
	CheckedUnixNano int64                             `json:"checked_unix_nano"`
	Digest          Digest                            `json:"digest"`
}

func (r ApplyContextCompactionRequestV1) digestValue() (Digest, error) {
	copy := r
	copy.Digest = ""
	return DigestJSON(copy)
}

func (r ApplyContextCompactionRequestV1) Validate() error {
	if ValidateContract(r.ContractVersion) != nil || r.PlanRef.Validate() != nil || r.PreparedDigest.Validate() != nil || r.ExpectedCurrent.Validate() != nil || r.CheckedUnixNano <= 0 || r.CheckedUnixNano >= r.ExpectedCurrent.ExpiresUnixNano || r.Digest.Validate() != nil {
		return fmt.Errorf("%w: apply compaction request", ErrInvalid)
	}
	want, err := r.digestValue()
	if err != nil || want != r.Digest {
		return fmt.Errorf("%w: apply compaction request digest", ErrConflict)
	}
	return nil
}

func SealApplyContextCompactionRequestV1(r ApplyContextCompactionRequestV1) (ApplyContextCompactionRequestV1, error) {
	r.ContractVersion = Version
	r.Digest = ""
	digest, err := r.digestValue()
	if err != nil {
		return ApplyContextCompactionRequestV1{}, err
	}
	r.Digest = digest
	return r, r.Validate()
}

type InspectContextCompactionRequestV1 struct {
	PlanRef FactRef `json:"plan_ref"`
}

func (r InspectContextCompactionRequestV1) Validate() error {
	if r.PlanRef.Validate() != nil {
		return fmt.Errorf("%w: inspect compaction request", ErrInvalid)
	}
	return nil
}

type ContextCompactionResultV1 struct {
	ContractVersion string                             `json:"contract_version"`
	PlanRef         FactRef                            `json:"plan_ref"`
	SummaryRef      FactRef                            `json:"summary_ref"`
	ManifestRef     FactRef                            `json:"manifest_ref"`
	FrameRef        FactRef                            `json:"frame_ref"`
	GenerationRef   FactRef                            `json:"generation_ref"`
	Current         *ContextGenerationCurrentPointerV1 `json:"current,omitempty"`
	Status          ContextCompactionStatusV1          `json:"status"`
	Digest          Digest                             `json:"digest"`
}

func (r ContextCompactionResultV1) digestValue() (Digest, error) {
	copy := r
	copy.Digest = ""
	return DigestJSON(copy)
}

func (r ContextCompactionResultV1) Validate() error {
	if ValidateContract(r.ContractVersion) != nil || r.PlanRef.Validate() != nil || r.SummaryRef.Validate() != nil || r.ManifestRef.Validate() != nil || r.FrameRef.Validate() != nil || r.GenerationRef.Validate() != nil || r.Digest.Validate() != nil {
		return fmt.Errorf("%w: compaction result", ErrInvalid)
	}
	switch r.Status {
	case ContextCompactionPendingV1:
		if r.Current != nil {
			return fmt.Errorf("%w: pending compaction current", ErrConflict)
		}
	case ContextCompactionAppliedV1:
		if r.Current == nil || r.Current.Validate() != nil || r.Current.GenerationRef != r.GenerationRef {
			return fmt.Errorf("%w: applied compaction current", ErrConflict)
		}
	default:
		return fmt.Errorf("%w: compaction result status", ErrInvalid)
	}
	want, err := r.digestValue()
	if err != nil || want != r.Digest {
		return fmt.Errorf("%w: compaction result digest", ErrConflict)
	}
	return nil
}

func SealContextCompactionResultV1(r ContextCompactionResultV1) (ContextCompactionResultV1, error) {
	r.ContractVersion = Version
	r.Digest = ""
	digest, err := r.digestValue()
	if err != nil {
		return ContextCompactionResultV1{}, err
	}
	r.Digest = digest
	return r, r.Validate()
}
