package contract

import (
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const (
	SingleCallActionCoordinateContractVersionV2   = "praxis.application.single-call-action-coordinate/v2"
	SingleCallToolActionContractVersionV2         = "praxis.application.single-call-tool-action/v2"
	SingleCallToolActionResultContractVersionV2   = "praxis.application.single-call-tool-action-result/v2"
	SingleCallToolActionResultCoordinateVersionV2 = "praxis.application.single-call-tool-action-result-coordinate/v2"
	SingleCallToolActionCurrentContractVersionV2  = "praxis.application.single-call-current/v2"
	SingleCallToolActionCoordinationVersionV2     = "praxis.application.single-call-coordination/v2"
	SingleCallModelProjectionContractVersionV1    = "praxis.model-invoker.tool-call-observation-projection/v1"
	SingleCallHarnessOwnerCurrentInputsVersionV1  = "praxis.harness.committed-pending-action-owner-current-inputs/v1"
	SingleCallIdentityContractVersionV1           = "praxis.harness.model-tool-call-pending-action-identity/v1"
	SingleCallHarnessBindingContractVersionV2     = "praxis.harness.pending-action-application-binding/v2"
	SingleCallHarnessCurrentContractVersionV3     = "praxis.harness.committed-pending-action-current/v3"
	SingleCallToolOwnerResultContractVersionV2    = "praxis.tool-mcp.result/v2"
	SingleCallCallOrdinalEncodingVersionV1        = "presence/v1"
	MaxSingleCallCoordinateIDBytesV2              = 512
)

func validSingleCallIDV2(value string) bool {
	return value != "" && len(value) <= MaxSingleCallCoordinateIDBytesV2 && strings.TrimSpace(value) == value
}

func validateDigestsV2(values ...core.Digest) error {
	for _, value := range values {
		if err := value.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func digestV2(domain, discriminator string, value any) (core.Digest, error) {
	return core.CanonicalJSONDigest(domain, "2.0.0", discriminator, value)
}

type SingleCallModelPendingActionIdentityCoordinateV2 struct {
	IdentityContractVersion    string                            `json:"identity_contract_version"`
	IdentityID                 string                            `json:"identity_id"`
	IdentityRevision           core.Revision                     `json:"identity_revision"`
	IdentityDigest             core.Digest                       `json:"identity_digest"`
	CreatedUnixNano            int64                             `json:"created_unix_nano"`
	ModelProjectionID          string                            `json:"model_projection_id"`
	ModelProjectionRevision    core.Revision                     `json:"model_projection_revision"`
	ModelProjectionDigest      core.Digest                       `json:"model_projection_digest"`
	ModelInvocationID          string                            `json:"model_invocation_id"`
	ModelInvocationDigest      core.Digest                       `json:"model_invocation_digest"`
	ModelObservationDigest     core.Digest                       `json:"model_observation_digest"`
	ModelSourceResponseID      string                            `json:"model_source_response_id,omitempty"`
	ModelSourceSequence        uint64                            `json:"model_source_sequence"`
	SourceKeyDigest            core.Digest                       `json:"source_key_digest"`
	SourceExecutionScopeDigest core.Digest                       `json:"source_execution_scope_digest"`
	SourceRunID                string                            `json:"source_run_id"`
	SourceSessionID            string                            `json:"source_session_id"`
	SourceTurn                 uint32                            `json:"source_turn"`
	CallOrdinalEncodingVersion string                            `json:"call_ordinal_encoding_version"`
	CallOrdinalPresent         bool                              `json:"call_ordinal_present"`
	CallOrdinalValue           uint32                            `json:"call_ordinal_value"`
	SettlementOwner            runtimeports.ProviderBindingRefV2 `json:"settlement_owner"`
	CallID                     string                            `json:"call_id"`
	CallName                   string                            `json:"call_name"`
	CanonicalArgumentsDigest   core.Digest                       `json:"canonical_arguments_digest"`
	PendingActionRef           string                            `json:"pending_action_ref"`
	PendingActionRequestDigest core.Digest                       `json:"pending_action_request_digest"`
	PayloadSchema              runtimeports.SchemaRefV2          `json:"payload_schema"`
	PayloadContentDigest       core.Digest                       `json:"payload_content_digest"`
	Capability                 runtimeports.CapabilityNameV2     `json:"capability"`
	SourceCandidateID          string                            `json:"source_candidate_id"`
	SourceCandidateRevision    core.Revision                     `json:"source_candidate_revision"`
	SourceCandidateDigest      core.Digest                       `json:"source_candidate_digest"`
	DomainResultDigest         core.Digest                       `json:"domain_result_digest"`
	NotAfterUnixNano           int64                             `json:"not_after_unix_nano"`
	Digest                     core.Digest                       `json:"digest"`
}

func (v SingleCallModelPendingActionIdentityCoordinateV2) bodyDigestV2() (core.Digest, error) {
	v.Digest = ""
	return digestV2("praxis.application.single-call-identity-coordinate-v2", "SingleCallModelPendingActionIdentityCoordinateV2", v)
}
func (v SingleCallModelPendingActionIdentityCoordinateV2) Validate() error {
	if v.IdentityContractVersion != SingleCallIdentityContractVersionV1 || !validSingleCallIDV2(v.IdentityID) || v.IdentityRevision == 0 || v.CreatedUnixNano <= 0 || v.NotAfterUnixNano <= v.CreatedUnixNano || !validSingleCallIDV2(v.ModelProjectionID) || v.ModelProjectionRevision == 0 || !validSingleCallIDV2(v.ModelInvocationID) || v.ModelSourceSequence == 0 || !validSingleCallIDV2(v.SourceRunID) || !validSingleCallIDV2(v.SourceSessionID) || v.CallOrdinalEncodingVersion != SingleCallCallOrdinalEncodingVersionV1 || !v.CallOrdinalPresent || v.CallOrdinalValue != 0 || !validSingleCallIDV2(v.CallID) || !validSingleCallIDV2(v.CallName) || !validSingleCallIDV2(v.PendingActionRef) || !validSingleCallIDV2(v.SourceCandidateID) || v.SourceCandidateRevision == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "single-call identity coordinate is incomplete")
	}
	if err := v.SettlementOwner.Validate(); err != nil {
		return err
	}
	if err := v.PayloadSchema.Validate(); err != nil {
		return err
	}
	if err := runtimeports.ValidateNamespacedNameV2(runtimeports.NamespacedNameV2(v.Capability)); err != nil {
		return err
	}
	if err := validateDigestsV2(v.IdentityDigest, v.ModelProjectionDigest, v.ModelInvocationDigest, v.ModelObservationDigest, v.SourceKeyDigest, v.SourceExecutionScopeDigest, v.CanonicalArgumentsDigest, v.PendingActionRequestDigest, v.PayloadContentDigest, v.SourceCandidateDigest, v.DomainResultDigest, v.Digest); err != nil {
		return err
	}
	d, e := v.bodyDigestV2()
	if e != nil || d != v.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "single-call identity coordinate digest drifted")
	}
	return nil
}
func SealSingleCallModelPendingActionIdentityCoordinateV2(v SingleCallModelPendingActionIdentityCoordinateV2) (SingleCallModelPendingActionIdentityCoordinateV2, error) {
	v.Digest = ""
	d, e := v.bodyDigestV2()
	if e != nil {
		return SingleCallModelPendingActionIdentityCoordinateV2{}, e
	}
	v.Digest = d
	return v, v.Validate()
}

type SingleCallModelPendingActionIdentityRefCoordinateV2 struct {
	ID                         string        `json:"id"`
	Revision                   core.Revision `json:"revision"`
	Digest                     core.Digest   `json:"digest"`
	ModelProjectionID          string        `json:"model_projection_id"`
	ModelProjectionRevision    core.Revision `json:"model_projection_revision"`
	ModelProjectionDigest      core.Digest   `json:"model_projection_digest"`
	PendingActionRef           string        `json:"pending_action_ref"`
	PendingActionRequestDigest core.Digest   `json:"pending_action_request_digest"`
	DomainResultDigest         core.Digest   `json:"domain_result_digest"`
	SourceKeyDigest            core.Digest   `json:"source_key_digest"`
}

func (v SingleCallModelPendingActionIdentityRefCoordinateV2) Validate() error {
	if !validSingleCallIDV2(v.ID) || v.Revision == 0 || !validSingleCallIDV2(v.ModelProjectionID) || v.ModelProjectionRevision == 0 || !validSingleCallIDV2(v.PendingActionRef) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "single-call identity ref is incomplete")
	}
	return validateDigestsV2(v.Digest, v.ModelProjectionDigest, v.PendingActionRequestDigest, v.DomainResultDigest, v.SourceKeyDigest)
}

type SingleCallSettledTurnDomainResultFactRefCoordinateV2 struct {
	FactID          string                                              `json:"fact_id"`
	Revision        core.Revision                                       `json:"revision"`
	FactDigest      core.Digest                                         `json:"fact_digest"`
	SourceKeyDigest core.Digest                                         `json:"source_key_digest"`
	Schema          runtimeports.SchemaRefV2                            `json:"schema"`
	ContentDigest   core.Digest                                         `json:"content_digest"`
	IdentityRef     SingleCallModelPendingActionIdentityRefCoordinateV2 `json:"identity_ref"`
}

func (v SingleCallSettledTurnDomainResultFactRefCoordinateV2) Validate() error {
	if !validSingleCallIDV2(v.FactID) || v.Revision == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "single-call domain result ref is incomplete")
	}
	if err := v.Schema.Validate(); err != nil {
		return err
	}
	if err := v.IdentityRef.Validate(); err != nil {
		return err
	}
	return validateDigestsV2(v.FactDigest, v.SourceKeyDigest, v.ContentDigest)
}

