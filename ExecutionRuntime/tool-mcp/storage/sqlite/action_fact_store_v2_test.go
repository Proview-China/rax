package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolaction "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/action"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
)

type sqliteEchoOwnerReaderV2 struct{}

func (sqliteEchoOwnerReaderV2) InspectOwnerCurrentV1(_ context.Context, value toolcontract.OwnerCurrentRefV1) (toolcontract.OwnerCurrentRefV1, error) {
	return value, nil
}

type sqliteDriftingOwnerReaderV2 struct {
	calls      atomic.Int32
	driftAfter int32
}

func (r *sqliteDriftingOwnerReaderV2) InspectOwnerCurrentV1(_ context.Context, value toolcontract.OwnerCurrentRefV1) (toolcontract.OwnerCurrentRefV1, error) {
	if r.calls.Add(1) > r.driftAfter {
		value.Digest = testkit.Digest("owner-current-drift")
	}
	return value, nil
}

type sqliteExactObservationReaderV2 struct{}

func (sqliteExactObservationReaderV2) InspectProviderObservationExactV1(_ context.Context, value runtimeports.ProviderAttemptObservationRefV2) (runtimeports.ProviderAttemptObservationRefV2, error) {
	return value, nil
}

type sqliteExactEnforcementReaderV2 struct{}

func (sqliteExactEnforcementReaderV2) InspectEnforcementPhaseExactV1(_ context.Context, value runtimeports.OperationDispatchEnforcementPhaseRefV4) (runtimeports.OperationDispatchEnforcementPhaseRefV4, error) {
	return value, nil
}

type sqliteExactConsumptionReaderV2 struct{}

func (sqliteExactConsumptionReaderV2) InspectEvidenceConsumptionExactV1(_ context.Context, value runtimeports.OperationScopeEvidenceConsumptionRefV3) (runtimeports.OperationScopeEvidenceConsumptionRefV3, error) {
	return value, nil
}

type sqliteSettlementReaderV2 struct {
	mu                sync.RWMutex
	current           runtimeports.OperationInspectionSettlementRefV4
	association       runtimeports.OperationSettlementEvidenceAssociationV4
	drift             bool
	driftCurrentAfter int32
	driftAssocAfter   int32
	currentCalls      atomic.Int32
	associationCalls  atomic.Int32
}

type sqliteMutableClockV2 struct {
	unixNano atomic.Int64
}

func newSQLiteMutableClockV2(now time.Time) *sqliteMutableClockV2 {
	clock := &sqliteMutableClockV2{}
	clock.Set(now)
	return clock
}

func (c *sqliteMutableClockV2) Now() time.Time {
	return time.Unix(0, c.unixNano.Load())
}

func (c *sqliteMutableClockV2) Set(now time.Time) {
	c.unixNano.Store(now.UnixNano())
}

type cancelAfterFirstErrContextV2 struct {
	calls atomic.Int32
}

func (*cancelAfterFirstErrContextV2) Deadline() (time.Time, bool) { return time.Time{}, false }
func (*cancelAfterFirstErrContextV2) Done() <-chan struct{}       { return nil }
func (c *cancelAfterFirstErrContextV2) Err() error {
	if c.calls.Add(1) > 1 {
		return context.Canceled
	}
	return nil
}
func (*cancelAfterFirstErrContextV2) Value(any) any { return nil }

func (r *sqliteSettlementReaderV2) InspectCurrentOperationSettlementV4(_ context.Context, request runtimeports.InspectCurrentOperationSettlementRequestV4) (runtimeports.OperationInspectionSettlementRefV4, error) {
	call := r.currentCalls.Add(1)
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.drift || (r.driftCurrentAfter > 0 && call > r.driftCurrentAfter) ||
		!runtimeports.SameOperationSubjectV3(request.Operation, r.current.DomainResult.Operation) ||
		request.EffectID != r.current.Settlement.EffectID {
		return runtimeports.OperationInspectionSettlementRefV4{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "injected Runtime settlement drift")
	}
	return r.current, nil
}

func (r *sqliteSettlementReaderV2) InspectOperationSettlementEvidenceAssociationV4(_ context.Context, operation runtimeports.OperationSubjectV3, ref runtimeports.OperationSettlementEvidenceAssociationRefV4) (runtimeports.OperationSettlementEvidenceAssociationV4, error) {
	call := r.associationCalls.Add(1)
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.drift || (r.driftAssocAfter > 0 && call > r.driftAssocAfter) ||
		!runtimeports.SameOperationSubjectV3(operation, r.current.DomainResult.Operation) ||
		!runtimeports.SameOperationSettlementEvidenceAssociationRefV4(ref, r.association.RefV4()) {
		return runtimeports.OperationSettlementEvidenceAssociationV4{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "injected Runtime association drift")
	}
	return r.association, nil
}

func sqliteActionReadersV2() toolaction.CausalReadersV1 {
	return toolaction.CausalReadersV1{OwnerCurrent: sqliteEchoOwnerReaderV2{}, Observation: sqliteExactObservationReaderV2{}, Enforcement: sqliteExactEnforcementReaderV2{}, Consumption: sqliteExactConsumptionReaderV2{}, Settlement: &sqliteSettlementReaderV2{}}
}

func sqliteActionConfigV2(path string, now time.Time) ConfigV1 {
	return ConfigV1{Path: path, Owner: core.OwnerRef{Domain: "tool-mcp", ID: "action-facts-v2"}, Clock: func() time.Time { return now.Add(7 * time.Second) }}
}

