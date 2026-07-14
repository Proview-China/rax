package fakes

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// EffectStoreV2 is a deterministic in-memory fact owner for conformance and
// recovery tests. It is not a production backend.
type EffectStoreV2 struct {
	mu                     sync.Mutex
	clock                  func() time.Time
	effects                map[core.EffectIntentID]control.EffectFactV2
	idempotency            map[string]core.EffectIntentID
	conflicts              map[string]core.EffectIntentID
	permits                map[string]control.DispatchPermitFactV2
	budgets                map[string]control.BudgetBindingFactV2
	runEffects             map[string]control.RunEffectIndexFactV2
	runEffectSegments      map[string]map[uint64]control.RunEffectSegmentFactV2
	indexedEffects         map[string]map[core.EffectIntentID]control.EffectFactV2
	indexedPermits         map[string]map[string]control.DispatchPermitFactV2
	runFacts               control.RunFactPort
	loseNextIssueReply     bool
	loseNextBeginReply     bool
	loseNextReceiptReply   bool
	loseNextPermitCASReply bool
	loseNextEffectCASReply bool
	loseNextRunEffectReply bool
}

func NewEffectStoreV2(clock func() time.Time) *EffectStoreV2 {
	if clock == nil {
		clock = time.Now
	}
	return &EffectStoreV2{clock: clock, effects: make(map[core.EffectIntentID]control.EffectFactV2), idempotency: make(map[string]core.EffectIntentID), conflicts: make(map[string]core.EffectIntentID), permits: make(map[string]control.DispatchPermitFactV2), budgets: make(map[string]control.BudgetBindingFactV2), runEffects: make(map[string]control.RunEffectIndexFactV2), runEffectSegments: make(map[string]map[uint64]control.RunEffectSegmentFactV2), indexedEffects: make(map[string]map[core.EffectIntentID]control.EffectFactV2), indexedPermits: make(map[string]map[string]control.DispatchPermitFactV2)}
}

// SetRunFacts binds the current Run reader used by the test Fact Owner. The
// production adapter must provide an equivalent current-state projection.
func (s *EffectStoreV2) SetRunFacts(reader control.RunFactPort) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runFacts = reader
}

func (s *EffectStoreV2) LoseNextRunEffectReply() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loseNextRunEffectReply = true
}

func (s *EffectStoreV2) SetClock(clock func() time.Time) {
	if clock == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clock = clock
}

func (s *EffectStoreV2) now() time.Time {
	s.mu.Lock()
	clock := s.clock
	s.mu.Unlock()
	return clock()
}

func (s *EffectStoreV2) LoseNextIssueReply() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loseNextIssueReply = true
}

func (s *EffectStoreV2) LoseNextBeginReply() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loseNextBeginReply = true
}

func (s *EffectStoreV2) LoseNextReceiptReply() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loseNextReceiptReply = true
}

func (s *EffectStoreV2) LoseNextPermitCASReply() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loseNextPermitCASReply = true
}

func (s *EffectStoreV2) LoseNextEffectCASReply() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loseNextEffectCASReply = true
}

func (s *EffectStoreV2) CreateEffect(ctx context.Context, fact control.EffectFactV2) (control.EffectFactV2, error) {
	if err := contextError(ctx); err != nil {
		return control.EffectFactV2{}, err
	}
	if err := fact.Validate(); err != nil {
		return control.EffectFactV2{}, err
	}
	if fact.State != control.EffectProposed || fact.Revision != 1 {
		return control.EffectFactV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonEffectStateConflict, "new effect must be proposed at revision one")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, exists := s.effects[fact.Intent.ID]; exists {
		if existing.IntentDigest == fact.IntentDigest {
			return cloneEffectFactV2(existing), nil
		}
		return control.EffectFactV2{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "effect id already exists with different intent")
	}
	idempotencyKey := effectIdempotencyKeyV2(fact)
	if existingID, exists := s.idempotency[idempotencyKey]; exists {
		existing := s.effects[existingID]
		if existing.IntentDigest == fact.IntentDigest {
			return cloneEffectFactV2(existing), nil
		}
		return control.EffectFactV2{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "idempotency scope and key already bind another payload")
	}
	s.effects[fact.Intent.ID] = cloneEffectFactV2(fact)
	s.idempotency[idempotencyKey] = fact.Intent.ID
	return cloneEffectFactV2(fact), nil
}

