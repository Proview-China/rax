package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

func TestWorkspaceReadReserveAcrossHandlesCreatesOnce(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Unix(1_900_000_000, 0)
	expires := now.Add(time.Hour)
	database := filepath.Join(t.TempDir(), "sandbox.db")
	first, err := OpenWithClock(ctx, database, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	second, err := OpenWithClock(ctx, database, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = first.Close(); _ = second.Close() })
	reservation, attempt := workspaceReadReservationFixture(t, now, expires, "request-a")
	type outcome struct {
		created bool
		err     error
	}
	results := make(chan outcome, 64)
	var group sync.WaitGroup
	for index := 0; index < 64; index++ {
		group.Add(1)
		store := first
		if index%2 == 1 {
			store = second
		}
		go func() {
			defer group.Done()
			projection, created, reserveErr := store.ReserveWorkspaceReadV1(ctx, reservation, attempt)
			if reserveErr == nil && (projection.Attempt.Meta.ID != attempt.Meta.ID || projection.AdmissionReceipt != attempt.AdmissionReceipt) {
				reserveErr = errors.New("projection drifted")
			}
			results <- outcome{created: created, err: reserveErr}
		}()
	}
	group.Wait()
	close(results)
	createdCount := 0
	for result := range results {
		if result.err != nil {
			t.Fatalf("reserve: %v", result.err)
		}
		if result.created {
			createdCount++
		}
	}
	if createdCount != 1 {
		t.Fatalf("created=%d, want 1", createdCount)
	}
}

func TestWorkspaceReadOriginalAttemptRecoversLatestCurrent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Unix(1_900_000_000, 0)
	expires := now.Add(time.Hour)
	store, err := OpenWithClock(ctx, filepath.Join(t.TempDir(), "sandbox.db"), func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	reservation, attempt := workspaceReadReservationFixture(t, now, expires, "request-a")
	started, created, err := store.ReserveWorkspaceReadV1(ctx, reservation, attempt)
	if err != nil || !created || started.Attempt.State != contract.WorkspaceReadStartedV1 || started.AdmissionReceipt != attempt.AdmissionReceipt {
		t.Fatalf("reserve: created=%v state=%q err=%v", created, started.Attempt.State, err)
	}
	providerReceipt := contract.WorkspaceReadReceiptBindingV1{ID: "provider-receipt", Revision: 1, Digest: mustWorkspaceReadDigest(t, "provider-receipt"), StableKeyDigest: reservation.StableKeyDigest, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires.UnixNano()}
	content := "hello"
	observation, err := contract.SealWorkspaceReadObservationV1(contract.WorkspaceReadObservationV1{
		Reservation: reservation.Meta.Ref(), Command: reservation.Command, WorkspaceView: reservation.WorkspaceView,
		File:         contract.Ref{ID: "workspace-file", Revision: reservation.WorkspaceView.Revision, Digest: mustWorkspaceReadDigest(t, "whole-file")},
		RelativePath: "src/main.txt", ReturnedBytes: uint64(len(content)), TotalBytes: uint64(len(content)), Complete: true,
		Content: content, ContentDigest: contract.WorkspaceReadContentDigestV1([]byte(content), 0, uint64(len(content)), true),
		S1CheckedUnixNano: now.UnixNano(), S2CheckedUnixNano: now.UnixNano(),
		AdmissionReceipt: attempt.AdmissionReceipt, ProviderReceipt: providerReceipt,
	}, "workspace-read-observation", now, expires)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.CompleteWorkspaceReadV1(ctx, attempt.Meta.Ref(), observation); err != nil {
		t.Fatal(err)
	}

	recovered, err := store.InspectBoundedWorkspaceReadV1(ctx, contract.WorkspaceReadAttemptRefV1{ID: attempt.Meta.ID, Revision: attempt.Meta.Revision, Digest: attempt.Meta.Digest})
	if err != nil || recovered.Attempt.State != contract.WorkspaceReadObservedV1 || recovered.Observation == nil || recovered.Observation.Content != content || recovered.ProviderReceipt == nil || *recovered.ProviderReceipt != providerReceipt {
		t.Fatalf("recover latest via original ref: %#v err=%v", recovered, err)
	}
}

