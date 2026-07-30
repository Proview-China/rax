package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-run-service/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-run-service/ports"
)

const schemaVersionV1 = "agent-run-service-command-journal/v1"

type ConfigV1 struct {
	Path        string
	BusyTimeout time.Duration
}

type JournalV1 struct{ db *sql.DB }

func OpenJournalV1(ctx context.Context, config ConfigV1) (*JournalV1, error) {
	if strings.TrimSpace(config.Path) == "" || config.Path != strings.TrimSpace(config.Path) {
		return nil, contract.NewError(contract.FaultInvalidArgumentV1, "sqlite_path_invalid", "command journal sqlite path is required")
	}
	if config.BusyTimeout <= 0 {
		config.BusyTimeout = 10 * time.Second
	}
	if config.BusyTimeout > time.Minute {
		return nil, contract.NewError(contract.FaultInvalidArgumentV1, "sqlite_busy_timeout_invalid", "command journal sqlite busy timeout exceeds one minute")
	}
	absolute, err := filepath.Abs(config.Path)
	if err != nil {
		return nil, contract.NewError(contract.FaultInvalidArgumentV1, "sqlite_path_invalid", "command journal sqlite path is invalid")
	}
	dsn := "file:" + filepath.ToSlash(absolute) + fmt.Sprintf("?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=synchronous(FULL)&_pragma=busy_timeout(%d)&_txlock=immediate", config.BusyTimeout.Milliseconds())
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, sqliteErrorV1(err, false)
	}
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(4)
	journal := &JournalV1{db: db}
	if err := journal.initializeV1(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return journal, nil
}

func (j *JournalV1) Close() error {
	if j == nil || j.db == nil {
		return nil
	}
	return j.db.Close()
}

