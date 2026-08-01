package sqlite

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	runtimecore "github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

func TestWorkspaceReadLegacyReserveV1FailsClosedWithoutFacts(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(1_900_000_000, 0)
	expires := now.Add(time.Hour)
	store, err := OpenWithClock(ctx, filepath.Join(t.TempDir(), "sandbox.db"), func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	reservation, attempt := workspaceReadReservationFixture(t, now, expires, "legacy-reserve")
	binding := workspaceReadAdmissionAttemptBindingFixtureV1(t, reservation, attempt)
	if _, created, reserveErr := store.ReserveWorkspaceReadV1(ctx, reservation, attempt, binding); !errors.Is(reserveErr, ports.ErrConflict) || created {
		t.Fatalf("legacy Reserve did not fail closed: created=%v err=%v", created, reserveErr)
	}
	for _, table := range []string{
		"workspace_read_reservation",
		"workspace_read_attempt_origin",
		"workspace_read_attempt_current",
		"workspace_read_admission_attempt_binding",
		"workspace_read_runtime_attempt_admission_binding_v2",
	} {
		var rows int
		if err = store.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&rows); err != nil || rows != 0 {
			t.Fatalf("legacy Reserve wrote %s rows=%d err=%v", table, rows, err)
		}
	}
}

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
			projection, created, reserveErr := reserveWorkspaceReadFixtureV1(t, store, ctx, reservation, attempt)
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

