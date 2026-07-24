package memory

import (
	"context"
	"reflect"
	"sort"
	"strconv"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func humanRefKey(tenant core.TenantID, id string, revision core.Revision, digest core.Digest) string {
	return key(tenant, id) + "\x00" + strconv.FormatUint(uint64(revision), 10) + "\x00" + string(digest)
}

func humanReviewerVoteKey(a contract.HumanAttestationV2) string {
	return key(a.TenantID, a.Panel.ID) + "\x00" + string(a.ReviewerIdentity.TenantID) + "\x00" + a.ReviewerIdentity.Ref
}

func samePanelRef(a, b contract.HumanPanelExactRefV2) bool {
	return a.TenantID == b.TenantID && a.ID == b.ID && a.Revision == b.Revision && a.Digest == b.Digest
}
func sameAssignmentRef(a, b contract.HumanPanelAssignmentExactRefV2) bool {
	return a.TenantID == b.TenantID && a.ID == b.ID && a.Revision == b.Revision && a.Digest == b.Digest
}
func sameCaseRefV2(a, b contract.HumanCaseExactRefV2) bool {
	return a.TenantID == b.TenantID && a.ID == b.ID && a.Revision == b.Revision && a.Digest == b.Digest
}
func sameTargetRefV2(a, b contract.HumanTargetExactRefV2) bool {
	return a.TenantID == b.TenantID && a.ID == b.ID && a.Revision == b.Revision && a.Digest == b.Digest
}
func sameRoundRefV2(a, b contract.HumanRoundExactRefV2) bool {
	return a.TenantID == b.TenantID && a.ID == b.ID && a.Revision == b.Revision && a.Digest == b.Digest
}

func panelRefFromFact(v contract.HumanReviewPanelV2) contract.HumanPanelExactRefV2 {
	return v.ExactRef()
}

func validateOptionalTrace(t contract.TraceFactV1) error {
	if t.ID == "" {
		return nil
	}
	return t.Validate()
}

func (s *Store) validateTraceInsertLocked(t contract.TraceFactV1) error {
	if t.ID == "" {
		return nil
	}
	if old, ok := s.traces[key(t.TenantID, t.ID)]; ok {
		if old.Digest == t.Digest {
			return nil
		}
		return exists("review trace")
	}
	for _, id := range s.traceByCase[key(t.TenantID, t.CaseID)] {
		old := s.traces[key(t.TenantID, id)]
		if old.SourceID == t.SourceID && old.SourceEpoch == t.SourceEpoch && old.SourceSequence == t.SourceSequence && old.Digest != t.Digest {
			return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "review trace source sequence changed content")
		}
	}
	return nil
}

