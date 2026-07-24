package sandbox_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	runtimecore "github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/applicationadapter"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/dataplaneadapter/hostlocal"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/runtimeadapter"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/storage/sqlite"
)

func TestWorkspaceRestoreProductionV1DurableLostReplySettlementClosure(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(1_970_000_000, 0)
	clock := func() time.Time { return now }
	root := t.TempDir()
	store, err := sqlite.OpenWithClock(ctx, filepath.Join(root, "sandbox.db"), clock)
	if err != nil {
		t.Fatal(err)
	}

	fixture := workspaceRestoreProductionFixtureV1(t, now)
	stage, err := hostlocal.NewWorkspaceStageV1(hostlocal.WorkspaceStageConfigV1{RootParent: filepath.Join(root, "workspaces"), Clock: clock})
	if err != nil {
		t.Fatal(err)
	}
	provider := &workspaceRestoreLostReplyProviderV1{delegate: stage}
	settlements := &workspaceRestoreSettlementReaderV1{}
	composition, err := applicationadapter.NewWorkspaceRestoreProductionCompositionV1(applicationadapter.WorkspaceRestoreProductionConfigV1{
		Store: store, Bundles: &workspaceRestoreBundleReaderV1{value: fixture.bundle}, Coordinates: fixture.coordinates,
		RuntimeCurrent: fixture.current, RuntimeSettlements: settlements, Provider: provider,
		DomainResultOwner: fixture.owner, DomainResultSchema: fixture.schema, Clock: clock,
		ApplySettlementOwner: runtimeports.ProviderBindingRefV2{BindingSetID: "sandbox-apply-binding", BindingSetRevision: 1, ComponentID: "praxis/sandbox", ManifestDigest: runtimecore.DigestBytes([]byte("sandbox-apply-manifest")), ArtifactDigest: runtimecore.DigestBytes([]byte("sandbox-apply-artifact")), Capability: "sandbox/workspace-restore-apply"},
		Limits:               kernel.WorkspaceRestoreOwnerLimitsV1{MaxAttemptTTL: time.Hour, MaxHistoryTTL: 24 * time.Hour},
	})
	if err != nil {
		t.Fatal(err)
	}

	prepared, err := composition.Restore.PrepareWorkspaceV1(ctx, &fixture.request)
	if err != nil || prepared.State != contract.WorkspaceRestoreAttemptPreparedV1 {
		t.Fatalf("prepare=%#v err=%v", prepared, err)
	}
	sandboxCurrentRequest := fixture.sandboxCurrentRequest(prepared)
	preparedCurrent, err := composition.PreparedCurrent.BindWorkspaceRestorePreparedRuntimeV1(ctx, prepared.ExactRef(), sandboxCurrentRequest)
	if err != nil || preparedCurrent.ValidateCurrent(now) != nil {
		t.Fatalf("prepared current=%#v err=%v", preparedCurrent, err)
	}

	stageFact, err := composition.Restore.StageWorkspaceV1(ctx, &fixture.request)
	if err != nil || stageFact.State != contract.WorkspaceRestoreStageCompleteV1 || provider.stageCalls != 1 || provider.inspectCalls == 0 {
		t.Fatalf("stage=%#v provider=(%d,%d) err=%v", stageFact, provider.stageCalls, provider.inspectCalls, err)
	}
	materialized := filepath.Join(root, "workspaces", stageFact.RootRef.ID, "src", "main.go")
	content, err := os.ReadFile(materialized)
	if err != nil || string(content) != "package main\n" {
		t.Fatalf("materialized=%q err=%v", content, err)
	}

	domain, err := composition.DomainResultCurrent.BindWorkspaceRestoreStageRuntimeV1(ctx, runtimeadapter.BindWorkspaceRestoreStageRuntimeV1Request{StageFactRef: stageFact.ExactRef(), Governance: fixture.coordinates.value})
	if err != nil || domain.Validate() != nil {
		t.Fatalf("domain=%#v err=%v", domain, err)
	}
	evidence := runtimeports.EvidenceRecordRefV2{LedgerScopeDigest: runtimecore.DigestBytes([]byte("restore-ledger")), Sequence: 1, RecordDigest: runtimecore.DigestBytes([]byte("restore-record"))}
	submission, err := runtimeports.SealRestoreStageSettlementSubmissionV1(runtimeports.RestoreStageSettlementSubmissionV1{
		ID: "restore-settlement", Operation: fixture.runtime.Operation, OperationDigest: domain.OperationDigest,
		EffectID: domain.EffectID, EffectRevision: domain.EffectRevision, RestoreAttempt: domain.RestoreAttempt,
		Eligibility: domain.Eligibility, Governance: fixture.runtime, DomainResult: domain, Evidence: evidence,
		IdempotencyKey: "restore-settlement-key", SettledUnixNano: now.UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	settlements.fact, err = runtimeports.SealRestoreStageSettlementFactV1(runtimeports.RestoreStageSettlementFactV1{Submission: submission})
	if err != nil {
		t.Fatal(err)
	}
	applied, err := composition.Settlement.ApplyWorkspaceRestoreStageSettlementV1(ctx, settlements.fact.RefV1(), stageFact.ExactRef())
	if err != nil || applied.RuntimeSettlement.DomainResult != stageFact.ExactRef() {
		t.Fatalf("apply=%#v err=%v", applied, err)
	}

	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := sqlite.OpenWithClock(ctx, filepath.Join(root, "sandbox.db"), clock)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if _, err := reopened.InspectWorkspaceRestorePreparedRuntimeBindingV1(ctx, fixture.request.TenantID, prepared.Meta.ID); err != nil {
		t.Fatalf("prepared binding not durable: %v", err)
	}
	if _, err := reopened.InspectWorkspaceRestoreStageRuntimeBindingV1(ctx, fixture.request.TenantID, stageFact.Meta.ID); err != nil {
		t.Fatalf("domain binding not durable: %v", err)
	}
	if recovered, err := reopened.InspectWorkspaceRestoreApplySettlementByStageV1(ctx, fixture.request.TenantID, stageFact.ExactRef()); err != nil || recovered.ExactRef() != applied.ExactRef() {
		t.Fatalf("ApplySettlement not durable: %#v err=%v", recovered, err)
	}
}

type workspaceRestoreProductionFixture struct {
	request     contract.WorkspaceRestoreStageRequestV1
	bundle      contract.WorkspaceRestoreBundleCurrentProjectionV1
	runtime     runtimeports.RestoreStageGovernanceCurrentProjectionV1
	coordinates *workspaceRestoreCoordinatesV1
	current     *workspaceRestoreRuntimeCurrentV1
	owner       runtimeports.ProviderBindingRefV2
	schema      runtimeports.SchemaRefV2
}

func workspaceRestoreProductionFixtureV1(t *testing.T, now time.Time) *workspaceRestoreProductionFixture {
	t.Helper()
	tenant := runtimecore.TenantID("tenant-restore")
	restoreAttempt := runtimeports.RestoreAttemptRefV2{TenantID: tenant, ID: "restore-attempt", Revision: 2, Digest: runtimecore.DigestBytes([]byte("restore-attempt"))}
	eligibility := runtimeports.RestoreEligibilityRefV2{TenantID: tenant, ID: "restore-eligibility", Revision: 1, Digest: runtimecore.DigestBytes([]byte("restore-eligibility")), ExpiresUnixNano: now.Add(time.Hour).UnixNano()}
	identity := runtimeports.RestoreIdentityReservationV2{SourceInstance: runtimecore.InstanceRef{ID: "source", Epoch: 1}, TargetInstance: runtimecore.InstanceRef{ID: "target", Epoch: 2}, TargetLease: runtimecore.SandboxLeaseRef{ID: "lease", Epoch: 2}, TargetFenceEpoch: 2}
	scope := runtimecore.ExecutionScope{Identity: runtimecore.AgentIdentityRef{TenantID: tenant, ID: "identity", Epoch: 1}, Lineage: runtimecore.LineageRef{ID: "lineage", PlanDigest: runtimecore.DigestBytes([]byte("plan"))}, Instance: identity.TargetInstance, SandboxLease: &identity.TargetLease, AuthorityEpoch: identity.TargetFenceEpoch}
	scopeDigest, _ := runtimeports.ExecutionScopeDigestV2(scope)
	operation := runtimeports.OperationSubjectV3{Kind: runtimeports.RestoreStageOperationKindV1, ExecutionScope: scope, ExecutionScopeDigest: scopeDigest, CustomOperationID: restoreAttempt.ID, SubjectRevision: 1, CurrentProjectionRef: "restore-current", CurrentProjectionDigest: runtimecore.DigestBytes([]byte("current")), CurrentProjectionRevision: 7}
	operationDigest, _ := operation.DigestV3()
	intentDigest := runtimecore.DigestBytes([]byte("intent"))
	admission := runtimeports.OperationEffectAdmissionReceiptV3{OperationDigest: operationDigest, EffectID: "restore-effect", IntentRevision: 1, IntentDigest: intentDigest, FactRevision: 3, State: "accepted"}
	authorization := runtimeports.OperationReviewAuthorizationRefV4{ID: "authorization", Revision: 4, Digest: runtimecore.DigestBytes([]byte("authorization"))}
	permitDigest := runtimecore.DigestBytes([]byte("permit"))
	dispatch := runtimeports.OperationDispatchAttemptRefV3{OperationDigest: operationDigest, EffectID: admission.EffectID, IntentRevision: 1, IntentDigest: intentDigest, PermitID: "permit", PermitRevision: 5, PermitDigest: permitDigest, AttemptID: "dispatch-attempt"}
	expires := now.Add(time.Hour).UnixNano()
	enforcement := runtimeports.OperationDispatchEnforcementPhaseRefV4{OperationDigest: operationDigest, EffectID: admission.EffectID, PermitID: dispatch.PermitID, PermitFactRevision: dispatch.PermitRevision, PermitDigest: permitDigest, AdmissionDigest: runtimecore.DigestBytes([]byte("admission")), ReviewAuthorization: authorization, AttemptID: dispatch.AttemptID, SandboxAttempt: runtimeports.OperationDispatchSandboxFactRefV4{ID: dispatch.AttemptID, Revision: 1, Digest: runtimecore.DigestBytes([]byte("sandbox-attempt")), ExpiresUnixNano: expires}, Phase: runtimeports.OperationDispatchEnforcementExecuteV4, ReceiptDigest: runtimecore.DigestBytes([]byte("receipt")), JournalRevision: 2, ValidatedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: expires, PrepareReceiptDigest: runtimecore.DigestBytes([]byte("prepare")), PreparedAttemptDigest: runtimecore.DigestBytes([]byte("prepared"))}
	artifactDigest := runtimecore.DigestBytes([]byte("artifact"))
	artifact := runtimeports.CheckpointExternalExactFactRefV2{ContractVersion: "praxis.sandbox/snapshot-artifact/v2", SchemaRef: "praxis.sandbox/snapshot-artifact-schema/v2", Owner: runtimeports.CheckpointManifestSealOwnerBindingV2{BindingSetID: "sandbox-binding", BindingRevision: 1, ComponentID: "praxis/sandbox", ManifestDigest: string(runtimecore.DigestBytes([]byte("manifest"))), ArtifactDigest: string(runtimecore.DigestBytes([]byte("artifact-code"))), Capability: "snapshot-artifact-current", FactKind: "snapshot-artifact-fact"}, TenantID: string(tenant), ID: "artifact", Revision: 2, Digest: string(artifactDigest), ScopeDigest: string(scopeDigest)}
	runtimeProjection, err := runtimeports.SealRestoreStageGovernanceCurrentProjectionV1(runtimeports.RestoreStageGovernanceCurrentProjectionV1{RestoreAttempt: restoreAttempt, Eligibility: eligibility, Identity: identity, Operation: operation, EffectID: admission.EffectID, EffectRevision: 1, IntentDigest: intentDigest, Admission: admission, DispatchAdmissionDigest: enforcement.AdmissionDigest, Authorization: authorization, PermitID: dispatch.PermitID, PermitFactRevision: dispatch.PermitRevision, PermitDigest: permitDigest, BeginRecordRevision: 6, BeginRecordDigest: runtimecore.DigestBytes([]byte("begin")), DispatchAttempt: dispatch, ExecuteEnforcement: enforcement, MaterializationDigest: runtimecore.DigestBytes([]byte("materialization")), SnapshotArtifact: artifact, CheckedUnixNano: enforcement.ValidatedUnixNano, ExpiresUnixNano: expires}, now)
	if err != nil {
		t.Fatal(err)
	}
	strip := func(value string) string { return strings.TrimPrefix(value, "sha256:") }
	exact := func(typeURL, domain, id string, revision uint64, digest string) contract.SnapshotArtifactExactRefV2 {
		return contract.SnapshotArtifactExactRefV2{TypeURL: typeURL, Version: 2, ID: id, Revision: revision, DigestAlgorithm: contract.SnapshotArtifactDigestSHA256, DigestDomain: domain, Digest: strip(digest), ExpiresUnixNano: expires}
	}
	request := contract.WorkspaceRestoreStageRequestV1{TenantID: string(tenant), DispatchAttemptID: dispatch.AttemptID, RuntimeRestoreAttempt: exact("praxis.runtime/restore-attempt/v2", "praxis.runtime/restore-attempt/body/v2", restoreAttempt.ID, uint64(restoreAttempt.Revision), string(restoreAttempt.Digest)), RestoreEligibility: exact("praxis.runtime/restore-eligibility/v2", "praxis.runtime/restore-eligibility/body/v2", eligibility.ID, uint64(eligibility.Revision), string(eligibility.Digest)), Target: contract.RuntimeLeaseBinding{TenantID: string(tenant), InstanceID: string(identity.TargetInstance.ID), InstanceEpoch: uint64(identity.TargetInstance.Epoch), LeaseID: string(identity.TargetLease.ID), LeaseEpoch: uint64(identity.TargetLease.Epoch), FenceEpoch: uint64(identity.TargetFenceEpoch), ScopeDigest: string(scopeDigest), ObservedRevision: uint64(operation.CurrentProjectionRevision), ExpiresUnixNano: expires}, SnapshotArtifactFactRef: exact(contract.SnapshotArtifactFactTypeURL, contract.SnapshotArtifactFactDomain, artifact.ID, uint64(artifact.Revision), artifact.Digest), RequestedNotAfter: expires}
	bundleValue, err := contract.SealWorkspaceSnapshotBundleV1(contract.WorkspaceSnapshotBundleV1{SnapshotID: "snapshot", TenantID: string(tenant), SourceScopeDigest: string(scopeDigest), Entries: []contract.WorkspaceSnapshotEntryV1{{Path: "src", Kind: contract.WorkspaceSnapshotDirectory}, {Path: "src/main.go", Kind: contract.WorkspaceSnapshotRegularFile, Content: []byte("package main\n")}}})
	if err != nil {
		t.Fatal(err)
	}
	storage := restoreExactStorageV1(t, request, bundleValue, now)
	bundle, err := contract.SealWorkspaceRestoreBundleCurrentProjectionV1(contract.WorkspaceRestoreBundleCurrentProjectionV1{TenantID: request.TenantID, SnapshotArtifactFactRef: request.SnapshotArtifactFactRef, StorageArtifactRef: storage, Bundle: bundleValue, CheckedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: expires})
	if err != nil {
		t.Fatal(err)
	}
	coordinates := runtimeports.InspectRestoreStageGovernanceCurrentRequestV1{RestoreAttempt: restoreAttempt, Eligibility: eligibility, Operation: operation, EffectID: admission.EffectID, Admission: admission, Authorization: authorization, PermitID: dispatch.PermitID, DispatchAttempt: dispatch, ExecuteEnforcement: enforcement, SnapshotArtifact: artifact}
	owner := runtimeports.ProviderBindingRefV2{BindingSetID: "sandbox-binding", BindingSetRevision: 1, ComponentID: "praxis/sandbox", ManifestDigest: runtimecore.DigestBytes([]byte("sandbox-manifest")), ArtifactDigest: runtimecore.DigestBytes([]byte("sandbox-artifact")), Capability: "sandbox/workspace-restore-stage"}
	schema := runtimeports.SchemaRefV2{Namespace: "praxis.sandbox", Name: "workspace-restore-stage-fact", Version: "1.0.0", MediaType: "application/json", ContentDigest: runtimecore.DigestBytes([]byte("workspace-restore-stage-schema"))}
	return &workspaceRestoreProductionFixture{request: request, bundle: bundle, runtime: runtimeProjection, coordinates: &workspaceRestoreCoordinatesV1{value: coordinates}, current: &workspaceRestoreRuntimeCurrentV1{value: runtimeProjection}, owner: owner, schema: schema}
}

func (f *workspaceRestoreProductionFixture) sandboxCurrentRequest(attempt contract.WorkspaceRestoreAttemptV1) runtimeports.InspectRestoreStageSandboxCurrentRequestV1 {
	return runtimeports.InspectRestoreStageSandboxCurrentRequestV1{Operation: f.runtime.Operation, EffectID: f.runtime.EffectID, IntentRevision: f.runtime.EffectRevision, IntentDigest: f.runtime.IntentDigest, DispatchAttempt: f.runtime.DispatchAttempt, SandboxAttempt: runtimeports.OperationDispatchSandboxFactRefV4{ID: attempt.Meta.ID, Revision: runtimecore.Revision(attempt.Meta.Revision), Digest: runtimecore.Digest("sha256:" + attempt.Meta.Digest), ExpiresUnixNano: attempt.Meta.ExpiresUnixNano}, RestoreAttempt: f.runtime.RestoreAttempt, Eligibility: f.runtime.Eligibility, Identity: f.runtime.Identity, SnapshotArtifact: f.runtime.SnapshotArtifact, Provider: f.owner}
}

func restoreExactStorageV1(t *testing.T, request contract.WorkspaceRestoreStageRequestV1, bundle contract.WorkspaceSnapshotBundleV1, now time.Time) contract.SnapshotStorageArtifactRefV2 {
	t.Helper()
	encoded, err := contract.EncodeWorkspaceSnapshotBundleV1(bundle)
	if err != nil {
		t.Fatal(err)
	}
	ref := func(id string) contract.Ref {
		digest, _ := contract.Digest("restore-production-ref-v1", id)
		return contract.Ref{ID: id, Revision: 1, Digest: digest}
	}
	namespaceDigest, _ := contract.Digest("restore-production-namespace-v1", "namespace")
	namespace := contract.SnapshotArtifactExactRefV2{TypeURL: "praxis.sandbox/storage-namespace/v1", Version: 1, ID: "namespace", Revision: 1, DigestAlgorithm: contract.SnapshotArtifactDigestSHA256, DigestDomain: "storage-namespace", Digest: namespaceDigest, ExpiresUnixNano: request.RequestedNotAfter}
	sum := sha256.Sum256(encoded)
	storage, err := contract.SealSnapshotStorageArtifactRefV2(contract.SnapshotStorageArtifactRefV2{StorageArtifactID: "storage-artifact", Revision: 1, TenantID: request.TenantID, DataDomain: "workspace-checkpoint", StorageNamespaceExactRef: namespace, ContentDigest: hex.EncodeToString(sum[:]), SchemaRef: ref("workspace-bundle-schema"), Length: uint64(len(encoded)), EncryptionFactRef: ref("encryption"), ResidencyFactRef: ref("residency"), CreatedUnixNano: now.Add(-time.Minute).UnixNano(), ExpiresUnixNano: request.RequestedNotAfter})
	if err != nil {
		t.Fatal(err)
	}
	return storage
}

type workspaceRestoreBundleReaderV1 struct {
	value contract.WorkspaceRestoreBundleCurrentProjectionV1
}

func (r *workspaceRestoreBundleReaderV1) InspectWorkspaceRestoreBundleCurrentV1(context.Context, contract.WorkspaceRestoreStageRequestV1) (contract.WorkspaceRestoreBundleCurrentProjectionV1, error) {
	value := r.value
	value.Bundle = r.value.Bundle.Clone()
	return value, nil
}
func (r *workspaceRestoreBundleReaderV1) InspectWorkspaceRestoreBundleExactV1(context.Context, contract.WorkspaceRestoreStageRequestV1) (contract.WorkspaceRestoreBundleCurrentProjectionV1, error) {
	value := r.value
	value.Bundle = r.value.Bundle.Clone()
	return value, nil
}

type workspaceRestoreCoordinatesV1 struct {
	value runtimeports.InspectRestoreStageGovernanceCurrentRequestV1
}

func (r *workspaceRestoreCoordinatesV1) ReadRestoreStageCoordinatesV1(context.Context, contract.WorkspaceRestoreStageRequestV1) (runtimeports.InspectRestoreStageGovernanceCurrentRequestV1, error) {
	return r.value, nil
}

type workspaceRestoreRuntimeCurrentV1 struct {
	value runtimeports.RestoreStageGovernanceCurrentProjectionV1
}

func (r *workspaceRestoreRuntimeCurrentV1) InspectRestoreStageGovernanceCurrentV1(context.Context, runtimeports.InspectRestoreStageGovernanceCurrentRequestV1) (runtimeports.RestoreStageGovernanceCurrentProjectionV1, error) {
	return r.value, nil
}

type workspaceRestoreSettlementReaderV1 struct {
	fact runtimeports.RestoreStageSettlementFactV1
}

func (r *workspaceRestoreSettlementReaderV1) InspectRestoreStageSettlementV1(context.Context, string) (runtimeports.RestoreStageSettlementFactV1, error) {
	if r.fact.Validate() != nil {
		return runtimeports.RestoreStageSettlementFactV1{}, ports.ErrNotFound
	}
	return r.fact, nil
}

type workspaceRestoreLostReplyProviderV1 struct {
	delegate     *hostlocal.WorkspaceStageV1
	stageCalls   int
	inspectCalls int
}

func (p *workspaceRestoreLostReplyProviderV1) StageWorkspaceRestoreV1(ctx context.Context, request *contract.WorkspaceRestoreProviderRequestV1) (contract.WorkspaceRestoreProviderResultV1, error) {
	p.stageCalls++
	result, err := p.delegate.StageWorkspaceRestoreV1(ctx, request)
	if err != nil {
		return result, err
	}
	return contract.WorkspaceRestoreProviderResultV1{}, errors.New("injected lost reply")
}
func (p *workspaceRestoreLostReplyProviderV1) InspectWorkspaceRestoreV1(ctx context.Context, request *contract.WorkspaceRestoreProviderRequestV1) (contract.WorkspaceRestoreProviderResultV1, error) {
	p.inspectCalls++
	return p.delegate.InspectWorkspaceRestoreV1(ctx, request)
}

var _ ports.WorkspaceRestoreProviderV1 = (*workspaceRestoreLostReplyProviderV1)(nil)
