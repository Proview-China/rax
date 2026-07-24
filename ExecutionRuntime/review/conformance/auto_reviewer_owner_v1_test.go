package conformance_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/autoreviewer"
	"github.com/Proview-China/rax/ExecutionRuntime/review/caseengine"
	"github.com/Proview-China/rax/ExecutionRuntime/review/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/storetestkit"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/review/memory"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/review/reviewer"
	reviewsqlite "github.com/Proview-China/rax/ExecutionRuntime/review/storage/sqlite"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type conformanceOwnerStoreV1 interface {
	reviewport.StoreV1
	reviewport.AutoReviewerStoreV1
}

type conformanceClockV1 struct {
	mu    sync.Mutex
	value time.Time
}

func (c *conformanceClockV1) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	value := c.value
	c.value = c.value.Add(time.Nanosecond)
	return value
}

func (c *conformanceClockV1) Set(value time.Time) {
	c.mu.Lock()
	c.value = value
	c.mu.Unlock()
}

type conformanceInvocationV1 struct {
	mu            sync.Mutex
	observedAt    time.Time
	tokens        uint64
	costMicros    uint64
	startErr      error
	inspectErr    error
	startMayCall  bool
	onStart       func()
	onObservation func()
	onInspect     func()
	startCalls    int32
	inspectCalls  int32
	providerCalls int32
	sealed        map[contract.ExactResourceRefV1]reviewport.AutoReviewerInvocationResultV1
	inspectRefs   []contract.ExactResourceRefV1
}

func (f *conformanceInvocationV1) StartOrInspectAutoReviewerInvocationV1(_ context.Context, command reviewport.AutoReviewerInvocationCommandV1) (reviewport.AutoReviewerInvocationResultV1, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.startCalls++
	if f.onStart != nil {
		f.onStart()
	}
	if f.startErr != nil {
		if f.startMayCall && f.providerCalls == 0 {
			f.providerCalls++
			f.sealed[command.Attempt.ExactRef()] = conformanceInvocationResultV1(conformanceObservationV1(command.Attempt, f.observedAt, f.tokens, f.costMicros))
			if f.onObservation != nil {
				f.onObservation()
			}
		}
		return reviewport.AutoReviewerInvocationResultV1{}, f.startErr
	}
	if observation, ok := f.sealed[command.Attempt.ExactRef()]; ok {
		return observation, nil
	}
	f.providerCalls++
	observation := conformanceInvocationResultV1(conformanceObservationV1(command.Attempt, f.observedAt, f.tokens, f.costMicros))
	f.sealed[command.Attempt.ExactRef()] = observation
	if f.onObservation != nil {
		f.onObservation()
	}
	return observation, nil
}

func (f *conformanceInvocationV1) InspectAutoReviewerInvocationV1(ctx context.Context, ref contract.ExactResourceRefV1) (reviewport.AutoReviewerInvocationResultV1, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.inspectCalls++
	f.inspectRefs = append(f.inspectRefs, ref)
	if f.inspectErr != nil {
		return reviewport.AutoReviewerInvocationResultV1{}, f.inspectErr
	}
	if err := ctx.Err(); err != nil {
		return reviewport.AutoReviewerInvocationResultV1{}, err
	}
	observation, ok := f.sealed[ref]
	if !ok {
		return reviewport.AutoReviewerInvocationResultV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "conformance invocation has no exact Observation")
	}
	if f.onInspect != nil {
		f.onInspect()
	}
	return observation, nil
}

func (f *conformanceInvocationV1) stats() (int32, int32, int32, []contract.ExactResourceRefV1) {
	f.mu.Lock()
	defer f.mu.Unlock()
	refs := append([]contract.ExactResourceRefV1(nil), f.inspectRefs...)
	return f.startCalls, f.inspectCalls, f.providerCalls, refs
}

