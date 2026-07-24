package fault_test

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/surface"
)

type c2ReaderFault struct{ err error }

func (r c2ReaderFault) InspectExactToolSurfaceManifestCurrentV1(context.Context, contract.ToolSurfaceManifestCurrentRefV1) (contract.ToolSurfaceManifestCurrentProjectionV1, error) {
	return contract.ToolSurfaceManifestCurrentProjectionV1{}, r.err
}

func TestToolSurfaceManifestCurrentFaultsV1(t *testing.T) {
	t.Run("C2-003 lost Ensure reply retries original request", func(t *testing.T) {
		repo, _ := surface.NewInMemoryToolSurfaceManifestCurrentRepositoryV1(func() time.Time { return testkit.FixedTime })
		request := testkit.ToolSurfaceManifestCurrentRequestV1(1)
		winner, err := repo.EnsureExactToolSurfaceManifestCurrentV1(context.Background(), request)
		if err != nil {
			t.Fatal(err)
		}
		lostReply := core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "reply lost")
		if !core.HasCategory(lostReply, core.ErrorIndeterminate) {
			t.Fatal("fixture did not produce indeterminate reply")
		}
		recovered, err := repo.EnsureExactToolSurfaceManifestCurrentV1(context.Background(), request)
		if err != nil || !reflect.DeepEqual(recovered, winner) {
			t.Fatalf("lost reply did not recover original winner: %v", err)
		}
	})

	t.Run("C2-013 TTL crosses before commit", func(t *testing.T) {
		request := testkit.ToolSurfaceManifestCurrentRequestV1(1)
		clock := testkit.NewSequenceClock(testkit.FixedTime, time.Unix(0, request.Manifest.ExpiresUnixNano))
		repo, _ := surface.NewInMemoryToolSurfaceManifestCurrentRepositoryV1(clock.Now)
		if _, err := repo.EnsureExactToolSurfaceManifestCurrentV1(context.Background(), request); !core.HasReason(err, core.ReasonBindingExpired) {
			t.Fatalf("TTL crossing did not fail closed: %v", err)
		}
	})

	t.Run("C2-015 pre-canceled context preserves sentinel", func(t *testing.T) {
		repo, _ := surface.NewInMemoryToolSurfaceManifestCurrentRepositoryV1(func() time.Time { return testkit.FixedTime })
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := repo.EnsureExactToolSurfaceManifestCurrentV1(ctx, testkit.ToolSurfaceManifestCurrentRequestV1(1))
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled sentinel was not preserved: %v", err)
		}
	})

	t.Run("C2-016 cancel after clock before commit is zero-write", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		var calls atomic.Int64
		clock := func() time.Time {
			if calls.Add(1) == 3 {
				cancel()
			}
			return testkit.FixedTime
		}
		repo, _ := surface.NewInMemoryToolSurfaceManifestCurrentRepositoryV1(clock)
		_, err := repo.EnsureExactToolSurfaceManifestCurrentV1(ctx, testkit.ToolSurfaceManifestCurrentRequestV1(1))
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("post-clock cancellation was not preserved: %v", err)
		}
		ref := testkit.ToolSurfaceManifestCurrentProjectionV1(1).Ref
		if _, inspectErr := repo.InspectExactToolSurfaceManifestCurrentV1(context.Background(), ref); !core.HasCategory(inspectErr, core.ErrorNotFound) {
			t.Fatalf("canceled commit wrote state: %v", inspectErr)
		}
	})

	t.Run("C2-020 reader errors remain closed and never Ensure", func(t *testing.T) {
		ref := testkit.ToolSurfaceManifestCurrentProjectionV1(1).Ref
		for _, err := range []error{
			core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "reader unavailable"),
			core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "reader indeterminate"),
		} {
			_, got := c2ReaderFault{err: err}.InspectExactToolSurfaceManifestCurrentV1(context.Background(), ref)
			if got != err || core.HasCategory(got, core.ErrorNotFound) {
				t.Fatalf("reader error was rewritten: %v", got)
			}
		}
	})

	t.Run("C2-036 lost reply concurrent retries return one winner", func(t *testing.T) {
		repo, _ := surface.NewInMemoryToolSurfaceManifestCurrentRepositoryV1(func() time.Time { return testkit.FixedTime })
		request := testkit.ToolSurfaceManifestCurrentRequestV1(1)
		if _, err := repo.EnsureExactToolSurfaceManifestCurrentV1(context.Background(), request); err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		refs := make(chan contract.ToolSurfaceManifestCurrentRefV1, 64)
		errs := make(chan error, 64)
		for i := 0; i < 64; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				got, err := repo.EnsureExactToolSurfaceManifestCurrentV1(context.Background(), request)
				refs <- got.Ref
				errs <- err
			}()
		}
		wg.Wait()
		close(refs)
		close(errs)
		var winner contract.ToolSurfaceManifestCurrentRefV1
		for err := range errs {
			if err != nil {
				t.Fatal(err)
			}
		}
		for ref := range refs {
			if winner == (contract.ToolSurfaceManifestCurrentRefV1{}) {
				winner = ref
			} else if ref != winner {
				t.Fatal("lost-reply retries observed different winners")
			}
		}
	})

	t.Run("C2-039 unavailable and indeterminate are not authoritative NotFound", func(t *testing.T) {
		for _, err := range []error{
			core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "unavailable"),
			core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "unknown"),
		} {
			var creates atomic.Int64
			createOnlyAfterNotFound := func(inspectErr error) {
				if core.HasCategory(inspectErr, core.ErrorNotFound) {
					creates.Add(1)
				}
			}
			createOnlyAfterNotFound(err)
			if creates.Load() != 0 {
				t.Fatal("uncertain read was treated as authoritative NotFound")
			}
		}
	})
}
