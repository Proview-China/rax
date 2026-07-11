package anthropicmessages

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/adaptercore"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol"
	anthropicsdk "github.com/anthropics/anthropic-sdk-go"
)

type errorEnvelope struct {
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
	RequestID string `json:"request_id"`
}

func normalizeFailure(ctx context.Context, base *protocol.Base, request modelinvoker.Request, operation string, headers http.Header, err error) error {
	if err == nil {
		return nil
	}
	failure, existing, ready := protocol.BeginFailureExtraction(ctx, base.RequestID(headers), err)
	if existing != nil {
		return base.StampError(ctx, request, existing, operation)
	}
	if ready {
		return base.NormalizeFailure(ctx, request, operation, failure)
	}
	var apiError *anthropicsdk.Error
	if errors.As(err, &apiError) && apiError != nil {
		if headers == nil && apiError.Response != nil {
			headers = apiError.Response.Header
		}
		var envelope errorEnvelope
		_ = json.Unmarshal([]byte(apiError.RawJSON()), &envelope)
		failure.Source = protocol.FailureSourceHTTP
		failure.HTTPStatus = apiError.StatusCode
		failure.Type = protocol.BoundedFailureText(string(apiError.Type()))
		if failure.Type == "" {
			failure.Type = protocol.BoundedFailureText(envelope.Error.Type)
		}
		failure.Code = failure.Type
		failure.Message = protocol.BoundedFailureText(strings.TrimSpace(envelope.Error.Message))
		if failure.Message == "" {
			failure.Message = http.StatusText(apiError.StatusCode)
		}
		if failure.Message == "" {
			failure.Message = "Messages API request failed"
		}
		failure.RequestID = protocol.BoundedFailureText(apiError.RequestID)
		if failure.RequestID == "" {
			failure.RequestID = base.RequestID(headers)
		}
		if failure.RequestID == "" {
			failure.RequestID = protocol.BoundedFailureText(envelope.RequestID)
		}
		failure.RetryAfter = messagesRetryAfter(headers)
		failure.Raw = errorRawPayload(err)
		return base.NormalizeFailure(ctx, request, operation, failure)
	}
	if protocol.ExtractCommonFailure(err, &failure) {
		return base.NormalizeFailure(ctx, request, operation, failure)
	}
	failure.Message = "provider operation failed"
	return base.NormalizeFailure(ctx, request, operation, failure)
}

func messagesRetryAfter(headers http.Header) time.Duration {
	if value := strings.TrimSpace(headers.Get("retry-after-ms")); value != "" {
		if milliseconds, err := strconv.ParseFloat(value, 64); err == nil && milliseconds >= 0 {
			return time.Duration(milliseconds * float64(time.Millisecond))
		}
	}
	return adaptercore.RetryAfter(headers)
}

func errorRawPayload(err error) modelinvoker.RawPayload {
	var apiError *anthropicsdk.Error
	if !errors.As(err, &apiError) || apiError == nil || apiError.RawJSON() == "" {
		return modelinvoker.RawPayload{}
	}
	return modelinvoker.NewRawPayload([]byte(apiError.RawJSON()))
}
