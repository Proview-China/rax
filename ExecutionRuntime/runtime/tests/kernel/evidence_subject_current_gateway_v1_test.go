package kernel_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimekernel "github.com/Proview-China/rax/ExecutionRuntime/runtime/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestEvidenceSubjectCurrentGatewayV1RejectsCandidateRegistrationAndPolicyTypePun(t *testing.T) {
	mutations := map[string]func(*ports.EvidenceSubjectCurrentProjectionV1){
		"trust": func(p *ports.EvidenceSubjectCurrentProjectionV1) { p.TrustClass = ports.EvidenceTrustReceipt },
		"class": func(p *ports.EvidenceSubjectCurrentProjectionV1) { p.CustomClass = "review/other-class" },
		"owner_fact": func(p *ports.EvidenceSubjectCurrentProjectionV1) {
			p.OwnerFact = &ports.EvidenceOwnerFactRefV2{}
		},
		"payload": func(p *ports.EvidenceSubjectCurrentProjectionV1) {
			p.Payload.ContentDigest = evidenceSubjectKernelDigestV1("other-payload")
		},
		"scope": func(p *ports.EvidenceSubjectCurrentProjectionV1) {
			p.ExecutionScope.Instance.Epoch++
			p.ExecutionScopeDigest, _ = ports.ExecutionScopeDigestV2(p.ExecutionScope)
		},
		"registration_id": func(p *ports.EvidenceSubjectCurrentProjectionV1) {
			p.Registration.RegistrationID = "other-registration"
		},
		"source_id":       func(p *ports.EvidenceSubjectCurrentProjectionV1) { p.Registration.SourceID = "review/other-source" },
		"source_epoch":    func(p *ports.EvidenceSubjectCurrentProjectionV1) { p.Registration.SourceEpoch++ },
		"source_sequence": func(p *ports.EvidenceSubjectCurrentProjectionV1) { p.Source.SourceSequence++ },
		"registration_state": func(p *ports.EvidenceSubjectCurrentProjectionV1) {
			p.RegistrationState = ports.EvidenceSourceRevoked
		},
		"registration_expiry": func(p *ports.EvidenceSubjectCurrentProjectionV1) { p.RegistrationExpiresUnixNano-- },
		"policy_state": func(p *ports.EvidenceSubjectCurrentProjectionV1) {
			p.SourcePolicyState = ports.EvidenceSourcePolicyRevoked
		},
		"policy_owner": func(p *ports.EvidenceSubjectCurrentProjectionV1) {
			p.SourcePolicyOwner.ComponentID = "review/other-policy-owner"
		},
		"policy_expiry": func(p *ports.EvidenceSubjectCurrentProjectionV1) { p.SourcePolicyExpiresUnixNano-- },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			fixture := newEvidenceSubjectKernelFixtureV1(t)
			originalCandidateDigest := fixture.projection.CandidateDigest
			projection := fixture.projection
			mutate(&projection)
			projection.Ref.Digest, projection.ProjectionDigest = "", ""
			sealed, err := ports.SealEvidenceSubjectCurrentProjectionV1(projection)
			if err != nil {
				// Source-coordinate self contradictions may be rejected by the
				// projection itself, before reaching the Gateway.
				if name != "source_sequence" || !core.HasCategory(err, core.ErrorConflict) {
					t.Fatalf("unexpected pre-Gateway rejection: %v", err)
				}
				return
			}
			fixture.facts.projection = sealed
			fixture.facts.index, err = ports.SealEvidenceSubjectCurrentIndexRefV1(ports.EvidenceSubjectCurrentIndexRefV1{
				Revision: sealed.Ref.Revision, SubjectKeyDigest: sealed.SubjectKeyDigest, CurrentProjection: sealed.Ref, OwnerWatermark: sealed.Ref.OwnerWatermark,
			})
			if err != nil {
				t.Fatal(err)
			}
			if sealed.CandidateDigest != originalCandidateDigest {
				t.Fatal("test mutation changed the genuine CandidateDigest")
			}
			got, err := fixture.gateway.InspectEvidenceSubjectCurrentV1(context.Background(), fixture.lookup)
			if err == nil {
				t.Fatalf("same CandidateDigest type-pun returned current Projection: %+v", got)
			}
			if !reflect.DeepEqual(got, ports.EvidenceSubjectCurrentSnapshotV1{}) {
				t.Fatalf("failed current validation leaked a Projection: %+v", got)
			}
		})
	}
}

