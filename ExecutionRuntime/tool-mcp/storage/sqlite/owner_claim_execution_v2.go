package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"
	"sync"
	"time"

	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

const toolOwnerSQLiteRecoveryTimeoutV2 = 5 * time.Second

func boundedSQLiteRecoveryContextV2(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), toolOwnerSQLiteRecoveryTimeoutV2)
}

const ownerClaimExecutionSchemaVersionV2 = 2

const ownerClaimExecutionSchemaV2 = `
CREATE TABLE IF NOT EXISTS tool_owner_claim_execution_schema_v2 (
    version INTEGER PRIMARY KEY,
    digest TEXT NOT NULL,
    applied_unix_nano INTEGER NOT NULL
) STRICT;
CREATE TABLE IF NOT EXISTS tool_owner_single_call_claim_v2 (
    claim_id TEXT PRIMARY KEY,
    claim_revision INTEGER NOT NULL,
    claim_digest TEXT NOT NULL,
    request_id TEXT NOT NULL,
    request_revision INTEGER NOT NULL,
    request_digest TEXT NOT NULL,
    action_coordinate_digest TEXT NOT NULL,
    execution_scope_digest TEXT NOT NULL,
    binding_id TEXT NOT NULL,
    binding_revision INTEGER NOT NULL,
    binding_digest TEXT NOT NULL,
    input_digest TEXT NOT NULL,
    claim_json BLOB NOT NULL,
    input_json BLOB NOT NULL,
    row_digest TEXT NOT NULL,
    UNIQUE(claim_id,claim_revision,claim_digest),
    UNIQUE(request_id,request_digest,action_coordinate_digest,execution_scope_digest)
) STRICT;
CREATE TABLE IF NOT EXISTS tool_owner_execution_history_v2 (
    state_id TEXT NOT NULL,
    state_revision INTEGER NOT NULL,
    state_digest TEXT NOT NULL,
    request_key_digest TEXT NOT NULL,
    state_json BLOB NOT NULL,
    row_digest TEXT NOT NULL,
    PRIMARY KEY(state_id,state_revision),
    UNIQUE(state_id,state_revision,state_digest)
) STRICT;
CREATE TABLE IF NOT EXISTS tool_owner_execution_head_v2 (
    request_key_digest TEXT PRIMARY KEY,
    request_id TEXT NOT NULL,
    request_digest TEXT NOT NULL,
    action_coordinate_digest TEXT NOT NULL,
    execution_scope_digest TEXT NOT NULL,
    binding_id TEXT NOT NULL,
    binding_revision INTEGER NOT NULL,
    binding_digest TEXT NOT NULL,
    input_digest TEXT NOT NULL,
    state_id TEXT NOT NULL UNIQUE,
    state_revision INTEGER NOT NULL,
    state_digest TEXT NOT NULL,
    FOREIGN KEY(state_id,state_revision,state_digest)
        REFERENCES tool_owner_execution_history_v2(state_id,state_revision,state_digest)
        ON DELETE RESTRICT
) STRICT;
CREATE TABLE IF NOT EXISTS tool_owner_entry_lease_history_v2 (
    lease_id TEXT NOT NULL,
    lease_revision INTEGER NOT NULL,
    lease_digest TEXT NOT NULL,
    execution_attempt_id TEXT NOT NULL,
    request_key_digest TEXT NOT NULL,
    request_digest TEXT NOT NULL,
    input_digest TEXT NOT NULL,
    holder_incarnation_id TEXT NOT NULL,
    phase TEXT NOT NULL CHECK(phase IN ('start_or_inspect','inspect','handoff_start_or_inspect','handoff_inspect')),
    acquired_unix_nano INTEGER NOT NULL,
    expires_unix_nano INTEGER NOT NULL,
    lease_json BLOB NOT NULL,
    row_digest TEXT NOT NULL,
    PRIMARY KEY(lease_id,lease_revision),
    UNIQUE(lease_id,lease_revision,lease_digest)
) STRICT;
CREATE TABLE IF NOT EXISTS tool_owner_entry_lease_head_v2 (
    execution_attempt_id TEXT PRIMARY KEY,
    lease_id TEXT NOT NULL UNIQUE,
    lease_revision INTEGER NOT NULL,
    lease_digest TEXT NOT NULL,
    FOREIGN KEY(lease_id,lease_revision,lease_digest)
        REFERENCES tool_owner_entry_lease_history_v2(lease_id,lease_revision,lease_digest)
        ON DELETE RESTRICT
) STRICT;
`

var ownerClaimExecutionSchemaDigestV2 = core.DigestBytes([]byte(ownerClaimExecutionSchemaV2))

var ownerClaimExecutionOwnedObjectsV2 = []string{
	"tool_owner_claim_execution_schema_v2",
	"tool_owner_single_call_claim_v2",
	"tool_owner_execution_history_v2",
	"tool_owner_execution_head_v2",
	"tool_owner_entry_lease_history_v2",
	"tool_owner_entry_lease_head_v2",
}

type OwnerRequestKeyRowV2 struct {
	RequestKeyDigest       string
	RequestID              string
	RequestRevision        int64
	RequestDigest          string
	ActionCoordinateDigest string
	ExecutionScopeDigest   string
}

type OwnerClaimRowV2 struct {
	ClaimID         string
	ClaimRevision   int64
	ClaimDigest     string
	Request         OwnerRequestKeyRowV2
	BindingID       string
	BindingRevision int64
	BindingDigest   string
	InputDigest     string
	ClaimJSON       []byte
	InputJSON       []byte
	RowDigest       string
}

type ownerClaimBodyRowV2 struct {
	ContractVersion      string                                               `json:"contract_version"`
	ID                   string                                               `json:"id"`
	Revision             core.Revision                                        `json:"revision"`
	RequestID            string                                               `json:"request_id"`
	RequestDigest        core.Digest                                          `json:"request_digest"`
	ActionDigest         core.Digest                                          `json:"action_digest"`
	ExecutionScopeDigest core.Digest                                          `json:"execution_scope_digest"`
	BindingRef           toolcontract.SingleCallToolActionBindingCurrentRefV2 `json:"binding_ref"`
	CreatedUnixNano      int64                                                `json:"created_unix_nano"`
	Digest               core.Digest                                          `json:"digest"`
}

type ownerBindingProjectionBodyRowV2 struct {
	ContractVersion          string                                               `json:"contract_version"`
	Ref                      toolcontract.SingleCallToolActionBindingCurrentRefV2 `json:"ref"`
	IssuanceSubject          json.RawMessage                                      `json:"issuance_subject"`
	CandidateRef             toolcontract.ObjectRef                               `json:"candidate_ref"`
	InputContractCurrentRef  json.RawMessage                                      `json:"input_contract_current_ref"`
	CandidateClosure         json.RawMessage                                      `json:"candidate_closure"`
	S2Snapshot               json.RawMessage                                      `json:"s2_snapshot"`
	RequestedExpiresUnixNano int64                                                `json:"requested_expires_unix_nano"`
	CheckedUnixNano          int64                                                `json:"checked_unix_nano"`
	ExpiresUnixNano          int64                                                `json:"expires_unix_nano"`
	ProjectionDigest         core.Digest                                          `json:"projection_digest"`
}

type ownerExecutionInputBodyRowV2 struct {
	Request applicationcontract.SingleCallToolActionRequestV2 `json:"request"`
	Binding ownerBindingProjectionBodyRowV2                   `json:"binding"`
}

