package modelinvoker

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"
)

type ProviderID string

type Protocol string

const (
	ProtocolAuto            Protocol = ""
	ProtocolResponses       Protocol = "responses"
	ProtocolChatCompletions Protocol = "chat_completions"
	ProtocolMessages        Protocol = "messages"
	ProtocolGenerateContent Protocol = "generate_content"
	ProtocolBedrockConverse Protocol = "bedrock_converse"
	ProtocolBedrockInvoke   Protocol = "bedrock_invoke_model"
)

func (p Protocol) valid() bool {
	switch p {
	case ProtocolResponses, ProtocolChatCompletions, ProtocolMessages, ProtocolGenerateContent,
		ProtocolBedrockConverse, ProtocolBedrockInvoke:
		return true
	default:
		return false
	}
}

type Role string

const (
	RoleSystem    Role = "system"
	RoleDeveloper Role = "developer"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

type InputType string

const (
	InputTypeMessage        InputType = "message"
	InputTypeFunctionCall   InputType = "function_call"
	InputTypeFunctionResult InputType = "function_result"
)

type Instruction struct {
	Role Role
	Text string
}

type Message struct {
	Role Role
	Text string
}

type FunctionCall struct {
	ID        string
	Name      string
	Arguments json.RawMessage
}

type FunctionResult struct {
	CallID  string
	Name    string
	Output  string
	IsError bool
}

// InputItem is a tagged union. Exactly one pointer matching Type must be set.
type InputItem struct {
	Type           InputType
	Message        *Message
	FunctionCall   *FunctionCall
	FunctionResult *FunctionResult
}

func MessageInput(role Role, text string) InputItem {
	return InputItem{Type: InputTypeMessage, Message: &Message{Role: role, Text: text}}
}

func FunctionCallInput(id, name string, arguments json.RawMessage) InputItem {
	return InputItem{Type: InputTypeFunctionCall, FunctionCall: &FunctionCall{ID: id, Name: name, Arguments: cloneJSON(arguments)}}
}

func FunctionResultInput(callID, output string, isError bool) InputItem {
	return InputItem{Type: InputTypeFunctionResult, FunctionResult: &FunctionResult{CallID: callID, Output: output, IsError: isError}}
}

func NamedFunctionResultInput(callID, name, output string, isError bool) InputItem {
	return InputItem{Type: InputTypeFunctionResult, FunctionResult: &FunctionResult{CallID: callID, Name: name, Output: output, IsError: isError}}
}

type Tool struct {
	Name        string
	Description string
	Parameters  json.RawMessage
	Strict      *bool
}

type ToolChoiceMode string

const (
	ToolChoiceAuto     ToolChoiceMode = ""
	ToolChoiceNone     ToolChoiceMode = "none"
	ToolChoiceRequired ToolChoiceMode = "required"
	ToolChoiceFunction ToolChoiceMode = "function"
)

type ToolChoice struct {
	Mode ToolChoiceMode
	Name string
}

type OutputConstraintType string

const (
	OutputText       OutputConstraintType = ""
	OutputJSONObject OutputConstraintType = "json_object"
	OutputJSONSchema OutputConstraintType = "json_schema"
)

type OutputConstraint struct {
	Type        OutputConstraintType
	Name        string
	Description string
	Schema      json.RawMessage
	Strict      *bool
}

type ReasoningEffort string

const (
	ReasoningEffortNone    ReasoningEffort = "none"
	ReasoningEffortMinimal ReasoningEffort = "minimal"
	ReasoningEffortLow     ReasoningEffort = "low"
	ReasoningEffortMedium  ReasoningEffort = "medium"
	ReasoningEffortHigh    ReasoningEffort = "high"
	ReasoningEffortXHigh   ReasoningEffort = "xhigh"
	ReasoningEffortMax     ReasoningEffort = "max"
)

type ReasoningSummary string

const (
	ReasoningSummaryAuto     ReasoningSummary = "auto"
	ReasoningSummaryConcise  ReasoningSummary = "concise"
	ReasoningSummaryDetailed ReasoningSummary = "detailed"
)

type Reasoning struct {
	Effort       ReasoningEffort
	Summary      ReasoningSummary
	BudgetTokens *int64
}

type StateKind string

const (
	StateServerContinuation   StateKind = "server_continuation"
	StateProviderContinuation StateKind = "provider_continuation"
)

type State struct {
	Kind     StateKind
	Provider ProviderID
	Protocol Protocol
	ID       string
	Payload  RawPayload
}

type Budget struct {
	MaxOutputTokens int64
	Timeout         time.Duration
}

type Metadata map[string]string

// ProviderOptions isolates provider-specific JSON under a provider ID.
type ProviderOptions map[ProviderID]json.RawMessage

type Request struct {
	Provider          ProviderID
	Protocol          Protocol
	Endpoint          string
	Model             string
	Input             []InputItem
	Instructions      []Instruction
	Tools             []Tool
	ToolChoice        ToolChoice
	ParallelToolCalls *bool
	Output            OutputConstraint
	Reasoning         *Reasoning
	State             *State
	Stream            bool
	Budget            Budget
	Metadata          Metadata
	ProviderOptions   ProviderOptions
	AllowDegradation  bool
}

type ResponseStatus string

const (
	ResponseStatusCompleted  ResponseStatus = "completed"
	ResponseStatusIncomplete ResponseStatus = "incomplete"
	ResponseStatusFailed     ResponseStatus = "failed"
	ResponseStatusCancelled  ResponseStatus = "cancelled"
	ResponseStatusInProgress ResponseStatus = "in_progress"
)

type StopReason string

const (
	StopReasonEndTurn         StopReason = "end_turn"
	StopReasonMaxOutputTokens StopReason = "max_output_tokens"
	StopReasonStopSequence    StopReason = "stop_sequence"
	StopReasonToolCall        StopReason = "tool_call"
	StopReasonContentFilter   StopReason = "content_filter"
	StopReasonPaused          StopReason = "paused"
	StopReasonOther           StopReason = "other"
)

type OutputItemType string

const (
	OutputItemText             OutputItemType = "text"
	OutputItemFunctionCall     OutputItemType = "function_call"
	OutputItemReasoningSummary OutputItemType = "reasoning_summary"
)

type OutputItem struct {
	Type             OutputItemType
	Text             string
	FunctionCall     *FunctionCall
	ReasoningSummary string
}

type Usage struct {
	InputTokens      int64
	OutputTokens     int64
	ReasoningTokens  int64
	CacheReadTokens  int64
	CacheWriteTokens int64
	TotalTokens      int64
}

type ProviderMetadata map[string]string

type Response struct {
	ID               string
	Provider         ProviderID
	Protocol         Protocol
	Model            string
	Status           ResponseStatus
	StopReason       StopReason
	StopSequence     string
	Output           []OutputItem
	Usage            Usage
	RequestID        string
	Metadata         Metadata
	State            *State
	ProviderMetadata ProviderMetadata
	MappingReport    MappingReport
	RawRequest       RawPayload
	RawResponse      RawPayload
	NativeEvents     []RawPayload
}

func (r Response) Text() string {
	var builder strings.Builder
	for _, item := range r.Output {
		if item.Type == OutputItemText {
			builder.WriteString(item.Text)
		}
	}
	return builder.String()
}

func (r Response) FunctionCalls() []FunctionCall {
	calls := make([]FunctionCall, 0)
	for _, item := range r.Output {
		if item.Type == OutputItemFunctionCall && item.FunctionCall != nil {
			call := *item.FunctionCall
			call.Arguments = cloneJSON(call.Arguments)
			calls = append(calls, call)
		}
	}
	return calls
}

func (r Request) Validate() error {
	if strings.TrimSpace(string(r.Provider)) == "" {
		return newValidationError("provider is required")
	}
	if strings.TrimSpace(r.Model) == "" {
		return newValidationError("model is required")
	}
	if r.Protocol != ProtocolAuto && !r.Protocol.valid() {
		return newValidationError(fmt.Sprintf("unknown protocol %q", r.Protocol))
	}
	if r.Endpoint != "" {
		u, err := url.Parse(r.Endpoint)
		if err != nil || u.Scheme == "" || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return newValidationError("endpoint must be an absolute HTTP(S) URL without credentials, query, or fragment")
		}
	}
	if len(r.Input) == 0 {
		return newValidationError("at least one input item is required")
	}
	for index, instruction := range r.Instructions {
		if instruction.Role != RoleSystem && instruction.Role != RoleDeveloper {
			return newValidationError(fmt.Sprintf("instruction %d has invalid role %q", index, instruction.Role))
		}
		if strings.TrimSpace(instruction.Text) == "" {
			return newValidationError(fmt.Sprintf("instruction %d text is required", index))
		}
	}
	for index, item := range r.Input {
		if err := validateInputItem(item); err != nil {
			return newValidationError(fmt.Sprintf("input %d: %s", index, err))
		}
	}
	toolNames := make(map[string]struct{}, len(r.Tools))
	for index, tool := range r.Tools {
		if strings.TrimSpace(tool.Name) == "" {
			return newValidationError(fmt.Sprintf("tool %d name is required", index))
		}
		if _, exists := toolNames[tool.Name]; exists {
			return newValidationError(fmt.Sprintf("tool %d duplicates name %q", index, tool.Name))
		}
		toolNames[tool.Name] = struct{}{}
		if !validJSONObject(tool.Parameters) {
			return newValidationError(fmt.Sprintf("tool %d parameters must be a JSON object", index))
		}
	}
	if err := validateToolChoice(r.ToolChoice, r.Tools); err != nil {
		return newValidationError(err.Error())
	}
	if err := validateOutputConstraint(r.Output); err != nil {
		return newValidationError(err.Error())
	}
	if err := validateReasoning(r.Reasoning); err != nil {
		return newValidationError(err.Error())
	}
	if err := validateState(r.State, r.Provider, r.Protocol); err != nil {
		return newValidationError(err.Error())
	}
	if r.Budget.MaxOutputTokens < 0 || r.Budget.Timeout < 0 {
		return newValidationError("budget values must not be negative")
	}
	for key := range r.Metadata {
		if strings.TrimSpace(key) == "" {
			return newValidationError("metadata keys must be non-empty")
		}
	}
	for provider, raw := range r.ProviderOptions {
		if strings.TrimSpace(string(provider)) == "" || !validJSONObject(raw) {
			return newValidationError("provider options must use a non-empty provider ID and JSON object")
		}
		if provider != r.Provider {
			return newValidationError("provider options namespace must match the selected provider")
		}
	}
	return nil
}

