package memory

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/decisioncurrent"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/nilcheck"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// ResolveDecisionCurrentRequestV1 returns the sole Attestation that advanced
// the exact current Case into attested/deciding state. It is a read-only
// current-index resolution; callers cannot inject an Attestation ID.
func (s *Store) ResolveDecisionCurrentRequestV1(ctx context.Context, request reviewport.DecisionCurrentResolveRequestV1) (reviewport.DecisionCurrentRequestV1, error) {
	if err := checkContext(ctx); err != nil {
		return reviewport.DecisionCurrentRequestV1{}, err
	}
	if request.TenantID == "" || request.CaseID == "" || request.ExpectedCase.Revision == 0 || request.ExpectedCase.Digest.Validate() != nil {
		return reviewport.DecisionCurrentRequestV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "decision current resolve request is incomplete")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	caseFact, ok := s.cases[key(request.TenantID, request.CaseID)]
	if !ok {
		return reviewport.DecisionCurrentRequestV1{}, notFound("review case")
	}
	if err := expected(caseFact.FactIdentityV1, request.ExpectedCase); err != nil {
		return reviewport.DecisionCurrentRequestV1{}, err
	}
	if caseFact.State != contract.CaseAttestedV1 && caseFact.State != contract.CaseDecidingV1 {
		return reviewport.DecisionCurrentRequestV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "review Case is not ready for decision current resolution")
	}
	prefix := string(request.TenantID) + "\x00"
	attestationID := ""
	for itemKey, value := range s.attestations {
		if !strings.HasPrefix(itemKey, prefix) || value.CaseID != caseFact.ID || value.CaseRevision+1 != caseFact.Revision || value.RoundID != caseFact.CurrentRoundID || value.AssignmentID != caseFact.CurrentAssignment {
			continue
		}
		if attestationID != "" && attestationID != value.ID {
			return reviewport.DecisionCurrentRequestV1{}, core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "multiple Attestations claim the exact current Case revision")
		}
		attestationID = value.ID
	}
	if attestationID == "" {
		return reviewport.DecisionCurrentRequestV1{}, notFound("current review attestation")
	}
	return reviewport.DecisionCurrentRequestV1{TenantID: request.TenantID, CaseID: request.CaseID, ExpectedCase: request.ExpectedCase, AttestationID: attestationID}, nil
}

