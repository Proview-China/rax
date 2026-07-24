package ports

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const (
	OperationScopeEvidenceMCPConnectEffectKindV1    EffectKindV2     = "praxis.mcp/connect"
	OperationScopeEvidenceMCPConnectPolicyProfileV1 NamespacedNameV2 = "praxis.mcp/run-connection-v1"
)

// OperationScopeEvidenceMCPConnectMatrixV1 is the Run/Session-only row for a
// governed MCP connection. It is deliberately distinct from the five-reader
// single Action row.
func OperationScopeEvidenceMCPConnectMatrixV1() OperationScopeEvidenceApplicabilityMatrixKeyV3 {
	return OperationScopeEvidenceApplicabilityMatrixKeyV3{
		OperationKind: OperationScopeRunV3,
		EffectKind:    OperationScopeEvidenceMCPConnectEffectKindV1,
		PolicyProfile: OperationScopeEvidenceMCPConnectPolicyProfileV1,
	}
}

func IsOperationScopeEvidenceMCPConnectMatrixKeyV1(key OperationScopeEvidenceApplicabilityMatrixKeyV3) bool {
	return key == OperationScopeEvidenceMCPConnectMatrixV1()
}

type OperationScopeEvidenceMCPConnectApplicabilityRouteV1 struct {
	Dimension            OperationScopeEvidenceApplicabilityDimensionV3 `json:"dimension"`
	Kind                 NamespacedNameV2                               `json:"kind"`
	OwnerContractVersion string                                         `json:"owner_contract_version"`
}

func (r OperationScopeEvidenceMCPConnectApplicabilityRouteV1) Validate() error {
	expected, ok := operationScopeEvidenceMCPConnectRouteV1(r.Dimension)
	if !ok || r != expected {
		return core.NewError(core.ErrorForbidden, core.ReasonUnknownGovernanceCategory, "MCP Connect Evidence applicability route is not registered")
	}
	version, err := core.ParseSemanticVersion(r.OwnerContractVersion)
	if err != nil || version.String() != r.OwnerContractVersion {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidSemanticVersion, "MCP Connect Evidence owner contract version is not canonical SemVer")
	}
	return nil
}

func OperationScopeEvidenceMCPConnectRoutesV1() []OperationScopeEvidenceMCPConnectApplicabilityRouteV1 {
	return []OperationScopeEvidenceMCPConnectApplicabilityRouteV1{
		{Dimension: OperationScopeEvidenceRunV3, Kind: OperationScopeEvidenceRunCurrentKindV3, OwnerContractVersion: OperationScopeEvidenceRunOwnerVersionV3},
		{Dimension: OperationScopeEvidenceSessionV3, Kind: OperationScopeEvidenceSessionCurrentKindV3, OwnerContractVersion: OperationScopeEvidenceSessionOwnerVersionV3},
	}
}

func operationScopeEvidenceMCPConnectRouteV1(dimension OperationScopeEvidenceApplicabilityDimensionV3) (OperationScopeEvidenceMCPConnectApplicabilityRouteV1, bool) {
	for _, route := range OperationScopeEvidenceMCPConnectRoutesV1() {
		if route.Dimension == dimension {
			return route, true
		}
	}
	return OperationScopeEvidenceMCPConnectApplicabilityRouteV1{}, false
}

func ValidateOperationScopeEvidenceMCPConnectApplicabilityV1(values []OperationScopeEvidenceApplicabilityV3) error {
	if err := ValidateOperationScopeEvidenceApplicabilitySetV3(values); err != nil {
		return err
	}
	for _, value := range NormalizeOperationScopeEvidenceApplicabilityV3(values) {
		route, required := operationScopeEvidenceMCPConnectRouteV1(value.Dimension)
		if required {
			if value.Mode != OperationScopeEvidenceRequiredV3 || value.Fact == nil || value.Fact.Kind != route.Kind {
				return core.NewError(core.ErrorForbidden, core.ReasonUnknownGovernanceCategory, "MCP Connect Evidence requires exact Run and Session Owner sources")
			}
			continue
		}
		if value.Mode != OperationScopeEvidenceForbiddenV3 || value.Fact != nil {
			return core.NewError(core.ErrorForbidden, core.ReasonUnknownGovernanceCategory, "MCP Connect Evidence forbids Turn, Action and Context sources")
		}
	}
	return nil
}

type OperationScopeEvidenceMCPConnectApplicabilityCurrentRouterV1 interface {
	InspectOperationScopeEvidenceMCPConnectApplicabilityCurrentV1(context.Context, OperationScopeEvidenceApplicabilityDimensionV3, OperationScopeEvidenceApplicabilityFactRefV3, core.Digest) (OperationScopeEvidenceApplicabilityCurrentProjectionV3, error)
}
