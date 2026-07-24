package memory

import (
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

// HumanMultiSignSnapshotV2 is embedded in the existing tenant SnapshotV1 so
// SQLite keeps one generation-CAS state and one database truth.
type HumanMultiSignSnapshotV2 struct {
	Panels                   map[string]contract.HumanReviewPanelV2                       `json:"panels"`
	PanelHistory             map[string]map[core.Revision]contract.HumanReviewPanelV2     `json:"panel_history"`
	Assignments              map[string]contract.HumanPanelAssignmentV2                   `json:"assignments"`
	AssignmentHistory        map[string]map[core.Revision]contract.HumanPanelAssignmentV2 `json:"assignment_history"`
	Attestations             map[string]contract.HumanAttestationV2                       `json:"attestations"`
	AttestationByIdempotency map[string]string                                            `json:"attestation_by_idempotency"`
	VoteByReviewer           map[string]string                                            `json:"vote_by_reviewer"`
	Quorums                  map[string]contract.HumanQuorumDecisionV2                    `json:"quorums"`
	QuorumByPanel            map[string]string                                            `json:"quorum_by_panel"`
	Verdicts                 map[string]contract.HumanVerdictV2                           `json:"verdicts"`
	VerdictHistory           map[string]map[core.Revision]contract.HumanVerdictV2         `json:"verdict_history"`
	VerdictByPanel           map[string]string                                            `json:"verdict_by_panel"`
	ClaimTraceByRevision     map[string]map[core.Revision]contract.FactIdentityV1         `json:"claim_trace_by_revision,omitempty"`
}

func emptyHumanMultiSignSnapshotV2() *HumanMultiSignSnapshotV2 {
	return &HumanMultiSignSnapshotV2{
		Panels: map[string]contract.HumanReviewPanelV2{}, PanelHistory: map[string]map[core.Revision]contract.HumanReviewPanelV2{},
		Assignments: map[string]contract.HumanPanelAssignmentV2{}, AssignmentHistory: map[string]map[core.Revision]contract.HumanPanelAssignmentV2{},
		Attestations: map[string]contract.HumanAttestationV2{}, AttestationByIdempotency: map[string]string{}, VoteByReviewer: map[string]string{},
		Quorums: map[string]contract.HumanQuorumDecisionV2{}, QuorumByPanel: map[string]string{},
		Verdicts: map[string]contract.HumanVerdictV2{}, VerdictHistory: map[string]map[core.Revision]contract.HumanVerdictV2{}, VerdictByPanel: map[string]string{},
		ClaimTraceByRevision: map[string]map[core.Revision]contract.FactIdentityV1{},
	}
}

func (h *HumanMultiSignSnapshotV2) validate(tenant core.TenantID) error {
	if h == nil {
		return nil
	}
	if h.Panels == nil || h.PanelHistory == nil || h.Assignments == nil || h.AssignmentHistory == nil || h.Attestations == nil || h.AttestationByIdempotency == nil || h.VoteByReviewer == nil || h.Quorums == nil || h.QuorumByPanel == nil || h.Verdicts == nil || h.VerdictHistory == nil || h.VerdictByPanel == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "human multisign snapshot maps must be present")
	}
	if err := validateCurrentHistory(tenant, h.Panels, h.PanelHistory, func(v contract.HumanReviewPanelV2) contract.FactIdentityV1 { return v.FactIdentityV1 }, func(v contract.HumanReviewPanelV2) error { return v.Validate() }, "human Panel"); err != nil {
		return err
	}
	if err := validateCurrentHistory(tenant, h.Assignments, h.AssignmentHistory, func(v contract.HumanPanelAssignmentV2) contract.FactIdentityV1 { return v.FactIdentityV1 }, func(v contract.HumanPanelAssignmentV2) error { return v.Validate() }, "human Assignment"); err != nil {
		return err
	}
	for id, a := range h.Attestations {
		if err := validateSnapshotFact(tenant, id, a.FactIdentityV1, a.Validate(), "human Attestation"); err != nil {
			return err
		}
		if h.AttestationByIdempotency[a.IdempotencyKey] != id {
			return core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "human Attestation idempotency index drifted")
		}
		voteKey := a.Panel.ID + "\x00" + string(a.ReviewerIdentity.TenantID) + "\x00" + a.ReviewerIdentity.Ref
		if h.VoteByReviewer[voteKey] != id {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "human reviewer vote index drifted")
		}
	}
	if len(h.AttestationByIdempotency) != len(h.Attestations) || len(h.VoteByReviewer) != len(h.Attestations) {
		return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "human Attestation indexes are incomplete")
	}
	for id, q := range h.Quorums {
		if err := validateSnapshotFact(tenant, id, q.FactIdentityV1, q.Validate(), "human Quorum"); err != nil {
			return err
		}
		if h.QuorumByPanel[q.Panel.ID] != id {
			return core.NewError(core.ErrorConflict, core.ReasonReviewVerdictStale, "human Quorum Panel index drifted")
		}
	}
	if len(h.QuorumByPanel) != len(h.Quorums) {
		return core.NewError(core.ErrorConflict, core.ReasonReviewVerdictStale, "human Quorum Panel index is incomplete")
	}
	if err := validateCurrentHistory(tenant, h.Verdicts, h.VerdictHistory, func(v contract.HumanVerdictV2) contract.FactIdentityV1 { return v.FactIdentityV1 }, func(v contract.HumanVerdictV2) error { return v.Validate() }, "human Verdict"); err != nil {
		return err
	}
	for id, v := range h.Verdicts {
		if h.VerdictByPanel[v.Panel.ID] != id {
			return core.NewError(core.ErrorConflict, core.ReasonReviewVerdictStale, "human Verdict Panel index drifted")
		}
	}
	if len(h.VerdictByPanel) != len(h.Verdicts) {
		return core.NewError(core.ErrorConflict, core.ReasonReviewVerdictStale, "human Verdict Panel index is incomplete")
	}
	for assignmentID, revisions := range h.ClaimTraceByRevision {
		for revision, trace := range revisions {
			assignment, ok := h.AssignmentHistory[assignmentID][revision]
			if !ok || assignment.State != contract.HumanAssignmentClaimedV2 || trace.TenantID != tenant || trace.ID == "" || trace.Revision == 0 || trace.Digest.Validate() != nil {
				return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "human claim Trace index drifted from claimed Assignment history")
			}
		}
	}
	for assignmentID, revisions := range h.AssignmentHistory {
		for revision, assignment := range revisions {
			if assignment.State == contract.HumanAssignmentClaimedV2 {
				if _, ok := h.ClaimTraceByRevision[assignmentID][revision]; !ok {
					return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "claimed Assignment history is missing its exact Trace index")
				}
			}
		}
	}
	return nil
}

