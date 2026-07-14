package kernel

import (
	"context"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/contract"
	harnessports "github.com/Proview-China/rax/ExecutionRuntime/harness/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type GovernedLoopConfigV2 struct {
	Sessions     harnessports.SessionFactPortV2
	Candidates   harnessports.CandidateFactPortV2
	Clock        func() time.Time
	CandidateTTL time.Duration
}

// GovernedLoopV2 owns only the durable Harness Session and candidate state
// machine. It intentionally has no provider execution method: Application and
// Runtime governance must persist and begin a distinct Effect before a
// provider can prepare or execute the candidate.
type GovernedLoopV2 struct{ config GovernedLoopConfigV2 }

func NewGovernedLoopV2(config GovernedLoopConfigV2) (*GovernedLoopV2, error) {
	if config.Sessions == nil || config.Candidates == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "governed Harness session and candidate fact ports are required")
	}
	if config.Clock == nil {
		config.Clock = time.Now
	}
	if config.CandidateTTL <= 0 || config.CandidateTTL > 10*time.Minute {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonPlanInvalid, "governed candidate TTL must be positive and bounded")
	}
	return &GovernedLoopV2{config: config}, nil
}

type PrepareInitialCandidateRequestV2 struct {
	Run             contract.RunRef                   `json:"run"`
	Endpoint        contract.EndpointRefV2            `json:"endpoint"`
	SessionID       string                            `json:"session_id"`
	CandidateID     string                            `json:"candidate_id"`
	Input           runtimeports.OpaquePayloadV2      `json:"input"`
	ContextRef      string                            `json:"context_ref"`
	ContextDigest   core.Digest                       `json:"context_digest"`
	Provider        runtimeports.ProviderBindingRefV2 `json:"provider"`
	CreatedUnixNano int64                             `json:"created_unix_nano"`
	ExpiresUnixNano int64                             `json:"expires_unix_nano"`
}

type PrepareInitialCandidateResultV2 struct {
	Session   contract.GovernedSessionV2    `json:"session"`
	Candidate contract.ModelTurnCandidateV2 `json:"candidate"`
}

