package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

const (
	kindReservation = "domain_reservation_v2"
	kindObservation = "provider_observation_v2"
	kindInspection  = "inspection_fact_v2"
	kindDomain      = "domain_result_v2"
)

func (s *Store) CreateReservation(ctx context.Context, value contract.DomainReservation) error {
	if err := value.ValidateShape(); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := insertFact(ctx, tx, kindReservation, value.Meta.ID, value.Meta.Revision, value.Meta.Digest, value); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO reservation_attempts(operation_id,effect_id,attempt_id,reservation_id) VALUES(?,?,?,?)`, value.OperationID, value.EffectID, value.AttemptID, value.Meta.ID); err != nil {
		return classifyWrite(err)
	}
	return tx.Commit()
}

func (s *Store) GetReservation(ctx context.Context, id string) (contract.DomainReservation, error) {
	value, err := readFact[contract.DomainReservation](ctx, s.db, kindReservation, id)
	if err == nil {
		err = value.ValidateShape()
	}
	return value, err
}

func (s *Store) InspectReservationByAttempt(ctx context.Context, operationID, effectID, attemptID string) (contract.DomainReservation, error) {
	var id string
	if err := s.db.QueryRowContext(ctx, `SELECT reservation_id FROM reservation_attempts WHERE operation_id=? AND effect_id=? AND attempt_id=?`, operationID, effectID, attemptID).Scan(&id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return contract.DomainReservation{}, ports.ErrNotFound
		}
		return contract.DomainReservation{}, err
	}
	return s.GetReservation(ctx, id)
}

func (s *Store) AppendObservation(ctx context.Context, reservationID string, value contract.Observation) (bool, error) {
	if err := value.ValidateShape(); err != nil {
		return false, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var reservationExists int
	if err := tx.QueryRowContext(ctx, `SELECT 1 FROM sandbox_facts WHERE kind=? AND id=?`, kindReservation, reservationID).Scan(&reservationExists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, ports.ErrNotFound
		}
		return false, err
	}
	var existingBody []byte
	if err := tx.QueryRowContext(ctx, `SELECT body FROM sandbox_facts WHERE kind=? AND id=?`, kindObservation, value.Meta.ID).Scan(&existingBody); err == nil {
		var existing contract.Observation
		if err := decode(existingBody, &existing); err != nil {
			return false, err
		}
		if existing.Meta.Digest == value.Meta.Digest && existing.PayloadDigest == value.PayloadDigest {
			return false, nil
		}
		return false, ports.ErrConflict
	} else if !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	var epoch, sequence uint64
	var payload string
	err = tx.QueryRowContext(ctx, `SELECT source_epoch,source_sequence,payload_digest FROM observation_source_current WHERE source_id=?`, value.SourceRegistrationID).Scan(&epoch, &sequence, &payload)
	if err == nil {
		switch {
		case value.SourceEpoch < epoch || value.SourceEpoch == epoch && value.SourceSequence < sequence:
			return false, ports.ErrStale
		case value.SourceEpoch == epoch && value.SourceSequence == sequence && value.PayloadDigest == payload:
			return false, nil
		case value.SourceEpoch == epoch && value.SourceSequence == sequence:
			return false, ports.ErrSourceConflict
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	if err := insertFact(ctx, tx, kindObservation, value.Meta.ID, value.Meta.Revision, value.Meta.Digest, value); err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO observation_source_current(source_id,source_epoch,source_sequence,payload_digest) VALUES(?,?,?,?) ON CONFLICT(source_id) DO UPDATE SET source_epoch=excluded.source_epoch,source_sequence=excluded.source_sequence,payload_digest=excluded.payload_digest`, value.SourceRegistrationID, value.SourceEpoch, value.SourceSequence, value.PayloadDigest); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

func (s *Store) GetObservation(ctx context.Context, id string) (contract.Observation, error) {
	value, err := readFact[contract.Observation](ctx, s.db, kindObservation, id)
	if err == nil {
		err = value.ValidateShape()
	}
	return value, err
}

func (s *Store) CreateInspection(ctx context.Context, value contract.InspectionFact) error {
	if err := value.ValidateShape(); err != nil {
		return err
	}
	return s.createFact(ctx, kindInspection, value.Meta.ID, value.Meta.Revision, value.Meta.Digest, value)
}

func (s *Store) GetInspection(ctx context.Context, id string) (contract.InspectionFact, error) {
	value, err := readFact[contract.InspectionFact](ctx, s.db, kindInspection, id)
	if err == nil {
		err = value.ValidateShape()
	}
	return value, err
}

