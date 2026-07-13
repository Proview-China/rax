package unioncontract_test

import (
	"encoding/json"
	"math"
	"strings"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/union"
)

func TestRequestValidatesContextReferencesAndDeclaredToolAllowlist(t *testing.T) {
	base := validRequest()
	base.Context = []union.ContextReference{{
		ID: "context-1", Kind: "file", Reference: "artifact://workspace/config.go",
		Access: "read", Visibility: "model", Required: true,
	}}
	requireValid(t, "context reference", base.Validate)

	tests := []struct {
		name     string
		contains string
		mutate   func(*union.UnifiedExecutionRequest)
	}{
		{
			name: "duplicate context identity", contains: "context.id",
			mutate: func(request *union.UnifiedExecutionRequest) {
				request.Context = append(request.Context, request.Context[0])
			},
		},
		{
			name: "missing context access", contains: "context[0].access",
			mutate: func(request *union.UnifiedExecutionRequest) { request.Context[0].Access = "" },
		},
		{
			name: "unknown context kind", contains: "v1 context union",
			mutate: func(request *union.UnifiedExecutionRequest) { request.Context[0].Kind = "provider_blob" },
		},
		{
			name: "credential in context reference", contains: "credential material",
			mutate: func(request *union.UnifiedExecutionRequest) {
				request.Context[0].Reference = "https://example.invalid/?auth=Bearer abcdefghijklmnopqrstuvwxyz"
			},
		},
		{
			name: "undeclared allowed tool", contains: "declared tool",
			mutate: func(request *union.UnifiedExecutionRequest) {
				request.ToolPolicy.AllowedToolIDs = []string{"tool-missing"}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request, err := base.Clone()
			if err != nil {
				t.Fatal(err)
			}
			test.mutate(&request)
			requireInvalid(t, test.name, test.contains, request.Validate)
		})
	}
}

func TestControlMapsRejectHighConfidenceCredentialValuesAndNestedExtensions(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*union.UnifiedExecutionRequest)
	}{
		{
			name: "bearer in benign metadata key",
			mutate: func(request *union.UnifiedExecutionRequest) {
				request.Metadata["trace"] = "Bearer abcdefghijklmnopqrstuvwxyz012345"
			},
		},
		{
			name: "provider token in benign constraint key",
			mutate: func(request *union.UnifiedExecutionRequest) {
				request.ProfileSelector.Exact = nil
				request.ProfileSelector.Constraints = map[string]string{"model_id": "sk-ant-abcdefghijklmnopqrstuvwxyz012345"}
			},
		},
		{
			name: "nested credential key in extension",
			mutate: func(request *union.UnifiedExecutionRequest) {
				request.Extensions["routing"] = json.RawMessage(`{"nested":{"clientSecret":"redacted"}}`)
			},
		},
		{
			name: "jwt in extension array",
			mutate: func(request *union.UnifiedExecutionRequest) {
				request.Extensions["routing"] = json.RawMessage(`{"values":["eyJabcdefghijk.abcdefghijkl.abcdefghijkl"]}`)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := validRequest()
			test.mutate(&request)
			requireInvalid(t, test.name, "credential material", request.Validate)
		})
	}

	benign := validRequest()
	benign.Metadata["trace"] = "sketch-token-and-bearer-description"
	benign.Extensions["routing"] = json.RawMessage(`{"authorization_mode":"reference_only","token_count":4}`)
	requireValid(t, "benign control metadata", benign.Validate)
}

func TestInputAndContentUnionsAreClosedAndNativeExtensionsAreNamespaced(t *testing.T) {
	tests := []struct {
		name     string
		contains string
		mutate   func(*union.UnifiedExecutionRequest)
	}{
		{
			name: "unknown input kind", contains: "v1 input union",
			mutate: func(request *union.UnifiedExecutionRequest) { request.Input[0].Kind = "provider_message" },
		},
		{
			name: "unknown content kind", contains: "v1 content union",
			mutate: func(request *union.UnifiedExecutionRequest) { request.Input[0].Content[0].Kind = "provider_text" },
		},
		{
			name: "ambiguous text and json", contains: "only non-empty text",
			mutate: func(request *union.UnifiedExecutionRequest) {
				request.Input[0].Content[0].JSON = json.RawMessage(`{"extra":true}`)
			},
		},
		{
			name: "tool result without action", contains: "action_id",
			mutate: func(request *union.UnifiedExecutionRequest) {
				request.Input[0] = union.InputItem{ID: "tool-result", Kind: "tool_result", Payload: json.RawMessage(`{"ok":true}`)}
			},
		},
		{
			name: "native extension without namespace", contains: "namespaced extension",
			mutate: func(request *union.UnifiedExecutionRequest) {
				request.Input[0] = union.InputItem{ID: "native", Kind: "native_extension", Payload: json.RawMessage(`{"value":true}`)}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := validRequest()
			test.mutate(&request)
			requireInvalid(t, test.name, test.contains, request.Validate)
		})
	}

	valid := validRequest()
	valid.Input[0] = union.InputItem{
		ID: "native", Kind: "native_extension",
		Payload: json.RawMessage(`{"namespace":"openai.responses","value":{"mode":"provider-native"}}`),
	}
	requireValid(t, "namespaced native extension", valid.Validate)
}

