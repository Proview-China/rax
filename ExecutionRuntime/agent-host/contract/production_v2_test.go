package contract_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/journal"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/owneradapter"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestHostStartClaimIsVersionNeutralPermanentConflictDomain(t *testing.T) {
	now := time.Unix(1_900_000_000, 0)
	claim := startClaimV1(t, now)
	if err := claim.ValidateCurrentV1(now); err != nil {
		t.Fatal(err)
	}
	if err := claim.ValidateCurrentV1(now.Add(2 * time.Hour)); !contract.HasCode(err, contract.ErrorPrecondition) {
		t.Fatalf("expired claim error=%v", err)
	}
	if err := claim.ValidateHistoricalV1(); err != nil {
		t.Fatalf("expired claim must remain a valid historical tombstone input: %v", err)
	}
	forged := claim
	forged.HostContractVersion = contract.ContractVersionV2
	forged.Digest = ""
	forged, err := contract.SealHostStartClaimV1(forged)
	if err != nil {
		t.Fatal(err)
	}
	if forged.Digest == claim.Digest {
		t.Fatal("Host contract version must be bound by the shared claim digest")
	}
}

func TestCleanupPlanV2SealsTypedBarrierOrder(t *testing.T) {
	plan := cleanupPlanV2(t)
	if err := plan.Validate(); err != nil {
		t.Fatal(err)
	}
	ref, err := plan.RefV2()
	if err != nil || ref.Revision != 1 || ref.Digest != plan.Digest {
		t.Fatalf("ref=%+v err=%v", ref, err)
	}

	tests := []struct {
		name   string
		mutate func(*contract.CleanupPlanV2)
	}{
		{"cycle", func(p *contract.CleanupPlanV2) {
			p.Nodes[nodeIndex(p.Nodes, contract.CleanupBarrierRuntimeCleanupAggregateV2)].RequiredBarrierIDs = []string{"host-handle"}
		}},
		{"release-before-lease-cleanup", func(p *contract.CleanupPlanV2) {
			p.Nodes[nodeIndex(p.Nodes, "lease-owner")].RequiredBarrierIDs = []string{contract.CleanupBarrierSandboxReleaseV2}
		}},
		{"live-after-close", func(p *contract.CleanupPlanV2) {
			p.Nodes[nodeIndex(p.Nodes, "live-owner")].RequiredBarrierIDs = []string{contract.CleanupBarrierHarnessCloseV2}
		}},
		{"host-handle-before-aggregate", func(p *contract.CleanupPlanV2) { p.Nodes[nodeIndex(p.Nodes, "host-handle")].RequiredBarrierIDs = nil }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bad := plan
			bad.Nodes = append([]contract.CleanupNodeV2(nil), plan.Nodes...)
			tt.mutate(&bad)
			for i := range bad.Nodes {
				bad.Nodes[i].Digest = ""
				bad.Nodes[i], _ = contract.SealCleanupNodeV2(bad.Nodes[i])
			}
			bad.Digest = ""
			if _, err := contract.SealCleanupPlanV2(bad); err == nil {
				t.Fatal("misordered cleanup plan was accepted")
			}
		})
	}
}

