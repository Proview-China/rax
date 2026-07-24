package fakes

import (
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// BuildRestoreStageSettlementFixtureV1 constructs one exact, fully sealed
// restore-stage closure for conformance and fault tests.
func BuildRestoreStageSettlementFixtureV1(suffix string, now time.Time) (ports.RestoreStageGovernanceCurrentProjectionV1, ports.RestoreStageDomainResultCurrentProjectionV1, ports.EvidenceLedgerRecordV2, ports.RestoreStageSettlementSubmissionV1, error) {
	tenant := core.TenantID("tenant-restore-stage-" + suffix)
	restoreAttempt := ports.RestoreAttemptRefV2{TenantID: tenant, ID: "restore-attempt-" + suffix, Revision: 2, Digest: core.DigestBytes([]byte("restore-attempt-" + suffix))}
	eligibility := ports.RestoreEligibilityRefV2{TenantID: tenant, ID: "restore-eligibility-" + suffix, Revision: 1, Digest: core.DigestBytes([]byte("restore-eligibility-" + suffix)), ExpiresUnixNano: now.Add(time.Hour).UnixNano()}
	identity := ports.RestoreIdentityReservationV2{SourceInstance: core.InstanceRef{ID: core.AgentInstanceID("source-" + suffix), Epoch: 1}, TargetInstance: core.InstanceRef{ID: core.AgentInstanceID("target-" + suffix), Epoch: 2}, TargetLease: core.SandboxLeaseRef{ID: core.SandboxLeaseID("lease-" + suffix), Epoch: 2}, TargetFenceEpoch: 2}
	snapshot := restoreStageExternalRefFixtureV1(tenant, "artifact-"+suffix)
	scope := core.ExecutionScope{Identity: core.AgentIdentityRef{TenantID: tenant, ID: core.AgentIdentityID("identity-" + suffix), Epoch: 1}, Lineage: core.LineageRef{ID: core.InstanceLineageID("lineage-" + suffix), PlanDigest: core.DigestBytes([]byte("plan-" + suffix))}, Instance: identity.TargetInstance, SandboxLease: &identity.TargetLease, AuthorityEpoch: identity.TargetFenceEpoch}
	scopeDigest, err := ports.ExecutionScopeDigestV2(scope)
	if err != nil {
		return ports.RestoreStageGovernanceCurrentProjectionV1{}, ports.RestoreStageDomainResultCurrentProjectionV1{}, ports.EvidenceLedgerRecordV2{}, ports.RestoreStageSettlementSubmissionV1{}, err
	}
	operation := ports.OperationSubjectV3{Kind: ports.RestoreStageOperationKindV1, ExecutionScope: scope, ExecutionScopeDigest: scopeDigest, CustomOperationID: restoreAttempt.ID, SubjectRevision: 1, CurrentProjectionRef: "restore-stage-current-" + suffix, CurrentProjectionDigest: core.DigestBytes([]byte("current-" + suffix)), CurrentProjectionRevision: 1}
	operationDigest, err := operation.DigestV3()
	if err != nil {
		return ports.RestoreStageGovernanceCurrentProjectionV1{}, ports.RestoreStageDomainResultCurrentProjectionV1{}, ports.EvidenceLedgerRecordV2{}, ports.RestoreStageSettlementSubmissionV1{}, err
	}
	effectID := core.EffectIntentID("restore-effect-" + suffix)
	intentDigest := core.DigestBytes([]byte("intent-" + suffix))
	admission := ports.OperationEffectAdmissionReceiptV3{OperationDigest: operationDigest, EffectID: effectID, IntentRevision: 1, IntentDigest: intentDigest, FactRevision: 1, State: "accepted"}
	authorization := ports.OperationReviewAuthorizationRefV4{ID: "authorization-" + suffix, Revision: 1, Digest: core.DigestBytes([]byte("authorization-" + suffix))}
	permitDigest := core.DigestBytes([]byte("permit-" + suffix))
	dispatch := ports.OperationDispatchAttemptRefV3{OperationDigest: operationDigest, EffectID: effectID, IntentRevision: 1, IntentDigest: intentDigest, PermitID: "permit-" + suffix, PermitRevision: 2, PermitDigest: permitDigest, AttemptID: "dispatch-attempt-" + suffix}
	enforcement := ports.OperationDispatchEnforcementPhaseRefV4{OperationDigest: operationDigest, EffectID: effectID, PermitID: dispatch.PermitID, PermitFactRevision: dispatch.PermitRevision, PermitDigest: permitDigest, AdmissionDigest: core.DigestBytes([]byte("dispatch-admission-" + suffix)), ReviewAuthorization: authorization, AttemptID: dispatch.AttemptID, SandboxAttempt: ports.OperationDispatchSandboxFactRefV4{ID: dispatch.AttemptID, Revision: 1, Digest: core.DigestBytes([]byte("sandbox-" + suffix)), ExpiresUnixNano: now.Add(time.Minute).UnixNano()}, Phase: ports.OperationDispatchEnforcementExecuteV4, ReceiptDigest: core.DigestBytes([]byte("receipt-" + suffix)), JournalRevision: 2, ValidatedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(time.Minute).UnixNano(), PrepareReceiptDigest: core.DigestBytes([]byte("prepare-" + suffix)), PreparedAttemptDigest: core.DigestBytes([]byte("prepared-" + suffix))}
	governance, err := ports.SealRestoreStageGovernanceCurrentProjectionV1(ports.RestoreStageGovernanceCurrentProjectionV1{RestoreAttempt: restoreAttempt, Eligibility: eligibility, Identity: identity, Operation: operation, EffectID: effectID, EffectRevision: 1, IntentDigest: intentDigest, Admission: admission, DispatchAdmissionDigest: enforcement.AdmissionDigest, Authorization: authorization, PermitID: dispatch.PermitID, PermitFactRevision: dispatch.PermitRevision, PermitDigest: permitDigest, BeginRecordRevision: 2, BeginRecordDigest: core.DigestBytes([]byte("begin-" + suffix)), DispatchAttempt: dispatch, ExecuteEnforcement: enforcement, MaterializationDigest: core.DigestBytes([]byte("materialization-" + suffix)), SnapshotArtifact: snapshot, CheckedUnixNano: enforcement.ValidatedUnixNano, ExpiresUnixNano: enforcement.ExpiresUnixNano}, now)
	if err != nil {
		return ports.RestoreStageGovernanceCurrentProjectionV1{}, ports.RestoreStageDomainResultCurrentProjectionV1{}, ports.EvidenceLedgerRecordV2{}, ports.RestoreStageSettlementSubmissionV1{}, err
	}
	owner := ports.ProviderBindingRefV2{BindingSetID: "sandbox-binding-" + suffix, BindingSetRevision: 1, ComponentID: "praxis/sandbox", ManifestDigest: core.DigestBytes([]byte("sandbox-manifest-" + suffix)), ArtifactDigest: core.DigestBytes([]byte("sandbox-artifact-" + suffix)), Capability: "sandbox/workspace-restore-stage"}
	schema := ports.SchemaRefV2{Namespace: "praxis.sandbox", Name: "workspace-restore-stage-fact", Version: "1.0.0", MediaType: "application/json", ContentDigest: core.DigestBytes([]byte("restore-stage-schema"))}
	domainRef := ports.RestoreStageDomainResultFactRefV1{Owner: owner, Kind: ports.RestoreStageDomainResultKindV1, ID: "workspace-stage-fact-" + suffix, Revision: 1, Digest: core.DigestBytes([]byte("workspace-stage-fact-" + suffix)), TenantID: tenant, Operation: operation, OperationDigest: operationDigest, EffectID: effectID, EffectRevision: 1, Attempt: dispatch, RestoreAttempt: restoreAttempt, Eligibility: eligibility, PayloadSchema: schema, PayloadDigest: core.DigestBytes([]byte("workspace-stage-payload-" + suffix)), PayloadRevision: 1, AuthoritativeTime: now.Add(-time.Second).UnixNano()}
	domain, err := ports.SealRestoreStageDomainResultCurrentProjectionV1(ports.RestoreStageDomainResultCurrentProjectionV1{Fact: domainRef, CheckedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(time.Minute).UnixNano()}, now)
	if err != nil {
		return ports.RestoreStageGovernanceCurrentProjectionV1{}, ports.RestoreStageDomainResultCurrentProjectionV1{}, ports.EvidenceLedgerRecordV2{}, ports.RestoreStageSettlementSubmissionV1{}, err
	}
	ledger := ports.EvidenceLedgerScopeV2{Partition: ports.EvidencePartitionInstance, TenantID: tenant, IdentityID: scope.Identity.ID, LineageID: scope.Lineage.ID, InstanceID: scope.Instance.ID}
	ledgerDigest, err := ledger.DigestV2()
	if err != nil {
		return ports.RestoreStageGovernanceCurrentProjectionV1{}, ports.RestoreStageDomainResultCurrentProjectionV1{}, ports.EvidenceLedgerRecordV2{}, ports.RestoreStageSettlementSubmissionV1{}, err
	}
	ownerFact := domainRef.EvidenceOwnerFactV2()
	candidate := ports.EvidenceEventCandidateV2{ContractVersion: ports.EvidenceContractVersionV2, LedgerScope: ledger, EventID: "restore-stage-domain-result-" + suffix, RegistrationID: "restore-stage-registration-" + suffix, RegistrationRevision: 1, SourceConfigurationDigest: core.DigestBytes([]byte("restore-stage-source-config-" + suffix)), SourcePolicy: ports.EvidenceSourcePolicyBindingRefV2{Ref: "restore-stage-source-policy-" + suffix, Digest: core.DigestBytes([]byte("restore-stage-policy-" + suffix)), Revision: 1}, SourceID: "praxis.sandbox/workspace-restore-stage", SourceEpoch: 1, SourceSequence: 1, TrustClass: ports.EvidenceTrustAuthoritativeFact, EventKind: "praxis.sandbox/workspace-restore-stage-fact", CustomClass: "praxis.sandbox/authoritative-fact", ExecutionScope: scope, Payload: ports.EvidencePayloadRefV2{Schema: schema, ContentDigest: domainRef.PayloadDigest, Revision: domainRef.PayloadRevision, Length: 1, Ref: "sandbox-fact://" + domainRef.ID}, CorrelationID: restoreAttempt.ID, Producer: ports.EvidenceProducerBindingRefV2(owner), Authority: ports.AuthorityBindingRefV2{Ref: "restore-stage-authority-" + suffix, Digest: core.DigestBytes([]byte("restore-stage-authority-" + suffix)), Revision: 1, Epoch: scope.AuthorityEpoch}, OwnerFact: &ownerFact, ObservedUnixNano: now.Add(-time.Second).UnixNano()}
	candidateDigest, err := candidate.DigestV2()
	if err != nil {
		return ports.RestoreStageGovernanceCurrentProjectionV1{}, ports.RestoreStageDomainResultCurrentProjectionV1{}, ports.EvidenceLedgerRecordV2{}, ports.RestoreStageSettlementSubmissionV1{}, err
	}
	record := ports.EvidenceLedgerRecordV2{Ref: ports.EvidenceRecordRefV2{LedgerScopeDigest: ledgerDigest, Sequence: 1, RecordDigest: core.DigestBytes([]byte("restore-stage-record-" + suffix))}, Candidate: candidate, CandidateDigest: candidateDigest, PreviousRecordDigest: core.DigestBytes([]byte("restore-stage-genesis-" + suffix)), IngestedUnixNano: now.UnixNano()}
	submission, err := ports.SealRestoreStageSettlementSubmissionV1(ports.RestoreStageSettlementSubmissionV1{ID: "restore-stage-settlement-" + suffix, Operation: operation, OperationDigest: operationDigest, EffectID: effectID, EffectRevision: 1, RestoreAttempt: restoreAttempt, Eligibility: eligibility, Governance: governance, DomainResult: domainRef, Evidence: record.Ref, IdempotencyKey: "restore-stage-settlement-key-" + suffix, SettledUnixNano: now.UnixNano()})
	return governance, domain, record, submission, err
}

func restoreStageExternalRefFixtureV1(tenant core.TenantID, id string) ports.CheckpointExternalExactFactRefV2 {
	return ports.CheckpointExternalExactFactRefV2{ContractVersion: "praxis.sandbox/snapshot-artifact/v2", SchemaRef: "praxis.sandbox/snapshot-artifact-schema/v2", Owner: ports.CheckpointManifestSealOwnerBindingV2{BindingSetID: "sandbox-binding", BindingRevision: 1, ComponentID: "praxis/sandbox", ManifestDigest: string(core.DigestBytes([]byte("manifest"))), ArtifactDigest: string(core.DigestBytes([]byte("artifact-code"))), Capability: "snapshot-artifact-current", FactKind: "snapshot-artifact-fact"}, TenantID: string(tenant), ID: id, Revision: 1, Digest: string(core.DigestBytes([]byte(id))), ScopeDigest: string(core.DigestBytes([]byte("source-scope")))}
}
