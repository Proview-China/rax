package adaptercore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strings"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
)

const secretReplacement = "[REDACTED]"

// Redactor is an immutable, concurrency-safe secret scrubber. It recognizes
// the exact secret plus the forms produced by JSON and URL escaping.
type Redactor struct {
	patterns []string
}

func (Redactor) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "adaptercore.Redactor([REDACTED])")
}

func (Redactor) GoString() string {
	return "adaptercore.Redactor([REDACTED])"
}

func NewRedactor(secrets ...string) Redactor {
	unique := make(map[string]struct{})
	for _, secret := range secrets {
		if secret == "" {
			continue
		}
		unique[secret] = struct{}{}
		if encoded, err := json.Marshal(secret); err == nil && len(encoded) >= 2 {
			unique[string(encoded[1:len(encoded)-1])] = struct{}{}
		}
		unique[url.QueryEscape(secret)] = struct{}{}
		unique[url.PathEscape(secret)] = struct{}{}
	}
	patterns := make([]string, 0, len(unique))
	for pattern := range unique {
		if pattern != "" {
			patterns = append(patterns, pattern)
		}
	}
	sort.Slice(patterns, func(i, j int) bool {
		if len(patterns[i]) == len(patterns[j]) {
			return patterns[i] < patterns[j]
		}
		return len(patterns[i]) > len(patterns[j])
	})
	return Redactor{patterns: patterns}
}

func (r Redactor) String(value string) string {
	for _, pattern := range r.patterns {
		value = strings.ReplaceAll(value, pattern, secretReplacement)
	}
	return value
}

func (r Redactor) Bytes(value []byte) []byte {
	if len(value) == 0 {
		return nil
	}
	return []byte(r.String(string(value)))
}

func (r Redactor) Raw(value modelinvoker.RawPayload) modelinvoker.RawPayload {
	return modelinvoker.NewRawPayload(r.Bytes(value.Bytes()))
}

func (r Redactor) State(value *modelinvoker.State) *modelinvoker.State {
	if value == nil {
		return nil
	}
	return &modelinvoker.State{
		Kind:     modelinvoker.StateKind(r.String(string(value.Kind))),
		Provider: modelinvoker.ProviderID(r.String(string(value.Provider))),
		Protocol: modelinvoker.Protocol(r.String(string(value.Protocol))),
		ID:       r.String(value.ID),
		Payload:  r.Raw(value.Payload),
	}
}

func (r Redactor) MappingReport(value modelinvoker.MappingReport) modelinvoker.MappingReport {
	result := modelinvoker.MappingReport{
		Provider: modelinvoker.ProviderID(r.String(string(value.Provider))),
		Protocol: modelinvoker.Protocol(r.String(string(value.Protocol))),
		Endpoint: r.String(value.Endpoint),
	}
	if value.Decisions != nil {
		result.Decisions = make([]modelinvoker.MappingDecision, len(value.Decisions))
		for index, decision := range value.Decisions {
			result.Decisions[index] = modelinvoker.MappingDecision{
				Capability: modelinvoker.Capability(r.String(string(decision.Capability))),
				Action:     modelinvoker.MappingAction(r.String(string(decision.Action))),
				Detail:     r.String(decision.Detail),
			}
		}
	}
	return result
}

