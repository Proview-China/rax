package assemblypublication

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycompiler"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	assemblytestkit "github.com/Proview-China/rax/ExecutionRuntime/harness/tests/assembly/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func TestSQLitePublicationV2PartialStagingIsInvisibleAcrossRestart(t *testing.T) {
	ctx := context.Background()
	path := t.TempDir() + "/publication.db"
	bundle := sqliteTestBundleV2(t)
	store := openSQLitePublicationTestStoreV2(t, path)
	stages := []func() error{
		func() error { return store.StageGenerationV2(ctx, bundle.Publication.PublicationID, bundle.Generation) },
		func() error { return store.StageManifestV2(ctx, bundle.Publication.PublicationID, bundle.Manifest) },
		func() error { return store.StageGraphV2(ctx, bundle.Publication.PublicationID, bundle.Graph) },
		func() error { return store.StageHandoffV2(ctx, bundle.Publication.PublicationID, bundle.Handoff) },
	}
	ref := publicationRefV2(bundle)
	for index, stage := range stages {
		if err := stage(); err != nil {
			t.Fatalf("stage %d: %v", index, err)
		}
		if err := store.Close(); err != nil {
			t.Fatal(err)
		}
		store = openSQLitePublicationTestStoreV2(t, path)
		if _, err := store.InspectHistoricalPublicationV2(ctx, ref); !core.HasCategory(err, core.ErrorNotFound) {
			t.Fatalf("stage %d leaked history: %v", index, err)
		}
		if _, err := store.InspectCurrentPublicationV2(ctx, bundle.Publication.ScopeRef); !core.HasCategory(err, core.ErrorNotFound) {
			t.Fatalf("stage %d leaked current: %v", index, err)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestSQLitePublicationV2AtomicCommitLostReplyAndExactRestartRecovery(t *testing.T) {
	ctx := context.Background()
	path := t.TempDir() + "/publication.db"
	bundle := sqliteTestBundleV2(t)
	current := sqliteTestCurrentV2(t, bundle, 1, "sqlite-lost")
	store := openSQLitePublicationTestStoreV2(t, path)
	stageSQLiteBundleV2(t, store, bundle)
	store.faultMu.Lock()
	store.loseNextReply = true
	store.faultMu.Unlock()
	if _, err := store.CommitPublicationCurrentV2(ctx, CommitPublicationCurrentRequestV2{Bundle: bundle, Current: current}); !core.HasCategory(err, core.ErrorIndeterminate) {
		t.Fatalf("lost commit reply = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store = openSQLitePublicationTestStoreV2(t, path)
	defer store.Close()
	ref := publicationRefV2(bundle)
	historical, err := store.InspectHistoricalPublicationV2(ctx, ref)
	if err != nil || historical.Publication.Digest != bundle.Publication.Digest {
		t.Fatalf("historical recovery = %+v, %v", historical.Publication, err)
	}
	committed, err := store.InspectCommittedPublicationCurrentV2(ctx, ref)
	if err != nil || committed != current {
		t.Fatalf("committed-current recovery = %+v, %v", committed, err)
	}
	latest, err := store.InspectCurrentPublicationV2(ctx, bundle.Publication.ScopeRef)
	if err != nil || latest != current {
		t.Fatalf("latest current recovery = %+v, %v", latest, err)
	}
	if err := store.IntegrityCheckV2(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestSQLitePublicationV2EveryStageLostReplyRecoversAfterRestart(t *testing.T) {
	ctx := context.Background()
	for stageIndex := 0; stageIndex < 4; stageIndex++ {
		path := t.TempDir() + "/publication.db"
		bundle := sqliteTestBundleV2(t)
		store := openSQLitePublicationTestStoreV2(t, path)
		stages := []struct {
			write func() error
			want  core.Digest
		}{
			{func() error { return store.StageGenerationV2(ctx, bundle.Publication.PublicationID, bundle.Generation) }, bundle.Generation.Digest},
			{func() error { return store.StageManifestV2(ctx, bundle.Publication.PublicationID, bundle.Manifest) }, bundle.Manifest.Digest},
			{func() error { return store.StageGraphV2(ctx, bundle.Publication.PublicationID, bundle.Graph) }, bundle.Graph.Digest},
			{func() error { return store.StageHandoffV2(ctx, bundle.Publication.PublicationID, bundle.Handoff) }, bundle.Handoff.Digest},
		}
		store.faultMu.Lock()
		store.loseNextReply = true
		store.faultMu.Unlock()
		if err := stages[stageIndex].write(); !core.HasCategory(err, core.ErrorIndeterminate) {
			t.Fatalf("stage %d lost reply = %v", stageIndex, err)
		}
		if err := store.Close(); err != nil {
			t.Fatal(err)
		}
		store = openSQLitePublicationTestStoreV2(t, path)
		inspection, err := store.InspectStagedPublicationV2(ctx, bundle.Publication.PublicationID)
		if err != nil {
			t.Fatal(err)
		}
		got := []core.Digest{inspection.GenerationDigest, inspection.ManifestDigest, inspection.GraphDigest, inspection.HandoffDigest}[stageIndex]
		if got != stages[stageIndex].want {
			t.Fatalf("stage %d recovered digest=%q want=%q", stageIndex, got, stages[stageIndex].want)
		}
		if _, err := store.InspectHistoricalPublicationV2(ctx, publicationRefV2(bundle)); !core.HasCategory(err, core.ErrorNotFound) {
			t.Fatalf("stage %d became historical: %v", stageIndex, err)
		}
		if err := store.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSQLitePublicationV2StrictDecodeRejectsStagedAndCommittedCorruption(t *testing.T) {
	ctx := context.Background()
	path := t.TempDir() + "/publication.db"
	bundle := sqliteTestBundleV2(t)
	current := sqliteTestCurrentV2(t, bundle, 1, "sqlite-corrupt")
	store := openSQLitePublicationTestStoreV2(t, path)
	defer store.Close()
	stageSQLiteBundleV2(t, store, bundle)
	if _, err := store.db.Exec(`UPDATE harness_publication_staged_v2 SET manifest_json=json_set(manifest_json,'$.unknown_field',1) WHERE publication_id=?`, bundle.Publication.PublicationID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitPublicationCurrentV2(ctx, CommitPublicationCurrentRequestV2{Bundle: bundle, Current: current}); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("corrupt staged commit = %v", err)
	}
	if _, err := store.db.Exec(`UPDATE harness_publication_staged_v2 SET manifest_json=? WHERE publication_id=?`, mustPublicationJSONV2(t, bundle.Manifest), bundle.Publication.PublicationID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitPublicationCurrentV2(ctx, CommitPublicationCurrentRequestV2{Bundle: bundle, Current: current}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`UPDATE harness_publication_committed_v2 SET bundle_json=json_set(bundle_json,'$.unknown_field',1) WHERE publication_id=?`, bundle.Publication.PublicationID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.InspectHistoricalPublicationV2(ctx, publicationRefV2(bundle)); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("corrupt committed Inspect = %v", err)
	}
}

func TestSQLitePublicationV2SixtyFourIndependentStoresHaveOneCASWinner(t *testing.T) {
	ctx := context.Background()
	path := t.TempDir() + "/publication.db"
	bundle := sqliteTestBundleV2(t)
	seed := openSQLitePublicationTestStoreV2(t, path)
	stageSQLiteBundleV2(t, seed, bundle)
	if err := seed.Close(); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	var winners, unexpected atomic.Int64
	var wait sync.WaitGroup
	for index := 0; index < 64; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			store, err := OpenSQLiteStoreV2(ctx, SQLiteStoreConfigV2{Path: path, Clock: func() time.Time { return assemblytestkit.Now }})
			if err != nil {
				unexpected.Add(1)
				return
			}
			defer store.Close()
			<-start
			current := sqliteTestCurrentV2(t, bundle, 1, "sqlite-race-"+twoDigits(index))
			_, err = store.CommitPublicationCurrentV2(ctx, CommitPublicationCurrentRequestV2{Bundle: bundle, Current: current})
			if err == nil {
				winners.Add(1)
			} else if !core.HasCategory(err, core.ErrorConflict) {
				unexpected.Add(1)
			}
		}(index)
	}
	close(start)
	wait.Wait()
	if winners.Load() != 1 || unexpected.Load() != 0 {
		t.Fatalf("winners=%d unexpected=%d", winners.Load(), unexpected.Load())
	}
}

func TestSQLitePublicationV2RejectsABAAndTypedNil(t *testing.T) {
	now := assemblytestkit.Now
	path := t.TempDir() + "/publication.db"
	store := openSQLitePublicationTestStoreV2(t, path)
	publisher := newTestPublisher(t, store, func() time.Time { return now })
	firstRequest := initialRequest(now, "sqlite-a")
	first, err := publisher.CompileAndPublishAssemblyV2(context.Background(), firstRequest)
	if err != nil {
		t.Fatal(err)
	}
	secondRequest := assemblycontract.CompileAndPublishAssemblyRequestV2{ContractVersion: assemblycontract.PublicationContractVersionV2, AttemptID: "sqlite-b", Input: nextInput(t, firstRequest.Input, first.Current.Artifacts.Generation), ExpectedCurrent: assemblycontract.AssemblyPublicationCurrentExpectationV2{Exists: true, Revision: first.Current.Revision, Digest: first.Current.Digest}, RequestedExpiresUnixNano: now.Add(2 * time.Minute).UnixNano()}
	second, err := publisher.CompileAndPublishAssemblyV2(context.Background(), secondRequest)
	if err != nil {
		t.Fatal(err)
	}
	aba := firstRequest
	aba.AttemptID = "sqlite-a-again"
	aba.ExpectedCurrent = assemblycontract.AssemblyPublicationCurrentExpectationV2{Exists: true, Revision: second.Current.Revision, Digest: second.Current.Digest}
	if _, err := publisher.CompileAndPublishAssemblyV2(context.Background(), aba); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("ABA = %v", err)
	}
	var nilStore *SQLiteStoreV2
	if _, err := nilStore.InspectCurrentPublicationV2(context.Background(), "scope"); err == nil {
		t.Fatal("typed-nil store did not fail closed")
	}
	if err := store.StageManifestV2(nil, "id", assemblycontract.AssemblyManifestV1{}); err == nil {
		t.Fatal("nil context did not fail closed")
	}
}

func TestSQLitePublicationV2SchemaDigestDriftFailsClosedOnReopen(t *testing.T) {
	path := t.TempDir() + "/publication.db"
	store := openSQLitePublicationTestStoreV2(t, path)
	if _, err := store.db.Exec(`UPDATE harness_publication_schema_v2 SET digest='sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa' WHERE version=?`, sqlitePublicationSchemaVersionV2); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if reopened, err := OpenSQLiteStoreV2(context.Background(), SQLiteStoreConfigV2{Path: path, Clock: func() time.Time { return assemblytestkit.Now }}); !core.HasCategory(err, core.ErrorConflict) {
		if reopened != nil {
			_ = reopened.Close()
		}
		t.Fatalf("schema digest drift reopen = %v", err)
	}
}

func sqliteTestBundleV2(t *testing.T) assemblycontract.AssemblyPublicationBundleV2 {
	t.Helper()
	input := assemblytestkit.ValidInput()
	compiled, err := assemblycompiler.New().Compile(input)
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := assemblycontract.NewAssemblyPublicationBundleV2(input.ScopeRef, compiled)
	if err != nil {
		t.Fatal(err)
	}
	return bundle
}

func sqliteTestCurrentV2(t *testing.T, bundle assemblycontract.AssemblyPublicationBundleV2, revision core.Revision, attempt string) assemblycontract.AssemblyPublicationCurrentV2 {
	t.Helper()
	value, err := assemblycontract.NewAssemblyPublicationCurrentV2(bundle, attempt, revision, assemblytestkit.Now, assemblytestkit.Now.Add(time.Minute).UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func publicationRefV2(bundle assemblycontract.AssemblyPublicationBundleV2) assemblycontract.AssemblyPublicationRefV2 {
	return assemblycontract.AssemblyPublicationRefV2{PublicationID: bundle.Publication.PublicationID, Revision: bundle.Publication.Revision, Digest: bundle.Publication.Digest}
}

func openSQLitePublicationTestStoreV2(t *testing.T, path string) *SQLiteStoreV2 {
	t.Helper()
	store, err := OpenSQLiteStoreV2(context.Background(), SQLiteStoreConfigV2{Path: path, Clock: func() time.Time { return assemblytestkit.Now }})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func stageSQLiteBundleV2(t *testing.T, store *SQLiteStoreV2, bundle assemblycontract.AssemblyPublicationBundleV2) {
	t.Helper()
	ctx := context.Background()
	id := bundle.Publication.PublicationID
	for _, stage := range []func() error{func() error { return store.StageGenerationV2(ctx, id, bundle.Generation) }, func() error { return store.StageManifestV2(ctx, id, bundle.Manifest) }, func() error { return store.StageGraphV2(ctx, id, bundle.Graph) }, func() error { return store.StageHandoffV2(ctx, id, bundle.Handoff) }} {
		if err := stage(); err != nil {
			t.Fatal(err)
		}
	}
}

func mustPublicationJSONV2(t *testing.T, value any) []byte {
	t.Helper()
	payload, _, err := encodePublicationRowV2("StagedManifestV2", value)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}
