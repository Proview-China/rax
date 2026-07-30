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
)

type EventStreamConfigV1 struct {
	Path        string
	BusyTimeout time.Duration
	Clock       func() time.Time
	ResultTTL   time.Duration
}

type EventStreamV1 struct {
	db        *sql.DB
	clock     func() time.Time
	resultTTL time.Duration
	commitV1  func(*sql.Tx) error
}

func OpenEventStreamV1(ctx context.Context, config EventStreamConfigV1) (*EventStreamV1, error) {
	if strings.TrimSpace(config.Path) == "" || config.Path != strings.TrimSpace(config.Path) {
		return nil, contract.NewError(contract.FaultInvalidArgumentV1, "sqlite_path_invalid", "event stream sqlite path is required")
	}
	if config.Clock == nil {
		config.Clock = time.Now
	}
	if config.ResultTTL <= 0 {
		config.ResultTTL = time.Second
	}
	if config.BusyTimeout <= 0 {
		config.BusyTimeout = 10 * time.Second
	}
	if config.BusyTimeout > time.Minute {
		return nil, contract.NewError(contract.FaultInvalidArgumentV1, "sqlite_busy_timeout_invalid", "event stream sqlite busy timeout exceeds one minute")
	}
	absolute, err := filepath.Abs(config.Path)
	if err != nil {
		return nil, contract.NewError(contract.FaultInvalidArgumentV1, "sqlite_path_invalid", "event stream sqlite path is invalid")
	}
	dsn := "file:" + filepath.ToSlash(absolute) + fmt.Sprintf("?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=synchronous(FULL)&_pragma=busy_timeout(%d)&_txlock=immediate", config.BusyTimeout.Milliseconds())
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, sqliteErrorV1(err, false)
	}
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(4)
	stream := &EventStreamV1{db: db, clock: config.Clock, resultTTL: config.ResultTTL, commitV1: func(tx *sql.Tx) error { return tx.Commit() }}
	if err := stream.initializeV1(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return stream, nil
}

