package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type schemaQueryerV2 interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type schemaColumnV2 struct {
	Name       string
	Type       string
	NotNull    int
	DefaultSQL sql.NullString
	PK         int
	Hidden     int
}

type ownedSchemaDispositionV2 int

const (
	ownedSchemaCreateV2 ownedSchemaDispositionV2 = iota + 1
	ownedSchemaVerifyV2
)

// classifyOwnedSchemaV2 is the only bootstrap decision made before an Owner
// schema is touched. A ledger is an assertion that the complete physical
// schema already exists; it must never authorize CREATE IF NOT EXISTS repair.
func classifyOwnedSchemaV2(ctx context.Context, query schemaQueryerV2, ledger string, owned []string) (ownedSchemaDispositionV2, error) {
	if strings.TrimSpace(ledger) == "" || len(owned) == 0 {
		return 0, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Tool SQLite owned schema inventory is invalid")
	}
	placeholders := make([]string, len(owned))
	args := make([]any, len(owned))
	for index, name := range owned {
		if strings.TrimSpace(name) == "" {
			return 0, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Tool SQLite owned schema object name is invalid")
		}
		placeholders[index], args[index] = "?", name
	}
	rows, err := query.QueryContext(ctx, `SELECT name,type FROM sqlite_master WHERE name IN (`+strings.Join(placeholders, ",")+`) ORDER BY name`, args...)
	if err != nil {
		return 0, mapDBErrorV1(ctx, err, false)
	}
	found := make(map[string]string)
	for rows.Next() {
		var name, kind string
		if err = rows.Scan(&name, &kind); err != nil {
			_ = rows.Close()
			return 0, mapDBErrorV1(ctx, err, false)
		}
		if _, duplicate := found[name]; duplicate {
			_ = rows.Close()
			return 0, core.NewError(core.ErrorConflict, core.ReasonUnknownSchema, "Tool SQLite owned schema inventory contains duplicate objects")
		}
		found[name] = kind
	}
	if err = rows.Err(); err != nil {
		_ = rows.Close()
		return 0, mapDBErrorV1(ctx, err, false)
	}
	if err = rows.Close(); err != nil {
		return 0, mapDBErrorV1(ctx, err, false)
	}
	ledgerKind, ledgerPresent := found[ledger]
	if !ledgerPresent {
		if len(found) != 0 {
			return 0, core.NewError(core.ErrorConflict, core.ReasonUnknownSchema, "Tool SQLite owned schema is partial without its ledger")
		}
		return ownedSchemaCreateV2, nil
	}
	if ledgerKind != "table" {
		return 0, core.NewError(core.ErrorConflict, core.ReasonUnknownSchema, "Tool SQLite owned schema ledger object kind drifted")
	}
	return ownedSchemaVerifyV2, nil
}

func verifyOwnedSchemaLedgerV2(ctx context.Context, query schemaQueryerV2, table string, version int64, expected core.Digest) error {
	var count, minVersion, maxVersion int64
	if err := query.QueryRowContext(ctx, `SELECT COUNT(*),COALESCE(MIN(version),0),COALESCE(MAX(version),0) FROM `+table).Scan(&count, &minVersion, &maxVersion); err != nil {
		return core.NewError(core.ErrorConflict, core.ReasonUnknownSchema, "Tool SQLite owned schema ledger cannot be read exactly")
	}
	if count != 1 || minVersion != version || maxVersion != version {
		return core.NewError(core.ErrorConflict, core.ReasonUnknownSchema, "Tool SQLite owned schema version set drifted")
	}
	var digest string
	if err := query.QueryRowContext(ctx, `SELECT digest FROM `+table+` WHERE version=?`, version).Scan(&digest); err != nil || digest != string(expected) {
		return core.NewError(core.ErrorConflict, core.ReasonUnknownSchema, "Tool SQLite owned schema digest drifted")
	}
	return nil
}

