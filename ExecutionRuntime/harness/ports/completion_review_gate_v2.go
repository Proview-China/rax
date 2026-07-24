package ports

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/bridgecontract"
)

type CompletionReviewWaitingCoordinatorV2 = bridgecontract.CompletionReviewWaitingCoordinatorV2

type CompletionReviewGateV2 interface {
	EvaluateCompletionReviewGateV2(context.Context, bridgecontract.CompletionReviewGateRequestV2) (bridgecontract.CompletionReviewGateResultV2, error)
}
