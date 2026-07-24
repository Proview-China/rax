package domain_test

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/domain"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/storage/memory"
)

func TestCheckpointManifestControllerV2RejectsTypedNilRepository(t *testing.T) {
	var backend *memory.Backend
	controller, err := domain.NewCheckpointManifestControllerV2(backend)
	if controller != nil || !contract.HasCode(err, contract.ErrInvalidArgument) {
		t.Fatalf("typed-nil repository accepted: controller=%v err=%v", controller, err)
	}
}

func TestCheckpointManifestControllerV2CreateCASHistoryAndSeal(t *testing.T) {
	ctx := context.Background()
	backend := memory.New()
	controller, err := domain.NewCheckpointManifestControllerV2(backend)
	if err != nil {
		t.Fatal(err)
	}
	initial := testkit.ManifestV2(contract.ManifestCollecting, 1)
	created, replay, err := controller.CreateCheckpointManifestV2(ctx, ports.CreateCheckpointManifestRequestV2{
		Candidate: initial, ExpectAbsent: true,
	})
	if err != nil || replay || created.Digest != initial.Digest {
		t.Fatalf("create=%#v replay=%v err=%v", created, replay, err)
	}
	_, replay, err = controller.CreateCheckpointManifestV2(ctx, ports.CreateCheckpointManifestRequestV2{
		Candidate: initial, ExpectAbsent: true,
	})
	if err != nil || !replay {
		t.Fatalf("exact create replay not idempotent: replay=%v err=%v", replay, err)
	}

	final := testkit.ManifestV2(contract.ManifestVerifiedCandidate, 2)
	finalized, replay, err := controller.CompareAndSwapCheckpointManifestV2(ctx, ports.CompareAndSwapCheckpointManifestRequestV2{
		Expected: initial.Ref(), Next: final,
	})
	if err != nil || replay || finalized.Digest != final.Digest {
		t.Fatalf("finalize=%#v replay=%v err=%v", finalized, replay, err)
	}
	_, replay, err = controller.CompareAndSwapCheckpointManifestV2(ctx, ports.CompareAndSwapCheckpointManifestRequestV2{
		Expected: initial.Ref(), Next: final,
	})
	if err != nil || !replay {
		t.Fatalf("lost CAS reply replay not recognized: replay=%v err=%v", replay, err)
	}
	historical, err := controller.InspectCheckpointManifestV2(ctx, testkit.InspectManifestRequestV2(initial.Ref()))
	if err != nil || historical.Revision != 1 || historical.State != contract.ManifestCollecting {
		t.Fatalf("historical revision overwritten: %#v err=%v", historical, err)
	}
	current, err := controller.InspectCurrentCheckpointManifestV2(ctx, testkit.CurrentManifestRequestV2(initial))
	if err != nil || current.Revision != 2 || current.State != contract.ManifestVerifiedCandidate {
		t.Fatalf("current pointer drifted: %#v err=%v", current, err)
	}

	seal := testkit.SealV2(final)
	sealed, replay, err := controller.CreateCheckpointManifestSealV2(ctx, ports.CreateCheckpointManifestSealRequestV2{Seal: seal})
	if err != nil || replay || sealed.Revision != 1 {
		t.Fatalf("seal=%#v replay=%v err=%v", sealed, replay, err)
	}
	_, replay, err = controller.CreateCheckpointManifestSealV2(ctx, ports.CreateCheckpointManifestSealRequestV2{Seal: seal})
	if err != nil || !replay {
		t.Fatalf("exact seal replay not idempotent: replay=%v err=%v", replay, err)
	}
	inspected, err := controller.InspectCheckpointManifestSealV2(ctx, testkit.InspectSealRequestV2(seal.Ref()))
	if err != nil || inspected.Digest != seal.Digest || inspected.Revision != 1 {
		t.Fatalf("inspect seal=%#v err=%v", inspected, err)
	}
}

