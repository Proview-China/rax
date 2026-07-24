package ports_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestReviewEvidenceCurrentV1LiteralCanonicalAndStableIDs(t *testing.T) {
	now := time.Unix(2_100_000_000, 0)
	snapshot := reviewEvidenceCurrentFixtureV1(t, now, ports.EvidenceTrustObservation)
	subjectDigest, err := ports.DigestReviewEvidenceApplicabilitySubjectV1(snapshot.Projection.Subject)
	if err != nil {
		t.Fatal(err)
	}
	const expectedSubjectDigest = "sha256:bd2068d47de056bf9ca2d2d5af72b89df5b5c2144336b3d554f540c5983f084e"
	if string(subjectDigest) != expectedSubjectDigest {
		t.Fatalf("literal subject digest drifted: got=%s want=%s", subjectDigest, expectedSubjectDigest)
	}
	projectionID, err := ports.DeriveReviewEvidenceApplicabilityProjectionIDV1(subjectDigest)
	if err != nil {
		t.Fatal(err)
	}
	const expectedProjectionID = "sha256:e40306f88b82cc8a5e708259812538abf59cc808722fdd5cfd10f88b342b65ef"
	if projectionID != expectedProjectionID {
		t.Fatalf("literal projection ID drifted: got=%s want=%s", projectionID, expectedProjectionID)
	}
	indexID, err := ports.DeriveReviewEvidenceApplicabilityCurrentIndexIDV1(subjectDigest)
	if err != nil {
		t.Fatal(err)
	}
	const expectedIndexID = "sha256:7f9eabca1a8ec2cb7686d61c2320e299fbe48e413e8e8d25647b00c7b5f7056e"
	if indexID != expectedIndexID {
		t.Fatalf("literal index ID drifted: got=%s want=%s", indexID, expectedIndexID)
	}

	nextInput := snapshot.Projection
	nextInput.Ref = ports.ReviewEvidenceApplicabilityRefV1{Revision: 2}
	nextInput.Previous = &snapshot.Projection.Ref
	nextInput.CheckedUnixNano = now.Add(time.Second).UnixNano()
	nextInput.ExpiresUnixNano = now.Add(20 * time.Second).UnixNano()
	nextInput.ProjectionDigest = ""
	next, err := ports.SealReviewEvidenceApplicabilityProjectionV1(nextInput)
	if err != nil {
		t.Fatal(err)
	}
	if next.Ref.ProjectionID != snapshot.Projection.Ref.ProjectionID {
		t.Fatalf("revision changed stable projection ID: first=%s next=%s", snapshot.Projection.Ref.ProjectionID, next.Ref.ProjectionID)
	}
}

func TestReviewEvidenceCurrentV1RejectsTargetScopeAndNominalDrift(t *testing.T) {
	now := time.Unix(2_100_000_100, 0)
	base := reviewEvidenceCurrentFixtureV1(t, now, ports.EvidenceTrustObservation).Projection
	cases := map[string]func(*ports.ReviewEvidenceApplicabilityProjectionV1){
		"target_id":       func(p *ports.ReviewEvidenceApplicabilityProjectionV1) { p.Subject.Target.ID = "other-target" },
		"target_revision": func(p *ports.ReviewEvidenceApplicabilityProjectionV1) { p.Subject.Target.Revision++ },
		"target_digest": func(p *ports.ReviewEvidenceApplicabilityProjectionV1) {
			p.Subject.Target.Digest = reviewEvidenceDigestV1("other-target")
		},
		"tenant": func(p *ports.ReviewEvidenceApplicabilityProjectionV1) { p.Subject.TenantID = "other-tenant" },
		"run":    func(p *ports.ReviewEvidenceApplicabilityProjectionV1) { p.Subject.RunID = "other-run" },
		"scope":  func(p *ports.ReviewEvidenceApplicabilityProjectionV1) { p.Subject.Scope.Instance.Epoch++ },
		"action_scope": func(p *ports.ReviewEvidenceApplicabilityProjectionV1) {
			p.Subject.ActionScopeDigest = reviewEvidenceDigestV1("other-action")
		},
		"classification": func(p *ports.ReviewEvidenceApplicabilityProjectionV1) {
			p.Subject.ReviewEvidence.Classification = "review/other-evidence"
		},
		"review_digest": func(p *ports.ReviewEvidenceApplicabilityProjectionV1) {
			p.Subject.ReviewEvidence.Digest = reviewEvidenceDigestV1("other-review")
		},
		"record":             func(p *ports.ReviewEvidenceApplicabilityProjectionV1) { p.Record.Sequence++ },
		"subject_projection": func(p *ports.ReviewEvidenceApplicabilityProjectionV1) { p.EvidenceSubjectProjection.Revision++ },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			changed := ports.CloneReviewEvidenceApplicabilityProjectionV1(base)
			mutate(&changed)
			if err := changed.Validate(); err == nil {
				t.Fatal("drifted exact applicability passed Validate")
			}
		})
	}
}

