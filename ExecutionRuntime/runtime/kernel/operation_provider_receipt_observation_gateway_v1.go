package kernel

import (
	"context"
	"reflect"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// OperationProviderReceiptObservationGatewayV1 converts one exact
// provider/domain receipt into governed Evidence and then into Runtime's
// formal Provider Observation. It never invokes or re-invokes a Provider.
type OperationProviderReceiptObservationGatewayV1 struct {
	Receipts     ports.OperationProviderReceiptReaderV1
	Sources      ports.EvidenceSourceRegistrationReaderV1
	Evidence     ports.EvidenceGovernancePortV2
	Observations ports.OperationObservationGovernancePortV3
	Clock        func() time.Time
}

func NewOperationProviderReceiptObservationGatewayV1(
	receipts ports.OperationProviderReceiptReaderV1,
	sources ports.EvidenceSourceRegistrationReaderV1,
	evidence ports.EvidenceGovernancePortV2,
	observations ports.OperationObservationGovernancePortV3,
	clock func() time.Time,
) (*OperationProviderReceiptObservationGatewayV1, error) {
	if nilOperationReceiptDependencyV1(receipts) || nilOperationReceiptDependencyV1(sources) || nilOperationReceiptDependencyV1(evidence) || nilOperationReceiptDependencyV1(observations) || clock == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "operation Provider receipt observation dependencies are incomplete")
	}
	return &OperationProviderReceiptObservationGatewayV1{Receipts: receipts, Sources: sources, Evidence: evidence, Observations: observations, Clock: clock}, nil
}

func (g *OperationProviderReceiptObservationGatewayV1) RecordOperationProviderReceiptObservationV1(ctx context.Context, request ports.OperationProviderReceiptObservationRequestV1) (ports.ProviderAttemptObservationRefV2, error) {
	if err := operationReceiptContextErrorV1(ctx); err != nil {
		return ports.ProviderAttemptObservationRefV2{}, err
	}
	if g == nil || nilOperationReceiptDependencyV1(g.Receipts) || nilOperationReceiptDependencyV1(g.Sources) || nilOperationReceiptDependencyV1(g.Evidence) || nilOperationReceiptDependencyV1(g.Observations) || g.Clock == nil {
		return ports.ProviderAttemptObservationRefV2{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "operation Provider receipt observation gateway is unavailable")
	}
	if err := request.Validate(); err != nil {
		return ports.ProviderAttemptObservationRefV2{}, err
	}
	receipt, err := g.Receipts.InspectOperationProviderReceiptV1(ctx, request.Receipt)
	if err != nil {
		return ports.ProviderAttemptObservationRefV2{}, err
	}
	now := g.Clock()
	if err := receipt.ValidateExact(request.Receipt, now); err != nil {
		return ports.ProviderAttemptObservationRefV2{}, err
	}
	if err := validateOperationReceiptAttemptV1(receipt, request); err != nil {
		return ports.ProviderAttemptObservationRefV2{}, err
	}
	source, err := g.Sources.InspectSource(ctx, request.SourceRegistration.RegistrationID)
	if err != nil {
		return ports.ProviderAttemptObservationRefV2{}, err
	}
	if err := validateOperationReceiptSourceV1(source, request); err != nil {
		return ports.ProviderAttemptObservationRefV2{}, err
	}
	candidate, key, err := buildOperationReceiptEvidenceCandidateV1(source, request.SourceRegistration, receipt)
	if err != nil {
		return ports.ProviderAttemptObservationRefV2{}, err
	}
	record, err := g.Evidence.InspectGovernedBySource(ctx, key)
	if err == nil {
		if err := validateOperationReceiptEvidenceRecordV1(record, candidate); err != nil {
			return ports.ProviderAttemptObservationRefV2{}, err
		}
	} else if !core.HasCategory(err, core.ErrorNotFound) {
		return ports.ProviderAttemptObservationRefV2{}, err
	} else {
		current, currentErr := control.NewEvidenceSourceRegistrationRefV1(source)
		if currentErr != nil {
			return ports.ProviderAttemptObservationRefV2{}, currentErr
		}
		if current != request.SourceRegistration || source.NextSourceSequence != 1 {
			return ports.ProviderAttemptObservationRefV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceSourceStale, "dedicated Provider receipt source is not at its exact first sequence")
		}
		record, err = g.Evidence.AppendGoverned(ctx, ports.EvidenceAppendRequestV2{Candidate: candidate, ExpectedSourceRevision: request.SourceRegistration.Revision})
		if err != nil {
			if !recoverableOperationReceiptErrorV1(err) {
				return ports.ProviderAttemptObservationRefV2{}, err
			}
			record, err = g.Evidence.InspectGovernedBySource(context.WithoutCancel(ctx), key)
			if err != nil {
				return ports.ProviderAttemptObservationRefV2{}, core.NewError(core.ErrorIndeterminate, core.ReasonEvidenceUnavailable, "cannot prove Provider receipt Evidence append")
			}
		}
		if err := validateOperationReceiptEvidenceRecordV1(record, candidate); err != nil {
			return ports.ProviderAttemptObservationRefV2{}, err
		}
	}

	observation := ports.ProviderAttemptObservationV2{
		ContractVersion: ports.ExecutionGovernanceContractVersionV2,
		Delegation:      request.Attempt.Delegation, Prepared: request.Attempt.Prepared,
		Revision: 1, State: ports.ProviderAttemptObservedV2,
		Payload: cloneOperationReceiptPayloadV1(receipt.Payload), PayloadRevision: receipt.PayloadRevision,
		ProviderOperationRef: receipt.ProviderOperationRef,
		SourceRegistrationID: key.RegistrationID, SourceEpoch: key.SourceEpoch, SourceSequence: key.SourceSequence,
		Evidence: record.Ref, ObservedUnixNano: receipt.ObservedUnixNano,
	}
	return g.Observations.RecordGovernedProviderObservationV3(ctx, ports.RecordGovernedProviderObservationRequestV2{
		Intent: request.Intent, Permit: request.Permit, Fence: request.Fence, Attempt: request.Attempt, Observation: observation,
	})
}

