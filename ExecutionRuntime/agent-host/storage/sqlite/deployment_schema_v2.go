package sqlite

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
)

const (
	sqliteConstraintCheckV2      = 275
	sqliteConstraintForeignKeyV2 = 787
)

type deploymentSchemaColumnV2 struct {
	Name       string
	Type       string
	NotNull    int
	PrimaryKey int
}

type deploymentSchemaForeignKeyV2 struct {
	Sequence int
	Table    string
	From     string
	To       string
	OnUpdate string
	OnDelete string
	Match    string
}

type deploymentSchemaTableV2 struct {
	Name          string
	Strict        int
	Columns       []deploymentSchemaColumnV2
	PrimaryIndex  []string
	UniqueIndexes [][]string
	ForeignKeys   []deploymentSchemaForeignKeyV2
	ExpectedDDL   string
}

func verifyDeploymentCurrentSchemaV2(ctx context.Context, tx *sql.Tx) error {
	expected := []deploymentSchemaTableV2{
		{
			Name: "agent_host_schema",
			Columns: []deploymentSchemaColumnV2{
				{Name: "version", Type: "INTEGER", PrimaryKey: 1},
				{Name: "digest", Type: "TEXT", NotNull: 1},
				{Name: "applied_unix_nano", Type: "INTEGER", NotNull: 1},
			},
		},
		{
			Name:   "agent_host_deployment_current_history_v2",
			Strict: 1,
			Columns: []deploymentSchemaColumnV2{
				{Name: "host_id", Type: "TEXT", NotNull: 1, PrimaryKey: 1},
				{Name: "deployment_id", Type: "TEXT", NotNull: 1, PrimaryKey: 2},
				{Name: "revision", Type: "INTEGER", NotNull: 1, PrimaryKey: 3},
				{Name: "digest", Type: "TEXT", NotNull: 1},
				{Name: "bootstrap_digest", Type: "TEXT", NotNull: 1},
				{Name: "selection_id", Type: "TEXT", NotNull: 1},
				{Name: "selection_revision", Type: "INTEGER", NotNull: 1},
				{Name: "selection_digest", Type: "TEXT", NotNull: 1},
				{Name: "selection_expires_unix_nano", Type: "INTEGER", NotNull: 1},
				{Name: "checked_unix_nano", Type: "INTEGER", NotNull: 1},
				{Name: "expires_unix_nano", Type: "INTEGER", NotNull: 1},
				{Name: "row_digest", Type: "TEXT", NotNull: 1},
				{Name: "canonical_json", Type: "BLOB", NotNull: 1},
			},
			UniqueIndexes: [][]string{
				{"host_id", "deployment_id", "revision", "digest"},
				{"host_id", "deployment_id", "revision", "digest", "selection_id", "selection_revision", "selection_digest", "selection_expires_unix_nano"},
			},
			PrimaryIndex: []string{"host_id", "deployment_id", "revision"},
			ExpectedDDL: `CREATE TABLE agent_host_deployment_current_history_v2 (
			  host_id TEXT NOT NULL,
			  deployment_id TEXT NOT NULL,
			  revision INTEGER NOT NULL CHECK(revision > 0),
			  digest TEXT NOT NULL,
			  bootstrap_digest TEXT NOT NULL,
			  selection_id TEXT NOT NULL,
			  selection_revision INTEGER NOT NULL CHECK(selection_revision > 0),
			  selection_digest TEXT NOT NULL,
			  selection_expires_unix_nano INTEGER NOT NULL CHECK(selection_expires_unix_nano > 0),
			  checked_unix_nano INTEGER NOT NULL CHECK(checked_unix_nano > 0),
			  expires_unix_nano INTEGER NOT NULL CHECK(expires_unix_nano > checked_unix_nano),
			  row_digest TEXT NOT NULL,
			  canonical_json BLOB NOT NULL,
			  PRIMARY KEY(host_id, deployment_id, revision),
			  UNIQUE(host_id, deployment_id, revision, digest),
			  UNIQUE(host_id, deployment_id, revision, digest, selection_id, selection_revision, selection_digest, selection_expires_unix_nano)
			) STRICT`,
		},
		{
			Name:   "agent_host_deployment_current_v2",
			Strict: 1,
			Columns: []deploymentSchemaColumnV2{
				{Name: "host_id", Type: "TEXT", NotNull: 1, PrimaryKey: 1},
				{Name: "deployment_id", Type: "TEXT", NotNull: 1, PrimaryKey: 2},
				{Name: "revision", Type: "INTEGER", NotNull: 1},
				{Name: "digest", Type: "TEXT", NotNull: 1},
				{Name: "selection_id", Type: "TEXT", NotNull: 1},
				{Name: "selection_revision", Type: "INTEGER", NotNull: 1},
				{Name: "selection_digest", Type: "TEXT", NotNull: 1},
				{Name: "selection_expires_unix_nano", Type: "INTEGER", NotNull: 1},
				{Name: "row_digest", Type: "TEXT", NotNull: 1},
			},
			ForeignKeys: []deploymentSchemaForeignKeyV2{
				{Sequence: 0, Table: "agent_host_deployment_current_history_v2", From: "host_id", To: "host_id", OnUpdate: "NO ACTION", OnDelete: "NO ACTION", Match: "NONE"},
				{Sequence: 1, Table: "agent_host_deployment_current_history_v2", From: "deployment_id", To: "deployment_id", OnUpdate: "NO ACTION", OnDelete: "NO ACTION", Match: "NONE"},
				{Sequence: 2, Table: "agent_host_deployment_current_history_v2", From: "revision", To: "revision", OnUpdate: "NO ACTION", OnDelete: "NO ACTION", Match: "NONE"},
				{Sequence: 3, Table: "agent_host_deployment_current_history_v2", From: "digest", To: "digest", OnUpdate: "NO ACTION", OnDelete: "NO ACTION", Match: "NONE"},
				{Sequence: 4, Table: "agent_host_deployment_current_history_v2", From: "selection_id", To: "selection_id", OnUpdate: "NO ACTION", OnDelete: "NO ACTION", Match: "NONE"},
				{Sequence: 5, Table: "agent_host_deployment_current_history_v2", From: "selection_revision", To: "selection_revision", OnUpdate: "NO ACTION", OnDelete: "NO ACTION", Match: "NONE"},
				{Sequence: 6, Table: "agent_host_deployment_current_history_v2", From: "selection_digest", To: "selection_digest", OnUpdate: "NO ACTION", OnDelete: "NO ACTION", Match: "NONE"},
				{Sequence: 7, Table: "agent_host_deployment_current_history_v2", From: "selection_expires_unix_nano", To: "selection_expires_unix_nano", OnUpdate: "NO ACTION", OnDelete: "NO ACTION", Match: "NONE"},
			},
			PrimaryIndex: []string{"host_id", "deployment_id"},
			ExpectedDDL: `CREATE TABLE agent_host_deployment_current_v2 (
			  host_id TEXT NOT NULL,
			  deployment_id TEXT NOT NULL,
			  revision INTEGER NOT NULL CHECK(revision > 0),
			  digest TEXT NOT NULL,
			  selection_id TEXT NOT NULL,
			  selection_revision INTEGER NOT NULL CHECK(selection_revision > 0),
			  selection_digest TEXT NOT NULL,
			  selection_expires_unix_nano INTEGER NOT NULL CHECK(selection_expires_unix_nano > 0),
			  row_digest TEXT NOT NULL,
			  PRIMARY KEY(host_id, deployment_id),
			  FOREIGN KEY(host_id, deployment_id, revision, digest, selection_id, selection_revision, selection_digest, selection_expires_unix_nano)
			    REFERENCES agent_host_deployment_current_history_v2(host_id, deployment_id, revision, digest, selection_id, selection_revision, selection_digest, selection_expires_unix_nano)
			) STRICT`,
		},
	}
	for _, table := range expected {
		if err := verifyDeploymentTableV2(ctx, tx, table); err != nil {
			return err
		}
	}
	return verifyDeploymentConstraintBehaviorV2(ctx, tx)
}

