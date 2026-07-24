package memory

import (
	"context"
	"reflect"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func sameClaimAssignmentLifecycleV2(a, b contract.HumanPanelAssignmentV2) bool {
	a, b = a.Clone(), b.Clone()
	a.Revision, b.Revision = 0, 0
	a.UpdatedUnixNano, b.UpdatedUnixNano = 0, 0
	a.Digest, b.Digest = "", ""
	a.State, b.State = "", ""
	a.LeaseHolder, b.LeaseHolder = "", ""
	a.LeaseExpiresUnixNano, b.LeaseExpiresUnixNano = 0, 0
	return reflect.DeepEqual(a, b)
}

func sameClaimPanelLifecycleV2(a, b contract.HumanReviewPanelV2) bool {
	a, b = a.Clone(), b.Clone()
	a.Revision, b.Revision = 0, 0
	a.UpdatedUnixNano, b.UpdatedUnixNano = 0, 0
	a.Digest, b.Digest = "", ""
	a.AssignmentRefs, b.AssignmentRefs = nil, nil
	return reflect.DeepEqual(a, b)
}

func validateClaimPanelRefsV2(current, next contract.HumanReviewPanelV2, expected contract.HumanPanelAssignmentExactRefV2, replacement contract.HumanPanelAssignmentExactRefV2) error {
	if len(current.AssignmentRefs) != len(next.AssignmentRefs) {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidTransition, "human claim changed the Panel Assignment set size")
	}
	found := 0
	for _, oldRef := range current.AssignmentRefs {
		wanted := oldRef
		if sameAssignmentRef(oldRef, expected) {
			wanted = replacement
			found++
		}
		matched := false
		for _, nextRef := range next.AssignmentRefs {
			if sameAssignmentRef(nextRef, wanted) {
				matched = true
				break
			}
		}
		if !matched {
			return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "human claim did not replace exactly one Panel Assignment ref")
		}
	}
	if found != 1 {
		return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "human claim expected Assignment is absent or duplicated in the Panel")
	}
	return nil
}

