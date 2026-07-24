package fakes_test

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/fakes"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestOperationSettlementV4LostReplyCurrentClosureAndConformance(t *testing.T) {
	fixture := newOperationSettlementFixtureV4(t, "happy")
	fixture.effect.effect.store.LoseNextOperationSettlementV4Reply()
	ref, err := fixture.gateway.SettleOperationV4(context.Background(), fixture.submission)
	if err != nil {
		t.Fatal(err)
	}
	if fixture.effect.effect.store.OperationSettlementV4CommitCount() != 1 || fixture.providerCalls.Load() != 0 {
		t.Fatalf("V4 settlement did not linearize once without Provider calls: commits=%d providers=%d", fixture.effect.effect.store.OperationSettlementV4CommitCount(), fixture.providerCalls.Load())
	}
	replayed, err := fixture.gateway.SettleOperationV4(context.Background(), fixture.submission)
	if err != nil || !ports.SameOperationSettlementRefV4(ref, replayed) {
		t.Fatalf("lost-reply replay was not exact: ref=%#v err=%v", replayed, err)
	}
	current, err := fixture.gateway.InspectCurrentOperationSettlementV4(context.Background(), ports.InspectCurrentOperationSettlementRequestV4{Operation: fixture.submission.Operation, EffectID: fixture.submission.EffectID})
	if err != nil {
		t.Fatal(err)
	}
	association, err := fixture.effect.effect.store.InspectOperationSettlementEvidenceAssociationV4(context.Background(), fixture.submission.Operation, current.Association)
	if err != nil || association.Prepare.Phase != ports.OperationDispatchEnforcementPrepareV4 || association.Execute.Phase != ports.OperationDispatchEnforcementExecuteV4 {
		t.Fatalf("public current closure did not recover exact phases: %#v err=%v", association, err)
	}
	changed := fixture.submission
	changed.IdempotencyKey = "changed-idempotency"
	changed, err = ports.SealOperationSettlementSubmissionV4(changed)
	if err != nil {
		t.Fatal(err)
	}
	report, err := conformance.CheckOperationSettlementV4(context.Background(), conformance.OperationSettlementConformanceCaseV4{Governance: fixture.gateway, Submission: fixture.submission, Changed: changed})
	if err != nil {
		t.Fatal(err)
	}
	if !report.SameContentReplayIdempotent || !report.ChangedContentConflicts || !report.HistoricalClosureExact || !report.CurrentClosureExact || report.ProviderCalled || report.EvidenceReconsumed || report.ProductionClaimEligible {
		t.Fatalf("unexpected public V4 conformance report: %#v", report)
	}
}

func TestOperationSettlementV4PublicClosureRejectsRefDriftAndMissingObjects(t *testing.T) {
	fixture := newOperationSettlementFixtureV4(t, "closure-drift")
	if _, err := fixture.gateway.SettleOperationV4(context.Background(), fixture.submission); err != nil {
		t.Fatal(err)
	}
	bundle, err := fixture.effect.effect.store.InspectOperationSettlementV4(context.Background(), fixture.submission.Operation, fixture.submission.ID)
	if err != nil {
		t.Fatal(err)
	}
	associationRef := bundle.Association.RefV4()
	associationRef.ID = "another-association"
	if _, err := fixture.effect.effect.store.InspectOperationSettlementEvidenceAssociationV4(context.Background(), fixture.submission.Operation, associationRef); err == nil {
		t.Fatal("drifted association ref was accepted")
	}
	guardRef := bundle.Guard.RefV4()
	guardRef.Digest = digestV3("another-guard")
	if _, err := fixture.effect.effect.store.InspectOperationSettlementTerminalGuardV4(context.Background(), fixture.submission.Operation, guardRef); err == nil {
		t.Fatal("drifted guard ref was accepted")
	}
	projectionRef := bundle.Projection.RefV4()
	projectionRef.Association.Digest = digestV3("another-association")
	if _, err := fixture.effect.effect.store.InspectOperationSettlementTerminalProjectionV4(context.Background(), fixture.submission.Operation, projectionRef); err == nil {
		t.Fatal("drifted projection ref was accepted")
	}

	changed := fixture.submission
	changed.IdempotencyKey = "changed-closure-idempotency"
	changed, err = ports.SealOperationSettlementSubmissionV4(changed)
	if err != nil {
		t.Fatal(err)
	}
	broken := &brokenOperationSettlementGovernancePortV4{OperationSettlementGovernancePortV4: fixture.gateway, corruptGuard: true}
	report, err := conformance.CheckOperationSettlementV4(context.Background(), conformance.OperationSettlementConformanceCaseV4{Governance: broken, Submission: fixture.submission, Changed: changed})
	if err != nil {
		t.Fatal(err)
	}
	if !report.HistoricalClosureExact || report.CurrentClosureExact {
		t.Fatalf("historical/current closure separation was not preserved with a corrupt current guard: %#v", report)
	}
	broken = &brokenOperationSettlementGovernancePortV4{OperationSettlementGovernancePortV4: fixture.gateway, missingProjection: true}
	if _, err := conformance.CheckOperationSettlementV4(context.Background(), conformance.OperationSettlementConformanceCaseV4{Governance: broken, Submission: fixture.submission, Changed: changed}); err == nil {
		t.Fatal("public Conformance accepted a missing terminal projection")
	}

	wrongOperation := fixture.submission.Operation
	wrongOperation.ActivationAttemptID = "another-activation-attempt"
	if _, err := fixture.gateway.InspectOperationSettlementEvidenceAssociationV4(context.Background(), wrongOperation, bundle.Association.RefV4()); err == nil {
		t.Fatal("governance read surface accepted an association for another operation")
	}
	if fixture.providerCalls.Load() != 0 {
		t.Fatal("governance read surface called Provider")
	}

	// Historical closure is independently readable even when a current index
	// adapter is unavailable.
	if _, err := fixture.gateway.InspectOperationSettlementClosureV4(context.Background(), ports.InspectOperationSettlementRequestV4{Operation: fixture.submission.Operation, SettlementID: fixture.submission.ID}); err != nil {
		t.Fatalf("historical four-object closure was not independently readable: %v", err)
	}
	missingCurrent := &brokenOperationSettlementGovernancePortV4{OperationSettlementGovernancePortV4: fixture.gateway, missingCurrent: true}
	if _, err := missingCurrent.InspectCurrentOperationSettlementV4(context.Background(), ports.InspectCurrentOperationSettlementRequestV4{Operation: fixture.submission.Operation, EffectID: fixture.submission.EffectID}); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("current index failure was not isolated from historical closure: %v", err)
	}

	missingFact := &brokenOperationSettlementFactPortV4{OperationSettlementFactPortV4: fixture.effect.effect.store, missingGuard: true}
	brokenGateway := fixture.gateway
	brokenGateway.Facts = missingFact
	if _, err := brokenGateway.InspectOperationSettlementClosureV4(context.Background(), ports.InspectOperationSettlementRequestV4{Operation: fixture.submission.Operation, SettlementID: fixture.submission.ID}); err == nil {
		t.Fatal("historical closure accepted a missing guard object")
	}
}

