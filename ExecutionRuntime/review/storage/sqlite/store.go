// Package sqlite provides the single-node durable Review StoreV1 backend.
// SQLite WAL is an implementation detail; no database handle or row shape is
// exposed through Review ports.
package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/memory"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	_ "modernc.org/sqlite"
)

const schemaVersionV1 = 1

const schemaV1 = `
CREATE TABLE IF NOT EXISTS review_schema (
  version INTEGER PRIMARY KEY,
  digest TEXT NOT NULL,
  applied_unix_nano INTEGER NOT NULL CHECK(applied_unix_nano > 0)
);
CREATE TABLE IF NOT EXISTS review_owner_state (
  tenant_id TEXT PRIMARY KEY,
  generation INTEGER NOT NULL CHECK(generation > 0),
  canonical_json BLOB NOT NULL,
  digest TEXT NOT NULL,
  updated_unix_nano INTEGER NOT NULL CHECK(updated_unix_nano > 0)
);`

type Config struct {
	Path         string
	BusyTimeout  time.Duration
	MaxOpenConns int
	Clock        func() time.Time
}

type Store struct {
	db    *sql.DB
	clock func() time.Time
}

var _ reviewport.StoreV1 = (*Store)(nil)
var _ reviewport.TraceEventStoreV2 = (*Store)(nil)

