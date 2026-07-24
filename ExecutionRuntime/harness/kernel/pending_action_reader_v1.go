package kernel

import (
	"context"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/contract"
	harnessports "github.com/Proview-China/rax/ExecutionRuntime/harness/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const MaxCommittedPendingActionProjectionTTLV1 = 30 * time.Second

// CommittedPendingActionReaderV1 performs an S1/S2 currentness read without
// creating Candidates, Evidence, Settlements, or exposing Session writes.
type CommittedPendingActionReaderV1 struct {
	sessions      harnessports.SessionFactPortV2
	clock         func() time.Time
	projectionTTL time.Duration
}

var _ harnessports.CommittedPendingActionReaderV1 = (*CommittedPendingActionReaderV1)(nil)

func NewCommittedPendingActionReaderV1(sessions harnessports.SessionFactPortV2, clock func() time.Time, projectionTTL time.Duration) (*CommittedPendingActionReaderV1, error) {
	if sessions == nil || clock == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "committed PendingAction reader requires Session facts and a clock")
	}
	if projectionTTL <= 0 || projectionTTL > MaxCommittedPendingActionProjectionTTLV1 {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonCapabilityExpired, "committed PendingAction projection TTL must be positive and short")
	}
	return &CommittedPendingActionReaderV1{sessions: sessions, clock: clock, projectionTTL: projectionTTL}, nil
}

func (r *CommittedPendingActionReaderV1) InspectCommittedPendingActionCurrentV1(ctx context.Context, request contract.InspectCommittedPendingActionCurrentRequestV1) (contract.CommittedPendingActionCurrentV1, error) {
	if r == nil || r.sessions == nil || r.clock == nil {
		return contract.CommittedPendingActionCurrentV1{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "committed PendingAction reader is unavailable")
	}
	if err := request.Validate(); err != nil {
		return contract.CommittedPendingActionCurrentV1{}, err
	}
	now := r.clock()
	if now.IsZero() || now.UnixNano() < request.CheckedAtUnixNano {
		return contract.CommittedPendingActionCurrentV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectFenceStale, "committed PendingAction clock rolled back before the requested observation")
	}

	s1, s1Digest, err := r.inspectExactV1(ctx, request)
	if err != nil {
		return contract.CommittedPendingActionCurrentV1{}, err
	}
	s2, s2Digest, err := r.inspectExactV1(ctx, request)
	if err != nil {
		return contract.CommittedPendingActionCurrentV1{}, err
	}
	if s1Digest != s2Digest || s1.Revision != s2.Revision || s1.Phase != s2.Phase || s1.Turn != s2.Turn || s1.PendingAction == nil || s2.PendingAction == nil || s1.PendingAction.Ref != s2.PendingAction.Ref || s1.PendingAction.RequestDigest != s2.PendingAction.RequestDigest {
		return contract.CommittedPendingActionCurrentV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "committed PendingAction changed between S1 and S2")
	}

	expires := now.Add(r.projectionTTL)
	projection := contract.CommittedPendingActionCurrentV1{
		Run: request.Run, ExecutionScopeDigest: request.ExecutionScopeDigest,
		SessionID: s2.ID, SessionRevision: s2.Revision, SessionDigest: s2Digest,
		Phase: s2.Phase, Turn: s2.Turn, PendingAction: *s2.PendingAction,
		CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires.UnixNano(),
	}
	return contract.SealCommittedPendingActionCurrentV1(projection, request, now)
}

func (r *CommittedPendingActionReaderV1) inspectExactV1(ctx context.Context, request contract.InspectCommittedPendingActionCurrentRequestV1) (contract.GovernedSessionV2, core.Digest, error) {
	session, err := r.sessions.InspectSessionV2(ctx, request.Run, request.SessionID)
	if err != nil {
		return contract.GovernedSessionV2{}, "", err
	}
	if err := session.Validate(); err != nil {
		return contract.GovernedSessionV2{}, "", err
	}
	if session.Run.RunID != request.Run.RunID || !runtimeports.SameExecutionScopeV2(session.Run.Scope, request.Run.Scope) {
		return contract.GovernedSessionV2{}, "", core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectFenceStale, "committed PendingAction Session belongs to another Run or Execution Scope")
	}
	if session.ID != request.SessionID || session.Revision != request.ExpectedSessionRevision {
		return contract.GovernedSessionV2{}, "", core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "committed PendingAction Session identity or revision drifted")
	}
	if session.Phase != contract.SessionWaitingActionV2 || session.Turn != request.ExpectedTurn || session.PendingAction == nil {
		return contract.GovernedSessionV2{}, "", core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "Session is not the exact waiting_action turn")
	}
	if session.PendingAction.Ref != request.ExpectedPendingActionRef || session.PendingAction.RequestDigest != request.ExpectedPendingActionDigest {
		return contract.GovernedSessionV2{}, "", core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "committed PendingAction ref or digest drifted")
	}
	digest, err := session.DigestV2()
	if err != nil {
		return contract.GovernedSessionV2{}, "", err
	}
	return session, digest, nil
}
