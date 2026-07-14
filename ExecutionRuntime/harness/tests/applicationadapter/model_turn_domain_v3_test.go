package applicationadapter_test

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	application "github.com/Proview-China/rax/ExecutionRuntime/application"
	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationfakes "github.com/Proview-China/rax/ExecutionRuntime/application/fakes"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/applicationadapter"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/fakes"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/kernel"
	harnessports "github.com/Proview-China/rax/ExecutionRuntime/harness/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const modelTurnStepV3 runtimeports.NamespacedNameV2 = "praxis.harness/model-turn"

type domainFixtureV3 struct {
	now           time.Time
	store         *fakes.GovernedStoreV2
	bindings      *fakes.ModelTurnOperationBindingStoreV3
	adapter       *applicationadapter.ModelTurnDomainAdapterV3
	candidate     contract.ModelTurnCandidateV2
	intent        runtimeports.OperationEffectIntentV3
	delegation    runtimeports.ExecutionDelegationFactV2
	runtime       runtimeports.GovernedExecutionAttemptRefsV2
	prepared      runtimeports.PreparedExecutionGovernanceResultV2
	authorization runtimeports.OperationDispatchAuthorizationV3
	app           applicationcontract.GovernedOperationAttemptRefV3
}

func newDomainFixtureV3(t *testing.T) domainFixtureV3 {
	t.Helper()
	now := time.Unix(2_100_000_000, 0)
	store := fakes.NewGovernedStoreV2()
	store.Clock = func() time.Time { return now }
	loop, err := kernel.NewGovernedLoopV2(kernel.GovernedLoopConfigV2{Sessions: store, Candidates: store, Clock: func() time.Time { return now }, CandidateTTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	_, candidateTemplate := testkit.GovernedFactsV2(now)
	preparedCandidate, err := loop.PrepareInitialCandidateV2(context.Background(), kernel.PrepareInitialCandidateRequestV2{
		Run: candidateTemplate.Run, Endpoint: candidateTemplate.Endpoint, SessionID: candidateTemplate.SessionRef,
		CandidateID: candidateTemplate.ID, Input: candidateTemplate.Input, ContextRef: candidateTemplate.ContextRef,
		ContextDigest: candidateTemplate.ContextDigest, Provider: candidateTemplate.Provider,
		CreatedUnixNano: candidateTemplate.CreatedUnixNano, ExpiresUnixNano: candidateTemplate.ExpiresUnixNano,
	})
	if err != nil {
		t.Fatal(err)
	}
	candidate := preparedCandidate.Candidate
	payload, err := contract.NewModelTurnEffectPayloadV2(candidate)
	if err != nil {
		t.Fatal(err)
	}
	scopeDigest, _ := runtimeports.ExecutionScopeDigestV2(candidate.Run.Scope)
	operation := runtimeports.OperationSubjectV3{
		Kind: runtimeports.OperationScopeRunV3, ExecutionScope: candidate.Run.Scope, ExecutionScopeDigest: scopeDigest,
		RunID: candidate.Run.RunID, SubjectRevision: 1, CurrentProjectionRef: "projection-model-turn",
		CurrentProjectionDigest: testkit.Digest("projection-model-turn"), CurrentProjectionRevision: 1,
	}
	operationDigest, _ := operation.DigestV3()
	authority := runtimeports.AuthorityBindingRefV2{Ref: "authority-model-turn", Digest: testkit.Digest("authority-model-turn"), Revision: 1, Epoch: 1}
	review := runtimeports.OperationReviewBindingRefV3{CaseRef: "review-model-turn", CandidateDigest: testkit.Digest("review-candidate"), CandidateRevision: 1, PolicyDigest: testkit.Digest("review-policy")}
	budget := runtimeports.OperationBudgetBindingRefV3{Ref: "budget-model-turn", Digest: testkit.Digest("budget"), Revision: 1, PolicyDigest: testkit.Digest("budget-policy"), SubjectDigest: operationDigest}
	policy := runtimeports.OperationPolicyBindingRefV3{Ref: "policy-model-turn", Digest: testkit.Digest("policy"), Revision: 1, SubjectDigest: operationDigest}
	owner := runtimeports.EffectOwnerRefV2{ComponentID: "praxis.harness/model-turn-owner", ManifestDigest: testkit.Digest("model-owner")}
	intent := runtimeports.OperationEffectIntentV3{
		ContractVersion: runtimeports.OperationEffectContractVersionV3, ID: "effect-model-turn", Revision: 1,
		Operation: operation, Kind: "praxis.harness/model-turn", RiskClass: "praxis.harness/controlled",
		ActionScopeDigest: testkit.Digest("model-action-scope"), Payload: payload, PayloadRevision: 1, Target: "model-turn",
		ConflictDomain: runtimeports.ConflictDomainBindingV2{Domain: "praxis.harness/model-turn", ScopeClass: runtimeports.EffectStableScopeTenantV2, ScopeDigest: runtimeports.StableTenantScopeDigestV2(candidate.Run.Scope.Identity.TenantID)},
		Owners: []runtimeports.EffectOwnerRefV2{
			{Role: runtimeports.OwnerCleanup, ComponentID: owner.ComponentID, ManifestDigest: owner.ManifestDigest},
			{Role: runtimeports.OwnerEffect, ComponentID: owner.ComponentID, ManifestDigest: owner.ManifestDigest},
			{Role: runtimeports.OwnerSettlement, ComponentID: owner.ComponentID, ManifestDigest: owner.ManifestDigest},
		},
		Provider: candidate.Provider, Authority: authority, Review: review, Budget: budget, Policy: policy,
		Idempotency:      runtimeports.IdempotencyBindingV2{Key: "model-turn-idempotency", ScopeClass: runtimeports.EffectStableScopeTenantV2, ScopeDigest: runtimeports.StableTenantScopeDigestV2(candidate.Run.Scope.Identity.TenantID), Class: core.IdempotencyQueryable},
		CredentialLeases: []runtimeports.CredentialLeaseRefV2{}, ExpiresUnixNano: now.Add(time.Minute).UnixNano(),
	}
	intentDigest, err := intent.DigestV3()
	if err != nil {
		t.Fatal(err)
	}
	governanceFact := func(ref string) runtimeports.OperationGovernanceFactRefV3 {
		return runtimeports.OperationGovernanceFactRefV3{Ref: ref, Revision: 1, Digest: testkit.Digest(ref), ExpiresUnixNano: now.Add(30 * time.Second).UnixNano()}
	}
	reviewAuthorization := runtimeports.OperationReviewAuthorizationV3{
		Case: governanceFact(review.CaseRef), CandidateDigest: review.CandidateDigest, CandidateRevision: review.CandidateRevision,
		Verdict: governanceFact("verdict-model-turn"), ReviewerAuthority: governanceFact("reviewer-model-turn"), PolicyDigest: review.PolicyDigest,
		ExpiresUnixNano: now.Add(30 * time.Second).UnixNano(),
	}
	capabilityDigest := testkit.Digest("capability-model-turn")
	fence := core.ExecutionFence{BoundaryScope: core.FenceBoundaryInstance, Scope: candidate.Run.Scope, CapabilityGrantDigest: capabilityDigest, EffectIntentID: intent.ID, EffectIntentRevision: intent.Revision, CanonicalPayloadDigest: payload.ContentDigest, ExpiresAt: now.Add(30 * time.Second)}
	fenceDigest, err := runtimeports.DigestOperationExecutionFenceV3(fence, operation)
	if err != nil {
		t.Fatal(err)
	}
	permit := runtimeports.OperationDispatchPermitV3{
		ContractVersion: runtimeports.OperationEffectContractVersionV3, ID: "permit-model-turn", Revision: 1, AttemptID: "attempt-model-turn",
		IntentID: intent.ID, IntentRevision: intent.Revision, IntentDigest: intentDigest, Operation: operation,
		PayloadSchema: payload.Schema, PayloadDigest: payload.ContentDigest, PayloadRevision: 1,
		ConflictDomain: intent.ConflictDomain, Provider: candidate.Provider, EnforcementPoint: candidate.Provider,
		Authority: authority, Review: review, ReviewAuthorization: reviewAuthorization, Budget: budget, Policy: policy,
		CapabilityGrantDigest: capabilityDigest, CredentialGrantDigest: testkit.Digest("credentials-model-turn"), GovernanceSnapshotDigest: testkit.Digest("governance-model-turn"), FenceDigest: fenceDigest,
		Idempotency: intent.Idempotency, IssuedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(30 * time.Second).UnixNano(),
	}
	permitDigest, err := permit.DigestV3()
	if err != nil {
		t.Fatal(err)
	}
	authorization := runtimeports.OperationDispatchAuthorizationV3{
		Attempt: runtimeports.OperationDispatchAttemptRefV3{OperationDigest: operationDigest, EffectID: intent.ID, IntentRevision: intent.Revision, IntentDigest: intentDigest, PermitID: permit.ID, PermitRevision: permit.Revision, PermitDigest: permitDigest, AttemptID: permit.AttemptID},
		Permit:  permit, EffectFactRevision: 2, PermitFactRevision: 1, State: runtimeports.OperationDispatchAuthorizationIssuedV3, Fence: fence, ExpiresUnixNano: permit.ExpiresUnixNano,
	}
	if err := authorization.Validate(); err != nil {
		t.Fatal(err)
	}
	delegation := runtimeports.ExecutionDelegationFactV2{
		ContractVersion: runtimeports.ExecutionGovernanceContractVersionV2, ID: "delegation-model-turn", Revision: 1, State: runtimeports.ExecutionDelegationDeclaredV2,
		BindingSetID: candidate.Provider.BindingSetID, BindingSetRevision: candidate.Provider.BindingSetRevision,
		Operation: operation, HostAdapter: candidate.Endpoint.Binding, DataProvider: candidate.Provider,
		RelayHops:  []runtimeports.ExecutionRelayHopV2{{Sequence: 1, Relay: candidate.Endpoint.Binding}},
		EndpointID: candidate.Endpoint.ID, RuntimeSessionRef: candidate.SessionRef,
		PayloadSchema: payload.Schema, PayloadDigest: payload.ContentDigest, PayloadRevision: 1,
		IntentID: intent.ID, IntentRevision: intent.Revision, IntentDigest: intentDigest,
		ProviderPermitID: "permit-model-turn", ProviderPermitRevision: 1, ProviderPermitDigest: permitDigest,
		ProviderAttemptID: "attempt-model-turn", OperationExpiresUnixNano: intent.ExpiresUnixNano,
		PermitExpiresUnixNano: permit.ExpiresUnixNano, HostBindingExpiresUnixNano: now.Add(45 * time.Second).UnixNano(), ProviderBindingExpiresUnixNano: now.Add(45 * time.Second).UnixNano(),
		CreatedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(20 * time.Second).UnixNano(),
	}
	delegation.PreparedAttemptID, _ = runtimeports.DerivePreparedProviderAttemptIDV2(delegation.ID, delegation.ProviderPermitID, delegation.ProviderAttemptID)
	declared, err := delegation.RefV2()
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := runtimeports.SealPreparedProviderAttemptRefV2(runtimeports.PreparedProviderAttemptRefV2{
		ID: delegation.PreparedAttemptID, Revision: 1, DeclaredDelegation: declared, OperationDigest: operationDigest,
		IntentID: intent.ID, IntentRevision: intent.Revision, IntentDigest: intentDigest,
		PermitID: delegation.ProviderPermitID, PermitRevision: delegation.ProviderPermitRevision, PermitDigest: permitDigest, AttemptID: delegation.ProviderAttemptID,
		Provider: candidate.Provider, PayloadSchema: payload.Schema, PayloadDigest: payload.ContentDigest, PayloadRevision: 1,
		PreparedUnixNano: now.Add(time.Nanosecond).UnixNano(), ExpiresUnixNano: delegation.ExpiresUnixNano,
	})
	if err != nil {
		t.Fatal(err)
	}
	preparedDelegation := runtimeports.ExecutionDelegationRefV2{ID: delegation.ID, Revision: 2, Digest: testkit.Digest("prepared-delegation-model-turn")}
	enforcement := runtimeports.PersistedOperationEnforcementRefV3{PermitID: delegation.ProviderPermitID, PermitRevision: 1, PermitDigest: permitDigest, AttemptID: delegation.ProviderAttemptID, OperationDigest: operationDigest, Provider: candidate.Provider, ReceiptDigest: testkit.Digest("enforcement-model-turn"), RecordedRevision: 3}
	runtimeAttempt := runtimeports.GovernedExecutionAttemptRefsV2{
		Admission: runtimeports.OperationEffectAdmissionReceiptV3{OperationDigest: operationDigest, EffectID: intent.ID, IntentRevision: intent.Revision, IntentDigest: intentDigest, FactRevision: 2, State: "accepted"},
		PermitID:  delegation.ProviderPermitID, PermitRevision: 1, PermitDigest: permitDigest, AttemptID: delegation.ProviderAttemptID,
		Delegation: preparedDelegation, Prepared: prepared, Enforcement: enforcement,
	}
	preparedResult := runtimeports.PreparedExecutionGovernanceResultV2{Delegation: preparedDelegation, Prepared: prepared, Enforcement: enforcement}
	descriptor := applicationcontract.StepDescriptorRefV2{Kind: modelTurnStepV3, Revision: 1, Digest: testkit.Digest("model-turn-descriptor"), ExpiresUnixNano: now.Add(time.Hour).UnixNano()}
	routingDigest, _ := core.CanonicalJSONDigest("praxis.application.governed-operation-attempt", applicationcontract.GovernedOperationAttemptContractVersionV3, "OperationRoutingBindingsV3", struct {
		StepKind        runtimeports.NamespacedNameV2           `json:"step_kind"`
		Descriptor      applicationcontract.StepDescriptorRefV2 `json:"descriptor"`
		PlannedProvider runtimeports.ProviderBindingRefV2       `json:"planned_provider"`
		DomainAdapter   runtimeports.ProviderBindingRefV2       `json:"domain_adapter"`
		PlanAuthority   runtimeports.AuthorityBindingRefV2      `json:"plan_authority"`
	}{modelTurnStepV3, descriptor, candidate.Provider, candidate.Endpoint.Binding, authority})
	app := applicationcontract.GovernedOperationAttemptRefV3{
		ID: "application-attempt-model-turn", Revision: 5, State: applicationcontract.OperationExecutionPreparedV3,
		Digest: testkit.Digest("application-prepared"), ScopeDigest: scopeDigest, JournalID: "journal-model-turn", StepID: "step-model-turn", StepKind: modelTurnStepV3,
		Descriptor: descriptor, PlannedProvider: candidate.Provider, DomainAdapter: candidate.Endpoint.Binding, PlanAuthority: authority, RoutingDigest: routingDigest,
		WorkflowAttempt: 1, OperationDigest: operationDigest, EffectID: intent.ID, AuthorizationDigest: testkit.Digest("authorization-model-turn"),
	}
	bindings := fakes.NewModelTurnOperationBindingStoreV3()
	adapter, err := applicationadapter.NewModelTurnDomainAdapterV3(applicationadapter.ModelTurnDomainAdapterConfigV3{
		StepKind: modelTurnStepV3, Adapter: candidate.Endpoint.Binding, Bindings: bindings, Reservations: store, Sessions: store, Candidates: store,
		Turns: &kernel.GovernedTurnStateCoordinatorV2{Sessions: store}, Clock: func() time.Time { return now.Add(10 * time.Second) },
	})
	if err != nil {
		t.Fatal(err)
	}
	initial := app
	initial.Revision, initial.State, initial.Digest = 1, applicationcontract.OperationIntentRecordedV3, testkit.Digest("application-intent-recorded")
	initial.AuthorizationDigest = testkit.Digest("authorization-not-yet-created")
	reservation, err := adapter.ReserveOperationIntentV3(context.Background(), applicationports.ReserveOperationIntentRequestV3{StepKind: modelTurnStepV3, Descriptor: descriptor, DomainAdapter: candidate.Endpoint.Binding, Attempt: initial, Intent: intent})
	if err != nil {
		t.Fatal(err)
	}
	app.DomainReservation = &reservation
	return domainFixtureV3{now: now, store: store, bindings: bindings, adapter: adapter, candidate: candidate, intent: intent, delegation: delegation, runtime: runtimeAttempt, prepared: preparedResult, authorization: authorization, app: app}
}

func (f domainFixtureV3) preparedRequest() applicationports.BindPreparedOperationRequestV3 {
	return applicationports.BindPreparedOperationRequestV3{StepKind: modelTurnStepV3, Attempt: f.app, Intent: f.intent, RuntimeAttempt: f.runtime, DelegationFact: f.delegation, Prepared: f.prepared}
}

func (f domainFixtureV3) observedRequest(t *testing.T) applicationports.BindObservedOperationRequestV3 {
	t.Helper()
	request := applicationports.BindObservedOperationRequestV3{StepKind: modelTurnStepV3, Attempt: f.app, Intent: f.intent, RuntimeAttempt: f.runtime}
	request.Attempt.Revision++
	request.Attempt.State = applicationcontract.OperationProviderObservedV3
	request.Attempt.Digest = testkit.Digest("application-observed")
	observation := runtimeports.ProviderAttemptObservationRefV2{
		Delegation: f.runtime.Delegation, PreparedAttemptID: f.runtime.Prepared.ID, ProviderOperationRef: "provider-operation-model-turn", Revision: 1,
		State: runtimeports.ProviderAttemptObservedV2, Digest: testkit.Digest("observation-model-turn"), PayloadDigest: f.intent.Payload.ContentDigest, PayloadRevision: 1,
		SourceRegistrationID: "source-model-turn", SourceEpoch: 1, SourceSequence: 1,
		Evidence: runtimeports.EvidenceRecordRefV2{LedgerScopeDigest: testkit.Digest("ledger-model-turn"), Sequence: 1, RecordDigest: testkit.Digest("record-model-turn")}, ObservedUnixNano: f.now.Add(time.Second).UnixNano(),
	}
	request.RuntimeAttempt.Observation = &observation
	request.Observation = observation
	if err := request.Validate(); err != nil {
		t.Fatal(err)
	}
	return request
}

func (f domainFixtureV3) unknownRequest(t *testing.T) applicationports.MarkUnknownOperationRequestV3 {
	t.Helper()
	app := f.app
	app.Revision++
	app.State = applicationcontract.OperationDispatchUnknownV3
	app.Digest = testkit.Digest("application-unknown")
	app.DispatchUnknown = true
	authorization := f.authorization
	authorization.State = runtimeports.OperationDispatchAuthorizationUnknownV3
	authorization.EffectFactRevision++
	request := applicationports.MarkUnknownOperationRequestV3{StepKind: modelTurnStepV3, Attempt: app, Intent: f.intent, RuntimeAttempt: f.runtime, Authorization: authorization}
	if err := request.Validate(); err != nil {
		t.Fatal(err)
	}
	return request
}

func (f domainFixtureV3) postpreparedUnknownSettlementRequest(t *testing.T, unknown applicationports.MarkUnknownOperationRequestV3) applicationports.ApplyOperationSettlementRequestV3 {
	t.Helper()
	candidateRef, _ := f.candidate.RefV2()
	domain, err := contract.NewSettledTurnDomainResultV2(contract.NewSettledTurnFailureV2(candidateRef, "praxis.harness/inspect-not-applied", []byte("independent Inspect proved failure")))
	if err != nil {
		t.Fatal(err)
	}
	delegation := f.runtime.Delegation
	settlement := runtimeports.OperationSettlementRefV3{
		ID: "settlement-inspected-model-turn", Revision: 1, Digest: testkit.Digest("settlement-inspected-model-turn"),
		Attempt:     runtimeports.OperationDispatchAttemptRefV3{OperationDigest: f.runtime.Admission.OperationDigest, EffectID: f.intent.ID, IntentRevision: f.intent.Revision, IntentDigest: f.runtime.Admission.IntentDigest, PermitID: f.runtime.PermitID, PermitRevision: f.runtime.PermitRevision, PermitDigest: f.runtime.PermitDigest, AttemptID: f.runtime.AttemptID, Delegation: &delegation},
		Disposition: runtimeports.OperationSettlementFailedV3, Owner: runtimeports.EffectOwnerRefV2{Role: runtimeports.OwnerSettlement, ComponentID: "praxis.harness/model-turn-owner", ManifestDigest: testkit.Digest("model-owner")},
		Evidence:           []runtimeports.EvidenceRecordRefV2{{LedgerScopeDigest: testkit.Digest("unknown-ledger"), Sequence: 1, RecordDigest: testkit.Digest("unknown-record")}},
		DomainResultSchema: &domain.Schema, DomainResultDigest: domain.ContentDigest,
	}
	runtimeAttempt := f.runtime
	runtimeAttempt.Settlement = &settlement
	testkit.AddUnknownInspectProvenanceV2(&runtimeAttempt)
	settlement = *runtimeAttempt.Settlement
	app := unknown.Attempt
	app.Revision++
	app.State = applicationcontract.OperationSettledV3
	app.Digest = testkit.Digest("application-unknown-settled")
	app.Settlement = &settlement
	request := applicationports.ApplyOperationSettlementRequestV3{StepKind: modelTurnStepV3, Attempt: app, Intent: f.intent, RuntimeAttempt: &runtimeAttempt, Settlement: settlement, DomainResult: &domain}
	if err := request.Validate(); err != nil {
		t.Fatal(err)
	}
	return request
}

func (f domainFixtureV3) settlementRequest(t *testing.T, observed applicationports.BindObservedOperationRequestV3) applicationports.ApplyOperationSettlementRequestV3 {
	t.Helper()
	candidateRef, _ := f.candidate.RefV2()
	domain, err := contract.NewSettledTurnDomainResultV2(contract.SettledTurnResultV2{ContractVersion: contract.SettledTurnResultContractV2, Candidate: candidateRef, State: contract.SettledTurnCompletedV2, Output: &f.candidate.Input})
	if err != nil {
		t.Fatal(err)
	}
	delegation := f.runtime.Delegation
	settlement := runtimeports.OperationSettlementRefV3{
		ID: "settlement-model-turn", Revision: 1, Digest: testkit.Digest("settlement-model-turn"),
		Attempt:     runtimeports.OperationDispatchAttemptRefV3{OperationDigest: f.runtime.Admission.OperationDigest, EffectID: f.intent.ID, IntentRevision: f.intent.Revision, IntentDigest: f.runtime.Admission.IntentDigest, PermitID: f.runtime.PermitID, PermitRevision: f.runtime.PermitRevision, PermitDigest: f.runtime.PermitDigest, AttemptID: f.runtime.AttemptID, Delegation: &delegation},
		Disposition: runtimeports.OperationSettlementAppliedV3, Owner: runtimeports.EffectOwnerRefV2{Role: runtimeports.OwnerSettlement, ComponentID: "praxis.harness/model-turn-owner", ManifestDigest: testkit.Digest("model-owner")},
		Observation: &observed.Observation, Evidence: []runtimeports.EvidenceRecordRefV2{observed.Observation.Evidence}, DomainResultSchema: &domain.Schema, DomainResultDigest: domain.ContentDigest,
	}
	runtimeAttempt := observed.RuntimeAttempt
	runtimeAttempt.Settlement = &settlement
	app := observed.Attempt
	app.Revision++
	app.State = applicationcontract.OperationSettledV3
	app.Digest = testkit.Digest("application-settled")
	app.Settlement = &settlement
	request := applicationports.ApplyOperationSettlementRequestV3{StepKind: modelTurnStepV3, Attempt: app, Intent: f.intent, RuntimeAttempt: &runtimeAttempt, Settlement: settlement, DomainResult: &domain}
	if err := request.Validate(); err != nil {
		t.Fatal(err)
	}
	return request
}

func (f domainFixtureV3) undispatchedSettlementRequest(t *testing.T) applicationports.ApplyOperationSettlementRequestV3 {
	t.Helper()
	candidateRef, _ := f.candidate.RefV2()
	domain, err := contract.NewSettledTurnDomainResultV2(contract.NewSettledTurnFailureV2(candidateRef, "praxis.harness/dispatch-not-applied", []byte("prepare was never reached")))
	if err != nil {
		t.Fatal(err)
	}
	settlement := runtimeports.OperationSettlementRefV3{
		ID: "settlement-undispatched-model-turn", Revision: 1, Digest: testkit.Digest("settlement-undispatched-model-turn"),
		Attempt:     runtimeports.OperationDispatchAttemptRefV3{OperationDigest: f.runtime.Admission.OperationDigest, EffectID: f.intent.ID, IntentRevision: f.intent.Revision, IntentDigest: f.runtime.Admission.IntentDigest, PermitID: f.runtime.PermitID, PermitRevision: f.runtime.PermitRevision, PermitDigest: f.runtime.PermitDigest, AttemptID: f.runtime.AttemptID},
		Disposition: runtimeports.OperationSettlementFailedV3, Owner: runtimeports.EffectOwnerRefV2{Role: runtimeports.OwnerSettlement, ComponentID: "praxis.harness/model-turn-owner", ManifestDigest: testkit.Digest("model-owner")},
		Evidence:           []runtimeports.EvidenceRecordRefV2{{LedgerScopeDigest: testkit.Digest("undispatched-ledger"), Sequence: 1, RecordDigest: testkit.Digest("undispatched-record")}},
		DomainResultSchema: &domain.Schema, DomainResultDigest: domain.ContentDigest,
	}
	app := f.app
	app.Revision++
	app.State = applicationcontract.OperationSettledV3
	app.Digest = testkit.Digest("application-undispatched-settled")
	app.DispatchUnknown = true
	app.Settlement = &settlement
	request := applicationports.ApplyOperationSettlementRequestV3{StepKind: modelTurnStepV3, Attempt: app, Intent: f.intent, Settlement: settlement, DomainResult: &domain}
	if err := request.Validate(); err != nil {
		t.Fatal(err)
	}
	return request
}

func TestModelTurnDomainAdapterV3ObservedRoundTripRecoversLostReplies(t *testing.T) {
	f := newDomainFixtureV3(t)
	f.store.LoseNextSessionCASReply = true
	f.bindings.LoseNextCreateReply = true
	prepared, err := f.adapter.BindPrepared(context.Background(), f.preparedRequest())
	if err != nil || prepared.State != applicationports.OperationDomainPreparedV3 || prepared.Revision != 1 {
		t.Fatalf("prepared binding failed: %#v err=%v", prepared, err)
	}
	replayed, err := f.adapter.BindPrepared(context.Background(), f.preparedRequest())
	if err != nil || replayed.Digest != prepared.Digest {
		t.Fatalf("exact prepared replay drifted: %#v err=%v", replayed, err)
	}
	observedRequest := f.observedRequest(t)
	f.store.LoseNextSessionCASReply = true
	f.bindings.LoseNextCASReply = true
	observed, err := f.adapter.BindObserved(context.Background(), observedRequest)
	if err != nil || observed.State != applicationports.OperationDomainObservedV3 || observed.Revision != 2 {
		t.Fatalf("observed binding failed: %#v err=%v", observed, err)
	}
	settlementRequest := f.settlementRequest(t, observedRequest)
	f.store.LoseNextSessionCASReply = true
	f.bindings.LoseNextCASReply = true
	settled, err := f.adapter.ApplySettlement(context.Background(), settlementRequest)
	if err != nil || settled.State != applicationports.OperationDomainSettledV3 || settled.Revision != 3 {
		t.Fatalf("settled binding failed: %#v err=%v", settled, err)
	}
	session, err := f.store.InspectSessionV2(context.Background(), f.candidate.Run, f.candidate.SessionRef)
	if err != nil || session.Phase != contract.SessionTerminalV2 || session.CompletionClaim != contract.ClaimCompleted {
		t.Fatalf("settlement did not terminally commit exact Session: %#v err=%v", session, err)
	}
	inspected, err := f.adapter.InspectOperationDomainStateV3(context.Background(), applicationports.OperationDomainInspectRequestV3{Scope: f.candidate.Run.Scope, StepKind: modelTurnStepV3, AttemptID: f.app.ID})
	if err != nil || inspected.Digest != settled.Digest {
		t.Fatalf("Inspect did not recover durable domain state: %#v err=%v", inspected, err)
	}
}

func TestModelTurnDomainAdapterV3PreparedConcurrentReplayLinearizesOnce(t *testing.T) {
	f := newDomainFixtureV3(t)
	const workers = 64
	results := make(chan applicationports.OperationDomainStateRefV3, workers)
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := f.adapter.BindPrepared(context.Background(), f.preparedRequest())
			results <- result
			errs <- err
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent exact replay failed: %v", err)
		}
	}
	var digest core.Digest
	for result := range results {
		if digest == "" {
			digest = result.Digest
		} else if result.Digest != digest {
			t.Fatalf("concurrent exact replay returned different facts: %s != %s", result.Digest, digest)
		}
	}
	fact, err := f.bindings.InspectModelTurnOperationBindingV3(context.Background(), f.candidate.Run.Scope, modelTurnStepV3, f.app.ID)
	if err != nil || fact.Revision != 1 || fact.SessionRevision != 4 {
		t.Fatalf("concurrent prepare did not linearize once: %#v err=%v", fact, err)
	}
}

