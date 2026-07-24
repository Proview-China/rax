package kernel

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

func (o *SnapshotArtifactOwner) CommitArtifact(ctx context.Context, input *contract.CommitSnapshotArtifactRequestV2) (contract.CommitSnapshotArtifactResultV2, error) {
	if input == nil {
		return contract.CommitSnapshotArtifactResultV2{}, errors.New("snapshot artifact commit request is required")
	}
	if nilInterface(o.commitCurrent) {
		return contract.CommitSnapshotArtifactResultV2{}, fmt.Errorf("%w: snapshot artifact commit current Reader is not installed", ports.ErrUnsupported)
	}
	request, err := cloneSnapshotOwnerValue(*input)
	if err != nil {
		return contract.CommitSnapshotArtifactResultV2{}, err
	}
	pre := o.now()
	if err := request.ValidateCurrent(pre); err != nil {
		return contract.CommitSnapshotArtifactResultV2{}, err
	}
	if replay, replayErr := o.recoverAvailableCommitByRequest(ctx, request); replayErr == nil {
		return replay, nil
	}
	reservation, reservationFact, current, previousEnvelope, previousEntry, err := o.inspectReservedCommitBase(ctx, request, pre)
	if err != nil {
		return contract.CommitSnapshotArtifactResultV2{}, err
	}
	s1, err := o.commitCurrent.InspectSnapshotArtifactCommitCurrentV2(ctx, request)
	if err != nil {
		return contract.CommitSnapshotArtifactResultV2{}, err
	}
	if err := validateSnapshotArtifactCommitCurrentV2(s1, request, reservation, pre); err != nil {
		return contract.CommitSnapshotArtifactResultV2{}, err
	}
	post := o.now()
	if post.IsZero() || post.Before(pre) {
		return contract.CommitSnapshotArtifactResultV2{}, fmt.Errorf("%w: snapshot artifact Owner clock moved backwards", ports.ErrStale)
	}
	bundle, err := o.buildAvailableBundle(request, reservation, reservationFact, current, previousEnvelope, previousEntry, s1, pre, post)
	if err != nil {
		return contract.CommitSnapshotArtifactResultV2{}, err
	}
	s2, err := o.commitCurrent.InspectSnapshotArtifactCommitCurrentV2(ctx, request)
	if err != nil {
		return contract.CommitSnapshotArtifactResultV2{}, err
	}
	if err := validateSnapshotArtifactCommitCurrentV2(s2, request, reservation, post); err != nil {
		return contract.CommitSnapshotArtifactResultV2{}, err
	}
	if s2.ProjectionDigest != s1.ProjectionDigest {
		return contract.CommitSnapshotArtifactResultV2{}, fmt.Errorf("%w: snapshot artifact commit current changed between S1 and S2", ports.ErrConflict)
	}
	created, err := o.store.CommitAvailableSnapshotArtifact(ctx, bundle)
	if err != nil {
		if recovered, recoverErr := o.recoverAvailableCommitByRequest(ctx, request); recoverErr == nil {
			recovered.Created = true
			return recovered, nil
		}
		return contract.CommitSnapshotArtifactResultV2{}, err
	}
	if !created {
		return o.recoverAvailableCommitByRequest(ctx, request)
	}
	return contract.CommitSnapshotArtifactResultV2{Fact: bundle.Fact, CurrentIndex: bundle.CurrentIndex, Created: true}, nil
}

