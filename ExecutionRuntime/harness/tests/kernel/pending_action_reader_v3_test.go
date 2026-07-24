package kernel_test

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/fakes"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/kernel"
	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestCommittedPendingActionReaderV3ExactCurrentAndNoAlias(t *testing.T) {
	fixture := newPendingActionReaderV3Fixture(t, pendingActionReaderV3Options{})
	reader := fixture.newReader(t)
	current, err := reader.InspectCommittedPendingActionCurrentV3(context.Background(), fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	if current.ExpiresUnixNano != fixture.now.Add(kernel.MaxCommittedPendingActionProjectionTTLV1).UnixNano() || fixture.sessionReads != 2 || fixture.contextReads != 1 {
		t.Fatalf("exact current sequence/TTL mismatch: expiry=%d session=%d context=%d", current.ExpiresUnixNano, fixture.sessionReads, fixture.contextReads)
	}
	unique := map[runtimeports.ProviderBindingRefV2]struct{}{}
	for _, ref := range fixture.roleRefs {
		unique[ref] = struct{}{}
	}
	if fixture.bindingReads != len(unique) {
		t.Fatalf("role-group reads=%d want=%d", fixture.bindingReads, len(unique))
	}
	current.PendingAction.Payload.Inline[0] ^= 1
	current.ApplicationBinding.Base.ModelTurnSettlementRef.Evidence[0].RecordDigest = testkit.Digest("mutated-output")
	if !reflect.DeepEqual(fixture.session, fixture.originalSession) {
		t.Fatal("returned Current aliased owner Session or nested Settlement")
	}
}

func TestGovernedSessionV4WaitingSettlementToActionWritesExactBindingAtomically(t *testing.T) {
	fixture := newPendingActionReaderV3Fixture(t, pendingActionReaderV3Options{})
	next := fixture.session.Clone()
	current := next.Clone()
	current.Revision--
	current.Phase = contract.SessionWaitingSettlementV2
	candidate := next.PendingAction.SourceCandidate
	current.Candidate = &candidate
	current.DomainReservation = &contract.ModelDispatchReservationRefV2{ID: "reservation-v4", Digest: testkit.Digest("reservation-v4"), AttemptID: "attempt-governed", IntentDigest: testkit.Digest("intent-v4"), CandidateDigest: candidate.Digest, ReservedUnixNano: fixture.now.Add(-time.Second).UnixNano(), ExpiresUnixNano: fixture.now.Add(time.Minute).UnixNano()}
	current.Execution.Settlement = nil
	current.PendingAction = nil
	current.ApplicationBinding = nil
	current.UpdatedUnixNano--
	current.Digest = ""
	var err error
	current, err = contract.SealGovernedSessionV4(current)
	mustNoErrorV3(t, err)
	mustNoErrorV3(t, contract.ValidateSessionTransitionV4(current, next))

	missingBinding := next.Clone()
	missingBinding.ApplicationBinding = nil
	missingBinding.Digest = ""
	if _, err := contract.SealGovernedSessionV4(missingBinding); err == nil {
		t.Fatal("waiting_action accepted PendingAction without the same-CAS BindingV2")
	}
	otherRun := next.Clone()
	otherRun.Run.RunID = "other-run"
	otherRun.Digest = ""
	if _, err := contract.SealGovernedSessionV4(otherRun); err == nil {
		t.Fatal("waiting_action accepted an exact BindingV2 owned by another Run")
	}
}

func TestGovernedSessionV4StoreDeepClonesNestedOwnerInputsAndSettlement(t *testing.T) {
	fixture := newPendingActionReaderV3Fixture(t, pendingActionReaderV3Options{})
	store := fakes.NewGovernedStoreV2()
	input := fixture.session.Clone()
	want := fixture.session.Clone()
	seedWaitingActionSessionV4(t, store, fixture, input)
	mutateGovernedSessionV4Deep(input)
	first, err := store.InspectSessionV4(context.Background(), want.Run, want.ID)
	if err != nil || !reflect.DeepEqual(first, want) {
		t.Fatalf("input mutation polluted V4 store: err=%v", err)
	}
	mutateGovernedSessionV4Deep(first)
	second, err := store.InspectSessionV4(context.Background(), want.Run, want.ID)
	if err != nil || !reflect.DeepEqual(second, want) {
		t.Fatalf("Inspect output mutation polluted V4 store: err=%v", err)
	}
}

func seedWaitingActionSessionV4(t *testing.T, store *fakes.GovernedStoreV2, fixture *pendingActionReaderV3Fixture, final contract.GovernedSessionV4) {
	t.Helper()
	creating, err := contract.SealGovernedSessionV4(contract.GovernedSessionV4{ID: final.ID, Revision: 1, Run: final.Run, Endpoint: final.Endpoint, Phase: contract.SessionCreatingV2, CreatedUnixNano: final.CreatedUnixNano, UpdatedUnixNano: final.CreatedUnixNano})
	mustNoErrorV3(t, err)
	if _, err := store.CreateSessionV4(context.Background(), creating); err != nil {
		t.Fatal(err)
	}
	candidateRef, err := fixture.candidate.RefV2()
	mustNoErrorV3(t, err)
	waitingDispatch := creating.Clone()
	waitingDispatch.Revision = 2
	waitingDispatch.Phase = contract.SessionWaitingModelDispatchV2
	waitingDispatch.Turn = 1
	waitingDispatch.Candidate = &candidateRef
	waitingDispatch.UpdatedUnixNano++
	waitingDispatch, err = contract.SealGovernedSessionV4(waitingDispatch)
	mustNoErrorV3(t, err)
	casSessionV4(t, store, creating, waitingDispatch)
	reservation := contract.ModelDispatchReservationRefV2{ID: "reservation-v4-seed", Digest: testkit.Digest("reservation-v4-seed"), AttemptID: final.Execution.AttemptID, IntentDigest: final.Execution.Admission.IntentDigest, CandidateDigest: candidateRef.Digest, ReservedUnixNano: final.CreatedUnixNano, ExpiresUnixNano: fixture.now.Add(time.Minute).UnixNano()}
	reserved := waitingDispatch.Clone()
	reserved.Revision = 3
	reserved.Phase = contract.SessionModelDispatchReservedV2
	reserved.DomainReservation = &reservation
	reserved.UpdatedUnixNano++
	reserved, err = contract.SealGovernedSessionV4(reserved)
	mustNoErrorV3(t, err)
	casSessionV4(t, store, waitingDispatch, reserved)
	prepared := *final.Execution
	prepared.Observation = nil
	prepared.Settlement = nil
	inflight := reserved.Clone()
	inflight.Revision = 4
	inflight.Phase = contract.SessionModelInFlightV2
	inflight.Execution = &prepared
	inflight.UpdatedUnixNano++
	inflight, err = contract.SealGovernedSessionV4(inflight)
	mustNoErrorV3(t, err)
	casSessionV4(t, store, reserved, inflight)
	observed := *final.Execution
	observed.Settlement = nil
	waitingSettlement := inflight.Clone()
	waitingSettlement.Revision = 5
	waitingSettlement.Phase = contract.SessionWaitingSettlementV2
	waitingSettlement.Execution = &observed
	waitingSettlement.UpdatedUnixNano++
	waitingSettlement, err = contract.SealGovernedSessionV4(waitingSettlement)
	mustNoErrorV3(t, err)
	casSessionV4(t, store, inflight, waitingSettlement)
	casSessionV4(t, store, waitingSettlement, final)
}

func casSessionV4(t *testing.T, store *fakes.GovernedStoreV2, current, next contract.GovernedSessionV4) {
	t.Helper()
	request, err := contract.SealSessionCASRequestV4(contract.SessionCASRequestV4{Run: current.Run, SessionID: current.ID, ExpectedRevision: current.Revision, ExpectedDigest: current.Digest, Next: next})
	mustNoErrorV3(t, err)
	if _, err := store.CompareAndSwapSessionV4(context.Background(), request); err != nil {
		t.Fatal(err)
	}
}

func mutateGovernedSessionV4Deep(value contract.GovernedSessionV4) {
	value.Run.Scope.SandboxLease.ID = "mutated-sandbox"
	value.Endpoint.Scope.SandboxLease.ID = "mutated-endpoint-sandbox"
	value.Execution.Settlement.Evidence[0].RecordDigest = testkit.Digest("mutated-evidence")
	value.Execution.Settlement.Attempt.Delegation.ID = "mutated-delegation"
	value.Execution.Settlement.DomainResultSchema.ContentDigest = testkit.Digest("mutated-schema")
	value.ApplicationBinding.Base.PendingAction.Payload.Inline[0] ^= 1
	value.ApplicationBinding.OwnerCurrentInputs.ModelTurnOperation.ExecutionScope.SandboxLease.ID = "mutated-owner-sandbox"
}

func TestCommittedPendingActionReaderV3UsesEveryNaturalTTLSourceMinimum(t *testing.T) {
	for name, options := range map[string]pendingActionReaderV3Options{
		"candidate":        {candidateTTL: 5 * time.Second},
		"identity":         {identityTTL: 6 * time.Second},
		"association":      {associationTTL: 7 * time.Second},
		"generation":       {generationTTL: 8 * time.Second},
		"route":            {routeTTL: 9 * time.Second},
		"provider-binding": {providerTTL: 10 * time.Second},
		"context":          {contextTTL: 11 * time.Second},
		"caller-bound":     {callerTTL: 12 * time.Second},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newPendingActionReaderV3Fixture(t, options)
			current, err := fixture.newReader(t).InspectCommittedPendingActionCurrentV3(context.Background(), fixture.request)
			if err != nil {
				t.Fatal(err)
			}
			want := fixture.now.Add(options.minimumTTL()).UnixNano()
			if current.ExpiresUnixNano != want {
				t.Fatalf("TTL minimum=%d want=%d", current.ExpiresUnixNano, want)
			}
		})
	}
}

