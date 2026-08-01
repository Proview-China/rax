package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

const (
	workspaceReadQualificationHistoryTableDDLV20 = `CREATE TABLE workspace_read_execution_qualification_history_v2 (
		qualification_id TEXT NOT NULL, revision INTEGER NOT NULL, digest TEXT NOT NULL,
		expires_unix_nano INTEGER NOT NULL,
		origin_attempt_id TEXT NOT NULL, origin_attempt_revision INTEGER NOT NULL, origin_attempt_digest TEXT NOT NULL,
		reservation_id TEXT NOT NULL, reservation_revision INTEGER NOT NULL, reservation_digest TEXT NOT NULL,
		admission_id TEXT NOT NULL, admission_revision INTEGER NOT NULL, admission_digest TEXT NOT NULL,
		runtime_admission_id TEXT NOT NULL, runtime_admission_revision INTEGER NOT NULL, runtime_admission_digest TEXT NOT NULL,
		runtime_attempt_digest TEXT NOT NULL, admission_attempt_binding_digest TEXT NOT NULL,
		authorization_digest TEXT NOT NULL,
		association_id TEXT NOT NULL, association_revision INTEGER NOT NULL, association_digest TEXT NOT NULL,
		command_id TEXT NOT NULL, command_revision INTEGER NOT NULL, command_digest TEXT NOT NULL,
		publication_id TEXT NOT NULL, publication_revision INTEGER NOT NULL, publication_digest TEXT NOT NULL,
		owner_current_id TEXT NOT NULL, owner_current_revision INTEGER NOT NULL, owner_current_digest TEXT NOT NULL,
		workspace_view_id TEXT NOT NULL, workspace_view_revision INTEGER NOT NULL, workspace_view_digest TEXT NOT NULL,
		workspace_lease_digest TEXT NOT NULL,
		current_query_digest TEXT NOT NULL, expected_runtime_current_digest TEXT NOT NULL,
		actual_request_digest TEXT NOT NULL, payload_digest TEXT NOT NULL,
		s1_checked_unix_nano INTEGER NOT NULL, body BLOB NOT NULL, row_digest TEXT NOT NULL,
		PRIMARY KEY(qualification_id,revision,digest))`
	workspaceReadQualificationIdentityIndexDDLV20 = `CREATE UNIQUE INDEX workspace_read_execution_qualification_identity_v2
		ON workspace_read_execution_qualification_history_v2(qualification_id,revision)`
	workspaceReadQualificationOriginIndexDDLV20 = `CREATE UNIQUE INDEX workspace_read_execution_qualification_origin_v2
		ON workspace_read_execution_qualification_history_v2(origin_attempt_id,origin_attempt_revision,origin_attempt_digest)`
	workspaceReadQualificationRuntimeIndexDDLV20 = `CREATE UNIQUE INDEX workspace_read_execution_qualification_runtime_v2
		ON workspace_read_execution_qualification_history_v2(runtime_attempt_digest)`
	workspaceReadQualificationQueryIndexDDLV20 = `CREATE UNIQUE INDEX workspace_read_execution_qualification_query_v2
		ON workspace_read_execution_qualification_history_v2(current_query_digest)`
	workspaceReadQualificationRequestIndexDDLV20 = `CREATE UNIQUE INDEX workspace_read_execution_qualification_request_v2
		ON workspace_read_execution_qualification_history_v2(actual_request_digest)`

	workspaceReadTerminalHistoryTableDDLV20 = `CREATE TABLE workspace_read_terminal_history_v2 (
		terminal_id TEXT NOT NULL, revision INTEGER NOT NULL, digest TEXT NOT NULL,
		qualification_id TEXT NOT NULL, qualification_revision INTEGER NOT NULL,
		qualification_digest TEXT NOT NULL, qualification_expires_unix_nano INTEGER NOT NULL,
		origin_attempt_id TEXT NOT NULL, origin_attempt_revision INTEGER NOT NULL, origin_attempt_digest TEXT NOT NULL,
		runtime_attempt_digest TEXT NOT NULL, actual_request_digest TEXT NOT NULL,
		journal_attempt_id TEXT NOT NULL, journal_request_digest TEXT NOT NULL, journal_payload_digest TEXT NOT NULL,
		journal_phase TEXT NOT NULL, journal_state TEXT NOT NULL, journal_revision INTEGER NOT NULL,
		journal_recorded_unix_nano INTEGER NOT NULL, journal_record_digest TEXT NOT NULL,
		outcome TEXT NOT NULL,
		observation_id TEXT NOT NULL, observation_revision INTEGER NOT NULL, observation_digest TEXT NOT NULL,
		provider_receipt_id TEXT NOT NULL, provider_receipt_revision INTEGER NOT NULL,
		provider_receipt_digest TEXT NOT NULL, s2_proof_digest TEXT NOT NULL,
		s2_checked_unix_nano INTEGER NOT NULL,
		indeterminate_boundary TEXT NOT NULL, indeterminate_stage TEXT NOT NULL,
		indeterminate_error_class TEXT NOT NULL, indeterminate_error_digest TEXT NOT NULL,
		indeterminate_evidence_digest TEXT NOT NULL, indeterminate_fact_digest TEXT NOT NULL,
		outcome_checked_unix_nano INTEGER NOT NULL, recorded_unix_nano INTEGER NOT NULL,
		body BLOB NOT NULL, row_digest TEXT NOT NULL,
		PRIMARY KEY(terminal_id,revision,digest),
		FOREIGN KEY(qualification_id,qualification_revision,qualification_digest)
			REFERENCES workspace_read_execution_qualification_history_v2(qualification_id,revision,digest),
		CHECK(outcome IN ('observed','indeterminate')),
		CHECK(journal_state IN ('started','completed')),
		CHECK((outcome='observed' AND journal_state='completed' AND observation_id<>'' AND observation_revision>0
			AND observation_digest<>'' AND provider_receipt_id<>'' AND provider_receipt_revision>0
			AND provider_receipt_digest<>'' AND s2_proof_digest<>'' AND s2_checked_unix_nano>0
			AND indeterminate_boundary='' AND indeterminate_stage=''
			AND indeterminate_error_class='' AND indeterminate_error_digest=''
			AND indeterminate_evidence_digest='' AND indeterminate_fact_digest='')
			OR (outcome='indeterminate' AND observation_id='' AND observation_revision=0
			AND observation_digest='' AND provider_receipt_id='' AND provider_receipt_revision=0
			AND provider_receipt_digest='' AND s2_proof_digest='' AND s2_checked_unix_nano=0
			AND indeterminate_boundary<>'' AND indeterminate_stage<>''
			AND indeterminate_error_class<>'' AND indeterminate_error_digest<>''
			AND indeterminate_evidence_digest<>'' AND indeterminate_fact_digest<>'')))`
	workspaceReadTerminalIdentityIndexDDLV20 = `CREATE UNIQUE INDEX workspace_read_terminal_identity_v2
		ON workspace_read_terminal_history_v2(terminal_id,revision)`
	workspaceReadTerminalQualificationIndexDDLV20 = `CREATE UNIQUE INDEX workspace_read_terminal_qualification_v2
		ON workspace_read_terminal_history_v2(qualification_id,qualification_revision,qualification_digest)`
	workspaceReadTerminalOriginIndexDDLV20 = `CREATE UNIQUE INDEX workspace_read_terminal_origin_v2
		ON workspace_read_terminal_history_v2(origin_attempt_id,origin_attempt_revision,origin_attempt_digest)`
	workspaceReadTerminalJournalIndexDDLV20 = `CREATE UNIQUE INDEX workspace_read_terminal_journal_v2
		ON workspace_read_terminal_history_v2(journal_record_digest)`

	workspaceReadPostActualLedgerTableDDLV20 = `CREATE TABLE workspace_read_post_actual_schema_v20 (
		singleton INTEGER NOT NULL PRIMARY KEY CHECK(singleton=1),
		contract_version TEXT NOT NULL, namespace_digest TEXT NOT NULL)`
)

