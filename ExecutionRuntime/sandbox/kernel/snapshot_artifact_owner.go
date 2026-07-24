package kernel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

// Kept as a package-local alias for existing white-box tests. The actual
// persistence contract is public so production State Plane drivers can
// implement it without entering kernel internals.
type snapshotArtifactStore = ports.SnapshotArtifactStoreV2

type SnapshotArtifactOwnerLimits struct {
	MaxReservationTTL time.Duration
	MaxHistoryTTL     time.Duration
	MaxProjectionTTL  time.Duration
}

func (l SnapshotArtifactOwnerLimits) validate() error {
	if l.MaxReservationTTL <= 0 || l.MaxHistoryTTL <= 0 || l.MaxProjectionTTL <= 0 {
		return errors.New("snapshot artifact Owner TTL limits must be positive")
	}
	return nil
}

type SnapshotArtifactOwner struct {
	store         ports.SnapshotArtifactStoreV2
	now           func() time.Time
	limits        SnapshotArtifactOwnerLimits
	commitCurrent ports.SnapshotArtifactCommitCurrentReaderV2
}

func NewSnapshotArtifactOwnerWithCommitCurrent(store ports.SnapshotArtifactStoreV2, current ports.SnapshotArtifactCommitCurrentReaderV2, now func() time.Time, limits SnapshotArtifactOwnerLimits) (*SnapshotArtifactOwner, error) {
	owner, err := NewSnapshotArtifactOwner(store, now, limits)
	if err != nil {
		return nil, err
	}
	if nilInterface(current) {
		return nil, errors.New("snapshot artifact commit current Reader is required")
	}
	owner.commitCurrent = current
	return owner, nil
}

var _ ports.SnapshotArtifactOwnerPortV2 = (*SnapshotArtifactOwner)(nil)

func NewSnapshotArtifactOwner(store ports.SnapshotArtifactStoreV2, now func() time.Time, limits SnapshotArtifactOwnerLimits) (*SnapshotArtifactOwner, error) {
	if nilInterface(store) || now == nil {
		return nil, errors.New("snapshot artifact store and Owner clock are required")
	}
	if err := limits.validate(); err != nil {
		return nil, err
	}
	return &SnapshotArtifactOwner{store: store, now: now, limits: limits}, nil
}

func (o *SnapshotArtifactOwner) ReserveArtifact(ctx context.Context, input *contract.ReserveArtifactRequestV2) (contract.ReserveArtifactResultV2, error) {
	if input == nil {
		return contract.ReserveArtifactResultV2{}, errors.New("snapshot artifact reserve request is required")
	}
	request, err := cloneSnapshotOwnerValue(*input)
	if err != nil {
		return contract.ReserveArtifactResultV2{}, err
	}
	if err := request.ValidateShape(); err != nil {
		return contract.ReserveArtifactResultV2{}, fmt.Errorf("validate snapshot artifact reserve request: %w", err)
	}

	existing, inspectErr := o.store.InspectSnapshotArtifactReservationByStableKey(ctx, request.StableSourceKey())
	if inspectErr == nil {
		return o.replaySnapshotArtifactReservation(ctx, existing, request, false)
	}
	if !errors.Is(inspectErr, ports.ErrNotFound) {
		return contract.ReserveArtifactResultV2{}, inspectErr
	}

	pre := o.now()
	if err := request.ValidateCurrent(pre); err != nil {
		return contract.ReserveArtifactResultV2{}, fmt.Errorf("validate current snapshot artifact reserve request: %w", err)
	}
	post := o.now()
	if post.IsZero() || post.Before(pre) {
		return contract.ReserveArtifactResultV2{}, fmt.Errorf("%w: snapshot artifact Owner clock moved backwards", ports.ErrStale)
	}
	bundle, err := o.buildReservedBundle(request, pre, post)
	if err != nil {
		return contract.ReserveArtifactResultV2{}, err
	}
	created, err := o.store.CreateReservedSnapshotArtifact(ctx, bundle)
	if err != nil {
		recovered, recoverErr := o.store.InspectSnapshotArtifactReservationByStableKey(ctx, request.StableSourceKey())
		if recoverErr == nil {
			result, replayErr := o.replaySnapshotArtifactReservation(ctx, recovered, request, true)
			if replayErr == nil && contract.SameSnapshotArtifactExactRef(recovered.ExactRef(), bundle.Reservation.ExactRef()) {
				return result, nil
			}
			if replayErr != nil {
				return contract.ReserveArtifactResultV2{}, replayErr
			}
		}
		return contract.ReserveArtifactResultV2{}, err
	}
	if !created {
		recovered, recoverErr := o.store.InspectSnapshotArtifactReservationByStableKey(ctx, request.StableSourceKey())
		if recoverErr != nil {
			return contract.ReserveArtifactResultV2{}, recoverErr
		}
		return o.replaySnapshotArtifactReservation(ctx, recovered, request, false)
	}
	return contract.ReserveArtifactResultV2{Reservation: bundle.Reservation, CurrentIndex: bundle.CurrentIndex, Created: true}, nil
}