type OwnerExecutionRowV2 struct {
	RequestKeyDigest       string
	RequestID              string
	RequestDigest          string
	ActionCoordinateDigest string
	ExecutionScopeDigest   string
	BindingID              string
	BindingRevision        int64
	BindingDigest          string
	InputDigest            string
	StateID                string
	StateRevision          int64
	StateDigest            string
	StateJSON              []byte
	RowDigest              string
}

type OwnerEntryLeaseRowV2 struct {
	LeaseID             string
	LeaseRevision       int64
	LeaseDigest         string
	ExecutionAttemptID  string
	RequestKeyDigest    string
	RequestDigest       string
	InputDigest         string
	HolderIncarnationID string
	Phase               string
	AcquiredUnixNano    int64
	ExpiresUnixNano     int64
	LeaseJSON           []byte
	RowDigest           string
}

type ownerExecutionUnknownRowV2 struct {
	Class          string      `json:"class"`
	ErrorDigest    core.Digest `json:"error_digest"`
	MarkedUnixNano int64       `json:"marked_unix_nano"`
}

type ownerExecutionStateRowV2 struct {
	ContractVersion      string                                               `json:"contract_version"`
	ID                   string                                               `json:"id"`
	Revision             core.Revision                                        `json:"revision"`
	Digest               core.Digest                                          `json:"digest"`
	ClaimRef             toolcontract.ObjectRef                               `json:"claim_ref"`
	RequestKey           applicationcontract.SingleCallToolActionInspectKeyV2 `json:"request_key"`
	RequestDigest        core.Digest                                          `json:"request_digest"`
	ActionDigest         core.Digest                                          `json:"action_digest"`
	ExecutionScopeDigest core.Digest                                          `json:"execution_scope_digest"`
	BindingRef           toolcontract.SingleCallToolActionBindingCurrentRefV2 `json:"binding_ref"`
	ExecutionInputDigest core.Digest                                          `json:"execution_input_digest"`
	ExecutionAttemptID   string                                               `json:"execution_attempt_id"`
	State                string                                               `json:"state"`
	Result               *toolcontract.ObjectRef                              `json:"result,omitempty"`
	Unknown              *ownerExecutionUnknownRowV2                          `json:"unknown,omitempty"`
	CreatedUnixNano      int64                                                `json:"created_unix_nano"`
	UpdatedUnixNano      int64                                                `json:"updated_unix_nano"`
	ExpiresUnixNano      int64                                                `json:"expires_unix_nano"`
}

type ownerEntryLeaseBodyRowV2 struct {
	ContractVersion      string                                               `json:"contract_version"`
	ID                   string                                               `json:"id"`
	Revision             core.Revision                                        `json:"revision"`
	Digest               core.Digest                                          `json:"digest"`
	RequestKey           applicationcontract.SingleCallToolActionInspectKeyV2 `json:"request_key"`
	RequestDigest        core.Digest                                          `json:"request_digest"`
	ExecutionInputDigest core.Digest                                          `json:"execution_input_digest"`
	ExecutionAttemptID   string                                               `json:"execution_attempt_id"`
	HolderIncarnationID  string                                               `json:"holder_incarnation_id"`
	Phase                string                                               `json:"phase"`
	AcquiredUnixNano     int64                                                `json:"acquired_unix_nano"`
	ExpiresUnixNano      int64                                                `json:"expires_unix_nano"`
}

type OwnerClaimExecutionStoreV2 struct {
	store   *StoreV1
	fault   func(string) error
	writeMu *sync.Mutex
}

var toolOwnerSchemaMigrationMuV2 sync.Mutex
var toolOwnerDatabaseWriteGatesV2 sync.Map

func OpenOwnerClaimExecutionStoreV2(ctx context.Context, config ConfigV1) (*OwnerClaimExecutionStoreV2, error) {
	store, err := OpenV1(ctx, config)
	if err != nil {
		return nil, err
	}
	owner, err := NewOwnerClaimExecutionStoreV2(ctx, store)
	if err != nil {
		_ = store.Close()
		return nil, err
	}
	return owner, nil
}

func NewOwnerClaimExecutionStoreV2(ctx context.Context, store *StoreV1) (*OwnerClaimExecutionStoreV2, error) {
	if store == nil || store.db == nil {
		return nil, invalidV1("Tool Owner claim/execution SQLite store is required")
	}
	if store.databasePath == "" {
		return nil, invalidV1("Tool Owner claim/execution SQLite database identity is required")
	}
	gate, _ := toolOwnerDatabaseWriteGatesV2.LoadOrStore(store.databasePath, &sync.Mutex{})
	owner := &OwnerClaimExecutionStoreV2{store: store, writeMu: gate.(*sync.Mutex)}
	if err := owner.migrateV2(ctx); err != nil {
		return nil, err
	}
	return owner, nil
}

func (s *OwnerClaimExecutionStoreV2) Close() error {
	if s == nil || s.store == nil {
		return nil
	}
	return s.store.Close()
}

