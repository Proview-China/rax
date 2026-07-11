package core_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	. "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
)

func validRequest() Request {
	return Request{
		Provider: "test",
		Model:    "test-model",
		Input:    []InputItem{MessageInput(RoleUser, "hello")},
	}
}

func TestRequestValidateAcceptsSupportedSemanticShapes(t *testing.T) {
	strict := true
	parallel := true
	reasoningBudget := int64(2048)
	tests := []struct {
		name    string
		request Request
	}{
		{name: "minimal text request", request: validRequest()},
		{
			name: "complete request",
			request: Request{
				Provider: "test",
				Protocol: ProtocolResponses,
				Endpoint: "http://127.0.0.1:8080/v1",
				Model:    "reasoning-model",
				Instructions: []Instruction{
					{Role: RoleSystem, Text: "be accurate"},
					{Role: RoleDeveloper, Text: "return JSON"},
				},
				Input: []InputItem{
					MessageInput(RoleUser, "find the weather"),
					FunctionCallInput("call-1", "weather", json.RawMessage(`{"city":"Shanghai"}`)),
					FunctionResultInput("call-1", `{"temperature":30}`, false),
				},
				Tools: []Tool{{
					Name:       "weather",
					Parameters: json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}}}`),
					Strict:     &strict,
				}},
				ToolChoice:        ToolChoice{Mode: ToolChoiceFunction, Name: "weather"},
				ParallelToolCalls: &parallel,
				Output: OutputConstraint{
					Type:   OutputJSONSchema,
					Name:   "weather_response",
					Schema: json.RawMessage(`{"type":"object"}`),
					Strict: &strict,
				},
				Reasoning: &Reasoning{Effort: ReasoningEffortHigh, Summary: ReasoningSummaryConcise, BudgetTokens: &reasoningBudget},
				State: &State{
					Kind: StateServerContinuation, Provider: "test", Protocol: ProtocolResponses, ID: "resp_previous",
				},
				Stream:   true,
				Budget:   Budget{MaxOutputTokens: 1024, Timeout: time.Second},
				Metadata: Metadata{"trace_id": "trace-1", "job": "weather"},
				ProviderOptions: ProviderOptions{
					"test": json.RawMessage(`{"extension":true}`),
				},
				AllowDegradation: true,
			},
		},
		{
			name: "provider continuation and name-correlated function result",
			request: Request{
				Provider: "test", Protocol: ProtocolGenerateContent, Model: "gemini-model",
				Input: []InputItem{
					FunctionCallInput("", "lookup.weather", json.RawMessage(`{"city":"Shanghai"}`)),
					NamedFunctionResultInput("", "lookup.weather", "sunny", false),
				},
				State: &State{
					Kind: StateProviderContinuation, Provider: "test", Protocol: ProtocolGenerateContent,
					Payload: NewRawPayload([]byte(`{"version":1}`)),
				},
			},
		},
		{
			name: "chat JSON object",
			request: Request{
				Provider: "test",
				Protocol: ProtocolChatCompletions,
				Model:    "chat-model",
				Input:    []InputItem{MessageInput(RoleAssistant, "prior answer")},
				Output:   OutputConstraint{Type: OutputJSONObject},
			},
		},
		{
			name: "provider-specific names remain provider neutral",
			request: Request{
				Provider: "test", Protocol: ProtocolGenerateContent, Model: "provider-model",
				Input: []InputItem{
					FunctionCallInput("", "vendor tool/"+strings.Repeat("x", 140), json.RawMessage(`{}`)),
					NamedFunctionResultInput("", "vendor result name", "done", false),
				},
				Tools:      []Tool{{Name: "vendor tool/" + strings.Repeat("x", 140), Parameters: json.RawMessage(`{}`)}},
				ToolChoice: ToolChoice{Mode: ToolChoiceFunction, Name: "vendor tool/" + strings.Repeat("x", 140)},
				Output:     OutputConstraint{Type: OutputJSONSchema, Name: "vendor schema/name", Schema: json.RawMessage(`{}`)},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.request.Validate(); err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
		})
	}
}

func TestRequestValidateRejectsInvalidFieldsAndTaggedUnions(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Request)
		want   string
	}{
		{name: "missing provider", mutate: func(r *Request) { r.Provider = "" }, want: "provider is required"},
		{name: "missing model", mutate: func(r *Request) { r.Model = " " }, want: "model is required"},
		{name: "unknown protocol", mutate: func(r *Request) { r.Protocol = "unknown" }, want: "unknown protocol"},
		{name: "relative endpoint", mutate: func(r *Request) { r.Endpoint = "/v1" }, want: "absolute HTTP(S)"},
		{name: "endpoint credentials", mutate: func(r *Request) { r.Endpoint = "https://user:secret@example.com/v1" }, want: "without credentials"},
		{name: "endpoint query", mutate: func(r *Request) { r.Endpoint = "https://example.com/v1?token=value" }, want: "query"},
		{name: "endpoint fragment", mutate: func(r *Request) { r.Endpoint = "https://example.com/v1#fragment" }, want: "fragment"},
		{name: "unsupported endpoint scheme", mutate: func(r *Request) { r.Endpoint = "ftp://example.com/v1" }, want: "absolute HTTP(S)"},
		{name: "empty input", mutate: func(r *Request) { r.Input = nil }, want: "at least one input"},
		{name: "instruction role", mutate: func(r *Request) { r.Instructions = []Instruction{{Role: RoleUser, Text: "bad"}} }, want: "invalid role"},
		{name: "instruction text", mutate: func(r *Request) { r.Instructions = []Instruction{{Role: RoleSystem}} }, want: "text is required"},
		{name: "union empty", mutate: func(r *Request) { r.Input = []InputItem{{Type: InputTypeMessage}} }, want: "exactly one union"},
		{name: "union multiple", mutate: func(r *Request) {
			r.Input = []InputItem{{Type: InputTypeMessage, Message: &Message{Role: RoleUser, Text: "x"}, FunctionResult: &FunctionResult{CallID: "call"}}}
		}, want: "exactly one union"},
		{name: "union tag mismatch", mutate: func(r *Request) {
			r.Input = []InputItem{{Type: InputTypeFunctionCall, Message: &Message{Role: RoleUser, Text: "x"}}}
		}, want: "tag does not match"},
		{name: "unknown input type", mutate: func(r *Request) { r.Input = []InputItem{{Type: "image", Message: &Message{Role: RoleUser, Text: "x"}}} }, want: "unknown input type"},
		{name: "invalid message role", mutate: func(r *Request) { r.Input = []InputItem{MessageInput("root", "x")} }, want: "invalid message role"},
		{name: "empty message", mutate: func(r *Request) { r.Input = []InputItem{MessageInput(RoleUser, " ")} }, want: "message text is required"},
		{name: "function call name", mutate: func(r *Request) { r.Input = []InputItem{FunctionCallInput("call", " ", json.RawMessage(`{}`))} }, want: "non-empty name"},
		{name: "function call arguments", mutate: func(r *Request) { r.Input = []InputItem{FunctionCallInput("call", "tool", json.RawMessage(`[]`))} }, want: "JSON object arguments"},
		{name: "function result identity", mutate: func(r *Request) { r.Input = []InputItem{FunctionResultInput("", "result", false)} }, want: "call id or name"},
		{name: "function result name", mutate: func(r *Request) { r.Input = []InputItem{NamedFunctionResultInput("call", " ", "result", false)} }, want: "name must be non-empty"},
		{name: "tool name", mutate: func(r *Request) { r.Tools = []Tool{{Name: " ", Parameters: json.RawMessage(`{}`)}} }, want: "name is required"},
		{name: "tool schema", mutate: func(r *Request) { r.Tools = []Tool{{Name: "tool", Parameters: json.RawMessage(`[]`)}} }, want: "JSON object"},
		{name: "duplicate tool", mutate: func(r *Request) {
			r.Tools = []Tool{{Name: "tool", Parameters: json.RawMessage(`{}`)}, {Name: "tool", Parameters: json.RawMessage(`{}`)}}
		}, want: "duplicate"},
		{name: "tool choice name in auto", mutate: func(r *Request) { r.ToolChoice = ToolChoice{Name: "tool"} }, want: "requires function mode"},
		{name: "required choice without tools", mutate: func(r *Request) { r.ToolChoice = ToolChoice{Mode: ToolChoiceRequired} }, want: "needs tools"},
		{name: "function choice undeclared", mutate: func(r *Request) { r.ToolChoice = ToolChoice{Mode: ToolChoiceFunction, Name: "tool"} }, want: "undeclared tool"},
		{name: "unknown tool choice", mutate: func(r *Request) { r.ToolChoice = ToolChoice{Mode: "random"} }, want: "unknown tool choice"},
		{name: "text output schema fields", mutate: func(r *Request) { r.Output = OutputConstraint{Name: "unexpected"} }, want: "must not include schema"},
		{name: "JSON object schema fields", mutate: func(r *Request) { r.Output = OutputConstraint{Type: OutputJSONObject, Schema: json.RawMessage(`{}`)} }, want: "must not include schema"},
		{name: "JSON schema missing name", mutate: func(r *Request) { r.Output = OutputConstraint{Type: OutputJSONSchema, Schema: json.RawMessage(`{}`)} }, want: "requires a non-empty name"},
		{name: "unknown output", mutate: func(r *Request) { r.Output.Type = "yaml" }, want: "unknown output constraint"},
		{name: "empty reasoning", mutate: func(r *Request) { r.Reasoning = &Reasoning{} }, want: "must request"},
		{name: "reasoning effort", mutate: func(r *Request) { r.Reasoning = &Reasoning{Effort: "extreme"} }, want: "unknown reasoning effort"},
		{name: "reasoning summary", mutate: func(r *Request) { r.Reasoning = &Reasoning{Summary: "full"} }, want: "unknown reasoning summary"},
		{name: "reasoning zero budget", mutate: func(r *Request) { zero := int64(0); r.Reasoning = &Reasoning{BudgetTokens: &zero} }, want: "must be positive"},
		{name: "reasoning none with summary", mutate: func(r *Request) { r.Reasoning = &Reasoning{Effort: ReasoningEffortNone, Summary: ReasoningSummaryAuto} }, want: "none cannot request"},
		{name: "state provider", mutate: func(r *Request) {
			r.State = &State{Kind: StateServerContinuation, Provider: "other", Protocol: ProtocolResponses, ID: "id"}
		}, want: "state provider"},
		{name: "state protocol", mutate: func(r *Request) {
			r.State = &State{Kind: StateServerContinuation, Provider: "test", Protocol: ProtocolAuto, ID: "id"}
		}, want: "state protocol is invalid"},
		{name: "server state id", mutate: func(r *Request) {
			r.State = &State{Kind: StateServerContinuation, Provider: "test", Protocol: ProtocolResponses}
		}, want: "requires id"},
		{name: "server state payload", mutate: func(r *Request) {
			r.State = &State{Kind: StateServerContinuation, Provider: "test", Protocol: ProtocolResponses, ID: "id", Payload: NewRawPayload([]byte(`{}`))}
		}, want: "no payload"},
		{name: "provider state payload", mutate: func(r *Request) {
			r.State = &State{Kind: StateProviderContinuation, Provider: "test", Protocol: ProtocolResponses}
		}, want: "requires payload"},
		{name: "negative tokens", mutate: func(r *Request) { r.Budget.MaxOutputTokens = -1 }, want: "must not be negative"},
		{name: "negative timeout", mutate: func(r *Request) { r.Budget.Timeout = -time.Nanosecond }, want: "must not be negative"},
		{name: "metadata empty key", mutate: func(r *Request) { r.Metadata = Metadata{"": "value"} }, want: "metadata keys"},
		{name: "provider option empty namespace", mutate: func(r *Request) { r.ProviderOptions = ProviderOptions{"": json.RawMessage(`{}`)} }, want: "non-empty provider ID"},
		{name: "provider option not object", mutate: func(r *Request) { r.ProviderOptions = ProviderOptions{"test": json.RawMessage(`[]`)} }, want: "JSON object"},
		{name: "provider option wrong namespace", mutate: func(r *Request) { r.ProviderOptions = ProviderOptions{"other": json.RawMessage(`{"x":1}`)} }, want: "namespace"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := validRequest()
			test.mutate(&request)
			err := request.Validate()
			if err == nil {
				t.Fatal("Validate() error = nil")
			}
			if ErrorKindOf(err) != ErrorInvalidRequest {
				t.Fatalf("ErrorKindOf(Validate()) = %q, want %q", ErrorKindOf(err), ErrorInvalidRequest)
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() error = %q, want substring %q", err, test.want)
			}
		})
	}
}

func TestInputConstructorsAndResponseAccessorsDefensivelyCopyJSON(t *testing.T) {
	arguments := json.RawMessage(`{"value":1}`)
	input := FunctionCallInput("call", "tool", arguments)
	arguments[2] = 'X'
	if got := string(input.FunctionCall.Arguments); got != `{"value":1}` {
		t.Fatalf("FunctionCallInput arguments = %q", got)
	}

	response := Response{Output: []OutputItem{
		{Type: OutputItemText, Text: "hello "},
		{Type: OutputItemFunctionCall, FunctionCall: input.FunctionCall},
		{Type: OutputItemText, Text: "world"},
	}}
	if got := response.Text(); got != "hello world" {
		t.Fatalf("Text() = %q, want %q", got, "hello world")
	}

	calls := response.FunctionCalls()
	if len(calls) != 1 {
		t.Fatalf("FunctionCalls() length = %d, want 1", len(calls))
	}
	calls[0].Arguments[2] = 'Y'
	if got := string(response.Output[1].FunctionCall.Arguments); got != `{"value":1}` {
		t.Fatalf("FunctionCalls() exposed response storage: %q", got)
	}
}
