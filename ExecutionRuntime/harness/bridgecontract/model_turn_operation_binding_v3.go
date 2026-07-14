package bridgecontract

import (
	"strings"

	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	harnesscontract "github.com/Proview-China/rax/ExecutionRuntime/harness/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const ModelTurnOperationBindingContractV3 = "praxis.harness.model-turn-operation-binding/v3"

type ModelTurnOperationBindingStateV3 string

const (
	ModelTurnOperationPreparedV3 ModelTurnOperationBindingStateV3 = "prepared"
	ModelTurnOperationUnknownV3  ModelTurnOperationBindingStateV3 = "unknown"
	ModelTurnOperationObservedV3 ModelTurnOperationBindingStateV3 = "observed"
	ModelTurnOperationSettledV3  ModelTurnOperationBindingStateV3 = "settled"
)

// ModelTurnOperationBindingFactV3 is the Harness-owned durable association
// from one Application attempt to one Run/Session/Candidate. It stores only
// immutable public refs and cannot grant Runtime or Settlement authority.
type ModelTurnOperationBindingFactV3 struct {
	ContractVersion      string                                            `json:"contract_version"`
	ID                   string                                            `json:"binding_id"`
	Revision             core.Revision                                     `json:"revision"`
	State                ModelTurnOperationBindingStateV3                  `json:"state"`
	StepKind             runtimeports.NamespacedNameV2                     `json:"step_kind"`
	Scope                core.ExecutionScope                               `json:"scope"`
	ScopeDigest          core.Digest                                       `json:"scope_digest"`
	Run                  harnesscontract.RunRef                            `json:"run"`
	SessionID            string                                            `json:"session_id"`
	SessionRevision      core.Revision                                     `json:"session_revision"`
	Candidate            harnesscontract.CandidateRefV2                    `json:"candidate"`
	Provider             runtimeports.ProviderBindingRefV2                 `json:"provider"`
	ApplicationAttempt   applicationcontract.GovernedOperationAttemptRefV3 `json:"application_attempt"`
	RuntimeAttempt       *runtimeports.GovernedExecutionAttemptRefsV2      `json:"runtime_attempt,omitempty"`
	DelegationFact       *runtimeports.ExecutionDelegationFactV2           `json:"delegation_fact,omitempty"`
	UnknownAuthorization *runtimeports.OperationDispatchAuthorizationV3    `json:"unknown_authorization,omitempty"`
	Settlement           *runtimeports.OperationSettlementRefV3            `json:"settlement,omitempty"`
	DomainResult         *runtimeports.OpaquePayloadV2                     `json:"domain_result,omitempty"`
	BasisDigest          core.Digest                                       `json:"basis_digest"`
	CreatedUnixNano      int64                                             `json:"created_unix_nano"`
	UpdatedUnixNano      int64                                             `json:"updated_unix_nano"`
}

func (f ModelTurnOperationBindingFactV3) Validate() error {
	if f.ContractVersion != ModelTurnOperationBindingContractV3 || strings.TrimSpace(f.ID) == "" || len(f.ID) > harnesscontract.MaxReferenceBytes || f.Revision == 0 || f.SessionRevision == 0 || strings.TrimSpace(f.SessionID) == "" || len(f.SessionID) > harnesscontract.MaxReferenceBytes || f.CreatedUnixNano <= 0 || f.UpdatedUnixNano < f.CreatedUnixNano {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "model-turn operation binding identity and watermarks are incomplete")
	}
	if runtimeports.ValidateNamespacedNameV2(f.StepKind) != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidNamespace, "model-turn operation step kind must be namespaced")
	}
	if err := f.Scope.Validate(); err != nil {
		return err
	}
	scopeDigest, err := runtimeports.ExecutionScopeDigestV2(f.Scope)
	if err != nil || scopeDigest != f.ScopeDigest || f.ApplicationAttempt.ScopeDigest != f.ScopeDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidReference, "model-turn operation scope digest drifted")
	}
	if err := f.Run.Validate(); err != nil {
		return err
	}
	if !sameBindingScopeV3(f.Run.Scope, f.Scope) {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidReference, "model-turn operation Run belongs to another scope")
	}
	if err := f.Candidate.Validate(); err != nil {
		return err
	}
	if err := f.Provider.Validate(); err != nil {
		return err
	}
	if err := f.ApplicationAttempt.Validate(); err != nil {
		return err
	}
	if f.ApplicationAttempt.DomainReservation == nil || f.ApplicationAttempt.DomainReservation.SessionRef != f.SessionID || f.ApplicationAttempt.DomainReservation.CandidateDigest != f.Candidate.Digest || f.ApplicationAttempt.DomainReservation.DomainAdapter != f.ApplicationAttempt.DomainAdapter || f.ID != f.ApplicationAttempt.ID || f.ApplicationAttempt.StepID == "" || f.BasisDigest.Validate() != nil {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "model-turn operation binding differs from its Application attempt")
	}
	if f.RuntimeAttempt != nil {
		if err := f.RuntimeAttempt.ValidatePrepared(); err != nil {
			return err
		}
		if f.RuntimeAttempt.Prepared.Provider != f.Provider {
			return core.NewError(core.ErrorConflict, core.ReasonProviderBindingStale, "model-turn operation provider drifted")
		}
		if f.DelegationFact == nil {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonProviderBindingStale, "prepared model-turn operation requires its immutable delegation route")
		}
		if err := validateBindingDelegationV3(*f.DelegationFact, f); err != nil {
			return err
		}
	} else if f.DelegationFact != nil {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonProviderBindingStale, "undispatched model-turn operation cannot carry a delegation route")
	}
	switch f.State {
	case ModelTurnOperationPreparedV3:
		if f.ApplicationAttempt.State != applicationcontract.OperationExecutionPreparedV3 || f.RuntimeAttempt == nil || f.RuntimeAttempt.Observation != nil || f.RuntimeAttempt.Settlement != nil || f.UnknownAuthorization != nil || f.Settlement != nil || f.DomainResult != nil {
			return invalidModelTurnOperationStateV3()
		}
	case ModelTurnOperationUnknownV3:
		if f.ApplicationAttempt.State != applicationcontract.OperationDispatchUnknownV3 || !f.ApplicationAttempt.DispatchUnknown || f.RuntimeAttempt == nil || f.RuntimeAttempt.Observation != nil || f.RuntimeAttempt.Settlement != nil || f.UnknownAuthorization == nil || f.Settlement != nil || f.DomainResult != nil {
			return invalidModelTurnOperationStateV3()
		}
		if err := f.UnknownAuthorization.Validate(); err != nil {
			return err
		}
		if f.UnknownAuthorization.State != runtimeports.OperationDispatchAuthorizationUnknownV3 || !sameDispatchAttemptBindingV3(f.UnknownAuthorization.Attempt, f.RuntimeAttempt) {
			return core.NewError(core.ErrorConflict, core.ReasonEffectUnknownOutcome, "model-turn unknown authorization differs from the persisted Runtime attempt")
		}
	case ModelTurnOperationObservedV3:
		if f.ApplicationAttempt.State != applicationcontract.OperationProviderObservedV3 || f.ApplicationAttempt.DispatchUnknown || f.RuntimeAttempt == nil || f.RuntimeAttempt.Observation == nil || f.RuntimeAttempt.Settlement != nil || f.UnknownAuthorization != nil || f.Settlement != nil || f.DomainResult != nil {
			return invalidModelTurnOperationStateV3()
		}
	case ModelTurnOperationSettledV3:
		if f.ApplicationAttempt.State != applicationcontract.OperationSettledV3 || f.Settlement == nil || f.DomainResult == nil {
			return invalidModelTurnOperationStateV3()
		}
		if err := f.Settlement.Validate(); err != nil {
			return err
		}
		if err := f.DomainResult.Validate(); err != nil {
			return err
		}
		if f.ApplicationAttempt.Settlement == nil || !sameOperationSettlementV3(*f.ApplicationAttempt.Settlement, *f.Settlement) || f.Settlement.DomainResultSchema == nil || *f.Settlement.DomainResultSchema != f.DomainResult.Schema || f.Settlement.DomainResultDigest != f.DomainResult.ContentDigest {
			return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "model-turn settlement binds another DomainResult")
		}
		if f.RuntimeAttempt == nil {
			if !f.ApplicationAttempt.DispatchUnknown || f.UnknownAuthorization != nil || f.Settlement.Observation != nil || f.Settlement.Attempt.Delegation != nil {
				return invalidModelTurnOperationStateV3()
			}
		} else {
			if f.RuntimeAttempt.Settlement == nil || !sameOperationSettlementV3(*f.RuntimeAttempt.Settlement, *f.Settlement) {
				return core.NewError(core.ErrorConflict, core.ReasonSettlementOwnerMismatch, "model-turn Runtime, Application and domain Settlements differ")
			}
			if f.ApplicationAttempt.DispatchUnknown {
				if f.UnknownAuthorization == nil || f.RuntimeAttempt.Observation != nil || f.Settlement.Observation != nil || f.Settlement.InspectionEffect == nil || f.Settlement.InspectionSettlement == nil {
					return invalidModelTurnOperationStateV3()
				}
			} else if f.UnknownAuthorization != nil || f.RuntimeAttempt.Observation == nil || f.Settlement.Observation == nil || *f.RuntimeAttempt.Observation != *f.Settlement.Observation {
				return invalidModelTurnOperationStateV3()
			}
		}
	default:
		return invalidModelTurnOperationStateV3()
	}
	expectedBasis, err := f.DeriveBasisDigestV3()
	if err != nil || expectedBasis != f.BasisDigest {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "model-turn binding basis does not match its persisted causal facts")
	}
	return nil
}

