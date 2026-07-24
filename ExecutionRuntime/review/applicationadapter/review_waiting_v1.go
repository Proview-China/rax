// Package applicationadapter projects Review Owner facts into the public
// Application review-waiting contract. It owns no Application, Harness or
// Runtime fact and never creates a Review Case from a partial neutral DTO.
package applicationadapter

import (
	"context"
	"reflect"
	"time"

	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/nilcheck"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type ClockV1 func() time.Time

// ReviewWaitingStoreV1 is the exact, read-only Review Owner surface required
// by the Application projection. In particular, it exposes no Case mutation.
type ReviewWaitingStoreV1 interface {
	InspectRequestExactV1(context.Context, core.TenantID, reviewport.ExactFactRefV1) (contract.ReviewRequestV1, error)
	InspectCaseV1(context.Context, core.TenantID, string) (contract.ReviewCaseV1, error)
	InspectCaseExactV1(context.Context, core.TenantID, reviewport.ExactFactRefV1) (contract.ReviewCaseV1, error)
	InspectTargetV1(context.Context, core.TenantID, string) (contract.TargetSnapshotV1, error)
	InspectVerdictExactV1(context.Context, core.TenantID, reviewport.ExactFactRefV1) (contract.VerdictV1, error)
}

type ReviewWaitingAdapterV1 struct {
	store ReviewWaitingStoreV1
	clock ClockV1
}

var _ applicationports.ReviewStartOrInspectPortV1 = (*ReviewWaitingAdapterV1)(nil)

func NewReviewWaitingAdapterV1(store ReviewWaitingStoreV1, clock ClockV1) (*ReviewWaitingAdapterV1, error) {
	if nilcheck.IsNil(store) || clock == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Review waiting adapter requires an exact Review Owner reader and clock")
	}
	return &ReviewWaitingAdapterV1{store: store, clock: clock}, nil
}

// StartOrInspectReviewV1 deliberately performs no mutation. The Application
// request carries the exact ReviewRequest coordinate, so the Review admission
// transaction must already exist. Authoritative NotFound is fail-closed and
// never grants this adapter permission to synthesize Request/Target/Case.
func (a *ReviewWaitingAdapterV1) StartOrInspectReviewV1(ctx context.Context, request applicationcontract.ReviewWaitingRequestV1) (applicationcontract.ReviewWaitingCurrentProjectionV1, error) {
	if err := request.Validate(); err != nil {
		return applicationcontract.ReviewWaitingCurrentProjectionV1{}, err
	}
	return a.projectV1(ctx, request.ReviewRequest, request.Target)
}

func (a *ReviewWaitingAdapterV1) InspectReviewV1(ctx context.Context, request applicationcontract.ReviewWaitingInspectRequestV1) (applicationcontract.ReviewWaitingCurrentProjectionV1, error) {
	if err := request.Validate(); err != nil {
		return applicationcontract.ReviewWaitingCurrentProjectionV1{}, err
	}
	return a.projectV1(ctx, request.Request, request.Target)
}

type reviewWaitingCutV1 struct {
	Request contract.ReviewRequestV1
	Case    contract.ReviewCaseV1
	Target  contract.TargetSnapshotV1
	Verdict *contract.VerdictV1
	// DecisionCase is the exact historical Case revision on which Verdict was
	// decided. It is nil while the current Case has no Verdict.
	DecisionCase *contract.ReviewCaseV1
}

