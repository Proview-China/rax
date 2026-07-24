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

func TestCompileAndPublishV2ExposesOneCompleteBarrierAndDefensiveReaders(t *testing.T) {
	now := assemblytestkit.Now
	store := NewMemoryStoreV2()
	publisher := newTestPublisher(t, store, func() time.Time { return now })
	request := initialRequest(now, "attempt-1")

	result, err := publisher.CompileAndPublishAssemblyV2(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result.RecoveredByInspect || result.Publication.PublicationID == "" || result.Current.Publication.PublicationID != result.Publication.PublicationID || result.Current.Revision != 1 {
		t.Fatalf("unexpected publish result: %+v", result)
	}
	expectedID, err := assemblycontract.DeriveAssemblyPublicationIDV2(request.Input.Digest, result.Current.Artifacts.Generation.ID)
	if err != nil || result.Publication.PublicationID != expectedID {
		t.Fatalf("PublicationID = %q, want %q (%v)", result.Publication.PublicationID, expectedID, err)
	}

	historical, err := publisher.InspectAssemblyPublicationHistoricalV2(context.Background(), result.Current.Publication)
	if err != nil {
		t.Fatal(err)
	}
	historical.Manifest.Modules[0].ModuleID = "mutated"
	again, err := publisher.InspectAssemblyPublicationHistoricalV2(context.Background(), result.Current.Publication)
	if err != nil || again.Manifest.Modules[0].ModuleID == "mutated" {
		t.Fatalf("historical reader leaked mutable storage: %v", err)
	}
	current, err := publisher.InspectAssemblyPublicationCurrentV2(context.Background(), request.Input.ScopeRef)
	if err != nil || current.Digest != result.Current.Digest {
		t.Fatalf("current Inspect = %+v, %v", current, err)
	}
}

func TestStagedPartialV2IsNeverHistoricallyOrCurrentlyReadable(t *testing.T) {
	ctx := context.Background()
	input := assemblytestkit.ValidInput()
	compiled, err := assemblycompiler.New().Compile(input)
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := assemblycontract.NewAssemblyPublicationBundleV2(input.ScopeRef, compiled)
	if err != nil {
		t.Fatal(err)
	}
	store := NewMemoryStoreV2()
	stages := []func() error{
		func() error { return store.StageGenerationV2(ctx, bundle.Publication.PublicationID, bundle.Generation) },
		func() error { return store.StageManifestV2(ctx, bundle.Publication.PublicationID, bundle.Manifest) },
		func() error { return store.StageGraphV2(ctx, bundle.Publication.PublicationID, bundle.Graph) },
		func() error { return store.StageHandoffV2(ctx, bundle.Publication.PublicationID, bundle.Handoff) },
	}
	ref := assemblycontract.AssemblyPublicationRefV2{PublicationID: bundle.Publication.PublicationID, Revision: 1, Digest: bundle.Publication.Digest}
	for index, stage := range stages {
		if err := stage(); err != nil {
			t.Fatalf("stage %d: %v", index, err)
		}
		if _, err := store.InspectHistoricalPublicationV2(ctx, ref); !core.HasCategory(err, core.ErrorNotFound) {
			t.Fatalf("stage %d leaked historical publication: %v", index, err)
		}
		if _, err := store.InspectCurrentPublicationV2(ctx, input.ScopeRef); !core.HasCategory(err, core.ErrorNotFound) {
			t.Fatalf("stage %d leaked current publication: %v", index, err)
		}
	}
}

func TestLostReplyAtEveryPublicationWritePointRecoversOnlyByExactInspect(t *testing.T) {
	for writePoint := 1; writePoint <= 5; writePoint++ {
		t.Run(string(rune('0'+writePoint)), func(t *testing.T) {
			now := assemblytestkit.Now
			store := NewMemoryStoreV2()
			store.afterWrite = func(sequence int) error {
				if sequence == writePoint {
					return core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected durable write reply loss")
				}
				return nil
			}
			publisher := newTestPublisher(t, store, func() time.Time { return now })
			result, err := publisher.CompileAndPublishAssemblyV2(context.Background(), initialRequest(now, "attempt-lost"))
			if err != nil || !result.RecoveredByInspect {
				t.Fatalf("write point %d = recovered %v, err %v", writePoint, result.RecoveredByInspect, err)
			}
		})
	}
}

func TestCrashAfterEachStagedWriteCanResumeWithoutPartialVisibility(t *testing.T) {
	for stopAfter := 1; stopAfter <= 4; stopAfter++ {
		now := assemblytestkit.Now
		ctx := context.Background()
		input := assemblytestkit.ValidInput()
		compiled, err := assemblycompiler.New().Compile(input)
		if err != nil {
			t.Fatal(err)
		}
		bundle, err := assemblycontract.NewAssemblyPublicationBundleV2(input.ScopeRef, compiled)
		if err != nil {
			t.Fatal(err)
		}
		store := NewMemoryStoreV2()
		stages := []func() error{
			func() error { return store.StageGenerationV2(ctx, bundle.Publication.PublicationID, bundle.Generation) },
			func() error { return store.StageManifestV2(ctx, bundle.Publication.PublicationID, bundle.Manifest) },
			func() error { return store.StageGraphV2(ctx, bundle.Publication.PublicationID, bundle.Graph) },
			func() error { return store.StageHandoffV2(ctx, bundle.Publication.PublicationID, bundle.Handoff) },
		}
		for index := 0; index < stopAfter; index++ {
			if err := stages[index](); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := store.InspectHistoricalPublicationV2(ctx, assemblycontract.AssemblyPublicationRefV2{PublicationID: bundle.Publication.PublicationID, Revision: 1, Digest: bundle.Publication.Digest}); !core.HasCategory(err, core.ErrorNotFound) {
			t.Fatalf("crash after write %d exposed partial history: %v", stopAfter, err)
		}
		publisher := newTestPublisher(t, store, func() time.Time { return now })
		if _, err := publisher.EnsureAssemblyPublicationV2(ctx, initialRequest(now, "attempt-resume")); err != nil {
			t.Fatalf("resume after write %d: %v", stopAfter, err)
		}
	}
}

func TestPublicationV2SameContentStagingIsIdempotentAndDriftConflicts(t *testing.T) {
	ctx := context.Background()
	input := assemblytestkit.ValidInput()
	compiled, err := assemblycompiler.New().Compile(input)
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := assemblycontract.NewAssemblyPublicationBundleV2(input.ScopeRef, compiled)
	if err != nil {
		t.Fatal(err)
	}
	store := NewMemoryStoreV2()
	if err := store.StageManifestV2(ctx, bundle.Publication.PublicationID, bundle.Manifest); err != nil {
		t.Fatal(err)
	}
	if err := store.StageManifestV2(ctx, bundle.Publication.PublicationID, clone(bundle.Manifest)); err != nil {
		t.Fatalf("same content staging was not idempotent: %v", err)
	}
	drift := clone(bundle.Manifest)
	drift.Digest = core.DigestBytes([]byte("drift"))
	if err := store.StageManifestV2(ctx, bundle.Publication.PublicationID, drift); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("same PublicationID drift = %v", err)
	}
}

func TestPublicationV2StalePredecessorCannotSucceedEvenForSameDesiredContent(t *testing.T) {
	now := assemblytestkit.Now
	store := NewMemoryStoreV2()
	publisher := newTestPublisher(t, store, func() time.Time { return now })
	request := initialRequest(now, "attempt-first")
	if _, err := publisher.CompileAndPublishAssemblyV2(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	request.AttemptID = "attempt-replay"
	if _, err := publisher.CompileAndPublishAssemblyV2(context.Background(), request); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("stale predecessor replay = %v", err)
	}
}

func TestPublicationV2RejectsABAAndAllowsExactMonotonicSuccessor(t *testing.T) {
	now := assemblytestkit.Now
	store := NewMemoryStoreV2()
	publisher := newTestPublisher(t, store, func() time.Time { return now })
	firstRequest := initialRequest(now, "attempt-a")
	first, err := publisher.CompileAndPublishAssemblyV2(context.Background(), firstRequest)
	if err != nil {
		t.Fatal(err)
	}
	secondInput := nextInput(t, firstRequest.Input, first.Current.Artifacts.Generation)
	secondRequest := assemblycontract.CompileAndPublishAssemblyRequestV2{
		ContractVersion: assemblycontract.PublicationContractVersionV2, AttemptID: "attempt-b", Input: secondInput,
		ExpectedCurrent:          assemblycontract.AssemblyPublicationCurrentExpectationV2{Exists: true, Revision: first.Current.Revision, Digest: first.Current.Digest},
		RequestedExpiresUnixNano: now.Add(2 * time.Minute).UnixNano(),
	}
	second, err := publisher.CompileAndPublishAssemblyV2(context.Background(), secondRequest)
	if err != nil || second.Current.Revision != 2 {
		t.Fatalf("successor = %+v, %v", second, err)
	}
	firstCommit, err := store.InspectCommittedPublicationCurrentV2(context.Background(), first.Current.Publication)
	if err != nil || firstCommit.Digest != first.Current.Digest || firstCommit.Revision != 1 {
		t.Fatalf("historical committed-current recovery = %+v, %v", firstCommit, err)
	}
	aba := firstRequest
	aba.AttemptID = "attempt-a-again"
	aba.ExpectedCurrent = assemblycontract.AssemblyPublicationCurrentExpectationV2{Exists: true, Revision: second.Current.Revision, Digest: second.Current.Digest}
	if _, err := publisher.CompileAndPublishAssemblyV2(context.Background(), aba); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("ABA publication = %v", err)
	}
}

