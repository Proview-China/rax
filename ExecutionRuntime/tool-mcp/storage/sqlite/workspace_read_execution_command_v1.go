package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	ownercommand "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/owner/workspacereadcommandrepo"
)

const workspaceReadExecutionCommandSchemaVersionV1 = 1

const workspaceReadExecutionCommandSchemaV1 = `
CREATE TABLE IF NOT EXISTS tool_workspace_read_execution_command_schema_v1 (
    version INTEGER PRIMARY KEY,
    digest TEXT NOT NULL,
    applied_unix_nano INTEGER NOT NULL
) STRICT;
CREATE TABLE IF NOT EXISTS tool_workspace_read_execution_command_v1 (
    command_id TEXT PRIMARY KEY,
    command_revision INTEGER NOT NULL CHECK(command_revision = 1),
    command_digest TEXT NOT NULL,
    request_key_digest TEXT NOT NULL UNIQUE,
    request_id TEXT NOT NULL,
    request_revision INTEGER NOT NULL,
    request_digest TEXT NOT NULL,
    action_coordinate_digest TEXT NOT NULL,
    execution_scope_digest TEXT NOT NULL,
    claim_id TEXT NOT NULL,
    claim_revision INTEGER NOT NULL,
    claim_digest TEXT NOT NULL,
    execution_state_id TEXT NOT NULL,
    execution_state_revision INTEGER NOT NULL,
    execution_state_digest TEXT NOT NULL,
    execution_input_digest TEXT NOT NULL,
    tool_execution_attempt_id TEXT NOT NULL UNIQUE,
    binding_id TEXT NOT NULL,
    binding_revision INTEGER NOT NULL,
    binding_digest TEXT NOT NULL,
    candidate_id TEXT NOT NULL,
    candidate_revision INTEGER NOT NULL,
    candidate_digest TEXT NOT NULL,
    candidate_closure_digest TEXT NOT NULL,
    input_contract_id TEXT NOT NULL,
    input_contract_revision INTEGER NOT NULL,
    input_contract_digest TEXT NOT NULL,
    tool_id TEXT NOT NULL,
    tool_revision INTEGER NOT NULL,
    tool_digest TEXT NOT NULL,
    tool_current_kind TEXT NOT NULL,
    tool_current_id TEXT NOT NULL,
    tool_current_revision INTEGER NOT NULL,
    tool_current_digest TEXT NOT NULL,
    operation_digest TEXT NOT NULL,
    prepared_id TEXT NOT NULL,
    prepared_revision INTEGER NOT NULL,
    prepared_digest TEXT NOT NULL,
    runtime_attempt_digest TEXT NOT NULL UNIQUE,
    payload_schema_key TEXT NOT NULL,
    payload_digest TEXT NOT NULL,
    payload_revision INTEGER NOT NULL,
    created_unix_nano INTEGER NOT NULL,
    not_after_unix_nano INTEGER NOT NULL,
    body_json BLOB NOT NULL,
    row_digest TEXT NOT NULL,
    UNIQUE(command_id,command_revision,command_digest),
    UNIQUE(request_id,request_revision,request_digest,action_coordinate_digest,execution_scope_digest)
) STRICT;
`

var workspaceReadExecutionCommandSchemaDigestV1 = core.DigestBytes([]byte(workspaceReadExecutionCommandSchemaV1))

var workspaceReadExecutionCommandOwnedObjectsV1 = []string{
	"tool_workspace_read_execution_command_schema_v1",
	"tool_workspace_read_execution_command_v1",
}

var workspaceReadExecutionCommandMigrationMuV1 sync.Mutex
var workspaceReadExecutionCommandWriteGatesV1 sync.Map

type workspaceReadExecutionCommandWriteGateV1 struct {
	mu      sync.Mutex
	lastNow time.Time
}

type WorkspaceReadExecutionCommandStoreV1 struct {
	store     *StoreV1
	writeGate *workspaceReadExecutionCommandWriteGateV1
	fault     func(string) error
}

func OpenWorkspaceReadExecutionCommandStoreV1(
	ctx context.Context,
	config ConfigV1,
) (*WorkspaceReadExecutionCommandStoreV1, error) {
	store, err := OpenV1(ctx, config)
	if err != nil {
		return nil, err
	}
	result, err := NewWorkspaceReadExecutionCommandStoreV1(ctx, store)
	if err != nil {
		_ = store.Close()
		return nil, err
	}
	return result, nil
}