func TestModelTurnDomainAdapterV3ReservationHasOneWinnerAcross64Attempts(t *testing.T) {
	f := newDomainFixtureV3(t)
	resetFixtureForCoordinatorV3(t, &f)
	const workers = 64
	start := make(chan struct{})
	type result struct {
		attempt     applicationcontract.GovernedOperationAttemptRefV3
		reservation applicationcontract.OperationDomainReservationRefV3
		err         error
	}
	results := make(chan result, workers)
	var wg sync.WaitGroup
	for index := range workers {
		attempt := initialReservationAttemptV3(f, "reservation-race-"+strconv.Itoa(index))
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			reservation, err := f.adapter.ReserveOperationIntentV3(context.Background(), reservationRequestV3(f, attempt))
			results <- result{attempt, reservation, err}
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	winners := 0
	var winner result
	for result := range results {
		if result.err == nil {
			winners++
			winner = result
		} else if !core.HasCategory(result.err, core.ErrorConflict) {
			t.Fatalf("loser returned non-conflict: %v", result.err)
		}
	}
	if winners != 1 {
		t.Fatalf("expected exactly one reservation winner, got %d", winners)
	}
	session, err := f.store.InspectSessionV2(context.Background(), f.candidate.Run, f.candidate.SessionRef)
	if err != nil || session.Phase != contract.SessionModelDispatchReservedV2 || session.DomainReservation == nil || session.DomainReservation.AttemptID != winner.attempt.ID || session.Revision != 3 {
		t.Fatalf("atomic reservation did not linearize once: %#v err=%v", session, err)
	}
	inspected, err := f.adapter.InspectOperationIntentReservationV3(context.Background(), applicationports.InspectOperationIntentReservationRequestV3{Scope: f.candidate.Run.Scope, StepKind: modelTurnStepV3, DomainAdapter: f.candidate.Endpoint.Binding, AttemptID: winner.attempt.ID})
	if err != nil || inspected.Digest != winner.reservation.Digest {
		t.Fatalf("winner reservation index is not recoverable: %#v err=%v", inspected, err)
	}
}

func TestModelTurnDomainAdapterV3LostReplyRecoversHistoricalReservationAfterExpiry(t *testing.T) {
	f := newDomainFixtureV3(t)
	now := f.now.Add(10 * time.Millisecond)
	resetFixtureForCoordinatorWithClockV3(t, &f, func() time.Time { return now })
	attempt := initialReservationAttemptV3(f, "reservation-lost-reply")
	f.store.LoseNextReservationCommitReply = true
	first, err := f.adapter.ReserveOperationIntentV3(context.Background(), reservationRequestV3(f, attempt))
	if err != nil {
		t.Fatal(err)
	}
	now = time.Unix(0, first.ExpiresUnixNano).Add(time.Nanosecond)
	replayed, err := f.adapter.ReserveOperationIntentV3(context.Background(), reservationRequestV3(f, attempt))
	if err != nil || replayed.Digest != first.Digest || now.Before(time.Unix(0, replayed.ExpiresUnixNano)) {
		t.Fatalf("expired committed reservation was not historically recovered: %#v err=%v", replayed, err)
	}
	session, _ := f.store.InspectSessionV2(context.Background(), f.candidate.Run, f.candidate.SessionRef)
	if session.Revision != 3 || session.DomainReservation == nil || session.DomainReservation.Digest != first.Digest {
		t.Fatalf("recovery mutated reservation Session: %#v", session)
	}
}

func TestModelTurnDomainAdapterV3FirstReservationFailsClosedWithoutMutation(t *testing.T) {
	t.Run("expired_candidate", func(t *testing.T) {
		f := newDomainFixtureV3(t)
		now := time.Unix(0, f.candidate.ExpiresUnixNano).Add(time.Nanosecond)
		resetFixtureForCoordinatorWithClockV3(t, &f, func() time.Time { return now })
		attempt := initialReservationAttemptV3(f, "reservation-expired")
		if _, err := f.adapter.ReserveOperationIntentV3(context.Background(), reservationRequestV3(f, attempt)); err == nil {
			t.Fatal("expired Candidate reserved")
		}
		assertWaitingUnreservedV3(t, f)
	})
	t.Run("terminal_session", func(t *testing.T) {
		f := newDomainFixtureV3(t)
		resetFixtureForCoordinatorV3(t, &f)
		session, _ := f.store.InspectSessionV2(context.Background(), f.candidate.Run, f.candidate.SessionRef)
		terminal := session
		terminal.Revision++
		terminal.Phase = contract.SessionTerminalV2
		terminal.Candidate = nil
		terminal.CompletionClaim = contract.ClaimCancelled
		terminal.UpdatedUnixNano++
		if _, err := f.store.CompareAndSwapSessionV2(context.Background(), harnessports.SessionCASRequestV2{ExpectedRevision: session.Revision, Next: terminal}); err != nil {
			t.Fatal(err)
		}
		attempt := initialReservationAttemptV3(f, "reservation-terminal")
		if _, err := f.adapter.ReserveOperationIntentV3(context.Background(), reservationRequestV3(f, attempt)); err == nil {
			t.Fatal("terminal Session reserved")
		}
	})
	t.Run("ref_only_payload", func(t *testing.T) {
		f := newDomainFixtureV3(t)
		resetFixtureForCoordinatorV3(t, &f)
		attempt := initialReservationAttemptV3(f, "reservation-ref-only")
		request := reservationRequestV3(f, attempt)
		request.Intent.Payload.Inline = nil
		if _, err := f.adapter.ReserveOperationIntentV3(context.Background(), request); err == nil {
			t.Fatal("ref-only model-turn payload crossed fail-closed boundary")
		}
		assertWaitingUnreservedV3(t, f)
	})
}

func TestModelTurnDomainAdapterV3AllowsConfiguredNamespacedCustomStepWithoutKindSwitch(t *testing.T) {
	f := newDomainFixtureV3(t)
	resetFixtureForCoordinatorV3(t, &f)
	customStep := runtimeports.NamespacedNameV2("custom.module/model-turn")
	adapter, err := applicationadapter.NewModelTurnDomainAdapterV3(applicationadapter.ModelTurnDomainAdapterConfigV3{StepKind: customStep, Adapter: f.candidate.Endpoint.Binding, Bindings: f.bindings, Reservations: f.store, Sessions: f.store, Candidates: f.store, Turns: &kernel.GovernedTurnStateCoordinatorV2{Sessions: f.store}, Clock: func() time.Time { return f.now.Add(10 * time.Millisecond) }})
	if err != nil {
		t.Fatal(err)
	}
	attempt := initialReservationAttemptV3(f, "custom-step-reservation")
	attempt.StepKind, attempt.Descriptor.Kind = customStep, customStep
	attempt.RoutingDigest, _ = core.CanonicalJSONDigest("praxis.application.governed-operation-attempt", applicationcontract.GovernedOperationAttemptContractVersionV3, "OperationRoutingBindingsV3", struct {
		StepKind        runtimeports.NamespacedNameV2           `json:"step_kind"`
		Descriptor      applicationcontract.StepDescriptorRefV2 `json:"descriptor"`
		PlannedProvider runtimeports.ProviderBindingRefV2       `json:"planned_provider"`
		DomainAdapter   runtimeports.ProviderBindingRefV2       `json:"domain_adapter"`
		PlanAuthority   runtimeports.AuthorityBindingRefV2      `json:"plan_authority"`
	}{attempt.StepKind, attempt.Descriptor, attempt.PlannedProvider, attempt.DomainAdapter, attempt.PlanAuthority})
	reservation, err := adapter.ReserveOperationIntentV3(context.Background(), applicationports.ReserveOperationIntentRequestV3{StepKind: customStep, Descriptor: attempt.Descriptor, DomainAdapter: attempt.DomainAdapter, Attempt: attempt, Intent: f.intent})
	if err != nil || reservation.StepKind != customStep {
		t.Fatalf("configured custom StepKind could not use generic reservation bridge: %#v err=%v", reservation, err)
	}
}

func TestModelTurnBindingFakeV3ReturnsDeepCopiesAndNoProductionAuthority(t *testing.T) {
	f := newDomainFixtureV3(t)
	if _, err := f.adapter.BindPrepared(context.Background(), f.preparedRequest()); err != nil {
		t.Fatal(err)
	}
	first, err := f.bindings.InspectModelTurnOperationBindingV3(context.Background(), f.candidate.Run.Scope, modelTurnStepV3, f.app.ID)
	if err != nil {
		t.Fatal(err)
	}
	first.DelegationFact.RelayHops[0].Relay.ComponentID = "malicious/local-mutation"
	first.RuntimeAttempt.Prepared.PayloadDigest = testkit.Digest("malicious/local-payload")
	first.ApplicationAttempt.DomainReservation.ID = "malicious/local-reservation"
	second, err := f.bindings.InspectModelTurnOperationBindingV3(context.Background(), f.candidate.Run.Scope, modelTurnStepV3, f.app.ID)
	if err != nil {
		t.Fatal(err)
	}
	if second.DelegationFact.RelayHops[0].Relay.ComponentID == "malicious/local-mutation" || second.RuntimeAttempt.Prepared.PayloadDigest == first.RuntimeAttempt.Prepared.PayloadDigest || second.ApplicationAttempt.DomainReservation.ID == "malicious/local-reservation" {
		t.Fatal("process-local fake leaked mutable nested Fact storage")
	}
	// The fake exposes only the Fact Port; it has no certification, production
	// durability or dispatch API to upgrade itself into authority.
	var port any = f.bindings
	if _, ok := port.(runtimeports.OperationGovernancePortV3); ok {
		t.Fatal("Harness binding fake unexpectedly exposed Runtime dispatch authority")
	}
}

func TestModelTurnBindingFakeV3DeepCopiesUnknownAndInspectionProvenance(t *testing.T) {
	f := newDomainFixtureV3(t)
	if _, err := f.adapter.BindPrepared(context.Background(), f.preparedRequest()); err != nil {
		t.Fatal(err)
	}
	unknownRequest := f.unknownRequest(t)
	if _, err := f.adapter.MarkUnknown(context.Background(), unknownRequest); err != nil {
		t.Fatal(err)
	}
	first, err := f.bindings.InspectModelTurnOperationBindingV3(context.Background(), f.candidate.Run.Scope, modelTurnStepV3, f.app.ID)
	if err != nil {
		t.Fatal(err)
	}
	if first.UnknownAuthorization == nil || first.UnknownAuthorization.Permit.Operation.ExecutionScope.SandboxLease == nil {
		t.Fatal("unknown binding lacks nested authorization scope fixture")
	}
	originalLeaseEpoch := first.UnknownAuthorization.Permit.Operation.ExecutionScope.SandboxLease.Epoch
	first.UnknownAuthorization.Permit.Operation.ExecutionScope.SandboxLease.Epoch++
	second, err := f.bindings.InspectModelTurnOperationBindingV3(context.Background(), f.candidate.Run.Scope, modelTurnStepV3, f.app.ID)
	if err != nil {
		t.Fatal(err)
	}
	if second.UnknownAuthorization.Permit.Operation.ExecutionScope.SandboxLease.Epoch != originalLeaseEpoch {
		t.Fatal("unknown authorization scope aliases fake storage")
	}

	settlementRequest := f.postpreparedUnknownSettlementRequest(t, unknownRequest)
	if _, err := f.adapter.ApplySettlement(context.Background(), settlementRequest); err != nil {
		t.Fatal(err)
	}
	settled, err := f.bindings.InspectModelTurnOperationBindingV3(context.Background(), f.candidate.Run.Scope, modelTurnStepV3, f.app.ID)
	if err != nil {
		t.Fatal(err)
	}
	if settled.Settlement == nil || settled.Settlement.Attempt.Delegation == nil || settled.Settlement.InspectionEffect == nil || settled.Settlement.InspectionSettlement == nil || len(settled.Settlement.InspectionSettlement.Evidence) == 0 {
		t.Fatal("settled unknown binding lacks nested inspection provenance fixture")
	}
	originalDelegation := settled.Settlement.Attempt.Delegation.ID
	originalSequence := settled.Settlement.InspectionSettlement.Evidence[0].Sequence
	settled.Settlement.Attempt.Delegation.ID = "malicious/local-inspection"
	settled.Settlement.InspectionSettlement.Evidence[0].Sequence++
	again, err := f.bindings.InspectModelTurnOperationBindingV3(context.Background(), f.candidate.Run.Scope, modelTurnStepV3, f.app.ID)
	if err != nil {
		t.Fatal(err)
	}
	if again.Settlement.Attempt.Delegation.ID != originalDelegation || again.Settlement.InspectionSettlement.Evidence[0].Sequence != originalSequence {
		t.Fatal("inspection provenance aliases fake storage")
	}
}

func TestModelTurnDomainAdapterV3PrepreparedUnknownFailsOnlyExactCandidate(t *testing.T) {
	f := newDomainFixtureV3(t)
	request := f.undispatchedSettlementRequest(t)
	f.store.LoseNextSessionCASReply = true
	f.bindings.LoseNextCreateReply = true
	settled, err := f.adapter.ApplySettlement(context.Background(), request)
	if err != nil || settled.State != applicationports.OperationDomainSettledV3 || settled.Revision != 1 {
		t.Fatalf("pre-prepared unknown did not persist exact failure: %#v err=%v", settled, err)
	}
	replayed, err := f.adapter.ApplySettlement(context.Background(), request)
	if err != nil || replayed.Digest != settled.Digest {
		t.Fatalf("pre-prepared unknown exact replay drifted: %#v err=%v", replayed, err)
	}
	session, err := f.store.InspectSessionV2(context.Background(), f.candidate.Run, f.candidate.SessionRef)
	if err != nil || session.Phase != contract.SessionTerminalV2 || session.CompletionClaim != contract.ClaimFailed || session.Execution != nil || session.UndispatchedSettlement == nil {
		t.Fatalf("pre-prepared failure escaped undispatched terminal boundary: %#v err=%v", session, err)
	}

	other := newDomainFixtureV3(t)
	forged := other.undispatchedSettlementRequest(t)
	wrong := contract.NewSettledTurnFailureV2(contract.CandidateRefV2{ID: "candidate-other", Revision: 1, Digest: testkit.Digest("candidate-other")}, "praxis.harness/dispatch-not-applied", []byte("wrong candidate"))
	wrongDomain, err := contract.NewSettledTurnDomainResultV2(wrong)
	if err != nil {
		t.Fatal(err)
	}
	forged.DomainResult = &wrongDomain
	forged.Settlement.DomainResultSchema = &wrongDomain.Schema
	forged.Settlement.DomainResultDigest = wrongDomain.ContentDigest
	forged.Attempt.Settlement = &forged.Settlement
	if _, err := other.adapter.ApplySettlement(context.Background(), forged); err == nil {
		t.Fatal("pre-prepared unknown failed a candidate other than the persisted Effect candidate")
	}
	stored, err := other.store.InspectSessionV2(context.Background(), other.candidate.Run, other.candidate.SessionRef)
	if err != nil || stored.Phase != contract.SessionModelDispatchReservedV2 {
		t.Fatalf("malicious pre-prepared result mutated Session: %#v err=%v", stored, err)
	}
}

func TestModelTurnDomainAdapterV3PostpreparedUnknownRequiresInspectProvenance(t *testing.T) {
	f := newDomainFixtureV3(t)
	if _, err := f.adapter.BindPrepared(context.Background(), f.preparedRequest()); err != nil {
		t.Fatal(err)
	}
	unknownRequest := f.unknownRequest(t)
	f.store.LoseNextSessionCASReply = true
	f.bindings.LoseNextCASReply = true
	unknown, err := f.adapter.MarkUnknown(context.Background(), unknownRequest)
	if err != nil || unknown.State != applicationports.OperationDomainUnknownV3 {
		t.Fatalf("prepared unknown did not enter reconciliation: %#v err=%v", unknown, err)
	}
	settlementRequest := f.postpreparedUnknownSettlementRequest(t, unknownRequest)
	missing := settlementRequest
	missingRuntime := *settlementRequest.RuntimeAttempt
	missingSettlement := *missingRuntime.Settlement
	missingSettlement.InspectionEffect = nil
	missingSettlement.InspectionSettlement = nil
	missingRuntime.Settlement = &missingSettlement
	missing.RuntimeAttempt = &missingRuntime
	missing.Settlement = missingSettlement
	missing.Attempt.Settlement = &missingSettlement
	if _, err := f.adapter.ApplySettlement(context.Background(), missing); !core.HasReason(err, core.ReasonEffectUnknownOutcome) {
		t.Fatalf("post-prepared unknown settled without exact Inspect provenance: %v", err)
	}
	f.store.LoseNextSessionCASReply = true
	f.bindings.LoseNextCASReply = true
	settled, err := f.adapter.ApplySettlement(context.Background(), settlementRequest)
	if err != nil || settled.State != applicationports.OperationDomainSettledV3 || settled.Revision != 3 {
		t.Fatalf("inspected unknown did not settle: %#v err=%v", settled, err)
	}
	session, err := f.store.InspectSessionV2(context.Background(), f.candidate.Run, f.candidate.SessionRef)
	if err != nil || session.CompletionClaim != contract.ClaimFailed {
		t.Fatalf("inspected unknown did not produce exact failed terminal Session: %#v err=%v", session, err)
	}
}

func TestModelTurnOperationBindingV3RecomputesInternalCausalBasis(t *testing.T) {
	t.Run("observation", func(t *testing.T) {
		f := newDomainFixtureV3(t)
		_, _ = f.adapter.BindPrepared(context.Background(), f.preparedRequest())
		observed := f.observedRequest(t)
		_, _ = f.adapter.BindObserved(context.Background(), observed)
		fact, _ := f.bindings.InspectModelTurnOperationBindingV3(context.Background(), f.candidate.Run.Scope, modelTurnStepV3, f.app.ID)
		forged := fact
		runtimeAttempt := *fact.RuntimeAttempt
		observation := *runtimeAttempt.Observation
		observation.Digest = testkit.Digest("swapped-observation")
		runtimeAttempt.Observation = &observation
		forged.RuntimeAttempt = &runtimeAttempt
		if err := forged.Validate(); !core.HasReason(err, core.ReasonEvidenceConflict) {
			t.Fatalf("same public Basis hid swapped Observation: %v", err)
		}
	})
	t.Run("unknown authorization", func(t *testing.T) {
		f := newDomainFixtureV3(t)
		_, _ = f.adapter.BindPrepared(context.Background(), f.preparedRequest())
		_, _ = f.adapter.MarkUnknown(context.Background(), f.unknownRequest(t))
		fact, _ := f.bindings.InspectModelTurnOperationBindingV3(context.Background(), f.candidate.Run.Scope, modelTurnStepV3, f.app.ID)
		forged := fact
		authorization := *fact.UnknownAuthorization
		authorization.EffectFactRevision++
		forged.UnknownAuthorization = &authorization
		if err := forged.Validate(); !core.HasReason(err, core.ReasonEvidenceConflict) {
			t.Fatalf("same public Basis hid swapped unknown Authorization: %v", err)
		}
	})
	t.Run("settlement and domain result", func(t *testing.T) {
		f := newDomainFixtureV3(t)
		_, _ = f.adapter.BindPrepared(context.Background(), f.preparedRequest())
		observed := f.observedRequest(t)
		_, _ = f.adapter.BindObserved(context.Background(), observed)
		_, _ = f.adapter.ApplySettlement(context.Background(), f.settlementRequest(t, observed))
		fact, _ := f.bindings.InspectModelTurnOperationBindingV3(context.Background(), f.candidate.Run.Scope, modelTurnStepV3, f.app.ID)
		forged := fact
		candidateRef, _ := f.candidate.RefV2()
		newOutput := f.candidate.Input
		newOutput.Inline = append([]byte(nil), f.candidate.Input.Inline...)
		newOutput.Inline = append(newOutput.Inline, '\n')
		newOutput.Length = uint64(len(newOutput.Inline))
		newOutput.ContentDigest = core.DigestBytes(newOutput.Inline)
		newDomain, err := contract.NewSettledTurnDomainResultV2(contract.SettledTurnResultV2{ContractVersion: contract.SettledTurnResultContractV2, Candidate: candidateRef, State: contract.SettledTurnCompletedV2, Output: &newOutput})
		if err != nil {
			t.Fatal(err)
		}
		newSettlement := *fact.Settlement
		newSettlement.DomainResultSchema = &newDomain.Schema
		newSettlement.DomainResultDigest = newDomain.ContentDigest
		forged.Settlement = &newSettlement
		forged.DomainResult = &newDomain
		forged.ApplicationAttempt.Settlement = &newSettlement
		runtimeAttempt := *fact.RuntimeAttempt
		runtimeAttempt.Settlement = &newSettlement
		forged.RuntimeAttempt = &runtimeAttempt
		if err := forged.Validate(); !core.HasReason(err, core.ReasonEvidenceConflict) {
			t.Fatalf("same public Basis hid coherent swapped Settlement/DomainResult: %v", err)
		}
	})
}

func TestModelTurnDomainAdapterV3RejectsEndpointSessionRelayAndStepBorrowing(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*domainFixtureV3)
	}{
		{"endpoint", func(f *domainFixtureV3) { f.delegation.EndpointID = "endpoint-swapped" }},
		{"session", func(f *domainFixtureV3) { f.delegation.RuntimeSessionRef = "session-swapped" }},
		{"relay host", func(f *domainFixtureV3) {
			other := f.candidate.Endpoint.Binding
			other.ComponentID = "custom/other-host"
			other.ManifestDigest = testkit.Digest("other-host-manifest")
			other.ArtifactDigest = testkit.Digest("other-host-artifact")
			f.delegation.HostAdapter = other
			f.delegation.RelayHops = []runtimeports.ExecutionRelayHopV2{{Sequence: 1, Relay: other}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newDomainFixtureV3(t)
			tc.mutate(&f)
			ref, err := f.delegation.RefV2()
			if err != nil {
				t.Fatal(err)
			}
			f.runtime.Prepared.DeclaredDelegation = ref
			f.runtime.Prepared, err = runtimeports.SealPreparedProviderAttemptRefV2(f.runtime.Prepared)
			if err != nil {
				t.Fatal(err)
			}
			f.prepared.Prepared = f.runtime.Prepared
			request := f.preparedRequest()
			if err := request.Validate(); err != nil {
				t.Fatalf("generic Application contract rejected fixture before Harness route check: %v", err)
			}
			if _, err := f.adapter.BindPrepared(context.Background(), request); err == nil {
				t.Fatal("Harness accepted a delegation routed to another endpoint, Session or relay host")
			}
		})
	}
	f := newDomainFixtureV3(t)
	request := f.preparedRequest()
	request.StepKind = "custom.component/model-turn"
	request.Attempt.StepKind = request.StepKind
	request.Attempt.Descriptor.Kind = request.StepKind
	request.Attempt.RoutingDigest, _ = core.CanonicalJSONDigest("praxis.application.governed-operation-attempt", applicationcontract.GovernedOperationAttemptContractVersionV3, "OperationRoutingBindingsV3", struct {
		StepKind        runtimeports.NamespacedNameV2           `json:"step_kind"`
		Descriptor      applicationcontract.StepDescriptorRefV2 `json:"descriptor"`
		PlannedProvider runtimeports.ProviderBindingRefV2       `json:"planned_provider"`
		DomainAdapter   runtimeports.ProviderBindingRefV2       `json:"domain_adapter"`
		PlanAuthority   runtimeports.AuthorityBindingRefV2      `json:"plan_authority"`
	}{request.StepKind, request.Attempt.Descriptor, request.Attempt.PlannedProvider, request.Attempt.DomainAdapter, request.Attempt.PlanAuthority})
	resealAttemptReservationForTestV3(t, &request.Attempt)
	if _, err := f.adapter.BindPrepared(context.Background(), request); err == nil {
		t.Fatal("a future custom component borrowed the Harness-only adapter")
	}

	f = newDomainFixtureV3(t)
	request = f.preparedRequest()
	otherAdapter := request.Attempt.DomainAdapter
	otherAdapter.ComponentID = "custom.component/domain-adapter"
	otherAdapter.ManifestDigest = testkit.Digest("custom-domain-adapter-manifest")
	otherAdapter.ArtifactDigest = testkit.Digest("custom-domain-adapter-artifact")
	request.Attempt.DomainAdapter = otherAdapter
	request.Attempt.RoutingDigest, _ = core.CanonicalJSONDigest("praxis.application.governed-operation-attempt", applicationcontract.GovernedOperationAttemptContractVersionV3, "OperationRoutingBindingsV3", struct {
		StepKind        runtimeports.NamespacedNameV2           `json:"step_kind"`
		Descriptor      applicationcontract.StepDescriptorRefV2 `json:"descriptor"`
		PlannedProvider runtimeports.ProviderBindingRefV2       `json:"planned_provider"`
		DomainAdapter   runtimeports.ProviderBindingRefV2       `json:"domain_adapter"`
		PlanAuthority   runtimeports.AuthorityBindingRefV2      `json:"plan_authority"`
	}{request.StepKind, request.Attempt.Descriptor, request.Attempt.PlannedProvider, otherAdapter, request.Attempt.PlanAuthority})
	resealAttemptReservationForTestV3(t, &request.Attempt)
	if err := request.Validate(); err != nil {
		t.Fatalf("generic request rejected alternate valid Domain Adapter before Harness ownership check: %v", err)
	}
	if _, err := f.adapter.BindPrepared(context.Background(), request); err == nil {
		t.Fatal("Harness adapter accepted an attempt routed to another Domain Adapter owner")
	}
}

