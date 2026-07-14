package ports

import (
	"context"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const (
	OperationDomainContractVersionV3            = "praxis.application.operation-domain/v3"
	MaxOperationDomainAdapterAuthorizationTTLV3 = 30 * time.Second
)

type OperationDomainAdapterAuthorizationStateV3 string

const (
	OperationDomainAdapterAuthorizedV3 OperationDomainAdapterAuthorizationStateV3 = "authorized"
	OperationDomainAdapterRevokedV3    OperationDomainAdapterAuthorizationStateV3 = "revoked"
	OperationDomainAdapterExpiredV3    OperationDomainAdapterAuthorizationStateV3 = "expired"
)

// OperationDomainAdapterAuthorizationV3 is a short-lived, host-owned
// projection of the current BindingSet. It is read-only to Application and is
// re-read both when an adapter is registered and whenever it is resolved.
type OperationDomainAdapterAuthorizationV3 struct {
	ContractVersion    string                                     `json:"contract_version"`
	Adapter            runtimeports.ProviderBindingRefV2          `json:"adapter"`
	Revision           core.Revision                              `json:"revision"`
	Digest             core.Digest                                `json:"digest"`
	State              OperationDomainAdapterAuthorizationStateV3 `json:"state"`
	IssuedUnixNano     int64                                      `json:"issued_unix_nano"`
	ExpiresUnixNano    int64                                      `json:"expires_unix_nano"`
	InvalidationReason core.ReasonCode                            `json:"invalidation_reason,omitempty"`
}

func (a OperationDomainAdapterAuthorizationV3) Validate() error {
	if err := a.validateShapeV3(); err != nil {
		return err
	}
	if err := a.Digest.Validate(); err != nil {
		return err
	}
	expected, err := a.ComputeContentDigestV3()
	if err != nil {
		return err
	}
	if a.Digest != expected {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "operation domain adapter authorization digest does not bind its exact content")
	}
	return nil
}

func (a OperationDomainAdapterAuthorizationV3) validateShapeV3() error {
	ttl := a.ExpiresUnixNano - a.IssuedUnixNano
	if a.ContractVersion != OperationDomainContractVersionV3 || a.Revision == 0 || a.IssuedUnixNano <= 0 || a.ExpiresUnixNano <= a.IssuedUnixNano || ttl <= 0 || time.Duration(ttl) > MaxOperationDomainAdapterAuthorizationTTLV3 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "operation domain adapter authorization requires a bounded identity, revision and short lifetime")
	}
	if err := a.Adapter.Validate(); err != nil {
		return err
	}
	switch a.State {
	case OperationDomainAdapterAuthorizedV3:
		if a.InvalidationReason != "" {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "active operation domain adapter authorization cannot carry an invalidation reason")
		}
	case OperationDomainAdapterRevokedV3, OperationDomainAdapterExpiredV3:
		if a.InvalidationReason == "" {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "inactive operation domain adapter authorization requires an invalidation reason")
		}
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "operation domain adapter authorization state is unknown")
	}
	return nil
}

// ComputeContentDigestV3 binds every authorization field except Digest itself.
func (a OperationDomainAdapterAuthorizationV3) ComputeContentDigestV3() (core.Digest, error) {
	if err := a.validateShapeV3(); err != nil {
		return "", err
	}
	a.Digest = ""
	return core.CanonicalJSONDigest("praxis.application.operation-domain", OperationDomainContractVersionV3, "OperationDomainAdapterAuthorizationV3", a)
}

func SealOperationDomainAdapterAuthorizationV3(a OperationDomainAdapterAuthorizationV3) (OperationDomainAdapterAuthorizationV3, error) {
	a.Digest = ""
	digest, err := a.ComputeContentDigestV3()
	if err != nil {
		return OperationDomainAdapterAuthorizationV3{}, err
	}
	a.Digest = digest
	return a, nil
}

