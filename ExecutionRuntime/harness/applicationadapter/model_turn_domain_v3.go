// Package applicationadapter binds Application operation state to Harness-
// owned persistent Session/Candidate facts. It never executes a provider and
// never authors Runtime governance or Settlement facts.
package applicationadapter

import (
	"context"
	"time"

	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	bridgecontract "github.com/Proview-China/rax/ExecutionRuntime/harness/bridgecontract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/contract"
	harnessports "github.com/Proview-China/rax/ExecutionRuntime/harness/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type ModelTurnDomainAdapterConfigV3 struct {
	StepKind     runtimeports.NamespacedNameV2
	Adapter      runtimeports.ProviderBindingRefV2
	Bindings     harnessports.ModelTurnOperationBindingFactPortV3
	Reservations harnessports.ModelTurnOperationReservationFactPortV3
	Sessions     harnessports.SessionFactPortV2
	Candidates   harnessports.CandidateFactPortV2
	Turns        harnessports.GovernedTurnStatePortV2
	Clock        func() time.Time
}

type ModelTurnDomainAdapterV3 struct {
	config ModelTurnDomainAdapterConfigV3
}

var _ applicationports.OperationDomainStatePortV3 = (*ModelTurnDomainAdapterV3)(nil)

func NewModelTurnDomainAdapterV3(config ModelTurnDomainAdapterConfigV3) (*ModelTurnDomainAdapterV3, error) {
	if runtimeports.ValidateNamespacedNameV2(config.StepKind) != nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidNamespace, "one exact namespaced model-turn StepKind is required")
	}
	if err := config.Adapter.Validate(); err != nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonProviderBindingStale, "one exact model-turn Domain Adapter binding is required")
	}
	if config.Bindings == nil || config.Reservations == nil || config.Sessions == nil || config.Candidates == nil || config.Turns == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "model-turn reservation, binding, Session, Candidate and Turn State ports are required")
	}
	if config.Clock == nil {
		config.Clock = time.Now
	}
	return &ModelTurnDomainAdapterV3{config: config}, nil
}

func (a *ModelTurnDomainAdapterV3) ReserveOperationIntentV3(ctx context.Context, request applicationports.ReserveOperationIntentRequestV3) (applicationcontract.OperationDomainReservationRefV3, error) {
	if err := request.Validate(); err != nil {
		return applicationcontract.OperationDomainReservationRefV3{}, err
	}
	if current, err := a.config.Reservations.InspectModelTurnOperationReservationV3(ctx, request.Intent.Operation.ExecutionScope, request.StepKind, request.Attempt.ID); err == nil {
		if err := validateReservationFactForRequestWithoutCurrentV3(current, request); err != nil {
			return applicationcontract.OperationDomainReservationRefV3{}, err
		}
		return current.Reservation, nil
	} else if !core.HasCategory(err, core.ErrorNotFound) {
		return applicationcontract.OperationDomainReservationRefV3{}, err
	}
	resolved, err := a.resolveCurrentV3(ctx, request.StepKind, request.Attempt, request.Intent)
	if err != nil {
		return applicationcontract.OperationDomainReservationRefV3{}, err
	}
	session, err := a.config.Sessions.InspectSessionV2(ctx, resolved.envelope.Candidate.Run, resolved.envelope.Candidate.SessionRef)
	if err != nil {
		return applicationcontract.OperationDomainReservationRefV3{}, err
	}
	if session.Phase == contract.SessionModelDispatchReservedV2 && session.DomainReservation != nil && session.DomainReservation.AttemptID == request.Attempt.ID {
		return applicationcontract.OperationDomainReservationRefV3{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "reserved Session is missing its atomic recovery index")
	}
	if session.Phase != contract.SessionWaitingModelDispatchV2 || session.Candidate == nil || *session.Candidate != resolved.candidateRef || session.DomainReservation != nil {
		return applicationcontract.OperationDomainReservationRefV3{}, core.NewError(core.ErrorConflict, core.ReasonOwnerConflict, "model-turn Session Candidate is not available")
	}
	now := a.config.Clock()
	expires := resolved.envelope.Candidate.ExpiresUnixNano
	if request.Intent.ExpiresUnixNano < expires {
		expires = request.Intent.ExpiresUnixNano
	}
	intentDigest, _ := request.Intent.DigestV3()
	subjectDigest, err := modelTurnDomainSubjectDigestV3(resolved)
	if err != nil {
		return applicationcontract.OperationDomainReservationRefV3{}, err
	}
	reservationIDDigest, err := core.CanonicalJSONDigest("praxis.harness.application-adapter", bridgecontract.ModelTurnOperationReservationContractV3, "ModelTurnReservationIdentityV3", struct {
		Subject   core.Digest `json:"subject_digest"`
		AttemptID string      `json:"attempt_id"`
	}{subjectDigest, request.Attempt.ID})
	if err != nil {
		return applicationcontract.OperationDomainReservationRefV3{}, err
	}
	reservation, err := applicationcontract.SealOperationDomainReservationRefV3(applicationcontract.OperationDomainReservationRefV3{ContractVersion: applicationcontract.GovernedOperationAttemptContractVersionV3, ID: "model-turn-reservation:" + string(reservationIDDigest), Revision: 1, StepKind: request.StepKind, Descriptor: request.Descriptor, DomainAdapter: request.DomainAdapter, AttemptID: request.Attempt.ID, AttemptRevision: request.Attempt.Revision, AttemptDigest: request.Attempt.Digest, IntentDigest: intentDigest, DomainSubjectDigest: subjectDigest, SessionRef: session.ID, CandidateDigest: resolved.candidateRef.Digest, ReservedUnixNano: now.UnixNano(), ExpiresUnixNano: expires})
	if err != nil {
		return applicationcontract.OperationDomainReservationRefV3{}, err
	}
	local := contract.ModelDispatchReservationRefV2{ID: reservation.ID, Digest: reservation.Digest, AttemptID: reservation.AttemptID, IntentDigest: reservation.IntentDigest, CandidateDigest: reservation.CandidateDigest, ReservedUnixNano: reservation.ReservedUnixNano, ExpiresUnixNano: reservation.ExpiresUnixNano}
	next := session
	next.Revision++
	next.Phase = contract.SessionModelDispatchReservedV2
	next.DomainReservation = &local
	next.UpdatedUnixNano = reservation.ReservedUnixNano
	fact := bridgecontract.ModelTurnOperationReservationFactV3{ContractVersion: bridgecontract.ModelTurnOperationReservationContractV3, Scope: resolved.envelope.Candidate.Run.Scope, StepKind: request.StepKind, Run: resolved.envelope.Candidate.Run, SessionID: session.ID, SessionRevision: next.Revision, Candidate: resolved.candidateRef, Application: cloneApplicationAttemptV3(request.Attempt), Reservation: reservation}
	committed, err := a.config.Reservations.CommitModelTurnOperationReservationV3(ctx, harnessports.CommitModelTurnOperationReservationRequestV3{ExpectedSessionRevision: session.Revision, NextSession: next, Reservation: fact})
	if err != nil {
		if !core.HasCategory(err, core.ErrorUnavailable) && !core.HasCategory(err, core.ErrorConflict) {
			return applicationcontract.OperationDomainReservationRefV3{}, err
		}
		stored, inspectErr := a.config.Reservations.InspectModelTurnOperationReservationV3(context.WithoutCancel(ctx), fact.Scope, fact.StepKind, fact.Application.ID)
		if inspectErr != nil {
			return applicationcontract.OperationDomainReservationRefV3{}, err
		}
		committed.Reservation = stored
		committed.Session, inspectErr = a.config.Sessions.InspectSessionV2(context.WithoutCancel(ctx), fact.Run, fact.SessionID)
		if inspectErr != nil {
			return applicationcontract.OperationDomainReservationRefV3{}, inspectErr
		}
	}
	if err := validateReservationFactForRequestV3(committed.Reservation, request, resolved); err != nil {
		return applicationcontract.OperationDomainReservationRefV3{}, err
	}
	if committed.Session.Phase != contract.SessionModelDispatchReservedV2 || committed.Session.Candidate == nil || *committed.Session.Candidate != resolved.candidateRef || !sameLocalReservationV3(committed.Session.DomainReservation, &local) {
		return applicationcontract.OperationDomainReservationRefV3{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "atomic reservation commit returned another Session projection")
	}
	return committed.Reservation.Reservation, nil
}