func (o *SnapshotArtifactOwner) inspectReservedCommitBase(ctx context.Context, request contract.CommitSnapshotArtifactRequestV2, now time.Time) (contract.SnapshotArtifactReservationV2, contract.SnapshotArtifactReservationFactV2, contract.SnapshotArtifactAggregateCurrentIndexV2, contract.SnapshotArtifactAggregateEnvelopeV2, contract.SnapshotArtifactAggregateEntryV2, error) {
	reservation, err := o.store.InspectSnapshotArtifactReservation(ctx, request.ReservationRef)
	if err != nil {
		return contract.SnapshotArtifactReservationV2{}, contract.SnapshotArtifactReservationFactV2{}, contract.SnapshotArtifactAggregateCurrentIndexV2{}, contract.SnapshotArtifactAggregateEnvelopeV2{}, contract.SnapshotArtifactAggregateEntryV2{}, err
	}
	if err := reservation.ValidateCurrent(now); err != nil || !contract.SameSnapshotArtifactExactRef(reservation.ExactRef(), request.ReservationRef) || !contract.SameRef(reservation.SourceAttemptRef, request.SourceAttemptRef) || reservation.ExpectedContentDigest != request.StorageArtifactRef.ContentDigest || reservation.TenantID != request.StorageArtifactRef.TenantID || reservation.DataDomain != request.StorageArtifactRef.DataDomain {
		return contract.SnapshotArtifactReservationV2{}, contract.SnapshotArtifactReservationFactV2{}, contract.SnapshotArtifactAggregateCurrentIndexV2{}, contract.SnapshotArtifactAggregateEnvelopeV2{}, contract.SnapshotArtifactAggregateEntryV2{}, fmt.Errorf("%w: snapshot artifact reservation is stale or mismatched", ports.ErrConflict)
	}
	current, err := o.store.InspectSnapshotArtifactCurrentIndex(ctx, reservation.SubjectRef.ArtifactAggregateID)
	if err != nil {
		return contract.SnapshotArtifactReservationV2{}, contract.SnapshotArtifactReservationFactV2{}, contract.SnapshotArtifactAggregateCurrentIndexV2{}, contract.SnapshotArtifactAggregateEnvelopeV2{}, contract.SnapshotArtifactAggregateEntryV2{}, err
	}
	if err := current.ValidateCurrent(now); err != nil || current.AggregateState != contract.SnapshotArtifactAggregateReserved || !contract.SameSnapshotArtifactAggregateRef(current.HeadAggregateEnvelopeRef, request.ExpectedAggregateRef) || current.ReservationCurrentRef.Ref == nil {
		return contract.SnapshotArtifactReservationV2{}, contract.SnapshotArtifactReservationFactV2{}, contract.SnapshotArtifactAggregateCurrentIndexV2{}, contract.SnapshotArtifactAggregateEnvelopeV2{}, contract.SnapshotArtifactAggregateEntryV2{}, fmt.Errorf("%w: snapshot artifact expected reserved aggregate is not current", ports.ErrStale)
	}
	reservationFact, err := o.store.InspectSnapshotArtifactReservationFact(ctx, *current.ReservationCurrentRef.Ref)
	if err != nil {
		return contract.SnapshotArtifactReservationV2{}, contract.SnapshotArtifactReservationFactV2{}, contract.SnapshotArtifactAggregateCurrentIndexV2{}, contract.SnapshotArtifactAggregateEnvelopeV2{}, contract.SnapshotArtifactAggregateEntryV2{}, err
	}
	if err := reservationFact.ValidateShape(); err != nil || !contract.SameSnapshotArtifactExactRef(reservationFact.ReservationRef, reservation.ExactRef()) {
		return contract.SnapshotArtifactReservationV2{}, contract.SnapshotArtifactReservationFactV2{}, contract.SnapshotArtifactAggregateCurrentIndexV2{}, contract.SnapshotArtifactAggregateEnvelopeV2{}, contract.SnapshotArtifactAggregateEntryV2{}, fmt.Errorf("%w: snapshot artifact reservation fact drifted", ports.ErrConflict)
	}
	envelope, err := o.store.InspectSnapshotArtifactEnvelope(ctx, current.HeadAggregateEnvelopeRef)
	if err != nil {
		return contract.SnapshotArtifactReservationV2{}, contract.SnapshotArtifactReservationFactV2{}, contract.SnapshotArtifactAggregateCurrentIndexV2{}, contract.SnapshotArtifactAggregateEnvelopeV2{}, contract.SnapshotArtifactAggregateEntryV2{}, err
	}
	entry, err := o.store.InspectSnapshotArtifactEntry(ctx, envelope.AppliedEntryRef)
	if err != nil {
		return contract.SnapshotArtifactReservationV2{}, contract.SnapshotArtifactReservationFactV2{}, contract.SnapshotArtifactAggregateCurrentIndexV2{}, contract.SnapshotArtifactAggregateEnvelopeV2{}, contract.SnapshotArtifactAggregateEntryV2{}, err
	}
	if envelope.ValidateShape() != nil || entry.ValidateShape() != nil || entry.EntryKind != contract.SnapshotArtifactEntryReservation || !contract.SameSnapshotArtifactExactRef(entry.ExactRef(), envelope.AppliedEntryRef) {
		return contract.SnapshotArtifactReservationV2{}, contract.SnapshotArtifactReservationFactV2{}, contract.SnapshotArtifactAggregateCurrentIndexV2{}, contract.SnapshotArtifactAggregateEnvelopeV2{}, contract.SnapshotArtifactAggregateEntryV2{}, fmt.Errorf("%w: snapshot artifact reserved history drifted", ports.ErrConflict)
	}
	return reservation, reservationFact, current, envelope, entry, nil
}

