package applicationadapter

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/owner/surfacebinding"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/surface"
)

func TestBindingCurrentReaderV2DurableRootAndExactInspect(t *testing.T) {
	fixture := newBindingV2Fixture(t)
	winner, err := fixture.reader.ResolveSingleCallToolActionBindingCurrentV2(context.Background(), fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	if err = winner.ValidateAgainst(fixture.request, fixture.now); err != nil {
		t.Fatal(err)
	}
	if winner.CandidateRef != winner.CandidateClosure.Candidate.ObjectRef() || winner.InputContractCurrentRef != winner.CandidateClosure.InputContract.Ref {
		t.Fatal("BindingV2 root did not persist CandidateV3 and Input Contract exact refs")
	}
	lookup := SingleCallToolActionBindingIssuanceLookupRequestV2{ApplicationRequest: fixture.request.ApplicationRequest, SourceSubject: fixture.request.SourceSubject, RequestedExpiresUnixNano: fixture.request.RequestedExpiresUnixNano}
	byIssuance, err := fixture.reader.InspectSingleCallToolActionBindingCurrentByIssuanceV2(context.Background(), lookup)
	if err != nil || byIssuance.Ref != winner.Ref {
		t.Fatalf("BindingV2 issuance recovery failed: %v", err)
	}
	exact, err := fixture.reader.InspectExactSingleCallToolActionBindingCurrentV2(context.Background(), SingleCallToolActionBindingInspectExactRequestV2{ApplicationRequest: fixture.request.ApplicationRequest, SourceSubject: fixture.request.SourceSubject, RequestedExpiresUnixNano: fixture.request.RequestedExpiresUnixNano, Expected: winner.Ref})
	if err != nil || exact.ProjectionDigest != winner.ProjectionDigest {
		t.Fatalf("BindingV2 exact recovery failed: %v", err)
	}

	attack := fixture.request
	attack.SourceSubject.PendingActionDigest = testkit.Digest("pending-splice")
	if _, err = fixture.reader.ResolveSingleCallToolActionBindingCurrentV2(context.Background(), attack); err == nil {
		t.Fatal("PendingAction splice was accepted")
	}
}

func TestBindingCurrentReaderV2ConcurrentSameIssuanceSingleRoot(t *testing.T) {
	fixture := newBindingV2Fixture(t)
	const workers = 64
	var wg sync.WaitGroup
	refs := make(chan toolcontract.SingleCallToolActionBindingCurrentRefV2, workers)
	errs := make(chan error, workers)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			winner, err := fixture.reader.ResolveSingleCallToolActionBindingCurrentV2(context.Background(), fixture.request)
			if err == nil {
				refs <- winner.Ref
			}
			errs <- err
		}()
	}
	wg.Wait()
	close(refs)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var winner toolcontract.SingleCallToolActionBindingCurrentRefV2
	for ref := range refs {
		if winner == (toolcontract.SingleCallToolActionBindingCurrentRefV2{}) {
			winner = ref
		} else if ref != winner {
			t.Fatalf("same issuance returned multiple BindingV2 roots: %#v != %#v", ref, winner)
		}
	}
}

