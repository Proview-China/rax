// Package applicationadapter maps Harness-owned current facts to Application
// public contracts. It never dispatches a Tool or authors Runtime facts.
package applicationadapter

import (
	"context"
	"reflect"
	"time"

	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	harnesscontract "github.com/Proview-China/rax/ExecutionRuntime/harness/contract"
	harnessports "github.com/Proview-China/rax/ExecutionRuntime/harness/ports"
	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type SingleCallToolActionAssemblerV2 struct {
	sessions      harnessports.SessionCurrentReaderV4
	current       harnessports.CommittedPendingActionReaderV3
	domainResults harnessports.SettledTurnDomainResultReaderV3
	models        modelinvoker.ToolCallCandidateObservationProjectionReaderV1
	authority     runtimeports.AuthorityFactReaderV2
	clock         func() time.Time
}

var _ applicationports.SingleCallToolActionInputCurrentReaderV2 = (*SingleCallToolActionAssemblerV2)(nil)

func NewSingleCallToolActionAssemblerV2(
	sessions harnessports.SessionCurrentReaderV4,
	current harnessports.CommittedPendingActionReaderV3,
	domainResults harnessports.SettledTurnDomainResultReaderV3,
	models modelinvoker.ToolCallCandidateObservationProjectionReaderV1,
	authority runtimeports.AuthorityFactReaderV2,
	clock func() time.Time,
) (*SingleCallToolActionAssemblerV2, error) {
	if unavailableInterfaceV2(sessions) || unavailableInterfaceV2(current) || unavailableInterfaceV2(domainResults) || unavailableInterfaceV2(models) || unavailableInterfaceV2(authority) || clock == nil {
		return nil, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "single-call V2 assembler requires five narrow readers and a clock")
	}
	return &SingleCallToolActionAssemblerV2{sessions: sessions, current: current, domainResults: domainResults, models: models, authority: authority, clock: clock}, nil
}

func (a *SingleCallToolActionAssemblerV2) AssembleSingleCallToolActionRequestV2(ctx context.Context, input applicationcontract.AssembleSingleCallToolActionRequestV2) (applicationcontract.SingleCallToolActionRequestV2, error) {
	if ctx == nil {
		return applicationcontract.SingleCallToolActionRequestV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "single-call assembler requires a context")
	}
	if a == nil || unavailableInterfaceV2(a.sessions) || unavailableInterfaceV2(a.current) || unavailableInterfaceV2(a.domainResults) || unavailableInterfaceV2(a.models) || unavailableInterfaceV2(a.authority) || a.clock == nil {
		return applicationcontract.SingleCallToolActionRequestV2{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "single-call V2 assembler is unavailable")
	}
	if err := input.Action.Validate(); err != nil {
		return applicationcontract.SingleCallToolActionRequestV2{}, err
	}
	if err := input.Authority.Validate(); err != nil {
		return applicationcontract.SingleCallToolActionRequestV2{}, err
	}
	if input.RequestedNotAfterUnixNano < 0 {
		return applicationcontract.SingleCallToolActionRequestV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "single-call requested upper bound is negative")
	}

	nowS1 := a.clock()
	if nowS1.IsZero() || input.RequestedNotAfterUnixNano > 0 && input.RequestedNotAfterUnixNano <= nowS1.UnixNano() {
		return applicationcontract.SingleCallToolActionRequestV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonCapabilityExpired, "single-call request upper bound is not current")
	}
	s1, err := a.inspectSnapshotV2(ctx, input, nowS1)
	if err != nil {
		return applicationcontract.SingleCallToolActionRequestV2{}, err
	}

	nowBeforeS2 := a.clock()
	if nowBeforeS2.IsZero() || nowBeforeS2.Before(nowS1) {
		return applicationcontract.SingleCallToolActionRequestV2{}, core.NewError(core.ErrorIndeterminate, core.ReasonClockRegression, "single-call assembler clock regressed before S2")
	}
	s2, err := a.inspectSnapshotV2(ctx, input, nowBeforeS2)
	if err != nil {
		return applicationcontract.SingleCallToolActionRequestV2{}, err
	}
	if err := sameAssemblerSnapshotV2(s1, s2); err != nil {
		return applicationcontract.SingleCallToolActionRequestV2{}, err
	}

	nowS2 := a.clock()
	if nowS2.IsZero() || nowS2.Before(nowBeforeS2) {
		return applicationcontract.SingleCallToolActionRequestV2{}, core.NewError(core.ErrorIndeterminate, core.ReasonClockRegression, "single-call assembler clock regressed after S2")
	}
	if err := s2.current.ValidateAgainst(s2.currentRequest, nowS2); err != nil {
		return applicationcontract.SingleCallToolActionRequestV2{}, err
	}
	if err := s2.authority.ValidateCurrent(input.Authority, input.Action.ExecutionScope, input.Action.Digest, nowS2); err != nil {
		return applicationcontract.SingleCallToolActionRequestV2{}, err
	}

	expires := s2.current.ExpiresUnixNano
	if s2.authority.ExpiresUnixNano < expires {
		expires = s2.authority.ExpiresUnixNano
	}
	if input.RequestedNotAfterUnixNano > 0 && input.RequestedNotAfterUnixNano < expires {
		expires = input.RequestedNotAfterUnixNano
	}
	if expires <= nowS2.UnixNano() {
		return applicationcontract.SingleCallToolActionRequestV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonCapabilityExpired, "single-call assembler crossed its currentness window")
	}

	// This is the only Request seal in the assembler. All Owner reads and both
	// currentness passes have completed before this call.
	sealed, err := applicationcontract.SealSingleCallToolActionRequestV2(applicationcontract.SingleCallToolActionRequestV2{
		Action:          cloneActionCoordinateV2(input.Action),
		Authority:       input.Authority,
		CreatedUnixNano: nowS2.UnixNano(),
		ExpiresUnixNano: expires,
	})
	if err != nil {
		return applicationcontract.SingleCallToolActionRequestV2{}, err
	}
	return sealed, sealed.ValidateCurrent(nowS2)
}

