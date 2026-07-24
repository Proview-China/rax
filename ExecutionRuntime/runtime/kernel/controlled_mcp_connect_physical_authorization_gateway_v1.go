package kernel

import (
	"context"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// ControlledMCPConnectPhysicalAuthorizationGatewayV1 re-reads the exact
// Runtime closure for one Run-scoped MCP Connect. It never contacts a Provider
// and never turns an Evidence Observation into a domain fact.
type ControlledMCPConnectPhysicalAuthorizationGatewayV1 struct {
	routes       ports.ControlledMCPConnectRouteCurrentReaderV1
	effects      ports.ControlledOperationEffectCurrentReaderV2
	governance   ports.OperationGovernanceCurrentReaderV3
	dispatch     ports.OperationDispatchCurrentReaderV3
	sandbox      ports.OperationDispatchSandboxCurrentReaderV4
	enforcement  ports.OperationProviderExecuteEnforcementCurrentReaderV1
	consumptions ports.OperationScopeEvidenceConsumptionClosureReaderV1
	handoffs     ports.OperationScopeEvidenceProviderHandoffClosureReaderV1
	associations ports.PreparedDomainCommandAssociationCurrentReaderV1
	clock        func() time.Time
}

func NewControlledMCPConnectPhysicalAuthorizationGatewayV1(
	routes ports.ControlledMCPConnectRouteCurrentReaderV1,
	effects ports.ControlledOperationEffectCurrentReaderV2,
	governance ports.OperationGovernanceCurrentReaderV3,
	dispatch ports.OperationDispatchCurrentReaderV3,
	sandbox ports.OperationDispatchSandboxCurrentReaderV4,
	enforcement ports.OperationProviderExecuteEnforcementCurrentReaderV1,
	consumptions ports.OperationScopeEvidenceConsumptionClosureReaderV1,
	handoffs ports.OperationScopeEvidenceProviderHandoffClosureReaderV1,
	associations ports.PreparedDomainCommandAssociationCurrentReaderV1,
	clock func() time.Time,
) (*ControlledMCPConnectPhysicalAuthorizationGatewayV1, error) {
	gateway := &ControlledMCPConnectPhysicalAuthorizationGatewayV1{
		routes: routes, effects: effects, governance: governance, dispatch: dispatch, sandbox: sandbox,
		enforcement: enforcement, consumptions: consumptions, handoffs: handoffs,
		associations: associations, clock: clock,
	}
	if err := gateway.validateDependencies(); err != nil {
		return nil, err
	}
	return gateway, nil
}

func (g *ControlledMCPConnectPhysicalAuthorizationGatewayV1) AuthorizeControlledMCPConnectPhysicalV1(ctx context.Context, request ports.ControlledMCPConnectPhysicalAuthorizationRequestV1) (ports.ControlledMCPConnectPhysicalAuthorizationV1, error) {
	if err := validatePhysicalAuthorizationContextV3(ctx); err != nil {
		return ports.ControlledMCPConnectPhysicalAuthorizationV1{}, err
	}
	if err := g.validateDependencies(); err != nil {
		return ports.ControlledMCPConnectPhysicalAuthorizationV1{}, err
	}
	if err := request.Validate(); err != nil {
		return ports.ControlledMCPConnectPhysicalAuthorizationV1{}, err
	}

	nowS1 := g.clock()
	if nowS1.IsZero() {
		return ports.ControlledMCPConnectPhysicalAuthorizationV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "controlled MCP Connect S1 clock is zero")
	}
	s1, err := g.readCurrentClosure(ctx, request, nowS1)
	if err != nil {
		return ports.ControlledMCPConnectPhysicalAuthorizationV1{}, err
	}

	nowS2 := g.clock()
	if nowS2.IsZero() || nowS2.Before(nowS1) {
		return ports.ControlledMCPConnectPhysicalAuthorizationV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "controlled MCP Connect clock regressed before S2")
	}
	s2, err := g.readCurrentClosure(ctx, request, nowS2)
	if err != nil {
		return ports.ControlledMCPConnectPhysicalAuthorizationV1{}, err
	}
	if !sameControlledMCPConnectClosureV1(s1, s2) {
		return ports.ControlledMCPConnectPhysicalAuthorizationV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "controlled MCP Connect current closure drifted between S1 and S2")
	}

	notAfter := minControlledProviderTimeV2(s2.notAfter, request.CallerDeadlineUnixNano)
	if !nowS2.Before(time.Unix(0, notAfter)) {
		return ports.ControlledMCPConnectPhysicalAuthorizationV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "controlled MCP Connect current closure expired before issuance")
	}
	return ports.SealControlledMCPConnectPhysicalAuthorizationV1(ports.ControlledMCPConnectPhysicalAuthorizationV1{
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
		IssuedUnixNano:          nowS2.UnixNano(),
	})
}