func TestCommittedPendingActionReaderV3RejectsS1S2DriftRollbackAndTTLCrossing(t *testing.T) {
	for name, configure := range map[string]func(*pendingActionReaderV3Fixture){
		"s1-s2-drift": func(f *pendingActionReaderV3Fixture) {
			next := f.session.Clone()
			next.Revision++
			next.UpdatedUnixNano++
			next.Digest = ""
			sealed, err := contract.SealGovernedSessionV4(next)
			if err != nil {
				panic(err)
			}
			f.secondSession = &sealed
		},
		"clock-rollback": func(f *pendingActionReaderV3Fixture) { f.clockValues = []time.Time{f.now, f.now.Add(-time.Nanosecond)} },
		"ttl-crossing": func(f *pendingActionReaderV3Fixture) {
			f.clockValues = []time.Time{f.now, f.now.Add(kernel.MaxCommittedPendingActionProjectionTTLV1)}
		},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newPendingActionReaderV3Fixture(t, pendingActionReaderV3Options{})
			configure(fixture)
			if _, err := fixture.newReader(t).InspectCommittedPendingActionCurrentV3(context.Background(), fixture.request); err == nil {
				t.Fatal("drift/rollback/TTL crossing was accepted")
			}
		})
	}
}

func TestCommittedPendingActionReaderV3RejectsOwnerSplicesBeforeContextCurrent(t *testing.T) {
	type ownerSpliceCaseV3 struct {
		mutate func(*testing.T, *pendingActionReaderV3Fixture)
		assert func(*testing.T, *pendingActionReaderV3Fixture)
	}
	for name, test := range map[string]ownerSpliceCaseV3{
		"candidate-run": {mutate: func(t *testing.T, f *pendingActionReaderV3Fixture) {
			f.candidate.Run.RunID = "other-run"
			mustNoErrorV3(t, f.candidate.Validate(f.now))
		}, assert: func(t *testing.T, f *pendingActionReaderV3Fixture) {
			if f.candidateReads != 1 || f.factReads != 0 {
				t.Fatalf("candidate splice read sequence=%d/%d", f.candidateReads, f.factReads)
			}
		}},
		"fact-scope": {mutate: func(t *testing.T, f *pendingActionReaderV3Fixture) {
			source := f.fact.Identity.SourceKey
			source.ExecutionScopeDigest = testkit.Digest("other-scope")
			identity, err := contract.SealModelToolCallPendingActionIdentityV1(source, f.projection, f.fact.PendingAction, f.fact.Identity.CreatedUnixNano, f.fact.Identity.NotAfterUnixNano)
			mustNoErrorV3(t, err)
			fact, err := contract.SealSettledTurnDomainResultFactV3(identity, f.fact.PendingAction, f.fact.Schema, f.fact.CreatedUnixNano)
			mustNoErrorV3(t, err)
			f.rebindFact(t, fact)
		}, assert: func(t *testing.T, f *pendingActionReaderV3Fixture) {
			if f.factReads != 1 || f.modelReads != 0 {
				t.Fatalf("fact splice read sequence=%d/%d", f.factReads, f.modelReads)
			}
		}},
		"model-call": {mutate: func(t *testing.T, f *pendingActionReaderV3Fixture) {
			identity := f.fact.Identity.Clone()
			identity.CallID = "other-valid-call"
			identity.Digest = ""
			digest, err := identity.DigestV1()
			mustNoErrorV3(t, err)
			identity.Digest = digest
			mustNoErrorV3(t, identity.Validate())
			fact, err := contract.SealSettledTurnDomainResultFactV3(identity, f.fact.PendingAction, f.fact.Schema, f.fact.CreatedUnixNano)
			mustNoErrorV3(t, err)
			f.rebindFact(t, fact)
		}, assert: func(t *testing.T, f *pendingActionReaderV3Fixture) {
			if f.modelReads != 1 || f.settlementReads != 0 {
				t.Fatalf("model splice read sequence=%d/%d", f.modelReads, f.settlementReads)
			}
		}},
		"route-generation": {mutate: func(t *testing.T, f *pendingActionReaderV3Fixture) {
			route := f.route
			route.Generation.ID = "other-generation"
			route.Ref = runtimeports.ControlledOperationProviderRouteCurrentRefV2{Revision: f.route.Ref.Revision}
			route.ProjectionDigest = ""
			sealed, err := runtimeports.SealControlledOperationProviderRouteCurrentProjectionV2(route)
			mustNoErrorV3(t, err)
			mustNoErrorV3(t, sealed.ValidateCurrent(sealed.Ref, runtimeports.OperationScopeEvidenceActionMatrixV3(), f.now))
			f.rebindRoute(t, sealed)
		}, assert: func(t *testing.T, f *pendingActionReaderV3Fixture) {
			if f.routeReads != 1 || f.bindingReads != 0 {
				t.Fatalf("route splice read sequence=%d/%d", f.routeReads, f.bindingReads)
			}
		}},
		"provider-set-digest": {mutate: func(t *testing.T, f *pendingActionReaderV3Fixture) {
			for ref, projection := range f.bindingCurrents {
				projection.BindingSetDigest = testkit.Digest("other-set")
				projection.ProjectionDigest = ""
				var err error
				projection, err = runtimeports.SealProviderBindingCurrentProjectionV2(projection)
				mustNoErrorV3(t, err)
				mustNoErrorV3(t, projection.ValidateCurrent(ref, f.now))
				f.bindingCurrents[ref] = projection
				break
			}
		}, assert: func(t *testing.T, f *pendingActionReaderV3Fixture) {
			if f.bindingReads == 0 || f.contextReads != 0 {
				t.Fatalf("binding splice read sequence=%d/%d", f.bindingReads, f.contextReads)
			}
		}},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newPendingActionReaderV3Fixture(t, pendingActionReaderV3Options{})
			test.mutate(t, fixture)
			if _, err := fixture.newReader(t).InspectCommittedPendingActionCurrentV3(context.Background(), fixture.request); err == nil || !core.HasCategory(err, core.ErrorConflict) || fixture.contextReads != 0 {
				t.Fatalf("owner splice reached Context or succeeded: context=%d err=%v", fixture.contextReads, err)
			}
			if fixture.sessionReads != 1 || fixture.candidateReads == 0 {
				t.Fatalf("splice failed outside owner-read sequence: session=%d candidate=%d", fixture.sessionReads, fixture.candidateReads)
			}
			test.assert(t, fixture)
		})
	}
}