func NewWorkspaceReadExecutionCommandStoreV1(
	ctx context.Context,
	store *StoreV1,
) (*WorkspaceReadExecutionCommandStoreV1, error) {
	if store == nil || store.db == nil || store.databasePath == "" {
		return nil, invalidV1("workspace.read execution command SQLite store is required")
	}
	gate, _ := workspaceReadExecutionCommandWriteGatesV1.LoadOrStore(store.databasePath, &workspaceReadExecutionCommandWriteGateV1{})
	result := &WorkspaceReadExecutionCommandStoreV1{store: store, writeGate: gate.(*workspaceReadExecutionCommandWriteGateV1)}
	if err := result.migrateV1(ctx); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *WorkspaceReadExecutionCommandStoreV1) Close() error {
	if s == nil || s.store == nil {
		return nil
	}
	return s.store.Close()
}

func (s *WorkspaceReadExecutionCommandStoreV1) migrateV1(ctx context.Context) error {
	if s == nil || s.store == nil {
		return unavailableV1("workspace.read execution command SQLite store is unavailable")
	}
	if err := s.store.writeReadyV1(ctx); err != nil {
		return err
	}
	workspaceReadExecutionCommandMigrationMuV1.Lock()
	defer workspaceReadExecutionCommandMigrationMuV1.Unlock()
	tx, err := s.store.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return mapDBErrorV1(ctx, err, true)
	}
	defer func() { _ = tx.Rollback() }()
	disposition, err := classifyOwnedSchemaV2(
		ctx,
		tx,
		"tool_workspace_read_execution_command_schema_v1",
		workspaceReadExecutionCommandOwnedObjectsV1,
	)
	if err != nil {
		return err
	}
	if disposition == ownedSchemaCreateV2 {
		if _, err = tx.ExecContext(ctx, workspaceReadExecutionCommandSchemaV1); err != nil {
			return mapDBErrorV1(ctx, err, true)
		}
		now := s.store.clock()
		if now.IsZero() {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "workspace.read execution command SQLite clock is unavailable")
		}
		if _, err = tx.ExecContext(
			ctx,
			`INSERT INTO tool_workspace_read_execution_command_schema_v1(version,digest,applied_unix_nano) VALUES(?,?,?)`,
			workspaceReadExecutionCommandSchemaVersionV1,
			string(workspaceReadExecutionCommandSchemaDigestV1),
			now.UnixNano(),
		); err != nil {
			return mapDBErrorV1(ctx, err, true)
		}
	}
	if err = verifyWorkspaceReadExecutionCommandSchemaV1(ctx, tx); err != nil {
		return err
	}
	if err = verifyOwnedSchemaLedgerV2(
		ctx,
		tx,
		"tool_workspace_read_execution_command_schema_v1",
		workspaceReadExecutionCommandSchemaVersionV1,
		workspaceReadExecutionCommandSchemaDigestV1,
	); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return indeterminateV1("workspace.read execution command schema commit outcome is unknown")
	}
	return nil
}

