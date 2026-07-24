package kernel

import (
	"context"
	"sort"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// GenerationBindingAssociationGatewayV1 is the only governance entry point
// that may turn an assembly handoff candidate into a Runtime-owned Fact. It
// deliberately consumes neutral read projections and never imports Harness.
type GenerationBindingAssociationGatewayV1 struct {
	Facts       ports.GenerationBindingAssociationFactPortV1
	Generations ports.GenerationCurrentReaderV1
	Activations ports.GenerationActivationCurrentReaderV1
	Bindings    control.BindingFactPortV2
	Clock       func() time.Time
}

func (g GenerationBindingAssociationGatewayV1) validate() error {
	if g.Facts == nil || g.Generations == nil || g.Activations == nil || g.Bindings == nil || g.Clock == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "generation association Fact owner, current readers, Binding owner and clock are required")
	}
	return nil
}

func (g GenerationBindingAssociationGatewayV1) AssociateGenerationBindingV1(ctx context.Context, candidate ports.GenerationBindingAssociationCandidateV1) (ports.GenerationBindingAssociationFactV1, error) {
	if err := g.validate(); err != nil {
		return ports.GenerationBindingAssociationFactV1{}, err
	}
	if err := candidate.Validate(); err != nil {
		return ports.GenerationBindingAssociationFactV1{}, err
	}

	// Inspect precedes currentness validation. A successful first create remains
	// recoverable after its source leases expire; it grants no new current use.
	existing, err := g.Facts.InspectGenerationBindingAssociationV1(ctx, candidate.AssociationID)
	if err == nil {
		return exactGenerationAssociationV1(existing, candidate)
	}
	if !core.HasCategory(err, core.ErrorNotFound) {
		return ports.GenerationBindingAssociationFactV1{}, err
	}

	now := g.Clock()
	if now.IsZero() {
		return ports.GenerationBindingAssociationFactV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "injected generation association clock is required")
	}
	if err := g.validateCandidateCurrent(ctx, candidate, now); err != nil {
		return ports.GenerationBindingAssociationFactV1{}, err
	}
	fact, err := ports.SealGenerationBindingAssociationFactV1(ports.GenerationBindingAssociationFactV1{
		ContractVersion: ports.GenerationBindingAssociationContractVersionV1,
		ID:              candidate.AssociationID, Revision: 1,
		State:     ports.GenerationBindingAssociationActiveV1,
		Candidate: candidate, CandidateDigest: candidate.Digest,
		CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano(),
		ExpiresUnixNano: minimumAssociationExpiryV1(candidate),
	})
	if err != nil {
		return ports.GenerationBindingAssociationFactV1{}, err
	}
	created, err := g.Facts.CreateGenerationBindingAssociationV1(ctx, fact)
	if err == nil {
		if created.Digest != fact.Digest {
			return ports.GenerationBindingAssociationFactV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "association Fact owner returned different content")
		}
		return created, created.Validate()
	}
	if !core.HasCategory(err, core.ErrorUnavailable) && !core.HasCategory(err, core.ErrorConflict) && !core.HasCategory(err, core.ErrorIndeterminate) {
		return ports.GenerationBindingAssociationFactV1{}, err
	}
	recovered, inspectErr := g.Facts.InspectGenerationBindingAssociationV1(context.WithoutCancel(ctx), candidate.AssociationID)
	if inspectErr != nil {
		return ports.GenerationBindingAssociationFactV1{}, err
	}
	return exactGenerationAssociationV1(recovered, candidate)
}

func (g GenerationBindingAssociationGatewayV1) InspectCurrentGenerationBindingAssociationV1(ctx context.Context, associationID string) (ports.GenerationBindingAssociationFactV1, error) {
	if err := g.validate(); err != nil {
		return ports.GenerationBindingAssociationFactV1{}, err
	}
	if associationID == "" {
		return ports.GenerationBindingAssociationFactV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "association ID is required")
	}
	fact, err := g.Facts.InspectGenerationBindingAssociationV1(ctx, associationID)
	if err != nil {
		return ports.GenerationBindingAssociationFactV1{}, err
	}
	if err := fact.Validate(); err != nil {
		return ports.GenerationBindingAssociationFactV1{}, err
	}
	now := g.Clock()
	if fact.State != ports.GenerationBindingAssociationActiveV1 || now.IsZero() || !now.Before(time.Unix(0, fact.ExpiresUnixNano)) {
		return ports.GenerationBindingAssociationFactV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "generation association is inactive or expired")
	}
	if err := g.validateCandidateCurrent(ctx, fact.Candidate, now); err != nil {
		return ports.GenerationBindingAssociationFactV1{}, err
	}
	return fact, nil
}

