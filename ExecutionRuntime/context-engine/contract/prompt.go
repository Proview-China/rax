package contract

import (
	"fmt"
	"sort"
)

const MaxPromptFragmentsV1 = 64

type PromptFragmentRoleV1 string

const (
	PromptFragmentInstructionV1 PromptFragmentRoleV1 = "instruction"
	PromptFragmentExampleV1     PromptFragmentRoleV1 = "example"
	PromptFragmentPolicyV1      PromptFragmentRoleV1 = "policy"
)

type PromptFragmentSpecV1 struct {
	ID              string               `json:"fragment_id"`
	Role            PromptFragmentRoleV1 `json:"role"`
	Content         ContentRef           `json:"content"`
	Required        bool                 `json:"required"`
	TokenEstimate   uint64               `json:"token_estimate"`
	EstimatorDigest Digest               `json:"estimator_digest"`
	CacheStability  uint8                `json:"cache_stability"`
	Evidence        EvidenceRef          `json:"evidence"`
}

func (f PromptFragmentSpecV1) Validate() error {
	if validateID(f.ID) != nil || f.Content.Validate() != nil || f.TokenEstimate == 0 || f.EstimatorDigest.Validate() != nil || f.CacheStability > 100 || f.Evidence.Validate() != nil {
		return fmt.Errorf("%w: prompt fragment spec", ErrInvalid)
	}
	if _, _, err := f.Role.KindAndTrustV1(); err != nil {
		return err
	}
	return nil
}

func (r PromptFragmentRoleV1) KindAndTrustV1() (FragmentKind, TrustClass, error) {
	switch r {
	case PromptFragmentInstructionV1:
		return FragmentInstruction, TrustAuthoritativeInstruction, nil
	case PromptFragmentExampleV1:
		return FragmentConversation, TrustRestrictedMaterial, nil
	case PromptFragmentPolicyV1:
		return FragmentPolicySnapshot, TrustRestrictedMaterial, nil
	default:
		return "", "", fmt.Errorf("%w: prompt fragment role", ErrInvalid)
	}
}

type PromptAssetV1 struct {
	ContractVersion     string                 `json:"contract_version"`
	ID                  string                 `json:"prompt_id"`
	SemanticVersion     string                 `json:"semantic_version"`
	Revision            uint64                 `json:"revision"`
	Owner               OwnerRef               `json:"owner"`
	AuthorityDigest     Digest                 `json:"authority_digest"`
	Sensitivity         Sensitivity            `json:"sensitivity"`
	Fragments           []PromptFragmentSpecV1 `json:"fragments"`
	ContentDigest       Digest                 `json:"content_digest"`
	RenderCompatibility []FactRef              `json:"render_compatibility"`
	Evidence            []EvidenceRef          `json:"evidence"`
	CreatedUnixNano     int64                  `json:"created_unix_nano"`
	ExpiresUnixNano     int64                  `json:"expires_unix_nano"`
}

// PromptAssetRefV1 is intentionally nominal. A Recipe FactRef or an arbitrary
// FactRef with identical strings must not be accepted as a PromptAsset ref.
type PromptAssetRefV1 struct {
	ID       string `json:"id"`
	Revision uint64 `json:"revision"`
	Digest   Digest `json:"digest"`
}

func (r PromptAssetRefV1) Validate() error {
	if validateID(r.ID) != nil || r.Revision == 0 || r.Digest.Validate() != nil {
		return fmt.Errorf("%w: prompt asset reference", ErrInvalid)
	}
	return nil
}

func (r PromptAssetRefV1) FactRefV1() FactRef {
	return FactRef{ID: r.ID, Revision: r.Revision, Digest: r.Digest}
}

