package kernel

import (
	"context"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// ControlledOperationProviderTransportV2 is held only by the kernel Runner.
// Its implementation must atomically check NotAfter and create-once the
// irreversible admission under StableKeyDigest. It does not return an
// Observation, DomainResult, Settlement or Outcome.
type ControlledOperationProviderTransportV2 interface {
	AdmitControlledOperationProviderV2(context.Context, ControlledOperationProviderTransportRequestV2) (ControlledOperationProviderTransportResultV2, error)
}

type ControlledOperationProviderTransportRequestV2 struct {
	StableKeyDigest   core.Digest                         `json:"stable_key_digest"`
	NotAfterUnixNano  int64                               `json:"not_after_unix_nano"`
	ProviderTransport ports.ProviderBindingRefV2          `json:"provider_transport_binding"`
	Provider          ports.ProviderBindingRefV2          `json:"provider_binding"`
	Operation         ports.OperationSubjectV3            `json:"operation"`
	EffectKind        ports.EffectKindV2                  `json:"effect_kind"`
	Prepared          ports.PreparedProviderAttemptRefV2  `json:"prepared"`
	Attempt           ports.OperationDispatchAttemptRefV3 `json:"attempt"`
}

type ControlledOperationProviderTransportResultV2 struct {
	AdmissionReceipt *ports.ControlledOperationProviderAdmissionReceiptRefV2
	Unknown          bool
}

type controlledOperationProviderAuthorizationV2 struct {
	entry control.ControlledOperationProviderEntryFactV2
}

type controlledOperationProviderRunnerV2 struct {
	transport ControlledOperationProviderTransportV2
	clock     func() time.Time
}

type controlledOperationProviderRouteInputsCurrentReaderV2 interface {
	InspectCurrentControlledOperationProviderRouteInputsV2(context.Context, ports.ControlledOperationProviderRouteCurrentProjectionV2) (ports.ControlledOperationProviderRouteCurrentProjectionV2, error)
}

func (r controlledOperationProviderRunnerV2) run(ctx context.Context, authorization controlledOperationProviderAuthorizationV2) (ControlledOperationProviderTransportResultV2, error) {
	if r.transport == nil || r.clock == nil {
		return ControlledOperationProviderTransportResultV2{}, core.NewError(core.ErrorCapabilityUnavailable, core.ReasonComponentMissing, "controlled Provider Runner dependencies are missing")
	}
	now := r.clock()
	if now.IsZero() || !now.Before(time.Unix(0, authorization.entry.UnifiedNotAfterUnixNano)) {
		return ControlledOperationProviderTransportResultV2{Unknown: true}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "controlled Provider authorization expired at the actual point")
	}
	request := authorization.entry.Request
	return r.transport.AdmitControlledOperationProviderV2(ctx, ControlledOperationProviderTransportRequestV2{
		StableKeyDigest:   authorization.entry.StableKeyDigest,
		NotAfterUnixNano:  authorization.entry.UnifiedNotAfterUnixNano,
		ProviderTransport: authorization.entry.FreshRoute.ProviderTransportBinding,
		Provider:          request.ProviderBinding,
		Operation:         request.Operation,
		EffectKind:        request.EffectKind,
		Prepared:          request.Prepared,
		Attempt:           request.Attempt,
	})
}

type ControlledOperationProviderGatewayV2 struct {
	Entries         control.ControlledOperationProviderEntryFactPortV2
	Routes          ports.ControlledOperationProviderRouteCurrentReaderV2
	RouteInputs     controlledOperationProviderRouteInputsCurrentReaderV2
	Bindings        ports.ProviderBindingCurrentnessPortV2
	Effects         ports.ControlledOperationEffectCurrentReaderV2
	Prepared        ports.ControlledOperationPreparedCurrentReaderV2
	Policies        ports.ControlledOperationEvidencePolicyCurrentReaderV2
	Enforcement     ports.OperationProviderExecuteEnforcementCurrentReaderV1
	Handoff         ports.OperationProviderEvidenceHandoffCurrentReaderV1
	Evidence        ports.OperationScopeEvidenceGovernancePortV3
	Boundary        ports.OperationProviderBoundaryCurrentReaderV1
	ProviderInspect ports.ControlledProviderInspectPortV2
	clock           func() time.Time
	runner          controlledOperationProviderRunnerV2
}

