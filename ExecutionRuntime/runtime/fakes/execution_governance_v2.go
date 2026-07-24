package fakes

import (
	"context"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// OperationEffectStoreV3 is a deterministic in-memory Fact Owner used only by
// tests and conformance fixtures. It makes no production durability or SLA
// claim and is not an Application governance gateway.
type OperationEffectStoreV3 struct {
	mu                         sync.Mutex
	clock                      func() time.Time
	effects                    map[string]map[core.EffectIntentID]control.OperationEffectFactV3
	permits                    map[string]map[string]control.OperationDispatchPermitFactV3
	permitsV4                  map[string]map[string]control.OperationDispatchPermitFactV4
	permitsV5                  map[string]map[string]control.OperationDispatchPermitFactV5
	enforcementV4              map[string]map[string]ports.OperationDispatchEnforcementJournalV4
	enforcementV5              map[string]map[string]ports.OperationDispatchEnforcementJournalV5
	settlementsV4              map[string]map[string]ports.OperationSettlementCommitBundleV4
	settlementsV4ByEffect      map[string]ports.OperationSettlementCommitBundleV4
	settlementsV4ByID          map[string]ports.OperationSettlementCommitBundleV4
	settlementsV5ByID          map[string]ports.OperationCheckpointRestoreSettlementCommitBundleV5
	settlementsV5ByEffect      map[string]ports.OperationCheckpointRestoreSettlementCommitBundleV5
	terminalEffectsV5          map[string]ports.OperationCheckpointRestoreEffectTerminalV5
	settlementTerminalGuards   map[string]operationSettlementTerminalGuardOwner
	reviewAuthorizationsV4     ports.OperationReviewAuthorizationFactPortV4
	reviewAuthorizationsV5     ports.OperationReviewAuthorizationGovernancePortV5
	issueV4CommitCount         uint64
	beginV4CommitCount         uint64
	issueV5CommitCount         uint64
	beginV5CommitCount         uint64
	loseNextCreateReply        bool
	loseNextCASReply           bool
	loseNextIssueReply         bool
	loseNextBeginReply         bool
	loseNextIssueV5Reply       bool
	loseNextBeginV5Reply       bool
	loseNextEnforcementReply   bool
	loseNextEnforcementV4Reply bool
	loseNextEnforcementV5Reply bool
	enforcementV4CommitCount   uint64
	enforcementV5CommitCount   uint64
	settlementV4CommitCount    uint64
	loseNextSettlementV4Reply  bool
	failNextSettlementV4Stage  int
	settlementV5CommitCount    uint64
	loseNextSettlementV5Reply  bool
	failNextSettlementV5Stage  int
}

func NewOperationEffectStoreV3(clock func() time.Time) *OperationEffectStoreV3 {
	if clock == nil {
		clock = time.Now
	}
	return &OperationEffectStoreV3{
		clock:                    clock,
		effects:                  map[string]map[core.EffectIntentID]control.OperationEffectFactV3{},
		permits:                  map[string]map[string]control.OperationDispatchPermitFactV3{},
		permitsV4:                map[string]map[string]control.OperationDispatchPermitFactV4{},
		permitsV5:                map[string]map[string]control.OperationDispatchPermitFactV5{},
		enforcementV4:            map[string]map[string]ports.OperationDispatchEnforcementJournalV4{},
		enforcementV5:            map[string]map[string]ports.OperationDispatchEnforcementJournalV5{},
		settlementsV4:            map[string]map[string]ports.OperationSettlementCommitBundleV4{},
		settlementsV4ByEffect:    map[string]ports.OperationSettlementCommitBundleV4{},
		settlementsV4ByID:        map[string]ports.OperationSettlementCommitBundleV4{},
		settlementsV5ByID:        map[string]ports.OperationCheckpointRestoreSettlementCommitBundleV5{},
		settlementsV5ByEffect:    map[string]ports.OperationCheckpointRestoreSettlementCommitBundleV5{},
		terminalEffectsV5:        map[string]ports.OperationCheckpointRestoreEffectTerminalV5{},
		settlementTerminalGuards: map[string]operationSettlementTerminalGuardOwner{},
	}
}

func (s *OperationEffectStoreV3) BindOperationReviewAuthorizationFactsV4(facts ports.OperationReviewAuthorizationFactPortV4) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reviewAuthorizationsV4 = facts
}

func (s *OperationEffectStoreV3) IssueV4CommitCount() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.issueV4CommitCount
}

func (s *OperationEffectStoreV3) BeginV4CommitCount() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.beginV4CommitCount
}

func (s *OperationEffectStoreV3) LoseNextCreateReply() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loseNextCreateReply = true
}

func (s *OperationEffectStoreV3) LoseNextIssueReply() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loseNextIssueReply = true
}

func (s *OperationEffectStoreV3) LoseNextCASReply() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loseNextCASReply = true
}

func (s *OperationEffectStoreV3) LoseNextBeginReply() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loseNextBeginReply = true
}

func (s *OperationEffectStoreV3) LoseNextEnforcementReply() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loseNextEnforcementReply = true
}

func (s *OperationEffectStoreV3) LoseNextEnforcementV4Reply() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loseNextEnforcementV4Reply = true
}

func (s *OperationEffectStoreV3) EnforcementV4CommitCount() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.enforcementV4CommitCount
}

func operationKeyV3(subject ports.OperationSubjectV3) (string, error) {
	digest, err := subject.DigestV3()
	return string(digest), err
}