func (o *SnapshotArtifactOwner) replaySnapshotArtifactReservation(ctx context.Context, existing contract.SnapshotArtifactReservationV2, request contract.ReserveArtifactRequestV2, created bool) (contract.ReserveArtifactResultV2, error) {
	if err := existing.ValidateShape(); err != nil {
		return contract.ReserveArtifactResultV2{}, fmt.Errorf("stored snapshot artifact reservation is invalid: %w", err)
	}
	if !contract.SnapshotArtifactReservationMatchesRequest(existing, request) {
		return contract.ReserveArtifactResultV2{}, fmt.Errorf("%w: snapshot artifact stable source key already has different content", ports.ErrConflict)
	}
	current, err := o.store.InspectSnapshotArtifactCurrentIndex(ctx, existing.SubjectRef.ArtifactAggregateID)
	if err != nil || current.ValidateCurrent(o.now()) != nil {
		return contract.ReserveArtifactResultV2{}, fmt.Errorf("%w: snapshot artifact reserved current index is unavailable", ports.ErrStale)
	}
	return contract.ReserveArtifactResultV2{Reservation: existing, CurrentIndex: current, Created: created}, nil
}

func (o *SnapshotArtifactOwner) buildReservedBundle(request contract.ReserveArtifactRequestV2, pre, post time.Time) (contract.SnapshotArtifactReservedBundleV2, error) {
	stableDigest, err := contract.SnapshotArtifactStableSourceKeyDigest(request.StableSourceKey())
	if err != nil {
		return contract.SnapshotArtifactReservedBundleV2{}, err
	}
	reservationID := "snapshot-reservation-" + stableDigest[:32]
	aggregateID := "snapshot-aggregate-" + stableDigest[:32]
	reservationExpiry := minTimeBound(request.RequestedNotAfter, pre.Add(o.limits.MaxReservationTTL).UnixNano())
	historyExpiry := minTimeBound(request.RequestedNotAfter, pre.Add(o.limits.MaxHistoryTTL).UnixNano())
	projectionExpiry := minTimeBound(request.RequestedNotAfter, reservationExpiry, pre.Add(o.limits.MaxProjectionTTL).UnixNano())
	if post.UnixNano() >= reservationExpiry || post.UnixNano() >= historyExpiry || post.UnixNano() >= projectionExpiry {
		return contract.SnapshotArtifactReservedBundleV2{}, fmt.Errorf("%w: snapshot artifact Owner TTL expired before commit", ports.ErrStale)
	}

	identity, err := contract.SealSnapshotArtifactSubjectIdentityV2(contract.SnapshotArtifactSubjectIdentityV2{
		ArtifactAggregateID: aggregateID,
		TenantID:            request.TenantID,
		DataDomain:          request.DataDomain,
		ReservationID:       reservationID,
		SourceAttemptID:     request.SourceAttemptRef.ID,
	})
	if err != nil {
		return contract.SnapshotArtifactReservedBundleV2{}, err
	}
	subjectRef, err := contract.SealSnapshotArtifactSubjectRefV2(contract.SnapshotArtifactSubjectRefV2{
		ArtifactAggregateID: aggregateID,
		Revision:            1,
		TenantID:            request.TenantID,
		DataDomain:          request.DataDomain,
		ReservationID:       reservationID,
		SourceAttemptID:     request.SourceAttemptRef.ID,
		SchemaRef:           snapshotArtifactSchemaRef(contract.SnapshotArtifactSubjectRefTypeURL),
		StableSubjectDigest: identity.StableSubjectDigest,
		ExpiresUnixNano:     reservationExpiry,
	})
	if err != nil {
		return contract.SnapshotArtifactReservedBundleV2{}, err
	}
	reservation, err := contract.SealSnapshotArtifactReservationV2(contract.SnapshotArtifactReservationV2{
		Meta: contract.Meta{
			ContractVersion: contract.ContractFamily,
			ID:              reservationID, Revision: 1,
			CreatedUnixNano: pre.UnixNano(), UpdatedUnixNano: pre.UnixNano(), ExpiresUnixNano: reservationExpiry,
		},
		TenantID: request.TenantID, DataDomain: request.DataDomain,
		SourceOperationID: request.SourceOperationID, SourceEffectID: request.SourceEffectID,
		SourceAttemptRef: request.SourceAttemptRef, SchemaRef: request.SchemaRef,
		ExpectedContentDigest: request.ExpectedContentDigest, RetentionPolicyRef: request.RetentionPolicyRef,
		EncryptionPolicyRef: request.EncryptionPolicyRef, ResidencyPolicyRef: request.ResidencyPolicyRef,
		ExpectedAggregateRef: request.ExpectedAggregateRef, RequestedNotAfter: request.RequestedNotAfter,
		SubjectIdentity: identity, SubjectRef: subjectRef,
	})
	if err != nil {
		return contract.SnapshotArtifactReservedBundleV2{}, err
	}
	reservationFact, err := contract.SealSnapshotArtifactReservationFactV2(contract.SnapshotArtifactReservationFactV2{
		Meta: contract.Meta{
			ContractVersion: contract.ContractFamily,
			ID:              reservationID + "-fact", Revision: 1,
			CreatedUnixNano: pre.UnixNano(), UpdatedUnixNano: pre.UnixNano(), ExpiresUnixNano: reservationExpiry,
		},
		TenantID: request.TenantID, ReservationRef: reservation.ExactRef(), ArtifactSubjectRef: subjectRef,
		RequestedNotAfter: request.RequestedNotAfter,
	})
	if err != nil {
		return contract.SnapshotArtifactReservedBundleV2{}, err
	}
	entry, err := contract.SealSnapshotArtifactAggregateEntryV2(contract.SnapshotArtifactAggregateEntryV2{
		Meta: contract.Meta{
			ContractVersion: contract.ContractFamily,
			ID:              aggregateID + "-entry-reservation", Revision: 1,
			CreatedUnixNano: pre.UnixNano(), UpdatedUnixNano: pre.UnixNano(), ExpiresUnixNano: historyExpiry,
		},
		TenantID: request.TenantID, ArtifactAggregateID: aggregateID, ArtifactSubjectRef: subjectRef,
		EntryKind: contract.SnapshotArtifactEntryReservation, FactRef: reservationFact.ExactRef(),
		PreviousEntryRef:  contract.SnapshotArtifactOptionalExactRefV2{Presence: contract.SnapshotArtifactAbsent},
		RequestedNotAfter: request.RequestedNotAfter,
	})
	if err != nil {
		return contract.SnapshotArtifactReservedBundleV2{}, err
	}
	envelope, err := contract.SealSnapshotArtifactAggregateEnvelopeV2(contract.SnapshotArtifactAggregateEnvelopeV2{
		AggregateRef: contract.SnapshotArtifactAggregateRefV2{
			AggregateID: aggregateID, Revision: 1, TenantID: request.TenantID, DataDomain: request.DataDomain,
			SchemaRef: snapshotArtifactSchemaRef(contract.SnapshotArtifactAggregateRefTypeURL), ExpiresUnixNano: historyExpiry,
		},
		RequestedNotAfter:    request.RequestedNotAfter,
		PreviousAggregateRef: contract.SnapshotArtifactOptionalAggregateRefV2{Presence: contract.SnapshotArtifactAbsent},
		AppliedEntryRef:      entry.ExactRef(), ReservationFactRef: reservationFact.ExactRef(),
		ArtifactFactRef:                  contract.SnapshotArtifactOptionalExactRefV2{Presence: contract.SnapshotArtifactAbsent},
		RetentionApplicationFactRef:      contract.SnapshotArtifactOptionalExactRefV2{Presence: contract.SnapshotArtifactAbsent},
		ActiveDeletionAttemptFactRef:     contract.SnapshotArtifactOptionalExactRefV2{Presence: contract.SnapshotArtifactAbsent},
		LastClosedDeletionAttemptFactRef: contract.SnapshotArtifactOptionalExactRefV2{Presence: contract.SnapshotArtifactAbsent},
		TerminalTombstoneRef:             contract.SnapshotArtifactOptionalExactRefV2{Presence: contract.SnapshotArtifactAbsent},
		AggregateState:                   contract.SnapshotArtifactAggregateReserved,
	})
	if err != nil {
		return contract.SnapshotArtifactReservedBundleV2{}, err
	}
	activeTTLClosure, err := contract.Digest("snapshot-artifact-reserved-active-ttl-closure-v2", struct {
		Reservation contract.SnapshotArtifactExactRefV2
		Subject     contract.SnapshotArtifactExactRefV2
		Expires     int64
	}{Reservation: reservationFact.ExactRef(), Subject: subjectRef.ExactRef(), Expires: projectionExpiry})
	if err != nil {
		return contract.SnapshotArtifactReservedBundleV2{}, err
	}
	reservationFactRef := reservationFact.ExactRef()
	currentIndex, err := contract.SealSnapshotArtifactAggregateCurrentIndexV2(contract.SnapshotArtifactAggregateCurrentIndexV2{
		CurrentIndexRef:     contract.SnapshotArtifactExactRefV2{ID: aggregateID + "-current", Revision: 1, ExpiresUnixNano: projectionExpiry},
		ArtifactAggregateID: aggregateID, ArtifactSubjectRef: subjectRef, HeadAggregateEnvelopeRef: envelope.AggregateRef,
		AggregateState:                 contract.SnapshotArtifactAggregateReserved,
		ReservationCurrentRef:          contract.SnapshotArtifactOptionalExactRefV2{Presence: contract.SnapshotArtifactPresent, Ref: &reservationFactRef},
		ArtifactFactRef:                contract.SnapshotArtifactOptionalExactRefV2{Presence: contract.SnapshotArtifactAbsent},
		RetentionApplicationCurrentRef: contract.SnapshotArtifactOptionalExactRefV2{Presence: contract.SnapshotArtifactAbsent},
		ActiveDeletionAttemptFactRef:   contract.SnapshotArtifactOptionalExactRefV2{Presence: contract.SnapshotArtifactAbsent},
		TerminalTombstoneRef:           contract.SnapshotArtifactOptionalExactRefV2{Presence: contract.SnapshotArtifactAbsent},
		ActiveTTLClosureDigest:         activeTTLClosure, OwnerClockWatermark: post.UnixNano(), CheckedUnixNano: post.UnixNano(),
		RequestedNotAfter: request.RequestedNotAfter,
	})
	if err != nil {
		return contract.SnapshotArtifactReservedBundleV2{}, err
	}
	bundle := contract.SnapshotArtifactReservedBundleV2{
		StableKey: request.StableSourceKey(), Reservation: reservation, ReservationFact: reservationFact,
		Entry: entry, Envelope: envelope, CurrentIndex: currentIndex, OwnerClockWatermark: post.UnixNano(),
	}
	return bundle, bundle.ValidateShape()
}

