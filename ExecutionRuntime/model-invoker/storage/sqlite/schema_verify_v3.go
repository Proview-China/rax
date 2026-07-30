package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	sqlite3 "modernc.org/sqlite/lib"
)

var invocationMaterialConstraintProbeSequenceV3 atomic.Uint64

const invocationMaterialHistoryCreateTableV3 = `
CREATE TABLE invocation_material_v2_history (
  material_id TEXT NOT NULL,
  revision INTEGER NOT NULL CHECK(revision > 0),
  material_digest TEXT NOT NULL,
  prepared_id TEXT NOT NULL,
  prepared_revision INTEGER NOT NULL CHECK(prepared_revision > 0),
  prepared_digest TEXT NOT NULL,
  current_id TEXT NOT NULL,
  current_revision INTEGER NOT NULL CHECK(current_revision > 0),
  current_digest TEXT NOT NULL,
  current_checked_unix_nano INTEGER NOT NULL CHECK(current_checked_unix_nano > 0),
  current_expires_unix_nano INTEGER NOT NULL CHECK(current_expires_unix_nano > 0),
  current_not_after_unix_nano INTEGER NOT NULL CHECK(current_not_after_unix_nano > 0),
  route_call_digest TEXT NOT NULL,
  authorization_id TEXT NOT NULL UNIQUE,
  source_lineage_digest TEXT NOT NULL,
  authorization_digest TEXT NOT NULL,
  expires_unix_nano INTEGER NOT NULL CHECK(expires_unix_nano > 0),
  canonical_json BLOB NOT NULL,
  PRIMARY KEY(material_id, revision)
)`

type schemaColumnV3 struct {
	name    string
	kind    string
	notNull int
	pk      int
	hidden  int
}

var invocationMaterialHistoryColumnsV3 = []schemaColumnV3{
	{name: "material_id", kind: "TEXT", notNull: 1, pk: 1},
	{name: "revision", kind: "INTEGER", notNull: 1, pk: 2},
	{name: "material_digest", kind: "TEXT", notNull: 1},
	{name: "prepared_id", kind: "TEXT", notNull: 1},
	{name: "prepared_revision", kind: "INTEGER", notNull: 1},
	{name: "prepared_digest", kind: "TEXT", notNull: 1},
	{name: "current_id", kind: "TEXT", notNull: 1},
	{name: "current_revision", kind: "INTEGER", notNull: 1},
	{name: "current_digest", kind: "TEXT", notNull: 1},
	{name: "current_checked_unix_nano", kind: "INTEGER", notNull: 1},
	{name: "current_expires_unix_nano", kind: "INTEGER", notNull: 1},
	{name: "current_not_after_unix_nano", kind: "INTEGER", notNull: 1},
	{name: "route_call_digest", kind: "TEXT", notNull: 1},
	{name: "authorization_id", kind: "TEXT", notNull: 1},
	{name: "source_lineage_digest", kind: "TEXT", notNull: 1},
	{name: "authorization_digest", kind: "TEXT", notNull: 1},
	{name: "expires_unix_nano", kind: "INTEGER", notNull: 1},
	{name: "canonical_json", kind: "BLOB", notNull: 1},
}

var invocationMaterialHistoryExactIndexColumnsV3 = []string{
	"material_id",
	"revision",
	"material_digest",
	"prepared_id",
	"prepared_revision",
	"prepared_digest",
	"current_id",
	"current_revision",
	"current_digest",
	"current_checked_unix_nano",
	"current_expires_unix_nano",
	"current_not_after_unix_nano",
	"route_call_digest",
	"authorization_id",
	"source_lineage_digest",
	"authorization_digest",
}

