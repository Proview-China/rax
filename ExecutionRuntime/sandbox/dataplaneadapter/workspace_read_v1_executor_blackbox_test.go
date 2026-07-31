package dataplaneadapter_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	runtimecontrol "github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	runtimecore "github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimefakes "github.com/Proview-China/rax/ExecutionRuntime/runtime/fakes"
	runtimekernel "github.com/Proview-China/rax/ExecutionRuntime/runtime/kernel"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	dataplaneadapter "github.com/Proview-China/rax/ExecutionRuntime/sandbox/dataplaneadapter"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/kernel"
	sandboxports "github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/runtimeadapter"
	sqlitestore "github.com/Proview-China/rax/ExecutionRuntime/sandbox/storage/sqlite"
)

type workspaceReadExecutorCaseV1 struct {
	setup               func(*testing.T, string)
	mutateWorkspace     func(*contract.WorkspaceView)
	startByte           uint64
	hiddenScopes        []string
	expectedState       contract.WorkspaceReadStateV1
	expectedContent     string
	expectedAdapter     uint64
	expectedPhysical    uint64
	expectedBoundary    kernel.WorkspaceReadActualPointBoundaryV1
	expectInspectError  bool
	expectBeforeActual  bool
	runtimeLeaseS2Drift bool
	driftReservation    bool
	driftAttempt        bool
	useFakeActual       bool
	verifyBindingV2     bool
}

type lockedBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (b *lockedBuffer) Write(value []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.Write(value)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.String()
}

func TestWorkspaceReadPublicExecutorCallsConcreteAdapterAndRustOnce(t *testing.T) {
	runWorkspaceReadPublicExecutorV1(t, workspaceReadExecutorCaseV1{startByte: 6, expectedState: contract.WorkspaceReadObservedV1, expectedContent: "Praxis", expectedAdapter: 1, expectedPhysical: 1})
}

func TestWorkspaceReadPublicExecutorCreatesRuntimeAttemptAdmissionBindingV2(t *testing.T) {
	runWorkspaceReadPublicExecutorV1(t, workspaceReadExecutorCaseV1{
		startByte: 6, expectedState: contract.WorkspaceReadObservedV1,
		expectedContent: "Praxis", expectedAdapter: 1, expectedPhysical: 1,
		useFakeActual: true, verifyBindingV2: true,
	})
}

func TestWorkspaceReadPublicExecutorClassifiesPhysicalBoundariesThroughRustIPC(t *testing.T) {
	cases := []struct {
		name string
		spec workspaceReadExecutorCaseV1
	}{
		{name: "symlink", spec: workspaceReadExecutorCaseV1{
			setup: func(t *testing.T, root string) {
				outside := filepath.Join(root, "outside.txt")
				if err := os.WriteFile(outside, []byte("outside"), 0o640); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside, filepath.Join(root, "src", "main.txt")); err != nil {
					t.Fatal(err)
				}
			},
			expectedState: contract.WorkspaceReadFailedV1, expectedAdapter: 1, expectedPhysical: 0, expectedBoundary: kernel.WorkspaceReadEffectNotStartedV1,
		}},
		{name: "special-directory", spec: workspaceReadExecutorCaseV1{
			setup: func(t *testing.T, root string) {
				if err := os.Mkdir(filepath.Join(root, "src", "main.txt"), 0o750); err != nil {
					t.Fatal(err)
				}
			},
			expectedState: contract.WorkspaceReadFailedV1, expectedAdapter: 1, expectedPhysical: 0, expectedBoundary: kernel.WorkspaceReadEffectNotStartedV1,
		}},
		{name: "oversized", spec: workspaceReadExecutorCaseV1{
			setup: func(t *testing.T, root string) {
				if err := os.WriteFile(filepath.Join(root, "src", "main.txt"), bytes.Repeat([]byte{'x'}, 1_048_577), 0o640); err != nil {
					t.Fatal(err)
				}
			},
			expectedState: contract.WorkspaceReadFailedV1, expectedAdapter: 1, expectedPhysical: 0, expectedBoundary: kernel.WorkspaceReadEffectNotStartedV1,
		}},
		{name: "hidden-scope", spec: workspaceReadExecutorCaseV1{
			hiddenScopes: []string{"src"}, expectedAdapter: 0, expectedPhysical: 0, expectBeforeActual: true,
		}},
		{name: "workspace-lease-fence-drift", spec: workspaceReadExecutorCaseV1{
			mutateWorkspace: func(value *contract.WorkspaceView) { value.Lease.FenceEpoch++ },
			expectedAdapter: 0, expectedPhysical: 0, expectBeforeActual: true,
		}},
		{name: "workspace-lease-s2-drift", spec: workspaceReadExecutorCaseV1{
			expectedState: contract.WorkspaceReadUnknownV1, expectedAdapter: 1, expectedPhysical: 1, runtimeLeaseS2Drift: true,
		}},
		{name: "sandbox-reservation-drift", spec: workspaceReadExecutorCaseV1{
			expectedState: contract.WorkspaceReadStartedV1, expectedAdapter: 1, expectedPhysical: 0, expectedBoundary: kernel.WorkspaceReadEffectNotStartedV1, expectInspectError: true, driftReservation: true,
		}},
		{name: "sandbox-attempt-drift", spec: workspaceReadExecutorCaseV1{
			expectedState: contract.WorkspaceReadUnknownV1, expectedAdapter: 1, expectedPhysical: 0, expectedBoundary: kernel.WorkspaceReadEffectNotStartedV1, driftAttempt: true,
		}},
		{name: "non-utf8", spec: workspaceReadExecutorCaseV1{
			setup: func(t *testing.T, root string) {
				if err := os.WriteFile(filepath.Join(root, "src", "main.txt"), []byte{0xff, 0xfe}, 0o640); err != nil {
					t.Fatal(err)
				}
			},
			expectedState: contract.WorkspaceReadUnknownV1, expectedAdapter: 1, expectedPhysical: 1, expectedBoundary: kernel.WorkspaceReadEffectStartedUnknownV1,
		}},
		{name: "eof-empty-success", spec: workspaceReadExecutorCaseV1{
			setup: func(t *testing.T, root string) {
				if err := os.WriteFile(filepath.Join(root, "src", "main.txt"), []byte("tiny"), 0o640); err != nil {
					t.Fatal(err)
				}
			},
			startByte: 4, expectedState: contract.WorkspaceReadObservedV1, expectedContent: "", expectedAdapter: 1, expectedPhysical: 1,
		}},
	}
	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			runWorkspaceReadPublicExecutorV1(t, testCase.spec)
		})
	}
}

