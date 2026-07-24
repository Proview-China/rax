package control_test

import (
	"context"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/conformance"
	runtimecontrol "github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/fakes"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestReviewEvidenceApplicabilityV1ConformanceUsesEvidenceSubjectCurrentGateway(t *testing.T) {
	fixture := newReviewEvidenceApplicabilityFixtureV1(t)
	report, err := conformance.VerifyReviewEvidenceApplicabilityCurrentV1(context.Background(), conformance.ReviewEvidenceApplicabilityCurrentCaseV1{
		Reader: fixture.gateway, Publisher: fixture.gateway, Create: fixture.create, Now: func() time.Time { return fixture.now },
	})
	if err != nil {
		t.Fatal(err)
	}
	if !report.OneLogicalRevision || !report.ResolvedExact || !report.CurrentExact || !report.HistoricalExact || !report.DeepClone || report.ConcurrentPublishCalls != 64 || report.ProductionClaim {
		t.Fatalf("unexpected Review evidence conformance report: %+v", report)
	}
}

func TestReviewEvidenceApplicabilityV1LostReplyRecoversExactWithoutOriginalContext(t *testing.T) {
	fixture := newReviewEvidenceApplicabilityFixtureV1(t)
	fixture.store.LoseNextReviewEvidenceApplicabilityReplyV1()
	ctx, cancel := context.WithCancel(context.Background())
	reader := cancelAfterPublishFactsV1{inner: fixture.store, cancel: cancel}
	fixture.gateway.Facts = reader
	receipt, err := fixture.gateway.PublishReviewEvidenceApplicabilityV1(ctx, fixture.create)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.PublishID != string(fixture.create.RequestDigest) || receipt.Projection != fixture.create.Projection.Ref {
		t.Fatalf("lost reply recovered another canonical receipt: %+v", receipt)
	}
	if ctx.Err() != context.Canceled {
		t.Fatal("fault wrapper did not cancel the original context")
	}
	changed := fixture.create
	changed.Projection.CheckedUnixNano++
	if _, err := fixture.gateway.PublishReviewEvidenceApplicabilityV1(context.Background(), changed); err == nil {
		t.Fatal("changed content must not reuse the canonical PublishID")
	}
}

func TestReviewEvidenceApplicabilityV1CanonicalReplayKeepsOriginalCommitReceipt(t *testing.T) {
	fixture := newReviewEvidenceApplicabilityFixtureV1(t)
	first, err := fixture.gateway.PublishReviewEvidenceApplicabilityV1(context.Background(), fixture.create)
	if err != nil {
		t.Fatal(err)
	}
	fixture.gateway.Clock = func() time.Time { return fixture.now.Add(time.Second) }
	replayed, err := fixture.gateway.PublishReviewEvidenceApplicabilityV1(context.Background(), fixture.create)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(replayed, first) || replayed.CommittedUnixNano != fixture.now.UnixNano() {
		t.Fatalf("canonical replay rewrote the original commit receipt: first=%+v replay=%+v", first, replayed)
	}
}

func TestReviewEvidenceApplicabilityV1StagedFailureLeaksNoHistoryIndexOrReceipt(t *testing.T) {
	fixture := newReviewEvidenceApplicabilityFixtureV1(t)
	fixture.store.FailNextReviewEvidenceApplicabilityCommitV1()
	if _, err := fixture.gateway.PublishReviewEvidenceApplicabilityV1(context.Background(), fixture.create); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("expected injected staged failure: %v", err)
	}
	if _, err := fixture.store.InspectReviewEvidenceApplicabilityProjectionFactV1(context.Background(), fixture.create.Projection.Ref); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("failed publish leaked history: %v", err)
	}
	if _, err := fixture.store.InspectReviewEvidenceApplicabilityCurrentFactV1(context.Background(), fixture.create.Projection.SubjectDigest); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("failed publish leaked current index: %v", err)
	}
	if _, err := fixture.store.InspectReviewEvidenceApplicabilityPublishFactV1(context.Background(), string(fixture.create.RequestDigest)); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("failed publish leaked receipt: %v", err)
	}
}

