package fakes_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/fakes"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestRunClaimAssociationStoreV2LostReplyInspectsAndReplaysExactFact(t *testing.T) {
	fact := runClaimAssociationFactV2(t, "lost", 1, core.RunClaimCompleted)
	store := fakes.NewRunClaimAssociationStoreV2()
	store.LoseNextReply()
	if _, err := store.CreateRunClaimAssociation(context.Background(), fact); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("create reply must be lost after durable write: %v", err)
	}
	inspected, err := store.InspectRunClaimAssociation(context.Background(), fact.ExecutionScopeDigest, fact.RunID)
	if err != nil {
		t.Fatal(err)
	}
	wantDigest, _ := fact.DigestV2()
	gotDigest, _ := inspected.DigestV2()
	if gotDigest != wantDigest {
		t.Fatalf("inspect must return exact durable association: got %s want %s", gotDigest, wantDigest)
	}
	replayed, err := store.CreateRunClaimAssociation(context.Background(), fact)
	if err != nil {
		t.Fatal(err)
	}
	replayDigest, _ := replayed.DigestV2()
	if replayDigest != wantDigest {
		t.Fatalf("same semantic create must be idempotent: got %s want %s", replayDigest, wantDigest)
	}
}

func TestRunClaimAssociationStoreV2ConcurrentDifferentClaimsLinearizeOnce(t *testing.T) {
	store := fakes.NewRunClaimAssociationStoreV2()
	facts := []ports.RunClaimAssociationFactV2{
		runClaimAssociationFactV2(t, "first", 1, core.RunClaimCompleted),
		runClaimAssociationFactV2(t, "second", 2, core.RunClaimFailed),
	}
	var wait sync.WaitGroup
	var mu sync.Mutex
	successes := 0
	conflicts := 0
	for _, fact := range facts {
		fact := fact
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, err := store.CreateRunClaimAssociation(context.Background(), fact)
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				successes++
			} else if core.HasReason(err, core.ReasonRunClaimConflict) {
				conflicts++
			} else {
				t.Errorf("unexpected create error: %v", err)
			}
		}()
	}
	wait.Wait()
	if successes != 1 || conflicts != 1 {
		t.Fatalf("create-once association must linearize one winner: successes=%d conflicts=%d", successes, conflicts)
	}
}

func runClaimAssociationFactV2(t *testing.T, suffix string, sequence uint64, kind core.RunCompletionClaimKind) ports.RunClaimAssociationFactV2 {
	t.Helper()
	now := time.Unix(116_000, 0)
	scope := core.ExecutionScope{Identity: core.AgentIdentityRef{TenantID: "tenant-association", ID: "identity-association", Epoch: 1}, Lineage: core.LineageRef{ID: "lineage-association", PlanDigest: evidenceDigestV2(t, "association-plan")}, Instance: core.InstanceRef{ID: "instance-association", Epoch: 1}, AuthorityEpoch: 1}
	run := core.AgentRunRecord{ID: "run-association", Scope: scope, Status: core.RunRunning, Revision: 1, SessionRef: "session-association", StartedAt: now.Add(-time.Second)}
	candidate := ports.EvidenceEventCandidateV2{ContractVersion: ports.EvidenceContractVersionV2, LedgerScope: ports.EvidenceLedgerScopeV2{Partition: ports.EvidencePartitionRun, TenantID: scope.Identity.TenantID, IdentityID: scope.Identity.ID, LineageID: scope.Lineage.ID, InstanceID: scope.Instance.ID, RunID: run.ID}, EventID: "event-" + suffix, RegistrationID: "registration-association", RegistrationRevision: 1, SourceConfigurationDigest: evidenceDigestV2(t, "association-configuration"), SourcePolicy: ports.EvidenceSourcePolicyBindingRefV2{Ref: "policy-association", Digest: evidenceDigestV2(t, "association-policy"), Revision: 1}, SourceID: "runtime/claim-source", SourceEpoch: 1, SourceSequence: sequence, TrustClass: ports.EvidenceTrustClaim, ClaimKind: kind, EventKind: "runtime/run-completion", CustomClass: "runtime/claim", ExecutionScope: scope, Payload: ports.EvidencePayloadRefV2{Schema: ports.SchemaRefV2{Namespace: "runtime", Name: "run-claim", Version: "2.0.0", MediaType: "application/octet-stream", ContentDigest: evidenceDigestV2(t, "association-schema")}, ContentDigest: evidenceDigestV2(t, "payload-"+suffix), Revision: 1, Length: 1, Ref: "memory://" + suffix}, Causation: []ports.EvidenceCausationRefV2{}, CorrelationID: string(run.ID), Producer: ports.EvidenceProducerBindingRefV2{BindingSetID: "binding-association", BindingSetRevision: 1, ComponentID: "runtime/harness", ManifestDigest: evidenceDigestV2(t, "association-manifest"), ArtifactDigest: evidenceDigestV2(t, "association-artifact"), Capability: "runtime/claim"}, Authority: ports.AuthorityBindingRefV2{Ref: "authority-association", Digest: evidenceDigestV2(t, "association-authority"), Revision: 1, Epoch: 1}, ObservedUnixNano: now.UnixNano()}
	record, err := control.NewEvidenceLedgerRecordV2(candidate, sequence, ports.EvidenceGenesisDigestV2, now)
	if err != nil {
		t.Fatal(err)
	}
	scopeDigest, _ := ports.ExecutionScopeDigestV2(scope)
	runIdentity, _ := ports.RunIdentityDigestV2(run)
	id, _ := ports.RunClaimAssociationIDV2(run.ID, record.Ref)
	return ports.RunClaimAssociationFactV2{ContractVersion: ports.RunClaimAssociationContractVersionV2, ID: id, Revision: 1, State: ports.RunClaimAssociatedV2, RunID: run.ID, RunRevisionAtAssociation: run.Revision, RunIdentityDigest: runIdentity, ExecutionScope: scope, ExecutionScopeDigest: scopeDigest, LineagePlanDigest: scope.Lineage.PlanDigest, ClaimKind: kind, RegistrationID: candidate.RegistrationID, SourceID: candidate.SourceID, SourceEpoch: candidate.SourceEpoch, SourceSequence: candidate.SourceSequence, EventID: candidate.EventID, Evidence: record.Ref, CandidateDigest: record.CandidateDigest, PayloadDigest: candidate.Payload.ContentDigest, ObservedUnixNano: candidate.ObservedUnixNano, EvidenceIngestedUnixNano: record.IngestedUnixNano, CreatedUnixNano: record.IngestedUnixNano}
}
