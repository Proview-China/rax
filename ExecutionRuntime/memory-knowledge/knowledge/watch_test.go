package knowledge

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
)

func TestKnowledgeWatchSourceRecordAndWithdrawalWithExactCursor(t *testing.T) {
	f := newFixture(t, true)
	view, _ := buildPublishedView(t, f)
	request := contract.WatchRequestV1{ViewRef: view.Ref, Limit: 1, ExpiresAt: f.now.Add(time.Minute)}
	first, err := f.store.WatchChanges(f.access, request)
	if err != nil || len(first.Events) != 1 || first.Events[0].Kind != contract.ChangeSourceRegistered || !contract.SameRef(first.Events[0].SubjectRef, f.source.Ref) {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	retry, err := f.store.WatchChanges(f.access, request)
	if err != nil || retry.Digest != first.Digest {
		t.Fatalf("lost reply page changed: %s/%s %v", first.Digest, retry.Digest, err)
	}
	request.Cursor = &first.NextCursor
	second, err := f.store.WatchChanges(f.access, request)
	if err != nil || len(second.Events) != 1 || second.Events[0].Kind != contract.ChangeRecordCommitted || !contract.SameRef(second.Events[0].SubjectRef, f.record.Ref) {
		t.Fatalf("second=%+v err=%v", second, err)
	}
	withdrawn, _, err := f.store.WithdrawSource(f.access, f.source.Ref.ID, "revoked", contract.ExpectRevision(f.source.Ref.Revision))
	if err != nil {
		t.Fatal(err)
	}
	request.Cursor = &second.NextCursor
	third, err := f.store.WatchChanges(f.access, request)
	if err != nil || len(third.Events) != 1 || third.Events[0].Kind != contract.ChangeSourceWithdrawn || !contract.SameRef(third.Events[0].SubjectRef, withdrawn.Ref) {
		t.Fatalf("third=%+v err=%v", third, err)
	}
}

func TestKnowledgeWatchTamperCrossPolicyAndTTLFailClosed(t *testing.T) {
	f := newFixture(t, true)
	view, _ := buildPublishedView(t, f)
	page, err := f.store.WatchChanges(f.access, contract.WatchRequestV1{ViewRef: view.Ref, Limit: 10, ExpiresAt: f.now.Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	tampered := page.NextCursor
	tampered.Sequence++
	request := contract.WatchRequestV1{ViewRef: view.Ref, Cursor: &tampered, Limit: 10, ExpiresAt: f.now.Add(time.Minute)}
	if _, err := f.store.WatchChanges(f.access, request); !errors.Is(err, contract.ErrInvalidArgument) && !errors.Is(err, contract.ErrEvidenceConflict) {
		t.Fatalf("tampered cursor accepted: %v", err)
	}
	other := page.NextCursor
	other.PolicyRef = ref("other-policy")
	other, err = contract.SealWatchCursorV1(other)
	if err != nil {
		t.Fatal(err)
	}
	request.Cursor = &other
	if _, err := f.store.WatchChanges(f.access, request); !errors.Is(err, contract.ErrScopeDenied) {
		t.Fatalf("cross-policy cursor accepted: %v", err)
	}
	*f.now = page.NextCursor.ExpiresAt
	request.Cursor = &page.NextCursor
	request.ExpiresAt = f.now.Add(time.Minute)
	if _, err := f.store.WatchChanges(f.access, request); !errors.Is(err, contract.ErrNotCurrent) {
		t.Fatalf("expired cursor accepted: %v", err)
	}
}

func TestKnowledgeWatch64ConcurrentReadersSeeOneExactPage(t *testing.T) {
	f := newFixture(t, true)
	view, _ := buildPublishedView(t, f)
	request := contract.WatchRequestV1{ViewRef: view.Ref, Limit: 10, ExpiresAt: f.now.Add(time.Minute)}
	const workers = 64
	digests := make(chan string, workers)
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			page, err := f.store.WatchChanges(f.access, request)
			if err != nil {
				errs <- err
				return
			}
			digests <- page.Digest
		}()
	}
	wg.Wait()
	close(errs)
	close(digests)
	for err := range errs {
		t.Fatal(err)
	}
	first := ""
	for digest := range digests {
		if first == "" {
			first = digest
		} else if digest != first {
			t.Fatalf("concurrent page drift: %s != %s", digest, first)
		}
	}
}
