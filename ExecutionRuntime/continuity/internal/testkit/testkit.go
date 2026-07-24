package testkit

import (
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
)

type Clock struct{ Time time.Time }

func (c *Clock) Now() time.Time { return c.Time }

func (c *Clock) Advance(d time.Duration) { c.Time = c.Time.Add(d) }

func Scope() contract.Scope {
	return contract.Scope{
		TenantID: "tenant-1", IdentityID: "identity-1", IdentityEpoch: 2,
		LineageID: "lineage-1", PlanDigest: "plan-digest", InstanceID: "instance-1",
		InstanceEpoch: 3, SandboxLeaseID: "lease-1", SandboxLeaseEpoch: 4,
		RunID: "run-1", RunIdentityDigest: "run-digest", AuthorityEpoch: 5,
		ExecutionScopeDigest: "execution-scope-digest",
	}
}

func Owner() contract.OwnerBinding {
	return contract.OwnerBinding{
		BindingSetID: "binding-set-1", BindingRevision: 1, ComponentID: "component-1",
		ManifestDigest: "manifest-digest", ArtifactDigest: "artifact-digest",
		Capability: "timeline-producer", FactKind: "observation",
	}
}

func Candidate(sequence uint64, sourceSequence uint64, trust contract.TrustClass) contract.TimelineProjectionCandidate {
	observed := time.Date(2026, 7, 15, 12, 0, 0, int(sequence), time.UTC).UnixNano()
	c := contract.TimelineProjectionCandidate{
		ContractVersion: contract.ContractVersion, CandidateID: "candidate-" + u64(sequence), Revision: 1,
		Scope: Scope(), Owner: Owner(),
		Evidence: contract.EvidenceAdmission{
			RecordRef: "evidence-" + u64(sequence), LedgerScopeDigest: "ledger-scope-1",
			LedgerSequence: sequence, RecordDigest: "record-digest-" + u64(sequence),
			SourceKey:  contract.EvidenceSourceKey{RegistrationID: "source-1", SourceEpoch: 1, SourceSequence: sourceSequence},
			TrustClass: trust, ObservedUnixNano: observed, RecordedUnixNano: observed + 1,
			PayloadRef: "payload-" + u64(sequence), PayloadSchema: "schema/observation-v1",
			PayloadDigest: "payload-digest-" + u64(sequence), PayloadRevision: 1,
			AdmittedByLedger: true, InspectedByOwner: true,
		},
		SemanticKind: "praxis/observation", CorrelationID: "correlation-1",
		ObjectRefs: []string{"object-1"}, ProjectionPolicyRef: "projection-policy-1",
	}
	if trust == contract.TrustAuthoritativeFact {
		c.OwnerFactRef = FactRef("owner-fact-" + u64(sequence))
	}
	digest, err := c.CanonicalDigest()
	if err != nil {
		panic(err)
	}
	c.Digest = digest
	return c
}

func FactRef(id string) *contract.FactRef {
	return &contract.FactRef{
		ID: id, Revision: 1, Digest: id + "-digest", SchemaRef: "schema/fact-v1",
		Owner: Owner(), ScopeDigest: "execution-scope-digest",
		CreatedAt: time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC).UnixNano(),
		UpdatedAt: time.Date(2026, 7, 15, 12, 0, 1, 0, time.UTC).UnixNano(),
	}
}

func ArtifactRelationOwnerV1() contract.OwnerBinding {
	return contract.OwnerBinding{
		BindingSetID: "continuity-binding-1", BindingRevision: 1,
		ComponentID: contract.ContinuityComponentID, ManifestDigest: "continuity-manifest-digest",
		ArtifactDigest: "continuity-artifact-digest", Capability: contract.ArtifactRelationCapabilityV1,
		FactKind: "artifact_relation_fact_v1",
	}
}

