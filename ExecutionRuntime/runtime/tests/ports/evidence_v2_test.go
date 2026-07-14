package ports_test

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestEvidenceSourcePolicyV2RejectsInconsistentTrustMappings(t *testing.T) {
	base := evidencePolicyV2(t)
	if _, err := base.DigestV2(); err != nil {
		t.Fatalf("valid claim policy must digest: %v", err)
	}
	tests := []struct {
		name   string
		mutate func(*ports.EvidenceSourcePolicyFactV2)
	}{
		{"claim kind absent from allowed kinds", func(p *ports.EvidenceSourcePolicyFactV2) { p.ClaimKinds[0].EventKind = "custom/missing" }},
		{"claim class not mapped to claim", func(p *ports.EvidenceSourcePolicyFactV2) { p.ClaimKinds[0].CustomClass = "custom/observation" }},
		{"duplicate claim mapping", func(p *ports.EvidenceSourcePolicyFactV2) { p.ClaimKinds = append(p.ClaimKinds, p.ClaimKinds[0]) }},
		{"owner kind absent from allowed kinds", func(p *ports.EvidenceSourcePolicyFactV2) {
			p.OwnerFactRules = []ports.EvidenceOwnerFactRuleV2{{EventKind: "custom/missing", CustomClass: "custom/authoritative", FactKind: "custom/fact", OwnerComponent: "custom/owner"}}
		}},
		{"owner class not authoritative", func(p *ports.EvidenceSourcePolicyFactV2) {
			p.OwnerFactRules = []ports.EvidenceOwnerFactRuleV2{{EventKind: "custom/claim-event", CustomClass: "custom/claim", FactKind: "custom/fact", OwnerComponent: "custom/owner"}}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			policy := base
			policy.ClassMappings = append([]ports.EvidenceClassMappingV2(nil), base.ClassMappings...)
			policy.AllowedKinds = append([]ports.NamespacedNameV2(nil), base.AllowedKinds...)
			policy.ClaimKinds = append([]ports.EvidenceClaimKindMappingV2(nil), base.ClaimKinds...)
			policy.OwnerFactRules = append([]ports.EvidenceOwnerFactRuleV2(nil), base.OwnerFactRules...)
			test.mutate(&policy)
			if _, err := policy.DigestV2(); !core.HasReason(err, core.ReasonEvidenceTrustInvalid) && !core.HasReason(err, core.ReasonInvalidCanonicalForm) {
				t.Fatalf("inconsistent policy must fail before canonical digest: %v", err)
			}
		})
	}
}

func TestEvidenceCandidateV2CanonicalNilEmptyAndFullContentBinding(t *testing.T) {
	base := evidenceCandidatePortV2(t)
	nilCandidate := base
	nilCandidate.Causation = nil
	emptyCandidate := base
	emptyCandidate.Causation = []ports.EvidenceCausationRefV2{}
	nilDigest, err := nilCandidate.DigestV2()
	if err != nil {
		t.Fatal(err)
	}
	emptyDigest, err := emptyCandidate.DigestV2()
	if err != nil {
		t.Fatal(err)
	}
	if nilDigest != emptyDigest {
		t.Fatalf("contract-declared nil and empty causation must be canonical equivalents: %s != %s", nilDigest, emptyDigest)
	}
	mutations := []func(*ports.EvidenceEventCandidateV2){
		func(c *ports.EvidenceEventCandidateV2) { c.EventID = "event-other" },
		func(c *ports.EvidenceEventCandidateV2) { c.TrustClass = ports.EvidenceTrustReceipt },
		func(c *ports.EvidenceEventCandidateV2) { c.CorrelationID = "correlation-other" },
		func(c *ports.EvidenceEventCandidateV2) {
			c.Payload.ContentDigest = portEffectDigestV2(t, "payload-other")
		},
		func(c *ports.EvidenceEventCandidateV2) { c.SourceSequence++ },
		func(c *ports.EvidenceEventCandidateV2) { c.ExecutionScope.AuthorityEpoch++ },
	}
	for index, mutate := range mutations {
		changed := emptyCandidate
		mutate(&changed)
		digest, digestErr := changed.DigestV2()
		if digestErr != nil {
			t.Fatalf("mutation %d must remain structurally valid: %v", index, digestErr)
		}
		if digest == emptyDigest {
			t.Fatalf("mutation %d must change full source content digest", index)
		}
	}
}

func FuzzEvidenceCandidateCanonicalV2(f *testing.F) {
	f.Add([]byte("alpha"))
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, payload []byte) {
		base := evidenceCandidatePortV2(t)
		digest, err := core.DigestJSON(payload)
		if err != nil {
			t.Skip()
		}
		base.Payload.ContentDigest = digest
		base.Causation = nil
		left, err := base.DigestV2()
		if err != nil {
			t.Fatal(err)
		}
		base.Causation = []ports.EvidenceCausationRefV2{}
		right, err := base.DigestV2()
		if err != nil {
			t.Fatal(err)
		}
		if left != right {
			t.Fatal("nil/empty canonical representation drifted")
		}
		base.TrustClass = ports.EvidenceTrustReceipt
		changed, err := base.DigestV2()
		if err != nil {
			t.Fatal(err)
		}
		if changed == right {
			t.Fatal("closed trust class is missing from source content digest")
		}
	})
}