// DeriveBasisDigestV3 reconstructs exactly the Application basis from the
// Harness-owned persisted facts. Callers cannot supply an opaque digest that
// hides different Observation, authorization, Settlement or DomainResult.
func (f ModelTurnOperationBindingFactV3) DeriveBasisDigestV3() (core.Digest, error) {
	switch f.State {
	case ModelTurnOperationPreparedV3:
		if f.RuntimeAttempt == nil || f.DelegationFact == nil {
			return "", invalidModelTurnOperationStateV3()
		}
		prepared := runtimeports.PreparedExecutionGovernanceResultV2{Delegation: f.RuntimeAttempt.Delegation, Prepared: f.RuntimeAttempt.Prepared, Enforcement: f.RuntimeAttempt.Enforcement}
		return applicationports.OperationDomainBasisDigestV3(struct {
			RuntimeAttempt runtimeports.GovernedExecutionAttemptRefsV2      `json:"runtime_attempt"`
			DelegationFact runtimeports.ExecutionDelegationFactV2           `json:"delegation_fact"`
			Prepared       runtimeports.PreparedExecutionGovernanceResultV2 `json:"prepared"`
		}{*f.RuntimeAttempt, *f.DelegationFact, prepared})
	case ModelTurnOperationUnknownV3:
		if f.RuntimeAttempt == nil || f.UnknownAuthorization == nil {
			return "", invalidModelTurnOperationStateV3()
		}
		return applicationports.OperationDomainBasisDigestV3(struct {
			RuntimeAttempt runtimeports.GovernedExecutionAttemptRefsV2   `json:"runtime_attempt"`
			Authorization  runtimeports.OperationDispatchAuthorizationV3 `json:"authorization"`
		}{*f.RuntimeAttempt, *f.UnknownAuthorization})
	case ModelTurnOperationObservedV3:
		if f.RuntimeAttempt == nil || f.RuntimeAttempt.Observation == nil {
			return "", invalidModelTurnOperationStateV3()
		}
		return applicationports.OperationDomainBasisDigestV3(struct {
			RuntimeAttempt runtimeports.GovernedExecutionAttemptRefsV2  `json:"runtime_attempt"`
			Observation    runtimeports.ProviderAttemptObservationRefV2 `json:"observation"`
		}{*f.RuntimeAttempt, *f.RuntimeAttempt.Observation})
	case ModelTurnOperationSettledV3:
		return applicationports.OperationDomainBasisDigestV3(struct {
			RuntimeAttempt *runtimeports.GovernedExecutionAttemptRefsV2 `json:"runtime_attempt,omitempty"`
			Settlement     runtimeports.OperationSettlementRefV3        `json:"settlement"`
			DomainResult   *runtimeports.OpaquePayloadV2                `json:"domain_result,omitempty"`
		}{f.RuntimeAttempt, dereferenceSettlementV3(f.Settlement), f.DomainResult})
	default:
		return "", invalidModelTurnOperationStateV3()
	}
}