func (o *SnapshotArtifactOwner) InspectReservation(ctx context.Context, input *contract.InspectSnapshotArtifactReservationRequestV2) (contract.SnapshotArtifactReservationV2, error) {
	if input == nil {
		return contract.SnapshotArtifactReservationV2{}, errors.New("snapshot artifact reservation inspect request is required")
	}
	if err := input.ValidateShape(); err != nil {
		return contract.SnapshotArtifactReservationV2{}, err
	}
	value, err := o.store.InspectSnapshotArtifactReservation(ctx, input.ExpectedRef)
	if err != nil {
		return contract.SnapshotArtifactReservationV2{}, err
	}
	if err := value.ValidateShape(); err != nil || !contract.SameSnapshotArtifactExactRef(value.ExactRef(), input.ExpectedRef) {
		return contract.SnapshotArtifactReservationV2{}, fmt.Errorf("%w: snapshot artifact reservation exact ref drifted", ports.ErrConflict)
	}
	return cloneSnapshotOwnerValue(value)
}

func (o *SnapshotArtifactOwner) InspectReservationByStableKey(ctx context.Context, input *contract.InspectSnapshotArtifactReservationByStableKeyRequestV2) (contract.SnapshotArtifactReservationV2, error) {
	if input == nil {
		return contract.SnapshotArtifactReservationV2{}, errors.New("snapshot artifact stable-key inspect request is required")
	}
	if err := input.ValidateShape(); err != nil {
		return contract.SnapshotArtifactReservationV2{}, err
	}
	value, err := o.store.InspectSnapshotArtifactReservationByStableKey(ctx, input.StableKey)
	if err != nil {
		return contract.SnapshotArtifactReservationV2{}, err
	}
	if err := value.ValidateShape(); err != nil || value.StableSourceKey() != input.StableKey {
		return contract.SnapshotArtifactReservationV2{}, fmt.Errorf("%w: snapshot artifact stable-key binding drifted", ports.ErrConflict)
	}
	return cloneSnapshotOwnerValue(value)
}

