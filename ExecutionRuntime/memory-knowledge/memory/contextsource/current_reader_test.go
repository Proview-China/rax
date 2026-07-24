package contextsource

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
)

type localContent struct {
	mu         sync.RWMutex
	items      map[string][]byte
	binding    StatePlaneBinding
	getStarted chan struct{}
	getRelease chan struct{}
	getOnce    sync.Once
}

func newLocalContent(binding StatePlaneBinding) *localContent {
	return &localContent{items: make(map[string][]byte), binding: binding}
}

func (*localContent) ownerLocalStatePlaneReader() {}

func (c *localContent) StatePlaneBinding() StatePlaneBinding {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.binding
}

func (c *localContent) put(body []byte) contract.ContentRef {
	sum := sha256.Sum256(body)
	digest := "sha256:" + hex.EncodeToString(sum[:])
	c.mu.Lock()
	c.items[digest] = bytes.Clone(body)
	c.mu.Unlock()
	return contract.ContentRef{ID: digest, Digest: digest, Length: int64(len(body)), MediaType: "text/plain"}
}

func (c *localContent) Get(ref contract.ContentRef) ([]byte, error) {
	c.mu.RLock()
	body, ok := c.items[ref.ID]
	body = bytes.Clone(body)
	started, release := c.getStarted, c.getRelease
	c.mu.RUnlock()
	if started != nil {
		c.getOnce.Do(func() { close(started) })
		<-release
	}
	if !ok {
		return nil, contract.ErrNotFound
	}
	return body, nil
}