func (s *Store) exportHumanMultiSignSnapshotV2(tenant core.TenantID) *HumanMultiSignSnapshotV2 {
	prefix := string(tenant) + "\x00"
	h := emptyHumanMultiSignSnapshotV2()
	count := 0
	copyID := func(k string) string { return strings.TrimPrefix(k, prefix) }
	for k, v := range s.humanPanels {
		if strings.HasPrefix(k, prefix) {
			h.Panels[copyID(k)] = v.Clone()
			count++
		}
	}
	for k, v := range s.humanPanelHistory {
		if strings.HasPrefix(k, prefix) {
			h.PanelHistory[copyID(k)], _ = clone(v)
		}
	}
	for k, v := range s.humanAssignments {
		if strings.HasPrefix(k, prefix) {
			h.Assignments[copyID(k)] = v.Clone()
		}
	}
	for k, v := range s.humanAssignmentHistory {
		if strings.HasPrefix(k, prefix) {
			h.AssignmentHistory[copyID(k)], _ = clone(v)
		}
	}
	for k, v := range s.humanAttestations {
		if strings.HasPrefix(k, prefix) {
			h.Attestations[copyID(k)] = v.Clone()
		}
	}
	for k, v := range s.humanAttestationKeys {
		if strings.HasPrefix(k, prefix) {
			h.AttestationByIdempotency[copyID(k)] = v
		}
	}
	for k, v := range s.humanVoteByReviewer {
		if strings.HasPrefix(k, prefix) {
			h.VoteByReviewer[copyID(k)] = v
		}
	}
	for k, v := range s.humanQuorums {
		if strings.HasPrefix(k, prefix) {
			h.Quorums[copyID(k)] = v.Clone()
		}
	}
	for k, v := range s.humanQuorumByPanel {
		if strings.HasPrefix(k, prefix) {
			h.QuorumByPanel[copyID(k)] = v
		}
	}
	for k, v := range s.humanVerdicts {
		if strings.HasPrefix(k, prefix) {
			h.Verdicts[copyID(k)] = v.Clone()
		}
	}
	for k, v := range s.humanVerdictHistory {
		if strings.HasPrefix(k, prefix) {
			h.VerdictHistory[copyID(k)], _ = clone(v)
		}
	}
	for k, v := range s.humanVerdictByPanel {
		if strings.HasPrefix(k, prefix) {
			h.VerdictByPanel[copyID(k)] = v
		}
	}
	for k, v := range s.humanClaimTraceByRevision {
		if strings.HasPrefix(k, prefix) {
			h.ClaimTraceByRevision[copyID(k)], _ = clone(v)
		}
	}
	if count == 0 {
		return nil
	}
	return h
}

func (s *Store) importHumanMultiSignSnapshotV2(tenant core.TenantID, h *HumanMultiSignSnapshotV2) {
	if h == nil {
		return
	}
	for id, v := range h.Panels {
		s.humanPanels[key(tenant, id)] = v.Clone()
	}
	for id, v := range h.PanelHistory {
		s.humanPanelHistory[key(tenant, id)], _ = clone(v)
	}
	for id, v := range h.Assignments {
		s.humanAssignments[key(tenant, id)] = v.Clone()
	}
	for id, v := range h.AssignmentHistory {
		s.humanAssignmentHistory[key(tenant, id)], _ = clone(v)
	}
	for id, v := range h.Attestations {
		s.humanAttestations[key(tenant, id)] = v.Clone()
	}
	for idem, id := range h.AttestationByIdempotency {
		s.humanAttestationKeys[key(tenant, idem)] = id
	}
	for local, id := range h.VoteByReviewer {
		s.humanVoteByReviewer[key(tenant, local)] = id
	}
	for id, v := range h.Quorums {
		s.humanQuorums[key(tenant, id)] = v.Clone()
	}
	for panel, id := range h.QuorumByPanel {
		s.humanQuorumByPanel[key(tenant, panel)] = id
	}
	for id, v := range h.Verdicts {
		s.humanVerdicts[key(tenant, id)] = v.Clone()
	}
	for id, v := range h.VerdictHistory {
		s.humanVerdictHistory[key(tenant, id)], _ = clone(v)
	}
	for panel, id := range h.VerdictByPanel {
		s.humanVerdictByPanel[key(tenant, panel)] = id
	}
	for assignment, revisions := range h.ClaimTraceByRevision {
		s.humanClaimTraceByRevision[key(tenant, assignment)], _ = clone(revisions)
	}
}
