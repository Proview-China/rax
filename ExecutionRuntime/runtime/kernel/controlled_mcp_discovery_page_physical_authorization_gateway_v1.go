package kernel

import (
	"context"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// ControlledMCPDiscoveryPagePhysicalAuthorizationGatewayV1 re-reads the exact
// Runtime closure for one Run-scoped MCP Discovery Page. It never contacts a Provider
// and never turns an Evidence Observation into a domain fact.
type ControlledMCPDiscoveryPagePhysicalAuthorizationGatewayV1 struct {
	routes       ports.ControlledMCPDiscoveryPageRouteCurrentReaderV1
	effects      ports.ControlledOperationEffectCurrentReaderV2
	governance   ports.OperationGovernanceCurrentReaderV3
	dispatch     ports.OperationDispatchCurrentReaderV3
	sandbox      ports.OperationDispatchSandboxCurrentReaderV4
	enforcement  ports.OperationProviderExecuteEnforcementCurrentReaderV1
	consumptions ports.OperationScopeEvidenceConsumptionClosureReaderV1
	handoffs     ports.OperationScopeEvidenceProviderHandoffClosureReaderV1
	associations ports.PreparedDomainCommandAssociationCurrentReaderV1
	availability ports.MCPConnectionAvailabilityNeutralCurrentReaderV1
	clock        func() time.Time
}

func NewControlledMCPDiscoveryPagePhysicalAuthorizationGatewayV1(
	routes ports.ControlledMCPDiscoveryPageRouteCurrentReaderV1,
	effects ports.ControlledOperationEffectCurrentReaderV2,
	governance ports.OperationGovernanceCurrentReaderV3,
	dispatch ports.OperationDispatchCurrentReaderV3,
	sandbox ports.OperationDispatchSandboxCurrentReaderV4,
	enforcement ports.OperationProviderExecuteEnforcementCurrentReaderV1,
	consumptions ports.OperationScopeEvidenceConsumptionClosureReaderV1,
	handoffs ports.OperationScopeEvidenceProviderHandoffClosureReaderV1,
	associations ports.PreparedDomainCommandAssociationCurrentReaderV1,
	availability ports.MCPConnectionAvailabilityNeutralCurrentReaderV1,
	clock func() time.Time,
) (*ControlledMCPDiscoveryPagePhysicalAuthorizationGatewayV1, error) {
	gateway := &ControlledMCPDiscoveryPagePhysicalAuthorizationGatewayV1{
		routes: routes, effects: effects, governance: governance, dispatch: dispatch, sandbox: sandbox,
		enforcement: enforcement, consumptions: consumptions, handoffs: handoffs,
		associations: associations, availability: availability, clock: clock,
	}
	if err := gateway.validateDependencies(); err != nil {
		return nil, err
	}
	return gateway, nil
}

func (g *ControlledMCPDiscoveryPagePhysicalAuthorizationGatewayV1) AuthorizeControlledMCPDiscoveryPagePhysicalV1(ctx context.Context, request ports.ControlledMCPDiscoveryPagePhysicalAuthorizationRequestV1) (ports.ControlledMCPDiscoveryPagePhysicalAuthorizationV1, error) {
	if err := validatePhysicalAuthorizationContextV3(ctx); err != nil {
		return ports.ControlledMCPDiscoveryPagePhysicalAuthorizationV1{}, err
	}
	if err := g.validateDependencies(); err != nil {
		return ports.ControlledMCPDiscoveryPagePhysicalAuthorizationV1{}, err
	}
	if err := request.Validate(); err != nil {
		return ports.ControlledMCPDiscoveryPagePhysicalAuthorizationV1{}, err
	}

	nowS1 := g.clock()
	if nowS1.IsZero() {
		return ports.ControlledMCPDiscoveryPagePhysicalAuthorizationV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "controlled MCP Discovery Page S1 clock is zero")
	}
	s1, err := g.readCurrentClosure(ctx, request, nowS1)
	if err != nil {
		return ports.ControlledMCPDiscoveryPagePhysicalAuthorizationV1{}, err
	}

	nowS2 := g.clock()
	if nowS2.IsZero() || nowS2.Before(nowS1) {
		return ports.ControlledMCPDiscoveryPagePhysicalAuthorizationV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "controlled MCP Discovery Page clock regressed before S2")
	}
	s2, err := g.readCurrentClosure(ctx, request, nowS2)
	if err != nil {
		return ports.ControlledMCPDiscoveryPagePhysicalAuthorizationV1{}, err
	}
	if !sameControlledMCPDiscoveryPageClosureV1(s1, s2) {
		return ports.ControlledMCPDiscoveryPagePhysicalAuthorizationV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "controlled MCP Discovery Page current closure drifted between S1 and S2")
	}

	notAfter := minControlledProviderTimeV2(s2.notAfter, request.CallerDeadlineUnixNano)
	if !nowS2.Before(time.Unix(0, notAfter)) {
		return ports.ControlledMCPDiscoveryPagePhysicalAuthorizationV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "controlled MCP Discovery Page current closure expired before issuance")
	}
	return ports.SealControlledMCPDiscoveryPagePhysicalAuthorizationV1(ports.ControlledMCPDiscoveryPagePhysicalAuthorizationV1{
		UnifiedNotAfterUnixNano: notAfter,
		Route:                   request.Route,
		ProviderTransport:       s2.route.ProviderTransport,
		Provider:                s2.route.Provider,
		Operation:               request.Execute.Intent.Operation,
		OperationDigest:         request.Attempt.OperationDigest,
		OperationScopeDigest:    request.Execute.Intent.Operation.ExecutionScopeDigest,
		EffectID:                request.Execute.Intent.ID,
		EffectRevision:          request.Execute.Intent.Revision,
		EffectFactRevision:      s2.effect.FactRevision,
		IntentDigest:            request.Attempt.IntentDigest,
		Prepared:                request.Execute.Prepared,
		Attempt:                 request.Attempt,
		ExecuteEnforcement:      request.ExecuteEnforcement,
		PrepareConsumption:      request.PrepareConsumption,
		ExecuteHandoff:          request.ExecuteHandoff,
		SandboxProjectionDigest: s2.sandbox.ProjectionDigest,
		CredentialFactsDigest:   s2.credentialDigest,
		Association:             request.Association,
		DomainCommand:           request.DomainCommand,
		ConnectionAvailability:  request.ConnectionAvailability,
		Namespace:               request.Namespace,
		CursorDigest:            request.CursorDigest,
		PageOrdinal:             request.PageOrdinal,
		IssuedUnixNano:          nowS2.UnixNano(),
	})
}