func TestReviewEvidenceCurrentV1OwnerFactTrustIsClosed(t *testing.T) {
	now := time.Unix(2_100_000_200, 0)
	observation := reviewEvidenceCurrentFixtureV1(t, now, ports.EvidenceTrustObservation).Projection
	observation.OwnerFact = &ports.EvidenceOwnerFactRefV2{}
	observation.Ref.Digest, observation.ProjectionDigest = "", ""
	if _, err := ports.SealReviewEvidenceApplicabilityProjectionV1(observation); !core.HasReason(err, core.ReasonEvidenceTrustInvalid) {
		t.Fatalf("Observation forged an Owner fact: %v", err)
	}

	authoritative := reviewEvidenceProjectionInputV1(t, now, ports.EvidenceTrustAuthoritativeFact)
	if _, err := ports.SealReviewEvidenceApplicabilityProjectionV1(authoritative); !core.HasReason(err, core.ReasonEvidenceTrustInvalid) {
		t.Fatalf("authoritative evidence omitted Owner fact: %v", err)
	}

	unknown := reviewEvidenceProjectionInputV1(t, now, ports.EvidenceTrustClassV2("custom/trust"))
	if _, err := ports.SealReviewEvidenceApplicabilityProjectionV1(unknown); !core.HasReason(err, core.ReasonEvidenceTrustInvalid) {
		t.Fatalf("custom trust class was accepted: %v", err)
	}
}

