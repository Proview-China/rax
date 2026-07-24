package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const DurableSessionEventCurrentContractVersionV1 = "praxis.harness.durable-session-event-current/v1"

type DurableSessionEventCurrentRefV1 struct {
	StoreID  string        `json:"store_id"`
	Revision core.Revision `json:"revision"`
	Digest   core.Digest   `json:"digest"`
}

func (r DurableSessionEventCurrentRefV1) Validate() error {
	if !validIDV1(r.StoreID) || r.Revision == 0 || r.Digest.Validate() != nil {
		return invalidV1("durable Session/Event current ref is incomplete")
	}
	return nil
}

type DurableSessionEventCurrentV1 struct {
	ContractVersion     string        `json:"contract_version"`
	StoreID             string        `json:"store_id"`
	Revision            core.Revision `json:"revision"`
	DatabaseIdentity    core.Digest   `json:"database_identity_digest"`
	SchemaDigest        core.Digest   `json:"schema_digest"`
	SessionHistoryCount uint64        `json:"session_history_count"`
	SessionCurrentCount uint64        `json:"session_current_count"`
	EventCount          uint64        `json:"event_count"`
	EventSourceCount    uint64        `json:"event_source_count"`
	CheckedUnixNano     int64         `json:"checked_unix_nano"`
	ExpiresUnixNano     int64         `json:"expires_unix_nano"`
	Digest              core.Digest   `json:"digest"`
}

func (p DurableSessionEventCurrentV1) RefV1() DurableSessionEventCurrentRefV1 {
	return DurableSessionEventCurrentRefV1{StoreID: p.StoreID, Revision: p.Revision, Digest: p.Digest}
}

func (p DurableSessionEventCurrentV1) digestV1() (core.Digest, error) {
	p.ContractVersion = DurableSessionEventCurrentContractVersionV1
	p.Digest = ""
	return core.CanonicalJSONDigest("praxis.harness.durable-session-event-current", DurableSessionEventCurrentContractVersionV1, "DurableSessionEventCurrentV1", p)
}

func sealProofV1(p DurableSessionEventCurrentV1) (DurableSessionEventCurrentV1, error) {
	p.ContractVersion = DurableSessionEventCurrentContractVersionV1
	p.Digest = ""
	digest, err := p.digestV1()
	if err != nil {
		return DurableSessionEventCurrentV1{}, err
	}
	p.Digest = digest
	return p, p.ValidateAt(time.Unix(0, p.CheckedUnixNano))
}

func (p DurableSessionEventCurrentV1) ValidateAt(now time.Time) error {
	if p.ContractVersion != DurableSessionEventCurrentContractVersionV1 || !validIDV1(p.StoreID) || p.Revision == 0 || p.DatabaseIdentity.Validate() != nil || p.SchemaDigest.Validate() != nil || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano || now.IsZero() {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "durable Session/Event current is unavailable")
	}
	if now.UnixNano() < p.CheckedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "durable Session/Event current clock regressed")
	}
	if now.UnixNano() >= p.ExpiresUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "durable Session/Event current is unavailable or expired")
	}
	digest, err := p.digestV1()
	if err != nil || digest != p.Digest {
		return corruptV1("durable Session/Event current digest drifted")
	}
	return nil
}

type DurableSessionEventCurrentReaderV1 interface {
	InspectDurableSessionEventCurrentV1(context.Context, DurableSessionEventCurrentRefV1) (DurableSessionEventCurrentV1, error)
	InspectCurrentDurableSessionEventV1(context.Context, string) (DurableSessionEventCurrentV1, error)
}

func (s *StoreV1) InspectDurableSessionEventCurrentV1(ctx context.Context, ref DurableSessionEventCurrentRefV1) (DurableSessionEventCurrentV1, error) {
	if err := s.readReadyV1(ctx); err != nil {
		return DurableSessionEventCurrentV1{}, err
	}
	if err := ref.Validate(); err != nil {
		return DurableSessionEventCurrentV1{}, err
	}
	value, err := s.readProofDBV1(ctx, ref, false)
	if err != nil {
		return DurableSessionEventCurrentV1{}, err
	}
	now, err := s.nowV1()
	if err != nil {
		return DurableSessionEventCurrentV1{}, err
	}
	if err := value.ValidateAt(now); err != nil {
		return DurableSessionEventCurrentV1{}, err
	}
	return value, nil
}