func (f ModelTurnOperationBindingFactV3) DigestV3() (core.Digest, error) {
	if err := f.Validate(); err != nil {
		return "", err
	}
	return core.CanonicalJSONDigest("praxis.harness.model-turn-operation", ModelTurnOperationBindingContractV3, "ModelTurnOperationBindingFactV3", f)
}

func ValidateModelTurnOperationBindingTransitionV3(current, next ModelTurnOperationBindingFactV3) error {
	if err := current.Validate(); err != nil {
		return err
	}
	if err := next.Validate(); err != nil {
		return err
	}
	if current.ID != next.ID || current.StepKind != next.StepKind || current.ScopeDigest != next.ScopeDigest || !sameBindingScopeV3(current.Scope, next.Scope) || !sameBindingRunV3(current.Run, next.Run) || current.SessionID != next.SessionID || current.Candidate != next.Candidate || current.Provider != next.Provider || current.CreatedUnixNano != next.CreatedUnixNano || !sameApplicationAttemptOriginV3(current.ApplicationAttempt, next.ApplicationAttempt) || !sameDelegationFactV3(current.DelegationFact, next.DelegationFact) {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidReference, "model-turn operation binding immutable identity changed")
	}
	if next.Revision != current.Revision+1 || next.SessionRevision != current.SessionRevision+1 || next.ApplicationAttempt.Revision <= current.ApplicationAttempt.Revision || next.UpdatedUnixNano < current.UpdatedUnixNano {
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "model-turn operation binding watermarks are not monotonic")
	}
	allowed := current.State == ModelTurnOperationPreparedV3 && (next.State == ModelTurnOperationUnknownV3 || next.State == ModelTurnOperationObservedV3) || (current.State == ModelTurnOperationUnknownV3 || current.State == ModelTurnOperationObservedV3) && next.State == ModelTurnOperationSettledV3
	if !allowed || !runtimeAttemptTransitionExactV3(current, next) || !sameUnknownAuthorizationTransitionV3(current, next) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "model-turn operation binding transition is not allowed")
	}
	return nil
}

