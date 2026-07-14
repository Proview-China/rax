package application_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	application "github.com/Proview-China/rax/ExecutionRuntime/application"
	applicationconformance "github.com/Proview-China/rax/ExecutionRuntime/application/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/application/fakes"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestGovernedOperationCoordinatorV3FullObservedSettlementSupportsFutureModules(t *testing.T) {
	for _, kind := range []runtimeports.NamespacedNameV2{"user.module-eight/execute", "user.module-eleven/execute"} {
		t.Run(string(kind), func(t *testing.T) {
			fx := newCoordinatorFixtureV3(t, kind)
			result, err := fx.coordinator.StartGovernedOperationV3(context.Background(), application.StartGovernedOperationRequestV3{Plan: fx.plan, Attempt: fx.initial})
			if err != nil {
				t.Fatal(err)
			}
			if result.Attempt.State != contract.OperationProviderObservedV3 || result.Attempt.DomainReservation == nil || result.Domain == nil || result.Domain.State != applicationports.OperationDomainObservedV3 || fx.runtime.executeCalls != 1 {
				t.Fatalf("full dispatch did not stop at independently observable watermark: %#v", result)
			}
			fx.domain.mu.Lock()
			reservation := fx.domain.reservation
			reserveCalls := fx.domain.reserveCalls
			fx.domain.mu.Unlock()
			if reservation == nil || reserveCalls != 1 || reservation.StepKind != kind || reservation.DomainAdapter != fx.initial.DomainAdapter || reservation.Descriptor != fx.initial.Descriptor {
				t.Fatalf("custom module did not traverse the generic exact reservation barrier: reservation=%#v calls=%d", reservation, reserveCalls)
			}
			assertStepStateV3(t, result.Journal, contract.StepWaitingInspectV2)

			submission := settlementSubmissionForAttemptV3(t, result.Attempt, fx.now)
			settled, err := fx.coordinator.SettleGovernedOperationV3(context.Background(), application.SettleGovernedOperationRequestV3{Plan: fx.plan, AttemptID: fx.initial.ID, Submission: submission})
			if err != nil {
				t.Fatal(err)
			}
			if settled.Attempt.State != contract.OperationSettledV3 || settled.Domain == nil || settled.Domain.State != applicationports.OperationDomainSettledV3 || settled.Attempt.SettlementDomainResult == nil {
				t.Fatalf("settlement did not apply exact opaque domain result: %#v", settled)
			}
			assertStepStateV3(t, settled.Journal, contract.StepCompletedV2)
			if fx.runtime.executeCalls != 1 {
				t.Fatalf("settlement redispatched provider %d times", fx.runtime.executeCalls)
			}
		})
	}
}

func TestGovernedOperationCoordinatorV3LostExecuteReplyInspectsAndNeverRedispatches(t *testing.T) {
	fx := newCoordinatorFixtureV3(t, "custom.loss/execute")
	fx.runtime.loseExecuteReply = true
	result, err := fx.coordinator.StartGovernedOperationV3(context.Background(), application.StartGovernedOperationRequestV3{Plan: fx.plan, Attempt: fx.initial})
	if err != nil {
		t.Fatal(err)
	}
	if result.Attempt.State != contract.OperationProviderObservedV3 || fx.runtime.executeCalls != 1 {
		t.Fatalf("lost reply was not recovered through Inspect: %#v calls=%d", result.Attempt, fx.runtime.executeCalls)
	}
	if _, err := fx.coordinator.ResumeGovernedOperationV3(context.Background(), application.ResumeGovernedOperationRequestV3{Plan: fx.plan, AttemptID: fx.initial.ID}); err != nil {
		t.Fatal(err)
	}
	if fx.runtime.executeCalls != 1 {
		t.Fatalf("resume blindly redispatched Execute: %d", fx.runtime.executeCalls)
	}
}

func TestGovernedOperationCoordinatorV3ExecuteUnknownMarksDomainOnlyAfterPrepared(t *testing.T) {
	fx := newCoordinatorFixtureV3(t, "custom.unknown/execute")
	fx.runtime.executeUnknown = true
	result, err := fx.coordinator.StartGovernedOperationV3(context.Background(), application.StartGovernedOperationRequestV3{Plan: fx.plan, Attempt: fx.initial})
	if err != nil {
		t.Fatal(err)
	}
	if result.Attempt.State != contract.OperationDispatchUnknownV3 || result.Domain == nil || result.Domain.State != applicationports.OperationDomainUnknownV3 || fx.domain.preparedCalls != 1 || fx.domain.unknownCalls != 1 {
		t.Fatalf("post-prepared unknown did not preserve both watermarks: %#v domain=%#v", result.Attempt, result.Domain)
	}
	if result.Attempt.UnknownAuthorization.PermitFactRevision != result.Attempt.Enforcement.RecordedRevision || result.Attempt.UnknownAuthorization.PermitFactRevision != result.Attempt.BegunAuthorization.PermitFactRevision+1 {
		t.Fatalf("post-prepared unknown lost the exact persisted Enforcement revision: begun=%d enforcement=%d unknown=%d", result.Attempt.BegunAuthorization.PermitFactRevision, result.Attempt.Enforcement.RecordedRevision, result.Attempt.UnknownAuthorization.PermitFactRevision)
	}
	if _, err := fx.coordinator.ResumeGovernedOperationV3(context.Background(), application.ResumeGovernedOperationRequestV3{Plan: fx.plan, AttemptID: fx.initial.ID}); err != nil {
		t.Fatal(err)
	}
	if fx.runtime.executeCalls != 1 {
		t.Fatalf("unknown Execute was redispatched: %d", fx.runtime.executeCalls)
	}
}

func TestGovernedOperationCoordinatorV3LostBeginReplyRecoversUnknownWithoutPrepare(t *testing.T) {
	fx := newCoordinatorFixtureV3(t, "custom.begin/unknown")
	fx.runtime.beginUnknown = true
	result, err := fx.coordinator.StartGovernedOperationV3(context.Background(), application.StartGovernedOperationRequestV3{Plan: fx.plan, Attempt: fx.initial})
	if err != nil {
		t.Fatal(err)
	}
	if result.Attempt.State != contract.OperationDispatchUnknownV3 || result.Attempt.BegunAuthorization != nil || result.Domain != nil || fx.runtime.prepareCalls != 0 || fx.runtime.executeCalls != 0 {
		t.Fatalf("Begin unknown crossed the prepare/domain boundary: %#v prepare=%d execute=%d", result.Attempt, fx.runtime.prepareCalls, fx.runtime.executeCalls)
	}
}

func TestGovernedOperationCoordinatorV3PrepareUnknownNeverForgesDomainPreparedState(t *testing.T) {
	fx := newCoordinatorFixtureV3(t, "custom.prepare/unknown")
	fx.runtime.prepareUnknown = true
	result, err := fx.coordinator.StartGovernedOperationV3(context.Background(), application.StartGovernedOperationRequestV3{Plan: fx.plan, Attempt: fx.initial})
	if err != nil {
		t.Fatal(err)
	}
	if result.Attempt.State != contract.OperationDispatchUnknownV3 || result.Attempt.Prepared != nil || result.Domain != nil || fx.domain.preparedCalls != 0 || fx.domain.unknownCalls != 0 {
		t.Fatalf("pre-prepared unknown leaked a domain dispatch watermark: %#v domain=%#v", result.Attempt, result.Domain)
	}
	if fx.runtime.prepareCalls != 1 || fx.runtime.executeCalls != 0 {
		t.Fatalf("unexpected provider calls prepare=%d execute=%d", fx.runtime.prepareCalls, fx.runtime.executeCalls)
	}

	submission := unknownSettlementSubmissionV3(t, result.Attempt, fx.now)
	bad := submission
	bad.Disposition = runtimeports.OperationSettlementNotAppliedV3
	if _, err := fx.coordinator.SettleGovernedOperationV3(context.Background(), application.SettleGovernedOperationRequestV3{Plan: fx.plan, AttemptID: fx.initial.ID, Submission: bad}); err == nil {
		t.Fatal("pre-prepared unknown accepted a non-failed settlement")
	}
	stillUnknown, err := fx.attempts.InspectGovernedOperationAttemptV3(context.Background(), fx.initial.Scope, fx.initial.ID)
	if err != nil || stillUnknown.State != contract.OperationDispatchUnknownV3 {
		t.Fatalf("invalid settlement poisoned the authoritative attempt: %#v %v", stillUnknown, err)
	}
	settled, err := fx.coordinator.SettleGovernedOperationV3(context.Background(), application.SettleGovernedOperationRequestV3{Plan: fx.plan, AttemptID: fx.initial.ID, Submission: submission})
	if err != nil {
		t.Fatal(err)
	}
	if settled.Domain == nil || settled.Domain.State != applicationports.OperationDomainSettledV3 || fx.domain.lastSettlement.RuntimeAttempt != nil {
		t.Fatalf("pre-prepared settlement must use the explicit nil RuntimeAttempt branch: %#v", fx.domain.lastSettlement)
	}
}

func TestGovernedOperationCoordinatorV3LostDomainUnknownReplyRecoversByInspect(t *testing.T) {
	fx := newCoordinatorFixtureV3(t, "custom.domain/unknown-loss")
	fx.runtime.executeUnknown = true
	fx.domain.loseUnknownReply = true
	result, err := fx.coordinator.StartGovernedOperationV3(context.Background(), application.StartGovernedOperationRequestV3{Plan: fx.plan, Attempt: fx.initial})
	if err != nil {
		t.Fatal(err)
	}
	if result.Domain == nil || result.Domain.State != applicationports.OperationDomainUnknownV3 || fx.runtime.executeCalls != 1 {
		t.Fatalf("lost Domain unknown reply did not recover exactly: %#v", result)
	}
}

func TestGovernedOperationCoordinatorV3SettlementReplayRejectsDomainResultDrift(t *testing.T) {
	fx := newCoordinatorFixtureV3(t, "custom.settlement/replay")
	observed, err := fx.coordinator.StartGovernedOperationV3(context.Background(), application.StartGovernedOperationRequestV3{Plan: fx.plan, Attempt: fx.initial})
	if err != nil {
		t.Fatal(err)
	}
	submission := settlementSubmissionForAttemptV3(t, observed.Attempt, fx.now)
	if _, err := fx.coordinator.SettleGovernedOperationV3(context.Background(), application.SettleGovernedOperationRequestV3{Plan: fx.plan, AttemptID: fx.initial.ID, Submission: submission}); err != nil {
		t.Fatal(err)
	}
	drift := submission
	drift.DomainResult = clonePayloadForTestV3(submission.DomainResult)
	drift.DomainResult.Inline = []byte(`{"result":"forged"}`)
	drift.DomainResult.Length = uint64(len(drift.DomainResult.Inline))
	drift.DomainResult.ContentDigest = core.DigestBytes(drift.DomainResult.Inline)
	if _, err := fx.coordinator.SettleGovernedOperationV3(context.Background(), application.SettleGovernedOperationRequestV3{Plan: fx.plan, AttemptID: fx.initial.ID, Submission: drift}); err == nil {
		t.Fatal("settled replay changed opaque DomainResult")
	}
}

