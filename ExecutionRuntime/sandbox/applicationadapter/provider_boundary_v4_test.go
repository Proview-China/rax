package applicationadapter

import (
	"context"
	"errors"
	"testing"
	"time"

	runtimecore "github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/dataplaneadapter"
)

type enforcementPortStubV4 struct {
	current runtimeports.CurrentOperationDispatchEnforcementV4
	err     error
	calls   int
}

func (p *enforcementPortStubV4) EnforceCurrentOperationDispatchV4(context.Context, runtimeports.EnforceCurrentOperationDispatchRequestV4) (runtimeports.CurrentOperationDispatchEnforcementV4, error) {
	p.calls++
	return p.current, p.err
}

func (p *enforcementPortStubV4) InspectOperationDispatchEnforcementV4(context.Context, runtimeports.InspectOperationDispatchEnforcementRequestV4) (runtimeports.OperationDispatchEnforcementJournalV4, error) {
	return runtimeports.OperationDispatchEnforcementJournalV4{}, errors.New("unexpected historical inspect")
}

func (p *enforcementPortStubV4) InspectCurrentOperationDispatchEnforcementV4(context.Context, runtimeports.InspectCurrentOperationDispatchEnforcementRequestV4) (runtimeports.CurrentOperationDispatchEnforcementV4, error) {
	return runtimeports.CurrentOperationDispatchEnforcementV4{}, errors.New("unexpected current inspect")
}

type evidencePortStubV3 struct{ calls int }

func (p *evidencePortStubV3) IssueOperationScopeEvidenceV3(context.Context, runtimeports.IssueOperationScopeEvidenceRequestV3) (runtimeports.OperationScopeEvidenceQualificationFactV3, error) {
	p.calls++
	return runtimeports.OperationScopeEvidenceQualificationFactV3{}, errors.New("unexpected evidence issue")
}
func (p *evidencePortStubV3) InspectOperationScopeEvidenceV3(context.Context, runtimeports.InspectOperationScopeEvidenceRequestV3) (runtimeports.OperationScopeEvidenceQualificationFactV3, error) {
	p.calls++
	return runtimeports.OperationScopeEvidenceQualificationFactV3{}, errors.New("unexpected evidence inspect")
}
func (p *evidencePortStubV3) InspectCurrentOperationScopeEvidenceV3(context.Context, runtimeports.InspectCurrentOperationScopeEvidenceRequestV3) (runtimeports.OperationScopeEvidenceQualificationFactV3, error) {
	p.calls++
	return runtimeports.OperationScopeEvidenceQualificationFactV3{}, errors.New("unexpected evidence current")
}
func (p *evidencePortStubV3) HandoffOperationScopeEvidenceV3(context.Context, runtimeports.HandoffOperationScopeEvidenceRequestV3) (runtimeports.OperationScopeEvidenceProviderHandoffFactV3, error) {
	p.calls++
	return runtimeports.OperationScopeEvidenceProviderHandoffFactV3{}, errors.New("unexpected evidence handoff")
}
func (p *evidencePortStubV3) ConsumeOperationScopeEvidenceV3(context.Context, runtimeports.ConsumeOperationScopeEvidenceRequestV3) (runtimeports.OperationScopeEvidenceConsumeResultV3, error) {
	p.calls++
	return runtimeports.OperationScopeEvidenceConsumeResultV3{}, errors.New("unexpected evidence consume")
}

type dataPlanePortStubV1 struct {
	dispatchCalls int
	inspectCalls  int
}

func (p *dataPlanePortStubV1) Dispatch(context.Context, dataplaneadapter.DispatchRequestV1) (dataplaneadapter.DispatchResponseV1, error) {
	p.dispatchCalls++
	return dataplaneadapter.DispatchResponseV1{}, errors.New("unexpected provider dispatch")
}
func (p *dataPlanePortStubV1) Inspect(context.Context, dataplaneadapter.DispatchRequestV1) (dataplaneadapter.DispatchResponseV1, error) {
	p.inspectCalls++
	return dataplaneadapter.DispatchResponseV1{}, errors.New("unexpected provider inspect")
}

