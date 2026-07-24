package sqlite

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type reviewFixture struct {
	now         time.Time
	path        string
	store       *Store
	source      ports.ReviewComponentBindingRefV2
	consumer    ports.ProviderBindingRefV2
	association ports.ReviewBindingConsumerAssociationCurrentProjectionV1
	subject     ports.ReviewBindingSubjectV1
	create      ports.CreateReviewBindingProjectionRequestV1
}

func newReviewFixture(t *testing.T) *reviewFixture {
	t.Helper()
	base := time.Unix(2_400_000_000, 0)
	now := base.Add(2 * time.Second)
	path := testDBPath(t)
	store := openTestStore(t, path, func() time.Time { return now })
	sourceSet, sourceFact := commitComponent(t, store, base, "source-set", "source-binding", "review/auto-worker", "review/attest")
	consumerSet, consumerFact := commitComponent(t, store, base, "consumer-set", "consumer-binding", "review/verdict-owner", "runtime/read-review-binding-current")
	source := ports.ReviewComponentBindingRefV2{BindingSetID: sourceSet.ID, BindingSetRevision: sourceSet.Revision, ComponentID: sourceFact.ComponentID, ManifestDigest: sourceFact.ManifestDigest, ArtifactDigest: sourceFact.Manifest.ArtifactDigest, Capability: "review/attest"}
	consumer := ports.ProviderBindingRefV2{BindingSetID: consumerSet.ID, BindingSetRevision: consumerSet.Revision, ComponentID: consumerFact.ComponentID, ManifestDigest: consumerFact.ManifestDigest, ArtifactDigest: consumerFact.Manifest.ArtifactDigest, Capability: "runtime/read-review-binding-current"}
	association, err := ports.SealReviewBindingConsumerAssociationCurrentProjectionV1(ports.ReviewBindingConsumerAssociationCurrentProjectionV1{Ref: ports.ReviewBindingConsumerAssociationRefV1{Revision: 1}, Consumer: consumer, Source: source, Current: true, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: base.Add(4 * time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateReviewBindingConsumerAssociationV1(context.Background(), association); err != nil {
		t.Fatal(err)
	}
	subject := ports.ReviewBindingSubjectV1{TenantID: "tenant-a", AssignmentID: "assignment-a", AssignmentRevision: 3, AssignmentDigest: testDigest(t, "assignment"), ReviewerID: "reviewer-a", TargetID: "target-a", TargetRevision: 5, TargetDigest: testDigest(t, "target")}
	input := ports.CreateReviewBindingProjectionCommandInputV1{Source: source, Subject: subject, Association: association.Ref}
	publishRef, err := ports.DeriveCreateReviewBindingProjectionPublishRefV1(input)
	if err != nil {
		t.Fatal(err)
	}
	return &reviewFixture{now: now, path: path, store: store, source: source, consumer: consumer, association: association, subject: subject, create: ports.CreateReviewBindingProjectionRequestV1{PublishRef: publishRef, Input: input}}
}

func (f *reviewFixture) nextAssociation(t *testing.T) ports.ReviewBindingConsumerAssociationCurrentProjectionV1 {
	t.Helper()
	next := f.association
	next.Ref.Revision++
	next.Ref.Digest, next.ProjectionDigest = "", ""
	next.ExpiresUnixNano += int64(10 * time.Second)
	sealed, err := ports.SealReviewBindingConsumerAssociationCurrentProjectionV1(next)
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

func (f *reviewFixture) compound(t *testing.T, expected ports.ReviewBindingProjectionRefV1, next ports.ReviewBindingConsumerAssociationCurrentProjectionV1) control.CompareAndSwapReviewBindingAssociationProjectionRequestV1 {
	t.Helper()
	input := ports.CompareAndSwapReviewBindingProjectionCommandInputV1{ExpectedCurrent: expected, Source: f.source, Subject: f.subject, Association: next.Ref}
	publishRef, err := ports.DeriveCompareAndSwapReviewBindingProjectionPublishRefV1(input)
	if err != nil {
		t.Fatal(err)
	}
	return control.CompareAndSwapReviewBindingAssociationProjectionRequestV1{ExpectedAssociation: f.association.Ref, NextAssociation: next, Projection: ports.CompareAndSwapReviewBindingProjectionRequestV1{PublishRef: publishRef, Input: input}}
}

func TestReviewBindingSQLiteLostReplyRestartTenantIsolationAndDeepClone(t *testing.T) {
	fixture := newReviewFixture(t)
	fixture.store.loseNextReplyForTest()
	if _, err := fixture.store.CreateReviewBindingProjectionV1(context.Background(), fixture.create); !core.HasCategory(err, core.ErrorIndeterminate) {
		t.Fatalf("lost Create reply did not remain Indeterminate: %v", err)
	}
	receipt, err := fixture.store.InspectReviewBindingProjectionPublishV1(context.Background(), fixture.create.PublishRef)
	if err != nil {
		t.Fatal(err)
	}
	currentRequest := ports.InspectCurrentReviewBindingRequestV1{ExpectedRef: receipt.Projection, ExpectedSource: fixture.source, ExpectedSubject: fixture.subject}
	current, err := fixture.store.InspectCurrentReviewBindingV1(context.Background(), currentRequest)
	if err != nil {
		t.Fatal(err)
	}
	current.Members[0].BindingID = "mutated-return"
	again, err := fixture.store.InspectCurrentReviewBindingV1(context.Background(), currentRequest)
	if err != nil || again.Members[0].BindingID == "mutated-return" {
		t.Fatalf("current Inspect leaked mutable alias: %+v %v", again, err)
	}
	if err := fixture.store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := openTestStore(t, fixture.path, func() time.Time { return fixture.now })
	resolved, err := reopened.ResolveCurrentReviewBindingV1(context.Background(), ports.ResolveReviewBindingCurrentRequestV1{Source: fixture.source, Subject: fixture.subject})
	if err != nil || resolved != receipt.Projection {
		t.Fatalf("restart lost exact current projection: %+v %v", resolved, err)
	}
	otherSubject := fixture.subject
	otherSubject.TenantID = "tenant-b"
	otherInput := ports.CreateReviewBindingProjectionCommandInputV1{Source: fixture.source, Subject: otherSubject, Association: fixture.association.Ref}
	otherPublish, _ := ports.DeriveCreateReviewBindingProjectionPublishRefV1(otherInput)
	otherReceipt, err := reopened.CreateReviewBindingProjectionV1(context.Background(), ports.CreateReviewBindingProjectionRequestV1{PublishRef: otherPublish, Input: otherInput})
	if err != nil || otherReceipt.Projection.ID == receipt.Projection.ID {
		t.Fatalf("tenant identities collided: %+v %v", otherReceipt, err)
	}
	if _, err := reopened.InspectReviewBindingProjectionV1(context.Background(), ports.InspectReviewBindingProjectionRequestV1{Ref: receipt.Projection, ExpectedSource: fixture.source, ExpectedSubject: otherSubject}); err == nil {
		t.Fatal("cross-tenant historical Inspect succeeded")
	}
}

func TestReviewBindingSQLiteCompoundCAS64ConcurrentOneRevision(t *testing.T) {
	fixture := newReviewFixture(t)
	created, err := fixture.store.CreateReviewBindingProjectionV1(context.Background(), fixture.create)
	if err != nil {
		t.Fatal(err)
	}
	next := fixture.nextAssociation(t)
	request := fixture.compound(t, created.Projection, next)
	var wait sync.WaitGroup
	receipts := make(chan ports.ReviewBindingProjectionPublishReceiptV1, 64)
	errors := make(chan error, 64)
	for range 64 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			receipt, callErr := fixture.store.CompareAndSwapReviewBindingAssociationProjectionV1(context.Background(), request)
			if callErr != nil {
				errors <- callErr
				return
			}
			receipts <- receipt
		}()
	}
	wait.Wait()
	close(receipts)
	close(errors)
	for err := range errors {
		t.Fatalf("canonical concurrent compound CAS failed: %v", err)
	}
	var canonical ports.ReviewBindingProjectionPublishReceiptV1
	for receipt := range receipts {
		if canonical.PublishRef.ID == "" {
			canonical = receipt
		}
		if receipt != canonical {
			t.Fatalf("concurrent compound CAS returned different receipts")
		}
	}
	if canonical.Projection.Revision != created.Projection.Revision+1 || canonical.HighestRevision != canonical.Projection.Revision {
		t.Fatalf("64 compound CAS calls advanced multiple revisions: %+v", canonical)
	}
	current, err := fixture.store.InspectCurrentReviewBindingV1(context.Background(), ports.InspectCurrentReviewBindingRequestV1{ExpectedRef: canonical.Projection, ExpectedSource: fixture.source, ExpectedSubject: fixture.subject})
	if err != nil || current.ConsumerAssociation.Ref != next.Ref {
		t.Fatalf("compound CAS did not publish one consistent current snapshot: %+v %v", current, err)
	}
	historical, err := fixture.store.InspectReviewBindingProjectionV1(context.Background(), ports.InspectReviewBindingProjectionRequestV1{Ref: created.Projection, ExpectedSource: fixture.source, ExpectedSubject: fixture.subject})
	if err != nil || historical.Ref != created.Projection {
		t.Fatalf("compound CAS overwrote immutable history: %+v %v", historical, err)
	}
}

func TestReviewBindingSQLiteCompoundStagedFailureAndLostReply(t *testing.T) {
	for _, lostReply := range []bool{false, true} {
		name := "stage"
		if lostReply {
			name = "lost-reply"
		}
		t.Run(name, func(t *testing.T) {
			fixture := newReviewFixture(t)
			created, err := fixture.store.CreateReviewBindingProjectionV1(context.Background(), fixture.create)
			if err != nil {
				t.Fatal(err)
			}
			next := fixture.nextAssociation(t)
			request := fixture.compound(t, created.Projection, next)
			if lostReply {
				fixture.store.loseNextReplyForTest()
			} else {
				fixture.store.failNextStageForTest()
			}
			_, err = fixture.store.CompareAndSwapReviewBindingAssociationProjectionV1(context.Background(), request)
			if lostReply {
				if !core.HasCategory(err, core.ErrorIndeterminate) {
					t.Fatalf("lost reply was not Indeterminate: %v", err)
				}
				receipt, inspectErr := fixture.store.InspectReviewBindingProjectionPublishV1(context.Background(), request.Projection.PublishRef)
				if inspectErr != nil || receipt.Projection.Revision != 2 {
					t.Fatalf("exact receipt did not recover compound commit: %+v %v", receipt, inspectErr)
				}
				return
			}
			if !core.HasCategory(err, core.ErrorUnavailable) {
				t.Fatalf("staged failure was not Unavailable: %v", err)
			}
			if _, inspectErr := fixture.store.InspectReviewBindingProjectionPublishV1(context.Background(), request.Projection.PublishRef); !core.HasCategory(inspectErr, core.ErrorNotFound) {
				t.Fatalf("staged failure leaked receipt: %v", inspectErr)
			}
			current, inspectErr := fixture.store.InspectCurrentReviewBindingV1(context.Background(), ports.InspectCurrentReviewBindingRequestV1{ExpectedRef: created.Projection, ExpectedSource: fixture.source, ExpectedSubject: fixture.subject})
			if inspectErr != nil || current.ConsumerAssociation.Ref != fixture.association.Ref {
				t.Fatalf("staged failure leaked association/projection half-state: %+v %v", current, inspectErr)
			}
		})
	}
}

func TestReviewBindingSQLiteUnderlyingDriftFailsCurrentHistorySurvives(t *testing.T) {
	fixture := newReviewFixture(t)
	created, err := fixture.store.CreateReviewBindingProjectionV1(context.Background(), fixture.create)
	if err != nil {
		t.Fatal(err)
	}
	set, err := fixture.store.InspectBindingSet(context.Background(), fixture.consumer.BindingSetID)
	if err != nil {
		t.Fatal(err)
	}
	revoked := set
	revoked.Revision++
	revoked.State = control.BindingSetRevoked
	revoked.InvalidationReason = core.ReasonBindingDrift
	if _, err := fixture.store.CompareAndSwapBindingSet(context.Background(), control.BindingSetCASRequestV2{ExpectedRevision: set.Revision, Next: revoked}); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.store.InspectCurrentReviewBindingV1(context.Background(), ports.InspectCurrentReviewBindingRequestV1{ExpectedRef: created.Projection, ExpectedSource: fixture.source, ExpectedSubject: fixture.subject}); err == nil {
		t.Fatal("underlying Consumer drift passed current Inspect")
	}
	historical, err := fixture.store.InspectReviewBindingProjectionV1(context.Background(), ports.InspectReviewBindingProjectionRequestV1{Ref: created.Projection, ExpectedSource: fixture.source, ExpectedSubject: fixture.subject})
	if err != nil || historical.Ref != created.Projection {
		t.Fatalf("bad current state contaminated immutable history: %+v %v", historical, err)
	}
}
