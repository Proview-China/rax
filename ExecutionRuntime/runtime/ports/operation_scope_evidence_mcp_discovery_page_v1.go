package ports

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const (
	OperationScopeEvidenceMCPDiscoveryPageEffectKindV1    EffectKindV2     = "praxis.mcp/discover"
	OperationScopeEvidenceMCPDiscoveryPagePolicyProfileV1 NamespacedNameV2 = "praxis.mcp/discovery-page-v1"
)

// OperationScopeEvidenceMCPDiscoveryPageMatrixV1 is the Run/Session-only row for a
// governed MCP connection. It is deliberately distinct from the five-reader
// single Action row.
func OperationScopeEvidenceMCPDiscoveryPageMatrixV1() OperationScopeEvidenceApplicabilityMatrixKeyV3 {
	return OperationScopeEvidenceApplicabilityMatrixKeyV3{
		OperationKind: OperationScopeRunV3,
		EffectKind:    OperationScopeEvidenceMCPDiscoveryPageEffectKindV1,
		PolicyProfile: OperationScopeEvidenceMCPDiscoveryPagePolicyProfileV1,
	}
}

func IsOperationScopeEvidenceMCPDiscoveryPageMatrixKeyV1(key OperationScopeEvidenceApplicabilityMatrixKeyV3) bool {
	return key == OperationScopeEvidenceMCPDiscoveryPageMatrixV1()
}

type OperationScopeEvidenceMCPDiscoveryPageApplicabilityRouteV1 struct {
	Dimension            OperationScopeEvidenceApplicabilityDimensionV3 `json:"dimension"`
	Kind                 NamespacedNameV2                               `json:"kind"`
	OwnerContractVersion string                                         `json:"owner_contract_version"`
}

func (r OperationScopeEvidenceMCPDiscoveryPageApplicabilityRouteV1) Validate() error {
	expected, ok := operationScopeEvidenceMCPDiscoveryPageRouteV1(r.Dimension)
	if !ok || r != expected {
		return core.NewError(core.ErrorForbidden, core.ReasonUnknownGovernanceCategory, "MCP Discovery Page Evidence applicability route is not registered")
	}
	version, err := core.ParseSemanticVersion(r.OwnerContractVersion)
	if err != nil || version.String() != r.OwnerContractVersion {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidSemanticVersion, "MCP Discovery Page Evidence owner contract version is not canonical SemVer")
	}
	return nil
}

func OperationScopeEvidenceMCPDiscoveryPageRoutesV1() []OperationScopeEvidenceMCPDiscoveryPageApplicabilityRouteV1 {
	return []OperationScopeEvidenceMCPDiscoveryPageApplicabilityRouteV1{
		{Dimension: OperationScopeEvidenceRunV3, Kind: OperationScopeEvidenceRunCurrentKindV3, OwnerContractVersion: OperationScopeEvidenceRunOwnerVersionV3},
		{Dimension: OperationScopeEvidenceSessionV3, Kind: OperationScopeEvidenceSessionCurrentKindV3, OwnerContractVersion: OperationScopeEvidenceSessionOwnerVersionV3},
	}
}

func operationScopeEvidenceMCPDiscoveryPageRouteV1(dimension OperationScopeEvidenceApplicabilityDimensionV3) (OperationScopeEvidenceMCPDiscoveryPageApplicabilityRouteV1, bool) {
	for _, route := range OperationScopeEvidenceMCPDiscoveryPageRoutesV1() {
		if route.Dimension == dimension {
			return route, true
		}
	}
	return OperationScopeEvidenceMCPDiscoveryPageApplicabilityRouteV1{}, false
}

func ValidateOperationScopeEvidenceMCPDiscoveryPageApplicabilityV1(values []OperationScopeEvidenceApplicabilityV3) error {
	if err := ValidateOperationScopeEvidenceApplicabilitySetV3(values); err != nil {
		return err
	}
	for _, value := range NormalizeOperationScopeEvidenceApplicabilityV3(values) {
		route, required := operationScopeEvidenceMCPDiscoveryPageRouteV1(value.Dimension)
		if required {
			if value.Mode != OperationScopeEvidenceRequiredV3 || value.Fact == nil || value.Fact.Kind != route.Kind {
				return core.NewError(core.ErrorForbidden, core.ReasonUnknownGovernanceCategory, "MCP Discovery Page Evidence requires exact Run and Session Owner sources")
			}
			continue
		}
		if value.Mode != OperationScopeEvidenceForbiddenV3 || value.Fact != nil {
			return core.NewError(core.ErrorForbidden, core.ReasonUnknownGovernanceCategory, "MCP Discovery Page Evidence forbids Turn, Action and Context sources")
		}
	}
	return nil
}

type OperationScopeEvidenceMCPDiscoveryPageApplicabilityCurrentRouterV1 interface {
	InspectOperationScopeEvidenceMCPDiscoveryPageApplicabilityCurrentV1(context.Context, OperationScopeEvidenceApplicabilityDimensionV3, OperationScopeEvidenceApplicabilityFactRefV3, core.Digest) (OperationScopeEvidenceApplicabilityCurrentProjectionV3, error)
}
