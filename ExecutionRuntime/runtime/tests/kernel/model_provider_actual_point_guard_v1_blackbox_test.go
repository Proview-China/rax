package kernel_test

import (
	"context"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/fakes"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestModelProviderActualPointGuardV1RealV4BegunAndExactVerifier(t *testing.T) {
	fixture := newActualPointFixtureV1(t)
	projection, err := fixture.guard.InspectCurrentModelProviderActualPointV1(context.Background(), fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	if err := projection.ValidateAgainst(fixture.request, fixture.now); err != nil {
		t.Fatal(err)
	}
	if projection.Provider != fixture.provider || projection.Verifier != fixture.verifier {
		t.Fatal("actual-point projection lost Provider or EnforcementPoint")
	}
	if fixture.effectStore.BeginV4CommitCount() != 1 {
		t.Fatalf("guard changed begun Permit: begin=%d", fixture.effectStore.BeginV4CommitCount())
	}

	wrong := fixture.request
	wrong.Verifier.ArtifactDigest = actualPointDigestV1("wrong-verifier")
	if _, err := fixture.guard.InspectCurrentModelProviderActualPointV1(context.Background(), wrong); err == nil {
		t.Fatal("wrong exact EnforcementPoint reached actual point")
	}
	if fixture.effectStore.BeginV4CommitCount() != 1 {
		t.Fatal("wrong verifier changed begun Permit")
	}
}

func TestModelProviderActualPointGuardV1BlackboxDriftMatrix(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*actualPointFixtureV1)
	}{
		{name: "run", mutate: func(f *actualPointFixtureV1) { f.run.value.Run.Revision++ }},
		{name: "control", mutate: func(f *actualPointFixtureV1) {
			value := f.runtimeControl.value
			value.State = control.ModelDispatchControlCancelRequestedV1
			f.runtimeControl.value, _ = control.SealModelDispatchControlCurrentProjectionV1(value)
		}},
		{name: "effect", mutate: func(f *actualPointFixtureV1) {
			f.effectReader.mutate = func(value control.OperationEffectFactV3) control.OperationEffectFactV3 {
				value.Intent.Kind = "praxis.harness/another-turn"
				return value
			}
		}},
		{name: "permit", mutate: func(f *actualPointFixtureV1) {
			f.dispatchReader.mutate = func(value ports.CurrentOperationDispatchAuthorizationV4) ports.CurrentOperationDispatchAuthorizationV4 {
				value.Record.State = ports.OperationPermitIssuedV4
				value.Record.BegunUnixNano = 0
				value.Record, _ = ports.SealOperationDispatchRecordV4(value.Record)
				return value
			}
		}},
		{name: "permit expired", mutate: func(f *actualPointFixtureV1) {
			f.dispatchReader.mutate = func(value ports.CurrentOperationDispatchAuthorizationV4) ports.CurrentOperationDispatchAuthorizationV4 {
				value.Record.State = ports.OperationPermitExpiredV4
				value.Record, _ = ports.SealOperationDispatchRecordV4(value.Record)
				return value
			}
		}},
		{name: "permit revoked", mutate: func(f *actualPointFixtureV1) {
			f.dispatchReader.mutate = func(value ports.CurrentOperationDispatchAuthorizationV4) ports.CurrentOperationDispatchAuthorizationV4 {
				value.Record.State = ports.OperationPermitRevokedV4
				value.Record, _ = ports.SealOperationDispatchRecordV4(value.Record)
				return value
			}
		}},
		{name: "authorization", mutate: func(f *actualPointFixtureV1) {
			f.request.ReviewAuthorization.Digest = actualPointDigestV1("wrong-review")
		}},
		{name: "review current", mutate: func(f *actualPointFixtureV1) {
			f.review.value.CurrentnessDigest = actualPointDigestV1("wrong-review-current")
			f.review.value, _ = ports.SealOperationReviewCurrentProjectionV4(f.review.value, f.now)
		}},
		{name: "authority current", mutate: func(f *actualPointFixtureV1) {
			f.governance.value.Authority.Digest = actualPointDigestV1("wrong-authority")
		}},
		{name: "binding current", mutate: func(f *actualPointFixtureV1) {
			f.governance.value.Binding.Digest = actualPointDigestV1("wrong-binding")
		}},
		{name: "credential current", mutate: func(f *actualPointFixtureV1) {
			f.governance.value.Credentials = append(f.governance.value.Credentials, ports.OperationCredentialCurrentFactV3{})
		}},
		{name: "admission", mutate: func(f *actualPointFixtureV1) {
			f.request.AdmissionDigest = actualPointDigestV1("wrong-admission")
		}},
		{name: "fence", mutate: func(f *actualPointFixtureV1) {
			f.request.FenceDigest = actualPointDigestV1("wrong-fence")
		}},
		{name: "lease", mutate: func(f *actualPointFixtureV1) {
			f.run.value.Run.Scope.SandboxLease = &core.SandboxLeaseRef{ID: "another-lease", Epoch: 1}
		}},
		{name: "epoch", mutate: func(f *actualPointFixtureV1) { f.run.value.Run.Scope.Instance.Epoch++ }},
		{name: "scope", mutate: func(f *actualPointFixtureV1) { f.run.value.Run.Scope.AuthorityEpoch++ }},
		{name: "provider", mutate: func(f *actualPointFixtureV1) {
			value := f.boundary.value
			value.Provider.ArtifactDigest = actualPointDigestV1("wrong-provider")
			f.boundary.value, _ = ports.SealModelProviderBoundaryCurrentProjectionV1(value)
		}},
		{name: "ttl", mutate: func(f *actualPointFixtureV1) {
			f.request.RequestedNotAfterUnixNano = f.now.UnixNano()
		}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newActualPointFixtureV1(t)
			testCase.mutate(fixture)
			if _, err := fixture.guard.InspectCurrentModelProviderActualPointV1(context.Background(), fixture.request); err == nil {
				t.Fatal("drifted current closure returned a projection")
			}
			if fixture.effectStore.BeginV4CommitCount() != 1 {
				t.Fatalf("failed guard changed begun Permit: begin=%d", fixture.effectStore.BeginV4CommitCount())
			}
		})
	}
}