func (a *ModelTurnDomainAdapterV3) InspectOperationIntentReservationV3(ctx context.Context, request applicationports.InspectOperationIntentReservationRequestV3) (applicationcontract.OperationDomainReservationRefV3, error) {
	if err := request.Validate(); err != nil {
		return applicationcontract.OperationDomainReservationRefV3{}, err
	}
	if err := a.requireStepV3(request.StepKind); err != nil {
		return applicationcontract.OperationDomainReservationRefV3{}, err
	}
	if request.DomainAdapter != a.config.Adapter {
		return applicationcontract.OperationDomainReservationRefV3{}, core.NewError(core.ErrorForbidden, core.ReasonProviderBindingStale, "reservation belongs to another Domain Adapter")
	}
	fact, err := a.config.Reservations.InspectModelTurnOperationReservationV3(ctx, request.Scope, request.StepKind, request.AttemptID)
	if err != nil {
		return applicationcontract.OperationDomainReservationRefV3{}, err
	}
	if err := fact.Validate(); err != nil {
		return applicationcontract.OperationDomainReservationRefV3{}, err
	}
	if !runtimeports.SameExecutionScopeV2(fact.Scope, request.Scope) || fact.StepKind != request.StepKind || fact.Application.ID != request.AttemptID || fact.Reservation.DomainAdapter != request.DomainAdapter {
		return applicationcontract.OperationDomainReservationRefV3{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "reservation inspection returned another scope, attempt or adapter")
	}
	return fact.Reservation, nil
}

func (a *ModelTurnDomainAdapterV3) BindPrepared(ctx context.Context, request applicationports.BindPreparedOperationRequestV3) (applicationports.OperationDomainStateRefV3, error) {
	if err := request.Validate(); err != nil {
		return applicationports.OperationDomainStateRefV3{}, err
	}
	resolved, err := a.resolveHistoricalV3(ctx, request.StepKind, request.Attempt, request.Intent)
	if err != nil {
		return applicationports.OperationDomainStateRefV3{}, err
	}
	if err := validateDelegationCandidateV3(request.DelegationFact, resolved, request.RuntimeAttempt); err != nil {
		return applicationports.OperationDomainStateRefV3{}, err
	}
	basis, err := preparedBasisDigestV3(request)
	if err != nil {
		return applicationports.OperationDomainStateRefV3{}, err
	}
	if current, ok, err := a.exactCurrentV3(ctx, resolved, request.Attempt, bridgecontract.ModelTurnOperationPreparedV3, basis); err != nil || ok {
		return current, err
	}
	session, err := a.ensurePreparedSessionV3(ctx, resolved, request.Attempt, request.RuntimeAttempt)
	if err != nil {
		return applicationports.OperationDomainStateRefV3{}, err
	}
	runtimeAttempt := cloneRuntimeAttemptV3(request.RuntimeAttempt)
	delegationFact := cloneDelegationFactV3(request.DelegationFact)
	fact := bridgecontract.ModelTurnOperationBindingFactV3{
		ContractVersion: bridgecontract.ModelTurnOperationBindingContractV3, ID: request.Attempt.ID, Revision: 1,
		State: bridgecontract.ModelTurnOperationPreparedV3, StepKind: request.StepKind, Scope: resolved.envelope.Candidate.Run.Scope,
		ScopeDigest: resolved.scopeDigest, Run: resolved.envelope.Candidate.Run, SessionID: resolved.envelope.Candidate.SessionRef,
		SessionRevision: session.Revision, Candidate: resolved.candidateRef, Provider: resolved.envelope.Candidate.Provider,
		ApplicationAttempt: cloneApplicationAttemptV3(request.Attempt), RuntimeAttempt: &runtimeAttempt, DelegationFact: &delegationFact, BasisDigest: basis,
		CreatedUnixNano: session.UpdatedUnixNano, UpdatedUnixNano: session.UpdatedUnixNano,
	}
	stored, err := a.createBindingV3(ctx, fact)
	if err != nil {
		return applicationports.OperationDomainStateRefV3{}, err
	}
	return domainRefV3(stored)
}

func (a *ModelTurnDomainAdapterV3) MarkUnknown(ctx context.Context, request applicationports.MarkUnknownOperationRequestV3) (applicationports.OperationDomainStateRefV3, error) {
	if err := request.Validate(); err != nil {
		return applicationports.OperationDomainStateRefV3{}, err
	}
	resolved, err := a.resolveHistoricalV3(ctx, request.StepKind, request.Attempt, request.Intent)
	if err != nil {
		return applicationports.OperationDomainStateRefV3{}, err
	}
	basis, err := unknownBasisDigestV3(request)
	if err != nil {
		return applicationports.OperationDomainStateRefV3{}, err
	}
	if current, ok, err := a.exactCurrentV3(ctx, resolved, request.Attempt, bridgecontract.ModelTurnOperationUnknownV3, basis); err != nil || ok {
		return current, err
	}
	current, err := a.requireCurrentV3(ctx, resolved, bridgecontract.ModelTurnOperationPreparedV3)
	if err != nil {
		return applicationports.OperationDomainStateRefV3{}, err
	}
	if !samePreparedRuntimeV3(current.RuntimeAttempt, &request.RuntimeAttempt) {
		return applicationports.OperationDomainStateRefV3{}, core.NewError(core.ErrorConflict, core.ReasonEffectUnknownOutcome, "unknown request changed the prepared Runtime attempt")
	}
	session, err := a.ensureReconcilingSessionV3(ctx, current, request.RuntimeAttempt)
	if err != nil {
		return applicationports.OperationDomainStateRefV3{}, err
	}
	next := current
	next.Revision++
	next.State = bridgecontract.ModelTurnOperationUnknownV3
	next.ApplicationAttempt = cloneApplicationAttemptV3(request.Attempt)
	runtimeAttempt := cloneRuntimeAttemptV3(request.RuntimeAttempt)
	next.RuntimeAttempt = &runtimeAttempt
	authorization := request.Authorization
	next.UnknownAuthorization = &authorization
	next.SessionRevision = session.Revision
	next.BasisDigest = basis
	next.UpdatedUnixNano = session.UpdatedUnixNano
	stored, err := a.casBindingV3(ctx, current, next)
	if err != nil {
		return applicationports.OperationDomainStateRefV3{}, err
	}
	return domainRefV3(stored)
}

