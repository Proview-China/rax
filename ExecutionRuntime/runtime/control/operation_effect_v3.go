package control

import (
	"context"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type OperationEffectStateV3 string

const (
	OperationEffectProposedV3       OperationEffectStateV3 = "proposed"
	OperationEffectAcceptedV3       OperationEffectStateV3 = "accepted"
	OperationEffectDispatchIntentV3 OperationEffectStateV3 = "dispatch_intent"
	OperationEffectDispatchedV3     OperationEffectStateV3 = "dispatched"
	OperationEffectUnknownOutcomeV3 OperationEffectStateV3 = "unknown_outcome"
	OperationEffectSettledV3        OperationEffectStateV3 = "settled"
	OperationEffectRejectedV3       OperationEffectStateV3 = "rejected"
)

type OperationEffectRefV3 struct {
	OperationDigest core.Digest         `json:"operation_digest"`
	EffectID        core.EffectIntentID `json:"effect_id"`
	IntentRevision  core.Revision       `json:"intent_revision"`
	IntentDigest    core.Digest         `json:"intent_digest"`
	FactRevision    core.Revision       `json:"fact_revision"`
}

func (r OperationEffectRefV3) Validate() error {
	if strings.TrimSpace(string(r.EffectID)) == "" || r.IntentRevision == 0 || r.FactRevision == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonEffectIntentMissing, "operation Effect ref is incomplete")
	}
	for _, digest := range []core.Digest{r.OperationDigest, r.IntentDigest} {
		if err := digest.Validate(); err != nil {
			return err
		}
	}
	return nil
}

type OperationProviderDispatchReceiptV3 struct {
	PermitID             string                                   `json:"permit_id"`
	PermitRevision       core.Revision                            `json:"permit_revision"`
	PermitDigest         core.Digest                              `json:"permit_digest"`
	AttemptID            string                                   `json:"attempt_id"`
	IntentID             core.EffectIntentID                      `json:"intent_id"`
	IntentRevision       core.Revision                            `json:"intent_revision"`
	IntentDigest         core.Digest                              `json:"intent_digest"`
	OperationDigest      core.Digest                              `json:"operation_digest"`
	Provider             ports.ProviderBindingRefV2               `json:"provider_binding"`
	PayloadSchema        ports.SchemaRefV2                        `json:"payload_schema"`
	PayloadDigest        core.Digest                              `json:"payload_digest"`
	PayloadRevision      core.Revision                            `json:"payload_revision"`
	Delegation           ports.ExecutionDelegationRefV2           `json:"delegation"`
	Prepared             ports.PreparedProviderAttemptRefV2       `json:"prepared_attempt"`
	Enforcement          ports.PersistedOperationEnforcementRefV3 `json:"persisted_enforcement"`
	Observation          ports.ProviderAttemptObservationRefV2    `json:"provider_observation"`
	ProviderOperationRef string                                   `json:"provider_operation_ref"`
	ObservationDigest    core.Digest                              `json:"observation_digest"`
	ObservedUnixNano     int64                                    `json:"observed_unix_nano"`
}