// InspectSingleCallToolActionInputCurrentV2 reconstructs the complete neutral
// Application proof from Owner-current reads. It does not reuse observations
// made while the Request was assembled and exposes no write capability.
func (a *SingleCallToolActionAssemblerV2) InspectSingleCallToolActionInputCurrentV2(ctx context.Context, request applicationcontract.SingleCallToolActionRequestV2) (applicationcontract.SingleCallToolActionInputCurrentProjectionV2, error) {
	if ctx == nil {
		return applicationcontract.SingleCallToolActionInputCurrentProjectionV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "single-call input-current reader requires a context")
	}
	if a == nil || unavailableInterfaceV2(a.sessions) || unavailableInterfaceV2(a.current) || unavailableInterfaceV2(a.domainResults) || unavailableInterfaceV2(a.models) || unavailableInterfaceV2(a.authority) || a.clock == nil {
		return applicationcontract.SingleCallToolActionInputCurrentProjectionV2{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "single-call V2 input-current reader is unavailable")
	}
	nowS1 := a.clock()
	if err := request.ValidateCurrent(nowS1); err != nil {
		return applicationcontract.SingleCallToolActionInputCurrentProjectionV2{}, err
	}
	input := applicationcontract.AssembleSingleCallToolActionRequestV2{Action: cloneActionCoordinateV2(request.Action), Authority: request.Authority, RequestedNotAfterUnixNano: request.ExpiresUnixNano}
	s1, err := a.inspectSnapshotV2(ctx, input, nowS1)
	if err != nil {
		return applicationcontract.SingleCallToolActionInputCurrentProjectionV2{}, err
	}

	nowBeforeS2 := a.clock()
	if nowBeforeS2.IsZero() || nowBeforeS2.Before(nowS1) {
		return applicationcontract.SingleCallToolActionInputCurrentProjectionV2{}, core.NewError(core.ErrorIndeterminate, core.ReasonClockRegression, "single-call input-current clock regressed before S2")
	}
	s2, err := a.inspectSnapshotV2(ctx, input, nowBeforeS2)
	if err != nil {
		return applicationcontract.SingleCallToolActionInputCurrentProjectionV2{}, err
	}
	if err := sameAssemblerSnapshotV2(s1, s2); err != nil {
		return applicationcontract.SingleCallToolActionInputCurrentProjectionV2{}, err
	}

	nowS2 := a.clock()
	if nowS2.IsZero() || nowS2.Before(nowBeforeS2) {
		return applicationcontract.SingleCallToolActionInputCurrentProjectionV2{}, core.NewError(core.ErrorIndeterminate, core.ReasonClockRegression, "single-call input-current clock regressed after S2")
	}
	if err := request.ValidateCurrent(nowS2); err != nil {
		return applicationcontract.SingleCallToolActionInputCurrentProjectionV2{}, err
	}
	if err := s2.current.ValidateAgainst(s2.currentRequest, nowS2); err != nil {
		return applicationcontract.SingleCallToolActionInputCurrentProjectionV2{}, err
	}
	if err := s2.authority.ValidateCurrent(request.Authority, request.Action.ExecutionScope, request.Action.Digest, nowS2); err != nil {
		return applicationcontract.SingleCallToolActionInputCurrentProjectionV2{}, err
	}

	identityRequest, err := applicationcontract.SealSingleCallModelPendingActionIdentityCurrentRequestV2(applicationcontract.SingleCallModelPendingActionIdentityCurrentRequestV2{
		Run:                       request.Action.PendingSubject.Run,
		SessionID:                 request.Action.PendingSubject.SessionID,
		Turn:                      request.Action.PendingSubject.Turn,
		IdentityRef:               request.Action.PendingSubject.Binding.Base.IdentityRef,
		DomainResultFact:          request.Action.PendingSubject.Binding.Base.DomainResultFact,
		RequestedNotAfterUnixNano: request.ExpiresUnixNano,
	})
	if err != nil {
		return applicationcontract.SingleCallToolActionInputCurrentProjectionV2{}, err
	}
	identityExpires := s2.identity.NotAfterUnixNano
	if request.ExpiresUnixNano < identityExpires {
		identityExpires = request.ExpiresUnixNano
	}
	identityCurrent, err := applicationcontract.SealSingleCallModelPendingActionIdentityCurrentV2(applicationcontract.SingleCallModelPendingActionIdentityCurrentV2{
		RequestDigest:    identityRequest.Digest,
		IdentityRef:      identityRequest.IdentityRef,
		DomainResultFact: identityRequest.DomainResultFact,
		Identity:         s2.identity,
		Projection:       applicationcontract.CloneSingleCallModelToolCallProjectionProofV2(s2.projectionProof),
		CheckedUnixNano:  nowS2.UnixNano(),
		ExpiresUnixNano:  identityExpires,
	}, identityRequest, nowS2)
	if err != nil {
		return applicationcontract.SingleCallToolActionInputCurrentProjectionV2{}, err
	}
	harnessExpires := s2.current.ExpiresUnixNano
	if identityCurrent.ExpiresUnixNano < harnessExpires {
		harnessExpires = identityCurrent.ExpiresUnixNano
	}
	harnessProof, err := applicationcontract.SealSingleCallHarnessOwnerCurrentProofV3(applicationcontract.SingleCallHarnessOwnerCurrentProofV3{
		Subject:                       request.Action.PendingSubject,
		Binding:                       request.Action.PendingSubject.Binding,
		HarnessCurrentContractVersion: s2.current.ContractVersion,
		HarnessCurrentDigest:          s2.current.Digest,
		IdentityCurrent:               identityCurrent,
		CheckedUnixNano:               nowS2.UnixNano(),
		ExpiresUnixNano:               harnessExpires,
	}, nowS2)
	if err != nil {
		return applicationcontract.SingleCallToolActionInputCurrentProjectionV2{}, err
	}
	authorityExpires := s2.authority.ExpiresUnixNano
	if request.ExpiresUnixNano < authorityExpires {
		authorityExpires = request.ExpiresUnixNano
	}
	authorityProof, err := applicationcontract.SealSingleCallAuthorityCurrentProofV2(applicationcontract.SingleCallAuthorityCurrentProofV2{
		Ref:                    request.Authority,
		ExecutionScopeDigest:   request.Action.ExecutionScopeDigest,
		ActionCoordinateDigest: request.Action.Digest,
		FactRevision:           s2.authority.Revision,
		FactDigest:             s2.authority.Digest,
		CheckedUnixNano:        nowS2.UnixNano(),
		ExpiresUnixNano:        authorityExpires,
	}, request, nowS2)
	if err != nil {
		return applicationcontract.SingleCallToolActionInputCurrentProjectionV2{}, err
	}
	expires := harnessProof.ExpiresUnixNano
	if authorityProof.ExpiresUnixNano < expires {
		expires = authorityProof.ExpiresUnixNano
	}
	sealed, err := applicationcontract.SealSingleCallToolActionInputCurrentProjectionV2(applicationcontract.SingleCallToolActionInputCurrentProjectionV2{
		HarnessCurrent:   harnessProof,
		AuthorityCurrent: authorityProof,
		CheckedUnixNano:  nowS2.UnixNano(),
		ExpiresUnixNano:  expires,
	}, request, nowS2)
	if err != nil {
		return applicationcontract.SingleCallToolActionInputCurrentProjectionV2{}, err
	}
	return applicationcontract.CloneSingleCallToolActionInputCurrentProjectionV2(sealed), sealed.ValidateFor(request, nowS2)
}