func (s *Store) verifyV3(ctx context.Context) error {
	if err := contextErrorV1(ctx, "verify_v3"); err != nil {
		return err
	}
	if s == nil || s.db == nil {
		return errorV1(
			modelinvoker.GovernedModelInvocationErrorUnavailable,
			"verify_v3",
			"sqlite repository is unavailable",
			nil,
		)
	}
	if err := verifyInvocationMaterialHistoryTableV3(ctx, s.db); err != nil {
		return err
	}
	if err := verifyInvocationMaterialHistoryColumnsV3(ctx, s.db); err != nil {
		return err
	}
	if err := verifyInvocationMaterialHistoryConstraintsV3(ctx, s.db); err != nil {
		return err
	}
	if err := verifyInvocationMaterialHistoryIndexesV3(ctx, s.db); err != nil {
		return err
	}
	if err := verifyInvocationMaterialHistoryTriggersV3(ctx, s.db); err != nil {
		return err
	}
	var forbidden int
	if err := s.db.QueryRowContext(
		ctx,
		`SELECT COUNT(*) FROM sqlite_master
		 WHERE type='table' AND name='invocation_material_v2_current'`,
	).Scan(&forbidden); err != nil {
		return mapDBErrorV1(ctx, "verify_v3", err, false)
	}
	if forbidden != 0 {
		return errorV1(
			modelinvoker.GovernedModelInvocationErrorConflict,
			"verify_v3",
			"forbidden invocation material V2 current table exists",
			nil,
		)
	}
	return nil
}

func verifyInvocationMaterialHistoryTableV3(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, `PRAGMA table_list('invocation_material_v2_history')`)
	if err != nil {
		return mapDBErrorV1(ctx, "verify_v3", err, false)
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var schema, name, kind string
		var columns, withoutRowID, strict int
		if err := rows.Scan(
			&schema,
			&name,
			&kind,
			&columns,
			&withoutRowID,
			&strict,
		); err != nil {
			return mapDBErrorV1(ctx, "verify_v3", err, false)
		}
		count++
		if schema != "main" || name != "invocation_material_v2_history" ||
			kind != "table" || columns != len(invocationMaterialHistoryColumnsV3) ||
			withoutRowID != 0 || strict != 0 {
			return schemaConflictV3("invocation material V2 history table options drifted")
		}
	}
	if err := rows.Err(); err != nil {
		return mapDBErrorV1(ctx, "verify_v3", err, false)
	}
	if count != 1 {
		return schemaConflictV3("invocation material V2 history table cardinality drifted")
	}
	var definition string
	if err := db.QueryRowContext(
		ctx,
		`SELECT sql FROM sqlite_master
		 WHERE type='table' AND name='invocation_material_v2_history'`,
	).Scan(&definition); err != nil {
		if err == sql.ErrNoRows {
			return schemaConflictV3("invocation material V2 history table is absent")
		}
		return mapDBErrorV1(ctx, "verify_v3", err, false)
	}
	actualTokens, err := sqliteSchemaTokensV3(definition)
	if err != nil {
		return schemaConflictV3("invocation material V2 history DDL cannot be tokenized")
	}
	expectedTokens, err := sqliteSchemaTokensV3(invocationMaterialHistoryCreateTableV3)
	if err != nil || !equalStringsV3(actualTokens, expectedTokens) {
		return schemaConflictV3("invocation material V2 history DDL drifted")
	}
	return nil
}

func sqliteSchemaTokensV3(definition string) ([]string, error) {
	tokens := make([]string, 0, len(definition)/2)
	for offset := 0; offset < len(definition); {
		current := definition[offset]
		if isSQLiteSpaceV3(current) {
			offset++
			continue
		}
		if current == '-' && offset+1 < len(definition) && definition[offset+1] == '-' {
			offset += 2
			for offset < len(definition) && definition[offset] != '\n' {
				offset++
			}
			continue
		}
		if current == '/' && offset+1 < len(definition) && definition[offset+1] == '*' {
			offset += 2
			closed := false
			for offset+1 < len(definition) {
				if definition[offset] == '*' && definition[offset+1] == '/' {
					offset += 2
					closed = true
					break
				}
				offset++
			}
			if !closed {
				return nil, fmt.Errorf("unterminated SQLite block comment")
			}
			continue
		}
		if current == '\'' {
			value, next, err := sqliteQuotedTokenV3(definition, offset, '\'', '\'')
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, "string:"+value)
			offset = next
			continue
		}
		if current == '"' || current == '`' {
			value, next, err := sqliteQuotedTokenV3(
				definition,
				offset,
				current,
				current,
			)
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, strings.ToLower(value))
			offset = next
			continue
		}
		if current == '[' {
			value, next, err := sqliteQuotedTokenV3(definition, offset, '[', ']')
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, strings.ToLower(value))
			offset = next
			continue
		}
		if isSQLiteIdentifierStartV3(current) {
			end := offset + 1
			for end < len(definition) && isSQLiteIdentifierPartV3(definition[end]) {
				end++
			}
			tokens = append(tokens, strings.ToLower(definition[offset:end]))
			offset = end
			continue
		}
		if current >= '0' && current <= '9' {
			end := offset + 1
			for end < len(definition) && definition[end] >= '0' && definition[end] <= '9' {
				end++
			}
			tokens = append(tokens, definition[offset:end])
			offset = end
			continue
		}
		if offset+1 < len(definition) {
			operator := definition[offset : offset+2]
			switch operator {
			case ">=", "<=", "<>", "!=", "==", "||", "->":
				tokens = append(tokens, operator)
				offset += 2
				continue
			}
		}
		tokens = append(tokens, string(current))
		offset++
	}
	if len(tokens) > 0 && tokens[len(tokens)-1] == ";" {
		tokens = tokens[:len(tokens)-1]
	}
	return tokens, nil
}