func (o *SnapshotArtifactOwner) InspectAggregateHistorical(ctx context.Context, input *contract.InspectSnapshotArtifactAggregateHistoricalRequestV2) (contract.SnapshotArtifactAggregateEnvelopeV2, error) {
	if input == nil {
		return contract.SnapshotArtifactAggregateEnvelopeV2{}, errors.New("snapshot artifact aggregate historical request is required")
	}
	if err := input.ValidateShape(); err != nil {
		return contract.SnapshotArtifactAggregateEnvelopeV2{}, err
	}
	value, err := o.store.InspectSnapshotArtifactEnvelope(ctx, input.ExpectedRef)
	if err != nil {
		return contract.SnapshotArtifactAggregateEnvelopeV2{}, err
	}
	if err := value.ValidateShape(); err != nil || !contract.SameSnapshotArtifactAggregateRef(value.AggregateRef, input.ExpectedRef) {
		return contract.SnapshotArtifactAggregateEnvelopeV2{}, fmt.Errorf("%w: snapshot artifact aggregate historical ref drifted", ports.ErrConflict)
	}
	return cloneSnapshotOwnerValue(value)
}

func (o *SnapshotArtifactOwner) InspectEntryHistorical(ctx context.Context, input *contract.InspectSnapshotArtifactEntryHistoricalRequestV2) (contract.SnapshotArtifactAggregateEntryV2, error) {
	if input == nil {
		return contract.SnapshotArtifactAggregateEntryV2{}, errors.New("snapshot artifact entry historical request is required")
	}
	if err := input.ValidateShape(); err != nil {
		return contract.SnapshotArtifactAggregateEntryV2{}, err
	}
	value, err := o.store.InspectSnapshotArtifactEntry(ctx, input.ExpectedRef)
	if err != nil {
		return contract.SnapshotArtifactAggregateEntryV2{}, err
	}
	if err := value.ValidateShape(); err != nil || !contract.SameSnapshotArtifactExactRef(value.ExactRef(), input.ExpectedRef) {
		return contract.SnapshotArtifactAggregateEntryV2{}, fmt.Errorf("%w: snapshot artifact entry historical ref drifted", ports.ErrConflict)
	}
	return cloneSnapshotOwnerValue(value)
}

