package fakes

import (
	"context"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// CheckpointEvidenceStoreV1 is a fixture-only Evidence Owner. It proves
// create-once qualification/handoff/consumption and makes no production claim.
type CheckpointEvidenceStoreV1 struct {
	mu             sync.Mutex
	clock          func() time.Time
	qualifications map[string]ports.CheckpointRestoreEvidenceQualificationFactV1
	handoffs       map[string]ports.CheckpointRestoreEvidenceProviderHandoffRefV1
	consumptions   map[string]ports.CheckpointRestoreEvidenceConsumptionRefV1
	cursor         uint64
	loseNextReply  bool
}

func NewCheckpointEvidenceStoreV1(clock func() time.Time) *CheckpointEvidenceStoreV1 {
	if clock == nil {
		clock = time.Now
	}
	return &CheckpointEvidenceStoreV1{clock: clock, qualifications: map[string]ports.CheckpointRestoreEvidenceQualificationFactV1{}, handoffs: map[string]ports.CheckpointRestoreEvidenceProviderHandoffRefV1{}, consumptions: map[string]ports.CheckpointRestoreEvidenceConsumptionRefV1{}}
}

func (s *CheckpointEvidenceStoreV1) CursorV1() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cursor
}
func (s *CheckpointEvidenceStoreV1) QualificationCountV1() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.qualifications)
}
func (s *CheckpointEvidenceStoreV1) LoseNextReplyV1() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loseNextReply = true
}

