package control

import (
	"context"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type operationEffectReadSpyV3 struct {
	OperationEffectFactPortV3
	reads int
}

func (s *operationEffectReadSpyV3) InspectOperationEffectV3(context.Context, ports.OperationSubjectV3, core.EffectIntentID) (OperationEffectFactV3, error) {
	s.reads++
	return OperationEffectFactV3{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "unexpected Effect read")
}

func (s *operationEffectReadSpyV3) InspectOperationDispatchPermitV3(context.Context, ports.OperationSubjectV3, string) (OperationDispatchPermitFactV3, error) {
	s.reads++
	return OperationDispatchPermitFactV3{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "unexpected Permit read")
}

type operationCurrentNoopV3 struct {
	ports.OperationGovernanceCurrentReaderV3
}
type observationOwnerNoopV3 struct {
	ports.ProviderAttemptObservationFactPortV2
}
type delegationOwnerNoopV3 struct {
	ports.ExecutionDelegationFactPortV2
}
type dispatchCurrentNoopV3 struct {
	ports.OperationDispatchCurrentReaderV3
}

type evidenceSourceReadSpyV3 struct {
	ports.EvidenceSourceRecordReaderV2
	reads int
}

func (s *evidenceSourceReadSpyV3) InspectBySource(context.Context, ports.EvidenceSourceKeyV2) (ports.EvidenceLedgerRecordV2, error) {
	s.reads++
	return ports.EvidenceLedgerRecordV2{}, core.NewError(core.ErrorNotFound, core.ReasonEvidenceUnavailable, "unexpected Evidence read")
}

func (s *evidenceSourceReadSpyV3) InspectRecord(context.Context, ports.EvidenceRecordRefV2) (ports.EvidenceLedgerRecordV2, error) {
	s.reads++
	return ports.EvidenceLedgerRecordV2{}, core.NewError(core.ErrorNotFound, core.ReasonEvidenceUnavailable, "unexpected Evidence read")
}

type certificationReadSpyV3 struct {
	ports.RunSettlementPlanCertificationFactPortV3
	reads int
}

func (s *certificationReadSpyV3) InspectRunSettlementPlanCertificationV3(context.Context, core.ExecutionScope, core.AgentRunID) (ports.RunSettlementPlanCertificationFactV3, error) {
	s.reads++
	return ports.RunSettlementPlanCertificationFactV3{}, core.NewError(core.ErrorNotFound, core.ReasonRunSettlementPlanConflict, "unexpected certification read")
}

func validPublicGatewayOperationSubjectV3(t *testing.T) ports.OperationSubjectV3 {
	t.Helper()
	scope := core.ExecutionScope{
		Identity:       core.AgentIdentityRef{TenantID: "tenant-public-gateway", ID: "identity-public-gateway", Epoch: 1},
		Lineage:        core.LineageRef{ID: "lineage-public-gateway", PlanDigest: core.DigestBytes([]byte("lineage-public-gateway"))},
		Instance:       core.InstanceRef{ID: "instance-public-gateway", Epoch: 1},
		AuthorityEpoch: 1,
	}
	scopeDigest, err := ports.ExecutionScopeDigestV2(scope)
	if err != nil {
		t.Fatal(err)
	}
	return ports.OperationSubjectV3{
		Kind:                      ports.OperationScopeActivationV3,
		ExecutionScope:            scope,
		ExecutionScopeDigest:      scopeDigest,
		ActivationAttemptID:       "activation-public-gateway",
		SubjectRevision:           1,
		CurrentProjectionRef:      "current-public-gateway",
		CurrentProjectionDigest:   core.DigestBytes([]byte("current-public-gateway")),
		CurrentProjectionRevision: 1,
	}
}

func TestOperationGovernanceV3BeginRejectsInvalidEnvelopeBeforeFactReads(t *testing.T) {
	spy := &operationEffectReadSpyV3{}
	gateway := OperationGovernanceGatewayV3{Effects: spy, Current: &operationCurrentNoopV3{}, Clock: func() time.Time { return time.Unix(1, 0) }}
	_, err := gateway.BeginOperationDispatchV3(context.Background(), ports.BeginGovernedOperationDispatchRequestV3{})
	if err == nil || spy.reads != 0 {
		t.Fatalf("invalid Begin reached Fact Owner: err=%v reads=%d", err, spy.reads)
	}
}

func TestOperationGovernanceV3InspectAuthorizationValidatesBeforeFactReads(t *testing.T) {
	spy := &operationEffectReadSpyV3{}
	gateway := OperationGovernanceGatewayV3{Effects: spy, Current: &operationCurrentNoopV3{}, Clock: func() time.Time { return time.Unix(1, 0) }}
	_, err := gateway.InspectOperationDispatchAuthorizationV3(context.Background(), ports.OperationSubjectV3{}, "", "")
	if err == nil || spy.reads != 0 {
		t.Fatalf("invalid authorization inspection reached Fact Owner: err=%v reads=%d", err, spy.reads)
	}
}

func TestOperationGovernanceV3MarkUnknownRejectsInvalidEnvelopeBeforeFactReads(t *testing.T) {
	spy := &operationEffectReadSpyV3{}
	subject := validPublicGatewayOperationSubjectV3(t)
	operationDigest, _ := subject.DigestV3()
	request := ports.MarkOperationDispatchUnknownRequestV3{
		Operation: subject,
		Permit: ports.OperationDispatchAttemptRefV3{
			OperationDigest: operationDigest,
			EffectID:        "effect-other",
			IntentRevision:  1,
			IntentDigest:    core.DigestBytes([]byte("intent")),
			PermitID:        "permit-public-gateway",
			PermitRevision:  1,
			PermitDigest:    core.DigestBytes([]byte("permit")),
			AttemptID:       "attempt-public-gateway",
		},
	}
	gateway := OperationGovernanceGatewayV3{Effects: spy, Current: &operationCurrentNoopV3{}, Clock: func() time.Time { return time.Unix(1, 0) }}
	_, err := gateway.MarkOperationDispatchUnknownV3(context.Background(), request)
	if err == nil || spy.reads != 0 {
		t.Fatalf("invalid unknown transition reached Fact Owner: err=%v reads=%d", err, spy.reads)
	}
}

func TestObservationRejectsInvalidEnvelopeBeforeEvidenceReads(t *testing.T) {
	evidence := &evidenceSourceReadSpyV3{}
	gateway := OperationObservationGovernanceGatewayV3{
		Effects: &operationEffectReadSpyV3{}, Observations: &observationOwnerNoopV3{}, Delegations: &delegationOwnerNoopV3{},
		Current: &operationCurrentNoopV3{}, Dispatch: &dispatchCurrentNoopV3{}, Evidence: evidence, Clock: func() time.Time { return time.Unix(1, 0) },
	}
	_, err := gateway.RecordGovernedProviderObservationV3(context.Background(), ports.RecordGovernedProviderObservationRequestV2{})
	if err == nil || evidence.reads != 0 {
		t.Fatalf("invalid Observation envelope reached Evidence: err=%v reads=%d", err, evidence.reads)
	}
}

func TestObservationInspectValidatesBeforeBackend(t *testing.T) {
	owner := &observationOwnerSpyV3{}
	gateway := OperationObservationGovernanceGatewayV3{Observations: owner}
	_, err := gateway.InspectGovernedProviderObservationV3(context.Background(), ports.ExecutionDelegationRefV2{}, "")
	if err == nil || owner.reads != 0 {
		t.Fatalf("invalid Observation inspection reached backend: err=%v reads=%d", err, owner.reads)
	}
}

type observationOwnerSpyV3 struct {
	ports.ProviderAttemptObservationFactPortV2
	reads int
}

func (s *observationOwnerSpyV3) InspectProviderAttemptObservationV2(context.Context, ports.ExecutionDelegationRefV2, string) (ports.ProviderAttemptObservationV2, error) {
	s.reads++
	return ports.ProviderAttemptObservationV2{}, core.NewError(core.ErrorNotFound, core.ReasonEvidenceUnavailable, "unexpected Observation read")
}

func TestSettlementInspectValidatesBeforeBackend(t *testing.T) {
	spy := &operationEffectReadSpyV3{}
	gateway := OperationSettlementGovernanceGatewayV3{Effects: spy}
	_, err := gateway.InspectOperationSettlementV3(context.Background(), ports.OperationSubjectV3{}, "")
	if err == nil || spy.reads != 0 {
		t.Fatalf("invalid settlement inspection reached backend: err=%v reads=%d", err, spy.reads)
	}
}

func TestAdmissionInspectRejectsEmptyEffectIDBeforeBackend(t *testing.T) {
	spy := &operationEffectReadSpyV3{}
	gateway := OperationEffectAdmissionGatewayV3{Effects: spy}
	_, err := gateway.InspectAcceptedOperationEffectV3(context.Background(), validPublicGatewayOperationSubjectV3(t), "")
	if err == nil || spy.reads != 0 {
		t.Fatalf("empty Effect ID reached admission backend: err=%v reads=%d", err, spy.reads)
	}
}

func TestRunPlanAdmissionPublicReadsValidateBeforeBackend(t *testing.T) {
	spy := &certificationReadSpyV3{}
	gateway := RunSettlementPlanAdmissionGatewayV3{Certifications: spy}
	if _, err := gateway.InspectCertifiedRunSettlementPlanV3(context.Background(), core.ExecutionScope{}, ""); err == nil {
		t.Fatal("invalid certification inspection unexpectedly succeeded")
	}
	if err := gateway.ValidateRunSettlementPlanCertificationV3(context.Background(), ports.RunSettlementPlanCertificationRefV3{}, core.AgentRunRecord{}, ports.RunSettlementPlanFactV2{}); err == nil {
		t.Fatal("invalid certification validation unexpectedly succeeded")
	}
	if spy.reads != 0 {
		t.Fatalf("invalid Plan admission input reached certification backend: reads=%d", spy.reads)
	}
}