type conformanceStoreFaultV1 struct {
	conformanceOwnerStoreV1
	beginCalls           atomic.Int32
	markCalls            atomic.Int32
	recordCalls          atomic.Int32
	rubricReads          atomic.Int32
	loseBegin            bool
	loseMarkBeforeCommit bool
	loseMark             bool
	loseRecord           bool
	driftAtRead          int32
}

func (s *conformanceStoreFaultV1) BeginAutoReviewerAttemptV1(ctx context.Context, mutation reviewport.BeginAutoReviewerAttemptMutationV1) (contract.AutoReviewerAttemptV1, error) {
	s.beginCalls.Add(1)
	value, err := s.conformanceOwnerStoreV1.BeginAutoReviewerAttemptV1(ctx, mutation)
	if err == nil && s.loseBegin && s.beginCalls.Load() == 1 {
		return contract.AutoReviewerAttemptV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "injected Begin reply loss")
	}
	return value, err
}

func (s *conformanceStoreFaultV1) MarkAutoReviewerWaitingInspectV1(ctx context.Context, mutation reviewport.MarkAutoReviewerWaitingInspectMutationV1) (reviewport.AutoReviewerInvocationStartClaimReceiptV1, error) {
	call := s.markCalls.Add(1)
	if s.loseMarkBeforeCommit && call == 1 {
		return reviewport.AutoReviewerInvocationStartClaimReceiptV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "injected pre-commit start-claim uncertainty")
	}
	value, err := s.conformanceOwnerStoreV1.MarkAutoReviewerWaitingInspectV1(ctx, mutation)
	if err == nil && s.loseMark && call == 1 {
		return reviewport.AutoReviewerInvocationStartClaimReceiptV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "injected waiting reply loss")
	}
	return value, err
}

func (s *conformanceStoreFaultV1) RecordAutoReviewerObservationV1(ctx context.Context, mutation reviewport.RecordAutoReviewerObservationMutationV1) (contract.AutoReviewerAttemptV1, contract.ReviewerInvocationResultFactV1, error) {
	s.recordCalls.Add(1)
	attempt, result, err := s.conformanceOwnerStoreV1.RecordAutoReviewerObservationV1(ctx, mutation)
	if err == nil && s.loseRecord && s.recordCalls.Load() == 1 {
		return contract.AutoReviewerAttemptV1{}, contract.ReviewerInvocationResultFactV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "injected Record reply loss")
	}
	return attempt, result, err
}

func (s *conformanceStoreFaultV1) InspectRubricCurrentV1(ctx context.Context, tenant core.TenantID, ref contract.ExactResourceRefV1, now time.Time) (contract.RubricDefinitionV1, error) {
	if read := s.rubricReads.Add(1); s.driftAtRead > 0 && read == s.driftAtRead {
		return contract.RubricDefinitionV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "injected Rubric S2 drift")
	}
	return s.conformanceOwnerStoreV1.InspectRubricCurrentV1(ctx, tenant, ref, now)
}

type conformanceFixtureFactoryV1 struct {
	t       *testing.T
	backend string
}

type conformanceFixtureV1 struct {
	base       time.Time
	store      *conformanceStoreFaultV1
	invocation *conformanceInvocationV1
	clock      *conformanceClockV1
	attempt    contract.AutoReviewerAttemptV1
	frame      reviewer.ContextFrameV1
	command    autoreviewer.RunCommandV1
}

