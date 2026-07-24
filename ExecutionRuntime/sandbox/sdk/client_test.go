package sdk

import (
	"context"
	"errors"
	"testing"
	"time"

	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

func TestSDKDescribesCopiesAndMatchesWithoutProviderAccess(t *testing.T) {
	store := testkit.NewMemoryStore()
	lifecycle := &lifecycleStub{}
	capture := &captureStub{}
	client, err := New(Config{Backends: []contract.BackendDescriptor{testkit.Backend()}, Facts: store, Lifecycle: lifecycle, WorkspaceCapture: capture, Clock: func() time.Time { return testkit.FixedNow }})
	if err != nil {
		t.Fatal(err)
	}
	backends, err := client.DescribeBackends()
	if err != nil {
		t.Fatal(err)
	}
	backends[0].Capabilities[contract.CapabilityProcessFence] = contract.CapabilityUnsupported
	again, err := client.DescribeBackends()
	if err != nil {
		t.Fatal(err)
	}
	if again[0].Capabilities[contract.CapabilityProcessFence] != contract.CapabilityEnforced {
		t.Fatal("SDK leaked a mutable backend alias")
	}

	result, err := client.MatchRequirement(testkit.Requirement(), testkit.Policy(), []PlacementInput{{Backend: testkit.Backend(), Candidate: testkit.Candidate()}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 || !result[0].Decision.Admitted {
		t.Fatalf("placement result = %#v", result)
	}
}

func TestSDKOnlyDelegatesGovernedLifecycleAndStagedCapture(t *testing.T) {
	store := testkit.NewMemoryStore()
	lifecycle := &lifecycleStub{}
	capture := &captureStub{set: workspaceSet(t)}
	client, err := New(Config{Backends: []contract.BackendDescriptor{testkit.Backend()}, Facts: store, Lifecycle: lifecycle, WorkspaceCapture: capture, Clock: func() time.Time { return testkit.FixedNow }})
	if err != nil {
		t.Fatal(err)
	}
	request := applicationcontract.SandboxLifecycleRequestV4{ID: "request"}
	if _, err := client.StartLifecycle(context.Background(), request); !errors.Is(err, errLifecycleSentinel) {
		t.Fatalf("lifecycle did not delegate to public Application Port: %v", err)
	}
	set, err := client.CaptureWorkspaceChangeSet(context.Background(), ports.CaptureWorkspaceChangeSetRequest{ChangeSetID: "set", View: testkit.WorkspaceView(), RequestedNotAfter: testkit.FixedNow.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if set.State != contract.ChangeSetStaged || set.RuntimeSettlement != nil || set.CommittedRevision != "" {
		t.Fatal("SDK capture forged commit authority")
	}
}

func TestSDKRejectsTypedNilDependencies(t *testing.T) {
	var lifecycle *lifecycleStub
	_, err := New(Config{Backends: []contract.BackendDescriptor{testkit.Backend()}, Facts: testkit.NewMemoryStore(), Lifecycle: lifecycle, WorkspaceCapture: &captureStub{}, Clock: func() time.Time { return testkit.FixedNow }})
	if err == nil {
		t.Fatal("typed-nil lifecycle dependency was accepted")
	}
}

func TestSDKOptionalGovernedSurfacesFailClosedWhenUnwired(t *testing.T) {
	client, err := New(Config{Backends: []contract.BackendDescriptor{testkit.Backend()}, Facts: testkit.NewMemoryStore(), Lifecycle: &lifecycleStub{}, WorkspaceCapture: &captureStub{}, Clock: func() time.Time { return testkit.FixedNow }})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.CompleteCheckpointParticipant(context.Background(), applicationcontract.CheckpointParticipantWorkRequestV1{}); !errors.Is(err, ports.ErrUnsupported) {
		t.Fatalf("unwired checkpoint SDK surface = %v", err)
	}
	if _, err := client.ReserveSnapshotArtifact(context.Background(), nil); !errors.Is(err, ports.ErrUnsupported) {
		t.Fatalf("unwired Snapshot SDK surface = %v", err)
	}
	if _, err := client.StageWorkspaceRestore(context.Background(), nil); !errors.Is(err, ports.ErrUnsupported) {
		t.Fatalf("unwired Restore SDK surface = %v", err)
	}
	if _, err := client.ComposeWorkspaceRewindV1(context.Background(), contract.ComposeWorkspaceRewindRequestV1{}); !errors.Is(err, ports.ErrUnsupported) {
		t.Fatalf("unwired Rewind SDK surface = %v", err)
	}
}

var errLifecycleSentinel = errors.New("governed lifecycle stub invoked")

type lifecycleStub struct{}

func (*lifecycleStub) StartOrInspectSandboxLifecycleV4(context.Context, applicationcontract.SandboxLifecycleRequestV4) (applicationcontract.SandboxLifecycleResultV4, error) {
	return applicationcontract.SandboxLifecycleResultV4{}, errLifecycleSentinel
}

func (*lifecycleStub) InspectSandboxLifecycleV4(context.Context, applicationcontract.SandboxLifecycleRequestV4) (applicationcontract.SandboxLifecycleResultV4, error) {
	return applicationcontract.SandboxLifecycleResultV4{}, errLifecycleSentinel
}

type captureStub struct {
	set contract.WorkspaceChangeSet
}

func (s *captureStub) CaptureWorkspaceChangeSetV1(context.Context, ports.CaptureWorkspaceChangeSetRequest) (contract.WorkspaceChangeSet, error) {
	return s.set, nil
}

func workspaceSet(t *testing.T) contract.WorkspaceChangeSet {
	t.Helper()
	view := testkit.WorkspaceView()
	change := contract.WorkspaceChange{Kind: contract.WorkspaceAdd, Path: "src/generated/file", BlobRef: refPointer(testkit.Ref("blob"))}
	set := contract.WorkspaceChangeSet{ViewRef: view.Meta.Ref(), BaseArtifactRef: view.BaseArtifactRef, BaseRevision: view.BaseRevision, Changes: []contract.WorkspaceChange{change}, CanonicalPathSet: []string{change.Path}, State: contract.ChangeSetStaged}
	meta, err := contract.SealWorkspaceChangeSetMeta("set", 1, testkit.FixedNow, testkit.FixedNow.Add(time.Hour), set)
	if err != nil {
		t.Fatal(err)
	}
	set.Meta = meta
	return set
}

func refPointer(value contract.Ref) *contract.Ref { return &value }
