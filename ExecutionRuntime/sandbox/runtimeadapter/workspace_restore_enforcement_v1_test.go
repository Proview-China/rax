package runtimeadapter

import (
	"context"
	"strings"
	"testing"
	"time"

	runtimecore "github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

func TestWorkspaceRestorePreparedCurrentAdapterV1LostReplyCurrentAndSplice(t *testing.T) {
	base := restoreStageAdapterFixtureV1(t)
	stable, err := base.request.StableKeyDigest()
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := contract.SealWorkspaceRestoreAttemptV1(contract.WorkspaceRestoreAttemptV1{Meta: contract.Meta{ID: base.request.DispatchAttemptID, Revision: 1, CreatedUnixNano: base.now.Add(-time.Second).UnixNano(), UpdatedUnixNano: base.now.Add(-time.Second).UnixNano(), ExpiresUnixNano: base.now.Add(time.Minute).UnixNano()}, StableKeyDigest: stable, Request: base.request, BundleProjectionDigest: strings.Repeat("c", contract.DigestSizeHex), BundleDigest: strings.Repeat("d", contract.DigestSizeHex), State: contract.WorkspaceRestoreAttemptPreparedV1})
	if err != nil {
		t.Fatal(err)
	}
	reader := &workspaceRestorePreparedAttemptReaderFakeV1{attempt: attempt}
	bindings := NewMemoryWorkspaceRestorePreparedRuntimeBindingStoreV1()
	bindings.LoseNextCreateReplyV1()
	adapter, err := NewWorkspaceRestorePreparedCurrentAdapterV1(reader, bindings, func() time.Time { return base.now })
	if err != nil {
		t.Fatal(err)
	}
	provider := runtimeports.ProviderBindingRefV2{BindingSetID: "sandbox-binding", BindingSetRevision: 1, ComponentID: "praxis/sandbox", ManifestDigest: runtimecore.DigestBytes([]byte("sandbox-manifest")), ArtifactDigest: runtimecore.DigestBytes([]byte("sandbox-artifact")), Capability: "sandbox/workspace-restore-stage"}
	request := runtimeports.InspectRestoreStageSandboxCurrentRequestV1{Operation: base.runtime.Operation, EffectID: base.runtime.EffectID, IntentRevision: base.runtime.EffectRevision, IntentDigest: base.runtime.IntentDigest, DispatchAttempt: base.runtime.DispatchAttempt, SandboxAttempt: runtimeFactRef(attempt.Meta), RestoreAttempt: base.runtime.RestoreAttempt, Eligibility: base.runtime.Eligibility, Identity: base.runtime.Identity, SnapshotArtifact: base.runtime.SnapshotArtifact, Provider: provider}
	projection, err := adapter.BindWorkspaceRestorePreparedRuntimeV1(context.Background(), attempt.ExactRef(), request)
	if err != nil || projection.ValidateCurrent(base.now) != nil || projection.Prepared.SandboxAttempt != request.SandboxAttempt || projection.Prepared.DispatchAttempt != request.DispatchAttempt {
		t.Fatalf("projection=%+v err=%v", projection, err)
	}
	current, err := adapter.InspectRestoreStageSandboxCurrentV1(context.Background(), request)
	if err != nil || current != projection {
		t.Fatalf("current=%+v err=%v", current, err)
	}
	splice := request
	splice.Identity.TargetFenceEpoch++
	if _, err := adapter.InspectRestoreStageSandboxCurrentV1(context.Background(), splice); err == nil {
		t.Fatal("Fence-spliced Restore Stage Sandbox current was accepted")
	}
	reader.attempt.BundleDigest = strings.Repeat("e", contract.DigestSizeHex)
	reader.attempt, _ = contract.SealWorkspaceRestoreAttemptV1(reader.attempt)
	if _, err := adapter.InspectRestoreStageSandboxCurrentV1(context.Background(), request); err == nil {
		t.Fatal("mutated prepared Attempt remained current")
	}
}

type workspaceRestorePreparedAttemptReaderFakeV1 struct {
	attempt contract.WorkspaceRestoreAttemptV1
}

func (r *workspaceRestorePreparedAttemptReaderFakeV1) InspectWorkspaceRestoreAttemptV1(_ context.Context, ref contract.SnapshotArtifactExactRefV2) (contract.WorkspaceRestoreAttemptV1, error) {
	if ref != r.attempt.ExactRef() {
		return contract.WorkspaceRestoreAttemptV1{}, ports.ErrNotFound
	}
	return r.attempt.Clone(), nil
}
