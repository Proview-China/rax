package contract

import (
	"fmt"
	"strings"
)

const MaxCompactionRefsV1 = 512

type ContextCompactionSourceRangeV1 struct {
	FirstFrameRef FactRef `json:"first_frame_ref"`
	LastFrameRef  FactRef `json:"last_frame_ref"`
	FrameCount    uint32  `json:"frame_count"`
}

func (r ContextCompactionSourceRangeV1) Validate() error {
	if r.FirstFrameRef.Validate() != nil || r.LastFrameRef.Validate() != nil || r.FrameCount == 0 {
		return fmt.Errorf("%w: compaction source range", ErrInvalid)
	}
	if r.FrameCount == 1 && r.FirstFrameRef != r.LastFrameRef {
		return fmt.Errorf("%w: singleton compaction source range", ErrConflict)
	}
	if r.FrameCount > 1 && r.FirstFrameRef == r.LastFrameRef {
		return fmt.Errorf("%w: multi-frame compaction source range", ErrConflict)
	}
	return nil
}

type ContextCompactionSummaryV1 struct {
	ContractVersion      string                         `json:"contract_version"`
	ID                   string                         `json:"summary_id"`
	Revision             uint64                         `json:"revision"`
	SourceGenerationRef  FactRef                        `json:"source_generation_ref"`
	SourceRange          ContextCompactionSourceRangeV1 `json:"source_range"`
	AlgorithmID          string                         `json:"algorithm_id"`
	AlgorithmVersion     string                         `json:"algorithm_version"`
	ModelProfileRef      *FactRef                       `json:"model_profile_ref,omitempty"`
	SourceDigest         Digest                         `json:"source_digest"`
	Summary              ContentRef                     `json:"summary"`
	RetainedAnchorRefs   []FactRef                      `json:"retained_anchor_refs"`
	RecentTail           *ContentRef                    `json:"recent_tail,omitempty"`
	OpenEffectRefs       []FactRef                      `json:"open_effect_refs"`
	OutstandingWorkRefs  []FactRef                      `json:"outstanding_work_refs"`
	UncompressibleRefs   []FactRef                      `json:"uncompressible_refs"`
	TokensBefore         uint64                         `json:"tokens_before"`
	TokensAfter          uint64                         `json:"tokens_after"`
	QualityEvaluationRef *FactRef                       `json:"quality_evaluation_ref,omitempty"`
	Evidence             EvidenceRef                    `json:"evidence"`
	CreatedUnixNano      int64                          `json:"created_unix_nano"`
	ExpiresUnixNano      int64                          `json:"expires_unix_nano"`
}

func (s ContextCompactionSummaryV1) Validate() error {
	if ValidateContract(s.ContractVersion) != nil || validateID(s.ID) != nil || s.Revision != 1 || s.SourceGenerationRef.Validate() != nil || s.SourceRange.Validate() != nil || validateID(s.AlgorithmID) != nil || validateID(s.AlgorithmVersion) != nil || s.SourceDigest.Validate() != nil || s.Summary.Validate() != nil || s.TokensBefore == 0 || s.TokensAfter >= s.TokensBefore || s.Evidence.Validate() != nil || validateTimes(s.CreatedUnixNano, s.ExpiresUnixNano) != nil {
		return fmt.Errorf("%w: compaction summary", ErrInvalid)
	}
	for _, ref := range []*FactRef{s.ModelProfileRef, s.QualityEvaluationRef} {
		if ref != nil && ref.Validate() != nil {
			return fmt.Errorf("%w: compaction optional reference", ErrInvalid)
		}
	}
	if s.RecentTail != nil && s.RecentTail.Validate() != nil {
		return fmt.Errorf("%w: compaction recent tail", ErrInvalid)
	}
	for name, refs := range map[string][]FactRef{
		"retained anchors": s.RetainedAnchorRefs,
		"open effects":     s.OpenEffectRefs,
		"outstanding work": s.OutstandingWorkRefs,
		"uncompressible":   s.UncompressibleRefs,
	} {
		if refs == nil || len(refs) > MaxCompactionRefsV1 || !canonicalFactRefsV1(refs) {
			return fmt.Errorf("%w: compaction %s", ErrConflict, name)
		}
	}
	return nil
}

func (s ContextCompactionSummaryV1) DigestValue() (Digest, error) {
	if err := s.Validate(); err != nil {
		return "", err
	}
	return DigestJSON(s)
}

type ContextCompactionPlanV1 struct {
	ContractVersion    string                            `json:"contract_version"`
	AttemptID          string                            `json:"attempt_id"`
	Revision           uint64                            `json:"revision"`
	IdempotencyKey     string                            `json:"idempotency_key"`
	ExpectedCurrent    ContextGenerationCurrentPointerV1 `json:"expected_current"`
	SummaryRef         FactRef                           `json:"summary_ref"`
	TargetGenerationID string                            `json:"target_generation_id"`
	TargetRootFrameRef FactRef                           `json:"target_root_frame_ref"`
	CheckedUnixNano    int64                             `json:"checked_unix_nano"`
	ExpiresUnixNano    int64                             `json:"expires_unix_nano"`
	Digest             Digest                            `json:"digest"`
}

