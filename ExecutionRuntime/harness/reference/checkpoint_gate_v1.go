package reference

import (
	"context"
	"reflect"
	"sync"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/contract"
	harnessports "github.com/Proview-China/rax/ExecutionRuntime/harness/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type checkpointScopeKeyV1 struct {
	tenant string
	scope  string
	run    string
}

type checkpointObjectKeyV1 struct {
	scope    checkpointScopeKeyV1
	id       string
	revision core.Revision
}

// CheckpointGateStoreV1 is an in-memory reference backend, not a production
// durability or SLA claim.
type CheckpointGateStoreV1 struct {
	mu        sync.RWMutex
	gates     map[checkpointObjectKeyV1]contract.CheckpointGateFactV1
	snapshots map[checkpointObjectKeyV1]contract.HarnessCheckpointSnapshotFactV1
	current   map[checkpointScopeKeyV1]contract.CheckpointGateRefV1
}

func NewCheckpointGateStoreV1() *CheckpointGateStoreV1 {
	return &CheckpointGateStoreV1{gates: make(map[checkpointObjectKeyV1]contract.CheckpointGateFactV1), snapshots: make(map[checkpointObjectKeyV1]contract.HarnessCheckpointSnapshotFactV1), current: make(map[checkpointScopeKeyV1]contract.CheckpointGateRefV1)}
}

func (s *CheckpointGateStoreV1) CreateCheckpointGateAndSnapshotV1(_ context.Context, gate contract.CheckpointGateFactV1, snapshot contract.HarnessCheckpointSnapshotFactV1) (contract.CheckpointGateFactV1, contract.HarnessCheckpointSnapshotFactV1, error) {
	if s == nil || gate.Validate() != nil || snapshot.Validate() != nil || gate.State != contract.CheckpointGateAcquiredV1 || gate.Ref.Revision != 1 || gate.Snapshot != snapshot.Ref || gate.Request.IntentDigest != snapshot.IntentDigest || gate.Request.Run.RunID != snapshot.Run.RunID || !runtimeports.SameExecutionScopeV2(gate.Request.Run.Scope, snapshot.Run.Scope) {
		return contract.CheckpointGateFactV1{}, contract.HarnessCheckpointSnapshotFactV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "reference checkpoint Gate+Snapshot create is invalid")
	}
	scope, err := scopeKeyV1(gate.Request.Run)
	if err != nil {
		return contract.CheckpointGateFactV1{}, contract.HarnessCheckpointSnapshotFactV1{}, err
	}
	gk := checkpointObjectKeyV1{scope: scope, id: gate.Ref.ID, revision: gate.Ref.Revision}
	sk := checkpointObjectKeyV1{scope: scope, id: snapshot.Ref.ID, revision: snapshot.Ref.Revision}
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.gates[gk]; ok {
		existingSnapshot := s.snapshots[sk]
		if reflect.DeepEqual(existing, gate) && reflect.DeepEqual(existingSnapshot, snapshot) {
			return existing.Clone(), existingSnapshot.Clone(), nil
		}
		return contract.CheckpointGateFactV1{}, contract.HarnessCheckpointSnapshotFactV1{}, core.NewError(core.ErrorConflict, core.ReasonAlreadyExists, "reference checkpoint gate content differs")
	}
	if current, ok := s.current[scope]; ok {
		fact := s.gates[checkpointObjectKeyV1{scope: scope, id: current.ID, revision: current.Revision}]
		if fact.State == contract.CheckpointGateAcquiredV1 || fact.State == contract.CheckpointGateBoundV1 {
			return contract.CheckpointGateFactV1{}, contract.HarnessCheckpointSnapshotFactV1{}, core.NewError(core.ErrorConflict, core.ReasonAlreadyExists, "Harness Run already has an acquired checkpoint gate")
		}
	}
	if existing, ok := s.snapshots[sk]; ok && !reflect.DeepEqual(existing, snapshot) {
		return contract.CheckpointGateFactV1{}, contract.HarnessCheckpointSnapshotFactV1{}, core.NewError(core.ErrorConflict, core.ReasonAlreadyExists, "reference checkpoint snapshot content differs")
	}
	s.gates[gk] = gate.Clone()
	s.snapshots[sk] = snapshot.Clone()
	s.current[scope] = gate.Ref
	return gate.Clone(), snapshot.Clone(), nil
}

func (s *CheckpointGateStoreV1) InspectCheckpointGateV1(_ context.Context, ref contract.CheckpointGateRefV1) (contract.CheckpointGateFactV1, error) {
	if s == nil || ref.Validate() != nil {
		return contract.CheckpointGateFactV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "checkpoint gate exact ref is invalid")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for key, fact := range s.gates {
		if key.id == ref.ID && key.revision == ref.Revision && fact.Ref == ref {
			return fact.Clone(), nil
		}
	}
	return contract.CheckpointGateFactV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "checkpoint gate not found")
}

func (s *CheckpointGateStoreV1) InspectCheckpointGateCurrentV1(_ context.Context, run contract.RunRef) (contract.CheckpointGateFactV1, error) {
	scope, err := scopeKeyV1(run)
	if s == nil || err != nil {
		if err != nil {
			return contract.CheckpointGateFactV1{}, err
		}
		return contract.CheckpointGateFactV1{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "checkpoint gate store unavailable")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	ref, ok := s.current[scope]
	if !ok {
		return contract.CheckpointGateFactV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "current checkpoint gate not found")
	}
	fact, ok := s.gates[checkpointObjectKeyV1{scope: scope, id: ref.ID, revision: ref.Revision}]
	if !ok || fact.Ref != ref {
		return contract.CheckpointGateFactV1{}, core.NewError(core.ErrorConflict, core.ReasonCheckpointInconsistent, "current checkpoint gate index drifted")
	}
	return fact.Clone(), nil
}

