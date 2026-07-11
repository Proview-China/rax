package anthropicmessages

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/adaptercore"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol"
)

type Driver struct {
	base               *protocol.Base
	client             Client
	requestMapper      RequestMapper
	continuationMapper ContinuationMapper
	stopReasonMapper   StopReasonMapper
}

func New(binding protocol.Binding, dialect protocol.Dialect, client Client) (*Driver, error) {
	if binding.Protocol != modelinvoker.ProtocolMessages {
		return nil, fmt.Errorf("create Messages driver: binding protocol must be %q", modelinvoker.ProtocolMessages)
	}
	if protocol.IsNil(client) {
		return nil, fmt.Errorf("create Messages driver: client is nil")
	}
	base, err := protocol.NewBase(binding, dialect)
	if err != nil {
		return nil, fmt.Errorf("create Messages driver: %w", err)
	}
	driver := &Driver{base: base, client: client}
	if mapper, ok := dialect.(RequestMapper); ok {
		driver.requestMapper = mapper
	}
	if mapper, ok := dialect.(ContinuationMapper); ok {
		driver.continuationMapper = mapper
	}
	if mapper, ok := dialect.(StopReasonMapper); ok {
		driver.stopReasonMapper = mapper
	}
	return driver, nil
}

func (d *Driver) Binding() protocol.Binding {
	if d == nil || d.base == nil {
		return protocol.Binding{}
	}
	return d.base.Binding()
}

func (d *Driver) Invoke(ctx context.Context, request modelinvoker.Request) (modelinvoker.Response, error) {
	if d == nil || d.base == nil || protocol.IsNil(d.client) {
		return modelinvoker.Response{}, driverError(modelinvoker.ErrorProviderUnavailable, "messages.create", "protocol driver is not initialized")
	}
	if ctx == nil {
		return modelinvoker.Response{}, d.base.StampError(nil, request,
			driverError(modelinvoker.ErrorInvalidRequest, "messages.create", "context is nil"), "messages.create")
	}
	request.Stream = false
	if request.State != nil && request.State.Kind != modelinvoker.StateProviderContinuation {
		return modelinvoker.Response{}, d.base.StampError(ctx, request,
			mappingError("messages.validate", "Messages requires provider continuation state"), "messages.validate")
	}
	if err := d.base.Validate(request); err != nil {
		return modelinvoker.Response{}, err
	}
	binding := d.Binding()
	params, rawRequest, decisions, err := buildMessageParams(binding, request, false, d.requestMapper)
	if err != nil {
		return d.failedResponse(request, rawRequest, decisions, adaptercore.CapturedResponse{}, nil, err),
			d.base.StampError(ctx, request, err, "messages.map")
	}
	callContext, capture := adaptercore.WithResponseCapture(ctx, false, binding.Provider)
	native, headers, err := d.client.Create(callContext, params)
	captured := capture.Snapshot()
	headers = capturedHeaders(captured, headers)
	if limitErr := capture.Err(); limitErr != nil {
		err = limitErr
	}
	if err != nil {
		normalized := normalizeFailure(ctx, d.base, request, "messages.create", headers, err)
		response := d.failedResponse(request, rawRequest, decisions, captured, headers, err)
		var invocationError *modelinvoker.Error
		if response.RequestID == "" && errors.As(normalized, &invocationError) && invocationError != nil {
			response.RequestID = invocationError.RequestID
		}
		return response, normalized
	}
	response, normalizeErr := normalizeMessage(d.base, request, native, headers, d.continuationMapper, d.stopReasonMapper)
	if len(captured.Body) > 0 {
		response.RawResponse = modelinvoker.NewRawPayload(captured.Body)
	}
	response.RawRequest = rawRequest
	response.MappingReport.Decisions = append(response.MappingReport.Decisions, decisions...)
	response = d.base.StampResponse(request, response)
	return response, d.base.StampError(ctx, request, normalizeErr, "messages.normalize")
}

func (d *Driver) Stream(ctx context.Context, request modelinvoker.Request) (modelinvoker.Stream, error) {
	if d == nil || d.base == nil || protocol.IsNil(d.client) {
		return nil, driverError(modelinvoker.ErrorProviderUnavailable, "messages.stream", "protocol driver is not initialized")
	}
	if ctx == nil {
		return nil, d.base.StampError(nil, request,
			driverError(modelinvoker.ErrorInvalidRequest, "messages.stream", "context is nil"), "messages.stream")
	}
	request.Stream = true
	if request.State != nil && request.State.Kind != modelinvoker.StateProviderContinuation {
		return nil, d.base.StampError(ctx, request,
			mappingError("messages.validate", "Messages requires provider continuation state"), "messages.validate")
	}
	if err := d.base.Validate(request); err != nil {
		return nil, err
	}
	binding := d.Binding()
	params, rawRequest, decisions, err := buildMessageParams(binding, request, true, d.requestMapper)
	if err != nil {
		return nil, d.base.StampError(ctx, request, err, "messages.map")
	}
	callContext, capture := adaptercore.WithResponseCapture(ctx, true, binding.Provider)
	native, headers := d.client.Stream(callContext, params)
	headers = capturedHeaders(capture.Snapshot(), headers)
	if limitErr := capture.Err(); limitErr != nil {
		if !protocol.IsNil(native) {
			_ = native.Close()
		}
		return nil, d.base.StampError(ctx, request, limitErr, "messages.stream")
	}
	if protocol.IsNil(native) {
		return nil, d.base.StampError(ctx, request,
			driverError(modelinvoker.ErrorStreamInterrupted, "messages.stream", "native client returned a nil stream"), "messages.stream")
	}
	if err := native.Err(); err != nil {
		_ = native.Close()
		return nil, normalizeFailure(ctx, d.base, request, "messages.stream", headers, err)
	}
	stream := adaptercore.StreamWithResponseCapture(newMessageStream(ctx, request, d.base, native, headers, rawRequest, decisions, d.continuationMapper, d.stopReasonMapper), capture)
	return d.base.BindStream(ctx, request, stream), nil
}

func (d *Driver) failedResponse(
	request modelinvoker.Request,
	rawRequest modelinvoker.RawPayload,
	decisions []modelinvoker.MappingDecision,
	captured adaptercore.CapturedResponse,
	headers http.Header,
	err error,
) modelinvoker.Response {
	rawResponse := modelinvoker.RawPayload{}
	if len(captured.Body) > 0 {
		rawResponse = modelinvoker.NewRawPayload(captured.Body)
	} else {
		rawResponse = errorRawPayload(err)
	}
	return d.base.StampResponse(request, modelinvoker.Response{
		Protocol: modelinvoker.ProtocolMessages, Model: request.Model, Status: modelinvoker.ResponseStatusFailed,
		RequestID: d.base.RequestID(headers), ProviderMetadata: d.base.ProviderMetadata(headers),
		MappingReport: modelinvoker.MappingReport{Protocol: modelinvoker.ProtocolMessages, Endpoint: request.Endpoint,
			Decisions: append([]modelinvoker.MappingDecision(nil), decisions...)},
		RawRequest: rawRequest, RawResponse: rawResponse,
	})
}

func capturedHeaders(captured adaptercore.CapturedResponse, fallback http.Header) http.Header {
	if captured.Header != nil {
		return captured.Header.Clone()
	}
	return fallback
}

var _ protocol.Driver = (*Driver)(nil)