type SingleCallHarnessBaseBindingCoordinateV2 struct {
	PendingAction       SingleCallPendingActionCoordinateV1                  `json:"pending_action"`
	IdentityRef         SingleCallModelPendingActionIdentityRefCoordinateV2  `json:"identity_ref"`
	DomainResultFact    SingleCallSettledTurnDomainResultFactRefCoordinateV2 `json:"domain_result_fact"`
	ModelTurnSettlement runtimeports.OperationSettlementRefV3                `json:"model_turn_settlement"`
	Digest              core.Digest                                          `json:"digest"`
}

func (v SingleCallHarnessBaseBindingCoordinateV2) Validate() error {
	if err := v.PendingAction.Validate(); err != nil {
		return err
	}
	if err := v.IdentityRef.Validate(); err != nil {
		return err
	}
	if err := v.DomainResultFact.Validate(); err != nil {
		return err
	}
	if err := v.ModelTurnSettlement.Validate(); err != nil {
		return err
	}
	if v.IdentityRef != v.DomainResultFact.IdentityRef || v.PendingAction.ActionRef != v.IdentityRef.PendingActionRef || v.PendingAction.RequestDigest != v.IdentityRef.PendingActionRequestDigest || v.PendingAction.SourceCandidateID == "" || v.PendingAction.ProjectionDigest != v.IdentityRef.ModelProjectionDigest || v.DomainResultFact.SourceKeyDigest != v.IdentityRef.SourceKeyDigest || v.DomainResultFact.ContentDigest != v.IdentityRef.DomainResultDigest || v.ModelTurnSettlement.DomainResultSchema == nil || *v.ModelTurnSettlement.DomainResultSchema != v.DomainResultFact.Schema || v.ModelTurnSettlement.DomainResultDigest != v.DomainResultFact.ContentDigest {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "identity ref and domain result drifted")
	}
	digest, err := v.DigestV2()
	if err != nil || digest != v.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "single-call base binding digest drifted")
	}
	return nil
}
func (v SingleCallHarnessBaseBindingCoordinateV2) DigestV2() (core.Digest, error) {
	v.Digest = ""
	return digestV2("praxis.application.single-call-harness-binding-coordinate-v2", "SingleCallHarnessBaseBindingCoordinateV2", v)
}
func SealSingleCallHarnessBaseBindingCoordinateV2(v SingleCallHarnessBaseBindingCoordinateV2) (SingleCallHarnessBaseBindingCoordinateV2, error) {
	v.Digest = ""
	d, e := v.DigestV2()
	if e != nil {
		return v, e
	}
	v.Digest = d
	return v, v.Validate()
}

type SingleCallHarnessOwnerCurrentInputsCoordinateV2 struct {
	HarnessContractVersion       string                                                      `json:"harness_contract_version"`
	ModelTurnOperation           runtimeports.OperationSubjectV3                             `json:"model_turn_operation"`
	GenerationBindingAssociation runtimeports.GenerationBindingAssociationRefV1              `json:"generation_binding_association"`
	RouteCurrent                 runtimeports.ControlledOperationProviderRouteCurrentRefV2   `json:"route_current"`
	RouteMatrix                  runtimeports.OperationScopeEvidenceApplicabilityMatrixKeyV3 `json:"route_matrix"`
	ContextApplicability         runtimeports.OperationScopeEvidenceApplicabilityFactRefV3   `json:"context_applicability"`
	HarnessDigest                core.Digest                                                 `json:"harness_digest"`
	Digest                       core.Digest                                                 `json:"digest"`
}

func (v SingleCallHarnessOwnerCurrentInputsCoordinateV2) Validate() error {
	if v.HarnessContractVersion != SingleCallHarnessOwnerCurrentInputsVersionV1 || v.ModelTurnOperation.Validate() != nil || v.GenerationBindingAssociation.Validate() != nil || v.RouteCurrent.Validate() != nil || v.RouteMatrix.Validate() != nil || v.ContextApplicability.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "single-call owner inputs are incomplete")
	}
	if v.ContextApplicability.Kind != runtimeports.OperationScopeEvidenceContextParentKindV3 {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "single-call owner inputs context applicability kind drifted")
	}
	if err := validateDigestsV2(v.HarnessDigest, v.Digest); err != nil {
		return err
	}
	expectedMatrix := runtimeports.OperationScopeEvidenceActionMatrixV3()
	matrixDigest, err := runtimeports.DigestOperationScopeEvidenceApplicabilityMatrixKeyV3(v.RouteMatrix)
	if err != nil || v.RouteMatrix != expectedMatrix || v.RouteCurrent.MatrixDigest != matrixDigest {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "single-call owner inputs route matrix drifted")
	}
	digest, err := v.DigestV2()
	if err != nil || digest != v.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "single-call owner inputs digest drifted")
	}
	return nil
}
func (v SingleCallHarnessOwnerCurrentInputsCoordinateV2) DigestV2() (core.Digest, error) {
	v.Digest = ""
	return digestV2("praxis.application.single-call-harness-binding-coordinate-v2", "SingleCallHarnessOwnerCurrentInputsCoordinateV2", v)
}
func SealSingleCallHarnessOwnerCurrentInputsCoordinateV2(v SingleCallHarnessOwnerCurrentInputsCoordinateV2) (SingleCallHarnessOwnerCurrentInputsCoordinateV2, error) {
	v.Digest = ""
	d, e := v.DigestV2()
	if e != nil {
		return v, e
	}
	v.Digest = d
	return v, v.Validate()
}

type SingleCallHarnessApplicationBindingCoordinateV2 struct {
	BindingVersion       string                                          `json:"binding_version"`
	Base                 SingleCallHarnessBaseBindingCoordinateV2        `json:"base"`
	OwnerInputs          SingleCallHarnessOwnerCurrentInputsCoordinateV2 `json:"owner_inputs"`
	HarnessBindingDigest core.Digest                                     `json:"harness_binding_digest"`
	Digest               core.Digest                                     `json:"digest"`
}

func (v SingleCallHarnessApplicationBindingCoordinateV2) Validate() error {
	if v.BindingVersion != SingleCallHarnessBindingContractVersionV2 || v.Base.Validate() != nil || v.OwnerInputs.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "single-call binding is incomplete")
	}
	if err := validateDigestsV2(v.HarnessBindingDigest, v.Digest); err != nil {
		return err
	}
	digest, err := v.DigestV2()
	if err != nil || digest != v.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "single-call binding digest drifted")
	}
	return nil
}
func (v SingleCallHarnessApplicationBindingCoordinateV2) DigestV2() (core.Digest, error) {
	v.Digest = ""
	return digestV2("praxis.application.single-call-harness-binding-coordinate-v2", "SingleCallHarnessApplicationBindingCoordinateV2", v)
}
func SealSingleCallHarnessApplicationBindingCoordinateV2(v SingleCallHarnessApplicationBindingCoordinateV2) (SingleCallHarnessApplicationBindingCoordinateV2, error) {
	v.Digest = ""
	d, e := v.DigestV2()
	if e != nil {
		return v, e
	}
	v.Digest = d
	return v, v.Validate()
}

type SingleCallRunSubjectCoordinateV2 struct {
	ExecutionScope       core.ExecutionScope `json:"execution_scope"`
	RunID                core.AgentRunID     `json:"run_id"`
	ExecutionScopeDigest core.Digest         `json:"execution_scope_digest"`
	Digest               core.Digest         `json:"digest"`
}

func (v SingleCallRunSubjectCoordinateV2) Validate() error {
	if v.ExecutionScope.Validate() != nil || v.RunID == "" {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "single-call run subject is incomplete")
	}
	d, e := runtimeports.ExecutionScopeDigestV2(v.ExecutionScope)
	if e != nil || d != v.ExecutionScopeDigest {
		return core.NewError(core.ErrorConflict, core.ReasonEffectFenceStale, "single-call run scope drifted")
	}
	if err := v.Digest.Validate(); err != nil {
		return err
	}
	digest, err := v.DigestV2()
	if err != nil || digest != v.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "single-call run subject digest drifted")
	}
	return nil
}
func (v SingleCallRunSubjectCoordinateV2) DigestV2() (core.Digest, error) {
	v.Digest = ""
	return digestV2("praxis.application.single-call-pending-subject-v2", "SingleCallRunSubjectCoordinateV2", v)
}
func SealSingleCallRunSubjectCoordinateV2(v SingleCallRunSubjectCoordinateV2) (SingleCallRunSubjectCoordinateV2, error) {
	d, e := runtimeports.ExecutionScopeDigestV2(v.ExecutionScope)
	if e != nil {
		return v, e
	}
	v.ExecutionScopeDigest = d
	v.Digest = ""
	d, e = v.DigestV2()
	if e != nil {
		return v, e
	}
	v.Digest = d
	return v, v.Validate()
}

type SingleCallPendingActionSubjectCoordinateV2 struct {
	Run                 SingleCallRunSubjectCoordinateV2                 `json:"run"`
	SessionID           string                                           `json:"session_id"`
	SessionRevision     core.Revision                                    `json:"session_revision"`
	SessionDigest       core.Digest                                      `json:"session_digest"`
	Turn                uint32                                           `json:"turn"`
	PendingActionRef    string                                           `json:"pending_action_ref"`
	PendingActionDigest core.Digest                                      `json:"pending_action_digest"`
	Binding             SingleCallHarnessApplicationBindingCoordinateV2  `json:"binding"`
	Identity            SingleCallModelPendingActionIdentityCoordinateV2 `json:"identity"`
	Digest              core.Digest                                      `json:"digest"`
}

