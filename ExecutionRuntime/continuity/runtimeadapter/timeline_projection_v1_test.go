package runtimeadapter

import (
	"context"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/domain"
	continuityports "github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/storage/memory"
	runtimecore "github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestTimelineProjectionAdapterV1PublishesOnlyAfterS1S2AndFreshValidation(t *testing.T) {
	f := newTimelineAdapterFixtureV1(t)
	visible, current, err := f.adapter.Project(context.Background(), f.request)
	if err != nil {
		t.Fatal(err)
	}
	if visible.State != contract.TimelineAttemptVisibleV1 || current.Event.EvidenceRecordRef != string(f.record.Ref.RecordDigest) {
		t.Fatalf("projection was not atomically published: visible=%+v current=%+v", visible, current)
	}
	if f.records.bySourceCalls != 2 || f.records.byRefCalls != 2 || f.current.inspectCalls != 2 || f.current.validateCalls != 1 || f.policy.inspectCalls != 2 || f.policy.validateCalls != 1 {
		t.Fatalf("S1/S2/fresh call closure drifted: records=%+v current=%+v policy=%+v", f.records, f.current, f.policy)
	}
	replayed, replayCurrent, err := f.adapter.Project(context.Background(), f.request)
	if err != nil || replayed.Ref != visible.Ref || replayCurrent != current {
		t.Fatalf("same request must Inspect the create-once result: replayed=%+v current=%+v err=%v", replayed, replayCurrent, err)
	}
	if f.records.bySourceCalls != 2 {
		t.Fatal("completed replay must not reread Runtime or republish")
	}
}

func TestTimelineProjectionAdapterV1RebuildUsesPerItemController(t *testing.T) {
	f := newTimelineAdapterFixtureV1(t)
	results, err := f.adapter.Rebuild(context.Background(), []contract.TimelineProjectionRequestV1{f.request})
	if err != nil || len(results) != 1 || results[0].State != contract.TimelineAttemptVisibleV1 {
		t.Fatalf("governed rebuild failed: results=%+v err=%v", results, err)
	}
	if f.records.bySourceCalls != 2 || f.current.inspectCalls != 2 {
		t.Fatal("rebuild bypassed the per-item S1/S2 path")
	}
}

func TestTimelineProjectionAdapterV1RejectsS1S2DriftBeforeEvent(t *testing.T) {
	f := newTimelineAdapterFixtureV1(t)
	f.current.driftSecond = true
	if _, _, err := f.adapter.Project(context.Background(), f.request); !contract.HasCode(err, contract.ErrProjectionConflict) {
		t.Fatalf("S1/S2 drift must fail closed: %v", err)
	}
	if _, err := f.backend.InspectByEvidence(context.Background(), string(f.record.Ref.RecordDigest)); !contract.HasCode(err, contract.ErrNotFound) {
		t.Fatalf("drift must leave zero Event: %v", err)
	}
}

func TestTimelineProjectionAdapterV1TypedNilAndAuthoritativeRoutingFailClosed(t *testing.T) {
	f := newTimelineAdapterFixtureV1(t)
	var records *timelineRecordReaderV1
	f.adapter.Records = records
	if _, _, err := f.adapter.Project(context.Background(), f.request); !contract.HasCode(err, contract.ErrUnavailable) {
		t.Fatalf("typed-nil reader must fail before use: %v", err)
	}

	f = newTimelineAdapterFixtureV1(t)
	f.record.Candidate.TrustClass = runtimeports.EvidenceTrustAuthoritativeFact
	f.record.Candidate.OwnerFact = runtimeOwnerFactV1(f.record.Candidate, f.request.ScopeDigest)
	f.record.CandidateDigest, _ = f.record.Candidate.DigestV2()
	f.records.record = f.record
	f.current.snapshot.Projection.TrustClass = runtimeports.EvidenceTrustAuthoritativeFact
	f.current.snapshot.Projection.OwnerFact = f.record.Candidate.OwnerFact
	f.current.snapshot.Projection.CandidateDigest = f.record.CandidateDigest
	f.current.snapshot = resealTimelineSnapshotV1(t, f.current.snapshot)
	f.request.OwnerFact = continuityOwnerFactV1(*f.record.Candidate.OwnerFact, f.request.ScopeDigest)
	f.request.Digest, _ = f.request.CanonicalDigest()
	if _, _, err := f.adapter.Project(context.Background(), f.request); !contract.HasCode(err, contract.ErrUnsupported) {
		t.Fatalf("authoritative evidence without typed Owner route must fail closed: %v", err)
	}
}