func TestReviewEvidenceCurrentV1FixedTTLAndClockFailClosed(t *testing.T) {
	now := time.Unix(2_100_000_300, 0)
	snapshot := reviewEvidenceCurrentFixtureV1(t, now, ports.EvidenceTrustObservation)
	if err := snapshot.ValidateCurrent(snapshot.Projection.Ref, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := snapshot.ValidateCurrent(snapshot.Projection.Ref, now.Add(-time.Nanosecond)); !core.HasReason(err, core.ReasonClockRegression) {
		t.Fatalf("clock rollback was accepted: %v", err)
	}
	if err := snapshot.ValidateCurrent(snapshot.Projection.Ref, time.Unix(0, snapshot.Projection.ExpiresUnixNano)); !core.HasReason(err, core.ReasonEvidenceSourceStale) {
		t.Fatalf("TTL boundary was accepted: %v", err)
	}

	tooLong := reviewEvidenceProjectionInputV1(t, now, ports.EvidenceTrustObservation)
	tooLong.ExpiresUnixNano = tooLong.EvidenceSubjectSnapshot.Projection.ExpiresUnixNano + 1
	if _, err := ports.SealReviewEvidenceApplicabilityProjectionV1(tooLong); !core.HasReason(err, core.ReasonEvidenceSourceStale) {
		t.Fatalf("applicability exceeded Evidence TTL: %v", err)
	}
}

func TestReviewEvidenceCurrentV1PublisherCASReceiptAndLostReplyKey(t *testing.T) {
	now := time.Unix(2_100_000_400, 0)
	snapshot := reviewEvidenceCurrentFixtureV1(t, now, ports.EvidenceTrustObservation)
	request, err := ports.SealPublishReviewEvidenceApplicabilityRequestV1(ports.PublishReviewEvidenceApplicabilityRequestV1{
		Projection: snapshot.Projection, NextCurrentIndex: snapshot.CurrentIndex,
	})
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := ports.SealReviewEvidenceApplicabilityPublishReceiptV1(ports.ReviewEvidenceApplicabilityPublishReceiptV1{
		RequestDigest: request.RequestDigest, Projection: snapshot.Projection.Ref, CurrentIndex: snapshot.CurrentIndex, CommittedUnixNano: now.UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if receipt.PublishID != string(request.RequestDigest) {
		t.Fatalf("lost-reply Inspect key is not the same canonical request: %+v", receipt)
	}
	changed := request
	changed.Projection.Subject.Target.ID = "changed"
	if err := changed.Validate(); err == nil {
		t.Fatal("same publish request digest accepted changed content")
	}
	badCAS := request
	badCAS.ExpectedCurrentIndex = &snapshot.CurrentIndex
	if err := badCAS.Validate(); err == nil {
		t.Fatal("first publish accepted a previous current index")
	}
}

func TestReviewEvidenceCurrentV1HistoricalRefAndCloneAreExact(t *testing.T) {
	now := time.Unix(2_100_000_500, 0)
	snapshot := reviewEvidenceCurrentFixtureV1(t, now, ports.EvidenceTrustObservation)
	cloned := ports.CloneReviewEvidenceApplicabilityCurrentSnapshotV1(snapshot)
	cloned.Projection.EvidenceSubjectSnapshot.Projection.Causation = append(cloned.Projection.EvidenceSubjectSnapshot.Projection.Causation, ports.EvidenceCausationRefV2{})
	if len(snapshot.Projection.EvidenceSubjectSnapshot.Projection.Causation) != 0 {
		t.Fatal("deep clone aliased nested Evidence slices")
	}
	changedRef := snapshot.Projection.Ref
	changedRef.Revision++
	if err := snapshot.ValidateCurrent(changedRef, now); err == nil {
		t.Fatal("current validation accepted a non-exact ref")
	}
	if err := snapshot.Projection.Validate(); err != nil {
		t.Fatalf("historical immutable projection no longer validates independently: %v", err)
	}
}

func TestReviewEvidenceCurrentV1PublicReaderAndOwnerPublisherStayDistinct(t *testing.T) {
	reader := reflect.TypeOf((*ports.ReviewEvidenceApplicabilityCurrentReaderV1)(nil)).Elem()
	publisher := reflect.TypeOf((*ports.ReviewEvidenceApplicabilityOwnerPublisherV1)(nil)).Elem()
	if reader.Implements(publisher) || publisher.Implements(reader) {
		t.Fatal("Review Evidence read and Owner publish capabilities collapsed")
	}
	for _, method := range []string{"PublishReviewEvidenceApplicabilityV1", "InspectReviewEvidenceApplicabilityPublishV1"} {
		if _, ok := reader.MethodByName(method); ok {
			t.Fatalf("public Reader leaked Owner mutation method %s", method)
		}
	}
	var _ ports.ReviewEvidenceApplicabilityCurrentReaderV1 = reviewEvidenceReaderShapeV1{}
	var _ ports.ReviewEvidenceApplicabilityOwnerPublisherV1 = reviewEvidencePublisherShapeV1{}
}

type reviewEvidenceReaderShapeV1 struct{}

func (reviewEvidenceReaderShapeV1) ResolveReviewEvidenceApplicabilityCurrentV1(context.Context, ports.ResolveReviewEvidenceApplicabilityCurrentRequestV1) (ports.ReviewEvidenceApplicabilityCurrentSnapshotV1, error) {
	return ports.ReviewEvidenceApplicabilityCurrentSnapshotV1{}, nil
}
func (reviewEvidenceReaderShapeV1) InspectCurrentReviewEvidenceApplicabilityV1(context.Context, ports.ReviewEvidenceApplicabilityRefV1) (ports.ReviewEvidenceApplicabilityCurrentSnapshotV1, error) {
	return ports.ReviewEvidenceApplicabilityCurrentSnapshotV1{}, nil
}
func (reviewEvidenceReaderShapeV1) InspectHistoricalReviewEvidenceApplicabilityV1(context.Context, ports.ReviewEvidenceApplicabilityRefV1) (ports.ReviewEvidenceApplicabilityProjectionV1, error) {
	return ports.ReviewEvidenceApplicabilityProjectionV1{}, nil
}

type reviewEvidencePublisherShapeV1 struct{}

func (reviewEvidencePublisherShapeV1) PublishReviewEvidenceApplicabilityV1(context.Context, ports.PublishReviewEvidenceApplicabilityRequestV1) (ports.ReviewEvidenceApplicabilityPublishReceiptV1, error) {
	return ports.ReviewEvidenceApplicabilityPublishReceiptV1{}, nil
}
func (reviewEvidencePublisherShapeV1) InspectReviewEvidenceApplicabilityPublishV1(context.Context, string) (ports.ReviewEvidenceApplicabilityPublishReceiptV1, error) {
	return ports.ReviewEvidenceApplicabilityPublishReceiptV1{}, nil
}

func reviewEvidenceCurrentFixtureV1(t *testing.T, now time.Time, trust ports.EvidenceTrustClassV2) ports.ReviewEvidenceApplicabilityCurrentSnapshotV1 {
	t.Helper()
	projection, err := ports.SealReviewEvidenceApplicabilityProjectionV1(reviewEvidenceProjectionInputV1(t, now, trust))
	if err != nil {
		t.Fatal(err)
	}
	index, err := ports.SealReviewEvidenceApplicabilityCurrentIndexRefV1(ports.ReviewEvidenceApplicabilityCurrentIndexRefV1{
		Revision: 1, SubjectDigest: projection.SubjectDigest, CurrentProjection: projection.Ref, HighestRevision: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := ports.ReviewEvidenceApplicabilityCurrentSnapshotV1{ContractVersion: ports.ReviewEvidenceCurrentContractVersionV1, Projection: projection, CurrentIndex: index}
	if err := snapshot.Validate(); err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func reviewEvidenceProjectionInputV1(t *testing.T, now time.Time, trust ports.EvidenceTrustClassV2) ports.ReviewEvidenceApplicabilityProjectionV1 {
	t.Helper()
	scope := core.ExecutionScope{
		Identity: core.AgentIdentityRef{TenantID: "tenant-review", ID: "identity-review", Epoch: 3},
		Lineage:  core.LineageRef{ID: "lineage-review", PlanDigest: reviewEvidenceDigestV1("plan")},
		Instance: core.InstanceRef{ID: "instance-review", Epoch: 4}, AuthorityEpoch: 5,
	}
	actionScope := reviewEvidenceDigestV1("action-scope")
	candidateDigest := reviewEvidenceDigestV1("candidate")
	evidenceSubject := ports.EvidenceSubjectKeyV1{
		Record: ports.EvidenceRecordRefV2{LedgerScopeDigest: reviewEvidenceDigestV1("ledger"), Sequence: 7, RecordDigest: reviewEvidenceDigestV1("record")},
		Source: ports.EvidenceSourceKeyV2{RegistrationID: "review-source", SourceEpoch: 2, SourceSequence: 7},
	}
	subjectKeyDigest, err := ports.DigestEvidenceSubjectKeyV1(evidenceSubject)
	if err != nil {
		t.Fatal(err)
	}
	absence, err := ports.SealEvidenceTombstoneAbsenceRefV1(ports.EvidenceTombstoneAbsenceRefV1{SubjectKeyDigest: subjectKeyDigest, Revision: 1, OwnerWatermark: 1})
	if err != nil {
		t.Fatal(err)
	}
	evidenceProjection, err := ports.SealEvidenceSubjectCurrentProjectionV1(ports.EvidenceSubjectCurrentProjectionV1{
		Ref: ports.EvidenceSubjectProjectionRefV1{Revision: 1, OwnerWatermark: 1}, Subject: evidenceSubject,
		Causation: []ports.EvidenceCausationRefV2{}, Presence: ports.EvidenceTombstoneAbsentSealedV1,
		Readability: ports.EvidenceSubjectReadableV1, TombstoneAbsence: &absence,
		ExecutionScope: scope, LedgerScope: ports.EvidenceLedgerScopeV2{Partition: ports.EvidencePartitionRun, TenantID: "tenant-review", RunID: "run-review"},
		ActionScopeDigest: actionScope, TrustClass: trust, CustomClass: "review/evidence", CandidateDigest: candidateDigest,
		CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(30 * time.Second).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	evidenceIndex, err := ports.SealEvidenceSubjectCurrentIndexRefV1(ports.EvidenceSubjectCurrentIndexRefV1{Revision: 1, SubjectKeyDigest: subjectKeyDigest, CurrentProjection: evidenceProjection.Ref, OwnerWatermark: 1})
	if err != nil {
		t.Fatal(err)
	}
	evidenceSnapshot := ports.EvidenceSubjectCurrentSnapshotV1{ContractVersion: ports.EvidenceSubjectCurrentContractVersionV1, Projection: evidenceProjection, CurrentIndex: evidenceIndex}
	return ports.ReviewEvidenceApplicabilityProjectionV1{
		Ref: ports.ReviewEvidenceApplicabilityRefV1{Revision: 1},
		Subject: ports.ReviewEvidenceApplicabilitySubjectV1{
			TenantID: "tenant-review", Target: ports.ReviewEvidenceTargetRefV1{ID: "target-review", Revision: 8, Digest: reviewEvidenceDigestV1("target")},
			RunID: "run-review", Scope: scope, ActionScopeDigest: actionScope,
			ReviewEvidence: ports.ReviewEvidenceRefV2{Ref: "review-evidence-7", Classification: "review/evidence", Digest: candidateDigest},
		},
		EvidenceSubject: evidenceSubject, EvidenceSubjectProjection: evidenceProjection.Ref, EvidenceSubjectSnapshot: evidenceSnapshot,
		Record: evidenceSubject.Record, TrustClass: trust,
		CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(20 * time.Second).UnixNano(),
	}
}

func reviewEvidenceDigestV1(value string) core.Digest {
	return core.DigestBytes([]byte("review-evidence-current-v1:" + value))
}
