package ports_test

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestRunIdentityDigestV2IsStableAcrossPendingRunningAndStartTime(t *testing.T) {
	scope := core.ExecutionScope{
		Identity:       core.AgentIdentityRef{TenantID: "tenant-run-identity", ID: "identity-run-identity", Epoch: 1},
		Lineage:        core.LineageRef{ID: "lineage-run-identity", PlanDigest: core.DigestBytes([]byte("lineage"))},
		Instance:       core.InstanceRef{ID: "instance-run-identity", Epoch: 1},
		AuthorityEpoch: 1,
	}
	session, err := ports.DeriveRuntimeExecutionSessionRefV2("endpoint-run-identity", "run-identity")
	if err != nil {
		t.Fatal(err)
	}
	pending := core.AgentRunRecord{ID: "run-identity", Scope: scope, Status: core.RunPending, Revision: 1, SessionRef: session}
	running := pending
	running.Status = core.RunRunning
	running.Revision = 2
	running.StartedAt = time.Unix(210_000, 0)
	pendingDigest, err := ports.RunIdentityDigestV2(pending)
	if err != nil {
		t.Fatal(err)
	}
	runningDigest, err := ports.RunIdentityDigestV2(running)
	if err != nil {
		t.Fatal(err)
	}
	if pendingDigest != runningDigest {
		t.Fatal("pending->running status, revision and StartedAt changed stable Run identity")
	}
	otherScope := pending
	otherScope.Scope.Identity.TenantID = "tenant-run-identity-other"
	otherScopeDigest, _ := ports.RunIdentityDigestV2(otherScope)
	if otherScopeDigest == pendingDigest {
		t.Fatal("same RunID in another tenant collided")
	}
	otherSession := pending
	otherSession.SessionRef = "runtime-session:" + string(core.DigestBytes([]byte("other-session")))
	otherSessionDigest, _ := ports.RunIdentityDigestV2(otherSession)
	if otherSessionDigest == pendingDigest {
		t.Fatal("session drift did not change stable Run identity")
	}
	otherEpoch := pending
	otherEpoch.Scope.Instance.Epoch++
	otherEpochDigest, _ := ports.RunIdentityDigestV2(otherEpoch)
	if otherEpochDigest == pendingDigest {
		t.Fatal("instance epoch drift did not change stable Run identity")
	}
}

func TestRunSettlementParticipantV2RequiresEvidenceExceptExplicitNotRequired(t *testing.T) {
	t.Parallel()
	now := time.Unix(200_000, 0)
	scope := core.ExecutionScope{
		Identity:       core.AgentIdentityRef{TenantID: "tenant-participant", ID: "identity-participant", Epoch: 1},
		Lineage:        core.LineageRef{ID: "lineage-participant", PlanDigest: core.DigestBytes([]byte("lineage-plan"))},
		Instance:       core.InstanceRef{ID: "instance-participant", Epoch: 1},
		AuthorityEpoch: 1,
	}
	scopeDigest, _ := ports.ExecutionScopeDigestV2(scope)
	policy := ports.RunSettlementPolicyBindingRefV2{
		Ref:            "policy-participant",
		Revision:       1,
		Digest:         core.DigestBytes([]byte("policy")),
		SemanticDigest: core.DigestBytes([]byte("policy-semantic")),
	}
	base := ports.RunSettlementParticipantFactV2{
		ContractVersion:      ports.RunSettlementContractVersionV2,
		ID:                   "participant-evidence",
		Revision:             1,
		RunID:                "run-participant",
		RunIdentityDigest:    core.DigestBytes([]byte("run-identity")),
		ExecutionScope:       scope,
		ExecutionScopeDigest: scopeDigest,
		Plan: ports.RunSettlementPlanRefV2{
			ID:       "plan-participant",
			Revision: 1,
			Digest:   core.DigestBytes([]byte("plan")),
		},
		RequirementID:     "custom/participant",
		RequirementDigest: core.DigestBytes([]byte("requirement")),
		SubjectDigest:     core.DigestBytes([]byte("subject")),
		Owner: ports.EvidenceProducerBindingRefV2{
			BindingSetID:       "binding-participant",
			BindingSetRevision: 1,
			ComponentID:        "custom/participant",
			ManifestDigest:     core.DigestBytes([]byte("manifest")),
			ArtifactDigest:     core.DigestBytes([]byte("artifact")),
			Capability:         "custom/settle",
		},
		CreatedUnixNano: now.UnixNano(),
		ExpiresUnixNano: now.Add(time.Minute).UnixNano(),
	}

	for _, disposition := range []ports.RunSettlementDispositionV2{
		ports.RunSettlementConfirmedSatisfied,
		ports.RunSettlementConfirmedFailed,
		ports.RunSettlementConfirmedNotApplied,
		ports.RunSettlementUnknown,
	} {
		for _, evidence := range [][]ports.EvidenceRecordRefV2{nil, []ports.EvidenceRecordRefV2{}} {
			fact := base
			fact.Disposition = disposition
			fact.Evidence = evidence
			if !core.HasReason(fact.Validate(), core.ReasonEvidenceConflict) {
				t.Fatalf("%s accepted empty Evidence: %#v", disposition, evidence)
			}
		}
	}

	notRequired := base
	notRequired.Disposition = ports.RunSettlementOperationNotRequired
	notRequired.Policy = &policy
	for _, evidence := range [][]ports.EvidenceRecordRefV2{nil, []ports.EvidenceRecordRefV2{}} {
		notRequired.Evidence = evidence
		if err := notRequired.Validate(); err != nil {
			t.Fatalf("explicit operation_not_required rejected canonical empty Evidence: %v", err)
		}
	}
}

func FuzzRunSettlementRequirementCanonicalV2(f *testing.F) {
	f.Add("custom/module", "custom/subject")
	f.Fuzz(func(t *testing.T, id, selector string) {
		requirement := ports.RunSettlementRequirementV2{
			ID:    ports.NamespacedNameV2(id),
			Kind:  "custom/domain-commit",
			Phase: ports.RunSettlementPhaseCompletion,
			Owner: ports.EvidenceProducerBindingRefV2{
				BindingSetID:       "binding-custom",
				BindingSetRevision: 1,
				ComponentID:        "custom/component",
				ManifestDigest:     core.DigestBytes([]byte("manifest")),
				ArtifactDigest:     core.DigestBytes([]byte("artifact")),
				Capability:         "custom/settle",
			},
			Schema: ports.SchemaRefV2{
				Namespace:     "custom",
				Name:          "settlement",
				Version:       "1.0.0",
				MediaType:     "application/octet-stream",
				ContentDigest: core.DigestBytes([]byte("schema")),
			},
			SubjectSelector: ports.NamespacedNameV2(selector),
			SubjectDigest:   core.DigestBytes([]byte("subject")),
			EvidenceTrust:   ports.EvidenceTrustAttestation,
			EvidenceKind:    "custom/settlement-attestation",
			Policy: ports.RunSettlementPolicyBindingRefV2{
				Ref:            "policy-custom",
				Revision:       1,
				Digest:         core.DigestBytes([]byte("policy")),
				SemanticDigest: core.DigestBytes([]byte("policy-semantic")),
			},
		}
		first, firstErr := requirement.DigestV2()
		second, secondErr := requirement.DigestV2()
		if (firstErr == nil) != (secondErr == nil) {
			t.Fatal("canonical validator is nondeterministic")
		}
		if firstErr == nil && first != second {
			t.Fatalf("canonical digest drifted: %s != %s", first, second)
		}
	})
}
