package runtimeadapter

import (
	"context"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/contract"
	harnessports "github.com/Proview-China/rax/ExecutionRuntime/harness/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const (
	maxCommittedPendingActionApplicabilityTTLV1 = 30 * time.Second
	operationScopeEvidenceCanonicalDomainV3     = "praxis.runtime.operation-scope-evidence"
)

type pendingActionApplicabilityKeyV1 struct {
	Kind     runtimeports.NamespacedNameV2
	ID       string
	Revision core.Revision
	Digest   core.Digest
}

type pendingActionApplicabilityEntryV1 struct {
	Binding     contract.CommittedPendingActionApplicabilityBindingV1
	ExpectedRef runtimeports.OperationScopeEvidenceApplicabilityFactRefV3
}

// CommittedPendingActionApplicabilityCurrentReaderV3 is an immutable adapter
// over a finite, constructor-sealed Binding set. It exposes no registration,
// deletion, replacement, or Fact Store API.
type CommittedPendingActionApplicabilityCurrentReaderV3 struct {
	reader   harnessports.CommittedPendingActionReaderV1
	clock    func() time.Time
	bindings map[pendingActionApplicabilityKeyV1]pendingActionApplicabilityEntryV1
}

var _ runtimeports.OperationScopeEvidenceApplicabilityCurrentReaderV3 = (*CommittedPendingActionApplicabilityCurrentReaderV3)(nil)

func NewCommittedPendingActionApplicabilityCurrentReaderV3(
	reader harnessports.CommittedPendingActionReaderV1,
	bindings []contract.CommittedPendingActionApplicabilityBindingV1,
	clock func() time.Time,
) (*CommittedPendingActionApplicabilityCurrentReaderV3, error) {
	if reader == nil || clock == nil || len(bindings) == 0 {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "committed PendingAction applicability current Reader requires a Reader, clock, and finite Bindings")
	}
	entries := make(map[pendingActionApplicabilityKeyV1]pendingActionApplicabilityEntryV1, len(bindings)*2)
	for index := range bindings {
		binding := bindings[index].Clone()
		if err := binding.Validate(); err != nil {
			return nil, err
		}
		refs := []runtimeports.OperationScopeEvidenceApplicabilityFactRefV3{
			sessionApplicabilityRefV3(binding.ExpectedSessionCoordinate),
			turnApplicabilityRefV3(binding.ExpectedTurnCoordinate),
		}
		for _, ref := range refs {
			key := pendingActionApplicabilityKeyFromRefV1(ref)
			if current, exists := entries[key]; exists {
				if current.Binding.Digest == binding.Digest && current.ExpectedRef == ref {
					continue
				}
				return nil, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "committed PendingAction applicability Binding key has conflicting content")
			}
			entries[key] = pendingActionApplicabilityEntryV1{Binding: binding.Clone(), ExpectedRef: ref}
		}
	}
	return &CommittedPendingActionApplicabilityCurrentReaderV3{reader: reader, clock: clock, bindings: entries}, nil
}