func (g *OperationProviderReceiptObservationGatewayV1) InspectOperationProviderReceiptObservationV1(ctx context.Context, delegation ports.ExecutionDelegationRefV2, preparedID string) (ports.ProviderAttemptObservationRefV2, error) {
	if err := operationReceiptContextErrorV1(ctx); err != nil {
		return ports.ProviderAttemptObservationRefV2{}, err
	}
	if g == nil || nilOperationReceiptDependencyV1(g.Observations) {
		return ports.ProviderAttemptObservationRefV2{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "operation Provider receipt Observation Inspect is unavailable")
	}
	return g.Observations.InspectGovernedProviderObservationV3(ctx, delegation, preparedID)
}

func validateOperationReceiptAttemptV1(receipt ports.OperationProviderReceiptProjectionV1, request ports.OperationProviderReceiptObservationRequestV1) error {
	attempt := request.Attempt
	if !ports.SameOperationSubjectV3(receipt.Operation, request.Intent.Operation) || receipt.OperationDigest != attempt.Admission.OperationDigest || receipt.Prepared != attempt.Prepared || !reflect.DeepEqual(receipt.Attempt, ports.OperationDispatchAttemptRefV3{OperationDigest: attempt.Admission.OperationDigest, EffectID: attempt.Admission.EffectID, IntentRevision: attempt.Admission.IntentRevision, IntentDigest: attempt.Admission.IntentDigest, PermitID: attempt.PermitID, PermitRevision: attempt.PermitRevision, PermitDigest: attempt.PermitDigest, AttemptID: attempt.AttemptID, Delegation: &attempt.Delegation}) || receipt.Provider != request.Permit.Provider {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Provider receipt belongs to another governed attempt")
	}
	return nil
}