type controlledMCPDiscoveryPageCurrentClosureV1 struct {
	route                ports.ControlledMCPDiscoveryPageRouteCurrentProjectionV1
	effect               ports.ControlledOperationEffectCurrentProjectionV2
	governanceDigest     core.Digest
	credentialDigest     core.Digest
	dispatch             ports.OperationDispatchCurrentProjectionV3
	sandbox              ports.OperationDispatchSandboxCurrentProjectionV4
	enforcement          ports.OperationDispatchEnforcementPhaseRefV4
	prepareConsumption   ports.OperationScopeEvidenceConsumptionFactV3
	prepareQualification ports.OperationScopeEvidenceQualificationFactV3
	prepareHandoff       ports.OperationScopeEvidenceProviderHandoffFactV3
	executeQualification ports.OperationScopeEvidenceQualificationFactV3
	executeHandoff       ports.OperationScopeEvidenceProviderHandoffFactV3
	association          ports.PreparedDomainCommandAssociationCurrentProjectionV1
	availability         ports.MCPConnectionAvailabilityNeutralProjectionV1
	notAfter             int64
}

func (g *ControlledMCPDiscoveryPagePhysicalAuthorizationGatewayV1) readCurrentClosure(ctx context.Context, request ports.ControlledMCPDiscoveryPagePhysicalAuthorizationRequestV1, now time.Time) (controlledMCPDiscoveryPageCurrentClosureV1, error) {
	var empty controlledMCPDiscoveryPageCurrentClosureV1
	route, err := g.routes.InspectCurrentControlledMCPDiscoveryPageRouteV1(ctx, request.Route)
	if err != nil {
		return empty, err
	}
	if err := route.ValidateCurrent(request.Route, now); err != nil {
		return empty, err
	}
	if route.Provider != request.Execute.Prepared.Provider || route.Provider != request.Execute.Intent.Provider {
		return empty, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "controlled MCP Discovery Page route binds another Provider")
	}
	effect, err := g.effects.InspectCurrentControlledOperationEffectV2(ctx, request.Execute.Intent.Operation, request.Execute.Intent.ID)
	if err != nil {
		return empty, err
	}
	if err := effect.Validate(now); err != nil {
		return empty, err
	}
	if !ports.SameOperationSubjectV3(effect.Intent.Operation, request.Execute.Intent.Operation) || effect.IntentDigest != request.Attempt.IntentDigest || effect.Intent.ID != request.Execute.Intent.ID || effect.Intent.Revision != request.Execute.Intent.Revision || effect.Intent.Kind != request.Execute.Intent.Kind || effect.Intent.Provider != request.Execute.Intent.Provider || effect.State != string(control.OperationEffectDispatchIntentV3) {
		return empty, core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectStateConflict, "controlled MCP Discovery Page Effect current drifted")
	}

	governance, err := g.governance.InspectOperationGovernance(ctx, request.Execute.Intent.Operation)
	if err != nil {
		return empty, err
	}
	if err := governance.ValidateCurrent(request.Execute.Intent, now); err != nil {
		return empty, err
	}
	governanceDigest, err := governance.DigestV3(now)
	if err != nil {
		return empty, err
	}
	credentialDigest, err := ports.DigestOperationCredentialFactsV3(governance.Credentials, now)
	if err != nil {
		return empty, err
	}

	dispatch, err := g.dispatch.InspectOperationDispatch(ctx, request.Execute.Intent.Operation, request.Execute.Permit.ID, request.Execute.Delegation.ID)
	if err != nil {
		return empty, err
	}
	if err := dispatch.ValidateForExecute(request.Execute, governance, now); err != nil {
		return empty, err
	}

	sandbox, err := g.sandbox.InspectOperationDispatchSandboxCurrentV4(ctx, request.Execute.Intent.Operation, request.Execute.Intent.ID, request.ExecuteEnforcement.SandboxAttempt)
	if err != nil {
		return empty, err
	}
	if err := sandbox.ValidateCurrent(request.Execute.Intent.Operation, request.Execute.Intent.ID, request.Execute.Intent.Revision, request.Attempt.IntentDigest, request.Attempt.AttemptID, route.Provider, now); err != nil {
		return empty, err
	}
	enforcement, err := g.enforcement.InspectCurrentOperationProviderExecuteEnforcementV1(ctx, request.Execute.Intent.Operation, request.ExecuteEnforcement)
	if err != nil {
		return empty, err
	}
	if enforcement != request.ExecuteEnforcement || !now.Before(time.Unix(0, enforcement.ExpiresUnixNano)) {
		return empty, core.NewError(core.ErrorPreconditionFailed, core.ReasonDispatchPermitInvalid, "controlled MCP Discovery Page execute Enforcement is stale or drifted")
	}

	prepareConsumption, prepareQualification, prepareHandoff, err := g.consumptions.InspectOperationScopeEvidenceConsumptionClosureV1(ctx, request.PrepareConsumption)
	if err != nil {
		return empty, err
	}
	if err := validateControlledMCPDiscoveryPagePrepareEvidenceV1(request, route, effect.FactRevision, prepareConsumption, prepareQualification, prepareHandoff, now); err != nil {
		return empty, err
	}
	executeHandoff, executeQualification, err := g.handoffs.InspectOperationScopeEvidenceProviderHandoffClosureV1(ctx, request.ExecuteHandoff)
	if err != nil {
		return empty, err
	}
	if err := validateControlledMCPDiscoveryPageExecuteEvidenceV1(request, route, effect.FactRevision, executeHandoff, executeQualification, now); err != nil {
		return empty, err
	}
	if prepareQualification.ID == executeQualification.ID || prepareHandoff.ID == executeHandoff.ID {
		return empty, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "controlled MCP Discovery Page prepare and execute Evidence were reused")
	}

	association, err := g.associations.InspectCurrentPreparedDomainCommandAssociationV1(ctx, request.Association)
	if err != nil {
		return empty, err
	}
	if err := association.ValidateCurrent(request.Association, now); err != nil {
		return empty, err
	}
	if err := validateControlledMCPDiscoveryPageAssociationV1(request, route, association); err != nil {
		return empty, err
	}

	availability, err := g.availability.InspectCurrentMCPConnectionAvailabilityNeutralV1(ctx, request.ConnectionAvailability)
	if err != nil {
		return empty, err
	}
	if err := availability.ValidateCurrent(request.ConnectionAvailability, now); err != nil {
		return empty, err
	}
	if availability.ProviderTransport != route.ProviderTransport || availability.Provider != route.Provider || availability.TenantID != request.Execute.Intent.Operation.ExecutionScope.Identity.TenantID || availability.RunID != string(request.Execute.Intent.Operation.RunID) {
		return empty, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "controlled MCP Discovery Page Connection availability drifted")
	}

	notAfter := minControlledProviderTimeV2(
		route.ExpiresUnixNano, effect.ExpiresUnixNano, governance.ExpiresUnixNano, dispatch.ExpiresUnixNano,
		sandbox.ExpiresUnixNano, enforcement.ExpiresUnixNano,
		request.Execute.Intent.ExpiresUnixNano, request.Execute.Permit.ExpiresUnixNano,
		request.Execute.Fence.ExpiresAt.UnixNano(), request.Execute.Prepared.ExpiresUnixNano,
		prepareQualification.ExpiresUnixNano, prepareHandoff.NotAfterUnixNano,
		executeQualification.ExpiresUnixNano, executeHandoff.NotAfterUnixNano,
		association.ExpiresUnixNano, availability.ExpiresUnixNano,
	)
	for _, credential := range governance.Credentials {
		notAfter = minControlledProviderTimeV2(notAfter, credential.ExpiresUnixNano)
	}
	if !now.Before(time.Unix(0, notAfter)) {
		return empty, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "controlled MCP Discovery Page current closure expired")
	}
	return controlledMCPDiscoveryPageCurrentClosureV1{
		route: route, effect: effect, governanceDigest: governanceDigest, credentialDigest: credentialDigest,
		dispatch: dispatch, sandbox: sandbox, enforcement: enforcement,
		prepareConsumption: prepareConsumption, prepareQualification: prepareQualification,
		prepareHandoff: prepareHandoff, executeQualification: executeQualification,
		executeHandoff: executeHandoff, association: association, availability: availability, notAfter: notAfter,
	}, nil
}

