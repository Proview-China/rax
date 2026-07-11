package core_test

import (
	"errors"
	"reflect"
	"sync"
	"testing"

	. "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
)

func nativeContract(capabilities ...Capability) CapabilityContract {
	contract := CapabilityContract{
		CapabilityTextGeneration: {Level: SupportNative},
		CapabilityUsageReporting: {Level: SupportNative},
	}
	for _, capability := range capabilities {
		contract[capability] = CapabilitySupport{Level: SupportNative}
	}
	return contract
}

func TestRequiredCapabilitiesIsSortedAndDeterministic(t *testing.T) {
	parallel := true
	request := validRequest()
	request.Stream = true
	request.Tools = []Tool{{Name: "tool", Parameters: []byte(`{}`)}}
	request.ParallelToolCalls = &parallel
	request.Output = OutputConstraint{Type: OutputJSONObject}
	request.Reasoning = &Reasoning{Effort: ReasoningEffortLow}
	request.State = &State{Kind: StateServerContinuation, Provider: request.Provider, Protocol: ProtocolResponses, ID: "resp_previous"}

	want := []Capability{
		CapabilityParallelToolCalling,
		CapabilityReasoning,
		CapabilityServerState,
		CapabilityStreaming,
		CapabilityStructuredOutput,
		CapabilityTextGeneration,
		CapabilityToolCalling,
		CapabilityUsageReporting,
	}
	for run := 0; run < 100; run++ {
		if got := RequiredCapabilities(request); !reflect.DeepEqual(got, want) {
			t.Fatalf("RequiredCapabilities() = %#v, want %#v", got, want)
		}
	}
}

func TestRequiredCapabilitiesDerivesToolUseFromHistory(t *testing.T) {
	falseValue := false
	tests := []struct {
		name    string
		request Request
		want    []Capability
	}{
		{
			name:    "function call",
			request: Request{Input: []InputItem{FunctionCallInput("call", "tool", []byte(`{}`))}},
			want:    []Capability{CapabilityTextGeneration, CapabilityToolCalling, CapabilityUsageReporting},
		},
		{
			name:    "function result",
			request: Request{Input: []InputItem{FunctionResultInput("call", "done", false)}},
			want:    []Capability{CapabilityTextGeneration, CapabilityToolCalling, CapabilityUsageReporting},
		},
		{
			name:    "function error result",
			request: Request{Input: []InputItem{FunctionResultInput("call", "failed", true)}},
			want:    []Capability{CapabilityFunctionErrorResult, CapabilityTextGeneration, CapabilityToolCalling, CapabilityUsageReporting},
		},
		{
			name:    "parallel explicitly false",
			request: Request{ParallelToolCalls: &falseValue},
			want:    []Capability{CapabilityTextGeneration, CapabilityUsageReporting},
		},
		{
			name:    "reasoning summary",
			request: Request{Reasoning: &Reasoning{Summary: ReasoningSummaryAuto}},
			want:    []Capability{CapabilityReasoning, CapabilityReasoningSummary, CapabilityTextGeneration, CapabilityUsageReporting},
		},
		{
			name: "provider continuation",
			request: Request{State: &State{
				Kind: StateProviderContinuation, Payload: NewRawPayload([]byte(`{}`)),
			}},
			want: []Capability{CapabilityProviderContinuation, CapabilityTextGeneration, CapabilityUsageReporting},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := RequiredCapabilities(test.request); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("RequiredCapabilities() = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestEvaluateCapabilitiesMapsAllFourSupportLevels(t *testing.T) {
	tests := []struct {
		name             string
		support          *CapabilitySupport
		allowDegradation bool
		wantAction       MappingAction
		wantKind         ErrorKind
		wantDegradation  bool
	}{
		{name: "native", support: &CapabilitySupport{Level: SupportNative}, wantAction: MappingExact},
		{name: "compatible", support: &CapabilitySupport{Level: SupportCompatible, Limitations: []string{"translated field"}}, wantAction: MappingTransformed},
		{name: "partial allowed", support: &CapabilitySupport{Level: SupportPartial, Limitations: []string{"format restricted", "size restricted"}}, allowDegradation: true, wantAction: MappingDegraded, wantDegradation: true},
		{name: "partial denied", support: &CapabilitySupport{Level: SupportPartial, Limitations: []string{"format restricted"}}, wantAction: MappingRejected, wantKind: ErrorUnsupportedCapability},
		{name: "unsupported", support: &CapabilitySupport{Level: SupportUnsupported, Limitations: []string{"not implemented"}}, wantAction: MappingRejected, wantKind: ErrorUnsupportedCapability},
		{name: "missing", support: nil, wantAction: MappingRejected, wantKind: ErrorUnsupportedCapability},
		{name: "unknown level", support: &CapabilitySupport{Level: "mostly"}, wantKind: ErrorMapping},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := validRequest()
			request.Protocol = ProtocolResponses
			request.Stream = true
			request.AllowDegradation = test.allowDegradation
			contract := nativeContract()
			if test.support != nil {
				contract[CapabilityStreaming] = *test.support
			}

			report, err := EvaluateCapabilities(request, contract)
			if got := ErrorKindOf(err); got != test.wantKind {
				t.Fatalf("ErrorKindOf(EvaluateCapabilities()) = %q, want %q (err=%v)", got, test.wantKind, err)
			}
			if report.Provider != request.Provider || report.Protocol != request.Protocol || report.Endpoint != request.Endpoint {
				t.Fatalf("report identity = (%q, %q), want (%q, %q)", report.Provider, report.Protocol, request.Provider, request.Protocol)
			}
			if test.wantAction != "" {
				decision, ok := findDecision(report, CapabilityStreaming)
				if !ok {
					t.Fatalf("streaming decision missing from %#v", report.Decisions)
				}
				if decision.Action != test.wantAction {
					t.Fatalf("streaming action = %q, want %q", decision.Action, test.wantAction)
				}
				if test.support != nil && len(test.support.Limitations) > 1 && decision.Detail != "format restricted; size restricted" {
					t.Fatalf("decision detail = %q", decision.Detail)
				}
			}
			if got := report.HasDegradation(); got != test.wantDegradation {
				t.Fatalf("HasDegradation() = %v, want %v", got, test.wantDegradation)
			}
			if test.wantKind == ErrorUnsupportedCapability && err != nil {
				var invocationError *Error
				if !errors.As(err, &invocationError) {
					t.Fatalf("error type = %T, want *Error", err)
				}
				if invocationError.Code != string(CapabilityStreaming) {
					t.Fatalf("error code = %q, want %q", invocationError.Code, CapabilityStreaming)
				}
			}
		})
	}
}

func TestEvaluateCapabilitiesCollectsEveryDecisionBeforeRejecting(t *testing.T) {
	request := validRequest()
	request.Endpoint = "https://example.test/v1"
	request.Output = OutputConstraint{Type: OutputJSONObject}
	request.Reasoning = &Reasoning{Effort: ReasoningEffortHigh}
	required := RequiredCapabilities(request)
	report, err := EvaluateCapabilities(request, nativeContract())
	if ErrorKindOf(err) != ErrorUnsupportedCapability {
		t.Fatalf("ErrorKind = %q, err=%v", ErrorKindOf(err), err)
	}
	if len(report.Decisions) != len(required) || report.Endpoint != request.Endpoint {
		t.Fatalf("report = %#v, required = %#v", report, required)
	}
	rejected := 0
	for _, decision := range report.Decisions {
		if decision.Action == MappingRejected {
			rejected++
		}
	}
	if rejected != 2 {
		t.Fatalf("rejected decisions = %d, report=%#v", rejected, report)
	}
}

func TestEvaluateCapabilitiesDoesNotMutateSharedLimitations(t *testing.T) {
	request := validRequest()
	request.Stream = true
	limitations := make([]string, 1, 8)
	limitations[0] = "shared"
	contract := nativeContract()
	contract[CapabilityStreaming] = CapabilitySupport{
		Level: SupportNative, Protocols: []Protocol{ProtocolChatCompletions}, Limitations: limitations,
	}

	var wait sync.WaitGroup
	for index := 0; index < 64; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, _ = EvaluateCapabilities(request, contract)
		}()
	}
	wait.Wait()
	if len(contract[CapabilityStreaming].Limitations) != 1 || contract[CapabilityStreaming].Limitations[0] != "shared" {
		t.Fatalf("shared limitations mutated: %#v", contract[CapabilityStreaming].Limitations)
	}
}

