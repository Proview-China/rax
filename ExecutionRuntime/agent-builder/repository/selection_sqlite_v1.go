package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-builder/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-builder/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	_ "modernc.org/sqlite"
)

const (
	selectionSchemaLedgerCreateV1 = "CREATE TABLE IF NOT EXISTS agent_package_selection_schema_v1(version INTEGER PRIMARY KEY,digest TEXT NOT NULL,applied_unix_nano INTEGER NOT NULL)"
	selectionHistoryCreateV1      = "CREATE TABLE IF NOT EXISTS agent_package_selection_history_v1(selection_id TEXT NOT NULL,revision INTEGER NOT NULL,digest TEXT NOT NULL,row_digest TEXT NOT NULL,payload_json BLOB NOT NULL,checked_unix_nano INTEGER NOT NULL,expires_unix_nano INTEGER NOT NULL,PRIMARY KEY(selection_id,revision),UNIQUE(selection_id,revision,digest))"
	selectionCurrentCreateV1      = "CREATE TABLE IF NOT EXISTS agent_package_selection_current_v1(selection_id TEXT PRIMARY KEY,revision INTEGER NOT NULL,digest TEXT NOT NULL,row_digest TEXT NOT NULL,FOREIGN KEY(selection_id,revision,digest) REFERENCES agent_package_selection_history_v1(selection_id,revision,digest))"
	selectionSchemaV1             = selectionSchemaLedgerCreateV1 + ";" + selectionHistoryCreateV1 + ";" + selectionCurrentCreateV1 + ";"
)

type SelectionSQLiteConfigV1 struct {
	Path         string
	BusyTimeout  time.Duration
	MaxOpenConns int
	Clock        func() time.Time
}

type SelectionSQLiteV1 struct {
	db       *sql.DB
	mu       *sync.Mutex
	clock    func() time.Time
	faultMu  sync.Mutex
	loseNext bool
}

var _ ports.AgentPackageSelectionCurrentRepositoryV1 = (*SelectionSQLiteV1)(nil)
var selectionLocksV1 sync.Map

func OpenSelectionSQLiteV1(ctx context.Context, config SelectionSQLiteConfigV1) (*SelectionSQLiteV1, error) {
	if ctx == nil || ctx.Err() != nil || strings.TrimSpace(config.Path) == "" {
		return nil, selectionInvalidV1("package selection SQLite open requires live context and path")
	}
	if config.BusyTimeout <= 0 {
		config.BusyTimeout = 5 * time.Second
	}
	if config.BusyTimeout > time.Minute {
		return nil, selectionInvalidV1("package selection SQLite busy timeout exceeds one minute")
	}
	if config.MaxOpenConns <= 0 {
		config.MaxOpenConns = 4
	}
	if config.MaxOpenConns > 32 {
		return nil, selectionInvalidV1("package selection SQLite connection count exceeds 32")
	}
	if config.Clock == nil {
		config.Clock = time.Now
	}
	absolute, err := filepath.Abs(config.Path)
	if err != nil {
		return nil, selectionInvalidV1("package selection SQLite path is invalid")
	}
	lock, _ := selectionLocksV1.LoadOrStore(absolute, &sync.Mutex{})
	dsn := "file:" + filepath.ToSlash(absolute) +
		fmt.Sprintf("?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(%d)&_pragma=synchronous(FULL)&_txlock=immediate", config.BusyTimeout.Milliseconds())
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, selectionDBErrorV1(ctx, err, false)
	}
	db.SetMaxOpenConns(config.MaxOpenConns)
	db.SetMaxIdleConns(config.MaxOpenConns)
	store := &SelectionSQLiteV1{db: db, mu: lock.(*sync.Mutex), clock: config.Clock}
	if err = store.migrateV1(ctx); err == nil {
		err = store.verifyV1(ctx)
	}
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (store *SelectionSQLiteV1) Close() error {
	if store == nil || store.db == nil {
		return nil
	}
	return store.db.Close()
}