func resealAttemptReservationForTestV3(t *testing.T, attempt *applicationcontract.GovernedOperationAttemptRefV3) {
	t.Helper()
	if attempt.DomainReservation == nil {
		t.Fatal("fixture reservation missing")
	}
	reservation := *attempt.DomainReservation
	reservation.StepKind, reservation.Descriptor, reservation.DomainAdapter = attempt.StepKind, attempt.Descriptor, attempt.DomainAdapter
	sealed, err := applicationcontract.SealOperationDomainReservationRefV3(reservation)
	if err != nil {
		t.Fatal(err)
	}
	attempt.DomainReservation = &sealed
}

// TestApplicationCoordinatorToModelTurnDomainV3 is a cross-module black-box
// test: the real Application coordinator drives the real persistent Harness
// adapter. Runtime public Port doubles remain deterministic and carry no
// production-authority claim.
func TestApplicationCoordinatorToModelTurnDomainV3(t *testing.T) {
	f := newDomainFixtureV3(t)
	resetFixtureForCoordinatorV3(t, &f)
	provider := f.candidate.Provider
	domainAdapter := f.candidate.Endpoint.Binding
	plan := applicationcontract.WorkflowPlanV2{
		ContractVersion: applicationcontract.WorkflowContractVersionV2, ID: "plan-model-turn-integration", Revision: 1,
		CommandID: "command-model-turn-integration", CommandPayloadDigest: testkit.Digest("command-model-turn-integration"),
		Target: f.candidate.Run.Scope, Authority: f.intent.Authority,
		Steps: []applicationcontract.WorkflowStepV2{{
			ID: "step-model-turn", Kind: modelTurnStepV3, Descriptor: f.app.Descriptor, ExecutionClass: applicationcontract.StepGovernedEffectV2,
			Required: true, Dependencies: []string{}, Payload: f.intent.Payload, Provider: &provider, DomainAdapter: &domainAdapter,
		}},
		CreatedUnixNano: f.now.UnixNano(), ExpiresUnixNano: f.now.Add(time.Hour).UnixNano(),
	}
	journal, err := applicationcontract.NewWorkflowJournalV2("journal-model-turn-integration", plan, f.now.UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	journalStore := applicationfakes.NewFactStoreV2()
	journalStore.Clock = func() time.Time { return f.now.Add(10 * time.Millisecond) }
	if _, err := journalStore.CreateWorkflowJournalV2(context.Background(), plan, journal); err != nil {
		t.Fatal(err)
	}
	initial, err := applicationcontract.NewGovernedOperationAttemptFactV3(
		"application-attempt-integration", plan, journal, "step-model-turn", 1, f.intent.Operation, f.intent,
		applicationcontract.OperationDispatchPlanV3{PermitID: f.authorization.Permit.ID, AttemptID: f.authorization.Permit.AttemptID, PermitTTLNanos: int64(30 * time.Second)},
		applicationcontract.ExecutionDelegationPlanV3{
			ContractVersion: applicationcontract.GovernedOperationAttemptContractVersionV3, DelegationID: "delegation-model-turn-integration",
			HostAdapter: f.candidate.Endpoint.Binding, RelayHops: []runtimeports.ExecutionRelayHopV2{{Sequence: 1, Relay: f.candidate.Endpoint.Binding}},
			EndpointID: f.candidate.Endpoint.ID, RuntimeSessionRef: f.candidate.SessionRef,
			HostBindingExpiresUnixNano: f.now.Add(45 * time.Second).UnixNano(), ProviderBindingExpiresUnixNano: f.now.Add(45 * time.Second).UnixNano(), DelegationTTLNanos: int64(20 * time.Second),
		},
		f.now.UnixNano(),
	)
	if err != nil {
		t.Fatal(err)
	}
	runtime := &integrationRuntimeV3{now: f.now.Add(10 * time.Millisecond), issued: f.authorization}
	attemptStore := applicationfakes.NewGovernedOperationAttemptStoreV3()
	routerClock := func() time.Time { return f.now.Add(10 * time.Millisecond) }
	router, err := application.NewOperationDomainRouterV3(routerClock, fixedDomainCurrentnessV3{adapter: f.candidate.Endpoint.Binding, now: routerClock()})
	if err != nil {
		t.Fatal(err)
	}
	if err := router.RegisterOperationDomainV3(context.Background(), application.OperationDomainAdapterRegistrationV3{StepKind: modelTurnStepV3, Descriptor: f.app.Descriptor, Adapter: f.candidate.Endpoint.Binding, Port: f.adapter}); err != nil {
		t.Fatal(err)
	}
	coordinator, err := application.NewGovernedOperationCoordinatorV3(application.GovernedOperationCoordinatorConfigV3{
		Attempts: attemptStore, Journals: journalStore, Admission: runtime, Governance: runtime, Delegations: runtime,
		Execution: runtime, Observations: runtime, Settlements: runtime, DomainResolver: router,
		Clock: func() time.Time { return f.now.Add(10 * time.Millisecond) },
	})
	if err != nil {
		t.Fatal(err)
	}
	f.store.LoseNextSessionCASReply = true
	f.bindings.LoseNextCreateReply = true
	f.bindings.LoseNextCASReply = true
	observed, err := coordinator.StartGovernedOperationV3(context.Background(), application.StartGovernedOperationRequestV3{Plan: plan, Attempt: initial})
	if err != nil {
		t.Fatal(err)
	}
	if observed.Attempt.State != applicationcontract.OperationProviderObservedV3 || observed.Domain == nil || observed.Domain.State != applicationports.OperationDomainObservedV3 || runtime.executeCalls != 1 {
		t.Fatalf("Application to Harness observed loop did not converge: %#v execute=%d", observed, runtime.executeCalls)
	}
	session, err := f.store.InspectSessionV2(context.Background(), f.candidate.Run, f.candidate.SessionRef)
	if err != nil || session.Phase != contract.SessionWaitingSettlementV2 {
		t.Fatalf("real Harness adapter did not persist waiting settlement: %#v err=%v", session, err)
	}
	candidateRef, _ := f.candidate.RefV2()
	domain, err := contract.NewSettledTurnDomainResultV2(contract.SettledTurnResultV2{ContractVersion: contract.SettledTurnResultContractV2, Candidate: candidateRef, State: contract.SettledTurnCompletedV2, Output: &f.candidate.Input})
	if err != nil {
		t.Fatal(err)
	}
	delegation := *observed.Attempt.PreparedDelegation
	settlementAttempt := runtimeports.OperationDispatchAttemptRefV3{
		OperationDigest: observed.Attempt.Intent.OperationDigest, EffectID: observed.Attempt.Intent.EffectID,
		IntentRevision: observed.Attempt.Intent.IntentRevision, IntentDigest: observed.Attempt.Intent.IntentDigest,
		PermitID: observed.Attempt.BegunAuthorization.Permit.ID, PermitRevision: observed.Attempt.BegunAuthorization.Permit.Revision,
		PermitDigest: observed.Attempt.BegunAuthorization.Attempt.PermitDigest, AttemptID: observed.Attempt.BegunAuthorization.Attempt.AttemptID, Delegation: &delegation,
	}
	submission := runtimeports.OperationSettlementSubmissionV3{
		ID: "settlement-model-turn-integration", Revision: 1, Attempt: settlementAttempt,
		Owner: observed.Attempt.IntentValue.Owners[2], Disposition: runtimeports.OperationSettlementAppliedV3,
		Observation: observed.Attempt.Observation, Evidence: []runtimeports.EvidenceRecordRefV2{observed.Attempt.Observation.Evidence},
		DomainResult: &domain, SettledUnixNano: f.now.Add(time.Second).UnixNano(),
	}
	f.store.LoseNextSessionCASReply = true
	f.bindings.LoseNextCASReply = true
	settled, err := coordinator.SettleGovernedOperationV3(context.Background(), application.SettleGovernedOperationRequestV3{Plan: plan, AttemptID: initial.ID, Submission: submission})
	if err != nil {
		t.Fatal(err)
	}
	if settled.Attempt.State != applicationcontract.OperationSettledV3 || settled.Domain == nil || settled.Domain.State != applicationports.OperationDomainSettledV3 || runtime.executeCalls != 1 {
		t.Fatalf("Application to Harness settlement did not converge exactly once: %#v execute=%d", settled, runtime.executeCalls)
	}
	session, err = f.store.InspectSessionV2(context.Background(), f.candidate.Run, f.candidate.SessionRef)
	if err != nil || session.Phase != contract.SessionTerminalV2 || session.CompletionClaim != contract.ClaimCompleted {
		t.Fatalf("cross-module settlement did not terminally persist Harness Session: %#v err=%v", session, err)
	}
}

func TestApplicationCoordinatorPrepreparedUnknownToHarnessFailedV3(t *testing.T) {
	f := newDomainFixtureV3(t)
	resetFixtureForCoordinatorV3(t, &f)
	provider := f.candidate.Provider
	domainAdapter := f.candidate.Endpoint.Binding
	plan := applicationcontract.WorkflowPlanV2{
		ContractVersion: applicationcontract.WorkflowContractVersionV2, ID: "plan-model-turn-undispatched", Revision: 1,
		CommandID: "command-model-turn-undispatched", CommandPayloadDigest: testkit.Digest("command-model-turn-undispatched"),
		Target: f.candidate.Run.Scope, Authority: f.intent.Authority,
		Steps:           []applicationcontract.WorkflowStepV2{{ID: "step-model-turn", Kind: modelTurnStepV3, Descriptor: f.app.Descriptor, ExecutionClass: applicationcontract.StepGovernedEffectV2, Required: true, Dependencies: []string{}, Payload: f.intent.Payload, Provider: &provider, DomainAdapter: &domainAdapter}},
		CreatedUnixNano: f.now.UnixNano(), ExpiresUnixNano: f.now.Add(time.Hour).UnixNano(),
	}
	journal, err := applicationcontract.NewWorkflowJournalV2("journal-model-turn-undispatched", plan, f.now.UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	journalStore := applicationfakes.NewFactStoreV2()
	journalStore.Clock = func() time.Time { return f.now.Add(10 * time.Millisecond) }
	if _, err := journalStore.CreateWorkflowJournalV2(context.Background(), plan, journal); err != nil {
		t.Fatal(err)
	}
	initial, err := applicationcontract.NewGovernedOperationAttemptFactV3(
		"application-attempt-undispatched", plan, journal, "step-model-turn", 1, f.intent.Operation, f.intent,
		applicationcontract.OperationDispatchPlanV3{PermitID: f.authorization.Permit.ID, AttemptID: f.authorization.Permit.AttemptID, PermitTTLNanos: int64(30 * time.Second)},
		applicationcontract.ExecutionDelegationPlanV3{ContractVersion: applicationcontract.GovernedOperationAttemptContractVersionV3, DelegationID: "delegation-model-turn-undispatched", HostAdapter: domainAdapter, RelayHops: []runtimeports.ExecutionRelayHopV2{{Sequence: 1, Relay: domainAdapter}}, EndpointID: f.candidate.Endpoint.ID, RuntimeSessionRef: f.candidate.SessionRef, HostBindingExpiresUnixNano: f.now.Add(45 * time.Second).UnixNano(), ProviderBindingExpiresUnixNano: f.now.Add(45 * time.Second).UnixNano(), DelegationTTLNanos: int64(20 * time.Second)},
		f.now.UnixNano(),
	)
	if err != nil {
		t.Fatal(err)
	}
	runtime := &integrationRuntimeV3{now: f.now.Add(10 * time.Millisecond), issued: f.authorization, prepareUnknown: true}
	routerClock := func() time.Time { return f.now.Add(10 * time.Millisecond) }
	router, _ := application.NewOperationDomainRouterV3(routerClock, fixedDomainCurrentnessV3{adapter: domainAdapter, now: routerClock()})
	if err := router.RegisterOperationDomainV3(context.Background(), application.OperationDomainAdapterRegistrationV3{StepKind: modelTurnStepV3, Descriptor: f.app.Descriptor, Adapter: domainAdapter, Port: f.adapter}); err != nil {
		t.Fatal(err)
	}
	coordinator, err := application.NewGovernedOperationCoordinatorV3(application.GovernedOperationCoordinatorConfigV3{
		Attempts: applicationfakes.NewGovernedOperationAttemptStoreV3(), Journals: journalStore, Admission: runtime, Governance: runtime, Delegations: runtime,
		Execution: runtime, Observations: runtime, Settlements: runtime, DomainResolver: router, Clock: func() time.Time { return f.now.Add(10 * time.Millisecond) },
	})
	if err != nil {
		t.Fatal(err)
	}
	unknown, err := coordinator.StartGovernedOperationV3(context.Background(), application.StartGovernedOperationRequestV3{Plan: plan, Attempt: initial})
	if err != nil {
		t.Fatal(err)
	}
	if unknown.Attempt.State != applicationcontract.OperationDispatchUnknownV3 || unknown.Attempt.Prepared != nil || unknown.Domain != nil || runtime.executeCalls != 0 {
		t.Fatalf("pre-prepared unknown crossed the Harness dispatch boundary: %#v execute=%d", unknown, runtime.executeCalls)
	}
	candidateRef, _ := f.candidate.RefV2()
	domain, err := contract.NewSettledTurnDomainResultV2(contract.NewSettledTurnFailureV2(candidateRef, "praxis.harness/prepare-not-applied", []byte("independent Inspect proved Prepare absent")))
	if err != nil {
		t.Fatal(err)
	}
	inspectionAttempt := unknown.Attempt.UnknownAuthorization.Attempt
	inspectionAttempt.EffectID = "inspect-effect-undispatched"
	inspectionAttempt.IntentDigest = testkit.Digest("inspect-intent-undispatched")
	inspectionAttempt.PermitID = "inspect-permit-undispatched"
	inspectionAttempt.PermitDigest = testkit.Digest("inspect-permit-undispatched")
	inspectionAttempt.AttemptID = "inspect-attempt-undispatched"
	inspectionAttempt.Delegation = nil
	evidence := runtimeports.EvidenceRecordRefV2{LedgerScopeDigest: testkit.Digest("inspect-ledger-undispatched"), Sequence: 1, RecordDigest: testkit.Digest("inspect-record-undispatched")}
	inspection := runtimeports.OperationInspectionSettlementRefV3{ID: "inspect-settlement-undispatched", Revision: 1, Digest: testkit.Digest("inspect-settlement-undispatched"), Attempt: inspectionAttempt, Disposition: runtimeports.OperationSettlementAppliedV3, Owner: unknown.Attempt.IntentValue.Owners[2], Evidence: []runtimeports.EvidenceRecordRefV2{evidence}}
	submission := runtimeports.OperationSettlementSubmissionV3{
		ID: "settlement-model-turn-undispatched", Revision: 1, Attempt: unknown.Attempt.UnknownAuthorization.Attempt,
		Owner: unknown.Attempt.IntentValue.Owners[2], Disposition: runtimeports.OperationSettlementFailedV3,
		InspectionEffect: &inspectionAttempt, InspectionSettlement: &inspection, Evidence: []runtimeports.EvidenceRecordRefV2{evidence}, DomainResult: &domain,
		SettledUnixNano: f.now.Add(time.Second).UnixNano(),
	}
	f.store.LoseNextSessionCASReply = true
	f.bindings.LoseNextCreateReply = true
	settled, err := coordinator.SettleGovernedOperationV3(context.Background(), application.SettleGovernedOperationRequestV3{Plan: plan, AttemptID: initial.ID, Submission: submission})
	if err != nil {
		t.Fatal(err)
	}
	if settled.Domain == nil || settled.Domain.State != applicationports.OperationDomainSettledV3 || runtime.executeCalls != 0 {
		t.Fatalf("pre-prepared unknown did not close through Harness failed-only path: %#v execute=%d", settled, runtime.executeCalls)
	}
	session, err := f.store.InspectSessionV2(context.Background(), f.candidate.Run, f.candidate.SessionRef)
	if err != nil || session.Phase != contract.SessionTerminalV2 || session.CompletionClaim != contract.ClaimFailed || session.Execution != nil || session.UndispatchedSettlement == nil {
		t.Fatalf("pre-prepared unknown did not persist failed-only terminal Session: %#v err=%v", session, err)
	}
}

func resetFixtureForCoordinatorV3(t *testing.T, fixture *domainFixtureV3) {
	resetFixtureForCoordinatorWithClockV3(t, fixture, func() time.Time { return fixture.now.Add(10 * time.Millisecond) })
}

func resetFixtureForCoordinatorWithClockV3(t *testing.T, fixture *domainFixtureV3, clock func() time.Time) {
	t.Helper()
	store := fakes.NewGovernedStoreV2()
	store.Clock = func() time.Time { return fixture.now }
	loop, err := kernel.NewGovernedLoopV2(kernel.GovernedLoopConfigV2{Sessions: store, Candidates: store, Clock: func() time.Time { return fixture.now }, CandidateTTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	candidate := fixture.candidate
	if _, err := loop.PrepareInitialCandidateV2(context.Background(), kernel.PrepareInitialCandidateRequestV2{Run: candidate.Run, Endpoint: candidate.Endpoint, SessionID: candidate.SessionRef, CandidateID: candidate.ID, Input: candidate.Input, ContextRef: candidate.ContextRef, ContextDigest: candidate.ContextDigest, Provider: candidate.Provider, CreatedUnixNano: candidate.CreatedUnixNano, ExpiresUnixNano: candidate.ExpiresUnixNano}); err != nil {
		t.Fatal(err)
	}
	bindings := fakes.NewModelTurnOperationBindingStoreV3()
	adapter, err := applicationadapter.NewModelTurnDomainAdapterV3(applicationadapter.ModelTurnDomainAdapterConfigV3{StepKind: modelTurnStepV3, Adapter: fixture.candidate.Endpoint.Binding, Bindings: bindings, Reservations: store, Sessions: store, Candidates: store, Turns: &kernel.GovernedTurnStateCoordinatorV2{Sessions: store}, Clock: clock})
	if err != nil {
		t.Fatal(err)
	}
	fixture.store, fixture.bindings, fixture.adapter = store, bindings, adapter
}

func initialReservationAttemptV3(f domainFixtureV3, id string) applicationcontract.GovernedOperationAttemptRefV3 {
	attempt := f.app
	attempt.ID, attempt.Revision, attempt.State, attempt.Digest = id, 1, applicationcontract.OperationIntentRecordedV3, testkit.Digest("intent-recorded-"+id)
	attempt.AuthorizationDigest = testkit.Digest("authorization-not-yet-created")
	attempt.DomainReservation, attempt.Settlement = nil, nil
	attempt.DispatchUnknown = false
	return attempt
}

func reservationRequestV3(f domainFixtureV3, attempt applicationcontract.GovernedOperationAttemptRefV3) applicationports.ReserveOperationIntentRequestV3 {
	return applicationports.ReserveOperationIntentRequestV3{StepKind: attempt.StepKind, Descriptor: attempt.Descriptor, DomainAdapter: attempt.DomainAdapter, Attempt: attempt, Intent: f.intent}
}

func assertWaitingUnreservedV3(t *testing.T, f domainFixtureV3) {
	t.Helper()
	session, err := f.store.InspectSessionV2(context.Background(), f.candidate.Run, f.candidate.SessionRef)
	if err != nil || session.Phase != contract.SessionWaitingModelDispatchV2 || session.Revision != 2 || session.DomainReservation != nil {
		t.Fatalf("rejected reservation mutated Session: %#v err=%v", session, err)
	}
}

type integrationRuntimeV3 struct {
	mu             sync.Mutex
	now            time.Time
	issued         runtimeports.OperationDispatchAuthorizationV3
	auth           *runtimeports.OperationDispatchAuthorizationV3
	admission      *runtimeports.OperationEffectAdmissionReceiptV3
	declaredFact   *runtimeports.ExecutionDelegationFactV2
	declared       *runtimeports.ExecutionDelegationRefV2
	attestation    *runtimeports.ProviderPreparationAttestationV2
	prepared       *runtimeports.PreparedExecutionGovernanceResultV2
	local          *runtimeports.ProviderAttemptObservationV2
	observation    *runtimeports.ProviderAttemptObservationRefV2
	settlement     *runtimeports.OperationSettlementRefV3
	executeCalls   int
	prepareUnknown bool
}

type fixedDomainCurrentnessV3 struct {
	adapter runtimeports.ProviderBindingRefV2
	now     time.Time
}

func (f fixedDomainCurrentnessV3) InspectOperationDomainAdapterCurrentV3(_ context.Context, adapter runtimeports.ProviderBindingRefV2) (applicationports.OperationDomainAdapterAuthorizationV3, error) {
	if adapter != f.adapter {
		return applicationports.OperationDomainAdapterAuthorizationV3{}, core.NewError(core.ErrorNotFound, core.ReasonComponentMissing, "test Domain Adapter authorization not found")
	}
	return applicationports.SealOperationDomainAdapterAuthorizationV3(applicationports.OperationDomainAdapterAuthorizationV3{
		ContractVersion: applicationports.OperationDomainContractVersionV3,
		Adapter:         adapter,
		Revision:        1,
		State:           applicationports.OperationDomainAdapterAuthorizedV3,
		IssuedUnixNano:  f.now.Add(-time.Second).UnixNano(),
		ExpiresUnixNano: f.now.Add(time.Second).UnixNano(),
	})
}

func (r *integrationRuntimeV3) AdmitOperationEffectV3(_ context.Context, intent runtimeports.OperationEffectIntentV3) (runtimeports.OperationEffectAdmissionReceiptV3, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.admission == nil {
		intentDigest, _ := intent.DigestV3()
		operationDigest, _ := intent.Operation.DigestV3()
		value := runtimeports.OperationEffectAdmissionReceiptV3{OperationDigest: operationDigest, EffectID: intent.ID, IntentRevision: intent.Revision, IntentDigest: intentDigest, FactRevision: 1, State: "accepted"}
		r.admission = &value
	}
	return *r.admission, nil
}

func (r *integrationRuntimeV3) InspectAcceptedOperationEffectV3(context.Context, runtimeports.OperationSubjectV3, core.EffectIntentID) (runtimeports.OperationEffectAdmissionReceiptV3, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.admission == nil {
		return runtimeports.OperationEffectAdmissionReceiptV3{}, integrationNotFoundV3("admission")
	}
	return *r.admission, nil
}

func (r *integrationRuntimeV3) IssueOperationDispatchV3(context.Context, runtimeports.IssueGovernedOperationDispatchRequestV3) (runtimeports.OperationDispatchAuthorizationV3, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	value := r.issued
	r.auth = &value
	return value, nil
}

func (r *integrationRuntimeV3) BeginOperationDispatchV3(context.Context, runtimeports.BeginGovernedOperationDispatchRequestV3) (runtimeports.OperationDispatchAuthorizationV3, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	value := *r.auth
	value.State = runtimeports.OperationDispatchAuthorizationBegunV3
	value.PermitFactRevision++
	r.auth = &value
	return value, nil
}

func (r *integrationRuntimeV3) MarkOperationDispatchUnknownV3(context.Context, runtimeports.MarkOperationDispatchUnknownRequestV3) (runtimeports.OperationDispatchAuthorizationV3, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	value := *r.auth
	value.State = runtimeports.OperationDispatchAuthorizationUnknownV3
	value.EffectFactRevision++
	r.auth = &value
	return value, nil
}

func (r *integrationRuntimeV3) InspectOperationDispatchAuthorizationV3(context.Context, runtimeports.OperationSubjectV3, core.EffectIntentID, string) (runtimeports.OperationDispatchAuthorizationV3, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.auth == nil {
		return runtimeports.OperationDispatchAuthorizationV3{}, integrationNotFoundV3("authorization")
	}
	return *r.auth, nil
}

func (r *integrationRuntimeV3) DeclareExecutionDelegationV2(_ context.Context, request runtimeports.DeclareExecutionDelegationRequestV2) (runtimeports.ExecutionDelegationRefV2, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ref, err := request.Delegation.RefV2()
	if err != nil {
		return runtimeports.ExecutionDelegationRefV2{}, err
	}
	fact := request.Delegation
	r.declaredFact, r.declared = &fact, &ref
	return ref, nil
}

func (r *integrationRuntimeV3) InspectDeclaredExecutionV2(context.Context, runtimeports.OperationSubjectV3, string) (runtimeports.ExecutionDelegationRefV2, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.declared == nil {
		return runtimeports.ExecutionDelegationRefV2{}, integrationNotFoundV3("delegation")
	}
	return *r.declared, nil
}

func (r *integrationRuntimeV3) RelayPrepare(_ context.Context, request runtimeports.PrepareGovernedExecutionRequestV2) (runtimeports.ProviderPreparationAttestationV2, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.prepareUnknown {
		return runtimeports.ProviderPreparationAttestationV2{}, core.NewError(core.ErrorUnavailable, core.ReasonEffectUnknownOutcome, "injected Prepare outcome unknown")
	}
	if r.attestation == nil {
		permitDigest, _ := request.Permit.DigestV3()
		operationDigest, _ := request.Intent.Operation.DigestV3()
		preparedID, _ := runtimeports.DerivePreparedProviderAttemptIDV2(request.Delegation.ID, request.Permit.ID, request.Permit.AttemptID)
		prepared, err := runtimeports.SealPreparedProviderAttemptRefV2(runtimeports.PreparedProviderAttemptRefV2{
			ID: preparedID, Revision: 1, DeclaredDelegation: request.Delegation, OperationDigest: operationDigest,
			IntentID: request.Intent.ID, IntentRevision: request.Intent.Revision, IntentDigest: request.Permit.IntentDigest,
			PermitID: request.Permit.ID, PermitRevision: request.Permit.Revision, PermitDigest: permitDigest, AttemptID: request.Permit.AttemptID,
			Provider: request.Permit.Provider, PayloadSchema: request.Intent.Payload.Schema, PayloadDigest: request.Intent.Payload.ContentDigest, PayloadRevision: request.Intent.PayloadRevision,
			PreparedUnixNano: r.now.UnixNano(), ExpiresUnixNano: request.Permit.ExpiresUnixNano,
		})
		if err != nil {
			return runtimeports.ProviderPreparationAttestationV2{}, err
		}
		receipt := runtimeports.OperationEnforcementReceiptV3{ContractVersion: runtimeports.OperationEffectContractVersionV3, PermitID: request.Permit.ID, PermitRevision: request.Permit.Revision, AttemptID: request.Permit.AttemptID, PermitDigest: permitDigest, Operation: request.Intent.Operation, Verifier: request.Permit.EnforcementPoint, ValidatedUnixNano: r.now.UnixNano()}
		value := runtimeports.ProviderPreparationAttestationV2{ContractVersion: runtimeports.ExecutionGovernanceContractVersionV2, Delegation: request.Delegation, Prepared: prepared, Enforcement: receipt, ObservedUnixNano: r.now.UnixNano()}
		r.attestation = &value
	}
	return *r.attestation, nil
}

func (r *integrationRuntimeV3) RelayInspectPrepared(context.Context, runtimeports.InspectPreparedProviderRequestV2) (runtimeports.ProviderPreparationAttestationV2, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.attestation == nil {
		return runtimeports.ProviderPreparationAttestationV2{}, integrationNotFoundV3("prepared relay")
	}
	return *r.attestation, nil
}

func (r *integrationRuntimeV3) CommitPreparedExecutionV2(_ context.Context, request runtimeports.CommitPreparedExecutionRequestV2) (runtimeports.PreparedExecutionGovernanceResultV2, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.prepared == nil {
		delegation := runtimeports.ExecutionDelegationRefV2{ID: request.Declared.ID, Revision: request.Declared.Revision + 1, Digest: testkit.Digest("integration-prepared-delegation")}
		enforcement := runtimeports.PersistedOperationEnforcementRefV3{PermitID: request.Preparation.Prepared.PermitID, PermitRevision: request.Preparation.Prepared.PermitRevision, PermitDigest: request.Preparation.Prepared.PermitDigest, AttemptID: request.Preparation.Prepared.AttemptID, OperationDigest: request.Preparation.Prepared.OperationDigest, Provider: request.Preparation.Prepared.Provider, ReceiptDigest: testkit.Digest("integration-enforcement"), RecordedRevision: 3}
		value := runtimeports.PreparedExecutionGovernanceResultV2{Delegation: delegation, Prepared: request.Preparation.Prepared, Enforcement: enforcement}
		r.prepared = &value
	}
	return *r.prepared, nil
}

func (r *integrationRuntimeV3) InspectPreparedExecutionV2(context.Context, runtimeports.OperationSubjectV3, string, string) (runtimeports.PreparedExecutionGovernanceResultV2, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.prepared == nil {
		return runtimeports.PreparedExecutionGovernanceResultV2{}, integrationNotFoundV3("prepared governance")
	}
	return *r.prepared, nil
}

func (r *integrationRuntimeV3) RelayExecutePrepared(_ context.Context, request runtimeports.ExecutePreparedRequestV2) (runtimeports.ProviderAttemptObservationV2, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.local == nil {
		r.executeCalls++
		value := runtimeports.ProviderAttemptObservationV2{
			ContractVersion: runtimeports.ExecutionGovernanceContractVersionV2, Delegation: request.Delegation, Prepared: request.Prepared,
			Revision: 1, State: runtimeports.ProviderAttemptObservedV2, Payload: request.Intent.Payload, PayloadRevision: request.Intent.PayloadRevision,
			ProviderOperationRef: "provider-operation-integration", SourceRegistrationID: "provider-source-integration", SourceEpoch: 1, SourceSequence: 1,
			Evidence: runtimeports.EvidenceRecordRefV2{LedgerScopeDigest: testkit.Digest("integration-ledger"), Sequence: 1, RecordDigest: testkit.Digest("integration-record")}, ObservedUnixNano: r.now.Add(time.Millisecond).UnixNano(),
		}
		r.local = &value
	}
	return *r.local, nil
}

func (r *integrationRuntimeV3) RelayInspectLocalAttempt(context.Context, runtimeports.InspectLocalProviderAttemptRequestV2) (runtimeports.ProviderAttemptObservationV2, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.local == nil {
		return runtimeports.ProviderAttemptObservationV2{}, integrationNotFoundV3("local attempt")
	}
	return *r.local, nil
}

func (r *integrationRuntimeV3) RecordGovernedProviderObservationV3(_ context.Context, request runtimeports.RecordGovernedProviderObservationRequestV2) (runtimeports.ProviderAttemptObservationRefV2, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ref, err := request.Observation.RefV2()
	if err == nil {
		r.observation = &ref
	}
	return ref, err
}

func (r *integrationRuntimeV3) InspectGovernedProviderObservationV3(context.Context, runtimeports.ExecutionDelegationRefV2, string) (runtimeports.ProviderAttemptObservationRefV2, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.observation == nil {
		return runtimeports.ProviderAttemptObservationRefV2{}, integrationNotFoundV3("observation")
	}
	return *r.observation, nil
}

func (r *integrationRuntimeV3) SettleOperationEffectV3(_ context.Context, _ runtimeports.OperationEffectIntentV3, submission runtimeports.OperationSettlementSubmissionV3) (runtimeports.OperationSettlementRefV3, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.settlement == nil {
		value := runtimeports.OperationSettlementRefV3{ID: submission.ID, Revision: submission.Revision, Digest: testkit.Digest("integration-settlement"), Attempt: submission.Attempt, Disposition: submission.Disposition, Owner: submission.Owner, Observation: submission.Observation, InspectionEffect: submission.InspectionEffect, InspectionSettlement: submission.InspectionSettlement, Evidence: append([]runtimeports.EvidenceRecordRefV2(nil), submission.Evidence...)}
		if submission.DomainResult != nil {
			schema := submission.DomainResult.Schema
			value.DomainResultSchema = &schema
			value.DomainResultDigest = submission.DomainResult.ContentDigest
		}
		r.settlement = &value
	}
	return *r.settlement, nil
}

func (r *integrationRuntimeV3) InspectOperationSettlementV3(context.Context, runtimeports.OperationSubjectV3, core.EffectIntentID) (runtimeports.OperationSettlementRefV3, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.settlement == nil {
		return runtimeports.OperationSettlementRefV3{}, integrationNotFoundV3("settlement")
	}
	return *r.settlement, nil
}

func integrationNotFoundV3(name string) error {
	return core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, name+" not found")
}
