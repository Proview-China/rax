package fakes

import (
	"context"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// RunPlanAdmissionStoreV3 is a deterministic create-once test Fact Owner. It
// proves idempotency and recovery semantics only; it is not a production
// catalog, certification authority, database or SLA claim.
type RunPlanAdmissionStoreV3 struct {
	mu                    sync.Mutex
	clock                 func() time.Time
	declarations          map[string]ports.RunSettlementDeclarationFactV3
	baselines             map[string]ports.RunSettlementBaselinePolicyFactV3
	certifications        map[string]ports.RunSettlementPlanCertificationFactV3
	loseNextCertification bool
}

func NewRunPlanAdmissionStoreV3(clock func() time.Time) *RunPlanAdmissionStoreV3 {
	if clock == nil {
		clock = time.Now
	}
	return &RunPlanAdmissionStoreV3{clock: clock, declarations: map[string]ports.RunSettlementDeclarationFactV3{}, baselines: map[string]ports.RunSettlementBaselinePolicyFactV3{}, certifications: map[string]ports.RunSettlementPlanCertificationFactV3{}}
}

func (s *RunPlanAdmissionStoreV3) LoseNextCertificationReply() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loseNextCertification = true
}

func (s *RunPlanAdmissionStoreV3) CreateRunSettlementDeclarationV3(ctx context.Context, fact ports.RunSettlementDeclarationFactV3) (ports.RunSettlementDeclarationFactV3, error) {
	if err := contextError(ctx); err != nil {
		return ports.RunSettlementDeclarationFactV3{}, err
	}
	if err := fact.Validate(); err != nil {
		return ports.RunSettlementDeclarationFactV3{}, err
	}
	key := fact.BindingSetID + "\x00" + string(fact.ComponentID)
	s.mu.Lock()
	defer s.mu.Unlock()
	if current, exists := s.declarations[key]; exists {
		if current.Digest == fact.Digest {
			return cloneRunSettlementDeclarationV3(current), nil
		}
		return ports.RunSettlementDeclarationFactV3{}, core.NewError(core.ErrorConflict, core.ReasonRunSettlementPlanConflict, "component declaration identity already binds different content")
	}
	s.declarations[key] = cloneRunSettlementDeclarationV3(fact)
	return cloneRunSettlementDeclarationV3(fact), nil
}