type pendingActionReaderV3Options struct {
	candidateTTL   time.Duration
	identityTTL    time.Duration
	associationTTL time.Duration
	generationTTL  time.Duration
	routeTTL       time.Duration
	providerTTL    time.Duration
	contextTTL     time.Duration
	callerTTL      time.Duration
}

func (o pendingActionReaderV3Options) withDefaults() pendingActionReaderV3Options {
	if o.candidateTTL == 0 {
		o.candidateTTL = time.Minute
	}
	if o.identityTTL == 0 {
		o.identityTTL = time.Minute
	}
	if o.associationTTL == 0 {
		o.associationTTL = 50 * time.Second
	}
	if o.generationTTL == 0 {
		o.generationTTL = 55 * time.Second
	}
	if o.routeTTL == 0 {
		o.routeTTL = 40 * time.Second
	}
	if o.providerTTL == 0 {
		o.providerTTL = 48 * time.Second
	}
	if o.contextTTL == 0 {
		o.contextTTL = 45 * time.Second
	}
	return o
}

func (o pendingActionReaderV3Options) minimumTTL() time.Duration {
	o = o.withDefaults()
	values := []time.Duration{kernel.MaxCommittedPendingActionProjectionTTLV1, o.candidateTTL, o.identityTTL, o.associationTTL, o.generationTTL, o.routeTTL, o.providerTTL, o.contextTTL}
	if o.callerTTL > 0 {
		values = append(values, o.callerTTL)
	}
	minimum := values[0]
	for _, value := range values[1:] {
		if value < minimum {
			minimum = value
		}
	}
	return minimum
}

