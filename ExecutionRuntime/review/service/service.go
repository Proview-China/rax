// Package service exposes the Review Owner's application-facing orchestration
// without adding a direct Verdict, dispatch or commit path.
package service

import (
	"context"
	"reflect"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/caseengine"
	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/nilcheck"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const lostReplyRecoveryTimeoutV1 = 5 * time.Second

type Clock func() time.Time

type StoreV1 interface {
	reviewport.StoreV1
	reviewport.CaseQueryStoreV1
	reviewport.EvidenceAttachmentStoreV1
	reviewport.TraceEventStoreV2
}

type Service struct {
	store StoreV1
	cases *caseengine.Engine
	clock Clock
}

func New(store StoreV1, clock Clock) (*Service, error) {
	if nilcheck.IsNil(store) || clock == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "review service requires store and clock")
	}
	cases, err := caseengine.New(store, caseengine.Clock(clock))
	if err != nil {
		return nil, err
	}
	return &Service{store: store, cases: cases, clock: clock}, nil
}

type SubmitCommandV1 struct {
	Request      contract.ReviewRequestV1       `json:"request"`
	ResultBundle *contract.ReviewResultBundleV1 `json:"result_bundle,omitempty"`
	Target       contract.TargetSnapshotV1      `json:"target"`
	Trace        contract.TraceFactV1           `json:"trace,omitempty"`
}

type ReviewViewV1 struct {
	Request      *contract.ReviewRequestV1      `json:"request,omitempty"`
	ResultBundle *contract.ReviewResultBundleV1 `json:"result_bundle,omitempty"`
	Case         contract.ReviewCaseV1          `json:"case"`
	Target       contract.TargetSnapshotV1      `json:"target"`
	Verdict      *contract.VerdictV1            `json:"verdict,omitempty"`
	Current      bool                           `json:"current"`
}

func (s *Service) SubmitV1(ctx context.Context, command SubmitCommandV1) (ReviewViewV1, error) {
	baseline := s.clock()
	if baseline.IsZero() {
		return ReviewViewV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "review submit clock is unavailable")
	}
	if err := command.Request.ValidateTarget(command.Target, baseline); err != nil {
		return ReviewViewV1{}, err
	}
	rubricS1, err := s.store.InspectRubricCurrentV1(ctx, command.Request.TenantID, command.Request.Rubric, baseline)
	if err != nil {
		return ReviewViewV1{}, err
	}
	if command.Request.ExpiresUnixNano > rubricS1.ExpiresUnixNano {
		return ReviewViewV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "Review Request exceeds its exact Rubric currentness window")
	}
	if command.ResultBundle != nil {
		if command.Request.ResultBundle == nil || command.ResultBundle.TenantID != command.Request.TenantID || command.Request.ResultBundle.ID != command.ResultBundle.ID || command.Request.ResultBundle.Revision != command.ResultBundle.Revision || command.Request.ResultBundle.Digest != command.ResultBundle.Digest {
			return ReviewViewV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Review Request and Result Bundle exact ref drifted")
		}
		if err := command.ResultBundle.Validate(); err != nil {
			return ReviewViewV1{}, err
		}
		if err := contract.ValidateNow(baseline, command.ResultBundle.CreatedUnixNano, command.ResultBundle.ExpiresUnixNano); err != nil {
			return ReviewViewV1{}, err
		}
	} else if command.Request.ResultBundle != nil {
		return ReviewViewV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceUnavailable, "Review Request requires its exact Result Bundle")
	}
	fresh := s.clock()
	if fresh.IsZero() || fresh.Before(baseline) {
		return ReviewViewV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "review submit Rubric clock regressed")
	}
	rubricS2, err := s.store.InspectRubricCurrentV1(ctx, command.Request.TenantID, command.Request.Rubric, fresh)
	if err != nil || rubricS2.Digest != rubricS1.Digest {
		if err != nil {
			return ReviewViewV1{}, err
		}
		return ReviewViewV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Review Request Rubric drifted between admission reads")
	}
	caseFact, err := s.cases.CreateCaseV1(ctx, caseengine.CreateCaseCommandV1{CaseID: command.Request.CaseID, Request: &command.Request, ResultBundle: command.ResultBundle, Target: command.Target, ExpiresUnixNano: command.Request.ExpiresUnixNano, Trace: command.Trace})
	if err != nil {
		return ReviewViewV1{}, err
	}
	expiries := []int64{command.Request.ExpiresUnixNano, command.Target.ExpiresUnixNano, rubricS2.ExpiresUnixNano, caseFact.ExpiresUnixNano}
	if command.ResultBundle != nil {
		expiries = append(expiries, command.ResultBundle.ExpiresUnixNano)
	}
	postMutation, cancel, ok := s.boundedRecoveryContextV1(ctx, fresh, expiries...)
	if !ok {
		return ReviewViewV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "review submit exact post-mutation closure crossed its currentness window")
	}
	defer cancel()
	storedRequest, err := s.store.InspectRequestExactV1(postMutation, command.Request.TenantID, reviewport.ExactV1(command.Request.ID, command.Request.Revision, command.Request.Digest))
	if err != nil || !reflect.DeepEqual(storedRequest, command.Request) {
		if err != nil {
			return ReviewViewV1{}, err
		}
		return ReviewViewV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "stored Review Request drifted")
	}
	view := ReviewViewV1{Request: &storedRequest, Case: caseFact, Target: command.Target, Current: true}
	if command.ResultBundle != nil {
		storedBundle, inspectErr := s.store.InspectResultBundleExactV1(postMutation, command.ResultBundle.TenantID, reviewport.ExactV1(command.ResultBundle.ID, command.ResultBundle.Revision, command.ResultBundle.Digest))
		if inspectErr != nil {
			return ReviewViewV1{}, inspectErr
		}
		if !reflect.DeepEqual(storedBundle, *command.ResultBundle) {
			return ReviewViewV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "stored Review Result Bundle drifted")
		}
		view.ResultBundle = &storedBundle
	}
	if !s.recoveryStillCurrentV1(fresh, expiries...) {
		return ReviewViewV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "review submit exact post-mutation closure expired while reading")
	}
	return view, nil
}