func (a *ReviewWaitingAdapterV1) projectV1(ctx context.Context, request applicationcontract.ReviewRequestCoordinateV1, target applicationcontract.ReviewWaitingTargetCoordinateV1) (applicationcontract.ReviewWaitingCurrentProjectionV1, error) {
	baseline := a.clock()
	if baseline.IsZero() {
		return applicationcontract.ReviewWaitingCurrentProjectionV1{}, clockErrorV1("Review waiting baseline clock is unavailable")
	}
	recoveryCtx, cancel, ok := reviewWaitingRecoveryContextV1(ctx, baseline, target.ExpiresUnixNano)
	if ok {
		defer cancel()
	}
	first, detached, err := a.readCutV1(ctx, recoveryCtx, request, target, baseline)
	if err != nil {
		return applicationcontract.ReviewWaitingCurrentProjectionV1{}, err
	}
	afterS1 := a.clock()
	if afterS1.IsZero() || afterS1.Before(baseline) {
		return applicationcontract.ReviewWaitingCurrentProjectionV1{}, clockErrorV1("Review waiting projection clock regressed")
	}
	snapshotExpiry := minPositiveV1(first.Request.ExpiresUnixNano, first.Case.ExpiresUnixNano, first.Target.ExpiresUnixNano)
	if first.Verdict != nil {
		snapshotExpiry = minPositiveV1(snapshotExpiry, first.Verdict.ExpiresUnixNano)
	}
	if first.DecisionCase != nil {
		snapshotExpiry = minPositiveV1(snapshotExpiry, first.DecisionCase.ExpiresUnixNano)
	}
	s2Ctx, s2Cancel, s2Ready := reviewWaitingTightenRecoveryV1(recoveryCtx, afterS1, snapshotExpiry)
	if !s2Ready {
		return applicationcontract.ReviewWaitingCurrentProjectionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "Review waiting S1 snapshot expired before S2")
	}
	defer s2Cancel()
	readCtx := ctx
	if detached {
		readCtx = s2Ctx
	}
	second, _, err := a.readCutV1(readCtx, s2Ctx, request, target, afterS1)
	if err != nil {
		return applicationcontract.ReviewWaitingCurrentProjectionV1{}, err
	}
	actual := a.clock()
	if actual.IsZero() || actual.Before(afterS1) {
		return applicationcontract.ReviewWaitingCurrentProjectionV1{}, clockErrorV1("Review waiting actual-point clock regressed across S2")
	}
	if !reflect.DeepEqual(first, second) {
		return applicationcontract.ReviewWaitingCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonReviewVerdictStale, "Review waiting facts drifted between current reads")
	}
	if err := validateCutV1(second, request, target, actual); err != nil {
		return applicationcontract.ReviewWaitingCurrentProjectionV1{}, err
	}
	return projectCutV1(second, target, actual)
}

func (a *ReviewWaitingAdapterV1) readCutV1(ctx, recoveryCtx context.Context, expectedRequest applicationcontract.ReviewRequestCoordinateV1, expectedTarget applicationcontract.ReviewWaitingTargetCoordinateV1, now time.Time) (reviewWaitingCutV1, bool, error) {
	read := func(readContext context.Context) (reviewWaitingCutV1, error) {
		request, err := a.store.InspectRequestExactV1(readContext, expectedRequest.TenantID, reviewport.ExactV1(expectedRequest.ID, expectedRequest.Revision, expectedRequest.Digest))
		if err != nil {
			return reviewWaitingCutV1{}, err
		}
		caseFact, err := a.store.InspectCaseV1(readContext, expectedRequest.TenantID, expectedRequest.CaseID)
		if err != nil {
			return reviewWaitingCutV1{}, err
		}
		target, err := a.store.InspectTargetV1(readContext, expectedRequest.TenantID, expectedTarget.ID)
		if err != nil {
			return reviewWaitingCutV1{}, err
		}
		cut := reviewWaitingCutV1{Request: request, Case: caseFact, Target: target}
		if caseFact.VerdictID != "" {
			verdict, inspectErr := a.store.InspectVerdictExactV1(readContext, expectedRequest.TenantID, reviewport.ExactV1(caseFact.VerdictID, caseFact.VerdictRevision, caseFact.VerdictDigest))
			if inspectErr != nil {
				return reviewWaitingCutV1{}, inspectErr
			}
			cut.Verdict = &verdict
			decisionCase, inspectErr := a.store.InspectCaseExactV1(readContext, expectedRequest.TenantID, reviewport.ExactV1(verdict.CaseID, verdict.CaseRevision, verdict.CaseDigest))
			if inspectErr != nil {
				return reviewWaitingCutV1{}, inspectErr
			}
			cut.DecisionCase = &decisionCase
		}
		return cut, nil
	}
	cut, err := read(ctx)
	detached := ctx == recoveryCtx && recoveryCtx != nil
	if err != nil && (core.HasCategory(err, core.ErrorUnavailable) || core.HasCategory(err, core.ErrorIndeterminate)) {
		// This is a read-only boundary. One exact retry may recover a lost read
		// reply and cannot create or mutate any Review fact. The retry is bounded
		// by both five seconds and the request/target lifetime.
		originalUnknown := err
		if recoveryCtx == nil {
			return reviewWaitingCutV1{}, false, originalUnknown
		}
		cut, err = read(recoveryCtx)
		detached = true
		if err != nil || recoveryCtx.Err() != nil {
			return reviewWaitingCutV1{}, true, originalUnknown
		}
	}
	if err != nil {
		return reviewWaitingCutV1{}, false, err
	}
	if err := validateCutV1(cut, expectedRequest, expectedTarget, now); err != nil {
		return reviewWaitingCutV1{}, false, err
	}
	return cut, detached, nil
}

func reviewWaitingRecoveryContextV1(parent context.Context, now time.Time, expiries ...int64) (context.Context, context.CancelFunc, bool) {
	if now.IsZero() {
		return nil, nil, false
	}
	limit := 5 * time.Second
	for _, expiry := range expiries {
		if expiry <= 0 {
			continue
		}
		remaining := time.Unix(0, expiry).Sub(now)
		if remaining <= 0 {
			return nil, nil, false
		}
		if remaining < limit {
			limit = remaining
		}
	}
	recovery, cancel := context.WithTimeout(context.WithoutCancel(parent), limit)
	return recovery, cancel, true
}

