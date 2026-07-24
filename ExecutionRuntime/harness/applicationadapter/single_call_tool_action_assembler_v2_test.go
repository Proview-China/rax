package applicationadapter

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	harnesscontract "github.com/Proview-China/rax/ExecutionRuntime/harness/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/internal/testkit"
	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestSingleCallToolActionAssemblerV2ExactS1S2AndSingleSeal(t *testing.T) {
	fixture := newAssemblerV2Fixture(t)
	assembler := fixture.newAssembler(t)

	request, err := assembler.AssembleSingleCallToolActionRequestV2(context.Background(), fixture.input)
	if err != nil {
		t.Fatal(err)
	}
	if err := request.ValidateCurrent(fixture.clockValues[2]); err != nil {
		t.Fatal(err)
	}
	if request.CreatedUnixNano != fixture.clockValues[2].UnixNano() || request.ExpiresUnixNano != fixture.authority.ExpiresUnixNano {
		t.Fatalf("request time window=%d..%d", request.CreatedUnixNano, request.ExpiresUnixNano)
	}
	if fixture.sessionReads != 2 || fixture.factReads != 2 || fixture.modelReads != 2 || fixture.currentReads != 2 || fixture.authorityReads != 2 || fixture.clockIndex != 3 {
		t.Fatalf("S1/S2 reads session=%d fact=%d model=%d current=%d authority=%d clock=%d", fixture.sessionReads, fixture.factReads, fixture.modelReads, fixture.currentReads, fixture.authorityReads, fixture.clockIndex)
	}
	if request.Action.Digest != fixture.input.Action.Digest || request.Authority != fixture.input.Authority {
		t.Fatal("sealed Request changed the exact Action or Authority subject")
	}
}

func TestSingleCallToolActionAssemblerV2ProjectionBytesAreDeepCopied(t *testing.T) {
	fixture := newAssemblerV2Fixture(t)
	original := append([]byte(nil), fixture.projection.Observation.Calls[0].CanonicalArguments...)
	proof, _, err := validateIdentityProjectionV2(fixture.fact, fixture.projection, *fixture.session.PendingAction, fixture.now)
	if err != nil {
		t.Fatal(err)
	}
	fixture.projection.Observation.Calls[0].CanonicalArguments[0] ^= 1
	if !reflect.DeepEqual(proof.CanonicalArguments, original) {
		t.Fatal("sealed neutral ProjectionProof aliases Model reader bytes")
	}
	proof.CanonicalArguments[0] ^= 1
	if reflect.DeepEqual(proof.CanonicalArguments, original) || fixture.fact.Identity.CanonicalArgumentsDigest != core.DigestBytes(original) {
		t.Fatal("ProjectionProof output mutation polluted the fact identity")
	}
}