func TestModelProviderActualPointGuardV1S2DriftTTLAndClockRollback(t *testing.T) {
	t.Run("S2 control drift", func(t *testing.T) {
		fixture := newActualPointFixtureV1(t)
		fixture.runtimeControl.afterFirst = func(value control.ModelDispatchControlCurrentProjectionV1) control.ModelDispatchControlCurrentProjectionV1 {
			value.DesiredStateRevision++
			value.WatermarkDigest = actualPointDigestV1("S2-control")
			value, _ = control.SealModelDispatchControlCurrentProjectionV1(value)
			return value
		}
		if _, err := fixture.guard.InspectCurrentModelProviderActualPointV1(context.Background(), fixture.request); !core.HasReason(err, core.ReasonBindingDrift) {
			t.Fatalf("S2 drift: %v", err)
		}
	})
	t.Run("TTL crossing", func(t *testing.T) {
		fixture := newActualPointFixtureV1(t)
		fixture.clocks = []time.Time{fixture.now, fixture.now, time.Unix(0, fixture.request.RequestedNotAfterUnixNano)}
		fixture.guard.Clock = fixture.clock
		if _, err := fixture.guard.InspectCurrentModelProviderActualPointV1(context.Background(), fixture.request); !core.HasReason(err, core.ReasonBindingExpired) {
			t.Fatalf("TTL crossing: %v", err)
		}
	})
	t.Run("clock rollback", func(t *testing.T) {
		fixture := newActualPointFixtureV1(t)
		fixture.clocks = []time.Time{fixture.now, fixture.now.Add(-time.Nanosecond)}
		fixture.guard.Clock = fixture.clock
		if _, err := fixture.guard.InspectCurrentModelProviderActualPointV1(context.Background(), fixture.request); !core.HasReason(err, core.ReasonClockRegression) {
			t.Fatalf("clock rollback: %v", err)
		}
	})
}

