package contract

import "fmt"

const ContextParentFrameApplicabilityKindV1 = "praxis.context/parent-frame-current-v1"

// ContextParentFrameApplicabilitySourceCoordinateV1 is a nominal Context
// coordinate. ID is the Frame ID query key, while Digest seals the complete
// applicability subject. It is not interchangeable with a generic FactRef.
type ContextParentFrameApplicabilitySourceCoordinateV1 struct {
	Kind     string `json:"kind"`
	ID       string `json:"id"`
	Revision uint64 `json:"revision"`
	Digest   Digest `json:"digest"`
}

func (c ContextParentFrameApplicabilitySourceCoordinateV1) Validate() error {
	if c.Kind != ContextParentFrameApplicabilityKindV1 || validateID(c.ID) != nil || c.Revision == 0 || c.Digest.Validate() != nil {
		return fmt.Errorf("%w: parent frame applicability source coordinate", ErrInvalid)
	}
	return nil
}

type ContextParentFrameApplicabilitySubjectV1 struct {
	ContractVersion                    string   `json:"contract_version"`
	FrameRef                           FactRef  `json:"frame_ref"`
	ManifestRef                        FactRef  `json:"manifest_ref"`
	GenerationRef                      FactRef  `json:"generation_ref"`
	GenerationOrdinal                  uint64   `json:"generation_ordinal"`
	ExecutionScopeDigest               Digest   `json:"execution_scope_digest"`
	RunID                              string   `json:"run_id"`
	SessionRef                         FactRef  `json:"session_ref"`
	Turn                               uint32   `json:"turn"`
	ParentFrameRef                     *FactRef `json:"parent_frame_ref,omitempty"`
	ParentGenerationRef                *FactRef `json:"parent_generation_ref,omitempty"`
	ParentFrameGenerationBindingDigest Digest   `json:"parent_frame_generation_binding_digest"`
	RecipeRef                          FactRef  `json:"recipe_ref"`
	AuthorityDigest                    Digest   `json:"authority_digest"`
}

func (s ContextParentFrameApplicabilitySubjectV1) Validate() error {
	if ValidateContract(s.ContractVersion) != nil || s.FrameRef.Validate() != nil || s.ManifestRef.Validate() != nil || s.GenerationRef.Validate() != nil || s.GenerationOrdinal == 0 || s.ExecutionScopeDigest.Validate() != nil || validateID(s.RunID) != nil || s.SessionRef.Validate() != nil || s.Turn == 0 || s.ParentFrameGenerationBindingDigest.Validate() != nil || s.RecipeRef.Validate() != nil || s.AuthorityDigest.Validate() != nil {
		return fmt.Errorf("%w: parent frame applicability subject", ErrInvalid)
	}
	if s.ParentFrameRef != nil && s.ParentFrameRef.Validate() != nil {
		return fmt.Errorf("%w: parent frame applicability parent frame", ErrInvalid)
	}
	if s.ParentGenerationRef != nil && s.ParentGenerationRef.Validate() != nil {
		return fmt.Errorf("%w: parent frame applicability parent generation", ErrInvalid)
	}
	return nil
}

func SealContextParentFrameApplicabilitySourceCoordinateV1(subject ContextParentFrameApplicabilitySubjectV1) (ContextParentFrameApplicabilitySourceCoordinateV1, error) {
	if err := subject.Validate(); err != nil {
		return ContextParentFrameApplicabilitySourceCoordinateV1{}, err
	}
	digest, err := DigestJSON(subject)
	if err != nil {
		return ContextParentFrameApplicabilitySourceCoordinateV1{}, err
	}
	coordinate := ContextParentFrameApplicabilitySourceCoordinateV1{
		Kind:     ContextParentFrameApplicabilityKindV1,
		ID:       subject.FrameRef.ID,
		Revision: subject.FrameRef.Revision,
		Digest:   digest,
	}
	return coordinate, coordinate.Validate()
}