func (s *Store) CreateHumanPanelV2(ctx context.Context, m reviewport.CreateHumanPanelMutationV2) (reviewport.CreateHumanPanelResultV2, error) {
	if err := checkContext(ctx); err != nil {
		return reviewport.CreateHumanPanelResultV2{}, err
	}
	if err := m.ExpectedCase.Validate(); err != nil {
		return reviewport.CreateHumanPanelResultV2{}, err
	}
	if err := m.ProposedPanel.Validate(); err != nil {
		return reviewport.CreateHumanPanelResultV2{}, err
	}
	if err := m.OpenPanel.Validate(); err != nil {
		return reviewport.CreateHumanPanelResultV2{}, err
	}
	if err := reviewport.ValidateCreateHumanPanelTraceV2(m); err != nil {
		return reviewport.CreateHumanPanelResultV2{}, err
	}
	if m.ProposedPanel.State != contract.HumanPanelProposedV2 || m.OpenPanel.State != contract.HumanPanelOpenV2 || m.OpenPanel.ID != m.ProposedPanel.ID || m.OpenPanel.TenantID != m.ProposedPanel.TenantID || m.OpenPanel.Revision != m.ProposedPanel.Revision+1 || !sameCaseRefV2(m.ProposedPanel.Case, m.ExpectedCase) || !sameCaseRefV2(m.OpenPanel.Case, m.ExpectedCase) || !sameTargetRefV2(m.OpenPanel.Target, m.ProposedPanel.Target) || !sameRoundRefV2(m.OpenPanel.Round, m.ProposedPanel.Round) || m.OpenPanel.QuorumPolicy != m.ProposedPanel.QuorumPolicy || m.OpenPanel.ResponsibilitySubject != m.ProposedPanel.ResponsibilitySubject || len(m.Assignments) != int(m.OpenPanel.MaximumPanelSize) {
		return reviewport.CreateHumanPanelResultV2{}, core.NewError(core.ErrorConflict, core.ReasonInvalidTransition, "human Panel proposed/open compound drifted")
	}
	assignments := append([]contract.HumanPanelAssignmentV2(nil), m.Assignments...)
	sort.Slice(assignments, func(i, j int) bool { return assignments[i].ID < assignments[j].ID })
	reviewerIdentities := make(map[string]bool, len(assignments))
	for i, a := range assignments {
		if err := a.Validate(); err != nil {
			return reviewport.CreateHumanPanelResultV2{}, err
		}
		if a.State != contract.HumanAssignmentOfferedV2 {
			return reviewport.CreateHumanPanelResultV2{}, core.NewError(core.ErrorConflict, core.ReasonInvalidTransition, "human Panel creation requires offered Assignments")
		}
		if !samePanelRef(a.Panel, m.ProposedPanel.ExactRef()) || !sameCaseRefV2(a.Case, m.ProposedPanel.Case) || !sameRoundRefV2(a.Round, m.ProposedPanel.Round) || !sameTargetRefV2(a.Target, m.ProposedPanel.Target) || !sameAssignmentRef(m.OpenPanel.AssignmentRefs[i], a.ExactRef()) {
			return reviewport.CreateHumanPanelResultV2{}, core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "human Assignment does not bind proposed Panel/open assignment set")
		}
		if i > 0 && assignments[i-1].ID == a.ID {
			return reviewport.CreateHumanPanelResultV2{}, core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "human Assignment ID duplicated")
		}
		identityKey := string(a.ReviewerIdentity.TenantID) + "\x00" + a.ReviewerIdentity.Ref
		if reviewerIdentities[identityKey] {
			return reviewport.CreateHumanPanelResultV2{}, core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "one human identity cannot hold multiple Panel assignments")
		}
		reviewerIdentities[identityKey] = true
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	pk := key(m.OpenPanel.TenantID, m.OpenPanel.ID)
	if existing, ok := s.humanPanels[pk]; ok {
		if existing.Digest != m.OpenPanel.Digest {
			return reviewport.CreateHumanPanelResultV2{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "human Panel create replay changed content")
		}
		out := reviewport.CreateHumanPanelResultV2{Panel: existing.Clone(), Assignments: make([]contract.HumanPanelAssignmentV2, 0, len(assignments))}
		for _, a := range assignments {
			stored, ok := s.humanAssignmentHistory[key(a.TenantID, a.ID)][a.Revision]
			if !ok || stored.Digest != a.Digest {
				return reviewport.CreateHumanPanelResultV2{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "human Panel replay Assignment set drifted")
			}
			out.Assignments = append(out.Assignments, stored.Clone())
		}
		if err := s.inspectCommittedTraceBatchLockedV2([]contract.TraceFactV1{m.Trace}); err != nil {
			return reviewport.CreateHumanPanelResultV2{}, err
		}
		return out, nil
	}
	caseCurrent, ok := s.cases[key(m.ExpectedCase.TenantID, m.ExpectedCase.ID)]
	if !ok {
		return reviewport.CreateHumanPanelResultV2{}, notFound("review case")
	}
	if caseCurrent.Revision != m.ExpectedCase.Revision || caseCurrent.Digest != m.ExpectedCase.Digest {
		return reviewport.CreateHumanPanelResultV2{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "human Panel Case current drifted")
	}
	target, ok := s.targetHistory[key(m.ProposedPanel.Target.TenantID, m.ProposedPanel.Target.ID)][m.ProposedPanel.Target.Revision]
	if !ok || target.Digest != m.ProposedPanel.Target.Digest || target.ID != caseCurrent.TargetID || target.Revision != caseCurrent.TargetRevision || target.Digest != caseCurrent.TargetDigest {
		return reviewport.CreateHumanPanelResultV2{}, core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "human Panel Target exact fact drifted")
	}
	round, ok := s.rounds[key(m.ProposedPanel.Round.TenantID, m.ProposedPanel.Round.ID)]
	if !ok || round.Revision != m.ProposedPanel.Round.Revision || round.Digest != m.ProposedPanel.Round.Digest || round.CaseID != caseCurrent.ID || round.TargetID != target.ID || round.TargetRevision != target.Revision || round.TargetDigest != target.Digest {
		return reviewport.CreateHumanPanelResultV2{}, core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "human Panel Round exact fact drifted")
	}
	for _, a := range assignments {
		if _, found := s.humanAssignments[key(a.TenantID, a.ID)]; found {
			return reviewport.CreateHumanPanelResultV2{}, exists("human Assignment")
		}
	}
	traceBatch, err := s.stageTraceBatchLockedV2([]contract.TraceFactV1{m.Trace})
	if err != nil {
		return reviewport.CreateHumanPanelResultV2{}, err
	}

	proposed := m.ProposedPanel.Clone()
	open := m.OpenPanel.Clone()
	appendHistory(s.humanPanelHistory, pk, proposed.Revision, proposed)
	appendHistory(s.humanPanelHistory, pk, open.Revision, open)
	s.humanPanels[pk] = open
	for _, a := range assignments {
		copyValue := a.Clone()
		ak := key(a.TenantID, a.ID)
		s.humanAssignments[ak] = copyValue
		appendHistory(s.humanAssignmentHistory, ak, a.Revision, copyValue)
	}
	s.commitTraceBatchLockedV2(traceBatch)
	return reviewport.CreateHumanPanelResultV2{Panel: open.Clone(), Assignments: append([]contract.HumanPanelAssignmentV2(nil), assignments...)}, nil
}

