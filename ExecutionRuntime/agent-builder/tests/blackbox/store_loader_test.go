package blackbox_test

import (
	"context"
	"database/sql"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-builder/compiler"
	packagecontract "github.com/Proview-China/rax/ExecutionRuntime/agent-builder/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-builder/loader"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-builder/repository"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycompiler"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func packageAndPublication(t *testing.T) (packagecontract.AgentPackageV1, assemblycontract.AssemblyPublicationBundleV2) {
	t.Helper()
	result := resolved(t)
	pkg, err := compiler.NewV1().Compile(result)
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := assemblycompiler.New().Compile(result.AssemblyInput)
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := assemblycontract.NewAssemblyPublicationBundleV2(result.AssemblyInput.ScopeRef, compiled)
	if err != nil {
		t.Fatal(err)
	}
	return pkg, bundle
}

type packageReaderV1 struct {
	value packagecontract.AgentPackageV1
	err   error
	seen  []packagecontract.AgentPackageRefV1
}

func (r *packageReaderV1) InspectExactAgentPackageV1(_ context.Context, ref packagecontract.AgentPackageRefV1) (packagecontract.AgentPackageV1, error) {
	r.seen = append(r.seen, ref)
	return r.value, r.err
}

type publicationReaderV2 struct {
	bundle assemblycontract.AssemblyPublicationBundleV2
	err    error
	seen   []assemblycontract.AssemblyPublicationRefV2
}

func (r *publicationReaderV2) InspectAssemblyPublicationHistoricalV2(_ context.Context, ref assemblycontract.AssemblyPublicationRefV2) (assemblycontract.AssemblyPublicationBundleV2, error) {
	r.seen = append(r.seen, ref)
	return r.bundle, r.err
}

func TestSQLitePackageStoreRestartRaceConflictAndLostReply(t *testing.T) {
	ctx := context.Background()
	pkg, _ := packageAndPublication(t)
	path := t.TempDir() + "/packages.db"
	store, err := repository.OpenSQLiteRepositoryV1(ctx, repository.SQLiteConfigV1{Path: path})
	if err != nil {
		t.Fatal(err)
	}

	const workers = 32
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, ensureErr := store.EnsureExactAgentPackageV1(ctx, pkg)
			if ensureErr == nil && !reflect.DeepEqual(got, pkg) {
				ensureErr = core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "concurrent ensure returned different package")
			}
			errs <- ensureErr
		}()
	}
	wg.Wait()
	close(errs)
	for ensureErr := range errs {
		if ensureErr != nil {
			t.Fatal(ensureErr)
		}
	}

	changed := pkg
	changed.Digest = core.DigestBytes([]byte("different-package-body"))
	if _, err = store.EnsureExactAgentPackageV1(ctx, changed); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("same coordinate different body must conflict: %v", err)
	}
	competing, err := repository.OpenSQLiteRepositoryV1(ctx, repository.SQLiteConfigV1{Path: t.TempDir() + "/competing-create.db"})
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, candidate := range []packagecontract.AgentPackageV1{pkg, changed} {
		wg.Add(1)
		go func(value packagecontract.AgentPackageV1) {
			defer wg.Done()
			<-start
			_, candidateErr := competing.EnsureExactAgentPackageV1(ctx, value)
			results <- candidateErr
		}(candidate)
	}
	close(start)
	wg.Wait()
	close(results)
	var successes int
	for candidateErr := range results {
		if candidateErr == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("competing create must produce one valid durable winner, got %d", successes)
	}
	competingValue, inspectErr := competing.InspectExactAgentPackageV1(ctx, pkg.RefV1())
	if inspectErr != nil || !reflect.DeepEqual(competingValue, pkg) {
		t.Fatalf("competing create did not preserve valid package: %v", inspectErr)
	}
	if err = competing.Close(); err != nil {
		t.Fatal(err)
	}
	if err = store.IntegrityCheckV1(ctx); err != nil {
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}

	store, err = repository.OpenSQLiteRepositoryV1(ctx, repository.SQLiteConfigV1{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	got, err := store.InspectExactAgentPackageV1(ctx, pkg.RefV1())
	if err != nil || !reflect.DeepEqual(got, pkg) {
		t.Fatalf("restart lost exact package: %v", err)
	}

	secondPath := t.TempDir() + "/lost-reply.db"
	lostStore, err := repository.OpenSQLiteRepositoryV1(ctx, repository.SQLiteConfigV1{Path: secondPath})
	if err != nil {
		t.Fatal(err)
	}
	defer lostStore.Close()
	lostStore.LoseNextEnsureReplyV1()
	recovered, err := repository.EnsureExactWithRecoveryV1(ctx, lostStore, lostStore, pkg)
	if err != nil || !reflect.DeepEqual(recovered, pkg) {
		t.Fatalf("exact Inspect did not recover committed lost reply: %v", err)
	}
}

func TestSQLitePackageInspectFailsClosedOnDirectRowCorruption(t *testing.T) {
	ctx := context.Background()
	pkg, _ := packageAndPublication(t)
	tests := map[string]struct {
		query string
		value any
	}{
		"payload json": {query: "UPDATE agent_package_v1 SET payload_json=?", value: []byte("{}")},
		"row digest":   {query: "UPDATE agent_package_v1 SET row_digest=?", value: string(core.DigestBytes([]byte("corrupt-row")))},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			path := t.TempDir() + "/corrupt.db"
			store, err := repository.OpenSQLiteRepositoryV1(ctx, repository.SQLiteConfigV1{Path: path})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = store.EnsureExactAgentPackageV1(ctx, pkg); err != nil {
				t.Fatal(err)
			}
			if err = store.Close(); err != nil {
				t.Fatal(err)
			}

			db, err := sql.Open("sqlite", "file:"+path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = db.ExecContext(ctx, test.query, test.value); err != nil {
				_ = db.Close()
				t.Fatal(err)
			}
			if err = db.Close(); err != nil {
				t.Fatal(err)
			}

			store, err = repository.OpenSQLiteRepositoryV1(ctx, repository.SQLiteConfigV1{Path: path})
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			if _, err = store.InspectExactAgentPackageV1(ctx, pkg.RefV1()); err == nil {
				t.Fatal("direct SQLite row corruption passed exact Inspect")
			}
		})
	}
}