func (s *RunPlanAdmissionStoreV3) InspectRunSettlementDeclarationV3(ctx context.Context, bindingSetID string, componentID ports.ComponentIDV2) (ports.RunSettlementDeclarationFactV3, error) {
	if err := contextError(ctx); err != nil {
		return ports.RunSettlementDeclarationFactV3{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	fact, exists := s.declarations[bindingSetID+"\x00"+string(componentID)]
	if !exists {
		return ports.RunSettlementDeclarationFactV3{}, core.NewError(core.ErrorNotFound, core.ReasonRunSettlementPlanConflict, "component Run settlement declaration is missing")
	}
	return cloneRunSettlementDeclarationV3(fact), nil
}

func (s *RunPlanAdmissionStoreV3) CreateRunSettlementBaselinePolicyV3(ctx context.Context, fact ports.RunSettlementBaselinePolicyFactV3) (ports.RunSettlementBaselinePolicyFactV3, error) {
	if err := contextError(ctx); err != nil {
		return ports.RunSettlementBaselinePolicyFactV3{}, err
	}
	if err := fact.Validate(); err != nil {
		return ports.RunSettlementBaselinePolicyFactV3{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if current, exists := s.baselines[fact.ID]; exists {
		if current.Digest == fact.Digest {
			return cloneRunSettlementBaselineV3(current), nil
		}
		return ports.RunSettlementBaselinePolicyFactV3{}, core.NewError(core.ErrorConflict, core.ReasonRunSettlementPlanConflict, "baseline policy identity already binds different content")
	}
	s.baselines[fact.ID] = cloneRunSettlementBaselineV3(fact)
	return cloneRunSettlementBaselineV3(fact), nil
}

func (s *RunPlanAdmissionStoreV3) InspectRunSettlementBaselinePolicyV3(ctx context.Context, id string) (ports.RunSettlementBaselinePolicyFactV3, error) {
	if err := contextError(ctx); err != nil {
		return ports.RunSettlementBaselinePolicyFactV3{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	fact, exists := s.baselines[id]
	if !exists {
		return ports.RunSettlementBaselinePolicyFactV3{}, core.NewError(core.ErrorNotFound, core.ReasonRunSettlementPlanConflict, "Run settlement baseline policy is missing")
	}
	return cloneRunSettlementBaselineV3(fact), nil
}

func (s *RunPlanAdmissionStoreV3) CreateRunSettlementPlanCertificationV3(ctx context.Context, fact ports.RunSettlementPlanCertificationFactV3) (ports.RunSettlementPlanCertificationFactV3, error) {
	if err := contextError(ctx); err != nil {
		return ports.RunSettlementPlanCertificationFactV3{}, err
	}
	if err := fact.Validate(); err != nil {
		return ports.RunSettlementPlanCertificationFactV3{}, err
	}
	key := runKey(fact.ExecutionScope, fact.RunID)
	s.mu.Lock()
	defer s.mu.Unlock()
	if current, exists := s.certifications[key]; exists {
		if current.Digest == fact.Digest {
			return cloneRunSettlementCertificationV3(current), nil
		}
		return ports.RunSettlementPlanCertificationFactV3{}, core.NewError(core.ErrorConflict, core.ReasonRunSettlementPlanConflict, "Plan certification identity already binds different content")
	}
	if !s.clock().Before(time.Unix(0, fact.ExpiresUnixNano)) || fact.CreatedUnixNano > s.clock().UnixNano() {
		return ports.RunSettlementPlanCertificationFactV3{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonRunSettlementPlanConflict, "new Plan certification is expired or from the future")
	}
	s.certifications[key] = cloneRunSettlementCertificationV3(fact)
	if s.loseNextCertification {
		s.loseNextCertification = false
		return ports.RunSettlementPlanCertificationFactV3{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected Plan certification reply loss")
	}
	return cloneRunSettlementCertificationV3(fact), nil
}

func (s *RunPlanAdmissionStoreV3) InspectRunSettlementPlanCertificationV3(ctx context.Context, scope core.ExecutionScope, runID core.AgentRunID) (ports.RunSettlementPlanCertificationFactV3, error) {
	if err := contextError(ctx); err != nil {
		return ports.RunSettlementPlanCertificationFactV3{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	fact, exists := s.certifications[runKey(scope, runID)]
	if !exists {
		return ports.RunSettlementPlanCertificationFactV3{}, core.NewError(core.ErrorNotFound, core.ReasonRunSettlementPlanConflict, "Run settlement Plan certification is missing")
	}
	return cloneRunSettlementCertificationV3(fact), nil
}

func cloneRunSettlementDeclarationV3(f ports.RunSettlementDeclarationFactV3) ports.RunSettlementDeclarationFactV3 {
	f.Requirements = append([]ports.RunSettlementRequirementV2{}, f.Requirements...)
	return f
}

func cloneRunSettlementBaselineV3(f ports.RunSettlementBaselinePolicyFactV3) ports.RunSettlementBaselinePolicyFactV3 {
	f.Requirements = append([]ports.RunSettlementRequirementV2{}, f.Requirements...)
	return f
}

func cloneRunSettlementCertificationV3(f ports.RunSettlementPlanCertificationFactV3) ports.RunSettlementPlanCertificationFactV3 {
	f.ExecutionScope = cloneScope(f.ExecutionScope)
	f.Declarations = append([]ports.RunSettlementDeclarationRefV3{}, f.Declarations...)
	return f
}

var _ ports.RunSettlementDeclarationFactPortV3 = (*RunPlanAdmissionStoreV3)(nil)
var _ ports.RunSettlementBaselinePolicyFactPortV3 = (*RunPlanAdmissionStoreV3)(nil)
var _ ports.RunSettlementPlanCertificationFactPortV3 = (*RunPlanAdmissionStoreV3)(nil)