func (a OperationDomainAdapterAuthorizationV3) ValidateCurrentFor(expected runtimeports.ProviderBindingRefV2, now time.Time) error {
	if err := a.Validate(); err != nil {
		return err
	}
	if a.Adapter != expected {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "operation domain adapter authorization differs from the requested BindingSet revision, manifest, artifact or capability")
	}
	if now.IsZero() || now.Before(time.Unix(0, a.IssuedUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "operation domain adapter authorization clock regressed")
	}
	if a.State == OperationDomainAdapterExpiredV3 || !now.Before(time.Unix(0, a.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "operation domain adapter authorization expired")
	}
	if a.State != OperationDomainAdapterAuthorizedV3 {
		return core.NewError(core.ErrorForbidden, core.ReasonBindingDrift, "operation domain adapter authorization was revoked")
	}
	return nil
}

// OperationDomainAdapterCurrentnessPortV3 is implemented by a trusted host
// projection over Runtime Binding facts. Component self-report is not enough.
type OperationDomainAdapterCurrentnessPortV3 interface {
	InspectOperationDomainAdapterCurrentV3(context.Context, runtimeports.ProviderBindingRefV2) (OperationDomainAdapterAuthorizationV3, error)
}

type OperationDomainStateV3 string

const (
	OperationDomainPreparedV3 OperationDomainStateV3 = "prepared"
	OperationDomainUnknownV3  OperationDomainStateV3 = "unknown"
	OperationDomainObservedV3 OperationDomainStateV3 = "observed"
	OperationDomainSettledV3  OperationDomainStateV3 = "settled"
)

// OperationDomainStateRefV3 is an opaque acknowledgement from the unique
// domain owner. Application compares its exact attempt/basis binding but never
// interprets domain result content.
type OperationDomainStateRefV3 struct {
	ContractVersion string                                 `json:"contract_version"`
	StepKind        runtimeports.NamespacedNameV2          `json:"step_kind"`
	Attempt         contract.GovernedOperationAttemptRefV3 `json:"attempt"`
	State           OperationDomainStateV3                 `json:"state"`
	Revision        core.Revision                          `json:"revision"`
	Digest          core.Digest                            `json:"digest"`
	BasisDigest     core.Digest                            `json:"basis_digest"`
}

func (r OperationDomainStateRefV3) Validate() error {
	if r.ContractVersion != OperationDomainContractVersionV3 || runtimeports.ValidateNamespacedNameV2(r.StepKind) != nil || r.Revision == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "operation domain state ref is incomplete")
	}
	if err := r.Attempt.Validate(); err != nil {
		return err
	}
	if r.StepKind != r.Attempt.StepKind {
		return core.NewError(core.ErrorConflict, core.ReasonComponentMismatch, "domain state StepKind differs from its Application attempt")
	}
	expected := map[OperationDomainStateV3]contract.GovernedOperationAttemptStateV3{OperationDomainPreparedV3: contract.OperationExecutionPreparedV3, OperationDomainUnknownV3: contract.OperationDispatchUnknownV3, OperationDomainObservedV3: contract.OperationProviderObservedV3, OperationDomainSettledV3: contract.OperationSettledV3}
	if r.Attempt.State != expected[r.State] {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidState, "domain state and Application attempt watermark differ")
	}
	if r.State != OperationDomainPreparedV3 && r.State != OperationDomainUnknownV3 && r.State != OperationDomainObservedV3 && r.State != OperationDomainSettledV3 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "operation domain state is unknown")
	}
	if err := r.Digest.Validate(); err != nil {
		return err
	}
	return r.BasisDigest.Validate()
}

type OperationDomainInspectRequestV3 struct {
	Scope     core.ExecutionScope           `json:"scope"`
	StepKind  runtimeports.NamespacedNameV2 `json:"step_kind"`
	AttemptID string                        `json:"attempt_id"`
}

// ReserveOperationIntentRequestV3 asks the unique domain owner to reserve its
// subject/session/candidate before Runtime admission. It grants no dispatch
// authority; the returned create-once reservation is only a causal barrier.
type ReserveOperationIntentRequestV3 struct {
	StepKind      runtimeports.NamespacedNameV2          `json:"step_kind"`
	Descriptor    contract.StepDescriptorRefV2           `json:"descriptor"`
	DomainAdapter runtimeports.ProviderBindingRefV2      `json:"domain_adapter"`
	Attempt       contract.GovernedOperationAttemptRefV3 `json:"attempt"`
	Intent        runtimeports.OperationEffectIntentV3   `json:"intent"`
}