// PrepareInitialCandidateV2 is restart-safe across the Session, Candidate and
// Session-CAS owners. Unavailable is resolved only by exact Inspect; arbitrary
// pre-existing content fails closed.
func (l *GovernedLoopV2) PrepareInitialCandidateV2(ctx context.Context, request PrepareInitialCandidateRequestV2) (PrepareInitialCandidateResultV2, error) {
	now := l.config.Clock()
	if request.CreatedUnixNano <= 0 || request.ExpiresUnixNano <= request.CreatedUnixNano || time.Duration(request.ExpiresUnixNano-request.CreatedUnixNano) > l.config.CandidateTTL || !now.Before(time.Unix(0, request.ExpiresUnixNano)) {
		return PrepareInitialCandidateResultV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonCapabilityExpired, "preallocated candidate lifetime is invalid, expired or exceeds policy")
	}
	session := contract.GovernedSessionV2{ContractVersion: contract.GovernedContractVersionV2, ID: request.SessionID, Revision: 1, Run: request.Run, Endpoint: request.Endpoint, Phase: contract.SessionCreatingV2, CreatedUnixNano: request.CreatedUnixNano, UpdatedUnixNano: request.CreatedUnixNano}
	if err := session.Validate(); err != nil {
		return PrepareInitialCandidateResultV2{}, err
	}
	candidate := contract.ModelTurnCandidateV2{ContractVersion: contract.GovernedContractVersionV2, ID: request.CandidateID, Revision: 1, Run: request.Run, Endpoint: request.Endpoint, SessionRef: request.SessionID, ExpectedSessionRevision: 1, Turn: 1, Kind: contract.CandidateInitialTurnV2, Input: request.Input, ContextRef: request.ContextRef, ContextDigest: request.ContextDigest, Provider: request.Provider, CreatedUnixNano: request.CreatedUnixNano, ExpiresUnixNano: request.ExpiresUnixNano}
	if err := candidate.Validate(now); err != nil {
		return PrepareInitialCandidateResultV2{}, err
	}
	candidateRef, err := candidate.RefV2()
	if err != nil {
		return PrepareInitialCandidateResultV2{}, err
	}
	next := session
	next.Revision = 2
	next.Phase = contract.SessionWaitingModelDispatchV2
	next.Turn = 1
	next.Candidate = &candidateRef
	next.UpdatedUnixNano = request.CreatedUnixNano

	createdSession, err := l.config.Sessions.CreateSessionV2(ctx, session)
	if err != nil {
		if !core.HasCategory(err, core.ErrorUnavailable) && !core.HasCategory(err, core.ErrorConflict) {
			return PrepareInitialCandidateResultV2{}, err
		}
		createdSession, err = l.config.Sessions.InspectSessionV2(context.WithoutCancel(ctx), request.Run, request.SessionID)
		if err != nil {
			return PrepareInitialCandidateResultV2{}, err
		}
	}
	if !sameGovernedSessionV2(createdSession, session) {
		// A previous call may already have completed the exact CAS. Nothing else
		// is accepted as recovery.
		if !sameGovernedSessionV2(createdSession, next) {
			return PrepareInitialCandidateResultV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "existing governed session differs from requested preparation")
		}
	}
	createdCandidate, err := l.config.Candidates.CreateCandidateV2(ctx, candidate)
	if err != nil {
		if !core.HasCategory(err, core.ErrorUnavailable) {
			return PrepareInitialCandidateResultV2{}, err
		}
		createdCandidate, err = l.config.Candidates.InspectCandidateV2(context.WithoutCancel(ctx), request.Run, request.CandidateID)
		if err != nil {
			return PrepareInitialCandidateResultV2{}, err
		}
	}
	if !sameModelCandidateV2(createdCandidate, candidate) {
		return PrepareInitialCandidateResultV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "existing model candidate differs from requested preparation")
	}
	if sameGovernedSessionV2(createdSession, next) {
		return PrepareInitialCandidateResultV2{Session: createdSession, Candidate: createdCandidate}, nil
	}
	transitioned, err := l.config.Sessions.CompareAndSwapSessionV2(ctx, harnessports.SessionCASRequestV2{ExpectedRevision: 1, Next: next})
	if err != nil {
		if !core.HasCategory(err, core.ErrorUnavailable) && !core.HasCategory(err, core.ErrorConflict) {
			return PrepareInitialCandidateResultV2{}, err
		}
		transitioned, err = l.config.Sessions.InspectSessionV2(context.WithoutCancel(ctx), request.Run, request.SessionID)
		if err != nil {
			return PrepareInitialCandidateResultV2{}, err
		}
	}
	if !sameGovernedSessionV2(transitioned, next) {
		return PrepareInitialCandidateResultV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "governed session CAS recovery differs from requested preparation")
	}
	return PrepareInitialCandidateResultV2{Session: transitioned, Candidate: createdCandidate}, nil
}

type PrepareContinuationCandidateRequestV2 struct {
	Run                     contract.RunRef                   `json:"run"`
	SessionID               string                            `json:"session_id"`
	ExpectedSessionRevision core.Revision                     `json:"expected_session_revision"`
	CandidateID             string                            `json:"candidate_id"`
	Input                   runtimeports.OpaquePayloadV2      `json:"input"`
	ContextRef              string                            `json:"context_ref"`
	ContextDigest           core.Digest                       `json:"context_digest"`
	Continuation            contract.ContinuationRefV2        `json:"continuation"`
	Provider                runtimeports.ProviderBindingRefV2 `json:"provider"`
	CreatedUnixNano         int64                             `json:"created_unix_nano"`
	ExpiresUnixNano         int64                             `json:"expires_unix_nano"`
}

