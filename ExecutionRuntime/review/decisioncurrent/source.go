// Package decisioncurrent composes one exact Review-owner snapshot with the
// independently owned Policy/Authority/Scope/Binding/Evidence current reads.
// It has no write access to those external domains.
package decisioncurrent

import (
	"context"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/nilcheck"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type SourceV1 struct {
	store    reviewport.StoreV1
	external reviewport.DecisionExternalCurrentReaderV1
	clock    func() time.Time
}

func NewSourceV1(store reviewport.StoreV1, external reviewport.DecisionExternalCurrentReaderV1, clock func() time.Time) (*SourceV1, error) {
	if nilcheck.IsNil(store) || nilcheck.IsNil(external) || clock == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "decision current source requires Store, external Owner reader and clock")
	}
	return &SourceV1{store: store, external: external, clock: clock}, nil
}

func (s *SourceV1) InspectDecisionCurrentV1(ctx context.Context, request reviewport.DecisionCurrentRequestV1) (contract.DecisionCurrentSnapshotV1, error) {
	baseline := s.clock()
	if baseline.IsZero() {
		return contract.DecisionCurrentSnapshotV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "decision Rubric S1 clock is unavailable")
	}
	inputs, err := s.store.InspectDecisionOwnerInputsV1(ctx, request)
	if err != nil {
		return contract.DecisionCurrentSnapshotV1{}, err
	}
	if inputs.Round.Rubric == nil {
		return contract.DecisionCurrentSnapshotV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "decision Round has no exact Rubric ref")
	}
	rubricS1, err := s.store.InspectRubricCurrentV1(ctx, request.TenantID, *inputs.Round.Rubric, baseline)
	if err != nil {
		return contract.DecisionCurrentSnapshotV1{}, err
	}
	external, err := s.external.InspectDecisionExternalCurrentV1(ctx, reviewport.DecisionExternalCurrentRequestV1{Target: inputs.Target, Assignment: inputs.Assignment, Attestation: inputs.Attestation, Evidence: append([]runtimeports.ReviewEvidenceRefV2(nil), inputs.Evidence...)})
	if err != nil {
		return contract.DecisionCurrentSnapshotV1{}, err
	}
	fresh := s.clock()
	if fresh.IsZero() || fresh.Before(baseline) {
		return contract.DecisionCurrentSnapshotV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "decision Rubric clock regressed between S1 and S2")
	}
	rubricS2, err := s.store.InspectRubricCurrentV1(ctx, request.TenantID, *inputs.Round.Rubric, fresh)
	if err != nil {
		return contract.DecisionCurrentSnapshotV1{}, err
	}
	if rubricS1.ExactRef() != rubricS2.ExactRef() || rubricS1.Digest != rubricS2.Digest || rubricS2.ExactRef() != inputs.Rubric.ExactRef() {
		return contract.DecisionCurrentSnapshotV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "decision exact Rubric drifted between S1 and S2")
	}
	expires := minimumPositive(inputs.Target.ExpiresUnixNano, inputs.Case.ExpiresUnixNano, inputs.Round.ExpiresUnixNano, rubricS2.ExpiresUnixNano, inputs.Assignment.ExpiresUnixNano, inputs.Assignment.LeaseExpiresUnixNano, inputs.Attestation.ExpiresUnixNano, external.ExpiresUnixNano)
	for _, finding := range inputs.Findings {
		expires = minimumPositive(expires, finding.ExpiresUnixNano)
	}
	for _, item := range external.Evidence {
		expires = minimumPositive(expires, item.ExpiresUnixNano)
	}
	snapshot := contract.DecisionCurrentSnapshotV1{Revision: inputs.Case.Revision, Target: inputs.Target, Case: inputs.Case, Round: inputs.Round, Rubric: rubricS2, Assignment: inputs.Assignment, Attestation: inputs.Attestation, Findings: append([]contract.FindingV1(nil), inputs.Findings...), ApplySettlement: inputs.ApplySettlement, DomainResult: inputs.DomainResult, Policy: external.Policy, ActorAuthority: external.ActorAuthority, ReviewerAuthority: external.ReviewerAuthority, Scope: external.Scope, Binding: external.Binding, Evidence: append([]contract.DecisionEvidenceCurrentV1(nil), external.Evidence...), ExternalProof: external.ExternalProof, Current: external.Current, ExpiresUnixNano: expires}
	return contract.SealDecisionCurrentSnapshotV1(snapshot)
}

func minimumPositive(values ...int64) int64 {
	minimum := int64(0)
	for _, value := range values {
		if value > 0 && (minimum == 0 || value < minimum) {
			minimum = value
		}
	}
	return minimum
}

var _ reviewport.DecisionCurrentReaderV1 = (*SourceV1)(nil)