type pendingActionReaderV3Fixture struct {
	now              time.Time
	clockValues      []time.Time
	clockIndex       int
	request          contract.CommittedPendingActionCurrentRequestV3
	session          contract.GovernedSessionV4
	originalSession  contract.GovernedSessionV4
	candidate        contract.ModelTurnCandidateV2
	fact             contract.SettledTurnDomainResultFactV3
	projection       modelinvoker.ToolCallCandidateObservationProjectionV1
	settlement       runtimeports.OperationSettlementRefV3
	association      runtimeports.GenerationBindingAssociationFactV1
	generation       runtimeports.GenerationCurrentProjectionV1
	route            runtimeports.ControlledOperationProviderRouteCurrentProjectionV2
	bindingCurrents  map[runtimeports.ProviderBindingRefV2]runtimeports.ProviderBindingCurrentProjectionV2
	roleRefs         []runtimeports.ProviderBindingRefV2
	contextCurrent   runtimeports.OperationScopeEvidenceApplicabilityCurrentProjectionV3
	sessionReads     int
	bindingReads     int
	contextReads     int
	candidateReads   int
	factReads        int
	modelReads       int
	settlementReads  int
	associationReads int
	generationReads  int
	routeReads       int
	secondSession    *contract.GovernedSessionV4
}