func (r Redactor) Response(value modelinvoker.Response) modelinvoker.Response {
	result := value
	result.ID = r.String(value.ID)
	result.Provider = modelinvoker.ProviderID(r.String(string(value.Provider)))
	result.Protocol = modelinvoker.Protocol(r.String(string(value.Protocol)))
	result.Model = r.String(value.Model)
	result.Status = modelinvoker.ResponseStatus(r.String(string(value.Status)))
	result.StopReason = modelinvoker.StopReason(r.String(string(value.StopReason)))
	result.StopSequence = r.String(value.StopSequence)
	result.RequestID = r.String(value.RequestID)
	result.Output = r.output(value.Output)
	result.Metadata = redactStringMap(r, value.Metadata)
	result.State = r.State(value.State)
	result.ProviderMetadata = redactStringMap(r, value.ProviderMetadata)
	result.MappingReport = r.MappingReport(value.MappingReport)
	result.RawRequest = r.Raw(value.RawRequest)
	result.RawResponse = r.Raw(value.RawResponse)
	if value.NativeEvents != nil {
		result.NativeEvents = make([]modelinvoker.RawPayload, len(value.NativeEvents))
		for index, raw := range value.NativeEvents {
			result.NativeEvents[index] = r.Raw(raw)
		}
	}
	return result
}

func (r Redactor) Error(value error) error {
	if value == nil {
		return nil
	}
	var invocationError *modelinvoker.Error
	if errors.As(value, &invocationError) && invocationError != nil {
		return r.invocationError(invocationError)
	}
	if errors.Is(value, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	if errors.Is(value, context.Canceled) {
		return context.Canceled
	}
	return r.safeCause(value)
}

func (r Redactor) StreamEvent(value modelinvoker.StreamEvent) modelinvoker.StreamEvent {
	result := value
	result.Type = modelinvoker.StreamEventType(r.String(string(value.Type)))
	result.ResponseID = r.String(value.ResponseID)
	result.TextDelta = r.String(value.TextDelta)
	result.ReasoningDelta = r.String(value.ReasoningDelta)
	result.ArgumentsDelta = r.String(value.ArgumentsDelta)
	result.FunctionCall = r.functionCall(value.FunctionCall)
	if value.Usage != nil {
		usage := *value.Usage
		result.Usage = &usage
	}
	if value.Response != nil {
		response := r.Response(*value.Response)
		result.Response = &response
	}
	if value.Error != nil {
		result.Error = r.invocationError(value.Error)
	}
	result.Raw = r.Raw(value.Raw)
	return result
}

// Stream wraps a provider stream so every public event, terminal error, and
// close error crosses the same redaction boundary.
func (r Redactor) Stream(value modelinvoker.Stream) modelinvoker.Stream {
	if value == nil {
		return nil
	}
	return &redactingStream{
		inner:     value,
		redactor:  r,
		arguments: make(map[string]*streamDeltaFilter),
	}
}

func (r Redactor) invocationError(value *modelinvoker.Error) *modelinvoker.Error {
	if value == nil {
		return nil
	}
	result := *value
	result.Kind = modelinvoker.ErrorKind(r.String(string(value.Kind)))
	result.Provider = modelinvoker.ProviderID(r.String(string(value.Provider)))
	result.Operation = r.String(value.Operation)
	result.Code = r.String(value.Code)
	result.Message = r.String(value.Message)
	result.RequestID = r.String(value.RequestID)
	result.MappingReport = r.MappingReport(value.MappingReport)
	result.Err = r.safeCause(value.Err)
	return &result
}

func (r Redactor) safeCause(value error) error {
	if value == nil {
		return nil
	}
	if errors.Is(value, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	if errors.Is(value, context.Canceled) {
		return context.Canceled
	}
	var invocationError *modelinvoker.Error
	if errors.As(value, &invocationError) && invocationError != nil {
		return r.invocationError(invocationError)
	}
	return errors.New(r.String(value.Error()))
}

func (r Redactor) output(value []modelinvoker.OutputItem) []modelinvoker.OutputItem {
	if value == nil {
		return nil
	}
	result := make([]modelinvoker.OutputItem, len(value))
	for index, item := range value {
		result[index] = modelinvoker.OutputItem{
			Type:             modelinvoker.OutputItemType(r.String(string(item.Type))),
			Text:             r.String(item.Text),
			FunctionCall:     r.functionCall(item.FunctionCall),
			ReasoningSummary: r.String(item.ReasoningSummary),
		}
	}
	return result
}

func (r Redactor) functionCall(value *modelinvoker.FunctionCall) *modelinvoker.FunctionCall {
	if value == nil {
		return nil
	}
	return &modelinvoker.FunctionCall{
		ID:        r.String(value.ID),
		Name:      r.String(value.Name),
		Arguments: json.RawMessage(r.Bytes(value.Arguments)),
	}
}

func redactStringMap[M ~map[string]string](redactor Redactor, value M) M {
	if value == nil {
		return nil
	}
	result := make(M, len(value))
	for key, item := range value {
		result[redactor.String(key)] = redactor.String(item)
	}
	return result
}

type redactingStream struct {
	inner        modelinvoker.Stream
	redactor     Redactor
	current      modelinvoker.StreamEvent
	queue        []modelinvoker.StreamEvent
	err          error
	ended        bool
	sequence     int64
	inputOrder   int64
	auditTainted bool
	text         streamDeltaFilter
	reasoning    streamDeltaFilter
	arguments    map[string]*streamDeltaFilter
}

func (*redactingStream) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "adaptercore.redactingStream([REDACTED])")
}

