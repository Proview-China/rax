package runtimeadapter

import (
	"context"
	"errors"
	"fmt"
	"time"

	runtimecore "github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

type CurrentReaderV4 struct {
	store       ports.ExactCurrentStore
	generations runtimeports.GenerationBindingAssociationGovernancePortV1
	now         func() time.Time
}

func NewCurrentReaderV4(store ports.ExactCurrentStore, generations runtimeports.GenerationBindingAssociationGovernancePortV1, now func() time.Time) (*CurrentReaderV4, error) {
	if store == nil || generations == nil || now == nil {
		return nil, errors.New("exact current store, generation reader, and clock are required")
	}
	return &CurrentReaderV4{store: store, generations: generations, now: now}, nil
}

var _ runtimeports.OperationDispatchSandboxCurrentReaderV4 = (*CurrentReaderV4)(nil)

func (r *CurrentReaderV4) InspectOperationDispatchSandboxCurrentV4(ctx context.Context, operation runtimeports.OperationSubjectV3, effectID runtimecore.EffectIntentID, expectedAttempt runtimeports.OperationDispatchSandboxFactRefV4) (runtimeports.OperationDispatchSandboxCurrentProjectionV4, error) {
	now := r.now()
	if now.IsZero() {
		return runtimeports.OperationDispatchSandboxCurrentProjectionV4{}, errors.New("sandbox current reader clock returned zero")
	}
	if err := operation.Validate(); err != nil {
		return runtimeports.OperationDispatchSandboxCurrentProjectionV4{}, err
	}
	operationDigest, err := operation.DigestV3()
	if err != nil {
		return runtimeports.OperationDispatchSandboxCurrentProjectionV4{}, err
	}
	operationID := exactOperationID(operation)
	if operationID == "" || effectID == "" {
		return runtimeports.OperationDispatchSandboxCurrentProjectionV4{}, errors.New("exact operation, effect, and attempt coordinates are required")
	}
	if err := expectedAttempt.Validate(); err != nil {
		return runtimeports.OperationDispatchSandboxCurrentProjectionV4{}, err
	}
	attempt, err := r.store.GetAttempt(ctx, expectedAttempt.ID)
	if err != nil {
		return runtimeports.OperationDispatchSandboxCurrentProjectionV4{}, err
	}
	if err := attempt.ValidateCurrent(now); err != nil {
		return runtimeports.OperationDispatchSandboxCurrentProjectionV4{}, fmt.Errorf("attempt currentness: %w", err)
	}
	if runtimeFactRef(attempt.Meta) != expectedAttempt {
		return runtimeports.OperationDispatchSandboxCurrentProjectionV4{}, errors.New("attempt exact ref drifted")
	}
	attemptID := attempt.AttemptID

	reservation, err := r.store.InspectReservationByAttempt(ctx, operationID, string(effectID), attemptID)
	if err != nil {
		return runtimeports.OperationDispatchSandboxCurrentProjectionV4{}, err
	}
	if err := reservation.ValidateCurrent(now); err != nil {
		return runtimeports.OperationDispatchSandboxCurrentProjectionV4{}, fmt.Errorf("reservation currentness: %w", err)
	}
	if reservation.OperationID != operationID || reservation.EffectID != string(effectID) || reservation.AttemptID != attemptID || reservation.OperationSubjectDigest != string(operationDigest) {
		return runtimeports.OperationDispatchSandboxCurrentProjectionV4{}, errors.New("reservation dispatch coordinates drifted")
	}

	leaseFact, err := r.store.GetRuntimeLeaseBinding(ctx, reservation.RuntimeLeaseBindingRef.ID)
	if err != nil {
		return runtimeports.OperationDispatchSandboxCurrentProjectionV4{}, err
	}
	projection, err := r.store.GetProjection(ctx, reservation.Lease.LeaseID)
	if err != nil {
		return runtimeports.OperationDispatchSandboxCurrentProjectionV4{}, err
	}
	requirement, err := r.store.GetRequirement(ctx, reservation.RequirementRef.ID)
	if err != nil {
		return runtimeports.OperationDispatchSandboxCurrentProjectionV4{}, err
	}
	policy, err := r.store.GetPolicy(ctx, reservation.PolicyRef.ID)
	if err != nil {
		return runtimeports.OperationDispatchSandboxCurrentProjectionV4{}, err
	}
	placement, err := r.store.GetPlacement(ctx, reservation.PlacementRef.ID)
	if err != nil {
		return runtimeports.OperationDispatchSandboxCurrentProjectionV4{}, err
	}
	backend, err := r.store.GetBackend(ctx, reservation.BackendRef.ID)
	if err != nil {
		return runtimeports.OperationDispatchSandboxCurrentProjectionV4{}, err
	}
	slot, err := r.store.GetSlot(ctx, reservation.SlotRef.ID)
	if err != nil {
		return runtimeports.OperationDispatchSandboxCurrentProjectionV4{}, err
	}
	generation, err := r.generations.InspectCurrentGenerationBindingAssociationV1(ctx, reservation.GenerationBindingRef.ID)
	if err != nil {
		return runtimeports.OperationDispatchSandboxCurrentProjectionV4{}, err
	}

	for name, validate := range map[string]func() error{
		"runtime lease": func() error { return leaseFact.ValidateCurrent(now) },
		"projection":    func() error { return projection.ValidateCurrent(now) },
		"requirement":   func() error { return requirement.ValidateCurrent(now) },
		"policy":        func() error { return policy.ValidateCurrent(now) },
		"placement":     func() error { return placement.ValidateCurrent(now) },
		"backend":       func() error { return backend.ValidateCurrent(now) },
		"slot":          func() error { return slot.ValidateCurrent(now) },
	} {
		if err := validate(); err != nil {
			return runtimeports.OperationDispatchSandboxCurrentProjectionV4{}, fmt.Errorf("%s currentness: %w", name, err)
		}
	}
	if err := generation.Validate(); err != nil || generation.State != runtimeports.GenerationBindingAssociationActiveV1 || !now.Before(time.Unix(0, generation.ExpiresUnixNano)) {
		return runtimeports.OperationDispatchSandboxCurrentProjectionV4{}, errors.New("generation binding association is not current")
	}

	if err := validateExactGraph(operation, attempt, reservation, leaseFact, projection, requirement, policy, placement, backend, slot, generation); err != nil {
		return runtimeports.OperationDispatchSandboxCurrentProjectionV4{}, err
	}
	provider := runtimeProvider(reservation.ProviderBinding)
	if err := provider.Validate(); err != nil {
		return runtimeports.OperationDispatchSandboxCurrentProjectionV4{}, err
	}

	expires := minimumExpiry(
		attempt.Meta.ExpiresUnixNano, reservation.Meta.ExpiresUnixNano, leaseFact.Meta.ExpiresUnixNano, leaseFact.Binding.ExpiresUnixNano,
		projection.Meta.ExpiresUnixNano, requirement.Meta.ExpiresUnixNano, policy.Meta.ExpiresUnixNano,
		placement.Meta.ExpiresUnixNano, backend.Meta.ExpiresUnixNano, slot.Meta.ExpiresUnixNano,
		generation.ExpiresUnixNano,
	)
	result := runtimeports.OperationDispatchSandboxCurrentProjectionV4{
		Operation: operation, OperationDigest: operationDigest, EffectID: effectID,
		IntentRevision: runtimecore.Revision(reservation.IntentRevision), IntentDigest: runtimeDigest(reservation.IntentDigest), AttemptID: attemptID,
		Attempt: runtimeFactRef(attempt.Meta), Reservation: runtimeFactRef(reservation.Meta),
		SandboxLease: runtimecore.SandboxLeaseRef{ID: runtimecore.SandboxLeaseID(reservation.Lease.LeaseID), Epoch: runtimecore.Epoch(reservation.Lease.LeaseEpoch)},
		RuntimeLease: runtimeports.OperationDispatchRuntimeLeaseBindingV4{
			Ref:        runtimeFactRef(leaseFact.Meta),
			Lease:      runtimecore.SandboxLeaseRef{ID: runtimecore.SandboxLeaseID(leaseFact.Binding.LeaseID), Epoch: runtimecore.Epoch(leaseFact.Binding.LeaseEpoch)},
			Instance:   runtimecore.InstanceRef{ID: runtimecore.AgentInstanceID(leaseFact.Binding.InstanceID), Epoch: runtimecore.Epoch(leaseFact.Binding.InstanceEpoch)},
			FenceEpoch: runtimecore.Epoch(leaseFact.Binding.FenceEpoch), ScopeDigest: runtimeDigest(leaseFact.Binding.ScopeDigest), ObservedRevision: runtimecore.Revision(leaseFact.Binding.ObservedRevision),
		},
		Generation: generation.RefV1(), Placement: runtimeFactRef(placement.Meta), Backend: runtimeFactRef(backend.Meta), Slot: runtimeFactRef(slot.Meta),
		ProviderBinding: provider, Current: true, ProjectionRevision: runtimecore.Revision(projection.Meta.Revision), ExpiresUnixNano: expires,
	}
	sealed, err := runtimeports.SealOperationDispatchSandboxCurrentProjectionV4(result)
	if err != nil {
		return runtimeports.OperationDispatchSandboxCurrentProjectionV4{}, err
	}
	return sealed, nil
}

func validateExactGraph(operation runtimeports.OperationSubjectV3, attempt contract.DomainAttemptFact, reservation contract.DomainReservation, leaseFact contract.RuntimeLeaseBindingFact, projection contract.EnvironmentProjection, requirement contract.ExecutionRequirement, policy contract.PolicyProjection, placement contract.PlacementCandidate, backend contract.BackendDescriptor, slot contract.SlotCandidate, generation runtimeports.GenerationBindingAssociationFactV1) error {
	if !contract.SameRef(reservation.AttemptRef, attempt.Meta.Ref()) || !contract.SameRef(attempt.ReservationRef, reservation.Meta.Ref()) || attempt.OperationID != reservation.OperationID || attempt.EffectID != reservation.EffectID || attempt.IntentRevision != reservation.IntentRevision || attempt.IntentDigest != reservation.IntentDigest || attempt.AttemptID != reservation.AttemptID || !contract.SameRef(attempt.RuntimeLeaseBindingRef, reservation.RuntimeLeaseBindingRef) {
		return errors.New("attempt or reservation binding drifted")
	}
	if !contract.SameRef(reservation.RuntimeLeaseBindingRef, leaseFact.Meta.Ref()) || !contract.SameRuntimeLeaseBinding(reservation.Lease, leaseFact.Binding) || !contract.SameRuntimeLeaseBinding(reservation.Lease, projection.Lease) || projection.Meta.Revision != reservation.ExpectedProjectionRevision {
		return errors.New("runtime lease or projection binding drifted")
	}
	if operation.ExecutionScope.SandboxLease == nil || string(operation.ExecutionScope.Identity.TenantID) != reservation.Lease.TenantID || string(operation.ExecutionScope.Instance.ID) != reservation.Lease.InstanceID || uint64(operation.ExecutionScope.Instance.Epoch) != reservation.Lease.InstanceEpoch || string(operation.ExecutionScope.SandboxLease.ID) != reservation.Lease.LeaseID || uint64(operation.ExecutionScope.SandboxLease.Epoch) != reservation.Lease.LeaseEpoch || string(operation.ExecutionScopeDigest) != reservation.Lease.ScopeDigest {
		return errors.New("tenant, instance, lease, or scope drifted")
	}
	if !contract.SameRef(reservation.RequirementRef, requirement.Meta.Ref()) || !contract.SameRef(reservation.PolicyRef, policy.Meta.Ref()) || !contract.SameRef(policy.RequirementRef, requirement.Meta.Ref()) || policy.ScopeDigest != reservation.Lease.ScopeDigest {
		return errors.New("requirement, policy, or scope binding drifted")
	}
	for name, exact := range map[string]bool{
		"reservation placement": contract.SameRef(reservation.PlacementRef, placement.Meta.Ref()),
		"reservation backend":   contract.SameRef(reservation.BackendRef, backend.Meta.Ref()),
		"reservation slot":      contract.SameRef(reservation.SlotRef, slot.Meta.Ref()),
		"placement requirement": contract.SameRef(placement.RequirementRef, requirement.Meta.Ref()),
		"placement policy":      contract.SameRef(placement.PolicyRef, policy.Meta.Ref()),
		"placement backend":     contract.SameRef(placement.BackendRef, backend.Meta.Ref()),
		"placement slot":        contract.SameRef(placement.SlotCandidateRef, slot.Meta.Ref()),
		"slot placement":        contract.SameRef(slot.PlacementRef, placement.Meta.Ref()),
		"slot backend":          contract.SameRef(slot.BackendRef, backend.Meta.Ref()),
		"slot provider":         slot.ProviderBinding == reservation.ProviderBinding,
	} {
		if !exact {
			return fmt.Errorf("%s binding drifted", name)
		}
	}
	if generation.RefV1().ID != reservation.GenerationBindingRef.ID || uint64(generation.RefV1().Revision) != reservation.GenerationBindingRef.Revision || string(generation.RefV1().Digest) != reservation.GenerationBindingRef.Digest {
		return errors.New("generation binding association drifted")
	}
	provider := runtimeProvider(reservation.ProviderBinding)
	if generation.Candidate.Binding.BindingSetID != provider.BindingSetID || generation.Candidate.Binding.BindingSetRevision != provider.BindingSetRevision || !runtimeports.SameExecutionScopeV2(generation.Candidate.Activation.Operation.ExecutionScope, operation.ExecutionScope) {
		return errors.New("generation binding set or activation scope drifted from provider execution")
	}
	manifestMatched := false
	for _, component := range generation.Candidate.Generation.ComponentManifests {
		if component.ComponentID == provider.ComponentID && component.ManifestDigest == provider.ManifestDigest && component.ArtifactDigest == provider.ArtifactDigest {
			manifestMatched = true
			break
		}
	}
	if !manifestMatched {
		return errors.New("provider is absent from the exact generation manifest set")
	}
	return nil
}

func exactOperationID(operation runtimeports.OperationSubjectV3) string {
	switch operation.Kind {
	case runtimeports.OperationScopeActivationV3:
		return operation.ActivationAttemptID
	case runtimeports.OperationScopeRunV3:
		return string(operation.RunID)
	case runtimeports.OperationScopeTerminationV3:
		return operation.TerminationAttemptID
	case runtimeports.OperationScopeAdminV3:
		return operation.AdminOperationID
	default:
		return operation.CustomOperationID
	}
}

func runtimeFactRef(meta contract.Meta) runtimeports.OperationDispatchSandboxFactRefV4 {
	return runtimeports.OperationDispatchSandboxFactRefV4{ID: meta.ID, Revision: runtimecore.Revision(meta.Revision), Digest: runtimeDigest(meta.Digest), ExpiresUnixNano: meta.ExpiresUnixNano}
}

func runtimeProvider(value contract.ProviderBindingRef) runtimeports.ProviderBindingRefV2 {
	return runtimeports.ProviderBindingRefV2{BindingSetID: value.BindingSetID, BindingSetRevision: runtimecore.Revision(value.BindingSetRevision), ComponentID: runtimeports.ComponentIDV2(value.ComponentID), ManifestDigest: runtimeDigest(value.ManifestDigest), ArtifactDigest: runtimeDigest(value.ArtifactDigest), Capability: runtimeports.CapabilityNameV2(value.Capability)}
}

func runtimeDigest(value string) runtimecore.Digest {
	if len(value) >= len("sha256:") && value[:len("sha256:")] == "sha256:" {
		return runtimecore.Digest(value)
	}
	return runtimecore.Digest("sha256:" + value)
}

func minimumExpiry(values ...int64) int64 {
	minimum := values[0]
	for _, value := range values[1:] {
		if value < minimum {
			minimum = value
		}
	}
	return minimum
}
