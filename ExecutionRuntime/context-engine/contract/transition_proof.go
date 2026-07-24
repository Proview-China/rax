package contract

import "fmt"

const (
	ContextTurnTransitionRequestStateV1 = "request_reserved"
	ContextTurnTransitionProofStateV1   = "proof_sealed_pending"
)

type ContextTurnTransitionRequestV1 struct {
	ContractVersion        string                            `json:"contract_version"`
	ID                     string                            `json:"id"`
	Revision               uint64                            `json:"revision"`
	ApplicationAttemptRef  FactRef                           `json:"application_attempt_ref"`
	RefreshAttemptRef      FactRef                           `json:"refresh_attempt_ref"`
	SourceSessionRef       ContextTypedFactRefV1             `json:"source_session_ref"`
	SourceTurnRef          ContextTypedFactRefV1             `json:"source_turn_ref"`
	SourceTurnOrdinal      uint32                            `json:"source_turn_ordinal"`
	ExpectedTargetOrdinal  uint32                            `json:"expected_target_ordinal"`
	ExpectedCurrent        ContextGenerationCurrentPointerV1 `json:"expected_current"`
	StableSourceSetDigest  Digest                            `json:"stable_source_set_digest"`
	S1AssociationSetDigest Digest                            `json:"s1_association_set_digest"`
	CheckedUnixNano        int64                             `json:"checked_unix_nano"`
	ExpiresUnixNano        int64                             `json:"expires_unix_nano"`
	State                  string                            `json:"state"`
	Digest                 Digest                            `json:"digest"`
}

func (r ContextTurnTransitionRequestV1) digestValue() (Digest, error) {
	copy := r
	copy.Digest = ""
	return DigestJSON(copy)
}

func (r ContextTurnTransitionRequestV1) ValidateAt(now int64) error {
	if ValidateContract(r.ContractVersion) != nil || validateID(r.ID) != nil || r.Revision != 1 || r.ApplicationAttemptRef.Validate() != nil || r.RefreshAttemptRef.Validate() != nil || r.SourceSessionRef.Validate() != nil || r.SourceTurnRef.Validate() != nil || r.SourceTurnOrdinal == 0 || r.SourceTurnOrdinal == ^uint32(0) || r.ExpectedTargetOrdinal != r.SourceTurnOrdinal+1 || r.ExpectedCurrent.Validate() != nil || r.ExpectedCurrent.Turn != r.SourceTurnOrdinal || r.StableSourceSetDigest.Validate() != nil || r.S1AssociationSetDigest.Validate() != nil || r.CheckedUnixNano <= 0 || r.ExpiresUnixNano <= r.CheckedUnixNano || now < r.CheckedUnixNano || now >= r.ExpiresUnixNano || r.State != ContextTurnTransitionRequestStateV1 || r.Digest.Validate() != nil {
		return fmt.Errorf("%w: context transition request", ErrExpired)
	}
	d, err := r.digestValue()
	if err != nil || d != r.Digest {
		return fmt.Errorf("%w: context transition request digest", ErrConflict)
	}
	return nil
}

func SealContextTurnTransitionRequestV1(r ContextTurnTransitionRequestV1, now int64) (ContextTurnTransitionRequestV1, error) {
	r.ContractVersion, r.Revision, r.State = Version, 1, ContextTurnTransitionRequestStateV1
	r.ID = "ctx-transition-request-" + r.ApplicationAttemptRef.ID
	r.Digest = ""
	d, err := r.digestValue()
	if err != nil {
		return ContextTurnTransitionRequestV1{}, err
	}
	r.Digest = d
	return r, r.ValidateAt(now)
}

func (r ContextTurnTransitionRequestV1) Ref() (FactRef, error) {
	if r.Digest.Validate() != nil {
		return FactRef{}, fmt.Errorf("%w: context transition request ref", ErrInvalid)
	}
	return FactRef{ID: r.ID, Revision: r.Revision, Digest: r.Digest}, nil
}