func validateControlledMCPDiscoveryPagePrepareEvidenceV1(request ports.ControlledMCPDiscoveryPagePhysicalAuthorizationRequestV1, route ports.ControlledMCPDiscoveryPageRouteCurrentProjectionV1, effectFactRevision core.Revision, consumption ports.OperationScopeEvidenceConsumptionFactV3, qualification ports.OperationScopeEvidenceQualificationFactV3, handoff ports.OperationScopeEvidenceProviderHandoffFactV3, now time.Time) error {
	if consumption.Validate() != nil || qualification.Validate() != nil || handoff.Validate() != nil || consumption.RefV3() != request.PrepareConsumption || consumption.Handoff != handoff.RefV3() || consumption.Qualification != handoff.Qualification || qualification.ID != consumption.Qualification.ID || qualification.Revision != consumption.Qualification.Revision+1 || qualification.Consumption == nil || *qualification.Consumption != request.PrepareConsumption {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "controlled MCP Discovery Page prepare Evidence closure drifted")
	}
	if qualification.State != ports.OperationScopeEvidenceConsumedCurrentV3 && qualification.State != ports.OperationScopeEvidenceConsumedObservationV3 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceUnavailable, "controlled MCP Discovery Page prepare Evidence is not consumed")
	}
	return validateControlledMCPDiscoveryPageEvidenceScopeV1(request, route, effectFactRevision, qualification, handoff, ports.OperationDispatchEnforcementPrepareV4, now)
}

