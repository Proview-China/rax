package ports

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
)

// ContextEvaluatorV1 is a Context-owned plug-in seam. Its output remains an
// Observation until Context validates and admits it as an Evaluation fact.
type ContextEvaluatorV1 interface {
	RefV1() contract.ContextEvaluatorRefV1
	EvaluateContextV1(context.Context, contract.ContextEvaluationInputV1) (contract.ContextEvaluationObservationV1, error)
}
