package modelinvokeradapter

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func TestSQLitePreparedAckRepositoryCreateOnceRecoveryRestartAndConflictV1(t *testing.T) {
	ctx := context.Background()
	config := SQLitePreparedModelInvocationAckRepositoryConfigV1{
		Path:  t.TempDir() + "/ack.db",
		Clock: func() time.Time { return time.Unix(0, 1_000) },
	}
	repository, err := OpenSQLitePreparedModelInvocationAckRepositoryV1(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	ack := preparedAckRepositoryFixtureV1(t, 2_000, 8_000, "surface-binding-1")
	repository.LoseNextReplyForTestingV1()
	if _, err := repository.EnsureAck(ctx, ack); !core.HasCategory(err, core.ErrorIndeterminate) {
		t.Fatalf("lost reply = %v", err)
	}
	recovered, err := repository.inspectByPreparedCurrent(ctx, ack.PreparedRef, ack.CurrentRef)
	if err != nil || recovered != ack {
		t.Fatalf("stable recovery = %#v, %v", recovered, err)
	}
	if err := repository.IntegrityCheckV1(ctx); err != nil {
		t.Fatal(err)
	}
	if err := repository.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenSQLitePreparedModelInvocationAckRepositoryV1(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if got, err := reopened.InspectExactAck(ctx, ack.Ref()); err != nil || got != ack {
		t.Fatalf("restart exact = %#v, %v", got, err)
	}
	drift := preparedAckRepositoryFixtureV1(t, 2_000, 8_000, "surface-binding-2")
	if _, err := reopened.EnsureAck(ctx, drift); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("same coordinate drift = %v", err)
	}
	otherEpoch := preparedAckRepositoryFixtureV1(t, 2_100, 7_900, "surface-binding-1")
	if _, err := reopened.EnsureAck(ctx, otherEpoch); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("prepared epoch drift = %v", err)
	}
}

func TestSQLitePreparedAckRepositoryConcurrentCreateOnceAndContextV1(t *testing.T) {
	config := SQLitePreparedModelInvocationAckRepositoryConfigV1{
		Path:  t.TempDir() + "/ack-concurrent.db",
		Clock: func() time.Time { return time.Unix(0, 1_000) },
	}
	repository, err := OpenSQLitePreparedModelInvocationAckRepositoryV1(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	ack := preparedAckRepositoryFixtureV1(t, 2_000, 8_000, "surface-binding-1")
	const workers = 64
	var wait sync.WaitGroup
	errors := make(chan error, workers)
	for range workers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			got, err := repository.EnsureAck(context.Background(), ack)
			if err == nil && got != ack {
				err = core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "concurrent SQLite ACK drifted")
			}
			errors <- err
		}()
	}
	wait.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err := repository.db.QueryRow(`SELECT COUNT(*) FROM harness_prepared_ack_v1`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("row count = %d, %v", count, err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := repository.InspectExactAck(canceled, ack.Ref()); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("canceled exact read = %v", err)
	}
}

func TestSQLitePreparedAckRepositoryDetectsRowDigestDriftV1(t *testing.T) {
	cases := map[string]string{
		"row digest":           `UPDATE harness_prepared_ack_v1 SET row_digest='sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa' WHERE ack_id=?`,
		"ack digest":           `UPDATE harness_prepared_ack_v1 SET ack_digest='sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa' WHERE ack_id=?`,
		"prepared current key": `UPDATE harness_prepared_ack_v1 SET prepared_current_key='sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa' WHERE ack_id=?`,
		"prepared ref key":     `UPDATE harness_prepared_ack_v1 SET prepared_ref_key='sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa' WHERE ack_id=?`,
	}
	for name, statement := range cases {
		t.Run(name, func(t *testing.T) {
			config := SQLitePreparedModelInvocationAckRepositoryConfigV1{
				Path:  t.TempDir() + "/ack-corrupt.db",
				Clock: func() time.Time { return time.Unix(0, 1_000) },
			}
			repository, err := OpenSQLitePreparedModelInvocationAckRepositoryV1(context.Background(), config)
			if err != nil {
				t.Fatal(err)
			}
			defer repository.Close()
			ack := preparedAckRepositoryFixtureV1(t, 2_000, 8_000, "surface-binding-1")
			if _, err := repository.EnsureAck(context.Background(), ack); err != nil {
				t.Fatal(err)
			}
			if _, err := repository.db.Exec(statement, ack.ID); err != nil {
				t.Fatal(err)
			}
			if _, err := repository.InspectExactAck(context.Background(), ack.Ref()); !core.HasCategory(err, core.ErrorConflict) {
				t.Fatalf("indexed row drift = %v", err)
			}
		})
	}
}
