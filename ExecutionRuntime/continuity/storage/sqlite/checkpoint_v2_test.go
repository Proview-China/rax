package sqlite_test

import (
	"context"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/domain"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
)

func TestStoreCheckpointManifestSealDurableCASAndHistory(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "continuity.db")
	store := openStore(t, path)
	controller, _ := domain.NewCheckpointManifestControllerV2(store)
	initial := testkit.ManifestV2(contract.ManifestCollecting, 1)
	if _, duplicate, err := controller.CreateCheckpointManifestV2(ctx, ports.CreateCheckpointManifestRequestV2{Candidate: initial, ExpectAbsent: true}); err != nil || duplicate {
		t.Fatalf("create duplicate=%v err=%v", duplicate, err)
	}
	if _, duplicate, err := controller.CreateCheckpointManifestV2(ctx, ports.CreateCheckpointManifestRequestV2{Candidate: initial, ExpectAbsent: true}); err != nil || !duplicate {
		t.Fatalf("lost create reply duplicate=%v err=%v", duplicate, err)
	}

	var winners atomic.Int32
	var unexpected atomic.Int32
	winner := make(chan contract.CheckpointManifestFactV2, 1)
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			next := testkit.ManifestV2(contract.ManifestVerifiedCandidate, 2)
			next.UpdatedUnixNano += int64(i)
			testkit.RefreshManifestV2(&next)
			got, duplicate, err := controller.CompareAndSwapCheckpointManifestV2(ctx, ports.CompareAndSwapCheckpointManifestRequestV2{Expected: initial.Ref(), Next: next})
			if err == nil && !duplicate {
				winners.Add(1)
				winner <- got
			} else if err != nil && !contract.HasCode(err, contract.ErrRevisionConflict) {
				unexpected.Add(1)
			}
		}(i)
	}
	wg.Wait()
	close(winner)
	if winners.Load() != 1 || unexpected.Load() != 0 {
		t.Fatalf("manifest CAS closure winners=%d unexpected=%d", winners.Load(), unexpected.Load())
	}
	verified := <-winner
	historical, err := controller.InspectCheckpointManifestV2(ctx, testkit.InspectManifestRequestV2(initial.Ref()))
	if err != nil || !historical.Ref().Exact().Equal(initial.Ref().Exact()) {
		t.Fatalf("history=%#v err=%v", historical, err)
	}

	sealWinner := make(chan contract.CheckpointManifestSealFactV2, 1)
	winners.Store(0)
	unexpected.Store(0)
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			seal := testkit.SealV2(verified)
			seal.SealID = "checkpoint-seal-" + decimal(i)
			seal.IdempotencyKey = "checkpoint-seal-request-" + decimal(i)
			testkit.RefreshSealV2(&seal)
			got, duplicate, err := controller.CreateCheckpointManifestSealV2(ctx, ports.CreateCheckpointManifestSealRequestV2{Seal: seal})
			if err == nil && !duplicate {
				winners.Add(1)
				sealWinner <- got
			} else if err != nil && !contract.HasCode(err, contract.ErrRevisionConflict) {
				unexpected.Add(1)
			}
		}(i)
	}
	wg.Wait()
	close(sealWinner)
	if winners.Load() != 1 || unexpected.Load() != 0 {
		t.Fatalf("seal create-once closure winners=%d unexpected=%d", winners.Load(), unexpected.Load())
	}
	seal := <-sealWinner
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store = openStore(t, path)
	defer store.Close()
	controller, _ = domain.NewCheckpointManifestControllerV2(store)
	current, err := controller.InspectCurrentCheckpointManifestV2(ctx, testkit.CurrentManifestRequestV2(verified))
	if err != nil || !current.Ref().Exact().Equal(verified.Ref().Exact()) {
		t.Fatalf("reopen current=%#v err=%v", current, err)
	}
	inspectedSeal, err := controller.InspectCheckpointManifestSealV2(ctx, testkit.InspectSealRequestV2(seal.Ref()))
	if err != nil || !inspectedSeal.Ref().Exact().Equal(seal.Ref().Exact()) {
		t.Fatalf("reopen seal=%#v err=%v", inspectedSeal, err)
	}
	if _, duplicate, err := controller.CreateCheckpointManifestSealV2(ctx, ports.CreateCheckpointManifestSealRequestV2{Seal: seal}); err != nil || !duplicate {
		t.Fatalf("lost seal reply duplicate=%v err=%v", duplicate, err)
	}
}