func TestSQLiteActionFactLifecycleRestartWALAndCorruption(t *testing.T) {
	ctx := context.Background()
	now := testkit.FixedTime
	path := filepath.Join(t.TempDir(), "action-facts.db")
	store, err := OpenActionFactStoreV2(ctx, sqliteActionConfigV2(path, now), sqliteActionReadersV2())
	if err != nil {
		t.Fatal(err)
	}
	candidate, reservation, domain, inspection := sqliteActionFixtureV2(t, store, now)
	result, err := store.ApplySettlementAndCreateResultV2(ctx, candidate.ID, objectRefActionV2(domain.ID, domain.Revision, domain.Digest), inspection, domain.Outcome, domain.Disposition, now.Add(7*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if result.Reservation != objectRefActionV2(reservation.ID, reservation.Revision, reservation.Digest) {
		t.Fatal("settled ToolResult lost Reservation exact ref")
	}
	var journal, synchronous string
	if err = store.store.db.QueryRow(`PRAGMA journal_mode`).Scan(&journal); err != nil || !strings.EqualFold(journal, "wal") {
		t.Fatalf("journal=%q err=%v", journal, err)
	}
	if err = store.store.db.QueryRow(`PRAGMA synchronous`).Scan(&synchronous); err != nil || synchronous == "0" {
		t.Fatalf("synchronous=%q err=%v", synchronous, err)
	}
	var foreignKeys int
	if err = store.store.db.QueryRow(`PRAGMA foreign_keys`).Scan(&foreignKeys); err != nil || foreignKeys != 1 {
		t.Fatalf("foreign_keys=%d err=%v", foreignKeys, err)
	}
	rows, err := store.store.db.Query(`SELECT name FROM sqlite_master WHERE type='table'`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var name string
		_ = rows.Scan(&name)
		if strings.HasPrefix(name, "runtime_") || strings.Contains(name, "sandbox") || strings.Contains(name, "application_result") {
			t.Fatalf("cross-owner table present: %s", name)
		}
	}
	_ = rows.Close()
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = OpenActionFactStoreV2(ctx, sqliteActionConfigV2(path, now.Add(8*time.Second)), sqliteActionReadersV2())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	exact := objectRefActionV2(result.ID, result.Revision, result.Digest)
	recovered, err := store.InspectResultExactV2(ctx, candidate.ID, exact)
	if err != nil || recovered.Digest != result.Digest {
		t.Fatalf("restart result=%#v err=%v", recovered, err)
	}
	settled, err := store.InspectSettledResultForApplyV2(ctx, candidate.ID, result.Apply)
	if err != nil || settled.Digest != result.Digest {
		t.Fatalf("restart settled=%#v err=%v", settled, err)
	}
	if _, err = store.store.db.Exec(`UPDATE tool_result_v2 SET body_json=? WHERE action_id=?`, []byte("{}"), candidate.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = store.InspectResultExactV2(ctx, candidate.ID, exact); err == nil || !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("corrupt result row error=%v, want Conflict", err)
	}
}

func TestActionFactStoreV2InMemorySQLiteConformance(t *testing.T) {
	now := testkit.FixedTime
	memoryReaders := sqliteActionReadersV2()
	memory, err := toolaction.NewContextFactStoreV2(toolaction.NewStoreV2(memoryReaders))
	if err != nil {
		t.Fatal(err)
	}
	durable, err := OpenActionFactStoreV2(context.Background(), sqliteActionConfigV2(filepath.Join(t.TempDir(), "parity.db"), now), sqliteActionReadersV2())
	if err != nil {
		t.Fatal(err)
	}
	defer durable.Close()
	var digests []core.Digest
	for index, store := range []toolaction.FactStoreV2{memory, durable} {
		candidate, _, domain, inspection := sqliteActionFixtureV2(t, store, now)
		if index == 0 {
			_, association := sqliteInspectionV2(t, domain, testkit.BoundaryFixture(now), now.Add(7*time.Second))
			reader := memoryReaders.Settlement.(*sqliteSettlementReaderV2)
			reader.current, reader.association = inspection, association
		}
		result, applyErr := store.ApplySettlementAndCreateResultV2(context.Background(), candidate.ID, objectRefActionV2(domain.ID, domain.Revision, domain.Digest), inspection, domain.Outcome, domain.Disposition, now.Add(7*time.Second))
		if applyErr != nil {
			t.Fatal(applyErr)
		}
		exact, inspectErr := store.InspectResultExactV2(context.Background(), candidate.ID, objectRefActionV2(result.ID, result.Revision, result.Digest))
		if inspectErr != nil {
			t.Fatal(inspectErr)
		}
		settled, inspectErr := store.InspectSettledResultForApplyV2(context.Background(), candidate.ID, result.Apply)
		if inspectErr != nil || settled.Digest != exact.Digest {
			t.Fatalf("settled=%#v exact=%#v err=%v", settled, exact, inspectErr)
		}
		digests = append(digests, result.Digest)
	}
	if digests[0] != digests[1] {
		t.Fatalf("in-memory=%s sqlite=%s", digests[0], digests[1])
	}
}

func TestActionFactStoreV2InMemorySQLiteSettlementS2DriftWritesZero(t *testing.T) {
	now := testkit.FixedTime
	type storeCaseV2 struct {
		name string
		open func(toolaction.CausalReadersV1) (toolaction.FactStoreV2, func())
	}
	cases := []storeCaseV2{
		{
			name: "in-memory",
			open: func(readers toolaction.CausalReadersV1) (toolaction.FactStoreV2, func()) {
				store, err := toolaction.NewContextFactStoreV2(toolaction.NewStoreV2(readers))
				if err != nil {
					t.Fatal(err)
				}
				return store, func() {}
			},
		},
		{
			name: "sqlite",
			open: func(readers toolaction.CausalReadersV1) (toolaction.FactStoreV2, func()) {
				store, err := OpenActionFactStoreV2(context.Background(), sqliteActionConfigV2(filepath.Join(t.TempDir(), "settlement-s2-drift.db"), now), readers)
				if err != nil {
					t.Fatal(err)
				}
				return store, func() { _ = store.Close() }
			},
		},
	}
	for _, axis := range []string{"current", "association"} {
		for _, test := range cases {
			t.Run(test.name+"_"+axis, func(t *testing.T) {
				readers := sqliteActionReadersV2()
				settlement := readers.Settlement.(*sqliteSettlementReaderV2)
				store, closeStore := test.open(readers)
				defer closeStore()
				candidate, _, domain, inspection := sqliteActionFixtureV2(t, store, now)
				_, association := sqliteInspectionV2(t, domain, testkit.BoundaryFixture(now), now.Add(7*time.Second))
				settlement.mu.Lock()
				settlement.current, settlement.association = inspection, association
				if axis == "current" {
					settlement.driftCurrentAfter = 1
				} else {
					settlement.driftAssocAfter = 1
				}
				settlement.mu.Unlock()

				mutationNow := now.Add(7 * time.Second)
				_, err := store.ApplySettlementAndCreateResultV2(
					context.Background(), candidate.ID,
					objectRefActionV2(domain.ID, domain.Revision, domain.Digest),
					inspection, domain.Outcome, domain.Disposition, mutationNow,
				)
				if err == nil || !core.HasCategory(err, core.ErrorConflict) {
					t.Fatalf("S2 %s drift error=%v, want Conflict", axis, err)
				}
				expected := sqliteExpectedToolResultV2(t, domain, inspection, mutationNow)
				if _, err = store.InspectResultExactV2(context.Background(), candidate.ID, objectRefActionV2(expected.ID, expected.Revision, expected.Digest)); err == nil || !core.HasCategory(err, core.ErrorNotFound) {
					t.Fatalf("S2 %s drift wrote ToolResult: %v", axis, err)
				}
			})
		}
	}
}

func TestActionFactStoreV2PostCommitCancellationIsIndeterminateAndInspectable(t *testing.T) {
	t.Run("in-memory candidate", func(t *testing.T) {
		now := testkit.FixedTime
		store, err := toolaction.NewContextFactStoreV2(toolaction.NewStoreV2(sqliteActionReadersV2()))
		if err != nil {
			t.Fatal(err)
		}
		candidate := testkit.CandidateV2(now)
		if _, err = store.CreateCandidateFactV2(&cancelAfterFirstErrContextV2{}, candidate); err == nil || !core.HasCategory(err, core.ErrorIndeterminate) {
			t.Fatalf("post-commit in-memory error=%v, want Indeterminate", err)
		}
		exact := objectRefActionV2(candidate.ID, candidate.Revision, candidate.Digest)
		if _, err = store.InspectCandidateCurrentV2(context.Background(), exact, now.Add(time.Second)); err != nil {
			t.Fatalf("committed in-memory Candidate exact Inspect failed: %v", err)
		}
	})

	for _, stage := range []string{"candidate", "reservation", "domain_result", "settled"} {
		t.Run("sqlite "+stage, func(t *testing.T) {
			ctx := context.Background()
			now := testkit.FixedTime
			store, err := OpenActionFactStoreV2(ctx, sqliteActionConfigV2(filepath.Join(t.TempDir(), "post-commit-cancel.db"), now), sqliteActionReadersV2())
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()

			candidate := testkit.CandidateV2(now)
			actionRef := objectRefActionV2(candidate.ID, candidate.Revision, candidate.Digest)
			appAttempt := toolcontract.ApplicationAttemptRefV1{ID: "post-commit-attempt-v2", Revision: 1, Digest: testkit.Digest("post-commit-attempt-v2")}
			var reservation toolcontract.ActionReservationFactV2
			var domain toolcontract.ToolDomainResultFactV2
			var inspection runtimeports.OperationInspectionSettlementRefV4
			if stage != "candidate" {
				if _, err = store.CreateCandidateFactV2(ctx, candidate); err != nil {
					t.Fatal(err)
				}
			}
			if stage == "domain_result" || stage == "settled" {
				reservation, err = store.CreateReservationFactV2(ctx, actionRef, appAttempt, testkit.Digest("post-commit-intent-v2"), candidate.SessionID, testkit.Digest("post-commit-subject-v2"), now.Add(time.Second), now.Add(20*time.Second))
				if err != nil {
					t.Fatal(err)
				}
				var association runtimeports.OperationSettlementEvidenceAssociationV4
				domain, inspection, association = sqliteDomainFixtureV2(t, candidate, reservation, appAttempt, now)
				reader := store.readers.Settlement.(*sqliteSettlementReaderV2)
				reader.current, reader.association = inspection, association
			}
			if stage == "settled" {
				if _, err = store.CreateDomainResultFactV2(ctx, domain); err != nil {
					t.Fatal(err)
				}
			}

			callCtx, cancel := context.WithCancel(context.Background())
			point := map[string]string{
				"candidate":     "candidate_after_commit",
				"reservation":   "reservation_after_commit",
				"domain_result": "domain_result_after_commit",
				"settled":       "after_commit",
			}[stage]
			store.fault = func(actual string) error {
				if actual == point {
					cancel()
				}
				return nil
			}
			switch stage {
			case "candidate":
				_, err = store.CreateCandidateFactV2(callCtx, candidate)
			case "reservation":
				_, err = store.CreateReservationFactV2(callCtx, actionRef, appAttempt, testkit.Digest("post-commit-intent-v2"), candidate.SessionID, testkit.Digest("post-commit-subject-v2"), now.Add(time.Second), now.Add(20*time.Second))
			case "domain_result":
				_, err = store.CreateDomainResultFactV2(callCtx, domain)
			case "settled":
				_, err = store.ApplySettlementAndCreateResultV2(callCtx, candidate.ID, objectRefActionV2(domain.ID, domain.Revision, domain.Digest), inspection, domain.Outcome, domain.Disposition, now.Add(7*time.Second))
			}
			if err == nil || !core.HasCategory(err, core.ErrorIndeterminate) {
				t.Fatalf("post-commit SQLite %s error=%v, want Indeterminate", stage, err)
			}
			store.fault = nil
			head, found, inspectErr := readActionHeadQueryV2(ctx, store.store.db, candidate.ID)
			expectedStage := stage
			if stage == "reservation" {
				expectedStage = "reserved"
			}
			if inspectErr != nil || !found || head.Stage != expectedStage {
				t.Fatalf("committed SQLite %s head=%#v found=%v err=%v", stage, head, found, inspectErr)
			}
		})
	}
}

func TestActionHeadReadValidatesCompleteImmutablePredecessorClosureV2(t *testing.T) {
	for _, table := range []string{"tool_action_candidate_v2", "tool_action_reservation_v2", "tool_domain_result_v2", "tool_apply_settlement_v2"} {
		t.Run(table, func(t *testing.T) {
			ctx := context.Background()
			now := testkit.FixedTime
			store, err := OpenActionFactStoreV2(ctx, sqliteActionConfigV2(filepath.Join(t.TempDir(), "head-closure.db"), now), sqliteActionReadersV2())
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			candidate, _, domain, inspection := sqliteActionFixtureV2(t, store, now)
			result, err := store.ApplySettlementAndCreateResultV2(ctx, candidate.ID, objectRefActionV2(domain.ID, domain.Revision, domain.Digest), inspection, domain.Outcome, domain.Disposition, now.Add(7*time.Second))
			if err != nil {
				t.Fatal(err)
			}
			if _, err = store.store.db.Exec(`UPDATE `+table+` SET body_json=? WHERE action_id=?`, []byte(`{}`), candidate.ID); err != nil {
				t.Fatal(err)
			}
			if _, err = store.InspectResultExactV2(ctx, candidate.ID, objectRefActionV2(result.ID, result.Revision, result.Digest)); err == nil || !core.HasCategory(err, core.ErrorConflict) {
				t.Fatalf("corrupt predecessor %s error=%v, want Conflict", table, err)
			}
		})
	}
}

func TestActionFactStoreV2MalformedInspectionParityAndZeroWrite(t *testing.T) {
	now := testkit.FixedTime
	for _, backend := range []string{"in-memory", "sqlite"} {
		t.Run(backend, func(t *testing.T) {
			readers := sqliteActionReadersV2()
			var store toolaction.FactStoreV2
			var durable *ActionFactStoreV2
			var err error
			switch backend {
			case "in-memory":
				store, err = toolaction.NewContextFactStoreV2(toolaction.NewStoreV2(readers))
			case "sqlite":
				durable, err = OpenActionFactStoreV2(context.Background(), sqliteActionConfigV2(filepath.Join(t.TempDir(), "malformed-inspection.db"), now), readers)
				store = durable
				if durable != nil {
					defer durable.Close()
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			candidate, _, domain, inspection := sqliteActionFixtureV2(t, store, now)
			_, association := sqliteInspectionV2(t, domain, testkit.BoundaryFixture(now), now.Add(7*time.Second))
			settlement := readers.Settlement.(*sqliteSettlementReaderV2)
			settlement.current, settlement.association = inspection, association
			malformed := inspection
			malformed.Digest = ""
			if _, err = store.ApplySettlementAndCreateResultV2(
				context.Background(), candidate.ID,
				objectRefActionV2(domain.ID, domain.Revision, domain.Digest),
				malformed, domain.Outcome, domain.Disposition, now.Add(7*time.Second),
			); err == nil || !core.HasCategory(err, core.ErrorInvalidArgument) || !core.HasReason(err, core.ReasonInvalidCanonicalForm) {
				t.Fatalf("malformed inspection error=%v, want InvalidArgument/InvalidCanonicalForm", err)
			}
			if settlement.currentCalls.Load() != 0 || settlement.associationCalls.Load() != 0 {
				t.Fatalf("malformed inspection crossed Runtime readers current=%d association=%d, want 0/0", settlement.currentCalls.Load(), settlement.associationCalls.Load())
			}
			if durable != nil {
				var applyRows, resultRows int
				if err = durable.store.db.QueryRow(`SELECT COUNT(*) FROM tool_apply_settlement_v2 WHERE action_id=?`, candidate.ID).Scan(&applyRows); err != nil {
					t.Fatal(err)
				}
				if err = durable.store.db.QueryRow(`SELECT COUNT(*) FROM tool_result_v2 WHERE action_id=?`, candidate.ID).Scan(&resultRows); err != nil {
					t.Fatal(err)
				}
				if applyRows != 0 || resultRows != 0 {
					t.Fatalf("malformed inspection writes apply/result=%d/%d, want 0/0", applyRows, resultRows)
				}
			}
			if _, err = store.ApplySettlementAndCreateResultV2(
				context.Background(), candidate.ID,
				objectRefActionV2(domain.ID, domain.Revision, domain.Digest),
				inspection, domain.Outcome, domain.Disposition, now.Add(7*time.Second),
			); err != nil {
				t.Fatalf("valid apply after malformed inspection failed: %v", err)
			}
		})
	}
}

func TestActionFactStoreV2InMemorySQLiteCurrentAndSettlementFailureParity(t *testing.T) {
	now := testkit.FixedTime
	type storeCaseV2 struct {
		name    string
		open    func(toolaction.CausalReadersV1) (toolaction.FactStoreV2, func())
		readNow time.Time
	}
	cases := []storeCaseV2{
		{
			name: "in-memory",
			open: func(readers toolaction.CausalReadersV1) (toolaction.FactStoreV2, func()) {
				store, err := toolaction.NewContextFactStoreV2(toolaction.NewStoreV2(readers))
				if err != nil {
					t.Fatal(err)
				}
				return store, func() {}
			},
			readNow: now.Add(7 * time.Second),
		},
		{
			name: "sqlite",
			open: func(readers toolaction.CausalReadersV1) (toolaction.FactStoreV2, func()) {
				store, err := OpenActionFactStoreV2(context.Background(), sqliteActionConfigV2(filepath.Join(t.TempDir(), "parity-failure.db"), now), readers)
				if err != nil {
					t.Fatal(err)
				}
				return store, func() { _ = store.Close() }
			},
			readNow: now.Add(7 * time.Second),
		},
	}
	var expectedExpiry int64
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			readers := sqliteActionReadersV2()
			store, closeStore := test.open(readers)
			defer closeStore()
			candidate, _, domain, inspection := sqliteActionFixtureV2(t, store, now)
			_, association := sqliteInspectionV2(t, domain, testkit.BoundaryFixture(now), now.Add(7*time.Second))
			settlement := readers.Settlement.(*sqliteSettlementReaderV2)
			settlement.mu.Lock()
			settlement.current, settlement.association = inspection, association
			settlement.mu.Unlock()
			projection, err := store.InspectDomainResultCurrentByExactV1(context.Background(), objectRefActionV2(domain.ID, domain.Revision, domain.Digest), test.readNow, toolcontract.MaxDomainResultCurrentTTLV1)
			if err != nil {
				t.Fatal(err)
			}
			if expectedExpiry == 0 {
				expectedExpiry = projection.ExpiresUnixNano
			} else if projection.ExpiresUnixNano != expectedExpiry {
				t.Fatalf("natural expiry=%d want=%d", projection.ExpiresUnixNano, expectedExpiry)
			}
			if _, err = store.InspectDomainResultCurrentByExactV1(context.Background(), objectRefActionV2(domain.ID, domain.Revision, domain.Digest), time.Unix(0, projection.ExpiresUnixNano), toolcontract.MaxDomainResultCurrentTTLV1); err == nil || !core.HasCategory(err, core.ErrorPreconditionFailed) {
				t.Fatalf("caller now==natural expiry error=%v, want PreconditionFailed", err)
			}
			if _, err = store.InspectDomainResultCurrentByExactV1(context.Background(), objectRefActionV2(domain.ID, domain.Revision, domain.Digest), time.Unix(0, domain.CreatedUnixNano-1), toolcontract.MaxDomainResultCurrentTTLV1); err == nil || !core.HasReason(err, core.ReasonClockRegression) {
				t.Fatalf("caller clock rollback error=%v, want ClockRegression", err)
			}
			settlement.mu.Lock()
			settlement.drift = true
			settlement.mu.Unlock()
			if _, err = store.ApplySettlementAndCreateResultV2(context.Background(), candidate.ID, objectRefActionV2(domain.ID, domain.Revision, domain.Digest), inspection, domain.Outcome, domain.Disposition, test.readNow); err == nil || !core.HasCategory(err, core.ErrorConflict) {
				t.Fatalf("Runtime settlement drift error=%v, want Conflict", err)
			}
			if settlement.currentCalls.Load() != 1 || settlement.associationCalls.Load() != 0 {
				t.Fatalf("fresh settlement calls=%d association=%d, want 1/0 on current drift", settlement.currentCalls.Load(), settlement.associationCalls.Load())
			}
		})
	}
}

func TestSQLiteActionReservation64SingleWinnerAndCanceledZeroWrite(t *testing.T) {
	ctx := context.Background()
	now := testkit.FixedTime
	store, err := OpenActionFactStoreV2(ctx, sqliteActionConfigV2(filepath.Join(t.TempDir(), "race.db"), now), sqliteActionReadersV2())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	candidate := testkit.CandidateV2(now)
	if _, err = store.CreateCandidateFactV2(ctx, candidate); err != nil {
		t.Fatal(err)
	}
	actionRef := objectRefActionV2(candidate.ID, candidate.Revision, candidate.Digest)
	var winners atomic.Int32
	var winner toolcontract.ActionReservationFactV2
	var winnerMu sync.Mutex
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			attempt := toolcontract.ApplicationAttemptRefV1{ID: "sqlite-attempt-" + twoDigitSQLiteV2(index), Revision: 1, Digest: testkit.Digest("sqlite-attempt-" + twoDigitSQLiteV2(index))}
			value, reserveErr := store.CreateReservationFactV2(ctx, actionRef, attempt, testkit.Digest("intent"), candidate.SessionID, testkit.Digest("subject"), now.Add(time.Second), now.Add(10*time.Second))
			if reserveErr == nil {
				winners.Add(1)
				winnerMu.Lock()
				winner = value
				winnerMu.Unlock()
			}
		}(i)
	}
	wg.Wait()
	if winners.Load() != 1 {
		t.Fatalf("reservation winners=%d, want 1", winners.Load())
	}
	if _, err = store.InspectReservationExactV2(ctx, candidate.ID, objectRefActionV2(winner.ID, winner.Revision, winner.Digest)); err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	other := testkit.CandidateV2(now)
	other.ID = "action-canceled-v2"
	if _, err = store.CreateCandidateFactV2(canceled, other); err == nil {
		t.Fatal("canceled Candidate mutation succeeded")
	}
	var count int
	if err = store.store.db.QueryRow(`SELECT COUNT(*) FROM tool_action_candidate_v2 WHERE action_id=?`, other.ID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("canceled write count=%d err=%v", count, err)
	}
}

