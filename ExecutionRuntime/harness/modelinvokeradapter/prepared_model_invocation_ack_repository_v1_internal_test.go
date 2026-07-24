package modelinvokeradapter

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestPreparedModelInvocationAckRepositoryCreateOnceAndExactRecoveryV1(t *testing.T) {
	repo := NewInMemoryPreparedModelInvocationAckRepositoryV1()
	ack := preparedAckRepositoryFixtureV1(t, 2_000, 8_000, "surface-binding-1")

	created, err := repo.EnsureAck(context.Background(), ack)
	if err != nil || created != ack {
		t.Fatalf("EnsureAck() = %#v, %v", created, err)
	}
	replayed, err := repo.EnsureAck(context.Background(), ack)
	if err != nil || replayed != ack {
		t.Fatalf("idempotent EnsureAck() = %#v, %v", replayed, err)
	}
	byRef, err := repo.InspectExactAck(context.Background(), ack.Ref())
	if err != nil || byRef != ack {
		t.Fatalf("InspectExactAck() = %#v, %v", byRef, err)
	}
	byStableKey, err := repo.inspectByPreparedCurrent(context.Background(), ack.PreparedRef, ack.CurrentRef)
	if err != nil || byStableKey != ack {
		t.Fatalf("inspectByPreparedCurrent() = %#v, %v", byStableKey, err)
	}

	created.SurfaceBindingRef.ID = "mutated-return"
	byRef.SurfaceBindingRef.ID = "mutated-inspect"
	again, err := repo.InspectExactAck(context.Background(), ack.Ref())
	if err != nil || again != ack {
		t.Fatalf("returned clone mutated Repository: %#v, %v", again, err)
	}
	if len(repo.byAckID) != 1 || len(repo.byPreparedCurrent) != 1 || len(repo.byPreparedRef) != 1 {
		t.Fatalf("index cardinality = %d/%d/%d", len(repo.byAckID), len(repo.byPreparedCurrent), len(repo.byPreparedRef))
	}
}

func TestPreparedModelInvocationAckRepositoryRejectsCanonicalAndEpochDriftV1(t *testing.T) {
	repo := NewInMemoryPreparedModelInvocationAckRepositoryV1()
	winner := preparedAckRepositoryFixtureV1(t, 2_000, 8_000, "surface-binding-1")
	if _, err := repo.EnsureAck(context.Background(), winner); err != nil {
		t.Fatal(err)
	}

	sameCoordinateDifferentContent := preparedAckRepositoryFixtureV1(t, 2_000, 8_000, "surface-binding-2")
	if sameCoordinateDifferentContent.ID != winner.ID || sameCoordinateDifferentContent.Digest == winner.Digest {
		t.Fatal("fixture does not preserve ACK identity while drifting content")
	}
	if _, err := repo.EnsureAck(context.Background(), sameCoordinateDifferentContent); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("same key changed content error = %v", err)
	}

	samePreparedDifferentCurrent := preparedAckRepositoryFixtureV1(t, 2_100, 7_900, "surface-binding-1")
	if samePreparedDifferentCurrent.PreparedRef != winner.PreparedRef || samePreparedDifferentCurrent.CurrentRef == winner.CurrentRef {
		t.Fatal("fixture does not drift only the Current epoch")
	}
	if _, err := repo.EnsureAck(context.Background(), samePreparedDifferentCurrent); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("same Prepared changed Current error = %v", err)
	}
	if _, err := repo.inspectByPreparedCurrent(context.Background(), samePreparedDifferentCurrent.PreparedRef, samePreparedDifferentCurrent.CurrentRef); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("same Prepared changed Current recovery error = %v", err)
	}

	wrongRef := winner.Ref()
	wrongRef.Digest = core.DigestBytes([]byte("another-valid-digest"))
	if err := wrongRef.Validate(); err != nil {
		t.Fatalf("wrong exact Ref must remain intrinsically valid: %v", err)
	}
	if _, err := repo.InspectExactAck(context.Background(), wrongRef); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("same ACK ID changed Ref error = %v", err)
	}
}

func TestPreparedModelInvocationAckRepositoryAuthoritativeAbsentV1(t *testing.T) {
	repo := NewInMemoryPreparedModelInvocationAckRepositoryV1()
	ack := preparedAckRepositoryFixtureV1(t, 2_000, 8_000, "surface-binding-1")

	if _, err := repo.inspectByPreparedCurrent(context.Background(), ack.PreparedRef, ack.CurrentRef); !core.HasCategory(err, core.ErrorNotFound) || !core.HasReason(err, core.ReasonInvalidReference) {
		t.Fatalf("stable-key absence error = %v", err)
	}
	if _, err := repo.InspectExactAck(context.Background(), ack.Ref()); !core.HasCategory(err, core.ErrorNotFound) || !core.HasReason(err, core.ReasonInvalidReference) {
		t.Fatalf("exact Ref absence error = %v", err)
	}
}

