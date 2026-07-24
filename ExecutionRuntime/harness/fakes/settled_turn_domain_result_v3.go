package fakes

import (
	"context"
	"sync"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/contract"
	harnessports "github.com/Proview-China/rax/ExecutionRuntime/harness/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

// SettledTurnDomainResultRepositoryV3 is a test-only, in-memory owner store.
// It proves contract behavior, not production durability or SLA.
type SettledTurnDomainResultRepositoryV3 struct {
	mu                  sync.RWMutex
	historyByFactID     map[string]contract.SettledTurnDomainResultFactV3
	currentBySource     map[core.Digest]string
	identityIndex       map[string]string
	LoseNextEnsureReply bool
}

func NewSettledTurnDomainResultRepositoryV3() *SettledTurnDomainResultRepositoryV3 {
	return &SettledTurnDomainResultRepositoryV3{historyByFactID: map[string]contract.SettledTurnDomainResultFactV3{}, currentBySource: map[core.Digest]string{}, identityIndex: map[string]string{}}
}

var _ harnessports.SettledTurnDomainResultRepositoryV3 = (*SettledTurnDomainResultRepositoryV3)(nil)
var _ harnessports.SettledTurnDomainResultReaderV3 = (*SettledTurnDomainResultRepositoryV3)(nil)

func (r *SettledTurnDomainResultRepositoryV3) EnsureExact(_ context.Context, fact contract.SettledTurnDomainResultFactV3) (contract.SettledTurnDomainResultFactV3, error) {
	if r == nil {
		return contract.SettledTurnDomainResultFactV3{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "SettledTurn repository is unavailable")
	}
	if err := fact.Validate(); err != nil {
		return contract.SettledTurnDomainResultFactV3{}, err
	}
	sourceDigest, _ := fact.SourceKey.DigestV1()
	r.mu.Lock()
	defer r.mu.Unlock()
	if existingID, ok := r.currentBySource[sourceDigest]; ok {
		existing := r.historyByFactID[existingID]
		if existing.FactDigest != fact.FactDigest {
			return contract.SettledTurnDomainResultFactV3{}, core.NewError(core.ErrorConflict, core.ReasonAlreadyExists, "SettledTurn source key already has different immutable content")
		}
		return existing.Clone(), nil
	}
	if existing, ok := r.historyByFactID[fact.FactID]; ok {
		if existing.FactDigest != fact.FactDigest {
			return contract.SettledTurnDomainResultFactV3{}, core.NewError(core.ErrorConflict, core.ReasonAlreadyExists, "SettledTurn fact ID already has different immutable content")
		}
		return existing.Clone(), nil
	}
	if existingID, ok := r.identityIndex[fact.Identity.ID]; ok && existingID != fact.FactID {
		return contract.SettledTurnDomainResultFactV3{}, core.NewError(core.ErrorConflict, core.ReasonAlreadyExists, "G6A identity ID already belongs to another DomainResult")
	}
	r.historyByFactID[fact.FactID] = fact.Clone()
	r.currentBySource[sourceDigest] = fact.FactID
	r.identityIndex[fact.Identity.ID] = fact.FactID
	if r.LoseNextEnsureReply {
		r.LoseNextEnsureReply = false
		return contract.SettledTurnDomainResultFactV3{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected SettledTurn Ensure reply loss")
	}
	return fact.Clone(), nil
}

func (r *SettledTurnDomainResultRepositoryV3) InspectExact(_ context.Context, ref contract.SettledTurnDomainResultFactRefV3) (contract.SettledTurnDomainResultFactV3, error) {
	if r == nil {
		return contract.SettledTurnDomainResultFactV3{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "SettledTurn repository is unavailable")
	}
	if err := ref.Validate(); err != nil {
		return contract.SettledTurnDomainResultFactV3{}, err
	}
	r.mu.RLock()
	fact, ok := r.historyByFactID[ref.FactID]
	r.mu.RUnlock()
	if !ok {
		return contract.SettledTurnDomainResultFactV3{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "SettledTurn DomainResult fact not found")
	}
	actual, err := fact.RefV3()
	if err != nil {
		return contract.SettledTurnDomainResultFactV3{}, err
	}
	if actual != ref {
		return contract.SettledTurnDomainResultFactV3{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "SettledTurn DomainResult exact ref drifted")
	}
	return fact.Clone(), nil
}

func (r *SettledTurnDomainResultRepositoryV3) HistoryLenV3() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.historyByFactID)
}