func TestPublicationV2SixtyFourConcurrentPublishersHaveOneCASWinner(t *testing.T) {
	now := assemblytestkit.Now
	store := NewMemoryStoreV2()
	publisher := newTestPublisher(t, store, func() time.Time { return now })
	start := make(chan struct{})
	var winners atomic.Int64
	var failures atomic.Int64
	var wait sync.WaitGroup
	for index := 0; index < 64; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			<-start
			request := initialRequest(now, "attempt-concurrent-"+twoDigits(index))
			_, err := publisher.CompileAndPublishAssemblyV2(context.Background(), request)
			if err == nil {
				winners.Add(1)
				return
			}
			if !core.HasCategory(err, core.ErrorConflict) {
				failures.Add(1)
			}
		}(index)
	}
	close(start)
	wait.Wait()
	if winners.Load() != 1 || failures.Load() != 0 {
		t.Fatalf("winners=%d non-conflicts=%d", winners.Load(), failures.Load())
	}
}

func TestPublicationV2CurrentReaderFailsClosedOnExpiryAndClockRollback(t *testing.T) {
	now := assemblytestkit.Now
	clock := now
	store := NewMemoryStoreV2()
	publisher := newTestPublisher(t, store, func() time.Time { return clock })
	request := initialRequest(now, "attempt-clock")
	request.RequestedExpiresUnixNano = now.Add(time.Second).UnixNano()
	if _, err := publisher.CompileAndPublishAssemblyV2(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	clock = now.Add(2 * time.Second)
	if _, err := publisher.InspectAssemblyPublicationCurrentV2(context.Background(), request.Input.ScopeRef); !core.HasReason(err, core.ReasonBindingExpired) {
		t.Fatalf("expired current = %v", err)
	}
	clock = now.Add(-time.Nanosecond)
	if _, err := publisher.InspectAssemblyPublicationCurrentV2(context.Background(), request.Input.ScopeRef); !core.HasReason(err, core.ReasonClockRegression) {
		t.Fatalf("clock rollback = %v", err)
	}
}

func TestPublicationV2RejectsTypedNilDependencies(t *testing.T) {
	var compiler *assemblycompiler.Compiler
	var store *MemoryStoreV2
	if _, err := NewPublisherV2(compiler, NewMemoryStoreV2(), time.Now); !core.HasCategory(err, core.ErrorInvalidArgument) {
		t.Fatalf("typed-nil compiler = %v", err)
	}
	if _, err := NewPublisherV2(assemblycompiler.New(), store, time.Now); !core.HasCategory(err, core.ErrorInvalidArgument) {
		t.Fatalf("typed-nil store = %v", err)
	}
}

func initialRequest(now time.Time, attemptID string) assemblycontract.CompileAndPublishAssemblyRequestV2 {
	return assemblycontract.CompileAndPublishAssemblyRequestV2{
		ContractVersion: assemblycontract.PublicationContractVersionV2, AttemptID: attemptID, Input: assemblytestkit.ValidInput(),
		ExpectedCurrent: assemblycontract.AssemblyPublicationCurrentExpectationV2{}, RequestedExpiresUnixNano: now.Add(time.Minute).UnixNano(),
	}
}

func nextInput(t *testing.T, previous assemblycontract.AssemblyInputV1, previousGeneration assemblycontract.ObjectRefV1) assemblycontract.AssemblyInputV1 {
	t.Helper()
	value := clone(previous)
	value.InputID = "assembly-input-fixture-next"
	value.Revision++
	value.CreatedUnixNano++
	value.PreviousGenerationRef = &previousGeneration
	value.Digest = ""
	sealed, err := assemblycontract.SealAssemblyInputV1(value)
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

func newTestPublisher(t *testing.T, store OwnerStoreV2, clock func() time.Time) *PublisherV2 {
	t.Helper()
	publisher, err := NewPublisherV2(assemblycompiler.New(), store, clock)
	if err != nil {
		t.Fatal(err)
	}
	return publisher
}

func twoDigits(value int) string {
	const digits = "0123456789"
	return string([]byte{digits[(value/10)%10], digits[value%10]})
}
