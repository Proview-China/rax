package applicationadapter_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/action"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/applicationadapter"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/runtimeadapter"
)

type echoOwnerCurrent struct{}

func (echoOwnerCurrent) InspectOwnerCurrentV1(_ context.Context, value toolcontract.OwnerCurrentRefV1) (toolcontract.OwnerCurrentRefV1, error) {
	return value, nil
}

type echoObservationExact struct{}

func (echoObservationExact) InspectProviderObservationExactV1(_ context.Context, value runtimeports.ProviderAttemptObservationRefV2) (runtimeports.ProviderAttemptObservationRefV2, error) {
	return value, nil
}

type echoEnforcementExact struct{}

func (echoEnforcementExact) InspectEnforcementPhaseExactV1(_ context.Context, value runtimeports.OperationDispatchEnforcementPhaseRefV4) (runtimeports.OperationDispatchEnforcementPhaseRefV4, error) {
	return value, nil
}

type echoConsumptionExact struct{}

func (echoConsumptionExact) InspectEvidenceConsumptionExactV1(_ context.Context, value runtimeports.OperationScopeEvidenceConsumptionRefV3) (runtimeports.OperationScopeEvidenceConsumptionRefV3, error) {
	return value, nil
}

type ownerPlanReader struct {
	plan applicationadapter.SingleCallToolOwnerPlanV1
}

func (r *ownerPlanReader) InspectSingleCallToolOwnerPlanV1(_ context.Context, _ applicationadapter.ToolOwnerSingleCallExecutionV1) (applicationadapter.SingleCallToolOwnerPlanV1, error) {
	return r.plan, nil
}

type countingEnforcementReader struct {
	value runtimeports.OperationDispatchEnforcementPhaseRefV4
	calls atomic.Int32
}

func (r *countingEnforcementReader) InspectCurrentOperationProviderExecuteEnforcementV1(context.Context, runtimeports.OperationSubjectV3, runtimeports.OperationDispatchEnforcementPhaseRefV4) (runtimeports.OperationDispatchEnforcementPhaseRefV4, error) {
	r.calls.Add(1)
	return r.value, nil
}

type providerObservationStore struct {
	mu    sync.RWMutex
	value applicationadapter.SingleCallToolProviderInspectionV1
	ready bool
}

func (s *providerObservationStore) InspectSingleCallToolProviderObservationV1(_ context.Context, _ runtimeports.OperationDispatchAttemptRefV3) (applicationadapter.SingleCallToolProviderInspectionV1, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.ready {
		return applicationadapter.SingleCallToolProviderInspectionV1{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "Provider observation unavailable")
	}
	return s.value, nil
}

type controlledProvider struct {
	observations        *providerObservationStore
	value               applicationadapter.SingleCallToolProviderInspectionV1
	calls               atomic.Int32
	lostReply           atomic.Bool
	entered             chan struct{}
	release             chan struct{}
	active              atomic.Int32
	maximum             atomic.Int32
	suppressObservation atomic.Bool
}

func (p *controlledProvider) CallControlledOperationProviderV1(_ context.Context, request runtimeports.ControlledOperationProviderCallRequestV1) error {
	if err := request.Validate(); err != nil {
		return err
	}
	p.calls.Add(1)
	active := p.active.Add(1)
	for current := p.maximum.Load(); active > current && !p.maximum.CompareAndSwap(current, active); current = p.maximum.Load() {
	}
	if p.entered != nil {
		p.entered <- struct{}{}
		<-p.release
	}
	if !p.suppressObservation.Load() {
		p.publishObservation()
	}
	p.active.Add(-1)
	if p.lostReply.Load() {
		return core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "Provider reply lost")
	}
	return nil
}

func (p *controlledProvider) publishObservation() {
	p.observations.mu.Lock()
	p.observations.value = p.value
	p.observations.ready = true
	p.observations.mu.Unlock()
}

type settlementGateway struct {
	mu          sync.RWMutex
	now         time.Time
	bundle      runtimeports.OperationSettlementCommitBundleV4
	inspection  runtimeports.OperationInspectionSettlementRefV4
	settleCalls atomic.Int32
	lostReply   atomic.Bool
}