func TestOperationSettlementV4EvidenceAndDomainDriftFailBeforeCommit(t *testing.T) {
	tests := []struct {
		name       string
		wantReason core.ReasonCode
		mutate     func(*operationSettlementFixtureV4)
	}{
		{name: "consumption_digest", mutate: func(f *operationSettlementFixtureV4) { f.submission.Evidence[0].Consumption.Digest = digestV3("drift") }},
		{name: "issued_revision", mutate: func(f *operationSettlementFixtureV4) { f.submission.Evidence[0].IssuedQualification.Revision++ }},
		{name: "final_digest", mutate: func(f *operationSettlementFixtureV4) {
			f.submission.Evidence[0].FinalQualification.Digest = digestV3("drift")
		}},
		{name: "record", mutate: func(f *operationSettlementFixtureV4) {
			f.submission.Evidence[0].Record.RecordDigest = digestV3("drift")
		}},
		{name: "v2 record type-pun", wantReason: core.ReasonEvidenceConflict, mutate: func(f *operationSettlementFixtureV4) {
			legacy := ports.EvidenceRecordRefV2{LedgerScopeDigest: digestV3("legacy-ledger"), Sequence: 99, RecordDigest: digestV3("legacy-record")}
			f.submission.Evidence[0].Record = ports.OperationScopeEvidenceRecordRefV3{LedgerScopeDigest: legacy.LedgerScopeDigest, Sequence: legacy.Sequence, RecordDigest: legacy.RecordDigest}
			f.submission.Evidence[0].Consumption.Record = f.submission.Evidence[0].Record
		}},
		{name: "candidate", mutate: func(f *operationSettlementFixtureV4) { f.submission.Evidence[0].CandidateDigest = digestV3("drift") }},
		{name: "handoff", mutate: func(f *operationSettlementFixtureV4) { f.submission.Evidence[0].Handoff.Digest = digestV3("drift") }},
		{name: "attempt", mutate: func(f *operationSettlementFixtureV4) { f.submission.Evidence[0].Attempt.AttemptID = "another-attempt" }},
		{name: "enforcement_phase", mutate: func(f *operationSettlementFixtureV4) {
			f.submission.Evidence[0].EnforcementPhase.ReceiptDigest = digestV3("drift")
		}},
		{name: "scope", mutate: func(f *operationSettlementFixtureV4) {
			f.submission.Evidence[0].OperationScopeDigest = digestV3("drift")
		}},
		{name: "phase scope swap", wantReason: core.ReasonEvidenceScopeConflict, mutate: func(f *operationSettlementFixtureV4) {
			f.submission.Evidence[0].OperationScopeDigest, f.submission.Evidence[1].OperationScopeDigest = f.submission.Evidence[1].OperationScopeDigest, f.submission.Evidence[0].OperationScopeDigest
			f.submission.OperationScopeDigest, _ = ports.DigestOperationSettlementScopeSetV4(f.submission.Evidence)
		}},
		{name: "domain_fact", mutate: func(f *operationSettlementFixtureV4) { f.domain.value.Fact.Digest = digestV3("drift") }},
		{name: "operation_identity", mutate: func(f *operationSettlementFixtureV4) {
			f.submission.DomainResult.Operation.SubjectRevision++
			f.submission.DomainResult.Operation.CurrentProjectionRevision++
			f.submission.DomainResult.OperationDigest, _ = f.submission.DomainResult.Operation.DigestV3()
			f.submission.DomainResult.Attempt.OperationDigest = f.submission.DomainResult.OperationDigest
		}},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newOperationSettlementFixtureV4(t, "drift-"+operationSettlementTestSlugV4(testCase.name))
			testCase.mutate(fixture)
			fixture.resealSubmissionIgnoringError()
			_, err := fixture.gateway.SettleOperationV4(context.Background(), fixture.submission)
			if err == nil {
				t.Fatal("drifted V4 settlement unexpectedly committed")
			}
			if core.HasReason(err, core.ReasonReviewVerdictMissing) {
				t.Fatalf("fixture failed before reaching the V4 drift gate: %v", err)
			}
			if testCase.wantReason != "" && !core.HasReason(err, testCase.wantReason) {
				t.Fatalf("V4 drift reached the wrong gate: want=%s err=%v", testCase.wantReason, err)
			}
			assertNoOperationSettlementV4Write(t, fixture)
		})
	}
}

func operationSettlementTestSlugV4(name string) string {
	return strings.NewReplacer(" ", "-", "_", "-").Replace(name)
}

func TestOperationSettlementV4FreshCurrentnessClockAndReaderFailure(t *testing.T) {
	t.Run("lease expires between reads", func(t *testing.T) {
		fixture := newOperationSettlementFixtureV4(t, "lease-expiry")
		first := fixture.now
		fixture.domain.value.CheckedUnixNano = first.Add(-time.Nanosecond).UnixNano()
		fixture.domain.value.ExpiresUnixNano = first.Add(time.Nanosecond).UnixNano()
		fixture.domain.value, _ = ports.SealOperationSettlementDomainResultCurrentV4(fixture.domain.value, first)
		var calls atomic.Int64
		fixture.gateway.Clock = func() time.Time {
			if calls.Add(1) == 1 {
				return first
			}
			return first.Add(2 * time.Nanosecond)
		}
		if _, err := fixture.gateway.SettleOperationV4(context.Background(), fixture.submission); err == nil {
			t.Fatal("expired second-read DomainResult lease committed")
		}
		assertNoOperationSettlementV4Write(t, fixture)
	})
	t.Run("clock rollback", func(t *testing.T) {
		fixture := newOperationSettlementFixtureV4(t, "clock-rollback")
		var calls atomic.Int64
		fixture.gateway.Clock = func() time.Time {
			if calls.Add(1) == 1 {
				return fixture.now
			}
			return fixture.now.Add(-time.Nanosecond)
		}
		if _, err := fixture.gateway.SettleOperationV4(context.Background(), fixture.submission); !core.HasReason(err, core.ReasonClockRegression) {
			t.Fatalf("clock rollback was not rejected: %v", err)
		}
		assertNoOperationSettlementV4Write(t, fixture)
	})
	t.Run("domain reader unavailable", func(t *testing.T) {
		fixture := newOperationSettlementFixtureV4(t, "domain-unavailable")
		fixture.domain.err = core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected DomainResult reader outage")
		if _, err := fixture.gateway.SettleOperationV4(context.Background(), fixture.submission); !core.HasCategory(err, core.ErrorUnavailable) {
			t.Fatalf("reader outage did not fail closed: %v", err)
		}
		assertNoOperationSettlementV4Write(t, fixture)
	})
	t.Run("evidence reader unavailable", func(t *testing.T) {
		fixture := newOperationSettlementFixtureV4(t, "evidence-unavailable")
		fixture.evidence.err = core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected Evidence reader outage")
		if _, err := fixture.gateway.SettleOperationV4(context.Background(), fixture.submission); !core.HasCategory(err, core.ErrorUnavailable) {
			t.Fatalf("Evidence reader outage did not fail closed: %v", err)
		}
		assertNoOperationSettlementV4Write(t, fixture)
	})
	t.Run("enforcement reader unavailable", func(t *testing.T) {
		fixture := newOperationSettlementFixtureV4(t, "enforcement-unavailable")
		fixture.gateway.Enforcement = unavailableOperationSettlementEnforcementReaderV4{}
		if _, err := fixture.gateway.SettleOperationV4(context.Background(), fixture.submission); !core.HasCategory(err, core.ErrorUnavailable) {
			t.Fatalf("Enforcement reader outage did not fail closed: %v", err)
		}
		assertNoOperationSettlementV4Write(t, fixture)
	})
}

func TestOperationSettlementV4RejectsLateOrObservationQualification(t *testing.T) {
	t.Run("late consumption", func(t *testing.T) {
		fixture := newOperationSettlementFixtureV4(t, "late")
		binding := &fixture.submission.Evidence[0]
		consumption := fixture.evidence.consumptions[binding.Consumption.ID]
		consumption.LateObservation = true
		consumption.Digest = ""
		consumption, err := ports.SealOperationScopeEvidenceConsumptionFactV3(consumption)
		if err != nil {
			t.Fatal(err)
		}
		fixture.evidence.consumptions[consumption.ID] = consumption
		binding.Consumption = consumption.RefV3()
		final := fixture.evidence.qualifications[binding.FinalQualification.ID]
		final.Consumption = ptrSettlementV4(consumption.RefV3())
		final.Digest = ""
		final, err = ports.SealOperationScopeEvidenceQualificationFactV3(final)
		if err != nil {
			t.Fatal(err)
		}
		fixture.evidence.qualifications[final.ID] = final
		binding.FinalQualification = final.RefV3()
		fixture.resealSubmissionIgnoringError()
		if _, err := fixture.gateway.SettleOperationV4(context.Background(), fixture.submission); err == nil {
			t.Fatal("late observation formed a V4 terminal settlement")
		}
		assertNoOperationSettlementV4Write(t, fixture)
	})
	t.Run("consumed observation", func(t *testing.T) {
		fixture := newOperationSettlementFixtureV4(t, "observation")
		binding := &fixture.submission.Evidence[0]
		final := fixture.evidence.qualifications[binding.FinalQualification.ID]
		final.State = ports.OperationScopeEvidenceConsumedObservationV3
		final.Digest = ""
		sealed, err := ports.SealOperationScopeEvidenceQualificationFactV3(final)
		if err != nil {
			t.Fatal(err)
		}
		fixture.evidence.qualifications[sealed.ID] = sealed
		binding.FinalQualification = sealed.RefV3()
		fixture.resealSubmissionIgnoringError()
		if _, err := fixture.gateway.SettleOperationV4(context.Background(), fixture.submission); err == nil {
			t.Fatal("consumed_observation formed a V4 terminal settlement")
		}
		assertNoOperationSettlementV4Write(t, fixture)
	})
}