// LoseNextAppendReplyV1 is a bounded fault hook used only to verify that
// unknown outcomes recover by exact Inspect without repeating the CAS.
func (store *SelectionSQLiteV1) LoseNextAppendReplyV1() {
	if store == nil {
		return
	}
	store.faultMu.Lock()
	store.loseNext = true
	store.faultMu.Unlock()
}

func (store *SelectionSQLiteV1) takeLostReplyV1() bool {
	store.faultMu.Lock()
	defer store.faultMu.Unlock()
	value := store.loseNext
	store.loseNext = false
	return value
}

func (store *SelectionSQLiteV1) migrateV1(ctx context.Context) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return selectionDBErrorV1(ctx, err, true)
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, selectionSchemaV1); err != nil {
		return selectionDBErrorV1(ctx, err, true)
	}
	now := store.clock()
	if now.IsZero() || now.UnixNano() <= 0 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "package selection SQLite migration clock invalid")
	}
	digest := core.DigestBytes([]byte(selectionSchemaV1))
	if _, err = tx.ExecContext(
		ctx,
		"INSERT OR IGNORE INTO agent_package_selection_schema_v1(version,digest,applied_unix_nano) VALUES(1,?,?)",
		string(digest),
		now.UnixNano(),
	); err != nil {
		return selectionDBErrorV1(ctx, err, true)
	}
	var storedDigest string
	var count int
	if err = tx.QueryRowContext(ctx, "SELECT digest FROM agent_package_selection_schema_v1 WHERE version=1").Scan(&storedDigest); err != nil {
		return selectionDBErrorV1(ctx, err, false)
	}
	if err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM agent_package_selection_schema_v1").Scan(&count); err != nil {
		return selectionDBErrorV1(ctx, err, false)
	}
	if storedDigest != string(digest) || count != 1 {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "package selection SQLite schema digest drifted")
	}
	if err = verifySelectionSchemaTxV1(ctx, tx); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return selectionIndeterminateV1("package selection SQLite migration commit outcome unknown")
	}
	return nil
}

func (store *SelectionSQLiteV1) verifyV1(ctx context.Context) error {
	var wal string
	var foreignKeys, synchronous int
	if err := store.db.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&wal); err != nil {
		return selectionDBErrorV1(ctx, err, false)
	}
	if err := store.db.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
		return selectionDBErrorV1(ctx, err, false)
	}
	if err := store.db.QueryRowContext(ctx, "PRAGMA synchronous").Scan(&synchronous); err != nil {
		return selectionDBErrorV1(ctx, err, false)
	}
	if !strings.EqualFold(wal, "wal") || foreignKeys != 1 || synchronous != 2 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "package selection SQLite WAL, foreign keys or FULL sync inactive")
	}
	return nil
}

type selectionSchemaColumnV1 struct {
	Name       string
	Type       string
	NotNull    int
	PrimaryKey int
}

type selectionForeignKeyV1 struct {
	Sequence int
	Table    string
	From     string
	To       string
	OnUpdate string
	OnDelete string
	Match    string
}

type selectionTableSchemaV1 struct {
	Name        string
	MasterSQL   string
	Columns     []selectionSchemaColumnV1
	Indexes     []string
	ForeignKeys []selectionForeignKeyV1
}

