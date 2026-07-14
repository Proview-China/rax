package fakes

import (
	"context"
	"reflect"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func (s *EffectStoreV2) CompareAndSwapRunEffectV2(ctx context.Context, partition control.RunEffectPartitionV2, request control.EffectFactCASRequestV2) (control.EffectFactV2, error) {
	if err := contextError(ctx); err != nil {
		return control.EffectFactV2{}, err
	}
	key, err := runEffectPartitionKeyV2(partition)
	if err != nil {
		return control.EffectFactV2{}, err
	}
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	effects := s.indexedEffects[key]
	current, exists := effects[request.Next.Intent.ID]
	if !exists {
		return control.EffectFactV2{}, core.NewError(core.ErrorNotFound, core.ReasonEffectIntentMissing, "partitioned Run Effect does not exist")
	}
	if current.Revision != request.ExpectedRevision {
		if sameAuthoritativeSettlementV2(current, request.Next) {
			return cloneEffectFactV2(current), nil
		}
		return control.EffectFactV2{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "partitioned Run Effect revision does not match")
	}
	permits := s.indexedPermits[key]
	transition := control.EffectTransitionContextV2{SettlementOwnerMatched: settlementOwnerMatchesBindingV2(request.Next), UnknownInspectionSettled: unknownInspectionSettledV2(effects, request.Next), CompensationSettled: compensationSettledV2(effects, request.Next), ResidualInspectSettled: resolutionSettledV2(effects, request.Next, true), CleanupEffectSettled: resolutionSettledV2(effects, request.Next, false)}
	if current.DispatchPermitID != "" {
		permit, ok := permits[current.DispatchPermitID]
		transition.PermitBegun = ok && permit.State == control.DispatchPermitBegun
		transition.DispatchReceiptMatched = ok && providerReceiptMatchesPermitV2(request.Next, permit, now)
	}
	if err := control.ValidateEffectFactTransitionV2(current, request.Next, transition, now); err != nil {
		return control.EffectFactV2{}, err
	}
	effects[current.Intent.ID] = cloneEffectFactV2(request.Next)
	return cloneEffectFactV2(request.Next), nil
}

func (s *EffectStoreV2) IssueRunDispatchPermitV2(ctx context.Context, partition control.RunEffectPartitionV2, request control.IssueDispatchPermitRequestV2) (control.IssueDispatchPermitResultV2, error) {
	if err := contextError(ctx); err != nil {
		return control.IssueDispatchPermitResultV2{}, err
	}
	if err := request.Permit.Validate(); err != nil {
		return control.IssueDispatchPermitResultV2{}, err
	}
	if err := request.Fence.Validate(); err != nil {
		return control.IssueDispatchPermitResultV2{}, err
	}
	key, err := runEffectPartitionKeyV2(partition)
	if err != nil {
		return control.IssueDispatchPermitResultV2{}, err
	}
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	effects := s.indexedEffects[key]
	permits := s.indexedPermits[key]
	if permits == nil {
		permits = map[string]control.DispatchPermitFactV2{}
		s.indexedPermits[key] = permits
	}
	if existing, exists := permits[request.Permit.ID]; exists {
		permitDigest, _ := request.Permit.DigestV2()
		fenceDigest, _ := ports.DigestExecutionFenceV2(request.Fence)
		current, effectExists := effects[request.EffectID]
		if effectExists && existing.Permit.AttemptID == request.Permit.AttemptID && existing.Permit.IntentID == request.EffectID && existing.PermitDigest == permitDigest && existing.Permit.FenceDigest == fenceDigest && reflect.DeepEqual(existing.Fence, request.Fence) {
			return control.IssueDispatchPermitResultV2{Effect: cloneEffectFactV2(current), Permit: clonePermitFactV2(existing)}, nil
		}
		return control.IssueDispatchPermitResultV2{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "partitioned permit identity already binds different content")
	}
	effect, exists := effects[request.EffectID]
	if !exists || effect.Revision != request.ExpectedEffectRevision || effect.State != control.EffectAccepted {
		return control.IssueDispatchPermitResultV2{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "permit issue requires current partitioned accepted Effect")
	}
	intentDigest, _ := effect.Intent.DigestV2()
	fenceDigest, fenceErr := ports.DigestExecutionFenceV2(request.Fence)
	if mismatch := partitionedPermitMismatchV2(request.Permit, effect.Intent, partition, intentDigest, fenceDigest, fenceErr, now); mismatch != "" {
		return control.IssueDispatchPermitResultV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonDispatchPermitInvalid, "partitioned permit drifted at "+mismatch)
	}
	permitDigest, _ := request.Permit.DigestV2()
	next := effect
	next.State = control.EffectDispatchIntent
	next.Revision++
	next.DispatchPermitID = request.Permit.ID
	next.DispatchPermitDigest = permitDigest
	next.UpdatedUnixNano = now.UnixNano()
	if err := control.ValidateEffectFactTransitionV2(effect, next, control.EffectTransitionContextV2{}, now); err != nil {
		return control.IssueDispatchPermitResultV2{}, err
	}
	permit := control.DispatchPermitFactV2{Permit: request.Permit, PermitDigest: permitDigest, Fence: request.Fence, State: control.DispatchPermitIssued, Revision: 1, EffectFactRevision: next.Revision}
	if err := permit.Validate(); err != nil {
		return control.IssueDispatchPermitResultV2{}, err
	}
	effects[effect.Intent.ID], permits[request.Permit.ID] = cloneEffectFactV2(next), clonePermitFactV2(permit)
	if s.loseNextIssueReply {
		s.loseNextIssueReply = false
		return control.IssueDispatchPermitResultV2{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected partitioned permit reply loss")
	}
	return control.IssueDispatchPermitResultV2{Effect: cloneEffectFactV2(next), Permit: clonePermitFactV2(permit)}, nil
}

func partitionedPermitMismatchV2(permit ports.DispatchPermitV2, intent ports.EffectIntentV2, partition control.RunEffectPartitionV2, intentDigest, fenceDigest core.Digest, fenceErr error, now time.Time) string {
	checks := []struct {
		name string
		ok   bool
	}{
		{"fence", fenceErr == nil && permit.FenceDigest == fenceDigest}, {"intent_id", permit.IntentID == intent.ID}, {"intent_revision", permit.IntentRevision == intent.Revision}, {"intent_digest", permit.IntentDigest == intentDigest}, {"payload_schema", permit.PayloadSchema == intent.Payload.Schema}, {"payload_digest", permit.PayloadDigest == intent.Payload.ContentDigest}, {"payload_revision", permit.PayloadRevision == intent.PayloadRevision}, {"run", permit.RunID == intent.RunID && permit.RunID == partition.RunID}, {"conflict_domain", permit.ConflictDomain == intent.ConflictDomain}, {"provider", permit.Provider == intent.Provider}, {"enforcement", permit.EnforcementPoint == intent.Provider}, {"authority", permit.Authority == intent.Authority}, {"review", permit.Review == intent.Review}, {"budget", permit.Budget == intent.Budget}, {"policy", permit.Policy == intent.Policy}, {"current_scope", permit.CurrentScope == intent.CurrentScope}, {"idempotency", permit.Idempotency == intent.Idempotency}, {"execution_scope", sameScopeForEffectStoreV2(permit.Scope, intent.Scope)}, {"ttl", now.Before(time.Unix(0, permit.ExpiresUnixNano))},
	}
	for _, check := range checks {
		if !check.ok {
			return check.name
		}
	}
	return ""
}

func (s *EffectStoreV2) InspectRunDispatchPermitV2(ctx context.Context, partition control.RunEffectPartitionV2, id string) (control.DispatchPermitFactV2, error) {
	if err := contextError(ctx); err != nil {
		return control.DispatchPermitFactV2{}, err
	}
	key, err := runEffectPartitionKeyV2(partition)
	if err != nil {
		return control.DispatchPermitFactV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	permit, ok := s.indexedPermits[key][id]
	if !ok {
		return control.DispatchPermitFactV2{}, core.NewError(core.ErrorNotFound, core.ReasonDispatchPermitInvalid, "partitioned permit does not exist")
	}
	return clonePermitFactV2(permit), nil
}

func (s *EffectStoreV2) BeginRunDispatchV2(ctx context.Context, partition control.RunEffectPartitionV2, request control.BeginDispatchRequestV2) (control.DispatchPermitFactV2, error) {
	if err := contextError(ctx); err != nil {
		return control.DispatchPermitFactV2{}, err
	}
	key, err := runEffectPartitionKeyV2(partition)
	if err != nil {
		return control.DispatchPermitFactV2{}, err
	}
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	effect, effectOK := s.indexedEffects[key][request.EffectID]
	permit, permitOK := s.indexedPermits[key][request.PermitID]
	if !effectOK || !permitOK || effect.State != control.EffectDispatchIntent || effect.Revision != request.ExpectedEffectRevision || permit.State != control.DispatchPermitIssued || permit.Revision != request.ExpectedPermitRevision || permit.EffectFactRevision != effect.Revision {
		return control.DispatchPermitFactV2{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "begin requires exact partitioned Effect and permit")
	}
	next := permit
	next.State = control.DispatchPermitBegun
	next.Revision++
	next.BegunUnixNano = now.UnixNano()
	if err := control.ValidateDispatchPermitTransitionV2(permit, next, now); err != nil {
		return control.DispatchPermitFactV2{}, err
	}
	s.indexedPermits[key][request.PermitID] = clonePermitFactV2(next)
	if s.loseNextBeginReply {
		s.loseNextBeginReply = false
		return control.DispatchPermitFactV2{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected partitioned Begin reply loss")
	}
	return clonePermitFactV2(next), nil
}

func (s *EffectStoreV2) RecordRunEnforcementReceiptV2(ctx context.Context, partition control.RunEffectPartitionV2, request control.RecordEnforcementReceiptRequestV2) (control.DispatchPermitFactV2, error) {
	if err := contextError(ctx); err != nil {
		return control.DispatchPermitFactV2{}, err
	}
	if err := request.Receipt.Validate(); err != nil {
		return control.DispatchPermitFactV2{}, err
	}
	key, err := runEffectPartitionKeyV2(partition)
	if err != nil {
		return control.DispatchPermitFactV2{}, err
	}
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.indexedPermits[key][request.PermitID]
	if !ok {
		return control.DispatchPermitFactV2{}, core.NewError(core.ErrorNotFound, core.ReasonDispatchPermitInvalid, "partitioned permit does not exist")
	}
	if current.Enforcement != nil {
		if reflect.DeepEqual(*current.Enforcement, request.Receipt) {
			return clonePermitFactV2(current), nil
		}
		return control.DispatchPermitFactV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "partitioned permit has different enforcement")
	}
	if current.Revision != request.ExpectedPermitRevision {
		return control.DispatchPermitFactV2{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "partitioned permit revision drifted")
	}
	next := current
	next.Revision++
	next.Enforcement = &request.Receipt
	if err := control.ValidateDispatchPermitTransitionV2(current, next, now); err != nil {
		return control.DispatchPermitFactV2{}, err
	}
	s.indexedPermits[key][request.PermitID] = clonePermitFactV2(next)
	return clonePermitFactV2(next), nil
}

func (s *EffectStoreV2) CompareAndSwapRunDispatchPermitV2(ctx context.Context, partition control.RunEffectPartitionV2, request control.DispatchPermitFactCASRequestV2) (control.DispatchPermitFactV2, error) {
	if err := contextError(ctx); err != nil {
		return control.DispatchPermitFactV2{}, err
	}
	key, err := runEffectPartitionKeyV2(partition)
	if err != nil {
		return control.DispatchPermitFactV2{}, err
	}
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.indexedPermits[key][request.PermitID]
	if !ok {
		return control.DispatchPermitFactV2{}, core.NewError(core.ErrorNotFound, core.ReasonDispatchPermitInvalid, "partitioned permit does not exist")
	}
	if current.Revision != request.ExpectedRevision {
		if reflect.DeepEqual(current, request.Next) {
			return clonePermitFactV2(current), nil
		}
		return control.DispatchPermitFactV2{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "partitioned permit revision drifted")
	}
	if err := control.ValidateDispatchPermitTransitionV2(current, request.Next, now); err != nil {
		return control.DispatchPermitFactV2{}, err
	}
	s.indexedPermits[key][request.PermitID] = clonePermitFactV2(request.Next)
	return clonePermitFactV2(request.Next), nil
}

// CreateRunEffectIndexV2 creates the per-Run enumeration fact. The fake keeps
// this fact and EffectFactV2 under one mutex solely to exercise the required
// Fact Owner transaction semantics; it is not a production transaction backend.
func (s *EffectStoreV2) CreateRunEffectIndexV2(ctx context.Context, initial control.RunEffectIndexFactV2) (control.RunEffectIndexFactV2, error) {
	if err := contextError(ctx); err != nil {
		return control.RunEffectIndexFactV2{}, err
	}
	if err := initial.Validate(); err != nil {
		return control.RunEffectIndexFactV2{}, err
	}
	if initial.State != control.RunEffectIndexOpen || initial.Revision != 1 || initial.Watermark != 1 || initial.SegmentCount != 0 || initial.EffectCount != 0 || initial.HeadSegmentDigest != ports.EvidenceGenesisDigestV2 {
		return control.RunEffectIndexFactV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonRunEffectIndexConflict, "new Run effect index must be empty, open and revision one")
	}
	if err := s.validateCurrentRunForIndexV2(ctx, initial.PartitionV2()); err != nil {
		return control.RunEffectIndexFactV2{}, err
	}
	key, err := runEffectPartitionKeyV2(initial.PartitionV2())
	if err != nil {
		return control.RunEffectIndexFactV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if current, exists := s.runEffects[key]; exists {
		if sameRunEffectIndexV2(current, initial) {
			return cloneRunEffectIndexV2(current), nil
		}
		return control.RunEffectIndexFactV2{}, core.NewError(core.ErrorConflict, core.ReasonRunEffectIndexConflict, "Run effect index already exists with different content")
	}
	s.runEffects[key] = cloneRunEffectIndexV2(initial)
	s.runEffectSegments[key] = map[uint64]control.RunEffectSegmentFactV2{}
	s.indexedEffects[key] = map[core.EffectIntentID]control.EffectFactV2{}
	if s.consumeRunEffectReplyLossLocked() {
		return control.RunEffectIndexFactV2{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected Run effect index reply loss")
	}
	return cloneRunEffectIndexV2(initial), nil
}

func (s *EffectStoreV2) ListRunEffectSegmentsV2(ctx context.Context, partition control.RunEffectPartitionV2, after uint64, limit uint32) (control.RunEffectSegmentPageV2, error) {
	if err := contextError(ctx); err != nil {
		return control.RunEffectSegmentPageV2{}, err
	}
	if limit == 0 || limit > 128 {
		return control.RunEffectSegmentPageV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "Run effect segment page limit must be between one and 128")
	}
	key, err := runEffectPartitionKeyV2(partition)
	if err != nil {
		return control.RunEffectSegmentPageV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	root, exists := s.runEffects[key]
	if !exists {
		return control.RunEffectSegmentPageV2{}, core.NewError(core.ErrorNotFound, core.ReasonRunEffectIndexConflict, "Run effect index does not exist")
	}
	segments := make([]control.RunEffectSegmentFactV2, 0, limit)
	next := after + 1
	for next <= root.SegmentCount && uint32(len(segments)) < limit {
		segment, exists := s.runEffectSegments[key][next]
		if !exists {
			return control.RunEffectSegmentPageV2{}, core.NewError(core.ErrorInternal, core.ReasonRunEffectIndexConflict, "Run effect segment chain has a gap")
		}
		segments = append(segments, cloneRunEffectSegmentV2(segment))
		next++
	}
	if next > root.SegmentCount {
		next = 0
	}
	return control.RunEffectSegmentPageV2{Segments: segments, NextNumber: next}, nil
}

func (s *EffectStoreV2) InspectRunEffectIndexV2(ctx context.Context, partition control.RunEffectPartitionV2) (control.RunEffectIndexFactV2, error) {
	if err := contextError(ctx); err != nil {
		return control.RunEffectIndexFactV2{}, err
	}
	key, err := runEffectPartitionKeyV2(partition)
	if err != nil {
		return control.RunEffectIndexFactV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, exists := s.runEffects[key]
	if !exists {
		return control.RunEffectIndexFactV2{}, core.NewError(core.ErrorNotFound, core.ReasonRunEffectIndexConflict, "Run effect index does not exist")
	}
	return cloneRunEffectIndexV2(current), nil
}

func (s *EffectStoreV2) CreateEffectForRunV2(ctx context.Context, request control.CreateRunEffectRequestV2) (control.CreateRunEffectResultV2, error) {
	if err := contextError(ctx); err != nil {
		return control.CreateRunEffectResultV2{}, err
	}
	if err := request.Effect.Validate(); err != nil {
		return control.CreateRunEffectResultV2{}, err
	}
	if request.Effect.State != control.EffectProposed || request.Effect.Revision != 1 || request.ExpectedIndexRevision == 0 {
		return control.CreateRunEffectResultV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonRunEffectIndexConflict, "atomic Run Effect create requires proposed Effect revision one and expected index revision")
	}
	if err := request.Partition.Validate(); err != nil {
		return control.CreateRunEffectResultV2{}, err
	}
	if request.Partition.RunID != request.Effect.Intent.RunID || !ports.SameExecutionScopeV2(request.Partition.ExecutionScope, request.Effect.Intent.Scope) {
		return control.CreateRunEffectResultV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonRunEffectIndexConflict, "Effect does not belong to the requested stable Run partition")
	}
	if err := s.validateCurrentRunForEffectV2(ctx, request.Effect); err != nil {
		return control.CreateRunEffectResultV2{}, err
	}
	key, err := runEffectPartitionKeyV2(request.Partition)
	if err != nil {
		return control.CreateRunEffectResultV2{}, err
	}
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	currentIndex, exists := s.runEffects[key]
	if !exists {
		return control.CreateRunEffectResultV2{}, core.NewError(core.ErrorNotFound, core.ReasonRunEffectIndexConflict, "Run effect index does not exist")
	}
	if currentEffect, exists := s.indexedEffects[key][request.Effect.Intent.ID]; exists {
		ref, segment, found := s.findRunEffectRefLockedV2(currentIndex, request.Effect.Intent.ID)
		if currentEffect.IntentDigest == request.Effect.IntentDigest && found && ref.IntentDigest == request.Effect.IntentDigest {
			return control.CreateRunEffectResultV2{Effect: cloneEffectFactV2(currentEffect), Index: cloneRunEffectIndexV2(currentIndex), Segment: cloneRunEffectSegmentV2(segment)}, nil
		}
		return control.CreateRunEffectResultV2{}, core.NewError(core.ErrorConflict, core.ReasonRunEffectIndexConflict, "Effect and Run index do not form the same atomic fact")
	}
	if currentIndex.State != control.RunEffectIndexOpen {
		return control.CreateRunEffectResultV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonRunEffectSetFrozen, "frozen Run effect set rejects new Effects")
	}
	if currentIndex.Revision != request.ExpectedIndexRevision {
		return control.CreateRunEffectResultV2{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Run effect index revision does not match create precondition")
	}
	if currentIndex.EffectCount == ^uint64(0) || currentIndex.SegmentCount == ^uint64(0) || uint64(currentIndex.Revision) == ^uint64(0) || currentIndex.Watermark == ^uint64(0) {
		return control.CreateRunEffectResultV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "Run effect index counters cannot overflow")
	}
	scopeDigest, err := ports.ExecutionScopeDigestV2(request.Effect.Intent.Scope)
	if err != nil || scopeDigest != currentIndex.ExecutionScopeDigest {
		return control.CreateRunEffectResultV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonRunEffectIndexConflict, "Effect scope differs from the Run effect index")
	}
	nextIndex := cloneRunEffectIndexV2(currentIndex)
	ref := control.RunEffectRefV2{EffectID: request.Effect.Intent.ID, IntentRevision: request.Effect.Intent.Revision, IntentDigest: request.Effect.IntentDigest, FactRevision: request.Effect.Revision}
	segments := s.runEffectSegments[key]
	var nextSegment control.RunEffectSegmentFactV2
	if nextIndex.SegmentCount == 0 {
		nextSegment = control.RunEffectSegmentFactV2{ContractVersion: ports.RunSettlementContractVersionV2, ID: runEffectSegmentIDV2(key, nextIndex.RunID, 1), RunID: nextIndex.RunID, RunIdentityDigest: nextIndex.RunIdentityDigest, ExecutionScopeDigest: nextIndex.ExecutionScopeDigest, Number: 1, Revision: 1, PreviousDigest: ports.EvidenceGenesisDigestV2, Effects: []control.RunEffectRefV2{ref}, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()}
		nextIndex.SegmentCount = 1
	} else {
		currentSegment := segments[nextIndex.SegmentCount]
		if len(currentSegment.Effects) >= control.MaxRunEffectSegmentEntriesV2 {
			nextSegment = control.RunEffectSegmentFactV2{ContractVersion: ports.RunSettlementContractVersionV2, ID: runEffectSegmentIDV2(key, nextIndex.RunID, nextIndex.SegmentCount+1), RunID: nextIndex.RunID, RunIdentityDigest: nextIndex.RunIdentityDigest, ExecutionScopeDigest: nextIndex.ExecutionScopeDigest, Number: nextIndex.SegmentCount + 1, Revision: 1, PreviousDigest: nextIndex.HeadSegmentDigest, Effects: []control.RunEffectRefV2{ref}, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()}
			nextIndex.SegmentCount++
		} else {
			if uint64(currentSegment.Revision) == ^uint64(0) {
				return control.CreateRunEffectResultV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "Run effect segment revision cannot overflow")
			}
			nextSegment = cloneRunEffectSegmentV2(currentSegment)
			nextSegment.Effects = append(nextSegment.Effects, ref)
			control.SortRunEffectRefsV2(nextSegment.Effects)
			nextSegment.Revision++
			nextSegment.UpdatedUnixNano = now.UnixNano()
		}
	}
	if err := nextSegment.Validate(); err != nil {
		return control.CreateRunEffectResultV2{}, err
	}
	segmentDigest, _ := nextSegment.DigestV2()
	nextIndex.HeadSegmentDigest = segmentDigest
	nextIndex.EffectCount++
	nextIndex.Revision++
	nextIndex.Watermark++
	if err := nextIndex.Validate(); err != nil {
		return control.CreateRunEffectResultV2{}, err
	}
	s.indexedEffects[key][request.Effect.Intent.ID] = cloneEffectFactV2(request.Effect)
	segments[nextSegment.Number] = cloneRunEffectSegmentV2(nextSegment)
	s.runEffects[key] = cloneRunEffectIndexV2(nextIndex)
	if s.consumeRunEffectReplyLossLocked() {
		return control.CreateRunEffectResultV2{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected atomic Effect and Run index reply loss")
	}
	return control.CreateRunEffectResultV2{Effect: cloneEffectFactV2(request.Effect), Index: cloneRunEffectIndexV2(nextIndex), Segment: cloneRunEffectSegmentV2(nextSegment)}, nil
}

