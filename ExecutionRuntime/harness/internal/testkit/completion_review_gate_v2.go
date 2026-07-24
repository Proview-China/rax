package testkit

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/bridgecontract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type CompletionReviewInputReaderV2 struct {
	Values map[applicationcontract.ReviewWaitingInputSubjectV1]applicationcontract.ReviewWaitingInputCurrentProjectionV1
	Err    error
	Calls  atomic.Int64
}

func (r *CompletionReviewInputReaderV2) InspectReviewWaitingInputCurrentV1(_ context.Context, subject applicationcontract.ReviewWaitingInputSubjectV1) (applicationcontract.ReviewWaitingInputCurrentProjectionV1, error) {
	r.Calls.Add(1)
	if r.Err != nil {
		return applicationcontract.ReviewWaitingInputCurrentProjectionV1{}, r.Err
	}
	value, ok := r.Values[subject]
	if !ok {
		return applicationcontract.ReviewWaitingInputCurrentProjectionV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "completion Review input is absent")
	}
	return value, nil
}

type CompletionReviewCoordinatorV2 struct {
	Values map[string]applicationcontract.ReviewWaitingOutcomeV1
	Err    error
	Calls  atomic.Int64
}

func (c *CompletionReviewCoordinatorV2) CoordinateReviewWaitingV1(_ context.Context, request applicationcontract.ReviewWaitingRequestV1) (applicationcontract.ReviewWaitingOutcomeV1, error) {
	c.Calls.Add(1)
	if c.Err != nil {
		return applicationcontract.ReviewWaitingOutcomeV1{}, c.Err
	}
	value, ok := c.Values[request.ID]
	if !ok {
		return applicationcontract.ReviewWaitingOutcomeV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "completion Review coordination is absent")
	}
	return value.Clone(), nil
}

type CompletionReviewGateFixtureV2 struct {
	Now     time.Time
	Request bridgecontract.CompletionReviewGateRequestV2
	Input   applicationcontract.ReviewWaitingInputCurrentProjectionV1
	Outcome applicationcontract.ReviewWaitingOutcomeV1
}

func CompletionReviewGateV2(t testing.TB, phase runtimeports.NamespacedNameV2, suffix string) CompletionReviewGateFixtureV2 {
	t.Helper()
	now := time.Unix(3_000_000_000, 0)
	tenant := core.TenantID("tenant-completion-review-" + suffix)
	scope := Scope(Digest("completion-review-plan-" + suffix))
	scope.Identity.TenantID = tenant
	scope.Identity.ID = core.AgentIdentityID("agent-completion-review-" + suffix)
	scope.Lineage.ID = core.InstanceLineageID("lineage-completion-review-" + suffix)
	scope.Instance.ID = core.AgentInstanceID("instance-completion-review-" + suffix)
	scope.SandboxLease.ID = core.SandboxLeaseID("sandbox-completion-review-" + suffix)
	scopeDigest, err := runtimeports.ExecutionScopeDigestV2(scope)
	if err != nil {
		t.Fatal(err)
	}
	target := applicationcontract.ReviewWaitingTargetCoordinateV1{
		TenantID: tenant, ID: "completion-target-" + suffix, Revision: 3,
		Digest: Digest("completion-target-" + suffix), RunID: core.AgentRunID("run-completion-review-" + suffix),
		CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(50 * time.Minute).UnixNano(),
	}
	waiting, err := applicationcontract.SealReviewWaitingRequestV1(applicationcontract.ReviewWaitingRequestV1{
		Delivery: applicationcontract.ReviewWaitingInlineV1, ExecutionScope: scope, ExecutionScopeDigest: scopeDigest,
		Phase:           applicationcontract.ReviewPhasePointCoordinateV1{Kind: phase, ID: "completion-phase-" + suffix, Revision: 2, Digest: Digest("completion-phase-" + suffix), CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Hour).UnixNano()},
		Target:          target,
		ReviewRequest:   applicationcontract.ReviewRequestCoordinateV1{TenantID: tenant, ID: "completion-review-request-" + suffix, Revision: 1, Digest: Digest("completion-review-request-" + suffix), CaseID: "completion-case-" + suffix},
		CreatedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(40 * time.Minute).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	input, err := applicationcontract.SealReviewWaitingInputCurrentProjectionV1(applicationcontract.ReviewWaitingInputCurrentProjectionV1{
		Subject: waiting.InputSubjectV1(), Phase: waiting.Phase, Target: target, ExecutionScopeDigest: scopeDigest,
		CheckedUnixNano: now.Add(time.Second).UnixNano(), ExpiresUnixNano: now.Add(30 * time.Minute).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	caseV1 := applicationcontract.ReviewWaitingCaseCoordinateV1{TenantID: tenant, ID: waiting.ReviewRequest.CaseID, Revision: 1, Digest: Digest("completion-case-r1-" + suffix), Target: target, ExpiresUnixNano: now.Add(25 * time.Minute).UnixNano()}
	verdict := applicationcontract.ReviewWaitingVerdictCoordinateV1{TenantID: tenant, ID: "completion-verdict-" + suffix, Revision: 1, Digest: Digest("completion-verdict-" + suffix), CaseID: caseV1.ID, CaseRevision: caseV1.Revision, CaseDigest: caseV1.Digest, Target: target, ExpiresUnixNano: now.Add(18 * time.Minute).UnixNano()}
	current, err := applicationcontract.SealReviewWaitingCurrentProjectionV1(applicationcontract.ReviewWaitingCurrentProjectionV1{
		RequestID: waiting.ReviewRequest.ID, RequestDigest: waiting.ReviewRequest.Digest,
		Case:    applicationcontract.ReviewWaitingCaseCoordinateV1{TenantID: tenant, ID: caseV1.ID, Revision: 2, Digest: Digest("completion-case-r2-" + suffix), Target: target, ExpiresUnixNano: now.Add(18 * time.Minute).UnixNano()},
		Verdict: &verdict, Decision: applicationcontract.ReviewPhaseAllowV1, Current: true,
		CheckedUnixNano: now.Add(3 * time.Second).UnixNano(), ExpiresUnixNano: now.Add(15 * time.Minute).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	receiptCoordination := applicationcontract.ReviewWaitingCoordinationRefV1{ID: waiting.ID, Revision: 3, Digest: Digest("completion-coordination-predecessor-" + suffix)}
	receipt, err := applicationcontract.SealReviewPhaseReceiptV1(applicationcontract.ReviewPhaseReceiptV1{Coordination: receiptCoordination}, waiting, current, input, now.Add(4*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	outcome := applicationcontract.ReviewWaitingOutcomeV1{Coordination: applicationcontract.ReviewWaitingCoordinationRefV1{ID: waiting.ID, Revision: 4, Digest: Digest("completion-coordination-current-" + suffix)}, Review: current, Receipt: &receipt}
	if err := outcome.ValidateFor(waiting, now.Add(4*time.Second)); err != nil {
		t.Fatal(err)
	}
	return CompletionReviewGateFixtureV2{
		Now:     now,
		Request: bridgecontract.CompletionReviewGateRequestV2{ContractVersion: bridgecontract.CompletionReviewGateContractVersionV2, Waiting: waiting},
		Input:   input, Outcome: outcome,
	}
}
