package ports_test

import (
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"testing"
	"time"
)

func TestCustomEvidenceSourceConformanceCannotSelfGrantTrustClaimAppendOrCommit(t *testing.T) {
	now := time.Unix(120_000, 0)
	scope := core.ExecutionScope{Identity: core.AgentIdentityRef{TenantID: "tenant-conformance", ID: "identity-conformance", Epoch: 1}, Lineage: core.LineageRef{ID: "lineage-conformance", PlanDigest: portEffectDigestV2(t, "evidence-conformance-plan")}, Instance: core.InstanceRef{ID: "instance-conformance", Epoch: 1}, AuthorityEpoch: 1}
	producer := ports.EvidenceProducerBindingRefV2{BindingSetID: "binding-conformance", BindingSetRevision: 1, ComponentID: "custom/source", ManifestDigest: portEffectDigestV2(t, "manifest"), ArtifactDigest: portEffectDigestV2(t, "artifact"), Capability: "custom/evidence"}
	source := ports.EvidenceSourceRegistrationFactV2{ContractVersion: ports.EvidenceContractVersionV2, ID: "registration-conformance", Revision: 1, SourceID: "custom/source", SourceEpoch: 1, LedgerScope: ports.EvidenceLedgerScopeV2{Partition: ports.EvidencePartitionRun, TenantID: scope.Identity.TenantID, IdentityID: scope.Identity.ID, LineageID: scope.Lineage.ID, InstanceID: scope.Instance.ID, RunID: "run-conformance"}, ExecutionScope: scope, CurrentScope: ports.ExecutionScopeBindingRefV2{Ref: "scope-conformance", Digest: portEffectDigestV2(t, "scope"), Revision: 1}, CurrentScopeWatermark: 1, Producer: producer, Authority: ports.AuthorityBindingRefV2{Ref: "authority-conformance", Digest: portEffectDigestV2(t, "authority"), Revision: 1, Epoch: 1}, ActionScopeDigest: portEffectDigestV2(t, "action"), Policy: ports.EvidenceSourcePolicyBindingRefV2{Ref: "policy-conformance", Digest: portEffectDigestV2(t, "policy"), Revision: 1}, ClassMappings: []ports.EvidenceClassMappingV2{{Class: "custom/claim", Trust: ports.EvidenceTrustClaim}}, AllowedKinds: []ports.NamespacedNameV2{"custom/completion"}, GapPolicy: ports.EvidenceGapStrictV2, NextSourceSequence: 1, State: ports.EvidenceSourceActive, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Minute).UnixNano()}
	configuration, _ := source.ConfigurationDigestV2()
	candidate := ports.EvidenceEventCandidateV2{ContractVersion: ports.EvidenceContractVersionV2, LedgerScope: source.LedgerScope, EventID: "event-conformance", RegistrationID: source.ID, RegistrationRevision: 1, SourceConfigurationDigest: configuration, SourcePolicy: source.Policy, SourceID: source.SourceID, SourceEpoch: 1, SourceSequence: 1, TrustClass: ports.EvidenceTrustClaim, ClaimKind: core.RunClaimCompleted, EventKind: "custom/completion", CustomClass: "custom/claim", ExecutionScope: scope, Payload: ports.EvidencePayloadRefV2{Schema: ports.SchemaRefV2{Namespace: "custom", Name: "claim", Version: "1.0.0", MediaType: "application/octet-stream", ContentDigest: portEffectDigestV2(t, "schema")}, ContentDigest: portEffectDigestV2(t, "payload"), Revision: 1, Length: 1, Ref: "memory://claim"}, Causation: []ports.EvidenceCausationRefV2{}, CorrelationID: "run-conformance", Producer: producer, Authority: source.Authority, ObservedUnixNano: now.UnixNano()}
	report, err := conformance.CheckEvidenceSourceAdapterV2(conformance.EvidenceSourceAdapterCaseV2{Source: source, Candidate: candidate, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if !report.EnvelopeValid || !report.CertificationCandidate || report.BindingEligible || report.TrustGranted || report.ClaimEligible || report.AuthoritativeFactEligible || report.AppendEligible || report.DomainCommitEligible {
		t.Fatalf("custom source envelope must remain non-authoritative: %+v", report)
	}
}