type controlledMCPConnectCurrentClosureV1 struct {
	route                ports.ControlledMCPConnectRouteCurrentProjectionV1
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
	notAfter             int64
}

func (g *ControlledMCPConnectPhysicalAuthorizationGatewayV1) readCurrentClosure(ctx context.Context, request ports.ControlledMCPConnectPhysicalAuthorizationRequestV1, now time.Time) (controlledMCPConnectCurrentClosureV1, error) {
	var empty controlledMCPConnectCurrentClosureV1
	route, err := g.routes.InspectCurrentControlledMCPConnectRouteV1(ctx, request.Route)
	if err != nil {
		return empty, err
	}
	if err := route.ValidateCurrent(request.Route, now); err != nil {
		return empty, err
	}
	if route.Provider != request.Execute.Prepared.Provider || route.Provider != request.Execute.Intent.Provider {
		return empty, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "controlled MCP Connect route binds another Provider")
	}
	effect, err := g.effects.InspectCurrentControlledOperationEffectV2(ctx, request.Execute.Intent.Operation, request.Execute.Intent.ID)
	if err != nil {
		return empty, err
	}
	if err := effect.Validate(now); err != nil {
		return empty, err
	}
	if !ports.SameOperationSubjectV3(effect.Intent.Operation, request.Execute.Intent.Operation) || effect.IntentDigest != request.Attempt.IntentDigest || effect.Intent.ID != request.Execute.Intent.ID || effect.Intent.Revision != request.Execute.Intent.Revision || effect.Intent.Kind != request.Execute.Intent.Kind || effect.Intent.Provider != request.Execute.Intent.Provider || effect.State != string(control.OperationEffectDispatchIntentV3) {
		return empty, core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectStateConflict, "controlled MCP Connect Effect current drifted")
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
		return empty, core.NewError(core.ErrorPreconditionFailed, core.ReasonDispatchPermitInvalid, "controlled MCP Connect execute Enforcement is stale or drifted")
	}

	prepareConsumption, prepareQualification, prepareHandoff, err := g.consumptions.InspectOperationScopeEvidenceConsumptionClosureV1(ctx, request.PrepareConsumption)
	if err != nil {
		return empty, err
	}
	if err := validateControlledMCPConnectPrepareEvidenceV1(request, route, effect.FactRevision, prepareConsumption, prepareQualification, prepareHandoff, now); err != nil {
		return empty, err
	}
	executeHandoff, executeQualification, err := g.handoffs.InspectOperationScopeEvidenceProviderHandoffClosureV1(ctx, request.ExecuteHandoff)
	if err != nil {
		return empty, err
	}
	if err := validateControlledMCPConnectExecuteEvidenceV1(request, route, effect.FactRevision, executeHandoff, executeQualification, now); err != nil {
		return empty, err
	}
	if prepareQualification.ID == executeQualification.ID || prepareHandoff.ID == executeHandoff.ID {
		return empty, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "controlled MCP Connect prepare and execute Evidence were reused")
	}

	association, err := g.associations.InspectCurrentPreparedDomainCommandAssociationV1(ctx, request.Association)
	if err != nil {
		return empty, err
	}
	if err := association.ValidateCurrent(request.Association, now); err != nil {
		return empty, err
	}
	if err := validateControlledMCPConnectAssociationV1(request, route, association); err != nil {
		return empty, err
	}

	notAfter := minControlledProviderTimeV2(
		route.ExpiresUnixNano, effect.ExpiresUnixNano, governance.ExpiresUnixNano, dispatch.ExpiresUnixNano,
		sandbox.ExpiresUnixNano, enforcement.ExpiresUnixNano,
		request.Execute.Intent.ExpiresUnixNano, request.Execute.Permit.ExpiresUnixNano,
		request.Execute.Fence.ExpiresAt.UnixNano(), request.Execute.Prepared.ExpiresUnixNano,
		prepareQualification.ExpiresUnixNano, prepareHandoff.NotAfterUnixNano,
		executeQualification.ExpiresUnixNano, executeHandoff.NotAfterUnixNano,
		association.ExpiresUnixNano,
	)
	for _, credential := range governance.Credentials {
		notAfter = minControlledProviderTimeV2(notAfter, credential.ExpiresUnixNano)
	}
	if !now.Before(time.Unix(0, notAfter)) {
		return empty, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "controlled MCP Connect current closure expired")
	}
	return controlledMCPConnectCurrentClosureV1{
		route: route, effect: effect, governanceDigest: governanceDigest, credentialDigest: credentialDigest,
		dispatch: dispatch, sandbox: sandbox, enforcement: enforcement,
		prepareConsumption: prepareConsumption, prepareQualification: prepareQualification,
		prepareHandoff: prepareHandoff, executeQualification: executeQualification,
		executeHandoff: executeHandoff, association: association, notAfter: notAfter,
	}, nil
}