func TestOperationSettlementV4TruthfulHistoricalPermitExpiryDoesNotReauthorize(t *testing.T) {
	fixture := newOperationSettlementFixtureV4(t, "historical-ttl")
	// Advance beyond both the dispatch Permit and the Review/Policy currentness
	// windows. Exact historical provenance remains truthful but grants no new
	// dispatch authority.
	fixture.now = fixture.now.Add(21 * time.Second)
	fixture.domain.value.CheckedUnixNano = fixture.now.UnixNano()
	fixture.domain.value.ExpiresUnixNano = fixture.now.Add(10 * time.Second).UnixNano()
	fixture.domain.value.Digest = ""
	var err error
	fixture.domain.value, err = ports.SealOperationSettlementDomainResultCurrentV4(fixture.domain.value, fixture.now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.gateway.SettleOperationV4(context.Background(), fixture.submission); err != nil {
		t.Fatalf("truthful settlement was blocked by historical dispatch TTL: %v", err)
	}
	if fixture.providerCalls.Load() != 0 {
		t.Fatal("truthful historical settlement called Provider")
	}
}

func TestOperationSettlementV4ClosedActivationMatrix(t *testing.T) {
	for _, kind := range []ports.EffectKindV2{"praxis.sandbox/allocate", "praxis.sandbox/activate", "praxis.sandbox/open", "praxis.sandbox/inspect"} {
		t.Run("allows "+string(kind), func(t *testing.T) {
			effect := newActivationOperationEnforcementFixtureForEffectKindV4(t, "matrix-allow-"+string(kind[len("praxis.sandbox/"):]), kind)
			fixture := newOperationSettlementFixtureFromEnforcementV4(t, effect, "matrix-allow-"+string(kind[len("praxis.sandbox/"):]))
			if _, err := fixture.gateway.SettleOperationV4(context.Background(), fixture.submission); err != nil {
				t.Fatalf("allowed activation matrix entry was rejected: %v", err)
			}
		})
	}
	for _, kind := range []ports.EffectKindV2{
		"praxis.sandbox/backend-discovery", "praxis.sandbox/cancel", "praxis.sandbox/rollback",
		"praxis.sandbox/close", "praxis.sandbox/release", "custom/activation",
	} {
		t.Run("rejects "+string(kind), func(t *testing.T) {
			effect := newActivationOperationEnforcementFixtureForEffectKindV4(t, "matrix-reject", kind)
			fixture := newOperationSettlementFixtureFromEnforcementV4(t, effect, "matrix-reject")
			if _, err := fixture.gateway.SettleOperationV4(context.Background(), fixture.submission); err == nil {
				t.Fatal("unsupported activation Effect kind formed a V4 settlement")
			}
			assertNoOperationSettlementV4Write(t, fixture)
		})
	}
	t.Run("rejects run operation scope", func(t *testing.T) {
		effect := newOperationEnforcementFixtureV4(t, "matrix-run")
		fixture := newOperationSettlementFixtureFromEnforcementV4(t, effect, "matrix-run")
		if _, err := fixture.gateway.SettleOperationV4(context.Background(), fixture.submission); err == nil {
			t.Fatal("run operation scope formed an activation first-slice settlement")
		}
		assertNoOperationSettlementV4Write(t, fixture)
	})
}

type unavailableOperationSettlementEnforcementReaderV4 struct{}

func (unavailableOperationSettlementEnforcementReaderV4) InspectOperationDispatchEnforcementV4(context.Context, ports.OperationSubjectV3, core.EffectIntentID, string) (ports.OperationDispatchEnforcementJournalV4, error) {
	return ports.OperationDispatchEnforcementJournalV4{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected Enforcement reader outage")
}

type brokenOperationSettlementGovernancePortV4 struct {
	ports.OperationSettlementGovernancePortV4
	corruptGuard      bool
	missingProjection bool
	missingCurrent    bool
}

func (p *brokenOperationSettlementGovernancePortV4) InspectCurrentOperationSettlementV4(ctx context.Context, request ports.InspectCurrentOperationSettlementRequestV4) (ports.OperationInspectionSettlementRefV4, error) {
	if p.missingCurrent {
		return ports.OperationInspectionSettlementRefV4{}, core.NewError(core.ErrorNotFound, core.ReasonEffectSettlementMissing, "injected missing V4 current index")
	}
	return p.OperationSettlementGovernancePortV4.InspectCurrentOperationSettlementV4(ctx, request)
}

func (p *brokenOperationSettlementGovernancePortV4) InspectOperationSettlementTerminalGuardV4(ctx context.Context, operation ports.OperationSubjectV3, ref ports.OperationSettlementTerminalGuardRefV4) (ports.OperationSettlementTerminalGuardV4, error) {
	guard, err := p.OperationSettlementGovernancePortV4.InspectOperationSettlementTerminalGuardV4(ctx, operation, ref)
	if err == nil && p.corruptGuard {
		guard.ID = "corrupt-guard"
	}
	return guard, err
}

func (p *brokenOperationSettlementGovernancePortV4) InspectOperationSettlementTerminalProjectionV4(ctx context.Context, operation ports.OperationSubjectV3, ref ports.OperationSettlementTerminalProjectionRefV4) (ports.OperationSettlementTerminalProjectionV4, error) {
	if p.missingProjection {
		return ports.OperationSettlementTerminalProjectionV4{}, core.NewError(core.ErrorNotFound, core.ReasonEffectSettlementMissing, "injected missing V4 projection")
	}
	return p.OperationSettlementGovernancePortV4.InspectOperationSettlementTerminalProjectionV4(ctx, operation, ref)
}

type brokenOperationSettlementFactPortV4 struct {
	ports.OperationSettlementFactPortV4
	missingGuard bool
}

func (p *brokenOperationSettlementFactPortV4) InspectOperationSettlementTerminalGuardV4(ctx context.Context, operation ports.OperationSubjectV3, ref ports.OperationSettlementTerminalGuardRefV4) (ports.OperationSettlementTerminalGuardV4, error) {
	if p.missingGuard {
		return ports.OperationSettlementTerminalGuardV4{}, core.NewError(core.ErrorNotFound, core.ReasonEffectSettlementMissing, "injected missing V4 historical guard")
	}
	return p.OperationSettlementFactPortV4.InspectOperationSettlementTerminalGuardV4(ctx, operation, ref)
}

func TestOperationSettlementV4AtomicFailureAndConcurrentCreateOnce(t *testing.T) {
	for stage := 1; stage <= 5; stage++ {
		fixture := newOperationSettlementFixtureV4(t, "atomic-stage-"+strconv.Itoa(stage))
		fixture.effect.effect.store.FailNextOperationSettlementV4CommitAfterStage(stage)
		if _, err := fixture.gateway.SettleOperationV4(context.Background(), fixture.submission); !core.HasCategory(err, core.ErrorUnavailable) {
			t.Fatalf("injected staged failure %d was not visible: %v", stage, err)
		}
		assertNoOperationSettlementV4Write(t, fixture)
		if _, err := fixture.gateway.SettleOperationV4(context.Background(), fixture.submission); err != nil {
			t.Fatalf("canonical retry after staged failure %d did not commit: %v", stage, err)
		}
	}

	fixture := newOperationSettlementFixtureV4(t, "atomic-concurrent")

	const workers = 64
	refs := make(chan ports.OperationSettlementRefV4, workers)
	errs := make(chan error, workers)
	var wait sync.WaitGroup
	for range workers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			ref, err := fixture.gateway.SettleOperationV4(context.Background(), fixture.submission)
			if err != nil {
				errs <- err
				return
			}
			refs <- ref
		}()
	}
	wait.Wait()
	close(refs)
	close(errs)
	if len(errs) != 0 {
		for err := range errs {
			t.Fatalf("same-content concurrent settlement failed: %v", err)
		}
	}
	var first ports.OperationSettlementRefV4
	for ref := range refs {
		if first.ID == "" {
			first = ref
		} else if !ports.SameOperationSettlementRefV4(first, ref) {
			t.Fatal("same-content concurrency returned multiple terminal facts")
		}
	}
	if fixture.effect.effect.store.OperationSettlementV4CommitCount() != 1 || fixture.providerCalls.Load() != 0 {
		t.Fatalf("concurrency linearized commits=%d providerCalls=%d", fixture.effect.effect.store.OperationSettlementV4CommitCount(), fixture.providerCalls.Load())
	}
}

