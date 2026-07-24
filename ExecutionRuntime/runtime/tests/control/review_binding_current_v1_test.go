package control_test

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestReviewBindingCurrentOwnerV1RejectsTypedNilAndClockRollback(t *testing.T) {
	var typedNil *reviewBindingRepositoryStubV1
	if _, err := control.NewReviewBindingCurrentOwnerV1(typedNil, time.Now); !core.HasReason(err, core.ReasonComponentMissing) {
		t.Fatalf("typed-nil repository was accepted: %v", err)
	}
	request := controlReviewBindingInspectCurrentRequestV1(t)
	repository := &reviewBindingRepositoryStubV1{}
	times := []time.Time{time.Unix(10, 0), time.Unix(9, 0)}
	var calls atomic.Int32
	owner, err := control.NewReviewBindingCurrentOwnerV1(repository, func() time.Time { return times[int(calls.Add(1))-1] })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := owner.InspectCurrentReviewBindingV1(context.Background(), request); !core.HasReason(err, core.ReasonClockRegression) {
		t.Fatalf("clock rollback across repository read did not fail closed: %v", err)
	}
}

func TestReviewBindingCurrentOwnerV1DoesNotRetryUnknownMutation(t *testing.T) {
	repository := &reviewBindingRepositoryStubV1{publishErr: core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "unknown commit")}
	owner, err := control.NewReviewBindingCurrentOwnerV1(repository, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	input := ports.CreateReviewBindingProjectionCommandInputV1{Source: controlReviewBindingSourceV1(), Subject: controlReviewBindingSubjectV1(), Association: controlReviewBindingAssociationRefV1(t)}
	publishRef, err := ports.DeriveCreateReviewBindingProjectionPublishRefV1(input)
	if err != nil {
		t.Fatal(err)
	}
	_, err = owner.CreateReviewBindingProjectionV1(context.Background(), ports.CreateReviewBindingProjectionRequestV1{PublishRef: publishRef, Input: input})
	if !errors.Is(err, repository.publishErr) || repository.createCalls.Load() != 1 || repository.inspectPublishCalls.Load() != 0 {
		t.Fatalf("Owner retried or translated unknown mutation: create=%d inspect=%d err=%v", repository.createCalls.Load(), repository.inspectPublishCalls.Load(), err)
	}
}

func TestReviewBindingCurrentOwnerV1PassesClosedReadErrorsUnchanged(t *testing.T) {
	sentinels := []error{
		core.NewError(core.ErrorNotFound, core.ReasonOwnerMissing, "missing"),
		core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "drift"),
		core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "backend unavailable"),
		core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "unknown read"),
	}
	for index, sentinel := range sentinels {
		t.Run(fmt.Sprintf("error-%d", index), func(t *testing.T) {
			repository := &reviewBindingRepositoryStubV1{readErr: sentinel}
			owner, err := control.NewReviewBindingCurrentOwnerV1(repository, time.Now)
			if err != nil {
				t.Fatal(err)
			}
			_, err = owner.InspectCurrentReviewBindingV1(context.Background(), controlReviewBindingInspectCurrentRequestV1(t))
			if !errors.Is(err, sentinel) {
				t.Fatalf("closed repository error was translated: got=%v want=%v", err, sentinel)
			}
		})
	}
}

func TestReviewBindingCurrentOwnerV1CompileShape(t *testing.T) {
	var _ ports.ReviewBindingAuthoritativeCurrentReaderV1 = (*control.ReviewBindingCurrentOwnerV1)(nil)
	var _ ports.ReviewBindingConsumerAssociationCurrentReaderV1 = (*control.ReviewBindingCurrentOwnerV1)(nil)
	var _ ports.ReviewBindingProjectionPublisherV1 = (*control.ReviewBindingCurrentOwnerV1)(nil)
}

type reviewBindingRepositoryStubV1 struct {
	projection          ports.ReviewBindingCurrentProjectionV1
	receipt             ports.ReviewBindingProjectionPublishReceiptV1
	readErr             error
	publishErr          error
	createCalls         atomic.Int32
	inspectPublishCalls atomic.Int32
}