func verifyDeploymentTableV2(ctx context.Context, tx *sql.Tx, expected deploymentSchemaTableV2) error {
	if expected.ExpectedDDL != "" {
		if err := verifyDeploymentTableDDLV2(ctx, tx, expected.Name, expected.ExpectedDDL); err != nil {
			return err
		}
	}
	actualColumns, err := readDeploymentColumnsV2(ctx, tx, expected.Name)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(actualColumns, expected.Columns) {
		return contract.NewError(contract.ErrorConflict, "sqlite_schema_physical_drift", "agent-host SQLite table columns, NOT NULL or primary key drifted: "+expected.Name)
	}
	strict, err := readDeploymentStrictV2(ctx, tx, expected.Name)
	if err != nil {
		return err
	}
	if strict != expected.Strict {
		return contract.NewError(contract.ErrorConflict, "sqlite_schema_physical_drift", "agent-host SQLite STRICT mode drifted: "+expected.Name)
	}
	actualPrimary, actualUnique, err := readDeploymentIndexesV2(ctx, tx, expected.Name)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(actualPrimary, expected.PrimaryIndex) {
		return contract.NewError(contract.ErrorConflict, "sqlite_schema_physical_drift", "agent-host SQLite primary index drifted: "+expected.Name)
	}
	sortStringMatrixV2(actualUnique)
	wantUnique := cloneStringMatrixV2(expected.UniqueIndexes)
	sortStringMatrixV2(wantUnique)
	if !reflect.DeepEqual(actualUnique, wantUnique) {
		return contract.NewError(contract.ErrorConflict, "sqlite_schema_physical_drift", "agent-host SQLite UNIQUE indexes drifted: "+expected.Name)
	}
	actualForeignKeys, err := readDeploymentForeignKeysV2(ctx, tx, expected.Name)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(actualForeignKeys, expected.ForeignKeys) {
		return contract.NewError(contract.ErrorConflict, "sqlite_schema_physical_drift", "agent-host SQLite foreign keys drifted: "+expected.Name)
	}
	var triggers int
	if err = tx.QueryRowContext(
		ctx,
		`SELECT COUNT(1) FROM sqlite_master WHERE type='trigger' AND tbl_name=?`,
		expected.Name,
	).Scan(&triggers); err != nil {
		return mapDBError(ctx, err, false)
	}
	if triggers != 0 {
		return contract.NewError(contract.ErrorConflict, "sqlite_schema_physical_drift", "agent-host SQLite unexpected trigger exists: "+expected.Name)
	}
	return nil
}

