package memory

import (
	"context"
	"strconv"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func bypassCaseKey(ref contract.BypassCaseExactRefV1) string {
	return key(ref.TenantID, ref.ID) + "\x00" + strconv.FormatUint(uint64(ref.Revision), 10) + "\x00" + string(ref.Digest)
}

func sameBypassRef(a, b contract.BypassDecisionExactRefV1) bool {
	return a.TenantID == b.TenantID && a.ID == b.ID && a.Revision == b.Revision && a.Digest == b.Digest
}

func validateBypassTrace(decision contract.BypassDecisionV1, trace contract.TraceFactV1) error {
	if trace.ID == "" {
		return nil
	}
	if err := trace.Validate(); err != nil {
		return err
	}
	if trace.TenantID != decision.TenantID || trace.CaseID != decision.Case.ID || trace.CaseRevision != decision.Case.Revision || trace.TargetID != decision.Target.ID || trace.TargetRevision != decision.Target.Revision || trace.TargetDigest != decision.Target.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "bypass trace does not bind the exact Decision Case and Target")
	}
	return nil
}

func sameBypassLifecycle(a, b contract.BypassDecisionV1) bool {
	a.Revision, b.Revision = 0, 0
	a.UpdatedUnixNano, b.UpdatedUnixNano = 0, 0
	a.Digest, b.Digest = "", ""
	a.State, b.State = "", ""
	a.InvalidationReason, b.InvalidationReason = "", ""
	return a == b
}

func (s *Store) validateBypassReplayTraceLocked(idKey string, revision core.Revision, trace contract.TraceFactV1) error {
	stored, hasStored := s.bypassTraceByRevision[idKey][revision]
	hasTrace := trace.ID != ""
	if hasStored != hasTrace {
		return core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "bypass Decision replay changed optional Trace presence")
	}
	if !hasTrace {
		return nil
	}
	if stored.TenantID != trace.TenantID || stored.ID != trace.ID || stored.Revision != trace.Revision || stored.Digest != trace.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "bypass Decision replay changed exact Trace")
	}
	actual, ok := s.traces[key(trace.TenantID, trace.ID)]
	if !ok || actual.Revision != trace.Revision || actual.Digest != trace.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "bypass Decision stored Trace binding drifted")
	}
	return nil
}

func (s *Store) bindBypassTraceLocked(idKey string, revision core.Revision, trace contract.TraceFactV1) {
	if trace.ID == "" {
		return
	}
	if s.bypassTraceByRevision[idKey] == nil {
		s.bypassTraceByRevision[idKey] = make(map[core.Revision]contract.FactIdentityV1)
	}
	s.bypassTraceByRevision[idKey][revision] = trace.FactIdentityV1
}

func (s *Store) CreateBypassDecisionV1(ctx context.Context, m reviewport.CreateBypassDecisionMutationV1) (contract.BypassDecisionV1, error) {
	if err := checkContext(ctx); err != nil {
		return contract.BypassDecisionV1{}, err
	}
	if err := m.Decision.Validate(); err != nil {
		return contract.BypassDecisionV1{}, err
	}
	if m.Decision.Revision != 1 || m.Decision.State != contract.BypassDecisionActiveV1 {
		return contract.BypassDecisionV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidTransition, "bypass Decision create must publish active revision one")
	}
	if err := validateBypassTrace(m.Decision, m.Trace); err != nil {
		return contract.BypassDecisionV1{}, err
	}
	decisionCopy, err := clone(m.Decision)
	if err != nil {
		return contract.BypassDecisionV1{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	idKey := key(m.Decision.TenantID, m.Decision.ID)
	caseKey := bypassCaseKey(m.Decision.Case)
	if history := s.bypassDecisionHistory[idKey]; history != nil {
		if existing, ok := history[m.Decision.Revision]; ok {
			if existing.Digest != m.Decision.Digest {
				return contract.BypassDecisionV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "bypass Decision create replay changed content")
			}
			if err := s.validateBypassReplayTraceLocked(idKey, m.Decision.Revision, m.Trace); err != nil {
				return contract.BypassDecisionV1{}, err
			}
			return clone(existing)
		}
		return contract.BypassDecisionV1{}, exists("bypass Decision")
	}
	if _, ok := s.bypassDecisions[idKey]; ok || s.bypassHighestRevision[idKey] != 0 {
		return contract.BypassDecisionV1{}, exists("bypass Decision")
	}
	if existing, ok := s.bypassDecisionByCase[caseKey]; ok {
		if sameBypassRef(existing, m.Decision.ExactRef()) {
			return contract.BypassDecisionV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "bypass Decision Case index exists without exact history")
		}
		return contract.BypassDecisionV1{}, core.NewError(core.ErrorConflict, core.ReasonAlreadyExists, "exact bypass Case already has a Decision lifecycle")
	}
	if err := s.validateTraceInsertLocked(m.Trace); err != nil {
		return contract.BypassDecisionV1{}, err
	}

	s.bypassDecisionHistory[idKey] = map[core.Revision]contract.BypassDecisionV1{m.Decision.Revision: decisionCopy}
	s.bypassDecisions[idKey] = decisionCopy
	s.bypassDecisionByCase[caseKey] = m.Decision.ExactRef()
	s.bypassHighestRevision[idKey] = m.Decision.Revision
	if m.Trace.ID != "" {
		_, _ = s.appendTraceLocked(m.Trace)
		s.bindBypassTraceLocked(idKey, m.Decision.Revision, m.Trace)
	}
	return clone(decisionCopy)
}