func (a PromptAssetV1) Validate() error {
	if ValidateContract(a.ContractVersion) != nil || validateID(a.ID) != nil || validateID(a.SemanticVersion) != nil || a.Revision == 0 || a.Owner.Validate() != nil || a.AuthorityDigest.Validate() != nil || !validSensitivity(a.Sensitivity) || validateTimes(a.CreatedUnixNano, a.ExpiresUnixNano) != nil {
		return fmt.Errorf("%w: prompt asset", ErrInvalid)
	}
	if len(a.Fragments) == 0 || len(a.Fragments) > MaxPromptFragmentsV1 || len(a.RenderCompatibility) == 0 || len(a.RenderCompatibility) > MaxPromptFragmentsV1 || len(a.Evidence) == 0 || len(a.Evidence) > MaxOutcomeRefsV1 {
		return fmt.Errorf("%w: prompt asset bounded sets", ErrInvalid)
	}
	previous := ""
	evidence := make(map[EvidenceRef]struct{}, len(a.Evidence))
	if !canonicalFactRefsV1(a.RenderCompatibility) || !canonicalEvidenceRefsV1(a.Evidence) {
		return fmt.Errorf("%w: prompt asset canonical references", ErrConflict)
	}
	for _, ref := range a.Evidence {
		evidence[ref] = struct{}{}
	}
	for index, fragment := range a.Fragments {
		if err := fragment.Validate(); err != nil {
			return err
		}
		if index > 0 && previous >= fragment.ID {
			return fmt.Errorf("%w: prompt fragments not canonical", ErrConflict)
		}
		if _, ok := evidence[fragment.Evidence]; !ok {
			return fmt.Errorf("%w: prompt fragment evidence closure", ErrConflict)
		}
		previous = fragment.ID
	}
	want, err := promptContentDigestV1(a.Fragments)
	if err != nil || want != a.ContentDigest {
		return fmt.Errorf("%w: prompt content digest", ErrConflict)
	}
	return nil
}

func (a PromptAssetV1) DigestValue() (Digest, error) {
	if err := a.Validate(); err != nil {
		return "", err
	}
	return DigestJSON(a)
}

func (a PromptAssetV1) RefV1() (PromptAssetRefV1, error) {
	digest, err := a.DigestValue()
	if err != nil {
		return PromptAssetRefV1{}, err
	}
	return PromptAssetRefV1{ID: a.ID, Revision: a.Revision, Digest: digest}, nil
}

func SealPromptAssetV1(a PromptAssetV1) (PromptAssetV1, error) {
	a.ContractVersion = Version
	a.Fragments = append([]PromptFragmentSpecV1(nil), a.Fragments...)
	sort.Slice(a.Fragments, func(i, j int) bool { return a.Fragments[i].ID < a.Fragments[j].ID })
	a.RenderCompatibility = canonicalPromptFactRefsV1(a.RenderCompatibility)
	a.Evidence = canonicalPromptEvidenceRefsV1(a.Evidence)
	digest, err := promptContentDigestV1(a.Fragments)
	if err != nil {
		return PromptAssetV1{}, err
	}
	a.ContentDigest = digest
	return a, a.Validate()
}

func promptContentDigestV1(fragments []PromptFragmentSpecV1) (Digest, error) {
	return DigestJSON(struct {
		Domain    string                 `json:"domain"`
		Version   string                 `json:"version"`
		Fragments []PromptFragmentSpecV1 `json:"fragments"`
	}{Domain: "praxis.context.prompt-content", Version: "v1", Fragments: fragments})
}

