package decisioncurrent

import (
	"context"
	"sort"
	"sync"
	"testing"
	"time"

	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestExternalSourceV1EvidenceNExactSortedAndMinimumTTL(t *testing.T) {
	for _, count := range []int{1, 8} {
		t.Run(string(rune('0'+count)), func(t *testing.T) {
			fixture := newExternalSourceFixtureV1(t)
			checked := time.Unix(0, fixture.policy.value.CheckedUnixNano)
			refs := reviewEvidenceRefsV1(count)
			reader := newExternalEvidenceReaderV1(t, fixture.request, refs, checked)
			fixture.request.Evidence = append([]runtimeports.ReviewEvidenceRefV2(nil), refs...)
			fixture.source.evidence = reader
			got, err := fixture.source.InspectDecisionExternalCurrentV1(context.Background(), fixture.request)
			if err != nil {
				t.Fatal(err)
			}
			if len(got.Evidence) != count || got.ExpiresUnixNano != reader.minimumExpiry {
				t.Fatalf("Evidence cut cardinality or min TTL drifted: count=%d expiry=%d want=%d", len(got.Evidence), got.ExpiresUnixNano, reader.minimumExpiry)
			}
			wantRefs := make([]string, 0, count)
			for _, item := range refs {
				wantRefs = append(wantRefs, item.Ref)
			}
			sort.Strings(wantRefs)
			for index, item := range got.Evidence {
				if item.Review.Ref != wantRefs[index] || !item.Current || item.ApplicabilityRef.Validate() != nil || item.Record.Validate() != nil {
					t.Fatalf("Evidence[%d] is not the exact sorted Owner projection: %+v", index, item)
				}
			}
		})
	}
}

func TestExternalSourceV1EvidenceS1S2DriftFailsClosed(t *testing.T) {
	for _, drift := range []string{"index", "source"} {
		t.Run(drift, func(t *testing.T) {
			fixture := newExternalSourceFixtureV1(t)
			checked := time.Unix(0, fixture.policy.value.CheckedUnixNano)
			refs := reviewEvidenceRefsV1(1)
			reader := newExternalEvidenceReaderV1(t, fixture.request, refs, checked)
			reader.driftKind = drift
			reader.driftOnInspect = 2
			fixture.request.Evidence = refs
			fixture.source.evidence = reader
			got, err := fixture.source.InspectDecisionExternalCurrentV1(context.Background(), fixture.request)
			if err == nil || len(got.Evidence) != 0 || got.Current {
				t.Fatalf("Evidence %s drift reached a current cut: value=%+v err=%v", drift, got, err)
			}
		})
	}
}

func TestExternalSourceV1EvidenceExactInspectLostReplyCompletesOriginalCut(t *testing.T) {
	fixture := newExternalSourceFixtureV1(t)
	checked := time.Unix(0, fixture.policy.value.CheckedUnixNano)
	refs := reviewEvidenceRefsV1(8)
	reader := newExternalEvidenceReaderV1(t, fixture.request, refs, checked)
	ctx, cancel := context.WithCancel(context.Background())
	reader.loseRef = reader.order[3]
	reader.cancel = cancel
	fixture.request.Evidence = refs
	fixture.source.evidence = reader
	got, err := fixture.source.InspectDecisionExternalCurrentV1(ctx, fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	if ctx.Err() != context.Canceled || len(got.Evidence) != 8 {
		t.Fatalf("lost exact Inspect did not complete the same immutable cut: ctx=%v count=%d", ctx.Err(), len(got.Evidence))
	}
	reader.mu.Lock()
	defer reader.mu.Unlock()
	for _, ref := range reader.order {
		if reader.inspectCalls[ref] < 2 {
			t.Fatalf("same cut did not continue through exact Evidence ref %s: calls=%d", ref.ProjectionID, reader.inspectCalls[ref])
		}
	}
	if reader.inspectCalls[reader.loseRef] < 3 {
		t.Fatalf("lost exact Evidence ref was not retried by Inspect: calls=%d", reader.inspectCalls[reader.loseRef])
	}
}

func TestExternalSourceV1EvidenceTTLCrossingAndClockRollbackFailClosed(t *testing.T) {
	for _, tc := range []struct {
		name   string
		final  func(time.Time) time.Time
		reason core.ReasonCode
	}{
		{name: "ttl_crossing", final: func(expires time.Time) time.Time { return expires }, reason: core.ReasonEvidenceSourceStale},
		{name: "clock_rollback", final: func(expires time.Time) time.Time { return expires.Add(-2 * time.Second) }, reason: core.ReasonClockRegression},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newExternalSourceFixtureV1(t)
			checked := time.Unix(0, fixture.policy.value.CheckedUnixNano)
			stable := fixture.now.Now()
			expires := stable.Add(time.Second)
			refs := reviewEvidenceRefsV1(1)
			reader := newExternalEvidenceReaderWithExpiryV1(t, fixture.request, refs, checked, expires)
			fixture.request.Evidence = refs
			fixture.source.evidence = reader
			fixture.source.clock = externalEvidenceFinalClockV1(39, stable, tc.final(expires))
			got, err := fixture.source.InspectDecisionExternalCurrentV1(context.Background(), fixture.request)
			if !core.HasReason(err, tc.reason) || got.Current || len(got.Evidence) != 0 {
				t.Fatalf("%s reached a current Evidence cut: value=%+v err=%v", tc.name, got, err)
			}
		})
	}
}

