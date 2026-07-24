// Package store provides a deterministic, thread-safe reference repository.
// It proves the public fact semantics but does not claim production durability.
package store

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-definition/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-definition/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type currentFactV1 struct {
	ref                    contract.AgentDefinitionRefV1
	state                  contract.DefinitionCurrentStateV1
	revision               core.Revision
	updatedUnixNano        int64
	highestCheckedUnixNano int64
	reason                 string
}

type MemoryRepositoryV1 struct {
	mu          sync.RWMutex
	catalog     contract.ValidationCatalogV1
	history     map[string]map[core.Revision]contract.AgentDefinitionV1
	current     map[string]currentFactV1
	loseCreate  bool
	unavailable bool
}

func NewMemoryRepositoryV1(catalog contract.ValidationCatalogV1) *MemoryRepositoryV1 {
	return &MemoryRepositoryV1{catalog: contract.CloneValidationCatalogV1(catalog), history: map[string]map[core.Revision]contract.AgentDefinitionV1{}, current: map[string]currentFactV1{}}
}

func (s *MemoryRepositoryV1) LoseNextCreateReply() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loseCreate = true
}

func (s *MemoryRepositoryV1) SetUnavailable(value bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.unavailable = value
}

func (s *MemoryRepositoryV1) CreateDefinitionV1(ctx context.Context, request ports.CreateDefinitionRequestV1) (ports.CreateDefinitionResultV1, error) {
	if err := contextError(ctx); err != nil {
		return ports.CreateDefinitionResultV1{}, err
	}
	if s == nil {
		return ports.CreateDefinitionResultV1{}, invalid("definition repository is nil")
	}
	if err := request.Definition.Validate(s.catalog); err != nil {
		return ports.CreateDefinitionResultV1{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.unavailable {
		return ports.CreateDefinitionResultV1{}, unavailable("definition repository is unavailable")
	}
	byRevision := s.history[request.Definition.DefinitionID]
	if byRevision == nil {
		byRevision = map[core.Revision]contract.AgentDefinitionV1{}
		s.history[request.Definition.DefinitionID] = byRevision
	}
	if existing, ok := byRevision[request.Definition.Revision]; ok {
		if existing.Digest != request.Definition.Digest {
			return ports.CreateDefinitionResultV1{}, conflict("definition id and revision already bind different canonical content")
		}
		fact, ok := s.current[request.Definition.DefinitionID]
		if !ok {
			return ports.CreateDefinitionResultV1{}, core.NewError(core.ErrorInternal, core.ReasonInvalidState, "definition history exists without a current fact")
		}
		currentDefinition, currentExists := byRevision[fact.ref.Revision]
		if !currentExists || currentDefinition.Digest != fact.ref.Digest {
			return ports.CreateDefinitionResultV1{}, core.NewError(core.ErrorInternal, core.ReasonEvidenceConflict, "current definition fact does not bind immutable history")
		}
		checked := fact.highestCheckedUnixNano
		if checked < fact.updatedUnixNano {
			checked = fact.updatedUnixNano
		}
		current, err := s.projectLocked(currentDefinition, &fact, checked)
		if err != nil {
			return ports.CreateDefinitionResultV1{}, err
		}
		s.current[request.Definition.DefinitionID] = fact
		return ports.CreateDefinitionResultV1{Definition: contract.CloneDefinitionV1(existing), Current: current}, nil
	}
	fact, exists := s.current[request.Definition.DefinitionID]
	if !exists {
		if request.ExpectedCurrentRevision != 0 || request.Definition.Revision != 1 {
			return ports.CreateDefinitionResultV1{}, conflict("first definition revision requires expected current revision zero and definition revision one")
		}
		fact = currentFactV1{ref: request.Definition.RefV1(), state: contract.DefinitionCurrentActiveV1, revision: 1, updatedUnixNano: request.Definition.CreatedUnixNano, highestCheckedUnixNano: request.Definition.CreatedUnixNano}
	} else {
		if fact.state != contract.DefinitionCurrentActiveV1 || fact.revision != request.ExpectedCurrentRevision || request.Definition.Revision != fact.ref.Revision+1 || request.Definition.CreatedUnixNano < fact.updatedUnixNano || request.Definition.CreatedUnixNano < fact.highestCheckedUnixNano {
			return ports.CreateDefinitionResultV1{}, conflict("definition current CAS, revision sequence, state, or clock precondition failed")
		}
		fact.ref = request.Definition.RefV1()
		fact.revision++
		fact.updatedUnixNano = request.Definition.CreatedUnixNano
		fact.highestCheckedUnixNano = request.Definition.CreatedUnixNano
		fact.reason = ""
	}
	byRevision[request.Definition.Revision] = contract.CloneDefinitionV1(request.Definition)
	s.current[request.Definition.DefinitionID] = fact
	current, err := s.projectLocked(request.Definition, &fact, request.Definition.CreatedUnixNano)
	if err != nil {
		return ports.CreateDefinitionResultV1{}, err
	}
	s.current[request.Definition.DefinitionID] = fact
	result := ports.CreateDefinitionResultV1{Definition: contract.CloneDefinitionV1(request.Definition), Current: current}
	if s.loseCreate {
		s.loseCreate = false
		return ports.CreateDefinitionResultV1{}, unavailable("definition create reply was lost after commit")
	}
	return result, nil
}

func (s *MemoryRepositoryV1) InspectExactDefinitionV1(ctx context.Context, ref contract.AgentDefinitionRefV1) (contract.AgentDefinitionV1, error) {
	if err := contextError(ctx); err != nil {
		return contract.AgentDefinitionV1{}, err
	}
	if s == nil {
		return contract.AgentDefinitionV1{}, invalid("definition repository is nil")
	}
	if err := ref.Validate(); err != nil {
		return contract.AgentDefinitionV1{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.unavailable {
		return contract.AgentDefinitionV1{}, unavailable("definition repository is unavailable")
	}
	value, ok := s.history[ref.DefinitionID][ref.Revision]
	if !ok {
		return contract.AgentDefinitionV1{}, notFound("definition revision was not found")
	}
	if value.Digest != ref.Digest {
		return contract.AgentDefinitionV1{}, conflict("definition exact ref digest does not match stored content")
	}
	return contract.CloneDefinitionV1(value), nil
}

func (s *MemoryRepositoryV1) InspectDefinitionRevisionV1(ctx context.Context, definitionID string, revision core.Revision) (contract.AgentDefinitionV1, error) {
	if err := contextError(ctx); err != nil {
		return contract.AgentDefinitionV1{}, err
	}
	if s == nil || strings.TrimSpace(definitionID) == "" || revision == 0 {
		return contract.AgentDefinitionV1{}, invalid("definition revision request is incomplete")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.unavailable {
		return contract.AgentDefinitionV1{}, unavailable("definition repository is unavailable")
	}
	value, ok := s.history[definitionID][revision]
	if !ok {
		return contract.AgentDefinitionV1{}, notFound("definition revision was not found")
	}
	return contract.CloneDefinitionV1(value), nil
}

func (s *MemoryRepositoryV1) InspectCurrentDefinitionV1(ctx context.Context, definitionID string, checkedUnixNano int64) (contract.DefinitionCurrentV1, error) {
	if err := contextError(ctx); err != nil {
		return contract.DefinitionCurrentV1{}, err
	}
	if s == nil || strings.TrimSpace(definitionID) == "" || checkedUnixNano <= 0 {
		return contract.DefinitionCurrentV1{}, invalid("current definition request is incomplete")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.unavailable {
		return contract.DefinitionCurrentV1{}, unavailable("definition repository is unavailable")
	}
	fact, ok := s.current[definitionID]
	if !ok {
		return contract.DefinitionCurrentV1{}, notFound("current definition was not found")
	}
	definition, ok := s.history[definitionID][fact.ref.Revision]
	if !ok || definition.Digest != fact.ref.Digest {
		return contract.DefinitionCurrentV1{}, core.NewError(core.ErrorInternal, core.ReasonEvidenceConflict, "current definition fact does not bind immutable history")
	}
	current, err := s.projectLocked(definition, &fact, checkedUnixNano)
	if err != nil {
		return contract.DefinitionCurrentV1{}, err
	}
	s.current[definitionID] = fact
	return current, nil
}

func (s *MemoryRepositoryV1) RevokeDefinitionV1(ctx context.Context, request ports.RevokeDefinitionRequestV1) (contract.DefinitionCurrentV1, error) {
	if err := contextError(ctx); err != nil {
		return contract.DefinitionCurrentV1{}, err
	}
	if s == nil || strings.TrimSpace(request.DefinitionID) == "" || strings.TrimSpace(request.Reason) == "" || request.RevokedUnixNano <= 0 {
		return contract.DefinitionCurrentV1{}, invalid("revoke request is incomplete")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.unavailable {
		return contract.DefinitionCurrentV1{}, unavailable("definition repository is unavailable")
	}
	fact, ok := s.current[request.DefinitionID]
	if !ok {
		return contract.DefinitionCurrentV1{}, notFound("current definition was not found")
	}
	if fact.state == contract.DefinitionCurrentRevokedV1 {
		if fact.reason != request.Reason || request.ExpectedCurrentRevision+1 != fact.revision || request.RevokedUnixNano != fact.updatedUnixNano {
			return contract.DefinitionCurrentV1{}, conflict("revocation replay changed content")
		}
		definition := s.history[request.DefinitionID][fact.ref.Revision]
		checked := request.RevokedUnixNano
		if checked < fact.highestCheckedUnixNano {
			checked = fact.highestCheckedUnixNano
		}
		return s.projectLocked(definition, &fact, checked)
	}
	if fact.state != contract.DefinitionCurrentActiveV1 || fact.revision != request.ExpectedCurrentRevision || request.RevokedUnixNano < fact.updatedUnixNano || request.RevokedUnixNano < fact.highestCheckedUnixNano {
		return contract.DefinitionCurrentV1{}, conflict("definition revoke CAS, state, or clock precondition failed")
	}
	fact.state = contract.DefinitionCurrentRevokedV1
	fact.revision++
	fact.updatedUnixNano = request.RevokedUnixNano
	fact.reason = request.Reason
	s.current[request.DefinitionID] = fact
	definition := s.history[request.DefinitionID][fact.ref.Revision]
	return s.projectLocked(definition, &fact, request.RevokedUnixNano)
}

func (s *MemoryRepositoryV1) projectLocked(definition contract.AgentDefinitionV1, fact *currentFactV1, checked int64) (contract.DefinitionCurrentV1, error) {
	if checked < fact.updatedUnixNano || checked < fact.highestCheckedUnixNano {
		return contract.DefinitionCurrentV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "definition current clock regressed")
	}
	fact.highestCheckedUnixNano = checked
	state := fact.state
	if state == contract.DefinitionCurrentActiveV1 && checked >= definition.EffectiveWindow.NotAfterUnixNano {
		state = contract.DefinitionCurrentExpiredV1
	}
	return contract.SealDefinitionCurrentV1(contract.DefinitionCurrentV1{Definition: fact.ref, State: state, Revision: fact.revision, UpdatedUnixNano: fact.updatedUnixNano, CheckedUnixNano: checked, Reason: fact.reason})
}

func contextError(ctx context.Context) error {
	if ctx == nil {
		return invalid("context is nil")
	}
	if err := ctx.Err(); err != nil {
		return core.NewError(core.ErrorUnavailable, core.ReasonInvalidState, "context is canceled")
	}
	return nil
}

func invalid(message string) error {
	return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, message)
}
func conflict(message string) error {
	return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, message)
}
func unavailable(message string) error {
	return core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, message)
}
func notFound(message string) error {
	return core.NewError(core.ErrorNotFound, core.ReasonComponentMissing, message)
}

func (s *MemoryRepositoryV1) String() string {
	if s == nil {
		return "<nil>"
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return fmt.Sprintf("MemoryRepositoryV1{definitions:%d}", len(s.current))
}