// ContextTurnTransitionProofV1 is the stable proof body. It excludes fresh
// association refs, checked time and expiry by construction.
type ContextTurnTransitionProofV1 struct {
	ContractVersion        string                            `json:"contract_version"`
	ID                     string                            `json:"id"`
	Revision               uint64                            `json:"revision"`
	TransitionRequestRef   FactRef                           `json:"transition_request_ref"`
	ApplicationAttemptRef  FactRef                           `json:"application_attempt_ref"`
	RefreshAttemptRef      FactRef                           `json:"refresh_attempt_ref"`
	SourceSessionRef       ContextTypedFactRefV1             `json:"source_session_ref"`
	SourceTurnRef          ContextTypedFactRefV1             `json:"source_turn_ref"`
	SourceTurnOrdinal      uint32                            `json:"source_turn_ordinal"`
	TargetTurnOrdinal      uint32                            `json:"target_turn_ordinal"`
	ExpectedCurrent        ContextGenerationCurrentPointerV1 `json:"expected_current"`
	ChildExecution         ExecutionBinding                  `json:"child_execution"`
	PendingDomainResultRef FactRef                           `json:"pending_domain_result_ref"`
	ManifestRef            FactRef                           `json:"manifest_ref"`
	FrameRef               FactRef                           `json:"frame_ref"`
	GenerationRef          FactRef                           `json:"generation_ref"`
	StableSourceSetDigest  Digest                            `json:"stable_source_set_digest"`
	State                  string                            `json:"state"`
	Digest                 Digest                            `json:"digest"`
}

func (p ContextTurnTransitionProofV1) digestValue() (Digest, error) {
	copy := p
	copy.Digest = ""
	return DigestJSON(copy)
}

func (p ContextTurnTransitionProofV1) Validate() error {
	if ValidateContract(p.ContractVersion) != nil || validateID(p.ID) != nil || p.Revision != 1 || p.TransitionRequestRef.Validate() != nil || p.ApplicationAttemptRef.Validate() != nil || p.RefreshAttemptRef.Validate() != nil || p.SourceSessionRef.Validate() != nil || p.SourceTurnRef.Validate() != nil || p.SourceTurnOrdinal == 0 || p.TargetTurnOrdinal != p.SourceTurnOrdinal+1 || p.ExpectedCurrent.Validate() != nil || p.ExpectedCurrent.Turn != p.SourceTurnOrdinal || p.ChildExecution.Validate() != nil || p.ChildExecution.Turn != p.TargetTurnOrdinal || p.PendingDomainResultRef.Validate() != nil || p.ManifestRef.Validate() != nil || p.FrameRef.Validate() != nil || p.GenerationRef.Validate() != nil || p.StableSourceSetDigest.Validate() != nil || p.State != ContextTurnTransitionProofStateV1 || p.Digest.Validate() != nil {
		return fmt.Errorf("%w: context transition proof", ErrInvalid)
	}
	d, err := p.digestValue()
	if err != nil || d != p.Digest {
		return fmt.Errorf("%w: context transition proof digest", ErrConflict)
	}
	return nil
}

func SealContextTurnTransitionProofV1(p ContextTurnTransitionProofV1) (ContextTurnTransitionProofV1, error) {
	p.ContractVersion, p.Revision, p.State = Version, 1, ContextTurnTransitionProofStateV1
	p.ID = "ctx-transition-proof-" + p.ApplicationAttemptRef.ID
	p.Digest = ""
	d, err := p.digestValue()
	if err != nil {
		return ContextTurnTransitionProofV1{}, err
	}
	p.Digest = d
	return p, p.Validate()
}

func (p ContextTurnTransitionProofV1) Ref() (FactRef, error) {
	if err := p.Validate(); err != nil {
		return FactRef{}, err
	}
	return FactRef{ID: p.ID, Revision: p.Revision, Digest: p.Digest}, nil
}

type ContextTurnTransitionProofCurrentV1 struct {
	Proof                  ContextTurnTransitionProofV1 `json:"proof"`
	S1AssociationSetDigest Digest                       `json:"s1_association_set_digest"`
	CheckedUnixNano        int64                        `json:"checked_unix_nano"`
	ExpiresUnixNano        int64                        `json:"expires_unix_nano"`
	Digest                 Digest                       `json:"digest"`
}

func (p ContextTurnTransitionProofCurrentV1) digestValue() (Digest, error) {
	copy := p
	copy.Digest = ""
	return DigestJSON(copy)
}

func (p ContextTurnTransitionProofCurrentV1) ValidateAt(now int64) error {
	if p.Proof.Validate() != nil || p.S1AssociationSetDigest.Validate() != nil || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano || now < p.CheckedUnixNano || now >= p.ExpiresUnixNano || p.Digest.Validate() != nil {
		return fmt.Errorf("%w: context transition proof current", ErrExpired)
	}
	d, err := p.digestValue()
	if err != nil || d != p.Digest {
		return fmt.Errorf("%w: context transition proof current digest", ErrConflict)
	}
	return nil
}

func SealContextTurnTransitionProofCurrentV1(p ContextTurnTransitionProofCurrentV1, now int64) (ContextTurnTransitionProofCurrentV1, error) {
	p.Digest = ""
	d, err := p.digestValue()
	if err != nil {
		return ContextTurnTransitionProofCurrentV1{}, err
	}
	p.Digest = d
	return p, p.ValidateAt(now)
}