func (s *OwnerClaimExecutionStoreV2) migrateV2(ctx context.Context) error {
	if s == nil || s.store == nil {
		return unavailableV1("Tool Owner claim/execution SQLite store is unavailable")
	}
	if err := s.store.writeReadyV1(ctx); err != nil {
		return err
	}
	toolOwnerSchemaMigrationMuV2.Lock()
	defer toolOwnerSchemaMigrationMuV2.Unlock()
	tx, err := s.store.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return mapDBErrorV1(ctx, err, true)
	}
	defer func() { _ = tx.Rollback() }()
	disposition, err := classifyOwnedSchemaV2(ctx, tx, "tool_owner_claim_execution_schema_v2", ownerClaimExecutionOwnedObjectsV2)
	if err != nil {
		return err
	}
	if disposition == ownedSchemaCreateV2 {
		if _, err = tx.ExecContext(ctx, ownerClaimExecutionSchemaV2); err != nil {
			return mapDBErrorV1(ctx, err, true)
		}
		now := s.store.clock()
		if now.IsZero() {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Tool Owner claim/execution SQLite clock is unavailable")
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO tool_owner_claim_execution_schema_v2(version,digest,applied_unix_nano) VALUES(?,?,?)`, ownerClaimExecutionSchemaVersionV2, string(ownerClaimExecutionSchemaDigestV2), now.UnixNano()); err != nil {
			return mapDBErrorV1(ctx, err, true)
		}
	}
	if err = verifyOwnerClaimExecutionSchemaV2(ctx, tx); err != nil {
		return err
	}
	if err = verifyOwnedSchemaLedgerV2(ctx, tx, "tool_owner_claim_execution_schema_v2", ownerClaimExecutionSchemaVersionV2, ownerClaimExecutionSchemaDigestV2); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return indeterminateV1("Tool Owner claim/execution schema commit outcome is unknown")
	}
	return nil
}

func (s *OwnerClaimExecutionStoreV2) CreateClaimRowV2(ctx context.Context, row OwnerClaimRowV2) (OwnerClaimRowV2, bool, error) {
	if err := s.store.writeReadyV1(ctx); err != nil {
		return OwnerClaimRowV2{}, false, err
	}
	if err := validateOwnerClaimRowV2(row); err != nil {
		return OwnerClaimRowV2{}, false, err
	}
	s.writeMu.Lock()
	writeLocked := true
	defer func() {
		if writeLocked {
			s.writeMu.Unlock()
		}
	}()
	tx, err := s.store.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return OwnerClaimRowV2{}, false, mapDBErrorV1(ctx, err, true)
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO tool_owner_single_call_claim_v2(claim_id,claim_revision,claim_digest,request_id,request_revision,request_digest,action_coordinate_digest,execution_scope_digest,binding_id,binding_revision,binding_digest,input_digest,claim_json,input_json,row_digest) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		row.ClaimID, row.ClaimRevision, row.ClaimDigest, row.Request.RequestID, row.Request.RequestRevision, row.Request.RequestDigest, row.Request.ActionCoordinateDigest, row.Request.ExecutionScopeDigest, row.BindingID, row.BindingRevision, row.BindingDigest, row.InputDigest, row.ClaimJSON, row.InputJSON, row.RowDigest)
	if err != nil {
		return OwnerClaimRowV2{}, false, mapDBErrorV1(ctx, err, true)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		winner, readErr := inspectClaimWinnerRowTxV2(ctx, tx, row.ClaimID, row.Request)
		if readErr != nil {
			return OwnerClaimRowV2{}, false, readErr
		}
		if validateErr := validateOwnerClaimRowV2(winner); validateErr != nil {
			return OwnerClaimRowV2{}, false, validateErr
		}
		return winner, false, nil
	}
	if err = tx.Commit(); err != nil {
		return OwnerClaimRowV2{}, false, indeterminateV1("Tool Owner claim row commit outcome is unknown")
	}
	s.writeMu.Unlock()
	writeLocked = false
	if s.fault != nil {
		if err = s.fault("claim_after_commit"); err != nil {
			recoveryCtx, cancel := boundedSQLiteRecoveryContextV2(ctx)
			_, _ = s.InspectClaimRowV2(recoveryCtx, row.Request)
			cancel()
			return OwnerClaimRowV2{}, false, indeterminateV1("Tool Owner claim row reply outcome is unknown; exact Inspect completed")
		}
	}
	return cloneOwnerClaimRowV2(row), true, nil
}

func validateOwnerClaimRowV2(row OwnerClaimRowV2) error {
	var claim ownerClaimBodyRowV2
	var input ownerExecutionInputBodyRowV2
	if core.DecodeStrictJSON(row.ClaimJSON, &claim) != nil || core.DecodeStrictJSON(row.InputJSON, &input) != nil {
		return conflictV1("Tool Owner claim row JSON is not strict")
	}
	claimJSON, err := json.Marshal(claim)
	if err != nil || !bytes.Equal(claimJSON, row.ClaimJSON) {
		return conflictV1("Tool Owner claim JSON is not canonical")
	}
	inputJSON, err := json.Marshal(input)
	if err != nil || !bytes.Equal(inputJSON, row.InputJSON) {
		return conflictV1("Tool Owner claim input JSON is not canonical")
	}
	if input.Request.Validate() != nil ||
		input.Binding.ContractVersion != toolcontract.SingleCallToolActionBindingCurrentContractVersionV2 ||
		input.Binding.Ref.Validate() != nil ||
		input.Binding.CandidateRef.Validate() != nil ||
		input.Binding.RequestedExpiresUnixNano < 0 ||
		input.Binding.CheckedUnixNano <= 0 ||
		input.Binding.ExpiresUnixNano <= input.Binding.CheckedUnixNano ||
		input.Binding.ProjectionDigest.Validate() != nil {
		return conflictV1("Tool Owner claim input body is invalid")
	}
	bindingForDigest := input.Binding
	bindingForDigest.Ref.Digest = ""
	bindingForDigest.ProjectionDigest = ""
	bindingDigest, err := core.CanonicalJSONDigest("praxis.tool", toolcontract.SingleCallToolActionBindingCurrentContractVersionV2, "SingleCallToolActionBindingCurrentProjectionV2", bindingForDigest)
	if err != nil || bindingDigest != input.Binding.Ref.Digest || bindingDigest != input.Binding.ProjectionDigest {
		return conflictV1("Tool Owner claim binding digest drifted")
	}
	if claim.ContractVersion != "praxis.tool-mcp.single-call-owner-claim/v2" ||
		claim.Revision != 1 ||
		toolcontract.ValidateStableID(claim.ID) != nil ||
		claim.RequestID != input.Request.ID ||
		claim.RequestDigest != input.Request.Digest ||
		claim.ActionDigest != input.Request.Action.Digest ||
		claim.ExecutionScopeDigest != input.Request.Action.ExecutionScopeDigest ||
		claim.BindingRef != input.Binding.Ref ||
		claim.CreatedUnixNano != input.Request.CreatedUnixNano {
		return conflictV1("Tool Owner claim immutable body drifted")
	}
	claimForDigest := claim
	claimForDigest.Digest = ""
	claimDigest, err := core.CanonicalJSONDigest("praxis.tool-mcp.single-call-owner-claim", "2.0.0", "ToolOwnerSingleCallClaimV2", claimForDigest)
	if err != nil || claimDigest != claim.Digest {
		return conflictV1("Tool Owner claim digest drifted")
	}
	claimID, err := toolcontract.StableID("tool-owner-single-call-claim-v2", input.Request.ID, string(input.Request.Digest), string(input.Request.Action.ExecutionScopeDigest))
	if err != nil || claimID != claim.ID {
		return conflictV1("Tool Owner claim ID drifted")
	}
	inputDigest, err := core.CanonicalJSONDigest("praxis.tool-mcp.single-call-owner-claim-input", "2.0.0", "ToolOwnerSingleCallExecutionV2", input)
	if err != nil || string(inputDigest) != row.InputDigest {
		return conflictV1("Tool Owner claim input digest drifted")
	}
	key, err := applicationcontract.SealSingleCallToolActionInspectKeyV2(input.Request)
	if err != nil ||
		row.Request.RequestKeyDigest != string(key.Digest) ||
		row.Request.RequestID != key.RequestID ||
		row.Request.RequestRevision != int64(key.RequestRevision) ||
		row.Request.RequestDigest != string(key.RequestDigest) ||
		row.Request.ActionCoordinateDigest != string(key.ActionCoordinateDigest) ||
		row.Request.ExecutionScopeDigest != string(key.ScopeDigest) {
		return conflictV1("Tool Owner claim request key columns drifted")
	}
	if row.ClaimID != claim.ID ||
		row.ClaimRevision != int64(claim.Revision) ||
		row.ClaimDigest != string(claim.Digest) ||
		row.BindingID != claim.BindingRef.ID ||
		row.BindingRevision != int64(claim.BindingRef.Revision) ||
		row.BindingDigest != string(claim.BindingRef.Digest) {
		return conflictV1("Tool Owner claim exact columns drifted")
	}
	rowDigest, err := core.CanonicalJSONDigest("praxis.tool-mcp.sqlite-row", "v1", "ToolOwnerSingleCallClaimRecordV2", struct {
		Claim       ownerClaimBodyRowV2          `json:"claim"`
		Input       ownerExecutionInputBodyRowV2 `json:"input"`
		InputDigest core.Digest                  `json:"input_digest"`
	}{claim, input, inputDigest})
	if err != nil || string(rowDigest) != row.RowDigest {
		return conflictV1("Tool Owner claim row digest drifted")
	}
	return nil
}

func (s *OwnerClaimExecutionStoreV2) InspectClaimRowV2(ctx context.Context, key OwnerRequestKeyRowV2) (OwnerClaimRowV2, error) {
	if err := s.store.readReadyV1(ctx); err != nil {
		return OwnerClaimRowV2{}, err
	}
	row, err := scanOwnerClaimRowV2(ctx, s.store.db.QueryRowContext(ctx, `SELECT claim_id,claim_revision,claim_digest,request_id,request_revision,request_digest,action_coordinate_digest,execution_scope_digest,binding_id,binding_revision,binding_digest,input_digest,claim_json,input_json,row_digest FROM tool_owner_single_call_claim_v2 WHERE request_id=? AND request_digest=? AND action_coordinate_digest=? AND execution_scope_digest=?`, key.RequestID, key.RequestDigest, key.ActionCoordinateDigest, key.ExecutionScopeDigest))
	if err != nil {
		return OwnerClaimRowV2{}, err
	}
	if err = validateOwnerClaimRowV2(row); err != nil {
		return OwnerClaimRowV2{}, err
	}
	if row.Request != key {
		return OwnerClaimRowV2{}, conflictV1("Tool Owner claim Inspect key does not match the durable winner")
	}
	return row, nil
}

func inspectClaimWinnerRowTxV2(ctx context.Context, tx *sql.Tx, claimID string, key OwnerRequestKeyRowV2) (OwnerClaimRowV2, error) {
	return scanOwnerClaimRowV2(ctx, tx.QueryRowContext(ctx, `SELECT claim_id,claim_revision,claim_digest,request_id,request_revision,request_digest,action_coordinate_digest,execution_scope_digest,binding_id,binding_revision,binding_digest,input_digest,claim_json,input_json,row_digest FROM tool_owner_single_call_claim_v2 WHERE claim_id=? OR (request_id=? AND request_digest=? AND action_coordinate_digest=? AND execution_scope_digest=?) ORDER BY CASE WHEN claim_id=? THEN 0 ELSE 1 END LIMIT 1`, claimID, key.RequestID, key.RequestDigest, key.ActionCoordinateDigest, key.ExecutionScopeDigest, claimID))
}

func scanOwnerClaimRowV2(ctx context.Context, row scanRowV1) (OwnerClaimRowV2, error) {
	var value OwnerClaimRowV2
	if err := row.Scan(&value.ClaimID, &value.ClaimRevision, &value.ClaimDigest, &value.Request.RequestID, &value.Request.RequestRevision, &value.Request.RequestDigest, &value.Request.ActionCoordinateDigest, &value.Request.ExecutionScopeDigest, &value.BindingID, &value.BindingRevision, &value.BindingDigest, &value.InputDigest, &value.ClaimJSON, &value.InputJSON, &value.RowDigest); err != nil {
		return OwnerClaimRowV2{}, mapDBErrorV1(ctx, err, false)
	}
	// request_key_digest is a derived lookup coordinate rather than a second
	// stored identity column. Reconstruct it from the strict canonical request
	// body; validateOwnerClaimRowV2 then compares every denormalized physical
	// column against the same sealed request.
	var input ownerExecutionInputBodyRowV2
	if core.DecodeStrictJSON(value.InputJSON, &input) == nil {
		if key, err := applicationcontract.SealSingleCallToolActionInspectKeyV2(input.Request); err == nil {
			value.Request.RequestKeyDigest = string(key.Digest)
		}
	}
	return cloneOwnerClaimRowV2(value), nil
}

func (s *OwnerClaimExecutionStoreV2) CreateExecutionRowV2(ctx context.Context, row OwnerExecutionRowV2) (OwnerExecutionRowV2, bool, error) {
	if err := s.store.writeReadyV1(ctx); err != nil {
		return OwnerExecutionRowV2{}, false, err
	}
	if _, err := validateOwnerExecutionRowV2(row); err != nil {
		return OwnerExecutionRowV2{}, false, err
	}
	s.writeMu.Lock()
	writeLocked := true
	defer func() {
		if writeLocked {
			s.writeMu.Unlock()
		}
	}()
	tx, err := s.store.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return OwnerExecutionRowV2{}, false, mapDBErrorV1(ctx, err, true)
	}
	defer func() { _ = tx.Rollback() }()
	_, found, err := inspectExecutionRowQueryV2(ctx, tx, "request_key_digest", row.RequestKeyDigest)
	if err != nil {
		return OwnerExecutionRowV2{}, false, err
	}
	if found {
		initial, initialErr := inspectExecutionHistoryExactV2(ctx, tx, row)
		if initialErr != nil || !reflect.DeepEqual(initial, row) {
			return OwnerExecutionRowV2{}, false, conflictV1("Tool Owner execution start replay binds different canonical content")
		}
		return initial, false, nil
	}
	if err = insertExecutionHistoryRowTxV2(ctx, tx, row); err != nil {
		return OwnerExecutionRowV2{}, false, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO tool_owner_execution_head_v2(request_key_digest,request_id,request_digest,action_coordinate_digest,execution_scope_digest,binding_id,binding_revision,binding_digest,input_digest,state_id,state_revision,state_digest) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`,
		row.RequestKeyDigest, row.RequestID, row.RequestDigest, row.ActionCoordinateDigest, row.ExecutionScopeDigest, row.BindingID, row.BindingRevision, row.BindingDigest, row.InputDigest, row.StateID, row.StateRevision, row.StateDigest); err != nil {
		return OwnerExecutionRowV2{}, false, mapDBErrorV1(ctx, err, true)
	}
	if err = tx.Commit(); err != nil {
		return OwnerExecutionRowV2{}, false, indeterminateV1("Tool Owner execution start row commit outcome is unknown")
	}
	s.writeMu.Unlock()
	writeLocked = false
	if s.fault != nil {
		if err = s.fault("execution_start_after_commit"); err != nil {
			recoveryCtx, cancel := boundedSQLiteRecoveryContextV2(ctx)
			_, _ = s.InspectExecutionHistoryExactRowV2(recoveryCtx, row)
			cancel()
			return OwnerExecutionRowV2{}, false, indeterminateV1("Tool Owner execution start row reply outcome is unknown; exact Inspect completed")
		}
	}
	return cloneOwnerExecutionRowV2(row), true, nil
}