func TestSQLiteActionSchemaDigestDriftFailsReopen(t *testing.T) {
	ctx := context.Background()
	now := testkit.FixedTime
	path := filepath.Join(t.TempDir(), "schema.db")
	store, err := OpenActionFactStoreV2(ctx, sqliteActionConfigV2(path, now), sqliteActionReadersV2())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.store.db.Exec(`UPDATE tool_action_fact_schema_v2 SET digest=? WHERE version=?`, string(testkit.Digest("attacker-schema")), actionFactSchemaVersionV2); err != nil {
		t.Fatal(err)
	}
	_ = store.Close()
	if _, err = OpenActionFactStoreV2(ctx, sqliteActionConfigV2(path, now), sqliteActionReadersV2()); err == nil || !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("schema drift reopen error=%v, want Conflict", err)
	}
}

func TestSQLiteActionApplyResultHeadAtomicFaultsAndLostReply(t *testing.T) {
	for _, point := range []string{"after_apply_insert", "after_result_insert", "after_head_cas", "after_commit"} {
		t.Run(point, func(t *testing.T) {
			ctx := context.Background()
			now := testkit.FixedTime
			store, err := OpenActionFactStoreV2(ctx, sqliteActionConfigV2(filepath.Join(t.TempDir(), "fault.db"), now), sqliteActionReadersV2())
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			candidate, _, domain, inspection := sqliteActionFixtureV2(t, store, now)
			store.fault = func(actual string) error {
				if actual == point {
					return errors.New("injected " + point)
				}
				return nil
			}
			_, err = store.ApplySettlementAndCreateResultV2(ctx, candidate.ID, objectRefActionV2(domain.ID, domain.Revision, domain.Digest), inspection, domain.Outcome, domain.Disposition, now.Add(7*time.Second))
			if err == nil {
				t.Fatal("fault did not fail mutation")
			}
			reader := store.readers.Settlement.(*sqliteSettlementReaderV2)
			expectedReads := int32(2)
			if point == "after_commit" {
				expectedReads = 3
			}
			if reader.currentCalls.Load() != expectedReads || reader.associationCalls.Load() != expectedReads {
				t.Fatalf("Runtime fresh rereads settlement=%d association=%d, want %d", reader.currentCalls.Load(), reader.associationCalls.Load(), expectedReads)
			}
			var applyCount, resultCount int
			var stage string
			if err = store.store.db.QueryRow(`SELECT COUNT(*) FROM tool_apply_settlement_v2 WHERE action_id=?`, candidate.ID).Scan(&applyCount); err != nil {
				t.Fatal(err)
			}
			if err = store.store.db.QueryRow(`SELECT COUNT(*) FROM tool_result_v2 WHERE action_id=?`, candidate.ID).Scan(&resultCount); err != nil {
				t.Fatal(err)
			}
			if err = store.store.db.QueryRow(`SELECT stage FROM tool_action_head_v2 WHERE action_id=?`, candidate.ID).Scan(&stage); err != nil {
				t.Fatal(err)
			}
			if point == "after_commit" {
				if applyCount != 1 || resultCount != 1 || stage != "settled" {
					t.Fatalf("lost reply apply=%d result=%d stage=%s", applyCount, resultCount, stage)
				}
				head, found, readErr := readActionHeadQueryV2(ctx, store.store.db, candidate.ID)
				if readErr != nil || !found || head.Result == nil {
					t.Fatalf("lost reply head=%#v found=%v err=%v", head, found, readErr)
				}
				if _, readErr = store.InspectResultExactV2(ctx, candidate.ID, *head.Result); readErr != nil {
					t.Fatalf("lost reply exact Inspect: %v", readErr)
				}
				return
			}
			if applyCount != 0 || resultCount != 0 || stage != "domain_result" {
				t.Fatalf("point=%s apply=%d result=%d stage=%s", point, applyCount, resultCount, stage)
			}
		})
	}
}