func validateControlledMCPConnectPrepareEvidenceV1(request ports.ControlledMCPConnectPhysicalAuthorizationRequestV1, route ports.ControlledMCPConnectRouteCurrentProjectionV1, effectFactRevision core.Revision, consumption ports.OperationScopeEvidenceConsumptionFactV3, qualification ports.OperationScopeEvidenceQualificationFactV3, handoff ports.OperationScopeEvidenceProviderHandoffFactV3, now time.Time) error {
	if consumption.Validate() != nil || qualification.Validate() != nil || handoff.Validate() != nil || consumption.RefV3() != request.PrepareConsumption || consumption.Handoff != handoff.RefV3() || consumption.Qualification != handoff.Qualification || qualification.ID != consumption.Qualification.ID || qualification.Revision != consumption.Qualification.Revision+1 || qualification.Consumption == nil || *qualification.Consumption != request.PrepareConsumption {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "controlled MCP Connect prepare Evidence closure drifted")
	}
	if qualification.State != ports.OperationScopeEvidenceConsumedCurrentV3 && qualification.State != ports.OperationScopeEvidenceConsumedObservationV3 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceUnavailable, "controlled MCP Connect prepare Evidence is not consumed")
	}
	return validateControlledMCPConnectEvidenceScopeV1(request, route, effectFactRevision, qualification, handoff, ports.OperationDispatchEnforcementPrepareV4, now)
}

func validateControlledMCPConnectExecuteEvidenceV1(request ports.ControlledMCPConnectPhysicalAuthorizationRequestV1, route ports.ControlledMCPConnectRouteCurrentProjectionV1, effectFactRevision core.Revision, handoff ports.OperationScopeEvidenceProviderHandoffFactV3, qualification ports.OperationScopeEvidenceQualificationFactV3, now time.Time) error {
	if handoff.Validate() != nil || qualification.Validate() != nil || handoff.RefV3() != request.ExecuteHandoff || handoff.Qualification != qualification.RefV3() || qualification.State != ports.OperationScopeEvidenceIssuedV3 || qualification.Consumption != nil || handoff.Phase != request.ExecuteEnforcement {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "controlled MCP Connect execute Evidence closure drifted or was pre-consumed")
	}
	return validateControlledMCPConnectEvidenceScopeV1(request, route, effectFactRevision, qualification, handoff, ports.OperationDispatchEnforcementExecuteV4, now)
}

func validateControlledMCPConnectEvidenceScopeV1(request ports.ControlledMCPConnectPhysicalAuthorizationRequestV1, route ports.ControlledMCPConnectRouteCurrentProjectionV1, effectFactRevision core.Revision, qualification ports.OperationScopeEvidenceQualificationFactV3, handoff ports.OperationScopeEvidenceProviderHandoffFactV3, phase ports.OperationDispatchEnforcementPhaseV4, now time.Time) error {
	scope := qualification.Scope
	if !ports.SameOperationSubjectV3(scope.Operation, request.Execute.Intent.Operation) || scope.OperationDigest != request.Attempt.OperationDigest || scope.EffectID != request.Attempt.EffectID || scope.EffectRevision != effectFactRevision || scope.EffectDigest != request.Attempt.IntentDigest || scope.EffectKind != ports.OperationScopeEvidenceMCPConnectEffectKindV1 || scope.AttemptID != request.Attempt.AttemptID || scope.Phase != phase || scope.Generation != route.Assembly || handoff.Phase.Phase != phase || handoff.Phase.OperationDigest != request.Attempt.OperationDigest || handoff.Phase.EffectID != request.Attempt.EffectID || handoff.Phase.AttemptID != request.Attempt.AttemptID {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceScopeConflict, "controlled MCP Connect Evidence binds another scope or phase")
	}
	if err := ports.ValidateOperationScopeEvidenceMCPConnectApplicabilityV1(scope.Applicability); err != nil {
		return err
	}
	if err := qualification.Runtime.Validate(now); err != nil {
		return err
	}
	if !now.Before(time.Unix(0, qualification.ExpiresUnixNano)) || !now.Before(time.Unix(0, handoff.NotAfterUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceUnavailable, "controlled MCP Connect Evidence is expired")
	}
	return nil
}

