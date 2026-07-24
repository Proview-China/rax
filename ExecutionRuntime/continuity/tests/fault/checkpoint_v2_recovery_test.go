package fault_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/domain"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/fakes"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/storage/memory"
)

func TestCheckpointManifestV2LostRepliesRecoverOnlyByOriginalInspect(t *testing.T) {
	ctx := context.Background()
	backend := memory.New()
	controller, _ := domain.NewCheckpointManifestControllerV2(backend)
	faults, err := fakes.NewCheckpointManifestGovernanceV2(controller)
	if err != nil {
		t.Fatal(err)
	}
	lost := errors.New("injected durable-write reply loss")

	initial := testkit.ManifestV2(contract.ManifestCollecting, 1)
	faults.LoseNextSuccessfulReply(fakes.CheckpointCreateManifestV2, lost)
	if _, _, err := faults.CreateCheckpointManifestV2(ctx, ports.CreateCheckpointManifestRequestV2{Candidate: initial, ExpectAbsent: true}); !errors.Is(err, lost) {
		t.Fatalf("create lost reply not injected: %v", err)
	}
	inspectedInitial, err := faults.InspectCurrentCheckpointManifestV2(ctx, testkit.CurrentManifestRequestV2(initial))
	if err != nil || inspectedInitial.Ref() != initial.Ref() {
		t.Fatalf("create lost reply exact Inspect=%#v err=%v", inspectedInitial, err)
	}
	alternate := initial.Clone()
	alternate.ManifestID = "checkpoint-manifest-v2-alternate"
	alternate.Digest, _ = alternate.CanonicalDigest()
	if _, _, err := faults.CreateCheckpointManifestV2(ctx, ports.CreateCheckpointManifestRequestV2{Candidate: alternate, ExpectAbsent: true}); !contract.HasCode(err, contract.ErrRevisionConflict) {
		t.Fatalf("lost create reply allowed replacement identity: %v", err)
	}

	final := testkit.ManifestV2(contract.ManifestVerifiedCandidate, 2)
	faults.LoseNextSuccessfulReply(fakes.CheckpointCASManifestV2, lost)
	if _, _, err := faults.CompareAndSwapCheckpointManifestV2(ctx, ports.CompareAndSwapCheckpointManifestRequestV2{Expected: initial.Ref(), Next: final}); !errors.Is(err, lost) {
		t.Fatalf("CAS lost reply not injected: %v", err)
	}
	inspectedFinal, err := faults.InspectCheckpointManifestV2(ctx, testkit.InspectManifestRequestV2(final.Ref()))
	if err != nil || inspectedFinal.Ref() != final.Ref() {
		t.Fatalf("CAS lost reply exact Inspect=%#v err=%v", inspectedFinal, err)
	}

	seal := testkit.SealV2(final)
	faults.LoseNextSuccessfulReply(fakes.CheckpointCreateSealV2, lost)
	if _, _, err := faults.CreateCheckpointManifestSealV2(ctx, ports.CreateCheckpointManifestSealRequestV2{Seal: seal}); !errors.Is(err, lost) {
		t.Fatalf("seal lost reply not injected: %v", err)
	}
	inspectedSeal, err := faults.InspectCheckpointManifestSealV2(ctx, testkit.InspectSealRequestV2(seal.Ref()))
	if err != nil || inspectedSeal.Ref() != seal.Ref() {
		t.Fatalf("seal lost reply exact Inspect=%#v err=%v", inspectedSeal, err)
	}
	alternateSeal := seal.Clone()
	alternateSeal.SealID = "checkpoint-manifest-seal-v2-alternate"
	alternateSeal.Digest, _ = alternateSeal.CanonicalDigest()
	if _, _, err := faults.CreateCheckpointManifestSealV2(ctx, ports.CreateCheckpointManifestSealRequestV2{Seal: alternateSeal}); !contract.HasCode(err, contract.ErrRevisionConflict) {
		t.Fatalf("lost seal reply allowed replacement identity: %v", err)
	}
}

