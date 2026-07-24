package policyrouter

import (
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

// DecideV1 applies the frozen matrix to an already assembled input. V1 is a
// pure reference/test surface: its Current flag is not an Owner-current proof,
// so production composition must not call it until a trusted aggregator has
// reread the exact Policy, Target, Authority and Scope inputs. It deliberately
// ignores a capability's identity after shape validation: future tools with
// the same risk/effect policy receive the same route and no tool permission is
// minted.
func DecideV1(input contract.RouteInputV1, now time.Time) (contract.RouteDecisionV1, error) {
	if err := input.ValidateCurrent(now); err != nil {
		return contract.RouteDecisionV1{}, err
	}
	if input.HumanRequired || input.Risk == contract.RiskCriticalV1 || input.EffectClass == contract.EffectIrreversibleV1 || (!input.EvidenceSufficient && (input.EffectClass != contract.EffectObserveOnlyV1 || !input.ObserveOnlyDowngradeAllowed)) {
		return human("review.route/hard-human")
	}
	if input.Profile == contract.ProfileBAPRV1 && input.Environment != contract.EnvironmentDevelopmentV1 && input.Environment != contract.EnvironmentTestV1 {
		return contract.RouteDecisionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonUnknownGovernanceCategory, "bapr is restricted to development and test")
	}

	switch input.Profile {
	case contract.ProfileRestrictedV1:
		if input.Risk != contract.RiskLowV1 || input.EffectClass == contract.EffectPersistentV1 {
			return human("review.route/restricted-human")
		}
		return auto("review.route/restricted-auto")
	case contract.ProfileStandardV1:
		if input.Risk == contract.RiskHighV1 {
			return human("review.route/standard-high-human")
		}
		if input.Risk == contract.RiskLowV1 && input.EffectClass == contract.EffectObserveOnlyV1 && input.BypassAllowed {
			return bypass("review.route/standard-observe-not-required")
		}
		return auto("review.route/standard-auto")
	case contract.ProfilePermissiveV1:
		if input.Risk == contract.RiskLowV1 && input.BypassAllowed {
			return bypass("review.route/permissive-not-required")
		}
		return auto("review.route/permissive-auto")
	case contract.ProfileYOLOV1:
		if input.Risk != contract.RiskHighV1 && input.BypassAllowed {
			return bypass("review.route/yolo-not-required")
		}
		return auto("review.route/yolo-auto")
	case contract.ProfileBAPRV1:
		if input.Risk == contract.RiskLowV1 && input.BypassAllowed {
			return bypass("review.route/bapr-not-required")
		}
		return auto("review.route/bapr-auto")
	default:
		return contract.RouteDecisionV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonUnknownGovernanceCategory, "review profile is unsupported")
	}
}

func human(reason string) (contract.RouteDecisionV1, error) {
	return contract.SealRouteDecisionV1(contract.RouteDecisionV1{Kind: contract.RouteDecisionHumanV1, ReviewerRoute: contract.RouteHumanV1, ReasonCodes: []string{reason}})
}

func auto(reason string) (contract.RouteDecisionV1, error) {
	return contract.SealRouteDecisionV1(contract.RouteDecisionV1{Kind: contract.RouteDecisionAutoV1, ReviewerRoute: contract.RouteAutoV1, ReasonCodes: []string{reason}})
}

func bypass(reason string) (contract.RouteDecisionV1, error) {
	return contract.SealRouteDecisionV1(contract.RouteDecisionV1{Kind: contract.RouteDecisionBypassV1, OperationNotRequired: true, ReasonCodes: []string{reason}})
}