func (v SingleCallPendingActionSubjectCoordinateV2) Validate() error {
	if v.Run.Validate() != nil || !validSingleCallIDV2(v.SessionID) || v.SessionRevision == 0 || !validSingleCallIDV2(v.PendingActionRef) || v.Binding.Validate() != nil || v.Identity.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "single-call pending subject is incomplete")
	}
	identityRef := v.Binding.Base.IdentityRef
	pending := v.Binding.Base.PendingAction
	settlementOwner := v.Binding.Base.ModelTurnSettlement.Owner
	operation := v.Binding.OwnerInputs.ModelTurnOperation
	operationDigest, operationErr := operation.DigestV3()
	if operationErr != nil || operation.Kind != runtimeports.OperationScopeRunV3 || operation.RunID != v.Run.RunID || operation.ExecutionScopeDigest != v.Run.ExecutionScopeDigest || !runtimeports.SameExecutionScopeV2(operation.ExecutionScope, v.Run.ExecutionScope) || v.Binding.Base.ModelTurnSettlement.Attempt.OperationDigest != operationDigest || v.Run.RunID != core.AgentRunID(v.Identity.SourceRunID) || v.Run.ExecutionScopeDigest != v.Identity.SourceExecutionScopeDigest || v.SessionID != v.Identity.SourceSessionID || v.Turn != v.Identity.SourceTurn || v.PendingActionRef != v.Identity.PendingActionRef || v.PendingActionDigest != pending.RequestDigest || pending.ActionRef != v.PendingActionRef || identityRef.ID != v.Identity.IdentityID || identityRef.Revision != v.Identity.IdentityRevision || identityRef.Digest != v.Identity.IdentityDigest || identityRef.ModelProjectionID != v.Identity.ModelProjectionID || identityRef.ModelProjectionRevision != v.Identity.ModelProjectionRevision || identityRef.ModelProjectionDigest != v.Identity.ModelProjectionDigest || identityRef.PendingActionRequestDigest != v.Identity.PendingActionRequestDigest || identityRef.SourceKeyDigest != v.Identity.SourceKeyDigest || identityRef.DomainResultDigest != v.Identity.DomainResultDigest || pending.RequestDigest != v.Identity.PendingActionRequestDigest || pending.PayloadSchema != v.Identity.PayloadSchema || pending.PayloadDigest != v.Identity.CanonicalArgumentsDigest || pending.PayloadDigest != v.Identity.PayloadContentDigest || pending.Capability != v.Identity.Capability || pending.SourceCandidateID != v.Identity.SourceCandidateID || pending.SourceCandidateRevision != v.Identity.SourceCandidateRevision || pending.SourceCandidateDigest != v.Identity.SourceCandidateDigest || pending.ProjectionDigest != v.Identity.ModelProjectionDigest || settlementOwner.Role != runtimeports.OwnerSettlement || settlementOwner.ComponentID != v.Identity.SettlementOwner.ComponentID || settlementOwner.ManifestDigest != v.Identity.SettlementOwner.ManifestDigest {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "single-call pending subject owner coordinates drifted")
	}
	if err := validateDigestsV2(v.SessionDigest, v.PendingActionDigest, v.Digest); err != nil {
		return err
	}
	digest, err := v.DigestV2()
	if err != nil || digest != v.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "single-call pending subject digest drifted")
	}
	return nil
}
func (v SingleCallPendingActionSubjectCoordinateV2) DigestV2() (core.Digest, error) {
	v.Digest = ""
	return digestV2("praxis.application.single-call-pending-subject-v2", "SingleCallPendingActionSubjectCoordinateV2", v)
}
func SealSingleCallPendingActionSubjectCoordinateV2(v SingleCallPendingActionSubjectCoordinateV2) (SingleCallPendingActionSubjectCoordinateV2, error) {
	v.Digest = ""
	d, e := v.DigestV2()
	if e != nil {
		return v, e
	}
	v.Digest = d
	return v, v.Validate()
}

type SingleCallActionCoordinateV2 struct {
	ContractVersion      string                                     `json:"contract_version"`
	ExecutionScope       core.ExecutionScope                        `json:"execution_scope"`
	ExecutionScopeDigest core.Digest                                `json:"execution_scope_digest"`
	PendingSubject       SingleCallPendingActionSubjectCoordinateV2 `json:"pending_subject"`
	Digest               core.Digest                                `json:"digest"`
}

func (v SingleCallActionCoordinateV2) bodyDigestV2() (core.Digest, error) {
	v.Digest = ""
	return digestV2("praxis.application.single-call-action-coordinate-v2", "SingleCallActionCoordinateV2", v)
}
func (v SingleCallActionCoordinateV2) Validate() error {
	if v.ContractVersion != SingleCallActionCoordinateContractVersionV2 || v.ExecutionScope.Validate() != nil || v.PendingSubject.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "single-call action coordinate is incomplete")
	}
	d, e := runtimeports.ExecutionScopeDigestV2(v.ExecutionScope)
	if e != nil || d != v.ExecutionScopeDigest || d != v.PendingSubject.Run.ExecutionScopeDigest || !runtimeports.SameExecutionScopeV2(v.ExecutionScope, v.PendingSubject.Run.ExecutionScope) {
		return core.NewError(core.ErrorConflict, core.ReasonEffectFenceStale, "single-call action scope drifted")
	}
	d, e = v.bodyDigestV2()
	if e != nil || d != v.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "single-call action digest drifted")
	}
	return nil
}
func SealSingleCallActionCoordinateV2(v SingleCallActionCoordinateV2) (SingleCallActionCoordinateV2, error) {
	v.ContractVersion = SingleCallActionCoordinateContractVersionV2
	d, e := runtimeports.ExecutionScopeDigestV2(v.ExecutionScope)
	if e != nil {
		return SingleCallActionCoordinateV2{}, e
	}
	v.ExecutionScopeDigest = d
	v.Digest = ""
	d, e = v.bodyDigestV2()
	if e != nil {
		return SingleCallActionCoordinateV2{}, e
	}
	v.Digest = d
	return v, v.Validate()
}

type SingleCallToolActionRequestV2 struct {
	ContractVersion string                             `json:"contract_version"`
	ID              string                             `json:"id"`
	Revision        core.Revision                      `json:"revision"`
	Action          SingleCallActionCoordinateV2       `json:"action"`
	Authority       runtimeports.AuthorityBindingRefV2 `json:"authority"`
	CreatedUnixNano int64                              `json:"created_unix_nano"`
	ExpiresUnixNano int64                              `json:"expires_unix_nano"`
	Digest          core.Digest                        `json:"digest"`
}
type AssembleSingleCallToolActionRequestV2 struct {
	Action                    SingleCallActionCoordinateV2       `json:"action"`
	Authority                 runtimeports.AuthorityBindingRefV2 `json:"authority"`
	RequestedNotAfterUnixNano int64                              `json:"requested_not_after_unix_nano"`
}
type SingleCallToolActionRequestSubjectV2 struct {
	ActionDigest core.Digest                        `json:"action_digest"`
	Authority    runtimeports.AuthorityBindingRefV2 `json:"authority"`
}

func (v SingleCallToolActionRequestV2) subjectDigestV2() (core.Digest, error) {
	return digestV2("praxis.application.single-call-tool-action-request-id-v2", "SingleCallToolActionRequestSubjectV2", SingleCallToolActionRequestSubjectV2{ActionDigest: v.Action.Digest, Authority: v.Authority})
}
func (v SingleCallToolActionRequestV2) bodyDigestV2() (core.Digest, error) {
	v.Digest = ""
	return digestV2("praxis.application.single-call-tool-action-v2", "SingleCallToolActionRequestV2", v)
}
func (v SingleCallToolActionRequestV2) Validate() error {
	if v.ContractVersion != SingleCallToolActionContractVersionV2 || !validSingleCallIDV2(v.ID) || v.Revision != 1 || v.Action.Validate() != nil || v.Authority.Validate() != nil || v.CreatedUnixNano <= 0 || v.ExpiresUnixNano <= v.CreatedUnixNano || time.Duration(v.ExpiresUnixNano-v.CreatedUnixNano) > runtimeports.MaxDispatchPermitTTL {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "single-call V2 request is incomplete")
	}
	sd, e := v.subjectDigestV2()
	if e != nil || v.ID != "single-call-request:v2:"+strings.TrimPrefix(string(sd), "sha256:") {
		return core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "single-call V2 request ID drifted")
	}
	d, e := v.bodyDigestV2()
	if e != nil || d != v.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "single-call V2 request digest drifted")
	}
	return nil
}
func (v SingleCallToolActionRequestV2) ValidateCurrent(now time.Time) error {
	if err := v.Validate(); err != nil {
		return err
	}
	if now.IsZero() || now.UnixNano() < v.CreatedUnixNano || !now.Before(time.Unix(0, v.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectFenceStale, "single-call V2 request is not current")
	}
	return nil
}
func SealSingleCallToolActionRequestV2(v SingleCallToolActionRequestV2) (SingleCallToolActionRequestV2, error) {
	v.ContractVersion = SingleCallToolActionContractVersionV2
	v.Revision = 1
	v.ID = ""
	v.Digest = ""
	sd, e := v.subjectDigestV2()
	if e != nil {
		return SingleCallToolActionRequestV2{}, e
	}
	v.ID = "single-call-request:v2:" + strings.TrimPrefix(string(sd), "sha256:")
	d, e := v.bodyDigestV2()
	if e != nil {
		return SingleCallToolActionRequestV2{}, e
	}
	v.Digest = d
	return v, v.Validate()
}

type SingleCallModelPendingActionIdentityCurrentRequestV2 struct {
	ContractVersion           string                                               `json:"contract_version"`
	Run                       SingleCallRunSubjectCoordinateV2                     `json:"run"`
	SessionID                 string                                               `json:"session_id"`
	Turn                      uint32                                               `json:"turn"`
	IdentityRef               SingleCallModelPendingActionIdentityRefCoordinateV2  `json:"identity_ref"`
	DomainResultFact          SingleCallSettledTurnDomainResultFactRefCoordinateV2 `json:"domain_result_fact"`
	RequestedNotAfterUnixNano int64                                                `json:"requested_not_after_unix_nano"`
	Digest                    core.Digest                                          `json:"digest"`
}