var workspaceReadPostActualSchemaStatementsV20 = []string{
	workspaceReadQualificationHistoryTableDDLV20,
	workspaceReadQualificationIdentityIndexDDLV20,
	workspaceReadQualificationOriginIndexDDLV20,
	workspaceReadQualificationRuntimeIndexDDLV20,
	workspaceReadQualificationQueryIndexDDLV20,
	workspaceReadQualificationRequestIndexDDLV20,
	workspaceReadTerminalHistoryTableDDLV20,
	workspaceReadTerminalIdentityIndexDDLV20,
	workspaceReadTerminalQualificationIndexDDLV20,
	workspaceReadTerminalOriginIndexDDLV20,
	workspaceReadTerminalJournalIndexDDLV20,
	workspaceReadPostActualLedgerTableDDLV20,
}

func installWorkspaceReadPostActualSchemaV20(ctx context.Context, tx *sql.Tx) error {
	objects, err := workspaceReadPostActualNamespaceV20(ctx, tx)
	if err != nil {
		return err
	}
	if len(objects) != 0 {
		return fmt.Errorf("%w: workspace read post-actual v20 namespace is partial without its ledger: %v", ports.ErrConflict, objects)
	}
	for _, statement := range workspaceReadPostActualSchemaStatementsV20 {
		if _, err = tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("create workspace read post-actual v20 schema: %w", err)
		}
	}
	if err = probeWorkspaceReadPostActualSchemaV20(ctx, tx); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx,
		`INSERT INTO workspace_read_post_actual_schema_v20(singleton,contract_version,namespace_digest)
		 VALUES(1,?,?)`,
		"praxis.sandbox/workspace-read-post-actual/v2",
		workspaceReadPostActualSchemaDigestV20(),
	); err != nil {
		return fmt.Errorf("write workspace read post-actual v20 schema ledger: %w", err)
	}
	return verifyWorkspaceReadPostActualSchemaV20(ctx, tx)
}

