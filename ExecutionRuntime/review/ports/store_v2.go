package ports

import (
	"context"
	"slices"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

// ResultBundleStoreV2 is the Review Owner's exact historical V2 result
// boundary. Publication is intentionally absent: a V2 Bundle can only be
// committed by StoreV1.CreateTargetCaseV1 together with its exact Request,
// Target and Case, so callers cannot create an orphan Bundle.
type ResultBundleStoreV2 interface {
	InspectResultBundleExactV2(context.Context, core.TenantID, ExactFactRefV1) (contract.ReviewResultBundleV2, error)
}

// CreateHumanPanelMutationV2 publishes one logical Panel atomically. The
// proposed revision breaks the Panel/Assignment digest cycle: every Assignment
// binds ProposedPanel exactly, while OpenPanel (revision +1) names the complete
// sorted Assignment exact set and becomes current.
type CreateHumanPanelMutationV2 struct {
	ExpectedCase  contract.HumanCaseExactRefV2      `json:"expected_case"`
	ProposedPanel contract.HumanReviewPanelV2       `json:"proposed_panel"`
	Assignments   []contract.HumanPanelAssignmentV2 `json:"assignments"`
	OpenPanel     contract.HumanReviewPanelV2       `json:"open_panel"`
	Trace         contract.TraceFactV1              `json:"trace"`
}

type CreateHumanPanelResultV2 struct {
	Panel       contract.HumanReviewPanelV2       `json:"panel"`
	Assignments []contract.HumanPanelAssignmentV2 `json:"assignments"`
}

// ClaimHumanAssignmentMutationV2 advances one offered Assignment to claimed
// and replaces its exact ref in the current Panel in the same Review-owner
// transaction. The lease is coordination only; it grants no Authority.
type ClaimHumanAssignmentMutationV2 struct {
	ExpectedPanel      contract.HumanPanelExactRefV2           `json:"expected_panel"`
	ExpectedAssignment contract.HumanPanelAssignmentExactRefV2 `json:"expected_assignment"`
	NextPanel          contract.HumanReviewPanelV2             `json:"next_panel"`
	NextAssignment     contract.HumanPanelAssignmentV2         `json:"next_assignment"`
	Trace              contract.TraceFactV1                    `json:"trace"`
}

func ValidateClaimHumanAssignmentTraceV2(m ClaimHumanAssignmentMutationV2) error {
	if err := m.Trace.Validate(); err != nil {
		return err
	}
	wantRefs := []string{m.NextAssignment.ID, m.NextPanel.ID}
	slices.Sort(wantRefs)
	if m.Trace.Event != contract.TraceStartedV1 || m.Trace.TenantID != m.NextAssignment.TenantID || m.Trace.CaseID != m.NextAssignment.Case.ID || m.Trace.CaseRevision != m.NextAssignment.Case.Revision || m.Trace.TargetID != m.NextAssignment.Target.ID || m.Trace.TargetRevision != m.NextAssignment.Target.Revision || m.Trace.TargetDigest != m.NextAssignment.Target.Digest || m.Trace.CreatedUnixNano != m.NextAssignment.UpdatedUnixNano || m.Trace.UpdatedUnixNano != m.NextAssignment.UpdatedUnixNano || m.NextPanel.UpdatedUnixNano != m.NextAssignment.UpdatedUnixNano || m.Trace.CausationID != m.NextAssignment.ID || m.Trace.CorrelationID != m.NextPanel.ID || !slices.Equal(m.Trace.FactRefs, wantRefs) {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "human claim ReviewStarted Trace does not bind the exact successor Panel, Assignment, Case and Target")
	}
	return nil
}

type ClaimHumanAssignmentResultV2 struct {
	Panel      contract.HumanReviewPanelV2     `json:"panel"`
	Assignment contract.HumanPanelAssignmentV2 `json:"assignment"`
}