func (r OperationProviderDispatchReceiptV3) Validate() error {
	if strings.TrimSpace(r.PermitID) == "" || r.PermitRevision == 0 || strings.TrimSpace(r.AttemptID) == "" || strings.TrimSpace(string(r.IntentID)) == "" || r.IntentRevision == 0 || r.PayloadRevision == 0 || strings.TrimSpace(r.ProviderOperationRef) == "" || r.ObservedUnixNano <= 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonDispatchPermitInvalid, "operation provider receipt identity is incomplete")
	}
	for _, digest := range []core.Digest{r.PermitDigest, r.IntentDigest, r.OperationDigest, r.PayloadDigest, r.ObservationDigest} {
		if err := digest.Validate(); err != nil {
			return err
		}
	}
	if err := r.Provider.Validate(); err != nil {
		return err
	}
	if err := r.PayloadSchema.Validate(); err != nil {
		return err
	}
	if err := r.Delegation.Validate(); err != nil {
		return err
	}
	if err := r.Prepared.Validate(); err != nil {
		return err
	}
	if err := r.Enforcement.Validate(); err != nil {
		return err
	}
	if err := r.Observation.Validate(); err != nil {
		return err
	}
	if r.Observation.Digest != r.ObservationDigest || r.Observation.Delegation != r.Delegation || r.Observation.PreparedAttemptID != r.Prepared.ID || r.Observation.ProviderOperationRef != r.ProviderOperationRef || r.Observation.ObservedUnixNano != r.ObservedUnixNano || r.Prepared.PermitID != r.PermitID || r.Prepared.PermitRevision != r.PermitRevision || r.Prepared.PermitDigest != r.PermitDigest || r.Prepared.AttemptID != r.AttemptID || r.Prepared.IntentID != r.IntentID || r.Prepared.IntentRevision != r.IntentRevision || r.Prepared.IntentDigest != r.IntentDigest || r.Prepared.OperationDigest != r.OperationDigest || r.Prepared.Provider != r.Provider || r.Prepared.PayloadSchema != r.PayloadSchema || r.Prepared.PayloadDigest != r.PayloadDigest || r.Prepared.PayloadRevision != r.PayloadRevision || r.Enforcement.PermitID != r.PermitID || r.Enforcement.PermitRevision != r.PermitRevision || r.Enforcement.PermitDigest != r.PermitDigest || r.Enforcement.AttemptID != r.AttemptID || r.Enforcement.OperationDigest != r.OperationDigest || r.Enforcement.Provider != r.Provider || r.ObservedUnixNano < r.Prepared.PreparedUnixNano {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "provider receipt does not bind one exact prepared/enforced observation")
	}
	return nil
}

type OperationSettlementFactV3 struct {
	ID                   string                                    `json:"id"`
	Revision             core.Revision                             `json:"revision"`
	Owner                ports.EffectOwnerRefV2                    `json:"owner"`
	Attempt              ports.OperationDispatchAttemptRefV3       `json:"attempt"`
	Observation          *ports.ProviderAttemptObservationRefV2    `json:"observation,omitempty"`
	Disposition          EffectSettlementDispositionV2             `json:"disposition"`
	Evidence             []ports.EvidenceRecordRefV2               `json:"evidence"`
	EvidenceDigest       core.Digest                               `json:"evidence_digest"`
	InspectionEffect     *ports.OperationDispatchAttemptRefV3      `json:"inspection_effect,omitempty"`
	InspectionSettlement *ports.OperationInspectionSettlementRefV3 `json:"inspection_settlement,omitempty"`
	DomainResult         *ports.OpaquePayloadV2                    `json:"domain_result,omitempty"`
	SettledUnixNano      int64                                     `json:"settled_unix_nano"`
}

func (f OperationSettlementFactV3) Validate() error {
	if strings.TrimSpace(f.ID) == "" || f.Revision != 1 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonSettlementOwnerMismatch, "operation settlement fact identity is incomplete")
	}
	if err := validateEffectOwnerRefV3(f.Owner); err != nil {
		return err
	}
	if f.Owner.Role != ports.OwnerSettlement {
		return core.NewError(core.ErrorForbidden, core.ReasonSettlementOwnerMismatch, "only the bound Settlement Owner may settle an operation Effect")
	}
	if err := f.Attempt.Validate(); err != nil {
		return err
	}
	if f.Observation != nil {
		if err := f.Observation.Validate(); err != nil {
			return err
		}
		if f.Attempt.Delegation == nil || *f.Attempt.Delegation != f.Observation.Delegation {
			return core.NewError(core.ErrorConflict, core.ReasonSettlementOwnerMismatch, "settlement observation belongs to another attempt")
		}
	}
	switch f.Disposition {
	case SettlementConfirmedApplied, SettlementConfirmedNotApplied, SettlementConfirmedFailed:
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonSettlementOwnerMismatch, "operation settlement disposition is invalid")
	}
	if len(f.Evidence) == 0 || len(f.Evidence) > 64 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceUnavailable, "operation settlement requires bounded exact evidence")
	}
	for _, evidence := range f.Evidence {
		if err := evidence.Validate(); err != nil {
			return err
		}
	}
	evidenceDigest, err := core.CanonicalJSONDigest("praxis.runtime.operation-effect", ports.OperationEffectContractVersionV3, "OperationSettlementEvidenceV3", f.Evidence)
	if err != nil {
		return err
	}
	if evidenceDigest != f.EvidenceDigest {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "operation settlement evidence digest drifted")
	}
	if (f.InspectionEffect == nil) != (f.InspectionSettlement == nil) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectUnknownOutcome, "operation settlement inspection provenance must be complete")
	}
	if f.InspectionEffect != nil {
		if err := f.InspectionEffect.Validate(); err != nil {
			return err
		}
		if err := f.InspectionSettlement.Validate(); err != nil {
			return err
		}
		if !sameOperationDispatchAttemptV3(*f.InspectionEffect, f.InspectionSettlement.Attempt) {
			return core.NewError(core.ErrorConflict, core.ReasonSettlementOwnerMismatch, "operation settlement inspection provenance drifted")
		}
	}
	if f.DomainResult != nil {
		if err := f.DomainResult.Validate(); err != nil {
			return err
		}
	}
	if f.SettledUnixNano <= 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonSettlementOwnerMismatch, "operation settlement time is required")
	}
	return nil
}

