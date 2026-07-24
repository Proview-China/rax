package contextsource

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
)

func TestMemoryCurrentReaderV2TamperBindingDriftAndGetCancellation(t *testing.T) {
	t.Run("stable field tamper", func(t *testing.T) {
		f := newReaderFixtureV2(t)
		projection, err := f.reader.InspectForTurn(context.Background(), f.request)
		if err != nil {
			t.Fatal(err)
		}
		projection.NextCursor = "forged-cursor"
		if err := projection.Validate(); !errors.Is(err, contract.ErrEvidenceConflict) {
			t.Fatalf("stable tamper accepted: %v", err)
		}
	})
	t.Run("binding drift during get", func(t *testing.T) {
		f := newReaderFixtureV2(t)
		projection, err := f.reader.InspectForTurn(context.Background(), f.request)
		if err != nil {
			t.Fatal(err)
		}
		req, err := SealExactContentRequestV2(ExactContentRequestV2{ContractVersion: ContractVersionV2, ObjectKind: ExactContentRequestKindV2, Coordinate: f.coordinate, Projection: projection, Rank: 0, CheckPhase: CheckPhaseS1V2, ExpectedStableClosureDigest: projection.StableClosureDigest, MaxBodyBytes: 1024, CheckedUpperBound: f.now, NotAfter: f.now.Add(time.Minute)})
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
		go func() { o, b, err := f.reader.ReadContentExact(context.Background(), req); done <- result{o, b, err} }()
		<-started
		drift := f.content.StatePlaneBinding()
		drift.Ref = testRefRevision(drift.Ref.ID, drift.Ref.Revision+1)
		f.content.setBinding(drift)
		close(release)
		got := <-done
		if !errors.Is(got.err, contract.ErrNotCurrent) || got.observation != (ExactContentObservationV2{}) || got.body != nil {
			t.Fatalf("binding drift leaked result: %+v %q %v", got.observation, got.body, got.err)
		}
	})
	t.Run("cancel during get", func(t *testing.T) {
		f := newReaderFixtureV2(t)
		projection, err := f.reader.InspectForTurn(context.Background(), f.request)
		if err != nil {
			t.Fatal(err)
		}
		req, err := SealExactContentRequestV2(ExactContentRequestV2{ContractVersion: ContractVersionV2, ObjectKind: ExactContentRequestKindV2, Coordinate: f.coordinate, Projection: projection, Rank: 0, CheckPhase: CheckPhaseS1V2, ExpectedStableClosureDigest: projection.StableClosureDigest, MaxBodyBytes: 1024, CheckedUpperBound: f.now, NotAfter: f.now.Add(time.Minute)})
		if err != nil {
			t.Fatal(err)
		}
		started, release := f.content.blockGets()
		ctx, cancel := context.WithCancel(context.Background())
		type result struct {
			observation ExactContentObservationV2
			body        []byte
			err         error
		}
		done := make(chan result, 1)
		go func() { o, b, err := f.reader.ReadContentExact(ctx, req); done <- result{o, b, err} }()
		<-started
		cancel()
		got := <-done
		close(release)
		if !errors.Is(got.err, context.Canceled) || got.observation != (ExactContentObservationV2{}) || got.body != nil {
			t.Fatalf("get cancellation leaked result: %+v %q %v", got.observation, got.body, got.err)
		}
	})
}