type assemblerSnapshotV2 struct {
	session         harnesscontract.GovernedSessionV4
	fact            harnesscontract.SettledTurnDomainResultFactV3
	projection      modelinvoker.ToolCallCandidateObservationProjectionV1
	projectionProof applicationcontract.SingleCallModelToolCallProjectionProofV2
	identity        applicationcontract.SingleCallModelPendingActionIdentityCoordinateV2
	currentRequest  harnesscontract.CommittedPendingActionCurrentRequestV3
	current         harnesscontract.CommittedPendingActionCurrentV3
	authority       runtimeports.DispatchAuthorityFactV2
}

func (a *SingleCallToolActionAssemblerV2) inspectSnapshotV2(ctx context.Context, input applicationcontract.AssembleSingleCallToolActionRequestV2, now time.Time) (assemblerSnapshotV2, error) {
	run := harnesscontract.RunRef{Scope: cloneExecutionScopeV2(input.Action.ExecutionScope), RunID: input.Action.PendingSubject.Run.RunID}
	session, err := a.sessions.InspectSessionV4(ctx, run, input.Action.PendingSubject.SessionID)
	if err != nil {
		return assemblerSnapshotV2{}, err
	}
	session = session.Clone()
	if err := session.Validate(); err != nil {
		return assemblerSnapshotV2{}, err
	}
	if session.ApplicationBinding == nil || session.PendingAction == nil || session.Phase != harnesscontract.SessionWaitingActionV2 {
		return assemblerSnapshotV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "single-call assembler requires one committed waiting_action Session")
	}

	fact, err := a.domainResults.InspectExact(ctx, session.ApplicationBinding.Base.DomainResultFactRef)
	if err != nil {
		return assemblerSnapshotV2{}, err
	}
	fact = fact.Clone()
	if err := fact.Validate(); err != nil {
		return assemblerSnapshotV2{}, err
	}
	factRef, err := fact.RefV3()
	if err != nil || !reflect.DeepEqual(factRef, session.ApplicationBinding.Base.DomainResultFactRef) {
		return assemblerSnapshotV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "single-call DomainResult exact ref drifted")
	}

	projection, err := a.models.InspectExactProjectionV1(ctx, fact.ModelProjection)
	if err != nil {
		return assemblerSnapshotV2{}, err
	}
	projection = projection.Clone()
	proof, identity, err := validateIdentityProjectionV2(fact, projection, *session.PendingAction, now)
	if err != nil {
		return assemblerSnapshotV2{}, err
	}
	expectedSubject, err := mapPendingSubjectV2(session, fact, identity)
	if err != nil {
		return assemblerSnapshotV2{}, err
	}
	if !reflect.DeepEqual(expectedSubject, input.Action.PendingSubject) || !runtimeports.SameExecutionScopeV2(input.Action.ExecutionScope, session.Run.Scope) || input.Action.ExecutionScopeDigest != expectedSubject.Run.ExecutionScopeDigest {
		return assemblerSnapshotV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "single-call Action and Harness current subject drifted")
	}

	currentRequest := harnesscontract.CommittedPendingActionCurrentRequestV3{
		Subject: harnesscontract.CommittedPendingActionSubjectV3{
			Base: harnesscontract.CommittedPendingActionSubjectV2{
				ExecutionScopeDigest: input.Action.ExecutionScopeDigest,
				Run:                  session.Run,
				SessionID:            session.ID,
				SessionRevision:      session.Revision,
				SessionDigest:        session.Digest,
				Turn:                 session.Turn,
				PendingActionRef:     session.PendingAction.Ref,
				IdentityRef:          session.ApplicationBinding.Base.IdentityRef,
				DomainResultFactRef:  session.ApplicationBinding.Base.DomainResultFactRef,
				ModelTurnSettlement:  session.ApplicationBinding.Base.ModelTurnSettlementRef,
			},
			ApplicationBinding: session.ApplicationBinding.Clone(),
		},
		RequestedNotAfterUnixNano: input.RequestedNotAfterUnixNano,
	}
	if err := currentRequest.Validate(now); err != nil {
		return assemblerSnapshotV2{}, err
	}
	current, err := a.current.InspectCommittedPendingActionCurrentV3(ctx, currentRequest)
	if err != nil {
		return assemblerSnapshotV2{}, err
	}
	current = current.Clone()
	if err := current.ValidateAgainst(currentRequest, now); err != nil {
		return assemblerSnapshotV2{}, err
	}

	authority, err := a.authority.InspectDispatchAuthority(ctx, input.Authority.Ref)
	if err != nil {
		return assemblerSnapshotV2{}, err
	}
	authority.Scope = cloneExecutionScopeV2(authority.Scope)
	if err := authority.ValidateCurrent(input.Authority, input.Action.ExecutionScope, input.Action.Digest, now); err != nil {
		return assemblerSnapshotV2{}, err
	}
	return assemblerSnapshotV2{session: session, fact: fact, projection: projection, projectionProof: proof, identity: identity, currentRequest: currentRequest.Clone(), current: current, authority: authority}, nil
}