func (s *EffectStoreV2) FreezeRunEffectSetV2(ctx context.Context, request control.FreezeRunEffectSetRequestV2) (control.RunEffectIndexFactV2, error) {
	if err := contextError(ctx); err != nil {
		return control.RunEffectIndexFactV2{}, err
	}
	if request.ExpectedIndexRevision == 0 || request.ExpectedRunRevision == 0 || request.Partition.Validate() != nil {
		return control.RunEffectIndexFactV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonRunEffectSetFrozen, "freeze requires expected Run and index revisions")
	}
	if err := s.validateCurrentRunForFreezeV2(ctx, request); err != nil {
		return control.RunEffectIndexFactV2{}, err
	}
	key, err := runEffectPartitionKeyV2(request.Partition)
	if err != nil {
		return control.RunEffectIndexFactV2{}, err
	}
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	current, exists := s.runEffects[key]
	if !exists {
		return control.RunEffectIndexFactV2{}, core.NewError(core.ErrorNotFound, core.ReasonRunEffectIndexConflict, "Run effect index does not exist")
	}
	if current.State == control.RunEffectIndexFrozen {
		return cloneRunEffectIndexV2(current), nil
	}
	if current.Revision != request.ExpectedIndexRevision {
		return control.RunEffectIndexFactV2{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Run effect index changed before freeze")
	}
	if uint64(current.Revision) == ^uint64(0) || current.Watermark == ^uint64(0) {
		return control.RunEffectIndexFactV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "Run effect freeze counters cannot overflow")
	}
	next := cloneRunEffectIndexV2(current)
	next.State = control.RunEffectIndexFrozen
	next.Revision++
	next.Watermark++
	next.FrozenUnixNano = now.UnixNano()
	if err := next.Validate(); err != nil {
		return control.RunEffectIndexFactV2{}, err
	}
	s.runEffects[key] = cloneRunEffectIndexV2(next)
	if s.consumeRunEffectReplyLossLocked() {
		return control.RunEffectIndexFactV2{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected Run effect freeze reply loss")
	}
	return cloneRunEffectIndexV2(next), nil
}

