package sqlite

import (
	"context"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestSQLiteEvidenceSubjectAtomicHistoryRestartAndLostReply(t *testing.T) {
	path := testDBPath(t)
	now := time.Unix(2_310_000_000, 0)
	store := openTestStore(t, path, func() time.Time { return now })
	firstCommit, firstProjection, firstIndex := sqliteEvidenceSubjectBundleV1(t, now, nil, nil)
	if _, err := store.PublishEvidenceSubjectMutationV1(context.Background(), firstCommit, firstProjection, firstIndex); err != nil {
		t.Fatal(err)
	}
	secondCommit, secondProjection, secondIndex := sqliteEvidenceSubjectBundleV1(t, now.Add(time.Second), &firstIndex, &firstProjection.Ref)
	store.loseNextReplyForTest()
	if _, err := store.PublishEvidenceSubjectMutationV1(context.Background(), secondCommit, secondProjection, secondIndex); !core.HasCategory(err, core.ErrorIndeterminate) {
		t.Fatalf("lost SQLite commit reply must be indeterminate: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	recovered, err := store.InspectEvidenceSubjectMutationV1(context.WithoutCancel(ctx), secondCommit.Key)
	if err != nil || !reflect.DeepEqual(recovered, secondCommit) {
		t.Fatalf("lost reply did not recover exact Mutation Commit: %+v %v", recovered, err)
	}
	historical, err := store.InspectEvidenceSubjectProjectionFactV1(context.Background(), firstProjection.Ref)
	if err != nil || !reflect.DeepEqual(historical, firstProjection) {
		t.Fatalf("current advance rewrote Evidence history: %+v %v", historical, err)
	}
	current, err := store.InspectEvidenceSubjectCurrentIndexV1(context.Background(), firstIndex.SubjectKeyDigest)
	if err != nil || !reflect.DeepEqual(current, secondIndex) {
		t.Fatalf("Evidence current did not advance exactly: %+v %v", current, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := openTestStore(t, path, func() time.Time { return now.Add(2 * time.Second) })
	current, err = reopened.InspectEvidenceSubjectCurrentIndexV1(context.Background(), firstIndex.SubjectKeyDigest)
	if err != nil || !reflect.DeepEqual(current, secondIndex) {
		t.Fatalf("restart lost Evidence current: %+v %v", current, err)
	}
}

func TestSQLiteEvidenceSubjectStagedFailureLeaksNothingAndConcurrentReplayOneCommit(t *testing.T) {
	now := time.Unix(2_310_000_100, 0)
	store := openTestStore(t, testDBPath(t), func() time.Time { return now })
	commit, projection, index := sqliteEvidenceSubjectBundleV1(t, now, nil, nil)
	store.failNextStageForTest()
	if _, err := store.PublishEvidenceSubjectMutationV1(context.Background(), commit, projection, index); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("expected staged failure: %v", err)
	}
	if _, err := store.InspectEvidenceSubjectProjectionFactV1(context.Background(), projection.Ref); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("staged failure leaked history: %v", err)
	}
	if _, err := store.InspectEvidenceSubjectCurrentIndexV1(context.Background(), index.SubjectKeyDigest); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("staged failure leaked current: %v", err)
	}
	results := make(chan error, 64)
	var wait sync.WaitGroup
	for range 64 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			got, err := store.PublishEvidenceSubjectMutationV1(context.Background(), commit, projection, index)
			if err == nil && !reflect.DeepEqual(got, commit) {
				err = core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "concurrent Evidence replay returned another Commit")
			}
			results <- err
		}()
	}
	wait.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestSQLiteReviewEvidenceAtomicCASCanonicalReplayAndLostReply(t *testing.T) {
	path := testDBPath(t)
	now := time.Unix(2_310_000_200, 0)
	store := openTestStore(t, path, func() time.Time { return now })
	_, evidenceProjection, evidenceIndex := sqliteEvidenceSubjectBundleV1(t, now, nil, nil)
	evidenceSnapshot := ports.EvidenceSubjectCurrentSnapshotV1{ContractVersion: ports.EvidenceSubjectCurrentContractVersionV1, Projection: evidenceProjection, CurrentIndex: evidenceIndex}
	firstRequest, firstReceipt := sqliteReviewEvidencePublishV1(t, evidenceSnapshot, now, nil, nil)
	first, err := store.PublishReviewEvidenceApplicabilityFactV1(context.Background(), firstRequest, firstReceipt)
	if err != nil {
		t.Fatal(err)
	}
	changedCommitTime, err := ports.SealReviewEvidenceApplicabilityPublishReceiptV1(ports.ReviewEvidenceApplicabilityPublishReceiptV1{RequestDigest: firstRequest.RequestDigest, Projection: firstRequest.Projection.Ref, CurrentIndex: firstRequest.NextCurrentIndex, CommittedUnixNano: now.Add(time.Second).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := store.PublishReviewEvidenceApplicabilityFactV1(context.Background(), firstRequest, changedCommitTime)
	if err != nil || !reflect.DeepEqual(replayed, first) {
		t.Fatalf("canonical replay rewrote first receipt: %+v %v", replayed, err)
	}
	secondRequest, secondReceipt := sqliteReviewEvidencePublishV1(t, evidenceSnapshot, now.Add(time.Second), &firstRequest.NextCurrentIndex, &firstRequest.Projection.Ref)
	store.loseNextReplyForTest()
	if _, err = store.PublishReviewEvidenceApplicabilityFactV1(context.Background(), secondRequest, secondReceipt); !core.HasCategory(err, core.ErrorIndeterminate) {
		t.Fatalf("lost Review evidence reply must be indeterminate: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	recovered, err := store.InspectReviewEvidenceApplicabilityPublishFactV1(context.WithoutCancel(ctx), secondReceipt.PublishID)
	if err != nil || !reflect.DeepEqual(recovered, secondReceipt) {
		t.Fatalf("lost Review evidence reply did not exact-Inspect: %+v %v", recovered, err)
	}
	historical, err := store.InspectReviewEvidenceApplicabilityProjectionFactV1(context.Background(), firstRequest.Projection.Ref)
	if err != nil || !reflect.DeepEqual(historical, firstRequest.Projection) {
		t.Fatalf("Review evidence CAS rewrote history: %+v %v", historical, err)
	}
	current, err := store.InspectReviewEvidenceApplicabilityCurrentFactV1(context.Background(), firstRequest.Projection.SubjectDigest)
	if err != nil || current.Projection.Ref != secondRequest.Projection.Ref || !reflect.DeepEqual(current.CurrentIndex, secondRequest.NextCurrentIndex) {
		t.Fatalf("Review evidence atomic current snapshot drifted: %+v %v", current, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := openTestStore(t, path, func() time.Time { return now.Add(2 * time.Second) })
	current, err = reopened.InspectReviewEvidenceApplicabilityCurrentFactV1(context.Background(), firstRequest.Projection.SubjectDigest)
	if err != nil || current.Projection.Ref != secondRequest.Projection.Ref {
		t.Fatalf("restart lost Review evidence current: %+v %v", current, err)
	}
}

func TestSQLiteReviewEvidenceStagedFailureAndConcurrentCanonicalPublish(t *testing.T) {
	now := time.Unix(2_310_000_300, 0)
	store := openTestStore(t, testDBPath(t), func() time.Time { return now })
	_, evidenceProjection, evidenceIndex := sqliteEvidenceSubjectBundleV1(t, now, nil, nil)
	evidenceSnapshot := ports.EvidenceSubjectCurrentSnapshotV1{ContractVersion: ports.EvidenceSubjectCurrentContractVersionV1, Projection: evidenceProjection, CurrentIndex: evidenceIndex}
	request, receipt := sqliteReviewEvidencePublishV1(t, evidenceSnapshot, now, nil, nil)
	store.failNextStageForTest()
	if _, err := store.PublishReviewEvidenceApplicabilityFactV1(context.Background(), request, receipt); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("expected staged Review evidence failure: %v", err)
	}
	if _, err := store.InspectReviewEvidenceApplicabilityProjectionFactV1(context.Background(), request.Projection.Ref); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("staged failure leaked Review history: %v", err)
	}
	if _, err := store.InspectReviewEvidenceApplicabilityCurrentFactV1(context.Background(), request.Projection.SubjectDigest); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("staged failure leaked Review current: %v", err)
	}
	results := make(chan error, 64)
	var wait sync.WaitGroup
	for range 64 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			got, err := store.PublishReviewEvidenceApplicabilityFactV1(context.Background(), request, receipt)
			if err == nil && !reflect.DeepEqual(got, receipt) {
				err = core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "concurrent Review evidence replay returned another receipt")
			}
			results <- err
		}()
	}
	wait.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestSQLiteEvidencePublicOwnerInterfaces(t *testing.T) {
	var _ ports.EvidenceSubjectCurrentFactPortV1 = (*Store)(nil)
	var _ control.ReviewEvidenceApplicabilityFactPortV1 = (*Store)(nil)
}

func sqliteEvidenceSubjectBundleV1(t *testing.T, now time.Time, previousIndex *ports.EvidenceSubjectCurrentIndexRefV1, previousProjection *ports.EvidenceSubjectProjectionRefV1) (ports.EvidenceSubjectMutationCommitV1, ports.EvidenceSubjectCurrentProjectionV1, ports.EvidenceSubjectCurrentIndexRefV1) {
	t.Helper()
	scope := core.ExecutionScope{Identity: core.AgentIdentityRef{TenantID: "tenant-sqlite-evidence", ID: "identity-sqlite-evidence", Epoch: 1}, Lineage: core.LineageRef{ID: "lineage-sqlite-evidence", PlanDigest: testDigest(t, "plan-sqlite-evidence")}, Instance: core.InstanceRef{ID: "instance-sqlite-evidence", Epoch: 1}, AuthorityEpoch: 1}
	ledger := ports.EvidenceLedgerScopeV2{Partition: ports.EvidencePartitionRun, TenantID: scope.Identity.TenantID, IdentityID: scope.Identity.ID, LineageID: scope.Lineage.ID, InstanceID: scope.Instance.ID, RunID: "run-sqlite-evidence"}
	ledgerDigest, err := ledger.DigestV2()
	if err != nil {
		t.Fatal(err)
	}
	record := ports.EvidenceRecordRefV2{LedgerScopeDigest: ledgerDigest, Sequence: 1, RecordDigest: testDigest(t, "record-sqlite-evidence")}
	subject := ports.EvidenceSubjectKeyV1{Record: record, Source: ports.EvidenceSourceKeyV2{RegistrationID: "source-sqlite-evidence", SourceEpoch: 1, SourceSequence: 1}}
	subjectDigest, err := ports.DigestEvidenceSubjectKeyV1(subject)
	if err != nil {
		t.Fatal(err)
	}
	absence, err := ports.SealEvidenceTombstoneAbsenceRefV1(ports.EvidenceTombstoneAbsenceRefV1{SubjectKeyDigest: subjectDigest, Revision: 1, OwnerWatermark: 1})
	if err != nil {
		t.Fatal(err)
	}
	watermark := core.Revision(1)
	if previousIndex != nil {
		watermark = previousIndex.OwnerWatermark + 1
	}
	projection := ports.EvidenceSubjectCurrentProjectionV1{Ref: ports.EvidenceSubjectProjectionRefV1{OwnerWatermark: watermark}, Subject: subject, Causation: []ports.EvidenceCausationRefV2{}, Presence: ports.EvidenceTombstoneAbsentSealedV1, Readability: ports.EvidenceSubjectReadableV1, TombstoneAbsence: &absence, ExecutionScope: scope, LedgerScope: ledger, ActionScopeDigest: testDigest(t, "action-sqlite-evidence"), TrustClass: ports.EvidenceTrustObservation, CustomClass: "review/evidence", CandidateDigest: testDigest(t, "candidate-sqlite-evidence"), CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Minute).UnixNano()}
	registration := ports.EvidenceSourceRegistrationRefV1{RegistrationID: subject.Source.RegistrationID, Revision: 1, FactDigest: testDigest(t, "registration-fact"), ConfigurationDigest: testDigest(t, "registration-config"), SourceID: "custom/sqlite-evidence", SourceEpoch: subject.Source.SourceEpoch}
	request, err := ports.SealEvidenceSubjectMutationRequestV1(ports.EvidenceSubjectMutationRequestV1{ContractVersion: ports.EvidenceSubjectCurrentContractVersionV1, Subject: subject, Kind: ports.EvidenceSubjectMutationSourceRegistrationAdvanceV1, ExpectedCurrentIndex: previousIndex, ExpectedCurrentProjection: previousProjection, Registration: &registration})
	if err != nil {
		t.Fatal(err)
	}
	commit, projection, index, err := control.NewEvidenceSubjectMutationBundleV1(request, projection, now)
	if err != nil {
		t.Fatal(err)
	}
	return commit, projection, index
}

