package fakes_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/fakes"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestEffectStoreV2LostRepliesRemainInspectable(t *testing.T) {
	t.Parallel()
	now := time.Unix(20_000, 0)
	store, accepted, permit := acceptedEffectStoreV2(t, now, "effect-lost", "permit-lost", "attempt-lost", "domain/lost")
	store.LoseNextIssueReply()
	if _, err := store.IssueDispatchPermit(context.Background(), control.IssueDispatchPermitRequestV2{EffectID: accepted.Intent.ID, ExpectedEffectRevision: accepted.Revision, Permit: permit, Fence: fenceForPermitV2(permit)}); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("issue reply loss must surface as unavailable: %v", err)
	}
	effect, err := store.InspectEffect(context.Background(), accepted.Intent.ID)
	if err != nil || effect.State != control.EffectDispatchIntent || effect.DispatchPermitID != permit.ID {
		t.Fatalf("issued permit and dispatch intent must already be durable: %v %+v", err, effect)
	}
	issued, err := store.InspectDispatchPermit(context.Background(), permit.ID)
	if err != nil || issued.State != control.DispatchPermitIssued {
		t.Fatalf("lost issue reply must recover by inspect: %v %+v", err, issued)
	}
	driftedPermit := permit
	driftedPermit.ExpiresUnixNano--
	driftedFence := fenceForPermitV2(driftedPermit)
	driftedPermit.FenceDigest, _ = ports.DigestExecutionFenceV2(driftedFence)
	if _, err := store.IssueDispatchPermit(context.Background(), control.IssueDispatchPermitRequestV2{EffectID: accepted.Intent.ID, ExpectedEffectRevision: accepted.Revision, Permit: driftedPermit, Fence: driftedFence}); !core.HasReason(err, core.ReasonIdempotencyPayloadMismatch) {
		t.Fatalf("same permit and attempt ids with different TTL/fence must conflict: %v", err)
	}
	store.LoseNextBeginReply()
	if _, err := store.BeginDispatch(context.Background(), control.BeginDispatchRequestV2{EffectID: effect.Intent.ID, ExpectedEffectRevision: effect.Revision, PermitID: permit.ID, ExpectedPermitRevision: issued.Revision}); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("begin reply loss must surface as unavailable: %v", err)
	}
	begun, err := store.InspectDispatchPermit(context.Background(), permit.ID)
	if err != nil || begun.State != control.DispatchPermitBegun {
		t.Fatalf("begin write-ahead must already be durable: %v %+v", err, begun)
	}
	decision, err := control.PlanEffectRecoveryV2(effect, &begun, now)
	if err != nil || decision.Action != control.RecoveryCreateInspectEffect || decision.AutomaticSafe {
		t.Fatalf("post-begin recovery must inspect and never blindly redispatch: %v %+v", err, decision)
	}
	receipt := enforcementReceiptV2(t, begun, now)
	store.LoseNextReceiptReply()
	if _, err := store.RecordEnforcementReceipt(context.Background(), control.RecordEnforcementReceiptRequestV2{PermitID: begun.Permit.ID, ExpectedPermitRevision: begun.Revision, Receipt: receipt}); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("receipt reply loss must surface as unavailable: %v", err)
	}
	recorded, err := store.InspectDispatchPermit(context.Background(), permit.ID)
	if err != nil || recorded.Enforcement == nil || *recorded.Enforcement != receipt {
		t.Fatalf("enforcement receipt must survive lost reply: %v %+v", err, recorded)
	}
	if replayed, err := store.RecordEnforcementReceipt(context.Background(), control.RecordEnforcementReceiptRequestV2{PermitID: begun.Permit.ID, ExpectedPermitRevision: begun.Revision, Receipt: receipt}); err != nil || replayed.Revision != recorded.Revision {
		t.Fatalf("same enforcement receipt replay must be idempotent: %v %+v", err, replayed)
	}
	dispatched := effect
	dispatched.State = control.EffectDispatched
	dispatched.Revision++
	dispatched.DispatchReceipt = &control.ProviderDispatchReceiptV2{PermitID: permit.ID, PermitDigest: begun.PermitDigest, AttemptID: permit.AttemptID, IntentID: permit.IntentID, IntentRevision: permit.IntentRevision, Provider: permit.Provider, ProviderOperationRef: "provider-operation-1", ReceiptRef: "provider-receipt-1", ObservationDigest: effectDigestV2(t, "provider-receipt"), ObservedUnixNano: now.UnixNano()}
	dispatched.UpdatedUnixNano = now.UnixNano()
	wrongReceipt := dispatched
	wrongReceipt.DispatchReceipt = cloneProviderReceiptV2(dispatched.DispatchReceipt)
	wrongReceipt.DispatchReceipt.AttemptID = "unrelated-attempt"
	if _, err := store.CompareAndSwapEffect(context.Background(), control.EffectFactCASRequestV2{ExpectedRevision: effect.Revision, Next: wrongReceipt}); !core.HasReason(err, core.ReasonEvidenceConflict) {
		t.Fatalf("receipt from an unrelated attempt must remain non-authoritative evidence: %v", err)
	}
	store.LoseNextEffectCASReply()
	if _, err := store.CompareAndSwapEffect(context.Background(), control.EffectFactCASRequestV2{ExpectedRevision: effect.Revision, Next: dispatched}); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("provider receipt CAS reply loss must surface as unavailable: %v", err)
	}
	dispatched, err = store.InspectEffect(context.Background(), effect.Intent.ID)
	if err != nil || dispatched.State != control.EffectDispatched || dispatched.DispatchReceipt == nil {
		t.Fatalf("provider receipt must already be durable after lost CAS reply: %v %+v", err, dispatched)
	}
	settled := dispatched
	settled.State = control.EffectSettled
	settled.Revision++
	settled.Settlement = &control.EffectSettlementFactV2{Owner: dispatched.Intent.Owners[2], Disposition: control.SettlementConfirmedApplied, ReceiptRef: "settlement-1", EvidenceDigest: effectDigestV2(t, "settlement"), SettledUnixNano: now.UnixNano()}
	settled.UpdatedUnixNano = now.UnixNano()
	if _, err := store.CompareAndSwapEffect(context.Background(), control.EffectFactCASRequestV2{ExpectedRevision: dispatched.Revision, Next: settled}); err != nil {
		t.Fatal(err)
	}
	if replayed, err := store.CompareAndSwapEffect(context.Background(), control.EffectFactCASRequestV2{ExpectedRevision: dispatched.Revision, Next: settled}); err != nil || replayed.State != control.EffectSettled {
		t.Fatalf("same settlement replay must be idempotent: %v %+v", err, replayed)
	}
	changed := settled
	changed.Settlement = &control.EffectSettlementFactV2{Owner: dispatched.Intent.Owners[2], Disposition: control.SettlementConfirmedFailed, ReceiptRef: "settlement-2", EvidenceDigest: effectDigestV2(t, "other-settlement"), SettledUnixNano: now.UnixNano()}
	if _, err := store.CompareAndSwapEffect(context.Background(), control.EffectFactCASRequestV2{ExpectedRevision: dispatched.Revision, Next: changed}); !core.HasReason(err, core.ReasonRevisionConflict) {
		t.Fatalf("different settlement result must conflict: %v", err)
	}
}