func (s *EffectStoreV2) consumeRunEffectReplyLossLocked() bool {
	if !s.loseNextRunEffectReply {
		return false
	}
	s.loseNextRunEffectReply = false
	return true
}

func cloneRunEffectIndexV2(fact control.RunEffectIndexFactV2) control.RunEffectIndexFactV2 {
	fact.ExecutionScope = cloneScope(fact.ExecutionScope)
	return fact
}

func cloneRunEffectSegmentV2(fact control.RunEffectSegmentFactV2) control.RunEffectSegmentFactV2 {
	fact.Effects = append([]control.RunEffectRefV2{}, fact.Effects...)
	return fact
}

func sameRunEffectIndexV2(left, right control.RunEffectIndexFactV2) bool {
	leftDigest, leftErr := left.DigestV2()
	rightDigest, rightErr := right.DigestV2()
	return leftErr == nil && rightErr == nil && leftDigest == rightDigest
}

func (s *EffectStoreV2) findRunEffectRefLockedV2(index control.RunEffectIndexFactV2, id core.EffectIntentID) (control.RunEffectRefV2, control.RunEffectSegmentFactV2, bool) {
	for number := uint64(1); number <= index.SegmentCount; number++ {
		key, _ := runEffectPartitionKeyV2(index.PartitionV2())
		segment := s.runEffectSegments[key][number]
		for _, ref := range segment.Effects {
			if ref.EffectID == id {
				return ref, segment, true
			}
		}
	}
	return control.RunEffectRefV2{}, control.RunEffectSegmentFactV2{}, false
}

