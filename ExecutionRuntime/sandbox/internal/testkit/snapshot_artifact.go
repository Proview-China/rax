package testkit

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

func SnapshotArtifactRequest(suffix string) contract.ReserveArtifactRequestV2 {
	return contract.ReserveArtifactRequestV2{
		TenantID:              "tenant-1",
		DataDomain:            "workspace-checkpoint",
		SourceOperationID:     "checkpoint-operation-" + suffix,
		SourceEffectID:        "checkpoint-effect-" + suffix,
		SourceAttemptRef:      Ref("checkpoint-attempt-" + suffix),
		SchemaRef:             Ref("snapshot-schema-v2"),
		ExpectedContentDigest: Ref("snapshot-content-" + suffix).Digest,
		RetentionPolicyRef:    Ref("retention-policy-v2"),
		EncryptionPolicyRef:   Ref("encryption-policy-v2"),
		ResidencyPolicyRef:    Ref("residency-policy-v2"),
		ExpectedAggregateRef:  contract.SnapshotArtifactOptionalAggregateRefV2{Presence: contract.SnapshotArtifactAbsent},
		RequestedNotAfter:     FixedNow.Add(2 * time.Hour).UnixNano(),
	}
}

// SnapshotArtifactMemoryStore is a deterministic, append-only test double. It
// is not a production persistence backend and has no Provider capability.
type SnapshotArtifactMemoryStore struct {
	mu sync.RWMutex

	reservations        map[string]contract.SnapshotArtifactReservationV2
	reservationByStable map[string]string
	reservationFacts    map[string]contract.SnapshotArtifactReservationFactV2
	artifactFacts       map[string]contract.SnapshotArtifactFactV2
	entries             map[string]contract.SnapshotArtifactAggregateEntryV2
	envelopes           map[string]contract.SnapshotArtifactAggregateEnvelopeV2
	currentIndexes      map[string]contract.SnapshotArtifactAggregateCurrentIndexV2
	currentByAggregate  map[string]string
	lastOwnerClock      int64
}

func NewSnapshotArtifactMemoryStore() *SnapshotArtifactMemoryStore {
	return &SnapshotArtifactMemoryStore{
		reservations:        make(map[string]contract.SnapshotArtifactReservationV2),
		reservationByStable: make(map[string]string),
		reservationFacts:    make(map[string]contract.SnapshotArtifactReservationFactV2),
		artifactFacts:       make(map[string]contract.SnapshotArtifactFactV2),
		entries:             make(map[string]contract.SnapshotArtifactAggregateEntryV2),
		envelopes:           make(map[string]contract.SnapshotArtifactAggregateEnvelopeV2),
		currentIndexes:      make(map[string]contract.SnapshotArtifactAggregateCurrentIndexV2),
		currentByAggregate:  make(map[string]string),
	}
}

