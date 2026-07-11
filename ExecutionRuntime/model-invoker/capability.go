package modelinvoker

import (
	"fmt"
	"sort"
	"strings"
)

type Capability string

const (
	CapabilityTextGeneration       Capability = "text_generation"
	CapabilityStreaming            Capability = "streaming"
	CapabilityToolCalling          Capability = "tool_calling"
	CapabilityParallelToolCalling  Capability = "parallel_tool_calling"
	CapabilityStructuredOutput     Capability = "structured_output"
	CapabilityReasoning            Capability = "reasoning"
	CapabilityReasoningSummary     Capability = "reasoning_summary"
	CapabilityFunctionErrorResult  Capability = "function_error_result"
	CapabilityVisionInput          Capability = "vision_input"
	CapabilityAudioInput           Capability = "audio_input"
	CapabilityVideoInput           Capability = "video_input"
	CapabilityFileInput            Capability = "file_input"
	CapabilityServerState          Capability = "server_state"
	CapabilityProviderContinuation Capability = "provider_continuation"
	CapabilityPromptCaching        Capability = "prompt_caching"
	CapabilityBatch                Capability = "batch"
	CapabilityBackgroundExecution  Capability = "background_execution"
	CapabilityRealtime             Capability = "realtime"
	CapabilityHostedTools          Capability = "hosted_tools"
	CapabilityUsageReporting       Capability = "usage_reporting"
)

type SupportLevel string

const (
	SupportNative      SupportLevel = "native"
	SupportCompatible  SupportLevel = "compatible"
	SupportPartial     SupportLevel = "partial"
	SupportUnsupported SupportLevel = "unsupported"
)

type CapabilitySupport struct {
	Level       SupportLevel
	Limitations []string
	Protocols   []Protocol
	Models      []string
}

type CapabilityContract map[Capability]CapabilitySupport

var allCapabilities = []Capability{
	CapabilityTextGeneration,
	CapabilityStreaming,
	CapabilityToolCalling,
	CapabilityParallelToolCalling,
	CapabilityStructuredOutput,
	CapabilityReasoning,
	CapabilityReasoningSummary,
	CapabilityFunctionErrorResult,
	CapabilityVisionInput,
	CapabilityAudioInput,
	CapabilityVideoInput,
	CapabilityFileInput,
	CapabilityServerState,
	CapabilityProviderContinuation,
	CapabilityPromptCaching,
	CapabilityBatch,
	CapabilityBackgroundExecution,
	CapabilityRealtime,
	CapabilityHostedTools,
	CapabilityUsageReporting,
}

// AllCapabilities returns the complete public capability vocabulary. The
// returned slice is a defensive copy and may be modified by the caller.
func AllCapabilities() []Capability {
	return append([]Capability(nil), allCapabilities...)
}

type MappingAction string

const (
	MappingExact       MappingAction = "exact"
	MappingTransformed MappingAction = "transformed"
	MappingDegraded    MappingAction = "degraded"
	MappingRejected    MappingAction = "rejected"
)

type MappingDecision struct {
	Capability Capability
	Action     MappingAction
	Detail     string
}

type MappingReport struct {
	Provider  ProviderID
	Protocol  Protocol
	Endpoint  string
	Decisions []MappingDecision
}

func (r MappingReport) HasDegradation() bool {
	for _, decision := range r.Decisions {
		if decision.Action == MappingDegraded {
			return true
		}
	}
	return false
}