func (f conformanceFixtureFactoryV1) NewAutoReviewerOwnerScenarioV1(ctx context.Context, scenario conformance.AutoReviewerOwnerScenarioV1) (conformance.AutoReviewerOwnerSubjectV1, error) {
	fixture := newConformanceFixtureV1(f.t, ctx, f.backend+"-"+string(scenario), f.backend)
	runContext := ctx
	unknown := core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "injected unknown outcome")
	switch scenario {
	case conformance.AutoReviewerOwnerHappyV1, conformance.AutoReviewerOwnerConcurrent64V1:
	case conformance.AutoReviewerOwnerUnknownRecoveredV1:
		cancelContext, cancel := context.WithCancel(ctx)
		runContext = cancelContext
		fixture.invocation.startErr = unknown
		fixture.invocation.startMayCall = true
		fixture.invocation.observedAt = fixture.base.Add(31 * time.Second)
		fixture.invocation.onStart = cancel
		fixture.invocation.onObservation = func() { fixture.clock.Set(fixture.base.Add(32 * time.Second)) }
		fixture.invocation.onInspect = func() { fixture.clock.Set(fixture.base.Add(33 * time.Second)) }
	case conformance.AutoReviewerOwnerUnknownPersistentV1:
		fixture.invocation.startErr = unknown
		fixture.invocation.inspectErr = unknown
		fixture.invocation.startMayCall = true
	case conformance.AutoReviewerOwnerBeginLostReplyV1:
		fixture.store.loseBegin = true
	case conformance.AutoReviewerOwnerMarkPrecommitUnknownV1:
		fixture.store.loseMarkBeforeCommit = true
	case conformance.AutoReviewerOwnerMarkLostReplyV1:
		fixture.store.loseMark = true
		fixture.invocation.startErr = unknown
		fixture.invocation.startMayCall = true
		fixture.invocation.observedAt = fixture.base.Add(31 * time.Second)
		fixture.invocation.onObservation = func() { fixture.clock.Set(fixture.base.Add(32 * time.Second)) }
		fixture.invocation.onInspect = func() { fixture.clock.Set(fixture.base.Add(33 * time.Second)) }
	case conformance.AutoReviewerOwnerRecordLostReplyV1:
		fixture.store.loseRecord = true
	case conformance.AutoReviewerOwnerKnownFailureV1:
		fixture.invocation.startErr = core.NewError(core.ErrorForbidden, core.ReasonUnknownCapability, "injected authoritative rejection")
	case conformance.AutoReviewerOwnerBudgetExceededV1:
		fixture.invocation.tokens = 100_000
	case conformance.AutoReviewerOwnerTTLCrossingV1:
		fixture.invocation.onObservation = func() { fixture.clock.Set(time.Unix(0, fixture.attempt.ExpiresUnixNano)) }
	case conformance.AutoReviewerOwnerClockRollbackV1:
		fixture.invocation.onObservation = func() { fixture.clock.Set(fixture.base.Add(25 * time.Second)) }
	case conformance.AutoReviewerOwnerS2DriftV1:
		fixture.store.driftAtRead = 4
	default:
		return conformance.AutoReviewerOwnerSubjectV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "unsupported Auto Reviewer Owner conformance scenario")
	}
	owner, err := autoreviewer.New(fixture.store, fixture.invocation, fixture.clock.Now)
	if err != nil {
		return conformance.AutoReviewerOwnerSubjectV1{}, err
	}
	observation := conformanceObservationV1(fixture.attempt, fixture.invocation.observedAt, fixture.invocation.tokens, fixture.invocation.costMicros)
	expectedResult := conformanceResultV1(fixture.command, observation)
	return conformance.AutoReviewerOwnerSubjectV1{
		Owner: owner, Store: fixture.store, Command: fixture.command, RunContext: runContext,
		OriginalInvocation: fixture.attempt.ExactRef(), ExpectedResult: expectedResult.ExactRef(),
		Stats: func() conformance.AutoReviewerOwnerStatsV1 {
			start, inspect, provider, refs := fixture.invocation.stats()
			return conformance.AutoReviewerOwnerStatsV1{BeginCalls: fixture.store.beginCalls.Load(), MarkCalls: fixture.store.markCalls.Load(), RecordCalls: fixture.store.recordCalls.Load(), StartCalls: start, InspectCalls: inspect, ProviderCalls: provider, InspectRefs: refs}
		},
	}, nil
}