func TestOperationSettlementV4ConcurrentChangedContentHasOneWinner(t *testing.T) {
	fixture := newOperationSettlementFixtureV4(t, "changed-concurrent")
	left := fixture.submission
	right := fixture.submission
	right.ID = "operation-settlement-changed-concurrent-other"
	right.IdempotencyKey = "settlement-idempotency-changed-concurrent-other"
	right.ConflictDomain = digestV3("settlement-conflict-changed-concurrent-other")
	var err error
	right, err = ports.SealOperationSettlementSubmissionV4(right)
	if err != nil {
		t.Fatal(err)
	}
	const workers = 64
	var successes atomic.Int64
	var conflicts atomic.Int64
	var wait sync.WaitGroup
	for index := 0; index < workers; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			value := left
			if index%2 == 1 {
				value = right
			}
			if _, err := fixture.gateway.SettleOperationV4(context.Background(), value); err == nil {
				successes.Add(1)
			} else if core.HasCategory(err, core.ErrorConflict) {
				conflicts.Add(1)
			}
		}(index)
	}
	wait.Wait()
	if successes.Load() == 0 || conflicts.Load() == 0 || fixture.effect.effect.store.OperationSettlementV4CommitCount() != 1 {
		t.Fatalf("changed-content race did not select one canonical terminal fact: success=%d conflict=%d commits=%d", successes.Load(), conflicts.Load(), fixture.effect.effect.store.OperationSettlementV4CommitCount())
	}
}

func TestOperationSettlementV4SharedGuardWithLegacyV3(t *testing.T) {
	t.Run("V3 settled blocks V4 with another ID and operation digest", func(t *testing.T) {
		legacy := newLegacyOperationSettlementFixtureV3(t, "v3-first")
		if _, err := legacy.gateway.SettleOperationEffectV3(context.Background(), legacy.effect.intent, legacy.submission); err != nil {
			t.Fatal(err)
		}
		base := newOperationSettlementFixtureV4(t, "retarget-v3-first")
		current, err := legacy.effect.store.InspectOperationEffectV3(context.Background(), legacy.effect.intent.Operation, legacy.effect.intent.ID)
		if err != nil {
			t.Fatal(err)
		}
		forgedEffect := current
		forgedEffect.Intent.Operation.SubjectRevision++
		forgedEffect.Intent.Operation.CurrentProjectionRevision++
		attempt := legacy.submission.Attempt
		attempt.OperationDigest, err = forgedEffect.Intent.Operation.DigestV3()
		if err != nil {
			t.Fatal(err)
		}
		retargeted := retargetOperationSettlementSubmissionV4(t, base.submission, forgedEffect, attempt, "v4-after-v3-different-id")
		if retargeted.ID == legacy.submission.ID || retargeted.OperationDigest == legacy.submission.Attempt.OperationDigest {
			t.Fatal("test did not change both Settlement ID and Operation digest")
		}
		bundle, err := control.BuildOperationSettlementCommitBundleV4(retargeted)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := legacy.effect.store.CommitOperationSettlementV4(context.Background(), ports.OperationSettlementCommitRequestV4{ExpectedEffectRevision: current.Revision, Bundle: bundle}); !core.HasReason(err, core.ReasonEffectStateConflict) {
			t.Fatalf("V3 terminal fact did not occupy the shared guard: %v", err)
		}
		if _, err := legacy.effect.store.InspectOperationSettlementV4(context.Background(), retargeted.Operation, retargeted.ID); !core.HasCategory(err, core.ErrorNotFound) {
			t.Fatalf("V3 terminal fact acquired a V4 sidecar: %v", err)
		}
	})
	t.Run("V4 settled blocks V3 with another settlement ID", func(t *testing.T) {
		fixture := newOperationSettlementFixtureV4(t, "v4-first")
		if _, err := fixture.gateway.SettleOperationV4(context.Background(), fixture.submission); err != nil {
			t.Fatal(err)
		}
		current, err := fixture.effect.effect.store.InspectOperationEffectV3(context.Background(), fixture.submission.Operation, fixture.submission.EffectID)
		if err != nil {
			t.Fatal(err)
		}
		next := current
		next.State = control.OperationEffectSettledV3
		next.Revision++
		next.Settlement = legacySettlementFactForV4Guard(t, fixture)
		if next.Settlement.ID == fixture.submission.ID {
			t.Fatal("test did not use a distinct V3 Settlement ID")
		}
		if _, err := fixture.effect.effect.store.CompareAndSwapOperationEffectV3(context.Background(), fixture.submission.Operation, control.OperationEffectCASRequestV3{ExpectedRevision: current.Revision, Next: next}); !core.HasReason(err, core.ReasonEffectStateConflict) {
			t.Fatalf("V4 terminal guard did not reject V3 terminal CAS: %v", err)
		}
	})
}

func TestOperationSettlementV4HistoricalGuardInspectIgnoresCurrentIndex(t *testing.T) {
	historical := newOperationSettlementFixtureV4(t, "historical-guard")
	if _, err := historical.gateway.SettleOperationV4(context.Background(), historical.submission); err != nil {
		t.Fatal(err)
	}
	historicalBundle, err := historical.effect.effect.store.InspectOperationSettlementV4(context.Background(), historical.submission.Operation, historical.submission.ID)
	if err != nil {
		t.Fatal(err)
	}

	replacement := newOperationSettlementFixtureV4(t, "replacement-current")
	if _, err := replacement.gateway.SettleOperationV4(context.Background(), replacement.submission); err != nil {
		t.Fatal(err)
	}
	replacementBundle, err := replacement.effect.effect.store.InspectOperationSettlementV4(context.Background(), replacement.submission.Operation, replacement.submission.ID)
	if err != nil {
		t.Fatal(err)
	}
	if replacementBundle.Settlement.Submission.ID == historicalBundle.Settlement.Submission.ID {
		t.Fatal("test did not construct a distinct current-index object")
	}
	if err := historical.effect.effect.store.ReplaceOperationSettlementCurrentIndexForTestV4(
		historical.submission.TenantID,
		historical.submission.EffectID,
		replacementBundle,
	); err != nil {
		t.Fatal(err)
	}

	guard, err := historical.effect.effect.store.InspectOperationSettlementTerminalGuardV4(
		context.Background(),
		historical.submission.Operation,
		historicalBundle.Guard.RefV4(),
	)
	if err != nil {
		t.Fatalf("historical guard Inspect borrowed the changed current index: %v", err)
	}
	if !ports.SameOperationSettlementTerminalGuardRefV4(guard.RefV4(), historicalBundle.Guard.RefV4()) {
		t.Fatalf("historical guard Inspect returned another closure: got=%#v want=%#v", guard.RefV4(), historicalBundle.Guard.RefV4())
	}
}