func verifyWorkspaceReadPostActualSchemaV20(ctx context.Context, tx *sql.Tx) error {
	if err := verifyWorkspaceReadPostActualObjectsV20(ctx, tx); err != nil {
		return err
	}
	var version, digest string
	if err := tx.QueryRowContext(ctx,
		`SELECT contract_version,namespace_digest FROM workspace_read_post_actual_schema_v20 WHERE singleton=1`,
	).Scan(&version, &digest); err != nil {
		return fmt.Errorf("inspect workspace read post-actual v20 schema ledger: %w", err)
	}
	if version != "praxis.sandbox/workspace-read-post-actual/v2" || digest != workspaceReadPostActualSchemaDigestV20() {
		return errors.New("workspace read post-actual v20 schema ledger drifted")
	}
	var rows int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM workspace_read_post_actual_schema_v20`).Scan(&rows); err != nil {
		return fmt.Errorf("count workspace read post-actual v20 schema ledger: %w", err)
	}
	if rows != 1 {
		return errors.New("workspace read post-actual v20 schema ledger is not singleton")
	}
	return nil
}

func workspaceReadPostActualNamespaceV20(ctx context.Context, tx *sql.Tx) ([]string, error) {
	rows, err := tx.QueryContext(ctx, `SELECT type,name,tbl_name FROM sqlite_master
		WHERE name LIKE 'workspace_read_execution_qualification_%'
		   OR name LIKE 'workspace_read_terminal_%'
		   OR name LIKE 'workspace_read_post_actual_%'
		   OR tbl_name LIKE 'workspace_read_execution_qualification_%'
		   OR tbl_name LIKE 'workspace_read_terminal_%'
		   OR tbl_name LIKE 'workspace_read_post_actual_%'
		ORDER BY type,name`)
	if err != nil {
		return nil, fmt.Errorf("inspect workspace read post-actual v20 namespace: %w", err)
	}
	defer rows.Close()
	var objects []string
	for rows.Next() {
		var kind, name, table string
		if err = rows.Scan(&kind, &name, &table); err != nil {
			return nil, fmt.Errorf("decode workspace read post-actual v20 namespace: %w", err)
		}
		objects = append(objects, kind+":"+name+":"+table)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("inspect workspace read post-actual v20 namespace rows: %w", err)
	}
	if err = rows.Close(); err != nil {
		return nil, fmt.Errorf("close workspace read post-actual v20 namespace rows: %w", err)
	}
	return objects, nil
}

func workspaceReadPostActualSchemaDigestV20() string {
	sum := sha256.Sum256([]byte(strings.Join(workspaceReadPostActualSchemaStatementsV20, "\n")))
	return hex.EncodeToString(sum[:])
}

func verifyWorkspaceReadPostActualObjectsV20(ctx context.Context, tx *sql.Tx) error {
	expected := map[string]struct{}{
		"table:workspace_read_execution_qualification_history_v2:workspace_read_execution_qualification_history_v2":                    {},
		"table:workspace_read_terminal_history_v2:workspace_read_terminal_history_v2":                                                  {},
		"table:workspace_read_post_actual_schema_v20:workspace_read_post_actual_schema_v20":                                            {},
		"index:sqlite_autoindex_workspace_read_execution_qualification_history_v2_1:workspace_read_execution_qualification_history_v2": {},
		"index:workspace_read_execution_qualification_identity_v2:workspace_read_execution_qualification_history_v2":                   {},
		"index:workspace_read_execution_qualification_origin_v2:workspace_read_execution_qualification_history_v2":                     {},
		"index:workspace_read_execution_qualification_runtime_v2:workspace_read_execution_qualification_history_v2":                    {},
		"index:workspace_read_execution_qualification_query_v2:workspace_read_execution_qualification_history_v2":                      {},
		"index:workspace_read_execution_qualification_request_v2:workspace_read_execution_qualification_history_v2":                    {},
		"index:sqlite_autoindex_workspace_read_terminal_history_v2_1:workspace_read_terminal_history_v2":                               {},
		"index:workspace_read_terminal_identity_v2:workspace_read_terminal_history_v2":                                                 {},
		"index:workspace_read_terminal_qualification_v2:workspace_read_terminal_history_v2":                                            {},
		"index:workspace_read_terminal_origin_v2:workspace_read_terminal_history_v2":                                                   {},
		"index:workspace_read_terminal_journal_v2:workspace_read_terminal_history_v2":                                                  {},
	}
	objects, err := workspaceReadPostActualNamespaceV20(ctx, tx)
	if err != nil {
		return err
	}
	found := make(map[string]struct{}, len(objects))
	for _, object := range objects {
		if _, ok := expected[object]; !ok {
			return fmt.Errorf("workspace read post-actual v20 namespace contains unexpected object %q", object)
		}
		found[object] = struct{}{}
	}
	if len(found) != len(expected) {
		return errors.New("workspace read post-actual v20 namespace is incomplete")
	}
	return verifyWorkspaceReadPostActualPhysicalV20(ctx, tx)
}

func verifyWorkspaceReadPostActualPhysicalV20(ctx context.Context, tx *sql.Tx) error {
	qualificationColumns := []workspaceReadSchemaColumnV19{
		{name: "qualification_id", kind: "TEXT", notNull: 1, primaryKey: 1},
		{name: "revision", kind: "INTEGER", notNull: 1, primaryKey: 2},
		{name: "digest", kind: "TEXT", notNull: 1, primaryKey: 3},
		{name: "expires_unix_nano", kind: "INTEGER", notNull: 1},
		{name: "origin_attempt_id", kind: "TEXT", notNull: 1}, {name: "origin_attempt_revision", kind: "INTEGER", notNull: 1}, {name: "origin_attempt_digest", kind: "TEXT", notNull: 1},
		{name: "reservation_id", kind: "TEXT", notNull: 1}, {name: "reservation_revision", kind: "INTEGER", notNull: 1}, {name: "reservation_digest", kind: "TEXT", notNull: 1},
		{name: "admission_id", kind: "TEXT", notNull: 1}, {name: "admission_revision", kind: "INTEGER", notNull: 1}, {name: "admission_digest", kind: "TEXT", notNull: 1},
		{name: "runtime_admission_id", kind: "TEXT", notNull: 1}, {name: "runtime_admission_revision", kind: "INTEGER", notNull: 1}, {name: "runtime_admission_digest", kind: "TEXT", notNull: 1},
		{name: "runtime_attempt_digest", kind: "TEXT", notNull: 1}, {name: "admission_attempt_binding_digest", kind: "TEXT", notNull: 1},
		{name: "authorization_digest", kind: "TEXT", notNull: 1},
		{name: "association_id", kind: "TEXT", notNull: 1}, {name: "association_revision", kind: "INTEGER", notNull: 1}, {name: "association_digest", kind: "TEXT", notNull: 1},
		{name: "command_id", kind: "TEXT", notNull: 1}, {name: "command_revision", kind: "INTEGER", notNull: 1}, {name: "command_digest", kind: "TEXT", notNull: 1},
		{name: "publication_id", kind: "TEXT", notNull: 1}, {name: "publication_revision", kind: "INTEGER", notNull: 1}, {name: "publication_digest", kind: "TEXT", notNull: 1},
		{name: "owner_current_id", kind: "TEXT", notNull: 1}, {name: "owner_current_revision", kind: "INTEGER", notNull: 1}, {name: "owner_current_digest", kind: "TEXT", notNull: 1},
		{name: "workspace_view_id", kind: "TEXT", notNull: 1}, {name: "workspace_view_revision", kind: "INTEGER", notNull: 1}, {name: "workspace_view_digest", kind: "TEXT", notNull: 1},
		{name: "workspace_lease_digest", kind: "TEXT", notNull: 1},
		{name: "current_query_digest", kind: "TEXT", notNull: 1}, {name: "expected_runtime_current_digest", kind: "TEXT", notNull: 1},
		{name: "actual_request_digest", kind: "TEXT", notNull: 1}, {name: "payload_digest", kind: "TEXT", notNull: 1},
		{name: "s1_checked_unix_nano", kind: "INTEGER", notNull: 1}, {name: "body", kind: "BLOB", notNull: 1}, {name: "row_digest", kind: "TEXT", notNull: 1},
	}
	terminalColumns := []workspaceReadSchemaColumnV19{
		{name: "terminal_id", kind: "TEXT", notNull: 1, primaryKey: 1}, {name: "revision", kind: "INTEGER", notNull: 1, primaryKey: 2}, {name: "digest", kind: "TEXT", notNull: 1, primaryKey: 3},
		{name: "qualification_id", kind: "TEXT", notNull: 1}, {name: "qualification_revision", kind: "INTEGER", notNull: 1}, {name: "qualification_digest", kind: "TEXT", notNull: 1}, {name: "qualification_expires_unix_nano", kind: "INTEGER", notNull: 1},
		{name: "origin_attempt_id", kind: "TEXT", notNull: 1}, {name: "origin_attempt_revision", kind: "INTEGER", notNull: 1}, {name: "origin_attempt_digest", kind: "TEXT", notNull: 1},
		{name: "runtime_attempt_digest", kind: "TEXT", notNull: 1}, {name: "actual_request_digest", kind: "TEXT", notNull: 1},
		{name: "journal_attempt_id", kind: "TEXT", notNull: 1}, {name: "journal_request_digest", kind: "TEXT", notNull: 1}, {name: "journal_payload_digest", kind: "TEXT", notNull: 1},
		{name: "journal_phase", kind: "TEXT", notNull: 1}, {name: "journal_state", kind: "TEXT", notNull: 1}, {name: "journal_revision", kind: "INTEGER", notNull: 1},
		{name: "journal_recorded_unix_nano", kind: "INTEGER", notNull: 1}, {name: "journal_record_digest", kind: "TEXT", notNull: 1},
		{name: "outcome", kind: "TEXT", notNull: 1},
		{name: "observation_id", kind: "TEXT", notNull: 1}, {name: "observation_revision", kind: "INTEGER", notNull: 1}, {name: "observation_digest", kind: "TEXT", notNull: 1},
		{name: "provider_receipt_id", kind: "TEXT", notNull: 1}, {name: "provider_receipt_revision", kind: "INTEGER", notNull: 1}, {name: "provider_receipt_digest", kind: "TEXT", notNull: 1}, {name: "s2_proof_digest", kind: "TEXT", notNull: 1},
		{name: "s2_checked_unix_nano", kind: "INTEGER", notNull: 1},
		{name: "indeterminate_boundary", kind: "TEXT", notNull: 1}, {name: "indeterminate_stage", kind: "TEXT", notNull: 1}, {name: "indeterminate_error_class", kind: "TEXT", notNull: 1}, {name: "indeterminate_error_digest", kind: "TEXT", notNull: 1}, {name: "indeterminate_evidence_digest", kind: "TEXT", notNull: 1}, {name: "indeterminate_fact_digest", kind: "TEXT", notNull: 1},
		{name: "outcome_checked_unix_nano", kind: "INTEGER", notNull: 1}, {name: "recorded_unix_nano", kind: "INTEGER", notNull: 1},
		{name: "body", kind: "BLOB", notNull: 1}, {name: "row_digest", kind: "TEXT", notNull: 1},
	}
	ledgerColumns := []workspaceReadSchemaColumnV19{{name: "singleton", kind: "INTEGER", notNull: 1, primaryKey: 1}, {name: "contract_version", kind: "TEXT", notNull: 1}, {name: "namespace_digest", kind: "TEXT", notNull: 1}}
	for table, columns := range map[string][]workspaceReadSchemaColumnV19{
		"workspace_read_execution_qualification_history_v2": qualificationColumns,
		"workspace_read_terminal_history_v2":                terminalColumns,
		"workspace_read_post_actual_schema_v20":             ledgerColumns,
	} {
		if err := verifyWorkspaceReadSchemaColumnsV19(ctx, tx, table, columns); err != nil {
			return err
		}
	}
	if err := verifyWorkspaceReadPostActualIndexesV20(ctx, tx); err != nil {
		return err
	}
	return verifyWorkspaceReadPostActualDDLV20(ctx, tx)
}

func verifyWorkspaceReadPostActualIndexesV20(ctx context.Context, tx *sql.Tx) error {
	qualification := map[string]workspaceReadIndexExpectationV19{
		"workspace_read_execution_qualification_request_v2":                    {sequence: 0, origin: "c", columns: []string{"actual_request_digest"}, cids: []int{37}},
		"workspace_read_execution_qualification_query_v2":                      {sequence: 1, origin: "c", columns: []string{"current_query_digest"}, cids: []int{35}},
		"workspace_read_execution_qualification_runtime_v2":                    {sequence: 2, origin: "c", columns: []string{"runtime_attempt_digest"}, cids: []int{16}},
		"workspace_read_execution_qualification_origin_v2":                     {sequence: 3, origin: "c", columns: []string{"origin_attempt_id", "origin_attempt_revision", "origin_attempt_digest"}, cids: []int{4, 5, 6}},
		"workspace_read_execution_qualification_identity_v2":                   {sequence: 4, origin: "c", columns: []string{"qualification_id", "revision"}, cids: []int{0, 1}},
		"sqlite_autoindex_workspace_read_execution_qualification_history_v2_1": {sequence: 5, origin: "pk", columns: []string{"qualification_id", "revision", "digest"}, cids: []int{0, 1, 2}},
	}
	terminal := map[string]workspaceReadIndexExpectationV19{
		"workspace_read_terminal_journal_v2":                    {sequence: 0, origin: "c", columns: []string{"journal_record_digest"}, cids: []int{19}},
		"workspace_read_terminal_origin_v2":                     {sequence: 1, origin: "c", columns: []string{"origin_attempt_id", "origin_attempt_revision", "origin_attempt_digest"}, cids: []int{7, 8, 9}},
		"workspace_read_terminal_qualification_v2":              {sequence: 2, origin: "c", columns: []string{"qualification_id", "qualification_revision", "qualification_digest"}, cids: []int{3, 4, 5}},
		"workspace_read_terminal_identity_v2":                   {sequence: 3, origin: "c", columns: []string{"terminal_id", "revision"}, cids: []int{0, 1}},
		"sqlite_autoindex_workspace_read_terminal_history_v2_1": {sequence: 4, origin: "pk", columns: []string{"terminal_id", "revision", "digest"}, cids: []int{0, 1, 2}},
	}
	if err := verifyWorkspaceReadIndexSetV19(ctx, tx, "workspace_read_execution_qualification_history_v2", qualification); err != nil {
		return err
	}
	if err := verifyWorkspaceReadIndexSetV19(ctx, tx, "workspace_read_terminal_history_v2", terminal); err != nil {
		return err
	}
	return verifyWorkspaceReadIndexSetV19(ctx, tx, "workspace_read_post_actual_schema_v20", map[string]workspaceReadIndexExpectationV19{})
}

func verifyWorkspaceReadPostActualDDLV20(ctx context.Context, tx *sql.Tx) error {
	expected := map[string]string{
		"workspace_read_execution_qualification_history_v2":  workspaceReadQualificationHistoryTableDDLV20,
		"workspace_read_execution_qualification_identity_v2": workspaceReadQualificationIdentityIndexDDLV20,
		"workspace_read_execution_qualification_origin_v2":   workspaceReadQualificationOriginIndexDDLV20,
		"workspace_read_execution_qualification_runtime_v2":  workspaceReadQualificationRuntimeIndexDDLV20,
		"workspace_read_execution_qualification_query_v2":    workspaceReadQualificationQueryIndexDDLV20,
		"workspace_read_execution_qualification_request_v2":  workspaceReadQualificationRequestIndexDDLV20,
		"workspace_read_terminal_history_v2":                 workspaceReadTerminalHistoryTableDDLV20,
		"workspace_read_terminal_identity_v2":                workspaceReadTerminalIdentityIndexDDLV20,
		"workspace_read_terminal_qualification_v2":           workspaceReadTerminalQualificationIndexDDLV20,
		"workspace_read_terminal_origin_v2":                  workspaceReadTerminalOriginIndexDDLV20,
		"workspace_read_terminal_journal_v2":                 workspaceReadTerminalJournalIndexDDLV20,
		"workspace_read_post_actual_schema_v20":              workspaceReadPostActualLedgerTableDDLV20,
	}
	for name, want := range expected {
		var actual sql.NullString
		if err := tx.QueryRowContext(ctx, `SELECT sql FROM sqlite_master WHERE name=?`, name).Scan(&actual); err != nil {
			return fmt.Errorf("inspect %s DDL: %w", name, err)
		}
		if !actual.Valid {
			return fmt.Errorf("%s DDL is missing", name)
		}
		gotTokens, err := canonicalSQLiteDDLTokensV2(actual.String)
		if err != nil {
			return err
		}
		wantTokens, err := canonicalSQLiteDDLTokensV2(want)
		if err != nil {
			return err
		}
		if !equalSQLiteDDLTokensV2(gotTokens, wantTokens) {
			return fmt.Errorf("%s DDL semantics drifted", name)
		}
	}
	return nil
}

func probeWorkspaceReadPostActualSchemaV20(ctx context.Context, tx *sql.Tx) error {
	const savepoint = "workspace_read_post_actual_v20_probe"
	if _, err := tx.ExecContext(ctx, "SAVEPOINT "+savepoint); err != nil {
		return fmt.Errorf("begin workspace read post-actual v20 probe: %w", err)
	}
	rollback := func() {
		_, _ = tx.ExecContext(ctx, "ROLLBACK TO "+savepoint)
		_, _ = tx.ExecContext(ctx, "RELEASE "+savepoint)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO workspace_read_execution_qualification_history_v2(
		qualification_id,revision,digest,expires_unix_nano,
		origin_attempt_id,origin_attempt_revision,origin_attempt_digest,
		reservation_id,reservation_revision,reservation_digest,
		admission_id,admission_revision,admission_digest,
		runtime_admission_id,runtime_admission_revision,runtime_admission_digest,
		runtime_attempt_digest,admission_attempt_binding_digest,authorization_digest,
		association_id,association_revision,association_digest,
		command_id,command_revision,command_digest,
		publication_id,publication_revision,publication_digest,
		owner_current_id,owner_current_revision,owner_current_digest,
		workspace_view_id,workspace_view_revision,workspace_view_digest,
		workspace_lease_digest,
		current_query_digest,expected_runtime_current_digest,actual_request_digest,payload_digest,
		s1_checked_unix_nano,body,row_digest)
		VALUES('probe-q',1,'probe-q-digest',2,
		'probe-origin',1,'probe-origin-digest','probe-reservation',1,'probe-reservation-digest',
		'probe-admission',1,'probe-admission-digest','probe-runtime-admission',1,'probe-runtime-admission-digest',
		'probe-runtime-attempt','probe-binding','probe-authorization','probe-association',1,'probe-association-digest',
		'probe-command',1,'probe-command-digest','probe-publication',1,'probe-publication-digest',
		'probe-current',1,'probe-current-digest','probe-view',1,'probe-view-digest','probe-lease-digest',
		'probe-query','probe-runtime-current','probe-request','probe-payload',1,x'00','probe-row')`); err != nil {
		rollback()
		return fmt.Errorf("write workspace read qualification v20 probe: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO workspace_read_terminal_history_v2(
		terminal_id,revision,digest,qualification_id,qualification_revision,qualification_digest,
		qualification_expires_unix_nano,origin_attempt_id,origin_attempt_revision,origin_attempt_digest,
		runtime_attempt_digest,actual_request_digest,
		journal_attempt_id,journal_request_digest,journal_payload_digest,journal_phase,journal_state,
		journal_revision,journal_recorded_unix_nano,journal_record_digest,outcome,
		observation_id,observation_revision,observation_digest,
		provider_receipt_id,provider_receipt_revision,provider_receipt_digest,s2_proof_digest,
		s2_checked_unix_nano,indeterminate_boundary,indeterminate_stage,indeterminate_error_class,
		indeterminate_error_digest,indeterminate_evidence_digest,indeterminate_fact_digest,
		outcome_checked_unix_nano,recorded_unix_nano,body,row_digest)
		VALUES('probe-terminal',1,'probe-terminal-digest','probe-q',1,'probe-q-digest',2,
		'probe-origin',1,'probe-origin-digest','probe-runtime-attempt','probe-request',
		'probe-origin','probe-request','probe-payload','execute','started',1,2,'probe-journal','indeterminate',
		'',0,'','',0,'','',0,'post_actual','physical_started','actual_point_unknown',
		'probe-error','probe-evidence','probe-indeterminate-fact',2,2,x'00','probe-terminal-row')`); err != nil {
		rollback()
		return fmt.Errorf("write workspace read terminal v20 probe: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "ROLLBACK TO "+savepoint); err != nil {
		rollback()
		return fmt.Errorf("rollback workspace read post-actual v20 probe: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "RELEASE "+savepoint); err != nil {
		return fmt.Errorf("release workspace read post-actual v20 probe: %w", err)
	}
	return nil
}