func (r ReserveOperationIntentRequestV3) Validate() error {
	if runtimeports.ValidateNamespacedNameV2(r.StepKind) != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "domain reservation StepKind must be namespaced")
	}
	if err := r.Descriptor.Validate(r.StepKind); err != nil {
		return err
	}
	if err := r.DomainAdapter.Validate(); err != nil {
		return err
	}
	if err := r.Attempt.Validate(); err != nil {
		return err
	}
	if err := r.Intent.Validate(); err != nil {
		return err
	}
	operationDigest, err := r.Intent.Operation.DigestV3()
	if err != nil {
		return err
	}
	scopeDigest, err := runtimeports.ExecutionScopeDigestV2(r.Intent.Operation.ExecutionScope)
	if err != nil {
		return err
	}
	if r.Attempt.State != contract.OperationIntentRecordedV3 || r.Attempt.Revision != 1 || r.Attempt.DomainReservation != nil || r.StepKind != r.Attempt.StepKind || r.Descriptor != r.Attempt.Descriptor || r.DomainAdapter != r.Attempt.DomainAdapter || r.Intent.ID != r.Attempt.EffectID || operationDigest != r.Attempt.OperationDigest || scopeDigest != r.Attempt.ScopeDigest || r.Intent.Provider != r.Attempt.PlannedProvider || r.Intent.Authority != r.Attempt.PlanAuthority {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "domain reservation does not bind the revision-one Application attempt and exact Intent")
	}
	return nil
}

type InspectOperationIntentReservationRequestV3 struct {
	Scope         core.ExecutionScope               `json:"scope"`
	StepKind      runtimeports.NamespacedNameV2     `json:"step_kind"`
	DomainAdapter runtimeports.ProviderBindingRefV2 `json:"domain_adapter"`
	AttemptID     string                            `json:"attempt_id"`
}

func (r InspectOperationIntentReservationRequestV3) Validate() error {
	if err := r.Scope.Validate(); err != nil {
		return err
	}
	if runtimeports.ValidateNamespacedNameV2(r.StepKind) != nil || strings.TrimSpace(r.AttemptID) == "" || len(r.AttemptID) > 512 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "domain reservation inspection key is incomplete")
	}
	return r.DomainAdapter.Validate()
}

func ValidateOperationDomainReservationForV3(reservation contract.OperationDomainReservationRefV3, request ReserveOperationIntentRequestV3) error {
	if err := request.Validate(); err != nil {
		return err
	}
	if err := reservation.Validate(); err != nil {
		return err
	}
	intentDigest, err := request.Intent.DigestV3()
	if err != nil {
		return err
	}
	if reservation.StepKind != request.StepKind || reservation.Descriptor != request.Descriptor || reservation.DomainAdapter != request.DomainAdapter || reservation.AttemptID != request.Attempt.ID || reservation.AttemptRevision != request.Attempt.Revision || reservation.AttemptDigest != request.Attempt.Digest || reservation.IntentDigest != intentDigest {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "domain reservation owner returned another attempt, Intent or routing binding")
	}
	return nil
}

func (r OperationDomainInspectRequestV3) Validate() error {
	if err := r.Scope.Validate(); err != nil {
		return err
	}
	if runtimeports.ValidateNamespacedNameV2(r.StepKind) != nil || strings.TrimSpace(r.AttemptID) == "" || len(r.AttemptID) > 512 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "domain inspection scope, step kind and attempt are required")
	}
	return nil
}

type BindPreparedOperationRequestV3 struct {
	StepKind       runtimeports.NamespacedNameV2                    `json:"step_kind"`
	Attempt        contract.GovernedOperationAttemptRefV3           `json:"attempt"`
	Intent         runtimeports.OperationEffectIntentV3             `json:"intent"`
	RuntimeAttempt runtimeports.GovernedExecutionAttemptRefsV2      `json:"runtime_attempt"`
	DelegationFact runtimeports.ExecutionDelegationFactV2           `json:"delegation_fact"`
	Prepared       runtimeports.PreparedExecutionGovernanceResultV2 `json:"prepared"`
}