func TestSingleCallToolActionAssemblerV2ImplementsExactInputCurrentReader(t *testing.T) {
	fixture := newAssemblerV2Fixture(t)
	request := fixture.assembleAndResetForInputCurrent(t)
	reader := fixture.newAssembler(t)

	projection, err := reader.InspectSingleCallToolActionInputCurrentV2(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	checked := fixture.clockValues[2]
	if err := projection.ValidateFor(request, checked); err != nil {
		t.Fatal(err)
	}
	if projection.CheckedUnixNano != checked.UnixNano() || projection.ExpiresUnixNano != request.ExpiresUnixNano {
		t.Fatalf("input-current time window=%d..%d", projection.CheckedUnixNano, projection.ExpiresUnixNano)
	}
	if projection.HarnessCurrent.CheckedUnixNano != checked.UnixNano() || projection.HarnessCurrent.IdentityCurrent.CheckedUnixNano != checked.UnixNano() || projection.AuthorityCurrent.CheckedUnixNano != checked.UnixNano() {
		t.Fatal("child proof observation time was recorded before S2 completed")
	}
	if fixture.sessionReads != 2 || fixture.factReads != 2 || fixture.modelReads != 2 || fixture.currentReads != 2 || fixture.authorityReads != 2 || fixture.clockIndex != 3 {
		t.Fatalf("input-current reads session=%d fact=%d model=%d current=%d authority=%d clock=%d", fixture.sessionReads, fixture.factReads, fixture.modelReads, fixture.currentReads, fixture.authorityReads, fixture.clockIndex)
	}
	want := append([]byte(nil), fixture.projection.Observation.Calls[0].CanonicalArguments...)
	if !bytes.Equal(projection.HarnessCurrent.IdentityCurrent.Projection.CanonicalArguments, want) {
		t.Fatal("input-current lost exact Model canonical arguments")
	}
	projection.HarnessCurrent.IdentityCurrent.Projection.CanonicalArguments[0] ^= 1
	if !bytes.Equal(fixture.projection.Observation.Calls[0].CanonicalArguments, want) {
		t.Fatalf("returned input-current proof aliases Model Owner bytes: got=%q want=%q", fixture.projection.Observation.Calls[0].CanonicalArguments, want)
	}
}

func TestSingleCallToolActionInputCurrentReaderV2RejectsS1S2OwnerDrift(t *testing.T) {
	tests := map[string]func(*testing.T, *assemblerV2Fixture){
		"session": func(t *testing.T, f *assemblerV2Fixture) {
			next := f.session.Clone()
			next.UpdatedUnixNano++
			next.Digest = ""
			sealed, err := harnesscontract.SealGovernedSessionV4(next)
			mustAssemblerV2(t, err)
			f.secondSession = &sealed
		},
		"model": func(t *testing.T, f *assemblerV2Fixture) {
			call := modelinvoker.FunctionCall{ID: "provider-call-2", Name: "lookup", Arguments: json.RawMessage(`{"input":"governed"}`)}
			observation, err := modelinvoker.FinalizeToolCallCandidateObservationV1(testkit.Digest("other-invocation"), modelinvoker.Response{ID: "other-response", Status: modelinvoker.ResponseStatusCompleted, StopReason: modelinvoker.StopReasonToolCall, Output: []modelinvoker.OutputItem{{Type: modelinvoker.OutputItemFunctionCall, FunctionCall: &call}}})
			mustAssemblerV2(t, err)
			projection, err := modelinvoker.NewToolCallCandidateObservationProjectionV1("other-execution", 1, "other-response", observation)
			mustAssemblerV2(t, err)
			f.secondProjection = &projection
		},
		"authority": func(_ *testing.T, f *assemblerV2Fixture) {
			changed := f.authority
			changed.ExpiresUnixNano--
			f.secondAuthority = &changed
		},
	}
	for name, configure := range tests {
		t.Run(name, func(t *testing.T) {
			fixture := newAssemblerV2Fixture(t)
			request := fixture.assembleAndResetForInputCurrent(t)
			configure(t, fixture)
			if _, err := fixture.newAssembler(t).InspectSingleCallToolActionInputCurrentV2(context.Background(), request); err == nil {
				t.Fatal("input-current S1/S2 Owner drift was accepted")
			}
		})
	}
}

func TestSingleCallToolActionInputCurrentReaderV2RejectsRollbackAndExpiredRequest(t *testing.T) {
	fixture := newAssemblerV2Fixture(t)
	request := fixture.assembleAndResetForInputCurrent(t)
	fixture.clockValues = []time.Time{fixture.now.Add(3 * time.Second), fixture.now.Add(2 * time.Second), fixture.now.Add(4 * time.Second)}
	if _, err := fixture.newAssembler(t).InspectSingleCallToolActionInputCurrentV2(context.Background(), request); err == nil {
		t.Fatal("input-current accepted a clock rollback")
	}

	fixture = newAssemblerV2Fixture(t)
	request = fixture.assembleAndResetForInputCurrent(t)
	fixture.clockValues = []time.Time{time.Unix(0, request.ExpiresUnixNano), time.Unix(0, request.ExpiresUnixNano), time.Unix(0, request.ExpiresUnixNano)}
	if _, err := fixture.newAssembler(t).InspectSingleCallToolActionInputCurrentV2(context.Background(), request); err == nil {
		t.Fatal("input-current accepted an expired sealed Request")
	}
}

func TestSingleCallToolActionInputCurrentReaderV2RejectsCurrentExpiryExpansion(t *testing.T) {
	fixture := newAssemblerV2Fixture(t)
	request := fixture.assembleAndResetForInputCurrent(t)
	changed := fixture.current.Clone()
	expected := harnesscontract.CommittedPendingActionCurrentRequestV3{Subject: changed.SubjectV3(), RequestedNotAfterUnixNano: request.ExpiresUnixNano}
	// The current projection is internally valid and belongs to the same
	// subject, but its observation lease expands between S1 and S2.
	changed.ExpiresUnixNano = request.ExpiresUnixNano - 2*int64(time.Second)
	changed.Digest = ""
	first, err := harnesscontract.SealCommittedPendingActionCurrentV3(changed, expected, fixture.now)
	mustAssemblerV2(t, err)
	fixture.current = first
	changed.ExpiresUnixNano = request.ExpiresUnixNano - int64(time.Second)
	changed.Digest = ""
	second, err := harnesscontract.SealCommittedPendingActionCurrentV3(changed, expected, fixture.now)
	mustAssemblerV2(t, err)
	fixture.secondCurrent = &second
	if _, err := fixture.newAssembler(t).InspectSingleCallToolActionInputCurrentV2(context.Background(), request); err == nil {
		t.Fatal("input-current accepted a widened S2 Current V3 lease")
	}
}

func TestSingleCallToolActionAssemblerV2RejectsOwnerDrift(t *testing.T) {
	tests := map[string]func(*testing.T, *assemblerV2Fixture){
		"session": func(t *testing.T, f *assemblerV2Fixture) {
			next := f.session.Clone()
			next.UpdatedUnixNano++
			next.Digest = ""
			sealed, err := harnesscontract.SealGovernedSessionV4(next)
			mustAssemblerV2(t, err)
			f.secondSession = &sealed
		},
		"domain-result": func(t *testing.T, f *assemblerV2Fixture) {
			changedIdentity, err := harnesscontract.SealModelToolCallPendingActionIdentityV1(f.fact.Identity.SourceKey, f.projection, *f.session.PendingAction, f.fact.CreatedUnixNano+1, f.fact.Identity.NotAfterUnixNano)
			mustAssemblerV2(t, err)
			changed, err := harnesscontract.SealSettledTurnDomainResultFactV3(changedIdentity, *f.session.PendingAction, f.fact.Schema, changedIdentity.CreatedUnixNano)
			mustAssemblerV2(t, err)
			f.secondFact = &changed
		},
		"model-projection": func(t *testing.T, f *assemblerV2Fixture) {
			call := modelinvoker.FunctionCall{ID: "provider-call-2", Name: "lookup", Arguments: json.RawMessage(`{"input":"governed"}`)}
			observation, err := modelinvoker.FinalizeToolCallCandidateObservationV1(testkit.Digest("other-invocation"), modelinvoker.Response{ID: "other-response", Status: modelinvoker.ResponseStatusCompleted, StopReason: modelinvoker.StopReasonToolCall, Output: []modelinvoker.OutputItem{{Type: modelinvoker.OutputItemFunctionCall, FunctionCall: &call}}})
			mustAssemblerV2(t, err)
			projection, err := modelinvoker.NewToolCallCandidateObservationProjectionV1("other-execution", 1, "other-response", observation)
			mustAssemblerV2(t, err)
			f.secondProjection = &projection
		},
		"current": func(t *testing.T, f *assemblerV2Fixture) {
			changed := f.current.Clone()
			changed.SessionRevision++
			changed.Digest = testkit.Digest("other-current")
			f.secondCurrent = &changed
		},
		"authority": func(_ *testing.T, f *assemblerV2Fixture) {
			changed := f.authority
			changed.ExpiresUnixNano--
			f.secondAuthority = &changed
		},
	}
	for name, configure := range tests {
		t.Run(name, func(t *testing.T) {
			fixture := newAssemblerV2Fixture(t)
			configure(t, fixture)
			_, err := fixture.newAssembler(t).AssembleSingleCallToolActionRequestV2(context.Background(), fixture.input)
			if err == nil {
				t.Fatal("S1/S2 owner drift was accepted")
			}
		})
	}
}

func TestSingleCallToolActionAssemblerV2RejectsClockRollbackAndTTLCrossing(t *testing.T) {
	for name, clocks := range map[string][]time.Time{
		"before-s2-rollback": {time.Unix(1_750_000_000, 0), time.Unix(1_749_999_999, 0), time.Unix(1_750_000_001, 0)},
		"after-s2-rollback":  {time.Unix(1_750_000_000, 0), time.Unix(1_750_000_001, 0), time.Unix(1_750_000_000, 0)},
		"ttl-crossing":       {time.Unix(1_750_000_000, 0), time.Unix(1_750_000_001, 0), time.Unix(1_750_000_020, 0)},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newAssemblerV2Fixture(t)
			fixture.clockValues = clocks
			if _, err := fixture.newAssembler(t).AssembleSingleCallToolActionRequestV2(context.Background(), fixture.input); err == nil {
				t.Fatal("clock rollback or TTL crossing was accepted")
			}
		})
	}
}

