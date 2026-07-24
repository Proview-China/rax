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

// CheckpointMemoryStore is a pure local test double. Phase Fact mutation is
// intentionally not part of ports.CheckpointPhaseStore; AppendAppliedPhase is
// only a fixture seam for facts that a future complete cross-owner Apply chain
// would already have committed.
type CheckpointMemoryStore struct {
	mu sync.RWMutex

	reservations       map[string]contract.CheckpointPhaseReservation
	reservationByID    map[string]string
	reservationByKey   map[string]string
	branchByKey        map[string]string
	participants       map[string]contract.CheckpointParticipantFact
	participantCurrent map[string]string
	facts              map[string]contract.CheckpointPhaseFact
	factCurrent        map[string]string
	factByReservation  map[string]string
	current            map[string]contract.CheckpointCurrentCoordinate
}

func NewCheckpointMemoryStore() *CheckpointMemoryStore {
	return &CheckpointMemoryStore{
		reservations:       make(map[string]contract.CheckpointPhaseReservation),
		reservationByID:    make(map[string]string),
		reservationByKey:   make(map[string]string),
		branchByKey:        make(map[string]string),
		participants:       make(map[string]contract.CheckpointParticipantFact),
		participantCurrent: make(map[string]string),
		facts:              make(map[string]contract.CheckpointPhaseFact),
		factCurrent:        make(map[string]string),
		factByReservation:  make(map[string]string),
		current:            make(map[string]contract.CheckpointCurrentCoordinate),
	}
}

func (s *CheckpointMemoryStore) SeedCheckpointParticipant(value contract.CheckpointParticipantFact) error {
	if err := value.ValidateShape(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.participantCurrent[value.Meta.ID]; exists {
		return ports.ErrConflict
	}
	key := checkpointRefKey(value.Meta.Ref())
	s.participants[key] = clone(value)
	s.participantCurrent[value.Meta.ID] = key
	return nil
}

func (s *CheckpointMemoryStore) ReserveCheckpointPhase(_ context.Context, expectedParticipant contract.Ref, reservation contract.CheckpointPhaseReservation, nextParticipant contract.CheckpointParticipantFact) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	reservationKey := checkpointRefKey(reservation.Meta.Ref())
	if existing, ok := s.reservations[reservationKey]; ok {
		if contract.SameRef(existing.Meta.Ref(), reservation.Meta.Ref()) {
			return false, nil
		}
		return false, ports.ErrConflict
	}
	if existingKey, exists := s.reservationByID[reservation.Meta.ID]; exists {
		if existing := s.reservations[existingKey]; !contract.SameRef(existing.Meta.Ref(), reservation.Meta.Ref()) {
			return false, ports.ErrConflict
		}
	}
	currentKey, ok := s.participantCurrent[expectedParticipant.ID]
	if !ok {
		return false, ports.ErrNotFound
	}
	current := s.participants[currentKey]
	if !contract.SameRef(current.Meta.Ref(), expectedParticipant) || nextParticipant.Meta.ID != current.Meta.ID || nextParticipant.Meta.Revision != current.Meta.Revision+1 {
		return false, ports.ErrConflict
	}
	if nextParticipant.ActiveReservation.Ref == nil || !contract.SameRef(*nextParticipant.ActiveReservation.Ref, reservation.Meta.Ref()) || nextParticipant.ActivePhase != reservation.Phase {
		return false, ports.ErrConflict
	}
	phaseKey, err := contract.CheckpointPhaseKey(reservation)
	if err != nil {
		return false, err
	}
	if existing, exists := s.reservationByKey[phaseKey]; exists && existing != reservationKey {
		return false, ports.ErrConflict
	}
	if reservation.PreviousPresence == contract.CheckpointPresent {
		branchKey, keyErr := contract.CheckpointBranchKey(reservation)
		if keyErr != nil {
			return false, keyErr
		}
		if existing, exists := s.branchByKey[branchKey]; exists && existing != reservationKey {
			return false, ports.ErrConflict
		}
		s.branchByKey[branchKey] = reservationKey
	}
	participantKey := checkpointRefKey(nextParticipant.Meta.Ref())
	if _, exists := s.participants[participantKey]; exists {
		return false, ports.ErrConflict
	}
	s.reservations[reservationKey] = clone(reservation)
	s.reservationByID[reservation.Meta.ID] = reservationKey
	s.reservationByKey[phaseKey] = reservationKey
	s.participants[participantKey] = clone(nextParticipant)
	s.participantCurrent[nextParticipant.Meta.ID] = participantKey
	return true, nil
}