func (s *OwnerClaimExecutionStoreV2) InspectExecutionRowV2(ctx context.Context, requestKeyDigest string) (OwnerExecutionRowV2, error) {
	if err := s.store.readReadyV1(ctx); err != nil {
		return OwnerExecutionRowV2{}, err
	}
	row, found, err := inspectExecutionRowQueryV2(ctx, s.store.db, "request_key_digest", requestKeyDigest)
	if err != nil {
		return OwnerExecutionRowV2{}, err
	}
	if !found {
		return OwnerExecutionRowV2{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "Tool Owner execution row not found")
	}
	if _, err = validateOwnerExecutionRowV2(row); err != nil {
		return OwnerExecutionRowV2{}, err
	}
	return row, nil
}

func (s *OwnerClaimExecutionStoreV2) InspectExecutionRowByStateIDV2(ctx context.Context, stateID string) (OwnerExecutionRowV2, error) {
	if err := s.store.readReadyV1(ctx); err != nil {
		return OwnerExecutionRowV2{}, err
	}
	row, found, err := inspectExecutionRowQueryV2(ctx, s.store.db, "state_id", stateID)
	if err != nil {
		return OwnerExecutionRowV2{}, err
	}
	if !found {
		return OwnerExecutionRowV2{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "Tool Owner execution row not found")
	}
	if _, err = validateOwnerExecutionRowV2(row); err != nil {
		return OwnerExecutionRowV2{}, err
	}
	return row, nil
}