type evidenceSubjectKernelFixtureV1 struct {
	gateway    runtimekernel.EvidenceSubjectCurrentGatewayV1
	lookup     ports.EvidenceSubjectCurrentLookupRequestV1
	projection ports.EvidenceSubjectCurrentProjectionV1
	facts      *evidenceSubjectKernelFactsV1
}

func newEvidenceSubjectKernelFixtureV1(t *testing.T) evidenceSubjectKernelFixtureV1 {
	t.Helper()
	now := time.Unix(2_100_100_000, 0)
	scope := core.ExecutionScope{
		Identity: core.AgentIdentityRef{TenantID: "tenant-review", ID: "identity-review", Epoch: 2},
		Lineage:  core.LineageRef{ID: "lineage-review", PlanDigest: evidenceSubjectKernelDigestV1("plan")},
		Instance: core.InstanceRef{ID: "instance-review", Epoch: 3}, AuthorityEpoch: 4,
	}
	ledgerScope := ports.EvidenceLedgerScopeV2{Partition: ports.EvidencePartitionRun, TenantID: "tenant-review", IdentityID: "identity-review", LineageID: "lineage-review", InstanceID: "instance-review", RunID: "run-review"}
	producer := ports.EvidenceProducerBindingRefV2{BindingSetID: "evidence-set", BindingSetRevision: 1, ComponentID: "review/evidence-producer", ManifestDigest: evidenceSubjectKernelDigestV1("manifest"), ArtifactDigest: evidenceSubjectKernelDigestV1("artifact"), Capability: "review/produce-evidence"}
	authority := ports.AuthorityBindingRefV2{Ref: "review-authority", Digest: evidenceSubjectKernelDigestV1("authority"), Revision: 1, Epoch: 4}
	policyRef := ports.EvidenceSourcePolicyBindingRefV2{Ref: "review-policy", Revision: 1, Digest: evidenceSubjectKernelDigestV1("placeholder-policy")}
	currentScope := ports.ExecutionScopeBindingRefV2{Ref: "review-scope", Revision: 1, Digest: evidenceSubjectKernelDigestV1("current-scope")}
	registration := ports.EvidenceSourceRegistrationFactV2{
		ContractVersion: ports.EvidenceContractVersionV2, ID: "review-registration", Revision: 1, SourceID: "review/source", SourceEpoch: 2,
		LedgerScope: ledgerScope, ExecutionScope: scope, CurrentScope: currentScope, CurrentScopeWatermark: 1,
		Producer: producer, Authority: authority, ActionScopeDigest: evidenceSubjectKernelDigestV1("action-scope"), Policy: policyRef,
		ClassMappings: []ports.EvidenceClassMappingV2{{Class: "review/observation", Trust: ports.EvidenceTrustObservation}}, AllowedKinds: []ports.NamespacedNameV2{"review/event"}, GapPolicy: ports.EvidenceGapStrictV2,
		NextSourceSequence: 1, State: ports.EvidenceSourceActive, CreatedUnixNano: now.Add(-2 * time.Second).UnixNano(), UpdatedUnixNano: now.Add(-2 * time.Second).UnixNano(), ExpiresUnixNano: now.Add(30 * time.Second).UnixNano(),
	}
	configurationDigest, err := registration.ConfigurationDigestV2()
	if err != nil {
		t.Fatal(err)
	}
	candidate := ports.EvidenceEventCandidateV2{
		ContractVersion: ports.EvidenceContractVersionV2, LedgerScope: ledgerScope, EventID: "review-event", RegistrationID: registration.ID, RegistrationRevision: registration.Revision,
		SourceConfigurationDigest: configurationDigest, SourcePolicy: policyRef, SourceID: registration.SourceID, SourceEpoch: registration.SourceEpoch, SourceSequence: 1,
		TrustClass: ports.EvidenceTrustObservation, EventKind: "review/event", CustomClass: "review/observation", ExecutionScope: scope,
		Payload:   ports.EvidencePayloadRefV2{Schema: ports.SchemaRefV2{Namespace: "review", Name: "evidence", Version: "1.0.0", MediaType: "application/json", ContentDigest: evidenceSubjectKernelDigestV1("schema")}, ContentDigest: evidenceSubjectKernelDigestV1("payload"), Revision: 1, Length: 1, Ref: "ledger://review-event"},
		Causation: []ports.EvidenceCausationRefV2{}, CorrelationID: "review-correlation", Producer: producer, Authority: authority, ObservedUnixNano: now.Add(-2 * time.Second).UnixNano(),
	}
	record, err := control.NewEvidenceLedgerRecordV2(candidate, 1, ports.EvidenceGenesisDigestV2, now.Add(-time.Second))
	if err != nil {
		t.Fatal(err)
	}
	subject := ports.EvidenceSubjectKeyV1{Record: record.Ref, Source: ports.EvidenceSourceKeyV2{RegistrationID: registration.ID, SourceEpoch: registration.SourceEpoch, SourceSequence: candidate.SourceSequence}}
	subjectDigest, _ := ports.DigestEvidenceSubjectKeyV1(subject)
	scopeDigest, _ := ports.ExecutionScopeDigestV2(scope)
	ledgerDigest, _ := ledgerScope.DigestV2()
	registrationRef, err := control.NewEvidenceSourceRegistrationRefV1(registration)
	if err != nil {
		t.Fatal(err)
	}
	recordResult, err := ports.SealEvidenceSubjectRecordRegistrationCurrentResultV1(ports.EvidenceSubjectRecordRegistrationCurrentResultV1{
		Subject: subject, Record: record, Registration: registration, CheckedUnixNano: registration.UpdatedUnixNano, ExpiresUnixNano: registration.ExpiresUnixNano,
	})
	if err != nil {
		t.Fatal(err)
	}
	consumer := ports.ProviderBindingRefV2{BindingSetID: "review-reader-set", BindingSetRevision: 1, ComponentID: "review/current-reader", ManifestDigest: evidenceSubjectKernelDigestV1("reader-manifest"), ArtifactDigest: evidenceSubjectKernelDigestV1("reader-artifact"), Capability: ports.EvidenceSubjectReaderCapabilityV1}
	consumerCurrent, err := ports.SealProviderBindingCurrentProjectionV2(ports.ProviderBindingCurrentProjectionV2{ContractVersion: ports.ProviderBindingCurrentnessContractVersionV2, Ref: consumer, State: ports.ProviderBindingCurrentActiveV2, BindingSetDigest: evidenceSubjectKernelDigestV1("reader-set"), BindingSetSemanticDigest: evidenceSubjectKernelDigestV1("reader-semantic"), BindingID: "review-reader", BindingRevision: 1, GrantDigest: evidenceSubjectKernelDigestV1("reader-grant"), IssuedUnixNano: now.Add(-2 * time.Second).UnixNano(), ExpiresUnixNano: now.Add(time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	principal := core.OwnerRef{Domain: "runtime-host", ID: "review-reader"}
	associationRef, err := ports.SealEvidenceSubjectConsumerAssociationRefV1(ports.EvidenceSubjectConsumerAssociationRefV1{Revision: 1, Principal: principal, Consumer: consumer, ExecutionScopeDigest: scopeDigest})
	if err != nil {
		t.Fatal(err)
	}
	association, err := ports.SealEvidenceSubjectConsumerAssociationCurrentProjectionV1(ports.EvidenceSubjectConsumerAssociationCurrentProjectionV1{Ref: associationRef, Principal: principal, Consumer: consumer, ExecutionScopeDigest: scopeDigest, BindingCurrent: consumerCurrent, CheckedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(20 * time.Second).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	absence, err := ports.SealEvidenceTombstoneAbsenceRefV1(ports.EvidenceTombstoneAbsenceRefV1{SubjectKeyDigest: subjectDigest, Revision: 1, OwnerWatermark: 1})
	if err != nil {
		t.Fatal(err)
	}
	projection, err := ports.SealEvidenceSubjectCurrentProjectionV1(ports.EvidenceSubjectCurrentProjectionV1{
		Ref: ports.EvidenceSubjectProjectionRefV1{Revision: 1, OwnerWatermark: 1}, Subject: subject, Record: record.Ref, Source: subject.Source,
		CandidateDigest: record.CandidateDigest, PreviousRecordDigest: record.PreviousRecordDigest, Registration: registrationRef, RegistrationState: registration.State, RegistrationExpiresUnixNano: registration.ExpiresUnixNano,
		SourcePolicy: policyRef, SourcePolicyState: ports.EvidenceSourcePolicyActive, SourcePolicyOwner: producer, SourcePolicyAuthority: authority, SourcePolicyExpiresUnixNano: now.Add(40 * time.Second).UnixNano(),
		LedgerScope: ledgerScope, LedgerScopeDigest: ledgerDigest, ExecutionScope: scope, ExecutionScopeDigest: scopeDigest, CurrentScope: currentScope, CurrentScopeWatermark: 1,
		Producer: producer, Authority: authority, ActionScopeDigest: registration.ActionScopeDigest, Consumer: consumer,
		TrustClass: candidate.TrustClass, ClaimKind: candidate.ClaimKind, EventKind: candidate.EventKind, CustomClass: candidate.CustomClass, Payload: candidate.Payload, Causation: candidate.Causation, CorrelationID: candidate.CorrelationID, OwnerFact: candidate.OwnerFact, HistoricalSource: candidate.HistoricalSource, ObservedUnixNano: candidate.ObservedUnixNano, IngestedUnixNano: record.IngestedUnixNano,
		Presence: ports.EvidenceTombstoneAbsentSealedV1, Readability: ports.EvidenceSubjectReadableV1, TombstoneAbsence: &absence, CheckedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(20 * time.Second).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	index, err := ports.SealEvidenceSubjectCurrentIndexRefV1(ports.EvidenceSubjectCurrentIndexRefV1{Revision: 1, SubjectKeyDigest: subjectDigest, CurrentProjection: projection.Ref, OwnerWatermark: 1})
	if err != nil {
		t.Fatal(err)
	}
	policy := ports.EvidenceSourcePolicyFactV2{
		Ref: policyRef.Ref, Revision: policyRef.Revision, Producer: producer, PolicyOwner: producer, PolicyAuthority: authority, PolicyScope: scope, ActionScopeDigest: registration.ActionScopeDigest,
		AllowedPartitions: []ports.EvidencePartitionV2{ports.EvidencePartitionRun}, ClassMappings: registration.ClassMappings, AllowedKinds: registration.AllowedKinds, OwnerFactRules: []ports.EvidenceOwnerFactRuleV2{}, ClaimKinds: []ports.EvidenceClaimKindMappingV2{},
		MaximumSourceTTL: time.Minute, State: ports.EvidenceSourcePolicyActive, ExpiresUnixNano: projection.SourcePolicyExpiresUnixNano,
	}
	policy.Digest, err = policy.DigestV2()
	if err != nil {
		t.Fatal(err)
	}
	// Make the exact policy Ref bind its canonical Owner digest.
	registration.Policy.Digest, candidate.SourcePolicy.Digest, projection.SourcePolicy.Digest = policy.Digest, policy.Digest, policy.Digest
	registrationRef, _ = control.NewEvidenceSourceRegistrationRefV1(registration)
	projection.Registration = registrationRef
	projection.Ref.Digest, projection.ProjectionDigest = "", ""
	projection, err = ports.SealEvidenceSubjectCurrentProjectionV1(projection)
	if err != nil {
		t.Fatal(err)
	}
	index, _ = ports.SealEvidenceSubjectCurrentIndexRefV1(ports.EvidenceSubjectCurrentIndexRefV1{Revision: 1, SubjectKeyDigest: subjectDigest, CurrentProjection: projection.Ref, OwnerWatermark: 1})
	recordResult.Registration = registration
	recordResult.ProjectionDigest = ""
	recordResult, _ = ports.SealEvidenceSubjectRecordRegistrationCurrentResultV1(recordResult)
	lookup := ports.EvidenceSubjectCurrentLookupRequestV1{ContractVersion: ports.EvidenceSubjectCurrentContractVersionV1, Subject: subject, ExpectedConsumer: consumer, ExpectedExecutionScopeDigest: scopeDigest, ExpectedSourcePolicy: registration.Policy}
	facts := &evidenceSubjectKernelFactsV1{projection: projection, index: index}
	gateway := runtimekernel.EvidenceSubjectCurrentGatewayV1{
		Facts: facts, Records: evidenceSubjectKernelRecordsV1{result: recordResult}, Policies: &evidenceSubjectKernelPolicyV1{fact: policy},
		CurrentScopes: evidenceSubjectKernelScopeV1{}, Bindings: evidenceSubjectKernelBindingsV1{}, Authority: evidenceSubjectKernelAuthorityV1{}, Presence: evidenceSubjectKernelPresenceV1{},
		ConsumerAssociations: evidenceSubjectKernelAssociationV1{projection: association}, ConsumerAssociation: associationRef, Clock: func() time.Time { return now },
	}
	return evidenceSubjectKernelFixtureV1{gateway: gateway, lookup: lookup, projection: projection, facts: facts}
}

type evidenceSubjectKernelFactsV1 struct {
	ports.EvidenceSubjectCurrentFactPortV1
	projection ports.EvidenceSubjectCurrentProjectionV1
	index      ports.EvidenceSubjectCurrentIndexRefV1
}

func (s *evidenceSubjectKernelFactsV1) InspectEvidenceSubjectProjectionFactV1(context.Context, ports.EvidenceSubjectProjectionRefV1) (ports.EvidenceSubjectCurrentProjectionV1, error) {
	return s.projection, nil
}
func (s *evidenceSubjectKernelFactsV1) InspectEvidenceSubjectCurrentIndexV1(context.Context, core.Digest) (ports.EvidenceSubjectCurrentIndexRefV1, error) {
	return s.index, nil
}

type evidenceSubjectKernelRecordsV1 struct {
	result ports.EvidenceSubjectRecordRegistrationCurrentResultV1
}

func (s evidenceSubjectKernelRecordsV1) InspectEvidenceSubjectRecordRegistrationCurrentV1(context.Context, ports.EvidenceSubjectRecordRegistrationCurrentRequestV1) (ports.EvidenceSubjectRecordRegistrationCurrentResultV1, error) {
	return s.result, nil
}

type evidenceSubjectKernelPolicyV1 struct {
	fact  ports.EvidenceSourcePolicyFactV2
	calls int
}

func (s *evidenceSubjectKernelPolicyV1) InspectEvidenceSourcePolicy(context.Context, string) (ports.EvidenceSourcePolicyFactV2, error) {
	s.calls++
	return s.fact, nil
}

type evidenceSubjectKernelScopeV1 struct {
	ports.ExecutionScopeFactReaderV2
}
type evidenceSubjectKernelBindingsV1 struct {
	ports.ProviderBindingCurrentnessPortV2
}
type evidenceSubjectKernelAuthorityV1 struct{ ports.AuthorityFactReaderV2 }
type evidenceSubjectKernelPresenceV1 struct {
	ports.EvidenceSubjectPresenceReadabilityCurrentReaderV1
}

type evidenceSubjectKernelAssociationV1 struct {
	projection ports.EvidenceSubjectConsumerAssociationCurrentProjectionV1
}

func (s evidenceSubjectKernelAssociationV1) InspectEvidenceSubjectConsumerAssociationCurrentV1(context.Context, ports.EvidenceSubjectConsumerAssociationRefV1) (ports.EvidenceSubjectConsumerAssociationCurrentProjectionV1, error) {
	return s.projection, nil
}

func evidenceSubjectKernelDigestV1(value string) core.Digest {
	return core.DigestBytes([]byte("evidence-subject-kernel-v1:" + value))
}