type MarkUnknownOperationRequestV3 struct {
	StepKind       runtimeports.NamespacedNameV2                 `json:"step_kind"`
	Attempt        contract.GovernedOperationAttemptRefV3        `json:"attempt"`
	Intent         runtimeports.OperationEffectIntentV3          `json:"intent"`
	RuntimeAttempt runtimeports.GovernedExecutionAttemptRefsV2   `json:"runtime_attempt"`
	Authorization  runtimeports.OperationDispatchAuthorizationV3 `json:"authorization"`
}

type BindObservedOperationRequestV3 struct {
	StepKind       runtimeports.NamespacedNameV2                `json:"step_kind"`
	Attempt        contract.GovernedOperationAttemptRefV3       `json:"attempt"`
	Intent         runtimeports.OperationEffectIntentV3         `json:"intent"`
	RuntimeAttempt runtimeports.GovernedExecutionAttemptRefsV2  `json:"runtime_attempt"`
	Observation    runtimeports.ProviderAttemptObservationRefV2 `json:"observation"`
}

type ApplyOperationSettlementRequestV3 struct {
	StepKind       runtimeports.NamespacedNameV2                `json:"step_kind"`
	Attempt        contract.GovernedOperationAttemptRefV3       `json:"attempt"`
	Intent         runtimeports.OperationEffectIntentV3         `json:"intent"`
	RuntimeAttempt *runtimeports.GovernedExecutionAttemptRefsV2 `json:"runtime_attempt,omitempty"`
	Settlement     runtimeports.OperationSettlementRefV3        `json:"settlement"`
	DomainResult   *runtimeports.OpaquePayloadV2                `json:"domain_result,omitempty"`
}

func (r BindPreparedOperationRequestV3) Validate() error {
	if err := validateOperationDomainRequestV3(r.StepKind, r.Attempt, r.Intent, r.RuntimeAttempt); err != nil {
		return err
	}
	if r.Attempt.State != contract.OperationExecutionPreparedV3 || r.RuntimeAttempt.Observation != nil || r.RuntimeAttempt.Settlement != nil {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "BindPrepared requires the exact prepared Application and Runtime watermarks")
	}
	if err := r.Prepared.Validate(); err != nil {
		return err
	}
	if err := r.DelegationFact.Validate(); err != nil {
		return err
	}
	declared, err := r.DelegationFact.RefV2()
	if err != nil || declared != r.RuntimeAttempt.Prepared.DeclaredDelegation || !runtimeports.SameOperationSubjectV3(r.DelegationFact.Operation, r.Intent.Operation) || r.DelegationFact.IntentID != r.Intent.ID || r.DelegationFact.IntentRevision != r.Intent.Revision || r.DelegationFact.IntentDigest != r.RuntimeAttempt.Admission.IntentDigest || r.DelegationFact.ProviderPermitID != r.RuntimeAttempt.PermitID || r.DelegationFact.ProviderPermitRevision != r.RuntimeAttempt.PermitRevision || r.DelegationFact.ProviderPermitDigest != r.RuntimeAttempt.PermitDigest || r.DelegationFact.ProviderAttemptID != r.RuntimeAttempt.AttemptID || r.DelegationFact.DataProvider != r.Intent.Provider || r.DelegationFact.PayloadSchema != r.Intent.Payload.Schema || r.DelegationFact.PayloadDigest != r.Intent.Payload.ContentDigest || r.DelegationFact.PayloadRevision != r.Intent.PayloadRevision {
		return core.NewError(core.ErrorConflict, core.ReasonProviderBindingStale, "BindPrepared delegation route differs from the exact Runtime attempt")
	}
	if r.Prepared.Delegation != r.RuntimeAttempt.Delegation || r.Prepared.Prepared != r.RuntimeAttempt.Prepared || r.Prepared.Enforcement != r.RuntimeAttempt.Enforcement {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "BindPrepared sidecars describe different provider attempts")
	}
	return nil
}

