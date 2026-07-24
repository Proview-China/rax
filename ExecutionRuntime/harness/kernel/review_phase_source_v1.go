package kernel

import (
	"context"
	"errors"
	"reflect"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/contract"
	harnessports "github.com/Proview-China/rax/ExecutionRuntime/harness/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type ReviewPhaseSourceCurrentReaderV1 struct {
	actions  harnessports.CommittedPendingActionReaderV3
	sessions harnessports.SessionCurrentReaderV4
	clock    func() time.Time
}

func NewReviewPhaseSourceCurrentReaderV1(actions harnessports.CommittedPendingActionReaderV3, sessions harnessports.SessionCurrentReaderV4, clock func() time.Time) (*ReviewPhaseSourceCurrentReaderV1, error) {
	if isNilDependencyV3(actions) || isNilDependencyV3(sessions) || clock == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Review phase source requires action/session current readers and clock")
	}
	return &ReviewPhaseSourceCurrentReaderV1{actions: actions, sessions: sessions, clock: clock}, nil
}

func (r *ReviewPhaseSourceCurrentReaderV1) InspectReviewPhaseSourceCurrentV1(ctx context.Context, request contract.ReviewPhaseSourceCurrentRequestV1) (contract.ReviewPhaseSourceCurrentProjectionV1, error) {
	if ctx == nil {
		return contract.ReviewPhaseSourceCurrentProjectionV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Review phase source context is required")
	}
	baseline, err := r.nowAfterReviewPhaseV1(time.Time{})
	if err != nil {
		return contract.ReviewPhaseSourceCurrentProjectionV1{}, err
	}
	if err := request.Validate(baseline); err != nil {
		return contract.ReviewPhaseSourceCurrentProjectionV1{}, err
	}
	if request.Source.Kind == contract.ReviewPhaseSubagentSourceV1 {
		return contract.ReviewPhaseSourceCurrentProjectionV1{}, core.NewError(core.ErrorCapabilityUnavailable, core.ReasonUnknownGovernanceCategory, "subagent Review phase has no exact Harness Owner reader")
	}

	s1, err := r.inspectReviewPhaseV1(ctx, request, baseline)
	if err != nil {
		return contract.ReviewPhaseSourceCurrentProjectionV1{}, err
	}
	afterS1, err := r.nowAfterReviewPhaseV1(baseline)
	if err != nil {
		return contract.ReviewPhaseSourceCurrentProjectionV1{}, err
	}
	if err := validateReviewPhaseInspectionV1(s1, request, afterS1); err != nil {
		return contract.ReviewPhaseSourceCurrentProjectionV1{}, err
	}
	s2, err := r.inspectReviewPhaseV1(ctx, request, afterS1)
	if err != nil {
		return contract.ReviewPhaseSourceCurrentProjectionV1{}, err
	}
	now, err := r.nowAfterReviewPhaseV1(afterS1)
	if err != nil {
		return contract.ReviewPhaseSourceCurrentProjectionV1{}, err
	}
	if err := validateReviewPhaseInspectionV1(s2, request, now); err != nil {
		return contract.ReviewPhaseSourceCurrentProjectionV1{}, err
	}
	closureS1, err := reviewPhaseInspectionClosureDigestV1(request.Source, s1)
	if err != nil {
		return contract.ReviewPhaseSourceCurrentProjectionV1{}, err
	}
	closureS2, err := reviewPhaseInspectionClosureDigestV1(request.Source, s2)
	if err != nil {
		return contract.ReviewPhaseSourceCurrentProjectionV1{}, err
	}
	if closureS1 != closureS2 {
		return contract.ReviewPhaseSourceCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Review phase source changed between S1 and S2")
	}
	expires := baseline.Add(contract.MaxReviewPhaseSourceProjectionTTLV1).UnixNano()
	if s1.expires > 0 && s1.expires < expires {
		expires = s1.expires
	}
	if s2.expires > 0 && s2.expires < expires {
		expires = s2.expires
	}
	if request.RequestedNotAfterUnixNano > 0 && request.RequestedNotAfterUnixNano < expires {
		expires = request.RequestedNotAfterUnixNano
	}
	if !now.Before(time.Unix(0, expires)) {
		return contract.ReviewPhaseSourceCurrentProjectionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonCapabilityExpired, "Review phase source TTL crossed during current reads")
	}
	projection := contract.ReviewPhaseSourceCurrentProjectionV1{
		Source: request.Source.Clone(), Run: s2.run, ExecutionScopeDigest: s2.scopeDigest,
		SessionID: s2.sessionID, SessionRevision: s2.sessionRevision, SessionDigest: s2.sessionDigest,
		Phase: s2.phase, Turn: s2.turn, CompletionClaim: s2.completionClaim,
		CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires,
	}
	if s2.action != nil {
		value := s2.action.Clone()
		projection.Action = &value
	}
	if s2.session != nil {
		value := s2.session.Clone()
		projection.RunSession = &value
	}
	return contract.SealReviewPhaseSourceCurrentProjectionV1(projection, request, now)
}

type reviewPhaseInspectionV1 struct {
	run             contract.RunRef
	scopeDigest     core.Digest
	sessionID       string
	sessionRevision core.Revision
	sessionDigest   core.Digest
	phase           contract.SessionPhaseV2
	turn            uint32
	completionClaim contract.CompletionClaim
	action          *contract.CommittedPendingActionCurrentV3
	session         *contract.GovernedSessionV4
	expires         int64
}