func TestEvaluateCapabilitiesHonorsProtocolAndModelRestrictions(t *testing.T) {
	tests := []struct {
		name     string
		support  CapabilitySupport
		protocol Protocol
		model    string
		wantKind ErrorKind
	}{
		{
			name:     "matching protocol and model",
			support:  CapabilitySupport{Level: SupportNative, Protocols: []Protocol{ProtocolResponses}, Models: []string{"model-a"}},
			protocol: ProtocolResponses,
			model:    "model-a",
		},
		{
			name:     "protocol mismatch",
			support:  CapabilitySupport{Level: SupportNative, Protocols: []Protocol{ProtocolChatCompletions}},
			protocol: ProtocolResponses,
			model:    "model-a",
			wantKind: ErrorUnsupportedCapability,
		},
		{
			name:     "model mismatch",
			support:  CapabilitySupport{Level: SupportNative, Models: []string{"model-b"}},
			protocol: ProtocolResponses,
			model:    "model-a",
			wantKind: ErrorUnsupportedCapability,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := validRequest()
			request.Protocol = test.protocol
			request.Model = test.model
			request.Stream = true
			contract := nativeContract()
			contract[CapabilityStreaming] = test.support
			_, err := EvaluateCapabilities(request, contract)
			if got := ErrorKindOf(err); got != test.wantKind {
				t.Fatalf("ErrorKindOf(EvaluateCapabilities()) = %q, want %q (err=%v)", got, test.wantKind, err)
			}
		})
	}
}

func TestEvaluateCapabilitiesDecisionOrderDoesNotDependOnMapIteration(t *testing.T) {
	parallel := true
	request := validRequest()
	request.Stream = true
	request.Tools = []Tool{{Name: "tool", Parameters: []byte(`{}`)}}
	request.ParallelToolCalls = &parallel
	request.Output = OutputConstraint{Type: OutputJSONObject}
	request.Reasoning = &Reasoning{Effort: ReasoningEffortLow}
	request.State = &State{Kind: StateServerContinuation, Provider: request.Provider, Protocol: ProtocolResponses, ID: "previous"}
	contract := nativeContract(RequiredCapabilities(request)...)

	want := RequiredCapabilities(request)
	for run := 0; run < 100; run++ {
		report, err := EvaluateCapabilities(request, contract)
		if err != nil {
			t.Fatalf("EvaluateCapabilities() error = %v", err)
		}
		got := make([]Capability, len(report.Decisions))
		for index, decision := range report.Decisions {
			got[index] = decision.Capability
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("decision order = %#v, want %#v", got, want)
		}
	}
}

func findDecision(report MappingReport, capability Capability) (MappingDecision, bool) {
	for _, decision := range report.Decisions {
		if decision.Capability == capability {
			return decision, true
		}
	}
	return MappingDecision{}, false
}