// ContextParentFrameSourceBindingV1 is reread from the Context Owner metadata
// store. The expiry fields are current upper bounds and are deliberately not
// part of the immutable source coordinate identity.
type ContextParentFrameSourceBindingV1 struct {
	Source                   ContextParentFrameApplicabilitySourceCoordinateV1 `json:"source"`
	Subject                  ContextParentFrameApplicabilitySubjectV1          `json:"subject"`
	BindingExpiresUnixNano   int64                                             `json:"binding_expires_unix_nano"`
	RecipeExpiresUnixNano    int64                                             `json:"recipe_expires_unix_nano"`
	AuthorityExpiresUnixNano int64                                             `json:"authority_expires_unix_nano"`
}

func (b ContextParentFrameSourceBindingV1) Validate() error {
	if b.Source.Validate() != nil || b.Subject.Validate() != nil || b.BindingExpiresUnixNano <= 0 || b.RecipeExpiresUnixNano <= 0 || b.AuthorityExpiresUnixNano <= 0 {
		return fmt.Errorf("%w: parent frame source binding", ErrInvalid)
	}
	want, err := SealContextParentFrameApplicabilitySourceCoordinateV1(b.Subject)
	if err != nil || want != b.Source {
		return fmt.Errorf("%w: parent frame source binding coordinate", ErrConflict)
	}
	return nil
}

type ContextGenerationCurrentPointerRequestV1 struct {
	ExecutionScopeDigest Digest  `json:"execution_scope_digest"`
	RunID                string  `json:"run_id"`
	SessionRef           FactRef `json:"session_ref"`
	Turn                 uint32  `json:"turn"`
}

func (r ContextGenerationCurrentPointerRequestV1) Validate() error {
	if r.ExecutionScopeDigest.Validate() != nil || validateID(r.RunID) != nil || r.SessionRef.Validate() != nil || r.Turn == 0 {
		return fmt.Errorf("%w: generation current pointer request", ErrInvalid)
	}
	return nil
}

type ContextGenerationCurrentPointerV1 struct {
	ContractVersion                    string  `json:"contract_version"`
	ID                                 string  `json:"id"`
	Revision                           uint64  `json:"revision"`
	Digest                             Digest  `json:"digest"`
	ExecutionScopeDigest               Digest  `json:"execution_scope_digest"`
	RunID                              string  `json:"run_id"`
	SessionRef                         FactRef `json:"session_ref"`
	Turn                               uint32  `json:"turn"`
	GenerationRef                      FactRef `json:"generation_ref"`
	GenerationOrdinal                  uint64  `json:"generation_ordinal"`
	ParentFrameGenerationBindingDigest Digest  `json:"parent_frame_generation_binding_digest"`
	ExpiresUnixNano                    int64   `json:"expires_unix_nano"`
}

func (p ContextGenerationCurrentPointerV1) digestValue() (Digest, error) {
	copy := p
	copy.Digest = ""
	return DigestJSON(copy)
}

func (p ContextGenerationCurrentPointerV1) Validate() error {
	if ValidateContract(p.ContractVersion) != nil || validateID(p.ID) != nil || p.Revision == 0 || p.Digest.Validate() != nil || p.ExecutionScopeDigest.Validate() != nil || validateID(p.RunID) != nil || p.SessionRef.Validate() != nil || p.Turn == 0 || p.GenerationRef.Validate() != nil || p.GenerationOrdinal == 0 || p.ParentFrameGenerationBindingDigest.Validate() != nil || p.ExpiresUnixNano <= 0 {
		return fmt.Errorf("%w: generation current pointer", ErrInvalid)
	}
	digest, err := p.digestValue()
	if err != nil || digest != p.Digest {
		return fmt.Errorf("%w: generation current pointer digest", ErrConflict)
	}
	return nil
}

func SealContextGenerationCurrentPointerV1(pointer ContextGenerationCurrentPointerV1) (ContextGenerationCurrentPointerV1, error) {
	pointer.ContractVersion = Version
	pointer.Digest = ""
	digest, err := pointer.digestValue()
	if err != nil {
		return ContextGenerationCurrentPointerV1{}, err
	}
	pointer.Digest = digest
	return pointer, pointer.Validate()
}

type ContextParentFrameCurrentRequestV1 struct {
	ContractVersion  string                                            `json:"contract_version"`
	Source           ContextParentFrameApplicabilitySourceCoordinateV1 `json:"source"`
	Subject          ContextParentFrameApplicabilitySubjectV1          `json:"subject"`
	CheckedUnixNano  int64                                             `json:"checked_unix_nano"`
	NotAfterUnixNano int64                                             `json:"not_after_unix_nano"`
	Digest           Digest                                            `json:"digest"`
}

