// Package testkit contains deterministic in-memory test doubles only. Nothing
// in this package is a production persistence or isolation backend.
package testkit

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

type sourceCursor struct {
	epoch         uint64
	sequence      uint64
	payloadDigest string
}

type settlementBinding struct {
	opaqueRef contract.Ref
	resultRef contract.Ref
}

type MemoryStore struct {
	mu sync.RWMutex

	reservations        map[string]contract.DomainReservation
	attempts            map[string]contract.DomainAttemptFact
	leaseBindings       map[string]contract.RuntimeLeaseBindingFact
	requirements        map[string]contract.ExecutionRequirement
	policies            map[string]contract.PolicyProjection
	placements          map[string]contract.PlacementCandidate
	backends            map[string]contract.BackendDescriptor
	slots               map[string]contract.SlotCandidate
	observations        map[string]contract.Observation
	inspections         map[string]contract.InspectionFact
	results             map[string]contract.SandboxDomainResultFact
	resultByReservation map[string]string
	projections         map[string]contract.EnvironmentProjection
	sources             map[string]sourceCursor
	settlements         map[string]settlementBinding
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		reservations:        make(map[string]contract.DomainReservation),
		attempts:            make(map[string]contract.DomainAttemptFact),
		leaseBindings:       make(map[string]contract.RuntimeLeaseBindingFact),
		requirements:        make(map[string]contract.ExecutionRequirement),
		policies:            make(map[string]contract.PolicyProjection),
		placements:          make(map[string]contract.PlacementCandidate),
		backends:            make(map[string]contract.BackendDescriptor),
		slots:               make(map[string]contract.SlotCandidate),
		observations:        make(map[string]contract.Observation),
		inspections:         make(map[string]contract.InspectionFact),
		results:             make(map[string]contract.SandboxDomainResultFact),
		resultByReservation: make(map[string]string),
		projections:         make(map[string]contract.EnvironmentProjection),
		sources:             make(map[string]sourceCursor),
		settlements:         make(map[string]settlementBinding),
	}
}

func (s *MemoryStore) SeedExactCurrentFacts(attempt contract.DomainAttemptFact, lease contract.RuntimeLeaseBindingFact, requirement contract.ExecutionRequirement, policy contract.PolicyProjection, placement contract.PlacementCandidate, backend contract.BackendDescriptor, slot contract.SlotCandidate) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, exists := range []bool{s.attempts[attempt.Meta.ID].Meta.ID != "", s.leaseBindings[lease.Meta.ID].Meta.ID != "", s.requirements[requirement.Meta.ID].Meta.ID != "", s.policies[policy.Meta.ID].Meta.ID != "", s.placements[placement.Meta.ID].Meta.ID != "", s.backends[backend.Meta.ID].Meta.ID != "", s.slots[slot.Meta.ID].Meta.ID != ""} {
		if exists {
			return ports.ErrConflict
		}
	}
	s.attempts[attempt.Meta.ID] = clone(attempt)
	s.leaseBindings[lease.Meta.ID] = clone(lease)
	s.requirements[requirement.Meta.ID] = clone(requirement)
	s.policies[policy.Meta.ID] = clone(policy)
	s.placements[placement.Meta.ID] = clone(placement)
	s.backends[backend.Meta.ID] = clone(backend)
	s.slots[slot.Meta.ID] = clone(slot)
	return nil
}

func (s *MemoryStore) GetAttempt(_ context.Context, id string) (contract.DomainAttemptFact, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.attempts[id]
	if !ok {
		return contract.DomainAttemptFact{}, ports.ErrNotFound
	}
	return clone(value), nil
}

func (s *MemoryStore) SeedProjection(projection contract.EnvironmentProjection) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.projections[projection.Lease.LeaseID]; exists {
		return ports.ErrConflict
	}
	s.projections[projection.Lease.LeaseID] = clone(projection)
	return nil
}

func (s *MemoryStore) CreateReservation(_ context.Context, reservation contract.DomainReservation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.reservations[reservation.Meta.ID]; exists {
		return ports.ErrConflict
	}
	s.reservations[reservation.Meta.ID] = clone(reservation)
	return nil
}

func (s *MemoryStore) GetReservation(_ context.Context, id string) (contract.DomainReservation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.reservations[id]
	if !ok {
		return contract.DomainReservation{}, ports.ErrNotFound
	}
	return clone(value), nil
}

func (s *MemoryStore) InspectReservationByAttempt(_ context.Context, operationID, effectID, attemptID string) (contract.DomainReservation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result contract.DomainReservation
	for _, value := range s.reservations {
		if value.OperationID == operationID && value.EffectID == effectID && value.AttemptID == attemptID {
			if result.Meta.ID != "" {
				return contract.DomainReservation{}, ports.ErrConflict
			}
			result = value
		}
	}
	if result.Meta.ID == "" {
		return contract.DomainReservation{}, ports.ErrNotFound
	}
	return clone(result), nil
}

func (s *MemoryStore) GetRuntimeLeaseBinding(_ context.Context, id string) (contract.RuntimeLeaseBindingFact, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.leaseBindings[id]
	if !ok {
		return contract.RuntimeLeaseBindingFact{}, ports.ErrNotFound
	}
	return clone(value), nil
}

func (s *MemoryStore) GetRequirement(_ context.Context, id string) (contract.ExecutionRequirement, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.requirements[id]
	if !ok {
		return contract.ExecutionRequirement{}, ports.ErrNotFound
	}
	return clone(value), nil
}

func (s *MemoryStore) GetPolicy(_ context.Context, id string) (contract.PolicyProjection, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.policies[id]
	if !ok {
		return contract.PolicyProjection{}, ports.ErrNotFound
	}
	return clone(value), nil
}