func TestCheckpointManifestV2UnknownFinalizesDiagnosticAndCannotSeal(t *testing.T) {
	ctx := context.Background()
	backend := memory.New()
	controller, _ := domain.NewCheckpointManifestControllerV2(backend)
	initial := testkit.ManifestV2(contract.ManifestCollecting, 1)
	if _, _, err := controller.CreateCheckpointManifestV2(ctx, ports.CreateCheckpointManifestRequestV2{Candidate: initial, ExpectAbsent: true}); err != nil {
		t.Fatal(err)
	}
	unknown := testkit.ManifestV2(contract.ManifestDiagnosticIndeterminate, 2)
	if _, _, err := controller.CompareAndSwapCheckpointManifestV2(ctx, ports.CompareAndSwapCheckpointManifestRequestV2{Expected: initial.Ref(), Next: unknown}); err != nil {
		t.Fatalf("diagnostic finalization rejected: %v", err)
	}
	seal := testkit.SealV2(unknown)
	if _, _, err := controller.CreateCheckpointManifestSealV2(ctx, ports.CreateCheckpointManifestSealRequestV2{Seal: seal}); !contract.HasCode(err, contract.ErrCheckpointPartial) {
		t.Fatalf("unknown finalization produced a seal: %v", err)
	}
	current, err := controller.InspectCurrentCheckpointManifestV2(ctx, testkit.CurrentManifestRequestV2(initial))
	if err != nil || current.State != contract.ManifestDiagnosticIndeterminate || len(current.ResidualRefs) == 0 || current.AttemptSettlementClosures[0].InspectionRef == nil {
		t.Fatalf("unknown did not remain exact Inspect/residual diagnostic: %#v err=%v", current, err)
	}
}

func TestCheckpointManifestV2ProgressedLostReplyRejectsOldCASReplayWithoutABA(t *testing.T) {
	ctx := context.Background()
	backend := memory.New()
	controller, _ := domain.NewCheckpointManifestControllerV2(backend)
	faults, err := fakes.NewCheckpointManifestGovernanceV2(controller)
	if err != nil {
		t.Fatal(err)
	}
	lost := errors.New("injected progressed CAS reply loss")

	revisionOne := testkit.ManifestV2(contract.ManifestCollecting, 1)
	if _, _, err := faults.CreateCheckpointManifestV2(ctx, ports.CreateCheckpointManifestRequestV2{Candidate: revisionOne, ExpectAbsent: true}); err != nil {
		t.Fatal(err)
	}
	revisionTwo := testkit.ManifestV2(contract.ManifestCollecting, 2)
	faults.LoseNextSuccessfulReply(fakes.CheckpointCASManifestV2, lost)
	if _, _, err := faults.CompareAndSwapCheckpointManifestV2(ctx, ports.CompareAndSwapCheckpointManifestRequestV2{
		Expected: revisionOne.Ref(), Next: revisionTwo,
	}); !errors.Is(err, lost) {
		t.Fatalf("CAS lost reply not injected: %v", err)
	}
	inspectedTwo, err := faults.InspectCheckpointManifestV2(ctx, testkit.InspectManifestRequestV2(revisionTwo.Ref()))
	if err != nil || inspectedTwo.Ref() != revisionTwo.Ref() {
		t.Fatalf("lost reply recovery did not Inspect exact revision 2: %#v err=%v", inspectedTwo, err)
	}

	revisionThree := testkit.ManifestV2(contract.ManifestVerifiedCandidate, 3)
	if _, replay, err := faults.CompareAndSwapCheckpointManifestV2(ctx, ports.CompareAndSwapCheckpointManifestRequestV2{
		Expected: revisionTwo.Ref(), Next: revisionThree,
	}); err != nil || replay {
		t.Fatalf("progress to revision 3 failed or was reported replay: replay=%v err=%v", replay, err)
	}
	if _, _, err := faults.CompareAndSwapCheckpointManifestV2(ctx, ports.CompareAndSwapCheckpointManifestRequestV2{
		Expected: revisionOne.Ref(), Next: revisionTwo,
	}); !contract.HasCode(err, contract.ErrRevisionConflict) {
		t.Fatalf("progressed old lost-reply CAS replay was accepted or treated idempotent: %v", err)
	}

	current, err := faults.InspectCurrentCheckpointManifestV2(ctx, testkit.CurrentManifestRequestV2(revisionOne))
	if err != nil || current.Ref() != revisionThree.Ref() {
		t.Fatalf("old replay changed current revision or caused ABA: current=%#v err=%v", current, err)
	}
	for _, historical := range []contract.CheckpointManifestFactV2{revisionOne, revisionTwo, revisionThree} {
		got, err := faults.InspectCheckpointManifestV2(ctx, testkit.InspectManifestRequestV2(historical.Ref()))
		if err != nil || got.Ref() != historical.Ref() {
			t.Fatalf("immutable history changed after progressed replay for revision %d: %#v err=%v", historical.Revision, got, err)
		}
	}
}
