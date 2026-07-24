package contextsource

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
)

func (c *localContent) StatePlaneBindingContext(ctx context.Context) (StatePlaneBinding, error) {
	if err := ctx.Err(); err != nil {
		return StatePlaneBinding{}, err
	}
	return c.StatePlaneBinding(), nil
}

func (c *localContent) GetExact(ctx context.Context, ref contract.ContentRef, maxBodyBytes int64) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	c.mu.RLock()
	body, ok := c.items[ref.ID]
	body = bytes.Clone(body)
	started, release := c.getStarted, c.getRelease
	c.mu.RUnlock()
	if started != nil {
		c.getOnce.Do(func() { close(started) })
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-release:
		}
	}
	if !ok {
		return nil, contract.ErrNotFound
	}
	if int64(len(body)) > maxBodyBytes {
		return nil, contract.ErrInvalidArgument
	}
	return body, nil
}

type readerFixtureV2 struct {
	*readerFixture
	reader     *CurrentReaderV2
	coordinate AttemptCoordinateV2
	request    CurrentRequestV2
}

func newReaderFixtureV2(t *testing.T) *readerFixtureV2 {
	t.Helper()
	base := newReaderFixture(t)
	attempt := cloneAttempt(base.attempt)
	attempt.Ref = contract.Ref{ID: attempt.Ref.ID + "/v2", Revision: 1}
	attempt.IdempotencyKey += "/v2"
	sessionRef := testRef("session-a")
	sessionEvidenceRef := contract.Ref{ID: "session:" + sessionRef.Digest, Revision: sessionRef.Revision, Digest: sessionRef.Digest}
	turnDigestRef := testRef(attempt.TurnID)
	turnRef := contract.Ref{ID: "turn:" + turnDigestRef.Digest, Revision: turnDigestRef.Revision, Digest: turnDigestRef.Digest}
	attempt.TurnID = turnRef.ID
	var err error
	attempt, err = base.store.PutAttempt(attempt, contract.ExpectAbsent())
	if err != nil {
		t.Fatal(err)
	}
	attempt.Ref = contract.Ref{ID: attempt.Ref.ID, Revision: 2}
	coordinate := AttemptCoordinateV2{
		ContractVersion: ContractVersionV2, ObjectKind: AttemptCoordinateKindV2,
		TenantID: attempt.TenantID, IdentityRef: testRef("identity-a"), IdentityEpoch: 7,
		ExecutionScopeDigest: attempt.ExecutionScopeDigest, RunID: attempt.RunID,
		SessionRef: sessionRef, SessionEvidenceRef: sessionEvidenceRef, SessionCheckedAt: base.now, SessionExpiresAt: base.now.Add(45 * time.Minute),
		SourceTurnOrdinal: 1, SourceTurnRef: turnRef, TurnEvidenceRef: turnRef, TurnCheckedAt: base.now, TurnExpiresAt: base.now.Add(40 * time.Minute), LegacyTurnID: attempt.TurnID,
		AttemptRef: attempt.Ref, RequestDigest: attempt.RequestDigest, IdempotencyKey: attempt.IdempotencyKey, ObservationRef: attempt.ObservationRef, ResultRef: attempt.ResultRef,
	}
	attempt, coordinate, err = base.store.PutAttemptV2(attempt, coordinate, contract.ExpectRevision(1))
	if err != nil {
		t.Fatal(err)
	}
	reader, err := NewCurrentReaderV2(base.store)
	if err != nil {
		t.Fatal(err)
	}
	req := CurrentRequestV2{
		ContractVersion: ContractVersionV2, ObjectKind: CurrentRequestKindV2, Coordinate: coordinate,
		CurrentStateRef: base.state.Ref, ExpectedQueryRef: attempt.QueryRef, ExpectedViewRef: attempt.ViewRef, ExpectedSnapshotRef: attempt.SnapshotRef, ExpectedPointerRef: attempt.PointerRef,
		AuthorityRef: attempt.AuthorityRef, AuthorityEpoch: 7, PolicyRef: attempt.PolicyRef, Purpose: attempt.Purpose, Scopes: append([]string{}, attempt.Scopes...), AllowedLicenses: append([]string{}, attempt.AllowedLicenses...), SensitivityMax: attempt.SensitivityMax,
		CheckPhase: CheckPhaseS1V2, MaxItems: 8, MaxBytes: 4096, MaxTokens: 1024, PerItemMaxBytes: 1024, EstimatorRef: testRef("estimator"),
		CheckedUpperBound: base.now, NotAfter: base.now.Add(30 * time.Minute), ProjectionID: "knowledge/contribution/v2/s1", ProjectionRevision: 1,
	}
	req, err = SealCurrentRequestV2(req)
	if err != nil {
		t.Fatal(err)
	}
	return &readerFixtureV2{readerFixture: base, reader: reader, coordinate: coordinate, request: req}
}