func (s *CheckpointMemoryStore) InspectCheckpointPhaseReservation(_ context.Context, expected contract.Ref) (contract.CheckpointPhaseReservation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.reservations[checkpointRefKey(expected)]
	if !ok {
		return contract.CheckpointPhaseReservation{}, ports.ErrNotFound
	}
	return clone(value), nil
}

func (s *CheckpointMemoryStore) InspectCheckpointParticipant(_ context.Context, expected contract.Ref) (contract.CheckpointParticipantFact, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.participants[checkpointRefKey(expected)]
	if !ok {
		return contract.CheckpointParticipantFact{}, ports.ErrNotFound
	}
	return clone(value), nil
}

func (s *CheckpointMemoryStore) InspectCheckpointParticipantCurrent(_ context.Context, id string) (contract.CheckpointParticipantFact, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	key, ok := s.participantCurrent[id]
	if !ok {
		return contract.CheckpointParticipantFact{}, ports.ErrNotFound
	}
	return clone(s.participants[key]), nil
}

func (s *CheckpointMemoryStore) InspectCheckpointPhaseFact(_ context.Context, expected contract.Ref) (contract.CheckpointPhaseFact, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.facts[checkpointRefKey(expected)]
	if !ok {
		return contract.CheckpointPhaseFact{}, ports.ErrNotFound
	}
	return clone(value), nil
}

func (s *CheckpointMemoryStore) InspectCheckpointPhaseFactCurrent(_ context.Context, id string) (contract.CheckpointPhaseFact, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	key, ok := s.factCurrent[id]
	if !ok {
		return contract.CheckpointPhaseFact{}, ports.ErrNotFound
	}
	return clone(s.facts[key]), nil
}

func (s *CheckpointMemoryStore) InspectCheckpointPhaseFactByReservation(_ context.Context, reservation contract.Ref) (contract.CheckpointPhaseFact, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	key, ok := s.factByReservation[checkpointRefKey(reservation)]
	if !ok {
		return contract.CheckpointPhaseFact{}, ports.ErrNotFound
	}
	return clone(s.facts[key]), nil
}