// PrepareContinuationCandidateV2 consumes only an exact, already-settled
// action/input continuation and emits a fresh model candidate. The settlement
// reference remains an observation here; its authoritative validation belongs
// to the domain owner and Application governance before this method is called.
func (l *GovernedLoopV2) PrepareContinuationCandidateV2(ctx context.Context, request PrepareContinuationCandidateRequestV2) (PrepareInitialCandidateResultV2, error) {
	if err := request.Run.Validate(); err != nil {
		return PrepareInitialCandidateResultV2{}, err
	}
	if strings.TrimSpace(request.SessionID) == "" || len(request.SessionID) > contract.MaxReferenceBytes || strings.TrimSpace(request.CandidateID) == "" || len(request.CandidateID) > contract.MaxReferenceBytes || strings.TrimSpace(request.ContextRef) == "" || len(request.ContextRef) > contract.MaxReferenceBytes {
		return PrepareInitialCandidateResultV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "continuation session, candidate and context references are required and bounded")
	}
	if err := request.Input.Validate(); err != nil {
		return PrepareInitialCandidateResultV2{}, err
	}
	if err := request.ContextDigest.Validate(); err != nil {
		return PrepareInitialCandidateResultV2{}, err
	}
	if err := request.Provider.Validate(); err != nil {
		return PrepareInitialCandidateResultV2{}, err
	}
	now := l.config.Clock()
	if request.ExpectedSessionRevision == 0 || request.CreatedUnixNano <= 0 || request.ExpiresUnixNano <= request.CreatedUnixNano || time.Duration(request.ExpiresUnixNano-request.CreatedUnixNano) > l.config.CandidateTTL || !now.Before(time.Unix(0, request.ExpiresUnixNano)) {
		return PrepareInitialCandidateResultV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonCapabilityExpired, "continuation candidate revision or lifetime is invalid")
	}
	if err := request.Continuation.Validate(); err != nil {
		return PrepareInitialCandidateResultV2{}, err
	}
	current, err := l.config.Sessions.InspectSessionV2(ctx, request.Run, request.SessionID)
	if err != nil {
		return PrepareInitialCandidateResultV2{}, err
	}
	if err := current.Validate(); err != nil {
		return PrepareInitialCandidateResultV2{}, err
	}
	if !sameRunRefV2(current.Run, request.Run) {
		return PrepareInitialCandidateResultV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "session store returned another Run identity")
	}
	if current.Revision == request.ExpectedSessionRevision+1 && current.Phase == contract.SessionWaitingModelDispatchV2 && current.Candidate != nil && current.Candidate.ID == request.CandidateID {
		candidate, inspectErr := l.config.Candidates.InspectCandidateV2(context.WithoutCancel(ctx), request.Run, request.CandidateID)
		if inspectErr != nil {
			return PrepareInitialCandidateResultV2{}, inspectErr
		}
		expected, buildErr := l.continuationCandidateV2(request, current.Endpoint, current.Turn, current.Candidate)
		if buildErr != nil {
			return PrepareInitialCandidateResultV2{}, buildErr
		}
		if !sameModelCandidateV2(candidate, expected) {
			return PrepareInitialCandidateResultV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "recovered continuation candidate differs from request")
		}
		candidateRef, refErr := candidate.RefV2()
		if refErr != nil || candidateRef != *current.Candidate {
			return PrepareInitialCandidateResultV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "session recovery does not bind the exact candidate fact")
		}
		return PrepareInitialCandidateResultV2{Session: current, Candidate: candidate}, nil
	}
	if current.Revision != request.ExpectedSessionRevision || (current.Phase != contract.SessionWaitingActionV2 && current.Phase != contract.SessionWaitingInputV2) {
		return PrepareInitialCandidateResultV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "session is not at the expected continuation revision")
	}
	if current.Phase == contract.SessionWaitingActionV2 {
		if request.Continuation.Kind != contract.CandidateActionTurnV2 || current.PendingAction == nil || request.Continuation.PendingRef != current.PendingAction.Ref || request.Continuation.PendingDigest != current.PendingAction.RequestDigest {
			return PrepareInitialCandidateResultV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidReference, "action continuation does not bind the exact pending action")
		}
	} else if request.Continuation.Kind != contract.CandidateInputTurnV2 || current.PendingInput == nil || request.Continuation.PendingRef != current.PendingInput.Ref || request.Continuation.PendingDigest != current.PendingInput.RequestDigest {
		return PrepareInitialCandidateResultV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidReference, "input continuation does not bind the exact pending input")
	}
	candidate, err := l.continuationCandidateV2(request, current.Endpoint, current.Turn+1, nil)
	if err != nil {
		return PrepareInitialCandidateResultV2{}, err
	}
	created, err := l.config.Candidates.CreateCandidateV2(ctx, candidate)
	if err != nil {
		if !core.HasCategory(err, core.ErrorUnavailable) && !core.HasCategory(err, core.ErrorConflict) {
			return PrepareInitialCandidateResultV2{}, err
		}
		created, err = l.config.Candidates.InspectCandidateV2(context.WithoutCancel(ctx), request.Run, request.CandidateID)
		if err != nil {
			return PrepareInitialCandidateResultV2{}, err
		}
	}
	if !sameModelCandidateV2(created, candidate) {
		return PrepareInitialCandidateResultV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "existing continuation candidate differs from request")
	}
	ref, err := candidate.RefV2()
	if err != nil {
		return PrepareInitialCandidateResultV2{}, err
	}
	next := current
	next.Revision++
	next.Phase = contract.SessionWaitingModelDispatchV2
	next.Turn++
	next.Candidate = &ref
	next.Execution = nil
	next.PendingAction = nil
	next.PendingInput = nil
	next.UpdatedUnixNano = request.CreatedUnixNano
	transitioned, err := l.config.Sessions.CompareAndSwapSessionV2(ctx, harnessports.SessionCASRequestV2{ExpectedRevision: current.Revision, Next: next})
	if err != nil {
		if !core.HasCategory(err, core.ErrorUnavailable) && !core.HasCategory(err, core.ErrorConflict) {
			return PrepareInitialCandidateResultV2{}, err
		}
		transitioned, err = l.config.Sessions.InspectSessionV2(context.WithoutCancel(ctx), request.Run, request.SessionID)
		if err != nil {
			return PrepareInitialCandidateResultV2{}, err
		}
	}
	if !sameGovernedSessionV2(transitioned, next) {
		return PrepareInitialCandidateResultV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "continuation session CAS recovery differs from request")
	}
	return PrepareInitialCandidateResultV2{Session: transitioned, Candidate: created}, nil
}

