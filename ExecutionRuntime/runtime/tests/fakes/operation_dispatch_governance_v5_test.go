package fakes_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/fakes"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestOperationDispatchV5ThreeBasesLostReplyAndConformance(t *testing.T) {
	for _, basis := range []ports.OperationReviewAuthorizationBasisV5{
		ports.OperationReviewBasisAcceptedQuorumV5,
		ports.OperationReviewBasisConditionalQuorumSatisfiedV5,
		ports.OperationReviewBasisPolicyNotRequiredV5,
	} {
		t.Run(string(basis), func(t *testing.T) {
			fixture := newOperationDispatchFixtureV5(t, "basis-"+string(basis), basis)
			fixture.effect.store.LoseNextIssueV5Reply()
			issued, err := fixture.gateway.IssueOperationDispatchV5(context.Background(), fixture.issue)
			if err != nil {
				t.Fatal(err)
			}
			if issued.AuthorizationBasis != basis || issued.Record.Permit.AuthorizationBasis != basis || fixture.effect.store.IssueV5CommitCount() != 1 {
				t.Fatalf("V5 Issue lost exact basis or linearized more than once: %#v commits=%d", issued, fixture.effect.store.IssueV5CommitCount())
			}
			fixture.effect.store.LoseNextBeginV5Reply()
			begun, err := fixture.gateway.BeginOperationDispatchV5(context.Background(), beginOperationDispatchRequestV5(fixture, issued))
			if err != nil {
				t.Fatal(err)
			}
			if begun.Record.State != ports.OperationPermitBegunV5 || fixture.effect.store.BeginV5CommitCount() != 1 {
				t.Fatalf("V5 Begin did not recover exact begun record: %#v commits=%d", begun.Record, fixture.effect.store.BeginV5CommitCount())
			}

			conformanceFixture := newOperationDispatchFixtureV5(t, "conformance-"+string(basis), basis)
			report, err := conformance.CheckOperationDispatchGovernanceV5(context.Background(), conformance.OperationDispatchGovernanceCaseV5{Gateway: conformanceFixture.gateway, Issue: conformanceFixture.issue})
			if err != nil {
				t.Fatal(err)
			}
			if report.ProviderCalled || report.V5MasqueradesAsV4 || report.ProductionClaimEligible {
				t.Fatalf("V5 governance conformance overclaimed capability: %#v", report)
			}
		})
	}
}

func TestOperationDispatchV5SameCanonicalConcurrentIssueCommitsOnce(t *testing.T) {
	fixture := newOperationDispatchFixtureV5(t, "concurrent", ports.OperationReviewBasisAcceptedQuorumV5)
	const workers = 64
	var wg sync.WaitGroup
	errCh := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := fixture.gateway.IssueOperationDispatchV5(context.Background(), fixture.issue)
			errCh <- err
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatal(err)
		}
	}
	if fixture.effect.store.IssueV5CommitCount() != 1 {
		t.Fatalf("same canonical V5 Issue committed %d times", fixture.effect.store.IssueV5CommitCount())
	}
}