func (v SingleCallModelPendingActionIdentityCurrentRequestV2) DigestV2() (core.Digest, error) {
	v.Digest = ""
	return digestV2("praxis.application.single-call-identity-current-v2", "SingleCallModelPendingActionIdentityCurrentRequestV2", v)
}
func (v SingleCallModelPendingActionIdentityCurrentRequestV2) Validate() error {
	if v.ContractVersion != SingleCallToolActionCurrentContractVersionV2 || v.Run.Validate() != nil || !validSingleCallIDV2(v.SessionID) || v.IdentityRef.Validate() != nil || v.DomainResultFact.Validate() != nil || v.RequestedNotAfterUnixNano < 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "single-call identity current request is invalid")
	}
	if v.IdentityRef != v.DomainResultFact.IdentityRef {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "identity current request refs drifted")
	}
	d, e := v.DigestV2()
	if e != nil || d != v.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "identity current request digest drifted")
	}
	return nil
}
func SealSingleCallModelPendingActionIdentityCurrentRequestV2(v SingleCallModelPendingActionIdentityCurrentRequestV2) (SingleCallModelPendingActionIdentityCurrentRequestV2, error) {
	v.ContractVersion = SingleCallToolActionCurrentContractVersionV2
	v.Digest = ""
	d, e := v.DigestV2()
	if e != nil {
		return v, e
	}
	v.Digest = d
	return v, v.Validate()
}

type SingleCallModelToolCallProjectionProofV2 struct {
	ProjectionContractVersion string        `json:"projection_contract_version"`
	ProjectionID              string        `json:"projection_id"`
	ProjectionRevision        core.Revision `json:"projection_revision"`
	ProjectionDigest          core.Digest   `json:"projection_digest"`
	InvocationID              string        `json:"invocation_id"`
	InvocationDigest          core.Digest   `json:"invocation_digest"`
	ObservationDigest         core.Digest   `json:"observation_digest"`
	SourceResponseID          string        `json:"source_response_id,omitempty"`
	SourceSequence            uint64        `json:"source_sequence"`
	CallOrdinal               uint32        `json:"call_ordinal"`
	CallID                    string        `json:"call_id"`
	CallName                  string        `json:"call_name"`
	CanonicalArguments        []byte        `json:"canonical_arguments"`
	CanonicalArgumentsLength  uint64        `json:"canonical_arguments_length"`
	CanonicalArgumentsDigest  core.Digest   `json:"canonical_arguments_digest"`
	Digest                    core.Digest   `json:"digest"`
}

func (v SingleCallModelToolCallProjectionProofV2) bodyDigestV2() (core.Digest, error) {
	v.CanonicalArguments = append([]byte(nil), v.CanonicalArguments...)
	v.Digest = ""
	return digestV2("praxis.application.single-call-model-projection-proof-v2", "SingleCallModelToolCallProjectionProofV2", v)
}
func (v SingleCallModelToolCallProjectionProofV2) Validate() error {
	if v.ProjectionContractVersion != SingleCallModelProjectionContractVersionV1 || !validSingleCallIDV2(v.ProjectionID) || v.ProjectionRevision == 0 || !validSingleCallIDV2(v.InvocationID) || v.SourceSequence == 0 || v.CallOrdinal != 0 || !validSingleCallIDV2(v.CallID) || !validSingleCallIDV2(v.CallName) || len(v.CanonicalArguments) == 0 || len(v.CanonicalArguments) > runtimeports.MaxOpaqueInlineBytes || v.CanonicalArgumentsLength != uint64(len(v.CanonicalArguments)) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "single-call model projection proof is invalid")
	}
	if core.DigestBytes(v.CanonicalArguments) != v.CanonicalArgumentsDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "canonical arguments digest drifted")
	}
	if err := validateDigestsV2(v.ProjectionDigest, v.InvocationDigest, v.ObservationDigest, v.CanonicalArgumentsDigest, v.Digest); err != nil {
		return err
	}
	d, e := v.bodyDigestV2()
	if e != nil || d != v.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "projection proof digest drifted")
	}
	return nil
}
func SealSingleCallModelToolCallProjectionProofV2(v SingleCallModelToolCallProjectionProofV2) (SingleCallModelToolCallProjectionProofV2, error) {
	v.CanonicalArguments = append([]byte(nil), v.CanonicalArguments...)
	v.CanonicalArgumentsLength = uint64(len(v.CanonicalArguments))
	v.CanonicalArgumentsDigest = core.DigestBytes(v.CanonicalArguments)
	v.Digest = ""
	d, e := v.bodyDigestV2()
	if e != nil {
		return SingleCallModelToolCallProjectionProofV2{}, e
	}
	v.Digest = d
	return CloneSingleCallModelToolCallProjectionProofV2(v), v.Validate()
}
func CloneSingleCallModelToolCallProjectionProofV2(v SingleCallModelToolCallProjectionProofV2) SingleCallModelToolCallProjectionProofV2 {
	v.CanonicalArguments = append([]byte(nil), v.CanonicalArguments...)
	return v
}

type SingleCallModelPendingActionIdentityCurrentV2 struct {
	ContractVersion  string                                               `json:"contract_version"`
	RequestDigest    core.Digest                                          `json:"request_digest"`
	IdentityRef      SingleCallModelPendingActionIdentityRefCoordinateV2  `json:"identity_ref"`
	DomainResultFact SingleCallSettledTurnDomainResultFactRefCoordinateV2 `json:"domain_result_fact"`
	Identity         SingleCallModelPendingActionIdentityCoordinateV2     `json:"identity"`
	Projection       SingleCallModelToolCallProjectionProofV2             `json:"projection"`
	CheckedUnixNano  int64                                                `json:"checked_unix_nano"`
	ExpiresUnixNano  int64                                                `json:"expires_unix_nano"`
	Digest           core.Digest                                          `json:"digest"`
}

func (v SingleCallModelPendingActionIdentityCurrentV2) Validate(now time.Time) error {
	if v.ContractVersion != SingleCallToolActionCurrentContractVersionV2 || v.IdentityRef.Validate() != nil || v.DomainResultFact.Validate() != nil || v.Identity.Validate() != nil || v.Projection.Validate() != nil || v.CheckedUnixNano <= 0 || v.ExpiresUnixNano <= v.CheckedUnixNano || now.IsZero() || now.UnixNano() < v.CheckedUnixNano || !now.Before(time.Unix(0, v.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectFenceStale, "single-call identity current is invalid")
	}
	ref := v.IdentityRef
	id := v.Identity
	p := v.Projection
	if ref != v.DomainResultFact.IdentityRef || ref.ID != id.IdentityID || ref.Revision != id.IdentityRevision || ref.Digest != id.IdentityDigest || ref.ModelProjectionID != id.ModelProjectionID || ref.ModelProjectionRevision != id.ModelProjectionRevision || ref.ModelProjectionDigest != id.ModelProjectionDigest || ref.PendingActionRef != id.PendingActionRef || ref.PendingActionRequestDigest != id.PendingActionRequestDigest || ref.DomainResultDigest != id.DomainResultDigest || ref.SourceKeyDigest != id.SourceKeyDigest || v.DomainResultFact.SourceKeyDigest != id.SourceKeyDigest || v.DomainResultFact.ContentDigest != id.DomainResultDigest || id.CreatedUnixNano > v.CheckedUnixNano || v.ExpiresUnixNano > id.NotAfterUnixNano || p.ProjectionID != id.ModelProjectionID || p.ProjectionRevision != id.ModelProjectionRevision || p.ProjectionDigest != id.ModelProjectionDigest || p.InvocationID != id.ModelInvocationID || p.InvocationDigest != id.ModelInvocationDigest || p.ObservationDigest != id.ModelObservationDigest || p.SourceResponseID != id.ModelSourceResponseID || p.SourceSequence != id.ModelSourceSequence || p.CallOrdinal != id.CallOrdinalValue || p.CallID != id.CallID || p.CallName != id.CallName || p.CanonicalArgumentsDigest != id.CanonicalArgumentsDigest {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "single-call identity current drifted")
	}
	if err := validateDigestsV2(v.RequestDigest, v.Digest); err != nil {
		return err
	}
	d, e := v.DigestV2()
	if e != nil || d != v.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "identity current digest drifted")
	}
	return nil
}

func (v SingleCallModelPendingActionIdentityCurrentV2) ValidateFor(request SingleCallModelPendingActionIdentityCurrentRequestV2, now time.Time) error {
	if err := request.Validate(); err != nil {
		return err
	}
	if err := v.Validate(now); err != nil {
		return err
	}
	if v.RequestDigest != request.Digest || v.IdentityRef != request.IdentityRef || v.DomainResultFact != request.DomainResultFact || v.Identity.SourceExecutionScopeDigest != request.Run.ExecutionScopeDigest || core.AgentRunID(v.Identity.SourceRunID) != request.Run.RunID || v.Identity.SourceSessionID != request.SessionID || v.Identity.SourceTurn != request.Turn || request.RequestedNotAfterUnixNano > 0 && v.ExpiresUnixNano > request.RequestedNotAfterUnixNano {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "single-call identity current does not bind its reader request")
	}
	return nil
}
func (v SingleCallModelPendingActionIdentityCurrentV2) DigestV2() (core.Digest, error) {
	v.Projection = CloneSingleCallModelToolCallProjectionProofV2(v.Projection)
	v.Digest = ""
	return digestV2("praxis.application.single-call-identity-current-v2", "SingleCallModelPendingActionIdentityCurrentV2", v)
}
func SealSingleCallModelPendingActionIdentityCurrentV2(v SingleCallModelPendingActionIdentityCurrentV2, request SingleCallModelPendingActionIdentityCurrentRequestV2, now time.Time) (SingleCallModelPendingActionIdentityCurrentV2, error) {
	if err := request.Validate(); err != nil {
		return SingleCallModelPendingActionIdentityCurrentV2{}, err
	}
	v.ContractVersion = SingleCallToolActionCurrentContractVersionV2
	v.RequestDigest = request.Digest
	v.Projection = CloneSingleCallModelToolCallProjectionProofV2(v.Projection)
	v.Digest = ""
	d, e := v.DigestV2()
	if e != nil {
		return v, e
	}
	v.Digest = d
	return CloneSingleCallModelPendingActionIdentityCurrentV2(v), v.ValidateFor(request, now)
}
func CloneSingleCallModelPendingActionIdentityCurrentV2(v SingleCallModelPendingActionIdentityCurrentV2) SingleCallModelPendingActionIdentityCurrentV2 {
	v.Projection = CloneSingleCallModelToolCallProjectionProofV2(v.Projection)
	return v
}

