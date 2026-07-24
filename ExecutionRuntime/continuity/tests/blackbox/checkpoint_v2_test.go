package blackbox_test

import (
	"context"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/domain"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/storage/memory"
)

func TestCheckpointManifestGovernancePortV2PublicHappyPathStopsAtSeal(t *testing.T) {
	ctx := context.Background()
	backend := memory.New()
	controller, err := domain.NewCheckpointManifestControllerV2(backend)
	if err != nil {
		t.Fatal(err)
	}
	var port ports.CheckpointManifestGovernancePortV2 = controller
	initial := testkit.ManifestV2(contract.ManifestCollecting, 1)
	if _, replay, err := port.CreateCheckpointManifestV2(ctx, ports.CreateCheckpointManifestRequestV2{Candidate: initial, ExpectAbsent: true}); err != nil || replay {
		t.Fatalf("create replay=%v err=%v", replay, err)
	}
	final := testkit.ManifestV2(contract.ManifestVerifiedCandidate, 2)
	if _, replay, err := port.CompareAndSwapCheckpointManifestV2(ctx, ports.CompareAndSwapCheckpointManifestRequestV2{Expected: initial.Ref(), Next: final}); err != nil || replay {
		t.Fatalf("finalize replay=%v err=%v", replay, err)
	}
	seal := testkit.SealV2(final)
	if _, replay, err := port.CreateCheckpointManifestSealV2(ctx, ports.CreateCheckpointManifestSealRequestV2{Seal: seal}); err != nil || replay {
		t.Fatalf("seal replay=%v err=%v", replay, err)
	}
	got, err := port.InspectCheckpointManifestSealV2(ctx, testkit.InspectSealRequestV2(seal.Ref()))
	if err != nil || got.Ref() != seal.Ref() || got.Revision != 1 {
		t.Fatalf("seal inspection=%#v err=%v", got, err)
	}
	// This public path intentionally ends at the immutable Continuity seal. It
	// exposes no Runtime consistency mutation, Restore, Provider, or activation.
}