func sqliteQuotedTokenV3(
	definition string,
	offset int,
	open byte,
	close byte,
) (string, int, error) {
	if offset >= len(definition) || definition[offset] != open {
		return "", 0, fmt.Errorf("invalid SQLite quoted token")
	}
	var value strings.Builder
	for index := offset + 1; index < len(definition); index++ {
		if definition[index] != close {
			value.WriteByte(definition[index])
			continue
		}
		if index+1 < len(definition) && definition[index+1] == close {
			value.WriteByte(close)
			index++
			continue
		}
		return value.String(), index + 1, nil
	}
	return "", 0, fmt.Errorf("unterminated SQLite quoted token")
}

func isSQLiteSpaceV3(value byte) bool {
	switch value {
	case ' ', '\t', '\n', '\r', '\f', '\v':
		return true
	default:
		return false
	}
}

func isSQLiteIdentifierStartV3(value byte) bool {
	return value == '_' ||
		value >= 'a' && value <= 'z' ||
		value >= 'A' && value <= 'Z' ||
		value >= 0x80
}

func isSQLiteIdentifierPartV3(value byte) bool {
	return isSQLiteIdentifierStartV3(value) || value >= '0' && value <= '9'
}

func verifyInvocationMaterialHistoryColumnsV3(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, `PRAGMA table_xinfo('invocation_material_v2_history')`)
	if err != nil {
		return mapDBErrorV1(ctx, "verify_v3", err, false)
	}
	defer rows.Close()
	actual := make([]schemaColumnV3, 0, len(invocationMaterialHistoryColumnsV3))
	for rows.Next() {
		var cid int
		var column schemaColumnV3
		var defaultValue sql.NullString
		if err := rows.Scan(
			&cid,
			&column.name,
			&column.kind,
			&column.notNull,
			&defaultValue,
			&column.pk,
			&column.hidden,
		); err != nil {
			return mapDBErrorV1(ctx, "verify_v3", err, false)
		}
		if cid != len(actual) || defaultValue.Valid {
			return schemaConflictV3("invocation material V2 history column order or default drifted")
		}
		column.kind = strings.ToUpper(strings.TrimSpace(column.kind))
		actual = append(actual, column)
	}
	if err := rows.Err(); err != nil {
		return mapDBErrorV1(ctx, "verify_v3", err, false)
	}
	if len(actual) != len(invocationMaterialHistoryColumnsV3) {
		return schemaConflictV3("invocation material V2 history column count drifted")
	}
	for index := range actual {
		if actual[index] != invocationMaterialHistoryColumnsV3[index] {
			return schemaConflictV3("invocation material V2 history column contract drifted")
		}
	}
	return nil
}

