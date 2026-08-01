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

type workspaceReadInspectionStoreV2 struct {
	sandboxports.WorkspaceReadOwnerStoreV1
	inspectCalls atomic.Uint64
}

func (s *workspaceReadInspectionStoreV2) InspectBoundedWorkspaceReadV2(
	context.Context,
	contract.WorkspaceReadAttemptRefV1,
) (sandboxports.WorkspaceReadInspectionEnvelopeV2, error) {
	s.inspectCalls.Add(1)
	return sandboxports.WorkspaceReadInspectionEnvelopeV2{}, nil
}

type countingWorkspaceReadActualPointV2 struct {
	calls atomic.Uint64
}

func (p *countingWorkspaceReadActualPointV2) ReadWorkspaceFileV1(
	context.Context,
	WorkspaceReadActualPointRequestV1,
) (WorkspaceReadActualPointResultV1, error) {
	p.calls.Add(1)
	return WorkspaceReadActualPointResultV1{}, nil
}

func (p *countingWorkspaceReadActualPointV2) PrepareWorkspaceReadV2(context.Context, WorkspaceReadActualPointRequestV1) (WorkspaceReadActualPointPreparationV2, error) {
	return nil, errors.New("unexpected physical prepare")
}

func (p *countingWorkspaceReadActualPointV2) DispatchPreparedWorkspaceReadV2(context.Context, WorkspaceReadActualPointPreparationV2) (WorkspaceReadActualPointResultV1, error) {
	p.calls.Add(1)
	return WorkspaceReadActualPointResultV1{}, errors.New("unexpected physical dispatch")
}

func (p *countingWorkspaceReadActualPointV2) InspectWorkspaceReadJournalV2(context.Context, contract.WorkspaceReadExecutionQualificationV2) (WorkspaceReadActualPointInspectionV2, error) {
	return WorkspaceReadActualPointInspectionV2{}, ErrWorkspaceReadPhysicalJournalNotFoundV2
}

func TestWorkspaceReadInspectionV2SixtyFourConcurrentReadsNeverReachPhysicalPoint(t *testing.T) {
	store := &workspaceReadInspectionStoreV2{}
	actualPoint := &countingWorkspaceReadActualPointV2{}
	executor := &WorkspaceReadPhysicalExecutorV1{
		store:       store,
		actualPoint: actualPoint,
	}
	origin := contract.WorkspaceReadAttemptRefV1{
		ID:       "origin",
		Revision: 1,
		Digest:   "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}

	const readers = 64
	var group sync.WaitGroup
	failures := make(chan error, readers)
	for range readers {
		group.Add(1)
		go func() {
			defer group.Done()
			_, err := executor.InspectBoundedWorkspaceReadV2(context.Background(), origin)
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
	if got := store.inspectCalls.Load(); got != readers {
		t.Fatalf("inspection calls=%d, want %d", got, readers)
	}
	if got := actualPoint.calls.Load(); got != 0 {
		t.Fatalf("read-only exact Inspect reached physical read %d times", got)
	}
}

func TestWorkspaceReadInspectionV2RejectsTypedNilReader(t *testing.T) {
	var store *workspaceReadInspectionStoreV2
	executor := &WorkspaceReadPhysicalExecutorV1{store: store}
	_, err := executor.InspectBoundedWorkspaceReadV2(
		context.Background(),
		contract.WorkspaceReadAttemptRefV1{
			ID:       "origin",
			Revision: 1,
			Digest:   "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		},
	)
	if err == nil {
		t.Fatal("typed-nil inspection reader was accepted")
	}
}