func (s *Service) InspectV1(ctx context.Context, tenant core.TenantID, caseID string) (ReviewViewV1, error) {
	caseFact, err := s.store.InspectCaseV1(ctx, tenant, caseID)
	if err != nil {
		return ReviewViewV1{}, err
	}
	target, err := s.store.InspectTargetExactV1(ctx, tenant, reviewport.ExactV1(caseFact.TargetID, caseFact.TargetRevision, caseFact.TargetDigest))
	if err != nil {
		return ReviewViewV1{}, err
	}
	view := ReviewViewV1{Case: caseFact, Target: target, Current: true}
	request, requestErr := s.store.InspectRequestByCaseV1(ctx, tenant, caseID)
	if requestErr == nil {
		view.Request = &request
		if request.ResultBundle != nil {
			bundle, inspectErr := s.store.InspectResultBundleExactV1(ctx, tenant, reviewport.ExactV1(request.ResultBundle.ID, request.ResultBundle.Revision, request.ResultBundle.Digest))
			if inspectErr != nil {
				return ReviewViewV1{}, inspectErr
			}
			view.ResultBundle = &bundle
		}
	} else if !core.HasCategory(requestErr, core.ErrorNotFound) {
		return ReviewViewV1{}, requestErr
	}
	if caseFact.VerdictID != "" {
		verdict, inspectErr := s.store.InspectVerdictExactV1(ctx, tenant, reviewport.ExactV1(caseFact.VerdictID, caseFact.VerdictRevision, caseFact.VerdictDigest))
		if inspectErr != nil {
			return ReviewViewV1{}, inspectErr
		}
		view.Verdict = &verdict
	}
	return view, nil
}

func (s *Service) ListV1(ctx context.Context, request reviewport.ListCasesRequestV1) (reviewport.ListCasesResultV1, error) {
	return s.store.ListCasesV1(ctx, request)
}

func (s *Service) EventsV1(ctx context.Context, tenant core.TenantID, caseID string) ([]contract.TraceFactV1, error) {
	if _, err := s.store.InspectCaseV1(ctx, tenant, caseID); err != nil {
		return nil, err
	}
	return s.store.ListTraceV1(ctx, tenant, caseID)
}

func (s *Service) EventsPageV2(ctx context.Context, request reviewport.ListTracePageRequestV2) (reviewport.ListTracePageResultV2, error) {
	return s.store.ListTracePageV2(ctx, request)
}