func runWorkspaceReadPublicExecutorV1(t *testing.T, spec workspaceReadExecutorCaseV1) {
	binary := os.Getenv("PRAXIS_SANDBOX_DATAPLANE_BIN")
	if binary == "" && !spec.useFakeActual {
		t.Skip("set PRAXIS_SANDBOX_DATAPLANE_BIN to run the public executor black box")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	root, err := os.MkdirTemp("/tmp", "praxis-wr-executor-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o750); err != nil {
		t.Fatal(err)
	}
	if spec.setup != nil {
		spec.setup(t, root)
	} else if err := os.WriteFile(filepath.Join(root, "src", "main.txt"), []byte("hello Praxis"), 0o640); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	expires := now.Add(30 * time.Second)
	workspaceExpires := time.Unix(0, 4_000_000_008_000_000_000)
	workspaceDigest := "sha256:021c67f065e27e7e93918819bfbd85d62dc7c9a50a7599779b68bdc4a293f2ba"
	fileScopeDigest := "sha256:a9e25d859be79d6716a9397eed2e1400d936a6b3233247f5664d1a06e42478a7"
	providerPayload, err := dataplaneadapter.NewWorkspaceReadPayloadV1(dataplaneadapter.WorkspaceReadPayloadV1{
		WorkspaceBindingID: "workspace-view", WorkspaceDigest: workspaceDigest,
		Workspace: dataplaneadapter.ExactRefV1{
			ID: "workspace-view", Revision: 1, Digest: workspaceDigest, ExpiresUnixNano: workspaceExpires.UnixNano(),
		},
		FileScopeDigest: fileScopeDigest, RelativePath: "src/main.txt", StartByte: spec.startByte, MaxBytes: 6, S1Checked: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	providerPayloadDigest, err := digestWorkspaceReadExecutorIPC("ProviderPayloadV1", providerPayload)
	if err != nil {
		t.Fatal(err)
	}
	current := buildWorkspaceReadRuntimeCurrentV4(t, now, providerPayloadDigest)
	workspace := contract.WorkspaceView{
		Meta:            contract.Meta{ContractVersion: contract.ContractFamily, ID: "workspace-view", Revision: 1, Digest: workspaceDigest, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano(), ExpiresUnixNano: workspaceExpires.UnixNano()},
		BaseArtifactRef: contract.Ref{ID: "base-artifact", Revision: 1, Digest: digestWorkspaceReadExecutorTest("base-artifact")}, BaseRevision: "main",
		OverlayRef: contract.Ref{ID: "overlay", Revision: 1, Digest: digestWorkspaceReadExecutorTest("overlay")}, PolicyRef: contract.Ref{ID: "policy", Revision: 1, Digest: digestWorkspaceReadExecutorTest("policy")},
		Lease:      contract.RuntimeLeaseBinding{TenantID: string(current.Sandbox.Operation.ExecutionScope.Identity.TenantID), InstanceID: string(current.Sandbox.RuntimeLease.Instance.ID), InstanceEpoch: uint64(current.Sandbox.RuntimeLease.Instance.Epoch), LeaseID: string(current.Sandbox.RuntimeLease.Lease.ID), LeaseEpoch: uint64(current.Sandbox.RuntimeLease.Lease.Epoch), FenceEpoch: uint64(current.Sandbox.RuntimeLease.FenceEpoch), ScopeDigest: string(current.Sandbox.RuntimeLease.ScopeDigest), ObservedRevision: uint64(current.Sandbox.RuntimeLease.ObservedRevision), ExpiresUnixNano: current.Sandbox.RuntimeLease.Ref.ExpiresUnixNano},
		ReadScopes: []string{"src"}, WriteScopes: []string{"src"}, HiddenScopes: spec.hiddenScopes, FileScopeDigest: fileScopeDigest,
	}
	if spec.mutateWorkspace != nil {
		spec.mutateWorkspace(&workspace)
	}
	legacy := current.Dispatch.Record.Permit.LegacyPermit
	legacyDigest, err := legacy.DigestV3()
	if err != nil {
		t.Fatal(err)
	}
	delegation := runtimeports.ExecutionDelegationRefV2{ID: "delegation-workspace-read", Revision: 1, Digest: runtimecore.DigestBytes([]byte("delegation-workspace-read"))}
	preparedID, err := runtimeports.DerivePreparedProviderAttemptIDV2(delegation.ID, legacy.ID, legacy.AttemptID)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := runtimeports.SealPreparedProviderAttemptRefV2(runtimeports.PreparedProviderAttemptRefV2{
		ID: preparedID, Revision: 1, DeclaredDelegation: delegation, OperationDigest: current.Sandbox.OperationDigest,
		IntentID: current.Sandbox.EffectID, IntentRevision: current.Sandbox.IntentRevision, IntentDigest: current.Sandbox.IntentDigest,
		PermitID: legacy.ID, PermitRevision: legacy.Revision, PermitDigest: legacyDigest, AttemptID: current.Sandbox.AttemptID,
		Provider: current.Sandbox.ProviderBinding, PayloadSchema: legacy.PayloadSchema, PayloadDigest: legacy.PayloadDigest, PayloadRevision: legacy.PayloadRevision,
		PreparedUnixNano: now.UnixNano(), ExpiresUnixNano: expires.UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	attempt := runtimeports.OperationDispatchAttemptRefV3{OperationDigest: current.Sandbox.OperationDigest, EffectID: current.Sandbox.EffectID, IntentRevision: current.Sandbox.IntentRevision, IntentDigest: current.Sandbox.IntentDigest, PermitID: legacy.ID, PermitRevision: legacy.Revision, PermitDigest: legacyDigest, AttemptID: current.Sandbox.AttemptID, Delegation: &delegation}
	if err = attempt.Validate(); err != nil {
		t.Fatal(err)
	}
	owner := runtimeports.EffectOwnerRefV2{Role: runtimeports.OwnerSettlement, ComponentID: current.Sandbox.ProviderBinding.ComponentID, ManifestDigest: current.Sandbox.ProviderBinding.ManifestDigest}
	commandDraft := contract.WorkspaceReadCommandV1{
		TenantID: string(current.Sandbox.Operation.ExecutionScope.Identity.TenantID), SourceToolCommand: contract.Ref{ID: "tool-command", Revision: 1, Digest: digestWorkspaceReadExecutorTest("tool-command")},
		SourceToolPayloadSchema: legacy.PayloadSchema.Key(), SourceToolPayloadDigest: string(legacy.PayloadDigest), SourceToolPayloadRevision: uint64(legacy.PayloadRevision),
		WorkspaceView: workspace.Meta.Ref(), FileScopeDigest: fileScopeDigest, RelativePath: "src/main.txt", StartByte: spec.startByte, MaxBytes: 6, RequestedNotAfterUnixNano: expires.UnixNano(),
		OperationDigest: string(current.Sandbox.OperationDigest), EffectID: string(current.Sandbox.EffectID), IntentRevision: uint64(current.Sandbox.IntentRevision), IntentDigest: string(current.Sandbox.IntentDigest), AttemptID: current.Sandbox.AttemptID,
		PreparedDigest: string(prepared.Digest), ProviderComponent: string(current.Sandbox.ProviderBinding.ComponentID), ProviderManifest: string(current.Sandbox.ProviderBinding.ManifestDigest),
	}
	if spec.verifyBindingV2 {
		fileID, fileErr := contract.WorkspaceReadFileIDV1(workspace.Meta.ID, commandDraft.RelativePath)
		if fileErr != nil {
			t.Fatal(fileErr)
		}
		commandDraft.ExpectedFileRef = &contract.Ref{
			ID: fileID, Revision: workspace.Meta.Revision,
			Digest: digestWorkspaceReadExecutorTest("fake-whole-file"),
		}
	}
	dispatchDigest, err := runtimecore.CanonicalJSONDigest("praxis.sandbox.workspace-read", "1.0.0", "OperationDispatchAttemptRefV3", attempt)
	if err != nil {
		t.Fatal(err)
	}
	commandDraft.DispatchDigest = string(dispatchDigest)
	command, err := contract.SealWorkspaceReadCommandV1(commandDraft, "workspace-read-command", now, expires)
	if err != nil {
		t.Fatal(err)
	}
	domainCommand := runtimeports.OperationDomainCommandRefV1{Owner: owner, Kind: runtimeports.NamespacedNameV2(contract.WorkspaceReadCommandKindV1), ID: command.Meta.ID, Revision: runtimecore.Revision(command.Meta.Revision), Digest: runtimecore.Digest("sha256:" + command.Meta.Digest)}
	if err = prepared.Validate(); err != nil {
		t.Fatalf("prepared ref: %v", err)
	}
	if err = attempt.Validate(); err != nil {
		t.Fatalf("dispatch attempt: %v", err)
	}
	if err = domainCommand.Validate(); err != nil {
		t.Fatalf("domain command: %v; value=%#v", err, domainCommand)
	}
	association, err := runtimeports.SealPreparedDomainCommandAssociationCurrentProjectionV1(runtimeports.PreparedDomainCommandAssociationCurrentProjectionV1{
		Operation: current.Sandbox.Operation, OperationDigest: current.Sandbox.OperationDigest, EffectID: current.Sandbox.EffectID, EffectRevision: current.Sandbox.IntentRevision, IntentDigest: current.Sandbox.IntentDigest,
		Prepared: prepared, Attempt: attempt, Provider: current.Sandbox.ProviderBinding, PayloadSchema: legacy.PayloadSchema, PayloadDigest: legacy.PayloadDigest, PayloadRevision: legacy.PayloadRevision,
		DomainCommand: domainCommand, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires.UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	transport := runtimeports.ProviderBindingRefV2{BindingSetID: "transport", BindingSetRevision: 1, ComponentID: "praxis.runtime/transport", ManifestDigest: runtimecore.DigestBytes([]byte("transport-manifest")), ArtifactDigest: runtimecore.DigestBytes([]byte("transport-artifact")), Capability: runtimeports.ControlledOperationProviderTransportCapabilityV2}
	handoff := runtimeports.OperationScopeEvidenceProviderHandoffRefV3{ID: "handoff-workspace-read", Revision: 1, Digest: runtimecore.DigestBytes([]byte("handoff-workspace-read")), ExpiresUnixNano: expires.UnixNano()}
	authorization, err := runtimeports.SealControlledOperationPhysicalExecutionAuthorizationV3(runtimeports.ControlledOperationPhysicalExecutionAuthorizationV3{
		UnifiedNotAfterUnixNano: expires.UnixNano(), ProviderTransport: transport, Provider: current.Sandbox.ProviderBinding,
		Operation: current.Sandbox.Operation, OperationDigest: current.Sandbox.OperationDigest, OperationScopeDigest: current.Sandbox.Operation.ExecutionScopeDigest,
		EffectKind: runtimeports.OperationScopeEvidenceActionEffectKindV3, Prepared: prepared, Attempt: attempt, ExecuteEnforcement: current.Phase,
		ExecuteEvidenceHandoff: handoff, Boundary: runtimeports.OperationProviderBoundaryRefV1{ID: "boundary-workspace-read", Revision: 1, Digest: runtimecore.DigestBytes([]byte("boundary-workspace-read"))},
		Association: association.Ref, DomainCommand: domainCommand,
	})
	if err != nil {
		t.Fatal(err)
	}

	databasePath := filepath.Join(root, "sandbox.sqlite")
	store, err := sqlitestore.OpenWithClock(ctx, databasePath, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err = store.CreateWorkspaceViewV1(ctx, workspace); err != nil {
		t.Fatal(err)
	}
	if _, err = store.CreateWorkspaceReadCommandV1(ctx, command); err != nil {
		t.Fatal(err)
	}

	dispatchSocket := filepath.Join(root, "dispatch.sock")
	currentSocket := filepath.Join(root, "current.sock")
	baseCurrent, err := runtimeadapter.NewWorkspaceReadCurrentAdapterV1(
		fixedWorkspaceReadEnforcementReaderV1{value: current},
		fixedWorkspaceReadAssociationReaderV1{association},
		store,
		store,
		time.Now,
	)
	if err != nil {
		t.Fatal(err)
	}
	exactCurrent, err := runtimeadapter.NewWorkspaceReadCurrentAdapterV2(baseCurrent, store, store, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	countedCurrent := &countingWorkspaceReadCurrentReaderV2{inner: exactCurrent}
	var output lockedBuffer
	var actualPoint kernel.WorkspaceReadActualPointV1
	if spec.useFakeActual {
		actualPoint = &successfulWorkspaceReadActualPointV2{}
	} else {
		currentServer := dataplaneadapter.CurrentServer{
			SocketPath: currentSocket, SocketMode: 0o660, AllowedUID: uint32(os.Getuid()),
			Governance: fixedWorkspaceReadEnforcementReaderV1{value: current},
			Sandbox:    testkit.NewMemoryStore(), WorkspaceReadCurrentV2: countedCurrent, Now: time.Now,
		}
		listener, listenErr := currentServer.Listen()
		if listenErr != nil {
			t.Fatal(listenErr)
		}
		defer listener.Close()
		go func() { _ = currentServer.Serve(ctx, listener) }()
		config := map[string]any{"contract_version": "praxis.sandbox/data-plane-root/v1", "dispatch_socket": dispatchSocket, "current_reader_socket": currentSocket, "journal_path": filepath.Join(root, "journal.jsonl"), "checkpoint_store_path": filepath.Join(root, "checkpoints.sqlite"), "allowed_dispatch_uid": os.Getuid(), "allowed_current_reader_uid": os.Getuid(), "socket_mode": 0o660, "wasmtime_component_bindings": map[string]string{}, "workspace_read": map[string]any{"bindings": map[string]any{"workspace-view": map[string]any{"path": root, "digest": workspaceDigest}}}}
		configBytes, _ := json.Marshal(config)
		configPath := filepath.Join(root, "config.json")
		if err = os.WriteFile(configPath, configBytes, 0o600); err != nil {
			t.Fatal(err)
		}
		process := exec.CommandContext(ctx, binary, configPath)
		process.Stdout, process.Stderr = &output, &output
		if err = process.Start(); err != nil {
			t.Fatal(err)
		}
		defer func() { cancel(); _ = process.Wait() }()
		waitForWorkspaceReadExecutorSocket(t, dispatchSocket, &output)
		concrete, concreteErr := dataplaneadapter.NewWorkspaceReadActualPointAdapterV1(dataplaneadapter.Client{SocketPath: dispatchSocket, AllowedUID: uint32(os.Getuid())})
		if concreteErr != nil {
			t.Fatal(concreteErr)
		}
		actualPoint = concrete
		if spec.driftReservation || spec.driftAttempt {
			actualPoint = &driftingWorkspaceReadActualPointV1{
				inner:            concrete,
				store:            store,
				databasePath:     databasePath,
				driftReservation: spec.driftReservation,
				driftAttempt:     spec.driftAttempt,
			}
		}
	}
	counted := &countingWorkspaceReadActualPointV1{inner: actualPoint}
	enforcementReader := &sequencedWorkspaceReadEnforcementReaderV1{first: current, second: current}
	if spec.runtimeLeaseS2Drift {
		enforcementReader.second.Sandbox.RuntimeLease.FenceEpoch++
		enforcementReader.second.Sandbox, err = runtimeports.SealOperationDispatchSandboxCurrentProjectionV4(enforcementReader.second.Sandbox)
		if err != nil {
			t.Fatal(err)
		}
		enforcementReader.second, err = runtimeports.SealCurrentOperationDispatchEnforcementV4(enforcementReader.second)
		if err != nil {
			t.Fatal(err)
		}
	}
	executor, err := kernel.NewWorkspaceReadPhysicalExecutorV1(store, fixedWorkspaceReadAssociationReaderV1{association}, store, fixedWorkspaceReadSandboxReaderV1{current.Sandbox}, enforcementReader, store, counted, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if spec.expectBeforeActual {
		if _, err = executor.ExecuteControlledOperationPhysicalV3(ctx, authorization); err == nil {
			t.Fatal("pre-actual scope rejection unexpectedly succeeded")
		}
		if counted.calls.Load() != spec.expectedAdapter {
			t.Fatalf("actual-point adapter calls=%d, want %d", counted.calls.Load(), spec.expectedAdapter)
		}
		inspectionDB, openErr := sql.Open("sqlite", databasePath)
		if openErr != nil {
			t.Fatal(openErr)
		}
		defer inspectionDB.Close()
		var rows int
		if countErr := inspectionDB.QueryRowContext(ctx, `SELECT count(*) FROM workspace_read_attempt_origin`).Scan(&rows); countErr != nil || rows != 0 {
			t.Fatalf("pre-actual rejection created attempt rows=%d err=%v", rows, countErr)
		}
		return
	}
	if spec.expectedState != contract.WorkspaceReadObservedV1 || spec.expectedContent != "Praxis" {
		_, callErr := executor.ExecuteControlledOperationPhysicalV3(ctx, authorization)
		if spec.expectedState == contract.WorkspaceReadObservedV1 {
			if callErr != nil {
				t.Fatalf("expected successful bounded read: %v", callErr)
			}
		} else if callErr == nil {
			t.Fatalf("expected terminal %q error", spec.expectedState)
		}
		if counted.calls.Load() != spec.expectedAdapter {
			t.Fatalf("actual-point adapter calls=%d, want %d", counted.calls.Load(), spec.expectedAdapter)
		}
		if spec.expectedBoundary != "" {
			var boundary *kernel.WorkspaceReadActualPointErrorV1
			if !errors.As(counted.lastError(), &boundary) || boundary.Boundary != spec.expectedBoundary {
				t.Fatalf("actual-point boundary=%v, want %q; err=%v", boundary, spec.expectedBoundary, counted.lastError())
			}
			var closed *dataplaneadapter.ClosedError
			if !errors.As(counted.lastError(), &closed) || closed.CrossedActualPoint == nil || closed.PhysicalReadCount == nil || *closed.PhysicalReadCount != spec.expectedPhysical || *closed.CrossedActualPoint != (spec.expectedPhysical != 0) {
				t.Fatalf("physical evidence=%#v, want reads=%d", closed, spec.expectedPhysical)
			}
		}
		inspectionDB, openErr := sql.Open("sqlite", databasePath)
		if openErr != nil {
			t.Fatal(openErr)
		}
		defer inspectionDB.Close()
		var originBody []byte
		if err = inspectionDB.QueryRowContext(ctx, "SELECT body FROM workspace_read_attempt_origin LIMIT 1").Scan(&originBody); err != nil {
			t.Fatal(err)
		}
		var origin contract.WorkspaceReadAttemptV1
		if err = json.Unmarshal(originBody, &origin); err != nil {
			t.Fatal(err)
		}
		projection, inspectErr := executor.InspectBoundedWorkspaceReadV1(ctx, contract.WorkspaceReadAttemptRefV1{ID: origin.Meta.ID, Revision: origin.Meta.Revision, Digest: origin.Meta.Digest})
		if spec.expectInspectError {
			if inspectErr == nil {
				t.Fatal("owner-current corruption unexpectedly produced an exact projection")
			}
			return
		}
		if inspectErr != nil || projection.Attempt.State != spec.expectedState {
			t.Fatalf("exact Inspect state=%q, want %q err=%v", projection.Attempt.State, spec.expectedState, inspectErr)
		}
		if spec.expectedState == contract.WorkspaceReadObservedV1 && (projection.Observation == nil || projection.Observation.Content != spec.expectedContent || !projection.Observation.Complete) {
			t.Fatalf("EOF result drifted: %#v", projection.Observation)
		}
		if spec.expectedAdapter != 0 && countedCurrent.calls.Load() == 0 {
			t.Fatal("public CurrentServer never inspected workspace read current v2")
		}
		return
	}

	const workers = 64
	var wait sync.WaitGroup
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, callErr := executor.ExecuteControlledOperationPhysicalV3(ctx, authorization)
			errs <- callErr
		}()
	}
	wait.Wait()
	close(errs)
	successful := 0
	var firstIndeterminate error
	errorCounts := map[string]int{}
	for callErr := range errs {
		if callErr == nil {
			successful++
		} else if !runtimecore.HasReason(callErr, runtimecore.ReasonEffectUnknownOutcome) {
			t.Fatalf("unexpected concurrent result: %v; actual-point=%v; rust=%s", callErr, counted.lastError(), output.String())
		} else if firstIndeterminate == nil {
			firstIndeterminate = callErr
		}
		if callErr != nil {
			errorCounts[callErr.Error()]++
		}
	}
	if successful == 0 {
		t.Fatalf("no concurrent execution observed the committed result; first indeterminate=%v; actual-point=%v; errors=%v; rust=%s", firstIndeterminate, counted.lastError(), errorCounts, output.String())
	}
	if counted.calls.Load() != 1 {
		t.Fatalf("actual point calls=%d, want 1", counted.calls.Load())
	}
	if _, err = executor.ExecuteControlledOperationPhysicalV3(ctx, authorization); err != nil {
		t.Fatalf("deterministic replay: %v", err)
	}
	if counted.calls.Load() != 1 {
		t.Fatalf("replay re-entered actual point: %d", counted.calls.Load())
	}

	db, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var originBody []byte
	if err = db.QueryRowContext(ctx, "SELECT body FROM workspace_read_attempt_origin LIMIT 1").Scan(&originBody); err != nil {
		t.Fatal(err)
	}
	var origin contract.WorkspaceReadAttemptV1
	if err = json.Unmarshal(originBody, &origin); err != nil {
		t.Fatal(err)
	}
	projection, err := executor.InspectBoundedWorkspaceReadV1(ctx, contract.WorkspaceReadAttemptRefV1{ID: origin.Meta.ID, Revision: origin.Meta.Revision, Digest: origin.Meta.Digest})
	if err != nil {
		t.Fatal(err)
	}
	if projection.Attempt.State != contract.WorkspaceReadObservedV1 || projection.Observation == nil || projection.Observation.Content != "Praxis" {
		t.Fatalf("unexpected exact Inspect: %#v", projection)
	}
	if !spec.useFakeActual && countedCurrent.calls.Load() == 0 {
		t.Fatal("public CurrentServer never inspected workspace read current v2")
	}
	if spec.verifyBindingV2 {
		bindingV2, inspectErr := executor.InspectWorkspaceReadAdmissionForRuntimeAttemptV2(ctx, authorization.Attempt)
		if inspectErr != nil {
			t.Fatal(inspectErr)
		}
		if !reflect.DeepEqual(bindingV2.RuntimeAttempt, authorization.Attempt) ||
			bindingV2.WorkspaceReadCommand.Meta.Ref() != command.Meta.Ref() ||
			bindingV2.WorkspaceReadAttempt.OwnerRef() != origin.Meta.Ref() {
			t.Fatalf("V2 historical closure drifted: %#v", bindingV2)
		}
		delegationSplices := []struct {
			name   string
			mutate func(*runtimeports.OperationDispatchAttemptRefV3)
		}{
			{name: "presence", mutate: func(value *runtimeports.OperationDispatchAttemptRefV3) { value.Delegation = nil }},
			{name: "id", mutate: func(value *runtimeports.OperationDispatchAttemptRefV3) { value.Delegation.ID = "delegation-unbound" }},
			{name: "revision", mutate: func(value *runtimeports.OperationDispatchAttemptRefV3) { value.Delegation.Revision++ }},
			{name: "digest", mutate: func(value *runtimeports.OperationDispatchAttemptRefV3) {
				value.Delegation.Digest = runtimecore.DigestBytes([]byte("delegation-unbound"))
			}},
		}
		for _, splice := range delegationSplices {
			spliced := authorization.Attempt
			delegationCopy := *authorization.Attempt.Delegation
			spliced.Delegation = &delegationCopy
			splice.mutate(&spliced)
			if _, inspectErr = executor.InspectWorkspaceReadAdmissionForRuntimeAttemptV2(ctx, spliced); inspectErr == nil {
				t.Fatalf("Delegation %s splice was accepted", splice.name)
			}
		}
		restarted, openErr := sqlitestore.OpenWithClock(ctx, databasePath, time.Now)
		if openErr != nil {
			t.Fatal(openErr)
		}
		recovered, inspectErr := restarted.InspectWorkspaceReadAdmissionForRuntimeAttemptV2(ctx, authorization.Attempt)
		_ = restarted.Close()
		if inspectErr != nil || !reflect.DeepEqual(recovered, bindingV2) {
			t.Fatalf("restart/lost-reply exact Inspect: %#v err=%v", recovered, inspectErr)
		}
		const restartHandles = 8
		handles := make([]*sqlitestore.Store, 0, restartHandles)
		for range restartHandles {
			handle, openHandleErr := sqlitestore.OpenWithClock(ctx, databasePath, time.Now)
			if openHandleErr != nil {
				t.Fatal(openHandleErr)
			}
			handles = append(handles, handle)
		}
		var restartGroup sync.WaitGroup
		restartErrors := make(chan error, restartHandles)
		for _, handle := range handles {
			handle := handle
			restartGroup.Add(1)
			go func() {
				defer restartGroup.Done()
				value, readErr := handle.InspectWorkspaceReadAdmissionForRuntimeAttemptV2(ctx, authorization.Attempt)
				if readErr == nil && !reflect.DeepEqual(value, bindingV2) {
					readErr = errors.New("reopened handle returned a different V2 binding")
				}
				restartErrors <- readErr
			}()
		}
		restartGroup.Wait()
		close(restartErrors)
		for _, handle := range handles {
			_ = handle.Close()
		}
		for readErr := range restartErrors {
			if readErr != nil {
				t.Fatal(readErr)
			}
		}
		expectedBefore := *bindingV2.WorkspaceReadCommand.ExpectedFileRef
		bindingV2.WorkspaceReadCommand.ExpectedFileRef.ID = "consumer-mutated"
		unalias, inspectErr := executor.InspectWorkspaceReadAdmissionForRuntimeAttemptV2(ctx, authorization.Attempt)
		if inspectErr != nil || unalias.WorkspaceReadCommand.ExpectedFileRef == nil ||
			*unalias.WorkspaceReadCommand.ExpectedFileRef != expectedBefore {
			t.Fatalf("V2 reader output alias leaked: %#v err=%v", unalias.WorkspaceReadCommand.ExpectedFileRef, inspectErr)
		}
		storedSplices := []struct {
			name    string
			splice  string
			restore string
			query   string
		}{
			{
				name: "RuntimeAttempt", splice: digestWorkspaceReadExecutorTest("spliced-operation"),
				restore: string(bindingV2.RuntimeAttempt.OperationDigest),
				query:   `UPDATE workspace_read_runtime_attempt_admission_binding_v2 SET operation_digest=?`,
			},
			{
				name: "Authorization", splice: digestWorkspaceReadExecutorTest("spliced-authorization"),
				restore: string(bindingV2.AuthorizationDigest),
				query:   `UPDATE workspace_read_runtime_attempt_admission_binding_v2 SET authorization_digest=?`,
			},
			{
				name: "Association", splice: digestWorkspaceReadExecutorTest("spliced-association"),
				restore: string(bindingV2.Association.Digest),
				query:   `UPDATE workspace_read_runtime_attempt_admission_binding_v2 SET association_digest=?`,
			},
			{
				name: "Command", splice: digestWorkspaceReadExecutorTest("spliced-command"),
				restore: bindingV2.WorkspaceReadCommand.Meta.Digest,
				query:   `UPDATE workspace_read_runtime_attempt_admission_binding_v2 SET command_digest=?`,
			},
			{
				name: "Admission", splice: digestWorkspaceReadExecutorTest("spliced-admission"),
				restore: string(bindingV2.AdmissionReceipt.Digest),
				query:   `UPDATE workspace_read_runtime_attempt_admission_binding_v2 SET admission_digest=?`,
			},
			{
				name: "Attempt", splice: digestWorkspaceReadExecutorTest("spliced-attempt"),
				restore: bindingV2.WorkspaceReadAttempt.Digest,
				query:   `UPDATE workspace_read_runtime_attempt_admission_binding_v2 SET workspace_attempt_digest=?`,
			},
		}
		for _, splice := range storedSplices {
			if _, err = db.ExecContext(ctx, splice.query, splice.splice); err != nil {
				t.Fatal(err)
			}
			if _, inspectErr = executor.InspectWorkspaceReadAdmissionForRuntimeAttemptV2(ctx, authorization.Attempt); inspectErr == nil {
				t.Fatalf("%s row splice was accepted", splice.name)
			}
			if _, err = db.ExecContext(ctx, splice.query, splice.restore); err != nil {
				t.Fatal(err)
			}
		}
		var (
			v2Body, v1Body, commandBody, reservationBody, originStoredBody []byte
			v2BindingDigest, v1AdmissionDigest                             string
		)
		if err = db.QueryRowContext(ctx, `SELECT body,binding_digest
			FROM workspace_read_runtime_attempt_admission_binding_v2`).Scan(&v2Body, &v2BindingDigest); err != nil {
			t.Fatal(err)
		}
		if err = db.QueryRowContext(ctx, `SELECT body,admission_digest
			FROM workspace_read_admission_attempt_binding`).Scan(&v1Body, &v1AdmissionDigest); err != nil {
			t.Fatal(err)
		}
		if err = db.QueryRowContext(ctx, `SELECT body FROM workspace_read_command_current`).Scan(&commandBody); err != nil {
			t.Fatal(err)
		}
		if err = db.QueryRowContext(ctx, `SELECT body FROM workspace_read_reservation`).Scan(&reservationBody); err != nil {
			t.Fatal(err)
		}
		if err = db.QueryRowContext(ctx, `SELECT body FROM workspace_read_attempt_origin`).Scan(&originStoredBody); err != nil {
			t.Fatal(err)
		}
		canonicalSplices := []struct {
			name    string
			query   string
			splice  any
			restore any
		}{
			{name: "V2 canonical body", query: `UPDATE workspace_read_runtime_attempt_admission_binding_v2 SET body=?`, splice: append(append([]byte(nil), v2Body...), ' '), restore: v2Body},
			{name: "V2 binding digest", query: `UPDATE workspace_read_runtime_attempt_admission_binding_v2 SET binding_digest=?`, splice: digestWorkspaceReadExecutorTest("spliced-binding"), restore: v2BindingDigest},
			{name: "V1 canonical body", query: `UPDATE workspace_read_admission_attempt_binding SET body=?`, splice: append(append([]byte(nil), v1Body...), ' '), restore: v1Body},
			{name: "V1 denormalized admission", query: `UPDATE workspace_read_admission_attempt_binding SET admission_digest=?`, splice: digestWorkspaceReadExecutorTest("spliced-v1-admission"), restore: v1AdmissionDigest},
			{name: "Command canonical row", query: `UPDATE workspace_read_command_current SET body=?`, splice: append(append([]byte(nil), commandBody...), ' '), restore: commandBody},
			{name: "Reservation canonical row", query: `UPDATE workspace_read_reservation SET body=?`, splice: append(append([]byte(nil), reservationBody...), ' '), restore: reservationBody},
			{name: "Attempt canonical row", query: `UPDATE workspace_read_attempt_origin SET body=?`, splice: append(append([]byte(nil), originStoredBody...), ' '), restore: originStoredBody},
		}
		for _, splice := range canonicalSplices {
			if _, err = db.ExecContext(ctx, splice.query, splice.splice); err != nil {
				t.Fatal(err)
			}
			if _, inspectErr = executor.InspectWorkspaceReadAdmissionForRuntimeAttemptV2(ctx, authorization.Attempt); inspectErr == nil {
				t.Fatalf("%s splice was accepted", splice.name)
			}
			if _, err = db.ExecContext(ctx, splice.query, splice.restore); err != nil {
				t.Fatal(err)
			}
		}
		splicedStable := digestWorkspaceReadExecutorTest("coordinated-stable-splice")
		if _, err = db.ExecContext(ctx, `UPDATE workspace_read_attempt_origin SET stable_digest=?`, splicedStable); err != nil {
			t.Fatal(err)
		}
		if _, err = db.ExecContext(ctx, `UPDATE workspace_read_reservation SET stable_digest=?`, splicedStable); err != nil {
			t.Fatal(err)
		}
		corruptBefore := workspaceReadV2DatabaseSnapshot(t, ctx, db)
		if _, inspectErr = executor.InspectWorkspaceReadAdmissionForRuntimeAttemptV2(ctx, authorization.Attempt); !errors.Is(inspectErr, sandboxports.ErrConflict) {
			t.Fatalf("coordinated stable denormalization splice error=%v", inspectErr)
		}
		if corruptAfter := workspaceReadV2DatabaseSnapshot(t, ctx, db); !reflect.DeepEqual(corruptAfter, corruptBefore) {
			t.Fatal("corrupt exact Inspect attempted repair")
		}
		if _, err = db.ExecContext(ctx, `UPDATE workspace_read_attempt_origin SET stable_digest=?`, origin.StableKeyDigest); err != nil {
			t.Fatal(err)
		}
		if _, err = db.ExecContext(ctx, `UPDATE workspace_read_reservation SET stable_digest=?`, projection.Reservation.StableKeyDigest); err != nil {
			t.Fatal(err)
		}

		referencedRows := []struct {
			name       string
			deleteSQL  string
			restoreSQL string
			args       []any
		}{
			{
				name:      "origin",
				deleteSQL: `DELETE FROM workspace_read_attempt_origin`,
				restoreSQL: `INSERT INTO workspace_read_attempt_origin(attempt_id,stable_digest,revision,digest,body)
					VALUES(?,?,?,?,?)`,
				args: []any{origin.Meta.ID, origin.StableKeyDigest, origin.Meta.Revision, origin.Meta.Digest, originStoredBody},
			},
			{
				name:      "reservation",
				deleteSQL: `DELETE FROM workspace_read_reservation`,
				restoreSQL: `INSERT INTO workspace_read_reservation(stable_digest,reservation_id,body)
					VALUES(?,?,?)`,
				args: []any{projection.Reservation.StableKeyDigest, projection.Reservation.Meta.ID, reservationBody},
			},
			{
				name:      "V1 binding",
				deleteSQL: `DELETE FROM workspace_read_admission_attempt_binding`,
				restoreSQL: `INSERT INTO workspace_read_admission_attempt_binding(
					admission_id,admission_revision,admission_digest,
					attempt_id,attempt_revision,attempt_digest,body
				) VALUES(?,?,?,?,?,?,?)`,
				args: []any{
					bindingV2.AdmissionBinding.AdmissionReceipt.ID,
					bindingV2.AdmissionBinding.AdmissionReceipt.Revision,
					bindingV2.AdmissionBinding.AdmissionReceipt.Digest,
					bindingV2.AdmissionBinding.Attempt.ID,
					bindingV2.AdmissionBinding.Attempt.Revision,
					bindingV2.AdmissionBinding.Attempt.Digest,
					v1Body,
				},
			},
		}
		for _, reference := range referencedRows {
			if _, err = db.ExecContext(ctx, reference.deleteSQL); err != nil {
				t.Fatal(err)
			}
			corruptBefore = workspaceReadV2DatabaseSnapshot(t, ctx, db)
			if _, inspectErr = executor.InspectWorkspaceReadAdmissionForRuntimeAttemptV2(ctx, authorization.Attempt); !errors.Is(inspectErr, sandboxports.ErrConflict) {
				t.Fatalf("missing referenced %s error=%v", reference.name, inspectErr)
			}
			if corruptAfter := workspaceReadV2DatabaseSnapshot(t, ctx, db); !reflect.DeepEqual(corruptAfter, corruptBefore) {
				t.Fatalf("missing %s Inspect attempted repair", reference.name)
			}
			if _, err = db.ExecContext(ctx, reference.restoreSQL, reference.args...); err != nil {
				t.Fatal(err)
			}
		}
		if counted.calls.Load() != 1 {
			t.Fatalf("read-only V2 inspections re-entered physical read: %d", counted.calls.Load())
		}
		watcher, watcherErr := db.Conn(ctx)
		if watcherErr != nil {
			t.Fatal(watcherErr)
		}
		defer watcher.Close()
		var dataVersionBefore, dataVersionAfter int64
		if watcherErr = watcher.QueryRowContext(ctx, `PRAGMA data_version`).Scan(&dataVersionBefore); watcherErr != nil {
			t.Fatal(watcherErr)
		}
		snapshotBefore := workspaceReadV2DatabaseSnapshot(t, ctx, db)
		const inspectors = 64
		var inspectGroup sync.WaitGroup
		inspectErrors := make(chan error, inspectors)
		for range inspectors {
			inspectGroup.Add(1)
			go func() {
				defer inspectGroup.Done()
				_, inspectCallErr := executor.InspectWorkspaceReadAdmissionForRuntimeAttemptV2(ctx, authorization.Attempt)
				inspectErrors <- inspectCallErr
			}()
		}
		inspectGroup.Wait()
		close(inspectErrors)
		for inspectCallErr := range inspectErrors {
			if inspectCallErr != nil {
				t.Fatal(inspectCallErr)
			}
		}
		if watcherErr = watcher.QueryRowContext(ctx, `PRAGMA data_version`).Scan(&dataVersionAfter); watcherErr != nil {
			t.Fatal(watcherErr)
		}
		if dataVersionAfter != dataVersionBefore {
			t.Fatalf("public V2 Inspect wrote database state: before=%d after=%d", dataVersionBefore, dataVersionAfter)
		}
		snapshotAfter := workspaceReadV2DatabaseSnapshot(t, ctx, db)
		if !reflect.DeepEqual(snapshotAfter, snapshotBefore) {
			t.Fatalf("public V2 Inspect changed owner rows: before=%v after=%v", snapshotBefore, snapshotAfter)
		}
		if counted.calls.Load() != 1 {
			t.Fatalf("64 public V2 inspections re-entered physical read: %d", counted.calls.Load())
		}
		var rows int
		if err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM workspace_read_runtime_attempt_admission_binding_v2`).Scan(&rows); err != nil || rows != 1 {
			t.Fatalf("V2 create-once rows=%d err=%v", rows, err)
		}

		// A same-Runtime-Attempt payload splice is rejected by exact Command
		// history before it can create another owner fact or physical read.
		splicedCommand := command
		splicedCommand.SourceToolPayloadDigest = digestWorkspaceReadExecutorTest("spliced-payload")
		splicedCommand, err = contract.SealWorkspaceReadCommandV1(splicedCommand, command.Meta.ID, now, expires)
		if err != nil {
			t.Fatal(err)
		}
		splicedBody, marshalErr := json.Marshal(splicedCommand)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		if _, err = db.ExecContext(ctx, `UPDATE workspace_read_command_current SET body=? WHERE command_id=?`, splicedBody, command.Meta.ID); err != nil {
			t.Fatal(err)
		}
		if _, callErr := executor.ExecuteControlledOperationPhysicalV3(ctx, authorization); callErr == nil {
			t.Fatal("same Runtime Attempt with spliced payload was accepted")
		}
		if counted.calls.Load() != 1 {
			t.Fatalf("payload splice re-entered physical read: %d", counted.calls.Load())
		}
	}
}

type successfulWorkspaceReadActualPointV2 struct{}

func (*successfulWorkspaceReadActualPointV2) ReadWorkspaceFileV1(
	_ context.Context,
	request kernel.WorkspaceReadActualPointRequestV1,
) (kernel.WorkspaceReadActualPointResultV1, error) {
	content := "Praxis"
	fileID, err := contract.WorkspaceReadFileIDV1(request.Workspace.Meta.ID, request.Command.RelativePath)
	if err != nil {
		return kernel.WorkspaceReadActualPointResultV1{}, err
	}
	file := contract.Ref{
		ID: fileID, Revision: request.Workspace.Meta.Revision,
		Digest: digestWorkspaceReadExecutorTest("fake-whole-file"),
	}
	if request.Command.ExpectedFileRef != nil {
		file = *request.Command.ExpectedFileRef
	}
	return kernel.WorkspaceReadActualPointResultV1{
		File: file, Content: content,
		ContentDigest: contract.WorkspaceReadContentDigestV1([]byte(content), request.Command.StartByte, request.Command.StartByte+uint64(len(content)), true),
		StartByte:     request.Command.StartByte, ReturnedBytes: uint64(len(content)),
		TotalBytes: request.Command.StartByte + uint64(len(content)), Complete: true,
		ProviderS1Checked: true, ProviderS2Checked: true, PhysicalReadCount: 1,
		ProviderReceipt: contract.WorkspaceReadReceiptBindingV1{
			ID: "fake-provider-receipt", Revision: 1,
			Digest:            digestWorkspaceReadExecutorTest("fake-provider-receipt"),
			ObservationDigest: digestWorkspaceReadExecutorTest("fake-provider-observation"),
			StableKeyDigest:   request.Reservation.StableKeyDigest,
			CheckedUnixNano:   request.S1CheckedUnixNano, ExpiresUnixNano: request.ExpiresUnixNano,
		},
	}, nil
}

type countingWorkspaceReadCurrentReaderV2 struct {
	inner sandboxports.WorkspaceReadCurrentProjectionReaderV2
	calls atomic.Uint64
}

func (r *countingWorkspaceReadCurrentReaderV2) InspectWorkspaceReadCurrentV2(ctx context.Context, query sandboxports.WorkspaceReadCurrentQueryV2) (sandboxports.WorkspaceReadCurrentProjectionV2, error) {
	r.calls.Add(1)
	return r.inner.InspectWorkspaceReadCurrentV2(ctx, query)
}

type fixedWorkspaceReadAssociationReaderV1 struct {
	value runtimeports.PreparedDomainCommandAssociationCurrentProjectionV1
}

func (r fixedWorkspaceReadAssociationReaderV1) InspectCurrentPreparedDomainCommandAssociationV1(context.Context, runtimeports.PreparedDomainCommandAssociationRefV1) (runtimeports.PreparedDomainCommandAssociationCurrentProjectionV1, error) {
	return r.value, nil
}

type fixedWorkspaceReadSandboxReaderV1 struct {
	value runtimeports.OperationDispatchSandboxCurrentProjectionV4
}

func (r fixedWorkspaceReadSandboxReaderV1) InspectOperationDispatchSandboxCurrentV4(context.Context, runtimeports.OperationSubjectV3, runtimecore.EffectIntentID, runtimeports.OperationDispatchSandboxFactRefV4) (runtimeports.OperationDispatchSandboxCurrentProjectionV4, error) {
	return r.value, nil
}

type fixedWorkspaceReadEnforcementReaderV1 struct {
	value runtimeports.CurrentOperationDispatchEnforcementV4
}

func (r fixedWorkspaceReadEnforcementReaderV1) EnforceCurrentOperationDispatchV4(context.Context, runtimeports.EnforceCurrentOperationDispatchRequestV4) (runtimeports.CurrentOperationDispatchEnforcementV4, error) {
	return runtimeports.CurrentOperationDispatchEnforcementV4{}, errors.New("not supported")
}
func (r fixedWorkspaceReadEnforcementReaderV1) InspectOperationDispatchEnforcementV4(context.Context, runtimeports.InspectOperationDispatchEnforcementRequestV4) (runtimeports.OperationDispatchEnforcementJournalV4, error) {
	return r.value.Journal, nil
}
func (r fixedWorkspaceReadEnforcementReaderV1) InspectCurrentOperationDispatchEnforcementV4(context.Context, runtimeports.InspectCurrentOperationDispatchEnforcementRequestV4) (runtimeports.CurrentOperationDispatchEnforcementV4, error) {
	return r.value, nil
}

type sequencedWorkspaceReadEnforcementReaderV1 struct {
	first  runtimeports.CurrentOperationDispatchEnforcementV4
	second runtimeports.CurrentOperationDispatchEnforcementV4
	calls  atomic.Uint64
}

func (r *sequencedWorkspaceReadEnforcementReaderV1) EnforceCurrentOperationDispatchV4(context.Context, runtimeports.EnforceCurrentOperationDispatchRequestV4) (runtimeports.CurrentOperationDispatchEnforcementV4, error) {
	return runtimeports.CurrentOperationDispatchEnforcementV4{}, errors.New("not supported")
}

func (r *sequencedWorkspaceReadEnforcementReaderV1) InspectOperationDispatchEnforcementV4(context.Context, runtimeports.InspectOperationDispatchEnforcementRequestV4) (runtimeports.OperationDispatchEnforcementJournalV4, error) {
	return r.first.Journal, nil
}

func (r *sequencedWorkspaceReadEnforcementReaderV1) InspectCurrentOperationDispatchEnforcementV4(context.Context, runtimeports.InspectCurrentOperationDispatchEnforcementRequestV4) (runtimeports.CurrentOperationDispatchEnforcementV4, error) {
	if r.calls.Add(1) == 1 {
		return r.first, nil
	}
	return r.second, nil
}

type countingWorkspaceReadActualPointV1 struct {
	inner kernel.WorkspaceReadActualPointV1
	calls atomic.Uint64
	mu    sync.Mutex
	err   error
}

type driftingWorkspaceReadActualPointV1 struct {
	inner            kernel.WorkspaceReadActualPointV1
	store            *sqlitestore.Store
	databasePath     string
	driftReservation bool
	driftAttempt     bool
	once             sync.Once
	err              error
}

func (a *driftingWorkspaceReadActualPointV1) ReadWorkspaceFileV1(ctx context.Context, input kernel.WorkspaceReadActualPointRequestV1) (kernel.WorkspaceReadActualPointResultV1, error) {
	a.once.Do(func() {
		switch {
		case a.driftReservation:
			drifted := input.Reservation
			drifted.RequestDigest = digestWorkspaceReadExecutorTest("spliced-reservation-request")
			var err error
			drifted, err = contract.SealWorkspaceReadReservationV1(
				drifted,
				input.Reservation.Meta.ID,
				time.Unix(0, input.Reservation.Meta.CreatedUnixNano),
				time.Unix(0, input.Reservation.Meta.ExpiresUnixNano),
			)
			if err != nil {
				a.err = err
				return
			}
			body, err := json.Marshal(drifted)
			if err != nil {
				a.err = err
				return
			}
			db, err := sql.Open("sqlite", a.databasePath)
			if err != nil {
				a.err = err
				return
			}
			defer db.Close()
			_, a.err = db.ExecContext(ctx, `UPDATE workspace_read_reservation SET body=? WHERE reservation_id=?`, body, input.Reservation.Meta.ID)
		case a.driftAttempt:
			unknown, err := contract.Digest("workspace-read-test-attempt-drift", input.CurrentQuery.Attempt)
			if err != nil {
				a.err = err
				return
			}
			_, a.err = a.store.MarkWorkspaceReadUnknownV1(ctx, input.CurrentQuery.Attempt.OwnerRef(), unknown)
		}
	})
	if a.err != nil {
		return kernel.WorkspaceReadActualPointResultV1{}, a.err
	}
	return a.inner.ReadWorkspaceFileV1(ctx, input)
}

func (a *countingWorkspaceReadActualPointV1) ReadWorkspaceFileV1(ctx context.Context, input kernel.WorkspaceReadActualPointRequestV1) (kernel.WorkspaceReadActualPointResultV1, error) {
	a.calls.Add(1)
	wire, wireErr := json.Marshal(input.CurrentQuery)
	if wireErr == nil {
		var roundTrip sandboxports.WorkspaceReadCurrentQueryV2
		wireErr = json.Unmarshal(wire, &roundTrip)
		if wireErr == nil {
			wireErr = roundTrip.Validate()
		}
	}
	if wireErr != nil {
		return kernel.WorkspaceReadActualPointResultV1{}, wireErr
	}
	result, err := a.inner.ReadWorkspaceFileV1(ctx, input)
	a.mu.Lock()
	a.err = err
	a.mu.Unlock()
	return result, err
}

func (a *countingWorkspaceReadActualPointV1) lastError() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.err
}

func digestWorkspaceReadExecutorTest(value string) string {
	return string(runtimecore.DigestBytes([]byte(value)))
}

func workspaceReadV2DatabaseSnapshot(t *testing.T, ctx context.Context, db *sql.DB) map[string]string {
	t.Helper()
	tables := []string{
		"workspace_read_command_current",
		"workspace_read_command_body_seal",
		"workspace_read_reservation",
		"workspace_read_attempt_origin",
		"workspace_read_attempt_current",
		"workspace_read_attempt_owner_incarnation",
		"workspace_read_admission_attempt_binding",
		"workspace_read_runtime_attempt_admission_binding_v2",
		"workspace_read_observation",
		"workspace_read_recovery_evidence",
	}
	snapshot := make(map[string]string, len(tables)+1)
	for _, table := range tables {
		rows, err := db.QueryContext(ctx, "SELECT * FROM "+table+" ORDER BY rowid")
		if err != nil {
			t.Fatal(err)
		}
		columns, err := rows.Columns()
		if err != nil {
			_ = rows.Close()
			t.Fatal(err)
		}
		hash := sha256.New()
		_, _ = hash.Write([]byte(strings.Join(columns, "\x00")))
		count := 0
		for rows.Next() {
			values := make([]any, len(columns))
			pointers := make([]any, len(columns))
			for index := range values {
				pointers[index] = &values[index]
			}
			if err = rows.Scan(pointers...); err != nil {
				_ = rows.Close()
				t.Fatal(err)
			}
			body, marshalErr := json.Marshal(values)
			if marshalErr != nil {
				_ = rows.Close()
				t.Fatal(marshalErr)
			}
			_, _ = hash.Write(body)
			count++
		}
		if err = rows.Err(); err != nil {
			_ = rows.Close()
			t.Fatal(err)
		}
		if err = rows.Close(); err != nil {
			t.Fatal(err)
		}
		snapshot[table] = fmt.Sprintf("%d:%x", count, hash.Sum(nil))
	}
	var version int
	if err := db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	snapshot["PRAGMA user_version"] = strconv.Itoa(version)
	return snapshot
}

func digestWorkspaceReadExecutorIPC(kind string, value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	_, _ = hash.Write([]byte(dataplaneadapter.ContractVersionV1))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write([]byte(kind))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write(data)
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}

func waitForWorkspaceReadExecutorSocket(t *testing.T, path string, output *lockedBuffer) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if info, err := os.Stat(path); err == nil && info.Mode()&os.ModeSocket != 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("Rust Data Plane did not become ready: %s", output.String())
}

type workspaceReadRuntimeCurrentReaderV4 struct {
	snapshot runtimeports.OperationGovernanceSnapshotV3
}

func (r workspaceReadRuntimeCurrentReaderV4) InspectOperationGovernance(context.Context, runtimeports.OperationSubjectV3) (runtimeports.OperationGovernanceSnapshotV3, error) {
	return r.snapshot, nil
}

type workspaceReadRuntimeReviewReaderV4 struct {
	value runtimeports.OperationReviewCurrentProjectionV4
}

func (r workspaceReadRuntimeReviewReaderV4) InspectOperationReviewCurrentV4(context.Context, runtimeports.OperationEffectIntentV3) (runtimeports.OperationReviewCurrentProjectionV4, error) {
	return r.value, nil
}

func buildWorkspaceReadRuntimeCurrentV4(t *testing.T, now time.Time, providerPayloadDigest string) runtimeports.CurrentOperationDispatchEnforcementV4 {
	t.Helper()
	clock := func() time.Time { return now }
	expires := now.Add(45 * time.Second).UnixNano()
	scope := runtimecore.ExecutionScope{
		Identity:       runtimecore.AgentIdentityRef{TenantID: "tenant-workspace-read", ID: "identity-workspace-read", Epoch: 1},
		Lineage:        runtimecore.LineageRef{ID: "lineage-workspace-read", PlanDigest: runtimecore.DigestBytes([]byte("lineage-workspace-read"))},
		Instance:       runtimecore.InstanceRef{ID: "instance-workspace-read", Epoch: 1},
		SandboxLease:   &runtimecore.SandboxLeaseRef{ID: "lease-workspace-read", Epoch: 1},
		AuthorityEpoch: 1,
	}
	scopeDigest, err := runtimeports.ExecutionScopeDigestV2(scope)
	if err != nil {
		t.Fatal(err)
	}
	subject := runtimeports.OperationSubjectV3{
		Kind:           runtimeports.OperationScopeRunV3,
		ExecutionScope: scope, ExecutionScopeDigest: scopeDigest,
		RunID: "run-workspace-read", SubjectRevision: 1,
		CurrentProjectionRef: "operation-current-workspace-read", CurrentProjectionDigest: runtimecore.DigestBytes([]byte("operation-current-workspace-read")), CurrentProjectionRevision: 1,
	}
	subjectDigest, err := subject.DigestV3()
	if err != nil {
		t.Fatal(err)
	}
	provider := runtimeports.ProviderBindingRefV2{
		BindingSetID: "binding-workspace-read", BindingSetRevision: 1,
		ComponentID: "praxis.sandbox/workspace-read", ManifestDigest: runtimecore.DigestBytes([]byte("manifest-workspace-read")),
		ArtifactDigest: runtimecore.DigestBytes([]byte("artifact-workspace-read")), Capability: runtimeports.CapabilityNameV2(runtimeports.OperationScopeEvidenceActionEffectKindV3),
	}
	payload := runtimeports.OpaquePayloadV2{
		Schema: runtimeports.SchemaRefV2{
			Namespace: "praxis.tool", Name: "workspace-read", Version: "1.0.0", MediaType: "application/json",
			ContentDigest: runtimecore.DigestBytes([]byte("workspace-read-schema")),
		},
		ContentDigest: runtimecore.Digest(providerPayloadDigest), Length: 1, Ref: "workspace-read-provider-payload",
		LimitPolicy: runtimeports.OpaqueLimitPolicyRefV2{Policy: "praxis.tool/workspace-read-limit", Digest: runtimecore.DigestBytes([]byte("workspace-read-limit"))},
	}
	intent := runtimeports.OperationEffectIntentV3{
		ContractVersion: runtimeports.OperationEffectContractVersionV3,
		ID:              "operation-effect-workspace-read", Revision: 1, Operation: subject,
		Kind: runtimeports.OperationScopeEvidenceActionEffectKindV3, RiskClass: "praxis.tool/controlled",
		ActionScopeDigest: runtimecore.DigestBytes([]byte("workspace-read-action-scope")),
		Payload:           payload, PayloadRevision: 1, Target: "praxis.sandbox/workspace-read",
		ConflictDomain: runtimeports.ConflictDomainBindingV2{
			Domain: "praxis.sandbox/workspace-read", ScopeClass: runtimeports.EffectStableScopeTenantV2,
			ScopeDigest: runtimeports.StableTenantScopeDigestV2(scope.Identity.TenantID),
		},
		Owners: []runtimeports.EffectOwnerRefV2{
			{Role: runtimeports.OwnerCleanup, ComponentID: provider.ComponentID, ManifestDigest: provider.ManifestDigest},
			{Role: runtimeports.OwnerEffect, ComponentID: provider.ComponentID, ManifestDigest: provider.ManifestDigest},
			{Role: runtimeports.OwnerSettlement, ComponentID: provider.ComponentID, ManifestDigest: provider.ManifestDigest},
		},
		Provider:  provider,
		Authority: runtimeports.AuthorityBindingRefV2{Ref: "authority-workspace-read", Digest: runtimecore.DigestBytes([]byte("authority-workspace-read")), Revision: 1, Epoch: 1},
		Review: runtimeports.OperationReviewBindingRefV3{
			CaseRef: "review-case-workspace-read", CandidateDigest: runtimecore.DigestBytes([]byte("candidate-workspace-read")),
			CandidateRevision: 1, PolicyDigest: runtimecore.DigestBytes([]byte("review-policy-workspace-read")),
		},
		Budget: runtimeports.OperationBudgetBindingRefV3{
			Ref: "budget-workspace-read", Digest: runtimecore.DigestBytes([]byte("budget-workspace-read")), Revision: 1,
			PolicyDigest: runtimecore.DigestBytes([]byte("budget-policy-workspace-read")), SubjectDigest: subjectDigest,
		},
		Policy: runtimeports.OperationPolicyBindingRefV3{
			Ref: "policy-workspace-read", Digest: runtimecore.DigestBytes([]byte("policy-workspace-read")), Revision: 1, SubjectDigest: subjectDigest,
		},
		Idempotency: runtimeports.IdempotencyBindingV2{
			Key: "workspace-read-key", ScopeClass: runtimeports.EffectStableScopeTenantV2,
			ScopeDigest: runtimeports.StableTenantScopeDigestV2(scope.Identity.TenantID), Class: runtimecore.IdempotencyQueryable,
		},
		CredentialLeases: []runtimeports.CredentialLeaseRefV2{},
		ExpiresUnixNano:  now.Add(time.Minute).UnixNano(),
	}
	governanceRef := func(id string, digest runtimecore.Digest) runtimeports.OperationGovernanceFactRefV3 {
		return runtimeports.OperationGovernanceFactRefV3{Ref: id, Revision: 1, Digest: digest, ExpiresUnixNano: expires}
	}
	snapshot := runtimeports.OperationGovernanceSnapshotV3{
		Operation: subject, Active: true, ProjectionWatermark: 1,
		Identity:     governanceRef("identity-workspace-read", runtimecore.DigestBytes([]byte("identity-workspace-read"))),
		Binding:      governanceRef(provider.BindingSetID, runtimecore.DigestBytes([]byte("binding-workspace-read"))),
		CurrentScope: governanceRef(subject.CurrentProjectionRef, subject.CurrentProjectionDigest),
		Authority:    governanceRef(intent.Authority.Ref, intent.Authority.Digest),
		Review: runtimeports.OperationReviewAuthorizationV3{
			Case:            governanceRef(intent.Review.CaseRef, runtimecore.DigestBytes([]byte("review-case-workspace-read"))),
			CandidateDigest: intent.Review.CandidateDigest, CandidateRevision: intent.Review.CandidateRevision,
			Verdict:           governanceRef("review-verdict-workspace-read", runtimecore.DigestBytes([]byte("review-verdict-workspace-read"))),
			ReviewerAuthority: governanceRef("reviewer-authority-workspace-read", runtimecore.DigestBytes([]byte("reviewer-authority-workspace-read"))),
			PolicyDigest:      intent.Review.PolicyDigest, ExpiresUnixNano: expires,
		},
		Budget: governanceRef(intent.Budget.Ref, intent.Budget.Digest), Policy: governanceRef(intent.Policy.Ref, intent.Policy.Digest),
		Provider: provider, EnforcementPoint: provider, CapabilityGrantDigest: runtimecore.DigestBytes([]byte("capability-grant-workspace-read")),
		Credentials: []runtimeports.OperationCredentialCurrentFactV3{}, ExpiresUnixNano: expires,
	}
	currentReader := workspaceReadRuntimeCurrentReaderV4{snapshot: snapshot}
	effectStore := runtimefakes.NewOperationEffectStoreV3(clock)
	proposed, err := runtimecontrol.NewProposedOperationEffectFactV3(intent, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = effectStore.CreateOperationEffectV3(context.Background(), proposed); err != nil {
		t.Fatal(err)
	}
	accepted := proposed
	accepted.State = runtimecontrol.OperationEffectAcceptedV3
	accepted.Revision++
	accepted.UpdatedUnixNano = now.UnixNano()
	if _, err = effectStore.CompareAndSwapOperationEffectV3(context.Background(), subject, runtimecontrol.OperationEffectCASRequestV3{ExpectedRevision: proposed.Revision, Next: accepted}); err != nil {
		t.Fatal(err)
	}
	intentDigest, err := intent.DigestV3()
	if err != nil {
		t.Fatal(err)
	}
	reviewCurrent, err := runtimeports.SealOperationReviewCurrentProjectionV4(runtimeports.OperationReviewCurrentProjectionV4{
		Operation: subject, IntentID: intent.ID, IntentRevision: intent.Revision, IntentDigest: intentDigest,
		PayloadSchema: intent.Payload.Schema, PayloadDigest: intent.Payload.ContentDigest, PayloadRevision: intent.PayloadRevision,
		Target: runtimeports.OperationReviewTargetRefV4{Ref: intent.Target, Revision: intent.Review.CandidateRevision, Digest: intent.Review.CandidateDigest},
		Case:   snapshot.Review.Case, Verdict: snapshot.Review.Verdict, Basis: runtimeports.OperationReviewBasisAcceptedV4,
		Policy: runtimeports.OperationGovernanceFactRefV3{
			Ref: "review-policy-workspace-read", Revision: 1, Digest: intent.Review.PolicyDigest, ExpiresUnixNano: now.Add(30 * time.Second).UnixNano(),
		},
		ReviewerAuthority: snapshot.Review.ReviewerAuthority, Scope: snapshot.CurrentScope, Binding: snapshot.Binding,
		DecisionEvidence: []runtimeports.EvidenceRecordRefV2{{
			LedgerScopeDigest: runtimecore.DigestBytes([]byte("review-ledger-workspace-read")), Sequence: 1,
			RecordDigest: runtimecore.DigestBytes([]byte("review-evidence-workspace-read")),
		}},
		Current: true, CurrentnessDigest: runtimecore.DigestBytes([]byte("review-currentness-workspace-read")),
		ExpiresUnixNano: now.Add(30 * time.Second).UnixNano(),
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	reviewStore := runtimefakes.NewOperationReviewAuthorizationStoreV4(clock)
	reviewGateway := runtimekernel.OperationReviewAuthorizationGatewayV4{
		Facts: reviewStore, Effects: effectStore, Governance: currentReader,
		Reviews: workspaceReadRuntimeReviewReaderV4{value: reviewCurrent}, Clock: clock,
	}
	effectStore.BindOperationReviewAuthorizationFactsV4(reviewStore)
	reviewAuthorization, err := reviewGateway.CreateOperationReviewAuthorizationV4(context.Background(), runtimeports.CreateOperationReviewAuthorizationRequestV4{
		AuthorizationID: "authorization-workspace-read", Operation: subject, EffectID: intent.ID,
		ExpectedEffectRevision: accepted.Revision, RequestedTTL: 20 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	admissions := runtimecontrol.OperationEffectAdmissionGatewayV3{Effects: effectStore}
	admission, err := admissions.InspectAcceptedOperationEffectV3(context.Background(), subject, intent.ID)
	if err != nil {
		t.Fatal(err)
	}
	dispatchGateway := runtimecontrol.OperationGovernanceGatewayV4{
		Effects: effectStore, Admissions: admissions, Reviews: reviewGateway, Current: currentReader, Clock: clock,
	}
	issue := runtimeports.IssueGovernedOperationDispatchRequestV4{
		Operation: subject, EffectID: intent.ID, ExpectedEffectRevision: accepted.Revision,
		Admission: admission, ReviewAuthorization: reviewAuthorization.RefV4(),
		PermitID: "permit-workspace-read", AttemptID: "attempt-workspace-read", PermitTTL: 10 * time.Second,
	}
	issued, err := dispatchGateway.IssueOperationDispatchV4(context.Background(), issue)
	if err != nil {
		t.Fatal(err)
	}
	begun, err := dispatchGateway.BeginOperationDispatchV4(context.Background(), runtimeports.BeginGovernedOperationDispatchRequestV4{
		Operation: subject, EffectID: intent.ID, ExpectedEffectRevision: issued.Record.EffectFactRevision,
		PermitID: issue.PermitID, ExpectedPermitFactRevision: issued.Record.Revision,
		AdmissionDigest: issued.Record.Permit.Admission.Digest, ReviewAuthorization: reviewAuthorization.RefV4(),
	})
	if err != nil {
		t.Fatal(err)
	}
	sandboxExpires := now.Add(8 * time.Second).UnixNano()
	sandboxRef := func(id string) runtimeports.OperationDispatchSandboxFactRefV4 {
		return runtimeports.OperationDispatchSandboxFactRefV4{ID: id, Revision: 1, Digest: runtimecore.DigestBytes([]byte(id)), ExpiresUnixNano: sandboxExpires}
	}
	sandboxProjection, err := runtimeports.SealOperationDispatchSandboxCurrentProjectionV4(runtimeports.OperationDispatchSandboxCurrentProjectionV4{
		Operation: subject, OperationDigest: mustWorkspaceReadRuntimeOperationDigestV4(t, subject),
		EffectID: intent.ID, IntentRevision: intent.Revision, IntentDigest: begun.Record.Permit.LegacyPermit.IntentDigest,
		AttemptID: issue.AttemptID, Attempt: sandboxRef(issue.AttemptID), Reservation: sandboxRef("sandbox-reservation-workspace-read"),
		SandboxLease: *scope.SandboxLease,
		RuntimeLease: runtimeports.OperationDispatchRuntimeLeaseBindingV4{
			Ref: sandboxRef("runtime-lease-binding-workspace-read"), Lease: *scope.SandboxLease, Instance: scope.Instance,
			FenceEpoch: 1, ScopeDigest: scopeDigest, ObservedRevision: 1,
		},
		Generation: runtimeports.GenerationBindingAssociationRefV1{ID: "generation-workspace-read", Revision: 1, Digest: runtimecore.DigestBytes([]byte("generation-workspace-read"))},
		Placement:  sandboxRef("placement-workspace-read"), Backend: sandboxRef("backend-workspace-read"), Slot: sandboxRef("slot-workspace-read"),
		ProviderBinding: begun.Record.Permit.LegacyPermit.EnforcementPoint, Current: true, ProjectionRevision: 1, ExpiresUnixNano: sandboxExpires,
	})
	if err != nil {
		t.Fatal(err)
	}
	sandboxReader := fixedWorkspaceReadSandboxReaderV1{value: sandboxProjection}
	enforcementGateway := runtimecontrol.OperationDispatchEnforcementGatewayV4{
		Dispatch: dispatchGateway, Sandbox: sandboxReader, Facts: effectStore, Clock: clock,
	}
	prepareRequest := runtimeports.EnforceCurrentOperationDispatchRequestV4{
		Operation: subject, EffectID: intent.ID, PermitID: issue.PermitID,
		ExpectedPermitFactRevision: begun.Record.Revision, PermitDigest: begun.Record.PermitDigest,
		AdmissionDigest: begun.Record.Permit.Admission.Digest, ReviewAuthorization: reviewAuthorization.RefV4(), AttemptID: issue.AttemptID,
		Phase: runtimeports.OperationDispatchEnforcementPrepareV4, SandboxAttempt: sandboxProjection.Attempt,
		SandboxReservation: sandboxProjection.Reservation, SandboxProjectionDigest: sandboxProjection.ProjectionDigest,
		Verifier: begun.Record.Permit.LegacyPermit.EnforcementPoint,
	}
	prepared, err := enforcementGateway.EnforceCurrentOperationDispatchV4(context.Background(), prepareRequest)
	if err != nil {
		t.Fatal(err)
	}
	legacy := prepared.Dispatch.Record.Permit.LegacyPermit
	legacyDigest, err := legacy.DigestV3()
	if err != nil {
		t.Fatal(err)
	}
	delegation := runtimeports.ExecutionDelegationRefV2{ID: "delegation-workspace-read-runtime", Revision: 1, Digest: runtimecore.DigestBytes([]byte("delegation-workspace-read-runtime"))}
	preparedID, err := runtimeports.DerivePreparedProviderAttemptIDV2(delegation.ID, legacy.ID, legacy.AttemptID)
	if err != nil {
		t.Fatal(err)
	}
	preparedAttempt, err := runtimeports.SealPreparedProviderAttemptRefV2(runtimeports.PreparedProviderAttemptRefV2{
		ID: preparedID, Revision: 1, DeclaredDelegation: delegation, OperationDigest: mustWorkspaceReadRuntimeOperationDigestV4(t, subject),
		IntentID: legacy.IntentID, IntentRevision: legacy.IntentRevision, IntentDigest: legacy.IntentDigest,
		PermitID: legacy.ID, PermitRevision: legacy.Revision, PermitDigest: legacyDigest, AttemptID: legacy.AttemptID,
		Provider: legacy.EnforcementPoint, PayloadSchema: legacy.PayloadSchema, PayloadDigest: legacy.PayloadDigest, PayloadRevision: legacy.PayloadRevision,
		PreparedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(7 * time.Second).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	executeRequest := prepareRequest
	executeRequest.Phase = runtimeports.OperationDispatchEnforcementExecuteV4
	executeRequest.ExpectedJournalRevision = 1
	executeRequest.Prepare = &prepared.Phase
	executeRequest.PreparedAttempt = &preparedAttempt
	executed, err := enforcementGateway.EnforceCurrentOperationDispatchV4(context.Background(), executeRequest)
	if err != nil {
		t.Fatal(err)
	}
	return executed
}

func mustWorkspaceReadRuntimeOperationDigestV4(t *testing.T, subject runtimeports.OperationSubjectV3) runtimecore.Digest {
	t.Helper()
	digest, err := subject.DigestV3()
	if err != nil {
		t.Fatal(err)
	}
	return digest
}