type SingleCallHarnessOwnerCurrentProofV3 struct {
	Subject                       SingleCallPendingActionSubjectCoordinateV2      `json:"subject"`
	Binding                       SingleCallHarnessApplicationBindingCoordinateV2 `json:"binding"`
	HarnessCurrentContractVersion string                                          `json:"harness_current_contract_version"`
	HarnessCurrentDigest          core.Digest                                     `json:"harness_current_digest"`
	IdentityCurrent               SingleCallModelPendingActionIdentityCurrentV2   `json:"identity_current"`
	CheckedUnixNano               int64                                           `json:"checked_unix_nano"`
	ExpiresUnixNano               int64                                           `json:"expires_unix_nano"`
	Digest                        core.Digest                                     `json:"digest"`
}

func (v SingleCallHarnessOwnerCurrentProofV3) DigestV2() (core.Digest, error) {
	v.IdentityCurrent = CloneSingleCallModelPendingActionIdentityCurrentV2(v.IdentityCurrent)
	v.Digest = ""
	return digestV2("praxis.application.single-call-current-v2", "SingleCallHarnessOwnerCurrentProofV3", v)
}
func (v SingleCallHarnessOwnerCurrentProofV3) Validate(now time.Time) error {
	if v.Subject.Validate() != nil || v.Binding.Validate() != nil || v.HarnessCurrentContractVersion != SingleCallHarnessCurrentContractVersionV3 || v.IdentityCurrent.Validate(now) != nil || v.CheckedUnixNano <= 0 || v.ExpiresUnixNano <= v.CheckedUnixNano || now.UnixNano() < v.CheckedUnixNano || !now.Before(time.Unix(0, v.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectFenceStale, "harness current proof is invalid")
	}
	if err := validateDigestsV2(v.HarnessCurrentDigest, v.Digest); err != nil {
		return err
	}
	if v.Subject.Binding.Digest != v.Binding.Digest || v.Subject.Binding.Base.IdentityRef != v.IdentityCurrent.IdentityRef || v.Subject.Binding.Base.DomainResultFact != v.IdentityCurrent.DomainResultFact || v.Subject.Identity != v.IdentityCurrent.Identity || v.Subject.PendingActionDigest != v.Binding.Base.PendingAction.RequestDigest || v.CheckedUnixNano < v.IdentityCurrent.CheckedUnixNano || v.ExpiresUnixNano > v.IdentityCurrent.ExpiresUnixNano {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "harness current proof drifted")
	}
	d, e := v.DigestV2()
	if e != nil || d != v.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "harness current proof digest drifted")
	}
	return nil
}
func SealSingleCallHarnessOwnerCurrentProofV3(v SingleCallHarnessOwnerCurrentProofV3, now time.Time) (SingleCallHarnessOwnerCurrentProofV3, error) {
	v.IdentityCurrent = CloneSingleCallModelPendingActionIdentityCurrentV2(v.IdentityCurrent)
	v.Digest = ""
	d, e := v.DigestV2()
	if e != nil {
		return v, e
	}
	v.Digest = d
	return v, v.Validate(now)
}

type SingleCallAuthorityCurrentProofV2 struct {
	Ref                    runtimeports.AuthorityBindingRefV2 `json:"ref"`
	ExecutionScopeDigest   core.Digest                        `json:"execution_scope_digest"`
	ActionCoordinateDigest core.Digest                        `json:"action_coordinate_digest"`
	FactRevision           core.Revision                      `json:"fact_revision"`
	FactDigest             core.Digest                        `json:"fact_digest"`
	CheckedUnixNano        int64                              `json:"checked_unix_nano"`
	ExpiresUnixNano        int64                              `json:"expires_unix_nano"`
	Digest                 core.Digest                        `json:"digest"`
}

func (v SingleCallAuthorityCurrentProofV2) DigestV2() (core.Digest, error) {
	v.Digest = ""
	return digestV2("praxis.application.single-call-current-v2", "SingleCallAuthorityCurrentProofV2", v)
}
func (v SingleCallAuthorityCurrentProofV2) Validate(request SingleCallToolActionRequestV2, now time.Time) error {
	if v.Ref.Validate() != nil || v.Ref != request.Authority || v.ExecutionScopeDigest != request.Action.ExecutionScopeDigest || v.ActionCoordinateDigest != request.Action.Digest || v.FactRevision == 0 || v.CheckedUnixNano <= 0 || v.ExpiresUnixNano <= v.CheckedUnixNano || v.ExpiresUnixNano > request.ExpiresUnixNano || now.UnixNano() < v.CheckedUnixNano || !now.Before(time.Unix(0, v.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonStaleAuthorityEpoch, "authority current proof is invalid")
	}
	if err := validateDigestsV2(v.FactDigest, v.Digest); err != nil {
		return err
	}
	d, e := v.DigestV2()
	if e != nil || d != v.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "authority current proof digest drifted")
	}
	return nil
}
func SealSingleCallAuthorityCurrentProofV2(v SingleCallAuthorityCurrentProofV2, request SingleCallToolActionRequestV2, now time.Time) (SingleCallAuthorityCurrentProofV2, error) {
	v.Digest = ""
	d, e := v.DigestV2()
	if e != nil {
		return v, e
	}
	v.Digest = d
	return v, v.Validate(request, now)
}

type SingleCallToolActionInputCurrentProjectionV2 struct {
	ContractVersion  string                               `json:"contract_version"`
	RequestID        string                               `json:"request_id"`
	RequestRevision  core.Revision                        `json:"request_revision"`
	RequestDigest    core.Digest                          `json:"request_digest"`
	ActionDigest     core.Digest                          `json:"action_digest"`
	HarnessCurrent   SingleCallHarnessOwnerCurrentProofV3 `json:"harness_current"`
	AuthorityCurrent SingleCallAuthorityCurrentProofV2    `json:"authority_current"`
	CheckedUnixNano  int64                                `json:"checked_unix_nano"`
	ExpiresUnixNano  int64                                `json:"expires_unix_nano"`
	Digest           core.Digest                          `json:"digest"`
}

func (v SingleCallToolActionInputCurrentProjectionV2) ValidateFor(r SingleCallToolActionRequestV2, now time.Time) error {
	if err := r.ValidateCurrent(now); err != nil {
		return err
	}
	if v.ContractVersion != SingleCallToolActionCurrentContractVersionV2 || v.RequestID != r.ID || v.RequestRevision != r.Revision || v.RequestDigest != r.Digest || v.ActionDigest != r.Action.Digest || v.CheckedUnixNano <= 0 || v.ExpiresUnixNano <= v.CheckedUnixNano || v.ExpiresUnixNano > r.ExpiresUnixNano || now.UnixNano() < v.CheckedUnixNano || !now.Before(time.Unix(0, v.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectFenceStale, "single-call input current projection drifted")
	}
	if err := v.HarnessCurrent.Validate(now); err != nil {
		return err
	}
	if err := v.AuthorityCurrent.Validate(r, now); err != nil {
		return err
	}
	expectedIdentityCurrent, err := SealSingleCallModelPendingActionIdentityCurrentRequestV2(SingleCallModelPendingActionIdentityCurrentRequestV2{
		Run:                       r.Action.PendingSubject.Run,
		SessionID:                 r.Action.PendingSubject.SessionID,
		Turn:                      r.Action.PendingSubject.Turn,
		IdentityRef:               r.Action.PendingSubject.Binding.Base.IdentityRef,
		DomainResultFact:          r.Action.PendingSubject.Binding.Base.DomainResultFact,
		RequestedNotAfterUnixNano: r.ExpiresUnixNano,
	})
	if err != nil {
		return err
	}
	if err := v.HarnessCurrent.IdentityCurrent.ValidateFor(expectedIdentityCurrent, now); err != nil {
		return err
	}
	if v.HarnessCurrent.Subject.Digest != r.Action.PendingSubject.Digest || v.HarnessCurrent.Binding.Digest != r.Action.PendingSubject.Binding.Digest || v.CheckedUnixNano < v.HarnessCurrent.CheckedUnixNano || v.CheckedUnixNano < v.AuthorityCurrent.CheckedUnixNano || v.ExpiresUnixNano > v.HarnessCurrent.ExpiresUnixNano || v.ExpiresUnixNano > v.AuthorityCurrent.ExpiresUnixNano {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "input current owner proof drifted")
	}
	d, e := v.DigestV2()
	if e != nil || d != v.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "input current projection digest drifted")
	}
	return nil
}
func (v SingleCallToolActionInputCurrentProjectionV2) DigestV2() (core.Digest, error) {
	v.HarnessCurrent.IdentityCurrent = CloneSingleCallModelPendingActionIdentityCurrentV2(v.HarnessCurrent.IdentityCurrent)
	v.Digest = ""
	return digestV2("praxis.application.single-call-current-v2", "SingleCallToolActionInputCurrentProjectionV2", v)
}
func SealSingleCallToolActionInputCurrentProjectionV2(v SingleCallToolActionInputCurrentProjectionV2, r SingleCallToolActionRequestV2, now time.Time) (SingleCallToolActionInputCurrentProjectionV2, error) {
	v.ContractVersion = SingleCallToolActionCurrentContractVersionV2
	v.RequestID = r.ID
	v.RequestRevision = r.Revision
	v.RequestDigest = r.Digest
	v.ActionDigest = r.Action.Digest
	v.HarnessCurrent.IdentityCurrent = CloneSingleCallModelPendingActionIdentityCurrentV2(v.HarnessCurrent.IdentityCurrent)
	v.Digest = ""
	d, e := v.DigestV2()
	if e != nil {
		return v, e
	}
	v.Digest = d
	return v, v.ValidateFor(r, now)
}
func CloneSingleCallToolActionInputCurrentProjectionV2(v SingleCallToolActionInputCurrentProjectionV2) SingleCallToolActionInputCurrentProjectionV2 {
	v.HarnessCurrent.IdentityCurrent = CloneSingleCallModelPendingActionIdentityCurrentV2(v.HarnessCurrent.IdentityCurrent)
	return v
}