func verifyInvocationMaterialHistoryConstraintsV3(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return mapDBErrorV1(ctx, "verify_v3", err, false)
	}
	defer func() { _ = tx.Rollback() }()

	type invalidConstraintProbeV3 struct {
		name                   string
		revision               int64
		preparedRevision       int64
		currentRevision        int64
		currentCheckedUnixNano int64
		currentExpiresUnixNano int64
		currentNotAfterNano    int64
		expiresUnixNano        int64
	}
	valid := invalidConstraintProbeV3{
		revision:               1,
		preparedRevision:       1,
		currentRevision:        1,
		currentCheckedUnixNano: 1,
		currentExpiresUnixNano: 2,
		currentNotAfterNano:    3,
		expiresUnixNano:        1,
	}
	probes := []invalidConstraintProbeV3{
		func() invalidConstraintProbeV3 { p := valid; p.name = "revision-zero"; p.revision = 0; return p }(),
		func() invalidConstraintProbeV3 { p := valid; p.name = "revision-negative"; p.revision = -1; return p }(),
		func() invalidConstraintProbeV3 {
			p := valid
			p.name = "prepared-revision-zero"
			p.preparedRevision = 0
			return p
		}(),
		func() invalidConstraintProbeV3 {
			p := valid
			p.name = "prepared-revision-negative"
			p.preparedRevision = -1
			return p
		}(),
		func() invalidConstraintProbeV3 {
			p := valid
			p.name = "current-revision-zero"
			p.currentRevision = 0
			return p
		}(),
		func() invalidConstraintProbeV3 {
			p := valid
			p.name = "current-revision-negative"
			p.currentRevision = -1
			return p
		}(),
		func() invalidConstraintProbeV3 {
			p := valid
			p.name = "current-checked-zero"
			p.currentCheckedUnixNano = 0
			return p
		}(),
		func() invalidConstraintProbeV3 {
			p := valid
			p.name = "current-checked-negative"
			p.currentCheckedUnixNano = -1
			return p
		}(),
		func() invalidConstraintProbeV3 {
			p := valid
			p.name = "current-expiry-zero"
			p.currentExpiresUnixNano = 0
			return p
		}(),
		func() invalidConstraintProbeV3 {
			p := valid
			p.name = "current-expiry-negative"
			p.currentExpiresUnixNano = -1
			return p
		}(),
		func() invalidConstraintProbeV3 {
			p := valid
			p.name = "current-not-after-zero"
			p.currentNotAfterNano = 0
			return p
		}(),
		func() invalidConstraintProbeV3 {
			p := valid
			p.name = "current-not-after-negative"
			p.currentNotAfterNano = -1
			return p
		}(),
		func() invalidConstraintProbeV3 { p := valid; p.name = "expiry-zero"; p.expiresUnixNano = 0; return p }(),
		func() invalidConstraintProbeV3 {
			p := valid
			p.name = "expiry-negative"
			p.expiresUnixNano = -1
			return p
		}(),
	}
	for _, probe := range probes {
		err := insertInvocationMaterialConstraintProbeV3(
			ctx,
			tx,
			probe.name,
			probe.revision,
			probe.preparedRevision,
			probe.currentRevision,
			probe.currentCheckedUnixNano,
			probe.currentExpiresUnixNano,
			probe.currentNotAfterNano,
			probe.expiresUnixNano,
		)
		if err == nil {
			return schemaConflictV3(
				"invocation material V2 history accepted invalid " + probe.name,
			)
		}
		if !isSQLiteConstraintErrorV3(err) {
			if isSQLiteAnyConstraintErrorV3(err) {
				return schemaConflictV3(
					"invocation material V2 invalid probe was rejected by a non-CHECK constraint",
				)
			}
			return mapDBErrorV1(ctx, "verify_v3", err, false)
		}
	}
	valid.name = "valid"
	if err := insertInvocationMaterialConstraintProbeV3(
		ctx, tx, valid.name, valid.revision, valid.preparedRevision,
		valid.currentRevision, valid.currentCheckedUnixNano,
		valid.currentExpiresUnixNano, valid.currentNotAfterNano,
		valid.expiresUnixNano,
	); err != nil {
		if isSQLiteConstraintErrorV3(err) {
			return schemaConflictV3(
				"invocation material V2 history rejected a valid constraint probe",
			)
		}
		return mapDBErrorV1(ctx, "verify_v3", err, false)
	}
	return nil
}