func (s *EffectStoreV2) InspectEffect(ctx context.Context, id core.EffectIntentID) (control.EffectFactV2, error) {
	if err := contextError(ctx); err != nil {
		return control.EffectFactV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	fact, exists := s.effects[id]
	if !exists {
		return control.EffectFactV2{}, core.NewError(core.ErrorNotFound, core.ReasonEffectIntentMissing, "effect fact does not exist")
	}
	return cloneEffectFactV2(fact), nil
}

func (s *EffectStoreV2) InspectEffectByIdempotency(ctx context.Context, scopeClass ports.EffectStableScopeClassV2, scopeDigest core.Digest, key string) (control.EffectFactV2, error) {
	if err := contextError(ctx); err != nil {
		return control.EffectFactV2{}, err
	}
	if err := scopeDigest.Validate(); err != nil {
		return control.EffectFactV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	id, exists := s.idempotency[fmt.Sprintf("%s|%s|%s", scopeClass, scopeDigest, key)]
	if !exists {
		return control.EffectFactV2{}, core.NewError(core.ErrorNotFound, core.ReasonEffectIntentMissing, "effect idempotency binding does not exist")
	}
	return cloneEffectFactV2(s.effects[id]), nil
}

func (s *EffectStoreV2) InspectConflictDomain(ctx context.Context, binding ports.ConflictDomainBindingV2) (control.EffectFactV2, error) {
	if err := contextError(ctx); err != nil {
		return control.EffectFactV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	id, exists := s.conflicts[effectConflictKeyV2(binding)]
	if !exists {
		return control.EffectFactV2{}, core.NewError(core.ErrorNotFound, core.ReasonEffectIntentMissing, "effect conflict domain is unoccupied")
	}
	fact, exists := s.effects[id]
	if !exists || !fact.ConflictDomainOccupied() {
		return control.EffectFactV2{}, core.NewError(core.ErrorInternal, core.ReasonEffectStateConflict, "effect conflict index drifted")
	}
	return cloneEffectFactV2(fact), nil
}

func (s *EffectStoreV2) CompareAndSwapEffect(ctx context.Context, request control.EffectFactCASRequestV2) (control.EffectFactV2, error) {
	if err := contextError(ctx); err != nil {
		return control.EffectFactV2{}, err
	}
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	current, exists := s.effects[request.Next.Intent.ID]
	if !exists {
		return control.EffectFactV2{}, core.NewError(core.ErrorNotFound, core.ReasonEffectIntentMissing, "effect fact does not exist")
	}
	if current.Revision != request.ExpectedRevision {
		if sameAuthoritativeSettlementV2(current, request.Next) {
			return cloneEffectFactV2(current), nil
		}
		return control.EffectFactV2{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "effect fact revision does not match CAS precondition")
	}
	transition := control.EffectTransitionContextV2{}
	if current.DispatchPermitID != "" {
		permit, permitExists := s.permits[current.DispatchPermitID]
		transition.PermitBegun = permitExists && permit.State == control.DispatchPermitBegun
		transition.DispatchReceiptMatched = permitExists && providerReceiptMatchesPermitV2(request.Next, permit, now)
	}
	transition.SettlementOwnerMatched = settlementOwnerMatchesBindingV2(request.Next)
	transition.UnknownInspectionSettled = unknownInspectionSettledV2(s.effects, request.Next)
	transition.CompensationSettled = compensationSettledV2(s.effects, request.Next)
	transition.ResidualInspectSettled = resolutionSettledV2(s.effects, request.Next, true)
	transition.CleanupEffectSettled = resolutionSettledV2(s.effects, request.Next, false)
	if err := control.ValidateEffectFactTransitionV2(current, request.Next, transition, now); err != nil {
		return control.EffectFactV2{}, err
	}
	if current.State == control.EffectProposed && request.Next.State == control.EffectAccepted {
		key := effectConflictKeyV2(current.Intent.ConflictDomain)
		if occupiedID, occupied := s.conflicts[key]; occupied && occupiedID != current.Intent.ID {
			return control.EffectFactV2{}, core.NewError(core.ErrorConflict, core.ReasonEffectConflictDomainOccupied, "effect conflict domain already has an active or unknown effect")
		}
		s.conflicts[key] = current.Intent.ID
	}
	next := cloneEffectFactV2(request.Next)
	s.effects[next.Intent.ID] = next
	if !next.ConflictDomainOccupied() {
		delete(s.conflicts, effectConflictKeyV2(next.Intent.ConflictDomain))
	}
	if s.loseNextEffectCASReply {
		s.loseNextEffectCASReply = false
		return control.EffectFactV2{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected effect CAS reply loss")
	}
	return cloneEffectFactV2(next), nil
}

func (s *EffectStoreV2) IssueDispatchPermit(ctx context.Context, request control.IssueDispatchPermitRequestV2) (control.IssueDispatchPermitResultV2, error) {
	if err := contextError(ctx); err != nil {
		return control.IssueDispatchPermitResultV2{}, err
	}
	if err := request.Permit.Validate(); err != nil {
		return control.IssueDispatchPermitResultV2{}, err
	}
	if err := request.Fence.Validate(); err != nil {
		return control.IssueDispatchPermitResultV2{}, err
	}
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, exists := s.permits[request.Permit.ID]; exists {
		permitDigest, _ := request.Permit.DigestV2()
		fenceDigest, _ := ports.DigestExecutionFenceV2(request.Fence)
		if existing.Permit.AttemptID == request.Permit.AttemptID && existing.Permit.IntentID == request.EffectID && existing.PermitDigest == permitDigest && existing.Permit.FenceDigest == fenceDigest && reflect.DeepEqual(existing.Fence, request.Fence) {
			return control.IssueDispatchPermitResultV2{Effect: cloneEffectFactV2(s.effects[request.EffectID]), Permit: clonePermitFactV2(existing)}, nil
		}
		return control.IssueDispatchPermitResultV2{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "permit id or attempt already binds different permit or fence content")
	}
	effect, exists := s.effects[request.EffectID]
	if !exists || effect.Revision != request.ExpectedEffectRevision || effect.State != control.EffectAccepted {
		return control.IssueDispatchPermitResultV2{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "permit issue requires the current accepted effect revision")
	}
	intentDigest, _ := effect.Intent.DigestV2()
	if request.Permit.IntentID != effect.Intent.ID || request.Permit.IntentRevision != effect.Intent.Revision || request.Permit.IntentDigest != intentDigest || request.Permit.PayloadSchema != effect.Intent.Payload.Schema || request.Permit.PayloadDigest != effect.Intent.Payload.ContentDigest || request.Permit.PayloadRevision != effect.Intent.PayloadRevision || request.Permit.RunID != effect.Intent.RunID || request.Permit.ConflictDomain != effect.Intent.ConflictDomain || request.Permit.Provider != effect.Intent.Provider || request.Permit.EnforcementPoint != effect.Intent.Provider || request.Permit.Authority != effect.Intent.Authority || request.Permit.Review != effect.Intent.Review || request.Permit.Budget != effect.Intent.Budget || request.Permit.Policy != effect.Intent.Policy || request.Permit.CurrentScope != effect.Intent.CurrentScope || request.Permit.Idempotency != effect.Intent.Idempotency || !sameScopeForEffectStoreV2(request.Permit.Scope, effect.Intent.Scope) {
		return control.IssueDispatchPermitResultV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonDispatchPermitInvalid, "permit does not bind the exact accepted effect")
	}
	if now.IsZero() || now.UnixNano() < request.Permit.IssuedUnixNano || !now.Before(time.Unix(0, request.Permit.ExpiresUnixNano)) {
		return control.IssueDispatchPermitResultV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonDispatchPermitExpired, "permit issue clock regressed or reached expiry")
	}
	permitDigest, _ := request.Permit.DigestV2()
	fenceDigest, err := ports.DigestExecutionFenceV2(request.Fence)
	if err != nil || fenceDigest != request.Permit.FenceDigest {
		return control.IssueDispatchPermitResultV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectFenceStale, "issued permit must bind the persisted gateway fence")
	}
	nextEffect := effect
	nextEffect.State = control.EffectDispatchIntent
	nextEffect.Revision++
	nextEffect.DispatchPermitID = request.Permit.ID
	nextEffect.DispatchPermitDigest = permitDigest
	nextEffect.UpdatedUnixNano = now.UnixNano()
	if err := control.ValidateEffectFactTransitionV2(effect, nextEffect, control.EffectTransitionContextV2{}, now); err != nil {
		return control.IssueDispatchPermitResultV2{}, err
	}
	permitFact := control.DispatchPermitFactV2{Permit: request.Permit, PermitDigest: permitDigest, Fence: request.Fence, State: control.DispatchPermitIssued, Revision: 1, EffectFactRevision: nextEffect.Revision}
	if err := permitFact.Validate(); err != nil {
		return control.IssueDispatchPermitResultV2{}, err
	}
	s.effects[effect.Intent.ID] = cloneEffectFactV2(nextEffect)
	s.permits[request.Permit.ID] = clonePermitFactV2(permitFact)
	result := control.IssueDispatchPermitResultV2{Effect: cloneEffectFactV2(nextEffect), Permit: clonePermitFactV2(permitFact)}
	if s.loseNextIssueReply {
		s.loseNextIssueReply = false
		return control.IssueDispatchPermitResultV2{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected permit issue reply loss")
	}
	return result, nil
}

func (s *EffectStoreV2) InspectDispatchPermit(ctx context.Context, id string) (control.DispatchPermitFactV2, error) {
	if err := contextError(ctx); err != nil {
		return control.DispatchPermitFactV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	fact, exists := s.permits[id]
	if !exists {
		return control.DispatchPermitFactV2{}, core.NewError(core.ErrorNotFound, core.ReasonDispatchPermitInvalid, "dispatch permit fact does not exist")
	}
	return clonePermitFactV2(fact), nil
}

func (s *EffectStoreV2) BeginDispatch(ctx context.Context, request control.BeginDispatchRequestV2) (control.DispatchPermitFactV2, error) {
	if err := contextError(ctx); err != nil {
		return control.DispatchPermitFactV2{}, err
	}
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	effect, effectExists := s.effects[request.EffectID]
	permit, permitExists := s.permits[request.PermitID]
	if !effectExists || !permitExists || effect.State != control.EffectDispatchIntent || effect.Revision != request.ExpectedEffectRevision || effect.DispatchPermitID != request.PermitID || permit.Revision != request.ExpectedPermitRevision || permit.EffectFactRevision != effect.Revision {
		return control.DispatchPermitFactV2{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "begin dispatch requires current effect and issued permit revisions")
	}
	next := permit
	next.State = control.DispatchPermitBegun
	next.Revision++
	next.BegunUnixNano = now.UnixNano()
	if err := control.ValidateDispatchPermitTransitionV2(permit, next, now); err != nil {
		return control.DispatchPermitFactV2{}, err
	}
	s.permits[permit.Permit.ID] = clonePermitFactV2(next)
	if s.loseNextBeginReply {
		s.loseNextBeginReply = false
		return control.DispatchPermitFactV2{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected begin dispatch reply loss")
	}
	return clonePermitFactV2(next), nil
}

func (s *EffectStoreV2) RecordEnforcementReceipt(ctx context.Context, request control.RecordEnforcementReceiptRequestV2) (control.DispatchPermitFactV2, error) {
	if err := contextError(ctx); err != nil {
		return control.DispatchPermitFactV2{}, err
	}
	if err := request.Receipt.Validate(); err != nil {
		return control.DispatchPermitFactV2{}, err
	}
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	current, exists := s.permits[request.PermitID]
	if !exists {
		return control.DispatchPermitFactV2{}, core.NewError(core.ErrorNotFound, core.ReasonDispatchPermitInvalid, "dispatch permit fact does not exist")
	}
	if current.Enforcement != nil {
		if reflect.DeepEqual(*current.Enforcement, request.Receipt) {
			return clonePermitFactV2(current), nil
		}
		return control.DispatchPermitFactV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "permit already records different enforcement evidence")
	}
	if current.Revision != request.ExpectedPermitRevision {
		return control.DispatchPermitFactV2{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "permit fact revision does not match receipt CAS precondition")
	}
	next := current
	next.Revision++
	next.Enforcement = &request.Receipt
	if err := control.ValidateDispatchPermitTransitionV2(current, next, now); err != nil {
		return control.DispatchPermitFactV2{}, err
	}
	s.permits[request.PermitID] = clonePermitFactV2(next)
	if s.loseNextReceiptReply {
		s.loseNextReceiptReply = false
		return control.DispatchPermitFactV2{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected enforcement receipt reply loss")
	}
	return clonePermitFactV2(next), nil
}

func (s *EffectStoreV2) CompareAndSwapDispatchPermit(ctx context.Context, request control.DispatchPermitFactCASRequestV2) (control.DispatchPermitFactV2, error) {
	if err := contextError(ctx); err != nil {
		return control.DispatchPermitFactV2{}, err
	}
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	current, exists := s.permits[request.PermitID]
	if !exists {
		return control.DispatchPermitFactV2{}, core.NewError(core.ErrorNotFound, core.ReasonDispatchPermitInvalid, "dispatch permit fact does not exist")
	}
	if current.Revision != request.ExpectedRevision {
		if reflect.DeepEqual(current, request.Next) {
			return clonePermitFactV2(current), nil
		}
		return control.DispatchPermitFactV2{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "permit fact revision does not match CAS precondition")
	}
	if err := control.ValidateDispatchPermitTransitionV2(current, request.Next, now); err != nil {
		return control.DispatchPermitFactV2{}, err
	}
	s.permits[request.PermitID] = clonePermitFactV2(request.Next)
	if s.loseNextPermitCASReply {
		s.loseNextPermitCASReply = false
		return control.DispatchPermitFactV2{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected permit CAS reply loss")
	}
	return clonePermitFactV2(request.Next), nil
}

func (s *EffectStoreV2) CreateBudgetBinding(ctx context.Context, fact control.BudgetBindingFactV2) (control.BudgetBindingFactV2, error) {
	if err := contextError(ctx); err != nil {
		return control.BudgetBindingFactV2{}, err
	}
	if err := fact.Validate(); err != nil {
		return control.BudgetBindingFactV2{}, err
	}
	if fact.State != control.BudgetFactActive || fact.Revision != 1 {
		return control.BudgetBindingFactV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonBudgetBindingMissing, "new budget binding must be active at revision one")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, exists := s.budgets[fact.Ref]; exists {
		if reflect.DeepEqual(existing, fact) {
			return existing, nil
		}
		return control.BudgetBindingFactV2{}, core.NewError(core.ErrorConflict, core.ReasonAlreadyExists, "budget binding ref already exists")
	}
	s.budgets[fact.Ref] = fact
	return fact, nil
}

func (s *EffectStoreV2) InspectBudgetBinding(ctx context.Context, ref string) (control.BudgetBindingFactV2, error) {
	if err := contextError(ctx); err != nil {
		return control.BudgetBindingFactV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	fact, exists := s.budgets[ref]
	if !exists {
		return control.BudgetBindingFactV2{}, core.NewError(core.ErrorNotFound, core.ReasonBudgetBindingMissing, "budget binding fact does not exist")
	}
	return fact, nil
}

func (s *EffectStoreV2) CompareAndSwapBudgetBinding(ctx context.Context, request control.BudgetFactCASRequestV2) (control.BudgetBindingFactV2, error) {
	if err := contextError(ctx); err != nil {
		return control.BudgetBindingFactV2{}, err
	}
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	current, exists := s.budgets[request.Next.Ref]
	if !exists || current.Revision != request.ExpectedRevision {
		return control.BudgetBindingFactV2{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "budget fact revision does not match CAS precondition")
	}
	if err := control.ValidateBudgetFactTransitionV2(current, request.Next, now); err != nil {
		return control.BudgetBindingFactV2{}, err
	}
	s.budgets[request.Next.Ref] = request.Next
	return request.Next, nil
}

func effectIdempotencyKeyV2(fact control.EffectFactV2) string {
	return fmt.Sprintf("%s|%s|%s", fact.Intent.Idempotency.ScopeClass, fact.Intent.Idempotency.ScopeDigest, fact.Intent.Idempotency.Key)
}

func effectConflictKeyV2(binding ports.ConflictDomainBindingV2) string {
	return fmt.Sprintf("%s|%s|%s", binding.ScopeClass, binding.ScopeDigest, binding.Domain)
}

func settlementOwnerMatchesBindingV2(next control.EffectFactV2) bool {
	if next.Settlement == nil {
		return false
	}
	for _, owner := range next.Intent.Owners {
		if owner.Role == ports.OwnerSettlement {
			return owner == next.Settlement.Owner
		}
	}
	return false
}

func compensationSettledV2(effects map[core.EffectIntentID]control.EffectFactV2, next control.EffectFactV2) bool {
	if next.Compensation == nil {
		return false
	}
	compensation, exists := effects[next.Compensation.EffectID]
	if !exists || compensation.Intent.Revision != next.Compensation.EffectRevision || compensation.State != control.EffectSettled || compensation.Settlement == nil {
		return false
	}
	if compensation.Intent.Relation.CompensatesEffectID != next.Intent.ID || compensation.Intent.Relation.CompensatesEffectRevision != next.Intent.Revision || compensation.Intent.Scope.Identity.TenantID != next.Intent.Scope.Identity.TenantID {
		return false
	}
	digest, err := core.CanonicalJSONDigest("praxis.runtime.effect", ports.EffectContractVersionV2, "EffectSettlementFactV2", *compensation.Settlement)
	return err == nil && digest == next.Compensation.SettlementDigest
}

func providerReceiptMatchesPermitV2(next control.EffectFactV2, permit control.DispatchPermitFactV2, now time.Time) bool {
	receipt := next.DispatchReceipt
	enforcement := permit.Enforcement
	return receipt != nil && permit.State == control.DispatchPermitBegun && enforcement != nil &&
		enforcement.PermitID == permit.Permit.ID && enforcement.PermitRevision == permit.Permit.Revision && enforcement.AttemptID == permit.Permit.AttemptID && enforcement.PermitDigest == permit.PermitDigest &&
		receipt.PermitID == permit.Permit.ID && receipt.PermitDigest == permit.PermitDigest && receipt.AttemptID == permit.Permit.AttemptID && receipt.IntentID == permit.Permit.IntentID && receipt.IntentRevision == permit.Permit.IntentRevision && receipt.Provider == permit.Permit.Provider &&
		receipt.ObservedUnixNano >= permit.BegunUnixNano && receipt.ObservedUnixNano >= enforcement.ValidatedAt && receipt.ObservedUnixNano <= now.UnixNano()
}

func unknownInspectionSettledV2(effects map[core.EffectIntentID]control.EffectFactV2, next control.EffectFactV2) bool {
	if next.Settlement == nil || next.Settlement.InspectionIntentID == "" || next.Settlement.InspectionIntentRevision == 0 {
		return false
	}
	inspection, exists := effects[next.Settlement.InspectionIntentID]
	if !exists || inspection.Intent.Revision != next.Settlement.InspectionIntentRevision || inspection.State != control.EffectSettled || inspection.Settlement == nil || inspection.Intent.Relation.InspectsEffectID != next.Intent.ID || inspection.Intent.Relation.InspectsEffectRevision != next.Intent.Revision || inspection.Intent.Scope.Identity.TenantID != next.Intent.Scope.Identity.TenantID {
		return false
	}
	digest, err := core.CanonicalJSONDigest("praxis.runtime.effect", ports.EffectContractVersionV2, "EffectSettlementFactV2", *inspection.Settlement)
	return err == nil && digest == next.Settlement.InspectionSettlementDigest
}

func resolutionSettledV2(effects map[core.EffectIntentID]control.EffectFactV2, next control.EffectFactV2, inspection bool) bool {
	resolution := next.CleanupResolution
	if inspection {
		resolution = next.ResidualResolution
	}
	if resolution == nil {
		return false
	}
	effect, exists := effects[resolution.EffectID]
	if !exists || effect.Intent.Revision != resolution.EffectRevision || effect.State != control.EffectSettled || effect.Settlement == nil {
		return false
	}
	if inspection {
		if effect.Intent.Relation.InspectsEffectID != next.Intent.ID || effect.Intent.Relation.InspectsEffectRevision != next.Intent.Revision {
			return false
		}
	} else if effect.Intent.Relation.CleansUpEffectID != next.Intent.ID || effect.Intent.Relation.CleansUpEffectRevision != next.Intent.Revision {
		return false
	}
	digest, err := core.CanonicalJSONDigest("praxis.runtime.effect", ports.EffectContractVersionV2, "EffectSettlementFactV2", *effect.Settlement)
	return err == nil && digest == resolution.SettlementDigest
}

func sameAuthoritativeSettlementV2(current, proposed control.EffectFactV2) bool {
	return current.Intent.ID == proposed.Intent.ID && reflect.DeepEqual(current, proposed)
}

func cloneEffectFactV2(fact control.EffectFactV2) control.EffectFactV2 {
	if fact.Intent.Scope.SandboxLease != nil {
		lease := *fact.Intent.Scope.SandboxLease
		fact.Intent.Scope.SandboxLease = &lease
	}
	fact.Intent.Owners = append([]ports.EffectOwnerRefV2(nil), fact.Intent.Owners...)
	fact.Intent.CredentialLeases = append([]ports.CredentialLeaseRefV2(nil), fact.Intent.CredentialLeases...)
	fact.Intent.Payload.Inline = append([]byte(nil), fact.Intent.Payload.Inline...)
	if fact.DispatchReceipt != nil {
		copy := *fact.DispatchReceipt
		fact.DispatchReceipt = &copy
	}
	if fact.Settlement != nil {
		copy := *fact.Settlement
		fact.Settlement = &copy
	}
	if fact.Compensation != nil {
		copy := *fact.Compensation
		fact.Compensation = &copy
	}
	if fact.ResidualResolution != nil {
		copy := *fact.ResidualResolution
		fact.ResidualResolution = &copy
	}
	if fact.CleanupResolution != nil {
		copy := *fact.CleanupResolution
		fact.CleanupResolution = &copy
	}
	return fact
}

func clonePermitFactV2(fact control.DispatchPermitFactV2) control.DispatchPermitFactV2 {
	if fact.Permit.Scope.SandboxLease != nil {
		lease := *fact.Permit.Scope.SandboxLease
		fact.Permit.Scope.SandboxLease = &lease
	}
	if fact.Fence.Scope.SandboxLease != nil {
		lease := *fact.Fence.Scope.SandboxLease
		fact.Fence.Scope.SandboxLease = &lease
	}
	if fact.Enforcement != nil {
		copy := *fact.Enforcement
		if fact.Enforcement.Attestation != nil {
			attestation := *fact.Enforcement.Attestation
			attestation.Payload.Inline = append([]byte(nil), attestation.Payload.Inline...)
			copy.Attestation = &attestation
		}
		fact.Enforcement = &copy
	}
	return fact
}

func sameScopeForEffectStoreV2(left, right core.ExecutionScope) bool {
	if left.Identity != right.Identity || left.Lineage != right.Lineage || left.Instance != right.Instance || left.AuthorityEpoch != right.AuthorityEpoch {
		return false
	}
	if left.SandboxLease == nil || right.SandboxLease == nil {
		return left.SandboxLease == nil && right.SandboxLease == nil
	}
	return *left.SandboxLease == *right.SandboxLease
}
