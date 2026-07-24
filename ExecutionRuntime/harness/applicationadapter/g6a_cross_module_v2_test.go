package applicationadapter

import (
	"context"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	application "github.com/Proview-China/rax/ExecutionRuntime/application"
	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationfakes "github.com/Proview-China/rax/ExecutionRuntime/application/fakes"
	harnesscontract "github.com/Proview-China/rax/ExecutionRuntime/harness/contract"
	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	tooladapter "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/applicationadapter"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

// TestSingleCallG6ACompositionV2 deliberately stops at the settled Tool result.
// It composes the real Harness assembler/input reader, Application coordinator
// and Tool Application adapter. The final Tool execution seam is a fixture
// because the Tool Owner production execution bridge is not public yet; this
// test therefore proves composition and no-Continuation boundaries, not a
// production/system G6A claim.
func TestSingleCallG6ACompositionV2(t *testing.T) {
	fixture := newAssemblerV2Fixture(t)
	now := fixture.now.Add(6 * time.Second)
	runtimeClosure := newG6ARuntimeClosureV2(t, fixture, now)
	resealAssemblerForG6AV2(t, fixture, runtimeClosure)

	request := fixture.assembleAndResetForInputCurrent(t)
	assembler := fixture.newAssembler(t)
	input, err := assembler.InspectSingleCallToolActionInputCurrentV2(context.Background(), request)
	mustG6AV2(t, err)
	fixture.sessionReads, fixture.factReads, fixture.modelReads, fixture.currentReads, fixture.authorityReads = 0, 0, 0, 0, 0
	fixture.clockIndex = 0
	fixture.clockValues = []time.Time{now, now, now}

	clock := &fixedG6AClockV2{now: now}
	binding := newG6ABindingProjectionV2(t, request, input, fixture.projection, runtimeClosure, now)
	bindingReader := &fixedG6ABindingReaderV2{value: binding, now: now}
	toolResult, association := newG6AToolResultV2(t, request, binding.CandidateClosure.Candidate, runtimeClosure.provider.Ref, now)
	settlements := &fixedG6ASettlementReaderV2{inspection: toolResult.Inspection, association: association}
	execution := &fixedG6AToolExecutionV2{result: toolResult}
	flow, err := tooladapter.NewToolOwnerSingleCallFlowV2(execution, clock)
	mustG6AV2(t, err)
	toolPort, err := tooladapter.NewSingleCallToolActionAdapterV2(bindingReader, flow, settlements, tooladapter.NewInMemoryApplicationResultStoreV2(), clock)
	mustG6AV2(t, err)
	coordinator, err := application.NewSingleCallToolActionCoordinatorV2(application.SingleCallToolActionCoordinatorConfigV2{
		Facts:       applicationfakes.NewSingleCallToolActionCoordinationStoreV2(),
		Tool:        toolPort,
		Inputs:      assembler,
		Settlements: settlements,
		Clock:       clock.Now,
	})
	mustG6AV2(t, err)

	got, err := coordinator.CoordinateSingleCallToolActionV2(context.Background(), request)
	mustG6AV2(t, err)
	if err = got.ValidateCurrentFor(request, now); err != nil {
		t.Fatal(err)
	}
	if got.Coordinate.Inspection.Digest != toolResult.Inspection.Digest || !runtimeports.SameOperationSettlementRefV4(got.Coordinate.Inspection.Settlement, toolResult.Inspection.Settlement) || !runtimeports.SameOperationSettlementEvidenceAssociationRefV4(got.Coordinate.Association, association.RefV4()) {
		t.Fatalf("G6A composition lost the exact V4 closure: inspection=%s/%s association=%+v/%+v", got.Coordinate.Inspection.Digest, toolResult.Inspection.Digest, got.Coordinate.Association, association.RefV4())
	}
	if got.Coordinate.ToolResult.ID != toolResult.ID || got.Coordinate.ToolResult.Digest != toolResult.Digest {
		t.Fatal("G6A composition changed the settled Tool result")
	}

	replayed, err := coordinator.CoordinateSingleCallToolActionV2(context.Background(), request)
	mustG6AV2(t, err)
	if replayed.Digest != got.Digest || execution.executeCalls.Load() != 1 {
		t.Fatalf("G6A replay was not inspect-only: digest=%s/%s executes=%d", replayed.Digest, got.Digest, execution.executeCalls.Load())
	}

	t.Run("expired binding stops before Tool execution", func(t *testing.T) {
		expiredNow := time.Unix(0, binding.ExpiresUnixNano)
		expiredClock := &fixedG6AClockV2{now: expiredNow}
		expiredExecution := &fixedG6AToolExecutionV2{result: toolResult}
		expiredFlow, buildErr := tooladapter.NewToolOwnerSingleCallFlowV2(expiredExecution, expiredClock)
		mustG6AV2(t, buildErr)
		expiredPort, buildErr := tooladapter.NewSingleCallToolActionAdapterV2(&fixedG6ABindingReaderV2{value: binding, now: expiredNow}, expiredFlow, settlements, tooladapter.NewInMemoryApplicationResultStoreV2(), expiredClock)
		mustG6AV2(t, buildErr)
		if _, callErr := expiredPort.ExecuteSingleCallToolActionV2(context.Background(), request); callErr == nil {
			t.Fatal("expired BindingV2 unexpectedly reached Tool execution")
		}
		if calls := expiredExecution.executeCalls.Load(); calls != 0 {
			t.Fatalf("expired BindingV2 executed Tool %d times", calls)
		}
	})

	t.Run("valid association splice is not persisted as Application result", func(t *testing.T) {
		driftAssociation, sealErr := runtimeports.SealOperationSettlementEvidenceAssociationV4(runtimeports.OperationSettlementEvidenceAssociationV4{ID: "settlement-association-tool-g6a-drift", Settlement: association.Settlement, Prepare: association.Prepare, Execute: association.Execute})
		mustG6AV2(t, sealErr)
		driftExecution := &fixedG6AToolExecutionV2{result: toolResult}
		driftFlow, buildErr := tooladapter.NewToolOwnerSingleCallFlowV2(driftExecution, clock)
		mustG6AV2(t, buildErr)
		driftResults := tooladapter.NewInMemoryApplicationResultStoreV2()
		driftPort, buildErr := tooladapter.NewSingleCallToolActionAdapterV2(bindingReader, driftFlow, &fixedG6ASettlementReaderV2{inspection: toolResult.Inspection, association: driftAssociation}, driftResults, clock)
		mustG6AV2(t, buildErr)
		if _, callErr := driftPort.ExecuteSingleCallToolActionV2(context.Background(), request); callErr == nil {
			t.Fatal("Association splice unexpectedly produced an Application result")
		}
		if calls := driftExecution.executeCalls.Load(); calls != 1 {
			t.Fatalf("Association splice should fail after one settled Tool execution, calls=%d", calls)
		}
		key, keyErr := applicationcontract.SealSingleCallToolActionInspectKeyV2(request)
		mustG6AV2(t, keyErr)
		if _, inspectErr := driftResults.InspectSingleCallApplicationResultRecordV2(context.Background(), key); inspectErr == nil || !core.HasCategory(inspectErr, core.ErrorNotFound) {
			t.Fatalf("Association splice polluted the Application result store: %v", inspectErr)
		}
	})
}

type fixedG6AClockV2 struct{ now time.Time }

func (c *fixedG6AClockV2) Now() time.Time { return c.now }

type g6aRuntimeClosureV2 struct {
	generation  runtimeports.GenerationCurrentProjectionV1
	association runtimeports.GenerationBindingAssociationFactV1
	route       runtimeports.ControlledOperationProviderRouteCurrentProjectionV2
	provider    runtimeports.ProviderBindingCurrentProjectionV2
}

func newG6ARuntimeClosureV2(t *testing.T, fixture *assemblerV2Fixture, now time.Time) g6aRuntimeClosureV2 {
	t.Helper()
	d := func(label string) core.Digest { return core.DigestBytes([]byte(label)) }
	provider := runtimeports.ProviderBindingRefV2{BindingSetID: "binding-set-g6a-system", BindingSetRevision: 1, ComponentID: "praxis.tool/engine", ManifestDigest: d("tool-provider-manifest-g6a-system"), ArtifactDigest: d("tool-provider-artifact-g6a-system"), Capability: runtimeports.CapabilityNameV2(runtimeports.OperationScopeEvidenceActionEffectKindV3)}
	generationRef := runtimeports.GenerationArtifactRefV1{ID: "generation-g6a-system", Revision: 1, Digest: d("generation-g6a-system"), InputDigest: d("generation-input-g6a-system"), ManifestDigest: d("generation-manifest-g6a-system"), GraphDigest: d("generation-graph-g6a-system"), CatalogDigest: d("generation-catalog-g6a-system")}
	declaration := runtimeports.ControlledOperationProviderRouteDeclarationRefV2{RouteID: "route-g6a-system", Revision: 1, PublisherComponentID: "praxis.harness/assembly", DeclarationDigest: d("route-declaration-g6a-system")}
	conformance := runtimeports.ControlledOperationProviderRouteConformanceRefV2{ConformanceID: "route-conformance-g6a-system", Revision: 1, DeclarationRef: declaration, ConformanceDigest: d("route-conformance-g6a-system")}
	role := func(component string, capability runtimeports.CapabilityNameV2) runtimeports.ProviderBindingRefV2 {
		return runtimeports.ProviderBindingRefV2{BindingSetID: provider.BindingSetID, BindingSetRevision: provider.BindingSetRevision, ComponentID: runtimeports.ComponentIDV2(component), ManifestDigest: d(component + "-manifest"), ArtifactDigest: d(component + "-artifact"), Capability: capability}
	}
	route, err := runtimeports.SealControlledOperationProviderRouteCurrentProjectionV2(runtimeports.ControlledOperationProviderRouteCurrentProjectionV2{
		Ref: runtimeports.ControlledOperationProviderRouteCurrentRefV2{Revision: 1}, DeclarationRef: declaration, ConformanceRef: conformance,
		Generation: generationRef, HandoffID: generationRef.ID + "/handoff", HandoffRevision: 1, HandoffDigest: d("handoff-g6a-system"),
		BindingSetID: provider.BindingSetID, BindingSetRevision: provider.BindingSetRevision, BindingSetDigest: d("binding-set-g6a-system"), BindingSetSemanticDigest: d("binding-semantic-g6a-system"), BindingSetCurrentnessDigest: d("binding-currentness-g6a-system"),
		ActiveRouteID: "active-route-g6a-system", ActiveRouteRevision: 1, ActiveRouteDigest: d("active-route-g6a-system"),
		ToolAdapterBinding: role("praxis.tool/adapter", runtimeports.ControlledOperationToolAdapterCapabilityV2), GatewayBinding: role("praxis.runtime/gateway", runtimeports.ControlledOperationGatewayCapabilityV2), ProviderTransportBinding: role("praxis.tool/transport", runtimeports.ControlledOperationProviderTransportCapabilityV2), PreparedReaderBinding: role("praxis.runtime/prepared-reader", runtimeports.ControlledOperationPreparedReaderCapabilityV2), BoundaryReaderBinding: role("praxis.runtime/boundary-reader", runtimeports.ControlledOperationBoundaryReaderCapabilityV2), ProviderInspectBinding: role("praxis.runtime/provider-inspect", runtimeports.ControlledOperationProviderInspectCapabilityV2), ProviderBinding: provider,
		CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(6 * time.Second).UnixNano(),
	})
	mustG6AV2(t, err)
	component := runtimeports.GenerationComponentManifestRefV1{ComponentID: provider.ComponentID, ManifestDigest: provider.ManifestDigest, ArtifactDigest: provider.ArtifactDigest}
	extension := runtimeports.GenerationGovernanceExtensionRefV1{Kind: "praxis.tool/g6a-extension", Contract: runtimeports.SchemaRefV2{Namespace: "praxis.tool", Name: "g6a-extension", Version: "1.0.0", MediaType: "application/json", ContentDigest: d("g6a-extension-schema")}, Digest: d("g6a-extension")}
	generation, err := runtimeports.SealGenerationCurrentProjectionV1(runtimeports.GenerationCurrentProjectionV1{Generation: generationRef, ComponentManifests: []runtimeports.GenerationComponentManifestRefV1{component}, Extension: extension, State: runtimeports.GenerationCurrentSealedV1, Current: true, Watermark: 1, ExpiresUnixNano: route.ExpiresUnixNano})
	mustG6AV2(t, err)
	bindingSet, err := runtimeports.SealGenerationBindingSetCurrentProjectionV1(runtimeports.GenerationBindingSetCurrentProjectionV1{BindingSetID: route.BindingSetID, BindingSetRevision: route.BindingSetRevision, BindingSetDigest: route.BindingSetDigest, BindingSetSemanticDigest: route.BindingSetSemanticDigest, PlanDigest: d("binding-plan-g6a-system"), GovernanceDigest: d("binding-governance-g6a-system"), ComponentManifestSetDigest: runtimeports.GenerationComponentManifestSetDigestV1(generation.ComponentManifests), CurrentnessDigest: route.BindingSetCurrentnessDigest, IssuedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: route.ExpiresUnixNano})
	mustG6AV2(t, err)
	scopeDigest, err := runtimeports.ExecutionScopeDigestV2(fixture.session.Run.Scope)
	mustG6AV2(t, err)
	activationOperation := runtimeports.OperationSubjectV3{Kind: runtimeports.OperationScopeActivationV3, ExecutionScope: fixture.session.Run.Scope, ExecutionScopeDigest: scopeDigest, ActivationAttemptID: "activation-g6a-system", SubjectRevision: 1, CurrentProjectionRef: "activation-current-g6a-system", CurrentProjectionDigest: d("activation-current-g6a-system"), CurrentProjectionRevision: 1}
	activationDigest, err := activationOperation.DigestV3()
	mustG6AV2(t, err)
	activation, err := runtimeports.SealGenerationActivationCurrentProjectionV1(runtimeports.GenerationActivationCurrentProjectionV1{Operation: activationOperation, OperationDigest: activationDigest, Active: true, Watermark: 1, CurrentnessDigest: d("activation-currentness-g6a-system"), ExpiresUnixNano: route.ExpiresUnixNano})
	mustG6AV2(t, err)
	candidate, err := runtimeports.SealGenerationBindingAssociationCandidateV1(runtimeports.GenerationBindingAssociationCandidateV1{AssociationID: "association-g6a-system", Generation: generation, Binding: bindingSet, Activation: activation, RequestedExpiresUnixNano: route.ExpiresUnixNano})
	mustG6AV2(t, err)
	association, err := runtimeports.SealGenerationBindingAssociationFactV1(runtimeports.GenerationBindingAssociationFactV1{ID: candidate.AssociationID, Revision: 1, State: runtimeports.GenerationBindingAssociationActiveV1, Candidate: candidate, CandidateDigest: candidate.Digest, CreatedUnixNano: now.Add(-time.Second).UnixNano(), UpdatedUnixNano: now.UnixNano(), ExpiresUnixNano: route.ExpiresUnixNano})
	mustG6AV2(t, err)
	providerCurrent, err := runtimeports.SealProviderBindingCurrentProjectionV2(runtimeports.ProviderBindingCurrentProjectionV2{ContractVersion: runtimeports.ProviderBindingCurrentnessContractVersionV2, Ref: provider, State: runtimeports.ProviderBindingCurrentActiveV2, BindingSetDigest: route.BindingSetDigest, BindingSetSemanticDigest: route.BindingSetSemanticDigest, BindingID: "provider-binding-g6a-system", BindingRevision: 1, GrantDigest: d("provider-grant-g6a-system"), IssuedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: route.ExpiresUnixNano})
	mustG6AV2(t, err)
	return g6aRuntimeClosureV2{generation: generation, association: association, route: route, provider: providerCurrent}
}

func resealAssemblerForG6AV2(t *testing.T, fixture *assemblerV2Fixture, runtimeClosure g6aRuntimeClosureV2) {
	t.Helper()
	pending := *fixture.session.PendingAction
	factRef, err := fixture.fact.RefV3()
	mustG6AV2(t, err)
	identityRef, err := fixture.fact.Identity.RefV1(fixture.fact.ContentDigest)
	mustG6AV2(t, err)
	ownerInputs, err := harnesscontract.SealCommittedPendingActionOwnerCurrentInputsV1(harnesscontract.CommittedPendingActionOwnerCurrentInputsV1{ModelTurnOperation: fixture.session.ApplicationBinding.OwnerCurrentInputs.ModelTurnOperation, GenerationBindingAssociation: runtimeClosure.association.RefV1(), RouteCurrent: runtimeClosure.route.Ref, RouteMatrix: runtimeports.OperationScopeEvidenceActionMatrixV3(), ContextApplicability: fixture.session.ApplicationBinding.OwnerCurrentInputs.ContextApplicability})
	mustG6AV2(t, err)
	binding, err := harnesscontract.SealPendingActionApplicationBindingV2(harnesscontract.PendingActionApplicationBindingV2{Base: harnesscontract.PendingActionApplicationBindingV1{PendingAction: pending, IdentityRef: identityRef, DomainResultFactRef: factRef, ModelTurnSettlementRef: *fixture.session.Execution.Settlement}, OwnerCurrentInputs: ownerInputs})
	mustG6AV2(t, err)
	session := fixture.session.Clone()
	session.ApplicationBinding = &binding
	session.Digest = ""
	session, err = harnesscontract.SealGovernedSessionV4(session)
	mustG6AV2(t, err)
	scopeDigest, err := runtimeports.ExecutionScopeDigestV2(session.Run.Scope)
	mustG6AV2(t, err)
	subject := harnesscontract.CommittedPendingActionSubjectV3{Base: harnesscontract.CommittedPendingActionSubjectV2{ExecutionScopeDigest: scopeDigest, Run: session.Run, SessionID: session.ID, SessionRevision: session.Revision, SessionDigest: session.Digest, Turn: session.Turn, PendingActionRef: pending.Ref, IdentityRef: identityRef, DomainResultFactRef: factRef, ModelTurnSettlement: *session.Execution.Settlement}, ApplicationBinding: binding}
	currentRequest := harnesscontract.CommittedPendingActionCurrentRequestV3{Subject: subject}
	current, err := harnesscontract.SealCommittedPendingActionCurrentV3(harnesscontract.CommittedPendingActionCurrentV3{Run: session.Run, ExecutionScopeDigest: scopeDigest, SessionID: session.ID, SessionRevision: session.Revision, SessionDigest: session.Digest, Phase: session.Phase, Turn: session.Turn, PendingAction: pending, ApplicationBinding: binding, CheckedUnixNano: fixture.now.UnixNano(), ExpiresUnixNano: fixture.now.Add(20 * time.Second).UnixNano()}, currentRequest, fixture.now)
	mustG6AV2(t, err)
	identityCoordinate, err := mapIdentityCoordinateV2(fixture.fact)
	mustG6AV2(t, err)
	pendingSubject, err := mapPendingSubjectV2(session, fixture.fact, identityCoordinate)
	mustG6AV2(t, err)
	action, err := applicationcontract.SealSingleCallActionCoordinateV2(applicationcontract.SingleCallActionCoordinateV2{ExecutionScope: session.Run.Scope, PendingSubject: pendingSubject})
	mustG6AV2(t, err)
	authority := fixture.authority
	authority.Scope, authority.ActionScopeDigest = session.Run.Scope, action.Digest
	mustG6AV2(t, authority.ValidateCurrent(fixture.input.Authority, action.ExecutionScope, action.Digest, fixture.now))
	fixture.session, fixture.current, fixture.authority = session, current, authority
	fixture.input = applicationcontract.AssembleSingleCallToolActionRequestV2{Action: action, Authority: fixture.input.Authority}
}

func newG6ABindingProjectionV2(t *testing.T, request applicationcontract.SingleCallToolActionRequestV2, input applicationcontract.SingleCallToolActionInputCurrentProjectionV2, model modelinvoker.ToolCallCandidateObservationProjectionV1, runtimeClosure g6aRuntimeClosureV2, now time.Time) tooladapter.SingleCallToolActionBindingCurrentProjectionV2 {
	t.Helper()
	d := func(label string) core.Digest { return core.DigestBytes([]byte(label)) }
	owner := core.OwnerRef{Domain: "praxis.tool", ID: "registry-owner-g6a"}
	inputSchema := request.Action.PendingSubject.Identity.PayloadSchema
	capability, err := toolcontract.SealCapability(toolcontract.CapabilityDescriptor{ID: "tool/capability-g6a", SemanticVersion: "1.0.0", Revision: 1, Owner: owner, InputSchema: inputSchema, OutputSchema: inputSchema, ActionScopeSchema: inputSchema, EffectKinds: []runtimeports.NamespacedNameV2{"praxis.tool/execute"}, Risk: toolcontract.RiskModerate, ReviewProfile: "review/g6a", AuthorityRequirement: "authority/g6a", BudgetRequirement: "budget/g6a", SandboxRequirement: "sandbox/g6a", EvidenceRequirement: "evidence/g6a", Compatibility: runtimeports.VersionRangeV2{MinimumInclusive: "1.0.0", MaximumExclusive: "2.0.0"}, CreatedUnixNano: now.Add(-time.Second).UnixNano()})
	mustG6AV2(t, err)
	capRef := toolcontract.ObjectRef{ID: string(capability.ID), Revision: capability.Revision, Digest: capability.Digest}
	tool, err := toolcontract.SealTool(toolcontract.ToolDescriptor{ID: "tool/lookup-g6a", SemanticVersion: "1.0.0", Revision: 1, Owner: owner, Capability: capRef, ArtifactDigest: runtimeClosure.provider.Ref.ArtifactDigest, Mechanism: toolcontract.MechanismLocal, InputSchema: inputSchema, OutputSchema: inputSchema, EffectKinds: []runtimeports.NamespacedNameV2{"praxis.tool/execute"}, TimeoutMillis: 5000, ConcurrencyLimit: 1, CancellationSupported: true, Idempotency: "tool/g6a", ConflictDomain: "tenant/{tenant}/g6a", ResultLimitBytes: 1 << 20, Conformance: "tool/g6a", CreatedUnixNano: now.Add(-time.Second).UnixNano()})
	mustG6AV2(t, err)
	toolRef := toolcontract.ObjectRef{ID: string(tool.ID), Revision: tool.Revision, Digest: tool.Digest}
	manifest, err := toolcontract.SealSurface(toolcontract.ToolSurfaceManifest{ID: "surface-g6a", Revision: 1, Owner: owner, ResolvedPlanDigest: d("surface-plan-g6a"), ProfileDigest: d("surface-profile-g6a"), CapabilityGrantDigest: d("surface-grant-g6a"), RegistrySnapshotDigest: d("surface-registry-g6a"), Entries: []toolcontract.ToolSurfaceEntry{{Capability: capRef, Tool: toolRef, ModelName: model.Observation.Calls[0].Name, InputSchema: inputSchema, DescriptionDigest: d("surface-description-g6a"), Visibility: toolcontract.SurfaceVisible, Allowed: true, Admission: toolcontract.AdmissionRequired, MechanismDigest: d("surface-mechanism-g6a"), EffectKinds: []runtimeports.NamespacedNameV2{"praxis.tool/execute"}}}, Dialect: "model/default", CreatedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(6 * time.Second).UnixNano()})
	mustG6AV2(t, err)
	surface, err := toolcontract.SealToolSurfaceManifestCurrentV1(toolcontract.ToolSurfaceManifestCurrentProjectionV1{Manifest: manifest, Owner: owner, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: manifest.ExpiresUnixNano})
	mustG6AV2(t, err)
	registry := runtimeports.RegistrySnapshotRefV1{Owner: owner, ContractVersion: "1.0.0", ID: "registry-snapshot-g6a", Revision: 1, Digest: manifest.RegistrySnapshotDigest}
	prepared, err := modelinvoker.SealPreparedModelInvocationFactV1(modelinvoker.PreparedModelInvocationFactV1{InvocationID: model.Ref.InvocationID, InvocationDigest: model.Ref.InvocationDigest, UnifiedRequestDigest: model.Ref.InvocationDigest, RequestToolsDigest: d("prepared-tools-g6a"), PreparedPlanDigest: d("prepared-plan-g6a"), RouteDigest: d("prepared-route-g6a"), ProfileDigest: manifest.ProfileDigest, ActualToolSurfaceDigest: manifest.ExpectedInjectionDigest, ActualProviderInjectionDigest: d("prepared-provider-g6a"), CapabilitySnapshotRef: modelinvoker.PreparedModelInvocationCapabilitySnapshotRefV1{ContractVersion: "1.0.0", ID: "capability-snapshot-g6a", Revision: 1, Digest: d("capability-snapshot-g6a")}, RegistrySnapshotRef: registry, CreatedUnixNano: now.Add(-time.Second).UnixNano(), NotAfterUnixNano: now.Add(6 * time.Second).UnixNano()})
	mustG6AV2(t, err)
	preparedCurrent, err := modelinvoker.SealPreparedModelInvocationCurrentV1(modelinvoker.PreparedModelInvocationCurrentProjectionV1{Prepared: prepared.Ref(), CapabilitySnapshotRef: prepared.CapabilitySnapshotRef, RegistrySnapshotRef: registry, ActualToolSurfaceDigest: prepared.ActualToolSurfaceDigest, ActualProviderInjectionDigest: prepared.ActualProviderInjectionDigest, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(6 * time.Second).UnixNano(), NotAfterUnixNano: prepared.NotAfterUnixNano})
	mustG6AV2(t, err)
	assembly, err := runtimeports.SealModelPreDispatchAssemblyCurrentProjectionV1(runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{Ref: runtimeports.ModelPreDispatchAssemblyCurrentRefV1{Revision: 1}, Generation: runtimeClosure.generation.Generation, Handoff: runtimeports.ModelPreDispatchAssemblyExactRefV1{ID: runtimeClosure.route.HandoffID, Revision: runtimeClosure.route.HandoffRevision, Digest: runtimeClosure.route.HandoffDigest}, BindingSet: runtimeports.ModelPreDispatchAssemblyBindingSetRefV1{ID: runtimeClosure.route.BindingSetID, Revision: runtimeClosure.route.BindingSetRevision, Digest: runtimeClosure.route.BindingSetDigest, SemanticDigest: runtimeClosure.route.BindingSetSemanticDigest, CurrentnessDigest: runtimeClosure.route.BindingSetCurrentnessDigest, ProjectionDigest: runtimeClosure.association.Candidate.Binding.ProjectionDigest, ExpiresUnixNano: runtimeClosure.route.ExpiresUnixNano}, Manifest: runtimeports.ModelPreDispatchAssemblyExactRefV1{ID: "assembly-manifest-g6a", Revision: 1, Digest: d("assembly-manifest-g6a")}, Conformance: runtimeports.ModelPreDispatchAssemblyExactRefV1{ID: "assembly-conformance-g6a", Revision: 1, Digest: d("assembly-conformance-g6a")}, ToolSurface: runtimeports.ModelPreDispatchAssemblyExactRefV1{ID: surface.Ref.ID, Revision: surface.Ref.Revision, Digest: surface.Ref.Digest}, ProfileDigest: prepared.ProfileDigest, RegistrySnapshot: registry, SemanticDigest: d("assembly-semantic-g6a"), CurrentnessDigest: d("assembly-currentness-g6a"), CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(6 * time.Second).UnixNano()})
	mustG6AV2(t, err)
	ensure := toolcontract.ToolSurfaceInvocationBindingEnsureRequestV1{Invocation: toolcontract.ToolSurfaceInvocationCoordinateV1{InvocationID: model.Ref.InvocationID, InvocationDigest: model.Ref.InvocationDigest}, PreparedFactRef: prepared.Ref(), PreparedHistoricalFact: prepared, PreparedCurrentRef: preparedCurrent.Ref(), PreparedCurrent: preparedCurrent, SurfaceCurrent: surface, AssemblyCurrentRef: assembly.Ref, AssemblyRegistrySnapshot: registry, AssemblyCurrent: assembly, RequestedNotAfterUnixNano: now.Add(5 * time.Second).UnixNano()}
	surfaceSubject, err := toolcontract.SealToolSurfaceInvocationBindingSubjectV1(ensure)
	mustG6AV2(t, err)
	surfaceBinding, err := toolcontract.SealToolSurfaceInvocationBindingV1(toolcontract.ToolSurfaceInvocationBindingV1{Ref: toolcontract.ToolSurfaceInvocationBindingRefV1{Owner: core.OwnerRef{Domain: "praxis.tool", ID: "surface-binding-owner-g6a"}}, Subject: surfaceSubject, CreatedUnixNano: now.UnixNano(), NotAfterUnixNano: ensure.RequestedNotAfterUnixNano})
	mustG6AV2(t, err)

	capCurrent := sealG6ARegistryCurrentV2(t, "capability", capRef, owner, now)
	toolCurrent := sealG6ARegistryCurrentV2(t, "tool", toolRef, owner, now)
	entry := surface.Manifest.Entries[0]
	surfaceObject := toolcontract.ObjectRef{ID: manifest.ID, Revision: manifest.Revision, Digest: manifest.Digest}
	limit, err := toolcontract.DeriveToolInputLimitPolicyV1(surfaceObject, entry.Order, entry, capRef, toolRef, inputSchema)
	mustG6AV2(t, err)
	expectedOwner := runtimeports.EffectOwnerRefV2{Role: runtimeports.OwnerSettlement, ComponentID: runtimeClosure.provider.Ref.ComponentID, ManifestDigest: runtimeClosure.provider.Ref.ManifestDigest}
	pending := toolcontract.PendingActionExactRefV2{ID: request.Action.PendingSubject.PendingActionRef, Revision: 1, RequestDigest: request.Action.PendingSubject.PendingActionDigest}
	inputSubject, err := toolcontract.SealToolInputContractBindingSubjectV1(toolcontract.ToolInputContractBindingSubjectV1{ApplicationRequestID: request.ID, ApplicationRequestRevision: request.Revision, ApplicationRequestDigest: request.Digest, PendingAction: pending, OperationScopeDigest: request.Action.ExecutionScopeDigest, ProviderBinding: runtimeClosure.provider.Ref, ExpectedOwner: expectedOwner, SurfaceOwner: owner, CapabilityRegistryOwner: owner, ToolRegistryOwner: owner, Surface: surfaceObject, SurfaceEntryOrdinal: entry.Order, SurfaceEntry: entry, Capability: capRef, Tool: toolRef, ToolArtifactDigest: tool.ArtifactDigest, InputSchema: inputSchema, LimitPolicy: limit})
	mustG6AV2(t, err)
	resolve := toolcontract.ToolInputContractResolveRequestV1{ApplicationRequestID: request.ID, ApplicationRequestRevision: request.Revision, ApplicationRequestDigest: request.Digest, PendingAction: pending, OperationScopeDigest: request.Action.ExecutionScopeDigest, ProviderBinding: runtimeClosure.provider.Ref, ExpectedOwner: expectedOwner, Surface: surfaceObject, CallName: entry.ModelName, Capability: capRef, Tool: toolRef, InputSchema: inputSchema, RequestedExpiresUnixNano: now.Add(5 * time.Second).UnixNano()}
	issuance, err := toolcontract.ToolInputContractIssuanceFromResolveRequestV1(resolve)
	mustG6AV2(t, err)
	schemaCurrent, err := toolcontract.SealToolInputSchemaCurrentRefV1(toolcontract.ToolInputSchemaCurrentRefV1{InputSchema: inputSchema, Authority: toolCurrent.Ref, RegistryOwner: owner, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(5 * time.Second).UnixNano()})
	mustG6AV2(t, err)
	inputContract, err := toolcontract.SealToolInputContractCurrentV1(toolcontract.ToolInputContractCurrentProjectionV1{IssuanceSubject: issuance, BindingSubject: inputSubject, SurfaceCurrent: surface, CapabilityCurrent: capCurrent, ToolCurrent: toolCurrent, InputSchemaCurrent: schemaCurrent, RequestedExpiresUnixNano: resolve.RequestedExpiresUnixNano, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(5 * time.Second).UnixNano()})
	mustG6AV2(t, err)
	source, err := toolcontract.SealModelSourceCandidateHistoricalRefV1(model)
	mustG6AV2(t, err)
	call := model.Observation.Calls[0]
	candidate, err := toolcontract.SealActionCandidateV3(toolcontract.ActionCandidateV3{TenantID: request.Action.ExecutionScope.Identity.TenantID, RunID: string(request.Action.PendingSubject.Run.RunID), SessionID: request.Action.PendingSubject.SessionID, TurnID: strconv.FormatUint(uint64(request.Action.PendingSubject.Turn), 10), PendingAction: pending, SourceCandidate: source, Surface: surfaceObject, Capability: capRef, Tool: toolRef, InputSchema: inputSchema, Payload: runtimeports.OpaquePayloadV2{Schema: inputSchema, ContentDigest: core.DigestBytes(call.CanonicalArguments), Length: uint64(len(call.CanonicalArguments)), Inline: append([]byte(nil), call.CanonicalArguments...), LimitPolicy: limit}, PayloadRevision: 1, LimitPolicy: limit, InputContractCurrentRef: inputContract.Ref, SurfaceCurrent: surface.Ref, CapabilityCurrent: capCurrent.Ref, ToolCurrent: toolCurrent.Ref, InputSchemaCurrent: schemaCurrent, OperationScopeDigest: request.Action.ExecutionScopeDigest, EffectKind: runtimeports.OperationScopeEvidenceActionEffectKindV3, ExpectedOwner: expectedOwner, ConflictDomain: tool.ConflictDomain, IdempotencyKey: request.ID, CreatedUnixNano: now.UnixNano(), RequestedExpiresUnixNano: now.Add(5 * time.Second).UnixNano()})
	mustG6AV2(t, err)
	closure, err := tooladapter.SealSingleCallToolActionCandidateClosureV2(tooladapter.SingleCallToolActionCandidateClosureV2{ApplicationInput: input, ModelProjection: model, SurfaceInvocationBinding: surfaceBinding, Association: runtimeClosure.association, Generation: runtimeClosure.generation, Route: runtimeClosure.route, ProviderCurrent: runtimeClosure.provider, SurfaceCurrent: surface, CapabilityCurrent: capCurrent, ToolCurrent: toolCurrent, InputContract: inputContract, Candidate: candidate})
	mustG6AV2(t, err)
	snapshot, err := tooladapter.SealSingleCallToolActionBindingS2SnapshotV2(tooladapter.SingleCallToolActionBindingS2SnapshotV2{ApplicationInput: input, SurfaceInvocationBinding: surfaceBinding, Association: runtimeClosure.association, Generation: runtimeClosure.generation, Route: runtimeClosure.route, ProviderCurrent: runtimeClosure.provider, SurfaceCurrent: surface, CapabilityCurrent: capCurrent, ToolCurrent: toolCurrent, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(5 * time.Second).UnixNano()})
	mustG6AV2(t, err)
	sourceDigest, err := request.Action.PendingSubject.DigestV2()
	mustG6AV2(t, err)
	bindingSubject, err := toolcontract.SealSingleCallToolActionBindingSubjectV2(toolcontract.SingleCallToolActionBindingSubjectV2{ApplicationRequestID: request.ID, ApplicationRequestRevision: request.Revision, ApplicationRequestDigest: request.Digest, PendingAction: pending, TenantID: request.Action.ExecutionScope.Identity.TenantID, RunID: string(request.Action.PendingSubject.Run.RunID), SessionID: request.Action.PendingSubject.SessionID, TurnID: strconv.FormatUint(uint64(request.Action.PendingSubject.Turn), 10), ActionCoordinateDigest: request.Action.Digest, ExecutionScope: request.Action.ExecutionScope, ExecutionScopeDigest: request.Action.ExecutionScopeDigest, SourceSubjectDigest: sourceDigest})
	mustG6AV2(t, err)
	bindingIssuance, err := toolcontract.SealSingleCallToolActionBindingIssuanceSubjectV2(toolcontract.SingleCallToolActionBindingIssuanceSubjectV2{BindingSubject: bindingSubject, RequestedExpiresUnixNano: request.ExpiresUnixNano})
	mustG6AV2(t, err)
	projection, err := tooladapter.SealSingleCallToolActionBindingCurrentProjectionV2(tooladapter.SingleCallToolActionBindingCurrentProjectionV2{IssuanceSubject: bindingIssuance, CandidateRef: candidate.ObjectRef(), InputContractCurrentRef: inputContract.Ref, CandidateClosure: closure, S2Snapshot: snapshot, RequestedExpiresUnixNano: request.ExpiresUnixNano, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(5 * time.Second).UnixNano()})
	mustG6AV2(t, err)
	return projection
}

func sealG6ARegistryCurrentV2(t *testing.T, kind string, object toolcontract.ObjectRef, owner core.OwnerRef, now time.Time) toolcontract.ToolRegistryObjectCurrentProjectionV1 {
	t.Helper()
	source, err := toolcontract.SealToolRegistryRecordSourceV1(toolcontract.ToolRegistryRecordSourceV1{Kind: kind, ID: object.ID, ObjectRevision: object.Revision, ObjectDigest: object.Digest, State: "active", RegistryRevision: 1, UpdatedUnixNano: now.Add(-time.Second).UnixNano()})
	mustG6AV2(t, err)
	projection, err := toolcontract.SealToolRegistryObjectCurrentProjectionV1(toolcontract.ToolRegistryObjectCurrentProjectionV1{Source: source, Object: object, RegistryOwner: owner, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(5 * time.Second).UnixNano()})
	mustG6AV2(t, err)
	return projection
}

type fixedG6ABindingReaderV2 struct {
	value tooladapter.SingleCallToolActionBindingCurrentProjectionV2
	now   time.Time
}

func (r *fixedG6ABindingReaderV2) ResolveSingleCallToolActionBindingCurrentV2(_ context.Context, request tooladapter.SingleCallToolActionBindingResolveRequestV2) (tooladapter.SingleCallToolActionBindingCurrentProjectionV2, error) {
	if err := r.value.ValidateAgainst(request, r.now); err != nil {
		return tooladapter.SingleCallToolActionBindingCurrentProjectionV2{}, err
	}
	return tooladapter.CloneSingleCallToolActionBindingCurrentProjectionV2(r.value), nil
}
func (r *fixedG6ABindingReaderV2) InspectSingleCallToolActionBindingCurrentByIssuanceV2(ctx context.Context, request tooladapter.SingleCallToolActionBindingIssuanceLookupRequestV2) (tooladapter.SingleCallToolActionBindingCurrentProjectionV2, error) {
	return r.ResolveSingleCallToolActionBindingCurrentV2(ctx, tooladapter.SingleCallToolActionBindingResolveRequestV2{ApplicationRequest: request.ApplicationRequest, SourceSubject: request.SourceSubject, RequestedExpiresUnixNano: request.RequestedExpiresUnixNano})
}
func (r *fixedG6ABindingReaderV2) InspectExactSingleCallToolActionBindingCurrentV2(ctx context.Context, request tooladapter.SingleCallToolActionBindingInspectExactRequestV2) (tooladapter.SingleCallToolActionBindingCurrentProjectionV2, error) {
	if request.Expected != r.value.Ref {
		return tooladapter.SingleCallToolActionBindingCurrentProjectionV2{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "G6A fixture binding ref drifted")
	}
	return r.ResolveSingleCallToolActionBindingCurrentV2(ctx, tooladapter.SingleCallToolActionBindingResolveRequestV2{ApplicationRequest: request.ApplicationRequest, SourceSubject: request.SourceSubject, RequestedExpiresUnixNano: request.RequestedExpiresUnixNano})
}

type fixedG6AToolExecutionV2 struct {
	result       toolcontract.ToolResultV2
	executeCalls atomic.Uint64
}

func (e *fixedG6AToolExecutionV2) ExecuteBoundSingleCallToolActionV2(context.Context, tooladapter.ToolOwnerSingleCallExecutionV2) (toolcontract.ToolResultV2, error) {
	e.executeCalls.Add(1)
	return e.result, nil
}
func (e *fixedG6AToolExecutionV2) InspectBoundSingleCallToolActionV2(context.Context, tooladapter.ToolOwnerSingleCallExecutionV2) (toolcontract.ToolResultV2, error) {
	return e.result, nil
}

type fixedG6ASettlementReaderV2 struct {
	inspection  runtimeports.OperationInspectionSettlementRefV4
	association runtimeports.OperationSettlementEvidenceAssociationV4
}

func (r *fixedG6ASettlementReaderV2) InspectCurrentOperationSettlementV4(_ context.Context, request runtimeports.InspectCurrentOperationSettlementRequestV4) (runtimeports.OperationInspectionSettlementRefV4, error) {
	if err := request.Validate(); err != nil {
		return runtimeports.OperationInspectionSettlementRefV4{}, err
	}
	if !runtimeports.SameOperationSubjectV3(request.Operation, r.inspection.DomainResult.Operation) || request.EffectID != r.inspection.DomainResult.EffectID {
		return runtimeports.OperationInspectionSettlementRefV4{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "G6A fixture settlement key drifted")
	}
	return r.inspection, nil
}
func (r *fixedG6ASettlementReaderV2) InspectOperationSettlementEvidenceAssociationV4(_ context.Context, operation runtimeports.OperationSubjectV3, expected runtimeports.OperationSettlementEvidenceAssociationRefV4) (runtimeports.OperationSettlementEvidenceAssociationV4, error) {
	if !runtimeports.SameOperationSubjectV3(operation, r.inspection.DomainResult.Operation) || !runtimeports.SameOperationSettlementEvidenceAssociationRefV4(expected, r.association.RefV4()) {
		return runtimeports.OperationSettlementEvidenceAssociationV4{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "G6A fixture association key drifted")
	}
	return r.association, nil
}

func newG6AToolResultV2(t *testing.T, request applicationcontract.SingleCallToolActionRequestV2, candidate toolcontract.ActionCandidateV3, provider runtimeports.ProviderBindingRefV2, now time.Time) (toolcontract.ToolResultV2, runtimeports.OperationSettlementEvidenceAssociationV4) {
	t.Helper()
	d := func(label string) core.Digest { return core.DigestBytes([]byte(label)) }
	operation := runtimeports.OperationSubjectV3{Kind: runtimeports.OperationScopeRunV3, ExecutionScope: request.Action.ExecutionScope, ExecutionScopeDigest: request.Action.ExecutionScopeDigest, RunID: request.Action.PendingSubject.Run.RunID, SubjectRevision: 1, CurrentProjectionRef: "tool-action-current-g6a", CurrentProjectionDigest: d("tool-action-current-g6a"), CurrentProjectionRevision: 1}
	operationDigest, err := operation.DigestV3()
	mustG6AV2(t, err)
	delegation := runtimeports.ExecutionDelegationRefV2{ID: "delegation-tool-g6a", Revision: 2, Digest: d("delegation-tool-g6a")}
	attempt := runtimeports.OperationDispatchAttemptRefV3{OperationDigest: operationDigest, EffectID: "effect-tool-g6a", IntentRevision: 1, IntentDigest: d("intent-tool-g6a"), PermitID: "permit-tool-g6a", PermitRevision: 1, PermitDigest: d("permit-tool-g6a"), AttemptID: "attempt-tool-g6a", Delegation: &delegation}
	expires := now.Add(5 * time.Second).UnixNano()
	phase := func(kind runtimeports.OperationDispatchEnforcementPhaseV4, journal core.Revision) runtimeports.OperationDispatchEnforcementPhaseRefV4 {
		value := runtimeports.OperationDispatchEnforcementPhaseRefV4{OperationDigest: operationDigest, EffectID: attempt.EffectID, PermitID: attempt.PermitID, PermitFactRevision: attempt.PermitRevision, PermitDigest: attempt.PermitDigest, AdmissionDigest: d("admission-tool-g6a"), ReviewAuthorization: runtimeports.OperationReviewAuthorizationRefV4{ID: "review-tool-g6a", Revision: 1, Digest: d("review-tool-g6a")}, AttemptID: attempt.AttemptID, SandboxAttempt: runtimeports.OperationDispatchSandboxFactRefV4{ID: attempt.AttemptID, Revision: 1, Digest: d("sandbox-tool-g6a"), ExpiresUnixNano: expires}, Phase: kind, ReceiptDigest: d("receipt-" + string(kind)), JournalRevision: journal, ValidatedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: expires}
		if kind == runtimeports.OperationDispatchEnforcementExecuteV4 {
			value.PrepareReceiptDigest, value.PreparedAttemptDigest = d("receipt-prepare"), d("prepared-tool-g6a")
		}
		mustG6AV2(t, value.Validate())
		return value
	}
	prepare, execute := phase(runtimeports.OperationDispatchEnforcementPrepareV4, 1), phase(runtimeports.OperationDispatchEnforcementExecuteV4, 2)
	makeEvidence := func(kind runtimeports.OperationDispatchEnforcementPhaseV4, enforcement runtimeports.OperationDispatchEnforcementPhaseRefV4, sequence uint64) runtimeports.OperationSettlementEvidenceBindingV4 {
		label := string(kind)
		record := runtimeports.OperationScopeEvidenceRecordRefV3{LedgerScopeDigest: d("ledger-" + label), Sequence: sequence, RecordDigest: d("record-" + label)}
		issued := runtimeports.OperationScopeEvidenceQualificationRefV3{ID: "qualification-" + label, Revision: 1, Digest: d("qualification-issued-" + label), ExpiresUnixNano: expires}
		final := issued
		final.Revision, final.Digest = 2, d("qualification-final-"+label)
		return runtimeports.OperationSettlementEvidenceBindingV4{Phase: kind, Consumption: runtimeports.OperationScopeEvidenceConsumptionRefV3{ID: "consumption-" + label, Revision: 1, Digest: d("consumption-" + label), Record: record}, IssuedQualification: issued, FinalQualification: final, Record: record, CandidateDigest: candidate.Digest, Handoff: runtimeports.OperationScopeEvidenceProviderHandoffRefV3{ID: "evidence-handoff-" + label, Revision: 1, Digest: d("evidence-handoff-" + label), ExpiresUnixNano: expires}, Attempt: attempt, EnforcementPhase: enforcement, OperationScopeDigest: d("operation-scope-" + label)}
	}
	evidence := []runtimeports.OperationSettlementEvidenceBindingV4{makeEvidence(runtimeports.OperationDispatchEnforcementPrepareV4, prepare, 1), makeEvidence(runtimeports.OperationDispatchEnforcementExecuteV4, execute, 2)}
	scopeSet, err := runtimeports.DigestOperationSettlementScopeSetV4(evidence)
	mustG6AV2(t, err)
	owner := candidate.ExpectedOwner
	schema := candidate.InputSchema
	domain := runtimeports.OperationSettlementDomainResultFactRefV4{Owner: provider, Kind: "praxis.tool/domain-result", ID: "tool-domain-result-g6a", Revision: 1, Digest: d("tool-domain-result-g6a"), TenantID: request.Action.ExecutionScope.Identity.TenantID, EffectID: attempt.EffectID, EffectRevision: 1, Operation: operation, OperationDigest: operationDigest, Attempt: attempt, Schema: schema, PayloadDigest: d("tool-payload-g6a"), PayloadRevision: 1, AuthoritativeTime: now.Add(-time.Second).UnixNano()}
	submission, err := runtimeports.SealOperationSettlementSubmissionV4(runtimeports.OperationSettlementSubmissionV4{ID: "runtime-settlement-tool-g6a", TenantID: domain.TenantID, Operation: operation, OperationDigest: operationDigest, OperationScopeDigest: scopeSet, EffectID: attempt.EffectID, ExpectedEffectRevision: 1, Owner: owner, DomainResult: domain, Evidence: evidence, IdempotencyKey: "settlement-tool-g6a", ConflictDomain: d("settlement-conflict-g6a"), SettledUnixNano: now.Add(-time.Second).UnixNano()})
	mustG6AV2(t, err)
	fact, err := runtimeports.SealOperationSettlementFactV4(runtimeports.OperationSettlementFactV4{Submission: submission})
	mustG6AV2(t, err)
	settlement := fact.RefV4()
	association, err := runtimeports.SealOperationSettlementEvidenceAssociationV4(runtimeports.OperationSettlementEvidenceAssociationV4{ID: "settlement-association-tool-g6a", Settlement: settlement, Prepare: evidence[0], Execute: evidence[1]})
	mustG6AV2(t, err)
	guard, err := runtimeports.SealOperationSettlementTerminalGuardV4(runtimeports.OperationSettlementTerminalGuardV4{ID: "settlement-guard-tool-g6a", TenantID: domain.TenantID, OperationDigest: operationDigest, EffectID: attempt.EffectID, Settlement: settlement})
	mustG6AV2(t, err)
	terminal, err := runtimeports.SealOperationSettlementTerminalProjectionV4(runtimeports.OperationSettlementTerminalProjectionV4{ID: "settlement-terminal-tool-g6a", TenantID: domain.TenantID, OperationDigest: operationDigest, EffectID: attempt.EffectID, Settlement: settlement, Association: association.RefV4(), Guard: guard.RefV4(), DomainResult: domain})
	mustG6AV2(t, err)
	inspection, err := runtimeports.SealOperationInspectionSettlementRefV4(runtimeports.OperationInspectionSettlementRefV4{Settlement: settlement, Association: association.RefV4(), Guard: guard.RefV4(), Projection: terminal.RefV4(), DomainResult: domain, EffectFactRevision: 2, Owner: owner, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires}, now)
	mustG6AV2(t, err)
	reservation := toolcontract.ObjectRef{ID: "tool-reservation-g6a", Revision: 1, Digest: d("tool-reservation-g6a")}
	domainObject := toolcontract.ObjectRef{ID: domain.ID, Revision: domain.Revision, Digest: domain.Digest}
	applyID, err := toolcontract.StableID("tool-apply-v2", candidate.ID, domain.ID, string(inspection.Digest))
	mustG6AV2(t, err)
	apply, err := toolcontract.SealToolApplySettlementFactV2(toolcontract.ToolApplySettlementFactV2{ID: applyID, TenantID: domain.TenantID, OperationScopeDigest: candidate.OperationScopeDigest, Action: candidate.ObjectRef(), Reservation: reservation, DomainResult: domainObject, Inspection: inspection, Outcome: toolcontract.ToolOutcomeSucceededV2, Disposition: toolcontract.ToolDispositionConfirmedAppliedV2, Owner: owner, AppliedUnixNano: now.UnixNano()})
	mustG6AV2(t, err)
	applyObject := toolcontract.ObjectRef{ID: apply.ID, Revision: apply.Revision, Digest: apply.Digest}
	resultID, err := toolcontract.StableID("tool-result-v2", candidate.ID, domain.ID, apply.ID, string(apply.Digest))
	mustG6AV2(t, err)
	result, err := toolcontract.SealToolResultV2(toolcontract.ToolResultV2{ID: resultID, Action: candidate.ObjectRef(), Reservation: reservation, DomainResult: domainObject, Apply: applyObject, Inspection: inspection, Outcome: toolcontract.ToolOutcomeSucceededV2, Disposition: toolcontract.ToolDispositionConfirmedAppliedV2, Schema: schema, PayloadDigest: domain.PayloadDigest, PayloadRevision: domain.PayloadRevision, FinalizedUnixNano: now.UnixNano()})
	mustG6AV2(t, err)
	return result, association
}

func mustG6AV2(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
