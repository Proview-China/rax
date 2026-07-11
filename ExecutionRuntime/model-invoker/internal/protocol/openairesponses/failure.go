package openairesponses

import (
	"context"
	"net/http"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol/openaierrors"
)

func normalizeFailure(ctx context.Context, base *protocol.Base, request modelinvoker.Request, operation string, headers http.Header, err error) error {
	if err == nil {
		return nil
	}
	failure, existing, ready := protocol.BeginFailureExtraction(ctx, base.RequestID(headers), err)
	if existing != nil {
		return base.StampError(ctx, request, existing, operation)
	}
	if ready || openaierrors.Extract(err, headers, base.RequestID, &failure) || protocol.ExtractCommonFailure(err, &failure) {
		return base.NormalizeFailure(ctx, request, operation, failure)
	}
	failure.Message = "provider operation failed"
	return base.NormalizeFailure(ctx, request, operation, failure)
}

func streamErrorPayload(err error) modelinvoker.RawPayload {
	return openaierrors.StreamErrorPayload(err)
}

func errorRawPayload(err error) modelinvoker.RawPayload {
	return openaierrors.ErrorRawPayload(err)
}