func runEffectSegmentIDV2(partitionKey string, runID core.AgentRunID, number uint64) string {
	digest, _ := core.CanonicalJSONDigest("praxis.runtime.run-settlement", ports.RunSettlementContractVersionV2, "RunEffectSegmentIdentityV2", struct {
		PartitionKey string          `json:"partition_key"`
		RunID        core.AgentRunID `json:"run_id"`
		Number       uint64          `json:"number"`
	}{partitionKey, runID, number})
	return "effect-segment:" + string(digest)
}

func (s *EffectStoreV2) validateCurrentRunForIndexV2(ctx context.Context, partition control.RunEffectPartitionV2) error {
	s.mu.Lock()
	reader := s.runFacts
	s.mu.Unlock()
	if reader == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "indexed Effect owner requires current Run facts")
	}
	run, err := reader.InspectRun(ctx, partition.ExecutionScope, partition.RunID)
	if err != nil {
		return err
	}
	runIdentity, _ := ports.RunIdentityDigestV2(run)
	if (run.Status != core.RunPending && run.Status != core.RunRunning) || runIdentity != partition.RunIdentityDigest || !ports.SameExecutionScopeV2(run.Scope, partition.ExecutionScope) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonRunEffectIndexConflict, "Run effect root requires the exact current pending/running Run identity")
	}
	return nil
}

