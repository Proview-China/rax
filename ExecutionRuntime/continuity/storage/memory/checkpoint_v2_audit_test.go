package memory_test

import (
	"context"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/storage/memory"
)

func TestAuditRepositoryStructuredTenantScopeKeyPreventsCrossRead(t *testing.T) {
	ctx := context.Background()
	backend := memory.New()
	tenantOne := testkit.ManifestV2(contract.ManifestCollecting, 1)
	tenantTwo := testkit.RetargetManifestScopeV2(tenantOne, "tenant-2", "execution-scope-tenant-2")
	if _, _, err := backend.CreateCheckpointManifestFactV2(ctx, tenantOne); err != nil {
		t.Fatal(err)
	}
	if _, _, err := backend.CreateCheckpointManifestFactV2(ctx, tenantTwo); err != nil {
		t.Fatalf("same ID in independent tenant scope rejected: %v", err)
	}
	for _, manifest := range []contract.CheckpointManifestFactV2{tenantOne, tenantTwo} {
		got, err := backend.InspectCurrentCheckpointManifestV2(ctx, testkit.CurrentManifestRequestV2(manifest))
		if err != nil || got.Scope.TenantID != manifest.Scope.TenantID || got.Scope.ExecutionScopeDigest != manifest.Scope.ExecutionScopeDigest {
			t.Fatalf("tenant current read crossed key: got=%#v err=%v", got, err)
		}
	}
	cross := testkit.CurrentManifestRequestV2(tenantOne)
	cross.ScopeDigest = tenantTwo.Scope.ExecutionScopeDigest
	if _, err := backend.InspectCurrentCheckpointManifestV2(ctx, cross); !contract.HasCode(err, contract.ErrNotFound) {
		t.Fatalf("mixed tenant/scope key read succeeded: %v", err)
	}
	wrongOwner := testkit.CurrentManifestRequestV2(tenantOne)
	wrongOwner.Owner.BindingRevision++
	if _, err := backend.InspectCurrentCheckpointManifestV2(ctx, wrongOwner); !contract.HasCode(err, contract.ErrRevisionConflict) {
		t.Fatalf("current reader ignored expected owner: %v", err)
	}
	splicedRef := tenantOne.Ref().Exact()
	splicedRef.TenantID = tenantTwo.Scope.TenantID
	if _, err := backend.InspectCheckpointManifestV2(ctx, ports.InspectCheckpointManifestRequestV2{Ref: contract.CheckpointManifestRefV2(splicedRef)}); !contract.HasCode(err, contract.ErrNotFound) {
		t.Fatalf("historical reader crossed tenant key: %v", err)
	}
}

func TestAuditRepositoryAllowsIndependentSameIDSealsAcrossTenants(t *testing.T) {
	ctx := context.Background()
	backend := memory.New()
	tenantOneInitial := testkit.ManifestV2(contract.ManifestCollecting, 1)
	tenantTwoInitial := testkit.RetargetManifestScopeV2(tenantOneInitial, "tenant-2", "execution-scope-tenant-2")
	for _, initial := range []contract.CheckpointManifestFactV2{tenantOneInitial, tenantTwoInitial} {
		if _, _, err := backend.CreateCheckpointManifestFactV2(ctx, initial); err != nil {
			t.Fatalf("create tenant %s manifest: %v", initial.Scope.TenantID, err)
		}
	}
	tenantOneFinal := testkit.ManifestV2(contract.ManifestVerifiedCandidate, 2)
	tenantTwoFinal := testkit.RetargetManifestScopeV2(tenantOneFinal, "tenant-2", "execution-scope-tenant-2")
	for _, transition := range []struct {
		initial contract.CheckpointManifestFactV2
		final   contract.CheckpointManifestFactV2
	}{{tenantOneInitial, tenantOneFinal}, {tenantTwoInitial, tenantTwoFinal}} {
		if _, _, err := backend.CompareAndSwapCheckpointManifestFactV2(ctx, transition.initial.Ref(), transition.final); err != nil {
			t.Fatalf("finalize tenant %s manifest: %v", transition.final.Scope.TenantID, err)
		}
	}

	seals := []contract.CheckpointManifestSealFactV2{testkit.SealV2(tenantOneFinal), testkit.SealV2(tenantTwoFinal)}
	for _, seal := range seals {
		if _, replay, err := backend.CreateCheckpointManifestSealFactV2(ctx, seal); err != nil || replay {
			t.Fatalf("same Seal ID in independent tenant %s failed: replay=%v err=%v", seal.TenantID, replay, err)
		}
		got, err := backend.InspectCheckpointManifestSealV2(ctx, testkit.InspectSealRequestV2(seal.Ref()))
		if err != nil || got.Ref() != seal.Ref() {
			t.Fatalf("tenant %s exact Seal Inspect crossed identity: got=%#v err=%v", seal.TenantID, got, err)
		}
	}
	cross := seals[0].Ref().Exact()
	cross.TenantID = seals[1].TenantID
	if _, err := backend.InspectCheckpointManifestSealV2(ctx, ports.InspectCheckpointManifestSealRequestV2{
		Ref: contract.CheckpointManifestSealRefV2(cross),
	}); !contract.HasCode(err, contract.ErrNotFound) {
		t.Fatalf("cross-tenant Seal ref read another tenant: %v", err)
	}
}

func TestAuditDirectRepositorySealMutationCannotBypassOwnerChecks(t *testing.T) {
	ctx := context.Background()
	backend := memory.New()
	initial := testkit.ManifestV2(contract.ManifestCollecting, 1)
	if _, _, err := backend.CreateCheckpointManifestFactV2(ctx, initial); err != nil {
		t.Fatal(err)
	}
	if _, _, err := backend.CreateCheckpointManifestSealFactV2(ctx, testkit.SealV2(initial)); !contract.HasCode(err, contract.ErrCheckpointPartial) {
		t.Fatalf("direct repository sealed collecting manifest: %v", err)
	}
	final := testkit.ManifestV2(contract.ManifestVerifiedCandidate, 2)
	if _, _, err := backend.CompareAndSwapCheckpointManifestFactV2(ctx, initial.Ref(), final); err != nil {
		t.Fatal(err)
	}
	if _, _, err := backend.CreateCheckpointManifestSealFactV2(ctx, testkit.SealV2(initial)); !contract.HasCode(err, contract.ErrRevisionConflict) {
		t.Fatalf("direct repository sealed stale manifest revision: %v", err)
	}
	changed := testkit.SealV2(final)
	changed.BarrierRef.ID = "other-barrier"
	testkit.RefreshSealV2(&changed)
	if _, _, err := backend.CreateCheckpointManifestSealFactV2(ctx, changed); !contract.HasCode(err, contract.ErrRevisionConflict) {
		t.Fatalf("direct repository accepted changed Seal binding: %v", err)
	}
	valid := testkit.SealV2(final)
	if _, _, err := backend.CreateCheckpointManifestSealFactV2(ctx, valid); err != nil {
		t.Fatalf("valid direct repository seal rejected: %v", err)
	}
	changed = valid.Clone()
	changed.ParticipantClosures[0].EvidenceRefs[0].Digest = "other-evidence-digest"
	testkit.RefreshSealV2(&changed)
	if _, _, err := backend.CreateCheckpointManifestSealFactV2(ctx, changed); !contract.HasCode(err, contract.ErrRevisionConflict) {
		t.Fatalf("direct repository overwrote immutable Seal: %v", err)
	}
}