func (g GenerationBindingAssociationGatewayV1) validateCandidateCurrent(ctx context.Context, candidate ports.GenerationBindingAssociationCandidateV1, now time.Time) error {
	generation, err := g.Generations.InspectGenerationCurrentV1(ctx, candidate.Generation.Generation)
	if err != nil {
		return err
	}
	if err := generation.ValidateCurrent(candidate.Generation.Generation, now); err != nil || generation.ProjectionDigest != candidate.Generation.ProjectionDigest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "sealed generation current projection drifted")
	}
	binding, err := BuildGenerationBindingSetCurrentProjectionV1(ctx, g.Bindings, candidate.Generation.ComponentManifests, candidate.Binding.BindingSetID, now)
	if err != nil {
		return err
	}
	if binding.ProjectionDigest != candidate.Binding.ProjectionDigest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "BindingSet current projection drifted")
	}
	activation, err := g.Activations.InspectGenerationActivationCurrentV1(ctx, candidate.Activation.Operation)
	if err != nil {
		return err
	}
	if err := activation.ValidateCurrent(candidate.Activation.Operation, now); err != nil || activation.ProjectionDigest != candidate.Activation.ProjectionDigest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonActivationFactDrift, "activation current projection drifted")
	}
	if !now.Before(time.Unix(0, candidate.RequestedExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "association request expired")
	}
	return nil
}