func (s *Service) StartRoundV1(ctx context.Context, mutation reviewport.StartRoundMutationV1) (contract.ReviewCaseV1, contract.ReviewRoundV1, contract.ReviewerAssignmentV1, error) {
	return s.cases.StartRoundV1(ctx, mutation)
}

func (s *Service) ClaimV1(ctx context.Context, mutation reviewport.ClaimAssignmentMutationV1) (contract.ReviewCaseV1, contract.ReviewerAssignmentV1, error) {
	if len(mutation.Traces) != 1 || mutation.Traces[0].Event != contract.TraceStartedV1 {
		return contract.ReviewCaseV1{}, contract.ReviewerAssignmentV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "production review Claim requires exactly one ReviewStarted Trace")
	}
	baseline := s.clock()
	if baseline.IsZero() {
		return contract.ReviewCaseV1{}, contract.ReviewerAssignmentV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "review Claim clock is unavailable")
	}
	previousCase, err := s.store.InspectCaseExactV1(ctx, mutation.TenantID, reviewport.ExactV1(mutation.CaseID, mutation.ExpectedCase.Revision, mutation.ExpectedCase.Digest))
	if err != nil {
		return contract.ReviewCaseV1{}, contract.ReviewerAssignmentV1{}, err
	}
	previousAssignment, err := s.store.InspectAssignmentExactV1(ctx, mutation.TenantID, reviewport.ExactV1(mutation.AssignmentID, mutation.ExpectedAssignment.Revision, mutation.ExpectedAssignment.Digest))
	if err != nil {
		return contract.ReviewCaseV1{}, contract.ReviewerAssignmentV1{}, err
	}
	actualPoint := s.clock()
	if actualPoint.IsZero() || actualPoint.Before(baseline) {
		return contract.ReviewCaseV1{}, contract.ReviewerAssignmentV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "review Claim clock regressed")
	}
	caseFact, assignment, err := s.cases.ClaimAssignmentV1(ctx, mutation)
	if err == nil || (!core.HasCategory(err, core.ErrorUnavailable) && !core.HasCategory(err, core.ErrorIndeterminate)) {
		return caseFact, assignment, err
	}
	nextAssignment := previousAssignment
	nextAssignment.Revision++
	nextAssignment.State = contract.AssignmentClaimedV1
	nextAssignment.LeaseHolder = mutation.LeaseHolder
	nextAssignment.LeaseExpiresUnixNano = mutation.LeaseExpiresUnixNano
	nextAssignment.UpdatedUnixNano = mutation.UpdatedUnixNano
	nextAssignment.Digest = ""
	nextAssignment, sealErr := contract.SealReviewerAssignmentV1(nextAssignment)
	if sealErr != nil {
		return contract.ReviewCaseV1{}, contract.ReviewerAssignmentV1{}, sealErr
	}
	nextCase := previousCase
	nextCase.Revision++
	nextCase.State = contract.CaseReviewingV1
	nextCase.UpdatedUnixNano = mutation.UpdatedUnixNano
	nextCase.Digest = ""
	nextCase, sealErr = contract.SealReviewCaseV1(nextCase)
	if sealErr != nil {
		return contract.ReviewCaseV1{}, contract.ReviewerAssignmentV1{}, sealErr
	}
	recovery, cancel, ok := s.boundedRecoveryContextV1(ctx, actualPoint, nextCase.ExpiresUnixNano, nextAssignment.ExpiresUnixNano, nextAssignment.LeaseExpiresUnixNano)
	if !ok {
		return contract.ReviewCaseV1{}, contract.ReviewerAssignmentV1{}, err
	}
	defer cancel()
	caseFact, inspectErr := s.store.InspectCaseExactV1(recovery, mutation.TenantID, reviewport.ExactV1(nextCase.ID, nextCase.Revision, nextCase.Digest))
	if inspectErr != nil {
		return contract.ReviewCaseV1{}, contract.ReviewerAssignmentV1{}, err
	}
	assignment, inspectErr = s.store.InspectAssignmentExactV1(recovery, mutation.TenantID, reviewport.ExactV1(nextAssignment.ID, nextAssignment.Revision, nextAssignment.Digest))
	if inspectErr != nil {
		return contract.ReviewCaseV1{}, contract.ReviewerAssignmentV1{}, err
	}
	if inspectErr = inspectTraceBatchExactV2(recovery, s.store, mutation.Traces); inspectErr != nil {
		return contract.ReviewCaseV1{}, contract.ReviewerAssignmentV1{}, err
	}
	if !reflect.DeepEqual(caseFact, nextCase) || !reflect.DeepEqual(assignment, nextAssignment) || !s.recoveryStillCurrentV1(actualPoint, nextCase.ExpiresUnixNano, nextAssignment.ExpiresUnixNano, nextAssignment.LeaseExpiresUnixNano) {
		return contract.ReviewCaseV1{}, contract.ReviewerAssignmentV1{}, err
	}
	return caseFact, assignment, nil
}