func newPendingActionReaderV3Fixture(t *testing.T, options pendingActionReaderV3Options) *pendingActionReaderV3Fixture {
	t.Helper()
	options = options.withDefaults()
	now := time.Unix(1_750_000_000, 0)
	baseSession, candidate := testkit.GovernedFactsV2(now)
	candidate.ExpiresUnixNano = now.Add(options.candidateTTL).UnixNano()
	candidateRef, err := candidate.RefV2()
	mustNoErrorV3(t, err)
	pending, err := contract.NewPendingActionV2("action-g6a", "praxis.tool/execute", candidate.Input, candidateRef)
	mustNoErrorV3(t, err)
	call := modelinvoker.FunctionCall{ID: "provider-call-1", Name: "lookup", Arguments: json.RawMessage(`{"input":"governed"}`)}
	observation, err := modelinvoker.FinalizeToolCallCandidateObservationV1(testkit.Digest("model-invocation"), modelinvoker.Response{ID: "response-g6a", Status: modelinvoker.ResponseStatusCompleted, StopReason: modelinvoker.StopReasonToolCall, Output: []modelinvoker.OutputItem{{Type: modelinvoker.OutputItemFunctionCall, FunctionCall: &call}}})
	mustNoErrorV3(t, err)
	projection, err := modelinvoker.NewToolCallCandidateObservationProjectionV1("model-execution-g6a", 1, "response-g6a", observation)
	mustNoErrorV3(t, err)
	scopeDigest, err := runtimeports.ExecutionScopeDigestV2(candidate.Run.Scope)
	mustNoErrorV3(t, err)
	source := contract.ModelToolCallPendingActionIdentitySourceKeyV1{ExecutionScopeDigest: scopeDigest, RunID: string(candidate.Run.RunID), SessionID: candidate.SessionRef, Turn: candidate.Turn, Candidate: candidateRef, ModelProjection: projection.Ref, CallOrdinal: contract.ModelToolCallOrdinalV1{EncodingVersion: contract.ModelToolCallOrdinalEncodingVersionV1, Present: true, Value: 0}, SettlementOwner: candidate.Provider}
	identity, err := contract.SealModelToolCallPendingActionIdentityV1(source, projection, pending, now.UnixNano(), now.Add(options.identityTTL).UnixNano())
	mustNoErrorV3(t, err)
	schema := runtimeports.SchemaRefV2{Namespace: contract.SettledTurnDomainResultSchemaNamespaceV3, Name: contract.SettledTurnDomainResultSchemaNameV3, Version: contract.SettledTurnDomainResultSchemaVersionV3, MediaType: "application/json", ContentDigest: testkit.Digest("settled-turn-v3-schema")}
	fact, err := contract.SealSettledTurnDomainResultFactV3(identity, pending, schema, now.UnixNano())
	mustNoErrorV3(t, err)
	factRef, err := fact.RefV3()
	mustNoErrorV3(t, err)
	identityRef, err := identity.RefV1(fact.ContentDigest)
	mustNoErrorV3(t, err)

	prepare, _, _, _ := testkit.GovernedProviderFixtureV2(now)
	operation := prepare.Intent.Operation
	operation.ExecutionScope = candidate.Run.Scope
	operation.ExecutionScopeDigest = scopeDigest
	operation.RunID = candidate.Run.RunID
	operation.ActivationAttemptID = ""
	operationDigest, err := operation.DigestV3()
	mustNoErrorV3(t, err)
	execution := testkit.GovernedAttemptRefsV2(now, candidate, runtimeports.ProviderAttemptObservedV2)
	execution.Admission.OperationDigest = operationDigest
	prepared := execution.Prepared
	prepared.OperationDigest = operationDigest
	prepared.Digest = ""
	execution.Prepared, err = runtimeports.SealPreparedProviderAttemptRefV2(prepared)
	mustNoErrorV3(t, err)
	execution.Enforcement.OperationDigest = operationDigest
	delegation := execution.Delegation
	observed := *execution.Observation
	settlement := runtimeports.OperationSettlementRefV3{ID: "settlement-g6a-v3", Revision: 1, Digest: testkit.Digest("settlement-g6a-v3"), Attempt: runtimeports.OperationDispatchAttemptRefV3{OperationDigest: operationDigest, EffectID: execution.Admission.EffectID, IntentRevision: execution.Admission.IntentRevision, IntentDigest: execution.Admission.IntentDigest, PermitID: execution.PermitID, PermitRevision: execution.PermitRevision, PermitDigest: execution.PermitDigest, AttemptID: execution.AttemptID, Delegation: &delegation}, Disposition: runtimeports.OperationSettlementAppliedV3, Owner: runtimeports.EffectOwnerRefV2{Role: runtimeports.OwnerSettlement, ComponentID: identity.SettlementOwner.ComponentID, ManifestDigest: identity.SettlementOwner.ManifestDigest}, Observation: &observed, Evidence: []runtimeports.EvidenceRecordRefV2{observed.Evidence}, DomainResultSchema: &fact.Schema, DomainResultDigest: fact.ContentDigest}
	mustNoErrorV3(t, settlement.Validate())
	execution.Settlement = &settlement
	mustNoErrorV3(t, execution.ValidatePrepared())

	generation, bindingSet, association, route, bindingCurrents, roleRefs := buildOwnerCurrentRuntimeV3(t, now, operation, baseSession.Endpoint.Binding, candidate.Provider, options)
	contextRef := runtimeports.OperationScopeEvidenceApplicabilityFactRefV3{Kind: runtimeports.OperationScopeEvidenceContextParentKindV3, ID: "context-parent-g6a", Revision: 1, Digest: testkit.Digest("context-parent-g6a")}
	contextCurrent := runtimeports.OperationScopeEvidenceApplicabilityCurrentProjectionV3{Fact: contextRef, ExecutionScopeDigest: scopeDigest, Current: true, ExpiresUnixNano: now.Add(options.contextTTL).UnixNano()}
	contextCurrent.Digest, err = core.CanonicalJSONDigest("praxis.runtime.operation-scope-evidence", runtimeports.OperationScopeEvidenceContractVersionV3, "OperationScopeEvidenceApplicabilityCurrentProjectionV3", contextCurrent)
	mustNoErrorV3(t, err)
	_ = bindingSet
	ownerInputs, err := contract.SealCommittedPendingActionOwnerCurrentInputsV1(contract.CommittedPendingActionOwnerCurrentInputsV1{ModelTurnOperation: operation, GenerationBindingAssociation: association.RefV1(), RouteCurrent: route.Ref, RouteMatrix: runtimeports.OperationScopeEvidenceActionMatrixV3(), ContextApplicability: contextRef})
	mustNoErrorV3(t, err)
	baseBinding := contract.PendingActionApplicationBindingV1{PendingAction: pending, IdentityRef: identityRef, DomainResultFactRef: factRef, ModelTurnSettlementRef: settlement}
	binding, err := contract.SealPendingActionApplicationBindingV2(contract.PendingActionApplicationBindingV2{Base: baseBinding, OwnerCurrentInputs: ownerInputs})
	mustNoErrorV3(t, err)
	session, err := contract.SealGovernedSessionV4(contract.GovernedSessionV4{ID: baseSession.ID, Revision: 6, Run: baseSession.Run, Endpoint: baseSession.Endpoint, Phase: contract.SessionWaitingActionV2, Turn: 1, Execution: &execution, PendingAction: &pending, ApplicationBinding: &binding, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.Add(time.Second).UnixNano()})
	mustNoErrorV3(t, err)
	subject := contract.CommittedPendingActionSubjectV3{Base: contract.CommittedPendingActionSubjectV2{ExecutionScopeDigest: scopeDigest, Run: session.Run, SessionID: session.ID, SessionRevision: session.Revision, SessionDigest: session.Digest, Turn: session.Turn, PendingActionRef: pending.Ref, IdentityRef: identityRef, DomainResultFactRef: factRef, ModelTurnSettlement: settlement}, ApplicationBinding: binding}
	request := contract.CommittedPendingActionCurrentRequestV3{Subject: subject}
	if options.callerTTL > 0 {
		request.RequestedNotAfterUnixNano = now.Add(options.callerTTL).UnixNano()
	}
	mustNoErrorV3(t, request.Validate(now))
	return &pendingActionReaderV3Fixture{now: now, clockValues: []time.Time{now, now.Add(time.Second)}, request: request, session: session, originalSession: session.Clone(), candidate: candidate, fact: fact, projection: projection, settlement: settlement, association: association, generation: generation, route: route, bindingCurrents: bindingCurrents, roleRefs: roleRefs, contextCurrent: contextCurrent}
}