func TestGovernedOperationCoordinatorV3RejectsPlanAndJournalWriteAheadDrift(t *testing.T) {
	fx := newCoordinatorFixtureV3(t, "custom.plan/guard")
	drifted := fx.plan
	drifted.Steps = append([]contract.WorkflowStepV2(nil), fx.plan.Steps...)
	drifted.Steps[0].Descriptor.Digest = core.DigestBytes([]byte("changed-descriptor"))
	if _, err := fx.coordinator.StartGovernedOperationV3(context.Background(), application.StartGovernedOperationRequestV3{Plan: drifted, Attempt: fx.initial}); !core.HasReason(err, core.ReasonPlanInvalid) {
		t.Fatalf("same plan ID/revision descriptor drift was accepted: %v", err)
	}

	fx2 := newCoordinatorFixtureV3(t, "custom.journal/guard")
	if _, err := fx2.attempts.CreateGovernedOperationAttemptV3(context.Background(), fx2.initial); err != nil {
		t.Fatal(err)
	}
	if _, err := fx2.coordinator.ResumeGovernedOperationV3(context.Background(), application.ResumeGovernedOperationRequestV3{Plan: fx2.plan, AttemptID: fx2.initial.ID}); err == nil {
		t.Fatal("Resume bypassed the required dispatch_intent journal write-ahead")
	}
}

func TestOperationDomainPortV3RejectsMissingPostPreparedSettlementSidecar(t *testing.T) {
	fx := newCoordinatorFixtureV3(t, "custom.domain/guard")
	result, err := fx.coordinator.StartGovernedOperationV3(context.Background(), application.StartGovernedOperationRequestV3{Plan: fx.plan, Attempt: fx.initial})
	if err != nil {
		t.Fatal(err)
	}
	submission := settlementSubmissionForAttemptV3(t, result.Attempt, fx.now)
	settlement := fx.runtime.settlementRef(submission)
	settled := result.Attempt
	settled.Revision++
	settled.State = contract.OperationSettledV3
	settled.Settlement = &settlement
	settled.SettlementDomainResult = clonePayloadForTestV3(submission.DomainResult)
	settled.UpdatedUnixNano++
	ref, err := settled.RefV3()
	if err != nil {
		t.Fatal(err)
	}
	request := applicationports.ApplyOperationSettlementRequestV3{StepKind: settled.StepKind, Attempt: ref, Intent: settled.IntentValue, Settlement: settlement, DomainResult: submission.DomainResult}
	if err := request.Validate(); err == nil {
		t.Fatal("post-prepared settlement omitted exact Runtime attempt sidecar")
	}
}

func TestGovernedOperationCoordinatorV3PlanTTLOnlyBlocksBeforeBegin(t *testing.T) {
	pre := newCoordinatorFixtureV3(t, "custom.ttl/pre-begin")
	expired := time.Unix(0, pre.plan.ExpiresUnixNano).Add(time.Second)
	*pre.clockNow = expired
	if _, err := pre.coordinator.StartGovernedOperationV3(context.Background(), application.StartGovernedOperationRequestV3{Plan: pre.plan, Attempt: pre.initial}); err == nil {
		t.Fatal("expired plan admitted a new Effect")
	}

	post := newCoordinatorFixtureV3(t, "custom.ttl/post-begin")
	post.runtime.beginUnknown = true
	if _, err := post.coordinator.StartGovernedOperationV3(context.Background(), application.StartGovernedOperationRequestV3{Plan: post.plan, Attempt: post.initial}); err != nil {
		t.Fatal(err)
	}
	*post.clockNow = time.Unix(0, post.plan.ExpiresUnixNano).Add(time.Hour)
	result, err := post.coordinator.ResumeGovernedOperationV3(context.Background(), application.ResumeGovernedOperationRequestV3{Plan: post.plan, AttemptID: post.initial.ID})
	if err != nil || result.Attempt.State != contract.OperationDispatchUnknownV3 {
		t.Fatalf("post-Begin recovery was blocked by plan TTL: %#v %v", result.Attempt, err)
	}
}

func TestGovernedOperationCoordinatorV3RejectsForgedDomainResponseAndRuntimeAttempt(t *testing.T) {
	forgedResponse := newCoordinatorFixtureV3(t, "custom.domain/forged-response")
	forgedResponse.domain.forgeNextBasis = true
	if _, err := forgedResponse.coordinator.StartGovernedOperationV3(context.Background(), application.StartGovernedOperationRequestV3{Plan: forgedResponse.plan, Attempt: forgedResponse.initial}); !core.HasReason(err, core.ReasonEvidenceConflict) {
		t.Fatalf("forged Domain Basis was accepted: %v", err)
	}
	if forgedResponse.runtime.executeCalls != 0 {
		t.Fatal("provider executed after forged Domain prepared response")
	}

	fx := newCoordinatorFixtureV3(t, "custom.domain/forged-request")
	if _, err := fx.coordinator.StartGovernedOperationV3(context.Background(), application.StartGovernedOperationRequestV3{Plan: fx.plan, Attempt: fx.initial}); err != nil {
		t.Fatal(err)
	}
	request := fx.domain.lastPrepared
	request.RuntimeAttempt.PermitID = "another-permit"
	if err := request.Validate(); err == nil {
		t.Fatal("Domain request accepted a forged Runtime attempt")
	}
	request = fx.domain.lastPrepared
	request.StepKind = "another.module/execute"
	if err := request.Validate(); err == nil {
		t.Fatal("Domain request accepted a forged StepKind")
	}
}

func TestGovernedOperationCoordinatorV3ConcurrentStartLinearizesOneProviderEffect(t *testing.T) {
	fx := newCoordinatorFixtureV3(t, "custom.concurrent/execute")
	var group sync.WaitGroup
	errors := make(chan error, 64)
	for range 64 {
		group.Add(1)
		go func() {
			defer group.Done()
			_, err := fx.coordinator.StartGovernedOperationV3(context.Background(), application.StartGovernedOperationRequestV3{Plan: fx.plan, Attempt: fx.initial})
			if err != nil {
				errors <- err
			}
		}()
	}
	group.Wait()
	close(errors)
	for err := range errors {
		t.Fatal(err)
	}
	if fx.runtime.prepareCalls != 1 || fx.runtime.executeCalls != 1 {
		t.Fatalf("concurrent Start linearized prepare=%d execute=%d", fx.runtime.prepareCalls, fx.runtime.executeCalls)
	}
	fact, err := fx.attempts.InspectGovernedOperationAttemptV3(context.Background(), fx.initial.Scope, fx.initial.ID)
	if err != nil || fact.State != contract.OperationProviderObservedV3 {
		t.Fatalf("concurrent attempt did not converge: %#v %v", fact, err)
	}
}

func TestOperationDomainStatePortV3ConformanceForCustomComponent(t *testing.T) {
	now := time.Unix(1_800_200_000, 0)
	chain := normalOperationAttemptChainV3(t, now, "user.module-eleven/conformance")
	prepared := chain[6]
	observed := chain[7]
	settledObserved := chain[8]
	unknown := unknownAfterV3(t, prepared)
	settledUnknown := settledPostPreparedUnknownV3(t, unknown)
	preparedRef, _ := prepared.RefV3()
	observedRef, _ := observed.RefV3()
	unknownRef, _ := unknown.RefV3()
	settledObservedRef, _ := settledObserved.RefV3()
	settledUnknownRef, _ := settledUnknown.RefV3()
	preparedRuntime := runtimeRefsForTestV3(prepared)
	observedRuntime := runtimeRefsForTestV3(observed)
	unknownRuntime := runtimeRefsForTestV3(unknown)
	settledObservedRuntime := runtimeRefsForTestV3(settledObserved)
	settledUnknownRuntime := runtimeRefsForTestV3(settledUnknown)
	testCase := applicationconformance.OperationDomainStateCaseV3{
		NewPort:         func() applicationports.OperationDomainStatePortV3 { return &coordinatorDomainV3{} },
		Prepared:        applicationports.BindPreparedOperationRequestV3{StepKind: prepared.StepKind, Attempt: preparedRef, Intent: prepared.IntentValue, RuntimeAttempt: preparedRuntime, DelegationFact: *prepared.DelegationFact, Prepared: runtimeports.PreparedExecutionGovernanceResultV2{Delegation: *prepared.PreparedDelegation, Prepared: *prepared.Prepared, Enforcement: *prepared.Enforcement}},
		Observed:        applicationports.BindObservedOperationRequestV3{StepKind: observed.StepKind, Attempt: observedRef, Intent: observed.IntentValue, RuntimeAttempt: observedRuntime, Observation: *observed.Observation},
		Unknown:         applicationports.MarkUnknownOperationRequestV3{StepKind: unknown.StepKind, Attempt: unknownRef, Intent: unknown.IntentValue, RuntimeAttempt: unknownRuntime, Authorization: *unknown.UnknownAuthorization},
		SettledObserved: applicationports.ApplyOperationSettlementRequestV3{StepKind: settledObserved.StepKind, Attempt: settledObservedRef, Intent: settledObserved.IntentValue, RuntimeAttempt: &settledObservedRuntime, Settlement: *settledObserved.Settlement, DomainResult: settledObserved.SettlementDomainResult},
		SettledUnknown:  applicationports.ApplyOperationSettlementRequestV3{StepKind: settledUnknown.StepKind, Attempt: settledUnknownRef, Intent: settledUnknown.IntentValue, RuntimeAttempt: &settledUnknownRuntime, Settlement: *settledUnknown.Settlement},
	}
	report, err := applicationconformance.CheckOperationDomainStatePortV3(context.Background(), testCase)
	if err != nil {
		t.Fatal(err)
	}
	if !report.CertificationCandidate || report.BindingEligible || report.ProductionEligible || report.DispatchEligible || report.CommitEligible {
		t.Fatalf("conformance report upgraded authority: %#v", report)
	}
}

func TestGovernedOperationCoordinatorV3LostReplyMatrixRecoversOnlyThroughFacts(t *testing.T) {
	cases := []struct {
		name      string
		configure func(*coordinatorFixtureV3)
	}{
		{"attempt-create", func(f *coordinatorFixtureV3) { f.attempts.LoseNextCreateReply = true }},
		{"attempt-cas", func(f *coordinatorFixtureV3) { f.attempts.LoseNextCASReply = true }},
		{"journal-dispatch", func(f *coordinatorFixtureV3) { f.journals.LoseNextJournalCASReply = true }},
		{"domain-reservation", func(f *coordinatorFixtureV3) { f.domain.loseReservationReply = true }},
		{"admission", func(f *coordinatorFixtureV3) { f.runtime.loseAdmissionReply = true }},
		{"issue", func(f *coordinatorFixtureV3) { f.runtime.loseIssueReply = true }},
		{"begin", func(f *coordinatorFixtureV3) { f.runtime.loseBeginReply = true }},
		{"declare", func(f *coordinatorFixtureV3) { f.runtime.loseDeclareReply = true }},
		{"prepare", func(f *coordinatorFixtureV3) { f.runtime.losePrepareReply = true }},
		{"commit-prepared", func(f *coordinatorFixtureV3) { f.runtime.loseCommitReply = true }},
		{"observation", func(f *coordinatorFixtureV3) { f.runtime.loseObservationReply = true }},
		{"domain-prepared", func(f *coordinatorFixtureV3) { f.domain.losePreparedReply = true }},
		{"domain-observed", func(f *coordinatorFixtureV3) { f.domain.loseObservedReply = true }},
		{"journal-waiting", func(f *coordinatorFixtureV3) {
			f.runtime.beforeObservationReturn = func() { f.journals.LoseNextJournalCASReply = true }
		}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			fx := newCoordinatorFixtureV3(t, "custom.loss/"+runtimeports.NamespacedNameV2(testCase.name))
			testCase.configure(&fx)
			result, err := fx.coordinator.StartGovernedOperationV3(context.Background(), application.StartGovernedOperationRequestV3{Plan: fx.plan, Attempt: fx.initial})
			if err != nil {
				t.Fatal(err)
			}
			if result.Attempt.State != contract.OperationProviderObservedV3 || fx.runtime.prepareCalls != 1 || fx.runtime.executeCalls != 1 {
				t.Fatalf("lost %s reply did not converge exactly once: %#v prepare=%d execute=%d", testCase.name, result.Attempt, fx.runtime.prepareCalls, fx.runtime.executeCalls)
			}
		})
	}
}

