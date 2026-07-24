package kernel_test

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/kernel"
)

func TestWorkspaceOverlayChangeSetIsPureAndScoped(t *testing.T) {
	t.Parallel()
	view := testkit.WorkspaceView()
	change := contract.WorkspaceChange{Kind: contract.WorkspaceAdd, Path: "src/generated/file.go", BlobRef: ptrRef(testkit.Ref("blob"))}
	set, err := kernel.StageWorkspaceChangeSet(testkit.FixedNow, testkit.FixedNow.Add(time.Hour), "changeset-1", view, []contract.WorkspaceChange{change})
	if err != nil {
		t.Fatal(err)
	}
	if set.State != contract.ChangeSetStaged || set.RuntimeSettlement != nil || set.CommittedRevision != "" {
		t.Fatalf("staged overlay claimed external commit: %#v", set)
	}

	change.Path = "src/generated/private/secret.txt"
	if _, err := kernel.StageWorkspaceChangeSet(testkit.FixedNow, testkit.FixedNow.Add(time.Hour), "changeset-2", view, []contract.WorkspaceChange{change}); err == nil {
		t.Fatal("hidden path was accepted")
	}
	change.Path = "../outside"
	if _, err := kernel.StageWorkspaceChangeSet(testkit.FixedNow, testkit.FixedNow.Add(time.Hour), "changeset-3", view, []contract.WorkspaceChange{change}); err == nil {
		t.Fatal("escaping path was accepted")
	}
}

func TestNoGoWorkspaceCannotForgeCommittedState(t *testing.T) {
	t.Parallel()
	view := testkit.WorkspaceView()
	change := contract.WorkspaceChange{Kind: contract.WorkspaceAdd, Path: "src/generated/file.go", BlobRef: ptrRef(testkit.Ref("blob"))}
	set, err := kernel.StageWorkspaceChangeSet(testkit.FixedNow, testkit.FixedNow.Add(time.Hour), "changeset-forged", view, []contract.WorkspaceChange{change})
	if err != nil {
		t.Fatal(err)
	}
	reservation := testkit.Reservation(contract.EffectWorkspaceCommit, 1, "forged-workspace")
	observation := testkit.Observation(reservation, 1, "forged-workspace")
	inspection := testkit.Inspection(reservation, observation, contract.DispositionConfirmedApplied, "forged-workspace")
	result := testkit.Result(reservation, inspection, contract.DomainResultPayload{WorkspaceChangeSetRef: ptrRef(set.Meta.Ref())}, "forged-workspace")
	settlement := testkit.Settlement(result, "forged-workspace")
	set.State = contract.ChangeSetCommitted
	set.RuntimeSettlement = &settlement
	set.CommittedRevision = "forged-revision"
	if err := set.ValidateShape(); err == nil {
		t.Fatal("workspace change set forged committed state without domain ApplySettlement CAS")
	}
}

func TestWorkspaceCommitAdvancesOnlyThroughExactDomainSettlement(t *testing.T) {
	view := testkit.WorkspaceView()
	change := contract.WorkspaceChange{Kind: contract.WorkspaceAdd, Path: "src/generated/commit.go", BlobRef: ptrRef(testkit.Ref("blob-commit"))}
	set, err := kernel.StageWorkspaceChangeSet(testkit.FixedNow, testkit.FixedNow.Add(time.Hour), "changeset-commit", view, []contract.WorkspaceChange{change})
	if err != nil {
		t.Fatal(err)
	}
	reservation := testkit.Reservation(contract.EffectWorkspaceCommit, 1, "workspace-commit")
	observation := testkit.Observation(reservation, 1, "workspace-commit")
	inspection := testkit.Inspection(reservation, observation, contract.DispositionConfirmedApplied, "workspace-commit")
	inspection.WorkspaceChangeSetRef = ptrRef(set.Meta.Ref())
	result := testkit.Result(reservation, inspection, contract.DomainResultPayload{WorkspaceChangeSetRef: ptrRef(set.Meta.Ref())}, "workspace-commit")
	settlement := testkit.Settlement(result, "workspace-commit")
	committedRevision := "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	committed, err := contract.ApplyWorkspaceCommitSettlement(testkit.FixedNow, set, result, settlement, committedRevision)
	if err != nil {
		t.Fatal(err)
	}
	if committed.State != contract.ChangeSetCommitted || committed.Meta.Revision != 2 || committed.CommittedRevision != committedRevision || committed.RuntimeSettlement == nil {
		t.Fatalf("unexpected committed set: %#v", committed)
	}
	if _, err := contract.ApplyWorkspaceCommitSettlement(testkit.FixedNow, committed, result, settlement, committedRevision); err != nil {
		t.Fatalf("exact replay was not idempotent: %v", err)
	}
	drift := result
	drift.Payload.WorkspaceChangeSetRef = ptrRef(testkit.Ref("another-change-set"))
	if _, err := contract.ApplyWorkspaceCommitSettlement(testkit.FixedNow, set, drift, settlement, committedRevision); err == nil {
		t.Fatal("cross-ChangeSet DomainResult advanced commit")
	}
}