func verifyStrictTableV2(ctx context.Context, query schemaQueryerV2, table string, expected []schemaColumnV2, uniqueSets [][]string, expectedSchema string) error {
	var count, strict int
	if err := query.QueryRowContext(ctx, `SELECT COUNT(*),COALESCE(MAX(strict),0) FROM pragma_table_list WHERE name=? AND type='table'`, table).Scan(&count, &strict); err != nil {
		return mapDBErrorV1(ctx, err, false)
	}
	if count != 1 || strict != 1 {
		return core.NewError(core.ErrorConflict, core.ReasonUnknownSchema, "Tool SQLite table "+table+" is absent or not STRICT")
	}
	rows, err := query.QueryContext(ctx, `SELECT cid,name,type,"notnull",dflt_value,pk,hidden FROM pragma_table_xinfo(?) ORDER BY cid`, table)
	if err != nil {
		return mapDBErrorV1(ctx, err, false)
	}
	var actual []schemaColumnV2
	expectedCID := 0
	for rows.Next() {
		var cid int
		var column schemaColumnV2
		if err = rows.Scan(&cid, &column.Name, &column.Type, &column.NotNull, &column.DefaultSQL, &column.PK, &column.Hidden); err != nil {
			_ = rows.Close()
			return mapDBErrorV1(ctx, err, false)
		}
		if sequenceErr := verifySchemaSequenceV2(table+" table_xinfo", cid, expectedCID); sequenceErr != nil {
			_ = rows.Close()
			return sequenceErr
		}
		expectedCID++
		actual = append(actual, column)
	}
	if err = rows.Err(); err != nil {
		_ = rows.Close()
		return mapDBErrorV1(ctx, err, false)
	}
	if err = rows.Close(); err != nil {
		return mapDBErrorV1(ctx, err, false)
	}
	if len(actual) != len(expected) {
		return core.NewError(core.ErrorConflict, core.ReasonUnknownSchema, "Tool SQLite table "+table+" column count drifted")
	}
	for index := range expected {
		if actual[index].Name != expected[index].Name || strings.ToUpper(actual[index].Type) != expected[index].Type ||
			actual[index].NotNull != expected[index].NotNull || actual[index].DefaultSQL.Valid != expected[index].DefaultSQL.Valid ||
			actual[index].DefaultSQL.String != expected[index].DefaultSQL.String || actual[index].PK != expected[index].PK ||
			actual[index].Hidden != 0 {
			return core.NewError(core.ErrorConflict, core.ReasonUnknownSchema, "Tool SQLite table "+table+" column shape drifted")
		}
	}
	indexRows, err := query.QueryContext(ctx, `SELECT seq,name,"unique",origin,partial FROM pragma_index_list(?) ORDER BY seq`, table)
	if err != nil {
		return mapDBErrorV1(ctx, err, false)
	}
	expectedIndexes := make(map[string]int)
	var primary []schemaColumnV2
	for _, column := range actual {
		if column.PK > 0 {
			primary = append(primary, column)
		}
	}
	sort.Slice(primary, func(i, j int) bool { return primary[i].PK < primary[j].PK })
	primarySignature := ""
	if len(primary) > 0 {
		names := make([]string, len(primary))
		for index := range primary {
			names[index] = primary[index].Name
		}
		primarySignature = strings.Join(names, ",")
	}
	for _, expectedSet := range uniqueSets {
		signature := strings.Join(expectedSet, ",")
		origin := "u"
		if signature == primarySignature {
			origin = "pk"
			if len(primary) == 1 && strings.EqualFold(primary[0].Type, "INTEGER") {
				continue
			}
		}
		expectedIndexes[origin+"\x00"+signature]++
	}
	expectedIndexSeq := 0
	for indexRows.Next() {
		var indexSeq int
		var indexName string
		var unique, partial int
		var origin string
		if err = indexRows.Scan(&indexSeq, &indexName, &unique, &origin, &partial); err != nil {
			_ = indexRows.Close()
			return mapDBErrorV1(ctx, err, false)
		}
		if sequenceErr := verifySchemaSequenceV2(table+" index_list", indexSeq, expectedIndexSeq); sequenceErr != nil {
			_ = indexRows.Close()
			return sequenceErr
		}
		expectedIndexSeq++
		if unique != 1 || partial != 0 || (origin != "u" && origin != "pk") {
			_ = indexRows.Close()
			return core.NewError(core.ErrorConflict, core.ReasonUnknownSchema, "Tool SQLite table "+table+" has an unexpected index")
		}
		columnRows, columnErr := query.QueryContext(ctx, `SELECT seqno,cid,name,"desc",coll,key FROM pragma_index_xinfo(?) ORDER BY seqno`, indexName)
		if columnErr != nil {
			_ = indexRows.Close()
			return mapDBErrorV1(ctx, columnErr, false)
		}
		var columns []string
		expectedIndexXSeq := 0
		auxiliaryRows := 0
		for columnRows.Next() {
			var seqno, cid, descending, key int
			var name, collation sql.NullString
			if columnErr = columnRows.Scan(&seqno, &cid, &name, &descending, &collation, &key); columnErr != nil {
				_ = columnRows.Close()
				_ = indexRows.Close()
				return mapDBErrorV1(ctx, columnErr, false)
			}
			if sequenceErr := verifySchemaSequenceV2(table+" index_xinfo", seqno, expectedIndexXSeq); sequenceErr != nil {
				_ = columnRows.Close()
				_ = indexRows.Close()
				return sequenceErr
			}
			expectedIndexXSeq++
			column, isKey, shapeErr := verifyIndexXInfoRowV2(table, cid, name, descending, collation, key)
			if shapeErr != nil {
				_ = columnRows.Close()
				_ = indexRows.Close()
				return shapeErr
			}
			if !isKey {
				auxiliaryRows++
				if auxiliaryRows != 1 {
					_ = columnRows.Close()
					_ = indexRows.Close()
					return core.NewError(core.ErrorConflict, core.ReasonUnknownSchema, "Tool SQLite table "+table+" index auxiliary row set drifted")
				}
				continue
			}
			if auxiliaryRows != 0 {
				_ = columnRows.Close()
				_ = indexRows.Close()
				return core.NewError(core.ErrorConflict, core.ReasonUnknownSchema, "Tool SQLite table "+table+" index key appears after auxiliary row")
			}
			columns = append(columns, column)
		}
		if columnErr = columnRows.Err(); columnErr != nil {
			_ = columnRows.Close()
			_ = indexRows.Close()
			return mapDBErrorV1(ctx, columnErr, false)
		}
		if columnErr = columnRows.Close(); columnErr != nil {
			_ = indexRows.Close()
			return mapDBErrorV1(ctx, columnErr, false)
		}
		if auxiliaryRows != 1 {
			_ = indexRows.Close()
			return core.NewError(core.ErrorConflict, core.ReasonUnknownSchema, "Tool SQLite table "+table+" index auxiliary row is incomplete")
		}
		signature := origin + "\x00" + strings.Join(columns, ",")
		if expectedIndexes[signature] != 1 {
			_ = indexRows.Close()
			return core.NewError(core.ErrorConflict, core.ReasonUnknownSchema, "Tool SQLite table "+table+" index set drifted")
		}
		delete(expectedIndexes, signature)
	}
	if err = indexRows.Err(); err != nil {
		_ = indexRows.Close()
		return mapDBErrorV1(ctx, err, false)
	}
	if err = indexRows.Close(); err != nil {
		return mapDBErrorV1(ctx, err, false)
	}
	if len(expectedIndexes) != 0 {
		return core.NewError(core.ErrorConflict, core.ReasonUnknownSchema, "Tool SQLite table "+table+" index set is incomplete")
	}
	var triggerCount int
	if err = query.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='trigger' AND tbl_name=?`, table).Scan(&triggerCount); err != nil {
		return mapDBErrorV1(ctx, err, false)
	}
	if triggerCount != 0 {
		return core.NewError(core.ErrorConflict, core.ReasonUnknownSchema, "Tool SQLite table "+table+" has unexpected triggers")
	}
	var source string
	if err = query.QueryRowContext(ctx, `SELECT sql FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&source); err != nil {
		return mapDBErrorV1(ctx, err, false)
	}
	expectedDDL, tokenErr := createTableTokensV2(expectedSchema, table)
	actualDDL, actualErr := createTableTokensV2(source, table)
	if tokenErr != nil || actualErr != nil || !reflectSchemaTokensV2(actualDDL, expectedDDL) {
		return core.NewError(core.ErrorConflict, core.ReasonUnknownSchema, "Tool SQLite table "+table+" DDL drifted")
	}
	return nil
}

