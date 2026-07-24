package ports

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
)

type ContextOutcomeFactStoreV1 interface {
	PutContextOutcomeV1(context.Context, contract.ContextOutcomeFactV1) (contract.FactRef, error)
	InspectContextOutcomeV1(context.Context, contract.FactRef) (contract.ContextOutcomeFactV1, error)
	PutContextEvaluationV1(context.Context, contract.ContextEvaluationFactV1) (contract.FactRef, error)
	InspectContextEvaluationV1(context.Context, contract.FactRef) (contract.ContextEvaluationFactV1, error)
	PutContextFeedbackCandidateV1(context.Context, contract.ContextFeedbackCandidateFactV1) (contract.FactRef, error)
	InspectContextFeedbackCandidateV1(context.Context, contract.FactRef) (contract.ContextFeedbackCandidateFactV1, error)
}
