package dataplaneadapter_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
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
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/kernel"
	sqlitestore "github.com/Proview-China/rax/ExecutionRuntime/sandbox/storage/sqlite"
)

func TestWorkspaceReadPublicExecutorCallsConcreteAdapterAndRustOnce(t *testing.T) {
	binary := os.Getenv("PRAXIS_SANDBOX_DATAPLANE_BIN")
	if binary == "" {
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
	if err := os.WriteFile(filepath.Join(root, "src", "main.txt"), []byte("hello Praxis"), 0o640); err != nil {
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
		FileScopeDigest: fileScopeDigest, RelativePath: "src/main.txt", StartByte: 6, MaxBytes: 6, S1Checked: true,
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
		Lease:      contract.RuntimeLeaseBinding{TenantID: string(current.Sandbox.Operation.ExecutionScope.Identity.TenantID), InstanceID: string(current.Sandbox.RuntimeLease.Instance.ID), InstanceEpoch: uint64(current.Sandbox.RuntimeLease.Instance.Epoch), LeaseID: string(current.Sandbox.RuntimeLease.Lease.ID), LeaseEpoch: uint64(current.Sandbox.RuntimeLease.Lease.Epoch), FenceEpoch: uint64(current.Sandbox.RuntimeLease.FenceEpoch), ScopeDigest: string(current.Sandbox.RuntimeLease.ScopeDigest), ObservedRevision: uint64(current.Sandbox.RuntimeLease.ObservedRevision), ExpiresUnixNano: workspaceExpires.UnixNano()},
		ReadScopes: []string{"src"}, WriteScopes: []string{"src"}, FileScopeDigest: fileScopeDigest,
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
		WorkspaceView: workspace.Meta.Ref(), FileScopeDigest: fileScopeDigest, RelativePath: "src/main.txt", StartByte: 6, MaxBytes: 6, RequestedNotAfterUnixNano: expires.UnixNano(),
		OperationDigest: string(current.Sandbox.OperationDigest), EffectID: string(current.Sandbox.EffectID), IntentRevision: uint64(current.Sandbox.IntentRevision), IntentDigest: string(current.Sandbox.IntentDigest), AttemptID: current.Sandbox.AttemptID,
		PreparedDigest: string(prepared.Digest), ProviderComponent: string(current.Sandbox.ProviderBinding.ComponentID), ProviderManifest: string(current.Sandbox.ProviderBinding.ManifestDigest),
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
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: currentSocket, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go serveWorkspaceReadExecutorCurrentV1(ctx, t, listener)
	config := map[string]any{"contract_version": "praxis.sandbox/data-plane-root/v1", "dispatch_socket": dispatchSocket, "current_reader_socket": currentSocket, "journal_path": filepath.Join(root, "journal.jsonl"), "checkpoint_store_path": filepath.Join(root, "checkpoints.sqlite"), "allowed_dispatch_uid": os.Getuid(), "allowed_current_reader_uid": os.Getuid(), "socket_mode": 0o660, "wasmtime_component_bindings": map[string]string{}, "workspace_read": map[string]any{"bindings": map[string]any{"workspace-view": map[string]any{"path": root, "digest": workspaceDigest}}}}
	configBytes, _ := json.Marshal(config)
	configPath := filepath.Join(root, "config.json")
	if err = os.WriteFile(configPath, configBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	process := exec.CommandContext(ctx, binary, configPath)
	var output bytes.Buffer
	process.Stdout, process.Stderr = &output, &output
	if err = process.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { cancel(); _ = process.Wait() }()
	waitForWorkspaceReadExecutorSocket(t, dispatchSocket, &output)
	concrete, err := dataplaneadapter.NewWorkspaceReadActualPointAdapterV1(dataplaneadapter.Client{SocketPath: dispatchSocket, AllowedUID: uint32(os.Getuid())})
	if err != nil {
		t.Fatal(err)
	}
	counted := &countingWorkspaceReadActualPointV1{inner: concrete}
	executor, err := kernel.NewWorkspaceReadPhysicalExecutorV1(store, fixedWorkspaceReadAssociationReaderV1{association}, store, fixedWorkspaceReadSandboxReaderV1{current.Sandbox}, fixedWorkspaceReadEnforcementReaderV1{current}, store, counted, time.Now)
	if err != nil {
		t.Fatal(err)
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

type countingWorkspaceReadActualPointV1 struct {
	inner kernel.WorkspaceReadActualPointV1
	calls atomic.Uint64
	mu    sync.Mutex
	err   error
}

func (a *countingWorkspaceReadActualPointV1) ReadWorkspaceFileV1(ctx context.Context, input kernel.WorkspaceReadActualPointRequestV1) (kernel.WorkspaceReadActualPointResultV1, error) {
	a.calls.Add(1)
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

func serveWorkspaceReadExecutorCurrentV1(ctx context.Context, t *testing.T, listener *net.UnixListener) {
	t.Helper()
	for {
		connection, err := listener.AcceptUnix()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			t.Errorf("workspace-read current accept: %v", err)
			return
		}
		var request dataplaneadapter.DispatchRequestV1
		if err = readWorkspaceReadExecutorFrameV1(connection, &request); err != nil {
			_ = connection.Close()
			continue
		}
		authorization := dataplaneadapter.CurrentAuthorizationV1{
			ContractVersion: dataplaneadapter.ContractVersionV1,
			RequestDigest:   request.Digest,
			OperationDigest: request.OperationDigest,
			EffectID:        request.EffectID,
			AttemptID:       request.AttemptID,
			Phase:           request.Phase,
			ProviderBinding: request.ProviderBinding,
			SandboxProjection: dataplaneadapter.SandboxProjectionRefV1{
				Revision: 1, Digest: digestWorkspaceReadExecutorTest("sandbox-projection"),
				ExpiresUnixNano: request.RequestedNotAfterUnixNano,
			},
			ExecutionBinding:   request.ExecutionBinding,
			RuntimeEnforcement: request.RuntimeEnforcement,
			CheckedUnixNano:    time.Now().UnixNano(),
			ExpiresUnixNano:    request.RequestedNotAfterUnixNano,
		}
		authorization.Digest, err = digestWorkspaceReadExecutorIPC("CurrentAuthorizationV1", authorization)
		response := dataplaneadapter.CurrentReadResponseV1{Authorization: &authorization}
		if err != nil {
			response = dataplaneadapter.CurrentReadResponseV1{Error: &dataplaneadapter.ClosedError{Reason: "internal", Message: err.Error()}}
		}
		_ = writeWorkspaceReadExecutorFrameV1(connection, response)
		_ = connection.Close()
	}
}

func readWorkspaceReadExecutorFrameV1(reader io.Reader, value any) error {
	var length uint32
	if err := binary.Read(reader, binary.BigEndian, &length); err != nil {
		return err
	}
	if length == 0 || length > 8<<20 {
		return errors.New("workspace-read IPC frame is outside bounds")
	}
	data := make([]byte, length)
	if _, err := io.ReadFull(reader, data); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	return decoder.Decode(value)
}

func writeWorkspaceReadExecutorFrameV1(writer io.Writer, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if err = binary.Write(writer, binary.BigEndian, uint32(len(data))); err != nil {
		return err
	}
	_, err = writer.Write(data)
	return err
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

func waitForWorkspaceReadExecutorSocket(t *testing.T, path string, output *bytes.Buffer) {
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
