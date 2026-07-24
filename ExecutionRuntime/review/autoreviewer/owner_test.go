package autoreviewer_test

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/autoreviewer"
	"github.com/Proview-China/rax/ExecutionRuntime/review/caseengine"
	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/storetestkit"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/review/memory"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/review/reviewer"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type ownerStoreV1 interface {
	reviewport.StoreV1
	reviewport.AutoReviewerStoreV1
}

type ownerFixtureV1 struct {
	base    time.Time
	store   ownerStoreV1
	attempt contract.AutoReviewerAttemptV1
	frame   reviewer.ContextFrameV1
	command autoreviewer.RunCommandV1
}

func newOwnerFixtureV1(t testing.TB, suffix string) ownerFixtureV1 {
	t.Helper()
	ctx := context.Background()
	base := time.Unix(1_903_000_000, 0)
	caseClock := testkit.NewClock(base)
	store := storetestkit.NewMemoryStoreV1(caseClock.Now)
	rubric := testkit.Rubric(base, "tenant-a")
	if _, err := store.PublishRubricV1(ctx, reviewport.PublishRubricMutationV1{Next: rubric}); err != nil {
		t.Fatal(err)
	}
	target := testkit.Target(base)
	engine, err := caseengine.New(store, caseClock.Now)
	if err != nil {
		t.Fatal(err)
	}
	caseID := "case-owner-" + suffix
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
	roundID, assignmentID := "round-owner-"+suffix, "assignment-owner-"+suffix
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
		TenantID: target.TenantID, CaseID: caseID, RoundID: roundID,
		TargetDigest: target.Digest, OriginalIntentDigest: testkit.Digest("owner-intent-" + suffix), StableRulesDigest: testkit.Digest("owner-rules-" + suffix), ConfirmedDecisionsDigest: testkit.Digest("owner-decisions-" + suffix), EvidenceSetDigest: target.EvidenceSetDigest,
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
	caseValue, assignment, err = engine.ClaimAssignmentV1(ctx, reviewport.ClaimAssignmentMutationV1{TenantID: caseValue.TenantID, ExpectedCase: reviewport.ExpectedV1(caseValue.Revision, caseValue.Digest), ExpectedAssignment: reviewport.ExpectedV1(assignment.Revision, assignment.Digest), CaseID: caseValue.ID, AssignmentID: assignment.ID, LeaseHolder: "owner-worker-" + suffix, LeaseExpiresUnixNano: base.Add(15 * time.Minute).UnixNano(), UpdatedUnixNano: caseClock.Now().UnixNano(), Traces: []contract.TraceFactV1{testkit.StartedTrace(caseClock.Now(), caseValue, assignment.ID)}})
	if err != nil {
		t.Fatal(err)
	}
	scopeDigest, err := runtimeports.ExecutionScopeDigestV2(target.Scope)
	if err != nil {
		t.Fatal(err)
	}
	operation := runtimeports.OperationSubjectV3{Kind: runtimeports.OperationScopeRunV3, ExecutionScope: target.Scope, ExecutionScopeDigest: scopeDigest, RunID: target.RunID, SubjectRevision: 1, CurrentProjectionRef: "owner-operation-current-" + suffix, CurrentProjectionDigest: testkit.Digest("owner-operation-current-" + suffix), CurrentProjectionRevision: 1}
	operationDigest, err := operation.DigestV3()
	if err != nil {
		t.Fatal(err)
	}
	provider := runtimeports.ProviderBindingRefV2{BindingSetID: "owner-binding", BindingSetRevision: 1, ComponentID: "praxis.model/reviewer", ManifestDigest: testkit.Digest("owner-model-manifest"), ArtifactDigest: testkit.Digest("owner-model-artifact"), Capability: "praxis.model/review"}
	attempt, err := contract.SealAutoReviewerAttemptV1(contract.AutoReviewerAttemptV1{
		FactIdentityV1: contract.FactIdentityV1{TenantID: caseValue.TenantID, ID: "owner-attempt-" + suffix, Revision: 1, CreatedUnixNano: base.Add(6 * time.Second).UnixNano(), UpdatedUnixNano: base.Add(6 * time.Second).UnixNano()},
		IdempotencyKey: "owner-idempotency-" + suffix, Case: resourceRefV1(caseValue.FactIdentityV1), Round: resourceRefV1(round.FactIdentityV1), Assignment: resourceRefV1(assignment.FactIdentityV1), Target: resourceRefV1(target.FactIdentityV1), Rubric: rubric.ExactRef(), ContextFrameDigest: frame.Digest,
		ReviewerID: assignment.ReviewerID, ReviewerAuthority: assignment.ReviewerAuthority, ReviewerBinding: assignment.ReviewerBinding, RouteID: "praxis.model/review-route", Operation: operation, OperationDigest: operationDigest,
		InvocationEffect: runtimeports.ReviewInvocationEffectRefV2{EffectID: core.EffectIntentID("owner-effect-" + suffix), EffectRevision: 1, EffectKind: "praxis.review/auto-reviewer-invoke", PayloadDigest: testkit.Digest("owner-payload-" + suffix), Provider: provider},
		ResultSchema:     resultSchema, RoundOrdinal: 1, MaxCostMicros: 1_000, State: contract.AutoReviewerAttemptPreparedV1, ExpiresUnixNano: base.Add(10 * time.Minute).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	command := autoreviewer.RunCommandV1{Attempt: attempt, Context: frame, ResultID: "owner-result-" + suffix}
	return ownerFixtureV1{base: base, store: store, attempt: attempt, frame: frame, command: command}
}

func resourceRefV1(value contract.FactIdentityV1) contract.ExactResourceRefV1 {
	return contract.ExactResourceRefV1{ID: value.ID, Revision: value.Revision, Digest: value.Digest}
}

type manualClockV1 struct {
	mu    sync.Mutex
	value time.Time
}

func (c *manualClockV1) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	value := c.value
	c.value = c.value.Add(time.Nanosecond)
	return value
}
func (c *manualClockV1) Set(value time.Time) { c.mu.Lock(); c.value = value; c.mu.Unlock() }

type invocationFakeV1 struct {
	mu            sync.Mutex
	observedAt    time.Time
	tokens        uint64
	costMicros    uint64
	rawOutput     json.RawMessage
	startErr      error
	inspectErr    error
	startMayCall  bool
	onStart       func()
	onObservation func()
	onInspect     func()
	startCalls    int
	inspectCalls  int
	providerCalls int
	sealed        map[contract.ExactResourceRefV1]reviewport.AutoReviewerInvocationResultV1
	inspectRefs   []contract.ExactResourceRefV1
}

type reviewerContextReaderFakeV1 struct {
	mu       sync.Mutex
	value    contract.ReviewerContextEnvelopeV1
	calls    int
	onCall   func(int)
	failCall int
}

func (f *reviewerContextReaderFakeV1) ResolveCurrentReviewerContextV1(_ context.Context, request reviewport.ReviewerContextCurrentResolveRequestV1) (contract.ReviewerContextEnvelopeRefV1, error) {
	if err := request.Validate(); err != nil {
		return contract.ReviewerContextEnvelopeRefV1{}, err
	}
	return f.value.Ref, nil
}

func (f *reviewerContextReaderFakeV1) InspectCurrentReviewerContextV1(_ context.Context, subject contract.ReviewerContextSubjectV1, expected contract.ReviewerContextEnvelopeRefV1) (contract.ReviewerContextEnvelopeV1, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.onCall != nil {
		f.onCall(f.calls)
	}
	if f.failCall == f.calls {
		return contract.ReviewerContextEnvelopeV1{}, core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "injected Context current drift")
	}
	if subject != f.value.Subject || expected != f.value.Ref {
		return contract.ReviewerContextEnvelopeV1{}, core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "Context exact subject or ref drifted")
	}
	return f.value.Clone(), nil
}

