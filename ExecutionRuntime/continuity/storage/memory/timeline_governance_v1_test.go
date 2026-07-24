package memory_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/storage/memory"
)

func TestTimelineAttemptCreateOnceAndCASNoABA(t *testing.T) {
	ctx := context.Background()
	backend := memory.New()
	proposed := proposedTimelineAttemptV1(t)
	created, duplicate, err := backend.CreateTimelineProjectionAttemptV1(ctx, proposed)
	if err != nil || duplicate || created.Ref != proposed.Ref {
		t.Fatalf("create=%#v duplicate=%v err=%v", created, duplicate, err)
	}
	if _, duplicate, err := backend.CreateTimelineProjectionAttemptV1(ctx, proposed); err != nil || !duplicate {
		t.Fatalf("exact create replay duplicate=%v err=%v", duplicate, err)
	}
	drifted := proposed.Clone()
	drifted.Request.ProjectionPolicy = "policy-2"
	drifted.Request.Digest = ""
	drifted.Request.Digest, _ = drifted.Request.CanonicalDigest()
	drifted, _ = contract.SealTimelineProjectionAttemptV1(drifted)
	if _, _, err := backend.CreateTimelineProjectionAttemptV1(ctx, drifted); !contract.HasCode(err, contract.ErrRevisionConflict) {
		t.Fatalf("same attempt changed request: %v", err)
	}

	inspecting := nextTimelineAttemptV1(t, proposed, contract.TimelineAttemptInspectingV1)
	var winners atomic.Int32
	var wg sync.WaitGroup
	for range 64 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := backend.CompareAndSwapTimelineProjectionAttemptV1(ctx, proposed.Ref, inspecting); err == nil {
				winners.Add(1)
			}
		}()
	}
	wg.Wait()
	if winners.Load() != 1 {
		t.Fatalf("CAS winners=%d want=1", winners.Load())
	}
	if _, err := backend.CompareAndSwapTimelineProjectionAttemptV1(ctx, proposed.Ref, inspecting); !contract.HasCode(err, contract.ErrRevisionConflict) {
		t.Fatalf("progressed replay formed ABA: %v", err)
	}
	historical, err := backend.InspectTimelineProjectionAttemptV1(ctx, proposed.Ref)
	if err != nil || historical.Ref != proposed.Ref {
		t.Fatalf("historical attempt lost: %#v err=%v", historical, err)
	}
}

func TestTimelineAtomicPublishRejectsHalfBindingAndLostReplyReplay(t *testing.T) {
	ctx := context.Background()
	backend := memory.New()
	proposed := proposedTimelineAttemptV1(t)
	if _, _, err := backend.CreateTimelineProjectionAttemptV1(ctx, proposed); err != nil {
		t.Fatal(err)
	}
	inspecting := nextTimelineAttemptV1(t, proposed, contract.TimelineAttemptInspectingV1)
	if _, err := backend.CompareAndSwapTimelineProjectionAttemptV1(ctx, proposed.Ref, inspecting); err != nil {
		t.Fatal(err)
	}
	admitted := admittedTimelineAttemptV1(t, inspecting)
	if _, err := backend.CompareAndSwapTimelineProjectionAttemptV1(ctx, inspecting.Ref, admitted); err != nil {
		t.Fatal(err)
	}
	publish := timelinePublishRequestV1(t, admitted)
	drifted := publish
	drifted.Current.EvidenceCurrentIndexDigest = "different-index-digest"
	drifted.Current, _ = contract.SealTimelineProjectionCurrentV1(drifted.Current)
	if _, _, err := backend.PublishTimelineProjectionV1(ctx, drifted); !contract.HasCode(err, contract.ErrProjectionConflict) {
		t.Fatalf("half-bound publish accepted: %v", err)
	}
	if _, err := backend.InspectByEvidence(ctx, publish.Event.EvidenceRecordRef); !contract.HasCode(err, contract.ErrNotFound) {
		t.Fatalf("failed publish leaked Event: %v", err)
	}
	currentAttempt, err := backend.InspectCurrentTimelineProjectionAttemptV1(ctx, admitted.Ref.ScopeDigest, admitted.Ref.AttemptID)
	if err != nil || currentAttempt.Ref != admitted.Ref {
		t.Fatalf("failed publish advanced Attempt: %#v err=%v", currentAttempt, err)
	}

	visible, current, err := backend.PublishTimelineProjectionV1(ctx, publish)
	if err != nil || visible.State != contract.TimelineAttemptVisibleV1 || current.Event != *visible.Event {
		t.Fatalf("publish visible=%#v current=%#v err=%v", visible, current, err)
	}
	if _, _, err := backend.PublishTimelineProjectionV1(ctx, publish); !contract.HasCode(err, contract.ErrRevisionConflict) {
		t.Fatalf("lost-reply replay should Inspect, not replay CAS: %v", err)
	}
	inspected, err := backend.InspectCurrentTimelineProjectionAttemptV1(ctx, visible.Ref.ScopeDigest, visible.Ref.AttemptID)
	if err != nil || inspected.Ref != visible.Ref || inspected.State != contract.TimelineAttemptVisibleV1 {
		t.Fatalf("lost reply inspect=%#v err=%v", inspected, err)
	}
	projection, err := backend.InspectTimelineProjectionCurrentV1(ctx, current.Event)
	if err != nil || projection.Digest != current.Digest {
		t.Fatalf("current inspect=%#v err=%v", projection, err)
	}
}