func verifyDeploymentTableDDLV2(ctx context.Context, tx *sql.Tx, table string, expected string) error {
	var actual string
	if err := tx.QueryRowContext(
		ctx,
		`SELECT sql FROM sqlite_master WHERE type='table' AND name=?`,
		table,
	).Scan(&actual); err != nil {
		return mapDBError(ctx, err, false)
	}
	actualTokens, err := normalizeDeploymentDDLV2(actual)
	if err != nil {
		return err
	}
	expectedTokens, err := normalizeDeploymentDDLV2(expected)
	if err != nil {
		return err
	}
	if actualTokens != expectedTokens {
		return contract.NewError(contract.ErrorConflict, "sqlite_schema_ddl_drift", "agent-host SQLite CREATE TABLE DDL drifted: "+table)
	}
	return nil
}

func normalizeDeploymentDDLV2(value string) (string, error) {
	var tokens []string
	for index := 0; index < len(value); {
		switch {
		case value[index] == ' ' || value[index] == '\t' || value[index] == '\r' || value[index] == '\n':
			index++
		case index+1 < len(value) && value[index:index+2] == "--":
			index += 2
			for index < len(value) && value[index] != '\n' {
				index++
			}
		case index+1 < len(value) && value[index:index+2] == "/*":
			end := strings.Index(value[index+2:], "*/")
			if end < 0 {
				return "", contract.NewError(contract.ErrorConflict, "sqlite_schema_ddl_invalid", "agent-host SQLite DDL has an unterminated comment")
			}
			index += end + 4
		case value[index] == '\'' || value[index] == '"' || value[index] == '`' || value[index] == '[':
			start := index
			open := value[index]
			closeByte := open
			if open == '[' {
				closeByte = ']'
			}
			index++
			closed := false
			for index < len(value) {
				if value[index] == closeByte {
					if open != '[' && index+1 < len(value) && value[index+1] == closeByte {
						index += 2
						continue
					}
					index++
					closed = true
					break
				}
				index++
			}
			if !closed {
				return "", contract.NewError(contract.ErrorConflict, "sqlite_schema_ddl_invalid", "agent-host SQLite DDL has an unterminated quoted token")
			}
			tokens = append(tokens, value[start:index])
		case isDeploymentDDLWordV2(value[index]):
			start := index
			for index < len(value) && isDeploymentDDLWordV2(value[index]) {
				index++
			}
			tokens = append(tokens, strings.ToLower(value[start:index]))
		default:
			tokens = append(tokens, value[index:index+1])
			index++
		}
	}
	return strings.Join(tokens, " "), nil
}