func TestEffectStoreV2BeginLinearizesOnceAndExpiresAtBoundary(t *testing.T) {
	t.Parallel()
	now := time.Unix(21_000, 0)
	store, accepted, permit := acceptedEffectStoreV2(t, now, "effect-race", "permit-race", "attempt-race", "domain/race")
	issued, err := store.IssueDispatchPermit(context.Background(), control.IssueDispatchPermitRequestV2{EffectID: accepted.Intent.ID, ExpectedEffectRevision: accepted.Revision, Permit: permit, Fence: fenceForPermitV2(permit)})
	if err != nil {
		t.Fatal(err)
	}
	var successes atomic.Int32
	var wait sync.WaitGroup
	for range 32 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, err := store.BeginDispatch(context.Background(), control.BeginDispatchRequestV2{EffectID: accepted.Intent.ID, ExpectedEffectRevision: issued.Effect.Revision, PermitID: permit.ID, ExpectedPermitRevision: issued.Permit.Revision})
			if err == nil {
				successes.Add(1)
				return
			}
			if !core.HasReason(err, core.ReasonRevisionConflict) && !core.HasReason(err, core.ReasonDispatchPermitConsumed) {
				t.Errorf("unexpected begin conflict: %v", err)
			}
		}()
	}
	wait.Wait()
	if successes.Load() != 1 {
		t.Fatalf("single permit attempt must linearize exactly once, got %d", successes.Load())
	}

	boundary := permit.ExpiresUnixNano
	boundaryStore, boundaryAccepted, boundaryPermit := acceptedEffectStoreV2(t, now, "effect-expiry", "permit-expiry", "attempt-expiry", "domain/expiry")
	boundaryIssued, err := boundaryStore.IssueDispatchPermit(context.Background(), control.IssueDispatchPermitRequestV2{EffectID: boundaryAccepted.Intent.ID, ExpectedEffectRevision: boundaryAccepted.Revision, Permit: boundaryPermit, Fence: fenceForPermitV2(boundaryPermit)})
	if err != nil {
		t.Fatal(err)
	}
	boundaryStore.SetClock(func() time.Time { return time.Unix(0, boundary) })
	if _, err := boundaryStore.BeginDispatch(context.Background(), control.BeginDispatchRequestV2{EffectID: boundaryAccepted.Intent.ID, ExpectedEffectRevision: boundaryIssued.Effect.Revision, PermitID: boundaryPermit.ID, ExpectedPermitRevision: boundaryIssued.Permit.Revision}); !core.HasReason(err, core.ReasonDispatchPermitExpired) {
		t.Fatalf("permit must be expired at exact boundary: %v", err)
	}
}