type externalEvidenceReaderV1 struct {
	mu             sync.Mutex
	bySubject      map[core.Digest]runtimeports.ReviewEvidenceApplicabilityCurrentSnapshotV1
	byRef          map[runtimeports.ReviewEvidenceApplicabilityRefV1]runtimeports.ReviewEvidenceApplicabilityCurrentSnapshotV1
	order          []runtimeports.ReviewEvidenceApplicabilityRefV1
	inspectCalls   map[runtimeports.ReviewEvidenceApplicabilityRefV1]int
	minimumExpiry  int64
	driftKind      string
	driftOnInspect int
	loseRef        runtimeports.ReviewEvidenceApplicabilityRefV1
	cancel         context.CancelFunc
}

func newExternalEvidenceReaderV1(t *testing.T, request reviewport.DecisionExternalCurrentRequestV1, refs []runtimeports.ReviewEvidenceRefV2, checked time.Time) *externalEvidenceReaderV1 {
	t.Helper()
	return newExternalEvidenceReaderWithExpiryV1(t, request, refs, checked, checked.Add(20*time.Second))
}

func newExternalEvidenceReaderWithExpiryV1(t *testing.T, request reviewport.DecisionExternalCurrentRequestV1, refs []runtimeports.ReviewEvidenceRefV2, checked, firstExpiry time.Time) *externalEvidenceReaderV1 {
	t.Helper()
	reader := &externalEvidenceReaderV1{
		bySubject:     make(map[core.Digest]runtimeports.ReviewEvidenceApplicabilityCurrentSnapshotV1),
		byRef:         make(map[runtimeports.ReviewEvidenceApplicabilityRefV1]runtimeports.ReviewEvidenceApplicabilityCurrentSnapshotV1),
		inspectCalls:  make(map[runtimeports.ReviewEvidenceApplicabilityRefV1]int),
		minimumExpiry: firstExpiry.UnixNano(),
	}
	for index, reviewRef := range refs {
		expires := firstExpiry.Add(time.Duration(index) * time.Second)
		snapshot := sealExternalEvidenceSnapshotV1(t, request, reviewRef, index+1, checked, expires)
		subjectDigest, err := runtimeports.DigestReviewEvidenceApplicabilitySubjectV1(snapshot.Projection.Subject)
		if err != nil {
			t.Fatal(err)
		}
		reader.bySubject[subjectDigest] = snapshot
		reader.byRef[snapshot.Projection.Ref] = snapshot
		reader.order = append(reader.order, snapshot.Projection.Ref)
		if expires.UnixNano() < reader.minimumExpiry {
			reader.minimumExpiry = expires.UnixNano()
		}
	}
	return reader
}