func isDeploymentDDLWordV2(value byte) bool {
	return value >= 'a' && value <= 'z' ||
		value >= 'A' && value <= 'Z' ||
		value >= '0' && value <= '9' ||
		value == '_'
}

func readDeploymentColumnsV2(ctx context.Context, tx *sql.Tx, table string) ([]deploymentSchemaColumnV2, error) {
	rows, err := tx.QueryContext(ctx, fmt.Sprintf("PRAGMA table_xinfo(%q)", table))
	if err != nil {
		return nil, mapDBError(ctx, err, false)
	}
	defer rows.Close()
	var result []deploymentSchemaColumnV2
	for rows.Next() {
		var cid, notNull, primaryKey, hidden int
		var name, columnType string
		var defaultValue sql.NullString
		if err = rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey, &hidden); err != nil {
			return nil, mapDBError(ctx, err, false)
		}
		if cid != len(result) || hidden != 0 || defaultValue.Valid {
			return nil, contract.NewError(contract.ErrorConflict, "sqlite_schema_physical_drift", "agent-host SQLite column order, hidden state or default drifted: "+table)
		}
		result = append(result, deploymentSchemaColumnV2{
			Name:       name,
			Type:       strings.ToUpper(columnType),
			NotNull:    notNull,
			PrimaryKey: primaryKey,
		})
	}
	if err = rows.Err(); err != nil {
		return nil, mapDBError(ctx, err, false)
	}
	return result, nil
}

func readDeploymentStrictV2(ctx context.Context, tx *sql.Tx, table string) (int, error) {
	rows, err := tx.QueryContext(ctx, "PRAGMA table_list")
	if err != nil {
		return 0, mapDBError(ctx, err, false)
	}
	defer rows.Close()
	for rows.Next() {
		var schema, name, objectType string
		var columns, withoutRowID, strict int
		if err = rows.Scan(&schema, &name, &objectType, &columns, &withoutRowID, &strict); err != nil {
			return 0, mapDBError(ctx, err, false)
		}
		if schema == "main" && name == table {
			if objectType != "table" || withoutRowID != 0 {
				return 0, contract.NewError(contract.ErrorConflict, "sqlite_schema_physical_drift", "agent-host SQLite table kind drifted: "+table)
			}
			return strict, nil
		}
	}
	if err = rows.Err(); err != nil {
		return 0, mapDBError(ctx, err, false)
	}
	return 0, contract.NewError(contract.ErrorConflict, "sqlite_schema_physical_drift", "agent-host SQLite table missing: "+table)
}

