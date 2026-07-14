package fakes

import (
	"context"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"sync"
)

// RunClaimAssociationStoreV2 is a test-only create-once sidecar owner. It
// preserves the exact V2 ledger reference without mutating the legacy Run
// completion-claim shape.
type RunClaimAssociationStoreV2 struct {
	mu            sync.Mutex
	facts         map[string]ports.RunClaimAssociationFactV2
	loseNextReply bool
}

func NewRunClaimAssociationStoreV2() *RunClaimAssociationStoreV2 {
	return &RunClaimAssociationStoreV2{facts: map[string]ports.RunClaimAssociationFactV2{}}
}
func (s *RunClaimAssociationStoreV2) LoseNextReply() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loseNextReply = true
}
func (s *RunClaimAssociationStoreV2) CreateRunClaimAssociation(ctx context.Context, fact ports.RunClaimAssociationFactV2) (ports.RunClaimAssociationFactV2, error) {
	if err := contextError(ctx); err != nil {
		return ports.RunClaimAssociationFactV2{}, err
	}
	if err := fact.Validate(); err != nil {
		return ports.RunClaimAssociationFactV2{}, err
	}
	key := runClaimAssociationKeyV2(fact.ExecutionScopeDigest, fact.RunID)
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.facts[key]; ok {
		left, _ := existing.DigestV2()
		right, _ := fact.DigestV2()
		if left == right {
			return cloneEvidenceV2(existing), nil
		}
		return ports.RunClaimAssociationFactV2{}, core.NewError(core.ErrorConflict, core.ReasonRunClaimConflict, "run already has a different V2 completion claim association")
	}
	s.facts[key] = cloneEvidenceV2(fact)
	if s.loseNextReply {
		s.loseNextReply = false
		return ports.RunClaimAssociationFactV2{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected run claim association reply loss")
	}
	return cloneEvidenceV2(fact), nil
}
func (s *RunClaimAssociationStoreV2) InspectRunClaimAssociation(ctx context.Context, scope core.Digest, run core.AgentRunID) (ports.RunClaimAssociationFactV2, error) {
	if err := contextError(ctx); err != nil {
		return ports.RunClaimAssociationFactV2{}, err
	}
	if scope.Validate() != nil || run == "" {
		return ports.RunClaimAssociationFactV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonRunClaimUnverified, "scope digest and run are required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	fact, ok := s.facts[runClaimAssociationKeyV2(scope, run)]
	if !ok {
		return ports.RunClaimAssociationFactV2{}, core.NewError(core.ErrorNotFound, core.ReasonRunClaimUnverified, "run claim association not found")
	}
	return cloneEvidenceV2(fact), nil
}
func runClaimAssociationKeyV2(scope core.Digest, run core.AgentRunID) string {
	return string(scope) + "\x00" + string(run)
}