func (a *ModelTurnDomainAdapterV3) BindObserved(ctx context.Context, request applicationports.BindObservedOperationRequestV3) (applicationports.OperationDomainStateRefV3, error) {
	if err := request.Validate(); err != nil {
		return applicationports.OperationDomainStateRefV3{}, err
	}
	resolved, err := a.resolveHistoricalV3(ctx, request.StepKind, request.Attempt, request.Intent)
	if err != nil {
		return applicationports.OperationDomainStateRefV3{}, err
	}
	basis, err := observedBasisDigestV3(request)
	if err != nil {
		return applicationports.OperationDomainStateRefV3{}, err
	}
	if result, ok, err := a.exactCurrentV3(ctx, resolved, request.Attempt, bridgecontract.ModelTurnOperationObservedV3, basis); err != nil || ok {
		return result, err
	}
	current, err := a.requireCurrentV3(ctx, resolved, bridgecontract.ModelTurnOperationPreparedV3)
	if err != nil {
		return applicationports.OperationDomainStateRefV3{}, err
	}
	if !samePreparedRuntimeV3(current.RuntimeAttempt, &request.RuntimeAttempt) {
		return applicationports.OperationDomainStateRefV3{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Observation changed the prepared Runtime attempt")
	}
	session, err := a.ensureObservedSessionV3(ctx, current, request.RuntimeAttempt)
	if err != nil {
		return applicationports.OperationDomainStateRefV3{}, err
	}
	next := current
	next.Revision++
	next.State = bridgecontract.ModelTurnOperationObservedV3
	next.ApplicationAttempt = cloneApplicationAttemptV3(request.Attempt)
	runtimeAttempt := cloneRuntimeAttemptV3(request.RuntimeAttempt)
	next.RuntimeAttempt = &runtimeAttempt
	next.SessionRevision = session.Revision
	next.BasisDigest = basis
	next.UpdatedUnixNano = session.UpdatedUnixNano
	stored, err := a.casBindingV3(ctx, current, next)
	if err != nil {
		return applicationports.OperationDomainStateRefV3{}, err
	}
	return domainRefV3(stored)
}

func (a *ModelTurnDomainAdapterV3) ApplySettlement(ctx context.Context, request applicationports.ApplyOperationSettlementRequestV3) (applicationports.OperationDomainStateRefV3, error) {
	if err := request.Validate(); err != nil {
		return applicationports.OperationDomainStateRefV3{}, err
	}
	resolved, err := a.resolveHistoricalV3(ctx, request.StepKind, request.Attempt, request.Intent)
	if err != nil {
		return applicationports.OperationDomainStateRefV3{}, err
	}
	if request.DomainResult == nil {
		return applicationports.OperationDomainStateRefV3{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonSettlementOwnerMismatch, "model-turn settlement requires a schema-bound DomainResult")
	}
	if err := validateSettlementAgainstIntentV3(request.Settlement, request.Intent); err != nil {
		return applicationports.OperationDomainStateRefV3{}, err
	}
	basis, err := settlementBasisDigestV3(request)
	if err != nil {
		return applicationports.OperationDomainStateRefV3{}, err
	}
	if result, ok, err := a.exactCurrentV3(ctx, resolved, request.Attempt, bridgecontract.ModelTurnOperationSettledV3, basis); err != nil || ok {
		return result, err
	}
	if request.RuntimeAttempt == nil {
		return a.applyUndispatchedSettlementV3(ctx, resolved, request, basis)
	}
	current, err := a.requireCurrentEitherV3(ctx, resolved, bridgecontract.ModelTurnOperationObservedV3, bridgecontract.ModelTurnOperationUnknownV3)
	if err != nil {
		return applicationports.OperationDomainStateRefV3{}, err
	}
	if !samePreparedRuntimeV3(current.RuntimeAttempt, request.RuntimeAttempt) {
		return applicationports.OperationDomainStateRefV3{}, core.NewError(core.ErrorConflict, core.ReasonSettlementOwnerMismatch, "Settlement changed the prepared Runtime attempt")
	}
	runtimeAttempt := cloneRuntimeAttemptV3(*request.RuntimeAttempt)
	settlement := cloneSettlementV3(request.Settlement)
	runtimeAttempt.Settlement = &settlement
	session, err := a.ensureSettledSessionV3(ctx, current, runtimeAttempt, *request.DomainResult)
	if err != nil {
		return applicationports.OperationDomainStateRefV3{}, err
	}
	next := current
	next.Revision++
	next.State = bridgecontract.ModelTurnOperationSettledV3
	next.ApplicationAttempt = cloneApplicationAttemptV3(request.Attempt)
	next.RuntimeAttempt = &runtimeAttempt
	next.Settlement = &settlement
	domainResult := cloneOpaqueV3(*request.DomainResult)
	next.DomainResult = &domainResult
	next.SessionRevision = session.Revision
	next.BasisDigest = basis
	next.UpdatedUnixNano = session.UpdatedUnixNano
	stored, err := a.casBindingV3(ctx, current, next)
	if err != nil {
		return applicationports.OperationDomainStateRefV3{}, err
	}
	return domainRefV3(stored)
}

func (a *ModelTurnDomainAdapterV3) InspectOperationDomainStateV3(ctx context.Context, request applicationports.OperationDomainInspectRequestV3) (applicationports.OperationDomainStateRefV3, error) {
	if err := request.Validate(); err != nil {
		return applicationports.OperationDomainStateRefV3{}, err
	}
	if err := a.requireStepV3(request.StepKind); err != nil {
		return applicationports.OperationDomainStateRefV3{}, err
	}
	fact, err := a.config.Bindings.InspectModelTurnOperationBindingV3(ctx, request.Scope, request.StepKind, request.AttemptID)
	if err != nil {
		return applicationports.OperationDomainStateRefV3{}, err
	}
	return domainRefV3(fact)
}

type resolvedModelTurnV3 struct {
	envelope     contract.ModelTurnEffectEnvelopeV2
	candidateRef contract.CandidateRefV2
	scopeDigest  core.Digest
	attemptID    string
}

func (a *ModelTurnDomainAdapterV3) resolveHistoricalV3(ctx context.Context, step runtimeports.NamespacedNameV2, attempt applicationcontract.GovernedOperationAttemptRefV3, intent runtimeports.OperationEffectIntentV3) (resolvedModelTurnV3, error) {
	if err := a.requireStepV3(step); err != nil {
		return resolvedModelTurnV3{}, err
	}
	if attempt.DomainAdapter != a.config.Adapter {
		return resolvedModelTurnV3{}, core.NewError(core.ErrorForbidden, core.ReasonProviderBindingStale, "Application attempt is routed to another Domain Adapter binding")
	}
	envelope, err := contract.DecodeModelTurnEffectPayloadV2(intent.Payload, time.Time{})
	if err != nil {
		return resolvedModelTurnV3{}, err
	}
	candidate := envelope.Candidate
	if candidate.Endpoint.Binding != a.config.Adapter {
		return resolvedModelTurnV3{}, core.NewError(core.ErrorConflict, core.ReasonProviderBindingStale, "model-turn Candidate endpoint is owned by another Domain Adapter binding")
	}
	scopeDigest, err := runtimeports.ExecutionScopeDigestV2(candidate.Run.Scope)
	if err != nil {
		return resolvedModelTurnV3{}, err
	}
	operationDigest, err := intent.Operation.DigestV3()
	if err != nil {
		return resolvedModelTurnV3{}, err
	}
	if intent.Operation.Kind != runtimeports.OperationScopeRunV3 || intent.Operation.RunID != candidate.Run.RunID || !sameScopeV3(intent.Operation.ExecutionScope, candidate.Run.Scope) || scopeDigest != attempt.ScopeDigest || operationDigest != attempt.OperationDigest || intent.ID != attempt.EffectID || candidate.Provider != intent.Provider {
		return resolvedModelTurnV3{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "model-turn Intent, Run, scope, provider and Application attempt are not exact")
	}
	ref, err := candidate.RefV2()
	if err != nil {
		return resolvedModelTurnV3{}, err
	}
	persisted, err := a.config.Candidates.InspectCandidateV2(ctx, candidate.Run, candidate.ID)
	if err != nil {
		return resolvedModelTurnV3{}, err
	}
	persistedRef, err := persisted.RefV2()
	if err != nil || persistedRef != ref {
		return resolvedModelTurnV3{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "model-turn Effect envelope does not match the persisted Candidate")
	}
	return resolvedModelTurnV3{envelope: envelope, candidateRef: ref, scopeDigest: scopeDigest, attemptID: attempt.ID}, nil
}

func (a *ModelTurnDomainAdapterV3) resolveCurrentV3(ctx context.Context, step runtimeports.NamespacedNameV2, attempt applicationcontract.GovernedOperationAttemptRefV3, intent runtimeports.OperationEffectIntentV3) (resolvedModelTurnV3, error) {
	now := a.config.Clock()
	resolved, err := a.resolveHistoricalV3(ctx, step, attempt, intent)
	if err != nil {
		return resolvedModelTurnV3{}, err
	}
	if now.IsZero() || !now.Before(time.Unix(0, intent.ExpiresUnixNano)) {
		return resolvedModelTurnV3{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonCapabilityExpired, "model-turn Intent expired before reservation")
	}
	if err := resolved.envelope.Candidate.Validate(now); err != nil {
		return resolvedModelTurnV3{}, err
	}
	return resolved, nil
}

func modelTurnDomainSubjectDigestV3(resolved resolvedModelTurnV3) (core.Digest, error) {
	return core.CanonicalJSONDigest("praxis.harness.application-adapter", bridgecontract.ModelTurnOperationReservationContractV3, "ModelTurnDomainSubjectV3", struct {
		Run       contract.RunRef         `json:"run"`
		SessionID string                  `json:"session_id"`
		Candidate contract.CandidateRefV2 `json:"candidate"`
		Endpoint  core.Digest             `json:"endpoint_identity_digest"`
	}{resolved.envelope.Candidate.Run, resolved.envelope.Candidate.SessionRef, resolved.candidateRef, resolved.envelope.Candidate.Endpoint.IdentityDigest})
}

func validateReservationFactForRequestV3(fact bridgecontract.ModelTurnOperationReservationFactV3, request applicationports.ReserveOperationIntentRequestV3, resolved resolvedModelTurnV3) error {
	if err := validateReservationFactForRequestWithoutCurrentV3(fact, request); err != nil {
		return err
	}
	if fact.SessionID != resolved.envelope.Candidate.SessionRef || fact.Candidate != resolved.candidateRef {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "model-turn reservation recovery changed Session or Candidate")
	}
	expected, err := modelTurnDomainSubjectDigestV3(resolved)
	if err != nil {
		return err
	}
	if fact.Reservation.DomainSubjectDigest != expected {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "model-turn reservation subject digest drifted")
	}
	return nil
}