func verifySelectionSchemaTxV1(ctx context.Context, tx *sql.Tx) error {
	expected := []selectionTableSchemaV1{
		{
			Name:      "agent_package_selection_schema_v1",
			MasterSQL: strings.Replace(selectionSchemaLedgerCreateV1, " IF NOT EXISTS", "", 1),
			Columns: []selectionSchemaColumnV1{
				{Name: "version", Type: "INTEGER", PrimaryKey: 1},
				{Name: "digest", Type: "TEXT", NotNull: 1},
				{Name: "applied_unix_nano", Type: "INTEGER", NotNull: 1},
			},
		},
		{
			Name:      "agent_package_selection_history_v1",
			MasterSQL: strings.Replace(selectionHistoryCreateV1, " IF NOT EXISTS", "", 1),
			Columns: []selectionSchemaColumnV1{
				{Name: "selection_id", Type: "TEXT", NotNull: 1, PrimaryKey: 1},
				{Name: "revision", Type: "INTEGER", NotNull: 1, PrimaryKey: 2},
				{Name: "digest", Type: "TEXT", NotNull: 1},
				{Name: "row_digest", Type: "TEXT", NotNull: 1},
				{Name: "payload_json", Type: "BLOB", NotNull: 1},
				{Name: "checked_unix_nano", Type: "INTEGER", NotNull: 1},
				{Name: "expires_unix_nano", Type: "INTEGER", NotNull: 1},
			},
			Indexes: []string{
				"pk|selection_id,revision",
				"u|selection_id,revision,digest",
			},
		},
		{
			Name:      "agent_package_selection_current_v1",
			MasterSQL: strings.Replace(selectionCurrentCreateV1, " IF NOT EXISTS", "", 1),
			Columns: []selectionSchemaColumnV1{
				{Name: "selection_id", Type: "TEXT", PrimaryKey: 1},
				{Name: "revision", Type: "INTEGER", NotNull: 1},
				{Name: "digest", Type: "TEXT", NotNull: 1},
				{Name: "row_digest", Type: "TEXT", NotNull: 1},
			},
			Indexes: []string{"pk|selection_id"},
			ForeignKeys: []selectionForeignKeyV1{
				{Sequence: 0, Table: "agent_package_selection_history_v1", From: "selection_id", To: "selection_id", OnUpdate: "NO ACTION", OnDelete: "NO ACTION", Match: "NONE"},
				{Sequence: 1, Table: "agent_package_selection_history_v1", From: "revision", To: "revision", OnUpdate: "NO ACTION", OnDelete: "NO ACTION", Match: "NONE"},
				{Sequence: 2, Table: "agent_package_selection_history_v1", From: "digest", To: "digest", OnUpdate: "NO ACTION", OnDelete: "NO ACTION", Match: "NONE"},
			},
		},
	}
	for _, table := range expected {
		if err := verifySelectionTableV1(ctx, tx, table); err != nil {
			return err
		}
	}
	return nil
}

func verifySelectionTableV1(ctx context.Context, tx *sql.Tx, expected selectionTableSchemaV1) error {
	var objectType, masterSQL string
	if err := tx.QueryRowContext(
		ctx,
		"SELECT type,sql FROM sqlite_master WHERE name=?",
		expected.Name,
	).Scan(&objectType, &masterSQL); err != nil {
		return selectionSchemaDriftV1()
	}
	if objectType != "table" || masterSQL != expected.MasterSQL {
		return selectionSchemaDriftV1()
	}

	rows, err := tx.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%q)", expected.Name))
	if err != nil {
		return selectionSchemaDriftV1()
	}
	var columns []selectionSchemaColumnV1
	for rows.Next() {
		var cid int
		var column selectionSchemaColumnV1
		var defaultValue sql.NullString
		if err = rows.Scan(&cid, &column.Name, &column.Type, &column.NotNull, &defaultValue, &column.PrimaryKey); err != nil ||
			cid != len(columns) ||
			defaultValue.Valid {
			_ = rows.Close()
			return selectionSchemaDriftV1()
		}
		columns = append(columns, column)
	}
	if err = rows.Err(); err != nil {
		_ = rows.Close()
		return selectionSchemaDriftV1()
	}
	if err = rows.Close(); err != nil || !reflect.DeepEqual(columns, expected.Columns) {
		return selectionSchemaDriftV1()
	}

	indexes, err := selectionIndexSignaturesV1(ctx, tx, expected.Name)
	if err != nil {
		return selectionSchemaDriftV1()
	}
	sort.Strings(indexes)
	sort.Strings(expected.Indexes)
	if strings.Join(indexes, "\x00") != strings.Join(expected.Indexes, "\x00") {
		return selectionSchemaDriftV1()
	}

	foreignKeys, err := selectionForeignKeysV1(ctx, tx, expected.Name)
	if err != nil || !reflect.DeepEqual(foreignKeys, expected.ForeignKeys) {
		return selectionSchemaDriftV1()
	}
	return nil
}

