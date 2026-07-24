package surface

import (
	"context"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
)

func TestToolSurfaceManifestCurrentRepositoryV1Whitebox(t *testing.T) {
	newRepo := func(t *testing.T, clock func() time.Time) *InMemoryToolSurfaceManifestCurrentRepositoryV1 {
		t.Helper()
		repo, err := NewInMemoryToolSurfaceManifestCurrentRepositoryV1(clock)
		if err != nil {
			t.Fatal(err)
		}
		return repo
	}
	ensure := func(t *testing.T, repo *InMemoryToolSurfaceManifestCurrentRepositoryV1, request contract.ToolSurfaceManifestCurrentEnsureRequestV1) contract.ToolSurfaceManifestCurrentProjectionV1 {
		t.Helper()
		projection, err := repo.EnsureExactToolSurfaceManifestCurrentV1(context.Background(), request)
		if err != nil {
			t.Fatal(err)
		}
		return projection
	}

	t.Run("C2-001 initial Ensure creates one history and current", func(t *testing.T) {
		repo := newRepo(t, func() time.Time { return testkit.FixedTime })
		got := ensure(t, repo, testkit.ToolSurfaceManifestCurrentRequestV1(1))
		if len(repo.history) != 1 || len(repo.current) != 1 || got.Manifest.Revision != 1 {
			t.Fatalf("unexpected repository state: history=%d current=%d", len(repo.history), len(repo.current))
		}
	})

	t.Run("C2-002 same canonical returns immutable winner", func(t *testing.T) {
		clock := testkit.NewManualClock(testkit.FixedTime)
		repo := newRepo(t, clock.Now)
		request := testkit.ToolSurfaceManifestCurrentRequestV1(1)
		first := ensure(t, repo, request)
		clock.Set(testkit.FixedTime.Add(time.Second))
		second := ensure(t, repo, request)
		if !reflect.DeepEqual(first, second) || len(repo.history) != 1 {
			t.Fatal("same canonical request did not return the persisted winner")
		}
	})

	t.Run("C2-004 same revision changed body conflicts", func(t *testing.T) {
		repo := newRepo(t, func() time.Time { return testkit.FixedTime })
		request := testkit.ToolSurfaceManifestCurrentRequestV1(1)
		ensure(t, repo, request)
		changed := request
		changed.Manifest = testkit.CloneToolSurfaceManifestV1(request.Manifest)
		changed.Manifest.ProfileDigest = testkit.Digest("changed-profile")
		changed.Manifest.Digest = ""
		var err error
		changed.Manifest, err = contract.SealSurface(changed.Manifest)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = repo.EnsureExactToolSurfaceManifestCurrentV1(context.Background(), changed); !core.HasCategory(err, core.ErrorConflict) || len(repo.history) != 1 {
			t.Fatalf("changed body was not a zero-write conflict: %v", err)
		}
	})

	t.Run("C2-010 corrupted current index fails exact", func(t *testing.T) {
		repo := newRepo(t, func() time.Time { return testkit.FixedTime })
		winner := ensure(t, repo, testkit.ToolSurfaceManifestCurrentRequestV1(1))
		wrong := winner.Ref
		wrong.Digest = testkit.Digest("wrong-current")
		repo.current[winner.Ref.ID] = wrong
		if _, err := repo.InspectExactToolSurfaceManifestCurrentV1(context.Background(), winner.Ref); err == nil {
			t.Fatal("corrupted current index passed exact inspection")
		}
	})

	t.Run("C2-011 expired Manifest is rejected", func(t *testing.T) {
		request := testkit.ToolSurfaceManifestCurrentRequestV1(1)
		repo := newRepo(t, func() time.Time { return time.Unix(0, request.Manifest.ExpiresUnixNano) })
		if _, err := repo.EnsureExactToolSurfaceManifestCurrentV1(context.Background(), request); !core.HasReason(err, core.ReasonBindingExpired) || len(repo.history) != 0 {
			t.Fatalf("expired Manifest was not rejected: %v", err)
		}
	})

	t.Run("C2-012 clock rollback rejects existing winner", func(t *testing.T) {
		clock := testkit.NewManualClock(testkit.FixedTime)
		repo := newRepo(t, clock.Now)
		winner := ensure(t, repo, testkit.ToolSurfaceManifestCurrentRequestV1(1))
		clock.Set(testkit.FixedTime.Add(-time.Second))
		if _, err := repo.InspectExactToolSurfaceManifestCurrentV1(context.Background(), winner.Ref); !core.HasReason(err, core.ReasonClockRegression) {
			t.Fatalf("clock rollback was not rejected: %v", err)
		}
	})

	t.Run("C2-014 nil context is rejected before state", func(t *testing.T) {
		repo := newRepo(t, func() time.Time { return testkit.FixedTime })
		if _, err := repo.EnsureExactToolSurfaceManifestCurrentV1(nil, testkit.ToolSurfaceManifestCurrentRequestV1(1)); !core.HasCategory(err, core.ErrorInvalidArgument) || len(repo.history) != 0 {
			t.Fatalf("nil context was not a zero-write invalid argument: %v", err)
		}
	})

	t.Run("C2-017 nil clock and nil receiver fail closed", func(t *testing.T) {
		var clock func() time.Time
		if _, err := NewInMemoryToolSurfaceManifestCurrentRepositoryV1(clock); err == nil {
			t.Fatal("nil clock passed constructor")
		}
		var repo *InMemoryToolSurfaceManifestCurrentRepositoryV1
		if _, err := repo.EnsureExactToolSurfaceManifestCurrentV1(context.Background(), testkit.ToolSurfaceManifestCurrentRequestV1(1)); !core.HasCategory(err, core.ErrorUnavailable) {
			t.Fatalf("nil Repository did not fail unavailable: %v", err)
		}
	})

	t.Run("C2-024 same revision digest ABA conflicts", func(t *testing.T) {
		repo := newRepo(t, func() time.Time { return testkit.FixedTime })
		request := testkit.ToolSurfaceManifestCurrentRequestV1(1)
		winner := ensure(t, repo, request)
		changed := request
		changed.Manifest.Digest = testkit.Digest("aba")
		if _, err := repo.EnsureExactToolSurfaceManifestCurrentV1(context.Background(), changed); err == nil || repo.current[winner.Ref.ID] != winner.Ref {
			t.Fatal("same revision digest ABA changed current")
		}
	})

	t.Run("C2-025 request is deep cloned", func(t *testing.T) {
		repo := newRepo(t, func() time.Time { return testkit.FixedTime })
		request := testkit.ToolSurfaceManifestCurrentRequestV1(1)
		winner := ensure(t, repo, request)
		request.Manifest.Entries[0].EffectKinds[0] = "evil/effect"
		got, err := repo.InspectExactToolSurfaceManifestCurrentV1(context.Background(), winner.Ref)
		if err != nil || got.Manifest.Entries[0].EffectKinds[0] == "evil/effect" {
			t.Fatal("caller mutation changed persisted Manifest")
		}
	})

	t.Run("C2-026 returned values are deep cloned", func(t *testing.T) {
		repo := newRepo(t, func() time.Time { return testkit.FixedTime })
		winner := ensure(t, repo, testkit.ToolSurfaceManifestCurrentRequestV1(1))
		winner.Manifest.Entries[0].EffectKinds[0] = "evil/effect"
		got, err := repo.InspectExactToolSurfaceManifestCurrentV1(context.Background(), winner.Ref)
		if err != nil || got.Manifest.Entries[0].EffectKinds[0] == "evil/effect" {
			t.Fatal("returned slice aliases repository state")
		}
	})

	t.Run("C2-033 concurrent same canonical has one winner", func(t *testing.T) {
		repo := newRepo(t, func() time.Time { return testkit.FixedTime })
		request := testkit.ToolSurfaceManifestCurrentRequestV1(1)
		var wg sync.WaitGroup
		results := make(chan contract.ToolSurfaceManifestCurrentProjectionV1, 64)
		errs := make(chan error, 64)
		for i := 0; i < 64; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				got, err := repo.EnsureExactToolSurfaceManifestCurrentV1(context.Background(), request)
				results <- got
				errs <- err
			}()
		}
		wg.Wait()
		close(results)
		close(errs)
		var ref contract.ToolSurfaceManifestCurrentRefV1
		for err := range errs {
			if err != nil {
				t.Fatal(err)
			}
		}
		for got := range results {
			if ref == (contract.ToolSurfaceManifestCurrentRefV1{}) {
				ref = got.Ref
			} else if got.Ref != ref {
				t.Fatal("same canonical produced multiple winners")
			}
		}
		if len(repo.history) != 1 || len(repo.current) != 1 {
			t.Fatal("same canonical wrote more than one fact")
		}
	})

	t.Run("C2-034 concurrent changed content has one winner", func(t *testing.T) {
		repo := newRepo(t, func() time.Time { return testkit.FixedTime })
		base := testkit.ToolSurfaceManifestCurrentRequestV1(1)
		var wg sync.WaitGroup
		var successes atomic.Int64
		for i := 0; i < 64; i++ {
			request := base
			request.Manifest = testkit.CloneToolSurfaceManifestV1(base.Manifest)
			request.Manifest.ProfileDigest = testkit.Digest(string(rune('a' + i)))
			request.Manifest.Digest = ""
			request.Manifest, _ = contract.SealSurface(request.Manifest)
			wg.Add(1)
			go func() {
				defer wg.Done()
				if _, err := repo.EnsureExactToolSurfaceManifestCurrentV1(context.Background(), request); err == nil {
					successes.Add(1)
				}
			}()
		}
		wg.Wait()
		if successes.Load() != 1 || len(repo.history) != 1 {
			t.Fatalf("changed-content race successes=%d history=%d", successes.Load(), len(repo.history))
		}
	})

	t.Run("C2-035 different IDs reach owner clock concurrently", func(t *testing.T) {
		var active atomic.Int64
		var peak atomic.Int64
		release := make(chan struct{})
		clock := func() time.Time {
			value := active.Add(1)
			for {
				seen := peak.Load()
				if value <= seen || peak.CompareAndSwap(seen, value) {
					break
				}
			}
			if value >= 2 {
				select {
				case <-release:
				default:
					close(release)
				}
			}
			select {
			case <-release:
			case <-time.After(time.Second):
			}
			active.Add(-1)
			return testkit.FixedTime
		}
		repo := newRepo(t, clock)
		left := testkit.ToolSurfaceManifestCurrentRequestV1(1)
		right := testkit.ToolSurfaceManifestCurrentRequestV1(1)
		right.Manifest.ID = "surface_other"
		right.Manifest.Digest = ""
		right.Manifest, _ = contract.SealSurface(right.Manifest)
		var wg sync.WaitGroup
		for _, request := range []contract.ToolSurfaceManifestCurrentEnsureRequestV1{left, right} {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, _ = repo.EnsureExactToolSurfaceManifestCurrentV1(context.Background(), request)
			}()
		}
		wg.Wait()
		if peak.Load() < 2 || len(repo.current) != 2 {
			t.Fatalf("different IDs were globally serialized: peak=%d current=%d", peak.Load(), len(repo.current))
		}
	})

	t.Run("C2-037 successor full exact CAS", func(t *testing.T) {
		clock := testkit.NewManualClock(testkit.FixedTime)
		repo := newRepo(t, clock.Now)
		first := ensure(t, repo, testkit.ToolSurfaceManifestCurrentRequestV1(1))
		clock.Set(testkit.FixedTime.Add(time.Second))
		second := ensure(t, repo, testkit.ToolSurfaceManifestSuccessorRequestV1(first))
		if second.Ref.Revision != 2 || len(repo.history) != 2 || repo.current[first.Ref.ID] != second.Ref {
			t.Fatal("successor CAS did not advance current exactly once")
		}
	})

	t.Run("C2-038 revision jump is rejected", func(t *testing.T) {
		repo := newRepo(t, func() time.Time { return testkit.FixedTime })
		first := ensure(t, repo, testkit.ToolSurfaceManifestCurrentRequestV1(1))
		request := testkit.ToolSurfaceManifestSuccessorRequestV1(first)
		request.Manifest.Revision = 3
		request.Manifest.Digest = ""
		request.Manifest, _ = contract.SealSurface(request.Manifest)
		if _, err := repo.EnsureExactToolSurfaceManifestCurrentV1(context.Background(), request); err == nil || len(repo.history) != 1 {
			t.Fatal("revision jump committed")
		}
	})

	t.Run("C2-040 zero clock fails closed", func(t *testing.T) {
		repo := newRepo(t, func() time.Time { return time.Time{} })
		if _, err := repo.EnsureExactToolSurfaceManifestCurrentV1(context.Background(), testkit.ToolSurfaceManifestCurrentRequestV1(1)); !core.HasReason(err, core.ReasonClockRegression) || len(repo.history) != 0 {
			t.Fatalf("zero clock did not fail closed: %v", err)
		}
	})

	t.Run("C2-048 internal slices never escape", func(t *testing.T) {
		repo := newRepo(t, func() time.Time { return testkit.FixedTime })
		winner := ensure(t, repo, testkit.ToolSurfaceManifestCurrentRequestV1(1))
		got, _ := repo.InspectExactToolSurfaceManifestCurrentV1(context.Background(), winner.Ref)
		got.Manifest.Residuals[0].Detail = "tampered"
		again, _ := repo.InspectExactToolSurfaceManifestCurrentV1(context.Background(), winner.Ref)
		if again.Manifest.Residuals[0].Detail == "tampered" {
			t.Fatal("Residual slice aliases repository state")
		}
	})

	t.Run("C2-050 same Manifest ID across Owner conflicts", func(t *testing.T) {
		repo := newRepo(t, func() time.Time { return testkit.FixedTime })
		request := testkit.ToolSurfaceManifestCurrentRequestV1(1)
		ensure(t, repo, request)
		changed := request
		changed.Manifest = testkit.CloneToolSurfaceManifestV1(request.Manifest)
		changed.Manifest.Owner.ID = "other-owner"
		changed.Manifest.Digest = ""
		changed.Manifest, _ = contract.SealSurface(changed.Manifest)
		if _, err := repo.EnsureExactToolSurfaceManifestCurrentV1(context.Background(), changed); !core.HasCategory(err, core.ErrorConflict) || len(repo.history) != 1 {
			t.Fatalf("cross-owner Manifest ID did not conflict: %v", err)
		}
	})

	t.Run("C2-052 rev1 retry after rev2 cannot roll current back", func(t *testing.T) {
		clock := testkit.NewManualClock(testkit.FixedTime)
		repo := newRepo(t, clock.Now)
		request := testkit.ToolSurfaceManifestCurrentRequestV1(1)
		first := ensure(t, repo, request)
		clock.Set(testkit.FixedTime.Add(time.Second))
		second := ensure(t, repo, testkit.ToolSurfaceManifestSuccessorRequestV1(first))
		if _, err := repo.EnsureExactToolSurfaceManifestCurrentV1(context.Background(), request); err == nil || repo.current[first.Ref.ID] != second.Ref {
			t.Fatal("historical rev1 was returned after rev2 became current")
		}
	})

	t.Run("C2-053 expected-current ABA fails successor CAS", func(t *testing.T) {
		clock := testkit.NewManualClock(testkit.FixedTime)
		repo := newRepo(t, clock.Now)
		first := ensure(t, repo, testkit.ToolSurfaceManifestCurrentRequestV1(1))
		request := testkit.ToolSurfaceManifestSuccessorRequestV1(first)
		request.ExpectedCurrent.Digest = testkit.Digest("aba-current")
		if _, err := repo.EnsureExactToolSurfaceManifestCurrentV1(context.Background(), request); err == nil || len(repo.history) != 1 {
			t.Fatal("wrong full ExpectedCurrent committed successor")
		}
	})
}