func validateReservationFactForRequestWithoutCurrentV3(fact bridgecontract.ModelTurnOperationReservationFactV3, request applicationports.ReserveOperationIntentRequestV3) error {
	if err := fact.Validate(); err != nil {
		return err
	}
	if err := applicationports.ValidateOperationDomainReservationForV3(fact.Reservation, request); err != nil {
		return err
	}
	if !runtimeports.SameExecutionScopeV2(fact.Scope, request.Intent.Operation.ExecutionScope) || fact.StepKind != request.StepKind || fact.Application.ID != request.Attempt.ID || fact.Reservation.DomainAdapter != request.DomainAdapter {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "model-turn reservation recovery changed scope, attempt or adapter")
	}
	return nil
}

func localReservationRefV3(value applicationcontract.OperationDomainReservationRefV3) contract.ModelDispatchReservationRefV2 {
	return contract.ModelDispatchReservationRefV2{ID: value.ID, Digest: value.Digest, AttemptID: value.AttemptID, IntentDigest: value.IntentDigest, CandidateDigest: value.CandidateDigest, ReservedUnixNano: value.ReservedUnixNano, ExpiresUnixNano: value.ExpiresUnixNano}
}
func sameLocalReservationV3(left, right *contract.ModelDispatchReservationRefV2) bool {
	return left != nil && right != nil && *left == *right
}

func (a *ModelTurnDomainAdapterV3) requireStepV3(step runtimeports.NamespacedNameV2) error {
	if step != a.config.StepKind {
		return core.NewError(core.ErrorForbidden, core.ReasonUnknownCapability, "model-turn Adapter rejects every unconfigured StepKind")
	}
	return nil
}