func TestSQLiteActionRuntimeSettlementDriftWritesZeroFacts(t *testing.T) {
	ctx := context.Background()
	now := testkit.FixedTime
	store, err := OpenActionFactStoreV2(ctx, sqliteActionConfigV2(filepath.Join(t.TempDir(), "runtime-drift.db"), now), sqliteActionReadersV2())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	candidate, _, domain, inspection := sqliteActionFixtureV2(t, store, now)
	reader := store.readers.Settlement.(*sqliteSettlementReaderV2)
	reader.mu.Lock()
	reader.drift = true
	reader.mu.Unlock()
	if _, err = store.ApplySettlementAndCreateResultV2(ctx, candidate.ID, objectRefActionV2(domain.ID, domain.Revision, domain.Digest), inspection, domain.Outcome, domain.Disposition, now.Add(7*time.Second)); err == nil {
		t.Fatal("Runtime settlement drift reached Tool Apply")
	}
	var applyCount, resultCount int
	var stage string
	_ = store.store.db.QueryRow(`SELECT COUNT(*) FROM tool_apply_settlement_v2 WHERE action_id=?`, candidate.ID).Scan(&applyCount)
	_ = store.store.db.QueryRow(`SELECT COUNT(*) FROM tool_result_v2 WHERE action_id=?`, candidate.ID).Scan(&resultCount)
	_ = store.store.db.QueryRow(`SELECT stage FROM tool_action_head_v2 WHERE action_id=?`, candidate.ID).Scan(&stage)
	if applyCount != 0 || resultCount != 0 || stage != "domain_result" {
		t.Fatalf("reader drift apply=%d result=%d stage=%s", applyCount, resultCount, stage)
	}
}

