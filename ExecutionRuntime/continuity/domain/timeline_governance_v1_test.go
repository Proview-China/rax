package domain_test

import (
	"context"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/domain"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/storage/memory"
)

func TestTimelineGovernanceV1AppliesCallerBoundOnlyAfterNaturalTTL(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 17, 6, 30, 0, 0, time.UTC)
	clock := &testkit.Clock{Time: now}
	controller, err := domain.NewTimelineProjectionControllerV1(memory.New(), clock)
	if err != nil {
		t.Fatal(err)
	}
	request := domainTimelineRequestV1(t, now.Add(30*time.Second).UnixNano())
	proposed, _, err := controller.CreateAttempt(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	inspecting, err := controller.BeginInspection(ctx, proposed.Ref)
	if err != nil {
		t.Fatal(err)
	}
	admitted, err := controller.Admit(ctx, inspecting.Ref, domain.TimelineAdmissionBindingsV1{
		EvidenceProjectionRef: "runtime-projection-1", EvidenceProjectionDigest: "runtime-projection-digest-1",
		EvidenceCurrentIndexRef: "runtime-index-1", EvidenceCurrentIndexDigest: "runtime-index-digest-1",
		PolicyProjectionDigest: "policy-projection-digest-1",
		CheckedUnixNano:        now.Add(-time.Second).UnixNano(), NaturalNotAfterUnixNano: now.Add(time.Minute).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if admitted.NotAfterUnixNano != request.RequestedNotAfter {
		t.Fatalf("caller bound was not applied last: got=%d want=%d", admitted.NotAfterUnixNano, request.RequestedNotAfter)
	}
	clock.Time = time.Unix(0, request.RequestedNotAfter)
	if _, _, err := controller.Publish(ctx, admitted.Ref, domainTimelineEventV1()); !contract.HasCode(err, contract.ErrPreconditionFailed) {
		t.Fatalf("publish accepted exact expiry boundary: %v", err)
	}
}

func TestTimelineGovernanceV1PublishesAtomicallyAndRejectsAuthoritativeWithoutOwnerProjection(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 17, 6, 30, 0, 0, time.UTC)
	clock := &testkit.Clock{Time: now}
	backend := memory.New()
	controller, _ := domain.NewTimelineProjectionControllerV1(backend, clock)
	request := domainTimelineRequestV1(t, 0)
	proposed, _, _ := controller.CreateAttempt(ctx, request)
	inspecting, _ := controller.BeginInspection(ctx, proposed.Ref)
	admitted, err := controller.Admit(ctx, inspecting.Ref, domain.TimelineAdmissionBindingsV1{
		EvidenceProjectionRef: "runtime-projection-1", EvidenceProjectionDigest: "runtime-projection-digest-1",
		EvidenceCurrentIndexRef: "runtime-index-1", EvidenceCurrentIndexDigest: "runtime-index-digest-1",
		PolicyProjectionDigest: "policy-projection-digest-1",
		CheckedUnixNano:        now.Add(-time.Second).UnixNano(), NaturalNotAfterUnixNano: now.Add(time.Minute).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	visible, current, err := controller.Publish(ctx, admitted.Ref, domainTimelineEventV1())
	if err != nil || visible.State != contract.TimelineAttemptVisibleV1 || current.Attempt != visible.Ref {
		t.Fatalf("visible=%#v current=%#v err=%v", visible, current, err)
	}
	history, err := backend.InspectByEvidence(ctx, current.Event.EvidenceRecordRef)
	if err != nil || history.Candidate.Digest != current.Event.Digest {
		t.Fatalf("atomic history=%#v err=%v", history, err)
	}

	authoritativeRequest := domainTimelineRequestV1(t, 0)
	authoritativeRequest.AttemptID = "attempt-authoritative"
	authoritativeRequest.IdempotencyKey = "idempotency-authoritative"
	authoritativeRequest.OwnerFact = &contract.TimelineOwnerFactRefV1{
		Owner: testkit.Owner(), FactKind: "praxis/fact", FactID: "fact-1", Revision: 1,
		FactDigest: "fact-digest-1", PayloadSchema: "schema/fact-v1", PayloadDigest: "payload-digest-1",
		PayloadRevision: 1, ScopeDigest: "execution-scope-digest",
	}
	authoritativeRequest, _ = contract.SealTimelineProjectionRequestV1(authoritativeRequest)
	proposed, _, _ = controller.CreateAttempt(ctx, authoritativeRequest)
	inspecting, _ = controller.BeginInspection(ctx, proposed.Ref)
	_, err = controller.Admit(ctx, inspecting.Ref, domain.TimelineAdmissionBindingsV1{
		EvidenceProjectionRef: "runtime-projection-2", EvidenceProjectionDigest: "runtime-projection-digest-2",
		EvidenceCurrentIndexRef: "runtime-index-2", EvidenceCurrentIndexDigest: "runtime-index-digest-2",
		PolicyProjectionDigest: "policy-projection-digest-1",
		CheckedUnixNano:        now.Add(-time.Second).UnixNano(), NaturalNotAfterUnixNano: now.Add(time.Minute).UnixNano(),
	})
	if !contract.HasCode(err, contract.ErrProjectionConflict) {
		t.Fatalf("authoritative admission omitted typed owner projection: %v", err)
	}
}

func domainTimelineRequestV1(t *testing.T, requestedNotAfter int64) contract.TimelineProjectionRequestV1 {
	t.Helper()
	request, err := contract.SealTimelineProjectionRequestV1(contract.TimelineProjectionRequestV1{
		ContractVersion: contract.TimelineGovernanceContractVersionV1,
		AttemptID:       "attempt-1", IdempotencyKey: "idempotency-1",
		EvidenceSource:   contract.EvidenceSourceKey{RegistrationID: "source-1", SourceEpoch: 1, SourceSequence: 1},
		ExpectedRecord:   &contract.TimelineEvidenceRecordRefV1{LedgerScopeDigest: "ledger-scope-1", Sequence: 1, RecordDigest: "record-digest-1"},
		ProjectionPolicy: "projection-policy-1", ScopeDigest: "execution-scope-digest",
		RequestedNotAfter: requestedNotAfter,
	})
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func domainTimelineEventV1() contract.TimelineEventRecord {
	return timelineRecordFromCandidateV1(testkit.Candidate(1, 1, contract.TrustObservation))
}

func timelineRecordFromCandidateV1(candidate contract.TimelineProjectionCandidate) contract.TimelineEventRecord {
	return contract.TimelineEventRecord{
		Candidate: candidate, EvidenceRecordRef: candidate.Evidence.RecordRef,
		LedgerScopeDigest: candidate.Evidence.LedgerScopeDigest, LedgerSequence: candidate.Evidence.LedgerSequence,
		EvidenceRecordDigest: candidate.Evidence.RecordDigest, TrustClass: candidate.Evidence.TrustClass,
		ProjectionRevision: 1, Visibility: "visible",
	}
}
