package control_test

import (
	"context"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestEvidenceSubjectCurrentGatewayV1PublicConformanceAndCurrentDrift(t *testing.T) {
	fixture := newEvidenceSubjectGatewayFixtureV1(t)
	report, err := conformance.VerifyEvidenceSubjectCurrentReaderV1(context.Background(), fixture.gateway, fixture.lookup)
	if err != nil {
		t.Fatal(err)
	}
	if !report.PublicReaderOnly || !report.HistoricalExact || !report.CurrentClosureExact || !report.StaleExpectedRejected || report.ProductionClaim {
		t.Fatalf("unexpected public conformance report: %+v", report)
	}
	fixture.bindings.values[ports.ProviderBindingRefV2(fixture.projection.Producer)] = fixture.bindings.values[fixture.lookup.ExpectedConsumer]
	if _, err = fixture.gateway.InspectEvidenceSubjectCurrentV1(context.Background(), fixture.lookup); err == nil {
		t.Fatal("producer Binding drift must fail closed")
	}
}

func TestEvidenceSubjectCurrentGatewayV1TypedNilFailsBeforeBackend(t *testing.T) {
	fixture := newEvidenceSubjectGatewayFixtureV1(t)
	var records *typedNilEvidenceSubjectRecordsV1
	fixture.gateway.Records = records
	if _, err := fixture.gateway.InspectEvidenceSubjectCurrentV1(context.Background(), fixture.lookup); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("typed-nil dependency must be unavailable: %v", err)
	}
}

type evidenceSubjectGatewayFixtureV1 struct {
	gateway    kernel.EvidenceSubjectCurrentGatewayV1
	lookup     ports.EvidenceSubjectCurrentLookupRequestV1
	projection ports.EvidenceSubjectCurrentProjectionV1
	bindings   *staticEvidenceSubjectBindingsV1
}

