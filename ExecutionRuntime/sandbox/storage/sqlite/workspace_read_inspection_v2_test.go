package sqlite

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	runtimecore "github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

func TestWorkspaceReadInspectionV2ReturnsEveryTerminalSuccessorAndKeepsV1(t *testing.T) {
	for _, state := range []contract.WorkspaceReadStateV1{
		contract.WorkspaceReadObservedV1,
		contract.WorkspaceReadFailedV1,
		contract.WorkspaceReadUnknownV1,
	} {
		t.Run(string(state), func(t *testing.T) {
			ctx := context.Background()
			now := time.Unix(1_900_000_000, 0)
			expires := now.Add(time.Hour)
			store, err := OpenWithClock(ctx, filepath.Join(t.TempDir(), "sandbox.db"), func() time.Time { return now })
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = store.Close() })
			reservation, attempt := seedWorkspaceReadInspectionTerminalV2(t, store, state, now, expires, string(state))
			origin := workspaceReadOriginRefV2(attempt)

			envelope, err := store.InspectBoundedWorkspaceReadV2(ctx, origin)
			if err != nil {
				t.Fatal(err)
			}
			if envelope.RequestedOriginAttemptRef != origin ||
				envelope.CurrentProjection.Attempt.State != state ||
				envelope.CurrentProjection.Attempt.Meta.Revision != origin.Revision+1 ||
				envelope.CurrentProjection.Reservation.Meta.Ref() != reservation.Meta.Ref() {
				t.Fatalf("terminal inspection did not close origin to current: %#v", envelope)
			}
			if err = envelope.ValidateCurrent(now); err != nil {
				t.Fatalf("fresh envelope is invalid: %v", err)
			}
			legacy, err := store.InspectBoundedWorkspaceReadV1(ctx, origin)
			if err != nil || legacy.Attempt.Meta.Ref() != envelope.CurrentProjection.Attempt.Meta.Ref() {
				t.Fatalf("V1 compatibility drifted: %#v err=%v", legacy, err)
			}
		})
	}
}

func TestWorkspaceReadInspectionV2SurvivesLostReplyRestartAndExpiredOrigin(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(1_900_000_000, 0)
	expires := now.Add(time.Hour)
	current := now
	database := filepath.Join(t.TempDir(), "sandbox.db")
	store, err := OpenWithClock(ctx, database, func() time.Time { return current })
	if err != nil {
		t.Fatal(err)
	}
	_, attempt := seedWorkspaceReadInspectionTerminalV2(
		t,
		store,
		contract.WorkspaceReadObservedV1,
		now,
		expires,
		"lost-reply",
	)
	origin := workspaceReadOriginRefV2(attempt)
	// Treat the successful completion reply as lost. Recovery keeps only the
	// original exact Attempt ref and never repeats the physical read.
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	current = expires.Add(time.Second)
	reopened, err := OpenWithClock(ctx, database, func() time.Time { return current })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })

	envelope, err := reopened.InspectBoundedWorkspaceReadV2(ctx, origin)
	if err != nil || envelope.CurrentProjection.Attempt.State != contract.WorkspaceReadObservedV1 {
		t.Fatalf("expired historical origin did not inspect terminal current: %#v err=%v", envelope, err)
	}
	if envelope.ExpiresUnixNano != current.Add(ports.WorkspaceReadInspectionMaxTTLV2).UnixNano() {
		t.Fatalf("inspection TTL was not owner-clock bounded: %#v", envelope)
	}
	if err = envelope.ValidateCurrent(time.Unix(0, envelope.ExpiresUnixNano)); err == nil {
		t.Fatal("now==inspection expiry was accepted")
	}
	if _, err = reopened.RecoverStartedWorkspaceReadAfterRestartV1(ctx, origin); err == nil {
		t.Fatal("expired terminal history was treated as execution recovery eligibility")
	}
}