func NewControlledOperationProviderGatewayV2(
	entries control.ControlledOperationProviderEntryFactPortV2,
	routes ports.ControlledOperationProviderRouteCurrentReaderV2,
	routeInputs controlledOperationProviderRouteInputsCurrentReaderV2,
	bindings ports.ProviderBindingCurrentnessPortV2,
	effects ports.ControlledOperationEffectCurrentReaderV2,
	prepared ports.ControlledOperationPreparedCurrentReaderV2,
	policies ports.ControlledOperationEvidencePolicyCurrentReaderV2,
	enforcement ports.OperationProviderExecuteEnforcementCurrentReaderV1,
	handoff ports.OperationProviderEvidenceHandoffCurrentReaderV1,
	evidence ports.OperationScopeEvidenceGovernancePortV3,
	boundary ports.OperationProviderBoundaryCurrentReaderV1,
	providerInspect ports.ControlledProviderInspectPortV2,
	transport ControlledOperationProviderTransportV2,
	clock func() time.Time,
) ControlledOperationProviderGatewayV2 {
	return ControlledOperationProviderGatewayV2{
		Entries: entries, Routes: routes, RouteInputs: routeInputs, Bindings: bindings,
		Effects: effects, Prepared: prepared, Policies: policies, Enforcement: enforcement,
		Handoff: handoff, Evidence: evidence, Boundary: boundary, ProviderInspect: providerInspect,
		clock: clock, runner: controlledOperationProviderRunnerV2{transport: transport, clock: clock},
	}
}

func (g ControlledOperationProviderGatewayV2) validateDependencies() error {
	if g.Entries == nil || g.Routes == nil || g.RouteInputs == nil || g.Bindings == nil || g.Effects == nil || g.Prepared == nil || g.Policies == nil || g.Enforcement == nil || g.Handoff == nil || g.Evidence == nil || g.Boundary == nil || g.ProviderInspect == nil || g.clock == nil || g.runner.transport == nil {
		return core.NewError(core.ErrorCapabilityUnavailable, core.ReasonComponentMissing, "controlled Provider Gateway dependencies are missing")
	}
	return nil
}