func TestOperationDispatchV4AndV5ShareSameEffectTerminalGuard(t *testing.T) {
	v5 := newOperationDispatchFixtureV5(t, "cross-version", ports.OperationReviewBasisAcceptedQuorumV5)
	v4Reviews, v4Store, _ := operationReviewAuthorizationForEffectV4(t, v5.effect)
	v5.effect.store.BindOperationReviewAuthorizationFactsV4(v4Store)
	v4Authorization, err := v4Reviews.CreateOperationReviewAuthorizationV4(context.Background(), ports.CreateOperationReviewAuthorizationRequestV4{
		AuthorizationID: "authorization-cross-version-v4", Operation: v5.effect.intent.Operation,
		EffectID: v5.effect.intent.ID, ExpectedEffectRevision: v5.effect.accepted.Revision, RequestedTTL: 20 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	v4Gateway := control.OperationGovernanceGatewayV4{Effects: v5.effect.store, Admissions: control.OperationEffectAdmissionGatewayV3{Effects: v5.effect.store}, Reviews: v4Reviews, Current: v5.effect.current, Clock: func() time.Time { return v5.effect.now }}
	v4Issue := ports.IssueGovernedOperationDispatchRequestV4{
		Operation: v5.effect.intent.Operation, EffectID: v5.effect.intent.ID, ExpectedEffectRevision: v5.effect.accepted.Revision,
		Admission: v5.issue.Admission, ReviewAuthorization: v4Authorization.RefV4(), PermitID: "permit-cross-version-v4",
		AttemptID: "attempt-cross-version-v4", PermitTTL: 10 * time.Second,
	}

	start := make(chan struct{})
	errCh := make(chan error, 2)
	go func() {
		<-start
		_, err := v5.gateway.IssueOperationDispatchV5(context.Background(), v5.issue)
		errCh <- err
	}()
	go func() {
		<-start
		_, err := v4Gateway.IssueOperationDispatchV4(context.Background(), v4Issue)
		errCh <- err
	}()
	close(start)
	var successes int
	for i := 0; i < 2; i++ {
		if err := <-errCh; err == nil {
			successes++
		} else if !core.HasCategory(err, core.ErrorConflict) && !core.HasCategory(err, core.ErrorPreconditionFailed) {
			t.Fatalf("cross-version loser returned unexpected error: %v", err)
		}
	}
	if successes != 1 || v5.effect.store.IssueV4CommitCount()+v5.effect.store.IssueV5CommitCount() != 1 {
		t.Fatalf("V4/V5 shared guard allowed successes=%d commits(v4=%d,v5=%d)", successes, v5.effect.store.IssueV4CommitCount(), v5.effect.store.IssueV5CommitCount())
	}
}

func TestOperationDispatchV5DriftAndChangedReplayFailClosed(t *testing.T) {
	fixture := newOperationDispatchFixtureV5(t, "drift", ports.OperationReviewBasisAcceptedQuorumV5)
	issued, err := fixture.gateway.IssueOperationDispatchV5(context.Background(), fixture.issue)
	if err != nil {
		t.Fatal(err)
	}
	changed := fixture.issue
	changed.AttemptID = "another-attempt"
	if _, err := fixture.gateway.IssueOperationDispatchV5(context.Background(), changed); !core.HasReason(err, core.ReasonIdempotencyPayloadMismatch) {
		t.Fatalf("same Permit ID with changed content did not conflict: %v", err)
	}
	fixture.review.mutateAndSeal(t, fixture.effect.now, func(value *ports.OperationReviewCurrentProjectionV5) {
		if value.Quorum != nil {
			value.Quorum.CurrentnessDigest = core.DigestBytes([]byte("drifted-v5-currentness"))
		}
	})
	if _, err := fixture.gateway.BeginOperationDispatchV5(context.Background(), beginOperationDispatchRequestV5(fixture, issued)); err == nil {
		t.Fatal("drifted V5 Review current reached Begin")
	}
	if fixture.effect.store.BeginV5CommitCount() != 0 {
		t.Fatal("failed V5 Begin changed Permit state")
	}
}

func TestOperationDispatchV5LostReplyRecoveryIgnoresCanceledMutationContext(t *testing.T) {
	fixture := newOperationDispatchFixtureV5(t, "canceled-recovery", ports.OperationReviewBasisAcceptedQuorumV5)
	ctx, cancel := context.WithCancel(context.Background())
	fixture.gateway.Effects = cancelingOperationEffectPortV5{OperationEffectDispatchFactPortV5: fixture.effect.store, cancel: cancel, loseIssue: true}
	issued, err := fixture.gateway.IssueOperationDispatchV5(ctx, fixture.issue)
	if err != nil || issued.Record.State != ports.OperationPermitIssuedV5 {
		t.Fatalf("V5 Issue did not recover through cancellation-independent exact Inspect: %#v err=%v", issued, err)
	}

	ctx, cancel = context.WithCancel(context.Background())
	fixture.gateway.Effects = cancelingOperationEffectPortV5{OperationEffectDispatchFactPortV5: fixture.effect.store, cancel: cancel, loseBegin: true}
	begun, err := fixture.gateway.BeginOperationDispatchV5(ctx, beginOperationDispatchRequestV5(fixture, issued))
	if err != nil || begun.Record.State != ports.OperationPermitBegunV5 {
		t.Fatalf("V5 Begin did not recover through cancellation-independent exact Inspect: %#v err=%v", begun, err)
	}
	if fixture.effect.store.IssueV5CommitCount() != 1 || fixture.effect.store.BeginV5CommitCount() != 1 {
		t.Fatalf("canceled lost-reply recovery repeated mutation: issue=%d begin=%d", fixture.effect.store.IssueV5CommitCount(), fixture.effect.store.BeginV5CommitCount())
	}
}

type cancelingOperationEffectPortV5 struct {
	control.OperationEffectDispatchFactPortV5
	cancel    context.CancelFunc
	loseIssue bool
	loseBegin bool
}

func (p cancelingOperationEffectPortV5) IssueOperationDispatchPermitV5(ctx context.Context, request control.IssueOperationPermitRequestV5) (control.IssueOperationPermitResultV5, error) {
	result, err := p.OperationEffectDispatchFactPortV5.IssueOperationDispatchPermitV5(ctx, request)
	if err == nil && p.loseIssue {
		p.cancel()
		return control.IssueOperationPermitResultV5{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected canceled V5 Issue reply loss")
	}
	return result, err
}

func (p cancelingOperationEffectPortV5) BeginOperationDispatchV5(ctx context.Context, request control.BeginOperationDispatchRequestV5) (control.OperationDispatchPermitFactV5, error) {
	result, err := p.OperationEffectDispatchFactPortV5.BeginOperationDispatchV5(ctx, request)
	if err == nil && p.loseBegin {
		p.cancel()
		return control.OperationDispatchPermitFactV5{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected canceled V5 Begin reply loss")
	}
	return result, err
}

type operationDispatchFixtureV5 struct {
	effect        *operationFixtureV3
	review        *operationReviewReaderV5
	authorization ports.OperationReviewAuthorizationFactV5
	gateway       control.OperationGovernanceGatewayV5
	issue         ports.IssueGovernedOperationDispatchRequestV5
}

func newOperationDispatchFixtureV5(t *testing.T, suffix string, basis ports.OperationReviewAuthorizationBasisV5) *operationDispatchFixtureV5 {
	t.Helper()
	effect, reviewGateway, _, review := newOperationReviewAuthorizationFixtureV5(t, basis)
	effect.store.BindOperationReviewAuthorizationV5(reviewGateway)
	authorization, err := reviewGateway.CreateOperationReviewAuthorizationV5(context.Background(), operationReviewAuthorizationRequestV5(effect, "authorization-"+suffix, basis))
	if err != nil {
		t.Fatal(err)
	}
	admissions := control.OperationEffectAdmissionGatewayV3{Effects: effect.store}
	admission, err := admissions.InspectAcceptedOperationEffectV3(context.Background(), effect.intent.Operation, effect.intent.ID)
	if err != nil {
		t.Fatal(err)
	}
	gateway := control.OperationGovernanceGatewayV5{Effects: effect.store, Admissions: admissions, Reviews: reviewGateway, Current: effect.current, Clock: func() time.Time { return effect.now }}
	issue := ports.IssueGovernedOperationDispatchRequestV5{
		Operation: effect.intent.Operation, EffectID: effect.intent.ID, ExpectedEffectRevision: effect.accepted.Revision,
		Admission: admission, ReviewAuthorization: authorization.RefV5(), AuthorizationBasis: basis,
		PermitID: "permit-" + suffix, AttemptID: "attempt-" + suffix, PermitTTL: 10 * time.Second,
	}
	return &operationDispatchFixtureV5{effect: effect, review: review, authorization: authorization, gateway: gateway, issue: issue}
}

func beginOperationDispatchRequestV5(fixture *operationDispatchFixtureV5, issued ports.CurrentOperationDispatchAuthorizationV5) ports.BeginGovernedOperationDispatchRequestV5 {
	return ports.BeginGovernedOperationDispatchRequestV5{
		Operation: fixture.issue.Operation, EffectID: fixture.issue.EffectID, ExpectedEffectRevision: issued.Record.EffectFactRevision,
		PermitID: fixture.issue.PermitID, ExpectedPermitFactRevision: issued.Record.Revision,
		AdmissionDigest: issued.Record.Permit.Admission.Digest, ReviewAuthorization: fixture.authorization.RefV5(), AuthorizationBasis: fixture.issue.AuthorizationBasis,
	}
}

var _ ports.OperationGovernancePortV5 = control.OperationGovernanceGatewayV5{}
var _ control.OperationEffectDispatchFactPortV5 = (*fakes.OperationEffectStoreV3)(nil)