func validateBindingDelegationV3(delegation runtimeports.ExecutionDelegationFactV2, binding ModelTurnOperationBindingFactV3) error {
	if err := delegation.Validate(); err != nil {
		return err
	}
	declared, err := delegation.RefV2()
	operationDigest, operationErr := delegation.Operation.DigestV3()
	if err != nil || operationErr != nil || binding.RuntimeAttempt == nil || declared != binding.RuntimeAttempt.Prepared.DeclaredDelegation || delegation.RuntimeSessionRef != binding.SessionID || delegation.DataProvider != binding.Provider || delegation.Operation.Kind != runtimeports.OperationScopeRunV3 || delegation.Operation.RunID != binding.Run.RunID || !sameBindingScopeV3(delegation.Operation.ExecutionScope, binding.Scope) || operationDigest != binding.RuntimeAttempt.Admission.OperationDigest || delegation.ProviderAttemptID != binding.RuntimeAttempt.AttemptID || delegation.ProviderPermitID != binding.RuntimeAttempt.PermitID || delegation.ProviderPermitRevision != binding.RuntimeAttempt.PermitRevision || delegation.ProviderPermitDigest != binding.RuntimeAttempt.PermitDigest {
		return core.NewError(core.ErrorConflict, core.ReasonProviderBindingStale, "model-turn binding delegation differs from its exact Runtime attempt, Session or provider")
	}
	return nil
}

func invalidModelTurnOperationStateV3() error {
	return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "model-turn operation binding state fields are inconsistent")
}

func sameBindingScopeV3(left, right core.ExecutionScope) bool {
	ld, le := core.CanonicalJSONDigest("praxis.harness.model-turn-operation", ModelTurnOperationBindingContractV3, "ExecutionScope", left)
	rd, re := core.CanonicalJSONDigest("praxis.harness.model-turn-operation", ModelTurnOperationBindingContractV3, "ExecutionScope", right)
	return le == nil && re == nil && ld == rd
}

func sameBindingRunV3(left, right harnesscontract.RunRef) bool {
	ld, le := core.CanonicalJSONDigest("praxis.harness.model-turn-operation", ModelTurnOperationBindingContractV3, "RunRef", left)
	rd, re := core.CanonicalJSONDigest("praxis.harness.model-turn-operation", ModelTurnOperationBindingContractV3, "RunRef", right)
	return le == nil && re == nil && ld == rd
}

func sameApplicationAttemptOriginV3(left, right applicationcontract.GovernedOperationAttemptRefV3) bool {
	return left.ID == right.ID && left.ScopeDigest == right.ScopeDigest && left.JournalID == right.JournalID && left.StepID == right.StepID && left.StepKind == right.StepKind && left.Descriptor == right.Descriptor && left.PlannedProvider == right.PlannedProvider && left.DomainAdapter == right.DomainAdapter && left.PlanAuthority == right.PlanAuthority && left.RoutingDigest == right.RoutingDigest && left.WorkflowAttempt == right.WorkflowAttempt && left.OperationDigest == right.OperationDigest && left.EffectID == right.EffectID && sameReservationV3(left.DomainReservation, right.DomainReservation)
}