func (s *Service) AttestV1(ctx context.Context, expected reviewport.ExpectedFactV1, attestation contract.AttestationV1, trace contract.TraceFactV1) (contract.ReviewCaseV1, contract.AttestationV1, error) {
	return s.cases.RecordAttestationV1(ctx, expected, attestation, trace)
}

func (s *Service) AttestWithTraceV2(ctx context.Context, expected reviewport.ExpectedFactV1, attestation contract.AttestationV1, trace contract.TraceFactV1, additional []contract.TraceFactV1) (contract.ReviewCaseV1, contract.AttestationV1, error) {
	return s.cases.RecordAttestationWithTraceV2(ctx, expected, attestation, trace, additional)
}

type CancelCommandV1 struct {
	TenantID core.TenantID             `json:"tenant_id"`
	CaseID   string                    `json:"case_id"`
	Expected reviewport.ExpectedFactV1 `json:"expected"`
	Reason   core.ReasonCode           `json:"reason"`
	Trace    contract.TraceFactV1      `json:"trace"`
}

func (s *Service) CancelV1(ctx context.Context, command CancelCommandV1) (contract.ReviewCaseV1, error) {
	latest, latestErr := s.store.InspectCaseV1(ctx, command.TenantID, command.CaseID)
	if latestErr != nil {
		return contract.ReviewCaseV1{}, latestErr
	}
	if latest.State == contract.CaseCancelledV1 && latest.Revision == command.Expected.Revision+1 && latest.InvalidationReason == command.Reason {
		previous, err := s.store.InspectCaseExactV1(ctx, command.TenantID, reviewport.ExactV1(command.CaseID, command.Expected.Revision, command.Expected.Digest))
		if err != nil {
			return contract.ReviewCaseV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "review cancel replay lost its exact predecessor Case")
		}
		if previous.VerdictID != "" {
			oldVerdict, inspectErr := s.store.InspectVerdictExactV1(ctx, previous.TenantID, reviewport.ExactV1(previous.VerdictID, previous.VerdictRevision, previous.VerdictDigest))
			if inspectErr != nil {
				return contract.ReviewCaseV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "review cancel replay lost its predecessor Verdict")
			}
			expectedVerdict := oldVerdict
			expectedVerdict.Revision++
			expectedVerdict.State = contract.VerdictRevokedV1
			expectedVerdict.UpdatedUnixNano = latest.UpdatedUnixNano
			expectedVerdict.InvalidationReason = command.Reason
			expectedVerdict.Digest = ""
			expectedVerdict, inspectErr = contract.SealVerdictV1(expectedVerdict)
			if inspectErr != nil {
				return contract.ReviewCaseV1{}, inspectErr
			}
			if _, inspectErr = s.store.InspectVerdictExactV1(ctx, expectedVerdict.TenantID, reviewport.ExactV1(expectedVerdict.ID, expectedVerdict.Revision, expectedVerdict.Digest)); inspectErr != nil {
				return contract.ReviewCaseV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "review cancel replay changed its exact Verdict")
			}
		}
		if _, err := s.store.InspectTraceExactV1(ctx, command.Trace.TenantID, reviewport.ExactV1(command.Trace.ID, command.Trace.Revision, command.Trace.Digest)); err != nil {
			return contract.ReviewCaseV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "review cancel replay changed its exact Trace")
		}
		return latest, nil
	}
	if latest.Revision != command.Expected.Revision || latest.Digest != command.Expected.Digest {
		return contract.ReviewCaseV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "review cancel expected Case is stale")
	}
	current, err := s.store.InspectCaseExactV1(ctx, command.TenantID, reviewport.ExactV1(command.CaseID, command.Expected.Revision, command.Expected.Digest))
	if err != nil {
		return contract.ReviewCaseV1{}, err
	}
	now := s.clock()
	if now.IsZero() || now.UnixNano() <= current.UpdatedUnixNano {
		return contract.ReviewCaseV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "review cancel clock did not advance")
	}
	var expectedVerdict *contract.VerdictV1
	if current.VerdictID != "" {
		oldVerdict, inspectErr := s.store.InspectVerdictExactV1(ctx, current.TenantID, reviewport.ExactV1(current.VerdictID, current.VerdictRevision, current.VerdictDigest))
		if inspectErr != nil {
			return contract.ReviewCaseV1{}, inspectErr
		}
		nextVerdict := oldVerdict
		nextVerdict.Revision++
		nextVerdict.State = contract.VerdictRevokedV1
		nextVerdict.UpdatedUnixNano = now.UnixNano()
		nextVerdict.InvalidationReason = command.Reason
		nextVerdict.Digest = ""
		nextVerdict, inspectErr = contract.SealVerdictV1(nextVerdict)
		if inspectErr != nil {
			return contract.ReviewCaseV1{}, inspectErr
		}
		expectedVerdict = &nextVerdict
	}
	result, _, err := s.store.InvalidateV1(ctx, reviewport.InvalidateMutationV1{TenantID: command.TenantID, Expected: command.Expected, CaseID: command.CaseID, CaseState: contract.CaseCancelledV1, VerdictState: contract.VerdictRevokedV1, Reason: command.Reason, UpdatedUnixNano: now.UnixNano(), Trace: command.Trace})
	if err == nil || (!core.HasCategory(err, core.ErrorUnavailable) && !core.HasCategory(err, core.ErrorIndeterminate)) {
		return result, err
	}
	expected := current
	expected.Revision++
	expected.State = contract.CaseCancelledV1
	expected.VerdictID, expected.VerdictRevision, expected.VerdictDigest = "", 0, ""
	expected.UpdatedUnixNano = now.UnixNano()
	expected.InvalidationReason = command.Reason
	expected.Digest = ""
	expected, sealErr := contract.SealReviewCaseV1(expected)
	if sealErr != nil {
		return contract.ReviewCaseV1{}, sealErr
	}
	recoveryCtx, cancel, ok := s.boundedRecoveryContextV1(ctx, now, expected.ExpiresUnixNano, expiryOfVerdictV1(expectedVerdict))
	if !ok {
		return contract.ReviewCaseV1{}, err
	}
	defer cancel()
	inspected, inspectErr := s.store.InspectCaseExactV1(recoveryCtx, expected.TenantID, reviewport.ExactV1(expected.ID, expected.Revision, expected.Digest))
	if inspectErr != nil {
		return contract.ReviewCaseV1{}, err
	}
	storedTrace, inspectErr := s.store.InspectTraceExactV1(recoveryCtx, command.Trace.TenantID, reviewport.ExactV1(command.Trace.ID, command.Trace.Revision, command.Trace.Digest))
	if inspectErr != nil || !reflect.DeepEqual(storedTrace, command.Trace) {
		return contract.ReviewCaseV1{}, err
	}
	if expectedVerdict != nil {
		storedVerdict, verdictErr := s.store.InspectVerdictExactV1(recoveryCtx, expectedVerdict.TenantID, reviewport.ExactV1(expectedVerdict.ID, expectedVerdict.Revision, expectedVerdict.Digest))
		if verdictErr != nil || !reflect.DeepEqual(storedVerdict, *expectedVerdict) {
			return contract.ReviewCaseV1{}, err
		}
	}
	if !reflect.DeepEqual(inspected, expected) || !s.recoveryStillCurrentV1(now, expected.ExpiresUnixNano, expiryOfVerdictV1(expectedVerdict)) {
		return contract.ReviewCaseV1{}, err
	}
	return inspected, nil
}