func (*redactingStream) GoString() string {
	return "adaptercore.redactingStream([REDACTED])"
}

func (s *redactingStream) Next() bool {
	for len(s.queue) == 0 && !s.ended {
		if !s.inner.Next() {
			s.ended = true
			s.err = s.redactor.Error(s.inner.Err())
			s.flushPending()
			break
		}
		s.inputOrder++
		native := s.inner.Event()
		redacted := s.redactor.StreamEvent(native)
		if streamEventTerminates(redacted.Type) {
			s.flushPending()
		}
		s.redactDeltas(native, &redacted)
		if s.auditTainted {
			s.redactStreamAudit(&redacted)
		}
		s.enqueue(redacted)
	}
	if len(s.queue) == 0 {
		s.current = modelinvoker.StreamEvent{}
		return false
	}
	s.current = s.queue[0]
	s.queue = s.queue[1:]
	return true
}

func (s *redactingStream) Event() modelinvoker.StreamEvent {
	return s.current
}

func (s *redactingStream) Err() error {
	return s.err
}

func (s *redactingStream) Close() error {
	return s.redactor.Error(s.inner.Close())
}

type streamDeltaFilter struct {
	pending      string
	eventType    modelinvoker.StreamEventType
	responseID   string
	functionCall *modelinvoker.FunctionCall
	order        int64
}

func (s *redactingStream) redactDeltas(native modelinvoker.StreamEvent, redacted *modelinvoker.StreamEvent) {
	if native.TextDelta != "" {
		redacted.TextDelta = s.consumeDelta(&s.text, native.TextDelta, modelinvoker.StreamEventTextDelta, redacted)
	}
	if native.ReasoningDelta != "" {
		redacted.ReasoningDelta = s.consumeDelta(&s.reasoning, native.ReasoningDelta, modelinvoker.StreamEventReasoningDelta, redacted)
	}
	if native.ArgumentsDelta != "" {
		key := streamArgumentsKey(native)
		filter := s.arguments[key]
		if filter == nil {
			filter = &streamDeltaFilter{}
			s.arguments[key] = filter
		}
		redacted.ArgumentsDelta = s.consumeDelta(filter, native.ArgumentsDelta, modelinvoker.StreamEventFunctionArgumentsDelta, redacted)
		if filter.pending == "" {
			delete(s.arguments, key)
		}
	}
}

func (s *redactingStream) consumeDelta(
	filter *streamDeltaFilter,
	value string,
	eventType modelinvoker.StreamEventType,
	event *modelinvoker.StreamEvent,
) string {
	previousPending := filter.pending
	output, pending := s.redactor.redactStreamChunk(previousPending+value, false)
	filter.pending = pending
	filter.eventType = eventType
	filter.responseID = event.ResponseID
	filter.functionCall = s.redactor.functionCall(event.FunctionCall)
	filter.order = s.inputOrder
	if previousPending != "" || pending != "" || output != value {
		s.auditTainted = true
		event.Raw = controlledStreamPayload()
	}
	return output
}

