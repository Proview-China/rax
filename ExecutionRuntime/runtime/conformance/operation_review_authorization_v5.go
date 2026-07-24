package conformance

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type OperationReviewAuthorizationCaseV5 struct {
	Gateway ports.OperationReviewAuthorizationGovernancePortV5
	Facts   ports.OperationReviewAuthorizationFactPortV5
	Request ports.CreateOperationReviewAuthorizationRequestV5
}

type OperationReviewAuthorizationReportV5 struct {
	RuntimeOwnerObserved     bool `json:"runtime_owner_observed"`
	CurrentInspectObserved   bool `json:"current_inspect_observed"`
	HistoricalExactObserved  bool `json:"historical_exact_observed"`
	AuthorizationIsPermit    bool `json:"authorization_is_permit"`
	ReviewOwnedAuthorization bool `json:"review_owned_authorization"`
	ProductionClaimEligible  bool `json:"production_claim_eligible"`
}

func CheckOperationReviewAuthorizationV5(ctx context.Context, testCase OperationReviewAuthorizationCaseV5) (OperationReviewAuthorizationReportV5, error) {
	if testCase.Gateway == nil || testCase.Facts == nil {
		return OperationReviewAuthorizationReportV5{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Review Authorization V5 governance and Fact Ports are required")
	}
	if err := testCase.Request.Validate(); err != nil {
		return OperationReviewAuthorizationReportV5{}, err
	}
	fact, err := testCase.Gateway.CreateOperationReviewAuthorizationV5(ctx, testCase.Request)
	if err != nil {
		return OperationReviewAuthorizationReportV5{}, err
	}
	if err := fact.Validate(); err != nil {
		return OperationReviewAuthorizationReportV5{}, err
	}
	current, err := testCase.Gateway.InspectCurrentOperationReviewAuthorizationV5(ctx, testCase.Request.Operation, testCase.Request.EffectID, fact.ID)
	if err != nil {
		return OperationReviewAuthorizationReportV5{}, err
	}
	historical, err := testCase.Facts.InspectOperationReviewAuthorizationExactV5(ctx, fact.RefV5())
	if err != nil {
		return OperationReviewAuthorizationReportV5{}, err
	}
	if current.Digest != fact.Digest || historical.Digest != fact.Digest {
		return OperationReviewAuthorizationReportV5{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Review Authorization V5 current or history returned different content")
	}
	return OperationReviewAuthorizationReportV5{
		RuntimeOwnerObserved: true, CurrentInspectObserved: true, HistoricalExactObserved: true,
		AuthorizationIsPermit: false, ReviewOwnedAuthorization: false, ProductionClaimEligible: false,
	}, nil
}