func (s *OwnerClaimExecutionStoreV2) AdvanceExecutionRowV2(ctx context.Context, expectedID string, expectedRevision int64, expectedDigest string, next OwnerExecutionRowV2) error {
	if err := s.store.writeReadyV1(ctx); err != nil {
		return err
	}
	nextState, err := validateOwnerExecutionRowV2(next)
	if err != nil {
		return err
	}
	s.writeMu.Lock()
	writeLocked := true
	defer func() {
		if writeLocked {
			s.writeMu.Unlock()
		}
	}()
	tx, err := s.store.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return mapDBErrorV1(ctx, err, true)
	}
	defer func() { _ = tx.Rollback() }()
	current, found, err := inspectExecutionRowQueryV2(ctx, tx, "state_id", expectedID)
	if err != nil {
		return err
	}
	if !found || current.StateRevision != expectedRevision || current.StateDigest != expectedDigest {
		return conflictV1("Tool Owner execution row CAS source drifted")
	}
	currentState, err := validateOwnerExecutionRowV2(current)
	if err != nil {
		return err
	}
	if err = validateOwnerExecutionAdvanceV2(current, currentState, next, nextState); err != nil {
		return err
	}
	if err = insertExecutionHistoryRowTxV2(ctx, tx, next); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE tool_owner_execution_head_v2 SET state_revision=?,state_digest=? WHERE state_id=? AND state_revision=? AND state_digest=?`, next.StateRevision, next.StateDigest, expectedID, expectedRevision, expectedDigest)
	if err != nil {
		return mapDBErrorV1(ctx, err, true)
	}
	affected, _ := result.RowsAffected()
	if affected != 1 {
		return conflictV1("Tool Owner execution row CAS lost")
	}
	if err = tx.Commit(); err != nil {
		return indeterminateV1("Tool Owner execution row commit outcome is unknown")
	}
	s.writeMu.Unlock()
	writeLocked = false
	if s.fault != nil {
		if err = s.fault("execution_advance_after_commit"); err != nil {
			recoveryCtx, cancel := boundedSQLiteRecoveryContextV2(ctx)
			_, _ = s.InspectExecutionHistoryExactRowV2(recoveryCtx, next)
			cancel()
			return indeterminateV1("Tool Owner execution row reply outcome is unknown; exact Inspect completed")
		}
	}
	return nil
}

func (s *OwnerClaimExecutionStoreV2) InspectExecutionHistoryExactRowV2(ctx context.Context, expected OwnerExecutionRowV2) (OwnerExecutionRowV2, error) {
	if err := s.store.readReadyV1(ctx); err != nil {
		return OwnerExecutionRowV2{}, err
	}
	if _, err := validateOwnerExecutionRowV2(expected); err != nil {
		return OwnerExecutionRowV2{}, err
	}
	row, err := inspectExecutionHistoryExactV2(ctx, s.store.db, expected)
	if err != nil {
		return OwnerExecutionRowV2{}, err
	}
	if _, err = validateOwnerExecutionRowV2(row); err != nil {
		return OwnerExecutionRowV2{}, err
	}
	if !reflect.DeepEqual(row, expected) {
		return OwnerExecutionRowV2{}, conflictV1("Tool Owner execution exact history row drifted")
	}
	return cloneOwnerExecutionRowV2(row), nil
}

func validateOwnerExecutionRowV2(row OwnerExecutionRowV2) (ownerExecutionStateRowV2, error) {
	var state ownerExecutionStateRowV2
	if err := core.DecodeStrictJSON(row.StateJSON, &state); err != nil {
		return ownerExecutionStateRowV2{}, conflictV1("Tool Owner raw execution state JSON is invalid")
	}
	canonical, err := json.Marshal(state)
	if err != nil || string(canonical) != string(row.StateJSON) {
		return ownerExecutionStateRowV2{}, conflictV1("Tool Owner raw execution state JSON is not canonical")
	}
	if state.ContractVersion != "praxis.tool-mcp.single-call-execution-state/v2" || state.Revision == 0 ||
		state.ClaimRef.Validate() != nil || state.RequestKey.Validate() != nil || state.RequestDigest.Validate() != nil ||
		state.ActionDigest.Validate() != nil || state.ExecutionScopeDigest.Validate() != nil || state.BindingRef.Validate() != nil ||
		state.ExecutionInputDigest.Validate() != nil || toolcontract.ValidateStableID(state.ID) != nil ||
		toolcontract.ValidateStableID(state.ExecutionAttemptID) != nil || state.CreatedUnixNano <= 0 ||
		state.UpdatedUnixNano < state.CreatedUnixNano || state.ExpiresUnixNano <= state.UpdatedUnixNano {
		return ownerExecutionStateRowV2{}, conflictV1("Tool Owner raw execution state shape is invalid")
	}
	if state.RequestKey.RequestDigest != state.RequestDigest || state.RequestKey.ActionCoordinateDigest != state.ActionDigest ||
		state.RequestKey.ScopeDigest != state.ExecutionScopeDigest {
		return ownerExecutionStateRowV2{}, conflictV1("Tool Owner raw execution request coordinates drifted")
	}
	stateID, err := toolcontract.StableID("tool-owner-execution-state-v2", state.ClaimRef.ID, string(state.ExecutionInputDigest))
	if err != nil || stateID != state.ID {
		return ownerExecutionStateRowV2{}, conflictV1("Tool Owner raw execution state ID drifted")
	}
	attemptID, err := toolcontract.StableID("tool-owner-execution-attempt-v2", state.ClaimRef.ID, string(state.ExecutionInputDigest))
	if err != nil || attemptID != state.ExecutionAttemptID {
		return ownerExecutionStateRowV2{}, conflictV1("Tool Owner raw execution attempt ID drifted")
	}
	switch state.State {
	case "start_committed":
		if state.Revision != 1 || state.Result != nil || state.Unknown != nil || state.UpdatedUnixNano != state.CreatedUnixNano {
			return ownerExecutionStateRowV2{}, conflictV1("Tool Owner raw execution start state is invalid")
		}
	case "inspect_only":
		if state.Revision != 2 || state.Result != nil || state.Unknown == nil ||
			(state.Unknown.Class != "entry_outcome_unknown" && state.Unknown.Class != "inspection_indeterminate") ||
			state.Unknown.ErrorDigest.Validate() != nil || state.Unknown.MarkedUnixNano != state.UpdatedUnixNano {
			return ownerExecutionStateRowV2{}, conflictV1("Tool Owner raw execution inspect-only state is invalid")
		}
	case "settled":
		if (state.Revision != 2 && state.Revision != 3) || state.Result == nil || state.Result.Validate() != nil || state.Unknown != nil {
			return ownerExecutionStateRowV2{}, conflictV1("Tool Owner raw execution settled state is invalid")
		}
	default:
		return ownerExecutionStateRowV2{}, conflictV1("Tool Owner raw execution state kind is invalid")
	}
	digestState := state
	digestState.Digest = ""
	digest, err := core.CanonicalJSONDigest("praxis.tool-mcp.single-call-execution-state", "2.0.0", "ToolOwnerSingleCallExecutionStateV2", digestState)
	if err != nil || digest != state.Digest {
		return ownerExecutionStateRowV2{}, conflictV1("Tool Owner raw execution state digest drifted")
	}
	rowDigest, err := core.CanonicalJSONDigest("praxis.tool-mcp.sqlite-row", "v1", "ToolOwnerSingleCallExecutionStateV2", state)
	if err != nil || string(rowDigest) != row.RowDigest {
		return ownerExecutionStateRowV2{}, conflictV1("Tool Owner raw execution row digest drifted")
	}
	if row.RequestKeyDigest != string(state.RequestKey.Digest) || row.RequestID != state.RequestKey.RequestID ||
		row.RequestDigest != string(state.RequestDigest) || row.ActionCoordinateDigest != string(state.ActionDigest) ||
		row.ExecutionScopeDigest != string(state.ExecutionScopeDigest) || row.BindingID != state.BindingRef.ID ||
		row.BindingRevision != int64(state.BindingRef.Revision) || row.BindingDigest != string(state.BindingRef.Digest) ||
		row.InputDigest != string(state.ExecutionInputDigest) || row.StateID != state.ID ||
		row.StateRevision != int64(state.Revision) || row.StateDigest != string(state.Digest) {
		return ownerExecutionStateRowV2{}, conflictV1("Tool Owner raw execution exact columns drifted")
	}
	return state, nil
}

func validateOwnerExecutionAdvanceV2(currentRow OwnerExecutionRowV2, current ownerExecutionStateRowV2, nextRow OwnerExecutionRowV2, next ownerExecutionStateRowV2) error {
	if nextRow.RequestKeyDigest != currentRow.RequestKeyDigest || nextRow.RequestID != currentRow.RequestID ||
		nextRow.RequestDigest != currentRow.RequestDigest || nextRow.ActionCoordinateDigest != currentRow.ActionCoordinateDigest ||
		nextRow.ExecutionScopeDigest != currentRow.ExecutionScopeDigest || nextRow.BindingID != currentRow.BindingID ||
		nextRow.BindingRevision != currentRow.BindingRevision || nextRow.BindingDigest != currentRow.BindingDigest ||
		nextRow.InputDigest != currentRow.InputDigest || nextRow.StateID != currentRow.StateID ||
		next.Revision != current.Revision+1 || next.ClaimRef != current.ClaimRef ||
		next.RequestKey != current.RequestKey || next.ExecutionAttemptID != current.ExecutionAttemptID ||
		next.CreatedUnixNano != current.CreatedUnixNano || next.ExpiresUnixNano != current.ExpiresUnixNano ||
		next.UpdatedUnixNano < current.UpdatedUnixNano {
		return conflictV1("Tool Owner raw execution transition changed immutable identity or violated monotonic CAS")
	}
	if current.State != "start_committed" && current.State != "inspect_only" {
		return conflictV1("Tool Owner raw execution terminal state cannot advance")
	}
	if current.State == "inspect_only" && next.State != "settled" || current.State == "start_committed" && next.State != "inspect_only" && next.State != "settled" {
		return conflictV1("Tool Owner raw execution transition is illegal")
	}
	return nil
}

func inspectExecutionRowQueryV2(ctx context.Context, query actionQueryerV2, column, key string) (OwnerExecutionRowV2, bool, error) {
	statement := `SELECT h.request_key_digest,h.request_id,h.request_digest,h.action_coordinate_digest,h.execution_scope_digest,h.binding_id,h.binding_revision,h.binding_digest,h.input_digest,h.state_id,h.state_revision,h.state_digest,x.state_json,x.row_digest