func TestWorkspaceReadAdmissionHandoffReturnsOriginalAttemptAcrossConcurrencyRestartAndExpiry(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Unix(1_900_000_000, 0)
	expires := now.Add(time.Hour)
	current := now
	database := filepath.Join(t.TempDir(), "sandbox.db")
	store, err := OpenWithClock(ctx, database, func() time.Time { return current })
	if err != nil {
		t.Fatal(err)
	}
	reservation, attempt := workspaceReadReservationFixture(t, now, expires, "admission-handoff")
	binding := workspaceReadAdmissionAttemptBindingFixtureV1(t, reservation, attempt)
	if _, created, reserveErr := reserveWorkspaceReadFixtureV1(t, store, ctx, reservation, attempt); reserveErr != nil || !created {
		t.Fatalf("reserve: created=%v err=%v", created, reserveErr)
	}

	const readers = 64
	results := make(chan ports.WorkspaceReadAdmissionAttemptBindingV1, readers)
	errorsFound := make(chan error, readers)
	var group sync.WaitGroup
	for range readers {
		group.Add(1)
		go func() {
			defer group.Done()
			got, inspectErr := store.InspectWorkspaceReadAttemptForAdmissionV1(ctx, binding.AdmissionReceipt)
			if inspectErr != nil {
				errorsFound <- inspectErr
				return
			}
			results <- got
		}()
	}
	group.Wait()
	close(results)
	close(errorsFound)
	for inspectErr := range errorsFound {
		t.Fatal(inspectErr)
	}
	for got := range results {
		if got != binding || got.Attempt != binding.Attempt {
			t.Fatalf("concurrent admission handoff drifted: %#v", got)
		}
	}

	if _, err = store.MarkWorkspaceReadUnknownV1(ctx, attempt.Meta.Ref(), mustWorkspaceReadDigest(t, "admission-handoff-unknown")); !errors.Is(err, ports.ErrConflict) {
		t.Fatalf("legacy Unknown advanced a reference attempt: %v", err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	current = expires.Add(time.Second)
	reopened, err := OpenWithClock(ctx, database, func() time.Time { return current })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	recovered, err := reopened.InspectWorkspaceReadAttemptForAdmissionV1(ctx, binding.AdmissionReceipt)
	if err != nil || recovered != binding || recovered.Attempt != binding.Attempt {
		t.Fatalf("expired historical handoff lost original attempt: %#v err=%v", recovered, err)
	}
	projection, err := reopened.InspectBoundedWorkspaceReadV1(ctx, recovered.Attempt)
	if err != nil || projection.Attempt.State != contract.WorkspaceReadStartedV1 {
		t.Fatalf("original attempt did not inspect latest current: %#v err=%v", projection, err)
	}
}

func TestWorkspaceReadAdmissionHandoffRejectsEveryReceiptSplice(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Unix(1_900_000_000, 0)
	expires := now.Add(time.Hour)
	store, err := OpenWithClock(ctx, filepath.Join(t.TempDir(), "sandbox.db"), func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	reservation, attempt := workspaceReadReservationFixture(t, now, expires, "admission-splice")
	binding := workspaceReadAdmissionAttemptBindingFixtureV1(t, reservation, attempt)
	if _, created, reserveErr := reserveWorkspaceReadFixtureV1(t, store, ctx, reservation, attempt); reserveErr != nil || !created {
		t.Fatalf("reserve: created=%v err=%v", created, reserveErr)
	}

	splices := []runtimeports.ControlledOperationProviderAdmissionReceiptRefV2{
		mustWorkspaceReadAdmissionReceiptV1(t, binding.AdmissionReceipt.ID, 2, binding.AdmissionReceipt.StableKeyDigest, true, false),
		mustWorkspaceReadAdmissionReceiptV1(t, binding.AdmissionReceipt.ID, 1, runtimecore.DigestBytes([]byte("other-stable-key")), true, false),
		mustWorkspaceReadAdmissionReceiptV1(t, binding.AdmissionReceipt.ID, 1, binding.AdmissionReceipt.StableKeyDigest, false, true),
	}
	for _, splice := range splices {
		if _, inspectErr := store.InspectWorkspaceReadAttemptForAdmissionV1(ctx, splice); inspectErr == nil {
			t.Fatalf("spliced receipt reached original attempt: %#v", splice)
		}
	}
}

func TestWorkspaceReadExactReservationAndAttemptReadersSurviveRestartAndCurrentAdvance(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Unix(1_900_000_000, 0)
	expires := now.Add(time.Hour)
	database := filepath.Join(t.TempDir(), "sandbox.db")
	store, err := OpenWithClock(ctx, database, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	reservation, attempt := workspaceReadReservationFixture(t, now, expires, "exact-current-readers")
	if _, created, reserveErr := reserveWorkspaceReadFixtureV1(t, store, ctx, reservation, attempt); reserveErr != nil || !created {
		t.Fatalf("reserve: created=%v err=%v", created, reserveErr)
	}
	originalAttempt := contract.WorkspaceReadAttemptRefV1{
		ID: attempt.Meta.ID, Revision: attempt.Meta.Revision, Digest: attempt.Meta.Digest,
	}
	if got, inspectErr := store.InspectWorkspaceReadReservationExactV1(ctx, reservation.Meta.Ref()); inspectErr != nil || got != reservation {
		t.Fatalf("exact reservation drifted: %#v err=%v", got, inspectErr)
	}
	if got, inspectErr := store.InspectWorkspaceReadAttemptCurrentV1(ctx, originalAttempt); inspectErr != nil || got.State != contract.WorkspaceReadStartedV1 {
		t.Fatalf("exact attempt current drifted: %#v err=%v", got, inspectErr)
	}
	if _, err = store.MarkWorkspaceReadUnknownV1(ctx, attempt.Meta.Ref(), mustWorkspaceReadDigest(t, "exact-current-unknown")); !errors.Is(err, ports.ErrConflict) {
		t.Fatalf("legacy Unknown advanced the exact current: %v", err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenWithClock(ctx, database, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	if got, inspectErr := reopened.InspectWorkspaceReadReservationExactV1(ctx, reservation.Meta.Ref()); inspectErr != nil || got != reservation {
		t.Fatalf("restart exact reservation drifted: %#v err=%v", got, inspectErr)
	}
	if got, inspectErr := reopened.InspectWorkspaceReadAttemptCurrentV1(ctx, originalAttempt); inspectErr != nil || got.State != contract.WorkspaceReadStartedV1 {
		t.Fatalf("restart exact attempt did not return latest current: %#v err=%v", got, inspectErr)
	}
	splicedReservation := reservation.Meta.Ref()
	splicedReservation.Digest = mustWorkspaceReadDigest(t, "other-reservation")
	if _, err = reopened.InspectWorkspaceReadReservationExactV1(ctx, splicedReservation); !errors.Is(err, ports.ErrConflict) {
		t.Fatalf("reservation digest splice was accepted: %v", err)
	}
	splicedAttempt := originalAttempt
	splicedAttempt.Digest = mustWorkspaceReadDigest(t, "other-attempt")
	if _, err = reopened.InspectWorkspaceReadAttemptCurrentV1(ctx, splicedAttempt); !errors.Is(err, ports.ErrConflict) {
		t.Fatalf("attempt digest splice was accepted: %v", err)
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
	seedWorkspaceReadCompletionInputsV1(t, store, now, expires)
	started, created, err := reserveWorkspaceReadFixtureV1(t, store, ctx, reservation, attempt)
	if err != nil || !created || started.Attempt.State != contract.WorkspaceReadStartedV1 || started.AdmissionReceipt != attempt.AdmissionReceipt {
		t.Fatalf("reserve: created=%v state=%q err=%v", created, started.Attempt.State, err)
	}
	providerReceipt := contract.WorkspaceReadReceiptBindingV1{ID: "provider-receipt", Revision: 1, Digest: mustWorkspaceReadDigest(t, "provider-receipt"), ObservationDigest: mustWorkspaceReadDigest(t, "provider-observation"), StableKeyDigest: reservation.StableKeyDigest, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires.UnixNano()}
	content := "hello"
	observation, err := contract.SealWorkspaceReadObservationV1(contract.WorkspaceReadObservationV1{
		Reservation: reservation.Meta.Ref(), Command: reservation.Command, WorkspaceView: reservation.WorkspaceView,
		File:         contract.Ref{ID: mustWorkspaceReadFileIDV1(t, reservation.WorkspaceView.ID, "src/main.go"), Revision: reservation.WorkspaceView.Revision, Digest: mustWorkspaceReadDigest(t, "whole-file")},
		RelativePath: "src/main.go", StartByte: 2, ReturnedBytes: uint64(len(content)), TotalBytes: 2 + uint64(len(content)), Complete: true,
		Content: content, ContentDigest: contract.WorkspaceReadContentDigestV1([]byte(content), 2, 2+uint64(len(content)), true),
		S1CheckedUnixNano: now.UnixNano(), S2CheckedUnixNano: now.UnixNano(),
		AdmissionReceipt: attempt.AdmissionReceipt, ProviderReceipt: providerReceipt,
	}, "workspace-read-observation", now, expires)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.CompleteWorkspaceReadV1(ctx, attempt.Meta.Ref(), observation); !errors.Is(err, ports.ErrConflict) {
		t.Fatalf("legacy Complete accepted a caller-sealed observation: %v", err)
	}

	recovered, err := store.InspectBoundedWorkspaceReadV1(ctx, contract.WorkspaceReadAttemptRefV1{ID: attempt.Meta.ID, Revision: attempt.Meta.Revision, Digest: attempt.Meta.Digest})
	if err != nil || recovered.Attempt.State != contract.WorkspaceReadStartedV1 || recovered.Observation != nil || recovered.ProviderReceipt != nil {
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
	if _, _, err = reserveWorkspaceReadFixtureV1(t, store, ctx, reservation, attempt); err != nil {
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
	seedWorkspaceReadCompletionInputsV1(t, store, now, expires)
	if _, created, reserveErr := reserveWorkspaceReadFixtureV1(t, store, ctx, reservation, attempt); reserveErr != nil || !created {
		t.Fatalf("reserve: created=%v err=%v", created, reserveErr)
	}
	original := contract.WorkspaceReadAttemptRefV1{ID: attempt.Meta.ID, Revision: attempt.Meta.Revision, Digest: attempt.Meta.Digest}
	content := "hello"
	providerReceipt := contract.WorkspaceReadReceiptBindingV1{
		ID: "provider-concurrent", Revision: 1, Digest: mustWorkspaceReadDigest(t, "provider-concurrent"),
		ObservationDigest: mustWorkspaceReadDigest(t, "provider-observation-concurrent"),
		StableKeyDigest:   reservation.StableKeyDigest, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires.UnixNano(),
	}
	observation, err := contract.SealWorkspaceReadObservationV1(contract.WorkspaceReadObservationV1{
		Reservation: reservation.Meta.Ref(), Command: reservation.Command, WorkspaceView: reservation.WorkspaceView,
		File:         contract.Ref{ID: mustWorkspaceReadFileIDV1(t, reservation.WorkspaceView.ID, "src/main.go"), Revision: reservation.WorkspaceView.Revision, Digest: mustWorkspaceReadDigest(t, "whole-file")},
		RelativePath: "src/main.go", StartByte: 2, ReturnedBytes: uint64(len(content)), TotalBytes: 2 + uint64(len(content)), Complete: true,
		Content: content, ContentDigest: contract.WorkspaceReadContentDigestV1([]byte(content), 2, 2+uint64(len(content)), true),
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
		if !errors.Is(completeErr, ports.ErrConflict) {
			results <- errors.New("legacy Complete did not fail closed")
			return
		}
		results <- nil
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
	if err != nil || final.Attempt.State != contract.WorkspaceReadStartedV1 || final.Observation != nil {
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
	if _, created, reserveErr := reserveWorkspaceReadFixtureV1(t, first, ctx, reservation, attempt); reserveErr != nil || !created {
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
	if _, err = reopened.RecoverStartedWorkspaceReadAfterRestartV1(ctx, original); !errors.Is(err, ports.ErrConflict) {
		t.Fatalf("raw V1 restart recovery bypassed the Runtime-attempt V2 binding: %v", err)
	}
	after, err := reopened.InspectBoundedWorkspaceReadV1(ctx, original)
	if err != nil || after.Attempt.State != contract.WorkspaceReadStartedV1 || after.Observation != nil {
		t.Fatalf("rejected raw restart recovery mutated state: %#v err=%v", after, err)
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
	stable := "sha256:" + mustWorkspaceReadDigest(t, "ttl-stable")
	reservationDraft := contract.WorkspaceReadReservationV1{
		StableKeyDigest: stable, AuthorizationDigest: "sha256:" + mustWorkspaceReadDigest(t, "ttl-authorization"),
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
	runtimeAdmission, err := runtimeports.SealControlledOperationProviderAdmissionReceiptRefV2(runtimeports.ControlledOperationProviderAdmissionReceiptRefV2{
		ID: "ttl-admission", Revision: 1, StableKeyDigest: runtimecore.Digest(stable), Admitted: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	admission := contract.WorkspaceReadReceiptBindingV1{
		ID: runtimeAdmission.ID, Revision: uint64(runtimeAdmission.Revision), Digest: string(runtimeAdmission.Digest),
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
	if _, _, err = reserveWorkspaceReadFixtureV1(t, store, ctx, reservation, attempt); err == nil {
		t.Fatal("now==effective expiry reached the durable reservation")
	}
	current = now
	if _, created, err := reserveWorkspaceReadFixtureV1(t, store, ctx, reservation, attempt); err != nil || !created {
		t.Fatalf("current shortest-bound reservation: created=%v err=%v", created, err)
	}
	content := "ttl"
	overlongProvider := contract.WorkspaceReadReceiptBindingV1{
		ID: "ttl-provider", Revision: 1, Digest: mustWorkspaceReadDigest(t, "ttl-provider"),
		ObservationDigest: mustWorkspaceReadDigest(t, "ttl-provider-observation"),
		StableKeyDigest:   stable, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: outer.UnixNano(),
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
	if _, _, err = reserveWorkspaceReadFixtureV1(t, store, ctx, reservation, attempt); err == nil {
		t.Fatal("future admission receipt reached the durable reservation")
	}

	reservation, attempt = workspaceReadReservationFixture(t, now, expires, "request-observation-future")
	seedWorkspaceReadCompletionInputsV1(t, store, now, expires)
	if _, created, reserveErr := reserveWorkspaceReadFixtureV1(t, store, ctx, reservation, attempt); reserveErr != nil || !created {
		t.Fatalf("reserve observation fixture: created=%v err=%v", created, reserveErr)
	}
	future := now.Add(time.Second)
	content := "hello"
	providerReceipt := contract.WorkspaceReadReceiptBindingV1{
		ID: "provider-future", Revision: 1, Digest: mustWorkspaceReadDigest(t, "provider-future"),
		ObservationDigest: mustWorkspaceReadDigest(t, "provider-future-observation"),
		StableKeyDigest:   reservation.StableKeyDigest, CheckedUnixNano: future.UnixNano(), ExpiresUnixNano: expires.UnixNano(),
	}
	observation, err := contract.SealWorkspaceReadObservationV1(contract.WorkspaceReadObservationV1{
		Reservation: reservation.Meta.Ref(), Command: reservation.Command, WorkspaceView: reservation.WorkspaceView,
		File:         contract.Ref{ID: mustWorkspaceReadFileIDV1(t, reservation.WorkspaceView.ID, "src/main.go"), Revision: reservation.WorkspaceView.Revision, Digest: mustWorkspaceReadDigest(t, "whole-file")},
		RelativePath: "src/main.go", StartByte: 2, ReturnedBytes: uint64(len(content)), TotalBytes: 2 + uint64(len(content)), Complete: true,
		Content: content, ContentDigest: contract.WorkspaceReadContentDigestV1([]byte(content), 2, 2+uint64(len(content)), true),
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
	if _, _, err = reserveWorkspaceReadFixtureV1(t, store, ctx, reservation, attempt); err != nil {
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
	_, _, err = reserveWorkspaceReadFixtureV1(t, store, ctx, otherReservation, otherAttempt)
	if !errors.Is(err, ports.ErrConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
}

func TestWorkspaceReadCompletionRejectsExactCoordinateSplicesAndMissingInputs(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		mutate func(*testing.T, *Store, *contract.WorkspaceReadObservationV1)
	}{
		{name: "relative-path", mutate: func(_ *testing.T, _ *Store, value *contract.WorkspaceReadObservationV1) {
			value.RelativePath = "src/other.txt"
		}},
		{name: "start-byte", mutate: func(_ *testing.T, _ *Store, value *contract.WorkspaceReadObservationV1) {
			value.StartByte = 1
			value.TotalBytes = value.StartByte + value.ReturnedBytes
			value.ContentDigest = contract.WorkspaceReadContentDigestV1([]byte(value.Content), value.StartByte, value.TotalBytes, true)
		}},
		{name: "returned-over-command-max", mutate: func(_ *testing.T, _ *Store, value *contract.WorkspaceReadObservationV1) {
			value.Content = "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
			value.ReturnedBytes = uint64(len(value.Content))
			value.TotalBytes = value.StartByte + value.ReturnedBytes
			value.ContentDigest = contract.WorkspaceReadContentDigestV1([]byte(value.Content), value.StartByte, value.TotalBytes, true)
		}},
		{name: "command-ref", mutate: func(t *testing.T, _ *Store, value *contract.WorkspaceReadObservationV1) {
			value.Command = contract.Ref{ID: "other-command", Revision: 1, Digest: mustWorkspaceReadDigest(t, "other-command")}
		}},
		{name: "workspace-view-ref", mutate: func(t *testing.T, _ *Store, value *contract.WorkspaceReadObservationV1) {
			value.WorkspaceView = contract.Ref{ID: "other-workspace", Revision: 1, Digest: mustWorkspaceReadDigest(t, "other-workspace")}
		}},
		{name: "file-id", mutate: func(_ *testing.T, _ *Store, value *contract.WorkspaceReadObservationV1) {
			value.File.ID = "workspace-file-spliced"
		}},
		{name: "file-revision", mutate: func(_ *testing.T, _ *Store, value *contract.WorkspaceReadObservationV1) {
			value.File.Revision++
		}},
		{name: "missing-command", mutate: func(t *testing.T, store *Store, _ *contract.WorkspaceReadObservationV1) {
			if _, err := store.db.Exec(`DELETE FROM workspace_read_command_current`); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "raw-only-v18", mutate: func(t *testing.T, store *Store, _ *contract.WorkspaceReadObservationV1) {
			if _, err := store.db.Exec(`DELETE FROM workspace_read_command_owner_current_pointer_v2;
				DELETE FROM workspace_read_command_owner_current_history_v2;
				DELETE FROM workspace_read_command_publication_v2`); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "missing-publication", mutate: func(t *testing.T, store *Store, _ *contract.WorkspaceReadObservationV1) {
			if _, err := store.db.Exec(`DELETE FROM workspace_read_command_publication_v2`); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "missing-owner-current-history", mutate: func(t *testing.T, store *Store, _ *contract.WorkspaceReadObservationV1) {
			if _, err := store.db.Exec(`DELETE FROM workspace_read_command_owner_current_history_v2`); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "missing-owner-current-pointer", mutate: func(t *testing.T, store *Store, _ *contract.WorkspaceReadObservationV1) {
			if _, err := store.db.Exec(`DELETE FROM workspace_read_command_owner_current_pointer_v2`); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "owner-current-pointer-drift", mutate: func(t *testing.T, store *Store, _ *contract.WorkspaceReadObservationV1) {
			if _, err := store.db.Exec(
				`UPDATE workspace_read_command_owner_current_pointer_v2 SET current_digest=?`,
				mustWorkspaceReadDigest(t, "spliced-owner-current"),
			); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "expired-owner-current", mutate: func(_ *testing.T, store *Store, value *contract.WorkspaceReadObservationV1) {
			checked := time.Unix(0, value.S1CheckedUnixNano)
			store.clock = func() time.Time { return checked.Add(10 * time.Second) }
		}},
		{name: "missing-view", mutate: func(t *testing.T, store *Store, _ *contract.WorkspaceReadObservationV1) {
			if _, err := store.db.Exec(`DELETE FROM workspace_view_history`); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "file-scope", mutate: func(t *testing.T, store *Store, _ *contract.WorkspaceReadObservationV1) {
			var body []byte
			closure := workspaceReadCompletionPublishedClosureV2(t, time.Unix(1_900_000_000, 0))
			if err := store.db.QueryRow(`SELECT body FROM workspace_view_history WHERE view_id=?`, closure.fixture.Workspace.Meta.ID).Scan(&body); err != nil {
				t.Fatal(err)
			}
			var workspace contract.WorkspaceView
			if err := decode(body, &workspace); err != nil {
				t.Fatal(err)
			}
			workspace.FileScopeDigest = mustWorkspaceReadDigest(t, "other-file-scope")
			body, err := encode(workspace)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = store.db.Exec(`UPDATE workspace_view_history SET body=? WHERE view_id=?`, body, closure.fixture.Workspace.Meta.ID); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			now := time.Unix(1_900_000_000, 0)
			expires := now.Add(time.Hour)
			store, err := OpenWithClock(ctx, filepath.Join(t.TempDir(), "sandbox.db"), func() time.Time { return now })
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = store.Close() })
			reservation, attempt := workspaceReadReservationFixture(t, now, expires, "splice-"+testCase.name)
			seedWorkspaceReadCompletionInputsV1(t, store, now, expires)
			if _, created, reserveErr := reserveWorkspaceReadFixtureV1(t, store, ctx, reservation, attempt); reserveErr != nil || !created {
				t.Fatalf("reserve: created=%v err=%v", created, reserveErr)
			}
			observation := workspaceReadCompletionObservationFixtureV1(t, reservation, attempt, now, expires)
			testCase.mutate(t, store, &observation)
			observation, err = contract.SealWorkspaceReadObservationV1(observation, "observation-"+testCase.name, now, expires)
			if err != nil {
				t.Fatalf("seal mutated but structurally valid observation: %v", err)
			}
			if _, err = store.CompleteWorkspaceReadV1(ctx, attempt.Meta.Ref(), observation); err == nil {
				t.Fatal("spliced completion reached durable observed state")
			}
			projection, inspectErr := store.InspectBoundedWorkspaceReadV1(ctx, contract.WorkspaceReadAttemptRefV1{ID: attempt.Meta.ID, Revision: attempt.Meta.Revision, Digest: attempt.Meta.Digest})
			if inspectErr != nil || projection.Attempt.State != contract.WorkspaceReadStartedV1 || projection.Observation != nil {
				t.Fatalf("rejected splice mutated current: %#v err=%v", projection, inspectErr)
			}
		})
	}
}

func TestWorkspaceReadCompletionRejectsRawOnlyCommandAfterRestart(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Unix(1_900_000_000, 0)
	expires := now.Add(time.Hour)
	database := filepath.Join(t.TempDir(), "sandbox.db")
	store, err := OpenWithClock(ctx, database, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	reservation, attempt := workspaceReadReservationFixture(t, now, expires, "restart-raw-only-v18")
	seedWorkspaceReadCompletionInputsV1(t, store, now, expires)
	if _, created, reserveErr := reserveWorkspaceReadFixtureV1(t, store, ctx, reservation, attempt); reserveErr != nil || !created {
		t.Fatalf("reserve: created=%v err=%v", created, reserveErr)
	}
	observation := workspaceReadCompletionObservationFixtureV1(t, reservation, attempt, now, expires)
	observation, err = contract.SealWorkspaceReadObservationV1(observation, "observation-restart-raw-only-v18", now, expires)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.db.Exec("DELETE FROM workspace_read_command_owner_current_pointer_v2; " +
		"DELETE FROM workspace_read_command_owner_current_history_v2; " +
		"DELETE FROM workspace_read_command_publication_v2"); err != nil {
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenWithClock(ctx, database, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	if _, err = reopened.CompleteWorkspaceReadV1(ctx, attempt.Meta.Ref(), observation); err == nil {
		t.Fatal("raw-only v18 command completed after restart")
	}
	projection, inspectErr := reopened.InspectBoundedWorkspaceReadV1(ctx, contract.WorkspaceReadAttemptRefV1{ID: attempt.Meta.ID, Revision: attempt.Meta.Revision, Digest: attempt.Meta.Digest})
	if inspectErr != nil || projection.Attempt.State != contract.WorkspaceReadStartedV1 || projection.Observation != nil {
		t.Fatalf("restart rejection mutated current: %#v err=%v", projection, inspectErr)
	}
	var observations int
	if err = reopened.db.QueryRow("SELECT COUNT(*) FROM workspace_read_observation").Scan(&observations); err != nil {
		t.Fatal(err)
	}
	if observations != 0 {
		t.Fatalf("restart rejection wrote %d observations", observations)
	}
}

func TestWorkspaceReadSQLiteHasNoExportedRawCommandWriter(t *testing.T) {
	t.Parallel()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if parseErr != nil {
			t.Fatal(parseErr)
		}
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Recv != nil || !ast.IsExported(function.Name.Name) {
				continue
			}
			name := strings.ToLower(function.Name.Name)
			workspaceReadCommand := strings.Contains(name, "workspacereadcommand")
			rawWriter := strings.Contains(name, "seed") || strings.Contains(name, "raw") || strings.Contains(name, "write") || function.Name.Name == "CreateWorkspaceReadCommandV1"
			if workspaceReadCommand && rawWriter {
				t.Errorf("exported raw workspace-read Command writer %s exists in %s", function.Name.Name, path)
			}
		}
	}
}

func workspaceReadReservationFixture(t *testing.T, now, expires time.Time, request string) (contract.WorkspaceReadReservationV1, contract.WorkspaceReadAttemptV1) {
	t.Helper()
	closure := workspaceReadCompletionPublishedClosureV2(t, now)
	workspace, command := closure.fixture.Workspace, closure.command
	stable := "sha256:" + mustWorkspaceReadDigest(t, "stable")
	runtimeAdmission, err := runtimeports.SealControlledOperationProviderAdmissionReceiptRefV2(runtimeports.ControlledOperationProviderAdmissionReceiptRefV2{
		ID: "admission", Revision: 1, StableKeyDigest: runtimecore.Digest(stable), Admitted: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	admission := contract.WorkspaceReadReceiptBindingV1{
		ID: runtimeAdmission.ID, Revision: uint64(runtimeAdmission.Revision), Digest: string(runtimeAdmission.Digest),
		StableKeyDigest: stable, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires.UnixNano(),
	}
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
	reservation, err := contract.SealWorkspaceReadReservationV1(contract.WorkspaceReadReservationV1{StableKeyDigest: stable, AuthorizationDigest: "sha256:" + mustWorkspaceReadDigest(t, "authorization"), RequestDigest: mustWorkspaceReadDigest(t, request), PayloadDigest: mustWorkspaceReadDigest(t, "payload"), Command: command.Meta.Ref(), WorkspaceView: workspace.Meta.Ref(), AttemptID: "attempt-1", TTLClosure: ttlClosure}, "reservation", now, expires)
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := contract.SealWorkspaceReadAttemptV1(contract.WorkspaceReadAttemptV1{StableKeyDigest: stable, RequestDigest: reservation.RequestDigest, PayloadDigest: reservation.PayloadDigest, Reservation: reservation.Meta.Ref(), AdmissionReceipt: admission, State: contract.WorkspaceReadStartedV1}, "attempt-1", 1, now, expires)
	if err != nil {
		t.Fatal(err)
	}
	return reservation, attempt
}

func reserveWorkspaceReadFixtureV1(t *testing.T, store *Store, ctx context.Context, reservation contract.WorkspaceReadReservationV1, attempt contract.WorkspaceReadAttemptV1) (contract.WorkspaceReadExecutionProjectionV1, bool, error) {
	t.Helper()
	return store.reserveWorkspaceReadV1(ctx, reservation, attempt, workspaceReadAdmissionAttemptBindingFixtureV1(t, reservation, attempt), nil)
}

func workspaceReadAdmissionAttemptBindingFixtureV1(t *testing.T, reservation contract.WorkspaceReadReservationV1, attempt contract.WorkspaceReadAttemptV1) ports.WorkspaceReadAdmissionAttemptBindingV1 {
	t.Helper()
	admission := mustWorkspaceReadAdmissionReceiptV1(t, attempt.AdmissionReceipt.ID, runtimecore.Revision(attempt.AdmissionReceipt.Revision), runtimecore.Digest(attempt.AdmissionReceipt.StableKeyDigest), true, false)
	if string(admission.Digest) != attempt.AdmissionReceipt.Digest {
		t.Fatal("fixture Runtime admission digest drifted")
	}
	manifest := runtimecore.Digest("sha256:" + mustWorkspaceReadDigest(t, "sandbox-manifest"))
	domain := runtimeports.OperationDomainCommandRefV1{
		Owner: runtimeports.EffectOwnerRefV2{
			Role: runtimeports.OwnerSettlement, ComponentID: "praxis.sandbox/workspace-read",
			ManifestDigest: manifest,
		},
		Kind: runtimeports.NamespacedNameV2(contract.WorkspaceReadCommandKindV1),
		ID:   reservation.Command.ID, Revision: runtimecore.Revision(reservation.Command.Revision),
		Digest: runtimecore.Digest("sha256:" + reservation.Command.Digest),
	}
	binding, err := ports.SealWorkspaceReadAdmissionAttemptBindingV1(ports.WorkspaceReadAdmissionAttemptBindingV1{
		AdmissionReceipt: admission,
		Attempt: contract.WorkspaceReadAttemptRefV1{
			ID: attempt.Meta.ID, Revision: attempt.Meta.Revision, Digest: attempt.Meta.Digest,
		},
		Command:             reservation.Command,
		AuthorizationDigest: runtimecore.Digest(reservation.AuthorizationDigest),
		StableKeyDigest:     runtimecore.Digest(reservation.StableKeyDigest),
		Association: runtimeports.PreparedDomainCommandAssociationRefV1{
			ID: "workspace-read-association", Revision: 1,
			Digest: runtimecore.Digest("sha256:" + mustWorkspaceReadDigest(t, "association")),
		},
		DomainCommand:   domain,
		CreatedUnixNano: attempt.Meta.CreatedUnixNano,
		ExpiresUnixNano: attempt.Meta.ExpiresUnixNano,
	})
	if err != nil {
		t.Fatal(err)
	}
	return binding
}

func mustWorkspaceReadAdmissionReceiptV1(t *testing.T, id string, revision runtimecore.Revision, stable runtimecore.Digest, admitted, noEffect bool) runtimeports.ControlledOperationProviderAdmissionReceiptRefV2 {
	t.Helper()
	receipt, err := runtimeports.SealControlledOperationProviderAdmissionReceiptRefV2(runtimeports.ControlledOperationProviderAdmissionReceiptRefV2{
		ID: id, Revision: revision, StableKeyDigest: stable, Admitted: admitted, NoEffect: noEffect,
	})
	if err != nil {
		t.Fatal(err)
	}
	return receipt
}

func workspaceReadCompletionInputFixtureV1(t *testing.T, now, expires time.Time) (contract.WorkspaceView, contract.WorkspaceReadCommandV1) {
	t.Helper()
	scope := mustWorkspaceReadDigest(t, "file-scope")
	workspace := contract.WorkspaceView{
		Meta:            contract.Meta{ContractVersion: contract.ContractFamily, ID: "workspace", Revision: 1, Digest: mustWorkspaceReadDigest(t, "workspace"), CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano(), ExpiresUnixNano: expires.UnixNano()},
		BaseArtifactRef: contract.Ref{ID: "base", Revision: 1, Digest: mustWorkspaceReadDigest(t, "base")},
		BaseRevision:    "main",
		OverlayRef:      contract.Ref{ID: "overlay", Revision: 1, Digest: mustWorkspaceReadDigest(t, "overlay")},
		PolicyRef:       contract.Ref{ID: "policy", Revision: 1, Digest: mustWorkspaceReadDigest(t, "policy")},
		Lease: contract.RuntimeLeaseBinding{
			TenantID: "tenant", InstanceID: "instance", InstanceEpoch: 1, LeaseID: "lease", LeaseEpoch: 1,
			FenceEpoch: 1, ScopeDigest: mustWorkspaceReadDigest(t, "scope"), ObservedRevision: 1, ExpiresUnixNano: expires.UnixNano(),
		},
		ReadScopes: []string{"src"}, WriteScopes: []string{"src"}, FileScopeDigest: scope,
	}
	command, err := contract.SealWorkspaceReadCommandV1(contract.WorkspaceReadCommandV1{
		TenantID: "tenant", SourceToolCommand: contract.Ref{ID: "tool-command", Revision: 1, Digest: mustWorkspaceReadDigest(t, "tool-command")},
		SourceToolPayloadSchema: "praxis.tool/workspace-read@1", SourceToolPayloadDigest: mustWorkspaceReadDigest(t, "payload"), SourceToolPayloadRevision: 1,
		WorkspaceView: workspace.Meta.Ref(), FileScopeDigest: scope, RelativePath: "src/main.txt", MaxBytes: 5,
		ExpectedFileRef:           &contract.Ref{ID: mustWorkspaceReadFileIDV1(t, workspace.Meta.ID, "src/main.txt"), Revision: workspace.Meta.Revision, Digest: mustWorkspaceReadDigest(t, "whole-file")},
		RequestedNotAfterUnixNano: expires.UnixNano(), OperationDigest: mustWorkspaceReadDigest(t, "operation"), EffectID: "effect",
		IntentRevision: 1, IntentDigest: mustWorkspaceReadDigest(t, "intent"), AttemptID: "attempt-1",
		PreparedDigest: mustWorkspaceReadDigest(t, "prepared"), DispatchDigest: mustWorkspaceReadDigest(t, "dispatch"),
		ProviderComponent: "provider", ProviderManifest: mustWorkspaceReadDigest(t, "provider-manifest"),
	}, "command", now, expires)
	if err != nil {
		t.Fatal(err)
	}
	return workspace, command
}

func seedWorkspaceReadCompletionInputsV1(t *testing.T, store *Store, now, expires time.Time) {
	t.Helper()
	_ = expires
	closure := workspaceReadCompletionPublishedClosureV2(t, now)
	if _, err := store.CreateWorkspaceViewV1(context.Background(), closure.fixture.Workspace); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.ApplyWorkspaceReadCommandPublicationV2(
		context.Background(),
		closure.capability,
	); err != nil {
		t.Fatal(err)
	}
}

func workspaceReadCompletionPublishedClosureV2(
	t *testing.T,
	now time.Time,
) workspaceReadPublicationClosureV2 {
	t.Helper()
	return newWorkspaceReadPublicationClosureV2(
		t,
		testkit.WorkspaceReadCommandPublicationV2(now, "sqlite-completion"),
		now,
	)
}

func workspaceReadCompletionObservationFixtureV1(t *testing.T, reservation contract.WorkspaceReadReservationV1, attempt contract.WorkspaceReadAttemptV1, now, expires time.Time) contract.WorkspaceReadObservationV1 {
	t.Helper()
	content := "hello"
	const startByte = uint64(2)
	return contract.WorkspaceReadObservationV1{
		Reservation: reservation.Meta.Ref(), Command: reservation.Command, WorkspaceView: reservation.WorkspaceView,
		File:         contract.Ref{ID: mustWorkspaceReadFileIDV1(t, reservation.WorkspaceView.ID, "src/main.go"), Revision: reservation.WorkspaceView.Revision, Digest: mustWorkspaceReadDigest(t, "whole-file")},
		RelativePath: "src/main.go", StartByte: startByte, ReturnedBytes: uint64(len(content)), TotalBytes: startByte + uint64(len(content)), Complete: true,
		Content: content, ContentDigest: contract.WorkspaceReadContentDigestV1([]byte(content), startByte, startByte+uint64(len(content)), true),
		S1CheckedUnixNano: now.UnixNano(), S2CheckedUnixNano: now.UnixNano(),
		AdmissionReceipt: attempt.AdmissionReceipt,
		ProviderReceipt: contract.WorkspaceReadReceiptBindingV1{
			ID: "provider-receipt", Revision: 1, Digest: mustWorkspaceReadDigest(t, "provider-receipt"),
			ObservationDigest: mustWorkspaceReadDigest(t, "provider-observation"),
			StableKeyDigest:   reservation.StableKeyDigest, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires.UnixNano(),
		},
	}
}

func mustWorkspaceReadFileIDV1(t *testing.T, workspaceID, relativePath string) string {
	t.Helper()
	id, err := contract.WorkspaceReadFileIDV1(workspaceID, relativePath)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func mustWorkspaceReadDigest(t *testing.T, value string) string {
	t.Helper()
	digest, err := contract.Digest("workspace-read-test", value)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}