func (s *SnapshotArtifactMemoryStore) CommitAvailableSnapshotArtifact(_ context.Context, bundle contract.SnapshotArtifactAvailableBundleV2) (bool, error) {
	if err := bundle.ValidateShape(); err != nil {
		return false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	currentKey, ok := s.currentByAggregate[bundle.CurrentIndex.ArtifactAggregateID]
	if !ok {
		return false, ports.ErrNotFound
	}
	current := s.currentIndexes[currentKey]
	if contract.SameSnapshotArtifactExactRef(current.CurrentIndexRef, bundle.CurrentIndex.CurrentIndexRef) {
		return false, nil
	}
	if !contract.SameSnapshotArtifactExactRef(current.CurrentIndexRef, bundle.ExpectedCurrentIndexRef) || current.AggregateState != contract.SnapshotArtifactAggregateReserved || bundle.CurrentIndex.CurrentIndexRef.Revision != current.CurrentIndexRef.Revision+1 || bundle.Envelope.AggregateRef.Revision != current.HeadAggregateEnvelopeRef.Revision+1 {
		return false, ports.ErrConflict
	}
	if bundle.OwnerClockWatermark < s.lastOwnerClock {
		return false, ports.ErrStale
	}
	factKey := snapshotArtifactExactKey(bundle.Fact.ExactRef())
	entryKey := snapshotArtifactExactKey(bundle.Entry.ExactRef())
	envelopeKey := snapshotArtifactExactKey(bundle.Envelope.AggregateRef.ExactRef())
	indexKey := snapshotArtifactExactKey(bundle.CurrentIndex.CurrentIndexRef)
	if _, exists := s.artifactFacts[factKey]; exists {
		return false, ports.ErrConflict
	}
	if _, exists := s.entries[entryKey]; exists {
		return false, ports.ErrConflict
	}
	if _, exists := s.envelopes[envelopeKey]; exists {
		return false, ports.ErrConflict
	}
	if _, exists := s.currentIndexes[indexKey]; exists {
		return false, ports.ErrConflict
	}
	s.artifactFacts[factKey] = clone(bundle.Fact)
	s.entries[entryKey] = clone(bundle.Entry)
	s.envelopes[envelopeKey] = clone(bundle.Envelope)
	s.currentIndexes[indexKey] = clone(bundle.CurrentIndex)
	s.currentByAggregate[bundle.CurrentIndex.ArtifactAggregateID] = indexKey
	s.lastOwnerClock = bundle.OwnerClockWatermark
	return true, nil
}

func (s *SnapshotArtifactMemoryStore) CreateReservedSnapshotArtifact(_ context.Context, bundle contract.SnapshotArtifactReservedBundleV2) (bool, error) {
	if err := bundle.ValidateShape(); err != nil {
		return false, err
	}
	stable, err := contract.SnapshotArtifactStableSourceKeyDigest(bundle.StableKey)
	if err != nil {
		return false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if existingKey, exists := s.reservationByStable[stable]; exists {
		existing := s.reservations[existingKey]
		if contract.SameSnapshotArtifactExactRef(existing.ExactRef(), bundle.Reservation.ExactRef()) {
			return false, nil
		}
		return false, ports.ErrConflict
	}
	if bundle.OwnerClockWatermark < s.lastOwnerClock {
		return false, ports.ErrStale
	}
	reservationKey := snapshotArtifactExactKey(bundle.Reservation.ExactRef())
	reservationFactKey := snapshotArtifactExactKey(bundle.ReservationFact.ExactRef())
	entryKey := snapshotArtifactExactKey(bundle.Entry.ExactRef())
	envelopeKey := snapshotArtifactExactKey(bundle.Envelope.AggregateRef.ExactRef())
	indexKey := snapshotArtifactExactKey(bundle.CurrentIndex.CurrentIndexRef)
	if _, exists := s.reservations[reservationKey]; exists {
		return false, ports.ErrConflict
	}
	if _, exists := s.reservationFacts[reservationFactKey]; exists {
		return false, ports.ErrConflict
	}
	if _, exists := s.entries[entryKey]; exists {
		return false, ports.ErrConflict
	}
	if _, exists := s.envelopes[envelopeKey]; exists {
		return false, ports.ErrConflict
	}
	if _, exists := s.currentIndexes[indexKey]; exists {
		return false, ports.ErrConflict
	}
	if _, exists := s.currentByAggregate[bundle.Envelope.AggregateRef.AggregateID]; exists {
		return false, ports.ErrConflict
	}
	s.reservations[reservationKey] = clone(bundle.Reservation)
	s.reservationByStable[stable] = reservationKey
	s.reservationFacts[reservationFactKey] = clone(bundle.ReservationFact)
	s.entries[entryKey] = clone(bundle.Entry)
	s.envelopes[envelopeKey] = clone(bundle.Envelope)
	s.currentIndexes[indexKey] = clone(bundle.CurrentIndex)
	s.currentByAggregate[bundle.Envelope.AggregateRef.AggregateID] = indexKey
	s.lastOwnerClock = bundle.OwnerClockWatermark
	return true, nil
}

func (s *SnapshotArtifactMemoryStore) InspectSnapshotArtifactReservation(_ context.Context, expected contract.SnapshotArtifactExactRefV2) (contract.SnapshotArtifactReservationV2, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.reservations[snapshotArtifactExactKey(expected)]
	if !ok {
		return contract.SnapshotArtifactReservationV2{}, ports.ErrNotFound
	}
	return clone(value), nil
}

func (s *SnapshotArtifactMemoryStore) InspectSnapshotArtifactReservationByStableKey(_ context.Context, stableKey contract.SnapshotArtifactStableSourceKeyV2) (contract.SnapshotArtifactReservationV2, error) {
	stable, err := contract.SnapshotArtifactStableSourceKeyDigest(stableKey)
	if err != nil {
		return contract.SnapshotArtifactReservationV2{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	key, ok := s.reservationByStable[stable]
	if !ok {
		return contract.SnapshotArtifactReservationV2{}, ports.ErrNotFound
	}
	return clone(s.reservations[key]), nil
}

func (s *SnapshotArtifactMemoryStore) InspectSnapshotArtifactReservationFact(_ context.Context, expected contract.SnapshotArtifactExactRefV2) (contract.SnapshotArtifactReservationFactV2, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.reservationFacts[snapshotArtifactExactKey(expected)]
	if !ok {
		return contract.SnapshotArtifactReservationFactV2{}, ports.ErrNotFound
	}
	return clone(value), nil
}

func (s *SnapshotArtifactMemoryStore) InspectSnapshotArtifactFact(_ context.Context, expected contract.SnapshotArtifactExactRefV2) (contract.SnapshotArtifactFactV2, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.artifactFacts[snapshotArtifactExactKey(expected)]
	if !ok {
		return contract.SnapshotArtifactFactV2{}, ports.ErrNotFound
	}
	return clone(value), nil
}

func (s *SnapshotArtifactMemoryStore) InspectSnapshotArtifactEntry(_ context.Context, expected contract.SnapshotArtifactExactRefV2) (contract.SnapshotArtifactAggregateEntryV2, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.entries[snapshotArtifactExactKey(expected)]
	if !ok {
		return contract.SnapshotArtifactAggregateEntryV2{}, ports.ErrNotFound
	}
	return clone(value), nil
}

func (s *SnapshotArtifactMemoryStore) InspectSnapshotArtifactEnvelope(_ context.Context, expected contract.SnapshotArtifactAggregateRefV2) (contract.SnapshotArtifactAggregateEnvelopeV2, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.envelopes[snapshotArtifactExactKey(expected.ExactRef())]
	if !ok {
		return contract.SnapshotArtifactAggregateEnvelopeV2{}, ports.ErrNotFound
	}
	return clone(value), nil
}

func (s *SnapshotArtifactMemoryStore) InspectSnapshotArtifactCurrentIndex(_ context.Context, aggregateID string) (contract.SnapshotArtifactAggregateCurrentIndexV2, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	key, ok := s.currentByAggregate[aggregateID]
	if !ok {
		return contract.SnapshotArtifactAggregateCurrentIndexV2{}, ports.ErrNotFound
	}
	return clone(s.currentIndexes[key]), nil
}

// ReplaceSnapshotArtifactCurrent is a fault-injection seam. It appends the
// supplied exact revision and changes only the test current pointer.
func (s *SnapshotArtifactMemoryStore) ReplaceSnapshotArtifactCurrent(value contract.SnapshotArtifactAggregateCurrentIndexV2) error {
	if err := value.ValidateShape(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	currentKey, ok := s.currentByAggregate[value.ArtifactAggregateID]
	if !ok {
		return ports.ErrNotFound
	}
	current := s.currentIndexes[currentKey]
	if value.CurrentIndexRef.ID != current.CurrentIndexRef.ID || value.CurrentIndexRef.Revision <= current.CurrentIndexRef.Revision {
		return ports.ErrConflict
	}
	key := snapshotArtifactExactKey(value.CurrentIndexRef)
	if _, exists := s.currentIndexes[key]; exists {
		return ports.ErrConflict
	}
	s.currentIndexes[key] = clone(value)
	s.currentByAggregate[value.ArtifactAggregateID] = key
	return nil
}

func snapshotArtifactExactKey(ref contract.SnapshotArtifactExactRefV2) string {
	return fmt.Sprintf("%s\x00%s\x00%d\x00%s", ref.TypeURL, ref.ID, ref.Revision, ref.Digest)
}

var ErrInjectedSnapshotArtifactLostReply = errors.New("injected snapshot artifact reply loss")

type SnapshotArtifactLostReplyStore struct {
	base     *SnapshotArtifactMemoryStore
	injected atomic.Bool
}

func NewSnapshotArtifactLostReplyStore(base *SnapshotArtifactMemoryStore) *SnapshotArtifactLostReplyStore {
	return &SnapshotArtifactLostReplyStore{base: base}
}

func (s *SnapshotArtifactLostReplyStore) CreateReservedSnapshotArtifact(ctx context.Context, bundle contract.SnapshotArtifactReservedBundleV2) (bool, error) {
	created, err := s.base.CreateReservedSnapshotArtifact(ctx, bundle)
	if err == nil && created && s.injected.CompareAndSwap(false, true) {
		return false, ErrInjectedSnapshotArtifactLostReply
	}
	return created, err
}

func (s *SnapshotArtifactLostReplyStore) CommitAvailableSnapshotArtifact(ctx context.Context, bundle contract.SnapshotArtifactAvailableBundleV2) (bool, error) {
	return s.base.CommitAvailableSnapshotArtifact(ctx, bundle)
}

func (s *SnapshotArtifactLostReplyStore) InspectSnapshotArtifactReservation(ctx context.Context, expected contract.SnapshotArtifactExactRefV2) (contract.SnapshotArtifactReservationV2, error) {
	return s.base.InspectSnapshotArtifactReservation(ctx, expected)
}

func (s *SnapshotArtifactLostReplyStore) InspectSnapshotArtifactReservationByStableKey(ctx context.Context, key contract.SnapshotArtifactStableSourceKeyV2) (contract.SnapshotArtifactReservationV2, error) {
	return s.base.InspectSnapshotArtifactReservationByStableKey(ctx, key)
}

func (s *SnapshotArtifactLostReplyStore) InspectSnapshotArtifactReservationFact(ctx context.Context, expected contract.SnapshotArtifactExactRefV2) (contract.SnapshotArtifactReservationFactV2, error) {
	return s.base.InspectSnapshotArtifactReservationFact(ctx, expected)
}

func (s *SnapshotArtifactLostReplyStore) InspectSnapshotArtifactFact(ctx context.Context, expected contract.SnapshotArtifactExactRefV2) (contract.SnapshotArtifactFactV2, error) {
	return s.base.InspectSnapshotArtifactFact(ctx, expected)
}

func (s *SnapshotArtifactLostReplyStore) InspectSnapshotArtifactEntry(ctx context.Context, expected contract.SnapshotArtifactExactRefV2) (contract.SnapshotArtifactAggregateEntryV2, error) {
	return s.base.InspectSnapshotArtifactEntry(ctx, expected)
}

func (s *SnapshotArtifactLostReplyStore) InspectSnapshotArtifactEnvelope(ctx context.Context, expected contract.SnapshotArtifactAggregateRefV2) (contract.SnapshotArtifactAggregateEnvelopeV2, error) {
	return s.base.InspectSnapshotArtifactEnvelope(ctx, expected)
}

func (s *SnapshotArtifactLostReplyStore) InspectSnapshotArtifactCurrentIndex(ctx context.Context, aggregateID string) (contract.SnapshotArtifactAggregateCurrentIndexV2, error) {
	return s.base.InspectSnapshotArtifactCurrentIndex(ctx, aggregateID)
}