func (s *Store) RecordHumanAttestationV2(ctx context.Context, m reviewport.RecordHumanAttestationMutationV2) (reviewport.RecordHumanAttestationResultV2, error) {
	if err := checkContext(ctx); err != nil {
		return reviewport.RecordHumanAttestationResultV2{}, err
	}
	if err := m.ExpectedPanel.Validate(); err != nil {
		return reviewport.RecordHumanAttestationResultV2{}, err
	}
	if err := m.Attestation.Validate(); err != nil {
		return reviewport.RecordHumanAttestationResultV2{}, err
	}
	if err := m.NextPanel.Validate(); err != nil {
		return reviewport.RecordHumanAttestationResultV2{}, err
	}
	if err := reviewport.ValidateRecordHumanAttestationTracesV2(m); err != nil {
		return reviewport.RecordHumanAttestationResultV2{}, err
	}
	if m.Quorum != nil {
		if err := m.Quorum.Validate(); err != nil {
			return reviewport.RecordHumanAttestationResultV2{}, err
		}
	}
	if (m.ExpectedCase == nil) != (m.NextCase == nil) {
		return reviewport.RecordHumanAttestationResultV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "human vote Case transition must be all-or-none")
	}
	if m.ExpectedCase != nil {
		if err := m.ExpectedCase.Validate(); err != nil {
			return reviewport.RecordHumanAttestationResultV2{}, err
		}
		if err := m.NextCase.Validate(); err != nil {
			return reviewport.RecordHumanAttestationResultV2{}, err
		}
	}
	if !samePanelRef(m.Attestation.Panel, m.ExpectedPanel) || m.NextPanel.TenantID != m.ExpectedPanel.TenantID || m.NextPanel.ID != m.ExpectedPanel.ID || m.NextPanel.Revision != m.ExpectedPanel.Revision+1 || m.NextPanel.AssignmentRefs == nil {
		return reviewport.RecordHumanAttestationResultV2{}, core.NewError(core.ErrorConflict, core.ReasonInvalidTransition, "human vote Panel revision drifted")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	idem := key(m.Attestation.TenantID, m.Attestation.IdempotencyKey)
	if existingID, ok := s.humanAttestationKeys[idem]; ok {
		existing := s.humanAttestations[key(m.Attestation.TenantID, existingID)]
		if existing.Digest != m.Attestation.Digest {
			return reviewport.RecordHumanAttestationResultV2{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "human Attestation idempotency replay changed content")
		}
		panel, ok := s.humanPanelHistory[key(m.NextPanel.TenantID, m.NextPanel.ID)][m.NextPanel.Revision]
		if !ok || panel.Digest != m.NextPanel.Digest {
			return reviewport.RecordHumanAttestationResultV2{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "human vote replay next Panel drifted")
		}
		out := reviewport.RecordHumanAttestationResultV2{Panel: panel.Clone(), Attestation: existing.Clone()}
		if m.Quorum != nil {
			q, ok := s.humanQuorums[key(m.Quorum.TenantID, m.Quorum.ID)]
			if !ok || q.Digest != m.Quorum.Digest {
				return reviewport.RecordHumanAttestationResultV2{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "human vote replay Quorum drifted")
			}
			x := q.Clone()
			out.Quorum = &x
		}
		if m.NextCase != nil {
			c, ok := s.caseHistory[key(m.NextCase.TenantID, m.NextCase.ID)][m.NextCase.Revision]
			if !ok || c.Digest != m.NextCase.Digest {
				return reviewport.RecordHumanAttestationResultV2{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "human vote replay Case drifted")
			}
			x, _ := clone(c)
			out.Case = &x
		}
		if err := s.inspectCommittedTraceBatchLockedV2(append([]contract.TraceFactV1{m.Trace}, m.AdditionalTraces...)); err != nil {
			return reviewport.RecordHumanAttestationResultV2{}, err
		}
		return out, nil
	}
	pk := key(m.ExpectedPanel.TenantID, m.ExpectedPanel.ID)
	current, ok := s.humanPanels[pk]
	if !ok {
		return reviewport.RecordHumanAttestationResultV2{}, notFound("human Panel")
	}
	if current.Revision != m.ExpectedPanel.Revision || current.Digest != m.ExpectedPanel.Digest {
		return reviewport.RecordHumanAttestationResultV2{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "human vote expected Panel is stale")
	}
	if current.State != contract.HumanPanelOpenV2 || m.NextPanel.ID != current.ID || m.NextPanel.Revision != current.Revision+1 || !sameCaseRefV2(m.NextPanel.Case, current.Case) || !sameTargetRefV2(m.NextPanel.Target, current.Target) || !sameRoundRefV2(m.NextPanel.Round, current.Round) || m.NextPanel.QuorumPolicy != current.QuorumPolicy || len(m.NextPanel.AssignmentRefs) != len(current.AssignmentRefs) {
		return reviewport.RecordHumanAttestationResultV2{}, core.NewError(core.ErrorConflict, core.ReasonInvalidTransition, "human vote does not advance the open Panel canonically")
	}
	for i := range current.AssignmentRefs {
		if !sameAssignmentRef(current.AssignmentRefs[i], m.NextPanel.AssignmentRefs[i]) {
			return reviewport.RecordHumanAttestationResultV2{}, core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "human vote changed Panel Assignment set")
		}
	}
	assignment, ok := s.humanAssignments[key(m.Attestation.Assignment.TenantID, m.Attestation.Assignment.ID)]
	if !ok {
		return reviewport.RecordHumanAttestationResultV2{}, notFound("human Assignment")
	}
	if assignment.Revision != m.Attestation.Assignment.Revision || assignment.Digest != m.Attestation.Assignment.Digest || !sameCaseRefV2(assignment.Case, current.Case) || !sameRoundRefV2(assignment.Round, current.Round) || !sameTargetRefV2(assignment.Target, current.Target) || assignment.ReviewerIdentity != m.Attestation.ReviewerIdentity || assignment.ReviewerAuthority != m.Attestation.ReviewerAuthority || assignment.ReviewerBinding != m.Attestation.ReviewerBinding || assignment.Delegated != (m.Attestation.Delegation != nil) {
		return reviewport.RecordHumanAttestationResultV2{}, core.NewError(core.ErrorConflict, core.ReasonStaleLeaseRevision, "human Attestation Assignment/reviewer exact binding drifted")
	}
	if m.Attestation.Policy != current.QuorumPolicy || m.Attestation.ResponsibilitySubject != current.ResponsibilitySubject || !sameCaseRefV2(m.Attestation.Case, current.Case) || !sameRoundRefV2(m.Attestation.Round, current.Round) || !sameTargetRefV2(m.Attestation.Target, current.Target) {
		return reviewport.RecordHumanAttestationResultV2{}, core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "human Attestation Panel provenance drifted")
	}
	target, ok := s.targetHistory[key(m.Attestation.Target.TenantID, m.Attestation.Target.ID)][m.Attestation.Target.Revision]
	if !ok || target.Digest != m.Attestation.Target.Digest {
		return reviewport.RecordHumanAttestationResultV2{}, core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "human Attestation Target exact fact drifted")
	}
	for _, condition := range m.Attestation.Conditions {
		if condition.ScopeDigest != target.ActionScopeDigest {
			return reviewport.RecordHumanAttestationResultV2{}, core.NewError(core.ErrorConflict, core.ReasonReviewConditionUnsatisfied, "human condition Scope drifted from exact Target")
		}
	}
	voteKey := humanReviewerVoteKey(m.Attestation)
	if previous, ok := s.humanVoteByReviewer[voteKey]; ok && previous != m.Attestation.ID {
		return reviewport.RecordHumanAttestationResultV2{}, core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "one human identity cannot cast multiple Panel votes")
	}
	if _, found := s.humanAttestations[key(m.Attestation.TenantID, m.Attestation.ID)]; found {
		return reviewport.RecordHumanAttestationResultV2{}, exists("human Attestation")
	}
	if m.Quorum != nil {
		if !samePanelRef(m.Quorum.Panel, m.NextPanel.ExactRef()) || m.Quorum.Policy != current.QuorumPolicy {
			return reviewport.RecordHumanAttestationResultV2{}, core.NewError(core.ErrorConflict, core.ReasonReviewVerdictStale, "human Quorum does not bind next Panel/policy")
		}
		if _, found := s.humanQuorums[key(m.Quorum.TenantID, m.Quorum.ID)]; found {
			return reviewport.RecordHumanAttestationResultV2{}, exists("human Quorum")
		}
		for _, ref := range append(append([]contract.HumanAttestationExactRefV2(nil), m.Quorum.AcceptedAttestationRefs...), m.Quorum.OtherAttestationRefs...) {
			if ref.ID == m.Attestation.ID && ref.Revision == m.Attestation.Revision && ref.Digest == m.Attestation.Digest {
				continue
			}
			stored, ok := s.humanAttestations[key(ref.TenantID, ref.ID)]
			if !ok || stored.Revision != ref.Revision || stored.Digest != ref.Digest {
				return reviewport.RecordHumanAttestationResultV2{}, core.NewError(core.ErrorConflict, core.ReasonReviewVerdictStale, "human Quorum Attestation set is not exact")
			}
		}
		all := make([]contract.HumanAttestationV2, 0, len(m.Quorum.AcceptedAttestationRefs)+len(m.Quorum.OtherAttestationRefs))
		for _, ref := range append(append([]contract.HumanAttestationExactRefV2(nil), m.Quorum.AcceptedAttestationRefs...), m.Quorum.OtherAttestationRefs...) {
			if ref == m.Attestation.ExactRef() {
				all = append(all, m.Attestation.Clone())
				continue
			}
			all = append(all, s.humanAttestations[key(ref.TenantID, ref.ID)].Clone())
		}
		conditions, digest, err := contract.CanonicalAcceptedConditionsV2(all, m.Quorum.AcceptedAttestationRefs)
		if err != nil {
			return reviewport.RecordHumanAttestationResultV2{}, err
		}
		if digest != m.Quorum.ConditionsDigest || !reflect.DeepEqual(conditions, m.Quorum.Conditions) {
			return reviewport.RecordHumanAttestationResultV2{}, core.NewError(core.ErrorConflict, core.ReasonReviewConditionUnsatisfied, "human Quorum condition union drifted")
		}
	}
	var nextCase *contract.ReviewCaseV1
	if m.ExpectedCase != nil {
		cc, ok := s.cases[key(m.ExpectedCase.TenantID, m.ExpectedCase.ID)]
		if !ok {
			return reviewport.RecordHumanAttestationResultV2{}, notFound("review case")
		}
		if cc.Revision != m.ExpectedCase.Revision || cc.Digest != m.ExpectedCase.Digest || current.Case.TenantID != m.ExpectedCase.TenantID || current.Case.ID != m.ExpectedCase.ID {
			return reviewport.RecordHumanAttestationResultV2{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "human vote Case current drifted")
		}
		if m.NextCase.ID != cc.ID || m.NextCase.TenantID != cc.TenantID || m.NextCase.Revision != cc.Revision+1 || !contract.CanTransitionCaseV1(cc.State, m.NextCase.State) {
			return reviewport.RecordHumanAttestationResultV2{}, core.NewError(core.ErrorConflict, core.ReasonInvalidTransition, "human vote Case transition is invalid")
		}
		x, _ := clone(*m.NextCase)
		nextCase = &x
	}
	traceBatch, err := s.stageTraceBatchLockedV2(append([]contract.TraceFactV1{m.Trace}, m.AdditionalTraces...))
	if err != nil {
		return reviewport.RecordHumanAttestationResultV2{}, err
	}
	att := m.Attestation.Clone()
	next := m.NextPanel.Clone()
	s.humanAttestations[key(att.TenantID, att.ID)] = att
	s.humanAttestationKeys[idem] = att.ID
	s.humanVoteByReviewer[voteKey] = att.ID
	s.humanPanels[pk] = next
	appendHistory(s.humanPanelHistory, pk, next.Revision, next)
	var quorumOut *contract.HumanQuorumDecisionV2
	if m.Quorum != nil {
		q := m.Quorum.Clone()
		s.humanQuorums[key(q.TenantID, q.ID)] = q
		s.humanQuorumByPanel[key(q.TenantID, q.Panel.ID)] = q.ID
		quorumOut = &q
	}
	if nextCase != nil {
		ck := key(nextCase.TenantID, nextCase.ID)
		s.cases[ck] = *nextCase
		appendHistory(s.caseHistory, ck, nextCase.Revision, *nextCase)
	}
	s.commitTraceBatchLockedV2(traceBatch)
	return reviewport.RecordHumanAttestationResultV2{Panel: next.Clone(), Attestation: att.Clone(), Quorum: quorumOut, Case: nextCase}, nil
}