func TestOperationSettlementV4SharedGuardRejectsDifferentOperationDigest(t *testing.T) {
	fixture := newOperationSettlementFixtureV4(t, "guard-operation")
	original, err := fixture.gateway.SettleOperationV4(context.Background(), fixture.submission)
	if err != nil {
		t.Fatal(err)
	}
	current, err := fixture.effect.effect.store.InspectOperationEffectV3(context.Background(), fixture.submission.Operation, fixture.submission.EffectID)
	if err != nil {
		t.Fatal(err)
	}
	forgedEffect := current
	forgedEffect.Intent.Operation.ActivationAttemptID = "another-activation-attempt"
	attempt := fixture.submission.DomainResult.Attempt
	attempt.OperationDigest, _ = forgedEffect.Intent.Operation.DigestV3()
	retargeted := retargetOperationSettlementSubmissionV4(t, fixture.submission, forgedEffect, attempt, "guard-other-operation")
	bundle, err := control.BuildOperationSettlementCommitBundleV4(retargeted)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.effect.effect.store.CommitOperationSettlementV4(context.Background(), ports.OperationSettlementCommitRequestV4{ExpectedEffectRevision: current.Revision, Bundle: bundle}); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("same tenant/effect with another operation digest bypassed the shared guard: %v", err)
	}
	inspected, err := fixture.gateway.InspectCurrentOperationSettlementV4(context.Background(), ports.InspectCurrentOperationSettlementRequestV4{
		Operation: fixture.submission.Operation,
		EffectID:  fixture.submission.EffectID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !ports.SameOperationSettlementRefV4(inspected.Settlement, original) || fixture.effect.effect.store.OperationSettlementV4CommitCount() != 1 {
		t.Fatalf("conflicting operation digest changed the existing terminal closure: original=%#v inspected=%#v commits=%d", original, inspected.Settlement, fixture.effect.effect.store.OperationSettlementV4CommitCount())
	}
}

func TestOperationSettlementV4LegacyRaceHasOneTerminalWinner(t *testing.T) {
	legacy := newLegacyOperationSettlementFixtureV3(t, "race")
	base := newOperationSettlementFixtureV4(t, "retarget-race")
	current, err := legacy.effect.store.InspectOperationEffectV3(context.Background(), legacy.effect.intent.Operation, legacy.effect.intent.ID)
	if err != nil {
		t.Fatal(err)
	}
	v4Submission := retargetOperationSettlementSubmissionV4(t, base.submission, current, legacy.submission.Attempt, "v4-race")
	v4Bundle, err := control.BuildOperationSettlementCommitBundleV4(v4Submission)
	if err != nil {
		t.Fatal(err)
	}
	const workers = 64
	var wait sync.WaitGroup
	for index := 0; index < workers; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			if index%2 == 0 {
				_, _ = legacy.gateway.SettleOperationEffectV3(context.Background(), legacy.effect.intent, legacy.submission)
				return
			}
			_, _ = legacy.effect.store.CommitOperationSettlementV4(context.Background(), ports.OperationSettlementCommitRequestV4{ExpectedEffectRevision: current.Revision, Bundle: v4Bundle})
		}(index)
	}
	wait.Wait()
	terminal, err := legacy.effect.store.InspectOperationEffectV3(context.Background(), legacy.effect.intent.Operation, legacy.effect.intent.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, v4Err := legacy.effect.store.InspectOperationSettlementByEffectV4(context.Background(), legacy.effect.intent.Operation, legacy.effect.intent.ID)
	v3Won := terminal.State == control.OperationEffectSettledV3 && terminal.Settlement != nil
	v4Won := v4Err == nil
	if v3Won == v4Won {
		t.Fatalf("V3/V4 race did not produce exactly one terminal winner: v3=%v v4=%v state=%s v4err=%v", v3Won, v4Won, terminal.State, v4Err)
	}
	if v3Won {
		if !core.HasCategory(v4Err, core.ErrorNotFound) || legacy.effect.store.OperationSettlementV4CommitCount() != 0 {
			t.Fatalf("V3 winner left a V4 sidecar: inspect=%v commits=%d", v4Err, legacy.effect.store.OperationSettlementV4CommitCount())
		}
		return
	}
	if terminal.Settlement != nil || legacy.effect.store.OperationSettlementV4CommitCount() != 1 {
		t.Fatalf("V4 winner left a V3 sidecar: settlement=%#v commits=%d", terminal.Settlement, legacy.effect.store.OperationSettlementV4CommitCount())
	}
}

