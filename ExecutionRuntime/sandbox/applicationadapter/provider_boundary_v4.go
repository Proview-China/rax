package applicationadapter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"time"

	runtimecore "github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/dataplaneadapter"
)

type DataPlanePortV1 interface {
	Dispatch(context.Context, dataplaneadapter.DispatchRequestV1) (dataplaneadapter.DispatchResponseV1, error)
	Inspect(context.Context, dataplaneadapter.DispatchRequestV1) (dataplaneadapter.DispatchResponseV1, error)
}

type ProviderPhaseEvidencePlanV4 struct {
	QualificationID string
	HandoffID       string
	ConsumptionID   string
	Scope           runtimeports.OperationScopeEvidenceScopeV3
	EvidencePolicy  runtimeports.OperationScopeEvidencePolicyRefV3
	Reservation     runtimeports.OperationScopeEvidenceSourceReservationV3
	RequestedTTL    time.Duration
	PayloadSchema   runtimeports.SchemaRefV2
	CorrelationID   string
}

type ProviderPhasePlanV4 struct {
	Enforcement       runtimeports.EnforceCurrentOperationDispatchRequestV4
	Evidence          ProviderPhaseEvidencePlanV4
	RequestID         string
	EffectKind        string
	PayloadSchema     string
	PayloadRevision   uint64
	Payload           dataplaneadapter.ProviderPayloadV1
	RequestedNotAfter time.Time
	Attempt           runtimeports.OperationDispatchAttemptRefV3
}

type ProviderPhaseResultV4 struct {
	Current       runtimeports.CurrentOperationDispatchEnforcementV4
	Dispatch      dataplaneadapter.DispatchRequestV1
	Response      dataplaneadapter.DispatchResponseV1
	Qualification runtimeports.OperationScopeEvidenceQualificationFactV3
	Handoff       runtimeports.OperationScopeEvidenceProviderHandoffFactV3
	Consumption   runtimeports.OperationScopeEvidenceConsumeResultV3
	Binding       runtimeports.OperationSettlementEvidenceBindingV4
}

type ProviderBoundaryV4 struct {
	enforcement runtimeports.OperationDispatchEnforcementGovernancePortV4
	evidence    runtimeports.OperationScopeEvidenceGovernancePortV3
	dataplane   DataPlanePortV1
	now         func() time.Time
}

func NewProviderBoundaryV4(enforcement runtimeports.OperationDispatchEnforcementGovernancePortV4, evidence runtimeports.OperationScopeEvidenceGovernancePortV3, dataplane DataPlanePortV1, now func() time.Time) (*ProviderBoundaryV4, error) {
	if nilLike(enforcement) || nilLike(evidence) || nilLike(dataplane) || nilLike(now) {
		return nil, errors.New("provider boundary requires enforcement, evidence, data plane, and clock")
	}
	return &ProviderBoundaryV4{enforcement: enforcement, evidence: evidence, dataplane: dataplane, now: now}, nil
}