func selectionIndexSignaturesV1(ctx context.Context, tx *sql.Tx, table string) ([]string, error) {
	rows, err := tx.QueryContext(ctx, fmt.Sprintf("PRAGMA index_list(%q)", table))
	if err != nil {
		return nil, err
	}
	type indexV1 struct {
		name   string
		origin string
	}
	var indexes []indexV1
	for rows.Next() {
		var sequence, unique, partial int
		var index indexV1
		if err = rows.Scan(&sequence, &index.name, &unique, &index.origin, &partial); err != nil ||
			sequence < 0 ||
			unique != 1 ||
			partial != 0 {
			_ = rows.Close()
			return nil, errors.New("invalid selection index")
		}
		indexes = append(indexes, index)
	}
	if err = rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err = rows.Close(); err != nil {
		return nil, err
	}
	result := make([]string, 0, len(indexes))
	for _, index := range indexes {
		info, queryErr := tx.QueryContext(ctx, fmt.Sprintf("PRAGMA index_info(%q)", index.name))
		if queryErr != nil {
			return nil, queryErr
		}
		var columns []string
		for info.Next() {
			var sequence, cid int
			var name string
			if queryErr = info.Scan(&sequence, &cid, &name); queryErr != nil ||
				sequence != len(columns) ||
				cid < 0 ||
				name == "" {
				_ = info.Close()
				return nil, errors.New("invalid selection index column")
			}
			columns = append(columns, name)
		}
		if queryErr = info.Err(); queryErr != nil {
			_ = info.Close()
			return nil, queryErr
		}
		if queryErr = info.Close(); queryErr != nil {
			return nil, queryErr
		}
		result = append(result, index.origin+"|"+strings.Join(columns, ","))
	}
	return result, nil
}

func selectionForeignKeysV1(ctx context.Context, tx *sql.Tx, table string) ([]selectionForeignKeyV1, error) {
	rows, err := tx.QueryContext(ctx, fmt.Sprintf("PRAGMA foreign_key_list(%q)", table))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []selectionForeignKeyV1
	for rows.Next() {
		var id int
		var value selectionForeignKeyV1
		if err = rows.Scan(
			&id,
			&value.Sequence,
			&value.Table,
			&value.From,
			&value.To,
			&value.OnUpdate,
			&value.OnDelete,
			&value.Match,
		); err != nil || id != 0 {
			return nil, errors.New("invalid selection foreign key")
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func selectionSchemaDriftV1() error {
	return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "package selection SQLite actual schema drifted")
}

func (store *SelectionSQLiteV1) IntegrityCheckV1(ctx context.Context) error {
	if store == nil || store.db == nil {
		return selectionInvalidV1("package selection SQLite store is nil")
	}
	var value string
	if err := store.db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&value); err != nil {
		return selectionDBErrorV1(ctx, err, false)
	}
	if value != "ok" {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidState, "package selection SQLite integrity check failed")
	}
	return nil
}