func validateIdentityProjectionV2(fact harnesscontract.SettledTurnDomainResultFactV3, projection modelinvoker.ToolCallCandidateObservationProjectionV1, pending harnesscontract.PendingActionV2, now time.Time) (applicationcontract.SingleCallModelToolCallProjectionProofV2, applicationcontract.SingleCallModelPendingActionIdentityCoordinateV2, error) {
	if err := projection.Validate(); err != nil {
		return applicationcontract.SingleCallModelToolCallProjectionProofV2{}, applicationcontract.SingleCallModelPendingActionIdentityCoordinateV2{}, err
	}
	identity := fact.Identity
	if err := identity.Validate(); err != nil {
		return applicationcontract.SingleCallModelToolCallProjectionProofV2{}, applicationcontract.SingleCallModelPendingActionIdentityCoordinateV2{}, err
	}
	if fact.CreatedUnixNano != identity.CreatedUnixNano || identity.NotAfterUnixNano <= now.UnixNano() || projection.Ref != fact.ModelProjection || projection.Ref != identity.ModelProjection || len(projection.Observation.Calls) != 1 {
		return applicationcontract.SingleCallModelToolCallProjectionProofV2{}, applicationcontract.SingleCallModelPendingActionIdentityCoordinateV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "single-call Identity and Model projection drifted")
	}
	call := projection.Observation.Calls[0]
	if call.Ordinal != identity.CallOrdinal.Value || call.CallID != identity.CallID || call.Name != identity.CallName || pending.Ref != identity.PendingActionRef || pending.RequestDigest != identity.PendingActionRequestDigest || pending.Payload.Schema != identity.PayloadSchema || pending.Payload.ContentDigest != identity.PayloadContentDigest || pending.Capability != identity.Capability || pending.SourceCandidate != identity.SourceCandidate || core.DigestBytes(call.CanonicalArguments) != identity.CanonicalArgumentsDigest {
		return applicationcontract.SingleCallModelToolCallProjectionProofV2{}, applicationcontract.SingleCallModelPendingActionIdentityCoordinateV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "single-call Model call and PendingAction identity drifted")
	}
	proof, err := applicationcontract.SealSingleCallModelToolCallProjectionProofV2(applicationcontract.SingleCallModelToolCallProjectionProofV2{
		ProjectionContractVersion: projection.ContractVersion,
		ProjectionID:              projection.Ref.ID,
		ProjectionRevision:        projection.Ref.Revision,
		ProjectionDigest:          projection.Ref.Digest,
		InvocationID:              projection.Ref.InvocationID,
		InvocationDigest:          projection.Ref.InvocationDigest,
		ObservationDigest:         projection.Ref.ObservationDigest,
		SourceResponseID:          projection.Ref.Source.ResponseID,
		SourceSequence:            projection.Ref.Source.SourceSequence,
		CallOrdinal:               call.Ordinal,
		CallID:                    call.CallID,
		CallName:                  call.Name,
		CanonicalArguments:        append([]byte(nil), call.CanonicalArguments...),
	})
	if err != nil {
		return applicationcontract.SingleCallModelToolCallProjectionProofV2{}, applicationcontract.SingleCallModelPendingActionIdentityCoordinateV2{}, err
	}
	coordinate, err := mapIdentityCoordinateV2(fact)
	if err != nil {
		return applicationcontract.SingleCallModelToolCallProjectionProofV2{}, applicationcontract.SingleCallModelPendingActionIdentityCoordinateV2{}, err
	}
	return applicationcontract.CloneSingleCallModelToolCallProjectionProofV2(proof), coordinate, nil
}