func (s *Service) CreateFindingWithTraceV2(ctx context.Context, mutation reviewport.CreateFindingWithTraceMutationV2) (contract.FindingV1, error) {
	baseline := s.clock()
	if baseline.IsZero() || contract.ValidateNow(baseline, mutation.Finding.CreatedUnixNano, mutation.Finding.ExpiresUnixNano) != nil {
		return contract.FindingV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "review Finding clock is unavailable or outside its TTL")
	}
	actualPoint := s.clock()
	if actualPoint.IsZero() || actualPoint.Before(baseline) || contract.ValidateNow(actualPoint, mutation.Finding.CreatedUnixNano, mutation.Finding.ExpiresUnixNano) != nil {
		return contract.FindingV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "review Finding clock regressed or crossed its TTL")
	}
	created, err := s.store.CreateFindingWithTraceV2(ctx, mutation)
	if err == nil || (!core.HasCategory(err, core.ErrorUnavailable) && !core.HasCategory(err, core.ErrorIndeterminate)) {
		return created, err
	}
	recovery, cancel, ok := s.boundedRecoveryContextV1(ctx, actualPoint, mutation.Finding.ExpiresUnixNano)
	if !ok {
		return contract.FindingV1{}, err
	}
	defer cancel()
	created, inspectErr := s.store.InspectFindingExactV1(recovery, mutation.Finding.TenantID, reviewport.ExactV1(mutation.Finding.ID, mutation.Finding.Revision, mutation.Finding.Digest))
	if inspectErr != nil {
		return contract.FindingV1{}, err
	}
	if inspectErr = inspectTraceBatchExactV2(recovery, s.store, []contract.TraceFactV1{mutation.Trace}); inspectErr != nil {
		return contract.FindingV1{}, err
	}
	if !reflect.DeepEqual(created, mutation.Finding) || !s.recoveryStillCurrentV1(actualPoint, mutation.Finding.ExpiresUnixNano) {
		return contract.FindingV1{}, err
	}
	return created, nil
}