func validateOperationReceiptSourceV1(source ports.EvidenceSourceRegistrationFactV2, request ports.OperationProviderReceiptObservationRequestV1) error {
	if err := source.Validate(); err != nil {
		return err
	}
	configuration, err := source.ConfigurationDigestV2()
	if err != nil {
		return err
	}
	expected := request.SourceRegistration
	producer := ports.EvidenceProducerBindingRefV2(request.Attempt.Prepared.Provider)
	if source.ID != expected.RegistrationID || source.SourceID != expected.SourceID || source.SourceEpoch != expected.SourceEpoch || configuration != expected.ConfigurationDigest || source.SourceID != ports.OperationProviderReceiptEvidenceSourceIDV1 || source.Producer != producer || !ports.SameExecutionScopeV2(source.ExecutionScope, request.Intent.Operation.ExecutionScope) || source.LedgerScope.Partition != ports.EvidencePartitionEffect || source.LedgerScope.RunID != request.Intent.Operation.RunID || source.LedgerScope.EffectID != request.Intent.ID {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceScopeConflict, "Provider receipt Evidence source does not bind the exact effect and Provider")
	}
	allowedKind, allowedClass := false, false
	for _, value := range source.AllowedKinds {
		allowedKind = allowedKind || value == ports.OperationProviderReceiptEvidenceKindV1
	}
	for _, value := range source.ClassMappings {
		allowedClass = allowedClass || value.Class == ports.OperationProviderReceiptEvidenceClassV1 && value.Trust == ports.EvidenceTrustObservation
	}
	if !allowedKind || !allowedClass {
		return core.NewError(core.ErrorForbidden, core.ReasonEvidenceTrustInvalid, "Provider receipt source does not allow the required observation class")
	}
	return nil
}

func buildOperationReceiptEvidenceCandidateV1(source ports.EvidenceSourceRegistrationFactV2, expected ports.EvidenceSourceRegistrationRefV1, receipt ports.OperationProviderReceiptProjectionV1) (ports.EvidenceEventCandidateV2, ports.EvidenceSourceKeyV2, error) {
	key := ports.EvidenceSourceKeyV2{RegistrationID: expected.RegistrationID, SourceEpoch: expected.SourceEpoch, SourceSequence: 1}
	ledgerDigest, err := source.LedgerScope.DigestV2()
	if err != nil {
		return ports.EvidenceEventCandidateV2{}, ports.EvidenceSourceKeyV2{}, err
	}
	candidate := ports.EvidenceEventCandidateV2{
		ContractVersion: ports.EvidenceContractVersionV2,
		LedgerScope:     source.LedgerScope, EventID: receipt.ProviderOperationRef,
		RegistrationID: expected.RegistrationID, RegistrationRevision: expected.Revision,
		SourceConfigurationDigest: expected.ConfigurationDigest, SourcePolicy: source.Policy,
		SourceID: expected.SourceID, SourceEpoch: expected.SourceEpoch, SourceSequence: 1,
		TrustClass: ports.EvidenceTrustObservation, EventKind: ports.OperationProviderReceiptEvidenceKindV1,
		CustomClass: ports.OperationProviderReceiptEvidenceClassV1, ExecutionScope: source.ExecutionScope,
		Payload:       ports.EvidencePayloadRefV2{Schema: receipt.Payload.Schema, ContentDigest: receipt.Payload.ContentDigest, Revision: receipt.PayloadRevision, Length: receipt.Payload.Length, Ref: receipt.Payload.Ref},
		Causation:     []ports.EvidenceCausationRefV2{{LedgerScopeDigest: ledgerDigest, EventID: receipt.Prepared.DeclaredDelegation.ID}},
		CorrelationID: receipt.Prepared.ID, Producer: source.Producer, Authority: source.Authority,
		ObservedUnixNano: receipt.ObservedUnixNano,
	}
	if err := candidate.Validate(); err != nil {
		return ports.EvidenceEventCandidateV2{}, ports.EvidenceSourceKeyV2{}, err
	}
	return candidate, key, nil
}

func validateOperationReceiptEvidenceRecordV1(record ports.EvidenceLedgerRecordV2, candidate ports.EvidenceEventCandidateV2) error {
	if err := control.ValidateEvidenceLedgerRecordV2(record); err != nil {
		return err
	}
	digest, err := candidate.DigestV2()
	if err != nil || record.CandidateDigest != digest || !reflect.DeepEqual(record.Candidate, candidate) {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Provider receipt Evidence record changed canonical content")
	}
	return nil
}

func cloneOperationReceiptPayloadV1(payload ports.OpaquePayloadV2) ports.OpaquePayloadV2 {
	payload.Inline = append([]byte(nil), payload.Inline...)
	return payload
}

func operationReceiptContextErrorV1(ctx context.Context) error {
	if ctx == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "operation Provider receipt context is nil")
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

func recoverableOperationReceiptErrorV1(err error) bool {
	return core.HasCategory(err, core.ErrorUnavailable) || core.HasCategory(err, core.ErrorIndeterminate)
}

func nilOperationReceiptDependencyV1(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}

var _ ports.OperationProviderReceiptObservationPortV1 = (*OperationProviderReceiptObservationGatewayV1)(nil)