func TestWorkspaceReadStartedInspectIsReadOnly(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Unix(1_900_000_000, 0)
	expires := now.Add(time.Hour)
	store, err := OpenWithClock(ctx, filepath.Join(t.TempDir(), "sandbox.db"), func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	reservation, attempt := workspaceReadReservationFixture(t, now, expires, "request-a")
	if _, _, err = store.ReserveWorkspaceReadV1(ctx, reservation, attempt); err != nil {
		t.Fatal(err)
	}
	projection, err := store.InspectBoundedWorkspaceReadV1(ctx, contract.WorkspaceReadAttemptRefV1{ID: attempt.Meta.ID, Revision: 1, Digest: attempt.Meta.Digest})
	if err != nil || projection.Attempt.State != contract.WorkspaceReadStartedV1 || projection.AdmissionReceipt != attempt.AdmissionReceipt {
		t.Fatalf("inspect started: state=%q err=%v", projection.Attempt.State, err)
	}
	again, err := store.InspectBoundedWorkspaceReadV1(ctx, contract.WorkspaceReadAttemptRefV1{ID: attempt.Meta.ID, Revision: 1, Digest: attempt.Meta.Digest})
	if err != nil || again.Attempt.Meta.Ref() != projection.Attempt.Meta.Ref() || again.Attempt.State != contract.WorkspaceReadStartedV1 {
		t.Fatalf("read-only Inspect mutated current: %#v err=%v", again, err)
	}
}

func TestWorkspaceReadConcurrentInspectCannotPoisonActiveCompletion(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Unix(1_900_000_000, 0)
	expires := now.Add(time.Hour)
	store, err := OpenWithClock(ctx, filepath.Join(t.TempDir(), "sandbox.db"), func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	reservation, attempt := workspaceReadReservationFixture(t, now, expires, "request-concurrent-inspect")
	if _, created, reserveErr := store.ReserveWorkspaceReadV1(ctx, reservation, attempt); reserveErr != nil || !created {
		t.Fatalf("reserve: created=%v err=%v", created, reserveErr)
	}
	original := contract.WorkspaceReadAttemptRefV1{ID: attempt.Meta.ID, Revision: attempt.Meta.Revision, Digest: attempt.Meta.Digest}
	content := "hello"
	providerReceipt := contract.WorkspaceReadReceiptBindingV1{
		ID: "provider-concurrent", Revision: 1, Digest: mustWorkspaceReadDigest(t, "provider-concurrent"),
		StableKeyDigest: reservation.StableKeyDigest, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires.UnixNano(),
	}
	observation, err := contract.SealWorkspaceReadObservationV1(contract.WorkspaceReadObservationV1{
		Reservation: reservation.Meta.Ref(), Command: reservation.Command, WorkspaceView: reservation.WorkspaceView,
		File:         contract.Ref{ID: "workspace-file", Revision: reservation.WorkspaceView.Revision, Digest: mustWorkspaceReadDigest(t, "whole-file")},
		RelativePath: "src/main.txt", ReturnedBytes: uint64(len(content)), TotalBytes: uint64(len(content)), Complete: true,
		Content: content, ContentDigest: contract.WorkspaceReadContentDigestV1([]byte(content), 0, uint64(len(content)), true),
		S1CheckedUnixNano: now.UnixNano(), S2CheckedUnixNano: now.UnixNano(),
		AdmissionReceipt: attempt.AdmissionReceipt, ProviderReceipt: providerReceipt,
	}, "workspace-read-observation-concurrent", now, expires)
	if err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	results := make(chan error, 65)
	var group sync.WaitGroup
	for index := 0; index < 64; index++ {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			projection, inspectErr := store.InspectBoundedWorkspaceReadV1(ctx, original)
			if inspectErr == nil && projection.Attempt.State != contract.WorkspaceReadStartedV1 && projection.Attempt.State != contract.WorkspaceReadObservedV1 {
				inspectErr = errors.New("read-only Inspect produced a non-owner terminal state")
			}
			results <- inspectErr
		}()
	}
	group.Add(1)
	go func() {
		defer group.Done()
		<-start
		_, completeErr := store.CompleteWorkspaceReadV1(ctx, attempt.Meta.Ref(), observation)
		results <- completeErr
	}()
	close(start)
	group.Wait()
	close(results)
	for result := range results {
		if result != nil {
			t.Fatalf("concurrent Inspect/Complete: %v", result)
		}
	}
	final, err := store.InspectBoundedWorkspaceReadV1(ctx, original)
	if err != nil || final.Attempt.State != contract.WorkspaceReadObservedV1 || final.Observation == nil || final.Observation.Meta.Ref() != observation.Meta.Ref() {
		t.Fatalf("active completion was poisoned by Inspect: %#v err=%v", final, err)
	}
}

func TestWorkspaceReadStartedRecoveryRequiresNewOwnerIncarnation(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(1_900_000_000, 0)
	current := now
	expires := now.Add(time.Hour)
	database := filepath.Join(t.TempDir(), "sandbox.db")
	first, err := OpenWithClock(ctx, database, func() time.Time { return current })
	if err != nil {
		t.Fatal(err)
	}
	reservation, attempt := workspaceReadReservationFixture(t, now, expires, "request-restart")
	if _, created, reserveErr := first.ReserveWorkspaceReadV1(ctx, reservation, attempt); reserveErr != nil || !created {
		t.Fatalf("reserve: created=%v err=%v", created, reserveErr)
	}
	original := contract.WorkspaceReadAttemptRefV1{ID: attempt.Meta.ID, Revision: attempt.Meta.Revision, Digest: attempt.Meta.Digest}
	if _, err = first.RecoverStartedWorkspaceReadAfterRestartV1(ctx, original); !errors.Is(err, ports.ErrConflict) {
		t.Fatalf("live owner must not recover its own started attempt: %v", err)
	}
	before, err := first.InspectBoundedWorkspaceReadV1(ctx, original)
	if err != nil || before.Attempt.State != contract.WorkspaceReadStartedV1 {
		t.Fatalf("live Inspect must remain read-only: %#v err=%v", before, err)
	}
	if err = first.Close(); err != nil {
		t.Fatal(err)
	}
	current = now.Add(time.Second)
	reopened, err := OpenWithClock(ctx, database, func() time.Time { return current })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	stillStarted, err := reopened.InspectBoundedWorkspaceReadV1(ctx, original)
	if err != nil || stillStarted.Attempt.State != contract.WorkspaceReadStartedV1 {
		t.Fatalf("restart Inspect must not mutate: %#v err=%v", stillStarted, err)
	}
	recovered, err := reopened.RecoverStartedWorkspaceReadAfterRestartV1(ctx, original)
	if err != nil || recovered.Attempt.State != contract.WorkspaceReadUnknownV1 || recovered.Observation != nil {
		t.Fatalf("explicit restart recovery: %#v err=%v", recovered, err)
	}
	again, err := reopened.RecoverStartedWorkspaceReadAfterRestartV1(ctx, original)
	if err != nil || again.Attempt.Meta.Ref() != recovered.Attempt.Meta.Ref() {
		t.Fatalf("restart recovery must be idempotent for the same incarnation: %#v err=%v", again, err)
	}
}

func TestWorkspaceReadStoreEnforcesSealedTTLClosureAtWriteBoundaries(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Unix(1_900_000_000, 0)
	effective := now.Add(30 * time.Minute)
	outer := now.Add(time.Hour)
	current := now
	store, err := OpenWithClock(ctx, filepath.Join(t.TempDir(), "sandbox.db"), func() time.Time { return current })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	closure, err := contract.SealWorkspaceReadTTLClosureV1(contract.WorkspaceReadTTLClosureV1{
		UnifiedNotAfterUnixNano: outer.UnixNano(), RuntimeEnforcementExpiresNano: outer.UnixNano(),
		AssociationExpiresUnixNano: effective.UnixNano(), CommandRequestedNotAfterNano: outer.UnixNano(),
		CommandExpiresUnixNano: outer.UnixNano(), WorkspaceViewExpiresUnixNano: outer.UnixNano(),
		WorkspaceLeaseExpiresUnixNano: outer.UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if closure.EffectiveExpiresUnixNano != effective.UnixNano() {
		t.Fatalf("effective expiry=%d, want shortest upstream=%d", closure.EffectiveExpiresUnixNano, effective.UnixNano())
	}
	stable := mustWorkspaceReadDigest(t, "ttl-stable")
	reservationDraft := contract.WorkspaceReadReservationV1{
		StableKeyDigest: stable, AuthorizationDigest: mustWorkspaceReadDigest(t, "ttl-authorization"),
		RequestDigest: mustWorkspaceReadDigest(t, "ttl-request"), PayloadDigest: mustWorkspaceReadDigest(t, "ttl-payload"),
		Command:       contract.Ref{ID: "ttl-command", Revision: 1, Digest: mustWorkspaceReadDigest(t, "ttl-command")},
		WorkspaceView: contract.Ref{ID: "ttl-workspace", Revision: 1, Digest: mustWorkspaceReadDigest(t, "ttl-workspace")},
		AttemptID:     "ttl-attempt", TTLClosure: closure,
	}
	if _, err = contract.SealWorkspaceReadReservationV1(reservationDraft, "ttl-reservation-overlong", now, outer); err == nil {
		t.Fatal("overlong reservation escaped the sealed upstream TTL closure")
	}
	tampered := reservationDraft
	tampered.TTLClosure.Digest = mustWorkspaceReadDigest(t, "tampered-ttl-closure")
	if _, err = contract.SealWorkspaceReadReservationV1(tampered, "ttl-reservation-tampered", now, effective); err == nil {
		t.Fatal("tampered TTL closure reached reservation sealing")
	}
	reservation, err := contract.SealWorkspaceReadReservationV1(reservationDraft, "ttl-reservation", now, effective)
	if err != nil {
		t.Fatal(err)
	}
	admission := contract.WorkspaceReadReceiptBindingV1{
		ID: "ttl-admission", Revision: 1, Digest: mustWorkspaceReadDigest(t, "ttl-admission"),
		StableKeyDigest: stable, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: effective.UnixNano(),
	}
	attempt, err := contract.SealWorkspaceReadAttemptV1(contract.WorkspaceReadAttemptV1{
		StableKeyDigest: stable, RequestDigest: reservation.RequestDigest, PayloadDigest: reservation.PayloadDigest,
		Reservation: reservation.Meta.Ref(), AdmissionReceipt: admission, State: contract.WorkspaceReadStartedV1,
	}, "ttl-attempt", 1, now, effective)
	if err != nil {
		t.Fatal(err)
	}
	current = effective
	if _, _, err = store.ReserveWorkspaceReadV1(ctx, reservation, attempt); err == nil {
		t.Fatal("now==effective expiry reached the durable reservation")
	}
	current = now
	if _, created, err := store.ReserveWorkspaceReadV1(ctx, reservation, attempt); err != nil || !created {
		t.Fatalf("current shortest-bound reservation: created=%v err=%v", created, err)
	}
	content := "ttl"
	overlongProvider := contract.WorkspaceReadReceiptBindingV1{
		ID: "ttl-provider", Revision: 1, Digest: mustWorkspaceReadDigest(t, "ttl-provider"),
		StableKeyDigest: stable, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: outer.UnixNano(),
	}
	if _, err = contract.SealWorkspaceReadObservationV1(contract.WorkspaceReadObservationV1{
		Reservation: reservation.Meta.Ref(), Command: reservation.Command, WorkspaceView: reservation.WorkspaceView,
		File:         contract.Ref{ID: "ttl-file", Revision: 1, Digest: mustWorkspaceReadDigest(t, "ttl-file")},
		RelativePath: "src/main.txt", ReturnedBytes: uint64(len(content)), TotalBytes: uint64(len(content)), Complete: true,
		Content: content, ContentDigest: contract.WorkspaceReadContentDigestV1([]byte(content), 0, uint64(len(content)), true),
		S1CheckedUnixNano: now.UnixNano(), S2CheckedUnixNano: now.UnixNano(),
		AdmissionReceipt: admission, ProviderReceipt: overlongProvider,
	}, "ttl-observation-overlong", now, outer); err == nil {
		t.Fatal("observation exceeded the reservation/admission TTL closure")
	}
}

func TestWorkspaceReadStoreRejectsFutureReceiptAndObservation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Unix(1_900_000_000, 0)
	expires := now.Add(time.Hour)
	store, err := OpenWithClock(ctx, filepath.Join(t.TempDir(), "sandbox.db"), func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	reservation, attempt := workspaceReadReservationFixture(t, now, expires, "request-future")
	attempt.AdmissionReceipt.CheckedUnixNano = now.Add(time.Second).UnixNano()
	attempt, err = contract.SealWorkspaceReadAttemptV1(attempt, attempt.Meta.ID, 1, now.Add(time.Second), expires)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = store.ReserveWorkspaceReadV1(ctx, reservation, attempt); err == nil {
		t.Fatal("future admission receipt reached the durable reservation")
	}

	reservation, attempt = workspaceReadReservationFixture(t, now, expires, "request-observation-future")
	if _, created, reserveErr := store.ReserveWorkspaceReadV1(ctx, reservation, attempt); reserveErr != nil || !created {
		t.Fatalf("reserve observation fixture: created=%v err=%v", created, reserveErr)
	}
	future := now.Add(time.Second)
	content := "hello"
	providerReceipt := contract.WorkspaceReadReceiptBindingV1{
		ID: "provider-future", Revision: 1, Digest: mustWorkspaceReadDigest(t, "provider-future"),
		StableKeyDigest: reservation.StableKeyDigest, CheckedUnixNano: future.UnixNano(), ExpiresUnixNano: expires.UnixNano(),
	}
	observation, err := contract.SealWorkspaceReadObservationV1(contract.WorkspaceReadObservationV1{
		Reservation: reservation.Meta.Ref(), Command: reservation.Command, WorkspaceView: reservation.WorkspaceView,
		File:         contract.Ref{ID: "workspace-file", Revision: reservation.WorkspaceView.Revision, Digest: mustWorkspaceReadDigest(t, "whole-file")},
		RelativePath: "src/main.txt", ReturnedBytes: uint64(len(content)), TotalBytes: uint64(len(content)), Complete: true,
		Content: content, ContentDigest: contract.WorkspaceReadContentDigestV1([]byte(content), 0, uint64(len(content)), true),
		S1CheckedUnixNano: now.UnixNano(), S2CheckedUnixNano: future.UnixNano(),
		AdmissionReceipt: attempt.AdmissionReceipt, ProviderReceipt: providerReceipt,
	}, "workspace-read-observation-future", future, expires)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.CompleteWorkspaceReadV1(ctx, attempt.Meta.Ref(), observation); err == nil {
		t.Fatal("future observation reached durable completion")
	}
	inspected, err := store.InspectBoundedWorkspaceReadV1(ctx, contract.WorkspaceReadAttemptRefV1{ID: attempt.Meta.ID, Revision: 1, Digest: attempt.Meta.Digest})
	if err != nil || inspected.Attempt.State != contract.WorkspaceReadStartedV1 {
		t.Fatalf("failed completion mutated current: %#v err=%v", inspected, err)
	}
}

func TestWorkspaceReadSameAttemptIDDifferentOriginalDigestConflicts(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Unix(1_900_000_000, 0)
	expires := now.Add(time.Hour)
	store, err := OpenWithClock(ctx, filepath.Join(t.TempDir(), "sandbox.db"), func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	reservation, attempt := workspaceReadReservationFixture(t, now, expires, "request-a")
	if _, _, err = store.ReserveWorkspaceReadV1(ctx, reservation, attempt); err != nil {
		t.Fatal(err)
	}
	otherReservation, otherAttempt := workspaceReadReservationFixture(t, now, expires, "request-b")
	otherReservation.StableKeyDigest = reservation.StableKeyDigest
	otherReservation.Meta, err = contract.NewMeta(reservation.Meta.ID, 1, now, expires, "workspace-read-reservation", otherReservation)
	if err == nil {
		// Re-seal through the public constructor so the changed request is exact.
		otherReservation, err = contract.SealWorkspaceReadReservationV1(contract.WorkspaceReadReservationV1{StableKeyDigest: reservation.StableKeyDigest, AuthorizationDigest: reservation.AuthorizationDigest, RequestDigest: mustWorkspaceReadDigest(t, "request-b"), PayloadDigest: reservation.PayloadDigest, Command: reservation.Command, WorkspaceView: reservation.WorkspaceView, AttemptID: reservation.AttemptID, TTLClosure: reservation.TTLClosure}, reservation.Meta.ID, now, expires)
	}
	if err != nil {
		t.Fatal(err)
	}
	otherAttempt.RequestDigest = otherReservation.RequestDigest
	otherAttempt.StableKeyDigest = reservation.StableKeyDigest
	otherAttempt.Reservation = otherReservation.Meta.Ref()
	otherAttempt.AdmissionReceipt = attempt.AdmissionReceipt
	otherAttempt, err = contract.SealWorkspaceReadAttemptV1(otherAttempt, attempt.Meta.ID, 1, now, expires)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = store.ReserveWorkspaceReadV1(ctx, otherReservation, otherAttempt)
	if !errors.Is(err, ports.ErrConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
}

func workspaceReadReservationFixture(t *testing.T, now, expires time.Time, request string) (contract.WorkspaceReadReservationV1, contract.WorkspaceReadAttemptV1) {
	t.Helper()
	stable := mustWorkspaceReadDigest(t, "stable")
	admission := contract.WorkspaceReadReceiptBindingV1{ID: "admission", Revision: 1, Digest: mustWorkspaceReadDigest(t, "admission"), StableKeyDigest: stable, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires.UnixNano()}
	ttlClosure, err := contract.SealWorkspaceReadTTLClosureV1(contract.WorkspaceReadTTLClosureV1{
		UnifiedNotAfterUnixNano:       expires.UnixNano(),
		RuntimeEnforcementExpiresNano: expires.UnixNano(),
		AssociationExpiresUnixNano:    expires.UnixNano(),
		CommandRequestedNotAfterNano:  expires.UnixNano(),
		CommandExpiresUnixNano:        expires.UnixNano(),
		WorkspaceViewExpiresUnixNano:  expires.UnixNano(),
		WorkspaceLeaseExpiresUnixNano: expires.UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	reservation, err := contract.SealWorkspaceReadReservationV1(contract.WorkspaceReadReservationV1{StableKeyDigest: stable, AuthorizationDigest: mustWorkspaceReadDigest(t, "authorization"), RequestDigest: mustWorkspaceReadDigest(t, request), PayloadDigest: mustWorkspaceReadDigest(t, "payload"), Command: contract.Ref{ID: "command", Revision: 1, Digest: mustWorkspaceReadDigest(t, "command")}, WorkspaceView: contract.Ref{ID: "workspace", Revision: 1, Digest: mustWorkspaceReadDigest(t, "workspace")}, AttemptID: "attempt-1", TTLClosure: ttlClosure}, "reservation", now, expires)
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := contract.SealWorkspaceReadAttemptV1(contract.WorkspaceReadAttemptV1{StableKeyDigest: stable, RequestDigest: reservation.RequestDigest, PayloadDigest: reservation.PayloadDigest, Reservation: reservation.Meta.Ref(), AdmissionReceipt: admission, State: contract.WorkspaceReadStartedV1}, "attempt-1", 1, now, expires)
	if err != nil {
		t.Fatal(err)
	}
	return reservation, attempt
}

func mustWorkspaceReadDigest(t *testing.T, value string) string {
	t.Helper()
	digest, err := contract.Digest("workspace-read-test", value)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}