FROM tool_owner_execution_head_v2 h
JOIN tool_owner_execution_history_v2 x ON x.state_id=h.state_id AND x.state_revision=h.state_revision AND x.state_digest=h.state_digest
WHERE h.request_key_digest=?`
	if column == "state_id" {
		statement = `SELECT h.request_key_digest,h.request_id,h.request_digest,h.action_coordinate_digest,h.execution_scope_digest,h.binding_id,h.binding_revision,h.binding_digest,h.input_digest,h.state_id,h.state_revision,h.state_digest,x.state_json,x.row_digest
FROM tool_owner_execution_head_v2 h
JOIN tool_owner_execution_history_v2 x ON x.state_id=h.state_id AND x.state_revision=h.state_revision AND x.state_digest=h.state_digest
WHERE h.state_id=?`
	}
	var row OwnerExecutionRowV2
	err := query.QueryRowContext(ctx, statement, key).Scan(&row.RequestKeyDigest, &row.RequestID, &row.RequestDigest, &row.ActionCoordinateDigest, &row.ExecutionScopeDigest, &row.BindingID, &row.BindingRevision, &row.BindingDigest, &row.InputDigest, &row.StateID, &row.StateRevision, &row.StateDigest, &row.StateJSON, &row.RowDigest)
	if errors.Is(err, sql.ErrNoRows) {
		return OwnerExecutionRowV2{}, false, nil
	}
	if err != nil {
		return OwnerExecutionRowV2{}, false, mapDBErrorV1(ctx, err, false)
	}
	return cloneOwnerExecutionRowV2(row), true, nil
}

func inspectExecutionHistoryExactV2(ctx context.Context, query actionQueryerV2, expected OwnerExecutionRowV2) (OwnerExecutionRowV2, error) {
	var row OwnerExecutionRowV2
	row.RequestKeyDigest, row.RequestID, row.RequestDigest = expected.RequestKeyDigest, expected.RequestID, expected.RequestDigest
	row.ActionCoordinateDigest, row.ExecutionScopeDigest = expected.ActionCoordinateDigest, expected.ExecutionScopeDigest
	row.BindingID, row.BindingRevision, row.BindingDigest, row.InputDigest = expected.BindingID, expected.BindingRevision, expected.BindingDigest, expected.InputDigest
	row.StateID, row.StateRevision, row.StateDigest = expected.StateID, expected.StateRevision, expected.StateDigest
	if err := query.QueryRowContext(ctx, `SELECT state_json,row_digest FROM tool_owner_execution_history_v2 WHERE state_id=? AND state_revision=? AND state_digest=? AND request_key_digest=?`,
		expected.StateID, expected.StateRevision, expected.StateDigest, expected.RequestKeyDigest).Scan(&row.StateJSON, &row.RowDigest); err != nil {
		return OwnerExecutionRowV2{}, mapDBErrorV1(ctx, err, false)
	}
	return row, nil
}

func insertExecutionHistoryRowTxV2(ctx context.Context, tx *sql.Tx, row OwnerExecutionRowV2) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO tool_owner_execution_history_v2(state_id,state_revision,state_digest,request_key_digest,state_json,row_digest) VALUES(?,?,?,?,?,?)`, row.StateID, row.StateRevision, row.StateDigest, row.RequestKeyDigest, row.StateJSON, row.RowDigest)
	if err != nil {
		return mapDBErrorV1(ctx, err, true)
	}
	return nil
}

func (s *OwnerClaimExecutionStoreV2) InspectEntryLeaseRowV2(ctx context.Context, executionAttemptID string) (OwnerEntryLeaseRowV2, error) {
	if err := s.store.readReadyV1(ctx); err != nil {
		return OwnerEntryLeaseRowV2{}, err
	}
	if toolcontract.ValidateStableID(executionAttemptID) != nil {
		return OwnerEntryLeaseRowV2{}, invalidV1("Tool Owner entry lease attempt ID is invalid")
	}
	row, found, err := inspectEntryLeaseRowQueryV2(ctx, s.store.db, executionAttemptID)
	if err != nil {
		return OwnerEntryLeaseRowV2{}, err
	}
	if !found {
		return OwnerEntryLeaseRowV2{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "Tool Owner entry lease not found")
	}
	if _, err = validateOwnerEntryLeaseRowV2(row); err != nil {
		return OwnerEntryLeaseRowV2{}, err
	}
	return cloneOwnerEntryLeaseRowV2(row), nil
}