// RecordHumanAttestationMutationV2 is the sole vote linearization point. A
// QuorumDecision and Case transition are optional, but when supplied they are
// part of the same transaction as the Attestation and next Panel revision.
type RecordHumanAttestationMutationV2 struct {
	ExpectedPanel    contract.HumanPanelExactRefV2   `json:"expected_panel"`
	Attestation      contract.HumanAttestationV2     `json:"attestation"`
	NextPanel        contract.HumanReviewPanelV2     `json:"next_panel"`
	Quorum           *contract.HumanQuorumDecisionV2 `json:"quorum,omitempty"`
	ExpectedCase     *contract.HumanCaseExactRefV2   `json:"expected_case,omitempty"`
	NextCase         *contract.ReviewCaseV1          `json:"next_case,omitempty"`
	Trace            contract.TraceFactV1            `json:"trace"`
	AdditionalTraces []contract.TraceFactV1          `json:"additional_traces,omitempty"`
}

type RecordHumanAttestationResultV2 struct {
	Panel       contract.HumanReviewPanelV2     `json:"panel"`
	Attestation contract.HumanAttestationV2     `json:"attestation"`
	Quorum      *contract.HumanQuorumDecisionV2 `json:"quorum,omitempty"`
	Case        *contract.ReviewCaseV1          `json:"case,omitempty"`
}

// DecideHumanPanelMutationV2 is the only V2 Verdict write boundary. Callers
// cannot write a Verdict independently from the terminal Panel and resolved
// Case revisions.
type DecideHumanPanelMutationV2 struct {
	ExpectedPanel    contract.HumanPanelExactRefV2          `json:"expected_panel"`
	ExpectedCase     contract.HumanCaseExactRefV2           `json:"expected_case"`
	Quorum           contract.HumanQuorumDecisionExactRefV2 `json:"quorum"`
	Verdict          contract.HumanVerdictV2                `json:"verdict"`
	NextPanel        contract.HumanReviewPanelV2            `json:"next_panel"`
	NextCase         contract.ReviewCaseV1                  `json:"next_case"`
	Trace            contract.TraceFactV1                   `json:"trace"`
	AdditionalTraces []contract.TraceFactV1                 `json:"additional_traces"`
}

type BeginHumanPanelDecisionMutationV2 struct {
	ExpectedPanel contract.HumanPanelExactRefV2 `json:"expected_panel"`
	NextPanel     contract.HumanReviewPanelV2   `json:"next_panel"`
	ExpectedCase  contract.HumanCaseExactRefV2  `json:"expected_case"`
	NextCase      contract.ReviewCaseV1         `json:"next_case"`
	Trace         contract.TraceFactV1          `json:"trace"`
}

const HumanDecisionTraceSourceV2 = "praxis.review/human-decision/v2"

func validateHumanMutationTraceV2(trace contract.TraceFactV1, event contract.TraceEventV1, tenant core.TenantID, caseRef contract.HumanCaseExactRefV2, targetRef contract.HumanTargetExactRefV2, updated int64, causation, correlation string, factRefs []string) error {
	if err := trace.Validate(); err != nil {
		return err
	}
	wantRefs := append([]string(nil), factRefs...)
	slices.Sort(wantRefs)
	if trace.Event != event || trace.TenantID != tenant || trace.CaseID != caseRef.ID || trace.CaseRevision != caseRef.Revision || trace.TargetID != targetRef.ID || trace.TargetRevision != targetRef.Revision || trace.TargetDigest != targetRef.Digest || trace.CreatedUnixNano != updated || trace.UpdatedUnixNano != updated || trace.CausationID != causation || trace.CorrelationID != correlation || !slices.Equal(trace.FactRefs, wantRefs) {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "human mutation Trace does not bind its exact event subject")
	}
	return nil
}