func (j *JournalV1) initializeV1(ctx context.Context) error {
	if _, err := j.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS agent_run_service_schema (
			singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
			version TEXT NOT NULL
		);
		CREATE TABLE IF NOT EXISTS command_journal (
			idempotency_key TEXT PRIMARY KEY,
			command_id TEXT NOT NULL UNIQUE,
			command_json BLOB NOT NULL,
			receipt_json BLOB
		);
	`); err != nil {
		return sqliteErrorV1(err, true)
	}
	var version string
	err := j.db.QueryRowContext(ctx, "SELECT version FROM agent_run_service_schema WHERE singleton=1").Scan(&version)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		if _, err := j.db.ExecContext(ctx, "INSERT INTO agent_run_service_schema(singleton,version) VALUES(1,?)", schemaVersionV1); err != nil {
			return sqliteErrorV1(err, true)
		}
	case err != nil:
		return sqliteErrorV1(err, false)
	case version != schemaVersionV1:
		return contract.NewError(contract.FaultRevisionConflictV1, "sqlite_schema_version_drift", "command journal sqlite schema version drifted")
	}
	var integrity string
	if err := j.db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&integrity); err != nil {
		return sqliteErrorV1(err, false)
	}
	if integrity != "ok" {
		return contract.NewError(contract.FaultRevisionConflictV1, "sqlite_integrity_failed", "command journal sqlite integrity check failed")
	}
	return nil
}

func (j *JournalV1) ReserveCommandV1(ctx context.Context, command contract.AgentRunCommandEnvelopeV1) (ports.CommandJournalDispositionV1, ports.CommandJournalEntryV1, error) {
	if err := command.Validate(); err != nil {
		return "", ports.CommandJournalEntryV1{}, err
	}
	encoded, err := json.Marshal(command)
	if err != nil {
		return "", ports.CommandJournalEntryV1{}, contract.NewError(contract.FaultInvalidArgumentV1, "command_encode_failed", "command journal could not encode command")
	}
	tx, err := j.db.BeginTx(ctx, nil)
	if err != nil {
		return "", ports.CommandJournalEntryV1{}, sqliteErrorV1(err, true)
	}
	defer tx.Rollback()
	entry, found, err := loadByKeyV1(ctx, tx, command.IdempotencyKey)
	if err != nil {
		return "", ports.CommandJournalEntryV1{}, err
	}
	if found {
		if _, err := contract.ClassifyAgentRunCommandReplayV1(entry.Command, command); err != nil {
			return "", ports.CommandJournalEntryV1{}, err
		}
		if err := tx.Commit(); err != nil {
			return "", ports.CommandJournalEntryV1{}, sqliteErrorV1(err, true)
		}
		return ports.CommandJournalReplayV1, entry, nil
	}
	var boundKey string
	err = tx.QueryRowContext(ctx, "SELECT idempotency_key FROM command_journal WHERE command_id=?", command.CommandID).Scan(&boundKey)
	if err == nil && boundKey != command.IdempotencyKey {
		return "", ports.CommandJournalEntryV1{}, contract.NewError(contract.FaultRevisionConflictV1, "command_identity_conflict", "same command ID was rebound to another idempotency key")
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", ports.CommandJournalEntryV1{}, sqliteErrorV1(err, false)
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO command_journal(idempotency_key,command_id,command_json) VALUES(?,?,?)", command.IdempotencyKey, command.CommandID, encoded); err != nil {
		return "", ports.CommandJournalEntryV1{}, sqliteErrorV1(err, true)
	}
	if err := tx.Commit(); err != nil {
		return "", ports.CommandJournalEntryV1{}, sqliteErrorV1(err, true)
	}
	return ports.CommandJournalReservedV1, ports.CommandJournalEntryV1{Command: command}, nil
}

func (j *JournalV1) RecordReceiptV1(ctx context.Context, command contract.AgentRunCommandEnvelopeV1, receipt contract.AgentRunCommandReceiptV1) error {
	if err := receipt.ValidateFor(command); err != nil {
		return err
	}
	encoded, err := json.Marshal(receipt)
	if err != nil {
		return contract.NewError(contract.FaultInvalidArgumentV1, "receipt_encode_failed", "command journal could not encode receipt")
	}
	tx, err := j.db.BeginTx(ctx, nil)
	if err != nil {
		return sqliteErrorV1(err, true)
	}
	defer tx.Rollback()
	entry, found, err := loadByKeyV1(ctx, tx, command.IdempotencyKey)
	if err != nil {
		return err
	}
	if !found {
		return contract.NewError(contract.FaultNotFoundV1, "command_reservation_missing", "command reservation was not found")
	}
	if _, err := contract.ClassifyAgentRunCommandReplayV1(entry.Command, command); err != nil {
		return err
	}
	if entry.Receipt != nil {
		if entry.Receipt.ReceiptDigest != receipt.ReceiptDigest {
			return contract.NewError(contract.FaultRevisionConflictV1, "command_receipt_conflict", "command receipt was already recorded with different content")
		}
		if err := tx.Commit(); err != nil {
			return sqliteErrorV1(err, true)
		}
		return nil
	}
	result, err := tx.ExecContext(ctx, "UPDATE command_journal SET receipt_json=? WHERE idempotency_key=? AND receipt_json IS NULL", encoded, command.IdempotencyKey)
	if err != nil {
		return sqliteErrorV1(err, true)
	}
	affected, err := result.RowsAffected()
	if err != nil || affected != 1 {
		return contract.NewError(contract.FaultUnknownOutcomeV1, "receipt_record_unknown", "command receipt mutation outcome is unknown")
	}
	if err := tx.Commit(); err != nil {
		return sqliteErrorV1(err, true)
	}
	return nil
}

func (j *JournalV1) InspectReservedCommandV1(ctx context.Context, command contract.AgentRunCommandEnvelopeV1) (ports.CommandJournalEntryV1, error) {
	if err := command.Validate(); err != nil {
		return ports.CommandJournalEntryV1{}, err
	}
	entry, found, err := loadByKeyV1(ctx, j.db, command.IdempotencyKey)
	if err != nil {
		return ports.CommandJournalEntryV1{}, err
	}
	if !found {
		return ports.CommandJournalEntryV1{}, contract.NewError(contract.FaultNotFoundV1, "original_command_not_found", "original command was not found")
	}
	if _, err := contract.ClassifyAgentRunCommandReplayV1(entry.Command, command); err != nil {
		return ports.CommandJournalEntryV1{}, err
	}
	return entry, nil
}

func (j *JournalV1) InspectCommandV1(ctx context.Context, request contract.InspectOriginalRequestV1) (ports.CommandJournalEntryV1, error) {
	if err := request.Validate(); err != nil {
		return ports.CommandJournalEntryV1{}, err
	}
	entry, found, err := loadByKeyV1(ctx, j.db, request.OriginalIdempotencyKey)
	if err != nil {
		return ports.CommandJournalEntryV1{}, err
	}
	if !found || request.OriginalCommandRef == nil {
		return ports.CommandJournalEntryV1{}, contract.NewError(contract.FaultNotFoundV1, "original_command_not_found", "original command was not found")
	}
	ref, _ := entry.Command.CommandRefV1()
	if ref != *request.OriginalCommandRef || entry.Command.RequestDigest != request.OriginalRequestDigest {
		return ports.CommandJournalEntryV1{}, contract.NewError(contract.FaultRevisionConflictV1, "original_command_splice", "Inspect Original coordinates do not bind the stored command")
	}
	return entry, nil
}

type queryRowerV1 interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func loadByKeyV1(ctx context.Context, source queryRowerV1, key string) (ports.CommandJournalEntryV1, bool, error) {
	var commandJSON, receiptJSON []byte
	err := source.QueryRowContext(ctx, "SELECT command_json,receipt_json FROM command_journal WHERE idempotency_key=?", key).Scan(&commandJSON, &receiptJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return ports.CommandJournalEntryV1{}, false, nil
	}
	if err != nil {
		return ports.CommandJournalEntryV1{}, false, sqliteErrorV1(err, false)
	}
	var command contract.AgentRunCommandEnvelopeV1
	if err := json.Unmarshal(commandJSON, &command); err != nil || command.Validate() != nil {
		return ports.CommandJournalEntryV1{}, false, contract.NewError(contract.FaultRevisionConflictV1, "stored_command_drift", "stored command failed canonical validation")
	}
	entry := ports.CommandJournalEntryV1{Command: command}
	if len(receiptJSON) > 0 {
		var receipt contract.AgentRunCommandReceiptV1
		if err := json.Unmarshal(receiptJSON, &receipt); err != nil || receipt.ValidateFor(command) != nil {
			return ports.CommandJournalEntryV1{}, false, contract.NewError(contract.FaultRevisionConflictV1, "stored_receipt_drift", "stored receipt failed canonical validation")
		}
		entry.Receipt = &receipt
	}
	return entry, true, nil
}

func sqliteErrorV1(err error, mutation bool) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		if mutation {
			return contract.NewError(contract.FaultUnknownOutcomeV1, "sqlite_mutation_unknown", "command journal mutation outcome is unknown")
		}
		return contract.NewError(contract.FaultUnavailableV1, "sqlite_read_canceled", "command journal read was canceled")
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "locked") || strings.Contains(message, "busy") {
		return contract.NewError(contract.FaultUnavailableV1, "sqlite_busy", "command journal sqlite is busy")
	}
	if strings.Contains(message, "unique") || strings.Contains(message, "constraint") {
		return contract.NewError(contract.FaultRevisionConflictV1, "sqlite_constraint_conflict", "command journal sqlite constraint conflicted")
	}
	if mutation {
		return contract.NewError(contract.FaultUnknownOutcomeV1, "sqlite_mutation_unknown", "command journal mutation outcome is unknown")
	}
	return contract.NewError(contract.FaultInternalV1, "sqlite_read_failed", "command journal sqlite read failed")
}