func (s *OperationEffectStoreV3) CreateOperationEffectV3(ctx context.Context, fact control.OperationEffectFactV3) (control.OperationEffectFactV3, error) {
	if err := contextError(ctx); err != nil {
		return control.OperationEffectFactV3{}, err
	}
	if err := fact.Validate(); err != nil {
		return control.OperationEffectFactV3{}, err
	}
	if fact.State != control.OperationEffectProposedV3 || fact.Revision != 1 {
		return control.OperationEffectFactV3{}, core.NewError(core.ErrorInvalidArgument, core.ReasonEffectStateConflict, "new operation Effect must be proposed revision one")
	}
	key, err := operationKeyV3(fact.Intent.Operation)
	if err != nil {
		return control.OperationEffectFactV3{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.effects[key][fact.Intent.ID]; ok {
		if existing.IntentDigest == fact.IntentDigest {
			return cloneOperationEffectFactV3(existing), nil
		}
		return control.OperationEffectFactV3{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "operation Effect ID binds different content")
	}
	if s.effects[key] == nil {
		s.effects[key] = map[core.EffectIntentID]control.OperationEffectFactV3{}
	}
	s.effects[key][fact.Intent.ID] = cloneOperationEffectFactV3(fact)
	if s.loseNextCreateReply {
		s.loseNextCreateReply = false
		return control.OperationEffectFactV3{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected operation Effect create reply loss")
	}
	return cloneOperationEffectFactV3(fact), nil
}

func (s *OperationEffectStoreV3) InspectOperationEffectV3(ctx context.Context, subject ports.OperationSubjectV3, effectID core.EffectIntentID) (control.OperationEffectFactV3, error) {
	if err := contextError(ctx); err != nil {
		return control.OperationEffectFactV3{}, err
	}
	key, err := operationKeyV3(subject)
	if err != nil {
		return control.OperationEffectFactV3{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	fact, ok := s.effects[key][effectID]
	if !ok {
		return control.OperationEffectFactV3{}, core.NewError(core.ErrorNotFound, core.ReasonEffectIntentMissing, "operation Effect not found in exact operation partition")
	}
	return cloneOperationEffectFactV3(fact), nil
}

func (s *OperationEffectStoreV3) CompareAndSwapOperationEffectV3(ctx context.Context, subject ports.OperationSubjectV3, request control.OperationEffectCASRequestV3) (control.OperationEffectFactV3, error) {
	if err := contextError(ctx); err != nil {
		return control.OperationEffectFactV3{}, err
	}
	key, err := operationKeyV3(subject)
	if err != nil {
		return control.OperationEffectFactV3{}, err
	}
	now := s.clock()
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.effects[key][request.Next.Intent.ID]
	if !ok {
		return control.OperationEffectFactV3{}, core.NewError(core.ErrorNotFound, core.ReasonEffectIntentMissing, "operation Effect not found")
	}
	if current.Revision != request.ExpectedRevision {
		currentDigest, currentErr := operationEffectFactDigestV3(current)
		nextDigest, nextErr := operationEffectFactDigestV3(request.Next)
		if currentErr == nil && nextErr == nil && currentDigest == nextDigest {
			return cloneOperationEffectFactV3(current), nil
		}
		return control.OperationEffectFactV3{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "operation Effect CAS revision or canonical content conflicts")
	}
	transition := control.OperationEffectTransitionContextV3{}
	if current.State == control.OperationEffectDispatchIntentV3 && request.Next.State == control.OperationEffectRejectedV3 {
		permit := s.permits[key][current.DispatchPermitID]
		transition.PreDispatchRejectionSafe = permit.State == control.OperationPermitIssuedV3 || permit.State == control.OperationPermitExpiredV3 || permit.State == control.OperationPermitRevokedV3
	}
	if request.Next.DispatchReceipt != nil {
		permit := s.permits[key][request.Next.DispatchPermitID]
		transition.PermitBegun = permit.State == control.OperationPermitBegunV3 && permit.Enforcement != nil
		transition.DispatchReceiptMatched = transition.PermitBegun && operationDispatchReceiptMatchesV3(*request.Next.DispatchReceipt, permit)
	}
	if request.Next.Settlement != nil {
		tenantID := current.Intent.Operation.ExecutionScope.Identity.TenantID
		guardKey := operationSettlementGuardKeyV4(tenantID, current.Intent.ID)
		if _, exists := s.settlementsV4ByID[operationSettlementIDKeyV4(tenantID, request.Next.Settlement.ID)]; exists {
			return control.OperationEffectFactV3{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "settlement ID already belongs to V4")
		}
		if _, exists := s.settlementTerminalGuards[guardKey]; exists {
			return control.OperationEffectFactV3{}, core.NewError(core.ErrorConflict, core.ReasonEffectStateConflict, "shared terminal guard already occupies this operation Effect")
		}
		for _, owner := range current.Intent.Owners {
			if owner.Role == ports.OwnerSettlement && owner == request.Next.Settlement.Owner {
				transition.SettlementOwnerMatched = true
			}
		}
	}
	if err := control.ValidateOperationEffectTransitionV3(current, request.Next, transition, now); err != nil {
		return control.OperationEffectFactV3{}, err
	}
	if request.Next.Settlement != nil {
		tenantID := current.Intent.Operation.ExecutionScope.Identity.TenantID
		operationDigest, digestErr := current.Intent.Operation.DigestV3()
		if digestErr != nil {
			return control.OperationEffectFactV3{}, digestErr
		}
		s.settlementTerminalGuards[operationSettlementGuardKeyV4(tenantID, current.Intent.ID)] = operationSettlementTerminalGuardOwner{
			Version:         operationSettlementTerminalVersionV3,
			SettlementID:    request.Next.Settlement.ID,
			OperationDigest: operationDigest,
		}
	}
	s.effects[key][request.Next.Intent.ID] = cloneOperationEffectFactV3(request.Next)
	if s.loseNextCASReply {
		s.loseNextCASReply = false
		return control.OperationEffectFactV3{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected operation Effect CAS reply loss")
	}
	return cloneOperationEffectFactV3(request.Next), nil
}

func (s *OperationEffectStoreV3) IssueOperationDispatchPermitV3(ctx context.Context, request control.IssueOperationPermitRequestV3) (control.IssueOperationPermitResultV3, error) {
	if err := contextError(ctx); err != nil {
		return control.IssueOperationPermitResultV3{}, err
	}
	key, err := operationKeyV3(request.Operation)
	if err != nil {
		return control.IssueOperationPermitResultV3{}, err
	}
	permitDigest, err := request.Permit.DigestV3()
	if err != nil {
		return control.IssueOperationPermitResultV3{}, err
	}
	fenceDigest, err := ports.DigestOperationExecutionFenceV3(request.Fence, request.Operation)
	if err != nil {
		return control.IssueOperationPermitResultV3{}, err
	}
	now := s.clock()
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.permitsV4[key][request.Permit.ID]; exists {
		return control.IssueOperationPermitResultV3{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "operation Permit ID already belongs to V4")
	}
	if existing, ok := s.permits[key][request.Permit.ID]; ok {
		if existing.PermitDigest == permitDigest && existing.Fence == request.Fence {
			return control.IssueOperationPermitResultV3{Effect: cloneOperationEffectFactV3(s.effects[key][request.EffectID]), Permit: cloneOperationPermitFactV3(existing)}, nil
		}
		return control.IssueOperationPermitResultV3{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "operation Permit ID binds different content")
	}
	effect, ok := s.effects[key][request.EffectID]
	if !ok {
		return control.IssueOperationPermitResultV3{}, core.NewError(core.ErrorNotFound, core.ReasonEffectIntentMissing, "operation Effect not found")
	}
	if effect.State != control.OperationEffectAcceptedV3 || effect.Revision != request.ExpectedEffectRevision {
		return control.IssueOperationPermitResultV3{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "operation Effect is not accepted at expected revision")
	}
	intentDigest, _ := effect.Intent.DigestV3()
	if request.Permit.IntentID != effect.Intent.ID || request.Permit.IntentRevision != effect.Intent.Revision || request.Permit.IntentDigest != intentDigest || !ports.SameOperationSubjectV3(request.Permit.Operation, effect.Intent.Operation) || request.Permit.PayloadSchema != effect.Intent.Payload.Schema || request.Permit.PayloadDigest != effect.Intent.Payload.ContentDigest || request.Permit.PayloadRevision != effect.Intent.PayloadRevision || request.Permit.ConflictDomain != effect.Intent.ConflictDomain || request.Permit.Provider != effect.Intent.Provider || request.Permit.Authority != effect.Intent.Authority || request.Permit.Review != effect.Intent.Review || request.Permit.Budget != effect.Intent.Budget || request.Permit.Policy != effect.Intent.Policy || request.Permit.Idempotency != effect.Intent.Idempotency || request.Permit.FenceDigest != fenceDigest || now.IsZero() || !now.Before(time.Unix(0, request.Permit.ExpiresUnixNano)) {
		return control.IssueOperationPermitResultV3{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonDispatchPermitInvalid, "operation Permit does not bind exact Effect and governance projection")
	}
	nextEffect := cloneOperationEffectFactV3(effect)
	nextEffect.State = control.OperationEffectDispatchIntentV3
	nextEffect.Revision++
	nextEffect.DispatchPermitID = request.Permit.ID
	nextEffect.DispatchPermitDigest = permitDigest
	nextEffect.UpdatedUnixNano = now.UnixNano()
	permitFact := control.OperationDispatchPermitFactV3{Permit: request.Permit, PermitDigest: permitDigest, Fence: request.Fence, State: control.OperationPermitIssuedV3, Revision: 1, EffectFactRevision: nextEffect.Revision}
	if err := nextEffect.Validate(); err != nil {
		return control.IssueOperationPermitResultV3{}, err
	}
	if err := permitFact.Validate(); err != nil {
		return control.IssueOperationPermitResultV3{}, err
	}
	if s.permits[key] == nil {
		s.permits[key] = map[string]control.OperationDispatchPermitFactV3{}
	}
	s.effects[key][request.EffectID] = cloneOperationEffectFactV3(nextEffect)
	s.permits[key][request.Permit.ID] = cloneOperationPermitFactV3(permitFact)
	if s.loseNextIssueReply {
		s.loseNextIssueReply = false
		return control.IssueOperationPermitResultV3{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected operation Permit issue reply loss")
	}
	return control.IssueOperationPermitResultV3{Effect: cloneOperationEffectFactV3(nextEffect), Permit: cloneOperationPermitFactV3(permitFact)}, nil
}

func (s *OperationEffectStoreV3) InspectOperationDispatchPermitV3(ctx context.Context, subject ports.OperationSubjectV3, permitID string) (control.OperationDispatchPermitFactV3, error) {
	if err := contextError(ctx); err != nil {
		return control.OperationDispatchPermitFactV3{}, err
	}
	key, err := operationKeyV3(subject)
	if err != nil {
		return control.OperationDispatchPermitFactV3{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	fact, ok := s.permits[key][permitID]
	if !ok {
		return control.OperationDispatchPermitFactV3{}, core.NewError(core.ErrorNotFound, core.ReasonDispatchPermitInvalid, "operation Permit not found")
	}
	return cloneOperationPermitFactV3(fact), nil
}

func (s *OperationEffectStoreV3) BeginOperationDispatchV3(ctx context.Context, request control.BeginOperationDispatchRequestV3) (control.OperationDispatchPermitFactV3, error) {
	if err := contextError(ctx); err != nil {
		return control.OperationDispatchPermitFactV3{}, err
	}
	key, err := operationKeyV3(request.Operation)
	if err != nil {
		return control.OperationDispatchPermitFactV3{}, err
	}
	now := s.clock()
	s.mu.Lock()
	defer s.mu.Unlock()
	permit, ok := s.permits[key][request.PermitID]
	if !ok {
		return control.OperationDispatchPermitFactV3{}, core.NewError(core.ErrorNotFound, core.ReasonDispatchPermitInvalid, "operation Permit not found")
	}
	if permit.State == control.OperationPermitBegunV3 {
		effect := s.effects[key][request.EffectID]
		if permit.Permit.IntentID == request.EffectID && effect.Revision == request.ExpectedEffectRevision && permit.Revision == request.ExpectedPermitRevision+1 {
			return cloneOperationPermitFactV3(permit), nil
		}
		return control.OperationDispatchPermitFactV3{}, core.NewError(core.ErrorConflict, core.ReasonDispatchPermitConsumed, "begun Permit replay changed Effect or expected revisions")
	}
	effect := s.effects[key][request.EffectID]
	if permit.State != control.OperationPermitIssuedV3 || permit.Revision != request.ExpectedPermitRevision || effect.Revision != request.ExpectedEffectRevision || permit.Permit.IntentID != request.EffectID || !now.Before(time.Unix(0, permit.Permit.ExpiresUnixNano)) {
		return control.OperationDispatchPermitFactV3{}, core.NewError(core.ErrorConflict, core.ReasonDispatchPermitConsumed, "operation Permit cannot begin at expected revisions")
	}
	permit.State = control.OperationPermitBegunV3
	permit.Revision++
	permit.BegunUnixNano = now.UnixNano()
	s.permits[key][request.PermitID] = cloneOperationPermitFactV3(permit)
	if s.loseNextBeginReply {
		s.loseNextBeginReply = false
		return control.OperationDispatchPermitFactV3{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected operation Begin reply loss")
	}
	return cloneOperationPermitFactV3(permit), nil
}

func (s *OperationEffectStoreV3) IssueOperationDispatchPermitV4(ctx context.Context, request control.IssueOperationPermitRequestV4) (control.IssueOperationPermitResultV4, error) {
	if err := contextError(ctx); err != nil {
		return control.IssueOperationPermitResultV4{}, err
	}
	if err := request.Permit.Validate(); err != nil {
		return control.IssueOperationPermitResultV4{}, err
	}
	now := s.clock()
	storedAuthorization, err := s.inspectReviewAuthorizationV4(ctx, request.Permit.Admission.Authorization, now)
	if err != nil {
		return control.IssueOperationPermitResultV4{}, err
	}
	if request.ReviewAuthorization.RefV4() != storedAuthorization.RefV4() || request.ReviewAuthorization.Digest != storedAuthorization.Digest {
		return control.IssueOperationPermitResultV4{}, core.NewError(core.ErrorConflict, core.ReasonReviewVerdictStale, "V4 Issue supplied another Review Authorization Fact")
	}
	if err := request.Permit.ValidateAgainstAuthorization(storedAuthorization, request.Fence, now); err != nil {
		return control.IssueOperationPermitResultV4{}, err
	}
	key, err := operationKeyV3(request.Operation)
	if err != nil {
		return control.IssueOperationPermitResultV4{}, err
	}
	fenceDigest, err := ports.DigestOperationExecutionFenceV3(request.Fence, request.Operation)
	if err != nil {
		return control.IssueOperationPermitResultV4{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	legacy := request.Permit.LegacyPermit
	if !ports.SameOperationSubjectV3(request.Operation, legacy.Operation) || request.EffectID != legacy.IntentID {
		return control.IssueOperationPermitResultV4{}, core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "V4 Issue request does not bind the Permit operation and Effect")
	}
	if _, exists := s.permits[key][legacy.ID]; exists {
		return control.IssueOperationPermitResultV4{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "operation Permit ID already belongs to V3")
	}
	if existing, exists := s.permitsV4[key][legacy.ID]; exists {
		existingFenceDigest, _ := ports.DigestOperationExecutionFenceV3(existing.Fence, request.Operation)
		effect, effectExists := s.effects[key][request.EffectID]
		if existing.PermitDigest == request.Permit.Digest && existingFenceDigest == fenceDigest && existing.Permit.Admission.Authorization == request.Permit.Admission.Authorization && effectExists && effect.State == control.OperationEffectDispatchIntentV3 && effect.Revision == existing.EffectFactRevision && effect.DispatchPermitID == legacy.ID && effect.DispatchPermitDigest == existing.PermitDigest && request.ExpectedEffectRevision < effect.Revision && effect.Revision-request.ExpectedEffectRevision == 1 {
			return control.IssueOperationPermitResultV4{Effect: cloneOperationEffectFactV3(effect), Permit: cloneOperationPermitFactV4(existing)}, nil
		}
		return control.IssueOperationPermitResultV4{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "V4 operation Permit ID binds different content")
	}
	effect, exists := s.effects[key][request.EffectID]
	if !exists {
		return control.IssueOperationPermitResultV4{}, core.NewError(core.ErrorNotFound, core.ReasonEffectIntentMissing, "V4 operation Effect not found")
	}
	if effect.State != control.OperationEffectAcceptedV3 || effect.Revision != request.ExpectedEffectRevision {
		return control.IssueOperationPermitResultV4{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "V4 operation Effect is not accepted at expected revision")
	}
	intentDigest, _ := effect.Intent.DigestV3()
	if legacy.IntentID != effect.Intent.ID || legacy.IntentRevision != effect.Intent.Revision || legacy.IntentDigest != intentDigest || !ports.SameOperationSubjectV3(legacy.Operation, effect.Intent.Operation) || legacy.PayloadSchema != effect.Intent.Payload.Schema || legacy.PayloadDigest != effect.Intent.Payload.ContentDigest || legacy.PayloadRevision != effect.Intent.PayloadRevision || legacy.ConflictDomain != effect.Intent.ConflictDomain || legacy.Provider != effect.Intent.Provider || legacy.EnforcementPoint != effect.Intent.Provider || legacy.Authority != effect.Intent.Authority || legacy.Review != effect.Intent.Review || legacy.Budget != effect.Intent.Budget || legacy.Policy != effect.Intent.Policy || legacy.Idempotency != effect.Intent.Idempotency || legacy.FenceDigest != fenceDigest || request.Permit.Admission.Admission.FactRevision != effect.Revision || now.IsZero() || !now.Before(time.Unix(0, legacy.ExpiresUnixNano)) {
		return control.IssueOperationPermitResultV4{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonDispatchPermitInvalid, "V4 Permit does not bind exact Effect, admission and Fence")
	}
	nextEffect := cloneOperationEffectFactV3(effect)
	nextEffect.State = control.OperationEffectDispatchIntentV3
	nextEffect.Revision++
	nextEffect.DispatchPermitID = legacy.ID
	nextEffect.DispatchPermitDigest = request.Permit.Digest
	nextEffect.UpdatedUnixNano = now.UnixNano()
	permitFact, err := ports.SealOperationDispatchRecordV4(ports.OperationDispatchRecordV4{
		Permit: request.Permit, PermitDigest: request.Permit.Digest, Fence: request.Fence,
		State: ports.OperationPermitIssuedV4, Revision: 1, EffectFactRevision: nextEffect.Revision,
	})
	if err != nil {
		return control.IssueOperationPermitResultV4{}, err
	}
	if err := nextEffect.Validate(); err != nil {
		return control.IssueOperationPermitResultV4{}, err
	}
	if s.permitsV4[key] == nil {
		s.permitsV4[key] = map[string]control.OperationDispatchPermitFactV4{}
	}
	s.effects[key][request.EffectID] = cloneOperationEffectFactV3(nextEffect)
	s.permitsV4[key][legacy.ID] = cloneOperationPermitFactV4(permitFact)
	s.issueV4CommitCount++
	if s.loseNextIssueReply {
		s.loseNextIssueReply = false
		return control.IssueOperationPermitResultV4{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected V4 operation Permit issue reply loss")
	}
	return control.IssueOperationPermitResultV4{Effect: cloneOperationEffectFactV3(nextEffect), Permit: cloneOperationPermitFactV4(permitFact)}, nil
}

func (s *OperationEffectStoreV3) InspectOperationDispatchPermitV4(ctx context.Context, subject ports.OperationSubjectV3, permitID string) (control.OperationDispatchPermitFactV4, error) {
	if err := contextError(ctx); err != nil {
		return control.OperationDispatchPermitFactV4{}, err
	}
	key, err := operationKeyV3(subject)
	if err != nil {
		return control.OperationDispatchPermitFactV4{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	fact, exists := s.permitsV4[key][permitID]
	if !exists {
		return control.OperationDispatchPermitFactV4{}, core.NewError(core.ErrorNotFound, core.ReasonDispatchPermitInvalid, "V4 operation Permit not found")
	}
	return cloneOperationPermitFactV4(fact), nil
}

func (s *OperationEffectStoreV3) BeginOperationDispatchV4(ctx context.Context, request control.BeginOperationDispatchRequestV4) (control.OperationDispatchPermitFactV4, error) {
	if err := contextError(ctx); err != nil {
		return control.OperationDispatchPermitFactV4{}, err
	}
	now := s.clock()
	if _, err := s.inspectReviewAuthorizationV4(ctx, request.ReviewAuthorization, now); err != nil {
		return control.OperationDispatchPermitFactV4{}, err
	}
	key, err := operationKeyV3(request.Operation)
	if err != nil {
		return control.OperationDispatchPermitFactV4{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	permit, exists := s.permitsV4[key][request.PermitID]
	if !exists {
		return control.OperationDispatchPermitFactV4{}, core.NewError(core.ErrorNotFound, core.ReasonDispatchPermitInvalid, "V4 operation Permit not found")
	}
	effect, effectExists := s.effects[key][request.EffectID]
	if permit.State == ports.OperationPermitBegunV4 {
		if effectExists && ports.SameOperationSubjectV3(request.Operation, permit.Permit.LegacyPermit.Operation) && permit.Permit.LegacyPermit.IntentID == request.EffectID && effect.Revision == request.ExpectedEffectRevision && permit.Revision == request.ExpectedPermitFactRevision+1 && permit.Permit.Admission.Digest == request.AdmissionDigest && permit.Permit.Admission.Authorization == request.ReviewAuthorization {
			return cloneOperationPermitFactV4(permit), nil
		}
		return control.OperationDispatchPermitFactV4{}, core.NewError(core.ErrorConflict, core.ReasonDispatchPermitConsumed, "begun V4 Permit replay changed exact watermarks")
	}
	if permit.State != ports.OperationPermitIssuedV4 || permit.Revision != request.ExpectedPermitFactRevision || !effectExists || !ports.SameOperationSubjectV3(request.Operation, permit.Permit.LegacyPermit.Operation) || permit.Permit.LegacyPermit.IntentID != request.EffectID || effect.State != control.OperationEffectDispatchIntentV3 || effect.Revision != request.ExpectedEffectRevision || effect.DispatchPermitID != request.PermitID || effect.DispatchPermitDigest != permit.PermitDigest || permit.Permit.Admission.Digest != request.AdmissionDigest || permit.Permit.Admission.Authorization != request.ReviewAuthorization || !now.Before(time.Unix(0, permit.Permit.LegacyPermit.ExpiresUnixNano)) {
		return control.OperationDispatchPermitFactV4{}, core.NewError(core.ErrorConflict, core.ReasonDispatchPermitConsumed, "V4 operation Permit cannot begin at expected watermarks")
	}
	permit.State = ports.OperationPermitBegunV4
	permit.Revision++
	permit.BegunUnixNano = now.UnixNano()
	permit, err = ports.SealOperationDispatchRecordV4(permit)
	if err != nil {
		return control.OperationDispatchPermitFactV4{}, err
	}
	s.permitsV4[key][request.PermitID] = cloneOperationPermitFactV4(permit)
	s.beginV4CommitCount++
	if s.loseNextBeginReply {
		s.loseNextBeginReply = false
		return control.OperationDispatchPermitFactV4{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected V4 operation Begin reply loss")
	}
	return cloneOperationPermitFactV4(permit), nil
}

func (s *OperationEffectStoreV3) AppendOperationDispatchEnforcementV4(ctx context.Context, request control.AppendOperationDispatchEnforcementRequestV4) (ports.OperationDispatchEnforcementJournalV4, error) {
	if err := contextError(ctx); err != nil {
		return ports.OperationDispatchEnforcementJournalV4{}, err
	}
	if err := request.Receipt.Validate(); err != nil {
		return ports.OperationDispatchEnforcementJournalV4{}, err
	}
	key, err := operationKeyV3(request.Operation)
	if err != nil {
		return ports.OperationDispatchEnforcementJournalV4{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	permit, permitExists := s.permitsV4[key][request.PermitID]
	effect, effectExists := s.effects[key][request.EffectID]
	if !permitExists || !effectExists || permit.State != ports.OperationPermitBegunV4 || !ports.SameOperationSubjectV3(request.Operation, permit.Permit.LegacyPermit.Operation) || permit.Permit.LegacyPermit.IntentID != request.EffectID || effect.DispatchPermitID != request.PermitID || effect.DispatchPermitDigest != permit.PermitDigest {
		return ports.OperationDispatchEnforcementJournalV4{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonDispatchPermitInvalid, "enforcement Effect Owner requires the exact begun V4 Permit")
	}
	receipt := request.Receipt
	legacy := permit.Permit.LegacyPermit
	if receipt.PermitID != legacy.ID || receipt.PermitFactRevision != permit.Revision || receipt.PermitDigest != permit.PermitDigest || receipt.AdmissionDigest != permit.Permit.Admission.Digest || receipt.ReviewAuthorization != permit.Permit.Admission.Authorization || receipt.EffectID != request.EffectID || receipt.IntentRevision != legacy.IntentRevision || receipt.IntentDigest != legacy.IntentDigest || receipt.AttemptID != legacy.AttemptID || receipt.Verifier != legacy.EnforcementPoint || !ports.SameOperationSubjectV3(receipt.Operation, request.Operation) || receipt.ValidatedUnixNano < permit.BegunUnixNano || receipt.ValidatedUnixNano >= legacy.ExpiresUnixNano {
		return ports.OperationDispatchEnforcementJournalV4{}, core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "enforcement receipt changed exact Permit, Effect, attempt or verifier")
	}
	if s.enforcementV4[key] == nil {
		s.enforcementV4[key] = map[string]ports.OperationDispatchEnforcementJournalV4{}
	}
	current, exists := s.enforcementV4[key][request.PermitID]
	if exists {
		stored := current.Prepare
		if receipt.Phase == ports.OperationDispatchEnforcementExecuteV4 {
			stored = current.Execute
		}
		if stored != nil {
			if stored.Digest == receipt.Digest {
				return cloneOperationDispatchEnforcementJournalV4(current), nil
			}
			return ports.OperationDispatchEnforcementJournalV4{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "enforcement phase slot already binds different content")
		}
	}
	var next ports.OperationDispatchEnforcementJournalV4
	switch receipt.Phase {
	case ports.OperationDispatchEnforcementPrepareV4:
		if exists || request.ExpectedJournalRevision != 0 {
			return ports.OperationDispatchEnforcementJournalV4{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "prepare enforcement journal already exists or expected revision drifted")
		}
		next, err = ports.SealOperationDispatchEnforcementJournalV4(ports.OperationDispatchEnforcementJournalV4{
			OperationDigest: receipt.OperationDigest, EffectID: receipt.EffectID, PermitID: receipt.PermitID,
			AttemptID: receipt.AttemptID, SandboxAttempt: receipt.SandboxAttempt, Revision: 1, Prepare: &receipt, UpdatedUnixNano: receipt.ValidatedUnixNano,
		})
	case ports.OperationDispatchEnforcementExecuteV4:
		if !exists || current.Revision != 1 || request.ExpectedJournalRevision != 1 || current.Prepare == nil || receipt.Prepare == nil {
			return ports.OperationDispatchEnforcementJournalV4{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "execute enforcement requires the exact prepare-only journal")
		}
		prepareRef, refErr := current.Prepare.RefV4(1)
		if refErr != nil || *receipt.Prepare != prepareRef {
			return ports.OperationDispatchEnforcementJournalV4{}, core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "execute enforcement changed the prepare phase ref")
		}
		next = cloneOperationDispatchEnforcementJournalV4(current)
		next.Revision = 2
		next.Execute = &receipt
		next.UpdatedUnixNano = receipt.ValidatedUnixNano
		next, err = ports.SealOperationDispatchEnforcementJournalV4(next)
	default:
		err = core.NewError(core.ErrorInvalidArgument, core.ReasonDispatchPermitInvalid, "enforcement phase is invalid")
	}
	if err != nil {
		return ports.OperationDispatchEnforcementJournalV4{}, err
	}
	s.enforcementV4[key][request.PermitID] = cloneOperationDispatchEnforcementJournalV4(next)
	s.enforcementV4CommitCount++
	if s.loseNextEnforcementV4Reply {
		s.loseNextEnforcementV4Reply = false
		return ports.OperationDispatchEnforcementJournalV4{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected V4.1 enforcement append reply loss")
	}
	return cloneOperationDispatchEnforcementJournalV4(next), nil
}

func (s *OperationEffectStoreV3) InspectOperationDispatchEnforcementV4(ctx context.Context, subject ports.OperationSubjectV3, effectID core.EffectIntentID, permitID string) (ports.OperationDispatchEnforcementJournalV4, error) {
	if err := contextError(ctx); err != nil {
		return ports.OperationDispatchEnforcementJournalV4{}, err
	}
	key, err := operationKeyV3(subject)
	if err != nil {
		return ports.OperationDispatchEnforcementJournalV4{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	journal, exists := s.enforcementV4[key][permitID]
	if !exists || journal.EffectID != effectID {
		return ports.OperationDispatchEnforcementJournalV4{}, core.NewError(core.ErrorNotFound, core.ReasonDispatchPermitInvalid, "V4.1 enforcement journal not found")
	}
	return cloneOperationDispatchEnforcementJournalV4(journal), nil
}

func (s *OperationEffectStoreV3) inspectReviewAuthorizationV4(ctx context.Context, ref ports.OperationReviewAuthorizationRefV4, now time.Time) (ports.OperationReviewAuthorizationFactV4, error) {
	s.mu.Lock()
	facts := s.reviewAuthorizationsV4
	s.mu.Unlock()
	if facts == nil {
		return ports.OperationReviewAuthorizationFactV4{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "V4 Effect Owner requires the Review Authorization Fact reader")
	}
	fact, err := facts.InspectOperationReviewAuthorizationV4(ctx, ref.ID)
	if err != nil {
		return ports.OperationReviewAuthorizationFactV4{}, err
	}
	if err := fact.Validate(); err != nil {
		return ports.OperationReviewAuthorizationFactV4{}, err
	}
	if fact.RefV4() != ref || fact.State != ports.OperationReviewAuthorizationActiveV4 || now.IsZero() || !now.Before(time.Unix(0, fact.ExpiresUnixNano)) {
		return ports.OperationReviewAuthorizationFactV4{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "V4 Effect Owner observed stale Review Authorization")
	}
	return fact, nil
}

func (s *OperationEffectStoreV3) RecordOperationEnforcementV3(ctx context.Context, request control.RecordOperationEnforcementRequestV3) (control.OperationDispatchPermitFactV3, error) {
	if err := contextError(ctx); err != nil {
		return control.OperationDispatchPermitFactV3{}, err
	}
	key, err := operationKeyV3(request.Operation)
	if err != nil {
		return control.OperationDispatchPermitFactV3{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	permit, ok := s.permits[key][request.PermitID]
	if !ok {
		return control.OperationDispatchPermitFactV3{}, core.NewError(core.ErrorNotFound, core.ReasonDispatchPermitInvalid, "operation Permit not found")
	}
	if permit.Enforcement != nil {
		existingDigest, _ := core.CanonicalJSONDigest("praxis.runtime.operation-effect", ports.OperationEffectContractVersionV3, "OperationEnforcementReceiptV3", permit.Enforcement)
		nextDigest, _ := core.CanonicalJSONDigest("praxis.runtime.operation-effect", ports.OperationEffectContractVersionV3, "OperationEnforcementReceiptV3", request.Receipt)
		if existingDigest == nextDigest {
			return cloneOperationPermitFactV3(permit), nil
		}
		return control.OperationDispatchPermitFactV3{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "operation enforcement receipt conflicts")
	}
	if permit.State != control.OperationPermitBegunV3 || permit.Revision != request.ExpectedPermitRevision || request.Receipt.PermitID != permit.Permit.ID || request.Receipt.PermitRevision != permit.Permit.Revision || request.Receipt.PermitDigest != permit.PermitDigest || request.Receipt.AttemptID != permit.Permit.AttemptID || !ports.SameOperationSubjectV3(request.Receipt.Operation, permit.Permit.Operation) || request.Receipt.Verifier != permit.Permit.EnforcementPoint || request.Receipt.ValidatedUnixNano < permit.BegunUnixNano || request.Receipt.ValidatedUnixNano >= permit.Permit.ExpiresUnixNano {
		return control.OperationDispatchPermitFactV3{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonDispatchPermitInvalid, "operation enforcement does not match exact begun Permit")
	}
	permit.Revision++
	receipt := request.Receipt
	permit.Enforcement = &receipt
	if err := permit.Validate(); err != nil {
		return control.OperationDispatchPermitFactV3{}, err
	}
	s.permits[key][request.PermitID] = cloneOperationPermitFactV3(permit)
	if s.loseNextEnforcementReply {
		s.loseNextEnforcementReply = false
		return control.OperationDispatchPermitFactV3{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected operation enforcement reply loss")
	}
	return cloneOperationPermitFactV3(permit), nil
}

func operationDispatchReceiptMatchesV3(receipt control.OperationProviderDispatchReceiptV3, permit control.OperationDispatchPermitFactV3) bool {
	operationDigest, _ := permit.Permit.Operation.DigestV3()
	if permit.Enforcement == nil {
		return false
	}
	enforcement, err := permit.PersistedEnforcementRefV3()
	return err == nil && receipt.Validate() == nil && receipt.PermitID == permit.Permit.ID && receipt.PermitRevision == permit.Permit.Revision && receipt.PermitDigest == permit.PermitDigest && receipt.AttemptID == permit.Permit.AttemptID && receipt.IntentID == permit.Permit.IntentID && receipt.IntentRevision == permit.Permit.IntentRevision && receipt.IntentDigest == permit.Permit.IntentDigest && receipt.OperationDigest == operationDigest && receipt.Provider == permit.Permit.Provider && receipt.PayloadSchema == permit.Permit.PayloadSchema && receipt.PayloadDigest == permit.Permit.PayloadDigest && receipt.PayloadRevision == permit.Permit.PayloadRevision && receipt.Enforcement == enforcement && receipt.Prepared.PermitID == permit.Permit.ID && receipt.Prepared.AttemptID == permit.Permit.AttemptID
}

func operationEffectFactDigestV3(fact control.OperationEffectFactV3) (core.Digest, error) {
	if err := fact.Validate(); err != nil {
		return "", err
	}
	if fact.Intent.Owners == nil {
		fact.Intent.Owners = []ports.EffectOwnerRefV2{}
	}
	if fact.Intent.CredentialLeases == nil {
		fact.Intent.CredentialLeases = []ports.CredentialLeaseRefV2{}
	}
	return core.CanonicalJSONDigest("praxis.runtime.operation-effect", ports.OperationEffectContractVersionV3, "OperationEffectFactV3", fact)
}

func cloneOperationEffectFactV3(fact control.OperationEffectFactV3) control.OperationEffectFactV3 {
	fact.Intent.Owners = append([]ports.EffectOwnerRefV2{}, fact.Intent.Owners...)
	fact.Intent.CredentialLeases = append([]ports.CredentialLeaseRefV2{}, fact.Intent.CredentialLeases...)
	fact.Intent.Payload.Inline = append([]byte{}, fact.Intent.Payload.Inline...)
	if fact.DispatchReceipt != nil {
		value := *fact.DispatchReceipt
		fact.DispatchReceipt = &value
	}
	if fact.Settlement != nil {
		value := *fact.Settlement
		value.Evidence = append([]ports.EvidenceRecordRefV2{}, value.Evidence...)
		if value.DomainResult != nil {
			payload := *value.DomainResult
			payload.Inline = append([]byte{}, payload.Inline...)
			value.DomainResult = &payload
		}
		fact.Settlement = &value
	}
	return fact
}

func cloneOperationPermitFactV3(fact control.OperationDispatchPermitFactV3) control.OperationDispatchPermitFactV3 {
	if fact.Enforcement != nil {
		value := *fact.Enforcement
		if value.Attestation != nil {
			extension := *value.Attestation
			extension.Payload.Inline = append([]byte{}, extension.Payload.Inline...)
			value.Attestation = &extension
		}
		fact.Enforcement = &value
	}
	return fact
}

func cloneOperationPermitFactV4(fact control.OperationDispatchPermitFactV4) control.OperationDispatchPermitFactV4 {
	if lease := fact.Permit.LegacyPermit.Operation.ExecutionScope.SandboxLease; lease != nil {
		value := *lease
		fact.Permit.LegacyPermit.Operation.ExecutionScope.SandboxLease = &value
	}
	if lease := fact.Fence.Scope.SandboxLease; lease != nil {
		value := *lease
		fact.Fence.Scope.SandboxLease = &value
	}
	if fact.Permit.LegacyPermit.ReviewAuthorization.Satisfaction != nil {
		value := *fact.Permit.LegacyPermit.ReviewAuthorization.Satisfaction
		fact.Permit.LegacyPermit.ReviewAuthorization.Satisfaction = &value
	}
	if fact.Enforcement != nil {
		value := *fact.Enforcement
		fact.Enforcement = &value
	}
	return fact
}

func cloneOperationDispatchEnforcementJournalV4(journal ports.OperationDispatchEnforcementJournalV4) ports.OperationDispatchEnforcementJournalV4 {
	if journal.Prepare != nil {
		value := cloneOperationDispatchEnforcementReceiptV4(*journal.Prepare)
		journal.Prepare = &value
	}
	if journal.Execute != nil {
		value := cloneOperationDispatchEnforcementReceiptV4(*journal.Execute)
		journal.Execute = &value
	}
	return journal
}

func cloneOperationDispatchEnforcementReceiptV4(receipt ports.OperationDispatchEnforcementPhaseReceiptV4) ports.OperationDispatchEnforcementPhaseReceiptV4 {
	if lease := receipt.Operation.ExecutionScope.SandboxLease; lease != nil {
		value := *lease
		receipt.Operation.ExecutionScope.SandboxLease = &value
	}
	if lease := receipt.Sandbox.Operation.ExecutionScope.SandboxLease; lease != nil {
		value := *lease
		receipt.Sandbox.Operation.ExecutionScope.SandboxLease = &value
	}
	if receipt.CheckpointSandbox != nil {
		value := *receipt.CheckpointSandbox
		if lease := value.Operation.ExecutionScope.SandboxLease; lease != nil {
			copyLease := *lease
			value.Operation.ExecutionScope.SandboxLease = &copyLease
		}
		if value.ChangeSet != nil {
			copyRef := *value.ChangeSet
			value.ChangeSet = &copyRef
		}
		if value.PrepareEnforcement != nil {
			copyRef := *value.PrepareEnforcement
			value.PrepareEnforcement = &copyRef
		}
		if value.PreparedAttempt != nil {
			copyRef := *value.PreparedAttempt
			value.PreparedAttempt = &copyRef
		}
		if value.Reservation.PreviousPhase != nil {
			copyRef := *value.Reservation.PreviousPhase
			value.Reservation.PreviousPhase = &copyRef
		}
		value.Watermarks = append([]ports.CheckpointRestoreDispatchWatermarkV1{}, value.Watermarks...)
		receipt.CheckpointSandbox = &value
	}
	if receipt.Prepare != nil {
		value := *receipt.Prepare
		receipt.Prepare = &value
	}
	if receipt.PreparedAttempt != nil {
		value := *receipt.PreparedAttempt
		receipt.PreparedAttempt = &value
	}
	return receipt
}

// ExecutionDelegationStoreV2 is a create-once/CAS in-memory Fact Owner for
// tests. Relay/provider implementations receive only the public Port.
type ExecutionDelegationStoreV2 struct {
	mu                  sync.Mutex
	clock               func() time.Time
	facts               map[string]ports.ExecutionDelegationFactV2
	loseNextCreateReply bool
	loseNextCASReply    bool
}

func NewExecutionDelegationStoreV2(clock func() time.Time) *ExecutionDelegationStoreV2 {
	if clock == nil {
		clock = time.Now
	}
	return &ExecutionDelegationStoreV2{clock: clock, facts: map[string]ports.ExecutionDelegationFactV2{}}
}

func (s *ExecutionDelegationStoreV2) LoseNextCreateReply() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loseNextCreateReply = true
}

func (s *ExecutionDelegationStoreV2) LoseNextCASReply() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loseNextCASReply = true
}

func (s *ExecutionDelegationStoreV2) CreateExecutionDelegationV2(ctx context.Context, fact ports.ExecutionDelegationFactV2) (ports.ExecutionDelegationFactV2, error) {
	if err := contextError(ctx); err != nil {
		return ports.ExecutionDelegationFactV2{}, err
	}
	if err := fact.Validate(); err != nil {
		return ports.ExecutionDelegationFactV2{}, err
	}
	if fact.Revision != 1 || fact.State != ports.ExecutionDelegationDeclaredV2 {
		return ports.ExecutionDelegationFactV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "new delegation must be declared revision one")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.facts[fact.ID]; ok {
		existingDigest, _ := existing.DigestV2()
		nextDigest, _ := fact.DigestV2()
		if existingDigest == nextDigest {
			return cloneExecutionDelegationV2(existing), nil
		}
		return ports.ExecutionDelegationFactV2{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "delegation ID binds different content")
	}
	s.facts[fact.ID] = cloneExecutionDelegationV2(fact)
	if s.loseNextCreateReply {
		s.loseNextCreateReply = false
		return ports.ExecutionDelegationFactV2{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected delegation create reply loss")
	}
	return cloneExecutionDelegationV2(fact), nil
}

func (s *ExecutionDelegationStoreV2) InspectExecutionDelegationV2(ctx context.Context, id string) (ports.ExecutionDelegationFactV2, error) {
	if err := contextError(ctx); err != nil {
		return ports.ExecutionDelegationFactV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	fact, ok := s.facts[id]
	if !ok {
		return ports.ExecutionDelegationFactV2{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "delegation not found")
	}
	return cloneExecutionDelegationV2(fact), nil
}

func (s *ExecutionDelegationStoreV2) CompareAndSwapExecutionDelegationV2(ctx context.Context, request ports.ExecutionDelegationCASRequestV2) (ports.ExecutionDelegationFactV2, error) {
	if err := contextError(ctx); err != nil {
		return ports.ExecutionDelegationFactV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.facts[request.Next.ID]
	if !ok {
		return ports.ExecutionDelegationFactV2{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "delegation not found")
	}
	if current.Revision != request.ExpectedRevision {
		currentDigest, _ := current.DigestV2()
		nextDigest, _ := request.Next.DigestV2()
		if currentDigest == nextDigest {
			return cloneExecutionDelegationV2(current), nil
		}
		return ports.ExecutionDelegationFactV2{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "delegation CAS lost")
	}
	if err := control.ValidateExecutionDelegationTransitionV2(current, request.Next, s.clock()); err != nil {
		return ports.ExecutionDelegationFactV2{}, err
	}
	s.facts[request.Next.ID] = cloneExecutionDelegationV2(request.Next)
	if s.loseNextCASReply {
		s.loseNextCASReply = false
		return ports.ExecutionDelegationFactV2{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected delegation CAS reply loss")
	}
	return cloneExecutionDelegationV2(request.Next), nil
}

func cloneExecutionDelegationV2(fact ports.ExecutionDelegationFactV2) ports.ExecutionDelegationFactV2 {
	fact.RelayHops = append([]ports.ExecutionRelayHopV2{}, fact.RelayHops...)
	if fact.Preparation != nil {
		preparation := *fact.Preparation
		if preparation.Enforcement.Attestation != nil {
			extension := *preparation.Enforcement.Attestation
			extension.Payload.Inline = append([]byte{}, extension.Payload.Inline...)
			preparation.Enforcement.Attestation = &extension
		}
		fact.Preparation = &preparation
	}
	return fact
}

// ProviderAttemptObservationStoreV2 is a test-only create-once Observation
// owner. An Observation is evidence and never a settlement/outcome grant.
type ProviderAttemptObservationStoreV2 struct {
	mu            sync.Mutex
	observations  map[string]ports.ProviderAttemptObservationV2
	loseNextReply bool
}

func NewProviderAttemptObservationStoreV2() *ProviderAttemptObservationStoreV2 {
	return &ProviderAttemptObservationStoreV2{observations: map[string]ports.ProviderAttemptObservationV2{}}
}

func (s *ProviderAttemptObservationStoreV2) LoseNextCreateReply() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loseNextReply = true
}

func providerObservationKeyV2(delegation ports.ExecutionDelegationRefV2, preparedID string) string {
	return delegation.ID + "\x00" + preparedID
}

func (s *ProviderAttemptObservationStoreV2) CreateProviderAttemptObservationV2(ctx context.Context, observation ports.ProviderAttemptObservationV2) (ports.ProviderAttemptObservationV2, error) {
	if err := contextError(ctx); err != nil {
		return ports.ProviderAttemptObservationV2{}, err
	}
	if err := observation.Validate(); err != nil {
		return ports.ProviderAttemptObservationV2{}, err
	}
	key := providerObservationKeyV2(observation.Delegation, observation.Prepared.ID)
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.observations[key]; ok {
		existingRef, _ := existing.RefV2()
		nextRef, _ := observation.RefV2()
		if existingRef.Digest == nextRef.Digest {
			return cloneProviderObservationV2(existing), nil
		}
		return ports.ProviderAttemptObservationV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "provider observation source attempt changed canonical content")
	}
	s.observations[key] = cloneProviderObservationV2(observation)
	if s.loseNextReply {
		s.loseNextReply = false
		return ports.ProviderAttemptObservationV2{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected provider observation reply loss")
	}
	return cloneProviderObservationV2(observation), nil
}

func (s *ProviderAttemptObservationStoreV2) InspectProviderAttemptObservationV2(ctx context.Context, delegation ports.ExecutionDelegationRefV2, preparedID string) (ports.ProviderAttemptObservationV2, error) {
	if err := contextError(ctx); err != nil {
		return ports.ProviderAttemptObservationV2{}, err
	}
	if err := delegation.Validate(); err != nil {
		return ports.ProviderAttemptObservationV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	observation, ok := s.observations[providerObservationKeyV2(delegation, preparedID)]
	if !ok {
		return ports.ProviderAttemptObservationV2{}, core.NewError(core.ErrorNotFound, core.ReasonEvidenceUnavailable, "provider observation not found")
	}
	return cloneProviderObservationV2(observation), nil
}

func cloneProviderObservationV2(observation ports.ProviderAttemptObservationV2) ports.ProviderAttemptObservationV2 {
	observation.Payload.Inline = append([]byte{}, observation.Payload.Inline...)
	return observation
}