func TestGovernedOperationCoordinatorV3SettlementLostReplyMatrix(t *testing.T) {
	for _, name := range []string{"settlement", "attempt-cas", "domain", "journal-completed"} {
		t.Run(name, func(t *testing.T) {
			fx := newCoordinatorFixtureV3(t, "custom.settlement/"+runtimeports.NamespacedNameV2(name))
			observed, err := fx.coordinator.StartGovernedOperationV3(context.Background(), application.StartGovernedOperationRequestV3{Plan: fx.plan, Attempt: fx.initial})
			if err != nil {
				t.Fatal(err)
			}
			switch name {
			case "settlement":
				fx.runtime.loseSettlementReply = true
			case "attempt-cas":
				fx.attempts.LoseNextCASReply = true
			case "domain":
				fx.domain.loseSettlementReply = true
			case "journal-completed":
				fx.journals.LoseNextJournalCASReply = true
			}
			result, err := fx.coordinator.SettleGovernedOperationV3(context.Background(), application.SettleGovernedOperationRequestV3{Plan: fx.plan, AttemptID: fx.initial.ID, Submission: settlementSubmissionForAttemptV3(t, observed.Attempt, fx.now)})
			if err != nil {
				t.Fatal(err)
			}
			if result.Attempt.State != contract.OperationSettledV3 || result.Domain == nil || result.Domain.State != applicationports.OperationDomainSettledV3 || fx.runtime.executeCalls != 1 {
				t.Fatalf("lost settlement stage %s did not converge: %#v", name, result)
			}
		})
	}
}

func TestOperationDomainRouterV3CustomModulesExactResolutionAndFailClosed(t *testing.T) {
	now := time.Unix(1_800_300_000, 0)
	clockNow := now
	currentness := newOperationDomainCurrentnessV3()
	router, err := application.NewOperationDomainRouterV3(func() time.Time { return clockNow }, currentness)
	if err != nil {
		t.Fatal(err)
	}
	registrations := make([]application.OperationDomainAdapterRegistrationV3, 0, 2)
	for _, kind := range []runtimeports.NamespacedNameV2{"user.module-eight/router", "user.module-eleven/router"} {
		base := operationAttemptFixtureV3(t, now, kind).base
		base.DomainAdapter.ComponentID = runtimeports.ComponentIDV2(kind)
		base.DomainAdapter.ManifestDigest = core.DigestBytes([]byte("manifest:" + string(kind)))
		base.DomainAdapter.ArtifactDigest = core.DigestBytes([]byte("artifact:" + string(kind)))
		base.DomainAdapter.Capability = runtimeports.CapabilityNameV2(kind)
		currentness.set(base.DomainAdapter, operationDomainAuthorizationV3(base.DomainAdapter, now))
		registrations = append(registrations, application.OperationDomainAdapterRegistrationV3{StepKind: kind, Descriptor: base.Descriptor, Adapter: base.DomainAdapter, Port: &coordinatorDomainV3{}})
	}
	var group sync.WaitGroup
	errors := make(chan error, len(registrations))
	for _, registration := range registrations {
		registration := registration
		group.Add(1)
		go func() {
			defer group.Done()
			errors <- router.RegisterOperationDomainV3(context.Background(), registration)
		}()
	}
	group.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, registration := range registrations {
		resolved, err := router.ResolveOperationDomainV3(context.Background(), applicationports.OperationDomainResolveRequestV3{StepKind: registration.StepKind, Descriptor: registration.Descriptor, DomainAdapter: registration.Adapter})
		if err != nil || resolved == nil {
			t.Fatalf("custom module resolution failed: %v", err)
		}
	}
	if _, err := router.ResolveOperationDomainV3(context.Background(), applicationports.OperationDomainResolveRequestV3{StepKind: registrations[0].StepKind, Descriptor: registrations[1].Descriptor, DomainAdapter: registrations[0].Adapter}); !core.HasReason(err, core.ReasonComponentMismatch) {
		t.Fatalf("cross-owner descriptor resolved: %v", err)
	}
	if _, err := router.ResolveOperationDomainV3(context.Background(), applicationports.OperationDomainResolveRequestV3{StepKind: registrations[0].StepKind, Descriptor: registrations[0].Descriptor, DomainAdapter: registrations[1].Adapter}); !core.HasReason(err, core.ReasonComponentMismatch) {
		t.Fatalf("cross-owner adapter resolved: %v", err)
	}
	if err := router.RegisterOperationDomainV3(context.Background(), registrations[0]); !core.HasReason(err, core.ReasonOwnerConflict) {
		t.Fatalf("duplicate domain owner accepted: %v", err)
	}
	wrong := registrations[0].Descriptor
	wrong.Digest = core.DigestBytes([]byte("wrong-descriptor"))
	if _, err := router.ResolveOperationDomainV3(context.Background(), applicationports.OperationDomainResolveRequestV3{StepKind: registrations[0].StepKind, Descriptor: wrong, DomainAdapter: registrations[0].Adapter}); !core.HasReason(err, core.ReasonComponentMismatch) {
		t.Fatalf("descriptor drift accepted: %v", err)
	}
	for _, mutate := range []func(runtimeports.ProviderBindingRefV2) runtimeports.ProviderBindingRefV2{
		func(v runtimeports.ProviderBindingRefV2) runtimeports.ProviderBindingRefV2 {
			v.BindingSetID += "-wrong"
			return v
		},
		func(v runtimeports.ProviderBindingRefV2) runtimeports.ProviderBindingRefV2 {
			v.BindingSetRevision++
			return v
		},
		func(v runtimeports.ProviderBindingRefV2) runtimeports.ProviderBindingRefV2 {
			v.ComponentID = "user.wrong/component"
			return v
		},
		func(v runtimeports.ProviderBindingRefV2) runtimeports.ProviderBindingRefV2 {
			v.ManifestDigest = core.DigestBytes([]byte("wrong-manifest"))
			return v
		},
		func(v runtimeports.ProviderBindingRefV2) runtimeports.ProviderBindingRefV2 {
			v.ArtifactDigest = core.DigestBytes([]byte("wrong-artifact"))
			return v
		},
		func(v runtimeports.ProviderBindingRefV2) runtimeports.ProviderBindingRefV2 {
			v.Capability = "custom.eighth/wrong"
			return v
		},
	} {
		wrongAdapter := mutate(registrations[0].Adapter)
		if _, err := router.ResolveOperationDomainV3(context.Background(), applicationports.OperationDomainResolveRequestV3{StepKind: registrations[0].StepKind, Descriptor: registrations[0].Descriptor, DomainAdapter: wrongAdapter}); !core.HasReason(err, core.ReasonComponentMismatch) {
			t.Fatalf("DomainAdapter drift accepted: %v", err)
		}
	}
	if _, err := router.ResolveOperationDomainV3(context.Background(), applicationports.OperationDomainResolveRequestV3{StepKind: "user.unknown/router", Descriptor: contract.StepDescriptorRefV2{Kind: "user.unknown/router", Revision: 1, Digest: core.DigestBytes([]byte("unknown")), ExpiresUnixNano: now.Add(time.Hour).UnixNano()}, DomainAdapter: registrations[0].Adapter}); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("unknown required domain did not fail closed: %v", err)
	}
	clockNow = time.Unix(0, registrations[0].Descriptor.ExpiresUnixNano).Add(time.Nanosecond)
	if _, err := router.ResolveOperationDomainV3(context.Background(), applicationports.OperationDomainResolveRequestV3{StepKind: registrations[0].StepKind, Descriptor: registrations[0].Descriptor, DomainAdapter: registrations[0].Adapter}); !core.HasReason(err, core.ReasonCapabilityExpired) {
		t.Fatalf("expired descriptor resolved: %v", err)
	}
}

