package modelinvoker

import (
	"context"
	"errors"
	"fmt"
	"math"
	"reflect"
	"time"
)

type RetryPolicy struct {
	MaxAttempts    int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
	Multiplier     float64
}

func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{MaxAttempts: 1, Multiplier: 2}
}

func (p RetryPolicy) validate() error {
	if p.MaxAttempts < 1 {
		return fmt.Errorf("max attempts must be at least 1")
	}
	if p.InitialBackoff < 0 || p.MaxBackoff < 0 {
		return fmt.Errorf("retry backoff must not be negative")
	}
	if p.MaxBackoff > 0 && p.InitialBackoff > p.MaxBackoff {
		return fmt.Errorf("initial backoff must not exceed maximum backoff")
	}
	if p.Multiplier < 1 {
		return fmt.Errorf("retry multiplier must be at least 1")
	}
	return nil
}

type SleepFunc func(context.Context, time.Duration) error

type InvokerOption func(*Invoker) error

func WithRetryPolicy(policy RetryPolicy) InvokerOption {
	return func(invoker *Invoker) error {
		if err := policy.validate(); err != nil {
			return err
		}
		invoker.retry = policy
		return nil
	}
}

func WithSleeper(sleeper SleepFunc) InvokerOption {
	return func(invoker *Invoker) error {
		if sleeper == nil {
			return fmt.Errorf("sleeper must not be nil")
		}
		invoker.sleep = sleeper
		return nil
	}
}

type Invoker struct {
	registry *Registry
	retry    RetryPolicy
	sleep    SleepFunc
}

func NewInvoker(registry *Registry, options ...InvokerOption) (*Invoker, error) {
	if registry == nil {
		return nil, &Error{Kind: ErrorInvalidRequest, Operation: "new_invoker", Message: "registry is required"}
	}
	invoker := &Invoker{
		registry: registry,
		retry:    DefaultRetryPolicy(),
		sleep:    sleepContext,
	}
	for _, option := range options {
		if option == nil {
			return nil, &Error{Kind: ErrorInvalidRequest, Operation: "new_invoker", Message: "invoker option is nil"}
		}
		if err := option(invoker); err != nil {
			return nil, &Error{Kind: ErrorInvalidRequest, Operation: "new_invoker", Message: err.Error(), Err: err}
		}
	}
	return invoker, nil
}

func (i *Invoker) Invoke(ctx context.Context, request Request) (Response, error) {
	request.Stream = false
	if ctx == nil {
		return Response{}, &Error{Kind: ErrorInvalidRequest, Provider: request.Provider, Operation: "prepare", Message: "context is nil"}
	}
	callContext, cancel := requestContext(ctx, request.Budget.Timeout)
	defer cancel()
	provider, report, err := i.prepare(callContext, &request)
	if err != nil {
		return Response{}, err
	}

	var response Response
	for attempt := 1; attempt <= i.retry.MaxAttempts; attempt++ {
		response, err = provider.Invoke(callContext, request)
		if err == nil {
			response = completeResponse(response, request, report)
			return response, nil
		}
		err = normalizeContextError(callContext, request.Provider, "invoke", err)
		if attempt == i.retry.MaxAttempts || !isRetryable(err) {
			response = completeResponse(response, request, report)
			return response, withMappingReport(err, response.MappingReport)
		}
		delay := i.backoff(attempt, err)
		if sleepErr := i.sleep(callContext, delay); sleepErr != nil {
			response = completeResponse(response, request, report)
			err = normalizeContextError(callContext, request.Provider, "retry_wait", sleepErr)
			return response, withMappingReport(err, response.MappingReport)
		}
	}
	return Response{}, err
}

func (i *Invoker) Stream(ctx context.Context, request Request) (Stream, error) {
	request.Stream = true
	if ctx == nil {
		return nil, &Error{Kind: ErrorInvalidRequest, Provider: request.Provider, Operation: "prepare", Message: "context is nil"}
	}
	callContext, cancel := requestContext(ctx, request.Budget.Timeout)
	provider, report, err := i.prepare(callContext, &request)
	if err != nil {
		cancel()
		return nil, err
	}
	stream, err := provider.Stream(callContext, request)
	if err != nil {
		cancel()
		err = normalizeContextError(callContext, request.Provider, "stream", err)
		return nil, withMappingReport(err, report)
	}
	if nilStream(stream) {
		cancel()
		err = &Error{Kind: ErrorStreamInterrupted, Provider: request.Provider, Operation: "stream", Message: "provider returned a nil stream"}
		return nil, withMappingReport(err, report)
	}
	return &mappedStream{inner: stream, ctx: callContext, cancel: cancel, request: request, report: report}, nil
}

func (i *Invoker) prepare(ctx context.Context, request *Request) (Provider, MappingReport, error) {
	if i == nil || i.registry == nil {
		return nil, MappingReport{}, &Error{Kind: ErrorInvalidRequest, Operation: "prepare", Message: "invoker is not initialized"}
	}
	if ctx == nil {
		return nil, MappingReport{}, &Error{Kind: ErrorInvalidRequest, Operation: "prepare", Message: "context is nil"}
	}
	if err := ctx.Err(); err != nil {
		return nil, MappingReport{}, normalizeContextError(ctx, request.Provider, "prepare", err)
	}
	if err := request.Validate(); err != nil {
		return nil, MappingReport{}, err
	}
	provider, err := i.registry.Get(request.Provider)
	if err != nil {
		return nil, MappingReport{}, err
	}
	if request.Protocol == ProtocolAuto {
		request.Protocol = provider.DefaultProtocol()
	}
	if err := validateState(request.State, request.Provider, request.Protocol); err != nil {
		return nil, MappingReport{}, newValidationError(err.Error())
	}
	contract, err := provider.Capabilities(ctx, CapabilityQuery{Protocol: request.Protocol, Endpoint: request.Endpoint, Model: request.Model})
	if err != nil {
		return nil, MappingReport{}, normalizeContextError(ctx, request.Provider, "capabilities", err)
	}
	report, err := EvaluateCapabilities(*request, contract)
	if err != nil {
		return nil, report, withMappingReport(err, report)
	}
	return provider, report, nil
}

