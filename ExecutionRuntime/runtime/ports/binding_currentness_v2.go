package ports

import (
	"context"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const ProviderBindingCurrentnessContractVersionV2 = "2.0.0"

type ProviderBindingCurrentStateV2 string

const (
	ProviderBindingCurrentActiveV2  ProviderBindingCurrentStateV2 = "active"
	ProviderBindingCurrentRevokedV2 ProviderBindingCurrentStateV2 = "revoked"
	ProviderBindingCurrentExpiredV2 ProviderBindingCurrentStateV2 = "expired"
)

// ProviderBindingCurrentProjectionV2 is a read-only, least-authority view. It
// proves currentness but never grants, renews, revokes or otherwise mutates a
// Binding Fact.
type ProviderBindingCurrentProjectionV2 struct {
	ContractVersion          string                        `json:"contract_version"`
	Ref                      ProviderBindingRefV2          `json:"ref"`
	State                    ProviderBindingCurrentStateV2 `json:"state"`
	BindingSetDigest         core.Digest                   `json:"binding_set_digest"`
	BindingSetSemanticDigest core.Digest                   `json:"binding_set_semantic_digest"`
	BindingID                string                        `json:"binding_id"`
	BindingRevision          core.Revision                 `json:"binding_revision"`
	GrantDigest              core.Digest                   `json:"grant_digest"`
	ProjectionDigest         core.Digest                   `json:"projection_digest"`
	IssuedUnixNano           int64                         `json:"issued_unix_nano"`
	ExpiresUnixNano          int64                         `json:"expires_unix_nano"`
}

func (p ProviderBindingCurrentProjectionV2) ValidateCurrent(expected ProviderBindingRefV2, now time.Time) error {
	if p.ContractVersion != ProviderBindingCurrentnessContractVersionV2 || p.Ref.Validate() != nil || expected.Validate() != nil || p.Ref != expected || validateEvidenceIDV2(p.BindingID) != nil || p.BindingRevision == 0 || p.IssuedUnixNano <= 0 || p.ExpiresUnixNano <= p.IssuedUnixNano {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonBindingDrift, "provider Binding currentness projection is incomplete or mismatched")
	}
	if p.BindingSetDigest.Validate() != nil || p.BindingSetSemanticDigest.Validate() != nil || p.GrantDigest.Validate() != nil || p.ProjectionDigest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonBindingDrift, "provider Binding currentness digests are invalid")
	}
	if now.IsZero() {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "provider Binding currentness requires injected time")
	}
	if p.State != ProviderBindingCurrentActiveV2 || now.Before(time.Unix(0, p.IssuedUnixNano)) || !now.Before(time.Unix(0, p.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "provider Binding is inactive or expired")
	}
	digest, err := p.DigestV2()
	if err != nil || digest != p.ProjectionDigest {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "provider Binding currentness projection digest drifted")
	}
	return nil
}

func SealProviderBindingCurrentProjectionV2(p ProviderBindingCurrentProjectionV2) (ProviderBindingCurrentProjectionV2, error) {
	p.ProjectionDigest = EvidenceGenesisDigestV2
	digest, err := p.DigestV2()
	if err != nil {
		return ProviderBindingCurrentProjectionV2{}, err
	}
	p.ProjectionDigest = digest
	return p, nil
}

func (p ProviderBindingCurrentProjectionV2) DigestV2() (core.Digest, error) {
	copy := p
	copy.ProjectionDigest = ""
	return core.CanonicalJSONDigest("praxis.runtime.binding-currentness", ProviderBindingCurrentnessContractVersionV2, "ProviderBindingCurrentProjectionV2", copy)
}

type ProviderBindingCurrentnessPortV2 interface {
	InspectProviderBindingCurrentV2(context.Context, ProviderBindingRefV2) (ProviderBindingCurrentProjectionV2, error)
}