func (s *CheckpointMemoryStore) AppendAppliedCheckpointPhase(expectedFact *contract.Ref, expectedParticipant contract.Ref, fact contract.CheckpointPhaseFact, nextParticipant contract.CheckpointParticipantFact, readerExpiresUnixNano int64) error {
	if err := fact.ValidateShape(); err != nil {
		return err
	}
	if err := nextParticipant.ValidateShape(); err != nil {
		return err
	}
	if fact.Meta.ExpiresUnixNano > readerExpiresUnixNano || nextParticipant.Meta.ExpiresUnixNano > readerExpiresUnixNano {
		return ports.ErrStale
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	participantKey, ok := s.participantCurrent[expectedParticipant.ID]
	if !ok || !contract.SameRef(s.participants[participantKey].Meta.Ref(), expectedParticipant) {
		return ports.ErrConflict
	}
	currentParticipant := s.participants[participantKey]
	reservation, reservationExists := s.reservations[checkpointRefKey(fact.ReservationRef)]
	if !reservationExists || currentParticipant.ActiveReservation.Ref == nil || !contract.SameRef(*currentParticipant.ActiveReservation.Ref, fact.ReservationRef) ||
		reservation.TenantID != fact.TenantID ||
		reservation.ParticipantRef.ID != fact.ParticipantRef.ID || !contract.SameRef(reservation.Base.CheckpointAttempt, fact.CheckpointAttemptRef) ||
		reservation.Phase != fact.Phase || reservation.PreviousPresence != fact.PreviousPresence || reservation.OperationID != fact.OperationID ||
		reservation.EffectID != fact.EffectID || reservation.AttemptID != fact.AttemptID {
		return ports.ErrConflict
	}
	if reservation.PreviousPhase != nil && (fact.PreviousPhase == nil || !contract.SameCheckpointPhaseClosure(*reservation.PreviousPhase, *fact.PreviousPhase)) {
		return ports.ErrConflict
	}
	if nextParticipant.Meta.ID != expectedParticipant.ID || nextParticipant.Meta.Revision != expectedParticipant.Revision+1 || nextParticipant.Closure == nil || !contract.SameCheckpointPhaseClosure(*nextParticipant.Closure, fact.ClosureRef()) {
		return ports.ErrConflict
	}
	if nextParticipant.ActiveReservation.Ref == nil || !contract.SameRef(*nextParticipant.ActiveReservation.Ref, fact.ReservationRef) || nextParticipant.State != fact.ParticipantState() {
		return ports.ErrConflict
	}
	currentFactKey, factExists := s.factCurrent[fact.Meta.ID]
	if expectedFact == nil {
		if factExists || fact.Meta.Revision != 1 || !contract.SameRef(fact.ParticipantRef, expectedParticipant) {
			return ports.ErrConflict
		}
	} else {
		if !factExists || !contract.SameRef(s.facts[currentFactKey].Meta.Ref(), *expectedFact) || fact.Meta.ID != expectedFact.ID || fact.Meta.Revision != expectedFact.Revision+1 {
			return ports.ErrConflict
		}
		if err := contract.ValidateCheckpointUnknownReconcile(s.facts[currentFactKey], fact); err != nil {
			return ports.ErrConflict
		}
	}
	if expectedFact == nil && fact.State == contract.CheckpointPhaseIndeterminate {
		return ports.ErrConflict
	}
	factKey := checkpointRefKey(fact.Meta.Ref())
	if _, exists := s.facts[factKey]; exists {
		return ports.ErrConflict
	}
	reservationKey := checkpointRefKey(fact.ReservationRef)
	if existing, exists := s.factByReservation[reservationKey]; exists && expectedFact == nil && existing != factKey {
		return ports.ErrConflict
	}
	nextParticipantKey := checkpointRefKey(nextParticipant.Meta.Ref())
	if _, exists := s.participants[nextParticipantKey]; exists {
		return ports.ErrConflict
	}
	s.facts[factKey] = clone(fact)
	s.factCurrent[fact.Meta.ID] = factKey
	s.factByReservation[reservationKey] = factKey
	s.participants[nextParticipantKey] = clone(nextParticipant)
	s.participantCurrent[nextParticipant.Meta.ID] = nextParticipantKey
	return nil
}

func (s *CheckpointMemoryStore) SeedCheckpointCurrent(values ...contract.CheckpointCurrentCoordinate) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, value := range values {
		key := checkpointCurrentQueryKey(queryFromCoordinate(value))
		if _, exists := s.current[key]; exists {
			return ports.ErrConflict
		}
		s.current[key] = clone(value)
	}
	return nil
}

func (s *CheckpointMemoryStore) ReplaceCheckpointCurrent(value contract.CheckpointCurrentCoordinate) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.current[checkpointCurrentQueryKey(queryFromCoordinate(value))] = clone(value)
}

func (s *CheckpointMemoryStore) InspectCheckpointCurrent(_ context.Context, query contract.CheckpointCurrentQuery) (contract.CheckpointCurrentCoordinate, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.current[checkpointCurrentQueryKey(query)]
	if !ok {
		return contract.CheckpointCurrentCoordinate{}, ports.ErrNotFound
	}
	return clone(value), nil
}

func checkpointCurrentQueryKey(query contract.CheckpointCurrentQuery) string {
	return fmt.Sprintf("%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00%d\x00%s", query.Kind, query.TenantID, query.ParticipantID, query.CheckpointAttemptRef.ID, query.Phase, query.OperationID, query.EffectID, query.AttemptID, query.ExpectedRuntimeAttemptRef.ID, query.ExpectedRuntimeAttemptRef.Revision, query.ExpectedRuntimeAttemptRef.Digest)
}