// BuildGenerationBindingSetCurrentProjectionV1 reads the complete BindingSet,
// including non-generation governance members. The generation component list
// must be an exact subset of current members; no caller-supplied member state is
// trusted.
func BuildGenerationBindingSetCurrentProjectionV1(ctx context.Context, facts control.BindingFactPortV2, generationComponents []ports.GenerationComponentManifestRefV1, setID string, now time.Time) (ports.GenerationBindingSetCurrentProjectionV1, error) {
	if facts == nil || now.IsZero() || setID == "" {
		return ports.GenerationBindingSetCurrentProjectionV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Binding Fact reader, set ID and clock are required")
	}
	set, err := facts.InspectBindingSet(ctx, setID)
	if err != nil {
		return ports.GenerationBindingSetCurrentProjectionV1{}, err
	}
	if err := set.Validate(); err != nil {
		return ports.GenerationBindingSetCurrentProjectionV1{}, err
	}
	if set.State != control.BindingSetActive || !now.Before(time.Unix(0, set.ExpiresUnixNano)) {
		return ports.GenerationBindingSetCurrentProjectionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "BindingSet is not current")
	}
	components := append([]ports.GenerationComponentManifestRefV1{}, generationComponents...)
	sort.Slice(components, func(i, j int) bool { return components[i].ComponentID < components[j].ComponentID })
	requested := make(map[ports.ComponentIDV2]ports.GenerationComponentManifestRefV1, len(components))
	for _, component := range components {
		if err := component.Validate(); err != nil {
			return ports.GenerationBindingSetCurrentProjectionV1{}, err
		}
		if _, exists := requested[component.ComponentID]; exists {
			return ports.GenerationBindingSetCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "generation contains duplicate component manifest")
		}
		requested[component.ComponentID] = component
	}
	type memberWatermark struct {
		BindingID       string        `json:"binding_id"`
		BindingRevision core.Revision `json:"binding_revision"`
		ManifestDigest  core.Digest   `json:"manifest_digest"`
		GrantSetDigest  core.Digest   `json:"grant_set_digest"`
		ExpiresUnixNano int64         `json:"expires_unix_nano"`
	}
	watermarks := make([]memberWatermark, 0, len(set.Members))
	currentFacts := make([]control.BindingFactV2, 0, len(set.Members))
	probes := make([]control.BindingCurrentProbeV2, 0, len(set.Members))
	expires := set.ExpiresUnixNano
	for _, member := range set.Members {
		fact, inspectErr := facts.InspectBinding(ctx, member.BindingID)
		if inspectErr != nil {
			return ports.GenerationBindingSetCurrentProjectionV1{}, inspectErr
		}
		if err := fact.Validate(); err != nil {
			return ports.GenerationBindingSetCurrentProjectionV1{}, err
		}
		memberGrants, memberErr := control.BindingGrantSetDigestV2(member.Grants)
		factGrants, factErr := control.BindingGrantSetDigestV2(fact.Grants)
		if memberErr != nil || factErr != nil || memberGrants != factGrants || fact.State != control.BindingBound || fact.BindingSetID != set.ID || fact.ID != member.BindingID || fact.Revision != member.BindingRevision || fact.ComponentID != member.ComponentID || fact.ManifestDigest != member.ManifestDigest || fact.Manifest.ArtifactDigest != member.ArtifactDigest || !now.Before(time.Unix(0, fact.ExpiresUnixNano)) {
			return ports.GenerationBindingSetCurrentProjectionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "Binding member Fact or grant drifted")
		}
		if expected, ok := requested[member.ComponentID]; ok {
			if expected.ManifestDigest != member.ManifestDigest || expected.ArtifactDigest != member.ArtifactDigest {
				return ports.GenerationBindingSetCurrentProjectionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonComponentMismatch, "generation component does not match its bound manifest")
			}
			delete(requested, member.ComponentID)
		}
		if fact.ExpiresUnixNano < expires {
			expires = fact.ExpiresUnixNano
		}
		currentFacts = append(currentFacts, fact)
		probes = append(probes, control.BindingCurrentProbeV2{ComponentID: member.ComponentID, ManifestDigest: fact.ManifestDigest})
		watermarks = append(watermarks, memberWatermark{member.BindingID, member.BindingRevision, member.ManifestDigest, factGrants, fact.ExpiresUnixNano})
	}
	if len(requested) != 0 {
		return ports.GenerationBindingSetCurrentProjectionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonComponentMismatch, "generation contains a component absent from the current BindingSet")
	}
	if err := control.ValidateBindingSetCurrentV2(set, currentFacts, probes, now); err != nil {
		return ports.GenerationBindingSetCurrentProjectionV1{}, err
	}
	setDigest, err := control.BindingSetDigestV2(set)
	if err != nil {
		return ports.GenerationBindingSetCurrentProjectionV1{}, err
	}
	semanticDigest, err := control.BindingSetSemanticDigestV2(set)
	if err != nil {
		return ports.GenerationBindingSetCurrentProjectionV1{}, err
	}
	currentness, err := core.CanonicalJSONDigest("praxis.runtime.generation-binding", ports.GenerationBindingAssociationContractVersionV1, "GenerationBindingSetCurrentnessV1", struct {
		SetDigest  core.Digest       `json:"binding_set_digest"`
		Watermarks []memberWatermark `json:"member_watermarks"`
	}{setDigest, watermarks})
	if err != nil {
		return ports.GenerationBindingSetCurrentProjectionV1{}, err
	}
	return ports.SealGenerationBindingSetCurrentProjectionV1(ports.GenerationBindingSetCurrentProjectionV1{
		ContractVersion: ports.GenerationBindingAssociationContractVersionV1,
		BindingSetID:    set.ID, BindingSetRevision: set.Revision,
		BindingSetDigest: setDigest, BindingSetSemanticDigest: semanticDigest,
		PlanDigest: set.PlanDigest, GovernanceDigest: set.GovernanceDigest,
		ComponentManifestSetDigest: ports.GenerationComponentManifestSetDigestV1(components),
		CurrentnessDigest:          currentness, IssuedUnixNano: set.CreatedUnixNano,
		ExpiresUnixNano: expires,
	})
}

func exactGenerationAssociationV1(fact ports.GenerationBindingAssociationFactV1, candidate ports.GenerationBindingAssociationCandidateV1) (ports.GenerationBindingAssociationFactV1, error) {
	if err := fact.Validate(); err != nil {
		return ports.GenerationBindingAssociationFactV1{}, err
	}
	if fact.CandidateDigest != candidate.Digest || fact.Candidate.Digest != candidate.Digest {
		return ports.GenerationBindingAssociationFactV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "association ID already binds different content")
	}
	return fact, nil
}

func minimumAssociationExpiryV1(candidate ports.GenerationBindingAssociationCandidateV1) int64 {
	minimum := candidate.RequestedExpiresUnixNano
	for _, value := range []int64{candidate.Generation.ExpiresUnixNano, candidate.Binding.ExpiresUnixNano, candidate.Activation.ExpiresUnixNano} {
		if value < minimum {
			minimum = value
		}
	}
	return minimum
}

var _ ports.GenerationBindingAssociationGovernancePortV1 = GenerationBindingAssociationGatewayV1{}
