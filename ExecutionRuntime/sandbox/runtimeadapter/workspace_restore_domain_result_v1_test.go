package runtimeadapter

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	runtimecore "github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

func TestWorkspaceRestoreSettlementAdapterV1ExactRuntimeInspectAndOpaqueApply(t *testing.T) {
	fixture := newWorkspaceRestoreStageDomainFixtureV1(t)
	domain, err := fixture.adapter.BindWorkspaceRestoreStageRuntimeV1(context.Background(), fixture.bind)
	if err != nil {
		t.Fatal(err)
	}
	evidence := runtimeports.EvidenceRecordRefV2{LedgerScopeDigest: runtimecore.DigestBytes([]byte("restore-stage-ledger")), Sequence: 1, RecordDigest: runtimecore.DigestBytes([]byte("restore-stage-evidence"))}
	submission, err := runtimeports.SealRestoreStageSettlementSubmissionV1(runtimeports.RestoreStageSettlementSubmissionV1{ID: "runtime-restore-stage-settlement", Operation: fixture.current.value.Operation, OperationDigest: domain.OperationDigest, EffectID: domain.EffectID, EffectRevision: domain.EffectRevision, RestoreAttempt: domain.RestoreAttempt, Eligibility: domain.Eligibility, Governance: fixture.current.value, DomainResult: domain, Evidence: evidence, IdempotencyKey: "runtime-restore-stage-settlement-key", SettledUnixNano: fixture.now.UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	runtimeFact, err := runtimeports.SealRestoreStageSettlementFactV1(runtimeports.RestoreStageSettlementFactV1{Submission: submission})
	if err != nil {
		t.Fatal(err)
	}
	runtimeReader := &workspaceRestoreRuntimeSettlementReaderFakeV1{fact: runtimeFact}
	settlementStore := testkit.NewWorkspaceRestoreSettlementMemoryStoreV1()
	settlementStore.LoseNextCreateReplyV1()
	owner, err := kernel.NewWorkspaceRestoreSettlementOwnerV1(fixture.facts, settlementStore, func() time.Time { return fixture.now })
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := NewWorkspaceRestoreSettlementAdapterV1(runtimeReader, owner)
	if err != nil {
		t.Fatal(err)
	}
	applyOwner := runtimeports.ProviderBindingRefV2{BindingSetID: "sandbox-apply-binding", BindingSetRevision: 1, ComponentID: "praxis/sandbox", ManifestDigest: runtimecore.DigestBytes([]byte("sandbox-apply-manifest")), ArtifactDigest: runtimecore.DigestBytes([]byte("sandbox-apply-artifact")), Capability: "sandbox/workspace-restore-apply"}
	currentAdapter, err := NewWorkspaceRestoreApplySettlementCurrentAdapterV1(adapter, owner, applyOwner, func() time.Time { return fixture.now })
	if err != nil {
		t.Fatal(err)
	}
	projection, err := currentAdapter.ApplyWorkspaceRestoreStageSettlementCurrentV1(context.Background(), runtimeFact.RefV1(), fixture.fact.ExactRef())
	if err != nil {
		t.Fatal(err)
	}
	inspected, err := currentAdapter.InspectRestoreStageApplySettlementCurrentV1(context.Background(), projection.Fact)
	if err != nil || inspected.ProjectionDigest != projection.ProjectionDigest || inspected.Fact.Owner != applyOwner {
		t.Fatalf("Sandbox ApplySettlement exact current recovery failed: projection=%+v inspected=%+v err=%v", projection, inspected, err)
	}
	applied, err := owner.InspectWorkspaceRestoreApplySettlementV1(context.Background(), string(runtimeFact.RefV1().DomainResult.TenantID), projection.Fact.ID)
	if err != nil || applied.StageFactRef != fixture.fact.ExactRef() || applied.RuntimeSettlement.ID != runtimeFact.RefV1().ID {
		t.Fatalf("applied=%+v err=%v", applied, err)
	}
	if applied.RuntimeSettlement.DomainResult != fixture.fact.ExactRef() {
		t.Fatal("opaque ApplySettlement lost exact Sandbox DomainResult binding")
	}
	runtimeReader.fact.Submission.IdempotencyKey = "tampered"
	if _, err := adapter.ApplyWorkspaceRestoreStageSettlementV1(context.Background(), runtimeFact.RefV1(), fixture.fact.ExactRef()); !errors.Is(err, ports.ErrConflict) {
		t.Fatalf("tampered Runtime Settlement exact Inspect was accepted: %v", err)
	}
}

func TestWorkspaceRestoreStageDomainResultAdapterV1LostReplyCurrentAndDrift(t *testing.T) {
	fixture := newWorkspaceRestoreStageDomainFixtureV1(t)
	fixture.bindings.LoseNextCreateReplyV1()
	ref, err := fixture.adapter.BindWorkspaceRestoreStageRuntimeV1(context.Background(), fixture.bind)
	if err != nil || ref.Validate() != nil || ref.ID != fixture.fact.Meta.ID {
		t.Fatalf("bind lost reply: ref=%+v err=%v", ref, err)
	}
	current, err := fixture.adapter.InspectRestoreStageDomainResultCurrentV1(context.Background(), ref)
	if err != nil || current.Fact != ref || current.CheckedUnixNano != fixture.fact.Meta.UpdatedUnixNano {
		t.Fatalf("current=%+v err=%v", current, err)
	}
	replay, err := fixture.adapter.BindWorkspaceRestoreStageRuntimeV1(context.Background(), fixture.bind)
	if err != nil || !runtimeports.SameRestoreStageDomainResultFactRefV1(replay, ref) {
		t.Fatalf("exact replay=%+v err=%v", replay, err)
	}

	fixture.current.err = errors.New("Runtime governance unavailable")
	if _, err := fixture.adapter.InspectRestoreStageDomainResultCurrentV1(context.Background(), ref); err == nil {
		t.Fatal("unavailable Runtime current left Sandbox DomainResult current")
	}
	fixture.current.err = nil
	fixture.fact.Meta.ExpiresUnixNano--
	fixture.fact, _ = contract.SealWorkspaceRestoreStageFactV1(fixture.fact)
	fixture.facts.fact = fixture.fact
	if _, err := fixture.adapter.InspectRestoreStageDomainResultCurrentV1(context.Background(), ref); err == nil {
		t.Fatalf("mutated historical Stage Fact was accepted: %v", err)
	}
}

func TestWorkspaceRestoreStageDomainEvidenceCurrentV1IsOwnerDerivedAndStable(t *testing.T) {
	fixture := newWorkspaceRestoreStageDomainFixtureV1(t)
	ref, err := fixture.adapter.BindWorkspaceRestoreStageRuntimeV1(context.Background(), fixture.bind)
	if err != nil {
		t.Fatal(err)
	}
	first, err := fixture.adapter.InspectRestoreStageDomainEvidenceCurrentV1(context.Background(), ref)
	if err != nil || first.ValidateCurrent(fixture.now) != nil {
		t.Fatalf("first Evidence current: projection=%+v err=%v", first, err)
	}
	second, err := fixture.adapter.InspectRestoreStageDomainEvidenceCurrentV1(context.Background(), ref)
	if err != nil || second.ProjectionDigest != first.ProjectionDigest || second.Payload != first.Payload {
		t.Fatalf("Owner-derived Evidence projection was not stable: first=%+v second=%+v err=%v", first, second, err)
	}
	if first.Payload.ContentDigest != ref.PayloadDigest || first.Payload.Revision != ref.PayloadRevision || first.Payload.Length == 0 || !strings.HasPrefix(first.Payload.Ref, "sandbox-fact://workspace-restore-stage/") {
		t.Fatalf("Evidence payload does not bind the exact Sandbox fact: %+v", first.Payload)
	}

	drift := ref
	drift.PayloadDigest = runtimecore.DigestBytes([]byte("other-payload"))
	if _, err := fixture.adapter.InspectRestoreStageDomainEvidenceCurrentV1(context.Background(), drift); err == nil {
		t.Fatal("caller payload digest drift was accepted")
	}
}

func TestWorkspaceRestoreStageDomainResultAdapterV1RejectsCoordinateAndTenantSplice(t *testing.T) {
	fixture := newWorkspaceRestoreStageDomainFixtureV1(t)
	splice := fixture.bind
	splice.Governance.Eligibility.ID = "other-eligibility"
	if _, err := fixture.adapter.BindWorkspaceRestoreStageRuntimeV1(context.Background(), splice); err == nil {
		t.Fatal("Eligibility splice was accepted")
	}
	splice = fixture.bind
	splice.Governance.Operation.ExecutionScope.Instance.Epoch++
	if _, err := fixture.adapter.BindWorkspaceRestoreStageRuntimeV1(context.Background(), splice); err == nil {
		t.Fatal("target Instance splice was accepted")
	}
	var facts *workspaceRestoreStageFactReaderFakeV1
	if _, err := NewWorkspaceRestoreStageDomainResultAdapterV1(facts, fixture.bindings, fixture.current, fixture.owner, fixture.schema, func() time.Time { return fixture.now }); err == nil {
		t.Fatal("typed-nil Sandbox Fact Reader was accepted")
	}
}

type workspaceRestoreStageDomainFixtureV1 struct {
	now      time.Time
	fact     contract.WorkspaceRestoreStageFactV1
	facts    *workspaceRestoreStageFactReaderFakeV1
	bindings *MemoryWorkspaceRestoreStageRuntimeBindingStoreV1
	current  *restoreStageCurrentFakeV1
	owner    runtimeports.ProviderBindingRefV2
	schema   runtimeports.SchemaRefV2
	adapter  *WorkspaceRestoreStageDomainResultAdapterV1
	bind     BindWorkspaceRestoreStageRuntimeV1Request
}

func newWorkspaceRestoreStageDomainFixtureV1(t *testing.T) *workspaceRestoreStageDomainFixtureV1 {
	t.Helper()
	base := restoreStageAdapterFixtureV1(t)
	mapped, err := mapRestoreStageProjectionV1(base.request, base.runtime, base.now)
	if err != nil {
		t.Fatal(err)
	}
	providerAttempt := workspaceRestoreDomainExactRefV1(contract.WorkspaceRestoreAttemptTypeURLV1, contract.WorkspaceRestoreAttemptDigestDomainV1, base.request.DispatchAttemptID, 3, base.now)
	bundleDigest := strings.Repeat("c", contract.DigestSizeHex)
	root, err := contract.SealWorkspaceRootRefV1(contract.WorkspaceRootRefV1{ID: "workspace-root", TenantID: base.request.TenantID, RestoreAttemptID: base.request.RuntimeRestoreAttempt.ID, RuntimeRestoreAttempt: base.request.RuntimeRestoreAttempt, StageAttemptRef: providerAttempt, Target: base.request.Target, BundleDigest: bundleDigest})
	if err != nil {
		t.Fatal(err)
	}
	fact, err := contract.SealWorkspaceRestoreStageFactV1(contract.WorkspaceRestoreStageFactV1{Meta: contract.Meta{ID: "workspace-stage-fact", Revision: 1, CreatedUnixNano: base.now.Add(-time.Second).UnixNano(), UpdatedUnixNano: base.now.Add(-time.Second).UnixNano(), ExpiresUnixNano: base.now.Add(time.Hour).UnixNano()}, TenantID: base.request.TenantID, AttemptRef: providerAttempt, RuntimeRestoreAttempt: base.request.RuntimeRestoreAttempt, RestoreEligibility: base.request.RestoreEligibility, Target: base.request.Target, SnapshotArtifactFactRef: base.request.SnapshotArtifactFactRef, BundleDigest: bundleDigest, RootRef: root, Governance: mapped, State: contract.WorkspaceRestoreStageCompleteV1})
	if err != nil {
		t.Fatal(err)
	}
	stable, err := base.request.StableKeyDigest()
	if err != nil {
		t.Fatal(err)
	}
	factRef := fact.ExactRef()
	attempt, err := contract.SealWorkspaceRestoreAttemptV1(contract.WorkspaceRestoreAttemptV1{Meta: contract.Meta{ID: providerAttempt.ID, Revision: 4, CreatedUnixNano: base.now.Add(-2 * time.Second).UnixNano(), UpdatedUnixNano: base.now.Add(-time.Second).UnixNano(), ExpiresUnixNano: base.now.Add(time.Hour).UnixNano()}, StableKeyDigest: stable, Request: base.request, BundleProjectionDigest: strings.Repeat("d", contract.DigestSizeHex), BundleDigest: bundleDigest, GovernanceProjectionDigest: mapped.ProjectionDigest, Governance: &mapped, State: contract.WorkspaceRestoreAttemptStagedV1, RootRef: &root, StageFactRef: &factRef, ProviderStageAttemptRef: &providerAttempt})
	if err != nil {
		t.Fatal(err)
	}
	facts := &workspaceRestoreStageFactReaderFakeV1{fact: fact, attempt: attempt}
	bindings := NewMemoryWorkspaceRestoreStageRuntimeBindingStoreV1()
	owner := runtimeports.ProviderBindingRefV2{BindingSetID: "sandbox-binding", BindingSetRevision: 1, ComponentID: "praxis/sandbox", ManifestDigest: runtimecore.DigestBytes([]byte("sandbox-manifest")), ArtifactDigest: runtimecore.DigestBytes([]byte("sandbox-artifact")), Capability: "sandbox/workspace-restore-stage"}
	schema := runtimeports.SchemaRefV2{Namespace: "praxis.sandbox", Name: "workspace-restore-stage-fact", Version: "1.0.0", MediaType: "application/json", ContentDigest: runtimecore.DigestBytes([]byte("workspace-restore-stage-schema"))}
	adapter, err := NewWorkspaceRestoreStageDomainResultAdapterV1(facts, bindings, base.current, owner, schema, func() time.Time { return base.now })
	if err != nil {
		t.Fatal(err)
	}
	return &workspaceRestoreStageDomainFixtureV1{now: base.now, fact: fact, facts: facts, bindings: bindings, current: base.current, owner: owner, schema: schema, adapter: adapter, bind: BindWorkspaceRestoreStageRuntimeV1Request{StageFactRef: factRef, Governance: base.coordinates.value}}
}

type workspaceRestoreStageFactReaderFakeV1 struct {
	fact    contract.WorkspaceRestoreStageFactV1
	attempt contract.WorkspaceRestoreAttemptV1
}

type workspaceRestoreRuntimeSettlementReaderFakeV1 struct {
	fact runtimeports.RestoreStageSettlementFactV1
	err  error
}

func (r *workspaceRestoreRuntimeSettlementReaderFakeV1) InspectRestoreStageSettlementV1(_ context.Context, _ string) (runtimeports.RestoreStageSettlementFactV1, error) {
	return r.fact, r.err
}

func (r *workspaceRestoreStageFactReaderFakeV1) InspectWorkspaceRestoreAttemptByStableKeyV1(_ context.Context, stable string) (contract.WorkspaceRestoreAttemptV1, error) {
	if stable != r.attempt.StableKeyDigest {
		return contract.WorkspaceRestoreAttemptV1{}, ports.ErrNotFound
	}
	return r.attempt.Clone(), nil
}

func (r *workspaceRestoreStageFactReaderFakeV1) InspectWorkspaceRestoreStageFactV1(_ context.Context, ref contract.SnapshotArtifactExactRefV2) (contract.WorkspaceRestoreStageFactV1, error) {
	if ref != r.fact.ExactRef() {
		return contract.WorkspaceRestoreStageFactV1{}, ports.ErrNotFound
	}
	return r.fact.Clone(), nil
}

func workspaceRestoreDomainExactRefV1(typeURL, domain, id string, revision uint64, now time.Time) contract.SnapshotArtifactExactRefV2 {
	digest, _ := contract.Digest("workspace-restore-domain-result-test/v1", struct {
		TypeURL  string
		ID       string
		Revision uint64
	}{typeURL, id, revision})
	return contract.SnapshotArtifactExactRefV2{TypeURL: typeURL, Version: 1, ID: id, Revision: revision, DigestAlgorithm: contract.SnapshotArtifactDigestSHA256, DigestDomain: domain, Digest: digest, ExpiresUnixNano: now.Add(time.Hour).UnixNano()}
}
