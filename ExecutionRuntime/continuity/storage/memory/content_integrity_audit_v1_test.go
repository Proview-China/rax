package memory_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/storage/memory"
)

func TestContentIntegrityAuditRepositoryConcurrentDifferentContentSingleWinner(t *testing.T) {
	ctx := context.Background()
	backend := memory.New()
	now := time.Date(2026, 7, 17, 17, 0, 0, 0, time.UTC)
	var winners atomic.Int32
	var conflicts atomic.Int32
	var unexpected atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			fact := contentIntegrityFactV1(t, testkit.Scope(), "content-audit-race", "content-audit-request-race", "finding-"+decimalArtifactV1(i), now)
			_, replay, err := backend.CreateContentIntegrityAuditFactV1(ctx, fact)
			switch {
			case err == nil && !replay:
				winners.Add(1)
			case contract.HasCode(err, contract.ErrRevisionConflict):
				conflicts.Add(1)
			default:
				unexpected.Add(1)
			}
		}(i)
	}
	wg.Wait()
	if winners.Load() != 1 || conflicts.Load() != 63 || unexpected.Load() != 0 {
		t.Fatalf("create-once closure winners=%d conflicts=%d unexpected=%d", winners.Load(), conflicts.Load(), unexpected.Load())
	}
}

func TestContentIntegrityAuditRepositoryTenantIsolationExactAndNoAlias(t *testing.T) {
	ctx := context.Background()
	backend := memory.New()
	now := time.Date(2026, 7, 17, 17, 0, 0, 0, time.UTC)
	base := contentIntegrityFactV1(t, testkit.Scope(), "same-audit", "same-request", "object_metadata_absent", now)
	if _, _, err := backend.CreateContentIntegrityAuditFactV1(ctx, base); err != nil {
		t.Fatal(err)
	}
	otherScope := testkit.Scope()
	otherScope.TenantID = "tenant-2"
	otherScope.ExecutionScopeDigest = "tenant-2-scope"
	other := contentIntegrityFactV1(t, otherScope, "same-audit", "same-request", "object_metadata_absent", now)
	if _, _, err := backend.CreateContentIntegrityAuditFactV1(ctx, other); err != nil {
		t.Fatalf("cross-tenant same ID must be independent: %v", err)
	}
	inspected, err := backend.InspectContentIntegrityAuditV1(ctx, ports.InspectContentIntegrityAuditRequestV1{Ref: base.Ref()})
	if err != nil {
		t.Fatal(err)
	}
	inspected.Findings[0].DetailCode = "mutated"
	again, err := backend.InspectContentIntegrityAuditV1(ctx, ports.InspectContentIntegrityAuditRequestV1{Ref: base.Ref()})
	if err != nil || again.Findings[0].DetailCode == "mutated" {
		t.Fatal("historical Content Integrity Audit aliases caller memory")
	}
	tamperedRef := base.Ref().Exact()
	tamperedRef.Digest = "other-digest"
	if _, err := backend.InspectContentIntegrityAuditV1(ctx, ports.InspectContentIntegrityAuditRequestV1{Ref: contract.ContentIntegrityAuditRefV1(tamperedRef)}); !contract.HasCode(err, contract.ErrRevisionConflict) {
		t.Fatalf("non-exact ref accepted: %v", err)
	}
}

func contentIntegrityFactV1(t *testing.T, scope contract.Scope, auditID, requestID, detail string, now time.Time) contract.ContentIntegrityAuditFactV1 {
	t.Helper()
	subjects := []contract.ContentIntegritySubjectV1{{ObjectID: "object-1", JournalID: "journal-1"}}
	request := ports.CreateContentIntegrityAuditRequestV1{AuditID: auditID, IdempotencyKey: requestID, Scope: scope, Subjects: subjects}
	requestDigest, err := request.CanonicalDigest()
	if err != nil {
		t.Fatal(err)
	}
	fact, err := contract.NewContentIntegrityAuditFactV1(auditID, requestID, requestDigest, scope, testkit.ContentIntegrityAuditOwnerV1(), subjects, []contract.ContentIntegrityFindingV1{{
		Subject: subjects[0], Classification: contract.ContentIntegrityMetadataAbsent, DetailCode: detail,
	}}, now)
	if err != nil {
		t.Fatal(err)
	}
	return fact
}
