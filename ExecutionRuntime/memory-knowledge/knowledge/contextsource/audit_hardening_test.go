package contextsource

import (
	"errors"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
)

type exactReadResult struct {
	observation ExactContentObservation
	body        []byte
	err         error
}

func TestKnowledgeCurrentReaderFreshOwnerClockAndTurnBinding(t *testing.T) {
	t.Run("owner now expired caller old", func(t *testing.T) {
		f := newReaderFixture(t)
		callerChecked := f.request.CheckedAt
		f.now = f.attempt.ExpiresAt
		req := f.request
		req.CheckedAt = callerChecked
		req.NotAfter = f.now.Add(time.Minute)
		if _, err := f.store.InspectForTurn(req); !errors.Is(err, contract.ErrNotCurrent) {
			t.Fatalf("expired owner facts accepted with old caller time: %v", err)
		}
	})

	t.Run("exact read uses fresh owner now", func(t *testing.T) {
		f := newReaderFixture(t)
		projection, err := f.store.InspectForTurn(f.request)
		if err != nil {
			t.Fatal(err)
		}
		f.now = projection.ExpiresAt
		if _, _, err := f.store.ReadContentExact(exactRequest(f, projection, f.now.Add(time.Minute))); !errors.Is(err, contract.ErrNotCurrent) {
			t.Fatalf("expired projection accepted with old caller time: %v", err)
		}
	})

	t.Run("clock rollback fails closed", func(t *testing.T) {
		f := newReaderFixture(t)
		base := f.now
		f.now = base.Add(10 * time.Minute)
		if _, err := f.store.InspectForTurn(f.request); err != nil {
			t.Fatal(err)
		}
		f.now = base.Add(5 * time.Minute)
		if _, err := f.store.InspectForTurn(f.request); !errors.Is(err, contract.ErrNotCurrent) {
			t.Fatalf("clock rollback accepted: %v", err)
		}
	})

	t.Run("cross turn projection replay", func(t *testing.T) {
		f := newReaderFixture(t)
		req := f.request
		req.TurnID = "turn-2"
		req.Coordinate.TurnID = "turn-2"
		if _, err := f.store.InspectForTurn(req); !errors.Is(err, contract.ErrEvidenceConflict) {
			t.Fatalf("cross-turn InspectForTurn accepted: %v", err)
		}
		projection, err := f.store.InspectForTurn(f.request)
		if err != nil {
			t.Fatal(err)
		}
		exact := exactRequest(f, projection, f.now.Add(time.Minute))
		exact.TurnID = "turn-2"
		if _, _, err := f.store.ReadContentExact(exact); err == nil {
			t.Fatal("cross-turn exact read accepted")
		}
		projection.TurnID = "turn-2"
		exact = exactRequest(f, projection, f.now.Add(time.Minute))
		if _, _, err := f.store.ReadContentExact(exact); !errors.Is(err, contract.ErrEvidenceConflict) {
			t.Fatalf("turn-tampered projection accepted: %v", err)
		}
	})

	t.Run("InspectAttempt binds exact run and turn", func(t *testing.T) {
		f := newReaderFixture(t)
		valid, err := f.store.InspectAttempt(coordinateFrom(f.attempt))
		if err != nil || valid.RunID != f.attempt.RunID || valid.TurnID != f.attempt.TurnID {
			t.Fatalf("inspection=%+v err=%v", valid, err)
		}
		wrongRun := coordinateFrom(f.attempt)
		wrongRun.RunID = "run-b"
		if _, err := f.store.InspectAttempt(wrongRun); !errors.Is(err, contract.ErrEvidenceConflict) {
			t.Fatalf("cross-Run inspection accepted: %v", err)
		}
		wrongTurn := coordinateFrom(f.attempt)
		wrongTurn.TurnID = "turn-2"
		if _, err := f.store.InspectAttempt(wrongTurn); !errors.Is(err, contract.ErrEvidenceConflict) {
			t.Fatalf("cross-Turn inspection accepted: %v", err)
		}
	})
}