func (s *Store) InspectBypassDecisionExactV1(ctx context.Context, ref contract.BypassDecisionExactRefV1) (contract.BypassDecisionV1, error) {
	if err := checkContext(ctx); err != nil {
		return contract.BypassDecisionV1{}, err
	}
	if err := ref.Validate(); err != nil {
		return contract.BypassDecisionV1{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.bypassDecisionHistory[key(ref.TenantID, ref.ID)][ref.Revision]
	if !ok {
		return contract.BypassDecisionV1{}, notFound("bypass Decision")
	}
	if !sameBypassRef(value.ExactRef(), ref) {
		return contract.BypassDecisionV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "bypass Decision exact ref drifted")
	}
	return clone(value)
}

func (s *Store) InspectCurrentBypassDecisionByCaseV1(ctx context.Context, caseRef contract.BypassCaseExactRefV1) (contract.BypassDecisionV1, error) {
	if err := checkContext(ctx); err != nil {
		return contract.BypassDecisionV1{}, err
	}
	if err := caseRef.Validate(); err != nil {
		return contract.BypassDecisionV1{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	ref, ok := s.bypassDecisionByCase[bypassCaseKey(caseRef)]
	if !ok {
		return contract.BypassDecisionV1{}, notFound("current bypass Decision")
	}
	value, ok := s.bypassDecisions[key(ref.TenantID, ref.ID)]
	if !ok || !sameBypassRef(value.ExactRef(), ref) || value.Case != caseRef || s.bypassHighestRevision[key(ref.TenantID, ref.ID)] != ref.Revision {
		return contract.BypassDecisionV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "bypass Decision current Case index drifted")
	}
	return clone(value)
}

func (s *Store) CompareAndSwapBypassDecisionV1(ctx context.Context, m reviewport.BypassDecisionCASMutationV1) (contract.BypassDecisionV1, error) {
	if err := checkContext(ctx); err != nil {
		return contract.BypassDecisionV1{}, err
	}
	if err := m.Expected.Validate(); err != nil {
		return contract.BypassDecisionV1{}, err
	}
	if err := m.Next.Validate(); err != nil {
		return contract.BypassDecisionV1{}, err
	}
	if m.Next.TenantID != m.Expected.TenantID || m.Next.ID != m.Expected.ID || m.Next.Revision != m.Expected.Revision+1 {
		return contract.BypassDecisionV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidTransition, "bypass Decision CAS identity or revision drifted")
	}
	if err := validateBypassTrace(m.Next, m.Trace); err != nil {
		return contract.BypassDecisionV1{}, err
	}
	nextCopy, err := clone(m.Next)
	if err != nil {
		return contract.BypassDecisionV1{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	idKey := key(m.Expected.TenantID, m.Expected.ID)
	if history := s.bypassDecisionHistory[idKey]; history != nil {
		if existing, ok := history[m.Next.Revision]; ok {
			if existing.Digest != m.Next.Digest {
				return contract.BypassDecisionV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "bypass Decision CAS replay changed content")
			}
			previous, ok := history[m.Expected.Revision]
			if !ok || !sameBypassRef(previous.ExactRef(), m.Expected) || !sameBypassLifecycle(previous, existing) {
				return contract.BypassDecisionV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "bypass Decision CAS replay changed expected lifecycle")
			}
			if err := s.validateBypassReplayTraceLocked(idKey, m.Next.Revision, m.Trace); err != nil {
				return contract.BypassDecisionV1{}, err
			}
			return clone(existing)
		}
	}
	current, ok := s.bypassDecisions[idKey]
	if !ok {
		return contract.BypassDecisionV1{}, notFound("bypass Decision")
	}
	if !sameBypassRef(current.ExactRef(), m.Expected) || s.bypassHighestRevision[idKey] != m.Expected.Revision {
		return contract.BypassDecisionV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "bypass Decision CAS expected ref is stale")
	}
	if m.Next.TenantID != current.TenantID || m.Next.ID != current.ID || m.Next.Revision != current.Revision+1 || m.Next.CreatedUnixNano != current.CreatedUnixNano || m.Next.UpdatedUnixNano <= current.UpdatedUnixNano || !sameBypassLifecycle(current, m.Next) {
		return contract.BypassDecisionV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidTransition, "bypass Decision CAS changed immutable lifecycle fields or revision")
	}
	if current.State != contract.BypassDecisionActiveV1 || m.Next.State == contract.BypassDecisionActiveV1 {
		return contract.BypassDecisionV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidTransition, "bypass Decision CAS must terminate an active lifecycle exactly once")
	}
	caseKey := bypassCaseKey(current.Case)
	indexed, ok := s.bypassDecisionByCase[caseKey]
	if !ok || !sameBypassRef(indexed, current.ExactRef()) {
		return contract.BypassDecisionV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "bypass Decision current Case index is stale")
	}
	if err := s.validateTraceInsertLocked(m.Trace); err != nil {
		return contract.BypassDecisionV1{}, err
	}

	appendHistory(s.bypassDecisionHistory, idKey, nextCopy.Revision, nextCopy)
	s.bypassDecisions[idKey] = nextCopy
	s.bypassDecisionByCase[caseKey] = nextCopy.ExactRef()
	s.bypassHighestRevision[idKey] = nextCopy.Revision
	if m.Trace.ID != "" {
		_, _ = s.appendTraceLocked(m.Trace)
		s.bindBypassTraceLocked(idKey, nextCopy.Revision, m.Trace)
	}
	return clone(nextCopy)
}