func TestTimelineProjectionAdapterV1AuthoritativeUsesTypedOwnerS1S2(t *testing.T) {
	f := newTimelineAdapterFixtureV1(t)
	makeAuthoritativeFixtureV1(t, &f)
	reader := &timelineOwnerReaderV2{projection: TimelineOwnerCurrentProjectionV1{Fact: *f.request.OwnerFact, CheckedUnixNano: f.policy.projection.CheckedUnixNano, ExpiresUnixNano: f.policy.projection.ExpiresUnixNano, Digest: string(runtimeDigestV1(t, "owner-current"))}}
	router, err := continuityports.NewClosedTimelineTypedOwnerRouterV2([]continuityports.TimelineTypedOwnerRouteV2{{
		OwnerComponentID: f.request.OwnerFact.Owner.ComponentID,
		Capability:       f.request.OwnerFact.Owner.Capability,
		FactKind:         f.request.OwnerFact.FactKind,
		PayloadSchema:    f.request.OwnerFact.PayloadSchema,
		Reader:           reader,
	}})
	if err != nil {
		t.Fatal(err)
	}
	f.adapter.OwnerRouterV2 = router
	visible, _, err := f.adapter.Project(context.Background(), f.request)
	if err != nil {
		t.Fatal(err)
	}
	if visible.OwnerProjectionDigest != reader.projection.Digest || reader.inspectCalls != 2 || reader.validateCalls != 1 {
		t.Fatalf("typed Owner S1/S2/fresh closure drifted: visible=%+v reader=%+v", visible, reader)
	}
	for _, request := range reader.requests {
		if request.TenantID != f.record.Candidate.ExecutionScope.Identity.TenantID || request.Fact != *f.request.OwnerFact {
			t.Fatalf("typed Owner request lost exact tenant/fact coordinates: %+v", request)
		}
	}
}

func TestTimelineProjectionAdapterV1LostPublishReplyOnlyInspect(t *testing.T) {
	f := newTimelineAdapterFixtureV1(t)
	failing := &lostReplyTimelineRepositoryV1{TimelineGovernanceRepositoryV1: f.backend, failOnce: true}
	controller, err := domain.NewTimelineProjectionControllerV1(failing, timelineClockV1{f.adapter.Clock()})
	if err != nil {
		t.Fatal(err)
	}
	f.adapter.Controller = controller
	_, _, err = f.adapter.Project(context.Background(), f.request)
	if !contract.HasCode(err, contract.ErrIndeterminate) {
		t.Fatalf("lost publish reply must be indeterminate: %v", err)
	}
	stored, err := f.backend.InspectCurrentTimelineProjectionAttemptV1(context.Background(), f.request.ScopeDigest, f.request.AttemptID)
	if err != nil || stored.State != contract.TimelineAttemptVisibleV1 {
		t.Fatalf("atomic commit must remain inspectable after lost reply: stored=%+v err=%v", stored, err)
	}
	inspected, err := f.adapter.InspectAttempt(context.Background(), stored.Ref)
	if err != nil || inspected.Ref != stored.Ref || inspected.Event == nil {
		t.Fatalf("recovery must Inspect exact committed attempt: inspected=%+v err=%v", inspected, err)
	}
}

func TestTimelineProjectionAdapterV1SixTrustClassesAreClosed(t *testing.T) {
	values := map[runtimeports.EvidenceTrustClassV2]contract.TrustClass{
		runtimeports.EvidenceTrustObservation: contract.TrustObservation, runtimeports.EvidenceTrustLateObservation: contract.TrustLateObservation,
		runtimeports.EvidenceTrustReceipt: contract.TrustReceipt, runtimeports.EvidenceTrustAttestation: contract.TrustAttestation,
		runtimeports.EvidenceTrustClaim: contract.TrustClaim, runtimeports.EvidenceTrustAuthoritativeFact: contract.TrustAuthoritativeFact,
	}
	for input, expected := range values {
		actual, err := mapTrustClass(input)
		if err != nil || actual != expected {
			t.Fatalf("trust route %q drifted: got=%q err=%v", input, actual, err)
		}
	}
	if _, err := mapTrustClass("custom/trust"); !contract.HasCode(err, contract.ErrInvalidArgument) {
		t.Fatalf("open trust class must reject: %v", err)
	}
}

