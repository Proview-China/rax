package geminigenerate

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol"
	"google.golang.org/genai"
)

const continuationVersion = 1

type continuationCall struct {
	Name      string `json:"name"`
	NativeID  string `json:"native_id,omitempty"`
	Responded bool   `json:"responded,omitempty"`
}

type continuationEnvelope struct {
	Version  int                         `json:"version"`
	Contents []*genai.Content            `json:"contents"`
	Calls    map[string]continuationCall `json:"calls,omitempty"`
}

func newContinuationEnvelope() continuationEnvelope {
	return continuationEnvelope{Version: continuationVersion, Calls: make(map[string]continuationCall)}
}

func decodeContinuation(state *modelinvoker.State) (continuationEnvelope, error) {
	if state == nil {
		return newContinuationEnvelope(), nil
	}
	var envelope continuationEnvelope
	decoder := json.NewDecoder(bytes.NewReader(state.Payload.Bytes()))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil {
		return continuationEnvelope{}, fmt.Errorf("invalid Gemini continuation payload: %w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return continuationEnvelope{}, fmt.Errorf("invalid Gemini continuation payload: %w", err)
	}
	if envelope.Version != continuationVersion {
		return continuationEnvelope{}, fmt.Errorf("unsupported Gemini continuation version %d", envelope.Version)
	}
	if envelope.Calls == nil {
		envelope.Calls = make(map[string]continuationCall)
	}
	if err := validateContinuationEnvelope(envelope); err != nil {
		return continuationEnvelope{}, err
	}
	return envelope, nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("payload contains trailing JSON values")
		}
		return err
	}
	return nil
}

func encodeContinuation(binding protocol.Binding, envelope continuationEnvelope, id string) (*modelinvoker.State, error) {
	envelope.Version = continuationVersion
	if err := validateContinuationEnvelope(envelope); err != nil {
		return nil, err
	}
	data, err := json.Marshal(envelope)
	if err != nil {
		return nil, fmt.Errorf("serialize Gemini continuation: %w", err)
	}
	return &modelinvoker.State{
		Kind:     modelinvoker.StateProviderContinuation,
		Provider: binding.Provider,
		Protocol: binding.Protocol,
		ID:       id,
		Payload:  modelinvoker.NewRawPayload(data),
	}, nil
}

type continuationNativeCall struct {
	name string
	id   string
}

func validateContinuationEnvelope(envelope continuationEnvelope) error {
	contentCalls := make(map[continuationNativeCall]int)
	contentResponses := make(map[continuationNativeCall]int)
	for contentIndex, content := range envelope.Contents {
		if content == nil {
			return fmt.Errorf("continuation content %d is nil", contentIndex)
		}
		if content.Role != genai.RoleUser && content.Role != genai.RoleModel {
			return fmt.Errorf("continuation content %d has invalid role %q", contentIndex, content.Role)
		}
		if len(content.Parts) == 0 {
			return fmt.Errorf("continuation content %d has no parts", contentIndex)
		}
		for partIndex, part := range content.Parts {
			if err := validateContinuationPart(content.Role, part); err != nil {
				return fmt.Errorf("continuation content %d part %d: %w", contentIndex, partIndex, err)
			}
			if part.FunctionCall != nil {
				contentCalls[continuationNativeCall{name: part.FunctionCall.Name, id: part.FunctionCall.ID}]++
			}
			if part.FunctionResponse != nil {
				contentResponses[continuationNativeCall{name: part.FunctionResponse.Name, id: part.FunctionResponse.ID}]++
			}
		}
	}

	recordedCalls := make(map[continuationNativeCall]int)
	recordedResponses := make(map[continuationNativeCall]int)
	for id, call := range envelope.Calls {
		if strings.TrimSpace(id) == "" || !nativeToolNamePattern.MatchString(call.Name) {
			return fmt.Errorf("continuation contains an invalid function-call reference")
		}
		if call.NativeID != "" && id != call.NativeID {
			return fmt.Errorf("continuation call %q does not preserve its native ID", id)
		}
		if call.NativeID == "" && !validSemanticCallID(id) {
			return fmt.Errorf("continuation call %q has an invalid generated ID", id)
		}
		key := continuationNativeCall{name: call.Name, id: call.NativeID}
		recordedCalls[key]++
		if call.Responded {
			recordedResponses[key]++
		}
	}
	if !equalCallCounts(contentCalls, recordedCalls) {
		return fmt.Errorf("continuation function-call index does not match content")
	}
	if !equalCallCounts(contentResponses, recordedResponses) {
		return fmt.Errorf("continuation function-response index does not match content")
	}
	for key, count := range contentResponses {
		if count > contentCalls[key] {
			return fmt.Errorf("continuation function response %q has no matching call", key.name)
		}
	}
	return nil
}