func (g *settlementGateway) SettleOperationV4(_ context.Context, submission runtimeports.OperationSettlementSubmissionV4) (runtimeports.OperationSettlementRefV4, error) {
	if err := submission.Validate(); err != nil {
		return runtimeports.OperationSettlementRefV4{}, err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.inspection.Digest != "" {
		return g.inspection.Settlement, nil
	}
	g.settleCalls.Add(1)
	fact, err := runtimeports.SealOperationSettlementFactV4(runtimeports.OperationSettlementFactV4{Submission: submission})
	if err != nil {
		return runtimeports.OperationSettlementRefV4{}, err
	}
	ref := fact.RefV4()
	association, err := runtimeports.SealOperationSettlementEvidenceAssociationV4(runtimeports.OperationSettlementEvidenceAssociationV4{ID: "association-real-flow", Settlement: ref, Prepare: submission.Evidence[0], Execute: submission.Evidence[1]})
	if err != nil {
		return runtimeports.OperationSettlementRefV4{}, err
	}
	guard, err := runtimeports.SealOperationSettlementTerminalGuardV4(runtimeports.OperationSettlementTerminalGuardV4{ID: "guard-real-flow", TenantID: submission.TenantID, OperationDigest: submission.OperationDigest, EffectID: submission.EffectID, Settlement: ref})
	if err != nil {
		return runtimeports.OperationSettlementRefV4{}, err
	}
	projection, err := runtimeports.SealOperationSettlementTerminalProjectionV4(runtimeports.OperationSettlementTerminalProjectionV4{ID: "projection-real-flow", TenantID: submission.TenantID, OperationDigest: submission.OperationDigest, EffectID: submission.EffectID, Settlement: ref, Association: association.RefV4(), Guard: guard.RefV4(), DomainResult: submission.DomainResult})
	if err != nil {
		return runtimeports.OperationSettlementRefV4{}, err
	}
	inspection, err := runtimeports.SealOperationInspectionSettlementRefV4(runtimeports.OperationInspectionSettlementRefV4{Settlement: ref, Association: association.RefV4(), Guard: guard.RefV4(), Projection: projection.RefV4(), DomainResult: submission.DomainResult, EffectFactRevision: submission.ExpectedEffectRevision + 1, Owner: submission.Owner, CheckedUnixNano: g.now.UnixNano(), ExpiresUnixNano: g.now.Add(6 * time.Second).UnixNano()}, g.now)
	if err != nil {
		return runtimeports.OperationSettlementRefV4{}, err
	}
	g.bundle = runtimeports.OperationSettlementCommitBundleV4{Settlement: fact, Association: association, Guard: guard, Projection: projection}
	g.inspection = inspection
	if g.lostReply.Load() {
		return runtimeports.OperationSettlementRefV4{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "settlement reply lost")
	}
	return ref, nil
}

func (g *settlementGateway) InspectCurrentOperationSettlementV4(_ context.Context, request runtimeports.InspectCurrentOperationSettlementRequestV4) (runtimeports.OperationInspectionSettlementRefV4, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if g.inspection.Digest == "" {
		return runtimeports.OperationInspectionSettlementRefV4{}, core.NewError(core.ErrorNotFound, core.ReasonEffectSettlementMissing, "settlement not found")
	}
	if request.EffectID != g.inspection.Settlement.EffectID || !runtimeports.SameOperationSubjectV3(request.Operation, g.inspection.DomainResult.Operation) {
		return runtimeports.OperationInspectionSettlementRefV4{}, core.NewError(core.ErrorConflict, core.ReasonEffectSettlementMissing, "settlement key drifted")
	}
	return g.inspection, nil
}

func (g *settlementGateway) InspectOperationSettlementEvidenceAssociationV4(_ context.Context, operation runtimeports.OperationSubjectV3, exact runtimeports.OperationSettlementEvidenceAssociationRefV4) (runtimeports.OperationSettlementEvidenceAssociationV4, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if g.bundle.Association.Digest == "" || !runtimeports.SameOperationSubjectV3(operation, g.bundle.Settlement.Submission.Operation) || !runtimeports.SameOperationSettlementEvidenceAssociationRefV4(exact, g.bundle.Association.RefV4()) {
		return runtimeports.OperationSettlementEvidenceAssociationV4{}, core.NewError(core.ErrorNotFound, core.ReasonEvidenceUnavailable, "association not found")
	}
	return g.bundle.Association, nil
}

func (g *settlementGateway) InspectOperationSettlementV4(context.Context, runtimeports.InspectOperationSettlementRequestV4) (runtimeports.OperationSettlementFactV4, error) {
	return g.bundle.Settlement, nil
}
func (g *settlementGateway) InspectOperationSettlementClosureV4(context.Context, runtimeports.InspectOperationSettlementRequestV4) (runtimeports.OperationSettlementCommitBundleV4, error) {
	return g.bundle, nil
}
func (g *settlementGateway) InspectOperationSettlementTerminalGuardV4(context.Context, runtimeports.OperationSubjectV3, runtimeports.OperationSettlementTerminalGuardRefV4) (runtimeports.OperationSettlementTerminalGuardV4, error) {
	return g.bundle.Guard, nil
}
func (g *settlementGateway) InspectOperationSettlementTerminalProjectionV4(context.Context, runtimeports.OperationSubjectV3, runtimeports.OperationSettlementTerminalProjectionRefV4) (runtimeports.OperationSettlementTerminalProjectionV4, error) {
	return g.bundle.Projection, nil
}

type realFlowFixture struct {
	adapter       *applicationadapter.SingleCallToolActionAdapterV1
	flow          *applicationadapter.ToolOwnerSingleCallFlowImplV1
	request       applicationcontract.SingleCallToolActionRequestV1
	provider      *controlledProvider
	settlements   *settlementGateway
	coordination  *action.CoordinationStoreV1
	facts         *action.StoreV2
	plans         *ownerPlanReader
	boundaryCalls *atomic.Int32
	model         *modelReader
	bindings      applicationadapter.SingleCallToolExactBindingsV1
	clock         applicationadapter.ClockV1
	observations  *providerObservationStore
}

func newRealFlowFixture(t *testing.T, providerLostReply, settlementLostReply bool) realFlowFixture {
	return newRealFlowFixtureMode(t, providerLostReply, settlementLostReply, "")
}

type mutableClock struct {
	mu  sync.RWMutex
	now time.Time
}

func (c *mutableClock) Now() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.now
}
func (c *mutableClock) Set(now time.Time) {
	c.mu.Lock()
	c.now = now
	c.mu.Unlock()
}