func TestOperationSettlementV4CrossTenantSameEffectIDDoesNotConflict(t *testing.T) {
	left := newOperationSettlementFixtureV4(t, "tenant-left")
	leftCurrent, err := left.gateway.SettleOperationV4(context.Background(), left.submission)
	if err != nil {
		t.Fatal(err)
	}
	rightEffect := newActivationOperationEnforcementFixtureForTenantV4(t, "settlement-tenant-right", "tenant-enforcement-right")
	// Build the right submission through the normal fixture, then copy only its
	// already-valid non-terminal Effect into the same reference Owner instance.
	right := newOperationSettlementFixtureFromEnforcementV4(t, rightEffect, "tenant-right")
	rightCurrent, err := right.effect.effect.store.InspectOperationEffectV3(context.Background(), right.submission.Operation, right.submission.EffectID)
	if err != nil {
		t.Fatal(err)
	}
	if left.submission.EffectID != right.submission.EffectID {
		t.Fatalf("test did not reuse the same Effect ID: left=%s right=%s", left.submission.EffectID, right.submission.EffectID)
	}
	if left.submission.TenantID == right.submission.TenantID {
		t.Fatalf("test did not use distinct tenants: %s", left.submission.TenantID)
	}
	if err := left.effect.effect.store.InstallOperationEffectFactForTestV3(rightCurrent); err != nil {
		t.Fatal(err)
	}
	bundle, err := control.BuildOperationSettlementCommitBundleV4(right.submission)
	if err != nil {
		t.Fatal(err)
	}
	rightCommitted, err := left.effect.effect.store.CommitOperationSettlementV4(context.Background(), ports.OperationSettlementCommitRequestV4{ExpectedEffectRevision: rightCurrent.Revision, Bundle: bundle})
	if err != nil {
		t.Fatalf("cross-tenant equal Effect ID conflicted: %v", err)
	}
	if left.effect.effect.store.OperationSettlementV4CommitCount() != 2 {
		t.Fatalf("cross-tenant settlements did not commit independently: %d", left.effect.effect.store.OperationSettlementV4CommitCount())
	}
	leftInspected, err := left.gateway.InspectCurrentOperationSettlementV4(context.Background(), ports.InspectCurrentOperationSettlementRequestV4{
		Operation: left.submission.Operation,
		EffectID:  left.submission.EffectID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if leftInspected.Projection.TenantID != left.submission.TenantID || rightCommitted.Projection.TenantID != right.submission.TenantID || ports.SameOperationSettlementTerminalProjectionRefV4(leftInspected.Projection, rightCommitted.Projection.RefV4()) || !ports.SameOperationSettlementRefV4(leftInspected.Settlement, leftCurrent) {
		t.Fatalf("cross-tenant terminal projections were not independently partitioned: left=%#v right=%#v", leftInspected.Projection, rightCommitted.Projection.RefV4())
	}
}

type operationSettlementEvidenceReaderV4 struct {
	err            error
	qualifications map[string]ports.OperationScopeEvidenceQualificationFactV3
	handoffs       map[string]ports.OperationScopeEvidenceProviderHandoffFactV3
	consumptions   map[string]ports.OperationScopeEvidenceConsumptionFactV3
	records        map[ports.OperationScopeEvidenceRecordRefV3]ports.OperationScopeEvidenceRecordV3
}

func (r *operationSettlementEvidenceReaderV4) InspectOperationScopeEvidenceQualificationV3(_ context.Context, id string) (ports.OperationScopeEvidenceQualificationFactV3, error) {
	if r.err != nil {
		return ports.OperationScopeEvidenceQualificationFactV3{}, r.err
	}
	fact, ok := r.qualifications[id]
	if !ok {
		return ports.OperationScopeEvidenceQualificationFactV3{}, core.NewError(core.ErrorNotFound, core.ReasonEvidenceUnavailable, "qualification not found")
	}
	return fact, nil
}

func (r *operationSettlementEvidenceReaderV4) InspectOperationScopeEvidenceProviderHandoffV3(_ context.Context, id string) (ports.OperationScopeEvidenceProviderHandoffFactV3, error) {
	if r.err != nil {
		return ports.OperationScopeEvidenceProviderHandoffFactV3{}, r.err
	}
	fact, ok := r.handoffs[id]
	if !ok {
		return ports.OperationScopeEvidenceProviderHandoffFactV3{}, core.NewError(core.ErrorNotFound, core.ReasonEvidenceUnavailable, "handoff not found")
	}
	return fact, nil
}

func (r *operationSettlementEvidenceReaderV4) InspectOperationScopeEvidenceConsumptionV3(_ context.Context, id string) (ports.OperationScopeEvidenceConsumptionFactV3, error) {
	if r.err != nil {
		return ports.OperationScopeEvidenceConsumptionFactV3{}, r.err
	}
	fact, ok := r.consumptions[id]
	if !ok {
		return ports.OperationScopeEvidenceConsumptionFactV3{}, core.NewError(core.ErrorNotFound, core.ReasonEvidenceUnavailable, "consumption not found")
	}
	return fact, nil
}

func (r *operationSettlementEvidenceReaderV4) InspectOperationScopeEvidenceRecordV3(_ context.Context, ref ports.OperationScopeEvidenceRecordRefV3) (ports.OperationScopeEvidenceRecordV3, error) {
	if r.err != nil {
		return ports.OperationScopeEvidenceRecordV3{}, r.err
	}
	record, ok := r.records[ref]
	if !ok {
		return ports.OperationScopeEvidenceRecordV3{}, core.NewError(core.ErrorNotFound, core.ReasonEvidenceUnavailable, "record not found")
	}
	return record, nil
}

type operationSettlementDomainReaderV4 struct {
	mu    sync.Mutex
	value ports.OperationSettlementDomainResultCurrentV4
	err   error
	calls int
}

func (r *operationSettlementDomainReaderV4) InspectOperationSettlementDomainResultCurrentV4(_ context.Context, _ ports.EffectKindV2, _ ports.OperationSettlementDomainResultFactRefV4) (ports.OperationSettlementDomainResultCurrentV4, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	return r.value, r.err
}

type operationSettlementFixtureV4 struct {
	t             *testing.T
	now           time.Time
	effect        *operationEnforcementFixtureV4
	evidence      *operationSettlementEvidenceReaderV4
	domain        *operationSettlementDomainReaderV4
	gateway       kernel.OperationSettlementGatewayV4
	submission    ports.OperationSettlementSubmissionV4
	providerCalls atomic.Int64
}

type legacyOperationSettlementFixtureV3 struct {
	effect     *operationFixtureV3
	gateway    control.OperationSettlementGovernanceGatewayV3
	submission ports.OperationSettlementSubmissionV3
}

func newLegacyOperationSettlementFixtureV3(t *testing.T, suffix string) legacyOperationSettlementFixtureV3 {
	t.Helper()
	ctx := context.Background()
	effect := newOperationFixtureForRunV3(t, core.AgentRunID("run-legacy-settlement-"+suffix), nil)
	declared := beginAndDeclareOperationV3(t, effect, "legacy-settlement-"+suffix)
	provider := newGovernedProviderV2(effect, declared.dispatch)
	preparation, err := provider.Prepare(ctx, ports.PrepareGovernedExecutionRequestV2{Delegation: declared.declaredRef, Intent: effect.intent, Permit: declared.begun.Permit, Fence: declared.begun.Fence})
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := declared.delegationGateway.CommitPreparedExecutionV2(ctx, ports.CommitPreparedExecutionRequestV2{Declared: declared.declaredRef, Intent: effect.intent, Permit: declared.begun.Permit, Fence: declared.begun.Fence, Preparation: preparation})
	if err != nil {
		t.Fatal(err)
	}
	observation, err := provider.ExecutePrepared(ctx, ports.ExecutePreparedRequestV2{Delegation: prepared.Delegation, Prepared: prepared.Prepared, Enforcement: prepared.Enforcement, Intent: effect.intent, Permit: declared.begun.Permit, Fence: declared.begun.Fence})
	if err != nil {
		t.Fatal(err)
	}
	observationRecord := operationEvidenceRecordV3(t, effect, observation, prepared.Delegation.ID, 1, ports.EvidenceTrustObservation, "custom/provider-observation")
	observation.Evidence = observationRecord.Ref
	evidence := operationEvidenceReaderV3{
		bySource: map[ports.EvidenceSourceKeyV2]ports.EvidenceLedgerRecordV2{{RegistrationID: observation.SourceRegistrationID, SourceEpoch: observation.SourceEpoch, SourceSequence: observation.SourceSequence}: observationRecord},
		byRef:    map[ports.EvidenceRecordRefV2]ports.EvidenceLedgerRecordV2{observationRecord.Ref: observationRecord},
	}
	operationDigest, _ := effect.intent.Operation.DigestV3()
	intentDigest, _ := effect.intent.DigestV3()
	attempts := ports.GovernedExecutionAttemptRefsV2{
		Admission: ports.OperationEffectAdmissionReceiptV3{OperationDigest: operationDigest, EffectID: effect.intent.ID, IntentRevision: effect.intent.Revision, IntentDigest: intentDigest, FactRevision: effect.accepted.Revision, State: "accepted"},
		PermitID:  declared.begun.Permit.ID, PermitRevision: declared.begun.Permit.Revision, PermitDigest: declared.begun.PermitDigest, AttemptID: declared.begun.Permit.AttemptID,
		Delegation: prepared.Delegation, Prepared: prepared.Prepared, Enforcement: prepared.Enforcement,
	}
	observationGateway := control.OperationObservationGovernanceGatewayV3{Effects: effect.store, Observations: fakes.NewProviderAttemptObservationStoreV2(), Delegations: effect.delegations, Current: effect.current, Dispatch: declared.dispatch, Evidence: evidence, Clock: func() time.Time { return effect.now }}
	observationRef, err := observationGateway.RecordGovernedProviderObservationV3(ctx, ports.RecordGovernedProviderObservationRequestV2{Intent: effect.intent, Permit: declared.begun.Permit, Fence: declared.begun.Fence, Attempt: attempts, Observation: observation})
	if err != nil {
		t.Fatal(err)
	}
	settlementRecord := operationEvidenceRecordV3(t, effect, observation, observation.ProviderOperationRef, 2, ports.EvidenceTrustAttestation, "custom/provider-settlement")
	evidence.byRef[settlementRecord.Ref] = settlementRecord
	owner, _ := exactOwnerForTestV3(effect.intent.Owners, ports.OwnerSettlement)
	dispatchAttempt := ports.OperationDispatchAttemptRefV3{OperationDigest: operationDigest, EffectID: effect.intent.ID, IntentRevision: effect.intent.Revision, IntentDigest: intentDigest, PermitID: declared.begun.Permit.ID, PermitRevision: declared.begun.Permit.Revision, PermitDigest: declared.begun.PermitDigest, AttemptID: declared.begun.Permit.AttemptID, Delegation: &prepared.Delegation}
	gateway := control.OperationSettlementGovernanceGatewayV3{Effects: effect.store, Evidence: evidence, Clock: func() time.Time { return effect.now }}
	submission := ports.OperationSettlementSubmissionV3{ID: "legacy-settlement-" + suffix, Revision: 1, Attempt: dispatchAttempt, Owner: owner, Disposition: ports.OperationSettlementAppliedV3, Observation: &observationRef, Evidence: []ports.EvidenceRecordRefV2{settlementRecord.Ref}, SettledUnixNano: effect.now.UnixNano()}
	if err := submission.Validate(); err != nil {
		t.Fatal(err)
	}
	return legacyOperationSettlementFixtureV3{effect: effect, gateway: gateway, submission: submission}
}

func newOperationSettlementFixtureV4(t *testing.T, suffix string) *operationSettlementFixtureV4 {
	t.Helper()
	return newOperationSettlementFixtureFromEnforcementV4(t, newActivationOperationEnforcementFixtureForEffectKindV4(t, "settlement-"+suffix, "praxis.sandbox/allocate"), suffix)
}

func newOperationSettlementFixtureFromEnforcementV4(t *testing.T, effect *operationEnforcementFixtureV4, suffix string) *operationSettlementFixtureV4 {
	t.Helper()
	prepared, err := effect.enforcement.EnforceCurrentOperationDispatchV4(context.Background(), effect.prepare)
	if err != nil {
		t.Fatal(err)
	}
	execute := effect.prepare
	execute.Phase = ports.OperationDispatchEnforcementExecuteV4
	execute.ExpectedJournalRevision = 1
	execute.Prepare = &prepared.Phase
	execute.PreparedAttempt = preparedAttemptForEnforcementV4(t, effect, prepared)
	executed, err := effect.enforcement.EnforceCurrentOperationDispatchV4(context.Background(), execute)
	if err != nil {
		t.Fatal(err)
	}
	currentEffect, err := effect.effect.store.InspectOperationEffectV3(context.Background(), effect.effect.intent.Operation, effect.effect.intent.ID)
	if err != nil {
		t.Fatal(err)
	}
	unknown := currentEffect
	unknown.State = control.OperationEffectUnknownOutcomeV3
	unknown.Revision++
	unknown.UpdatedUnixNano = effect.effect.now.UnixNano()
	currentEffect, err = effect.effect.store.CompareAndSwapOperationEffectV3(context.Background(), effect.effect.intent.Operation, control.OperationEffectCASRequestV3{ExpectedRevision: currentEffect.Revision, Next: unknown})
	if err != nil {
		t.Fatal(err)
	}
	legacy := executed.Dispatch.Record.Permit.LegacyPermit
	legacyDigest, err := legacy.DigestV3()
	if err != nil {
		t.Fatal(err)
	}
	operationDigest, _ := effect.effect.intent.Operation.DigestV3()
	attempt := ports.OperationDispatchAttemptRefV3{OperationDigest: operationDigest, EffectID: effect.effect.intent.ID, IntentRevision: effect.effect.intent.Revision, IntentDigest: currentEffect.IntentDigest, PermitID: legacy.ID, PermitRevision: legacy.Revision, PermitDigest: legacyDigest, AttemptID: legacy.AttemptID}
	evidence := &operationSettlementEvidenceReaderV4{qualifications: map[string]ports.OperationScopeEvidenceQualificationFactV3{}, handoffs: map[string]ports.OperationScopeEvidenceProviderHandoffFactV3{}, consumptions: map[string]ports.OperationScopeEvidenceConsumptionFactV3{}, records: map[ports.OperationScopeEvidenceRecordRefV3]ports.OperationScopeEvidenceRecordV3{}}
	prepare := buildOperationSettlementEvidenceBindingV4(t, evidence, effect, attempt, executed.Journal, ports.OperationDispatchEnforcementPrepareV4, suffix, 1)
	executeBinding := buildOperationSettlementEvidenceBindingV4(t, evidence, effect, attempt, executed.Journal, ports.OperationDispatchEnforcementExecuteV4, suffix, 2)
	owner := operationSettlementOwnerV4(t, effect.effect.intent)
	domainFact := ports.OperationSettlementDomainResultFactRefV4{
		Owner: effect.effect.intent.Provider, Kind: "praxis.sandbox/domain-result", ID: "domain-result-" + suffix, Revision: 1, Digest: digestV3("domain-result-fact-" + suffix),
		TenantID: effect.effect.intent.Operation.ExecutionScope.Identity.TenantID, EffectID: effect.effect.intent.ID, EffectRevision: effect.effect.intent.Revision,
		Operation: effect.effect.intent.Operation, OperationDigest: operationDigest, Attempt: attempt,
		Schema: effect.effect.intent.Payload.Schema, PayloadDigest: digestV3("domain-result-payload-" + suffix), PayloadRevision: 1, AuthoritativeTime: effect.effect.now.Add(-time.Nanosecond).UnixNano(),
	}
	domainProjection, err := ports.SealOperationSettlementDomainResultCurrentV4(ports.OperationSettlementDomainResultCurrentV4{EffectKind: effect.effect.intent.Kind, Fact: domainFact, CheckedUnixNano: effect.effect.now.UnixNano(), ExpiresUnixNano: effect.effect.now.Add(30 * time.Second).UnixNano()}, effect.effect.now)
	if err != nil {
		t.Fatal(err)
	}
	domain := &operationSettlementDomainReaderV4{value: domainProjection}
	scopeSetDigest, err := ports.DigestOperationSettlementScopeSetV4([]ports.OperationSettlementEvidenceBindingV4{executeBinding, prepare})
	if err != nil {
		t.Fatal(err)
	}
	submission, err := ports.SealOperationSettlementSubmissionV4(ports.OperationSettlementSubmissionV4{
		ID: "operation-settlement-" + suffix, TenantID: domainFact.TenantID, Operation: effect.effect.intent.Operation, OperationDigest: operationDigest, OperationScopeDigest: scopeSetDigest,
		EffectID: effect.effect.intent.ID, ExpectedEffectRevision: currentEffect.Revision, Owner: owner, DomainResult: domainFact,
		Evidence: []ports.OperationSettlementEvidenceBindingV4{executeBinding, prepare}, IdempotencyKey: "settlement-idempotency-" + suffix,
		ConflictDomain: digestV3("settlement-conflict-" + suffix), SettledUnixNano: effect.effect.now.UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	fixture := &operationSettlementFixtureV4{t: t, now: effect.effect.now, effect: effect, evidence: evidence, domain: domain, submission: submission}
	fixture.gateway = kernel.OperationSettlementGatewayV4{Facts: effect.effect.store, Effects: effect.effect.store, Evidence: evidence, Enforcement: effect.effect.store, Domain: domain, Clock: func() time.Time { return fixture.now }}
	return fixture
}

func buildOperationSettlementEvidenceBindingV4(t *testing.T, reader *operationSettlementEvidenceReaderV4, fixture *operationEnforcementFixtureV4, attempt ports.OperationDispatchAttemptRefV3, journal ports.OperationDispatchEnforcementJournalV4, phase ports.OperationDispatchEnforcementPhaseV4, suffix string, sequence uint64) ports.OperationSettlementEvidenceBindingV4 {
	t.Helper()
	phaseRef, err := journal.PhaseRefV4(phase)
	if err != nil {
		t.Fatal(err)
	}
	operation := fixture.effect.intent.Operation
	operationDigest, _ := operation.DigestV3()
	applicability := ports.NormalizeOperationScopeEvidenceApplicabilityV3([]ports.OperationScopeEvidenceApplicabilityV3{
		{Dimension: ports.OperationScopeEvidenceRunV3, Mode: ports.OperationScopeEvidenceForbiddenV3},
		{Dimension: ports.OperationScopeEvidenceSessionV3, Mode: ports.OperationScopeEvidenceForbiddenV3},
		{Dimension: ports.OperationScopeEvidenceTurnV3, Mode: ports.OperationScopeEvidenceForbiddenV3},
		{Dimension: ports.OperationScopeEvidenceActionV3, Mode: ports.OperationScopeEvidenceForbiddenV3},
		{Dimension: ports.OperationScopeEvidenceContextV3, Mode: ports.OperationScopeEvidenceForbiddenV3},
	})
	expires := fixture.effect.now.Add(6 * time.Second).UnixNano()
	appPolicy := ports.OperationScopeEvidenceApplicabilityPolicyRefV3{ID: "app-policy-" + string(phase) + "-" + suffix, Revision: 1, Digest: digestV3("app-policy-" + string(phase) + "-" + suffix), ExpiresUnixNano: expires}
	scope := ports.OperationScopeEvidenceScopeV3{
		LedgerScope: ports.OperationScopeEvidenceLedgerScopeV3{TenantID: operation.ExecutionScope.Identity.TenantID, OperationDigest: operationDigest, ChainID: "settlement-chain-" + string(phase) + "-" + suffix},
		Operation:   operation, OperationDigest: operationDigest, EffectID: fixture.effect.intent.ID, EffectRevision: fixture.effect.intent.Revision, EffectDigest: attempt.IntentDigest,
		EffectKind: fixture.effect.intent.Kind, AttemptID: attempt.AttemptID, Phase: phase, ApplicabilityPolicy: appPolicy, Applicability: applicability,
		Generation: ports.GenerationBindingAssociationRefV1{ID: "generation-" + suffix, Revision: 1, Digest: digestV3("generation-" + suffix)},
	}
	runtimeCurrent, err := ports.SealOperationScopeEvidenceRuntimeCurrentProjectionV3(ports.OperationScopeEvidenceRuntimeCurrentProjectionV3{
		Scope: scope, PermitID: phaseRef.PermitID, PermitFactRevision: phaseRef.PermitFactRevision, PermitDigest: phaseRef.PermitDigest,
		AdmissionDigest: phaseRef.AdmissionDigest, Authorization: phaseRef.ReviewAuthorization, Phase: phaseRef,
		CheckedUnixNano: fixture.effect.now.UnixNano(), ExpiresUnixNano: expires,
	}, fixture.effect.now)
	if err != nil {
		t.Fatal(err)
	}
	schema := ports.SchemaRefV2{Namespace: "praxis.sandbox", Name: "operation-observation", Version: "1.0.0", MediaType: "application/json", ContentDigest: digestV3("evidence-schema")}
	sourceID := "evidence-source-" + string(phase) + "-" + suffix
	sourceKey := ports.OperationScopeEvidenceSourceKeyV3{RegistrationID: sourceID, SourceEpoch: 1, SourceSequence: sequence}
	reservation := ports.OperationScopeEvidenceSourceReservationV3{
		Registration: ports.OperationScopeEvidenceFactRefV3{ID: sourceID, Revision: 1, Digest: digestV3(sourceID), ExpiresUnixNano: expires},
		Source:       sourceKey, EventID: "event-" + string(phase) + "-" + suffix, Schema: schema,
	}
	qualificationID := "qualification-" + string(phase) + "-" + suffix
	issued, err := ports.SealOperationScopeEvidenceQualificationFactV3(ports.OperationScopeEvidenceQualificationFactV3{
		ID: qualificationID, Revision: 1, State: ports.OperationScopeEvidenceIssuedV3, Scope: scope, Runtime: runtimeCurrent,
		EvidencePolicy: ports.OperationScopeEvidencePolicyRefV3{ID: "evidence-policy-" + string(phase) + "-" + suffix, Revision: 1, Digest: digestV3("evidence-policy-" + string(phase) + "-" + suffix), ExpiresUnixNano: expires},
		Reservation:    reservation, RequestedTTL: 5 * time.Second, CreatedUnixNano: fixture.effect.now.UnixNano(), UpdatedUnixNano: fixture.effect.now.UnixNano(), ExpiresUnixNano: expires, IngestNotAfterUnixNano: time.Unix(0, expires).Add(time.Second).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	handoff, err := ports.SealOperationScopeEvidenceProviderHandoffFactV3(ports.OperationScopeEvidenceProviderHandoffFactV3{ID: "handoff-" + string(phase) + "-" + suffix, Revision: 1, Qualification: issued.RefV3(), Phase: phaseRef, CheckedUnixNano: fixture.effect.now.UnixNano(), NotAfterUnixNano: expires})
	if err != nil {
		t.Fatal(err)
	}
	candidate := ports.OperationScopeEvidenceCandidateV3{
		ContractVersion: ports.OperationScopeEvidenceContractVersionV3, Qualification: issued.RefV3(), Source: sourceKey, EventID: reservation.EventID, TrustClass: ports.EvidenceTrustObservation,
		Payload:   ports.EvidencePayloadRefV2{Schema: schema, ContentDigest: digestV3("evidence-payload-" + string(phase) + "-" + suffix), Revision: 1, Length: 8, Ref: "evidence://" + string(phase) + "/" + suffix},
		Causation: []ports.EvidenceCausationRefV2{}, CorrelationID: "correlation-" + suffix, ObservedUnixNano: fixture.effect.now.UnixNano(),
	}
	ledgerDigest, _ := scope.LedgerScope.DigestV3()
	record, err := ports.SealOperationScopeEvidenceRecordV3(ports.OperationScopeEvidenceRecordV3{Ref: ports.OperationScopeEvidenceRecordRefV3{LedgerScopeDigest: ledgerDigest, Sequence: sequence}, Candidate: candidate, PreviousRecordDigest: ports.EvidenceGenesisDigestV2, IngestedUnixNano: fixture.effect.now.UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	consumption, err := ports.SealOperationScopeEvidenceConsumptionFactV3(ports.OperationScopeEvidenceConsumptionFactV3{ID: "consumption-" + string(phase) + "-" + suffix, Revision: 1, Qualification: issued.RefV3(), Handoff: handoff.RefV3(), CandidateDigest: record.CandidateDigest, Record: record.Ref, CreatedUnixNano: fixture.effect.now.UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	final := issued
	final.Revision = 2
	final.State = ports.OperationScopeEvidenceConsumedCurrentV3
	final.Consumption = ptrSettlementV4(consumption.RefV3())
	final.Digest = ""
	final, err = ports.SealOperationScopeEvidenceQualificationFactV3(final)
	if err != nil {
		t.Fatal(err)
	}
	reader.qualifications[qualificationID] = final
	reader.handoffs[handoff.ID] = handoff
	reader.consumptions[consumption.ID] = consumption
	reader.records[record.Ref] = record
	scopeDigest, _ := ports.DigestOperationSettlementEvidenceScopeV4(scope)
	return ports.OperationSettlementEvidenceBindingV4{Phase: phase, Consumption: consumption.RefV3(), IssuedQualification: issued.RefV3(), FinalQualification: final.RefV3(), Record: record.Ref, CandidateDigest: record.CandidateDigest, Handoff: handoff.RefV3(), Attempt: attempt, EnforcementPhase: phaseRef, OperationScopeDigest: scopeDigest}
}

func operationSettlementOwnerV4(t *testing.T, intent ports.OperationEffectIntentV3) ports.EffectOwnerRefV2 {
	t.Helper()
	for _, owner := range intent.Owners {
		if owner.Role == ports.OwnerSettlement {
			return owner
		}
	}
	t.Fatal("settlement owner missing from fixture")
	return ports.EffectOwnerRefV2{}
}

func retargetOperationSettlementSubmissionV4(t *testing.T, base ports.OperationSettlementSubmissionV4, effect control.OperationEffectFactV3, attempt ports.OperationDispatchAttemptRefV3, suffix string) ports.OperationSettlementSubmissionV4 {
	t.Helper()
	operationDigest, err := effect.Intent.Operation.DigestV3()
	if err != nil {
		t.Fatal(err)
	}
	value := base
	value.ID = "operation-settlement-" + suffix
	value.TenantID = effect.Intent.Operation.ExecutionScope.Identity.TenantID
	value.Operation = effect.Intent.Operation
	value.OperationDigest = operationDigest
	value.EffectID = effect.Intent.ID
	value.ExpectedEffectRevision = effect.Revision
	value.Owner = operationSettlementOwnerV4(t, effect.Intent)
	value.DomainResult.Owner = effect.Intent.Provider
	value.DomainResult.ID = "domain-result-" + suffix
	value.DomainResult.Digest = digestV3("domain-result-" + suffix)
	value.DomainResult.TenantID = value.TenantID
	value.DomainResult.EffectID = value.EffectID
	value.DomainResult.EffectRevision = effect.Intent.Revision
	value.DomainResult.Operation = value.Operation
	value.DomainResult.OperationDigest = operationDigest
	value.DomainResult.Attempt = attempt
	value.DomainResult.Schema = effect.Intent.Payload.Schema
	value.DomainResult.PayloadDigest = digestV3("domain-result-payload-" + suffix)
	value.IdempotencyKey = "settlement-idempotency-" + suffix
	value.ConflictDomain = digestV3("settlement-conflict-" + suffix)
	value.Evidence = append([]ports.OperationSettlementEvidenceBindingV4{}, value.Evidence...)
	for index := range value.Evidence {
		value.Evidence[index].Attempt = attempt
		value.Evidence[index].EnforcementPhase.OperationDigest = operationDigest
		value.Evidence[index].EnforcementPhase.EffectID = attempt.EffectID
		value.Evidence[index].EnforcementPhase.PermitID = attempt.PermitID
		value.Evidence[index].EnforcementPhase.AttemptID = attempt.AttemptID
		value.Evidence[index].EnforcementPhase.SandboxAttempt.ID = attempt.AttemptID
	}
	value.OperationScopeDigest, err = ports.DigestOperationSettlementScopeSetV4(value.Evidence)
	if err != nil {
		t.Fatal(err)
	}
	value.Digest = ""
	value, err = ports.SealOperationSettlementSubmissionV4(value)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func legacySettlementFactForV4Guard(t *testing.T, fixture *operationSettlementFixtureV4) *control.OperationSettlementFactV3 {
	t.Helper()
	evidence := []ports.EvidenceRecordRefV2{{LedgerScopeDigest: digestV3("legacy-ledger"), Sequence: 1, RecordDigest: digestV3("legacy-record")}}
	evidenceDigest, err := core.CanonicalJSONDigest("praxis.runtime.operation-effect", ports.OperationEffectContractVersionV3, "OperationSettlementEvidenceV3", evidence)
	if err != nil {
		t.Fatal(err)
	}
	fact := &control.OperationSettlementFactV3{
		ID: "legacy-after-v4", Revision: 1, Owner: operationSettlementOwnerV4(t, fixture.effect.effect.intent), Attempt: fixture.submission.DomainResult.Attempt,
		Disposition: control.SettlementConfirmedApplied, Evidence: evidence, EvidenceDigest: evidenceDigest, SettledUnixNano: fixture.now.UnixNano(),
	}
	if err := fact.Validate(); err != nil {
		t.Fatal(err)
	}
	return fact
}

func (f *operationSettlementFixtureV4) resealSubmissionIgnoringError() {
	f.submission.Digest = ""
	if sealed, err := ports.SealOperationSettlementSubmissionV4(f.submission); err == nil {
		f.submission = sealed
	}
}

func assertNoOperationSettlementV4Write(t *testing.T, fixture *operationSettlementFixtureV4) {
	t.Helper()
	if fixture.providerCalls.Load() != 0 {
		t.Fatalf("rejected V4 settlement called Provider: %d", fixture.providerCalls.Load())
	}
	if fixture.effect.effect.store.OperationSettlementV4CommitCount() != 0 {
		t.Fatal("rejected V4 settlement changed the terminal Owner")
	}
	if _, err := fixture.effect.effect.store.InspectOperationSettlementV4(context.Background(), fixture.submission.Operation, fixture.submission.ID); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("rejected V4 settlement left a sidecar: %v", err)
	}
	if _, err := fixture.effect.effect.store.InspectOperationSettlementByEffectV4(context.Background(), fixture.submission.Operation, fixture.submission.EffectID); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("rejected V4 settlement left a terminal guard: %v", err)
	}
	bundle, err := control.BuildOperationSettlementCommitBundleV4(fixture.submission)
	if err != nil {
		return
	}
	if _, err := fixture.effect.effect.store.InspectOperationSettlementEvidenceAssociationV4(context.Background(), fixture.submission.Operation, bundle.Association.RefV4()); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("rejected V4 settlement left an association: %v", err)
	}
	if _, err := fixture.effect.effect.store.InspectOperationSettlementTerminalGuardV4(context.Background(), fixture.submission.Operation, bundle.Guard.RefV4()); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("rejected V4 settlement left a guard ref: %v", err)
	}
	if _, err := fixture.effect.effect.store.InspectOperationSettlementTerminalProjectionV4(context.Background(), fixture.submission.Operation, bundle.Projection.RefV4()); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("rejected V4 settlement left a projection: %v", err)
	}
}

func ptrSettlementV4[T any](value T) *T { return &value }