func TestWorkspaceChangeSetExpiryIsBoundedByUpstreamFacts(t *testing.T) {
	t.Parallel()
	change := contract.WorkspaceChange{Kind: contract.WorkspaceAdd, Path: "src/generated/file.go", BlobRef: ptrRef(testkit.Ref("blob-ttl"))}

	t.Run("over lease boundary returns zero value", func(t *testing.T) {
		view := testkit.WorkspaceView()
		limit := time.Unix(0, view.Lease.ExpiresUnixNano)
		set, err := kernel.StageWorkspaceChangeSet(testkit.FixedNow, limit.Add(time.Nanosecond), "changeset-over-lease", view, []contract.WorkspaceChange{change})
		if err == nil {
			t.Fatal("change set outlived runtime lease")
		}
		if set.Meta.ID != "" || set.Meta.Revision != 0 || len(set.Changes) != 0 {
			t.Fatalf("expiry rejection returned a partial change set: %#v", set)
		}
	})

	t.Run("equal lease boundary is accepted", func(t *testing.T) {
		view := testkit.WorkspaceView()
		limit := time.Unix(0, view.Lease.ExpiresUnixNano)
		set, err := kernel.StageWorkspaceChangeSet(testkit.FixedNow, limit, "changeset-at-lease", view, []contract.WorkspaceChange{change})
		if err != nil {
			t.Fatal(err)
		}
		if set.Meta.ExpiresUnixNano != view.Lease.ExpiresUnixNano {
			t.Fatalf("change set expiry = %d, want lease boundary %d", set.Meta.ExpiresUnixNano, view.Lease.ExpiresUnixNano)
		}
	})

	t.Run("workspace boundary wins when earlier", func(t *testing.T) {
		view := testkit.WorkspaceView()
		view.Lease.ExpiresUnixNano = view.Meta.ExpiresUnixNano + int64(time.Hour)
		limit := time.Unix(0, view.Meta.ExpiresUnixNano)
		set, err := kernel.StageWorkspaceChangeSet(testkit.FixedNow, limit, "changeset-at-view", view, []contract.WorkspaceChange{change})
		if err != nil {
			t.Fatal(err)
		}
		if set.Meta.ExpiresUnixNano != view.Meta.ExpiresUnixNano {
			t.Fatalf("change set expiry = %d, want workspace boundary %d", set.Meta.ExpiresUnixNano, view.Meta.ExpiresUnixNano)
		}
	})

	t.Run("derived fact expires exactly at boundary", func(t *testing.T) {
		view := testkit.WorkspaceView()
		limit := time.Unix(0, view.Lease.ExpiresUnixNano)
		set, err := kernel.StageWorkspaceChangeSet(testkit.FixedNow, limit, "changeset-expiry", view, []contract.WorkspaceChange{change})
		if err != nil {
			t.Fatal(err)
		}
		if err := set.ValidateCurrent(limit.Add(-time.Nanosecond)); err != nil {
			t.Fatalf("change set expired before boundary: %v", err)
		}
		if err := set.ValidateCurrent(limit); err == nil {
			t.Fatal("change set remained current at its expiry boundary")
		}
		zero, err := kernel.StageWorkspaceChangeSet(limit, limit, "changeset-after-expiry", view, []contract.WorkspaceChange{change})
		if err == nil {
			t.Fatal("expired upstream facts produced a change set")
		}
		if zero.Meta.ID != "" || len(zero.Changes) != 0 {
			t.Fatalf("expired upstream facts returned partial output: %#v", zero)
		}
	})
}

func TestWorkspaceChangeSetCanonicalDigestBindsFinalExpiry(t *testing.T) {
	t.Parallel()
	view := testkit.WorkspaceView()
	change := contract.WorkspaceChange{Kind: contract.WorkspaceAdd, Path: "src/generated/file.go", BlobRef: ptrRef(testkit.Ref("blob-seal"))}

	earlier, err := kernel.StageWorkspaceChangeSet(testkit.FixedNow, testkit.FixedNow.Add(time.Hour), "changeset-expiry-seal", view, []contract.WorkspaceChange{change})
	if err != nil {
		t.Fatal(err)
	}
	later, err := kernel.StageWorkspaceChangeSet(testkit.FixedNow, testkit.FixedNow.Add(2*time.Hour), "changeset-expiry-seal", view, []contract.WorkspaceChange{change})
	if err != nil {
		t.Fatal(err)
	}
	if earlier.Meta.Digest == later.Meta.Digest {
		t.Fatal("identical change-set content with different legal expiries produced the same digest")
	}

	tampered := earlier
	tampered.Meta.ExpiresUnixNano++
	if err := tampered.ValidateShape(); err == nil {
		t.Fatal("tampered expiry was accepted under the old canonical digest")
	}

	tampered = earlier
	tampered.BaseArtifactRef = testkit.Ref("another-base-artifact")
	if err := tampered.ValidateShape(); err == nil {
		t.Fatal("tampered base Artifact ref was accepted under the old canonical digest")
	}
}

func ptrRef(value contract.Ref) *contract.Ref { return &value }
