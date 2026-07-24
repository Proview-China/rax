package memory

import (
	"context"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func sameRubricRef(a, b contract.ExactResourceRefV1) bool {
	return a.ID == b.ID && a.Revision == b.Revision && a.Digest == b.Digest
}

func sameRubricRevocationPayload(a, b contract.RubricDefinitionV1) bool {
	a.Revision, b.Revision = 0, 0
	a.UpdatedUnixNano, b.UpdatedUnixNano = 0, 0
	a.Digest, b.Digest = "", ""
	a.State, b.State = "", ""
	return rubricEqual(a, b)
}

func rubricEqual(a, b contract.RubricDefinitionV1) bool {
	aPayload, err := clone(a)
	if err != nil {
		return false
	}
	bPayload, err := clone(b)
	if err != nil {
		return false
	}
	// Canonical structs contain slices and are therefore compared by their
	// canonical JSON representation through a deterministic digest.
	aDigest, err := core.CanonicalJSONDigest("praxis.review.rubric.compare", contract.ContractVersionV1, "RubricDefinitionV1", aPayload)
	if err != nil {
		return false
	}
	bDigest, err := core.CanonicalJSONDigest("praxis.review.rubric.compare", contract.ContractVersionV1, "RubricDefinitionV1", bPayload)
	return err == nil && aDigest == bDigest
}

func (s *Store) PublishRubricV1(ctx context.Context, m reviewport.PublishRubricMutationV1) (contract.RubricDefinitionV1, error) {
	if err := checkContext(ctx); err != nil {
		return contract.RubricDefinitionV1{}, err
	}
	if err := m.Next.Validate(); err != nil {
		return contract.RubricDefinitionV1{}, err
	}
	if m.Next.State != contract.RubricActiveV1 {
		return contract.RubricDefinitionV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidTransition, "rubric publish requires an active definition")
	}
	if m.Expected == nil {
		if m.Next.Revision != 1 {
			return contract.RubricDefinitionV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "rubric create must start at revision one")
		}
	} else {
		if err := m.Expected.Validate(); err != nil {
			return contract.RubricDefinitionV1{}, err
		}
		if m.Next.ID != m.Expected.ID || m.Next.Revision != m.Expected.Revision+1 {
			return contract.RubricDefinitionV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "rubric supersede must advance the same ID by one revision")
		}
	}
	next, err := clone(m.Next)
	if err != nil {
		return contract.RubricDefinitionV1{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.publishRubricLocked(m.Expected, next)
}

func (s *Store) publishRubricLocked(expected *contract.ExactResourceRefV1, next contract.RubricDefinitionV1) (contract.RubricDefinitionV1, error) {
	idKey := key(next.TenantID, next.ID)
	if existing, ok := s.rubricHistory[idKey][next.Revision]; ok {
		if existing.Digest != next.Digest {
			return contract.RubricDefinitionV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "rubric publish replay changed content")
		}
		if expected == nil {
			if next.Revision != 1 {
				return contract.RubricDefinitionV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "rubric publish replay changed create semantics")
			}
		} else {
			previous, ok := s.rubricHistory[idKey][expected.Revision]
			if !ok || previous.Digest != expected.Digest {
				return contract.RubricDefinitionV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "rubric publish replay changed expected ref")
			}
		}
		return clone(existing)
	}
	if expected == nil {
		if s.rubricHighestRevision[idKey] != 0 || s.rubricHistory[idKey] != nil {
			return contract.RubricDefinitionV1{}, exists("rubric")
		}
	} else {
		current, ok := s.rubricCurrent[idKey]
		if !ok || !sameRubricRef(current, *expected) || s.rubricHighestRevision[idKey] != expected.Revision {
			return contract.RubricDefinitionV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "rubric publish expected current ref is stale")
		}
		previous, ok := s.rubricHistory[idKey][expected.Revision]
		if !ok || previous.Digest != expected.Digest {
			return contract.RubricDefinitionV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "rubric current index points to missing history")
		}
		if previous.State != contract.RubricActiveV1 || next.TenantID != previous.TenantID || next.ID != previous.ID || next.Kind != previous.Kind || next.CreatedUnixNano != previous.CreatedUnixNano || next.UpdatedUnixNano <= previous.UpdatedUnixNano {
			return contract.RubricDefinitionV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidTransition, "rubric supersede changed immutable identity or revived a terminal definition")
		}
	}
	appendHistory(s.rubricHistory, idKey, next.Revision, next)
	s.rubricCurrent[idKey] = next.ExactRef()
	s.rubricHighestRevision[idKey] = next.Revision
	return clone(next)
}