func ValidateCreateHumanPanelTraceV2(m CreateHumanPanelMutationV2) error {
	refs := []string{m.OpenPanel.ID}
	for _, assignment := range m.Assignments {
		refs = append(refs, assignment.ID)
	}
	return validateHumanMutationTraceV2(m.Trace, contract.TraceAssignedV1, m.OpenPanel.TenantID, m.OpenPanel.Case, m.OpenPanel.Target, m.OpenPanel.UpdatedUnixNano, m.OpenPanel.ID, m.OpenPanel.Case.ID, refs)
}

func ValidateRecordHumanAttestationTracesV2(m RecordHumanAttestationMutationV2) error {
	refs := []string{m.Attestation.ID}
	if m.Quorum != nil {
		refs = append(refs, m.Quorum.ID)
	}
	if err := validateHumanMutationTraceV2(m.Trace, contract.TraceAttestedV1, m.Attestation.TenantID, m.Attestation.Case, m.Attestation.Target, m.Attestation.UpdatedUnixNano, m.Attestation.ID, m.NextPanel.ID, refs); err != nil {
		return err
	}
	if m.Attestation.Resolution != contract.ResolutionEscalateHumanV1 {
		if len(m.AdditionalTraces) != 0 {
			return core.NewError(core.ErrorConflict, core.ReasonInvalidState, "non-escalating human Attestation cannot publish Escalated")
		}
		return nil
	}
	if len(m.AdditionalTraces) != 1 || m.NextCase == nil {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "escalating human Attestation requires exactly one Escalated Trace and successor Case")
	}
	caseRef := contract.HumanCaseExactRefV2{TenantID: m.NextCase.TenantID, ID: m.NextCase.ID, Revision: m.NextCase.Revision, Digest: m.NextCase.Digest}
	return validateHumanMutationTraceV2(m.AdditionalTraces[0], contract.TraceEscalatedV1, m.NextCase.TenantID, caseRef, m.NextPanel.Target, m.NextCase.UpdatedUnixNano, m.Attestation.ID, m.NextPanel.ID, []string{m.Attestation.ID})
}

func ValidateBeginHumanPanelDecisionTraceV2(m BeginHumanPanelDecisionMutationV2, quorumID string) error {
	caseRef := contract.HumanCaseExactRefV2{TenantID: m.NextCase.TenantID, ID: m.NextCase.ID, Revision: m.NextCase.Revision, Digest: m.NextCase.Digest}
	if m.Trace.SourceID != HumanDecisionTraceSourceV2 || m.Trace.SourceEpoch != 1 || m.Trace.SourceSequence != uint64(m.NextPanel.Revision) {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "human decision ReviewStarted source coordinate drifted")
	}
	return validateHumanMutationTraceV2(m.Trace, contract.TraceStartedV1, m.NextCase.TenantID, caseRef, m.NextPanel.Target, m.NextCase.UpdatedUnixNano, m.NextPanel.ID, m.NextCase.ID, []string{m.NextPanel.ID, m.NextCase.ID, m.NextPanel.Target.ID, quorumID})
}

func ValidateDecideHumanPanelTracesV2(m DecideHumanPanelMutationV2) error {
	if err := validateHumanMutationTraceV2(m.Trace, contract.TraceVerdictV1, m.Verdict.TenantID, m.Verdict.Case, m.Verdict.Target, m.Verdict.UpdatedUnixNano, m.Verdict.ID, m.NextPanel.ID, []string{m.Verdict.ID, m.Quorum.ID}); err != nil {
		return err
	}
	if len(m.AdditionalTraces) != 1 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "human Verdict requires exactly one Resolved Trace")
	}
	caseRef := contract.HumanCaseExactRefV2{TenantID: m.NextCase.TenantID, ID: m.NextCase.ID, Revision: m.NextCase.Revision, Digest: m.NextCase.Digest}
	return validateHumanMutationTraceV2(m.AdditionalTraces[0], contract.TraceResolvedV1, m.NextCase.TenantID, caseRef, m.Verdict.Target, m.NextCase.UpdatedUnixNano, m.Verdict.ID, m.NextPanel.ID, []string{m.Verdict.ID})
}