func sqliteReviewEvidencePublishV1(t *testing.T, evidence ports.EvidenceSubjectCurrentSnapshotV1, now time.Time, previousIndex *ports.ReviewEvidenceApplicabilityCurrentIndexRefV1, previousProjection *ports.ReviewEvidenceApplicabilityRefV1) (ports.PublishReviewEvidenceApplicabilityRequestV1, ports.ReviewEvidenceApplicabilityPublishReceiptV1) {
	t.Helper()
	revision := core.Revision(1)
	if previousIndex != nil {
		revision = previousIndex.Revision + 1
	}
	projection, err := ports.SealReviewEvidenceApplicabilityProjectionV1(ports.ReviewEvidenceApplicabilityProjectionV1{Ref: ports.ReviewEvidenceApplicabilityRefV1{Revision: revision}, Previous: previousProjection, Subject: ports.ReviewEvidenceApplicabilitySubjectV1{TenantID: evidence.Projection.LedgerScope.TenantID, Target: ports.ReviewEvidenceTargetRefV1{ID: "target-sqlite-evidence", Revision: 1, Digest: testDigest(t, "target-sqlite-evidence")}, RunID: evidence.Projection.LedgerScope.RunID, Scope: evidence.Projection.ExecutionScope, ActionScopeDigest: evidence.Projection.ActionScopeDigest, ReviewEvidence: ports.ReviewEvidenceRefV2{Ref: "review-evidence-sqlite", Classification: evidence.Projection.CustomClass, Digest: evidence.Projection.CandidateDigest}}, EvidenceSubject: evidence.Projection.Subject, EvidenceSubjectProjection: evidence.Projection.Ref, EvidenceSubjectSnapshot: evidence, Record: evidence.Projection.Record, TrustClass: evidence.Projection.TrustClass, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(20 * time.Second).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	index, err := ports.SealReviewEvidenceApplicabilityCurrentIndexRefV1(ports.ReviewEvidenceApplicabilityCurrentIndexRefV1{Revision: revision, SubjectDigest: projection.SubjectDigest, Previous: previousProjection, CurrentProjection: projection.Ref, HighestRevision: revision})
	if err != nil {
		t.Fatal(err)
	}
	request, err := ports.SealPublishReviewEvidenceApplicabilityRequestV1(ports.PublishReviewEvidenceApplicabilityRequestV1{Projection: projection, ExpectedCurrentIndex: previousIndex, NextCurrentIndex: index})
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := ports.SealReviewEvidenceApplicabilityPublishReceiptV1(ports.ReviewEvidenceApplicabilityPublishReceiptV1{RequestDigest: request.RequestDigest, Projection: projection.Ref, CurrentIndex: index, CommittedUnixNano: now.UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	return request, receipt
}