func TestProviderBoundaryV4StopsBeforeEvidenceAndProviderOnEnforcementFailure(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	enforcement := &enforcementPortStubV4{err: errors.New("current governance rejected")}
	evidence := &evidencePortStubV3{}
	dataplane := &dataPlanePortStubV1{}
	boundary, err := NewProviderBoundaryV4(enforcement, evidence, dataplane, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := boundary.ExecutePhase(context.Background(), providerPhasePlanFixtureV4(t, now)); err == nil {
		t.Fatal("enforcement failure was ignored")
	}
	if enforcement.calls != 1 || evidence.calls != 0 || dataplane.dispatchCalls != 0 || dataplane.inspectCalls != 0 {
		t.Fatalf("early gate leaked calls enforcement=%d evidence=%d dispatch=%d inspect=%d", enforcement.calls, evidence.calls, dataplane.dispatchCalls, dataplane.inspectCalls)
	}
}

func TestProviderBoundaryV4RejectsAnotherOrZeroCurrentBeforeEvidenceAndProvider(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	enforcement := &enforcementPortStubV4{}
	evidence := &evidencePortStubV3{}
	dataplane := &dataPlanePortStubV1{}
	boundary, err := NewProviderBoundaryV4(enforcement, evidence, dataplane, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := boundary.ExecutePhase(context.Background(), providerPhasePlanFixtureV4(t, now)); err == nil {
		t.Fatal("zero Runtime current reached Evidence or Provider")
	}
	if evidence.calls != 0 || dataplane.dispatchCalls != 0 || dataplane.inspectCalls != 0 {
		t.Fatalf("invalid current leaked calls evidence=%d dispatch=%d inspect=%d", evidence.calls, dataplane.dispatchCalls, dataplane.inspectCalls)
	}
}

func TestProviderBoundaryV4RejectsInvalidPlanAndTypedNilDependencies(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	enforcement := &enforcementPortStubV4{}
	evidence := &evidencePortStubV3{}
	dataplane := &dataPlanePortStubV1{}
	boundary, err := NewProviderBoundaryV4(enforcement, evidence, dataplane, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := boundary.ExecutePhase(context.Background(), ProviderPhasePlanV4{}); err == nil || enforcement.calls != 0 {
		t.Fatal("invalid plan reached Runtime enforcement")
	}
	var typedNil *dataPlanePortStubV1
	if _, err := NewProviderBoundaryV4(enforcement, evidence, typedNil, func() time.Time { return now }); err == nil {
		t.Fatal("typed-nil Data Plane was accepted")
	}
}

func providerPhasePlanFixtureV4(t *testing.T, now time.Time) ProviderPhasePlanV4 {
	t.Helper()
	digest := func(value string) runtimecore.Digest { return runtimecore.DigestBytes([]byte(value)) }
	lease := runtimecore.SandboxLeaseRef{ID: "lease-1", Epoch: 1}
	scope := runtimecore.ExecutionScope{
		Identity: runtimecore.AgentIdentityRef{TenantID: "tenant-1", ID: "identity-1", Epoch: 1},
		Lineage:  runtimecore.LineageRef{ID: "lineage-1", PlanDigest: digest("plan")},
		Instance: runtimecore.InstanceRef{ID: "instance-1", Epoch: 1}, SandboxLease: &lease, AuthorityEpoch: 1,
	}
	scopeDigest, err := runtimeports.ExecutionScopeDigestV2(scope)
	if err != nil {
		t.Fatal(err)
	}
	operation := runtimeports.OperationSubjectV3{
		Kind: runtimeports.OperationScopeActivationV3, ExecutionScope: scope, ExecutionScopeDigest: scopeDigest,
		ActivationAttemptID: "activation-1", SubjectRevision: 1, CurrentProjectionRef: "activation-current-1",
		CurrentProjectionDigest: digest("activation-current"), CurrentProjectionRevision: 1,
	}
	operationDigest, err := operation.DigestV3()
	if err != nil {
		t.Fatal(err)
	}
	effectID := runtimecore.EffectIntentID("effect-1")
	attemptID := "attempt-1"
	expires := now.Add(time.Minute).UnixNano()
	factRef := func(id string) runtimeports.OperationDispatchSandboxFactRefV4 {
		return runtimeports.OperationDispatchSandboxFactRefV4{ID: id, Revision: 1, Digest: digest(id), ExpiresUnixNano: expires}
	}
	provider := runtimeports.ProviderBindingRefV2{BindingSetID: "bindings-1", BindingSetRevision: 1, ComponentID: "praxis.sandbox/provider", ManifestDigest: digest("manifest"), ArtifactDigest: digest("artifact"), Capability: "praxis.sandbox/execute"}
	authorization := runtimeports.OperationReviewAuthorizationRefV4{ID: "authorization-1", Revision: 1, Digest: digest("authorization")}
	permitDigest := digest("permit")
	admissionDigest := digest("admission")
	intentDigest := digest("intent")
	applicability := runtimeports.NormalizeOperationScopeEvidenceApplicabilityV3([]runtimeports.OperationScopeEvidenceApplicabilityV3{
		{Dimension: runtimeports.OperationScopeEvidenceRunV3, Mode: runtimeports.OperationScopeEvidenceForbiddenV3},
		{Dimension: runtimeports.OperationScopeEvidenceSessionV3, Mode: runtimeports.OperationScopeEvidenceForbiddenV3},
		{Dimension: runtimeports.OperationScopeEvidenceTurnV3, Mode: runtimeports.OperationScopeEvidenceForbiddenV3},
		{Dimension: runtimeports.OperationScopeEvidenceActionV3, Mode: runtimeports.OperationScopeEvidenceForbiddenV3},
		{Dimension: runtimeports.OperationScopeEvidenceContextV3, Mode: runtimeports.OperationScopeEvidenceForbiddenV3},
	})
	appPolicy := runtimeports.OperationScopeEvidenceApplicabilityPolicyRefV3{ID: "applicability-1", Revision: 1, Digest: digest("applicability"), ExpiresUnixNano: expires}
	evidenceScope := runtimeports.OperationScopeEvidenceScopeV3{
		LedgerScope: runtimeports.OperationScopeEvidenceLedgerScopeV3{TenantID: "tenant-1", OperationDigest: operationDigest, ChainID: "evidence-chain-1"},
		Operation:   operation, OperationDigest: operationDigest, EffectID: effectID, EffectRevision: 1, EffectDigest: intentDigest,
		EffectKind: "praxis.sandbox/allocate", AttemptID: attemptID, Phase: runtimeports.OperationDispatchEnforcementPrepareV4,
		ApplicabilityPolicy: appPolicy, Applicability: applicability,
		Generation: runtimeports.GenerationBindingAssociationRefV1{ID: "generation-1", Revision: 1, Digest: digest("generation")},
	}
	schema := runtimeports.SchemaRefV2{Namespace: "praxis.sandbox", Name: "provider-observation", Version: "1.0.0", MediaType: "application/json", ContentDigest: digest("schema")}
	registration := runtimeports.OperationScopeEvidenceFactRefV3{ID: "source-1", Revision: 1, Digest: digest("source"), ExpiresUnixNano: expires}
	attempt := runtimeports.OperationDispatchAttemptRefV3{OperationDigest: operationDigest, EffectID: effectID, IntentRevision: 1, IntentDigest: intentDigest, PermitID: "permit-1", PermitRevision: 1, PermitDigest: permitDigest, AttemptID: attemptID}
	payload, err := dataplaneadapter.NewWasmPayload(dataplaneadapter.WasmPayloadV1{ComponentPathBindingID: "component-1", ComponentDigest: string(digest("component")), World: "praxis:sandbox/capability@1.0.0", Export: "run", Fuel: 1000, EpochDeadlineTicks: 10, MemoryLimitBytes: 16 << 20, TableElementsLimit: 128, InstanceLimit: 4})
	if err != nil {
		t.Fatal(err)
	}
	plan := ProviderPhasePlanV4{
		Enforcement: runtimeports.EnforceCurrentOperationDispatchRequestV4{
			Operation: operation, EffectID: effectID, PermitID: "permit-1", ExpectedPermitFactRevision: 1,
			PermitDigest: permitDigest, AdmissionDigest: admissionDigest, ReviewAuthorization: authorization, AttemptID: attemptID,
			Phase: runtimeports.OperationDispatchEnforcementPrepareV4, SandboxAttempt: factRef(attemptID), SandboxReservation: factRef("reservation-1"),
			SandboxProjectionDigest: digest("sandbox-projection"), Verifier: provider,
		},
		Evidence: ProviderPhaseEvidencePlanV4{
			QualificationID: "qualification-1", HandoffID: "handoff-1", ConsumptionID: "consumption-1", Scope: evidenceScope,
			EvidencePolicy: runtimeports.OperationScopeEvidencePolicyRefV3{ID: "evidence-policy-1", Revision: 1, Digest: digest("evidence-policy"), ExpiresUnixNano: expires},
			Reservation:    runtimeports.OperationScopeEvidenceSourceReservationV3{Registration: registration, Source: runtimeports.OperationScopeEvidenceSourceKeyV3{RegistrationID: registration.ID, SourceEpoch: 1, SourceSequence: 1}, EventID: "event-1", Schema: schema},
			RequestedTTL:   time.Second, PayloadSchema: schema, CorrelationID: "correlation-1",
		},
		RequestID: "provider-request-1", EffectKind: "praxis.sandbox/allocate", PayloadSchema: "praxis.sandbox/provider-payload/v1", PayloadRevision: 1,
		Payload: payload, RequestedNotAfter: now.Add(time.Second), Attempt: attempt,
	}
	if err := validatePhasePlanV4(plan); err != nil {
		t.Fatal(err)
	}
	return plan
}