func readDeploymentIndexesV2(ctx context.Context, tx *sql.Tx, table string) ([]string, [][]string, error) {
	rows, err := tx.QueryContext(ctx, fmt.Sprintf("PRAGMA index_list(%q)", table))
	if err != nil {
		return nil, nil, mapDBError(ctx, err, false)
	}
	type indexRecord struct {
		name   string
		unique int
		origin string
	}
	var indexes []indexRecord
	for rows.Next() {
		var sequence, partial int
		var record indexRecord
		if err = rows.Scan(&sequence, &record.name, &record.unique, &record.origin, &partial); err != nil {
			_ = rows.Close()
			return nil, nil, mapDBError(ctx, err, false)
		}
		if partial != 0 {
			_ = rows.Close()
			return nil, nil, contract.NewError(contract.ErrorConflict, "sqlite_schema_physical_drift", "agent-host SQLite partial index is not allowed: "+table)
		}
		if sequence != len(indexes) {
			_ = rows.Close()
			return nil, nil, contract.NewError(contract.ErrorConflict, "sqlite_schema_physical_drift", "agent-host SQLite index_list sequence drifted: "+table)
		}
		indexes = append(indexes, record)
	}
	if err = rows.Err(); err != nil {
		_ = rows.Close()
		return nil, nil, mapDBError(ctx, err, false)
	}
	if err = rows.Close(); err != nil {
		return nil, nil, mapDBError(ctx, err, false)
	}
	var primary []string
	var result [][]string
	for _, index := range indexes {
		if index.origin != "u" && index.origin != "pk" {
			return nil, nil, contract.NewError(contract.ErrorConflict, "sqlite_schema_physical_drift", "agent-host SQLite unexpected user index exists: "+table)
		}
		if index.unique != 1 {
			return nil, nil, contract.NewError(contract.ErrorConflict, "sqlite_schema_physical_drift", "agent-host SQLite expected only unique automatic indexes: "+table)
		}
		indexRows, queryErr := tx.QueryContext(ctx, fmt.Sprintf("PRAGMA index_xinfo(%q)", index.name))
		if queryErr != nil {
			return nil, nil, mapDBError(ctx, queryErr, false)
		}
		var columns []string
		rowSequence := 0
		for indexRows.Next() {
			var sequence, cid, descending, key int
			var name, collation sql.NullString
			if queryErr = indexRows.Scan(&sequence, &cid, &name, &descending, &collation, &key); queryErr != nil {
				_ = indexRows.Close()
				return nil, nil, mapDBError(ctx, queryErr, false)
			}
			if sequence != rowSequence {
				_ = indexRows.Close()
				return nil, nil, contract.NewError(contract.ErrorConflict, "sqlite_schema_physical_drift", "agent-host SQLite index_xinfo sequence drifted: "+table)
			}
			rowSequence++
			isKey, validationErr := validateDeploymentIndexXInfoRowV2(key, cid, name, descending, collation, table)
			if validationErr != nil {
				_ = indexRows.Close()
				return nil, nil, validationErr
			}
			if isKey {
				columns = append(columns, name.String)
			}
		}
		if queryErr = indexRows.Err(); queryErr != nil {
			_ = indexRows.Close()
			return nil, nil, mapDBError(ctx, queryErr, false)
		}
		if queryErr = indexRows.Close(); queryErr != nil {
			return nil, nil, mapDBError(ctx, queryErr, false)
		}
		if index.origin == "pk" {
			if primary != nil {
				return nil, nil, contract.NewError(contract.ErrorConflict, "sqlite_schema_physical_drift", "agent-host SQLite has multiple primary indexes: "+table)
			}
			primary = columns
		} else {
			result = append(result, columns)
		}
	}
	return primary, result, nil
}

func validateDeploymentIndexXInfoRowV2(
	key int,
	cid int,
	name sql.NullString,
	descending int,
	collation sql.NullString,
	table string,
) (bool, error) {
	switch key {
	case 1:
		if cid < 0 || !name.Valid || descending != 0 || !collation.Valid || collation.String != "BINARY" {
			return false, contract.NewError(contract.ErrorConflict, "sqlite_schema_physical_drift", "agent-host SQLite index key shape or collation drifted: "+table)
		}
		return true, nil
	case 0:
		if cid != -1 || name.Valid || descending != 0 || !collation.Valid || collation.String != "BINARY" {
			return false, contract.NewError(contract.ErrorConflict, "sqlite_schema_physical_drift", "agent-host SQLite index auxiliary row drifted: "+table)
		}
		return false, nil
	default:
		return false, contract.NewError(contract.ErrorConflict, "sqlite_schema_physical_drift", "agent-host SQLite index_xinfo key class drifted: "+table)
	}
}