func (f *reviewerContextReaderFakeV1) InspectHistoricalReviewerContextV1(_ context.Context, expected contract.ReviewerContextEnvelopeRefV1) (contract.ReviewerContextEnvelopeV1, error) {
	if expected != f.value.Ref {
		return contract.ReviewerContextEnvelopeV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "Context history is absent")
	}
	return f.value.Clone(), nil
}

func bindProductionReviewerContextV1(t testing.TB, fixture *ownerFixtureV1) *reviewerContextReaderFakeV1 {
	t.Helper()
	attempt := fixture.attempt
	expires := fixture.base.Add(12 * time.Minute).UnixNano()
	kinds := []contract.ReviewerContextMaterialKindV1{
		contract.ReviewerContextOriginalIntentV1, contract.ReviewerContextRequirementV1,
		contract.ReviewerContextAcceptanceCriterionV1, contract.ReviewerContextStableRuleV1,
		contract.ReviewerContextCandidateV1, contract.ReviewerContextEvidenceV1, contract.ReviewerContextKnownRiskV1,
	}
	materials := make([]contract.ReviewerContextMaterialV1, 0, len(kinds))
	for _, kind := range kinds {
		content := string(kind)
		trust := contract.ReviewerContextObservationV1
		if kind == contract.ReviewerContextOriginalIntentV1 || kind == contract.ReviewerContextRequirementV1 || kind == contract.ReviewerContextAcceptanceCriterionV1 || kind == contract.ReviewerContextStableRuleV1 {
			trust = contract.ReviewerContextInstructionV1
		}
		materials = append(materials, contract.ReviewerContextMaterialV1{
			Kind:      kind,
			Source:    contract.ReviewerContextSourceRefV1{Owner: "praxis.context/source", ID: "owner-context-" + content, Revision: 1, Digest: testkit.Digest("owner-context-source-" + content), ExpiresUnixNano: expires},
			MediaType: "text/plain", Content: content, ContentDigest: core.DigestBytes([]byte(content)), Trust: trust,
		})
	}
	capabilities := append([]string(nil), fixture.frame.AllowedReadCapabilities...)
	value, err := contract.SealReviewerContextEnvelopeV1(contract.ReviewerContextEnvelopeV1{
		Ref: contract.ReviewerContextEnvelopeRefV1{Revision: 1}, Subject: attempt.ReviewerContextSubjectV1(),
		Materials: materials, AllowedReadCapabilities: capabilities, ReadOnly: true, WorkIdentityRemoved: true,
		State: contract.ReviewerContextEnvelopeActiveV1, Current: true,
		CheckedUnixNano: fixture.base.Add(5 * time.Second).UnixNano(), ExpiresUnixNano: expires,
	})
	if err != nil {
		t.Fatal(err)
	}
	ref := value.Ref
	attempt.ReviewerContext = &ref
	attempt.Digest = ""
	attempt, err = contract.SealAutoReviewerAttemptV1(attempt)
	if err != nil {
		t.Fatal(err)
	}
	fixture.attempt, fixture.command.Attempt = attempt, attempt
	return &reviewerContextReaderFakeV1{value: value}
}

func (f *invocationFakeV1) StartOrInspectAutoReviewerInvocationV1(_ context.Context, command reviewport.AutoReviewerInvocationCommandV1) (reviewport.AutoReviewerInvocationResultV1, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.startCalls++
	if f.onStart != nil {
		f.onStart()
	}
	if f.startErr != nil {
		if f.startMayCall && f.providerCalls == 0 {
			f.providerCalls++
			f.sealed[command.Attempt.ExactRef()] = f.invocationResultV1(command.Attempt)
			if f.onObservation != nil {
				f.onObservation()
			}
		}
		return reviewport.AutoReviewerInvocationResultV1{}, f.startErr
	}
	if value, ok := f.sealed[command.Attempt.ExactRef()]; ok {
		return value, nil
	}
	f.providerCalls++
	value := f.invocationResultV1(command.Attempt)
	f.sealed[command.Attempt.ExactRef()] = value
	if f.onObservation != nil {
		f.onObservation()
	}
	return value, nil
}

func (f *invocationFakeV1) InspectAutoReviewerInvocationV1(ctx context.Context, ref contract.ExactResourceRefV1) (reviewport.AutoReviewerInvocationResultV1, error) {
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
	if value, ok := f.sealed[ref]; ok {
		if f.onInspect != nil {
			f.onInspect()
		}
		return value, nil
	}
	return reviewport.AutoReviewerInvocationResultV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "fake has no exact original invocation Observation")
}