func TestCleanupAttemptV2OnlyAdvancesThroughInspectRecoveryStates(t *testing.T) {
	plan := cleanupPlanV2(t)
	planRef, _ := plan.RefV2()
	now := time.Unix(1_900_000_000, 0)
	intent := contract.CleanupAttemptV2{ContractVersion: contract.CleanupContractVersionV2, AttemptID: "cleanup-attempt-1", Revision: 1, HostID: "host-1", StartID: "start-1", PlanRef: planRef, NodeID: "lease-owner", RequestDigest: digestV1(t, "cleanup-request"), PredecessorRevision: 4, BarrierCurrentRefs: []contract.ExactRefV1{exactRefV1(t, "praxis.sandbox/fence-current", "fence-1")}, State: contract.CleanupIntentRecordedV2, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()}
	intent, err := contract.SealCleanupAttemptV2(intent)
	if err != nil {
		t.Fatal(err)
	}
	unknown := intent
	unknown.Revision++
	unknown.State = contract.CleanupOutcomeUnknownV2
	unknown.UpdatedUnixNano++
	unknown.Digest = ""
	unknown, _ = contract.SealCleanupAttemptV2(unknown)
	if err := contract.ValidateCleanupAttemptSuccessorV2(intent, unknown); err != nil {
		t.Fatal(err)
	}
	reconcile := unknown
	reconcile.Revision++
	reconcile.State = contract.CleanupReconciliationRequiredV2
	reconcile.UpdatedUnixNano++
	reconcile.Digest = ""
	reconcile, _ = contract.SealCleanupAttemptV2(reconcile)
	if err := contract.ValidateCleanupAttemptSuccessorV2(unknown, reconcile); err != nil {
		t.Fatal(err)
	}
	forged := reconcile
	forged.Revision++
	forged.State = contract.CleanupIntentRecordedV2
	forged.UpdatedUnixNano++
	forged.Digest = ""
	forged, _ = contract.SealCleanupAttemptV2(forged)
	if err := contract.ValidateCleanupAttemptSuccessorV2(reconcile, forged); !contract.HasCode(err, contract.ErrorPrecondition) {
		t.Fatalf("state regression error=%v", err)
	}
}

