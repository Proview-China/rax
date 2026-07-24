package conformance

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type OperationReviewAuthorizationCaseV4 struct {
	Gateway ports.OperationReviewAuthorizationGovernancePortV4
	Request ports.CreateOperationReviewAuthorizationRequestV4
}

type OperationReviewAuthorizationReportV4 struct {
	RuntimeOwnerObserved     bool `json:"runtime_owner_observed"`
	CurrentInspectObserved   bool `json:"current_inspect_observed"`
	AuthorizationIsPermit    bool `json:"authorization_is_permit"`
	ReviewOwnedAuthorization bool `json:"review_owned_authorization"`
	ProductionClaimEligible  bool `json:"production_claim_eligible"`
}

func CheckOperationReviewAuthorizationV4(ctx context.Context, testCase OperationReviewAuthorizationCaseV4) (OperationReviewAuthorizationReportV4, error) {
	if testCase.Gateway == nil {
		return OperationReviewAuthorizationReportV4{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Review Authorization governance Port is required")
	}
	if err := testCase.Request.Validate(); err != nil {
		return OperationReviewAuthorizationReportV4{}, err
	}
	fact, err := testCase.Gateway.CreateOperationReviewAuthorizationV4(ctx, testCase.Request)
	if err != nil {
		return OperationReviewAuthorizationReportV4{}, err
	}
	if err := fact.Validate(); err != nil {
		return OperationReviewAuthorizationReportV4{}, err
	}
	current, err := testCase.Gateway.InspectCurrentOperationReviewAuthorizationV4(ctx, testCase.Request.Operation, testCase.Request.EffectID, fact.ID)
	if err != nil {
		return OperationReviewAuthorizationReportV4{}, err
	}
	if current.Digest != fact.Digest {
		return OperationReviewAuthorizationReportV4{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Review Authorization current Inspect returned different authority")
	}
	return OperationReviewAuthorizationReportV4{
		RuntimeOwnerObserved: true, CurrentInspectObserved: true,
		AuthorizationIsPermit: false, ReviewOwnedAuthorization: false,
		// Contract behavior never proves a production store, adapter or SLA.
		ProductionClaimEligible: false,
	}, nil
}
