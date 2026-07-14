package control

import (
	"context"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// ProviderBindingCurrentnessAdapterV2 exposes only a read-only currentness
// projection to Application and external adapters. Binding facts and mutation
// methods remain owned by the Runtime control plane.
type ProviderBindingCurrentnessAdapterV2 struct {
	Bindings BindingFactPortV2
	Clock    func() time.Time
}

func (a ProviderBindingCurrentnessAdapterV2) InspectProviderBindingCurrentV2(ctx context.Context, expected ports.ProviderBindingRefV2) (ports.ProviderBindingCurrentProjectionV2, error) {
	if a.Bindings == nil || a.Clock == nil {
		return ports.ProviderBindingCurrentProjectionV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Binding Fact owner and clock are required")
	}
	if err := expected.Validate(); err != nil {
		return ports.ProviderBindingCurrentProjectionV2{}, err
	}
	now := a.Clock()
	set, err := a.Bindings.InspectBindingSet(ctx, expected.BindingSetID)
	if err != nil {
		return ports.ProviderBindingCurrentProjectionV2{}, err
	}
	setDigest, err := BindingSetDigestV2(set)
	if err != nil {
		return ports.ProviderBindingCurrentProjectionV2{}, err
	}
	semanticDigest, err := BindingSetSemanticDigestV2(set)
	if err != nil {
		return ports.ProviderBindingCurrentProjectionV2{}, err
	}
	if set.Revision != expected.BindingSetRevision || set.State != BindingSetActive || !now.Before(time.Unix(0, set.ExpiresUnixNano)) {
		return ports.ProviderBindingCurrentProjectionV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "provider BindingSet is not current")
	}
	var member BindingMemberV2
	found := false
	for _, candidate := range set.Members {
		if candidate.ComponentID == expected.ComponentID {
			member = candidate
			found = true
			break
		}
	}
	if !found || member.ManifestDigest != expected.ManifestDigest || member.ArtifactDigest != expected.ArtifactDigest {
		return ports.ProviderBindingCurrentProjectionV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "provider Binding member identity drifted")
	}
	fact, err := a.Bindings.InspectBinding(ctx, member.BindingID)
	if err != nil {
		return ports.ProviderBindingCurrentProjectionV2{}, err
	}
	if err := fact.Validate(); err != nil {
		return ports.ProviderBindingCurrentProjectionV2{}, err
	}
	if fact.State != BindingBound || fact.BindingSetID != set.ID || fact.Revision != member.BindingRevision || fact.ManifestDigest != member.ManifestDigest || fact.Manifest.ArtifactDigest != member.ArtifactDigest || !now.Before(time.Unix(0, fact.ExpiresUnixNano)) {
		return ports.ProviderBindingCurrentProjectionV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "provider Binding Fact drifted from the current set")
	}
	probes := make([]BindingCurrentProbeV2, 0, len(set.Members))
	facts := make([]BindingFactV2, 0, len(set.Members))
	currentWatermarkExpiry := set.ExpiresUnixNano
	for _, currentMember := range set.Members {
		currentFact, inspectErr := a.Bindings.InspectBinding(ctx, currentMember.BindingID)
		if inspectErr != nil {
			return ports.ProviderBindingCurrentProjectionV2{}, inspectErr
		}
		if err := currentFact.Validate(); err != nil {
			return ports.ProviderBindingCurrentProjectionV2{}, err
		}
		if !now.Before(time.Unix(0, currentFact.ExpiresUnixNano)) {
			return ports.ProviderBindingCurrentProjectionV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "a Binding member Fact is expired")
		}
		if currentFact.ExpiresUnixNano < currentWatermarkExpiry {
			currentWatermarkExpiry = currentFact.ExpiresUnixNano
		}
		memberGrantDigest, grantErr := BindingGrantSetDigestV2(currentMember.Grants)
		factGrantDigest, factGrantErr := BindingGrantSetDigestV2(currentFact.Grants)
		if grantErr != nil || factGrantErr != nil || memberGrantDigest != factGrantDigest {
			return ports.ProviderBindingCurrentProjectionV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "provider Binding grants drifted from the current Fact")
		}
		facts = append(facts, currentFact)
		probes = append(probes, BindingCurrentProbeV2{ComponentID: currentMember.ComponentID, ManifestDigest: currentFact.ManifestDigest})
	}
	if err := ValidateBindingSetCurrentV2(set, facts, probes, now); err != nil {
		return ports.ProviderBindingCurrentProjectionV2{}, err
	}
	var grant ports.CapabilityGrantV2
	grantFound := false
	for _, candidate := range member.Grants {
		if candidate.Capability == expected.Capability {
			grant = candidate
			grantFound = true
			break
		}
	}
	if !grantFound || !now.Before(time.Unix(0, grant.ExpiresUnixNano)) {
		return ports.ProviderBindingCurrentProjectionV2{}, core.NewError(core.ErrorForbidden, core.ReasonUnknownCapability, "provider capability grant is missing or expired")
	}
	grantDigest, err := core.CanonicalJSONDigest("praxis.runtime.binding", ports.ProviderBindingCurrentnessContractVersionV2, "CapabilityGrantV2", grant)
	if err != nil {
		return ports.ProviderBindingCurrentProjectionV2{}, err
	}
	expires := currentWatermarkExpiry
	if fact.ExpiresUnixNano < expires {
		expires = fact.ExpiresUnixNano
	}
	if grant.ExpiresUnixNano < expires {
		expires = grant.ExpiresUnixNano
	}
	projection, err := ports.SealProviderBindingCurrentProjectionV2(ports.ProviderBindingCurrentProjectionV2{
		ContractVersion: ports.ProviderBindingCurrentnessContractVersionV2,
		Ref:             expected, State: ports.ProviderBindingCurrentActiveV2,
		BindingSetDigest: setDigest, BindingSetSemanticDigest: semanticDigest,
		BindingID: member.BindingID, BindingRevision: member.BindingRevision,
		GrantDigest: grantDigest, IssuedUnixNano: grant.ObservedUnixNano, ExpiresUnixNano: expires,
	})
	if err != nil {
		return ports.ProviderBindingCurrentProjectionV2{}, err
	}
	return projection, projection.ValidateCurrent(expected, now)
}

var _ ports.ProviderBindingCurrentnessPortV2 = ProviderBindingCurrentnessAdapterV2{}
