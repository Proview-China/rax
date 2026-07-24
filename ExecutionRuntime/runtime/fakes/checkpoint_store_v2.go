package fakes

import (
	"context"
	"sync"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// CheckpointStoreV2 is a deterministic in-memory reference Fact Owner. It is
// not a production backend and makes no durability, availability or SLA claim.
type CheckpointStoreV2 struct {
	mu               sync.Mutex
	bundles          map[string]ports.CheckpointAttemptBarrierBundleV2
	attemptHistory   map[string]map[core.Revision]ports.CheckpointAttemptFactV2
	barrierHistory   map[string]map[core.Revision]ports.CheckpointBarrierLeaseFactV2
	cuts             map[string]ports.EffectCutFactV2
	finalizationCuts map[string]ports.CheckpointFinalizationCutFactV2
	closures         map[string]ports.CheckpointFinalizationInputClosureFactV2
	consistencies    map[string]ports.CheckpointConsistencyFactV2
	loseNextReply    bool
	failNextStage    bool
}

func NewCheckpointStoreV2() *CheckpointStoreV2 {
	return &CheckpointStoreV2{bundles: map[string]ports.CheckpointAttemptBarrierBundleV2{}, attemptHistory: map[string]map[core.Revision]ports.CheckpointAttemptFactV2{}, barrierHistory: map[string]map[core.Revision]ports.CheckpointBarrierLeaseFactV2{}, cuts: map[string]ports.EffectCutFactV2{}, finalizationCuts: map[string]ports.CheckpointFinalizationCutFactV2{}, closures: map[string]ports.CheckpointFinalizationInputClosureFactV2{}, consistencies: map[string]ports.CheckpointConsistencyFactV2{}}
}

func (s *CheckpointStoreV2) LoseNextCheckpointReplyV2() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loseNextReply = true
}
func (s *CheckpointStoreV2) FailNextCheckpointAtomicStageV2() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failNextStage = true
}

func checkpointStoreKeyV2(tenant core.TenantID, id string) string {
	return string(tenant) + "\x00" + id
}

func (s *CheckpointStoreV2) CreateCheckpointAttemptBundleV2(ctx context.Context, bundle ports.CheckpointAttemptBarrierBundleV2) (ports.CheckpointAttemptBarrierBundleV2, error) {
	if err := contextError(ctx); err != nil {
		return ports.CheckpointAttemptBarrierBundleV2{}, err
	}
	if err := bundle.Validate(); err != nil {
		return ports.CheckpointAttemptBarrierBundleV2{}, err
	}
	key := checkpointStoreKeyV2(bundle.Attempt.TenantID, bundle.Attempt.ID)
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.bundles[key]; ok {
		if checkpointBundleEqualV2(existing, bundle) {
			return cloneOSE(existing), nil
		}
		if historical, ok := s.checkpointHistoricalBundleV2(key, bundle.Attempt.RefV2(), bundle.Barrier.RefV2()); ok && checkpointBundleEqualV2(historical, bundle) {
			return cloneOSE(historical), nil
		}
		return ports.CheckpointAttemptBarrierBundleV2{}, checkpointConflictV2("checkpoint Attempt ID binds different bundle")
	}
	if s.failNextStage {
		s.failNextStage = false
		return ports.CheckpointAttemptBarrierBundleV2{}, checkpointUnavailableV2("injected Attempt+Barrier staged failure")
	}
	s.bundles[key] = cloneOSE(bundle)
	s.appendCheckpointHistoryV2(key, bundle)
	if s.loseNextReply {
		s.loseNextReply = false
		return ports.CheckpointAttemptBarrierBundleV2{}, checkpointUnavailableV2("injected Attempt+Barrier reply loss")
	}
	return cloneOSE(bundle), nil
}