func (r *reviewBindingRepositoryStubV1) ResolveCurrentReviewBindingV1(context.Context, ports.ResolveReviewBindingCurrentRequestV1) (ports.ReviewBindingProjectionRefV1, error) {
	return r.projection.Ref, r.readErr
}
func (r *reviewBindingRepositoryStubV1) InspectReviewBindingProjectionV1(context.Context, ports.InspectReviewBindingProjectionRequestV1) (ports.ReviewBindingCurrentProjectionV1, error) {
	return r.projection, r.readErr
}
func (r *reviewBindingRepositoryStubV1) InspectCurrentReviewBindingV1(context.Context, ports.InspectCurrentReviewBindingRequestV1) (ports.ReviewBindingCurrentProjectionV1, error) {
	return r.projection, r.readErr
}
func (r *reviewBindingRepositoryStubV1) InspectCurrentReviewBindingConsumerAssociationV1(context.Context, ports.ReviewBindingConsumerAssociationRefV1) (ports.ReviewBindingConsumerAssociationCurrentProjectionV1, error) {
	return r.projection.ConsumerAssociation, r.readErr
}
func (r *reviewBindingRepositoryStubV1) CreateReviewBindingProjectionV1(context.Context, ports.CreateReviewBindingProjectionRequestV1) (ports.ReviewBindingProjectionPublishReceiptV1, error) {
	r.createCalls.Add(1)
	return r.receipt, r.publishErr
}
func (r *reviewBindingRepositoryStubV1) CompareAndSwapReviewBindingProjectionV1(context.Context, ports.CompareAndSwapReviewBindingProjectionRequestV1) (ports.ReviewBindingProjectionPublishReceiptV1, error) {
	return r.receipt, r.publishErr
}
func (r *reviewBindingRepositoryStubV1) CompareAndSwapReviewBindingAssociationProjectionV1(context.Context, control.CompareAndSwapReviewBindingAssociationProjectionRequestV1) (ports.ReviewBindingProjectionPublishReceiptV1, error) {
	return r.receipt, r.publishErr
}
func (r *reviewBindingRepositoryStubV1) InspectReviewBindingProjectionPublishV1(context.Context, ports.ReviewBindingProjectionPublishRefV1) (ports.ReviewBindingProjectionPublishReceiptV1, error) {
	r.inspectPublishCalls.Add(1)
	return r.receipt, r.readErr
}

func controlReviewBindingInspectCurrentRequestV1(t *testing.T) ports.InspectCurrentReviewBindingRequestV1 {
	t.Helper()
	source, subject := controlReviewBindingSourceV1(), controlReviewBindingSubjectV1()
	id, err := ports.DeriveReviewBindingProjectionIDV1(ports.ReviewBindingProjectionIdentityInputV1{Source: source, Subject: subject})
	if err != nil {
		t.Fatal(err)
	}
	return ports.InspectCurrentReviewBindingRequestV1{ExpectedRef: ports.ReviewBindingProjectionRefV1{ID: id, Revision: 1, Digest: controlReviewBindingDigestV1(t, "projection")}, ExpectedSource: source, ExpectedSubject: subject}
}

func controlReviewBindingSourceV1() ports.ReviewComponentBindingRefV2 {
	return ports.ReviewComponentBindingRefV2{BindingSetID: "set-a", BindingSetRevision: 1, ComponentID: "review/worker", ManifestDigest: core.Digest("sha256:1111111111111111111111111111111111111111111111111111111111111111"), ArtifactDigest: core.Digest("sha256:2222222222222222222222222222222222222222222222222222222222222222"), Capability: "review/attest"}
}

func controlReviewBindingSubjectV1() ports.ReviewBindingSubjectV1 {
	return ports.ReviewBindingSubjectV1{TenantID: "tenant-a", AssignmentID: "assignment-a", AssignmentRevision: 1, AssignmentDigest: core.Digest("sha256:3333333333333333333333333333333333333333333333333333333333333333"), ReviewerID: "reviewer-a", TargetID: "target-a", TargetRevision: 1, TargetDigest: core.Digest("sha256:4444444444444444444444444444444444444444444444444444444444444444")}
}

func controlReviewBindingAssociationRefV1(t *testing.T) ports.ReviewBindingConsumerAssociationRefV1 {
	t.Helper()
	return ports.ReviewBindingConsumerAssociationRefV1{ID: "association-a", Revision: 1, Digest: controlReviewBindingDigestV1(t, "association")}
}

func controlReviewBindingDigestV1(t *testing.T, value any) core.Digest {
	t.Helper()
	digest, err := core.DigestJSON(value)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}