func TestWorkspaceReadInspectionV2RejectsOriginAndLineageSplices(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(1_900_000_000, 0)
	expires := now.Add(time.Hour)

	t.Run("origin revision and digest", func(t *testing.T) {
		store, _, attempt := openWorkspaceReadInspectionFixtureV2(t, now, expires, "origin-splice")
		origin := workspaceReadOriginRefV2(attempt)
		for _, splice := range []contract.WorkspaceReadAttemptRefV1{
			{ID: origin.ID, Revision: origin.Revision + 1, Digest: origin.Digest},
			{ID: origin.ID, Revision: origin.Revision, Digest: mustWorkspaceReadDigest(t, "other-origin")},
		} {
			if _, err := store.InspectBoundedWorkspaceReadV2(ctx, splice); !errors.Is(err, ports.ErrConflict) {
				t.Fatalf("spliced origin was accepted: %#v err=%v", splice, err)
			}
		}
	})

	mutations := map[string]func(*testing.T, *Store, contract.WorkspaceReadReservationV1, contract.WorkspaceReadAttemptV1){
		"reservation": func(t *testing.T, store *Store, reservation contract.WorkspaceReadReservationV1, _ contract.WorkspaceReadAttemptV1) {
			reservation.RequestDigest = mustWorkspaceReadDigest(t, "spliced-reservation-request")
			spliced, err := contract.SealWorkspaceReadReservationV1(
				reservation,
				reservation.Meta.ID,
				now,
				expires,
			)
			if err != nil {
				t.Fatal(err)
			}
			body, _ := encode(spliced)
			if _, err = store.db.Exec(`UPDATE workspace_read_reservation SET body=? WHERE stable_digest=?`, body, reservation.StableKeyDigest); err != nil {
				t.Fatal(err)
			}
		},
		"command": func(t *testing.T, store *Store, reservation contract.WorkspaceReadReservationV1, _ contract.WorkspaceReadAttemptV1) {
			var body []byte
			if err := store.db.QueryRow(`SELECT body FROM workspace_read_command_current WHERE command_id=?`, reservation.Command.ID).Scan(&body); err != nil {
				t.Fatal(err)
			}
			var command contract.WorkspaceReadCommandV1
			if err := decode(body, &command); err != nil {
				t.Fatal(err)
			}
			command.RelativePath = "src/spliced.txt"
			command.ExpectedFileRef = nil
			spliced, err := contract.SealWorkspaceReadCommandV1(command, command.Meta.ID, now, expires)
			if err != nil {
				t.Fatal(err)
			}
			body, _ = encode(spliced)
			if _, err = store.db.Exec(
				`UPDATE workspace_read_command_current SET revision=?,digest=?,body=? WHERE command_id=?`,
				spliced.Meta.Revision,
				spliced.Meta.Digest,
				body,
				spliced.Meta.ID,
			); err != nil {
				t.Fatal(err)
			}
		},
		"admission": func(t *testing.T, store *Store, _ contract.WorkspaceReadReservationV1, attempt contract.WorkspaceReadAttemptV1) {
			var body []byte
			if err := store.db.QueryRow(`SELECT body FROM workspace_read_admission_attempt_binding WHERE attempt_id=?`, attempt.Meta.ID).Scan(&body); err != nil {
				t.Fatal(err)
			}
			var binding ports.WorkspaceReadAdmissionAttemptBindingV1
			if err := decode(body, &binding); err != nil {
				t.Fatal(err)
			}
			binding.AuthorizationDigest = runtimecore.Digest("sha256:" + mustWorkspaceReadDigest(t, "spliced-authorization"))
			spliced, err := ports.SealWorkspaceReadAdmissionAttemptBindingV1(binding)
			if err != nil {
				t.Fatal(err)
			}
			body, _ = encode(spliced)
			if _, err = store.db.Exec(`UPDATE workspace_read_admission_attempt_binding SET body=? WHERE attempt_id=?`, body, attempt.Meta.ID); err != nil {
				t.Fatal(err)
			}
		},
		"current": func(t *testing.T, store *Store, _ contract.WorkspaceReadReservationV1, attempt contract.WorkspaceReadAttemptV1) {
			var body []byte
			if err := store.db.QueryRow(`SELECT body FROM workspace_read_attempt_current WHERE attempt_id=?`, attempt.Meta.ID).Scan(&body); err != nil {
				t.Fatal(err)
			}
			var current contract.WorkspaceReadAttemptV1
			if err := decode(body, &current); err != nil {
				t.Fatal(err)
			}
			current.RequestDigest = mustWorkspaceReadDigest(t, "spliced-current-request")
			spliced, err := contract.SealWorkspaceReadAttemptV1(current, current.Meta.ID, current.Meta.Revision, now, expires)
			if err != nil {
				t.Fatal(err)
			}
			body, _ = encode(spliced)
			if _, err = store.db.Exec(
				`UPDATE workspace_read_attempt_current SET revision=?,digest=?,body=? WHERE attempt_id=?`,
				spliced.Meta.Revision,
				spliced.Meta.Digest,
				body,
				spliced.Meta.ID,
			); err != nil {
				t.Fatal(err)
			}
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			store, reservation, attempt := openWorkspaceReadInspectionFixtureV2(t, now, expires, "splice-"+name)
			mutate(t, store, reservation, attempt)
			if _, err := store.InspectBoundedWorkspaceReadV2(ctx, workspaceReadOriginRefV2(attempt)); !errors.Is(err, ports.ErrConflict) {
				t.Fatalf("%s splice was accepted: %v", name, err)
			}
		})
	}
}