func validateControlledMCPConnectAssociationV1(request ports.ControlledMCPConnectPhysicalAuthorizationRequestV1, route ports.ControlledMCPConnectRouteCurrentProjectionV1, association ports.PreparedDomainCommandAssociationCurrentProjectionV1) error {
	if association.Ref != request.Association || association.DomainCommand != request.DomainCommand || !ports.SameOperationSubjectV3(association.Operation, request.Execute.Intent.Operation) || association.OperationDigest != request.Attempt.OperationDigest || association.EffectID != request.Attempt.EffectID || association.EffectRevision != request.Attempt.IntentRevision || association.IntentDigest != request.Attempt.IntentDigest || association.Prepared != request.Execute.Prepared || association.Attempt != request.Attempt || association.Provider != route.Provider || association.PayloadSchema != request.Execute.Prepared.PayloadSchema || association.PayloadDigest != request.Execute.Prepared.PayloadDigest || association.PayloadRevision != request.Execute.Prepared.PayloadRevision {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "controlled MCP Connect domain command association drifted")
	}
	if request.DomainCommand.Owner.Role != ports.OwnerSettlement || request.DomainCommand.Owner.ComponentID != route.Provider.ComponentID || request.DomainCommand.Owner.ManifestDigest != route.Provider.ManifestDigest {
		return core.NewError(core.ErrorConflict, core.ReasonSettlementOwnerMismatch, "controlled MCP Connect command Owner differs from the route Provider")
	}
	return nil
}

func sameControlledMCPConnectClosureV1(left, right controlledMCPConnectCurrentClosureV1) bool {
	return left.route.Ref == right.route.Ref && left.route.ProjectionDigest == right.route.ProjectionDigest &&
		left.effect.Digest == right.effect.Digest && left.effect.FactRevision == right.effect.FactRevision &&
		left.governanceDigest == right.governanceDigest && left.credentialDigest == right.credentialDigest &&
		left.dispatch.PermitDigest == right.dispatch.PermitDigest && left.dispatch.PermitFactRevision == right.dispatch.PermitFactRevision && left.dispatch.Delegation == right.dispatch.Delegation && left.dispatch.PreparedAttemptID == right.dispatch.PreparedAttemptID && left.dispatch.PreparationDigest == right.dispatch.PreparationDigest &&
		left.sandbox.ProjectionDigest == right.sandbox.ProjectionDigest && left.enforcement == right.enforcement &&
		left.prepareConsumption.Digest == right.prepareConsumption.Digest && left.prepareQualification.Digest == right.prepareQualification.Digest && left.prepareHandoff.Digest == right.prepareHandoff.Digest &&
		left.executeQualification.Digest == right.executeQualification.Digest && left.executeHandoff.Digest == right.executeHandoff.Digest &&
		left.association.Ref == right.association.Ref && left.association.ProjectionDigest == right.association.ProjectionDigest
}

func (g *ControlledMCPConnectPhysicalAuthorizationGatewayV1) validateDependencies() error {
	if g == nil || nilPhysicalAuthorizationDependencyV3(g.routes) || nilPhysicalAuthorizationDependencyV3(g.effects) || nilPhysicalAuthorizationDependencyV3(g.governance) || nilPhysicalAuthorizationDependencyV3(g.dispatch) || nilPhysicalAuthorizationDependencyV3(g.sandbox) || nilPhysicalAuthorizationDependencyV3(g.enforcement) || nilPhysicalAuthorizationDependencyV3(g.consumptions) || nilPhysicalAuthorizationDependencyV3(g.handoffs) || nilPhysicalAuthorizationDependencyV3(g.associations) || g.clock == nil {
		return core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "controlled MCP Connect physical authorization readers are unavailable")
	}
	return nil
}

var _ ports.ControlledMCPConnectPhysicalAuthorizationPortV1 = (*ControlledMCPConnectPhysicalAuthorizationGatewayV1)(nil)