func canonicalPromptFactRefsV1(refs []FactRef) []FactRef {
	result := append([]FactRef(nil), refs...)
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

func canonicalPromptEvidenceRefsV1(refs []EvidenceRef) []EvidenceRef {
	result := append([]EvidenceRef(nil), refs...)
	sort.Slice(result, func(i, j int) bool {
		if result[i].ID != result[j].ID {
			return result[i].ID < result[j].ID
		}
		return result[i].Digest < result[j].Digest
	})
	return result
}

type BuildPromptCandidatesRequestV1 struct {
	ContractVersion        string           `json:"contract_version"`
	PromptAssetRef         PromptAssetRefV1 `json:"prompt_asset_ref"`
	Execution              ExecutionBinding `json:"execution"`
	RenderCompatibilityRef FactRef          `json:"render_compatibility_ref"`
	CreatedUnixNano        int64            `json:"created_unix_nano"`
	NotAfterUnixNano       int64            `json:"not_after_unix_nano"`
	RequestDigest          Digest           `json:"request_digest"`
}

func (r BuildPromptCandidatesRequestV1) digestValue() (Digest, error) {
	copy := r
	copy.RequestDigest = ""
	return DigestJSON(copy)
}

func (r BuildPromptCandidatesRequestV1) Validate() error {
	if ValidateContract(r.ContractVersion) != nil || r.PromptAssetRef.Validate() != nil || r.Execution.Validate() != nil || r.RenderCompatibilityRef.Validate() != nil || validateTimes(r.CreatedUnixNano, r.NotAfterUnixNano) != nil || r.RequestDigest.Validate() != nil {
		return fmt.Errorf("%w: build prompt candidates request", ErrInvalid)
	}
	want, err := r.digestValue()
	if err != nil || want != r.RequestDigest {
		return fmt.Errorf("%w: build prompt candidates request digest", ErrConflict)
	}
	return nil
}

func SealBuildPromptCandidatesRequestV1(r BuildPromptCandidatesRequestV1) (BuildPromptCandidatesRequestV1, error) {
	r.ContractVersion = Version
	r.RequestDigest = ""
	digest, err := r.digestValue()
	if err != nil {
		return BuildPromptCandidatesRequestV1{}, err
	}
	r.RequestDigest = digest
	return r, r.Validate()
}

type PromptCandidateSetV1 struct {
	ContractVersion        string             `json:"contract_version"`
	PromptAssetRef         PromptAssetRefV1   `json:"prompt_asset_ref"`
	Execution              ExecutionBinding   `json:"execution"`
	RenderCompatibilityRef FactRef            `json:"render_compatibility_ref"`
	Candidates             []ContextCandidate `json:"candidates"`
	CreatedUnixNano        int64              `json:"created_unix_nano"`
	ExpiresUnixNano        int64              `json:"expires_unix_nano"`
	ProjectionDigest       Digest             `json:"projection_digest"`
}

func (s PromptCandidateSetV1) digestValue() (Digest, error) {
	copy := s
	copy.ProjectionDigest = ""
	return DigestJSON(copy)
}

func (s PromptCandidateSetV1) Validate() error {
	if ValidateContract(s.ContractVersion) != nil || s.PromptAssetRef.Validate() != nil || s.Execution.Validate() != nil || s.RenderCompatibilityRef.Validate() != nil || validateTimes(s.CreatedUnixNano, s.ExpiresUnixNano) != nil || s.ProjectionDigest.Validate() != nil || len(s.Candidates) == 0 || len(s.Candidates) > MaxPromptFragmentsV1 {
		return fmt.Errorf("%w: prompt candidate set", ErrInvalid)
	}
	previous := ""
	for index, candidate := range s.Candidates {
		if candidate.Validate() != nil || candidate.Execution != s.Execution || candidate.CreatedUnixNano != s.CreatedUnixNano || candidate.ExpiresUnixNano != s.ExpiresUnixNano {
			return fmt.Errorf("%w: prompt candidate set binding", ErrConflict)
		}
		if index > 0 && previous >= candidate.ID {
			return fmt.Errorf("%w: prompt candidates not canonical", ErrConflict)
		}
		previous = candidate.ID
	}
	want, err := s.digestValue()
	if err != nil || want != s.ProjectionDigest {
		return fmt.Errorf("%w: prompt candidate set digest", ErrConflict)
	}
	return nil
}

func SealPromptCandidateSetV1(s PromptCandidateSetV1) (PromptCandidateSetV1, error) {
	s.ContractVersion = Version
	s.Candidates = append([]ContextCandidate(nil), s.Candidates...)
	sort.Slice(s.Candidates, func(i, j int) bool { return s.Candidates[i].ID < s.Candidates[j].ID })
	s.ProjectionDigest = ""
	digest, err := s.digestValue()
	if err != nil {
		return PromptCandidateSetV1{}, err
	}
	s.ProjectionDigest = digest
	return s, s.Validate()
}