func TestReviewEvidenceApplicabilityV1HistorySurvivesCurrentAdvance(t *testing.T) {
	fixture := newReviewEvidenceApplicabilityFixtureV1(t)
	first, err := fixture.gateway.PublishReviewEvidenceApplicabilityV1(context.Background(), fixture.create)
	if err != nil {
		t.Fatal(err)
	}
	nextProjection := fixture.create.Projection
	nextProjection.Ref = ports.ReviewEvidenceApplicabilityRefV1{Revision: 2}
	nextProjection.ProjectionDigest = ""
	nextProjection.Previous = &first.Projection
	nextProjection.CheckedUnixNano++
	nextProjection, err = ports.SealReviewEvidenceApplicabilityProjectionV1(nextProjection)
	if err != nil {
		t.Fatal(err)
	}
	nextIndex, err := ports.SealReviewEvidenceApplicabilityCurrentIndexRefV1(ports.ReviewEvidenceApplicabilityCurrentIndexRefV1{Revision: 2, SubjectDigest: nextProjection.SubjectDigest, Previous: &first.Projection, CurrentProjection: nextProjection.Ref, HighestRevision: 2})
	if err != nil {
		t.Fatal(err)
	}
	next, err := ports.SealPublishReviewEvidenceApplicabilityRequestV1(ports.PublishReviewEvidenceApplicabilityRequestV1{Projection: nextProjection, ExpectedCurrentIndex: &first.CurrentIndex, NextCurrentIndex: nextIndex})
	if err != nil {
		t.Fatal(err)
	}
	fixture.gateway.Clock = func() time.Time { return fixture.now.Add(time.Nanosecond) }
	if _, err = fixture.gateway.PublishReviewEvidenceApplicabilityV1(context.Background(), next); err != nil {
		t.Fatal(err)
	}
	historical, err := fixture.gateway.InspectHistoricalReviewEvidenceApplicabilityV1(context.Background(), first.Projection)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(historical, fixture.create.Projection) {
		t.Fatal("current advance rewrote immutable Review evidence history")
	}
	if _, err = fixture.gateway.InspectCurrentReviewEvidenceApplicabilityV1(context.Background(), first.Projection); err == nil {
		t.Fatal("old historical Ref must not be accepted as current")
	}
}

func TestReviewEvidenceApplicabilityV1EvidenceS1S2DriftFailsClosed(t *testing.T) {
	fixture := newReviewEvidenceApplicabilityFixtureV1(t)
	drifting := &driftingReviewEvidenceSubjectReaderV1{snapshot: fixture.create.Projection.EvidenceSubjectSnapshot}
	fixture.gateway.Evidence = drifting
	if _, err := fixture.gateway.PublishReviewEvidenceApplicabilityV1(context.Background(), fixture.create); !core.HasReason(err, core.ReasonEvidenceConflict) {
		t.Fatalf("underlying EvidenceSubject drift must fail closed: %v", err)
	}
	if _, err := fixture.store.InspectReviewEvidenceApplicabilityCurrentFactV1(context.Background(), fixture.create.Projection.SubjectDigest); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("Evidence drift leaked applicability state: %v", err)
	}
}

func TestReviewEvidenceApplicabilityV1OwnerSnapshotS1S2DriftFailsClosed(t *testing.T) {
	fixture := newReviewEvidenceApplicabilityFixtureV1(t)
	if _, err := fixture.gateway.PublishReviewEvidenceApplicabilityV1(context.Background(), fixture.create); err != nil {
		t.Fatal(err)
	}
	drifting := &driftingReviewEvidenceApplicabilityFactsV1{inner: fixture.store}
	fixture.gateway.Facts = drifting
	got, err := fixture.gateway.InspectCurrentReviewEvidenceApplicabilityV1(context.Background(), fixture.create.Projection.Ref)
	if !core.HasCategory(err, core.ErrorIndeterminate) {
		t.Fatalf("applicability S1/S2 drift must be indeterminate: %v", err)
	}
	if !reflect.DeepEqual(got, ports.ReviewEvidenceApplicabilityCurrentSnapshotV1{}) {
		t.Fatalf("drift leaked a current snapshot: %+v", got)
	}
}

