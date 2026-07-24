package application_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	application "github.com/Proview-China/rax/ExecutionRuntime/application"
	"github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/application/fakes"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/tests/testsupport"
)

type singleCallFixtureV2 struct {
	now         time.Time
	request     contract.SingleCallToolActionRequestV2
	result      contract.SingleCallToolActionResultV2
	association runtimeports.OperationSettlementEvidenceAssociationV4
	store       *fakes.SingleCallToolActionCoordinationStoreV2
	inputs      *singleCallInputV2
	tool        *singleCallToolV2
	settlements *singleCallSettlementV2
	coordinator *application.SingleCallToolActionCoordinatorV2
}

func newSingleCallFixtureV2(t *testing.T) *singleCallFixtureV2 {
	t.Helper()
	now := time.Unix(500000, 0)
	submission := testsupport.OperationSettlementSubmissionV4()
	v1req := singleCallRequestV1(t, now, submission)
	v1result, association := singleCallResultV1(t, now, v1req, submission)
	request, projection := singleCallRequestAndProjectionV2(t, now, submission)
	owner := contract.SingleCallToolOwnerResultRefCoordinateV2{OwnerContractVersion: contract.SingleCallToolOwnerResultContractVersionV2, ID: "tool-result-v2", Revision: 1, Digest: core.DigestBytes([]byte("tool-result-v2")), ActionID: request.Action.PendingSubject.PendingActionRef, ActionRevision: 1, ActionDigest: request.Action.Digest, ApplyID: "apply-v2", ApplyRevision: 1, ApplyDigest: core.DigestBytes([]byte("apply-v2")), Inspection: v1result.Inspection, Schema: v1result.Inspection.DomainResult.Schema, PayloadDigest: v1result.Inspection.DomainResult.PayloadDigest, PayloadRevision: v1result.Inspection.DomainResult.PayloadRevision, FinalizedUnixNano: now.Add(-time.Nanosecond).UnixNano()}
	coord, err := contract.SealSingleCallToolActionResultCoordinateV2(contract.SingleCallToolActionResultCoordinateV2{ToolResult: owner, Inspection: v1result.Inspection, Association: association.RefV4(), AssociationCheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(5 * time.Second).UnixNano()}, request, now)
	if err != nil {
		t.Fatal(err)
	}
	result, err := contract.SealSingleCallToolActionResultV2(contract.SingleCallToolActionResultV2{Coordinate: coord}, request, now)
	if err != nil {
		t.Fatal(err)
	}
	store := fakes.NewSingleCallToolActionCoordinationStoreV2()
	inputs := &singleCallInputV2{projection: projection}
	tool := &singleCallToolV2{result: result}
	settlements := &singleCallSettlementV2{inspection: v1result.Inspection, association: association}
	coordinator, err := application.NewSingleCallToolActionCoordinatorV2(application.SingleCallToolActionCoordinatorConfigV2{Facts: store, Tool: tool, Inputs: inputs, Settlements: settlements, Clock: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	return &singleCallFixtureV2{now, request, result, association, store, inputs, tool, settlements, coordinator}
}

func singleCallRequestAndProjectionV2(t *testing.T, now time.Time, submission runtimeports.OperationSettlementSubmissionV4) (contract.SingleCallToolActionRequestV2, contract.SingleCallToolActionInputCurrentProjectionV2) {
	t.Helper()
	d := core.DigestBytes
	args := []byte(`{"x":1}`)
	argsDigest := d(args)
	domainDigest := d([]byte("model-domain-result"))
	modelOperation := submission.Operation
	modelOperation.Kind = runtimeports.OperationScopeRunV3
	modelOperation.ActivationAttemptID = ""
	modelOperation.RunID = "run-v2"
	modelOperation.CurrentProjectionRef = "model-turn-projection-v2"
	modelOperation.CurrentProjectionRevision = 1
	modelOperation.CurrentProjectionDigest = d([]byte("model-turn-projection-v2"))
	modelOperationDigest, err := modelOperation.DigestV3()
	if err != nil {
		t.Fatal(err)
	}
	inputSchema := runtimeports.SchemaRefV2{Namespace: "praxis.tool", Name: "call-arguments", Version: "1.0.0", MediaType: "application/json", ContentDigest: d([]byte("tool-call-arguments-schema-v2"))}
	if err := inputSchema.Validate(); err != nil {
		t.Fatal(err)
	}
	pending := contract.SingleCallPendingActionCoordinateV1{ActionRef: "action-v2", RequestDigest: d([]byte("pending-request-v2")), Capability: "praxis.tool/execute", PayloadSchema: inputSchema, PayloadDigest: argsDigest, SourceCandidateID: "candidate-v2", SourceCandidateRevision: 1, SourceCandidateDigest: d([]byte("candidate-v2")), ProjectionDigest: d([]byte("projection-v2"))}
	identity, err := contract.SealSingleCallModelPendingActionIdentityCoordinateV2(contract.SingleCallModelPendingActionIdentityCoordinateV2{IdentityContractVersion: contract.SingleCallIdentityContractVersionV1, IdentityID: "identity-v2", IdentityRevision: 1, IdentityDigest: d([]byte("identity-owner-v2")), CreatedUnixNano: now.Add(-time.Second).UnixNano(), ModelProjectionID: "projection-v2", ModelProjectionRevision: 1, ModelProjectionDigest: pending.ProjectionDigest, ModelInvocationID: "invocation-v2", ModelInvocationDigest: d([]byte("invocation-v2")), ModelObservationDigest: d([]byte("observation-v2")), ModelSourceResponseID: "response-v2", ModelSourceSequence: 1, SourceKeyDigest: d([]byte("source-key-v2")), SourceExecutionScopeDigest: mustScopeDigestV2(t, modelOperation.ExecutionScope), SourceRunID: string(modelOperation.RunID), SourceSessionID: "session-v2", SourceTurn: 1, CallOrdinalEncodingVersion: contract.SingleCallCallOrdinalEncodingVersionV1, CallOrdinalPresent: true, SettlementOwner: submission.DomainResult.Owner, CallID: "call-v2", CallName: "tool-v2", CanonicalArgumentsDigest: argsDigest, PendingActionRef: pending.ActionRef, PendingActionRequestDigest: pending.RequestDigest, PayloadSchema: pending.PayloadSchema, PayloadContentDigest: argsDigest, Capability: pending.Capability, SourceCandidateID: pending.SourceCandidateID, SourceCandidateRevision: pending.SourceCandidateRevision, SourceCandidateDigest: pending.SourceCandidateDigest, DomainResultDigest: domainDigest, NotAfterUnixNano: now.Add(9 * time.Second).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	idref := contract.SingleCallModelPendingActionIdentityRefCoordinateV2{ID: identity.IdentityID, Revision: identity.IdentityRevision, Digest: identity.IdentityDigest, ModelProjectionID: identity.ModelProjectionID, ModelProjectionRevision: identity.ModelProjectionRevision, ModelProjectionDigest: identity.ModelProjectionDigest, PendingActionRef: identity.PendingActionRef, PendingActionRequestDigest: identity.PendingActionRequestDigest, DomainResultDigest: identity.DomainResultDigest, SourceKeyDigest: identity.SourceKeyDigest}
	factref := contract.SingleCallSettledTurnDomainResultFactRefCoordinateV2{FactID: "domain-fact-v2", Revision: 1, FactDigest: d([]byte("domain-fact-v2")), SourceKeyDigest: identity.SourceKeyDigest, Schema: submission.DomainResult.Schema, ContentDigest: domainDigest, IdentityRef: idref}
	evidence := runtimeports.EvidenceRecordRefV2{LedgerScopeDigest: d([]byte("ledger-v2")), Sequence: 1, RecordDigest: d([]byte("record-v2"))}
	modelAttempt := submission.DomainResult.Attempt
	modelAttempt.OperationDigest = modelOperationDigest
	settlement := runtimeports.OperationSettlementRefV3{ID: "model-settlement-v2", Revision: 1, Digest: d([]byte("model-settlement-v2")), Attempt: modelAttempt, Disposition: runtimeports.OperationSettlementAppliedV3, Owner: submission.Owner, Evidence: []runtimeports.EvidenceRecordRefV2{evidence}, DomainResultSchema: &factref.Schema, DomainResultDigest: domainDigest}
	base, err := contract.SealSingleCallHarnessBaseBindingCoordinateV2(contract.SingleCallHarnessBaseBindingCoordinateV2{PendingAction: pending, IdentityRef: idref, DomainResultFact: factref, ModelTurnSettlement: settlement})
	if err != nil {
		t.Fatal(err)
	}
	matrix := runtimeports.OperationScopeEvidenceActionMatrixV3()
	matrixDigest, err := runtimeports.DigestOperationScopeEvidenceApplicabilityMatrixKeyV3(matrix)
	if err != nil {
		t.Fatal(err)
	}
	decl := runtimeports.ControlledOperationProviderRouteDeclarationRefV2{RouteID: "route-v2", Revision: 1, PublisherComponentID: "praxis.harness/assembly", DeclarationDigest: d([]byte("route-decl-v2"))}
	conf := runtimeports.ControlledOperationProviderRouteConformanceRefV2{ConformanceID: "route-conf-v2", Revision: 1, DeclarationRef: decl, ConformanceDigest: d([]byte("route-conf-v2"))}
	currentID, err := runtimeports.DeriveControlledOperationProviderRouteCurrentIDV2(decl.RouteID, matrixDigest)
	if err != nil {
		t.Fatal(err)
	}
	route, err := runtimeports.SealControlledOperationProviderRouteCurrentRefV2(runtimeports.ControlledOperationProviderRouteCurrentRefV2{CurrentID: currentID, Revision: 1, DeclarationRef: decl, ConformanceRef: conf, MatrixDigest: matrixDigest, Watermark: d([]byte("route-watermark-v2"))})
	if err != nil {
		t.Fatal(err)
	}
	ownerInputs, err := contract.SealSingleCallHarnessOwnerCurrentInputsCoordinateV2(contract.SingleCallHarnessOwnerCurrentInputsCoordinateV2{HarnessContractVersion: contract.SingleCallHarnessOwnerCurrentInputsVersionV1, ModelTurnOperation: modelOperation, GenerationBindingAssociation: runtimeports.GenerationBindingAssociationRefV1{ID: "gen-assoc-v2", Revision: 1, Digest: d([]byte("gen-assoc-v2"))}, RouteCurrent: route, RouteMatrix: matrix, ContextApplicability: runtimeports.OperationScopeEvidenceApplicabilityFactRefV3{Kind: runtimeports.OperationScopeEvidenceContextParentKindV3, ID: "context-v2", Revision: 1, Digest: d([]byte("context-v2"))}, HarnessDigest: d([]byte("owner-inputs-v2"))})
	if err != nil {
		t.Fatal(err)
	}
	binding, err := contract.SealSingleCallHarnessApplicationBindingCoordinateV2(contract.SingleCallHarnessApplicationBindingCoordinateV2{BindingVersion: contract.SingleCallHarnessBindingContractVersionV2, Base: base, OwnerInputs: ownerInputs, HarnessBindingDigest: d([]byte("binding-owner-v2"))})
	if err != nil {
		t.Fatal(err)
	}
	run, err := contract.SealSingleCallRunSubjectCoordinateV2(contract.SingleCallRunSubjectCoordinateV2{ExecutionScope: modelOperation.ExecutionScope, RunID: modelOperation.RunID})
	if err != nil {
		t.Fatal(err)
	}
	subject, err := contract.SealSingleCallPendingActionSubjectCoordinateV2(contract.SingleCallPendingActionSubjectCoordinateV2{Run: run, SessionID: "session-v2", SessionRevision: 1, SessionDigest: d([]byte("session-v2")), Turn: 1, PendingActionRef: pending.ActionRef, PendingActionDigest: pending.RequestDigest, Binding: binding, Identity: identity})
	if err != nil {
		t.Fatal(err)
	}
	action, err := contract.SealSingleCallActionCoordinateV2(contract.SingleCallActionCoordinateV2{ExecutionScope: submission.Operation.ExecutionScope, PendingSubject: subject})
	if err != nil {
		t.Fatal(err)
	}
	authority := runtimeports.AuthorityBindingRefV2{Ref: "authority-v2", Revision: 1, Digest: d([]byte("authority-v2")), Epoch: submission.Operation.ExecutionScope.AuthorityEpoch}
	request, err := contract.SealSingleCallToolActionRequestV2(contract.SingleCallToolActionRequestV2{Action: action, Authority: authority, CreatedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(8 * time.Second).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	proof, err := contract.SealSingleCallModelToolCallProjectionProofV2(contract.SingleCallModelToolCallProjectionProofV2{ProjectionContractVersion: contract.SingleCallModelProjectionContractVersionV1, ProjectionID: identity.ModelProjectionID, ProjectionRevision: identity.ModelProjectionRevision, ProjectionDigest: identity.ModelProjectionDigest, InvocationID: identity.ModelInvocationID, InvocationDigest: identity.ModelInvocationDigest, ObservationDigest: identity.ModelObservationDigest, SourceResponseID: identity.ModelSourceResponseID, SourceSequence: identity.ModelSourceSequence, CallID: identity.CallID, CallName: identity.CallName, CanonicalArguments: args})
	if err != nil {
		t.Fatal(err)
	}
	identityCurrentRequest, err := contract.SealSingleCallModelPendingActionIdentityCurrentRequestV2(contract.SingleCallModelPendingActionIdentityCurrentRequestV2{Run: run, SessionID: subject.SessionID, Turn: subject.Turn, IdentityRef: idref, DomainResultFact: factref, RequestedNotAfterUnixNano: request.ExpiresUnixNano})
	if err != nil {
		t.Fatal(err)
	}
	identityCurrent, err := contract.SealSingleCallModelPendingActionIdentityCurrentV2(contract.SingleCallModelPendingActionIdentityCurrentV2{IdentityRef: idref, DomainResultFact: factref, Identity: identity, Projection: proof, CheckedUnixNano: now.Add(-time.Millisecond).UnixNano(), ExpiresUnixNano: now.Add(7 * time.Second).UnixNano()}, identityCurrentRequest, now)
	if err != nil {
		t.Fatal(err)
	}
	harnessCurrent, err := contract.SealSingleCallHarnessOwnerCurrentProofV3(contract.SingleCallHarnessOwnerCurrentProofV3{Subject: subject, Binding: binding, HarnessCurrentContractVersion: contract.SingleCallHarnessCurrentContractVersionV3, HarnessCurrentDigest: d([]byte("harness-current-v2")), IdentityCurrent: identityCurrent, CheckedUnixNano: now.Add(-time.Millisecond).UnixNano(), ExpiresUnixNano: now.Add(6 * time.Second).UnixNano()}, now)
	if err != nil {
		t.Fatal(err)
	}
	authorityCurrent, err := contract.SealSingleCallAuthorityCurrentProofV2(contract.SingleCallAuthorityCurrentProofV2{Ref: authority, ExecutionScopeDigest: action.ExecutionScopeDigest, ActionCoordinateDigest: action.Digest, FactRevision: 1, FactDigest: d([]byte("authority-fact-v2")), CheckedUnixNano: now.Add(-time.Millisecond).UnixNano(), ExpiresUnixNano: now.Add(6 * time.Second).UnixNano()}, request, now)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := contract.SealSingleCallToolActionInputCurrentProjectionV2(contract.SingleCallToolActionInputCurrentProjectionV2{HarnessCurrent: harnessCurrent, AuthorityCurrent: authorityCurrent, CheckedUnixNano: now.Add(-time.Millisecond).UnixNano(), ExpiresUnixNano: now.Add(5 * time.Second).UnixNano()}, request, now)
	if err != nil {
		t.Fatal(err)
	}
	return request, projection
}

func mustScopeDigestV2(t *testing.T, s core.ExecutionScope) core.Digest {
	t.Helper()
	d, e := runtimeports.ExecutionScopeDigestV2(s)
	if e != nil {
		t.Fatal(e)
	}
	return d
}

type singleCallInputV2 struct {
	mu         sync.Mutex
	projection contract.SingleCallToolActionInputCurrentProjectionV2
	calls      int
}

func (r *singleCallInputV2) InspectSingleCallToolActionInputCurrentV2(context.Context, contract.SingleCallToolActionRequestV2) (contract.SingleCallToolActionInputCurrentProjectionV2, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	return contract.CloneSingleCallToolActionInputCurrentProjectionV2(r.projection), nil
}

type singleCallToolV2 struct {
	mu                         sync.Mutex
	result                     contract.SingleCallToolActionResultV2
	stored                     bool
	executeCalls, inspectCalls int
	loseExecute                bool
	onExecute                  func()
}

func (p *singleCallToolV2) ExecuteSingleCallToolActionV2(context.Context, contract.SingleCallToolActionRequestV2) (contract.SingleCallToolActionResultV2, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.executeCalls++
	p.stored = true
	if p.onExecute != nil {
		p.onExecute()
	}
	if p.loseExecute {
		p.loseExecute = false
		return contract.SingleCallToolActionResultV2{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "lost execute reply")
	}
	return p.result, nil
}
func (p *singleCallToolV2) InspectSingleCallToolActionV2(context.Context, contract.SingleCallToolActionInspectKeyV2) (contract.SingleCallToolActionResultV2, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.inspectCalls++
	if !p.stored {
		return contract.SingleCallToolActionResultV2{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "tool result absent")
	}
	return p.result, nil
}

type singleCallSettlementV2 struct {
	mu               sync.Mutex
	inspection       runtimeports.OperationInspectionSettlementRefV4
	association      runtimeports.OperationSettlementEvidenceAssociationV4
	currentCalls     int
	associationCalls int
	loseCurrent      bool
	loseAssociation  bool
}

func (r *singleCallSettlementV2) InspectCurrentOperationSettlementV4(context.Context, runtimeports.InspectCurrentOperationSettlementRequestV4) (runtimeports.OperationInspectionSettlementRefV4, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.currentCalls++
	if r.loseCurrent {
		r.loseCurrent = false
		return runtimeports.OperationInspectionSettlementRefV4{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "lost settlement Inspect reply")
	}
	return r.inspection, nil
}
func (r *singleCallSettlementV2) InspectOperationSettlementEvidenceAssociationV4(context.Context, runtimeports.OperationSubjectV3, runtimeports.OperationSettlementEvidenceAssociationRefV4) (runtimeports.OperationSettlementEvidenceAssociationV4, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.associationCalls++
	if r.loseAssociation {
		r.loseAssociation = false
		return runtimeports.OperationSettlementEvidenceAssociationV4{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "lost association Inspect reply")
	}
	return r.association, nil
}

func TestSingleCallToolActionV2CanonicalBytesNoAlias(t *testing.T) {
	fx := newSingleCallFixtureV2(t)
	p := fx.inputs.projection.HarnessCurrent.IdentityCurrent.Projection
	original := append([]byte(nil), p.CanonicalArguments...)
	clone := contract.CloneSingleCallToolActionInputCurrentProjectionV2(fx.inputs.projection)
	clone.HarnessCurrent.IdentityCurrent.Projection.CanonicalArguments[0] ^= 0xff
	if string(p.CanonicalArguments) != string(original) {
		t.Fatal("projection proof aliased returned bytes")
	}
	bad := p
	bad.CanonicalArguments = nil
	if _, err := contract.SealSingleCallModelToolCallProjectionProofV2(bad); err == nil {
		t.Fatal("empty canonical bytes accepted")
	}
	bad = p
	bad.CanonicalArguments = make([]byte, runtimeports.MaxOpaqueInlineBytes+1)
	if _, err := contract.SealSingleCallModelToolCallProjectionProofV2(bad); err == nil {
		t.Fatal("oversize canonical bytes accepted")
	}
}

func TestSingleCallToolActionCoordinatorV2HappyAndReplay(t *testing.T) {
	fx := newSingleCallFixtureV2(t)
	got, err := fx.coordinator.CoordinateSingleCallToolActionV2(context.Background(), fx.request)
	if err != nil || got.Digest != fx.result.Digest {
		t.Fatalf("coordinate failed: %v", err)
	}
	if fx.tool.executeCalls != 1 {
		t.Fatalf("execute calls=%d", fx.tool.executeCalls)
	}
	if _, err = fx.coordinator.CoordinateSingleCallToolActionV2(context.Background(), fx.request); err != nil {
		t.Fatal(err)
	}
	if fx.tool.executeCalls != 1 {
		t.Fatal("replay executed again")
	}
}

func TestSingleCallToolActionDispatchIntentLostReplyIsPermanentlyInspectOnlyV2(t *testing.T) {
	fx := newSingleCallFixtureV2(t)
	if err := fx.store.LoseNextCASReplyForTestV2(); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.coordinator.CoordinateSingleCallToolActionV2(context.Background(), fx.request); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("want inspect NotFound, got %v", err)
	}
	if fx.tool.executeCalls != 0 {
		t.Fatal("lost dispatch CAS executed")
	}
	second, err := application.NewSingleCallToolActionCoordinatorV2(application.SingleCallToolActionCoordinatorConfigV2{Facts: fx.store, Tool: fx.tool, Inputs: fx.inputs, Settlements: fx.settlements, Clock: func() time.Time { return fx.now }})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = second.CoordinateSingleCallToolActionV2(context.Background(), fx.request); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("restart should inspect only: %v", err)
	}
	if fx.tool.executeCalls != 0 {
		t.Fatal("restart reclaimed dispatch_intent")
	}
	if creates, cas := fx.store.Counts(); creates != 1 || cas != 1 {
		t.Fatalf("lost dispatch reply/restart changed commits: create=%d cas=%d", creates, cas)
	}
}

func TestSingleCallToolActionCoordinatorV2ConcurrentOneExecute(t *testing.T) {
	fx := newSingleCallFixtureV2(t)
	facts := &countingDispatchFactPortV2{SingleCallToolActionCoordinationFactPortV2: fx.store}
	coordinators := make([]*application.SingleCallToolActionCoordinatorV2, 64)
	for index := range coordinators {
		coordinator, err := application.NewSingleCallToolActionCoordinatorV2(application.SingleCallToolActionCoordinatorConfigV2{Facts: facts, Tool: fx.tool, Inputs: fx.inputs, Settlements: fx.settlements, Clock: func() time.Time { return fx.now }})
		if err != nil {
			t.Fatal(err)
		}
		coordinators[index] = coordinator
	}
	var wg sync.WaitGroup
	errs := make(chan error, 64)
	for _, coordinator := range coordinators {
		wg.Add(1)
		go func(coordinator *application.SingleCallToolActionCoordinatorV2) {
			defer wg.Done()
			_, err := coordinator.CoordinateSingleCallToolActionV2(context.Background(), fx.request)
			errs <- err
		}(coordinator)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil && !core.HasCategory(err, core.ErrorNotFound) {
			t.Fatal(err)
		}
	}
	if facts.dispatchSuccesses.Load() != 1 {
		t.Fatalf("normal prepared->dispatch_intent successes=%d", facts.dispatchSuccesses.Load())
	}
	if fx.tool.executeCalls != 1 {
		t.Fatalf("execute calls=%d", fx.tool.executeCalls)
	}
	if creates, cas := fx.store.Counts(); creates != 1 || cas != 3 {
		t.Fatalf("64 coordinators produced unexpected commits: create=%d cas=%d", creates, cas)
	}
}

func TestSingleCallToolActionCoordinatorV2RefreshesStaleDispatchBeforeCompletion(t *testing.T) {
	fx := newSingleCallFixtureV2(t)
	prepared := mustPreparedFactV2(t, fx.request)
	if _, err := fx.store.CreateSingleCallToolActionCoordinationV2(context.Background(), prepared); err != nil {
		t.Fatal(err)
	}
	dispatch := mustNextFactV2(t, prepared, contract.SingleCallToolActionDispatchIntentV2, nil, fx.now)
	if _, err := fx.store.CompareAndSwapSingleCallToolActionCoordinationV2(context.Background(), mustCASV2(t, prepared, dispatch)); err != nil {
		t.Fatal(err)
	}
	claimID, err := contract.DeriveSingleCallToolActionStartClaimIDV2(fx.request)
	if err != nil {
		t.Fatal(err)
	}
	waiting, err := contract.ClaimSingleCallToolActionStartV2(dispatch, claimID, fx.now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = fx.store.CompareAndSwapSingleCallToolActionCoordinationV2(context.Background(), mustCASV2(t, dispatch, waiting)); err != nil {
		t.Fatal(err)
	}
	fx.tool.mu.Lock()
	fx.tool.stored = true
	fx.tool.mu.Unlock()
	facts := &staleCreateFactPortV2{SingleCallToolActionCoordinationFactPortV2: fx.store, stale: dispatch}
	coordinator, err := application.NewSingleCallToolActionCoordinatorV2(application.SingleCallToolActionCoordinatorConfigV2{Facts: facts, Tool: fx.tool, Inputs: fx.inputs, Settlements: fx.settlements, Clock: func() time.Time { return fx.now }})
	if err != nil {
		t.Fatal(err)
	}
	result, err := coordinator.CoordinateSingleCallToolActionV2(context.Background(), fx.request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Digest != fx.result.Digest {
		t.Fatal("stale dispatch recovery returned another result")
	}
	completed, err := fx.store.InspectSingleCallToolActionCoordinationV2(context.Background(), fx.request.Action.ExecutionScope, fx.request.ID)
	if err != nil {
		t.Fatal(err)
	}
	if completed.State != contract.SingleCallToolActionCompletedV2 || completed.Result == nil || *completed.Result != fx.result.RefV2() {
		t.Fatalf("stale dispatch recovery did not complete exact result: %#v", completed)
	}
}

func TestSingleCallToolActionCoordinatorV2RejectsResultWithoutPersistedStartClaim(t *testing.T) {
	fx := newSingleCallFixtureV2(t)
	prepared := mustPreparedFactV2(t, fx.request)
	if _, err := fx.store.CreateSingleCallToolActionCoordinationV2(context.Background(), prepared); err != nil {
		t.Fatal(err)
	}
	dispatch := mustNextFactV2(t, prepared, contract.SingleCallToolActionDispatchIntentV2, nil, fx.now)
	if _, err := fx.store.CompareAndSwapSingleCallToolActionCoordinationV2(context.Background(), mustCASV2(t, prepared, dispatch)); err != nil {
		t.Fatal(err)
	}
	fx.tool.mu.Lock()
	fx.tool.stored = true
	fx.tool.mu.Unlock()
	facts := &staleCreateFactPortV2{SingleCallToolActionCoordinationFactPortV2: fx.store, stale: dispatch}
	coordinator, err := application.NewSingleCallToolActionCoordinatorV2(application.SingleCallToolActionCoordinatorConfigV2{Facts: facts, Tool: fx.tool, Inputs: fx.inputs, Settlements: fx.settlements, Clock: func() time.Time { return fx.now }})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = coordinator.CoordinateSingleCallToolActionV2(context.Background(), fx.request); !core.HasCategory(err, core.ErrorPreconditionFailed) || !core.HasReason(err, core.ReasonInvalidState) {
		t.Fatalf("Tool result bypassed the persisted start claim: %v", err)
	}
	current, err := fx.store.InspectSingleCallToolActionCoordinationV2(context.Background(), fx.request.Action.ExecutionScope, fx.request.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.State != contract.SingleCallToolActionDispatchIntentV2 || current.Result != nil {
		t.Fatalf("start-claim bypass changed coordination: %#v", current)
	}
	if creates, cas := fx.store.Counts(); creates != 1 || cas != 1 {
		t.Fatalf("start-claim bypass committed terminal state: create=%d cas=%d", creates, cas)
	}
}

type countingDispatchFactPortV2 struct {
	applicationports.SingleCallToolActionCoordinationFactPortV2
	dispatchSuccesses atomic.Uint64
}

type staleCreateFactPortV2 struct {
	applicationports.SingleCallToolActionCoordinationFactPortV2
	stale contract.SingleCallToolActionCoordinationFactV2
}

func (p *staleCreateFactPortV2) CreateSingleCallToolActionCoordinationV2(_ context.Context, fact contract.SingleCallToolActionCoordinationFactV2) (contract.SingleCallToolActionCoordinationFactV2, error) {
	if fact.ID != p.stale.ID || fact.Request.Digest != p.stale.Request.Digest {
		return contract.SingleCallToolActionCoordinationFactV2{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "stale create fixture received another request")
	}
	return p.stale, nil
}

func (p *countingDispatchFactPortV2) CompareAndSwapSingleCallToolActionCoordinationV2(ctx context.Context, request applicationports.SingleCallToolActionCoordinationCASRequestV2) (contract.SingleCallToolActionCoordinationFactV2, error) {
	stored, err := p.SingleCallToolActionCoordinationFactPortV2.CompareAndSwapSingleCallToolActionCoordinationV2(ctx, request)
	if err == nil && request.Next.State == contract.SingleCallToolActionDispatchIntentV2 {
		p.dispatchSuccesses.Add(1)
	}
	return stored, err
}

func TestSingleCallToolActionCoordinatorV2RejectsTypedNil(t *testing.T) {
	var store *fakes.SingleCallToolActionCoordinationStoreV2
	fx := newSingleCallFixtureV2(t)
	if _, err := application.NewSingleCallToolActionCoordinatorV2(application.SingleCallToolActionCoordinatorConfigV2{Facts: store, Tool: fx.tool, Inputs: fx.inputs, Settlements: fx.settlements}); !core.HasCategory(err, core.ErrorInvalidArgument) {
		t.Fatalf("typed nil accepted: %v", err)
	}
}

func TestSingleCallToolActionCompletedResultRefMustBeExactV2(t *testing.T) {
	fx := newSingleCallFixtureV2(t)
	prepared := mustPreparedFactV2(t, fx.request)
	dispatch := mustNextFactV2(t, prepared, contract.SingleCallToolActionDispatchIntentV2, nil, fx.now)
	claimID, err := contract.DeriveSingleCallToolActionStartClaimIDV2(fx.request)
	if err != nil {
		t.Fatal(err)
	}
	waiting, err := contract.ClaimSingleCallToolActionStartV2(dispatch, claimID, fx.now)
	if err != nil {
		t.Fatal(err)
	}
	exact := fx.result.RefV2()
	completed, err := contract.CompleteSingleCallToolActionCoordinationFactV2(waiting, fx.result, fx.now)
	if err != nil {
		t.Fatal(err)
	}
	if err := completed.Validate(); err != nil {
		t.Fatalf("exact completion invalid: %v", err)
	}
	mutations := map[string]func(*contract.SingleCallToolActionResultRefV2){
		"request_id":       func(v *contract.SingleCallToolActionResultRefV2) { v.RequestID += "-splice" },
		"request_revision": func(v *contract.SingleCallToolActionResultRefV2) { v.RequestRevision++ },
		"request_digest": func(v *contract.SingleCallToolActionResultRefV2) {
			v.RequestDigest = core.DigestBytes([]byte("other request"))
		},
		"action_digest": func(v *contract.SingleCallToolActionResultRefV2) {
			v.ActionCoordinateDigest = core.DigestBytes([]byte("other action"))
		},
		"tool_result_id": func(v *contract.SingleCallToolActionResultRefV2) { v.ToolResultID += "-splice" },
		"tool_result_digest": func(v *contract.SingleCallToolActionResultRefV2) {
			v.ToolResultDigest = core.DigestBytes([]byte("other tool result"))
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			// Coordination Fact validation rejects a stale outer digest after a
			// stored ResultRef is tampered. Tool Owner provenance is a P4 gate.
			changed := exact
			mutate(&changed)
			forged := completed
			forged.Result = &changed
			if err := forged.Validate(); err == nil {
				t.Fatal("spliced completed ResultRef accepted by Fact validation")
			}
		})
	}
	if _, err := contract.NextSingleCallToolActionCoordinationFactV2(waiting, contract.SingleCallToolActionCompletedV2, &exact, fx.now); err == nil {
		t.Fatal("generic transition API accepted completed ResultRef without the full Result")
	}
	if _, err := applicationports.SealSingleCallToolActionCoordinationCASRequestV2(applicationports.SingleCallToolActionCoordinationCASRequestV2{Scope: waiting.Request.Action.ExecutionScope, ID: waiting.ID, ExpectedRevision: waiting.Revision, ExpectedDigest: waiting.Digest, Next: completed}); err != nil {
		t.Fatalf("structurally exact completed successor was rejected: %v", err)
	}
}

func TestSingleCallToolActionNeutralAndCurrentCanonicalSpliceRejectedV2(t *testing.T) {
	fx := newSingleCallFixtureV2(t)
	requestSplice := fx.request
	requestSplice.Action.PendingSubject.Binding.Base.PendingAction.RequestDigest = core.DigestBytes([]byte("spliced pending action"))
	if err := requestSplice.Validate(); err == nil {
		t.Fatal("neutral PendingAction splice accepted under old canonical digest")
	}
	currentSplice := contract.CloneSingleCallToolActionInputCurrentProjectionV2(fx.inputs.projection)
	currentSplice.HarnessCurrent.IdentityCurrent.Projection.CallName = "spliced-tool"
	if err := currentSplice.ValidateFor(fx.request, fx.now); err == nil {
		t.Fatal("nested current Projection splice accepted")
	}
	authoritySplice := contract.CloneSingleCallToolActionInputCurrentProjectionV2(fx.inputs.projection)
	authoritySplice.AuthorityCurrent.ActionCoordinateDigest = core.DigestBytes([]byte("spliced action"))
	if err := authoritySplice.ValidateFor(fx.request, fx.now); err == nil {
		t.Fatal("Authority current splice accepted")
	}
	t.Run("input_and_domain_result_schema_are_distinct", func(t *testing.T) {
		base := fx.request.Action.PendingSubject.Binding.Base
		if base.PendingAction.PayloadSchema == base.DomainResultFact.Schema {
			t.Fatal("tool input schema unexpectedly equals settled domain-result schema")
		}
		if err := base.Validate(); err != nil {
			t.Fatalf("distinct input/result schemas rejected: %v", err)
		}
	})
	t.Run("domain_result_schema_settlement_mismatch", func(t *testing.T) {
		binding := fx.request.Action.PendingSubject.Binding
		base := binding.Base
		splicedSchema := base.DomainResultFact.Schema
		splicedSchema.Name = "spliced-tool-result"
		splicedSchema.ContentDigest = core.DigestBytes([]byte("spliced tool result schema"))
		if err := splicedSchema.Validate(); err != nil {
			t.Fatal(err)
		}
		base.DomainResultFact.Schema = splicedSchema
		base.Digest = ""
		digest, err := base.DigestV2()
		if err != nil {
			t.Fatal(err)
		}
		base.Digest = digest
		if err := base.Validate(); !core.HasCategory(err, core.ErrorConflict) {
			t.Fatalf("Fact/Settlement schema mismatch was not rejected: %v", err)
		}
	})
}

func TestSingleCallToolActionModelOperationAndRouteSpliceRejectedV2(t *testing.T) {
	fx := newSingleCallFixtureV2(t)
	t.Run("harness_contract_version", func(t *testing.T) {
		ownerInputs := fx.request.Action.PendingSubject.Binding.OwnerInputs
		ownerInputs.HarnessContractVersion = "praxis.harness.committed-pending-action-owner-current-inputs/v2"
		if _, err := contract.SealSingleCallHarnessOwnerCurrentInputsCoordinateV2(ownerInputs); err == nil {
			t.Fatal("another Harness OwnerInputs contract version was accepted after canonical reseal")
		}
	})
	t.Run("context_applicability_kind", func(t *testing.T) {
		ownerInputs := fx.request.Action.PendingSubject.Binding.OwnerInputs
		ownerInputs.ContextApplicability.Kind = runtimeports.OperationScopeEvidenceSessionCurrentKindV3
		ownerInputs.ContextApplicability.Digest = core.DigestBytes([]byte("spliced context applicability"))
		if _, err := contract.SealSingleCallHarnessOwnerCurrentInputsCoordinateV2(ownerInputs); err == nil {
			t.Fatal("non-ContextParent applicability was accepted after canonical reseal")
		}
	})
	t.Run("model_operation_kind", func(t *testing.T) {
		subject := fx.request.Action.PendingSubject
		ownerInputs := subject.Binding.OwnerInputs
		operation := ownerInputs.ModelTurnOperation
		operation.Kind = runtimeports.OperationScopeActivationV3
		operation.RunID = ""
		operation.ActivationAttemptID = "another-activation"
		operation.CurrentProjectionRef = "another-activation-projection"
		operation.CurrentProjectionDigest = core.DigestBytes([]byte("another-activation-projection"))
		ownerInputs.ModelTurnOperation = operation
		ownerInputs, err := contract.SealSingleCallHarnessOwnerCurrentInputsCoordinateV2(ownerInputs)
		if err != nil {
			t.Fatal(err)
		}
		binding := subject.Binding
		binding.OwnerInputs = ownerInputs
		binding, err = contract.SealSingleCallHarnessApplicationBindingCoordinateV2(binding)
		if err != nil {
			t.Fatal(err)
		}
		subject.Binding = binding
		if _, err := contract.SealSingleCallPendingActionSubjectCoordinateV2(subject); err == nil {
			t.Fatal("activation ModelTurnOperation accepted for a Run Action")
		}
	})
	t.Run("model_operation_run", func(t *testing.T) {
		subject := fx.request.Action.PendingSubject
		ownerInputs := subject.Binding.OwnerInputs
		operation := ownerInputs.ModelTurnOperation
		operation.RunID = "another-run"
		operation.CurrentProjectionRef = "another-run-projection"
		operation.CurrentProjectionDigest = core.DigestBytes([]byte("another-run-projection"))
		ownerInputs.ModelTurnOperation = operation
		ownerInputs, err := contract.SealSingleCallHarnessOwnerCurrentInputsCoordinateV2(ownerInputs)
		if err != nil {
			t.Fatal(err)
		}
		binding := subject.Binding
		binding.OwnerInputs = ownerInputs
		binding, err = contract.SealSingleCallHarnessApplicationBindingCoordinateV2(binding)
		if err != nil {
			t.Fatal(err)
		}
		subject.Binding = binding
		if _, err := contract.SealSingleCallPendingActionSubjectCoordinateV2(subject); err == nil {
			t.Fatal("same-scope different Run operation splice accepted")
		}
	})
	t.Run("operation_attempt_digest", func(t *testing.T) {
		subject := fx.request.Action.PendingSubject
		base := subject.Binding.Base
		base.ModelTurnSettlement.Attempt.OperationDigest = core.DigestBytes([]byte("another operation"))
		base, err := contract.SealSingleCallHarnessBaseBindingCoordinateV2(base)
		if err != nil {
			t.Fatal(err)
		}
		binding := subject.Binding
		binding.Base = base
		binding, err = contract.SealSingleCallHarnessApplicationBindingCoordinateV2(binding)
		if err != nil {
			t.Fatal(err)
		}
		subject.Binding = binding
		if _, err := contract.SealSingleCallPendingActionSubjectCoordinateV2(subject); err == nil {
			t.Fatal("Model operation/Settlement attempt digest splice accepted")
		}
	})
	t.Run("route_matrix", func(t *testing.T) {
		ownerInputs := fx.request.Action.PendingSubject.Binding.OwnerInputs
		ownerInputs.RouteMatrix = runtimeports.OperationScopeEvidenceApplicabilityMatrixKeyV3{OperationKind: runtimeports.OperationScopeActivationV3, EffectKind: "praxis.sandbox/allocate", PolicyProfile: runtimeports.OperationScopeEvidenceActivationProfileV3}
		if _, err := contract.SealSingleCallHarnessOwnerCurrentInputsCoordinateV2(ownerInputs); err == nil {
			t.Fatal("non-Action RouteMatrix accepted")
		}
	})
	t.Run("route_current_matrix_digest", func(t *testing.T) {
		ownerInputs := fx.request.Action.PendingSubject.Binding.OwnerInputs
		route := ownerInputs.RouteCurrent
		route.MatrixDigest = core.DigestBytes([]byte("another route matrix"))
		currentID, err := runtimeports.DeriveControlledOperationProviderRouteCurrentIDV2(route.DeclarationRef.RouteID, route.MatrixDigest)
		if err != nil {
			t.Fatal(err)
		}
		route.CurrentID = currentID
		route.Digest = ""
		route, err = runtimeports.SealControlledOperationProviderRouteCurrentRefV2(route)
		if err != nil {
			t.Fatal(err)
		}
		ownerInputs.RouteCurrent = route
		if _, err := contract.SealSingleCallHarnessOwnerCurrentInputsCoordinateV2(ownerInputs); err == nil {
			t.Fatal("RouteCurrent with another MatrixDigest accepted")
		}
	})
}

func TestSingleCallToolActionIdentityCurrentExactRequestAndDomainResultV2(t *testing.T) {
	fx := newSingleCallFixtureV2(t)
	subject := fx.request.Action.PendingSubject
	expected, err := contract.SealSingleCallModelPendingActionIdentityCurrentRequestV2(contract.SingleCallModelPendingActionIdentityCurrentRequestV2{Run: subject.Run, SessionID: subject.SessionID, Turn: subject.Turn, IdentityRef: subject.Binding.Base.IdentityRef, DomainResultFact: subject.Binding.Base.DomainResultFact, RequestedNotAfterUnixNano: fx.request.ExpiresUnixNano})
	if err != nil {
		t.Fatal(err)
	}
	if err := fx.inputs.projection.HarnessCurrent.IdentityCurrent.ValidateFor(expected, fx.now); err != nil {
		t.Fatal(err)
	}

	for name, mutate := range map[string]func(*testing.T, *contract.SingleCallModelPendingActionIdentityCurrentRequestV2){
		"run": func(t *testing.T, request *contract.SingleCallModelPendingActionIdentityCurrentRequestV2) {
			request.Run.RunID = "another-run-v2"
			sealed, err := contract.SealSingleCallRunSubjectCoordinateV2(request.Run)
			if err != nil {
				t.Fatal(err)
			}
			request.Run = sealed
		},
		"session": func(_ *testing.T, request *contract.SingleCallModelPendingActionIdentityCurrentRequestV2) {
			request.SessionID = "another-session-v2"
		},
		"turn": func(_ *testing.T, request *contract.SingleCallModelPendingActionIdentityCurrentRequestV2) {
			request.Turn++
		},
	} {
		t.Run(name, func(t *testing.T) {
			wrongExpected := expected
			mutate(t, &wrongExpected)
			wrongExpected, err = contract.SealSingleCallModelPendingActionIdentityCurrentRequestV2(wrongExpected)
			if err != nil {
				t.Fatal(err)
			}
			if err := fx.inputs.projection.HarnessCurrent.IdentityCurrent.ValidateFor(wrongExpected, fx.now); err == nil {
				t.Fatal("Identity Current accepted another exact reader subject")
			}
		})
	}

	t.Run("request_digest", func(t *testing.T) {
		wrongExpected := expected
		wrongExpected.RequestedNotAfterUnixNano--
		wrongExpected, err := contract.SealSingleCallModelPendingActionIdentityCurrentRequestV2(wrongExpected)
		if err != nil {
			t.Fatal(err)
		}
		current := fx.inputs.projection.HarnessCurrent.IdentityCurrent
		current, err = contract.SealSingleCallModelPendingActionIdentityCurrentV2(current, wrongExpected, fx.now)
		if err != nil {
			t.Fatal(err)
		}
		projection := contract.CloneSingleCallToolActionInputCurrentProjectionV2(fx.inputs.projection)
		projection.HarnessCurrent.IdentityCurrent = current
		projection.HarnessCurrent, err = contract.SealSingleCallHarnessOwnerCurrentProofV3(projection.HarnessCurrent, fx.now)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := contract.SealSingleCallToolActionInputCurrentProjectionV2(projection, fx.request, fx.now); err == nil {
			t.Fatal("Identity Current from another exact Reader Request accepted")
		}
	})

	t.Run("domain_result_fact", func(t *testing.T) {
		wrongExpected := expected
		wrongExpected.DomainResultFact.FactDigest = core.DigestBytes([]byte("another domain result fact"))
		wrongExpected, err := contract.SealSingleCallModelPendingActionIdentityCurrentRequestV2(wrongExpected)
		if err != nil {
			t.Fatal(err)
		}
		current := fx.inputs.projection.HarnessCurrent.IdentityCurrent
		current.DomainResultFact = wrongExpected.DomainResultFact
		current, err = contract.SealSingleCallModelPendingActionIdentityCurrentV2(current, wrongExpected, fx.now)
		if err != nil {
			t.Fatal(err)
		}
		harness := fx.inputs.projection.HarnessCurrent
		harness.IdentityCurrent = current
		if _, err := contract.SealSingleCallHarnessOwnerCurrentProofV3(harness, fx.now); err == nil {
			t.Fatal("complete DomainResult FactRef splice accepted")
		}
	})
}

func TestSingleCallToolActionVersionClaimAtomicAndCrossVersionConflictV2(t *testing.T) {
	fx := newSingleCallFixtureV2(t)
	prepared := mustPreparedFactV2(t, fx.request)
	created, err := fx.store.CreateSingleCallToolActionCoordinationV2(context.Background(), prepared)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := fx.store.InspectSingleCallToolActionVersionClaimForTestV1(fx.request.Action.ExecutionScope, fx.request.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := claim.ValidateFor(created); err != nil {
		t.Fatalf("atomic VersionClaim does not bind initial Fact: %v", err)
	}
	wrongVersion := claim
	wrongVersion.ClaimedActionVersion = contract.SingleCallToolActionContractVersionV1
	if err := wrongVersion.ValidateFor(created); err == nil {
		t.Fatal("V1 claim type-punned as a V2 initial claim")
	}
	unknownVersion := claim
	unknownVersion.ClaimedActionVersion = "praxis.application.single-call-tool-action/v999"
	if _, err := contract.SealSingleCallToolActionVersionClaimV1(unknownVersion); err == nil {
		t.Fatal("unknown action version was sealed into VersionClaim")
	}
	keyMutations := map[string]func(*contract.SingleCallToolActionCrossVersionConflictKeyV1){
		"scope": func(v *contract.SingleCallToolActionCrossVersionConflictKeyV1) {
			v.ExecutionScopeDigest = core.DigestBytes([]byte("other scope"))
		},
		"run":            func(v *contract.SingleCallToolActionCrossVersionConflictKeyV1) { v.RunID = "other-run" },
		"session":        func(v *contract.SingleCallToolActionCrossVersionConflictKeyV1) { v.SessionID = "other-session" },
		"turn":           func(v *contract.SingleCallToolActionCrossVersionConflictKeyV1) { v.Turn++ },
		"pending_action": func(v *contract.SingleCallToolActionCrossVersionConflictKeyV1) { v.PendingActionRef = "other-action" },
		"request_digest": func(v *contract.SingleCallToolActionCrossVersionConflictKeyV1) {
			v.PendingActionRequestDigest = core.DigestBytes([]byte("other pending request"))
		},
	}
	for name, mutate := range keyMutations {
		t.Run("conflict_key_"+name, func(t *testing.T) {
			badKey := claim.ConflictKey
			mutate(&badKey)
			badKey.Digest = ""
			digest, err := core.CanonicalJSONDigest("praxis.application.single-call-tool-action-cross-version-key-v1", "2.0.0", "SingleCallToolActionCrossVersionConflictKeyV1", badKey)
			if err != nil {
				t.Fatal(err)
			}
			badKey.Digest = digest
			badClaim := claim
			badClaim.ConflictKey = badKey
			badClaim, err = contract.SealSingleCallToolActionVersionClaimV1(badClaim)
			if err != nil {
				t.Fatal(err)
			}
			if err := badClaim.ValidateFor(created); err == nil {
				t.Fatal("spliced ConflictKey still bound initial Fact")
			}
		})
	}
	conflictStore := fakes.NewSingleCallToolActionCoordinationStoreV2()
	conflictKey, err := contract.DeriveSingleCallToolActionCrossVersionConflictKeyV1(fx.request)
	if err != nil {
		t.Fatal(err)
	}
	v1Claim, err := contract.SealSingleCallToolActionVersionClaimV1(contract.SingleCallToolActionVersionClaimV1{
		ConflictKey:          conflictKey,
		ClaimedActionVersion: contract.SingleCallToolActionContractVersionV1,
		CoordinationID:       "single-call-v1-coordination",
		CoordinationDigest:   core.DigestBytes([]byte("single-call-v1-coordination")),
		CreatedUnixNano:      fx.request.CreatedUnixNano,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := conflictStore.OccupySingleCallToolActionVersionClaimForTestV1(v1Claim); err != nil {
		t.Fatal(err)
	}
	if _, err := conflictStore.CreateSingleCallToolActionCoordinationV2(context.Background(), prepared); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("V1 semantic claim did not block V2 create: %v", err)
	}
	if creates, cas := conflictStore.Counts(); creates != 0 || cas != 0 {
		t.Fatalf("V1/V2 conflict wrote V2 state: create=%d cas=%d", creates, cas)
	}
	changedRequest := fx.request
	changedRequest.ExpiresUnixNano--
	changedRequest, err = contract.SealSingleCallToolActionRequestV2(changedRequest)
	if err != nil {
		t.Fatal(err)
	}
	if changedRequest.ID != fx.request.ID || changedRequest.Digest == fx.request.Digest {
		t.Fatal("changed-content fixture did not retain the stable semantic ID")
	}
	changedFact := mustPreparedFactV2(t, changedRequest)
	if _, err := fx.store.CreateSingleCallToolActionCoordinationV2(context.Background(), changedFact); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("same semantic key with changed content was not rejected: %v", err)
	}
	inspected, err := fx.store.InspectSingleCallToolActionCoordinationV2(context.Background(), fx.request.Action.ExecutionScope, fx.request.ID)
	if err != nil || inspected.Digest != prepared.Digest {
		t.Fatalf("conflict changed the linearized initial Fact: %v", err)
	}
}

func TestSingleCallToolActionFakeV2TypedNilAndDeepClone(t *testing.T) {
	fx := newSingleCallFixtureV2(t)
	prepared := mustPreparedFactV2(t, fx.request)
	var nilStore *fakes.SingleCallToolActionCoordinationStoreV2
	if _, err := nilStore.CreateSingleCallToolActionCoordinationV2(context.Background(), prepared); !core.HasCategory(err, core.ErrorInvalidArgument) {
		t.Fatalf("typed-nil Create did not fail closed: %v", err)
	}
	if _, err := nilStore.InspectSingleCallToolActionCoordinationV2(context.Background(), fx.request.Action.ExecutionScope, fx.request.ID); !core.HasCategory(err, core.ErrorInvalidArgument) {
		t.Fatalf("typed-nil Inspect did not fail closed: %v", err)
	}
	invalid := prepared
	invalid.Request.Action.PendingSubject.Binding.Base.ModelTurnSettlement.Evidence = append([]runtimeports.EvidenceRecordRefV2(nil), prepared.Request.Action.PendingSubject.Binding.Base.ModelTurnSettlement.Evidence...)
	invalid.Request.Action.PendingSubject.Binding.Base.ModelTurnSettlement.Evidence[0].RecordDigest = core.DigestBytes([]byte("invalid before clone"))
	if _, err := fx.store.CreateSingleCallToolActionCoordinationV2(context.Background(), invalid); err == nil {
		t.Fatal("invalid nested input reached the store clone path")
	}
	if creates, cas := fx.store.Counts(); creates != 0 || cas != 0 {
		t.Fatalf("invalid nested input wrote state: create=%d cas=%d", creates, cas)
	}
	created, err := fx.store.CreateSingleCallToolActionCoordinationV2(context.Background(), prepared)
	if err != nil {
		t.Fatal(err)
	}
	original := created.Request.Action.PendingSubject.Binding.Base.ModelTurnSettlement.Evidence[0].RecordDigest
	created.Request.Action.PendingSubject.Binding.Base.ModelTurnSettlement.Evidence[0].RecordDigest = core.DigestBytes([]byte("caller mutation"))
	inspected, err := fx.store.InspectSingleCallToolActionCoordinationV2(context.Background(), fx.request.Action.ExecutionScope, fx.request.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := inspected.Request.Action.PendingSubject.Binding.Base.ModelTurnSettlement.Evidence[0].RecordDigest; got != original {
		t.Fatal("store aliased caller-owned nested Evidence slice")
	}
	dispatch := mustNextFactV2(t, inspected, contract.SingleCallToolActionDispatchIntentV2, nil, fx.now)
	cas := mustCASV2(t, inspected, dispatch)
	if _, err := nilStore.CompareAndSwapSingleCallToolActionCoordinationV2(context.Background(), cas); !core.HasCategory(err, core.ErrorInvalidArgument) {
		t.Fatalf("typed-nil CAS did not fail closed: %v", err)
	}
}

func TestSingleCallToolActionCASV2SameNextReplayConflicts(t *testing.T) {
	fx := newSingleCallFixtureV2(t)
	prepared := mustPreparedFactV2(t, fx.request)
	if _, err := fx.store.CreateSingleCallToolActionCoordinationV2(context.Background(), prepared); err != nil {
		t.Fatal(err)
	}
	dispatch := mustNextFactV2(t, prepared, contract.SingleCallToolActionDispatchIntentV2, nil, fx.now)
	request := mustCASV2(t, prepared, dispatch)
	if _, err := fx.store.CompareAndSwapSingleCallToolActionCoordinationV2(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.store.CompareAndSwapSingleCallToolActionCoordinationV2(context.Background(), request); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("same-Next replay bypassed Expected revision/digest: %v", err)
	}
	creates, cas := fx.store.Counts()
	if creates != 1 || cas != 1 {
		t.Fatalf("same-Next replay linearized twice: create=%d cas=%d", creates, cas)
	}
}

func TestSingleCallToolActionCoordinatorV2RejectsNilContext(t *testing.T) {
	fx := newSingleCallFixtureV2(t)
	if _, err := fx.coordinator.CoordinateSingleCallToolActionV2(nil, fx.request); !core.HasCategory(err, core.ErrorInvalidArgument) {
		t.Fatalf("nil context did not fail closed: %v", err)
	}
	if fx.tool.executeCalls != 0 {
		t.Fatal("nil context reached Tool execute")
	}
}

func TestSingleCallToolActionFakeV2RejectsNilContext(t *testing.T) {
	fx := newSingleCallFixtureV2(t)
	prepared := mustPreparedFactV2(t, fx.request)
	if _, err := fx.store.CreateSingleCallToolActionCoordinationV2(nil, prepared); !core.HasCategory(err, core.ErrorInvalidArgument) {
		t.Fatalf("nil Create context accepted: %v", err)
	}
	if _, err := fx.store.InspectSingleCallToolActionCoordinationV2(nil, fx.request.Action.ExecutionScope, fx.request.ID); !core.HasCategory(err, core.ErrorInvalidArgument) {
		t.Fatalf("nil Inspect context accepted: %v", err)
	}
	dispatch := mustNextFactV2(t, prepared, contract.SingleCallToolActionDispatchIntentV2, nil, fx.now)
	if _, err := fx.store.CompareAndSwapSingleCallToolActionCoordinationV2(nil, mustCASV2(t, prepared, dispatch)); !core.HasCategory(err, core.ErrorInvalidArgument) {
		t.Fatalf("nil CAS context accepted: %v", err)
	}
	if creates, cas := fx.store.Counts(); creates != 0 || cas != 0 {
		t.Fatalf("nil context wrote state: create=%d cas=%d", creates, cas)
	}
}

func TestSingleCallToolActionCoordinatorV2FaultRecovery(t *testing.T) {
	tests := []struct {
		name               string
		setup              func(*singleCallFixtureV2)
		wantError          bool
		wantExecute        int
		wantInspectAtLeast int
	}{
		{name: "lost_create_reply", setup: func(f *singleCallFixtureV2) {
			if err := f.store.LoseNextCreateReplyForTestV2(); err != nil {
				t.Fatal(err)
			}
		}, wantExecute: 1},
		{name: "lost_execute_reply", setup: func(f *singleCallFixtureV2) { f.tool.loseExecute = true }, wantExecute: 1, wantInspectAtLeast: 1},
		{name: "lost_settlement_reply", setup: func(f *singleCallFixtureV2) { f.settlements.loseCurrent = true }, wantError: true, wantExecute: 1},
		{name: "lost_association_reply", setup: func(f *singleCallFixtureV2) { f.settlements.loseAssociation = true }, wantError: true, wantExecute: 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fx := newSingleCallFixtureV2(t)
			tc.setup(fx)
			_, err := fx.coordinator.CoordinateSingleCallToolActionV2(context.Background(), fx.request)
			if tc.wantError && err == nil {
				t.Fatal("fault unexpectedly returned success")
			}
			if !tc.wantError && err != nil {
				t.Fatal(err)
			}
			if fx.tool.executeCalls != tc.wantExecute || fx.tool.inspectCalls < tc.wantInspectAtLeast {
				t.Fatalf("execute=%d inspect=%d", fx.tool.executeCalls, fx.tool.inspectCalls)
			}
			if tc.wantError {
				got, replayErr := fx.coordinator.CoordinateSingleCallToolActionV2(context.Background(), fx.request)
				if replayErr != nil || got.Digest != fx.result.Digest {
					t.Fatalf("inspect-only replay failed: %v", replayErr)
				}
				if fx.tool.executeCalls != 1 {
					t.Fatal("recovery redispatched Tool")
				}
			}
		})
	}
}

func TestSingleCallToolActionStartClaimLostReplyIsInspectOnlyV2(t *testing.T) {
	fx := newSingleCallFixtureV2(t)
	facts := &loseNthCASFactPortV2{SingleCallToolActionCoordinationFactPortV2: fx.store, loseAt: 2}
	coordinator, err := application.NewSingleCallToolActionCoordinatorV2(application.SingleCallToolActionCoordinatorConfigV2{Facts: facts, Tool: fx.tool, Inputs: fx.inputs, Settlements: fx.settlements, Clock: func() time.Time { return fx.now }})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = coordinator.CoordinateSingleCallToolActionV2(context.Background(), fx.request); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("lost StartClaim reply should inspect only: %v", err)
	}
	if fx.tool.executeCalls != 0 || fx.tool.inspectCalls == 0 {
		t.Fatalf("execute=%d inspect=%d", fx.tool.executeCalls, fx.tool.inspectCalls)
	}
	if creates, cas := fx.store.Counts(); creates != 1 || cas != 2 {
		t.Fatalf("lost committed StartClaim changed commit counts: create=%d cas=%d", creates, cas)
	}
}

func TestSingleCallToolActionStartClaimNoCommitFailuresRemainInspectOnlyV2(t *testing.T) {
	tests := []struct {
		name        string
		failure     error
		loseInspect bool
	}{
		{name: "conflict", failure: core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "injected StartClaim conflict")},
		{name: "unavailable", failure: core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected StartClaim unavailable")},
		{name: "indeterminate", failure: core.NewError(core.ErrorIndeterminate, core.ReasonEvidenceUnavailable, "injected StartClaim indeterminate")},
		{name: "inspect_reply_loss", failure: core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected StartClaim unavailable"), loseInspect: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fx := newSingleCallFixtureV2(t)
			if tc.loseInspect {
				if err := fx.store.LoseNextInspectReplyForTestV2(); err != nil {
					t.Fatal(err)
				}
			}
			facts := &failNthCASFactPortV2{SingleCallToolActionCoordinationFactPortV2: fx.store, failAt: 2, failure: tc.failure}
			coordinator, err := application.NewSingleCallToolActionCoordinatorV2(application.SingleCallToolActionCoordinatorConfigV2{Facts: facts, Tool: fx.tool, Inputs: fx.inputs, Settlements: fx.settlements, Clock: func() time.Time { return fx.now }})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := coordinator.CoordinateSingleCallToolActionV2(context.Background(), fx.request); err == nil {
				t.Fatal("uncommitted StartClaim failure unexpectedly succeeded")
			}
			fact, err := fx.store.InspectSingleCallToolActionCoordinationV2(context.Background(), fx.request.Action.ExecutionScope, fx.request.ID)
			if err != nil {
				t.Fatal(err)
			}
			if fact.State != contract.SingleCallToolActionDispatchIntentV2 {
				t.Fatalf("authoritative state=%s, want dispatch_intent", fact.State)
			}
			restarted, err := application.NewSingleCallToolActionCoordinatorV2(application.SingleCallToolActionCoordinatorConfigV2{Facts: fx.store, Tool: fx.tool, Inputs: fx.inputs, Settlements: fx.settlements, Clock: func() time.Time { return fx.now }})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := restarted.CoordinateSingleCallToolActionV2(context.Background(), fx.request); !core.HasCategory(err, core.ErrorNotFound) {
				t.Fatalf("restart did not remain Inspect-only: %v", err)
			}
			if fx.tool.executeCalls != 0 {
				t.Fatalf("uncommitted StartClaim failure executed Tool %d times", fx.tool.executeCalls)
			}
			if creates, cas := fx.store.Counts(); creates != 1 || cas != 1 {
				t.Fatalf("uncommitted StartClaim/restart changed commits: create=%d cas=%d", creates, cas)
			}
		})
	}
}

func TestSingleCallToolActionInputS2DriftFailsBeforeExecuteV2(t *testing.T) {
	fx := newSingleCallFixtureV2(t)
	inputs := &driftOnSecondInputV2{projection: fx.inputs.projection}
	coordinator, err := application.NewSingleCallToolActionCoordinatorV2(application.SingleCallToolActionCoordinatorConfigV2{Facts: fx.store, Tool: fx.tool, Inputs: inputs, Settlements: fx.settlements, Clock: func() time.Time { return fx.now }})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := coordinator.CoordinateSingleCallToolActionV2(context.Background(), fx.request); err == nil {
		t.Fatal("S2 current drift was accepted")
	}
	if fx.tool.executeCalls != 0 {
		t.Fatal("S2 drift reached Tool execute")
	}
}

func TestSingleCallToolActionClockRegressionAndTTLCrossingV2(t *testing.T) {
	t.Run("clock_regression", func(t *testing.T) {
		fx := newSingleCallFixtureV2(t)
		var mu sync.Mutex
		calls := 0
		clock := func() time.Time {
			mu.Lock()
			defer mu.Unlock()
			calls++
			if calls == 1 {
				return fx.now
			}
			return fx.now.Add(-time.Nanosecond)
		}
		coordinator, err := application.NewSingleCallToolActionCoordinatorV2(application.SingleCallToolActionCoordinatorConfigV2{Facts: fx.store, Tool: fx.tool, Inputs: fx.inputs, Settlements: fx.settlements, Clock: clock})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := coordinator.CoordinateSingleCallToolActionV2(context.Background(), fx.request); !core.HasReason(err, core.ReasonClockRegression) {
			t.Fatalf("clock regression was not rejected: %v", err)
		}
		if fx.tool.executeCalls != 0 {
			t.Fatal("clock regression reached Tool execute")
		}
	})

	t.Run("ttl_crosses_during_execute", func(t *testing.T) {
		fx := newSingleCallFixtureV2(t)
		var mu sync.Mutex
		current := fx.now
		fx.tool.onExecute = func() {
			mu.Lock()
			current = time.Unix(0, fx.request.ExpiresUnixNano)
			mu.Unlock()
		}
		clock := func() time.Time {
			mu.Lock()
			defer mu.Unlock()
			return current
		}
		coordinator, err := application.NewSingleCallToolActionCoordinatorV2(application.SingleCallToolActionCoordinatorConfigV2{Facts: fx.store, Tool: fx.tool, Inputs: fx.inputs, Settlements: fx.settlements, Clock: clock})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := coordinator.CoordinateSingleCallToolActionV2(context.Background(), fx.request); !core.HasCategory(err, core.ErrorPreconditionFailed) {
			t.Fatalf("TTL crossing was not rejected: %v", err)
		}
		if fx.tool.executeCalls != 1 {
			t.Fatalf("execute calls=%d", fx.tool.executeCalls)
		}
		fact, err := fx.store.InspectSingleCallToolActionCoordinationV2(context.Background(), fx.request.Action.ExecutionScope, fx.request.ID)
		if err != nil {
			t.Fatal(err)
		}
		if fact.State != contract.SingleCallToolActionWaitingInspectV2 {
			t.Fatalf("TTL crossing incorrectly completed fact: %s", fact.State)
		}
	})
}

type driftOnSecondInputV2 struct {
	mu         sync.Mutex
	projection contract.SingleCallToolActionInputCurrentProjectionV2
	calls      int
}

func (r *driftOnSecondInputV2) InspectSingleCallToolActionInputCurrentV2(_ context.Context, _ contract.SingleCallToolActionRequestV2) (contract.SingleCallToolActionInputCurrentProjectionV2, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	projection := contract.CloneSingleCallToolActionInputCurrentProjectionV2(r.projection)
	if r.calls >= 2 {
		projection.HarnessCurrent.IdentityCurrent.Projection.CallName = "drifted-tool"
	}
	return projection, nil
}

type loseNthCASFactPortV2 struct {
	applicationports.SingleCallToolActionCoordinationFactPortV2
	mu     sync.Mutex
	calls  int
	loseAt int
}

type failNthCASFactPortV2 struct {
	applicationports.SingleCallToolActionCoordinationFactPortV2
	mu      sync.Mutex
	calls   int
	failAt  int
	failure error
}

func (p *failNthCASFactPortV2) CompareAndSwapSingleCallToolActionCoordinationV2(ctx context.Context, request applicationports.SingleCallToolActionCoordinationCASRequestV2) (contract.SingleCallToolActionCoordinationFactV2, error) {
	p.mu.Lock()
	p.calls++
	call := p.calls
	p.mu.Unlock()
	if call == p.failAt {
		return contract.SingleCallToolActionCoordinationFactV2{}, p.failure
	}
	return p.SingleCallToolActionCoordinationFactPortV2.CompareAndSwapSingleCallToolActionCoordinationV2(ctx, request)
}

func (p *loseNthCASFactPortV2) CompareAndSwapSingleCallToolActionCoordinationV2(ctx context.Context, request applicationports.SingleCallToolActionCoordinationCASRequestV2) (contract.SingleCallToolActionCoordinationFactV2, error) {
	p.mu.Lock()
	p.calls++
	call := p.calls
	p.mu.Unlock()
	stored, err := p.SingleCallToolActionCoordinationFactPortV2.CompareAndSwapSingleCallToolActionCoordinationV2(ctx, request)
	if err == nil && call == p.loseAt {
		return contract.SingleCallToolActionCoordinationFactV2{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "lost CAS reply")
	}
	return stored, err
}

func mustPreparedFactV2(t *testing.T, request contract.SingleCallToolActionRequestV2) contract.SingleCallToolActionCoordinationFactV2 {
	t.Helper()
	fact, err := contract.NewSingleCallToolActionCoordinationFactV2(request)
	if err != nil {
		t.Fatal(err)
	}
	return fact
}

func mustNextFactV2(t *testing.T, current contract.SingleCallToolActionCoordinationFactV2, state contract.SingleCallToolActionCoordinationStateV2, result *contract.SingleCallToolActionResultRefV2, now time.Time) contract.SingleCallToolActionCoordinationFactV2 {
	t.Helper()
	next, err := contract.NextSingleCallToolActionCoordinationFactV2(current, state, result, now)
	if err != nil {
		t.Fatal(err)
	}
	return next
}

func mustCASV2(t *testing.T, current, next contract.SingleCallToolActionCoordinationFactV2) applicationports.SingleCallToolActionCoordinationCASRequestV2 {
	t.Helper()
	request, err := applicationports.SealSingleCallToolActionCoordinationCASRequestV2(applicationports.SingleCallToolActionCoordinationCASRequestV2{Scope: current.Request.Action.ExecutionScope, ID: current.ID, ExpectedRevision: current.Revision, ExpectedDigest: current.Digest, Next: next})
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func mustCompletedSuccessorCASV2(t *testing.T, current, next contract.SingleCallToolActionCoordinationFactV2) applicationports.SingleCallToolActionCoordinationCASRequestV2 {
	t.Helper()
	request, err := applicationports.SealSingleCallToolActionCoordinationCASRequestV2(applicationports.SingleCallToolActionCoordinationCASRequestV2{Scope: current.Request.Action.ExecutionScope, ID: current.ID, ExpectedRevision: current.Revision, ExpectedDigest: current.Digest, Next: next})
	if err != nil {
		t.Fatal(err)
	}
	return request
}

var _ applicationports.SingleCallToolActionPortV2 = (*singleCallToolV2)(nil)
