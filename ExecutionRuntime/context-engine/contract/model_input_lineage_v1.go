package contract

import (
	"fmt"
	"strings"
)

type ContextInvocationSourceKindV1 string

const (
	ContextInvocationSourceFrameV1              ContextInvocationSourceKindV1 = "praxis.context/frame"
	ContextInvocationSourceModelInputMaterialV1 ContextInvocationSourceKindV1 = "praxis.context/model-input-material"
)

type ContextInvocationExactSourceRefV1 struct {
	Owner    OwnerRef                      `json:"owner"`
	Kind     ContextInvocationSourceKindV1 `json:"kind"`
	ID       string                        `json:"id"`
	Revision uint64                        `json:"revision"`
	Digest   Digest                        `json:"digest"`
}

func (r ContextInvocationExactSourceRefV1) Validate() error {
	if r.Owner.Validate() != nil || !validContextInvocationSourceKindV1(r.Kind) || validateID(r.ID) != nil || r.Revision == 0 || r.Digest.Validate() != nil {
		return fmt.Errorf("%w: context invocation exact source", ErrInvalid)
	}
	return nil
}

func ContextModelInputMaterialExactSourceV1(owner OwnerRef, ref ContextModelInputMaterialRefV1) (ContextInvocationExactSourceRefV1, error) {
	source := ContextInvocationExactSourceRefV1{
		Owner:    owner,
		Kind:     ContextInvocationSourceModelInputMaterialV1,
		ID:       ref.ID,
		Revision: ref.Revision,
		Digest:   ref.Digest,
	}
	if ref.Validate() != nil || source.Validate() != nil {
		return ContextInvocationExactSourceRefV1{}, fmt.Errorf("%w: context model input material exact source", ErrInvalid)
	}
	return source, nil
}

func ContextFrameExactSourceV1(owner OwnerRef, ref FactRef) (ContextInvocationExactSourceRefV1, error) {
	source := ContextInvocationExactSourceRefV1{
		Owner:    owner,
		Kind:     ContextInvocationSourceFrameV1,
		ID:       ref.ID,
		Revision: ref.Revision,
		Digest:   ref.Digest,
	}
	if ref.Validate() != nil || source.Validate() != nil {
		return ContextInvocationExactSourceRefV1{}, fmt.Errorf("%w: context frame exact source", ErrInvalid)
	}
	return source, nil
}

func (r ContextInvocationExactSourceRefV1) MaterialRefV1() (ContextModelInputMaterialRefV1, error) {
	if r.Validate() != nil || r.Kind != ContextInvocationSourceModelInputMaterialV1 {
		return ContextModelInputMaterialRefV1{}, fmt.Errorf("%w: context invocation material source", ErrConflict)
	}
	return ContextModelInputMaterialRefV1{ID: r.ID, Revision: r.Revision, Digest: r.Digest}, nil
}

func (r ContextInvocationExactSourceRefV1) FrameRefV1() (FactRef, error) {
	if r.Validate() != nil || r.Kind != ContextInvocationSourceFrameV1 {
		return FactRef{}, fmt.Errorf("%w: context invocation frame source", ErrConflict)
	}
	return FactRef{ID: r.ID, Revision: r.Revision, Digest: r.Digest}, nil
}

type ContextModelInputLineageCurrentRequestV1 struct {
	ContractVersion  string                            `json:"contract_version"`
	Source           ContextInvocationExactSourceRefV1 `json:"source"`
	CheckedUnixNano  int64                             `json:"checked_unix_nano"`
	NotAfterUnixNano int64                             `json:"not_after_unix_nano"`
	Digest           Digest                            `json:"digest"`
}

func (r ContextModelInputLineageCurrentRequestV1) digestValue() (Digest, error) {
	copy := r
	copy.Digest = ""
	return digestDomainV1("praxis.context/model-input-lineage-current-request-v1", copy)
}

func (r ContextModelInputLineageCurrentRequestV1) Validate() error {
	if ValidateContract(r.ContractVersion) != nil || r.Source.Validate() != nil || r.Source.Kind != ContextInvocationSourceModelInputMaterialV1 || validateTimes(r.CheckedUnixNano, r.NotAfterUnixNano) != nil || r.Digest.Validate() != nil {
		return fmt.Errorf("%w: context model input lineage current request", ErrInvalid)
	}
	want, err := r.digestValue()
	if err != nil || want != r.Digest {
		return fmt.Errorf("%w: context model input lineage current request digest", ErrConflict)
	}
	return nil
}

func SealContextModelInputLineageCurrentRequestV1(r ContextModelInputLineageCurrentRequestV1) (ContextModelInputLineageCurrentRequestV1, error) {
	r.ContractVersion = Version
	r.Digest = ""
	digest, err := r.digestValue()
	if err != nil {
		return ContextModelInputLineageCurrentRequestV1{}, err
	}
	r.Digest = digest
	if err := r.Validate(); err != nil {
		return ContextModelInputLineageCurrentRequestV1{}, err
	}
	return r, nil
}