func (s *Store) RevokeRubricV1(ctx context.Context, m reviewport.RevokeRubricMutationV1) (contract.RubricDefinitionV1, error) {
	if err := checkContext(ctx); err != nil {
		return contract.RubricDefinitionV1{}, err
	}
	if err := m.Expected.Validate(); err != nil {
		return contract.RubricDefinitionV1{}, err
	}
	if err := m.Next.Validate(); err != nil {
		return contract.RubricDefinitionV1{}, err
	}
	if m.Next.State != contract.RubricRevokedV1 || m.Next.ID != m.Expected.ID || m.Next.Revision != m.Expected.Revision+1 {
		return contract.RubricDefinitionV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidTransition, "rubric revoke must publish the next terminal revision")
	}
	next, err := clone(m.Next)
	if err != nil {
		return contract.RubricDefinitionV1{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	idKey := key(next.TenantID, next.ID)
	if existing, ok := s.rubricHistory[idKey][next.Revision]; ok {
		if existing.Digest != next.Digest {
			return contract.RubricDefinitionV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "rubric revoke replay changed content")
		}
		previous, ok := s.rubricHistory[idKey][m.Expected.Revision]
		if !ok || previous.Digest != m.Expected.Digest || !sameRubricRevocationPayload(previous, existing) {
			return contract.RubricDefinitionV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "rubric revoke replay changed lifecycle")
		}
		return clone(existing)
	}
	current, ok := s.rubricCurrent[idKey]
	if !ok || !sameRubricRef(current, m.Expected) || s.rubricHighestRevision[idKey] != m.Expected.Revision {
		return contract.RubricDefinitionV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "rubric revoke expected current ref is stale")
	}
	previous, ok := s.rubricHistory[idKey][m.Expected.Revision]
	if !ok || previous.Digest != m.Expected.Digest || previous.State != contract.RubricActiveV1 {
		return contract.RubricDefinitionV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidTransition, "rubric revoke cannot resolve the active historical definition")
	}
	if next.TenantID != previous.TenantID || next.ID != previous.ID || next.Revision != previous.Revision+1 || next.CreatedUnixNano != previous.CreatedUnixNano || next.UpdatedUnixNano <= previous.UpdatedUnixNano || !sameRubricRevocationPayload(previous, next) {
		return contract.RubricDefinitionV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidTransition, "rubric revoke changed immutable definition content")
	}
	appendHistory(s.rubricHistory, idKey, next.Revision, next)
	s.rubricCurrent[idKey] = next.ExactRef()
	s.rubricHighestRevision[idKey] = next.Revision
	return clone(next)
}

func (s *Store) InspectRubricExactV1(ctx context.Context, tenant core.TenantID, ref contract.ExactResourceRefV1) (contract.RubricDefinitionV1, error) {
	if err := checkContext(ctx); err != nil {
		return contract.RubricDefinitionV1{}, err
	}
	if err := ref.Validate(); err != nil {
		return contract.RubricDefinitionV1{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.rubricHistory[key(tenant, ref.ID)][ref.Revision]
	if !ok {
		return contract.RubricDefinitionV1{}, notFound("rubric")
	}
	if value.TenantID != tenant || !sameRubricRef(value.ExactRef(), ref) {
		return contract.RubricDefinitionV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "rubric exact ref drifted")
	}
	return clone(value)
}

func (s *Store) InspectRubricCurrentV1(ctx context.Context, tenant core.TenantID, expected contract.ExactResourceRefV1, now time.Time) (contract.RubricDefinitionV1, error) {
	if err := checkContext(ctx); err != nil {
		return contract.RubricDefinitionV1{}, err
	}
	if err := expected.Validate(); err != nil {
		return contract.RubricDefinitionV1{}, err
	}
	if now.IsZero() {
		return contract.RubricDefinitionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "rubric current clock is unavailable")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	idKey := key(tenant, expected.ID)
	current, ok := s.rubricCurrent[idKey]
	if !ok {
		return contract.RubricDefinitionV1{}, notFound("current rubric")
	}
	if !sameRubricRef(current, expected) || s.rubricHighestRevision[idKey] != expected.Revision {
		return contract.RubricDefinitionV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "rubric current index drifted")
	}
	value, ok := s.rubricHistory[idKey][expected.Revision]
	if !ok || !sameRubricRef(value.ExactRef(), expected) {
		return contract.RubricDefinitionV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "rubric current index points to missing history")
	}
	if err := value.ValidateCurrent(expected, now); err != nil {
		return contract.RubricDefinitionV1{}, err
	}
	return clone(value)
}
