package action_test

import (
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/action"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
)

func prepareDomainResult(t *testing.T) (*action.Controller, string, string) {
	t.Helper()
	controller := action.NewController()
	candidate := testkit.Candidate()
	if _, err := controller.PutCandidate(candidate, candidate.PendingActionDigest); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.Reserve(candidate.ID, testkit.Digest("app-attempt"), testkit.Digest("intent"), testkit.Digest("subject"), candidate.SessionID, testkit.FixedTime, testkit.FixedTime.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	attemptID := "attempt-1"
	if _, err := controller.RecordDomainResult(candidate.ID, attemptID, testkit.Digest("observation"), testkit.Payload(`{"ok":true}`), nil, testkit.FixedTime.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	return controller, candidate.ID, attemptID
}

func TestWhiteboxDomainResultPrecedesRuntimeSettlement(t *testing.T) {
	controller := action.NewController()
	candidate := testkit.Candidate()
	if _, err := controller.PutCandidate(candidate, candidate.PendingActionDigest); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.ApplySettlement(candidate.ID, testkit.Settlement("attempt-1", testkit.Payload(`{"ok":true}`)), testkit.FixedTime); err == nil {
		t.Fatal("settlement without a DomainResultFact was accepted")
	}
}

func TestWhiteboxApplySettlementExactReference(t *testing.T) {
	controller, actionID, attemptID := prepareDomainResult(t)
	payload := testkit.Payload(`{"ok":true}`)
	settlement := testkit.Settlement(attemptID, payload)
	record, err := controller.ApplySettlement(actionID, settlement, testkit.FixedTime.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if record.State != action.StateSettled || record.Result == nil || record.Result.Settlement.Digest != settlement.Digest {
		t.Fatal("settled record does not retain exact runtime settlement")
	}
	second, err := controller.ApplySettlement(actionID, settlement, testkit.FixedTime.Add(3*time.Second))
	if err != nil || second.Result.Digest != record.Result.Digest {
		t.Fatalf("lost-reply ApplySettlement was not idempotent: %v", err)
	}
}

func TestWhiteboxApplySettlementRejectsDifferentResult(t *testing.T) {
	controller, actionID, attemptID := prepareDomainResult(t)
	settlement := testkit.Settlement(attemptID, testkit.Payload(`{"different":true}`))
	if _, err := controller.ApplySettlement(actionID, settlement, testkit.FixedTime.Add(2*time.Second)); err == nil {
		t.Fatal("settlement referencing different DomainResult was accepted")
	}
}

func TestWhiteboxConcurrentApplySettlementSingleWinner(t *testing.T) {
	controller, actionID, attemptID := prepareDomainResult(t)
	settlement := testkit.Settlement(attemptID, testkit.Payload(`{"ok":true}`))
	const workers = 64
	var wg sync.WaitGroup
	errors := make(chan error, workers)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := controller.ApplySettlement(actionID, settlement, testkit.FixedTime.Add(2*time.Second))
			errors <- err
		}()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	record, ok := controller.Inspect(actionID)
	if !ok || record.State != action.StateSettled || record.Revision != 4 {
		t.Fatalf("want one settled transition at revision 4, got %+v", record)
	}
}

func TestWhiteboxInspectReturnsDefensiveCopy(t *testing.T) {
	controller := action.NewController()
	candidate := testkit.Candidate()
	if _, err := controller.PutCandidate(candidate, candidate.PendingActionDigest); err != nil {
		t.Fatal(err)
	}
	first, _ := controller.Inspect(candidate.ID)
	first.Candidate.EffectKinds[0] = "praxis.tool/cancel"
	first.Candidate.Payload.Inline[0] = 'x'
	second, _ := controller.Inspect(candidate.ID)
	if second.Candidate.EffectKinds[0] != "praxis.tool/execute" || second.Candidate.Payload.Inline[0] == 'x' {
		t.Fatal("action controller exposed internal candidate storage")
	}
}

func TestWhiteboxReserveRejectsCandidateOutsideCurrentWindow(t *testing.T) {
	for name, now := range map[string]time.Time{
		"not-yet-current": testkit.FixedTime.Add(-time.Nanosecond),
		"expired":         testkit.FixedTime.Add(time.Minute),
	} {
		t.Run(name, func(t *testing.T) {
			controller := action.NewController()
			candidate := testkit.Candidate()
			if _, err := controller.PutCandidate(candidate, candidate.PendingActionDigest); err != nil {
				t.Fatal(err)
			}
			if _, err := controller.Reserve(candidate.ID, testkit.Digest("app"), testkit.Digest("intent"), testkit.Digest("subject"), candidate.SessionID, now, now.Add(time.Minute)); err == nil {
				t.Fatalf("%s ActionCandidate was reserved", name)
			}
			record, ok := controller.Inspect(candidate.ID)
			if !ok || record.State != action.StateCandidate || record.Revision != 1 || record.Reservation != nil {
				t.Fatalf("failed reserve changed state: %+v", record)
			}
		})
	}
}

func TestWhiteboxExpiredRetryCannotReuseReservation(t *testing.T) {
	controller := action.NewController()
	candidate := testkit.Candidate()
	if _, err := controller.PutCandidate(candidate, candidate.PendingActionDigest); err != nil {
		t.Fatal(err)
	}
	first, err := controller.Reserve(candidate.ID, testkit.Digest("app"), testkit.Digest("intent"), testkit.Digest("subject"), candidate.SessionID, testkit.FixedTime, testkit.FixedTime.Add(30*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := controller.Reserve(candidate.ID, testkit.Digest("app"), testkit.Digest("intent"), testkit.Digest("subject"), candidate.SessionID, time.Unix(0, candidate.ExpiresUnixNano), testkit.FixedTime.Add(30*time.Second)); err == nil {
		t.Fatal("expired retry reused an existing reservation")
	}
	record, _ := controller.Inspect(candidate.ID)
	if record.Revision != first.Revision || record.Reservation == nil || record.Reservation.Digest != first.Reservation.Digest {
		t.Fatal("expired retry changed the existing reservation")
	}
}

func TestWhiteboxReservationCannotOutliveCandidate(t *testing.T) {
	controller := action.NewController()
	candidate := testkit.Candidate()
	if _, err := controller.PutCandidate(candidate, candidate.PendingActionDigest); err != nil {
		t.Fatal(err)
	}
	candidateExpiry := time.Unix(0, candidate.ExpiresUnixNano)
	tooLong := candidateExpiry.Add(time.Nanosecond)
	if _, err := controller.Reserve(candidate.ID, testkit.Digest("app"), testkit.Digest("intent"), testkit.Digest("subject"), candidate.SessionID, testkit.FixedTime, tooLong); err == nil {
		t.Fatal("Reservation longer than its ActionCandidate was accepted")
	}
	unchanged, ok := controller.Inspect(candidate.ID)
	if !ok || unchanged.State != action.StateCandidate || unchanged.Revision != 1 || unchanged.Reservation != nil {
		t.Fatalf("overlong Reservation request changed state: %+v", unchanged)
	}
	valid, err := controller.Reserve(candidate.ID, testkit.Digest("app"), testkit.Digest("intent"), testkit.Digest("subject"), candidate.SessionID, testkit.FixedTime, candidateExpiry)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := controller.Reserve(candidate.ID, testkit.Digest("app"), testkit.Digest("intent"), testkit.Digest("subject"), candidate.SessionID, testkit.FixedTime, tooLong); err == nil {
		t.Fatal("overlong Reservation retry bypassed the Candidate TTL")
	}
	afterRetry, _ := controller.Inspect(candidate.ID)
	if afterRetry.Revision != valid.Revision || afterRetry.Reservation == nil || afterRetry.Reservation.Digest != valid.Reservation.Digest {
		t.Fatal("overlong Reservation retry changed the existing fact")
	}
}

func TestWhiteboxConcurrentReserveCurrentness(t *testing.T) {
	controller := action.NewController()
	candidate := testkit.Candidate()
	if _, err := controller.PutCandidate(candidate, candidate.PendingActionDigest); err != nil {
		t.Fatal(err)
	}
	const workers = 64
	var wg sync.WaitGroup
	results := make(chan error, workers)
	for i := range workers {
		wg.Add(1)
		go func(valid bool) {
			defer wg.Done()
			now := time.Unix(0, candidate.ExpiresUnixNano)
			if valid {
				now = testkit.FixedTime
			}
			_, err := controller.Reserve(candidate.ID, testkit.Digest("app"), testkit.Digest("intent"), testkit.Digest("subject"), candidate.SessionID, now, testkit.FixedTime.Add(30*time.Second))
			results <- err
		}(i%2 == 0)
	}
	wg.Wait()
	close(results)
	successes, failures := 0, 0
	for err := range results {
		if err == nil {
			successes++
		} else {
			failures++
		}
	}
	if successes != workers/2 || failures != workers/2 {
		t.Fatalf("want %d current successes and %d expired failures, got %d/%d", workers/2, workers/2, successes, failures)
	}
	record, _ := controller.Inspect(candidate.ID)
	if record.State != action.StateReserved || record.Revision != 2 || record.Reservation == nil {
		t.Fatalf("concurrent reserve currentness produced invalid state: %+v", record)
	}
}

func TestWhiteboxConcurrentPendingActionBindingCreateOnce(t *testing.T) {
	controller := action.NewController()
	candidate := testkit.Candidate()
	const workers = 64
	var wg sync.WaitGroup
	results := make(chan error, workers)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := controller.PutCandidate(candidate, candidate.PendingActionDigest)
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatal(err)
		}
	}
	record, ok := controller.Inspect(candidate.ID)
	if !ok || record.Revision != 1 || record.State != action.StateCandidate || record.Candidate.Digest != candidate.Digest {
		t.Fatalf("concurrent PendingAction binding was not create-once: %+v", record)
	}
}