func mapIdentityCoordinateV2(fact harnesscontract.SettledTurnDomainResultFactV3) (applicationcontract.SingleCallModelPendingActionIdentityCoordinateV2, error) {
	i := fact.Identity
	sourceDigest, err := i.SourceKey.DigestV1()
	if err != nil {
		return applicationcontract.SingleCallModelPendingActionIdentityCoordinateV2{}, err
	}
	return applicationcontract.SealSingleCallModelPendingActionIdentityCoordinateV2(applicationcontract.SingleCallModelPendingActionIdentityCoordinateV2{
		IdentityContractVersion:    i.ContractVersion,
		IdentityID:                 i.ID,
		IdentityRevision:           i.Revision,
		IdentityDigest:             i.Digest,
		CreatedUnixNano:            i.CreatedUnixNano,
		ModelProjectionID:          i.ModelProjection.ID,
		ModelProjectionRevision:    i.ModelProjection.Revision,
		ModelProjectionDigest:      i.ModelProjection.Digest,
		ModelInvocationID:          i.ModelProjection.InvocationID,
		ModelInvocationDigest:      i.ModelProjection.InvocationDigest,
		ModelObservationDigest:     i.ModelProjection.ObservationDigest,
		ModelSourceResponseID:      i.ModelProjection.Source.ResponseID,
		ModelSourceSequence:        i.ModelProjection.Source.SourceSequence,
		SourceKeyDigest:            sourceDigest,
		SourceExecutionScopeDigest: i.SourceKey.ExecutionScopeDigest,
		SourceRunID:                i.SourceKey.RunID,
		SourceSessionID:            i.SourceKey.SessionID,
		SourceTurn:                 i.SourceKey.Turn,
		CallOrdinalEncodingVersion: applicationcontract.SingleCallCallOrdinalEncodingVersionV1,
		CallOrdinalPresent:         i.CallOrdinal.Present,
		CallOrdinalValue:           i.CallOrdinal.Value,
		SettlementOwner:            i.SettlementOwner,
		CallID:                     i.CallID,
		CallName:                   i.CallName,
		CanonicalArgumentsDigest:   i.CanonicalArgumentsDigest,
		PendingActionRef:           i.PendingActionRef,
		PendingActionRequestDigest: i.PendingActionRequestDigest,
		PayloadSchema:              i.PayloadSchema,
		PayloadContentDigest:       i.PayloadContentDigest,
		Capability:                 i.Capability,
		SourceCandidateID:          i.SourceCandidate.ID,
		SourceCandidateRevision:    i.SourceCandidate.Revision,
		SourceCandidateDigest:      i.SourceCandidate.Digest,
		DomainResultDigest:         fact.ContentDigest,
		NotAfterUnixNano:           i.NotAfterUnixNano,
	})
}