func TestCheckpointManifestControllerV2ConcurrentCASAndSealHaveOneCreate(t *testing.T) {
	ctx := context.Background()
	backend := memory.New()
	controller, _ := domain.NewCheckpointManifestControllerV2(backend)
	initial := testkit.ManifestV2(contract.ManifestCollecting, 1)
	if _, _, err := controller.CreateCheckpointManifestV2(ctx, ports.CreateCheckpointManifestRequestV2{Candidate: initial, ExpectAbsent: true}); err != nil {
		t.Fatal(err)
	}
	final := testkit.ManifestV2(contract.ManifestVerifiedCandidate, 2)
	var created atomic.Int32
	var failures atomic.Int32
	var wg sync.WaitGroup
	for range 64 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, replay, err := controller.CompareAndSwapCheckpointManifestV2(ctx, ports.CompareAndSwapCheckpointManifestRequestV2{
				Expected: initial.Ref(), Next: final,
			})
			if err != nil {
				failures.Add(1)
			} else if !replay {
				created.Add(1)
			}
		}()
	}
	wg.Wait()
	if created.Load() != 1 || failures.Load() != 0 {
		t.Fatalf("CAS created=%d failures=%d", created.Load(), failures.Load())
	}

	seal := testkit.SealV2(final)
	created.Store(0)
	failures.Store(0)
	for range 64 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, replay, err := controller.CreateCheckpointManifestSealV2(ctx, ports.CreateCheckpointManifestSealRequestV2{Seal: seal})
			if err != nil {
				failures.Add(1)
			} else if !replay {
				created.Add(1)
			}
		}()
	}
	wg.Wait()
	if created.Load() != 1 || failures.Load() != 0 {
		t.Fatalf("seal created=%d failures=%d", created.Load(), failures.Load())
	}
}

func TestCheckpointManifestControllerV2ConflictsOnChangedContent(t *testing.T) {
	ctx := context.Background()
	backend := memory.New()
	controller, _ := domain.NewCheckpointManifestControllerV2(backend)
	initial := testkit.ManifestV2(contract.ManifestCollecting, 1)
	if _, _, err := controller.CreateCheckpointManifestV2(ctx, ports.CreateCheckpointManifestRequestV2{Candidate: initial, ExpectAbsent: true}); err != nil {
		t.Fatal(err)
	}
	changed := initial.Clone()
	changed.FrozenRefSetDigest = "changed-frozen-ref-set-digest"
	changed.Digest, _ = changed.CanonicalDigest()
	if _, _, err := controller.CreateCheckpointManifestV2(ctx, ports.CreateCheckpointManifestRequestV2{Candidate: changed, ExpectAbsent: true}); !contract.HasCode(err, contract.ErrRevisionConflict) {
		t.Fatalf("same manifest ID changed content accepted: %v", err)
	}

	final := testkit.ManifestV2(contract.ManifestVerifiedCandidate, 2)
	if _, _, err := controller.CompareAndSwapCheckpointManifestV2(ctx, ports.CompareAndSwapCheckpointManifestRequestV2{Expected: initial.Ref(), Next: final}); err != nil {
		t.Fatal(err)
	}
	seal := testkit.SealV2(final)
	if _, _, err := controller.CreateCheckpointManifestSealV2(ctx, ports.CreateCheckpointManifestSealRequestV2{Seal: seal}); err != nil {
		t.Fatal(err)
	}
	changedSeal := seal.Clone()
	changedSeal.IdempotencyKey = "changed-seal-request"
	changedSeal.Digest, _ = changedSeal.CanonicalDigest()
	if _, _, err := controller.CreateCheckpointManifestSealV2(ctx, ports.CreateCheckpointManifestSealRequestV2{Seal: changedSeal}); !contract.HasCode(err, contract.ErrRevisionConflict) {
		t.Fatalf("same seal ID changed content accepted: %v", err)
	}
	changedClosure := seal.Clone()
	changedClosure.SealID = "checkpoint-manifest-seal-v2-changed-closure"
	changedClosure.IdempotencyKey = "checkpoint-manifest-seal-request-changed-closure"
	changedClosure.ParticipantClosures[0].EvidenceRefs[0].Digest = "changed-evidence-digest"
	changedClosure.Digest, _ = changedClosure.CanonicalDigest()
	if _, _, err := controller.CreateCheckpointManifestSealV2(ctx, ports.CreateCheckpointManifestSealRequestV2{Seal: changedClosure}); !contract.HasCode(err, contract.ErrRevisionConflict) {
		t.Fatalf("seal changed participant closure accepted: %v", err)
	}
}