func insertInvocationMaterialConstraintProbeV3(
	ctx context.Context,
	tx *sql.Tx,
	name string,
	revision int64,
	preparedRevision int64,
	currentRevision int64,
	currentCheckedUnixNano int64,
	currentExpiresUnixNano int64,
	currentNotAfterUnixNano int64,
	expiresUnixNano int64,
) error {
	sequence := invocationMaterialConstraintProbeSequenceV3.Add(1)
	id := fmt.Sprintf(
		"schema-v3-constraint-probe-%s-%d-%d",
		name,
		time.Now().UnixNano(),
		sequence,
	)
	_, err := tx.ExecContext(
		ctx,
		`INSERT INTO invocation_material_v2_history(
		   material_id,revision,material_digest,
		   prepared_id,prepared_revision,prepared_digest,
		   current_id,current_revision,current_digest,
		   current_checked_unix_nano,current_expires_unix_nano,current_not_after_unix_nano,
		   route_call_digest,authorization_id,source_lineage_digest,authorization_digest,
		   expires_unix_nano,canonical_json
		 ) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		id,
		revision,
		"probe-material-digest",
		"probe-prepared-id",
		preparedRevision,
		"probe-prepared-digest",
		id+"-current",
		currentRevision,
		"probe-current-digest",
		currentCheckedUnixNano,
		currentExpiresUnixNano,
		currentNotAfterUnixNano,
		"probe-route-call-digest",
		id+"-authorization",
		"probe-source-lineage-digest",
		"probe-authorization-digest",
		expiresUnixNano,
		[]byte(`{}`),
	)
	return err
}

func isSQLiteConstraintErrorV3(err error) bool {
	code, ok := sqliteErrorCodeV3(err)
	return ok && code == sqlite3.SQLITE_CONSTRAINT_CHECK
}

func isSQLiteAnyConstraintErrorV3(err error) bool {
	code, ok := sqliteErrorCodeV3(err)
	return ok && code&0xff == sqlite3.SQLITE_CONSTRAINT
}

func sqliteErrorCodeV3(err error) (int, bool) {
	type sqliteCodeError interface {
		Code() int
	}
	var coded sqliteCodeError
	if !errors.As(err, &coded) || coded == nil {
		return 0, false
	}
	return coded.Code(), true
}

func verifyInvocationMaterialHistoryIndexesV3(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, `PRAGMA index_list('invocation_material_v2_history')`)
	if err != nil {
		return mapDBErrorV1(ctx, "verify_v3", err, false)
	}
	defer rows.Close()
	type indexV3 struct {
		name    string
		unique  int
		origin  string
		partial int
	}
	var indexes []indexV3
	for rows.Next() {
		var sequence, unique, partial int
		var name, origin string
		if err := rows.Scan(&sequence, &name, &unique, &origin, &partial); err != nil {
			return mapDBErrorV1(ctx, "verify_v3", err, false)
		}
		indexes = append(indexes, indexV3{
			name: name, unique: unique, origin: origin, partial: partial,
		})
	}
	if err := rows.Err(); err != nil {
		return mapDBErrorV1(ctx, "verify_v3", err, false)
	}
	if err := rows.Close(); err != nil {
		return mapDBErrorV1(ctx, "verify_v3", err, false)
	}
	if len(indexes) != 3 {
		return schemaConflictV3("invocation material V2 history index set drifted")
	}
	exactIndex := false
	authorizationUnique := false
	primaryKey := false
	for _, index := range indexes {
		if index.partial != 0 {
			return schemaConflictV3("invocation material V2 history partial index is forbidden")
		}
		columns, err := indexKeyColumnsV3(ctx, db, index.name)
		if err != nil {
			return err
		}
		switch index.origin {
		case "c":
			if exactIndex || index.name != "invocation_material_v2_history_exact" ||
				index.unique != 0 ||
				!equalStringsV3(columns, invocationMaterialHistoryExactIndexColumnsV3) {
				return schemaConflictV3("invocation material V2 history exact index drifted")
			}
			exactIndex = true
		case "u":
			if authorizationUnique || index.unique != 1 ||
				!equalStringsV3(columns, []string{"authorization_id"}) {
				return schemaConflictV3("invocation material V2 authorization UNIQUE index drifted")
			}
			authorizationUnique = true
		case "pk":
			if primaryKey || index.unique != 1 ||
				!equalStringsV3(columns, []string{"material_id", "revision"}) {
				return schemaConflictV3("invocation material V2 history primary-key index drifted")
			}
			primaryKey = true
		default:
			return schemaConflictV3("invocation material V2 history index origin drifted")
		}
	}
	if !exactIndex || !authorizationUnique || !primaryKey {
		return schemaConflictV3("invocation material V2 history index set is incomplete")
	}
	return nil
}

// IndexXInfoConformanceRowV4 is a SQLite implementation conformance row. It is
// not a Model domain fact and grants no runtime authority.
type IndexXInfoConformanceRowV4 struct {
	Sequence   int
	CID        int
	Column     sql.NullString
	Descending int
	Collation  string
	Key        int
}

func indexKeyColumnsV3(ctx context.Context, db *sql.DB, name string) ([]string, error) {
	rows, err := db.QueryContext(ctx, `PRAGMA index_xinfo(`+quoteSQLiteIdentifierV3(name)+`)`)
	if err != nil {
		return nil, mapDBErrorV1(ctx, "verify_v3", err, false)
	}
	var physical []IndexXInfoConformanceRowV4
	var scanErr error
	for rows.Next() {
		var row IndexXInfoConformanceRowV4
		if err := rows.Scan(
			&row.Sequence,
			&row.CID,
			&row.Column,
			&row.Descending,
			&row.Collation,
			&row.Key,
		); err != nil {
			scanErr = err
			break
		}
		physical = append(physical, row)
	}
	iterationErr := errors.Join(scanErr, rows.Err())
	closeErr := rows.Close()
	if err := errors.Join(iterationErr, closeErr); err != nil {
		return nil, mapDBErrorV1(ctx, "verify_v3", err, false)
	}
	columns, err := ValidateIndexXInfoRowsForConformanceV4(physical)
	if err != nil {
		return nil, schemaConflictV3(err.Error())
	}
	return columns, nil
}

// ValidateIndexXInfoRowsForConformanceV4 freezes the physical index_xinfo
// shape shared by the SQLite V3 and V4 schema gates.
func ValidateIndexXInfoRowsForConformanceV4(
	physical []IndexXInfoConformanceRowV4,
) ([]string, error) {
	var columns []string
	auxiliaryRows := 0
	for sequence, row := range physical {
		if row.Sequence != sequence || row.Descending != 0 ||
			row.Collation != "BINARY" {
			return nil, fmt.Errorf("sqlite index column order or collation drifted")
		}
		switch row.Key {
		case 1:
			if row.CID < 0 || !row.Column.Valid ||
				strings.TrimSpace(row.Column.String) == "" ||
				auxiliaryRows != 0 {
				return nil, fmt.Errorf("sqlite index key expression drifted")
			}
			columns = append(columns, row.Column.String)
		case 0:
			if row.CID != -1 || row.Column.Valid || auxiliaryRows != 0 {
				return nil, fmt.Errorf("sqlite index auxiliary column drifted")
			}
			auxiliaryRows++
		default:
			return nil, fmt.Errorf("sqlite index key classification drifted")
		}
	}
	if len(columns) == 0 || auxiliaryRows != 1 {
		return nil, fmt.Errorf("sqlite index physical shape drifted")
	}
	return columns, nil
}

func verifyInvocationMaterialHistoryTriggersV3(ctx context.Context, db *sql.DB) error {
	var count int
	if err := db.QueryRowContext(
		ctx,
		`SELECT COUNT(*) FROM sqlite_master
		 WHERE type='trigger' AND tbl_name='invocation_material_v2_history'`,
	).Scan(&count); err != nil {
		return mapDBErrorV1(ctx, "verify_v3", err, false)
	}
	if count != 0 {
		return schemaConflictV3("invocation material V2 history triggers are forbidden")
	}
	return nil
}

func quoteSQLiteIdentifierV3(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func equalStringsV3(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func schemaConflictV3(message string) error {
	return errorV1(
		modelinvoker.GovernedModelInvocationErrorConflict,
		"verify_v3",
		message,
		nil,
	)
}
