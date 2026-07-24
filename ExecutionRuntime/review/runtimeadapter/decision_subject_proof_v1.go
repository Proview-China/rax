package runtimeadapter

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/nilcheck"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// DecisionSubjectProofReaderV1 is the Review Owner adapter used by Runtime's
// Review decision-governance current projector. It exposes only exact,
// tenant-qualified Target and Assignment proofs and has no mutation surface.
type DecisionSubjectProofReaderV1 struct {
	store reviewport.StoreV1
}

func NewDecisionSubjectProofReaderV1(store reviewport.StoreV1) (*DecisionSubjectProofReaderV1, error) {
	if nilcheck.IsNil(store) {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Review decision subject proof reader requires the Review Store")
	}
	return &DecisionSubjectProofReaderV1{store: store}, nil
}

func (r *DecisionSubjectProofReaderV1) InspectReviewDecisionTargetProofV1(ctx context.Context, expected runtimeports.ReviewDecisionTargetRefV1) (runtimeports.ReviewDecisionTargetRefV1, error) {
	if err := expected.Validate(); err != nil {
		return runtimeports.ReviewDecisionTargetRefV1{}, err
	}
	value, err := r.store.InspectTargetExactV1(ctx, expected.TenantID, reviewport.ExactV1(expected.ID, expected.Revision, expected.Digest))
	if err != nil {
		return runtimeports.ReviewDecisionTargetRefV1{}, err
	}
	if err := value.Validate(); err != nil {
		return runtimeports.ReviewDecisionTargetRefV1{}, err
	}
	actual := runtimeports.ReviewDecisionTargetRefV1{TenantID: value.TenantID, ID: value.ID, Revision: value.Revision, Digest: value.Digest, RunID: value.RunID}
	if actual != expected {
		return runtimeports.ReviewDecisionTargetRefV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Review Target proof drifted from the exact Runtime request")
	}
	return actual, nil
}

func (r *DecisionSubjectProofReaderV1) InspectReviewDecisionAssignmentProofV1(ctx context.Context, expected runtimeports.ReviewDecisionAssignmentRefV1) (runtimeports.ReviewDecisionAssignmentRefV1, error) {
	if err := expected.Validate(); err != nil {
		return runtimeports.ReviewDecisionAssignmentRefV1{}, err
	}
	value, err := r.store.InspectAssignmentExactV1(ctx, expected.TenantID, reviewport.ExactV1(expected.ID, expected.Revision, expected.Digest))
	if err != nil {
		return runtimeports.ReviewDecisionAssignmentRefV1{}, err
	}
	if err := value.Validate(); err != nil {
		return runtimeports.ReviewDecisionAssignmentRefV1{}, err
	}
	actual := runtimeports.ReviewDecisionAssignmentRefV1{TenantID: value.TenantID, ID: value.ID, Revision: value.Revision, Digest: value.Digest, ReviewerID: value.ReviewerID}
	if actual != expected {
		return runtimeports.ReviewDecisionAssignmentRefV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Review Assignment proof drifted from the exact Runtime request")
	}
	return actual, nil
}