func TestSingleCallToolActionAssemblerV2RejectsCurrentV3ExpiryExpansionAndCheckedRollback(t *testing.T) {
	for name, mutate := range map[string]func(*testing.T, *assemblerV2Fixture, *harnesscontract.CommittedPendingActionCurrentV3){
		"expiry-expansion": func(_ *testing.T, f *assemblerV2Fixture, current *harnesscontract.CommittedPendingActionCurrentV3) {
			current.ExpiresUnixNano = f.current.ExpiresUnixNano + int64(time.Second)
		},
		"checked-rollback": func(_ *testing.T, f *assemblerV2Fixture, current *harnesscontract.CommittedPendingActionCurrentV3) {
			current.CheckedUnixNano = f.current.CheckedUnixNano - int64(time.Second)
		},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newAssemblerV2Fixture(t)
			changed := fixture.current.Clone()
			mutate(t, fixture, &changed)
			changed.Digest = ""
			expected := harnesscontract.CommittedPendingActionCurrentRequestV3{Subject: changed.SubjectV3()}
			sealed, err := harnesscontract.SealCommittedPendingActionCurrentV3(changed, expected, fixture.now)
			mustAssemblerV2(t, err)
			fixture.secondCurrent = &sealed
			if _, err := fixture.newAssembler(t).AssembleSingleCallToolActionRequestV2(context.Background(), fixture.input); err == nil {
				t.Fatal("Current V3 observation envelope regression was accepted")
			}
		})
	}
}