func TestSQLiteActionReservationLockWaitCrossingExpiryWritesZero(t *testing.T) {
	ctx := context.Background()
	now := testkit.FixedTime
	clock := newSQLiteMutableClockV2(now.Add(time.Second))
	store, err := OpenActionFactStoreV2(ctx, ConfigV1{
		Path:  filepath.Join(t.TempDir(), "reservation-lock-expiry.db"),
		Owner: core.OwnerRef{Domain: "tool-mcp", ID: "action-facts-v2"},
		Clock: clock.Now, BusyTimeout: 5 * time.Second,
	}, sqliteActionReadersV2())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	candidate := testkit.CandidateV2(now)
	if _, err = store.CreateCandidateFactV2(ctx, candidate); err != nil {
		t.Fatal(err)
	}
	blocker, err := store.store.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	result := make(chan error, 1)
	go func() {
		close(started)
		_, callErr := store.CreateReservationFactV2(
			ctx, objectRefActionV2(candidate.ID, candidate.Revision, candidate.Digest),
			toolcontract.ApplicationAttemptRefV1{ID: "lock-wait-attempt-v2", Revision: 1, Digest: testkit.Digest("lock-wait-attempt-v2")},
			testkit.Digest("lock-wait-intent-v2"), candidate.SessionID, testkit.Digest("lock-wait-subject-v2"),
			now.Add(time.Second), now.Add(5*time.Second),
		)
		result <- callErr
	}()
	<-started
	time.Sleep(50 * time.Millisecond)
	clock.Set(now.Add(5 * time.Second))
	if err = blocker.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err = <-result; err == nil || !core.HasReason(err, core.ReasonCapabilityExpired) {
		t.Fatalf("lock-wait reservation error=%v, want CapabilityExpired", err)
	}
	var rows int
	var stage string
	if err = store.store.db.QueryRow(`SELECT COUNT(*) FROM tool_action_reservation_v2 WHERE action_id=?`, candidate.ID).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if err = store.store.db.QueryRow(`SELECT stage FROM tool_action_head_v2 WHERE action_id=?`, candidate.ID).Scan(&stage); err != nil {
		t.Fatal(err)
	}
	if rows != 0 || stage != "candidate" {
		t.Fatalf("reservation rows=%d stage=%s, want 0/candidate", rows, stage)
	}
}

func TestSQLiteActionReservationUsesAbsoluteOwnerClockAndFreshOwnerCurrentS2(t *testing.T) {
	t.Run("stale caller cannot hide owner expiry", func(t *testing.T) {
		ctx := context.Background()
		now := testkit.FixedTime
		store, err := OpenActionFactStoreV2(ctx, sqliteActionConfigV2(filepath.Join(t.TempDir(), "owner-expired.db"), now), sqliteActionReadersV2())
		if err != nil {
			t.Fatal(err)
		}
		defer store.Close()
		candidate := testkit.CandidateV2(now)
		if _, err = store.CreateCandidateFactV2(ctx, candidate); err != nil {
			t.Fatal(err)
		}
		_, err = store.CreateReservationFactV2(
			ctx, objectRefActionV2(candidate.ID, candidate.Revision, candidate.Digest),
			toolcontract.ApplicationAttemptRefV1{ID: "owner-expired-attempt-v2", Revision: 1, Digest: testkit.Digest("owner-expired-attempt-v2")},
			testkit.Digest("owner-expired-intent-v2"), candidate.SessionID, testkit.Digest("owner-expired-subject-v2"),
			now.Add(time.Second), now.Add(5*time.Second),
		)
		if err == nil || !core.HasReason(err, core.ReasonCapabilityExpired) {
			t.Fatalf("owner-expired reservation error=%v, want CapabilityExpired", err)
		}
		assertSQLiteActionStageAndCountV2(t, store, candidate.ID, "candidate", "tool_action_reservation_v2", 0)
	})

	t.Run("future caller fails closed", func(t *testing.T) {
		ctx := context.Background()
		now := testkit.FixedTime
		clock := newSQLiteMutableClockV2(now.Add(2 * time.Second))
		store, err := OpenActionFactStoreV2(ctx, ConfigV1{Path: filepath.Join(t.TempDir(), "future-caller.db"), Owner: core.OwnerRef{Domain: "tool-mcp", ID: "action-facts-v2"}, Clock: clock.Now}, sqliteActionReadersV2())
		if err != nil {
			t.Fatal(err)
		}
		defer store.Close()
		candidate := testkit.CandidateV2(now)
		if _, err = store.CreateCandidateFactV2(ctx, candidate); err != nil {
			t.Fatal(err)
		}
		_, err = store.CreateReservationFactV2(
			ctx, objectRefActionV2(candidate.ID, candidate.Revision, candidate.Digest),
			toolcontract.ApplicationAttemptRefV1{ID: "future-caller-attempt-v2", Revision: 1, Digest: testkit.Digest("future-caller-attempt-v2")},
			testkit.Digest("future-caller-intent-v2"), candidate.SessionID, testkit.Digest("future-caller-subject-v2"),
			now.Add(3*time.Second), now.Add(10*time.Second),
		)
		if err == nil || !core.HasReason(err, core.ReasonClockRegression) {
			t.Fatalf("future caller reservation error=%v, want ClockRegression", err)
		}
		assertSQLiteActionStageAndCountV2(t, store, candidate.ID, "candidate", "tool_action_reservation_v2", 0)
	})

	t.Run("owner current drift after lock writes zero", func(t *testing.T) {
		ctx := context.Background()
		now := testkit.FixedTime
		readers := sqliteActionReadersV2()
		drifting := &sqliteDriftingOwnerReaderV2{driftAfter: 6}
		readers.OwnerCurrent = drifting
		store, err := OpenActionFactStoreV2(ctx, sqliteActionConfigV2(filepath.Join(t.TempDir(), "owner-current-drift.db"), now), readers)
		if err != nil {
			t.Fatal(err)
		}
		defer store.Close()
		candidate := testkit.CandidateV2(now)
		if _, err = store.CreateCandidateFactV2(ctx, candidate); err != nil {
			t.Fatal(err)
		}
		_, err = store.CreateReservationFactV2(
			ctx, objectRefActionV2(candidate.ID, candidate.Revision, candidate.Digest),
			toolcontract.ApplicationAttemptRefV1{ID: "owner-current-drift-attempt-v2", Revision: 1, Digest: testkit.Digest("owner-current-drift-attempt-v2")},
			testkit.Digest("owner-current-drift-intent-v2"), candidate.SessionID, testkit.Digest("owner-current-drift-subject-v2"),
			now.Add(time.Second), now.Add(20*time.Second),
		)
		if err == nil || !core.HasCategory(err, core.ErrorConflict) {
			t.Fatalf("owner-current S2 drift error=%v, want Conflict", err)
		}
		if drifting.calls.Load() != 7 {
			t.Fatalf("owner current calls=%d, want S1 six refs plus first S2 drift", drifting.calls.Load())
		}
		assertSQLiteActionStageAndCountV2(t, store, candidate.ID, "candidate", "tool_action_reservation_v2", 0)
	})
}