func mapPendingSubjectV2(session harnesscontract.GovernedSessionV4, fact harnesscontract.SettledTurnDomainResultFactV3, identity applicationcontract.SingleCallModelPendingActionIdentityCoordinateV2) (applicationcontract.SingleCallPendingActionSubjectCoordinateV2, error) {
	binding := session.ApplicationBinding
	pending := session.PendingAction
	identityRef := binding.Base.IdentityRef
	factRef := binding.Base.DomainResultFactRef
	base := applicationcontract.SingleCallHarnessBaseBindingCoordinateV2{
		PendingAction: applicationcontract.SingleCallPendingActionCoordinateV1{
			ActionRef: pending.Ref, RequestDigest: pending.RequestDigest, Capability: pending.Capability,
			PayloadSchema: pending.Payload.Schema, PayloadDigest: pending.Payload.ContentDigest,
			SourceCandidateID: pending.SourceCandidate.ID, SourceCandidateRevision: pending.SourceCandidate.Revision,
			SourceCandidateDigest: pending.SourceCandidate.Digest, ProjectionDigest: fact.ModelProjection.Digest,
		},
		IdentityRef: applicationcontract.SingleCallModelPendingActionIdentityRefCoordinateV2{
			ID: identityRef.ID, Revision: identityRef.Revision, Digest: identityRef.Digest,
			ModelProjectionID: identityRef.ModelProjectionID, ModelProjectionRevision: identityRef.ModelProjectionRevision,
			ModelProjectionDigest: identityRef.ModelProjectionDigest, PendingActionRef: identityRef.PendingActionRef,
			PendingActionRequestDigest: identityRef.PendingActionRequestDigest, DomainResultDigest: identityRef.DomainResultDigest,
			SourceKeyDigest: identityRef.SourceKeyDigest,
		},
		DomainResultFact: applicationcontract.SingleCallSettledTurnDomainResultFactRefCoordinateV2{
			FactID: factRef.FactID, Revision: factRef.Revision, FactDigest: factRef.FactDigest,
			SourceKeyDigest: factRef.SourceKeyDigest, Schema: factRef.Schema, ContentDigest: factRef.ContentDigest,
			IdentityRef: applicationcontract.SingleCallModelPendingActionIdentityRefCoordinateV2{
				ID: identityRef.ID, Revision: identityRef.Revision, Digest: identityRef.Digest,
				ModelProjectionID: identityRef.ModelProjectionID, ModelProjectionRevision: identityRef.ModelProjectionRevision,
				ModelProjectionDigest: identityRef.ModelProjectionDigest, PendingActionRef: identityRef.PendingActionRef,
				PendingActionRequestDigest: identityRef.PendingActionRequestDigest, DomainResultDigest: identityRef.DomainResultDigest,
				SourceKeyDigest: identityRef.SourceKeyDigest,
			},
		},
		ModelTurnSettlement: cloneSettlementRefV2(binding.Base.ModelTurnSettlementRef),
	}
	base, err := applicationcontract.SealSingleCallHarnessBaseBindingCoordinateV2(base)
	if err != nil {
		return applicationcontract.SingleCallPendingActionSubjectCoordinateV2{}, err
	}
	owner := binding.OwnerCurrentInputs
	ownerCoordinate := applicationcontract.SingleCallHarnessOwnerCurrentInputsCoordinateV2{
		HarnessContractVersion:       owner.ContractVersion,
		ModelTurnOperation:           owner.ModelTurnOperation,
		GenerationBindingAssociation: owner.GenerationBindingAssociation,
		RouteCurrent:                 owner.RouteCurrent, RouteMatrix: owner.RouteMatrix,
		ContextApplicability: owner.ContextApplicability, HarnessDigest: owner.Digest,
	}
	ownerCoordinate.ModelTurnOperation.ExecutionScope = cloneExecutionScopeV2(ownerCoordinate.ModelTurnOperation.ExecutionScope)
	ownerCoordinate, err = applicationcontract.SealSingleCallHarnessOwnerCurrentInputsCoordinateV2(ownerCoordinate)
	if err != nil {
		return applicationcontract.SingleCallPendingActionSubjectCoordinateV2{}, err
	}
	bindingCoordinate, err := applicationcontract.SealSingleCallHarnessApplicationBindingCoordinateV2(applicationcontract.SingleCallHarnessApplicationBindingCoordinateV2{BindingVersion: binding.ContractVersion, Base: base, OwnerInputs: ownerCoordinate, HarnessBindingDigest: binding.Digest})
	if err != nil {
		return applicationcontract.SingleCallPendingActionSubjectCoordinateV2{}, err
	}
	run, err := applicationcontract.SealSingleCallRunSubjectCoordinateV2(applicationcontract.SingleCallRunSubjectCoordinateV2{ExecutionScope: cloneExecutionScopeV2(session.Run.Scope), RunID: core.AgentRunID(session.Run.RunID)})
	if err != nil {
		return applicationcontract.SingleCallPendingActionSubjectCoordinateV2{}, err
	}
	return applicationcontract.SealSingleCallPendingActionSubjectCoordinateV2(applicationcontract.SingleCallPendingActionSubjectCoordinateV2{
		Run: run, SessionID: session.ID, SessionRevision: session.Revision, SessionDigest: session.Digest,
		Turn: session.Turn, PendingActionRef: pending.Ref, PendingActionDigest: pending.RequestDigest,
		Binding: bindingCoordinate, Identity: identity,
	})
}