func queryFromCoordinate(value contract.CheckpointCurrentCoordinate) contract.CheckpointCurrentQuery {
	return contract.CheckpointCurrentQuery{Kind: value.Kind, TenantID: value.TenantID, ParticipantID: value.ParticipantID, CheckpointAttemptRef: value.CheckpointAttemptRef, Phase: value.Phase, OperationID: value.OperationID, EffectID: value.EffectID, AttemptID: value.AttemptID, ExpectedRuntimeAttemptRef: value.ExpectedRuntimeAttemptRef}
}

func checkpointRefKey(ref contract.Ref) string {
	return fmt.Sprintf("%s\x00%d\x00%s", ref.ID, ref.Revision, ref.Digest)
}

var ErrInjectedCheckpointLostReply = errors.New("injected checkpoint reply loss")

type CheckpointLostReplyPoint string

const CheckpointLoseReserveReply CheckpointLostReplyPoint = "reserve"

type CheckpointLostReplyStore struct {
	ports.CheckpointPhaseStore
	point    CheckpointLostReplyPoint
	injected atomic.Bool
}

func NewCheckpointLostReplyStore(store ports.CheckpointPhaseStore, point CheckpointLostReplyPoint) *CheckpointLostReplyStore {
	return &CheckpointLostReplyStore{CheckpointPhaseStore: store, point: point}
}

func (s *CheckpointLostReplyStore) ReserveCheckpointPhase(ctx context.Context, expected contract.Ref, reservation contract.CheckpointPhaseReservation, next contract.CheckpointParticipantFact) (bool, error) {
	created, err := s.CheckpointPhaseStore.ReserveCheckpointPhase(ctx, expected, reservation, next)
	if err == nil && created && s.point == CheckpointLoseReserveReply && s.injected.CompareAndSwap(false, true) {
		return false, ErrInjectedCheckpointLostReply
	}
	return created, err
}

type LocalCheckpointConformance struct {
	mu      sync.RWMutex
	reports map[string]contract.CheckpointConformanceReport
}

func NewLocalCheckpointConformance(reports ...contract.CheckpointConformanceReport) *LocalCheckpointConformance {
	values := make(map[string]contract.CheckpointConformanceReport, len(reports))
	for _, report := range reports {
		values[report.ReservationRef.ID] = clone(report)
	}
	return &LocalCheckpointConformance{reports: values}
}

