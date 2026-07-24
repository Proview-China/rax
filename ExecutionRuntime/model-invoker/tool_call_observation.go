package modelinvoker

import (
	"bytes"
	"encoding/json"
	"io"
	"math/big"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const (
	ToolCallCandidateObservationContractVersionV1 = "praxis.model-invoker.tool-call-observation/v1"
	maxToolCallJSONDepthV1                        = 128
)

// ToolCallCandidateEntryV1 is model output only. It is not a PendingAction,
// ActionCandidate, authorization, permit, dispatch, settlement, or result.
type ToolCallCandidateEntryV1 struct {
	Ordinal            uint32          `json:"ordinal"`
	CallID             string          `json:"call_id"`
	Name               string          `json:"name"`
	CanonicalArguments json.RawMessage `json:"canonical_arguments"`
}

// ToolCallCandidateObservationV1 atomically describes every completed tool
// call in one completed/tool_call response. Runtime evidence and governance
// envelopes remain outside this provider-neutral payload.
type ToolCallCandidateObservationV1 struct {
	ContractVersion  string                     `json:"contract_version"`
	InvocationDigest core.Digest                `json:"invocation_digest"`
	ResponseStatus   ResponseStatus             `json:"response_status"`
	StopReason       StopReason                 `json:"stop_reason"`
	Calls            []ToolCallCandidateEntryV1 `json:"calls"`
	Digest           core.Digest                `json:"digest"`
}

// Clone returns a deep copy so callers cannot mutate a finalizer-owned result.
func (o ToolCallCandidateObservationV1) Clone() ToolCallCandidateObservationV1 {
	clone := o
	clone.Calls = make([]ToolCallCandidateEntryV1, len(o.Calls))
	for index, call := range o.Calls {
		clone.Calls[index] = call
		clone.Calls[index].CanonicalArguments = append(json.RawMessage(nil), call.CanonicalArguments...)
	}
	return clone
}

// Validate verifies the complete immutable observation, including its digest.
func (o ToolCallCandidateObservationV1) Validate() error {
	normalized, err := normalizeToolCallObservationV1(o)
	if err != nil {
		return err
	}
	if o.Digest.Validate() != nil {
		return toolCallObservationError(ErrorInvalidRequest, "tool_call_observation_digest_invalid", "tool call observation digest is invalid")
	}
	expected, err := digestToolCallObservationV1(normalized)
	if err != nil {
		return err
	}
	if expected != o.Digest {
		return toolCallObservationError(ErrorMapping, "tool_call_observation_digest_drift", "tool call observation digest drifted")
	}
	for index := range normalized.Calls {
		if !bytes.Equal(normalized.Calls[index].CanonicalArguments, o.Calls[index].CanonicalArguments) {
			return toolCallObservationError(ErrorMapping, "tool_call_arguments_not_canonical", "tool call arguments are not in canonical form")
		}
	}
	return nil
}

// FinalizeToolCallCandidateObservationV1 is the common sync/stream terminal
// finalizer. The caller supplies the already-authoritative invocation digest;
// Model Invoker never invents or upgrades that lineage.
func FinalizeToolCallCandidateObservationV1(invocationDigest core.Digest, response Response) (ToolCallCandidateObservationV1, error) {
	if invocationDigest.Validate() != nil {
		return ToolCallCandidateObservationV1{}, toolCallObservationError(ErrorInvalidRequest, "invocation_digest_invalid", "an authoritative invocation digest is required")
	}
	if response.Status != ResponseStatusCompleted || response.StopReason != StopReasonToolCall {
		return ToolCallCandidateObservationV1{}, toolCallObservationError(ErrorMapping, "tool_call_terminal_mismatch", "tool call observation requires a completed/tool_call response")
	}

	calls := make([]ToolCallCandidateEntryV1, 0)
	seen := make(map[string]struct{})
	for _, output := range response.Output {
		if output.Type != OutputItemFunctionCall {
			continue
		}
		if output.FunctionCall == nil {
			return ToolCallCandidateObservationV1{}, toolCallObservationError(ErrorMapping, "tool_call_missing", "function_call output is missing its call")
		}
		call := output.FunctionCall
		if strings.TrimSpace(call.ID) == "" || strings.TrimSpace(call.Name) == "" {
			return ToolCallCandidateObservationV1{}, toolCallObservationError(ErrorMapping, "tool_call_identity_invalid", "tool call ID and name are required")
		}
		if _, duplicate := seen[call.ID]; duplicate {
			return ToolCallCandidateObservationV1{}, toolCallObservationError(ErrorMapping, "duplicate_tool_call_id", "tool call IDs must be unique within one response")
		}
		seen[call.ID] = struct{}{}
		arguments, err := canonicalizeToolCallArgumentsV1(call.Arguments)
		if err != nil {
			return ToolCallCandidateObservationV1{}, err
		}
		calls = append(calls, ToolCallCandidateEntryV1{
			Ordinal: uint32(len(calls)), CallID: call.ID, Name: call.Name, CanonicalArguments: arguments,
		})
	}
	if len(calls) == 0 {
		return ToolCallCandidateObservationV1{}, toolCallObservationError(ErrorMapping, "tool_call_terminal_empty", "tool_call response contains no complete function calls")
	}

	observation := ToolCallCandidateObservationV1{
		ContractVersion:  ToolCallCandidateObservationContractVersionV1,
		InvocationDigest: invocationDigest,
		ResponseStatus:   response.Status,
		StopReason:       response.StopReason,
		Calls:            calls,
	}
	digest, err := digestToolCallObservationV1(observation)
	if err != nil {
		return ToolCallCandidateObservationV1{}, err
	}
	observation.Digest = digest
	return observation.Clone(), observation.Validate()
}

func normalizeToolCallObservationV1(value ToolCallCandidateObservationV1) (ToolCallCandidateObservationV1, error) {
	if value.ContractVersion != ToolCallCandidateObservationContractVersionV1 || value.InvocationDigest.Validate() != nil || value.ResponseStatus != ResponseStatusCompleted || value.StopReason != StopReasonToolCall || len(value.Calls) == 0 {
		return ToolCallCandidateObservationV1{}, toolCallObservationError(ErrorInvalidRequest, "tool_call_observation_invalid", "tool call observation identity or terminal state is invalid")
	}
	normalized := value.Clone()
	normalized.Digest = ""
	seen := make(map[string]struct{}, len(normalized.Calls))
	for index := range normalized.Calls {
		call := &normalized.Calls[index]
		if call.Ordinal != uint32(index) || strings.TrimSpace(call.CallID) == "" || strings.TrimSpace(call.Name) == "" {
			return ToolCallCandidateObservationV1{}, toolCallObservationError(ErrorInvalidRequest, "tool_call_entry_invalid", "tool call ordinals must be continuous and identities non-empty")
		}
		if _, duplicate := seen[call.CallID]; duplicate {
			return ToolCallCandidateObservationV1{}, toolCallObservationError(ErrorMapping, "duplicate_tool_call_id", "tool call IDs must be unique within one observation")
		}
		seen[call.CallID] = struct{}{}
		arguments, err := canonicalizeToolCallArgumentsV1(call.CanonicalArguments)
		if err != nil {
			return ToolCallCandidateObservationV1{}, err
		}
		call.CanonicalArguments = arguments
	}
	return normalized, nil
}

func digestToolCallObservationV1(value ToolCallCandidateObservationV1) (core.Digest, error) {
	normalized, err := normalizeToolCallObservationV1(value)
	if err != nil {
		return "", err
	}
	return core.CanonicalJSONDigest(
		"praxis.model-invoker.tool-call-observation",
		"v1",
		"ToolCallCandidateObservationV1",
		normalized,
	)
}

type canonicalToolCallNumberV1 string

func (number canonicalToolCallNumberV1) MarshalJSON() ([]byte, error) {
	return []byte(number), nil
}

func canonicalizeToolCallArgumentsV1(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 || len(raw) > core.MaxCanonicalDocumentBytes {
		return nil, toolCallObservationError(ErrorMapping, "tool_call_arguments_size_invalid", "tool call arguments are empty or exceed the canonical document limit")
	}
	if !utf8.Valid(raw) {
		return nil, toolCallObservationError(ErrorMapping, "tool_call_arguments_utf8_invalid", "tool call arguments must be valid UTF-8 JSON")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	value, err := decodeCanonicalToolCallJSONV1(decoder, 0)
	if err != nil {
		return nil, err
	}
	if _, ok := value.(map[string]any); !ok {
		return nil, toolCallObservationError(ErrorMapping, "tool_call_arguments_not_object", "tool call arguments must be one JSON object")
	}
	if token, err := decoder.Token(); err != io.EOF || token != nil {
		return nil, toolCallObservationError(ErrorMapping, "tool_call_arguments_trailing", "tool call arguments contain trailing JSON data")
	}
	canonical, err := json.Marshal(value)
	if err != nil || len(canonical) > core.MaxCanonicalDocumentBytes {
		return nil, toolCallObservationError(ErrorMapping, "tool_call_arguments_canonical_invalid", "tool call arguments cannot be represented within canonical limits")
	}
	return append(json.RawMessage(nil), canonical...), nil
}

func decodeCanonicalToolCallJSONV1(decoder *json.Decoder, depth int) (any, error) {
	if depth > maxToolCallJSONDepthV1 {
		return nil, toolCallObservationError(ErrorMapping, "tool_call_arguments_depth_exceeded", "tool call arguments exceed the canonical nesting limit")
	}
	token, err := decoder.Token()
	if err != nil {
		return nil, toolCallObservationError(ErrorMapping, "tool_call_arguments_json_invalid", "tool call arguments are invalid JSON")
	}
	switch value := token.(type) {
	case json.Delim:
		switch value {
		case '{':
			object := make(map[string]any)
			for decoder.More() {
				keyToken, keyErr := decoder.Token()
				key, ok := keyToken.(string)
				if keyErr != nil || !ok {
					return nil, toolCallObservationError(ErrorMapping, "tool_call_arguments_key_invalid", "tool call argument object key is invalid")
				}
				if _, duplicate := object[key]; duplicate {
					return nil, toolCallObservationError(ErrorMapping, "tool_call_arguments_duplicate_key", "tool call arguments contain a duplicate object key")
				}
				child, childErr := decodeCanonicalToolCallJSONV1(decoder, depth+1)
				if childErr != nil {
					return nil, childErr
				}
				object[key] = child
			}
			if end, endErr := decoder.Token(); endErr != nil || end != json.Delim('}') {
				return nil, toolCallObservationError(ErrorMapping, "tool_call_arguments_object_unclosed", "tool call argument object is not closed")
			}
			return object, nil
		case '[':
			array := make([]any, 0)
			for decoder.More() {
				child, childErr := decodeCanonicalToolCallJSONV1(decoder, depth+1)
				if childErr != nil {
					return nil, childErr
				}
				array = append(array, child)
			}
			if end, endErr := decoder.Token(); endErr != nil || end != json.Delim(']') {
				return nil, toolCallObservationError(ErrorMapping, "tool_call_arguments_array_unclosed", "tool call argument array is not closed")
			}
			return array, nil
		default:
			return nil, toolCallObservationError(ErrorMapping, "tool_call_arguments_delimiter_invalid", "tool call arguments contain an invalid delimiter")
		}
	case json.Number:
		normalized, normalizeErr := normalizeToolCallNumberV1(string(value))
		if normalizeErr != nil {
			return nil, normalizeErr
		}
		return canonicalToolCallNumberV1(normalized), nil
	case string, bool, nil:
		return value, nil
	default:
		return nil, toolCallObservationError(ErrorMapping, "tool_call_arguments_value_invalid", "tool call arguments contain an unsupported JSON value")
	}
}

func normalizeToolCallNumberV1(raw string) (string, error) {
	negative := strings.HasPrefix(raw, "-")
	unsigned := strings.TrimPrefix(raw, "-")
	mantissa, exponentText := unsigned, "0"
	if index := strings.IndexAny(unsigned, "eE"); index >= 0 {
		mantissa, exponentText = unsigned[:index], unsigned[index+1:]
	}
	exponent := new(big.Int)
	if _, ok := exponent.SetString(exponentText, 10); !ok {
		return "", toolCallObservationError(ErrorMapping, "tool_call_arguments_number_invalid", "tool call arguments contain an invalid JSON number")
	}
	fractionDigits := 0
	if point := strings.IndexByte(mantissa, '.'); point >= 0 {
		fractionDigits = len(mantissa) - point - 1
		mantissa = mantissa[:point] + mantissa[point+1:]
	}
	digits := strings.TrimLeft(mantissa, "0")
	if digits == "" {
		return "0", nil
	}
	trailing := len(digits) - len(strings.TrimRight(digits, "0"))
	digits = strings.TrimRight(digits, "0")
	exponent.Sub(exponent, big.NewInt(int64(fractionDigits)))
	exponent.Add(exponent, big.NewInt(int64(trailing)))
	prefix := ""
	if negative {
		prefix = "-"
	}
	if exponent.Sign() == 0 {
		return prefix + digits, nil
	}
	return prefix + digits + "e" + exponent.String(), nil
}

func toolCallObservationError(kind ErrorKind, code, message string) *Error {
	return &Error{Kind: kind, Operation: "finalize_tool_call_candidate", Code: code, Message: message}
}

type toolCallStreamStateV1 struct {
	name      string
	arguments bytes.Buffer
	completed bool
	canonical json.RawMessage
}

// ToolCallCandidateStreamFinalizerV1 consumes only public StreamEvent values.
// It emits nothing before a valid terminal response and never dispatches work.
type ToolCallCandidateStreamFinalizerV1 struct {
	mu               sync.Mutex
	invocationDigest core.Digest
	lastSequence     int64
	responseID       string
	calls            map[string]*toolCallStreamStateV1
	terminal         bool
	observation      *ToolCallCandidateObservationV1
	err              error
}

func NewToolCallCandidateStreamFinalizerV1(invocationDigest core.Digest) (*ToolCallCandidateStreamFinalizerV1, error) {
	if invocationDigest.Validate() != nil {
		return nil, toolCallObservationError(ErrorInvalidRequest, "invocation_digest_invalid", "an authoritative invocation digest is required")
	}
	return &ToolCallCandidateStreamFinalizerV1{invocationDigest: invocationDigest, calls: make(map[string]*toolCallStreamStateV1)}, nil
}

// Observe returns nil until response_completed. Successful and repeated reads
// of Result return independent deep copies.
func (f *ToolCallCandidateStreamFinalizerV1) Observe(event StreamEvent) (*ToolCallCandidateObservationV1, error) {
	if f == nil {
		return nil, toolCallObservationError(ErrorInvalidRequest, "tool_call_finalizer_nil", "tool call stream finalizer is nil")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	if f.terminal {
		return nil, toolCallObservationError(ErrorMapping, "tool_call_stream_after_terminal", "tool call stream emitted an event after terminal")
	}
	if event.Sequence <= 0 || event.Sequence <= f.lastSequence {
		return nil, f.failLocked(toolCallObservationError(ErrorMapping, "tool_call_stream_sequence_invalid", "tool call stream sequence must be positive and strictly increasing"))
	}
	f.lastSequence = event.Sequence
	if err := f.bindResponseIDLocked(event.ResponseID); err != nil {
		return nil, f.failLocked(err)
	}

	switch event.Type {
	case StreamEventResponseStarted:
		return nil, nil
	case StreamEventFunctionCallStarted:
		if event.FunctionCall == nil || strings.TrimSpace(event.FunctionCall.ID) == "" || strings.TrimSpace(event.FunctionCall.Name) == "" {
			return nil, f.failLocked(toolCallObservationError(ErrorMapping, "tool_call_stream_start_invalid", "tool call start requires an ID and name"))
		}
		if _, duplicate := f.calls[event.FunctionCall.ID]; duplicate {
			return nil, f.failLocked(toolCallObservationError(ErrorMapping, "duplicate_tool_call_id", "tool call stream repeated a call ID"))
		}
		f.calls[event.FunctionCall.ID] = &toolCallStreamStateV1{name: event.FunctionCall.Name}
		return nil, nil
	case StreamEventFunctionArgumentsDelta:
		if event.FunctionCall == nil || strings.TrimSpace(event.FunctionCall.ID) == "" {
			return nil, f.failLocked(toolCallObservationError(ErrorUnsupportedCapability, "tool_call_stream_correlation_unsupported", "streamed tool arguments lack portable call correlation"))
		}
		state, exists := f.calls[event.FunctionCall.ID]
		if !exists || state.completed {
			return nil, f.failLocked(toolCallObservationError(ErrorMapping, "tool_call_stream_delta_state_invalid", "tool argument delta has no open matching call"))
		}
		if event.FunctionCall.Name != "" && event.FunctionCall.Name != state.name {
			return nil, f.failLocked(toolCallObservationError(ErrorMapping, "tool_call_stream_name_conflict", "tool call name changed during streaming"))
		}
		state.arguments.WriteString(event.ArgumentsDelta)
		if len(event.FunctionCall.Arguments) > 0 && !bytes.Equal(event.FunctionCall.Arguments, state.arguments.Bytes()) {
			return nil, f.failLocked(toolCallObservationError(ErrorMapping, "tool_call_stream_delta_conflict", "tool argument cumulative snapshot conflicts with deltas"))
		}
		return nil, nil
	case StreamEventFunctionCallCompleted:
		if event.FunctionCall == nil || strings.TrimSpace(event.FunctionCall.ID) == "" || strings.TrimSpace(event.FunctionCall.Name) == "" {
			return nil, f.failLocked(toolCallObservationError(ErrorMapping, "tool_call_stream_complete_invalid", "completed tool call requires an ID and name"))
		}
		state, exists := f.calls[event.FunctionCall.ID]
		if !exists || state.completed || state.name != event.FunctionCall.Name {
			return nil, f.failLocked(toolCallObservationError(ErrorMapping, "tool_call_stream_complete_state_invalid", "completed tool call does not match one open call"))
		}
		canonical, err := canonicalizeToolCallArgumentsV1(event.FunctionCall.Arguments)
		if err != nil {
			return nil, f.failLocked(err)
		}
		if state.arguments.Len() > 0 {
			assembled, assembleErr := canonicalizeToolCallArgumentsV1(state.arguments.Bytes())
			if assembleErr != nil || !bytes.Equal(assembled, canonical) {
				return nil, f.failLocked(toolCallObservationError(ErrorMapping, "tool_call_stream_complete_conflict", "completed tool arguments conflict with accumulated deltas"))
			}
		}
		state.completed = true
		state.canonical = canonical
		return nil, nil
	case StreamEventResponseCompleted:
		if event.Response == nil {
			return nil, f.failLocked(toolCallObservationError(ErrorMapping, "tool_call_stream_terminal_missing", "tool call stream terminal response is missing"))
		}
		terminalResponse := *event.Response
		if err := f.bindTerminalResponseIDLocked(terminalResponse.ID); err != nil {
			return nil, f.failLocked(err)
		}
		terminalResponse.ID = f.responseID
		observation, err := FinalizeToolCallCandidateObservationV1(f.invocationDigest, terminalResponse)
		if err != nil {
			return nil, f.failLocked(err)
		}
		if err := f.matchTerminalLocked(observation); err != nil {
			return nil, f.failLocked(err)
		}
		f.terminal = true
		clone := observation.Clone()
		f.observation = &clone
		result := clone.Clone()
		return &result, nil
	case StreamEventError:
		return nil, f.failLocked(toolCallObservationError(ErrorStreamInterrupted, "tool_call_stream_failed", "tool call stream failed before a valid terminal response"))
	default:
		return nil, nil
	}
}

func (f *ToolCallCandidateStreamFinalizerV1) Result() (*ToolCallCandidateObservationV1, error) {
	if f == nil {
		return nil, toolCallObservationError(ErrorInvalidRequest, "tool_call_finalizer_nil", "tool call stream finalizer is nil")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	if f.observation == nil {
		return nil, nil
	}
	clone := f.observation.Clone()
	return &clone, nil
}

// FinalizedResponseID returns the unique source response ID sealed by a
// successful terminal observation. An empty terminal Response.ID inherits an
// already-bound StreamEvent.ResponseID; a stream with no ID cannot finalize.
func (f *ToolCallCandidateStreamFinalizerV1) FinalizedResponseID() (string, error) {
	if f == nil {
		return "", toolCallObservationError(ErrorInvalidRequest, "tool_call_finalizer_nil", "tool call stream finalizer is nil")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return "", f.err
	}
	if f.observation == nil || f.responseID == "" {
		return "", toolCallObservationError(ErrorMapping, "tool_call_stream_response_unfinalized", "tool call stream has no finalized response identity")
	}
	return f.responseID, nil
}

func (f *ToolCallCandidateStreamFinalizerV1) bindResponseIDLocked(responseID string) error {
	if responseID == "" {
		return nil
	}
	if f.responseID == "" {
		f.responseID = responseID
		return nil
	}
	if f.responseID != responseID {
		return toolCallObservationError(ErrorMapping, "tool_call_stream_response_conflict", "tool call stream response ID changed")
	}
	return nil
}

func (f *ToolCallCandidateStreamFinalizerV1) bindTerminalResponseIDLocked(responseID string) error {
	if responseID != "" {
		return f.bindResponseIDLocked(responseID)
	}
	if f.responseID == "" {
		return toolCallObservationError(ErrorMapping, "tool_call_stream_response_missing", "tool call stream terminal has no bound response ID")
	}
	return nil
}

func (f *ToolCallCandidateStreamFinalizerV1) matchTerminalLocked(observation ToolCallCandidateObservationV1) error {
	terminal := make(map[string]ToolCallCandidateEntryV1, len(observation.Calls))
	for _, call := range observation.Calls {
		terminal[call.CallID] = call
		if state, exists := f.calls[call.CallID]; exists {
			if !state.completed || state.name != call.Name || !bytes.Equal(state.canonical, call.CanonicalArguments) {
				return toolCallObservationError(ErrorMapping, "tool_call_stream_terminal_conflict", "terminal tool call conflicts with streamed tool call")
			}
		}
	}
	for callID, state := range f.calls {
		if !state.completed {
			return toolCallObservationError(ErrorMapping, "tool_call_stream_partial_terminal", "terminal response arrived with a partial tool call")
		}
		if _, exists := terminal[callID]; !exists {
			return toolCallObservationError(ErrorMapping, "tool_call_stream_terminal_missing_call", "terminal response omitted a streamed tool call")
		}
	}
	return nil
}

func (f *ToolCallCandidateStreamFinalizerV1) failLocked(err error) error {
	f.terminal = true
	f.err = err
	return err
}
