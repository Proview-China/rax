package hostroot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/api"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/storage/sqlite"
)

func TestProductionHostV1ServesAPIAndCurrentPlaneAsOneLifecycle(t *testing.T) {
	ctx := context.Background()
	store, err := sqlite.OpenWithClock(ctx, filepath.Join(t.TempDir(), "sandbox.db"), func() time.Time { return testkit.FixedNow })
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	surfaces := &hostSurfacesV1{}
	current := &currentPlaneStubV1{path: filepath.Join(t.TempDir(), "current.sock"), started: make(chan struct{})}
	root, err := NewProductionHostV1(ProductionHostConfigV1{
		Backends: []contract.BackendDescriptor{testkit.Backend()}, Facts: surfaces, Lifecycle: surfaces, WorkspaceCapture: surfaces,
		Checkpoint: surfaces, SnapshotArtifact: surfaces, WorkspaceRestore: surfaces, WorkspaceRewind: surfaces, Operations: store, Authorization: transportAuthorizationStubV1{}, CurrentPlane: current,
		Clock: func() time.Time { return testkit.FixedNow }, ReadHeaderTimeout: time.Second, IdleTimeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	serveCtx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- root.Serve(serveCtx, listener) }()
	select {
	case <-current.started:
	case <-time.After(2 * time.Second):
		t.Fatal("current plane did not start")
	}
	for _, path := range []string{"/livez", "/readyz"} {
		response, requestErr := http.Get("http://" + listener.Addr().String() + path)
		if requestErr != nil {
			t.Fatal(requestErr)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("%s status = %s", path, response.Status)
		}
	}
	if !root.Ready() {
		t.Fatal("host root did not publish readiness after current-plane bind")
	}
	request, err := api.SealOperationRequestV1(api.OperationRequestV1{RequestID: "host-root-request", IdempotencyKey: "host-root-key", TenantID: "tenant-1", Action: api.ActionDescribeBackendsV1, PayloadSchema: "praxis.sandbox.api/describe-backends/v1", PayloadRevision: 1, Payload: json.RawMessage(`{}`), RequestedUnixNano: testkit.FixedNow.UnixNano(), RequestedNotAfterUnixNano: testkit.FixedNow.Add(time.Hour).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(request)
	response, err := http.Post("http://"+listener.Addr().String()+"/v1/operations", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("API submit status = %s", response.Status)
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("host root shutdown = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("host root did not shut down")
	}
	if root.Ready() {
		t.Fatal("host root remained ready after shutdown")
	}
}

func TestProductionHostV1RejectsIncompleteProductionSurfaces(t *testing.T) {
	var checkpoint *hostSurfacesV1
	_, err := NewProductionHostV1(ProductionHostConfigV1{Backends: []contract.BackendDescriptor{testkit.Backend()}, Facts: &hostSurfacesV1{}, Lifecycle: &hostSurfacesV1{}, WorkspaceCapture: &hostSurfacesV1{}, Checkpoint: checkpoint, Clock: func() time.Time { return testkit.FixedNow }})
	if err == nil {
		t.Fatal("typed-nil production checkpoint surface was accepted")
	}
}

type currentPlaneStubV1 struct {
	path    string
	started chan struct{}
}

func (s *currentPlaneStubV1) Listen() (*net.UnixListener, error) {
	_ = os.Remove(s.path)
	return net.ListenUnix("unix", &net.UnixAddr{Name: s.path, Net: "unix"})
}

func (s *currentPlaneStubV1) Serve(ctx context.Context, listener *net.UnixListener) error {
	close(s.started)
	<-ctx.Done()
	_ = listener.Close()
	return ctx.Err()
}

type transportAuthorizationStubV1 struct{}

func (transportAuthorizationStubV1) AuthorizeSandboxAPI(*http.Request, string, api.ActionV1) error {
	return nil
}

type hostSurfacesV1 struct{}

func (*hostSurfacesV1) GetReservation(context.Context, string) (contract.DomainReservation, error) {
	return contract.DomainReservation{}, ports.ErrNotFound
}
func (*hostSurfacesV1) GetInspection(context.Context, string) (contract.InspectionFact, error) {
	return contract.InspectionFact{}, ports.ErrNotFound
}
func (*hostSurfacesV1) GetDomainResult(context.Context, string) (contract.SandboxDomainResultFact, error) {
	return contract.SandboxDomainResultFact{}, ports.ErrNotFound
}
func (*hostSurfacesV1) GetProjection(context.Context, string) (contract.EnvironmentProjection, error) {
	return contract.EnvironmentProjection{}, ports.ErrNotFound
}
func (*hostSurfacesV1) StartOrInspectSandboxLifecycleV4(context.Context, applicationcontract.SandboxLifecycleRequestV4) (applicationcontract.SandboxLifecycleResultV4, error) {
	return applicationcontract.SandboxLifecycleResultV4{}, ports.ErrUnsupported
}
func (*hostSurfacesV1) InspectSandboxLifecycleV4(context.Context, applicationcontract.SandboxLifecycleRequestV4) (applicationcontract.SandboxLifecycleResultV4, error) {
	return applicationcontract.SandboxLifecycleResultV4{}, ports.ErrUnsupported
}
func (*hostSurfacesV1) CaptureWorkspaceChangeSetV1(context.Context, ports.CaptureWorkspaceChangeSetRequest) (contract.WorkspaceChangeSet, error) {
	return contract.WorkspaceChangeSet{}, ports.ErrUnsupported
}
func (*hostSurfacesV1) ComposeWorkspaceRewindV1(context.Context, contract.ComposeWorkspaceRewindRequestV1) (ports.WorkspaceRewindCompositionResultV1, error) {
	return ports.WorkspaceRewindCompositionResultV1{}, ports.ErrUnsupported
}
func (*hostSurfacesV1) InspectWorkspaceRewindV1(context.Context, contract.ComposeWorkspaceRewindRequestV1) (ports.WorkspaceRewindCompositionResultV1, error) {
	return ports.WorkspaceRewindCompositionResultV1{}, ports.ErrUnsupported
}
func (*hostSurfacesV1) CompleteCheckpointParticipantV1(context.Context, applicationcontract.CheckpointParticipantWorkRequestV1) (applicationcontract.CheckpointParticipantCommitV1, error) {
	return applicationcontract.CheckpointParticipantCommitV1{}, ports.ErrUnsupported
}
func (*hostSurfacesV1) InspectCheckpointParticipantV1(context.Context, runtimeports.CheckpointAttemptRefV2, runtimeports.CheckpointParticipantRefV2) (applicationcontract.CheckpointParticipantCommitV1, error) {
	return applicationcontract.CheckpointParticipantCommitV1{}, ports.ErrUnsupported
}
func (*hostSurfacesV1) ReserveArtifact(context.Context, *contract.ReserveArtifactRequestV2) (contract.ReserveArtifactResultV2, error) {
	return contract.ReserveArtifactResultV2{}, ports.ErrUnsupported
}
func (*hostSurfacesV1) CommitArtifact(context.Context, *contract.CommitSnapshotArtifactRequestV2) (contract.CommitSnapshotArtifactResultV2, error) {
	return contract.CommitSnapshotArtifactResultV2{}, ports.ErrUnsupported
}
func (*hostSurfacesV1) InspectReservation(context.Context, *contract.InspectSnapshotArtifactReservationRequestV2) (contract.SnapshotArtifactReservationV2, error) {
	return contract.SnapshotArtifactReservationV2{}, ports.ErrUnsupported
}
func (*hostSurfacesV1) InspectReservationByStableKey(context.Context, *contract.InspectSnapshotArtifactReservationByStableKeyRequestV2) (contract.SnapshotArtifactReservationV2, error) {
	return contract.SnapshotArtifactReservationV2{}, ports.ErrUnsupported
}
func (*hostSurfacesV1) InspectAggregateHistorical(context.Context, *contract.InspectSnapshotArtifactAggregateHistoricalRequestV2) (contract.SnapshotArtifactAggregateEnvelopeV2, error) {
	return contract.SnapshotArtifactAggregateEnvelopeV2{}, ports.ErrUnsupported
}
func (*hostSurfacesV1) InspectAggregateCurrent(context.Context, *contract.InspectSnapshotArtifactAggregateCurrentRequestV2) (contract.SnapshotArtifactAggregateCurrentProjectionV2, error) {
	return contract.SnapshotArtifactAggregateCurrentProjectionV2{}, ports.ErrUnsupported
}
func (*hostSurfacesV1) InspectEntryHistorical(context.Context, *contract.InspectSnapshotArtifactEntryHistoricalRequestV2) (contract.SnapshotArtifactAggregateEntryV2, error) {
	return contract.SnapshotArtifactAggregateEntryV2{}, ports.ErrUnsupported
}
func (*hostSurfacesV1) InspectArtifactFact(context.Context, *contract.InspectSnapshotArtifactFactRequestV2) (contract.SnapshotArtifactFactV2, error) {
	return contract.SnapshotArtifactFactV2{}, ports.ErrUnsupported
}
func (*hostSurfacesV1) PrepareWorkspaceV1(context.Context, *contract.WorkspaceRestoreStageRequestV1) (contract.WorkspaceRestoreAttemptV1, error) {
	return contract.WorkspaceRestoreAttemptV1{}, ports.ErrUnsupported
}
func (*hostSurfacesV1) StageWorkspaceV1(context.Context, *contract.WorkspaceRestoreStageRequestV1) (contract.WorkspaceRestoreStageFactV1, error) {
	return contract.WorkspaceRestoreStageFactV1{}, ports.ErrUnsupported
}
func (*hostSurfacesV1) ReconcileWorkspaceV1(context.Context, *contract.WorkspaceRestoreStageRequestV1) (contract.WorkspaceRestoreStageFactV1, error) {
	return contract.WorkspaceRestoreStageFactV1{}, ports.ErrUnsupported
}
func (*hostSurfacesV1) InspectWorkspaceRestoreAttemptV1(context.Context, *contract.SnapshotArtifactExactRefV2) (contract.WorkspaceRestoreAttemptV1, error) {
	return contract.WorkspaceRestoreAttemptV1{}, ports.ErrUnsupported
}
func (*hostSurfacesV1) InspectWorkspaceRestoreStageFactV1(context.Context, *contract.SnapshotArtifactExactRefV2) (contract.WorkspaceRestoreStageFactV1, error) {
	return contract.WorkspaceRestoreStageFactV1{}, ports.ErrUnsupported
}