func (o *SnapshotArtifactOwner) InspectAggregateCurrent(ctx context.Context, input *contract.InspectSnapshotArtifactAggregateCurrentRequestV2) (contract.SnapshotArtifactAggregateCurrentProjectionV2, error) {
	if input == nil {
		return contract.SnapshotArtifactAggregateCurrentProjectionV2{}, errors.New("snapshot artifact aggregate current request is required")
	}
	request, err := cloneSnapshotOwnerValue(*input)
	if err != nil {
		return contract.SnapshotArtifactAggregateCurrentProjectionV2{}, err
	}
	now := o.now()
	if err := request.ValidateCurrent(now); err != nil {
		return contract.SnapshotArtifactAggregateCurrentProjectionV2{}, err
	}
	index, err := o.store.InspectSnapshotArtifactCurrentIndex(ctx, request.ArtifactAggregateID)
	if err != nil {
		return contract.SnapshotArtifactAggregateCurrentProjectionV2{}, err
	}
	if err := index.ValidateCurrent(now); err != nil {
		return contract.SnapshotArtifactAggregateCurrentProjectionV2{}, fmt.Errorf("%w: validate snapshot artifact current index: %v", ports.ErrStale, err)
	}
	if request.ExpectedAggregateRef.Presence != contract.SnapshotArtifactPresent || request.ExpectedAggregateRef.Ref == nil || !contract.SameSnapshotArtifactAggregateRef(index.HeadAggregateEnvelopeRef, *request.ExpectedAggregateRef.Ref) {
		return contract.SnapshotArtifactAggregateCurrentProjectionV2{}, fmt.Errorf("%w: snapshot artifact expected aggregate is not current", ports.ErrStale)
	}
	envelope, err := o.store.InspectSnapshotArtifactEnvelope(ctx, index.HeadAggregateEnvelopeRef)
	if err != nil {
		return contract.SnapshotArtifactAggregateCurrentProjectionV2{}, err
	}
	if err := envelope.ValidateShape(); err != nil || !contract.SameSnapshotArtifactAggregateRef(envelope.AggregateRef, index.HeadAggregateEnvelopeRef) {
		return contract.SnapshotArtifactAggregateCurrentProjectionV2{}, fmt.Errorf("%w: snapshot artifact current envelope binding drifted", ports.ErrConflict)
	}
	reservationFactRef := envelope.ReservationFactRef
	artifactFactRef := contract.SnapshotArtifactOptionalExactRefV2{Presence: contract.SnapshotArtifactAbsent}
	expires := minTimeBound(request.RequestedNotAfter, index.CurrentIndexRef.ExpiresUnixNano, now.Add(o.limits.MaxProjectionTTL).UnixNano())
	switch index.AggregateState {
	case contract.SnapshotArtifactAggregateReserved:
		if index.ReservationCurrentRef.Ref == nil {
			return contract.SnapshotArtifactAggregateCurrentProjectionV2{}, fmt.Errorf("%w: reserved snapshot artifact lacks reservation current", ports.ErrConflict)
		}
		reservationFact, inspectErr := o.store.InspectSnapshotArtifactReservationFact(ctx, *index.ReservationCurrentRef.Ref)
		if inspectErr != nil {
			return contract.SnapshotArtifactAggregateCurrentProjectionV2{}, inspectErr
		}
		if err := reservationFact.ValidateShape(); err != nil || reservationFact.Meta.ValidateCurrent(now) != nil {
			return contract.SnapshotArtifactAggregateCurrentProjectionV2{}, fmt.Errorf("%w: snapshot artifact reservation fact is not current", ports.ErrStale)
		}
		reservation, inspectErr := o.store.InspectSnapshotArtifactReservation(ctx, reservationFact.ReservationRef)
		if inspectErr != nil {
			return contract.SnapshotArtifactAggregateCurrentProjectionV2{}, inspectErr
		}
		if err := reservation.ValidateCurrent(now); err != nil || !contract.SameSnapshotArtifactExactRef(reservation.ExactRef(), reservationFact.ReservationRef) || !contract.SameSnapshotArtifactExactRef(reservationFact.ExactRef(), *index.ReservationCurrentRef.Ref) || !contract.SameSnapshotArtifactExactRef(reservationFact.ExactRef(), envelope.ReservationFactRef) {
			return contract.SnapshotArtifactAggregateCurrentProjectionV2{}, fmt.Errorf("%w: snapshot artifact current reservation binding drifted", ports.ErrConflict)
		}
		reservationFactRef = reservationFact.ExactRef()
		expires = minTimeBound(expires, reservationFact.Meta.ExpiresUnixNano, reservation.Meta.ExpiresUnixNano)
	case contract.SnapshotArtifactAggregateAvailable:
		if index.ArtifactFactRef.Ref == nil || envelope.ArtifactFactRef.Ref == nil || !contract.SameSnapshotArtifactExactRef(*index.ArtifactFactRef.Ref, *envelope.ArtifactFactRef.Ref) {
			return contract.SnapshotArtifactAggregateCurrentProjectionV2{}, fmt.Errorf("%w: available snapshot artifact lacks exact fact current", ports.ErrConflict)
		}
		fact, inspectErr := o.store.InspectSnapshotArtifactFact(ctx, *index.ArtifactFactRef.Ref)
		if inspectErr != nil {
			return contract.SnapshotArtifactAggregateCurrentProjectionV2{}, inspectErr
		}
		if err := fact.ValidateCurrent(now); err != nil || !contract.SameSnapshotArtifactExactRef(fact.ExactRef(), *index.ArtifactFactRef.Ref) || !contract.SameSnapshotArtifactExactRef(fact.ReservationFactRef, envelope.ReservationFactRef) {
			return contract.SnapshotArtifactAggregateCurrentProjectionV2{}, fmt.Errorf("%w: available snapshot artifact fact is stale or drifted", ports.ErrStale)
		}
		factRef := fact.ExactRef()
		artifactFactRef = contract.SnapshotArtifactOptionalExactRefV2{Presence: contract.SnapshotArtifactPresent, Ref: &factRef}
		expires = minTimeBound(expires, fact.Meta.ExpiresUnixNano)
	default:
		return contract.SnapshotArtifactAggregateCurrentProjectionV2{}, fmt.Errorf("%w: snapshot artifact current state is unsupported", ports.ErrUnsupported)
	}
	refreshed, err := o.store.InspectSnapshotArtifactCurrentIndex(ctx, request.ArtifactAggregateID)
	if err != nil {
		return contract.SnapshotArtifactAggregateCurrentProjectionV2{}, err
	}
	if err := refreshed.ValidateCurrent(now); err != nil {
		return contract.SnapshotArtifactAggregateCurrentProjectionV2{}, fmt.Errorf("%w: refreshed snapshot artifact current index is invalid", ports.ErrStale)
	}
	if !contract.SameSnapshotArtifactExactRef(index.CurrentIndexRef, refreshed.CurrentIndexRef) {
		return contract.SnapshotArtifactAggregateCurrentProjectionV2{}, fmt.Errorf("%w: snapshot artifact current pointer changed during read", ports.ErrStale)
	}
	if now.UnixNano() >= expires {
		return contract.SnapshotArtifactAggregateCurrentProjectionV2{}, fmt.Errorf("%w: snapshot artifact current projection expired", ports.ErrStale)
	}
	projection, err := contract.SealSnapshotArtifactAggregateCurrentProjectionV2(contract.SnapshotArtifactAggregateCurrentProjectionV2{
		AggregateCurrentIndexRef: index.CurrentIndexRef,
		HeadAggregateEnvelopeRef: index.HeadAggregateEnvelopeRef,
		ArtifactSubjectRef:       index.ArtifactSubjectRef,
		ReservationFactRef:       reservationFactRef,
		ArtifactFactRef:          artifactFactRef,
		AggregateState:           index.AggregateState,
		ActiveTTLClosureDigest:   index.ActiveTTLClosureDigest,
		OwnerComputedCurrent:     true,
		CheckedUnixNano:          now.UnixNano(), RequestedNotAfter: request.RequestedNotAfter, ExpiresUnixNano: expires,
	})
	if err != nil {
		return contract.SnapshotArtifactAggregateCurrentProjectionV2{}, err
	}
	return cloneSnapshotOwnerValue(projection)
}

func snapshotArtifactSchemaRef(typeURL string) contract.Ref {
	digest, err := contract.Digest("snapshot-artifact-schema-ref-v2", struct {
		TypeURL string `json:"type_url"`
		Version uint32 `json:"version"`
	}{TypeURL: typeURL, Version: contract.SnapshotArtifactVersion})
	if err != nil {
		panic(err)
	}
	return contract.Ref{ID: typeURL, Revision: uint64(contract.SnapshotArtifactVersion), Digest: digest}
}

func minTimeBound(values ...int64) int64 {
	if len(values) == 0 {
		return 0
	}
	result := values[0]
	for _, value := range values[1:] {
		if value < result {
			result = value
		}
	}
	return result
}

func cloneSnapshotOwnerValue[T any](value T) (T, error) {
	var zero T
	payload, err := json.Marshal(value)
	if err != nil {
		return zero, err
	}
	var cloned T
	if err := json.Unmarshal(payload, &cloned); err != nil {
		return zero, err
	}
	return cloned, nil
}