func (p ContextCompactionPlanV1) digestValue() (Digest, error) {
	copy := p
	copy.Digest = ""
	return DigestJSON(copy)
}

func (p ContextCompactionPlanV1) Validate() error {
	if ValidateContract(p.ContractVersion) != nil || validateID(p.AttemptID) != nil || p.Revision != 1 || validateID(p.IdempotencyKey) != nil || p.ExpectedCurrent.Validate() != nil || p.SummaryRef.Validate() != nil || validateID(p.TargetGenerationID) != nil || p.TargetRootFrameRef.Validate() != nil || validateTimes(p.CheckedUnixNano, p.ExpiresUnixNano) != nil || p.Digest.Validate() != nil {
		return fmt.Errorf("%w: compaction plan", ErrInvalid)
	}
	if p.CheckedUnixNano >= p.ExpectedCurrent.ExpiresUnixNano || p.ExpiresUnixNano > p.ExpectedCurrent.ExpiresUnixNano {
		return fmt.Errorf("%w: compaction plan currentness", ErrExpired)
	}
	want, err := p.digestValue()
	if err != nil || want != p.Digest {
		return fmt.Errorf("%w: compaction plan digest", ErrConflict)
	}
	return nil
}

func SealContextCompactionPlanV1(p ContextCompactionPlanV1) (ContextCompactionPlanV1, error) {
	p.ContractVersion = Version
	p.Revision = 1
	p.Digest = ""
	digest, err := p.digestValue()
	if err != nil {
		return ContextCompactionPlanV1{}, err
	}
	p.Digest = digest
	return p, p.Validate()
}

type ContextCompactionPreparedV1 struct {
	ContractVersion     string            `json:"contract_version"`
	PlanRef             FactRef           `json:"plan_ref"`
	SummaryRef          FactRef           `json:"summary_ref"`
	Generation          ContextGeneration `json:"generation"`
	GenerationRef       FactRef           `json:"generation_ref"`
	OutstandingWorkRefs []FactRef         `json:"outstanding_work_refs"`
	UncompressibleRefs  []FactRef         `json:"uncompressible_refs"`
	Current             bool              `json:"current"`
	PreparedUnixNano    int64             `json:"prepared_unix_nano"`
	ExpiresUnixNano     int64             `json:"expires_unix_nano"`
	Digest              Digest            `json:"digest"`
}

func (p ContextCompactionPreparedV1) digestValue() (Digest, error) {
	copy := p
	copy.Digest = ""
	return DigestJSON(copy)
}

func (p ContextCompactionPreparedV1) Validate() error {
	if ValidateContract(p.ContractVersion) != nil || p.PlanRef.Validate() != nil || p.SummaryRef.Validate() != nil || p.Generation.Validate() != nil || p.GenerationRef.Validate() != nil || p.PreparedUnixNano <= 0 || p.ExpiresUnixNano <= p.PreparedUnixNano || p.Digest.Validate() != nil || p.Current {
		return fmt.Errorf("%w: compaction prepared", ErrInvalid)
	}
	if !canonicalFactRefsV1(p.OutstandingWorkRefs) || !canonicalFactRefsV1(p.UncompressibleRefs) {
		return fmt.Errorf("%w: compaction prepared references", ErrConflict)
	}
	generationDigest, err := DigestJSON(p.Generation)
	if err != nil || p.GenerationRef != (FactRef{ID: p.Generation.ID, Revision: p.Generation.Revision, Digest: generationDigest}) {
		return fmt.Errorf("%w: compaction prepared generation", ErrConflict)
	}
	want, err := p.digestValue()
	if err != nil || want != p.Digest {
		return fmt.Errorf("%w: compaction prepared digest", ErrConflict)
	}
	return nil
}

func SealContextCompactionPreparedV1(p ContextCompactionPreparedV1) (ContextCompactionPreparedV1, error) {
	p.ContractVersion = Version
	p.Current = false
	p.Digest = ""
	digest, err := p.digestValue()
	if err != nil {
		return ContextCompactionPreparedV1{}, err
	}
	p.Digest = digest
	return p, p.Validate()
}

func canonicalFactRefsV1(refs []FactRef) bool {
	if len(refs) > MaxCompactionRefsV1 {
		return false
	}
	previous := ""
	for index, ref := range refs {
		if ref.Validate() != nil {
			return false
		}
		key := ref.ID + "\x00" + fmt.Sprintf("%020d", ref.Revision) + "\x00" + string(ref.Digest)
		if index > 0 && strings.Compare(previous, key) >= 0 {
			return false
		}
		previous = key
	}
	return true
}
