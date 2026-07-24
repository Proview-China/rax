package fakes_test

import (
	"context"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/fakes"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestEvidenceSubjectCurrentV1AtomicLostReplyAndNoHalfWrite(t *testing.T) {
	now := time.Unix(2_200_000_100, 0)
	store := fakes.NewEvidenceLedgerStoreV2(func() time.Time { return now })
	commit, projection, index := evidenceSubjectAtomicBundleV1(t, now, "lost")
	store.FailNextEvidenceSubjectPublishAtV1(2)
	if _, err := store.PublishEvidenceSubjectMutationV1(context.Background(), commit, projection, index); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("staged failure must be unavailable: %v", err)
	}
	if _, err := store.InspectEvidenceSubjectProjectionFactV1(context.Background(), projection.Ref); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("staged failure leaked Projection: %v", err)
	}
	if _, err := store.InspectEvidenceSubjectCurrentIndexV1(context.Background(), projection.SubjectKeyDigest); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("staged failure leaked Current Index: %v", err)
	}
	if _, err := store.InspectEvidenceSubjectMutationV1(context.Background(), commit.Key); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("staged failure leaked Commit: %v", err)
	}

	store.LoseNextEvidenceSubjectReplyV1()
	if _, err := store.PublishEvidenceSubjectMutationV1(context.Background(), commit, projection, index); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("lost reply must be unavailable after commit: %v", err)
	}
	stored, err := store.InspectEvidenceSubjectMutationV1(context.Background(), commit.Key)
	if err != nil || stored.CommitDigest != commit.CommitDigest {
		t.Fatalf("lost reply must recover exact Commit: %v %+v", err, stored)
	}
	if _, err = store.PublishEvidenceSubjectMutationV1(context.Background(), commit, projection, index); err != nil {
		t.Fatalf("same canonical replay must be idempotent: %v", err)
	}
}

func TestEvidenceSubjectCurrentV1Concurrent64SingleLinearization(t *testing.T) {
	now := time.Unix(2_200_000_200, 0)
	store := fakes.NewEvidenceLedgerStoreV2(func() time.Time { return now })
	commit, projection, index := evidenceSubjectAtomicBundleV1(t, now, "concurrent")
	var wait sync.WaitGroup
	errors := make(chan error, 64)
	for range 64 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, err := store.PublishEvidenceSubjectMutationV1(context.Background(), commit, projection, index)
			errors <- err
		}()
	}
	wait.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatalf("same immutable concurrent publish failed: %v", err)
		}
	}
	current, err := store.InspectEvidenceSubjectCurrentIndexV1(context.Background(), projection.SubjectKeyDigest)
	if err != nil || current.Revision != 1 || current != index {
		t.Fatalf("concurrency advanced more than once: %v %+v", err, current)
	}
}