func validateControlledMCPDiscoveryPageExecuteEvidenceV1(request ports.ControlledMCPDiscoveryPagePhysicalAuthorizationRequestV1, route ports.ControlledMCPDiscoveryPageRouteCurrentProjectionV1, effectFactRevision core.Revision, handoff ports.OperationScopeEvidenceProviderHandoffFactV3, qualification ports.OperationScopeEvidenceQualificationFactV3, now time.Time) error {
	if handoff.Validate() != nil || qualification.Validate() != nil || handoff.RefV3() != request.ExecuteHandoff || handoff.Qualification != qualification.RefV3() || qualification.State != ports.OperationScopeEvidenceIssuedV3 || qualification.Consumption != nil || handoff.Phase != request.ExecuteEnforcement {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "controlled MCP Discovery Page execute Evidence closure drifted or was pre-consumed")
	}
	return validateControlledMCPDiscoveryPageEvidenceScopeV1(request, route, effectFactRevision, qualification, handoff, ports.OperationDispatchEnforcementExecuteV4, now)
}

func validateControlledMCPDiscoveryPageEvidenceScopeV1(request ports.ControlledMCPDiscoveryPagePhysicalAuthorizationRequestV1, route ports.ControlledMCPDiscoveryPageRouteCurrentProjectionV1, effectFactRevision core.Revision, qualification ports.OperationScopeEvidenceQualificationFactV3, handoff ports.OperationScopeEvidenceProviderHandoffFactV3, phase ports.OperationDispatchEnforcementPhaseV4, now time.Time) error {
	scope := qualification.Scope
	if !ports.SameOperationSubjectV3(scope.Operation, request.Execute.Intent.Operation) || scope.OperationDigest != request.Attempt.OperationDigest || scope.EffectID != request.Attempt.EffectID || scope.EffectRevision != effectFactRevision || scope.EffectDigest != request.Attempt.IntentDigest || scope.EffectKind != ports.OperationScopeEvidenceMCPDiscoveryPageEffectKindV1 || scope.AttemptID != request.Attempt.AttemptID || scope.Phase != phase || scope.Generation != route.Assembly || handoff.Phase.Phase != phase || handoff.Phase.OperationDigest != request.Attempt.OperationDigest || handoff.Phase.EffectID != request.Attempt.EffectID || handoff.Phase.AttemptID != request.Attempt.AttemptID {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceScopeConflict, "controlled MCP Discovery Page Evidence binds another scope or phase")
	}
	if err := ports.ValidateOperationScopeEvidenceMCPDiscoveryPageApplicabilityV1(scope.Applicability); err != nil {
		return err
	}
	if err := qualification.Runtime.Validate(now); err != nil {
		return err
	}
	if !now.Before(time.Unix(0, qualification.ExpiresUnixNano)) || !now.Before(time.Unix(0, handoff.NotAfterUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceUnavailable, "controlled MCP Discovery Page Evidence is expired")
	}
	return nil
}