func validateSnapshotArtifactCommitCurrentV2(current contract.SnapshotArtifactCommitCurrentProjectionV2, request contract.CommitSnapshotArtifactRequestV2, reservation contract.SnapshotArtifactReservationV2, now time.Time) error {
	if err := current.ValidateCurrent(now); err != nil {
		return err
	}
	if !current.MatchesRequest(request) || current.TenantID != reservation.TenantID || current.DataDomain != reservation.DataDomain {
		return fmt.Errorf("%w: snapshot artifact commit current projection is cross-scoped or drifted", ports.ErrConflict)
	}
	return nil
}

func (o *SnapshotArtifactOwner) buildAvailableBundle(request contract.CommitSnapshotArtifactRequestV2, reservation contract.SnapshotArtifactReservationV2, reservationFact contract.SnapshotArtifactReservationFactV2, current contract.SnapshotArtifactAggregateCurrentIndexV2, previousEnvelope contract.SnapshotArtifactAggregateEnvelopeV2, previousEntry contract.SnapshotArtifactAggregateEntryV2, proof contract.SnapshotArtifactCommitCurrentProjectionV2, pre, post time.Time) (contract.SnapshotArtifactAvailableBundleV2, error) {
	factExpiry := minTimeBound(request.RequestedNotAfter, reservation.SubjectRef.ExpiresUnixNano, request.StorageArtifactRef.ExpiresUnixNano, proof.ExpiresUnixNano, pre.Add(o.limits.MaxReservationTTL).UnixNano())
	historyExpiry := minTimeBound(request.RequestedNotAfter, pre.Add(o.limits.MaxHistoryTTL).UnixNano())
	projectionExpiry := minTimeBound(factExpiry, pre.Add(o.limits.MaxProjectionTTL).UnixNano())
	if post.UnixNano() >= factExpiry || post.UnixNano() >= historyExpiry || post.UnixNano() >= projectionExpiry {
		return contract.SnapshotArtifactAvailableBundleV2{}, fmt.Errorf("%w: snapshot artifact available TTL expired before commit", ports.ErrStale)
	}
	fact, err := contract.SealSnapshotArtifactFactV2(contract.SnapshotArtifactFactV2{
		Meta:     contract.Meta{ContractVersion: contract.ContractFamily, ID: reservation.SubjectRef.ArtifactAggregateID + "-fact", Revision: 1, CreatedUnixNano: pre.UnixNano(), UpdatedUnixNano: pre.UnixNano(), ExpiresUnixNano: factExpiry},
		TenantID: reservation.TenantID, DataDomain: reservation.DataDomain, ReservationFactRef: reservationFact.ExactRef(), ArtifactSubjectRef: reservation.SubjectRef,
		StorageArtifactRef: request.StorageArtifactRef, SchemaRef: request.StorageArtifactRef.SchemaRef, ContentDigest: request.StorageArtifactRef.ContentDigest, Length: request.StorageArtifactRef.Length,
		EncryptionFactRef: request.StorageArtifactRef.EncryptionFactRef, ResidencyFactRef: request.StorageArtifactRef.ResidencyFactRef, ProviderObservationRef: request.ProviderObservationRef,
		ProviderReceiptRef: request.ProviderReceiptRef, FormalEvidenceRefs: append([]contract.Ref(nil), request.FormalEvidenceRefs...), OwnerInspectionRef: request.OwnerInspectionRef,
		SourceAttemptRef: request.SourceAttemptRef, RequestedNotAfter: request.RequestedNotAfter, State: contract.SnapshotArtifactAvailable,
	})
	if err != nil {
		return contract.SnapshotArtifactAvailableBundleV2{}, err
	}
	previousEntryRef := previousEntry.ExactRef()
	entry, err := contract.SealSnapshotArtifactAggregateEntryV2(contract.SnapshotArtifactAggregateEntryV2{
		Meta:     contract.Meta{ContractVersion: contract.ContractFamily, ID: reservation.SubjectRef.ArtifactAggregateID + "-entry-artifact", Revision: 2, CreatedUnixNano: previousEntry.Meta.CreatedUnixNano, UpdatedUnixNano: pre.UnixNano(), ExpiresUnixNano: historyExpiry},
		TenantID: reservation.TenantID, ArtifactAggregateID: reservation.SubjectRef.ArtifactAggregateID, ArtifactSubjectRef: reservation.SubjectRef,
		EntryKind: contract.SnapshotArtifactEntryArtifact, FactRef: fact.ExactRef(), PreviousEntryRef: contract.SnapshotArtifactOptionalExactRefV2{Presence: contract.SnapshotArtifactPresent, Ref: &previousEntryRef}, RequestedNotAfter: request.RequestedNotAfter,
	})
	if err != nil {
		return contract.SnapshotArtifactAvailableBundleV2{}, err
	}
	factRef := fact.ExactRef()
	previousAggregate := previousEnvelope.AggregateRef
	envelope, err := contract.SealSnapshotArtifactAggregateEnvelopeV2(contract.SnapshotArtifactAggregateEnvelopeV2{
		AggregateRef:      contract.SnapshotArtifactAggregateRefV2{AggregateID: previousAggregate.AggregateID, Revision: 2, TenantID: previousAggregate.TenantID, DataDomain: previousAggregate.DataDomain, SchemaRef: previousAggregate.SchemaRef, ExpiresUnixNano: historyExpiry},
		RequestedNotAfter: request.RequestedNotAfter, PreviousAggregateRef: contract.SnapshotArtifactOptionalAggregateRefV2{Presence: contract.SnapshotArtifactPresent, Ref: &previousAggregate}, AppliedEntryRef: entry.ExactRef(), ReservationFactRef: reservationFact.ExactRef(),
		ArtifactFactRef: contract.SnapshotArtifactOptionalExactRefV2{Presence: contract.SnapshotArtifactPresent, Ref: &factRef}, RetentionApplicationFactRef: contract.SnapshotArtifactOptionalExactRefV2{Presence: contract.SnapshotArtifactAbsent}, ActiveDeletionAttemptFactRef: contract.SnapshotArtifactOptionalExactRefV2{Presence: contract.SnapshotArtifactAbsent}, LastClosedDeletionAttemptFactRef: contract.SnapshotArtifactOptionalExactRefV2{Presence: contract.SnapshotArtifactAbsent}, TerminalTombstoneRef: contract.SnapshotArtifactOptionalExactRefV2{Presence: contract.SnapshotArtifactAbsent}, AggregateState: contract.SnapshotArtifactAggregateAvailable,
	})
	if err != nil {
		return contract.SnapshotArtifactAvailableBundleV2{}, err
	}
	closure, err := contract.Digest("snapshot-artifact-available-active-ttl-closure-v2", struct {
		Artifact contract.SnapshotArtifactExactRefV2 `json:"artifact"`
		Subject  contract.SnapshotArtifactExactRefV2 `json:"subject"`
		Expires  int64                               `json:"expires"`
	}{Artifact: fact.ExactRef(), Subject: reservation.SubjectRef.ExactRef(), Expires: projectionExpiry})
	if err != nil {
		return contract.SnapshotArtifactAvailableBundleV2{}, err
	}
	index, err := contract.SealSnapshotArtifactAggregateCurrentIndexV2(contract.SnapshotArtifactAggregateCurrentIndexV2{
		CurrentIndexRef: contract.SnapshotArtifactExactRefV2{ID: current.CurrentIndexRef.ID, Revision: 2, ExpiresUnixNano: projectionExpiry}, ArtifactAggregateID: reservation.SubjectRef.ArtifactAggregateID, ArtifactSubjectRef: reservation.SubjectRef, HeadAggregateEnvelopeRef: envelope.AggregateRef, AggregateState: contract.SnapshotArtifactAggregateAvailable,
		ReservationCurrentRef: contract.SnapshotArtifactOptionalExactRefV2{Presence: contract.SnapshotArtifactAbsent}, ArtifactFactRef: contract.SnapshotArtifactOptionalExactRefV2{Presence: contract.SnapshotArtifactPresent, Ref: &factRef}, RetentionApplicationCurrentRef: contract.SnapshotArtifactOptionalExactRefV2{Presence: contract.SnapshotArtifactAbsent}, ActiveDeletionAttemptFactRef: contract.SnapshotArtifactOptionalExactRefV2{Presence: contract.SnapshotArtifactAbsent}, TerminalTombstoneRef: contract.SnapshotArtifactOptionalExactRefV2{Presence: contract.SnapshotArtifactAbsent}, ActiveTTLClosureDigest: closure, OwnerClockWatermark: post.UnixNano(), CheckedUnixNano: post.UnixNano(), RequestedNotAfter: request.RequestedNotAfter,
	})
	if err != nil {
		return contract.SnapshotArtifactAvailableBundleV2{}, err
	}
	bundle := contract.SnapshotArtifactAvailableBundleV2{ExpectedCurrentIndexRef: current.CurrentIndexRef, Fact: fact, Entry: entry, Envelope: envelope, CurrentIndex: index, OwnerClockWatermark: post.UnixNano()}
	return bundle, bundle.ValidateShape()
}

