package action_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/action"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
)

type echoOwnerReader struct{}

func (echoOwnerReader) InspectOwnerCurrentV1(_ context.Context, r contract.OwnerCurrentRefV1) (contract.OwnerCurrentRefV1, error) {
	return r, nil
}

type exactObservationReader struct{}

func (exactObservationReader) InspectProviderObservationExactV1(_ context.Context, r runtimeports.ProviderAttemptObservationRefV2) (runtimeports.ProviderAttemptObservationRefV2, error) {
	return r, nil
}

type exactEnforcementReader struct{}

func (exactEnforcementReader) InspectEnforcementPhaseExactV1(_ context.Context, r runtimeports.OperationDispatchEnforcementPhaseRefV4) (runtimeports.OperationDispatchEnforcementPhaseRefV4, error) {
	return r, nil
}

type exactConsumptionReader struct{}

func (exactConsumptionReader) InspectEvidenceConsumptionExactV1(_ context.Context, r runtimeports.OperationScopeEvidenceConsumptionRefV3) (runtimeports.OperationScopeEvidenceConsumptionRefV3, error) {
	return r, nil
}

func TestActionCandidateReservationV2ConcurrentSingleWinnerAndLostReplyInspect(t *testing.T) {
	now := testkit.FixedTime
	store := action.NewStoreV2(action.CausalReadersV1{OwnerCurrent: echoOwnerReader{}})
	candidate := testkit.CandidateV2(now)
	if _, err := store.PutCandidateV2(candidate); err != nil {
		t.Fatal(err)
	}
	actionRef := contract.ObjectRef{ID: candidate.ID, Revision: candidate.Revision, Digest: candidate.Digest}
	var winners atomic.Int32
	var winnerMu sync.Mutex
	var winner contract.ActionReservationFactV2
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			attempt := contract.ApplicationAttemptRefV1{ID: "app-attempt-" + twoDigits(index), Revision: 1, Digest: testkit.Digest("app-attempt-" + twoDigits(index))}
			got, err := store.ReserveV2(context.Background(), actionRef, attempt, testkit.Digest("intent"), candidate.SessionID, testkit.Digest("subject"), now.Add(time.Second), now.Add(10*time.Second))
			if err == nil {
				winners.Add(1)
				winnerMu.Lock()
				winner = got
				winnerMu.Unlock()
			}
		}(i)
	}
	wg.Wait()
	if winners.Load() != 1 {
		t.Fatalf("reservation winners=%d, want 1", winners.Load())
	}
	exact := contract.ObjectRef{ID: winner.ID, Revision: winner.Revision, Digest: winner.Digest}
	inspected, err := store.InspectReservationV2(candidate.ID, exact)
	if err != nil || inspected.Digest != winner.Digest {
		t.Fatalf("lost reply Inspect=%#v err=%v", inspected, err)
	}
}

func TestActionReservationV2ExpiredAndOverlongAreZeroWrite(t *testing.T) {
	now := testkit.FixedTime
	store := action.NewStoreV2(action.CausalReadersV1{OwnerCurrent: echoOwnerReader{}})
	candidate := testkit.CandidateV2(now)
	_, _ = store.PutCandidateV2(candidate)
	ref := contract.ObjectRef{ID: candidate.ID, Revision: 1, Digest: candidate.Digest}
	attempt := contract.ApplicationAttemptRefV1{ID: "app-attempt-expiry", Revision: 1, Digest: testkit.Digest("app-attempt-expiry")}
	if _, err := store.ReserveV2(context.Background(), ref, attempt, testkit.Digest("intent"), candidate.SessionID, testkit.Digest("subject"), now.Add(21*time.Second), now.Add(22*time.Second)); err == nil {
		t.Fatal("expired Candidate reserved")
	}
	if _, err := store.ReserveV2(context.Background(), ref, attempt, testkit.Digest("intent"), candidate.SessionID, testkit.Digest("subject"), now.Add(time.Second), now.Add(30*time.Second)); err == nil {
		t.Fatal("Reservation outlived Candidate")
	}
	if _, err := store.InspectReservationV2(candidate.ID, contract.ObjectRef{ID: "reservation-none", Revision: 1, Digest: testkit.Digest("none")}); err == nil {
		t.Fatal("rejected reservations wrote state")
	}
}