func (c *LocalCheckpointConformance) AssessCheckpointParticipant(_ context.Context, request ports.CheckpointConformanceRequest) (contract.CheckpointConformanceReport, error) {
	if err := request.Validate(); err != nil {
		return contract.CheckpointConformanceReport{}, err
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	report, ok := c.reports[request.ReservationRef.ID]
	if !ok {
		return contract.CheckpointConformanceReport{}, ports.ErrNotFound
	}
	if !contract.SameRef(report.ReservationRef, request.ReservationRef) {
		return contract.CheckpointConformanceReport{}, ports.ErrConflict
	}
	if report.ProductionProof || report.ProviderCalls != 0 {
		return contract.CheckpointConformanceReport{}, ports.ErrUnsupported
	}
	return clone(report), nil
}

func CheckpointParticipant(suffix string) contract.CheckpointParticipantFact {
	fact := contract.CheckpointParticipantFact{
		Meta:                 Meta("checkpoint-participant-"+suffix, 1),
		TenantID:             "tenant-1",
		CheckpointAttemptRef: Ref("runtime-checkpoint-attempt-" + suffix),
		State:                contract.CheckpointParticipantUnprepared,
		ActiveReservation:    contract.CheckpointOptionalRef{Presence: contract.CheckpointAbsent},
	}
	sealed, err := contract.SealCheckpointParticipantFact(fact)
	if err != nil {
		panic(err)
	}
	return sealed
}

func CheckpointReservation(phase contract.CheckpointPhase, suffix string, participant contract.CheckpointParticipantFact, previous *contract.CheckpointPhaseClosureRef) contract.CheckpointPhaseReservation {
	runtimeAttemptID := "checkpoint-runtime-attempt-" + suffix
	changeSet := Ref("checkpoint-change-set-" + suffix)
	reservation := contract.CheckpointPhaseReservation{
		Meta:                        Meta("checkpoint-reservation-"+suffix, 1),
		TenantID:                    participant.TenantID,
		ParticipantRef:              participant.Meta.Ref(),
		ExpectedParticipantRevision: participant.Meta.Revision,
		Phase:                       phase,
		PreviousPresence:            contract.CheckpointAbsent,
		PreviousPhase:               previous,
		OperationID:                 "checkpoint-operation-" + suffix,
		EffectID:                    "checkpoint-effect-" + suffix,
		AttemptID:                   participant.CheckpointAttemptRef.ID,
		ExpectedRuntimeAttemptRef:   Ref(runtimeAttemptID),
		Runtime: contract.CheckpointRuntimeBinding{
			InstanceID: "instance-1", InstanceEpoch: 7, LeaseID: "lease-1", LeaseEpoch: 11, FenceEpoch: 3,
		},
		ChangeSet:  contract.CheckpointOptionalRef{Presence: contract.CheckpointPresent, Ref: &changeSet},
		Watermarks: []contract.CheckpointWatermark{{SourceID: "provider-1", SourceEpoch: 1, Sequence: 1}},
		Base: contract.CheckpointBaseCurrentRefs{
			CheckpointAttempt:   participant.CheckpointAttemptRef,
			Barrier:             Ref("runtime-checkpoint-barrier-" + participant.Meta.ID),
			EffectCut:           Ref("runtime-checkpoint-effect-cut-" + participant.Meta.ID),
			RuntimeLeaseBinding: Ref("runtime-checkpoint-lease-" + participant.Meta.ID),
			Requirement:         Ref("checkpoint-requirement-" + participant.Meta.ID),
			Policy:              Ref("checkpoint-policy-" + participant.Meta.ID),
			Workspace:           Ref("checkpoint-workspace-" + participant.Meta.ID),
			Placement:           Ref("checkpoint-placement-" + participant.Meta.ID),
			Backend:             Ref("checkpoint-backend-" + participant.Meta.ID),
			Slot:                Ref("checkpoint-slot-" + participant.Meta.ID),
			Generation:          Ref("checkpoint-generation-" + participant.Meta.ID),
		},
	}
	if previous != nil {
		reservation.PreviousPresence = contract.CheckpointPresent
		if reservation.Meta.ExpiresUnixNano > previous.ExpiresUnixNano {
			reservation.Meta.ExpiresUnixNano = previous.ExpiresUnixNano
		}
	}
	if reservation.Meta.ExpiresUnixNano > participant.Meta.ExpiresUnixNano {
		reservation.Meta.ExpiresUnixNano = participant.Meta.ExpiresUnixNano
	}
	sealed, err := contract.SealCheckpointPhaseReservation(reservation)
	if err != nil {
		panic(err)
	}
	return sealed
}

func CheckpointAppliedPhase(reservation contract.CheckpointPhaseReservation, participant contract.CheckpointParticipantFact, state contract.CheckpointPhaseState, suffix string, readerExpires time.Time) (contract.CheckpointPhaseFact, contract.CheckpointParticipantFact) {
	expires := min(readerExpires.UnixNano(), reservation.Meta.ExpiresUnixNano, participant.Meta.ExpiresUnixNano)
	fact := contract.CheckpointPhaseFact{
		Meta:                 Meta("checkpoint-phase-fact-"+suffix, 1),
		ReservationRef:       reservation.Meta.Ref(),
		TenantID:             reservation.TenantID,
		ParticipantRef:       participant.Meta.Ref(),
		CheckpointAttemptRef: reservation.Base.CheckpointAttempt,
		Phase:                reservation.Phase,
		PreviousPresence:     reservation.PreviousPresence,
		PreviousPhase:        reservation.PreviousPhase,
		OperationID:          reservation.OperationID,
		EffectID:             reservation.EffectID,
		AttemptID:            reservation.AttemptID,
		State:                state,
		EvidenceRefs:         []contract.Ref{Ref("checkpoint-evidence-" + suffix)},
		DomainResultRef:      Ref("checkpoint-domain-result-" + suffix),
		RuntimeSettlementRef: Ref("checkpoint-runtime-settlement-v5-" + suffix),
		ApplySettlementRef:   Ref("checkpoint-apply-settlement-" + suffix),
	}
	fact.Meta.ExpiresUnixNano = expires
	sealedFact, err := contract.SealCheckpointPhaseFact(fact)
	if err != nil {
		panic(err)
	}
	next := participant
	next.Meta.Revision++
	next.Meta.UpdatedUnixNano = FixedNow.UnixNano()
	next.Meta.ExpiresUnixNano = expires
	next.State = sealedFact.ParticipantState()
	next.ActivePhase = reservation.Phase
	reservationRef := reservation.Meta.Ref()
	next.ActiveReservation = contract.CheckpointOptionalRef{Presence: contract.CheckpointPresent, Ref: &reservationRef}
	closure := sealedFact.ClosureRef()
	next.Closure = &closure
	sealedParticipant, err := contract.SealCheckpointParticipantFact(next)
	if err != nil {
		panic(err)
	}
	return sealedFact, sealedParticipant
}

func ReconciledCheckpointAppliedPhase(current contract.CheckpointPhaseFact, participant contract.CheckpointParticipantFact, suffix string, readerExpires time.Time) (contract.CheckpointPhaseFact, contract.CheckpointParticipantFact) {
	nextFact := current
	nextFact.Meta.Revision++
	nextFact.Meta.UpdatedUnixNano = FixedNow.Add(time.Second).UnixNano()
	nextFact.Meta.ExpiresUnixNano = min(nextFact.Meta.ExpiresUnixNano, readerExpires.UnixNano())
	nextFact.State = contract.CheckpointPhaseIndeterminate
	nextFact.EvidenceRefs = []contract.Ref{Ref("checkpoint-reconcile-evidence-" + suffix)}
	nextFact.DomainResultRef = Ref("checkpoint-reconcile-domain-result-" + suffix)
	nextFact.RuntimeSettlementRef = Ref("checkpoint-reconcile-runtime-settlement-v5-" + suffix)
	nextFact.ApplySettlementRef = Ref("checkpoint-reconcile-apply-settlement-" + suffix)
	sealedFact, err := contract.SealCheckpointPhaseFact(nextFact)
	if err != nil {
		panic(err)
	}
	nextParticipant := participant
	nextParticipant.Meta.Revision++
	nextParticipant.Meta.UpdatedUnixNano = FixedNow.Add(time.Second).UnixNano()
	nextParticipant.Meta.ExpiresUnixNano = min(nextParticipant.Meta.ExpiresUnixNano, readerExpires.UnixNano())
	nextParticipant.State = contract.CheckpointParticipantIndeterminate
	closure := sealedFact.ClosureRef()
	nextParticipant.Closure = &closure
	sealedParticipant, err := contract.SealCheckpointParticipantFact(nextParticipant)
	if err != nil {
		panic(err)
	}
	return sealedFact, sealedParticipant
}

func CheckpointCurrentFixture(reservation contract.CheckpointPhaseReservation, participant contract.CheckpointParticipantFact, stage contract.CheckpointReadStage) ([]contract.CheckpointCurrentCoordinate, contract.CheckpointCurrentReadRequest) {
	base := map[contract.CheckpointCurrentKind]contract.Ref{
		contract.CheckpointCurrentCheckpointAttempt: reservation.Base.CheckpointAttempt,
		contract.CheckpointCurrentBarrier:           reservation.Base.Barrier,
		contract.CheckpointCurrentEffectCut:         reservation.Base.EffectCut,
		contract.CheckpointCurrentRuntimeLease:      reservation.Base.RuntimeLeaseBinding,
		contract.CheckpointCurrentRequirement:       reservation.Base.Requirement,
		contract.CheckpointCurrentPolicy:            reservation.Base.Policy,
		contract.CheckpointCurrentWorkspace:         reservation.Base.Workspace,
		contract.CheckpointCurrentPlacement:         reservation.Base.Placement,
		contract.CheckpointCurrentBackend:           reservation.Base.Backend,
		contract.CheckpointCurrentSlot:              reservation.Base.Slot,
		contract.CheckpointCurrentGeneration:        reservation.Base.Generation,
		contract.CheckpointCurrentAttempt:           reservation.ExpectedRuntimeAttemptRef,
	}
	if reservation.ChangeSet.Ref != nil {
		base[contract.CheckpointCurrentChangeSet] = *reservation.ChangeSet.Ref
	}
	coordinates := make([]contract.CheckpointCurrentCoordinate, 0)
	expected := make([]contract.CheckpointExpectedCurrentRef, 0, len(contract.AllCheckpointCurrentKinds()))
	for _, kind := range contract.AllCheckpointCurrentKinds() {
		presence := contract.CheckpointExpectedPresenceFor(stage, kind, reservation.ChangeSet.Presence)
		var expectedRef *contract.Ref
		if presence == contract.CheckpointPresent {
			ref, ok := base[kind]
			if !ok {
				ref = Ref("checkpoint-current-" + string(kind) + "-" + reservation.Meta.ID)
			}
			refCopy := ref
			expectedRef = &refCopy
			coordinates = append(coordinates, contract.CheckpointCurrentCoordinate{
				Meta:                      metaForExactRef(ref, FixedNow.Add(6*time.Hour)),
				State:                     contract.CurrentFactActive,
				Kind:                      kind,
				TenantID:                  reservation.TenantID,
				ParticipantID:             participant.Meta.ID,
				CheckpointAttemptRef:      reservation.Base.CheckpointAttempt,
				Phase:                     reservation.Phase,
				OperationID:               reservation.OperationID,
				EffectID:                  reservation.EffectID,
				AttemptID:                 reservation.AttemptID,
				ExpectedRuntimeAttemptRef: reservation.ExpectedRuntimeAttemptRef,
				Runtime:                   reservation.Runtime,
				ChangeSet:                 clone(reservation.ChangeSet),
				Watermarks:                clone(reservation.Watermarks),
			})
		}
		expected = append(expected, contract.CheckpointExpectedCurrentRef{Kind: kind, Presence: presence, Ref: expectedRef})
	}
	request := contract.CheckpointCurrentReadRequest{
		TenantID:               reservation.TenantID,
		ParticipantRef:         participant.Meta.Ref(),
		CheckpointAttemptRef:   reservation.Base.CheckpointAttempt,
		Phase:                  reservation.Phase,
		PreviousPresence:       reservation.PreviousPresence,
		Stage:                  stage,
		ExpectedReservationRef: reservation.Meta.Ref(),
		ExpectedPreviousPhase:  clone(reservation.PreviousPhase),
		OperationID:            reservation.OperationID,
		EffectID:               reservation.EffectID,
		AttemptID:              reservation.AttemptID,
		ExpectedRuntimeAttempt: reservation.ExpectedRuntimeAttemptRef,
		Runtime:                reservation.Runtime,
		ChangeSet:              clone(reservation.ChangeSet),
		Watermarks:             clone(reservation.Watermarks),
		ExpectedCurrentRefs:    expected,
	}
	return coordinates, request
}

func CheckpointConformance(reservation contract.CheckpointPhaseReservation) contract.CheckpointConformanceReport {
	report := contract.CheckpointConformanceReport{
		Meta:           Meta("checkpoint-conformance-"+reservation.Meta.ID, 1),
		ReservationRef: reservation.Meta.Ref(),
		Capabilities:   contract.RequiredCheckpointConformanceCapabilities(),
		EvidenceRefs:   []contract.Ref{Ref("checkpoint-conformance-evidence")},
	}
	sealed, err := contract.SealCheckpointConformanceReport(report)
	if err != nil {
		panic(err)
	}
	return sealed
}

func metaForExactRef(ref contract.Ref, expires time.Time) contract.Meta {
	return contract.Meta{
		ContractVersion: contract.ContractFamily,
		ID:              ref.ID,
		Revision:        ref.Revision,
		Digest:          ref.Digest,
		CreatedUnixNano: FixedNow.Add(-time.Second).UnixNano(),
		UpdatedUnixNano: FixedNow.UnixNano(),
		ExpiresUnixNano: expires.UnixNano(),
	}
}