func (s *CheckpointStoreV2) InspectCheckpointAttemptHistoricalV2(ctx context.Context, ref ports.CheckpointAttemptRefV2) (ports.CheckpointAttemptFactV2, error) {
	if err := contextError(ctx); err != nil {
		return ports.CheckpointAttemptFactV2{}, err
	}
	if err := ref.Validate(); err != nil {
		return ports.CheckpointAttemptFactV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	fact, ok := s.attemptHistory[checkpointStoreKeyV2(ref.TenantID, ref.ID)][ref.Revision]
	if !ok {
		return ports.CheckpointAttemptFactV2{}, checkpointNotFoundV2("checkpoint Attempt historical fact not found")
	}
	if fact.RefV2() != ref {
		return ports.CheckpointAttemptFactV2{}, checkpointConflictV2("checkpoint Attempt historical ref drifted")
	}
	return cloneOSE(fact), nil
}

func (s *CheckpointStoreV2) InspectCheckpointAttemptBundleV2(ctx context.Context, request ports.InspectCheckpointAttemptRequestV2) (ports.CheckpointAttemptBarrierBundleV2, error) {
	if err := contextError(ctx); err != nil {
		return ports.CheckpointAttemptBarrierBundleV2{}, err
	}
	if err := request.Validate(); err != nil {
		return ports.CheckpointAttemptBarrierBundleV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	bundle, ok := s.bundles[checkpointStoreKeyV2(request.TenantID, request.AttemptID)]
	if !ok {
		return ports.CheckpointAttemptBarrierBundleV2{}, checkpointNotFoundV2("checkpoint Attempt bundle not found")
	}
	return cloneOSE(bundle), nil
}

func (s *CheckpointStoreV2) InspectCheckpointBarrierHistoricalV2(ctx context.Context, ref ports.CheckpointBarrierLeaseRefV2) (ports.CheckpointBarrierLeaseFactV2, error) {
	if err := contextError(ctx); err != nil {
		return ports.CheckpointBarrierLeaseFactV2{}, err
	}
	if err := ref.Validate(); err != nil {
		return ports.CheckpointBarrierLeaseFactV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	bundle, ok := s.barrierHistory[checkpointStoreKeyV2(ref.TenantID, ref.AttemptID)]
	if !ok {
		return ports.CheckpointBarrierLeaseFactV2{}, checkpointNotFoundV2("checkpoint Barrier not found")
	}
	fact, ok := bundle[ref.Revision]
	if !ok {
		return ports.CheckpointBarrierLeaseFactV2{}, checkpointNotFoundV2("checkpoint Barrier historical revision not found")
	}
	if fact.RefV2() != ref {
		return ports.CheckpointBarrierLeaseFactV2{}, checkpointConflictV2("checkpoint Barrier historical ref drifted")
	}
	return cloneOSE(fact), nil
}

func (s *CheckpointStoreV2) InspectCheckpointAttemptLineageV2(ctx context.Context, request ports.InspectCheckpointAttemptLineageRequestV2) (ports.CheckpointAttemptLineageV2, error) {
	if err := contextError(ctx); err != nil {
		return ports.CheckpointAttemptLineageV2{}, err
	}
	if err := request.Validate(); err != nil {
		return ports.CheckpointAttemptLineageV2{}, err
	}
	key := checkpointStoreKeyV2(request.TenantID, request.AttemptID)
	s.mu.Lock()
	defer s.mu.Unlock()
	attempts := make([]ports.CheckpointAttemptFactV2, 0, int(request.ToRevision-request.FromRevision)+1)
	for revision := request.FromRevision; revision <= request.ToRevision; revision++ {
		fact, ok := s.attemptHistory[key][revision]
		if !ok {
			return ports.CheckpointAttemptLineageV2{}, checkpointNotFoundV2("checkpoint Attempt lineage has a revision gap")
		}
		attempts = append(attempts, cloneOSE(fact))
	}
	barriers := make([]ports.CheckpointBarrierLeaseFactV2, 0, len(s.barrierHistory[key]))
	for revision := core.Revision(1); ; revision++ {
		fact, ok := s.barrierHistory[key][revision]
		if !ok {
			break
		}
		barriers = append(barriers, cloneOSE(fact))
	}
	return ports.CheckpointAttemptLineageV2{Attempts: attempts, Barriers: barriers}, nil
}

func (s *CheckpointStoreV2) CommitCheckpointEffectCutV2(ctx context.Context, request ports.CheckpointEffectCutCommitRequestV2) (ports.CheckpointEffectCutBundleV2, error) {
	if err := contextError(ctx); err != nil {
		return ports.CheckpointEffectCutBundleV2{}, err
	}
	if err := request.NextAttempt.Validate(); err != nil {
		return ports.CheckpointEffectCutBundleV2{}, err
	}
	if err := request.Cut.Validate(); err != nil {
		return ports.CheckpointEffectCutBundleV2{}, err
	}
	key := checkpointStoreKeyV2(request.NextAttempt.TenantID, request.NextAttempt.ID)
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.bundles[key]
	if !ok {
		return ports.CheckpointEffectCutBundleV2{}, checkpointNotFoundV2("checkpoint Attempt not found for Effect Cut")
	}
	if existing, ok := s.cuts[request.Cut.Ref.ID]; ok {
		if existing.Ref == request.Cut.Ref && request.NextAttempt.EffectCut != nil && *request.NextAttempt.EffectCut == existing.Ref {
			historical, found := s.attemptHistory[key][request.NextAttempt.Revision]
			if found && historical.RefV2() == request.NextAttempt.RefV2() {
				return ports.CheckpointEffectCutBundleV2{Attempt: cloneOSE(historical), Cut: cloneOSE(existing)}, nil
			}
		}
		return ports.CheckpointEffectCutBundleV2{}, checkpointConflictV2("checkpoint Effect Cut ID binds different content")
	}
	if current.Attempt.Revision != request.ExpectedAttemptRevision || current.Barrier.Revision != request.ExpectedBarrierRevision {
		return ports.CheckpointEffectCutBundleV2{}, checkpointConflictV2("checkpoint Effect Cut CAS revision drifted")
	}
	if err := control.ValidateCheckpointAttemptTransitionV2(current.Attempt, request.NextAttempt); err != nil {
		return ports.CheckpointEffectCutBundleV2{}, err
	}
	nextBundle := current
	nextBundle.Attempt = request.NextAttempt
	result := ports.CheckpointEffectCutBundleV2{Attempt: request.NextAttempt, Cut: request.Cut}
	if err := result.Validate(); err != nil {
		return ports.CheckpointEffectCutBundleV2{}, err
	}
	if s.failNextStage {
		s.failNextStage = false
		return ports.CheckpointEffectCutBundleV2{}, checkpointUnavailableV2("injected Effect Cut staged failure")
	}
	s.bundles[key] = cloneOSE(nextBundle)
	s.appendCheckpointHistoryV2(key, nextBundle)
	s.cuts[request.Cut.Ref.ID] = cloneOSE(request.Cut)
	if s.loseNextReply {
		s.loseNextReply = false
		return ports.CheckpointEffectCutBundleV2{}, checkpointUnavailableV2("injected Effect Cut reply loss")
	}
	return cloneOSE(result), nil
}

func (s *CheckpointStoreV2) InspectCheckpointEffectCutV2(ctx context.Context, ref ports.EffectCutRefV2) (ports.EffectCutFactV2, error) {
	if err := contextError(ctx); err != nil {
		return ports.EffectCutFactV2{}, err
	}
	if err := ref.Validate(); err != nil {
		return ports.EffectCutFactV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	fact, ok := s.cuts[ref.ID]
	if !ok {
		return ports.EffectCutFactV2{}, checkpointNotFoundV2("checkpoint Effect Cut not found")
	}
	if fact.Ref != ref {
		return ports.EffectCutFactV2{}, checkpointConflictV2("checkpoint Effect Cut ref drifted")
	}
	return cloneOSE(fact), nil
}

func (s *CheckpointStoreV2) CommitCheckpointFinalizationCutV2(ctx context.Context, request ports.CheckpointFinalizationCutCommitRequestV2) (ports.CheckpointFinalizationCutFactV2, error) {
	if err := contextError(ctx); err != nil {
		return ports.CheckpointFinalizationCutFactV2{}, err
	}
	if err := request.NextAttempt.Validate(); err != nil {
		return ports.CheckpointFinalizationCutFactV2{}, err
	}
	if err := request.Cut.Validate(); err != nil {
		return ports.CheckpointFinalizationCutFactV2{}, err
	}
	key := checkpointStoreKeyV2(request.NextAttempt.TenantID, request.NextAttempt.ID)
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.bundles[key]
	if !ok {
		return ports.CheckpointFinalizationCutFactV2{}, checkpointNotFoundV2("checkpoint Attempt not found for Finalization Cut")
	}
	if existing, ok := s.finalizationCuts[request.Cut.Ref.ID]; ok {
		if existing.Ref == request.Cut.Ref {
			historical, found := s.attemptHistory[key][request.NextAttempt.Revision]
			if found && historical.RefV2() == request.NextAttempt.RefV2() {
				return cloneOSE(existing), nil
			}
		}
		return ports.CheckpointFinalizationCutFactV2{}, checkpointConflictV2("checkpoint Finalization Cut ID binds different content")
	}
	if current.Attempt.Revision != request.ExpectedAttemptRevision {
		return ports.CheckpointFinalizationCutFactV2{}, checkpointConflictV2("checkpoint Finalization Cut CAS revision drifted")
	}
	if err := control.ValidateCheckpointAttemptTransitionV2(current.Attempt, request.NextAttempt); err != nil {
		return ports.CheckpointFinalizationCutFactV2{}, err
	}
	if s.failNextStage {
		s.failNextStage = false
		return ports.CheckpointFinalizationCutFactV2{}, checkpointUnavailableV2("injected Finalization Cut staged failure")
	}
	current.Attempt = request.NextAttempt
	s.bundles[key] = cloneOSE(current)
	s.appendCheckpointHistoryV2(key, current)
	s.finalizationCuts[request.Cut.Ref.ID] = cloneOSE(request.Cut)
	if s.loseNextReply {
		s.loseNextReply = false
		return ports.CheckpointFinalizationCutFactV2{}, checkpointUnavailableV2("injected Finalization Cut reply loss")
	}
	return cloneOSE(request.Cut), nil
}

func (s *CheckpointStoreV2) InspectCheckpointFinalizationCutV2(ctx context.Context, ref ports.CheckpointFinalizationCutRefV2) (ports.CheckpointFinalizationCutFactV2, error) {
	if err := contextError(ctx); err != nil {
		return ports.CheckpointFinalizationCutFactV2{}, err
	}
	if err := ref.Validate(); err != nil {
		return ports.CheckpointFinalizationCutFactV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	fact, ok := s.finalizationCuts[ref.ID]
	if !ok {
		return ports.CheckpointFinalizationCutFactV2{}, checkpointNotFoundV2("checkpoint Finalization Cut not found")
	}
	if fact.Ref != ref {
		return ports.CheckpointFinalizationCutFactV2{}, checkpointConflictV2("checkpoint Finalization Cut ref drifted")
	}
	return cloneOSE(fact), nil
}

func (s *CheckpointStoreV2) CommitCheckpointFinalizationInputsV2(ctx context.Context, request ports.CheckpointFinalizationInputsCommitRequestV2) (ports.CheckpointFinalizationInputClosureFactV2, error) {
	if err := contextError(ctx); err != nil {
		return ports.CheckpointFinalizationInputClosureFactV2{}, err
	}
	if request.NextAttempt.Validate() != nil || request.Closure.Validate() != nil {
		return ports.CheckpointFinalizationInputClosureFactV2{}, checkpointInvalidStoreV2("checkpoint Closure commit is invalid")
	}
	key := checkpointStoreKeyV2(request.NextAttempt.TenantID, request.NextAttempt.ID)
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.bundles[key]
	if !ok {
		return ports.CheckpointFinalizationInputClosureFactV2{}, checkpointNotFoundV2("checkpoint Attempt not found for Closure")
	}
	if existing, ok := s.closures[request.Closure.Ref.ID]; ok {
		if ports.SameCheckpointFinalizationInputClosureRefV2(existing.Ref, request.Closure.Ref) {
			historical, found := s.attemptHistory[key][request.NextAttempt.Revision]
			if found && historical.RefV2() == request.NextAttempt.RefV2() {
				return cloneOSE(existing), nil
			}
		}
		return ports.CheckpointFinalizationInputClosureFactV2{}, checkpointConflictV2("checkpoint Closure ID binds different content")
	}
	if current.Attempt.Revision != request.ExpectedAttemptRevision {
		return ports.CheckpointFinalizationInputClosureFactV2{}, checkpointConflictV2("checkpoint Closure CAS revision drifted")
	}
	if err := control.ValidateCheckpointAttemptTransitionV2(current.Attempt, request.NextAttempt); err != nil {
		return ports.CheckpointFinalizationInputClosureFactV2{}, err
	}
	if s.failNextStage {
		s.failNextStage = false
		return ports.CheckpointFinalizationInputClosureFactV2{}, checkpointUnavailableV2("injected Closure staged failure")
	}
	current.Attempt = request.NextAttempt
	s.bundles[key] = cloneOSE(current)
	s.appendCheckpointHistoryV2(key, current)
	s.closures[request.Closure.Ref.ID] = cloneOSE(request.Closure)
	if s.loseNextReply {
		s.loseNextReply = false
		return ports.CheckpointFinalizationInputClosureFactV2{}, checkpointUnavailableV2("injected Closure reply loss")
	}
	return cloneOSE(request.Closure), nil
}

func (s *CheckpointStoreV2) InspectCheckpointFinalizationInputsV2(ctx context.Context, ref ports.CheckpointFinalizationInputClosureRefV2) (ports.CheckpointFinalizationInputClosureFactV2, error) {
	if err := contextError(ctx); err != nil {
		return ports.CheckpointFinalizationInputClosureFactV2{}, err
	}
	if err := ref.Validate(); err != nil {
		return ports.CheckpointFinalizationInputClosureFactV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	fact, ok := s.closures[ref.ID]
	if !ok {
		return ports.CheckpointFinalizationInputClosureFactV2{}, checkpointNotFoundV2("checkpoint Closure not found")
	}
	if !ports.SameCheckpointFinalizationInputClosureRefV2(fact.Ref, ref) {
		return ports.CheckpointFinalizationInputClosureFactV2{}, checkpointConflictV2("checkpoint Closure ref drifted")
	}
	return cloneOSE(fact), nil
}

func (s *CheckpointStoreV2) CommitCheckpointConsistencyV2(ctx context.Context, request ports.CheckpointConsistencyOwnerCommitRequestV2) (ports.CheckpointConsistencyCommitBundleV2, error) {
	if err := contextError(ctx); err != nil {
		return ports.CheckpointConsistencyCommitBundleV2{}, err
	}
	if err := request.Bundle.Validate(); err != nil {
		return ports.CheckpointConsistencyCommitBundleV2{}, err
	}
	key := checkpointStoreKeyV2(request.Bundle.Attempt.TenantID, request.Bundle.Attempt.ID)
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.bundles[key]
	if !ok {
		return ports.CheckpointConsistencyCommitBundleV2{}, checkpointNotFoundV2("checkpoint Attempt not found for Consistency")
	}
	if existing, ok := s.consistencies[request.Bundle.Consistency.Ref.ID]; ok {
		if existing.Ref == request.Bundle.Consistency.Ref {
			historical, found := s.checkpointHistoricalBundleV2(key, request.Bundle.Attempt.RefV2(), request.Bundle.Barrier.RefV2())
			if found {
				result := ports.CheckpointConsistencyCommitBundleV2{Attempt: historical.Attempt, Barrier: historical.Barrier, Consistency: existing}
				if result.Validate() == nil {
					return cloneOSE(result), nil
				}
			}
		}
		return ports.CheckpointConsistencyCommitBundleV2{}, checkpointConflictV2("checkpoint Consistency ID binds different content")
	}
	if current.Attempt.Revision != request.ExpectedAttemptRevision || current.Barrier.Revision != request.ExpectedBarrierRevision {
		return ports.CheckpointConsistencyCommitBundleV2{}, checkpointConflictV2("checkpoint Consistency CAS revision drifted")
	}
	if err := control.ValidateCheckpointAttemptTransitionV2(current.Attempt, request.Bundle.Attempt); err != nil {
		return ports.CheckpointConsistencyCommitBundleV2{}, err
	}
	if err := control.ValidateCheckpointBarrierTransitionV2(current.Barrier, request.Bundle.Barrier); err != nil {
		return ports.CheckpointConsistencyCommitBundleV2{}, err
	}
	if s.failNextStage {
		s.failNextStage = false
		return ports.CheckpointConsistencyCommitBundleV2{}, checkpointUnavailableV2("injected Consistency staged failure")
	}
	nextBundle := ports.CheckpointAttemptBarrierBundleV2{Attempt: cloneOSE(request.Bundle.Attempt), Barrier: cloneOSE(request.Bundle.Barrier)}
	s.bundles[key] = nextBundle
	s.appendCheckpointHistoryV2(key, nextBundle)
	s.consistencies[request.Bundle.Consistency.Ref.ID] = cloneOSE(request.Bundle.Consistency)
	if s.loseNextReply {
		s.loseNextReply = false
		return ports.CheckpointConsistencyCommitBundleV2{}, checkpointUnavailableV2("injected Consistency reply loss")
	}
	return cloneOSE(request.Bundle), nil
}

func (s *CheckpointStoreV2) CommitCheckpointFinalizationV2(ctx context.Context, request ports.CheckpointFinalizationOwnerCommitRequestV2) (ports.CheckpointAttemptFinalizationBundleV2, error) {
	if err := contextError(ctx); err != nil {
		return ports.CheckpointAttemptFinalizationBundleV2{}, err
	}
	if err := request.Bundle.Validate(); err != nil {
		return ports.CheckpointAttemptFinalizationBundleV2{}, err
	}
	key := checkpointStoreKeyV2(request.Bundle.Attempt.TenantID, request.Bundle.Attempt.ID)
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.bundles[key]
	if !ok {
		return ports.CheckpointAttemptFinalizationBundleV2{}, checkpointNotFoundV2("checkpoint Attempt not found for Finalize")
	}
	if current.Attempt.RefV2() == request.Bundle.Attempt.RefV2() && current.Barrier.RefV2() == request.Bundle.Barrier.RefV2() {
		return cloneOSE(request.Bundle), nil
	}
	if historical, found := s.checkpointHistoricalBundleV2(key, request.Bundle.Attempt.RefV2(), request.Bundle.Barrier.RefV2()); found {
		result := ports.CheckpointAttemptFinalizationBundleV2{Attempt: historical.Attempt, Barrier: historical.Barrier, Inputs: request.Bundle.Inputs}
		if result.Validate() == nil {
			return cloneOSE(result), nil
		}
	}
	if current.Attempt.Revision != request.ExpectedAttemptRevision || current.Barrier.Revision != request.ExpectedBarrierRevision {
		return ports.CheckpointAttemptFinalizationBundleV2{}, checkpointConflictV2("checkpoint Finalize CAS revision drifted")
	}
	if err := control.ValidateCheckpointAttemptTransitionV2(current.Attempt, request.Bundle.Attempt); err != nil {
		return ports.CheckpointAttemptFinalizationBundleV2{}, err
	}
	if err := control.ValidateCheckpointBarrierTransitionV2(current.Barrier, request.Bundle.Barrier); err != nil {
		return ports.CheckpointAttemptFinalizationBundleV2{}, err
	}
	if s.failNextStage {
		s.failNextStage = false
		return ports.CheckpointAttemptFinalizationBundleV2{}, checkpointUnavailableV2("injected Finalize staged failure")
	}
	nextBundle := ports.CheckpointAttemptBarrierBundleV2{Attempt: cloneOSE(request.Bundle.Attempt), Barrier: cloneOSE(request.Bundle.Barrier)}
	s.bundles[key] = nextBundle
	s.appendCheckpointHistoryV2(key, nextBundle)
	if s.loseNextReply {
		s.loseNextReply = false
		return ports.CheckpointAttemptFinalizationBundleV2{}, checkpointUnavailableV2("injected Finalize reply loss")
	}
	return cloneOSE(request.Bundle), nil
}

func (s *CheckpointStoreV2) InspectCheckpointConsistencyV2(ctx context.Context, ref ports.CheckpointConsistencyRefV2) (ports.CheckpointConsistencyFactV2, error) {
	if err := contextError(ctx); err != nil {
		return ports.CheckpointConsistencyFactV2{}, err
	}
	if err := ref.Validate(); err != nil {
		return ports.CheckpointConsistencyFactV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	fact, ok := s.consistencies[ref.ID]
	if !ok {
		return ports.CheckpointConsistencyFactV2{}, checkpointNotFoundV2("checkpoint Consistency not found")
	}
	if fact.Ref != ref {
		return ports.CheckpointConsistencyFactV2{}, checkpointConflictV2("checkpoint Consistency ref drifted")
	}
	return cloneOSE(fact), nil
}

func checkpointBundleEqualV2(left, right ports.CheckpointAttemptBarrierBundleV2) bool {
	a, _ := control.CheckpointCanonicalDigestV2("CheckpointAttemptBarrierBundleV2", left)
	b, _ := control.CheckpointCanonicalDigestV2("CheckpointAttemptBarrierBundleV2", right)
	return a == b
}

func (s *CheckpointStoreV2) appendCheckpointHistoryV2(key string, bundle ports.CheckpointAttemptBarrierBundleV2) {
	if s.attemptHistory[key] == nil {
		s.attemptHistory[key] = map[core.Revision]ports.CheckpointAttemptFactV2{}
	}
	if s.barrierHistory[key] == nil {
		s.barrierHistory[key] = map[core.Revision]ports.CheckpointBarrierLeaseFactV2{}
	}
	s.attemptHistory[key][bundle.Attempt.Revision] = cloneOSE(bundle.Attempt)
	s.barrierHistory[key][bundle.Barrier.Revision] = cloneOSE(bundle.Barrier)
}

func (s *CheckpointStoreV2) checkpointHistoricalBundleV2(key string, attempt ports.CheckpointAttemptRefV2, barrier ports.CheckpointBarrierLeaseRefV2) (ports.CheckpointAttemptBarrierBundleV2, bool) {
	attemptFact, attemptOK := s.attemptHistory[key][attempt.Revision]
	barrierFact, barrierOK := s.barrierHistory[key][barrier.Revision]
	if !attemptOK || !barrierOK || attemptFact.RefV2() != attempt || barrierFact.RefV2() != barrier {
		return ports.CheckpointAttemptBarrierBundleV2{}, false
	}
	return ports.CheckpointAttemptBarrierBundleV2{Attempt: cloneOSE(attemptFact), Barrier: cloneOSE(barrierFact)}, true
}
func checkpointConflictV2(message string) error {
	return core.NewError(core.ErrorConflict, core.ReasonCheckpointInconsistent, message)
}
func checkpointNotFoundV2(message string) error {
	return core.NewError(core.ErrorNotFound, core.ReasonCheckpointInconsistent, message)
}
func checkpointUnavailableV2(message string) error {
	return core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, message)
}
func checkpointInvalidStoreV2(message string) error {
	return core.NewError(core.ErrorInvalidArgument, core.ReasonCheckpointInconsistent, message)
}
