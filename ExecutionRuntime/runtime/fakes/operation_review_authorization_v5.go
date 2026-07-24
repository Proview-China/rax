package fakes

import (
	"context"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// OperationReviewAuthorizationStoreV5 is an append-only reference/test store.
// It is not a production backend and makes no durability or SLA claim.
type OperationReviewAuthorizationStoreV5 struct {
	mu            sync.Mutex
	clock         func() time.Time
	history       map[string]map[core.Revision]ports.OperationReviewAuthorizationFactV5
	current       map[string]ports.OperationReviewAuthorizationRefV5
	activeEffect  map[string]string
	loseCreate    bool
	loseCAS       bool
	loseInspect   bool
	createCommits uint64
}

func NewOperationReviewAuthorizationStoreV5(clock func() time.Time) *OperationReviewAuthorizationStoreV5 {
	return &OperationReviewAuthorizationStoreV5{
		clock: clock, history: make(map[string]map[core.Revision]ports.OperationReviewAuthorizationFactV5),
		current: make(map[string]ports.OperationReviewAuthorizationRefV5), activeEffect: make(map[string]string),
	}
}

func (s *OperationReviewAuthorizationStoreV5) LoseNextCreateReplyV5() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loseCreate = true
}
func (s *OperationReviewAuthorizationStoreV5) LoseNextCASReplyV5() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loseCAS = true
}
func (s *OperationReviewAuthorizationStoreV5) LoseNextInspectReplyV5() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loseInspect = true
}
func (s *OperationReviewAuthorizationStoreV5) CreateCommitCountV5() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.createCommits
}

