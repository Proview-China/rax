package operation

import (
	"context"
	"errors"
	"fmt"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
)

type Invoker struct{ registry *Registry }

func NewInvoker(registry *Registry) (*Invoker, error) {
	if registry == nil {
		return nil, operationError("", modelinvoker.ErrorInvalidRequest, "new_invoker", "", "operation registry is required")
	}
	return &Invoker{registry: registry}, nil
}

func (i *Invoker) Invoke(ctx context.Context, request Request) (Result, error) {
	request = cloneRequest(request)
	provider, report, call, cancel, err := i.prepare(ctx, request)
	if err != nil {
		return Result{}, err
	}
	defer cancel()
	result, err := provider.Invoke(call, request)
	result = cloneResult(result)
	if validationErr := validateProviderResult(result, request); validationErr != nil {
		return Result{MappingReport: report}, validationErr
	}
	result = complete(result, request, report)
	if err != nil {
		return result, normalizeError(call, request.Provider, "invoke", err)
	}
	return result, nil
}

func (i *Invoker) Stream(ctx context.Context, request Request) (Stream, error) {
	request = cloneRequest(request)
	provider, report, call, cancel, err := i.prepare(ctx, request)
	if err != nil {
		return nil, err
	}
	stream, err := provider.Stream(call, request)
	if err != nil {
		cancel()
		return nil, normalizeError(call, request.Provider, "stream", err)
	}
	if stream == nil {
		cancel()
		return nil, operationError(request.Provider, modelinvoker.ErrorStreamInterrupted, "stream", "", "operation provider returned a nil stream")
	}
	return &contextStream{inner: stream, cancel: cancel, ctx: call, request: request, report: report}, nil
}

func (i *Invoker) prepare(ctx context.Context, request Request) (Provider, MappingReport, context.Context, context.CancelFunc, error) {
	if i == nil || i.registry == nil {
		return nil, MappingReport{}, nil, nil, operationError(request.Provider, modelinvoker.ErrorInvalidRequest, "prepare", "", "operation invoker is not initialized")
	}
	if ctx == nil {
		return nil, MappingReport{}, nil, nil, operationError(request.Provider, modelinvoker.ErrorInvalidRequest, "prepare", "", "context is nil")
	}
	if err := request.Validate(); err != nil {
		return nil, MappingReport{}, nil, nil, operationError(request.Provider, modelinvoker.ErrorInvalidRequest, "validate", "", err.Error())
	}
	provider, err := i.registry.Get(request.Provider)
	if err != nil {
		return nil, MappingReport{}, nil, nil, err
	}
	contract, err := provider.Capabilities(ctx, Query{Kind: request.Kind, Model: request.Model})
	if err != nil {
		return nil, MappingReport{}, nil, nil, err
	}
	report, err := Evaluate(request, contract)
	if err != nil {
		return nil, report, nil, nil, err
	}
	call, cancel := context.WithCancel(ctx)
	if request.Budget.Timeout > 0 {
		call, cancel = context.WithTimeout(ctx, request.Budget.Timeout)
	}
	return provider, report, call, cancel, nil
}

func complete(result Result, request Request, report MappingReport) Result {
	if result.Provider == "" {
		result.Provider = request.Provider
	}
	if result.Kind == "" {
		result.Kind = request.Kind
	}
	if result.Model == "" {
		result.Model = request.Model
	}
	result.MappingReport = report
	return result
}

func normalizeError(ctx context.Context, provider modelinvoker.ProviderID, operation string, err error) error {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return operationError(provider, modelinvoker.ErrorTimeout, operation, "", "operation timed out")
	}
	if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
		return operationError(provider, modelinvoker.ErrorCancelled, operation, "", "operation was cancelled")
	}
	var typed *modelinvoker.Error
	if errors.As(err, &typed) {
		return typed
	}
	return &modelinvoker.Error{Kind: modelinvoker.ErrorProvider, Provider: provider, Operation: operation, Message: "operation provider failed", Err: err}
}

type contextStream struct {
	inner       Stream
	cancel      context.CancelFunc
	ctx         context.Context
	request     Request
	report      MappingReport
	event       StreamEvent
	sequence    int64
	started     bool
	terminal    bool
	err         error
	closed      bool
	innerClosed bool
}