func (r *CommittedPendingActionApplicabilityCurrentReaderV3) InspectOperationScopeEvidenceApplicabilityCurrentV3(
	ctx context.Context,
	ref runtimeports.OperationScopeEvidenceApplicabilityFactRefV3,
) (runtimeports.OperationScopeEvidenceApplicabilityCurrentProjectionV3, error) {
	if r == nil || r.reader == nil || r.clock == nil || r.bindings == nil {
		return runtimeports.OperationScopeEvidenceApplicabilityCurrentProjectionV3{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "committed PendingAction applicability current Reader is unavailable")
	}
	if ctx == nil {
		return runtimeports.OperationScopeEvidenceApplicabilityCurrentProjectionV3{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "committed PendingAction applicability current Reader requires context")
	}
	if err := ref.Validate(); err != nil {
		return runtimeports.OperationScopeEvidenceApplicabilityCurrentProjectionV3{}, err
	}
	entry, exists := r.bindings[pendingActionApplicabilityKeyFromRefV1(ref)]
	if !exists || entry.ExpectedRef != ref {
		return runtimeports.OperationScopeEvidenceApplicabilityCurrentProjectionV3{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "committed PendingAction applicability Binding is missing")
	}
	if err := entry.Binding.Validate(); err != nil {
		return runtimeports.OperationScopeEvidenceApplicabilityCurrentProjectionV3{}, err
	}
	startedAt := r.clock()
	if startedAt.IsZero() {
		return runtimeports.OperationScopeEvidenceApplicabilityCurrentProjectionV3{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectFenceStale, "committed PendingAction applicability clock is unavailable")
	}
	request, err := entry.Binding.Subject.InspectRequestAtV1(startedAt)
	if err != nil {
		return runtimeports.OperationScopeEvidenceApplicabilityCurrentProjectionV3{}, err
	}
	current, err := r.reader.InspectCommittedPendingActionCurrentV1(ctx, request)
	if err != nil {
		return runtimeports.OperationScopeEvidenceApplicabilityCurrentProjectionV3{}, err
	}
	verifiedAt := r.clock()
	if verifiedAt.IsZero() || verifiedAt.UnixNano() < current.CheckedUnixNano || !verifiedAt.Before(time.Unix(0, current.ExpiresUnixNano)) {
		return runtimeports.OperationScopeEvidenceApplicabilityCurrentProjectionV3{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectFenceStale, "committed PendingAction applicability verification clock is stale or expired")
	}
	if err := current.Validate(request, verifiedAt); err != nil {
		return runtimeports.OperationScopeEvidenceApplicabilityCurrentProjectionV3{}, err
	}
	if current.SessionApplicability != entry.Binding.ExpectedSessionCoordinate || current.TurnApplicability != entry.Binding.ExpectedTurnCoordinate || current.ExecutionScopeDigest != request.ExecutionScopeDigest || current.ExpiresUnixNano-current.CheckedUnixNano > int64(maxCommittedPendingActionApplicabilityTTLV1) {
		return runtimeports.OperationScopeEvidenceApplicabilityCurrentProjectionV3{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "committed PendingAction applicability source or current lease drifted")
	}
	result := runtimeports.OperationScopeEvidenceApplicabilityCurrentProjectionV3{
		Fact:                 ref,
		ExecutionScopeDigest: current.ExecutionScopeDigest,
		Current:              true,
		ExpiresUnixNano:      current.ExpiresUnixNano,
	}
	copy := result
	copy.Digest = ""
	result.Digest, err = core.CanonicalJSONDigest(
		operationScopeEvidenceCanonicalDomainV3,
		runtimeports.OperationScopeEvidenceContractVersionV3,
		"OperationScopeEvidenceApplicabilityCurrentProjectionV3",
		copy,
	)
	if err != nil {
		return runtimeports.OperationScopeEvidenceApplicabilityCurrentProjectionV3{}, err
	}
	if result.ExpiresUnixNano > current.ExpiresUnixNano {
		return runtimeports.OperationScopeEvidenceApplicabilityCurrentProjectionV3{}, core.NewError(core.ErrorConflict, core.ReasonCapabilityExpired, "public applicability projection exceeded the Harness lease")
	}
	if err := result.Validate(ref, current.ExecutionScopeDigest, verifiedAt); err != nil {
		return runtimeports.OperationScopeEvidenceApplicabilityCurrentProjectionV3{}, err
	}
	return result, nil
}

func sessionApplicabilityRefV3(coordinate contract.CommittedPendingActionSessionApplicabilityCoordinateV1) runtimeports.OperationScopeEvidenceApplicabilityFactRefV3 {
	return runtimeports.OperationScopeEvidenceApplicabilityFactRefV3{Kind: coordinate.Kind, ID: coordinate.ID, Revision: coordinate.Revision, Digest: coordinate.Digest}
}

func turnApplicabilityRefV3(coordinate contract.CommittedPendingActionTurnApplicabilityCoordinateV1) runtimeports.OperationScopeEvidenceApplicabilityFactRefV3 {
	return runtimeports.OperationScopeEvidenceApplicabilityFactRefV3{Kind: coordinate.Kind, ID: coordinate.ID, Revision: coordinate.Revision, Digest: coordinate.Digest}
}

func pendingActionApplicabilityKeyFromRefV1(ref runtimeports.OperationScopeEvidenceApplicabilityFactRefV3) pendingActionApplicabilityKeyV1 {
	return pendingActionApplicabilityKeyV1{Kind: ref.Kind, ID: ref.ID, Revision: ref.Revision, Digest: ref.Digest}
}