func verifySchemaSequenceV2(scope string, actual, expected int) error {
	if actual != expected {
		return core.NewError(core.ErrorConflict, core.ReasonUnknownSchema, "Tool SQLite "+scope+" sequence drifted")
	}
	return nil
}

func verifyIndexXInfoRowV2(table string, cid int, name sql.NullString, descending int, collation sql.NullString, key int) (string, bool, error) {
	switch key {
	case 0:
		if cid != -1 || name.Valid || descending != 0 || !collation.Valid || collation.String != "BINARY" {
			return "", false, core.NewError(core.ErrorConflict, core.ReasonUnknownSchema, "Tool SQLite table "+table+" index auxiliary shape drifted")
		}
		return "", false, nil
	case 1:
		if cid < 0 || !name.Valid || descending != 0 || !collation.Valid || collation.String != "BINARY" {
			return "", false, core.NewError(core.ErrorConflict, core.ReasonUnknownSchema, "Tool SQLite table "+table+" index term shape drifted")
		}
		return name.String, true, nil
	default:
		return "", false, core.NewError(core.ErrorConflict, core.ReasonUnknownSchema, "Tool SQLite table "+table+" index key flag drifted")
	}
}

func createTableTokensV2(source, table string) ([]string, error) {
	tokens, err := schemaTokensV2(source)
	if err != nil {
		return nil, err
	}
	for start := 0; start < len(tokens); start++ {
		if tokens[start] != "word:create" || start+2 >= len(tokens) || tokens[start+1] != "word:table" {
			continue
		}
		nameIndex := start + 2
		if nameIndex+2 < len(tokens) && tokens[nameIndex] == "word:if" && tokens[nameIndex+1] == "word:not" && tokens[nameIndex+2] == "word:exists" {
			nameIndex += 3
		}
		if nameIndex >= len(tokens) || (tokens[nameIndex] != "word:"+strings.ToLower(table) && tokens[nameIndex] != "quoted-ident:"+table) {
			continue
		}
		end := nameIndex + 1
		for end < len(tokens) && tokens[end] != "punct:;" {
			end++
		}
		statement := []string{"word:create", "word:table"}
		statement = append(statement, tokens[nameIndex:end]...)
		return statement, nil
	}
	return nil, fmt.Errorf("SQLite CREATE TABLE %s not found", table)
}

