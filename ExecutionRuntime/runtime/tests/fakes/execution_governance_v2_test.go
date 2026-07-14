package fakes_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/fakes"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type operationCurrentReaderV3 struct {
	mu       sync.Mutex
	snapshot ports.OperationGovernanceSnapshotV3
}

func (r *operationCurrentReaderV3) InspectOperationGovernance(_ context.Context, subject ports.OperationSubjectV3) (ports.OperationGovernanceSnapshotV3, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !ports.SameOperationSubjectV3(subject, r.snapshot.Operation) {
		return ports.OperationGovernanceSnapshotV3{}, core.NewError(core.ErrorNotFound, core.ReasonEffectFenceStale, "operation current projection not found")
	}
	return r.snapshot, nil
}

func (r *operationCurrentReaderV3) mutate(change func(*ports.OperationGovernanceSnapshotV3)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	change(&r.snapshot)
}

type operationDispatchReaderV3 struct {
	effects     control.OperationEffectFactPortV3
	delegations ports.ExecutionDelegationFactPortV2
}

type operationEvidenceReaderV3 struct {
	bySource map[ports.EvidenceSourceKeyV2]ports.EvidenceLedgerRecordV2
	byRef    map[ports.EvidenceRecordRefV2]ports.EvidenceLedgerRecordV2
}

func (r operationEvidenceReaderV3) InspectBySource(_ context.Context, key ports.EvidenceSourceKeyV2) (ports.EvidenceLedgerRecordV2, error) {
	record, ok := r.bySource[key]
	if !ok {
		return ports.EvidenceLedgerRecordV2{}, core.NewError(core.ErrorNotFound, core.ReasonEvidenceUnavailable, "operation Evidence source record not found")
	}
	return record, nil
}

func (r operationEvidenceReaderV3) InspectRecord(_ context.Context, ref ports.EvidenceRecordRefV2) (ports.EvidenceLedgerRecordV2, error) {
	record, ok := r.byRef[ref]
	if !ok {
		return ports.EvidenceLedgerRecordV2{}, core.NewError(core.ErrorNotFound, core.ReasonEvidenceUnavailable, "operation Evidence record not found")
	}
	return record, nil
}

func (r operationDispatchReaderV3) InspectOperationDispatch(ctx context.Context, operation ports.OperationSubjectV3, permitID, delegationID string) (ports.OperationDispatchCurrentProjectionV3, error) {
	permit, err := r.effects.InspectOperationDispatchPermitV3(ctx, operation, permitID)
	if err != nil {
		return ports.OperationDispatchCurrentProjectionV3{}, err
	}
	delegation, err := r.delegations.InspectExecutionDelegationV2(ctx, delegationID)
	if err != nil {
		return ports.OperationDispatchCurrentProjectionV3{}, err
	}
	delegationRef, _ := delegation.RefV2()
	projection := ports.OperationDispatchCurrentProjectionV3{
		Operation:          operation,
		Permit:             permit.Permit,
		PermitDigest:       permit.PermitDigest,
		PermitFactRevision: permit.Revision,
		PermitFactState:    string(permit.State),
		Delegation:         delegationRef,
		DelegationState:    delegation.State,
		PreparedAttemptID:  delegation.PreparedAttemptID,
		ExpiresUnixNano:    delegation.ExpiresUnixNano,
	}
	if permit.Enforcement != nil {
		ref, refErr := permit.PersistedEnforcementRefV3()
		if refErr != nil {
			return ports.OperationDispatchCurrentProjectionV3{}, refErr
		}
		projection.Enforcement = &ref
	}
	if delegation.Preparation != nil {
		projection.PreparationDigest, err = core.CanonicalJSONDigest("praxis.runtime.execution-governance", ports.ExecutionGovernanceContractVersionV2, "ProviderPreparationAttestationV2", delegation.Preparation)
		if err != nil {
			return ports.OperationDispatchCurrentProjectionV3{}, err
		}
	}
	return projection, nil
}

type governedProviderV2 struct {
	mu                   sync.Mutex
	current              ports.OperationGovernanceCurrentReaderV3
	dispatch             ports.OperationDispatchCurrentReaderV3
	clock                func() time.Time
	preparations         map[string]ports.ProviderPreparationAttestationV2
	observations         map[string]ports.ProviderAttemptObservationV2
	prepareCalls         int
	executeCalls         int
	executeMethodEntries int
	inspectCalls         int
	loseNextPrepareReply bool
	loseNextExecuteReply bool
}