func (s *Service) CreateFindingForReviewerWithTraceV2(ctx context.Context, reviewerID string, mutation reviewport.CreateFindingWithTraceMutationV2) (contract.FindingV1, error) {
	value := mutation.Finding
	round, err := s.store.InspectRoundExactV1(ctx, value.TenantID, reviewport.ExactV1(value.RoundID, value.RoundRevision, value.RoundDigest))
	if err != nil {
		return contract.FindingV1{}, err
	}
	assignment, err := s.store.InspectAssignmentV1(ctx, value.TenantID, round.AssignmentID)
	if err != nil {
		return contract.FindingV1{}, err
	}
	now := s.clock()
	if now.IsZero() || now.UnixNano() < assignment.UpdatedUnixNano {
		return contract.FindingV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "review Finding clock regressed")
	}
	if assignment.State != contract.AssignmentClaimedV1 || assignment.ReviewerID != reviewerID || assignment.LeaseHolder != reviewerID || now.UnixNano() >= assignment.LeaseExpiresUnixNano {
		return contract.FindingV1{}, core.NewError(core.ErrorForbidden, core.ReasonIdentityLeaseConflict, "review Finding requires the current claimed reviewer lease")
	}
	return s.CreateFindingWithTraceV2(ctx, mutation)
}

type exactTraceReaderV2 interface {
	InspectTraceExactV1(context.Context, core.TenantID, reviewport.ExactFactRefV1) (contract.TraceFactV1, error)
}

func inspectTraceBatchExactV2(ctx context.Context, reader exactTraceReaderV2, events []contract.TraceFactV1) error {
	for _, event := range events {
		stored, err := reader.InspectTraceExactV1(ctx, event.TenantID, reviewport.ExactV1(event.ID, event.Revision, event.Digest))
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(stored, event) {
			return core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "review Trace exact read returned different content")
		}
	}
	return nil
}

func (s *Service) CreateBehaviorFeedbackCandidateV1(ctx context.Context, value contract.BehaviorFeedbackCandidateV1) (contract.BehaviorFeedbackCandidateV1, error) {
	baseline := s.clock()
	if baseline.IsZero() {
		return contract.BehaviorFeedbackCandidateV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "behavior feedback clock is unavailable")
	}
	if err := contract.ValidateNow(baseline, value.CreatedUnixNano, value.ExpiresUnixNano); err != nil {
		return contract.BehaviorFeedbackCandidateV1{}, err
	}
	now := s.clock()
	if now.IsZero() || now.Before(baseline) {
		return contract.BehaviorFeedbackCandidateV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "behavior feedback clock regressed")
	}
	if err := contract.ValidateNow(now, value.CreatedUnixNano, value.ExpiresUnixNano); err != nil {
		return contract.BehaviorFeedbackCandidateV1{}, err
	}
	created, err := s.store.CreateBehaviorFeedbackCandidateV1(ctx, value)
	if err != nil && (core.HasCategory(err, core.ErrorUnavailable) || core.HasCategory(err, core.ErrorIndeterminate)) {
		original := err
		recovery, cancel, ok := s.boundedRecoveryContextV1(ctx, now, value.ExpiresUnixNano)
		if !ok {
			return contract.BehaviorFeedbackCandidateV1{}, original
		}
		defer cancel()
		created, err = s.store.InspectBehaviorFeedbackCandidateExactV1(recovery, value.TenantID, reviewport.ExactV1(value.ID, value.Revision, value.Digest))
		if err != nil || !reflect.DeepEqual(created, value) || !s.recoveryStillCurrentV1(now, value.ExpiresUnixNano) {
			return contract.BehaviorFeedbackCandidateV1{}, original
		}
	}
	if err != nil {
		return contract.BehaviorFeedbackCandidateV1{}, err
	}
	return created, nil
}

