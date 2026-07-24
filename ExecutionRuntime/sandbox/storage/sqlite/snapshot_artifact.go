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
	kindSnapshotReservation     = "snapshot_reservation_v2"
	kindSnapshotReservationFact = "snapshot_reservation_fact_v2"
	kindSnapshotArtifactFact    = "snapshot_artifact_fact_v2"
	kindSnapshotEntry           = "snapshot_aggregate_entry_v2"
	kindSnapshotEnvelope        = "snapshot_aggregate_envelope_v2"
	kindSnapshotCurrentIndex    = "snapshot_current_index_v2"
)

func (s *Store) CreateReservedSnapshotArtifact(ctx context.Context, bundle contract.SnapshotArtifactReservedBundleV2) (bool, error) {
	if err := bundle.ValidateShape(); err != nil {
		return false, err
	}
	stableDigest, err := contract.SnapshotArtifactStableSourceKeyDigest(bundle.StableKey)
	if err != nil {
		return false, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var reservationID string
	var reservationRevision uint64
	var reservationDigest string
	var reservationKey string
	err = tx.QueryRowContext(ctx, `SELECT reservation_key,reservation_id,reservation_revision,reservation_digest FROM snapshot_reservation_by_stable WHERE stable_digest=?`, stableDigest).Scan(&reservationKey, &reservationID, &reservationRevision, &reservationDigest)
	if err == nil {
		existingRef := bundle.Reservation.ExactRef()
		if reservationID == existingRef.ID && reservationRevision == existingRef.Revision && reservationDigest == existingRef.Digest {
			return false, nil
		}
		return false, ports.ErrConflict
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	if err := checkAndAdvanceSnapshotOwnerClock(ctx, tx, bundle.OwnerClockWatermark, false); err != nil {
		return false, err
	}
	var exists int
	err = tx.QueryRowContext(ctx, `SELECT 1 FROM snapshot_current_index WHERE aggregate_id=?`, bundle.Envelope.AggregateRef.AggregateID).Scan(&exists)
	if err == nil {
		return false, ports.ErrConflict
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	for _, item := range []struct {
		kind string
		ref  contract.SnapshotArtifactExactRefV2
		body any
	}{
		{kindSnapshotReservation, bundle.Reservation.ExactRef(), bundle.Reservation},
		{kindSnapshotReservationFact, bundle.ReservationFact.ExactRef(), bundle.ReservationFact},
		{kindSnapshotEntry, bundle.Entry.ExactRef(), bundle.Entry},
		{kindSnapshotEnvelope, bundle.Envelope.AggregateRef.ExactRef(), bundle.Envelope},
		{kindSnapshotCurrentIndex, bundle.CurrentIndex.CurrentIndexRef, bundle.CurrentIndex},
	} {
		if err := insertSnapshotFact(ctx, tx, item.kind, item.ref, item.body); err != nil {
			return false, err
		}
	}
	reservationRef := bundle.Reservation.ExactRef()
	if _, err := tx.ExecContext(ctx, `INSERT INTO snapshot_reservation_by_stable(stable_digest,reservation_key,reservation_id,reservation_revision,reservation_digest) VALUES(?,?,?,?,?)`, stableDigest, snapshotExactKey(reservationRef), reservationRef.ID, reservationRef.Revision, reservationRef.Digest); err != nil {
		return false, classifyWrite(err)
	}
	currentBody, err := encode(bundle.CurrentIndex)
	if err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO snapshot_current_index(aggregate_id,ref_id,revision,digest,owner_clock_watermark,body) VALUES(?,?,?,?,?,?)`, bundle.CurrentIndex.ArtifactAggregateID, bundle.CurrentIndex.CurrentIndexRef.ID, bundle.CurrentIndex.CurrentIndexRef.Revision, bundle.CurrentIndex.CurrentIndexRef.Digest, bundle.OwnerClockWatermark, currentBody); err != nil {
		return false, classifyWrite(err)
	}
	if err := setSnapshotOwnerClock(ctx, tx, bundle.OwnerClockWatermark); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

func (s *Store) CommitAvailableSnapshotArtifact(ctx context.Context, bundle contract.SnapshotArtifactAvailableBundleV2) (bool, error) {
	if err := bundle.ValidateShape(); err != nil {
		return false, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var currentBody []byte
	if err := tx.QueryRowContext(ctx, `SELECT body FROM snapshot_current_index WHERE aggregate_id=?`, bundle.CurrentIndex.ArtifactAggregateID).Scan(&currentBody); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, ports.ErrNotFound
		}
		return false, err
	}
	var current contract.SnapshotArtifactAggregateCurrentIndexV2
	if err := decode(currentBody, &current); err != nil {
		return false, err
	}
	if contract.SameSnapshotArtifactExactRef(current.CurrentIndexRef, bundle.CurrentIndex.CurrentIndexRef) {
		return false, nil
	}
	if !contract.SameSnapshotArtifactExactRef(current.CurrentIndexRef, bundle.ExpectedCurrentIndexRef) || current.AggregateState != contract.SnapshotArtifactAggregateReserved || bundle.CurrentIndex.CurrentIndexRef.Revision != current.CurrentIndexRef.Revision+1 || bundle.Envelope.AggregateRef.Revision != current.HeadAggregateEnvelopeRef.Revision+1 {
		return false, ports.ErrConflict
	}
	if err := checkAndAdvanceSnapshotOwnerClock(ctx, tx, bundle.OwnerClockWatermark, false); err != nil {
		return false, err
	}
	for _, item := range []struct {
		kind string
		ref  contract.SnapshotArtifactExactRefV2
		body any
	}{
		{kindSnapshotArtifactFact, bundle.Fact.ExactRef(), bundle.Fact},
		{kindSnapshotEntry, bundle.Entry.ExactRef(), bundle.Entry},
		{kindSnapshotEnvelope, bundle.Envelope.AggregateRef.ExactRef(), bundle.Envelope},
		{kindSnapshotCurrentIndex, bundle.CurrentIndex.CurrentIndexRef, bundle.CurrentIndex},
	} {
		if err := insertSnapshotFact(ctx, tx, item.kind, item.ref, item.body); err != nil {
			return false, err
		}
	}
	nextBody, err := encode(bundle.CurrentIndex)
	if err != nil {
		return false, err
	}
	result, err := tx.ExecContext(ctx, `UPDATE snapshot_current_index SET ref_id=?,revision=?,digest=?,owner_clock_watermark=?,body=? WHERE aggregate_id=? AND revision=? AND digest=?`, bundle.CurrentIndex.CurrentIndexRef.ID, bundle.CurrentIndex.CurrentIndexRef.Revision, bundle.CurrentIndex.CurrentIndexRef.Digest, bundle.OwnerClockWatermark, nextBody, bundle.CurrentIndex.ArtifactAggregateID, bundle.ExpectedCurrentIndexRef.Revision, bundle.ExpectedCurrentIndexRef.Digest)
	if err != nil {
		return false, err
	}
	if rows, err := result.RowsAffected(); err != nil || rows != 1 {
		return false, ports.ErrConflict
	}
	if err := setSnapshotOwnerClock(ctx, tx, bundle.OwnerClockWatermark); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

func (s *Store) InspectSnapshotArtifactReservation(ctx context.Context, expected contract.SnapshotArtifactExactRefV2) (contract.SnapshotArtifactReservationV2, error) {
	return inspectSnapshotFact[contract.SnapshotArtifactReservationV2](ctx, s.db, kindSnapshotReservation, expected, func(value contract.SnapshotArtifactReservationV2) contract.SnapshotArtifactExactRefV2 {
		return value.ExactRef()
	})
}

func (s *Store) InspectSnapshotArtifactReservationByStableKey(ctx context.Context, stable contract.SnapshotArtifactStableSourceKeyV2) (contract.SnapshotArtifactReservationV2, error) {
	digest, err := contract.SnapshotArtifactStableSourceKeyDigest(stable)
	if err != nil {
		return contract.SnapshotArtifactReservationV2{}, err
	}
	var key, id, refDigest string
	var revision uint64
	if err := s.db.QueryRowContext(ctx, `SELECT reservation_key,reservation_id,reservation_revision,reservation_digest FROM snapshot_reservation_by_stable WHERE stable_digest=?`, digest).Scan(&key, &id, &revision, &refDigest); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return contract.SnapshotArtifactReservationV2{}, ports.ErrNotFound
		}
		return contract.SnapshotArtifactReservationV2{}, err
	}
	var body []byte
	if err := s.db.QueryRowContext(ctx, `SELECT body FROM sandbox_facts WHERE kind=? AND id=? AND revision=? AND digest=?`, kindSnapshotReservation, key, revision, refDigest).Scan(&body); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return contract.SnapshotArtifactReservationV2{}, ports.ErrNotFound
		}
		return contract.SnapshotArtifactReservationV2{}, err
	}
	var value contract.SnapshotArtifactReservationV2
	if err := decode(body, &value); err != nil {
		return contract.SnapshotArtifactReservationV2{}, err
	}
	if value.ExactRef().ID != id || value.ExactRef().Revision != revision || value.ExactRef().Digest != refDigest {
		return contract.SnapshotArtifactReservationV2{}, ports.ErrConflict
	}
	return value, value.ValidateShape()
}

func (s *Store) InspectSnapshotArtifactReservationFact(ctx context.Context, expected contract.SnapshotArtifactExactRefV2) (contract.SnapshotArtifactReservationFactV2, error) {
	return inspectSnapshotFact[contract.SnapshotArtifactReservationFactV2](ctx, s.db, kindSnapshotReservationFact, expected, func(value contract.SnapshotArtifactReservationFactV2) contract.SnapshotArtifactExactRefV2 {
		return value.ExactRef()
	})
}

func (s *Store) InspectSnapshotArtifactFact(ctx context.Context, expected contract.SnapshotArtifactExactRefV2) (contract.SnapshotArtifactFactV2, error) {
	return inspectSnapshotFact[contract.SnapshotArtifactFactV2](ctx, s.db, kindSnapshotArtifactFact, expected, func(value contract.SnapshotArtifactFactV2) contract.SnapshotArtifactExactRefV2 {
		return value.ExactRef()
	})
}

func (s *Store) InspectSnapshotArtifactEntry(ctx context.Context, expected contract.SnapshotArtifactExactRefV2) (contract.SnapshotArtifactAggregateEntryV2, error) {
	return inspectSnapshotFact[contract.SnapshotArtifactAggregateEntryV2](ctx, s.db, kindSnapshotEntry, expected, func(value contract.SnapshotArtifactAggregateEntryV2) contract.SnapshotArtifactExactRefV2 {
		return value.ExactRef()
	})
}

func (s *Store) InspectSnapshotArtifactEnvelope(ctx context.Context, expected contract.SnapshotArtifactAggregateRefV2) (contract.SnapshotArtifactAggregateEnvelopeV2, error) {
	value, err := inspectSnapshotFact[contract.SnapshotArtifactAggregateEnvelopeV2](ctx, s.db, kindSnapshotEnvelope, expected.ExactRef(), func(value contract.SnapshotArtifactAggregateEnvelopeV2) contract.SnapshotArtifactExactRefV2 {
		return value.AggregateRef.ExactRef()
	})
	if err == nil && value.AggregateRef != expected {
		return contract.SnapshotArtifactAggregateEnvelopeV2{}, ports.ErrConflict
	}
	return value, err
}

func (s *Store) InspectSnapshotArtifactCurrentIndex(ctx context.Context, aggregateID string) (contract.SnapshotArtifactAggregateCurrentIndexV2, error) {
	var body []byte
	if err := s.db.QueryRowContext(ctx, `SELECT body FROM snapshot_current_index WHERE aggregate_id=?`, aggregateID).Scan(&body); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return contract.SnapshotArtifactAggregateCurrentIndexV2{}, ports.ErrNotFound
		}
		return contract.SnapshotArtifactAggregateCurrentIndexV2{}, err
	}
	var value contract.SnapshotArtifactAggregateCurrentIndexV2
	if err := decode(body, &value); err != nil {
		return contract.SnapshotArtifactAggregateCurrentIndexV2{}, err
	}
	return value, value.ValidateShape()
}

func insertSnapshotFact(ctx context.Context, tx *sql.Tx, kind string, ref contract.SnapshotArtifactExactRefV2, value any) error {
	return insertFact(ctx, tx, kind, snapshotExactKey(ref), ref.Revision, ref.Digest, value)
}

func inspectSnapshotFact[T any](ctx context.Context, db queryer, kind string, expected contract.SnapshotArtifactExactRefV2, ref func(T) contract.SnapshotArtifactExactRefV2) (T, error) {
	var zero T
	if err := expected.ValidateShape("snapshot exact ref"); err != nil {
		return zero, err
	}
	value, err := readFact[T](ctx, db, kind, snapshotExactKey(expected))
	if err != nil {
		return zero, err
	}
	if !contract.SameSnapshotArtifactExactRef(ref(value), expected) {
		return zero, ports.ErrConflict
	}
	return value, nil
}

func snapshotExactKey(ref contract.SnapshotArtifactExactRefV2) string {
	return fmt.Sprintf("%s\x00%s\x00%d\x00%s", ref.TypeURL, ref.ID, ref.Revision, ref.Digest)
}

func checkAndAdvanceSnapshotOwnerClock(ctx context.Context, tx *sql.Tx, next int64, allowEqual bool) error {
	var current int64
	if err := tx.QueryRowContext(ctx, `SELECT watermark FROM snapshot_owner_clock WHERE singleton=1`).Scan(&current); err != nil {
		return err
	}
	if next < current || !allowEqual && next == 0 {
		return ports.ErrStale
	}
	return nil
}

func setSnapshotOwnerClock(ctx context.Context, tx *sql.Tx, next int64) error {
	result, err := tx.ExecContext(ctx, `UPDATE snapshot_owner_clock SET watermark=? WHERE singleton=1 AND watermark<=?`, next, next)
	if err != nil {
		return err
	}
	if rows, err := result.RowsAffected(); err != nil || rows != 1 {
		return ports.ErrStale
	}
	return nil
}

var _ ports.SnapshotArtifactStoreV2 = (*Store)(nil)