func TestWorkspaceReadInspectionV2ConcurrentInspectIsReadOnly(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(1_900_000_000, 0)
	expires := now.Add(time.Hour)
	store, _, attempt := openWorkspaceReadInspectionFixtureV2(t, now, expires, "concurrent-inspect")
	origin := workspaceReadOriginRefV2(attempt)
	before := workspaceReadInspectionRowCountsV2(t, store)

	const readers = 64
	results := make(chan ports.WorkspaceReadInspectionEnvelopeV2, readers)
	failures := make(chan error, readers)
	var group sync.WaitGroup
	for range readers {
		group.Add(1)
		go func() {
			defer group.Done()
			envelope, err := store.InspectBoundedWorkspaceReadV2(ctx, origin)
			if err != nil {
				failures <- err
				return
			}
			results <- envelope
		}()
	}
	group.Wait()
	close(results)
	close(failures)
	for err := range failures {
		t.Fatal(err)
	}
	var digest string
	for envelope := range results {
		if digest == "" {
			digest = envelope.ProjectionDigest
		}
		if envelope.ProjectionDigest != digest ||
			envelope.CurrentProjection.Attempt.State != contract.WorkspaceReadUnknownV1 {
			t.Fatalf("concurrent exact Inspect drifted: %#v", envelope)
		}
	}
	after := workspaceReadInspectionRowCountsV2(t, store)
	if before != after {
		t.Fatalf("64 read-only inspections wrote owner state: before=%v after=%v", before, after)
	}
}

