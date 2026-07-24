package memory

import (
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

// RubricSnapshotV1 is optional for backward compatibility with snapshots that
// predate Review-owned Rubric publication.
type RubricSnapshotV1 struct {
	Current         map[string]contract.ExactResourceRefV1                   `json:"current"`
	History         map[string]map[core.Revision]contract.RubricDefinitionV1 `json:"history"`
	HighestRevision map[string]core.Revision                                 `json:"highest_revision"`
}

func emptyRubricSnapshotV1() *RubricSnapshotV1 {
	return &RubricSnapshotV1{Current: map[string]contract.ExactResourceRefV1{}, History: map[string]map[core.Revision]contract.RubricDefinitionV1{}, HighestRevision: map[string]core.Revision{}}
}

func (r *RubricSnapshotV1) validate(tenant core.TenantID) error {
	if r == nil {
		return nil
	}
	if r.Current == nil || r.History == nil || r.HighestRevision == nil || len(r.Current) != len(r.History) || len(r.Current) != len(r.HighestRevision) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "rubric snapshot indexes are incomplete")
	}
	for id, history := range r.History {
		current, ok := r.Current[id]
		if !ok || current.ID != id || r.HighestRevision[id] != current.Revision || len(history) != int(current.Revision) {
			return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "rubric snapshot current or highest revision drifted")
		}
		for revision := core.Revision(1); revision <= current.Revision; revision++ {
			value, ok := history[revision]
			if !ok || value.TenantID != tenant || value.ID != id || value.Revision != revision || value.Validate() != nil {
				return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "rubric snapshot history is invalid or has a gap")
			}
			if revision == 1 {
				if value.State != contract.RubricActiveV1 {
					return core.NewError(core.ErrorConflict, core.ReasonInvalidTransition, "rubric snapshot first revision is not active")
				}
			} else {
				previous := history[revision-1]
				if previous.State != contract.RubricActiveV1 || value.Kind != previous.Kind || value.CreatedUnixNano != previous.CreatedUnixNano || value.UpdatedUnixNano <= previous.UpdatedUnixNano {
					return core.NewError(core.ErrorConflict, core.ReasonInvalidTransition, "rubric snapshot history revives a terminal revision or changes immutable identity")
				}
				if value.State == contract.RubricRevokedV1 && !sameRubricRevocationPayload(previous, value) {
					return core.NewError(core.ErrorConflict, core.ReasonInvalidTransition, "rubric snapshot revoke changed immutable definition content")
				}
			}
			if revision == current.Revision && !sameRubricRef(value.ExactRef(), current) {
				return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "rubric snapshot current ref does not match history")
			}
		}
	}
	return nil
}

func (s *Store) exportRubricSnapshotV1(tenant core.TenantID) *RubricSnapshotV1 {
	prefix := string(tenant) + "\x00"
	r := emptyRubricSnapshotV1()
	for itemKey, value := range s.rubricCurrent {
		if strings.HasPrefix(itemKey, prefix) {
			r.Current[strings.TrimPrefix(itemKey, prefix)], _ = clone(value)
		}
	}
	if len(r.Current) == 0 {
		return nil
	}
	for itemKey, value := range s.rubricHistory {
		if strings.HasPrefix(itemKey, prefix) {
			r.History[strings.TrimPrefix(itemKey, prefix)], _ = clone(value)
		}
	}
	for itemKey, value := range s.rubricHighestRevision {
		if strings.HasPrefix(itemKey, prefix) {
			r.HighestRevision[strings.TrimPrefix(itemKey, prefix)] = value
		}
	}
	return r
}

func (s *Store) importRubricSnapshotV1(tenant core.TenantID, r *RubricSnapshotV1) {
	if r == nil {
		return
	}
	for id, value := range r.Current {
		s.rubricCurrent[key(tenant, id)], _ = clone(value)
	}
	for id, value := range r.History {
		s.rubricHistory[key(tenant, id)], _ = clone(value)
	}
	for id, value := range r.HighestRevision {
		s.rubricHighestRevision[key(tenant, id)] = value
	}
}