func (store *SelectionSQLiteV1) CompareAndSwapAgentPackageSelectionCurrentV1(
	ctx context.Context,
	expected contract.AgentPackageSelectionCurrentRefV1,
	current contract.AgentPackageSelectionCurrentV1,
) (contract.AgentPackageSelectionCurrentV1, error) {
	if store == nil || store.db == nil {
		return contract.AgentPackageSelectionCurrentV1{}, selectionInvalidV1("package selection SQLite store is nil")
	}
	if ctx == nil || ctx.Err() != nil {
		return contract.AgentPackageSelectionCurrentV1{}, selectionIndeterminateV1("package selection append requires live context")
	}
	if err := validateSelectionCASV1(current, expected); err != nil {
		return contract.AgentPackageSelectionCurrentV1{}, err
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return contract.AgentPackageSelectionCurrentV1{}, selectionDBErrorV1(ctx, err, true)
	}
	defer tx.Rollback()

	storedNext, storedNextErr := inspectSelectionExactQueryV1(ctx, tx, current.Ref)
	if storedNextErr == nil {
		if !reflect.DeepEqual(storedNext, current) {
			return contract.AgentPackageSelectionCurrentV1{}, selectionConflictV1("package selection replay body drifted")
		}
		if !expected.IsZero() {
			if _, predecessorErr := inspectSelectionExactQueryV1(ctx, tx, expected); predecessorErr != nil {
				if core.HasReason(predecessorErr, core.ReasonBindingDrift) {
					return contract.AgentPackageSelectionCurrentV1{}, selectionConflictV1("package selection replay expected another exact predecessor")
				}
				return contract.AgentPackageSelectionCurrentV1{}, predecessorErr
			}
		}
		return contract.CloneAgentPackageSelectionCurrentV1(storedNext), nil
	}
	if core.HasReason(storedNextErr, core.ReasonBindingDrift) {
		return contract.AgentPackageSelectionCurrentV1{}, selectionConflictV1("package selection next coordinate already carries another exact body")
	}
	if !core.HasCategory(storedNextErr, core.ErrorNotFound) {
		return contract.AgentPackageSelectionCurrentV1{}, storedNextErr
	}

	now := store.clock()
	if err := current.ValidateCurrent(current.Ref, now); err != nil {
		return contract.AgentPackageSelectionCurrentV1{}, err
	}
	actual, actualErr := inspectSelectionCurrentPointerQueryV1(ctx, tx, current.Ref.SelectionID)
	switch {
	case expected.IsZero() && actualErr == nil:
		return contract.AgentPackageSelectionCurrentV1{}, selectionConflictV1("package selection current already exists")
	case expected.IsZero() && !core.HasCategory(actualErr, core.ErrorNotFound):
		return contract.AgentPackageSelectionCurrentV1{}, actualErr
	case !expected.IsZero() && actualErr != nil:
		return contract.AgentPackageSelectionCurrentV1{}, actualErr
	case !expected.IsZero() && actual.Ref != expected:
		return contract.AgentPackageSelectionCurrentV1{}, selectionConflictV1("package selection current predecessor changed")
	}
	var predecessor contract.AgentPackageSelectionCurrentV1
	if !expected.IsZero() {
		var inspectErr error
		predecessor, inspectErr = inspectSelectionExactQueryV1(ctx, tx, expected)
		if inspectErr != nil {
			return contract.AgentPackageSelectionCurrentV1{}, inspectErr
		}
		if inspectErr = predecessor.ValidateCurrent(expected, now); inspectErr != nil {
			return contract.AgentPackageSelectionCurrentV1{}, inspectErr
		}
		if current.CheckedUnixNano < predecessor.CheckedUnixNano {
			return contract.AgentPackageSelectionCurrentV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "package selection current checked time regressed")
		}
	}

	raw, rowDigest, err := encodeSelectionCurrentV1(current)
	if err != nil {
		return contract.AgentPackageSelectionCurrentV1{}, err
	}
	if _, err = tx.ExecContext(
		ctx,
		"INSERT INTO agent_package_selection_history_v1(selection_id,revision,digest,row_digest,payload_json,checked_unix_nano,expires_unix_nano) VALUES(?,?,?,?,?,?,?)",
		current.Ref.SelectionID,
		uint64(current.Ref.Revision),
		string(current.ProjectionDigest),
		string(rowDigest),
		raw,
		current.CheckedUnixNano,
		current.ExpiresUnixNano,
	); err != nil {
		return contract.AgentPackageSelectionCurrentV1{}, selectionDBErrorV1(ctx, err, true)
	}
	nextPointerRowDigest, err := selectionPointerRowDigestV1(current.Ref)
	if err != nil {
		return contract.AgentPackageSelectionCurrentV1{}, err
	}

	if expected.IsZero() {
		if _, err = tx.ExecContext(
			ctx,
			"INSERT INTO agent_package_selection_current_v1(selection_id,revision,digest,row_digest) VALUES(?,?,?,?)",
			current.Ref.SelectionID,
			uint64(current.Ref.Revision),
			string(current.ProjectionDigest),
			string(nextPointerRowDigest),
		); err != nil {
			return contract.AgentPackageSelectionCurrentV1{}, selectionDBErrorV1(ctx, err, true)
		}
	} else {
		result, updateErr := tx.ExecContext(
			ctx,
			"UPDATE agent_package_selection_current_v1 SET revision=?,digest=?,row_digest=? WHERE selection_id=? AND revision=? AND digest=? AND row_digest=?",
			uint64(current.Ref.Revision),
			string(current.ProjectionDigest),
			string(nextPointerRowDigest),
			expected.SelectionID,
			uint64(expected.Revision),
			string(expected.Digest),
			string(actual.RowDigest),
		)
		if updateErr != nil {
			return contract.AgentPackageSelectionCurrentV1{}, selectionDBErrorV1(ctx, updateErr, true)
		}
		affected, affectedErr := result.RowsAffected()
		if affectedErr != nil {
			return contract.AgentPackageSelectionCurrentV1{}, selectionDBErrorV1(ctx, affectedErr, true)
		}
		if affected != 1 {
			return contract.AgentPackageSelectionCurrentV1{}, selectionConflictV1("package selection current CAS lost")
		}
	}
	commitNow := store.clock()
	if commitNow.IsZero() || commitNow.Before(now) {
		return contract.AgentPackageSelectionCurrentV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "package selection SQLite clock regressed before commit")
	}
	if err = current.ValidateCurrent(current.Ref, commitNow); err != nil {
		return contract.AgentPackageSelectionCurrentV1{}, err
	}
	if !expected.IsZero() {
		if err = predecessor.ValidateCurrent(expected, commitNow); err != nil {
			return contract.AgentPackageSelectionCurrentV1{}, err
		}
	}
	if err = tx.Commit(); err != nil {
		return contract.AgentPackageSelectionCurrentV1{}, selectionIndeterminateV1("package selection current commit outcome unknown")
	}
	if store.takeLostReplyV1() {
		return contract.AgentPackageSelectionCurrentV1{}, selectionIndeterminateV1("package selection current reply lost after commit")
	}
	return contract.CloneAgentPackageSelectionCurrentV1(current), nil
}