type unknownRepositoryV1 struct{}

func (unknownRepositoryV1) EnsureExactAgentPackageV1(context.Context, packagecontract.AgentPackageV1) (packagecontract.AgentPackageV1, error) {
	return packagecontract.AgentPackageV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "lost")
}
func (unknownRepositoryV1) InspectExactAgentPackageV1(context.Context, packagecontract.AgentPackageRefV1) (packagecontract.AgentPackageV1, error) {
	return packagecontract.AgentPackageV1{}, core.NewError(core.ErrorNotFound, core.ReasonEvidenceUnavailable, "not found")
}

type countingNotFoundReaderV1 struct {
	calls atomic.Int32
	ref   packagecontract.AgentPackageRefV1
}

func (r *countingNotFoundReaderV1) InspectExactAgentPackageV1(_ context.Context, ref packagecontract.AgentPackageRefV1) (packagecontract.AgentPackageV1, error) {
	r.calls.Add(1)
	r.ref = ref
	return packagecontract.AgentPackageV1{}, core.NewError(core.ErrorNotFound, core.ReasonEvidenceUnavailable, "not found")
}

func TestLostReplyWithoutOriginalEvidenceNeverSucceeds(t *testing.T) {
	pkg, _ := packageAndPublication(t)
	reader := &countingNotFoundReaderV1{}
	_, err := repository.EnsureExactWithRecoveryV1(context.Background(), unknownRepositoryV1{}, reader, pkg)
	if !core.HasCategory(err, core.ErrorIndeterminate) || reader.calls.Load() != 1 || reader.ref != pkg.RefV1() {
		t.Fatalf("recovery must inspect only original ref and remain indeterminate: %v %#v", err, reader.ref)
	}
}