func sameAssemblerSnapshotV2(left, right assemblerSnapshotV2) error {
	if !reflect.DeepEqual(left.session, right.session) || !reflect.DeepEqual(left.fact, right.fact) || !reflect.DeepEqual(left.projection, right.projection) || !reflect.DeepEqual(left.projectionProof, right.projectionProof) || !reflect.DeepEqual(left.identity, right.identity) || !reflect.DeepEqual(left.authority, right.authority) || !sameCurrentStableV2(left.current, right.current) || !reflect.DeepEqual(left.currentRequest, right.currentRequest) {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "single-call assembler S1/S2 owner state drifted")
	}
	if right.current.CheckedUnixNano < left.current.CheckedUnixNano {
		return core.NewError(core.ErrorIndeterminate, core.ReasonClockRegression, "single-call Current V3 observation time regressed")
	}
	if right.current.ExpiresUnixNano > left.current.ExpiresUnixNano {
		return core.NewError(core.ErrorConflict, core.ReasonCapabilityExpired, "single-call Current V3 S2 expiry expanded")
	}
	return nil
}

func sameCurrentStableV2(left, right harnesscontract.CommittedPendingActionCurrentV3) bool {
	left.CheckedUnixNano, left.ExpiresUnixNano, left.Digest = 0, 0, ""
	right.CheckedUnixNano, right.ExpiresUnixNano, right.Digest = 0, 0, ""
	return reflect.DeepEqual(left, right)
}