func (s *CheckpointGateStoreV1) InspectHarnessCheckpointSnapshotV1(_ context.Context, ref contract.HarnessCheckpointSnapshotRefV1) (contract.HarnessCheckpointSnapshotFactV1, error) {
	if s == nil || ref.Validate() != nil {
		return contract.HarnessCheckpointSnapshotFactV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Harness checkpoint snapshot exact ref is invalid")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for key, fact := range s.snapshots {
		if key.id == ref.ID && key.revision == ref.Revision && fact.Ref == ref {
			return fact.Clone(), nil
		}
	}
	return contract.HarnessCheckpointSnapshotFactV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "Harness checkpoint snapshot not found")
}

func (s *CheckpointGateStoreV1) BindCheckpointGateRuntimeV1(_ context.Context, expected contract.CheckpointGateRefV1, next contract.CheckpointGateFactV1) (contract.CheckpointGateFactV1, error) {
	if next.State != contract.CheckpointGateBoundV1 || next.Ref.Revision != 2 || next.Runtime == nil {
		return contract.CheckpointGateFactV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "reference checkpoint gate Runtime bind is invalid")
	}
	return s.transitionCheckpointGateV1(expected, next, contract.CheckpointGateAcquiredV1)
}

func (s *CheckpointGateStoreV1) InvalidateCheckpointGateV1(_ context.Context, expected contract.CheckpointGateRefV1, next contract.CheckpointGateFactV1) (contract.CheckpointGateFactV1, error) {
	if next.State != contract.CheckpointGateInvalidatedV1 || next.Ref.Revision != 2 || next.Runtime != nil {
		return contract.CheckpointGateFactV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "reference checkpoint gate invalidation is invalid")
	}
	return s.transitionCheckpointGateV1(expected, next, contract.CheckpointGateAcquiredV1)
}

func (s *CheckpointGateStoreV1) ReleaseCheckpointGateV1(_ context.Context, expected contract.CheckpointGateRefV1, next contract.CheckpointGateFactV1) (contract.CheckpointGateFactV1, error) {
	if next.State != contract.CheckpointGateReleasedV1 || next.Ref.Revision != 3 || next.Runtime == nil {
		return contract.CheckpointGateFactV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "reference checkpoint gate release is invalid")
	}
	return s.transitionCheckpointGateV1(expected, next, contract.CheckpointGateBoundV1)
}

func (s *CheckpointGateStoreV1) transitionCheckpointGateV1(expected contract.CheckpointGateRefV1, next contract.CheckpointGateFactV1, expectedState contract.CheckpointGateStateV1) (contract.CheckpointGateFactV1, error) {
	if s == nil || expected.Validate() != nil || next.Validate() != nil || next.Ref.ID != expected.ID || next.Ref.Revision != expected.Revision+1 {
		return contract.CheckpointGateFactV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "reference checkpoint gate transition is invalid")
	}
	scope, err := scopeKeyV1(next.Request.Run)
	if err != nil {
		return contract.CheckpointGateFactV1{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	currentRef, ok := s.current[scope]
	if !ok {
		return contract.CheckpointGateFactV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "current checkpoint gate not found")
	}
	if currentRef == next.Ref {
		stored := s.gates[checkpointObjectKeyV1{scope: scope, id: next.Ref.ID, revision: next.Ref.Revision}]
		if reflect.DeepEqual(stored, next) {
			return stored.Clone(), nil
		}
		return contract.CheckpointGateFactV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "checkpoint gate release replay differs")
	}
	if currentRef != expected {
		return contract.CheckpointGateFactV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "checkpoint gate current revision drifted")
	}
	current := s.gates[checkpointObjectKeyV1{scope: scope, id: expected.ID, revision: expected.Revision}]
	if current.Ref != expected || current.State != expectedState || !sameGateImmutableV1(current, next) {
		return contract.CheckpointGateFactV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "checkpoint gate transition changed immutable content")
	}
	nextKey := checkpointObjectKeyV1{scope: scope, id: next.Ref.ID, revision: next.Ref.Revision}
	s.gates[nextKey] = next.Clone()
	s.current[scope] = next.Ref
	return next.Clone(), nil
}

func scopeKeyV1(run contract.RunRef) (checkpointScopeKeyV1, error) {
	if err := run.Validate(); err != nil {
		return checkpointScopeKeyV1{}, err
	}
	digest, err := runtimeports.ExecutionScopeDigestV2(run.Scope)
	if err != nil {
		return checkpointScopeKeyV1{}, err
	}
	return checkpointScopeKeyV1{tenant: string(run.Scope.Identity.TenantID), scope: string(digest), run: string(run.RunID)}, nil
}

func sameGateImmutableV1(a, b contract.CheckpointGateFactV1) bool {
	a.Ref, b.Ref = contract.CheckpointGateRefV1{}, contract.CheckpointGateRefV1{}
	a.State, b.State = "", ""
	a.Runtime, b.Runtime = nil, nil
	a.BoundUnixNano, b.BoundUnixNano = 0, 0
	a.ReleasedUnixNano, b.ReleasedUnixNano = 0, 0
	if a.ExpiresUnixNano > b.ExpiresUnixNano {
		a.ExpiresUnixNano = b.ExpiresUnixNano
	}
	return reflect.DeepEqual(a, b)
}

var _ harnessports.CheckpointGateStoreV1 = (*CheckpointGateStoreV1)(nil)