func TestSingleCallToolActionAssemblerV2RejectsNilContextBeforeClockOrOwnerReads(t *testing.T) {
	fixture := newAssemblerV2Fixture(t)
	assembler := fixture.newAssembler(t)
	if _, err := assembler.AssembleSingleCallToolActionRequestV2(nil, fixture.input); err == nil {
		t.Fatal("assembler accepted nil context")
	}
	if fixture.totalReads() != 0 || fixture.clockIndex != 0 {
		t.Fatalf("nil context touched clock/Owners: reads=%d clock=%d", fixture.totalReads(), fixture.clockIndex)
	}

	request := fixture.assembleAndResetForInputCurrent(t)
	if _, err := assembler.InspectSingleCallToolActionInputCurrentV2(nil, request); err == nil {
		t.Fatal("input-current reader accepted nil context")
	}
	if fixture.totalReads() != 0 || fixture.clockIndex != 0 {
		t.Fatalf("nil context touched input-current clock/Owners: reads=%d clock=%d", fixture.totalReads(), fixture.clockIndex)
	}
}

func TestSingleCallToolActionAssemblerV2ConstructorRejectsEveryTypedNilDependency(t *testing.T) {
	valid := newAssemblerV2Fixture(t)
	var typedNil *assemblerV2Fixture
	for name, construct := range map[string]func() error{
		"session": func() error {
			_, err := NewSingleCallToolActionAssemblerV2(typedNil, valid, valid, valid, valid, valid.clock)
			return err
		},
		"current": func() error {
			_, err := NewSingleCallToolActionAssemblerV2(valid, typedNil, valid, valid, valid, valid.clock)
			return err
		},
		"domain-result": func() error {
			_, err := NewSingleCallToolActionAssemblerV2(valid, valid, typedNil, valid, valid, valid.clock)
			return err
		},
		"model": func() error {
			_, err := NewSingleCallToolActionAssemblerV2(valid, valid, valid, typedNil, valid, valid.clock)
			return err
		},
		"authority": func() error {
			_, err := NewSingleCallToolActionAssemblerV2(valid, valid, valid, valid, typedNil, valid.clock)
			return err
		},
		"clock": func() error {
			_, err := NewSingleCallToolActionAssemblerV2(valid, valid, valid, valid, valid, nil)
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			err := construct()
			if !core.HasCategory(err, core.ErrorUnavailable) || !core.HasReason(err, core.ReasonComponentMissing) {
				t.Fatalf("typed nil error=%v", err)
			}
			if valid.totalReads() != 0 {
				t.Fatalf("constructor performed Owner reads: %d", valid.totalReads())
			}
		})
	}
}

func TestSingleCallToolActionAssemblerV2NilReceiverFailsClosed(t *testing.T) {
	fixture := newAssemblerV2Fixture(t)
	var assembler *SingleCallToolActionAssemblerV2
	if _, err := assembler.AssembleSingleCallToolActionRequestV2(context.Background(), fixture.input); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("nil assembler error=%v", err)
	}
	request := fixture.assembleAndResetForInputCurrent(t)
	if _, err := assembler.InspectSingleCallToolActionInputCurrentV2(context.Background(), request); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("nil input-current reader error=%v", err)
	}
}

