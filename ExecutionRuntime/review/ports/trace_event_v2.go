package ports

import (
	"context"
	"slices"
	"sort"
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const MaxTracePageV2 = 256

// TracePageAfterV2 is the full immutable position of the last delivered Review
// Trace. It is a paging coordinate only; it grants no Evidence or Timeline
// authority.
type TracePageAfterV2 struct {
	SourceID       string         `json:"source_id"`
	SourceEpoch    core.Epoch     `json:"source_epoch"`
	SourceSequence uint64         `json:"source_sequence"`
	Trace          ExactFactRefV1 `json:"trace"`
}

func (v TracePageAfterV2) Validate() error {
	if strings.TrimSpace(v.SourceID) == "" || v.SourceEpoch == 0 || v.SourceSequence == 0 || strings.TrimSpace(v.Trace.ID) == "" || v.Trace.Revision == 0 || v.Trace.Digest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonEvidenceCursorInvalid, "review Trace page position is incomplete")
	}
	return nil
}

type ListTracePageRequestV2 struct {
	TenantID core.TenantID     `json:"tenant_id"`
	CaseID   string            `json:"case_id"`
	After    *TracePageAfterV2 `json:"after,omitempty"`
	Limit    int               `json:"limit"`
}

func (v ListTracePageRequestV2) Validate() error {
	if strings.TrimSpace(string(v.TenantID)) == "" || strings.TrimSpace(v.CaseID) == "" || v.Limit <= 0 || v.Limit > MaxTracePageV2 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "review Trace page request is invalid")
	}
	if v.After != nil {
		return v.After.Validate()
	}
	return nil
}

type ListTracePageResultV2 struct {
	Events []contract.TraceFactV1 `json:"events"`
	Next   *TracePageAfterV2      `json:"next,omitempty"`
}

// TraceEventReaderV2 is the narrow, read-only Review Owner event source. It
// returns deep clones from committed Review facts and cannot publish a
// Continuity Timeline projection.
type TraceEventReaderV2 interface {
	InspectTraceExactV1(context.Context, core.TenantID, ExactFactRefV1) (contract.TraceFactV1, error)
	ListTracePageV2(context.Context, ListTracePageRequestV2) (ListTracePageResultV2, error)
}

type CreateFindingWithTraceMutationV2 struct {
	Finding contract.FindingV1   `json:"finding"`
	Trace   contract.TraceFactV1 `json:"trace"`
}

// TransitionCaseWithTraceMutationV2 is the sole public admission/routing Case
// transition. The Case revision and its Trace are staged and committed under
// one Review-owner lock; neither object may become visible alone.
type TransitionCaseWithTraceMutationV2 struct {
	Expected ExpectedFactV1        `json:"expected"`
	Next     contract.ReviewCaseV1 `json:"next"`
	Trace    contract.TraceFactV1  `json:"trace"`
}

func ValidateTransitionCaseTraceV2(m TransitionCaseWithTraceMutationV2) error {
	if err := m.Next.Validate(); err != nil {
		return err
	}
	if err := m.Trace.Validate(); err != nil {
		return err
	}
	wantEvent := contract.TraceEventV1("")
	switch m.Next.State {
	case contract.CaseAdmittedV1:
		wantEvent = contract.TraceAdmittedV1
	case contract.CaseRoutedV1:
		wantEvent = contract.TraceRoutedV1
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidTransition, "compound Case transition only supports admission and routing")
	}
	if m.Trace.Event != wantEvent || m.Trace.TenantID != m.Next.TenantID || m.Trace.CaseID != m.Next.ID || m.Trace.CaseRevision != m.Next.Revision || m.Trace.TargetID != m.Next.TargetID || m.Trace.TargetRevision != m.Next.TargetRevision || m.Trace.TargetDigest != m.Next.TargetDigest || m.Trace.CreatedUnixNano != m.Next.UpdatedUnixNano || m.Trace.UpdatedUnixNano != m.Next.UpdatedUnixNano || m.Trace.CausationID != m.Next.ID || m.Trace.CorrelationID != m.Next.ID || len(m.Trace.FactRefs) != 1 || m.Trace.FactRefs[0] != m.Next.ID {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Case transition Trace does not bind the exact successor Case")
	}
	return nil
}

// TraceEventStoreV2 adds the one compound mutation whose V1 shape could not
// carry a Trace. Claim/Attestation/Decide remain source-compatible V1 methods
// and use their optional V2 Trace fields.
type TraceEventStoreV2 interface {
	TraceEventReaderV2
	CreateFindingWithTraceV2(context.Context, CreateFindingWithTraceMutationV2) (contract.FindingV1, error)
}

type CaseTransitionStoreV2 interface {
	TraceEventReaderV2
	TransitionCaseWithTraceV2(context.Context, TransitionCaseWithTraceMutationV2) (contract.ReviewCaseV1, error)
}