func TestOperationDomainRouterV3RequiresAndRereadsHostCurrentness(t *testing.T) {
	now := time.Unix(1_800_300_000, 0)
	clockNow := now
	base := operationAttemptFixtureV3(t, now, "user.currentness/router").base
	registration := application.OperationDomainAdapterRegistrationV3{StepKind: base.StepKind, Descriptor: base.Descriptor, Adapter: base.DomainAdapter, Port: &coordinatorDomainV3{}}

	withoutReader, err := application.NewOperationDomainRouterV3(func() time.Time { return clockNow })
	if err != nil {
		t.Fatal(err)
	}
	if err := withoutReader.RegisterOperationDomainV3(context.Background(), registration); !core.HasReason(err, core.ReasonComponentMissing) {
		t.Fatalf("nil currentness reader did not fail closed: %v", err)
	}

	reader := newOperationDomainCurrentnessV3()
	reader.set(base.DomainAdapter, operationDomainAuthorizationV3(base.DomainAdapter, now))
	router, err := application.NewOperationDomainRouterV3(func() time.Time { return clockNow }, reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := router.RegisterOperationDomainV3(context.Background(), registration); err != nil {
		t.Fatal(err)
	}
	request := applicationports.OperationDomainResolveRequestV3{StepKind: base.StepKind, Descriptor: base.Descriptor, DomainAdapter: base.DomainAdapter}
	if _, err := router.ResolveOperationDomainV3(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if reader.calls() != 2 {
		t.Fatalf("Register and Resolve did not independently re-read currentness: calls=%d", reader.calls())
	}

	revoked := operationDomainAuthorizationV3(base.DomainAdapter, now)
	revoked.State = applicationports.OperationDomainAdapterRevokedV3
	revoked.InvalidationReason = core.ReasonBindingDrift
	revoked = sealOperationDomainAuthorizationForTestV3(revoked)
	reader.set(base.DomainAdapter, revoked)
	if _, err := router.ResolveOperationDomainV3(context.Background(), request); !core.HasReason(err, core.ReasonBindingDrift) {
		t.Fatalf("revoked adapter resolved: %v", err)
	}

	expired := operationDomainAuthorizationV3(base.DomainAdapter, now)
	expired.State = applicationports.OperationDomainAdapterExpiredV3
	expired.InvalidationReason = core.ReasonBindingExpired
	expired = sealOperationDomainAuthorizationForTestV3(expired)
	reader.set(base.DomainAdapter, expired)
	if _, err := router.ResolveOperationDomainV3(context.Background(), request); !core.HasReason(err, core.ReasonBindingExpired) {
		t.Fatalf("expired adapter resolved: %v", err)
	}

	drifted := operationDomainAuthorizationV3(base.DomainAdapter, now)
	drifted.Adapter.ArtifactDigest = core.DigestBytes([]byte("drifted-artifact"))
	drifted = sealOperationDomainAuthorizationForTestV3(drifted)
	reader.set(base.DomainAdapter, drifted)
	if _, err := router.ResolveOperationDomainV3(context.Background(), request); !core.HasReason(err, core.ReasonBindingDrift) {
		t.Fatalf("drifted current projection resolved: %v", err)
	}
}

func TestOperationDomainRouterV3RegistrationRejectsInactiveAndDriftedCurrentness(t *testing.T) {
	now := time.Unix(1_800_300_000, 0)
	base := operationAttemptFixtureV3(t, now, "user.registration-currentness/router").base
	registration := application.OperationDomainAdapterRegistrationV3{StepKind: base.StepKind, Descriptor: base.Descriptor, Adapter: base.DomainAdapter, Port: &coordinatorDomainV3{}}
	for _, testCase := range []struct {
		name   string
		mutate func(*applicationports.OperationDomainAdapterAuthorizationV3)
		reason core.ReasonCode
	}{
		{name: "revoked", reason: core.ReasonBindingDrift, mutate: func(value *applicationports.OperationDomainAdapterAuthorizationV3) {
			value.State, value.InvalidationReason = applicationports.OperationDomainAdapterRevokedV3, core.ReasonBindingDrift
		}},
		{name: "expired-state", reason: core.ReasonBindingExpired, mutate: func(value *applicationports.OperationDomainAdapterAuthorizationV3) {
			value.State, value.InvalidationReason = applicationports.OperationDomainAdapterExpiredV3, core.ReasonBindingExpired
		}},
		{name: "expired-ttl", reason: core.ReasonBindingExpired, mutate: func(value *applicationports.OperationDomainAdapterAuthorizationV3) {
			value.IssuedUnixNano, value.ExpiresUnixNano = now.Add(-20*time.Second).UnixNano(), now.UnixNano()
		}},
		{name: "binding-revision-drift", reason: core.ReasonBindingDrift, mutate: func(value *applicationports.OperationDomainAdapterAuthorizationV3) {
			value.Adapter.BindingSetRevision++
		}},
		{name: "manifest-drift", reason: core.ReasonBindingDrift, mutate: func(value *applicationports.OperationDomainAdapterAuthorizationV3) {
			value.Adapter.ManifestDigest = core.DigestBytes([]byte("wrong-manifest"))
		}},
		{name: "artifact-drift", reason: core.ReasonBindingDrift, mutate: func(value *applicationports.OperationDomainAdapterAuthorizationV3) {
			value.Adapter.ArtifactDigest = core.DigestBytes([]byte("wrong-artifact"))
		}},
		{name: "capability-drift", reason: core.ReasonBindingDrift, mutate: func(value *applicationports.OperationDomainAdapterAuthorizationV3) {
			value.Adapter.Capability = "user.registration-currentness/wrong"
		}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			reader := newOperationDomainCurrentnessV3()
			current := operationDomainAuthorizationV3(base.DomainAdapter, now)
			testCase.mutate(&current)
			current = sealOperationDomainAuthorizationForTestV3(current)
			reader.set(base.DomainAdapter, current)
			router, err := application.NewOperationDomainRouterV3(func() time.Time { return now }, reader)
			if err != nil {
				t.Fatal(err)
			}
			if err := router.RegisterOperationDomainV3(context.Background(), registration); !core.HasReason(err, testCase.reason) {
				t.Fatalf("inactive or drifted authorization registered: %v", err)
			}
		})
	}
}

func TestOperationDomainAdapterAuthorizationV3DigestRejectsFieldSwaps(t *testing.T) {
	now := time.Unix(1_800_300_000, 0)
	base := operationAttemptFixtureV3(t, now, "user.authorization-digest/router").base
	authorization := operationDomainAuthorizationV3(base.DomainAdapter, now)
	for _, testCase := range []struct {
		name   string
		mutate func(*applicationports.OperationDomainAdapterAuthorizationV3)
	}{
		{name: "state", mutate: func(value *applicationports.OperationDomainAdapterAuthorizationV3) {
			value.State, value.InvalidationReason = applicationports.OperationDomainAdapterRevokedV3, core.ReasonBindingDrift
		}},
		{name: "revision", mutate: func(value *applicationports.OperationDomainAdapterAuthorizationV3) { value.Revision++ }},
		{name: "issued", mutate: func(value *applicationports.OperationDomainAdapterAuthorizationV3) { value.IssuedUnixNano++ }},
		{name: "expires", mutate: func(value *applicationports.OperationDomainAdapterAuthorizationV3) { value.ExpiresUnixNano-- }},
		{name: "adapter", mutate: func(value *applicationports.OperationDomainAdapterAuthorizationV3) {
			value.Adapter.ArtifactDigest = core.DigestBytes([]byte("swapped-artifact"))
		}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			forged := authorization
			testCase.mutate(&forged)
			if err := forged.Validate(); !core.HasReason(err, core.ReasonBindingDrift) {
				t.Fatalf("same Digest accepted changed authorization content: %v", err)
			}
		})
	}
	resealed := authorization
	resealed.Revision++
	resealed = sealOperationDomainAuthorizationForTestV3(resealed)
	if err := resealed.Validate(); err != nil {
		t.Fatalf("explicitly resealed authorization is invalid: %v", err)
	}
}

func TestGovernedOperationCoordinatorV3PreflightsAdapterBeforeEveryRuntimeSideEffect(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		mutate func(*coordinatorFixtureV3)
		reason core.ReasonCode
	}{
		{name: "missing", reason: core.ReasonComponentMissing, mutate: func(fx *coordinatorFixtureV3) {
			fx.currentness.remove(fx.initial.DomainAdapter)
		}},
		{name: "revoked", reason: core.ReasonBindingDrift, mutate: func(fx *coordinatorFixtureV3) {
			value := operationDomainAuthorizationV3(fx.initial.DomainAdapter, *fx.clockNow)
			value.State, value.InvalidationReason = applicationports.OperationDomainAdapterRevokedV3, core.ReasonBindingDrift
			fx.currentness.set(fx.initial.DomainAdapter, sealOperationDomainAuthorizationForTestV3(value))
		}},
		{name: "expired", reason: core.ReasonBindingExpired, mutate: func(fx *coordinatorFixtureV3) {
			value := operationDomainAuthorizationV3(fx.initial.DomainAdapter, *fx.clockNow)
			value.State, value.InvalidationReason = applicationports.OperationDomainAdapterExpiredV3, core.ReasonBindingExpired
			fx.currentness.set(fx.initial.DomainAdapter, sealOperationDomainAuthorizationForTestV3(value))
		}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			fx := newCoordinatorFixtureV3(t, "user.preflight-adapter/"+runtimeports.NamespacedNameV2(testCase.name))
			testCase.mutate(&fx)
			if _, err := fx.coordinator.StartGovernedOperationV3(context.Background(), application.StartGovernedOperationRequestV3{Plan: fx.plan, Attempt: fx.initial}); !core.HasReason(err, testCase.reason) {
				t.Fatalf("Start crossed invalid adapter preflight: %v", err)
			}
			assertNoRuntimeOperationSideEffectsV3(t, fx)
			if _, err := fx.coordinator.ResumeGovernedOperationV3(context.Background(), application.ResumeGovernedOperationRequestV3{Plan: fx.plan, AttemptID: fx.initial.ID}); !core.HasReason(err, testCase.reason) {
				t.Fatalf("Resume crossed invalid adapter preflight: %v", err)
			}
			assertNoRuntimeOperationSideEffectsV3(t, fx)
		})
	}
}

func TestGovernedOperationCoordinatorV3PreflightsOpaqueDomainSemanticsBeforeRuntime(t *testing.T) {
	for _, name := range []string{"oversized", "malformed", "candidate-ref-drift", "endpoint-drift", "session-drift", "provider-drift"} {
		t.Run(name, func(t *testing.T) {
			fx := newCoordinatorFixtureV3(t, "user.preflight-domain/"+runtimeports.NamespacedNameV2(name))
			fx.domain.reserveErr = core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceConflict, "injected "+name+" domain reservation rejection")
			if _, err := fx.coordinator.StartGovernedOperationV3(context.Background(), application.StartGovernedOperationRequestV3{Plan: fx.plan, Attempt: fx.initial}); err == nil {
				t.Fatal("Start crossed rejected opaque domain preflight")
			}
			assertNoRuntimeOperationSideEffectsV3(t, fx)
			if _, err := fx.coordinator.ResumeGovernedOperationV3(context.Background(), application.ResumeGovernedOperationRequestV3{Plan: fx.plan, AttemptID: fx.initial.ID}); err == nil {
				t.Fatal("Resume crossed rejected opaque domain preflight")
			}
			assertNoRuntimeOperationSideEffectsV3(t, fx)
		})
	}
}

func TestGovernedOperationCoordinatorV3ExpiredReservationRemainsInspectableButCannotMutateRuntimeForFutureModules(t *testing.T) {
	for _, kind := range []runtimeports.NamespacedNameV2{"user.module-eight/reservation-expiry", "user.module-eleven/reservation-expiry"} {
		t.Run(string(kind), func(t *testing.T) {
			fx := newCoordinatorFixtureV3(t, kind)
			fx.domain.reservationExpiresUnixNano = fx.now.Add(time.Second).UnixNano()
			fx.domain.afterReserve = func(reservation contract.OperationDomainReservationRefV3) {
				*fx.clockNow = time.Unix(0, reservation.ExpiresUnixNano)
			}
			if _, err := fx.coordinator.StartGovernedOperationV3(context.Background(), application.StartGovernedOperationRequestV3{Plan: fx.plan, Attempt: fx.initial}); !core.HasReason(err, core.ReasonCapabilityExpired) {
				t.Fatalf("expired reservation crossed Runtime boundary: %v", err)
			}
			persisted, err := fx.attempts.InspectGovernedOperationAttemptV3(context.Background(), fx.initial.Scope, fx.initial.ID)
			if err != nil || persisted.State != contract.OperationDomainReservedV3 || persisted.DomainReservation == nil {
				t.Fatalf("reservation watermark was not durably recoverable: %#v err=%v", persisted, err)
			}
			inspected, err := fx.domain.InspectOperationIntentReservationV3(context.Background(), applicationports.InspectOperationIntentReservationRequestV3{Scope: fx.initial.Scope, StepKind: fx.initial.StepKind, DomainAdapter: fx.initial.DomainAdapter, AttemptID: fx.initial.ID})
			if err != nil || inspected.Digest != persisted.DomainReservation.Digest {
				t.Fatalf("historical expired reservation could not be inspected exactly: %#v err=%v", inspected, err)
			}
			initialRef, _ := fx.initial.RefV3()
			if err := applicationports.ValidateOperationDomainReservationForV3(inspected, applicationports.ReserveOperationIntentRequestV3{StepKind: fx.initial.StepKind, Descriptor: fx.initial.Descriptor, DomainAdapter: fx.initial.DomainAdapter, Attempt: initialRef, Intent: fx.initial.IntentValue}); err != nil {
				t.Fatalf("historical reservation lost its exact request binding: %v", err)
			}
			if _, err := fx.coordinator.ResumeGovernedOperationV3(context.Background(), application.ResumeGovernedOperationRequestV3{Plan: fx.plan, AttemptID: fx.initial.ID}); !core.HasReason(err, core.ReasonCapabilityExpired) {
				t.Fatalf("expired reservation resumed Runtime mutation: %v", err)
			}
			fx.runtime.mu.Lock()
			admission, governance, delegation, provider := fx.runtime.admissionCalls, fx.runtime.governanceCalls, fx.runtime.delegationCalls, fx.runtime.providerCalls
			fx.runtime.mu.Unlock()
			if admission != 0 || governance != 0 || delegation != 0 || provider != 0 {
				t.Fatalf("expired reservation leaked Runtime mutation: admission=%d governance=%d delegation=%d provider=%d", admission, governance, delegation, provider)
			}
		})
	}
}