func (s *contextStream) Next() bool {
	if s == nil || s.inner == nil || s.closed || s.terminal || s.err != nil {
		return false
	}
	if s.ctx != nil && s.ctx.Err() != nil {
		s.err = normalizeError(s.ctx, s.request.Provider, "stream", s.ctx.Err())
		s.cancel()
		if closeErr := s.closeInner(); closeErr != nil {
			s.err = errors.Join(s.err, normalizeError(s.ctx, s.request.Provider, "stream_close", closeErr))
		}
		return false
	}
	if !s.started {
		s.started = true
		s.sequence++
		result := complete(Result{Status: StatusRunning}, s.request, s.report)
		s.event = StreamEvent{Type: StreamStarted, Sequence: s.sequence, Result: &result}
		return true
	}
	for s.inner.Next() {
		event := cloneStreamEvent(s.inner.Event())
		// The Invoker owns the public lifecycle. A transport may emit a native
		// start marker, but it must not create a second semantic start event.
		if event.Type == StreamStarted {
			continue
		}
		s.sequence++
		event.Sequence = s.sequence
		if event.Result != nil {
			if err := validateProviderResult(*event.Result, s.request); err != nil {
				s.err = err
				s.cancel()
				if closeErr := s.closeInner(); closeErr != nil {
					s.err = errors.Join(s.err, normalizeError(s.ctx, s.request.Provider, "stream_close", closeErr))
				}
				return false
			}
			completed := complete(*event.Result, s.request, s.report)
			event.Result = &completed
		}
		switch event.Type {
		case StreamCompleted:
			if event.Result == nil {
				result := complete(Result{Status: StatusSucceeded}, s.request, s.report)
				event.Result = &result
			} else if event.Result.Status == StatusUnknown || event.Result.Status == "" {
				event.Result.Status = StatusSucceeded
			}
			s.terminal = true
			s.cancel()
			if closeErr := s.closeInner(); closeErr != nil {
				s.err = normalizeError(s.ctx, s.request.Provider, "stream_close", closeErr)
			}
		case StreamError:
			s.terminal = true
			s.cancel()
			if closeErr := s.closeInner(); closeErr != nil {
				s.err = normalizeError(s.ctx, s.request.Provider, "stream_close", closeErr)
			}
			if event.Error == nil {
				event.Error = operationError(s.request.Provider, modelinvoker.ErrorProvider, "stream", "", "operation stream reported an error without details")
			}
		}
		s.event = event
		return true
	}
	if err := s.inner.Err(); err != nil {
		s.err = normalizeError(s.ctx, s.request.Provider, "stream", err)
		s.cancel()
		if closeErr := s.closeInner(); closeErr != nil {
			s.err = errors.Join(s.err, normalizeError(s.ctx, s.request.Provider, "stream_close", closeErr))
		}
		return false
	}
	if s.ctx != nil && s.ctx.Err() != nil {
		s.err = normalizeError(s.ctx, s.request.Provider, "stream", s.ctx.Err())
		s.cancel()
		if closeErr := s.closeInner(); closeErr != nil {
			s.err = errors.Join(s.err, normalizeError(s.ctx, s.request.Provider, "stream_close", closeErr))
		}
		return false
	}
	// Provider transports are not trusted to supply a terminal marker. Normal
	// exhaustion is projected into exactly one successful completion event.
	s.sequence++
	result := complete(Result{Status: StatusSucceeded}, s.request, s.report)
	s.event = StreamEvent{Type: StreamCompleted, Sequence: s.sequence, Result: &result}
	s.terminal = true
	s.cancel()
	if closeErr := s.closeInner(); closeErr != nil {
		s.err = normalizeError(s.ctx, s.request.Provider, "stream_close", closeErr)
	}
	return true
}

func validateProviderResult(result Result, request Request) error {
	if result.Provider != "" && result.Provider != request.Provider {
		return operationError(request.Provider, modelinvoker.ErrorMapping, "validate_result", "provider", "operation result provider does not match the selected provider")
	}
	if result.Kind != "" && result.Kind != request.Kind {
		return operationError(request.Provider, modelinvoker.ErrorMapping, "validate_result", "kind", "operation result kind does not match the requested primitive")
	}
	if result.Model != "" && result.Model != request.Model {
		return operationError(request.Provider, modelinvoker.ErrorMapping, "validate_result", "model", "operation result model does not match the selected model")
	}
	switch result.Status {
	case "", StatusUnknown, StatusQueued, StatusValidating, StatusRunning, StatusFinalizing,
		StatusSucceeded, StatusFailed, StatusCancelling, StatusCancelled, StatusExpired:
	default:
		return operationError(request.Provider, modelinvoker.ErrorMapping, "validate_result", "status", "operation result status is outside the stable state machine")
	}
	for index, artifact := range result.Artifacts {
		if err := ValidateArtifact(artifact); err != nil {
			return operationError(request.Provider, modelinvoker.ErrorMapping, "validate_result", "artifact", fmt.Sprintf("operation result artifact %d is invalid: %s", index, err))
		}
		if artifact.URL != "" && artifact.ExpiresAt.IsZero() && !artifact.ExpiryUnknown {
			return operationError(request.Provider, modelinvoker.ErrorMapping, "validate_result", "artifact_expiry", "operation result URL must declare expiry or explicitly mark it unknown")
		}
	}
	return nil
}
func (s *contextStream) Event() StreamEvent {
	if s == nil || s.inner == nil {
		return StreamEvent{}
	}
	return cloneStreamEvent(s.event)
}

