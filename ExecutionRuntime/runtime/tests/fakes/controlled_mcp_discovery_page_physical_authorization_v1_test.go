package fakes_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestControlledMCPDiscoveryPagePhysicalAuthorizationV1ExactClosure(t *testing.T) {
	fixture := newControlledMCPDiscoveryPageAuthorizationFixtureV1(t, "positive")
	gateway := fixture.gateway(t, func() time.Time { return fixture.now })
	authorization, err := gateway.AuthorizeControlledMCPDiscoveryPagePhysicalV1(context.Background(), fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	if err := authorization.ValidateCurrent(fixture.now); err != nil {
		t.Fatal(err)
	}
	if authorization.EffectFactRevision != fixture.readers.effect.FactRevision || authorization.Provider != fixture.readers.route.Provider || authorization.ProviderTransport != fixture.readers.route.ProviderTransport || authorization.PrepareConsumption != fixture.request.PrepareConsumption || authorization.ExecuteHandoff != fixture.request.ExecuteHandoff || authorization.Association != fixture.readers.association.Ref || authorization.ConnectionAvailability != fixture.readers.availability.Ref || authorization.Namespace != fixture.request.Namespace || authorization.CursorDigest != fixture.request.CursorDigest || authorization.PageOrdinal != fixture.request.PageOrdinal {
		t.Fatalf("authorization lost exact closure coordinates: %#v", authorization)
	}
	if fixture.readers.calls.Load() < 20 {
		t.Fatalf("S1/S2 did not re-read all current owners: calls=%d", fixture.readers.calls.Load())
	}
}

func TestControlledMCPDiscoveryPagePhysicalAuthorizationV1FailsClosed(t *testing.T) {
	t.Run("nil_context", func(t *testing.T) {
		fixture := newControlledMCPDiscoveryPageAuthorizationFixtureV1(t, "nil-context")
		if _, err := fixture.gateway(t, func() time.Time { return fixture.now }).AuthorizeControlledMCPDiscoveryPagePhysicalV1(nil, fixture.request); !core.HasCategory(err, core.ErrorInvalidArgument) {
			t.Fatalf("nil context error=%v", err)
		}
	})

	t.Run("typed_nil", func(t *testing.T) {
		fixture := newControlledMCPDiscoveryPageAuthorizationFixtureV1(t, "typed-nil")
		var routes *controlledMCPDiscoveryPageReadersV1
		if _, err := kernel.NewControlledMCPDiscoveryPagePhysicalAuthorizationGatewayV1(routes, fixture.readers, fixture.readers, fixture.readers, fixture.readers, fixture.readers, fixture.readers, fixture.readers, fixture.readers, fixture.readers, func() time.Time { return fixture.now }); !core.HasReason(err, core.ReasonComponentMissing) {
			t.Fatalf("typed nil dependency error=%v", err)
		}
	})

	t.Run("clock_rollback", func(t *testing.T) {
		fixture := newControlledMCPDiscoveryPageAuthorizationFixtureV1(t, "clock-rollback")
		values := []time.Time{fixture.now, fixture.now.Add(-time.Nanosecond)}
		var index atomic.Int64
		clock := func() time.Time {
			value := values[min(int(index.Add(1)-1), len(values)-1)]
			return value
		}
		if _, err := fixture.gateway(t, clock).AuthorizeControlledMCPDiscoveryPagePhysicalV1(context.Background(), fixture.request); !core.HasReason(err, core.ReasonClockRegression) {
			t.Fatalf("clock rollback error=%v", err)
		}
	})

	t.Run("s1_s2_route_drift", func(t *testing.T) {
		fixture := newControlledMCPDiscoveryPageAuthorizationFixtureV1(t, "route-drift")
		changed := fixture.readers.route
		changed.CheckedUnixNano++
		changed.ProjectionDigest = ""
		var err error
		changed, err = ports.SealControlledMCPDiscoveryPageRouteCurrentProjectionV1(changed)
		if err != nil {
			t.Fatal(err)
		}
		fixture.readers.routeSecond = &changed
		if _, err = fixture.gateway(t, func() time.Time { return fixture.now }).AuthorizeControlledMCPDiscoveryPagePhysicalV1(context.Background(), fixture.request); !core.HasReason(err, core.ReasonBindingDrift) {
			t.Fatalf("S1/S2 route drift error=%v", err)
		}
	})

	t.Run("prepare_execute_evidence_reuse", func(t *testing.T) {
		fixture := newControlledMCPDiscoveryPageAuthorizationFixtureV1(t, "evidence-reuse")
		fixture.request.ExecuteHandoff = fixture.readers.prepareHandoff.RefV3()
		fixture.readers.executeHandoff = fixture.readers.prepareHandoff
		fixture.readers.executeQualification = fixture.readers.prepareQualification
		if _, err := fixture.gateway(t, func() time.Time { return fixture.now }).AuthorizeControlledMCPDiscoveryPagePhysicalV1(context.Background(), fixture.request); err == nil {
			t.Fatal("prepare Evidence was reused as execute authority")
		}
	})

	t.Run("owner_drift", func(t *testing.T) {
		fixture := newControlledMCPDiscoveryPageAuthorizationFixtureV1(t, "owner-drift")
		fixture.request.DomainCommand.Owner.ComponentID = "praxis.mcp/other-owner"
		if _, err := fixture.gateway(t, func() time.Time { return fixture.now }).AuthorizeControlledMCPDiscoveryPagePhysicalV1(context.Background(), fixture.request); err == nil {
			t.Fatal("domain command Owner drift was accepted")
		}
	})
}

func TestControlledMCPDiscoveryPagePhysicalAuthorizationV1AvailabilityAndPageBinding(t *testing.T) {
	t.Run("availability_s1_s2_drift", func(t *testing.T) {
		fixture := newControlledMCPDiscoveryPageAuthorizationFixtureV1(t, "availability-drift")
		changed := fixture.readers.availability
		changed.CheckedUnixNano++
		changed.ProjectionDigest = ""
		var err error
		changed, err = ports.SealMCPConnectionAvailabilityNeutralProjectionV1(changed)
		if err != nil {
			t.Fatal(err)
		}
		fixture.readers.availabilitySecond = &changed
		if _, err = fixture.gateway(t, func() time.Time { return fixture.now }).AuthorizeControlledMCPDiscoveryPagePhysicalV1(context.Background(), fixture.request); !core.HasReason(err, core.ReasonBindingDrift) {
			t.Fatalf("availability drift error=%v", err)
		}
	})
	for _, mutate := range []struct {
		name  string
		apply func(*ports.ControlledMCPDiscoveryPagePhysicalAuthorizationRequestV1)
	}{
		{"unknown_namespace", func(request *ports.ControlledMCPDiscoveryPagePhysicalAuthorizationRequestV1) {
			request.Namespace = "praxis.mcp/unknown"
		}},
		{"missing_cursor", func(request *ports.ControlledMCPDiscoveryPagePhysicalAuthorizationRequestV1) {
			request.CursorDigest = ""
		}},
		{"availability_owner_drift", func(request *ports.ControlledMCPDiscoveryPagePhysicalAuthorizationRequestV1) {
			request.ConnectionAvailability.Owner.ComponentID = "praxis.mcp/other-owner"
		}},
	} {
		t.Run(mutate.name, func(t *testing.T) {
			fixture := newControlledMCPDiscoveryPageAuthorizationFixtureV1(t, mutate.name)
			mutate.apply(&fixture.request)
			if _, err := fixture.gateway(t, func() time.Time { return fixture.now }).AuthorizeControlledMCPDiscoveryPagePhysicalV1(context.Background(), fixture.request); err == nil {
				t.Fatal("drifted page request was authorized")
			}
		})
	}
}

type controlledMCPDiscoveryPageAuthorizationFixtureV1 struct {
	now     time.Time
	request ports.ControlledMCPDiscoveryPagePhysicalAuthorizationRequestV1
	readers *controlledMCPDiscoveryPageReadersV1
}

func (f controlledMCPDiscoveryPageAuthorizationFixtureV1) gateway(t *testing.T, clock func() time.Time) *kernel.ControlledMCPDiscoveryPagePhysicalAuthorizationGatewayV1 {
	t.Helper()
	gateway, err := kernel.NewControlledMCPDiscoveryPagePhysicalAuthorizationGatewayV1(f.readers, f.readers, f.readers, f.readers, f.readers, f.readers, f.readers, f.readers, f.readers, f.readers, clock)
	if err != nil {
		t.Fatal(err)
	}
	return gateway
}

func newControlledMCPDiscoveryPageAuthorizationFixtureV1(t *testing.T, suffix string) controlledMCPDiscoveryPageAuthorizationFixtureV1 {
	t.Helper()
	enforcement := newOperationEnforcementFixtureForScopeV4(t, "mcp-discovery-page-"+suffix, "run-mcp-discovery-page", "tenant-mcp-discovery-page", ports.OperationScopeEvidenceMCPDiscoveryPageEffectKindV1)
	now := enforcement.effect.now
	preparedPhase, err := enforcement.enforcement.EnforceCurrentOperationDispatchV4(context.Background(), enforcement.prepare)
	if err != nil {
		t.Fatal(err)
	}
	prepared := preparedAttemptForEnforcementV4(t, enforcement, preparedPhase)
	executeEnforcementRequest := enforcement.prepare
	executeEnforcementRequest.Phase = ports.OperationDispatchEnforcementExecuteV4
	executeEnforcementRequest.ExpectedJournalRevision = 1
	executeEnforcementRequest.Prepare = &preparedPhase.Phase
	executeEnforcementRequest.PreparedAttempt = prepared
	executed, err := enforcement.enforcement.EnforceCurrentOperationDispatchV4(context.Background(), executeEnforcementRequest)
	if err != nil {
		t.Fatal(err)
	}

	intent := enforcement.effect.intent
	intentDigest, err := intent.DigestV3()
	if err != nil {
		t.Fatal(err)
	}
	effectFact, err := enforcement.effect.store.InspectOperationEffectV3(context.Background(), intent.Operation, intent.ID)
	if err != nil {
		t.Fatal(err)
	}
	effectCurrent, err := ports.SealControlledOperationEffectCurrentProjectionV2(ports.ControlledOperationEffectCurrentProjectionV2{
		Intent: intent, IntentDigest: intentDigest, FactRevision: effectFact.Revision, State: string(effectFact.State),
		CheckedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(6 * time.Second).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}

	legacyPermit := executed.Dispatch.Record.Permit.LegacyPermit
	delegation := ports.ExecutionDelegationRefV2{ID: prepared.DeclaredDelegation.ID, Revision: prepared.DeclaredDelegation.Revision + 1, Digest: digestV3("mcp-discovery-page-prepared-delegation-" + suffix)}
	persisted := ports.PersistedOperationEnforcementRefV3{
		PermitID: executed.Phase.PermitID, PermitRevision: prepared.PermitRevision, PermitDigest: prepared.PermitDigest,
		AttemptID: prepared.AttemptID, OperationDigest: prepared.OperationDigest, Provider: prepared.Provider,
		ReceiptDigest: digestV3("mcp-discovery-page-persisted-enforcement-" + suffix), RecordedRevision: executed.Phase.PermitFactRevision,
	}
	execute := ports.ExecutePreparedRequestV2{Delegation: delegation, Prepared: *prepared, Enforcement: persisted, Intent: intent, Permit: legacyPermit, Fence: executed.Dispatch.Record.Fence}
	if err := execute.Validate(); err != nil {
		t.Fatal(err)
	}
	attempt := ports.OperationDispatchAttemptRefV3{
		OperationDigest: prepared.OperationDigest, EffectID: prepared.IntentID, IntentRevision: prepared.IntentRevision, IntentDigest: prepared.IntentDigest,
		PermitID: prepared.PermitID, PermitRevision: prepared.PermitRevision, PermitDigest: prepared.PermitDigest, AttemptID: prepared.AttemptID,
	}
	dispatch := ports.OperationDispatchCurrentProjectionV3{
		Operation: intent.Operation, Permit: legacyPermit, PermitDigest: prepared.PermitDigest,
		PermitFactRevision: 3, PermitFactState: "begun", Enforcement: &persisted,
		Delegation: delegation, DelegationState: ports.ExecutionDelegationPreparedV2, PreparedAttemptID: prepared.ID,
		PreparationDigest: digestV3("mcp-discovery-page-preparation-" + suffix), ExpiresUnixNano: now.Add(6 * time.Second).UnixNano(),
	}
	if err := dispatch.ValidateForExecute(execute, enforcement.effect.current.snapshot, now); err != nil {
		t.Fatal(err)
	}

	provider := intent.Provider
	transport := ports.ProviderBindingRefV2{
		BindingSetID: provider.BindingSetID, BindingSetRevision: provider.BindingSetRevision,
		ComponentID: "praxis.mcp/controlled-transport", ManifestDigest: digestV3("mcp-discovery-page-transport-manifest-" + suffix),
		ArtifactDigest: digestV3("mcp-discovery-page-transport-artifact-" + suffix), Capability: ports.ControlledMCPDiscoveryPageProviderTransportCapabilityV1,
	}
	declaration := ports.ControlledOperationProviderRouteDeclarationRefV2{RouteID: "mcp-discovery-page-route-" + suffix, Revision: 1, PublisherComponentID: "praxis.harness/assembly", DeclarationDigest: digestV3("mcp-discovery-page-route-declaration-" + suffix)}
	conformance := ports.ControlledOperationProviderRouteConformanceRefV2{ConformanceID: "mcp-discovery-page-route-conformance-" + suffix, Revision: 1, DeclarationRef: declaration, ConformanceDigest: digestV3("mcp-discovery-page-route-conformance-" + suffix)}
	route, err := ports.SealControlledMCPDiscoveryPageRouteCurrentProjectionV1(ports.ControlledMCPDiscoveryPageRouteCurrentProjectionV1{
		Ref:        ports.ControlledMCPDiscoveryPageRouteCurrentRefV1{Revision: 1, DeclarationRef: declaration, ConformanceRef: conformance},
		Generation: ports.GenerationArtifactRefV1{ID: "mcp-discovery-page-generation-" + suffix, Revision: 1, Digest: digestV3("mcp-discovery-page-generation-" + suffix), InputDigest: digestV3("mcp-discovery-page-generation-input-" + suffix), ManifestDigest: digestV3("mcp-discovery-page-generation-manifest-" + suffix), GraphDigest: digestV3("mcp-discovery-page-generation-graph-" + suffix), CatalogDigest: digestV3("mcp-discovery-page-generation-catalog-" + suffix)},
		Assembly:   enforcement.sandbox.value.Generation,
		HandoffID:  "mcp-discovery-page-assembly-handoff-" + suffix, HandoffRevision: 1, HandoffDigest: digestV3("mcp-discovery-page-assembly-handoff-" + suffix),
		BindingSetID: provider.BindingSetID, BindingSetRevision: provider.BindingSetRevision, BindingSetDigest: digestV3("mcp-discovery-page-binding-set-" + suffix),
		BindingSetSemanticDigest: digestV3("mcp-discovery-page-binding-semantic-" + suffix), BindingSetCurrentnessDigest: digestV3("mcp-discovery-page-binding-currentness-" + suffix),
		ActiveRouteID: "mcp-discovery-page-active-route-" + suffix, ActiveRouteRevision: 1, ActiveRouteDigest: digestV3("mcp-discovery-page-active-route-" + suffix),
		ProviderTransport: transport, Provider: provider, CheckedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(5 * time.Second).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}

	applicability := controlledMCPDiscoveryPageApplicabilityV1(suffix)
	appPolicy := ports.OperationScopeEvidenceApplicabilityPolicyRefV3{ID: "mcp-discovery-page-app-policy-" + suffix, Revision: 1, Digest: digestV3("mcp-discovery-page-app-policy-" + suffix), ExpiresUnixNano: now.Add(6 * time.Second).UnixNano()}
	prepareQualification, prepareHandoff, prepareConsumption := controlledMCPDiscoveryPagePrepareEvidenceFixtureV1(t, suffix, now, effectFact.Revision, intent, attempt, preparedPhase.Phase, route.Assembly, appPolicy, applicability)
	executeQualification, executeHandoff := controlledMCPDiscoveryPageExecuteEvidenceFixtureV1(t, suffix, now, effectFact.Revision, intent, attempt, executed.Phase, route.Assembly, appPolicy, applicability)

	command := ports.OperationDomainCommandRefV1{
		Owner: ports.EffectOwnerRefV2{Role: ports.OwnerSettlement, ComponentID: provider.ComponentID, ManifestDigest: provider.ManifestDigest},
		Kind:  "praxis.mcp/connect-command", ID: "mcp-discovery-page-command-" + suffix, Revision: 1, Digest: digestV3("mcp-discovery-page-command-" + suffix),
	}
	association, err := ports.SealPreparedDomainCommandAssociationCurrentProjectionV1(ports.PreparedDomainCommandAssociationCurrentProjectionV1{
		Operation: intent.Operation, OperationDigest: attempt.OperationDigest, EffectID: intent.ID, EffectRevision: intent.Revision, IntentDigest: intentDigest,
		Prepared: *prepared, Attempt: attempt, Provider: provider, PayloadSchema: prepared.PayloadSchema, PayloadDigest: prepared.PayloadDigest, PayloadRevision: prepared.PayloadRevision,
		DomainCommand: command, CheckedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(4 * time.Second).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}

	availability, err := ports.SealMCPConnectionAvailabilityNeutralProjectionV1(ports.MCPConnectionAvailabilityNeutralProjectionV1{
		Ref: ports.MCPConnectionAvailabilityNeutralRefV1{
			Owner:        command.Owner,
			ConnectionID: "mcp-discovery-page-connection-" + suffix, ConnectionRevision: 1, ConnectionDigest: digestV3("mcp-discovery-page-connection-" + suffix),
			ApplyID: "mcp-discovery-page-connect-apply-" + suffix, ApplyRevision: 1, ApplyDigest: digestV3("mcp-discovery-page-connect-apply-" + suffix),
			DomainResultID: "mcp-discovery-page-connect-result-" + suffix, DomainResultRevision: 1, DomainResultDigest: digestV3("mcp-discovery-page-connect-result-" + suffix),
			SourceProjectionDigest: digestV3("mcp-discovery-page-connect-source-" + suffix),
		},
		TenantID: intent.Operation.ExecutionScope.Identity.TenantID, RunID: string(intent.Operation.RunID),
		SessionID: "mcp-discovery-page-session-" + suffix, SessionRevision: 1, SessionDigest: digestV3("mcp-discovery-page-session-" + suffix), ConnectionEpoch: 1,
		ProviderTransport: transport, Provider: provider,
		CheckedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(4 * time.Second).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	request := ports.ControlledMCPDiscoveryPagePhysicalAuthorizationRequestV1{
		Route: route.Ref, Execute: execute, Attempt: attempt, ExecuteEnforcement: executed.Phase,
		PrepareConsumption: prepareConsumption.RefV3(), ExecuteHandoff: executeHandoff.RefV3(),
		Association: association.Ref, DomainCommand: command, ConnectionAvailability: availability.Ref,
		Namespace: ports.MCPDiscoveryPageToolsNamespaceV1, CursorDigest: digestV3("mcp-discovery-page-cursor-" + suffix), PageOrdinal: 0, CallerDeadlineUnixNano: now.Add(4 * time.Second).UnixNano(),
	}
	if err := request.Validate(); err != nil {
		t.Fatal(err)
	}
	readers := &controlledMCPDiscoveryPageReadersV1{
		route: route, effect: effectCurrent, governance: enforcement.effect.current.snapshot, dispatch: dispatch,
		sandbox: enforcement.sandbox.value, enforcement: executed.Phase,
		prepareConsumption: prepareConsumption, prepareQualification: prepareQualification, prepareHandoff: prepareHandoff,
		executeQualification: executeQualification, executeHandoff: executeHandoff, association: association, availability: availability,
	}
	return controlledMCPDiscoveryPageAuthorizationFixtureV1{now: now, request: request, readers: readers}
}

func controlledMCPDiscoveryPageApplicabilityV1(suffix string) []ports.OperationScopeEvidenceApplicabilityV3 {
	values := make([]ports.OperationScopeEvidenceApplicabilityV3, 0, 5)
	for _, dimension := range []ports.OperationScopeEvidenceApplicabilityDimensionV3{ports.OperationScopeEvidenceRunV3, ports.OperationScopeEvidenceSessionV3, ports.OperationScopeEvidenceTurnV3, ports.OperationScopeEvidenceActionV3, ports.OperationScopeEvidenceContextV3} {
		value := ports.OperationScopeEvidenceApplicabilityV3{Dimension: dimension, Mode: ports.OperationScopeEvidenceForbiddenV3}
		for _, route := range ports.OperationScopeEvidenceMCPDiscoveryPageRoutesV1() {
			if route.Dimension == dimension {
				ref := ports.OperationScopeEvidenceApplicabilityFactRefV3{Kind: route.Kind, ID: "mcp-discovery-page-" + string(dimension) + "-" + suffix, Revision: 1, Digest: digestV3("mcp-discovery-page-" + string(dimension) + "-" + suffix)}
				value.Mode, value.Fact = ports.OperationScopeEvidenceRequiredV3, &ref
			}
		}
		values = append(values, value)
	}
	return ports.NormalizeOperationScopeEvidenceApplicabilityV3(values)
}

func controlledMCPDiscoveryPageEvidenceBaseV1(t *testing.T, suffix string, now time.Time, effectFactRevision core.Revision, intent ports.OperationEffectIntentV3, attempt ports.OperationDispatchAttemptRefV3, phase ports.OperationDispatchEnforcementPhaseRefV4, generation ports.GenerationBindingAssociationRefV1, appPolicy ports.OperationScopeEvidenceApplicabilityPolicyRefV3, applicability []ports.OperationScopeEvidenceApplicabilityV3, label string) ports.OperationScopeEvidenceQualificationFactV3 {
	t.Helper()
	scope := ports.OperationScopeEvidenceScopeV3{
		LedgerScope: ports.OperationScopeEvidenceLedgerScopeV3{TenantID: intent.Operation.ExecutionScope.Identity.TenantID, OperationDigest: attempt.OperationDigest, ChainID: "mcp-discovery-page-chain-" + label + "-" + suffix},
		Operation:   intent.Operation, OperationDigest: attempt.OperationDigest, EffectID: intent.ID, EffectRevision: effectFactRevision,
		EffectDigest: attempt.IntentDigest, EffectKind: ports.OperationScopeEvidenceMCPDiscoveryPageEffectKindV1, AttemptID: attempt.AttemptID,
		Phase: phase.Phase, ApplicabilityPolicy: appPolicy, Applicability: applicability, Generation: generation,
	}
	runtimeCurrent, err := ports.SealOperationScopeEvidenceRuntimeCurrentProjectionV3(ports.OperationScopeEvidenceRuntimeCurrentProjectionV3{
		Scope: scope, PermitID: phase.PermitID, PermitFactRevision: phase.PermitFactRevision, PermitDigest: phase.PermitDigest,
		AdmissionDigest: phase.AdmissionDigest, Authorization: phase.ReviewAuthorization, Phase: phase,
		CheckedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(5 * time.Second).UnixNano(),
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	registration := ports.OperationScopeEvidenceFactRefV3{ID: "mcp-discovery-page-registration-" + label + "-" + suffix, Revision: 1, Digest: digestV3("mcp-discovery-page-registration-" + label + "-" + suffix), ExpiresUnixNano: now.Add(8 * time.Second).UnixNano()}
	schema := ports.SchemaRefV2{Namespace: "praxis.mcp", Name: "connect-" + label, Version: "1.0.0", MediaType: "application/json", ContentDigest: digestV3("mcp-discovery-page-schema-" + label)}
	qualification, err := ports.SealOperationScopeEvidenceQualificationFactV3(ports.OperationScopeEvidenceQualificationFactV3{
		ID: "mcp-discovery-page-qualification-" + label + "-" + suffix, Revision: 1, State: ports.OperationScopeEvidenceIssuedV3,
		Scope: scope, Runtime: runtimeCurrent,
		EvidencePolicy: ports.OperationScopeEvidencePolicyRefV3{ID: "mcp-discovery-page-evidence-policy-" + label + "-" + suffix, Revision: 1, Digest: digestV3("mcp-discovery-page-evidence-policy-" + label + "-" + suffix), ExpiresUnixNano: now.Add(6 * time.Second).UnixNano()},
		Reservation:    ports.OperationScopeEvidenceSourceReservationV3{Registration: registration, Source: ports.OperationScopeEvidenceSourceKeyV3{RegistrationID: registration.ID, SourceEpoch: 1, SourceSequence: 1}, EventID: "mcp-discovery-page-event-" + label + "-" + suffix, Schema: schema},
		RequestedTTL:   4 * time.Second, CreatedUnixNano: now.Add(-time.Second).UnixNano(), UpdatedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(4 * time.Second).UnixNano(), IngestNotAfterUnixNano: now.Add(5 * time.Second).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return qualification
}

func controlledMCPDiscoveryPagePrepareEvidenceFixtureV1(t *testing.T, suffix string, now time.Time, effectFactRevision core.Revision, intent ports.OperationEffectIntentV3, attempt ports.OperationDispatchAttemptRefV3, phase ports.OperationDispatchEnforcementPhaseRefV4, generation ports.GenerationBindingAssociationRefV1, appPolicy ports.OperationScopeEvidenceApplicabilityPolicyRefV3, applicability []ports.OperationScopeEvidenceApplicabilityV3) (ports.OperationScopeEvidenceQualificationFactV3, ports.OperationScopeEvidenceProviderHandoffFactV3, ports.OperationScopeEvidenceConsumptionFactV3) {
	issued := controlledMCPDiscoveryPageEvidenceBaseV1(t, suffix, now, effectFactRevision, intent, attempt, phase, generation, appPolicy, applicability, "prepare")
	handoff, err := ports.SealOperationScopeEvidenceProviderHandoffFactV3(ports.OperationScopeEvidenceProviderHandoffFactV3{ID: "mcp-discovery-page-handoff-prepare-" + suffix, Revision: 1, Qualification: issued.RefV3(), Phase: phase, CheckedUnixNano: now.Add(-time.Second).UnixNano(), NotAfterUnixNano: now.Add(4 * time.Second).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	record := ports.OperationScopeEvidenceRecordRefV3{LedgerScopeDigest: digestV3("mcp-discovery-page-ledger-prepare-" + suffix), Sequence: 1, RecordDigest: digestV3("mcp-discovery-page-record-prepare-" + suffix)}
	consumption, err := ports.SealOperationScopeEvidenceConsumptionFactV3(ports.OperationScopeEvidenceConsumptionFactV3{ID: "mcp-discovery-page-consumption-prepare-" + suffix, Revision: 1, Qualification: issued.RefV3(), Handoff: handoff.RefV3(), CandidateDigest: digestV3("mcp-discovery-page-candidate-prepare-" + suffix), Record: record, CreatedUnixNano: now.UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	current := issued
	current.Revision = 2
	current.State = ports.OperationScopeEvidenceConsumedCurrentV3
	ref := consumption.RefV3()
	current.Consumption = &ref
	current.UpdatedUnixNano = now.UnixNano()
	current.Digest = ""
	current, err = ports.SealOperationScopeEvidenceQualificationFactV3(current)
	if err != nil {
		t.Fatal(err)
	}
	return current, handoff, consumption
}

func controlledMCPDiscoveryPageExecuteEvidenceFixtureV1(t *testing.T, suffix string, now time.Time, effectFactRevision core.Revision, intent ports.OperationEffectIntentV3, attempt ports.OperationDispatchAttemptRefV3, phase ports.OperationDispatchEnforcementPhaseRefV4, generation ports.GenerationBindingAssociationRefV1, appPolicy ports.OperationScopeEvidenceApplicabilityPolicyRefV3, applicability []ports.OperationScopeEvidenceApplicabilityV3) (ports.OperationScopeEvidenceQualificationFactV3, ports.OperationScopeEvidenceProviderHandoffFactV3) {
	qualification := controlledMCPDiscoveryPageEvidenceBaseV1(t, suffix, now, effectFactRevision, intent, attempt, phase, generation, appPolicy, applicability, "execute")
	handoff, err := ports.SealOperationScopeEvidenceProviderHandoffFactV3(ports.OperationScopeEvidenceProviderHandoffFactV3{ID: "mcp-discovery-page-handoff-execute-" + suffix, Revision: 1, Qualification: qualification.RefV3(), Phase: phase, CheckedUnixNano: now.Add(-time.Second).UnixNano(), NotAfterUnixNano: now.Add(4 * time.Second).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	return qualification, handoff
}

type controlledMCPDiscoveryPageReadersV1 struct {
	callIndex            atomic.Int64
	calls                atomic.Int64
	route                ports.ControlledMCPDiscoveryPageRouteCurrentProjectionV1
	routeSecond          *ports.ControlledMCPDiscoveryPageRouteCurrentProjectionV1
	effect               ports.ControlledOperationEffectCurrentProjectionV2
	governance           ports.OperationGovernanceSnapshotV3
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
	availabilitySecond   *ports.MCPConnectionAvailabilityNeutralProjectionV1
}

func (r *controlledMCPDiscoveryPageReadersV1) InspectCurrentControlledMCPDiscoveryPageRouteV1(context.Context, ports.ControlledMCPDiscoveryPageRouteCurrentRefV1) (ports.ControlledMCPDiscoveryPageRouteCurrentProjectionV1, error) {
	r.calls.Add(1)
	if r.callIndex.Add(1) > 1 && r.routeSecond != nil {
		return *r.routeSecond, nil
	}
	return r.route, nil
}
func (r *controlledMCPDiscoveryPageReadersV1) InspectCurrentControlledOperationEffectV2(context.Context, ports.OperationSubjectV3, core.EffectIntentID) (ports.ControlledOperationEffectCurrentProjectionV2, error) {
	r.calls.Add(1)
	return r.effect, nil
}
func (r *controlledMCPDiscoveryPageReadersV1) InspectOperationGovernance(context.Context, ports.OperationSubjectV3) (ports.OperationGovernanceSnapshotV3, error) {
	r.calls.Add(1)
	return r.governance, nil
}
func (r *controlledMCPDiscoveryPageReadersV1) InspectOperationDispatch(context.Context, ports.OperationSubjectV3, string, string) (ports.OperationDispatchCurrentProjectionV3, error) {
	r.calls.Add(1)
	return r.dispatch, nil
}
func (r *controlledMCPDiscoveryPageReadersV1) InspectOperationDispatchSandboxCurrentV4(context.Context, ports.OperationSubjectV3, core.EffectIntentID, ports.OperationDispatchSandboxFactRefV4) (ports.OperationDispatchSandboxCurrentProjectionV4, error) {
	r.calls.Add(1)
	return r.sandbox, nil
}
func (r *controlledMCPDiscoveryPageReadersV1) InspectCurrentOperationProviderExecuteEnforcementV1(context.Context, ports.OperationSubjectV3, ports.OperationDispatchEnforcementPhaseRefV4) (ports.OperationDispatchEnforcementPhaseRefV4, error) {
	r.calls.Add(1)
	return r.enforcement, nil
}
func (r *controlledMCPDiscoveryPageReadersV1) InspectOperationScopeEvidenceConsumptionClosureV1(context.Context, ports.OperationScopeEvidenceConsumptionRefV3) (ports.OperationScopeEvidenceConsumptionFactV3, ports.OperationScopeEvidenceQualificationFactV3, ports.OperationScopeEvidenceProviderHandoffFactV3, error) {
	r.calls.Add(1)
	return r.prepareConsumption, r.prepareQualification, r.prepareHandoff, nil
}
func (r *controlledMCPDiscoveryPageReadersV1) InspectOperationScopeEvidenceProviderHandoffClosureV1(context.Context, ports.OperationScopeEvidenceProviderHandoffRefV3) (ports.OperationScopeEvidenceProviderHandoffFactV3, ports.OperationScopeEvidenceQualificationFactV3, error) {
	r.calls.Add(1)
	return r.executeHandoff, r.executeQualification, nil
}
func (r *controlledMCPDiscoveryPageReadersV1) InspectCurrentPreparedDomainCommandAssociationV1(context.Context, ports.PreparedDomainCommandAssociationRefV1) (ports.PreparedDomainCommandAssociationCurrentProjectionV1, error) {
	r.calls.Add(1)
	return r.association, nil
}

func (r *controlledMCPDiscoveryPageReadersV1) InspectCurrentMCPConnectionAvailabilityNeutralV1(context.Context, ports.MCPConnectionAvailabilityNeutralRefV1) (ports.MCPConnectionAvailabilityNeutralProjectionV1, error) {
	r.calls.Add(1)
	if r.availabilitySecond != nil && r.calls.Load() >= 20 {
		return *r.availabilitySecond, nil
	}
	return r.availability, nil
}

var _ ports.ControlledMCPDiscoveryPageRouteCurrentReaderV1 = (*controlledMCPDiscoveryPageReadersV1)(nil)
var _ ports.ControlledOperationEffectCurrentReaderV2 = (*controlledMCPDiscoveryPageReadersV1)(nil)
var _ ports.OperationGovernanceCurrentReaderV3 = (*controlledMCPDiscoveryPageReadersV1)(nil)
var _ ports.OperationDispatchCurrentReaderV3 = (*controlledMCPDiscoveryPageReadersV1)(nil)
var _ ports.OperationDispatchSandboxCurrentReaderV4 = (*controlledMCPDiscoveryPageReadersV1)(nil)
var _ ports.OperationProviderExecuteEnforcementCurrentReaderV1 = (*controlledMCPDiscoveryPageReadersV1)(nil)
var _ ports.OperationScopeEvidenceConsumptionClosureReaderV1 = (*controlledMCPDiscoveryPageReadersV1)(nil)
var _ ports.OperationScopeEvidenceProviderHandoffClosureReaderV1 = (*controlledMCPDiscoveryPageReadersV1)(nil)
var _ ports.PreparedDomainCommandAssociationCurrentReaderV1 = (*controlledMCPDiscoveryPageReadersV1)(nil)
var _ ports.MCPConnectionAvailabilityNeutralCurrentReaderV1 = (*controlledMCPDiscoveryPageReadersV1)(nil)