func reflectSchemaTokensV2(left, right []string) bool {
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

func schemaTokensV2(value string) ([]string, error) {
	var tokens []string
	for index := 0; index < len(value); {
		switch {
		case value[index] == ' ' || value[index] == '\t' || value[index] == '\n' || value[index] == '\r':
			index++
		case value[index] == '-' && index+1 < len(value) && value[index+1] == '-':
			index += 2
			for index < len(value) && value[index] != '\n' {
				index++
			}
		case value[index] == '/' && index+1 < len(value) && value[index+1] == '*':
			index += 2
			for index+1 < len(value) && !(value[index] == '*' && value[index+1] == '/') {
				index++
			}
			if index+1 >= len(value) {
				return nil, fmt.Errorf("unterminated SQLite block comment")
			}
			index += 2
		case value[index] == '\'':
			token, next, err := schemaQuotedTokenV2(value, index, '\'', "string:")
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, token)
			index = next
		case value[index] == '"' || value[index] == '`':
			token, next, err := schemaQuotedTokenV2(value, index, value[index], "quoted-ident:")
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, token)
			index = next
		case value[index] == '[':
			end := index + 1
			for end < len(value) && value[end] != ']' {
				end++
			}
			if end >= len(value) {
				return nil, fmt.Errorf("unterminated SQLite bracket identifier")
			}
			tokens = append(tokens, "quoted-ident:"+value[index+1:end])
			index = end + 1
		case isSchemaWordByteV2(value[index]):
			end := index + 1
			for end < len(value) && isSchemaWordByteV2(value[end]) {
				end++
			}
			tokens = append(tokens, "word:"+strings.ToLower(value[index:end]))
			index = end
		default:
			tokens = append(tokens, "punct:"+value[index:index+1])
			index++
		}
	}
	return tokens, nil
}