type ContextFrameExactCurrentProjectionV1 struct {
	ContractVersion string  `json:"contract_version"`
	FrameRef        FactRef `json:"frame_ref"`
	Current         bool    `json:"current"`
	CheckedUnixNano int64   `json:"checked_unix_nano"`
	ExpiresUnixNano int64   `json:"expires_unix_nano"`
	Digest          Digest  `json:"digest"`
}

func (p ContextFrameExactCurrentProjectionV1) digestValue() (Digest, error) {
	copy := p
	copy.Digest = ""
	return digestDomainV1("praxis.context/frame-exact-current-projection-v1", copy)
}

func (p ContextFrameExactCurrentProjectionV1) ValidateAt(nowUnixNano int64) error {
	if ValidateContract(p.ContractVersion) != nil || p.FrameRef.Validate() != nil || !p.Current || validateTimes(p.CheckedUnixNano, p.ExpiresUnixNano) != nil || p.Digest.Validate() != nil {
		return fmt.Errorf("%w: context frame exact current projection", ErrInvalid)
	}
	if nowUnixNano < p.CheckedUnixNano || nowUnixNano >= p.ExpiresUnixNano {
		return fmt.Errorf("%w: context frame exact current projection", ErrExpired)
	}
	want, err := p.digestValue()
	if err != nil || want != p.Digest {
		return fmt.Errorf("%w: context frame exact current projection digest", ErrConflict)
	}
	return nil
}

func SealContextFrameExactCurrentProjectionV1(p ContextFrameExactCurrentProjectionV1, nowUnixNano int64) (ContextFrameExactCurrentProjectionV1, error) {
	p.ContractVersion = Version
	p.Digest = ""
	digest, err := p.digestValue()
	if err != nil {
		return ContextFrameExactCurrentProjectionV1{}, err
	}
	p.Digest = digest
	if err := p.ValidateAt(nowUnixNano); err != nil {
		return ContextFrameExactCurrentProjectionV1{}, err
	}
	return p, nil
}

type ContextModelInputLineageCurrentProjectionV1 struct {
	ContractVersion string                            `json:"contract_version"`
	Material        ContextInvocationExactSourceRefV1 `json:"material"`
	Frame           ContextInvocationExactSourceRefV1 `json:"frame"`
	CheckedUnixNano int64                             `json:"checked_unix_nano"`
	ExpiresUnixNano int64                             `json:"expires_unix_nano"`
	Digest          Digest                            `json:"digest"`
}

func (p ContextModelInputLineageCurrentProjectionV1) digestValue() (Digest, error) {
	copy := p
	copy.Digest = ""
	return digestDomainV1("praxis.context/model-input-lineage-current-projection-v1", copy)
}

func (p ContextModelInputLineageCurrentProjectionV1) ValidateAt(nowUnixNano int64) error {
	if ValidateContract(p.ContractVersion) != nil || p.Material.Validate() != nil || p.Frame.Validate() != nil || p.Material.Kind != ContextInvocationSourceModelInputMaterialV1 || p.Frame.Kind != ContextInvocationSourceFrameV1 || p.Material.Owner != p.Frame.Owner || p.Material.Digest == p.Frame.Digest || validateTimes(p.CheckedUnixNano, p.ExpiresUnixNano) != nil || p.Digest.Validate() != nil {
		return fmt.Errorf("%w: context model input lineage current projection", ErrInvalid)
	}
	if nowUnixNano < p.CheckedUnixNano || nowUnixNano >= p.ExpiresUnixNano {
		return fmt.Errorf("%w: context model input lineage current projection", ErrExpired)
	}
	want, err := p.digestValue()
	if err != nil || want != p.Digest {
		return fmt.Errorf("%w: context model input lineage current projection digest", ErrConflict)
	}
	return nil
}

func (p ContextModelInputLineageCurrentProjectionV1) ValidateAgainst(request ContextModelInputLineageCurrentRequestV1, nowUnixNano int64) error {
	if request.Validate() != nil || p.ValidateAt(nowUnixNano) != nil || p.Material != request.Source || p.ExpiresUnixNano > request.NotAfterUnixNano {
		return fmt.Errorf("%w: context model input lineage request projection binding", ErrConflict)
	}
	return nil
}

func SealContextModelInputLineageCurrentProjectionV1(p ContextModelInputLineageCurrentProjectionV1, nowUnixNano int64) (ContextModelInputLineageCurrentProjectionV1, error) {
	p.ContractVersion = Version
	p.Digest = ""
	digest, err := p.digestValue()
	if err != nil {
		return ContextModelInputLineageCurrentProjectionV1{}, err
	}
	p.Digest = digest
	if err := p.ValidateAt(nowUnixNano); err != nil {
		return ContextModelInputLineageCurrentProjectionV1{}, err
	}
	return p, nil
}

func validContextInvocationSourceKindV1(kind ContextInvocationSourceKindV1) bool {
	if strings.TrimSpace(string(kind)) == "" {
		return false
	}
	return kind == ContextInvocationSourceFrameV1 || kind == ContextInvocationSourceModelInputMaterialV1
}
