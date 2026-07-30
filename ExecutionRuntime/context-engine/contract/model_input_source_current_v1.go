package contract

import "fmt"

// ContextModelInputSourceCurrentRequestV1 names the exact Context-owned
// Material and durable Frame pair that must still be current. Owner is kept
// raw here; neutral Model ownership is derived only by the adapter.
type ContextModelInputSourceCurrentRequestV1 struct {
	ContractVersion  string                            `json:"contract_version"`
	Owner            OwnerRef                          `json:"owner"`
	Material         ContextInvocationExactSourceRefV1 `json:"material"`
	Frame            ContextInvocationExactSourceRefV1 `json:"frame"`
	CheckedUnixNano  int64                             `json:"checked_unix_nano"`
	NotAfterUnixNano int64                             `json:"not_after_unix_nano"`
	Digest           Digest                            `json:"digest"`
}

func (r ContextModelInputSourceCurrentRequestV1) digestValue() (Digest, error) {
	copy := r
	copy.Digest = ""
	return digestDomainV1("praxis.context/model-input-source-current-request-v1", copy)
}

func (r ContextModelInputSourceCurrentRequestV1) Validate() error {
	if ValidateContract(r.ContractVersion) != nil || r.Owner.Validate() != nil ||
		r.Material.Validate() != nil || r.Frame.Validate() != nil ||
		r.Material.Owner != r.Owner || r.Frame.Owner != r.Owner ||
		r.Material.Kind != ContextInvocationSourceModelInputMaterialV1 ||
		r.Frame.Kind != ContextInvocationSourceFrameV1 ||
		r.Material.Digest == r.Frame.Digest ||
		validateTimes(r.CheckedUnixNano, r.NotAfterUnixNano) != nil ||
		r.Digest.Validate() != nil {
		return fmt.Errorf("%w: context model input source current request", ErrInvalid)
	}
	want, err := r.digestValue()
	if err != nil || want != r.Digest {
		return fmt.Errorf("%w: context model input source current request digest", ErrConflict)
	}
	return nil
}

func SealContextModelInputSourceCurrentRequestV1(r ContextModelInputSourceCurrentRequestV1) (ContextModelInputSourceCurrentRequestV1, error) {
	r.ContractVersion = Version
	r.Digest = ""
	digest, err := r.digestValue()
	if err != nil {
		return ContextModelInputSourceCurrentRequestV1{}, err
	}
	r.Digest = digest
	if err := r.Validate(); err != nil {
		return ContextModelInputSourceCurrentRequestV1{}, err
	}
	return r, nil
}

// ContextModelInputSourceCurrentProjectionV1 is the full Context-owned source
// snapshot used for exact Model lowering. Its Digest is Context-domain
// provenance and is never substituted for Model's mapped-input digest.
type ContextModelInputSourceCurrentProjectionV1 struct {
	ContractVersion string                               `json:"contract_version"`
	Owner           OwnerRef                             `json:"owner"`
	MaterialSource  ContextInvocationExactSourceRefV1    `json:"material_source"`
	Material        ContextModelInputMaterialV1          `json:"material"`
	FrameSource     ContextInvocationExactSourceRefV1    `json:"frame_source"`
	Frame           ContextFrameExactCurrentProjectionV1 `json:"frame"`
	CheckedUnixNano int64                                `json:"checked_unix_nano"`
	ExpiresUnixNano int64                                `json:"expires_unix_nano"`
	Digest          Digest                               `json:"digest"`
}

func (p ContextModelInputSourceCurrentProjectionV1) digestValue() (Digest, error) {
	copy := p.Clone()
	copy.Digest = ""
	return digestDomainV1("praxis.context/model-input-source-current-projection-v1", copy)
}

func (p ContextModelInputSourceCurrentProjectionV1) ValidateAt(nowUnixNano int64) error {
	if ValidateContract(p.ContractVersion) != nil || p.Owner.Validate() != nil ||
		p.MaterialSource.Validate() != nil || p.Material.Validate() != nil ||
		p.FrameSource.Validate() != nil ||
		p.MaterialSource.Owner != p.Owner || p.FrameSource.Owner != p.Owner ||
		p.MaterialSource.Kind != ContextInvocationSourceModelInputMaterialV1 ||
		p.FrameSource.Kind != ContextInvocationSourceFrameV1 ||
		p.MaterialSource.ID != p.Material.Ref.ID ||
		p.MaterialSource.Revision != p.Material.Ref.Revision ||
		p.MaterialSource.Digest != p.Material.Ref.Digest ||
		p.FrameSource.ID != p.Material.FrameRef.ID ||
		p.FrameSource.Revision != p.Material.FrameRef.Revision ||
		p.FrameSource.Digest != p.Material.FrameRef.Digest ||
		p.Frame.FrameRef != p.Material.FrameRef ||
		p.MaterialSource.Digest == p.FrameSource.Digest ||
		validateTimes(p.CheckedUnixNano, p.ExpiresUnixNano) != nil ||
		p.CheckedUnixNano < p.Material.CheckedUnixNano ||
		p.CheckedUnixNano < p.Frame.CheckedUnixNano ||
		p.ExpiresUnixNano > p.Material.ExpiresUnixNano ||
		p.ExpiresUnixNano > p.Frame.ExpiresUnixNano ||
		p.Digest.Validate() != nil {
		return fmt.Errorf("%w: context model input source current projection", ErrInvalid)
	}
	if err := p.Frame.ValidateAt(nowUnixNano); err != nil {
		return fmt.Errorf("context model input durable Frame currentness: %w", err)
	}
	if nowUnixNano < p.CheckedUnixNano || nowUnixNano >= p.ExpiresUnixNano {
		return fmt.Errorf("%w: context model input source current projection", ErrExpired)
	}
	want, err := p.digestValue()
	if err != nil || want != p.Digest {
		return fmt.Errorf("%w: context model input source current projection digest", ErrConflict)
	}
	return nil
}

func (p ContextModelInputSourceCurrentProjectionV1) ValidateAgainst(request ContextModelInputSourceCurrentRequestV1, nowUnixNano int64) error {
	if request.Validate() != nil || p.ValidateAt(nowUnixNano) != nil ||
		p.Owner != request.Owner || p.MaterialSource != request.Material ||
		p.FrameSource != request.Frame ||
		p.CheckedUnixNano < request.CheckedUnixNano ||
		p.ExpiresUnixNano > request.NotAfterUnixNano {
		return fmt.Errorf("%w: context model input source request projection binding", ErrConflict)
	}
	return nil
}

func SealContextModelInputSourceCurrentProjectionV1(p ContextModelInputSourceCurrentProjectionV1, nowUnixNano int64) (ContextModelInputSourceCurrentProjectionV1, error) {
	p.ContractVersion = Version
	p.Material = p.Material.Clone()
	p.Digest = ""
	digest, err := p.digestValue()
	if err != nil {
		return ContextModelInputSourceCurrentProjectionV1{}, err
	}
	p.Digest = digest
	if err := p.ValidateAt(nowUnixNano); err != nil {
		return ContextModelInputSourceCurrentProjectionV1{}, err
	}
	return p, nil
}

func (p ContextModelInputSourceCurrentProjectionV1) Clone() ContextModelInputSourceCurrentProjectionV1 {
	copy := p
	copy.Material = p.Material.Clone()
	return copy
}