func (store *SelectionSQLiteV1) InspectAgentPackageSelectionExactV1(
	ctx context.Context,
	ref contract.AgentPackageSelectionCurrentRefV1,
) (contract.AgentPackageSelectionCurrentV1, error) {
	if store == nil || store.db == nil {
		return contract.AgentPackageSelectionCurrentV1{}, selectionInvalidV1("package selection SQLite store is nil")
	}
	if ctx == nil || ctx.Err() != nil {
		return contract.AgentPackageSelectionCurrentV1{}, selectionUnavailableV1("package selection exact Inspect requires live context")
	}
	if err := ref.Validate(); err != nil {
		return contract.AgentPackageSelectionCurrentV1{}, err
	}
	return inspectSelectionExactQueryV1(
		ctx,
		store.db,
		ref,
	)
}

func (store *SelectionSQLiteV1) InspectAgentPackageSelectionCurrentV1(ctx context.Context, selectionID string) (contract.AgentPackageSelectionCurrentV1, error) {
	if store == nil || store.db == nil {
		return contract.AgentPackageSelectionCurrentV1{}, selectionInvalidV1("package selection SQLite store is nil")
	}
	if ctx == nil || ctx.Err() != nil {
		return contract.AgentPackageSelectionCurrentV1{}, selectionUnavailableV1("package selection current Inspect requires live context")
	}
	if err := contract.ValidateAgentPackageSelectionIDV1(selectionID); err != nil {
		return contract.AgentPackageSelectionCurrentV1{}, err
	}
	pointer, err := inspectSelectionCurrentPointerQueryV1(ctx, store.db, selectionID)
	if err != nil {
		return contract.AgentPackageSelectionCurrentV1{}, err
	}
	value, err := inspectSelectionExactQueryV1(ctx, store.db, pointer.Ref)
	if err != nil {
		return contract.AgentPackageSelectionCurrentV1{}, err
	}
	now := store.clock()
	if err = value.ValidateCurrent(pointer.Ref, now); err != nil {
		return contract.AgentPackageSelectionCurrentV1{}, err
	}
	return value, nil
}

