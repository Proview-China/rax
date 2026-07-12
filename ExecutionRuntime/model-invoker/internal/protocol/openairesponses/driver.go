package openairesponses

import (
	"context"
	"fmt"
	"net/http"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/adaptercore"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol"
)

type Driver struct {
	base           *protocol.Base
	client         Client
	requestMapper  RequestMapper
	responseMapper ResponseMapper
}

func New(binding protocol.Binding, dialect protocol.Dialect, client Client) (*Driver, error) {
	if binding.Protocol != modelinvoker.ProtocolResponses {
		return nil, fmt.Errorf("create responses driver: binding protocol must be %q", modelinvoker.ProtocolResponses)
	}
	if protocol.IsNil(client) {
		return nil, fmt.Errorf("create responses driver: client is nil")
	}
	base, err := protocol.NewBase(binding, dialect)
	if err != nil {
		return nil, fmt.Errorf("create responses driver: %w", err)
	}
	driver := &Driver{base: base, client: client}
	if mapper, ok := dialect.(RequestMapper); ok {
		driver.requestMapper = mapper
	}
	if mapper, ok := dialect.(ResponseMapper); ok {
		driver.responseMapper = mapper
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
		return modelinvoker.Response{}, driverError(modelinvoker.ErrorProviderUnavailable, "responses.create", "protocol driver is not initialized")
	}
	if ctx == nil {
		return modelinvoker.Response{}, d.base.StampError(nil, request,
			driverError(modelinvoker.ErrorInvalidRequest, "responses.create", "context is nil"), "responses.create")
	}
	request.Stream = false
	if request.State != nil && request.State.Kind != modelinvoker.StateServerContinuation {
		return modelinvoker.Response{}, d.base.StampError(ctx, request,
			mappingError("responses.validate", "Responses requires server continuation state"), "responses.validate")
	}
	if err := d.base.Validate(request); err != nil {
		return modelinvoker.Response{}, err
	}
	params, rawRequest, decisions, err := buildParams(request, d.requestMapper)
	if err != nil {
		return d.failedResponse(request, rawRequest, decisions, adaptercore.CapturedResponse{}, nil, err),
			d.base.StampError(ctx, request, err, "responses.map")
	}
	callContext, capture := adaptercore.WithResponseCapture(ctx, false, d.Binding().Provider)
	native, headers, err := d.client.Create(callContext, params)
	captured := capture.Snapshot()
	headers = capturedHeaders(captured, headers)
	if limitErr := capture.Err(); limitErr != nil {
		err = limitErr
	}
	if err != nil {
		return d.failedResponse(request, rawRequest, decisions, captured, headers, err),
			normalizeFailure(ctx, d.base, request, "responses.create", headers, err)
	}
	actualModel := ""
	if native != nil {
		actualModel = string(native.Model)
	}
	if identityErr := d.base.VerifyResponseModel(request, actualModel, "responses.response_model"); identityErr != nil {
		return modelinvoker.Response{Provider: d.Binding().Provider, Protocol: modelinvoker.ProtocolResponses, Status: modelinvoker.ResponseStatusFailed},
			d.base.StampError(ctx, request, identityErr, "responses.response_model")
	}
	response, normalizeErr := normalizeResponse(ctx, d.base, request, native, headers)
	if len(captured.Body) > 0 {
		response.RawResponse = modelinvoker.NewRawPayload(captured.Body)
	}
	response.RawRequest = rawRequest
	response.MappingReport.Decisions = append(response.MappingReport.Decisions, decisions...)
	if d.responseMapper != nil {
		if extensionErr := d.responseMapper.MapResponsesResponse(request, native, &response); extensionErr != nil {
			return response, d.base.StampError(ctx, request, extensionErr, "responses.normalize_extension")
		}
	}
	response = d.base.StampResponse(request, response)
	return response, d.base.StampError(ctx, request, normalizeErr, "responses.normalize")
}

func (d *Driver) Stream(ctx context.Context, request modelinvoker.Request) (modelinvoker.Stream, error) {
	if d == nil || d.base == nil || protocol.IsNil(d.client) {
		return nil, driverError(modelinvoker.ErrorProviderUnavailable, "responses.stream", "protocol driver is not initialized")
	}
	if ctx == nil {
		return nil, d.base.StampError(nil, request,
			driverError(modelinvoker.ErrorInvalidRequest, "responses.stream", "context is nil"), "responses.stream")
	}
	request.Stream = true
	if request.State != nil && request.State.Kind != modelinvoker.StateServerContinuation {
		return nil, d.base.StampError(ctx, request,
			mappingError("responses.validate", "Responses requires server continuation state"), "responses.validate")
	}
	if err := d.base.Validate(request); err != nil {
		return nil, err
	}
	params, rawRequest, decisions, err := buildParams(request, d.requestMapper)
	if err != nil {
		return nil, d.base.StampError(ctx, request, err, "responses.map")
	}
	callContext, capture := adaptercore.WithResponseCapture(ctx, true, d.Binding().Provider)
	native, headers := d.client.Stream(callContext, params)
	headers = capturedHeaders(capture.Snapshot(), headers)
	if limitErr := capture.Err(); limitErr != nil {
		if !protocol.IsNil(native) {
			_ = native.Close()
		}
		return nil, d.base.StampError(ctx, request, limitErr, "responses.stream")
	}
	if protocol.IsNil(native) {
		return nil, d.base.StampError(ctx, request,
			driverError(modelinvoker.ErrorStreamInterrupted, "responses.stream", "native client returned a nil stream"), "responses.stream")
	}
	if err := native.Err(); err != nil {
		_ = native.Close()
		return nil, normalizeFailure(ctx, d.base, request, "responses.stream", headers, err)
	}
	stream := adaptercore.StreamWithResponseCapture(newStream(ctx, d.base, request, native, headers, rawRequest, decisions, d.responseMapper), capture)
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
		Protocol: modelinvoker.ProtocolResponses, Model: request.Model, Status: modelinvoker.ResponseStatusFailed,
		RequestID: d.base.RequestID(headers), ProviderMetadata: d.base.ProviderMetadata(headers),
		MappingReport: modelinvoker.MappingReport{Protocol: modelinvoker.ProtocolResponses, Endpoint: request.Endpoint,
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