type SingleCallToolOwnerResultRefCoordinateV2 struct {
	OwnerContractVersion string                                          `json:"owner_contract_version"`
	ID                   string                                          `json:"id"`
	Revision             core.Revision                                   `json:"revision"`
	Digest               core.Digest                                     `json:"digest"`
	ActionID             string                                          `json:"action_id"`
	ActionRevision       core.Revision                                   `json:"action_revision"`
	ActionDigest         core.Digest                                     `json:"action_digest"`
	ApplyID              string                                          `json:"apply_id"`
	ApplyRevision        core.Revision                                   `json:"apply_revision"`
	ApplyDigest          core.Digest                                     `json:"apply_digest"`
	Inspection           runtimeports.OperationInspectionSettlementRefV4 `json:"inspection"`
	Schema               runtimeports.SchemaRefV2                        `json:"schema"`
	PayloadDigest        core.Digest                                     `json:"payload_digest"`
	PayloadRevision      core.Revision                                   `json:"payload_revision"`
	FinalizedUnixNano    int64                                           `json:"finalized_unix_nano"`
}

func (v SingleCallToolOwnerResultRefCoordinateV2) Validate(now time.Time) error {
	if v.OwnerContractVersion != SingleCallToolOwnerResultContractVersionV2 || !validSingleCallIDV2(v.ID) || v.Revision == 0 || !validSingleCallIDV2(v.ActionID) || v.ActionRevision == 0 || !validSingleCallIDV2(v.ApplyID) || v.ApplyRevision == 0 || v.PayloadRevision == 0 || v.FinalizedUnixNano <= 0 || now.IsZero() || v.FinalizedUnixNano > now.UnixNano() || v.Inspection.Validate(now) != nil || v.Schema.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "single-call Tool owner result ref is invalid")
	}
	return validateDigestsV2(v.Digest, v.ActionDigest, v.ApplyDigest, v.PayloadDigest)
}

type SingleCallToolActionResultCoordinateV2 struct {
	ContractVersion            string                                                   `json:"contract_version"`
	ID                         string                                                   `json:"id"`
	Revision                   core.Revision                                            `json:"revision"`
	RequestID                  string                                                   `json:"request_id"`
	RequestRevision            core.Revision                                            `json:"request_revision"`
	RequestDigest              core.Digest                                              `json:"request_digest"`
	ActionCoordinateDigest     core.Digest                                              `json:"action_coordinate_digest"`
	ToolResult                 SingleCallToolOwnerResultRefCoordinateV2                 `json:"tool_result"`
	Inspection                 runtimeports.OperationInspectionSettlementRefV4          `json:"inspection"`
	Association                runtimeports.OperationSettlementEvidenceAssociationRefV4 `json:"association"`
	AssociationCheckedUnixNano int64                                                    `json:"association_checked_unix_nano"`
	ExpiresUnixNano            int64                                                    `json:"expires_unix_nano"`
	Digest                     core.Digest                                              `json:"digest"`
}
type SingleCallToolActionResultCoordinateSubjectV2 struct {
	RequestID              string                                                   `json:"request_id"`
	RequestRevision        core.Revision                                            `json:"request_revision"`
	RequestDigest          core.Digest                                              `json:"request_digest"`
	ActionCoordinateDigest core.Digest                                              `json:"action_coordinate_digest"`
	ToolResultID           string                                                   `json:"tool_result_id"`
	ToolResultRevision     core.Revision                                            `json:"tool_result_revision"`
	ToolResultDigest       core.Digest                                              `json:"tool_result_digest"`
	InspectionDigest       core.Digest                                              `json:"inspection_digest"`
	Association            runtimeports.OperationSettlementEvidenceAssociationRefV4 `json:"association"`
}
type SingleCallToolActionResultV2 struct {
	ContractVersion string                                 `json:"contract_version"`
	Coordinate      SingleCallToolActionResultCoordinateV2 `json:"coordinate"`
	Digest          core.Digest                            `json:"digest"`
}
type SingleCallToolActionResultRefV2 struct {
	ID                     string        `json:"id"`
	Revision               core.Revision `json:"revision"`
	Digest                 core.Digest   `json:"digest"`
	RequestID              string        `json:"request_id"`
	RequestRevision        core.Revision `json:"request_revision"`
	RequestDigest          core.Digest   `json:"request_digest"`
	ActionCoordinateDigest core.Digest   `json:"action_coordinate_digest"`
	ToolResultID           string        `json:"tool_result_id"`
	ToolResultRevision     core.Revision `json:"tool_result_revision"`
	ToolResultDigest       core.Digest   `json:"tool_result_digest"`
}

func (v SingleCallToolActionResultCoordinateV2) subjectDigestV2() (core.Digest, error) {
	subject := SingleCallToolActionResultCoordinateSubjectV2{RequestID: v.RequestID, RequestRevision: v.RequestRevision, RequestDigest: v.RequestDigest, ActionCoordinateDigest: v.ActionCoordinateDigest, ToolResultID: v.ToolResult.ID, ToolResultRevision: v.ToolResult.Revision, ToolResultDigest: v.ToolResult.Digest, InspectionDigest: v.Inspection.Digest, Association: v.Association}
	return digestV2("praxis.application.single-call-tool-action-result-coordinate-id-v2", "SingleCallToolActionResultCoordinateSubjectV2", subject)
}

func (v SingleCallToolActionResultCoordinateV2) DigestV2() (core.Digest, error) {
	v.Digest = ""
	return digestV2("praxis.application.single-call-tool-action-result-coordinate-v2", "SingleCallToolActionResultCoordinateV2", v)
}

func (v SingleCallToolActionResultCoordinateV2) ValidateCurrentFor(request SingleCallToolActionRequestV2, now time.Time) error {
	if err := request.ValidateCurrent(now); err != nil {
		return err
	}
	if v.ContractVersion != SingleCallToolActionResultCoordinateVersionV2 || !validSingleCallIDV2(v.ID) || v.Revision != 1 || v.RequestID != request.ID || v.RequestRevision != request.Revision || v.RequestDigest != request.Digest || v.ActionCoordinateDigest != request.Action.Digest || v.AssociationCheckedUnixNano <= 0 || v.ExpiresUnixNano <= v.AssociationCheckedUnixNano || v.ExpiresUnixNano > request.ExpiresUnixNano || now.UnixNano() < v.AssociationCheckedUnixNano || !now.Before(time.Unix(0, v.ExpiresUnixNano)) {
		return core.NewError(core.ErrorConflict, core.ReasonEffectSettlementMissing, "single-call result coordinate is incomplete or stale")
	}
	if err := v.ToolResult.Validate(now); err != nil {
		return err
	}
	if err := v.Inspection.Validate(now); err != nil {
		return err
	}
	if err := v.Association.Validate(); err != nil {
		return err
	}
	if v.ToolResult.Inspection.Digest != v.Inspection.Digest || !runtimeports.SameOperationSettlementEvidenceAssociationRefV4(v.Inspection.Association, v.Association) || v.ToolResult.Schema != v.Inspection.DomainResult.Schema || v.ToolResult.PayloadDigest != v.Inspection.DomainResult.PayloadDigest || v.ToolResult.PayloadRevision != v.Inspection.DomainResult.PayloadRevision || v.ToolResult.ActionID != request.Action.PendingSubject.PendingActionRef || v.ToolResult.ActionDigest != request.Action.Digest || !runtimeports.SameExecutionScopeV2(v.Inspection.DomainResult.Operation.ExecutionScope, request.Action.ExecutionScope) {
		return core.NewError(core.ErrorConflict, core.ReasonEffectSettlementMissing, "single-call Tool result and Runtime settlement are not exact")
	}
	subjectDigest, err := v.subjectDigestV2()
	if err != nil || v.ID != "single-call-result-coordinate:v2:"+strings.TrimPrefix(string(subjectDigest), "sha256:") {
		return core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "single-call result coordinate ID drifted")
	}
	digest, err := v.DigestV2()
	if err != nil || digest != v.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "single-call result coordinate digest drifted")
	}
	return nil
}

func SealSingleCallToolActionResultCoordinateV2(v SingleCallToolActionResultCoordinateV2, request SingleCallToolActionRequestV2, now time.Time) (SingleCallToolActionResultCoordinateV2, error) {
	v.ContractVersion = SingleCallToolActionResultCoordinateVersionV2
	v.Revision = 1
	v.RequestID = request.ID
	v.RequestRevision = request.Revision
	v.RequestDigest = request.Digest
	v.ActionCoordinateDigest = request.Action.Digest
	v.ID = ""
	v.Digest = ""
	subjectDigest, err := v.subjectDigestV2()
	if err != nil {
		return SingleCallToolActionResultCoordinateV2{}, err
	}
	v.ID = "single-call-result-coordinate:v2:" + strings.TrimPrefix(string(subjectDigest), "sha256:")
	digest, err := v.DigestV2()
	if err != nil {
		return SingleCallToolActionResultCoordinateV2{}, err
	}
	v.Digest = digest
	return v, v.ValidateCurrentFor(request, now)
}

func (v SingleCallToolActionResultV2) DigestV2() (core.Digest, error) {
	v.Digest = ""
	return digestV2("praxis.application.single-call-tool-action-result-v2", "SingleCallToolActionResultV2", v)
}

