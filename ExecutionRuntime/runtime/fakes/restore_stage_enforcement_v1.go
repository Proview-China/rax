package fakes

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type restoreStageEnforcementKeyV1 struct {
	TenantID        core.TenantID
	OperationDigest core.Digest
	EffectID        core.EffectIntentID
	PermitID        string
}

// RestoreStageEnforcementStoreV1 is a reference-only in-memory Effect Owner
// journal. It is suitable for tests and conformance only.
type RestoreStageEnforcementStoreV1 struct {
	mu        sync.Mutex
	values    map[restoreStageEnforcementKeyV1]ports.RestoreStageEnforcementJournalV1
	commits   int
	loseReply bool
}

func NewRestoreStageEnforcementStoreV1() *RestoreStageEnforcementStoreV1 {
	return &RestoreStageEnforcementStoreV1{values: make(map[restoreStageEnforcementKeyV1]ports.RestoreStageEnforcementJournalV1)}
}

func (s *RestoreStageEnforcementStoreV1) AppendRestoreStageEnforcementV1(_ context.Context, request control.AppendRestoreStageEnforcementRequestV1) (ports.RestoreStageEnforcementJournalV1, error) {
	if s == nil || request.Next.Validate() != nil || request.Next.Revision != request.ExpectedRevision+1 {
		return ports.RestoreStageEnforcementJournalV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidTransition, "Restore Stage enforcement append is invalid")
	}
	key := restoreStageEnforcementKey(request.Next.Operation, request.Next.EffectID, request.Next.PermitID)
	s.mu.Lock()
	defer s.mu.Unlock()
	current, exists := s.values[key]
	if exists && current.Digest == request.Next.Digest {
		return cloneRestoreStageEnforcementJournalV1(current), nil
	}
	currentRevision := core.Revision(0)
	if exists {
		currentRevision = current.Revision
	}
	if currentRevision != request.ExpectedRevision {
		return ports.RestoreStageEnforcementJournalV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Restore Stage enforcement journal CAS lost")
	}
	if exists && (current.Revision != 1 || request.Next.Revision != 2 || current.Prepare == nil || request.Next.Prepare == nil || *current.Prepare != *request.Next.Prepare || current.Execute != nil || request.Next.Execute == nil) {
		return ports.RestoreStageEnforcementJournalV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidTransition, "Restore Stage enforcement history cannot be overwritten")
	}
	stored := cloneRestoreStageEnforcementJournalV1(request.Next)
	s.values[key] = stored
	s.commits++
	if s.loseReply {
		s.loseReply = false
		return ports.RestoreStageEnforcementJournalV1{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "Restore Stage enforcement append reply was lost")
	}
	return cloneRestoreStageEnforcementJournalV1(stored), nil
}

func (s *RestoreStageEnforcementStoreV1) InspectRestoreStageEnforcementV1(_ context.Context, operation ports.OperationSubjectV3, effectID core.EffectIntentID, permitID string) (ports.RestoreStageEnforcementJournalV1, error) {
	if s == nil || operation.Validate() != nil || effectID == "" || permitID == "" {
		return ports.RestoreStageEnforcementJournalV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonDispatchPermitInvalid, "Restore Stage enforcement Inspect is invalid")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	value, exists := s.values[restoreStageEnforcementKey(operation, effectID, permitID)]
	if !exists {
		return ports.RestoreStageEnforcementJournalV1{}, core.NewError(core.ErrorNotFound, core.ReasonEvidenceUnavailable, "Restore Stage enforcement journal was not found")
	}
	return cloneRestoreStageEnforcementJournalV1(value), nil
}

func (s *RestoreStageEnforcementStoreV1) LoseNextAppendReplyV1() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loseReply = true
}

func (s *RestoreStageEnforcementStoreV1) CommitCountV1() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.commits
}

func restoreStageEnforcementKey(operation ports.OperationSubjectV3, effectID core.EffectIntentID, permitID string) restoreStageEnforcementKeyV1 {
	digest, _ := operation.DigestV3()
	return restoreStageEnforcementKeyV1{TenantID: operation.ExecutionScope.Identity.TenantID, OperationDigest: digest, EffectID: effectID, PermitID: permitID}
}

func cloneRestoreStageEnforcementJournalV1(value ports.RestoreStageEnforcementJournalV1) ports.RestoreStageEnforcementJournalV1 {
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	var clone ports.RestoreStageEnforcementJournalV1
	if err := json.Unmarshal(data, &clone); err != nil {
		panic(err)
	}
	return clone
}

var _ control.RestoreStageEnforcementFactPortV1 = (*RestoreStageEnforcementStoreV1)(nil)