func validateContinuationPart(role string, part *genai.Part) error {
	if part == nil {
		return fmt.Errorf("part is nil")
	}
	if part.MediaResolution != nil || part.CodeExecutionResult != nil || part.ExecutableCode != nil ||
		part.FileData != nil || part.InlineData != nil || part.VideoMetadata != nil ||
		part.ToolCall != nil || part.ToolResponse != nil || len(part.PartMetadata) != 0 {
		return fmt.Errorf("part contains fields outside the Gemini GenerateContent continuation slice")
	}
	if role == genai.RoleModel {
		if unsupported := unsupportedPartDescription(part); unsupported != "" {
			return fmt.Errorf("%s is outside the Gemini GenerateContent continuation slice", unsupported)
		}
	}
	semanticValues := 0
	if part.Text != "" {
		semanticValues++
	}
	if part.FunctionCall != nil {
		semanticValues++
	}
	if part.FunctionResponse != nil {
		semanticValues++
	}
	if semanticValues == 0 && !part.Thought && len(part.ThoughtSignature) == 0 {
		return fmt.Errorf("part is empty")
	}
	if semanticValues > 1 {
		return fmt.Errorf("part contains multiple semantic values")
	}
	if role == genai.RoleUser {
		if part.FunctionCall != nil || part.Thought || len(part.ThoughtSignature) != 0 {
			return fmt.Errorf("user part contains model-only fields")
		}
	} else if part.FunctionResponse != nil {
		return fmt.Errorf("model part contains a function response")
	}
	if call := part.FunctionCall; call != nil {
		if !nativeToolNamePattern.MatchString(call.Name) || call.PartialArgs != nil || call.WillContinue != nil {
			return fmt.Errorf("function call is outside the supported Gemini slice")
		}
		if _, err := json.Marshal(call.Args); err != nil {
			return fmt.Errorf("function call arguments are not JSON encodable")
		}
	}
	if response := part.FunctionResponse; response != nil {
		if !nativeToolNamePattern.MatchString(response.Name) || response.Response == nil ||
			response.WillContinue != nil || response.Scheduling != "" || len(response.Parts) != 0 {
			return fmt.Errorf("function response is outside the supported Gemini slice")
		}
		if len(response.Response) != 1 {
			return fmt.Errorf("function response must contain exactly one output or error field")
		}
		if _, output := response.Response["output"]; !output {
			if _, failure := response.Response["error"]; !failure {
				return fmt.Errorf("function response must contain output or error")
			}
		}
	}
	return nil
}

func equalCallCounts(left, right map[continuationNativeCall]int) bool {
	if len(left) != len(right) {
		return false
	}
	for key, count := range left {
		if right[key] != count {
			return false
		}
	}
	return true
}

func validSemanticCallID(id string) bool {
	const prefix = "gemini_call_"
	if !strings.HasPrefix(id, prefix) || len(id) != len(prefix)+24 {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(id, prefix))
	return err == nil
}

func cloneContents(contents []*genai.Content) ([]*genai.Content, error) {
	if contents == nil {
		return nil, nil
	}
	data, err := json.Marshal(contents)
	if err != nil {
		return nil, err
	}
	var clone []*genai.Content
	if err := json.Unmarshal(data, &clone); err != nil {
		return nil, err
	}
	return clone, nil
}