func validateSelectionCASV1(
	current contract.AgentPackageSelectionCurrentV1,
	expected contract.AgentPackageSelectionCurrentRefV1,
) error {
	if err := current.Validate(); err != nil {
		return err
	}
	if expected.IsZero() {
		if current.Ref.Revision != 1 {
			return selectionConflictV1("package selection create requires zero expected and revision one")
		}
		return nil
	}
	if err := expected.Validate(); err != nil {
		return err
	}
	if current.Ref.SelectionID != expected.SelectionID || current.Ref.Revision != expected.Revision+1 {
		return selectionConflictV1("package selection advance requires exact predecessor and revision plus one")
	}
	return nil
}

type selectionCurrentPointerV1 struct {
	Ref       contract.AgentPackageSelectionCurrentRefV1
	RowDigest core.Digest
}

func inspectSelectionCurrentPointerQueryV1(ctx context.Context, query selectionQueryV1, selectionID string) (selectionCurrentPointerV1, error) {
	var revision uint64
	var digest, rowDigest string
	var expires int64
	var maxRevision uint64
	err := query.QueryRowContext(
		ctx,
		"SELECT c.revision,c.digest,c.row_digest,h.expires_unix_nano,(SELECT MAX(m.revision) FROM agent_package_selection_history_v1 m WHERE m.selection_id=c.selection_id) FROM agent_package_selection_current_v1 c JOIN agent_package_selection_history_v1 h ON h.selection_id=c.selection_id AND h.revision=c.revision AND h.digest=c.digest WHERE c.selection_id=?",
		selectionID,
	).Scan(&revision, &digest, &rowDigest, &expires, &maxRevision)
	if errors.Is(err, sql.ErrNoRows) {
		return selectionCurrentPointerV1{}, selectionNotFoundV1()
	}
	if err != nil {
		return selectionCurrentPointerV1{}, selectionDBErrorV1(ctx, err, false)
	}
	ref := contract.AgentPackageSelectionCurrentRefV1{
		SelectionID:     selectionID,
		Revision:        core.Revision(revision),
		Digest:          core.Digest(digest),
		ExpiresUnixNano: expires,
	}
	wantRowDigest, digestErr := selectionPointerRowDigestV1(ref)
	if digestErr != nil || wantRowDigest != core.Digest(rowDigest) {
		return selectionCurrentPointerV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "package selection current pointer row digest drifted")
	}
	if revision != maxRevision {
		return selectionCurrentPointerV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "package selection current pointer rolled back behind history")
	}
	return selectionCurrentPointerV1{Ref: ref, RowDigest: core.Digest(rowDigest)}, nil
}

type selectionQueryV1 interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func inspectSelectionExactQueryV1(
	ctx context.Context,
	query selectionQueryV1,
	ref contract.AgentPackageSelectionCurrentRefV1,
) (contract.AgentPackageSelectionCurrentV1, error) {
	var raw []byte
	var storedDigest, storedRowDigest string
	var checked, expires int64
	err := query.QueryRowContext(
		ctx,
		"SELECT digest,row_digest,payload_json,checked_unix_nano,expires_unix_nano FROM agent_package_selection_history_v1 WHERE selection_id=? AND revision=?",
		ref.SelectionID,
		uint64(ref.Revision),
	).Scan(&storedDigest, &storedRowDigest, &raw, &checked, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return contract.AgentPackageSelectionCurrentV1{}, selectionNotFoundV1()
	}
	if err != nil {
		return contract.AgentPackageSelectionCurrentV1{}, selectionDBErrorV1(ctx, err, false)
	}
	expectedRowDigest, err := selectionRowDigestV1(ref.SelectionID, ref.Revision, core.Digest(storedDigest), raw)
	if err != nil || expectedRowDigest != core.Digest(storedRowDigest) {
		return contract.AgentPackageSelectionCurrentV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "package selection SQLite row digest drifted")
	}
	value, err := strictSelectionJSONV1[contract.AgentPackageSelectionCurrentV1](raw)
	if err != nil ||
		value.Ref.SelectionID != ref.SelectionID ||
		value.Ref.Revision != ref.Revision ||
		value.ProjectionDigest != core.Digest(storedDigest) ||
		value.Ref.ExpiresUnixNano != ref.ExpiresUnixNano ||
		value.CheckedUnixNano != checked ||
		value.ExpiresUnixNano != expires ||
		value.Validate() != nil {
		return contract.AgentPackageSelectionCurrentV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "package selection SQLite payload drifted")
	}
	if value.RefV1() != ref {
		return contract.AgentPackageSelectionCurrentV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "package selection exact ref mismatch")
	}
	return contract.CloneAgentPackageSelectionCurrentV1(value), nil
}