func (s *Store) BeginHumanPanelDecisionV2(ctx context.Context, m reviewport.BeginHumanPanelDecisionMutationV2) (contract.HumanReviewPanelV2, contract.ReviewCaseV1, error) {
	if err := checkContext(ctx); err != nil {
		return contract.HumanReviewPanelV2{}, contract.ReviewCaseV1{}, err
	}
	for _, err := range []error{m.ExpectedPanel.Validate(), m.NextPanel.Validate(), m.ExpectedCase.Validate(), m.NextCase.Validate(), m.Trace.Validate()} {
		if err != nil {
			return contract.HumanReviewPanelV2{}, contract.ReviewCaseV1{}, err
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	pk := key(m.ExpectedPanel.TenantID, m.ExpectedPanel.ID)
	p, ok := s.humanPanels[pk]
	if !ok {
		return contract.HumanReviewPanelV2{}, contract.ReviewCaseV1{}, notFound("human Panel")
	}
	if p.Revision == m.NextPanel.Revision && p.Digest == m.NextPanel.Digest {
		c, ok := s.cases[key(m.NextCase.TenantID, m.NextCase.ID)]
		if ok && c.Revision == m.NextCase.Revision && c.Digest == m.NextCase.Digest {
			if err := s.inspectCommittedTraceBatchLockedV2([]contract.TraceFactV1{m.Trace}); err != nil {
				return contract.HumanReviewPanelV2{}, contract.ReviewCaseV1{}, err
			}
			return p.Clone(), c, nil
		}
	}
	if p.Revision != m.ExpectedPanel.Revision || p.Digest != m.ExpectedPanel.Digest {
		return contract.HumanReviewPanelV2{}, contract.ReviewCaseV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "human decision begin Panel is stale")
	}
	if m.NextPanel.ID != p.ID || m.NextPanel.Revision != p.Revision+1 || m.NextPanel.State != contract.HumanPanelDecidingV2 || !contract.CanTransitionHumanPanelV2(p.State, m.NextPanel.State) {
		return contract.HumanReviewPanelV2{}, contract.ReviewCaseV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidTransition, "human decision begin Panel transition is invalid")
	}
	ck := key(m.ExpectedCase.TenantID, m.ExpectedCase.ID)
	c, ok := s.cases[ck]
	if !ok {
		return contract.HumanReviewPanelV2{}, contract.ReviewCaseV1{}, notFound("review case")
	}
	if c.Revision != m.ExpectedCase.Revision || c.Digest != m.ExpectedCase.Digest {
		return contract.HumanReviewPanelV2{}, contract.ReviewCaseV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "human decision begin Case is stale")
	}
	if m.NextCase.ID != c.ID || m.NextCase.Revision != c.Revision+1 || m.NextCase.State != contract.CaseDecidingV1 || !contract.CanTransitionCaseV1(c.State, m.NextCase.State) {
		return contract.HumanReviewPanelV2{}, contract.ReviewCaseV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidTransition, "human decision begin Case transition is invalid")
	}
	quorumID, ok := s.humanQuorumByPanel[key(p.TenantID, p.ID)]
	if !ok {
		return contract.HumanReviewPanelV2{}, contract.ReviewCaseV1{}, notFound("human Quorum")
	}
	if err := reviewport.ValidateBeginHumanPanelDecisionTraceV2(m, quorumID); err != nil {
		return contract.HumanReviewPanelV2{}, contract.ReviewCaseV1{}, err
	}
	traceBatch, err := s.stageTraceBatchLockedV2([]contract.TraceFactV1{m.Trace})
	if err != nil {
		return contract.HumanReviewPanelV2{}, contract.ReviewCaseV1{}, err
	}
	np := m.NextPanel.Clone()
	nc, _ := clone(m.NextCase)
	s.humanPanels[pk] = np
	appendHistory(s.humanPanelHistory, pk, np.Revision, np)
	s.cases[ck] = nc
	appendHistory(s.caseHistory, ck, nc.Revision, nc)
	s.commitTraceBatchLockedV2(traceBatch)
	return np.Clone(), nc, nil
}