func makeInvocationFakeV1(f ownerFixtureV1) *invocationFakeV1 {
	return &invocationFakeV1{observedAt: f.base.Add(20 * time.Second), tokens: 100, costMicros: 50, sealed: map[contract.ExactResourceRefV1]reviewport.AutoReviewerInvocationResultV1{}}
}

func (f *invocationFakeV1) invocationResultV1(attempt contract.AutoReviewerAttemptV1) reviewport.AutoReviewerInvocationResultV1 {
	result := untrustedInvocationResultV1(makeObservationV1(attempt, f.observedAt, f.tokens, f.costMicros))
	if f.rawOutput != nil {
		result.RawOutput = append(json.RawMessage(nil), f.rawOutput...)
	}
	return result
}

func untrustedInvocationResultV1(observation contract.AutoReviewerInvocationObservationV1) reviewport.AutoReviewerInvocationResultV1 {
	draft := contract.AutoReviewerStructuredOutputDraftV1{Resolution: observation.Output.Resolution, ReasonCodes: observation.Output.ReasonCodes, Findings: append([]contract.AutoFindingDraftV1{}, observation.Output.Findings...), Evidence: observation.Output.Evidence, Conditions: observation.Output.Conditions}
	raw, err := json.Marshal(draft)
	if err != nil {
		panic(err)
	}
	return reviewport.AutoReviewerInvocationResultV1{ObservationID: observation.ID, Attempt: contract.ExactResourceRefV1{ID: observation.AttemptID, Revision: observation.AttemptRevision, Digest: observation.AttemptDigest}, OperationDigest: observation.OperationDigest, RuntimeAttempt: observation.RuntimeAttempt, ProviderObservation: observation.ProviderObservation, ResultSchema: observation.ResultSchema, RawOutput: raw, Tokens: observation.Tokens, CostMicros: observation.CostMicros, ObservedUnixNano: observation.ObservedUnixNano, ExpiresUnixNano: observation.ExpiresUnixNano}
}

func makeObservationV1(attempt contract.AutoReviewerAttemptV1, observedAt time.Time, tokens, cost uint64) contract.AutoReviewerInvocationObservationV1 {
	output, err := contract.SealAutoReviewerStructuredOutputV1(contract.AutoReviewerStructuredOutputV1{Resolution: contract.ResolutionAcceptV1, ReasonCodes: []string{"review.auto/checked"}, Evidence: []runtimeports.ReviewEvidenceRefV2{{Ref: "evidence://authority", Classification: "review.authority/current", Digest: testkit.Digest("owner-authority-current")}, {Ref: "evidence://scope", Classification: "review.scope/current", Digest: testkit.Digest("owner-scope-current")}}})
	if err != nil {
		panic(err)
	}
	delegation := runtimeports.ExecutionDelegationRefV2{ID: "owner-delegation", Revision: 1, Digest: testkit.Digest("owner-delegation")}
	runtimeAttempt := runtimeports.OperationDispatchAttemptRefV3{OperationDigest: attempt.OperationDigest, EffectID: attempt.InvocationEffect.EffectID, IntentRevision: attempt.InvocationEffect.EffectRevision, IntentDigest: testkit.Digest("owner-intent"), PermitID: "owner-permit", PermitRevision: 1, PermitDigest: testkit.Digest("owner-permit"), AttemptID: "owner-runtime-attempt", Delegation: &delegation}
	provider := runtimeports.ProviderAttemptObservationRefV2{Delegation: delegation, PreparedAttemptID: "owner-prepared", ProviderOperationRef: "owner-provider-operation", Revision: 1, State: runtimeports.ProviderAttemptObservedV2, Digest: testkit.Digest("owner-provider-observation"), PayloadDigest: testkit.Digest("owner-provider-payload"), PayloadRevision: 1, SourceRegistrationID: "owner-provider-source", SourceEpoch: 1, SourceSequence: 1, Evidence: runtimeports.EvidenceRecordRefV2{LedgerScopeDigest: testkit.Digest("owner-ledger"), Sequence: 1, RecordDigest: testkit.Digest("owner-record")}, ObservedUnixNano: observedAt.UnixNano()}
	value, err := contract.SealAutoReviewerInvocationObservationV1(contract.AutoReviewerInvocationObservationV1{FactIdentityV1: contract.FactIdentityV1{TenantID: attempt.TenantID, ID: "owner-observation-" + attempt.ID, Revision: 1, CreatedUnixNano: observedAt.UnixNano(), UpdatedUnixNano: observedAt.UnixNano()}, AttemptID: attempt.ID, AttemptRevision: attempt.Revision, AttemptDigest: attempt.Digest, OperationDigest: attempt.OperationDigest, RuntimeAttempt: runtimeAttempt, ProviderObservation: provider, Output: output, ResultSchema: attempt.ResultSchema, Tokens: tokens, CostMicros: cost, ObservedUnixNano: observedAt.UnixNano(), ExpiresUnixNano: observedAt.Add(5 * time.Minute).UnixNano()})
	if err != nil {
		panic(err)
	}
	return value
}