func reviewWaitingTightenRecoveryV1(parent context.Context, now time.Time, expiries ...int64) (context.Context, context.CancelFunc, bool) {
	if parent == nil || now.IsZero() {
		return nil, nil, false
	}
	var limit time.Duration
	for _, expiry := range expiries {
		if expiry <= 0 {
			continue
		}
		remaining := time.Unix(0, expiry).Sub(now)
		if remaining <= 0 {
			return nil, nil, false
		}
		if limit == 0 || remaining < limit {
			limit = remaining
		}
	}
	if limit == 0 {
		limit = 5 * time.Second
	}
	tightened, cancel := context.WithTimeout(parent, limit)
	return tightened, cancel, true
}

func validateCutV1(cut reviewWaitingCutV1, expectedRequest applicationcontract.ReviewRequestCoordinateV1, expectedTarget applicationcontract.ReviewWaitingTargetCoordinateV1, now time.Time) error {
	if err := cut.Request.Validate(); err != nil {
		return err
	}
	if err := cut.Case.Validate(); err != nil {
		return err
	}
	if err := cut.Target.Validate(); err != nil {
		return err
	}
	if cut.Request.TenantID != expectedRequest.TenantID || cut.Request.ID != expectedRequest.ID || cut.Request.Revision != expectedRequest.Revision || cut.Request.Digest != expectedRequest.Digest || cut.Request.CaseID != expectedRequest.CaseID {
		return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "Review waiting Request exact coordinate drifted")
	}
	if cut.Case.TenantID != expectedRequest.TenantID || cut.Case.ID != expectedRequest.CaseID {
		return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "Review waiting current Case coordinate drifted")
	}
	if cut.Target.TenantID != expectedTarget.TenantID || cut.Target.ID != expectedTarget.ID || cut.Target.Revision != expectedTarget.Revision || cut.Target.Digest != expectedTarget.Digest || cut.Target.RunID != expectedTarget.RunID {
		return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "Review waiting Target exact coordinate drifted")
	}
	if cut.Request.TargetID != cut.Target.ID || cut.Request.TargetRevision != cut.Target.Revision || cut.Request.TargetDigest != cut.Target.Digest || cut.Case.TargetID != cut.Target.ID || cut.Case.TargetRevision != cut.Target.Revision || cut.Case.TargetDigest != cut.Target.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "Review waiting Request, Case and Target lineage drifted")
	}
	for _, lifetime := range []struct {
		created int64
		expires int64
	}{
		{cut.Request.CreatedUnixNano, cut.Request.ExpiresUnixNano},
		{cut.Case.CreatedUnixNano, cut.Case.ExpiresUnixNano},
		{cut.Target.CreatedUnixNano, cut.Target.ExpiresUnixNano},
	} {
		if err := contract.ValidateNow(now, lifetime.created, lifetime.expires); err != nil {
			return err
		}
	}
	if cut.Verdict != nil {
		if cut.DecisionCase == nil {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictMissing, "Review waiting Verdict lacks its exact predecessor Case")
		}
		if err := cut.DecisionCase.Validate(); err != nil {
			return err
		}
		if cut.Case.State != contract.CaseResolvedV1 || cut.Case.Revision != cut.Verdict.CaseRevision+1 || cut.Case.VerdictID != cut.Verdict.ID || cut.Case.VerdictRevision != cut.Verdict.Revision || cut.Case.VerdictDigest != cut.Verdict.Digest || cut.Verdict.TenantID != expectedRequest.TenantID || cut.Verdict.CaseID != cut.DecisionCase.ID || cut.Verdict.CaseRevision != cut.DecisionCase.Revision || cut.Verdict.CaseDigest != cut.DecisionCase.Digest || cut.DecisionCase.TenantID != expectedRequest.TenantID || cut.DecisionCase.ID != expectedRequest.CaseID || cut.DecisionCase.TargetID != cut.Target.ID || cut.DecisionCase.TargetRevision != cut.Target.Revision || cut.DecisionCase.TargetDigest != cut.Target.Digest || cut.Verdict.TargetID != cut.Target.ID || cut.Verdict.TargetRevision != cut.Target.Revision || cut.Verdict.TargetDigest != cut.Target.Digest {
			return core.NewError(core.ErrorConflict, core.ReasonReviewVerdictStale, "Review waiting Verdict does not atomically resolve the exact Case")
		}
		if err := contract.ValidateNow(now, cut.DecisionCase.CreatedUnixNano, cut.DecisionCase.ExpiresUnixNano); err != nil {
			return err
		}
		if err := contract.ValidateNow(now, cut.Verdict.CreatedUnixNano, cut.Verdict.ExpiresUnixNano); err != nil {
			return err
		}
	} else if cut.Case.State == contract.CaseResolvedV1 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictMissing, "resolved Review waiting Case lacks an exact Verdict")
	}
	return nil
}