type DecideHumanPanelResultV2 struct {
	Panel   contract.HumanReviewPanelV2 `json:"panel"`
	Case    contract.ReviewCaseV1       `json:"case"`
	Verdict contract.HumanVerdictV2     `json:"verdict"`
}

// StoreV2 is a narrow Review-owned fact port. It exposes only compound Owner
// mutations and exact/current reads; it has no direct WriteVerdict method.
type StoreV2 interface {
	CreateHumanPanelV2(context.Context, CreateHumanPanelMutationV2) (CreateHumanPanelResultV2, error)
	ClaimHumanAssignmentV2(context.Context, ClaimHumanAssignmentMutationV2) (ClaimHumanAssignmentResultV2, error)
	RecordHumanAttestationV2(context.Context, RecordHumanAttestationMutationV2) (RecordHumanAttestationResultV2, error)
	BeginHumanPanelDecisionV2(context.Context, BeginHumanPanelDecisionMutationV2) (contract.HumanReviewPanelV2, contract.ReviewCaseV1, error)
	DecideHumanPanelV2(context.Context, DecideHumanPanelMutationV2) (DecideHumanPanelResultV2, error)

	InspectHumanPanelCurrentV2(context.Context, core.TenantID, string) (contract.HumanReviewPanelV2, error)
	InspectHumanPanelExactV2(context.Context, contract.HumanPanelExactRefV2) (contract.HumanReviewPanelV2, error)
	ListHumanPanelAssignmentsV2(context.Context, contract.HumanPanelExactRefV2) ([]contract.HumanPanelAssignmentV2, error)
	InspectHumanPanelAssignmentCurrentV2(context.Context, core.TenantID, string) (contract.HumanPanelAssignmentV2, error)
	InspectHumanPanelAssignmentExactV2(context.Context, contract.HumanPanelAssignmentExactRefV2) (contract.HumanPanelAssignmentV2, error)
	InspectHumanAttestationExactV2(context.Context, contract.HumanAttestationExactRefV2) (contract.HumanAttestationV2, error)
	InspectHumanAttestationByIdempotencyV2(context.Context, core.TenantID, string) (contract.HumanAttestationV2, error)
	ListHumanAttestationsByPanelV2(context.Context, contract.HumanPanelExactRefV2) ([]contract.HumanAttestationV2, error)
	InspectHumanQuorumDecisionExactV2(context.Context, contract.HumanQuorumDecisionExactRefV2) (contract.HumanQuorumDecisionV2, error)
	InspectHumanQuorumDecisionByPanelV2(context.Context, contract.HumanPanelExactRefV2) (contract.HumanQuorumDecisionV2, error)
	InspectHumanQuorumDecisionCurrentByPanelIDV2(context.Context, core.TenantID, string) (contract.HumanQuorumDecisionV2, error)
	InspectHumanVerdictExactV2(context.Context, contract.HumanVerdictExactRefV2) (contract.HumanVerdictV2, error)
	InspectHumanVerdictByPanelV2(context.Context, contract.HumanPanelExactRefV2) (contract.HumanVerdictV2, error)
	InspectHumanVerdictCurrentByPanelIDV2(context.Context, core.TenantID, string) (contract.HumanVerdictV2, error)
	InspectCaseExactV1(context.Context, core.TenantID, ExactFactRefV1) (contract.ReviewCaseV1, error)
	InspectTargetExactV1(context.Context, core.TenantID, ExactFactRefV1) (contract.TargetSnapshotV1, error)
	InspectRoundExactV1(context.Context, core.TenantID, ExactFactRefV1) (contract.ReviewRoundV1, error)
	InspectTraceExactV1(context.Context, core.TenantID, ExactFactRefV1) (contract.TraceFactV1, error)
}