func ArtifactOwnerV1() contract.OwnerBinding {
	return contract.OwnerBinding{
		BindingSetID: "artifact-binding-1", BindingRevision: 1,
		ComponentID: "praxis/artifact-owner", ManifestDigest: "artifact-owner-manifest",
		ArtifactDigest: "artifact-owner-build", Capability: "artifact-history-v1",
		FactKind: "artifact_fact_v1",
	}
}

func ExactRefV1(id, factKind string, revision uint64, owner contract.OwnerBinding) contract.ExactFactRefV2 {
	owner.FactKind = factKind
	return contract.ExactFactRefV2{
		ContractVersion: "praxis.test/" + factKind + "/v1", SchemaRef: "praxis.test/" + factKind + "/schema-v1",
		Owner: owner, TenantID: Scope().TenantID, ID: id, Revision: revision,
		Digest: id + "-digest-" + u64(revision), ScopeDigest: Scope().ExecutionScopeDigest,
	}
}

func ArtifactSourceProjectionV1(evidenceRef, evidenceDigest string) contract.ArtifactRelationSourceProjectionV1 {
	owner := ArtifactOwnerV1()
	artifact := ExactRefV1("artifact-1", "artifact_fact_v1", 2, owner)
	parent := ExactRefV1("artifact-1", "artifact_fact_v1", 1, owner)
	relatedOwner := Owner()
	related := ExactRefV1("context-frame-1", "context_frame_fact_v1", 1, relatedOwner)
	projectionOwner := owner
	projectionOwner.Capability = "artifact-relation-source-v1"
	projection := ExactRefV1("artifact-source-projection-1", "artifact_relation_source_projection_v1", 1, projectionOwner)
	return contract.ArtifactRelationSourceProjectionV1{
		SourceProjectionRef: projection,
		Artifact: contract.ArtifactRefV1{
			ArtifactFactRef: artifact, StorageRef: "object/artifact-1/revision/2",
			StorageDigest: "artifact-storage-digest-2", ParentRevisionRef: &parent,
			OriginEvidenceRecordRef: evidenceRef, OriginEvidenceDigest: evidenceDigest,
		},
		RelatedFactRef: related, Kind: contract.ArtifactRelationContextFrame,
		EvidenceRecordRef: evidenceRef, EvidenceRecordDigest: evidenceDigest,
		ExecutionScopeDigest: Scope().ExecutionScopeDigest,
	}
}

func ArtifactRelationRequestV1(source contract.ArtifactRelationSourceProjectionV1) ports.CreateArtifactRelationRequestV1 {
	return ports.CreateArtifactRelationRequestV1{
		RelationID: "artifact-relation-1", IdempotencyKey: "artifact-relation-request-1",
		Scope: Scope(), ArtifactFactRef: source.Artifact.ArtifactFactRef,
		RelatedFactRef: source.RelatedFactRef, Kind: source.Kind,
		EvidenceRecordRef: source.EvidenceRecordRef, ExpectedSourceProjectionRef: &source.SourceProjectionRef,
	}
}

func ContentIntegrityAuditOwnerV1() contract.OwnerBinding {
	return contract.OwnerBinding{
		BindingSetID: "continuity-binding-1", BindingRevision: 1,
		ComponentID: contract.ContinuityComponentID, ManifestDigest: "continuity-manifest-digest",
		ArtifactDigest: "continuity-artifact-digest", Capability: contract.ContentIntegrityAuditCapabilityV1,
		FactKind: "content_integrity_audit_fact_v1",
	}
}

func ContentIntegrityAuditRequestV1() ports.CreateContentIntegrityAuditRequestV1 {
	return ports.CreateContentIntegrityAuditRequestV1{
		AuditID: "content-audit-1", IdempotencyKey: "content-audit-request-1", Scope: Scope(),
		Subjects: []contract.ContentIntegritySubjectV1{{ObjectID: "object-1", JournalID: "journal-1"}},
	}
}

