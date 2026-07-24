package testsupport

import (
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// OperationSettlementSubmissionV4 returns a structurally complete public-only
// fixture for ports/control tests. It contains no currentness or production
// eligibility claim.
func OperationSettlementSubmissionV4() ports.OperationSettlementSubmissionV4 {
	now := time.Unix(400_000, 0)
	expires := now.Add(time.Minute).UnixNano()
	execution := core.ExecutionScope{
		Identity: core.AgentIdentityRef{TenantID: "tenant-settlement-test", ID: "identity-settlement-test", Epoch: 1},
		Lineage:  core.LineageRef{ID: "lineage-settlement-test", PlanDigest: core.DigestBytes([]byte("plan"))},
		Instance: core.InstanceRef{ID: "instance-settlement-test", Epoch: 1}, AuthorityEpoch: 1,
	}
	executionDigest, err := ports.ExecutionScopeDigestV2(execution)
	if err != nil {
		panic(err)
	}
	operation := ports.OperationSubjectV3{
		Kind: ports.OperationScopeActivationV3, ExecutionScope: execution, ExecutionScopeDigest: executionDigest,
		ActivationAttemptID: "activation-settlement-test", SubjectRevision: 1,
		CurrentProjectionRef: "projection-settlement-test", CurrentProjectionRevision: 1,
		CurrentProjectionDigest: core.DigestBytes([]byte("projection")),
	}
	operationDigest, err := operation.DigestV3()
	if err != nil {
		panic(err)
	}
	attempt := ports.OperationDispatchAttemptRefV3{
		OperationDigest: operationDigest, EffectID: "effect-settlement-test", IntentRevision: 1,
		IntentDigest: core.DigestBytes([]byte("intent")), PermitID: "permit-settlement-test", PermitRevision: 1,
		PermitDigest: core.DigestBytes([]byte("permit")), AttemptID: "attempt-settlement-test",
	}
	authorization := ports.OperationReviewAuthorizationRefV4{ID: "authorization-settlement-test", Revision: 1, Digest: core.DigestBytes([]byte("authorization"))}
	sandboxAttempt := ports.OperationDispatchSandboxFactRefV4{ID: attempt.AttemptID, Revision: 1, Digest: core.DigestBytes([]byte("sandbox-attempt")), ExpiresUnixNano: expires}
	makeBinding := func(phase ports.OperationDispatchEnforcementPhaseV4, sequence uint64) ports.OperationSettlementEvidenceBindingV4 {
		name := string(phase)
		record := ports.OperationScopeEvidenceRecordRefV3{LedgerScopeDigest: core.DigestBytes([]byte("ledger-" + name)), Sequence: sequence, RecordDigest: core.DigestBytes([]byte("record-" + name))}
		issued := ports.OperationScopeEvidenceQualificationRefV3{ID: "qualification-" + name, Revision: 1, Digest: core.DigestBytes([]byte("issued-" + name)), ExpiresUnixNano: expires}
		final := issued
		final.Revision = 2
		final.Digest = core.DigestBytes([]byte("final-" + name))
		phaseRef := ports.OperationDispatchEnforcementPhaseRefV4{
			OperationDigest: operationDigest, EffectID: attempt.EffectID, PermitID: attempt.PermitID, PermitFactRevision: 2,
			PermitDigest: core.DigestBytes([]byte("permit-v4")), AdmissionDigest: core.DigestBytes([]byte("admission")), ReviewAuthorization: authorization,
			AttemptID: attempt.AttemptID, SandboxAttempt: sandboxAttempt, Phase: phase, ReceiptDigest: core.DigestBytes([]byte("receipt-" + name)),
			JournalRevision: 1, ValidatedUnixNano: now.UnixNano(), ExpiresUnixNano: expires,
		}
		if phase == ports.OperationDispatchEnforcementExecuteV4 {
			phaseRef.JournalRevision = 2
			phaseRef.PrepareReceiptDigest = core.DigestBytes([]byte("receipt-prepare"))
			phaseRef.PreparedAttemptDigest = core.DigestBytes([]byte("prepared-attempt"))
		}
		return ports.OperationSettlementEvidenceBindingV4{
			Phase:               phase,
			Consumption:         ports.OperationScopeEvidenceConsumptionRefV3{ID: "consumption-" + name, Revision: 1, Digest: core.DigestBytes([]byte("consumption-" + name)), Record: record},
			IssuedQualification: issued, FinalQualification: final, Record: record, CandidateDigest: core.DigestBytes([]byte("candidate-" + name)),
			Handoff: ports.OperationScopeEvidenceProviderHandoffRefV3{ID: "handoff-" + name, Revision: 1, Digest: core.DigestBytes([]byte("handoff-" + name)), ExpiresUnixNano: expires},
			Attempt: attempt, EnforcementPhase: phaseRef, OperationScopeDigest: core.DigestBytes([]byte("scope-" + name)),
		}
	}
	evidence := []ports.OperationSettlementEvidenceBindingV4{makeBinding(ports.OperationDispatchEnforcementExecuteV4, 2), makeBinding(ports.OperationDispatchEnforcementPrepareV4, 1)}
	scopeSet, err := ports.DigestOperationSettlementScopeSetV4(evidence)
	if err != nil {
		panic(err)
	}
	provider := ports.ProviderBindingRefV2{BindingSetID: "binding-settlement-test", BindingSetRevision: 1, ComponentID: "praxis.sandbox/provider", ManifestDigest: core.DigestBytes([]byte("manifest")), ArtifactDigest: core.DigestBytes([]byte("artifact")), Capability: "praxis.sandbox/settle"}
	domain := ports.OperationSettlementDomainResultFactRefV4{
		Owner: provider, Kind: "praxis.sandbox/domain-result", ID: "domain-settlement-test", Revision: 1, Digest: core.DigestBytes([]byte("domain")),
		TenantID: execution.Identity.TenantID, EffectID: attempt.EffectID, EffectRevision: attempt.IntentRevision,
		Operation: operation, OperationDigest: operationDigest, Attempt: attempt,
		Schema:        ports.SchemaRefV2{Namespace: "praxis.sandbox", Name: "domain-result", Version: "1.0.0", MediaType: "application/json", ContentDigest: core.DigestBytes([]byte("schema"))},
		PayloadDigest: core.DigestBytes([]byte("payload")), PayloadRevision: 1, AuthoritativeTime: now.UnixNano(),
	}
	value, err := ports.SealOperationSettlementSubmissionV4(ports.OperationSettlementSubmissionV4{
		ID: "settlement-test", TenantID: execution.Identity.TenantID, Operation: operation, OperationDigest: operationDigest,
		OperationScopeDigest: scopeSet, EffectID: attempt.EffectID, ExpectedEffectRevision: 3,
		Owner:        ports.EffectOwnerRefV2{Role: ports.OwnerSettlement, ComponentID: provider.ComponentID, ManifestDigest: provider.ManifestDigest},
		DomainResult: domain, Evidence: evidence, IdempotencyKey: "settlement-idempotency-test",
		ConflictDomain: core.DigestBytes([]byte("conflict")), SettledUnixNano: now.UnixNano(),
	})
	if err != nil {
		panic(err)
	}
	return value
}