func schemaQuotedTokenV2(value string, start int, delimiter byte, prefix string) (string, int, error) {
	var decoded strings.Builder
	for index := start + 1; index < len(value); index++ {
		if value[index] != delimiter {
			decoded.WriteByte(value[index])
			continue
		}
		if index+1 < len(value) && value[index+1] == delimiter {
			decoded.WriteByte(delimiter)
			index++
			continue
		}
		return prefix + decoded.String(), index + 1, nil
	}
	return "", 0, fmt.Errorf("unterminated SQLite quoted token")
}

func isSchemaWordByteV2(value byte) bool {
	return value == '_' || value >= '0' && value <= '9' || value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z'
}

func verifyStageCheckBehaviorV2(ctx context.Context, query schemaQueryerV2) error {
	const savepoint = "tool_action_stage_check_v2"
	if _, err := query.ExecContext(ctx, `SAVEPOINT `+savepoint); err != nil {
		return mapDBErrorV1(ctx, err, true)
	}
	closed := false
	defer func() {
		if !closed {
			recoveryCtx, cancel := boundedSQLiteRecoveryContextV2(ctx)
			_, _ = query.ExecContext(recoveryCtx, `ROLLBACK TO `+savepoint)
			_, _ = query.ExecContext(recoveryCtx, `RELEASE `+savepoint)
			cancel()
		}
	}()
	_, insertErr := query.ExecContext(ctx, `INSERT INTO tool_action_head_v2(
action_id,head_revision,head_digest,stage,candidate_id,candidate_revision,candidate_digest,updated_unix_nano,row_digest
) SELECT lower(hex(randomblob(16))),1,lower(hex(randomblob(32))),'schema-proof-invalid-stage',
lower(hex(randomblob(16))),1,lower(hex(randomblob(32))),1,lower(hex(randomblob(32)))`)
	var sqliteErr interface{ Code() int }
	if insertErr == nil || !errors.As(insertErr, &sqliteErr) || sqliteErr.Code() != 275 {
		return core.NewError(core.ErrorConflict, core.ReasonUnknownSchema, "Tool SQLite action head stage CHECK behavior drifted")
	}
	if _, err := query.ExecContext(ctx, `ROLLBACK TO `+savepoint); err != nil {
		return mapDBErrorV1(ctx, err, true)
	}
	if _, err := query.ExecContext(ctx, `RELEASE `+savepoint); err != nil {
		return mapDBErrorV1(ctx, err, true)
	}
	closed = true
	return nil
}