func ValidateClaimAssignmentTraceV2(trace contract.TraceFactV1, successor contract.ReviewCaseV1, assignment contract.ReviewerAssignmentV1) error {
	if err := trace.Validate(); err != nil {
		return err
	}
	if err := successor.Validate(); err != nil {
		return err
	}
	if err := assignment.Validate(); err != nil {
		return err
	}
	wantRefs := []string{assignment.ID}
	if trace.Event != contract.TraceStartedV1 || trace.TenantID != successor.TenantID || trace.CaseID != successor.ID || trace.CaseRevision != successor.Revision || trace.TargetID != successor.TargetID || trace.TargetRevision != successor.TargetRevision || trace.TargetDigest != successor.TargetDigest || trace.CreatedUnixNano != successor.UpdatedUnixNano || trace.UpdatedUnixNano != successor.UpdatedUnixNano || assignment.UpdatedUnixNano != successor.UpdatedUnixNano || trace.CausationID != assignment.ID || trace.CorrelationID != successor.ID || !slices.Equal(trace.FactRefs, wantRefs) {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "ReviewStarted Trace does not bind the exact successor Case and Assignment")
	}
	return nil
}

// CanonicalAttestationTraceRefsV2 derives the only accepted Trace reference
// set from the sealed Attestation. Auto attestations additionally bind the
// exact stored invocation/result/settlement provenance that the Store has
// already re-read under its owner lock. domainResultID must therefore come
// from that stored result, never from caller input.
func CanonicalAttestationTraceRefsV2(attestation contract.AttestationV1, domainResultID string) ([]string, error) {
	if err := attestation.ValidateProductionAutoProvenanceV4(); err != nil {
		return nil, err
	}
	refs := append([]string{attestation.ID}, attestation.FindingRefs...)
	switch attestation.Route {
	case contract.RouteAutoV1:
		if attestation.AutoProvenance == nil || attestation.DomainApplySettlement == nil || strings.TrimSpace(domainResultID) == "" {
			return nil, core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectSettlementMissing, "auto Attestation Trace lacks exact provenance refs")
		}
		refs = append(refs,
			attestation.AutoProvenance.Attempt.ID,
			attestation.AutoProvenance.Observation.ID,
			attestation.AutoProvenance.Rubric.ID,
			attestation.DomainApplySettlement.ID,
			domainResultID,
		)
	default:
		if domainResultID != "" {
			return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "non-auto Attestation cannot bind an auto DomainResult")
		}
	}
	sort.Strings(refs)
	for index, value := range refs {
		if strings.TrimSpace(value) == "" || (index > 0 && refs[index-1] == value) {
			return nil, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Attestation Trace provenance refs are empty or collide")
		}
	}
	return refs, nil
}

func ValidateAttestationTracesV2(primary contract.TraceFactV1, additional []contract.TraceFactV1, predecessor, successor contract.ReviewCaseV1, attestation contract.AttestationV1, domainResultID string) error {
	refs, err := CanonicalAttestationTraceRefsV2(attestation, domainResultID)
	if err != nil {
		return err
	}
	if err := validateMutationTraceV2(primary, contract.TraceAttestedV1, predecessor, attestation.ID, attestation.ObservedUnixNano, refs); err != nil {
		return err
	}
	if successor.State == contract.CaseWaitingHumanV1 {
		if len(additional) != 1 {
			return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "escalating Attestation requires exactly one Escalated Trace")
		}
		return validateMutationTraceV2(additional[0], contract.TraceEscalatedV1, successor, attestation.ID, attestation.ObservedUnixNano, refs)
	}
	if len(additional) != 0 {
		return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "non-escalating Attestation cannot publish an Escalated Trace")
	}
	return nil
}

func ValidateDecisionTracesV2(primary contract.TraceFactV1, additional []contract.TraceFactV1, predecessor, successor contract.ReviewCaseV1, verdict contract.VerdictV1) error {
	if err := validateMutationTraceV2(primary, contract.TraceVerdictV1, predecessor, verdict.ID, verdict.UpdatedUnixNano, []string{verdict.ID}); err != nil {
		return err
	}
	if len(additional) != 1 {
		return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "Decide requires exactly one Resolved Trace")
	}
	return validateMutationTraceV2(additional[0], contract.TraceResolvedV1, successor, verdict.ID, verdict.UpdatedUnixNano, []string{verdict.ID})
}

func ValidateInvalidationTraceV2(trace contract.TraceFactV1, predecessor contract.ReviewCaseV1, event contract.TraceEventV1, updatedUnixNano int64) error {
	causationID := predecessor.ID
	if predecessor.VerdictID != "" {
		causationID = predecessor.VerdictID
	}
	return validateMutationTraceV2(trace, event, predecessor, causationID, updatedUnixNano, []string{causationID})
}

func validateMutationTraceV2(trace contract.TraceFactV1, event contract.TraceEventV1, caseValue contract.ReviewCaseV1, causationID string, updatedUnixNano int64, factRefs []string) error {
	if err := trace.Validate(); err != nil {
		return err
	}
	if err := caseValue.Validate(); err != nil {
		return err
	}
	if trace.Event != event || trace.TenantID != caseValue.TenantID || trace.CaseID != caseValue.ID || trace.CaseRevision != caseValue.Revision || trace.TargetID != caseValue.TargetID || trace.TargetRevision != caseValue.TargetRevision || trace.TargetDigest != caseValue.TargetDigest || trace.CreatedUnixNano != updatedUnixNano || trace.UpdatedUnixNano != updatedUnixNano || trace.CausationID != causationID || trace.CorrelationID != caseValue.ID || !slices.Equal(trace.FactRefs, factRefs) {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Review mutation Trace does not bind the exact canonical domain facts")
	}
	return nil
}