func buildOwnerCurrentRuntimeV3(t *testing.T, now time.Time, modelOperation runtimeports.OperationSubjectV3, endpoint, candidateProvider runtimeports.ProviderBindingRefV2, options pendingActionReaderV3Options) (runtimeports.GenerationCurrentProjectionV1, runtimeports.GenerationBindingSetCurrentProjectionV1, runtimeports.GenerationBindingAssociationFactV1, runtimeports.ControlledOperationProviderRouteCurrentProjectionV2, map[runtimeports.ProviderBindingRefV2]runtimeports.ProviderBindingCurrentProjectionV2, []runtimeports.ProviderBindingRefV2) {
	t.Helper()
	generationRef := runtimeports.GenerationArtifactRefV1{ID: "generation-g6a", Revision: 1, Digest: testkit.Digest("generation"), InputDigest: testkit.Digest("generation-input"), ManifestDigest: testkit.Digest("generation-manifest"), GraphDigest: testkit.Digest("generation-graph"), CatalogDigest: testkit.Digest("generation-catalog")}
	components := []runtimeports.GenerationComponentManifestRefV1{{ComponentID: "praxis.harness/assembly", ManifestDigest: testkit.Digest("component-manifest"), ArtifactDigest: testkit.Digest("component-artifact")}}
	generation, err := runtimeports.SealGenerationCurrentProjectionV1(runtimeports.GenerationCurrentProjectionV1{Generation: generationRef, ComponentManifests: components, Extension: runtimeports.GenerationGovernanceExtensionRefV1{Kind: "praxis.harness/assembly-v1", Contract: runtimeports.SchemaRefV2{Namespace: "praxis.harness", Name: "assembly", Version: "1.0.0", MediaType: "application/json", ContentDigest: testkit.Digest("assembly-schema")}, Digest: testkit.Digest("extension")}, State: runtimeports.GenerationCurrentSealedV1, Current: true, Watermark: 1, ExpiresUnixNano: now.Add(options.generationTTL).UnixNano()})
	mustNoErrorV3(t, err)
	set, err := runtimeports.SealGenerationBindingSetCurrentProjectionV1(runtimeports.GenerationBindingSetCurrentProjectionV1{BindingSetID: endpoint.BindingSetID, BindingSetRevision: endpoint.BindingSetRevision, BindingSetDigest: testkit.Digest("binding-set"), BindingSetSemanticDigest: testkit.Digest("binding-semantic"), PlanDigest: testkit.Digest("plan"), GovernanceDigest: testkit.Digest("governance"), ComponentManifestSetDigest: runtimeports.GenerationComponentManifestSetDigestV1(components), CurrentnessDigest: testkit.Digest("binding-currentness"), IssuedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(options.associationTTL).UnixNano()})
	mustNoErrorV3(t, err)
	activationOperation := modelOperation
	activationOperation.Kind = runtimeports.OperationScopeActivationV3
	activationOperation.RunID = ""
	activationOperation.ActivationAttemptID = "activation-g6a"
	activationDigest, err := activationOperation.DigestV3()
	mustNoErrorV3(t, err)
	activation, err := runtimeports.SealGenerationActivationCurrentProjectionV1(runtimeports.GenerationActivationCurrentProjectionV1{Operation: activationOperation, OperationDigest: activationDigest, Active: true, Watermark: 1, CurrentnessDigest: testkit.Digest("activation-current"), ExpiresUnixNano: now.Add(options.associationTTL).UnixNano()})
	mustNoErrorV3(t, err)
	candidate, err := runtimeports.SealGenerationBindingAssociationCandidateV1(runtimeports.GenerationBindingAssociationCandidateV1{AssociationID: "association-g6a", Generation: generation, Binding: set, Activation: activation, RequestedExpiresUnixNano: now.Add(options.associationTTL).UnixNano()})
	mustNoErrorV3(t, err)
	associationExpiry := options.associationTTL
	if options.generationTTL < associationExpiry {
		associationExpiry = options.generationTTL
	}
	association, err := runtimeports.SealGenerationBindingAssociationFactV1(runtimeports.GenerationBindingAssociationFactV1{ID: candidate.AssociationID, Revision: 1, State: runtimeports.GenerationBindingAssociationActiveV1, Candidate: candidate, CandidateDigest: candidate.Digest, CreatedUnixNano: now.Add(-time.Second).UnixNano(), UpdatedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(associationExpiry).UnixNano()})
	mustNoErrorV3(t, err)

	declaration := runtimeports.ControlledOperationProviderRouteDeclarationRefV2{RouteID: "route-g6a", Revision: 1, PublisherComponentID: "praxis.harness/assembly", DeclarationDigest: testkit.Digest("route-declaration")}
	conformance := runtimeports.ControlledOperationProviderRouteConformanceRefV2{ConformanceID: "route-conformance-g6a", Revision: 1, DeclarationRef: declaration, ConformanceDigest: testkit.Digest("route-conformance")}
	role := func(component runtimeports.ComponentIDV2, capability runtimeports.CapabilityNameV2) runtimeports.ProviderBindingRefV2 {
		return runtimeports.ProviderBindingRefV2{BindingSetID: set.BindingSetID, BindingSetRevision: set.BindingSetRevision, ComponentID: component, ManifestDigest: testkit.Digest("manifest-" + string(component)), ArtifactDigest: testkit.Digest("artifact-" + string(component)), Capability: capability}
	}
	routeRefs := []runtimeports.ProviderBindingRefV2{role("praxis.tool/adapter", runtimeports.ControlledOperationToolAdapterCapabilityV2), role("praxis.runtime/gateway", runtimeports.ControlledOperationGatewayCapabilityV2), role("praxis.tool/transport", runtimeports.ControlledOperationProviderTransportCapabilityV2), role("praxis.runtime/prepared-reader", runtimeports.ControlledOperationPreparedReaderCapabilityV2), role("praxis.runtime/boundary-reader", runtimeports.ControlledOperationBoundaryReaderCapabilityV2), role("praxis.runtime/provider-inspect", runtimeports.ControlledOperationProviderInspectCapabilityV2), role("praxis.tool/provider", runtimeports.CapabilityNameV2(runtimeports.OperationScopeEvidenceActionEffectKindV3))}
	route, err := runtimeports.SealControlledOperationProviderRouteCurrentProjectionV2(runtimeports.ControlledOperationProviderRouteCurrentProjectionV2{Ref: runtimeports.ControlledOperationProviderRouteCurrentRefV2{Revision: 1}, DeclarationRef: declaration, ConformanceRef: conformance, Generation: generationRef, HandoffID: "handoff-g6a", HandoffRevision: 1, HandoffDigest: testkit.Digest("handoff"), BindingSetID: set.BindingSetID, BindingSetRevision: set.BindingSetRevision, BindingSetDigest: set.BindingSetDigest, BindingSetSemanticDigest: set.BindingSetSemanticDigest, BindingSetCurrentnessDigest: set.CurrentnessDigest, ActiveRouteID: "active-route-g6a", ActiveRouteRevision: 1, ActiveRouteDigest: testkit.Digest("active-route"), ToolAdapterBinding: routeRefs[0], GatewayBinding: routeRefs[1], ProviderTransportBinding: routeRefs[2], PreparedReaderBinding: routeRefs[3], BoundaryReaderBinding: routeRefs[4], ProviderInspectBinding: routeRefs[5], ProviderBinding: routeRefs[6], CheckedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(options.routeTTL).UnixNano()})
	mustNoErrorV3(t, err)
	all := append([]runtimeports.ProviderBindingRefV2{endpoint, candidateProvider, candidateProvider}, routeRefs...)
	currents := map[runtimeports.ProviderBindingRefV2]runtimeports.ProviderBindingCurrentProjectionV2{}
	for index, ref := range all {
		if _, exists := currents[ref]; exists {
			continue
		}
		projection, err := runtimeports.SealProviderBindingCurrentProjectionV2(runtimeports.ProviderBindingCurrentProjectionV2{ContractVersion: runtimeports.ProviderBindingCurrentnessContractVersionV2, Ref: ref, State: runtimeports.ProviderBindingCurrentActiveV2, BindingSetDigest: set.BindingSetDigest, BindingSetSemanticDigest: set.BindingSetSemanticDigest, BindingID: "binding-current-" + string(rune('a'+index)), BindingRevision: 1, GrantDigest: testkit.Digest("grant-" + string(rune('a'+index))), IssuedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(options.providerTTL).UnixNano()})
		mustNoErrorV3(t, err)
		currents[ref] = projection
	}
	return generation, set, association, route, currents, all
}

