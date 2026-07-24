package ports

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// ReviewGateAuthorizationReaderV1 is a consumer-only view over two existing
// Runtime V5 read methods. It deliberately exposes no FactPort mutation.
type ReviewGateAuthorizationReaderV1 interface {
	InspectOperationReviewAuthorizationExactV5(context.Context, runtimeports.OperationReviewAuthorizationRefV5) (runtimeports.OperationReviewAuthorizationFactV5, error)
	InspectCurrentOperationReviewAuthorizationV5(context.Context, runtimeports.OperationSubjectV3, core.EffectIntentID, string) (runtimeports.OperationReviewAuthorizationFactV5, error)
}

type ReviewGateV1 interface {
	EvaluateReviewGateV1(context.Context, contract.ReviewGateRequestV1) (contract.ReviewGateResultV1, error)
}