func colsV2(spec ...string) []schemaColumnV2 {
	out := make([]schemaColumnV2, 0, len(spec))
	for _, item := range spec {
		parts := strings.Split(item, ":")
		column := schemaColumnV2{Name: parts[0], Type: strings.ToUpper(parts[1]), NotNull: 1}
		if len(parts) == 3 {
			if parts[2] == "nullable" {
				column.NotNull = 0
			} else {
				_, _ = fmt.Sscanf(parts[2], "%d", &column.PK)
			}
		}
		if len(parts) == 4 && parts[3] == "nullable" {
			_, _ = fmt.Sscanf(parts[2], "%d", &column.PK)
			column.NotNull = 0
		}
		out = append(out, column)
	}
	return out
}

func verifyActionFactSchemaV2(ctx context.Context, query schemaQueryerV2) error {
	fact := func(table string, extras ...string) error {
		columns := []string{"fact_id:TEXT:1", "revision:INTEGER", "digest:TEXT", "action_id:TEXT"}
		columns = append(columns, extras...)
		columns = append(columns, "body_json:BLOB", "row_digest:TEXT")
		return verifyStrictTableV2(ctx, query, table, colsV2(columns...), [][]string{{"fact_id"}, {"fact_id", "revision", "digest"}, {"action_id"}}, actionFactSchemaV2)
	}
	if err := verifyStrictTableV2(ctx, query, "tool_action_fact_schema_v2", colsV2("version:INTEGER:1:nullable", "digest:TEXT", "applied_unix_nano:INTEGER"), [][]string{{"version"}}, actionFactSchemaV2); err != nil {
		return err
	}
	for _, table := range []string{"tool_action_candidate_v2", "tool_action_reservation_v2", "tool_domain_result_v2", "tool_apply_settlement_v2"} {
		if err := fact(table); err != nil {
			return err
		}
	}
	if err := verifyStrictTableV2(ctx, query, "tool_result_v2", colsV2("fact_id:TEXT:1", "revision:INTEGER", "digest:TEXT", "action_id:TEXT", "apply_id:TEXT", "body_json:BLOB", "row_digest:TEXT"), [][]string{{"fact_id"}, {"fact_id", "revision", "digest"}, {"action_id"}, {"apply_id"}}, actionFactSchemaV2); err != nil {
		return err
	}
	headColumns := colsV2("action_id:TEXT:1", "head_revision:INTEGER", "head_digest:TEXT", "stage:TEXT", "candidate_id:TEXT", "candidate_revision:INTEGER", "candidate_digest:TEXT", "reservation_id:TEXT:nullable", "reservation_revision:INTEGER:nullable", "reservation_digest:TEXT:nullable", "domain_result_id:TEXT:nullable", "domain_result_revision:INTEGER:nullable", "domain_result_digest:TEXT:nullable", "apply_id:TEXT:nullable", "apply_revision:INTEGER:nullable", "apply_digest:TEXT:nullable", "result_id:TEXT:nullable", "result_revision:INTEGER:nullable", "result_digest:TEXT:nullable", "updated_unix_nano:INTEGER", "row_digest:TEXT")
	if err := verifyStrictTableV2(ctx, query, "tool_action_head_v2", headColumns, [][]string{{"action_id"}}, actionFactSchemaV2); err != nil {
		return err
	}
	return verifyStageCheckBehaviorV2(ctx, query)
}