func TestLoaderRereadsAndValidatesExactHistoricalPublication(t *testing.T) {
	pkg, bundle := packageAndPublication(t)
	packageReader := &packageReaderV1{value: pkg}
	publicationReader := &publicationReaderV2{bundle: bundle}
	exactLoader, err := loader.NewV1(packageReader, publicationReader)
	if err != nil {
		t.Fatal(err)
	}
	closure, err := exactLoader.LoadExactV1(context.Background(), pkg.RefV1())
	if err != nil {
		t.Fatal(err)
	}
	if len(packageReader.seen) != 1 || packageReader.seen[0] != pkg.RefV1() || len(publicationReader.seen) != 1 || publicationReader.seen[0] != pkg.Lock.PublicationRef || closure.Publication.Publication.Artifacts.Generation != pkg.Lock.GenerationRef {
		t.Fatal("loader did not read the exact Package and locked historical Publication")
	}
	closure.Publication.Generation.GenerationID = "caller-mutated"
	again, err := exactLoader.LoadExactV1(context.Background(), pkg.RefV1())
	if err != nil || again.Publication.Generation.GenerationID == "caller-mutated" {
		t.Fatal("loader returned an aliased closure")
	}

	tests := map[string]func(*packagecontract.AgentPackageV1, *assemblycontract.AssemblyPublicationBundleV2){
		"publication ref splice": func(_ *packagecontract.AgentPackageV1, value *assemblycontract.AssemblyPublicationBundleV2) {
			value.Publication.Digest = core.DigestBytes([]byte("other-publication"))
		},
		"manifest revision splice": func(_ *packagecontract.AgentPackageV1, value *assemblycontract.AssemblyPublicationBundleV2) {
			value.Publication.Artifacts.Manifest.Revision++
		},
		"input closure splice": func(_ *packagecontract.AgentPackageV1, value *assemblycontract.AssemblyPublicationBundleV2) {
			value.Generation.InputDigest = core.DigestBytes([]byte("other-input"))
		},
		"rejected generation": func(_ *packagecontract.AgentPackageV1, value *assemblycontract.AssemblyPublicationBundleV2) {
			value.Generation.State = assemblycontract.AssemblyStateRejectedV1
		},
		"package lock splice": func(value *packagecontract.AgentPackageV1, _ *assemblycontract.AssemblyPublicationBundleV2) {
			value.Lock.PublicationRef.Digest = core.DigestBytes([]byte("other-lock-publication"))
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			changedPackage, changedBundle := pkg, bundle
			mutate(&changedPackage, &changedBundle)
			candidate, newErr := loader.NewV1(&packageReaderV1{value: changedPackage}, &publicationReaderV2{bundle: changedBundle})
			if newErr != nil {
				t.Fatal(newErr)
			}
			if _, loadErr := candidate.LoadExactV1(context.Background(), pkg.RefV1()); loadErr == nil {
				t.Fatal("spliced or drifted historical Publication closure accepted")
			}
		})
	}
}

func TestLoaderRejectsLockWithoutCommittedPublicationAndTypedNilReaders(t *testing.T) {
	pkg, bundle := packageAndPublication(t)
	notFound := core.NewError(core.ErrorNotFound, core.ReasonEvidenceUnavailable, "publication absent")
	publicationReader := &publicationReaderV2{bundle: bundle, err: notFound}
	exactLoader, err := loader.NewV1(&packageReaderV1{value: pkg}, publicationReader)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = exactLoader.LoadExactV1(context.Background(), pkg.RefV1()); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("lock alone must not grant an uncommitted Publication closure: %v", err)
	}
	var nilPackageReader *packageReaderV1
	if _, err = loader.NewV1(nilPackageReader, publicationReader); err == nil {
		t.Fatal("typed-nil package reader accepted")
	}
	var nilPublicationReader *publicationReaderV2
	if _, err = loader.NewV1(&packageReaderV1{value: pkg}, nilPublicationReader); err == nil {
		t.Fatal("typed-nil historical Publication reader accepted")
	}
}
