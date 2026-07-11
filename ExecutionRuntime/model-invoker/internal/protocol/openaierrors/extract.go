// Package openaierrors extracts safe, data-only failures from the shared
// OpenAI Go SDK used by distinct Chat Completions and Responses drivers.
package openaierrors

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/adaptercore"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol"
	openaisdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/ssestream"
)

func Extract(err error, headers http.Header, requestID func(http.Header) string, failure *protocol.Failure) bool {
	if err == nil || failure == nil {
		return false
	}
	var apiError *openaisdk.Error
	if errors.As(err, &apiError) && apiError != nil {
		if headers == nil && apiError.Response != nil {
			headers = apiError.Response.Header
		}
		failure.Source = protocol.FailureSourceHTTP
		failure.HTTPStatus = apiError.StatusCode
		failure.Type = protocol.BoundedFailureText(apiError.Type)
		failure.Code = protocol.BoundedFailureText(apiError.Code)
		failure.Message = protocol.BoundedFailureText(strings.TrimSpace(apiError.Message))
		if failure.Message == "" {
			failure.Message = http.StatusText(apiError.StatusCode)
		}
		if requestID != nil {
			failure.RequestID = requestID(headers)
		}
		failure.RetryAfter = adaptercore.RetryAfter(headers)
		failure.Raw = ErrorRawPayload(err)
		return true
	}
	var streamError *ssestream.StreamError
	if errors.As(err, &streamError) && streamError != nil {
		failure.Source = protocol.FailureSourceStream
		failure.Raw = StreamErrorPayload(err)
		var envelope struct {
			Error struct {
				Message string `json:"message"`
				Type    string `json:"type"`
				Code    string `json:"code"`
			} `json:"error"`
		}
		if json.Unmarshal(streamError.Event.Data, &envelope) != nil {
			failure.Code = "invalid_stream_error"
			failure.Message = "stream returned an invalid error event"
		} else {
			failure.Type = protocol.BoundedFailureText(envelope.Error.Type)
			failure.Code = protocol.BoundedFailureText(envelope.Error.Code)
			failure.Message = protocol.BoundedFailureText(strings.TrimSpace(envelope.Error.Message))
			if failure.Message == "" {
				failure.Message = "stream returned an error event"
			}
		}
		return true
	}
	return false
}

func StreamErrorPayload(err error) modelinvoker.RawPayload {
	var streamError *ssestream.StreamError
	if !errors.As(err, &streamError) || streamError == nil || len(streamError.Event.Data) == 0 {
		return modelinvoker.RawPayload{}
	}
	return modelinvoker.NewRawPayload(streamError.Event.Data)
}

func ErrorRawPayload(err error) modelinvoker.RawPayload {
	var apiError *openaisdk.Error
	if !errors.As(err, &apiError) || apiError == nil {
		return modelinvoker.RawPayload{}
	}
	raw := apiError.RawJSON()
	if raw == "" {
		return modelinvoker.RawPayload{}
	}
	return modelinvoker.NewRawPayload([]byte(raw))
}