func Open(ctx context.Context, config Config) (*Store, error) {
	if err := ctx.Err(); err != nil {
		return nil, indeterminate("sqlite open context ended")
	}
	if strings.TrimSpace(config.Path) == "" {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "review sqlite path is required")
	}
	if config.BusyTimeout <= 0 {
		config.BusyTimeout = 5 * time.Second
	}
	if config.BusyTimeout > time.Minute {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "review sqlite busy timeout exceeds its bound")
	}
	if config.MaxOpenConns <= 0 {
		config.MaxOpenConns = 4
	}
	if config.MaxOpenConns > 32 {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "review sqlite connection count exceeds its bound")
	}
	if config.Clock == nil {
		config.Clock = time.Now
	}
	abs, err := filepath.Abs(config.Path)
	if err != nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "review sqlite path is invalid")
	}
	dsn := (&url.URL{Scheme: "file", Path: abs}).String()
	dsn += fmt.Sprintf("?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(%d)&_txlock=immediate", config.BusyTimeout.Milliseconds())
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, mapDBError(ctx, err, false)
	}
	db.SetMaxOpenConns(config.MaxOpenConns)
	db.SetMaxIdleConns(config.MaxOpenConns)
	store := &Store{db: db, clock: config.Clock}
	if err := store.migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := store.verifyPragmas(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) migrate(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return mapDBError(ctx, err, true)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, schemaV1); err != nil {
		return mapDBError(ctx, err, true)
	}
	digest := core.DigestBytes([]byte(schemaV1))
	now := s.clock()
	if now.IsZero() || now.UnixNano() <= 0 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "review sqlite migration clock is invalid")
	}
	result, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO review_schema(version,digest,applied_unix_nano) VALUES(?,?,?)`, schemaVersionV1, string(digest), now.UnixNano())
	if err != nil {
		return mapDBError(ctx, err, true)
	}
	if affected, err := result.RowsAffected(); err != nil {
		return mapDBError(ctx, err, true)
	} else if affected == 0 {
		var stored string
		if err := tx.QueryRowContext(ctx, `SELECT digest FROM review_schema WHERE version=?`, schemaVersionV1).Scan(&stored); err != nil {
			return mapDBError(ctx, err, true)
		}
		if stored != string(digest) {
			return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "review sqlite schema digest drifted")
		}
	}
	if err := tx.Commit(); err != nil {
		return indeterminate("review sqlite migration commit outcome is unknown")
	}
	return nil
}

func (s *Store) verifyPragmas(ctx context.Context) error {
	var journal string
	if err := s.db.QueryRowContext(ctx, `PRAGMA journal_mode`).Scan(&journal); err != nil {
		return mapDBError(ctx, err, false)
	}
	if !strings.EqualFold(journal, "wal") {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "review sqlite WAL mode is not active")
	}
	var foreignKeys int
	if err := s.db.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&foreignKeys); err != nil {
		return mapDBError(ctx, err, false)
	}
	if foreignKeys != 1 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "review sqlite foreign keys are not active")
	}
	return nil
}

func (s *Store) IntegrityCheckV1(ctx context.Context) error {
	if s == nil || s.db == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "review sqlite Store is unavailable")
	}
	if ctx == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "review sqlite integrity context is required")
	}
	var result string
	if err := s.db.QueryRowContext(ctx, `PRAGMA integrity_check`).Scan(&result); err != nil {
		return mapDBError(ctx, err, false)
	}
	if result != "ok" {
		return core.NewError(core.ErrorInternal, core.ReasonInvalidDigest, "review sqlite integrity check failed")
	}
	return nil
}

type queryRower interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func loadState(ctx context.Context, source queryRower, tenant core.TenantID, clock func() time.Time) (*memory.Store, uint64, core.Digest, bool, error) {
	if strings.TrimSpace(string(tenant)) == "" {
		return nil, 0, "", false, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "review sqlite tenant is required")
	}
	var generation uint64
	var payload []byte
	var storedDigest string
	err := source.QueryRowContext(ctx, `SELECT generation,canonical_json,digest FROM review_owner_state WHERE tenant_id=?`, string(tenant)).Scan(&generation, &payload, &storedDigest)
	if errors.Is(err, sql.ErrNoRows) {
		snapshot, sealErr := memory.SealSnapshotV1(memorySnapshotEmpty(tenant))
		if sealErr != nil {
			return nil, 0, "", false, sealErr
		}
		state, restoreErr := memory.NewStoreFromSnapshotWithClockV1(snapshot, clock)
		return state, 0, snapshot.Digest, false, restoreErr
	}
	if err != nil {
		return nil, 0, "", false, mapDBError(ctx, err, false)
	}
	if len(payload) == 0 || len(payload) > memory.MaxSnapshotBytesV1 {
		return nil, 0, "", false, core.NewError(core.ErrorInternal, core.ReasonCanonicalLimitExceeded, "review sqlite snapshot exceeds its bounded size")
	}
	var snapshot memory.SnapshotV1
	if err := decodeSnapshotStrictV1(payload, &snapshot); err != nil {
		return nil, 0, "", false, err
	}
	if string(snapshot.Digest) != storedDigest {
		return nil, 0, "", false, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "review sqlite row digest drifted")
	}
	state, err := memory.NewStoreFromSnapshotWithClockV1(snapshot, clock)
	if err != nil {
		return nil, 0, "", false, err
	}
	return state, generation, snapshot.Digest, true, nil
}

// memorySnapshotEmpty uses the package's exported seal/restore path without
// exposing a second state constructor to production callers.
func memorySnapshotEmpty(tenant core.TenantID) memory.SnapshotV1 {
	return memory.SnapshotV1{
		ContractVersion: memory.SnapshotContractVersionV1, TenantID: tenant,
		Requests: map[string]contract.ReviewRequestV1{}, RequestHistory: map[string]map[core.Revision]contract.ReviewRequestV1{}, RequestByIdempotency: map[string]string{}, RequestByCase: map[string]string{}, ResultBundles: map[string]contract.ReviewResultBundleV1{}, ResultBundlesV2: map[string]contract.ReviewResultBundleV2{},
		Targets: map[string]contract.TargetSnapshotV1{}, TargetHistory: map[string]map[core.Revision]contract.TargetSnapshotV1{},
		Cases: map[string]contract.ReviewCaseV1{}, CaseHistory: map[string]map[core.Revision]contract.ReviewCaseV1{}, CurrentCaseByTarget: map[string]string{},
		Rounds: map[string]contract.ReviewRoundV1{}, Assignments: map[string]contract.ReviewerAssignmentV1{}, AssignmentHistory: map[string]map[core.Revision]contract.ReviewerAssignmentV1{},
		Findings: map[string]contract.FindingV1{}, Attestations: map[string]contract.AttestationV1{}, Verdicts: map[string]contract.VerdictV1{}, VerdictHistory: map[string]map[core.Revision]contract.VerdictV1{},
		Traces: map[string]contract.TraceFactV1{}, TraceByCase: map[string][]string{}, DomainResults: map[string]contract.ReviewerInvocationResultFactV1{}, ApplySettlements: map[string]contract.DomainApplySettlementFactV1{}, BehaviorFeedback: map[string]contract.BehaviorFeedbackCandidateV1{}, EvidenceAttachments: map[string]contract.EvidenceAttachmentV1{}, EvidenceAttachmentByIdempotency: map[string]string{},
	}
}

func (s *Store) read(ctx context.Context, tenant core.TenantID, inspect func(*memory.Store) error) error {
	if s == nil || s.db == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "review sqlite Store is unavailable")
	}
	if ctx == nil || inspect == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "review sqlite read dependencies are incomplete")
	}
	if err := ctx.Err(); err != nil {
		return indeterminate("review sqlite read context ended")
	}
	state, _, _, _, err := loadState(ctx, s.db, tenant, s.clock)
	if err != nil {
		return err
	}
	return inspect(state)
}

func (s *Store) mutate(ctx context.Context, tenant core.TenantID, apply func(*memory.Store) error) error {
	if s == nil || s.db == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "review sqlite Store is unavailable")
	}
	if ctx == nil || apply == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "review sqlite mutation dependencies are incomplete")
	}
	if err := ctx.Err(); err != nil {
		return indeterminate("review sqlite mutation context ended")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return mapDBError(ctx, err, true)
	}
	defer func() { _ = tx.Rollback() }()
	state, generation, previousDigest, exists, err := loadState(ctx, tx, tenant, s.clock)
	if err != nil {
		return err
	}
	if err := apply(state); err != nil {
		return err
	}
	snapshot, err := state.ExportSnapshotV1(tenant)
	if err != nil {
		return err
	}
	if exists && snapshot.Digest == previousDigest {
		if err := tx.Commit(); err != nil {
			return indeterminate("review sqlite replay commit outcome is unknown")
		}
		return nil
	}
	payload, err := json.Marshal(snapshot)
	if err != nil || len(payload) == 0 || len(payload) > memory.MaxSnapshotBytesV1 {
		return core.NewError(core.ErrorInternal, core.ReasonCanonicalLimitExceeded, "review sqlite snapshot serialization failed or exceeded its bound")
	}
	now := s.clock()
	if now.IsZero() || now.UnixNano() <= 0 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "review sqlite mutation clock is invalid")
	}
	if !exists {
		result, execErr := tx.ExecContext(ctx, `INSERT OR IGNORE INTO review_owner_state(tenant_id,generation,canonical_json,digest,updated_unix_nano) VALUES(?,?,?,?,?)`, string(tenant), 1, payload, string(snapshot.Digest), now.UnixNano())
		if execErr != nil {
			return mapDBError(ctx, execErr, true)
		}
		affected, rowsErr := result.RowsAffected()
		if rowsErr != nil {
			return mapDBError(ctx, rowsErr, true)
		}
		if affected != 1 {
			return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "review sqlite tenant state was concurrently created")
		}
	} else {
		if generation == ^uint64(0) {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonCanonicalLimitExceeded, "review sqlite generation is exhausted")
		}
		result, execErr := tx.ExecContext(ctx, `UPDATE review_owner_state SET generation=?,canonical_json=?,digest=?,updated_unix_nano=? WHERE tenant_id=? AND generation=? AND digest=?`, generation+1, payload, string(snapshot.Digest), now.UnixNano(), string(tenant), generation, string(previousDigest))
		if execErr != nil {
			return mapDBError(ctx, execErr, true)
		}
		affected, rowsErr := result.RowsAffected()
		if rowsErr != nil {
			return mapDBError(ctx, rowsErr, true)
		}
		if affected != 1 {
			return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "review sqlite tenant generation CAS lost")
		}
	}
	if err := tx.Commit(); err != nil {
		return indeterminate("review sqlite mutation commit outcome is unknown")
	}
	return nil
}

func mapDBError(ctx context.Context, err error, mutation bool) error {
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return indeterminate("review sqlite request outcome is unknown after context end")
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "locked") || strings.Contains(message, "busy") {
		return core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "review sqlite is busy")
	}
	if strings.Contains(message, "constraint") || strings.Contains(message, "unique") {
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "review sqlite constraint conflict")
	}
	if mutation {
		return indeterminate("review sqlite mutation outcome is unknown")
	}
	return core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "review sqlite read is unavailable")
}

func indeterminate(message string) error {
	return core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, message)
}

func (s *Store) InspectDecisionOwnerInputsV1(ctx context.Context, request reviewport.DecisionCurrentRequestV1) (result reviewport.DecisionOwnerInputsV1, err error) {
	err = s.read(ctx, request.TenantID, func(state *memory.Store) error {
		result, err = state.InspectDecisionOwnerInputsV1(ctx, request)
		return err
	})
	return
}

func (s *Store) ResolveDecisionCurrentRequestV1(ctx context.Context, request reviewport.DecisionCurrentResolveRequestV1) (result reviewport.DecisionCurrentRequestV1, err error) {
	err = s.read(ctx, request.TenantID, func(state *memory.Store) error {
		result, err = state.ResolveDecisionCurrentRequestV1(ctx, request)
		return err
	})
	return
}

func (s *Store) CreateTargetCaseV1(ctx context.Context, m reviewport.CreateTargetCaseMutationV1) (result contract.ReviewCaseV1, err error) {
	err = s.mutate(ctx, m.Target.TenantID, func(state *memory.Store) error { result, err = state.CreateTargetCaseV1(ctx, m); return err })
	return
}
func (s *Store) InspectRequestExactV1(ctx context.Context, tenant core.TenantID, ref reviewport.ExactFactRefV1) (result contract.ReviewRequestV1, err error) {
	err = s.read(ctx, tenant, func(state *memory.Store) error {
		result, err = state.InspectRequestExactV1(ctx, tenant, ref)
		return err
	})
	return
}
func (s *Store) InspectRequestByIdempotencyV1(ctx context.Context, tenant core.TenantID, idempotency string) (result contract.ReviewRequestV1, err error) {
	err = s.read(ctx, tenant, func(state *memory.Store) error {
		result, err = state.InspectRequestByIdempotencyV1(ctx, tenant, idempotency)
		return err
	})
	return
}
func (s *Store) InspectRequestByCaseV1(ctx context.Context, tenant core.TenantID, caseID string) (result contract.ReviewRequestV1, err error) {
	err = s.read(ctx, tenant, func(state *memory.Store) error {
		result, err = state.InspectRequestByCaseV1(ctx, tenant, caseID)
		return err
	})
	return
}
func (s *Store) InspectResultBundleExactV1(ctx context.Context, tenant core.TenantID, ref reviewport.ExactFactRefV1) (result contract.ReviewResultBundleV1, err error) {
	err = s.read(ctx, tenant, func(state *memory.Store) error {
		result, err = state.InspectResultBundleExactV1(ctx, tenant, ref)
		return err
	})
	return
}
func (s *Store) InspectResultBundleExactV2(ctx context.Context, tenant core.TenantID, ref reviewport.ExactFactRefV1) (result contract.ReviewResultBundleV2, err error) {
	err = s.read(ctx, tenant, func(state *memory.Store) error {
		result, err = state.InspectResultBundleExactV2(ctx, tenant, ref)
		return err
	})
	return
}
func (s *Store) InspectTargetV1(ctx context.Context, tenant core.TenantID, id string) (result contract.TargetSnapshotV1, err error) {
	err = s.read(ctx, tenant, func(state *memory.Store) error { result, err = state.InspectTargetV1(ctx, tenant, id); return err })
	return
}
func (s *Store) InspectTargetExactV1(ctx context.Context, tenant core.TenantID, ref reviewport.ExactFactRefV1) (result contract.TargetSnapshotV1, err error) {
	err = s.read(ctx, tenant, func(state *memory.Store) error {
		result, err = state.InspectTargetExactV1(ctx, tenant, ref)
		return err
	})
	return
}
func (s *Store) InspectCaseByTargetV1(ctx context.Context, tenant core.TenantID, id string, revision core.Revision, digest core.Digest) (result contract.ReviewCaseV1, err error) {
	err = s.read(ctx, tenant, func(state *memory.Store) error {
		result, err = state.InspectCaseByTargetV1(ctx, tenant, id, revision, digest)
		return err
	})
	return
}
func (s *Store) InspectCaseV1(ctx context.Context, tenant core.TenantID, id string) (result contract.ReviewCaseV1, err error) {
	err = s.read(ctx, tenant, func(state *memory.Store) error { result, err = state.InspectCaseV1(ctx, tenant, id); return err })
	return
}
func (s *Store) InspectCaseExactV1(ctx context.Context, tenant core.TenantID, ref reviewport.ExactFactRefV1) (result contract.ReviewCaseV1, err error) {
	err = s.read(ctx, tenant, func(state *memory.Store) error { result, err = state.InspectCaseExactV1(ctx, tenant, ref); return err })
	return
}
func (s *Store) TransitionCaseWithTraceV2(ctx context.Context, m reviewport.TransitionCaseWithTraceMutationV2) (result contract.ReviewCaseV1, err error) {
	err = s.mutate(ctx, m.Next.TenantID, func(state *memory.Store) error {
		result, err = state.TransitionCaseWithTraceV2(ctx, m)
		return err
	})
	return
}
func (s *Store) StartRoundV1(ctx context.Context, m reviewport.StartRoundMutationV1) (caseValue contract.ReviewCaseV1, round contract.ReviewRoundV1, assignment contract.ReviewerAssignmentV1, err error) {
	err = s.mutate(ctx, m.Round.TenantID, func(state *memory.Store) error {
		caseValue, round, assignment, err = state.StartRoundV1(ctx, m)
		return err
	})
	return
}
func (s *Store) InspectRoundV1(ctx context.Context, tenant core.TenantID, id string) (result contract.ReviewRoundV1, err error) {
	err = s.read(ctx, tenant, func(state *memory.Store) error { result, err = state.InspectRoundV1(ctx, tenant, id); return err })
	return
}
func (s *Store) InspectRoundExactV1(ctx context.Context, tenant core.TenantID, ref reviewport.ExactFactRefV1) (result contract.ReviewRoundV1, err error) {
	err = s.read(ctx, tenant, func(state *memory.Store) error { result, err = state.InspectRoundExactV1(ctx, tenant, ref); return err })
	return
}
func (s *Store) InspectAssignmentV1(ctx context.Context, tenant core.TenantID, id string) (result contract.ReviewerAssignmentV1, err error) {
	err = s.read(ctx, tenant, func(state *memory.Store) error { result, err = state.InspectAssignmentV1(ctx, tenant, id); return err })
	return
}
func (s *Store) InspectAssignmentExactV1(ctx context.Context, tenant core.TenantID, ref reviewport.ExactFactRefV1) (result contract.ReviewerAssignmentV1, err error) {
	err = s.read(ctx, tenant, func(state *memory.Store) error {
		result, err = state.InspectAssignmentExactV1(ctx, tenant, ref)
		return err
	})
	return
}
func (s *Store) ClaimAssignmentV1(ctx context.Context, m reviewport.ClaimAssignmentMutationV1) (caseValue contract.ReviewCaseV1, assignment contract.ReviewerAssignmentV1, err error) {
	err = s.mutate(ctx, m.TenantID, func(state *memory.Store) error {
		caseValue, assignment, err = state.ClaimAssignmentV1(ctx, m)
		return err
	})
	return
}
func (s *Store) CreateFindingWithTraceV2(ctx context.Context, mutation reviewport.CreateFindingWithTraceMutationV2) (result contract.FindingV1, err error) {
	err = s.mutate(ctx, mutation.Finding.TenantID, func(state *memory.Store) error {
		result, err = state.CreateFindingWithTraceV2(ctx, mutation)
		return err
	})
	return
}
func (s *Store) InspectFindingV1(ctx context.Context, tenant core.TenantID, id string) (result contract.FindingV1, err error) {
	err = s.read(ctx, tenant, func(state *memory.Store) error { result, err = state.InspectFindingV1(ctx, tenant, id); return err })
	return
}
func (s *Store) InspectFindingExactV1(ctx context.Context, tenant core.TenantID, ref reviewport.ExactFactRefV1) (result contract.FindingV1, err error) {
	err = s.read(ctx, tenant, func(state *memory.Store) error {
		result, err = state.InspectFindingExactV1(ctx, tenant, ref)
		return err
	})
	return
}
func (s *Store) RecordAttestationV1(ctx context.Context, m reviewport.RecordAttestationMutationV1) (caseValue contract.ReviewCaseV1, attestation contract.AttestationV1, err error) {
	err = s.mutate(ctx, m.Attestation.TenantID, func(state *memory.Store) error {
		caseValue, attestation, err = state.RecordAttestationV1(ctx, m)
		return err
	})
	return
}
func (s *Store) InspectAttestationV1(ctx context.Context, tenant core.TenantID, id string) (result contract.AttestationV1, err error) {
	err = s.read(ctx, tenant, func(state *memory.Store) error { result, err = state.InspectAttestationV1(ctx, tenant, id); return err })
	return
}
func (s *Store) InspectAttestationExactV1(ctx context.Context, tenant core.TenantID, ref reviewport.ExactFactRefV1) (result contract.AttestationV1, err error) {
	err = s.read(ctx, tenant, func(state *memory.Store) error {
		result, err = state.InspectAttestationExactV1(ctx, tenant, ref)
		return err
	})
	return
}
func (s *Store) InspectAttestationByIdempotencyV1(ctx context.Context, tenant core.TenantID, idempotency string) (result contract.AttestationV1, err error) {
	err = s.read(ctx, tenant, func(state *memory.Store) error {
		result, err = state.InspectAttestationByIdempotencyV1(ctx, tenant, idempotency)
		return err
	})
	return
}
func (s *Store) DecideV1(ctx context.Context, m reviewport.DecideMutationV1) (caseValue contract.ReviewCaseV1, verdict contract.VerdictV1, err error) {
	err = s.mutate(ctx, m.Verdict.TenantID, func(state *memory.Store) error { caseValue, verdict, err = state.DecideV1(ctx, m); return err })
	return
}
func (s *Store) InspectVerdictV1(ctx context.Context, tenant core.TenantID, id string) (result contract.VerdictV1, err error) {
	err = s.read(ctx, tenant, func(state *memory.Store) error { result, err = state.InspectVerdictV1(ctx, tenant, id); return err })
	return
}
func (s *Store) InspectVerdictExactV1(ctx context.Context, tenant core.TenantID, ref reviewport.ExactFactRefV1) (result contract.VerdictV1, err error) {
	err = s.read(ctx, tenant, func(state *memory.Store) error {
		result, err = state.InspectVerdictExactV1(ctx, tenant, ref)
		return err
	})
	return
}
func (s *Store) InvalidateV1(ctx context.Context, m reviewport.InvalidateMutationV1) (caseValue contract.ReviewCaseV1, verdict *contract.VerdictV1, err error) {
	err = s.mutate(ctx, m.TenantID, func(state *memory.Store) error { caseValue, verdict, err = state.InvalidateV1(ctx, m); return err })
	return
}
func (s *Store) InspectTraceExactV1(ctx context.Context, tenant core.TenantID, ref reviewport.ExactFactRefV1) (result contract.TraceFactV1, err error) {
	err = s.read(ctx, tenant, func(state *memory.Store) error { result, err = state.InspectTraceExactV1(ctx, tenant, ref); return err })
	return
}
func (s *Store) ListTraceV1(ctx context.Context, tenant core.TenantID, caseID string) (result []contract.TraceFactV1, err error) {
	err = s.read(ctx, tenant, func(state *memory.Store) error { result, err = state.ListTraceV1(ctx, tenant, caseID); return err })
	return
}
func (s *Store) ListTracePageV2(ctx context.Context, request reviewport.ListTracePageRequestV2) (result reviewport.ListTracePageResultV2, err error) {
	err = s.read(ctx, request.TenantID, func(state *memory.Store) error {
		result, err = state.ListTracePageV2(ctx, request)
		return err
	})
	return
}
func (s *Store) CreateDomainResultV1(ctx context.Context, value contract.ReviewerInvocationResultFactV1) (result contract.ReviewerInvocationResultFactV1, err error) {
	err = s.mutate(ctx, value.TenantID, func(state *memory.Store) error { result, err = state.CreateDomainResultV1(ctx, value); return err })
	return
}
func (s *Store) InspectDomainResultV1(ctx context.Context, tenant core.TenantID, id string) (result contract.ReviewerInvocationResultFactV1, err error) {
	err = s.read(ctx, tenant, func(state *memory.Store) error {
		result, err = state.InspectDomainResultV1(ctx, tenant, id)
		return err
	})
	return
}
func (s *Store) InspectDomainResultExactV1(ctx context.Context, tenant core.TenantID, ref reviewport.ExactFactRefV1) (result contract.ReviewerInvocationResultFactV1, err error) {
	err = s.read(ctx, tenant, func(state *memory.Store) error {
		result, err = state.InspectDomainResultExactV1(ctx, tenant, ref)
		return err
	})
	return
}
func (s *Store) CreateApplySettlementV1(ctx context.Context, value contract.DomainApplySettlementFactV1) (result contract.DomainApplySettlementFactV1, err error) {
	err = s.mutate(ctx, value.TenantID, func(state *memory.Store) error { result, err = state.CreateApplySettlementV1(ctx, value); return err })
	return
}
func (s *Store) InspectApplySettlementV1(ctx context.Context, tenant core.TenantID, id string) (result contract.DomainApplySettlementFactV1, err error) {
	err = s.read(ctx, tenant, func(state *memory.Store) error {
		result, err = state.InspectApplySettlementV1(ctx, tenant, id)
		return err
	})
	return
}
func (s *Store) InspectApplySettlementExactV1(ctx context.Context, tenant core.TenantID, ref reviewport.ExactFactRefV1) (result contract.DomainApplySettlementFactV1, err error) {
	err = s.read(ctx, tenant, func(state *memory.Store) error {
		result, err = state.InspectApplySettlementExactV1(ctx, tenant, ref)
		return err
	})
	return
}
func (s *Store) CreateBehaviorFeedbackCandidateV1(ctx context.Context, value contract.BehaviorFeedbackCandidateV1) (result contract.BehaviorFeedbackCandidateV1, err error) {
	err = s.mutate(ctx, value.TenantID, func(state *memory.Store) error {
		result, err = state.CreateBehaviorFeedbackCandidateV1(ctx, value)
		return err
	})
	return
}
func (s *Store) InspectBehaviorFeedbackCandidateExactV1(ctx context.Context, tenant core.TenantID, ref reviewport.ExactFactRefV1) (result contract.BehaviorFeedbackCandidateV1, err error) {
	err = s.read(ctx, tenant, func(state *memory.Store) error {
		result, err = state.InspectBehaviorFeedbackCandidateExactV1(ctx, tenant, ref)
		return err
	})
	return
}

func (s *Store) CreateEvidenceAttachmentV1(ctx context.Context, mutation reviewport.CreateEvidenceAttachmentMutationV1) (result contract.EvidenceAttachmentV1, err error) {
	err = s.mutate(ctx, mutation.Attachment.TenantID, func(state *memory.Store) error {
		actual := s.clock()
		if actual.IsZero() || actual.UnixNano() <= 0 || (mutation.CheckedUnixNano > 0 && actual.UnixNano() < mutation.CheckedUnixNano) {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "review SQLite Evidence Attachment clock regressed")
		}
		mutation.CheckedUnixNano = actual.UnixNano()
		result, err = state.CreateEvidenceAttachmentV1(ctx, mutation)
		return err
	})
	return
}

func (s *Store) InspectEvidenceAttachmentExactV1(ctx context.Context, tenant core.TenantID, ref reviewport.ExactFactRefV1) (result contract.EvidenceAttachmentV1, err error) {
	err = s.read(ctx, tenant, func(state *memory.Store) error {
		result, err = state.InspectEvidenceAttachmentExactV1(ctx, tenant, ref)
		return err
	})
	return
}

func (s *Store) InspectEvidenceAttachmentByIdempotencyV1(ctx context.Context, tenant core.TenantID, idempotency string) (result contract.EvidenceAttachmentV1, err error) {
	err = s.read(ctx, tenant, func(state *memory.Store) error {
		result, err = state.InspectEvidenceAttachmentByIdempotencyV1(ctx, tenant, idempotency)
		return err
	})
	return
}