func TestKnowledgeCurrentReaderCanonicalDiscriminatorsAndSemanticKeys(t *testing.T) {
	t.Run("version and object kind", func(t *testing.T) {
		f := newReaderFixture(t)
		badAttempt := cloneAttempt(f.attempt)
		badAttempt.Ref = contract.Ref{ID: badAttempt.Ref.ID, Revision: 2}
		badAttempt.ContractVersion = ContractVersion + "/v2"
		if _, err := f.store.PutAttempt(badAttempt, contract.ExpectRevision(1)); err == nil {
			t.Fatal("cross-version attempt accepted")
		}
		badState := cloneCurrentState(f.state)
		badState.Ref = contract.Ref{ID: badState.Ref.ID, Revision: 2}
		badState.ObjectKind = "memory_current_state"
		if _, err := f.store.PublishCurrent(badState, contract.ExpectRevision(1)); err == nil {
			t.Fatal("type-punned current state accepted")
		}
		projection, err := f.store.InspectForTurn(f.request)
		if err != nil {
			t.Fatal(err)
		}
		wrongProjection := projection
		wrongProjection.ObjectKind = "memory_contribution_current_projection"
		if _, _, err := f.store.ReadContentExact(exactRequest(f, wrongProjection, f.now.Add(time.Minute))); err == nil {
			t.Fatal("type-punned projection accepted")
		}
		observation, _, err := f.store.ReadContentExact(exactRequest(f, projection, f.now.Add(time.Minute)))
		if err != nil {
			t.Fatal(err)
		}
		observation.ContractVersion = ContractVersion + "/v2"
		if _, err := sealContentObservation(observation); err == nil {
			t.Fatal("cross-version content observation accepted")
		}
	})

	t.Run("nested refs and duplicate semantic record", func(t *testing.T) {
		f := newReaderFixture(t)
		bad := cloneAttempt(f.attempt)
		bad.Ref = contract.Ref{ID: bad.Ref.ID, Revision: 2}
		bad.Items[0].EvidenceRefs[0] = contract.Ref{}
		if _, err := f.store.PutAttempt(bad, contract.ExpectRevision(1)); err == nil {
			t.Fatal("invalid nested Evidence ref accepted")
		}

		duplicate := cloneAttempt(f.attempt)
		duplicate.Ref = contract.Ref{ID: duplicate.Ref.ID, Revision: 2}
		second := duplicate.Items[0]
		second.RecordRef = testRefRevision(second.RecordRef.ID, 2)
		duplicate.Items = append(duplicate.Items, second)
		if _, err := f.store.PutAttempt(duplicate, contract.ExpectRevision(1)); err == nil {
			t.Fatal("duplicate semantic Record key accepted")
		}

		badCurrent := cloneCurrentState(f.state)
		badCurrent.Ref = contract.Ref{ID: badCurrent.Ref.ID, Revision: 2}
		badCurrent.Items[0].SourceRefs[0] = contract.Ref{}
		if _, err := f.store.PublishCurrent(badCurrent, contract.ExpectRevision(1)); err == nil {
			t.Fatal("invalid nested current Source ref accepted")
		}
	})

	t.Run("rank derived from semantic order", func(t *testing.T) {
		f := newReaderFixture(t)
		candidate := cloneAttempt(f.attempt)
		candidate.Ref = contract.Ref{ID: candidate.Ref.ID, Revision: 2}
		low := candidate.Items[0]
		low.Rank = 99
		low.Score = 1
		low.RecordRef = testRef("knowledge/record-low")
		high := candidate.Items[0]
		high.Rank = 77
		high.Score = 100
		high.RecordRef = testRef("knowledge/record-high")
		candidate.Items = []StoredItem{low, high}
		sealed, err := f.store.PutAttempt(candidate, contract.ExpectRevision(1))
		if err != nil {
			t.Fatal(err)
		}
		if sealed.Items[0].RecordRef.ID != "knowledge/record-high" || sealed.Items[0].Rank != 0 || sealed.Items[1].Rank != 1 {
			t.Fatalf("caller rank influenced canonical order: %+v", sealed.Items)
		}
	})
}