func (s *Service) InspectBehaviorFeedbackCandidateV1(ctx context.Context, tenant core.TenantID, ref reviewport.ExactFactRefV1) (contract.BehaviorFeedbackCandidateV1, error) {
	return s.store.InspectBehaviorFeedbackCandidateExactV1(ctx, tenant, ref)
}

func (s *Service) AttachEvidenceV1(ctx context.Context, value contract.EvidenceAttachmentV1) (contract.EvidenceAttachmentV1, error) {
	baseline := s.clock()
	if baseline.IsZero() {
		return contract.EvidenceAttachmentV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "review Evidence Attachment clock is unavailable")
	}
	if err := value.Validate(); err != nil {
		return contract.EvidenceAttachmentV1{}, err
	}
	if err := contract.ValidateNow(baseline, value.ObservedUnixNano, value.ExpiresUnixNano); err != nil {
		return contract.EvidenceAttachmentV1{}, err
	}
	fresh := s.clock()
	if fresh.IsZero() || fresh.Before(baseline) {
		return contract.EvidenceAttachmentV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "review Evidence Attachment clock regressed")
	}
	if err := contract.ValidateNow(fresh, value.ObservedUnixNano, value.ExpiresUnixNano); err != nil {
		return contract.EvidenceAttachmentV1{}, err
	}
	created, err := s.store.CreateEvidenceAttachmentV1(ctx, reviewport.CreateEvidenceAttachmentMutationV1{Attachment: value, CheckedUnixNano: fresh.UnixNano()})
	if err == nil || (!core.HasCategory(err, core.ErrorUnavailable) && !core.HasCategory(err, core.ErrorIndeterminate)) {
		return created, err
	}
	recovery, cancel, ok := s.boundedRecoveryContextV1(ctx, fresh, value.ExpiresUnixNano)
	if !ok {
		return contract.EvidenceAttachmentV1{}, err
	}
	defer cancel()
	inspected, inspectErr := s.store.InspectEvidenceAttachmentExactV1(recovery, value.TenantID, reviewport.ExactV1(value.ID, value.Revision, value.Digest))
	if inspectErr != nil || !reflect.DeepEqual(inspected, value) || !s.recoveryStillCurrentV1(fresh, value.ExpiresUnixNano) {
		return contract.EvidenceAttachmentV1{}, err
	}
	return inspected, nil
}

func (s *Service) boundedRecoveryContextV1(parent context.Context, baseline time.Time, expiries ...int64) (context.Context, context.CancelFunc, bool) {
	now := s.clock()
	if now.IsZero() || now.Before(baseline) {
		return nil, nil, false
	}
	remaining := lostReplyRecoveryTimeoutV1
	for _, expiry := range expiries {
		if expiry <= 0 {
			continue
		}
		if expiry <= now.UnixNano() {
			return nil, nil, false
		}
		if d := time.Duration(expiry - now.UnixNano()); d < remaining {
			remaining = d
		}
	}
	if remaining <= 0 {
		return nil, nil, false
	}
	recovery, cancel := context.WithTimeout(context.WithoutCancel(parent), remaining)
	return recovery, cancel, true
}

func (s *Service) recoveryStillCurrentV1(baseline time.Time, expiries ...int64) bool {
	now := s.clock()
	if now.IsZero() || now.Before(baseline) {
		return false
	}
	for _, expiry := range expiries {
		if expiry > 0 && now.UnixNano() >= expiry {
			return false
		}
	}
	return true
}

func expiryOfVerdictV1(value *contract.VerdictV1) int64 {
	if value == nil {
		return 0
	}
	return value.ExpiresUnixNano
}
