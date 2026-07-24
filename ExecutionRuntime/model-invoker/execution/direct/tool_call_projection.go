package direct

import (
	"context"
	"fmt"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
)

func ensureToolCallObservationProjectionV1(
	ctx context.Context,
	repository modelinvoker.ToolCallCandidateObservationProjectionRepositoryV1,
	invocationID string,
	responseID string,
	sourceSequence uint64,
	observation modelinvoker.ToolCallCandidateObservationV1,
) (modelinvoker.ToolCallCandidateObservationProjectionV1, error) {
	if projectionRepositoryUnavailableV1(repository) {
		return modelinvoker.ToolCallCandidateObservationProjectionV1{}, ErrToolCallObservationProjectionUnavailable
	}
	sealed, err := modelinvoker.NewToolCallCandidateObservationProjectionV1(invocationID, sourceSequence, responseID, observation)
	if err != nil {
		return modelinvoker.ToolCallCandidateObservationProjectionV1{}, err
	}
	return modelinvoker.EnsureToolCallCandidateObservationProjectionV1(
		ctx, repository, sealed,
	)
}

func projectionFailureCodeV1(err error) string {
	kind := modelinvoker.ToolCallCandidateObservationProjectionErrorKindOfV1(err)
	if kind == "" {
		return "tool_call_observation_projection_failed"
	}
	return fmt.Sprintf("tool_call_observation_projection_%s", kind)
}