func (s *redactingStream) flushPending() {
	filters := make([]*streamDeltaFilter, 0, len(s.arguments)+2)
	if s.text.pending != "" {
		filters = append(filters, &s.text)
	}
	if s.reasoning.pending != "" {
		filters = append(filters, &s.reasoning)
	}
	for _, filter := range s.arguments {
		if filter.pending != "" {
			filters = append(filters, filter)
		}
	}
	sort.SliceStable(filters, func(i, j int) bool { return filters[i].order < filters[j].order })
	for _, filter := range filters {
		output, _ := s.redactor.redactStreamChunk(filter.pending, true)
		filter.pending = ""
		if output == "" {
			continue
		}
		event := modelinvoker.StreamEvent{
			Type:         filter.eventType,
			ResponseID:   filter.responseID,
			FunctionCall: s.redactor.functionCall(filter.functionCall),
			Raw:          controlledStreamPayload(),
		}
		switch filter.eventType {
		case modelinvoker.StreamEventTextDelta:
			event.TextDelta = output
		case modelinvoker.StreamEventReasoningDelta:
			event.ReasoningDelta = output
		case modelinvoker.StreamEventFunctionArgumentsDelta:
			event.ArgumentsDelta = output
		}
		s.auditTainted = true
		s.enqueue(event)
	}
}

func (s *redactingStream) enqueue(event modelinvoker.StreamEvent) {
	next := s.sequence + 1
	if event.Sequence > next {
		next = event.Sequence
	}
	s.sequence = next
	event.Sequence = next
	s.queue = append(s.queue, event)
}

func (s *redactingStream) redactStreamAudit(event *modelinvoker.StreamEvent) {
	if event.Raw.Empty() || string(event.Raw.Bytes()) != secretReplacement {
		event.Raw = controlledStreamPayload()
	}
	if event.Response == nil {
		return
	}
	response := *event.Response
	response.RawResponse = controlledStreamPayload()
	if response.NativeEvents != nil {
		response.NativeEvents = make([]modelinvoker.RawPayload, len(response.NativeEvents))
		for index := range response.NativeEvents {
			response.NativeEvents[index] = controlledStreamPayload()
		}
	}
	event.Response = &response
}

func (r Redactor) redactStreamChunk(value string, final bool) (string, string) {
	if value == "" || len(r.patterns) == 0 {
		return value, ""
	}
	var result strings.Builder
	result.Grow(len(value))
	for offset := 0; offset < len(value); {
		matched := ""
		for _, pattern := range r.patterns {
			if strings.HasPrefix(value[offset:], pattern) {
				matched = pattern
				break
			}
		}
		if matched != "" {
			result.WriteString(secretReplacement)
			offset += len(matched)
			continue
		}
		if !final {
			suffix := value[offset:]
			for _, pattern := range r.patterns {
				if len(suffix) < len(pattern) && strings.HasPrefix(pattern, suffix) {
					return result.String(), suffix
				}
			}
		}
		result.WriteByte(value[offset])
		offset++
	}
	return result.String(), ""
}

func streamArgumentsKey(event modelinvoker.StreamEvent) string {
	if event.FunctionCall != nil {
		if event.FunctionCall.ID != "" {
			return "id:" + event.FunctionCall.ID
		}
		if event.FunctionCall.Name != "" {
			return "name:" + event.FunctionCall.Name
		}
	}
	return "response:" + event.ResponseID
}

func streamEventTerminates(eventType modelinvoker.StreamEventType) bool {
	return eventType == modelinvoker.StreamEventResponseCompleted || eventType == modelinvoker.StreamEventError
}

func controlledStreamPayload() modelinvoker.RawPayload {
	return modelinvoker.NewRawPayload([]byte(secretReplacement))
}