func (a *ModelTurnDomainAdapterV3) ensurePreparedSessionV3(ctx context.Context, resolved resolvedModelTurnV3, applicationAttempt applicationcontract.GovernedOperationAttemptRefV3, attempt runtimeports.GovernedExecutionAttemptRefsV2) (contract.GovernedSessionV2, error) {
	if applicationAttempt.DomainReservation == nil {
		return contract.GovernedSessionV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidReference, "prepared model-turn attempt lacks its domain reservation")
	}
	reservationFact, err := a.config.Reservations.InspectModelTurnOperationReservationV3(ctx, resolved.envelope.Candidate.Run.Scope, a.config.StepKind, applicationAttempt.ID)
	if err != nil {
		return contract.GovernedSessionV2{}, err
	}
	if err := reservationFact.Validate(); err != nil {
		return contract.GovernedSessionV2{}, err
	}
	if !runtimeports.SameExecutionScopeV2(reservationFact.Scope, resolved.envelope.Candidate.Run.Scope) || reservationFact.StepKind != a.config.StepKind || reservationFact.SessionID != resolved.envelope.Candidate.SessionRef || reservationFact.Candidate != resolved.candidateRef || reservationFact.Reservation.DomainAdapter != a.config.Adapter || reservationFact.Reservation.ID != applicationAttempt.DomainReservation.ID || reservationFact.Reservation.Digest != applicationAttempt.DomainReservation.Digest {
		return contract.GovernedSessionV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "prepared model-turn attempt changed its reservation")
	}
	local := localReservationRefV3(reservationFact.Reservation)
	session, err := a.config.Sessions.InspectSessionV2(ctx, resolved.envelope.Candidate.Run, resolved.envelope.Candidate.SessionRef)
	if err != nil {
		return contract.GovernedSessionV2{}, err
	}
	if session.Phase == contract.SessionModelInFlightV2 && session.Candidate != nil && *session.Candidate == resolved.candidateRef && sameLocalReservationV3(session.DomainReservation, &local) && sameRuntimeAttemptV3(session.Execution, &attempt) {
		return session, nil
	}
	if session.Phase != contract.SessionModelDispatchReservedV2 || session.Candidate == nil || *session.Candidate != resolved.candidateRef || !sameLocalReservationV3(session.DomainReservation, &local) {
		return contract.GovernedSessionV2{}, core.NewError(core.ErrorConflict, core.ReasonInvalidState, "model-turn Session is not at the exact reservation")
	}
	stored, callErr := a.config.Turns.AttachPreparedAttemptV2(ctx, harnessports.AttachPreparedAttemptRequestV2{
		Run: resolved.envelope.Candidate.Run, SessionID: session.ID, ExpectedSessionRevision: session.Revision,
		Candidate: resolved.candidateRef, Reservation: local, Attempt: attempt, UpdatedUnixNano: a.nowV3(),
	})
	if callErr == nil {
		return stored, nil
	}
	inspected, inspectErr := a.config.Sessions.InspectSessionV2(context.WithoutCancel(ctx), resolved.envelope.Candidate.Run, session.ID)
	if inspectErr == nil && inspected.Phase == contract.SessionModelInFlightV2 && inspected.Candidate != nil && *inspected.Candidate == resolved.candidateRef && sameLocalReservationV3(inspected.DomainReservation, &local) && sameRuntimeAttemptV3(inspected.Execution, &attempt) {
		return inspected, nil
	}
	return contract.GovernedSessionV2{}, callErr
}

func (a *ModelTurnDomainAdapterV3) ensureReconcilingSessionV3(ctx context.Context, binding bridgecontract.ModelTurnOperationBindingFactV3, attempt runtimeports.GovernedExecutionAttemptRefsV2) (contract.GovernedSessionV2, error) {
	session, err := a.config.Sessions.InspectSessionV2(ctx, binding.Run, binding.SessionID)
	if err != nil {
		return contract.GovernedSessionV2{}, err
	}
	if session.Phase == contract.SessionReconcilingV2 && samePreparedRuntimeV3(session.Execution, &attempt) {
		return session, nil
	}
	if session.Phase != contract.SessionModelInFlightV2 || !samePreparedRuntimeV3(session.Execution, &attempt) {
		return contract.GovernedSessionV2{}, core.NewError(core.ErrorConflict, core.ReasonEffectUnknownOutcome, "only the exact prepared Session may enter reconciliation")
	}
	stored, callErr := a.config.Turns.MarkAttemptReconcilingV2(ctx, harnessports.MarkAttemptReconcilingRequestV2{Run: binding.Run, SessionID: binding.SessionID, ExpectedSessionRevision: session.Revision, UpdatedUnixNano: a.nowV3()})
	if callErr == nil {
		return stored, nil
	}
	inspected, inspectErr := a.config.Sessions.InspectSessionV2(context.WithoutCancel(ctx), binding.Run, binding.SessionID)
	if inspectErr == nil && inspected.Phase == contract.SessionReconcilingV2 && samePreparedRuntimeV3(inspected.Execution, &attempt) {
		return inspected, nil
	}
	return contract.GovernedSessionV2{}, callErr
}

func (a *ModelTurnDomainAdapterV3) ensureObservedSessionV3(ctx context.Context, binding bridgecontract.ModelTurnOperationBindingFactV3, attempt runtimeports.GovernedExecutionAttemptRefsV2) (contract.GovernedSessionV2, error) {
	session, err := a.config.Sessions.InspectSessionV2(ctx, binding.Run, binding.SessionID)
	if err != nil {
		return contract.GovernedSessionV2{}, err
	}
	if session.Phase == contract.SessionWaitingSettlementV2 && sameRuntimeAttemptV3(session.Execution, &attempt) {
		return session, nil
	}
	if session.Revision != binding.SessionRevision || session.Phase != contract.SessionModelInFlightV2 && session.Phase != contract.SessionReconcilingV2 || !samePreparedRuntimeV3(session.Execution, &attempt) {
		return contract.GovernedSessionV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Observation does not descend from the bound prepared Session")
	}
	stored, callErr := a.config.Turns.AttachObservedAttemptV2(ctx, harnessports.AttachObservedAttemptRequestV2{Run: binding.Run, SessionID: binding.SessionID, ExpectedSessionRevision: session.Revision, Attempt: attempt, UpdatedUnixNano: a.nowV3()})
	if callErr == nil {
		return stored, nil
	}
	inspected, inspectErr := a.config.Sessions.InspectSessionV2(context.WithoutCancel(ctx), binding.Run, binding.SessionID)
	if inspectErr == nil && inspected.Phase == contract.SessionWaitingSettlementV2 && sameRuntimeAttemptV3(inspected.Execution, &attempt) {
		return inspected, nil
	}
	return contract.GovernedSessionV2{}, callErr
}

func (a *ModelTurnDomainAdapterV3) ensureSettledSessionV3(ctx context.Context, binding bridgecontract.ModelTurnOperationBindingFactV3, attempt runtimeports.GovernedExecutionAttemptRefsV2, domain runtimeports.OpaquePayloadV2) (contract.GovernedSessionV2, error) {
	session, err := a.config.Sessions.InspectSessionV2(ctx, binding.Run, binding.SessionID)
	if err != nil {
		return contract.GovernedSessionV2{}, err
	}
	if session.Revision == binding.SessionRevision+1 && isSettledSessionPhaseV3(session.Phase) && sameRuntimeAttemptV3(session.Execution, &attempt) {
		return session, nil
	}
	if session.Revision != binding.SessionRevision || session.Phase != contract.SessionWaitingSettlementV2 && session.Phase != contract.SessionReconcilingV2 {
		return contract.GovernedSessionV2{}, core.NewError(core.ErrorConflict, core.ReasonSettlementOwnerMismatch, "Settlement does not descend from the bound Session watermark")
	}
	stored, callErr := a.config.Turns.ApplySettledTurnV2(ctx, harnessports.ApplySettledTurnRequestV2{Run: binding.Run, SessionID: binding.SessionID, ExpectedSessionRevision: session.Revision, Attempt: attempt, DomainResult: domain, UpdatedUnixNano: a.nowV3()})
	if callErr == nil {
		return stored, nil
	}
	inspected, inspectErr := a.config.Sessions.InspectSessionV2(context.WithoutCancel(ctx), binding.Run, binding.SessionID)
	if inspectErr == nil && inspected.Revision == binding.SessionRevision+1 && sameRuntimeAttemptV3(inspected.Execution, &attempt) {
		return inspected, nil
	}
	return contract.GovernedSessionV2{}, callErr
}