func (g ControlledOperationProviderGatewayV2) EnterControlledOperationProviderV2(ctx context.Context, request ports.ControlledOperationProviderRequestV2) (ports.ControlledOperationProviderResultV2, error) {
	if err := request.Validate(); err != nil {
		return ports.ControlledOperationProviderResultV2{}, err
	}
	if err := g.validateDependencies(); err != nil {
		return ports.ControlledOperationProviderResultV2{}, err
	}
	stable, entryID, err := control.DeriveControlledOperationProviderEntryIdentityV2(request)
	if err != nil {
		return ports.ControlledOperationProviderResultV2{}, err
	}
	existing, err := g.Entries.InspectControlledOperationProviderEntryV2(ctx, request.Operation, entryID)
	if err == nil {
		if existing.StableKeyDigest != stable || existing.Request.RequestDigest != request.RequestDigest {
			return ports.ControlledOperationProviderResultV2{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "controlled Provider Entry exists with different content")
		}
		return g.inspectEntry(ctx, existing)
	}
	if !core.HasCategory(err, core.ErrorNotFound) {
		return ports.ControlledOperationProviderResultV2{}, err
	}

	firstNow := g.clock()
	if firstNow.IsZero() {
		return ports.ControlledOperationProviderResultV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "controlled Provider current read clock is zero")
	}
	closure, err := g.readCurrentClosure(ctx, request, firstNow)
	if err != nil {
		return ports.ControlledOperationProviderResultV2{}, err
	}
	commitNow := g.clock()
	if commitNow.IsZero() || commitNow.Before(firstNow) {
		return ports.ControlledOperationProviderResultV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "controlled Provider clock regressed before Entry create")
	}
	if !commitNow.Before(time.Unix(0, closure.notAfter)) {
		return ports.ControlledOperationProviderResultV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "controlled Provider current closure expired before Entry create")
	}
	fact, err := control.SealControlledOperationProviderEntryFactV2(control.ControlledOperationProviderEntryFactV2{
		EntryID: entryID, Revision: 1, StableKeyDigest: stable, State: control.ControlledOperationProviderEntryEnteredV2,
		Request: request, UnifiedNotAfterUnixNano: closure.notAfter, FreshEffect: closure.effect, FreshRoute: closure.route,
		FreshBindings: closure.bindings, FreshPrepared: closure.prepared, FreshEvidencePolicy: closure.evidencePolicy,
		FreshApplicabilityPolicy: closure.applicabilityPolicy, FreshBoundary: closure.boundary,
		FreshExecuteEnforcement: closure.enforcement, FreshExecuteHandoff: closure.handoff,
		FreshQualification: closure.qualification, EnteredUnixNano: commitNow.UnixNano(), UpdatedUnixNano: commitNow.UnixNano(),
	})
	if err != nil {
		return ports.ControlledOperationProviderResultV2{}, err
	}
	created, err := g.Entries.CreateControlledOperationProviderEntryV2(ctx, fact)
	if err != nil {
		if !core.HasCategory(err, core.ErrorUnavailable) && !core.HasCategory(err, core.ErrorIndeterminate) && !core.HasCategory(err, core.ErrorConflict) {
			return ports.ControlledOperationProviderResultV2{}, err
		}
		stored, inspectErr := g.Entries.InspectControlledOperationProviderEntryV2(context.WithoutCancel(ctx), request.Operation, entryID)
		if inspectErr != nil {
			return ports.ControlledOperationProviderResultV2{}, err
		}
		if !control.SameControlledOperationProviderEntryImmutableV2(stored, fact) {
			return ports.ControlledOperationProviderResultV2{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "controlled Provider Entry recovery found different content")
		}
		// The create reply was lost: authority to run cannot be reconstructed.
		return g.inspectEntry(context.WithoutCancel(ctx), stored)
	}
	if created.Disposition == control.ControlledOperationProviderEntryExistingV2 || !created.HasOpaqueClaimV2() {
		return g.inspectEntry(ctx, created.Fact)
	}
	raw, runErr := g.runner.run(ctx, controlledOperationProviderAuthorizationV2{entry: created.Fact})
	if raw.AdmissionReceipt != nil {
		if err := raw.AdmissionReceipt.Validate(); err != nil || raw.AdmissionReceipt.StableKeyDigest != created.Fact.StableKeyDigest {
			raw = ControlledOperationProviderTransportResultV2{Unknown: true}
			runErr = core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "Provider admission receipt is invalid")
		}
	}
	if raw.AdmissionReceipt != nil && raw.AdmissionReceipt.NoEffect && runErr == nil {
		return g.transitionAndInspect(ctx, created.Fact, control.ControlledOperationProviderEntryRejectedNoEffectV2, raw.AdmissionReceipt, nil)
	}
	// Admitted, unavailable, deadline crossing, or malformed response all mean
	// possibly executed. Persist unknown, then use only the original Inspect key.
	unknown, transitionErr := g.transitionAndInspect(ctx, created.Fact, control.ControlledOperationProviderEntryUnknownV2, raw.AdmissionReceipt, nil)
	if transitionErr != nil {
		return ports.ControlledOperationProviderResultV2{}, transitionErr
	}
	if runErr != nil || raw.Unknown || raw.AdmissionReceipt != nil && raw.AdmissionReceipt.Admitted {
		key, keyErr := ports.DeriveControlledOperationProviderEntryKeyV2(request)
		if keyErr != nil {
			return ports.ControlledOperationProviderResultV2{}, keyErr
		}
		return g.InspectControlledOperationProviderV2(context.WithoutCancel(ctx), ports.ControlledOperationProviderInspectRequestV2{Operation: request.Operation, Key: key})
	}
	return unknown, nil
}

type controlledOperationProviderCurrentClosureV2 struct {
	effect              ports.ControlledOperationEffectCurrentProjectionV2
	route               ports.ControlledOperationProviderRouteCurrentProjectionV2
	bindings            []ports.ProviderBindingCurrentProjectionV2
	prepared            ports.ControlledOperationPreparedCurrentProjectionV2
	evidencePolicy      ports.OperationScopeEvidencePolicyFactV3
	applicabilityPolicy ports.OperationScopeEvidenceApplicabilityPolicyFactV3
	boundary            ports.OperationProviderBoundaryCurrentProjectionV1
	enforcement         ports.OperationDispatchEnforcementPhaseRefV4
	handoff             ports.OperationScopeEvidenceProviderHandoffFactV3
	qualification       ports.OperationScopeEvidenceQualificationFactV3
	notAfter            int64
}