func readDeploymentForeignKeysV2(ctx context.Context, tx *sql.Tx, table string) ([]deploymentSchemaForeignKeyV2, error) {
	rows, err := tx.QueryContext(ctx, fmt.Sprintf("PRAGMA foreign_key_list(%q)", table))
	if err != nil {
		return nil, mapDBError(ctx, err, false)
	}
	defer rows.Close()
	var result []deploymentSchemaForeignKeyV2
	for rows.Next() {
		var id, sequence int
		var target, from, to, onUpdate, onDelete, match string
		if err = rows.Scan(&id, &sequence, &target, &from, &to, &onUpdate, &onDelete, &match); err != nil {
			return nil, mapDBError(ctx, err, false)
		}
		if id != 0 {
			return nil, contract.NewError(contract.ErrorConflict, "sqlite_schema_physical_drift", "agent-host SQLite unexpected foreign key group: "+table)
		}
		result = append(result, deploymentSchemaForeignKeyV2{
			Sequence: sequence,
			Table:    target,
			From:     from,
			To:       to,
			OnUpdate: onUpdate,
			OnDelete: onDelete,
			Match:    match,
		})
	}
	if err = rows.Err(); err != nil {
		return nil, mapDBError(ctx, err, false)
	}
	return result, nil
}

func verifyDeploymentConstraintBehaviorV2(ctx context.Context, tx *sql.Tx) error {
	checks := []struct {
		name string
		sql  string
		args func(string, string) []any
	}{
		{
			name: "history revision",
			sql:  deploymentHistoryConstraintInsertV2(),
			args: func(hostID, deploymentID string) []any {
				return deploymentHistoryConstraintArgsV2(hostID, deploymentID, 0, 1, 1, 1, 2)
			},
		},
		{
			name: "history selection revision",
			sql:  deploymentHistoryConstraintInsertV2(),
			args: func(hostID, deploymentID string) []any {
				return deploymentHistoryConstraintArgsV2(hostID, deploymentID, 1, 0, 1, 1, 2)
			},
		},
		{
			name: "history selection expiry",
			sql:  deploymentHistoryConstraintInsertV2(),
			args: func(hostID, deploymentID string) []any {
				return deploymentHistoryConstraintArgsV2(hostID, deploymentID, 1, 1, 0, 1, 2)
			},
		},
		{
			name: "history checked time",
			sql:  deploymentHistoryConstraintInsertV2(),
			args: func(hostID, deploymentID string) []any {
				return deploymentHistoryConstraintArgsV2(hostID, deploymentID, 1, 1, 1, 0, 2)
			},
		},
		{
			name: "history expiry window",
			sql:  deploymentHistoryConstraintInsertV2(),
			args: func(hostID, deploymentID string) []any {
				return deploymentHistoryConstraintArgsV2(hostID, deploymentID, 1, 1, 1, 2, 2)
			},
		},
		{
			name: "current revision",
			sql:  deploymentCurrentConstraintInsertV2(),
			args: func(hostID, deploymentID string) []any {
				return deploymentCurrentConstraintArgsV2(hostID, deploymentID, 0, 1, 1)
			},
		},
		{
			name: "current selection revision",
			sql:  deploymentCurrentConstraintInsertV2(),
			args: func(hostID, deploymentID string) []any {
				return deploymentCurrentConstraintArgsV2(hostID, deploymentID, 1, 0, 1)
			},
		},
		{
			name: "current selection expiry",
			sql:  deploymentCurrentConstraintInsertV2(),
			args: func(hostID, deploymentID string) []any {
				return deploymentCurrentConstraintArgsV2(hostID, deploymentID, 1, 1, 0)
			},
		},
	}
	for _, check := range checks {
		hostID, deploymentID, err := freshDeploymentProbeCoordinateV2(ctx, tx)
		if err != nil {
			return err
		}
		if err = expectDeploymentConstraintCodeV2(
			ctx,
			tx,
			check.name,
			sqliteConstraintCheckV2,
			check.sql,
			check.args(hostID, deploymentID)...,
		); err != nil {
			return err
		}
	}
	hostID, deploymentID, err := freshDeploymentProbeCoordinateV2(ctx, tx)
	if err != nil {
		return err
	}
	if err = expectDeploymentConstraintCodeV2(
		ctx,
		tx,
		"current foreign key",
		sqliteConstraintForeignKeyV2,
		deploymentCurrentConstraintInsertV2(),
		deploymentCurrentConstraintArgsV2(hostID, deploymentID, 1, 1, 1)...,
	); err != nil {
		return err
	}
	return verifyDeploymentPositiveConstraintBehaviorV2(ctx, tx)
}