func (f *pendingActionReaderV3Fixture) newReader(t *testing.T) *kernel.CommittedPendingActionReaderV3 {
	t.Helper()
	reader, err := kernel.NewCommittedPendingActionReaderV3(f, f, f, f, f, f, f, f, f, f, f.clock)
	mustNoErrorV3(t, err)
	return reader
}

func (f *pendingActionReaderV3Fixture) clock() time.Time {
	if f.clockIndex >= len(f.clockValues) {
		return f.clockValues[len(f.clockValues)-1]
	}
	value := f.clockValues[f.clockIndex]
	f.clockIndex++
	return value
}

func (f *pendingActionReaderV3Fixture) rebindFact(t *testing.T, fact contract.SettledTurnDomainResultFactV3) {
	t.Helper()
	f.fact = fact.Clone()
	factRef, err := fact.RefV3()
	mustNoErrorV3(t, err)
	identityRef, err := fact.Identity.RefV1(fact.ContentDigest)
	mustNoErrorV3(t, err)
	f.settlement.DomainResultSchema = &fact.Schema
	f.settlement.DomainResultDigest = fact.ContentDigest
	base := f.session.ApplicationBinding.Base.Clone()
	base.IdentityRef = identityRef
	base.DomainResultFactRef = factRef
	base.ModelTurnSettlementRef = f.settlement
	binding, err := contract.SealPendingActionApplicationBindingV2(contract.PendingActionApplicationBindingV2{Base: base, OwnerCurrentInputs: f.session.ApplicationBinding.OwnerCurrentInputs})
	mustNoErrorV3(t, err)
	f.resealSessionAndRequest(t, binding)
}