func (s *EffectStoreV2) validateCurrentRunForEffectV2(ctx context.Context, effect control.EffectFactV2) error {
	s.mu.Lock()
	reader := s.runFacts
	s.mu.Unlock()
	if reader == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "indexed Effect create requires current Run facts")
	}
	run, err := reader.InspectRun(ctx, effect.Intent.Scope, effect.Intent.RunID)
	if err != nil {
		return err
	}
	if run.Status != core.RunRunning {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonRunEffectSetFrozen, "stopping or terminal Run rejects new Effects")
	}
	return nil
}

func (s *EffectStoreV2) validateCurrentRunForFreezeV2(ctx context.Context, request control.FreezeRunEffectSetRequestV2) error {
	s.mu.Lock()
	reader := s.runFacts
	key, keyErr := runEffectPartitionKeyV2(request.Partition)
	root := s.runEffects[key]
	s.mu.Unlock()
	if keyErr != nil {
		return keyErr
	}
	if reader == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Effect freeze requires current Run facts")
	}
	run, err := reader.InspectRun(ctx, request.Partition.ExecutionScope, request.Partition.RunID)
	if err != nil {
		return err
	}
	runIdentity, _ := ports.RunIdentityDigestV2(run)
	if run.Status != core.RunStopping || run.Revision != request.ExpectedRunRevision || runIdentity != root.RunIdentityDigest || !ports.SameExecutionScopeV2(run.Scope, request.Partition.ExecutionScope) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonRunEffectSetFrozen, "Effect freeze requires exact current stopping Run revision")
	}
	return nil
}

func (s *EffectStoreV2) InspectRunEffectV2(ctx context.Context, partition control.RunEffectPartitionV2, effectID core.EffectIntentID) (control.EffectFactV2, error) {
	if err := contextError(ctx); err != nil {
		return control.EffectFactV2{}, err
	}
	key, err := runEffectPartitionKeyV2(partition)
	if err != nil {
		return control.EffectFactV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	fact, exists := s.indexedEffects[key][effectID]
	if !exists || fact.Intent.RunID != partition.RunID || !ports.SameExecutionScopeV2(fact.Intent.Scope, partition.ExecutionScope) {
		return control.EffectFactV2{}, core.NewError(core.ErrorNotFound, core.ReasonEffectIntentMissing, "Run-scoped Effect does not exist in the requested partition")
	}
	return cloneEffectFactV2(fact), nil
}

func runEffectPartitionKeyV2(partition control.RunEffectPartitionV2) (string, error) {
	digest, err := partition.DigestV2()
	if err != nil {
		return "", err
	}
	return string(digest), nil
}

var _ control.RunEffectFactPortV2 = (*EffectStoreV2)(nil)
