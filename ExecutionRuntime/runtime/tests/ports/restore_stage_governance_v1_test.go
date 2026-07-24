package ports_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestRestoreStageOperationPayloadV1StrictCanonicalAndExact(t *testing.T) {
	now := time.Unix(1_950_200_000, 0)
	payload := restoreStagePayloadFixtureV1(now)
	encoded, err := payload.CanonicalBytesV1()
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := ports.DecodeRestoreStageOperationPayloadV1(encoded)
	if err != nil || decoded != payload {
		t.Fatalf("decode=%#v err=%v", decoded, err)
	}
	if _, err := ports.DecodeRestoreStageOperationPayloadV1(append([]byte(" "), encoded...)); err == nil {
		t.Fatal("non-canonical whitespace was accepted")
	}
	unknown := bytes.Replace(encoded, []byte(`{"contract_version":`), []byte(`{"unknown":true,"contract_version":`), 1)
	if _, err := ports.DecodeRestoreStageOperationPayloadV1(unknown); err == nil {
		t.Fatal("unknown field was accepted")
	}
	splice := payload
	splice.Identity.TargetInstance.Epoch++
	changed, _ := splice.CanonicalBytesV1()
	if bytes.Equal(encoded, changed) {
		t.Fatal("identity splice did not change canonical payload")
	}
}

func TestRestoreStageGovernanceProjectionV1RejectsTargetAndEnforcementSplice(t *testing.T) {
	now := time.Unix(1_950_200_000, 0)
	projection := restoreStageProjectionFixtureV1(t, now)
	if err := projection.Validate(now); err != nil {
		t.Fatal(err)
	}
	splice := projection
	splice.Identity.TargetFenceEpoch++
	if err := splice.Validate(now); err == nil {
		t.Fatal("target Fence splice was accepted")
	}
	splice = projection
	splice.ExecuteEnforcement.AttemptID = "other-attempt"
	if err := splice.Validate(now); err == nil {
		t.Fatal("enforcement attempt splice was accepted")
	}
	if err := projection.Validate(time.Unix(0, projection.ExpiresUnixNano)); err == nil {
		t.Fatal("projection remained current at exact expiry")
	}
}

