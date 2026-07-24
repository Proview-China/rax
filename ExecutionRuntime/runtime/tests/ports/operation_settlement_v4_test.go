package ports_test

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestOperationSettlementV4CanonicalPhaseSet(t *testing.T) {
	base := operationSettlementSubmissionV4(t)
	reversed := base
	reversed.Evidence = []ports.OperationSettlementEvidenceBindingV4{base.Evidence[1], base.Evidence[0]}
	reversed.Digest = ""
	sealed, err := ports.SealOperationSettlementSubmissionV4(reversed)
	if err != nil {
		t.Fatal(err)
	}
	if sealed.Digest != base.Digest || sealed.Evidence[0].Phase != ports.OperationDispatchEnforcementPrepareV4 || sealed.Evidence[1].Phase != ports.OperationDispatchEnforcementExecuteV4 {
		t.Fatalf("inverse phase input did not seal to the stable prepare/execute canonical form: %#v", sealed.Evidence)
	}
	resealed, err := ports.SealOperationSettlementSubmissionV4(sealed)
	if err != nil || resealed.Digest != sealed.Digest {
		t.Fatalf("repeat Seal was not stable: digest=%s err=%v", resealed.Digest, err)
	}
}

func TestOperationSettlementV4RejectsIncompletePhaseAndScopeSets(t *testing.T) {
	base := operationSettlementSubmissionV4(t)
	tests := []struct {
		name   string
		mutate func(*ports.OperationSettlementSubmissionV4)
	}{
		{name: "missing", mutate: func(value *ports.OperationSettlementSubmissionV4) { value.Evidence = value.Evidence[:1] }},
		{name: "duplicate", mutate: func(value *ports.OperationSettlementSubmissionV4) { value.Evidence[1] = value.Evidence[0] }},
		{name: "extra", mutate: func(value *ports.OperationSettlementSubmissionV4) {
			value.Evidence = append(value.Evidence, value.Evidence[0])
		}},
		{name: "phase swap without matching phase ref", mutate: func(value *ports.OperationSettlementSubmissionV4) {
			value.Evidence[0].Phase = ports.OperationDispatchEnforcementExecuteV4
		}},
		{name: "single phase scope digest", mutate: func(value *ports.OperationSettlementSubmissionV4) {
			value.Evidence[0].OperationScopeDigest = core.DigestBytes([]byte("other-scope"))
		}},
		{name: "aggregate scope digest", mutate: func(value *ports.OperationSettlementSubmissionV4) {
			value.OperationScopeDigest = core.DigestBytes([]byte("other-set"))
		}},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			changed := base
			changed.Evidence = append([]ports.OperationSettlementEvidenceBindingV4{}, base.Evidence...)
			testCase.mutate(&changed)
			changed.Digest = ""
			if _, err := ports.SealOperationSettlementSubmissionV4(changed); err == nil {
				t.Fatal("invalid phase/scope set was sealed")
			}
		})
	}
}

func TestOperationSettlementV4TypedDomainOperationIdentityIsExact(t *testing.T) {
	base := operationSettlementSubmissionV4(t)
	changed := base
	changed.DomainResult.Operation.ActivationAttemptID = "another-activation"
	changed.DomainResult.OperationDigest, _ = changed.DomainResult.Operation.DigestV3()
	changed.Digest = ""
	if _, err := ports.SealOperationSettlementSubmissionV4(changed); err == nil {
		t.Fatal("DomainResult with another explicit operation identity was sealed")
	}
}