func (r MarkUnknownOperationRequestV3) Validate() error {
	if err := validateOperationDomainRequestV3(r.StepKind, r.Attempt, r.Intent, r.RuntimeAttempt); err != nil {
		return err
	}
	if r.Attempt.State != contract.OperationDispatchUnknownV3 || !r.Attempt.DispatchUnknown || r.Authorization.State != runtimeports.OperationDispatchAuthorizationUnknownV3 || r.RuntimeAttempt.Observation != nil || r.RuntimeAttempt.Settlement != nil {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectUnknownOutcome, "MarkUnknown requires post-prepared Runtime refs and exact unknown authorization")
	}
	if r.Authorization.Attempt.EffectID != r.RuntimeAttempt.Admission.EffectID || r.Authorization.Attempt.PermitID != r.RuntimeAttempt.PermitID || r.Authorization.Attempt.AttemptID != r.RuntimeAttempt.AttemptID {
		return core.NewError(core.ErrorConflict, core.ReasonEffectUnknownOutcome, "unknown authorization belongs to another Runtime attempt")
	}
	return nil
}

func (r BindObservedOperationRequestV3) Validate() error {
	if err := validateOperationDomainRequestV3(r.StepKind, r.Attempt, r.Intent, r.RuntimeAttempt); err != nil {
		return err
	}
	if r.Attempt.State != contract.OperationProviderObservedV3 || r.RuntimeAttempt.Observation == nil || *r.RuntimeAttempt.Observation != r.Observation || r.RuntimeAttempt.Settlement != nil {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceConflict, "BindObserved requires one exact provider Observation sidecar")
	}
	return nil
}

func (r ApplyOperationSettlementRequestV3) Validate() error {
	if runtimeports.ValidateNamespacedNameV2(r.StepKind) != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "domain operation step kind must be namespaced")
	}
	if err := r.Attempt.Validate(); err != nil {
		return err
	}
	if err := r.Intent.Validate(); err != nil {
		return err
	}
	if r.StepKind != r.Attempt.StepKind || r.Attempt.EffectID != r.Intent.ID || r.Intent.Provider != r.Attempt.PlannedProvider || r.Intent.Authority != r.Attempt.PlanAuthority {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "settled Application attempt belongs to another Intent")
	}
	if r.Attempt.State != contract.OperationSettledV3 || r.Attempt.Settlement == nil {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectStateConflict, "ApplySettlement requires a settled Application attempt")
	}
	if err := r.Attempt.ValidateSettledForV3(r.Settlement); err != nil {
		return err
	}
	if r.DomainResult == nil {
		if r.Settlement.DomainResultSchema != nil {
			return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "ApplySettlement is missing its exact DomainResult")
		}
	} else if err := r.DomainResult.Validate(); err != nil {
		return err
	} else if r.Settlement.DomainResultSchema == nil || *r.Settlement.DomainResultSchema != r.DomainResult.Schema || r.Settlement.DomainResultDigest != r.DomainResult.ContentDigest {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "ApplySettlement DomainResult differs from the settlement")
	}
	if r.RuntimeAttempt == nil {
		if !r.Attempt.DispatchUnknown || r.Settlement.Disposition != runtimeports.OperationSettlementFailedV3 || r.Settlement.Observation != nil || r.Settlement.Attempt.Delegation != nil {
			return core.NewError(core.ErrorConflict, core.ReasonEffectUnknownOutcome, "only a pre-prepared unknown settlement may omit Runtime prepared refs")
		}
		return nil
	}
	if err := ValidateOperationDomainRuntimeAttemptV3(r.Intent, *r.RuntimeAttempt); err != nil {
		return err
	}
	if r.RuntimeAttempt.Admission.OperationDigest != r.Attempt.OperationDigest {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "settled Runtime attempt belongs to another operation")
	}
	if !r.Attempt.DispatchUnknown {
		if r.RuntimeAttempt.Settlement == nil || !sameDomainSettlementRefV3(*r.RuntimeAttempt.Settlement, r.Settlement) {
			return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "observed settlement sidecar differs from Runtime attempt")
		}
	} else if r.RuntimeAttempt.Settlement == nil || !sameDomainSettlementRefV3(*r.RuntimeAttempt.Settlement, r.Settlement) {
		return core.NewError(core.ErrorConflict, core.ReasonEffectUnknownOutcome, "post-prepared unknown settlement requires its exact Runtime sidecar")
	}
	return nil
}

