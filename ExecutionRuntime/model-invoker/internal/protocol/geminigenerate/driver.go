package geminigenerate

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
	base   *protocol.Base
	client Client
}

func New(binding protocol.Binding, dialect protocol.Dialect, client Client) (*Driver, error) {
	if binding.Protocol != modelinvoker.ProtocolGenerateContent {
		return nil, fmt.Errorf("create GenerateContent driver: binding protocol must be %q", modelinvoker.ProtocolGenerateContent)
	}
	if protocol.IsNil(client) {
		return nil, fmt.Errorf("create GenerateContent driver: client is nil")
	}
	base, err := protocol.NewBase(binding, dialect)
	if err != nil {
		return nil, fmt.Errorf("create GenerateContent driver: %w", err)
	}
	return &Driver{base: base, client: client}, nil
}

func (d *Driver) Binding() protocol.Binding {
	if d == nil || d.base == nil {
		return protocol.Binding{}
	}
	return d.base.Binding()
}

func (d *Driver) Invoke(ctx context.Context, request modelinvoker.Request) (modelinvoker.Response, error) {
	if d == nil || d.base == nil || protocol.IsNil(d.client) {
		return modelinvoker.Response{}, driverError(modelinvoker.ErrorProviderUnavailable, "generate_content.create", "protocol driver is not initialized")
	}
	if ctx == nil {
		return modelinvoker.Response{}, d.base.StampError(nil, request,
			driverError(modelinvoker.ErrorInvalidRequest, "generate_content.create", "context is nil"), "generate_content.create")
	}
	request.Stream = false
	if request.State != nil && request.State.Kind != modelinvoker.StateProviderContinuation {
		return modelinvoker.Response{}, d.base.StampError(ctx, request,
			mappingError("generate_content.validate", "GenerateContent requires provider continuation state"), "generate_content.validate")
	}
	if err := d.base.Validate(request); err != nil {
		return modelinvoker.Response{}, err
	}
	mapped, err := buildMappedRequest(request)
	if err != nil {
		return d.failedResponse(request, mapped.raw, modelinvoker.RawPayload{}, mapped.decisions),
			d.base.StampError(ctx, request, err, "generate_content.map")
	}
	binding := d.Binding()
	callContext, capture := adaptercore.WithResponseCapture(ctx, false, binding.Provider)
	native, headers, err := d.client.GenerateContent(callContext, request.Model, mapped.contents, mapped.config)
	captured := capture.Snapshot()
	headers = capturedHeaders(captured, headers)
	if limitErr := capture.Err(); limitErr != nil {
		err = limitErr
	}
	if err != nil {
		rawResponse := modelinvoker.RawPayload{}
		var rawErr error
		if len(captured.Body) > 0 {
			rawResponse = modelinvoker.NewRawPayload(captured.Body)
		} else {
			rawResponse, rawErr = rawErrorPayload(err)
		}
		response := d.failedResponse(request, mapped.raw, rawResponse, mapped.decisions)
		if rawErr != nil {
			return response, d.base.StampError(ctx, request,
				protocolError("generate_content.error_audit", "failed to construct controlled GenerateContent error payload", d.base.RequestID(headers)),
				"generate_content.error_audit")
		}
		normalized := normalizeFailure(ctx, d.base, request, "generate_content.create", headers, err)
		var invocationError *modelinvoker.Error
		if response.RequestID == "" && errors.As(normalized, &invocationError) && invocationError != nil {
			response.RequestID = invocationError.RequestID
		}
		return response, normalized
	}
	response, normalizeErr := normalizeGenerateContent(d.base, request, native, headers, mapped.envelope)
	if len(captured.Body) > 0 {
		response.RawResponse = modelinvoker.NewRawPayload(captured.Body)
	}
	response.RawRequest = mapped.raw
	response.MappingReport.Decisions = append(response.MappingReport.Decisions, mapped.decisions...)
	response = d.base.StampResponse(request, response)
	return response, d.base.StampError(ctx, request, normalizeErr, "generate_content.normalize")
}

func (d *Driver) Stream(ctx context.Context, request modelinvoker.Request) (modelinvoker.Stream, error) {
	if d == nil || d.base == nil || protocol.IsNil(d.client) {
		return nil, driverError(modelinvoker.ErrorProviderUnavailable, "generate_content.stream", "protocol driver is not initialized")
	}
	if ctx == nil {
		return nil, d.base.StampError(nil, request,
			driverError(modelinvoker.ErrorInvalidRequest, "generate_content.stream", "context is nil"), "generate_content.stream")
	}
	request.Stream = true
	if request.State != nil && request.State.Kind != modelinvoker.StateProviderContinuation {
		return nil, d.base.StampError(ctx, request,
			mappingError("generate_content.validate", "GenerateContent requires provider continuation state"), "generate_content.validate")
	}
	if err := d.base.Validate(request); err != nil {
		return nil, err
	}
	mapped, err := buildMappedRequest(request)
	if err != nil {
		return nil, d.base.StampError(ctx, request, err, "generate_content.map")
	}
	streamContext, cancel := context.WithCancel(ctx)
	callContext, capture := adaptercore.WithResponseCapture(streamContext, true, d.Binding().Provider)
	native, headers, err := d.client.GenerateContentStream(callContext, request.Model, mapped.contents, mapped.config)
	headers = capturedHeaders(capture.Snapshot(), headers)
	if limitErr := capture.Err(); limitErr != nil {
		err = limitErr
	}
	if err != nil {
		normalized := normalizeFailure(streamContext, d.base, request, "generate_content.stream", headers, err)
		cancel()
		return nil, normalized
	}
	if protocol.IsNil(native) {
		cancel()
		return nil, d.base.StampError(ctx, request,
			driverError(modelinvoker.ErrorStreamInterrupted, "generate_content.stream", "native client returned a nil stream"), "generate_content.stream")
	}
	if err := native.Err(); err != nil {
		normalized := normalizeFailure(streamContext, d.base, request, "generate_content.stream", headers, err)
		_ = native.Close()
		cancel()
		return nil, normalized
	}
	stream := adaptercore.StreamWithResponseCapture(newGenerateContentStream(
		streamContext, cancel, request, d.base, native, headers, mapped.raw, mapped.decisions, mapped.envelope,
	), capture)
	return d.base.BindStream(ctx, request, stream), nil
}

func (d *Driver) failedResponse(
	request modelinvoker.Request,
	rawRequest modelinvoker.RawPayload,
	rawResponse modelinvoker.RawPayload,
	decisions []modelinvoker.MappingDecision,
) modelinvoker.Response {
	return d.base.StampResponse(request, modelinvoker.Response{
		Protocol: modelinvoker.ProtocolGenerateContent, Model: request.Model, Status: modelinvoker.ResponseStatusFailed,
		RawRequest: rawRequest, RawResponse: rawResponse,
		MappingReport: modelinvoker.MappingReport{Protocol: modelinvoker.ProtocolGenerateContent, Endpoint: request.Endpoint,
			Decisions: append([]modelinvoker.MappingDecision(nil), decisions...)},
	})
}

func capturedHeaders(captured adaptercore.CapturedResponse, fallback http.Header) http.Header {
	if captured.Header != nil {
		return captured.Header.Clone()
	}
	return fallback
}

var _ protocol.Driver = (*Driver)(nil)