func (a *ModelTurnDomainAdapterV3) applyUndispatchedSettlementV3(ctx context.Context, resolved resolvedModelTurnV3, request applicationports.ApplyOperationSettlementRequestV3, basis core.Digest) (applicationports.OperationDomainStateRefV3, error) {
	result, err := contract.DecodeSettledTurnDomainResultV2(*request.DomainResult)
	if err != nil {
		return applicationports.OperationDomainStateRefV3{}, err
	}
	if result.Candidate != resolved.candidateRef || result.State != contract.SettledTurnFailedV2 {
		return applicationports.OperationDomainStateRefV3{}, core.NewError(core.ErrorConflict, core.ReasonEffectUnknownOutcome, "undispatched settlement may only fail the exact Candidate")
	}
	session, err := a.config.Sessions.InspectSessionV2(ctx, resolved.envelope.Candidate.Run, resolved.envelope.Candidate.SessionRef)
	if err != nil {
		return applicationports.OperationDomainStateRefV3{}, err
	}
	if session.Phase != contract.SessionTerminalV2 || session.UndispatchedSettlement == nil {
		if request.Attempt.DomainReservation == nil {
			return applicationports.OperationDomainStateRefV3{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidReference, "undispatched settlement lacks its domain reservation")
		}
		reservation, inspectErr := a.config.Reservations.InspectModelTurnOperationReservationV3(ctx, resolved.envelope.Candidate.Run.Scope, request.StepKind, request.Attempt.ID)
		if inspectErr != nil {
			return applicationports.OperationDomainStateRefV3{}, inspectErr
		}
		if err := reservation.Validate(); err != nil {
			return applicationports.OperationDomainStateRefV3{}, err
		}
		local := localReservationRefV3(reservation.Reservation)
		if !runtimeports.SameExecutionScopeV2(reservation.Scope, resolved.envelope.Candidate.Run.Scope) || reservation.StepKind != request.StepKind || reservation.SessionID != session.ID || reservation.Candidate != resolved.candidateRef || reservation.Reservation.DomainAdapter != a.config.Adapter || reservation.Reservation.Digest != request.Attempt.DomainReservation.Digest || session.Phase != contract.SessionModelDispatchReservedV2 || session.Candidate == nil || *session.Candidate != resolved.candidateRef || !sameLocalReservationV3(session.DomainReservation, &local) {
			return applicationports.OperationDomainStateRefV3{}, core.NewError(core.ErrorConflict, core.ReasonEffectUnknownOutcome, "undispatched settlement does not descend from the exact reserved Candidate")
		}
		stored, callErr := a.config.Turns.ApplyUndispatchedSettlementV2(ctx, harnessports.ApplyUndispatchedSettlementRequestV2{
			Run: resolved.envelope.Candidate.Run, SessionID: session.ID, ExpectedSessionRevision: session.Revision,
			Candidate: resolved.candidateRef, Settlement: request.Settlement, DomainResult: *request.DomainResult, UpdatedUnixNano: a.nowV3(),
		})
		if callErr != nil {
			stored, err = a.config.Sessions.InspectSessionV2(context.WithoutCancel(ctx), resolved.envelope.Candidate.Run, session.ID)
			if err != nil || !sameUndispatchedSessionV3(stored, resolved.candidateRef, request.Settlement, *request.DomainResult) {
				return applicationports.OperationDomainStateRefV3{}, callErr
			}
		}
		session = stored
	}
	if !sameUndispatchedSessionV3(session, resolved.candidateRef, request.Settlement, *request.DomainResult) {
		return applicationports.OperationDomainStateRefV3{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "undispatched terminal Session differs from exact Settlement")
	}
	settlement := cloneSettlementV3(request.Settlement)
	domain := cloneOpaqueV3(*request.DomainResult)
	fact := bridgecontract.ModelTurnOperationBindingFactV3{
		ContractVersion: bridgecontract.ModelTurnOperationBindingContractV3, ID: request.Attempt.ID, Revision: 1,
		State: bridgecontract.ModelTurnOperationSettledV3, StepKind: request.StepKind, Scope: resolved.envelope.Candidate.Run.Scope,
		ScopeDigest: resolved.scopeDigest, Run: resolved.envelope.Candidate.Run, SessionID: resolved.envelope.Candidate.SessionRef,
		SessionRevision: session.Revision, Candidate: resolved.candidateRef, Provider: resolved.envelope.Candidate.Provider,
		ApplicationAttempt: cloneApplicationAttemptV3(request.Attempt), Settlement: &settlement, DomainResult: &domain, BasisDigest: basis,
		CreatedUnixNano: session.UpdatedUnixNano, UpdatedUnixNano: session.UpdatedUnixNano,
	}
	stored, err := a.createBindingV3(ctx, fact)
	if err != nil {
		return applicationports.OperationDomainStateRefV3{}, err
	}
	return domainRefV3(stored)
}

func (a *ModelTurnDomainAdapterV3) exactCurrentV3(ctx context.Context, resolved resolvedModelTurnV3, attempt applicationcontract.GovernedOperationAttemptRefV3, state bridgecontract.ModelTurnOperationBindingStateV3, basis core.Digest) (applicationports.OperationDomainStateRefV3, bool, error) {
	current, err := a.config.Bindings.InspectModelTurnOperationBindingV3(ctx, resolved.envelope.Candidate.Run.Scope, a.config.StepKind, attempt.ID)
	if core.HasCategory(err, core.ErrorNotFound) {
		return applicationports.OperationDomainStateRefV3{}, false, nil
	}
	if err != nil {
		return applicationports.OperationDomainStateRefV3{}, false, err
	}
	if current.State != state {
		return applicationports.OperationDomainStateRefV3{}, false, nil
	}
	if !sameApplicationAttemptV3(current.ApplicationAttempt, attempt) || current.BasisDigest != basis || current.Candidate != resolved.candidateRef || current.Provider != resolved.envelope.Candidate.Provider {
		return applicationports.OperationDomainStateRefV3{}, false, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "model-turn domain replay differs from persisted exact basis")
	}
	ref, err := domainRefV3(current)
	return ref, err == nil, err
}

func (a *ModelTurnDomainAdapterV3) requireCurrentV3(ctx context.Context, resolved resolvedModelTurnV3, state bridgecontract.ModelTurnOperationBindingStateV3) (bridgecontract.ModelTurnOperationBindingFactV3, error) {
	return a.requireCurrentEitherV3(ctx, resolved, state, "")
}