func (f conformanceFixtureFactoryV1) NewAutoReviewerOwnerConstructorFixtureV1(ctx context.Context) (conformance.AutoReviewerOwnerConstructorFixtureV1, error) {
	fixture := newConformanceFixtureV1(f.t, ctx, f.backend+"-constructor", f.backend)
	var typedNilStore *memory.Store
	var typedNilInvocation *conformanceInvocationV1
	return conformance.AutoReviewerOwnerConstructorFixtureV1{ValidStore: fixture.store, ValidInvocation: fixture.invocation, TypedNilStore: typedNilStore, TypedNilInvocation: typedNilInvocation, Clock: fixture.clock.Now}, nil
}

func newConformanceFixtureV1(t testing.TB, ctx context.Context, suffix, backend string) conformanceFixtureV1 {
	t.Helper()
	base := time.Unix(1_904_000_000, 0)
	var baseStore conformanceOwnerStoreV1 = storetestkit.NewMemoryStoreV1(func() time.Time { return base.Add(30 * time.Second) })
	if backend == "sqlite" {
		store, err := reviewsqlite.Open(ctx, reviewsqlite.Config{Path: filepath.Join(t.TempDir(), suffix+".sqlite"), Clock: func() time.Time { return base.Add(30 * time.Second) }})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = store.Close() })
		baseStore = store
	}
	rubric := testkit.Rubric(base, "tenant-a")
	if _, err := baseStore.PublishRubricV1(ctx, reviewport.PublishRubricMutationV1{Next: rubric}); err != nil {
		t.Fatal(err)
	}
	target := testkit.Target(base)
	caseClock := testkit.NewClock(base)
	engine, err := caseengine.New(baseStore, caseClock.Now)
	if err != nil {
		t.Fatal(err)
	}
	caseID := "case-conformance-" + suffix
	request := testkit.Request(base, target, caseID)
	caseClock.Advance(time.Second)
	caseValue, err := engine.CreateCaseV1(ctx, caseengine.CreateCaseCommandV1{CaseID: caseID, Request: &request, Target: target, ExpiresUnixNano: request.ExpiresUnixNano, Trace: testkit.TraceForTarget(caseClock.Now(), caseID, target, contract.TraceRequestedV1, 1, request.ID)})
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range []contract.CaseStateV1{contract.CaseAdmittedV1, contract.CaseRoutedV1} {
		caseClock.Advance(time.Second)
		caseValue, err = engine.TransitionWithTraceV2(ctx, caseengine.TransitionWithTraceCommandV2{TransitionCommandV1: caseengine.TransitionCommandV1{TenantID: caseValue.TenantID, CaseID: caseValue.ID, Expected: reviewport.ExpectedV1(caseValue.Revision, caseValue.Digest), Next: state}, Trace: testkit.TransitionTrace(caseClock.Now(), caseValue, state)})
		if err != nil {
			t.Fatal(err)
		}
	}
	roundID, assignmentID := "round-conformance-"+suffix, "assignment-conformance-"+suffix
	schemaDocument, err := contract.BuiltinAutoReviewerOutputSchemaDocumentV1()
	if err != nil {
		t.Fatal(err)
	}
	resultSchema := schemaDocument.Schema
	capabilities := make([]string, len(rubric.AllowedReadOnlyCapabilities))
	for index, capability := range rubric.AllowedReadOnlyCapabilities {
		capabilities[index] = string(capability)
	}
	frame, err := reviewer.SealContextFrameV1(reviewer.ContextFrameV1{
		TenantID: target.TenantID, CaseID: caseID, RoundID: roundID, TargetDigest: target.Digest,
		OriginalIntentDigest: testkit.Digest("conformance-intent-" + suffix), StableRulesDigest: testkit.Digest("conformance-rules-" + suffix), ConfirmedDecisionsDigest: testkit.Digest("conformance-decisions-" + suffix), EvidenceSetDigest: target.EvidenceSetDigest,
		RubricDigest: rubric.Digest, OutputSchemaDigest: resultSchema.ContentDigest, AllowedReadCapabilities: capabilities, ReadOnly: true,
		CreatedUnixNano: base.Add(3 * time.Second).UnixNano(), ExpiresUnixNano: base.Add(12 * time.Minute).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	caseClock.Advance(time.Second)
	round := testkit.Round(caseClock.Now(), caseValue, contract.RouteAutoV1)
	round.ID, round.AssignmentID, round.ContextFrameDigest, round.RubricDigest = roundID, assignmentID, frame.Digest, rubric.Digest
	rubricRef := rubric.ExactRef()
	round.Rubric = &rubricRef
	round.Digest = ""
	round, err = contract.SealReviewRoundV1(round)
	if err != nil {
		t.Fatal(err)
	}
	assignment := testkit.Assignment(caseClock.Now(), caseValue, round, contract.RouteAutoV1)
	assignment.ID, assignment.RoundID, assignment.RoundDigest = assignmentID, roundID, round.Digest
	assignment.Digest = ""
	assignment, err = contract.SealReviewerAssignmentV1(assignment)
	if err != nil {
		t.Fatal(err)
	}
	caseValue, _, assignment, err = engine.StartRoundV1(ctx, reviewport.StartRoundMutationV1{Expected: reviewport.ExpectedV1(caseValue.Revision, caseValue.Digest), Round: round, Assignment: assignment, Trace: testkit.Trace(caseClock.Now(), caseValue, contract.TraceAssignedV1, 2, round.ID, assignment.ID)})
	if err != nil {
		t.Fatal(err)
	}
	caseClock.Advance(time.Second)
	caseValue, assignment, err = engine.ClaimAssignmentV1(ctx, reviewport.ClaimAssignmentMutationV1{TenantID: caseValue.TenantID, ExpectedCase: reviewport.ExpectedV1(caseValue.Revision, caseValue.Digest), ExpectedAssignment: reviewport.ExpectedV1(assignment.Revision, assignment.Digest), CaseID: caseValue.ID, AssignmentID: assignment.ID, LeaseHolder: "conformance-worker-" + suffix, LeaseExpiresUnixNano: base.Add(15 * time.Minute).UnixNano(), UpdatedUnixNano: caseClock.Now().UnixNano(), Traces: []contract.TraceFactV1{testkit.StartedTrace(caseClock.Now(), caseValue, assignment.ID)}})
	if err != nil {
		t.Fatal(err)
	}
	scopeDigest, err := runtimeports.ExecutionScopeDigestV2(target.Scope)
	if err != nil {
		t.Fatal(err)
	}
	operation := runtimeports.OperationSubjectV3{Kind: runtimeports.OperationScopeRunV3, ExecutionScope: target.Scope, ExecutionScopeDigest: scopeDigest, RunID: target.RunID, SubjectRevision: 1, CurrentProjectionRef: "conformance-operation-current-" + suffix, CurrentProjectionDigest: testkit.Digest("conformance-operation-current-" + suffix), CurrentProjectionRevision: 1}
	operationDigest, err := operation.DigestV3()
	if err != nil {
		t.Fatal(err)
	}
	provider := runtimeports.ProviderBindingRefV2{BindingSetID: "conformance-binding", BindingSetRevision: 1, ComponentID: "praxis.model/reviewer", ManifestDigest: testkit.Digest("conformance-model-manifest"), ArtifactDigest: testkit.Digest("conformance-model-artifact"), Capability: "praxis.model/review"}
	attempt, err := contract.SealAutoReviewerAttemptV1(contract.AutoReviewerAttemptV1{
		FactIdentityV1: contract.FactIdentityV1{TenantID: caseValue.TenantID, ID: "conformance-attempt-" + suffix, Revision: 1, CreatedUnixNano: base.Add(6 * time.Second).UnixNano(), UpdatedUnixNano: base.Add(6 * time.Second).UnixNano()},
		IdempotencyKey: "conformance-idempotency-" + suffix, Case: conformanceResourceRefV1(caseValue.FactIdentityV1), Round: conformanceResourceRefV1(round.FactIdentityV1), Assignment: conformanceResourceRefV1(assignment.FactIdentityV1), Target: conformanceResourceRefV1(target.FactIdentityV1), Rubric: rubric.ExactRef(), ContextFrameDigest: frame.Digest,
		ReviewerID: assignment.ReviewerID, ReviewerAuthority: assignment.ReviewerAuthority, ReviewerBinding: assignment.ReviewerBinding, RouteID: "praxis.model/review-route", Operation: operation, OperationDigest: operationDigest,
		InvocationEffect: runtimeports.ReviewInvocationEffectRefV2{EffectID: core.EffectIntentID("conformance-effect-" + suffix), EffectRevision: 1, EffectKind: "praxis.review/auto-reviewer-invoke", PayloadDigest: testkit.Digest("conformance-payload-" + suffix), Provider: provider},
		ResultSchema:     resultSchema, RoundOrdinal: 1, MaxCostMicros: 1_000, State: contract.AutoReviewerAttemptPreparedV1, ExpiresUnixNano: base.Add(10 * time.Minute).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	store := &conformanceStoreFaultV1{conformanceOwnerStoreV1: baseStore}
	invocation := &conformanceInvocationV1{observedAt: base.Add(20 * time.Second), tokens: 100, costMicros: 50, sealed: make(map[contract.ExactResourceRefV1]reviewport.AutoReviewerInvocationResultV1)}
	clock := &conformanceClockV1{value: base.Add(30 * time.Second)}
	command := autoreviewer.RunCommandV1{Attempt: attempt, Context: frame, ResultID: "conformance-result-" + suffix}
	return conformanceFixtureV1{base: base, store: store, invocation: invocation, clock: clock, attempt: attempt, frame: frame, command: command}
}

func conformanceResourceRefV1(value contract.FactIdentityV1) contract.ExactResourceRefV1 {
	return contract.ExactResourceRefV1{ID: value.ID, Revision: value.Revision, Digest: value.Digest}
}

func conformanceObservationV1(attempt contract.AutoReviewerAttemptV1, observedAt time.Time, tokens, cost uint64) contract.AutoReviewerInvocationObservationV1 {
	output, err := contract.SealAutoReviewerStructuredOutputV1(contract.AutoReviewerStructuredOutputV1{Resolution: contract.ResolutionAcceptV1, ReasonCodes: []string{"review.auto/checked"}, Evidence: []runtimeports.ReviewEvidenceRefV2{{Ref: "evidence://authority", Classification: "review.authority/current", Digest: testkit.Digest("conformance-authority-current")}, {Ref: "evidence://scope", Classification: "review.scope/current", Digest: testkit.Digest("conformance-scope-current")}}})
	if err != nil {
		panic(err)
	}
	delegation := runtimeports.ExecutionDelegationRefV2{ID: "conformance-delegation", Revision: 1, Digest: testkit.Digest("conformance-delegation")}
	runtimeAttempt := runtimeports.OperationDispatchAttemptRefV3{OperationDigest: attempt.OperationDigest, EffectID: attempt.InvocationEffect.EffectID, IntentRevision: attempt.InvocationEffect.EffectRevision, IntentDigest: testkit.Digest("conformance-intent"), PermitID: "conformance-permit", PermitRevision: 1, PermitDigest: testkit.Digest("conformance-permit"), AttemptID: "conformance-runtime-attempt", Delegation: &delegation}
	provider := runtimeports.ProviderAttemptObservationRefV2{Delegation: delegation, PreparedAttemptID: "conformance-prepared", ProviderOperationRef: "conformance-provider-operation", Revision: 1, State: runtimeports.ProviderAttemptObservedV2, Digest: testkit.Digest("conformance-provider-observation"), PayloadDigest: testkit.Digest("conformance-provider-payload"), PayloadRevision: 1, SourceRegistrationID: "conformance-provider-source", SourceEpoch: 1, SourceSequence: 1, Evidence: runtimeports.EvidenceRecordRefV2{LedgerScopeDigest: testkit.Digest("conformance-ledger"), Sequence: 1, RecordDigest: testkit.Digest("conformance-record")}, ObservedUnixNano: observedAt.UnixNano()}
	value, err := contract.SealAutoReviewerInvocationObservationV1(contract.AutoReviewerInvocationObservationV1{FactIdentityV1: contract.FactIdentityV1{TenantID: attempt.TenantID, ID: "conformance-observation-" + attempt.ID, Revision: 1, CreatedUnixNano: observedAt.UnixNano(), UpdatedUnixNano: observedAt.UnixNano()}, AttemptID: attempt.ID, AttemptRevision: attempt.Revision, AttemptDigest: attempt.Digest, OperationDigest: attempt.OperationDigest, RuntimeAttempt: runtimeAttempt, ProviderObservation: provider, Output: output, ResultSchema: attempt.ResultSchema, Tokens: tokens, CostMicros: cost, ObservedUnixNano: observedAt.UnixNano(), ExpiresUnixNano: observedAt.Add(5 * time.Minute).UnixNano()})
	if err != nil {
		panic(err)
	}
	return value
}

func conformanceInvocationResultV1(observation contract.AutoReviewerInvocationObservationV1) reviewport.AutoReviewerInvocationResultV1 {
	draft := contract.AutoReviewerStructuredOutputDraftV1{Resolution: observation.Output.Resolution, ReasonCodes: observation.Output.ReasonCodes, Findings: append([]contract.AutoFindingDraftV1{}, observation.Output.Findings...), Evidence: observation.Output.Evidence, Conditions: observation.Output.Conditions}
	raw, err := json.Marshal(draft)
	if err != nil {
		panic(err)
	}
	return reviewport.AutoReviewerInvocationResultV1{ObservationID: observation.ID, Attempt: contract.ExactResourceRefV1{ID: observation.AttemptID, Revision: observation.AttemptRevision, Digest: observation.AttemptDigest}, OperationDigest: observation.OperationDigest, RuntimeAttempt: observation.RuntimeAttempt, ProviderObservation: observation.ProviderObservation, ResultSchema: observation.ResultSchema, RawOutput: raw, Tokens: observation.Tokens, CostMicros: observation.CostMicros, ObservedUnixNano: observation.ObservedUnixNano, ExpiresUnixNano: observation.ExpiresUnixNano}
}

func conformanceResultV1(command autoreviewer.RunCommandV1, observation contract.AutoReviewerInvocationObservationV1) contract.ReviewerInvocationResultFactV1 {
	attempt := command.Attempt
	value, err := contract.SealReviewerInvocationResultFactV1(contract.ReviewerInvocationResultFactV1{
		FactIdentityV1: contract.FactIdentityV1{TenantID: attempt.TenantID, ID: command.ResultID, Revision: 1, CreatedUnixNano: observation.ObservedUnixNano, UpdatedUnixNano: observation.ObservedUnixNano},
		CaseID:         attempt.Case.ID, CaseRevision: attempt.Case.Revision, RoundID: attempt.Round.ID, RoundRevision: attempt.Round.Revision, RoundDigest: attempt.Round.Digest,
		AssignmentID: attempt.Assignment.ID, AssignmentRevision: attempt.Assignment.Revision, AssignmentDigest: attempt.Assignment.Digest,
		TargetID: attempt.Target.ID, TargetRevision: attempt.Target.Revision, TargetDigest: attempt.Target.Digest,
		AttemptID: observation.RuntimeAttempt.AttemptID, ResultSchema: attempt.ResultSchema, ResultDigest: observation.Output.Digest, ObservationRefs: []string{observation.ID},
	})
	if err != nil {
		panic(err)
	}
	return value
}

func TestAutoReviewerOwnerConformanceV1(t *testing.T) {
	for _, backend := range []string{"memory", "sqlite"} {
		t.Run(backend, func(t *testing.T) {
			factory := conformanceFixtureFactoryV1{t: t, backend: backend}
			if err := conformance.CheckAutoReviewerOwnerV1(context.Background(), factory); err != nil {
				t.Fatal(err)
			}
		})
	}
}