type mutatingEnforcementReader struct {
	value  runtimeports.OperationDispatchEnforcementPhaseRefV4
	mutate func()
	once   sync.Once
}

func (r *mutatingEnforcementReader) InspectCurrentOperationProviderExecuteEnforcementV1(context.Context, runtimeports.OperationSubjectV3, runtimeports.OperationDispatchEnforcementPhaseRefV4) (runtimeports.OperationDispatchEnforcementPhaseRefV4, error) {
	r.once.Do(r.mutate)
	return r.value, nil
}

type mutatingHandoffReader struct {
	value  runtimeports.OperationScopeEvidenceProviderHandoffFactV3
	mutate func()
	once   sync.Once
}

func (r *mutatingHandoffReader) InspectCurrentOperationProviderEvidenceHandoffV1(context.Context, runtimeports.OperationScopeEvidenceProviderHandoffRefV3) (runtimeports.OperationScopeEvidenceProviderHandoffFactV3, error) {
	r.once.Do(r.mutate)
	return r.value, nil
}

type mutatingBoundaryReader struct {
	base   runtimeports.OperationProviderBoundaryCurrentReaderV1
	mutate func()
}

func (r mutatingBoundaryReader) InspectCurrentOperationProviderBoundaryV1(ctx context.Context, exact runtimeports.OperationProviderBoundaryRefV1) (runtimeports.OperationProviderBoundaryCurrentProjectionV1, error) {
	projection, err := r.base.InspectCurrentOperationProviderBoundaryV1(ctx, exact)
	if err == nil {
		r.mutate()
	}
	return projection, err
}

