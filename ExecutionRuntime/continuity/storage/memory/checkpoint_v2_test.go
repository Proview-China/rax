package memory_test

import (
	"context"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/domain"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/storage/memory"
)

func TestCheckpointManifestV2RepositoryReturnsDeepClones(t *testing.T) {
	ctx := context.Background()
	backend := memory.New()
	controller, _ := domain.NewCheckpointManifestControllerV2(backend)
	initial := testkit.ManifestV2(contract.ManifestCollecting, 1)
	created, _, err := controller.CreateCheckpointManifestV2(ctx, ports.CreateCheckpointManifestRequestV2{Candidate: initial, ExpectAbsent: true})
	if err != nil {
		t.Fatal(err)
	}
	created.ContextFrameRefs[0].ID = "caller-mutated-frame"
	created.ParticipantClosures[0].SnapshotRef.ID = "caller-mutated-snapshot"
	inspected, err := controller.InspectCheckpointManifestV2(ctx, testkit.InspectManifestRequestV2(initial.Ref()))
	if err != nil || inspected.ContextFrameRefs[0].ID == "caller-mutated-frame" || inspected.ParticipantClosures[0].SnapshotRef.ID == "caller-mutated-snapshot" {
		t.Fatalf("manifest repository leaked alias: %#v err=%v", inspected, err)
	}

	final := testkit.ManifestV2(contract.ManifestVerifiedCandidate, 2)
	if _, _, err := controller.CompareAndSwapCheckpointManifestV2(ctx, ports.CompareAndSwapCheckpointManifestRequestV2{Expected: initial.Ref(), Next: final}); err != nil {
		t.Fatal(err)
	}
	seal := testkit.SealV2(final)
	createdSeal, _, err := controller.CreateCheckpointManifestSealV2(ctx, ports.CreateCheckpointManifestSealRequestV2{Seal: seal})
	if err != nil {
		t.Fatal(err)
	}
	createdSeal.ParticipantClosures[0].EvidenceRefs[0].ID = "caller-mutated-evidence"
	inspectedSeal, err := controller.InspectCheckpointManifestSealV2(ctx, testkit.InspectSealRequestV2(seal.Ref()))
	if err != nil || inspectedSeal.ParticipantClosures[0].EvidenceRefs[0].ID == "caller-mutated-evidence" {
		t.Fatalf("seal repository leaked alias: %#v err=%v", inspectedSeal, err)
	}
}