func (g ControlledOperationProviderGatewayV2) readCurrentClosure(ctx context.Context, request ports.ControlledOperationProviderRequestV2, now time.Time) (controlledOperationProviderCurrentClosureV2, error) {
	matrix := ports.OperationScopeEvidenceActionMatrixV3()
	route, err := g.Routes.InspectCurrentControlledOperationProviderRouteV2(ctx, request.RouteCurrentRef, matrix)
	if err != nil {
		return controlledOperationProviderCurrentClosureV2{}, err
	}
	if err := route.ValidateCurrent(request.RouteCurrentRef, matrix, now); err != nil || route.DeclarationRef != request.RouteDeclarationRef || route.ConformanceRef != request.RouteConformanceRef || route.ToolAdapterBinding != request.ToolAdapterBinding || route.ProviderBinding != request.ProviderBinding {
		if err != nil {
			return controlledOperationProviderCurrentClosureV2{}, err
		}
		return controlledOperationProviderCurrentClosureV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "controlled Provider route does not bind the request")
	}
	reread, err := g.RouteInputs.InspectCurrentControlledOperationProviderRouteInputsV2(ctx, route)
	if err != nil {
		return controlledOperationProviderCurrentClosureV2{}, err
	}
	if reread.ProjectionDigest != route.ProjectionDigest || reread.Ref != route.Ref {
		return controlledOperationProviderCurrentClosureV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "controlled Provider route inputs drifted")
	}

	bindingRefs := []ports.ProviderBindingRefV2{route.ToolAdapterBinding, route.GatewayBinding, route.ProviderTransportBinding, route.PreparedReaderBinding, route.BoundaryReaderBinding, route.ProviderInspectBinding, route.ProviderBinding}
	bindings := make([]ports.ProviderBindingCurrentProjectionV2, 0, len(bindingRefs))
	notAfter := route.ExpiresUnixNano
	for _, ref := range bindingRefs {
		current, inspectErr := g.Bindings.InspectProviderBindingCurrentV2(ctx, ref)
		if inspectErr != nil {
			return controlledOperationProviderCurrentClosureV2{}, inspectErr
		}
		if err := current.ValidateCurrent(ref, now); err != nil {
			return controlledOperationProviderCurrentClosureV2{}, err
		}
		if current.BindingSetDigest != route.BindingSetDigest || current.BindingSetSemanticDigest != route.BindingSetSemanticDigest {
			return controlledOperationProviderCurrentClosureV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "controlled Provider Binding current set content drifted from the route")
		}
		bindings = append(bindings, current)
		notAfter = minControlledProviderTimeV2(notAfter, current.ExpiresUnixNano)
	}

	effect, err := g.Effects.InspectCurrentControlledOperationEffectV2(ctx, request.Operation, request.EffectID)
	if err != nil {
		return controlledOperationProviderCurrentClosureV2{}, err
	}
	if err := effect.Validate(now); err != nil || effect.FactRevision != request.EffectRevision || effect.IntentDigest != request.IntentDigest || effect.Intent.Kind != request.EffectKind || effect.Intent.Provider != request.ProviderBinding || effect.State != string(control.OperationEffectDispatchIntentV3) {
		if err != nil {
			return controlledOperationProviderCurrentClosureV2{}, err
		}
		return controlledOperationProviderCurrentClosureV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectStateConflict, "controlled Provider Effect is not at exact dispatch intent")
	}
	notAfter = minControlledProviderTimeV2(notAfter, effect.ExpiresUnixNano)

	prepared, err := g.Prepared.InspectCurrentControlledOperationPreparedV2(ctx, request.Prepared)
	if err != nil {
		return controlledOperationProviderCurrentClosureV2{}, err
	}
	if err := prepared.Validate(); err != nil || prepared.Snapshot.SemanticDigest != request.PreparedSemantics.SemanticDigest || prepared.Snapshot.Prepared != request.Prepared {
		if err != nil {
			return controlledOperationProviderCurrentClosureV2{}, err
		}
		return controlledOperationProviderCurrentClosureV2{}, core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "controlled Provider Prepared current drifted")
	}
	if now.Before(time.Unix(0, prepared.CheckedUnixNano)) || !now.Before(time.Unix(0, prepared.ExpiresUnixNano)) {
		return controlledOperationProviderCurrentClosureV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "controlled Provider Prepared current expired")
	}
	notAfter = minControlledProviderTimeV2(notAfter, prepared.ExpiresUnixNano)

	evidencePolicy, err := g.Policies.InspectCurrentControlledOperationEvidencePolicyV2(ctx, request.EvidencePolicy)
	if err != nil {
		return controlledOperationProviderCurrentClosureV2{}, err
	}
	applicabilityPolicy, err := g.Policies.InspectCurrentControlledOperationApplicabilityPolicyV2(ctx, request.ApplicabilityPolicy)
	if err != nil {
		return controlledOperationProviderCurrentClosureV2{}, err
	}
	if evidencePolicy.Validate() != nil || applicabilityPolicy.Validate() != nil || evidencePolicy.RefV3() != request.EvidencePolicy || applicabilityPolicy.RefV3() != request.ApplicabilityPolicy || evidencePolicy.State != ports.OperationScopeEvidencePolicyActiveV3 || applicabilityPolicy.State != ports.OperationScopeEvidencePolicyActiveV3 || evidencePolicy.OperationKind != ports.OperationScopeRunV3 || applicabilityPolicy.OperationKind != ports.OperationScopeRunV3 || evidencePolicy.EffectKind != request.EffectKind || applicabilityPolicy.EffectKind != request.EffectKind || applicabilityPolicy.Profile != ports.OperationScopeEvidenceActionPolicyProfileV3 || !now.Before(time.Unix(0, evidencePolicy.ExpiresUnixNano)) || !now.Before(time.Unix(0, applicabilityPolicy.ExpiresUnixNano)) {
		return controlledOperationProviderCurrentClosureV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceTrustInvalid, "controlled Provider Evidence policies are stale or mismatched")
	}
	notAfter = minControlledProviderTimeV2(notAfter, evidencePolicy.ExpiresUnixNano, applicabilityPolicy.ExpiresUnixNano)

	enforcement, err := g.Enforcement.InspectCurrentOperationProviderExecuteEnforcementV1(ctx, request.Operation, request.ExecuteEnforcement)
	if err != nil {
		return controlledOperationProviderCurrentClosureV2{}, err
	}
	if enforcement != request.ExecuteEnforcement || enforcement.Phase != ports.OperationDispatchEnforcementExecuteV4 {
		return controlledOperationProviderCurrentClosureV2{}, core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "controlled Provider execute Enforcement drifted")
	}
	notAfter = minControlledProviderTimeV2(notAfter, enforcement.ExpiresUnixNano)

	handoff, err := g.Handoff.InspectCurrentOperationProviderEvidenceHandoffV1(ctx, request.ExecuteEvidenceHandoff)
	if err != nil {
		return controlledOperationProviderCurrentClosureV2{}, err
	}
	if handoff.Validate() != nil || handoff.RefV3() != request.ExecuteEvidenceHandoff || handoff.Phase != enforcement || !now.Before(time.Unix(0, handoff.NotAfterUnixNano)) {
		return controlledOperationProviderCurrentClosureV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "controlled Provider Evidence handoff drifted")
	}
	qualification, err := g.Evidence.InspectCurrentOperationScopeEvidenceV3(ctx, ports.InspectCurrentOperationScopeEvidenceRequestV3{Qualification: handoff.Qualification})
	if err != nil {
		return controlledOperationProviderCurrentClosureV2{}, err
	}
	if qualification.Validate() != nil || qualification.RefV3() != handoff.Qualification || qualification.State != ports.OperationScopeEvidenceIssuedV3 || qualification.EvidencePolicy != request.EvidencePolicy || qualification.Scope.ApplicabilityPolicy != request.ApplicabilityPolicy || !ports.SameOperationSubjectV3(qualification.Scope.Operation, request.Operation) || qualification.Scope.OperationDigest != request.OperationDigest || qualification.Scope.EffectID != request.EffectID || qualification.Scope.EffectRevision != request.EffectRevision || qualification.Scope.EffectDigest != request.IntentDigest || qualification.Scope.EffectKind != request.EffectKind || qualification.Scope.AttemptID != request.Attempt.AttemptID || qualification.Scope.Phase != ports.OperationDispatchEnforcementExecuteV4 || ports.ValidateOperationScopeEvidenceActionApplicabilityV3(qualification.Scope.Applicability) != nil || !now.Before(time.Unix(0, qualification.ExpiresUnixNano)) {
		return controlledOperationProviderCurrentClosureV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "controlled Provider Evidence qualification drifted")
	}
	notAfter = minControlledProviderTimeV2(notAfter, handoff.NotAfterUnixNano, qualification.ExpiresUnixNano, qualification.IngestNotAfterUnixNano)

	boundary, err := g.Boundary.InspectCurrentOperationProviderBoundaryV1(ctx, request.Boundary)
	if err != nil {
		return controlledOperationProviderCurrentClosureV2{}, err
	}
	if err := boundary.ValidateCurrent(request.Boundary, request.Operation, request.OperationScopeDigest, request.Attempt, enforcement, request.ExecuteEvidenceHandoff, now); err != nil {
		return controlledOperationProviderCurrentClosureV2{}, err
	}
	notAfter = minControlledProviderTimeV2(notAfter, boundary.ExpiresUnixNano, request.CallerDeadlineUnixNano)
	if !now.Before(time.Unix(0, notAfter)) {
		return controlledOperationProviderCurrentClosureV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "controlled Provider unified current lifetime expired")
	}
	return controlledOperationProviderCurrentClosureV2{effect: effect, route: route, bindings: bindings, prepared: prepared, evidencePolicy: evidencePolicy, applicabilityPolicy: applicabilityPolicy, boundary: boundary, enforcement: enforcement, handoff: handoff, qualification: qualification, notAfter: notAfter}, nil
}