func verifyWorkspaceReadExecutionCommandSchemaV1(ctx context.Context, query schemaQueryerV2) error {
	if err := verifyStrictTableV2(
		ctx,
		query,
		"tool_workspace_read_execution_command_schema_v1",
		colsV2("version:INTEGER:1:nullable", "digest:TEXT", "applied_unix_nano:INTEGER"),
		[][]string{{"version"}},
		workspaceReadExecutionCommandSchemaV1,
	); err != nil {
		return err
	}
	columns := colsV2(
		"command_id:TEXT:1",
		"command_revision:INTEGER",
		"command_digest:TEXT",
		"request_key_digest:TEXT",
		"request_id:TEXT",
		"request_revision:INTEGER",
		"request_digest:TEXT",
		"action_coordinate_digest:TEXT",
		"execution_scope_digest:TEXT",
		"claim_id:TEXT",
		"claim_revision:INTEGER",
		"claim_digest:TEXT",
		"execution_state_id:TEXT",
		"execution_state_revision:INTEGER",
		"execution_state_digest:TEXT",
		"execution_input_digest:TEXT",
		"tool_execution_attempt_id:TEXT",
		"binding_id:TEXT",
		"binding_revision:INTEGER",
		"binding_digest:TEXT",
		"candidate_id:TEXT",
		"candidate_revision:INTEGER",
		"candidate_digest:TEXT",
		"candidate_closure_digest:TEXT",
		"input_contract_id:TEXT",
		"input_contract_revision:INTEGER",
		"input_contract_digest:TEXT",
		"tool_id:TEXT",
		"tool_revision:INTEGER",
		"tool_digest:TEXT",
		"tool_current_kind:TEXT",
		"tool_current_id:TEXT",
		"tool_current_revision:INTEGER",
		"tool_current_digest:TEXT",
		"operation_digest:TEXT",
		"prepared_id:TEXT",
		"prepared_revision:INTEGER",
		"prepared_digest:TEXT",
		"runtime_attempt_digest:TEXT",
		"payload_schema_key:TEXT",
		"payload_digest:TEXT",
		"payload_revision:INTEGER",
		"created_unix_nano:INTEGER",
		"not_after_unix_nano:INTEGER",
		"body_json:BLOB",
		"row_digest:TEXT",
	)
	return verifyStrictTableV2(
		ctx,
		query,
		"tool_workspace_read_execution_command_v1",
		columns,
		[][]string{
			{"command_id"},
			{"request_key_digest"},
			{"tool_execution_attempt_id"},
			{"runtime_attempt_digest"},
			{"command_id", "command_revision", "command_digest"},
			{"request_id", "request_revision", "request_digest", "action_coordinate_digest", "execution_scope_digest"},
		},
		workspaceReadExecutionCommandSchemaV1,
	)
}

