package modelinvoker

import (
	"context"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const (
	InvocationContextOwnerBindingRequestContractVersionV1    = "praxis.model-invoker.invocation-context-owner-binding-request/v1"
	InvocationContextOwnerBindingProjectionContractVersionV1 = "praxis.model-invoker.invocation-context-owner-binding-projection/v1"
	ContextNeutralOwnerDomainV1                              = "praxis.context"
)

// ContextOwnerRef preserves the complete authoritative Context owner
// coordinate. It is not a Runtime owner and must never be replaced by a hash
// or by ComponentID alone.
type ContextOwnerRef struct {
	ComponentID   string      `json:"component_id"`
	BindingDigest core.Digest `json:"binding_digest"`
}

func (r ContextOwnerRef) Validate() error {
	if !exactContextOwnerBindingLabelV1(r.ComponentID) || r.BindingDigest.Validate() != nil {
		return governedInvalidV1("Context authoritative owner Ref is invalid")
	}
	return nil
}

// ContextMaterialLookupV1 is the owner-neutral exact lookup accepted by the
// Context adapter. Model does not assign or interpret Context Kind values.
type ContextMaterialLookupV1 struct {
	Kind     string        `json:"kind"`
	ID       string        `json:"id"`
	Revision core.Revision `json:"revision"`
	Digest   core.Digest   `json:"digest"`
}

func (r ContextMaterialLookupV1) Validate() error {
	if !exactContextOwnerBindingLabelV1(r.Kind) ||
		!exactContextOwnerBindingLabelV1(r.ID) ||
		r.Revision == 0 ||
		r.Digest.Validate() != nil {
		return governedInvalidV1("Context material exact lookup is invalid")
	}
	return nil
}

type InvocationContextOwnerBindingRequestV1 struct {
	ContractVersion  string                  `json:"contract_version"`
	MaterialLookup   ContextMaterialLookupV1 `json:"material_lookup"`
	CheckedUnixNano  int64                   `json:"checked_unix_nano"`
	NotAfterUnixNano int64                   `json:"not_after_unix_nano"`
	Digest           core.Digest             `json:"digest"`
}

func (r InvocationContextOwnerBindingRequestV1) Validate() error {
	if r.ContractVersion != InvocationContextOwnerBindingRequestContractVersionV1 ||
		r.MaterialLookup.Validate() != nil ||
		r.CheckedUnixNano <= 0 ||
		r.NotAfterUnixNano <= r.CheckedUnixNano ||
		r.Digest.Validate() != nil {
		return governedInvalidV1("invocation Context owner binding request is invalid")
	}
	expected, err := invocationContextOwnerBindingRequestDigestV1(r)
	if err != nil || expected != r.Digest {
		return governedConflictV1("invocation Context owner binding request digest drifted")
	}
	return nil
}

func SealInvocationContextOwnerBindingRequestV1(
	r InvocationContextOwnerBindingRequestV1,
) (InvocationContextOwnerBindingRequestV1, error) {
	if r.ContractVersion != "" &&
		r.ContractVersion != InvocationContextOwnerBindingRequestContractVersionV1 {
		return InvocationContextOwnerBindingRequestV1{}, governedInvalidV1(
			"invocation Context owner binding request version drifted",
		)
	}
	r.ContractVersion = InvocationContextOwnerBindingRequestContractVersionV1
	provided := r.Digest
	r.Digest = ""
	digest, err := invocationContextOwnerBindingRequestDigestV1(r)
	if err != nil {
		return InvocationContextOwnerBindingRequestV1{}, err
	}
	if provided != "" && provided != digest {
		return InvocationContextOwnerBindingRequestV1{}, governedConflictV1(
			"supplied invocation Context owner binding request digest drifted",
		)
	}
	r.Digest = digest
	return r, r.Validate()
}

type InvocationContextOwnerBindingProjectionV1 struct {
	ContractVersion            string                             `json:"contract_version"`
	ContextOwner               ContextOwnerRef                    `json:"context_owner"`
	ContextOwnerIdentityDigest core.Digest                        `json:"context_owner_identity_digest"`
	NeutralOwner               core.OwnerRef                      `json:"neutral_owner"`
	Material                   InvocationMaterialExactSourceRefV1 `json:"material"`
	Frame                      InvocationMaterialExactSourceRefV1 `json:"frame"`
	ContextLineageDigest       core.Digest                        `json:"context_lineage_digest"`
	CheckedUnixNano            int64                              `json:"checked_unix_nano"`
	ExpiresUnixNano            int64                              `json:"expires_unix_nano"`
	ProjectionDigest           core.Digest                        `json:"projection_digest"`
}

// ValidateAgainstV1 verifies the complete authoritative-to-neutral mapping,
// exact Material/Frame sources, request binding, current window and canonical
// projection digest. Context Kind meaning remains owned by the Context adapter.
func (p InvocationContextOwnerBindingProjectionV1) ValidateAgainstV1(
	request InvocationContextOwnerBindingRequestV1,
	now time.Time,
) error {
	if request.Validate() != nil ||
		p.ContractVersion != InvocationContextOwnerBindingProjectionContractVersionV1 ||
		p.ContextOwner.Validate() != nil ||
		p.ContextOwnerIdentityDigest.Validate() != nil ||
		p.NeutralOwner.Validate() != nil ||
		p.Material.Validate() != nil ||
		p.Frame.Validate() != nil ||
		p.ContextLineageDigest.Validate() != nil ||
		p.ProjectionDigest.Validate() != nil ||
		now.IsZero() {
		return governedInvalidV1("invocation Context owner binding projection is invalid")
	}

	identityDigest, neutralOwner, err := MapContextOwnerRefToNeutralOwnerV1(p.ContextOwner)
	if err != nil {
		return err
	}
	if p.ContextOwnerIdentityDigest != identityDigest ||
		p.NeutralOwner != neutralOwner ||
		p.Material.Owner != neutralOwner ||
		p.Frame.Owner != neutralOwner {
		return governedConflictV1("invocation Context owner identity mapping drifted")
	}
	if p.Material.Kind != request.MaterialLookup.Kind ||
		p.Material.ID != request.MaterialLookup.ID ||
		p.Material.Revision != request.MaterialLookup.Revision ||
		p.Material.Digest != request.MaterialLookup.Digest {
		return governedConflictV1("invocation Context material lookup drifted")
	}
	if p.Material.Kind == p.Frame.Kind ||
		p.Material.Digest == p.Frame.Digest ||
		p.Material == p.Frame {
		return governedConflictV1("invocation Context Material and Frame roles collapsed")
	}
	if p.CheckedUnixNano < request.CheckedUnixNano ||
		p.ExpiresUnixNano <= p.CheckedUnixNano ||
		p.ExpiresUnixNano > request.NotAfterUnixNano ||
		now.UnixNano() < p.CheckedUnixNano ||
		!now.Before(time.Unix(0, p.ExpiresUnixNano)) {
		return governedConflictV1("invocation Context owner binding projection is not current")
	}
	expected, err := invocationContextOwnerBindingProjectionDigestV1(p)
	if err != nil || expected != p.ProjectionDigest {
		return governedConflictV1("invocation Context owner binding projection digest drifted")
	}
	return nil
}

func SealInvocationContextOwnerBindingProjectionV1(
	p InvocationContextOwnerBindingProjectionV1,
	request InvocationContextOwnerBindingRequestV1,
	now time.Time,
) (InvocationContextOwnerBindingProjectionV1, error) {
	if p.ContractVersion != "" &&
		p.ContractVersion != InvocationContextOwnerBindingProjectionContractVersionV1 {
		return InvocationContextOwnerBindingProjectionV1{}, governedInvalidV1(
			"invocation Context owner binding projection version drifted",
		)
	}
	p.ContractVersion = InvocationContextOwnerBindingProjectionContractVersionV1

	identityDigest, neutralOwner, err := MapContextOwnerRefToNeutralOwnerV1(p.ContextOwner)
	if err != nil {
		return InvocationContextOwnerBindingProjectionV1{}, err
	}
	if p.ContextOwnerIdentityDigest != "" &&
		p.ContextOwnerIdentityDigest != identityDigest {
		return InvocationContextOwnerBindingProjectionV1{}, governedConflictV1(
			"supplied Context owner identity digest drifted",
		)
	}
	if p.NeutralOwner != (core.OwnerRef{}) && p.NeutralOwner != neutralOwner {
		return InvocationContextOwnerBindingProjectionV1{}, governedConflictV1(
			"supplied neutral Context owner drifted",
		)
	}
	p.ContextOwnerIdentityDigest = identityDigest
	p.NeutralOwner = neutralOwner

	provided := p.ProjectionDigest
	p.ProjectionDigest = ""
	digest, err := invocationContextOwnerBindingProjectionDigestV1(p)
	if err != nil {
		return InvocationContextOwnerBindingProjectionV1{}, err
	}
	if provided != "" && provided != digest {
		return InvocationContextOwnerBindingProjectionV1{}, governedConflictV1(
			"supplied invocation Context owner binding projection digest drifted",
		)
	}
	p.ProjectionDigest = digest
	return p, p.ValidateAgainstV1(request, now)
}

// ContextOwnerIdentityDigestV1 applies the frozen authoritative Context owner
// identity mapping without interpreting ComponentID or BindingDigest.
func ContextOwnerIdentityDigestV1(owner ContextOwnerRef) (core.Digest, error) {
	if err := owner.Validate(); err != nil {
		return "", err
	}
	return core.CanonicalJSONDigest(
		"praxis.context/model-neutral-owner",
		"v1",
		"ContextOwnerRef",
		owner,
	)
}

func MapContextOwnerRefToNeutralOwnerV1(
	owner ContextOwnerRef,
) (core.Digest, core.OwnerRef, error) {
	digest, err := ContextOwnerIdentityDigestV1(owner)
	if err != nil {
		return "", core.OwnerRef{}, err
	}
	neutral := core.OwnerRef{
		Domain: ContextNeutralOwnerDomainV1,
		ID:     core.OwnerID(digest),
	}
	if err := neutral.Validate(); err != nil {
		return "", core.OwnerRef{}, governedInvalidV1("mapped neutral Context owner is invalid")
	}
	return digest, neutral, nil
}

type InvocationContextOwnerBindingReaderV1 interface {
	InspectCurrentInvocationContextOwnerBindingV1(
		context.Context,
		InvocationContextOwnerBindingRequestV1,
	) (InvocationContextOwnerBindingProjectionV1, error)
}

func invocationContextOwnerBindingRequestDigestV1(
	r InvocationContextOwnerBindingRequestV1,
) (core.Digest, error) {
	r.Digest = ""
	return core.CanonicalJSONDigest(
		"praxis.model-invoker.invocation-context-owner-binding-request",
		"v1",
		"InvocationContextOwnerBindingRequestV1",
		r,
	)
}

func invocationContextOwnerBindingProjectionDigestV1(
	p InvocationContextOwnerBindingProjectionV1,
) (core.Digest, error) {
	p.ProjectionDigest = ""
	return core.CanonicalJSONDigest(
		"praxis.model-invoker.invocation-context-owner-binding-projection",
		"v1",
		"InvocationContextOwnerBindingProjectionV1",
		p,
	)
}

func exactContextOwnerBindingLabelV1(value string) bool {
	return value != "" && strings.TrimSpace(value) == value
}