func (s *MemoryStore) GetPlacement(_ context.Context, id string) (contract.PlacementCandidate, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.placements[id]
	if !ok {
		return contract.PlacementCandidate{}, ports.ErrNotFound
	}
	return clone(value), nil
}

func (s *MemoryStore) GetBackend(_ context.Context, id string) (contract.BackendDescriptor, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.backends[id]
	if !ok {
		return contract.BackendDescriptor{}, ports.ErrNotFound
	}
	return clone(value), nil
}

func (s *MemoryStore) GetSlot(_ context.Context, id string) (contract.SlotCandidate, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.slots[id]
	if !ok {
		return contract.SlotCandidate{}, ports.ErrNotFound
	}
	return clone(value), nil
}

func (s *MemoryStore) AppendObservation(_ context.Context, reservationID string, observation contract.Observation) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.reservations[reservationID]; !ok {
		return false, ports.ErrNotFound
	}
	if existing, ok := s.observations[observation.Meta.ID]; ok {
		if existing.Meta.Digest == observation.Meta.Digest && existing.PayloadDigest == observation.PayloadDigest {
			return false, nil
		}
		return false, ports.ErrConflict
	}
	key := observation.SourceRegistrationID
	if cursor, ok := s.sources[key]; ok {
		switch {
		case observation.SourceEpoch < cursor.epoch:
			return false, ports.ErrStale
		case observation.SourceEpoch == cursor.epoch && observation.SourceSequence < cursor.sequence:
			return false, ports.ErrStale
		case observation.SourceEpoch == cursor.epoch && observation.SourceSequence == cursor.sequence:
			if observation.PayloadDigest == cursor.payloadDigest {
				return false, nil
			}
			return false, ports.ErrSourceConflict
		}
	}
	s.observations[observation.Meta.ID] = clone(observation)
	s.sources[key] = sourceCursor{epoch: observation.SourceEpoch, sequence: observation.SourceSequence, payloadDigest: observation.PayloadDigest}
	return true, nil
}

func (s *MemoryStore) GetObservation(_ context.Context, id string) (contract.Observation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.observations[id]
	if !ok {
		return contract.Observation{}, ports.ErrNotFound
	}
	return clone(value), nil
}

func (s *MemoryStore) CreateInspection(_ context.Context, inspection contract.InspectionFact) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.inspections[inspection.Meta.ID]; exists {
		return ports.ErrConflict
	}
	s.inspections[inspection.Meta.ID] = clone(inspection)
	return nil
}

func (s *MemoryStore) GetInspection(_ context.Context, id string) (contract.InspectionFact, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.inspections[id]
	if !ok {
		return contract.InspectionFact{}, ports.ErrNotFound
	}
	return clone(value), nil
}

func (s *MemoryStore) CreateDomainResult(_ context.Context, result contract.SandboxDomainResultFact) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.results[result.Meta.ID]; exists {
		return ports.ErrConflict
	}
	if _, exists := s.resultByReservation[result.ReservationRef.ID]; exists {
		return ports.ErrConflict
	}
	s.results[result.Meta.ID] = clone(result)
	s.resultByReservation[result.ReservationRef.ID] = result.Meta.ID
	return nil
}

func (s *MemoryStore) GetDomainResult(_ context.Context, id string) (contract.SandboxDomainResultFact, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.results[id]
	if !ok {
		return contract.SandboxDomainResultFact{}, ports.ErrNotFound
	}
	return clone(value), nil
}

func (s *MemoryStore) GetSettlementBinding(_ context.Context, opaqueRef contract.Ref) (contract.Ref, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	binding, ok := s.settlements[opaqueRef.ID]
	if !ok {
		return contract.Ref{}, ports.ErrNotFound
	}
	if !contract.SameRef(binding.opaqueRef, opaqueRef) {
		return contract.Ref{}, ports.ErrConflict
	}
	return binding.resultRef, nil
}

func (s *MemoryStore) GetProjection(_ context.Context, leaseID string) (contract.EnvironmentProjection, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.projections[leaseID]
	if !ok {
		return contract.EnvironmentProjection{}, ports.ErrNotFound
	}
	return clone(value), nil
}

func (s *MemoryStore) CompareAndSwapProjection(_ context.Context, expectedRevision uint64, projection contract.EnvironmentProjection) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.projections[projection.Lease.LeaseID]
	if !ok {
		return ports.ErrNotFound
	}
	if current.Meta.Revision != expectedRevision || projection.Meta.Revision != expectedRevision+1 {
		return ports.ErrStale
	}
	if err := projection.LastDomainResultRef.ValidateShape("domain result ref applied by CAS"); err != nil {
		return ports.ErrConflict
	}
	if err := projection.LastSettlementRef.ValidateShape("opaque settlement ref applied by CAS"); err != nil {
		return ports.ErrConflict
	}
	if binding, exists := s.settlements[projection.LastSettlementRef.ID]; exists {
		if !contract.SameRef(binding.opaqueRef, projection.LastSettlementRef) || !contract.SameRef(binding.resultRef, projection.LastDomainResultRef) {
			return ports.ErrConflict
		}
	}
	s.projections[projection.Lease.LeaseID] = clone(projection)
	s.settlements[projection.LastSettlementRef.ID] = settlementBinding{
		opaqueRef: projection.LastSettlementRef,
		resultRef: projection.LastDomainResultRef,
	}
	return nil
}

func clone[T any](value T) T {
	payload, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	var result T
	if err := json.Unmarshal(payload, &result); err != nil {
		panic(err)
	}
	return result
}