func TestModelProviderActualPointGuardV1DependenciesAreReadOnlyAndProviderFree(t *testing.T) {
	gatewayType := reflect.TypeOf(kernel.ModelProviderActualPointGuardGatewayV1{})
	expected := map[string][]string{
		"ModelBoundary": {"InspectCurrentModelProviderBoundaryV1"},
		"Runs":          {"InspectRunLifecycleV3"},
		"Control":       {"InspectModelDispatchControlCurrentV1"},
		"Effects":       {"InspectOperationEffectV3"},
		"Dispatch":      {"InspectCurrentOperationDispatchV4"},
	}
	if gatewayType.NumField() != len(expected)+1 {
		t.Fatalf("unexpected actual-point dependency count: %d", gatewayType.NumField())
	}
	for name, methods := range expected {
		field, ok := gatewayType.FieldByName(name)
		if !ok {
			t.Fatalf("missing read-only dependency %s", name)
		}
		if field.Type.Kind() != reflect.Interface || field.Type.NumMethod() != len(methods) {
			t.Fatalf("%s is not the frozen narrow reader", name)
		}
		for index, methodName := range methods {
			if field.Type.Method(index).Name != methodName {
				t.Fatalf("%s exposes %s, want %s", name, field.Type.Method(index).Name, methodName)
			}
		}
	}
	clock, ok := gatewayType.FieldByName("Clock")
	if !ok || clock.Type.Kind() != reflect.Func {
		t.Fatal("actual-point gateway lost its injected clock")
	}
}

type actualPointFixtureV1 struct {
	now            time.Time
	provider       ports.ProviderBindingRefV2
	verifier       ports.ProviderBindingRefV2
	request        ports.InspectCurrentModelProviderActualPointRequestV1
	guard          kernel.ModelProviderActualPointGuardGatewayV1
	boundary       *actualPointBoundaryReaderV1
	run            *actualPointRunReaderV1
	runtimeControl *actualPointControlReaderV1
	effectReader   *actualPointEffectReaderV1
	dispatchReader *actualPointDispatchReaderV1
	governance     *actualPointGovernanceReaderV1
	review         *actualPointReviewReaderV1
	effectStore    *fakes.OperationEffectStoreV3
	clockMu        sync.Mutex
	clocks         []time.Time
}

func (f *actualPointFixtureV1) clock() time.Time {
	f.clockMu.Lock()
	defer f.clockMu.Unlock()
	if len(f.clocks) == 0 {
		return f.now
	}
	value := f.clocks[0]
	f.clocks = f.clocks[1:]
	return value
}