func SealSingleCallToolActionResultV2(v SingleCallToolActionResultV2, request SingleCallToolActionRequestV2, now time.Time) (SingleCallToolActionResultV2, error) {
	v.ContractVersion = SingleCallToolActionResultContractVersionV2
	v.Digest = ""
	digest, err := v.DigestV2()
	if err != nil {
		return SingleCallToolActionResultV2{}, err
	}
	v.Digest = digest
	return v, v.ValidateCurrentFor(request, now)
}

func (v SingleCallToolActionResultRefV2) Validate() error {
	if !validSingleCallIDV2(v.ID) || v.Revision != 1 || !validSingleCallIDV2(v.RequestID) || v.RequestRevision != 1 || !validSingleCallIDV2(v.ToolResultID) || v.ToolResultRevision == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "single-call result ref is incomplete")
	}
	return validateDigestsV2(v.Digest, v.RequestDigest, v.ActionCoordinateDigest, v.ToolResultDigest)
}

func (v SingleCallToolActionResultV2) RefV2() SingleCallToolActionResultRefV2 {
	c := v.Coordinate
	return SingleCallToolActionResultRefV2{ID: c.ID, Revision: c.Revision, Digest: v.Digest, RequestID: c.RequestID, RequestRevision: c.RequestRevision, RequestDigest: c.RequestDigest, ActionCoordinateDigest: c.ActionCoordinateDigest, ToolResultID: c.ToolResult.ID, ToolResultRevision: c.ToolResult.Revision, ToolResultDigest: c.ToolResult.Digest}
}
func (v SingleCallToolActionResultV2) ValidateCurrentFor(r SingleCallToolActionRequestV2, now time.Time) error {
	if v.ContractVersion != SingleCallToolActionResultContractVersionV2 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "single-call result contract version is invalid")
	}
	if err := v.Coordinate.ValidateCurrentFor(r, now); err != nil {
		return err
	}
	digest, err := v.DigestV2()
	if err != nil || digest != v.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "single-call result digest drifted")
	}
	return nil
}

func CloneSingleCallToolActionResultV2(v SingleCallToolActionResultV2) SingleCallToolActionResultV2 {
	return v
}

type SingleCallToolActionInspectKeyV2 struct {
	ContractVersion        string        `json:"contract_version"`
	RequestID              string        `json:"request_id"`
	RequestRevision        core.Revision `json:"request_revision"`
	RequestDigest          core.Digest   `json:"request_digest"`
	ActionCoordinateDigest core.Digest   `json:"action_coordinate_digest"`
	ScopeDigest            core.Digest   `json:"scope_digest"`
	Digest                 core.Digest   `json:"digest"`
}

func (v SingleCallToolActionInspectKeyV2) DigestV2() (core.Digest, error) {
	v.Digest = ""
	return digestV2("praxis.application.single-call-tool-action-v2", "SingleCallToolActionInspectKeyV2", v)
}

func (v SingleCallToolActionInspectKeyV2) Validate() error {
	if v.ContractVersion != SingleCallToolActionContractVersionV2 || !validSingleCallIDV2(v.RequestID) || v.RequestRevision != 1 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "single-call inspect key is incomplete")
	}
	if err := validateDigestsV2(v.RequestDigest, v.ActionCoordinateDigest, v.ScopeDigest, v.Digest); err != nil {
		return err
	}
	digest, err := v.DigestV2()
	if err != nil || digest != v.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "single-call inspect key digest drifted")
	}
	return nil
}

func SealSingleCallToolActionInspectKeyV2(request SingleCallToolActionRequestV2) (SingleCallToolActionInspectKeyV2, error) {
	key := SingleCallToolActionInspectKeyV2{ContractVersion: SingleCallToolActionContractVersionV2, RequestID: request.ID, RequestRevision: request.Revision, RequestDigest: request.Digest, ActionCoordinateDigest: request.Action.Digest, ScopeDigest: request.Action.ExecutionScopeDigest}
	digest, err := key.DigestV2()
	if err != nil {
		return SingleCallToolActionInspectKeyV2{}, err
	}
	key.Digest = digest
	return key, key.Validate()
}

type SingleCallToolActionCoordinationStateV2 string

const (
	SingleCallToolActionPreparedV2       SingleCallToolActionCoordinationStateV2 = "prepared"
	SingleCallToolActionDispatchIntentV2 SingleCallToolActionCoordinationStateV2 = "dispatch_intent"
	SingleCallToolActionWaitingInspectV2 SingleCallToolActionCoordinationStateV2 = "waiting_inspect"
	SingleCallToolActionCompletedV2      SingleCallToolActionCoordinationStateV2 = "completed"
)

type SingleCallToolActionCoordinationFactV2 struct {
	ContractVersion string                                  `json:"contract_version"`
	ID              string                                  `json:"id"`
	Revision        core.Revision                           `json:"revision"`
	State           SingleCallToolActionCoordinationStateV2 `json:"state"`
	StartClaimID    string                                  `json:"start_claim_id,omitempty"`
	Request         SingleCallToolActionRequestV2           `json:"request"`
	Result          *SingleCallToolActionResultRefV2        `json:"result,omitempty"`
	CreatedUnixNano int64                                   `json:"created_unix_nano"`
	UpdatedUnixNano int64                                   `json:"updated_unix_nano"`
	Digest          core.Digest                             `json:"digest"`
}

func (v SingleCallToolActionCoordinationFactV2) bodyDigestV2() (core.Digest, error) {
	v.Digest = ""
	return digestV2("praxis.application.single-call-coordination-v2", "SingleCallToolActionCoordinationFactV2", v)
}
func (v SingleCallToolActionCoordinationFactV2) Validate() error {
	if v.ContractVersion != SingleCallToolActionCoordinationVersionV2 || v.ID != v.Request.ID || v.Revision == 0 || v.Request.Validate() != nil || v.CreatedUnixNano != v.Request.CreatedUnixNano || v.UpdatedUnixNano < v.CreatedUnixNano {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "single-call coordination fact is invalid")
	}
	switch v.State {
	case SingleCallToolActionPreparedV2, SingleCallToolActionDispatchIntentV2:
		if v.StartClaimID != "" || v.Result != nil {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "pre-dispatch fact contains terminal data")
		}
	case SingleCallToolActionWaitingInspectV2:
		expected, err := DeriveSingleCallToolActionStartClaimIDV2(v.Request)
		if err != nil || v.StartClaimID != expected || v.Result != nil {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "waiting fact is invalid")
		}
	case SingleCallToolActionCompletedV2:
		expected, err := DeriveSingleCallToolActionStartClaimIDV2(v.Request)
		if err != nil || v.StartClaimID != expected || v.Result == nil || v.Result.Validate() != nil || v.Result.RequestID != v.Request.ID || v.Result.RequestRevision != v.Request.Revision || v.Result.RequestDigest != v.Request.Digest || v.Result.ActionCoordinateDigest != v.Request.Action.Digest {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "completed fact is invalid")
		}
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "unknown coordination state")
	}
	d, e := v.bodyDigestV2()
	if e != nil || d != v.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "coordination fact digest drifted")
	}
	return nil
}
func sealCoordinationV2(v SingleCallToolActionCoordinationFactV2) (SingleCallToolActionCoordinationFactV2, error) {
	v.Digest = ""
	d, e := v.bodyDigestV2()
	if e != nil {
		return SingleCallToolActionCoordinationFactV2{}, e
	}
	v.Digest = d
	return v, v.Validate()
}
func NewSingleCallToolActionCoordinationFactV2(r SingleCallToolActionRequestV2) (SingleCallToolActionCoordinationFactV2, error) {
	v := SingleCallToolActionCoordinationFactV2{ContractVersion: SingleCallToolActionCoordinationVersionV2, ID: r.ID, Revision: 1, State: SingleCallToolActionPreparedV2, Request: r, CreatedUnixNano: r.CreatedUnixNano, UpdatedUnixNano: r.CreatedUnixNano}
	return sealCoordinationV2(v)
}
func NextSingleCallToolActionCoordinationFactV2(cur SingleCallToolActionCoordinationFactV2, state SingleCallToolActionCoordinationStateV2, result *SingleCallToolActionResultRefV2, now time.Time) (SingleCallToolActionCoordinationFactV2, error) {
	if err := cur.Validate(); err != nil {
		return SingleCallToolActionCoordinationFactV2{}, err
	}
	if now.IsZero() || now.UnixNano() < cur.UpdatedUnixNano {
		return SingleCallToolActionCoordinationFactV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "coordination clock regressed")
	}
	if cur.State != SingleCallToolActionPreparedV2 || state != SingleCallToolActionDispatchIntentV2 || result != nil {
		return SingleCallToolActionCoordinationFactV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "coordination transition is invalid")
	}
	next := cur
	next.Revision++
	next.State = state
	next.Result = result
	next.UpdatedUnixNano = now.UnixNano()
	return sealCoordinationV2(next)
}