func (a *ModelTurnDomainAdapterV3) requireCurrentEitherV3(ctx context.Context, resolved resolvedModelTurnV3, first, second bridgecontract.ModelTurnOperationBindingStateV3) (bridgecontract.ModelTurnOperationBindingFactV3, error) {
	current, err := a.config.Bindings.InspectModelTurnOperationBindingV3(ctx, resolved.envelope.Candidate.Run.Scope, a.config.StepKind, resolved.attemptID)
	if err != nil {
		return bridgecontract.ModelTurnOperationBindingFactV3{}, err
	}
	if current.State != first && (second == "" || current.State != second) || current.Candidate != resolved.candidateRef || current.Provider != resolved.envelope.Candidate.Provider {
		return bridgecontract.ModelTurnOperationBindingFactV3{}, core.NewError(core.ErrorConflict, core.ReasonInvalidState, "model-turn operation binding is not at the required predecessor")
	}
	return current, nil
}

func (a *ModelTurnDomainAdapterV3) createBindingV3(ctx context.Context, fact bridgecontract.ModelTurnOperationBindingFactV3) (bridgecontract.ModelTurnOperationBindingFactV3, error) {
	stored, err := a.config.Bindings.CreateModelTurnOperationBindingV3(ctx, fact)
	if err == nil {
		return stored, nil
	}
	inspected, inspectErr := a.config.Bindings.InspectModelTurnOperationBindingV3(context.WithoutCancel(ctx), fact.Scope, fact.StepKind, fact.ID)
	if inspectErr == nil && sameBindingFactV3(inspected, fact) {
		return inspected, nil
	}
	return bridgecontract.ModelTurnOperationBindingFactV3{}, err
}

func (a *ModelTurnDomainAdapterV3) casBindingV3(ctx context.Context, current, next bridgecontract.ModelTurnOperationBindingFactV3) (bridgecontract.ModelTurnOperationBindingFactV3, error) {
	stored, err := a.config.Bindings.CompareAndSwapModelTurnOperationBindingV3(ctx, harnessports.ModelTurnOperationBindingCASRequestV3{Scope: current.Scope, StepKind: current.StepKind, ID: current.ID, ExpectedRevision: current.Revision, Next: next})
	if err == nil {
		return stored, nil
	}
	inspected, inspectErr := a.config.Bindings.InspectModelTurnOperationBindingV3(context.WithoutCancel(ctx), current.Scope, current.StepKind, current.ID)
	if inspectErr == nil && sameBindingFactV3(inspected, next) {
		return inspected, nil
	}
	return bridgecontract.ModelTurnOperationBindingFactV3{}, err
}

func (a *ModelTurnDomainAdapterV3) nowV3() int64 {
	now := a.config.Clock()
	if now.IsZero() {
		return 0
	}
	return now.UnixNano()
}

func domainRefV3(fact bridgecontract.ModelTurnOperationBindingFactV3) (applicationports.OperationDomainStateRefV3, error) {
	if err := fact.Validate(); err != nil {
		return applicationports.OperationDomainStateRefV3{}, err
	}
	digest, err := fact.DigestV3()
	if err != nil {
		return applicationports.OperationDomainStateRefV3{}, err
	}
	state := applicationports.OperationDomainStateV3(fact.State)
	ref := applicationports.OperationDomainStateRefV3{ContractVersion: applicationports.OperationDomainContractVersionV3, StepKind: fact.StepKind, Attempt: fact.ApplicationAttempt, State: state, Revision: fact.Revision, Digest: digest, BasisDigest: fact.BasisDigest}
	return ref, ref.Validate()
}

func preparedBasisDigestV3(request applicationports.BindPreparedOperationRequestV3) (core.Digest, error) {
	return applicationports.OperationDomainBasisDigestV3(struct {
		RuntimeAttempt runtimeports.GovernedExecutionAttemptRefsV2      `json:"runtime_attempt"`
		DelegationFact runtimeports.ExecutionDelegationFactV2           `json:"delegation_fact"`
		Prepared       runtimeports.PreparedExecutionGovernanceResultV2 `json:"prepared"`
	}{request.RuntimeAttempt, request.DelegationFact, request.Prepared})
}

func validateDelegationCandidateV3(delegation runtimeports.ExecutionDelegationFactV2, resolved resolvedModelTurnV3, attempt runtimeports.GovernedExecutionAttemptRefsV2) error {
	declared, err := delegation.RefV2()
	operationDigest, operationErr := delegation.Operation.DigestV3()
	if err != nil || operationErr != nil || declared != attempt.Prepared.DeclaredDelegation || delegation.EndpointID != resolved.envelope.Candidate.Endpoint.ID || delegation.RuntimeSessionRef != resolved.envelope.Candidate.SessionRef || delegation.HostAdapter != resolved.envelope.Candidate.Endpoint.Binding || len(delegation.RelayHops) == 0 || delegation.RelayHops[0].Relay != resolved.envelope.Candidate.Endpoint.Binding || delegation.DataProvider != resolved.envelope.Candidate.Provider || delegation.ProviderAttemptID != attempt.AttemptID || operationDigest != attempt.Admission.OperationDigest {
		return core.NewError(core.ErrorConflict, core.ReasonProviderBindingStale, "delegation endpoint, Session, provider or Runtime attempt differs from the exact model Candidate")
	}
	return nil
}

func unknownBasisDigestV3(request applicationports.MarkUnknownOperationRequestV3) (core.Digest, error) {
	return applicationports.OperationDomainBasisDigestV3(struct {
		RuntimeAttempt runtimeports.GovernedExecutionAttemptRefsV2   `json:"runtime_attempt"`
		Authorization  runtimeports.OperationDispatchAuthorizationV3 `json:"authorization"`
	}{request.RuntimeAttempt, request.Authorization})
}

func observedBasisDigestV3(request applicationports.BindObservedOperationRequestV3) (core.Digest, error) {
	return applicationports.OperationDomainBasisDigestV3(struct {
		RuntimeAttempt runtimeports.GovernedExecutionAttemptRefsV2  `json:"runtime_attempt"`
		Observation    runtimeports.ProviderAttemptObservationRefV2 `json:"observation"`
	}{request.RuntimeAttempt, request.Observation})
}

func settlementBasisDigestV3(request applicationports.ApplyOperationSettlementRequestV3) (core.Digest, error) {
	return applicationports.OperationDomainBasisDigestV3(struct {
		RuntimeAttempt *runtimeports.GovernedExecutionAttemptRefsV2 `json:"runtime_attempt,omitempty"`
		Settlement     runtimeports.OperationSettlementRefV3        `json:"settlement"`
		DomainResult   *runtimeports.OpaquePayloadV2                `json:"domain_result,omitempty"`
	}{request.RuntimeAttempt, request.Settlement, request.DomainResult})
}

func validateSettlementAgainstIntentV3(settlement runtimeports.OperationSettlementRefV3, intent runtimeports.OperationEffectIntentV3) error {
	intentDigest, err := intent.DigestV3()
	operationDigest, operationErr := intent.Operation.DigestV3()
	if err != nil || operationErr != nil || settlement.Attempt.OperationDigest != operationDigest || settlement.Attempt.EffectID != intent.ID || settlement.Attempt.IntentRevision != intent.Revision || settlement.Attempt.IntentDigest != intentDigest {
		return core.NewError(core.ErrorConflict, core.ReasonSettlementOwnerMismatch, "Settlement belongs to another model-turn Intent")
	}
	return nil
}

