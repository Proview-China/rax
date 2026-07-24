package blackbox_test

import (
	"context"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/domain"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/storage/memory"
)

func TestWave1TimelineAndContentPublicBehavior(t *testing.T) {
	ctx := context.Background()
	clock := &testkit.Clock{Time: time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)}
	backend := memory.New()
	timeline, err := domain.NewReferenceTimeline(backend, clock, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	for i := uint64(1); i <= 3; i++ {
		record, _, err := timeline.Project(ctx, testkit.Candidate(i, i, contract.TrustObservation))
		if err != nil {
			t.Fatalf("project %d: %v", i, err)
		}
		if record.TrustClass != contract.TrustObservation {
			t.Fatalf("observation %d was upgraded", i)
		}
	}
	query := contract.TimelineQuery{
		LedgerScopeDigest: "ledger-scope-1", TenantID: "tenant-1",
		AuthorityWatermark: "authority-1", PolicyWatermark: "policy-1", PageLimit: 2,
	}
	page1, err := timeline.Query(ctx, query)
	if err != nil || len(page1.Records) != 2 || page1.Exhausted {
		t.Fatalf("page1=%#v err=%v", page1, err)
	}
	query.Cursor = page1.NextCursor
	page2, err := timeline.Query(ctx, query)
	if err != nil || len(page2.Records) != 1 || !page2.Exhausted {
		t.Fatalf("page2=%#v err=%v", page2, err)
	}
	tombstoneRequest := contract.TimelineProjectionTombstoneRequestV1{
		TombstoneID: "tombstone-fact-1", EvidenceRecordRef: "evidence-2",
		SourceTombstoneRef: "runtime-tombstone-1", PolicyBasisRef: "retention-policy-1",
		IdempotencyKey: "tombstone-request-1",
	}
	if _, duplicate, err := timeline.CreateTombstone(ctx, tombstoneRequest); err != nil || duplicate {
		t.Fatal(err)
	}
	historical, err := timeline.Inspect(ctx, "evidence-2")
	if err != nil || historical.Visibility != "visible" || historical.TombstoneRef != "" || historical.ProjectionRevision != 1 {
		t.Fatalf("historical Event was mutated: %#v err=%v", historical, err)
	}
	if _, duplicate, err := timeline.CreateTombstone(ctx, tombstoneRequest); err != nil || !duplicate {
		t.Fatalf("exact tombstone replay duplicate=%v err=%v", duplicate, err)
	}
	query.Cursor = ""
	query.PageLimit = 10
	visible, err := timeline.Query(ctx, query)
	if err != nil || len(visible.Records) != 2 {
		t.Fatalf("tombstone visibility page=%#v err=%v", visible, err)
	}
	query.IncludeTombstoned = true
	withHistory, err := timeline.Query(ctx, query)
	if err != nil || len(withHistory.Records) != 3 || withHistory.Records[1].Visibility != "tombstoned" || withHistory.Records[1].TombstoneRef != tombstoneRequest.TombstoneID {
		t.Fatalf("tombstone overlay page=%#v err=%v", withHistory, err)
	}

	content, _ := domain.NewContentManager(backend, backend, clock, 5, nil)
	payload := []byte("content-addressed-blackbox")
	manifest, journal, err := content.Put(ctx, domain.PutObjectRequest{
		JournalID: "journal-blackbox", ObjectID: "object-blackbox", SchemaVersion: "content/v1",
		Classification: "internal", OwnerID: "continuity", ScopeDigest: "scope-1",
		RetentionPolicyRef: "retention-1", Compression: "identity", Data: payload,
	})
	if err != nil || journal.State != contract.JournalClosed {
		t.Fatalf("content put journal=%#v err=%v", journal, err)
	}
	read, gotManifest, err := content.Read(ctx, manifest.ObjectID)
	if err != nil || string(read) != string(payload) || gotManifest.Digest != manifest.Digest {
		t.Fatalf("content read manifest=%#v err=%v", gotManifest, err)
	}
}

func TestPurePlansDoNotExecuteRestoreOrRewind(t *testing.T) {
	// The public Wave 1 packages expose validation objects only. Runtime Instance,
	// Sandbox, and external effect execution are intentionally absent.
	restore := contract.RestorePlan{SourceInstanceID: "instance-1", NewInstanceID: "instance-1"}
	if err := restore.Validate(time.Now()); !contract.HasCode(err, contract.ErrInvalidArgument) && !contract.HasCode(err, contract.ErrRestoreIncompatible) {
		t.Fatalf("incomplete restore plan should fail closed, got %v", err)
	}
}