func sameDomainSettlementRefV3(left, right runtimeports.OperationSettlementRefV3) bool {
	ld, le := core.CanonicalJSONDigest("praxis.application.operation-domain", OperationDomainContractVersionV3, "OperationSettlementRefV3", left)
	rd, re := core.CanonicalJSONDigest("praxis.application.operation-domain", OperationDomainContractVersionV3, "OperationSettlementRefV3", right)
	return le == nil && re == nil && ld == rd
}

// OperationDomainStatePortV3 is implemented by every built-in or user-defined
// namespaced component. Each mutation is create-once/idempotent and Inspect is
// the mandatory recovery path after an uncertain reply.
type OperationDomainStatePortV3 interface {
	ReserveOperationIntentV3(context.Context, ReserveOperationIntentRequestV3) (contract.OperationDomainReservationRefV3, error)
	InspectOperationIntentReservationV3(context.Context, InspectOperationIntentReservationRequestV3) (contract.OperationDomainReservationRefV3, error)
	BindPrepared(context.Context, BindPreparedOperationRequestV3) (OperationDomainStateRefV3, error)
	MarkUnknown(context.Context, MarkUnknownOperationRequestV3) (OperationDomainStateRefV3, error)
	BindObserved(context.Context, BindObservedOperationRequestV3) (OperationDomainStateRefV3, error)
	ApplySettlement(context.Context, ApplyOperationSettlementRequestV3) (OperationDomainStateRefV3, error)
	InspectOperationDomainStateV3(context.Context, OperationDomainInspectRequestV3) (OperationDomainStateRefV3, error)
}

type OperationDomainResolveRequestV3 struct {
	StepKind      runtimeports.NamespacedNameV2     `json:"step_kind"`
	Descriptor    contract.StepDescriptorRefV2      `json:"descriptor"`
	DomainAdapter runtimeports.ProviderBindingRefV2 `json:"domain_adapter"`
}

func (r OperationDomainResolveRequestV3) Validate(now time.Time) error {
	if err := r.Descriptor.Validate(r.StepKind); err != nil {
		return err
	}
	if err := r.DomainAdapter.Validate(); err != nil {
		return err
	}
	if now.IsZero() || !now.Before(time.Unix(0, r.Descriptor.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonCapabilityExpired, "operation domain descriptor is expired")
	}
	return nil
}

type OperationDomainResolverV3 interface {
	ResolveOperationDomainV3(context.Context, OperationDomainResolveRequestV3) (OperationDomainStatePortV3, error)
}

func OperationDomainBasisDigestV3(value any) (core.Digest, error) {
	return core.CanonicalJSONDigest("praxis.application.operation-domain", OperationDomainContractVersionV3, "OperationDomainBasisV3", value)
}

func ValidateOperationDomainRuntimeAttemptV3(intent runtimeports.OperationEffectIntentV3, refs runtimeports.GovernedExecutionAttemptRefsV2) error {
	if err := intent.Validate(); err != nil {
		return err
	}
	if err := refs.ValidatePrepared(); err != nil {
		return err
	}
	intentDigest, err := intent.DigestV3()
	if err != nil {
		return err
	}
	if refs.Admission.EffectID != intent.ID || refs.Admission.IntentRevision != intent.Revision || refs.Admission.IntentDigest != intentDigest || refs.Prepared.Provider != intent.Provider {
		return core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "domain Runtime attempt does not bind the exact operation intent")
	}
	return nil
}

func validateOperationDomainRequestV3(step runtimeports.NamespacedNameV2, attempt contract.GovernedOperationAttemptRefV3, intent runtimeports.OperationEffectIntentV3, refs runtimeports.GovernedExecutionAttemptRefsV2) error {
	if runtimeports.ValidateNamespacedNameV2(step) != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "domain operation step kind must be namespaced")
	}
	if err := attempt.Validate(); err != nil {
		return err
	}
	if err := ValidateOperationDomainRuntimeAttemptV3(intent, refs); err != nil {
		return err
	}
	if step != attempt.StepKind || attempt.EffectID != intent.ID || attempt.OperationDigest != refs.Admission.OperationDigest || intent.Provider != attempt.PlannedProvider || intent.Authority != attempt.PlanAuthority {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Application attempt, Intent and Runtime attempt are not the same operation")
	}
	return nil
}