func TestSQLiteActionApplyLockWaitCrossingInspectionExpiryWritesZero(t *testing.T) {
	ctx := context.Background()
	now := testkit.FixedTime
	clock := newSQLiteMutableClockV2(now.Add(time.Second))
	store, err := OpenActionFactStoreV2(ctx, ConfigV1{
		Path:  filepath.Join(t.TempDir(), "apply-lock-expiry.db"),
		Owner: core.OwnerRef{Domain: "tool-mcp", ID: "action-facts-v2"},
		Clock: clock.Now, BusyTimeout: 5 * time.Second,
	}, sqliteActionReadersV2())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	candidate, _, domain, inspection := sqliteActionFixtureV2(t, store, now)
	blocker, err := store.store.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	result := make(chan error, 1)
	go func() {
		close(started)
		_, callErr := store.ApplySettlementAndCreateResultV2(
			ctx, candidate.ID, objectRefActionV2(domain.ID, domain.Revision, domain.Digest),
			inspection, domain.Outcome, domain.Disposition, now.Add(7*time.Second),
		)
		result <- callErr
	}()
	<-started
	time.Sleep(50 * time.Millisecond)
	// The owner clock advances by ten seconds while the IMMEDIATE transaction
	// waits. Applied to the caller's exact now, this lands exactly at expiry.
	clock.Set(now.Add(11 * time.Second))
	if err = blocker.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err = <-result; err == nil {
		t.Fatal("lock-wait Apply unexpectedly committed after Runtime inspection expiry")
	}
	var applyRows, resultRows int
	var stage string
	if err = store.store.db.QueryRow(`SELECT COUNT(*) FROM tool_apply_settlement_v2 WHERE action_id=?`, candidate.ID).Scan(&applyRows); err != nil {
		t.Fatal(err)
	}
	if err = store.store.db.QueryRow(`SELECT COUNT(*) FROM tool_result_v2 WHERE action_id=?`, candidate.ID).Scan(&resultRows); err != nil {
		t.Fatal(err)
	}
	if err = store.store.db.QueryRow(`SELECT stage FROM tool_action_head_v2 WHERE action_id=?`, candidate.ID).Scan(&stage); err != nil {
		t.Fatal(err)
	}
	if applyRows != 0 || resultRows != 0 || stage != "domain_result" {
		t.Fatalf("apply=%d result=%d stage=%s, want 0/0/domain_result", applyRows, resultRows, stage)
	}
}

func TestSQLiteActionApplyUsesAbsoluteOwnerClock(t *testing.T) {
	for _, test := range []struct {
		name       string
		ownerNow   time.Time
		callerNow  time.Time
		wantReason core.ReasonCode
	}{
		{name: "owner crossed inspection ttl", ownerNow: testkit.FixedTime.Add(20 * time.Second), callerNow: testkit.FixedTime.Add(7 * time.Second), wantReason: core.ReasonCapabilityExpired},
		{name: "future caller", ownerNow: testkit.FixedTime.Add(7 * time.Second), callerNow: testkit.FixedTime.Add(8 * time.Second), wantReason: core.ReasonClockRegression},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			now := testkit.FixedTime
			clock := newSQLiteMutableClockV2(now.Add(7 * time.Second))
			store, err := OpenActionFactStoreV2(ctx, ConfigV1{Path: filepath.Join(t.TempDir(), "apply-owner-clock.db"), Owner: core.OwnerRef{Domain: "tool-mcp", ID: "action-facts-v2"}, Clock: clock.Now}, sqliteActionReadersV2())
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			candidate, _, domain, inspection := sqliteActionFixtureV2(t, store, now)
			clock.Set(test.ownerNow)
			_, err = store.ApplySettlementAndCreateResultV2(ctx, candidate.ID, objectRefActionV2(domain.ID, domain.Revision, domain.Digest), inspection, domain.Outcome, domain.Disposition, test.callerNow)
			if err == nil || !core.HasReason(err, test.wantReason) {
				t.Fatalf("Apply owner clock error=%v, want reason %s", err, test.wantReason)
			}
			assertSQLiteActionStageAndCountV2(t, store, candidate.ID, "domain_result", "tool_apply_settlement_v2", 0)
			var resultRows int
			if err = store.store.db.QueryRow(`SELECT COUNT(*) FROM tool_result_v2 WHERE action_id=?`, candidate.ID).Scan(&resultRows); err != nil || resultRows != 0 {
				t.Fatalf("result rows=%d err=%v, want zero", resultRows, err)
			}
		})
	}
}

