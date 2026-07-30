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
	"github.com/Proview-China/rax/ExecutionRuntime/agent-builder/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-builder/repository"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycompiler"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func packageAndArtifacts(t *testing.T) (packagecontract.AgentPackageV1, artifactReaderV1) {
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
	return pkg, artifactReaderV1{
		generation: ports.AssemblyGenerationArtifactV1{Ref: pkg.Lock.GenerationRef, Value: *compiled.Generation},
		manifest:   ports.AssemblyManifestArtifactV1{Ref: pkg.Lock.ManifestRef, Value: *compiled.Manifest},
		graph:      ports.CompiledHarnessGraphArtifactV1{Ref: pkg.Lock.GraphRef, Value: *compiled.Graph},
		handoff:    ports.AssemblyHandoffArtifactV1{Ref: pkg.Lock.HandoffRef, Value: *compiled.Handoff},
	}
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

type artifactReaderV1 struct {
	generation ports.AssemblyGenerationArtifactV1
	manifest   ports.AssemblyManifestArtifactV1
	graph      ports.CompiledHarnessGraphArtifactV1
	handoff    ports.AssemblyHandoffArtifactV1
	err        error
}

func (r artifactReaderV1) InspectExactGenerationV1(context.Context, assemblycontract.ObjectRefV1) (ports.AssemblyGenerationArtifactV1, error) {
	return r.generation, r.err
}
func (r artifactReaderV1) InspectExactManifestV1(context.Context, assemblycontract.ObjectRefV1) (ports.AssemblyManifestArtifactV1, error) {
	return r.manifest, r.err
}
func (r artifactReaderV1) InspectExactGraphV1(context.Context, assemblycontract.ObjectRefV1) (ports.CompiledHarnessGraphArtifactV1, error) {
	return r.graph, r.err
}
func (r artifactReaderV1) InspectExactHandoffV1(context.Context, assemblycontract.ObjectRefV1) (ports.AssemblyHandoffArtifactV1, error) {
	return r.handoff, r.err
}

func TestSQLitePackageStoreRestartRaceConflictAndLostReply(t *testing.T) {
	ctx := context.Background()
	pkg, _ := packageAndArtifacts(t)
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
	pkg, _ := packageAndArtifacts(t)
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
	pkg, _ := packageAndArtifacts(t)
	reader := &countingNotFoundReaderV1{}
	_, err := repository.EnsureExactWithRecoveryV1(context.Background(), unknownRepositoryV1{}, reader, pkg)
	if !core.HasCategory(err, core.ErrorIndeterminate) || reader.calls.Load() != 1 || reader.ref != pkg.RefV1() {
		t.Fatalf("recovery must inspect only original ref and remain indeterminate: %v %#v", err, reader.ref)
	}
}

func TestLoaderRereadsAndValidatesExactArtifactClosure(t *testing.T) {
	pkg, artifacts := packageAndArtifacts(t)
	packageReader := &packageReaderV1{value: pkg}
	exactLoader, err := loader.NewV1(packageReader, artifacts)
	if err != nil {
		t.Fatal(err)
	}
	closure, err := exactLoader.LoadExactV1(context.Background(), pkg.RefV1())
	if err != nil {
		t.Fatal(err)
	}
	if len(packageReader.seen) != 1 || packageReader.seen[0] != pkg.RefV1() || closure.Generation.Ref != pkg.Lock.GenerationRef || closure.Manifest.Ref != pkg.Lock.ManifestRef || closure.Graph.Ref != pkg.Lock.GraphRef || closure.Handoff.Ref != pkg.Lock.HandoffRef {
		t.Fatal("loader did not read the exact Package and four locked artifact refs")
	}
	closure.Generation.Value.GenerationID = "caller-mutated"
	again, err := exactLoader.LoadExactV1(context.Background(), pkg.RefV1())
	if err != nil || again.Generation.Value.GenerationID == "caller-mutated" {
		t.Fatal("loader returned an aliased closure")
	}

	tests := map[string]func(*packagecontract.AgentPackageV1, *artifactReaderV1){
		"generation id splice":     func(_ *packagecontract.AgentPackageV1, a *artifactReaderV1) { a.generation.Ref.ID += "-other" },
		"manifest revision splice": func(_ *packagecontract.AgentPackageV1, a *artifactReaderV1) { a.manifest.Ref.Revision++ },
		"graph digest splice": func(_ *packagecontract.AgentPackageV1, a *artifactReaderV1) {
			a.graph.Ref.Digest = core.DigestBytes([]byte("other"))
		},
		"handoff body drift": func(_ *packagecontract.AgentPackageV1, a *artifactReaderV1) {
			a.handoff.Value.GraphDigest = core.DigestBytes([]byte("other"))
		},
		"package lock drift": func(p *packagecontract.AgentPackageV1, _ *artifactReaderV1) { p.Lock.GenerationRef.ID += "-other" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			changedPackage, changedArtifacts := pkg, artifacts
			mutate(&changedPackage, &changedArtifacts)
			candidate, newErr := loader.NewV1(&packageReaderV1{value: changedPackage}, changedArtifacts)
			if newErr != nil {
				t.Fatal(newErr)
			}
			if _, loadErr := candidate.LoadExactV1(context.Background(), pkg.RefV1()); loadErr == nil {
				t.Fatal("spliced or drifted closure accepted")
			}
		})
	}
}

func TestLoaderRejectsLockWithoutArtifactsAndTypedNilReaders(t *testing.T) {
	pkg, artifacts := packageAndArtifacts(t)
	notFound := core.NewError(core.ErrorNotFound, core.ReasonEvidenceUnavailable, "artifact absent")
	artifacts.err = notFound
	exactLoader, err := loader.NewV1(&packageReaderV1{value: pkg}, artifacts)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = exactLoader.LoadExactV1(context.Background(), pkg.RefV1()); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("lock alone must not grant artifact closure: %v", err)
	}
	var nilPackageReader *packageReaderV1
	if _, err = loader.NewV1(nilPackageReader, artifacts); err == nil {
		t.Fatal("typed-nil package reader accepted")
	}
}

func TestLoaderRejectsSelfConsistentRejectedGeneration(t *testing.T) {
	pkg, artifacts := packageAndArtifacts(t)
	artifacts.generation.Value.State = assemblycontract.AssemblyStateRejectedV1
	var err error
	artifacts.generation.Value.Digest, err = assemblycontract.GenerationDigestV1(artifacts.generation.Value)
	if err != nil {
		t.Fatal(err)
	}
	artifacts.generation.Ref.Digest = artifacts.generation.Value.Digest
	artifacts.handoff.Value.GenerationRef = artifacts.generation.Ref
	artifacts.handoff.Value.Digest, err = assemblycontract.HandoffDigestV1(artifacts.handoff.Value)
	if err != nil {
		t.Fatal(err)
	}
	artifacts.handoff.Ref.Digest = artifacts.handoff.Value.Digest
	pkg.Lock.GenerationRef = artifacts.generation.Ref
	pkg.Lock.HandoffRef = artifacts.handoff.Ref
	pkg.Lock, err = packagecontract.SealLockManifestV1(pkg.Lock)
	if err != nil {
		t.Fatal(err)
	}
	pkg, err = packagecontract.SealPackageV1(pkg)
	if err != nil {
		t.Fatal(err)
	}
	exactLoader, err := loader.NewV1(&packageReaderV1{value: pkg}, artifacts)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = exactLoader.LoadExactV1(context.Background(), pkg.RefV1()); err == nil {
		t.Fatal("self-consistent rejected generation became a verified closure")
	}
}
