package blackbox_test

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-builder/compiler"
	packagecontract "github.com/Proview-China/rax/ExecutionRuntime/agent-builder/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-builder/loader"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-builder/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-builder/repository"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-builder/selection"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycompiler"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblypublication"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type selectionFixtureV1 struct {
	now              time.Time
	pkg              packagecontract.AgentPackageV1
	packageStore     *repository.SQLiteRepositoryV1
	publicationStore *assemblypublication.SQLiteStoreV2
	publisher        *assemblypublication.PublisherV2
	selectionStore   *repository.SelectionSQLiteV1
	selectionPath    string
	loader           *loader.LoaderV1
	service          *selection.ServiceV1
}

func newSelectionFixtureV1(t *testing.T) *selectionFixtureV1 {
	t.Helper()
	ctx := context.Background()
	now := time.Unix(1_800_100_000, 0).UTC()
	resolvedResult := resolved(t)
	pkg, err := compiler.NewV1().Compile(resolvedResult)
	if err != nil {
		t.Fatal(err)
	}
	packageStore, err := repository.OpenSQLiteRepositoryV1(ctx, repository.SQLiteConfigV1{
		Path:  t.TempDir() + "/packages.db",
		Clock: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = packageStore.EnsureExactAgentPackageV1(ctx, pkg); err != nil {
		_ = packageStore.Close()
		t.Fatal(err)
	}
	publicationStore, err := assemblypublication.OpenSQLiteStoreV2(ctx, assemblypublication.SQLiteStoreConfigV2{
		Path:  t.TempDir() + "/publications.db",
		Clock: func() time.Time { return now },
	})
	if err != nil {
		_ = packageStore.Close()
		t.Fatal(err)
	}
	publisher, err := assemblypublication.NewPublisherV2(assemblycompiler.New(), publicationStore, func() time.Time { return now })
	if err != nil {
		_ = publicationStore.Close()
		_ = packageStore.Close()
		t.Fatal(err)
	}
	published, err := publisher.CompileAndPublishAssemblyV2(ctx, assemblycontract.CompileAndPublishAssemblyRequestV2{
		ContractVersion:          assemblycontract.PublicationContractVersionV2,
		AttemptID:                "agent-package-selection-publication",
		Input:                    resolvedResult.AssemblyInput,
		ExpectedCurrent:          assemblycontract.AssemblyPublicationCurrentExpectationV2{},
		RequestedExpiresUnixNano: now.Add(2 * time.Hour).UnixNano(),
	})
	if err != nil {
		_ = publicationStore.Close()
		_ = packageStore.Close()
		t.Fatal(err)
	}
	publishedRef := assemblycontract.AssemblyPublicationRefV2{
		PublicationID: published.Publication.PublicationID,
		Revision:      published.Publication.Revision,
		Digest:        published.Publication.Digest,
	}
	if publishedRef != pkg.Lock.PublicationRef {
		t.Fatal("real Harness Publication does not match Package lock")
	}
	selectionPath := t.TempDir() + "/selection.db"
	selectionStore, err := repository.OpenSelectionSQLiteV1(ctx, repository.SelectionSQLiteConfigV1{
		Path:  selectionPath,
		Clock: func() time.Time { return now },
	})
	if err != nil {
		_ = publicationStore.Close()
		_ = packageStore.Close()
		t.Fatal(err)
	}
	exactLoader, err := loader.NewV1(packageStore, publisher)
	if err != nil {
		_ = selectionStore.Close()
		_ = publicationStore.Close()
		_ = packageStore.Close()
		t.Fatal(err)
	}
	service, err := selection.NewServiceV1(selection.ConfigV1{
		Closures:   exactLoader,
		Selections: selectionStore,
		Clock:      func() time.Time { return now },
	})
	if err != nil {
		_ = selectionStore.Close()
		_ = publicationStore.Close()
		_ = packageStore.Close()
		t.Fatal(err)
	}
	fixture := &selectionFixtureV1{
		now:              now,
		pkg:              pkg,
		packageStore:     packageStore,
		publicationStore: publicationStore,
		publisher:        publisher,
		selectionStore:   selectionStore,
		selectionPath:    selectionPath,
		loader:           exactLoader,
		service:          service,
	}
	t.Cleanup(func() {
		_ = fixture.selectionStore.Close()
		_ = fixture.publicationStore.Close()
		_ = fixture.packageStore.Close()
	})
	return fixture
}

func selectionRequestV1(t *testing.T, fixture *selectionFixtureV1, selectionID string, expected packagecontract.AgentPackageSelectionCurrentRefV1) packagecontract.AgentPackageSelectionRequestV1 {
	t.Helper()
	request, err := packagecontract.SealAgentPackageSelectionRequestV1(packagecontract.AgentPackageSelectionRequestV1{
		SelectionID:               selectionID,
		PackageRef:                fixture.pkg.RefV1(),
		ExpectedCurrent:           expected,
		RequestedNotAfterUnixNano: fixture.now.Add(time.Hour).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func TestPackageSelectionUsesRealExactStoresAndDerivesNominalCurrentV1(t *testing.T) {
	fixture := newSelectionFixtureV1(t)
	ctx := context.Background()
	request := selectionRequestV1(t, fixture, "agent/example/current-package", packagecontract.AgentPackageSelectionCurrentRefV1{})
	current, err := fixture.service.SelectPackageV1(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	closure, err := fixture.loader.LoadExactV1(ctx, fixture.pkg.RefV1())
	if err != nil {
		t.Fatal(err)
	}
	if current.Ref.Revision != 1 ||
		current.PackageRef != fixture.pkg.RefV1() ||
		current.PublicationRef != fixture.pkg.Lock.PublicationRef ||
		current.ClosureDigest != closure.ClosureDigest ||
		current.Ref.Digest != current.ProjectionDigest ||
		current.Ref.ExpiresUnixNano != current.ExpiresUnixNano {
		t.Fatalf("selection was not derived from the freshly verified closure: %+v", current)
	}
	if _, ok := any(current).(interface{ ProductionEligible() bool }); ok {
		t.Fatal("nominal package selection exposed a production eligibility grant")
	}
	inspected, err := fixture.service.InspectCurrentV1(ctx, request.SelectionID)
	if err != nil || !reflect.DeepEqual(inspected, current) {
		t.Fatalf("current Inspect drifted: %v", err)
	}
}

type countingSelectionStoreV1 struct {
	inner    ports.AgentPackageSelectionCurrentRepositoryV1
	appends  atomic.Int32
	inspects atomic.Int32
}

func (store *countingSelectionStoreV1) CompareAndSwapAgentPackageSelectionCurrentV1(
	ctx context.Context,
	expected packagecontract.AgentPackageSelectionCurrentRefV1,
	current packagecontract.AgentPackageSelectionCurrentV1,
) (packagecontract.AgentPackageSelectionCurrentV1, error) {
	store.appends.Add(1)
	return store.inner.CompareAndSwapAgentPackageSelectionCurrentV1(ctx, expected, current)
}
func (store *countingSelectionStoreV1) InspectAgentPackageSelectionExactV1(
	ctx context.Context,
	ref packagecontract.AgentPackageSelectionCurrentRefV1,
) (packagecontract.AgentPackageSelectionCurrentV1, error) {
	store.inspects.Add(1)
	return store.inner.InspectAgentPackageSelectionExactV1(ctx, ref)
}
func (store *countingSelectionStoreV1) InspectAgentPackageSelectionCurrentV1(ctx context.Context, id string) (packagecontract.AgentPackageSelectionCurrentV1, error) {
	return store.inner.InspectAgentPackageSelectionCurrentV1(ctx, id)
}

func TestPackageSelectionServiceRejectsTypedNilDependenciesV1(t *testing.T) {
	fixture := newSelectionFixtureV1(t)
	tests := []struct {
		name   string
		config selection.ConfigV1
	}{
		{
			name: "closure reader",
			config: selection.ConfigV1{
				Closures:   (*mutatedClosureReaderV1)(nil),
				Selections: fixture.selectionStore,
			},
		},
		{
			name: "selection repository",
			config: selection.ConfigV1{
				Closures:   fixture.loader,
				Selections: (*countingSelectionStoreV1)(nil),
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service, err := selection.NewServiceV1(test.config)
			if err == nil || service != nil || !core.HasCategory(err, core.ErrorInvalidArgument) {
				t.Fatalf("typed-nil %s dependency was accepted: service=%v err=%v", test.name, service, err)
			}
		})
	}
}

func TestPackageSelectionClosureFailureWritesZeroV1(t *testing.T) {
	fixture := newSelectionFixtureV1(t)
	request := selectionRequestV1(t, fixture, "agent/example/rejected-package", packagecontract.AgentPackageSelectionCurrentRefV1{})
	tests := map[string]ports.VerifiedAgentPackageClosureReaderV1{
		"spliced publication": mustLoaderV1(t, fixture.packageStore, splicingHistoricalReaderV2{delegate: fixture.publisher}),
		"missing publication": mustLoaderV1(t, fixture.packageStore, &publicationReaderV2{
			err: core.NewError(core.ErrorNotFound, core.ReasonEvidenceUnavailable, "missing"),
		}),
		"missing package": mustLoaderV1(t, &packageReaderV1{
			err: core.NewError(core.ErrorNotFound, core.ReasonEvidenceUnavailable, "missing"),
		}, fixture.publisher),
	}
	for name, closureLoader := range tests {
		t.Run(name, func(t *testing.T) {
			counting := &countingSelectionStoreV1{inner: fixture.selectionStore}
			service, err := selection.NewServiceV1(selection.ConfigV1{
				Closures:   closureLoader,
				Selections: counting,
				Clock:      func() time.Time { return fixture.now },
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = service.SelectPackageV1(context.Background(), request); err == nil {
				t.Fatal("missing or spliced verified closure was selected")
			}
			if counting.appends.Load() != 0 || counting.inspects.Load() != 0 {
				t.Fatalf("closure failure reached selection write/Inspect: append=%d inspect=%d", counting.appends.Load(), counting.inspects.Load())
			}
		})
	}
}

type mutatedClosureReaderV1 struct {
	value packagecontract.VerifiedAgentPackageClosureV1
	loads atomic.Int32
}

func (reader *mutatedClosureReaderV1) LoadVerifiedAgentPackageClosureV1(
	context.Context,
	packagecontract.AgentPackageRefV1,
) (packagecontract.VerifiedAgentPackageClosureV1, error) {
	reader.loads.Add(1)
	return packagecontract.CloneVerifiedAgentPackageClosureV1(reader.value), nil
}

func TestPackageSelectionVerifiedClosureAxisSpliceWritesZeroV1(t *testing.T) {
	fixture := newSelectionFixtureV1(t)
	base, err := fixture.loader.LoadVerifiedAgentPackageClosureV1(context.Background(), fixture.pkg.RefV1())
	if err != nil {
		t.Fatal(err)
	}
	request := selectionRequestV1(t, fixture, "agent/example/axis-splice", packagecontract.AgentPackageSelectionCurrentRefV1{})
	tests := map[string]func(*packagecontract.VerifiedAgentPackageClosureV1){
		"Package": func(value *packagecontract.VerifiedAgentPackageClosureV1) {
			value.Package.Digest = core.DigestBytes([]byte("spliced-package"))
		},
		"PublicationRef": func(value *packagecontract.VerifiedAgentPackageClosureV1) {
			value.Publication.Publication.Digest = core.DigestBytes([]byte("spliced-publication"))
		},
		"GenerationRef": func(value *packagecontract.VerifiedAgentPackageClosureV1) {
			value.Publication.Publication.Artifacts.Generation.Digest = core.DigestBytes([]byte("spliced-generation"))
		},
		"ManifestRef": func(value *packagecontract.VerifiedAgentPackageClosureV1) {
			value.Publication.Publication.Artifacts.Manifest.Digest = core.DigestBytes([]byte("spliced-manifest"))
		},
		"GraphRef": func(value *packagecontract.VerifiedAgentPackageClosureV1) {
			value.Publication.Publication.Artifacts.Graph.Digest = core.DigestBytes([]byte("spliced-graph"))
		},
		"HandoffRef": func(value *packagecontract.VerifiedAgentPackageClosureV1) {
			value.Publication.Publication.Artifacts.Handoff.Digest = core.DigestBytes([]byte("spliced-handoff"))
		},
		"AssemblyInputDigest": func(value *packagecontract.VerifiedAgentPackageClosureV1) {
			value.Publication.Generation.InputDigest = core.DigestBytes([]byte("spliced-input"))
		},
		"HarnessCompilerVersion": func(value *packagecontract.VerifiedAgentPackageClosureV1) {
			value.Publication.Generation.CompilerVersion = "spliced-compiler"
		},
		"FrozenUnixNano": func(value *packagecontract.VerifiedAgentPackageClosureV1) {
			value.Publication.Generation.CreatedUnixNano++
		},
		"HandoffClosure": func(value *packagecontract.VerifiedAgentPackageClosureV1) {
			value.Publication.Handoff.ManifestDigest = core.DigestBytes([]byte("spliced-handoff-closure"))
		},
		"ClosureDigest": func(value *packagecontract.VerifiedAgentPackageClosureV1) {
			value.ClosureDigest = core.DigestBytes([]byte("spliced-closure"))
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			changed := packagecontract.CloneVerifiedAgentPackageClosureV1(base)
			mutate(&changed)
			reader := &mutatedClosureReaderV1{value: changed}
			counting := &countingSelectionStoreV1{inner: fixture.selectionStore}
			service, serviceErr := selection.NewServiceV1(selection.ConfigV1{
				Closures:   reader,
				Selections: counting,
				Clock:      func() time.Time { return fixture.now },
			})
			if serviceErr != nil {
				t.Fatal(serviceErr)
			}
			if _, serviceErr = service.SelectPackageV1(context.Background(), request); serviceErr == nil {
				t.Fatal("spliced verified closure axis was selected")
			}
			if reader.loads.Load() != 1 || counting.appends.Load() != 0 || counting.inspects.Load() != 0 {
				t.Fatalf(
					"spliced closure reached selection Owner: loads=%d CAS=%d Inspect=%d",
					reader.loads.Load(),
					counting.appends.Load(),
					counting.inspects.Load(),
				)
			}
		})
	}
}

func mustLoaderV1(
	t *testing.T,
	packages ports.AgentPackageExactReaderV1,
	publications ports.HarnessAssemblyPublicationHistoricalReaderV2,
) *loader.LoaderV1 {
	t.Helper()
	value, err := loader.NewV1(packages, publications)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestPackageSelectionCASRaceAdvanceHistoryAndRestartV1(t *testing.T) {
	fixture := newSelectionFixtureV1(t)
	ctx := context.Background()
	request := selectionRequestV1(t, fixture, "agent/example/raced-package", packagecontract.AgentPackageSelectionCurrentRefV1{})
	var serviceClockUnixNano atomic.Int64
	serviceClockUnixNano.Store(fixture.now.Add(-time.Second).UnixNano())
	racingService, err := selection.NewServiceV1(selection.ConfigV1{
		Closures:   fixture.loader,
		Selections: fixture.selectionStore,
		Clock: func() time.Time {
			return time.Unix(0, serviceClockUnixNano.Add(1))
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	const workers = 64
	start := make(chan struct{})
	results := make(chan error, workers)
	var wait sync.WaitGroup
	for index := 0; index < workers; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			_, err := racingService.SelectPackageV1(ctx, request)
			results <- err
		}()
	}
	close(start)
	wait.Wait()
	close(results)
	var success, conflict int
	for err := range results {
		switch {
		case err == nil:
			success++
		case core.HasCategory(err, core.ErrorConflict):
			conflict++
		default:
			t.Fatalf("unexpected concurrent selection error: %v", err)
		}
	}
	if success != 1 || conflict != workers-1 {
		t.Fatalf("64-way varying-clock Service calls must have one successor: success=%d conflict=%d", success, conflict)
	}
	db, err := sql.Open("sqlite", "file:"+fixture.selectionPath)
	if err != nil {
		t.Fatal(err)
	}
	var historyCount int
	if err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM agent_package_selection_history_v1 WHERE selection_id=?", request.SelectionID).Scan(&historyCount); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	var wal string
	var synchronous int
	if err = db.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&wal); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err = db.QueryRowContext(ctx, "PRAGMA synchronous").Scan(&synchronous); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	if historyCount != 1 || wal != "wal" || synchronous != 2 {
		t.Fatalf("race produced more than one successor or WAL/FULL is inactive: history=%d wal=%s synchronous=%d", historyCount, wal, synchronous)
	}
	first, err := fixture.service.InspectCurrentV1(ctx, request.SelectionID)
	if err != nil {
		t.Fatal(err)
	}
	advance := selectionRequestV1(t, fixture, request.SelectionID, first.RefV1())
	second, err := fixture.service.SelectPackageV1(ctx, advance)
	if err != nil {
		t.Fatal(err)
	}
	if second.Ref.Revision != first.Ref.Revision+1 {
		t.Fatal("selection advance did not increment revision by exactly one")
	}
	historical, err := fixture.selectionStore.InspectAgentPackageSelectionExactV1(ctx, first.RefV1())
	if err != nil || !reflect.DeepEqual(historical, first) {
		t.Fatalf("append-only selection history lost predecessor: %v", err)
	}
	replayed, err := fixture.service.SelectPackageV1(ctx, advance)
	if err != nil || !reflect.DeepEqual(replayed, second) {
		t.Fatalf("same expected/same next did not read back idempotently: %v", err)
	}
	advanceThird := selectionRequestV1(t, fixture, request.SelectionID, second.RefV1())
	third, err := fixture.service.SelectPackageV1(ctx, advanceThird)
	if err != nil {
		t.Fatal(err)
	}
	replayedAfterAdvance, err := fixture.service.SelectPackageV1(ctx, advance)
	if err != nil || !reflect.DeepEqual(replayedAfterAdvance, second) {
		t.Fatalf("rev2 replay after current advanced to rev3 did not return original rev2: %v", err)
	}
	currentAfterReplay, err := fixture.service.InspectCurrentV1(ctx, request.SelectionID)
	if err != nil || !reflect.DeepEqual(currentAfterReplay, third) {
		t.Fatalf("historical replay moved current away from rev3: %v", err)
	}
	for name, mutate := range map[string]func(*packagecontract.AgentPackageSelectionCurrentRefV1){
		"wrong digest": func(ref *packagecontract.AgentPackageSelectionCurrentRefV1) {
			ref.Digest = core.DigestBytes([]byte("forged-replay-predecessor"))
		},
		"wrong expiry": func(ref *packagecontract.AgentPackageSelectionCurrentRefV1) {
			ref.ExpiresUnixNano++
		},
	} {
		t.Run("replay rejects "+name+" predecessor", func(t *testing.T) {
			forged := first.Ref
			mutate(&forged)
			forgedReplay := selectionRequestV1(t, fixture, request.SelectionID, forged)
			if _, replayErr := fixture.service.SelectPackageV1(ctx, forgedReplay); !core.HasCategory(replayErr, core.ErrorConflict) {
				t.Fatalf("historical rev2 accepted forged predecessor %s: %v", name, replayErr)
			}
			exactSecond, exactErr := fixture.selectionStore.InspectAgentPackageSelectionExactV1(ctx, second.Ref)
			if exactErr != nil || !reflect.DeepEqual(exactSecond, second) {
				t.Fatalf("forged predecessor changed rev2 history: %v", exactErr)
			}
			stillThird, currentErr := fixture.service.InspectCurrentV1(ctx, request.SelectionID)
			if currentErr != nil || !reflect.DeepEqual(stillThird, third) {
				t.Fatalf("forged predecessor moved current away from rev3: %v", currentErr)
			}
		})
	}
	different, err := packagecontract.SealAgentPackageSelectionRequestV1(packagecontract.AgentPackageSelectionRequestV1{
		SelectionID:               request.SelectionID,
		PackageRef:                fixture.pkg.RefV1(),
		ExpectedCurrent:           first.RefV1(),
		RequestedNotAfterUnixNano: fixture.now.Add(30 * time.Minute).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = fixture.service.SelectPackageV1(ctx, different); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("same expected/different next did not conflict: %v", err)
	}
	if err = fixture.selectionStore.IntegrityCheckV1(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestPackageSelectionLostReplyIsExactInspectOnlyV1(t *testing.T) {
	fixture := newSelectionFixtureV1(t)
	counting := &countingSelectionStoreV1{inner: fixture.selectionStore}
	service, err := selection.NewServiceV1(selection.ConfigV1{
		Closures:   fixture.loader,
		Selections: counting,
		Clock:      func() time.Time { return fixture.now },
	})
	if err != nil {
		t.Fatal(err)
	}
	fixture.selectionStore.LoseNextAppendReplyV1()
	request := selectionRequestV1(t, fixture, "agent/example/lost-reply", packagecontract.AgentPackageSelectionCurrentRefV1{})
	current, err := service.SelectPackageV1(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if counting.appends.Load() != 1 || counting.inspects.Load() != 1 || current.Ref.Revision != 1 {
		t.Fatalf("lost reply was not one CAS followed by one exact Inspect: append=%d inspect=%d", counting.appends.Load(), counting.inspects.Load())
	}
}

type unknownSelectionStoreV1 struct {
	appends  atomic.Int32
	exacts   atomic.Int32
	currents atomic.Int32
}

func (store *unknownSelectionStoreV1) CompareAndSwapAgentPackageSelectionCurrentV1(
	context.Context,
	packagecontract.AgentPackageSelectionCurrentRefV1,
	packagecontract.AgentPackageSelectionCurrentV1,
) (packagecontract.AgentPackageSelectionCurrentV1, error) {
	store.appends.Add(1)
	return packagecontract.AgentPackageSelectionCurrentV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "unknown")
}
func (store *unknownSelectionStoreV1) InspectAgentPackageSelectionExactV1(
	context.Context,
	packagecontract.AgentPackageSelectionCurrentRefV1,
) (packagecontract.AgentPackageSelectionCurrentV1, error) {
	store.exacts.Add(1)
	return packagecontract.AgentPackageSelectionCurrentV1{}, core.NewError(core.ErrorNotFound, core.ReasonEvidenceUnavailable, "missing")
}
func (store *unknownSelectionStoreV1) InspectAgentPackageSelectionCurrentV1(
	context.Context,
	string,
) (packagecontract.AgentPackageSelectionCurrentV1, error) {
	store.currents.Add(1)
	return packagecontract.AgentPackageSelectionCurrentV1{}, errors.New("unexpected current read")
}

func TestPackageSelectionUnknownWithoutEvidenceRemainsUnknownV1(t *testing.T) {
	fixture := newSelectionFixtureV1(t)
	store := &unknownSelectionStoreV1{}
	service, err := selection.NewServiceV1(selection.ConfigV1{
		Closures:   fixture.loader,
		Selections: store,
		Clock:      func() time.Time { return fixture.now },
	})
	if err != nil {
		t.Fatal(err)
	}
	request := selectionRequestV1(t, fixture, "agent/example/unknown", packagecontract.AgentPackageSelectionCurrentRefV1{})
	_, err = service.SelectPackageV1(context.Background(), request)
	if !core.HasCategory(err, core.ErrorIndeterminate) ||
		store.appends.Load() != 1 ||
		store.exacts.Load() != 1 ||
		store.currents.Load() != 0 {
		t.Fatalf("unknown outcome retried or escaped exact Inspect: %v append=%d exact=%d current=%d", err, store.appends.Load(), store.exacts.Load(), store.currents.Load())
	}
}

func TestPackageSelectionRestartExpiryAndSchemaFailClosedV1(t *testing.T) {
	ctx := context.Background()
	fixture := newSelectionFixtureV1(t)
	request := selectionRequestV1(t, fixture, "agent/example/restart", packagecontract.AgentPackageSelectionCurrentRefV1{})
	current, err := fixture.service.SelectPackageV1(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	path := t.TempDir() + "/restart-selection.db"
	checked := fixture.now
	store, err := repository.OpenSelectionSQLiteV1(ctx, repository.SelectionSQLiteConfigV1{
		Path:  path,
		Clock: func() time.Time { return checked },
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.CompareAndSwapAgentPackageSelectionCurrentV1(ctx, packagecontract.AgentPackageSelectionCurrentRefV1{}, current); err != nil {
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = repository.OpenSelectionSQLiteV1(ctx, repository.SelectionSQLiteConfigV1{
		Path:  path,
		Clock: func() time.Time { return checked },
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.InspectAgentPackageSelectionExactV1(ctx, current.RefV1()); err != nil {
		t.Fatalf("restart lost exact selection history: %v", err)
	}
	checked = time.Unix(0, current.ExpiresUnixNano)
	if _, err = store.InspectAgentPackageSelectionCurrentV1(ctx, current.Ref.SelectionID); !core.HasReason(err, core.ReasonBindingExpired) {
		t.Fatalf("expired current did not fail closed: %v", err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}

	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, "UPDATE agent_package_selection_schema_v1 SET digest=?", string(core.DigestBytes([]byte("wrong-schema")))); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	if reopened, openErr := repository.OpenSelectionSQLiteV1(ctx, repository.SelectionSQLiteConfigV1{
		Path:  path,
		Clock: func() time.Time { return fixture.now },
	}); openErr == nil {
		_ = reopened.Close()
		t.Fatal("schema digest drift was accepted")
	}
}

func TestPackageSelectionOpenRejectsActualSchemaDriftWithValidLedgerDigestV1(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(1_800_100_000, 0).UTC()
	referencePath := t.TempDir() + "/reference.db"
	reference, err := repository.OpenSelectionSQLiteV1(ctx, repository.SelectionSQLiteConfigV1{
		Path:  referencePath,
		Clock: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = reference.Close(); err != nil {
		t.Fatal(err)
	}
	referenceDB, err := sql.Open("sqlite", "file:"+referencePath)
	if err != nil {
		t.Fatal(err)
	}
	var publicDigest string
	if err = referenceDB.QueryRowContext(ctx, "SELECT digest FROM agent_package_selection_schema_v1 WHERE version=1").Scan(&publicDigest); err != nil {
		_ = referenceDB.Close()
		t.Fatal(err)
	}
	if err = referenceDB.Close(); err != nil {
		t.Fatal(err)
	}

	driftPath := t.TempDir() + "/schema-drift.db"
	driftDB, err := sql.Open("sqlite", "file:"+driftPath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = driftDB.ExecContext(
		ctx,
		"CREATE TABLE agent_package_selection_schema_v1(version INTEGER,digest TEXT NOT NULL,applied_unix_nano INTEGER NOT NULL);"+
			"CREATE TABLE agent_package_selection_history_v1(selection_id TEXT NOT NULL,revision INTEGER NOT NULL,digest TEXT NOT NULL,row_digest TEXT NOT NULL,payload_json BLOB NOT NULL,checked_unix_nano INTEGER NOT NULL,expires_unix_nano INTEGER NOT NULL);"+
			"CREATE TABLE agent_package_selection_current_v1(selection_id TEXT,revision INTEGER NOT NULL,digest TEXT NOT NULL,row_digest TEXT NOT NULL);"+
			"INSERT INTO agent_package_selection_schema_v1(version,digest,applied_unix_nano) VALUES(1,?,?)",
		publicDigest,
		now.UnixNano(),
	)
	if err != nil {
		_ = driftDB.Close()
		t.Fatal(err)
	}
	if err = driftDB.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, openErr := repository.OpenSelectionSQLiteV1(ctx, repository.SelectionSQLiteConfigV1{
		Path:  driftPath,
		Clock: func() time.Time { return now },
	})
	if openErr == nil {
		_ = reopened.Close()
		t.Fatal("same-name same-column tables without PK, UNIQUE and FK constraints were accepted")
	}
	if !core.HasCategory(openErr, core.ErrorConflict) || !core.HasReason(openErr, core.ReasonInvalidDigest) {
		t.Fatalf("actual schema drift returned the wrong closed failure: %v", openErr)
	}
}

func TestPackageSelectionPersistentJSONRejectsStrictDriftWithSynchronizedRowDigestV1(t *testing.T) {
	ctx := context.Background()
	type rowDigestInputV1 struct {
		SelectionID   string
		Revision      core.Revision
		Digest        core.Digest
		PayloadDigest core.Digest
	}
	tests := []struct {
		name   string
		mutate func([]byte) []byte
	}{
		{
			name: "duplicate first conflict last valid",
			mutate: func(raw []byte) []byte {
				return append([]byte(`{"object_kind":"spliced-object-kind",`), raw[1:]...)
			},
		},
		{
			name: "unknown field",
			mutate: func(raw []byte) []byte {
				return append([]byte(`{"unexpected_selection_field":true,`), raw[1:]...)
			},
		},
		{
			name: "trailing document",
			mutate: func(raw []byte) []byte {
				return append(append([]byte(nil), raw...), []byte(` {}`)...)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newSelectionFixtureV1(t)
			request := selectionRequestV1(t, fixture, "agent/example/strict-json-"+strings.ReplaceAll(test.name, " ", "-"), packagecontract.AgentPackageSelectionCurrentRefV1{})
			current, err := fixture.service.SelectPackageV1(ctx, request)
			if err != nil {
				t.Fatal(err)
			}
			if err = fixture.selectionStore.Close(); err != nil {
				t.Fatal(err)
			}
			db, err := sql.Open("sqlite", "file:"+fixture.selectionPath)
			if err != nil {
				t.Fatal(err)
			}
			var raw []byte
			var storedDigest string
			if err = db.QueryRowContext(
				ctx,
				"SELECT payload_json,digest FROM agent_package_selection_history_v1 WHERE selection_id=? AND revision=?",
				current.Ref.SelectionID,
				uint64(current.Ref.Revision),
			).Scan(&raw, &storedDigest); err != nil {
				_ = db.Close()
				t.Fatal(err)
			}
			mutated := test.mutate(raw)
			rowDigest, digestErr := core.CanonicalJSONDigest(
				"praxis.agent-builder.selection-sqlite",
				"v1",
				"AgentPackageSelectionSQLiteRowV1",
				rowDigestInputV1{
					SelectionID:   current.Ref.SelectionID,
					Revision:      current.Ref.Revision,
					Digest:        core.Digest(storedDigest),
					PayloadDigest: core.DigestBytes(mutated),
				},
			)
			if digestErr != nil {
				_ = db.Close()
				t.Fatal(digestErr)
			}
			if _, err = db.ExecContext(
				ctx,
				"UPDATE agent_package_selection_history_v1 SET payload_json=?,row_digest=? WHERE selection_id=? AND revision=?",
				mutated,
				string(rowDigest),
				current.Ref.SelectionID,
				uint64(current.Ref.Revision),
			); err != nil {
				_ = db.Close()
				t.Fatal(err)
			}
			if err = db.Close(); err != nil {
				t.Fatal(err)
			}
			fixture.selectionStore, err = repository.OpenSelectionSQLiteV1(ctx, repository.SelectionSQLiteConfigV1{
				Path:  fixture.selectionPath,
				Clock: func() time.Time { return fixture.now },
			})
			if err != nil {
				t.Fatal(err)
			}
			_, inspectErr := fixture.selectionStore.InspectAgentPackageSelectionExactV1(ctx, current.RefV1())
			if !core.HasCategory(inspectErr, core.ErrorConflict) || !core.HasReason(inspectErr, core.ReasonInvalidDigest) {
				t.Fatalf("strict JSON drift with synchronized row digest was accepted: %v", inspectErr)
			}
		})
	}
}

func TestPackageSelectionRepositoryClockRejectsFutureExpiredAndExpiredAdvanceV1(t *testing.T) {
	ctx := context.Background()
	fixture := newSelectionFixtureV1(t)
	closure, err := fixture.loader.LoadVerifiedAgentPackageClosureV1(ctx, fixture.pkg.RefV1())
	if err != nil {
		t.Fatal(err)
	}
	repoNow := fixture.now
	store, err := repository.OpenSelectionSQLiteV1(ctx, repository.SelectionSQLiteConfigV1{
		Path:  t.TempDir() + "/clock-selection.db",
		Clock: func() time.Time { return repoNow },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	seal := func(id string, revision core.Revision, checked, expires time.Time) packagecontract.AgentPackageSelectionCurrentV1 {
		t.Helper()
		value, sealErr := packagecontract.SealAgentPackageSelectionCurrentV1(packagecontract.AgentPackageSelectionCurrentV1{
			Ref: packagecontract.AgentPackageSelectionCurrentRefV1{
				SelectionID:     id,
				Revision:        revision,
				ExpiresUnixNano: expires.UnixNano(),
			},
			PackageRef:      fixture.pkg.RefV1(),
			PublicationRef:  closure.PublicationRefV2(),
			ClosureDigest:   closure.ClosureDigest,
			CheckedUnixNano: checked.UnixNano(),
			ExpiresUnixNano: expires.UnixNano(),
		})
		if sealErr != nil {
			t.Fatal(sealErr)
		}
		return value
	}

	future := seal("agent/example/future", 1, repoNow.Add(time.Second), repoNow.Add(time.Hour))
	if _, err = store.CompareAndSwapAgentPackageSelectionCurrentV1(ctx, packagecontract.AgentPackageSelectionCurrentRefV1{}, future); !core.HasReason(err, core.ReasonClockRegression) {
		t.Fatalf("future-checked next was accepted: %v", err)
	}
	if _, err = store.InspectAgentPackageSelectionExactV1(ctx, future.Ref); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("future-checked rejection wrote history: %v", err)
	}

	atExpiry := seal("agent/example/at-expiry", 1, repoNow.Add(-time.Second), repoNow)
	if _, err = store.CompareAndSwapAgentPackageSelectionCurrentV1(ctx, packagecontract.AgentPackageSelectionCurrentRefV1{}, atExpiry); !core.HasReason(err, core.ReasonBindingExpired) {
		t.Fatalf("next with now==expires was accepted: %v", err)
	}
	if _, err = store.InspectAgentPackageSelectionExactV1(ctx, atExpiry.Ref); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("expired next rejection wrote history: %v", err)
	}

	first := seal("agent/example/expired-predecessor", 1, repoNow, repoNow.Add(time.Minute))
	if _, err = store.CompareAndSwapAgentPackageSelectionCurrentV1(ctx, packagecontract.AgentPackageSelectionCurrentRefV1{}, first); err != nil {
		t.Fatal(err)
	}
	repoNow = time.Unix(0, first.ExpiresUnixNano)
	if _, err = store.InspectAgentPackageSelectionExactV1(ctx, first.Ref); err != nil {
		t.Fatalf("expired exact historical record became unreadable: %v", err)
	}
	second := seal(first.Ref.SelectionID, 2, repoNow, repoNow.Add(time.Hour))
	if _, err = store.CompareAndSwapAgentPackageSelectionCurrentV1(ctx, first.Ref, second); !core.HasReason(err, core.ReasonBindingExpired) {
		t.Fatalf("advance from expired predecessor was accepted: %v", err)
	}
	if _, err = store.InspectAgentPackageSelectionExactV1(ctx, second.Ref); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("expired predecessor rejection wrote a successor: %v", err)
	}
	if _, err = store.InspectAgentPackageSelectionCurrentV1(ctx, first.Ref.SelectionID); !core.HasReason(err, core.ReasonBindingExpired) {
		t.Fatalf("expired current remained authoritative: %v", err)
	}
	for name, selectionID := range map[string]string{
		"leading whitespace":  " agent/example/invalid",
		"trailing whitespace": "agent/example/invalid ",
		"oversize":            strings.Repeat("x", packagecontract.MaxPackageSelectionIDBytesV1+1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, inspectErr := store.InspectAgentPackageSelectionCurrentV1(ctx, selectionID); !core.HasCategory(inspectErr, core.ErrorInvalidArgument) {
				t.Fatalf("invalid SelectionID reached current lookup: %v", inspectErr)
			}
		})
	}
}

func TestPackageSelectionRepositoryResamplesClockAfterLockAndBeforeCommitV1(t *testing.T) {
	ctx := context.Background()
	fixture := newSelectionFixtureV1(t)
	closure, err := fixture.loader.LoadVerifiedAgentPackageClosureV1(ctx, fixture.pkg.RefV1())
	if err != nil {
		t.Fatal(err)
	}
	base := fixture.now
	expires := base.Add(time.Minute)
	var controlled atomic.Bool
	var calls atomic.Int32
	var nowUnixNano atomic.Int64
	nowUnixNano.Store(base.UnixNano())
	commitClockReached := make(chan struct{})
	releaseCommitClock := make(chan struct{})
	store, err := repository.OpenSelectionSQLiteV1(ctx, repository.SelectionSQLiteConfigV1{
		Path: t.TempDir() + "/blocked-clock.db",
		Clock: func() time.Time {
			if controlled.Load() {
				call := calls.Add(1)
				if call == 2 {
					close(commitClockReached)
					<-releaseCommitClock
				}
			}
			return time.Unix(0, nowUnixNano.Load())
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	controlled.Store(true)

	seal := func(id string) packagecontract.AgentPackageSelectionCurrentV1 {
		t.Helper()
		value, sealErr := packagecontract.SealAgentPackageSelectionCurrentV1(packagecontract.AgentPackageSelectionCurrentV1{
			Ref: packagecontract.AgentPackageSelectionCurrentRefV1{
				SelectionID:     id,
				Revision:        1,
				ExpiresUnixNano: expires.UnixNano(),
			},
			PackageRef:      fixture.pkg.RefV1(),
			PublicationRef:  closure.PublicationRefV2(),
			ClosureDigest:   closure.ClosureDigest,
			CheckedUnixNano: base.UnixNano(),
			ExpiresUnixNano: expires.UnixNano(),
		})
		if sealErr != nil {
			t.Fatal(sealErr)
		}
		return value
	}
	first := seal("agent/example/blocked-clock-first")
	second := seal("agent/example/blocked-clock-second")
	results := make(chan error, 2)
	go func() {
		_, casErr := store.CompareAndSwapAgentPackageSelectionCurrentV1(ctx, packagecontract.AgentPackageSelectionCurrentRefV1{}, first)
		results <- casErr
	}()
	select {
	case <-commitClockReached:
	case <-time.After(5 * time.Second):
		t.Fatal("first CAS did not reach commit-time clock while holding the lock")
	}
	secondStarted := make(chan struct{})
	go func() {
		close(secondStarted)
		_, casErr := store.CompareAndSwapAgentPackageSelectionCurrentV1(ctx, packagecontract.AgentPackageSelectionCurrentRefV1{}, second)
		results <- casErr
	}()
	<-secondStarted
	nowUnixNano.Store(expires.UnixNano())
	close(releaseCommitClock)
	for index := 0; index < 2; index++ {
		if casErr := <-results; !core.HasReason(casErr, core.ReasonBindingExpired) {
			t.Fatalf("CAS waiting across the lock published an expired current: %v", casErr)
		}
	}
	for _, value := range []packagecontract.AgentPackageSelectionCurrentV1{first, second} {
		if _, inspectErr := store.InspectAgentPackageSelectionExactV1(ctx, value.Ref); !core.HasCategory(inspectErr, core.ErrorNotFound) {
			t.Fatalf("expired blocked CAS left visible history: %v", inspectErr)
		}
	}

	var regressionControlled atomic.Bool
	var regressionCalls atomic.Int32
	regressionStore, err := repository.OpenSelectionSQLiteV1(ctx, repository.SelectionSQLiteConfigV1{
		Path: t.TempDir() + "/regressed-commit-clock.db",
		Clock: func() time.Time {
			if !regressionControlled.Load() {
				return base
			}
			if regressionCalls.Add(1) == 1 {
				return base
			}
			return base.Add(-time.Nanosecond)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer regressionStore.Close()
	regressionControlled.Store(true)
	regressed := seal("agent/example/regressed-commit-clock")
	if _, err = regressionStore.CompareAndSwapAgentPackageSelectionCurrentV1(ctx, packagecontract.AgentPackageSelectionCurrentRefV1{}, regressed); !core.HasReason(err, core.ReasonClockRegression) {
		t.Fatalf("commit-time clock regression was accepted: %v", err)
	}
	if _, err = regressionStore.InspectAgentPackageSelectionExactV1(ctx, regressed.Ref); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("commit-time clock regression left visible history: %v", err)
	}
}

func TestPackageSelectionCurrentPointerDigestAndRollbackFailClosedV1(t *testing.T) {
	ctx := context.Background()
	fixture := newSelectionFixtureV1(t)
	closure, err := fixture.loader.LoadVerifiedAgentPackageClosureV1(ctx, fixture.pkg.RefV1())
	if err != nil {
		t.Fatal(err)
	}
	type pointerFixtureV1 struct {
		path                  string
		first                 packagecontract.AgentPackageSelectionCurrentV1
		second                packagecontract.AgentPackageSelectionCurrentV1
		third                 packagecontract.AgentPackageSelectionCurrentV1
		firstPointerRowDigest string
	}
	setup := func(t *testing.T, id string) pointerFixtureV1 {
		t.Helper()
		path := t.TempDir() + "/pointer.db"
		store, openErr := repository.OpenSelectionSQLiteV1(ctx, repository.SelectionSQLiteConfigV1{
			Path:  path,
			Clock: func() time.Time { return fixture.now },
		})
		if openErr != nil {
			t.Fatal(openErr)
		}
		seal := func(revision core.Revision) packagecontract.AgentPackageSelectionCurrentV1 {
			value, sealErr := packagecontract.SealAgentPackageSelectionCurrentV1(packagecontract.AgentPackageSelectionCurrentV1{
				Ref: packagecontract.AgentPackageSelectionCurrentRefV1{
					SelectionID:     id,
					Revision:        revision,
					ExpiresUnixNano: fixture.now.Add(time.Hour).UnixNano(),
				},
				PackageRef:      fixture.pkg.RefV1(),
				PublicationRef:  closure.PublicationRefV2(),
				ClosureDigest:   closure.ClosureDigest,
				CheckedUnixNano: fixture.now.UnixNano(),
				ExpiresUnixNano: fixture.now.Add(time.Hour).UnixNano(),
			})
			if sealErr != nil {
				t.Fatal(sealErr)
			}
			return value
		}
		first := seal(1)
		if _, openErr = store.CompareAndSwapAgentPackageSelectionCurrentV1(ctx, packagecontract.AgentPackageSelectionCurrentRefV1{}, first); openErr != nil {
			_ = store.Close()
			t.Fatal(openErr)
		}
		db, openErr := sql.Open("sqlite", "file:"+path)
		if openErr != nil {
			_ = store.Close()
			t.Fatal(openErr)
		}
		var firstPointerRowDigest string
		if openErr = db.QueryRowContext(ctx, "SELECT row_digest FROM agent_package_selection_current_v1 WHERE selection_id=?", id).Scan(&firstPointerRowDigest); openErr != nil {
			_ = db.Close()
			_ = store.Close()
			t.Fatal(openErr)
		}
		if openErr = db.Close(); openErr != nil {
			_ = store.Close()
			t.Fatal(openErr)
		}
		second := seal(2)
		if _, openErr = store.CompareAndSwapAgentPackageSelectionCurrentV1(ctx, first.Ref, second); openErr != nil {
			_ = store.Close()
			t.Fatal(openErr)
		}
		if openErr = store.Close(); openErr != nil {
			t.Fatal(openErr)
		}
		return pointerFixtureV1{path: path, first: first, second: second, third: seal(3), firstPointerRowDigest: firstPointerRowDigest}
	}
	openAfterMutation := func(t *testing.T, value pointerFixtureV1) *repository.SelectionSQLiteV1 {
		t.Helper()
		store, openErr := repository.OpenSelectionSQLiteV1(ctx, repository.SelectionSQLiteConfigV1{
			Path:  value.path,
			Clock: func() time.Time { return fixture.now },
		})
		if openErr != nil {
			t.Fatal(openErr)
		}
		t.Cleanup(func() { _ = store.Close() })
		return store
	}

	t.Run("pointer row digest splice", func(t *testing.T) {
		value := setup(t, "agent/example/pointer-splice")
		db, openErr := sql.Open("sqlite", "file:"+value.path)
		if openErr != nil {
			t.Fatal(openErr)
		}
		if _, openErr = db.ExecContext(ctx, "UPDATE agent_package_selection_current_v1 SET row_digest=? WHERE selection_id=?", string(core.DigestBytes([]byte("spliced-pointer-row"))), value.second.Ref.SelectionID); openErr != nil {
			_ = db.Close()
			t.Fatal(openErr)
		}
		if openErr = db.Close(); openErr != nil {
			t.Fatal(openErr)
		}
		store := openAfterMutation(t, value)
		if _, inspectErr := store.InspectAgentPackageSelectionCurrentV1(ctx, value.second.Ref.SelectionID); !core.HasCategory(inspectErr, core.ErrorConflict) {
			t.Fatalf("pointer row digest splice was accepted: %v", inspectErr)
		}
		if _, casErr := store.CompareAndSwapAgentPackageSelectionCurrentV1(ctx, value.second.Ref, value.third); !core.HasCategory(casErr, core.ErrorConflict) {
			t.Fatalf("CAS accepted a spliced current pointer row: %v", casErr)
		}
		if _, inspectErr := store.InspectAgentPackageSelectionExactV1(ctx, value.third.Ref); !core.HasCategory(inspectErr, core.ErrorNotFound) {
			t.Fatalf("spliced-pointer CAS wrote a successor: %v", inspectErr)
		}
		if _, inspectErr := store.InspectAgentPackageSelectionExactV1(ctx, value.second.Ref); inspectErr != nil {
			t.Fatalf("pointer corruption hid valid exact history: %v", inspectErr)
		}
	})

	t.Run("pointer rollback to valid history", func(t *testing.T) {
		value := setup(t, "agent/example/pointer-rollback")
		db, openErr := sql.Open("sqlite", "file:"+value.path)
		if openErr != nil {
			t.Fatal(openErr)
		}
		if _, openErr = db.ExecContext(
			ctx,
			"UPDATE agent_package_selection_current_v1 SET revision=?,digest=?,row_digest=? WHERE selection_id=?",
			uint64(value.first.Ref.Revision),
			string(value.first.Ref.Digest),
			value.firstPointerRowDigest,
			value.first.Ref.SelectionID,
		); openErr != nil {
			_ = db.Close()
			t.Fatal(openErr)
		}
		if openErr = db.Close(); openErr != nil {
			t.Fatal(openErr)
		}
		store := openAfterMutation(t, value)
		if _, inspectErr := store.InspectAgentPackageSelectionCurrentV1(ctx, value.first.Ref.SelectionID); !core.HasCategory(inspectErr, core.ErrorConflict) {
			t.Fatalf("pointer rollback behind MAX(history) was accepted: %v", inspectErr)
		}
		if _, casErr := store.CompareAndSwapAgentPackageSelectionCurrentV1(ctx, value.second.Ref, value.third); !core.HasCategory(casErr, core.ErrorConflict) {
			t.Fatalf("CAS accepted a pointer rolled back behind MAX(history): %v", casErr)
		}
		if _, inspectErr := store.InspectAgentPackageSelectionExactV1(ctx, value.third.Ref); !core.HasCategory(inspectErr, core.ErrorNotFound) {
			t.Fatalf("rolled-back-pointer CAS wrote a successor: %v", inspectErr)
		}
		for _, exact := range []packagecontract.AgentPackageSelectionCurrentRefV1{value.first.Ref, value.second.Ref} {
			if _, inspectErr := store.InspectAgentPackageSelectionExactV1(ctx, exact); inspectErr != nil {
				t.Fatalf("pointer rollback hid append-only history: %v", inspectErr)
			}
		}
	})
}