func projectCutV1(cut reviewWaitingCutV1, target applicationcontract.ReviewWaitingTargetCoordinateV1, now time.Time) (applicationcontract.ReviewWaitingCurrentProjectionV1, error) {
	// The projection is a deterministic function of Review Owner facts and the
	// exact Target coordinate carried by both Start and Inspect. The Application
	// Waiting Request applies its own (possibly shorter) TTL in ValidateFor; this
	// adapter never reseals a different projection merely because the caller used
	// another delivery envelope.
	expires := minPositiveV1(target.ExpiresUnixNano, cut.Request.ExpiresUnixNano, cut.Case.ExpiresUnixNano, cut.Target.ExpiresUnixNano)
	decision := phaseDecisionV1(cut.Case, cut.Verdict)
	var verdictCoordinate *applicationcontract.ReviewWaitingVerdictCoordinateV1
	if cut.Verdict != nil {
		expires = minPositiveV1(expires, cut.Verdict.ExpiresUnixNano)
	}
	if cut.Verdict != nil && decision != applicationcontract.ReviewPhaseDeferV1 {
		verdictCoordinate = &applicationcontract.ReviewWaitingVerdictCoordinateV1{
			TenantID: cut.Verdict.TenantID, ID: cut.Verdict.ID, Revision: cut.Verdict.Revision, Digest: cut.Verdict.Digest,
			CaseID: cut.Verdict.CaseID, CaseRevision: cut.Verdict.CaseRevision, CaseDigest: cut.Verdict.CaseDigest,
			Target: target, ExpiresUnixNano: cut.Verdict.ExpiresUnixNano,
		}
	}
	if expires <= now.UnixNano() {
		return applicationcontract.ReviewWaitingCurrentProjectionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "Review waiting projection expired at the actual point")
	}
	checked := maxPositiveV1(cut.Request.UpdatedUnixNano, cut.Case.UpdatedUnixNano, cut.Target.UpdatedUnixNano)
	if cut.Verdict != nil {
		checked = maxPositiveV1(checked, cut.Verdict.UpdatedUnixNano)
	}
	if checked <= 0 || checked >= expires || now.UnixNano() < checked {
		return applicationcontract.ReviewWaitingCurrentProjectionV1{}, clockErrorV1("Review waiting projection fact clock is invalid")
	}
	return applicationcontract.SealReviewWaitingCurrentProjectionV1(applicationcontract.ReviewWaitingCurrentProjectionV1{
		RequestID: cut.Request.ID, RequestDigest: cut.Request.Digest,
		Case:    applicationcontract.ReviewWaitingCaseCoordinateV1{TenantID: cut.Case.TenantID, ID: cut.Case.ID, Revision: cut.Case.Revision, Digest: cut.Case.Digest, Target: target, ExpiresUnixNano: cut.Case.ExpiresUnixNano},
		Verdict: verdictCoordinate, Decision: decision, Current: true, CheckedUnixNano: checked, ExpiresUnixNano: expires,
	})
}

func phaseDecisionV1(caseFact contract.ReviewCaseV1, verdict *contract.VerdictV1) applicationcontract.ReviewPhaseDecisionV1 {
	if caseFact.State == contract.CaseResolvedV1 && verdict != nil {
		switch verdict.State {
		case contract.VerdictAcceptedV1:
			return applicationcontract.ReviewPhaseAllowV1
		case contract.VerdictRejectedV1:
			return applicationcontract.ReviewPhaseDenyV1
		case contract.VerdictConditionalV1:
			return applicationcontract.ReviewPhaseDeferV1
		}
	}
	switch caseFact.State {
	case contract.CaseWaitingRevisionV1, contract.CaseWaitingHumanV1, contract.CaseWaitingEvidenceV1:
		return applicationcontract.ReviewPhaseAskV1
	case contract.CaseExpiredV1, contract.CaseRevokedV1, contract.CaseSupersededV1, contract.CaseCancelledV1:
		return applicationcontract.ReviewPhaseDenyV1
	default:
		return applicationcontract.ReviewPhaseDeferV1
	}
}

func minPositiveV1(values ...int64) int64 {
	var minimum int64
	for _, value := range values {
		if value > 0 && (minimum == 0 || value < minimum) {
			minimum = value
		}
	}
	return minimum
}

func maxPositiveV1(values ...int64) int64 {
	var maximum int64
	for _, value := range values {
		if value > maximum {
			maximum = value
		}
	}
	return maximum
}

func clockErrorV1(message string) error {
	return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, message)
}