func TestEvidenceSubjectCurrentV1HistoryProgressesWithoutABA(t *testing.T) {
	now := time.Unix(2_200_000_300, 0)
	store := fakes.NewEvidenceLedgerStoreV2(func() time.Time { return now })
	firstCommit, firstProjection, firstIndex := evidenceSubjectAtomicBundleV1(t, now, "history")
	if _, err := store.PublishEvidenceSubjectMutationV1(context.Background(), firstCommit, firstProjection, firstIndex); err != nil {
		t.Fatal(err)
	}
	registration := firstCommit.Request.Registration
	request, err := ports.SealEvidenceSubjectMutationRequestV1(ports.EvidenceSubjectMutationRequestV1{ContractVersion: ports.EvidenceSubjectCurrentContractVersionV1, Subject: firstProjection.Subject, Kind: ports.EvidenceSubjectMutationSourceRegistrationAdvanceV1, ExpectedCurrentIndex: &firstIndex, ExpectedCurrentProjection: &firstProjection.Ref, Registration: registration})
	if err != nil {
		t.Fatal(err)
	}
	nextInput := firstProjection
	nextInput.Ref.Digest, nextInput.ProjectionDigest = "", ""
	nextInput.Ref.OwnerWatermark = 2
	nextInput.CheckedUnixNano = now.Add(time.Second).UnixNano()
	nextInput.ExpiresUnixNano = now.Add(time.Minute).UnixNano()
	nextCommit, nextProjection, nextIndex, err := control.NewEvidenceSubjectMutationBundleV1(request, nextInput, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.PublishEvidenceSubjectMutationV1(context.Background(), nextCommit, nextProjection, nextIndex); err != nil {
		t.Fatal(err)
	}
	if err = control.ValidateEvidenceSubjectProgressionV1(firstIndex, nextIndex); err != nil {
		t.Fatal(err)
	}
	historical, err := store.InspectEvidenceSubjectProjectionFactV1(context.Background(), firstProjection.Ref)
	if err != nil || historical.ProjectionDigest != firstProjection.ProjectionDigest {
		t.Fatalf("old Projection must remain immutable historical: %v %+v", err, historical)
	}
	current, err := store.InspectEvidenceSubjectCurrentIndexV1(context.Background(), firstProjection.SubjectKeyDigest)
	if err != nil || !reflect.DeepEqual(current, nextIndex) {
		t.Fatalf("Current Index did not advance exactly once: %v %+v", err, current)
	}
	if _, err = store.PublishEvidenceSubjectMutationV1(context.Background(), firstCommit, firstProjection, firstIndex); err != nil {
		t.Fatalf("immutable historical replay must inspect-idempotently succeed: %v", err)
	}
}

func TestEvidenceSubjectCurrentV1ConcurrentChangedContentOnlyOneWins(t *testing.T) {
	now := time.Unix(2_200_000_400, 0)
	store := fakes.NewEvidenceLedgerStoreV2(func() time.Time { return now })
	baseCommit, baseProjection, _ := evidenceSubjectAtomicBundleV1(t, now, "changed")
	results := make(chan error, 64)
	var wait sync.WaitGroup
	for index := range 64 {
		request := baseCommit.Request
		registration := *request.Registration
		registration.FactDigest = core.DigestBytes([]byte("registration-content-" + string(rune(index+1))))
		request.Registration = &registration
		request.RequestDigest = ""
		request, err := ports.SealEvidenceSubjectMutationRequestV1(request)
		if err != nil {
			t.Fatal(err)
		}
		projection := baseProjection
		projection.Ref.Digest, projection.ProjectionDigest = "", ""
		projection.Registration = registration
		commit, sealed, current, err := control.NewEvidenceSubjectMutationBundleV1(request, projection, now)
		if err != nil {
			t.Fatal(err)
		}
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, publishErr := store.PublishEvidenceSubjectMutationV1(context.Background(), commit, sealed, current)
			results <- publishErr
		}()
	}
	wait.Wait()
	close(results)
	successes, conflicts := 0, 0
	for err := range results {
		switch {
		case err == nil:
			successes++
		case core.HasCategory(err, core.ErrorConflict):
			conflicts++
		default:
			t.Fatalf("unexpected changed-content result: %v", err)
		}
	}
	if successes != 1 || conflicts != 63 {
		t.Fatalf("changed-content concurrency did not linearize once: successes=%d conflicts=%d", successes, conflicts)
	}
}

func evidenceSubjectAtomicBundleV1(t *testing.T, now time.Time, suffix string) (ports.EvidenceSubjectMutationCommitV1, ports.EvidenceSubjectCurrentProjectionV1, ports.EvidenceSubjectCurrentIndexRefV1) {
	t.Helper()
	digest := func(label string) core.Digest {
		return core.Digest("sha256:" + string(core.DigestBytes([]byte(suffix + label)))[7:])
	}
	subject := ports.EvidenceSubjectKeyV1{Record: ports.EvidenceRecordRefV2{LedgerScopeDigest: digest("scope"), Sequence: 1, RecordDigest: digest("record")}, Source: ports.EvidenceSourceKeyV2{RegistrationID: "registration-" + suffix, SourceEpoch: 1, SourceSequence: 1}}
	subjectDigest, _ := ports.DigestEvidenceSubjectKeyV1(subject)
	absence, err := ports.SealEvidenceTombstoneAbsenceRefV1(ports.EvidenceTombstoneAbsenceRefV1{SubjectKeyDigest: subjectDigest, Revision: 1, OwnerWatermark: 1})
	if err != nil {
		t.Fatal(err)
	}
	registration := ports.EvidenceSourceRegistrationRefV1{RegistrationID: subject.Source.RegistrationID, Revision: 1, FactDigest: digest("fact"), ConfigurationDigest: digest("configuration"), SourceID: "custom/source", SourceEpoch: 1}
	request, err := ports.SealEvidenceSubjectMutationRequestV1(ports.EvidenceSubjectMutationRequestV1{ContractVersion: ports.EvidenceSubjectCurrentContractVersionV1, Subject: subject, Kind: ports.EvidenceSubjectMutationSourceRegistrationAdvanceV1, Registration: &registration})
	if err != nil {
		t.Fatal(err)
	}
	projection := ports.EvidenceSubjectCurrentProjectionV1{Ref: ports.EvidenceSubjectProjectionRefV1{OwnerWatermark: 1}, Subject: subject, SubjectKeyDigest: subjectDigest, Record: subject.Record, Source: subject.Source, CandidateDigest: digest("candidate"), Registration: registration, RegistrationState: ports.EvidenceSourceActive, RegistrationExpiresUnixNano: now.Add(time.Minute).UnixNano(), Presence: ports.EvidenceTombstoneAbsentSealedV1, Readability: ports.EvidenceSubjectReadableV1, TombstoneAbsence: &absence, Causation: []ports.EvidenceCausationRefV2{}, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Minute).UnixNano()}
	commit, sealed, index, err := control.NewEvidenceSubjectMutationBundleV1(request, projection, now)
	if err != nil {
		t.Fatal(err)
	}
	return commit, sealed, index
}