func (s *CheckpointEvidenceStoreV1) CreateCheckpointPhaseQualificationFactV1(ctx context.Context, ownerRequest ports.CreateCheckpointPhaseQualificationOwnerRequestV1) (ports.CheckpointRestoreEvidenceQualificationRefV1, error) {
	if err := contextError(ctx); err != nil {
		return ports.CheckpointRestoreEvidenceQualificationRefV1{}, err
	}
	now := s.clock()
	if err := ownerRequest.Validate(now); err != nil {
		return ports.CheckpointRestoreEvidenceQualificationRefV1{}, err
	}
	request := ownerRequest.Request
	request.ExpiresUnixNano = ownerRequest.DerivedExpiresUnixNano
	scopeDigest, err := request.Scope.DigestV1()
	if err != nil {
		return ports.CheckpointRestoreEvidenceQualificationRefV1{}, err
	}
	ref := ports.CheckpointRestoreEvidenceQualificationRefV1{ID: request.ID, Revision: 1, Attempt: request.Attempt, Barrier: request.Barrier, EffectCut: request.EffectCut, Reservation: request.Reservation, Phase: request.Phase, ScopeDigest: scopeDigest, ExpiresUnixNano: request.ExpiresUnixNano}
	ref.Digest, _ = ref.DigestV1()
	fact := ports.CheckpointRestoreEvidenceQualificationFactV1{ContractVersion: ports.CheckpointRestoreEvidenceContractVersionV1, Ref: ref, Request: request, CreatedUnixNano: now.UnixNano()}
	if err := fact.Validate(); err != nil {
		return ports.CheckpointRestoreEvidenceQualificationRefV1{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.qualifications[ref.ID]; ok {
		if existing.Ref == ref {
			return cloneOSE(existing.Ref), nil
		}
		return ports.CheckpointRestoreEvidenceQualificationRefV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "checkpoint Evidence qualification ID binds different content")
	}
	s.qualifications[ref.ID] = cloneOSE(fact)
	if s.loseNextReply {
		s.loseNextReply = false
		return ports.CheckpointRestoreEvidenceQualificationRefV1{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected qualification reply loss")
	}
	return ref, nil
}

func (s *CheckpointEvidenceStoreV1) InspectCheckpointPhaseQualificationHistoricalV1(ctx context.Context, ref ports.CheckpointRestoreEvidenceQualificationRefV1) (ports.CheckpointRestoreEvidenceQualificationFactV1, error) {
	if err := contextError(ctx); err != nil {
		return ports.CheckpointRestoreEvidenceQualificationFactV1{}, err
	}
	if err := ref.Validate(); err != nil {
		return ports.CheckpointRestoreEvidenceQualificationFactV1{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	fact, ok := s.qualifications[ref.ID]
	if !ok {
		return ports.CheckpointRestoreEvidenceQualificationFactV1{}, core.NewError(core.ErrorNotFound, core.ReasonEvidenceUnavailable, "checkpoint Evidence qualification not found")
	}
	if fact.Ref != ref {
		return ports.CheckpointRestoreEvidenceQualificationFactV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "checkpoint Evidence qualification ref drifted")
	}
	return cloneOSE(fact), nil
}

func (s *CheckpointEvidenceStoreV1) InspectCheckpointPhaseQualificationCurrentV1(ctx context.Context, ref ports.CheckpointRestoreEvidenceQualificationRefV1) (ports.CheckpointRestoreEvidenceQualificationCurrentProjectionV1, error) {
	if _, err := s.InspectCheckpointPhaseQualificationHistoricalV1(ctx, ref); err != nil {
		return ports.CheckpointRestoreEvidenceQualificationCurrentProjectionV1{}, err
	}
	now := s.clock()
	s.mu.Lock()
	fact := s.qualifications[ref.ID]
	s.mu.Unlock()
	projection := ports.CheckpointRestoreEvidenceQualificationCurrentProjectionV1{Ref: ref, Scope: fact.Request.Scope, Current: now.Before(time.Unix(0, ref.ExpiresUnixNano)), CheckedUnixNano: now.UnixNano()}
	copy := projection
	copy.ProjectionDigest = ""
	projection.ProjectionDigest, _ = core.CanonicalJSONDigest("praxis.runtime.checkpoint-restore-evidence", ports.CheckpointRestoreEvidenceContractVersionV1, "CheckpointRestoreEvidenceQualificationCurrentProjectionV1", copy)
	return projection, projection.Validate(now)
}

func (s *CheckpointEvidenceStoreV1) CreateCheckpointPhaseProviderHandoffV1(ctx context.Context, request ports.CreateCheckpointPhaseProviderHandoffRequestV1) (ports.CheckpointRestoreEvidenceProviderHandoffRefV1, error) {
	if err := contextError(ctx); err != nil {
		return ports.CheckpointRestoreEvidenceProviderHandoffRefV1{}, err
	}
	if err := request.Validate(); err != nil {
		return ports.CheckpointRestoreEvidenceProviderHandoffRefV1{}, err
	}
	qualification, err := s.InspectCheckpointPhaseQualificationCurrentV1(ctx, request.Qualification)
	if err != nil {
		return ports.CheckpointRestoreEvidenceProviderHandoffRefV1{}, err
	}
	if qualification.Scope.DispatchAttempt != request.Attempt {
		return ports.CheckpointRestoreEvidenceProviderHandoffRefV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "checkpoint Evidence handoff changed the qualified Operation Attempt")
	}
	ref := ports.CheckpointRestoreEvidenceProviderHandoffRefV1{ID: request.ID, Revision: 1, Qualification: request.Qualification, Attempt: request.Attempt, Phase: request.Phase, ScopeDigest: request.ScopeDigest}
	ref.Digest, _ = ref.DigestV1()
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.handoffs[ref.ID]; ok {
		if existing == ref {
			return existing, nil
		}
		return ports.CheckpointRestoreEvidenceProviderHandoffRefV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "checkpoint Evidence handoff ID binds different content")
	}
	s.handoffs[ref.ID] = ref
	if s.loseNextReply {
		s.loseNextReply = false
		return ports.CheckpointRestoreEvidenceProviderHandoffRefV1{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected handoff reply loss")
	}
	return ref, nil
}

func (s *CheckpointEvidenceStoreV1) InspectCheckpointPhaseProviderHandoffCurrentV1(ctx context.Context, ref ports.CheckpointRestoreEvidenceProviderHandoffRefV1) (ports.CheckpointRestoreEvidenceHandoffCurrentProjectionV1, error) {
	if _, err := s.InspectCheckpointPhaseProviderHandoffHistoricalV1(ctx, ref); err != nil {
		return ports.CheckpointRestoreEvidenceHandoffCurrentProjectionV1{}, err
	}
	now := s.clock()
	projection := ports.CheckpointRestoreEvidenceHandoffCurrentProjectionV1{Ref: ref, Current: now.Before(time.Unix(0, ref.Qualification.ExpiresUnixNano)), CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: ref.Qualification.ExpiresUnixNano}
	copy := projection
	copy.ProjectionDigest = ""
	projection.ProjectionDigest, _ = core.CanonicalJSONDigest("praxis.runtime.checkpoint-restore-evidence", ports.CheckpointRestoreEvidenceContractVersionV1, "CheckpointRestoreEvidenceHandoffCurrentProjectionV1", copy)
	return projection, projection.Validate(now)
}

func (s *CheckpointEvidenceStoreV1) InspectCheckpointPhaseProviderHandoffHistoricalV1(ctx context.Context, ref ports.CheckpointRestoreEvidenceProviderHandoffRefV1) (ports.CheckpointRestoreEvidenceProviderHandoffRefV1, error) {
	if err := contextError(ctx); err != nil {
		return ports.CheckpointRestoreEvidenceProviderHandoffRefV1{}, err
	}
	if err := ref.Validate(); err != nil {
		return ports.CheckpointRestoreEvidenceProviderHandoffRefV1{}, err
	}
	s.mu.Lock()
	stored, ok := s.handoffs[ref.ID]
	s.mu.Unlock()
	if !ok {
		return ports.CheckpointRestoreEvidenceProviderHandoffRefV1{}, core.NewError(core.ErrorNotFound, core.ReasonEvidenceUnavailable, "checkpoint Evidence handoff not found")
	}
	if stored != ref {
		return ports.CheckpointRestoreEvidenceProviderHandoffRefV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "checkpoint Evidence handoff drifted")
	}
	return stored, nil
}

func (s *CheckpointEvidenceStoreV1) ConsumeCheckpointPhaseEvidenceCurrentV1(ctx context.Context, request ports.ConsumeCheckpointPhaseEvidenceRequestV1) (ports.CheckpointRestoreEvidenceConsumptionRefV1, error) {
	return s.consumeV1(ctx, request, ports.CheckpointEvidenceConsumedCurrentV1)
}
func (s *CheckpointEvidenceStoreV1) ConsumeCheckpointPhaseEvidenceObservationV1(ctx context.Context, request ports.ConsumeCheckpointPhaseEvidenceRequestV1) (ports.CheckpointRestoreEvidenceConsumptionRefV1, error) {
	return s.consumeV1(ctx, request, ports.CheckpointEvidenceConsumedObservationV1)
}

func (s *CheckpointEvidenceStoreV1) consumeV1(ctx context.Context, request ports.ConsumeCheckpointPhaseEvidenceRequestV1, state ports.CheckpointRestoreEvidenceConsumptionStateV1) (ports.CheckpointRestoreEvidenceConsumptionRefV1, error) {
	if err := contextError(ctx); err != nil {
		return ports.CheckpointRestoreEvidenceConsumptionRefV1{}, err
	}
	if err := request.Validate(); err != nil {
		return ports.CheckpointRestoreEvidenceConsumptionRefV1{}, err
	}
	if _, err := s.InspectCheckpointPhaseQualificationCurrentV1(ctx, request.Qualification); err != nil {
		return ports.CheckpointRestoreEvidenceConsumptionRefV1{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	handoff, ok := s.handoffs[request.Handoff.ID]
	if !ok || handoff != request.Handoff {
		return ports.CheckpointRestoreEvidenceConsumptionRefV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "checkpoint Evidence handoff is missing or drifted")
	}
	ref := ports.CheckpointRestoreEvidenceConsumptionRefV1{ID: request.ID, Revision: 1, Qualification: request.Qualification, Handoff: request.Handoff, Record: request.Record, Attempt: request.Qualification.Attempt, Phase: request.Qualification.Phase, State: state, ScopeDigest: request.Qualification.ScopeDigest, Source: request.Source}
	ref.Digest, _ = ref.DigestV1()
	if existing, ok := s.consumptions[ref.ID]; ok {
		if existing == ref {
			return existing, nil
		}
		return ports.CheckpointRestoreEvidenceConsumptionRefV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "checkpoint Evidence consumption ID binds different content")
	}
	s.consumptions[ref.ID] = ref
	s.cursor++
	if s.loseNextReply {
		s.loseNextReply = false
		return ports.CheckpointRestoreEvidenceConsumptionRefV1{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected consumption reply loss")
	}
	return ref, nil
}

func (s *CheckpointEvidenceStoreV1) InspectCheckpointPhaseEvidenceConsumptionCurrentV1(ctx context.Context, ref ports.CheckpointRestoreEvidenceConsumptionRefV1) (ports.CheckpointRestoreEvidenceConsumptionCurrentProjectionV1, error) {
	if _, err := s.InspectCheckpointPhaseEvidenceConsumptionHistoricalV1(ctx, ref); err != nil {
		return ports.CheckpointRestoreEvidenceConsumptionCurrentProjectionV1{}, err
	}
	projection := ports.CheckpointRestoreEvidenceConsumptionCurrentProjectionV1{Ref: ref, Current: ref.State == ports.CheckpointEvidenceConsumedCurrentV1, CheckedUnixNano: s.clock().UnixNano()}
	copy := projection
	copy.ProjectionDigest = ""
	projection.ProjectionDigest, _ = core.CanonicalJSONDigest("praxis.runtime.checkpoint-restore-evidence", ports.CheckpointRestoreEvidenceContractVersionV1, "CheckpointRestoreEvidenceConsumptionCurrentProjectionV1", copy)
	return projection, projection.Validate()
}

func (s *CheckpointEvidenceStoreV1) InspectCheckpointPhaseEvidenceConsumptionHistoricalV1(ctx context.Context, ref ports.CheckpointRestoreEvidenceConsumptionRefV1) (ports.CheckpointRestoreEvidenceConsumptionRefV1, error) {
	if err := contextError(ctx); err != nil {
		return ports.CheckpointRestoreEvidenceConsumptionRefV1{}, err
	}
	if err := ref.Validate(); err != nil {
		return ports.CheckpointRestoreEvidenceConsumptionRefV1{}, err
	}
	s.mu.Lock()
	stored, ok := s.consumptions[ref.ID]
	s.mu.Unlock()
	if !ok {
		return ports.CheckpointRestoreEvidenceConsumptionRefV1{}, core.NewError(core.ErrorNotFound, core.ReasonEvidenceUnavailable, "checkpoint Evidence consumption not found")
	}
	if stored != ref {
		return ports.CheckpointRestoreEvidenceConsumptionRefV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "checkpoint Evidence consumption drifted")
	}
	return stored, nil
}