func sameScopeV3(left, right core.ExecutionScope) bool {
	ld, le := runtimeports.ExecutionScopeDigestV2(left)
	rd, re := runtimeports.ExecutionScopeDigestV2(right)
	return le == nil && re == nil && ld == rd
}

func sameRuntimeAttemptV3(left, right *runtimeports.GovernedExecutionAttemptRefsV2) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	ld, le := core.CanonicalJSONDigest("praxis.harness.application-adapter", bridgecontract.ModelTurnOperationBindingContractV3, "RuntimeAttempt", left)
	rd, re := core.CanonicalJSONDigest("praxis.harness.application-adapter", bridgecontract.ModelTurnOperationBindingContractV3, "RuntimeAttempt", right)
	return le == nil && re == nil && ld == rd
}

func samePreparedRuntimeV3(left, right *runtimeports.GovernedExecutionAttemptRefsV2) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	l, r := cloneRuntimeAttemptV3(*left), cloneRuntimeAttemptV3(*right)
	l.Observation, l.Settlement = nil, nil
	r.Observation, r.Settlement = nil, nil
	return sameRuntimeAttemptV3(&l, &r)
}

func sameApplicationAttemptV3(left, right applicationcontract.GovernedOperationAttemptRefV3) bool {
	ld, le := core.CanonicalJSONDigest("praxis.harness.application-adapter", bridgecontract.ModelTurnOperationBindingContractV3, "ApplicationAttempt", left)
	rd, re := core.CanonicalJSONDigest("praxis.harness.application-adapter", bridgecontract.ModelTurnOperationBindingContractV3, "ApplicationAttempt", right)
	return le == nil && re == nil && ld == rd
}

func sameBindingFactV3(left, right bridgecontract.ModelTurnOperationBindingFactV3) bool {
	ld, le := left.DigestV3()
	rd, re := right.DigestV3()
	return le == nil && re == nil && ld == rd
}

func sameUndispatchedSessionV3(session contract.GovernedSessionV2, candidate contract.CandidateRefV2, settlement runtimeports.OperationSettlementRefV3, domain runtimeports.OpaquePayloadV2) bool {
	return session.Phase == contract.SessionTerminalV2 && session.CompletionClaim == contract.ClaimFailed && session.Execution == nil && session.UndispatchedSettlement != nil && session.UndispatchedSettlement.Candidate == candidate && sameSettlementV3(session.UndispatchedSettlement.Settlement, settlement) && session.UndispatchedSettlement.DomainResultSchema == domain.Schema && session.UndispatchedSettlement.DomainResultDigest == domain.ContentDigest
}

func sameSettlementV3(left, right runtimeports.OperationSettlementRefV3) bool {
	ld, le := core.CanonicalJSONDigest("praxis.harness.application-adapter", bridgecontract.ModelTurnOperationBindingContractV3, "Settlement", left)
	rd, re := core.CanonicalJSONDigest("praxis.harness.application-adapter", bridgecontract.ModelTurnOperationBindingContractV3, "Settlement", right)
	return le == nil && re == nil && ld == rd
}

func isSettledSessionPhaseV3(phase contract.SessionPhaseV2) bool {
	return phase == contract.SessionWaitingActionV2 || phase == contract.SessionWaitingInputV2 || phase == contract.SessionTerminalV2
}

func cloneRuntimeAttemptV3(value runtimeports.GovernedExecutionAttemptRefsV2) runtimeports.GovernedExecutionAttemptRefsV2 {
	clone := value
	if value.Observation != nil {
		observation := *value.Observation
		clone.Observation = &observation
	}
	if value.Settlement != nil {
		settlement := cloneSettlementV3(*value.Settlement)
		clone.Settlement = &settlement
	}
	return clone
}

func cloneDelegationFactV3(value runtimeports.ExecutionDelegationFactV2) runtimeports.ExecutionDelegationFactV2 {
	clone := value
	clone.Operation.ExecutionScope = cloneScopeV3(value.Operation.ExecutionScope)
	clone.RelayHops = append([]runtimeports.ExecutionRelayHopV2(nil), value.RelayHops...)
	if value.Preparation != nil {
		preparation := *value.Preparation
		clone.Preparation = &preparation
	}
	return clone
}

func cloneScopeV3(value core.ExecutionScope) core.ExecutionScope {
	clone := value
	if value.SandboxLease != nil {
		lease := *value.SandboxLease
		clone.SandboxLease = &lease
	}
	return clone
}

func cloneApplicationAttemptV3(value applicationcontract.GovernedOperationAttemptRefV3) applicationcontract.GovernedOperationAttemptRefV3 {
	clone := value
	if value.DomainReservation != nil {
		reservation := *value.DomainReservation
		clone.DomainReservation = &reservation
	}
	if value.Settlement != nil {
		settlement := cloneSettlementV3(*value.Settlement)
		clone.Settlement = &settlement
	}
	return clone
}

func cloneSettlementV3(value runtimeports.OperationSettlementRefV3) runtimeports.OperationSettlementRefV3 {
	clone := value
	clone.Attempt = cloneDispatchAttemptV3(value.Attempt)
	if value.Observation != nil {
		observation := *value.Observation
		clone.Observation = &observation
	}
	if value.InspectionEffect != nil {
		inspection := cloneDispatchAttemptV3(*value.InspectionEffect)
		clone.InspectionEffect = &inspection
	}
	if value.InspectionSettlement != nil {
		inspection := cloneInspectionSettlementV3(*value.InspectionSettlement)
		clone.InspectionSettlement = &inspection
	}
	clone.Evidence = append([]runtimeports.EvidenceRecordRefV2(nil), value.Evidence...)
	if value.DomainResultSchema != nil {
		schema := *value.DomainResultSchema
		clone.DomainResultSchema = &schema
	}
	return clone
}

func cloneInspectionSettlementV3(value runtimeports.OperationInspectionSettlementRefV3) runtimeports.OperationInspectionSettlementRefV3 {
	clone := value
	clone.Attempt = cloneDispatchAttemptV3(value.Attempt)
	if value.Observation != nil {
		observation := *value.Observation
		clone.Observation = &observation
	}
	clone.Evidence = append([]runtimeports.EvidenceRecordRefV2(nil), value.Evidence...)
	if value.DomainResultSchema != nil {
		schema := *value.DomainResultSchema
		clone.DomainResultSchema = &schema
	}
	return clone
}

func cloneDispatchAttemptV3(value runtimeports.OperationDispatchAttemptRefV3) runtimeports.OperationDispatchAttemptRefV3 {
	clone := value
	if value.Delegation != nil {
		delegation := *value.Delegation
		clone.Delegation = &delegation
	}
	return clone
}

func cloneOpaqueV3(value runtimeports.OpaquePayloadV2) runtimeports.OpaquePayloadV2 {
	clone := value
	clone.Inline = append([]byte(nil), value.Inline...)
	return clone
}