func evidencePolicyV2(t *testing.T) ports.EvidenceSourcePolicyFactV2 {
	t.Helper()
	now := time.Unix(121_000, 0)
	scope := evidenceScopePortV2(t)
	producer := ports.EvidenceProducerBindingRefV2{BindingSetID: "binding-policy", BindingSetRevision: 1, ComponentID: "custom/producer", ManifestDigest: portEffectDigestV2(t, "policy-manifest"), ArtifactDigest: portEffectDigestV2(t, "policy-artifact"), Capability: "runtime/evidence-append"}
	return ports.EvidenceSourcePolicyFactV2{Ref: "policy-evidence", Revision: 1, Producer: producer, PolicyOwner: producer, PolicyAuthority: ports.AuthorityBindingRefV2{Ref: "authority-policy", Digest: portEffectDigestV2(t, "policy-authority"), Revision: 1, Epoch: 1}, PolicyScope: scope, ActionScopeDigest: portEffectDigestV2(t, "policy-action"), AllowedPartitions: []ports.EvidencePartitionV2{ports.EvidencePartitionRun}, ClassMappings: []ports.EvidenceClassMappingV2{{Class: "custom/authoritative", Trust: ports.EvidenceTrustAuthoritativeFact}, {Class: "custom/claim", Trust: ports.EvidenceTrustClaim}, {Class: "custom/observation", Trust: ports.EvidenceTrustObservation}}, AllowedKinds: []ports.NamespacedNameV2{"custom/claim-event", "custom/fact-event"}, OwnerFactRules: []ports.EvidenceOwnerFactRuleV2{{EventKind: "custom/fact-event", CustomClass: "custom/authoritative", FactKind: "custom/fact", OwnerComponent: "custom/owner"}}, ClaimKinds: []ports.EvidenceClaimKindMappingV2{{EventKind: "custom/claim-event", CustomClass: "custom/claim", ClaimKind: core.RunClaimCompleted}}, MaximumSourceTTL: time.Minute, State: ports.EvidenceSourcePolicyActive, ExpiresUnixNano: now.Add(time.Hour).UnixNano()}
}

func evidenceCandidatePortV2(t *testing.T) ports.EvidenceEventCandidateV2 {
	t.Helper()
	now := time.Unix(122_000, 0)
	scope := evidenceScopePortV2(t)
	producer := ports.EvidenceProducerBindingRefV2{BindingSetID: "binding-candidate", BindingSetRevision: 1, ComponentID: "custom/source", ManifestDigest: portEffectDigestV2(t, "candidate-manifest"), ArtifactDigest: portEffectDigestV2(t, "candidate-artifact"), Capability: "runtime/evidence-append"}
	return ports.EvidenceEventCandidateV2{ContractVersion: ports.EvidenceContractVersionV2, LedgerScope: ports.EvidenceLedgerScopeV2{Partition: ports.EvidencePartitionRun, TenantID: scope.Identity.TenantID, IdentityID: scope.Identity.ID, LineageID: scope.Lineage.ID, InstanceID: scope.Instance.ID, RunID: "run-candidate"}, EventID: "event-candidate", RegistrationID: "registration-candidate", RegistrationRevision: 1, SourceConfigurationDigest: portEffectDigestV2(t, "candidate-config"), SourcePolicy: ports.EvidenceSourcePolicyBindingRefV2{Ref: "policy-candidate", Digest: portEffectDigestV2(t, "candidate-policy"), Revision: 1}, SourceID: "custom/source", SourceEpoch: 1, SourceSequence: 1, TrustClass: ports.EvidenceTrustObservation, EventKind: "custom/event", CustomClass: "custom/observation", ExecutionScope: scope, Payload: ports.EvidencePayloadRefV2{Schema: ports.SchemaRefV2{Namespace: "custom", Name: "event", Version: "1.0.0", MediaType: "application/octet-stream", ContentDigest: portEffectDigestV2(t, "candidate-schema")}, ContentDigest: portEffectDigestV2(t, "candidate-payload"), Revision: 1, Length: 1, Ref: "memory://candidate"}, Causation: []ports.EvidenceCausationRefV2{}, CorrelationID: "correlation-candidate", Producer: producer, Authority: ports.AuthorityBindingRefV2{Ref: "authority-candidate", Digest: portEffectDigestV2(t, "candidate-authority"), Revision: 1, Epoch: 1}, ObservedUnixNano: now.UnixNano()}
}

func evidenceScopePortV2(t *testing.T) core.ExecutionScope {
	t.Helper()
	return core.ExecutionScope{Identity: core.AgentIdentityRef{TenantID: "tenant-evidence-port", ID: "identity-evidence-port", Epoch: 1}, Lineage: core.LineageRef{ID: "lineage-evidence-port", PlanDigest: portEffectDigestV2(t, "evidence-port-plan")}, Instance: core.InstanceRef{ID: "instance-evidence-port", Epoch: 1}, AuthorityEpoch: 1}
}
