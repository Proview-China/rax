package kernel

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	sandboxports "github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

type workspaceReadCommandExactStoreV1 struct {
	sandboxports.WorkspaceReadOwnerStoreV1
	command contract.WorkspaceReadCommandV1
	err     error
	reads   atomic.Uint64
}

func (s *workspaceReadCommandExactStoreV1) InspectWorkspaceReadCommandExactV1(
	context.Context,
	contract.Ref,
) (contract.WorkspaceReadCommandV1, error) {
	s.reads.Add(1)
	return s.command, s.err
}

func TestWorkspaceReadCommandExactReaderV1SixtyFourConcurrentReadsNeverReachPhysicalPoint(t *testing.T) {
	store := &workspaceReadCommandExactStoreV1{}
	actualPoint := &countingWorkspaceReadActualPointV2{}
	executor := &WorkspaceReadPhysicalExecutorV1{store: store, actualPoint: actualPoint}
	exact := contract.Ref{
		ID: "command", Revision: 1,
		Digest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}

	const readers = 64
	var group sync.WaitGroup
	failures := make(chan error, readers)
	for range readers {
		group.Add(1)
		go func() {
			defer group.Done()
			_, err := executor.InspectWorkspaceReadCommandExactV1(context.Background(), exact)
			failures <- err
		}()
	}
	group.Wait()
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	if got := store.reads.Load(); got != readers {
		t.Fatalf("exact Command reads=%d, want %d", got, readers)
	}
	if got := actualPoint.calls.Load(); got != 0 {
		t.Fatalf("historical exact Command read reached physical point %d times", got)
	}
}

func TestWorkspaceReadCommandExactReaderV1ExecutorFailsClosedWhenUnavailable(t *testing.T) {
	exact := contract.Ref{
		ID: "command", Revision: 1,
		Digest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}
	actualPoint := &countingWorkspaceReadActualPointV2{}
	for name, store := range map[string]sandboxports.WorkspaceReadOwnerStoreV1{
		"missing exact reader": &workspaceReadInspectionStoreV2{},
		"typed nil reader":     (*workspaceReadCommandExactStoreV1)(nil),
	} {
		t.Run(name, func(t *testing.T) {
			executor := &WorkspaceReadPhysicalExecutorV1{store: store, actualPoint: actualPoint}
			if _, err := executor.InspectWorkspaceReadCommandExactV1(context.Background(), exact); err == nil {
				t.Fatal("unavailable exact Command reader was accepted")
			}
		})
	}
	if got := actualPoint.calls.Load(); got != 0 {
		t.Fatalf("unavailable historical reader reached physical point %d times", got)
	}
}

func TestWorkspaceReadCommandExactReaderV1ExecutorPreservesReaderFailure(t *testing.T) {
	expected := errors.New("historical store unavailable")
	store := &workspaceReadCommandExactStoreV1{err: expected}
	executor := &WorkspaceReadPhysicalExecutorV1{
		store: store, actualPoint: &countingWorkspaceReadActualPointV2{},
	}
	_, err := executor.InspectWorkspaceReadCommandExactV1(
		context.Background(),
		contract.Ref{
			ID: "command", Revision: 1,
			Digest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		},
	)
	if !errors.Is(err, expected) {
		t.Fatalf("reader failure was not preserved: %v", err)
	}
}
