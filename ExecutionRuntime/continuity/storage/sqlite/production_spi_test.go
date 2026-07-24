package sqlite

import (
	"context"
	"encoding/json"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
)

func TestProductionSPIStrictDecodeRejectsUnknownDuplicateAndTrailingJSON(t *testing.T) {
	manifest := testkit.ManifestV2(contract.ManifestCollecting, 1)
	body, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	inputs := [][]byte{
		append([]byte(`{"unknown":true,`), body[1:]...),
		append([]byte(`{"contract_version":"duplicate",`), body[1:]...),
		append(append([]byte(nil), body...), []byte(` {}`)...),
	}
	for index, input := range inputs {
		var decoded contract.CheckpointManifestFactV2
		if err := decode(input, &decoded); !contract.HasCode(err, contract.ErrContentDigestMismatch) {
			t.Fatalf("strict JSON case %d accepted: %v", index, err)
		}
	}
}

func TestProductionSPICurrentReadersRejectTypedNilStoreAndNilContext(t *testing.T) {
	var typedNil *Store
	manifest := testkit.ManifestV2(contract.ManifestCollecting, 1)
	if _, err := typedNil.InspectCurrentCheckpointManifestV2(context.Background(), testkit.CurrentManifestRequestV2(manifest)); !contract.HasCode(err, contract.ErrInvalidArgument) {
		t.Fatalf("typed-nil current reader error = %v", err)
	}

	store, err := Open(context.Background(), filepath.Join(t.TempDir(), "continuity.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.InspectCurrentCheckpointManifestV2(nil, testkit.CurrentManifestRequestV2(manifest)); !contract.HasCode(err, contract.ErrInvalidArgument) {
		t.Fatalf("nil-context current reader error = %v", err)
	}
}

func TestProductionSPI64StoreObjectsShareOneDurableCASLinearization(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "continuity.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	initial := testkit.ManifestV2(contract.ManifestCollecting, 1)
	if _, replay, err := store.CreateCheckpointManifestFactV2(ctx, initial); err != nil || replay {
		t.Fatalf("create = (%v,%v)", replay, err)
	}

	stores := make([]*Store, 64)
	for index := range stores {
		stores[index] = &Store{db: store.db, clock: store.clock}
	}
	var winners atomic.Int32
	var conflicts atomic.Int32
	var unexpected atomic.Int32
	var winnerMu sync.Mutex
	var winner contract.CheckpointManifestFactV2
	var wait sync.WaitGroup
	for index, candidateStore := range stores {
		index, candidateStore := index, candidateStore
		wait.Add(1)
		go func() {
			defer wait.Done()
			next := testkit.ManifestV2(contract.ManifestVerifiedCandidate, 2)
			next.UpdatedUnixNano += int64(index)
			testkit.RefreshManifestV2(&next)
			got, replay, err := candidateStore.CompareAndSwapCheckpointManifestFactV2(ctx, initial.Ref(), next)
			switch {
			case err == nil && !replay:
				winners.Add(1)
				winnerMu.Lock()
				winner = got
				winnerMu.Unlock()
			case contract.HasCode(err, contract.ErrRevisionConflict):
				conflicts.Add(1)
			default:
				unexpected.Add(1)
			}
		}()
	}
	wait.Wait()
	if winners.Load() != 1 || conflicts.Load() != 63 || unexpected.Load() != 0 {
		t.Fatalf("64 Store CAS = winners %d conflicts %d unexpected %d", winners.Load(), conflicts.Load(), unexpected.Load())
	}

	current, err := store.InspectCurrentCheckpointManifestV2(ctx, testkit.CurrentManifestRequestV2(initial))
	if err != nil || !current.Ref().Exact().Equal(winner.Ref().Exact()) {
		t.Fatalf("current = (%+v,%v), winner=%+v", current.Ref(), err, winner.Ref())
	}
	historical, err := store.InspectCheckpointManifestV2(ctx, ports.InspectCheckpointManifestRequestV2{Ref: initial.Ref()})
	if err != nil || !historical.Ref().Exact().Equal(initial.Ref().Exact()) {
		t.Fatalf("history = (%+v,%v)", historical.Ref(), err)
	}
}

var _ ProductionSPI = (*Store)(nil)
