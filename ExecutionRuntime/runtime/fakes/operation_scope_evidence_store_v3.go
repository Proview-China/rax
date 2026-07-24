package fakes

import (
	"context"
	"encoding/json"
	"strconv"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// OperationScopeEvidenceStoreV3 is a deterministic reference Fact Owner. One
// mutex is its transaction boundary; it claims neither production durability
// nor an SLA.
type OperationScopeEvidenceStoreV3 struct {
	mu                     sync.Mutex
	clock                  func() time.Time
	policies               map[string]ports.OperationScopeEvidencePolicyFactV3
	applicabilityPolicies  map[string]ports.OperationScopeEvidenceApplicabilityPolicyFactV3
	sources                map[string]ports.OperationScopeEvidenceSourceRegistrationFactV3
	sourceEpochs           map[string]string
	qualifications         map[string]ports.OperationScopeEvidenceQualificationFactV3
	qualificationsBySource map[string]string
	events                 map[string]string
	handoffs               map[string]ports.OperationScopeEvidenceProviderHandoffFactV3
	consumptions           map[string]ports.OperationScopeEvidenceConsumptionFactV3
	records                map[core.Digest]map[uint64]ports.OperationScopeEvidenceRecordV3
	lastSequence           map[core.Digest]uint64
	lastDigest             map[core.Digest]core.Digest
	loseQualificationReply bool
	loseHandoffReply       bool
	loseConsumeReply       bool
	losePolicyCASReply     bool
	loseAppPolicyCASReply  bool
}

func NewOperationScopeEvidenceStoreV3(clock func() time.Time) *OperationScopeEvidenceStoreV3 {
	if clock == nil {
		clock = time.Now
	}
	return &OperationScopeEvidenceStoreV3{
		clock:                  clock,
		policies:               map[string]ports.OperationScopeEvidencePolicyFactV3{},
		applicabilityPolicies:  map[string]ports.OperationScopeEvidenceApplicabilityPolicyFactV3{},
		sources:                map[string]ports.OperationScopeEvidenceSourceRegistrationFactV3{},
		sourceEpochs:           map[string]string{},
		qualifications:         map[string]ports.OperationScopeEvidenceQualificationFactV3{},
		qualificationsBySource: map[string]string{}, events: map[string]string{},
		handoffs:     map[string]ports.OperationScopeEvidenceProviderHandoffFactV3{},
		consumptions: map[string]ports.OperationScopeEvidenceConsumptionFactV3{},
		records:      map[core.Digest]map[uint64]ports.OperationScopeEvidenceRecordV3{},
		lastSequence: map[core.Digest]uint64{}, lastDigest: map[core.Digest]core.Digest{},
	}
}

func (s *OperationScopeEvidenceStoreV3) LoseNextQualificationReply() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loseQualificationReply = true
}
func (s *OperationScopeEvidenceStoreV3) LoseNextHandoffReply() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loseHandoffReply = true
}
func (s *OperationScopeEvidenceStoreV3) LoseNextConsumeReply() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loseConsumeReply = true
}
func (s *OperationScopeEvidenceStoreV3) LoseNextPolicyCASReply() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.losePolicyCASReply = true
}
func (s *OperationScopeEvidenceStoreV3) LoseNextApplicabilityPolicyCASReply() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loseAppPolicyCASReply = true
}