func validateControlledMCPDiscoveryPageAssociationV1(request ports.ControlledMCPDiscoveryPagePhysicalAuthorizationRequestV1, route ports.ControlledMCPDiscoveryPageRouteCurrentProjectionV1, association ports.PreparedDomainCommandAssociationCurrentProjectionV1) error {
	if association.Ref != request.Association || association.DomainCommand != request.DomainCommand || !ports.SameOperationSubjectV3(association.Operation, request.Execute.Intent.Operation) || association.OperationDigest != request.Attempt.OperationDigest || association.EffectID != request.Attempt.EffectID || association.EffectRevision != request.Attempt.IntentRevision || association.IntentDigest != request.Attempt.IntentDigest || association.Prepared != request.Execute.Prepared || association.Attempt != request.Attempt || association.Provider != route.Provider || association.PayloadSchema != request.Execute.Prepared.PayloadSchema || association.PayloadDigest != request.Execute.Prepared.PayloadDigest || association.PayloadRevision != request.Execute.Prepared.PayloadRevision {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "controlled MCP Discovery Page domain command association drifted")
	}
	if request.DomainCommand.Owner.Role != ports.OwnerSettlement || request.DomainCommand.Owner.ComponentID != route.Provider.ComponentID || request.DomainCommand.Owner.ManifestDigest != route.Provider.ManifestDigest {
		return core.NewError(core.ErrorConflict, core.ReasonSettlementOwnerMismatch, "controlled MCP Discovery Page command Owner differs from the route Provider")
	}
	return nil
}