func TestHostJournalV2WriteAheadAndReconciliationBlockRedispatch(t *testing.T) {
	now := time.Unix(1_900_000_000, 0)
	claim := startClaimV1(t, now)
	claimRef, _ := claim.RefV1()
	journal, err := contract.SealHostJournalV2(contract.HostJournalV2{ContractVersion: contract.HostJournalContractVersionV2, HostID: "host-1", StartID: "start-1", Revision: 1, Phase: contract.HostAcceptedV2, StartClaimRef: claimRef, ConfigDigest: claim.ConfigDigest, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	input := contract.HostOperationCoordinateV2{ContractKind: "praxis.agent-definition/source-current-v1", OwnerID: "praxis.agent-definition", ID: "source-1", Revision: 1, Digest: digestV1(t, "source-current"), Current: true, ExpiresUnixNano: now.Add(time.Hour).UnixNano()}
	attempt, err := contract.SealHostOperationAttemptV2(contract.HostOperationAttemptV2{ContractVersion: contract.HostJournalContractVersionV2, AttemptID: "operation-1", Revision: 1, OperationKind: "praxis.agent-host/validate-definition", Phase: contract.HostValidatingV2, RequestDigest: digestV1(t, "validate-request"), Inputs: []contract.HostOperationCoordinateV2{input}, State: contract.HostOperationIntentRecordedV2, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	withIntent := journal
	withIntent.Revision++
	withIntent.Phase = contract.HostValidatingV2
	withIntent.Operations = []contract.HostOperationAttemptV2{attempt}
	withIntent.UpdatedUnixNano++
	withIntent.Digest = ""
	withIntent, _ = contract.SealHostJournalV2(withIntent)
	if err := contract.ValidateHostJournalSuccessorV2(journal, withIntent); err != nil {
		t.Fatal(err)
	}

	unknownAttempt := attempt
	unknownAttempt.Revision++
	unknownAttempt.State = contract.HostOperationOutcomeUnknownV2
	unknownAttempt.UpdatedUnixNano++
	unknownAttempt.Digest = ""
	unknownAttempt, _ = contract.SealHostOperationAttemptV2(unknownAttempt)
	unknown := withIntent
	unknown.Revision++
	unknown.Operations = []contract.HostOperationAttemptV2{unknownAttempt}
	unknown.UpdatedUnixNano++
	unknown.Digest = ""
	unknown, _ = contract.SealHostJournalV2(unknown)
	if err := contract.ValidateHostJournalSuccessorV2(withIntent, unknown); err != nil {
		t.Fatal(err)
	}

	second := attempt
	second.AttemptID = "operation-2"
	second.OperationKind = "praxis.agent-host/resolve-plan"
	second.Phase = contract.HostResolvingV2
	second.RequestDigest = digestV1(t, "resolve-request")
	second.Digest = ""
	second, _ = contract.SealHostOperationAttemptV2(second)
	redispatch := unknown
	redispatch.Revision++
	redispatch.Phase = contract.HostResolvingV2
	redispatch.Operations = append([]contract.HostOperationAttemptV2{unknownAttempt}, second)
	redispatch.UpdatedUnixNano++
	redispatch.Digest = ""
	redispatch, _ = contract.SealHostJournalV2(redispatch)
	if err := contract.ValidateHostJournalSuccessorV2(unknown, redispatch); !contract.HasCode(err, contract.ErrorPrecondition) {
		t.Fatalf("redispatch error=%v", err)
	}
}

func TestControlAdapterFactoryV2BindsExactZeroEffectResources(t *testing.T) {
	now := time.Unix(1_900_000_000, 0)
	descriptor, conformance, _, request := controlAdapterFixtureV2(t, now)
	if err := descriptor.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := conformance.ValidateCurrent(descriptor.Ref, now); err != nil {
		t.Fatal(err)
	}
	if err := request.ValidateCurrent(now); err != nil {
		t.Fatal(err)
	}

	badEffect := descriptor
	badEffect.EffectClass = "network"
	badEffect.Ref.Digest, badEffect.DescriptorDigest = "", ""
	if _, err := contract.SealControlAdapterFactoryDescriptorV2(badEffect); !contract.HasCode(err, contract.ErrorPrecondition) {
		t.Fatalf("effect class error=%v", err)
	}

	wrongResource := request
	wrongResource.Descriptor.ResourceHandles = append([]runtimeports.ResourceHandleRefV1(nil), request.Descriptor.ResourceHandles...)
	wrongResource.Descriptor.ResourceHandles[0].ID = "resource/other"
	wrongResource.Descriptor.Ref.Digest, wrongResource.Descriptor.DescriptorDigest = "", ""
	wrongResource.Descriptor, _ = contract.SealControlAdapterFactoryDescriptorV2(wrongResource.Descriptor)
	wrongResource.Conformance.DescriptorRef = wrongResource.Descriptor.Ref
	wrongResource.Conformance.Digest = ""
	wrongResource.Conformance, _ = contract.SealControlAdapterConformanceV2(wrongResource.Conformance)
	wrongResource.RequestDigest = ""
	if _, err := contract.SealControlAdapterConstructRequestV2(wrongResource); !contract.HasCode(err, contract.ErrorConflict) {
		t.Fatalf("resource splice error=%v", err)
	}

	expired := request
	expired.RequestedNotAfterUnixNano = now.Add(3 * time.Hour).UnixNano()
	expired.RequestDigest = ""
	if _, err := contract.SealControlAdapterConstructRequestV2(expired); !contract.HasCode(err, contract.ErrorPrecondition) {
		t.Fatalf("ttl expansion error=%v", err)
	}
}

func TestControlAdapterInstanceV2IsExactRequestResult(t *testing.T) {
	now := time.Unix(1_900_000_000, 0)
	descriptor, _, _, request := controlAdapterFixtureV2(t, now)
	instance, err := contract.SealControlAdapterInstanceV2(contract.ControlAdapterInstanceV2{InstanceRef: exactRefV1(t, "praxis.agent-host/control-adapter-instance", "instance-1"), AttemptID: request.AttemptID, RequestDigest: request.RequestDigest, DescriptorRef: descriptor.Ref, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(30 * time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	if err := instance.ValidateCurrent(request, now); err != nil {
		t.Fatal(err)
	}
	forged := instance
	forged.RequestDigest = coreDigestV2(t, "other-request")
	forged.Digest = ""
	forged, _ = contract.SealControlAdapterInstanceV2(forged)
	if err := forged.ValidateCurrent(request, now); !contract.HasCode(err, contract.ErrorConflict) {
		t.Fatalf("request splice error=%v", err)
	}
}

func TestSystemReadyV2PublishesRuntimeNeutralAvailabilityAndFencesEpoch(t *testing.T) {
	now := time.Unix(1_900_000_000, 0)
	fact := systemReadyFactV2(t, now)
	if err := fact.ValidateCurrent(now); err != nil {
		t.Fatal(err)
	}
	current, err := contract.SealSystemReadyCurrentV2(contract.SystemReadyCurrentV2{Ref: contract.SystemReadyCurrentRefV2{ID: contract.DeriveSystemReadyCurrentIDV2("host-1", "start-1"), Revision: 1, Epoch: 1}, HostID: "host-1", StartID: "start-1", FactRef: fact.Ref, State: contract.SystemReadyCurrentReadyV2, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: fact.ExpiresUnixNano})
	if err != nil {
		t.Fatal(err)
	}
	availability, err := current.ToAgentExecutionAvailabilityV1(core.OwnerRef{Domain: "praxis.agent-host", ID: "host-owner"})
	if err != nil {
		t.Fatal(err)
	}
	if err := availability.ValidateCurrent(availability.Ref, now); err != nil {
		t.Fatal(err)
	}
	fenced := current
	fenced.Ref.Revision++
	fenced.State = contract.SystemReadyCurrentFencedV2
	fenced.CheckedUnixNano++
	fenced.Ref.Digest, fenced.ProjectionDigest = "", ""
	fenced, _ = contract.SealSystemReadyCurrentV2(fenced)
	if err := contract.ValidateSystemReadyCurrentSuccessorV2(current, fenced); err != nil {
		t.Fatal(err)
	}
	fencedAvailability, err := fenced.ToAgentExecutionAvailabilityV1(core.OwnerRef{Domain: "praxis.agent-host", ID: "host-owner"})
	if err != nil {
		t.Fatal(err)
	}
	if err := fencedAvailability.ValidateCurrent(fencedAvailability.Ref, now); err == nil {
		t.Fatal("fenced availability admitted new work")
	}
	reopen := fenced
	reopen.Ref.Revision++
	reopen.State = contract.SystemReadyCurrentReadyV2
	reopen.CheckedUnixNano++
	reopen.Ref.Digest, reopen.ProjectionDigest = "", ""
	reopen, _ = contract.SealSystemReadyCurrentV2(reopen)
	if err := contract.ValidateSystemReadyCurrentSuccessorV2(fenced, reopen); !contract.HasCode(err, contract.ErrorPrecondition) {
		t.Fatalf("reopen error=%v", err)
	}
}

func TestSystemReadyV2CanFenceAfterReadyFactExpiry(t *testing.T) {
	now := time.Unix(1_900_000_000, 0)
	fact := systemReadyFactV2(t, now)
	ready, err := contract.SealSystemReadyCurrentV2(contract.SystemReadyCurrentV2{Ref: contract.SystemReadyCurrentRefV2{ID: contract.DeriveSystemReadyCurrentIDV2("host-1", "start-1"), Revision: 1, Epoch: 7}, HostID: "host-1", StartID: "start-1", FactRef: fact.Ref, State: contract.SystemReadyCurrentReadyV2, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: fact.ExpiresUnixNano})
	if err != nil {
		t.Fatal(err)
	}
	checked := time.Unix(0, fact.ExpiresUnixNano).Add(time.Second)
	fenced, err := contract.SealSystemReadyCurrentV2(contract.SystemReadyCurrentV2{Ref: contract.SystemReadyCurrentRefV2{ID: ready.Ref.ID, Revision: 2, Epoch: ready.Ref.Epoch}, HostID: ready.HostID, StartID: ready.StartID, FactRef: ready.FactRef, State: contract.SystemReadyCurrentFencedV2, CheckedUnixNano: checked.UnixNano(), ExpiresUnixNano: checked.Add(time.Hour).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	if err := contract.ValidateSystemReadyCurrentSuccessorV2(ready, fenced); err != nil {
		t.Fatal(err)
	}
	oldAvailability, _ := ready.ToAgentExecutionAvailabilityV1(core.OwnerRef{Domain: "praxis.agent-host", ID: "host-owner"})
	newAvailability, err := fenced.ToAgentExecutionAvailabilityV1(core.OwnerRef{Domain: "praxis.agent-host", ID: "host-owner"})
	if err != nil {
		t.Fatal(err)
	}
	if err := runtimeports.ValidateAgentExecutionAvailabilityTransitionV1(oldAvailability, newAvailability); err != nil {
		t.Fatal(err)
	}
}

func TestSystemReadyV2RejectsTTLAboveComponentAndMinimumWindow(t *testing.T) {
	now := time.Unix(1_900_000_000, 0)
	fact := systemReadyFactV2(t, now)
	tooLong := fact
	tooLong.ExpiresUnixNano = now.Add(2 * time.Hour).UnixNano()
	tooLong.Ref.ExpiresUnixNano = tooLong.ExpiresUnixNano
	tooLong.Ref.ID, tooLong.Ref.Digest, tooLong.Digest = "", "", ""
	if _, err := contract.SealSystemReadyFactV2(tooLong); !contract.HasCode(err, contract.ErrorPrecondition) {
		t.Fatalf("ttl expansion error=%v", err)
	}
	tooShort := fact
	tooShort.ExpiresUnixNano = now.Add(time.Minute).UnixNano()
	tooShort.Ref.ExpiresUnixNano = tooShort.ExpiresUnixNano
	tooShort.Ref.ID, tooShort.Ref.Digest, tooShort.Digest = "", "", ""
	if _, err := contract.SealSystemReadyFactV2(tooShort); !contract.HasCode(err, contract.ErrorPrecondition) {
		t.Fatalf("minimum window error=%v", err)
	}
	claimLimited := fact
	claimLimited.HostStartClaim.ExpiresUnixNano = now.Add(20 * time.Minute).UnixNano()
	claimLimited.Ref.ID, claimLimited.Ref.Digest, claimLimited.Digest = "", "", ""
	if _, err := contract.SealSystemReadyFactV2(claimLimited); !contract.HasCode(err, contract.ErrorPrecondition) {
		t.Fatalf("claim TTL error=%v", err)
	}
	wrongID := fact
	wrongID.Ref.ID = "host-1/start-1/another-ready"
	wrongID.Ref.Digest, wrongID.Digest = "", ""
	if _, err := contract.SealSystemReadyFactV2(wrongID); !contract.HasCode(err, contract.ErrorConflict) {
		t.Fatalf("ready ID error=%v", err)
	}
}

func TestSystemReadyStoreAndAvailabilityReaderLinearizeOneFence(t *testing.T) {
	now := time.Unix(1_900_000_000, 0)
	fact := systemReadyFactV2(t, now)
	current, err := contract.SealSystemReadyCurrentV2(contract.SystemReadyCurrentV2{Ref: contract.SystemReadyCurrentRefV2{ID: contract.DeriveSystemReadyCurrentIDV2("host-1", "start-1"), Revision: 1, Epoch: 1}, HostID: "host-1", StartID: "start-1", FactRef: fact.Ref, State: contract.SystemReadyCurrentReadyV2, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: fact.ExpiresUnixNano})
	if err != nil {
		t.Fatal(err)
	}
	owner := core.OwnerRef{Domain: "praxis.agent-host", ID: "host-owner"}
	store, err := journal.NewMemorySystemReadyStoreV2(owner)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateSystemReadyFactV2(context.Background(), fact); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateSystemReadyCurrentV2(context.Background(), current); err != nil {
		t.Fatal(err)
	}
	reader, err := owneradapter.NewAvailabilityReaderV1(owner, store)
	if err != nil {
		t.Fatal(err)
	}
	expected, _ := current.ToAgentExecutionAvailabilityV1(owner)
	actual, err := reader.InspectAgentExecutionAvailabilityCurrentV1(context.Background(), expected.Ref)
	if err != nil || actual.Ref != expected.Ref {
		t.Fatalf("actual=%+v err=%v", actual, err)
	}

	fenced := current
	fenced.Ref.Revision++
	fenced.State = contract.SystemReadyCurrentFencedV2
	fenced.CheckedUnixNano++
	fenced.Ref.Digest, fenced.ProjectionDigest = "", ""
	fenced, _ = contract.SealSystemReadyCurrentV2(fenced)
	var success atomic.Int64
	var conflict atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := store.CompareAndSwapSystemReadyCurrentV2(context.Background(), current.Ref, fenced); err == nil {
				success.Add(1)
			} else if contract.HasCode(err, contract.ErrorConflict) {
				conflict.Add(1)
			} else {
				t.Errorf("CAS error=%v", err)
			}
		}()
	}
	wg.Wait()
	if success.Load() != 1 || conflict.Load() != 63 {
		t.Fatalf("success=%d conflict=%d", success.Load(), conflict.Load())
	}
	if _, err := reader.InspectAgentExecutionAvailabilityCurrentV1(context.Background(), expected.Ref); !contract.HasCode(err, contract.ErrorConflict) {
		t.Fatalf("stale availability error=%v", err)
	}
}

func startClaimV1(t *testing.T, now time.Time) contract.HostStartClaimV1 {
	t.Helper()
	value := contract.HostStartClaimV1{ContractVersion: contract.HostStartClaimContractVersionV1, HostContractVersion: contract.ContractVersionV1, HostID: "host-1", StartID: "start-1", ConfigDigest: digestV1(t, "host-config"), DefinitionSourceRef: exactRefV1(t, "praxis.agent-definition/source-current", "definition-source-1"), RequestedOperation: contract.HostStartOperationStartV1, CreatedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Hour).UnixNano()}
	sealed, err := contract.SealHostStartClaimV1(value)
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

func cleanupPlanV2(t *testing.T) contract.CleanupPlanV2 {
	t.Helper()
	node := func(id string, kind contract.CleanupNodeKindV2, class contract.CleanupResourceClassV2, deps ...string) contract.CleanupNodeV2 {
		owner := "owner-" + id
		if kind == contract.CleanupBarrierNodeV2 {
			switch id {
			case contract.CleanupBarrierHarnessCloseV2:
				owner = contract.CleanupBarrierOwnerHarnessV2
			case contract.CleanupBarrierSandboxFenceV2, contract.CleanupBarrierSandboxReleaseV2:
				owner = contract.CleanupBarrierOwnerSandboxV2
			case contract.CleanupBarrierRuntimeCleanupAggregateV2:
				owner = contract.CleanupBarrierOwnerRuntimeV2
			}
		}
		value := contract.CleanupNodeV2{NodeID: id, Kind: kind, OwnerComponentID: owner, CleanupContractRef: exactRefV1(t, "praxis.component/cleanup-contract", "cleanup-"+id), ResourceClass: class, RequiredBarrierIDs: deps, InspectPortBinding: exactRefV1(t, "praxis.binding/inspect-port", "inspect-"+id), RequestSchemaDigest: digestV1(t, "request-schema-"+id), ResultSchemaDigest: digestV1(t, "result-schema-"+id)}
		sealed, err := contract.SealCleanupNodeV2(value)
		if err != nil {
			t.Fatal(err)
		}
		return sealed
	}
	nodes := []contract.CleanupNodeV2{
		node("live-owner", contract.CleanupOwnerNodeV2, contract.CleanupLiveExecutionV2),
		node(contract.CleanupBarrierHarnessCloseV2, contract.CleanupBarrierNodeV2, contract.CleanupLiveExecutionV2, "live-owner"),
		node(contract.CleanupBarrierSandboxFenceV2, contract.CleanupBarrierNodeV2, contract.CleanupFencedSandboxLeaseV2, contract.CleanupBarrierHarnessCloseV2),
		node("lease-owner", contract.CleanupOwnerNodeV2, contract.CleanupFencedSandboxLeaseV2, contract.CleanupBarrierSandboxFenceV2),
		node(contract.CleanupBarrierSandboxReleaseV2, contract.CleanupBarrierNodeV2, contract.CleanupFencedSandboxLeaseV2, "lease-owner"),
		node(contract.CleanupBarrierRuntimeCleanupAggregateV2, contract.CleanupBarrierNodeV2, contract.CleanupHostControlHandleV2, contract.CleanupBarrierSandboxReleaseV2),
		node("host-handle", contract.CleanupOwnerNodeV2, contract.CleanupHostControlHandleV2, contract.CleanupBarrierRuntimeCleanupAggregateV2),
	}
	plan, err := contract.SealCleanupPlanV2(contract.CleanupPlanV2{ContractVersion: contract.CleanupContractVersionV2, PlanID: "cleanup-plan-1", Revision: 1, HostID: "host-1", StartID: "start-1", Nodes: nodes})
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func nodeIndex(nodes []contract.CleanupNodeV2, id string) int {
	for i := range nodes {
		if nodes[i].NodeID == id {
			return i
		}
	}
	return -1
}

func digestV1(t *testing.T, value string) contract.DigestV1 {
	t.Helper()
	digest, err := contract.DigestJSONV1(value)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

func exactRefV1(t *testing.T, kind, id string) contract.ExactRefV1 {
	t.Helper()
	return contract.ExactRefV1{Kind: kind, ID: id, Revision: 1, Digest: digestV1(t, kind+":"+id)}
}

func controlAdapterFixtureV2(t *testing.T, now time.Time) (contract.ControlAdapterFactoryDescriptorV2, contract.ControlAdapterConformanceV2, runtimeports.ResourceBindingSetV1, contract.ControlAdapterConstructRequestV2) {
	t.Helper()
	owner := core.OwnerRef{Domain: "fixture.resources", ID: "owner-1"}
	ownerCurrent := func(id string, expiry time.Time) runtimeports.OwnerCurrentRefV1 {
		return runtimeports.OwnerCurrentRefV1{Owner: owner, ContractVersion: "1.0.0", ID: id, Revision: 1, Digest: coreDigestV2(t, id), ExpiresUnixNano: expiry.UnixNano()}
	}
	expires := now.Add(time.Hour)
	cleanup := ownerCurrent("cleanup-current-1", expires)
	deployment := ownerCurrent("deployment-current-1", expires)
	handle, err := runtimeports.SealResourceHandleCurrentV1(runtimeports.ResourceHandleCurrentV1{Ref: runtimeports.ResourceHandleRefV1{Owner: owner, ID: "resource/db-1", Revision: 1, Kind: "fixture/sqlite", ScopeDigest: coreDigestV2(t, "resource-scope")}, CleanupContract: cleanup, DeploymentAttestation: deployment, CheckedUnixNano: now.Add(-time.Minute).UnixNano(), ExpiresUnixNano: expires.UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	resources, err := runtimeports.SealResourceBindingSetV1(runtimeports.ResourceBindingSetV1{Ref: runtimeports.ResourceBindingSetRefV1{ID: "resource-set-1", Revision: 1}, Bindings: []runtimeports.ResourceBindingV1{{ComponentID: "fixture/control-adapter", Handle: handle.Ref, CleanupContract: cleanup, DeploymentAttestation: deployment}}, CheckedUnixNano: now.Add(-time.Minute).UnixNano(), ExpiresUnixNano: expires.UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	descriptor, err := contract.SealControlAdapterFactoryDescriptorV2(contract.ControlAdapterFactoryDescriptorV2{Ref: contract.ControlAdapterFactoryRefV2{FactoryID: "factory/control-adapter-1", Revision: 1}, ComponentID: "fixture/control-adapter", ArtifactDigest: coreDigestV2(t, "factory-artifact"), ComponentContract: "1.0.0", Capability: "fixture/control", Binding: runtimeports.BindingAdmissionBindingRefV1{ComponentID: "fixture/control-adapter", ID: "binding-1", Revision: 1, Digest: coreDigestV2(t, "binding"), ExpiresUnixNano: expires.UnixNano()}, Generation: runtimeports.OwnerCurrentRefV1{Owner: core.OwnerRef{Domain: "fixture.assembly", ID: "owner-2"}, ContractVersion: "1.0.0", ID: "generation-current-1", Revision: 1, Digest: coreDigestV2(t, "generation"), ExpiresUnixNano: expires.UnixNano()}, ResourceBindingSet: resources.Ref, ResourceHandles: []runtimeports.ResourceHandleRefV1{handle.Ref}, OutputPortCapabilities: []runtimeports.CapabilityNameV2{"fixture/control-current"}, EffectClass: contract.ControlAdapterEffectNoneV2})
	if err != nil {
		t.Fatal(err)
	}
	evidenceOwner := core.OwnerRef{Domain: "fixture.certification", ID: "owner-3"}
	evidence := func(id string) runtimeports.OwnerCurrentRefV1 {
		return runtimeports.OwnerCurrentRefV1{Owner: evidenceOwner, ContractVersion: "1.0.0", ID: id, Revision: 1, Digest: coreDigestV2(t, id), ExpiresUnixNano: expires.UnixNano()}
	}
	conformance, err := contract.SealControlAdapterConformanceV2(contract.ControlAdapterConformanceV2{ConformanceID: "conformance-1", Revision: 1, DescriptorRef: descriptor.Ref, CertificationCurrent: evidence("certification-1"), StaticImportEvidence: evidence("static-import-1"), NoRawProviderEvidence: evidence("no-raw-provider-1"), ZeroEffectEvidence: evidence("zero-effect-1"), CheckedUnixNano: now.Add(-time.Minute).UnixNano(), ExpiresUnixNano: expires.UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	request, err := contract.SealControlAdapterConstructRequestV2(contract.ControlAdapterConstructRequestV2{HostID: "host-1", StartID: "start-1", AttemptID: "control-attempt-1", Descriptor: descriptor, Conformance: conformance, ResourceBindings: resources, RequestedNotAfterUnixNano: now.Add(45 * time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	return descriptor, conformance, resources, request
}

func coreDigestV2(t *testing.T, value string) core.Digest {
	t.Helper()
	digest, err := core.CanonicalJSONDigest("fixture", "1.0.0", "Fixture", value)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

func systemReadyFactV2(t *testing.T, now time.Time) contract.SystemReadyFactV2 {
	t.Helper()
	expires := now.Add(time.Hour)
	ownerRef := func(domain, id string) runtimeports.OwnerCurrentRefV1 {
		return runtimeports.OwnerCurrentRefV1{Owner: core.OwnerRef{Domain: domain, ID: core.OwnerID("owner-" + id)}, ContractVersion: "1.0.0", ID: id, Revision: 1, Digest: coreDigestV2(t, domain+":"+id), ExpiresUnixNano: expires.UnixNano()}
	}
	claim := startClaimV1(t, now)
	claimRef, err := claim.CurrentRefV1()
	if err != nil {
		t.Fatal(err)
	}
	component := contract.ComponentProductionCurrentV2{Domain: "praxis.test/component", ReleaseCurrent: ownerRef("praxis.release", "release-1"), ConstructedComponent: exactRefV1(t, "praxis.component/instance", "component-1"), Binding: runtimeports.BindingAdmissionBindingRefV1{ComponentID: "praxis.test/component", ID: "binding-1", Revision: 1, Digest: coreDigestV2(t, "binding-1"), ExpiresUnixNano: expires.UnixNano()}, GenerationCurrent: ownerRef("praxis.harness", "generation-1"), ActivationCurrent: ownerRef("praxis.runtime", "activation-1"), ProductionCurrent: ownerRef("praxis.test", "production-current-1")}
	fact, err := contract.SealSystemReadyFactV2(contract.SystemReadyFactV2{Ref: contract.SystemReadyFactRefV2{Revision: 1, ExpiresUnixNano: now.Add(30 * time.Minute).UnixNano()}, HostID: "host-1", StartID: "start-1", HostStartClaim: claimRef, DefinitionCurrent: ownerRef("praxis.agent-definition", "definition-1"), PlanCurrent: ownerRef("praxis.agent-assembler", "plan-1"), AssemblyCurrent: ownerRef("praxis.harness", "assembly-1"), BindingSetCurrent: ownerRef("praxis.runtime", "binding-set-1"), ActivationCurrent: ownerRef("praxis.runtime", "activation-1"), GenerationBindingCurrent: ownerRef("praxis.runtime", "generation-binding-1"), ApplicationStartCurrent: ownerRef("praxis.application", "start-current-1"), SandboxLeaseCurrent: ownerRef("praxis.sandbox", "sandbox-lease-1"), SandboxActiveCurrent: ownerRef("praxis.sandbox", "sandbox-active-1"), ExecutionReadyCurrent: ownerRef("praxis.harness", "execution-ready-1"), SupervisionPolicyCurrent: ownerRef("praxis.runtime", "supervision-policy-1"), Components: []contract.ComponentProductionCurrentV2{component}, MinimumReadyWindowNanos: int64(10 * time.Minute), CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(30 * time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	return fact
}