func operationSettlementSubmissionV4(t *testing.T) ports.OperationSettlementSubmissionV4 {
	t.Helper()
	now := time.Unix(400_000, 0)
	expires := now.Add(time.Minute).UnixNano()
	execution := core.ExecutionScope{
		Identity:       core.AgentIdentityRef{TenantID: "tenant-settlement-ports", ID: "identity-settlement-ports", Epoch: 1},
		Lineage:        core.LineageRef{ID: "lineage-settlement-ports", PlanDigest: core.DigestBytes([]byte("plan"))},
		Instance:       core.InstanceRef{ID: "instance-settlement-ports", Epoch: 1},
		AuthorityEpoch: 1,
	}
	executionDigest, err := ports.ExecutionScopeDigestV2(execution)
	if err != nil {
		t.Fatal(err)
	}
	operation := ports.OperationSubjectV3{
		Kind: ports.OperationScopeActivationV3, ExecutionScope: execution, ExecutionScopeDigest: executionDigest,
		ActivationAttemptID: "activation-settlement-ports", SubjectRevision: 1,
		CurrentProjectionRef: "projection-settlement-ports", CurrentProjectionRevision: 1,
		CurrentProjectionDigest: core.DigestBytes([]byte("projection")),
	}
	operationDigest, err := operation.DigestV3()
	if err != nil {
		t.Fatal(err)
	}
	attempt := ports.OperationDispatchAttemptRefV3{
		OperationDigest: operationDigest, EffectID: "effect-settlement-ports", IntentRevision: 1,
		IntentDigest: core.DigestBytes([]byte("intent")), PermitID: "permit-settlement-ports", PermitRevision: 1,
		PermitDigest: core.DigestBytes([]byte("permit")), AttemptID: "attempt-settlement-ports",
	}
	authorization := ports.OperationReviewAuthorizationRefV4{ID: "authorization-settlement-ports", Revision: 1, Digest: core.DigestBytes([]byte("authorization"))}
	sandboxAttempt := ports.OperationDispatchSandboxFactRefV4{ID: attempt.AttemptID, Revision: 1, Digest: core.DigestBytes([]byte("sandbox-attempt")), ExpiresUnixNano: expires}
	makeBinding := func(phase ports.OperationDispatchEnforcementPhaseV4, sequence uint64) ports.OperationSettlementEvidenceBindingV4 {
		phaseName := string(phase)
		record := ports.OperationScopeEvidenceRecordRefV3{LedgerScopeDigest: core.DigestBytes([]byte("ledger-" + phaseName)), Sequence: sequence, RecordDigest: core.DigestBytes([]byte("record-" + phaseName))}
		issued := ports.OperationScopeEvidenceQualificationRefV3{ID: "qualification-" + phaseName, Revision: 1, Digest: core.DigestBytes([]byte("issued-" + phaseName)), ExpiresUnixNano: expires}
		final := issued
		final.Revision = 2
		final.Digest = core.DigestBytes([]byte("final-" + phaseName))
		phaseRef := ports.OperationDispatchEnforcementPhaseRefV4{
			OperationDigest: operationDigest, EffectID: attempt.EffectID, PermitID: attempt.PermitID,
			PermitFactRevision: 2, PermitDigest: core.DigestBytes([]byte("permit-v4")), AdmissionDigest: core.DigestBytes([]byte("admission")),
			ReviewAuthorization: authorization, AttemptID: attempt.AttemptID, SandboxAttempt: sandboxAttempt, Phase: phase,
			ReceiptDigest: core.DigestBytes([]byte("receipt-" + phaseName)), JournalRevision: 1,
			ValidatedUnixNano: now.UnixNano(), ExpiresUnixNano: expires,
		}
		if phase == ports.OperationDispatchEnforcementExecuteV4 {
			phaseRef.JournalRevision = 2
			phaseRef.PrepareReceiptDigest = core.DigestBytes([]byte("receipt-prepare"))
			phaseRef.PreparedAttemptDigest = core.DigestBytes([]byte("prepared-attempt"))
		}
		return ports.OperationSettlementEvidenceBindingV4{
			Phase:               phase,
			Consumption:         ports.OperationScopeEvidenceConsumptionRefV3{ID: "consumption-" + phaseName, Revision: 1, Digest: core.DigestBytes([]byte("consumption-" + phaseName)), Record: record},
			IssuedQualification: issued, FinalQualification: final, Record: record,
			CandidateDigest: core.DigestBytes([]byte("candidate-" + phaseName)),
			Handoff:         ports.OperationScopeEvidenceProviderHandoffRefV3{ID: "handoff-" + phaseName, Revision: 1, Digest: core.DigestBytes([]byte("handoff-" + phaseName)), ExpiresUnixNano: expires},
			Attempt:         attempt, EnforcementPhase: phaseRef, OperationScopeDigest: core.DigestBytes([]byte("scope-" + phaseName)),
		}
	}
	evidence := []ports.OperationSettlementEvidenceBindingV4{makeBinding(ports.OperationDispatchEnforcementExecuteV4, 2), makeBinding(ports.OperationDispatchEnforcementPrepareV4, 1)}
	scopeSet, err := ports.DigestOperationSettlementScopeSetV4(evidence)
	if err != nil {
		t.Fatal(err)
	}
	provider := ports.ProviderBindingRefV2{BindingSetID: "binding-settlement-ports", BindingSetRevision: 1, ComponentID: "praxis.sandbox/provider", ManifestDigest: core.DigestBytes([]byte("manifest")), ArtifactDigest: core.DigestBytes([]byte("artifact")), Capability: "praxis.sandbox/settle"}
	domain := ports.OperationSettlementDomainResultFactRefV4{
		Owner: provider, Kind: "praxis.sandbox/domain-result", ID: "domain-settlement-ports", Revision: 1, Digest: core.DigestBytes([]byte("domain")),
		TenantID: execution.Identity.TenantID, EffectID: attempt.EffectID, EffectRevision: attempt.IntentRevision,
		Operation: operation, OperationDigest: operationDigest, Attempt: attempt,
		Schema:        ports.SchemaRefV2{Namespace: "praxis.sandbox", Name: "domain-result", Version: "1.0.0", MediaType: "application/json", ContentDigest: core.DigestBytes([]byte("schema"))},
		PayloadDigest: core.DigestBytes([]byte("payload")), PayloadRevision: 1, AuthoritativeTime: now.UnixNano(),
	}
	owner := ports.EffectOwnerRefV2{Role: ports.OwnerSettlement, ComponentID: provider.ComponentID, ManifestDigest: provider.ManifestDigest}
	value, err := ports.SealOperationSettlementSubmissionV4(ports.OperationSettlementSubmissionV4{
		ID: "settlement-ports", TenantID: execution.Identity.TenantID, Operation: operation, OperationDigest: operationDigest,
		OperationScopeDigest: scopeSet, EffectID: attempt.EffectID, ExpectedEffectRevision: 3, Owner: owner, DomainResult: domain,
		Evidence: evidence, IdempotencyKey: "settlement-idempotency-ports", ConflictDomain: core.DigestBytes([]byte("conflict")), SettledUnixNano: now.UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return value
}
