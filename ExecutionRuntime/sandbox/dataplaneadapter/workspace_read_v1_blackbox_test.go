package dataplaneadapter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestWorkspaceReadGoAdapterCallsRustDataPlane(t *testing.T) {
	binary := os.Getenv("PRAXIS_SANDBOX_DATAPLANE_BIN")
	if binary == "" {
		t.Skip("set PRAXIS_SANDBOX_DATAPLANE_BIN to run the Go to Rust IPC black box")
	}
	root, err := os.MkdirTemp("/tmp", "praxis-wr-")
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
	now := time.Now()
	dispatchSocket := filepath.Join(root, "dispatch.sock")
	currentSocket := filepath.Join(root, "current.sock")
	currentListener, err := net.ListenUnix("unix", &net.UnixAddr{Name: currentSocket, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	defer currentListener.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go serveWorkspaceReadCurrentForBlackbox(ctx, t, currentListener)

	bindingDigest := digestForTest(t, "workspace-binding")
	config := map[string]any{
		"contract_version":            "praxis.sandbox/data-plane-root/v1",
		"dispatch_socket":             dispatchSocket,
		"current_reader_socket":       currentSocket,
		"journal_path":                filepath.Join(root, "journal.jsonl"),
		"checkpoint_store_path":       filepath.Join(root, "checkpoints.sqlite"),
		"allowed_dispatch_uid":        os.Getuid(),
		"allowed_current_reader_uid":  os.Getuid(),
		"socket_mode":                 0o660,
		"wasmtime_component_bindings": map[string]string{},
		"workspace_read": map[string]any{"bindings": map[string]any{
			"workspace-1": map[string]any{"path": root, "digest": bindingDigest},
		}},
	}
	configBytes, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(root, "config.json")
	if err := os.WriteFile(configPath, configBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(ctx, binary, configPath)
	var processOutput bytes.Buffer
	command.Stdout, command.Stderr = &processOutput, &processOutput
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { cancel(); _ = command.Wait() }()
	waitForUnixSocketBlackbox(t, dispatchSocket, &processOutput)

	client := Client{SocketPath: dispatchSocket, AllowedUID: uint32(os.Getuid())}
	prepare := workspaceReadRequestForBlackbox(t, now, PhasePrepare, bindingDigest)
	if _, err := client.Dispatch(context.Background(), prepare); err != nil {
		t.Fatalf("prepare through Rust Host: %v; %s", err, processOutput.String())
	}
	execute := workspaceReadRequestForBlackbox(t, now, PhaseExecute, bindingDigest)
	adapter, err := NewWorkspaceReadActualPointAdapterV1(client)
	if err != nil {
		t.Fatal(err)
	}
	stable := digestForTest(t, "stable-key")
	result, err := adapter.dispatchSealedWorkspaceReadV1(context.Background(), execute, stable)
	if err != nil {
		t.Fatalf("execute through concrete adapter and Rust Host: %v; %s", err, processOutput.String())
	}
	if result.Content != "Praxis" || result.StartByte != 6 || result.ReturnedBytes != 6 || !result.Complete || result.PhysicalReadCount != 1 || result.ProviderReceipt.StableKeyDigest != stable {
		t.Fatalf("unexpected exact result: %#v", result)
	}
	recovered, err := client.Inspect(context.Background(), execute)
	if err != nil || recovered.ProviderObservation == nil || recovered.ProviderObservation.WorkspaceRead == nil || recovered.ProviderObservation.WorkspaceRead.Content != "Praxis" {
		t.Fatalf("exact Rust journal Inspect failed: %#v, %v", recovered, err)
	}

	outside := filepath.Join(root, "outside.txt")
	if err = os.WriteFile(outside, []byte("outside"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(outside, filepath.Join(root, "src", "link.txt")); err != nil {
		t.Fatal(err)
	}
	prepareSymlink := workspaceReadRequestForPathBlackbox(t, now, PhasePrepare, bindingDigest, "workspace-read-symlink", "attempt-symlink", "src/link.txt")
	if _, err = client.Dispatch(context.Background(), prepareSymlink); err != nil {
		t.Fatalf("prepare symlink request: %v", err)
	}
	executeSymlink := workspaceReadRequestForPathBlackbox(t, now, PhaseExecute, bindingDigest, "workspace-read-symlink", "attempt-symlink", "src/link.txt")
	_, err = client.Dispatch(context.Background(), executeSymlink)
	assertWorkspaceReadIPCActualPointEvidence(t, err, "effect_not_started", false, 0)

	if err = os.WriteFile(filepath.Join(root, "src", "non-utf8.bin"), []byte{0xff, 0xfe}, 0o640); err != nil {
		t.Fatal(err)
	}
	prepareNonUTF8 := workspaceReadRequestForPathBlackbox(t, now, PhasePrepare, bindingDigest, "workspace-read-non-utf8", "attempt-non-utf8", "src/non-utf8.bin")
	if _, err = client.Dispatch(context.Background(), prepareNonUTF8); err != nil {
		t.Fatalf("prepare non-UTF8 request: %v", err)
	}
	executeNonUTF8 := workspaceReadRequestForPathBlackbox(t, now, PhaseExecute, bindingDigest, "workspace-read-non-utf8", "attempt-non-utf8", "src/non-utf8.bin")
	_, err = client.Dispatch(context.Background(), executeNonUTF8)
	assertWorkspaceReadIPCActualPointEvidence(t, err, "effect_started_unknown", true, 1)
}

func workspaceReadRequestForBlackbox(t *testing.T, now time.Time, phase EnforcementPhaseV1, bindingDigest string) DispatchRequestV1 {
	return workspaceReadRequestForPathBlackbox(t, now, phase, bindingDigest, "workspace-read-request", "attempt-1", "src/main.txt")
}

func workspaceReadRequestForPathBlackbox(t *testing.T, now time.Time, phase EnforcementPhaseV1, bindingDigest, requestID, attemptID, relativePath string) DispatchRequestV1 {
	t.Helper()
	expires := now.Add(time.Minute).UnixNano()
	digest := func(value string) string { return digestForTest(t, value) }
	payload, err := NewWorkspaceReadPayloadV1(WorkspaceReadPayloadV1{
		WorkspaceBindingID: "workspace-1", WorkspaceDigest: bindingDigest,
		Workspace:       ExactRefV1{ID: "workspace-1", Revision: 1, Digest: digest("workspace"), ExpiresUnixNano: expires},
		FileScopeDigest: digest("scope"), RelativePath: relativePath, StartByte: 0, MaxBytes: 64, S1Checked: true,
	})
	if relativePath == "src/main.txt" {
		var decoded WorkspaceReadPayloadV1
		if decodeErr := json.Unmarshal(payload.ProviderPayload, &decoded); decodeErr != nil {
			t.Fatal(decodeErr)
		}
		decoded.StartByte = 6
		decoded.MaxBytes = 6
		payload, err = NewWorkspaceReadPayloadV1(decoded)
	}
	if err != nil {
		t.Fatal(err)
	}
	provider := ProviderBindingV1{BindingSetID: "bindings", BindingSetRevision: 1, ComponentID: "workspace-read", ManifestDigest: digest("manifest"), ArtifactDigest: digest("artifact"), Capability: "runtime/operation-scope-evidence-action"}
	provider.Digest, err = canonicalDigest("ProviderBindingV1", provider)
	if err != nil {
		t.Fatal(err)
	}
	query, _ := canonicalJSON([]byte("{\"blackbox\":true}"))
	request := DispatchRequestV1{
		ContractVersion: ContractVersionV1, RequestID: requestID, Phase: phase,
		EffectKind: "praxis.sandbox/workspace-read", OperationDigest: digest("operation"), EffectID: "effect-1",
		IntentRevision: 1, IntentDigest: digest("intent"), AttemptID: attemptID, TenantID: "tenant-1",
		ProviderBinding:     provider,
		SandboxAttempt:      ExactRefV1{ID: attemptID, Revision: 1, Digest: digest(attemptID), ExpiresUnixNano: expires},
		ExecutionBinding:    ExecutionBindingV1{TenantID: "tenant-1", InstanceID: "instance-1", InstanceEpoch: 1, LeaseID: "lease-1", LeaseEpoch: 1, FenceEpoch: 1, ScopeDigest: digest("execution-scope"), ObservedRevision: 1, ExpiresUnixNano: expires},
		RuntimeEnforcement:  RuntimeEnforcementRefV1{OperationDigest: digest("operation"), EffectID: "effect-1", PermitID: "permit-1", AttemptID: attemptID, Phase: phase, ReceiptDigest: digest("receipt-" + string(phase)), JournalRevision: map[EnforcementPhaseV1]uint64{PhasePrepare: 1, PhaseExecute: 2}[phase], ExpiresUnixNano: expires},
		RuntimeCurrentQuery: query, RequestedNotAfterUnixNano: expires,
		PayloadSchema: "praxis.sandbox/workspace-read-payload/v1", PayloadRevision: 1, Payload: payload,
	}
	request.RuntimeCurrentQueryDigest, err = canonicalDigest("RuntimeCurrentQueryV1", json.RawMessage(query))
	if err != nil {
		t.Fatal(err)
	}
	request.PayloadDigest, err = canonicalDigest("ProviderPayloadV1", payload)
	if err != nil {
		t.Fatal(err)
	}
	request.Digest, err = request.digestV1()
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func assertWorkspaceReadIPCActualPointEvidence(t *testing.T, err error, boundary string, crossed bool, physicalReads uint64) {
	t.Helper()
	var closed *ClosedError
	if !errors.As(err, &closed) {
		t.Fatalf("workspace read IPC did not return a typed closed error: %v", err)
	}
	if closed.EffectBoundary != boundary || closed.CrossedActualPoint == nil || *closed.CrossedActualPoint != crossed || closed.PhysicalReadCount == nil || *closed.PhysicalReadCount != physicalReads {
		t.Fatalf("workspace read IPC actual-point evidence drifted: %#v", closed)
	}
}

func serveWorkspaceReadCurrentForBlackbox(ctx context.Context, t *testing.T, listener *net.UnixListener) {
	for {
		connection, err := listener.AcceptUnix()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			t.Errorf("current reader accept: %v", err)
			return
		}
		var request DispatchRequestV1
		if err := readFrame(connection, &request); err != nil {
			_ = connection.Close()
			continue
		}
		now := time.Now().UnixNano()
		authorization := CurrentAuthorizationV1{
			ContractVersion: ContractVersionV1, RequestDigest: request.Digest,
			OperationDigest: request.OperationDigest, EffectID: request.EffectID, AttemptID: request.AttemptID,
			Phase: request.Phase, ProviderBinding: request.ProviderBinding,
			SandboxProjection: SandboxProjectionRefV1{Revision: 1, Digest: digestForTest(t, "projection"), ExpiresUnixNano: request.RequestedNotAfterUnixNano},
			ExecutionBinding:  request.ExecutionBinding, RuntimeEnforcement: request.RuntimeEnforcement,
			CheckedUnixNano: now, ExpiresUnixNano: request.RequestedNotAfterUnixNano,
		}
		authorization.Digest, err = canonicalDigest("CurrentAuthorizationV1", authorization)
		response := CurrentReadResponseV1{Authorization: &authorization}
		if err != nil {
			response = CurrentReadResponseV1{Error: &ClosedError{Reason: "internal", Message: err.Error()}}
		}
		_ = writeFrame(connection, response)
		_ = connection.Close()
	}
}

func waitForUnixSocketBlackbox(t *testing.T, path string, output *bytes.Buffer) {
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