type modelReader struct {
	projection modelinvoker.ToolCallCandidateObservationProjectionV1
	err        error
	calls      atomic.Int32
}

func (r *modelReader) InspectExactProjectionV1(_ context.Context, _ modelinvoker.ToolCallCandidateObservationRefV1) (modelinvoker.ToolCallCandidateObservationProjectionV1, error) {
	r.calls.Add(1)
	return r.projection.Clone(), r.err
}

type enforcementReader struct {
	value runtimeports.OperationDispatchEnforcementPhaseRefV4
	calls atomic.Int32
}

func (r *enforcementReader) InspectCurrentOperationProviderExecuteEnforcementV1(_ context.Context, _ runtimeports.OperationSubjectV3, _ runtimeports.OperationDispatchEnforcementPhaseRefV4) (runtimeports.OperationDispatchEnforcementPhaseRefV4, error) {
	r.calls.Add(1)
	return r.value, nil
}

type handoffReader struct {
	value runtimeports.OperationScopeEvidenceProviderHandoffFactV3
	calls atomic.Int32
}

func (r *handoffReader) InspectCurrentOperationProviderEvidenceHandoffV1(_ context.Context, _ runtimeports.OperationScopeEvidenceProviderHandoffRefV3) (runtimeports.OperationScopeEvidenceProviderHandoffFactV3, error) {
	r.calls.Add(1)
	return r.value, nil
}

func TestCoordinationModelExactReaderPrecedesWatermarkAndN1(t *testing.T) {
	now := testkit.FixedTime
	one := testkit.ModelProjection(1)
	reader := &modelReader{projection: one, err: errors.New("unavailable")}
	store := action.NewCoordinationStoreV1(reader, nil, nil, testkit.SettlementOwner())
	command := canonicalCommand(one)
	if _, err := store.StartOrInspectV1(context.Background(), command, now, now.Add(10*time.Second)); err == nil {
		t.Fatal("Reader failure created Watermark")
	}
	id := watermarkID(t, command)
	if _, err := store.InspectWatermarkV1(id, 1, testkit.Digest("unknown")); err == nil {
		t.Fatal("Reader failure wrote Watermark")
	}
	reader.err = nil
	reader.projection = testkit.ModelProjection(2)
	command.ModelProjection = reader.projection.Ref
	command.ObservationDigest = reader.projection.Observation.Digest
	if _, err := store.StartOrInspectV1(context.Background(), command, now, now.Add(10*time.Second)); err == nil {
		t.Fatal("N>1 created Watermark")
	}
}

func TestCoordinationProviderCapabilityMustEqualEffectKindZeroWrite(t *testing.T) {
	now := testkit.FixedTime
	projection := testkit.ModelProjection(1)
	reader := &modelReader{projection: projection}
	store := action.NewCoordinationStoreV1(reader, nil, nil, testkit.SettlementOwner())
	command := canonicalCommand(projection)
	command.Provider.Capability = "praxis.tool/other"
	if _, err := store.StartOrInspectV1(context.Background(), command, now, now.Add(10*time.Second)); err == nil || !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("Provider capability drift error=%v, want Conflict", err)
	}
	if reader.calls.Load() != 0 {
		t.Fatalf("invalid canonical command reached Model reader: calls=%d", reader.calls.Load())
	}
	if _, err := store.InspectWatermarkV1(watermarkID(t, command), 1, testkit.Digest("unknown")); err == nil {
		t.Fatal("Provider capability drift wrote Watermark")
	}
}