func (s *OperationScopeEvidenceStoreV3) CreateOperationScopeEvidencePolicyV3(ctx context.Context, fact ports.OperationScopeEvidencePolicyFactV3) (ports.OperationScopeEvidencePolicyFactV3, error) {
	if err := contextError(ctx); err != nil {
		return ports.OperationScopeEvidencePolicyFactV3{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if current, ok := s.policies[fact.ID]; ok {
		if current.Digest == fact.Digest {
			return cloneOSE(current), nil
		}
		return ports.OperationScopeEvidencePolicyFactV3{}, conflict("policy id changed content")
	}
	if err := fact.Validate(); err != nil {
		return ports.OperationScopeEvidencePolicyFactV3{}, err
	}
	s.policies[fact.ID] = cloneOSE(fact)
	return cloneOSE(fact), nil
}
func (s *OperationScopeEvidenceStoreV3) InspectOperationScopeEvidencePolicyV3(ctx context.Context, id string) (ports.OperationScopeEvidencePolicyFactV3, error) {
	if err := contextError(ctx); err != nil {
		return ports.OperationScopeEvidencePolicyFactV3{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	f, ok := s.policies[id]
	if !ok {
		return ports.OperationScopeEvidencePolicyFactV3{}, missing("evidence policy not found")
	}
	return cloneOSE(f), nil
}
func (s *OperationScopeEvidenceStoreV3) CompareAndSwapOperationScopeEvidencePolicyV3(ctx context.Context, request ports.OperationScopeEvidencePolicyCASRequestV3) (ports.OperationScopeEvidencePolicyFactV3, error) {
	if err := contextError(ctx); err != nil {
		return ports.OperationScopeEvidencePolicyFactV3{}, err
	}
	if err := request.Next.Validate(); err != nil {
		return ports.OperationScopeEvidencePolicyFactV3{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.policies[request.Next.ID]
	if !ok {
		return ports.OperationScopeEvidencePolicyFactV3{}, missing("evidence policy not found")
	}
	if current.Revision != request.ExpectedRevision {
		if current.Digest == request.Next.Digest {
			return cloneOSE(current), nil
		}
		return ports.OperationScopeEvidencePolicyFactV3{}, conflict("evidence policy revision conflict")
	}
	if err := ports.ValidateOperationScopeEvidencePolicyTransitionV3(current, request.Next); err != nil {
		return ports.OperationScopeEvidencePolicyFactV3{}, err
	}
	s.policies[current.ID] = cloneOSE(request.Next)
	if s.losePolicyCASReply {
		s.losePolicyCASReply = false
		return ports.OperationScopeEvidencePolicyFactV3{}, unavailable("injected Evidence policy CAS reply loss")
	}
	return cloneOSE(request.Next), nil
}
func (s *OperationScopeEvidenceStoreV3) CreateOperationScopeEvidenceApplicabilityPolicyV3(ctx context.Context, fact ports.OperationScopeEvidenceApplicabilityPolicyFactV3) (ports.OperationScopeEvidenceApplicabilityPolicyFactV3, error) {
	if err := contextError(ctx); err != nil {
		return ports.OperationScopeEvidenceApplicabilityPolicyFactV3{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if current, ok := s.applicabilityPolicies[fact.ID]; ok {
		if current.Digest == fact.Digest {
			return cloneOSE(current), nil
		}
		return ports.OperationScopeEvidenceApplicabilityPolicyFactV3{}, conflict("applicability policy id changed content")
	}
	if err := fact.Validate(); err != nil {
		return ports.OperationScopeEvidenceApplicabilityPolicyFactV3{}, err
	}
	s.applicabilityPolicies[fact.ID] = cloneOSE(fact)
	return cloneOSE(fact), nil
}
func (s *OperationScopeEvidenceStoreV3) InspectOperationScopeEvidenceApplicabilityPolicyV3(ctx context.Context, id string) (ports.OperationScopeEvidenceApplicabilityPolicyFactV3, error) {
	if err := contextError(ctx); err != nil {
		return ports.OperationScopeEvidenceApplicabilityPolicyFactV3{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	f, ok := s.applicabilityPolicies[id]
	if !ok {
		return ports.OperationScopeEvidenceApplicabilityPolicyFactV3{}, missing("applicability policy not found")
	}
	return cloneOSE(f), nil
}
func (s *OperationScopeEvidenceStoreV3) CompareAndSwapOperationScopeEvidenceApplicabilityPolicyV3(ctx context.Context, request ports.OperationScopeEvidenceApplicabilityPolicyCASRequestV3) (ports.OperationScopeEvidenceApplicabilityPolicyFactV3, error) {
	if err := contextError(ctx); err != nil {
		return ports.OperationScopeEvidenceApplicabilityPolicyFactV3{}, err
	}
	if err := request.Next.Validate(); err != nil {
		return ports.OperationScopeEvidenceApplicabilityPolicyFactV3{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.applicabilityPolicies[request.Next.ID]
	if !ok {
		return ports.OperationScopeEvidenceApplicabilityPolicyFactV3{}, missing("applicability policy not found")
	}
	if current.Revision != request.ExpectedRevision {
		if current.Digest == request.Next.Digest {
			return cloneOSE(current), nil
		}
		return ports.OperationScopeEvidenceApplicabilityPolicyFactV3{}, conflict("applicability policy revision conflict")
	}
	if err := ports.ValidateOperationScopeEvidenceApplicabilityPolicyTransitionV3(current, request.Next); err != nil {
		return ports.OperationScopeEvidenceApplicabilityPolicyFactV3{}, err
	}
	s.applicabilityPolicies[current.ID] = cloneOSE(request.Next)
	if s.loseAppPolicyCASReply {
		s.loseAppPolicyCASReply = false
		return ports.OperationScopeEvidenceApplicabilityPolicyFactV3{}, unavailable("injected applicability policy CAS reply loss")
	}
	return cloneOSE(request.Next), nil
}
func (s *OperationScopeEvidenceStoreV3) CreateOperationScopeEvidenceSourceV3(ctx context.Context, fact ports.OperationScopeEvidenceSourceRegistrationFactV3) (ports.OperationScopeEvidenceSourceRegistrationFactV3, error) {
	if err := contextError(ctx); err != nil {
		return ports.OperationScopeEvidenceSourceRegistrationFactV3{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if current, ok := s.sources[fact.ID]; ok {
		if current.Digest == fact.Digest {
			return cloneOSE(current), nil
		}
		return ports.OperationScopeEvidenceSourceRegistrationFactV3{}, conflict("source id changed content")
	}
	if err := fact.Validate(); err != nil {
		return ports.OperationScopeEvidenceSourceRegistrationFactV3{}, err
	}
	policy, ok := s.policies[fact.Policy.ID]
	if !ok || policy.RefV3() != fact.Policy {
		return ports.OperationScopeEvidenceSourceRegistrationFactV3{}, stale("source policy is missing or mismatched")
	}
	ledgerDigest, _ := fact.LedgerScope.DigestV3()
	epochKey := string(ledgerDigest) + "\x00" + string(fact.SourceID) + "\x00" + strconv.FormatUint(uint64(fact.SourceEpoch), 10)
	if existing, ok := s.sourceEpochs[epochKey]; ok && existing != fact.ID {
		return ports.OperationScopeEvidenceSourceRegistrationFactV3{}, conflict("ledger scope, source and epoch already have a registration")
	}
	s.sources[fact.ID] = cloneOSE(fact)
	s.sourceEpochs[epochKey] = fact.ID
	return cloneOSE(fact), nil
}
func (s *OperationScopeEvidenceStoreV3) InspectOperationScopeEvidenceSourceV3(ctx context.Context, id string) (ports.OperationScopeEvidenceSourceRegistrationFactV3, error) {
	if err := contextError(ctx); err != nil {
		return ports.OperationScopeEvidenceSourceRegistrationFactV3{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	f, ok := s.sources[id]
	if !ok {
		return ports.OperationScopeEvidenceSourceRegistrationFactV3{}, missing("source not found")
	}
	return cloneOSE(f), nil
}

func (s *OperationScopeEvidenceStoreV3) CreateOperationScopeEvidenceQualificationV3(ctx context.Context, fact ports.OperationScopeEvidenceQualificationFactV3) (ports.OperationScopeEvidenceQualificationFactV3, error) {
	if err := contextError(ctx); err != nil {
		return ports.OperationScopeEvidenceQualificationFactV3{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if current, ok := s.qualifications[fact.ID]; ok {
		if current.Digest == fact.Digest {
			return cloneOSE(current), nil
		}
		return ports.OperationScopeEvidenceQualificationFactV3{}, conflict("qualification id changed content")
	}
	if err := fact.Validate(); err != nil {
		return ports.OperationScopeEvidenceQualificationFactV3{}, err
	}
	now := s.clock()
	if now.IsZero() || fact.CreatedUnixNano > now.UnixNano() || !now.Before(time.Unix(0, fact.ExpiresUnixNano)) {
		return ports.OperationScopeEvidenceQualificationFactV3{}, stale("new qualification is future-dated or expired")
	}
	source, ok := s.sources[fact.Reservation.Source.RegistrationID]
	if !ok {
		return ports.OperationScopeEvidenceQualificationFactV3{}, missing("reserved source not found")
	}
	if sourceRef(source) != fact.Reservation.Registration || source.SourceEpoch != fact.Reservation.Source.SourceEpoch || source.NextSequence != fact.Reservation.Source.SourceSequence {
		return ports.OperationScopeEvidenceQualificationFactV3{}, stale("source reservation is not current")
	}
	policy, ok := s.policies[fact.EvidencePolicy.ID]
	if !ok || policy.RefV3() != fact.EvidencePolicy {
		return ports.OperationScopeEvidenceQualificationFactV3{}, stale("evidence policy is not exact")
	}
	app, ok := s.applicabilityPolicies[fact.Scope.ApplicabilityPolicy.ID]
	if !ok || app.RefV3() != fact.Scope.ApplicabilityPolicy {
		return ports.OperationScopeEvidenceQualificationFactV3{}, stale("applicability policy is not exact")
	}
	key := sourceKey(fact.Reservation.Source)
	if prior, ok := s.qualificationsBySource[key]; ok && prior != fact.ID {
		return ports.OperationScopeEvidenceQualificationFactV3{}, conflict("source sequence is already reserved")
	}
	scopeDigest, _ := fact.Scope.LedgerScope.DigestV3()
	eventKey := string(scopeDigest) + "\x00" + fact.Reservation.EventID
	if prior, ok := s.events[eventKey]; ok && prior != fact.ID {
		return ports.OperationScopeEvidenceQualificationFactV3{}, conflict("event id is already reserved")
	}
	s.qualifications[fact.ID] = cloneOSE(fact)
	s.qualificationsBySource[key] = fact.ID
	s.events[eventKey] = fact.ID
	if s.loseQualificationReply {
		s.loseQualificationReply = false
		return ports.OperationScopeEvidenceQualificationFactV3{}, unavailable("injected qualification reply loss")
	}
	return cloneOSE(fact), nil
}
func (s *OperationScopeEvidenceStoreV3) InspectOperationScopeEvidenceQualificationV3(ctx context.Context, id string) (ports.OperationScopeEvidenceQualificationFactV3, error) {
	if err := contextError(ctx); err != nil {
		return ports.OperationScopeEvidenceQualificationFactV3{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	f, ok := s.qualifications[id]
	if !ok {
		return ports.OperationScopeEvidenceQualificationFactV3{}, missing("qualification not found")
	}
	return cloneOSE(f), nil
}
func (s *OperationScopeEvidenceStoreV3) InspectOperationScopeEvidenceQualificationBySourceV3(ctx context.Context, key ports.OperationScopeEvidenceSourceKeyV3) (ports.OperationScopeEvidenceQualificationFactV3, error) {
	if err := contextError(ctx); err != nil {
		return ports.OperationScopeEvidenceQualificationFactV3{}, err
	}
	if err := key.Validate(); err != nil {
		return ports.OperationScopeEvidenceQualificationFactV3{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.qualificationsBySource[sourceKey(key)]
	if !ok {
		return ports.OperationScopeEvidenceQualificationFactV3{}, missing("source reservation not found")
	}
	return cloneOSE(s.qualifications[id]), nil
}
func (s *OperationScopeEvidenceStoreV3) CreateOperationScopeEvidenceProviderHandoffV3(ctx context.Context, fact ports.OperationScopeEvidenceProviderHandoffFactV3) (ports.OperationScopeEvidenceProviderHandoffFactV3, error) {
	if err := contextError(ctx); err != nil {
		return ports.OperationScopeEvidenceProviderHandoffFactV3{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if current, ok := s.handoffs[fact.ID]; ok {
		if current.Digest == fact.Digest {
			return cloneOSE(current), nil
		}
		return ports.OperationScopeEvidenceProviderHandoffFactV3{}, conflict("handoff id changed content")
	}
	if err := fact.Validate(); err != nil {
		return ports.OperationScopeEvidenceProviderHandoffFactV3{}, err
	}
	now := s.clock()
	if now.IsZero() || fact.CheckedUnixNano > now.UnixNano() || !now.Before(time.Unix(0, fact.NotAfterUnixNano)) {
		return ports.OperationScopeEvidenceProviderHandoffFactV3{}, stale("new handoff is future-dated or expired")
	}
	qualification, ok := s.qualifications[fact.Qualification.ID]
	if !ok || qualification.RefV3() != fact.Qualification || qualification.State != ports.OperationScopeEvidenceIssuedV3 || fact.Phase != qualification.Runtime.Phase {
		return ports.OperationScopeEvidenceProviderHandoffFactV3{}, stale("handoff does not bind current qualification")
	}
	s.handoffs[fact.ID] = cloneOSE(fact)
	if s.loseHandoffReply {
		s.loseHandoffReply = false
		return ports.OperationScopeEvidenceProviderHandoffFactV3{}, unavailable("injected handoff reply loss")
	}
	return cloneOSE(fact), nil
}
func (s *OperationScopeEvidenceStoreV3) InspectOperationScopeEvidenceProviderHandoffV3(ctx context.Context, id string) (ports.OperationScopeEvidenceProviderHandoffFactV3, error) {
	if err := contextError(ctx); err != nil {
		return ports.OperationScopeEvidenceProviderHandoffFactV3{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	f, ok := s.handoffs[id]
	if !ok {
		return ports.OperationScopeEvidenceProviderHandoffFactV3{}, missing("handoff not found")
	}
	return cloneOSE(f), nil
}

func (s *OperationScopeEvidenceStoreV3) ConsumeOperationScopeEvidenceV3(ctx context.Context, request ports.OperationScopeEvidenceAtomicConsumeRequestV3) (ports.OperationScopeEvidenceConsumeResultV3, error) {
	if err := contextError(ctx); err != nil {
		return ports.OperationScopeEvidenceConsumeResultV3{}, err
	}
	if err := request.Validate(); err != nil {
		return ports.OperationScopeEvidenceConsumeResultV3{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if current, ok := s.consumptions[request.ConsumptionID]; ok {
		digest, _ := request.Candidate.DigestV3()
		if current.CandidateDigest != digest || current.Handoff != request.Handoff {
			return ports.OperationScopeEvidenceConsumeResultV3{}, conflict("consumption id changed content")
		}
		return s.consumeResult(current), nil
	}
	qualification, ok := s.qualifications[request.Candidate.Qualification.ID]
	if !ok {
		return ports.OperationScopeEvidenceConsumeResultV3{}, missing("qualification not found")
	}
	handoff, ok := s.handoffs[request.Handoff.ID]
	if !ok {
		return ports.OperationScopeEvidenceConsumeResultV3{}, missing("handoff not found")
	}
	source, ok := s.sources[request.Candidate.Source.RegistrationID]
	if !ok {
		return ports.OperationScopeEvidenceConsumeResultV3{}, missing("source not found")
	}
	if qualification.Revision != request.ExpectedQualificationRevision || qualification.RefV3() != request.Candidate.Qualification || qualification.State != ports.OperationScopeEvidenceIssuedV3 || handoff.RefV3() != request.Handoff || handoff.Qualification != qualification.RefV3() || handoff.Phase != qualification.Runtime.Phase || source.Revision != request.ExpectedSourceRevision || source.SourceEpoch != request.Candidate.Source.SourceEpoch || source.NextSequence != request.Candidate.Source.SourceSequence {
		return ports.OperationScopeEvidenceConsumeResultV3{}, stale("atomic consume watermarks drifted")
	}
	now := s.clock()
	late := !now.Before(time.Unix(0, qualification.ExpiresUnixNano))
	if now.IsZero() || request.ConsumedUnixNano > now.UnixNano() || request.Candidate.ObservedUnixNano > request.ConsumedUnixNano || request.LateObservation != late || !now.Before(time.Unix(0, qualification.IngestNotAfterUnixNano)) {
		return ports.OperationScopeEvidenceConsumeResultV3{}, stale("consume clock or exact TTL classification drifted")
	}
	if request.Candidate.Source != qualification.Reservation.Source || request.Candidate.EventID != qualification.Reservation.EventID || request.Candidate.Payload.Schema != qualification.Reservation.Schema {
		return ports.OperationScopeEvidenceConsumeResultV3{}, conflict("candidate does not bind qualification")
	}
	candidateDigest, _ := request.Candidate.DigestV3()
	ledgerDigest, _ := qualification.Scope.LedgerScope.DigestV3()
	sequence := s.lastSequence[ledgerDigest] + 1
	if sequence == 0 {
		return ports.OperationScopeEvidenceConsumeResultV3{}, conflict("ledger sequence overflow")
	}
	previous := s.lastDigest[ledgerDigest]
	if sequence == 1 {
		previous = ports.EvidenceGenesisDigestV2
	}
	record, err := ports.SealOperationScopeEvidenceRecordV3(ports.OperationScopeEvidenceRecordV3{Ref: ports.OperationScopeEvidenceRecordRefV3{LedgerScopeDigest: ledgerDigest, Sequence: sequence}, Candidate: request.Candidate, CandidateDigest: candidateDigest, PreviousRecordDigest: previous, IngestedUnixNano: request.ConsumedUnixNano, LateObservation: request.LateObservation})
	if err != nil {
		return ports.OperationScopeEvidenceConsumeResultV3{}, err
	}
	consumption, err := ports.SealOperationScopeEvidenceConsumptionFactV3(ports.OperationScopeEvidenceConsumptionFactV3{ID: request.ConsumptionID, Revision: 1, Qualification: qualification.RefV3(), Handoff: handoff.RefV3(), CandidateDigest: candidateDigest, Record: record.Ref, LateObservation: request.LateObservation, CreatedUnixNano: request.ConsumedUnixNano})
	if err != nil {
		return ports.OperationScopeEvidenceConsumeResultV3{}, err
	}
	qualification.Revision++
	qualification.UpdatedUnixNano = request.ConsumedUnixNano
	qualification.Consumption = ptrOSE(consumption.RefV3())
	if request.LateObservation {
		qualification.State = ports.OperationScopeEvidenceConsumedObservationV3
	} else {
		qualification.State = ports.OperationScopeEvidenceConsumedCurrentV3
	}
	qualification, err = ports.SealOperationScopeEvidenceQualificationFactV3(qualification)
	if err != nil {
		return ports.OperationScopeEvidenceConsumeResultV3{}, err
	}
	source.Revision++
	source.NextSequence++
	if source.NextSequence == 0 {
		return ports.OperationScopeEvidenceConsumeResultV3{}, conflict("source cursor overflow")
	}
	source.UpdatedUnixNano = request.ConsumedUnixNano
	source, err = ports.SealOperationScopeEvidenceSourceRegistrationFactV3(source)
	if err != nil {
		return ports.OperationScopeEvidenceConsumeResultV3{}, err
	}
	if s.records[ledgerDigest] == nil {
		s.records[ledgerDigest] = map[uint64]ports.OperationScopeEvidenceRecordV3{}
	}
	s.records[ledgerDigest][sequence] = cloneOSE(record)
	s.lastSequence[ledgerDigest] = sequence
	s.lastDigest[ledgerDigest] = record.Ref.RecordDigest
	s.consumptions[consumption.ID] = cloneOSE(consumption)
	s.qualifications[qualification.ID] = cloneOSE(qualification)
	s.sources[source.ID] = cloneOSE(source)
	result := ports.OperationScopeEvidenceConsumeResultV3{Qualification: qualification, Consumption: consumption, Record: record, Source: source}
	if s.loseConsumeReply {
		s.loseConsumeReply = false
		return ports.OperationScopeEvidenceConsumeResultV3{}, unavailable("injected atomic consume reply loss")
	}
	return cloneOSE(result), nil
}
func (s *OperationScopeEvidenceStoreV3) InspectOperationScopeEvidenceConsumptionV3(ctx context.Context, id string) (ports.OperationScopeEvidenceConsumptionFactV3, error) {
	if err := contextError(ctx); err != nil {
		return ports.OperationScopeEvidenceConsumptionFactV3{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	f, ok := s.consumptions[id]
	if !ok {
		return ports.OperationScopeEvidenceConsumptionFactV3{}, missing("consumption not found")
	}
	return cloneOSE(f), nil
}
func (s *OperationScopeEvidenceStoreV3) InspectOperationScopeEvidenceRecordV3(ctx context.Context, ref ports.OperationScopeEvidenceRecordRefV3) (ports.OperationScopeEvidenceRecordV3, error) {
	if err := contextError(ctx); err != nil {
		return ports.OperationScopeEvidenceRecordV3{}, err
	}
	if err := ref.Validate(); err != nil {
		return ports.OperationScopeEvidenceRecordV3{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	f, ok := s.records[ref.LedgerScopeDigest][ref.Sequence]
	if !ok {
		return ports.OperationScopeEvidenceRecordV3{}, missing("record not found")
	}
	if f.Ref != ref {
		return ports.OperationScopeEvidenceRecordV3{}, conflict("record ref changed")
	}
	return cloneOSE(f), nil
}

func (s *OperationScopeEvidenceStoreV3) consumeResult(c ports.OperationScopeEvidenceConsumptionFactV3) ports.OperationScopeEvidenceConsumeResultV3 {
	q := s.qualifications[c.Qualification.ID]
	source := s.sources[q.Reservation.Source.RegistrationID]
	record := s.records[c.Record.LedgerScopeDigest][c.Record.Sequence]
	return cloneOSE(ports.OperationScopeEvidenceConsumeResultV3{Qualification: q, Consumption: c, Record: record, Source: source})
}
func sourceRef(f ports.OperationScopeEvidenceSourceRegistrationFactV3) ports.OperationScopeEvidenceFactRefV3 {
	return ports.OperationScopeEvidenceFactRefV3{ID: f.ID, Revision: f.Revision, Digest: f.Digest, ExpiresUnixNano: f.ExpiresUnixNano}
}
func sourceKey(k ports.OperationScopeEvidenceSourceKeyV3) string {
	return k.RegistrationID + "\x00" + strconv.FormatUint(uint64(k.SourceEpoch), 10) + "\x00" + strconv.FormatUint(k.SourceSequence, 10)
}
func cloneOSE[T any](value T) T {
	raw, _ := json.Marshal(value)
	var copy T
	_ = json.Unmarshal(raw, &copy)
	return copy
}
func ptrOSE[T any](value T) *T { return &value }
func conflict(message string) error {
	return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, message)
}
func missing(message string) error {
	return core.NewError(core.ErrorNotFound, core.ReasonEvidenceUnavailable, message)
}
func stale(message string) error {
	return core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceUnavailable, message)
}
func unavailable(message string) error {
	return core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, message)
}