func sealExternalEvidenceSnapshotV1(t *testing.T, request reviewport.DecisionExternalCurrentRequestV1, reviewRef runtimeports.ReviewEvidenceRefV2, sequence int, checked, expires time.Time) runtimeports.ReviewEvidenceApplicabilityCurrentSnapshotV1 {
	t.Helper()
	ledger := runtimeports.EvidenceLedgerScopeV2{
		Partition: runtimeports.EvidencePartitionRun, TenantID: request.Target.TenantID,
		IdentityID: request.Target.Scope.Identity.ID, LineageID: request.Target.Scope.Lineage.ID,
		InstanceID: request.Target.Scope.Instance.ID, RunID: request.Target.RunID,
	}
	ledgerDigest, err := ledger.DigestV2()
	if err != nil {
		t.Fatal(err)
	}
	record := runtimeports.EvidenceRecordRefV2{LedgerScopeDigest: ledgerDigest, Sequence: uint64(sequence), RecordDigest: externalDigestV1("evidence-record-" + reviewRef.Ref)}
	evidenceSubject := runtimeports.EvidenceSubjectKeyV1{Record: record, Source: runtimeports.EvidenceSourceKeyV2{RegistrationID: "review-evidence-source", SourceEpoch: 1, SourceSequence: uint64(sequence)}}
	subjectKeyDigest, err := runtimeports.DigestEvidenceSubjectKeyV1(evidenceSubject)
	if err != nil {
		t.Fatal(err)
	}
	absence, err := runtimeports.SealEvidenceTombstoneAbsenceRefV1(runtimeports.EvidenceTombstoneAbsenceRefV1{SubjectKeyDigest: subjectKeyDigest, Revision: 1, OwnerWatermark: 1})
	if err != nil {
		t.Fatal(err)
	}
	evidenceProjection, err := runtimeports.SealEvidenceSubjectCurrentProjectionV1(runtimeports.EvidenceSubjectCurrentProjectionV1{
		Ref: runtimeports.EvidenceSubjectProjectionRefV1{Revision: 1, OwnerWatermark: 1}, Subject: evidenceSubject,
		Causation: []runtimeports.EvidenceCausationRefV2{}, Presence: runtimeports.EvidenceTombstoneAbsentSealedV1,
		Readability: runtimeports.EvidenceSubjectReadableV1, TombstoneAbsence: &absence,
		ExecutionScope: request.Target.Scope, LedgerScope: ledger, ActionScopeDigest: request.Target.ActionScopeDigest,
		TrustClass: runtimeports.EvidenceTrustObservation, CustomClass: reviewRef.Classification, CandidateDigest: reviewRef.Digest,
		CheckedUnixNano: checked.UnixNano(), ExpiresUnixNano: expires.UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	evidenceIndex, err := runtimeports.SealEvidenceSubjectCurrentIndexRefV1(runtimeports.EvidenceSubjectCurrentIndexRefV1{Revision: 1, SubjectKeyDigest: subjectKeyDigest, CurrentProjection: evidenceProjection.Ref, OwnerWatermark: 1})
	if err != nil {
		t.Fatal(err)
	}
	evidenceSnapshot := runtimeports.EvidenceSubjectCurrentSnapshotV1{ContractVersion: runtimeports.EvidenceSubjectCurrentContractVersionV1, Projection: evidenceProjection, CurrentIndex: evidenceIndex}
	projection, err := runtimeports.SealReviewEvidenceApplicabilityProjectionV1(runtimeports.ReviewEvidenceApplicabilityProjectionV1{
		Ref: runtimeports.ReviewEvidenceApplicabilityRefV1{Revision: 1},
		Subject: runtimeports.ReviewEvidenceApplicabilitySubjectV1{
			TenantID: request.Target.TenantID,
			Target:   runtimeports.ReviewEvidenceTargetRefV1{ID: request.Target.ID, Revision: request.Target.Revision, Digest: request.Target.Digest},
			RunID:    request.Target.RunID, Scope: request.Target.Scope, ActionScopeDigest: request.Target.ActionScopeDigest, ReviewEvidence: reviewRef,
		},
		EvidenceSubject: evidenceSubject, EvidenceSubjectProjection: evidenceProjection.Ref, EvidenceSubjectSnapshot: evidenceSnapshot,
		Record: record, TrustClass: runtimeports.EvidenceTrustObservation, CheckedUnixNano: checked.UnixNano(), ExpiresUnixNano: expires.UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	index, err := runtimeports.SealReviewEvidenceApplicabilityCurrentIndexRefV1(runtimeports.ReviewEvidenceApplicabilityCurrentIndexRefV1{Revision: 1, SubjectDigest: projection.SubjectDigest, CurrentProjection: projection.Ref, HighestRevision: 1})
	if err != nil {
		t.Fatal(err)
	}
	return runtimeports.ReviewEvidenceApplicabilityCurrentSnapshotV1{ContractVersion: runtimeports.ReviewEvidenceCurrentContractVersionV1, Projection: projection, CurrentIndex: index}
}

func (r *externalEvidenceReaderV1) ResolveReviewEvidenceApplicabilityCurrentV1(ctx context.Context, request runtimeports.ResolveReviewEvidenceApplicabilityCurrentRequestV1) (runtimeports.ReviewEvidenceApplicabilityCurrentSnapshotV1, error) {
	if err := ctx.Err(); err != nil {
		return runtimeports.ReviewEvidenceApplicabilityCurrentSnapshotV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonInspectCoverageIncomplete, "Evidence Resolve context ended")
	}
	digest, err := runtimeports.DigestReviewEvidenceApplicabilitySubjectV1(request.Subject)
	if err != nil {
		return runtimeports.ReviewEvidenceApplicabilityCurrentSnapshotV1{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	value, ok := r.bySubject[digest]
	if !ok {
		return runtimeports.ReviewEvidenceApplicabilityCurrentSnapshotV1{}, core.NewError(core.ErrorNotFound, core.ReasonEvidenceSourceMissing, "Evidence subject is absent")
	}
	return runtimeports.CloneReviewEvidenceApplicabilityCurrentSnapshotV1(value), nil
}

func (r *externalEvidenceReaderV1) InspectCurrentReviewEvidenceApplicabilityV1(ctx context.Context, ref runtimeports.ReviewEvidenceApplicabilityRefV1) (runtimeports.ReviewEvidenceApplicabilityCurrentSnapshotV1, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.inspectCalls[ref]++
	call := r.inspectCalls[ref]
	if ref == r.loseRef && call == 1 {
		if r.cancel != nil {
			r.cancel()
		}
		return runtimeports.ReviewEvidenceApplicabilityCurrentSnapshotV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonInspectCoverageIncomplete, "Evidence exact Inspect reply was lost")
	}
	if err := ctx.Err(); err != nil {
		return runtimeports.ReviewEvidenceApplicabilityCurrentSnapshotV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonInspectCoverageIncomplete, "Evidence exact Inspect context ended")
	}
	value, ok := r.byRef[ref]
	if !ok {
		return runtimeports.ReviewEvidenceApplicabilityCurrentSnapshotV1{}, core.NewError(core.ErrorNotFound, core.ReasonEvidenceSourceMissing, "Evidence exact ref is absent")
	}
	if r.driftOnInspect > 0 && call >= r.driftOnInspect {
		switch r.driftKind {
		case "index":
			value.CurrentIndex.Digest = core.DigestBytes([]byte("external-evidence-index-drift"))
		case "source":
			if len(r.order) == 1 {
				value.Projection.Subject.ReviewEvidence.Ref += "-drift"
			}
		}
	}
	return runtimeports.CloneReviewEvidenceApplicabilityCurrentSnapshotV1(value), nil
}

func (r *externalEvidenceReaderV1) InspectHistoricalReviewEvidenceApplicabilityV1(_ context.Context, ref runtimeports.ReviewEvidenceApplicabilityRefV1) (runtimeports.ReviewEvidenceApplicabilityProjectionV1, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	value, ok := r.byRef[ref]
	if !ok {
		return runtimeports.ReviewEvidenceApplicabilityProjectionV1{}, core.NewError(core.ErrorNotFound, core.ReasonEvidenceSourceMissing, "Evidence historical ref is absent")
	}
	return runtimeports.CloneReviewEvidenceApplicabilityProjectionV1(value.Projection), nil
}

func reviewEvidenceRefsV1(count int) []runtimeports.ReviewEvidenceRefV2 {
	values := make([]runtimeports.ReviewEvidenceRefV2, 0, count)
	for index := count; index > 0; index-- {
		values = append(values, runtimeports.ReviewEvidenceRefV2{Ref: "evidence-" + string(rune('a'+index-1)), Classification: "review/evidence", Digest: externalDigestV1("evidence-candidate-" + string(rune('a'+index-1)))})
	}
	return values
}

func externalEvidenceFinalClockV1(finalCall int, stable, final time.Time) func() time.Time {
	var mu sync.Mutex
	calls := 0
	return func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		calls++
		if calls >= finalCall {
			return final
		}
		return stable
	}
}
