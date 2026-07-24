package contract

import (
	"sort"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type ProfileV1 string

const (
	ProfileRestrictedV1 ProfileV1 = "restricted"
	ProfileStandardV1   ProfileV1 = "standard"
	ProfilePermissiveV1 ProfileV1 = "permissive"
	ProfileYOLOV1       ProfileV1 = "yolo"
	ProfileBAPRV1       ProfileV1 = "bapr"
)

func ValidateProfileV1(profile ProfileV1) error {
	switch profile {
	case ProfileRestrictedV1, ProfileStandardV1, ProfilePermissiveV1, ProfileYOLOV1, ProfileBAPRV1:
		return nil
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonUnknownGovernanceCategory, "review profile is unsupported")
	}
}

type RiskLevelV1 string

const (
	RiskLowV1      RiskLevelV1 = "low"
	RiskMediumV1   RiskLevelV1 = "medium"
	RiskHighV1     RiskLevelV1 = "high"
	RiskCriticalV1 RiskLevelV1 = "critical"
)

type EffectClassV1 string

const (
	EffectObserveOnlyV1  EffectClassV1 = "observe_only"
	EffectReversibleV1   EffectClassV1 = "reversible"
	EffectPersistentV1   EffectClassV1 = "persistent"
	EffectIrreversibleV1 EffectClassV1 = "irreversible"
)

type EnvironmentV1 string

const (
	EnvironmentDevelopmentV1 EnvironmentV1 = "development"
	EnvironmentTestV1        EnvironmentV1 = "test"
	EnvironmentStagingV1     EnvironmentV1 = "staging"
	EnvironmentProductionV1  EnvironmentV1 = "production"
)

type RouteDecisionKindV1 string

const (
	RouteDecisionHumanV1  RouteDecisionKindV1 = "human"
	RouteDecisionAutoV1   RouteDecisionKindV1 = "auto"
	RouteDecisionBypassV1 RouteDecisionKindV1 = "bypass"
)

// RouteInputV1 contains only policy attributes. ToolCapability is provenance,
// not an allow-list key, and never participates in the routing matrix.
type RouteInputV1 struct {
	ToolCapability     string        `json:"tool_capability"`
	Profile            ProfileV1     `json:"profile"`
	Risk               RiskLevelV1   `json:"risk"`
	EffectClass        EffectClassV1 `json:"effect_class"`
	Environment        EnvironmentV1 `json:"environment"`
	HumanRequired      bool          `json:"human_required"`
	BypassAllowed      bool          `json:"bypass_allowed"`
	EvidenceSufficient bool          `json:"evidence_sufficient"`
	// ObserveOnlyDowngradeAllowed is a Policy-derived input. It is distinct
	// from BypassAllowed: it only says that missing grounding for a pure
	// observation need not force a Human route.
	ObserveOnlyDowngradeAllowed bool  `json:"observe_only_downgrade_allowed"`
	Current                     bool  `json:"current"`
	CheckedUnixNano             int64 `json:"checked_unix_nano"`
	ExpiresUnixNano             int64 `json:"expires_unix_nano"`
}

func (v RouteInputV1) ValidateCurrent(now time.Time) error {
	if invalidText(v.ToolCapability) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "review route input requires capability provenance")
	}
	if err := ValidateProfileV1(v.Profile); err != nil {
		return err
	}
	switch v.Risk {
	case RiskLowV1, RiskMediumV1, RiskHighV1, RiskCriticalV1:
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonUnknownGovernanceCategory, "review risk is unsupported")
	}
	switch v.EffectClass {
	case EffectObserveOnlyV1, EffectReversibleV1, EffectPersistentV1, EffectIrreversibleV1:
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonUnknownGovernanceCategory, "review effect class is unsupported")
	}
	switch v.Environment {
	case EnvironmentDevelopmentV1, EnvironmentTestV1, EnvironmentStagingV1, EnvironmentProductionV1:
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonUnknownGovernanceCategory, "review environment is unsupported")
	}
	if !v.Current || v.CheckedUnixNano <= 0 || v.CheckedUnixNano >= v.ExpiresUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "review route policy is not current")
	}
	if now.IsZero() || now.UnixNano() < v.CheckedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "review route clock regressed")
	}
	if now.UnixNano() >= v.ExpiresUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "review route policy expired")
	}
	return nil
}

type RouteDecisionV1 struct {
	Kind                 RouteDecisionKindV1 `json:"kind"`
	ReviewerRoute        RouteV1             `json:"reviewer_route,omitempty"`
	OperationNotRequired bool                `json:"operation_not_required"`
	ReasonCodes          []string            `json:"reason_codes"`
	Digest               core.Digest         `json:"digest"`
}

func (v RouteDecisionV1) digestValue() RouteDecisionV1 { v.Digest = ""; return v }

func (v RouteDecisionV1) validateShape() error {
	if len(v.ReasonCodes) == 0 || len(v.ReasonCodes) > MaxListItemsV1 || !sort.StringsAreSorted(v.ReasonCodes) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "review route reasons must be sorted and bounded")
	}
	for i, reason := range v.ReasonCodes {
		if invalidText(reason) || (i > 0 && v.ReasonCodes[i-1] == reason) {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "review route reasons are invalid or duplicated")
		}
	}
	switch v.Kind {
	case RouteDecisionHumanV1:
		if v.ReviewerRoute != RouteHumanV1 || v.OperationNotRequired {
			return core.NewError(core.ErrorConflict, core.ReasonInvalidState, "human route decision is inconsistent")
		}
	case RouteDecisionAutoV1:
		if v.ReviewerRoute != RouteAutoV1 || v.OperationNotRequired {
			return core.NewError(core.ErrorConflict, core.ReasonInvalidState, "auto route decision is inconsistent")
		}
	case RouteDecisionBypassV1:
		if v.ReviewerRoute != "" || !v.OperationNotRequired {
			return core.NewError(core.ErrorConflict, core.ReasonInvalidState, "bypass must be an explicit not-required decision")
		}
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonUnknownGovernanceCategory, "review route decision is unsupported")
	}
	return nil
}

func SealRouteDecisionV1(v RouteDecisionV1) (RouteDecisionV1, error) {
	v.Digest = ""
	sort.Strings(v.ReasonCodes)
	if err := v.validateShape(); err != nil {
		return RouteDecisionV1{}, err
	}
	digest, err := seal("RouteDecisionV1", v.digestValue())
	if err != nil {
		return RouteDecisionV1{}, err
	}
	v.Digest = digest
	return v, v.Validate()
}

func (v RouteDecisionV1) Validate() error {
	if err := v.validateShape(); err != nil {
		return err
	}
	return validateSealed("RouteDecisionV1", v.digestValue(), v.Digest)
}