func (o *SnapshotArtifactOwner) recoverAvailableCommitByRequest(ctx context.Context, request contract.CommitSnapshotArtifactRequestV2) (contract.CommitSnapshotArtifactResultV2, error) {
	current, err := o.store.InspectSnapshotArtifactCurrentIndex(ctx, request.ExpectedAggregateRef.AggregateID)
	if err != nil {
		return contract.CommitSnapshotArtifactResultV2{}, err
	}
	if current.AggregateState != contract.SnapshotArtifactAggregateAvailable || current.ArtifactFactRef.Ref == nil {
		return contract.CommitSnapshotArtifactResultV2{}, fmt.Errorf("%w: snapshot artifact is not available", ports.ErrConflict)
	}
	envelope, err := o.store.InspectSnapshotArtifactEnvelope(ctx, current.HeadAggregateEnvelopeRef)
	if err != nil || envelope.PreviousAggregateRef.Ref == nil || !contract.SameSnapshotArtifactAggregateRef(*envelope.PreviousAggregateRef.Ref, request.ExpectedAggregateRef) {
		return contract.CommitSnapshotArtifactResultV2{}, fmt.Errorf("%w: snapshot artifact available predecessor differs", ports.ErrConflict)
	}
	fact, err := o.store.InspectSnapshotArtifactFact(ctx, *current.ArtifactFactRef.Ref)
	if err != nil {
		return contract.CommitSnapshotArtifactResultV2{}, err
	}
	if err := fact.ValidateShape(); err != nil || !contract.SnapshotArtifactFactMatchesCommitRequestV2(fact, request) {
		return contract.CommitSnapshotArtifactResultV2{}, fmt.Errorf("%w: snapshot artifact available fact differs", ports.ErrConflict)
	}
	return contract.CommitSnapshotArtifactResultV2{Fact: fact, CurrentIndex: current, Created: false}, nil
}

func (o *SnapshotArtifactOwner) InspectArtifactFact(ctx context.Context, input *contract.InspectSnapshotArtifactFactRequestV2) (contract.SnapshotArtifactFactV2, error) {
	if input == nil {
		return contract.SnapshotArtifactFactV2{}, errors.New("snapshot artifact fact inspect request is required")
	}
	if err := input.ValidateShape(); err != nil {
		return contract.SnapshotArtifactFactV2{}, err
	}
	fact, err := o.store.InspectSnapshotArtifactFact(ctx, input.ExpectedRef)
	if err != nil {
		return contract.SnapshotArtifactFactV2{}, err
	}
	if err := fact.ValidateShape(); err != nil || !contract.SameSnapshotArtifactExactRef(fact.ExactRef(), input.ExpectedRef) {
		return contract.SnapshotArtifactFactV2{}, fmt.Errorf("%w: snapshot artifact fact exact ref drifted", ports.ErrConflict)
	}
	return cloneSnapshotOwnerValue(fact)
}
