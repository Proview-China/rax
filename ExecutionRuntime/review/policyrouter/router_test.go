package policyrouter_test

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/policyrouter"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func TestSafeBaselineMatrixV1(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	base := contract.RouteInputV1{ToolCapability: "praxis.tool/future-capability", Profile: contract.ProfileStandardV1, Risk: contract.RiskLowV1, EffectClass: contract.EffectObserveOnlyV1, Environment: contract.EnvironmentProductionV1, EvidenceSufficient: true, Current: true, CheckedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(time.Minute).UnixNano()}
	tests := []struct {
		name      string
		edit      func(*contract.RouteInputV1)
		kind      contract.RouteDecisionKindV1
		errReason core.ReasonCode
	}{
		{"restricted-persistent-low", func(v *contract.RouteInputV1) {
			v.Profile = contract.ProfileRestrictedV1
			v.EffectClass = contract.EffectPersistentV1
		}, contract.RouteDecisionHumanV1, ""},
		{"standard-observe-bypass", func(v *contract.RouteInputV1) { v.BypassAllowed = true }, contract.RouteDecisionBypassV1, ""},
		{"permissive-high", func(v *contract.RouteInputV1) {
			v.Profile = contract.ProfilePermissiveV1
			v.Risk = contract.RiskHighV1
			v.EffectClass = contract.EffectReversibleV1
		}, contract.RouteDecisionAutoV1, ""},
		{"permissive-medium", func(v *contract.RouteInputV1) {
			v.Profile = contract.ProfilePermissiveV1
			v.Risk = contract.RiskMediumV1
			v.EffectClass = contract.EffectReversibleV1
		}, contract.RouteDecisionAutoV1, ""},
		{"permissive-low-auto", func(v *contract.RouteInputV1) {
			v.Profile = contract.ProfilePermissiveV1
		}, contract.RouteDecisionAutoV1, ""},
		{"critical", func(v *contract.RouteInputV1) { v.Risk = contract.RiskCriticalV1; v.BypassAllowed = true }, contract.RouteDecisionHumanV1, ""},
		{"irreversible", func(v *contract.RouteInputV1) { v.EffectClass = contract.EffectIrreversibleV1; v.BypassAllowed = true }, contract.RouteDecisionHumanV1, ""},
		{"yolo-no-bypass", func(v *contract.RouteInputV1) { v.Profile = contract.ProfileYOLOV1 }, contract.RouteDecisionAutoV1, ""},
		{"yolo-explicit-not-required", func(v *contract.RouteInputV1) {
			v.Profile = contract.ProfileYOLOV1
			v.BypassAllowed = true
		}, contract.RouteDecisionBypassV1, ""},
		{"yolo-high-cannot-bypass", func(v *contract.RouteInputV1) {
			v.Profile = contract.ProfileYOLOV1
			v.Risk = contract.RiskHighV1
			v.BypassAllowed = true
		}, contract.RouteDecisionAutoV1, ""},
		{"bapr-production", func(v *contract.RouteInputV1) { v.Profile = contract.ProfileBAPRV1 }, "", core.ReasonUnknownGovernanceCategory},
		{"human-required", func(v *contract.RouteInputV1) { v.HumanRequired = true }, contract.RouteDecisionHumanV1, ""},
		{"insufficient-evidence", func(v *contract.RouteInputV1) { v.EvidenceSufficient = false }, contract.RouteDecisionHumanV1, ""},
		{"observe-only-evidence-downgrade", func(v *contract.RouteInputV1) {
			v.EvidenceSufficient = false
			v.ObserveOnlyDowngradeAllowed = true
		}, contract.RouteDecisionAutoV1, ""},
		{"bapr-hard-human-before-environment", func(v *contract.RouteInputV1) {
			v.Profile = contract.ProfileBAPRV1
			v.HumanRequired = true
		}, contract.RouteDecisionHumanV1, ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := base
			test.edit(&input)
			got, err := policyrouter.DecideV1(input, now)
			if test.errReason != "" {
				if !core.HasReason(err, test.errReason) {
					t.Fatalf("expected %s, got %v", test.errReason, err)
				}
				return
			}
			if err != nil || got.Kind != test.kind || got.Validate() != nil {
				t.Fatalf("unexpected route: %+v err=%v", got, err)
			}
			if got.Kind == contract.RouteDecisionBypassV1 && (!got.OperationNotRequired || got.ReviewerRoute != "") {
				t.Fatalf("bypass was encoded as a reviewer verdict: %+v", got)
			}
		})
	}
}

