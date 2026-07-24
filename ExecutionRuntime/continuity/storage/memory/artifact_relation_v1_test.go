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

func TestArtifactRelationRepositoryConcurrentDifferentContentSingleWinner(t *testing.T) {
	ctx := context.Background()
	backend := memory.New()
	now := time.Date(2026, 7, 17, 16, 0, 0, 0, time.UTC)
	var winners atomic.Int32
	var conflicts atomic.Int32
	var unexpected atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			source := testkit.ArtifactSourceProjectionV1("evidence-1", "evidence-digest-1")
			source.Artifact.StorageDigest = decimalArtifactV1(i)
			fact, err := contract.NewArtifactRelationFactV1("relation-race", "request-race", testkit.Scope(), testkit.ArtifactRelationOwnerV1(), source, now)
			if err != nil {
				unexpected.Add(1)
				return
			}
			_, replay, err := backend.CreateArtifactRelationFactV1(ctx, fact)
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

func TestArtifactRelationRepositoryTenantIsolationAndNoAlias(t *testing.T) {
	ctx := context.Background()
	backend := memory.New()
	now := time.Date(2026, 7, 17, 16, 0, 0, 0, time.UTC)
	baseSource := testkit.ArtifactSourceProjectionV1("evidence-1", "evidence-digest-1")
	base, err := contract.NewArtifactRelationFactV1("same-id", "same-request", testkit.Scope(), testkit.ArtifactRelationOwnerV1(), baseSource, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := backend.CreateArtifactRelationFactV1(ctx, base); err != nil {
		t.Fatal(err)
	}

	otherScope := testkit.Scope()
	otherScope.TenantID = "tenant-2"
	otherScope.ExecutionScopeDigest = "tenant-2-scope"
	otherSource := baseSource.Clone()
	otherSource.SourceProjectionRef.TenantID = "tenant-2"
	otherSource.SourceProjectionRef.ScopeDigest = otherScope.ExecutionScopeDigest
	otherSource.Artifact.ArtifactFactRef.TenantID = "tenant-2"
	otherSource.Artifact.ArtifactFactRef.ScopeDigest = otherScope.ExecutionScopeDigest
	otherSource.Artifact.ParentRevisionRef.TenantID = "tenant-2"
	otherSource.Artifact.ParentRevisionRef.ScopeDigest = otherScope.ExecutionScopeDigest
	otherSource.RelatedFactRef.TenantID = "tenant-2"
	otherSource.RelatedFactRef.ScopeDigest = otherScope.ExecutionScopeDigest
	otherSource.ExecutionScopeDigest = otherScope.ExecutionScopeDigest
	other, err := contract.NewArtifactRelationFactV1("same-id", "same-request", otherScope, testkit.ArtifactRelationOwnerV1(), otherSource, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := backend.CreateArtifactRelationFactV1(ctx, other); err != nil {
		t.Fatalf("cross-tenant same ID must be independent: %v", err)
	}

	inspected, err := backend.InspectArtifactRelationV1(ctx, ports.InspectArtifactRelationRequestV1{Ref: base.Ref()})
	if err != nil {
		t.Fatal(err)
	}
	inspected.SourceProjection.Artifact.ParentRevisionRef.Digest = "mutated"
	again, err := backend.InspectArtifactRelationV1(ctx, ports.InspectArtifactRelationRequestV1{Ref: base.Ref()})
	if err != nil || again.SourceProjection.Artifact.ParentRevisionRef.Digest == "mutated" {
		t.Fatal("historical Artifact Relation aliases caller memory")
	}
}

func decimalArtifactV1(value int) string {
	const digits = "0123456789"
	if value == 0 {
		return "storage-digest-0"
	}
	var buf [20]byte
	i := len(buf)
	for value > 0 {
		i--
		buf[i] = digits[value%10]
		value /= 10
	}
	return "storage-digest-" + string(buf[i:])
}