// ExecutePhase crosses exactly one prepare or execute physical boundary. The
// persisted Runtime enforcement receipt is created before Provider entry, and
// a lost Provider reply is recovered only by Data Plane Inspect.
func (b *ProviderBoundaryV4) ExecutePhase(ctx context.Context, plan ProviderPhasePlanV4) (ProviderPhaseResultV4, error) {
	if b == nil || nilLike(ctx) {
		return ProviderPhaseResultV4{}, errors.New("provider boundary or context is nil")
	}
	if err := validatePhasePlanV4(plan); err != nil {
		return ProviderPhaseResultV4{}, err
	}
	current, err := b.enforcement.EnforceCurrentOperationDispatchV4(ctx, plan.Enforcement)
	if err != nil {
		return ProviderPhaseResultV4{}, err
	}
	now := b.now()
	if err := validateEnforcementCurrentForRequestV4(current, plan.Enforcement, now); err != nil {
		return ProviderPhaseResultV4{}, errors.New("Runtime enforcement returned another phase")
	}

	scope := plan.Evidence.Scope
	scope.Phase = current.Phase.Phase
	issue := runtimeports.IssueOperationScopeEvidenceRequestV3{
		QualificationID: plan.Evidence.QualificationID, Scope: scope,
		PermitID: current.Phase.PermitID, PermitFactRevision: current.Phase.PermitFactRevision,
		PermitDigest: current.Phase.PermitDigest, AdmissionDigest: current.Phase.AdmissionDigest,
		Authorization: current.Phase.ReviewAuthorization, PhaseRef: current.Phase,
		EvidencePolicy: plan.Evidence.EvidencePolicy, Reservation: plan.Evidence.Reservation,
		RequestedTTL: plan.Evidence.RequestedTTL,
	}
	qualification, err := b.evidence.IssueOperationScopeEvidenceV3(ctx, issue)
	if err != nil {
		recovered, inspectErr := b.evidence.InspectOperationScopeEvidenceV3(context.WithoutCancel(ctx), runtimeports.InspectOperationScopeEvidenceRequestV3{QualificationID: issue.QualificationID})
		if inspectErr != nil {
			return ProviderPhaseResultV4{}, err
		}
		qualification = recovered
	}
	if err := qualification.Validate(); err != nil || qualification.State != runtimeports.OperationScopeEvidenceIssuedV3 || !reflect.DeepEqual(qualification.Scope, scope) || qualification.EvidencePolicy != plan.Evidence.EvidencePolicy || !reflect.DeepEqual(qualification.Reservation, plan.Evidence.Reservation) || qualification.Runtime.Phase != current.Phase || !now.Before(time.Unix(0, qualification.ExpiresUnixNano)) {
		return ProviderPhaseResultV4{}, errors.New("Evidence qualification returned another Runtime phase")
	}
	handoff, err := b.evidence.HandoffOperationScopeEvidenceV3(ctx, runtimeports.HandoffOperationScopeEvidenceRequestV3{HandoffID: plan.Evidence.HandoffID, Qualification: qualification.RefV3()})
	if err != nil {
		return ProviderPhaseResultV4{}, err
	}
	if err := handoff.Validate(); err != nil || handoff.Qualification != qualification.RefV3() || handoff.Phase != current.Phase || !now.Before(time.Unix(0, handoff.NotAfterUnixNano)) {
		return ProviderPhaseResultV4{}, errors.New("Evidence handoff returned another qualification or phase")
	}

	dispatch, err := dataplaneadapter.NewDispatchRequestV1(dataplaneadapter.DispatchInput{
		RequestID: plan.RequestID, Current: current, EffectKind: plan.EffectKind,
		PayloadSchema: plan.PayloadSchema, PayloadRevision: plan.PayloadRevision,
		Payload: plan.Payload, RequestedNotAfter: plan.RequestedNotAfter,
	})
	if err != nil {
		return ProviderPhaseResultV4{}, err
	}
	response, dispatchErr := b.dataplane.Dispatch(ctx, dispatch)
	if dispatchErr != nil {
		recovered, inspectErr := b.dataplane.Inspect(context.WithoutCancel(ctx), dispatch)
		if inspectErr != nil {
			return ProviderPhaseResultV4{}, dispatchErr
		}
		response = recovered
	}
	if err := response.Validate(dispatch); err != nil || response.ProviderObservation == nil || response.ProviderReceipt == nil || !now.Before(time.Unix(0, response.ExpiresUnixNano)) || response.ExpiresUnixNano > current.ExpiresUnixNano || response.ExpiresUnixNano > qualification.ExpiresUnixNano || response.ExpiresUnixNano > handoff.NotAfterUnixNano {
		return ProviderPhaseResultV4{}, errors.New("Data Plane returned an invalid Provider observation")
	}

	candidate, err := evidenceCandidateV4(plan, qualification, dispatch, response)
	if err != nil {
		return ProviderPhaseResultV4{}, err
	}
	consumed, err := b.evidence.ConsumeOperationScopeEvidenceV3(ctx, runtimeports.ConsumeOperationScopeEvidenceRequestV3{
		ConsumptionID: plan.Evidence.ConsumptionID, Handoff: handoff.RefV3(), Candidate: candidate,
	})
	if err != nil {
		return ProviderPhaseResultV4{}, err
	}
	if err := consumed.Qualification.Validate(); err != nil || consumed.Consumption.Validate() != nil || consumed.Record.Validate() != nil || consumed.Source.Validate() != nil || consumed.Qualification.State != runtimeports.OperationScopeEvidenceConsumedCurrentV3 || consumed.Qualification.ID != qualification.ID || !reflect.DeepEqual(consumed.Qualification.Scope, scope) || consumed.Consumption.Handoff != handoff.RefV3() || consumed.Record.CandidateDigest != consumed.Consumption.CandidateDigest {
		return ProviderPhaseResultV4{}, errors.New("Evidence consume returned an incomplete closure")
	}
	scopeDigest, err := runtimeports.DigestOperationSettlementEvidenceScopeV4(scope)
	if err != nil {
		return ProviderPhaseResultV4{}, err
	}
	binding := runtimeports.OperationSettlementEvidenceBindingV4{
		Phase: current.Phase.Phase, Consumption: consumed.Consumption.RefV3(),
		IssuedQualification: qualification.RefV3(), FinalQualification: consumed.Qualification.RefV3(),
		Record: consumed.Record.Ref, CandidateDigest: consumed.Record.CandidateDigest,
		Handoff: handoff.RefV3(), Attempt: plan.Attempt, EnforcementPhase: current.Phase,
		OperationScopeDigest: scopeDigest,
	}
	if err := binding.Validate(); err != nil {
		return ProviderPhaseResultV4{}, err
	}
	return ProviderPhaseResultV4{Current: current, Dispatch: dispatch, Response: response, Qualification: qualification, Handoff: handoff, Consumption: consumed, Binding: binding}, nil
}