func newRealFlowFixtureMode(t *testing.T, providerLostReply, settlementLostReply bool, actualPointMode string) realFlowFixture {
	t.Helper()
	now := testkit.FixedTime
	mutable := &mutableClock{now: now}
	var clock applicationadapter.ClockV1 = fixedClock{now}
	if actualPointMode != "" {
		clock = mutable
	}
	app := testkit.ApplicationG6AFixture(now)
	model := &modelReader{projection: app.Projection}
	boundary := testkit.BoundaryFixture(now)
	bindings := applicationadapter.SingleCallToolExactBindingsV1{ActionID: "action-real-flow", PendingAction: toolcontract.PendingActionExactRefV2{ID: app.Request.PendingAction.ActionRef, Revision: 1, RequestDigest: app.Request.PendingAction.RequestDigest}, Capability: toolcontract.ObjectRef{ID: string(app.Request.PendingAction.Capability), Revision: 1, Digest: testkit.Digest("capability-real-flow")}, Tool: toolcontract.ObjectRef{ID: "tool-real-flow", Revision: 1, Digest: testkit.Digest("tool-real-flow")}, SourceCandidate: toolcontract.ObjectRef{ID: app.Request.PendingAction.SourceCandidateID, Revision: app.Request.PendingAction.SourceCandidateRevision, Digest: app.Request.PendingAction.SourceCandidateDigest}, Provider: app.Provider}
	candidate := realFlowCandidate(t, now, app.Request, bindings)
	bindings.Candidate = candidate
	prepared := testkit.PreparedAttemptFor(now, boundary, app.Provider, candidate.InputSchema, candidate.Payload.ContentDigest, candidate.PayloadRevision)
	boundary.Enforcement.PreparedAttemptDigest = prepared.Digest
	if err := boundary.Enforcement.Validate(); err != nil {
		t.Fatal(err)
	}
	boundary.Handoff.Phase = boundary.Enforcement
	var err error
	boundary.Handoff, err = runtimeports.SealOperationScopeEvidenceProviderHandoffFactV3(boundary.Handoff)
	if err != nil {
		t.Fatal(err)
	}
	countingReader := &countingEnforcementReader{value: boundary.Enforcement}
	var executeReader runtimeports.OperationProviderExecuteEnforcementCurrentReaderV1 = countingReader
	var handoffCurrentReader runtimeports.OperationProviderEvidenceHandoffCurrentReaderV1 = handoffReader{boundary.Handoff}
	if actualPointMode == "enforcement-expiry" {
		executeReader = &mutatingEnforcementReader{value: boundary.Enforcement, mutate: func() { mutable.Set(now.Add(21 * time.Second)) }}
	}
	if actualPointMode == "handoff-expiry" {
		handoffCurrentReader = &mutatingHandoffReader{value: boundary.Handoff, mutate: func() { mutable.Set(now.Add(21 * time.Second)) }}
	}
	coordination := action.NewCoordinationStoreV1(model, executeReader, handoffCurrentReader, testkit.SettlementOwner())
	prepare := boundary.Enforcement
	prepare.Phase = runtimeports.OperationDispatchEnforcementPrepareV4
	prepare.JournalRevision = 1
	prepare.ReceiptDigest = testkit.Digest("prepare-receipt-real")
	prepare.PrepareReceiptDigest = ""
	prepare.PreparedAttemptDigest = ""
	if err := prepare.Validate(); err != nil {
		t.Fatal(err)
	}
	evidence := realFlowEvidence(t, now, boundary, prepare)
	plan := applicationadapter.SingleCallToolOwnerPlanV1{Candidate: candidate, ApplicationAttempt: toolcontract.ApplicationAttemptRefV1{ID: "application-attempt-real", Revision: 1, Digest: testkit.Digest("application-attempt-real")}, IntentDigest: boundary.Attempt.IntentDigest, DomainSubjectDigest: testkit.Digest("domain-subject-real"), ReservationExpiresUnixNano: now.Add(5 * time.Second).UnixNano(), Operation: boundary.Operation, Attempt: boundary.Attempt, ExecuteEnforcement: boundary.Enforcement, ExecuteHandoff: boundary.Handoff.RefV3(), Settlement: applicationadapter.SingleCallToolSettlementPlanV1{ID: "runtime-settlement-real", DomainOwner: app.Provider, DomainKind: "praxis.tool/domain-result", Evidence: evidence, ExpectedEffectRevision: 3, IdempotencyKey: "settlement-real", ConflictDomain: testkit.Digest("settlement-conflict-real")}}
	observation := testkit.ProviderObservation(now)
	observation.Delegation = *boundary.Attempt.Delegation
	observation.PreparedAttemptID = prepared.ID
	observations := &providerObservationStore{}
	provider := &controlledProvider{observations: observations, value: applicationadapter.SingleCallToolProviderInspectionV1{Operation: boundary.Operation, Attempt: boundary.Attempt, Prepared: prepared, Observation: observation, Schema: testkit.Schema("real-output"), PayloadDigest: observation.PayloadDigest, PayloadRevision: observation.PayloadRevision, Outcome: toolcontract.ToolOutcomeSucceededV2, Disposition: toolcontract.ToolDispositionConfirmedAppliedV2, ObservedUnixNano: observation.ObservedUnixNano}}
	provider.lostReply.Store(providerLostReply)
	settlements := &settlementGateway{now: now}
	settlements.lostReply.Store(settlementLostReply)
	store := action.NewStoreV2(action.CausalReadersV1{OwnerCurrent: echoOwnerCurrent{}, Observation: echoObservationExact{}, Enforcement: echoEnforcementExact{}, Consumption: echoConsumptionExact{}})
	plans := &ownerPlanReader{plan: plan}
	flow, err := applicationadapter.NewToolOwnerSingleCallFlowV1(applicationadapter.ToolOwnerSingleCallFlowConfigV1{Facts: store, Coordination: coordination, Plans: plans, Observations: observations, Provider: provider, Settlements: settlements, Clock: clock, RecoveryTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	adapter := applicationadapter.NewSingleCallToolActionAdapterV1(model, &bindingReader{value: bindings}, coordination, flow, settlements, applicationadapter.NewInMemoryApplicationResultStoreV1(), clock)
	return realFlowFixture{adapter: adapter, flow: flow, request: app.Request, provider: provider, settlements: settlements, coordination: coordination, facts: store, plans: plans, boundaryCalls: &countingReader.calls, model: model, bindings: bindings, clock: clock, observations: observations}
}

type ownerV2RouteReader struct {
	route runtimeports.ControlledOperationProviderRouteCurrentProjectionV2
}

func (r ownerV2RouteReader) InspectCurrentControlledOperationProviderRouteV2(context.Context, runtimeports.ControlledOperationProviderRouteCurrentRefV2, runtimeports.OperationScopeEvidenceApplicabilityMatrixKeyV3) (runtimeports.ControlledOperationProviderRouteCurrentProjectionV2, error) {
	return r.route, nil
}

type ownerV2Gateway struct {
	mu           sync.Mutex
	results      map[string]runtimeports.ControlledOperationProviderResultV2
	status       runtimeports.ControlledOperationProviderResultStatusV2
	provider     *controlledProvider
	enterCalls   atomic.Int32
	entryCalls   atomic.Int32
	admissions   atomic.Int32
	inspectCalls atomic.Int32
	lostReply    atomic.Bool
}

func (g *ownerV2Gateway) EnterControlledOperationProviderV2(_ context.Context, request runtimeports.ControlledOperationProviderRequestV2) (runtimeports.ControlledOperationProviderResultV2, error) {
	g.enterCalls.Add(1)
	key, err := runtimeports.DeriveControlledOperationProviderEntryKeyV2(request)
	if err != nil {
		return runtimeports.ControlledOperationProviderResultV2{}, err
	}
	g.mu.Lock()
	result, exists := g.results[key.EntryID]
	if !exists {
		g.entryCalls.Add(1)
		if g.status == runtimeports.ControlledOperationProviderObservedV2 {
			g.admissions.Add(1)
			g.provider.publishObservation()
			observation := g.provider.value.Observation
			result = testkit.ControlledProviderResultV2(request, g.status, &observation, testkit.FixedTime)
		} else {
			result = testkit.ControlledProviderResultV2(request, g.status, nil, testkit.FixedTime)
		}
		g.results[key.EntryID] = result
	}
	g.mu.Unlock()
	if g.lostReply.CompareAndSwap(true, false) {
		return runtimeports.ControlledOperationProviderResultV2{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "lost Runtime V2 Gateway reply")
	}
	return result, nil
}

func (g *ownerV2Gateway) InspectControlledOperationProviderV2(_ context.Context, request runtimeports.ControlledOperationProviderInspectRequestV2) (runtimeports.ControlledOperationProviderResultV2, error) {
	g.inspectCalls.Add(1)
	if err := request.Validate(); err != nil {
		return runtimeports.ControlledOperationProviderResultV2{}, err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	result, exists := g.results[request.Key.EntryID]
	if !exists {
		return runtimeports.ControlledOperationProviderResultV2{}, core.NewError(core.ErrorNotFound, core.ReasonEvidenceUnavailable, "Runtime V2 Entry not found")
	}
	return result, nil
}

func enableRealFlowV2(t *testing.T, fixture realFlowFixture, status runtimeports.ControlledOperationProviderResultStatusV2, lostReply bool) (realFlowFixture, *ownerV2Gateway) {
	t.Helper()
	plan := fixture.plans.plan
	prepared := fixture.provider.value.Prepared
	persisted := runtimeports.PersistedOperationEnforcementRefV3{PermitID: plan.Attempt.PermitID, PermitRevision: plan.Attempt.PermitRevision, PermitDigest: plan.Attempt.PermitDigest, AttemptID: plan.Attempt.AttemptID, OperationDigest: plan.Attempt.OperationDigest, Provider: plan.Settlement.DomainOwner, ReceiptDigest: testkit.Digest("owner-flow-v2-persisted"), RecordedRevision: 1}
	semantics, err := runtimeports.SealControlledOperationPreparedSemanticSnapshotV2(runtimeports.ControlledOperationPreparedSemanticSnapshotV2{Prepared: prepared, Delegation: *plan.Attempt.Delegation, PersistedEnforcement: persisted, OperationDigest: plan.Attempt.OperationDigest, EffectID: plan.Attempt.EffectID, IntentRevision: plan.Attempt.IntentRevision, IntentDigest: plan.Attempt.IntentDigest, Attempt: plan.Attempt, ProviderBinding: plan.Settlement.DomainOwner, PayloadSchema: prepared.PayloadSchema, PayloadDigest: prepared.PayloadDigest, PayloadRevision: prepared.PayloadRevision})
	if err != nil {
		t.Fatal(err)
	}
	route := testkit.ControlledProviderRouteV2(testkit.FixedTime, plan.Settlement.DomainOwner)
	policy := runtimeports.OperationScopeEvidenceFactRefV3{ID: "owner-flow-evidence-policy-v2", Revision: 1, Digest: testkit.Digest("owner-flow-evidence-policy-v2"), ExpiresUnixNano: testkit.FixedTime.Add(10 * time.Second).UnixNano()}
	plan.ControlledProviderV2 = &applicationadapter.SingleCallToolControlledProviderPlanV2{RouteCurrentRef: route.Ref, ToolAdapterBinding: route.ToolAdapterBinding, PreparedSemantics: semantics, EvidencePolicy: runtimeports.OperationScopeEvidencePolicyRefV3(policy), ApplicabilityPolicy: runtimeports.OperationScopeEvidenceApplicabilityPolicyRefV3(runtimeports.OperationScopeEvidenceFactRefV3{ID: "owner-flow-applicability-policy-v2", Revision: 1, Digest: testkit.Digest("owner-flow-applicability-policy-v2"), ExpiresUnixNano: policy.ExpiresUnixNano}), EffectRevision: 3, CallerDeadlineUnixNano: testkit.FixedTime.Add(5 * time.Second).UnixNano()}
	fixture.plans.plan = plan
	gateway := &ownerV2Gateway{results: map[string]runtimeports.ControlledOperationProviderResultV2{}, status: status, provider: fixture.provider}
	gateway.lostReply.Store(lostReply)
	controlled, err := runtimeadapter.NewControlledProviderV2(ownerV2RouteReader{route}, gateway, fixture.clock, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	flow, err := applicationadapter.NewToolOwnerSingleCallFlowV1(applicationadapter.ToolOwnerSingleCallFlowConfigV1{Facts: fixture.facts, Coordination: fixture.coordination, Plans: fixture.plans, Observations: fixture.observations, Provider: fixture.provider, ControlledV2: controlled, Settlements: fixture.settlements, Clock: fixture.clock, RecoveryTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	fixture.flow = flow
	fixture.adapter = applicationadapter.NewSingleCallToolActionAdapterV1(fixture.model, &bindingReader{value: fixture.bindings}, fixture.coordination, flow, fixture.settlements, applicationadapter.NewInMemoryApplicationResultStoreV1(), fixture.clock)
	return fixture, gateway
}

func realFlowCandidate(t *testing.T, now time.Time, request applicationcontract.SingleCallToolActionRequestV1, bindings applicationadapter.SingleCallToolExactBindingsV1) toolcontract.ActionCandidateV2 {
	t.Helper()
	owner := testkit.SettlementOwner()
	current := func(kind runtimeports.NamespacedNameV2, ref toolcontract.ObjectRef) toolcontract.OwnerCurrentRefV1 {
		return testkit.CurrentRef(kind, ref, owner, now)
	}
	payload := runtimeports.OpaquePayloadV2{Schema: request.PendingAction.PayloadSchema, ContentDigest: request.PendingAction.PayloadDigest, Length: 1, Ref: "test://single-call/arguments", LimitPolicy: runtimeports.OpaqueLimitPolicyRefV2{Policy: "tool/standard", Digest: testkit.Digest("limit-real")}}
	pending := bindings.PendingAction
	schemaRef := toolcontract.ObjectRef{ID: "schema-real-flow", Revision: 1, Digest: request.PendingAction.PayloadSchema.ContentDigest}
	surface := toolcontract.ObjectRef{ID: "surface-real-flow", Revision: 1, Digest: testkit.Digest("surface-real-flow")}
	candidate, err := toolcontract.SealActionCandidateV2(toolcontract.ActionCandidateV2{ID: bindings.ActionID, TenantID: request.ExecutionScope.Identity.TenantID, RunID: string(request.Run.RunID), SessionID: request.Session.ID, TurnID: request.Turn.ID, PendingAction: pending, SourceCandidate: bindings.SourceCandidate, Capability: bindings.Capability, Tool: bindings.Tool, InputSchema: request.PendingAction.PayloadSchema, Payload: payload, PayloadRevision: 1, OperationScopeDigest: request.ExecutionScopeDigest, EffectKind: "praxis.tool/execute", ExpectedOwner: owner, ConflictDomain: "tenant/tenant-v2/tool/real", IdempotencyKey: request.ID, CreatedUnixNano: now.Add(-time.Second).UnixNano(), RequestedExpiresUnixNano: now.Add(6 * time.Second).UnixNano(), PendingActionCurrent: current("praxis.harness/pending-action", toolcontract.ObjectRef{ID: pending.ID, Revision: pending.Revision, Digest: pending.RequestDigest}), SurfaceCurrent: current("praxis.tool/surface", surface), CapabilityCurrent: current("praxis.tool/capability", bindings.Capability), ToolCurrent: current("praxis.tool/descriptor", bindings.Tool), InputSchemaCurrent: current("praxis.tool/input-schema", schemaRef), SourceCandidateCurrent: current("praxis.model/source-candidate", bindings.SourceCandidate)})
	if err != nil {
		t.Fatal(err)
	}
	return candidate
}

func realFlowEvidence(t *testing.T, now time.Time, boundary testkit.BoundaryFixtureV1, prepare runtimeports.OperationDispatchEnforcementPhaseRefV4) []runtimeports.OperationSettlementEvidenceBindingV4 {
	t.Helper()
	makeBinding := func(phase runtimeports.OperationDispatchEnforcementPhaseV4, enforcement runtimeports.OperationDispatchEnforcementPhaseRefV4, handoff runtimeports.OperationScopeEvidenceProviderHandoffRefV3, sequence uint64) runtimeports.OperationSettlementEvidenceBindingV4 {
		label := string(phase)
		record := runtimeports.OperationScopeEvidenceRecordRefV3{LedgerScopeDigest: testkit.Digest("ledger-real-" + label), Sequence: sequence, RecordDigest: testkit.Digest("record-real-" + label)}
		issued := runtimeports.OperationScopeEvidenceQualificationRefV3{ID: "qualification-real-" + label, Revision: 1, Digest: testkit.Digest("qualification-issued-real-" + label), ExpiresUnixNano: now.Add(20 * time.Second).UnixNano()}
		final := issued
		final.Revision = 2
		final.Digest = testkit.Digest("qualification-final-real-" + label)
		return runtimeports.OperationSettlementEvidenceBindingV4{Phase: phase, Consumption: runtimeports.OperationScopeEvidenceConsumptionRefV3{ID: "consumption-real-" + label, Revision: 1, Digest: testkit.Digest("consumption-real-" + label), Record: record}, IssuedQualification: issued, FinalQualification: final, Record: record, CandidateDigest: testkit.Digest("candidate-real-" + label), Handoff: handoff, Attempt: boundary.Attempt, EnforcementPhase: enforcement, OperationScopeDigest: testkit.Digest("scope-real-" + label)}
	}
	prepareHandoff := boundary.Handoff.RefV3()
	prepareHandoff.ID = "handoff-real-prepare"
	prepareHandoff.Digest = testkit.Digest("handoff-real-prepare")
	evidence := []runtimeports.OperationSettlementEvidenceBindingV4{makeBinding(runtimeports.OperationDispatchEnforcementPrepareV4, prepare, prepareHandoff, 1), makeBinding(runtimeports.OperationDispatchEnforcementExecuteV4, boundary.Enforcement, boundary.Handoff.RefV3(), 2)}
	for _, binding := range evidence {
		if err := binding.Validate(); err != nil {
			t.Fatal(err)
		}
	}
	return evidence
}

func TestRealOwnerFlowV1ProviderPathIsNoGoForConcurrentCanonicalCommand(t *testing.T) {
	fixture := newRealFlowFixture(t, false, false)
	const workers = 64
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := fixture.adapter.ExecuteSingleCallToolActionV1(context.Background(), fixture.request)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err == nil || !core.HasCategory(err, core.ErrorUnavailable) {
			t.Fatalf("V1 Provider path error=%v, want Unavailable", err)
		}
	}
	if fixture.boundaryCalls.Load() != 0 || fixture.provider.calls.Load() != 0 || fixture.settlements.settleCalls.Load() != 0 {
		t.Fatalf("boundary=%d provider=%d settlement=%d, want zero", fixture.boundaryCalls.Load(), fixture.provider.calls.Load(), fixture.settlements.settleCalls.Load())
	}
}

func TestRealOwnerFlowProviderCapabilityDriftIsZeroWrite(t *testing.T) {
	fixture := newRealFlowFixture(t, false, false)
	fixture.plans.plan.Settlement.DomainOwner.Capability = "praxis.tool/other"
	_, err := fixture.adapter.ExecuteSingleCallToolActionV1(context.Background(), fixture.request)
	if err == nil || !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("capability drift error=%v, want Conflict", err)
	}
	if fixture.boundaryCalls.Load() != 0 || fixture.provider.calls.Load() != 0 || fixture.settlements.settleCalls.Load() != 0 {
		t.Fatalf("boundary=%d provider=%d settlement=%d, want zero", fixture.boundaryCalls.Load(), fixture.provider.calls.Load(), fixture.settlements.settleCalls.Load())
	}
	exact := toolcontract.ObjectRef{ID: fixture.plans.plan.Candidate.ID, Revision: fixture.plans.plan.Candidate.Revision, Digest: fixture.plans.plan.Candidate.Digest}
	if _, inspectErr := fixture.facts.InspectCandidateCurrentV2(context.Background(), exact, testkit.FixedTime); inspectErr == nil {
		t.Fatal("rejected capability drift wrote Candidate")
	}
}

func TestRealOwnerFlowV2PositiveLostReplyAndConcurrentCanonical(t *testing.T) {
	for _, lost := range []bool{false, true} {
		t.Run(map[bool]string{false: "positive", true: "lost-reply"}[lost], func(t *testing.T) {
			fixture, gateway := enableRealFlowV2(t, newRealFlowFixture(t, false, false), runtimeports.ControlledOperationProviderObservedV2, lost)
			const workers = 64
			var wg sync.WaitGroup
			results := make(chan core.Digest, workers)
			errs := make(chan error, workers)
			for i := 0; i < workers; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					result, err := fixture.adapter.ExecuteSingleCallToolActionV1(context.Background(), fixture.request)
					results <- result.Digest
					errs <- err
				}()
			}
			wg.Wait()
			close(results)
			close(errs)
			for err := range errs {
				if err != nil {
					t.Fatal(err)
				}
			}
			var digest core.Digest
			for resultDigest := range results {
				if digest == "" {
					digest = resultDigest
				} else if resultDigest != digest {
					t.Fatal("same canonical command returned different Tool results")
				}
			}
			if gateway.entryCalls.Load() != 1 || gateway.admissions.Load() != 1 || fixture.provider.calls.Load() != 0 || fixture.settlements.settleCalls.Load() != 1 {
				t.Fatalf("entry=%d admission=%d V1=%d settlement=%d", gateway.entryCalls.Load(), gateway.admissions.Load(), fixture.provider.calls.Load(), fixture.settlements.settleCalls.Load())
			}
			if lost && gateway.inspectCalls.Load() != 1 {
				t.Fatalf("lost reply inspect=%d", gateway.inspectCalls.Load())
			}
		})
	}
}

func TestRealOwnerFlowV2UnknownAndRejectedNeverBecomeDomainResultOrSettlement(t *testing.T) {
	for _, status := range []runtimeports.ControlledOperationProviderResultStatusV2{runtimeports.ControlledOperationProviderUnknownV2, runtimeports.ControlledOperationProviderRejectedNoEffectV2} {
		t.Run(string(status), func(t *testing.T) {
			fixture, gateway := enableRealFlowV2(t, newRealFlowFixture(t, false, false), status, false)
			_, err := fixture.adapter.ExecuteSingleCallToolActionV1(context.Background(), fixture.request)
			if err == nil {
				t.Fatal("non-observed Runtime result advanced Tool facts")
			}
			if gateway.entryCalls.Load() != 1 || gateway.admissions.Load() != 0 || fixture.settlements.settleCalls.Load() != 0 {
				t.Fatalf("entry=%d admission=%d settlement=%d", gateway.entryCalls.Load(), gateway.admissions.Load(), fixture.settlements.settleCalls.Load())
			}
			if fixture.observations.ready {
				t.Fatal("unknown/rejected result was upgraded to a Provider Observation")
			}
		})
	}
}

func TestRealOwnerFlowV2ModelPendingPayloadSpliceIsZeroWriteAndZeroGateway(t *testing.T) {
	fixture, gateway := enableRealFlowV2(t, newRealFlowFixture(t, false, false), runtimeports.ControlledOperationProviderObservedV2, false)
	spliced := fixture.request
	spliced.PendingAction.PayloadDigest = testkit.Digest("attacker-pending-payload")
	var err error
	spliced, err = applicationcontract.SealSingleCallToolActionRequestV1(spliced)
	if err != nil {
		t.Fatal(err)
	}
	bindings := bindingsFor(spliced, fixture.plans.plan.Settlement.DomainOwner)
	fixture.adapter = applicationadapter.NewSingleCallToolActionAdapterV1(fixture.model, &bindingReader{value: bindings}, fixture.coordination, fixture.flow, fixture.settlements, applicationadapter.NewInMemoryApplicationResultStoreV1(), fixture.clock)
	if _, err = fixture.adapter.ExecuteSingleCallToolActionV1(context.Background(), spliced); err == nil || !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("payload splice error=%v, want Conflict", err)
	}
	if fixture.model.calls.Load() != 1 {
		t.Fatalf("payload splice reached coordination Model reread or skipped exact Reader: calls=%d", fixture.model.calls.Load())
	}
	if gateway.enterCalls.Load() != 0 || gateway.entryCalls.Load() != 0 || gateway.admissions.Load() != 0 || fixture.provider.calls.Load() != 0 || fixture.settlements.settleCalls.Load() != 0 {
		t.Fatalf("gateway=%d entry=%d admission=%d V1=%d settlement=%d", gateway.enterCalls.Load(), gateway.entryCalls.Load(), gateway.admissions.Load(), fixture.provider.calls.Load(), fixture.settlements.settleCalls.Load())
	}
	exact := toolcontract.ObjectRef{ID: fixture.plans.plan.Candidate.ID, Revision: fixture.plans.plan.Candidate.Revision, Digest: fixture.plans.plan.Candidate.Digest}
	if _, inspectErr := fixture.facts.InspectCandidateCurrentV2(context.Background(), exact, testkit.FixedTime); inspectErr == nil {
		t.Fatal("payload splice wrote Candidate")
	}
}

var _ runtimeports.OperationSettlementGovernancePortV4 = (*settlementGateway)(nil)