func validateInputItem(item InputItem) error {
	set := 0
	if item.Message != nil {
		set++
	}
	if item.FunctionCall != nil {
		set++
	}
	if item.FunctionResult != nil {
		set++
	}
	if set != 1 {
		return fmt.Errorf("exactly one union value is required")
	}
	switch item.Type {
	case InputTypeMessage:
		if item.Message == nil || item.FunctionCall != nil || item.FunctionResult != nil {
			return fmt.Errorf("message tag does not match union value")
		}
		if item.Message.Role != RoleSystem && item.Message.Role != RoleDeveloper && item.Message.Role != RoleUser && item.Message.Role != RoleAssistant {
			return fmt.Errorf("invalid message role %q", item.Message.Role)
		}
		if strings.TrimSpace(item.Message.Text) == "" {
			return fmt.Errorf("message text is required")
		}
	case InputTypeFunctionCall:
		if item.FunctionCall == nil || item.Message != nil || item.FunctionResult != nil {
			return fmt.Errorf("function_call tag does not match union value")
		}
		if strings.TrimSpace(item.FunctionCall.Name) == "" || !validJSONObject(item.FunctionCall.Arguments) {
			return fmt.Errorf("function call requires a non-empty name and JSON object arguments")
		}
	case InputTypeFunctionResult:
		if item.FunctionResult == nil || item.Message != nil || item.FunctionCall != nil {
			return fmt.Errorf("function_result tag does not match union value")
		}
		if strings.TrimSpace(item.FunctionResult.CallID) == "" && strings.TrimSpace(item.FunctionResult.Name) == "" {
			return fmt.Errorf("function result call id or name is required")
		}
		if item.FunctionResult.Name != "" && strings.TrimSpace(item.FunctionResult.Name) == "" {
			return fmt.Errorf("function result name must be non-empty when provided")
		}
	default:
		return fmt.Errorf("unknown input type %q", item.Type)
	}
	return nil
}

