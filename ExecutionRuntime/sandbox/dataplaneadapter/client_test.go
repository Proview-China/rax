package dataplaneadapter

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestClientSendsExplicitDispatchAndInspectEnvelopes(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	request := fixtureRequest(t, now)
	for _, operation := range []DataPlaneOperationV1{DataPlaneDispatchV1, DataPlaneInspectV1} {
		t.Run(string(operation), func(t *testing.T) {
			expectedResponse := acceptedResponseForTest(t, request, now)
			socket := filepath.Join(t.TempDir(), "sandbox.sock")
			listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: socket, Net: "unix"})
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()

			serverErr := make(chan error, 1)
			go func() {
				connection, err := listener.AcceptUnix()
				if err != nil {
					serverErr <- err
					return
				}
				defer connection.Close()
				var envelope DataPlaneRequestV1
				if err := readFrame(connection, &envelope); err != nil {
					serverErr <- err
					return
				}
				if envelope.ContractVersion != ContractVersionV1 || envelope.Operation != operation || envelope.Request.Digest != request.Digest {
					serverErr <- ClosedError{Reason: "invalid_envelope", Message: "client changed the exact request envelope"}
					return
				}
				serverErr <- writeFrame(connection, expectedResponse)
			}()

			client := Client{SocketPath: socket, AllowedUID: uint32(os.Getuid())}
			var response DispatchResponseV1
			if operation == DataPlaneDispatchV1 {
				response, err = client.Dispatch(context.Background(), request)
			} else {
				response, err = client.Inspect(context.Background(), request)
			}
			if err != nil {
				t.Fatal(err)
			}
			if response.RequestDigest != request.Digest || !response.Accepted {
				t.Fatalf("response does not bind request: %#v", response)
			}
			if err := <-serverErr; err != nil {
				t.Fatal(err)
			}
		})
	}
}

