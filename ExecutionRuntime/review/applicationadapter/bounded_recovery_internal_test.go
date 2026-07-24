package applicationadapter

import (
	"context"
	"testing"
	"time"
)

func TestReviewWaitingRecoveryUsesShortestSnapshotTTL(t *testing.T) {
	now := time.Unix(2_400_100_000, 0)
	parent, cancel, ok := reviewWaitingRecoveryContextV1(context.Background(), now, now.Add(time.Second).UnixNano())
	if !ok {
		t.Fatal("recovery context was not constructed")
	}
	defer cancel()
	snapshot, snapshotCancel, ok := reviewWaitingTightenRecoveryV1(parent, now, now.Add(20*time.Millisecond).UnixNano())
	if !ok {
		t.Fatal("snapshot recovery context was not tightened")
	}
	defer snapshotCancel()
	started := time.Now()
	<-snapshot.Done()
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("snapshot TTL did not shorten recovery: %s", elapsed)
	}
}