func (s *Store) CreateDomainResult(ctx context.Context, value contract.SandboxDomainResultFact) error {
	if err := value.ValidateShape(); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := insertFact(ctx, tx, kindDomain, value.Meta.ID, value.Meta.Revision, value.Meta.Digest, value); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO domain_result_by_reservation(reservation_id,result_id) VALUES(?,?)`, value.ReservationRef.ID, value.Meta.ID); err != nil {
		return classifyWrite(err)
	}
	return tx.Commit()
}

func (s *Store) GetDomainResult(ctx context.Context, id string) (contract.SandboxDomainResultFact, error) {
	value, err := readFact[contract.SandboxDomainResultFact](ctx, s.db, kindDomain, id)
	if err == nil {
		err = value.ValidateShape()
	}
	return value, err
}

func (s *Store) InitializeEnvironmentProjection(ctx context.Context, value contract.EnvironmentProjection) error {
	if err := value.ValidateShape(); err != nil {
		return err
	}
	body, err := encode(value)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO environment_projection_history(lease_id,revision,digest,body) VALUES(?,?,?,?)`, value.Lease.LeaseID, value.Meta.Revision, value.Meta.Digest, body); err != nil {
		return classifyWrite(err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO environment_projection_current(lease_id,revision,digest,body) VALUES(?,?,?,?)`, value.Lease.LeaseID, value.Meta.Revision, value.Meta.Digest, body); err != nil {
		return classifyWrite(err)
	}
	return tx.Commit()
}

func (s *Store) GetProjection(ctx context.Context, leaseID string) (contract.EnvironmentProjection, error) {
	var body []byte
	if err := s.db.QueryRowContext(ctx, `SELECT body FROM environment_projection_current WHERE lease_id=?`, leaseID).Scan(&body); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return contract.EnvironmentProjection{}, ports.ErrNotFound
		}
		return contract.EnvironmentProjection{}, err
	}
	var value contract.EnvironmentProjection
	if err := decode(body, &value); err != nil {
		return contract.EnvironmentProjection{}, err
	}
	return value, value.ValidateShape()
}

func (s *Store) GetSettlementBinding(ctx context.Context, opaque contract.Ref) (contract.Ref, error) {
	if err := opaque.ValidateShape("opaque settlement ref"); err != nil {
		return contract.Ref{}, err
	}
	var opaqueRevision, resultRevision uint64
	var opaqueDigest, resultID, resultDigest string
	if err := s.db.QueryRowContext(ctx, `SELECT opaque_revision,opaque_digest,result_id,result_revision,result_digest FROM settlement_bindings WHERE opaque_id=?`, opaque.ID).Scan(&opaqueRevision, &opaqueDigest, &resultID, &resultRevision, &resultDigest); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return contract.Ref{}, ports.ErrNotFound
		}
		return contract.Ref{}, err
	}
	if opaqueRevision != opaque.Revision || opaqueDigest != opaque.Digest {
		return contract.Ref{}, ports.ErrConflict
	}
	return contract.Ref{ID: resultID, Revision: resultRevision, Digest: resultDigest}, nil
}

func (s *Store) CompareAndSwapProjection(ctx context.Context, expectedRevision uint64, value contract.EnvironmentProjection) error {
	if err := value.ValidateShape(); err != nil {
		return err
	}
	if value.Meta.Revision != expectedRevision+1 || value.LastDomainResultRef.ValidateShape("domain result ref applied by CAS") != nil || value.LastSettlementRef.ValidateShape("opaque settlement ref applied by CAS") != nil {
		return ports.ErrConflict
	}
	body, err := encode(value)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var currentRevision uint64
	if err := tx.QueryRowContext(ctx, `SELECT revision FROM environment_projection_current WHERE lease_id=?`, value.Lease.LeaseID).Scan(&currentRevision); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ports.ErrNotFound
		}
		return err
	}
	if currentRevision != expectedRevision {
		return ports.ErrStale
	}
	var boundRevision, resultRevision uint64
	var boundDigest, resultID, resultDigest string
	err = tx.QueryRowContext(ctx, `SELECT opaque_revision,opaque_digest,result_id,result_revision,result_digest FROM settlement_bindings WHERE opaque_id=?`, value.LastSettlementRef.ID).Scan(&boundRevision, &boundDigest, &resultID, &resultRevision, &resultDigest)
	if err == nil {
		if boundRevision != value.LastSettlementRef.Revision || boundDigest != value.LastSettlementRef.Digest || resultID != value.LastDomainResultRef.ID || resultRevision != value.LastDomainResultRef.Revision || resultDigest != value.LastDomainResultRef.Digest {
			return ports.ErrConflict
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO environment_projection_history(lease_id,revision,digest,body) VALUES(?,?,?,?)`, value.Lease.LeaseID, value.Meta.Revision, value.Meta.Digest, body); err != nil {
		return classifyWrite(err)
	}
	result, err := tx.ExecContext(ctx, `UPDATE environment_projection_current SET revision=?,digest=?,body=? WHERE lease_id=? AND revision=?`, value.Meta.Revision, value.Meta.Digest, body, value.Lease.LeaseID, expectedRevision)
	if err != nil {
		return err
	}
	if rows, err := result.RowsAffected(); err != nil || rows != 1 {
		return ports.ErrStale
	}
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO settlement_bindings(opaque_id,opaque_revision,opaque_digest,result_id,result_revision,result_digest) VALUES(?,?,?,?,?,?)`, value.LastSettlementRef.ID, value.LastSettlementRef.Revision, value.LastSettlementRef.Digest, value.LastDomainResultRef.ID, value.LastDomainResultRef.Revision, value.LastDomainResultRef.Digest); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) createFact(ctx context.Context, kind, id string, revision uint64, digest string, value any) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := insertFact(ctx, tx, kind, id, revision, digest, value); err != nil {
		return err
	}
	return tx.Commit()
}

var _ ports.FactStore = (*Store)(nil)

func exactRef(value contract.Ref) string {
	return fmt.Sprintf("%s/%d/%s", value.ID, value.Revision, value.Digest)
}
