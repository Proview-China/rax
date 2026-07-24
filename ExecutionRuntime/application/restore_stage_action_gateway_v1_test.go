package application

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationfakes "github.com/Proview-China/rax/ExecutionRuntime/application/fakes"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimefakes "github.com/Proview-China/rax/ExecutionRuntime/runtime/fakes"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestRestoreStageActionGatewayV1OrderedExactLostReplyReplay(t *testing.T) {
	fixture := newRestoreStageActionGatewayFixtureV1(t)
	fixture.results.LoseNextReplyForTestV1()
	result, err := fixture.gateway.ExecuteRestoreStageActionV1(context.Background(), fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	if err := result.ValidateFor(fixture.request, fixture.now); err != nil {
		t.Fatal(err)
	}
	if fixture.authorization.calls != 1 || fixture.participant.prepareCalls != 1 || fixture.enforcement.prepareCalls != 1 || fixture.enforcement.executeCalls != 1 || fixture.participant.executeCalls != 1 || fixture.evidence.calls != 1 || fixture.settlements.calls != 1 || fixture.participant.applyCalls != 1 {
		t.Fatalf("Restore Stage order/call counts drifted: auth=%d participant=(%d,%d,%d) enforcement=(%d,%d) evidence=%d settlement=%d", fixture.authorization.calls, fixture.participant.prepareCalls, fixture.participant.executeCalls, fixture.participant.applyCalls, fixture.enforcement.prepareCalls, fixture.enforcement.executeCalls, fixture.evidence.calls, fixture.settlements.calls)
	}
	replay, err := fixture.gateway.ExecuteRestoreStageActionV1(context.Background(), fixture.request)
	if err != nil || replay.Digest != result.Digest || fixture.participant.executeCalls != 1 {
		t.Fatalf("Restore Stage replay repeated execution: replay=%+v err=%v execute=%d", replay, err, fixture.participant.executeCalls)
	}
}

func TestRestoreStageActionGatewayV1RejectsSnapshotAndAuthorizationSpliceBeforeParticipant(t *testing.T) {
	fixture := newRestoreStageActionGatewayFixtureV1(t)
	fixture.authorization.value.SnapshotArtifact.ID = "other-snapshot"
	fixture.authorization.value, _ = applicationcontract.SealRestoreStageAuthorizedDispatchV1(fixture.authorization.value)
	if _, err := fixture.gateway.ExecuteRestoreStageActionV1(context.Background(), fixture.request); err == nil {
		t.Fatal("spliced Snapshot authorization was accepted")
	}
	if fixture.participant.prepareCalls != 0 {
		t.Fatal("spliced authorization reached Sandbox Prepare")
	}
}

type restoreStageActionGatewayFixtureV1 struct {
	now           time.Time
	request       applicationcontract.RestoreStageActionRequestV1
	results       *applicationfakes.RestoreStageActionResultStoreV1
	authorization *restoreStageAuthorizationFakeV1
	participant   *restoreStageParticipantFakeV1
	enforcement   *restoreStageEnforcementFakeV1
	governance    *restoreStageGovernanceFakeV1
	evidence      *restoreStageEvidenceFakeV1
	settlements   *restoreStageSettlementFakeV1
	gateway       *RestoreStageActionGatewayV1
}

func newRestoreStageActionGatewayFixtureV1(t *testing.T) *restoreStageActionGatewayFixtureV1 {
	t.Helper()
	now := time.Unix(1_800_000_000, 0)
	plan, err := runtimefakes.BuildRestorePlanCurrentFixtureV2("application-stage", now)
	if err != nil {
		t.Fatal(err)
	}
	tenant := core.TenantID(plan.RestorePlan.TenantID)
	attempt := runtimeports.RestoreAttemptRefV2{TenantID: tenant, ID: "restore-attempt-stage-action", Revision: 2, Digest: core.DigestBytes([]byte("restore-attempt-stage-action"))}
	eligibility := runtimeports.RestoreEligibilityRefV2{TenantID: tenant, ID: "restore-eligibility-stage-action", Revision: 1, Digest: core.DigestBytes([]byte("restore-eligibility-stage-action")), ExpiresUnixNano: now.Add(time.Minute).UnixNano()}
	snapshot := restoreStageActionExternalRefV1(tenant, plan.SourceScopeDigest, "workspace-snapshot-stage-action", "snapshot-artifact-fact")
	materialization, err := runtimeports.SealRestoreMaterializationCurrentProjectionV1(runtimeports.RestoreMaterializationCurrentProjectionV1{Attempt: attempt, Eligibility: eligibility, RestorePlan: plan.RestorePlan, Consistency: plan.CheckpointConsistency.Ref, ManifestSeal: plan.ManifestSeal, SourceScopeDigest: plan.SourceScopeDigest, Identity: plan.IdentityProposal, ContextGeneration: restoreStageActionExternalRefV1(tenant, plan.SourceScopeDigest, "context-generation-stage-action", "context-generation"), ContextFrames: []runtimeports.CheckpointExternalExactFactRefV2{restoreStageActionExternalRefV1(tenant, plan.SourceScopeDigest, "context-frame-stage-action", "context-frame")}, Memory: []runtimeports.CheckpointExternalExactFactRefV2{restoreStageActionExternalRefV1(tenant, plan.SourceScopeDigest, "memory-stage-action", "memory")}, Knowledge: []runtimeports.CheckpointExternalExactFactRefV2{restoreStageActionExternalRefV1(tenant, plan.SourceScopeDigest, "knowledge-stage-action", "knowledge")}, Snapshots: []runtimeports.CheckpointExternalExactFactRefV2{snapshot}, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(50 * time.Second).UnixNano()}, now)
	if err != nil {
		t.Fatal(err)
	}
	request, err := applicationcontract.SealRestoreStageActionRequestV1(applicationcontract.RestoreStageActionRequestV1{ID: "restore-stage-action", IdempotencyKey: "restore-stage-action-key", Attempt: attempt, Eligibility: eligibility, Materialization: materialization, NotAfterUnixNano: now.Add(45 * time.Second).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	authorized := buildRestoreStageAuthorizedDispatchV1(t, request, snapshot, now)
	results := applicationfakes.NewRestoreStageActionResultStoreV1()
	authorization := &restoreStageAuthorizationFakeV1{value: authorized}
	participant := &restoreStageParticipantFakeV1{now: now}
	enforcement := &restoreStageEnforcementFakeV1{now: now}
	governance := &restoreStageGovernanceFakeV1{now: now, participant: participant, enforcement: enforcement}
	evidence := &restoreStageEvidenceFakeV1{}
	settlements := &restoreStageSettlementFakeV1{}
	gateway, err := NewRestoreStageActionGatewayV1(RestoreStageActionGatewayConfigV1{Results: results, Authorization: authorization, Participant: participant, Enforcement: enforcement, Governance: governance, Evidence: evidence, Settlements: settlements, Clock: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	participant.governance = governance
	return &restoreStageActionGatewayFixtureV1{now, request, results, authorization, participant, enforcement, governance, evidence, settlements, gateway}
}

func buildRestoreStageAuthorizedDispatchV1(t *testing.T, request applicationcontract.RestoreStageActionRequestV1, snapshot runtimeports.CheckpointExternalExactFactRefV2, now time.Time) applicationcontract.RestoreStageAuthorizedDispatchV1 {
	t.Helper()
	identity := request.Materialization.Identity
	scope := core.ExecutionScope{Identity: core.AgentIdentityRef{TenantID: request.Attempt.TenantID, ID: "identity-stage-action", Epoch: 1}, Lineage: core.LineageRef{ID: "lineage-stage-action", PlanDigest: core.DigestBytes([]byte("lineage-stage-action"))}, Instance: identity.TargetInstance, SandboxLease: &identity.TargetLease, AuthorityEpoch: identity.TargetFenceEpoch}
	scopeDigest, _ := runtimeports.ExecutionScopeDigestV2(scope)
	operation := runtimeports.OperationSubjectV3{Kind: runtimeports.RestoreStageOperationKindV1, ExecutionScope: scope, ExecutionScopeDigest: scopeDigest, CustomOperationID: request.Attempt.ID, SubjectRevision: 1, CurrentProjectionRef: "restore-stage-action-current", CurrentProjectionRevision: 1, CurrentProjectionDigest: core.DigestBytes([]byte("restore-stage-action-current"))}
	opDigest, _ := operation.DigestV3()
	expires := now.Add(25 * time.Second).UnixNano()
	fact := func(id string) runtimeports.OperationGovernanceFactRefV3 {
		return runtimeports.OperationGovernanceFactRefV3{Ref: id, Revision: 1, Digest: core.DigestBytes([]byte(id)), ExpiresUnixNano: expires}
	}
	review := runtimeports.OperationReviewBindingRefV3{CaseRef: "restore-review-case", CandidateDigest: core.DigestBytes([]byte("restore-review-candidate")), CandidateRevision: 1, PolicyDigest: core.DigestBytes([]byte("restore-review-policy"))}
	legacyReview := runtimeports.OperationReviewAuthorizationV3{Case: fact(review.CaseRef), CandidateDigest: review.CandidateDigest, CandidateRevision: review.CandidateRevision, Verdict: fact("restore-review-verdict"), ReviewerAuthority: fact("restore-reviewer-authority"), PolicyDigest: review.PolicyDigest, ExpiresUnixNano: expires}
	legacyReviewDigest, _ := runtimeports.DigestOperationReviewAuthorizationV3(legacyReview)
	authorization := runtimeports.OperationReviewAuthorizationRefV4{ID: "restore-review-authorization", Revision: 1, Digest: core.DigestBytes([]byte("restore-review-authorization"))}
	effectID := core.EffectIntentID("restore-stage-effect")
	intentDigest := core.DigestBytes([]byte("restore-stage-intent"))
	admission := runtimeports.OperationEffectAdmissionReceiptV3{OperationDigest: opDigest, EffectID: effectID, IntentRevision: 1, IntentDigest: intentDigest, FactRevision: 1, State: "accepted"}
	schema := runtimeports.SchemaRefV2{Namespace: "praxis.runtime", Name: "restore-stage-payload", Version: "1.0.0", MediaType: "application/json", ContentDigest: core.DigestBytes([]byte("restore-stage-payload-schema"))}
	payloadDigest := core.DigestBytes([]byte("restore-stage-payload"))
	governanceDigest := core.DigestBytes([]byte("restore-governance-snapshot"))
	authorizedAdmission, err := runtimeports.SealOperationAuthorizedAdmissionV4(runtimeports.OperationAuthorizedAdmissionV4{Admission: admission, Authorization: authorization, PayloadSchema: schema, PayloadDigest: payloadDigest, PayloadRevision: 1, ReviewProjectionDigest: core.DigestBytes([]byte("restore-review-projection")), ReviewCurrentnessDigest: core.DigestBytes([]byte("restore-review-currentness")), LegacyReviewProjectionDigest: legacyReviewDigest, GovernanceSnapshotDigest: governanceDigest, AuthorizationFenceDigest: core.DigestBytes([]byte("restore-authorization-fence")), ExpiresUnixNano: expires})
	if err != nil {
		t.Fatal(err)
	}
	provider := restoreStageActionProviderV1("praxis/sandbox", "sandbox/workspace-restore-stage")
	subjectDigest := core.DigestBytes([]byte("restore-operation-subject"))
	fence := core.ExecutionFence{BoundaryScope: core.FenceBoundaryInstance, Scope: scope, CapabilityGrantDigest: core.DigestBytes([]byte("restore-capability")), EffectIntentID: effectID, EffectIntentRevision: 1, CanonicalPayloadDigest: payloadDigest, ExpiresAt: time.Unix(0, expires)}
	fenceDigest, _ := runtimeports.DigestOperationExecutionFenceV3(fence, operation)
	legacy := runtimeports.OperationDispatchPermitV3{ContractVersion: runtimeports.OperationEffectContractVersionV3, ID: "restore-stage-permit", Revision: 1, AttemptID: "restore-stage-dispatch", IntentID: effectID, IntentRevision: 1, IntentDigest: intentDigest, Operation: operation, PayloadSchema: schema, PayloadDigest: payloadDigest, PayloadRevision: 1, ConflictDomain: runtimeports.ConflictDomainBindingV2{Domain: "praxis.runtime/restore-stage", ScopeClass: runtimeports.EffectStableScopeTenantV2, ScopeDigest: runtimeports.StableTenantScopeDigestV2(request.Attempt.TenantID)}, Provider: provider, EnforcementPoint: provider, Authority: runtimeports.AuthorityBindingRefV2{Ref: "restore-authority", Digest: core.DigestBytes([]byte("restore-authority")), Revision: 1, Epoch: identity.TargetFenceEpoch}, Review: review, ReviewAuthorization: legacyReview, Budget: runtimeports.OperationBudgetBindingRefV3{Ref: "restore-budget", Digest: core.DigestBytes([]byte("restore-budget")), Revision: 1, PolicyDigest: core.DigestBytes([]byte("restore-budget-policy")), SubjectDigest: subjectDigest}, Policy: runtimeports.OperationPolicyBindingRefV3{Ref: "restore-policy", Digest: core.DigestBytes([]byte("restore-policy")), Revision: 1, SubjectDigest: subjectDigest}, CapabilityGrantDigest: fence.CapabilityGrantDigest, CredentialGrantDigest: core.DigestBytes([]byte("restore-credentials")), GovernanceSnapshotDigest: governanceDigest, FenceDigest: fenceDigest, Idempotency: runtimeports.IdempotencyBindingV2{Key: "restore-stage-idempotency", ScopeClass: runtimeports.EffectStableScopeTenantV2, ScopeDigest: runtimeports.StableTenantScopeDigestV2(request.Attempt.TenantID), Class: core.IdempotencyQueryable}, IssuedUnixNano: now.UnixNano(), ExpiresUnixNano: expires}
	permit, err := runtimeports.SealOperationDispatchPermitV4(runtimeports.OperationDispatchPermitV4{LegacyPermit: legacy, Admission: authorizedAdmission})
	if err != nil {
		t.Fatal(err)
	}
	record, err := runtimeports.SealOperationDispatchRecordV4(runtimeports.OperationDispatchRecordV4{Permit: permit, PermitDigest: permit.Digest, Fence: fence, State: runtimeports.OperationPermitBegunV4, Revision: 2, EffectFactRevision: 3, BegunUnixNano: now.UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	current := runtimeports.CurrentOperationDispatchAuthorizationV4{Record: record, ReviewAuthorization: authorization, ReviewProjectionDigest: authorizedAdmission.ReviewProjectionDigest, ReviewCurrentnessDigest: authorizedAdmission.ReviewCurrentnessDigest, GovernanceSnapshotDigest: authorizedAdmission.GovernanceSnapshotDigest, CheckedUnixNano: now.UnixNano()}
	if err := current.Validate(); err != nil {
		t.Fatal(err)
	}
	value, err := applicationcontract.SealRestoreStageAuthorizedDispatchV1(applicationcontract.RestoreStageAuthorizedDispatchV1{RequestDigest: request.Digest, Dispatch: current, SnapshotArtifact: snapshot, EvidenceSource: restoreStageEvidenceSourceRefV1("action"), CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires})
	if err != nil || value.ValidateFor(request, now) != nil {
		t.Fatalf("authorized dispatch: %v", err)
	}
	return value
}

type restoreStageAuthorizationFakeV1 struct {
	mu    sync.Mutex
	value applicationcontract.RestoreStageAuthorizedDispatchV1
	calls int
}

func (f *restoreStageAuthorizationFakeV1) AuthorizeRestoreStageV1(context.Context, applicationcontract.RestoreStageActionRequestV1) (applicationcontract.RestoreStageAuthorizedDispatchV1, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return f.value, nil
}
func (f *restoreStageAuthorizationFakeV1) InspectRestoreStageAuthorizationV1(context.Context, applicationcontract.RestoreStageActionInspectKeyV1) (applicationcontract.RestoreStageAuthorizedDispatchV1, error) {
	return f.value, nil
}

type restoreStageParticipantFakeV1 struct {
	mu                                     sync.Mutex
	now                                    time.Time
	governance                             *restoreStageGovernanceFakeV1
	prepareCalls, executeCalls, applyCalls int
	prepared                               runtimeports.RestoreStageSandboxCurrentProjectionV1
}

func (f *restoreStageParticipantFakeV1) PrepareRestoreStageV1(_ context.Context, request applicationcontract.RestoreStageActionRequestV1, authorized applicationcontract.RestoreStageAuthorizedDispatchV1) (runtimeports.RestoreStageSandboxCurrentProjectionV1, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.prepareCalls++
	legacy := authorized.Dispatch.Record.Permit.LegacyPermit
	opDigest, _ := legacy.Operation.DigestV3()
	dispatch := runtimeports.OperationDispatchAttemptRefV3{OperationDigest: opDigest, EffectID: legacy.IntentID, IntentRevision: legacy.IntentRevision, IntentDigest: legacy.IntentDigest, PermitID: legacy.ID, PermitRevision: authorized.Dispatch.Record.Revision, PermitDigest: authorized.Dispatch.Record.PermitDigest, AttemptID: legacy.AttemptID}
	sandboxAttempt := runtimeports.OperationDispatchSandboxFactRefV4{ID: dispatch.AttemptID, Revision: 1, Digest: core.DigestBytes([]byte("sandbox-stage-attempt")), ExpiresUnixNano: f.now.Add(30 * time.Second).UnixNano()}
	prepared, err := runtimeports.SealRestoreStagePreparedAttemptRefV1(runtimeports.RestoreStagePreparedAttemptRefV1{SandboxAttempt: sandboxAttempt, OperationDigest: opDigest, EffectID: dispatch.EffectID, IntentRevision: dispatch.IntentRevision, IntentDigest: dispatch.IntentDigest, DispatchAttempt: dispatch, Provider: legacy.EnforcementPoint, BundleDigest: core.DigestBytes([]byte("restore-bundle")), PreparedUnixNano: f.now.UnixNano(), ExpiresUnixNano: sandboxAttempt.ExpiresUnixNano})
	if err != nil {
		return runtimeports.RestoreStageSandboxCurrentProjectionV1{}, err
	}
	f.prepared, err = runtimeports.SealRestoreStageSandboxCurrentProjectionV1(runtimeports.RestoreStageSandboxCurrentProjectionV1{Operation: legacy.Operation, OperationDigest: opDigest, EffectID: dispatch.EffectID, IntentRevision: dispatch.IntentRevision, IntentDigest: dispatch.IntentDigest, DispatchAttempt: dispatch, SandboxAttempt: sandboxAttempt, RestoreAttempt: request.Attempt, Eligibility: request.Eligibility, Identity: request.Materialization.Identity, SnapshotArtifact: authorized.SnapshotArtifact, BundleProjectionDigest: core.DigestBytes([]byte("restore-bundle-projection")), BundleDigest: prepared.BundleDigest, Provider: legacy.EnforcementPoint, Prepared: prepared, Current: true, CheckedUnixNano: f.now.UnixNano(), ExpiresUnixNano: prepared.ExpiresUnixNano}, f.now)
	return f.prepared, err
}
func (f *restoreStageParticipantFakeV1) ExecuteRestoreStageV1(_ context.Context, request applicationcontract.RestoreStageActionRequestV1, authorized applicationcontract.RestoreStageAuthorizedDispatchV1, execute runtimeports.OperationDispatchEnforcementPhaseRefV4) (runtimeports.RestoreStageDomainResultCurrentProjectionV1, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.executeCalls++
	legacy := authorized.Dispatch.Record.Permit.LegacyPermit
	opDigest, _ := legacy.Operation.DigestV3()
	domain := runtimeports.RestoreStageDomainResultFactRefV1{Owner: legacy.Provider, Kind: runtimeports.RestoreStageDomainResultKindV1, ID: "restore-stage-domain", Revision: 1, Digest: core.DigestBytes([]byte("restore-stage-domain")), TenantID: request.Attempt.TenantID, Operation: legacy.Operation, OperationDigest: opDigest, EffectID: legacy.IntentID, EffectRevision: legacy.IntentRevision, Attempt: f.prepared.DispatchAttempt, RestoreAttempt: request.Attempt, Eligibility: request.Eligibility, PayloadSchema: runtimeports.SchemaRefV2{Namespace: "praxis.sandbox", Name: "workspace-restore-stage", Version: "1.0.0", MediaType: "application/json", ContentDigest: core.DigestBytes([]byte("restore-stage-domain-schema"))}, PayloadDigest: core.DigestBytes([]byte("restore-stage-domain-payload")), PayloadRevision: 1, AuthoritativeTime: f.now.UnixNano()}
	return runtimeports.SealRestoreStageDomainResultCurrentProjectionV1(runtimeports.RestoreStageDomainResultCurrentProjectionV1{Fact: domain, CheckedUnixNano: f.now.UnixNano(), ExpiresUnixNano: f.now.Add(25 * time.Second).UnixNano()}, f.now)
}
func (f *restoreStageParticipantFakeV1) ApplyRestoreStageSettlementV1(_ context.Context, settlement runtimeports.RestoreStageSettlementRefV1, stage runtimeports.RestoreStageDomainResultCurrentProjectionV1) (runtimeports.RestoreStageApplySettlementCurrentProjectionV1, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.applyCalls++
	domain := stage.Fact
	ref := runtimeports.RestoreStageApplySettlementRefV1{Owner: restoreStageActionProviderV1("praxis/sandbox", "sandbox/workspace-restore-apply"), ID: "restore-stage-apply", Revision: 1, Digest: core.DigestBytes([]byte("restore-stage-apply")), TenantID: domain.TenantID, DomainResult: domain, RuntimeSettlement: settlement}
	return runtimeports.SealRestoreStageApplySettlementCurrentProjectionV1(runtimeports.RestoreStageApplySettlementCurrentProjectionV1{Fact: ref, CheckedUnixNano: f.now.UnixNano(), ExpiresUnixNano: f.now.Add(20 * time.Second).UnixNano()}, f.now)
}

type restoreStageEnforcementFakeV1 struct {
	mu                         sync.Mutex
	now                        time.Time
	prepareCalls, executeCalls int
	prepare                    runtimeports.OperationDispatchEnforcementPhaseRefV4
	execute                    runtimeports.OperationDispatchEnforcementPhaseRefV4
	losePrepareReply           bool
	loseExecuteReply           bool
}

func (f *restoreStageEnforcementFakeV1) EnforceRestoreStageDispatchV1(_ context.Context, request runtimeports.EnforceRestoreStageDispatchRequestV1) (runtimeports.OperationDispatchEnforcementPhaseRefV4, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := request.Validate(); err != nil {
		return runtimeports.OperationDispatchEnforcementPhaseRefV4{}, err
	}
	ref := runtimeports.OperationDispatchEnforcementPhaseRefV4{OperationDigest: request.DispatchAttempt.OperationDigest, EffectID: request.EffectID, PermitID: request.PermitID, PermitFactRevision: request.ExpectedPermitFactRevision, PermitDigest: request.PermitDigest, AdmissionDigest: request.AdmissionDigest, ReviewAuthorization: request.ReviewAuthorization, AttemptID: request.DispatchAttempt.AttemptID, SandboxAttempt: request.SandboxAttempt, Phase: request.Phase, ReceiptDigest: core.DigestBytes([]byte("enforcement-" + string(request.Phase))), JournalRevision: request.ExpectedJournalRevision + 1, ValidatedUnixNano: f.now.UnixNano(), ExpiresUnixNano: f.now.Add(20 * time.Second).UnixNano()}
	if request.Phase == runtimeports.OperationDispatchEnforcementPrepareV4 {
		f.prepareCalls++
		f.prepare = ref
		if f.losePrepareReply {
			f.losePrepareReply = false
			return runtimeports.OperationDispatchEnforcementPhaseRefV4{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "prepare enforcement reply lost")
		}
	} else {
		f.executeCalls++
		ref.PrepareReceiptDigest = request.Prepare.ReceiptDigest
		ref.PreparedAttemptDigest = request.Prepared.Digest
		f.execute = ref
		if f.loseExecuteReply {
			f.loseExecuteReply = false
			return runtimeports.OperationDispatchEnforcementPhaseRefV4{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "execute enforcement reply lost")
		}
	}
	return ref, ref.Validate()
}
func (f *restoreStageEnforcementFakeV1) InspectRestoreStageDispatchEnforcementByRequestV1(_ context.Context, request runtimeports.EnforceRestoreStageDispatchRequestV1) (runtimeports.OperationDispatchEnforcementPhaseRefV4, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if request.Phase == runtimeports.OperationDispatchEnforcementPrepareV4 && f.prepare.Validate() == nil {
		return f.prepare, nil
	}
	if request.Phase == runtimeports.OperationDispatchEnforcementExecuteV4 && f.execute.Validate() == nil && request.Prepare != nil && request.Prepared != nil && f.execute.PrepareReceiptDigest == request.Prepare.ReceiptDigest && f.execute.PreparedAttemptDigest == request.Prepared.Digest {
		return f.execute, nil
	}
	return runtimeports.OperationDispatchEnforcementPhaseRefV4{}, core.NewError(core.ErrorNotFound, core.ReasonEvidenceUnavailable, "enforcement attempt absent")
}
func (f *restoreStageEnforcementFakeV1) InspectCurrentRestoreStageDispatchEnforcementV1(context.Context, runtimeports.OperationSubjectV3, runtimeports.OperationDispatchEnforcementPhaseRefV4) (runtimeports.OperationDispatchEnforcementPhaseRefV4, error) {
	panic("unused")
}

type restoreStageGovernanceFakeV1 struct {
	now         time.Time
	participant *restoreStageParticipantFakeV1
	enforcement *restoreStageEnforcementFakeV1
}

func (f *restoreStageGovernanceFakeV1) InspectRestoreStageGovernanceCurrentV1(_ context.Context, request runtimeports.InspectRestoreStageGovernanceCurrentRequestV1) (runtimeports.RestoreStageGovernanceCurrentProjectionV1, error) {
	legacy := f.participant.prepared
	return runtimeports.SealRestoreStageGovernanceCurrentProjectionV1(runtimeports.RestoreStageGovernanceCurrentProjectionV1{RestoreAttempt: request.RestoreAttempt, Eligibility: request.Eligibility, Identity: legacy.Identity, Operation: request.Operation, EffectID: request.EffectID, EffectRevision: request.Admission.IntentRevision, IntentDigest: request.Admission.IntentDigest, Admission: request.Admission, DispatchAdmissionDigest: request.ExecuteEnforcement.AdmissionDigest, Authorization: request.Authorization, PermitID: request.PermitID, PermitFactRevision: request.DispatchAttempt.PermitRevision, PermitDigest: request.DispatchAttempt.PermitDigest, BeginRecordRevision: request.DispatchAttempt.PermitRevision, BeginRecordDigest: core.DigestBytes([]byte("restore-begin-record")), DispatchAttempt: request.DispatchAttempt, ExecuteEnforcement: request.ExecuteEnforcement, MaterializationDigest: core.DigestBytes([]byte("restore-materialization")), SnapshotArtifact: request.SnapshotArtifact, CheckedUnixNano: f.now.UnixNano(), ExpiresUnixNano: f.now.Add(20 * time.Second).UnixNano()}, f.now)
}

type restoreStageEvidenceFakeV1 struct{ calls int }

func (f *restoreStageEvidenceFakeV1) PublishRestoreStageEvidenceV1(_ context.Context, request applicationcontract.RestoreStageEvidenceRequestV1) (runtimeports.EvidenceRecordRefV2, error) {
	f.calls++
	return runtimeports.EvidenceRecordRefV2{LedgerScopeDigest: core.DigestBytes([]byte("restore-ledger")), Sequence: 1, RecordDigest: core.DigestBytes([]byte("restore-evidence"))}, nil
}

type restoreStageSettlementFakeV1 struct {
	mu        sync.Mutex
	calls     int
	loseReply bool
	value     runtimeports.RestoreStageSettlementRefV1
}

func (f *restoreStageSettlementFakeV1) SettleRestoreStageV1(_ context.Context, submission runtimeports.RestoreStageSettlementSubmissionV1) (runtimeports.RestoreStageSettlementRefV1, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.value = runtimeports.RestoreStageSettlementRefV1{ID: submission.ID, Revision: 1, Digest: core.DigestBytes([]byte("restore-runtime-settlement")), OperationDigest: submission.OperationDigest, EffectID: submission.EffectID, DomainResult: submission.DomainResult}
	if f.loseReply {
		f.loseReply = false
		return runtimeports.RestoreStageSettlementRefV1{}, errors.New("injected Restore Stage Settlement lost reply")
	}
	return f.value, nil
}
func (f *restoreStageSettlementFakeV1) InspectRestoreStageSettlementV1(context.Context, string) (runtimeports.RestoreStageSettlementFactV1, error) {
	panic("unused")
}
func (f *restoreStageSettlementFakeV1) InspectCurrentRestoreStageSettlementV1(context.Context, runtimeports.OperationSubjectV3, core.EffectIntentID) (runtimeports.RestoreStageSettlementRefV1, error) {
	return f.value, nil
}

func restoreStageActionProviderV1(component, capability string) runtimeports.ProviderBindingRefV2 {
	return runtimeports.ProviderBindingRefV2{BindingSetID: "binding-set-" + capability, BindingSetRevision: 1, ComponentID: runtimeports.ComponentIDV2(component), ManifestDigest: core.DigestBytes([]byte("manifest-" + capability)), ArtifactDigest: core.DigestBytes([]byte("artifact-" + capability)), Capability: runtimeports.CapabilityNameV2(capability)}
}

func restoreStageEvidenceSourceRefV1(suffix string) runtimeports.EvidenceSourceRegistrationRefV1 {
	return runtimeports.EvidenceSourceRegistrationRefV1{RegistrationID: "restore-stage-source-" + suffix, Revision: 1, FactDigest: core.DigestBytes([]byte("restore-stage-source-fact-" + suffix)), ConfigurationDigest: core.DigestBytes([]byte("restore-stage-source-config-" + suffix)), SourceID: runtimeports.RestoreStageEvidenceSourceIDV1, SourceEpoch: 1}
}
func restoreStageActionExternalRefV1(tenant core.TenantID, scope core.Digest, id, kind string) runtimeports.CheckpointExternalExactFactRefV2 {
	owner := restoreStageActionProviderV1("praxis/sandbox", "sandbox/"+kind)
	return runtimeports.CheckpointExternalExactFactRefV2{ContractVersion: "praxis.sandbox/" + kind + "/v1", SchemaRef: "praxis.sandbox/" + kind + "-schema/v1", Owner: runtimeports.CheckpointManifestSealOwnerBindingV2{BindingSetID: owner.BindingSetID, BindingRevision: owner.BindingSetRevision, ComponentID: string(owner.ComponentID), ManifestDigest: string(owner.ManifestDigest), ArtifactDigest: string(owner.ArtifactDigest), Capability: string(owner.Capability), FactKind: kind}, TenantID: string(tenant), ID: id, Revision: 1, Digest: string(core.DigestBytes([]byte(id))), ScopeDigest: string(scope)}
}