func (c *localContent) blockGets() (<-chan struct{}, chan<- struct{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.getStarted = make(chan struct{})
	c.getRelease = make(chan struct{})
	c.getOnce = sync.Once{}
	return c.getStarted, c.getRelease
}

func (c *localContent) setBinding(binding StatePlaneBinding) {
	c.mu.Lock()
	c.binding = binding
	c.mu.Unlock()
}

func (c *localContent) evict(ref contract.ContentRef) {
	c.mu.Lock()
	delete(c.items, ref.ID)
	c.mu.Unlock()
}

type readerFixture struct {
	now     time.Time
	content *localContent
	store   *Store
	attempt LocalAttempt
	state   CurrentState
	request CurrentRequest
}

func newReaderFixture(t *testing.T) *readerFixture {
	t.Helper()
	f := &readerFixture{now: time.Date(2026, 7, 16, 2, 0, 0, 0, time.UTC)}
	binding, err := NewStatePlaneBinding("memory/state-plane-binding", 1, "memory-owner-state-plane", f.now.Add(20*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	f.content = newLocalContent(binding)
	f.store, err = NewStore(contract.ClockFunc(func() time.Time { return f.now }), f.content)
	if err != nil {
		t.Fatal(err)
	}
	contentRef := f.content.put([]byte("owner local memory content"))
	resultRef := testRef("memory/retrieval-result")
	domainResult, err := contract.NewDomainResultFact(contract.OwnerMemory, "memory/domain-result", "memory/local-attempt", testRef("memory/operation"), resultRef, testRef("memory/inspection"), 0, 1, nil, contract.Coverage{Status: contract.CoverageComplete, Expected: 1, Available: 1}, "local_complete", nil, f.now)
	if err != nil {
		t.Fatal(err)
	}
	association, err := contract.AssociateDomainResult(domainResult)
	if err != nil {
		t.Fatal(err)
	}
	application, err := contract.NewSettlementApplication(contract.OwnerMemory, "memory/application", 1, domainResult, association, contract.RuntimeSettlementRef{Ref: testRef("memory/runtime-settlement")}, f.now)
	if err != nil {
		t.Fatal(err)
	}
	recordRef := testRef("memory/record")
	projectionRef := testRef("memory/projection")
	viewRef := testRef("memory/view")
	watermarkRef := testRef("memory/watermark")
	f.attempt, err = f.store.PutAttempt(LocalAttempt{
		Ref: contract.Ref{ID: "memory/local-attempt", Revision: 1}, TenantID: "tenant-a", IdentityID: "identity-a",
		ExecutionScopeDigest: "sha256:execution", RunID: "run-a", TurnID: "turn-1", RequestDigest: "sha256:request", IdempotencyKey: "memory-idempotency",
		ObservationRef: testRef("memory/observation"), ResultRef: resultRef, QueryRef: testRef("memory/query"), ViewRef: viewRef, WatermarkRef: watermarkRef,
		AuthorityRef: testRef("authority"), AuthorityEpoch: 7, PolicyRef: testRef("policy"), Purpose: "assist", Scopes: []string{"identity_private"}, SensitivityMax: "internal",
		Coverage:     contract.Coverage{Status: contract.CoverageComplete, Expected: 1, Available: 1, ProjectionRefs: []contract.Ref{projectionRef}},
		Items:        []StoredItem{{Rank: 0, Score: 10, RecordRef: recordRef, ContentRef: contentRef, SourceRefs: []contract.Ref{testRef("source")}, EvidenceRefs: []contract.Ref{testRef("evidence")}, ProjectionRefs: []contract.Ref{projectionRef}, CitationDigest: testRef("citation").Digest, RecordExpiresAt: f.now.Add(time.Hour), ProjectionExpires: f.now.Add(time.Hour)}},
		DomainResult: domainResult, Association: association, Application: application, ObservedAt: f.now, ExpiresAt: f.now.Add(time.Hour),
	}, contract.ExpectAbsent())
	if err != nil {
		t.Fatal(err)
	}
	f.state, err = f.store.PublishCurrent(CurrentState{
		Ref: contract.Ref{ID: "memory/current", Revision: 1}, TenantID: "tenant-a", IdentityID: "identity-a", AuthorityRef: testRef("authority"), AuthorityEpoch: 7, PolicyRef: testRef("policy"), Purpose: "assist",
		Scopes: []string{"identity_private"}, SensitivityMax: "internal", ViewRef: viewRef, WatermarkRef: watermarkRef,
		Items: []CurrentItem{{RecordRef: recordRef, ContentRef: contentRef, ProjectionRefs: []contract.Ref{projectionRef}, Active: true, PoisoningCleared: true, RecordExpiresAt: f.now.Add(time.Hour), ProjectionExpires: f.now.Add(time.Hour)}}, ExpiresAt: f.now.Add(time.Hour),
	}, contract.ExpectAbsent())
	if err != nil {
		t.Fatal(err)
	}
	f.request = CurrentRequest{ContractVersion: ContractVersion, Coordinate: coordinateFrom(f.attempt), RunID: "run-a", TurnID: "turn-1", CurrentStateRef: f.state.Ref, AuthorityRef: testRef("authority"), AuthorityEpoch: 7, PolicyRef: testRef("policy"), Purpose: "assist", Scopes: []string{"identity_private"}, SensitivityMax: "internal", CheckedAt: f.now, NotAfter: f.now.Add(30 * time.Minute), ProjectionID: "memory/contribution", ProjectionRevision: 1}
	return f
}

func exactRequest(f *readerFixture, projection CurrentProjection, notAfter time.Time) ExactContentRequest {
	return ExactContentRequest{ContractVersion: ContractVersion, Projection: projection, RunID: projection.RunID, TurnID: projection.TurnID, Rank: 0, CheckedAt: f.request.CheckedAt, NotAfter: notAfter}
}

func TestMemoryCurrentReaderExactCanonicalAndCopyIsolation(t *testing.T) {
	f := newReaderFixture(t)
	inspection, err := f.store.InspectAttempt(coordinateFrom(f.attempt))
	if err != nil || inspection.Status != AttemptPersistedAndSettled || inspection.Ref.Validate() != nil {
		t.Fatalf("inspection=%+v err=%v", inspection, err)
	}
	projection, err := f.store.InspectForTurn(f.request)
	if err != nil || !projection.Current || projection.Ref.Validate() != nil || len(projection.Items) != 1 {
		t.Fatalf("projection=%+v err=%v", projection, err)
	}
	observation, body, err := f.store.ReadContentExact(exactRequest(f, projection, f.now.Add(20*time.Minute)))
	if err != nil || observation.Ref.Validate() != nil || string(body) != "owner local memory content" || observation.ObservedDigest != projection.Items[0].ContentRef.Digest {
		t.Fatalf("observation=%+v body=%q err=%v", observation, body, err)
	}
	projection.Items[0].SourceRefs[0] = testRef("mutated")
	projection.Scopes[0] = "mutated"
	body[0] = 'X'
	again, err := f.store.InspectForTurn(f.request)
	if err != nil || again.Scopes[0] != "identity_private" || again.Items[0].SourceRefs[0].ID != "source" {
		t.Fatalf("caller mutated owner snapshot: %+v err=%v", again, err)
	}
	_, againBody, err := f.store.ReadContentExact(exactRequest(f, again, f.now.Add(20*time.Minute)))
	if err != nil || string(againBody) != "owner local memory content" {
		t.Fatalf("caller mutated local content: %q %v", againBody, err)
	}
}

func TestMemoryCurrentReaderFailClosedCases(t *testing.T) {
	t.Run("ttl boundary", func(t *testing.T) {
		f := newReaderFixture(t)
		f.request.CheckedAt = f.attempt.ExpiresAt
		f.request.NotAfter = f.request.CheckedAt.Add(time.Minute)
		if _, err := f.store.InspectForTurn(f.request); !errors.Is(err, contract.ErrNotCurrent) {
			t.Fatalf("now==expires accepted: %v", err)
		}
	})
	t.Run("historical is not current", func(t *testing.T) {
		f := newReaderFixture(t)
		next := cloneCurrentState(f.state)
		next.Ref = contract.Ref{ID: f.state.Ref.ID, Revision: 2}
		next.Items[0].RecordRef = testRefRevision("memory/record", 2)
		next.Items[0].ContentRef = f.content.put([]byte("corrected"))
		if _, err := f.store.PublishCurrent(next, contract.ExpectRevision(1)); err != nil {
			t.Fatal(err)
		}
		if _, err := f.store.InspectForTurn(f.request); !errors.Is(err, contract.ErrNotCurrent) {
			t.Fatalf("historical state accepted: %v", err)
		}
	})
	t.Run("stale projection and tombstone", func(t *testing.T) {
		f := newReaderFixture(t)
		next := cloneCurrentState(f.state)
		next.Ref = contract.Ref{ID: f.state.Ref.ID, Revision: 2}
		next.Items[0].Active = false
		next.Items[0].ProjectionRefs = []contract.Ref{testRefRevision("memory/projection", 2)}
		next, err := f.store.PublishCurrent(next, contract.ExpectRevision(1))
		if err != nil {
			t.Fatal(err)
		}
		req := f.request
		req.CurrentStateRef = next.Ref
		if _, err := f.store.InspectForTurn(req); !errors.Is(err, contract.ErrNotCurrent) {
			t.Fatalf("tombstoned/stale item accepted: %v", err)
		}
	})
	t.Run("evicted local bytes", func(t *testing.T) {
		f := newReaderFixture(t)
		projection, err := f.store.InspectForTurn(f.request)
		if err != nil {
			t.Fatal(err)
		}
		f.content.evict(projection.Items[0].ContentRef)
		if _, _, err := f.store.ReadContentExact(exactRequest(f, projection, f.now.Add(time.Minute))); !errors.Is(err, contract.ErrContextUnmaterialized) {
			t.Fatalf("evicted bytes accepted: %v", err)
		}
	})
	t.Run("canonical tamper", func(t *testing.T) {
		f := newReaderFixture(t)
		f.store.mu.Lock()
		versions := f.store.attempts[f.attempt.Ref.ID]
		versions[0].Items[0].CitationDigest = "sha256:tampered"
		f.store.attempts[f.attempt.Ref.ID] = versions
		f.store.mu.Unlock()
		if _, err := f.store.InspectAttempt(coordinateFrom(f.attempt)); !errors.Is(err, contract.ErrEvidenceConflict) {
			t.Fatalf("tamper accepted: %v", err)
		}
	})
	t.Run("current state canonical tamper", func(t *testing.T) {
		f := newReaderFixture(t)
		f.store.mu.Lock()
		versions := f.store.states[f.state.Ref.ID]
		versions[0].Items[0].Active = false
		f.store.states[f.state.Ref.ID] = versions
		f.store.mu.Unlock()
		if _, err := f.store.InspectForTurn(f.request); !errors.Is(err, contract.ErrEvidenceConflict) {
			t.Fatalf("current state tamper accepted: %v", err)
		}
	})
	t.Run("settlement association drift", func(t *testing.T) {
		f := newReaderFixture(t)
		bad := cloneAttempt(f.attempt)
		bad.Ref = contract.Ref{ID: bad.Ref.ID, Revision: 2}
		bad.Association.DomainResultRef = testRef("memory/wrong-domain-result")
		bad, err := f.store.PutAttempt(bad, contract.ExpectRevision(1))
		if err != nil {
			t.Fatal(err)
		}
		inspection, err := f.store.InspectAttempt(coordinateFrom(bad))
		if err != nil || inspection.Status != AttemptPersistedUnsettled {
			t.Fatalf("inspection=%+v err=%v", inspection, err)
		}
		req := f.request
		req.Coordinate = coordinateFrom(bad)
		if _, err := f.store.InspectForTurn(req); !errors.Is(err, contract.ErrNotCurrent) {
			t.Fatalf("wrong association became current: %v", err)
		}
	})
}

func TestMemoryCurrentReader64ConcurrentConsistentSnapshot(t *testing.T) {
	f := newReaderFixture(t)
	const workers = 64
	start := make(chan struct{})
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			projection, err := f.store.InspectForTurn(f.request)
			if err != nil {
				if errors.Is(err, contract.ErrNotCurrent) {
					return
				}
				errs <- err
				return
			}
			if !contract.SameRef(projection.Items[0].RecordRef, f.attempt.Items[0].RecordRef) {
				errs <- fmt.Errorf("mixed snapshot record: %v", projection.Items[0].RecordRef)
				return
			}
			_, body, err := f.store.ReadContentExact(exactRequest(f, projection, f.now.Add(time.Minute)))
			if errors.Is(err, contract.ErrNotCurrent) {
				return
			}
			if err != nil || string(body) != "owner local memory content" {
				errs <- fmt.Errorf("read %d: %q %v", i, body, err)
				return
			}
			body[0] = 'X'
		}(i)
	}
	close(start)
	next := cloneCurrentState(f.state)
	next.Ref = contract.Ref{ID: f.state.Ref.ID, Revision: 2}
	next.Items[0].RecordRef = testRefRevision("memory/record", 2)
	next.Items[0].ContentRef = f.content.put([]byte("new current"))
	if _, err := f.store.PublishCurrent(next, contract.ExpectRevision(1)); err != nil {
		t.Fatal(err)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

func testRef(id string) contract.Ref { return testRefRevision(id, 1) }

func testRefRevision(id string, revision uint64) contract.Ref {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s-%d", id, revision)))
	return contract.Ref{ID: id, Revision: revision, Digest: "sha256:" + hex.EncodeToString(sum[:])}
}
