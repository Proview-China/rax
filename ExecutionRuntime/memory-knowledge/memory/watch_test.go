package memory

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
)

func memoryWatchView(t *testing.T, f *fixture, id string) View {
	t.Helper()
	watermark, err := f.store.CurrentWatermark(f.access)
	if err != nil {
		t.Fatal(err)
	}
	view := SealView(View{Ref: contract.Ref{ID: id, Revision: 1}, TenantID: f.access.TenantID, PrincipalID: f.access.IdentityID, AuthorityRef: f.access.AuthorityRef, AuthorityEpoch: f.access.AuthorityEpoch, PolicyRef: f.access.PolicyRef, Purpose: "watch", Scopes: []string{"identity_private"}, SensitivityMax: "internal", WatermarkRef: watermark.Ref, CreatedAt: f.now, ExpiresAt: f.now.Add(time.Hour)})
	view, err = f.store.PublishView(f.access, view, contract.ExpectAbsent())
	if err != nil {
		t.Fatal(err)
	}
	return view
}

func TestMemoryWatchExactCursorPaginationLostReplyAndTamper(t *testing.T) {
	f := newFixture(t)
	_, firstRecord := createMemoryRecord(t, f, "record-a", 1)
	_, secondRecord := createMemoryRecord(t, f, "record-b", 2)
	view := memoryWatchView(t, f, "watch-view")
	request := contract.WatchRequestV1{ViewRef: view.Ref, Limit: 1, ExpiresAt: f.now.Add(time.Minute)}
	first, err := f.store.WatchChanges(f.access, request)
	if err != nil || len(first.Events) != 1 || !contract.SameRef(first.Events[0].SubjectRef, firstRecord.Ref) {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	retry, err := f.store.WatchChanges(f.access, request)
	if err != nil || retry.Digest != first.Digest {
		t.Fatalf("lost reply changed page: first=%s retry=%s err=%v", first.Digest, retry.Digest, err)
	}
	request.Cursor = &first.NextCursor
	second, err := f.store.WatchChanges(f.access, request)
	if err != nil || len(second.Events) != 1 || !contract.SameRef(second.Events[0].SubjectRef, secondRecord.Ref) {
		t.Fatalf("second=%+v err=%v", second, err)
	}
	tampered := first.NextCursor
	tampered.Sequence++
	request.Cursor = &tampered
	if _, err := f.store.WatchChanges(f.access, request); !errors.Is(err, contract.ErrInvalidArgument) && !errors.Is(err, contract.ErrEvidenceConflict) {
		t.Fatalf("tampered cursor accepted: %v", err)
	}
}

func TestMemoryWatchCursorBindingAndTTLFailClosed(t *testing.T) {
	f := newFixture(t)
	createMemoryRecord(t, f, "record", 1)
	view := memoryWatchView(t, f, "watch-view")
	page, err := f.store.WatchChanges(f.access, contract.WatchRequestV1{ViewRef: view.Ref, Limit: 10, ExpiresAt: f.now.Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	forged := page.NextCursor
	forged.ViewRef = ref("other-view")
	request := contract.WatchRequestV1{ViewRef: view.Ref, Cursor: &forged, Limit: 10, ExpiresAt: f.now.Add(time.Minute)}
	if _, err := f.store.WatchChanges(f.access, request); !errors.Is(err, contract.ErrEvidenceConflict) {
		t.Fatalf("forged cursor accepted: %v", err)
	}
	f.now = page.NextCursor.ExpiresAt
	request.Cursor = &page.NextCursor
	request.ExpiresAt = f.now.Add(time.Minute)
	if _, err := f.store.WatchChanges(f.access, request); !errors.Is(err, contract.ErrNotCurrent) {
		t.Fatalf("expired cursor accepted: %v", err)
	}
}

func TestMemoryWatch64ConcurrentReadersSeeOneExactPage(t *testing.T) {
	f := newFixture(t)
	createMemoryRecord(t, f, "record", 1)
	view := memoryWatchView(t, f, "watch-view")
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