func (s *EventStreamV1) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *EventStreamV1) initializeV1(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS event_stream_current (
			stream_digest TEXT PRIMARY KEY,
			stream_ref_json BLOB NOT NULL,
			retention_ref_json BLOB NOT NULL,
			first_sequence TEXT NOT NULL,
			last_sequence TEXT NOT NULL
		);
		CREATE TABLE IF NOT EXISTS agent_run_events (
			stream_digest TEXT NOT NULL,
			sequence TEXT NOT NULL,
			event_json BLOB NOT NULL,
			PRIMARY KEY(stream_digest, sequence),
			FOREIGN KEY(stream_digest) REFERENCES event_stream_current(stream_digest)
		);
	`)
	if err != nil {
		return sqliteErrorV1(err, true)
	}
	return nil
}

// AppendEventV1 stores an already sealed owner event projection. It does not
// synthesize an owner event or make the stream an additional domain ledger.
func (s *EventStreamV1) AppendEventV1(ctx context.Context, event contract.AgentRunEventV1, retention contract.ExactRefWireV1) error {
	if err := event.Validate(); err != nil {
		return err
	}
	if err := retention.Validate(); err != nil {
		return err
	}
	if retention.Kind != contract.AgentRunEventRetentionCurrentRefKindV1 {
		return contract.NewError(contract.FaultPreconditionFailedV1, "event_retention_kind_drift", "event retention current kind drifted")
	}
	eventJSON, _ := json.Marshal(event)
	streamJSON, _ := json.Marshal(event.StreamRef)
	retentionJSON, _ := json.Marshal(retention)
	sequence, _ := event.Sequence.Uint64V1()
	encodedSequence := eventSequenceV1(sequence)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return sqliteErrorV1(err, true)
	}
	defer tx.Rollback()
	var storedStreamJSON, storedRetentionJSON []byte
	var first, last string
	err = tx.QueryRowContext(ctx, "SELECT stream_ref_json,retention_ref_json,first_sequence,last_sequence FROM event_stream_current WHERE stream_digest=?", event.StreamRef.Digest).Scan(&storedStreamJSON, &storedRetentionJSON, &first, &last)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		if sequence != 1 {
			return contract.NewError(contract.FaultPreconditionFailedV1, "event_stream_initial_sequence_invalid", "new event stream must start at sequence 1")
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO event_stream_current(stream_digest,stream_ref_json,retention_ref_json,first_sequence,last_sequence) VALUES(?,?,?,?,?)", event.StreamRef.Digest, streamJSON, retentionJSON, encodedSequence, encodedSequence); err != nil {
			return sqliteErrorV1(err, true)
		}
	case err != nil:
		return sqliteErrorV1(err, false)
	default:
		var storedStream, storedRetention contract.ExactRefWireV1
		if json.Unmarshal(storedStreamJSON, &storedStream) != nil || json.Unmarshal(storedRetentionJSON, &storedRetention) != nil || storedStream != event.StreamRef || storedRetention != retention {
			return contract.NewError(contract.FaultRevisionConflictV1, "event_stream_current_drift", "event stream or retention current drifted")
		}
		lastValue, err := parseEventSequenceV1(last)
		if err != nil {
			return err
		}
		if sequence <= lastValue {
			var originalJSON []byte
			if err := tx.QueryRowContext(ctx, "SELECT event_json FROM agent_run_events WHERE stream_digest=? AND sequence=?", event.StreamRef.Digest, encodedSequence).Scan(&originalJSON); err != nil {
				return contract.NewError(contract.FaultRevisionConflictV1, "event_replay_missing", "event sequence exists outside retained exact history")
			}
			var original contract.AgentRunEventV1
			if json.Unmarshal(originalJSON, &original) != nil || contract.ValidateAgentRunEventReplayV1(original, event) != nil {
				return contract.NewError(contract.FaultRevisionConflictV1, "event_replay_conflict", "same event sequence was rebound to different content")
			}
			if err := tx.Commit(); err != nil {
				return sqliteErrorV1(err, false)
			}
			return nil
		}
		if lastValue == ^uint64(0) || sequence != lastValue+1 {
			return contract.NewError(contract.FaultPreconditionFailedV1, "event_sequence_gap", "event append must be the exact next sequence")
		}
		if _, err := tx.ExecContext(ctx, "UPDATE event_stream_current SET last_sequence=? WHERE stream_digest=? AND last_sequence=?", encodedSequence, event.StreamRef.Digest, last); err != nil {
			return sqliteErrorV1(err, true)
		}
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO agent_run_events(stream_digest,sequence,event_json) VALUES(?,?,?)", event.StreamRef.Digest, encodedSequence, eventJSON); err != nil {
		return sqliteErrorV1(err, true)
	}
	if err := tx.Commit(); err != nil {
		return sqliteErrorV1(err, true)
	}
	return nil
}

// AdvanceRetentionV1 records an exact new retention current and removes events
// below firstAvailable. The caller owns the retention decision.
func (s *EventStreamV1) AdvanceRetentionV1(ctx context.Context, stream, expected, next contract.ExactRefWireV1, firstAvailable contract.WireUint64V1) error {
	for _, ref := range []contract.ExactRefWireV1{stream, expected, next} {
		if err := ref.Validate(); err != nil {
			return err
		}
	}
	if stream.Kind != contract.AgentRunEventStreamRefKindV1 || expected.Kind != contract.AgentRunEventRetentionCurrentRefKindV1 || next.Kind != contract.AgentRunEventRetentionCurrentRefKindV1 {
		return contract.NewError(contract.FaultPreconditionFailedV1, "event_retention_kind_drift", "event retention exact ref kind drifted")
	}
	expectedRevision, _ := expected.Revision.Uint64V1()
	nextRevision, _ := next.Revision.Uint64V1()
	if next.ID != expected.ID || expectedRevision == ^uint64(0) || nextRevision != expectedRevision+1 {
		return contract.NewError(contract.FaultRevisionConflictV1, "event_retention_successor_splice", "next retention current must preserve identity and advance exactly one revision")
	}
	first, err := firstAvailable.Uint64V1()
	if err != nil || first == 0 {
		return contract.NewError(contract.FaultInvalidArgumentV1, "event_retention_sequence_invalid", "first available sequence must be positive")
	}
	expectedJSON, _ := json.Marshal(expected)
	nextJSON, _ := json.Marshal(next)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return sqliteErrorV1(err, true)
	}
	defer tx.Rollback()
	var storedStreamJSON, storedRetentionJSON []byte
	var lastEncoded string
	if err := tx.QueryRowContext(ctx, "SELECT stream_ref_json,retention_ref_json,last_sequence FROM event_stream_current WHERE stream_digest=?", stream.Digest).Scan(&storedStreamJSON, &storedRetentionJSON, &lastEncoded); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return contract.NewError(contract.FaultNotFoundV1, "event_stream_not_found", "event stream was not found")
		}
		return sqliteErrorV1(err, false)
	}
	var storedStream, storedRetention contract.ExactRefWireV1
	if json.Unmarshal(storedStreamJSON, &storedStream) != nil || json.Unmarshal(storedRetentionJSON, &storedRetention) != nil || storedStream != stream || storedRetention != expected {
		return contract.NewError(contract.FaultRevisionConflictV1, "event_retention_cas_failed", "event stream or retention expected current drifted")
	}
	last, err := parseEventSequenceV1(lastEncoded)
	if err != nil {
		return err
	}
	if (last != ^uint64(0) && first > last+1) || (last == ^uint64(0) && first > last) {
		return contract.NewError(contract.FaultPreconditionFailedV1, "event_retention_beyond_tail", "first available sequence cannot exceed the stream tail plus one")
	}
	result, err := tx.ExecContext(ctx, "UPDATE event_stream_current SET retention_ref_json=?,first_sequence=? WHERE stream_digest=? AND retention_ref_json=?", nextJSON, eventSequenceV1(first), stream.Digest, expectedJSON)
	if err != nil {
		return sqliteErrorV1(err, true)
	}
	affected, _ := result.RowsAffected()
	if affected != 1 {
		return contract.NewError(contract.FaultRevisionConflictV1, "event_retention_cas_failed", "event retention current CAS failed")
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM agent_run_events WHERE stream_digest=? AND sequence<?", stream.Digest, eventSequenceV1(first)); err != nil {
		return sqliteErrorV1(err, true)
	}
	if err := s.commitV1(tx); err != nil {
		return sqliteErrorV1(err, true)
	}
	return nil
}

func (s *EventStreamV1) WatchAgentRunV1(ctx context.Context, request contract.AgentRunWatchRequestV1) (contract.AgentRunWatchResultV1, error) {
	if err := request.ValidateCurrentV1(contract.NewWireUnixNanoV1(s.clock().UTC().UnixNano())); err != nil {
		return contract.AgentRunWatchResultV1{}, err
	}
	var streamJSON, retentionJSON []byte
	var firstEncoded, lastEncoded string
	if err := s.db.QueryRowContext(ctx, "SELECT stream_ref_json,retention_ref_json,first_sequence,last_sequence FROM event_stream_current WHERE stream_digest=?", request.StreamRef.Digest).Scan(&streamJSON, &retentionJSON, &firstEncoded, &lastEncoded); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return contract.AgentRunWatchResultV1{}, contract.NewError(contract.FaultNotFoundV1, "event_stream_not_found", "event stream was not found")
		}
		return contract.AgentRunWatchResultV1{}, sqliteErrorV1(err, false)
	}
	var stream, retention contract.ExactRefWireV1
	if json.Unmarshal(streamJSON, &stream) != nil || json.Unmarshal(retentionJSON, &retention) != nil || stream != request.StreamRef || retention != request.RetentionCurrentRef {
		return contract.AgentRunWatchResultV1{}, contract.NewError(contract.FaultRevisionConflictV1, "event_watch_current_drift", "Watch exact stream or retention current drifted")
	}
	first, _ := parseEventSequenceV1(firstEncoded)
	after, _ := request.AfterSequence.Uint64V1()
	if after != ^uint64(0) && after+1 < first {
		current := request.RetentionCurrentRef
		return s.sealWatchV1(request, contract.AgentRunWatchResyncRequiredV1, nil, nil, &contract.FaultV1{
			Code: contract.FaultPreconditionFailedV1, Reason: "event_retention_gap",
			Message:    "requested event history is outside retention; inspect current state",
			CurrentRef: &current, TraceID: request.TraceID, RetryDirective: contract.RetryInspectV1,
		})
	}
	limit, _ := request.Limit.Uint64V1()
	rows, err := s.db.QueryContext(ctx, "SELECT event_json FROM agent_run_events WHERE stream_digest=? AND sequence>? ORDER BY sequence LIMIT ?", request.StreamRef.Digest, eventSequenceV1(after), limit)
	if err != nil {
		return contract.AgentRunWatchResultV1{}, sqliteErrorV1(err, false)
	}
	defer rows.Close()
	events := []contract.AgentRunEventV1{}
	for rows.Next() {
		var encoded []byte
		if err := rows.Scan(&encoded); err != nil {
			return contract.AgentRunWatchResultV1{}, sqliteErrorV1(err, false)
		}
		var event contract.AgentRunEventV1
		if json.Unmarshal(encoded, &event) != nil || event.Validate() != nil {
			return contract.AgentRunWatchResultV1{}, contract.NewError(contract.FaultRevisionConflictV1, "stored_event_drift", "stored event failed canonical validation")
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return contract.AgentRunWatchResultV1{}, sqliteErrorV1(err, false)
	}
	nextSequence := request.AfterSequence
	disposition := contract.AgentRunWatchTimeoutV1
	if len(events) > 0 {
		disposition = contract.AgentRunWatchEventsV1
		nextSequence = events[len(events)-1].Sequence
	}
	cursor, err := contract.SealAgentRunEventCursorV1(contract.AgentRunEventCursorV1{StreamRef: request.StreamRef, Sequence: nextSequence})
	if err != nil {
		return contract.AgentRunWatchResultV1{}, err
	}
	return s.sealWatchV1(request, disposition, events, &cursor, nil)
}

func (s *EventStreamV1) sealWatchV1(request contract.AgentRunWatchRequestV1, disposition contract.AgentRunWatchDispositionV1, events []contract.AgentRunEventV1, cursor *contract.AgentRunEventCursorV1, fault *contract.FaultV1) (contract.AgentRunWatchResultV1, error) {
	now := s.clock().UTC().UnixNano()
	expires := now + s.resultTTL.Nanoseconds()
	notAfter, _ := request.NotAfterUnixNano.Int64V1()
	if expires > notAfter {
		expires = notAfter
	}
	result, err := contract.SealAgentRunWatchResultV1(contract.AgentRunWatchResultV1{
		RequestDigest: request.RequestDigest, TraceID: request.TraceID,
		StreamRef: request.StreamRef, RetentionCurrentRef: request.RetentionCurrentRef,
		Disposition: disposition, Events: events, NextCursor: cursor, Fault: fault,
		Window: contract.WireValidityWindowV1{CheckedUnixNano: contract.NewWireUnixNanoV1(now), ExpiresUnixNano: contract.NewWireUnixNanoV1(expires)},
	})
	if err != nil {
		return contract.AgentRunWatchResultV1{}, err
	}
	return result, result.ValidateFor(request)
}

func eventSequenceV1(sequence uint64) string { return fmt.Sprintf("%020d", sequence) }
func parseEventSequenceV1(encoded string) (uint64, error) {
	var value uint64
	if _, err := fmt.Sscanf(encoded, "%d", &value); err != nil || eventSequenceV1(value) != encoded {
		return 0, contract.NewError(contract.FaultRevisionConflictV1, "stored_event_sequence_drift", "stored event sequence is not canonical")
	}
	return value, nil
}
