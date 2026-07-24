package kernel

import (
	"context"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type OperationScopeEvidenceActionRouteBindingV3 struct {
	Route  ports.OperationScopeEvidenceActionApplicabilityRouteV3
	Reader ports.OperationScopeEvidenceApplicabilityCurrentReaderV3
}

// OperationScopeEvidenceActionRouterV3 is a closed Router, not a dynamic
// registry. Construction requires exactly one Reader for every frozen route.
type OperationScopeEvidenceActionRouterV3 struct {
	bindings map[ports.OperationScopeEvidenceApplicabilityDimensionV3]OperationScopeEvidenceActionRouteBindingV3
	Clock    func() time.Time
}

func NewOperationScopeEvidenceActionRouterV3(bindings []OperationScopeEvidenceActionRouteBindingV3, clock func() time.Time) (*OperationScopeEvidenceActionRouterV3, error) {
	if clock == nil {
		return nil, missingComponent("Action Evidence Router clock is required")
	}
	expected := ports.OperationScopeEvidenceActionRoutesV3()
	if len(bindings) != len(expected) {
		return nil, missingComponent("Action Evidence Router requires all five Owner Readers")
	}
	result := &OperationScopeEvidenceActionRouterV3{bindings: make(map[ports.OperationScopeEvidenceApplicabilityDimensionV3]OperationScopeEvidenceActionRouteBindingV3, len(bindings)), Clock: clock}
	for _, binding := range bindings {
		if err := binding.Route.Validate(); err != nil {
			return nil, err
		}
		if binding.Reader == nil {
			return nil, missingComponent("Action Evidence route Reader is required")
		}
		if _, exists := result.bindings[binding.Route.Dimension]; exists {
			return nil, core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "Action Evidence route is duplicated")
		}
		result.bindings[binding.Route.Dimension] = binding
	}
	for _, route := range expected {
		if _, ok := result.bindings[route.Dimension]; !ok {
			return nil, missingComponent("Action Evidence route is missing")
		}
	}
	return result, nil
}