func minControlledProviderTimeV2(values ...int64) int64 {
	minimum := int64(0)
	for _, value := range values {
		if value > 0 && (minimum == 0 || value < minimum) {
			minimum = value
		}
	}
	return minimum
}

func (g ControlledOperationProviderGatewayV2) InspectControlledOperationProviderV2(ctx context.Context, request ports.ControlledOperationProviderInspectRequestV2) (ports.ControlledOperationProviderResultV2, error) {
	if err := request.Validate(); err != nil {
		return ports.ControlledOperationProviderResultV2{}, err
	}
	if g.Entries == nil || g.ProviderInspect == nil || g.clock == nil {
		return ports.ControlledOperationProviderResultV2{}, core.NewError(core.ErrorCapabilityUnavailable, core.ReasonComponentMissing, "controlled Provider Inspect dependencies are missing")
	}
	fact, err := g.Entries.InspectControlledOperationProviderEntryV2(ctx, request.Operation, request.Key.EntryID)
	if err != nil {
		return ports.ControlledOperationProviderResultV2{}, err
	}
	if err := fact.Validate(); err != nil {
		return ports.ControlledOperationProviderResultV2{}, err
	}
	derived, err := ports.DeriveControlledOperationProviderEntryKeyV2(fact.Request)
	if err != nil {
		return ports.ControlledOperationProviderResultV2{}, err
	}
	if fact.EntryID != request.Key.EntryID || derived != request.Key || fact.StableKeyDigest != request.Key.StableKeyDigest || fact.Request.RequestDigest != request.Key.ExpectedRequestDigest || !ports.SameOperationSubjectV3(fact.Request.Operation, request.Operation) {
		return ports.ControlledOperationProviderResultV2{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "controlled Provider Inspect key binds different Entry content")
	}
	return g.inspectEntry(ctx, fact)
}