type selectionRowDigestInputV1 struct {
	SelectionID   string
	Revision      core.Revision
	Digest        core.Digest
	PayloadDigest core.Digest
}

type selectionPointerRowDigestInputV1 struct {
	SelectionID string
	Revision    core.Revision
	Digest      core.Digest
}

func selectionPointerRowDigestV1(ref contract.AgentPackageSelectionCurrentRefV1) (core.Digest, error) {
	if err := ref.Validate(); err != nil {
		return "", err
	}
	return core.CanonicalJSONDigest(
		"praxis.agent-builder.selection-pointer-sqlite",
		"v1",
		"AgentPackageSelectionCurrentPointerRowV1",
		selectionPointerRowDigestInputV1{
			SelectionID: ref.SelectionID,
			Revision:    ref.Revision,
			Digest:      ref.Digest,
		},
	)
}

func encodeSelectionCurrentV1(value contract.AgentPackageSelectionCurrentV1) ([]byte, core.Digest, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, "", core.NewError(core.ErrorInternal, core.ReasonInvalidCanonicalForm, "package selection SQLite encode failed")
	}
	digest, err := selectionRowDigestV1(value.Ref.SelectionID, value.Ref.Revision, value.ProjectionDigest, raw)
	return raw, digest, err
}

func selectionRowDigestV1(selectionID string, revision core.Revision, digest core.Digest, raw []byte) (core.Digest, error) {
	return core.CanonicalJSONDigest(
		"praxis.agent-builder.selection-sqlite",
		"v1",
		"AgentPackageSelectionSQLiteRowV1",
		selectionRowDigestInputV1{selectionID, revision, digest, core.DigestBytes(raw)},
	)
}

func strictSelectionJSONV1[T any](raw []byte) (T, error) {
	var value T
	if err := core.DecodeStrictJSON(raw, &value); err != nil {
		return value, err
	}
	return value, nil
}

func selectionInvalidV1(message string) error {
	return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, message)
}
func selectionUnavailableV1(message string) error {
	return core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, message)
}
func selectionIndeterminateV1(message string) error {
	return core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, message)
}
func selectionNotFoundV1() error {
	return core.NewError(core.ErrorNotFound, core.ReasonEvidenceUnavailable, "package selection current not found")
}
func selectionConflictV1(message string) error {
	return core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, message)
}
func selectionDBErrorV1(ctx context.Context, err error, mutation bool) error {
	if err == nil {
		return nil
	}
	if ctx == nil || ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		if mutation {
			return selectionIndeterminateV1("package selection SQLite mutation outcome unknown")
		}
		return selectionUnavailableV1("package selection SQLite read unavailable")
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "locked") || strings.Contains(message, "busy") {
		return selectionUnavailableV1("package selection SQLite busy")
	}
	if strings.Contains(message, "constraint") || strings.Contains(message, "unique") {
		return selectionConflictV1("package selection SQLite CAS conflict")
	}
	if mutation {
		return selectionIndeterminateV1("package selection SQLite mutation outcome unknown")
	}
	return selectionUnavailableV1("package selection SQLite read failed")
}