// CompareAndSwapEntryLeaseRowV2 only coordinates entry into the exact
// downstream create-once seam. It does not record or infer external effect
// state. A nil expected ref creates revision one; takeover requires that the
// prior lease has expired at the next lease's acquired timestamp.
func (s *OwnerClaimExecutionStoreV2) CompareAndSwapEntryLeaseRowV2(ctx context.Context, expected *OwnerEntryLeaseRowV2, next OwnerEntryLeaseRowV2) error {
	if err := s.store.writeReadyV1(ctx); err != nil {
		return err
	}
	nextBody, err := validateOwnerEntryLeaseRowV2(next)
	if err != nil {
		return err
	}
	s.writeMu.Lock()
	writeLocked := true
	defer func() {
		if writeLocked {
			s.writeMu.Unlock()
		}
	}()
	tx, err := s.store.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return mapDBErrorV1(ctx, err, true)
	}
	defer func() { _ = tx.Rollback() }()
	current, found, err := inspectEntryLeaseRowQueryV2(ctx, tx, next.ExecutionAttemptID)
	if err != nil {
		return err
	}
	if expected == nil {
		if found || next.LeaseRevision != 1 {
			return conflictV1("Tool Owner entry lease create source drifted")
		}
	} else {
		if _, validateErr := validateOwnerEntryLeaseRowV2(*expected); validateErr != nil {
			return validateErr
		}
		if !found || !reflect.DeepEqual(current, *expected) {
			return conflictV1("Tool Owner entry lease CAS source drifted")
		}
		currentBody, validateErr := validateOwnerEntryLeaseRowV2(current)
		if validateErr != nil {
			return validateErr
		}
		if nextBody.ID != currentBody.ID || nextBody.RequestKey != currentBody.RequestKey ||
			nextBody.RequestDigest != currentBody.RequestDigest || nextBody.ExecutionInputDigest != currentBody.ExecutionInputDigest ||
			nextBody.ExecutionAttemptID != currentBody.ExecutionAttemptID || nextBody.Revision != currentBody.Revision+1 {
			return conflictV1("Tool Owner entry lease transition changed immutable identity or revision")
		}
		if !validEntryLeaseRowTransitionV2(currentBody, nextBody) {
			return conflictV1("Tool Owner entry lease transition is neither exact handoff nor eligible takeover")
		}
	}
	if err = insertEntryLeaseHistoryRowTxV2(ctx, tx, next); err != nil {
		return err
	}
	if expected == nil {
		_, err = tx.ExecContext(ctx, `INSERT INTO tool_owner_entry_lease_head_v2(execution_attempt_id,lease_id,lease_revision,lease_digest) VALUES(?,?,?,?)`,
			next.ExecutionAttemptID, next.LeaseID, next.LeaseRevision, next.LeaseDigest)
	} else {
		var result sql.Result
		result, err = tx.ExecContext(ctx, `UPDATE tool_owner_entry_lease_head_v2 SET lease_revision=?,lease_digest=? WHERE execution_attempt_id=? AND lease_id=? AND lease_revision=? AND lease_digest=?`,
			next.LeaseRevision, next.LeaseDigest, next.ExecutionAttemptID, expected.LeaseID, expected.LeaseRevision, expected.LeaseDigest)
		if err == nil {
			affected, rowsErr := result.RowsAffected()
			if rowsErr != nil {
				return mapDBErrorV1(ctx, rowsErr, true)
			}
			if affected != 1 {
				return conflictV1("Tool Owner entry lease CAS lost")
			}
		}
	}
	if err != nil {
		return mapDBErrorV1(ctx, err, true)
	}
	if err = tx.Commit(); err != nil {
		return indeterminateV1("Tool Owner entry lease commit outcome is unknown")
	}
	s.writeMu.Unlock()
	writeLocked = false
	if s.fault != nil {
		if err = s.fault("entry_lease_after_commit"); err != nil {
			recoveryCtx, cancel := boundedSQLiteRecoveryContextV2(ctx)
			_, _ = s.InspectEntryLeaseHistoryExactRowV2(recoveryCtx, next)
			cancel()
			return indeterminateV1("Tool Owner entry lease reply outcome is unknown; exact Inspect completed")
		}
	}
	return nil
}

func (s *OwnerClaimExecutionStoreV2) InspectEntryLeaseHistoryExactRowV2(ctx context.Context, expected OwnerEntryLeaseRowV2) (OwnerEntryLeaseRowV2, error) {
	if err := s.store.readReadyV1(ctx); err != nil {
		return OwnerEntryLeaseRowV2{}, err
	}
	if _, err := validateOwnerEntryLeaseRowV2(expected); err != nil {
		return OwnerEntryLeaseRowV2{}, err
	}
	row, err := inspectEntryLeaseHistoryExactRowV2(ctx, s.store.db, expected)
	if err != nil {
		return OwnerEntryLeaseRowV2{}, err
	}
	if _, err = validateOwnerEntryLeaseRowV2(row); err != nil {
		return OwnerEntryLeaseRowV2{}, err
	}
	if !reflect.DeepEqual(row, expected) {
		return OwnerEntryLeaseRowV2{}, conflictV1("Tool Owner entry lease exact history row drifted")
	}
	return cloneOwnerEntryLeaseRowV2(row), nil
}

func validateOwnerEntryLeaseRowV2(row OwnerEntryLeaseRowV2) (ownerEntryLeaseBodyRowV2, error) {
	var lease ownerEntryLeaseBodyRowV2
	if err := core.DecodeStrictJSON(row.LeaseJSON, &lease); err != nil {
		return ownerEntryLeaseBodyRowV2{}, conflictV1("Tool Owner entry lease JSON is invalid")
	}
	canonical, err := json.Marshal(lease)
	if err != nil || !bytes.Equal(canonical, row.LeaseJSON) {
		return ownerEntryLeaseBodyRowV2{}, conflictV1("Tool Owner entry lease JSON is not canonical")
	}
	if lease.ContractVersion != "praxis.tool-mcp.single-call-entry-lease/v2" ||
		toolcontract.ValidateStableID(lease.ID) != nil || lease.Revision == 0 || lease.RequestKey.Validate() != nil ||
		lease.RequestDigest.Validate() != nil || lease.ExecutionInputDigest.Validate() != nil ||
		toolcontract.ValidateStableID(lease.ExecutionAttemptID) != nil || toolcontract.ValidateStableID(lease.HolderIncarnationID) != nil ||
		!validEntryLeaseRowPhaseV2(lease.Phase) ||
		lease.AcquiredUnixNano <= 0 || lease.ExpiresUnixNano <= lease.AcquiredUnixNano ||
		lease.RequestKey.RequestDigest != lease.RequestDigest {
		return ownerEntryLeaseBodyRowV2{}, conflictV1("Tool Owner entry lease shape is invalid")
	}
	id, err := toolcontract.StableID("tool-owner-entry-lease-v2", lease.ExecutionAttemptID)
	if err != nil || id != lease.ID {
		return ownerEntryLeaseBodyRowV2{}, conflictV1("Tool Owner entry lease ID drifted")
	}
	digestBody := lease
	digestBody.Digest = ""
	digest, err := core.CanonicalJSONDigest("praxis.tool-mcp.single-call-entry-lease", "2.0.0", "ToolOwnerSingleCallEntryLeaseV2", digestBody)
	if err != nil || digest != lease.Digest {
		return ownerEntryLeaseBodyRowV2{}, conflictV1("Tool Owner entry lease digest drifted")
	}
	rowDigest, err := core.CanonicalJSONDigest("praxis.tool-mcp.sqlite-row", "v1", "ToolOwnerSingleCallEntryLeaseV2", lease)
	if err != nil || string(rowDigest) != row.RowDigest {
		return ownerEntryLeaseBodyRowV2{}, conflictV1("Tool Owner entry lease row digest drifted")
	}
	if row.LeaseID != lease.ID || row.LeaseRevision != int64(lease.Revision) || row.LeaseDigest != string(lease.Digest) ||
		row.ExecutionAttemptID != lease.ExecutionAttemptID || row.RequestKeyDigest != string(lease.RequestKey.Digest) ||
		row.RequestDigest != string(lease.RequestDigest) || row.InputDigest != string(lease.ExecutionInputDigest) ||
		row.HolderIncarnationID != lease.HolderIncarnationID || row.Phase != lease.Phase ||
		row.AcquiredUnixNano != lease.AcquiredUnixNano || row.ExpiresUnixNano != lease.ExpiresUnixNano {
		return ownerEntryLeaseBodyRowV2{}, conflictV1("Tool Owner entry lease exact columns drifted")
	}
	return lease, nil
}