func (r *ReviewPhaseSourceCurrentReaderV1) inspectReviewPhaseV1(ctx context.Context, request contract.ReviewPhaseSourceCurrentRequestV1, now time.Time) (reviewPhaseInspectionV1, error) {
	switch request.Source.Kind {
	case contract.ReviewPhaseActionSourceV1:
		actionRequest := contract.CommittedPendingActionCurrentRequestV3{Subject: request.Source.Action.Subject.Clone(), RequestedNotAfterUnixNano: request.RequestedNotAfterUnixNano}
		value, err := r.actions.InspectCommittedPendingActionCurrentV3(ctx, actionRequest)
		if reviewPhaseRetryableReadV1(err) {
			value, err = r.actions.InspectCommittedPendingActionCurrentV3(context.WithoutCancel(ctx), actionRequest)
		}
		if err != nil {
			return reviewPhaseInspectionV1{}, err
		}
		return reviewPhaseInspectionV1{run: value.Run, scopeDigest: value.ExecutionScopeDigest, sessionID: value.SessionID, sessionRevision: value.SessionRevision, sessionDigest: value.SessionDigest, phase: value.Phase, turn: value.Turn, action: &value, expires: value.ExpiresUnixNano}, nil
	case contract.ReviewPhaseRunSourceV1:
		source := request.Source.Run
		value, err := r.sessions.InspectSessionV4(ctx, source.Run, source.SessionID)
		if reviewPhaseRetryableReadV1(err) {
			value, err = r.sessions.InspectSessionV4(context.WithoutCancel(ctx), source.Run, source.SessionID)
		}
		if err != nil {
			return reviewPhaseInspectionV1{}, err
		}
		return reviewPhaseInspectionV1{run: value.Run, scopeDigest: source.ExecutionScopeDigest, sessionID: value.ID, sessionRevision: value.Revision, sessionDigest: value.Digest, phase: value.Phase, turn: value.Turn, completionClaim: value.CompletionClaim, session: &value}, nil
	default:
		return reviewPhaseInspectionV1{}, core.NewError(core.ErrorCapabilityUnavailable, core.ReasonUnknownGovernanceCategory, "Review phase source kind has no Owner reader")
	}
}

func validateReviewPhaseInspectionV1(value reviewPhaseInspectionV1, request contract.ReviewPhaseSourceCurrentRequestV1, now time.Time) error {
	switch request.Source.Kind {
	case contract.ReviewPhaseActionSourceV1:
		if value.action == nil {
			return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "action Review phase read returned no action")
		}
		return value.action.ValidateAgainst(contract.CommittedPendingActionCurrentRequestV3{Subject: request.Source.Action.Subject.Clone(), RequestedNotAfterUnixNano: request.RequestedNotAfterUnixNano}, now)
	case contract.ReviewPhaseRunSourceV1:
		if value.session == nil || value.session.Validate() != nil || !reviewPhaseSameRunSourceV1(*value.session, *request.Source.Run) {
			return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "run Review phase Session differs from exact source")
		}
		return nil
	default:
		return core.NewError(core.ErrorCapabilityUnavailable, core.ReasonUnknownGovernanceCategory, "Review phase source kind has no current validation")
	}
}

func reviewPhaseInspectionClosureDigestV1(source contract.ReviewPhaseSourceRefV1, value reviewPhaseInspectionV1) (core.Digest, error) {
	projection := contract.ReviewPhaseSourceCurrentProjectionV1{Source: source.Clone(), Run: value.run, ExecutionScopeDigest: value.scopeDigest, SessionID: value.sessionID, SessionRevision: value.sessionRevision, SessionDigest: value.sessionDigest, Phase: value.phase, Turn: value.turn, CompletionClaim: value.completionClaim}
	if value.action != nil {
		action := value.action.Clone()
		projection.Action = &action
	}
	if value.session != nil {
		session := value.session.Clone()
		projection.RunSession = &session
	}
	return projection.ClosureDigestV1()
}

func reviewPhaseSameRunSourceV1(session contract.GovernedSessionV4, source contract.ReviewRunPhaseSourceRefV1) bool {
	return session.ID == source.SessionID && session.Revision == source.SessionRevision && session.Digest == source.SessionDigest && session.Run.RunID == source.Run.RunID && reflect.DeepEqual(session.Run.Scope, source.Run.Scope) && session.Phase == source.Phase && session.Turn == source.Turn && session.CompletionClaim == source.CompletionClaim
}

func (r *ReviewPhaseSourceCurrentReaderV1) nowAfterReviewPhaseV1(previous time.Time) (time.Time, error) {
	now := r.clock()
	if now.IsZero() || !previous.IsZero() && now.Before(previous) {
		return time.Time{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Review phase source clock is zero or moved backwards")
	}
	return now, nil
}

func reviewPhaseRetryableReadV1(err error) bool {
	return err != nil && (errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || core.HasCategory(err, core.ErrorUnavailable) || core.HasCategory(err, core.ErrorIndeterminate))
}

var _ harnessports.ReviewPhaseSourceCurrentReaderV1 = (*ReviewPhaseSourceCurrentReaderV1)(nil)