func RequiredCapabilities(request Request) []Capability {
	required := map[Capability]struct{}{
		CapabilityTextGeneration: {},
		CapabilityUsageReporting: {},
	}
	if request.Stream {
		required[CapabilityStreaming] = struct{}{}
	}
	if len(request.Tools) > 0 {
		required[CapabilityToolCalling] = struct{}{}
	}
	for _, input := range request.Input {
		if input.Type == InputTypeFunctionCall || input.Type == InputTypeFunctionResult {
			required[CapabilityToolCalling] = struct{}{}
		}
	}
	if request.ParallelToolCalls != nil && *request.ParallelToolCalls {
		required[CapabilityParallelToolCalling] = struct{}{}
	}
	if request.Output.Type == OutputJSONObject || request.Output.Type == OutputJSONSchema {
		required[CapabilityStructuredOutput] = struct{}{}
	}
	if request.Reasoning != nil {
		required[CapabilityReasoning] = struct{}{}
		if request.Reasoning.Summary != "" {
			required[CapabilityReasoningSummary] = struct{}{}
		}
	}
	for _, input := range request.Input {
		if input.Type == InputTypeFunctionResult && input.FunctionResult != nil && input.FunctionResult.IsError {
			required[CapabilityFunctionErrorResult] = struct{}{}
		}
	}
	if request.State != nil {
		switch request.State.Kind {
		case StateServerContinuation:
			required[CapabilityServerState] = struct{}{}
		case StateProviderContinuation:
			required[CapabilityProviderContinuation] = struct{}{}
		}
	}

	result := make([]Capability, 0, len(required))
	for capability := range required {
		result = append(result, capability)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func EvaluateCapabilities(request Request, contract CapabilityContract) (MappingReport, error) {
	report := MappingReport{Provider: request.Provider, Protocol: request.Protocol, Endpoint: request.Endpoint}
	var evaluationError error
	for _, capability := range RequiredCapabilities(request) {
		support, ok := contract[capability]
		if !ok {
			support = CapabilitySupport{Level: SupportUnsupported, Limitations: []string{"capability is not declared"}}
		}
		support.Limitations = append([]string(nil), support.Limitations...)
		if len(support.Protocols) > 0 && !containsProtocol(support.Protocols, request.Protocol) {
			support.Level = SupportUnsupported
			support.Limitations = append(support.Limitations, fmt.Sprintf("protocol %s is not supported", request.Protocol))
		}
		if len(support.Models) > 0 && !containsString(support.Models, request.Model) {
			support.Level = SupportUnsupported
			support.Limitations = append(support.Limitations, fmt.Sprintf("model %s is not supported", request.Model))
		}
		detail := strings.Join(support.Limitations, "; ")
		switch support.Level {
		case SupportNative:
			report.Decisions = append(report.Decisions, MappingDecision{Capability: capability, Action: MappingExact, Detail: detail})
		case SupportCompatible:
			report.Decisions = append(report.Decisions, MappingDecision{Capability: capability, Action: MappingTransformed, Detail: detail})
		case SupportPartial:
			if request.AllowDegradation {
				report.Decisions = append(report.Decisions, MappingDecision{Capability: capability, Action: MappingDegraded, Detail: detail})
				continue
			}
			report.Decisions = append(report.Decisions, MappingDecision{Capability: capability, Action: MappingRejected, Detail: detail})
			if evaluationError == nil {
				evaluationError = unsupportedCapabilityError(request.Provider, capability, "partial support requires explicit degradation permission", detail)
			}
		case SupportUnsupported, "":
			report.Decisions = append(report.Decisions, MappingDecision{Capability: capability, Action: MappingRejected, Detail: detail})
			if evaluationError == nil {
				evaluationError = unsupportedCapabilityError(request.Provider, capability, "capability is unsupported", detail)
			}
		default:
			report.Decisions = append(report.Decisions, MappingDecision{
				Capability: capability, Action: MappingRejected,
				Detail: fmt.Sprintf("unknown support level %q", support.Level),
			})
			if evaluationError == nil {
				evaluationError = &Error{
					Kind: ErrorMapping, Provider: request.Provider, Operation: "capability_check",
					Code: string(capability), Message: fmt.Sprintf("capability %s has unknown support level %q", capability, support.Level),
				}
			}
		}
	}
	return report, evaluationError
}

func containsProtocol(values []Protocol, target Protocol) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func unsupportedCapabilityError(provider ProviderID, capability Capability, message, detail string) *Error {
	if detail != "" {
		message += ": " + detail
	}
	return &Error{
		Kind:      ErrorUnsupportedCapability,
		Provider:  provider,
		Operation: "capability_check",
		Code:      string(capability),
		Message:   message,
	}
}