func (g ControlledOperationProviderGatewayV2) inspectEntry(ctx context.Context, fact control.ControlledOperationProviderEntryFactV2) (ports.ControlledOperationProviderResultV2, error) {
	if err := fact.Validate(); err != nil {
		return ports.ControlledOperationProviderResultV2{}, err
	}
	switch fact.State {
	case control.ControlledOperationProviderEntryObservedV2, control.ControlledOperationProviderEntryRejectedNoEffectV2:
		return resultFromControlledOperationProviderEntryV2(fact, ports.ControlledOperationProviderErrorNoneV2, g.clock())
	}
	observation, err := g.ProviderInspect.InspectOriginalControlledProviderAttemptV2(ctx, fact.Request.Prepared, fact.Request.Attempt)
	if err != nil {
		if core.HasCategory(err, core.ErrorNotFound) {
			if fact.State == control.ControlledOperationProviderEntryEnteredV2 {
				return resultFromControlledOperationProviderEntryV2(fact, ports.ControlledOperationProviderInspectionRequiredV2, g.clock())
			}
			return resultFromControlledOperationProviderEntryV2(fact, ports.ControlledOperationProviderOutcomeUnknownV2, g.clock())
		}
		if core.HasCategory(err, core.ErrorUnavailable) || core.HasCategory(err, core.ErrorIndeterminate) {
			return resultFromControlledOperationProviderEntryV2(fact, ports.ControlledOperationProviderInspectionUnavailableV2, g.clock())
		}
		return ports.ControlledOperationProviderResultV2{}, err
	}
	if observation.Validate() != nil || observation.PreparedAttemptID != fact.Request.Prepared.ID || observation.Delegation != fact.Request.PreparedSemantics.Delegation {
		return ports.ControlledOperationProviderResultV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "controlled Provider observation belongs to another attempt")
	}
	return g.transitionAndInspect(ctx, fact, control.ControlledOperationProviderEntryObservedV2, fact.AdmissionReceipt, &observation)
}