type timelineAdapterFixtureV1 struct {
	adapter *TimelineProjectionAdapterV1
	request contract.TimelineProjectionRequestV1
	record  runtimeports.EvidenceLedgerRecordV2
	records *timelineRecordReaderV1
	current *timelineCurrentReaderV1
	policy  *timelinePolicyReaderV1
	backend *memory.Backend
}

type lostReplyTimelineRepositoryV1 struct {
	continuityports.TimelineGovernanceRepositoryV1
	failOnce bool
}

func (r *lostReplyTimelineRepositoryV1) PublishTimelineProjectionV1(ctx context.Context, request continuityports.PublishTimelineProjectionV1Request) (contract.TimelineProjectionAttemptFactV1, contract.TimelineProjectionCurrentV1, error) {
	attempt, current, err := r.TimelineGovernanceRepositoryV1.PublishTimelineProjectionV1(ctx, request)
	if err == nil && r.failOnce {
		r.failOnce = false
		return contract.TimelineProjectionAttemptFactV1{}, contract.TimelineProjectionCurrentV1{}, contract.NewError(contract.ErrIndeterminate, "publish", "injected lost reply")
	}
	return attempt, current, err
}

type timelineClockV1 struct{ now time.Time }

func (c timelineClockV1) Now() time.Time { return c.now }