func sameControlledMCPDiscoveryPageClosureV1(left, right controlledMCPDiscoveryPageCurrentClosureV1) bool {
	return left.route.Ref == right.route.Ref && left.route.ProjectionDigest == right.route.ProjectionDigest &&
		left.effect.Digest == right.effect.Digest && left.effect.FactRevision == right.effect.FactRevision &&
		left.governanceDigest == right.governanceDigest && left.credentialDigest == right.credentialDigest &&
		left.dispatch.PermitDigest == right.dispatch.PermitDigest && left.dispatch.PermitFactRevision == right.dispatch.PermitFactRevision && left.dispatch.Delegation == right.dispatch.Delegation && left.dispatch.PreparedAttemptID == right.dispatch.PreparedAttemptID && left.dispatch.PreparationDigest == right.dispatch.PreparationDigest &&
		left.sandbox.ProjectionDigest == right.sandbox.ProjectionDigest && left.enforcement == right.enforcement &&
		left.prepareConsumption.Digest == right.prepareConsumption.Digest && left.prepareQualification.Digest == right.prepareQualification.Digest && left.prepareHandoff.Digest == right.prepareHandoff.Digest &&
		left.executeQualification.Digest == right.executeQualification.Digest && left.executeHandoff.Digest == right.executeHandoff.Digest &&
		left.association.Ref == right.association.Ref && left.association.ProjectionDigest == right.association.ProjectionDigest &&
		left.availability.Ref == right.availability.Ref && left.availability.ProjectionDigest == right.availability.ProjectionDigest
}

func (g *ControlledMCPDiscoveryPagePhysicalAuthorizationGatewayV1) validateDependencies() error {
	if g == nil || nilPhysicalAuthorizationDependencyV3(g.routes) || nilPhysicalAuthorizationDependencyV3(g.effects) || nilPhysicalAuthorizationDependencyV3(g.governance) || nilPhysicalAuthorizationDependencyV3(g.dispatch) || nilPhysicalAuthorizationDependencyV3(g.sandbox) || nilPhysicalAuthorizationDependencyV3(g.enforcement) || nilPhysicalAuthorizationDependencyV3(g.consumptions) || nilPhysicalAuthorizationDependencyV3(g.handoffs) || nilPhysicalAuthorizationDependencyV3(g.associations) || nilPhysicalAuthorizationDependencyV3(g.availability) || g.clock == nil {
		return core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "controlled MCP Discovery Page physical authorization readers are unavailable")
	}
	return nil
}

var _ ports.ControlledMCPDiscoveryPagePhysicalAuthorizationPortV1 = (*ControlledMCPDiscoveryPagePhysicalAuthorizationGatewayV1)(nil)