func newEvidenceSubjectGatewayFixtureV1(t *testing.T) evidenceSubjectGatewayFixtureV1 {
	t.Helper()
	base := newEvidenceGovernanceFixtureV2(t, ports.EvidencePartitionInstance, ports.EvidenceTrustObservation)
	if _, err := base.gateway.RegisterGovernedSource(context.Background(), base.source); err != nil {
		t.Fatal(err)
	}
	candidate := governedEvidenceCandidateV2(t, base.source, "event-subject-current", 1, ports.EvidenceTrustObservation)
	record, err := base.gateway.AppendGoverned(context.Background(), ports.EvidenceAppendRequestV2{Candidate: candidate, ExpectedSourceRevision: 1})
	if err != nil {
		t.Fatal(err)
	}
	registration, err := base.ledger.InspectSource(context.Background(), base.source.ID)
	if err != nil {
		t.Fatal(err)
	}
	registrationRef, err := control.NewEvidenceSourceRegistrationRefV1(registration)
	if err != nil {
		t.Fatal(err)
	}
	subject := ports.EvidenceSubjectKeyV1{Record: record.Ref, Source: ports.EvidenceSourceKeyV2{RegistrationID: candidate.RegistrationID, SourceEpoch: candidate.SourceEpoch, SourceSequence: candidate.SourceSequence}}
	subjectDigest, _ := ports.DigestEvidenceSubjectKeyV1(subject)
	scopeDigest, _ := ports.ExecutionScopeDigestV2(candidate.ExecutionScope)
	ledgerDigest, _ := candidate.LedgerScope.DigestV2()
	producerCurrent := evidenceSubjectBindingProjectionV1(t, ports.ProviderBindingRefV2(candidate.Producer), base.now, "producer")
	consumer := ports.ProviderBindingRefV2{BindingSetID: "reader-binding-set", BindingSetRevision: 1, ComponentID: "custom/continuity-reader", ManifestDigest: controlEffectDigestV2(t, "reader-manifest"), ArtifactDigest: controlEffectDigestV2(t, "reader-artifact"), Capability: ports.EvidenceSubjectReaderCapabilityV1}
	consumerCurrent := evidenceSubjectBindingProjectionV1(t, consumer, base.now, "consumer")
	bindings := &staticEvidenceSubjectBindingsV1{values: map[ports.ProviderBindingRefV2]ports.ProviderBindingCurrentProjectionV2{ports.ProviderBindingRefV2(candidate.Producer): producerCurrent, consumer: consumerCurrent}}
	readerBinding, err := ports.EvidenceSubjectReaderBindingFromCurrentV1(consumerCurrent)
	if err != nil {
		t.Fatal(err)
	}
	principal := core.OwnerRef{Domain: "runtime-host", ID: "continuity-reader"}
	associationRef, err := ports.SealEvidenceSubjectConsumerAssociationRefV1(ports.EvidenceSubjectConsumerAssociationRefV1{Revision: 1, Principal: principal, Consumer: consumer, ExecutionScopeDigest: scopeDigest})
	if err != nil {
		t.Fatal(err)
	}
	association, err := ports.SealEvidenceSubjectConsumerAssociationCurrentProjectionV1(ports.EvidenceSubjectConsumerAssociationCurrentProjectionV1{Ref: associationRef, Principal: principal, Consumer: consumer, ExecutionScopeDigest: scopeDigest, BindingCurrent: consumerCurrent, CheckedUnixNano: base.now.UnixNano(), ExpiresUnixNano: base.now.Add(25 * time.Second).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	if err := association.Validate(base.now); err != nil {
		t.Fatal(err)
	}
	readability, err := ports.SealEvidenceReadabilityPolicyRefV1(ports.EvidenceReadabilityPolicyRefV1{PolicyID: "readability-subject-current", Revision: 1, Owner: candidate.Producer, SubjectKeyDigest: subjectDigest, ExecutionScopeDigest: scopeDigest, Consumer: consumer, AllowRead: true, State: ports.EvidenceReadabilityPolicyActiveV1, ExpiresUnixNano: base.now.Add(25 * time.Second).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	absence, err := ports.SealEvidenceTombstoneAbsenceRefV1(ports.EvidenceTombstoneAbsenceRefV1{SubjectKeyDigest: subjectDigest, Revision: 1, OwnerWatermark: 1})
	if err != nil {
		t.Fatal(err)
	}
	presence, err := ports.SealEvidenceSubjectPresenceReadabilityCurrentResultV1(ports.EvidenceSubjectPresenceReadabilityCurrentResultV1{Subject: subject, SubjectKeyDigest: subjectDigest, Presence: ports.EvidenceTombstoneAbsentSealedV1, Readability: ports.EvidenceSubjectReadableV1, TombstoneAbsence: &absence, ReadabilityPolicy: readability, OwnerWatermark: 1, CheckedUnixNano: base.now.UnixNano(), ExpiresUnixNano: base.now.Add(25 * time.Second).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	projection := ports.EvidenceSubjectCurrentProjectionV1{
		Ref: ports.EvidenceSubjectProjectionRefV1{OwnerWatermark: 1}, Subject: subject, SubjectKeyDigest: subjectDigest, Record: record.Ref, Source: subject.Source,
		CandidateDigest: record.CandidateDigest, PreviousRecordDigest: record.PreviousRecordDigest, Registration: registrationRef, RegistrationState: registration.State, RegistrationExpiresUnixNano: registration.ExpiresUnixNano,
		SourcePolicy: candidate.SourcePolicy, SourcePolicyState: base.policy.fact.State, SourcePolicyOwner: base.policy.fact.PolicyOwner, SourcePolicyAuthority: base.policy.fact.PolicyAuthority, SourcePolicyAuthorityCurrent: base.authority.fact, SourcePolicyExpiresUnixNano: base.policy.fact.ExpiresUnixNano,
		LedgerScope: candidate.LedgerScope, LedgerScopeDigest: ledgerDigest, ExecutionScope: candidate.ExecutionScope, ExecutionScopeDigest: scopeDigest, CurrentScope: registration.CurrentScope, CurrentScopeWatermark: registration.CurrentScopeWatermark, ExecutionScopeCurrent: base.current.fact,
		Producer: candidate.Producer, ProducerBindingCurrent: producerCurrent, Authority: candidate.Authority, AuthorityCurrent: base.authority.fact, ActionScopeDigest: registration.ActionScopeDigest, Consumer: consumer, ReaderBinding: readerBinding, ReaderCapability: readerBinding.Capability,
		TrustClass: candidate.TrustClass, ClaimKind: candidate.ClaimKind, EventKind: candidate.EventKind, CustomClass: candidate.CustomClass, Payload: candidate.Payload, Causation: candidate.Causation, CorrelationID: candidate.CorrelationID, OwnerFact: candidate.OwnerFact, HistoricalSource: candidate.HistoricalSource, ObservedUnixNano: candidate.ObservedUnixNano, IngestedUnixNano: record.IngestedUnixNano,
		Presence: presence.Presence, Readability: presence.Readability, TombstoneAbsence: presence.TombstoneAbsence, ReadabilityPolicy: readability, CheckedUnixNano: base.now.UnixNano(), ExpiresUnixNano: base.now.Add(25 * time.Second).UnixNano(),
	}
	request, err := ports.SealEvidenceSubjectMutationRequestV1(ports.EvidenceSubjectMutationRequestV1{ContractVersion: ports.EvidenceSubjectCurrentContractVersionV1, Subject: subject, Kind: ports.EvidenceSubjectMutationSourceRegistrationAdvanceV1, Registration: &registrationRef})
	if err != nil {
		t.Fatal(err)
	}
	commit, projection, index, err := control.NewEvidenceSubjectMutationBundleV1(request, projection, base.now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = base.ledger.PublishEvidenceSubjectMutationV1(context.Background(), commit, projection, index); err != nil {
		t.Fatal(err)
	}
	lookup := ports.EvidenceSubjectCurrentLookupRequestV1{ContractVersion: ports.EvidenceSubjectCurrentContractVersionV1, Subject: subject, ExpectedConsumer: consumer, ExpectedExecutionScopeDigest: scopeDigest, ExpectedSourcePolicy: candidate.SourcePolicy}
	gateway := kernel.EvidenceSubjectCurrentGatewayV1{Facts: base.ledger, Records: base.ledger, Policies: base.policy, CurrentScopes: base.current, Bindings: bindings, Authority: base.authority, Presence: staticEvidenceSubjectPresenceV1{result: presence}, ConsumerAssociations: staticEvidenceSubjectAssociationV1{projection: association}, ConsumerAssociation: associationRef, Clock: func() time.Time { return base.now }}
	return evidenceSubjectGatewayFixtureV1{gateway: gateway, lookup: lookup, projection: projection, bindings: bindings}
}

func evidenceSubjectBindingProjectionV1(t *testing.T, ref ports.ProviderBindingRefV2, now time.Time, suffix string) ports.ProviderBindingCurrentProjectionV2 {
	t.Helper()
	projection, err := ports.SealProviderBindingCurrentProjectionV2(ports.ProviderBindingCurrentProjectionV2{ContractVersion: ports.ProviderBindingCurrentnessContractVersionV2, Ref: ref, State: ports.ProviderBindingCurrentActiveV2, BindingSetDigest: controlEffectDigestV2(t, suffix+"-set"), BindingSetSemanticDigest: controlEffectDigestV2(t, suffix+"-semantic"), BindingID: "binding-" + suffix, BindingRevision: ref.BindingSetRevision, GrantDigest: controlEffectDigestV2(t, suffix+"-grant"), IssuedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	return projection
}

type staticEvidenceSubjectBindingsV1 struct {
	values map[ports.ProviderBindingRefV2]ports.ProviderBindingCurrentProjectionV2
}

func (r *staticEvidenceSubjectBindingsV1) InspectProviderBindingCurrentV2(_ context.Context, ref ports.ProviderBindingRefV2) (ports.ProviderBindingCurrentProjectionV2, error) {
	value, ok := r.values[ref]
	if !ok {
		return ports.ProviderBindingCurrentProjectionV2{}, core.NewError(core.ErrorNotFound, core.ReasonBindingDrift, "binding missing")
	}
	return value, nil
}

type staticEvidenceSubjectPresenceV1 struct {
	result ports.EvidenceSubjectPresenceReadabilityCurrentResultV1
}

func (r staticEvidenceSubjectPresenceV1) InspectEvidenceSubjectPresenceReadabilityCurrentV1(context.Context, ports.EvidenceSubjectPresenceReadabilityCurrentRequestV1) (ports.EvidenceSubjectPresenceReadabilityCurrentResultV1, error) {
	return r.result, nil
}

type staticEvidenceSubjectAssociationV1 struct {
	projection ports.EvidenceSubjectConsumerAssociationCurrentProjectionV1
}

func (r staticEvidenceSubjectAssociationV1) InspectEvidenceSubjectConsumerAssociationCurrentV1(context.Context, ports.EvidenceSubjectConsumerAssociationRefV1) (ports.EvidenceSubjectConsumerAssociationCurrentProjectionV1, error) {
	return r.projection, nil
}

type typedNilEvidenceSubjectRecordsV1 struct{}

func (*typedNilEvidenceSubjectRecordsV1) InspectEvidenceSubjectRecordRegistrationCurrentV1(context.Context, ports.EvidenceSubjectRecordRegistrationCurrentRequestV1) (ports.EvidenceSubjectRecordRegistrationCurrentResultV1, error) {
	return ports.EvidenceSubjectRecordRegistrationCurrentResultV1{}, nil
}