func TestFrozenProfileRiskMatrixV1(t *testing.T) {
	now := time.Unix(1_500_000, 0)
	profiles := []contract.ProfileV1{
		contract.ProfileRestrictedV1,
		contract.ProfileStandardV1,
		contract.ProfilePermissiveV1,
		contract.ProfileYOLOV1,
		contract.ProfileBAPRV1,
	}
	risks := []contract.RiskLevelV1{
		contract.RiskLowV1,
		contract.RiskMediumV1,
		contract.RiskHighV1,
		contract.RiskCriticalV1,
	}
	want := map[contract.ProfileV1][]contract.RouteDecisionKindV1{
		contract.ProfileRestrictedV1: {contract.RouteDecisionAutoV1, contract.RouteDecisionHumanV1, contract.RouteDecisionHumanV1, contract.RouteDecisionHumanV1},
		contract.ProfileStandardV1:   {contract.RouteDecisionAutoV1, contract.RouteDecisionAutoV1, contract.RouteDecisionHumanV1, contract.RouteDecisionHumanV1},
		contract.ProfilePermissiveV1: {contract.RouteDecisionAutoV1, contract.RouteDecisionAutoV1, contract.RouteDecisionAutoV1, contract.RouteDecisionHumanV1},
		contract.ProfileYOLOV1:       {contract.RouteDecisionAutoV1, contract.RouteDecisionAutoV1, contract.RouteDecisionAutoV1, contract.RouteDecisionHumanV1},
		contract.ProfileBAPRV1:       {contract.RouteDecisionAutoV1, contract.RouteDecisionAutoV1, contract.RouteDecisionAutoV1, contract.RouteDecisionHumanV1},
	}
	for _, profile := range profiles {
		for index, risk := range risks {
			t.Run(string(profile)+"/"+string(risk), func(t *testing.T) {
				environment := contract.EnvironmentProductionV1
				if profile == contract.ProfileBAPRV1 {
					environment = contract.EnvironmentTestV1
				}
				input := contract.RouteInputV1{
					ToolCapability: "praxis.tool/future", Profile: profile, Risk: risk,
					EffectClass: contract.EffectReversibleV1, Environment: environment,
					EvidenceSufficient: true, Current: true,
					CheckedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(time.Minute).UnixNano(),
				}
				got, err := policyrouter.DecideV1(input, now)
				if err != nil || got.Kind != want[profile][index] {
					t.Fatalf("matrix drift: got=%+v err=%v want=%s", got, err, want[profile][index])
				}
			})
		}
	}
}

func TestFrozenBypassAndEffectMatrixV1(t *testing.T) {
	now := time.Unix(1_750_000, 0)
	base := contract.RouteInputV1{
		ToolCapability: "praxis.tool/future", Risk: contract.RiskLowV1,
		EffectClass: contract.EffectObserveOnlyV1, Environment: contract.EnvironmentProductionV1,
		EvidenceSufficient: true, BypassAllowed: true, Current: true,
		CheckedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(time.Minute).UnixNano(),
	}
	tests := []struct {
		name   string
		edit   func(*contract.RouteInputV1)
		want   contract.RouteDecisionKindV1
		reason core.ReasonCode
	}{
		{"restricted-low-observe", func(v *contract.RouteInputV1) { v.Profile = contract.ProfileRestrictedV1 }, contract.RouteDecisionAutoV1, ""},
		{"restricted-low-persistent", func(v *contract.RouteInputV1) {
			v.Profile = contract.ProfileRestrictedV1
			v.EffectClass = contract.EffectPersistentV1
		}, contract.RouteDecisionHumanV1, ""},
		{"standard-low-observe", func(v *contract.RouteInputV1) { v.Profile = contract.ProfileStandardV1 }, contract.RouteDecisionBypassV1, ""},
		{"standard-low-persistent", func(v *contract.RouteInputV1) {
			v.Profile = contract.ProfileStandardV1
			v.EffectClass = contract.EffectPersistentV1
		}, contract.RouteDecisionAutoV1, ""},
		{"standard-medium-observe", func(v *contract.RouteInputV1) { v.Profile = contract.ProfileStandardV1; v.Risk = contract.RiskMediumV1 }, contract.RouteDecisionAutoV1, ""},
		{"permissive-low-persistent", func(v *contract.RouteInputV1) {
			v.Profile = contract.ProfilePermissiveV1
			v.EffectClass = contract.EffectPersistentV1
		}, contract.RouteDecisionBypassV1, ""},
		{"permissive-medium", func(v *contract.RouteInputV1) {
			v.Profile = contract.ProfilePermissiveV1
			v.Risk = contract.RiskMediumV1
		}, contract.RouteDecisionAutoV1, ""},
		{"permissive-high", func(v *contract.RouteInputV1) { v.Profile = contract.ProfilePermissiveV1; v.Risk = contract.RiskHighV1 }, contract.RouteDecisionAutoV1, ""},
		{"yolo-low", func(v *contract.RouteInputV1) { v.Profile = contract.ProfileYOLOV1 }, contract.RouteDecisionBypassV1, ""},
		{"yolo-medium", func(v *contract.RouteInputV1) { v.Profile = contract.ProfileYOLOV1; v.Risk = contract.RiskMediumV1 }, contract.RouteDecisionBypassV1, ""},
		{"yolo-high", func(v *contract.RouteInputV1) { v.Profile = contract.ProfileYOLOV1; v.Risk = contract.RiskHighV1 }, contract.RouteDecisionAutoV1, ""},
		{"bapr-low-test", func(v *contract.RouteInputV1) {
			v.Profile = contract.ProfileBAPRV1
			v.Environment = contract.EnvironmentTestV1
		}, contract.RouteDecisionBypassV1, ""},
		{"bapr-medium-development", func(v *contract.RouteInputV1) {
			v.Profile = contract.ProfileBAPRV1
			v.Environment = contract.EnvironmentDevelopmentV1
			v.Risk = contract.RiskMediumV1
		}, contract.RouteDecisionAutoV1, ""},
		{"bapr-staging", func(v *contract.RouteInputV1) {
			v.Profile = contract.ProfileBAPRV1
			v.Environment = contract.EnvironmentStagingV1
		}, "", core.ReasonUnknownGovernanceCategory},
		{"bapr-production", func(v *contract.RouteInputV1) { v.Profile = contract.ProfileBAPRV1 }, "", core.ReasonUnknownGovernanceCategory},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := base
			test.edit(&input)
			got, err := policyrouter.DecideV1(input, now)
			if test.reason != "" {
				if !core.HasReason(err, test.reason) || got.Kind != "" {
					t.Fatalf("expected %s with zero route, got=%+v err=%v", test.reason, got, err)
				}
				return
			}
			if err != nil || got.Kind != test.want {
				t.Fatalf("route drift: got=%+v err=%v want=%s", got, err, test.want)
			}
		})
	}

	for _, profile := range []contract.ProfileV1{contract.ProfileRestrictedV1, contract.ProfileStandardV1, contract.ProfilePermissiveV1, contract.ProfileYOLOV1, contract.ProfileBAPRV1} {
		t.Run(string(profile)+"/irreversible-hard-human", func(t *testing.T) {
			input := base
			input.Profile = profile
			input.EffectClass = contract.EffectIrreversibleV1
			got, err := policyrouter.DecideV1(input, now)
			if err != nil || got.Kind != contract.RouteDecisionHumanV1 {
				t.Fatalf("irreversible route drift: got=%+v err=%v", got, err)
			}
		})
	}
}

