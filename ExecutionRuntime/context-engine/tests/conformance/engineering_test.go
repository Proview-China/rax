package conformance_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/sdk"
)

func TestContextEngineeringOperationClosureV1(t *testing.T) {
	operations := []sdk.ContextEngineeringOperationV1{
		sdk.EngineeringValidatePromptAssetV1,
		sdk.EngineeringPreviewPromptV1,
		sdk.EngineeringPrepareEvaluationV1,
		sdk.EngineeringAdmitEvaluationV1,
		sdk.EngineeringBuildFeedbackV1,
	}
	for _, operation := range operations {
		if err := operation.Validate(); err != nil {
			t.Fatalf("declared operation %q rejected: %v", operation, err)
		}
	}
	for _, forbidden := range []sdk.ContextEngineeringOperationV1{"publish", "remote_judge", "provider_call", "continue_turn", "runtime_settlement"} {
		if err := forbidden.Validate(); err == nil {
			t.Fatalf("cross-owner operation %q accepted", forbidden)
		}
	}
}

func TestContextEngineeringDTOsDoNotOwnRuntimeProviderOrHarnessV1(t *testing.T) {
	asset := testkit.PromptAssetV1()
	payload, err := json.Marshal(struct {
		Asset  any `json:"asset"`
		Limits any `json:"limits"`
	}{Asset: asset, Limits: sdk.DefaultContextEngineeringLimitsV1()})
	if err != nil {
		t.Fatal(err)
	}
	lower := strings.ToLower(string(payload))
	for _, forbidden := range []string{
		"runtime_settlement", "operation_permit", "provider_request", "provider_response",
		"harness_continuation", "turn_advance", "capability_registration", "production_root",
	} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("cross-owner field %q escaped into engineering SDK DTOs: %s", forbidden, payload)
		}
	}
}
