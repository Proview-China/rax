package contract

import "fmt"

type ContextFragment struct {
	CandidateRef FactRef      `json:"candidate_ref"`
	Kind         FragmentKind `json:"kind"`
	Region       FrameRegion  `json:"region"`
	Position     uint32       `json:"position"`
	Content      ContentRef   `json:"content"`
	Tokens       uint64       `json:"tokens"`
}

func (f ContextFragment) Validate() error {
	if f.CandidateRef.Validate() != nil || !validFragmentKind(f.Kind) || !validRegion(f.Region) || f.Position == 0 || f.Content.Validate() != nil || f.Tokens == 0 {
		return fmt.Errorf("%w: context fragment", ErrInvalid)
	}
	return nil
}

type ContextManifest struct {
	ContractVersion  string              `json:"contract_version"`
	ID               string              `json:"manifest_id"`
	Revision         uint64              `json:"revision"`
	Execution        ExecutionBinding    `json:"execution"`
	RecipeRef        FactRef             `json:"recipe_ref"`
	GenerationID     string              `json:"generation_id"`
	ParentFrame      *FactRef            `json:"parent_frame,omitempty"`
	Decisions        []AdmissionDecision `json:"decisions"`
	Fragments        []ContextFragment   `json:"fragments"`
	StableTokens     uint64              `json:"stable_tokens"`
	SemiStableTokens uint64              `json:"semi_stable_tokens"`
	DynamicTokens    uint64              `json:"dynamic_tokens"`
	TotalTokens      uint64              `json:"total_tokens"`
	SourceSetDigest  Digest              `json:"source_set_digest"`
	CreatedUnixNano  int64               `json:"created_unix_nano"`
	ExpiresUnixNano  int64               `json:"expires_unix_nano"`
}

func (m ContextManifest) Validate() error {
	if ValidateContract(m.ContractVersion) != nil || validateID(m.ID) != nil || m.Revision != 1 || m.Execution.Validate() != nil || m.RecipeRef.Validate() != nil || validateID(m.GenerationID) != nil || m.SourceSetDigest.Validate() != nil || validateTimes(m.CreatedUnixNano, m.ExpiresUnixNano) != nil {
		return fmt.Errorf("%w: manifest", ErrInvalid)
	}
	if m.ParentFrame != nil && m.ParentFrame.Validate() != nil {
		return fmt.Errorf("%w: parent frame", ErrInvalid)
	}
	decisionByRef := make(map[FactRef]AdmissionDecision, len(m.Decisions))
	seenCandidateIDs := make(map[string]struct{}, len(m.Decisions))
	sourceRefs := make([]FactRef, 0, len(m.Decisions))
	admitted := 0
	for _, d := range m.Decisions {
		if err := d.Validate(); err != nil {
			return err
		}
		if _, exists := seenCandidateIDs[d.CandidateRef.ID]; exists {
			return fmt.Errorf("%w: duplicate manifest candidate", ErrConflict)
		}
		seenCandidateIDs[d.CandidateRef.ID] = struct{}{}
		decisionByRef[d.CandidateRef] = d
		sourceRefs = append(sourceRefs, d.CandidateRef)
		if d.Disposition == AdmissionAdmitted {
			admitted++
		}
	}
	sourceSetDigest, err := DigestJSON(sourceRefs)
	if err != nil || sourceSetDigest != m.SourceSetDigest {
		return fmt.Errorf("%w: manifest source set", ErrConflict)
	}
	var stable, semi, dynamic uint64
	seenFragments := make(map[FactRef]struct{}, len(m.Fragments))
	for index, f := range m.Fragments {
		if err := f.Validate(); err != nil {
			return err
		}
		if f.Position != uint32(index+1) {
			return fmt.Errorf("%w: fragment position", ErrConflict)
		}
		if _, exists := seenFragments[f.CandidateRef]; exists {
			return fmt.Errorf("%w: duplicate manifest fragment", ErrConflict)
		}
		decision, exists := decisionByRef[f.CandidateRef]
		if !exists || decision.Disposition != AdmissionAdmitted || decision.Region != f.Region || decision.Tokens != f.Tokens {
			return fmt.Errorf("%w: fragment admission binding", ErrConflict)
		}
		seenFragments[f.CandidateRef] = struct{}{}
		switch f.Region {
		case RegionStablePrefix:
			stable += f.Tokens
		case RegionSemiStable:
			semi += f.Tokens
		case RegionDynamicTail:
			dynamic += f.Tokens
		}
	}
	if admitted != len(m.Fragments) {
		return fmt.Errorf("%w: admitted fragment cardinality", ErrConflict)
	}
	if stable != m.StableTokens || semi != m.SemiStableTokens || dynamic != m.DynamicTokens || stable+semi+dynamic != m.TotalTokens {
		return fmt.Errorf("%w: manifest token totals", ErrConflict)
	}
	return nil
}