func TestBindingCurrentReaderV2LostCreateReplyInspectOnly(t *testing.T) {
	fixture := newBindingV2Fixture(t)
	fault := &lostReplyBindingStoreV2{inner: NewInMemorySingleCallToolActionBindingLeaseStoreV2()}
	fixture.rebuildReader(t, fault)
	winner, err := fixture.reader.ResolveSingleCallToolActionBindingCurrentV2(context.Background(), fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	if winner.Ref.ID == "" || fault.creates.Load() != 1 || fault.inspects.Load() == 0 {
		t.Fatalf("lost reply did not recover one durable root: creates=%d inspects=%d", fault.creates.Load(), fault.inspects.Load())
	}
}

func TestBindingCurrentReaderV2TypedNilCanceledAndExpiry(t *testing.T) {
	fixture := newBindingV2Fixture(t)
	if _, err := fixture.reader.ResolveSingleCallToolActionBindingCurrentV2(nil, fixture.request); err == nil {
		t.Fatal("nil context was accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := fixture.reader.ResolveSingleCallToolActionBindingCurrentV2(ctx, fixture.request); err != context.Canceled {
		t.Fatalf("canceled context was not preserved: %v", err)
	}
	winner, err := fixture.reader.ResolveSingleCallToolActionBindingCurrentV2(context.Background(), fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	fixture.clock.Set(time.Unix(0, winner.ExpiresUnixNano))
	if _, err = fixture.reader.InspectExactSingleCallToolActionBindingCurrentV2(context.Background(), SingleCallToolActionBindingInspectExactRequestV2{ApplicationRequest: fixture.request.ApplicationRequest, SourceSubject: fixture.request.SourceSubject, RequestedExpiresUnixNano: fixture.request.RequestedExpiresUnixNano, Expected: winner.Ref}); err == nil || !core.HasCategory(err, core.ErrorPreconditionFailed) {
		t.Fatalf("expired BindingV2 root remained current: %v", err)
	}
}

type bindingV2Fixture struct {
	now            time.Time
	clock          *testkit.ManualClock
	request        SingleCallToolActionBindingResolveRequestV2
	input          *inputReaderV2
	model          *modelReaderV2
	surfaceBinding *surfacebinding.InMemoryRepositoryV1
	surface        *surface.InMemoryToolSurfaceManifestCurrentRepositoryV1
	registry       *RegistryObjectCurrentReaderV1
	inputContract  *ToolInputContractCurrentResolverV1
	candidate      *CandidateBuilderV3
	association    *associationReaderV2
	generation     *generationReaderV2
	route          *routeReaderV2
	provider       *providerReaderV2
	reader         *BindingCurrentReaderV2
}

func newBindingV2Fixture(t *testing.T) *bindingV2Fixture {
	t.Helper()
	now := testkit.FixedTime
	clock := testkit.NewManualClock(now)
	modelProjection := testkit.ModelProjection(1)
	surfaceRepository, err := surface.NewInMemoryToolSurfaceManifestCurrentRepositoryV1(clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	surfaceCurrent, err := surfaceRepository.EnsureExactToolSurfaceManifestCurrentV1(context.Background(), testkit.ToolSurfaceManifestCurrentRequestV1(1))
	if err != nil {
		t.Fatal(err)
	}
	prepared := testkit.PreparedSurfaceInvocationFactV1()
	prepared.ID, prepared.Digest = "", ""
	prepared.InvocationID, prepared.InvocationDigest, prepared.UnifiedRequestDigest = modelProjection.Ref.InvocationID, modelProjection.Ref.InvocationDigest, modelProjection.Ref.InvocationDigest
	prepared, err = modelinvoker.SealPreparedModelInvocationFactV1(prepared)
	if err != nil {
		t.Fatal(err)
	}
	preparedCurrent := testkit.PreparedSurfaceInvocationCurrentV1(prepared)
	assembly := testkit.ModelPreDispatchAssemblyCurrentV1(surfaceCurrent, prepared)
	surfaceBindingRepo, err := surfacebinding.NewInMemoryRepositoryV1(testkit.Owner(), clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = surfaceBindingRepo.EnsureToolSurfaceInvocationBindingV1(context.Background(), toolcontract.ToolSurfaceInvocationBindingEnsureRequestV1{
		Invocation:      toolcontract.ToolSurfaceInvocationCoordinateV1{InvocationID: modelProjection.Ref.InvocationID, InvocationDigest: modelProjection.Ref.InvocationDigest},
		PreparedFactRef: prepared.Ref(), PreparedHistoricalFact: prepared, PreparedCurrentRef: preparedCurrent.Ref(), PreparedCurrent: preparedCurrent,
		SurfaceCurrent: surfaceCurrent, AssemblyCurrentRef: assembly.Ref, AssemblyRegistrySnapshot: prepared.RegistrySnapshotRef, AssemblyCurrent: assembly,
		RequestedNotAfterUnixNano: now.Add(10 * time.Minute).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	registryReader, _, _, tool := registryCurrentFixtureV1(t)
	provider := runtimeports.ProviderBindingRefV2{BindingSetID: "binding-set-v2", BindingSetRevision: 1, ComponentID: testkit.SettlementOwner().ComponentID, ManifestDigest: testkit.SettlementOwner().ManifestDigest, ArtifactDigest: tool.ArtifactDigest, Capability: runtimeports.CapabilityNameV2(runtimeports.OperationScopeEvidenceActionEffectKindV3)}
	route := testkit.ControlledProviderRouteV2(now, provider)
	generation, association := generationAssociationFixtureV2(t, now, route)
	providerCurrent, err := runtimeports.SealProviderBindingCurrentProjectionV2(runtimeports.ProviderBindingCurrentProjectionV2{
		ContractVersion: runtimeports.ProviderBindingCurrentnessContractVersionV2, Ref: provider, State: runtimeports.ProviderBindingCurrentActiveV2,
		BindingSetDigest: route.BindingSetDigest, BindingSetSemanticDigest: route.BindingSetSemanticDigest, BindingID: "provider-binding-v2", BindingRevision: 1,
		GrantDigest: testkit.Digest("provider-grant-v2"), IssuedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(5 * time.Second).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	applicationRequest, inputProjection := applicationV2Fixture(t, now, modelProjection, provider, association.RefV1(), route.Ref)
	inputReader := &inputReaderV2{projection: inputProjection}
	modelReader := &modelReaderV2{projection: modelProjection}
	inputStore := NewInMemoryToolInputContractLeaseStoreV1()
	inputResolver, err := NewToolInputContractCurrentResolverV1(surfaceRepository, registryReader, inputStore, clock)
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := NewCandidateBuilderV3(registryReader, clock)
	if err != nil {
		t.Fatal(err)
	}
	fixture := &bindingV2Fixture{now: now, clock: clock, request: SingleCallToolActionBindingResolveRequestV2{ApplicationRequest: applicationRequest, SourceSubject: applicationRequest.Action.PendingSubject, RequestedExpiresUnixNano: now.Add(4 * time.Second).UnixNano()}, input: inputReader, model: modelReader, surfaceBinding: surfaceBindingRepo, surface: surfaceRepository, registry: registryReader, inputContract: inputResolver, candidate: candidate, association: &associationReaderV2{fact: association}, generation: &generationReaderV2{projection: generation}, route: &routeReaderV2{projection: route}, provider: &providerReaderV2{projection: providerCurrent}}
	fixture.rebuildReader(t, NewInMemorySingleCallToolActionBindingLeaseStoreV2())
	return fixture
}

func (f *bindingV2Fixture) rebuildReader(t *testing.T, store SingleCallToolActionBindingLeaseStoreV2) {
	t.Helper()
	reader, err := NewBindingCurrentReaderV2(f.input, f.model, f.surfaceBinding, f.surface, f.registry, f.inputContract, f.candidate, f.association, f.generation, f.route, f.provider, store, f.clock)
	if err != nil {
		t.Fatal(err)
	}
	f.reader = reader
}

func generationAssociationFixtureV2(t *testing.T, now time.Time, route runtimeports.ControlledOperationProviderRouteCurrentProjectionV2) (runtimeports.GenerationCurrentProjectionV1, runtimeports.GenerationBindingAssociationFactV1) {
	t.Helper()
	component := runtimeports.GenerationComponentManifestRefV1{ComponentID: route.ProviderBinding.ComponentID, ManifestDigest: route.ProviderBinding.ManifestDigest, ArtifactDigest: route.ProviderBinding.ArtifactDigest}
	extension := runtimeports.GenerationGovernanceExtensionRefV1{Kind: "praxis.tool/test-extension", Contract: testkit.Schema("generation-extension"), Digest: testkit.Digest("generation-extension")}
	generation, err := runtimeports.SealGenerationCurrentProjectionV1(runtimeports.GenerationCurrentProjectionV1{Generation: route.Generation, ComponentManifests: []runtimeports.GenerationComponentManifestRefV1{component}, Extension: extension, State: runtimeports.GenerationCurrentSealedV1, Current: true, Watermark: 1, ExpiresUnixNano: now.Add(5 * time.Second).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	binding, err := runtimeports.SealGenerationBindingSetCurrentProjectionV1(runtimeports.GenerationBindingSetCurrentProjectionV1{
		BindingSetID: route.BindingSetID, BindingSetRevision: route.BindingSetRevision, BindingSetDigest: route.BindingSetDigest, BindingSetSemanticDigest: route.BindingSetSemanticDigest,
		PlanDigest: testkit.Digest("binding-plan-v2"), GovernanceDigest: testkit.Digest("binding-governance-v2"), ComponentManifestSetDigest: runtimeports.GenerationComponentManifestSetDigestV1(generation.ComponentManifests),
		CurrentnessDigest: route.BindingSetCurrentnessDigest, IssuedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(5 * time.Second).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	boundary := testkit.BoundaryFixture(now)
	activationOperation := boundary.Operation
	activationOperation.Kind, activationOperation.RunID, activationOperation.ActivationAttemptID = runtimeports.OperationScopeActivationV3, "", "activation-attempt-v2"
	activationOperation.CurrentProjectionRef = "activation-current-v2"
	activationOperation.CurrentProjectionDigest = testkit.Digest("activation-current-v2")
	activationDigest, err := activationOperation.DigestV3()
	if err != nil {
		t.Fatal(err)
	}
	activation, err := runtimeports.SealGenerationActivationCurrentProjectionV1(runtimeports.GenerationActivationCurrentProjectionV1{Operation: activationOperation, OperationDigest: activationDigest, Active: true, Watermark: 1, CurrentnessDigest: testkit.Digest("activation-currentness-v2"), ExpiresUnixNano: now.Add(5 * time.Second).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := runtimeports.SealGenerationBindingAssociationCandidateV1(runtimeports.GenerationBindingAssociationCandidateV1{AssociationID: "gen-assoc-v2", Generation: generation, Binding: binding, Activation: activation, RequestedExpiresUnixNano: now.Add(5 * time.Second).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	fact, err := runtimeports.SealGenerationBindingAssociationFactV1(runtimeports.GenerationBindingAssociationFactV1{ID: candidate.AssociationID, Revision: 1, State: runtimeports.GenerationBindingAssociationActiveV1, Candidate: candidate, CandidateDigest: candidate.Digest, CreatedUnixNano: now.Add(-time.Second).UnixNano(), UpdatedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(5 * time.Second).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	return generation, fact
}

func applicationV2Fixture(t *testing.T, now time.Time, model modelinvoker.ToolCallCandidateObservationProjectionV1, provider runtimeports.ProviderBindingRefV2, association runtimeports.GenerationBindingAssociationRefV1, route runtimeports.ControlledOperationProviderRouteCurrentRefV2) (applicationcontract.SingleCallToolActionRequestV2, applicationcontract.SingleCallToolActionInputCurrentProjectionV2) {
	t.Helper()
	d := testkit.Digest
	boundary := testkit.BoundaryFixture(now)
	operation := boundary.Operation
	args := append([]byte(nil), model.Observation.Calls[0].CanonicalArguments...)
	argsDigest := core.DigestBytes(args)
	schema := testkit.Schema("input")
	domainSchema := testkit.Schema("model-domain-result")
	domainDigest := d("model-domain-result")
	pending := applicationcontract.SingleCallPendingActionCoordinateV1{ActionRef: "pending-action-v2", RequestDigest: d("pending-request-v2"), Capability: runtimeports.CapabilityNameV2(runtimeports.OperationScopeEvidenceActionEffectKindV3), PayloadSchema: schema, PayloadDigest: argsDigest, SourceCandidateID: "source-candidate-v2", SourceCandidateRevision: 1, SourceCandidateDigest: d("source-candidate-v2"), ProjectionDigest: model.Ref.Digest}
	identity, err := applicationcontract.SealSingleCallModelPendingActionIdentityCoordinateV2(applicationcontract.SingleCallModelPendingActionIdentityCoordinateV2{
		IdentityContractVersion: applicationcontract.SingleCallIdentityContractVersionV1, IdentityID: "identity-v2", IdentityRevision: 1, IdentityDigest: d("identity-owner-v2"), CreatedUnixNano: now.Add(-time.Second).UnixNano(),
		ModelProjectionID: model.Ref.ID, ModelProjectionRevision: model.Ref.Revision, ModelProjectionDigest: model.Ref.Digest, ModelInvocationID: model.Ref.InvocationID, ModelInvocationDigest: model.Ref.InvocationDigest,
		ModelObservationDigest: model.Ref.ObservationDigest, ModelSourceResponseID: model.Ref.Source.ResponseID, ModelSourceSequence: model.Ref.Source.SourceSequence,
		SourceKeyDigest: d("source-key-v2"), SourceExecutionScopeDigest: operation.ExecutionScopeDigest, SourceRunID: string(operation.RunID), SourceSessionID: "session-v2", SourceTurn: 1,
		CallOrdinalEncodingVersion: applicationcontract.SingleCallCallOrdinalEncodingVersionV1, CallOrdinalPresent: true, CallOrdinalValue: 0,
		SettlementOwner: provider, CallID: model.Observation.Calls[0].CallID, CallName: model.Observation.Calls[0].Name, CanonicalArgumentsDigest: argsDigest,
		PendingActionRef: pending.ActionRef, PendingActionRequestDigest: pending.RequestDigest, PayloadSchema: schema, PayloadContentDigest: argsDigest, Capability: pending.Capability,
		SourceCandidateID: pending.SourceCandidateID, SourceCandidateRevision: pending.SourceCandidateRevision, SourceCandidateDigest: pending.SourceCandidateDigest, DomainResultDigest: domainDigest,
		NotAfterUnixNano: now.Add(8 * time.Second).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	idRef := applicationcontract.SingleCallModelPendingActionIdentityRefCoordinateV2{ID: identity.IdentityID, Revision: identity.IdentityRevision, Digest: identity.IdentityDigest, ModelProjectionID: identity.ModelProjectionID, ModelProjectionRevision: identity.ModelProjectionRevision, ModelProjectionDigest: identity.ModelProjectionDigest, PendingActionRef: identity.PendingActionRef, PendingActionRequestDigest: identity.PendingActionRequestDigest, DomainResultDigest: identity.DomainResultDigest, SourceKeyDigest: identity.SourceKeyDigest}
	factRef := applicationcontract.SingleCallSettledTurnDomainResultFactRefCoordinateV2{FactID: "domain-fact-v2", Revision: 1, FactDigest: d("domain-fact-v2"), SourceKeyDigest: identity.SourceKeyDigest, Schema: domainSchema, ContentDigest: domainDigest, IdentityRef: idRef}
	owner := runtimeports.EffectOwnerRefV2{Role: runtimeports.OwnerSettlement, ComponentID: provider.ComponentID, ManifestDigest: provider.ManifestDigest}
	settlement := runtimeports.OperationSettlementRefV3{ID: "model-settlement-v2", Revision: 1, Digest: d("model-settlement-v2"), Attempt: boundary.Attempt, Disposition: runtimeports.OperationSettlementAppliedV3, Owner: owner, Evidence: []runtimeports.EvidenceRecordRefV2{{LedgerScopeDigest: d("ledger-v2"), Sequence: 1, RecordDigest: d("record-v2")}}, DomainResultSchema: &domainSchema, DomainResultDigest: domainDigest}
	base, err := applicationcontract.SealSingleCallHarnessBaseBindingCoordinateV2(applicationcontract.SingleCallHarnessBaseBindingCoordinateV2{PendingAction: pending, IdentityRef: idRef, DomainResultFact: factRef, ModelTurnSettlement: settlement})
	if err != nil {
		t.Fatal(err)
	}
	ownerInputs, err := applicationcontract.SealSingleCallHarnessOwnerCurrentInputsCoordinateV2(applicationcontract.SingleCallHarnessOwnerCurrentInputsCoordinateV2{HarnessContractVersion: applicationcontract.SingleCallHarnessOwnerCurrentInputsVersionV1, ModelTurnOperation: operation, GenerationBindingAssociation: association, RouteCurrent: route, RouteMatrix: runtimeports.OperationScopeEvidenceActionMatrixV3(), ContextApplicability: runtimeports.OperationScopeEvidenceApplicabilityFactRefV3{Kind: runtimeports.OperationScopeEvidenceContextParentKindV3, ID: "context-v2", Revision: 1, Digest: d("context-v2")}, HarnessDigest: d("harness-v2")})
	if err != nil {
		t.Fatal(err)
	}
	binding, err := applicationcontract.SealSingleCallHarnessApplicationBindingCoordinateV2(applicationcontract.SingleCallHarnessApplicationBindingCoordinateV2{BindingVersion: applicationcontract.SingleCallHarnessBindingContractVersionV2, Base: base, OwnerInputs: ownerInputs, HarnessBindingDigest: d("binding-owner-v2")})
	if err != nil {
		t.Fatal(err)
	}
	run, err := applicationcontract.SealSingleCallRunSubjectCoordinateV2(applicationcontract.SingleCallRunSubjectCoordinateV2{ExecutionScope: operation.ExecutionScope, RunID: operation.RunID})
	if err != nil {
		t.Fatal(err)
	}
	subject, err := applicationcontract.SealSingleCallPendingActionSubjectCoordinateV2(applicationcontract.SingleCallPendingActionSubjectCoordinateV2{Run: run, SessionID: "session-v2", SessionRevision: 1, SessionDigest: d("session-v2"), Turn: 1, PendingActionRef: pending.ActionRef, PendingActionDigest: pending.RequestDigest, Binding: binding, Identity: identity})
	if err != nil {
		t.Fatal(err)
	}
	action, err := applicationcontract.SealSingleCallActionCoordinateV2(applicationcontract.SingleCallActionCoordinateV2{ExecutionScope: operation.ExecutionScope, PendingSubject: subject})
	if err != nil {
		t.Fatal(err)
	}
	authority := runtimeports.AuthorityBindingRefV2{Ref: "authority-v2", Revision: 1, Digest: d("authority-v2"), Epoch: operation.ExecutionScope.AuthorityEpoch}
	request, err := applicationcontract.SealSingleCallToolActionRequestV2(applicationcontract.SingleCallToolActionRequestV2{Action: action, Authority: authority, CreatedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(5 * time.Second).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	proof, err := applicationcontract.SealSingleCallModelToolCallProjectionProofV2(applicationcontract.SingleCallModelToolCallProjectionProofV2{ProjectionContractVersion: applicationcontract.SingleCallModelProjectionContractVersionV1, ProjectionID: model.Ref.ID, ProjectionRevision: model.Ref.Revision, ProjectionDigest: model.Ref.Digest, InvocationID: model.Ref.InvocationID, InvocationDigest: model.Ref.InvocationDigest, ObservationDigest: model.Ref.ObservationDigest, SourceResponseID: model.Ref.Source.ResponseID, SourceSequence: model.Ref.Source.SourceSequence, CallID: identity.CallID, CallName: identity.CallName, CanonicalArguments: args})
	if err != nil {
		t.Fatal(err)
	}
	identityCurrentRequest, err := applicationcontract.SealSingleCallModelPendingActionIdentityCurrentRequestV2(applicationcontract.SingleCallModelPendingActionIdentityCurrentRequestV2{Run: run, SessionID: subject.SessionID, Turn: subject.Turn, IdentityRef: idRef, DomainResultFact: factRef, RequestedNotAfterUnixNano: request.ExpiresUnixNano})
	if err != nil {
		t.Fatal(err)
	}
	identityCurrent, err := applicationcontract.SealSingleCallModelPendingActionIdentityCurrentV2(applicationcontract.SingleCallModelPendingActionIdentityCurrentV2{IdentityRef: idRef, DomainResultFact: factRef, Identity: identity, Projection: proof, CheckedUnixNano: now.Add(-time.Millisecond).UnixNano(), ExpiresUnixNano: now.Add(5 * time.Second).UnixNano()}, identityCurrentRequest, now)
	if err != nil {
		t.Fatal(err)
	}
	harnessCurrent, err := applicationcontract.SealSingleCallHarnessOwnerCurrentProofV3(applicationcontract.SingleCallHarnessOwnerCurrentProofV3{Subject: subject, Binding: binding, HarnessCurrentContractVersion: applicationcontract.SingleCallHarnessCurrentContractVersionV3, HarnessCurrentDigest: d("harness-current-v2"), IdentityCurrent: identityCurrent, CheckedUnixNano: now.Add(-time.Millisecond).UnixNano(), ExpiresUnixNano: now.Add(5 * time.Second).UnixNano()}, now)
	if err != nil {
		t.Fatal(err)
	}
	authorityCurrent, err := applicationcontract.SealSingleCallAuthorityCurrentProofV2(applicationcontract.SingleCallAuthorityCurrentProofV2{Ref: authority, ExecutionScopeDigest: action.ExecutionScopeDigest, ActionCoordinateDigest: action.Digest, FactRevision: 1, FactDigest: d("authority-fact-v2"), CheckedUnixNano: now.Add(-time.Millisecond).UnixNano(), ExpiresUnixNano: now.Add(5 * time.Second).UnixNano()}, request, now)
	if err != nil {
		t.Fatal(err)
	}
	input, err := applicationcontract.SealSingleCallToolActionInputCurrentProjectionV2(applicationcontract.SingleCallToolActionInputCurrentProjectionV2{HarnessCurrent: harnessCurrent, AuthorityCurrent: authorityCurrent, CheckedUnixNano: now.Add(-time.Millisecond).UnixNano(), ExpiresUnixNano: now.Add(5 * time.Second).UnixNano()}, request, now)
	if err != nil {
		t.Fatal(err)
	}
	return request, input
}

type inputReaderV2 struct {
	projection applicationcontract.SingleCallToolActionInputCurrentProjectionV2
}

func (r *inputReaderV2) InspectSingleCallToolActionInputCurrentV2(context.Context, applicationcontract.SingleCallToolActionRequestV2) (applicationcontract.SingleCallToolActionInputCurrentProjectionV2, error) {
	return applicationcontract.CloneSingleCallToolActionInputCurrentProjectionV2(r.projection), nil
}

type modelReaderV2 struct {
	projection modelinvoker.ToolCallCandidateObservationProjectionV1
}

func (r *modelReaderV2) InspectExactProjectionV1(_ context.Context, exact modelinvoker.ToolCallCandidateObservationRefV1) (modelinvoker.ToolCallCandidateObservationProjectionV1, error) {
	if r.projection.Ref != exact {
		return modelinvoker.ToolCallCandidateObservationProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Model exact Ref drifted")
	}
	return r.projection.Clone(), nil
}

type associationReaderV2 struct {
	fact runtimeports.GenerationBindingAssociationFactV1
}

func (r *associationReaderV2) InspectCurrentGenerationBindingAssociationV1(context.Context, string) (runtimeports.GenerationBindingAssociationFactV1, error) {
	return cloneAssociationV1(r.fact), nil
}

type generationReaderV2 struct {
	projection runtimeports.GenerationCurrentProjectionV1
}

func (r *generationReaderV2) InspectGenerationCurrentV1(context.Context, runtimeports.GenerationArtifactRefV1) (runtimeports.GenerationCurrentProjectionV1, error) {
	return cloneGenerationV1(r.projection), nil
}

type routeReaderV2 struct {
	projection runtimeports.ControlledOperationProviderRouteCurrentProjectionV2
}

func (r *routeReaderV2) InspectCurrentControlledOperationProviderRouteV2(context.Context, runtimeports.ControlledOperationProviderRouteCurrentRefV2, runtimeports.OperationScopeEvidenceApplicabilityMatrixKeyV3) (runtimeports.ControlledOperationProviderRouteCurrentProjectionV2, error) {
	return r.projection, nil
}

type providerReaderV2 struct {
	projection runtimeports.ProviderBindingCurrentProjectionV2
}

func (r *providerReaderV2) InspectProviderBindingCurrentV2(context.Context, runtimeports.ProviderBindingRefV2) (runtimeports.ProviderBindingCurrentProjectionV2, error) {
	return r.projection, nil
}

type lostReplyBindingStoreV2 struct {
	inner    *InMemorySingleCallToolActionBindingLeaseStoreV2
	creates  atomic.Uint64
	inspects atomic.Uint64
}

func (s *lostReplyBindingStoreV2) CreateSingleCallToolActionBindingCurrentOnceV2(ctx context.Context, p SingleCallToolActionBindingCurrentProjectionV2) (SingleCallToolActionBindingCurrentProjectionV2, error) {
	s.creates.Add(1)
	if _, err := s.inner.CreateSingleCallToolActionBindingCurrentOnceV2(ctx, p); err != nil {
		return SingleCallToolActionBindingCurrentProjectionV2{}, err
	}
	return SingleCallToolActionBindingCurrentProjectionV2{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "lost BindingV2 create reply")
}
func (s *lostReplyBindingStoreV2) InspectSingleCallToolActionBindingCurrentByIssuanceIDV2(ctx context.Context, id string) (SingleCallToolActionBindingCurrentProjectionV2, error) {
	s.inspects.Add(1)
	return s.inner.InspectSingleCallToolActionBindingCurrentByIssuanceIDV2(ctx, id)
}
func (s *lostReplyBindingStoreV2) InspectExactSingleCallToolActionBindingCurrentV2(ctx context.Context, ref toolcontract.SingleCallToolActionBindingCurrentRefV2) (SingleCallToolActionBindingCurrentProjectionV2, error) {
	return s.inner.InspectExactSingleCallToolActionBindingCurrentV2(ctx, ref)
}