func newTimelineAdapterFixtureV1(t *testing.T) timelineAdapterFixtureV1 {
	t.Helper()
	now := time.Unix(1_800_000_000, 0)
	d := func(value string) runtimecore.Digest { return runtimeDigestV1(t, value) }
	scope := runtimecore.ExecutionScope{Identity: runtimecore.AgentIdentityRef{TenantID: "tenant-a", ID: "identity-a", Epoch: 1}, Lineage: runtimecore.LineageRef{ID: "lineage-a", PlanDigest: d("plan")}, Instance: runtimecore.InstanceRef{ID: "instance-a", Epoch: 1}, AuthorityEpoch: 1}
	scopeDigest, err := runtimeports.ExecutionScopeDigestV2(scope)
	if err != nil {
		t.Fatal(err)
	}
	ledger := runtimeports.EvidenceLedgerScopeV2{Partition: runtimeports.EvidencePartitionInstance, TenantID: scope.Identity.TenantID, IdentityID: scope.Identity.ID, LineageID: scope.Lineage.ID, InstanceID: scope.Instance.ID}
	ledgerDigest, err := ledger.DigestV2()
	if err != nil {
		t.Fatal(err)
	}
	producer := runtimeports.EvidenceProducerBindingRefV2{BindingSetID: "producer-set", BindingSetRevision: 1, ComponentID: "custom/evidence-producer", ManifestDigest: d("producer-manifest"), ArtifactDigest: d("producer-artifact"), Capability: "runtime/evidence-append"}
	authority := runtimeports.AuthorityBindingRefV2{Ref: "authority-a", Digest: d("authority"), Revision: 1, Epoch: 1}
	policyRef := runtimeports.EvidenceSourcePolicyBindingRefV2{Ref: "source-policy-a", Digest: d("source-policy"), Revision: 1}
	candidate := runtimeports.EvidenceEventCandidateV2{ContractVersion: runtimeports.EvidenceContractVersionV2, LedgerScope: ledger, EventID: "event-a", RegistrationID: "registration-a", RegistrationRevision: 1, SourceConfigurationDigest: d("configuration"), SourcePolicy: policyRef, SourceID: "custom/source", SourceEpoch: 1, SourceSequence: 1, TrustClass: runtimeports.EvidenceTrustObservation, EventKind: "custom/event", CustomClass: "custom/observation", ExecutionScope: scope, Payload: runtimeports.EvidencePayloadRefV2{Schema: runtimeports.SchemaRefV2{Namespace: "custom", Name: "event", Version: "1.0.0", MediaType: "application/octet-stream", ContentDigest: d("schema")}, ContentDigest: d("payload"), Revision: 1, Length: 1, Ref: "memory://event-a"}, Causation: []runtimeports.EvidenceCausationRefV2{}, CorrelationID: "correlation-a", Producer: producer, Authority: authority, ObservedUnixNano: now.Add(-time.Second).UnixNano()}
	candidateDigest, err := candidate.DigestV2()
	if err != nil {
		t.Fatal(err)
	}
	record := runtimeports.EvidenceLedgerRecordV2{Ref: runtimeports.EvidenceRecordRefV2{LedgerScopeDigest: ledgerDigest, Sequence: 1, RecordDigest: d("record")}, Candidate: candidate, CandidateDigest: candidateDigest, PreviousRecordDigest: d("genesis"), IngestedUnixNano: now.Add(-500 * time.Millisecond).UnixNano()}
	if err := record.Validate(); err != nil {
		t.Fatal(err)
	}
	source := runtimeports.EvidenceSourceKeyV2{RegistrationID: candidate.RegistrationID, SourceEpoch: candidate.SourceEpoch, SourceSequence: candidate.SourceSequence}
	subject := runtimeports.EvidenceSubjectKeyV1{Record: record.Ref, Source: source}
	subjectDigest, _ := runtimeports.DigestEvidenceSubjectKeyV1(subject)
	absence, err := runtimeports.SealEvidenceTombstoneAbsenceRefV1(runtimeports.EvidenceTombstoneAbsenceRefV1{SubjectKeyDigest: subjectDigest, Revision: 1, OwnerWatermark: 1})
	if err != nil {
		t.Fatal(err)
	}
	consumer := runtimeports.ProviderBindingRefV2{BindingSetID: "consumer-set", BindingSetRevision: 1, ComponentID: "custom/continuity-reader", ManifestDigest: d("consumer-manifest"), ArtifactDigest: d("consumer-artifact"), Capability: runtimeports.EvidenceSubjectReaderCapabilityV1}
	capability := runtimeports.EvidenceSubjectReaderCapabilityRefV1{Name: runtimeports.EvidenceSubjectReaderCapabilityV1, BindingRevision: 1, GrantDigest: d("reader-grant"), BindingCurrentProjectionDigest: d("reader-current"), IssuedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(time.Minute).UnixNano()}
	readerBinding := runtimeports.EvidenceSubjectReaderBindingRefV1{Binding: consumer, BindingSetDigest: d("consumer-set-digest"), BindingSetSemanticDigest: d("consumer-semantic"), BindingID: "reader-binding", Capability: capability}
	registration := runtimeports.EvidenceSourceRegistrationRefV1{RegistrationID: candidate.RegistrationID, Revision: 1, FactDigest: d("registration-fact"), ConfigurationDigest: candidate.SourceConfigurationDigest, SourceID: candidate.SourceID, SourceEpoch: candidate.SourceEpoch}
	projection, err := runtimeports.SealEvidenceSubjectCurrentProjectionV1(runtimeports.EvidenceSubjectCurrentProjectionV1{Ref: runtimeports.EvidenceSubjectProjectionRefV1{Revision: 1, OwnerWatermark: 1}, Subject: subject, SubjectKeyDigest: subjectDigest, Record: record.Ref, Source: source, CandidateDigest: record.CandidateDigest, PreviousRecordDigest: record.PreviousRecordDigest, Registration: registration, SourcePolicy: policyRef, LedgerScope: ledger, LedgerScopeDigest: ledgerDigest, ExecutionScope: scope, ExecutionScopeDigest: scopeDigest, Producer: producer, Consumer: consumer, ReaderBinding: readerBinding, ReaderCapability: capability, TrustClass: candidate.TrustClass, EventKind: candidate.EventKind, CustomClass: candidate.CustomClass, Payload: candidate.Payload, Causation: candidate.Causation, CorrelationID: candidate.CorrelationID, ObservedUnixNano: candidate.ObservedUnixNano, IngestedUnixNano: record.IngestedUnixNano, Presence: runtimeports.EvidenceTombstoneAbsentSealedV1, Readability: runtimeports.EvidenceSubjectReadableV1, TombstoneAbsence: &absence, CheckedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(30 * time.Second).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	index, err := runtimeports.SealEvidenceSubjectCurrentIndexRefV1(runtimeports.EvidenceSubjectCurrentIndexRefV1{Revision: 1, SubjectKeyDigest: subjectDigest, CurrentProjection: projection.Ref, OwnerWatermark: 1})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := runtimeports.EvidenceSubjectCurrentSnapshotV1{ContractVersion: runtimeports.EvidenceSubjectCurrentContractVersionV1, Projection: projection, CurrentIndex: index}
	backend := memory.NewWithClock(func() time.Time { return now })
	controller, err := domain.NewTimelineProjectionControllerV1(backend, timelineClockV1{now})
	if err != nil {
		t.Fatal(err)
	}
	request := contract.TimelineProjectionRequestV1{ContractVersion: contract.TimelineGovernanceContractVersionV1, AttemptID: "attempt-a", IdempotencyKey: "idem-a", EvidenceSource: contract.EvidenceSourceKey{RegistrationID: candidate.RegistrationID, SourceEpoch: uint64(candidate.SourceEpoch), SourceSequence: candidate.SourceSequence}, ProjectionPolicy: "projection-policy-a", ScopeDigest: string(scopeDigest)}
	request.Digest, _ = request.CanonicalDigest()
	records := &timelineRecordReaderV1{record: record}
	current := &timelineCurrentReaderV1{snapshot: snapshot}
	policyProjection, err := contract.SealTimelineProjectionPolicyCurrentV1(contract.TimelineProjectionPolicyCurrentV1{Ref: contract.TimelineProjectionPolicyRefV1{PolicyID: request.ProjectionPolicy, Revision: 1, ScopeDigest: request.ScopeDigest}, State: contract.TimelineProjectionPolicyActiveV1, CheckedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(20 * time.Second).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = backend.CreateTimelineProjectionPolicyV1(context.Background(), policyProjection); err != nil {
		t.Fatal(err)
	}
	policy := &timelinePolicyReaderV1{delegate: backend, projection: policyProjection}
	adapter := &TimelineProjectionAdapterV1{Controller: controller, Records: records, Current: current, Consumer: consumer, Policy: policy, Clock: func() time.Time { return now }}
	return timelineAdapterFixtureV1{adapter: adapter, request: request, record: record, records: records, current: current, policy: policy, backend: backend}
}

type timelineRecordReaderV1 struct {
	record                    runtimeports.EvidenceLedgerRecordV2
	bySourceCalls, byRefCalls int
}

func (r *timelineRecordReaderV1) InspectBySource(context.Context, runtimeports.EvidenceSourceKeyV2) (runtimeports.EvidenceLedgerRecordV2, error) {
	r.bySourceCalls++
	return r.record, nil
}
func (r *timelineRecordReaderV1) InspectRecord(context.Context, runtimeports.EvidenceRecordRefV2) (runtimeports.EvidenceLedgerRecordV2, error) {
	r.byRefCalls++
	return r.record, nil
}

type timelineCurrentReaderV1 struct {
	snapshot                    runtimeports.EvidenceSubjectCurrentSnapshotV1
	inspectCalls, validateCalls int
	driftSecond                 bool
}

func (r *timelineCurrentReaderV1) InspectEvidenceSubjectProjectionV1(context.Context, runtimeports.EvidenceSubjectProjectionRefV1) (runtimeports.EvidenceSubjectCurrentProjectionV1, error) {
	return r.snapshot.Projection, nil
}
func (r *timelineCurrentReaderV1) InspectEvidenceSubjectCurrentV1(context.Context, runtimeports.EvidenceSubjectCurrentLookupRequestV1) (runtimeports.EvidenceSubjectCurrentSnapshotV1, error) {
	r.inspectCalls++
	value := r.snapshot
	if r.driftSecond && r.inspectCalls == 2 {
		value.Projection.ExpiresUnixNano++
	}
	return value, nil
}
func (r *timelineCurrentReaderV1) ValidateEvidenceSubjectCurrentV1(_ context.Context, request runtimeports.EvidenceSubjectCurrentValidationRequestV1) (runtimeports.EvidenceSubjectCurrentSnapshotV1, error) {
	r.validateCalls++
	if err := request.Validate(); err != nil {
		return runtimeports.EvidenceSubjectCurrentSnapshotV1{}, err
	}
	return r.snapshot, nil
}

type timelinePolicyReaderV1 struct {
	delegate                    continuityports.TimelineProjectionPolicyCurrentReaderV1
	projection                  TimelineProjectionPolicyCurrentV1
	inspectCalls, validateCalls int
}

func (r *timelinePolicyReaderV1) InspectTimelineProjectionPolicyCurrentV1(ctx context.Context, policy, scope string) (TimelineProjectionPolicyCurrentV1, error) {
	r.inspectCalls++
	return r.delegate.InspectTimelineProjectionPolicyCurrentV1(ctx, policy, scope)
}
func (r *timelinePolicyReaderV1) ValidateTimelineProjectionPolicyCurrentV1(ctx context.Context, projection TimelineProjectionPolicyCurrentV1) error {
	r.validateCalls++
	return r.delegate.ValidateTimelineProjectionPolicyCurrentV1(ctx, projection)
}

func runtimeDigestV1(t *testing.T, value string) runtimecore.Digest {
	t.Helper()
	d, err := runtimecore.CanonicalJSONDigest("test.continuity", "1.0.0", "Digest", value)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func runtimeOwnerFactV1(candidate runtimeports.EvidenceEventCandidateV2, scope string) *runtimeports.EvidenceOwnerFactRefV2 {
	return &runtimeports.EvidenceOwnerFactRefV2{Owner: candidate.Producer, FactKind: "custom/fact", FactID: "fact-a", Revision: 1, FactDigest: runtimecore.Digest(scope), PayloadSchema: candidate.Payload.Schema, PayloadDigest: candidate.Payload.ContentDigest, PayloadRevision: candidate.Payload.Revision}
}

func makeAuthoritativeFixtureV1(t *testing.T, f *timelineAdapterFixtureV1) {
	t.Helper()
	f.record.Candidate.TrustClass = runtimeports.EvidenceTrustAuthoritativeFact
	f.record.Candidate.OwnerFact = runtimeOwnerFactV1(f.record.Candidate, f.request.ScopeDigest)
	f.record.CandidateDigest, _ = f.record.Candidate.DigestV2()
	f.records.record = f.record
	f.current.snapshot.Projection.TrustClass = runtimeports.EvidenceTrustAuthoritativeFact
	f.current.snapshot.Projection.OwnerFact = f.record.Candidate.OwnerFact
	f.current.snapshot.Projection.CandidateDigest = f.record.CandidateDigest
	f.current.snapshot = resealTimelineSnapshotV1(t, f.current.snapshot)
	f.request.OwnerFact = continuityOwnerFactV1(*f.record.Candidate.OwnerFact, f.request.ScopeDigest)
	f.request.Digest, _ = f.request.CanonicalDigest()
}

type timelineOwnerReaderV2 struct {
	projection                  TimelineOwnerCurrentProjectionV1
	inspectCalls, validateCalls int
	requests                    []TimelineOwnerCurrentInspectRequestV2
}

func (r *timelineOwnerReaderV2) InspectTimelineOwnerCurrentV2(_ context.Context, request TimelineOwnerCurrentInspectRequestV2) (TimelineOwnerCurrentProjectionV1, error) {
	r.inspectCalls++
	r.requests = append(r.requests, request)
	return r.projection, nil
}
func (r *timelineOwnerReaderV2) ValidateTimelineOwnerCurrentV2(_ context.Context, request TimelineOwnerCurrentInspectRequestV2, _ TimelineOwnerCurrentProjectionV1) error {
	r.validateCalls++
	r.requests = append(r.requests, request)
	return nil
}

func continuityOwnerFactV1(value runtimeports.EvidenceOwnerFactRefV2, scope string) *contract.TimelineOwnerFactRefV1 {
	return &contract.TimelineOwnerFactRefV1{Owner: contract.OwnerBinding{BindingSetID: value.Owner.BindingSetID, BindingRevision: uint64(value.Owner.BindingSetRevision), ComponentID: string(value.Owner.ComponentID), ManifestDigest: string(value.Owner.ManifestDigest), ArtifactDigest: string(value.Owner.ArtifactDigest), Capability: string(value.Owner.Capability), FactKind: string(value.FactKind)}, FactKind: string(value.FactKind), FactID: value.FactID, Revision: uint64(value.Revision), FactDigest: string(value.FactDigest), PayloadSchema: value.PayloadSchema.Key(), PayloadDigest: string(value.PayloadDigest), PayloadRevision: uint64(value.PayloadRevision), ScopeDigest: scope}
}

func resealTimelineSnapshotV1(t *testing.T, value runtimeports.EvidenceSubjectCurrentSnapshotV1) runtimeports.EvidenceSubjectCurrentSnapshotV1 {
	t.Helper()
	value.Projection.Ref.Digest, value.Projection.ProjectionDigest = "", ""
	p, err := runtimeports.SealEvidenceSubjectCurrentProjectionV1(value.Projection)
	if err != nil {
		t.Fatal(err)
	}
	value.Projection = p
	value.CurrentIndex.CurrentProjection = p.Ref
	value.CurrentIndex.Digest = ""
	index, err := runtimeports.SealEvidenceSubjectCurrentIndexRefV1(value.CurrentIndex)
	if err != nil {
		t.Fatal(err)
	}
	value.CurrentIndex = index
	return value
}