func TestReviewEvidenceApplicabilityV1ClockRollbackAndTTLCrossingAreZeroWrite(t *testing.T) {
	for _, tc := range []struct {
		name  string
		clock []time.Time
	}{
		{name: "rollback", clock: []time.Time{time.Unix(2_100_000_000, 0), time.Unix(2_099_999_999, 0)}},
		{name: "ttl_crossing", clock: []time.Time{time.Unix(2_100_000_000, 0), time.Unix(2_100_000_020, 0)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newReviewEvidenceApplicabilityFixtureV1(t)
			fixture.gateway.Clock = sequenceReviewEvidenceClockV1(tc.clock...)
			if _, err := fixture.gateway.PublishReviewEvidenceApplicabilityV1(context.Background(), fixture.create); err == nil {
				t.Fatal("clock failure must fail closed")
			}
			if _, err := fixture.store.InspectReviewEvidenceApplicabilityCurrentFactV1(context.Background(), fixture.create.Projection.SubjectDigest); !core.HasCategory(err, core.ErrorNotFound) {
				t.Fatalf("clock failure leaked current state: %v", err)
			}
		})
	}
}

func TestReviewEvidenceApplicabilityV1InspectUsesFreshClock(t *testing.T) {
	for _, tc := range []struct {
		name  string
		clock []time.Time
	}{
		{name: "rollback", clock: []time.Time{time.Unix(2_100_000_001, 0), time.Unix(2_100_000_000, 0)}},
		{name: "ttl_crossing", clock: []time.Time{time.Unix(2_100_000_000, 0), time.Unix(2_100_000_020, 0)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newReviewEvidenceApplicabilityFixtureV1(t)
			if _, err := fixture.gateway.PublishReviewEvidenceApplicabilityV1(context.Background(), fixture.create); err != nil {
				t.Fatal(err)
			}
			fixture.gateway.Clock = sequenceReviewEvidenceClockV1(tc.clock...)
			got, err := fixture.gateway.InspectCurrentReviewEvidenceApplicabilityV1(context.Background(), fixture.create.Projection.Ref)
			if err == nil {
				t.Fatal("Inspect clock failure must fail closed")
			}
			if !reflect.DeepEqual(got, ports.ReviewEvidenceApplicabilityCurrentSnapshotV1{}) {
				t.Fatalf("Inspect clock failure leaked current state: %+v", got)
			}
		})
	}
}

func TestReviewEvidenceApplicabilityV1TypedNilDependenciesFailClosed(t *testing.T) {
	fixture := newReviewEvidenceApplicabilityFixtureV1(t)
	var facts *fakes.ReviewEvidenceApplicabilityStoreV1
	fixture.gateway.Facts = facts
	if _, err := fixture.gateway.ResolveReviewEvidenceApplicabilityCurrentV1(context.Background(), ports.ResolveReviewEvidenceApplicabilityCurrentRequestV1{ContractVersion: ports.ReviewEvidenceCurrentContractVersionV1, Subject: fixture.create.Projection.Subject}); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("typed-nil fact store must fail closed: %v", err)
	}
}

type reviewEvidenceApplicabilityFixtureV1 struct {
	now     time.Time
	store   *fakes.ReviewEvidenceApplicabilityStoreV1
	gateway kernel.ReviewEvidenceApplicabilityGatewayV1
	create  ports.PublishReviewEvidenceApplicabilityRequestV1
}