func validateToolChoice(choice ToolChoice, tools []Tool) error {
	switch choice.Mode {
	case ToolChoiceAuto, ToolChoiceNone:
		if choice.Name != "" {
			return fmt.Errorf("tool choice name requires function mode")
		}
	case ToolChoiceRequired:
		if len(tools) == 0 || choice.Name != "" {
			return fmt.Errorf("required tool choice needs tools and no function name")
		}
	case ToolChoiceFunction:
		if strings.TrimSpace(choice.Name) == "" {
			return fmt.Errorf("function tool choice requires a non-empty name")
		}
		found := false
		for _, tool := range tools {
			found = found || tool.Name == choice.Name
		}
		if !found {
			return fmt.Errorf("function tool choice references an undeclared tool")
		}
	default:
		return fmt.Errorf("unknown tool choice mode %q", choice.Mode)
	}
	return nil
}

func validateOutputConstraint(output OutputConstraint) error {
	switch output.Type {
	case OutputText:
		if output.Name != "" || output.Description != "" || len(output.Schema) != 0 || output.Strict != nil {
			return fmt.Errorf("text output must not include schema fields")
		}
	case OutputJSONObject:
		if output.Name != "" || output.Description != "" || len(output.Schema) != 0 || output.Strict != nil {
			return fmt.Errorf("json_object output must not include schema fields")
		}
	case OutputJSONSchema:
		if strings.TrimSpace(output.Name) == "" || !validJSONObject(output.Schema) {
			return fmt.Errorf("json_schema output requires a non-empty name and JSON object schema")
		}
	default:
		return fmt.Errorf("unknown output constraint %q", output.Type)
	}
	return nil
}