func (m ContextManifest) DigestValue() (Digest, error) {
	if err := m.Validate(); err != nil {
		return "", err
	}
	return DigestJSON(m)
}

type ContextFrame struct {
	ContractVersion string           `json:"contract_version"`
	ID              string           `json:"frame_id"`
	Revision        uint64           `json:"revision"`
	Execution       ExecutionBinding `json:"execution"`
	ManifestRef     FactRef          `json:"manifest_ref"`
	ParentFrame     *FactRef         `json:"parent_frame,omitempty"`
	GenerationID    string           `json:"generation_id"`
	Generation      uint64           `json:"generation"`
	StablePrefix    ContentRef       `json:"stable_prefix"`
	SemiStable      *ContentRef      `json:"semi_stable,omitempty"`
	DynamicTail     ContentRef       `json:"dynamic_tail"`
	Rendered        ContentRef       `json:"rendered"`
	SourceSetDigest Digest           `json:"source_set_digest"`
	CreatedUnixNano int64            `json:"created_unix_nano"`
	ExpiresUnixNano int64            `json:"expires_unix_nano"`
}

func (f ContextFrame) Validate() error {
	if ValidateContract(f.ContractVersion) != nil || validateID(f.ID) != nil || f.Revision != 1 || f.Execution.Validate() != nil || f.ManifestRef.Validate() != nil || validateID(f.GenerationID) != nil || f.Generation == 0 || f.StablePrefix.Validate() != nil || f.DynamicTail.Validate() != nil || f.Rendered.Validate() != nil || f.SourceSetDigest.Validate() != nil || validateTimes(f.CreatedUnixNano, f.ExpiresUnixNano) != nil {
		return fmt.Errorf("%w: frame", ErrInvalid)
	}
	if f.ParentFrame != nil && f.ParentFrame.Validate() != nil {
		return fmt.Errorf("%w: frame parent", ErrInvalid)
	}
	if f.SemiStable != nil && f.SemiStable.Validate() != nil {
		return fmt.Errorf("%w: frame semi stable", ErrInvalid)
	}
	return nil
}

func (f ContextFrame) DigestValue() (Digest, error) {
	if err := f.Validate(); err != nil {
		return "", err
	}
	return DigestJSON(f)
}

type ContextGeneration struct {
	ContractVersion string      `json:"contract_version"`
	ID              string      `json:"generation_id"`
	Revision        uint64      `json:"revision"`
	Ordinal         uint64      `json:"ordinal"`
	Parent          *FactRef    `json:"parent,omitempty"`
	RootFrame       FactRef     `json:"root_frame"`
	Summary         *ContentRef `json:"compaction_summary,omitempty"`
	RetainedAnchors []FactRef   `json:"retained_anchors"`
	OpenEffects     []FactRef   `json:"open_effects"`
	CreatedUnixNano int64       `json:"created_unix_nano"`
}

func (g ContextGeneration) Validate() error {
	if ValidateContract(g.ContractVersion) != nil || validateID(g.ID) != nil || g.Revision != 1 || g.Ordinal == 0 || g.RootFrame.Validate() != nil || g.CreatedUnixNano <= 0 {
		return fmt.Errorf("%w: generation", ErrInvalid)
	}
	if g.Parent != nil && g.Parent.Validate() != nil {
		return fmt.Errorf("%w: generation parent", ErrInvalid)
	}
	if g.Summary != nil && g.Summary.Validate() != nil {
		return fmt.Errorf("%w: generation summary", ErrInvalid)
	}
	for _, refs := range [][]FactRef{g.RetainedAnchors, g.OpenEffects} {
		for _, ref := range refs {
			if ref.Validate() != nil {
				return fmt.Errorf("%w: generation refs", ErrInvalid)
			}
		}
	}
	return nil
}