func cloneStreamEvent(event StreamEvent) StreamEvent {
	event.Chunk = append([]byte(nil), event.Chunk...)
	event.Raw = modelinvoker.NewRawPayload(event.Raw.Bytes())
	if event.Usage != nil {
		usage := *event.Usage
		event.Usage = &usage
	}
	if event.Error != nil {
		typed := *event.Error
		event.Error = &typed
	}
	if event.Result != nil {
		result := cloneResult(*event.Result)
		event.Result = &result
	}
	return event
}

func cloneRequest(request Request) Request {
	request.Body = modelinvoker.NewRawPayload(request.Body.Bytes())
	request.Query = cloneStringSlices(request.Query)
	request.Metadata = cloneMetadata(request.Metadata)
	request.ProviderOptions = cloneProviderOptions(request.ProviderOptions)
	return request
}

func cloneResult(result Result) Result {
	if result.Job != nil {
		job := *result.Job
		result.Job = &job
	}
	if result.Resource != nil {
		resource := *result.Resource
		result.Resource = &resource
	}
	result.Artifacts = append([]Artifact(nil), result.Artifacts...)
	for index := range result.Artifacts {
		result.Artifacts[index].Data = append([]byte(nil), result.Artifacts[index].Data...)
		if result.Artifacts[index].Metadata != nil {
			metadata := make(map[string]string, len(result.Artifacts[index].Metadata))
			for key, value := range result.Artifacts[index].Metadata {
				metadata[key] = value
			}
			result.Artifacts[index].Metadata = metadata
		}
	}
	result.Vectors = append([]Vector(nil), result.Vectors...)
	for index := range result.Vectors {
		result.Vectors[index].Values = append([]float32(nil), result.Vectors[index].Values...)
	}
	result.Rankings = append([]Ranking(nil), result.Rankings...)
	if result.ProviderMetadata != nil {
		metadata := make(modelinvoker.ProviderMetadata, len(result.ProviderMetadata))
		for key, value := range result.ProviderMetadata {
			metadata[key] = value
		}
		result.ProviderMetadata = metadata
	}
	result.RawRequest = modelinvoker.NewRawPayload(result.RawRequest.Bytes())
	result.RawResponse = modelinvoker.NewRawPayload(result.RawResponse.Bytes())
	return result
}

func cloneStringSlices(input map[string][]string) map[string][]string {
	if input == nil {
		return nil
	}
	cloned := make(map[string][]string, len(input))
	for key, values := range input {
		cloned[key] = append([]string(nil), values...)
	}
	return cloned
}

func cloneMetadata(input modelinvoker.Metadata) modelinvoker.Metadata {
	if input == nil {
		return nil
	}
	cloned := make(modelinvoker.Metadata, len(input))
	for key, value := range input {
		cloned[key] = value
	}
	return cloned
}

func cloneProviderOptions(input modelinvoker.ProviderOptions) modelinvoker.ProviderOptions {
	if input == nil {
		return nil
	}
	cloned := make(modelinvoker.ProviderOptions, len(input))
	for key, value := range input {
		cloned[key] = append([]byte(nil), value...)
	}
	return cloned
}
func (s *contextStream) Err() error {
	if s == nil || s.inner == nil {
		return nil
	}
	return s.err
}
func (s *contextStream) Close() error {
	if s == nil || s.closed {
		return nil
	}
	s.closed = true
	if s.cancel != nil {
		s.cancel()
	}
	return s.closeInner()
}

func (s *contextStream) closeInner() error {
	if s == nil || s.inner == nil || s.innerClosed {
		return nil
	}
	s.innerClosed = true
	return s.inner.Close()
}