func TestPreparedModelInvocationAckRepositoryNilCanceledAndPostLockV1(t *testing.T) {
	ack := preparedAckRepositoryFixtureV1(t, 2_000, 8_000, "surface-binding-1")
	var nilRepo *InMemoryPreparedModelInvocationAckRepositoryV1
	if _, err := nilRepo.EnsureAck(context.Background(), ack); !core.HasCategory(err, core.ErrorUnavailable) || !core.HasReason(err, core.ReasonComponentMissing) {
		t.Fatalf("nil Repository error = %v", err)
	}
	repo := NewInMemoryPreparedModelInvocationAckRepositoryV1()
	if _, err := repo.EnsureAck(nil, ack); !core.HasCategory(err, core.ErrorInvalidArgument) {
		t.Fatalf("nil context error = %v", err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := repo.EnsureAck(canceled, ack); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("canceled context error = %v", err)
	}

	postLockCanceled := &preparedAckRepositoryStepContextV1{cancelAt: 2}
	if _, err := repo.EnsureAck(postLockCanceled, ack); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("post-lock canceled Ensure error = %v", err)
	}
	if len(repo.byAckID) != 0 || len(repo.byPreparedCurrent) != 0 || len(repo.byPreparedRef) != 0 {
		t.Fatal("post-lock cancellation mutated Repository")
	}

	if _, err := repo.EnsureAck(context.Background(), ack); err != nil {
		t.Fatal(err)
	}
	postReadLockCanceled := &preparedAckRepositoryStepContextV1{cancelAt: 2}
	if _, err := repo.InspectExactAck(postReadLockCanceled, ack.Ref()); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("post-lock canceled Inspect error = %v", err)
	}
}

func TestPreparedModelInvocationAckRepositoryConcurrentSameCanonicalV1(t *testing.T) {
	repo := NewInMemoryPreparedModelInvocationAckRepositoryV1()
	ack := preparedAckRepositoryFixtureV1(t, 2_000, 8_000, "surface-binding-1")
	const workers = 64
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := repo.EnsureAck(context.Background(), ack)
			if err == nil && got != ack {
				err = core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "concurrent ACK clone drifted")
			}
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(repo.byAckID) != 1 || len(repo.byPreparedCurrent) != 1 || len(repo.byPreparedRef) != 1 {
		t.Fatalf("concurrent index cardinality = %d/%d/%d", len(repo.byAckID), len(repo.byPreparedCurrent), len(repo.byPreparedRef))
	}
}

type preparedAckRepositoryStepContextV1 struct {
	calls    atomic.Int32
	cancelAt int32
}

func (*preparedAckRepositoryStepContextV1) Deadline() (time.Time, bool) { return time.Time{}, false }
func (*preparedAckRepositoryStepContextV1) Done() <-chan struct{}       { return nil }
func (*preparedAckRepositoryStepContextV1) Value(any) any               { return nil }
func (c *preparedAckRepositoryStepContextV1) Err() error {
	if c.calls.Add(1) >= c.cancelAt {
		return context.Canceled
	}
	return nil
}

func preparedAckRepositoryFixtureV1(t *testing.T, currentChecked, currentExpires int64, surfaceID string) modelinvoker.PreparedModelInvocationCommitAckV1 {
	t.Helper()
	digest := func(label string) core.Digest { return core.DigestBytes([]byte(label)) }
	owner := func(domain, id string) core.OwnerRef { return core.OwnerRef{Domain: domain, ID: core.OwnerID(id)} }
	requestDigest := digest("ack-repository-request")
	fact, err := modelinvoker.SealPreparedModelInvocationFactV1(modelinvoker.PreparedModelInvocationFactV1{
		InvocationID:                  "ack-repository-invocation",
		InvocationDigest:              requestDigest,
		UnifiedRequestDigest:          requestDigest,
		RequestToolsDigest:            digest("request-tools"),
		PreparedPlanDigest:            digest("prepared-plan"),
		RouteDigest:                   digest("route"),
		ProfileDigest:                 digest("profile"),
		ActualToolSurfaceDigest:       digest("tool-surface"),
		ActualProviderInjectionDigest: digest("provider-injection"),
		CapabilitySnapshotRef: modelinvoker.PreparedModelInvocationCapabilitySnapshotRefV1{
			ContractVersion: "capability/v1", ID: "capability-1", Revision: 1, Digest: digest("capability"),
		},
		RegistrySnapshotRef: runtimeports.RegistrySnapshotRefV1{
			Owner: owner("registry", "registry-owner"), ContractVersion: "1.0.0", ID: "registry-1", Revision: 1, Digest: digest("registry"),
		},
		CreatedUnixNano:  1_000,
		NotAfterUnixNano: 10_000,
	})
	if err != nil {
		t.Fatal(err)
	}
	current, err := modelinvoker.SealPreparedModelInvocationCurrentV1(modelinvoker.PreparedModelInvocationCurrentProjectionV1{
		Prepared: fact.Ref(), CapabilitySnapshotRef: fact.CapabilitySnapshotRef, RegistrySnapshotRef: fact.RegistrySnapshotRef,
		ActualToolSurfaceDigest: fact.ActualToolSurfaceDigest, ActualProviderInjectionDigest: fact.ActualProviderInjectionDigest,
		CheckedUnixNano: currentChecked, ExpiresUnixNano: currentExpires, NotAfterUnixNano: fact.NotAfterUnixNano,
	})
	if err != nil {
		t.Fatal(err)
	}
	ack, err := modelinvoker.SealPreparedModelInvocationCommitAckV1(modelinvoker.PreparedModelInvocationCommitAckV1{
		PreparedRef: fact.Ref(), CurrentRef: current.Ref(),
		GateImplementationRef: modelinvoker.PreparedModelInvocationGateImplementationRefV1{
			Owner: owner("harness", "model-gate"), ContractVersion: "gate/v1", ID: "gate-1", Revision: 1, Digest: digest("gate"),
		},
		SurfaceBindingRef: modelinvoker.PreparedModelInvocationSurfaceBindingRefV1{
			Owner: owner("tool", "surface-binding-owner"), ContractVersion: "tool-binding/v1", ID: surfaceID, Revision: 1, Digest: digest(surfaceID),
		},
		CheckedUnixNano: currentChecked + 100, ExpiresUnixNano: currentExpires - 100, NotAfterUnixNano: fact.NotAfterUnixNano,
	})
	if err != nil {
		t.Fatal(err)
	}
	return ack
}