func validEntryLeaseRowPhaseV2(phase string) bool {
	return phase == "start_or_inspect" || phase == "inspect" ||
		phase == "handoff_start_or_inspect" || phase == "handoff_inspect"
}

func activeEntryLeaseRowPhaseV2(phase string) string {
	switch phase {
	case "handoff_start_or_inspect":
		return "start_or_inspect"
	case "handoff_inspect":
		return "inspect"
	default:
		return phase
	}
}

func validEntryLeaseRowTransitionV2(current, next ownerEntryLeaseBodyRowV2) bool {
	if next.AcquiredUnixNano < current.AcquiredUnixNano || next.ExpiresUnixNano < next.AcquiredUnixNano {
		return false
	}
	switch {
	case current.Phase == "start_or_inspect" && (next.Phase == "handoff_start_or_inspect" || next.Phase == "handoff_inspect"),
		current.Phase == "inspect" && (next.Phase == "handoff_start_or_inspect" || next.Phase == "handoff_inspect"):
		return next.HolderIncarnationID == current.HolderIncarnationID &&
			next.ExpiresUnixNano == current.ExpiresUnixNano &&
			next.AcquiredUnixNano < current.ExpiresUnixNano
	case current.Phase == "handoff_start_or_inspect" || current.Phase == "handoff_inspect":
		return next.Phase == activeEntryLeaseRowPhaseV2(current.Phase)
	default:
		return next.AcquiredUnixNano >= current.ExpiresUnixNano &&
			(next.Phase == "start_or_inspect" || next.Phase == "inspect")
	}
}

func inspectEntryLeaseRowQueryV2(ctx context.Context, query actionQueryerV2, executionAttemptID string) (OwnerEntryLeaseRowV2, bool, error) {
	var row OwnerEntryLeaseRowV2
	err := query.QueryRowContext(ctx, `SELECT h.lease_id,h.lease_revision,h.lease_digest,h.execution_attempt_id,
x.request_key_digest,x.request_digest,x.input_digest,x.holder_incarnation_id,x.phase,x.acquired_unix_nano,x.expires_unix_nano,x.lease_json,x.row_digest
FROM tool_owner_entry_lease_head_v2 h
JOIN tool_owner_entry_lease_history_v2 x ON x.lease_id=h.lease_id AND x.lease_revision=h.lease_revision AND x.lease_digest=h.lease_digest
WHERE h.execution_attempt_id=?`, executionAttemptID).Scan(
		&row.LeaseID, &row.LeaseRevision, &row.LeaseDigest, &row.ExecutionAttemptID,
		&row.RequestKeyDigest, &row.RequestDigest, &row.InputDigest, &row.HolderIncarnationID,
		&row.Phase, &row.AcquiredUnixNano, &row.ExpiresUnixNano, &row.LeaseJSON, &row.RowDigest,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return OwnerEntryLeaseRowV2{}, false, nil
	}
	if err != nil {
		return OwnerEntryLeaseRowV2{}, false, mapDBErrorV1(ctx, err, false)
	}
	return cloneOwnerEntryLeaseRowV2(row), true, nil
}

func inspectEntryLeaseHistoryExactRowV2(ctx context.Context, query actionQueryerV2, expected OwnerEntryLeaseRowV2) (OwnerEntryLeaseRowV2, error) {
	var row OwnerEntryLeaseRowV2
	if err := query.QueryRowContext(ctx, `SELECT lease_id,lease_revision,lease_digest,execution_attempt_id,
request_key_digest,request_digest,input_digest,holder_incarnation_id,phase,acquired_unix_nano,expires_unix_nano,lease_json,row_digest
FROM tool_owner_entry_lease_history_v2
WHERE lease_id=? AND lease_revision=? AND lease_digest=? AND execution_attempt_id=?`,
		expected.LeaseID, expected.LeaseRevision, expected.LeaseDigest, expected.ExecutionAttemptID).Scan(
		&row.LeaseID, &row.LeaseRevision, &row.LeaseDigest, &row.ExecutionAttemptID,
		&row.RequestKeyDigest, &row.RequestDigest, &row.InputDigest, &row.HolderIncarnationID,
		&row.Phase, &row.AcquiredUnixNano, &row.ExpiresUnixNano, &row.LeaseJSON, &row.RowDigest,
	); err != nil {
		return OwnerEntryLeaseRowV2{}, mapDBErrorV1(ctx, err, false)
	}
	return cloneOwnerEntryLeaseRowV2(row), nil
}

func insertEntryLeaseHistoryRowTxV2(ctx context.Context, tx *sql.Tx, row OwnerEntryLeaseRowV2) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO tool_owner_entry_lease_history_v2(
lease_id,lease_revision,lease_digest,execution_attempt_id,request_key_digest,request_digest,input_digest,
holder_incarnation_id,phase,acquired_unix_nano,expires_unix_nano,lease_json,row_digest
) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		row.LeaseID, row.LeaseRevision, row.LeaseDigest, row.ExecutionAttemptID, row.RequestKeyDigest,
		row.RequestDigest, row.InputDigest, row.HolderIncarnationID, row.Phase, row.AcquiredUnixNano,
		row.ExpiresUnixNano, row.LeaseJSON, row.RowDigest)
	if err != nil {
		return mapDBErrorV1(ctx, err, true)
	}
	return nil
}

func cloneOwnerClaimRowV2(row OwnerClaimRowV2) OwnerClaimRowV2 {
	row.ClaimJSON = append([]byte(nil), row.ClaimJSON...)
	row.InputJSON = append([]byte(nil), row.InputJSON...)
	return row
}

func cloneOwnerExecutionRowV2(row OwnerExecutionRowV2) OwnerExecutionRowV2 {
	row.StateJSON = append([]byte(nil), row.StateJSON...)
	return row
}

func cloneOwnerEntryLeaseRowV2(row OwnerEntryLeaseRowV2) OwnerEntryLeaseRowV2 {
	row.LeaseJSON = append([]byte(nil), row.LeaseJSON...)
	return row
}