func acceptedResponseForTest(t *testing.T, request DispatchRequestV1, now time.Time) DispatchResponseV1 {
	t.Helper()
	expires := request.RequestedNotAfterUnixNano
	attempt := ExactRefV1{ID: "wasmtime/" + request.TenantID + "/" + request.AttemptID, Revision: 1, ExpiresUnixNano: expires}
	var err error
	attempt.Digest, err = canonicalDigest("ProviderAttemptRefV1", attempt)
	if err != nil {
		t.Fatal(err)
	}
	observation := ProviderObservationV1{Provider: "wasmtime_component", Attempt: attempt, State: "prepared", PayloadDigest: request.PayloadDigest, ObservedUnixNano: now.UnixNano()}
	observation.Digest, err = canonicalDigest("ProviderObservationV1", observation)
	if err != nil {
		t.Fatal(err)
	}
	receipt := ProviderReceiptV1{Provider: observation.Provider, Attempt: attempt, Phase: string(request.Phase), ObservationDigest: observation.Digest, RecordedUnixNano: now.UnixNano(), ExpiresUnixNano: expires}
	receipt.Digest, err = canonicalDigest("ProviderReceiptV1", receipt)
	if err != nil {
		t.Fatal(err)
	}
	response := DispatchResponseV1{
		ContractVersion: ContractVersionV1, RequestID: request.RequestID, RequestDigest: request.Digest, Accepted: true,
		ProviderAttempt: &attempt, ProviderObservation: &observation, ProviderReceipt: &receipt,
		ObservationDigest: &observation.Digest, ReceiptDigest: &receipt.Digest,
		CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires,
	}
	response.Digest, err = canonicalDigest("DispatchResponseV1", response)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func TestCheckpointExecuteResponseRequiresStructuredOpaqueArtifact(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	request := fixtureRequest(t, now)
	request.EffectKind = CheckpointEffectKindV1
	request.Phase = PhaseExecute
	request.RuntimeEnforcement.Phase = PhaseExecute
	request.RuntimeEnforcement.JournalRevision = 2
	request.Digest = ""
	var err error
	request.Digest, err = request.digestV1()
	if err != nil {
		t.Fatal(err)
	}
	expires := request.RequestedNotAfterUnixNano
	attempt := ExactRefV1{ID: "wasmtime/" + request.TenantID + "/" + request.AttemptID, Revision: 2, ExpiresUnixNano: expires}
	attempt.Digest, err = canonicalDigest("ProviderAttemptRefV1", attempt)
	if err != nil {
		t.Fatal(err)
	}
	subject := digestForTest(t, "checkpoint-subject")
	observation := ProviderObservationV1{
		Provider: "wasmtime_component", Attempt: attempt, State: "checkpoint_prepared",
		PayloadDigest: request.PayloadDigest, ObservedUnixNano: now.UnixNano(),
		CheckpointArtifact: &CheckpointArtifactObservationV1{
			ContractVersion: "praxis.sandbox/checkpoint-artifact-observation/v1",
			ArtifactID:      "praxis-checkpoint:" + strings.TrimPrefix(subject, "sha256:"),
			SubjectDigest:   subject, ContentDigest: digestForTest(t, "checkpoint-content"),
			ContentLength: 1,
			State:         "prepared", CheckpointPhase: "checkpoint_prepare",
			RecordedUnixNano: now.UnixNano(), ExpiresUnixNano: expires,
		},
	}
	response := sealCheckpointResponseForTest(t, request, observation, now)
	if err := response.Validate(request); err != nil {
		t.Fatalf("structured checkpoint artifact rejected: %v", err)
	}

	missing := observation
	missing.CheckpointArtifact = nil
	if err := sealCheckpointResponseForTest(t, request, missing, now).Validate(request); err == nil {
		t.Fatal("self-consistent checkpoint response without artifact was accepted")
	}
	drift := observation
	drift.CheckpointArtifact = &CheckpointArtifactObservationV1{
		ContractVersion: observation.CheckpointArtifact.ContractVersion,
		ArtifactID:      observation.CheckpointArtifact.ArtifactID,
		SubjectDigest:   observation.CheckpointArtifact.SubjectDigest,
		ContentDigest:   "sha256:bad", State: "prepared", CheckpointPhase: "checkpoint_prepare",
		ContentLength:    1,
		RecordedUnixNano: now.UnixNano(), ExpiresUnixNano: expires,
	}
	if err := sealCheckpointResponseForTest(t, request, drift, now).Validate(request); err == nil {
		t.Fatal("self-consistent checkpoint response with invalid artifact digest was accepted")
	}
}

func TestWorkspaceCommitResponseRequiresExactStructuredObservation(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	request := fixtureRequest(t, now)
	expires := request.RequestedNotAfterUnixNano
	payload, err := NewWorkspaceCommitPayload(WorkspaceCommitPayloadV1{
		WorkspaceBindingID: "workspace-1", WorkspaceDigest: digestForTest(t, "workspace-binding"),
		ChangeSet:    ExactRefV1{ID: "changes-1", Revision: 1, Digest: digestForTest(t, "changes"), ExpiresUnixNano: expires},
		View:         ExactRefV1{ID: "view-1", Revision: 1, Digest: digestForTest(t, "view"), ExpiresUnixNano: expires},
		BaseRevision: digestForTest(t, "base"), FileScopeDigest: digestForTest(t, "scope"),
		WriteScopes: []string{"src/generated"},
		Changes:     []WorkspaceMutationV1{{Kind: "delete", Path: "src/generated/old.go"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	request.Payload = payload
	request.PayloadDigest, err = canonicalDigest("ProviderPayloadV1", payload)
	if err != nil {
		t.Fatal(err)
	}
	request.EffectKind = "praxis.sandbox/workspace-commit"
	request.Phase = PhaseExecute
	request.RuntimeEnforcement.Phase = PhaseExecute
	request.RuntimeEnforcement.JournalRevision = 2
	request.Digest, err = request.digestV1()
	if err != nil {
		t.Fatal(err)
	}
	attempt := ExactRefV1{ID: "workspace-commit/" + request.TenantID + "/" + request.AttemptID, Revision: 2, ExpiresUnixNano: expires}
	attempt.Digest, err = canonicalDigest("ProviderAttemptRefV1", attempt)
	if err != nil {
		t.Fatal(err)
	}
	var exact WorkspaceCommitPayloadV1
	if err := json.Unmarshal(payload.ProviderPayload, &exact); err != nil {
		t.Fatal(err)
	}
	observation := ProviderObservationV1{
		Provider: "workspace_commit", Attempt: attempt, State: "workspace_committed:" + digestForTest(t, "committed"), PayloadDigest: request.PayloadDigest, ObservedUnixNano: now.UnixNano(),
		WorkspaceCommit: &WorkspaceCommitObservationV1{ContractVersion: "praxis.sandbox/workspace-commit-observation/v1", ChangeSet: exact.ChangeSet, View: exact.View, BaseRevision: exact.BaseRevision, CommittedRevision: digestForTest(t, "committed"), State: "committed", RecordedUnixNano: now.UnixNano(), ExpiresUnixNano: expires},
	}
	response := sealCheckpointResponseForTest(t, request, observation, now)
	if err := response.Validate(request); err != nil {
		t.Fatalf("exact workspace response rejected: %v", err)
	}
	missing := observation
	missing.WorkspaceCommit = nil
	if err := sealCheckpointResponseForTest(t, request, missing, now).Validate(request); err == nil {
		t.Fatal("workspace response without structured observation was accepted")
	}
	drift := observation
	drift.WorkspaceCommit = &WorkspaceCommitObservationV1{
		ContractVersion: observation.WorkspaceCommit.ContractVersion, ChangeSet: observation.WorkspaceCommit.ChangeSet,
		View: observation.WorkspaceCommit.View, BaseRevision: observation.WorkspaceCommit.BaseRevision,
		CommittedRevision: digestForTest(t, "another-commit"), State: "committed",
		RecordedUnixNano: now.UnixNano(), ExpiresUnixNano: expires,
	}
	drift.WorkspaceCommit.ChangeSet.Digest = digestForTest(t, "another-change-set")
	if err := sealCheckpointResponseForTest(t, request, drift, now).Validate(request); err == nil {
		t.Fatal("self-consistent workspace response with another ChangeSet was accepted")
	}
}

func sealCheckpointResponseForTest(t *testing.T, request DispatchRequestV1, observation ProviderObservationV1, now time.Time) DispatchResponseV1 {
	t.Helper()
	var err error
	observation.Digest = ""
	observation.Digest, err = canonicalDigest("ProviderObservationV1", observation)
	if err != nil {
		t.Fatal(err)
	}
	receipt := ProviderReceiptV1{Provider: observation.Provider, Attempt: observation.Attempt, Phase: string(request.Phase), ObservationDigest: observation.Digest, RecordedUnixNano: now.UnixNano(), ExpiresUnixNano: observation.Attempt.ExpiresUnixNano}
	receipt.Digest, err = canonicalDigest("ProviderReceiptV1", receipt)
	if err != nil {
		t.Fatal(err)
	}
	response := DispatchResponseV1{ContractVersion: ContractVersionV1, RequestID: request.RequestID, RequestDigest: request.Digest, Accepted: true, ProviderAttempt: &observation.Attempt, ProviderObservation: &observation, ProviderReceipt: &receipt, ObservationDigest: &observation.Digest, ReceiptDigest: &receipt.Digest, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: observation.Attempt.ExpiresUnixNano}
	response.Digest, err = canonicalDigest("DispatchResponseV1", response)
	if err != nil {
		t.Fatal(err)
	}
	return response
}