func (s *OperationReviewAuthorizationStoreV5) CreateOperationReviewAuthorizationV5(ctx context.Context, fact ports.OperationReviewAuthorizationFactV5) (ports.OperationReviewAuthorizationFactV5, error) {
	if err := contextErrorV5(ctx); err != nil {
		return ports.OperationReviewAuthorizationFactV5{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := fact.Validate(); err != nil {
		return ports.OperationReviewAuthorizationFactV5{}, err
	}
	if ref, exists := s.current[fact.ID]; exists {
		current := s.history[fact.ID][ref.Revision]
		if current.Digest == fact.Digest {
			return cloneOperationReviewAuthorizationFactV5(current), nil
		}
		return ports.OperationReviewAuthorizationFactV5{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Review Authorization V5 ID contains different content")
	}
	if s.clock == nil || fact.Revision != 1 || fact.State != ports.OperationReviewAuthorizationActiveV5 {
		return ports.OperationReviewAuthorizationFactV5{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidTransition, "new Review Authorization V5 must be active revision one")
	}
	now := s.clock()
	if now.IsZero() || fact.CreatedUnixNano > now.UnixNano() || fact.UpdatedUnixNano != fact.CreatedUnixNano || !now.Before(time.Unix(0, fact.ExpiresUnixNano)) {
		return ports.OperationReviewAuthorizationFactV5{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "new Review Authorization V5 time is inconsistent")
	}
	effectKey := operationReviewAuthorizationEffectKeyV5(fact)
	if owner, occupied := s.activeEffect[effectKey]; occupied && owner != fact.ID {
		return ports.OperationReviewAuthorizationFactV5{}, core.NewError(core.ErrorConflict, core.ReasonEffectConflictDomainOccupied, "another V5 Review Authorization is current for this Effect")
	}
	stored := cloneOperationReviewAuthorizationFactV5(fact)
	s.history[fact.ID] = map[core.Revision]ports.OperationReviewAuthorizationFactV5{fact.Revision: stored}
	s.current[fact.ID] = fact.RefV5()
	s.activeEffect[effectKey] = fact.ID
	s.createCommits++
	if s.loseCreate {
		s.loseCreate = false
		return ports.OperationReviewAuthorizationFactV5{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected lost V5 create reply")
	}
	return cloneOperationReviewAuthorizationFactV5(stored), nil
}

func (s *OperationReviewAuthorizationStoreV5) InspectOperationReviewAuthorizationV5(ctx context.Context, id string) (ports.OperationReviewAuthorizationFactV5, error) {
	if err := contextErrorV5(ctx); err != nil {
		return ports.OperationReviewAuthorizationFactV5{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.loseInspect {
		s.loseInspect = false
		return ports.OperationReviewAuthorizationFactV5{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "injected lost V5 inspect reply")
	}
	ref, exists := s.current[id]
	if !exists {
		return ports.OperationReviewAuthorizationFactV5{}, core.NewError(core.ErrorNotFound, core.ReasonReviewVerdictMissing, "Review Authorization V5 is absent")
	}
	return cloneOperationReviewAuthorizationFactV5(s.history[id][ref.Revision]), nil
}

func (s *OperationReviewAuthorizationStoreV5) InspectOperationReviewAuthorizationExactV5(ctx context.Context, ref ports.OperationReviewAuthorizationRefV5) (ports.OperationReviewAuthorizationFactV5, error) {
	if err := contextErrorV5(ctx); err != nil {
		return ports.OperationReviewAuthorizationFactV5{}, err
	}
	if err := ref.Validate(); err != nil {
		return ports.OperationReviewAuthorizationFactV5{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	byRevision, exists := s.history[ref.ID]
	if !exists {
		return ports.OperationReviewAuthorizationFactV5{}, core.NewError(core.ErrorNotFound, core.ReasonReviewVerdictMissing, "Review Authorization V5 history is absent")
	}
	fact, exists := byRevision[ref.Revision]
	if !exists {
		return ports.OperationReviewAuthorizationFactV5{}, core.NewError(core.ErrorNotFound, core.ReasonReviewVerdictMissing, "Review Authorization V5 revision is absent")
	}
	if fact.Digest != ref.Digest {
		return ports.OperationReviewAuthorizationFactV5{}, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Review Authorization V5 exact ref digest drifted")
	}
	return cloneOperationReviewAuthorizationFactV5(fact), nil
}

func (s *OperationReviewAuthorizationStoreV5) CompareAndSwapOperationReviewAuthorizationV5(ctx context.Context, request ports.OperationReviewAuthorizationCASRequestV5) (ports.OperationReviewAuthorizationFactV5, error) {
	if err := contextErrorV5(ctx); err != nil {
		return ports.OperationReviewAuthorizationFactV5{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := request.Next.Validate(); err != nil {
		return ports.OperationReviewAuthorizationFactV5{}, err
	}
	ref, exists := s.current[request.Next.ID]
	if !exists {
		return ports.OperationReviewAuthorizationFactV5{}, core.NewError(core.ErrorNotFound, core.ReasonReviewVerdictMissing, "Review Authorization V5 is absent")
	}
	current := s.history[request.Next.ID][ref.Revision]
	if current.Digest == request.Next.Digest {
		return cloneOperationReviewAuthorizationFactV5(current), nil
	}
	if current.Revision != request.ExpectedRevision {
		return ports.OperationReviewAuthorizationFactV5{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Review Authorization V5 revision changed")
	}
	if s.clock == nil {
		return ports.OperationReviewAuthorizationFactV5{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Review Authorization V5 store clock is required")
	}
	if err := ports.ValidateOperationReviewAuthorizationTransitionV5(current, request.Next, s.clock()); err != nil {
		return ports.OperationReviewAuthorizationFactV5{}, err
	}
	if _, exists := s.history[current.ID][request.Next.Revision]; exists {
		return ports.OperationReviewAuthorizationFactV5{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Review Authorization V5 history revision exists")
	}
	stored := cloneOperationReviewAuthorizationFactV5(request.Next)
	s.history[current.ID][request.Next.Revision] = stored
	s.current[current.ID] = request.Next.RefV5()
	delete(s.activeEffect, operationReviewAuthorizationEffectKeyV5(current))
	if s.loseCAS {
		s.loseCAS = false
		return ports.OperationReviewAuthorizationFactV5{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected lost V5 CAS reply")
	}
	return cloneOperationReviewAuthorizationFactV5(stored), nil
}

func operationReviewAuthorizationEffectKeyV5(fact ports.OperationReviewAuthorizationFactV5) string {
	operationDigest, _ := fact.Intent.Operation.DigestV3()
	return string(fact.Intent.Operation.ExecutionScope.Identity.TenantID) + "\x00" + string(operationDigest) + "\x00" + string(fact.Intent.IntentID)
}

func cloneOperationReviewAuthorizationFactV5(fact ports.OperationReviewAuthorizationFactV5) ports.OperationReviewAuthorizationFactV5 {
	if fact.Review.Quorum != nil {
		q := *fact.Review.Quorum
		q.SatisfiedRoleCounts = append([]ports.OperationReviewRoleCountV5{}, q.SatisfiedRoleCounts...)
		q.ReviewerAuthorityRefs = append([]ports.OperationGovernanceFactRefV3{}, q.ReviewerAuthorityRefs...)
		q.BindingRefs = append([]ports.OperationGovernanceFactRefV3{}, q.BindingRefs...)
		q.DecisionEvidence = append([]ports.EvidenceRecordRefV2{}, q.DecisionEvidence...)
		if q.Satisfaction != nil {
			satisfaction := *q.Satisfaction
			satisfaction.Evidence = append([]ports.EvidenceRecordRefV2{}, satisfaction.Evidence...)
			q.Satisfaction = &satisfaction
		}
		if q.Operation.ExecutionScope.SandboxLease != nil {
			lease := *q.Operation.ExecutionScope.SandboxLease
			q.Operation.ExecutionScope.SandboxLease = &lease
		}
		fact.Review.Quorum = &q
	}
	if fact.Review.PolicyNotRequired != nil {
		n := *fact.Review.PolicyNotRequired
		if n.Operation.ExecutionScope.SandboxLease != nil {
			lease := *n.Operation.ExecutionScope.SandboxLease
			n.Operation.ExecutionScope.SandboxLease = &lease
		}
		fact.Review.PolicyNotRequired = &n
	}
	if fact.Intent.Operation.ExecutionScope.SandboxLease != nil {
		lease := *fact.Intent.Operation.ExecutionScope.SandboxLease
		fact.Intent.Operation.ExecutionScope.SandboxLease = &lease
	}
	if fact.Fence.Scope.SandboxLease != nil {
		lease := *fact.Fence.Scope.SandboxLease
		fact.Fence.Scope.SandboxLease = &lease
	}
	return fact
}

func contextErrorV5(ctx context.Context) error {
	if ctx == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "context is required")
	}
	if err := ctx.Err(); err != nil {
		return core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "Review Authorization V5 context ended")
	}
	return nil
}

var _ ports.OperationReviewAuthorizationFactPortV5 = (*OperationReviewAuthorizationStoreV5)(nil)

// OperationReviewAuthorizationSharedStoreV45 is a reference-only composition
// that gives V4 and V5 one atomic active-effect guard. Production persistence
// must provide the same invariant in its own transaction.
type OperationReviewAuthorizationSharedStoreV45 struct {
	mu           sync.Mutex
	v4           *OperationReviewAuthorizationStoreV4
	v5           *OperationReviewAuthorizationStoreV5
	active       map[string]string
	failCreateV4 bool
	failCreateV5 bool
}

func NewOperationReviewAuthorizationSharedStoreV45(clock func() time.Time) *OperationReviewAuthorizationSharedStoreV45 {
	return &OperationReviewAuthorizationSharedStoreV45{v4: NewOperationReviewAuthorizationStoreV4(clock), v5: NewOperationReviewAuthorizationStoreV5(clock), active: make(map[string]string)}
}

func (s *OperationReviewAuthorizationSharedStoreV45) LoseNextCreateReplyV4() {
	s.v4.LoseNextCreateReplyV4()
}

func (s *OperationReviewAuthorizationSharedStoreV45) LoseNextCASReplyV4() {
	s.v4.LoseNextCASReplyV4()
}

func (s *OperationReviewAuthorizationSharedStoreV45) LoseNextCreateReplyV5() {
	s.v5.LoseNextCreateReplyV5()
}

func (s *OperationReviewAuthorizationSharedStoreV45) LoseNextCASReplyV5() {
	s.v5.LoseNextCASReplyV5()
}

func (s *OperationReviewAuthorizationSharedStoreV45) FailNextCreateBeforeCommitV4() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failCreateV4 = true
}

func (s *OperationReviewAuthorizationSharedStoreV45) FailNextCreateBeforeCommitV5() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failCreateV5 = true
}

func (s *OperationReviewAuthorizationSharedStoreV45) CreateOperationReviewAuthorizationV4(ctx context.Context, fact ports.OperationReviewAuthorizationFactV4) (ports.OperationReviewAuthorizationFactV4, error) {
	if ctx == nil {
		return ports.OperationReviewAuthorizationFactV4{}, sharedReviewAuthorizationContextErrorV45()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.v4.InspectOperationReviewAuthorizationV4(ctx, fact.ID); err == nil {
		return s.v4.CreateOperationReviewAuthorizationV4(ctx, fact)
	} else if !core.HasCategory(err, core.ErrorNotFound) {
		return ports.OperationReviewAuthorizationFactV4{}, err
	}
	key := operationReviewAuthorizationEffectKeyV4ForShared(fact)
	owner := "v4\x00" + fact.ID
	if occupied, ok := s.active[key]; ok && occupied != owner {
		return ports.OperationReviewAuthorizationFactV4{}, core.NewError(core.ErrorConflict, core.ReasonEffectConflictDomainOccupied, "another Review Authorization version is current for this Effect")
	}
	if s.failCreateV4 {
		s.failCreateV4 = false
		return ports.OperationReviewAuthorizationFactV4{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected shared V4 create pre-commit failure")
	}
	created, err := s.v4.CreateOperationReviewAuthorizationV4(ctx, fact)
	if err == nil {
		s.active[key] = owner
		return created, nil
	}
	if sharedReviewAuthorizationUnknownV45(err) {
		recovered, inspectErr := s.v4.InspectOperationReviewAuthorizationV4(context.WithoutCancel(ctx), fact.ID)
		if inspectErr == nil && recovered.Digest == fact.Digest && recovered.State == ports.OperationReviewAuthorizationActiveV4 {
			s.active[key] = owner
		}
	}
	return created, err
}

func (s *OperationReviewAuthorizationSharedStoreV45) InspectOperationReviewAuthorizationV4(ctx context.Context, id string) (ports.OperationReviewAuthorizationFactV4, error) {
	return s.v4.InspectOperationReviewAuthorizationV4(ctx, id)
}

func (s *OperationReviewAuthorizationSharedStoreV45) CompareAndSwapOperationReviewAuthorizationV4(ctx context.Context, request ports.OperationReviewAuthorizationCASRequestV4) (ports.OperationReviewAuthorizationFactV4, error) {
	if ctx == nil {
		return ports.OperationReviewAuthorizationFactV4{}, sharedReviewAuthorizationContextErrorV45()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	updated, err := s.v4.CompareAndSwapOperationReviewAuthorizationV4(ctx, request)
	if err == nil && updated.State != ports.OperationReviewAuthorizationActiveV4 {
		s.deleteActiveV4(updated)
		return updated, nil
	}
	if sharedReviewAuthorizationUnknownV45(err) {
		recovered, inspectErr := s.v4.InspectOperationReviewAuthorizationV4(context.WithoutCancel(ctx), request.Next.ID)
		if inspectErr == nil && recovered.Digest == request.Next.Digest && recovered.State != ports.OperationReviewAuthorizationActiveV4 {
			s.deleteActiveV4(recovered)
		}
	}
	return updated, err
}

func (s *OperationReviewAuthorizationSharedStoreV45) CreateOperationReviewAuthorizationV5(ctx context.Context, fact ports.OperationReviewAuthorizationFactV5) (ports.OperationReviewAuthorizationFactV5, error) {
	if ctx == nil {
		return ports.OperationReviewAuthorizationFactV5{}, sharedReviewAuthorizationContextErrorV45()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.v5.InspectOperationReviewAuthorizationV5(ctx, fact.ID); err == nil {
		return s.v5.CreateOperationReviewAuthorizationV5(ctx, fact)
	} else if !core.HasCategory(err, core.ErrorNotFound) {
		return ports.OperationReviewAuthorizationFactV5{}, err
	}
	key := operationReviewAuthorizationEffectKeyV5(fact)
	owner := "v5\x00" + fact.ID
	if occupied, ok := s.active[key]; ok && occupied != owner {
		return ports.OperationReviewAuthorizationFactV5{}, core.NewError(core.ErrorConflict, core.ReasonEffectConflictDomainOccupied, "another Review Authorization version is current for this Effect")
	}
	if s.failCreateV5 {
		s.failCreateV5 = false
		return ports.OperationReviewAuthorizationFactV5{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected shared V5 create pre-commit failure")
	}
	created, err := s.v5.CreateOperationReviewAuthorizationV5(ctx, fact)
	if err == nil {
		s.active[key] = owner
		return created, nil
	}
	if sharedReviewAuthorizationUnknownV45(err) {
		recovered, inspectErr := s.v5.InspectOperationReviewAuthorizationExactV5(context.WithoutCancel(ctx), fact.RefV5())
		if inspectErr == nil && recovered.Digest == fact.Digest && recovered.State == ports.OperationReviewAuthorizationActiveV5 {
			s.active[key] = owner
		}
	}
	return created, err
}

func (s *OperationReviewAuthorizationSharedStoreV45) InspectOperationReviewAuthorizationV5(ctx context.Context, id string) (ports.OperationReviewAuthorizationFactV5, error) {
	return s.v5.InspectOperationReviewAuthorizationV5(ctx, id)
}

func (s *OperationReviewAuthorizationSharedStoreV45) InspectOperationReviewAuthorizationExactV5(ctx context.Context, ref ports.OperationReviewAuthorizationRefV5) (ports.OperationReviewAuthorizationFactV5, error) {
	return s.v5.InspectOperationReviewAuthorizationExactV5(ctx, ref)
}

func (s *OperationReviewAuthorizationSharedStoreV45) CompareAndSwapOperationReviewAuthorizationV5(ctx context.Context, request ports.OperationReviewAuthorizationCASRequestV5) (ports.OperationReviewAuthorizationFactV5, error) {
	if ctx == nil {
		return ports.OperationReviewAuthorizationFactV5{}, sharedReviewAuthorizationContextErrorV45()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	updated, err := s.v5.CompareAndSwapOperationReviewAuthorizationV5(ctx, request)
	if err == nil && updated.State != ports.OperationReviewAuthorizationActiveV5 {
		s.deleteActiveV5(updated)
		return updated, nil
	}
	if sharedReviewAuthorizationUnknownV45(err) {
		recovered, inspectErr := s.v5.InspectOperationReviewAuthorizationExactV5(context.WithoutCancel(ctx), request.Next.RefV5())
		if inspectErr == nil && recovered.Digest == request.Next.Digest && recovered.State != ports.OperationReviewAuthorizationActiveV5 {
			s.deleteActiveV5(recovered)
		}
	}
	return updated, err
}

func (s *OperationReviewAuthorizationSharedStoreV45) deleteActiveV4(fact ports.OperationReviewAuthorizationFactV4) {
	key := operationReviewAuthorizationEffectKeyV4ForShared(fact)
	if s.active[key] == "v4\x00"+fact.ID {
		delete(s.active, key)
	}
}

func (s *OperationReviewAuthorizationSharedStoreV45) deleteActiveV5(fact ports.OperationReviewAuthorizationFactV5) {
	key := operationReviewAuthorizationEffectKeyV5(fact)
	if s.active[key] == "v5\x00"+fact.ID {
		delete(s.active, key)
	}
}

func sharedReviewAuthorizationUnknownV45(err error) bool {
	return core.HasCategory(err, core.ErrorUnavailable) || core.HasCategory(err, core.ErrorIndeterminate)
}

func sharedReviewAuthorizationContextErrorV45() error {
	return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Review Authorization shared store context is required")
}

func operationReviewAuthorizationEffectKeyV4ForShared(fact ports.OperationReviewAuthorizationFactV4) string {
	operationDigest, _ := fact.Intent.Operation.DigestV3()
	return string(fact.Intent.Operation.ExecutionScope.Identity.TenantID) + "\x00" + string(operationDigest) + "\x00" + string(fact.Intent.IntentID)
}

var _ ports.OperationReviewAuthorizationFactPortV4 = (*OperationReviewAuthorizationSharedStoreV45)(nil)
var _ ports.OperationReviewAuthorizationFactPortV5 = (*OperationReviewAuthorizationSharedStoreV45)(nil)