func TestEffectStoreV2DispatchRequiresPersistedExactEnforcementReceipt(t *testing.T) {
	t.Parallel()
	now := time.Unix(21_500, 0)
	current := now
	store, accepted, permit := acceptedEffectStoreV2(t, now, "effect-enforcement", "permit-enforcement", "attempt-enforcement", "domain/enforcement")
	store.SetClock(func() time.Time { return current })
	issued, err := store.IssueDispatchPermit(context.Background(), control.IssueDispatchPermitRequestV2{EffectID: accepted.Intent.ID, ExpectedEffectRevision: accepted.Revision, Permit: permit, Fence: fenceForPermitV2(permit)})
	if err != nil {
		t.Fatal(err)
	}
	begun, err := store.BeginDispatch(context.Background(), control.BeginDispatchRequestV2{EffectID: accepted.Intent.ID, ExpectedEffectRevision: issued.Effect.Revision, PermitID: permit.ID, ExpectedPermitRevision: issued.Permit.Revision})
	if err != nil {
		t.Fatal(err)
	}
	dispatchedAt := func(observed time.Time) control.EffectFactV2 {
		next := issued.Effect
		next.State = control.EffectDispatched
		next.Revision++
		next.DispatchReceipt = &control.ProviderDispatchReceiptV2{PermitID: permit.ID, PermitDigest: begun.PermitDigest, AttemptID: permit.AttemptID, IntentID: permit.IntentID, IntentRevision: permit.IntentRevision, Provider: permit.Provider, ProviderOperationRef: "provider-operation-enforcement", ReceiptRef: "provider-receipt-enforcement", ObservationDigest: effectDigestV2(t, "provider-receipt-enforcement"), ObservedUnixNano: observed.UnixNano()}
		next.UpdatedUnixNano = current.UnixNano()
		return next
	}
	if _, err := store.CompareAndSwapEffect(context.Background(), control.EffectFactCASRequestV2{ExpectedRevision: issued.Effect.Revision, Next: dispatchedAt(now)}); !core.HasReason(err, core.ReasonEvidenceConflict) {
		t.Fatalf("begun permit without execution-point enforcement cannot become dispatched: %v", err)
	}
	current = now.Add(2 * time.Second)
	wrong := enforcementReceiptV2(t, begun, current)
	wrong.AttemptID = "wrong-attempt"
	if _, err := store.RecordEnforcementReceipt(context.Background(), control.RecordEnforcementReceiptRequestV2{PermitID: begun.Permit.ID, ExpectedPermitRevision: begun.Revision, Receipt: wrong}); !core.HasReason(err, core.ReasonDispatchPermitInvalid) {
		t.Fatalf("wrong execution-point enforcement receipt must not authorize dispatch: %v", err)
	}
	store.LoseNextReceiptReply()
	exact := enforcementReceiptV2(t, begun, current)
	if _, err := store.RecordEnforcementReceipt(context.Background(), control.RecordEnforcementReceiptRequestV2{PermitID: begun.Permit.ID, ExpectedPermitRevision: begun.Revision, Receipt: exact}); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("injected enforcement reply loss must occur after persistence: %v", err)
	}
	recovered, err := store.InspectDispatchPermit(context.Background(), permit.ID)
	if err != nil || recovered.Enforcement == nil || *recovered.Enforcement != exact {
		t.Fatalf("caller must inspect the persisted enforcement after reply loss: %v %+v", err, recovered)
	}
	if _, err := store.CompareAndSwapEffect(context.Background(), control.EffectFactCASRequestV2{ExpectedRevision: issued.Effect.Revision, Next: dispatchedAt(now.Add(time.Second))}); !core.HasReason(err, core.ReasonEvidenceConflict) {
		t.Fatalf("provider receipt observed before enforcement cannot become authoritative: %v", err)
	}
	if _, err := store.CompareAndSwapEffect(context.Background(), control.EffectFactCASRequestV2{ExpectedRevision: issued.Effect.Revision, Next: dispatchedAt(current)}); err != nil {
		t.Fatalf("provider receipt may advance only after exact persisted enforcement: %v", err)
	}
}