// CompleteSingleCallToolActionCoordinationFactV2 is the only constructor for a
// completed Fact. It accepts the full settled Result, validates its current
// owner bindings, and derives the stored Ref itself; callers cannot seal an
// arbitrary format-valid ResultRef into the terminal state.
func CompleteSingleCallToolActionCoordinationFactV2(cur SingleCallToolActionCoordinationFactV2, result SingleCallToolActionResultV2, now time.Time) (SingleCallToolActionCoordinationFactV2, error) {
	if err := cur.Validate(); err != nil {
		return SingleCallToolActionCoordinationFactV2{}, err
	}
	if cur.State != SingleCallToolActionWaitingInspectV2 {
		return SingleCallToolActionCoordinationFactV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "coordination completion requires waiting_inspect")
	}
	if now.IsZero() || now.UnixNano() < cur.UpdatedUnixNano {
		return SingleCallToolActionCoordinationFactV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "coordination clock regressed")
	}
	if err := result.ValidateCurrentFor(cur.Request, now); err != nil {
		return SingleCallToolActionCoordinationFactV2{}, err
	}
	ref := result.RefV2()
	if err := ref.Validate(); err != nil {
		return SingleCallToolActionCoordinationFactV2{}, err
	}
	next := cur
	next.Revision++
	next.State = SingleCallToolActionCompletedV2
	next.Result = &ref
	next.UpdatedUnixNano = now.UnixNano()
	return sealCoordinationV2(next)
}
func ClaimSingleCallToolActionStartV2(cur SingleCallToolActionCoordinationFactV2, claim string, now time.Time) (SingleCallToolActionCoordinationFactV2, error) {
	if err := cur.Validate(); err != nil {
		return SingleCallToolActionCoordinationFactV2{}, err
	}
	expected, deriveErr := DeriveSingleCallToolActionStartClaimIDV2(cur.Request)
	if cur.State != SingleCallToolActionDispatchIntentV2 || deriveErr != nil || claim != expected {
		return SingleCallToolActionCoordinationFactV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "start claim is invalid")
	}
	if now.IsZero() || now.UnixNano() < cur.UpdatedUnixNano {
		return SingleCallToolActionCoordinationFactV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "coordination clock regressed")
	}
	next := cur
	next.Revision++
	next.State = SingleCallToolActionWaitingInspectV2
	next.StartClaimID = claim
	next.UpdatedUnixNano = now.UnixNano()
	return sealCoordinationV2(next)
}

func DeriveSingleCallToolActionStartClaimIDV2(request SingleCallToolActionRequestV2) (string, error) {
	if err := request.Validate(); err != nil {
		return "", err
	}
	digest, err := core.CanonicalJSONDigest("praxis.application.single-call-tool-action-start-claim-v2", SingleCallToolActionContractVersionV2, "SingleCallToolActionStartClaimSubjectV2", struct {
		RequestID     string      `json:"request_id"`
		RequestDigest core.Digest `json:"request_digest"`
		ActionDigest  core.Digest `json:"action_digest"`
	}{request.ID, request.Digest, request.Action.Digest})
	if err != nil {
		return "", err
	}
	return "single-call-start:v2:" + strings.TrimPrefix(string(digest), "sha256:"), nil
}

func ValidateSingleCallToolActionCoordinationTransitionV2(current, next SingleCallToolActionCoordinationFactV2) error {
	if err := current.Validate(); err != nil {
		return err
	}
	if err := next.Validate(); err != nil {
		return err
	}
	if current.ID != next.ID || current.Request.Digest != next.Request.Digest || current.CreatedUnixNano != next.CreatedUnixNano || next.Revision != current.Revision+1 || next.UpdatedUnixNano < current.UpdatedUnixNano {
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "single-call coordination immutable fields or revision drifted")
	}
	valid := current.State == SingleCallToolActionPreparedV2 && next.State == SingleCallToolActionDispatchIntentV2 || current.State == SingleCallToolActionDispatchIntentV2 && next.State == SingleCallToolActionWaitingInspectV2 || current.State == SingleCallToolActionWaitingInspectV2 && next.State == SingleCallToolActionCompletedV2
	if !valid {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "single-call coordination transition is invalid")
	}
	if current.StartClaimID != "" && current.StartClaimID != next.StartClaimID {
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "single-call start claim changed")
	}
	return nil
}

type SingleCallToolActionCrossVersionConflictKeyV1 struct {
	ContractVersion            string          `json:"contract_version"`
	ExecutionScopeDigest       core.Digest     `json:"execution_scope_digest"`
	RunID                      core.AgentRunID `json:"run_id"`
	SessionID                  string          `json:"session_id"`
	Turn                       uint32          `json:"turn"`
	PendingActionRef           string          `json:"pending_action_ref"`
	PendingActionRequestDigest core.Digest     `json:"pending_action_request_digest"`
	Digest                     core.Digest     `json:"digest"`
}

func (v SingleCallToolActionCrossVersionConflictKeyV1) Validate() error {
	if v.ContractVersion != "praxis.application.single-call-tool-action-cross-version-key/v1" || v.ExecutionScopeDigest.Validate() != nil || v.RunID == "" || !validSingleCallIDV2(v.SessionID) || !validSingleCallIDV2(v.PendingActionRef) || v.PendingActionRequestDigest.Validate() != nil || v.Digest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "single-call cross-version conflict key is invalid")
	}
	copy := v
	copy.Digest = ""
	digest, err := digestV2("praxis.application.single-call-tool-action-cross-version-key-v1", "SingleCallToolActionCrossVersionConflictKeyV1", copy)
	if err != nil || digest != v.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "single-call cross-version conflict key digest drifted")
	}
	return nil
}

func CrossVersionConflictKeyForRequestV2(r SingleCallToolActionRequestV2) (SingleCallToolActionCrossVersionConflictKeyV1, error) {
	if err := r.Validate(); err != nil {
		return SingleCallToolActionCrossVersionConflictKeyV1{}, err
	}
	k := SingleCallToolActionCrossVersionConflictKeyV1{ContractVersion: "praxis.application.single-call-tool-action-cross-version-key/v1", ExecutionScopeDigest: r.Action.ExecutionScopeDigest, RunID: r.Action.PendingSubject.Run.RunID, SessionID: r.Action.PendingSubject.SessionID, Turn: r.Action.PendingSubject.Turn, PendingActionRef: r.Action.PendingSubject.PendingActionRef, PendingActionRequestDigest: r.Action.PendingSubject.Binding.Base.PendingAction.RequestDigest}
	d, e := digestV2("praxis.application.single-call-tool-action-cross-version-key-v1", "SingleCallToolActionCrossVersionConflictKeyV1", k)
	if e != nil {
		return k, e
	}
	k.Digest = d
	return k, nil
}

func DeriveSingleCallToolActionCrossVersionConflictKeyV1(r SingleCallToolActionRequestV2) (SingleCallToolActionCrossVersionConflictKeyV1, error) {
	return CrossVersionConflictKeyForRequestV2(r)
}

type SingleCallToolActionVersionClaimV1 struct {
	ContractVersion      string                                        `json:"contract_version"`
	ConflictKey          SingleCallToolActionCrossVersionConflictKeyV1 `json:"conflict_key"`
	ClaimedActionVersion string                                        `json:"claimed_action_version"`
	CoordinationID       string                                        `json:"coordination_id"`
	CoordinationDigest   core.Digest                                   `json:"coordination_digest"`
	Revision             core.Revision                                 `json:"revision"`
	CreatedUnixNano      int64                                         `json:"created_unix_nano"`
	Digest               core.Digest                                   `json:"digest"`
}

func NewSingleCallToolActionVersionClaimV1(f SingleCallToolActionCoordinationFactV2) (SingleCallToolActionVersionClaimV1, error) {
	if err := f.Validate(); err != nil {
		return SingleCallToolActionVersionClaimV1{}, err
	}
	if f.Revision != 1 || f.State != SingleCallToolActionPreparedV2 {
		return SingleCallToolActionVersionClaimV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "version claim requires initial prepared fact")
	}
	k, e := CrossVersionConflictKeyForRequestV2(f.Request)
	if e != nil {
		return SingleCallToolActionVersionClaimV1{}, e
	}
	return SealSingleCallToolActionVersionClaimV1(SingleCallToolActionVersionClaimV1{ContractVersion: "praxis.application.single-call-tool-action-version-claim/v1", ConflictKey: k, ClaimedActionVersion: f.Request.ContractVersion, CoordinationID: f.ID, CoordinationDigest: f.Digest, Revision: 1, CreatedUnixNano: f.CreatedUnixNano})
}

func (v SingleCallToolActionVersionClaimV1) Validate() error {
	if v.ContractVersion != "praxis.application.single-call-tool-action-version-claim/v1" || v.ConflictKey.Validate() != nil || v.ClaimedActionVersion != SingleCallToolActionContractVersionV1 && v.ClaimedActionVersion != SingleCallToolActionContractVersionV2 || !validSingleCallIDV2(v.CoordinationID) || v.CoordinationDigest.Validate() != nil || v.Revision != 1 || v.CreatedUnixNano <= 0 || v.Digest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "single-call version claim is invalid")
	}
	copy := v
	copy.Digest = ""
	digest, err := digestV2("praxis.application.single-call-tool-action-version-claim-v1", "SingleCallToolActionVersionClaimV1", copy)
	if err != nil || digest != v.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "single-call version claim digest drifted")
	}
	return nil
}

func SealSingleCallToolActionVersionClaimV1(v SingleCallToolActionVersionClaimV1) (SingleCallToolActionVersionClaimV1, error) {
	v.ContractVersion = "praxis.application.single-call-tool-action-version-claim/v1"
	v.Revision = 1
	v.Digest = ""
	digest, err := digestV2("praxis.application.single-call-tool-action-version-claim-v1", "SingleCallToolActionVersionClaimV1", v)
	if err != nil {
		return SingleCallToolActionVersionClaimV1{}, err
	}
	v.Digest = digest
	return v, v.Validate()
}

func (v SingleCallToolActionVersionClaimV1) ValidateFor(initial SingleCallToolActionCoordinationFactV2) error {
	if err := initial.Validate(); err != nil {
		return err
	}
	if err := v.Validate(); err != nil {
		return err
	}
	if initial.Revision != 1 || initial.State != SingleCallToolActionPreparedV2 || v.ContractVersion != "praxis.application.single-call-tool-action-version-claim/v1" || v.ClaimedActionVersion != initial.Request.ContractVersion || v.ClaimedActionVersion != SingleCallToolActionContractVersionV2 || v.CoordinationID != initial.ID || v.CoordinationDigest != initial.Digest || v.Revision != 1 || v.CreatedUnixNano != initial.CreatedUnixNano {
		return core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "single-call version claim does not bind the initial prepared fact")
	}
	expectedKey, err := DeriveSingleCallToolActionCrossVersionConflictKeyV1(initial.Request)
	if err != nil {
		return err
	}
	if v.ConflictKey != expectedKey {
		return core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "single-call version claim conflict key drifted")
	}
	return nil
}
