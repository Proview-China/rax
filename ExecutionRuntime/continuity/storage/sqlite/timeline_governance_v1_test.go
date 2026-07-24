package sqlite_test

import (
	"context"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
	continuitysqlite "github.com/Proview-China/rax/ExecutionRuntime/continuity/storage/sqlite"
)

func TestStoreTimelineGovernanceAtomicPublishCASAndReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "continuity.db")
	store := openStore(t, path)
	proposed := sqliteProposedAttempt(t)
	if _, duplicate, err := store.CreateTimelineProjectionAttemptV1(ctx, proposed); err != nil || duplicate {
		t.Fatalf("create duplicate=%v err=%v", duplicate, err)
	}
	if _, duplicate, err := store.CreateTimelineProjectionAttemptV1(ctx, proposed); err != nil || !duplicate {
		t.Fatalf("lost create reply duplicate=%v err=%v", duplicate, err)
	}
	inspecting := sqliteNextAttempt(t, proposed, contract.TimelineAttemptInspectingV1)
	var winners atomic.Int32
	var unexpected atomic.Int32
	var wg sync.WaitGroup
	for range 64 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := store.CompareAndSwapTimelineProjectionAttemptV1(ctx, proposed.Ref, inspecting); err == nil {
				winners.Add(1)
			} else if !contract.HasCode(err, contract.ErrRevisionConflict) {
				unexpected.Add(1)
			}
		}()
	}
	wg.Wait()
	if winners.Load() != 1 || unexpected.Load() != 0 {
		t.Fatalf("attempt CAS closure winners=%d unexpected=%d", winners.Load(), unexpected.Load())
	}
	admitted := sqliteAdmittedAttempt(t, inspecting)
	if _, err := store.CompareAndSwapTimelineProjectionAttemptV1(ctx, inspecting.Ref, admitted); err != nil {
		t.Fatal(err)
	}
	publish := sqlitePublishRequest(t, admitted)
	drifted := publish
	drifted.Current.EvidenceCurrentIndexDigest = "drifted-index"
	drifted.Current, _ = contract.SealTimelineProjectionCurrentV1(drifted.Current)
	if _, _, err := store.PublishTimelineProjectionV1(ctx, drifted); !contract.HasCode(err, contract.ErrProjectionConflict) {
		t.Fatalf("half-bound publish was accepted: %v", err)
	}
	if _, err := store.InspectByEvidence(ctx, publish.Event.EvidenceRecordRef); !contract.HasCode(err, contract.ErrNotFound) {
		t.Fatalf("failed publish leaked event: %v", err)
	}
	visible, current, err := store.PublishTimelineProjectionV1(ctx, publish)
	if err != nil || visible.State != contract.TimelineAttemptVisibleV1 || current.Event != *visible.Event {
		t.Fatalf("visible=%#v current=%#v err=%v", visible, current, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store = openStore(t, path)
	defer store.Close()
	if _, _, err := store.PublishTimelineProjectionV1(ctx, publish); !contract.HasCode(err, contract.ErrRevisionConflict) {
		t.Fatalf("lost publish reply replayed stale CAS: %v", err)
	}
	gotAttempt, err := store.InspectCurrentTimelineProjectionAttemptV1(ctx, visible.Ref.ScopeDigest, visible.Ref.AttemptID)
	if err != nil || gotAttempt.Ref != visible.Ref {
		t.Fatalf("current attempt=%#v err=%v", gotAttempt, err)
	}
	gotCurrent, err := store.InspectTimelineProjectionCurrentV1(ctx, current.Event)
	if err != nil || gotCurrent.Digest != current.Digest {
		t.Fatalf("current projection=%#v err=%v", gotCurrent, err)
	}
	historical, err := store.InspectTimelineProjectionAttemptV1(ctx, proposed.Ref)
	if err != nil || historical.Ref != proposed.Ref {
		t.Fatalf("history=%#v err=%v", historical, err)
	}
}

func TestStoreTimelinePolicyHistoryCurrentCASAndExpiry(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(1_800_000_000, 0)
	currentTime := now
	path := filepath.Join(t.TempDir(), "continuity.db")
	store, err := continuitysqlite.OpenWithClock(ctx, path, func() time.Time { return currentTime })
	if err != nil {
		t.Fatal(err)
	}
	first := sqlitePolicy(t, "policy-a", "scope-a", 1, now, now.Add(time.Minute))
	if _, duplicate, err := store.CreateTimelineProjectionPolicyV1(ctx, first); err != nil || duplicate {
		t.Fatalf("create duplicate=%v err=%v", duplicate, err)
	}
	if _, duplicate, err := store.CreateTimelineProjectionPolicyV1(ctx, first); err != nil || !duplicate {
		t.Fatalf("lost create reply duplicate=%v err=%v", duplicate, err)
	}
	var winners atomic.Int32
	var unexpected atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			next := sqlitePolicy(t, "policy-a", "scope-a", 2, now.Add(time.Second), now.Add(time.Duration(120+i)*time.Second))
			if _, err := store.CompareAndSwapTimelineProjectionPolicyV1(ctx, first.Ref, next); err == nil {
				winners.Add(1)
			} else if !contract.HasCode(err, contract.ErrRevisionConflict) {
				unexpected.Add(1)
			}
		}(i)
	}
	wg.Wait()
	if winners.Load() != 1 || unexpected.Load() != 0 {
		t.Fatalf("policy CAS closure winners=%d unexpected=%d", winners.Load(), unexpected.Load())
	}
	currentTime = now.Add(time.Second)
	current, err := store.InspectTimelineProjectionPolicyCurrentV1(ctx, first.Ref.PolicyID, first.Ref.ScopeDigest)
	if err != nil || current.Ref.Revision != 2 {
		t.Fatalf("current=%#v err=%v", current, err)
	}
	if err := store.ValidateTimelineProjectionPolicyCurrentV1(ctx, first); !contract.HasCode(err, contract.ErrRevisionConflict) {
		t.Fatalf("old policy remained current: %v", err)
	}
	if historical, err := store.InspectTimelineProjectionPolicyV1(ctx, first.Ref); err != nil || historical.Ref != first.Ref {
		t.Fatalf("history=%#v err=%v", historical, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = continuitysqlite.OpenWithClock(ctx, path, func() time.Time { return currentTime })
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	current, err = store.InspectTimelineProjectionPolicyCurrentV1(ctx, first.Ref.PolicyID, first.Ref.ScopeDigest)
	if err != nil || current.Ref.Revision != 2 {
		t.Fatalf("reopen current=%#v err=%v", current, err)
	}
	currentTime = time.Unix(0, current.ExpiresUnixNano)
	if err := store.ValidateTimelineProjectionPolicyCurrentV1(ctx, current); !contract.HasCode(err, contract.ErrPreconditionFailed) {
		t.Fatalf("exact expiry did not fail closed: %v", err)
	}
}

func sqliteProposedAttempt(t *testing.T) contract.TimelineProjectionAttemptFactV1 {
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

func sqliteNextAttempt(t *testing.T, current contract.TimelineProjectionAttemptFactV1, state contract.TimelineProjectionAttemptStateV1) contract.TimelineProjectionAttemptFactV1 {
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

func sqliteAdmittedAttempt(t *testing.T, current contract.TimelineProjectionAttemptFactV1) contract.TimelineProjectionAttemptFactV1 {
	t.Helper()
	next := current.Clone()
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

func sqlitePublishRequest(t *testing.T, admitted contract.TimelineProjectionAttemptFactV1) ports.PublishTimelineProjectionV1Request {
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

func sqlitePolicy(t *testing.T, id, scope string, revision uint64, checked, expires time.Time) contract.TimelineProjectionPolicyCurrentV1 {
	t.Helper()
	value, err := contract.SealTimelineProjectionPolicyCurrentV1(contract.TimelineProjectionPolicyCurrentV1{
		Ref:   contract.TimelineProjectionPolicyRefV1{PolicyID: id, Revision: revision, ScopeDigest: scope},
		State: contract.TimelineProjectionPolicyActiveV1, CheckedUnixNano: checked.UnixNano(), ExpiresUnixNano: expires.UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return value
}