func newReviewEvidenceApplicabilityFixtureV1(t *testing.T) reviewEvidenceApplicabilityFixtureV1 {
	t.Helper()
	evidence := newReviewEvidenceSubjectGatewayFixtureV1(t)
	now := time.Unix(0, evidence.projection.CheckedUnixNano)
	snapshot, err := evidence.gateway.InspectEvidenceSubjectCurrentV1(context.Background(), evidence.lookup)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := ports.SealReviewEvidenceApplicabilityProjectionV1(ports.ReviewEvidenceApplicabilityProjectionV1{
		Ref: ports.ReviewEvidenceApplicabilityRefV1{Revision: 1},
		Subject: ports.ReviewEvidenceApplicabilitySubjectV1{
			TenantID: snapshot.Projection.LedgerScope.TenantID,
			Target:   ports.ReviewEvidenceTargetRefV1{ID: "review-target", Revision: 1, Digest: core.DigestBytes([]byte("review-target-v1"))},
			RunID:    snapshot.Projection.LedgerScope.RunID, Scope: snapshot.Projection.ExecutionScope, ActionScopeDigest: snapshot.Projection.ActionScopeDigest,
			ReviewEvidence: ports.ReviewEvidenceRefV2{Ref: "review-evidence-record", Classification: snapshot.Projection.CustomClass, Digest: snapshot.Projection.CandidateDigest},
		},
		EvidenceSubject: snapshot.Projection.Subject, EvidenceSubjectProjection: snapshot.Projection.Ref, EvidenceSubjectSnapshot: snapshot,
		Record: snapshot.Projection.Record, TrustClass: snapshot.Projection.TrustClass, OwnerFact: snapshot.Projection.OwnerFact,
		CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(20 * time.Second).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	index, err := ports.SealReviewEvidenceApplicabilityCurrentIndexRefV1(ports.ReviewEvidenceApplicabilityCurrentIndexRefV1{Revision: 1, SubjectDigest: projection.SubjectDigest, CurrentProjection: projection.Ref, HighestRevision: 1})
	if err != nil {
		t.Fatal(err)
	}
	create, err := ports.SealPublishReviewEvidenceApplicabilityRequestV1(ports.PublishReviewEvidenceApplicabilityRequestV1{Projection: projection, NextCurrentIndex: index})
	if err != nil {
		t.Fatal(err)
	}
	store := fakes.NewReviewEvidenceApplicabilityStoreV1()
	gateway := kernel.ReviewEvidenceApplicabilityGatewayV1{Facts: store, Evidence: evidence.gateway, Clock: func() time.Time { return now }}
	return reviewEvidenceApplicabilityFixtureV1{now: now, store: store, gateway: gateway, create: create}
}

// newReviewEvidenceSubjectGatewayFixtureV1 uses a Run-partitioned ledger.
// The general EvidenceSubject fixture is intentionally instance-partitioned
// and therefore cannot satisfy Review's exact non-empty Run coordinate.
func newReviewEvidenceSubjectGatewayFixtureV1(t *testing.T) evidenceSubjectGatewayFixtureV1 {
	t.Helper()
	base := newEvidenceGovernanceFixtureV2(t, ports.EvidencePartitionRun, ports.EvidenceTrustObservation)
	if _, err := base.gateway.RegisterGovernedSource(context.Background(), base.source); err != nil {
		t.Fatal(err)
	}
	candidate := governedEvidenceCandidateV2(t, base.source, "event-review-evidence-current", 1, ports.EvidenceTrustObservation)
	record, err := base.gateway.AppendGoverned(context.Background(), ports.EvidenceAppendRequestV2{Candidate: candidate, ExpectedSourceRevision: 1})
	if err != nil {
		t.Fatal(err)
	}
	registration, err := base.ledger.InspectSource(context.Background(), base.source.ID)
	if err != nil {
		t.Fatal(err)
	}
	registrationRef, err := runtimecontrol.NewEvidenceSourceRegistrationRefV1(registration)
	if err != nil {
		t.Fatal(err)
	}
	subject := ports.EvidenceSubjectKeyV1{Record: record.Ref, Source: ports.EvidenceSourceKeyV2{RegistrationID: candidate.RegistrationID, SourceEpoch: candidate.SourceEpoch, SourceSequence: candidate.SourceSequence}}
	subjectDigest, _ := ports.DigestEvidenceSubjectKeyV1(subject)
	scopeDigest, _ := ports.ExecutionScopeDigestV2(candidate.ExecutionScope)
	ledgerDigest, _ := candidate.LedgerScope.DigestV2()
	producerCurrent := evidenceSubjectBindingProjectionV1(t, ports.ProviderBindingRefV2(candidate.Producer), base.now, "review-producer")
	consumer := ports.ProviderBindingRefV2{BindingSetID: "review-reader-binding-set", BindingSetRevision: 1, ComponentID: "custom/review-reader", ManifestDigest: controlEffectDigestV2(t, "review-reader-manifest"), ArtifactDigest: controlEffectDigestV2(t, "review-reader-artifact"), Capability: ports.EvidenceSubjectReaderCapabilityV1}
	consumerCurrent := evidenceSubjectBindingProjectionV1(t, consumer, base.now, "review-consumer")
	bindings := &staticEvidenceSubjectBindingsV1{values: map[ports.ProviderBindingRefV2]ports.ProviderBindingCurrentProjectionV2{ports.ProviderBindingRefV2(candidate.Producer): producerCurrent, consumer: consumerCurrent}}
	readerBinding, err := ports.EvidenceSubjectReaderBindingFromCurrentV1(consumerCurrent)
	if err != nil {
		t.Fatal(err)
	}
	principal := core.OwnerRef{Domain: "runtime-host", ID: "review-reader"}
	associationRef, err := ports.SealEvidenceSubjectConsumerAssociationRefV1(ports.EvidenceSubjectConsumerAssociationRefV1{Revision: 1, Principal: principal, Consumer: consumer, ExecutionScopeDigest: scopeDigest})
	if err != nil {
		t.Fatal(err)
	}
	association, err := ports.SealEvidenceSubjectConsumerAssociationCurrentProjectionV1(ports.EvidenceSubjectConsumerAssociationCurrentProjectionV1{Ref: associationRef, Principal: principal, Consumer: consumer, ExecutionScopeDigest: scopeDigest, BindingCurrent: consumerCurrent, CheckedUnixNano: base.now.UnixNano(), ExpiresUnixNano: base.now.Add(25 * time.Second).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	readability, err := ports.SealEvidenceReadabilityPolicyRefV1(ports.EvidenceReadabilityPolicyRefV1{PolicyID: "readability-review-evidence", Revision: 1, Owner: candidate.Producer, SubjectKeyDigest: subjectDigest, ExecutionScopeDigest: scopeDigest, Consumer: consumer, AllowRead: true, State: ports.EvidenceReadabilityPolicyActiveV1, ExpiresUnixNano: base.now.Add(25 * time.Second).UnixNano()})
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
	commit, projection, index, err := runtimecontrol.NewEvidenceSubjectMutationBundleV1(request, projection, base.now)
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

type cancelAfterPublishFactsV1 struct {
	inner  runtimecontrol.ReviewEvidenceApplicabilityFactPortV1
	cancel context.CancelFunc
}

func (f cancelAfterPublishFactsV1) InspectReviewEvidenceApplicabilityCurrentFactV1(ctx context.Context, digest core.Digest) (ports.ReviewEvidenceApplicabilityCurrentSnapshotV1, error) {
	return f.inner.InspectReviewEvidenceApplicabilityCurrentFactV1(ctx, digest)
}

func (f cancelAfterPublishFactsV1) InspectReviewEvidenceApplicabilityProjectionFactV1(ctx context.Context, ref ports.ReviewEvidenceApplicabilityRefV1) (ports.ReviewEvidenceApplicabilityProjectionV1, error) {
	return f.inner.InspectReviewEvidenceApplicabilityProjectionFactV1(ctx, ref)
}

func (f cancelAfterPublishFactsV1) InspectReviewEvidenceApplicabilityPublishFactV1(ctx context.Context, id string) (ports.ReviewEvidenceApplicabilityPublishReceiptV1, error) {
	return f.inner.InspectReviewEvidenceApplicabilityPublishFactV1(ctx, id)
}

func (f cancelAfterPublishFactsV1) PublishReviewEvidenceApplicabilityFactV1(ctx context.Context, request ports.PublishReviewEvidenceApplicabilityRequestV1, receipt ports.ReviewEvidenceApplicabilityPublishReceiptV1) (ports.ReviewEvidenceApplicabilityPublishReceiptV1, error) {
	got, err := f.inner.PublishReviewEvidenceApplicabilityFactV1(ctx, request, receipt)
	f.cancel()
	return got, err
}

type driftingReviewEvidenceSubjectReaderV1 struct {
	snapshot ports.EvidenceSubjectCurrentSnapshotV1
}

type driftingReviewEvidenceApplicabilityFactsV1 struct {
	inner runtimecontrol.ReviewEvidenceApplicabilityFactPortV1
	mu    sync.Mutex
	reads int
}

func (f *driftingReviewEvidenceApplicabilityFactsV1) InspectReviewEvidenceApplicabilityCurrentFactV1(ctx context.Context, digest core.Digest) (ports.ReviewEvidenceApplicabilityCurrentSnapshotV1, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reads++
	value, err := f.inner.InspectReviewEvidenceApplicabilityCurrentFactV1(ctx, digest)
	if err == nil && f.reads == 2 {
		value.CurrentIndex.Digest = core.DigestBytes([]byte("injected-current-index-drift"))
	}
	return value, err
}

func (f *driftingReviewEvidenceApplicabilityFactsV1) InspectReviewEvidenceApplicabilityProjectionFactV1(ctx context.Context, ref ports.ReviewEvidenceApplicabilityRefV1) (ports.ReviewEvidenceApplicabilityProjectionV1, error) {
	return f.inner.InspectReviewEvidenceApplicabilityProjectionFactV1(ctx, ref)
}

func (f *driftingReviewEvidenceApplicabilityFactsV1) PublishReviewEvidenceApplicabilityFactV1(ctx context.Context, request ports.PublishReviewEvidenceApplicabilityRequestV1, receipt ports.ReviewEvidenceApplicabilityPublishReceiptV1) (ports.ReviewEvidenceApplicabilityPublishReceiptV1, error) {
	return f.inner.PublishReviewEvidenceApplicabilityFactV1(ctx, request, receipt)
}

func (f *driftingReviewEvidenceApplicabilityFactsV1) InspectReviewEvidenceApplicabilityPublishFactV1(ctx context.Context, id string) (ports.ReviewEvidenceApplicabilityPublishReceiptV1, error) {
	return f.inner.InspectReviewEvidenceApplicabilityPublishFactV1(ctx, id)
}

func (r *driftingReviewEvidenceSubjectReaderV1) InspectEvidenceSubjectProjectionV1(context.Context, ports.EvidenceSubjectProjectionRefV1) (ports.EvidenceSubjectCurrentProjectionV1, error) {
	return r.snapshot.Projection, nil
}

func (r *driftingReviewEvidenceSubjectReaderV1) InspectEvidenceSubjectCurrentV1(context.Context, ports.EvidenceSubjectCurrentLookupRequestV1) (ports.EvidenceSubjectCurrentSnapshotV1, error) {
	return r.snapshot, nil
}

func (r *driftingReviewEvidenceSubjectReaderV1) ValidateEvidenceSubjectCurrentV1(context.Context, ports.EvidenceSubjectCurrentValidationRequestV1) (ports.EvidenceSubjectCurrentSnapshotV1, error) {
	return ports.EvidenceSubjectCurrentSnapshotV1{}, nil
}

func sequenceReviewEvidenceClockV1(values ...time.Time) func() time.Time {
	var mu sync.Mutex
	index := 0
	return func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		if index >= len(values) {
			return values[len(values)-1]
		}
		value := values[index]
		index++
		return value
	}
}