func TestCanonicalCommandRejectsModelPendingPayloadSpliceBeforeReaderOrWatermark(t *testing.T) {
	now := testkit.FixedTime
	projection := testkit.ModelProjection(1)
	reader := &modelReader{projection: projection}
	store := action.NewCoordinationStoreV1(reader, nil, nil, testkit.SettlementOwner())
	command := canonicalCommand(projection)
	command.PayloadDigest = testkit.Digest("attacker-pending-payload")
	if _, err := store.StartOrInspectV1(context.Background(), command, now, now.Add(10*time.Second)); err == nil || !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("payload splice error=%v, want Conflict", err)
	}
	if reader.calls.Load() != 0 {
		t.Fatalf("invalid command reached Model reader: calls=%d", reader.calls.Load())
	}
	if _, err := store.InspectWatermarkV1(watermarkID(t, command), 1, testkit.Digest("not-written")); err == nil {
		t.Fatal("payload splice wrote Watermark")
	}
}

func TestCoordinationBoundaryConcurrentSingleWinnerAndInspectOnly(t *testing.T) {
	now := testkit.FixedTime
	projection := testkit.ModelProjection(1)
	model := &modelReader{projection: projection}
	fixture := testkit.BoundaryFixture(now)
	enforcement := &enforcementReader{value: fixture.Enforcement}
	handoff := &handoffReader{value: fixture.Handoff}
	store := action.NewCoordinationStoreV1(model, enforcement, handoff, testkit.SettlementOwner())
	w, err := store.StartOrInspectV1(context.Background(), canonicalCommand(projection), now, now.Add(15*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	key := source(w)
	candidate := contract.ObjectRef{ID: "action-v2", Revision: 1, Digest: testkit.Digest("candidate")}
	w, err = store.BindCandidateV1(key, candidate, now.Add(time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	w, err = store.BindReservationV1(source(w), contract.ObjectRef{ID: "reservation-v2", Revision: 1, Digest: testkit.Digest("reservation")}, now.Add(2*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	w, err = store.BindRuntimeAttemptV1(source(w), fixture.Operation, fixture.Attempt, now.Add(3*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	preBoundary := source(w)
	var winners atomic.Int32
	var boundaryMu sync.Mutex
	var boundary contract.ToolProviderBoundarySourceRefV1
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, e := store.CrossProviderBoundaryV1(context.Background(), preBoundary, fixture.Enforcement, fixture.Handoff.RefV3(), now.Add(4*time.Millisecond))
			if e == nil {
				winners.Add(1)
				boundaryMu.Lock()
				boundary = got
				boundaryMu.Unlock()
			}
		}()
	}
	wg.Wait()
	if winners.Load() != 1 {
		t.Fatalf("boundary CAS winners=%d, want 1", winners.Load())
	}
	if _, err := store.CrossProviderBoundaryV1(context.Background(), boundary, fixture.Enforcement, fixture.Handoff.RefV3(), now.Add(5*time.Millisecond)); err == nil {
		t.Fatal("post-boundary retry was not inspect-only")
	}
	if enforcement.calls.Load() < 1 || handoff.calls.Load() < 1 {
		t.Fatal("boundary did not reread current Enforcement/Handoff")
	}
	if _, err := store.InspectBoundarySourceCurrentV1(context.Background(), boundary, now.Add(5*time.Millisecond)); err != nil {
		t.Fatalf("exact boundary Inspect failed: %v", err)
	}
}

func TestToolOutcomeDispositionV2Matrix(t *testing.T) {
	valid := [][2]string{{"succeeded", "confirmed_applied"}, {"failed", "confirmed_applied"}, {"failed", "confirmed_not_applied"}}
	for _, pair := range valid {
		if err := contract.ValidateToolOutcomeDispositionV2(contract.ToolOutcomeV2(pair[0]), contract.ToolDispositionV2(pair[1])); err != nil {
			t.Fatalf("valid pair %v: %v", pair, err)
		}
	}
	for _, pair := range [][2]string{{"succeeded", "confirmed_not_applied"}, {"unknown", "confirmed_applied"}, {"failed", "indeterminate"}} {
		if err := contract.ValidateToolOutcomeDispositionV2(contract.ToolOutcomeV2(pair[0]), contract.ToolDispositionV2(pair[1])); err == nil {
			t.Fatalf("invalid pair %v accepted", pair)
		}
	}
}

func TestDomainResultLateTruthCurrentLeaseAndSettlementV4(t *testing.T) {
	now := testkit.FixedTime
	readers := action.CausalReadersV1{OwnerCurrent: echoOwnerReader{}, Observation: exactObservationReader{}, Enforcement: exactEnforcementReader{}, Consumption: exactConsumptionReader{}}
	store := action.NewStoreV2(readers)
	candidate := testkit.CandidateV2(now)
	if _, err := store.PutCandidateV2(candidate); err != nil {
		t.Fatal(err)
	}
	actionRef := contract.ObjectRef{ID: candidate.ID, Revision: candidate.Revision, Digest: candidate.Digest}
	appAttempt := contract.ApplicationAttemptRefV1{ID: "app-attempt-domain", Revision: 1, Digest: testkit.Digest("app-attempt-domain")}
	reservation, err := store.ReserveV2(context.Background(), actionRef, appAttempt, testkit.Digest("intent-v2"), candidate.SessionID, testkit.Digest("subject-v2"), now.Add(time.Second), now.Add(5*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	fixture := testkit.BoundaryFixture(now)
	prepared := testkit.PreparedAttemptFor(now, fixture, testkit.ProviderBinding(), candidate.InputSchema, candidate.Payload.ContentDigest, candidate.PayloadRevision)
	fixture.Enforcement.PreparedAttemptDigest = prepared.Digest
	if err := fixture.Enforcement.Validate(); err != nil {
		t.Fatal(err)
	}
	prepare := fixture.Enforcement
	prepare.Phase = runtimeports.OperationDispatchEnforcementPrepareV4
	prepare.JournalRevision = 1
	prepare.ReceiptDigest = testkit.Digest("prepare-receipt-v2")
	prepare.PrepareReceiptDigest = ""
	prepare.PreparedAttemptDigest = ""
	if err := prepare.Validate(); err != nil {
		t.Fatal(err)
	}
	reservationRef := contract.ObjectRef{ID: reservation.ID, Revision: reservation.Revision, Digest: reservation.Digest}
	causality, err := contract.SealRuntimeAttemptCausalityV1(contract.RuntimeAttemptCausalityV1{Reservation: reservationRef, ApplicationAttempt: appAttempt, Operation: fixture.Operation, OperationDigest: fixture.Attempt.OperationDigest, Attempt: fixture.Attempt, EffectID: fixture.Attempt.EffectID, EffectRevision: fixture.Attempt.IntentRevision, IntentDigest: fixture.Attempt.IntentDigest})
	if err != nil {
		t.Fatal(err)
	}
	prepareConsumption := consumption("prepare", 1)
	executeConsumption := consumption("execute", 2)
	observation := testkit.ProviderObservation(now.Add(6 * time.Second))
	observation.Delegation = *fixture.Attempt.Delegation
	observation.PreparedAttemptID = prepared.ID
	domain, err := contract.SealToolDomainResultFactV2(contract.ToolDomainResultFactV2{ID: "tool-domain-result-v2", TenantID: candidate.TenantID, OperationScopeDigest: candidate.OperationScopeDigest, Action: actionRef, Reservation: reservationRef, ApplicationAttempt: appAttempt, Causality: causality, PreparedAttempt: prepared, Observation: observation, PrepareEnforcement: prepare, ExecuteEnforcement: fixture.Enforcement, PrepareConsumption: prepareConsumption, ExecuteConsumption: executeConsumption, Schema: testkit.Schema("result"), PayloadDigest: testkit.Digest("domain-payload"), PayloadRevision: 1, Owner: testkit.SettlementOwner(), Outcome: contract.ToolOutcomeSucceededV2, Disposition: contract.ToolDispositionConfirmedAppliedV2, CreatedUnixNano: now.Add(6 * time.Second).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	ownerDrift := domain
	ownerDrift.Owner.ComponentID = "praxis.tool/other"
	ownerDrift.Owner.ManifestDigest = testkit.Digest("other-owner-manifest")
	ownerDrift, err = contract.SealToolDomainResultFactV2(ownerDrift)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.PutDomainResultV2(context.Background(), ownerDrift); err == nil || !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("DomainResult Owner drift error=%v, want Conflict", err)
	}
	if _, err = store.InspectDomainResultV2(candidate.ID, contract.ObjectRef{ID: ownerDrift.ID, Revision: ownerDrift.Revision, Digest: ownerDrift.Digest}); err == nil {
		t.Fatal("DomainResult Owner drift wrote state")
	}
	if _, err = store.PutDomainResultV2(context.Background(), domain); err != nil {
		t.Fatal(err)
	}
	domainRef := contract.ObjectRef{ID: domain.ID, Revision: domain.Revision, Digest: domain.Digest}
	lateNow := now.Add(7 * time.Second)
	if _, err = store.InspectDomainResultCurrentV1(context.Background(), candidate.ID, domainRef, lateNow, 31*time.Second); err == nil {
		t.Fatal("lease above 30 seconds accepted")
	}
	current, err := store.InspectDomainResultCurrentV1(context.Background(), candidate.ID, domainRef, lateNow, 30*time.Second)
	if err != nil || current.Fact != domainRef {
		t.Fatalf("late truthful DomainResult current=%#v err=%v", current, err)
	}
	inspection := inspectionV4(t, domain, fixture, lateNow)
	if _, err = store.ApplySettlementV2(candidate.ID, domainRef, inspection, contract.ToolOutcomeSucceededV2, contract.ToolDispositionConfirmedNotAppliedV2, lateNow); err == nil {
		t.Fatal("illegal outcome/disposition wrote settlement")
	}
	result, err := store.ApplySettlementV2(candidate.ID, domainRef, inspection, domain.Outcome, domain.Disposition, lateNow)
	if err != nil {
		t.Fatal(err)
	}
	resultRef := contract.ObjectRef{ID: result.ID, Revision: result.Revision, Digest: result.Digest}
	inspected, err := store.InspectResultV2(candidate.ID, resultRef)
	if err != nil || inspected.Digest != result.Digest {
		t.Fatalf("lost Apply reply Inspect=%#v err=%v", inspected, err)
	}
}

func consumption(label string, sequence uint64) runtimeports.OperationScopeEvidenceConsumptionRefV3 {
	return runtimeports.OperationScopeEvidenceConsumptionRefV3{ID: "consumption-" + label, Revision: 1, Digest: testkit.Digest("consumption-" + label), Record: runtimeports.OperationScopeEvidenceRecordRefV3{LedgerScopeDigest: testkit.Digest("ledger-" + label), Sequence: sequence, RecordDigest: testkit.Digest("record-" + label)}}
}

func inspectionV4(t *testing.T, domain contract.ToolDomainResultFactV2, fixture testkit.BoundaryFixtureV1, now time.Time) runtimeports.OperationInspectionSettlementRefV4 {
	t.Helper()
	provider := runtimeports.ProviderBindingRefV2{BindingSetID: "tool-binding-v2", BindingSetRevision: 1, ComponentID: domain.Owner.ComponentID, ManifestDigest: domain.Owner.ManifestDigest, ArtifactDigest: testkit.Digest("tool-artifact-v2"), Capability: "praxis.tool/execute"}
	runtimeDomain := runtimeports.OperationSettlementDomainResultFactRefV4{Owner: provider, Kind: "praxis.tool/domain-result", ID: domain.ID, Revision: domain.Revision, Digest: domain.Digest, TenantID: domain.TenantID, EffectID: fixture.Attempt.EffectID, EffectRevision: fixture.Attempt.IntentRevision, Operation: fixture.Operation, OperationDigest: fixture.Attempt.OperationDigest, Attempt: fixture.Attempt, Schema: domain.Schema, PayloadDigest: domain.PayloadDigest, PayloadRevision: domain.PayloadRevision, AuthoritativeTime: domain.CreatedUnixNano}
	settlement := runtimeports.OperationSettlementRefV4{ID: "runtime-settlement-v4", Revision: 1, Digest: testkit.Digest("runtime-settlement-v4"), OperationDigest: fixture.Attempt.OperationDigest, EffectID: fixture.Attempt.EffectID, DomainResult: runtimeDomain}
	association := runtimeports.OperationSettlementEvidenceAssociationRefV4{ID: "runtime-association-v4", Revision: 1, Digest: testkit.Digest("runtime-association-v4"), Settlement: settlement, OperationDigest: settlement.OperationDigest, EffectID: settlement.EffectID}
	guard := runtimeports.OperationSettlementTerminalGuardRefV4{ID: "runtime-guard-v4", TenantID: domain.TenantID, EffectID: settlement.EffectID, OperationDigest: settlement.OperationDigest, Revision: 1, Digest: testkit.Digest("runtime-guard-v4"), Settlement: settlement}
	projection := runtimeports.OperationSettlementTerminalProjectionRefV4{ID: "runtime-projection-v4", Revision: 1, Digest: testkit.Digest("runtime-projection-v4"), TenantID: domain.TenantID, OperationDigest: settlement.OperationDigest, EffectID: settlement.EffectID, Settlement: settlement, Association: association, Guard: guard}
	inspection, err := runtimeports.SealOperationInspectionSettlementRefV4(runtimeports.OperationInspectionSettlementRefV4{Settlement: settlement, Association: association, Guard: guard, Projection: projection, DomainResult: runtimeDomain, EffectFactRevision: 4, Owner: domain.Owner, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(10 * time.Second).UnixNano()}, now)
	if err != nil {
		t.Fatal(err)
	}
	return inspection
}

func canonicalCommand(p modelinvoker.ToolCallCandidateObservationProjectionV1) contract.SingleCallCanonicalCommandV1 {
	call := p.Observation.Calls[0]
	argumentsDigest := core.DigestBytes(call.CanonicalArguments)
	return contract.SingleCallCanonicalCommandV1{TenantID: "tenant-v2", ApplicationRequestID: "application-request-v2", ApplicationRequestRevision: 1, ApplicationRequestDigest: testkit.Digest("application-request-v2"), ActionCoordinateDigest: testkit.Digest("action-coordinate-v2"), OperationScopeDigest: testkit.Digest("operation-scope-v2"), ModelProjection: p.Ref, ObservationDigest: p.Observation.Digest, CallID: call.CallID, CallName: call.Name, CanonicalArgumentsDigest: argumentsDigest, PendingAction: contract.PendingActionExactRefV2{ID: "pending-v2", Revision: 1, RequestDigest: testkit.Digest("pending-v2")}, RunID: "run-v2", SessionID: "session-v2", TurnID: "turn-v2", ActionID: "action-v2", ActionCandidate: contract.ObjectRef{ID: "action-v2", Revision: 1, Digest: testkit.Digest("action-candidate-v2")}, Capability: contract.ObjectRef{ID: "capability-v2", Revision: 1, Digest: testkit.Digest("capability-v2")}, Tool: contract.ObjectRef{ID: "tool-v2", Revision: 1, Digest: testkit.Digest("tool-v2")}, InputSchema: testkit.Schema("input"), SourceCandidate: contract.ObjectRef{ID: "source-v2", Revision: 1, Digest: testkit.Digest("source-v2")}, PayloadDigest: argumentsDigest, Provider: testkit.ProviderBinding(), EffectKind: "praxis.tool/execute", PolicyProfile: "praxis.tool/single-call-action-v1"}
}
func source(w contract.SingleCallToolActionCoordinationWatermarkV1) contract.ToolProviderBoundarySourceRefV1 {
	return contract.ToolProviderBoundarySourceRefV1{WatermarkID: w.ID, WatermarkRevision: w.Revision, WatermarkDigest: w.Digest}
}
func watermarkID(t *testing.T, c contract.SingleCallCanonicalCommandV1) string {
	t.Helper()
	id, err := contract.StableID("tool-watermark", string(c.TenantID), c.ApplicationRequestID, "1", string(c.OperationScopeDigest))
	if err != nil {
		t.Fatal(err)
	}
	return id
}
func twoDigits(v int) string { return string([]byte{'0' + byte(v/10), '0' + byte(v%10)}) }