func proposedTimelineAttemptV1(t *testing.T) contract.TimelineProjectionAttemptFactV1 {
	t.Helper()
	request, err := contract.SealTimelineProjectionRequestV1(contract.TimelineProjectionRequestV1{
		ContractVersion: contract.TimelineGovernanceContractVersionV1,
		AttemptID:       "attempt-1", IdempotencyKey: "idempotency-1",
		EvidenceSource:   contract.EvidenceSourceKey{RegistrationID: "source-1", SourceEpoch: 1, SourceSequence: 1},
		ExpectedRecord:   &contract.TimelineEvidenceRecordRefV1{LedgerScopeDigest: "ledger-scope-1", Sequence: 1, RecordDigest: "record-digest-1"},
		ProjectionPolicy: "policy-1", ScopeDigest: "execution-scope-digest",
	})
	if err != nil {
		t.Fatal(err)
	}
	fact, err := contract.SealTimelineProjectionAttemptV1(contract.TimelineProjectionAttemptFactV1{
		ContractVersion: contract.TimelineGovernanceContractVersionV1,
		Ref:             contract.TimelineProjectionAttemptRefV1{AttemptID: request.AttemptID, Revision: 1, ScopeDigest: request.ScopeDigest},
		Request:         request, State: contract.TimelineAttemptProposedV1,
	})
	if err != nil {
		t.Fatal(err)
	}
	return fact
}

func nextTimelineAttemptV1(t *testing.T, current contract.TimelineProjectionAttemptFactV1, state contract.TimelineProjectionAttemptStateV1) contract.TimelineProjectionAttemptFactV1 {
	t.Helper()
	next := current.Clone()
	next.Ref.Revision++
	next.State = state
	next.Ref.Digest = ""
	sealed, err := contract.SealTimelineProjectionAttemptV1(next)
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

func admittedTimelineAttemptV1(t *testing.T, inspecting contract.TimelineProjectionAttemptFactV1) contract.TimelineProjectionAttemptFactV1 {
	t.Helper()
	next := inspecting.Clone()
	next.Ref.Revision++
	next.State = contract.TimelineAttemptAdmittedV1
	next.EvidenceProjectionRef = "runtime-projection-1"
	next.EvidenceProjectionDigest = "runtime-projection-digest-1"
	next.EvidenceCurrentIndexRef = "runtime-index-1"
	next.EvidenceCurrentIndexDigest = "runtime-index-digest-1"
	next.PolicyProjectionDigest = "policy-projection-digest-1"
	next.CheckedUnixNano = 10
	next.NotAfterUnixNano = 20
	sealed, err := contract.SealTimelineProjectionAttemptV1(next)
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

func timelinePublishRequestV1(t *testing.T, admitted contract.TimelineProjectionAttemptFactV1) ports.PublishTimelineProjectionV1Request {
	t.Helper()
	candidate := testkit.Candidate(1, 1, contract.TrustObservation)
	event := contract.TimelineEventRecord{
		Candidate: candidate, EvidenceRecordRef: candidate.Evidence.RecordRef,
		LedgerScopeDigest: candidate.Evidence.LedgerScopeDigest, LedgerSequence: candidate.Evidence.LedgerSequence,
		EvidenceRecordDigest: candidate.Evidence.RecordDigest, TrustClass: candidate.Evidence.TrustClass,
		ProjectionRevision: 1, Visibility: "visible",
	}
	eventRef := contract.TimelineEventRefV1{
		EventID: candidate.CandidateID, EvidenceRecordRef: event.EvidenceRecordRef,
		LedgerScopeDigest: event.LedgerScopeDigest, LedgerSequence: event.LedgerSequence, Digest: candidate.Digest,
	}
	visible := admitted.Clone()
	visible.Ref.Revision++
	visible.State = contract.TimelineAttemptVisibleV1
	visible.Event = &eventRef
	visible, err := contract.SealTimelineProjectionAttemptV1(visible)
	if err != nil {
		t.Fatal(err)
	}
	current, err := contract.SealTimelineProjectionCurrentV1(contract.TimelineProjectionCurrentV1{
		ContractVersion: contract.TimelineGovernanceContractVersionV1,
		Event:           eventRef, Attempt: visible.Ref,
		EvidenceProjectionRef: admitted.EvidenceProjectionRef, EvidenceProjectionDigest: admitted.EvidenceProjectionDigest,
		EvidenceCurrentIndexRef: admitted.EvidenceCurrentIndexRef, EvidenceCurrentIndexDigest: admitted.EvidenceCurrentIndexDigest,
		PolicyProjectionDigest: admitted.PolicyProjectionDigest,
		CheckedUnixNano:        admitted.CheckedUnixNano, NotAfterUnixNano: admitted.NotAfterUnixNano,
	})
	if err != nil {
		t.Fatal(err)
	}
	return ports.PublishTimelineProjectionV1Request{ExpectedAttempt: admitted.Ref, VisibleAttempt: visible, Event: event, Current: current}
}