func (p *governedProviderV2) Prepare(ctx context.Context, request ports.PrepareGovernedExecutionRequestV2) (ports.ProviderPreparationAttestationV2, error) {
	now := p.clock()
	dispatch, err := p.dispatch.InspectOperationDispatch(ctx, request.Intent.Operation, request.Permit.ID, request.Delegation.ID)
	if err != nil {
		return ports.ProviderPreparationAttestationV2{}, err
	}
	current, err := p.current.InspectOperationGovernance(ctx, request.Intent.Operation)
	if err != nil {
		return ports.ProviderPreparationAttestationV2{}, err
	}
	if err := dispatch.ValidateForPrepare(request, current, now); err != nil {
		return ports.ProviderPreparationAttestationV2{}, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if existing, ok := p.preparations[dispatch.PreparedAttemptID]; ok {
		return existing, nil
	}
	operationDigest, _ := request.Intent.Operation.DigestV3()
	permitDigest, _ := request.Permit.DigestV3()
	prepared, err := ports.SealPreparedProviderAttemptRefV2(ports.PreparedProviderAttemptRefV2{
		ID:                 dispatch.PreparedAttemptID,
		Revision:           1,
		DeclaredDelegation: request.Delegation,
		OperationDigest:    operationDigest,
		IntentID:           request.Intent.ID,
		IntentRevision:     request.Intent.Revision,
		IntentDigest:       request.Permit.IntentDigest,
		PermitID:           request.Permit.ID,
		PermitRevision:     request.Permit.Revision,
		PermitDigest:       permitDigest,
		AttemptID:          request.Permit.AttemptID,
		Provider:           request.Permit.Provider,
		PayloadSchema:      request.Intent.Payload.Schema,
		PayloadDigest:      request.Intent.Payload.ContentDigest,
		PayloadRevision:    request.Intent.PayloadRevision,
		PreparedUnixNano:   now.UnixNano(),
		ExpiresUnixNano:    request.Permit.ExpiresUnixNano,
	})
	if err != nil {
		return ports.ProviderPreparationAttestationV2{}, err
	}
	attestation := ports.ProviderPreparationAttestationV2{
		ContractVersion: ports.ExecutionGovernanceContractVersionV2,
		Delegation:      request.Delegation,
		Prepared:        prepared,
		Enforcement: ports.OperationEnforcementReceiptV3{
			ContractVersion: ports.OperationEffectContractVersionV3, PermitID: request.Permit.ID, PermitRevision: request.Permit.Revision,
			AttemptID: request.Permit.AttemptID, PermitDigest: permitDigest, Operation: request.Intent.Operation,
			Verifier: request.Permit.EnforcementPoint, ValidatedUnixNano: now.UnixNano(),
		},
		ObservedUnixNano: now.UnixNano(),
	}
	if err := attestation.ValidateAgainstPrepare(request, now); err != nil {
		return ports.ProviderPreparationAttestationV2{}, err
	}
	p.prepareCalls++
	p.preparations[prepared.ID] = attestation
	if p.loseNextPrepareReply {
		p.loseNextPrepareReply = false
		return ports.ProviderPreparationAttestationV2{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected Prepare reply loss")
	}
	return attestation, nil
}

func (p *governedProviderV2) InspectPrepared(_ context.Context, request ports.InspectPreparedProviderRequestV2) (ports.ProviderPreparationAttestationV2, error) {
	if err := request.Validate(); err != nil {
		return ports.ProviderPreparationAttestationV2{}, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.inspectCalls++
	attestation, ok := p.preparations[request.PreparedAttemptID]
	if !ok {
		return ports.ProviderPreparationAttestationV2{}, core.NewError(core.ErrorNotFound, core.ReasonEvidenceUnavailable, "prepared attempt not found")
	}
	if attestation.Delegation != request.DeclaredDelegation || attestation.Prepared.PermitID != request.PermitID || attestation.Prepared.AttemptID != request.AttemptID {
		return ports.ProviderPreparationAttestationV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "prepared attempt key changed content")
	}
	return attestation, nil
}

func (p *governedProviderV2) ExecutePrepared(ctx context.Context, request ports.ExecutePreparedRequestV2) (ports.ProviderAttemptObservationV2, error) {
	now := p.clock()
	dispatch, err := p.dispatch.InspectOperationDispatch(ctx, request.Intent.Operation, request.Permit.ID, request.Delegation.ID)
	if err != nil {
		return ports.ProviderAttemptObservationV2{}, err
	}
	current, err := p.current.InspectOperationGovernance(ctx, request.Intent.Operation)
	if err != nil {
		return ports.ProviderAttemptObservationV2{}, err
	}
	if err := dispatch.ValidateForExecute(request, current, now); err != nil {
		return ports.ProviderAttemptObservationV2{}, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.executeMethodEntries++
	if existing, ok := p.observations[request.Prepared.ID]; ok {
		return existing, nil
	}
	p.executeCalls++
	payload := opaquePayloadV3("executed")
	observation := ports.ProviderAttemptObservationV2{
		ContractVersion: ports.ExecutionGovernanceContractVersionV2, Delegation: request.Delegation, Prepared: request.Prepared,
		Revision: 1, State: ports.ProviderAttemptObservedV2, Payload: payload, PayloadRevision: 1, ProviderOperationRef: "provider-operation-1",
		SourceRegistrationID: "provider-source-1", SourceEpoch: 1, SourceSequence: 1,
		Evidence:         ports.EvidenceRecordRefV2{LedgerScopeDigest: core.DigestBytes([]byte("ledger-scope")), Sequence: 1, RecordDigest: core.DigestBytes([]byte("record"))},
		ObservedUnixNano: now.UnixNano(),
	}
	if err := observation.ValidateAgainstPrepared(request.Prepared); err != nil {
		return ports.ProviderAttemptObservationV2{}, err
	}
	p.observations[request.Prepared.ID] = observation
	if p.loseNextExecuteReply {
		p.loseNextExecuteReply = false
		return ports.ProviderAttemptObservationV2{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected Execute reply loss")
	}
	return observation, nil
}

func (p *governedProviderV2) isolatedExecutionCounts() (uint64, int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return uint64(p.executeCalls), p.executeMethodEntries
}

func (p *governedProviderV2) InspectLocalAttempt(_ context.Context, request ports.InspectLocalProviderAttemptRequestV2) (ports.ProviderAttemptObservationV2, error) {
	if err := request.Validate(); err != nil {
		return ports.ProviderAttemptObservationV2{}, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.inspectCalls++
	observation, ok := p.observations[request.Prepared.ID]
	if !ok {
		return ports.ProviderAttemptObservationV2{}, core.NewError(core.ErrorNotFound, core.ReasonEvidenceUnavailable, "provider attempt observation not found")
	}
	return observation, nil
}

type operationFixtureV3 struct {
	now         time.Time
	store       *fakes.OperationEffectStoreV3
	delegations *fakes.ExecutionDelegationStoreV2
	current     *operationCurrentReaderV3
	gateway     control.OperationGovernanceGatewayV3
	intent      ports.OperationEffectIntentV3
	accepted    control.OperationEffectFactV3
}

func newOperationFixtureV3(t *testing.T) *operationFixtureV3 {
	return newOperationFixtureForRunV3(t, "", nil)
}

func newOperationFixtureForRunV3(t *testing.T, runID core.AgentRunID, scopeOverride *core.ExecutionScope) *operationFixtureV3 {
	t.Helper()
	now := time.Unix(310_000, 0)
	scope := core.ExecutionScope{
		Identity:       core.AgentIdentityRef{TenantID: "tenant-operation", ID: "identity-operation", Epoch: 1},
		Lineage:        core.LineageRef{ID: "lineage-operation", PlanDigest: core.DigestBytes([]byte("lineage"))},
		Instance:       core.InstanceRef{ID: "instance-operation", Epoch: 1},
		AuthorityEpoch: 1,
	}
	if scopeOverride != nil {
		scope = *scopeOverride
	}
	scopeDigest, _ := ports.ExecutionScopeDigestV2(scope)
	subject := ports.OperationSubjectV3{
		Kind:                      ports.OperationScopeActivationV3,
		ExecutionScope:            scope,
		ExecutionScopeDigest:      scopeDigest,
		ActivationAttemptID:       "activation-attempt-1",
		SubjectRevision:           1,
		CurrentProjectionRef:      "operation-current-1",
		CurrentProjectionDigest:   core.DigestBytes([]byte("operation-current")),
		CurrentProjectionRevision: 1,
	}
	effectKind := ports.EffectKindV2("custom/activation-open")
	if runID != "" {
		subject.Kind = ports.OperationScopeRunV3
		subject.ActivationAttemptID = ""
		subject.RunID = runID
		effectKind = ports.OperationEffectKindExecutionStartV3
	}
	subjectDigest, _ := subject.DigestV3()
	provider := providerBindingV3("custom/provider", "custom/execute")
	manifestDigest := provider.ManifestDigest
	intent := ports.OperationEffectIntentV3{
		ContractVersion:   ports.OperationEffectContractVersionV3,
		ID:                "operation-effect-1",
		Revision:          1,
		Operation:         subject,
		Kind:              effectKind,
		RiskClass:         "custom/controlled",
		ActionScopeDigest: core.DigestBytes([]byte("action-scope")),
		Payload:           opaquePayloadV3("open"),
		PayloadRevision:   1,
		Target:            "execution/provider",
		ConflictDomain: ports.ConflictDomainBindingV2{
			Domain:      "custom/execution-open",
			ScopeClass:  ports.EffectStableScopeTenantV2,
			ScopeDigest: ports.StableTenantScopeDigestV2(scope.Identity.TenantID),
		},
		Owners: []ports.EffectOwnerRefV2{
			{Role: ports.OwnerCleanup, ComponentID: "custom/provider", ManifestDigest: manifestDigest},
			{Role: ports.OwnerEffect, ComponentID: "custom/provider", ManifestDigest: manifestDigest},
			{Role: ports.OwnerSettlement, ComponentID: "custom/provider", ManifestDigest: manifestDigest},
		},
		Provider: provider,
		Authority: ports.AuthorityBindingRefV2{
			Ref: "authority-operation", Digest: core.DigestBytes([]byte("authority")), Revision: 1, Epoch: 1,
		},
		Review: ports.OperationReviewBindingRefV3{
			CaseRef: "review-case-operation", CandidateDigest: core.DigestBytes([]byte("candidate")), CandidateRevision: 1, PolicyDigest: core.DigestBytes([]byte("review-policy")),
		},
		Budget: ports.OperationBudgetBindingRefV3{
			Ref: "budget-operation", Digest: core.DigestBytes([]byte("budget")), Revision: 1, PolicyDigest: core.DigestBytes([]byte("budget-policy")), SubjectDigest: subjectDigest,
		},
		Policy: ports.OperationPolicyBindingRefV3{
			Ref: "policy-operation", Digest: core.DigestBytes([]byte("policy")), Revision: 1, SubjectDigest: subjectDigest,
		},
		Idempotency: ports.IdempotencyBindingV2{
			Key: "operation-key-1", ScopeClass: ports.EffectStableScopeTenantV2, ScopeDigest: ports.StableTenantScopeDigestV2(scope.Identity.TenantID), Class: core.IdempotencyQueryable,
		},
		CredentialLeases: []ports.CredentialLeaseRefV2{},
		ExpiresUnixNano:  now.Add(time.Minute).UnixNano(),
	}
	expires := now.Add(45 * time.Second).UnixNano()
	governanceRef := func(ref string, digest core.Digest) ports.OperationGovernanceFactRefV3 {
		return ports.OperationGovernanceFactRefV3{Ref: ref, Revision: 1, Digest: digest, ExpiresUnixNano: expires}
	}
	snapshot := ports.OperationGovernanceSnapshotV3{
		Operation:           subject,
		Active:              true,
		ProjectionWatermark: 1,
		Identity:            governanceRef("identity-operation", core.DigestBytes([]byte("identity"))),
		Binding:             governanceRef(provider.BindingSetID, core.DigestBytes([]byte("binding"))),
		CurrentScope:        governanceRef(subject.CurrentProjectionRef, subject.CurrentProjectionDigest),
		Authority:           governanceRef(intent.Authority.Ref, intent.Authority.Digest),
		Review: ports.OperationReviewAuthorizationV3{
			Case:              governanceRef(intent.Review.CaseRef, core.DigestBytes([]byte("review-case"))),
			CandidateDigest:   intent.Review.CandidateDigest,
			CandidateRevision: intent.Review.CandidateRevision,
			Verdict:           governanceRef("review-verdict-operation", core.DigestBytes([]byte("review-verdict"))),
			ReviewerAuthority: governanceRef("reviewer-authority-operation", core.DigestBytes([]byte("reviewer-authority"))),
			PolicyDigest:      intent.Review.PolicyDigest,
			ExpiresUnixNano:   expires,
		},
		Budget:                governanceRef(intent.Budget.Ref, intent.Budget.Digest),
		Policy:                governanceRef(intent.Policy.Ref, intent.Policy.Digest),
		Provider:              provider,
		EnforcementPoint:      provider,
		CapabilityGrantDigest: core.DigestBytes([]byte("capability-grant")),
		Credentials:           []ports.OperationCredentialCurrentFactV3{},
		ExpiresUnixNano:       expires,
	}
	current := &operationCurrentReaderV3{snapshot: snapshot}
	clock := func() time.Time { return now }
	store := fakes.NewOperationEffectStoreV3(clock)
	proposed, err := control.NewProposedOperationEffectFactV3(intent, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateOperationEffectV3(context.Background(), proposed); err != nil {
		t.Fatal(err)
	}
	accepted := proposed
	accepted.State = control.OperationEffectAcceptedV3
	accepted.Revision++
	accepted.UpdatedUnixNano = now.UnixNano()
	if _, err := store.CompareAndSwapOperationEffectV3(context.Background(), subject, control.OperationEffectCASRequestV3{ExpectedRevision: proposed.Revision, Next: accepted}); err != nil {
		t.Fatal(err)
	}
	delegations := fakes.NewExecutionDelegationStoreV2(clock)
	return &operationFixtureV3{
		now: now, store: store, delegations: delegations, current: current,
		gateway: control.OperationGovernanceGatewayV3{Effects: store, Current: current, Clock: clock},
		intent:  intent, accepted: accepted,
	}
}

func providerBindingV3(component ports.ComponentIDV2, capability ports.CapabilityNameV2) ports.ProviderBindingRefV2 {
	return ports.ProviderBindingRefV2{
		BindingSetID:       "binding-operation",
		BindingSetRevision: 1,
		ComponentID:        component,
		ManifestDigest:     core.DigestBytes([]byte("manifest-" + string(component))),
		ArtifactDigest:     core.DigestBytes([]byte("artifact-" + string(component))),
		Capability:         capability,
	}
}

func opaquePayloadV3(value string) ports.OpaquePayloadV2 {
	bytes := []byte(value)
	return ports.OpaquePayloadV2{
		Schema: ports.SchemaRefV2{
			Namespace: "custom", Name: "command", Version: "1.0.0", MediaType: "application/octet-stream", ContentDigest: core.DigestBytes([]byte("schema-command")),
		},
		ContentDigest: core.DigestBytes(bytes),
		Length:        uint64(len(bytes)),
		Inline:        bytes,
		LimitPolicy: ports.OpaqueLimitPolicyRefV2{
			Policy: "custom/opaque-limit", Digest: core.DigestBytes([]byte("opaque-limit")),
		},
	}
}

type declaredOperationV3 struct {
	issued            control.IssueOperationPermitResultV3
	begun             control.OperationDispatchPermitFactV3
	delegation        ports.ExecutionDelegationFactV2
	declaredRef       ports.ExecutionDelegationRefV2
	delegationGateway control.ExecutionDelegationGovernanceGatewayV2
	dispatch          operationDispatchReaderV3
}

func beginAndDeclareOperationV3(t *testing.T, fixture *operationFixtureV3, suffix string) declaredOperationV3 {
	t.Helper()
	ctx := context.Background()
	issued, err := fixture.gateway.Issue(ctx, control.IssueGovernedOperationDispatchRequestV3{
		Operation: fixture.intent.Operation, EffectID: fixture.intent.ID, ExpectedEffectRevision: fixture.accepted.Revision,
		PermitID: "operation-permit-" + suffix, AttemptID: "provider-attempt-" + suffix, PermitTTL: 20 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	begun, err := fixture.gateway.Begin(ctx, control.BeginGovernedOperationDispatchRequestV3{
		Operation: fixture.intent.Operation, EffectID: fixture.intent.ID, ExpectedEffectRevision: issued.Effect.Revision,
		PermitID: issued.Permit.Permit.ID, ExpectedPermitRevision: issued.Permit.Revision,
	})
	if err != nil {
		t.Fatal(err)
	}
	host := providerBindingV3("custom/provider", "custom/relay")
	delegationID := "delegation-" + suffix
	preparedAttemptID, err := ports.DerivePreparedProviderAttemptIDV2(delegationID, begun.Permit.ID, begun.Permit.AttemptID)
	if err != nil {
		t.Fatal(err)
	}
	delegation := ports.ExecutionDelegationFactV2{
		ContractVersion: ports.ExecutionGovernanceContractVersionV2, ID: delegationID, Revision: 1, State: ports.ExecutionDelegationDeclaredV2,
		BindingSetID: fixture.intent.Provider.BindingSetID, BindingSetRevision: fixture.intent.Provider.BindingSetRevision,
		Operation: fixture.intent.Operation, HostAdapter: host, DataProvider: fixture.intent.Provider,
		RelayHops: []ports.ExecutionRelayHopV2{{Sequence: 1, Relay: host}}, EndpointID: "endpoint-" + suffix, RuntimeSessionRef: "runtime-session-" + suffix,
		PayloadSchema: fixture.intent.Payload.Schema, PayloadDigest: fixture.intent.Payload.ContentDigest, PayloadRevision: fixture.intent.PayloadRevision,
		IntentID: fixture.intent.ID, IntentRevision: fixture.intent.Revision, IntentDigest: begun.Permit.IntentDigest,
		ProviderPermitID: begun.Permit.ID, ProviderPermitRevision: begun.Permit.Revision, ProviderPermitDigest: begun.PermitDigest,
		ProviderAttemptID: begun.Permit.AttemptID, PreparedAttemptID: preparedAttemptID,
		OperationExpiresUnixNano: fixture.intent.ExpiresUnixNano, PermitExpiresUnixNano: begun.Permit.ExpiresUnixNano,
		HostBindingExpiresUnixNano: fixture.now.Add(30 * time.Second).UnixNano(), ProviderBindingExpiresUnixNano: fixture.now.Add(30 * time.Second).UnixNano(),
		CreatedUnixNano: fixture.now.UnixNano(), ExpiresUnixNano: fixture.now.Add(15 * time.Second).UnixNano(),
	}
	if fixture.intent.Operation.Kind == ports.OperationScopeRunV3 {
		delegation.EndpointID = "endpoint-run"
		delegation.RuntimeSessionRef, err = ports.DeriveRuntimeExecutionSessionRefV2(delegation.EndpointID, fixture.intent.Operation.RunID)
		if err != nil {
			t.Fatal(err)
		}
	}
	delegationGateway := control.ExecutionDelegationGovernanceGatewayV2{Effects: fixture.store, Delegations: fixture.delegations, Current: fixture.current, Clock: func() time.Time { return fixture.now }}
	declaredRef, err := delegationGateway.DeclareExecutionDelegationV2(ctx, ports.DeclareExecutionDelegationRequestV2{Delegation: delegation, Intent: fixture.intent, Permit: begun.Permit, Fence: begun.Fence})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := delegationGateway.InspectPreparedExecutionV2(ctx, fixture.intent.Operation, delegation.ID, begun.Permit.ID); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("exact declared delegation without preparation must report NotFound: %v", err)
	}
	if _, err := delegationGateway.InspectPreparedExecutionV2(ctx, fixture.intent.Operation, delegation.ID, "forged-permit"); !core.HasCategory(err, core.ErrorPreconditionFailed) {
		t.Fatalf("forged Permit must remain a precondition failure: %v", err)
	}
	return declaredOperationV3{issued: issued, begun: begun, delegation: delegation, declaredRef: declaredRef, delegationGateway: delegationGateway, dispatch: operationDispatchReaderV3{effects: fixture.store, delegations: fixture.delegations}}
}

func newGovernedProviderV2(fixture *operationFixtureV3, dispatch operationDispatchReaderV3) *governedProviderV2 {
	return &governedProviderV2{current: fixture.current, dispatch: dispatch, clock: func() time.Time { return fixture.now }, preparations: map[string]ports.ProviderPreparationAttestationV2{}, observations: map[string]ports.ProviderAttemptObservationV2{}}
}

func TestOperationGovernanceV3DeclaredPrepareCASExecutePrepared(t *testing.T) {
	fixture := newOperationFixtureV3(t)
	ctx := context.Background()
	issued, err := fixture.gateway.Issue(ctx, control.IssueGovernedOperationDispatchRequestV3{
		Operation: fixture.intent.Operation, EffectID: fixture.intent.ID, ExpectedEffectRevision: fixture.accepted.Revision,
		PermitID: "operation-permit-1", AttemptID: "provider-attempt-1", PermitTTL: 20 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	begun, err := fixture.gateway.Begin(ctx, control.BeginGovernedOperationDispatchRequestV3{
		Operation: fixture.intent.Operation, EffectID: fixture.intent.ID, ExpectedEffectRevision: issued.Effect.Revision,
		PermitID: issued.Permit.Permit.ID, ExpectedPermitRevision: issued.Permit.Revision,
	})
	if err != nil {
		t.Fatal(err)
	}
	host := providerBindingV3("custom/provider", "custom/relay")
	preparedAttemptID, err := ports.DerivePreparedProviderAttemptIDV2("delegation-1", begun.Permit.ID, begun.Permit.AttemptID)
	if err != nil {
		t.Fatal(err)
	}
	delegation := ports.ExecutionDelegationFactV2{
		ContractVersion: ports.ExecutionGovernanceContractVersionV2,
		ID:              "delegation-1", Revision: 1, State: ports.ExecutionDelegationDeclaredV2,
		BindingSetID: fixture.intent.Provider.BindingSetID, BindingSetRevision: fixture.intent.Provider.BindingSetRevision,
		Operation: fixture.intent.Operation, HostAdapter: host, DataProvider: fixture.intent.Provider,
		RelayHops:  []ports.ExecutionRelayHopV2{{Sequence: 1, Relay: host}},
		EndpointID: "endpoint-operation", RuntimeSessionRef: "runtime-session-operation",
		PayloadSchema: fixture.intent.Payload.Schema, PayloadDigest: fixture.intent.Payload.ContentDigest, PayloadRevision: fixture.intent.PayloadRevision,
		IntentID: fixture.intent.ID, IntentRevision: fixture.intent.Revision, IntentDigest: begun.Permit.IntentDigest,
		ProviderPermitID: begun.Permit.ID, ProviderPermitRevision: begun.Permit.Revision, ProviderPermitDigest: begun.PermitDigest, ProviderAttemptID: begun.Permit.AttemptID,
		PreparedAttemptID:        preparedAttemptID,
		OperationExpiresUnixNano: fixture.intent.ExpiresUnixNano, PermitExpiresUnixNano: begun.Permit.ExpiresUnixNano,
		HostBindingExpiresUnixNano: fixture.now.Add(30 * time.Second).UnixNano(), ProviderBindingExpiresUnixNano: fixture.now.Add(30 * time.Second).UnixNano(),
		CreatedUnixNano: fixture.now.UnixNano(), ExpiresUnixNano: fixture.now.Add(15 * time.Second).UnixNano(),
	}
	delegationGateway := control.ExecutionDelegationGovernanceGatewayV2{Effects: fixture.store, Delegations: fixture.delegations, Current: fixture.current, Clock: func() time.Time { return fixture.now }}
	declaredRef, err := delegationGateway.DeclareExecutionDelegationV2(ctx, ports.DeclareExecutionDelegationRequestV2{Delegation: delegation, Intent: fixture.intent, Permit: begun.Permit, Fence: begun.Fence})
	if err != nil {
		t.Fatal(err)
	}
	dispatch := operationDispatchReaderV3{effects: fixture.store, delegations: fixture.delegations}
	provider := &governedProviderV2{current: fixture.current, dispatch: dispatch, clock: func() time.Time { return fixture.now }, preparations: map[string]ports.ProviderPreparationAttestationV2{}, observations: map[string]ports.ProviderAttemptObservationV2{}}
	prepare := ports.PrepareGovernedExecutionRequestV2{
		Delegation: declaredRef, Intent: fixture.intent, Permit: begun.Permit, Fence: begun.Fence,
	}
	attestation, err := provider.Prepare(ctx, prepare)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := delegationGateway.CommitPreparedExecutionV2(ctx, ports.CommitPreparedExecutionRequestV2{Declared: declaredRef, Intent: fixture.intent, Permit: begun.Permit, Fence: begun.Fence, Preparation: attestation})
	if err != nil {
		t.Fatal(err)
	}
	inspectedPrepared, err := delegationGateway.InspectPreparedExecutionV2(ctx, fixture.intent.Operation, delegation.ID, begun.Permit.ID)
	if err != nil || inspectedPrepared != prepared {
		t.Fatalf("prepared execution was not recoverable after first commit: inspected=%#v prepared=%#v err=%v", inspectedPrepared, prepared, err)
	}
	executeRequest := ports.ExecutePreparedRequestV2{Delegation: prepared.Delegation, Prepared: prepared.Prepared, Enforcement: prepared.Enforcement, Intent: fixture.intent, Permit: begun.Permit, Fence: begun.Fence}
	observation, err := provider.ExecutePrepared(ctx, executeRequest)
	if err != nil {
		t.Fatal(err)
	}
	if observation.State != ports.ProviderAttemptObservedV2 || provider.prepareCalls != 1 {
		t.Fatalf("unexpected provider result: %#v calls=%d", observation, provider.prepareCalls)
	}
}

func TestOperationDispatchPermitFactV3UsesSandboxLeaseValueSemantics(t *testing.T) {
	scope := core.ExecutionScope{
		Identity:       core.AgentIdentityRef{TenantID: "tenant-permit-copy", ID: "identity-permit-copy", Epoch: 1},
		Lineage:        core.LineageRef{ID: "lineage-permit-copy", PlanDigest: core.DigestBytes([]byte("lineage-permit-copy"))},
		Instance:       core.InstanceRef{ID: "instance-permit-copy", Epoch: 1},
		SandboxLease:   &core.SandboxLeaseRef{ID: "lease-permit-copy", Epoch: 1},
		AuthorityEpoch: 1,
	}
	fixture := newOperationFixtureForRunV3(t, "", &scope)
	flow := beginAndDeclareOperationV3(t, fixture, "permit-copy")
	provider := newGovernedProviderV2(fixture, flow.dispatch)
	preparation, err := provider.Prepare(context.Background(), ports.PrepareGovernedExecutionRequestV2{
		Delegation: flow.declaredRef,
		Intent:     fixture.intent,
		Permit:     flow.begun.Permit,
		Fence:      flow.begun.Fence,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := flow.delegationGateway.CommitPreparedExecutionV2(context.Background(), ports.CommitPreparedExecutionRequestV2{
		Declared:    flow.declaredRef,
		Intent:      fixture.intent,
		Permit:      flow.begun.Permit,
		Fence:       flow.begun.Fence,
		Preparation: preparation,
	}); err != nil {
		t.Fatal(err)
	}
	fact, err := fixture.store.InspectOperationDispatchPermitV3(context.Background(), fixture.intent.Operation, flow.begun.Permit.ID)
	if err != nil || fact.Enforcement == nil {
		t.Fatalf("persisted enforcement missing: fact=%#v err=%v", fact, err)
	}

	deepCopy := fact
	receipt := *fact.Enforcement
	operation := receipt.Operation
	lease := *operation.ExecutionScope.SandboxLease
	operation.ExecutionScope.SandboxLease = &lease
	receipt.Operation = operation
	deepCopy.Enforcement = &receipt
	if fact.Enforcement.Operation.ExecutionScope.SandboxLease == deepCopy.Enforcement.Operation.ExecutionScope.SandboxLease {
		t.Fatal("test did not create distinct SandboxLease pointers")
	}
	if err := deepCopy.Validate(); err != nil {
		t.Fatalf("deep-copied equal SandboxLease value was rejected: %v", err)
	}

	for name, mutate := range map[string]func(*core.SandboxLeaseRef){
		"id":    func(value *core.SandboxLeaseRef) { value.ID = "lease-permit-forged" },
		"epoch": func(value *core.SandboxLeaseRef) { value.Epoch++ },
	} {
		t.Run(name, func(t *testing.T) {
			forged := deepCopy
			forgedReceipt := *deepCopy.Enforcement
			forgedOperation := forgedReceipt.Operation
			forgedLease := *forgedOperation.ExecutionScope.SandboxLease
			mutate(&forgedLease)
			forgedOperation.ExecutionScope.SandboxLease = &forgedLease
			forgedOperation.ExecutionScopeDigest, err = ports.ExecutionScopeDigestV2(forgedOperation.ExecutionScope)
			if err != nil {
				t.Fatal(err)
			}
			forgedReceipt.Operation = forgedOperation
			forged.Enforcement = &forgedReceipt
			if err := forged.Enforcement.Validate(); err != nil {
				t.Fatalf("forged Enforcement should be structurally valid before cross-binding check: %v", err)
			}
			if err := forged.Validate(); !core.HasReason(err, core.ReasonDispatchPermitInvalid) {
				t.Fatalf("SandboxLease %s drift did not fail exact Permit binding: %v", name, err)
			}
		})
	}
}

func TestOperationGovernanceV3ReviewDriftFailsAtBeginAndProviderPrepare(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		change func(*ports.OperationGovernanceSnapshotV3, time.Time)
	}{
		{name: "verdict revoked or replaced", change: func(snapshot *ports.OperationGovernanceSnapshotV3, _ time.Time) {
			snapshot.Review.Verdict.Digest = core.DigestBytes([]byte("revoked-verdict"))
		}},
		{name: "review exact expiry", change: func(snapshot *ports.OperationGovernanceSnapshotV3, now time.Time) {
			snapshot.Review.ExpiresUnixNano = now.UnixNano()
		}},
		{name: "candidate drift", change: func(snapshot *ports.OperationGovernanceSnapshotV3, _ time.Time) {
			snapshot.Review.CandidateDigest = core.DigestBytes([]byte("other-candidate"))
		}},
	} {
		t.Run(testCase.name+" at begin", func(t *testing.T) {
			fixture := newOperationFixtureV3(t)
			issued, err := fixture.gateway.Issue(context.Background(), control.IssueGovernedOperationDispatchRequestV3{Operation: fixture.intent.Operation, EffectID: fixture.intent.ID, ExpectedEffectRevision: fixture.accepted.Revision, PermitID: "permit-drift", AttemptID: "attempt-drift", PermitTTL: 10 * time.Second})
			if err != nil {
				t.Fatal(err)
			}
			fixture.current.mutate(func(snapshot *ports.OperationGovernanceSnapshotV3) { testCase.change(snapshot, fixture.now) })
			if _, err := fixture.gateway.Begin(context.Background(), control.BeginGovernedOperationDispatchRequestV3{Operation: fixture.intent.Operation, EffectID: fixture.intent.ID, ExpectedEffectRevision: issued.Effect.Revision, PermitID: issued.Permit.Permit.ID, ExpectedPermitRevision: issued.Permit.Revision}); err == nil {
				t.Fatal("Begin accepted stale Review authorization")
			}
		})
	}
}

func TestOperationGovernanceV3ProviderPrepareRereadsEveryCurrentFact(t *testing.T) {
	cases := []struct {
		name   string
		change func(*ports.OperationGovernanceSnapshotV3, time.Time)
	}{
		{name: "verdict", change: func(s *ports.OperationGovernanceSnapshotV3, _ time.Time) {
			s.Review.Verdict.Digest = core.DigestBytes([]byte("other-verdict"))
		}},
		{name: "review expiry", change: func(s *ports.OperationGovernanceSnapshotV3, now time.Time) { s.Review.ExpiresUnixNano = now.UnixNano() }},
		{name: "candidate", change: func(s *ports.OperationGovernanceSnapshotV3, _ time.Time) {
			s.Review.CandidateDigest = core.DigestBytes([]byte("other-candidate"))
		}},
		{name: "satisfaction", change: func(s *ports.OperationGovernanceSnapshotV3, _ time.Time) {
			s.Review.Satisfaction = &ports.OperationGovernanceFactRefV3{Ref: "new-satisfaction", Revision: 1, Digest: core.DigestBytes([]byte("satisfaction")), ExpiresUnixNano: s.ExpiresUnixNano}
		}},
		{name: "reviewer authority", change: func(s *ports.OperationGovernanceSnapshotV3, _ time.Time) {
			s.Review.ReviewerAuthority.Digest = core.DigestBytes([]byte("other-reviewer"))
		}},
		{name: "binding", change: func(s *ports.OperationGovernanceSnapshotV3, _ time.Time) { s.Binding.Revision++ }},
		{name: "authority", change: func(s *ports.OperationGovernanceSnapshotV3, _ time.Time) {
			s.Authority.Digest = core.DigestBytes([]byte("other-authority"))
		}},
		{name: "budget", change: func(s *ports.OperationGovernanceSnapshotV3, _ time.Time) {
			s.Budget.Digest = core.DigestBytes([]byte("other-budget"))
		}},
		{name: "policy", change: func(s *ports.OperationGovernanceSnapshotV3, _ time.Time) {
			s.Policy.Digest = core.DigestBytes([]byte("other-policy"))
		}},
		{name: "credential", change: func(s *ports.OperationGovernanceSnapshotV3, _ time.Time) {
			s.Credentials = append(s.Credentials, ports.OperationCredentialCurrentFactV3{})
		}},
		{name: "current scope", change: func(s *ports.OperationGovernanceSnapshotV3, _ time.Time) {
			s.CurrentScope.Digest = core.DigestBytes([]byte("other-scope"))
		}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newOperationFixtureV3(t)
			flow := beginAndDeclareOperationV3(t, fixture, "prepare-drift")
			provider := newGovernedProviderV2(fixture, flow.dispatch)
			fixture.current.mutate(func(snapshot *ports.OperationGovernanceSnapshotV3) { testCase.change(snapshot, fixture.now) })
			_, err := provider.Prepare(context.Background(), ports.PrepareGovernedExecutionRequestV2{Delegation: flow.declaredRef, Intent: fixture.intent, Permit: flow.begun.Permit, Fence: flow.begun.Fence})
			if err == nil {
				t.Fatal("Provider Prepare accepted stale governance")
			}
			if provider.prepareCalls != 0 {
				t.Fatalf("Provider was touched before fail-closed validation: %d", provider.prepareCalls)
			}
		})
	}
}

func TestOperationGovernanceV3LostPrepareAndExecuteRepliesRecoverOnlyByInspect(t *testing.T) {
	fixture := newOperationFixtureV3(t)
	flow := beginAndDeclareOperationV3(t, fixture, "lost-reply")
	provider := newGovernedProviderV2(fixture, flow.dispatch)
	provider.loseNextPrepareReply = true
	prepare := ports.PrepareGovernedExecutionRequestV2{Delegation: flow.declaredRef, Intent: fixture.intent, Permit: flow.begun.Permit, Fence: flow.begun.Fence}
	if _, err := provider.Prepare(context.Background(), prepare); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("expected lost Prepare reply: %v", err)
	}
	preparedID, _ := ports.DerivePreparedProviderAttemptIDV2(flow.delegation.ID, flow.begun.Permit.ID, flow.begun.Permit.AttemptID)
	attestation, err := provider.InspectPrepared(context.Background(), ports.InspectPreparedProviderRequestV2{DeclaredDelegation: flow.declaredRef, PreparedAttemptID: preparedID, PermitID: flow.begun.Permit.ID, AttemptID: flow.begun.Permit.AttemptID})
	if err != nil {
		t.Fatal(err)
	}
	if provider.prepareCalls != 1 || provider.inspectCalls != 1 {
		t.Fatalf("Prepare recovery re-executed: prepare=%d inspect=%d", provider.prepareCalls, provider.inspectCalls)
	}
	prepared, err := flow.delegationGateway.CommitPreparedExecutionV2(context.Background(), ports.CommitPreparedExecutionRequestV2{Declared: flow.declaredRef, Intent: fixture.intent, Permit: flow.begun.Permit, Fence: flow.begun.Fence, Preparation: attestation})
	if err != nil {
		t.Fatal(err)
	}
	provider.loseNextExecuteReply = true
	execute := ports.ExecutePreparedRequestV2{Delegation: prepared.Delegation, Prepared: prepared.Prepared, Enforcement: prepared.Enforcement, Intent: fixture.intent, Permit: flow.begun.Permit, Fence: flow.begun.Fence}
	if _, err := provider.ExecutePrepared(context.Background(), execute); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("expected lost Execute reply: %v", err)
	}
	observation, err := provider.InspectLocalAttempt(context.Background(), ports.InspectLocalProviderAttemptRequestV2{Delegation: prepared.Delegation, Prepared: prepared.Prepared})
	if err != nil || observation.State != ports.ProviderAttemptObservedV2 {
		t.Fatalf("local Inspect did not recover execution: %#v %v", observation, err)
	}
	if provider.executeCalls != 1 || provider.inspectCalls < 2 {
		t.Fatalf("Execute recovery was not inspect-only: execute=%d inspect=%d", provider.executeCalls, provider.inspectCalls)
	}
}

func TestGovernedExecutionProviderV2IsolatedFixtureLogicalIdempotencyProfile(t *testing.T) {
	fixture := newOperationFixtureV3(t)
	flow := beginAndDeclareOperationV3(t, fixture, "isolated-idempotency")
	provider := newGovernedProviderV2(fixture, flow.dispatch)
	prepare := ports.PrepareGovernedExecutionRequestV2{Delegation: flow.declaredRef, Intent: fixture.intent, Permit: flow.begun.Permit, Fence: flow.begun.Fence}
	attestation, err := provider.Prepare(context.Background(), prepare)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := flow.delegationGateway.CommitPreparedExecutionV2(context.Background(), ports.CommitPreparedExecutionRequestV2{Declared: flow.declaredRef, Intent: fixture.intent, Permit: flow.begun.Permit, Fence: flow.begun.Fence, Preparation: attestation})
	if err != nil {
		t.Fatal(err)
	}
	execute := ports.ExecutePreparedRequestV2{Delegation: prepared.Delegation, Prepared: prepared.Prepared, Enforcement: prepared.Enforcement, Intent: fixture.intent, Permit: flow.begun.Permit, Fence: flow.begun.Fence}
	report, err := conformance.CheckIsolatedGovernedExecutionIdempotencyV2(context.Background(), conformance.IsolatedGovernedExecutionIdempotencyCaseV2{
		Provider: provider, Execute: execute, Concurrency: 64, IsolatedFixtureOnly: true,
		ObservedLogicalEffects: func() uint64 {
			logical, _ := provider.isolatedExecutionCounts()
			return logical
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	logical, entries := provider.isolatedExecutionCounts()
	if report.MethodEntries != 64 || entries != 64 || report.DistinctAttemptObservations != 1 || report.ObservedLogicalEffects != 1 || logical != 1 || report.ProductionClaimEligible {
		t.Fatalf("isolated fixture profile overstated or failed logical idempotency: report=%#v entries=%d logical=%d", report, entries, logical)
	}
	if _, err := conformance.CheckIsolatedGovernedExecutionIdempotencyV2(context.Background(), conformance.IsolatedGovernedExecutionIdempotencyCaseV2{Provider: provider, Execute: execute, Concurrency: 64}); err == nil {
		t.Fatal("destructive idempotency profile ran without explicit isolated-fixture opt-in")
	}
}

func TestOperationGovernanceV3ExecuteRereadsCurrentFacts(t *testing.T) {
	cases := []struct {
		name   string
		change func(*ports.OperationGovernanceSnapshotV3)
	}{
		{name: "review", change: func(s *ports.OperationGovernanceSnapshotV3) {
			s.Review.Verdict.Digest = core.DigestBytes([]byte("revoked"))
		}},
		{name: "authority", change: func(s *ports.OperationGovernanceSnapshotV3) { s.Authority.Digest = core.DigestBytes([]byte("revoked")) }},
		{name: "credential", change: func(s *ports.OperationGovernanceSnapshotV3) {
			s.Credentials = append(s.Credentials, ports.OperationCredentialCurrentFactV3{})
		}},
		{name: "binding", change: func(s *ports.OperationGovernanceSnapshotV3) { s.Binding.Revision++ }},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newOperationFixtureV3(t)
			flow := beginAndDeclareOperationV3(t, fixture, "execute-drift")
			provider := newGovernedProviderV2(fixture, flow.dispatch)
			prepare := ports.PrepareGovernedExecutionRequestV2{Delegation: flow.declaredRef, Intent: fixture.intent, Permit: flow.begun.Permit, Fence: flow.begun.Fence}
			attestation, err := provider.Prepare(context.Background(), prepare)
			if err != nil {
				t.Fatal(err)
			}
			prepared, err := flow.delegationGateway.CommitPreparedExecutionV2(context.Background(), ports.CommitPreparedExecutionRequestV2{Declared: flow.declaredRef, Intent: fixture.intent, Permit: flow.begun.Permit, Fence: flow.begun.Fence, Preparation: attestation})
			if err != nil {
				t.Fatal(err)
			}
			fixture.current.mutate(func(snapshot *ports.OperationGovernanceSnapshotV3) { testCase.change(snapshot) })
			_, err = provider.ExecutePrepared(context.Background(), ports.ExecutePreparedRequestV2{Delegation: prepared.Delegation, Prepared: prepared.Prepared, Enforcement: prepared.Enforcement, Intent: fixture.intent, Permit: flow.begun.Permit, Fence: flow.begun.Fence})
			if err == nil {
				t.Fatal("ExecutePrepared accepted stale governance")
			}
			if provider.executeCalls != 0 {
				t.Fatalf("Provider executed after governance drift: %d", provider.executeCalls)
			}
		})
	}
}

func TestOperationSubjectV3CustomAndDelegationLoopRules(t *testing.T) {
	fixture := newOperationFixtureV3(t)
	custom := fixture.intent.Operation
	custom.Kind = "custom/index-maintenance"
	custom.ActivationAttemptID = ""
	custom.CustomOperationID = "custom-operation-1"
	if err := custom.Validate(); err != nil {
		t.Fatalf("namespaced custom operation rejected: %v", err)
	}
	custom.Kind = "not-namespaced"
	if err := custom.Validate(); err == nil {
		t.Fatal("non-namespaced custom operation accepted")
	}

	host := providerBindingV3("custom/provider", "custom/relay")
	provider := providerBindingV3("custom/provider", "custom/execute")
	base := ports.ExecutionDelegationFactV2{
		ContractVersion: ports.ExecutionGovernanceContractVersionV2, ID: "delegation-loop", Revision: 1, State: ports.ExecutionDelegationDeclaredV2,
		BindingSetID: provider.BindingSetID, BindingSetRevision: provider.BindingSetRevision, Operation: fixture.intent.Operation,
		HostAdapter: host, DataProvider: provider, RelayHops: []ports.ExecutionRelayHopV2{{Sequence: 1, Relay: host}},
		EndpointID: "endpoint", RuntimeSessionRef: "session", PayloadSchema: fixture.intent.Payload.Schema, PayloadDigest: fixture.intent.Payload.ContentDigest, PayloadRevision: 1,
		IntentID: fixture.intent.ID, IntentRevision: 1, IntentDigest: core.DigestBytes([]byte("intent")), ProviderPermitID: "permit", ProviderPermitRevision: 1, ProviderPermitDigest: core.DigestBytes([]byte("permit")), ProviderAttemptID: "attempt",
		OperationExpiresUnixNano: fixture.now.Add(time.Minute).UnixNano(), PermitExpiresUnixNano: fixture.now.Add(time.Minute).UnixNano(), HostBindingExpiresUnixNano: fixture.now.Add(time.Minute).UnixNano(), ProviderBindingExpiresUnixNano: fixture.now.Add(time.Minute).UnixNano(), CreatedUnixNano: fixture.now.UnixNano(), ExpiresUnixNano: fixture.now.Add(10 * time.Second).UnixNano(),
	}
	base.PreparedAttemptID, _ = ports.DerivePreparedProviderAttemptIDV2(base.ID, base.ProviderPermitID, base.ProviderAttemptID)
	if err := base.Validate(); err != nil {
		t.Fatalf("same component with distinct capabilities rejected: %v", err)
	}
	exact := base
	exact.DataProvider = host
	if err := exact.Validate(); err == nil {
		t.Fatal("exact same host/provider binding accepted")
	}
	loop := base
	loop.RelayHops = append(loop.RelayHops, ports.ExecutionRelayHopV2{Sequence: 2, Relay: host})
	if !core.HasReason(loop.Validate(), core.ReasonDependencyCycle) {
		t.Fatalf("duplicate relay was not rejected as a loop: %v", loop.Validate())
	}
}

func TestExecutionGovernanceV2ConformanceNeverSelfGrantsProduction(t *testing.T) {
	fixture := newOperationFixtureV3(t)
	// The backend testkit proves create-once visibility only and keeps every
	// authority/production claim false.
	host := providerBindingV3("custom/provider", "custom/relay")
	delegation := ports.ExecutionDelegationFactV2{
		ContractVersion: ports.ExecutionGovernanceContractVersionV2, ID: "delegation-conformance", Revision: 1, State: ports.ExecutionDelegationDeclaredV2,
		BindingSetID: fixture.intent.Provider.BindingSetID, BindingSetRevision: 1, Operation: fixture.intent.Operation,
		HostAdapter: host, DataProvider: fixture.intent.Provider, RelayHops: []ports.ExecutionRelayHopV2{{Sequence: 1, Relay: host}}, EndpointID: "endpoint-conformance", RuntimeSessionRef: "session-conformance",
		PayloadSchema: fixture.intent.Payload.Schema, PayloadDigest: fixture.intent.Payload.ContentDigest, PayloadRevision: 1,
		IntentID: fixture.intent.ID, IntentRevision: 1, IntentDigest: core.DigestBytes([]byte("intent-conformance")), ProviderPermitID: "permit-conformance", ProviderPermitRevision: 1, ProviderPermitDigest: core.DigestBytes([]byte("permit-conformance")), ProviderAttemptID: "attempt-conformance",
		OperationExpiresUnixNano: fixture.now.Add(time.Minute).UnixNano(), PermitExpiresUnixNano: fixture.now.Add(time.Minute).UnixNano(), HostBindingExpiresUnixNano: fixture.now.Add(time.Minute).UnixNano(), ProviderBindingExpiresUnixNano: fixture.now.Add(time.Minute).UnixNano(), CreatedUnixNano: fixture.now.UnixNano(), ExpiresUnixNano: fixture.now.Add(10 * time.Second).UnixNano(),
	}
	delegation.PreparedAttemptID, _ = ports.DerivePreparedProviderAttemptIDV2(delegation.ID, delegation.ProviderPermitID, delegation.ProviderAttemptID)
	report, err := conformance.CheckExecutionDelegationBackendV2(context.Background(), conformance.ExecutionDelegationBackendCaseV2{Facts: fixture.delegations, Delegation: delegation})
	if err != nil {
		t.Fatal(err)
	}
	if report.DispatchAuthorityClaim || report.ProductionDurability || report.SLAClaim {
		t.Fatalf("test fake self-granted production authority: %#v", report)
	}
}

func TestOperationAdmissionAndDelegationPublicInspectRecoverExactWatermarks(t *testing.T) {
	fixture := newOperationFixtureV3(t)
	ctx := context.Background()
	admission := control.OperationEffectAdmissionGatewayV3{Effects: fixture.store}
	receipt, err := admission.InspectAcceptedOperationEffectV3(ctx, fixture.intent.Operation, fixture.intent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := receipt.Validate(); err != nil || receipt.FactRevision != fixture.accepted.Revision {
		t.Fatalf("accepted Effect inspection did not recover exact receipt: %#v err=%v", receipt, err)
	}
	if _, err := admission.InspectAcceptedOperationEffectV3(ctx, fixture.intent.Operation, "different-effect"); err == nil {
		t.Fatal("accepted Effect inspection leaked another identity")
	}

	declared := beginAndDeclareOperationV3(t, fixture, "public-inspect")
	ref, err := declared.delegationGateway.InspectDeclaredExecutionV2(ctx, fixture.intent.Operation, declared.declaredRef.ID)
	if err != nil || ref != declared.declaredRef {
		t.Fatalf("declared delegation inspection did not recover exact ref: %#v err=%v", ref, err)
	}
	other := fixture.intent.Operation
	other.SubjectRevision++
	other.CurrentProjectionRevision++
	if _, err := declared.delegationGateway.InspectDeclaredExecutionV2(ctx, other, declared.declaredRef.ID); err == nil {
		t.Fatal("declared delegation inspection accepted another operation")
	}
}

func TestRunStartGovernanceV3PendingToRunningRequiresExactSettledAttempt(t *testing.T) {
	ctx := context.Background()
	scope := runScope(t)
	runID := core.AgentRunID("run-governed-start")
	fixture := newOperationFixtureForRunV3(t, runID, &scope)
	declared := beginAndDeclareOperationV3(t, fixture, "start")
	provider := newGovernedProviderV2(fixture, declared.dispatch)
	prepareRequest := ports.PrepareGovernedExecutionRequestV2{Delegation: declared.declaredRef, Intent: fixture.intent, Permit: declared.begun.Permit, Fence: declared.begun.Fence}
	preparation, err := provider.Prepare(ctx, prepareRequest)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := declared.delegationGateway.CommitPreparedExecutionV2(ctx, ports.CommitPreparedExecutionRequestV2{Declared: declared.declaredRef, Intent: fixture.intent, Permit: declared.begun.Permit, Fence: declared.begun.Fence, Preparation: preparation})
	if err != nil {
		t.Fatal(err)
	}
	observation, err := provider.ExecutePrepared(ctx, ports.ExecutePreparedRequestV2{Delegation: prepared.Delegation, Prepared: prepared.Prepared, Enforcement: prepared.Enforcement, Intent: fixture.intent, Permit: declared.begun.Permit, Fence: declared.begun.Fence})
	if err != nil {
		t.Fatal(err)
	}
	observationRecord := operationEvidenceRecordV3(t, fixture, observation, prepared.Delegation.ID, 1, ports.EvidenceTrustObservation, "custom/provider-observation")
	observation.Evidence = observationRecord.Ref
	evidence := operationEvidenceReaderV3{
		bySource: map[ports.EvidenceSourceKeyV2]ports.EvidenceLedgerRecordV2{{RegistrationID: observation.SourceRegistrationID, SourceEpoch: observation.SourceEpoch, SourceSequence: observation.SourceSequence}: observationRecord},
		byRef:    map[ports.EvidenceRecordRefV2]ports.EvidenceLedgerRecordV2{observationRecord.Ref: observationRecord},
	}
	operationDigest, _ := fixture.intent.Operation.DigestV3()
	intentDigest, _ := fixture.intent.DigestV3()
	attempt := ports.GovernedExecutionAttemptRefsV2{
		Admission: ports.OperationEffectAdmissionReceiptV3{OperationDigest: operationDigest, EffectID: fixture.intent.ID, IntentRevision: fixture.intent.Revision, IntentDigest: intentDigest, FactRevision: fixture.accepted.Revision, State: "accepted"},
		PermitID:  declared.begun.Permit.ID, PermitRevision: declared.begun.Permit.Revision, PermitDigest: declared.begun.PermitDigest, AttemptID: declared.begun.Permit.AttemptID,
		Delegation: prepared.Delegation, Prepared: prepared.Prepared, Enforcement: prepared.Enforcement,
	}
	observationGateway := control.OperationObservationGovernanceGatewayV3{Effects: fixture.store, Observations: fakes.NewProviderAttemptObservationStoreV2(), Delegations: fixture.delegations, Current: fixture.current, Dispatch: declared.dispatch, Evidence: evidence, Clock: func() time.Time { return fixture.now }}
	observationRef, err := observationGateway.RecordGovernedProviderObservationV3(ctx, ports.RecordGovernedProviderObservationRequestV2{Intent: fixture.intent, Permit: declared.begun.Permit, Fence: declared.begun.Fence, Attempt: attempt, Observation: observation})
	if err != nil {
		t.Fatal(err)
	}
	attempt.Observation = &observationRef
	settlementRecord := operationEvidenceRecordV3(t, fixture, observation, observation.ProviderOperationRef, 2, ports.EvidenceTrustAttestation, "custom/provider-settlement")
	evidence.byRef[settlementRecord.Ref] = settlementRecord
	settlementOwner, _ := exactOwnerForTestV3(fixture.intent.Owners, ports.OwnerSettlement)
	dispatchAttempt := ports.OperationDispatchAttemptRefV3{OperationDigest: operationDigest, EffectID: fixture.intent.ID, IntentRevision: fixture.intent.Revision, IntentDigest: intentDigest, PermitID: declared.begun.Permit.ID, PermitRevision: declared.begun.Permit.Revision, PermitDigest: declared.begun.PermitDigest, AttemptID: declared.begun.Permit.AttemptID, Delegation: &prepared.Delegation}
	settlementGateway := control.OperationSettlementGovernanceGatewayV3{Effects: fixture.store, Evidence: evidence, Clock: func() time.Time { return fixture.now }}
	settlementSubmission := ports.OperationSettlementSubmissionV3{ID: "settlement-run-start", Revision: 1, Attempt: dispatchAttempt, Owner: settlementOwner, Disposition: ports.OperationSettlementAppliedV3, Observation: &observationRef, Evidence: []ports.EvidenceRecordRefV2{settlementRecord.Ref}, SettledUnixNano: fixture.now.UnixNano()}
	fixture.store.LoseNextCASReply()
	settlementRef, err := settlementGateway.SettleOperationEffectV3(ctx, fixture.intent, settlementSubmission)
	if err != nil {
		t.Fatal(err)
	}
	replayedSettlement, err := settlementGateway.SettleOperationEffectV3(ctx, fixture.intent, settlementSubmission)
	if err != nil || !sameSettlementRefForTestV3(settlementRef, replayedSettlement) {
		t.Fatalf("settlement lost-reply replay was not exact: ref=%#v err=%v", replayedSettlement, err)
	}
	changedSettlement := settlementSubmission
	changedSettlement.Disposition = ports.OperationSettlementFailedV3
	if _, err := settlementGateway.SettleOperationEffectV3(ctx, fixture.intent, changedSettlement); !core.HasReason(err, core.ReasonIdempotencyPayloadMismatch) {
		t.Fatalf("same settlement ID with changed content did not conflict: %v", err)
	}
	attempt.Settlement = &settlementRef
	inspectionAttempt := dispatchAttempt
	inspectionAttempt.EffectID = "operation-effect-inspect-start"
	inspectionAttempt.IntentDigest = core.DigestBytes([]byte("inspect-intent"))
	inspectionAttempt.PermitID = "inspect-permit-start"
	inspectionAttempt.PermitDigest = core.DigestBytes([]byte("inspect-permit"))
	inspectionAttempt.AttemptID = "inspect-attempt-start"
	inspectionSettlement, err := settlementRef.InspectionRefV3()
	if err != nil {
		t.Fatal(err)
	}
	inspectionSettlement.Attempt = inspectionAttempt
	postPreparedUnknown := settlementRef
	postPreparedUnknown.Observation = nil
	postPreparedUnknown.InspectionEffect = &inspectionAttempt
	postPreparedUnknown.InspectionSettlement = &inspectionSettlement
	unknownAttempt := attempt
	unknownAttempt.Observation = nil
	unknownAttempt.Settlement = &postPreparedUnknown
	if err := unknownAttempt.ValidatePrepared(); err != nil {
		t.Fatalf("post-prepared unknown settlement with exact Inspect provenance was rejected: %v", err)
	}
	missingInspect := postPreparedUnknown
	missingInspect.InspectionEffect = nil
	unknownAttempt.Settlement = &missingInspect
	if err := unknownAttempt.ValidatePrepared(); !core.HasReason(err, core.ReasonEffectUnknownOutcome) {
		t.Fatalf("post-prepared unknown settlement without complete Inspect provenance was accepted: %v", err)
	}

	pending := runningRecord(scope, runID, fixture.now)
	pending.Status = core.RunPending
	pending.Revision = 1
	pending.StartedAt = time.Time{}
	plan := runSettlementPlanFixtureV2(t, pending, fixture.now)
	plan.Execution.Binding = ports.EvidenceProducerBindingRefV2(fixture.intent.Provider)
	plan.Execution.SubjectDigest, _ = plan.Execution.DigestV2()
	for index := range plan.Requirements {
		if plan.Requirements[index].Kind == ports.RunRequirementExecutionTruth {
			plan.Requirements[index].Owner = plan.Execution.Binding
			plan.Requirements[index].SubjectDigest = plan.Execution.SubjectDigest
		}
	}
	runs := fakes.NewRunSettlementStoreV2(func() time.Time { return fixture.now })
	planAdmission := staticPlanCertificationV3(t, pending, plan, fixture.now)
	certificationRef, err := planAdmission.fact.RefV3()
	if err != nil {
		t.Fatal(err)
	}
	certification, err := ports.NewRunSettlementPlanCertificationAssociationV3(pending, plan, certificationRef)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runs.CreateRunBundleV3(ctx, control.RunBundleCreateRequestV3{Run: pending, Plan: plan, Certification: certification}); err != nil {
		t.Fatal(err)
	}
	runStart := control.RunStartGovernanceGatewayV3{Runs: runs, Effects: fixture.store, Delegations: fixture.delegations, PlanAdmissions: planAdmission, Clock: func() time.Time { return fixture.now }}
	request := ports.ConfirmRunStartedRequestV3{ExecutionScope: scope, RunID: runID, ExpectedRunRevision: pending.Revision, Operation: fixture.intent.Operation, Attempt: attempt}
	bareRuns := fakes.NewRunSettlementStoreV2(func() time.Time { return fixture.now })
	if _, err := bareRuns.CreateRunBundleV2(ctx, control.RunBundleCreateRequestV2{Run: pending, Plan: plan}); err != nil {
		t.Fatal(err)
	}
	bareStart := control.RunStartGovernanceGatewayV3{Runs: bareRuns, Effects: fixture.store, Delegations: fixture.delegations, PlanAdmissions: planAdmission, Clock: func() time.Time { return fixture.now }}
	if _, err := bareStart.ConfirmRunStartedV3(ctx, request); err == nil {
		t.Fatal("legacy uncertified Run bundle was promoted to running")
	}
	bareRun, err := bareRuns.InspectRun(ctx, scope, runID)
	if err != nil || bareRun.Status != core.RunPending || bareRun.Revision != 1 {
		t.Fatalf("failed certified start preflight mutated the Run: %#v err=%v", bareRun, err)
	}
	if _, err := bareRuns.InspectRunStartConfirmationV3(ctx, scope, runID); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("failed certified start preflight created a proof: %v", err)
	}
	confirmed, err := runStart.ConfirmRunStartedV3(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	running := confirmed.Run
	if running.Status != core.RunRunning || running.Revision != 2 || running.StartedAt.UnixNano() != observationRef.ObservedUnixNano {
		t.Fatalf("unexpected confirmed Run: %#v", running)
	}
	if err := confirmed.Validate(); err != nil || confirmed.Confirmation.Attempt.Settlement == nil || confirmed.Confirmation.Attempt.Settlement.Digest != settlementRef.Digest {
		t.Fatalf("run start confirmation does not preserve the exact attempt: %#v err=%v", confirmed, err)
	}
	replayed, err := runStart.ConfirmRunStartedV3(ctx, request)
	if err != nil || !sameRunStartForTestV3(running, replayed.Run) || replayed.Confirmation.Digest != confirmed.Confirmation.Digest {
		t.Fatalf("run start replay was not idempotent: run=%#v err=%v", replayed, err)
	}
	inspected, err := runStart.InspectRunStartV3(ctx, scope, runID)
	if err != nil || inspected.Confirmation.Digest != confirmed.Confirmation.Digest || !sameRunStartForTestV3(inspected.Run, running) {
		t.Fatalf("run start proof cannot be recovered independently: %#v err=%v", inspected, err)
	}
	forged := request
	forged.Attempt.Settlement = &ports.OperationSettlementRefV3{ID: settlementRef.ID, Revision: settlementRef.Revision, Digest: settlementRef.Digest, Attempt: settlementRef.Attempt, Disposition: ports.OperationSettlementFailedV3, Owner: settlementRef.Owner, Observation: settlementRef.Observation, Evidence: settlementRef.Evidence}
	if _, err := runStart.ConfirmRunStartedV3(ctx, forged); err == nil {
		t.Fatal("forged settlement disposition confirmed a Run start")
	}
	otherAttempt := request
	otherSettlement := *request.Attempt.Settlement
	otherSettlement.ID = "other-start-settlement"
	otherAttempt.Attempt.Settlement = &otherSettlement
	if _, err := runStart.ConfirmRunStartedV3(ctx, otherAttempt); err == nil {
		t.Fatal("same running Run and StartedAt accepted another start attempt")
	}
	forgedEnvelope := confirmed
	forgedEnvelope.Confirmation.Attempt = otherAttempt.Attempt
	if err := forgedEnvelope.Validate(); err == nil {
		t.Fatal("public start envelope accepted another attempt for the same Run and StartedAt")
	}
}

func operationEvidenceRecordV3(t *testing.T, fixture *operationFixtureV3, observation ports.ProviderAttemptObservationV2, causation string, sequence uint64, trust ports.EvidenceTrustClassV2, kind ports.NamespacedNameV2) ports.EvidenceLedgerRecordV2 {
	t.Helper()
	ledgerScope := ports.EvidenceLedgerScopeV2{Partition: ports.EvidencePartitionRun, TenantID: fixture.intent.Operation.ExecutionScope.Identity.TenantID, IdentityID: fixture.intent.Operation.ExecutionScope.Identity.ID, LineageID: fixture.intent.Operation.ExecutionScope.Lineage.ID, InstanceID: fixture.intent.Operation.ExecutionScope.Instance.ID, RunID: fixture.intent.Operation.RunID}
	ledgerDigest, _ := ledgerScope.DigestV2()
	payload := observation.Payload
	candidate := ports.EvidenceEventCandidateV2{
		ContractVersion: ports.EvidenceContractVersionV2, LedgerScope: ledgerScope, EventID: observation.ProviderOperationRef, RegistrationID: observation.SourceRegistrationID, RegistrationRevision: 1,
		SourceConfigurationDigest: core.DigestBytes([]byte("operation-source-config")), SourcePolicy: ports.EvidenceSourcePolicyBindingRefV2{Ref: "operation-source-policy", Revision: 1, Digest: core.DigestBytes([]byte("operation-source-policy"))},
		SourceID: "custom/provider", SourceEpoch: observation.SourceEpoch, SourceSequence: sequence, TrustClass: trust, EventKind: kind, CustomClass: "custom/provider-evidence", ExecutionScope: fixture.intent.Operation.ExecutionScope,
		Payload:   ports.EvidencePayloadRefV2{Schema: payload.Schema, ContentDigest: payload.ContentDigest, Revision: observation.PayloadRevision, Length: payload.Length, Ref: "memory://operation-evidence"},
		Causation: []ports.EvidenceCausationRefV2{{LedgerScopeDigest: ledgerDigest, EventID: causation}}, CorrelationID: observation.Prepared.ID, Producer: ports.EvidenceProducerBindingRefV2(fixture.intent.Provider),
		Authority: ports.AuthorityBindingRefV2{Ref: "operation-evidence-authority", Revision: 1, Digest: core.DigestBytes([]byte("operation-evidence-authority")), Epoch: fixture.intent.Operation.ExecutionScope.AuthorityEpoch}, ObservedUnixNano: observation.ObservedUnixNano,
	}
	previous := ports.EvidenceGenesisDigestV2
	if sequence > 1 {
		previous = core.DigestBytes([]byte("operation-previous"))
	}
	record, err := control.NewEvidenceLedgerRecordV2(candidate, sequence, previous, fixture.now)
	if err != nil {
		t.Fatal(err)
	}
	return record
}

func exactOwnerForTestV3(owners []ports.EffectOwnerRefV2, role ports.OwnerRoleV2) (ports.EffectOwnerRefV2, bool) {
	for _, owner := range owners {
		if owner.Role == role {
			return owner, true
		}
	}
	return ports.EffectOwnerRefV2{}, false
}

func sameRunStartForTestV3(left, right core.AgentRunRecord) bool {
	leftDigest, leftErr := ports.RunIdentityDigestV2(left)
	rightDigest, rightErr := ports.RunIdentityDigestV2(right)
	return leftErr == nil && rightErr == nil && leftDigest == rightDigest && left.Status == right.Status && left.Revision == right.Revision && left.StartedAt.Equal(right.StartedAt)
}

func sameSettlementRefForTestV3(left, right ports.OperationSettlementRefV3) bool {
	leftDigest, leftErr := core.CanonicalJSONDigest("praxis.runtime.operation-effect", ports.OperationEffectContractVersionV3, "OperationSettlementRefV3", left)
	rightDigest, rightErr := core.CanonicalJSONDigest("praxis.runtime.operation-effect", ports.OperationEffectContractVersionV3, "OperationSettlementRefV3", right)
	return leftErr == nil && rightErr == nil && leftDigest == rightDigest
}

func TestOperationGovernancePortV3UnknownRecoveryNeverRejectsOrReissues(t *testing.T) {
	fixture := newOperationFixtureV3(t)
	ctx := context.Background()
	issued, err := fixture.gateway.IssueOperationDispatchV3(ctx, ports.IssueGovernedOperationDispatchRequestV3{Operation: fixture.intent.Operation, EffectID: fixture.intent.ID, ExpectedEffectRevision: fixture.accepted.Revision, PermitID: "permit-public-unknown", AttemptID: "attempt-public-unknown", PermitTTL: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if issued.State != ports.OperationDispatchAuthorizationIssuedV3 || issued.Permit.ID != issued.Attempt.PermitID {
		t.Fatalf("unexpected public Issue envelope: %#v", issued)
	}
	begun, err := fixture.gateway.BeginOperationDispatchV3(ctx, ports.BeginGovernedOperationDispatchRequestV3{Operation: fixture.intent.Operation, EffectID: fixture.intent.ID, ExpectedEffectRevision: issued.EffectFactRevision, PermitID: issued.Permit.ID, ExpectedPermitRevision: issued.PermitFactRevision})
	if err != nil {
		t.Fatal(err)
	}
	fixture.store.LoseNextCASReply()
	unknown, err := fixture.gateway.MarkOperationDispatchUnknownV3(ctx, ports.MarkOperationDispatchUnknownRequestV3{Operation: fixture.intent.Operation, EffectID: fixture.intent.ID, ExpectedEffectRevision: begun.EffectFactRevision, Permit: begun.Attempt})
	if err != nil {
		t.Fatal(err)
	}
	if unknown.State != ports.OperationDispatchAuthorizationUnknownV3 || unknown.Permit.ID != begun.Permit.ID || unknown.Attempt != begun.Attempt {
		t.Fatalf("unknown transition changed the governed attempt: %#v", unknown)
	}
	replayed, err := fixture.gateway.MarkOperationDispatchUnknownV3(ctx, ports.MarkOperationDispatchUnknownRequestV3{Operation: fixture.intent.Operation, EffectID: fixture.intent.ID, ExpectedEffectRevision: begun.EffectFactRevision, Permit: begun.Attempt})
	if err != nil || replayed.State != ports.OperationDispatchAuthorizationUnknownV3 {
		t.Fatalf("unknown transition did not recover by Inspect: %#v err=%v", replayed, err)
	}
	stored, err := fixture.store.InspectOperationEffectV3(ctx, fixture.intent.Operation, fixture.intent.ID)
	if err != nil || stored.State != control.OperationEffectUnknownOutcomeV3 {
		t.Fatalf("unknown transition was overwritten or rejected: %#v err=%v", stored, err)
	}
}