func assertSQLiteActionStageAndCountV2(t *testing.T, store *ActionFactStoreV2, actionID, wantStage, table string, wantCount int) {
	t.Helper()
	var stage string
	var count int
	if err := store.store.db.QueryRow(`SELECT stage FROM tool_action_head_v2 WHERE action_id=?`, actionID).Scan(&stage); err != nil {
		t.Fatal(err)
	}
	if err := store.store.db.QueryRow(`SELECT COUNT(*) FROM `+table+` WHERE action_id=?`, actionID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if stage != wantStage || count != wantCount {
		t.Fatalf("stage=%s rows=%d, want %s/%d", stage, count, wantStage, wantCount)
	}
}

func TestSQLiteDomainResultCurrentUsesCallerNowAndNaturalBounds(t *testing.T) {
	ctx := context.Background()
	base := testkit.FixedTime
	clockNow := base.Add(17 * time.Second)
	callerNow := base.Add(7 * time.Second)
	config := ConfigV1{Path: filepath.Join(t.TempDir(), "domain-current.db"), Owner: core.OwnerRef{Domain: "tool-mcp", ID: "action-facts-v2"}, Clock: func() time.Time { return clockNow }}
	store, err := OpenActionFactStoreV2(ctx, config, sqliteActionReadersV2())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	_, _, domain, _ := sqliteActionFixtureV2(t, store, base)
	exact := objectRefActionV2(domain.ID, domain.Revision, domain.Digest)
	projection, err := store.InspectDomainResultCurrentByExactV1(ctx, exact, callerNow, toolcontract.MaxDomainResultCurrentTTLV1)
	if err != nil {
		t.Fatal(err)
	}
	expectedExpiry := callerNow.Add(toolcontract.MaxDomainResultCurrentTTLV1).UnixNano()
	for _, bound := range []int64{domain.PreparedAttempt.ExpiresUnixNano, domain.PrepareEnforcement.ExpiresUnixNano, domain.ExecuteEnforcement.ExpiresUnixNano} {
		if bound < expectedExpiry {
			expectedExpiry = bound
		}
	}
	if projection.CheckedUnixNano != callerNow.UnixNano() || projection.ExpiresUnixNano != expectedExpiry {
		t.Fatalf("projection checked=%d expires=%d want %d/%d", projection.CheckedUnixNano, projection.ExpiresUnixNano, callerNow.UnixNano(), expectedExpiry)
	}
	clockNow = base.Add(18 * time.Second)
	if _, err = store.InspectDomainResultCurrentByExactV1(ctx, exact, time.Unix(0, expectedExpiry), time.Second); err == nil || !core.HasCategory(err, core.ErrorPreconditionFailed) {
		t.Fatalf("now==expiry error=%v, want PreconditionFailed", err)
	}
	clockNow = base.Add(17 * time.Second)
	if _, err = store.InspectDomainResultCurrentByExactV1(ctx, exact, callerNow, time.Second); err == nil || !core.HasReason(err, core.ReasonClockRegression) {
		t.Fatalf("clock rollback error=%v, want ClockRegression", err)
	}
}

func TestSQLiteActionCandidateReservationDomainLostRepliesAndStageRestarts(t *testing.T) {
	ctx := context.Background()
	now := testkit.FixedTime
	path := filepath.Join(t.TempDir(), "stage-restarts.db")
	open := func() *ActionFactStoreV2 {
		store, err := OpenActionFactStoreV2(ctx, sqliteActionConfigV2(path, now), sqliteActionReadersV2())
		if err != nil {
			t.Fatal(err)
		}
		return store
	}
	store := open()
	candidate := testkit.CandidateV2(now)
	store.fault = func(point string) error {
		if point == "candidate_after_commit" {
			return errors.New("lost candidate reply")
		}
		return nil
	}
	if _, err := store.CreateCandidateFactV2(ctx, candidate); err == nil || !core.HasCategory(err, core.ErrorIndeterminate) {
		t.Fatalf("candidate lost reply error=%v", err)
	}
	_ = store.Close()
	store = open()
	actionRef := objectRefActionV2(candidate.ID, candidate.Revision, candidate.Digest)
	if _, err := store.InspectCandidateCurrentV2(ctx, actionRef, now); err != nil {
		t.Fatal(err)
	}
	appAttempt := toolcontract.ApplicationAttemptRefV1{ID: "sqlite-app-attempt-v2", Revision: 1, Digest: testkit.Digest("sqlite-app-attempt-v2")}
	store.fault = func(point string) error {
		if point == "reservation_after_commit" {
			return errors.New("lost reservation reply")
		}
		return nil
	}
	reservation, err := store.CreateReservationFactV2(ctx, actionRef, appAttempt, testkit.Digest("intent-v2"), candidate.SessionID, testkit.Digest("subject-v2"), now.Add(time.Second), now.Add(20*time.Second))
	if err == nil || !core.HasCategory(err, core.ErrorIndeterminate) {
		t.Fatalf("reservation lost reply error=%v", err)
	}
	_ = store.Close()
	store = open()
	head, found, err := readActionHeadQueryV2(ctx, store.store.db, candidate.ID)
	if err != nil || !found || head.Reservation == nil {
		t.Fatalf("reservation restart head=%#v found=%v err=%v", head, found, err)
	}
	reservation, err = store.InspectReservationExactV2(ctx, candidate.ID, *head.Reservation)
	if err != nil {
		t.Fatal(err)
	}
	domain, _, _ := sqliteDomainFixtureV2(t, candidate, reservation, appAttempt, now)
	store.fault = func(point string) error {
		if point == "domain_result_after_commit" {
			return errors.New("lost domain reply")
		}
		return nil
	}
	if _, err = store.CreateDomainResultFactV2(ctx, domain); err == nil || !core.HasCategory(err, core.ErrorIndeterminate) {
		t.Fatalf("domain lost reply error=%v", err)
	}
	_ = store.Close()
	store = open()
	defer store.Close()
	if _, err = store.InspectDomainResultExactV2(ctx, candidate.ID, objectRefActionV2(domain.ID, domain.Revision, domain.Digest)); err != nil {
		t.Fatal(err)
	}
}

func sqliteActionFixtureV2(t *testing.T, store toolaction.FactStoreV2, now time.Time) (toolcontract.ActionCandidateV2, toolcontract.ActionReservationFactV2, toolcontract.ToolDomainResultFactV2, runtimeports.OperationInspectionSettlementRefV4) {
	t.Helper()
	ctx := context.Background()
	candidate := testkit.CandidateV2(now)
	if _, err := store.CreateCandidateFactV2(ctx, candidate); err != nil {
		t.Fatal(err)
	}
	actionRef := objectRefActionV2(candidate.ID, candidate.Revision, candidate.Digest)
	appAttempt := toolcontract.ApplicationAttemptRefV1{ID: "sqlite-app-attempt-v2", Revision: 1, Digest: testkit.Digest("sqlite-app-attempt-v2")}
	reservation, err := store.CreateReservationFactV2(ctx, actionRef, appAttempt, testkit.Digest("intent-v2"), candidate.SessionID, testkit.Digest("subject-v2"), now.Add(time.Second), now.Add(20*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	domain, inspection, association := sqliteDomainFixtureV2(t, candidate, reservation, appAttempt, now)
	if _, err = store.CreateDomainResultFactV2(ctx, domain); err != nil {
		t.Fatal(err)
	}
	if durable, ok := store.(*ActionFactStoreV2); ok {
		reader, ok := durable.readers.Settlement.(*sqliteSettlementReaderV2)
		if !ok {
			t.Fatal("SQLite Action Fact fixture lacks Runtime settlement reader")
		}
		reader.mu.Lock()
		reader.current, reader.association = inspection, association
		reader.mu.Unlock()
	}
	return candidate, reservation, domain, inspection
}

func sqliteDomainFixtureV2(t *testing.T, candidate toolcontract.ActionCandidateV2, reservation toolcontract.ActionReservationFactV2, appAttempt toolcontract.ApplicationAttemptRefV1, now time.Time) (toolcontract.ToolDomainResultFactV2, runtimeports.OperationInspectionSettlementRefV4, runtimeports.OperationSettlementEvidenceAssociationV4) {
	t.Helper()
	actionRef := objectRefActionV2(candidate.ID, candidate.Revision, candidate.Digest)
	fixture := testkit.BoundaryFixture(now)
	prepared := testkit.PreparedAttemptFor(now, fixture, testkit.ProviderBinding(), candidate.InputSchema, candidate.Payload.ContentDigest, candidate.PayloadRevision)
	fixture.Enforcement.PreparedAttemptDigest = prepared.Digest
	prepare := fixture.Enforcement
	prepare.Phase, prepare.JournalRevision, prepare.ReceiptDigest, prepare.PrepareReceiptDigest, prepare.PreparedAttemptDigest = runtimeports.OperationDispatchEnforcementPrepareV4, 1, testkit.Digest("prepare-receipt-v2"), "", ""
	reservationRef := objectRefActionV2(reservation.ID, reservation.Revision, reservation.Digest)
	causality, err := toolcontract.SealRuntimeAttemptCausalityV1(toolcontract.RuntimeAttemptCausalityV1{Reservation: reservationRef, ApplicationAttempt: appAttempt, Operation: fixture.Operation, OperationDigest: fixture.Attempt.OperationDigest, Attempt: fixture.Attempt, EffectID: fixture.Attempt.EffectID, EffectRevision: fixture.Attempt.IntentRevision, IntentDigest: fixture.Attempt.IntentDigest})
	if err != nil {
		t.Fatal(err)
	}
	observation := testkit.ProviderObservation(now.Add(6 * time.Second))
	observation.Delegation, observation.PreparedAttemptID = *fixture.Attempt.Delegation, prepared.ID
	domain, err := toolcontract.SealToolDomainResultFactV2(toolcontract.ToolDomainResultFactV2{ID: "sqlite-domain-result-v2", TenantID: candidate.TenantID, OperationScopeDigest: candidate.OperationScopeDigest, Action: actionRef, Reservation: reservationRef, ApplicationAttempt: appAttempt, Causality: causality, PreparedAttempt: prepared, Observation: observation, PrepareEnforcement: prepare, ExecuteEnforcement: fixture.Enforcement, PrepareConsumption: sqliteConsumptionV2("prepare", 1), ExecuteConsumption: sqliteConsumptionV2("execute", 2), Schema: testkit.Schema("result"), PayloadDigest: testkit.Digest("domain-payload"), PayloadRevision: 1, Owner: testkit.SettlementOwner(), Outcome: toolcontract.ToolOutcomeSucceededV2, Disposition: toolcontract.ToolDispositionConfirmedAppliedV2, CreatedUnixNano: now.Add(6 * time.Second).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	inspection, association := sqliteInspectionV2(t, domain, fixture, now.Add(7*time.Second))
	return domain, inspection, association
}

func sqliteConsumptionV2(label string, sequence uint64) runtimeports.OperationScopeEvidenceConsumptionRefV3 {
	return runtimeports.OperationScopeEvidenceConsumptionRefV3{ID: "sqlite-consumption-" + label, Revision: 1, Digest: testkit.Digest("sqlite-consumption-" + label), Record: runtimeports.OperationScopeEvidenceRecordRefV3{LedgerScopeDigest: testkit.Digest("sqlite-ledger-" + label), Sequence: sequence, RecordDigest: testkit.Digest("sqlite-record-" + label)}}
}

func sqliteInspectionV2(t *testing.T, domain toolcontract.ToolDomainResultFactV2, fixture testkit.BoundaryFixtureV1, now time.Time) (runtimeports.OperationInspectionSettlementRefV4, runtimeports.OperationSettlementEvidenceAssociationV4) {
	t.Helper()
	provider := runtimeports.ProviderBindingRefV2{BindingSetID: "tool-binding-v2", BindingSetRevision: 1, ComponentID: domain.Owner.ComponentID, ManifestDigest: domain.Owner.ManifestDigest, ArtifactDigest: testkit.Digest("tool-artifact-v2"), Capability: "praxis.tool/execute"}
	runtimeDomain := runtimeports.OperationSettlementDomainResultFactRefV4{Owner: provider, Kind: "praxis.tool/domain-result", ID: domain.ID, Revision: domain.Revision, Digest: domain.Digest, TenantID: domain.TenantID, EffectID: fixture.Attempt.EffectID, EffectRevision: fixture.Attempt.IntentRevision, Operation: fixture.Operation, OperationDigest: fixture.Attempt.OperationDigest, Attempt: fixture.Attempt, Schema: domain.Schema, PayloadDigest: domain.PayloadDigest, PayloadRevision: domain.PayloadRevision, AuthoritativeTime: domain.CreatedUnixNano}
	settlement := runtimeports.OperationSettlementRefV4{ID: "sqlite-runtime-settlement-v4", Revision: 1, Digest: testkit.Digest("sqlite-runtime-settlement-v4"), OperationDigest: fixture.Attempt.OperationDigest, EffectID: fixture.Attempt.EffectID, DomainResult: runtimeDomain}
	makeBinding := func(label string, phase runtimeports.OperationDispatchEnforcementPhaseV4, consumption runtimeports.OperationScopeEvidenceConsumptionRefV3, enforcement runtimeports.OperationDispatchEnforcementPhaseRefV4) runtimeports.OperationSettlementEvidenceBindingV4 {
		issued := runtimeports.OperationScopeEvidenceQualificationRefV3{ID: "sqlite-qualification-" + label, Revision: 1, Digest: testkit.Digest("sqlite-issued-" + label), ExpiresUnixNano: now.Add(10 * time.Second).UnixNano()}
		final := issued
		final.Revision, final.Digest = 2, testkit.Digest("sqlite-final-"+label)
		return runtimeports.OperationSettlementEvidenceBindingV4{Phase: phase, Consumption: consumption, IssuedQualification: issued, FinalQualification: final, Record: consumption.Record, CandidateDigest: testkit.Digest("sqlite-candidate-" + label), Handoff: runtimeports.OperationScopeEvidenceProviderHandoffRefV3{ID: "sqlite-handoff-" + label, Revision: 1, Digest: testkit.Digest("sqlite-handoff-" + label), ExpiresUnixNano: now.Add(10 * time.Second).UnixNano()}, Attempt: domain.Causality.Attempt, EnforcementPhase: enforcement, OperationScopeDigest: testkit.Digest("sqlite-scope-" + label)}
	}
	associationFact, err := runtimeports.SealOperationSettlementEvidenceAssociationV4(runtimeports.OperationSettlementEvidenceAssociationV4{ID: "sqlite-runtime-association-v4", Settlement: settlement, Prepare: makeBinding("prepare", runtimeports.OperationDispatchEnforcementPrepareV4, domain.PrepareConsumption, domain.PrepareEnforcement), Execute: makeBinding("execute", runtimeports.OperationDispatchEnforcementExecuteV4, domain.ExecuteConsumption, domain.ExecuteEnforcement)})
	if err != nil {
		t.Fatal(err)
	}
	association := associationFact.RefV4()
	guard := runtimeports.OperationSettlementTerminalGuardRefV4{ID: "sqlite-runtime-guard-v4", TenantID: domain.TenantID, EffectID: settlement.EffectID, OperationDigest: settlement.OperationDigest, Revision: 1, Digest: testkit.Digest("sqlite-runtime-guard-v4"), Settlement: settlement}
	projection := runtimeports.OperationSettlementTerminalProjectionRefV4{ID: "sqlite-runtime-projection-v4", Revision: 1, Digest: testkit.Digest("sqlite-runtime-projection-v4"), TenantID: domain.TenantID, OperationDigest: settlement.OperationDigest, EffectID: settlement.EffectID, Settlement: settlement, Association: association, Guard: guard}
	inspection, err := runtimeports.SealOperationInspectionSettlementRefV4(runtimeports.OperationInspectionSettlementRefV4{Settlement: settlement, Association: association, Guard: guard, Projection: projection, DomainResult: runtimeDomain, EffectFactRevision: 4, Owner: domain.Owner, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(10 * time.Second).UnixNano()}, now)
	if err != nil {
		t.Fatal(err)
	}
	return inspection, associationFact
}

func sqliteExpectedToolResultV2(t *testing.T, domain toolcontract.ToolDomainResultFactV2, inspection runtimeports.OperationInspectionSettlementRefV4, now time.Time) toolcontract.ToolResultV2 {
	t.Helper()
	domainRef := objectRefActionV2(domain.ID, domain.Revision, domain.Digest)
	applyID, err := toolcontract.StableID("tool-apply-v2", domain.Action.ID, domain.ID, string(inspection.Digest))
	if err != nil {
		t.Fatal(err)
	}
	apply, err := toolcontract.SealToolApplySettlementFactV2(toolcontract.ToolApplySettlementFactV2{
		ID: applyID, TenantID: domain.TenantID, OperationScopeDigest: domain.OperationScopeDigest,
		Action: domain.Action, Reservation: domain.Reservation, DomainResult: domainRef,
		Inspection: inspection, Outcome: domain.Outcome, Disposition: domain.Disposition,
		Owner: domain.Owner, AppliedUnixNano: now.UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	resultID, err := toolcontract.StableID("tool-result-v2", domain.Action.ID, domain.ID, apply.ID, string(apply.Digest))
	if err != nil {
		t.Fatal(err)
	}
	result, err := toolcontract.SealToolResultV2(toolcontract.ToolResultV2{
		ID: resultID, Action: domain.Action, Reservation: domain.Reservation, DomainResult: domainRef,
		Apply: objectRefActionV2(apply.ID, apply.Revision, apply.Digest), Inspection: inspection,
		Outcome: domain.Outcome, Disposition: domain.Disposition, Schema: domain.Schema,
		PayloadDigest: domain.PayloadDigest, PayloadRevision: domain.PayloadRevision,
		Residuals: append([]toolcontract.Residual(nil), domain.Residuals...), FinalizedUnixNano: now.UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func twoDigitSQLiteV2(value int) string {
	return string([]byte{'0' + byte(value/10), '0' + byte(value%10)})
}