func cloneActionCoordinateV2(value applicationcontract.SingleCallActionCoordinateV2) applicationcontract.SingleCallActionCoordinateV2 {
	value.ExecutionScope = cloneExecutionScopeV2(value.ExecutionScope)
	value.PendingSubject.Run.ExecutionScope = cloneExecutionScopeV2(value.PendingSubject.Run.ExecutionScope)
	value.PendingSubject.Binding.OwnerInputs.ModelTurnOperation.ExecutionScope = cloneExecutionScopeV2(value.PendingSubject.Binding.OwnerInputs.ModelTurnOperation.ExecutionScope)
	value.PendingSubject.Binding.Base.ModelTurnSettlement = cloneSettlementRefV2(value.PendingSubject.Binding.Base.ModelTurnSettlement)
	return value
}

func cloneExecutionScopeV2(value core.ExecutionScope) core.ExecutionScope {
	if value.SandboxLease != nil {
		lease := *value.SandboxLease
		value.SandboxLease = &lease
	}
	return value
}

func cloneSettlementRefV2(value runtimeports.OperationSettlementRefV3) runtimeports.OperationSettlementRefV3 {
	if value.Attempt.Delegation != nil {
		delegation := *value.Attempt.Delegation
		value.Attempt.Delegation = &delegation
	}
	if value.Observation != nil {
		observation := *value.Observation
		value.Observation = &observation
	}
	if value.InspectionEffect != nil {
		inspection := *value.InspectionEffect
		if inspection.Delegation != nil {
			delegation := *inspection.Delegation
			inspection.Delegation = &delegation
		}
		value.InspectionEffect = &inspection
	}
	if value.InspectionSettlement != nil {
		inspection := *value.InspectionSettlement
		if inspection.Attempt.Delegation != nil {
			delegation := *inspection.Attempt.Delegation
			inspection.Attempt.Delegation = &delegation
		}
		inspection.Evidence = append([]runtimeports.EvidenceRecordRefV2(nil), inspection.Evidence...)
		if inspection.Observation != nil {
			observation := *inspection.Observation
			inspection.Observation = &observation
		}
		if inspection.DomainResultSchema != nil {
			schema := *inspection.DomainResultSchema
			inspection.DomainResultSchema = &schema
		}
		value.InspectionSettlement = &inspection
	}
	value.Evidence = append([]runtimeports.EvidenceRecordRefV2(nil), value.Evidence...)
	if value.DomainResultSchema != nil {
		schema := *value.DomainResultSchema
		value.DomainResultSchema = &schema
	}
	return value
}

func unavailableInterfaceV2(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}