func cloneContent(content *genai.Content) (*genai.Content, error) {
	if content == nil {
		return nil, nil
	}
	contents, err := cloneContents([]*genai.Content{content})
	if err != nil {
		return nil, err
	}
	return contents[0], nil
}

func semanticCallID(responseID string, contentIndex, candidateIndex, partIndex int, call *genai.FunctionCall) string {
	hash := sha256.New()
	_, _ = hash.Write([]byte(responseID))
	_, _ = hash.Write([]byte(fmt.Sprintf("\x00%d\x00%d\x00%d\x00", contentIndex, candidateIndex, partIndex)))
	if call != nil {
		_, _ = hash.Write([]byte(call.Name))
		args, _ := json.Marshal(call.Args)
		_, _ = hash.Write(args)
	}
	return "gemini_call_" + hex.EncodeToString(hash.Sum(nil))[:24]
}

func addContinuationCall(
	envelope *continuationEnvelope,
	responseID string,
	candidateIndex int,
	partIndex int,
	call *genai.FunctionCall,
) (string, error) {
	if envelope == nil || call == nil || strings.TrimSpace(call.Name) == "" {
		return "", fmt.Errorf("Gemini function call requires a name")
	}
	id := call.ID
	if id == "" {
		// The response ID is optional. The native assistant content ordinal keeps
		// otherwise-identical calls in later turns distinct even when it is empty.
		id = semanticCallID(responseID, len(envelope.Contents), candidateIndex, partIndex, call)
	}
	if existing, exists := envelope.Calls[id]; exists {
		if existing.Name != call.Name || existing.NativeID != call.ID {
			return "", fmt.Errorf("Gemini function call ID %q is duplicated", id)
		}
		return id, nil
	}
	envelope.Calls[id] = continuationCall{Name: call.Name, NativeID: call.ID}
	return id, nil
}

func addInputContinuationCall(
	envelope *continuationEnvelope,
	contentIndex int,
	partIndex int,
	call *genai.FunctionCall,
) (string, error) {
	if envelope == nil || call == nil || !nativeToolNamePattern.MatchString(call.Name) {
		return "", fmt.Errorf("Gemini function call requires a valid name")
	}
	id := call.ID
	if id == "" {
		id = semanticCallID("", contentIndex, 0, partIndex, call)
	}
	if _, exists := envelope.Calls[id]; exists {
		return "", fmt.Errorf("Gemini function call ID %q is duplicated", id)
	}
	envelope.Calls[id] = continuationCall{Name: call.Name, NativeID: call.ID}
	return id, nil
}

func resolveFunctionResult(envelope *continuationEnvelope, result *modelinvoker.FunctionResult) (string, continuationCall, error) {
	if envelope == nil || result == nil {
		return "", continuationCall{}, fmt.Errorf("function result is nil")
	}
	if result.CallID != "" {
		call, ok := envelope.Calls[result.CallID]
		if !ok {
			return "", continuationCall{}, fmt.Errorf("function result references unknown call ID %q", result.CallID)
		}
		if call.Responded {
			return "", continuationCall{}, fmt.Errorf("function result call ID %q was already answered", result.CallID)
		}
		if result.Name != "" && result.Name != call.Name {
			return "", continuationCall{}, fmt.Errorf("function result name %q does not match call %q", result.Name, call.Name)
		}
		return result.CallID, call, nil
	}
	var matchedID string
	var matched continuationCall
	for id, call := range envelope.Calls {
		if call.Responded || call.Name != result.Name {
			continue
		}
		if matchedID != "" {
			return "", continuationCall{}, fmt.Errorf("function result name %q is ambiguous without a call ID", result.Name)
		}
		matchedID, matched = id, call
	}
	if matchedID == "" {
		return "", continuationCall{}, fmt.Errorf("function result name %q has no pending call", result.Name)
	}
	return matchedID, matched, nil
}