func verifyOwnerClaimExecutionSchemaV2(ctx context.Context, query schemaQueryerV2) error {
	if err := verifyStrictTableV2(ctx, query, "tool_owner_claim_execution_schema_v2", colsV2("version:INTEGER:1:nullable", "digest:TEXT", "applied_unix_nano:INTEGER"), [][]string{{"version"}}, ownerClaimExecutionSchemaV2); err != nil {
		return err
	}
	claimColumns := colsV2("claim_id:TEXT:1", "claim_revision:INTEGER", "claim_digest:TEXT", "request_id:TEXT", "request_revision:INTEGER", "request_digest:TEXT", "action_coordinate_digest:TEXT", "execution_scope_digest:TEXT", "binding_id:TEXT", "binding_revision:INTEGER", "binding_digest:TEXT", "input_digest:TEXT", "claim_json:BLOB", "input_json:BLOB", "row_digest:TEXT")
	if err := verifyStrictTableV2(ctx, query, "tool_owner_single_call_claim_v2", claimColumns, [][]string{{"claim_id"}, {"claim_id", "claim_revision", "claim_digest"}, {"request_id", "request_digest", "action_coordinate_digest", "execution_scope_digest"}}, ownerClaimExecutionSchemaV2); err != nil {
		return err
	}
	historyColumns := colsV2("state_id:TEXT:1", "state_revision:INTEGER:2", "state_digest:TEXT", "request_key_digest:TEXT", "state_json:BLOB", "row_digest:TEXT")
	if err := verifyStrictTableV2(ctx, query, "tool_owner_execution_history_v2", historyColumns, [][]string{{"state_id", "state_revision"}, {"state_id", "state_revision", "state_digest"}}, ownerClaimExecutionSchemaV2); err != nil {
		return err
	}
	headColumns := colsV2("request_key_digest:TEXT:1", "request_id:TEXT", "request_digest:TEXT", "action_coordinate_digest:TEXT", "execution_scope_digest:TEXT", "binding_id:TEXT", "binding_revision:INTEGER", "binding_digest:TEXT", "input_digest:TEXT", "state_id:TEXT", "state_revision:INTEGER", "state_digest:TEXT")
	if err := verifyStrictTableV2(ctx, query, "tool_owner_execution_head_v2", headColumns, [][]string{{"request_key_digest"}, {"state_id"}}, ownerClaimExecutionSchemaV2); err != nil {
		return err
	}
	if err := verifyForeignKeyV2(ctx, query, "tool_owner_execution_head_v2", "tool_owner_execution_history_v2", []string{"state_id", "state_revision", "state_digest"}, []string{"state_id", "state_revision", "state_digest"}, "NO ACTION", "RESTRICT"); err != nil {
		return err
	}
	leaseHistoryColumns := colsV2("lease_id:TEXT:1", "lease_revision:INTEGER:2", "lease_digest:TEXT", "execution_attempt_id:TEXT", "request_key_digest:TEXT", "request_digest:TEXT", "input_digest:TEXT", "holder_incarnation_id:TEXT", "phase:TEXT", "acquired_unix_nano:INTEGER", "expires_unix_nano:INTEGER", "lease_json:BLOB", "row_digest:TEXT")
	if err := verifyStrictTableV2(ctx, query, "tool_owner_entry_lease_history_v2", leaseHistoryColumns, [][]string{{"lease_id", "lease_revision"}, {"lease_id", "lease_revision", "lease_digest"}}, ownerClaimExecutionSchemaV2); err != nil {
		return err
	}
	leaseHeadColumns := colsV2("execution_attempt_id:TEXT:1", "lease_id:TEXT", "lease_revision:INTEGER", "lease_digest:TEXT")
	if err := verifyStrictTableV2(ctx, query, "tool_owner_entry_lease_head_v2", leaseHeadColumns, [][]string{{"execution_attempt_id"}, {"lease_id"}}, ownerClaimExecutionSchemaV2); err != nil {
		return err
	}
	if err := verifyForeignKeyV2(ctx, query, "tool_owner_entry_lease_head_v2", "tool_owner_entry_lease_history_v2", []string{"lease_id", "lease_revision", "lease_digest"}, []string{"lease_id", "lease_revision", "lease_digest"}, "NO ACTION", "RESTRICT"); err != nil {
		return err
	}
	return verifyEntryLeasePhaseCheckBehaviorV2(ctx, query)
}