func TestEffectStoreV2UnknownOutcomeOccupiesConflictDomain(t *testing.T) {
	t.Parallel()
	now := time.Unix(22_000, 0)
	store, accepted, permit := acceptedEffectStoreV2(t, now, "effect-unknown", "permit-unknown", "attempt-unknown", "domain/shared")
	issued, err := store.IssueDispatchPermit(context.Background(), control.IssueDispatchPermitRequestV2{EffectID: accepted.Intent.ID, ExpectedEffectRevision: accepted.Revision, Permit: permit, Fence: fenceForPermitV2(permit)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.BeginDispatch(context.Background(), control.BeginDispatchRequestV2{EffectID: accepted.Intent.ID, ExpectedEffectRevision: issued.Effect.Revision, PermitID: permit.ID, ExpectedPermitRevision: issued.Permit.Revision}); err != nil {
		t.Fatal(err)
	}
	unknown := issued.Effect
	unknown.State = control.EffectUnknownOutcome
	unknown.Revision++
	unknown.UpdatedUnixNano = now.UnixNano()
	if _, err := store.CompareAndSwapEffect(context.Background(), control.EffectFactCASRequestV2{ExpectedRevision: issued.Effect.Revision, Next: unknown}); err != nil {
		t.Fatal(err)
	}
	secondIntent := effectIntentV2(t, now, "effect-second", "idem-second", "domain/shared")
	secondIntent.Scope.Instance = core.InstanceRef{ID: "instance-restored", Epoch: 2}
	secondIntent.Scope.SandboxLease = &core.SandboxLeaseRef{ID: "sandbox-restored", Epoch: 2}
	secondFact, err := control.NewProposedEffectFactV2(secondIntent, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateEffect(context.Background(), secondFact); err != nil {
		t.Fatal(err)
	}
	secondAccepted := secondFact
	secondAccepted.State = control.EffectAccepted
	secondAccepted.Revision++
	secondAccepted.UpdatedUnixNano = now.UnixNano()
	if _, err := store.CompareAndSwapEffect(context.Background(), control.EffectFactCASRequestV2{ExpectedRevision: secondFact.Revision, Next: secondAccepted}); !core.HasReason(err, core.ReasonEffectConflictDomainOccupied) {
		t.Fatalf("unknown outcome must continue to occupy the conflict domain: %v", err)
	}
	unsafeReject := unknown
	unsafeReject.State = control.EffectRejected
	unsafeReject.Revision++
	unsafeReject.RejectionReason = core.ReasonEffectUnknownOutcome
	if _, err := store.CompareAndSwapEffect(context.Background(), control.EffectFactCASRequestV2{ExpectedRevision: unknown.Revision, Next: unsafeReject}); !core.HasReason(err, core.ReasonInvalidTransition) {
		t.Fatalf("post-begin unknown outcome must not be converted to pre-dispatch rejection: %v", err)
	}
}

func TestEffectStoreV2ExpiredPermitRecoversToSafePreDispatchRejection(t *testing.T) {
	t.Parallel()
	now := time.Unix(22_500, 0)
	store, accepted, permit := acceptedEffectStoreV2(t, now, "effect-expire-recovery", "permit-expire-recovery", "attempt-expire-recovery", "domain/expire-recovery")
	issued, err := store.IssueDispatchPermit(context.Background(), control.IssueDispatchPermitRequestV2{EffectID: accepted.Intent.ID, ExpectedEffectRevision: accepted.Revision, Permit: permit, Fence: fenceForPermitV2(permit)})
	if err != nil {
		t.Fatal(err)
	}
	boundary := time.Unix(0, permit.ExpiresUnixNano)
	store.SetClock(func() time.Time { return boundary })
	expired := issued.Permit
	expired.State = control.DispatchPermitExpired
	expired.Revision++
	store.LoseNextPermitCASReply()
	if _, err := store.CompareAndSwapDispatchPermit(context.Background(), control.DispatchPermitFactCASRequestV2{PermitID: expired.Permit.ID, ExpectedRevision: issued.Permit.Revision, Next: expired}); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("permit expiry reply loss must surface as unavailable: %v", err)
	}
	inspected, err := store.InspectDispatchPermit(context.Background(), permit.ID)
	if err != nil || inspected.State != control.DispatchPermitExpired {
		t.Fatalf("expired permit must be durable and inspectable: %v %+v", err, inspected)
	}
	decision, err := control.PlanEffectRecoveryV2(issued.Effect, &inspected, boundary)
	if err != nil || decision.Action != control.RecoveryRejectPreDispatch || !decision.AutomaticSafe {
		t.Fatalf("expired never-begun permit must recover deterministically to safe rejection: %v %+v", err, decision)
	}
	if _, err := store.BeginDispatch(context.Background(), control.BeginDispatchRequestV2{EffectID: issued.Effect.Intent.ID, ExpectedEffectRevision: issued.Effect.Revision, PermitID: permit.ID, ExpectedPermitRevision: inspected.Revision}); err == nil {
		t.Fatal("expired permit must never begin or touch a provider")
	}
	rejected := issued.Effect
	rejected.State = control.EffectRejected
	rejected.Revision++
	rejected.RejectionReason = core.ReasonDispatchPermitExpired
	rejected.UpdatedUnixNano = boundary.UnixNano()
	if _, err := store.CompareAndSwapEffect(context.Background(), control.EffectFactCASRequestV2{ExpectedRevision: issued.Effect.Revision, Next: rejected}); err != nil {
		t.Fatalf("expired never-begun effect must be rejectable: %v", err)
	}
}

func TestEffectStoreV2UnknownAndCompensationRequireExactRelatedSettledEffects(t *testing.T) {
	t.Parallel()
	now := time.Unix(23_000, 0)
	store := fakes.NewEffectStoreV2(func() time.Time { return now })
	originalIntent := effectIntentV2(t, now, "effect-original", "idem-original", "domain/original")
	originalUnknown := createUnknownEffectV2(t, store, now, originalIntent, "permit-original")
	forged := originalUnknown
	forged.State = control.EffectSettled
	forged.Revision++
	forged.Settlement = &control.EffectSettlementFactV2{Owner: forged.Intent.Owners[2], Disposition: control.SettlementConfirmedApplied, ReceiptRef: "forged", EvidenceDigest: effectDigestV2(t, "forged"), InspectionIntentID: "missing-inspect", InspectionIntentRevision: 1, InspectionSettlementDigest: effectDigestV2(t, "missing-inspect-settlement"), SettledUnixNano: now.UnixNano()}
	if _, err := store.CompareAndSwapEffect(context.Background(), control.EffectFactCASRequestV2{ExpectedRevision: originalUnknown.Revision, Next: forged}); !core.HasReason(err, core.ReasonEffectUnknownOutcome) {
		t.Fatalf("arbitrary inspection ids cannot settle an unknown outcome: %v", err)
	}
	wrongInspectIntent := effectIntentV2(t, now, "effect-inspect-wrong-revision", "idem-inspect-wrong-revision", "domain/inspect-wrong-revision")
	wrongInspectIntent.Relation = ports.EffectRelationV2{InspectsEffectID: originalIntent.ID, InspectsEffectRevision: originalIntent.Revision + 1}
	wrongInspectSettled := createSettledEffectV2(t, store, now, wrongInspectIntent, "permit-inspect-wrong-revision")
	wrongInspectDigest, _ := core.CanonicalJSONDigest("praxis.runtime.effect", ports.EffectContractVersionV2, "EffectSettlementFactV2", *wrongInspectSettled.Settlement)
	wrongRevisionSettlement := originalUnknown
	wrongRevisionSettlement.State = control.EffectSettled
	wrongRevisionSettlement.Revision++
	wrongRevisionSettlement.Settlement = &control.EffectSettlementFactV2{Owner: wrongRevisionSettlement.Intent.Owners[2], Disposition: control.SettlementConfirmedApplied, ReceiptRef: "wrong-revision", EvidenceDigest: effectDigestV2(t, "wrong-revision"), InspectionIntentID: wrongInspectIntent.ID, InspectionIntentRevision: wrongInspectIntent.Revision, InspectionSettlementDigest: wrongInspectDigest, SettledUnixNano: now.UnixNano()}
	if _, err := store.CompareAndSwapEffect(context.Background(), control.EffectFactCASRequestV2{ExpectedRevision: originalUnknown.Revision, Next: wrongRevisionSettlement}); !core.HasReason(err, core.ReasonEffectUnknownOutcome) {
		t.Fatalf("inspect relation to a different original revision cannot settle unknown: %v", err)
	}
	inspectIntent := effectIntentV2(t, now, "effect-inspect", "idem-inspect", "domain/inspect")
	inspectIntent.Relation = ports.EffectRelationV2{InspectsEffectID: originalIntent.ID, InspectsEffectRevision: originalIntent.Revision}
	inspectSettled := createSettledEffectV2(t, store, now, inspectIntent, "permit-inspect")
	inspectSettlementDigest, _ := core.CanonicalJSONDigest("praxis.runtime.effect", ports.EffectContractVersionV2, "EffectSettlementFactV2", *inspectSettled.Settlement)
	settledOriginal := originalUnknown
	settledOriginal.State = control.EffectSettled
	settledOriginal.Revision++
	settledOriginal.Settlement = &control.EffectSettlementFactV2{Owner: settledOriginal.Intent.Owners[2], Disposition: control.SettlementConfirmedApplied, ReceiptRef: "inspected", EvidenceDigest: effectDigestV2(t, "inspected"), InspectionIntentID: inspectIntent.ID, InspectionIntentRevision: inspectIntent.Revision, InspectionSettlementDigest: inspectSettlementDigest, SettledUnixNano: now.UnixNano()}
	settledOriginal, err := store.CompareAndSwapEffect(context.Background(), control.EffectFactCASRequestV2{ExpectedRevision: originalUnknown.Revision, Next: settledOriginal})
	if err != nil {
		t.Fatalf("exact independently settled inspect effect should settle unknown: %v", err)
	}
	unrelatedIntent := effectIntentV2(t, now, "effect-unrelated", "idem-unrelated", "domain/unrelated")
	unrelatedSettled := createSettledEffectV2(t, store, now, unrelatedIntent, "permit-unrelated")
	unrelatedDigest, _ := core.CanonicalJSONDigest("praxis.runtime.effect", ports.EffectContractVersionV2, "EffectSettlementFactV2", *unrelatedSettled.Settlement)
	forgedCompensation := settledOriginal
	forgedCompensation.State = control.EffectCompensated
	forgedCompensation.Revision++
	forgedCompensation.Compensation = &control.CompensationCompletionV2{EffectID: unrelatedIntent.ID, EffectRevision: unrelatedIntent.Revision, SettlementDigest: unrelatedDigest}
	if _, err := store.CompareAndSwapEffect(context.Background(), control.EffectFactCASRequestV2{ExpectedRevision: settledOriginal.Revision, Next: forgedCompensation}); !core.HasReason(err, core.ReasonCompensationIncomplete) {
		t.Fatalf("unrelated settled effect cannot impersonate compensation: %v", err)
	}
	wrongCompensationIntent := effectIntentV2(t, now, "effect-compensation-wrong-revision", "idem-compensation-wrong-revision", "domain/compensation-wrong-revision")
	wrongCompensationIntent.Relation = ports.EffectRelationV2{CompensatesEffectID: originalIntent.ID, CompensatesEffectRevision: originalIntent.Revision + 1}
	wrongCompensationSettled := createSettledEffectV2(t, store, now, wrongCompensationIntent, "permit-compensation-wrong-revision")
	wrongCompensationDigest, _ := core.CanonicalJSONDigest("praxis.runtime.effect", ports.EffectContractVersionV2, "EffectSettlementFactV2", *wrongCompensationSettled.Settlement)
	wrongRevisionCompensation := settledOriginal
	wrongRevisionCompensation.State = control.EffectCompensated
	wrongRevisionCompensation.Revision++
	wrongRevisionCompensation.Compensation = &control.CompensationCompletionV2{EffectID: wrongCompensationIntent.ID, EffectRevision: wrongCompensationIntent.Revision, SettlementDigest: wrongCompensationDigest}
	if _, err := store.CompareAndSwapEffect(context.Background(), control.EffectFactCASRequestV2{ExpectedRevision: settledOriginal.Revision, Next: wrongRevisionCompensation}); !core.HasReason(err, core.ReasonCompensationIncomplete) {
		t.Fatalf("compensation relation to a different original revision must fail: %v", err)
	}
	crossTenantIntent := effectIntentV2(t, now, "effect-compensation-cross-tenant", "idem-compensation-cross-tenant", "domain/compensation-cross-tenant")
	crossTenantIntent.Scope.Identity.TenantID = "tenant-2"
	crossTenantIntent.Scope.Identity.ID = "agent-2"
	stableTenantTwo := ports.StableTenantScopeDigestV2("tenant-2")
	crossTenantIntent.ConflictDomain.ScopeDigest = stableTenantTwo
	crossTenantIntent.Idempotency.ScopeDigest = stableTenantTwo
	crossTenantIntent.Relation = ports.EffectRelationV2{CompensatesEffectID: originalIntent.ID, CompensatesEffectRevision: originalIntent.Revision}
	crossTenantSettled := createSettledEffectV2(t, store, now, crossTenantIntent, "permit-compensation-cross-tenant")
	crossTenantDigest, _ := core.CanonicalJSONDigest("praxis.runtime.effect", ports.EffectContractVersionV2, "EffectSettlementFactV2", *crossTenantSettled.Settlement)
	crossTenantCompensation := settledOriginal
	crossTenantCompensation.State = control.EffectCompensated
	crossTenantCompensation.Revision++
	crossTenantCompensation.Compensation = &control.CompensationCompletionV2{EffectID: crossTenantIntent.ID, EffectRevision: crossTenantIntent.Revision, SettlementDigest: crossTenantDigest}
	if _, err := store.CompareAndSwapEffect(context.Background(), control.EffectFactCASRequestV2{ExpectedRevision: settledOriginal.Revision, Next: crossTenantCompensation}); !core.HasReason(err, core.ReasonCompensationIncomplete) {
		t.Fatalf("settled compensation cannot be reused across tenant boundaries: %v", err)
	}
	compensationIntent := effectIntentV2(t, now, "effect-compensation", "idem-compensation", "domain/compensation")
	compensationIntent.Relation = ports.EffectRelationV2{CompensatesEffectID: originalIntent.ID, CompensatesEffectRevision: originalIntent.Revision}
	compensationSettled := createSettledEffectV2(t, store, now, compensationIntent, "permit-compensation")
	compensationDigest, _ := core.CanonicalJSONDigest("praxis.runtime.effect", ports.EffectContractVersionV2, "EffectSettlementFactV2", *compensationSettled.Settlement)
	compensated := settledOriginal
	compensated.State = control.EffectCompensated
	compensated.Revision++
	compensated.Compensation = &control.CompensationCompletionV2{EffectID: compensationIntent.ID, EffectRevision: compensationIntent.Revision, SettlementDigest: compensationDigest}
	if _, err := store.CompareAndSwapEffect(context.Background(), control.EffectFactCASRequestV2{ExpectedRevision: settledOriginal.Revision, Next: compensated}); err != nil {
		t.Fatalf("exact related settled compensation should close original: %v", err)
	}
}

func TestEffectStoreV2OrthogonalCleanupCASRemainsRecoverableAfterSettlement(t *testing.T) {
	t.Parallel()
	now := time.Unix(23_500, 0)
	store := fakes.NewEffectStoreV2(func() time.Time { return now })
	originalIntent := effectIntentV2(t, now, "effect-cleanup-original", "idem-cleanup-original", "domain/cleanup-original")
	originalIntent.RequiresCleanup = true
	settledOriginal := createSettledEffectV2(t, store, now, originalIntent, "permit-cleanup-original")
	if settledOriginal.State != control.EffectSettled || settledOriginal.Cleanup != control.EffectCleanupPending || !settledOriginal.ConflictDomainOccupied() {
		t.Fatalf("settlement alone must not release a pending cleanup domain: %+v", settledOriginal)
	}
	cleanupIntent := effectIntentV2(t, now, "effect-cleanup", "idem-cleanup", "domain/cleanup")
	cleanupIntent.Relation = ports.EffectRelationV2{CleansUpEffectID: originalIntent.ID, CleansUpEffectRevision: originalIntent.Revision}
	cleanupSettled := createSettledEffectV2(t, store, now, cleanupIntent, "permit-cleanup")
	cleanupDigest, _ := core.CanonicalJSONDigest("praxis.runtime.effect", ports.EffectContractVersionV2, "EffectSettlementFactV2", *cleanupSettled.Settlement)
	closed := settledOriginal
	closed.Revision++
	closed.Cleanup = control.EffectCleanupComplete
	closed.CleanupResolution = &control.EffectResolutionCompletionV2{EffectID: cleanupIntent.ID, EffectRevision: cleanupIntent.Revision, SettlementDigest: cleanupDigest}
	store.LoseNextEffectCASReply()
	if _, err := store.CompareAndSwapEffect(context.Background(), control.EffectFactCASRequestV2{ExpectedRevision: settledOriginal.Revision, Next: closed}); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("lost cleanup CAS reply must surface as unavailable after durable write: %v", err)
	}
	inspected, err := store.InspectEffect(context.Background(), originalIntent.ID)
	if err != nil || inspected.Cleanup != control.EffectCleanupComplete || inspected.ConflictDomainOccupied() {
		t.Fatalf("cleanup completion must be inspectable and release only after closure: %v %+v", err, inspected)
	}
	if replayed, err := store.CompareAndSwapEffect(context.Background(), control.EffectFactCASRequestV2{ExpectedRevision: settledOriginal.Revision, Next: closed}); err != nil || replayed.Revision != closed.Revision {
		t.Fatalf("exact cleanup CAS replay must be idempotent after reply loss: %v %+v", err, replayed)
	}
	conflicting := closed
	conflicting.CleanupResolution = &control.EffectResolutionCompletionV2{EffectID: "different-cleanup", EffectRevision: 1, SettlementDigest: effectDigestV2(t, "different-cleanup")}
	if _, err := store.CompareAndSwapEffect(context.Background(), control.EffectFactCASRequestV2{ExpectedRevision: settledOriginal.Revision, Next: conflicting}); !core.HasReason(err, core.ReasonRevisionConflict) {
		t.Fatalf("a concurrent different cleanup resolution must lose linearization: %v", err)
	}
}

func acceptedEffectStoreV2(t *testing.T, now time.Time, effectID, permitID, attemptID, domain string) (*fakes.EffectStoreV2, control.EffectFactV2, ports.DispatchPermitV2) {
	t.Helper()
	store := fakes.NewEffectStoreV2(func() time.Time { return now })
	intent := effectIntentV2(t, now, effectID, "idem-"+effectID, domain)
	proposed, err := control.NewProposedEffectFactV2(intent, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateEffect(context.Background(), proposed); err != nil {
		t.Fatal(err)
	}
	accepted := proposed
	accepted.State = control.EffectAccepted
	accepted.Revision++
	accepted.UpdatedUnixNano = now.UnixNano()
	accepted, err = store.CompareAndSwapEffect(context.Background(), control.EffectFactCASRequestV2{ExpectedRevision: proposed.Revision, Next: accepted})
	if err != nil {
		t.Fatal(err)
	}
	intentDigest, _ := intent.DigestV2()
	grantDigest := effectDigestV2(t, "grant")
	fence := core.ExecutionFence{BoundaryScope: core.FenceBoundaryInstance, Scope: intent.Scope, CapabilityGrantDigest: grantDigest, EffectIntentID: intent.ID, EffectIntentRevision: intent.Revision, CanonicalPayloadDigest: intent.Payload.ContentDigest, ExpiresAt: now.Add(10 * time.Second)}
	fenceDigest, _ := ports.DigestExecutionFenceV2(fence)
	credentialDigest, _ := ports.DigestCredentialLeaseFactsV2(nil)
	permit := ports.DispatchPermitV2{ContractVersion: ports.EffectContractVersionV2, ID: permitID, Revision: 1, AttemptID: attemptID, IntentID: intent.ID, IntentRevision: intent.Revision, IntentDigest: intentDigest, PayloadSchema: intent.Payload.Schema, PayloadDigest: intent.Payload.ContentDigest, PayloadRevision: intent.PayloadRevision, Scope: intent.Scope, RunID: intent.RunID, ConflictDomain: intent.ConflictDomain, Provider: intent.Provider, EnforcementPoint: intent.Provider, Authority: intent.Authority, Review: intent.Review, ReviewVerdictDigest: effectDigestV2(t, "review-verdict"), ReviewVerdictRevision: 1, Budget: intent.Budget, Policy: intent.Policy, CurrentScope: intent.CurrentScope, CapabilityGrantDigest: grantDigest, CredentialGrantDigest: credentialDigest, FenceDigest: fenceDigest, Idempotency: intent.Idempotency, IssuedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(10 * time.Second).UnixNano()}
	return store, accepted, permit
}

func effectIntentV2(t *testing.T, now time.Time, id, idempotencyKey, domain string) ports.EffectIntentV2 {
	t.Helper()
	payload := []byte(`{"action":"test"}`)
	schemaDigest := effectDigestV2(t, "schema")
	manifestDigest := effectDigestV2(t, "manifest")
	scope := core.ExecutionScope{Identity: core.AgentIdentityRef{TenantID: "tenant-1", ID: "agent-1", Epoch: 1}, Lineage: core.LineageRef{ID: "lineage-1", PlanDigest: effectDigestV2(t, "plan")}, Instance: core.InstanceRef{ID: "instance-1", Epoch: 1}, SandboxLease: &core.SandboxLeaseRef{ID: "sandbox-1", Epoch: 1}, AuthorityEpoch: 1}
	owners := []ports.EffectOwnerRefV2{{Role: ports.OwnerCleanup, ComponentID: "vendor/provider", ManifestDigest: manifestDigest}, {Role: ports.OwnerEffect, ComponentID: "vendor/provider", ManifestDigest: manifestDigest}, {Role: ports.OwnerSettlement, ComponentID: "vendor/provider", ManifestDigest: manifestDigest}}
	stableScope := ports.StableTenantScopeDigestV2(scope.Identity.TenantID)
	return ports.EffectIntentV2{ContractVersion: ports.EffectContractVersionV2, ID: core.EffectIntentID(id), Revision: 1, Scope: scope, RunID: "run-1", Kind: "vendor/custom-effect", RiskClass: "vendor/high", ActionScopeDigest: effectDigestV2(t, "action-scope"), Payload: ports.OpaquePayloadV2{Schema: ports.SchemaRefV2{Namespace: "vendor", Name: "effect", Version: "1.0.0", MediaType: "application/json", ContentDigest: schemaDigest}, ContentDigest: core.DigestBytes(payload), Length: uint64(len(payload)), Inline: payload, LimitPolicy: ports.OpaqueLimitPolicyRefV2{Policy: "vendor/default-limit", Digest: effectDigestV2(t, "limit")}}, PayloadRevision: 1, Target: "provider://target", ConflictDomain: ports.ConflictDomainBindingV2{Domain: ports.NamespacedNameV2(domain), ScopeClass: ports.EffectStableScopeTenantV2, ScopeDigest: stableScope}, Owners: owners, Provider: ports.ProviderBindingRefV2{BindingSetID: "set-1", BindingSetRevision: 1, ComponentID: "vendor/provider", ManifestDigest: manifestDigest, ArtifactDigest: effectDigestV2(t, "artifact"), Capability: "vendor/execute"}, Authority: ports.AuthorityBindingRefV2{Ref: "authority-1", Digest: effectDigestV2(t, "authority"), Revision: 1, Epoch: 1}, Review: ports.ReviewBindingRefV2{Ref: "review-1", Digest: effectDigestV2(t, "review"), Revision: 1, PolicyDigest: effectDigestV2(t, "review-policy")}, Budget: ports.BudgetBindingRefV2{Ref: "budget-1", Digest: effectDigestV2(t, "budget"), Revision: 1, PolicyDigest: effectDigestV2(t, "budget-policy")}, Policy: ports.DispatchPolicyBindingRefV2{Ref: "dispatch-policy-1", Digest: effectDigestV2(t, "dispatch-policy"), Revision: 1}, CurrentScope: ports.ExecutionScopeBindingRefV2{Ref: "scope-current-1", Digest: effectDigestV2(t, "scope-current"), Revision: 1}, Idempotency: ports.IdempotencyBindingV2{Key: idempotencyKey, ScopeClass: ports.EffectStableScopeTenantV2, ScopeDigest: stableScope, Class: core.IdempotencyQueryable}, CredentialLeases: []ports.CredentialLeaseRefV2{}, ExpiresUnixNano: now.Add(time.Minute).UnixNano()}
}

func fenceForPermitV2(permit ports.DispatchPermitV2) core.ExecutionFence {
	return core.ExecutionFence{BoundaryScope: core.FenceBoundaryInstance, Scope: permit.Scope, CapabilityGrantDigest: permit.CapabilityGrantDigest, EffectIntentID: permit.IntentID, EffectIntentRevision: permit.IntentRevision, CanonicalPayloadDigest: permit.PayloadDigest, ExpiresAt: time.Unix(0, permit.ExpiresUnixNano)}
}

func cloneProviderReceiptV2(receipt *control.ProviderDispatchReceiptV2) *control.ProviderDispatchReceiptV2 {
	if receipt == nil {
		return nil
	}
	copy := *receipt
	return &copy
}

func createUnknownEffectV2(t *testing.T, store *fakes.EffectStoreV2, now time.Time, intent ports.EffectIntentV2, permitID string) control.EffectFactV2 {
	t.Helper()
	dispatchIntent, permitFact := createBegunEffectV2(t, store, now, intent, permitID)
	unknown := dispatchIntent
	unknown.State = control.EffectUnknownOutcome
	unknown.Revision++
	unknown.UpdatedUnixNano = now.UnixNano()
	unknown, err := store.CompareAndSwapEffect(context.Background(), control.EffectFactCASRequestV2{ExpectedRevision: dispatchIntent.Revision, Next: unknown})
	if err != nil {
		t.Fatal(err)
	}
	_ = permitFact
	return unknown
}

func createSettledEffectV2(t *testing.T, store *fakes.EffectStoreV2, now time.Time, intent ports.EffectIntentV2, permitID string) control.EffectFactV2 {
	t.Helper()
	dispatchIntent, permitFact := createBegunEffectV2(t, store, now, intent, permitID)
	if _, err := store.RecordEnforcementReceipt(context.Background(), control.RecordEnforcementReceiptRequestV2{PermitID: permitFact.Permit.ID, ExpectedPermitRevision: permitFact.Revision, Receipt: enforcementReceiptV2(t, permitFact, now)}); err != nil {
		t.Fatal(err)
	}
	dispatched := dispatchIntent
	dispatched.State = control.EffectDispatched
	dispatched.Revision++
	dispatched.DispatchReceipt = &control.ProviderDispatchReceiptV2{PermitID: permitFact.Permit.ID, PermitDigest: permitFact.PermitDigest, AttemptID: permitFact.Permit.AttemptID, IntentID: intent.ID, IntentRevision: intent.Revision, Provider: intent.Provider, ProviderOperationRef: "operation-" + permitID, ReceiptRef: "receipt-" + permitID, ObservationDigest: effectDigestV2(t, "receipt-"+permitID), ObservedUnixNano: now.UnixNano()}
	dispatched.UpdatedUnixNano = now.UnixNano()
	dispatched, err := store.CompareAndSwapEffect(context.Background(), control.EffectFactCASRequestV2{ExpectedRevision: dispatchIntent.Revision, Next: dispatched})
	if err != nil {
		t.Fatal(err)
	}
	settled := dispatched
	settled.State = control.EffectSettled
	settled.Revision++
	settled.Settlement = &control.EffectSettlementFactV2{Owner: intent.Owners[2], Disposition: control.SettlementConfirmedApplied, ReceiptRef: "settlement-" + permitID, EvidenceDigest: effectDigestV2(t, "settlement-"+permitID), SettledUnixNano: now.UnixNano()}
	settled.UpdatedUnixNano = now.UnixNano()
	settled, err = store.CompareAndSwapEffect(context.Background(), control.EffectFactCASRequestV2{ExpectedRevision: dispatched.Revision, Next: settled})
	if err != nil {
		t.Fatal(err)
	}
	return settled
}

func createBegunEffectV2(t *testing.T, store *fakes.EffectStoreV2, now time.Time, intent ports.EffectIntentV2, permitID string) (control.EffectFactV2, control.DispatchPermitFactV2) {
	t.Helper()
	proposed, err := control.NewProposedEffectFactV2(intent, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateEffect(context.Background(), proposed); err != nil {
		t.Fatal(err)
	}
	accepted := proposed
	accepted.State = control.EffectAccepted
	accepted.Revision++
	accepted.UpdatedUnixNano = now.UnixNano()
	accepted, err = store.CompareAndSwapEffect(context.Background(), control.EffectFactCASRequestV2{ExpectedRevision: proposed.Revision, Next: accepted})
	if err != nil {
		t.Fatal(err)
	}
	permit := permitForIntentV2(t, intent, permitID, "attempt-"+permitID, now)
	issued, err := store.IssueDispatchPermit(context.Background(), control.IssueDispatchPermitRequestV2{EffectID: intent.ID, ExpectedEffectRevision: accepted.Revision, Permit: permit, Fence: fenceForPermitV2(permit)})
	if err != nil {
		t.Fatal(err)
	}
	begun, err := store.BeginDispatch(context.Background(), control.BeginDispatchRequestV2{EffectID: intent.ID, ExpectedEffectRevision: issued.Effect.Revision, PermitID: permit.ID, ExpectedPermitRevision: issued.Permit.Revision})
	if err != nil {
		t.Fatal(err)
	}
	return issued.Effect, begun
}

func permitForIntentV2(t *testing.T, intent ports.EffectIntentV2, permitID, attemptID string, now time.Time) ports.DispatchPermitV2 {
	t.Helper()
	intentDigest, _ := intent.DigestV2()
	grantDigest := effectDigestV2(t, "grant")
	fence := core.ExecutionFence{BoundaryScope: core.FenceBoundaryInstance, Scope: intent.Scope, CapabilityGrantDigest: grantDigest, EffectIntentID: intent.ID, EffectIntentRevision: intent.Revision, CanonicalPayloadDigest: intent.Payload.ContentDigest, ExpiresAt: now.Add(10 * time.Second)}
	fenceDigest, _ := ports.DigestExecutionFenceV2(fence)
	credentialDigest, _ := ports.DigestCredentialLeaseFactsV2(nil)
	return ports.DispatchPermitV2{ContractVersion: ports.EffectContractVersionV2, ID: permitID, Revision: 1, AttemptID: attemptID, IntentID: intent.ID, IntentRevision: intent.Revision, IntentDigest: intentDigest, PayloadSchema: intent.Payload.Schema, PayloadDigest: intent.Payload.ContentDigest, PayloadRevision: intent.PayloadRevision, Scope: intent.Scope, RunID: intent.RunID, ConflictDomain: intent.ConflictDomain, Provider: intent.Provider, EnforcementPoint: intent.Provider, Authority: intent.Authority, Review: intent.Review, ReviewVerdictDigest: effectDigestV2(t, "review-verdict"), ReviewVerdictRevision: 1, Budget: intent.Budget, Policy: intent.Policy, CurrentScope: intent.CurrentScope, CapabilityGrantDigest: grantDigest, CredentialGrantDigest: credentialDigest, FenceDigest: fenceDigest, Idempotency: intent.Idempotency, IssuedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(10 * time.Second).UnixNano()}
}

func enforcementReceiptV2(t *testing.T, permit control.DispatchPermitFactV2, now time.Time) ports.EnforcementReceiptV2 {
	t.Helper()
	return ports.EnforcementReceiptV2{ContractVersion: ports.EffectContractVersionV2, PermitID: permit.Permit.ID, PermitRevision: permit.Permit.Revision, AttemptID: permit.Permit.AttemptID, PermitDigest: permit.PermitDigest, Verifier: permit.Permit.EnforcementPoint, ValidatedAt: now.UnixNano()}
}

func effectDigestV2(t *testing.T, value any) core.Digest {
	t.Helper()
	digest, err := core.DigestJSON(value)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}