func (f OperationSettlementFactV3) RefV3() (ports.OperationSettlementRefV3, error) {
	if err := f.Validate(); err != nil {
		return ports.OperationSettlementRefV3{}, err
	}
	digest, err := core.CanonicalJSONDigest("praxis.runtime.operation-effect", ports.OperationEffectContractVersionV3, "OperationSettlementFactV3", f)
	if err != nil {
		return ports.OperationSettlementRefV3{}, err
	}
	disposition := ports.OperationSettlementAppliedV3
	switch f.Disposition {
	case SettlementConfirmedNotApplied:
		disposition = ports.OperationSettlementNotAppliedV3
	case SettlementConfirmedFailed:
		disposition = ports.OperationSettlementFailedV3
	}
	domainResultDigest := core.Digest("")
	var domainResultSchema *ports.SchemaRefV2
	if f.DomainResult != nil {
		domainResultDigest = f.DomainResult.ContentDigest
		schema := f.DomainResult.Schema
		domainResultSchema = &schema
	}
	var observation *ports.ProviderAttemptObservationRefV2
	if f.Observation != nil {
		cloned := *f.Observation
		observation = &cloned
	}
	var inspectionEffect *ports.OperationDispatchAttemptRefV3
	var inspectionSettlement *ports.OperationInspectionSettlementRefV3
	if f.InspectionEffect != nil {
		clonedEffect := *f.InspectionEffect
		clonedSettlement := *f.InspectionSettlement
		inspectionEffect = &clonedEffect
		inspectionSettlement = &clonedSettlement
	}
	return ports.OperationSettlementRefV3{ID: f.ID, Revision: f.Revision, Digest: digest, Attempt: f.Attempt, Disposition: disposition, Owner: f.Owner, Observation: observation, InspectionEffect: inspectionEffect, InspectionSettlement: inspectionSettlement, Evidence: append([]ports.EvidenceRecordRefV2{}, f.Evidence...), DomainResultSchema: domainResultSchema, DomainResultDigest: domainResultDigest}, nil
}

func validateEffectOwnerRefV3(owner ports.EffectOwnerRefV2) error {
	if owner.Role != ports.OwnerEffect && owner.Role != ports.OwnerSettlement && owner.Role != ports.OwnerCleanup {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonOwnerMissing, "operation Effect owner role is invalid")
	}
	if err := ports.ValidateNamespacedNameV2(ports.NamespacedNameV2(owner.ComponentID)); err != nil {
		return err
	}
	return owner.ManifestDigest.Validate()
}

type OperationEffectFactV3 struct {
	Intent               ports.OperationEffectIntentV3       `json:"intent"`
	IntentDigest         core.Digest                         `json:"intent_digest"`
	State                OperationEffectStateV3              `json:"state"`
	Revision             core.Revision                       `json:"revision"`
	DispatchPermitID     string                              `json:"dispatch_permit_id,omitempty"`
	DispatchPermitDigest core.Digest                         `json:"dispatch_permit_digest,omitempty"`
	DispatchReceipt      *OperationProviderDispatchReceiptV3 `json:"dispatch_receipt,omitempty"`
	Settlement           *OperationSettlementFactV3          `json:"settlement,omitempty"`
	CreatedUnixNano      int64                               `json:"created_unix_nano"`
	UpdatedUnixNano      int64                               `json:"updated_unix_nano"`
}

