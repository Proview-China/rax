package hostlocal

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

func TestCheckpointWorkspaceArtifactReaderV2ExactReplayAndDrift(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(1_800_000_000, 0).UTC()
	root := t.TempDir()
	subject := checkpointTestDigestV2("subject")
	artifactRoot := filepath.Join(root, "artifacts", subject)
	staging := filepath.Join(artifactRoot, "staging")
	if err := os.MkdirAll(filepath.Join(staging, "src"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staging, "src", "main.sh"), []byte("#!/bin/sh\necho praxis\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	manifest, length, err := inspectCheckpointContentManifestV2(ctx, staging)
	if err != nil {
		t.Fatal(err)
	}
	expires := now.Add(time.Hour).UnixNano()
	record := checkpointArtifactRecordV2{
		ContractVersion: checkpointRuntimeQueryVersionV2, SubjectDigest: subject, CheckpointAttemptID: "checkpoint-attempt", ParticipantID: "sandbox-workspace", PrepareReservationID: "prepare-reservation", PrepareReservationRevision: 1,
		PrepareReservationDigest: "sha256:" + checkpointTestDigestV2("reservation"), PrepareReservationExpiresUnixNano: expires, SourceDigest: "sha256:" + checkpointTestDigestV2("source"), ContentDigest: manifest, ContentLength: length,
		State: "prepared", OperationDigest: "sha256:" + checkpointTestDigestV2("operation"), DispatchAttemptID: "dispatch-attempt", RecordedUnixNano: now.UnixNano(), ExpiresUnixNano: expires,
	}
	payload, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(artifactRoot, "current.json"), payload, 0o600); err != nil {
		t.Fatal(err)
	}
	reader, err := NewCheckpointWorkspaceArtifactReaderV2(CheckpointWorkspaceArtifactReaderConfigV2{CheckpointStoreRoot: root, Clock: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	request := contract.InspectCheckpointWorkspaceArtifactRequestV2{Observation: contract.CheckpointWorkspaceArtifactObservationV2{Provider: "host_workspace", ArtifactID: "praxis-checkpoint:" + subject, SubjectDigest: "sha256:" + subject, ContentDigest: manifest, ContentLength: length, State: "prepared", CheckpointPhase: "checkpoint_prepare", RecordedUnixNano: now.UnixNano(), ExpiresUnixNano: expires}, SnapshotID: "snapshot-1", TenantID: "tenant-1", SourceScopeDigest: checkpointTestDigestV2("scope")}
	first, err := reader.InspectCheckpointWorkspaceArtifactV2(ctx, &request)
	if err != nil || first.ValidateCurrent(now) != nil || len(first.Bundle.Entries) != 2 || !first.Bundle.Entries[1].Executable {
		t.Fatalf("checkpoint workspace inspection = %+v err=%v", first, err)
	}
	second, err := reader.InspectCheckpointWorkspaceArtifactV2(ctx, &request)
	if err != nil || !reflect.DeepEqual(second, first) {
		t.Fatalf("exact checkpoint workspace replay drifted: second=%+v err=%v", second, err)
	}
	if err := os.WriteFile(filepath.Join(staging, "src", "main.sh"), []byte("tampered\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.InspectCheckpointWorkspaceArtifactV2(ctx, &request); !errors.Is(err, ports.ErrConflict) {
		t.Fatalf("checkpoint content drift was not rejected: %v", err)
	}
}

func TestCheckpointWorkspaceArtifactReaderV2RejectsExpiryAndNonWorkspaceProvider(t *testing.T) {
	now := time.Unix(1_800_000_000, 0).UTC()
	reader, err := NewCheckpointWorkspaceArtifactReaderV2(CheckpointWorkspaceArtifactReaderConfigV2{CheckpointStoreRoot: t.TempDir(), Clock: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	digest := checkpointTestDigestV2("shape")
	request := contract.InspectCheckpointWorkspaceArtifactRequestV2{Observation: contract.CheckpointWorkspaceArtifactObservationV2{Provider: "wasmtime_component", ArtifactID: "praxis-checkpoint:" + digest, SubjectDigest: "sha256:" + digest, ContentDigest: "sha256:" + digest, ContentLength: 1, State: "prepared", CheckpointPhase: "checkpoint_prepare", RecordedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(time.Hour).UnixNano()}, SnapshotID: "snapshot", TenantID: "tenant", SourceScopeDigest: digest}
	if _, err := reader.InspectCheckpointWorkspaceArtifactV2(context.Background(), &request); err == nil {
		t.Fatal("WASM checkpoint was accepted as a workspace artifact")
	}
	request.Observation.Provider = "host_workspace"
	request.Observation.ExpiresUnixNano = now.UnixNano()
	if _, err := reader.InspectCheckpointWorkspaceArtifactV2(context.Background(), &request); err == nil {
		t.Fatal("now==expires checkpoint artifact was accepted")
	}
}

func checkpointTestDigestV2(value string) string {
	digest, err := contract.Digest("checkpoint-workspace-artifact-test/v2", value)
	if err != nil {
		panic(err)
	}
	return digest
}