func TestRestoreStageSettlementV1OpaqueBindingAndEvidence(t *testing.T) {
	now := time.Unix(1_950_200_000, 0)
	governance := restoreStageProjectionFixtureV1(t, now)
	domain, record := restoreStageSettlementDomainAndEvidenceFixtureV1(t, governance, now)
	if err := ports.ValidateRestoreStageEvidenceRecordV1(record, domain); err != nil {
		t.Fatal(err)
	}
	submission, err := ports.SealRestoreStageSettlementSubmissionV1(ports.RestoreStageSettlementSubmissionV1{
		ID: "restore-stage-settlement", Operation: governance.Operation, OperationDigest: domain.OperationDigest, EffectID: governance.EffectID, EffectRevision: governance.EffectRevision,
		RestoreAttempt: governance.RestoreAttempt, Eligibility: governance.Eligibility, Governance: governance, DomainResult: domain, Evidence: record.Ref, IdempotencyKey: "restore-stage-settlement-key", SettledUnixNano: now.UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	fact, err := ports.SealRestoreStageSettlementFactV1(ports.RestoreStageSettlementFactV1{Submission: submission})
	if err != nil || fact.RefV1().Validate() != nil {
		t.Fatalf("fact=%#v err=%v", fact, err)
	}
	splice := domain
	splice.Eligibility.ID = "other-eligibility"
	if _, err := ports.SealRestoreStageSettlementSubmissionV1(ports.RestoreStageSettlementSubmissionV1{
		ID: "restore-stage-settlement-splice", Operation: governance.Operation, OperationDigest: domain.OperationDigest, EffectID: governance.EffectID, EffectRevision: governance.EffectRevision,
		RestoreAttempt: governance.RestoreAttempt, Eligibility: governance.Eligibility, Governance: governance, DomainResult: splice, Evidence: record.Ref, IdempotencyKey: "restore-stage-settlement-splice-key", SettledUnixNano: now.UnixNano(),
	}); err == nil {
		t.Fatal("cross-Eligibility DomainResult splice was accepted")
	}
	observation := record
	observation.Candidate.TrustClass = ports.EvidenceTrustObservation
	observation.Candidate.OwnerFact = nil
	if err := ports.ValidateRestoreStageEvidenceRecordV1(observation, domain); err == nil {
		t.Fatal("Observation was accepted as authoritative Restore Stage Evidence")
	}
}

func restoreStageSettlementDomainAndEvidenceFixtureV1(t *testing.T, governance ports.RestoreStageGovernanceCurrentProjectionV1, now time.Time) (ports.RestoreStageDomainResultFactRefV1, ports.EvidenceLedgerRecordV2) {
	t.Helper()
	owner := ports.ProviderBindingRefV2{BindingSetID: "sandbox-binding", BindingSetRevision: 1, ComponentID: "praxis/sandbox", ManifestDigest: core.DigestBytes([]byte("sandbox-manifest")), ArtifactDigest: core.DigestBytes([]byte("sandbox-artifact")), Capability: "sandbox/workspace-restore-stage"}
	schema := ports.SchemaRefV2{Namespace: "praxis.sandbox", Name: "workspace-restore-stage-fact", Version: "1.0.0", MediaType: "application/json", ContentDigest: core.DigestBytes([]byte("restore-stage-schema"))}
	opDigest, _ := governance.Operation.DigestV3()
	domain := ports.RestoreStageDomainResultFactRefV1{Owner: owner, Kind: ports.RestoreStageDomainResultKindV1, ID: "workspace-stage-fact", Revision: 1, Digest: core.DigestBytes([]byte("workspace-stage-fact")), TenantID: governance.RestoreAttempt.TenantID, Operation: governance.Operation, OperationDigest: opDigest, EffectID: governance.EffectID, EffectRevision: governance.EffectRevision, Attempt: governance.DispatchAttempt, RestoreAttempt: governance.RestoreAttempt, Eligibility: governance.Eligibility, PayloadSchema: schema, PayloadDigest: core.DigestBytes([]byte("workspace-stage-payload")), PayloadRevision: 1, AuthoritativeTime: now.Add(-time.Second).UnixNano()}
	ledger := ports.EvidenceLedgerScopeV2{Partition: ports.EvidencePartitionInstance, TenantID: governance.Operation.ExecutionScope.Identity.TenantID, IdentityID: governance.Operation.ExecutionScope.Identity.ID, LineageID: governance.Operation.ExecutionScope.Lineage.ID, InstanceID: governance.Operation.ExecutionScope.Instance.ID}
	ledgerDigest, _ := ledger.DigestV2()
	candidate := ports.EvidenceEventCandidateV2{ContractVersion: ports.EvidenceContractVersionV2, LedgerScope: ledger, EventID: "restore-stage-domain-result", RegistrationID: "restore-stage-registration", RegistrationRevision: 1, SourceConfigurationDigest: core.DigestBytes([]byte("restore-stage-source-config")), SourcePolicy: ports.EvidenceSourcePolicyBindingRefV2{Ref: "restore-stage-source-policy", Digest: core.DigestBytes([]byte("restore-stage-policy")), Revision: 1}, SourceID: "praxis.sandbox/workspace-restore-stage", SourceEpoch: 1, SourceSequence: 1, TrustClass: ports.EvidenceTrustAuthoritativeFact, EventKind: "praxis.sandbox/workspace-restore-stage-fact", CustomClass: "praxis.sandbox/authoritative-fact", ExecutionScope: governance.Operation.ExecutionScope, Payload: ports.EvidencePayloadRefV2{Schema: schema, ContentDigest: domain.PayloadDigest, Revision: domain.PayloadRevision, Length: 1, Ref: "sandbox-fact://workspace-stage-fact"}, Causation: []ports.EvidenceCausationRefV2{}, CorrelationID: governance.RestoreAttempt.ID, Producer: ports.EvidenceProducerBindingRefV2(owner), Authority: ports.AuthorityBindingRefV2{Ref: "restore-stage-authority", Digest: core.DigestBytes([]byte("restore-stage-authority")), Revision: 1, Epoch: governance.Operation.ExecutionScope.AuthorityEpoch}, OwnerFact: ptrRestoreStageEvidenceOwnerFactV1(domain.EvidenceOwnerFactV2()), ObservedUnixNano: now.Add(-time.Second).UnixNano()}
	candidateDigest, err := candidate.DigestV2()
	if err != nil {
		t.Fatal(err)
	}
	record := ports.EvidenceLedgerRecordV2{Ref: ports.EvidenceRecordRefV2{LedgerScopeDigest: ledgerDigest, Sequence: 1, RecordDigest: core.DigestBytes([]byte("restore-stage-record"))}, Candidate: candidate, CandidateDigest: candidateDigest, PreviousRecordDigest: core.DigestBytes([]byte("restore-stage-genesis")), IngestedUnixNano: now.UnixNano()}
	return domain, record
}

func ptrRestoreStageEvidenceOwnerFactV1(value ports.EvidenceOwnerFactRefV2) *ports.EvidenceOwnerFactRefV2 {
	return &value
}

func restoreStagePayloadFixtureV1(now time.Time) ports.RestoreStageOperationPayloadV1 {
	tenant := core.TenantID("tenant-restore-stage")
	attempt := ports.RestoreAttemptRefV2{TenantID: tenant, ID: "restore-attempt", Revision: 2, Digest: core.DigestBytes([]byte("restore-attempt"))}
	eligibility := ports.RestoreEligibilityRefV2{TenantID: tenant, ID: "restore-eligibility", Revision: 1, Digest: core.DigestBytes([]byte("restore-eligibility")), ExpiresUnixNano: now.Add(time.Hour).UnixNano()}
	return ports.RestoreStageOperationPayloadV1{ContractVersion: ports.RestoreStageGovernanceContractVersionV1, RestoreAttempt: attempt, Eligibility: eligibility, Identity: ports.RestoreIdentityReservationV2{SourceInstance: core.InstanceRef{ID: "source", Epoch: 1}, TargetInstance: core.InstanceRef{ID: "target", Epoch: 2}, TargetLease: core.SandboxLeaseRef{ID: "lease", Epoch: 2}, TargetFenceEpoch: 2}, SnapshotArtifact: restoreStageExternalRefV1(tenant, "artifact")}
}

func restoreStageProjectionFixtureV1(t *testing.T, now time.Time) ports.RestoreStageGovernanceCurrentProjectionV1 {
	t.Helper()
	payload := restoreStagePayloadFixtureV1(now)
	scope := core.ExecutionScope{Identity: core.AgentIdentityRef{TenantID: payload.RestoreAttempt.TenantID, ID: "identity", Epoch: 1}, Lineage: core.LineageRef{ID: "lineage", PlanDigest: core.DigestBytes([]byte("plan"))}, Instance: payload.Identity.TargetInstance, SandboxLease: &payload.Identity.TargetLease, AuthorityEpoch: payload.Identity.TargetFenceEpoch}
	scopeDigest, _ := ports.ExecutionScopeDigestV2(scope)
	operation := ports.OperationSubjectV3{Kind: ports.RestoreStageOperationKindV1, ExecutionScope: scope, ExecutionScopeDigest: scopeDigest, CustomOperationID: payload.RestoreAttempt.ID, SubjectRevision: 1, CurrentProjectionRef: "restore-stage-current", CurrentProjectionDigest: core.DigestBytes([]byte("current")), CurrentProjectionRevision: 1}
	operationDigest, _ := operation.DigestV3()
	intentDigest := core.DigestBytes([]byte("intent"))
	admission := ports.OperationEffectAdmissionReceiptV3{OperationDigest: operationDigest, EffectID: "restore-effect", IntentRevision: 1, IntentDigest: intentDigest, FactRevision: 1, State: "accepted"}
	authorization := ports.OperationReviewAuthorizationRefV4{ID: "authorization", Revision: 1, Digest: core.DigestBytes([]byte("authorization"))}
	permitDigest := core.DigestBytes([]byte("permit"))
	attempt := ports.OperationDispatchAttemptRefV3{OperationDigest: operationDigest, EffectID: admission.EffectID, IntentRevision: 1, IntentDigest: intentDigest, PermitID: "permit", PermitRevision: 2, PermitDigest: permitDigest, AttemptID: "dispatch-attempt"}
	enforcement := ports.OperationDispatchEnforcementPhaseRefV4{OperationDigest: operationDigest, EffectID: admission.EffectID, PermitID: attempt.PermitID, PermitFactRevision: attempt.PermitRevision, PermitDigest: permitDigest, AdmissionDigest: core.DigestBytes([]byte("dispatch-admission")), ReviewAuthorization: authorization, AttemptID: attempt.AttemptID, SandboxAttempt: ports.OperationDispatchSandboxFactRefV4{ID: attempt.AttemptID, Revision: 1, Digest: core.DigestBytes([]byte("sandbox")), ExpiresUnixNano: now.Add(time.Minute).UnixNano()}, Phase: ports.OperationDispatchEnforcementExecuteV4, ReceiptDigest: core.DigestBytes([]byte("receipt")), JournalRevision: 2, ValidatedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(time.Minute).UnixNano(), PrepareReceiptDigest: core.DigestBytes([]byte("prepare")), PreparedAttemptDigest: core.DigestBytes([]byte("prepared"))}
	value, err := ports.SealRestoreStageGovernanceCurrentProjectionV1(ports.RestoreStageGovernanceCurrentProjectionV1{RestoreAttempt: payload.RestoreAttempt, Eligibility: payload.Eligibility, Identity: payload.Identity, Operation: operation, EffectID: admission.EffectID, EffectRevision: 1, IntentDigest: intentDigest, Admission: admission, DispatchAdmissionDigest: enforcement.AdmissionDigest, Authorization: authorization, PermitID: attempt.PermitID, PermitFactRevision: attempt.PermitRevision, PermitDigest: permitDigest, BeginRecordRevision: 2, BeginRecordDigest: core.DigestBytes([]byte("begin")), DispatchAttempt: attempt, ExecuteEnforcement: enforcement, MaterializationDigest: core.DigestBytes([]byte("materialization")), SnapshotArtifact: payload.SnapshotArtifact, CheckedUnixNano: enforcement.ValidatedUnixNano, ExpiresUnixNano: enforcement.ExpiresUnixNano}, now)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func restoreStageExternalRefV1(tenant core.TenantID, id string) ports.CheckpointExternalExactFactRefV2 {
	scope := core.DigestBytes([]byte("source-scope"))
	return ports.CheckpointExternalExactFactRefV2{ContractVersion: "praxis.sandbox/snapshot-artifact/v2", SchemaRef: "praxis.sandbox/snapshot-artifact-schema/v2", Owner: ports.CheckpointManifestSealOwnerBindingV2{BindingSetID: "sandbox-binding", BindingRevision: 1, ComponentID: "praxis/sandbox", ManifestDigest: string(core.DigestBytes([]byte("manifest"))), ArtifactDigest: string(core.DigestBytes([]byte("artifact-code"))), Capability: "snapshot-artifact-current", FactKind: "snapshot-artifact-fact"}, TenantID: string(tenant), ID: id, Revision: 1, Digest: string(core.DigestBytes([]byte(id))), ScopeDigest: string(scope)}
}