func NewProposedOperationEffectFactV3(intent ports.OperationEffectIntentV3, now time.Time) (OperationEffectFactV3, error) {
	if now.IsZero() {
		return OperationEffectFactV3{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "operation Effect clock is zero")
	}
	digest, err := intent.DigestV3()
	if err != nil {
		return OperationEffectFactV3{}, err
	}
	fact := OperationEffectFactV3{Intent: intent, IntentDigest: digest, State: OperationEffectProposedV3, Revision: 1, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()}
	return fact, fact.Validate()
}

func (f OperationEffectFactV3) Validate() error {
	if err := f.Intent.Validate(); err != nil {
		return err
	}
	digest, err := f.Intent.DigestV3()
	if err != nil || digest != f.IntentDigest || f.Revision == 0 || f.CreatedUnixNano <= 0 || f.UpdatedUnixNano < f.CreatedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectStateConflict, "operation Effect fact identity or intent digest drifted")
	}
	switch f.State {
	case OperationEffectProposedV3, OperationEffectAcceptedV3:
		if f.DispatchPermitID != "" || f.DispatchPermitDigest != "" || f.DispatchReceipt != nil || f.Settlement != nil {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectStateConflict, "pre-dispatch operation Effect carries later facts")
		}
	case OperationEffectDispatchIntentV3, OperationEffectUnknownOutcomeV3:
		if f.DispatchPermitID == "" || f.DispatchPermitDigest.Validate() != nil || f.Settlement != nil {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonDispatchPermitInvalid, "operation Effect dispatch intent lacks exact Permit")
		}
	case OperationEffectDispatchedV3:
		if f.DispatchPermitID == "" || f.DispatchPermitDigest.Validate() != nil || f.DispatchReceipt == nil || f.DispatchReceipt.Validate() != nil || f.Settlement != nil {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectUnknownOutcome, "dispatched operation Effect lacks exact provider receipt")
		}
	case OperationEffectSettledV3:
		if f.DispatchPermitID == "" || f.DispatchPermitDigest.Validate() != nil || f.Settlement == nil || f.Settlement.Validate() != nil {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonSettlementOwnerMismatch, "settled operation Effect lacks exact settlement")
		}
	case OperationEffectRejectedV3:
		if f.DispatchReceipt != nil || f.Settlement != nil {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectStateConflict, "rejected operation Effect cannot claim dispatch or settlement")
		}
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonEffectStateConflict, "operation Effect state is unknown")
	}
	return nil
}

func (f OperationEffectFactV3) RefV3() (OperationEffectRefV3, error) {
	if err := f.Validate(); err != nil {
		return OperationEffectRefV3{}, err
	}
	operationDigest, _ := f.Intent.Operation.DigestV3()
	return OperationEffectRefV3{OperationDigest: operationDigest, EffectID: f.Intent.ID, IntentRevision: f.Intent.Revision, IntentDigest: f.IntentDigest, FactRevision: f.Revision}, nil
}

type OperationEffectTransitionContextV3 struct {
	PermitBegun              bool `json:"permit_begun"`
	PreDispatchRejectionSafe bool `json:"pre_dispatch_rejection_safe"`
	DispatchReceiptMatched   bool `json:"dispatch_receipt_matched"`
	SettlementOwnerMatched   bool `json:"settlement_owner_matched"`
	UnknownInspectionSettled bool `json:"unknown_inspection_settled"`
}

