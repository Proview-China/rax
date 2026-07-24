package memory

import (
	"strconv"
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

// BypassSnapshotV1 is optional in SnapshotV1. Nil means an empty pre-Bypass
// state, preserving compatibility with snapshots sealed before Bypass V1.
type BypassSnapshotV1 struct {
	Decisions       map[string]contract.BypassDecisionV1                   `json:"decisions"`
	DecisionHistory map[string]map[core.Revision]contract.BypassDecisionV1 `json:"decision_history"`
	DecisionByCase  map[string]contract.BypassDecisionExactRefV1           `json:"decision_by_case"`
	HighestRevision map[string]core.Revision                               `json:"highest_revision"`
	TraceByRevision map[string]map[core.Revision]contract.FactIdentityV1   `json:"trace_by_revision"`
}

func emptyBypassSnapshotV1() *BypassSnapshotV1 {
	return &BypassSnapshotV1{
		Decisions: map[string]contract.BypassDecisionV1{}, DecisionHistory: map[string]map[core.Revision]contract.BypassDecisionV1{},
		DecisionByCase: map[string]contract.BypassDecisionExactRefV1{}, HighestRevision: map[string]core.Revision{}, TraceByRevision: map[string]map[core.Revision]contract.FactIdentityV1{},
	}
}

func snapshotBypassCaseKey(ref contract.BypassCaseExactRefV1) string {
	return ref.ID + "\x00" + strconv.FormatUint(uint64(ref.Revision), 10) + "\x00" + string(ref.Digest)
}

func (b *BypassSnapshotV1) validate(tenant core.TenantID) error {
	if b == nil {
		return nil
	}
	if b.Decisions == nil || b.DecisionHistory == nil || b.DecisionByCase == nil || b.HighestRevision == nil || b.TraceByRevision == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "bypass snapshot maps must be present")
	}
	if err := validateCurrentHistory(tenant, b.Decisions, b.DecisionHistory, func(v contract.BypassDecisionV1) contract.FactIdentityV1 { return v.FactIdentityV1 }, func(v contract.BypassDecisionV1) error { return v.Validate() }, "bypass Decision"); err != nil {
		return err
	}
	if len(b.DecisionByCase) != len(b.Decisions) || len(b.HighestRevision) != len(b.Decisions) {
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "bypass snapshot current indexes are incomplete")
	}
	for id, current := range b.Decisions {
		if current.TenantID != tenant || b.HighestRevision[id] != current.Revision {
			return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "bypass snapshot highest revision drifted")
		}
		ref, ok := b.DecisionByCase[snapshotBypassCaseKey(current.Case)]
		if !ok || !sameBypassRef(ref, current.ExactRef()) {
			return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "bypass snapshot Case current index drifted")
		}
		history := b.DecisionHistory[id]
		if len(history) != int(current.Revision) {
			return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "bypass snapshot revision history is not contiguous")
		}
		for revision := core.Revision(1); revision <= current.Revision; revision++ {
			if _, ok := history[revision]; !ok {
				return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "bypass snapshot revision history has a gap")
			}
		}
	}
	for id, revisions := range b.TraceByRevision {
		for revision, trace := range revisions {
			if _, ok := b.DecisionHistory[id][revision]; !ok || trace.TenantID != tenant || trace.ValidateShape() != nil {
				return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "bypass snapshot Trace binding has no exact Decision revision")
			}
		}
	}
	return nil
}

func (b *BypassSnapshotV1) validateTraces(traces map[string]contract.TraceFactV1) error {
	if b == nil {
		return nil
	}
	for _, revisions := range b.TraceByRevision {
		for _, ref := range revisions {
			trace, ok := traces[ref.ID]
			if !ok || trace.Revision != ref.Revision || trace.Digest != ref.Digest || trace.TenantID != ref.TenantID {
				return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "bypass snapshot exact Trace binding drifted")
			}
		}
	}
	return nil
}

func (s *Store) exportBypassSnapshotV1(tenant core.TenantID) *BypassSnapshotV1 {
	prefix := string(tenant) + "\x00"
	b := emptyBypassSnapshotV1()
	for k, v := range s.bypassDecisions {
		if strings.HasPrefix(k, prefix) {
			b.Decisions[strings.TrimPrefix(k, prefix)], _ = clone(v)
		}
	}
	if len(b.Decisions) == 0 {
		return nil
	}
	for k, v := range s.bypassDecisionHistory {
		if strings.HasPrefix(k, prefix) {
			b.DecisionHistory[strings.TrimPrefix(k, prefix)], _ = clone(v)
		}
	}
	for k, v := range s.bypassDecisionByCase {
		if strings.HasPrefix(k, prefix) {
			b.DecisionByCase[strings.TrimPrefix(k, prefix)], _ = clone(v)
		}
	}
	for k, v := range s.bypassHighestRevision {
		if strings.HasPrefix(k, prefix) {
			b.HighestRevision[strings.TrimPrefix(k, prefix)] = v
		}
	}
	for k, v := range s.bypassTraceByRevision {
		if strings.HasPrefix(k, prefix) {
			b.TraceByRevision[strings.TrimPrefix(k, prefix)], _ = clone(v)
		}
	}
	return b
}

func (s *Store) importBypassSnapshotV1(tenant core.TenantID, b *BypassSnapshotV1) {
	if b == nil {
		return
	}
	for id, value := range b.Decisions {
		s.bypassDecisions[key(tenant, id)], _ = clone(value)
	}
	for id, history := range b.DecisionHistory {
		s.bypassDecisionHistory[key(tenant, id)], _ = clone(history)
	}
	for caseKey, ref := range b.DecisionByCase {
		s.bypassDecisionByCase[string(tenant)+"\x00"+caseKey], _ = clone(ref)
	}
	for id, revision := range b.HighestRevision {
		s.bypassHighestRevision[key(tenant, id)] = revision
	}
	for id, revisions := range b.TraceByRevision {
		s.bypassTraceByRevision[key(tenant, id)], _ = clone(revisions)
	}
}