func (g ControlledOperationProviderGatewayV2) transitionAndInspect(ctx context.Context, current control.ControlledOperationProviderEntryFactV2, state control.ControlledOperationProviderEntryStateV2, receipt *ports.ControlledOperationProviderAdmissionReceiptRefV2, observation *ports.ProviderAttemptObservationRefV2) (ports.ControlledOperationProviderResultV2, error) {
	now := g.clock()
	if now.IsZero() || now.Before(time.Unix(0, current.UpdatedUnixNano)) {
		return ports.ControlledOperationProviderResultV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "controlled Provider Entry transition clock regressed")
	}
	next := current
	next.Revision++
	next.State = state
	next.AdmissionReceipt = receipt
	next.Observation = observation
	next.UpdatedUnixNano = now.UnixNano()
	sealed, err := control.SealControlledOperationProviderEntryFactV2(next)
	if err != nil {
		return ports.ControlledOperationProviderResultV2{}, err
	}
	stored, err := g.Entries.CompareAndSwapControlledOperationProviderEntryV2(ctx, current.Request.Operation, control.ControlledOperationProviderEntryCASRequestV2{ExpectedRevision: current.Revision, Next: sealed})
	if err != nil {
		if !core.HasCategory(err, core.ErrorUnavailable) && !core.HasCategory(err, core.ErrorIndeterminate) && !core.HasCategory(err, core.ErrorConflict) {
			return ports.ControlledOperationProviderResultV2{}, err
		}
		stored, err = g.Entries.InspectControlledOperationProviderEntryV2(context.WithoutCancel(ctx), current.Request.Operation, current.EntryID)
		if err != nil || !control.IsControlledOperationProviderEntryRecoverySuccessV2(sealed, stored) {
			if err != nil {
				return ports.ControlledOperationProviderResultV2{}, err
			}
			return ports.ControlledOperationProviderResultV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "controlled Provider Entry transition recovery found different content")
		}
		return g.inspectEntry(context.WithoutCancel(ctx), stored)
	}
	resultError := ports.ControlledOperationProviderErrorNoneV2
	if stored.State == control.ControlledOperationProviderEntryUnknownV2 {
		resultError = ports.ControlledOperationProviderOutcomeUnknownV2
	}
	return resultFromControlledOperationProviderEntryV2(stored, resultError, g.clock())
}

func resultFromControlledOperationProviderEntryV2(fact control.ControlledOperationProviderEntryFactV2, resultError ports.ControlledOperationProviderResultErrorV2, now time.Time) (ports.ControlledOperationProviderResultV2, error) {
	if now.IsZero() {
		return ports.ControlledOperationProviderResultV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "controlled Provider result clock is zero")
	}
	status := ports.ControlledOperationProviderResultStatusV2(fact.State)
	if status == ports.ControlledOperationProviderObservedV2 || status == ports.ControlledOperationProviderRejectedNoEffectV2 {
		resultError = ports.ControlledOperationProviderErrorNoneV2
	}
	receipt := fact.AdmissionReceipt
	if status != ports.ControlledOperationProviderRejectedNoEffectV2 {
		receipt = nil
	}
	return ports.SealControlledOperationProviderResultV2(ports.ControlledOperationProviderResultV2{
		EntryRef: fact.RefV2(), Status: status, Error: resultError, Prepared: fact.Request.Prepared,
		Attempt: fact.Request.Attempt, AdmissionReceipt: receipt, Observation: fact.Observation,
		InspectedUnixNano: now.UnixNano(),
	})
}

var _ ports.ControlledOperationProviderPortV2 = ControlledOperationProviderGatewayV2{}
