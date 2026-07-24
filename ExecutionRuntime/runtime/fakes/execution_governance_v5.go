package fakes

import (
	"context"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func (s *OperationEffectStoreV3) BindOperationReviewAuthorizationV5(g ports.OperationReviewAuthorizationGovernancePortV5) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reviewAuthorizationsV5 = g
}
func (s *OperationEffectStoreV3) LoseNextIssueV5Reply() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loseNextIssueV5Reply = true
}
func (s *OperationEffectStoreV3) LoseNextBeginV5Reply() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loseNextBeginV5Reply = true
}
func (s *OperationEffectStoreV3) IssueV5CommitCount() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.issueV5CommitCount
}
func (s *OperationEffectStoreV3) BeginV5CommitCount() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.beginV5CommitCount
}
func (s *OperationEffectStoreV3) LoseNextEnforcementV5Reply() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loseNextEnforcementV5Reply = true
}
func (s *OperationEffectStoreV3) EnforcementV5CommitCount() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.enforcementV5CommitCount
}

func (s *OperationEffectStoreV3) IssueOperationDispatchPermitV5(ctx context.Context, request control.IssueOperationPermitRequestV5) (control.IssueOperationPermitResultV5, error) {
	if err := contextError(ctx); err != nil {
		return control.IssueOperationPermitResultV5{}, err
	}
	if err := request.Permit.Validate(); err != nil {
		return control.IssueOperationPermitResultV5{}, err
	}
	now := s.clock()
	authorization, err := s.inspectReviewAuthorizationV5(ctx, request.Permit.Authorization, request.Permit.AuthorizationBasis, request.Operation, request.EffectID, now)
	if err != nil {
		return control.IssueOperationPermitResultV5{}, err
	}
	if request.ReviewAuthorization.RefV5() != authorization.RefV5() || request.ReviewAuthorization.Digest != authorization.Digest {
		return control.IssueOperationPermitResultV5{}, core.NewError(core.ErrorConflict, core.ReasonReviewVerdictStale, "V5 Issue supplied another Review Authorization Fact")
	}
	if err := request.Permit.ValidateAgainstAuthorization(authorization, request.Fence, now); err != nil {
		return control.IssueOperationPermitResultV5{}, err
	}
	key, err := operationKeyV3(request.Operation)
	if err != nil {
		return control.IssueOperationPermitResultV5{}, err
	}
	fenceDigest, err := ports.DigestOperationExecutionFenceV3(request.Fence, request.Operation)
	if err != nil {
		return control.IssueOperationPermitResultV5{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	permit := request.Permit
	if _, exists := s.permits[key][permit.ID]; exists {
		return control.IssueOperationPermitResultV5{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Permit ID already belongs to V3")
	}
	if _, exists := s.permitsV4[key][permit.ID]; exists {
		return control.IssueOperationPermitResultV5{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Permit ID already belongs to V4")
	}
	if existing, exists := s.permitsV5[key][permit.ID]; exists {
		fd, _ := ports.DigestOperationExecutionFenceV3(existing.Fence, request.Operation)
		effect, ok := s.effects[key][request.EffectID]
		if existing.PermitDigest == permit.Digest && fd == fenceDigest && existing.Permit.Authorization == permit.Authorization && ok && effect.State == control.OperationEffectDispatchIntentV3 && effect.Revision == existing.EffectFactRevision && effect.DispatchPermitID == permit.ID && effect.DispatchPermitDigest == existing.PermitDigest && request.ExpectedEffectRevision+1 == effect.Revision {
			return control.IssueOperationPermitResultV5{Effect: cloneOperationEffectFactV3(effect), Permit: cloneOperationPermitFactV5(existing)}, nil
		}
		return control.IssueOperationPermitResultV5{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "V5 Permit ID binds different content")
	}
	effect, exists := s.effects[key][request.EffectID]
	if !exists {
		return control.IssueOperationPermitResultV5{}, core.NewError(core.ErrorNotFound, core.ReasonEffectIntentMissing, "V5 Effect not found")
	}
	if effect.State != control.OperationEffectAcceptedV3 || effect.Revision != request.ExpectedEffectRevision {
		return control.IssueOperationPermitResultV5{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "V5 Effect is not accepted at expected revision")
	}
	intentDigest, _ := effect.Intent.DigestV3()
	if permit.IntentID != effect.Intent.ID || permit.IntentRevision != effect.Intent.Revision || permit.IntentDigest != intentDigest || !ports.SameOperationSubjectV3(permit.Operation, effect.Intent.Operation) || permit.PayloadSchema != effect.Intent.Payload.Schema || permit.PayloadDigest != effect.Intent.Payload.ContentDigest || permit.PayloadRevision != effect.Intent.PayloadRevision || permit.ConflictDomain != effect.Intent.ConflictDomain || permit.Provider != effect.Intent.Provider || permit.EnforcementPoint != effect.Intent.Provider || permit.Authority != effect.Intent.Authority || permit.Review != effect.Intent.Review || permit.Budget != effect.Intent.Budget || permit.Policy != effect.Intent.Policy || permit.Idempotency != effect.Intent.Idempotency || permit.FenceDigest != fenceDigest || permit.Admission.Admission.FactRevision != effect.Revision || now.IsZero() || !now.Before(time.Unix(0, permit.ExpiresUnixNano)) {
		return control.IssueOperationPermitResultV5{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonDispatchPermitInvalid, "V5 Permit does not bind exact Effect and Fence")
	}
	next := cloneOperationEffectFactV3(effect)
	next.State = control.OperationEffectDispatchIntentV3
	next.Revision++
	next.DispatchPermitID = permit.ID
	next.DispatchPermitDigest = permit.Digest
	next.UpdatedUnixNano = now.UnixNano()
	record, err := ports.SealOperationDispatchRecordV5(ports.OperationDispatchRecordV5{Permit: permit, PermitDigest: permit.Digest, Fence: request.Fence, State: ports.OperationPermitIssuedV5, Revision: 1, EffectFactRevision: next.Revision})
	if err != nil {
		return control.IssueOperationPermitResultV5{}, err
	}
	if err := next.Validate(); err != nil {
		return control.IssueOperationPermitResultV5{}, err
	}
	if s.permitsV5[key] == nil {
		s.permitsV5[key] = map[string]control.OperationDispatchPermitFactV5{}
	}
	s.effects[key][request.EffectID] = cloneOperationEffectFactV3(next)
	s.permitsV5[key][permit.ID] = cloneOperationPermitFactV5(record)
	s.issueV5CommitCount++
	if s.loseNextIssueV5Reply {
		s.loseNextIssueV5Reply = false
		return control.IssueOperationPermitResultV5{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected V5 Issue reply loss")
	}
	return control.IssueOperationPermitResultV5{Effect: cloneOperationEffectFactV3(next), Permit: cloneOperationPermitFactV5(record)}, nil
}

func (s *OperationEffectStoreV3) InspectOperationDispatchPermitV5(ctx context.Context, subject ports.OperationSubjectV3, permitID string) (control.OperationDispatchPermitFactV5, error) {
	if err := contextError(ctx); err != nil {
		return control.OperationDispatchPermitFactV5{}, err
	}
	key, err := operationKeyV3(subject)
	if err != nil {
		return control.OperationDispatchPermitFactV5{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	fact, ok := s.permitsV5[key][permitID]
	if !ok {
		return control.OperationDispatchPermitFactV5{}, core.NewError(core.ErrorNotFound, core.ReasonDispatchPermitInvalid, "V5 Permit not found")
	}
	return cloneOperationPermitFactV5(fact), nil
}

func (s *OperationEffectStoreV3) BeginOperationDispatchV5(ctx context.Context, request control.BeginOperationDispatchRequestV5) (control.OperationDispatchPermitFactV5, error) {
	if err := contextError(ctx); err != nil {
		return control.OperationDispatchPermitFactV5{}, err
	}
	now := s.clock()
	if _, err := s.inspectReviewAuthorizationV5(ctx, request.ReviewAuthorization, request.AuthorizationBasis, request.Operation, request.EffectID, now); err != nil {
		return control.OperationDispatchPermitFactV5{}, err
	}
	key, err := operationKeyV3(request.Operation)
	if err != nil {
		return control.OperationDispatchPermitFactV5{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	permit, exists := s.permitsV5[key][request.PermitID]
	if !exists {
		return control.OperationDispatchPermitFactV5{}, core.NewError(core.ErrorNotFound, core.ReasonDispatchPermitInvalid, "V5 Permit not found")
	}
	effect, effectExists := s.effects[key][request.EffectID]
	if permit.State == ports.OperationPermitBegunV5 {
		if effectExists && ports.SameOperationSubjectV3(request.Operation, permit.Permit.Operation) && permit.Permit.IntentID == request.EffectID && effect.Revision == request.ExpectedEffectRevision && permit.Revision == request.ExpectedPermitFactRevision+1 && permit.Permit.Admission.Digest == request.AdmissionDigest && permit.Permit.Authorization == request.ReviewAuthorization && permit.Permit.AuthorizationBasis == request.AuthorizationBasis {
			return cloneOperationPermitFactV5(permit), nil
		}
		return control.OperationDispatchPermitFactV5{}, core.NewError(core.ErrorConflict, core.ReasonDispatchPermitConsumed, "begun V5 Permit replay changed watermarks")
	}
	if permit.State != ports.OperationPermitIssuedV5 || permit.Revision != request.ExpectedPermitFactRevision || !effectExists || !ports.SameOperationSubjectV3(request.Operation, permit.Permit.Operation) || permit.Permit.IntentID != request.EffectID || effect.State != control.OperationEffectDispatchIntentV3 || effect.Revision != request.ExpectedEffectRevision || effect.DispatchPermitID != request.PermitID || effect.DispatchPermitDigest != permit.PermitDigest || permit.Permit.Admission.Digest != request.AdmissionDigest || permit.Permit.Authorization != request.ReviewAuthorization || permit.Permit.AuthorizationBasis != request.AuthorizationBasis || !now.Before(time.Unix(0, permit.Permit.ExpiresUnixNano)) {
		return control.OperationDispatchPermitFactV5{}, core.NewError(core.ErrorConflict, core.ReasonDispatchPermitConsumed, "V5 Permit cannot begin at expected watermarks")
	}
	permit.State = ports.OperationPermitBegunV5
	permit.Revision++
	permit.BegunUnixNano = now.UnixNano()
	permit, err = ports.SealOperationDispatchRecordV5(permit)
	if err != nil {
		return control.OperationDispatchPermitFactV5{}, err
	}
	s.permitsV5[key][request.PermitID] = cloneOperationPermitFactV5(permit)
	s.beginV5CommitCount++
	if s.loseNextBeginV5Reply {
		s.loseNextBeginV5Reply = false
		return control.OperationDispatchPermitFactV5{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected V5 Begin reply loss")
	}
	return cloneOperationPermitFactV5(permit), nil
}

func (s *OperationEffectStoreV3) inspectReviewAuthorizationV5(ctx context.Context, ref ports.OperationReviewAuthorizationRefV5, basis ports.OperationReviewAuthorizationBasisV5, operation ports.OperationSubjectV3, effectID core.EffectIntentID, now time.Time) (ports.OperationReviewAuthorizationFactV5, error) {
	s.mu.Lock()
	reader := s.reviewAuthorizationsV5
	s.mu.Unlock()
	if reader == nil {
		return ports.OperationReviewAuthorizationFactV5{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "V5 Effect Owner requires current Review Authorization gateway")
	}
	fact, err := reader.InspectCurrentOperationReviewAuthorizationV5(ctx, operation, effectID, ref.ID)
	if err != nil {
		return ports.OperationReviewAuthorizationFactV5{}, err
	}
	if err := fact.Validate(); err != nil {
		return ports.OperationReviewAuthorizationFactV5{}, err
	}
	if fact.RefV5() != ref || fact.Review.Basis != basis || fact.State != ports.OperationReviewAuthorizationActiveV5 || now.IsZero() || !now.Before(time.Unix(0, fact.ExpiresUnixNano)) {
		return ports.OperationReviewAuthorizationFactV5{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "V5 Effect Owner observed stale Authorization")
	}
	return fact, nil
}

func (s *OperationEffectStoreV3) AppendOperationDispatchEnforcementV5(ctx context.Context, request control.AppendOperationDispatchEnforcementRequestV5) (ports.OperationDispatchEnforcementJournalV5, error) {
	if err := contextError(ctx); err != nil {
		return ports.OperationDispatchEnforcementJournalV5{}, err
	}
	if err := request.Receipt.Validate(); err != nil {
		return ports.OperationDispatchEnforcementJournalV5{}, err
	}
	now := s.clock()
	if _, err := s.inspectReviewAuthorizationV5(ctx, request.Receipt.ReviewAuthorization, request.Receipt.AuthorizationBasis, request.Operation, request.EffectID, now); err != nil {
		return ports.OperationDispatchEnforcementJournalV5{}, err
	}
	key, err := operationKeyV3(request.Operation)
	if err != nil {
		return ports.OperationDispatchEnforcementJournalV5{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	permit, permitExists := s.permitsV5[key][request.PermitID]
	effect, effectExists := s.effects[key][request.EffectID]
	if !permitExists || !effectExists || permit.State != ports.OperationPermitBegunV5 || !ports.SameOperationSubjectV3(request.Operation, permit.Permit.Operation) || permit.Permit.IntentID != request.EffectID || effect.DispatchPermitID != request.PermitID || effect.DispatchPermitDigest != permit.PermitDigest {
		return ports.OperationDispatchEnforcementJournalV5{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonDispatchPermitInvalid, "V5 enforcement requires exact begun Permit")
	}
	receipt := request.Receipt
	p := permit.Permit
	if receipt.PermitID != p.ID || receipt.PermitFactRevision != permit.Revision || receipt.PermitDigest != permit.PermitDigest || receipt.AdmissionDigest != p.Admission.Digest || receipt.ReviewAuthorization != p.Authorization || receipt.AuthorizationBasis != p.AuthorizationBasis || receipt.EffectID != request.EffectID || receipt.IntentRevision != p.IntentRevision || receipt.IntentDigest != p.IntentDigest || receipt.AttemptID != p.AttemptID || receipt.Verifier != p.EnforcementPoint || !ports.SameOperationSubjectV3(receipt.Operation, request.Operation) || receipt.ValidatedUnixNano < permit.BegunUnixNano || receipt.ValidatedUnixNano >= p.ExpiresUnixNano {
		return ports.OperationDispatchEnforcementJournalV5{}, core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "V5 enforcement receipt changed Permit or actual-point bindings")
	}
	if s.enforcementV5[key] == nil {
		s.enforcementV5[key] = map[string]ports.OperationDispatchEnforcementJournalV5{}
	}
	current, exists := s.enforcementV5[key][request.PermitID]
	if exists {
		stored := current.Prepare
		if receipt.Phase == ports.OperationDispatchEnforcementExecuteV4 {
			stored = current.Execute
		}
		if stored != nil {
			if stored.Digest == receipt.Digest {
				return cloneOperationDispatchEnforcementJournalV5(current), nil
			}
			return ports.OperationDispatchEnforcementJournalV5{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "V5 enforcement phase already binds different content")
		}
	}
	var next ports.OperationDispatchEnforcementJournalV5
	switch receipt.Phase {
	case ports.OperationDispatchEnforcementPrepareV4:
		if exists || request.ExpectedJournalRevision != 0 {
			return ports.OperationDispatchEnforcementJournalV5{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "V5 prepare journal revision drifted")
		}
		next, err = ports.SealOperationDispatchEnforcementJournalV5(ports.OperationDispatchEnforcementJournalV5{OperationDigest: receipt.OperationDigest, EffectID: receipt.EffectID, PermitID: receipt.PermitID, AttemptID: receipt.AttemptID, SandboxAttempt: receipt.SandboxAttempt, Revision: 1, Prepare: &receipt, UpdatedUnixNano: receipt.ValidatedUnixNano})
	case ports.OperationDispatchEnforcementExecuteV4:
		if !exists || current.Revision != 1 || request.ExpectedJournalRevision != 1 || current.Prepare == nil || receipt.Prepare == nil {
			return ports.OperationDispatchEnforcementJournalV5{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "V5 execute requires exact prepare journal")
		}
		prepareRef, refErr := current.Prepare.RefV5(1)
		if refErr != nil || *receipt.Prepare != prepareRef {
			return ports.OperationDispatchEnforcementJournalV5{}, core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "V5 execute changed prepare ref")
		}
		next = cloneOperationDispatchEnforcementJournalV5(current)
		next.Revision = 2
		next.Execute = &receipt
		next.UpdatedUnixNano = receipt.ValidatedUnixNano
		next, err = ports.SealOperationDispatchEnforcementJournalV5(next)
	default:
		err = core.NewError(core.ErrorInvalidArgument, core.ReasonDispatchPermitInvalid, "V5 enforcement phase invalid")
	}
	if err != nil {
		return ports.OperationDispatchEnforcementJournalV5{}, err
	}
	s.enforcementV5[key][request.PermitID] = cloneOperationDispatchEnforcementJournalV5(next)
	s.enforcementV5CommitCount++
	if s.loseNextEnforcementV5Reply {
		s.loseNextEnforcementV5Reply = false
		return ports.OperationDispatchEnforcementJournalV5{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected V5.1 enforcement append reply loss")
	}
	return cloneOperationDispatchEnforcementJournalV5(next), nil
}

func (s *OperationEffectStoreV3) InspectOperationDispatchEnforcementV5(ctx context.Context, subject ports.OperationSubjectV3, effectID core.EffectIntentID, permitID string) (ports.OperationDispatchEnforcementJournalV5, error) {
	if err := contextError(ctx); err != nil {
		return ports.OperationDispatchEnforcementJournalV5{}, err
	}
	key, err := operationKeyV3(subject)
	if err != nil {
		return ports.OperationDispatchEnforcementJournalV5{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	journal, ok := s.enforcementV5[key][permitID]
	if !ok || journal.EffectID != effectID {
		return ports.OperationDispatchEnforcementJournalV5{}, core.NewError(core.ErrorNotFound, core.ReasonDispatchPermitInvalid, "V5.1 enforcement journal not found")
	}
	return cloneOperationDispatchEnforcementJournalV5(journal), nil
}

func cloneOperationDispatchEnforcementJournalV5(j ports.OperationDispatchEnforcementJournalV5) ports.OperationDispatchEnforcementJournalV5 {
	if j.Prepare != nil {
		v := cloneOperationDispatchEnforcementReceiptV5(*j.Prepare)
		j.Prepare = &v
	}
	if j.Execute != nil {
		v := cloneOperationDispatchEnforcementReceiptV5(*j.Execute)
		j.Execute = &v
	}
	return j
}
func cloneOperationDispatchEnforcementReceiptV5(r ports.OperationDispatchEnforcementPhaseReceiptV5) ports.OperationDispatchEnforcementPhaseReceiptV5 {
	if lease := r.Operation.ExecutionScope.SandboxLease; lease != nil {
		v := *lease
		r.Operation.ExecutionScope.SandboxLease = &v
	}
	if lease := r.Sandbox.Operation.ExecutionScope.SandboxLease; lease != nil {
		v := *lease
		r.Sandbox.Operation.ExecutionScope.SandboxLease = &v
	}
	if r.Prepare != nil {
		v := *r.Prepare
		r.Prepare = &v
	}
	if r.PreparedAttempt != nil {
		v := *r.PreparedAttempt
		r.PreparedAttempt = &v
	}
	return r
}

func cloneOperationPermitFactV5(f control.OperationDispatchPermitFactV5) control.OperationDispatchPermitFactV5 {
	if lease := f.Permit.Operation.ExecutionScope.SandboxLease; lease != nil {
		v := *lease
		f.Permit.Operation.ExecutionScope.SandboxLease = &v
	}
	if lease := f.Fence.Scope.SandboxLease; lease != nil {
		v := *lease
		f.Fence.Scope.SandboxLease = &v
	}
	return f
}

var _ control.OperationEffectDispatchFactPortV5 = (*OperationEffectStoreV3)(nil)
var _ control.OperationDispatchEnforcementFactPortV5 = (*OperationEffectStoreV3)(nil)