func (s *Store) ClaimHumanAssignmentV2(ctx context.Context, m reviewport.ClaimHumanAssignmentMutationV2) (reviewport.ClaimHumanAssignmentResultV2, error) {
	if err := checkContext(ctx); err != nil {
		return reviewport.ClaimHumanAssignmentResultV2{}, err
	}
	for _, err := range []error{m.ExpectedPanel.Validate(), m.ExpectedAssignment.Validate(), m.NextPanel.Validate(), m.NextAssignment.Validate(), reviewport.ValidateClaimHumanAssignmentTraceV2(m)} {
		if err != nil {
			return reviewport.ClaimHumanAssignmentResultV2{}, err
		}
	}
	if m.NextPanel.TenantID != m.ExpectedPanel.TenantID || m.NextPanel.ID != m.ExpectedPanel.ID || m.NextPanel.Revision != m.ExpectedPanel.Revision+1 || m.NextAssignment.TenantID != m.ExpectedAssignment.TenantID || m.NextAssignment.ID != m.ExpectedAssignment.ID || m.NextAssignment.Revision != m.ExpectedAssignment.Revision+1 || m.NextAssignment.State != contract.HumanAssignmentClaimedV2 {
		return reviewport.ClaimHumanAssignmentResultV2{}, core.NewError(core.ErrorConflict, core.ReasonInvalidTransition, "human claim identities, revisions or claimed state drifted")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	pk := key(m.ExpectedPanel.TenantID, m.ExpectedPanel.ID)
	ak := key(m.ExpectedAssignment.TenantID, m.ExpectedAssignment.ID)
	if existing, ok := s.humanAssignmentHistory[ak][m.NextAssignment.Revision]; ok {
		if existing.Digest != m.NextAssignment.Digest {
			return reviewport.ClaimHumanAssignmentResultV2{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "human claim replay changed Assignment content")
		}
		panel, panelOK := s.humanPanelHistory[pk][m.NextPanel.Revision]
		previous, previousOK := s.humanAssignmentHistory[ak][m.ExpectedAssignment.Revision]
		if !panelOK || panel.Digest != m.NextPanel.Digest || !previousOK || !sameAssignmentRef(previous.ExactRef(), m.ExpectedAssignment) || !sameClaimAssignmentLifecycleV2(previous, existing) {
			return reviewport.ClaimHumanAssignmentResultV2{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "human claim replay exact closure drifted")
		}
		traceRef, traceOK := s.humanClaimTraceByRevision[ak][m.NextAssignment.Revision]
		storedTrace, storedTraceOK := s.traces[key(m.Trace.TenantID, m.Trace.ID)]
		if !traceOK || traceRef != m.Trace.FactIdentityV1 || !storedTraceOK || storedTrace.FactIdentityV1 != m.Trace.FactIdentityV1 {
			return reviewport.ClaimHumanAssignmentResultV2{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "human claim replay changed or lost its exact Trace")
		}
		return reviewport.ClaimHumanAssignmentResultV2{Panel: panel.Clone(), Assignment: existing.Clone()}, nil
	}
	panel, ok := s.humanPanels[pk]
	if !ok {
		return reviewport.ClaimHumanAssignmentResultV2{}, notFound("human Panel")
	}
	assignment, ok := s.humanAssignments[ak]
	if !ok {
		return reviewport.ClaimHumanAssignmentResultV2{}, notFound("human Assignment")
	}
	if !samePanelRef(panel.ExactRef(), m.ExpectedPanel) || !sameAssignmentRef(assignment.ExactRef(), m.ExpectedAssignment) {
		return reviewport.ClaimHumanAssignmentResultV2{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "human claim expected Panel or Assignment is stale")
	}
	proposed, proposedOK := s.humanPanelHistory[pk][assignment.Panel.Revision]
	if panel.State != contract.HumanPanelOpenV2 || assignment.State != contract.HumanAssignmentOfferedV2 || !proposedOK || !samePanelRef(assignment.Panel, proposed.ExactRef()) {
		return reviewport.ClaimHumanAssignmentResultV2{}, core.NewError(core.ErrorConflict, core.ReasonInvalidTransition, "human claim requires an open Panel and offered Assignment")
	}
	if !sameClaimPanelLifecycleV2(panel, m.NextPanel) || !sameClaimAssignmentLifecycleV2(assignment, m.NextAssignment) || m.NextPanel.State != panel.State || m.NextAssignment.UpdatedUnixNano <= assignment.UpdatedUnixNano || m.NextPanel.UpdatedUnixNano <= panel.UpdatedUnixNano {
		return reviewport.ClaimHumanAssignmentResultV2{}, core.NewError(core.ErrorConflict, core.ReasonInvalidTransition, "human claim changed immutable fields or timestamps")
	}
	if err := validateClaimPanelRefsV2(panel, m.NextPanel, assignment.ExactRef(), m.NextAssignment.ExactRef()); err != nil {
		return reviewport.ClaimHumanAssignmentResultV2{}, err
	}
	if err := s.validateTraceInsertLocked(m.Trace); err != nil {
		return reviewport.ClaimHumanAssignmentResultV2{}, err
	}

	nextPanel, nextAssignment := m.NextPanel.Clone(), m.NextAssignment.Clone()
	s.humanPanels[pk] = nextPanel
	appendHistory(s.humanPanelHistory, pk, nextPanel.Revision, nextPanel)
	s.humanAssignments[ak] = nextAssignment
	appendHistory(s.humanAssignmentHistory, ak, nextAssignment.Revision, nextAssignment)
	if s.humanClaimTraceByRevision[ak] == nil {
		s.humanClaimTraceByRevision[ak] = make(map[core.Revision]contract.FactIdentityV1)
	}
	s.humanClaimTraceByRevision[ak][nextAssignment.Revision] = m.Trace.FactIdentityV1
	_, _ = s.appendTraceLocked(m.Trace)
	return reviewport.ClaimHumanAssignmentResultV2{Panel: nextPanel.Clone(), Assignment: nextAssignment.Clone()}, nil
}
