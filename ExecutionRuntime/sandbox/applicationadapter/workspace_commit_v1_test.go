package applicationadapter

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/dataplaneadapter"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/kernel"
)

func TestWorkspaceCommitBindingsRequireExactCurrentFactsInBothPhases(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	plan, reservation := lifecyclePlanValidationFixtureV4(t, now, contract.EffectWorkspaceCommit)
	view := testkit.WorkspaceView()
	view.BaseRevision = "sha256:" + testkit.Ref("workspace-base").Digest
	change := contract.WorkspaceChange{Kind: contract.WorkspaceAdd, Path: "src/generated/commit.go", BlobRef: workspaceRefPtrV1(testkit.Ref("workspace-blob-" + strings.Repeat("a", 64)))}
	set, err := kernel.StageWorkspaceChangeSet(testkit.FixedNow, testkit.FixedNow.Add(time.Hour), "workspace-current", view, []contract.WorkspaceChange{change})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := dataplaneadapter.NewWorkspaceCommitPayload(dataplaneadapter.WorkspaceCommitPayloadV1{
		WorkspaceBindingID: "workspace-1", WorkspaceDigest: "sha256:" + testkit.Ref("binding").Digest,
		ChangeSet:    dataplaneadapter.ExactRefV1{ID: set.Meta.ID, Revision: set.Meta.Revision, Digest: "sha256:" + set.Meta.Digest, ExpiresUnixNano: set.Meta.ExpiresUnixNano},
		View:         dataplaneadapter.ExactRefV1{ID: view.Meta.ID, Revision: view.Meta.Revision, Digest: "sha256:" + view.Meta.Digest, ExpiresUnixNano: view.Meta.ExpiresUnixNano},
		BaseRevision: view.BaseRevision, FileScopeDigest: "sha256:" + view.FileScopeDigest, WriteScopes: append([]string(nil), view.WriteScopes...),
		Changes: []dataplaneadapter.WorkspaceMutationV1{{Kind: "add", Path: change.Path, BlobID: change.BlobRef.ID, BlobDigest: "sha256:" + change.BlobRef.Digest, Mode: 0o600}},
	})
	if err != nil {
		t.Fatal(err)
	}
	plan.Prepare.Payload = payload
	plan.Execute.Payload = payload
	if err := validateWorkspaceCommitBindingsV1(reservation, view, set, plan.Prepare, plan.Execute); err != nil {
		t.Fatalf("exact current rejected: %v", err)
	}

	var drift dataplaneadapter.WorkspaceCommitPayloadV1
	if err := json.Unmarshal(plan.Execute.Payload.ProviderPayload, &drift); err != nil {
		t.Fatal(err)
	}
	drift.ChangeSet.Revision++
	plan.Execute.Payload, err = dataplaneadapter.NewWorkspaceCommitPayload(drift)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateWorkspaceCommitBindingsV1(reservation, view, set, plan.Prepare, plan.Execute); err == nil {
		t.Fatal("cross-phase ChangeSet drift was accepted")
	}

	plan.Execute.Payload = plan.Prepare.Payload
	leaseDrift := view
	leaseDrift.Lease.FenceEpoch++
	if err := validateWorkspaceCommitBindingsV1(reservation, leaseDrift, set, plan.Prepare, plan.Execute); err == nil {
		t.Fatal("workspace fence drift was accepted")
	}
}

func TestWorkspaceCommitRecoveryOnlyAcceptsExactAppendOnlySuccessor(t *testing.T) {
	view := testkit.WorkspaceView()
	change := contract.WorkspaceChange{Kind: contract.WorkspaceAdd, Path: "src/generated/recovery.go", BlobRef: workspaceRefPtrV1(testkit.Ref("workspace-recovery-blob"))}
	staged, err := kernel.StageWorkspaceChangeSet(testkit.FixedNow, testkit.FixedNow.Add(time.Hour), "workspace-recovery", view, []contract.WorkspaceChange{change})
	if err != nil {
		t.Fatal(err)
	}
	reservation := testkit.Reservation(contract.EffectWorkspaceCommit, 1, "workspace-recovery")
	observation := testkit.Observation(reservation, 1, "workspace-recovery")
	inspection := testkit.Inspection(reservation, observation, contract.DispositionConfirmedApplied, "workspace-recovery")
	inspection.WorkspaceChangeSetRef = workspaceRefPtrV1(staged.Meta.Ref())
	result := testkit.Result(reservation, inspection, contract.DomainResultPayload{WorkspaceChangeSetRef: workspaceRefPtrV1(staged.Meta.Ref())}, "workspace-recovery")
	committed, err := contract.ApplyWorkspaceCommitSettlement(testkit.FixedNow, staged, result, testkit.Settlement(result, "workspace-recovery"), "sha256:"+strings.Repeat("c", 64))
	if err != nil {
		t.Fatal(err)
	}
	if !isExactWorkspaceCommitSuccessorV1(staged, committed) {
		t.Fatal("exact committed successor rejected")
	}
	drift := committed
	drift.BaseRevision = "sha256:" + strings.Repeat("d", 64)
	if isExactWorkspaceCommitSuccessorV1(staged, drift) {
		t.Fatal("base drift accepted as committed successor")
	}
	ABA := committed
	ABA.Meta.Revision = staged.Meta.Revision
	if isExactWorkspaceCommitSuccessorV1(staged, ABA) {
		t.Fatal("ABA revision accepted as committed successor")
	}
}

func workspaceRefPtrV1(value contract.Ref) *contract.Ref { return &value }