func (r *OperationScopeEvidenceActionRouterV3) InspectOperationScopeEvidenceActionApplicabilityCurrentV3(ctx context.Context, dimension ports.OperationScopeEvidenceApplicabilityDimensionV3, fact ports.OperationScopeEvidenceApplicabilityFactRefV3, scopeDigest core.Digest) (ports.OperationScopeEvidenceApplicabilityCurrentProjectionV3, error) {
	if r == nil || r.Clock == nil || len(r.bindings) != len(ports.OperationScopeEvidenceActionRoutesV3()) {
		return ports.OperationScopeEvidenceApplicabilityCurrentProjectionV3{}, missingComponent("Action Evidence Router is incomplete")
	}
	if err := fact.Validate(); err != nil {
		return ports.OperationScopeEvidenceApplicabilityCurrentProjectionV3{}, err
	}
	if err := scopeDigest.Validate(); err != nil {
		return ports.OperationScopeEvidenceApplicabilityCurrentProjectionV3{}, err
	}
	binding, ok := r.bindings[dimension]
	if !ok || fact.Kind != binding.Route.Kind {
		return ports.OperationScopeEvidenceApplicabilityCurrentProjectionV3{}, core.NewError(core.ErrorForbidden, core.ReasonUnknownGovernanceCategory, "Action Evidence source Kind is not registered for the dimension")
	}
	now := r.Clock()
	if now.IsZero() {
		return ports.OperationScopeEvidenceApplicabilityCurrentProjectionV3{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Action Evidence Router clock returned zero")
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

type controlledOperationProviderStateV1 struct {
	mu       sync.Mutex
	attempts map[string]core.Digest
}

// ControlledOperationProviderSeamV1 is fixture-only. It proves the read order
// and one logical call per boundary key; it is not a production composition
// root, durability claim or physical exactly-once guarantee.
type ControlledOperationProviderSeamV1 struct {
	Enforcement ports.OperationProviderExecuteEnforcementCurrentReaderV1
	Handoff     ports.OperationProviderEvidenceHandoffCurrentReaderV1
	Boundary    ports.OperationProviderBoundaryCurrentReaderV1
	Provider    ports.OperationProviderTestInvokerV1
	Clock       func() time.Time
	state       *controlledOperationProviderStateV1
}

func NewControlledOperationProviderSeamV1(enforcement ports.OperationProviderExecuteEnforcementCurrentReaderV1, handoff ports.OperationProviderEvidenceHandoffCurrentReaderV1, boundary ports.OperationProviderBoundaryCurrentReaderV1, provider ports.OperationProviderTestInvokerV1, clock func() time.Time) (*ControlledOperationProviderSeamV1, error) {
	if enforcement == nil || handoff == nil || boundary == nil || provider == nil || clock == nil {
		return nil, missingComponent("controlled Provider seam requires all current Readers, Provider and clock")
	}
	return &ControlledOperationProviderSeamV1{Enforcement: enforcement, Handoff: handoff, Boundary: boundary, Provider: provider, Clock: clock, state: &controlledOperationProviderStateV1{attempts: map[string]core.Digest{}}}, nil
}

func (s *ControlledOperationProviderSeamV1) CallControlledOperationProviderV1(ctx context.Context, request ports.ControlledOperationProviderCallRequestV1) error {
	if s == nil || s.Enforcement == nil || s.Handoff == nil || s.Boundary == nil || s.Provider == nil || s.Clock == nil || s.state == nil {
		return missingComponent("controlled Provider seam is incomplete")
	}
	if err := request.Validate(); err != nil {
		return err
	}
	first := s.Clock()
	if first.IsZero() {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "controlled Provider seam clock returned zero")
	}
	enforcement, err := s.Enforcement.InspectCurrentOperationProviderExecuteEnforcementV1(ctx, request.Operation, request.ExecuteEnforcement)
	if err != nil {
		return err
	}
	if enforcement != request.ExecuteEnforcement || enforcement.Phase != ports.OperationDispatchEnforcementExecuteV4 || !first.Before(time.Unix(0, enforcement.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonDispatchPermitInvalid, "execute enforcement is not exact and current")
	}
	handoff, err := s.Handoff.InspectCurrentOperationProviderEvidenceHandoffV1(ctx, request.ExecuteEvidenceHandoff)
	if err != nil {
		return err
	}
	if err := handoff.Validate(); err != nil {
		return err
	}
	if handoff.RefV3() != request.ExecuteEvidenceHandoff || handoff.Phase != enforcement || !first.Before(time.Unix(0, handoff.NotAfterUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceUnavailable, "execute Evidence handoff is not exact and current")
	}
	boundary, err := s.Boundary.InspectCurrentOperationProviderBoundaryV1(ctx, request.Boundary)
	if err != nil {
		return err
	}
	if err := boundary.ValidateCurrent(request.Boundary, request.Operation, request.OperationScopeDigest, request.Attempt, enforcement, request.ExecuteEvidenceHandoff, first); err != nil {
		return err
	}
	second := s.Clock()
	if second.IsZero() || second.Before(first) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "controlled Provider seam clock regressed")
	}
	if !second.Before(time.Unix(0, boundary.ExpiresUnixNano)) || !second.Before(time.Unix(0, enforcement.ExpiresUnixNano)) || !second.Before(time.Unix(0, handoff.NotAfterUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonProviderBindingStale, "controlled Provider boundary expired before the call")
	}
	requestDigest, err := core.CanonicalJSONDigest("praxis.runtime.controlled-operation-provider-call", ports.OperationProviderBoundaryContractVersionV1, "ControlledOperationProviderCallRequestV1", request)
	if err != nil {
		return err
	}
	stateKey := string(request.Attempt.OperationDigest) + "\x00" + request.Boundary.ID
	s.state.mu.Lock()
	if previous, exists := s.state.attempts[stateKey]; exists {
		s.state.mu.Unlock()
		if previous != requestDigest {
			return core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "provider boundary id changed immutable call content")
		}
		return core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "provider boundary was already crossed; recovery must Inspect the original attempt")
	}
	s.state.attempts[stateKey] = requestDigest
	s.state.mu.Unlock()
	return s.Provider.InvokeOperationProviderTestV1(ctx, request)
}