func verifyEntryLeasePhaseCheckBehaviorV2(ctx context.Context, query schemaQueryerV2) error {
	const savepoint = "tool_owner_entry_lease_phase_check_v2"
	if _, err := query.ExecContext(ctx, `SAVEPOINT `+savepoint); err != nil {
		return mapDBErrorV1(ctx, err, true)
	}
	closed := false
	defer func() {
		if !closed {
			recoveryCtx, cancel := boundedSQLiteRecoveryContextV2(ctx)
			_, _ = query.ExecContext(recoveryCtx, `ROLLBACK TO `+savepoint)
			_, _ = query.ExecContext(recoveryCtx, `RELEASE `+savepoint)
			cancel()
		}
	}()
	_, insertErr := query.ExecContext(ctx, `INSERT INTO tool_owner_entry_lease_history_v2(
lease_id,lease_revision,lease_digest,execution_attempt_id,request_key_digest,request_digest,input_digest,
holder_incarnation_id,phase,acquired_unix_nano,expires_unix_nano,lease_json,row_digest
) VALUES('schema-proof',1,'schema-proof-digest','schema-proof-attempt','schema-proof-key','schema-proof-request',
'schema-proof-input','schema-proof-holder','invalid-phase',1,2,X'7B7D','schema-proof-row')`)
	var sqliteErr interface{ Code() int }
	if insertErr == nil || !errors.As(insertErr, &sqliteErr) || sqliteErr.Code() != 275 {
		return core.NewError(core.ErrorConflict, core.ReasonUnknownSchema, "Tool SQLite entry lease phase CHECK behavior drifted")
	}
	if _, err := query.ExecContext(ctx, `ROLLBACK TO `+savepoint); err != nil {
		return mapDBErrorV1(ctx, err, true)
	}
	if _, err := query.ExecContext(ctx, `RELEASE `+savepoint); err != nil {
		return mapDBErrorV1(ctx, err, true)
	}
	closed = true
	return nil
}

func verifyForeignKeyV2(ctx context.Context, query schemaQueryerV2, table, target string, from, to []string, onUpdate, onDelete string) error {
	rows, err := query.QueryContext(ctx, `SELECT id,seq,"table","from","to",on_update,on_delete FROM pragma_foreign_key_list(?) ORDER BY id,seq`, table)
	if err != nil {
		return mapDBErrorV1(ctx, err, false)
	}
	type edge struct {
		seq      int
		target   string
		from     string
		to       string
		onUpdate string
		onDelete string
	}
	groups := make(map[int][]edge)
	for rows.Next() {
		var id int
		var value edge
		if err = rows.Scan(&id, &value.seq, &value.target, &value.from, &value.to, &value.onUpdate, &value.onDelete); err != nil {
			_ = rows.Close()
			return mapDBErrorV1(ctx, err, false)
		}
		groups[id] = append(groups[id], value)
	}
	if err = rows.Err(); err != nil {
		_ = rows.Close()
		return mapDBErrorV1(ctx, err, false)
	}
	if err = rows.Close(); err != nil {
		return mapDBErrorV1(ctx, err, false)
	}
	if len(groups) != 1 {
		return core.NewError(core.ErrorConflict, core.ReasonUnknownSchema, "Tool SQLite table "+table+" foreign-key set drifted")
	}
	for _, edges := range groups {
		if len(edges) != len(from) {
			return core.NewError(core.ErrorConflict, core.ReasonUnknownSchema, "Tool SQLite table "+table+" foreign-key width drifted")
		}
		sort.Slice(edges, func(i, j int) bool { return edges[i].seq < edges[j].seq })
		for index, value := range edges {
			if value.seq != index || value.target != target || value.from != from[index] || value.to != to[index] || value.onUpdate != onUpdate || value.onDelete != onDelete {
				return core.NewError(core.ErrorConflict, core.ReasonUnknownSchema, "Tool SQLite table "+table+" foreign-key columns drifted")
			}
		}
	}
	return nil
}