func TestWorkspaceReadInspectionV2RejectsClockRollbackDigestTamperStrictJSONAndTypedNil(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(1_900_000_000, 0)
	expires := now.Add(time.Hour)
	var clock atomic.Int64
	clock.Store(now.UnixNano())
	store, err := OpenWithClock(ctx, filepath.Join(t.TempDir(), "sandbox.db"), func() time.Time {
		return time.Unix(0, clock.Load())
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	_, attempt := seedWorkspaceReadInspectionTerminalV2(
		t,
		store,
		contract.WorkspaceReadUnknownV1,
		now,
		expires,
		"strict",
	)
	origin := workspaceReadOriginRefV2(attempt)
	envelope, err := store.InspectBoundedWorkspaceReadV2(ctx, origin)
	if err != nil {
		t.Fatal(err)
	}

	tampered := envelope
	tampered.CheckedUnixNano++
	if err = tampered.ValidateShape(); err == nil {
		t.Fatal("inspection digest did not bind CheckedUnixNano")
	}
	widened := envelope
	widened.ExpiresUnixNano = widened.CheckedUnixNano + ports.WorkspaceReadInspectionMaxTTLV2.Nanoseconds() + 1
	widened.ProjectionDigest = ""
	if _, err = ports.SealWorkspaceReadInspectionEnvelopeV2(widened); err == nil {
		t.Fatal("inspection envelope exceeded the owner TTL cap")
	}
	wire, err := ports.EncodeWorkspaceReadInspectionEnvelopeV2(envelope)
	if err != nil {
		t.Fatal(err)
	}
	for name, payload := range map[string][]byte{
		"nested unknown": bytes.Replace(
			wire,
			[]byte(`"current_projection":{`),
			[]byte(`"current_projection":{"unknown_field":true,`),
			1,
		),
		"nested duplicate": bytes.Replace(
			wire,
			[]byte(`"requested_origin_attempt_ref":{`),
			[]byte(`"requested_origin_attempt_ref":{"id":"duplicate",`),
			1,
		),
		"trailing": append(append([]byte(nil), wire...), []byte(` {}`)...),
	} {
		t.Run(name, func(t *testing.T) {
			if _, decodeErr := ports.DecodeWorkspaceReadInspectionEnvelopeV2(payload); decodeErr == nil {
				t.Fatalf("%s JSON was accepted", name)
			}
		})
	}

	clock.Store(now.Add(-time.Nanosecond).UnixNano())
	if _, err = store.InspectBoundedWorkspaceReadV2(ctx, origin); !errors.Is(err, ports.ErrConflict) {
		t.Fatalf("clock rollback was accepted: %v", err)
	}
	var nilStore *Store
	if _, err = nilStore.InspectBoundedWorkspaceReadV2(ctx, origin); err == nil {
		t.Fatal("typed-nil Store was accepted")
	}
}

func openWorkspaceReadInspectionFixtureV2(
	t *testing.T,
	now time.Time,
	expires time.Time,
	name string,
) (*Store, contract.WorkspaceReadReservationV1, contract.WorkspaceReadAttemptV1) {
	t.Helper()
	store, err := OpenWithClock(context.Background(), filepath.Join(t.TempDir(), "sandbox.db"), func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	reservation, attempt := seedWorkspaceReadInspectionTerminalV2(
		t,
		store,
		contract.WorkspaceReadUnknownV1,
		now,
		expires,
		name,
	)
	return store, reservation, attempt
}

func seedWorkspaceReadInspectionTerminalV2(
	t *testing.T,
	store *Store,
	state contract.WorkspaceReadStateV1,
	now time.Time,
	expires time.Time,
	name string,
) (contract.WorkspaceReadReservationV1, contract.WorkspaceReadAttemptV1) {
	t.Helper()
	ctx := context.Background()
	reservation, attempt := workspaceReadReservationFixture(t, now, expires, name)
	_, command := workspaceReadCompletionInputFixtureV1(t, now, expires)
	reservation.RequestDigest = command.Meta.Digest
	var err error
	reservation, err = contract.SealWorkspaceReadReservationV1(
		reservation,
		reservation.Meta.ID,
		now,
		expires,
	)
	if err != nil {
		t.Fatal(err)
	}
	attempt.RequestDigest = reservation.RequestDigest
	attempt.Reservation = reservation.Meta.Ref()
	attempt, err = contract.SealWorkspaceReadAttemptV1(
		attempt,
		attempt.Meta.ID,
		attempt.Meta.Revision,
		now,
		expires,
	)
	if err != nil {
		t.Fatal(err)
	}
	seedWorkspaceReadCompletionInputsV1(t, store, now, expires)
	if _, created, err := reserveWorkspaceReadFixtureV1(t, store, ctx, reservation, attempt); err != nil || !created {
		t.Fatalf("reserve inspection fixture: created=%v err=%v", created, err)
	}
	switch state {
	case contract.WorkspaceReadObservedV1:
		observation := workspaceReadCompletionObservationFixtureV1(t, reservation, attempt, now, expires)
		sealed, err := contract.SealWorkspaceReadObservationV1(observation, "inspection-observation-"+name, now, expires)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = store.CompleteWorkspaceReadV1(ctx, attempt.Meta.Ref(), sealed); err != nil {
			t.Fatal(err)
		}
	case contract.WorkspaceReadFailedV1:
		if _, err := store.FailWorkspaceReadV1(ctx, attempt.Meta.Ref(), mustWorkspaceReadDigest(t, "failed-"+name)); err != nil {
			t.Fatal(err)
		}
	case contract.WorkspaceReadUnknownV1:
		if _, err := store.MarkWorkspaceReadUnknownV1(ctx, attempt.Meta.Ref(), mustWorkspaceReadDigest(t, "unknown-"+name)); err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatalf("unsupported terminal fixture state %q", state)
	}
	return reservation, attempt
}

func workspaceReadOriginRefV2(attempt contract.WorkspaceReadAttemptV1) contract.WorkspaceReadAttemptRefV1 {
	return contract.WorkspaceReadAttemptRefV1{
		ID:       attempt.Meta.ID,
		Revision: attempt.Meta.Revision,
		Digest:   attempt.Meta.Digest,
	}
}

func workspaceReadInspectionRowCountsV2(t *testing.T, store *Store) [5]int {
	t.Helper()
	tables := []string{
		"workspace_read_reservation",
		"workspace_read_attempt_origin",
		"workspace_read_attempt_current",
		"workspace_read_admission_attempt_binding",
		"workspace_read_observation",
	}
	var counts [5]int
	for index, table := range tables {
		if err := store.db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s", table)).Scan(&counts[index]); err != nil {
			t.Fatal(err)
		}
	}
	return counts
}