func TestGovernedOperationCoordinatorV3RejectsForgedSuccessfulCASReply(t *testing.T) {
	fx := newCoordinatorFixtureV3(t, "user.forged-cas/operation")
	forgedStore := &forgedSuccessfulOperationAttemptStoreV3{GovernedOperationAttemptFactPortV3: fx.attempts}
	coordinator, err := application.NewGovernedOperationCoordinatorV3(application.GovernedOperationCoordinatorConfigV3{
		Attempts: forgedStore, Journals: fx.journals, Admission: fx.runtime, Governance: fx.runtime,
		Delegations: fx.runtime, Execution: fx.runtime, Observations: fx.runtime, Settlements: fx.runtime,
		DomainResolver: fx.coordinatorDomainResolverV3(), Clock: func() time.Time { return *fx.clockNow },
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := coordinator.StartGovernedOperationV3(context.Background(), application.StartGovernedOperationRequestV3{Plan: fx.plan, Attempt: fx.initial}); !core.HasReason(err, core.ReasonRevisionConflict) {
		t.Fatalf("same-revision later-state successful CAS reply was trusted: %v", err)
	}
	fx.runtime.mu.Lock()
	defer fx.runtime.mu.Unlock()
	if fx.runtime.admissionCalls != 0 || fx.runtime.governanceCalls != 0 || fx.runtime.delegationCalls != 0 || fx.runtime.providerCalls != 0 {
		t.Fatal("forged CAS reply reached Runtime")
	}
}

func TestGovernedOperationCoordinatorV3RejectsMalformedSuccessfulCreateReply(t *testing.T) {
	fx := newCoordinatorFixtureV3(t, "user.forged-create/operation")
	store := &forgedSuccessfulOperationAttemptCreateStoreV3{GovernedOperationAttemptFactPortV3: fx.attempts}
	coordinator, err := application.NewGovernedOperationCoordinatorV3(application.GovernedOperationCoordinatorConfigV3{
		Attempts: store, Journals: fx.journals, Admission: fx.runtime, Governance: fx.runtime,
		Delegations: fx.runtime, Execution: fx.runtime, Observations: fx.runtime, Settlements: fx.runtime,
		DomainResolver: fx.coordinatorDomainResolverV3(), Clock: func() time.Time { return *fx.clockNow },
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := coordinator.StartGovernedOperationV3(context.Background(), application.StartGovernedOperationRequestV3{Plan: fx.plan, Attempt: fx.initial}); err == nil {
		t.Fatal("malformed successful create reply was trusted")
	}
	fx.runtime.mu.Lock()
	defer fx.runtime.mu.Unlock()
	if fx.runtime.admissionCalls != 0 || fx.runtime.governanceCalls != 0 || fx.runtime.delegationCalls != 0 || fx.runtime.providerCalls != 0 {
		t.Fatal("malformed create reply reached Runtime")
	}
}

func assertNoRuntimeOperationSideEffectsV3(t *testing.T, fx coordinatorFixtureV3) {
	t.Helper()
	fx.runtime.mu.Lock()
	admission, governance, delegation, provider := fx.runtime.admissionCalls, fx.runtime.governanceCalls, fx.runtime.delegationCalls, fx.runtime.providerCalls
	fx.runtime.mu.Unlock()
	if admission != 0 || governance != 0 || delegation != 0 || provider != 0 {
		t.Fatalf("domain preflight failure leaked Runtime side effects: admission=%d governance=%d delegation=%d provider=%d", admission, governance, delegation, provider)
	}
	attempt, err := fx.attempts.InspectGovernedOperationAttemptV3(context.Background(), fx.initial.Scope, fx.initial.ID)
	if err != nil || attempt.State != contract.OperationIntentRecordedV3 || attempt.Revision != 1 {
		t.Fatalf("domain preflight failure advanced Application attempt: %#v err=%v", attempt, err)
	}
	journal, err := fx.journals.InspectWorkflowJournalV2(context.Background(), fx.plan.Target, fx.initial.JournalID)
	if err != nil {
		t.Fatal(err)
	}
	assertStepStateV3(t, journal, contract.StepDispatchIntentV2)
	fx.domain.mu.Lock()
	defer fx.domain.mu.Unlock()
	if fx.domain.reservation != nil || fx.domain.current != nil || fx.domain.preparedCalls != 0 || fx.domain.unknownCalls != 0 || fx.domain.observedCalls != 0 || fx.domain.settlementCalls != 0 {
		t.Fatalf("read-only domain preflight persisted or advanced domain state: %#v", fx.domain.current)
	}
}

func TestGovernedJournalCompletionV3RejectsRawAndForgedCompletion(t *testing.T) {
	fx := newCoordinatorFixtureV3(t, "custom.journal/governed")
	observed, err := fx.coordinator.StartGovernedOperationV3(context.Background(), application.StartGovernedOperationRequestV3{Plan: fx.plan, Attempt: fx.initial})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := application.NewJournalCoordinatorV2(application.JournalCoordinatorConfigV2{Facts: fx.journals, Clock: func() time.Time { return *fx.clockNow }})
	forged := &contract.ApplicationFactRefV2{Ref: "forged-settlement", Revision: 1, Digest: core.DigestBytes([]byte("forged-settlement"))}
	if _, err := raw.AdvanceStepV2(context.Background(), application.AdvanceStepRequestV2{Plan: fx.plan, JournalID: fx.initial.JournalID, StepID: fx.initial.StepID, Target: contract.StepCompletedV2, Settlement: forged}); !core.HasCategory(err, core.ErrorForbidden) {
		t.Fatalf("legacy raw Journal completed governed step: %v", err)
	}
	completion, _ := application.NewGovernedJournalCompletionV3(application.GovernedJournalCompletionConfigV3{Attempts: fx.attempts, Journals: fx.journals, Clock: func() time.Time { return *fx.clockNow }})
	if _, err := completion.Complete(context.Background(), fx.plan, fx.initial.ID); err == nil {
		t.Fatal("unsettled governed Attempt completed Journal")
	}
	settled, err := fx.coordinator.SettleGovernedOperationV3(context.Background(), application.SettleGovernedOperationRequestV3{Plan: fx.plan, AttemptID: fx.initial.ID, Submission: settlementSubmissionForAttemptV3(t, observed.Attempt, fx.now)})
	if err != nil {
		t.Fatal(err)
	}
	assertStepStateV3(t, settled.Journal, contract.StepCompletedV2)
	drifted := fx.plan
	drifted.Steps = append([]contract.WorkflowStepV2(nil), fx.plan.Steps...)
	drifted.Steps[0].DomainAdapter = cloneProviderBindingForTestV3(fx.plan.Steps[0].DomainAdapter)
	drifted.Steps[0].DomainAdapter.ArtifactDigest = core.DigestBytes([]byte("forged-domain-adapter"))
	if _, err := completion.Complete(context.Background(), drifted, fx.initial.ID); err == nil {
		t.Fatal("forged Plan/DomainAdapter completed governed Journal")
	}
}

type coordinatorFixtureV3 struct {
	now         time.Time
	plan        contract.WorkflowPlanV2
	initial     contract.GovernedOperationAttemptFactV3
	attempts    *fakes.GovernedOperationAttemptStoreV3
	journals    *fakes.FactStoreV2
	runtime     *coordinatorRuntimeV3
	domain      *coordinatorDomainV3
	coordinator *application.GovernedOperationCoordinatorV3
	clockNow    *time.Time
	currentness *operationDomainCurrentnessV3
	router      *application.OperationDomainRouterV3
}

func newCoordinatorFixtureV3(t *testing.T, kind runtimeports.NamespacedNameV2) coordinatorFixtureV3 {
	t.Helper()
	now := time.Unix(1_800_100_000, 0)
	bundle, _ := applicationFixtureV2(t, now, kind, true)
	journal, err := contract.NewWorkflowJournalV2("journal-operation", bundle.Plan, now.UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	base := operationAttemptFixtureV3(t, now, kind).base
	journalStore := fakes.NewFactStoreV2()
	journalStore.Clock = func() time.Time { return now.Add(10 * time.Millisecond) }
	if _, err := journalStore.CreateWorkflowJournalV2(context.Background(), bundle.Plan, journal); err != nil {
		t.Fatal(err)
	}
	attempts := fakes.NewGovernedOperationAttemptStoreV3()
	runtime := newCoordinatorRuntimeV3(t, base, now.Add(10*time.Millisecond))
	domain := &coordinatorDomainV3{}
	clockNow := now.Add(10 * time.Millisecond)
	currentness := newOperationDomainCurrentnessV3()
	currentness.set(base.DomainAdapter, operationDomainAuthorizationV3(base.DomainAdapter, clockNow))
	router, err := application.NewOperationDomainRouterV3(func() time.Time { return clockNow }, currentness)
	if err != nil {
		t.Fatal(err)
	}
	if err := router.RegisterOperationDomainV3(context.Background(), application.OperationDomainAdapterRegistrationV3{StepKind: base.StepKind, Descriptor: base.Descriptor, Adapter: base.DomainAdapter, Port: domain}); err != nil {
		t.Fatal(err)
	}
	coordinator, err := application.NewGovernedOperationCoordinatorV3(application.GovernedOperationCoordinatorConfigV3{Attempts: attempts, Journals: journalStore, Admission: runtime, Governance: runtime, Delegations: runtime, Execution: runtime, Observations: runtime, Settlements: runtime, DomainResolver: router, Clock: func() time.Time { return clockNow }})
	if err != nil {
		t.Fatal(err)
	}
	return coordinatorFixtureV3{now: now, plan: bundle.Plan, initial: base, attempts: attempts, journals: journalStore, runtime: runtime, domain: domain, coordinator: coordinator, clockNow: &clockNow, currentness: currentness, router: router}
}

func (f coordinatorFixtureV3) coordinatorDomainResolverV3() applicationports.OperationDomainResolverV3 {
	return f.router
}

type forgedSuccessfulOperationAttemptStoreV3 struct {
	applicationports.GovernedOperationAttemptFactPortV3
	forged atomic.Bool
}

type forgedSuccessfulOperationAttemptCreateStoreV3 struct {
	applicationports.GovernedOperationAttemptFactPortV3
}

func (s *forgedSuccessfulOperationAttemptCreateStoreV3) CreateGovernedOperationAttemptV3(_ context.Context, requested contract.GovernedOperationAttemptFactV3) (contract.GovernedOperationAttemptFactV3, error) {
	forged := requested
	forged.ContractVersion = ""
	return forged, nil
}

func (s *forgedSuccessfulOperationAttemptStoreV3) CompareAndSwapGovernedOperationAttemptV3(ctx context.Context, request applicationports.GovernedOperationAttemptCASRequestV3) (contract.GovernedOperationAttemptFactV3, error) {
	if request.Next.State == contract.OperationDomainReservedV3 && !s.forged.Swap(true) {
		forged := request.Next
		forged.State = contract.OperationEffectAdmittedV3
		forged.Admission = &runtimeports.OperationEffectAdmissionReceiptV3{OperationDigest: forged.Intent.OperationDigest, EffectID: forged.Intent.EffectID, IntentRevision: forged.Intent.IntentRevision, IntentDigest: forged.Intent.IntentDigest, FactRevision: 1, State: "accepted"}
		if err := forged.Validate(); err != nil {
			return contract.GovernedOperationAttemptFactV3{}, err
		}
		return forged, nil
	}
	return s.GovernedOperationAttemptFactPortV3.CompareAndSwapGovernedOperationAttemptV3(ctx, request)
}

type operationDomainCurrentnessV3 struct {
	mu     sync.Mutex
	values map[runtimeports.ProviderBindingRefV2]applicationports.OperationDomainAdapterAuthorizationV3
	reads  int
}

func newOperationDomainCurrentnessV3() *operationDomainCurrentnessV3 {
	return &operationDomainCurrentnessV3{values: make(map[runtimeports.ProviderBindingRefV2]applicationports.OperationDomainAdapterAuthorizationV3)}
}

func (r *operationDomainCurrentnessV3) set(adapter runtimeports.ProviderBindingRefV2, value applicationports.OperationDomainAdapterAuthorizationV3) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.values[adapter] = value
}

func (r *operationDomainCurrentnessV3) remove(adapter runtimeports.ProviderBindingRefV2) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.values, adapter)
}

func (r *operationDomainCurrentnessV3) calls() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.reads
}

func (r *operationDomainCurrentnessV3) InspectOperationDomainAdapterCurrentV3(_ context.Context, adapter runtimeports.ProviderBindingRefV2) (applicationports.OperationDomainAdapterAuthorizationV3, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reads++
	value, ok := r.values[adapter]
	if !ok {
		return applicationports.OperationDomainAdapterAuthorizationV3{}, core.NewError(core.ErrorNotFound, core.ReasonComponentMissing, "test currentness projection not found")
	}
	return value, nil
}

func operationDomainAuthorizationV3(adapter runtimeports.ProviderBindingRefV2, now time.Time) applicationports.OperationDomainAdapterAuthorizationV3 {
	return sealOperationDomainAuthorizationForTestV3(applicationports.OperationDomainAdapterAuthorizationV3{
		ContractVersion: applicationports.OperationDomainContractVersionV3,
		Adapter:         adapter,
		Revision:        1,
		State:           applicationports.OperationDomainAdapterAuthorizedV3,
		IssuedUnixNano:  now.Add(-time.Second).UnixNano(),
		ExpiresUnixNano: now.Add(20 * time.Second).UnixNano(),
	})
}

func sealOperationDomainAuthorizationForTestV3(value applicationports.OperationDomainAdapterAuthorizationV3) applicationports.OperationDomainAdapterAuthorizationV3 {
	sealed, err := applicationports.SealOperationDomainAdapterAuthorizationV3(value)
	if err != nil {
		panic(err)
	}
	return sealed
}

type coordinatorRuntimeV3 struct {
	mu                      sync.Mutex
	t                       *testing.T
	now                     time.Time
	base                    contract.GovernedOperationAttemptFactV3
	admission               *runtimeports.OperationEffectAdmissionReceiptV3
	auth                    *runtimeports.OperationDispatchAuthorizationV3
	declared                *runtimeports.ExecutionDelegationRefV2
	attestation             *runtimeports.ProviderPreparationAttestationV2
	prepared                *runtimeports.PreparedExecutionGovernanceResultV2
	local                   *runtimeports.ProviderAttemptObservationV2
	observation             *runtimeports.ProviderAttemptObservationRefV2
	settlement              *runtimeports.OperationSettlementRefV3
	prepareUnknown          bool
	loseExecuteReply        bool
	executeUnknown          bool
	beginUnknown            bool
	loseAdmissionReply      bool
	loseIssueReply          bool
	loseBeginReply          bool
	loseDeclareReply        bool
	losePrepareReply        bool
	loseCommitReply         bool
	loseObservationReply    bool
	loseSettlementReply     bool
	beforeObservationReturn func()
	prepareCalls            int
	executeCalls            int
	admissionCalls          int
	governanceCalls         int
	delegationCalls         int
	providerCalls           int
}

func newCoordinatorRuntimeV3(t *testing.T, base contract.GovernedOperationAttemptFactV3, now time.Time) *coordinatorRuntimeV3 {
	return &coordinatorRuntimeV3{t: t, now: now, base: base}
}

func (r *coordinatorRuntimeV3) AdmitOperationEffectV3(_ context.Context, intent runtimeports.OperationEffectIntentV3) (runtimeports.OperationEffectAdmissionReceiptV3, error) {
	digest, _ := intent.DigestV3()
	op, _ := intent.Operation.DigestV3()
	value := runtimeports.OperationEffectAdmissionReceiptV3{OperationDigest: op, EffectID: intent.ID, IntentRevision: intent.Revision, IntentDigest: digest, FactRevision: 1, State: "accepted"}
	r.mu.Lock()
	r.admissionCalls++
	r.admission = &value
	r.mu.Unlock()
	if r.loseAdmissionReply {
		r.loseAdmissionReply = false
		return runtimeports.OperationEffectAdmissionReceiptV3{}, unavailableV3("admission reply lost")
	}
	return value, nil
}

func (r *coordinatorRuntimeV3) InspectAcceptedOperationEffectV3(context.Context, runtimeports.OperationSubjectV3, core.EffectIntentID) (runtimeports.OperationEffectAdmissionReceiptV3, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.admission == nil {
		return runtimeports.OperationEffectAdmissionReceiptV3{}, notFoundV3("admission")
	}
	return *r.admission, nil
}

func (r *coordinatorRuntimeV3) IssueOperationDispatchV3(context.Context, runtimeports.IssueGovernedOperationDispatchRequestV3) (runtimeports.OperationDispatchAuthorizationV3, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.governanceCalls++
	if r.auth == nil {
		a := validAuthorizationV3(r.t, r.base, r.now)
		r.auth = &a
	}
	if r.loseIssueReply {
		r.loseIssueReply = false
		return runtimeports.OperationDispatchAuthorizationV3{}, unavailableV3("Issue reply lost")
	}
	return *r.auth, nil
}

func (r *coordinatorRuntimeV3) BeginOperationDispatchV3(context.Context, runtimeports.BeginGovernedOperationDispatchRequestV3) (runtimeports.OperationDispatchAuthorizationV3, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.governanceCalls++
	a := *r.auth
	a.State = runtimeports.OperationDispatchAuthorizationBegunV3
	a.PermitFactRevision++
	if r.beginUnknown {
		a.State = runtimeports.OperationDispatchAuthorizationUnknownV3
		a.EffectFactRevision += 2
	}
	r.auth = &a
	if r.beginUnknown {
		return runtimeports.OperationDispatchAuthorizationV3{}, unavailableV3("Begin outcome unknown")
	}
	if r.loseBeginReply {
		r.loseBeginReply = false
		return runtimeports.OperationDispatchAuthorizationV3{}, unavailableV3("Begin reply lost")
	}
	return a, nil
}

func (r *coordinatorRuntimeV3) MarkOperationDispatchUnknownV3(context.Context, runtimeports.MarkOperationDispatchUnknownRequestV3) (runtimeports.OperationDispatchAuthorizationV3, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.governanceCalls++
	a := *r.auth
	a.State = runtimeports.OperationDispatchAuthorizationUnknownV3
	a.EffectFactRevision++
	r.auth = &a
	return a, nil
}

func (r *coordinatorRuntimeV3) InspectOperationDispatchAuthorizationV3(context.Context, runtimeports.OperationSubjectV3, core.EffectIntentID, string) (runtimeports.OperationDispatchAuthorizationV3, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.auth == nil {
		return runtimeports.OperationDispatchAuthorizationV3{}, notFoundV3("authorization")
	}
	return *r.auth, nil
}

func (r *coordinatorRuntimeV3) DeclareExecutionDelegationV2(_ context.Context, request runtimeports.DeclareExecutionDelegationRequestV2) (runtimeports.ExecutionDelegationRefV2, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.delegationCalls++
	ref, err := request.Delegation.RefV2()
	if err == nil {
		r.declared = &ref
	}
	if err == nil && r.loseDeclareReply {
		r.loseDeclareReply = false
		return runtimeports.ExecutionDelegationRefV2{}, unavailableV3("Declare reply lost")
	}
	return ref, err
}

func (r *coordinatorRuntimeV3) InspectDeclaredExecutionV2(context.Context, runtimeports.OperationSubjectV3, string) (runtimeports.ExecutionDelegationRefV2, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.declared == nil {
		return runtimeports.ExecutionDelegationRefV2{}, notFoundV3("declared delegation")
	}
	return *r.declared, nil
}

func (r *coordinatorRuntimeV3) CommitPreparedExecutionV2(_ context.Context, request runtimeports.CommitPreparedExecutionRequestV2) (runtimeports.PreparedExecutionGovernanceResultV2, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.delegationCalls++
	if r.prepared == nil {
		delegation := runtimeports.ExecutionDelegationRefV2{ID: request.Declared.ID, Revision: request.Declared.Revision + 1, Digest: core.DigestBytes([]byte("prepared-delegation"))}
		enforcement := runtimeports.PersistedOperationEnforcementRefV3{PermitID: request.Preparation.Prepared.PermitID, PermitRevision: request.Preparation.Prepared.PermitRevision, PermitDigest: request.Preparation.Prepared.PermitDigest, AttemptID: request.Preparation.Prepared.AttemptID, OperationDigest: request.Preparation.Prepared.OperationDigest, Provider: request.Preparation.Prepared.Provider, ReceiptDigest: core.DigestBytes([]byte("persisted-enforcement")), RecordedRevision: 3}
		value := runtimeports.PreparedExecutionGovernanceResultV2{Delegation: delegation, Prepared: request.Preparation.Prepared, Enforcement: enforcement}
		r.prepared = &value
		// Runtime persists Enforcement in the Permit fact before returning the
		// prepared result. Keep this fixture on that same authoritative
		// revision so a later unknown Authorization proves the exact +1 cause.
		if r.auth != nil {
			authorization := *r.auth
			authorization.PermitFactRevision = enforcement.RecordedRevision
			r.auth = &authorization
		}
	}
	if r.loseCommitReply {
		r.loseCommitReply = false
		return runtimeports.PreparedExecutionGovernanceResultV2{}, unavailableV3("CommitPrepared reply lost")
	}
	return *r.prepared, nil
}

func (r *coordinatorRuntimeV3) InspectPreparedExecutionV2(context.Context, runtimeports.OperationSubjectV3, string, string) (runtimeports.PreparedExecutionGovernanceResultV2, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.prepared == nil {
		return runtimeports.PreparedExecutionGovernanceResultV2{}, notFoundV3("prepared governance")
	}
	return *r.prepared, nil
}

func (r *coordinatorRuntimeV3) RelayPrepare(_ context.Context, request runtimeports.PrepareGovernedExecutionRequestV2) (runtimeports.ProviderPreparationAttestationV2, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providerCalls++
	if r.prepareUnknown {
		r.prepareCalls++
		return runtimeports.ProviderPreparationAttestationV2{}, unavailableV3("prepare reply unknown")
	}
	if r.attestation == nil {
		r.prepareCalls++
		permitDigest, _ := request.Permit.DigestV3()
		opDigest, _ := request.Intent.Operation.DigestV3()
		preparedID, _ := runtimeports.DerivePreparedProviderAttemptIDV2(request.Delegation.ID, request.Permit.ID, request.Permit.AttemptID)
		raw := runtimeports.PreparedProviderAttemptRefV2{ID: preparedID, Revision: 1, DeclaredDelegation: request.Delegation, OperationDigest: opDigest, IntentID: request.Intent.ID, IntentRevision: request.Intent.Revision, IntentDigest: request.Permit.IntentDigest, PermitID: request.Permit.ID, PermitRevision: request.Permit.Revision, PermitDigest: permitDigest, AttemptID: request.Permit.AttemptID, Provider: request.Permit.Provider, PayloadSchema: request.Intent.Payload.Schema, PayloadDigest: request.Intent.Payload.ContentDigest, PayloadRevision: request.Intent.PayloadRevision, PreparedUnixNano: r.now.UnixNano(), ExpiresUnixNano: r.now.Add(5 * time.Second).UnixNano()}
		prepared, err := runtimeports.SealPreparedProviderAttemptRefV2(raw)
		if err != nil {
			return runtimeports.ProviderPreparationAttestationV2{}, err
		}
		receipt := runtimeports.OperationEnforcementReceiptV3{ContractVersion: runtimeports.OperationEffectContractVersionV3, PermitID: request.Permit.ID, PermitRevision: request.Permit.Revision, AttemptID: request.Permit.AttemptID, PermitDigest: permitDigest, Operation: request.Intent.Operation, Verifier: request.Permit.EnforcementPoint, ValidatedUnixNano: r.now.UnixNano()}
		value := runtimeports.ProviderPreparationAttestationV2{ContractVersion: runtimeports.ExecutionGovernanceContractVersionV2, Delegation: request.Delegation, Prepared: prepared, Enforcement: receipt, ObservedUnixNano: r.now.UnixNano()}
		r.attestation = &value
	}
	if r.losePrepareReply {
		r.losePrepareReply = false
		return runtimeports.ProviderPreparationAttestationV2{}, unavailableV3("Prepare reply lost")
	}
	return *r.attestation, nil
}

func (r *coordinatorRuntimeV3) RelayInspectPrepared(context.Context, runtimeports.InspectPreparedProviderRequestV2) (runtimeports.ProviderPreparationAttestationV2, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.attestation == nil {
		return runtimeports.ProviderPreparationAttestationV2{}, notFoundV3("provider prepared")
	}
	return *r.attestation, nil
}

func (r *coordinatorRuntimeV3) RelayExecutePrepared(_ context.Context, request runtimeports.ExecutePreparedRequestV2) (runtimeports.ProviderAttemptObservationV2, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providerCalls++
	if r.executeUnknown {
		r.executeCalls++
		return runtimeports.ProviderAttemptObservationV2{}, unavailableV3("execute outcome unknown")
	}
	if r.local == nil {
		r.executeCalls++
		value := runtimeports.ProviderAttemptObservationV2{ContractVersion: runtimeports.ExecutionGovernanceContractVersionV2, Delegation: request.Delegation, Prepared: request.Prepared, Revision: 1, State: runtimeports.ProviderAttemptObservedV2, Payload: request.Intent.Payload, PayloadRevision: request.Intent.PayloadRevision, ProviderOperationRef: "provider-operation", SourceRegistrationID: "provider-source", SourceEpoch: 1, SourceSequence: 1, Evidence: runtimeports.EvidenceRecordRefV2{LedgerScopeDigest: core.DigestBytes([]byte("ledger")), Sequence: 1, RecordDigest: core.DigestBytes([]byte("record"))}, ObservedUnixNano: r.now.Add(time.Millisecond).UnixNano()}
		r.local = &value
	}
	if r.loseExecuteReply {
		return runtimeports.ProviderAttemptObservationV2{}, unavailableV3("execute reply lost")
	}
	return *r.local, nil
}

func (r *coordinatorRuntimeV3) RelayInspectLocalAttempt(context.Context, runtimeports.InspectLocalProviderAttemptRequestV2) (runtimeports.ProviderAttemptObservationV2, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.local == nil {
		return runtimeports.ProviderAttemptObservationV2{}, notFoundV3("local attempt")
	}
	return *r.local, nil
}

func (r *coordinatorRuntimeV3) RecordGovernedProviderObservationV3(_ context.Context, request runtimeports.RecordGovernedProviderObservationRequestV2) (runtimeports.ProviderAttemptObservationRefV2, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ref, err := request.Observation.RefV2()
	if err == nil {
		r.observation = &ref
	}
	if r.beforeObservationReturn != nil {
		r.beforeObservationReturn()
		r.beforeObservationReturn = nil
	}
	if err == nil && r.loseObservationReply {
		r.loseObservationReply = false
		return runtimeports.ProviderAttemptObservationRefV2{}, unavailableV3("Observation reply lost")
	}
	return ref, err
}

func (r *coordinatorRuntimeV3) InspectGovernedProviderObservationV3(context.Context, runtimeports.ExecutionDelegationRefV2, string) (runtimeports.ProviderAttemptObservationRefV2, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.observation == nil {
		return runtimeports.ProviderAttemptObservationRefV2{}, notFoundV3("observation")
	}
	return *r.observation, nil
}

func (r *coordinatorRuntimeV3) SettleOperationEffectV3(_ context.Context, _ runtimeports.OperationEffectIntentV3, submission runtimeports.OperationSettlementSubmissionV3) (runtimeports.OperationSettlementRefV3, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ref := r.settlementRef(submission)
	if r.settlement != nil && !sameSettlementForTestV3(*r.settlement, ref) {
		return runtimeports.OperationSettlementRefV3{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "settlement replay changed")
	}
	r.settlement = &ref
	if r.loseSettlementReply {
		r.loseSettlementReply = false
		return runtimeports.OperationSettlementRefV3{}, unavailableV3("Settlement reply lost")
	}
	return ref, nil
}

func (r *coordinatorRuntimeV3) InspectOperationSettlementV3(context.Context, runtimeports.OperationSubjectV3, core.EffectIntentID) (runtimeports.OperationSettlementRefV3, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.settlement == nil {
		return runtimeports.OperationSettlementRefV3{}, notFoundV3("settlement")
	}
	return *r.settlement, nil
}

func (r *coordinatorRuntimeV3) settlementRef(submission runtimeports.OperationSettlementSubmissionV3) runtimeports.OperationSettlementRefV3 {
	ref := runtimeports.OperationSettlementRefV3{ID: submission.ID, Revision: submission.Revision, Digest: core.DigestBytes([]byte("settlement-" + submission.ID)), Attempt: submission.Attempt, Disposition: submission.Disposition, Owner: submission.Owner, Observation: submission.Observation, InspectionEffect: submission.InspectionEffect, InspectionSettlement: submission.InspectionSettlement, Evidence: append([]runtimeports.EvidenceRecordRefV2(nil), submission.Evidence...)}
	// A provider/store round-trip must not depend on Go pointer identity. Return
	// semantically equal delegation values through distinct allocations so the
	// coordinator's exact-match check exercises wire/value semantics.
	if submission.Attempt.Delegation != nil {
		delegation := *submission.Attempt.Delegation
		ref.Attempt.Delegation = &delegation
	}
	if submission.InspectionEffect != nil && submission.InspectionEffect.Delegation != nil {
		inspection := *submission.InspectionEffect
		delegation := *submission.InspectionEffect.Delegation
		inspection.Delegation = &delegation
		ref.InspectionEffect = &inspection
	}
	if submission.DomainResult != nil {
		schema := submission.DomainResult.Schema
		ref.DomainResultSchema = &schema
		ref.DomainResultDigest = submission.DomainResult.ContentDigest
	}
	return ref
}

type coordinatorDomainV3 struct {
	mu                         sync.Mutex
	reservation                *contract.OperationDomainReservationRefV3
	current                    *applicationports.OperationDomainStateRefV3
	reserveCalls               int
	preparedCalls              int
	unknownCalls               int
	observedCalls              int
	settlementCalls            int
	lastSettlement             applicationports.ApplyOperationSettlementRequestV3
	lastPrepared               applicationports.BindPreparedOperationRequestV3
	forgeNextBasis             bool
	losePreparedReply          bool
	loseUnknownReply           bool
	loseObservedReply          bool
	loseSettlementReply        bool
	loseReservationReply       bool
	reserveErr                 error
	reservationExpiresUnixNano int64
	afterReserve               func(contract.OperationDomainReservationRefV3)
}

func (d *coordinatorDomainV3) ReserveOperationIntentV3(_ context.Context, request applicationports.ReserveOperationIntentRequestV3) (contract.OperationDomainReservationRefV3, error) {
	if err := request.Validate(); err != nil {
		return contract.OperationDomainReservationRefV3{}, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.reserveCalls++
	if d.reserveErr != nil {
		return contract.OperationDomainReservationRefV3{}, d.reserveErr
	}
	intentDigest, _ := request.Intent.DigestV3()
	operationDigest, _ := request.Intent.Operation.DigestV3()
	expiresUnixNano := request.Intent.ExpiresUnixNano
	if d.reservationExpiresUnixNano != 0 {
		expiresUnixNano = d.reservationExpiresUnixNano
	}
	value, err := contract.SealOperationDomainReservationRefV3(contract.OperationDomainReservationRefV3{
		ContractVersion: contract.GovernedOperationAttemptContractVersionV3,
		ID:              "domain-reservation:" + request.Attempt.ID, Revision: 1,
		StepKind: request.StepKind, Descriptor: request.Descriptor, DomainAdapter: request.DomainAdapter,
		AttemptID: request.Attempt.ID, AttemptRevision: request.Attempt.Revision, AttemptDigest: request.Attempt.Digest,
		IntentDigest: intentDigest, DomainSubjectDigest: operationDigest, SessionRef: "session-operation",
		CandidateDigest: request.Intent.Payload.ContentDigest, ReservedUnixNano: expiresUnixNano - int64(30*time.Second),
		ExpiresUnixNano: expiresUnixNano,
	})
	if err != nil {
		return contract.OperationDomainReservationRefV3{}, err
	}
	if d.reservation != nil {
		if d.reservation.Digest != value.Digest {
			return contract.OperationDomainReservationRefV3{}, core.NewError(core.ErrorConflict, core.ReasonOwnerConflict, "domain subject is reserved by another attempt")
		}
		return *d.reservation, nil
	}
	d.reservation = &value
	if d.afterReserve != nil {
		d.afterReserve(value)
	}
	if d.loseReservationReply {
		d.loseReservationReply = false
		return contract.OperationDomainReservationRefV3{}, unavailableV3("Domain reservation reply lost")
	}
	return value, nil
}

func (d *coordinatorDomainV3) InspectOperationIntentReservationV3(_ context.Context, request applicationports.InspectOperationIntentReservationRequestV3) (contract.OperationDomainReservationRefV3, error) {
	if err := request.Validate(); err != nil {
		return contract.OperationDomainReservationRefV3{}, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.reservation == nil {
		return contract.OperationDomainReservationRefV3{}, notFoundV3("domain reservation")
	}
	if d.reservation.AttemptID != request.AttemptID || d.reservation.StepKind != request.StepKind || d.reservation.DomainAdapter != request.DomainAdapter {
		return contract.OperationDomainReservationRefV3{}, notFoundV3("domain reservation")
	}
	return *d.reservation, nil
}

func (d *coordinatorDomainV3) BindPrepared(_ context.Context, request applicationports.BindPreparedOperationRequestV3) (applicationports.OperationDomainStateRefV3, error) {
	if err := request.Validate(); err != nil {
		return applicationports.OperationDomainStateRefV3{}, err
	}
	basis := struct {
		RuntimeAttempt runtimeports.GovernedExecutionAttemptRefsV2      `json:"runtime_attempt"`
		DelegationFact runtimeports.ExecutionDelegationFactV2           `json:"delegation_fact"`
		Prepared       runtimeports.PreparedExecutionGovernanceResultV2 `json:"prepared"`
	}{request.RuntimeAttempt, request.DelegationFact, request.Prepared}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.reservation == nil || request.Attempt.DomainReservation == nil || d.reservation.Digest != request.Attempt.DomainReservation.Digest {
		return applicationports.OperationDomainStateRefV3{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectIntentMissing, "BindPrepared lacks the exact Domain Reservation")
	}
	d.preparedCalls++
	d.lastPrepared = request
	value, err := d.advance(request.StepKind, request.Attempt, applicationports.OperationDomainPreparedV3, basis)
	if d.forgeNextBasis {
		d.forgeNextBasis = false
		value.BasisDigest = core.DigestBytes([]byte("forged-domain-basis"))
		d.current = &value
	}
	if d.losePreparedReply {
		d.losePreparedReply = false
		return applicationports.OperationDomainStateRefV3{}, unavailableV3("Domain prepared reply lost")
	}
	return value, err
}

func (d *coordinatorDomainV3) MarkUnknown(_ context.Context, request applicationports.MarkUnknownOperationRequestV3) (applicationports.OperationDomainStateRefV3, error) {
	if err := request.Validate(); err != nil {
		return applicationports.OperationDomainStateRefV3{}, err
	}
	basis := struct {
		RuntimeAttempt runtimeports.GovernedExecutionAttemptRefsV2   `json:"runtime_attempt"`
		Authorization  runtimeports.OperationDispatchAuthorizationV3 `json:"authorization"`
	}{request.RuntimeAttempt, request.Authorization}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.unknownCalls++
	value, err := d.advance(request.StepKind, request.Attempt, applicationports.OperationDomainUnknownV3, basis)
	if d.loseUnknownReply {
		d.loseUnknownReply = false
		return applicationports.OperationDomainStateRefV3{}, unavailableV3("Domain unknown reply lost")
	}
	return value, err
}

func (d *coordinatorDomainV3) BindObserved(_ context.Context, request applicationports.BindObservedOperationRequestV3) (applicationports.OperationDomainStateRefV3, error) {
	if err := request.Validate(); err != nil {
		return applicationports.OperationDomainStateRefV3{}, err
	}
	basis := struct {
		RuntimeAttempt runtimeports.GovernedExecutionAttemptRefsV2  `json:"runtime_attempt"`
		Observation    runtimeports.ProviderAttemptObservationRefV2 `json:"observation"`
	}{request.RuntimeAttempt, request.Observation}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.observedCalls++
	value, err := d.advance(request.StepKind, request.Attempt, applicationports.OperationDomainObservedV3, basis)
	if d.loseObservedReply {
		d.loseObservedReply = false
		return applicationports.OperationDomainStateRefV3{}, unavailableV3("Domain observed reply lost")
	}
	return value, err
}

func (d *coordinatorDomainV3) ApplySettlement(_ context.Context, request applicationports.ApplyOperationSettlementRequestV3) (applicationports.OperationDomainStateRefV3, error) {
	if err := request.Validate(); err != nil {
		return applicationports.OperationDomainStateRefV3{}, err
	}
	basis := struct {
		RuntimeAttempt *runtimeports.GovernedExecutionAttemptRefsV2 `json:"runtime_attempt,omitempty"`
		Settlement     runtimeports.OperationSettlementRefV3        `json:"settlement"`
		DomainResult   *runtimeports.OpaquePayloadV2                `json:"domain_result,omitempty"`
	}{request.RuntimeAttempt, request.Settlement, request.DomainResult}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.settlementCalls++
	d.lastSettlement = request
	value, err := d.advance(request.StepKind, request.Attempt, applicationports.OperationDomainSettledV3, basis)
	if d.loseSettlementReply {
		d.loseSettlementReply = false
		return applicationports.OperationDomainStateRefV3{}, unavailableV3("Domain settlement reply lost")
	}
	return value, err
}

func (d *coordinatorDomainV3) InspectOperationDomainStateV3(_ context.Context, _ applicationports.OperationDomainInspectRequestV3) (applicationports.OperationDomainStateRefV3, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.current == nil {
		return applicationports.OperationDomainStateRefV3{}, notFoundV3("domain")
	}
	return *d.current, nil
}

func (d *coordinatorDomainV3) advance(kind runtimeports.NamespacedNameV2, attempt contract.GovernedOperationAttemptRefV3, state applicationports.OperationDomainStateV3, basis any) (applicationports.OperationDomainStateRefV3, error) {
	basisDigest, err := applicationports.OperationDomainBasisDigestV3(basis)
	if err != nil {
		return applicationports.OperationDomainStateRefV3{}, err
	}
	revision := core.Revision(1)
	if d.current != nil {
		if d.current.State == state && d.current.StepKind == kind && d.current.BasisDigest == basisDigest {
			left, _ := core.CanonicalJSONDigest("praxis.application.test", applicationports.OperationDomainContractVersionV3, "attempt", d.current.Attempt)
			right, _ := core.CanonicalJSONDigest("praxis.application.test", applicationports.OperationDomainContractVersionV3, "attempt", attempt)
			if left == right {
				return *d.current, nil
			}
			return applicationports.OperationDomainStateRefV3{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "domain attempt ref changed under one exact state basis")
		}
		revision = d.current.Revision + 1
	}
	digest := core.DigestBytes([]byte(string(state) + string(basisDigest)))
	value := applicationports.OperationDomainStateRefV3{ContractVersion: applicationports.OperationDomainContractVersionV3, StepKind: kind, Attempt: attempt, State: state, Revision: revision, Digest: digest, BasisDigest: basisDigest}
	d.current = &value
	return value, nil
}

func settlementSubmissionForAttemptV3(t *testing.T, attempt contract.GovernedOperationAttemptFactV3, now time.Time) runtimeports.OperationSettlementSubmissionV3 {
	t.Helper()
	result := settlementDomainPayloadV3("coordinator-result")
	delegation := *attempt.PreparedDelegation
	dispatch := attempt.BegunAuthorization.Attempt
	dispatch.Delegation = &delegation
	evidence := runtimeports.EvidenceRecordRefV2{LedgerScopeDigest: core.DigestBytes([]byte("settlement-ledger")), Sequence: 2, RecordDigest: core.DigestBytes([]byte("settlement-record"))}
	return runtimeports.OperationSettlementSubmissionV3{ID: "settlement-coordinator", Revision: 1, Attempt: dispatch, Owner: runtimeports.EffectOwnerRefV2{Role: runtimeports.OwnerSettlement, ComponentID: attempt.IntentValue.Provider.ComponentID, ManifestDigest: attempt.IntentValue.Provider.ManifestDigest}, Disposition: runtimeports.OperationSettlementAppliedV3, Observation: attempt.Observation, Evidence: []runtimeports.EvidenceRecordRefV2{evidence}, DomainResult: &result, SettledUnixNano: now.Add(20 * time.Millisecond).UnixNano()}
}

func unknownSettlementSubmissionV3(t *testing.T, attempt contract.GovernedOperationAttemptFactV3, now time.Time) runtimeports.OperationSettlementSubmissionV3 {
	t.Helper()
	evidence := runtimeports.EvidenceRecordRefV2{LedgerScopeDigest: core.DigestBytes([]byte("unknown-ledger")), Sequence: 2, RecordDigest: core.DigestBytes([]byte("unknown-record"))}
	return runtimeports.OperationSettlementSubmissionV3{ID: "settlement-prepared-unknown", Revision: 1, Attempt: attempt.UnknownAuthorization.Attempt, Owner: runtimeports.EffectOwnerRefV2{Role: runtimeports.OwnerSettlement, ComponentID: attempt.IntentValue.Provider.ComponentID, ManifestDigest: attempt.IntentValue.Provider.ManifestDigest}, Disposition: runtimeports.OperationSettlementFailedV3, Evidence: []runtimeports.EvidenceRecordRefV2{evidence}, SettledUnixNano: now.Add(20 * time.Millisecond).UnixNano()}
}

func assertStepStateV3(t *testing.T, journal contract.WorkflowJournalV2, want contract.WorkflowStepStateV2) {
	t.Helper()
	if len(journal.Steps) != 1 || journal.Steps[0].State != want {
		t.Fatalf("workflow step state=%#v want=%s", journal.Steps, want)
	}
}

func notFoundV3(subject string) error {
	return core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, subject+" not found")
}

func unavailableV3(message string) error {
	return core.NewError(core.ErrorUnavailable, core.ReasonEffectUnknownOutcome, message)
}

func sameSettlementForTestV3(left, right runtimeports.OperationSettlementRefV3) bool {
	ld, _ := core.CanonicalJSONDigest("praxis.application.test", applicationports.OperationDomainContractVersionV3, "settlement", left)
	rd, _ := core.CanonicalJSONDigest("praxis.application.test", applicationports.OperationDomainContractVersionV3, "settlement", right)
	return ld == rd
}

func clonePayloadForTestV3(value *runtimeports.OpaquePayloadV2) *runtimeports.OpaquePayloadV2 {
	if value == nil {
		return nil
	}
	copy := *value
	copy.Inline = append([]byte(nil), value.Inline...)
	return &copy
}

func cloneProviderBindingForTestV3(value *runtimeports.ProviderBindingRefV2) *runtimeports.ProviderBindingRefV2 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func runtimeRefsForTestV3(fact contract.GovernedOperationAttemptFactV3) runtimeports.GovernedExecutionAttemptRefsV2 {
	a := fact.BegunAuthorization.Attempt
	refs := runtimeports.GovernedExecutionAttemptRefsV2{Admission: *fact.Admission, PermitID: a.PermitID, PermitRevision: a.PermitRevision, PermitDigest: a.PermitDigest, AttemptID: a.AttemptID, Delegation: *fact.PreparedDelegation, Prepared: *fact.Prepared, Enforcement: *fact.Enforcement}
	if fact.Observation != nil {
		o := *fact.Observation
		refs.Observation = &o
	}
	if fact.Settlement != nil {
		s := *fact.Settlement
		refs.Settlement = &s
	}
	return refs
}

func settledPostPreparedUnknownV3(t *testing.T, unknown contract.GovernedOperationAttemptFactV3) contract.GovernedOperationAttemptFactV3 {
	t.Helper()
	inspectionAttempt := unknown.UnknownAuthorization.Attempt
	inspectionAttempt.EffectID = "inspection-effect"
	inspectionAttempt.IntentDigest = core.DigestBytes([]byte("inspection-intent"))
	inspectionAttempt.PermitID = "inspection-permit"
	inspectionAttempt.PermitDigest = core.DigestBytes([]byte("inspection-permit"))
	inspectionAttempt.AttemptID = "inspection-attempt"
	inspectionAttempt.Delegation = nil
	evidence := runtimeports.EvidenceRecordRefV2{LedgerScopeDigest: core.DigestBytes([]byte("inspection-ledger")), Sequence: 1, RecordDigest: core.DigestBytes([]byte("inspection-record"))}
	inspectionSettlement := runtimeports.OperationSettlementRefV3{ID: "inspection-settlement", Revision: 1, Digest: core.DigestBytes([]byte("inspection-settlement")), Attempt: inspectionAttempt, Disposition: runtimeports.OperationSettlementAppliedV3, Owner: runtimeports.EffectOwnerRefV2{Role: runtimeports.OwnerSettlement, ComponentID: unknown.IntentValue.Provider.ComponentID, ManifestDigest: unknown.IntentValue.Provider.ManifestDigest}, Evidence: []runtimeports.EvidenceRecordRefV2{evidence}}
	inspectionSettlementRef, err := inspectionSettlement.InspectionRefV3()
	if err != nil {
		t.Fatal(err)
	}
	value := unknown
	value.Revision++
	value.State = contract.OperationSettledV3
	value.UpdatedUnixNano++
	attempt := unknown.UnknownAuthorization.Attempt
	delegation := *unknown.PreparedDelegation
	attempt.Delegation = &delegation
	value.Settlement = &runtimeports.OperationSettlementRefV3{ID: "unknown-postprepared-settlement", Revision: 1, Digest: core.DigestBytes([]byte("unknown-postprepared-settlement")), Attempt: attempt, Disposition: runtimeports.OperationSettlementFailedV3, Owner: runtimeports.EffectOwnerRefV2{Role: runtimeports.OwnerSettlement, ComponentID: unknown.IntentValue.Provider.ComponentID, ManifestDigest: unknown.IntentValue.Provider.ManifestDigest}, InspectionEffect: &inspectionAttempt, InspectionSettlement: &inspectionSettlementRef, Evidence: []runtimeports.EvidenceRecordRefV2{evidence}}
	if err := value.Validate(); err != nil {
		t.Fatal(err)
	}
	return value
}
