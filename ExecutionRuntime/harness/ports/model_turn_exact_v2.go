package ports

import (
	"context"

	bridgecontract "github.com/Proview-China/rax/ExecutionRuntime/harness/bridgecontract"
)

type ModelTurnDispatchRepositoryV2 interface {
	EnsureModelTurnDispatchAttemptV2(context.Context, bridgecontract.ModelTurnDispatchFactV2) (bridgecontract.ModelTurnDispatchFactV2, error)
	BindModelTurnDispatchOutcomeV2(context.Context, bridgecontract.ModelTurnDispatchFactV2) (bridgecontract.ModelTurnDispatchFactV2, error)
	InspectExactModelTurnDispatchV2(context.Context, bridgecontract.ModelTurnDispatchRefV2) (bridgecontract.ModelTurnDispatchFactV2, error)
}

// ExactModelTurnPortV2 is additive. It binds a Model V3 owner-local prepared
// outcome and never substitutes for the legacy ModelTurnPort or a Provider
// actual-point gateway.
type ExactModelTurnPortV2 interface {
	StartOrInspectExactModelTurnV2(context.Context, bridgecontract.ModelTurnExactEnvelopeV2) (bridgecontract.ModelTurnDispatchFactV2, error)
	InspectExactModelTurnV2(context.Context, bridgecontract.ModelTurnDispatchRefV2) (bridgecontract.ModelTurnDispatchFactV2, error)
}