func verifyDeploymentPositiveConstraintBehaviorV2(ctx context.Context, tx *sql.Tx) error {
	hostID, deploymentID, err := freshDeploymentProbeCoordinateV2(ctx, tx)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "SAVEPOINT agent_host_deployment_v2_positive"); err != nil {
		return mapDBError(ctx, err, false)
	}
	cleanup := func() error {
		cleanupCtx := context.WithoutCancel(ctx)
		if _, cleanupErr := tx.ExecContext(cleanupCtx, "ROLLBACK TO agent_host_deployment_v2_positive"); cleanupErr != nil {
			return mapDBError(cleanupCtx, cleanupErr, false)
		}
		if _, cleanupErr := tx.ExecContext(cleanupCtx, "RELEASE agent_host_deployment_v2_positive"); cleanupErr != nil {
			return mapDBError(cleanupCtx, cleanupErr, false)
		}
		return nil
	}
	if _, err = tx.ExecContext(
		ctx,
		deploymentHistoryConstraintInsertV2(),
		deploymentHistoryConstraintArgsV2(hostID, deploymentID, 1, 1, 3, 1, 2)...,
	); err != nil {
		if cleanupErr := cleanup(); cleanupErr != nil {
			return cleanupErr
		}
		return contract.NewError(contract.ErrorConflict, "sqlite_schema_positive_probe_failed", "agent-host SQLite rejected a valid deployment history row")
	}
	if _, err = tx.ExecContext(
		ctx,
		deploymentCurrentConstraintInsertV2(),
		deploymentCurrentConstraintArgsV2(hostID, deploymentID, 1, 1, 3)...,
	); err != nil {
		if cleanupErr := cleanup(); cleanupErr != nil {
			return cleanupErr
		}
		return contract.NewError(contract.ErrorConflict, "sqlite_schema_positive_probe_failed", "agent-host SQLite rejected a valid deployment current row")
	}
	if err = cleanup(); err != nil {
		return err
	}
	var historyCount, currentCount int
	if err = tx.QueryRowContext(
		ctx,
		`SELECT COUNT(1) FROM agent_host_deployment_current_history_v2 WHERE host_id=? AND deployment_id=?`,
		hostID,
		deploymentID,
	).Scan(&historyCount); err != nil {
		return mapDBError(ctx, err, false)
	}
	if err = tx.QueryRowContext(
		ctx,
		`SELECT COUNT(1) FROM agent_host_deployment_current_v2 WHERE host_id=? AND deployment_id=?`,
		hostID,
		deploymentID,
	).Scan(&currentCount); err != nil {
		return mapDBError(ctx, err, false)
	}
	if historyCount != 0 || currentCount != 0 {
		return contract.NewError(contract.ErrorConflict, "sqlite_schema_positive_probe_residual", "agent-host SQLite positive schema probe left residual rows")
	}
	return nil
}