func validateReasoning(reasoning *Reasoning) error {
	if reasoning == nil {
		return nil
	}
	switch reasoning.Effort {
	case "", ReasoningEffortNone, ReasoningEffortMinimal, ReasoningEffortLow, ReasoningEffortMedium, ReasoningEffortHigh, ReasoningEffortXHigh, ReasoningEffortMax:
	default:
		return fmt.Errorf("unknown reasoning effort %q", reasoning.Effort)
	}
	switch reasoning.Summary {
	case "", ReasoningSummaryAuto, ReasoningSummaryConcise, ReasoningSummaryDetailed:
	default:
		return fmt.Errorf("unknown reasoning summary %q", reasoning.Summary)
	}
	if reasoning.BudgetTokens != nil && *reasoning.BudgetTokens <= 0 {
		return fmt.Errorf("reasoning budget tokens must be positive")
	}
	if reasoning.Effort == ReasoningEffortNone && (reasoning.Summary != "" || reasoning.BudgetTokens != nil) {
		return fmt.Errorf("reasoning effort none cannot request summary or budget")
	}
	if reasoning.Effort == "" && reasoning.Summary == "" && reasoning.BudgetTokens == nil {
		return fmt.Errorf("reasoning must request effort, summary, or budget")
	}
	return nil
}

func validateState(state *State, provider ProviderID, protocol Protocol) error {
	if state == nil {
		return nil
	}
	if state.Provider != provider {
		return fmt.Errorf("state provider must match the selected provider")
	}
	if state.Protocol == ProtocolAuto || !state.Protocol.valid() {
		return fmt.Errorf("state protocol is invalid")
	}
	if protocol != ProtocolAuto && state.Protocol != protocol {
		return fmt.Errorf("state protocol must match the selected protocol")
	}
	switch state.Kind {
	case StateServerContinuation:
		if strings.TrimSpace(state.ID) == "" || !state.Payload.Empty() {
			return fmt.Errorf("server continuation state requires id and no payload")
		}
	case StateProviderContinuation:
		if state.Payload.Empty() {
			return fmt.Errorf("provider continuation state requires payload")
		}
	default:
		return fmt.Errorf("state kind is invalid")
	}
	return nil
}

func validJSONObject(raw json.RawMessage) bool {
	if len(raw) == 0 || !json.Valid(raw) {
		return false
	}
	var object map[string]json.RawMessage
	return json.Unmarshal(raw, &object) == nil && object != nil
}

func cloneJSON(raw json.RawMessage) json.RawMessage {
	return append(json.RawMessage(nil), raw...)
}