func ContentIntegrityAuditFactV1(classification contract.ContentIntegrityClassificationV1, detail string, now time.Time) contract.ContentIntegrityAuditFactV1 {
	request := ContentIntegrityAuditRequestV1()
	requestDigest, err := request.CanonicalDigest()
	if err != nil {
		panic(err)
	}
	finding := contract.ContentIntegrityFindingV1{
		Subject: request.Subjects[0], Classification: classification, DetailCode: detail,
	}
	fact, err := contract.NewContentIntegrityAuditFactV1(
		request.AuditID, request.IdempotencyKey, requestDigest, request.Scope,
		ContentIntegrityAuditOwnerV1(), request.Subjects, []contract.ContentIntegrityFindingV1{finding}, now,
	)
	if err != nil {
		panic(err)
	}
	return fact
}

func ContentDeltaOwnerV1() contract.OwnerBinding {
	return contract.OwnerBinding{
		BindingSetID: "continuity-binding-1", BindingRevision: 1,
		ComponentID: contract.ContinuityComponentID, ManifestDigest: "continuity-manifest-digest",
		ArtifactDigest: "continuity-artifact-digest", Capability: contract.ContentDeltaCapabilityV1,
		FactKind: "content_delta_fact_v1",
	}
}

func HistoryDerivationOwnerV1() contract.OwnerBinding {
	return contract.OwnerBinding{
		BindingSetID: "continuity-binding-1", BindingRevision: 1,
		ComponentID: contract.ContinuityComponentID, ManifestDigest: "continuity-manifest-digest",
		ArtifactDigest: "continuity-artifact-digest", Capability: contract.HistoryDerivationCapabilityV1,
		FactKind: "history_derivation_candidate_fact_v1",
	}
}

func TimelineEvent(sequence uint64, sourceSequence uint64, trust contract.TrustClass) contract.TimelineEventRecord {
	candidate := Candidate(sequence, sourceSequence, trust)
	return contract.TimelineEventRecord{
		Candidate: candidate, EvidenceRecordRef: candidate.Evidence.RecordRef,
		LedgerScopeDigest: candidate.Evidence.LedgerScopeDigest, LedgerSequence: candidate.Evidence.LedgerSequence,
		EvidenceRecordDigest: candidate.Evidence.RecordDigest, TrustClass: candidate.Evidence.TrustClass,
		ProjectionRevision: 1, Visibility: "visible",
	}
}

func ContentDeltaSourceV1(scope contract.Scope) contract.ContentDeltaSourceProjectionV1 {
	chunk := func(data string) contract.ChunkRef {
		return contract.ChunkRef{SchemaVersion: "content/v1", Digest: contract.DigestBytes([]byte(data)), Length: int64(len(data))}
	}
	return contract.ContentDeltaSourceProjectionV1{
		Base:         contract.ContentObjectRefV1{ObjectID: "object-base", SchemaVersion: "content/v1", ManifestDigest: "base-manifest-digest", ContentDigest: contract.DigestBytes([]byte("AAAABBBBCCCC")), TotalLength: 12, Compression: "identity", EncryptionRef: "key-envelope-1", Classification: "sensitive", OwnerID: "continuity", ScopeDigest: scope.ExecutionScopeDigest, RetentionPolicyRef: "retention-1"},
		BaseChunks:   []contract.ChunkRef{chunk("AAAA"), chunk("BBBB"), chunk("CCCC")},
		Target:       contract.ContentObjectRefV1{ObjectID: "object-target", SchemaVersion: "content/v1", ManifestDigest: "target-manifest-digest", ContentDigest: contract.DigestBytes([]byte("AAAABBBBDDDD")), TotalLength: 12, Compression: "identity", EncryptionRef: "key-envelope-1", Classification: "sensitive", OwnerID: "continuity", ScopeDigest: scope.ExecutionScopeDigest, RetentionPolicyRef: "retention-1"},
		TargetChunks: []contract.ChunkRef{chunk("AAAA"), chunk("BBBB"), chunk("DDDD")},
	}
}

func u64(value uint64) string {
	const digits = "0123456789"
	if value == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for value > 0 {
		i--
		buf[i] = digits[value%10]
		value /= 10
	}
	return string(buf[i:])
}