func TestFutureToolIdentityDoesNotChangeRouteV1(t *testing.T) {
	now := time.Unix(2_000_000, 0)
	input := contract.RouteInputV1{ToolCapability: "praxis.tool/old", Profile: contract.ProfilePermissiveV1, Risk: contract.RiskMediumV1, EffectClass: contract.EffectPersistentV1, Environment: contract.EnvironmentProductionV1, EvidenceSufficient: true, Current: true, CheckedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(time.Minute).UnixNano()}
	oldDecision, err := policyrouter.DecideV1(input, now)
	if err != nil {
		t.Fatal(err)
	}
	input.ToolCapability = "vendor.future/new-tool-that-did-not-exist"
	newDecision, err := policyrouter.DecideV1(input, now)
	if err != nil {
		t.Fatal(err)
	}
	if oldDecision.Kind != newDecision.Kind || oldDecision.ReviewerRoute != newDecision.ReviewerRoute || oldDecision.OperationNotRequired != newDecision.OperationNotRequired {
		t.Fatalf("tool identity changed policy route: old=%+v new=%+v", oldDecision, newDecision)
	}
}

func TestRouteCurrentnessFailsClosedV1(t *testing.T) {
	now := time.Unix(3_000_000, 0)
	input := contract.RouteInputV1{ToolCapability: "praxis.tool/x", Profile: contract.ProfileStandardV1, Risk: contract.RiskLowV1, EffectClass: contract.EffectObserveOnlyV1, Environment: contract.EnvironmentProductionV1, EvidenceSufficient: true, Current: true, CheckedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(time.Minute).UnixNano()}
	for name, edit := range map[string]func(*contract.RouteInputV1){
		"not-current": func(v *contract.RouteInputV1) { v.Current = false },
		"expired":     func(v *contract.RouteInputV1) { v.ExpiresUnixNano = now.UnixNano() },
		"rollback": func(v *contract.RouteInputV1) {
			v.CheckedUnixNano = now.Add(time.Second).UnixNano()
			v.ExpiresUnixNano = now.Add(time.Minute).UnixNano()
		},
	} {
		t.Run(name, func(t *testing.T) {
			value := input
			edit(&value)
			if got, err := policyrouter.DecideV1(value, now); err == nil || got.Kind != "" || got.Digest != "" || len(got.ReasonCodes) != 0 {
				t.Fatalf("invalid current input produced a route: %+v err=%v", got, err)
			}
		})
	}
}