func (s *StoreV1) InspectCurrentDurableSessionEventV1(ctx context.Context, storeID string) (DurableSessionEventCurrentV1, error) {
	if err := s.readReadyV1(ctx); err != nil {
		return DurableSessionEventCurrentV1{}, err
	}
	if storeID != s.storeID || !validIDV1(storeID) {
		return DurableSessionEventCurrentV1{}, conflictV1("durable Session/Event current StoreID drifted")
	}
	value, err := s.readProofDBV1(ctx, DurableSessionEventCurrentRefV1{StoreID: storeID}, true)
	if err != nil {
		return DurableSessionEventCurrentV1{}, err
	}
	now, err := s.nowV1()
	if err != nil {
		return DurableSessionEventCurrentV1{}, err
	}
	if err := value.ValidateAt(now); err != nil {
		return DurableSessionEventCurrentV1{}, err
	}
	return value, nil
}

func (s *StoreV1) advanceProofTxV1(ctx context.Context, tx *sql.Tx, now time.Time, identity, schema core.Digest) (DurableSessionEventCurrentV1, error) {
	if identity.Validate() != nil || schema.Validate() != nil || now.IsZero() {
		return DurableSessionEventCurrentV1{}, corruptV1("durable Session/Event proof inputs are invalid")
	}
	var revision int64
	err := tx.QueryRowContext(ctx, `SELECT revision FROM harness_session_event_proof_current_v1 WHERE store_id=?`, s.storeID).Scan(&revision)
	if errors.Is(err, sql.ErrNoRows) {
		revision = 0
	} else if err != nil {
		return DurableSessionEventCurrentV1{}, mapDBErrorV1(ctx, err, true)
	}
	if revision == math.MaxInt64 {
		return DurableSessionEventCurrentV1{}, conflictV1("durable Session/Event proof revision overflowed")
	}
	var histories, currents, events, sources uint64
	for _, query := range []struct {
		destination *uint64
		statement   string
	}{
		{&histories, `SELECT COUNT(*) FROM harness_session_history_v4`},
		{&currents, `SELECT COUNT(*) FROM harness_session_current_v4`},
		{&events, `SELECT COUNT(*) FROM harness_event_candidate_v1`},
		{&sources, `SELECT COUNT(*) FROM harness_event_source_head_v1`},
	} {
		if err := tx.QueryRowContext(ctx, query.statement).Scan(query.destination); err != nil {
			return DurableSessionEventCurrentV1{}, mapDBErrorV1(ctx, err, true)
		}
	}
	expires := now.Add(s.proofTTL)
	if !expires.After(now) {
		return DurableSessionEventCurrentV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "durable Session/Event proof TTL overflowed")
	}
	value, err := sealProofV1(DurableSessionEventCurrentV1{
		StoreID: s.storeID, Revision: core.Revision(revision + 1), DatabaseIdentity: identity, SchemaDigest: schema,
		SessionHistoryCount: histories, SessionCurrentCount: currents, EventCount: events, EventSourceCount: sources,
		CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires.UnixNano(),
	})
	if err != nil {
		return DurableSessionEventCurrentV1{}, err
	}
	payload, rowDigest, err := encodeRowV1("DurableSessionEventCurrentV1", value)
	if err != nil {
		return DurableSessionEventCurrentV1{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO harness_session_event_proof_history_v1(store_id,revision,digest,expires_unix_nano,row_digest,canonical_json) VALUES(?,?,?,?,?,?)`, value.StoreID, value.Revision, value.Digest, value.ExpiresUnixNano, rowDigest, payload); err != nil {
		return DurableSessionEventCurrentV1{}, mapDBErrorV1(ctx, err, true)
	}
	if revision == 0 {
		_, err = tx.ExecContext(ctx, `INSERT INTO harness_session_event_proof_current_v1(store_id,revision,digest) VALUES(?,?,?)`, value.StoreID, value.Revision, value.Digest)
	} else {
		var result sql.Result
		result, err = tx.ExecContext(ctx, `UPDATE harness_session_event_proof_current_v1 SET revision=?,digest=? WHERE store_id=? AND revision=?`, value.Revision, value.Digest, value.StoreID, revision)
		if err == nil {
			affected, rowsErr := result.RowsAffected()
			if rowsErr != nil || affected != 1 {
				return DurableSessionEventCurrentV1{}, conflictV1("durable Session/Event proof current CAS lost")
			}
		}
	}
	if err != nil {
		return DurableSessionEventCurrentV1{}, mapDBErrorV1(ctx, err, true)
	}
	return value, nil
}

func (s *StoreV1) readProofDBV1(ctx context.Context, ref DurableSessionEventCurrentRefV1, current bool) (DurableSessionEventCurrentV1, error) {
	if current {
		var revision int64
		var digest, rowDigest string
		var payload []byte
		if err := s.db.QueryRowContext(ctx, `SELECT c.revision,c.digest,h.row_digest,h.canonical_json FROM harness_session_event_proof_current_v1 c JOIN harness_session_event_proof_history_v1 h ON h.store_id=c.store_id AND h.revision=c.revision WHERE c.store_id=?`, ref.StoreID).Scan(&revision, &digest, &rowDigest, &payload); errors.Is(err, sql.ErrNoRows) {
			return DurableSessionEventCurrentV1{}, notFoundV1("durable Session/Event current is absent")
		} else if err != nil {
			return DurableSessionEventCurrentV1{}, mapDBErrorV1(ctx, err, false)
		} else {
			value, decodeErr := decodeRowV1[DurableSessionEventCurrentV1](payload, rowDigest, "DurableSessionEventCurrentV1")
			if decodeErr != nil {
				return DurableSessionEventCurrentV1{}, decodeErr
			}
			if value.StoreID != ref.StoreID || value.Revision != core.Revision(revision) || value.Digest != core.Digest(digest) {
				return DurableSessionEventCurrentV1{}, corruptV1("durable Session/Event current pointer drifted")
			}
			if err := value.ValidateAt(time.Unix(0, value.CheckedUnixNano)); err != nil {
				return DurableSessionEventCurrentV1{}, err
			}
			return value, nil
		}
	}
	return readProofQueryV1(ctx, s.db, ref, true)
}

func (s *StoreV1) readProofTxV1(ctx context.Context, tx *sql.Tx, ref DurableSessionEventCurrentRefV1, exact bool) (DurableSessionEventCurrentV1, error) {
	return readProofQueryV1(ctx, tx, ref, exact)
}

func readProofQueryV1(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, ref DurableSessionEventCurrentRefV1, exact bool) (DurableSessionEventCurrentV1, error) {
	var storedDigest, rowDigest string
	var payload []byte
	err := q.QueryRowContext(ctx, `SELECT digest,row_digest,canonical_json FROM harness_session_event_proof_history_v1 WHERE store_id=? AND revision=?`, ref.StoreID, ref.Revision).Scan(&storedDigest, &rowDigest, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return DurableSessionEventCurrentV1{}, notFoundV1("durable Session/Event proof is absent")
	}
	if err != nil {
		return DurableSessionEventCurrentV1{}, mapDBErrorV1(ctx, err, false)
	}
	value, err := decodeRowV1[DurableSessionEventCurrentV1](payload, rowDigest, "DurableSessionEventCurrentV1")
	if err != nil {
		return DurableSessionEventCurrentV1{}, err
	}
	if value.StoreID != ref.StoreID || value.Revision != ref.Revision || value.Digest != core.Digest(storedDigest) || exact && value.Digest != ref.Digest {
		return DurableSessionEventCurrentV1{}, corruptV1("durable Session/Event proof coordinates drifted")
	}
	if err := value.ValidateAt(time.Unix(0, value.CheckedUnixNano)); err != nil {
		return DurableSessionEventCurrentV1{}, err
	}
	return value, nil
}