func (i *Invoker) backoff(completedAttempts int, err error) time.Duration {
	delay := i.retry.InitialBackoff
	if delay > 0 && completedAttempts > 1 {
		delay = time.Duration(float64(delay) * math.Pow(i.retry.Multiplier, float64(completedAttempts-1)))
	}
	if i.retry.MaxBackoff > 0 && delay > i.retry.MaxBackoff {
		delay = i.retry.MaxBackoff
	}
	var invocationError *Error
	if errors.As(err, &invocationError) && invocationError != nil && invocationError.RetryAfter > delay {
		delay = invocationError.RetryAfter
	}
	return delay
}

func completeResponse(response Response, request Request, report MappingReport) Response {
	if response.Provider == "" {
		response.Provider = request.Provider
	}
	if response.Protocol == ProtocolAuto {
		response.Protocol = request.Protocol
	}
	response.MappingReport = mergeMappingReports(report, response.MappingReport)
	return response
}

func isRetryable(err error) bool {
	var invocationError *Error
	return errors.As(err, &invocationError) && invocationError != nil && invocationError.Retryable
}

func normalizeContextError(ctx context.Context, provider ProviderID, operation string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return &Error{Kind: ErrorTimeout, Provider: provider, Operation: operation, Message: "operation timed out", Err: err}
	}
	if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
		return &Error{Kind: ErrorCancelled, Provider: provider, Operation: operation, Message: "operation was cancelled", Err: err}
	}
	var invocationError *Error
	if errors.As(err, &invocationError) && invocationError != nil {
		copy := *invocationError
		copy.MappingReport = cloneMappingReport(invocationError.MappingReport)
		if copy.Provider == "" {
			copy.Provider = provider
		}
		if copy.Operation == "" {
			copy.Operation = operation
		}
		return &copy
	}
	return &Error{Kind: ErrorProvider, Provider: provider, Operation: operation, Message: "provider operation failed", Err: err}
}

func requestContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout > 0 {
		return context.WithTimeout(ctx, timeout)
	}
	return context.WithCancel(ctx)
}

func sleepContext(ctx context.Context, duration time.Duration) error {
	if duration <= 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			return nil
		}
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

type mappedStream struct {
	inner       Stream
	ctx         context.Context
	cancel      context.CancelFunc
	request     Request
	report      MappingReport
	event       StreamEvent
	err         error
	closed      bool
	done        bool
	innerClosed bool
}

func (s *mappedStream) Next() bool {
	if s.closed || s.done {
		return false
	}
	if !s.inner.Next() {
		s.done = true
		s.err = normalizeContextError(s.ctx, s.request.Provider, "stream", s.inner.Err())
		if closeErr := s.closeInner(); s.err == nil && closeErr != nil {
			s.err = normalizeContextError(s.ctx, s.request.Provider, "stream_close", closeErr)
		}
		s.err = withMappingReport(s.err, s.report)
		s.cancel()
		return false
	}
	s.event = s.inner.Event()
	if s.event.Error != nil {
		err := normalizeContextError(s.ctx, s.request.Provider, "stream", s.event.Error)
		err = withMappingReport(err, s.report)
		var invocationError *Error
		if errors.As(err, &invocationError) && invocationError != nil {
			s.event.Error = invocationError
		}
	}
	if s.event.Response != nil {
		response := completeResponse(*s.event.Response, s.request, s.report)
		s.event.Response = &response
	}
	return true
}

func (s *mappedStream) Event() StreamEvent { return s.event }

func (s *mappedStream) Err() error {
	return s.err
}

func (s *mappedStream) Close() error {
	if s.closed {
		return nil
	}
	s.closed = true
	s.done = true
	err := s.closeInner()
	s.cancel()
	if err != nil {
		return withMappingReport(normalizeContextError(s.ctx, s.request.Provider, "stream_close", err), s.report)
	}
	return nil
}

func (s *mappedStream) closeInner() error {
	if s.innerClosed {
		return nil
	}
	s.innerClosed = true
	return s.inner.Close()
}

func nilStream(stream Stream) bool {
	if stream == nil {
		return true
	}
	value := reflect.ValueOf(stream)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

func withMappingReport(err error, report MappingReport) error {
	if err == nil {
		return nil
	}
	var invocationError *Error
	if !errors.As(err, &invocationError) || invocationError == nil {
		return err
	}
	copy := *invocationError
	copy.MappingReport = mergeMappingReports(report, invocationError.MappingReport)
	return &copy
}

func mergeMappingReports(base, additional MappingReport) MappingReport {
	result := cloneMappingReport(base)
	if result.Provider == "" {
		result.Provider = additional.Provider
	}
	if result.Protocol == ProtocolAuto {
		result.Protocol = additional.Protocol
	}
	if result.Endpoint == "" {
		result.Endpoint = additional.Endpoint
	}
	result.Decisions = append(result.Decisions, additional.Decisions...)
	return result
}

func cloneMappingReport(report MappingReport) MappingReport {
	report.Decisions = append([]MappingDecision(nil), report.Decisions...)
	return report
}