func (f *pendingActionReaderV3Fixture) rebindRoute(t *testing.T, route runtimeports.ControlledOperationProviderRouteCurrentProjectionV2) {
	t.Helper()
	f.route = route
	inputs := f.session.ApplicationBinding.OwnerCurrentInputs.Clone()
	inputs.RouteCurrent = route.Ref
	inputs, err := contract.SealCommittedPendingActionOwnerCurrentInputsV1(inputs)
	mustNoErrorV3(t, err)
	binding, err := contract.SealPendingActionApplicationBindingV2(contract.PendingActionApplicationBindingV2{Base: f.session.ApplicationBinding.Base, OwnerCurrentInputs: inputs})
	mustNoErrorV3(t, err)
	f.resealSessionAndRequest(t, binding)
}

func (f *pendingActionReaderV3Fixture) resealSessionAndRequest(t *testing.T, binding contract.PendingActionApplicationBindingV2) {
	t.Helper()
	session := f.session.Clone()
	session.ApplicationBinding = &binding
	session.Execution.Settlement = &f.settlement
	session.Digest = ""
	sealed, err := contract.SealGovernedSessionV4(session)
	mustNoErrorV3(t, err)
	f.session = sealed
	f.originalSession = sealed.Clone()
	f.request.Subject.ApplicationBinding = binding
	f.request.Subject.Base.SessionDigest = sealed.Digest
	f.request.Subject.Base.IdentityRef = binding.Base.IdentityRef
	f.request.Subject.Base.DomainResultFactRef = binding.Base.DomainResultFactRef
	f.request.Subject.Base.ModelTurnSettlement = binding.Base.ModelTurnSettlementRef
	mustNoErrorV3(t, f.request.Validate(f.now))
}

func (f *pendingActionReaderV3Fixture) CreateSessionV4(context.Context, contract.GovernedSessionV4) (contract.GovernedSessionV4, error) {
	panic("write forbidden")
}
func (f *pendingActionReaderV3Fixture) CompareAndSwapSessionV4(context.Context, contract.SessionCASRequestV4) (contract.GovernedSessionV4, error) {
	panic("write forbidden")
}
func (f *pendingActionReaderV3Fixture) InspectSessionV4(context.Context, contract.RunRef, string) (contract.GovernedSessionV4, error) {
	f.sessionReads++
	if f.sessionReads == 2 && f.secondSession != nil {
		return f.secondSession.Clone(), nil
	}
	return f.session.Clone(), nil
}
func (f *pendingActionReaderV3Fixture) CreateCandidateV2(context.Context, contract.ModelTurnCandidateV2) (contract.ModelTurnCandidateV2, error) {
	panic("write forbidden")
}
func (f *pendingActionReaderV3Fixture) InspectCandidateV2(context.Context, contract.RunRef, string) (contract.ModelTurnCandidateV2, error) {
	f.candidateReads++
	return f.candidate, nil
}
func (f *pendingActionReaderV3Fixture) InspectExact(context.Context, contract.SettledTurnDomainResultFactRefV3) (contract.SettledTurnDomainResultFactV3, error) {
	f.factReads++
	return f.fact.Clone(), nil
}
func (f *pendingActionReaderV3Fixture) InspectExactProjectionV1(context.Context, modelinvoker.ToolCallCandidateObservationRefV1) (modelinvoker.ToolCallCandidateObservationProjectionV1, error) {
	f.modelReads++
	return f.projection.Clone(), nil
}
func (f *pendingActionReaderV3Fixture) InspectOperationSettlementV3(context.Context, runtimeports.OperationSubjectV3, core.EffectIntentID) (runtimeports.OperationSettlementRefV3, error) {
	f.settlementReads++
	return f.settlement, nil
}
func (f *pendingActionReaderV3Fixture) AssociateGenerationBindingV1(context.Context, runtimeports.GenerationBindingAssociationCandidateV1) (runtimeports.GenerationBindingAssociationFactV1, error) {
	panic("write forbidden")
}
func (f *pendingActionReaderV3Fixture) InspectCurrentGenerationBindingAssociationV1(context.Context, string) (runtimeports.GenerationBindingAssociationFactV1, error) {
	f.associationReads++
	return f.association, nil
}
func (f *pendingActionReaderV3Fixture) InspectGenerationCurrentV1(context.Context, runtimeports.GenerationArtifactRefV1) (runtimeports.GenerationCurrentProjectionV1, error) {
	f.generationReads++
	return f.generation, nil
}
func (f *pendingActionReaderV3Fixture) InspectCurrentControlledOperationProviderRouteV2(context.Context, runtimeports.ControlledOperationProviderRouteCurrentRefV2, runtimeports.OperationScopeEvidenceApplicabilityMatrixKeyV3) (runtimeports.ControlledOperationProviderRouteCurrentProjectionV2, error) {
	f.routeReads++
	return f.route, nil
}
func (f *pendingActionReaderV3Fixture) InspectProviderBindingCurrentV2(_ context.Context, ref runtimeports.ProviderBindingRefV2) (runtimeports.ProviderBindingCurrentProjectionV2, error) {
	f.bindingReads++
	return f.bindingCurrents[ref], nil
}
func (f *pendingActionReaderV3Fixture) InspectOperationScopeEvidenceApplicabilityCurrentV3(context.Context, runtimeports.OperationScopeEvidenceApplicabilityFactRefV3) (runtimeports.OperationScopeEvidenceApplicabilityCurrentProjectionV3, error) {
	f.contextReads++
	return f.contextCurrent, nil
}

func mustNoErrorV3(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