// InspectDecisionOwnerInputsV1 returns all Review-owned Decide inputs from one
// Store read lock. The returned value is a deep clone and therefore cannot
// mutate Store state after the lock is released.
func (s *Store) InspectDecisionOwnerInputsV1(ctx context.Context, request reviewport.DecisionCurrentRequestV1) (reviewport.DecisionOwnerInputsV1, error) {
	if err := checkContext(ctx); err != nil {
		return reviewport.DecisionOwnerInputsV1{}, err
	}
	if request.TenantID == "" || request.CaseID == "" || request.AttestationID == "" || request.ExpectedCase.Revision == 0 || request.ExpectedCase.Digest.Validate() != nil {
		return reviewport.DecisionOwnerInputsV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "decision current request is incomplete")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	caseKey := key(request.TenantID, request.CaseID)
	caseFact, ok := s.cases[caseKey]
	if !ok {
		return reviewport.DecisionOwnerInputsV1{}, notFound("review case")
	}
	if err := expected(caseFact.FactIdentityV1, request.ExpectedCase); err != nil {
		historical, exists := s.caseHistory[caseKey][request.ExpectedCase.Revision]
		if !exists || historical.Digest != request.ExpectedCase.Digest {
			return reviewport.DecisionOwnerInputsV1{}, err
		}
		caseFact = historical
	}
	target, ok := s.targetHistory[key(request.TenantID, caseFact.TargetID)][caseFact.TargetRevision]
	if !ok || target.Digest != caseFact.TargetDigest {
		return reviewport.DecisionOwnerInputsV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonReviewCandidateConflict, "case target exact history is missing or drifted")
	}
	round, ok := s.rounds[key(request.TenantID, caseFact.CurrentRoundID)]
	if !ok {
		return reviewport.DecisionOwnerInputsV1{}, notFound("review round")
	}
	if caseFact.Rubric == nil || round.Rubric == nil || *round.Rubric != *caseFact.Rubric || round.RubricDigest != caseFact.Rubric.Digest {
		return reviewport.DecisionOwnerInputsV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "decision Case/Round lacks one exact Rubric ref")
	}
	rubricKey := key(request.TenantID, caseFact.Rubric.ID)
	rubric, ok := s.rubricHistory[rubricKey][caseFact.Rubric.Revision]
	if !ok || rubric.ExactRef() != *caseFact.Rubric || s.rubricCurrent[rubricKey] != *caseFact.Rubric || s.rubricHighestRevision[rubricKey] != caseFact.Rubric.Revision {
		return reviewport.DecisionOwnerInputsV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "decision exact Rubric history/current index drifted")
	}
	actualPoint := s.clock()
	if actualPoint.IsZero() || rubric.ValidateCurrent(*caseFact.Rubric, actualPoint) != nil {
		return reviewport.DecisionOwnerInputsV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "decision exact Rubric is expired, revoked or not current")
	}
	assignment, ok := s.assignments[key(request.TenantID, caseFact.CurrentAssignment)]
	if !ok {
		return reviewport.DecisionOwnerInputsV1{}, notFound("review assignment")
	}
	attestation, ok := s.attestations[key(request.TenantID, request.AttestationID)]
	if !ok {
		return reviewport.DecisionOwnerInputsV1{}, notFound("review attestation")
	}
	if err := attestation.ValidateProductionAutoProvenanceV4(); err != nil {
		return reviewport.DecisionOwnerInputsV1{}, err
	}
	findings := make([]contract.FindingV1, 0, len(attestation.FindingRefs))
	for _, id := range attestation.FindingRefs {
		finding, exists := s.findings[key(request.TenantID, id)]
		if !exists {
			return reviewport.DecisionOwnerInputsV1{}, notFound("review finding")
		}
		findings = append(findings, finding)
	}
	sort.Slice(findings, func(i, j int) bool { return findings[i].ID < findings[j].ID })

	var apply *contract.DomainApplySettlementFactV1
	var result *contract.ReviewerInvocationResultFactV1
	if attestation.Route == contract.RouteAutoV1 {
		if attestation.DomainApplySettlement == nil {
			return reviewport.DecisionOwnerInputsV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectSettlementMissing, "auto attestation has no ApplySettlement ref")
		}
		storedApply, exists := s.applySettlements[key(request.TenantID, attestation.DomainApplySettlement.ID)]
		if !exists {
			return reviewport.DecisionOwnerInputsV1{}, notFound("review ApplySettlement")
		}
		storedResult, exists := s.domainResults[key(request.TenantID, storedApply.DomainResultID)]
		if !exists {
			return reviewport.DecisionOwnerInputsV1{}, notFound("review domain result")
		}
		applyCopy, _ := clone(storedApply)
		resultCopy, _ := clone(storedResult)
		apply, result = &applyCopy, &resultCopy
	}
	evidence, err := decisionEvidence(target, attestation, findings)
	if err != nil {
		return reviewport.DecisionOwnerInputsV1{}, err
	}
	value := reviewport.DecisionOwnerInputsV1{Target: target, Case: caseFact, Round: round, Rubric: rubric, Assignment: assignment, Attestation: attestation, Findings: findings, ApplySettlement: apply, DomainResult: result, Evidence: evidence}
	return clone(value)
}

func decisionEvidence(target contract.TargetSnapshotV1, attestation contract.AttestationV1, findings []contract.FindingV1) ([]runtimeports.ReviewEvidenceRefV2, error) {
	byRef := make(map[string]runtimeports.ReviewEvidenceRefV2)
	add := func(value runtimeports.ReviewEvidenceRefV2) error {
		if old, ok := byRef[value.Ref]; ok && old != value {
			return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "same Review evidence ref changed content")
		}
		byRef[value.Ref] = value
		return nil
	}
	for _, value := range target.Evidence {
		if err := add(value); err != nil {
			return nil, err
		}
	}
	for _, value := range attestation.Evidence {
		if err := add(value); err != nil {
			return nil, err
		}
	}
	for _, finding := range findings {
		for _, value := range finding.Evidence {
			if err := add(value); err != nil {
				return nil, err
			}
		}
	}
	out := make([]runtimeports.ReviewEvidenceRefV2, 0, len(byRef))
	for _, value := range byRef {
		out = append(out, value)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Ref < out[j].Ref })
	return out, nil
}

// DecisionCurrentSourceV1 remains an alias for callers that used the original
// memory package constructor. The implementation is Store-neutral and is also
// used by the durable SQLite backend.
type DecisionCurrentSourceV1 = decisioncurrent.SourceV1

func NewDecisionCurrentSourceV1(store reviewport.StoreV1, external reviewport.DecisionExternalCurrentReaderV1, clock func() time.Time) (*decisioncurrent.SourceV1, error) {
	if nilcheck.IsNil(store) || nilcheck.IsNil(external) || clock == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "decision current source requires Store, external Owner reader and clock")
	}
	return decisioncurrent.NewSourceV1(store, external, clock)
}

var _ reviewport.StoreV1 = (*Store)(nil)