func newActualPointFixtureV1(t *testing.T) *actualPointFixtureV1 {
	t.Helper()
	now := time.Unix(1_400_000, 0)
	lease := &core.SandboxLeaseRef{ID: "lease-actual-point", Epoch: 7}
	scope := core.ExecutionScope{
		Identity: core.AgentIdentityRef{TenantID: "tenant-actual-point", ID: "agent-actual-point", Epoch: 1},
		Lineage:  core.LineageRef{ID: "lineage-actual-point", PlanDigest: actualPointDigestV1("plan")},
		Instance: core.InstanceRef{ID: "instance-actual-point", Epoch: 5}, SandboxLease: lease, AuthorityEpoch: 3,
	}
	scopeDigest, _ := ports.ExecutionScopeDigestV2(scope)
	operation := ports.OperationSubjectV3{
		Kind: ports.OperationScopeRunV3, ExecutionScope: scope, ExecutionScopeDigest: scopeDigest, RunID: "run-actual-point",
		SubjectRevision: 1, CurrentProjectionRef: "run-current-actual-point", CurrentProjectionRevision: 1,
		CurrentProjectionDigest: actualPointDigestV1("run-current"),
	}
	operationDigest, _ := operation.DigestV3()
	provider := actualPointProviderV1("praxis.model/provider", ports.ModelInvokeCapabilityV1)
	intent := actualPointIntentV1(operation, provider, now)
	intentDigest, _ := intent.DigestV3()
	governance := actualPointGovernanceV1(intent, provider, now)
	current := &actualPointGovernanceReaderV1{value: governance}
	effectStore := fakes.NewOperationEffectStoreV3(func() time.Time { return now })
	proposed, err := control.NewProposedOperationEffectFactV3(intent, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := effectStore.CreateOperationEffectV3(context.Background(), proposed); err != nil {
		t.Fatal(err)
	}
	accepted := proposed
	accepted.State = control.OperationEffectAcceptedV3
	accepted.Revision++
	accepted.UpdatedUnixNano = now.UnixNano()
	accepted, err = effectStore.CompareAndSwapOperationEffectV3(context.Background(), operation, control.OperationEffectCASRequestV3{ExpectedRevision: proposed.Revision, Next: accepted})
	if err != nil {
		t.Fatal(err)
	}
	reviewProjection := actualPointReviewV1(intent, intentDigest, governance, now)
	reviewReader := &actualPointReviewReaderV1{value: reviewProjection}
	authorizationStore := fakes.NewOperationReviewAuthorizationStoreV4(func() time.Time { return now })
	effectStore.BindOperationReviewAuthorizationFactsV4(authorizationStore)
	reviewGateway := kernel.OperationReviewAuthorizationGatewayV4{
		Facts: authorizationStore, Effects: effectStore, Governance: current, Reviews: reviewReader, Clock: func() time.Time { return now },
	}
	authorization, err := reviewGateway.CreateOperationReviewAuthorizationV4(context.Background(), ports.CreateOperationReviewAuthorizationRequestV4{
		AuthorizationID: "authorization-actual-point", Operation: operation, EffectID: intent.ID,
		ExpectedEffectRevision: accepted.Revision, RequestedTTL: 20 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	admissions := control.OperationEffectAdmissionGatewayV3{Effects: effectStore}
	admission, err := admissions.InspectAcceptedOperationEffectV3(context.Background(), operation, intent.ID)
	if err != nil {
		t.Fatal(err)
	}
	dispatchGateway := control.OperationGovernanceGatewayV4{
		Effects: effectStore, Admissions: admissions, Reviews: reviewGateway, Current: current, Clock: func() time.Time { return now },
	}
	issued, err := dispatchGateway.IssueOperationDispatchV4(context.Background(), ports.IssueGovernedOperationDispatchRequestV4{
		Operation: operation, EffectID: intent.ID, ExpectedEffectRevision: accepted.Revision, Admission: admission,
		ReviewAuthorization: authorization.RefV4(), PermitID: "permit-actual-point", AttemptID: "attempt-actual-point", PermitTTL: 10 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	begun, err := dispatchGateway.BeginOperationDispatchV4(context.Background(), ports.BeginGovernedOperationDispatchRequestV4{
		Operation: operation, EffectID: intent.ID, ExpectedEffectRevision: issued.Record.EffectFactRevision,
		PermitID: issued.Record.Permit.LegacyPermit.ID, ExpectedPermitFactRevision: issued.Record.Revision,
		AdmissionDigest: issued.Record.Permit.Admission.Digest, ReviewAuthorization: authorization.RefV4(),
	})
	if err != nil {
		t.Fatal(err)
	}
	legacy := begun.Record.Permit.LegacyPermit
	legacyDigest, _ := legacy.DigestV3()
	attempt := ports.OperationDispatchAttemptRefV3{
		OperationDigest: operationDigest, EffectID: intent.ID, IntentRevision: intent.Revision, IntentDigest: intentDigest,
		PermitID: legacy.ID, PermitRevision: legacy.Revision, PermitDigest: legacyDigest, AttemptID: legacy.AttemptID,
	}
	boundaryRef, err := ports.SealModelProviderBoundaryCurrentRefV1(ports.ModelProviderBoundaryCurrentRefV1{
		Owner: core.OwnerRef{Domain: "praxis.model", ID: "model-invoker"}, ID: "boundary-actual-point", Revision: 1,
		OperationDigest: operationDigest, EffectID: intent.ID, RuntimeAttempt: attempt, DispatchSequence: 11, ProviderAttemptOrdinal: 1,
		AttemptRequestDigest: actualPointDigestV1("attempt-request"), AcknowledgementDigest: actualPointDigestV1("ack"),
		ExpiresUnixNano: now.Add(8 * time.Second).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	boundaryProjection, err := ports.SealModelProviderBoundaryCurrentProjectionV1(ports.ModelProviderBoundaryCurrentProjectionV1{
		Ref: boundaryRef, State: ports.ModelProviderBoundaryCrossedV1, Provider: provider,
		CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(8 * time.Second).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	run := actualPointRunEnvelopeV1(t, operation, now)
	controlProjection, err := control.SealModelDispatchControlCurrentProjectionV1(control.ModelDispatchControlCurrentProjectionV1{
		OperationDigest: operationDigest, EffectID: intent.ID, RunID: operation.RunID, ExecutionScopeDigest: scopeDigest,
		RunRevision: run.Run.Revision, DesiredStateRevision: 1, LastCommandID: "command-actual-point",
		State: control.ModelDispatchControlDispatchableV1, WatermarkDigest: actualPointDigestV1("control"),
		CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(8 * time.Second).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	request := ports.InspectCurrentModelProviderActualPointRequestV1{
		Operation: operation, EffectID: intent.ID, ExpectedEffectRevision: begun.Record.EffectFactRevision,
		PermitID: legacy.ID, ExpectedPermitFactRevision: begun.Record.Revision, PermitDigest: begun.Record.PermitDigest,
		AdmissionDigest: begun.Record.Permit.Admission.Digest, ReviewAuthorization: authorization.RefV4(),
		Attempt: attempt, Verifier: legacy.EnforcementPoint, FenceDigest: legacy.FenceDigest, ModelBoundary: boundaryRef,
		RequestedNotAfterUnixNano: now.Add(7 * time.Second).UnixNano(),
	}
	fixture := &actualPointFixtureV1{
		now: now, provider: provider, verifier: legacy.EnforcementPoint, request: request,
		boundary:       &actualPointBoundaryReaderV1{value: boundaryProjection},
		run:            &actualPointRunReaderV1{value: run},
		runtimeControl: &actualPointControlReaderV1{value: controlProjection},
		effectReader:   &actualPointEffectReaderV1{store: effectStore},
		dispatchReader: &actualPointDispatchReaderV1{gateway: dispatchGateway},
		effectStore:    effectStore, governance: current, review: reviewReader,
	}
	fixture.guard = kernel.ModelProviderActualPointGuardGatewayV1{
		ModelBoundary: fixture.boundary, Runs: fixture.run, Control: fixture.runtimeControl,
		Effects: fixture.effectReader, Dispatch: fixture.dispatchReader, Clock: fixture.clock,
	}
	return fixture
}

func actualPointIntentV1(operation ports.OperationSubjectV3, provider ports.ProviderBindingRefV2, now time.Time) ports.OperationEffectIntentV3 {
	operationDigest, _ := operation.DigestV3()
	return ports.OperationEffectIntentV3{
		ContractVersion: ports.OperationEffectContractVersionV3, ID: "effect-actual-point", Revision: 1,
		Operation: operation, Kind: ports.ModelTurnEffectKindV1, RiskClass: "praxis.model/controlled",
		ActionScopeDigest: actualPointDigestV1("action-scope"),
		Payload: ports.OpaquePayloadV2{
			Schema:        ports.SchemaRefV2{Namespace: "praxis.model", Name: "turn", Version: "1.0.0", MediaType: "application/json", ContentDigest: actualPointDigestV1("schema")},
			ContentDigest: actualPointDigestV1("{}"), Length: 2, Inline: []byte("{}"),
			LimitPolicy: ports.OpaqueLimitPolicyRefV2{Policy: "praxis.model/payload-limit", Digest: actualPointDigestV1("limit")},
		},
		PayloadRevision: 1, Target: "model/provider",
		ConflictDomain: ports.ConflictDomainBindingV2{Domain: "praxis.model/turn", ScopeClass: ports.EffectStableScopeTenantV2, ScopeDigest: ports.StableTenantScopeDigestV2(operation.ExecutionScope.Identity.TenantID)},
		Owners: []ports.EffectOwnerRefV2{
			{Role: ports.OwnerCleanup, ComponentID: provider.ComponentID, ManifestDigest: provider.ManifestDigest},
			{Role: ports.OwnerEffect, ComponentID: provider.ComponentID, ManifestDigest: provider.ManifestDigest},
			{Role: ports.OwnerSettlement, ComponentID: provider.ComponentID, ManifestDigest: provider.ManifestDigest},
		},
		Provider:         provider,
		Authority:        ports.AuthorityBindingRefV2{Ref: "authority-actual-point", Digest: actualPointDigestV1("authority"), Revision: 1, Epoch: 1},
		Review:           ports.OperationReviewBindingRefV3{CaseRef: "review-case-actual-point", CandidateDigest: actualPointDigestV1("candidate"), CandidateRevision: 1, PolicyDigest: actualPointDigestV1("review-policy")},
		Budget:           ports.OperationBudgetBindingRefV3{Ref: "budget-actual-point", Digest: actualPointDigestV1("budget"), Revision: 1, PolicyDigest: actualPointDigestV1("budget-policy"), SubjectDigest: operationDigest},
		Policy:           ports.OperationPolicyBindingRefV3{Ref: "policy-actual-point", Digest: actualPointDigestV1("policy"), Revision: 1, SubjectDigest: operationDigest},
		Idempotency:      ports.IdempotencyBindingV2{Key: "model-turn-actual-point", ScopeClass: ports.EffectStableScopeTenantV2, ScopeDigest: ports.StableTenantScopeDigestV2(operation.ExecutionScope.Identity.TenantID), Class: core.IdempotencyQueryable},
		CredentialLeases: []ports.CredentialLeaseRefV2{}, ExpiresUnixNano: now.Add(30 * time.Second).UnixNano(),
	}
}

func actualPointGovernanceV1(intent ports.OperationEffectIntentV3, provider ports.ProviderBindingRefV2, now time.Time) ports.OperationGovernanceSnapshotV3 {
	ref := func(id string, digest core.Digest) ports.OperationGovernanceFactRefV3 {
		return ports.OperationGovernanceFactRefV3{Ref: id, Revision: 1, Digest: digest, ExpiresUnixNano: now.Add(20 * time.Second).UnixNano()}
	}
	return ports.OperationGovernanceSnapshotV3{
		Operation: intent.Operation, Active: true, ProjectionWatermark: 1,
		Identity:     ref("identity-actual-point", actualPointDigestV1("identity")),
		Binding:      ref(provider.BindingSetID, actualPointDigestV1("binding")),
		CurrentScope: ref(intent.Operation.CurrentProjectionRef, intent.Operation.CurrentProjectionDigest),
		Authority:    ref(intent.Authority.Ref, intent.Authority.Digest),
		Review: ports.OperationReviewAuthorizationV3{
			Case: ref(intent.Review.CaseRef, actualPointDigestV1("review-case")), CandidateDigest: intent.Review.CandidateDigest,
			CandidateRevision: intent.Review.CandidateRevision, Verdict: ref("verdict-actual-point", actualPointDigestV1("verdict")),
			ReviewerAuthority: ref("reviewer-actual-point", actualPointDigestV1("reviewer")), PolicyDigest: intent.Review.PolicyDigest,
			ExpiresUnixNano: now.Add(20 * time.Second).UnixNano(),
		},
		Budget: ref(intent.Budget.Ref, intent.Budget.Digest), Policy: ref(intent.Policy.Ref, intent.Policy.Digest),
		Provider: provider, EnforcementPoint: provider, CapabilityGrantDigest: actualPointDigestV1("capability"),
		Credentials: []ports.OperationCredentialCurrentFactV3{}, ExpiresUnixNano: now.Add(20 * time.Second).UnixNano(),
	}
}

func actualPointReviewV1(intent ports.OperationEffectIntentV3, intentDigest core.Digest, governance ports.OperationGovernanceSnapshotV3, now time.Time) ports.OperationReviewCurrentProjectionV4 {
	value := ports.OperationReviewCurrentProjectionV4{
		Operation: intent.Operation, IntentID: intent.ID, IntentRevision: intent.Revision, IntentDigest: intentDigest,
		PayloadSchema: intent.Payload.Schema, PayloadDigest: intent.Payload.ContentDigest, PayloadRevision: intent.PayloadRevision,
		Target: ports.OperationReviewTargetRefV4{Ref: intent.Target, Revision: intent.Review.CandidateRevision, Digest: intent.Review.CandidateDigest},
		Case:   governance.Review.Case, Verdict: governance.Review.Verdict, Basis: ports.OperationReviewBasisAcceptedV4,
		Policy:            ports.OperationGovernanceFactRefV3{Ref: "review-policy-actual-point", Revision: 1, Digest: intent.Review.PolicyDigest, ExpiresUnixNano: now.Add(20 * time.Second).UnixNano()},
		ReviewerAuthority: governance.Review.ReviewerAuthority, Scope: governance.CurrentScope, Binding: governance.Binding,
		DecisionEvidence: []ports.EvidenceRecordRefV2{{LedgerScopeDigest: actualPointDigestV1("ledger"), Sequence: 1, RecordDigest: actualPointDigestV1("evidence")}},
		Current:          true, CurrentnessDigest: actualPointDigestV1("review-currentness"), ExpiresUnixNano: now.Add(20 * time.Second).UnixNano(),
	}
	value, _ = ports.SealOperationReviewCurrentProjectionV4(value, now)
	return value
}

func actualPointRunEnvelopeV1(t *testing.T, operation ports.OperationSubjectV3, now time.Time) ports.RunLifecycleEnvelopeV3 {
	t.Helper()
	run := core.AgentRunRecord{ID: operation.RunID, Scope: operation.ExecutionScope, Status: core.RunRunning, Revision: 4, SessionRef: "session-actual-point", StartedAt: now.Add(-time.Minute)}
	identityDigest, _ := ports.RunIdentityDigestV2(run)
	scopeDigest, _ := ports.ExecutionScopeDigestV2(run.Scope)
	planRef := ports.RunSettlementPlanRefV2{ID: "plan-actual-point", Revision: 1, Digest: actualPointDigestV1("plan-ref")}
	return ports.RunLifecycleEnvelopeV3{
		ContractVersion: ports.RunLifecycleContractVersionV3, Phase: ports.RunLifecycleRunningV3, Run: run,
		Plan: ports.RunSettlementPlanLifecycleRefV3{RunSettlementPlanRefV2: planRef, RunID: run.ID, RunIdentityDigest: identityDigest, ExecutionScopeDigest: scopeDigest},
		Certification: ports.RunSettlementPlanCertificationAssociationV3{
			Certification: ports.RunSettlementPlanCertificationRefV3{ID: "certification-actual-point", Revision: 1, Digest: actualPointDigestV1("certification")},
			RunID:         run.ID, RunIdentityDigest: identityDigest, ExecutionScopeDigest: scopeDigest, Plan: planRef,
		},
		EffectIndex: ports.RunEffectIndexRefV3{
			ID: "effect-index-actual-point", Revision: 1, Digest: actualPointDigestV1("index"), RunID: run.ID,
			RunIdentityDigest: identityDigest, ExecutionScopeDigest: scopeDigest, Watermark: 1, HeadDigest: ports.EvidenceGenesisDigestV2,
		},
	}
}

type actualPointGovernanceReaderV1 struct {
	value ports.OperationGovernanceSnapshotV3
}

func (r *actualPointGovernanceReaderV1) InspectOperationGovernance(context.Context, ports.OperationSubjectV3) (ports.OperationGovernanceSnapshotV3, error) {
	return r.value, nil
}

type actualPointReviewReaderV1 struct {
	value ports.OperationReviewCurrentProjectionV4
}

func (r actualPointReviewReaderV1) InspectOperationReviewCurrentV4(context.Context, ports.OperationEffectIntentV3) (ports.OperationReviewCurrentProjectionV4, error) {
	return r.value, nil
}

type actualPointBoundaryReaderV1 struct {
	value ports.ModelProviderBoundaryCurrentProjectionV1
}

func (r *actualPointBoundaryReaderV1) InspectCurrentModelProviderBoundaryV1(context.Context, ports.ModelProviderBoundaryCurrentRefV1) (ports.ModelProviderBoundaryCurrentProjectionV1, error) {
	return r.value, nil
}

type actualPointRunReaderV1 struct{ value ports.RunLifecycleEnvelopeV3 }

func (r *actualPointRunReaderV1) InspectRunLifecycleV3(context.Context, core.ExecutionScope, core.AgentRunID) (ports.RunLifecycleEnvelopeV3, error) {
	return r.value, nil
}

type actualPointControlReaderV1 struct {
	value      control.ModelDispatchControlCurrentProjectionV1
	calls      int
	afterFirst func(control.ModelDispatchControlCurrentProjectionV1) control.ModelDispatchControlCurrentProjectionV1
}

func (r *actualPointControlReaderV1) InspectModelDispatchControlCurrentV1(context.Context, ports.OperationSubjectV3, core.EffectIntentID) (control.ModelDispatchControlCurrentProjectionV1, error) {
	r.calls++
	if r.calls > 1 && r.afterFirst != nil {
		return r.afterFirst(r.value), nil
	}
	return r.value, nil
}

type actualPointEffectReaderV1 struct {
	store  *fakes.OperationEffectStoreV3
	mutate func(control.OperationEffectFactV3) control.OperationEffectFactV3
}

func (r *actualPointEffectReaderV1) InspectOperationEffectV3(ctx context.Context, operation ports.OperationSubjectV3, effectID core.EffectIntentID) (control.OperationEffectFactV3, error) {
	value, err := r.store.InspectOperationEffectV3(ctx, operation, effectID)
	if err == nil && r.mutate != nil {
		value = r.mutate(value)
	}
	return value, err
}

type actualPointDispatchReaderV1 struct {
	gateway control.OperationGovernanceGatewayV4
	mutate  func(ports.CurrentOperationDispatchAuthorizationV4) ports.CurrentOperationDispatchAuthorizationV4
}

func (r *actualPointDispatchReaderV1) InspectCurrentOperationDispatchV4(ctx context.Context, request ports.InspectCurrentOperationDispatchRequestV4) (ports.CurrentOperationDispatchAuthorizationV4, error) {
	value, err := r.gateway.InspectCurrentOperationDispatchV4(ctx, request)
	if err == nil && r.mutate != nil {
		value = r.mutate(value)
	}
	return value, err
}

func actualPointProviderV1(component ports.ComponentIDV2, capability ports.CapabilityNameV2) ports.ProviderBindingRefV2 {
	return ports.ProviderBindingRefV2{
		BindingSetID: "binding-actual-point", BindingSetRevision: 1, ComponentID: component,
		ManifestDigest: actualPointDigestV1("manifest-" + string(component)), ArtifactDigest: actualPointDigestV1("artifact-" + string(component)), Capability: capability,
	}
}

func actualPointDigestV1(value string) core.Digest {
	return core.DigestBytes([]byte(value))
}