func validateEnforcementCurrentForRequestV4(current runtimeports.CurrentOperationDispatchEnforcementV4, request runtimeports.EnforceCurrentOperationDispatchRequestV4, now time.Time) error {
	if err := current.Validate(); err != nil {
		return err
	}
	operationDigest, err := request.Operation.DigestV3()
	if err != nil {
		return err
	}
	if now.IsZero() || !now.Before(time.Unix(0, current.ExpiresUnixNano)) ||
		current.Phase.Phase != request.Phase || current.Phase.OperationDigest != operationDigest ||
		current.Phase.EffectID != request.EffectID || current.Phase.PermitID != request.PermitID ||
		current.Phase.PermitFactRevision != request.ExpectedPermitFactRevision || current.Phase.PermitDigest != request.PermitDigest ||
		current.Phase.AdmissionDigest != request.AdmissionDigest || current.Phase.ReviewAuthorization != request.ReviewAuthorization ||
		current.Phase.AttemptID != request.AttemptID || current.Phase.SandboxAttempt != request.SandboxAttempt ||
		current.Sandbox.Reservation != request.SandboxReservation || current.Sandbox.ProjectionDigest != request.SandboxProjectionDigest ||
		current.Sandbox.ProviderBinding != request.Verifier {
		return errors.New("Runtime enforcement current does not bind the exact request")
	}
	return nil
}

func evidenceCandidateV4(plan ProviderPhasePlanV4, qualification runtimeports.OperationScopeEvidenceQualificationFactV3, dispatch dataplaneadapter.DispatchRequestV1, response dataplaneadapter.DispatchResponseV1) (runtimeports.OperationScopeEvidenceCandidateV3, error) {
	payload, err := json.Marshal(response.ProviderObservation)
	if err != nil {
		return runtimeports.OperationScopeEvidenceCandidateV3{}, err
	}
	if len(payload) == 0 {
		return runtimeports.OperationScopeEvidenceCandidateV3{}, errors.New("Provider observation payload is empty")
	}
	ref := fmt.Sprintf("praxis.sandbox.dataplane/journal/%s", dispatch.Digest)
	candidate := runtimeports.OperationScopeEvidenceCandidateV3{
		ContractVersion: runtimeports.OperationScopeEvidenceContractVersionV3,
		Qualification:   qualification.RefV3(), Source: plan.Evidence.Reservation.Source,
		EventID: plan.Evidence.Reservation.EventID, TrustClass: runtimeports.EvidenceTrustObservation,
		Payload:   runtimeports.EvidencePayloadRefV2{Schema: plan.Evidence.PayloadSchema, ContentDigest: runtimecore.Digest(*response.ObservationDigest), Revision: 1, Length: uint64(len(payload)), Ref: ref},
		Causation: []runtimeports.EvidenceCausationRefV2{}, CorrelationID: plan.Evidence.CorrelationID,
		ObservedUnixNano: response.ProviderObservation.ObservedUnixNano,
	}
	return candidate, candidate.Validate()
}

func validatePhasePlanV4(plan ProviderPhasePlanV4) error {
	if err := plan.Enforcement.Validate(); err != nil {
		return err
	}
	if plan.RequestID == "" || plan.EffectKind == "" || plan.PayloadSchema == "" || plan.PayloadRevision == 0 || plan.RequestedNotAfter.IsZero() || plan.Evidence.QualificationID == "" || plan.Evidence.HandoffID == "" || plan.Evidence.ConsumptionID == "" || plan.Evidence.RequestedTTL <= 0 || plan.Evidence.CorrelationID == "" {
		return errors.New("provider phase plan is incomplete")
	}
	if err := plan.Evidence.Scope.Validate(); err != nil {
		return err
	}
	if err := plan.Evidence.EvidencePolicy.Validate(); err != nil {
		return err
	}
	if err := plan.Evidence.Reservation.Validate(); err != nil {
		return err
	}
	if err := plan.Evidence.PayloadSchema.Validate(); err != nil {
		return err
	}
	if err := plan.Attempt.Validate(); err != nil {
		return err
	}
	if plan.Enforcement.Operation.Validate() != nil || !runtimeports.SameOperationSubjectV3(plan.Enforcement.Operation, plan.Evidence.Scope.Operation) ||
		plan.Enforcement.EffectID != plan.Evidence.Scope.EffectID || plan.Enforcement.AttemptID != plan.Evidence.Scope.AttemptID ||
		plan.Enforcement.EffectID != plan.Attempt.EffectID || plan.Enforcement.AttemptID != plan.Attempt.AttemptID ||
		plan.Enforcement.ExpectedPermitFactRevision != plan.Attempt.PermitRevision || plan.Enforcement.PermitID != plan.Attempt.PermitID || plan.Enforcement.PermitDigest != plan.Attempt.PermitDigest ||
		plan.Evidence.Scope.EffectKind != runtimeports.EffectKindV2(plan.EffectKind) || plan.Evidence.Scope.EffectRevision != plan.Attempt.IntentRevision || plan.Evidence.Scope.EffectDigest != plan.Attempt.IntentDigest ||
		plan.Evidence.Reservation.Schema != plan.Evidence.PayloadSchema {
		return errors.New("provider phase plan combines different Operation attempts")
	}
	return nil
}

func nilLike(value any) bool {
	if value == nil {
		return true
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return rv.IsNil()
	default:
		return false
	}
}