func TestSingleCallToolActionAssemblerV2UsesNarrowedS2CurrentExpiry(t *testing.T) {
	fixture := newAssemblerV2Fixture(t)
	fixture.secondCurrent = fixture.sealCurrentWithExpiry(t, fixture.now.Add(10*time.Second).UnixNano(), 0)
	request, err := fixture.newAssembler(t).AssembleSingleCallToolActionRequestV2(context.Background(), fixture.input)
	if err != nil {
		t.Fatal(err)
	}
	if request.ExpiresUnixNano != fixture.secondCurrent.ExpiresUnixNano {
		t.Fatalf("Request expiry=%d want narrowed S2=%d", request.ExpiresUnixNano, fixture.secondCurrent.ExpiresUnixNano)
	}

	fixture = newAssemblerV2Fixture(t)
	request = fixture.assembleAndResetForInputCurrent(t)
	fixture.current = *fixture.sealCurrentWithExpiry(t, fixture.now.Add(12*time.Second).UnixNano(), request.ExpiresUnixNano)
	fixture.secondCurrent = fixture.sealCurrentWithExpiry(t, fixture.now.Add(10*time.Second).UnixNano(), request.ExpiresUnixNano)
	projection, err := fixture.newAssembler(t).InspectSingleCallToolActionInputCurrentV2(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if projection.ExpiresUnixNano != fixture.secondCurrent.ExpiresUnixNano || projection.HarnessCurrent.ExpiresUnixNano != fixture.secondCurrent.ExpiresUnixNano {
		t.Fatalf("InputCurrent expiry=%d harness=%d want narrowed S2=%d", projection.ExpiresUnixNano, projection.HarnessCurrent.ExpiresUnixNano, fixture.secondCurrent.ExpiresUnixNano)
	}
}

func TestSingleCallToolActionAssemblerV2RequestedNotAfterThreeStates(t *testing.T) {
	negative := newAssemblerV2Fixture(t)
	negative.input.RequestedNotAfterUnixNano = -1
	if _, err := negative.newAssembler(t).AssembleSingleCallToolActionRequestV2(context.Background(), negative.input); err == nil {
		t.Fatal("negative RequestedNotAfter was accepted")
	}
	if negative.totalReads() != 0 || negative.clockIndex != 0 {
		t.Fatal("negative RequestedNotAfter reached clock or Owner readers")
	}

	zero := newAssemblerV2Fixture(t)
	request, err := zero.newAssembler(t).AssembleSingleCallToolActionRequestV2(context.Background(), zero.input)
	if err != nil {
		t.Fatal(err)
	}
	if request.ExpiresUnixNano != zero.authority.ExpiresUnixNano {
		t.Fatalf("zero RequestedNotAfter changed natural minimum: %d", request.ExpiresUnixNano)
	}

	positive := newAssemblerV2Fixture(t)
	bound := positive.now.Add(8 * time.Second).UnixNano()
	positive.input.RequestedNotAfterUnixNano = bound
	request, err = positive.newAssembler(t).AssembleSingleCallToolActionRequestV2(context.Background(), positive.input)
	if err != nil {
		t.Fatal(err)
	}
	if request.ExpiresUnixNano != bound {
		t.Fatalf("positive RequestedNotAfter did not shorten expiry: %d want %d", request.ExpiresUnixNano, bound)
	}
}

type assemblerV2Fixture struct {
	now              time.Time
	clockValues      []time.Time
	clockIndex       int
	input            applicationcontract.AssembleSingleCallToolActionRequestV2
	session          harnesscontract.GovernedSessionV4
	fact             harnesscontract.SettledTurnDomainResultFactV3
	projection       modelinvoker.ToolCallCandidateObservationProjectionV1
	current          harnesscontract.CommittedPendingActionCurrentV3
	authority        runtimeports.DispatchAuthorityFactV2
	secondSession    *harnesscontract.GovernedSessionV4
	secondFact       *harnesscontract.SettledTurnDomainResultFactV3
	secondProjection *modelinvoker.ToolCallCandidateObservationProjectionV1
	secondCurrent    *harnesscontract.CommittedPendingActionCurrentV3
	secondAuthority  *runtimeports.DispatchAuthorityFactV2
	sessionReads     int
	factReads        int
	modelReads       int
	currentReads     int
	authorityReads   int
}

func newAssemblerV2Fixture(t *testing.T) *assemblerV2Fixture {
	t.Helper()
	now := time.Unix(1_750_000_000, 0)
	base, candidate := testkit.GovernedFactsV2(now)
	candidateRef, err := candidate.RefV2()
	mustAssemblerV2(t, err)
	pending, err := harnesscontract.NewPendingActionV2("action-g6a", "praxis.tool/execute", candidate.Input, candidateRef)
	mustAssemblerV2(t, err)
	call := modelinvoker.FunctionCall{ID: "provider-call-1", Name: "lookup", Arguments: json.RawMessage(`{"input":"governed"}`)}
	observation, err := modelinvoker.FinalizeToolCallCandidateObservationV1(testkit.Digest("model-invocation"), modelinvoker.Response{ID: "response-g6a", Status: modelinvoker.ResponseStatusCompleted, StopReason: modelinvoker.StopReasonToolCall, Output: []modelinvoker.OutputItem{{Type: modelinvoker.OutputItemFunctionCall, FunctionCall: &call}}})
	mustAssemblerV2(t, err)
	projection, err := modelinvoker.NewToolCallCandidateObservationProjectionV1("model-execution-g6a", 1, "response-g6a", observation)
	mustAssemblerV2(t, err)
	scopeDigest, err := runtimeports.ExecutionScopeDigestV2(candidate.Run.Scope)
	mustAssemblerV2(t, err)
	source := harnesscontract.ModelToolCallPendingActionIdentitySourceKeyV1{ExecutionScopeDigest: scopeDigest, RunID: string(candidate.Run.RunID), SessionID: candidate.SessionRef, Turn: candidate.Turn, Candidate: candidateRef, ModelProjection: projection.Ref, CallOrdinal: harnesscontract.ModelToolCallOrdinalV1{EncodingVersion: harnesscontract.ModelToolCallOrdinalEncodingVersionV1, Present: true, Value: 0}, SettlementOwner: candidate.Provider}
	identity, err := harnesscontract.SealModelToolCallPendingActionIdentityV1(source, projection, pending, now.UnixNano(), now.Add(25*time.Second).UnixNano())
	mustAssemblerV2(t, err)
	schema := runtimeports.SchemaRefV2{Namespace: harnesscontract.SettledTurnDomainResultSchemaNamespaceV3, Name: harnesscontract.SettledTurnDomainResultSchemaNameV3, Version: harnesscontract.SettledTurnDomainResultSchemaVersionV3, MediaType: "application/json", ContentDigest: testkit.Digest("settled-turn-schema")}
	fact, err := harnesscontract.SealSettledTurnDomainResultFactV3(identity, pending, schema, now.UnixNano())
	mustAssemblerV2(t, err)
	factRef, err := fact.RefV3()
	mustAssemblerV2(t, err)
	identityRef, err := identity.RefV1(fact.ContentDigest)
	mustAssemblerV2(t, err)

	prepare, _, _, _ := testkit.GovernedProviderFixtureV2(now)
	operation := prepare.Intent.Operation
	operation.ExecutionScope = candidate.Run.Scope
	operation.ExecutionScopeDigest = scopeDigest
	operation.RunID = candidate.Run.RunID
	operation.ActivationAttemptID = ""
	operationDigest, err := operation.DigestV3()
	mustAssemblerV2(t, err)
	execution := testkit.GovernedAttemptRefsV2(now, candidate, runtimeports.ProviderAttemptObservedV2)
	execution.Admission.OperationDigest = operationDigest
	prepared := execution.Prepared
	prepared.OperationDigest = operationDigest
	prepared.Digest = ""
	execution.Prepared, err = runtimeports.SealPreparedProviderAttemptRefV2(prepared)
	mustAssemblerV2(t, err)
	execution.Enforcement.OperationDigest = operationDigest
	delegation := execution.Delegation
	observed := *execution.Observation
	settlement := runtimeports.OperationSettlementRefV3{ID: "settlement-g6a-v3", Revision: 1, Digest: testkit.Digest("settlement-g6a-v3"), Attempt: runtimeports.OperationDispatchAttemptRefV3{OperationDigest: operationDigest, EffectID: execution.Admission.EffectID, IntentRevision: execution.Admission.IntentRevision, IntentDigest: execution.Admission.IntentDigest, PermitID: execution.PermitID, PermitRevision: execution.PermitRevision, PermitDigest: execution.PermitDigest, AttemptID: execution.AttemptID, Delegation: &delegation}, Disposition: runtimeports.OperationSettlementAppliedV3, Owner: runtimeports.EffectOwnerRefV2{Role: runtimeports.OwnerSettlement, ComponentID: identity.SettlementOwner.ComponentID, ManifestDigest: identity.SettlementOwner.ManifestDigest}, Observation: &observed, Evidence: []runtimeports.EvidenceRecordRefV2{observed.Evidence}, DomainResultSchema: &fact.Schema, DomainResultDigest: fact.ContentDigest}
	mustAssemblerV2(t, settlement.Validate())
	execution.Settlement = &settlement
	mustAssemblerV2(t, execution.ValidatePrepared())

	matrix := runtimeports.OperationScopeEvidenceActionMatrixV3()
	matrixDigest, err := runtimeports.DigestOperationScopeEvidenceApplicabilityMatrixKeyV3(matrix)
	mustAssemblerV2(t, err)
	declaration := runtimeports.ControlledOperationProviderRouteDeclarationRefV2{RouteID: "route-g6a", Revision: 1, PublisherComponentID: "praxis.harness/assembly", DeclarationDigest: testkit.Digest("route-declaration")}
	conformance := runtimeports.ControlledOperationProviderRouteConformanceRefV2{ConformanceID: "route-conformance-g6a", Revision: 1, DeclarationRef: declaration, ConformanceDigest: testkit.Digest("route-conformance")}
	routeRef, err := runtimeports.SealControlledOperationProviderRouteCurrentRefV2(runtimeports.ControlledOperationProviderRouteCurrentRefV2{CurrentID: "route-current-g6a", Revision: 1, DeclarationRef: declaration, ConformanceRef: conformance, MatrixDigest: matrixDigest, Watermark: testkit.Digest("route-watermark")})
	mustAssemblerV2(t, err)
	contextRef := runtimeports.OperationScopeEvidenceApplicabilityFactRefV3{Kind: runtimeports.OperationScopeEvidenceContextParentKindV3, ID: "context-parent-g6a", Revision: 1, Digest: testkit.Digest("context-parent")}
	ownerInputs, err := harnesscontract.SealCommittedPendingActionOwnerCurrentInputsV1(harnesscontract.CommittedPendingActionOwnerCurrentInputsV1{ModelTurnOperation: operation, GenerationBindingAssociation: runtimeports.GenerationBindingAssociationRefV1{ID: "association-g6a", Revision: 1, Digest: testkit.Digest("association")}, RouteCurrent: routeRef, RouteMatrix: matrix, ContextApplicability: contextRef})
	mustAssemblerV2(t, err)
	baseBinding := harnesscontract.PendingActionApplicationBindingV1{PendingAction: pending, IdentityRef: identityRef, DomainResultFactRef: factRef, ModelTurnSettlementRef: settlement}
	binding, err := harnesscontract.SealPendingActionApplicationBindingV2(harnesscontract.PendingActionApplicationBindingV2{Base: baseBinding, OwnerCurrentInputs: ownerInputs})
	mustAssemblerV2(t, err)
	session, err := harnesscontract.SealGovernedSessionV4(harnesscontract.GovernedSessionV4{ID: base.ID, Revision: 6, Run: base.Run, Endpoint: base.Endpoint, Phase: harnesscontract.SessionWaitingActionV2, Turn: 1, Execution: &execution, PendingAction: &pending, ApplicationBinding: &binding, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.Add(time.Second).UnixNano()})
	mustAssemblerV2(t, err)
	subject := harnesscontract.CommittedPendingActionSubjectV3{Base: harnesscontract.CommittedPendingActionSubjectV2{ExecutionScopeDigest: scopeDigest, Run: session.Run, SessionID: session.ID, SessionRevision: session.Revision, SessionDigest: session.Digest, Turn: session.Turn, PendingActionRef: pending.Ref, IdentityRef: identityRef, DomainResultFactRef: factRef, ModelTurnSettlement: settlement}, ApplicationBinding: binding}
	currentRequest := harnesscontract.CommittedPendingActionCurrentRequestV3{Subject: subject}
	current, err := harnesscontract.SealCommittedPendingActionCurrentV3(harnesscontract.CommittedPendingActionCurrentV3{Run: session.Run, ExecutionScopeDigest: scopeDigest, SessionID: session.ID, SessionRevision: session.Revision, SessionDigest: session.Digest, Phase: session.Phase, Turn: session.Turn, PendingAction: pending, ApplicationBinding: binding, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(20 * time.Second).UnixNano()}, currentRequest, now)
	mustAssemblerV2(t, err)
	identityCoordinate, err := mapIdentityCoordinateV2(fact)
	mustAssemblerV2(t, err)
	pendingSubject, err := mapPendingSubjectV2(session, fact, identityCoordinate)
	mustAssemblerV2(t, err)
	action, err := applicationcontract.SealSingleCallActionCoordinateV2(applicationcontract.SingleCallActionCoordinateV2{ExecutionScope: session.Run.Scope, PendingSubject: pendingSubject})
	mustAssemblerV2(t, err)
	authorityRef := runtimeports.AuthorityBindingRefV2{Ref: "authority-g6a", Digest: testkit.Digest("authority"), Revision: 1, Epoch: session.Run.Scope.AuthorityEpoch}
	authority := runtimeports.DispatchAuthorityFactV2{Ref: authorityRef.Ref, Digest: authorityRef.Digest, Revision: authorityRef.Revision, Scope: session.Run.Scope, ActionScopeDigest: action.Digest, State: runtimeports.AuthorityFactActive, ExpiresUnixNano: now.Add(15 * time.Second).UnixNano()}
	mustAssemblerV2(t, authority.ValidateCurrent(authorityRef, action.ExecutionScope, action.Digest, now))
	return &assemblerV2Fixture{now: now, clockValues: []time.Time{now, now.Add(time.Second), now.Add(2 * time.Second)}, input: applicationcontract.AssembleSingleCallToolActionRequestV2{Action: action, Authority: authorityRef}, session: session, fact: fact, projection: projection, current: current, authority: authority}
}

func (f *assemblerV2Fixture) newAssembler(t *testing.T) *SingleCallToolActionAssemblerV2 {
	t.Helper()
	assembler, err := NewSingleCallToolActionAssemblerV2(f, f, f, f, f, f.clock)
	mustAssemblerV2(t, err)
	return assembler
}

func (f *assemblerV2Fixture) assembleAndResetForInputCurrent(t *testing.T) applicationcontract.SingleCallToolActionRequestV2 {
	t.Helper()
	request, err := f.newAssembler(t).AssembleSingleCallToolActionRequestV2(context.Background(), f.input)
	mustAssemblerV2(t, err)
	f.sessionReads, f.factReads, f.modelReads, f.currentReads, f.authorityReads = 0, 0, 0, 0, 0
	f.clockIndex = 0
	f.clockValues = []time.Time{f.now.Add(3 * time.Second), f.now.Add(4 * time.Second), f.now.Add(5 * time.Second)}
	return request
}

func (f *assemblerV2Fixture) sealCurrentWithExpiry(t *testing.T, expires, requested int64) *harnesscontract.CommittedPendingActionCurrentV3 {
	t.Helper()
	current := f.current.Clone()
	current.ExpiresUnixNano = expires
	current.Digest = ""
	expected := harnesscontract.CommittedPendingActionCurrentRequestV3{Subject: current.SubjectV3(), RequestedNotAfterUnixNano: requested}
	sealed, err := harnesscontract.SealCommittedPendingActionCurrentV3(current, expected, f.now)
	mustAssemblerV2(t, err)
	return &sealed
}

func (f *assemblerV2Fixture) clock() time.Time {
	if f.clockIndex >= len(f.clockValues) {
		return f.clockValues[len(f.clockValues)-1]
	}
	value := f.clockValues[f.clockIndex]
	f.clockIndex++
	return value
}

func (f *assemblerV2Fixture) InspectSessionV4(context.Context, harnesscontract.RunRef, string) (harnesscontract.GovernedSessionV4, error) {
	f.sessionReads++
	if f.sessionReads == 2 && f.secondSession != nil {
		return f.secondSession.Clone(), nil
	}
	return f.session.Clone(), nil
}

func (f *assemblerV2Fixture) InspectExact(context.Context, harnesscontract.SettledTurnDomainResultFactRefV3) (harnesscontract.SettledTurnDomainResultFactV3, error) {
	f.factReads++
	if f.factReads == 2 && f.secondFact != nil {
		return f.secondFact.Clone(), nil
	}
	return f.fact.Clone(), nil
}

func (f *assemblerV2Fixture) InspectExactProjectionV1(context.Context, modelinvoker.ToolCallCandidateObservationRefV1) (modelinvoker.ToolCallCandidateObservationProjectionV1, error) {
	f.modelReads++
	if f.modelReads == 2 && f.secondProjection != nil {
		return f.secondProjection.Clone(), nil
	}
	return f.projection.Clone(), nil
}

func (f *assemblerV2Fixture) InspectCommittedPendingActionCurrentV3(_ context.Context, request harnesscontract.CommittedPendingActionCurrentRequestV3) (harnesscontract.CommittedPendingActionCurrentV3, error) {
	f.currentReads++
	current := f.current.Clone()
	if request.RequestedNotAfterUnixNano > 0 && request.RequestedNotAfterUnixNano < current.ExpiresUnixNano {
		current.ExpiresUnixNano = request.RequestedNotAfterUnixNano
		current.Digest = ""
		var err error
		current, err = harnesscontract.SealCommittedPendingActionCurrentV3(current, request, f.now)
		if err != nil {
			return harnesscontract.CommittedPendingActionCurrentV3{}, err
		}
	}
	if f.currentReads == 2 && f.secondCurrent != nil {
		return f.secondCurrent.Clone(), nil
	}
	return current, nil
}

func (f *assemblerV2Fixture) InspectDispatchAuthority(context.Context, string) (runtimeports.DispatchAuthorityFactV2, error) {
	f.authorityReads++
	if f.authorityReads == 2 && f.secondAuthority != nil {
		return *f.secondAuthority, nil
	}
	return f.authority, nil
}

func (f *assemblerV2Fixture) totalReads() int {
	return f.sessionReads + f.factReads + f.modelReads + f.currentReads + f.authorityReads
}

func mustAssemblerV2(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