func ValidateOperationEffectTransitionV3(current, next OperationEffectFactV3, context OperationEffectTransitionContextV3, now time.Time) error {
	if err := current.Validate(); err != nil {
		return err
	}
	if err := next.Validate(); err != nil {
		return err
	}
	if now.IsZero() || next.UpdatedUnixNano != now.UnixNano() || next.UpdatedUnixNano < current.UpdatedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "operation Effect transition requires monotonic injected clock")
	}
	if next.Revision != current.Revision+1 || next.IntentDigest != current.IntentDigest || next.Intent.ID != current.Intent.ID || next.Intent.Revision != current.Intent.Revision || next.CreatedUnixNano != current.CreatedUnixNano {
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "operation Effect transition changed immutable identity or skipped a revision")
	}
	if current.State == OperationEffectDispatchIntentV3 || current.State == OperationEffectDispatchedV3 || current.State == OperationEffectUnknownOutcomeV3 {
		if next.DispatchPermitID != current.DispatchPermitID || next.DispatchPermitDigest != current.DispatchPermitDigest {
			return core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "operation Effect transition changed its dispatch watermark")
		}
		if current.DispatchReceipt != nil && (next.DispatchReceipt == nil || *next.DispatchReceipt != *current.DispatchReceipt) {
			return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "operation Effect settlement discarded or changed provider receipt")
		}
	}
	allowed := false
	switch current.State {
	case OperationEffectProposedV3:
		allowed = next.State == OperationEffectAcceptedV3 || next.State == OperationEffectRejectedV3
	case OperationEffectAcceptedV3:
		allowed = next.State == OperationEffectDispatchIntentV3 || next.State == OperationEffectRejectedV3
	case OperationEffectDispatchIntentV3:
		allowed = next.State == OperationEffectUnknownOutcomeV3 || next.State == OperationEffectRejectedV3 && context.PreDispatchRejectionSafe && !context.PermitBegun || next.State == OperationEffectDispatchedV3 && context.PermitBegun && context.DispatchReceiptMatched
	case OperationEffectDispatchedV3:
		allowed = next.State == OperationEffectSettledV3 && context.SettlementOwnerMatched
	case OperationEffectUnknownOutcomeV3:
		allowed = next.State == OperationEffectSettledV3 && context.SettlementOwnerMatched && context.UnknownInspectionSettled
	}
	if !allowed {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidTransition, "operation Effect transition is not authorized")
	}
	return nil
}

type OperationDispatchPermitStateV3 string

const (
	OperationPermitIssuedV3  OperationDispatchPermitStateV3 = "issued"
	OperationPermitBegunV3   OperationDispatchPermitStateV3 = "begun"
	OperationPermitExpiredV3 OperationDispatchPermitStateV3 = "expired"
	OperationPermitRevokedV3 OperationDispatchPermitStateV3 = "revoked"
)

type OperationDispatchPermitFactV3 struct {
	Permit             ports.OperationDispatchPermitV3      `json:"permit"`
	PermitDigest       core.Digest                          `json:"permit_digest"`
	Fence              core.ExecutionFence                  `json:"fence"`
	State              OperationDispatchPermitStateV3       `json:"state"`
	Revision           core.Revision                        `json:"revision"`
	EffectFactRevision core.Revision                        `json:"effect_fact_revision"`
	BegunUnixNano      int64                                `json:"begun_unix_nano,omitempty"`
	Enforcement        *ports.OperationEnforcementReceiptV3 `json:"enforcement,omitempty"`
}

func (f OperationDispatchPermitFactV3) Validate() error {
	if err := f.Permit.Validate(); err != nil {
		return err
	}
	digest, err := f.Permit.DigestV3()
	if err != nil || digest != f.PermitDigest || f.Revision == 0 || f.EffectFactRevision == 0 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonDispatchPermitInvalid, "operation Permit fact digest or revision drifted")
	}
	fenceDigest, err := ports.DigestOperationExecutionFenceV3(f.Fence, f.Permit.Operation)
	if err != nil || fenceDigest != f.Permit.FenceDigest || f.Fence.EffectIntentID != f.Permit.IntentID || f.Fence.EffectIntentRevision != f.Permit.IntentRevision || f.Fence.CanonicalPayloadDigest != f.Permit.PayloadDigest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectFenceStale, "operation Permit Fence drifted")
	}
	switch f.State {
	case OperationPermitIssuedV3:
		if f.BegunUnixNano != 0 || f.Enforcement != nil {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonDispatchPermitInvalid, "issued operation Permit carries begin/enforcement facts")
		}
	case OperationPermitBegunV3:
		if f.BegunUnixNano <= 0 {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonDispatchPermitInvalid, "begun operation Permit lacks begin time")
		}
		if f.Enforcement != nil {
			if err := f.Enforcement.Validate(); err != nil {
				return err
			}
			if f.Enforcement.PermitID != f.Permit.ID || f.Enforcement.PermitRevision != f.Permit.Revision || f.Enforcement.AttemptID != f.Permit.AttemptID || f.Enforcement.PermitDigest != f.PermitDigest || !ports.SameOperationSubjectV3(f.Enforcement.Operation, f.Permit.Operation) || f.Enforcement.Verifier != f.Permit.EnforcementPoint || f.Enforcement.ValidatedUnixNano < f.BegunUnixNano {
				return core.NewError(core.ErrorPreconditionFailed, core.ReasonDispatchPermitInvalid, "operation Enforcement does not bind exact begun Permit/provider")
			}
		}
	case OperationPermitExpiredV3, OperationPermitRevokedV3:
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonDispatchPermitInvalid, "operation Permit state is unknown")
	}
	return nil
}