func TestKnowledgeCurrentReaderV2StableFreshBoundedAndCopyIsolation(t *testing.T) {
	f := newReaderFixtureV2(t)
	inspection, err := f.reader.InspectAttempt(context.Background(), f.coordinate)
	if err != nil || inspection.Status != AttemptPersistedAndSettled {
		t.Fatalf("inspection=%+v err=%v", inspection, err)
	}
	s1, err := f.reader.InspectForTurn(context.Background(), f.request)
	if err != nil || !s1.Current || len(s1.Items) != 1 || s1.Items[0].License == "" {
		t.Fatalf("s1=%+v err=%v", s1, err)
	}
	f.now = f.now.Add(time.Second)
	s2req := f.request
	s2req.CheckPhase = CheckPhaseS2V2
	s2req.ExpectedS1ClosureDigest = s1.StableClosureDigest
	s2req.ProjectionID = "knowledge/contribution/v2/s2"
	s2req.ProjectionRevision = 2
	s2req.Digest = ""
	s2req, err = SealCurrentRequestV2(s2req)
	if err != nil {
		t.Fatal(err)
	}
	s2, err := f.reader.InspectForTurn(context.Background(), s2req)
	if err != nil || s2.StableClosureDigest != s1.StableClosureDigest || s2.Digest == s1.Digest {
		t.Fatalf("stable/fresh separation failed: s1=%+v s2=%+v err=%v", s1, s2, err)
	}
	contentReq, err := SealExactContentRequestV2(ExactContentRequestV2{
		ContractVersion: ContractVersionV2, ObjectKind: ExactContentRequestKindV2, Coordinate: f.coordinate, Projection: s2, Rank: 0,
		CheckPhase: CheckPhaseS2V2, ExpectedStableClosureDigest: s2.StableClosureDigest, MaxBodyBytes: 1024, CheckedUpperBound: f.now, NotAfter: f.now.Add(10 * time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	observation, body, err := f.reader.ReadContentExact(context.Background(), contentReq)
	if err != nil || string(body) != "owner local knowledge content" || observation.License != s2.Items[0].License || observation.ObservedDigest != s2.Items[0].ContentRef.Digest {
		t.Fatalf("observation=%+v body=%q err=%v", observation, body, err)
	}
	body[0] = 'X'
	_, again, err := f.reader.ReadContentExact(context.Background(), contentReq)
	if err != nil || string(again) != "owner local knowledge content" {
		t.Fatalf("copy isolation failed: %q %v", again, err)
	}
	tooSmall := contentReq
	tooSmall.MaxBodyBytes = 1
	tooSmall.Digest = ""
	tooSmall, err = SealExactContentRequestV2(tooSmall)
	if err != nil {
		t.Fatal(err)
	}
	if got, body, err := f.reader.ReadContentExact(context.Background(), tooSmall); !errors.Is(err, contract.ErrInvalidArgument) || got != (ExactContentObservationV2{}) || body != nil {
		t.Fatalf("oversize did not fail closed: observation=%+v body=%q err=%v", got, body, err)
	}
}

func TestKnowledgeCurrentReaderV2CancellationTTLCurrentnessEvictionAndPoison(t *testing.T) {
	t.Run("lost reply inspect and exact drift classification", func(t *testing.T) {
		f := newReaderFixtureV2(t)
		inspection, err := f.reader.InspectAttempt(context.Background(), f.coordinate)
		if err != nil || inspection.Status != AttemptPersistedAndSettled {
			t.Fatalf("lost-reply inspect failed: %+v %v", inspection, err)
		}
		missing := f.coordinate
		missing.AttemptRef = testRef("knowledge/missing-v2-attempt")
		missing.Digest = ""
		missing, err = SealAttemptCoordinateV2(missing)
		if err != nil {
			t.Fatal(err)
		}
		inspection, err = f.reader.InspectAttempt(context.Background(), missing)
		if err != nil || inspection.Status != AttemptNotPersisted {
			t.Fatalf("missing ID classification=%+v %v", inspection, err)
		}
		drift := f.coordinate
		drift.AttemptRef = testRefRevision(f.coordinate.AttemptRef.ID, f.coordinate.AttemptRef.Revision+1)
		drift.Digest = ""
		drift, err = SealAttemptCoordinateV2(drift)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.reader.InspectAttempt(context.Background(), drift); !errors.Is(err, contract.ErrEvidenceConflict) {
			t.Fatalf("same-ID drift classified as absent: %v", err)
		}
	})
	t.Run("cancel while waiting for owner lock", func(t *testing.T) {
		f := newReaderFixtureV2(t)
		f.store.mu.Lock()
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()
		_, err := f.reader.InspectForTurn(ctx, f.request)
		f.store.mu.Unlock()
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("lock wait ignored context: %v", err)
		}
	})
	t.Run("owner ttl beats caller upper bound", func(t *testing.T) {
		f := newReaderFixtureV2(t)
		f.now = f.coordinate.SessionExpiresAt
		if _, err := f.reader.InspectForTurn(context.Background(), f.request); !errors.Is(err, contract.ErrNotCurrent) {
			t.Fatalf("expired session accepted: %v", err)
		}
	})
	t.Run("evicted and poisoned fail closed", func(t *testing.T) {
		f := newReaderFixtureV2(t)
		p, err := f.reader.InspectForTurn(context.Background(), f.request)
		if err != nil {
			t.Fatal(err)
		}
		f.content.evict(p.Items[0].ContentRef)
		r, err := SealExactContentRequestV2(ExactContentRequestV2{ContractVersion: ContractVersionV2, ObjectKind: ExactContentRequestKindV2, Coordinate: f.coordinate, Projection: p, Rank: 0, CheckPhase: CheckPhaseS1V2, ExpectedStableClosureDigest: p.StableClosureDigest, MaxBodyBytes: 1024, CheckedUpperBound: f.now, NotAfter: f.now.Add(time.Minute)})
		if err != nil {
			t.Fatal(err)
		}
		if got, body, err := f.reader.ReadContentExact(context.Background(), r); !errors.Is(err, contract.ErrContextUnmaterialized) || got != (ExactContentObservationV2{}) || body != nil {
			t.Fatalf("eviction leaked result: %+v %q %v", got, body, err)
		}
		next := cloneCurrentState(f.state)
		next.Ref = contract.Ref{ID: next.Ref.ID, Revision: 2}
		next.Items[0].PoisoningCleared = false
		next, err = f.store.PublishCurrent(next, contract.ExpectRevision(1))
		if err != nil {
			t.Fatal(err)
		}
		req := f.request
		req.CurrentStateRef = next.Ref
		req.Digest = ""
		req, err = SealCurrentRequestV2(req)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.reader.InspectForTurn(context.Background(), req); !errors.Is(err, contract.ErrNotCurrent) {
			t.Fatalf("poisoned record accepted: %v", err)
		}
	})
}

func TestKnowledgeCurrentReaderV2GetCrossesTTLAnd64Concurrent(t *testing.T) {
	f := newReaderFixtureV2(t)
	p, err := f.reader.InspectForTurn(context.Background(), f.request)
	if err != nil {
		t.Fatal(err)
	}
	r, err := SealExactContentRequestV2(ExactContentRequestV2{ContractVersion: ContractVersionV2, ObjectKind: ExactContentRequestKindV2, Coordinate: f.coordinate, Projection: p, Rank: 0, CheckPhase: CheckPhaseS1V2, ExpectedStableClosureDigest: p.StableClosureDigest, MaxBodyBytes: 1024, CheckedUpperBound: f.now, NotAfter: f.now.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	started, release := f.content.blockGets()
	type result struct {
		observation ExactContentObservationV2
		body        []byte
		err         error
	}
	done := make(chan result, 1)
	go func() { o, b, err := f.reader.ReadContentExact(context.Background(), r); done <- result{o, b, err} }()
	<-started
	f.now = f.coordinate.TurnExpiresAt
	close(release)
	got := <-done
	if !errors.Is(got.err, contract.ErrNotCurrent) || got.observation != (ExactContentObservationV2{}) || got.body != nil {
		t.Fatalf("ttl crossing leaked result: %+v %q %v", got.observation, got.body, got.err)
	}
	f = newReaderFixtureV2(t)
	const workers = 64
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p, err := f.reader.InspectForTurn(context.Background(), f.request)
			if err != nil || len(p.Items) != 1 {
				errs <- errors.Join(err, errors.New("inconsistent projection"))
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
}