func (l *GovernedLoopV2) continuationCandidateV2(request PrepareContinuationCandidateRequestV2, endpoint contract.EndpointRefV2, turn uint32, existing *contract.CandidateRefV2) (contract.ModelTurnCandidateV2, error) {
	_ = existing // The caller uses it only to recognize the post-CAS recovery state.
	candidate := contract.ModelTurnCandidateV2{ContractVersion: contract.GovernedContractVersionV2, ID: request.CandidateID, Revision: 1, Run: request.Run, Endpoint: endpoint, SessionRef: request.SessionID, ExpectedSessionRevision: request.ExpectedSessionRevision, Turn: turn, Kind: request.Continuation.Kind, Input: request.Input, ContextRef: request.ContextRef, ContextDigest: request.ContextDigest, Continuation: &request.Continuation, Provider: request.Provider, CreatedUnixNano: request.CreatedUnixNano, ExpiresUnixNano: request.ExpiresUnixNano}
	return candidate, candidate.Validate(l.config.Clock())
}

func sameGovernedSessionV2(a, b contract.GovernedSessionV2) bool {
	ad, err := core.CanonicalJSONDigest("praxis.harness.governed", contract.GovernedContractVersionV2, "GovernedSessionV2", a)
	if err != nil {
		return false
	}
	bd, err := core.CanonicalJSONDigest("praxis.harness.governed", contract.GovernedContractVersionV2, "GovernedSessionV2", b)
	return err == nil && ad == bd
}

func sameModelCandidateV2(a, b contract.ModelTurnCandidateV2) bool {
	ad, err := a.DigestV2()
	if err != nil {
		return false
	}
	bd, err := b.DigestV2()
	return err == nil && ad == bd
}

func sameRunRefV2(a, b contract.RunRef) bool {
	ad, err := core.CanonicalJSONDigest("praxis.harness.governed", contract.GovernedContractVersionV2, "RunRefV2", a)
	if err != nil {
		return false
	}
	bd, err := core.CanonicalJSONDigest("praxis.harness.governed", contract.GovernedContractVersionV2, "RunRefV2", b)
	return err == nil && ad == bd
}