func (f OperationDispatchPermitFactV3) PersistedEnforcementRefV3() (ports.PersistedOperationEnforcementRefV3, error) {
	if err := f.Validate(); err != nil {
		return ports.PersistedOperationEnforcementRefV3{}, err
	}
	if f.State != OperationPermitBegunV3 || f.Enforcement == nil {
		return ports.PersistedOperationEnforcementRefV3{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonDispatchPermitInvalid, "operation Enforcement is not yet persisted")
	}
	digest, err := core.CanonicalJSONDigest("praxis.runtime.operation-effect", ports.OperationEffectContractVersionV3, "OperationEnforcementReceiptV3", f.Enforcement)
	if err != nil {
		return ports.PersistedOperationEnforcementRefV3{}, err
	}
	operationDigest, _ := f.Permit.Operation.DigestV3()
	return ports.PersistedOperationEnforcementRefV3{
		PermitID:         f.Permit.ID,
		PermitRevision:   f.Permit.Revision,
		PermitDigest:     f.PermitDigest,
		AttemptID:        f.Permit.AttemptID,
		OperationDigest:  operationDigest,
		Provider:         f.Permit.EnforcementPoint,
		ReceiptDigest:    digest,
		RecordedRevision: f.Revision,
	}, nil
}

type OperationEffectCASRequestV3 struct {
	ExpectedRevision core.Revision         `json:"expected_revision"`
	Next             OperationEffectFactV3 `json:"next"`
}

type IssueOperationPermitRequestV3 struct {
	Operation              ports.OperationSubjectV3        `json:"operation_subject"`
	EffectID               core.EffectIntentID             `json:"effect_id"`
	ExpectedEffectRevision core.Revision                   `json:"expected_effect_revision"`
	Permit                 ports.OperationDispatchPermitV3 `json:"permit"`
	Fence                  core.ExecutionFence             `json:"fence"`
}

type IssueOperationPermitResultV3 struct {
	Effect OperationEffectFactV3         `json:"effect"`
	Permit OperationDispatchPermitFactV3 `json:"permit"`
}

type BeginOperationDispatchRequestV3 struct {
	Operation              ports.OperationSubjectV3 `json:"operation_subject"`
	EffectID               core.EffectIntentID      `json:"effect_id"`
	ExpectedEffectRevision core.Revision            `json:"expected_effect_revision"`
	PermitID               string                   `json:"permit_id"`
	ExpectedPermitRevision core.Revision            `json:"expected_permit_revision"`
}

type RecordOperationEnforcementRequestV3 struct {
	Operation              ports.OperationSubjectV3            `json:"operation_subject"`
	PermitID               string                              `json:"permit_id"`
	ExpectedPermitRevision core.Revision                       `json:"expected_permit_revision"`
	Receipt                ports.OperationEnforcementReceiptV3 `json:"receipt"`
}

type OperationEffectFactPortV3 interface {
	CreateOperationEffectV3(context.Context, OperationEffectFactV3) (OperationEffectFactV3, error)
	InspectOperationEffectV3(context.Context, ports.OperationSubjectV3, core.EffectIntentID) (OperationEffectFactV3, error)
	CompareAndSwapOperationEffectV3(context.Context, ports.OperationSubjectV3, OperationEffectCASRequestV3) (OperationEffectFactV3, error)
	IssueOperationDispatchPermitV3(context.Context, IssueOperationPermitRequestV3) (IssueOperationPermitResultV3, error)
	InspectOperationDispatchPermitV3(context.Context, ports.OperationSubjectV3, string) (OperationDispatchPermitFactV3, error)
	BeginOperationDispatchV3(context.Context, BeginOperationDispatchRequestV3) (OperationDispatchPermitFactV3, error)
	RecordOperationEnforcementV3(context.Context, RecordOperationEnforcementRequestV3) (OperationDispatchPermitFactV3, error)
}
