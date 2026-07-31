package action

import (
	"context"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
)

type settlementSnapshotReaderV2 struct {
	store               *StoreV2
	current             runtimeports.OperationInspectionSettlementRefV4
	association         runtimeports.OperationSettlementEvidenceAssociationV4
	holdMutationAtS2    bool
	currentCalls        atomic.Int32
	associationCalls    atomic.Int32
	mutationLockHeld    chan struct{}
	releaseMutationLock chan struct{}
	lockOnce            sync.Once
}

func (r *settlementSnapshotReaderV2) InspectCurrentOperationSettlementV4(_ context.Context, request runtimeports.InspectCurrentOperationSettlementRequestV4) (runtimeports.OperationInspectionSettlementRefV4, error) {
	r.currentCalls.Add(1)
	if !runtimeports.SameOperationSubjectV3(request.Operation, r.current.DomainResult.Operation) ||
		request.EffectID != r.current.Settlement.EffectID {
		return runtimeports.OperationInspectionSettlementRefV4{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "settlement snapshot request drifted")
	}
	return r.current, nil
}

func (r *settlementSnapshotReaderV2) InspectOperationSettlementEvidenceAssociationV4(_ context.Context, operation runtimeports.OperationSubjectV3, ref runtimeports.OperationSettlementEvidenceAssociationRefV4) (runtimeports.OperationSettlementEvidenceAssociationV4, error) {
	call := r.associationCalls.Add(1)
	if !runtimeports.SameOperationSubjectV3(operation, r.current.DomainResult.Operation) ||
		!runtimeports.SameOperationSettlementEvidenceAssociationRefV4(ref, r.association.RefV4()) {
		return runtimeports.OperationSettlementEvidenceAssociationV4{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "settlement association snapshot drifted")
	}
	if r.holdMutationAtS2 && call == 2 {
		r.lockOnce.Do(func() {
			go func() {
				r.store.mu.Lock()
				close(r.mutationLockHeld)
				<-r.releaseMutationLock
				r.store.mu.Unlock()
			}()
		})
		<-r.mutationLockHeld
	}
	return r.association, nil
}

func TestContextFactStoreSettlementTwoSnapshotAndNonWaitingMutationGateV2(t *testing.T) {
	t.Run("uncontended reads S1 and S2 exactly once", func(t *testing.T) {
		store, reader, actionID, domainRef, inspection, outcome, disposition, now := inMemorySettlementSnapshotFixtureV2(t)
		result, err := store.ApplySettlementWithContextV2(context.Background(), actionID, domainRef, inspection, outcome, disposition, now)
		if err != nil {
			t.Fatal(err)
		}
		if result.Validate() != nil || reader.currentCalls.Load() != 2 || reader.associationCalls.Load() != 2 {
			t.Fatalf("result=%#v current reads=%d association reads=%d, want valid result and 2/2", result, reader.currentCalls.Load(), reader.associationCalls.Load())
		}
	})

	t.Run("held Tool mutation lock after S2 fails closed without a write", func(t *testing.T) {
		store, reader, actionID, domainRef, inspection, outcome, disposition, now := inMemorySettlementSnapshotFixtureV2(t)
		reader.holdMutationAtS2 = true
		reader.mutationLockHeld = make(chan struct{})
		reader.releaseMutationLock = make(chan struct{})
		_, err := store.ApplySettlementWithContextV2(context.Background(), actionID, domainRef, inspection, outcome, disposition, now)
		close(reader.releaseMutationLock)
		if err == nil || !core.HasCategory(err, core.ErrorConflict) {
			t.Fatalf("held mutation lock error=%v, want Conflict", err)
		}
		store.mu.RLock()
		record := store.records[actionID]
		store.mu.RUnlock()
		if record.Apply != nil || record.Result != nil || record.Revision != 1 {
			t.Fatalf("held mutation lock wrote Tool facts: %#v", record)
		}
		if reader.currentCalls.Load() != 2 || reader.associationCalls.Load() != 2 {
			t.Fatalf("held mutation lock reads=%d/%d, want complete S1/S2 before TryLock", reader.currentCalls.Load(), reader.associationCalls.Load())
		}
	})
}

func inMemorySettlementSnapshotFixtureV2(t *testing.T) (*StoreV2, *settlementSnapshotReaderV2, string, contract.ObjectRef, runtimeports.OperationInspectionSettlementRefV4, contract.ToolOutcomeV2, contract.ToolDispositionV2, time.Time) {
	t.Helper()
	now := testkit.FixedTime
	fixture := testkit.ApplicationG6AFixture(now)
	runtimeDomain := fixture.Inspection.DomainResult
	appAttempt := contract.ApplicationAttemptRefV1{ID: "in-memory-app-attempt-v2", Revision: 1, Digest: testkit.Digest("in-memory-app-attempt-v2")}
	causality, err := contract.SealRuntimeAttemptCausalityV1(contract.RuntimeAttemptCausalityV1{
		Reservation: fixture.ToolResult.Reservation, ApplicationAttempt: appAttempt,
		Operation: runtimeDomain.Operation, OperationDigest: runtimeDomain.OperationDigest,
		Attempt: runtimeDomain.Attempt, EffectID: runtimeDomain.EffectID,
		EffectRevision: runtimeDomain.EffectRevision, IntentDigest: runtimeDomain.Attempt.IntentDigest,
	})
	if err != nil {
		t.Fatal(err)
	}
	domain := contract.ToolDomainResultFactV2{
		ID: runtimeDomain.ID, Revision: runtimeDomain.Revision, Digest: runtimeDomain.Digest,
		TenantID: runtimeDomain.TenantID, OperationScopeDigest: testkit.Digest("in-memory-operation-scope-v2"),
		Action: fixture.ToolResult.Action, Reservation: fixture.ToolResult.Reservation,
		ApplicationAttempt: appAttempt, Causality: causality,
		Schema: runtimeDomain.Schema, PayloadDigest: runtimeDomain.PayloadDigest,
		PayloadRevision: runtimeDomain.PayloadRevision, Owner: fixture.Inspection.Owner,
		Outcome: fixture.ToolResult.Outcome, Disposition: fixture.ToolResult.Disposition,
		CreatedUnixNano: runtimeDomain.AuthoritativeTime,
	}
	actionID := domain.Action.ID
	reservation := contract.ActionReservationFactV2{}
	store := NewStoreV2(CausalReadersV1{})
	store.records[actionID] = RecordV2{Reservation: &reservation, DomainResult: &domain, Revision: 1}
	store.domainIndex[domain.ID] = actionID
	reader := &settlementSnapshotReaderV2{store: store, current: fixture.Inspection, association: fixture.Association}
	store.readers.Settlement = reader
	domainRef := contract.ObjectRef{ID: domain.ID, Revision: domain.Revision, Digest: domain.Digest}
	if !reflect.DeepEqual(fixture.Inspection.DomainResult.Attempt, domain.Causality.Attempt) {
		t.Fatal("in-memory settlement fixture lost exact Runtime Attempt")
	}
	return store, reader, actionID, domainRef, fixture.Inspection, domain.Outcome, domain.Disposition, now
}