func expectDeploymentConstraintCodeV2(
	ctx context.Context,
	tx *sql.Tx,
	name string,
	wantCode int,
	statement string,
	args ...any,
) error {
	_, err := tx.ExecContext(ctx, statement, args...)
	if err == nil {
		return contract.NewError(contract.ErrorConflict, "sqlite_schema_constraint_missing", "agent-host SQLite constraint accepted an invalid row: "+name)
	}
	coded, ok := err.(interface{ Code() int })
	if !ok || coded.Code() != wantCode {
		return contract.NewError(contract.ErrorConflict, "sqlite_schema_constraint_drift", "agent-host SQLite constraint returned the wrong failure class: "+name)
	}
	return nil
}

func deploymentHistoryConstraintInsertV2() string {
	return `INSERT INTO agent_host_deployment_current_history_v2(
		host_id,deployment_id,revision,digest,bootstrap_digest,
		selection_id,selection_revision,selection_digest,selection_expires_unix_nano,
		checked_unix_nano,expires_unix_nano,row_digest,canonical_json
	) VALUES(?,?,?,'proof-digest','proof-bootstrap','proof-selection',?,'proof-selection-digest',?, ?,?,'proof-row',X'7B7D')`
}

func deploymentHistoryConstraintArgsV2(hostID, deploymentID string, revision, selectionRevision uint64, selectionExpires, checked, expires int64) []any {
	return []any{hostID, deploymentID, revision, selectionRevision, selectionExpires, checked, expires}
}

func deploymentCurrentConstraintInsertV2() string {
	return `INSERT INTO agent_host_deployment_current_v2(
		host_id,deployment_id,revision,digest,
		selection_id,selection_revision,selection_digest,selection_expires_unix_nano,row_digest
	) VALUES(?,?,?,'proof-digest','proof-selection',?,'proof-selection-digest',?,'proof-row')`
}

func deploymentCurrentConstraintArgsV2(hostID, deploymentID string, revision, selectionRevision uint64, selectionExpires int64) []any {
	return []any{hostID, deploymentID, revision, selectionRevision, selectionExpires}
}

func freshDeploymentProbeCoordinateV2(ctx context.Context, tx *sql.Tx) (string, string, error) {
	for attempt := 0; attempt < 8; attempt++ {
		nonce := make([]byte, 16)
		if _, err := rand.Read(nonce); err != nil {
			return "", "", contract.NewError(contract.ErrorUnavailable, "sqlite_schema_probe_entropy_unavailable", "agent-host SQLite schema probe nonce is unavailable")
		}
		suffix := hex.EncodeToString(nonce)
		hostID := "schema-probe-host-" + suffix
		deploymentID := "schema-probe-deployment-" + suffix
		var historyCount, currentCount int
		if err := tx.QueryRowContext(
			ctx,
			`SELECT COUNT(1) FROM agent_host_deployment_current_history_v2 WHERE host_id=? AND deployment_id=?`,
			hostID,
			deploymentID,
		).Scan(&historyCount); err != nil {
			return "", "", mapDBError(ctx, err, false)
		}
		if err := tx.QueryRowContext(
			ctx,
			`SELECT COUNT(1) FROM agent_host_deployment_current_v2 WHERE host_id=? AND deployment_id=?`,
			hostID,
			deploymentID,
		).Scan(&currentCount); err != nil {
			return "", "", mapDBError(ctx, err, false)
		}
		if historyCount == 0 && currentCount == 0 {
			return hostID, deploymentID, nil
		}
	}
	return "", "", contract.NewError(contract.ErrorUnavailable, "sqlite_schema_probe_collision", "agent-host SQLite could not allocate a fresh schema probe coordinate")
}

func cloneStringMatrixV2(values [][]string) [][]string {
	if values == nil {
		return nil
	}
	result := make([][]string, len(values))
	for index := range values {
		result[index] = append([]string(nil), values[index]...)
	}
	return result
}

func sortStringMatrixV2(values [][]string) {
	sort.Slice(values, func(i, j int) bool {
		return strings.Join(values[i], "\x00") < strings.Join(values[j], "\x00")
	})
}