func TestOwnerHappyPathAndReplayAreExactV1(t *testing.T) {
	fixture := newOwnerFixtureV1(t, "happy")
	clock := &manualClockV1{value: fixture.base.Add(30 * time.Second)}
	invocation := makeInvocationFakeV1(fixture)
	owner, err := autoreviewer.New(fixture.store, invocation, clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	got, err := owner.RunV1(context.Background(), fixture.command)
	if err != nil || got.Attempt.State != contract.AutoReviewerAttemptObservedV1 || got.Observation == nil || got.DomainResult == nil {
		t.Fatalf("Auto Reviewer happy path failed: %+v %v", got, err)
	}
	replayed, err := owner.RunV1(context.Background(), fixture.command)
	if err != nil || replayed.Attempt.Digest != got.Attempt.Digest || replayed.DomainResult.Digest != got.DomainResult.Digest {
		t.Fatalf("Auto Reviewer exact replay failed: %+v %v", replayed, err)
	}
	if invocation.startCalls != 1 || invocation.inspectCalls != 0 || invocation.providerCalls != 1 {
		t.Fatalf("unexpected invocation counts: start=%d inspect=%d provider=%d", invocation.startCalls, invocation.inspectCalls, invocation.providerCalls)
	}
	changed := fixture.command
	changed.ResultID = "changed-result-id"
	if _, err := owner.RunV1(context.Background(), changed); !core.HasReason(err, core.ReasonIdempotencyPayloadMismatch) || invocation.startCalls != 1 {
		t.Fatalf("changed DomainResult identity did not conflict before invocation: %v", err)
	}
}

func TestOwnerStrictlyValidatesRawProviderOutputBeforeAnyReviewFactV1(t *testing.T) {
	for _, test := range []struct {
		name string
		raw  string
	}{
		{name: "duplicate-top-level", raw: `{"resolution":"accept","resolution":"reject","reason_codes":["x"],"findings":[],"evidence":[]}`},
		{name: "nested-duplicate", raw: `{"resolution":"accept","reason_codes":["x"],"findings":[],"evidence":[{"ref":"e","ref":"other","classification":"review.test/result","digest":"sha256:0000000000000000000000000000000000000000000000000000000000000000"}]}`},
		{name: "model-schema-flag", raw: `{"resolution":"accept","reason_codes":["x"],"findings":[],"evidence":[],"schema_valid":true}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newOwnerFixtureV1(t, "raw-"+test.name)
			clock := &manualClockV1{value: fixture.base.Add(30 * time.Second)}
			invocation := makeInvocationFakeV1(fixture)
			invocation.rawOutput = json.RawMessage(test.raw)
			owner, err := autoreviewer.New(fixture.store, invocation, clock.Now)
			if err != nil {
				t.Fatal(err)
			}
			result, err := owner.RunV1(context.Background(), fixture.command)
			if err == nil || result.Observation != nil || result.DomainResult != nil {
				t.Fatalf("untrusted raw output crossed the Review Owner boundary: %+v err=%v", result, err)
			}
			current, inspectErr := fixture.store.InspectAutoReviewerAttemptCurrentV1(context.Background(), fixture.attempt.TenantID, fixture.attempt.ID)
			if inspectErr != nil || current.State != contract.AutoReviewerAttemptWaitingInspectV1 || current.Observation != nil || current.DomainResult != nil {
				t.Fatalf("invalid raw output changed Review current facts: %+v err=%v", current, inspectErr)
			}
		})
	}
}

func TestProductionOwnerRereadsExactReviewerContextAtEveryCurrentCutV1(t *testing.T) {
	fixture := newOwnerFixtureV1(t, "production-context")
	reader := bindProductionReviewerContextV1(t, &fixture)
	clock := &manualClockV1{value: fixture.base.Add(30 * time.Second)}
	invocation := makeInvocationFakeV1(fixture)
	owner, err := autoreviewer.NewProduction(fixture.store, invocation, reader, outputSchemaReaderV1(t), clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	fixture.command.Context = reviewer.ContextFrameV1{} // production must not trust caller legacy current.
	got, err := owner.RunV1(context.Background(), fixture.command)
	if err != nil || got.Attempt.State != contract.AutoReviewerAttemptObservedV1 {
		t.Fatalf("production exact Context path failed: %+v %v", got, err)
	}
	if reader.calls < 3 || invocation.providerCalls != 1 {
		t.Fatalf("production Context was not reread at S1/S2/result cut: reads=%d provider=%d", reader.calls, invocation.providerCalls)
	}
}

func TestProductionOwnerFailsClosedOnReviewerContextS2DriftV1(t *testing.T) {
	fixture := newOwnerFixtureV1(t, "production-context-drift")
	reader := bindProductionReviewerContextV1(t, &fixture)
	reader.failCall = 2
	clock := &manualClockV1{value: fixture.base.Add(30 * time.Second)}
	invocation := makeInvocationFakeV1(fixture)
	owner, err := autoreviewer.NewProduction(fixture.store, invocation, reader, outputSchemaReaderV1(t), clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	fixture.command.Context = reviewer.ContextFrameV1{}
	if _, err = owner.RunV1(context.Background(), fixture.command); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("Reviewer Context S2 drift did not fail closed: %v", err)
	}
	if invocation.startCalls != 0 || invocation.providerCalls != 0 {
		t.Fatalf("Context drift crossed invocation boundary: start=%d provider=%d", invocation.startCalls, invocation.providerCalls)
	}
}

func TestOwnerUnknownMovesPermanentlyToInspectAndDetachesRecoveryV1(t *testing.T) {
	fixture := newOwnerFixtureV1(t, "unknown")
	clock := &manualClockV1{value: fixture.base.Add(30 * time.Second)}
	invocation := makeInvocationFakeV1(fixture)
	ctx, cancel := context.WithCancel(context.Background())
	invocation.startErr = core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "injected lost reply")
	invocation.startMayCall = true
	invocation.onStart = cancel
	invocation.observedAt = fixture.base.Add(31 * time.Second)
	invocation.onObservation = func() { clock.Set(fixture.base.Add(32 * time.Second)) }
	invocation.onInspect = func() { clock.Set(fixture.base.Add(33 * time.Second)) }
	owner, err := autoreviewer.New(fixture.store, invocation, clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	got, err := owner.RunV1(ctx, fixture.command)
	if err != nil || got.Attempt.State != contract.AutoReviewerAttemptObservedV1 || invocation.startCalls != 1 || invocation.inspectCalls != 1 || invocation.providerCalls != 1 {
		t.Fatalf("lost reply was not recovered by detached exact Inspect: %+v start=%d inspect=%d provider=%d err=%v", got, invocation.startCalls, invocation.inspectCalls, invocation.providerCalls, err)
	}
	if got.Attempt.InvocationAttempt == nil || *got.Attempt.InvocationAttempt != fixture.attempt.ExactRef() || got.Observation.AttemptRevision != fixture.attempt.Revision || got.Observation.AttemptDigest != fixture.attempt.Digest || len(invocation.inspectRefs) != 1 || invocation.inspectRefs[0] != fixture.attempt.ExactRef() {
		t.Fatalf("unknown recovery did not preserve the original invocation ref: attempt=%+v observation=%+v inspect=%+v", got.Attempt.InvocationAttempt, got.Observation, invocation.inspectRefs)
	}
}

func TestOwnerPersistentUnknownNeverRestartsV1(t *testing.T) {
	fixture := newOwnerFixtureV1(t, "persistent")
	clock := &manualClockV1{value: fixture.base.Add(30 * time.Second)}
	invocation := makeInvocationFakeV1(fixture)
	unknown := core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "persistent unknown")
	invocation.startErr, invocation.inspectErr = unknown, unknown
	invocation.startMayCall = true
	owner, _ := autoreviewer.New(fixture.store, invocation, clock.Now)
	first, err := owner.RunV1(context.Background(), fixture.command)
	if !core.HasCategory(err, core.ErrorIndeterminate) || first.Attempt.State != contract.AutoReviewerAttemptWaitingInspectV1 {
		t.Fatalf("first unknown did not persist waiting_inspect: %+v %v", first, err)
	}
	providerBefore := invocation.providerCalls
	if _, waitingErr := invocation.InspectAutoReviewerInvocationV1(context.Background(), first.Attempt.ExactRef()); !core.HasCategory(waitingErr, core.ErrorIndeterminate) || invocation.providerCalls != providerBefore {
		t.Fatalf("waiting revision was accepted as the original invocation or created a provider call: %v provider=%d", waitingErr, invocation.providerCalls)
	}
	second, err := owner.RunV1(context.Background(), fixture.command)
	if !core.HasCategory(err, core.ErrorIndeterminate) || second.Attempt.State != contract.AutoReviewerAttemptWaitingInspectV1 || invocation.startCalls != 1 || invocation.inspectCalls != 3 {
		t.Fatalf("waiting_inspect restarted invocation: %+v start=%d inspect=%d err=%v", second, invocation.startCalls, invocation.inspectCalls, err)
	}
}

func TestOwnerInspectNotFoundKeepsWaitingAndNeverCallsProviderV1(t *testing.T) {
	fixture := newOwnerFixtureV1(t, "inspect-not-found")
	clock := &manualClockV1{value: fixture.base.Add(30 * time.Second)}
	invocation := makeInvocationFakeV1(fixture)
	invocation.startErr = core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "unknown before authoritative observation")
	owner, _ := autoreviewer.New(fixture.store, invocation, clock.Now)
	first, err := owner.RunV1(context.Background(), fixture.command)
	if !core.HasCategory(err, core.ErrorIndeterminate) || first.Attempt.State != contract.AutoReviewerAttemptWaitingInspectV1 || invocation.providerCalls != 0 || invocation.startCalls != 1 || invocation.inspectCalls != 1 {
		t.Fatalf("Inspect NotFound did not remain waiting with zero provider: %+v provider=%d start=%d inspect=%d err=%v", first, invocation.providerCalls, invocation.startCalls, invocation.inspectCalls, err)
	}
	if _, waitingErr := invocation.InspectAutoReviewerInvocationV1(context.Background(), first.Attempt.ExactRef()); !core.HasCategory(waitingErr, core.ErrorNotFound) || invocation.providerCalls != 0 {
		t.Fatalf("waiting revision unexpectedly resolved or created a provider call: %v provider=%d", waitingErr, invocation.providerCalls)
	}
	second, err := owner.RunV1(context.Background(), fixture.command)
	if !core.HasCategory(err, core.ErrorNotFound) || second.Attempt.State != contract.AutoReviewerAttemptWaitingInspectV1 || invocation.providerCalls != 0 || invocation.startCalls != 1 || invocation.inspectCalls != 3 || invocation.inspectRefs[0] != fixture.attempt.ExactRef() || invocation.inspectRefs[1] != first.Attempt.ExactRef() || invocation.inspectRefs[2] != fixture.attempt.ExactRef() {
		t.Fatalf("waiting NotFound retried Start/provider or changed exact ref: %+v provider=%d start=%d inspect=%d refs=%+v err=%v", second, invocation.providerCalls, invocation.startCalls, invocation.inspectCalls, invocation.inspectRefs, err)
	}
}

type beginLostReplyStoreV1 struct {
	ownerStoreV1
	calls atomic.Int32
}

func (s *beginLostReplyStoreV1) BeginAutoReviewerAttemptV1(ctx context.Context, mutation reviewport.BeginAutoReviewerAttemptMutationV1) (contract.AutoReviewerAttemptV1, error) {
	value, err := s.ownerStoreV1.BeginAutoReviewerAttemptV1(ctx, mutation)
	if err != nil {
		return value, err
	}
	if s.calls.Add(1) == 1 {
		return contract.AutoReviewerAttemptV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "injected Begin reply loss")
	}
	return value, nil
}

func TestOwnerBeginLostReplyUsesExactInspectWithoutMutationReplayV1(t *testing.T) {
	fixture := newOwnerFixtureV1(t, "begin-lost")
	store := &beginLostReplyStoreV1{ownerStoreV1: fixture.store}
	invocation := makeInvocationFakeV1(fixture)
	clock := &manualClockV1{value: fixture.base.Add(30 * time.Second)}
	owner, _ := autoreviewer.New(store, invocation, clock.Now)
	got, err := owner.RunV1(context.Background(), fixture.command)
	if err != nil || got.Attempt.State != contract.AutoReviewerAttemptObservedV1 || store.calls.Load() != 1 {
		t.Fatalf("Begin lost reply replayed mutation or failed recovery: %+v calls=%d err=%v", got, store.calls.Load(), err)
	}
}

type mutationLostReplyStoreV1 struct {
	ownerStoreV1
	markCalls   atomic.Int32
	recordCalls atomic.Int32
	loseMark    bool
	loseRecord  bool
}

type markUnknownBeforeCommitStoreV1 struct {
	ownerStoreV1
	markCalls atomic.Int32
}

func (s *markUnknownBeforeCommitStoreV1) MarkAutoReviewerWaitingInspectV1(ctx context.Context, mutation reviewport.MarkAutoReviewerWaitingInspectMutationV1) (reviewport.AutoReviewerInvocationStartClaimReceiptV1, error) {
	if s.markCalls.Add(1) == 1 {
		return reviewport.AutoReviewerInvocationStartClaimReceiptV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "injected pre-commit start-claim uncertainty")
	}
	return s.ownerStoreV1.MarkAutoReviewerWaitingInspectV1(ctx, mutation)
}

func (s *mutationLostReplyStoreV1) MarkAutoReviewerWaitingInspectV1(ctx context.Context, mutation reviewport.MarkAutoReviewerWaitingInspectMutationV1) (reviewport.AutoReviewerInvocationStartClaimReceiptV1, error) {
	value, err := s.ownerStoreV1.MarkAutoReviewerWaitingInspectV1(ctx, mutation)
	if err != nil {
		return value, err
	}
	if s.markCalls.Add(1) == 1 && s.loseMark {
		return reviewport.AutoReviewerInvocationStartClaimReceiptV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "injected waiting reply loss")
	}
	return value, nil
}

func (s *mutationLostReplyStoreV1) RecordAutoReviewerObservationV1(ctx context.Context, mutation reviewport.RecordAutoReviewerObservationMutationV1) (contract.AutoReviewerAttemptV1, contract.ReviewerInvocationResultFactV1, error) {
	attempt, result, err := s.ownerStoreV1.RecordAutoReviewerObservationV1(ctx, mutation)
	if err != nil {
		return attempt, result, err
	}
	if s.recordCalls.Add(1) == 1 && s.loseRecord {
		return contract.AutoReviewerAttemptV1{}, contract.ReviewerInvocationResultFactV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "injected record reply loss")
	}
	return attempt, result, nil
}

func TestOwnerWaitingAndRecordLostRepliesRecoverByExactInspectV1(t *testing.T) {
	t.Run("waiting", func(t *testing.T) {
		fixture := newOwnerFixtureV1(t, "waiting-lost")
		store := &mutationLostReplyStoreV1{ownerStoreV1: fixture.store, loseMark: true}
		clock := &manualClockV1{value: fixture.base.Add(30 * time.Second)}
		invocation := makeInvocationFakeV1(fixture)
		owner, _ := autoreviewer.New(store, invocation, clock.Now)
		got, err := owner.RunV1(context.Background(), fixture.command)
		if !core.HasCategory(err, core.ErrorNotFound) || got.Attempt.State != contract.AutoReviewerAttemptWaitingInspectV1 || store.markCalls.Load() != 1 || invocation.startCalls != 0 || invocation.inspectCalls != 1 || invocation.providerCalls != 0 {
			t.Fatalf("waiting lost reply was replayed or not recovered: %+v mark=%d start=%d inspect=%d err=%v", got, store.markCalls.Load(), invocation.startCalls, invocation.inspectCalls, err)
		}
		_, replayErr := owner.RunV1(context.Background(), fixture.command)
		if !core.HasCategory(replayErr, core.ErrorNotFound) || invocation.startCalls != 0 || invocation.providerCalls != 0 {
			t.Fatalf("lost start-claim reply later granted an invocation: start=%d provider=%d err=%v", invocation.startCalls, invocation.providerCalls, replayErr)
		}
	})
	t.Run("record", func(t *testing.T) {
		fixture := newOwnerFixtureV1(t, "record-lost")
		store := &mutationLostReplyStoreV1{ownerStoreV1: fixture.store, loseRecord: true}
		clock := &manualClockV1{value: fixture.base.Add(30 * time.Second)}
		invocation := makeInvocationFakeV1(fixture)
		owner, _ := autoreviewer.New(store, invocation, clock.Now)
		got, err := owner.RunV1(context.Background(), fixture.command)
		if err != nil || got.Attempt.State != contract.AutoReviewerAttemptObservedV1 || got.DomainResult == nil || store.recordCalls.Load() != 1 {
			t.Fatalf("record lost reply was replayed or not recovered: %+v record=%d err=%v", got, store.recordCalls.Load(), err)
		}
	})
}

func TestOwnerStartClaimUnknownBeforeCommitNeverCrossesInvocationBoundaryV1(t *testing.T) {
	fixture := newOwnerFixtureV1(t, "claim-precommit-unknown")
	store := &markUnknownBeforeCommitStoreV1{ownerStoreV1: fixture.store}
	clock := &manualClockV1{value: fixture.base.Add(30 * time.Second)}
	invocation := makeInvocationFakeV1(fixture)
	owner, err := autoreviewer.New(store, invocation, clock.Now)
	if err != nil {
		t.Fatal(err)
	}

	first, firstErr := owner.RunV1(context.Background(), fixture.command)
	current, inspectErr := store.InspectAutoReviewerAttemptCurrentV1(context.Background(), fixture.attempt.TenantID, fixture.attempt.ID)
	if !core.HasCategory(firstErr, core.ErrorIndeterminate) || first.Attempt.State != contract.AutoReviewerAttemptPreparedV1 || inspectErr != nil || current.State != contract.AutoReviewerAttemptPreparedV1 || invocation.startCalls != 0 || invocation.inspectCalls != 0 || invocation.providerCalls != 0 {
		t.Fatalf("pre-commit start-claim uncertainty crossed the invocation boundary: result=%+v current=%s start=%d inspect=%d provider=%d inspectErr=%v err=%v", first, current.State, invocation.startCalls, invocation.inspectCalls, invocation.providerCalls, inspectErr, firstErr)
	}

	second, secondErr := owner.RunV1(context.Background(), fixture.command)
	if secondErr != nil || second.Attempt.State != contract.AutoReviewerAttemptObservedV1 || store.markCalls.Load() != 2 || invocation.startCalls != 1 || invocation.providerCalls != 1 {
		t.Fatalf("safe retry did not first acquire the persistent start claim: result=%+v marks=%d start=%d provider=%d err=%v", second, store.markCalls.Load(), invocation.startCalls, invocation.providerCalls, secondErr)
	}
}

func TestOwnerConcurrentLostReplyConvergesReadOnlyV1(t *testing.T) {
	t.Run("persistent_unknown_64", func(t *testing.T) {
		fixture := newOwnerFixtureV1(t, "concurrent-persistent-unknown")
		clock := &manualClockV1{value: fixture.base.Add(30 * time.Second)}
		invocation := makeInvocationFakeV1(fixture)
		unknown := core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "persistent concurrent unknown")
		invocation.startErr, invocation.inspectErr, invocation.startMayCall = unknown, unknown, true
		owner, err := autoreviewer.New(fixture.store, invocation, clock.Now)
		if err != nil {
			t.Fatal(err)
		}
		var waiting atomic.Int32
		var wg sync.WaitGroup
		for i := 0; i < 64; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				got, runErr := owner.RunV1(context.Background(), fixture.command)
				if core.HasCategory(runErr, core.ErrorIndeterminate) && got.Attempt.State == contract.AutoReviewerAttemptWaitingInspectV1 {
					waiting.Add(1)
				}
			}()
		}
		wg.Wait()
		startBeforeReplay := invocation.startCalls
		got, replayErr := owner.RunV1(context.Background(), fixture.command)
		if !core.HasCategory(replayErr, core.ErrorIndeterminate) || got.Attempt.State != contract.AutoReviewerAttemptWaitingInspectV1 || waiting.Load() != 64 || invocation.startCalls != startBeforeReplay || invocation.providerCalls != 1 {
			t.Fatalf("persistent concurrent unknown escaped inspect-only: waiting=%d state=%s starts=%d/%d provider=%d err=%v", waiting.Load(), got.Attempt.State, startBeforeReplay, invocation.startCalls, invocation.providerCalls, replayErr)
		}
	})
}

func TestOwnerBudgetTTLAndClockFailuresHaveNoDomainResultV1(t *testing.T) {
	t.Run("budget", func(t *testing.T) {
		fixture := newOwnerFixtureV1(t, "budget")
		clock := &manualClockV1{value: fixture.base.Add(30 * time.Second)}
		invocation := makeInvocationFakeV1(fixture)
		invocation.tokens = 100_000
		owner, _ := autoreviewer.New(fixture.store, invocation, clock.Now)
		got, err := owner.RunV1(context.Background(), fixture.command)
		if !core.HasReason(err, core.ReasonBudgetBindingStale) || got.Attempt.State != contract.AutoReviewerAttemptEscalatedV1 {
			t.Fatalf("budget overrun did not escalate: %+v %v", got, err)
		}
		assertNoOwnerResultV1(t, fixture)
	})
	t.Run("ttl_crossing", func(t *testing.T) {
		fixture := newOwnerFixtureV1(t, "ttl")
		clock := &manualClockV1{value: fixture.base.Add(30 * time.Second)}
		invocation := makeInvocationFakeV1(fixture)
		invocation.onObservation = func() { clock.Set(time.Unix(0, fixture.attempt.ExpiresUnixNano)) }
		owner, _ := autoreviewer.New(fixture.store, invocation, clock.Now)
		if _, err := owner.RunV1(context.Background(), fixture.command); !core.HasCategory(err, core.ErrorPreconditionFailed) {
			t.Fatalf("TTL crossing did not fail closed: %v", err)
		}
		assertNoOwnerResultV1(t, fixture)
	})
	t.Run("clock_rollback", func(t *testing.T) {
		fixture := newOwnerFixtureV1(t, "rollback")
		clock := &manualClockV1{value: fixture.base.Add(30 * time.Second)}
		invocation := makeInvocationFakeV1(fixture)
		invocation.onObservation = func() { clock.Set(fixture.base.Add(25 * time.Second)) }
		owner, _ := autoreviewer.New(fixture.store, invocation, clock.Now)
		if _, err := owner.RunV1(context.Background(), fixture.command); !core.HasReason(err, core.ReasonClockRegression) {
			t.Fatalf("clock rollback did not fail closed: %v", err)
		}
		assertNoOwnerResultV1(t, fixture)
	})
}

type rubricDriftStoreV1 struct {
	ownerStoreV1
	reads atomic.Int32
}

func (s *rubricDriftStoreV1) InspectRubricCurrentV1(ctx context.Context, tenant core.TenantID, ref contract.ExactResourceRefV1, now time.Time) (contract.RubricDefinitionV1, error) {
	if s.reads.Add(1) == 4 {
		return contract.RubricDefinitionV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "injected Rubric S2 drift")
	}
	return s.ownerStoreV1.InspectRubricCurrentV1(ctx, tenant, ref, now)
}

func TestOwnerPostInvocationS2DriftHasZeroReviewResultV1(t *testing.T) {
	fixture := newOwnerFixtureV1(t, "s2-drift")
	store := &rubricDriftStoreV1{ownerStoreV1: fixture.store}
	clock := &manualClockV1{value: fixture.base.Add(30 * time.Second)}
	invocation := makeInvocationFakeV1(fixture)
	owner, _ := autoreviewer.New(store, invocation, clock.Now)
	got, err := owner.RunV1(context.Background(), fixture.command)
	if !core.HasCategory(err, core.ErrorConflict) || got.Attempt.State != contract.AutoReviewerAttemptWaitingInspectV1 {
		t.Fatalf("post-invocation S2 drift did not fail before Review result: %+v %v", got, err)
	}
	assertNoOwnerResultV1(t, fixture)
	if _, err := fixture.store.InspectAutoReviewerObservationExactV1(context.Background(), fixture.attempt.TenantID, contract.AutoReviewerInvocationObservationRefV1{ID: "owner-observation-" + fixture.attempt.ID, Revision: 1, Digest: testkit.Digest("not-the-observation")}); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("post-invocation S2 drift leaked Review Observation: %v", err)
	}
}

func TestOwnerKnownInvocationFailureTerminatesWithoutRetryV1(t *testing.T) {
	fixture := newOwnerFixtureV1(t, "known-failure")
	clock := &manualClockV1{value: fixture.base.Add(30 * time.Second)}
	invocation := makeInvocationFakeV1(fixture)
	invocation.startErr = core.NewError(core.ErrorForbidden, core.ReasonUnknownCapability, "injected authoritative rejection")
	owner, _ := autoreviewer.New(fixture.store, invocation, clock.Now)
	got, err := owner.RunV1(context.Background(), fixture.command)
	if !core.HasCategory(err, core.ErrorForbidden) || got.Attempt.State != contract.AutoReviewerAttemptFailedClosedV1 || invocation.startCalls != 1 || invocation.inspectCalls != 0 {
		t.Fatalf("known invocation failure did not terminate exactly once: %+v start=%d inspect=%d err=%v", got, invocation.startCalls, invocation.inspectCalls, err)
	}
	replayed, replayErr := owner.RunV1(context.Background(), fixture.command)
	if replayErr != nil || replayed.Attempt.Digest != got.Attempt.Digest || invocation.startCalls != 1 {
		t.Fatalf("terminated Attempt was re-invoked: %+v start=%d err=%v", replayed, invocation.startCalls, replayErr)
	}
}

func assertNoOwnerResultV1(t testing.TB, fixture ownerFixtureV1) {
	t.Helper()
	ref := reviewport.ExactV1(fixture.command.ResultID, 1, testkit.Digest("not-the-result"))
	if _, err := fixture.store.InspectDomainResultExactV1(context.Background(), fixture.attempt.TenantID, ref); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("failed Auto Reviewer path leaked DomainResult: %v", err)
	}
}

func TestOwnerConcurrentStartOrInspectHasOneLogicalProviderV1(t *testing.T) {
	fixture := newOwnerFixtureV1(t, "concurrent")
	clock := &manualClockV1{value: fixture.base.Add(30 * time.Second)}
	invocation := makeInvocationFakeV1(fixture)
	owner, _ := autoreviewer.New(fixture.store, invocation, clock.Now)
	var successes atomic.Int32
	var waiting atomic.Int32
	errorsSeen := make(chan error, 64)
	var wg sync.WaitGroup
	for index := 0; index < 64; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := owner.RunV1(context.Background(), fixture.command)
			if err == nil && result.Attempt.State == contract.AutoReviewerAttemptObservedV1 {
				successes.Add(1)
			} else if core.HasCategory(err, core.ErrorNotFound) && result.Attempt.State == contract.AutoReviewerAttemptWaitingInspectV1 {
				// A loser may Inspect after the persistent claim but before the
				// unique winner has crossed/sealed the external attempt. This is a
				// fail-closed transient, never permission to Start.
				waiting.Add(1)
			} else {
				errorsSeen <- fmt.Errorf("state=%s err=%w", result.Attempt.State, err)
			}
		}()
	}
	wg.Wait()
	close(errorsSeen)
	current, err := fixture.store.InspectAutoReviewerAttemptCurrentV1(context.Background(), fixture.attempt.TenantID, fixture.attempt.ID)
	if err != nil || current.State != contract.AutoReviewerAttemptObservedV1 || invocation.providerCalls != 1 || invocation.startCalls != 1 || successes.Load()+waiting.Load() != 64 {
		var first error
		for seen := range errorsSeen {
			if first == nil {
				first = seen
			}
		}
		t.Fatalf("concurrent owner flow violated the single-start fence: state=%s provider=%d start=%d successes=%d waiting=%d err=%v first=%v", current.State, invocation.providerCalls, invocation.startCalls, successes.Load(), waiting.Load(), err, first)
	}
	for index := 0; index < 64; index++ {
		replay, replayErr := owner.RunV1(context.Background(), fixture.command)
		if replayErr != nil || replay.Attempt.State != contract.AutoReviewerAttemptObservedV1 || replay.DomainResult == nil {
			t.Fatalf("post-closure replay %d did not converge by Review exact Inspect: %+v err=%v", index, replay, replayErr)
		}
	}
	if invocation.startCalls != 1 || invocation.providerCalls != 1 {
		t.Fatalf("post-closure replays crossed the external boundary: start=%d provider=%d", invocation.startCalls, invocation.providerCalls)
	}
}

type typedNilInvocationV1 struct{}

type typedNilSchemaReaderV1 struct{}

func (*typedNilSchemaReaderV1) InspectAutoReviewerOutputSchemaV1(context.Context, runtimeports.SchemaRefV2) (contract.AutoReviewerOutputSchemaDocumentV1, error) {
	return contract.AutoReviewerOutputSchemaDocumentV1{}, nil
}

func (*typedNilInvocationV1) StartOrInspectAutoReviewerInvocationV1(context.Context, reviewport.AutoReviewerInvocationCommandV1) (reviewport.AutoReviewerInvocationResultV1, error) {
	return reviewport.AutoReviewerInvocationResultV1{}, nil
}
func (*typedNilInvocationV1) InspectAutoReviewerInvocationV1(context.Context, contract.ExactResourceRefV1) (reviewport.AutoReviewerInvocationResultV1, error) {
	return reviewport.AutoReviewerInvocationResultV1{}, nil
}

func TestOwnerConstructorRejectsTypedNilDependenciesV1(t *testing.T) {
	fixture := newOwnerFixtureV1(t, "typed-nil")
	var nilStore *memory.Store
	var nilInvocation *typedNilInvocationV1
	if _, err := autoreviewer.New(nilStore, makeInvocationFakeV1(fixture), time.Now); !core.HasCategory(err, core.ErrorInvalidArgument) {
		t.Fatalf("typed-nil Store was accepted: %v", err)
	}
	if _, err := autoreviewer.New(fixture.store, nilInvocation, time.Now); !core.HasCategory(err, core.ErrorInvalidArgument) {
		t.Fatalf("typed-nil invocation Port was accepted: %v", err)
	}
	if _, err := autoreviewer.New(fixture.store, makeInvocationFakeV1(fixture), nil); !core.HasCategory(err, core.ErrorInvalidArgument) {
		t.Fatalf("nil clock was accepted: %v", err)
	}
	var nilContextReader *reviewerContextReaderFakeV1
	if _, err := autoreviewer.NewProduction(fixture.store, makeInvocationFakeV1(fixture), nilContextReader, outputSchemaReaderV1(t), time.Now); !core.HasCategory(err, core.ErrorInvalidArgument) {
		t.Fatalf("typed-nil Reviewer Context Reader was accepted: %v", err)
	}
	var nilSchemaReader *typedNilSchemaReaderV1
	if _, err := autoreviewer.NewProduction(fixture.store, makeInvocationFakeV1(fixture), &reviewerContextReaderFakeV1{}, nilSchemaReader, time.Now); !core.HasCategory(err, core.ErrorInvalidArgument) {
		t.Fatalf("typed-nil output Schema Reader was accepted: %v", err)
	}
}

func outputSchemaReaderV1(t *testing.T) reviewport.AutoReviewerOutputSchemaReaderV1 {
	t.Helper()
	reader, err := reviewer.NewBuiltinOutputSchemaReaderV1()
	if err != nil {
		t.Fatal(err)
	}
	return reader
}