func sameReservationV3(left, right *applicationcontract.OperationDomainReservationRefV3) bool {
	return left != nil && right != nil && left.ID == right.ID && left.Digest == right.Digest
}

func runtimeAttemptTransitionExactV3(current, next ModelTurnOperationBindingFactV3) bool {
	if current.RuntimeAttempt == nil || next.RuntimeAttempt == nil {
		return false
	}
	expected := *current.RuntimeAttempt
	switch next.State {
	case ModelTurnOperationUnknownV3:
		// No provider sidecar may appear at the unknown watermark.
	case ModelTurnOperationObservedV3:
		expected.Observation = next.RuntimeAttempt.Observation
	case ModelTurnOperationSettledV3:
		expected.Settlement = next.RuntimeAttempt.Settlement
	default:
		return false
	}
	return sameRuntimeAttemptV3(expected, *next.RuntimeAttempt)
}

func sameUnknownAuthorizationTransitionV3(current, next ModelTurnOperationBindingFactV3) bool {
	if next.State == ModelTurnOperationUnknownV3 {
		return current.UnknownAuthorization == nil && next.UnknownAuthorization != nil
	}
	if current.State == ModelTurnOperationUnknownV3 && next.State == ModelTurnOperationSettledV3 {
		return sameAuthorizationV3(current.UnknownAuthorization, next.UnknownAuthorization)
	}
	return current.UnknownAuthorization == nil && next.UnknownAuthorization == nil
}

func sameRuntimeAttemptV3(left, right runtimeports.GovernedExecutionAttemptRefsV2) bool {
	ld, le := core.CanonicalJSONDigest("praxis.harness.model-turn-operation", ModelTurnOperationBindingContractV3, "RuntimeAttempt", left)
	rd, re := core.CanonicalJSONDigest("praxis.harness.model-turn-operation", ModelTurnOperationBindingContractV3, "RuntimeAttempt", right)
	return le == nil && re == nil && ld == rd
}

func sameAuthorizationV3(left, right *runtimeports.OperationDispatchAuthorizationV3) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	ld, le := core.CanonicalJSONDigest("praxis.harness.model-turn-operation", ModelTurnOperationBindingContractV3, "UnknownAuthorization", left)
	rd, re := core.CanonicalJSONDigest("praxis.harness.model-turn-operation", ModelTurnOperationBindingContractV3, "UnknownAuthorization", right)
	return le == nil && re == nil && ld == rd
}

func sameDispatchAttemptBindingV3(attempt runtimeports.OperationDispatchAttemptRefV3, runtimeAttempt *runtimeports.GovernedExecutionAttemptRefsV2) bool {
	return runtimeAttempt != nil && attempt.OperationDigest == runtimeAttempt.Admission.OperationDigest && attempt.EffectID == runtimeAttempt.Admission.EffectID && attempt.IntentRevision == runtimeAttempt.Admission.IntentRevision && attempt.IntentDigest == runtimeAttempt.Admission.IntentDigest && attempt.PermitID == runtimeAttempt.PermitID && attempt.PermitRevision == runtimeAttempt.PermitRevision && attempt.PermitDigest == runtimeAttempt.PermitDigest && attempt.AttemptID == runtimeAttempt.AttemptID
}

func sameOperationSettlementV3(left, right runtimeports.OperationSettlementRefV3) bool {
	ld, le := core.CanonicalJSONDigest("praxis.harness.model-turn-operation", ModelTurnOperationBindingContractV3, "OperationSettlementRefV3", left)
	rd, re := core.CanonicalJSONDigest("praxis.harness.model-turn-operation", ModelTurnOperationBindingContractV3, "OperationSettlementRefV3", right)
	return le == nil && re == nil && ld == rd
}

func dereferenceSettlementV3(value *runtimeports.OperationSettlementRefV3) runtimeports.OperationSettlementRefV3 {
	if value == nil {
		return runtimeports.OperationSettlementRefV3{}
	}
	return *value
}

func sameDelegationFactV3(left, right *runtimeports.ExecutionDelegationFactV2) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	ld, le := left.DigestV2()
	rd, re := right.DigestV2()
	return le == nil && re == nil && ld == rd
}
