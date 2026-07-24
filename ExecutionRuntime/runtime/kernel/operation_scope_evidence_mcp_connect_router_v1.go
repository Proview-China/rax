package kernel

import (
	"context"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type OperationScopeEvidenceMCPConnectRouteBindingV1 struct {
	Route  ports.OperationScopeEvidenceMCPConnectApplicabilityRouteV1
	Reader ports.OperationScopeEvidenceApplicabilityCurrentReaderV3
}

type OperationScopeEvidenceMCPConnectRouterV1 struct {
	bindings map[ports.OperationScopeEvidenceApplicabilityDimensionV3]OperationScopeEvidenceMCPConnectRouteBindingV1
	Clock    func() time.Time
}

func NewOperationScopeEvidenceMCPConnectRouterV1(bindings []OperationScopeEvidenceMCPConnectRouteBindingV1, clock func() time.Time) (*OperationScopeEvidenceMCPConnectRouterV1, error) {
	if clock == nil {
		return nil, missingComponent("MCP Connect Evidence Router clock is required")
	}
	expected := ports.OperationScopeEvidenceMCPConnectRoutesV1()
	if len(bindings) != len(expected) {
		return nil, missingComponent("MCP Connect Evidence Router requires Run and Session Owner Readers")
	}
	result := &OperationScopeEvidenceMCPConnectRouterV1{bindings: make(map[ports.OperationScopeEvidenceApplicabilityDimensionV3]OperationScopeEvidenceMCPConnectRouteBindingV1, len(bindings)), Clock: clock}
	for _, binding := range bindings {
		if err := binding.Route.Validate(); err != nil {
			return nil, err
		}
		if nilPhysicalAuthorizationDependencyV3(binding.Reader) {
			return nil, missingComponent("MCP Connect Evidence route Reader is required")
		}
		if _, exists := result.bindings[binding.Route.Dimension]; exists {
			return nil, core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "MCP Connect Evidence route is duplicated")
		}
		result.bindings[binding.Route.Dimension] = binding
	}
	for _, route := range expected {
		if _, ok := result.bindings[route.Dimension]; !ok {
			return nil, missingComponent("MCP Connect Evidence route is missing")
		}
	}
	return result, nil
}

func (r *OperationScopeEvidenceMCPConnectRouterV1) InspectOperationScopeEvidenceMCPConnectApplicabilityCurrentV1(ctx context.Context, dimension ports.OperationScopeEvidenceApplicabilityDimensionV3, fact ports.OperationScopeEvidenceApplicabilityFactRefV3, scopeDigest core.Digest) (ports.OperationScopeEvidenceApplicabilityCurrentProjectionV3, error) {
	if r == nil || r.Clock == nil || len(r.bindings) != len(ports.OperationScopeEvidenceMCPConnectRoutesV1()) {
		return ports.OperationScopeEvidenceApplicabilityCurrentProjectionV3{}, missingComponent("MCP Connect Evidence Router is incomplete")
	}
	if err := fact.Validate(); err != nil {
		return ports.OperationScopeEvidenceApplicabilityCurrentProjectionV3{}, err
	}
	if err := scopeDigest.Validate(); err != nil {
		return ports.OperationScopeEvidenceApplicabilityCurrentProjectionV3{}, err
	}
	binding, ok := r.bindings[dimension]
	if !ok || fact.Kind != binding.Route.Kind {
		return ports.OperationScopeEvidenceApplicabilityCurrentProjectionV3{}, core.NewError(core.ErrorForbidden, core.ReasonUnknownGovernanceCategory, "MCP Connect Evidence source Kind is not registered for the dimension")
	}
	now := r.Clock()
	if now.IsZero() {
		return ports.OperationScopeEvidenceApplicabilityCurrentProjectionV3{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "MCP Connect Evidence Router clock returned zero")
	}
	projection, err := binding.Reader.InspectOperationScopeEvidenceApplicabilityCurrentV3(ctx, fact)
	if err != nil {
		return ports.OperationScopeEvidenceApplicabilityCurrentProjectionV3{}, err
	}
	if err := projection.Validate(fact, scopeDigest, now); err != nil {
		return ports.OperationScopeEvidenceApplicabilityCurrentProjectionV3{}, err
	}
	return projection, nil
}

var _ ports.OperationScopeEvidenceMCPConnectApplicabilityCurrentRouterV1 = (*OperationScopeEvidenceMCPConnectRouterV1)(nil)
