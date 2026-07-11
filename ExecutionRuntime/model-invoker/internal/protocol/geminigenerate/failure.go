package geminigenerate

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/adaptercore"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol"
	"google.golang.org/genai"
)

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
	var apiError genai.APIError
	if errors.As(err, &apiError) {
		failure.Source = protocol.FailureSourceHTTP
		failure.HTTPStatus = apiError.Code
		failure.Type = protocol.BoundedFailureText(apiError.Status)
		failure.Code = failure.Type
		if failure.Code == "" && apiError.Code != 0 {
			failure.Code = strconv.Itoa(apiError.Code)
		}
		failure.Message = protocol.BoundedFailureText(strings.TrimSpace(apiError.Message))
		if failure.Message == "" {
			failure.Message = http.StatusText(apiError.Code)
		}
		failure.RequestID = protocol.BoundedFailureText(requestIDFromDetails(apiError.Details))
		if failure.RequestID == "" {
			failure.RequestID = base.RequestID(headers)
		}
		failure.RetryAfter = retryDelayFromDetails(apiError.Details)
		if headerDelay := adaptercore.RetryAfter(headers); headerDelay > failure.RetryAfter {
			failure.RetryAfter = headerDelay
		}
		if isBillingError(apiError) {
			failure.Signals = []protocol.Signal{{Key: "billing", Value: "true"}}
		}
		failure.Raw, _ = rawErrorPayload(err)
		return base.NormalizeFailure(ctx, request, operation, failure)
	}
	if protocol.ExtractCommonFailure(err, &failure) {
		return base.NormalizeFailure(ctx, request, operation, failure)
	}
	failure.Message = "provider operation failed"
	return base.NormalizeFailure(ctx, request, operation, failure)
}

func isBillingError(apiError genai.APIError) bool {
	for _, detail := range apiError.Details {
		reason, _ := detail["reason"].(string)
		if billingReason(reason) {
			return true
		}
		if metadata, ok := detail["metadata"].(map[string]any); ok {
			for _, key := range []string{"reason", "quota_limit", "quotaLimit"} {
				if value, _ := metadata[key].(string); billingReason(value) {
					return true
				}
			}
		}
	}
	return billingReason(apiError.Status)
}

func billingReason(value string) bool {
	upper := strings.ToUpper(value)
	return strings.Contains(upper, "BILLING") || strings.Contains(upper, "PAYMENT") || strings.Contains(upper, "CREDIT")
}

func requestIDFromDetails(details []map[string]any) string {
	for _, detail := range details {
		typeURL, _ := detail["@type"].(string)
		if typeURL != "type.googleapis.com/google.rpc.RequestInfo" {
			continue
		}
		if id, _ := detail["requestId"].(string); id != "" {
			return id
		}
		if id, _ := detail["request_id"].(string); id != "" {
			return id
		}
	}
	return ""
}

func retryDelayFromDetails(details []map[string]any) time.Duration {
	for _, detail := range details {
		typeURL, _ := detail["@type"].(string)
		if typeURL != "type.googleapis.com/google.rpc.RetryInfo" {
			continue
		}
		value, _ := detail["retryDelay"].(string)
		if value == "" {
			value, _ = detail["retry_delay"].(string)
		}
		if delay, err := time.ParseDuration(value); err == nil && delay >= 0 {
			return delay
		}
	}
	return 0
}

func rawErrorPayload(err error) (modelinvoker.RawPayload, error) {
	var apiError genai.APIError
	if !errors.As(err, &apiError) {
		return modelinvoker.RawPayload{}, nil
	}
	payload, payloadErr := adaptercore.RawPayload("", map[string]any{"error": apiError})
	if payloadErr != nil {
		return modelinvoker.RawPayload{}, fmt.Errorf("serialize GenerateContent error audit payload: %w", payloadErr)
	}
	return payload, nil
}