func TestCheckpointManifestControllerV2ConcurrentDifferentCASHasOneWinner(t *testing.T) {
	ctx := context.Background()
	backend := memory.New()
	controller, _ := domain.NewCheckpointManifestControllerV2(backend)
	initial := testkit.ManifestV2(contract.ManifestCollecting, 1)
	if _, _, err := controller.CreateCheckpointManifestV2(ctx, ports.CreateCheckpointManifestRequestV2{Candidate: initial, ExpectAbsent: true}); err != nil {
		t.Fatal(err)
	}
	candidates := []contract.CheckpointManifestFactV2{
		testkit.ManifestV2(contract.ManifestVerifiedCandidate, 2),
		testkit.ManifestV2(contract.ManifestDiagnosticPartial, 2),
	}
	start := make(chan struct{})
	var successes atomic.Int32
	var conflicts atomic.Int32
	var wg sync.WaitGroup
	for _, candidate := range candidates {
		candidate := candidate
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, replay, err := controller.CompareAndSwapCheckpointManifestV2(ctx, ports.CompareAndSwapCheckpointManifestRequestV2{
				Expected: initial.Ref(), Next: candidate,
			})
			switch {
			case err == nil && !replay:
				successes.Add(1)
			case contract.HasCode(err, contract.ErrRevisionConflict):
				conflicts.Add(1)
			default:
				t.Errorf("unexpected concurrent CAS result replay=%v err=%v", replay, err)
			}
		}()
	}
	close(start)
	wg.Wait()
	if successes.Load() != 1 || conflicts.Load() != 1 {
		t.Fatalf("different CAS successes=%d conflicts=%d", successes.Load(), conflicts.Load())
	}
	current, err := controller.InspectCurrentCheckpointManifestV2(ctx, testkit.CurrentManifestRequestV2(initial))
	if err != nil || current.Revision != 2 {
		t.Fatalf("current after competing CAS=%#v err=%v", current, err)
	}
}

func TestAuditCheckpointManifestV2SixtyFourDifferentCASAndSealsHaveOneWinner(t *testing.T) {
	ctx := context.Background()
	backend := memory.New()
	controller, _ := domain.NewCheckpointManifestControllerV2(backend)
	initial := testkit.ManifestV2(contract.ManifestCollecting, 1)
	if _, _, err := controller.CreateCheckpointManifestV2(ctx, ports.CreateCheckpointManifestRequestV2{Candidate: initial, ExpectAbsent: true}); err != nil {
		t.Fatal(err)
	}
	var casWinners atomic.Int32
	var casConflicts atomic.Int32
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range 64 {
		candidate := testkit.ManifestV2(contract.ManifestVerifiedCandidate, 2)
		candidate.MemoryRefs = append(candidate.MemoryRefs, testkit.ExactRefV2(fmt.Sprintf("memory-variant-%d", i), "praxis/memory", "memory-watermark"))
		testkit.RefreshManifestV2(&candidate)
		wg.Add(1)
		go func(candidate contract.CheckpointManifestFactV2) {
			defer wg.Done()
			<-start
			_, replay, err := controller.CompareAndSwapCheckpointManifestV2(ctx, ports.CompareAndSwapCheckpointManifestRequestV2{Expected: initial.Ref(), Next: candidate})
			switch {
			case err == nil && !replay:
				casWinners.Add(1)
			case contract.HasCode(err, contract.ErrRevisionConflict):
				casConflicts.Add(1)
			default:
				t.Errorf("CAS replay=%v err=%v", replay, err)
			}
		}(candidate)
	}
	close(start)
	wg.Wait()
	if casWinners.Load() != 1 || casConflicts.Load() != 63 {
		t.Fatalf("64 different CAS winners=%d conflicts=%d", casWinners.Load(), casConflicts.Load())
	}
	current, err := controller.InspectCurrentCheckpointManifestV2(ctx, testkit.CurrentManifestRequestV2(initial))
	if err != nil {
		t.Fatal(err)
	}

	var sealWinners atomic.Int32
	var sealConflicts atomic.Int32
	start = make(chan struct{})
	for i := range 64 {
		seal := testkit.SealV2(current)
		seal.CreatedUnixNano += int64(i)
		testkit.RefreshSealV2(&seal)
		wg.Add(1)
		go func(seal contract.CheckpointManifestSealFactV2) {
			defer wg.Done()
			<-start
			_, replay, err := controller.CreateCheckpointManifestSealV2(ctx, ports.CreateCheckpointManifestSealRequestV2{Seal: seal})
			switch {
			case err == nil && !replay:
				sealWinners.Add(1)
			case contract.HasCode(err, contract.ErrRevisionConflict):
				sealConflicts.Add(1)
			default:
				t.Errorf("seal replay=%v err=%v", replay, err)
			}
		}(seal)
	}
	close(start)
	wg.Wait()
	if sealWinners.Load() != 1 || sealConflicts.Load() != 63 {
		t.Fatalf("64 different Seals winners=%d conflicts=%d", sealWinners.Load(), sealConflicts.Load())
	}
}