func TestKnowledgeCurrentReaderStatePlaneBindingAndInspectClassification(t *testing.T) {
	t.Run("remote or tampered binding rejected", func(t *testing.T) {
		now := time.Date(2026, 7, 16, 2, 0, 0, 0, time.UTC)
		binding, err := NewStatePlaneBinding("knowledge/binding", 1, "knowledge-owner-state-plane", now.Add(time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		remote := newLocalContent(binding)
		remote.binding.RemoteLocator = "https://remote.invalid/content"
		if _, err := NewStore(contract.ClockFunc(func() time.Time { return now }), remote); err == nil {
			t.Fatal("remote reader injection accepted")
		}
		tampered := newLocalContent(binding)
		tampered.binding.StoreDomain = "other-store"
		if _, err := NewStore(contract.ClockFunc(func() time.Time { return now }), tampered); err == nil {
			t.Fatal("tampered State Plane proof accepted")
		}
	})

	t.Run("live binding drift fails exact read", func(t *testing.T) {
		f := newReaderFixture(t)
		projection, err := f.store.InspectForTurn(f.request)
		if err != nil {
			t.Fatal(err)
		}
		f.content.binding.RemoteLocator = "remote://drift"
		if _, _, err := f.store.ReadContentExact(exactRequest(f, projection, f.now.Add(time.Minute))); !errors.Is(err, contract.ErrContextUnmaterialized) {
			t.Fatalf("binding drift accepted: %v", err)
		}
	})

	t.Run("missing id differs from exact ref drift", func(t *testing.T) {
		f := newReaderFixture(t)
		missing := coordinateFrom(f.attempt)
		missing.AttemptRef = testRef("knowledge/missing-attempt")
		inspection, err := f.store.InspectAttempt(missing)
		if err != nil || inspection.Status != AttemptNotPersisted {
			t.Fatalf("missing id inspection=%+v err=%v", inspection, err)
		}
		drift := coordinateFrom(f.attempt)
		drift.AttemptRef = testRefRevision(f.attempt.Ref.ID, f.attempt.Ref.Revision+1)
		if _, err := f.store.InspectAttempt(drift); !errors.Is(err, contract.ErrEvidenceConflict) {
			t.Fatalf("same-id ref drift classified as absent: %v", err)
		}
	})
}

func TestKnowledgeCurrentReaderBarrierTTLAndBinding(t *testing.T) {
	t.Run("InspectAttempt lock wait crosses ttl", func(t *testing.T) {
		f := newReaderFixture(t)
		started := make(chan struct{})
		result := make(chan error, 1)
		f.store.mu.Lock()
		go func() {
			close(started)
			_, err := f.store.InspectAttempt(coordinateFrom(f.attempt))
			result <- err
		}()
		<-started
		f.now = f.attempt.ExpiresAt
		f.store.mu.Unlock()
		if err := <-result; !errors.Is(err, contract.ErrNotCurrent) {
			t.Fatalf("InspectAttempt used pre-wait time: %v", err)
		}
	})

	t.Run("lock wait crosses ttl", func(t *testing.T) {
		f := newReaderFixture(t)
		started := make(chan struct{})
		result := make(chan error, 1)
		f.store.mu.Lock()
		go func() {
			close(started)
			_, err := f.store.InspectForTurn(f.request)
			result <- err
		}()
		<-started
		f.now = f.attempt.ExpiresAt
		f.store.mu.Unlock()
		if err := <-result; !errors.Is(err, contract.ErrNotCurrent) {
			t.Fatalf("lock wait crossed TTL but became current: %v", err)
		}
	})

	t.Run("Get crosses ttl returns no body", func(t *testing.T) {
		f := newReaderFixture(t)
		projection, err := f.store.InspectForTurn(f.request)
		if err != nil {
			t.Fatal(err)
		}
		started, release := f.content.blockGets()
		result := make(chan exactReadResult, 1)
		go func() {
			observation, body, err := f.store.ReadContentExact(exactRequest(f, projection, projection.ExpiresAt.Add(time.Minute)))
			result <- exactReadResult{observation: observation, body: body, err: err}
		}()
		<-started
		f.now = projection.ExpiresAt
		close(release)
		got := <-result
		if !errors.Is(got.err, contract.ErrNotCurrent) || len(got.body) != 0 || got.observation.Ref.ID != "" {
			t.Fatalf("Get crossed TTL: observation=%+v body=%q err=%v", got.observation, got.body, got.err)
		}
	})

	t.Run("Get binding drift returns no body", func(t *testing.T) {
		f := newReaderFixture(t)
		projection, err := f.store.InspectForTurn(f.request)
		if err != nil {
			t.Fatal(err)
		}
		started, release := f.content.blockGets()
		result := make(chan exactReadResult, 1)
		go func() {
			observation, body, err := f.store.ReadContentExact(exactRequest(f, projection, f.now.Add(time.Minute)))
			result <- exactReadResult{observation: observation, body: body, err: err}
		}()
		<-started
		drift, err := NewStatePlaneBinding("knowledge/state-plane-binding", 2, "knowledge-owner-state-plane", f.now.Add(time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		drift.RemoteLocator = "remote://changed-during-get"
		f.content.setBinding(drift)
		close(release)
		got := <-result
		if !errors.Is(got.err, contract.ErrContextUnmaterialized) || len(got.body) != 0 || got.observation.Ref.ID != "" {
			t.Fatalf("binding drift during Get: observation=%+v body=%q err=%v", got.observation, got.body, got.err)
		}
	})

	t.Run("Get binding revision drift returns no body", func(t *testing.T) {
		f := newReaderFixture(t)
		projection, err := f.store.InspectForTurn(f.request)
		if err != nil {
			t.Fatal(err)
		}
		started, release := f.content.blockGets()
		result := make(chan exactReadResult, 1)
		go func() {
			observation, body, err := f.store.ReadContentExact(exactRequest(f, projection, f.now.Add(time.Minute)))
			result <- exactReadResult{observation: observation, body: body, err: err}
		}()
		<-started
		drift, err := NewStatePlaneBinding("knowledge/state-plane-binding", 2, "knowledge-owner-state-plane", f.now.Add(time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		f.content.setBinding(drift)
		close(release)
		got := <-result
		if !errors.Is(got.err, contract.ErrContextUnmaterialized) || len(got.body) != 0 || got.observation.Ref.ID != "" {
			t.Fatalf("binding revision drift during Get: observation=%+v body=%q err=%v", got.observation, got.body, got.err)
		}
	})
}

func TestKnowledgeStatePlaneContentStoreCopyAndEvict(t *testing.T) {
	now := time.Date(2026, 7, 16, 2, 0, 0, 0, time.UTC)
	binding, err := NewStatePlaneBinding("knowledge/reference-binding", 1, "knowledge-owner-state-plane", now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewStatePlaneContentStore(binding)
	if err != nil {
		t.Fatal(err)
	}
	body := []byte("immutable knowledge bytes")
	ref, err := store.PutExact(body, "text/plain")
	if err != nil {
		t.Fatal(err)
	}
	body[0] = 'X'
	first, err := store.Get(ref)
	if err != nil || string(first) != "immutable knowledge bytes" {
		t.Fatalf("first=%q err=%v", first, err)
	}
	first[0] = 'Y'
	second, err := store.Get(ref)
	if err != nil || string(second) != "immutable knowledge bytes" {
		t.Fatalf("copy isolation failed: %q %v", second, err)
	}
	store.EvictExact(ref)
	if _, err := store.Get(ref); !errors.Is(err, contract.ErrNotFound) {
		t.Fatalf("evicted reference remained readable: %v", err)
	}
}