func (s *Store) DecideHumanPanelV2(ctx context.Context, m reviewport.DecideHumanPanelMutationV2) (reviewport.DecideHumanPanelResultV2, error) {
	if err := checkContext(ctx); err != nil {
		return reviewport.DecideHumanPanelResultV2{}, err
	}
	for _, err := range []error{m.ExpectedPanel.Validate(), m.ExpectedCase.Validate(), m.Quorum.Validate(), m.Verdict.Validate(), m.NextPanel.Validate(), m.NextCase.Validate(), reviewport.ValidateDecideHumanPanelTracesV2(m)} {
		if err != nil {
			return reviewport.DecideHumanPanelResultV2{}, err
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	vk := key(m.Verdict.TenantID, m.Verdict.ID)
	if old, ok := s.humanVerdicts[vk]; ok {
		if old.Digest != m.Verdict.Digest {
			return reviewport.DecideHumanPanelResultV2{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "human Verdict replay changed content")
		}
		p, ok := s.humanPanelHistory[key(m.NextPanel.TenantID, m.NextPanel.ID)][m.NextPanel.Revision]
		if !ok || p.Digest != m.NextPanel.Digest {
			return reviewport.DecideHumanPanelResultV2{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "human Verdict replay Panel drifted")
		}
		c, ok := s.caseHistory[key(m.NextCase.TenantID, m.NextCase.ID)][m.NextCase.Revision]
		if !ok || c.Digest != m.NextCase.Digest {
			return reviewport.DecideHumanPanelResultV2{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "human Verdict replay Case drifted")
		}
		if err := s.inspectCommittedTraceBatchLockedV2(append([]contract.TraceFactV1{m.Trace}, m.AdditionalTraces...)); err != nil {
			return reviewport.DecideHumanPanelResultV2{}, err
		}
		return reviewport.DecideHumanPanelResultV2{Panel: p.Clone(), Case: c, Verdict: old.Clone()}, nil
	}
	pk := key(m.ExpectedPanel.TenantID, m.ExpectedPanel.ID)
	current, ok := s.humanPanels[pk]
	if !ok {
		return reviewport.DecideHumanPanelResultV2{}, notFound("human Panel")
	}
	if current.Revision != m.ExpectedPanel.Revision || current.Digest != m.ExpectedPanel.Digest {
		return reviewport.DecideHumanPanelResultV2{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "human Verdict expected Panel is stale")
	}
	q, ok := s.humanQuorums[key(m.Quorum.TenantID, m.Quorum.ID)]
	if !ok {
		return reviewport.DecideHumanPanelResultV2{}, notFound("human Quorum")
	}
	if q.Revision != m.Quorum.Revision || q.Digest != m.Quorum.Digest {
		return reviewport.DecideHumanPanelResultV2{}, core.NewError(core.ErrorConflict, core.ReasonReviewVerdictStale, "human Verdict Quorum exact ref drifted")
	}
	cc, ok := s.cases[key(m.ExpectedCase.TenantID, m.ExpectedCase.ID)]
	if !ok {
		return reviewport.DecideHumanPanelResultV2{}, notFound("review case")
	}
	if cc.Revision != m.ExpectedCase.Revision || cc.Digest != m.ExpectedCase.Digest {
		return reviewport.DecideHumanPanelResultV2{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "human Verdict Case current drifted")
	}
	if !samePanelRef(m.Verdict.Panel, m.ExpectedPanel) || m.Verdict.QuorumDecision != m.Quorum || !sameCaseRefV2(m.Verdict.Case, m.ExpectedCase) || !sameTargetRefV2(m.Verdict.Target, current.Target) || !sameRoundRefV2(m.Verdict.Round, current.Round) || m.Verdict.Policy != current.QuorumPolicy {
		return reviewport.DecideHumanPanelResultV2{}, core.NewError(core.ErrorConflict, core.ReasonReviewVerdictStale, "human Verdict provenance drifted")
	}
	if m.Verdict.ConditionsDigest != q.ConditionsDigest || !reflect.DeepEqual(m.Verdict.Conditions, q.Conditions) {
		return reviewport.DecideHumanPanelResultV2{}, core.NewError(core.ErrorConflict, core.ReasonReviewConditionUnsatisfied, "human Verdict condition set differs from exact Quorum")
	}
	if m.NextPanel.ID != current.ID || m.NextPanel.TenantID != current.TenantID || m.NextPanel.Revision != current.Revision+1 || !contract.CanTransitionHumanPanelV2(current.State, m.NextPanel.State) || (m.NextPanel.State != contract.HumanPanelDecidedV2 && m.NextPanel.State != contract.HumanPanelVetoedV2) {
		return reviewport.DecideHumanPanelResultV2{}, core.NewError(core.ErrorConflict, core.ReasonInvalidTransition, "human Verdict terminal Panel transition is invalid")
	}
	if m.NextCase.ID != cc.ID || m.NextCase.TenantID != cc.TenantID || m.NextCase.Revision != cc.Revision+1 || m.NextCase.State != contract.CaseResolvedV1 || m.NextCase.VerdictID != m.Verdict.ID || m.NextCase.VerdictRevision != m.Verdict.Revision || m.NextCase.VerdictDigest != m.Verdict.Digest || !contract.CanTransitionCaseV1(cc.State, m.NextCase.State) {
		return reviewport.DecideHumanPanelResultV2{}, core.NewError(core.ErrorConflict, core.ReasonInvalidTransition, "human Verdict Case transition is invalid")
	}
	traceBatch, err := s.stageTraceBatchLockedV2(append([]contract.TraceFactV1{m.Trace}, m.AdditionalTraces...))
	if err != nil {
		return reviewport.DecideHumanPanelResultV2{}, err
	}
	v := m.Verdict.Clone()
	p := m.NextPanel.Clone()
	c, _ := clone(m.NextCase)
	s.humanVerdicts[vk] = v
	appendHistory(s.humanVerdictHistory, vk, v.Revision, v)
	s.humanVerdictByPanel[key(v.TenantID, v.Panel.ID)] = v.ID
	s.humanPanels[pk] = p
	appendHistory(s.humanPanelHistory, pk, p.Revision, p)
	ck := key(c.TenantID, c.ID)
	s.cases[ck] = c
	appendHistory(s.caseHistory, ck, c.Revision, c)
	s.commitTraceBatchLockedV2(traceBatch)
	return reviewport.DecideHumanPanelResultV2{Panel: p.Clone(), Case: c, Verdict: v.Clone()}, nil
}

func (s *Store) InspectHumanPanelCurrentV2(ctx context.Context, t core.TenantID, id string) (contract.HumanReviewPanelV2, error) {
	if err := checkContext(ctx); err != nil {
		return contract.HumanReviewPanelV2{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.humanPanels[key(t, id)]
	if !ok {
		return contract.HumanReviewPanelV2{}, notFound("human Panel")
	}
	return v.Clone(), nil
}
func (s *Store) InspectHumanPanelExactV2(ctx context.Context, ref contract.HumanPanelExactRefV2) (contract.HumanReviewPanelV2, error) {
	if err := checkContext(ctx); err != nil {
		return contract.HumanReviewPanelV2{}, err
	}
	if err := ref.Validate(); err != nil {
		return contract.HumanReviewPanelV2{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.humanPanelHistory[key(ref.TenantID, ref.ID)][ref.Revision]
	if !ok {
		return contract.HumanReviewPanelV2{}, notFound("human Panel")
	}
	if v.Digest != ref.Digest {
		return contract.HumanReviewPanelV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "human Panel exact ref drifted")
	}
	return v.Clone(), nil
}
func (s *Store) ListHumanPanelAssignmentsV2(ctx context.Context, ref contract.HumanPanelExactRefV2) ([]contract.HumanPanelAssignmentV2, error) {
	p, err := s.InspectHumanPanelExactV2(ctx, ref)
	if err != nil {
		return nil, err
	}
	out := make([]contract.HumanPanelAssignmentV2, 0, len(p.AssignmentRefs))
	for _, r := range p.AssignmentRefs {
		v, e := s.InspectHumanPanelAssignmentExactV2(ctx, r)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, nil
}
func (s *Store) InspectHumanPanelAssignmentCurrentV2(ctx context.Context, t core.TenantID, id string) (contract.HumanPanelAssignmentV2, error) {
	if err := checkContext(ctx); err != nil {
		return contract.HumanPanelAssignmentV2{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.humanAssignments[key(t, id)]
	if !ok {
		return contract.HumanPanelAssignmentV2{}, notFound("human Assignment")
	}
	return v.Clone(), nil
}
func (s *Store) InspectHumanPanelAssignmentExactV2(ctx context.Context, ref contract.HumanPanelAssignmentExactRefV2) (contract.HumanPanelAssignmentV2, error) {
	if err := checkContext(ctx); err != nil {
		return contract.HumanPanelAssignmentV2{}, err
	}
	if err := ref.Validate(); err != nil {
		return contract.HumanPanelAssignmentV2{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.humanAssignmentHistory[key(ref.TenantID, ref.ID)][ref.Revision]
	if !ok {
		return contract.HumanPanelAssignmentV2{}, notFound("human Assignment")
	}
	if v.Digest != ref.Digest {
		return contract.HumanPanelAssignmentV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "human Assignment exact ref drifted")
	}
	return v.Clone(), nil
}
func (s *Store) InspectHumanAttestationExactV2(ctx context.Context, ref contract.HumanAttestationExactRefV2) (contract.HumanAttestationV2, error) {
	if err := checkContext(ctx); err != nil {
		return contract.HumanAttestationV2{}, err
	}
	if err := ref.Validate(); err != nil {
		return contract.HumanAttestationV2{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.humanAttestations[key(ref.TenantID, ref.ID)]
	if !ok {
		return contract.HumanAttestationV2{}, notFound("human Attestation")
	}
	if v.Revision != ref.Revision || v.Digest != ref.Digest {
		return contract.HumanAttestationV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "human Attestation exact ref drifted")
	}
	return v.Clone(), nil
}
func (s *Store) InspectHumanAttestationByIdempotencyV2(ctx context.Context, t core.TenantID, idem string) (contract.HumanAttestationV2, error) {
	if err := checkContext(ctx); err != nil {
		return contract.HumanAttestationV2{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	id, ok := s.humanAttestationKeys[key(t, idem)]
	if !ok {
		return contract.HumanAttestationV2{}, notFound("human Attestation")
	}
	return s.humanAttestations[key(t, id)].Clone(), nil
}
func (s *Store) ListHumanAttestationsByPanelV2(ctx context.Context, ref contract.HumanPanelExactRefV2) ([]contract.HumanAttestationV2, error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
	}
	if err := ref.Validate(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.humanPanelHistory[key(ref.TenantID, ref.ID)][ref.Revision]; !ok {
		return nil, notFound("human Panel")
	}
	out := make([]contract.HumanAttestationV2, 0)
	for _, v := range s.humanAttestations {
		if v.TenantID == ref.TenantID && v.Panel.ID == ref.ID {
			out = append(out, v.Clone())
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}
func (s *Store) InspectHumanQuorumDecisionExactV2(ctx context.Context, ref contract.HumanQuorumDecisionExactRefV2) (contract.HumanQuorumDecisionV2, error) {
	if err := checkContext(ctx); err != nil {
		return contract.HumanQuorumDecisionV2{}, err
	}
	if err := ref.Validate(); err != nil {
		return contract.HumanQuorumDecisionV2{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.humanQuorums[key(ref.TenantID, ref.ID)]
	if !ok {
		return contract.HumanQuorumDecisionV2{}, notFound("human Quorum")
	}
	if v.Revision != ref.Revision || v.Digest != ref.Digest {
		return contract.HumanQuorumDecisionV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "human Quorum exact ref drifted")
	}
	return v.Clone(), nil
}
func (s *Store) InspectHumanQuorumDecisionByPanelV2(ctx context.Context, ref contract.HumanPanelExactRefV2) (contract.HumanQuorumDecisionV2, error) {
	if err := checkContext(ctx); err != nil {
		return contract.HumanQuorumDecisionV2{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	id, ok := s.humanQuorumByPanel[key(ref.TenantID, ref.ID)]
	if !ok {
		return contract.HumanQuorumDecisionV2{}, notFound("human Quorum")
	}
	v := s.humanQuorums[key(ref.TenantID, id)]
	if !samePanelRef(v.Panel, ref) {
		return contract.HumanQuorumDecisionV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "human Quorum Panel exact ref drifted")
	}
	return v.Clone(), nil
}
func (s *Store) InspectHumanQuorumDecisionCurrentByPanelIDV2(ctx context.Context, t core.TenantID, panelID string) (contract.HumanQuorumDecisionV2, error) {
	if err := checkContext(ctx); err != nil {
		return contract.HumanQuorumDecisionV2{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	id, ok := s.humanQuorumByPanel[key(t, panelID)]
	if !ok {
		return contract.HumanQuorumDecisionV2{}, notFound("human Quorum")
	}
	return s.humanQuorums[key(t, id)].Clone(), nil
}
func (s *Store) InspectHumanVerdictExactV2(ctx context.Context, ref contract.HumanVerdictExactRefV2) (contract.HumanVerdictV2, error) {
	if err := checkContext(ctx); err != nil {
		return contract.HumanVerdictV2{}, err
	}
	if err := ref.Validate(); err != nil {
		return contract.HumanVerdictV2{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.humanVerdictHistory[key(ref.TenantID, ref.ID)][ref.Revision]
	if !ok {
		return contract.HumanVerdictV2{}, notFound("human Verdict")
	}
	if v.Digest != ref.Digest {
		return contract.HumanVerdictV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "human Verdict exact ref drifted")
	}
	return v.Clone(), nil
}
func (s *Store) InspectHumanVerdictByPanelV2(ctx context.Context, ref contract.HumanPanelExactRefV2) (contract.HumanVerdictV2, error) {
	if err := checkContext(ctx); err != nil {
		return contract.HumanVerdictV2{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	id, ok := s.humanVerdictByPanel[key(ref.TenantID, ref.ID)]
	if !ok {
		return contract.HumanVerdictV2{}, notFound("human Verdict")
	}
	v := s.humanVerdicts[key(ref.TenantID, id)]
	if !samePanelRef(v.Panel, ref) {
		return contract.HumanVerdictV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "human Verdict Panel exact ref drifted")
	}
	return v.Clone(), nil
}
func (s *Store) InspectHumanVerdictCurrentByPanelIDV2(ctx context.Context, t core.TenantID, panelID string) (contract.HumanVerdictV2, error) {
	if err := checkContext(ctx); err != nil {
		return contract.HumanVerdictV2{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	id, ok := s.humanVerdictByPanel[key(t, panelID)]
	if !ok {
		return contract.HumanVerdictV2{}, notFound("human Verdict")
	}
	return s.humanVerdicts[key(t, id)].Clone(), nil
}

// keep the compiler checking the exact-key helper until SnapshotV1 uses it for
// corruption diagnostics as well.
var _ = humanRefKey
