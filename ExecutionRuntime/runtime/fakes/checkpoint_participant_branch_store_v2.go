package fakes

import (
	"context"
	"sync"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// CheckpointParticipantBranchStoreV2 is a deterministic in-memory reference
// semantic Owner. It is not a production backend and makes no durability or SLA claim.
type CheckpointParticipantBranchStoreV2 struct {
	mu     sync.Mutex
	guards map[string]ports.CheckpointParticipantBranchGuardFactV2
}

func NewCheckpointParticipantBranchStoreV2() *CheckpointParticipantBranchStoreV2 {
	return &CheckpointParticipantBranchStoreV2{guards: map[string]ports.CheckpointParticipantBranchGuardFactV2{}}
}

func checkpointParticipantBranchKeyV2(tenant core.TenantID, attemptID, participantID string) string {
	return string(tenant) + "\x00" + attemptID + "\x00" + participantID
}

func (s *CheckpointParticipantBranchStoreV2) SelectCheckpointParticipantBranchV2(ctx context.Context, request ports.SelectCheckpointParticipantBranchRequestV2) (ports.CheckpointParticipantBranchGuardFactV2, error) {
	if err := contextError(ctx); err != nil {
		return ports.CheckpointParticipantBranchGuardFactV2{}, err
	}
	if err := request.Validate(); err != nil {
		return ports.CheckpointParticipantBranchGuardFactV2{}, err
	}
	fact, err := ports.SealCheckpointParticipantBranchGuardFactV2(ports.CheckpointParticipantBranchGuardFactV2{
		Ref: ports.CheckpointParticipantBranchGuardRefV2{
			TenantID: request.Attempt.TenantID, AttemptID: request.Attempt.ID,
			ParticipantID: request.Participant.ID, SelectedPhase: request.Terminal.Phase,
		},
		Attempt: request.Attempt, Participant: request.Participant, Terminal: request.Terminal,
		CreatedUnixNano: request.SelectedAt,
	})
	if err != nil {
		return ports.CheckpointParticipantBranchGuardFactV2{}, err
	}
	key := checkpointParticipantBranchKeyV2(request.Attempt.TenantID, request.Attempt.ID, request.Participant.ID)
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.guards[key]; ok {
		if existing.Ref == fact.Ref && existing.Terminal.Digest == fact.Terminal.Digest {
			return cloneOSE(existing), nil
		}
		return ports.CheckpointParticipantBranchGuardFactV2{}, core.NewError(core.ErrorConflict, core.ReasonCheckpointInconsistent, "checkpoint Participant commit/abort branch is already selected")
	}
	s.guards[key] = cloneOSE(fact)
	return cloneOSE(fact), nil
}

func (s *CheckpointParticipantBranchStoreV2) InspectCheckpointParticipantBranchV2(ctx context.Context, ref ports.CheckpointParticipantBranchGuardRefV2) (ports.CheckpointParticipantBranchGuardFactV2, error) {
	if err := contextError(ctx); err != nil {
		return ports.CheckpointParticipantBranchGuardFactV2{}, err
	}
	if err := ref.Validate(); err != nil {
		return ports.CheckpointParticipantBranchGuardFactV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	fact, ok := s.guards[checkpointParticipantBranchKeyV2(ref.TenantID, ref.AttemptID, ref.ParticipantID)]
	if !ok {
		return ports.CheckpointParticipantBranchGuardFactV2{}, core.NewError(core.ErrorNotFound, core.ReasonCheckpointInconsistent, "checkpoint Participant branch guard not found")
	}
	if fact.Ref != ref {
		return ports.CheckpointParticipantBranchGuardFactV2{}, core.NewError(core.ErrorConflict, core.ReasonCheckpointInconsistent, "checkpoint Participant branch guard ref drifted")
	}
	return cloneOSE(fact), nil
}
