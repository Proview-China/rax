package sandbox_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	runtimecore "github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimefakes "github.com/Proview-China/rax/ExecutionRuntime/runtime/fakes"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/applicationadapter"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/dataplaneadapter/hostlocal"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/storage/sqlite"
)

func TestWorkspaceRestoreStageParticipantV1RealCompositionLostReplyAndDurability(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(2_000_000_000, 0)
	clock := func() time.Time { return now }
	request, authorized := workspaceRestoreApplicationRequestV1(t, now)
	root := t.TempDir()
	store, err := sqlite.OpenWithClock(ctx, filepath.Join(root, "sandbox.db"), clock)
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := contract.SealWorkspaceSnapshotBundleV1(contract.WorkspaceSnapshotBundleV1{SnapshotID: "participant-snapshot", TenantID: string(request.Attempt.TenantID), SourceScopeDigest: string(request.Materialization.SourceScopeDigest), Entries: []contract.WorkspaceSnapshotEntryV1{{Path: "src", Kind: contract.WorkspaceSnapshotDirectory}, {Path: "src/main.go", Kind: contract.WorkspaceSnapshotRegularFile, Content: []byte("package main\n")}}})
	if err != nil {
		t.Fatal(err)
	}
	stage, err := hostlocal.NewWorkspaceStageV1(hostlocal.WorkspaceStageConfigV1{RootParent: filepath.Join(root, "roots"), Clock: clock})
	if err != nil {
		t.Fatal(err)
	}
	provider := &workspaceRestoreLostReplyProviderV1{delegate: stage}
	runtimeCurrent := &workspaceRestoreParticipantRuntimeCurrentV1{now: now, identity: request.Materialization.Identity, materializationDigest: request.Materialization.ProjectionDigest}
	settlements := &workspaceRestoreSettlementReaderV1{}
	legacy := authorized.Dispatch.Record.Permit.LegacyPermit
	composition, err := applicationadapter.NewWorkspaceRestoreProductionCompositionV1(applicationadapter.WorkspaceRestoreProductionConfigV1{
		Store: store, Bundles: &workspaceRestoreParticipantBundleReaderV1{now: now, value: bundle}, Coordinates: store,
		RuntimeCurrent: runtimeCurrent, RuntimeSettlements: settlements, Provider: provider,
		DomainResultOwner:    legacy.EnforcementPoint,
		DomainResultSchema:   runtimeports.SchemaRefV2{Namespace: "praxis.sandbox", Name: "workspace-restore-stage-fact", Version: "1.0.0", MediaType: "application/json", ContentDigest: runtimecore.DigestBytes([]byte("workspace-restore-stage-schema"))},
		ApplySettlementOwner: workspaceRestoreParticipantProviderV1("sandbox/workspace-restore-apply"), Clock: clock,
		Limits: kernel.WorkspaceRestoreOwnerLimitsV1{MaxAttemptTTL: time.Hour, MaxHistoryTTL: 24 * time.Hour},
	})
	if err != nil {
		t.Fatal(err)
	}
	participant, err := applicationadapter.NewWorkspaceRestoreStageParticipantAdapterV1(composition, store, clock)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := participant.PrepareRestoreStageV1(ctx, request, authorized)
	if err != nil || prepared.ValidateCurrent(now) != nil {
		t.Fatalf("prepared=%+v err=%v", prepared, err)
	}
	execute := workspaceRestoreParticipantExecuteV1(t, authorized, prepared, now)
	domain, err := participant.ExecuteRestoreStageV1(ctx, request, authorized, execute)
	if err != nil || domain.Validate(now) != nil || provider.stageCalls != 1 || provider.inspectCalls == 0 {
		t.Fatalf("domain=%+v provider=(%d,%d) err=%v", domain, provider.stageCalls, provider.inspectCalls, err)
	}
	materialized := filepath.Join(root, "roots", domain.Fact.ID, "src", "main.go")
	if _, err := os.Stat(materialized); err != nil {
		// Root IDs are Sandbox-owned and are not the Runtime DomainResult ID.
		entries, globErr := filepath.Glob(filepath.Join(root, "roots", "*", "src", "main.go"))
		if globErr != nil || len(entries) != 1 {
			t.Fatalf("materialized workspace not found: %v entries=%v", err, entries)
		}
		materialized = entries[0]
	}
	content, err := os.ReadFile(materialized)
	if err != nil || string(content) != "package main\n" {
		t.Fatalf("materialized=%q err=%v", content, err)
	}
	governance := runtimeCurrent.value
	evidence := runtimeports.EvidenceRecordRefV2{LedgerScopeDigest: runtimecore.DigestBytes([]byte("participant-ledger")), Sequence: 1, RecordDigest: runtimecore.DigestBytes([]byte("participant-record"))}
	submission, err := runtimeports.SealRestoreStageSettlementSubmissionV1(runtimeports.RestoreStageSettlementSubmissionV1{ID: "participant-settlement", Operation: governance.Operation, OperationDigest: domain.Fact.OperationDigest, EffectID: domain.Fact.EffectID, EffectRevision: domain.Fact.EffectRevision, RestoreAttempt: domain.Fact.RestoreAttempt, Eligibility: domain.Fact.Eligibility, Governance: governance, DomainResult: domain.Fact, Evidence: evidence, IdempotencyKey: "participant-settlement-key", SettledUnixNano: now.UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	settlements.fact, err = runtimeports.SealRestoreStageSettlementFactV1(runtimeports.RestoreStageSettlementFactV1{Submission: submission})
	if err != nil {
		t.Fatal(err)
	}
	applied, err := participant.ApplyRestoreStageSettlementV1(ctx, settlements.fact.RefV1(), domain)
	if err != nil || applied.ValidateCurrent(now) != nil {
		t.Fatalf("apply=%+v err=%v", applied, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := sqlite.OpenWithClock(ctx, filepath.Join(root, "sandbox.db"), clock)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	workspaceRequest, err := workspaceRestoreParticipantRequestV1(request, authorized, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.ReadRestoreStageCoordinatesV1(ctx, workspaceRequest); err != nil {
		t.Fatalf("durable governance coordinates: %v", err)
	}
	stageExact := contract.SnapshotArtifactExactRefV2{TypeURL: contract.WorkspaceRestoreFactTypeURLV1, Version: 1, ID: domain.Fact.ID, Revision: uint64(domain.Fact.Revision), DigestAlgorithm: contract.SnapshotArtifactDigestSHA256, DigestDomain: contract.WorkspaceRestoreFactDigestDomainV1, Digest: strings.TrimPrefix(string(domain.Fact.Digest), "sha256:"), ExpiresUnixNano: domain.ExpiresUnixNano}
	if _, err := reopened.InspectWorkspaceRestoreApplySettlementByStageV1(ctx, string(request.Attempt.TenantID), stageExact); err != nil {
		t.Fatalf("durable ApplySettlement: %v", err)
	}
}

type workspaceRestoreParticipantBundleReaderV1 struct {
	now   time.Time
	value contract.WorkspaceSnapshotBundleV1
}

func (r *workspaceRestoreParticipantBundleReaderV1) InspectWorkspaceRestoreBundleCurrentV1(_ context.Context, request contract.WorkspaceRestoreStageRequestV1) (contract.WorkspaceRestoreBundleCurrentProjectionV1, error) {
	return r.project(request)
}
func (r *workspaceRestoreParticipantBundleReaderV1) InspectWorkspaceRestoreBundleExactV1(_ context.Context, request contract.WorkspaceRestoreStageRequestV1) (contract.WorkspaceRestoreBundleCurrentProjectionV1, error) {
	return r.project(request)
}
func (r *workspaceRestoreParticipantBundleReaderV1) project(request contract.WorkspaceRestoreStageRequestV1) (contract.WorkspaceRestoreBundleCurrentProjectionV1, error) {
	encoded, err := contract.EncodeWorkspaceSnapshotBundleV1(r.value)
	if err != nil {
		return contract.WorkspaceRestoreBundleCurrentProjectionV1{}, err
	}
	ref := func(id string) contract.Ref {
		digest, _ := contract.Digest("participant-ref-v1", id)
		return contract.Ref{ID: id, Revision: 1, Digest: digest}
	}
	namespaceDigest, _ := contract.Digest("participant-namespace-v1", request.TenantID)
	namespace := contract.SnapshotArtifactExactRefV2{TypeURL: "praxis.sandbox/storage-namespace/v1", Version: 1, ID: "participant-namespace", Revision: 1, DigestAlgorithm: contract.SnapshotArtifactDigestSHA256, DigestDomain: "storage-namespace", Digest: namespaceDigest, ExpiresUnixNano: request.RequestedNotAfter}
	sum := sha256.Sum256(encoded)
	storage, err := contract.SealSnapshotStorageArtifactRefV2(contract.SnapshotStorageArtifactRefV2{StorageArtifactID: "participant-storage", Revision: 1, TenantID: request.TenantID, DataDomain: "workspace-checkpoint", StorageNamespaceExactRef: namespace, ContentDigest: hex.EncodeToString(sum[:]), SchemaRef: ref("workspace-bundle-schema"), Length: uint64(len(encoded)), EncryptionFactRef: ref("encryption"), ResidencyFactRef: ref("residency"), CreatedUnixNano: r.now.Add(-time.Minute).UnixNano(), ExpiresUnixNano: request.RequestedNotAfter})
	if err != nil {
		return contract.WorkspaceRestoreBundleCurrentProjectionV1{}, err
	}
	return contract.SealWorkspaceRestoreBundleCurrentProjectionV1(contract.WorkspaceRestoreBundleCurrentProjectionV1{TenantID: request.TenantID, SnapshotArtifactFactRef: request.SnapshotArtifactFactRef, StorageArtifactRef: storage, Bundle: r.value, CheckedUnixNano: r.now.Add(-time.Second).UnixNano(), ExpiresUnixNano: request.RequestedNotAfter})
}

type workspaceRestoreParticipantRuntimeCurrentV1 struct {
	now                   time.Time
	identity              runtimeports.RestoreIdentityReservationV2
	materializationDigest runtimecore.Digest
	value                 runtimeports.RestoreStageGovernanceCurrentProjectionV1
}

func (r *workspaceRestoreParticipantRuntimeCurrentV1) InspectRestoreStageGovernanceCurrentV1(_ context.Context, request runtimeports.InspectRestoreStageGovernanceCurrentRequestV1) (runtimeports.RestoreStageGovernanceCurrentProjectionV1, error) {
	value, err := runtimeports.SealRestoreStageGovernanceCurrentProjectionV1(runtimeports.RestoreStageGovernanceCurrentProjectionV1{RestoreAttempt: request.RestoreAttempt, Eligibility: request.Eligibility, Identity: r.identity, Operation: request.Operation, EffectID: request.EffectID, EffectRevision: request.Admission.IntentRevision, IntentDigest: request.Admission.IntentDigest, Admission: request.Admission, DispatchAdmissionDigest: request.ExecuteEnforcement.AdmissionDigest, Authorization: request.Authorization, PermitID: request.PermitID, PermitFactRevision: request.DispatchAttempt.PermitRevision, PermitDigest: request.DispatchAttempt.PermitDigest, BeginRecordRevision: request.DispatchAttempt.PermitRevision, BeginRecordDigest: runtimecore.DigestBytes([]byte("participant-begin")), DispatchAttempt: request.DispatchAttempt, ExecuteEnforcement: request.ExecuteEnforcement, MaterializationDigest: r.materializationDigest, SnapshotArtifact: request.SnapshotArtifact, CheckedUnixNano: request.ExecuteEnforcement.ValidatedUnixNano, ExpiresUnixNano: request.ExecuteEnforcement.ExpiresUnixNano}, r.now)
	if err == nil {
		r.value = value
	}
	return value, err
}

func workspaceRestoreApplicationRequestV1(t *testing.T, now time.Time) (applicationcontract.RestoreStageActionRequestV1, applicationcontract.RestoreStageAuthorizedDispatchV1) {
	t.Helper()
	plan, err := runtimefakes.BuildRestorePlanCurrentFixtureV2("sandbox-participant", now)
	if err != nil {
		t.Fatal(err)
	}
	tenant := runtimecore.TenantID(plan.RestorePlan.TenantID)
	attempt := runtimeports.RestoreAttemptRefV2{TenantID: tenant, ID: "participant-attempt", Revision: 2, Digest: runtimecore.DigestBytes([]byte("participant-attempt"))}
	eligibility := runtimeports.RestoreEligibilityRefV2{TenantID: tenant, ID: "participant-eligibility", Revision: 1, Digest: runtimecore.DigestBytes([]byte("participant-eligibility")), ExpiresUnixNano: now.Add(time.Minute).UnixNano()}
	snapshot := workspaceRestoreParticipantExternalRefV1(tenant, plan.SourceScopeDigest, "participant-artifact", "snapshot-artifact-fact")
	materialization, err := runtimeports.SealRestoreMaterializationCurrentProjectionV1(runtimeports.RestoreMaterializationCurrentProjectionV1{Attempt: attempt, Eligibility: eligibility, RestorePlan: plan.RestorePlan, Consistency: plan.CheckpointConsistency.Ref, ManifestSeal: plan.ManifestSeal, SourceScopeDigest: plan.SourceScopeDigest, Identity: plan.IdentityProposal, ContextGeneration: workspaceRestoreParticipantExternalRefV1(tenant, plan.SourceScopeDigest, "participant-generation", "context-generation"), ContextFrames: []runtimeports.CheckpointExternalExactFactRefV2{workspaceRestoreParticipantExternalRefV1(tenant, plan.SourceScopeDigest, "participant-frame", "context-frame")}, Memory: []runtimeports.CheckpointExternalExactFactRefV2{workspaceRestoreParticipantExternalRefV1(tenant, plan.SourceScopeDigest, "participant-memory", "memory")}, Knowledge: []runtimeports.CheckpointExternalExactFactRefV2{workspaceRestoreParticipantExternalRefV1(tenant, plan.SourceScopeDigest, "participant-knowledge", "knowledge")}, Snapshots: []runtimeports.CheckpointExternalExactFactRefV2{snapshot}, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(50 * time.Second).UnixNano()}, now)
	if err != nil {
		t.Fatal(err)
	}
	request, err := applicationcontract.SealRestoreStageActionRequestV1(applicationcontract.RestoreStageActionRequestV1{ID: "participant-stage", IdempotencyKey: "participant-stage-key", Attempt: attempt, Eligibility: eligibility, Materialization: materialization, NotAfterUnixNano: now.Add(45 * time.Second).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	return request, workspaceRestoreParticipantAuthorizationV1(t, request, snapshot, now)
}

func workspaceRestoreParticipantAuthorizationV1(t *testing.T, request applicationcontract.RestoreStageActionRequestV1, snapshot runtimeports.CheckpointExternalExactFactRefV2, now time.Time) applicationcontract.RestoreStageAuthorizedDispatchV1 {
	t.Helper()
	identity := request.Materialization.Identity
	scope := runtimecore.ExecutionScope{Identity: runtimecore.AgentIdentityRef{TenantID: request.Attempt.TenantID, ID: "participant-identity", Epoch: 1}, Lineage: runtimecore.LineageRef{ID: "participant-lineage", PlanDigest: runtimecore.DigestBytes([]byte("participant-lineage"))}, Instance: identity.TargetInstance, SandboxLease: &identity.TargetLease, AuthorityEpoch: identity.TargetFenceEpoch}
	scopeDigest, _ := runtimeports.ExecutionScopeDigestV2(scope)
	operation := runtimeports.OperationSubjectV3{Kind: runtimeports.RestoreStageOperationKindV1, ExecutionScope: scope, ExecutionScopeDigest: scopeDigest, CustomOperationID: request.Attempt.ID, SubjectRevision: 1, CurrentProjectionRef: "participant-current", CurrentProjectionRevision: 1, CurrentProjectionDigest: runtimecore.DigestBytes([]byte("participant-current"))}
	opDigest, _ := operation.DigestV3()
	expires := now.Add(30 * time.Second).UnixNano()
	fact := func(id string) runtimeports.OperationGovernanceFactRefV3 {
		return runtimeports.OperationGovernanceFactRefV3{Ref: id, Revision: 1, Digest: runtimecore.DigestBytes([]byte(id)), ExpiresUnixNano: expires}
	}
	review := runtimeports.OperationReviewBindingRefV3{CaseRef: "participant-review", CandidateDigest: runtimecore.DigestBytes([]byte("participant-candidate")), CandidateRevision: 1, PolicyDigest: runtimecore.DigestBytes([]byte("participant-policy"))}
	legacyReview := runtimeports.OperationReviewAuthorizationV3{Case: fact(review.CaseRef), CandidateDigest: review.CandidateDigest, CandidateRevision: review.CandidateRevision, Verdict: fact("participant-verdict"), ReviewerAuthority: fact("participant-reviewer"), PolicyDigest: review.PolicyDigest, ExpiresUnixNano: expires}
	legacyReviewDigest, _ := runtimeports.DigestOperationReviewAuthorizationV3(legacyReview)
	authorization := runtimeports.OperationReviewAuthorizationRefV4{ID: "participant-authorization", Revision: 1, Digest: runtimecore.DigestBytes([]byte("participant-authorization"))}
	effectID := runtimecore.EffectIntentID("participant-effect")
	intentDigest := runtimecore.DigestBytes([]byte("participant-intent"))
	admission := runtimeports.OperationEffectAdmissionReceiptV3{OperationDigest: opDigest, EffectID: effectID, IntentRevision: 1, IntentDigest: intentDigest, FactRevision: 1, State: "accepted"}
	schema := runtimeports.SchemaRefV2{Namespace: "praxis.runtime", Name: "restore-stage-payload", Version: "1.0.0", MediaType: "application/json", ContentDigest: runtimecore.DigestBytes([]byte("participant-payload-schema"))}
	payloadDigest := runtimecore.DigestBytes([]byte("participant-payload"))
	governanceDigest := runtimecore.DigestBytes([]byte("participant-governance"))
	admitted, err := runtimeports.SealOperationAuthorizedAdmissionV4(runtimeports.OperationAuthorizedAdmissionV4{Admission: admission, Authorization: authorization, PayloadSchema: schema, PayloadDigest: payloadDigest, PayloadRevision: 1, ReviewProjectionDigest: runtimecore.DigestBytes([]byte("participant-review-projection")), ReviewCurrentnessDigest: runtimecore.DigestBytes([]byte("participant-review-current")), LegacyReviewProjectionDigest: legacyReviewDigest, GovernanceSnapshotDigest: governanceDigest, AuthorizationFenceDigest: runtimecore.DigestBytes([]byte("participant-authorization-fence")), ExpiresUnixNano: expires})
	if err != nil {
		t.Fatal(err)
	}
	provider := workspaceRestoreParticipantProviderV1("sandbox/workspace-restore-stage")
	subjectDigest := runtimecore.DigestBytes([]byte("participant-subject"))
	fence := runtimecore.ExecutionFence{BoundaryScope: runtimecore.FenceBoundaryInstance, Scope: scope, CapabilityGrantDigest: runtimecore.DigestBytes([]byte("participant-capability")), EffectIntentID: effectID, EffectIntentRevision: 1, CanonicalPayloadDigest: payloadDigest, ExpiresAt: time.Unix(0, expires)}
	fenceDigest, _ := runtimeports.DigestOperationExecutionFenceV3(fence, operation)
	legacy := runtimeports.OperationDispatchPermitV3{ContractVersion: runtimeports.OperationEffectContractVersionV3, ID: "participant-permit", Revision: 1, AttemptID: "participant-dispatch", IntentID: effectID, IntentRevision: 1, IntentDigest: intentDigest, Operation: operation, PayloadSchema: schema, PayloadDigest: payloadDigest, PayloadRevision: 1, ConflictDomain: runtimeports.ConflictDomainBindingV2{Domain: "praxis.runtime/restore-stage", ScopeClass: runtimeports.EffectStableScopeTenantV2, ScopeDigest: runtimeports.StableTenantScopeDigestV2(request.Attempt.TenantID)}, Provider: provider, EnforcementPoint: provider, Authority: runtimeports.AuthorityBindingRefV2{Ref: "participant-authority", Digest: runtimecore.DigestBytes([]byte("participant-authority")), Revision: 1, Epoch: identity.TargetFenceEpoch}, Review: review, ReviewAuthorization: legacyReview, Budget: runtimeports.OperationBudgetBindingRefV3{Ref: "participant-budget", Digest: runtimecore.DigestBytes([]byte("participant-budget")), Revision: 1, PolicyDigest: runtimecore.DigestBytes([]byte("participant-budget-policy")), SubjectDigest: subjectDigest}, Policy: runtimeports.OperationPolicyBindingRefV3{Ref: "participant-policy", Digest: runtimecore.DigestBytes([]byte("participant-policy-binding")), Revision: 1, SubjectDigest: subjectDigest}, CapabilityGrantDigest: fence.CapabilityGrantDigest, CredentialGrantDigest: runtimecore.DigestBytes([]byte("participant-credential")), GovernanceSnapshotDigest: governanceDigest, FenceDigest: fenceDigest, Idempotency: runtimeports.IdempotencyBindingV2{Key: "participant-idempotency", ScopeClass: runtimeports.EffectStableScopeTenantV2, ScopeDigest: runtimeports.StableTenantScopeDigestV2(request.Attempt.TenantID), Class: runtimecore.IdempotencyQueryable}, IssuedUnixNano: now.UnixNano(), ExpiresUnixNano: expires}
	permit, err := runtimeports.SealOperationDispatchPermitV4(runtimeports.OperationDispatchPermitV4{LegacyPermit: legacy, Admission: admitted})
	if err != nil {
		t.Fatal(err)
	}
	record, err := runtimeports.SealOperationDispatchRecordV4(runtimeports.OperationDispatchRecordV4{Permit: permit, PermitDigest: permit.Digest, Fence: fence, State: runtimeports.OperationPermitBegunV4, Revision: 2, EffectFactRevision: 3, BegunUnixNano: now.UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	current := runtimeports.CurrentOperationDispatchAuthorizationV4{Record: record, ReviewAuthorization: authorization, ReviewProjectionDigest: admitted.ReviewProjectionDigest, ReviewCurrentnessDigest: admitted.ReviewCurrentnessDigest, GovernanceSnapshotDigest: admitted.GovernanceSnapshotDigest, CheckedUnixNano: now.UnixNano()}
	value, err := applicationcontract.SealRestoreStageAuthorizedDispatchV1(applicationcontract.RestoreStageAuthorizedDispatchV1{RequestDigest: request.Digest, Dispatch: current, SnapshotArtifact: snapshot, EvidenceSource: runtimeports.EvidenceSourceRegistrationRefV1{RegistrationID: "participant-restore-stage-source", Revision: 1, FactDigest: runtimecore.DigestBytes([]byte("participant-source-fact")), ConfigurationDigest: runtimecore.DigestBytes([]byte("participant-source-config")), SourceID: runtimeports.RestoreStageEvidenceSourceIDV1, SourceEpoch: 1}, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires})
	if err != nil || value.ValidateFor(request, now) != nil {
		t.Fatalf("authorization: %v", err)
	}
	return value
}

func workspaceRestoreParticipantExecuteV1(t *testing.T, authorized applicationcontract.RestoreStageAuthorizedDispatchV1, prepared runtimeports.RestoreStageSandboxCurrentProjectionV1, now time.Time) runtimeports.OperationDispatchEnforcementPhaseRefV4 {
	t.Helper()
	record := authorized.Dispatch.Record
	legacy := record.Permit.LegacyPermit
	opDigest, _ := legacy.Operation.DigestV3()
	value := runtimeports.OperationDispatchEnforcementPhaseRefV4{OperationDigest: opDigest, EffectID: legacy.IntentID, PermitID: legacy.ID, PermitFactRevision: record.Revision, PermitDigest: record.PermitDigest, AdmissionDigest: record.Permit.Admission.Digest, ReviewAuthorization: authorized.Dispatch.ReviewAuthorization, AttemptID: legacy.AttemptID, SandboxAttempt: prepared.SandboxAttempt, Phase: runtimeports.OperationDispatchEnforcementExecuteV4, ReceiptDigest: runtimecore.DigestBytes([]byte("participant-execute")), JournalRevision: 2, ValidatedUnixNano: now.UnixNano(), ExpiresUnixNano: prepared.ExpiresUnixNano, PrepareReceiptDigest: runtimecore.DigestBytes([]byte("participant-prepare")), PreparedAttemptDigest: prepared.Prepared.Digest}
	if err := value.Validate(); err != nil {
		t.Fatal(err)
	}
	return value
}

func workspaceRestoreParticipantRequestV1(request applicationcontract.RestoreStageActionRequestV1, authorized applicationcontract.RestoreStageAuthorizedDispatchV1, now time.Time) (contract.WorkspaceRestoreStageRequestV1, error) {
	legacy := authorized.Dispatch.Record.Permit.LegacyPermit
	expires := request.NotAfterUnixNano
	if authorized.ExpiresUnixNano < expires {
		expires = authorized.ExpiresUnixNano
	}
	if request.Eligibility.ExpiresUnixNano < expires {
		expires = request.Eligibility.ExpiresUnixNano
	}
	exact := func(kind, domain, id string, revision uint64, digest string, exactExpires int64) contract.SnapshotArtifactExactRefV2 {
		return contract.SnapshotArtifactExactRefV2{TypeURL: kind, Version: 2, ID: id, Revision: revision, DigestAlgorithm: contract.SnapshotArtifactDigestSHA256, DigestDomain: domain, Digest: strings.TrimPrefix(digest, "sha256:"), ExpiresUnixNano: exactExpires}
	}
	identity := request.Materialization.Identity
	value := contract.WorkspaceRestoreStageRequestV1{TenantID: string(request.Attempt.TenantID), DispatchAttemptID: legacy.AttemptID, RuntimeRestoreAttempt: exact("praxis.runtime/restore-attempt/v2", "praxis.runtime/restore-attempt/body/v2", request.Attempt.ID, uint64(request.Attempt.Revision), string(request.Attempt.Digest), expires), RestoreEligibility: exact("praxis.runtime/restore-eligibility/v2", "praxis.runtime/restore-eligibility/body/v2", request.Eligibility.ID, uint64(request.Eligibility.Revision), string(request.Eligibility.Digest), request.Eligibility.ExpiresUnixNano), Target: contract.RuntimeLeaseBinding{TenantID: string(request.Attempt.TenantID), InstanceID: string(identity.TargetInstance.ID), InstanceEpoch: uint64(identity.TargetInstance.Epoch), LeaseID: string(identity.TargetLease.ID), LeaseEpoch: uint64(identity.TargetLease.Epoch), FenceEpoch: uint64(identity.TargetFenceEpoch), ScopeDigest: string(legacy.Operation.ExecutionScopeDigest), ObservedRevision: uint64(legacy.Operation.CurrentProjectionRevision), ExpiresUnixNano: expires}, SnapshotArtifactFactRef: exact(contract.SnapshotArtifactFactTypeURL, contract.SnapshotArtifactFactDomain, authorized.SnapshotArtifact.ID, uint64(authorized.SnapshotArtifact.Revision), authorized.SnapshotArtifact.Digest, expires), RequestedNotAfter: expires}
	return value, value.ValidateCurrent(now)
}

func workspaceRestoreParticipantProviderV1(capability string) runtimeports.ProviderBindingRefV2 {
	return runtimeports.ProviderBindingRefV2{BindingSetID: "participant-binding-" + capability, BindingSetRevision: 1, ComponentID: "praxis/sandbox", ManifestDigest: runtimecore.DigestBytes([]byte("participant-manifest-" + capability)), ArtifactDigest: runtimecore.DigestBytes([]byte("participant-artifact-" + capability)), Capability: runtimeports.CapabilityNameV2(capability)}
}

func workspaceRestoreParticipantExternalRefV1(tenant runtimecore.TenantID, scope runtimecore.Digest, id, kind string) runtimeports.CheckpointExternalExactFactRefV2 {
	owner := workspaceRestoreParticipantProviderV1("sandbox/" + kind)
	return runtimeports.CheckpointExternalExactFactRefV2{ContractVersion: "praxis.sandbox/" + kind + "/v1", SchemaRef: "praxis.sandbox/" + kind + "-schema/v1", Owner: runtimeports.CheckpointManifestSealOwnerBindingV2{BindingSetID: owner.BindingSetID, BindingRevision: owner.BindingSetRevision, ComponentID: string(owner.ComponentID), ManifestDigest: string(owner.ManifestDigest), ArtifactDigest: string(owner.ArtifactDigest), Capability: string(owner.Capability), FactKind: kind}, TenantID: string(tenant), ID: id, Revision: 1, Digest: string(runtimecore.DigestBytes([]byte(id))), ScopeDigest: string(scope)}
}