// CreateWorkspaceReadExecutionCommandOwnedV1 is an owner-storage method. It is
// intentionally absent from public ports; only the Tool-owned producer may
// pass a Fact assembled from authoritative readers.
func (s *WorkspaceReadExecutionCommandStoreV1) CreateWorkspaceReadExecutionCommandOwnedV1(
	ctx context.Context,
	capability ownercommand.WriteCapabilityV1,
	fact toolcontract.WorkspaceReadExecutionCommandV1,
) (toolcontract.WorkspaceReadExecutionCommandV1, bool, error) {
	if s == nil || s.store == nil || s.writeGate == nil {
		return toolcontract.WorkspaceReadExecutionCommandV1{}, false, unavailableV1("workspace.read execution command SQLite store is unavailable")
	}
	if err := s.store.writeReadyV1(ctx); err != nil {
		return toolcontract.WorkspaceReadExecutionCommandV1{}, false, err
	}
	if err := capability.Validate(); err != nil {
		return toolcontract.WorkspaceReadExecutionCommandV1{}, false, err
	}
	fact = toolcontract.CloneWorkspaceReadExecutionCommandV1(fact)
	if err := fact.Validate(); err != nil {
		return toolcontract.WorkspaceReadExecutionCommandV1{}, false, err
	}
	body, rowDigest, err := encodeRowV1("WorkspaceReadExecutionCommandV1", fact)
	if err != nil {
		return toolcontract.WorkspaceReadExecutionCommandV1{}, false, err
	}
	s.writeGate.mu.Lock()
	writeLocked := true
	defer func() {
		if writeLocked {
			s.writeGate.mu.Unlock()
		}
	}()
	tx, err := s.store.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return toolcontract.WorkspaceReadExecutionCommandV1{}, false, mapWorkspaceReadExecutionCommandPreCommitErrorV1(ctx, err)
	}
	defer func() { _ = tx.Rollback() }()
	winner, exists, err := inspectWorkspaceReadExecutionCommandWinnerIfPresentV1(ctx, tx, fact)
	if err != nil {
		return toolcontract.WorkspaceReadExecutionCommandV1{}, false, err
	}
	if exists {
		same, compareErr := toolcontract.SameWorkspaceReadExecutionCommandStableClosureV1(winner, fact)
		if compareErr != nil {
			return toolcontract.WorkspaceReadExecutionCommandV1{}, false, compareErr
		}
		if !same {
			return toolcontract.WorkspaceReadExecutionCommandV1{}, false, conflictV1("workspace.read execution command unique axis binds different stable content")
		}
		if err = ctx.Err(); err != nil {
			return toolcontract.WorkspaceReadExecutionCommandV1{}, false, err
		}
		return toolcontract.CloneWorkspaceReadExecutionCommandV1(winner), false, nil
	}
	if err = s.validateMutationClockLockedV1(fact); err != nil {
		return toolcontract.WorkspaceReadExecutionCommandV1{}, false, err
	}
	if s.fault != nil {
		if err = s.fault("workspace_read_execution_command_before_insert"); err != nil {
			return toolcontract.WorkspaceReadExecutionCommandV1{}, false, mapWorkspaceReadExecutionCommandPreCommitErrorV1(ctx, err)
		}
	}
	result, err := tx.ExecContext(ctx, `
INSERT OR IGNORE INTO tool_workspace_read_execution_command_v1(
command_id,command_revision,command_digest,
request_key_digest,request_id,request_revision,request_digest,action_coordinate_digest,execution_scope_digest,
claim_id,claim_revision,claim_digest,
execution_state_id,execution_state_revision,execution_state_digest,execution_input_digest,tool_execution_attempt_id,
binding_id,binding_revision,binding_digest,
candidate_id,candidate_revision,candidate_digest,candidate_closure_digest,
input_contract_id,input_contract_revision,input_contract_digest,
tool_id,tool_revision,tool_digest,
tool_current_kind,tool_current_id,tool_current_revision,tool_current_digest,
operation_digest,prepared_id,prepared_revision,prepared_digest,runtime_attempt_digest,
payload_schema_key,payload_digest,payload_revision,created_unix_nano,not_after_unix_nano,body_json,row_digest
) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		fact.Ref.ID, int64(fact.Ref.Revision), string(fact.Ref.Digest),
		string(fact.Source.RequestKey.Digest), fact.Source.RequestKey.RequestID, int64(fact.Source.RequestKey.RequestRevision),
		string(fact.Source.RequestKey.RequestDigest), string(fact.Source.RequestKey.ActionCoordinateDigest), string(fact.Source.RequestKey.ScopeDigest),
		fact.Source.ClaimRef.ID, int64(fact.Source.ClaimRef.Revision), string(fact.Source.ClaimRef.Digest),
		fact.Source.ExecutionStateRef.ID, int64(fact.Source.ExecutionStateRef.Revision), string(fact.Source.ExecutionStateRef.Digest),
		string(fact.Source.ExecutionInputDigest), fact.Source.ToolExecutionAttemptID,
		fact.Source.BindingCurrent.ID, int64(fact.Source.BindingCurrent.Revision), string(fact.Source.BindingCurrent.Digest),
		fact.Source.Candidate.ID, int64(fact.Source.Candidate.Revision), string(fact.Source.Candidate.Digest), string(fact.Source.CandidateClosureDigest),
		fact.Source.InputContractCurrent.ID, int64(fact.Source.InputContractCurrent.Revision), string(fact.Source.InputContractCurrent.Digest),
		fact.Source.Tool.ID, int64(fact.Source.Tool.Revision), string(fact.Source.Tool.Digest),
		string(fact.Source.ToolCurrent.Kind), fact.Source.ToolCurrent.ID, int64(fact.Source.ToolCurrent.Revision), string(fact.Source.ToolCurrent.Digest),
		string(fact.OperationDigest), fact.Prepared.ID, int64(fact.Prepared.Revision), string(fact.Prepared.Digest), string(fact.RuntimeAttemptDigest),
		fact.PayloadSchema.Key(), string(fact.PayloadDigest), int64(fact.PayloadRevision),
		fact.CreatedUnixNano, fact.NotAfterUnixNano, body, string(rowDigest),
	)
	if err != nil {
		return toolcontract.WorkspaceReadExecutionCommandV1{}, false, mapWorkspaceReadExecutionCommandPreCommitErrorV1(ctx, err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return toolcontract.WorkspaceReadExecutionCommandV1{}, false, mapWorkspaceReadExecutionCommandPreCommitErrorV1(ctx, err)
	}
	if affected == 0 {
		winner, inspectErr := inspectWorkspaceReadExecutionCommandWinnerV1(ctx, tx, fact)
		if inspectErr != nil {
			return toolcontract.WorkspaceReadExecutionCommandV1{}, false, inspectErr
		}
		same, compareErr := toolcontract.SameWorkspaceReadExecutionCommandStableClosureV1(winner, fact)
		if compareErr != nil {
			return toolcontract.WorkspaceReadExecutionCommandV1{}, false, compareErr
		}
		if !same {
			return toolcontract.WorkspaceReadExecutionCommandV1{}, false, conflictV1("workspace.read execution command unique axis binds different stable content")
		}
		if err = ctx.Err(); err != nil {
			return toolcontract.WorkspaceReadExecutionCommandV1{}, false, err
		}
		return toolcontract.CloneWorkspaceReadExecutionCommandV1(winner), false, nil
	}
	if err = ctx.Err(); err != nil {
		return toolcontract.WorkspaceReadExecutionCommandV1{}, false, err
	}
	if s.fault != nil {
		if err = s.fault("workspace_read_execution_command_before_commit"); err != nil {
			return toolcontract.WorkspaceReadExecutionCommandV1{}, false, mapWorkspaceReadExecutionCommandPreCommitErrorV1(ctx, err)
		}
	}
	if err = s.validateMutationClockLockedV1(fact); err != nil {
		return toolcontract.WorkspaceReadExecutionCommandV1{}, false, err
	}
	if err = tx.Commit(); err != nil {
		return toolcontract.WorkspaceReadExecutionCommandV1{}, false, indeterminateV1("workspace.read execution command commit outcome is unknown")
	}
	s.writeGate.mu.Unlock()
	writeLocked = false
	if s.fault != nil {
		if err = s.fault("workspace_read_execution_command_after_commit"); err != nil {
			return toolcontract.WorkspaceReadExecutionCommandV1{}, false, indeterminateV1("workspace.read execution command reply outcome is unknown")
		}
	}
	return toolcontract.CloneWorkspaceReadExecutionCommandV1(fact), true, nil
}

// Every error mapped here occurs before Commit and therefore has a known
// rollback outcome. It must never be reported as an unknown external effect.
func mapWorkspaceReadExecutionCommandPreCommitErrorV1(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "constraint") || strings.Contains(message, "unique") {
		return conflictV1("workspace.read execution command SQLite uniqueness conflict")
	}
	if strings.Contains(message, "busy") || strings.Contains(message, "locked") {
		return unavailableV1("workspace.read execution command SQLite is busy")
	}
	return unavailableV1("workspace.read execution command SQLite pre-commit operation failed")
}

func (s *WorkspaceReadExecutionCommandStoreV1) validateMutationClockLockedV1(
	fact toolcontract.WorkspaceReadExecutionCommandV1,
) error {
	now := s.store.clock()
	if now.IsZero() || !s.writeGate.lastNow.IsZero() && now.Before(s.writeGate.lastNow) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "workspace.read execution command SQLite clock regressed")
	}
	s.writeGate.lastNow = now
	return fact.ValidateCurrent(now)
}

func (s *WorkspaceReadExecutionCommandStoreV1) InspectWorkspaceReadExecutionCommandExactV1(
	ctx context.Context,
	exact toolcontract.WorkspaceReadExecutionCommandRefV1,
) (toolcontract.WorkspaceReadExecutionCommandV1, error) {
	if s == nil || s.store == nil {
		return toolcontract.WorkspaceReadExecutionCommandV1{}, unavailableV1("workspace.read execution command SQLite store is unavailable")
	}
	if err := s.store.readReadyV1(ctx); err != nil {
		return toolcontract.WorkspaceReadExecutionCommandV1{}, err
	}
	if err := exact.Validate(); err != nil {
		return toolcontract.WorkspaceReadExecutionCommandV1{}, err
	}
	fact, err := inspectWorkspaceReadExecutionCommandByIDV1(ctx, s.store.db, exact.ID)
	if err != nil {
		return toolcontract.WorkspaceReadExecutionCommandV1{}, err
	}
	if fact.Ref != exact {
		return toolcontract.WorkspaceReadExecutionCommandV1{}, conflictV1("workspace.read execution command exact Ref drifted")
	}
	return toolcontract.CloneWorkspaceReadExecutionCommandV1(fact), nil
}

func (s *WorkspaceReadExecutionCommandStoreV1) InspectWorkspaceReadExecutionCommandByRuntimeAttemptV1(
	ctx context.Context,
	attempt runtimeports.OperationDispatchAttemptRefV3,
) (toolcontract.WorkspaceReadExecutionCommandV1, error) {
	if s == nil || s.store == nil {
		return toolcontract.WorkspaceReadExecutionCommandV1{}, unavailableV1("workspace.read execution command SQLite store is unavailable")
	}
	if err := s.store.readReadyV1(ctx); err != nil {
		return toolcontract.WorkspaceReadExecutionCommandV1{}, err
	}
	attemptDigest, err := toolcontract.DigestWorkspaceReadExecutionRuntimeAttemptV1(attempt)
	if err != nil {
		return toolcontract.WorkspaceReadExecutionCommandV1{}, err
	}
	fact, err := inspectWorkspaceReadExecutionCommandByAttemptDigestV1(ctx, s.store.db, attemptDigest)
	if err != nil {
		return toolcontract.WorkspaceReadExecutionCommandV1{}, err
	}
	if fact.RuntimeAttemptDigest != attemptDigest ||
		!toolcontract.SameWorkspaceReadExecutionRuntimeAttemptV1(fact.RuntimeAttempt, attempt) {
		return toolcontract.WorkspaceReadExecutionCommandV1{}, conflictV1("workspace.read execution command Runtime Attempt reverse index drifted")
	}
	return toolcontract.CloneWorkspaceReadExecutionCommandV1(fact), nil
}

type workspaceReadExecutionCommandQueryerV1 interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func inspectWorkspaceReadExecutionCommandWinnerV1(
	ctx context.Context,
	query workspaceReadExecutionCommandQueryerV1,
	expected toolcontract.WorkspaceReadExecutionCommandV1,
) (toolcontract.WorkspaceReadExecutionCommandV1, error) {
	ids := make(map[string]struct{}, 4)
	for _, lookup := range []struct {
		query string
		arg   any
	}{
		{`SELECT command_id FROM tool_workspace_read_execution_command_v1 WHERE command_id=?`, expected.Ref.ID},
		{`SELECT command_id FROM tool_workspace_read_execution_command_v1 WHERE request_key_digest=?`, string(expected.Source.RequestKey.Digest)},
		{`SELECT command_id FROM tool_workspace_read_execution_command_v1 WHERE tool_execution_attempt_id=?`, expected.Source.ToolExecutionAttemptID},
		{`SELECT command_id FROM tool_workspace_read_execution_command_v1 WHERE runtime_attempt_digest=?`, string(expected.RuntimeAttemptDigest)},
	} {
		var id string
		err := query.QueryRowContext(ctx, lookup.query, lookup.arg).Scan(&id)
		if err != nil {
			if err == sql.ErrNoRows {
				return toolcontract.WorkspaceReadExecutionCommandV1{}, conflictV1("workspace.read execution command unique index set is incomplete")
			}
			return toolcontract.WorkspaceReadExecutionCommandV1{}, mapDBErrorV1(ctx, err, false)
		}
		ids[id] = struct{}{}
	}
	if len(ids) != 1 {
		return toolcontract.WorkspaceReadExecutionCommandV1{}, conflictV1("workspace.read execution command unique axes point to different winners")
	}
	var id string
	for value := range ids {
		id = value
	}
	return inspectWorkspaceReadExecutionCommandByIDV1(ctx, query, id)
}

func inspectWorkspaceReadExecutionCommandWinnerIfPresentV1(
	ctx context.Context,
	query workspaceReadExecutionCommandQueryerV1,
	expected toolcontract.WorkspaceReadExecutionCommandV1,
) (toolcontract.WorkspaceReadExecutionCommandV1, bool, error) {
	ids := make(map[string]struct{}, 4)
	found := 0
	for _, lookup := range []struct {
		query string
		arg   any
	}{
		{`SELECT command_id FROM tool_workspace_read_execution_command_v1 WHERE command_id=?`, expected.Ref.ID},
		{`SELECT command_id FROM tool_workspace_read_execution_command_v1 WHERE request_key_digest=?`, string(expected.Source.RequestKey.Digest)},
		{`SELECT command_id FROM tool_workspace_read_execution_command_v1 WHERE tool_execution_attempt_id=?`, expected.Source.ToolExecutionAttemptID},
		{`SELECT command_id FROM tool_workspace_read_execution_command_v1 WHERE runtime_attempt_digest=?`, string(expected.RuntimeAttemptDigest)},
	} {
		var id string
		err := query.QueryRowContext(ctx, lookup.query, lookup.arg).Scan(&id)
		if err != nil {
			if err == sql.ErrNoRows {
				continue
			}
			return toolcontract.WorkspaceReadExecutionCommandV1{}, false, mapDBErrorV1(ctx, err, false)
		}
		found++
		ids[id] = struct{}{}
	}
	if found == 0 {
		return toolcontract.WorkspaceReadExecutionCommandV1{}, false, nil
	}
	if found != 4 || len(ids) != 1 {
		return toolcontract.WorkspaceReadExecutionCommandV1{}, false, conflictV1("workspace.read execution command unique axes are partially bound or split")
	}
	var id string
	for value := range ids {
		id = value
	}
	winner, err := inspectWorkspaceReadExecutionCommandByIDV1(ctx, query, id)
	return winner, err == nil, err
}

func inspectWorkspaceReadExecutionCommandByIDV1(
	ctx context.Context,
	query workspaceReadExecutionCommandQueryerV1,
	id string,
) (toolcontract.WorkspaceReadExecutionCommandV1, error) {
	return decodeWorkspaceReadExecutionCommandRowV1(ctx, query.QueryRowContext(ctx, workspaceReadExecutionCommandSelectV1+` WHERE command_id=?`, id))
}

func inspectWorkspaceReadExecutionCommandByAttemptDigestV1(
	ctx context.Context,
	query workspaceReadExecutionCommandQueryerV1,
	digest core.Digest,
) (toolcontract.WorkspaceReadExecutionCommandV1, error) {
	return decodeWorkspaceReadExecutionCommandRowV1(ctx, query.QueryRowContext(ctx, workspaceReadExecutionCommandSelectV1+` WHERE runtime_attempt_digest=?`, string(digest)))
}

const workspaceReadExecutionCommandSelectV1 = `
SELECT
command_id,command_revision,command_digest,
request_key_digest,request_id,request_revision,request_digest,action_coordinate_digest,execution_scope_digest,
claim_id,claim_revision,claim_digest,
execution_state_id,execution_state_revision,execution_state_digest,execution_input_digest,tool_execution_attempt_id,
binding_id,binding_revision,binding_digest,
candidate_id,candidate_revision,candidate_digest,candidate_closure_digest,
input_contract_id,input_contract_revision,input_contract_digest,
tool_id,tool_revision,tool_digest,
tool_current_kind,tool_current_id,tool_current_revision,tool_current_digest,
operation_digest,prepared_id,prepared_revision,prepared_digest,runtime_attempt_digest,
payload_schema_key,payload_digest,payload_revision,created_unix_nano,not_after_unix_nano,body_json,row_digest
FROM tool_workspace_read_execution_command_v1`

func decodeWorkspaceReadExecutionCommandRowV1(
	ctx context.Context,
	row scanRowV1,
) (toolcontract.WorkspaceReadExecutionCommandV1, error) {
	var commandID, commandDigest string
	var requestKeyDigest, requestID, requestDigest, actionDigest, scopeDigest string
	var claimID, claimDigest, stateID, stateDigest, inputDigest, toolAttemptID string
	var bindingID, bindingDigest, candidateID, candidateDigest, closureDigest string
	var inputContractID, inputContractDigest, toolID, toolDigest string
	var toolCurrentKind, toolCurrentID, toolCurrentDigest string
	var operationDigest, preparedID, preparedDigest, runtimeAttemptDigest string
	var payloadSchemaKey, payloadDigest, storedRowDigest string
	var commandRevision, requestRevision, claimRevision, stateRevision int64
	var bindingRevision, candidateRevision, inputContractRevision, toolRevision int64
	var toolCurrentRevision, preparedRevision, payloadRevision int64
	var createdUnixNano, notAfterUnixNano int64
	var body []byte
	if err := row.Scan(
		&commandID, &commandRevision, &commandDigest,
		&requestKeyDigest, &requestID, &requestRevision, &requestDigest, &actionDigest, &scopeDigest,
		&claimID, &claimRevision, &claimDigest,
		&stateID, &stateRevision, &stateDigest, &inputDigest, &toolAttemptID,
		&bindingID, &bindingRevision, &bindingDigest,
		&candidateID, &candidateRevision, &candidateDigest, &closureDigest,
		&inputContractID, &inputContractRevision, &inputContractDigest,
		&toolID, &toolRevision, &toolDigest,
		&toolCurrentKind, &toolCurrentID, &toolCurrentRevision, &toolCurrentDigest,
		&operationDigest, &preparedID, &preparedRevision, &preparedDigest, &runtimeAttemptDigest,
		&payloadSchemaKey, &payloadDigest, &payloadRevision, &createdUnixNano, &notAfterUnixNano,
		&body, &storedRowDigest,
	); err != nil {
		return toolcontract.WorkspaceReadExecutionCommandV1{}, mapDBErrorV1(ctx, err, false)
	}
	var fact toolcontract.WorkspaceReadExecutionCommandV1
	if core.DecodeStrictJSON(body, &fact) != nil || !canonicalJSONBytesV1(body, fact) || fact.Validate() != nil {
		return toolcontract.WorkspaceReadExecutionCommandV1{}, conflictV1("stored workspace.read execution command is non-canonical or invalid")
	}
	rowDigest, err := rowDigestV1("WorkspaceReadExecutionCommandV1", fact)
	if err != nil ||
		commandID != fact.Ref.ID || commandRevision != int64(fact.Ref.Revision) || commandDigest != string(fact.Ref.Digest) ||
		requestKeyDigest != string(fact.Source.RequestKey.Digest) || requestID != fact.Source.RequestKey.RequestID ||
		requestRevision != int64(fact.Source.RequestKey.RequestRevision) || requestDigest != string(fact.Source.RequestKey.RequestDigest) ||
		actionDigest != string(fact.Source.RequestKey.ActionCoordinateDigest) || scopeDigest != string(fact.Source.RequestKey.ScopeDigest) ||
		claimID != fact.Source.ClaimRef.ID || claimRevision != int64(fact.Source.ClaimRef.Revision) || claimDigest != string(fact.Source.ClaimRef.Digest) ||
		stateID != fact.Source.ExecutionStateRef.ID || stateRevision != int64(fact.Source.ExecutionStateRef.Revision) || stateDigest != string(fact.Source.ExecutionStateRef.Digest) ||
		inputDigest != string(fact.Source.ExecutionInputDigest) || toolAttemptID != fact.Source.ToolExecutionAttemptID ||
		bindingID != fact.Source.BindingCurrent.ID || bindingRevision != int64(fact.Source.BindingCurrent.Revision) || bindingDigest != string(fact.Source.BindingCurrent.Digest) ||
		candidateID != fact.Source.Candidate.ID || candidateRevision != int64(fact.Source.Candidate.Revision) || candidateDigest != string(fact.Source.Candidate.Digest) ||
		closureDigest != string(fact.Source.CandidateClosureDigest) ||
		inputContractID != fact.Source.InputContractCurrent.ID || inputContractRevision != int64(fact.Source.InputContractCurrent.Revision) || inputContractDigest != string(fact.Source.InputContractCurrent.Digest) ||
		toolID != fact.Source.Tool.ID || toolRevision != int64(fact.Source.Tool.Revision) || toolDigest != string(fact.Source.Tool.Digest) ||
		toolCurrentKind != string(fact.Source.ToolCurrent.Kind) || toolCurrentID != fact.Source.ToolCurrent.ID ||
		toolCurrentRevision != int64(fact.Source.ToolCurrent.Revision) || toolCurrentDigest != string(fact.Source.ToolCurrent.Digest) ||
		operationDigest != string(fact.OperationDigest) || preparedID != fact.Prepared.ID ||
		preparedRevision != int64(fact.Prepared.Revision) || preparedDigest != string(fact.Prepared.Digest) ||
		runtimeAttemptDigest != string(fact.RuntimeAttemptDigest) ||
		payloadSchemaKey != fact.PayloadSchema.Key() || payloadDigest != string(fact.PayloadDigest) ||
		payloadRevision != int64(fact.PayloadRevision) || createdUnixNano != fact.CreatedUnixNano ||
		notAfterUnixNano != fact.NotAfterUnixNano || storedRowDigest != string(rowDigest) {
		return toolcontract.WorkspaceReadExecutionCommandV1{}, conflictV1("stored workspace.read execution command row columns drifted")
	}
	return toolcontract.CloneWorkspaceReadExecutionCommandV1(fact), nil
}

var _ toolcontract.WorkspaceReadExecutionCommandExactReaderV1 = (*WorkspaceReadExecutionCommandStoreV1)(nil)
var _ toolcontract.WorkspaceReadExecutionCommandAttemptReaderV1 = (*WorkspaceReadExecutionCommandStoreV1)(nil)
var _ ownercommand.RepositoryV1 = (*WorkspaceReadExecutionCommandStoreV1)(nil)