func (r ContextParentFrameCurrentRequestV1) digestValue() (Digest, error) {
	copy := r
	copy.Digest = ""
	return DigestJSON(copy)
}

func (r ContextParentFrameCurrentRequestV1) Validate() error {
	if ValidateContract(r.ContractVersion) != nil || r.Source.Validate() != nil || r.Subject.Validate() != nil || r.CheckedUnixNano <= 0 || r.NotAfterUnixNano <= r.CheckedUnixNano || r.Digest.Validate() != nil {
		return fmt.Errorf("%w: parent frame current request", ErrInvalid)
	}
	want, err := SealContextParentFrameApplicabilitySourceCoordinateV1(r.Subject)
	if err != nil || want != r.Source {
		return fmt.Errorf("%w: parent frame current request binding", ErrConflict)
	}
	digest, err := r.digestValue()
	if err != nil || digest != r.Digest {
		return fmt.Errorf("%w: parent frame current request digest", ErrConflict)
	}
	return nil
}

func SealContextParentFrameCurrentRequestV1(request ContextParentFrameCurrentRequestV1) (ContextParentFrameCurrentRequestV1, error) {
	request.ContractVersion = Version
	request.Digest = ""
	digest, err := request.digestValue()
	if err != nil {
		return ContextParentFrameCurrentRequestV1{}, err
	}
	request.Digest = digest
	return request, request.Validate()
}

type ContextParentFrameCurrentProjectionV1 struct {
	ContractVersion      string                                            `json:"contract_version"`
	Source               ContextParentFrameApplicabilitySourceCoordinateV1 `json:"source"`
	FrameRef             FactRef                                           `json:"frame_ref"`
	ManifestRef          FactRef                                           `json:"manifest_ref"`
	GenerationRef        FactRef                                           `json:"generation_ref"`
	GenerationOrdinal    uint64                                            `json:"generation_ordinal"`
	ExecutionScopeDigest Digest                                            `json:"execution_scope_digest"`
	Current              bool                                              `json:"current"`
	CheckedUnixNano      int64                                             `json:"checked_unix_nano"`
	ExpiresUnixNano      int64                                             `json:"expires_unix_nano"`
	Digest               Digest                                            `json:"digest"`
}

func (p ContextParentFrameCurrentProjectionV1) digestValue() (Digest, error) {
	copy := p
	copy.Digest = ""
	return DigestJSON(copy)
}

func (p ContextParentFrameCurrentProjectionV1) ValidateAt(nowUnixNano int64) error {
	if ValidateContract(p.ContractVersion) != nil || p.Source.Validate() != nil || p.FrameRef.Validate() != nil || p.ManifestRef.Validate() != nil || p.GenerationRef.Validate() != nil || p.GenerationOrdinal == 0 || p.ExecutionScopeDigest.Validate() != nil || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano || p.Digest.Validate() != nil {
		return fmt.Errorf("%w: parent frame current projection", ErrInvalid)
	}
	if !p.Current {
		return fmt.Errorf("%w: parent frame current projection is not current", ErrConflict)
	}
	if nowUnixNano < p.CheckedUnixNano || nowUnixNano >= p.ExpiresUnixNano {
		return fmt.Errorf("%w: parent frame current projection", ErrExpired)
	}
	digest, err := p.digestValue()
	if err != nil || digest != p.Digest {
		return fmt.Errorf("%w: parent frame current projection digest", ErrConflict)
	}
	return nil
}

func SealContextParentFrameCurrentProjectionV1(projection ContextParentFrameCurrentProjectionV1, nowUnixNano int64) (ContextParentFrameCurrentProjectionV1, error) {
	projection.ContractVersion = Version
	projection.Digest = ""
	digest, err := projection.digestValue()
	if err != nil {
		return ContextParentFrameCurrentProjectionV1{}, err
	}
	projection.Digest = digest
	return projection, projection.ValidateAt(nowUnixNano)
}

func (g ContextGeneration) DigestValue() (Digest, error) {
	if err := g.Validate(); err != nil {
		return "", err
	}
	return DigestJSON(g)
}