func TestInstructionAuthorityScopeAndConflictPolicyAreClosed(t *testing.T) {
	for _, test := range []struct {
		name     string
		contains string
		mutate   func(*union.Instruction)
	}{
		{name: "forged authority", contains: "authority", mutate: func(value *union.Instruction) { value.Authority = "vendor_mandatory" }},
		{name: "unknown scope", contains: "scope", mutate: func(value *union.Instruction) { value.Scope = "provider_session" }},
		{name: "unknown conflict policy", contains: "conflict_policy", mutate: func(value *union.Instruction) { value.ConflictPolicy = "last_writer_wins" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := validRequest()
			test.mutate(&request.Instructions[0])
			requireInvalid(t, test.name, test.contains, request.Validate)
		})
	}
}

func TestEffectPayloadTargetAndToolExecutionProvenanceAreClosed(t *testing.T) {
	workspace := validEffect()
	workspace.Target = "/workspace/unrelated.txt"
	requireInvalid(t, "workspace target", "workspace change path", workspace.Validate)

	tool := union.EffectRecord{
		ID: "effect-tool", IntentIDs: []union.IntentID{"intent-tool"}, MechanismAttemptID: "attempt-tool",
		Kind: "tool_call_completed", Target: "tool-1",
		Payload: union.EffectPayload{ToolCall: &union.ToolCallEffect{
			ToolID: "tool-1", ActionID: "action-1", Mechanism: "caller_tool",
			Origin: union.CapabilityOriginCallerHosted, Owner: union.ExecutionOwnerPraxis, Executed: true,
			InputDigest: "sha256:input", ResultOrigin: union.EventOriginModel, SideEffectState: union.SideEffectNone,
		}},
		ObservationSource: "observer", VerificationStatus: union.VerificationUnverified, OccurredAt: contractTime,
	}
	requireInvalid(t, "model-origin tool result", "not model prose", tool.Validate)
	tool.Payload.ToolCall.ResultOrigin = union.EventOriginPraxis
	tool.Target = "other-tool"
	requireInvalid(t, "tool target", "executed tool identity", tool.Validate)
}

func TestUsageMetricRejectsNonFiniteNumbers(t *testing.T) {
	for _, value := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		result := validResult()
		result.UsageMetrics = []union.UsageMetric{{Kind: "tokens", Value: value, Unit: "token", Scope: "execution", Source: "provider", Quality: "reported"}}
		requireInvalid(t, "non-finite usage", "finite and non-negative", result.Validate)
	}
}

func TestResultClosesAttemptLineageAndVerificationAssociations(t *testing.T) {
	t.Run("retry must reference earlier attempt", func(t *testing.T) {
		result := validResult()
		result.MechanismTrace[0].RetryOf = "missing-attempt"
		requireInvalid(t, "retry lineage", "earlier result attempt", result.Validate)
	})

	t.Run("superseded attempt must be later", func(t *testing.T) {
		result := validResult()
		result.MechanismTrace[0].SupersededBy = result.MechanismTrace[0].ID
		requireInvalid(t, "attempt supersession", "later result attempt", result.Validate)
	})

	t.Run("effect verification reference is reciprocal", func(t *testing.T) {
		result := validResult()
		result.Verifications[0].EffectIDs = nil
		requireInvalid(t, "verification association", "same Effect", result.Validate)
	})

	t.Run("verification effect association is reciprocal", func(t *testing.T) {
		result := validResult()
		result.Status = union.ExecutionStatusPartial
		result.VerificationStatus = union.VerificationPartiallyVerified
		result.Effects[0].VerificationStatus = union.VerificationUnverified
		result.Effects[0].VerificationRefs = nil
		requireInvalid(t, "reverse verification association", "reciprocally referenced", result.Validate)
	})
}

func TestCrossIntentEffectCannotSupersedeAnotherIntent(t *testing.T) {
	result := validResult()
	result.Status = union.ExecutionStatusPartial
	result.VerificationStatus = union.VerificationPartiallyVerified
	result.IntentSatisfaction = append(result.IntentSatisfaction, union.IntentSatisfaction{
		IntentID: "intent-2", Status: union.IntentPartiallySatisfied, EffectIDs: []union.EffectID{"effect-2"},
	})
	effect2, err := result.Effects[0].Clone()
	if err != nil {
		t.Fatal(err)
	}
	effect2.ID = "effect-2"
	effect2.IntentIDs = []union.IntentID{"intent-2"}
	effect2.Target = "/workspace/intent-2.txt"
	effect2.Payload.WorkspaceChange.Path = effect2.Target
	effect2.Payload.WorkspaceChange.Before.Path = effect2.Target
	effect2.Payload.WorkspaceChange.After.Path = effect2.Target
	effect2.VerificationStatus = union.VerificationUnverified
	effect2.VerificationRefs = nil
	effect2.SupersedesEffectIDs = []union.EffectID{"effect-1"}
	result.Effects = append(result.Effects, effect2)
	requireInvalid(t, "cross-intent supersession", "share an intent", result.Validate)
}

func TestCredentialErrorsNeverEchoTheSecret(t *testing.T) {
	secret := "sk-ant-abcdefghijklmnopqrstuvwxyz012345"
	request := validRequest()
	request.Metadata["trace"] = secret
	err := request.Validate()
	if err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("credential error leaked secret: %v", err)
	}
}
