package contract

import (
	"reflect"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const (
	CommittedPendingActionOwnerCurrentInputsContractVersionV1 = "praxis.harness.committed-pending-action-owner-current-inputs/v1"
	PendingActionApplicationBindingContractVersionV2          = "praxis.harness.pending-action-application-binding/v2"
	CommittedPendingActionCurrentContractVersionV3            = "praxis.harness.committed-pending-action-current/v3"
	committedPendingActionOwnerInputsDomainV1                 = "praxis.harness.committed-pending-action-owner-current-inputs"
	pendingActionApplicationBindingDomainV2                   = "praxis.harness.pending-action-application-binding"
	committedPendingActionCurrentDomainV3                     = "praxis.harness.committed-pending-action-current"
	committedPendingActionSessionCoordinateDomainV3           = "praxis.harness.committed-pending-action-session-coordinate-v3"
	committedPendingActionTurnCoordinateDomainV3              = "praxis.harness.committed-pending-action-turn-coordinate-v3"
)

type CommittedPendingActionOwnerCurrentInputsV1 struct {
	ContractVersion              string                                                      `json:"contract_version"`
	ModelTurnOperation           runtimeports.OperationSubjectV3                             `json:"model_turn_operation"`
	GenerationBindingAssociation runtimeports.GenerationBindingAssociationRefV1              `json:"generation_binding_association"`
	RouteCurrent                 runtimeports.ControlledOperationProviderRouteCurrentRefV2   `json:"route_current"`
	RouteMatrix                  runtimeports.OperationScopeEvidenceApplicabilityMatrixKeyV3 `json:"route_matrix"`
	ContextApplicability         runtimeports.OperationScopeEvidenceApplicabilityFactRefV3   `json:"context_applicability"`
	Digest                       core.Digest                                                 `json:"digest"`
}

func (v CommittedPendingActionOwnerCurrentInputsV1) Clone() CommittedPendingActionOwnerCurrentInputsV1 {
	clone := v
	clone.ModelTurnOperation.ExecutionScope = cloneExecutionScopeV3(v.ModelTurnOperation.ExecutionScope)
	return clone
}

func (v CommittedPendingActionOwnerCurrentInputsV1) DigestV1() (core.Digest, error) {
	copy := v.Clone()
	copy.Digest = ""
	return core.CanonicalJSONDigest(committedPendingActionOwnerInputsDomainV1, CommittedPendingActionOwnerCurrentInputsContractVersionV1, "CommittedPendingActionOwnerCurrentInputsV1", copy)
}

func (v CommittedPendingActionOwnerCurrentInputsV1) Validate() error {
	if v.ContractVersion != CommittedPendingActionOwnerCurrentInputsContractVersionV1 || v.ModelTurnOperation.Validate() != nil || v.ModelTurnOperation.Kind != runtimeports.OperationScopeRunV3 || v.GenerationBindingAssociation.Validate() != nil || v.RouteCurrent.Validate() != nil || v.RouteMatrix.Validate() != nil || v.ContextApplicability.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "committed PendingAction owner-current inputs are incomplete")
	}
	if v.ContextApplicability.Kind != runtimeports.OperationScopeEvidenceContextParentKindV3 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonEvidenceScopeConflict, "committed PendingAction context applicability kind is not parent-frame current")
	}
	action := runtimeports.OperationScopeEvidenceActionMatrixV3()
	if v.RouteMatrix != action {
		return core.NewError(core.ErrorConflict, core.ReasonUnknownGovernanceCategory, "committed PendingAction route matrix is not the closed action matrix")
	}
	matrixDigest, err := runtimeports.DigestOperationScopeEvidenceApplicabilityMatrixKeyV3(v.RouteMatrix)
	if err != nil || matrixDigest != v.RouteCurrent.MatrixDigest {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "committed PendingAction route matrix digest drifted")
	}
	digest, err := v.DigestV1()
	if err != nil || digest != v.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "committed PendingAction owner-current inputs digest drifted")
	}
	return nil
}

func SealCommittedPendingActionOwnerCurrentInputsV1(v CommittedPendingActionOwnerCurrentInputsV1) (CommittedPendingActionOwnerCurrentInputsV1, error) {
	v = v.Clone()
	v.ContractVersion = CommittedPendingActionOwnerCurrentInputsContractVersionV1
	v.Digest = ""
	digest, err := v.DigestV1()
	if err != nil {
		return CommittedPendingActionOwnerCurrentInputsV1{}, err
	}
	v.Digest = digest
	return v.Clone(), v.Validate()
}

type PendingActionApplicationBindingV2 struct {
	ContractVersion    string                                     `json:"contract_version"`
	Base               PendingActionApplicationBindingV1          `json:"base"`
	OwnerCurrentInputs CommittedPendingActionOwnerCurrentInputsV1 `json:"owner_current_inputs"`
	Digest             core.Digest                                `json:"digest"`
}

func (v PendingActionApplicationBindingV2) Clone() PendingActionApplicationBindingV2 {
	clone := v
	clone.Base = v.Base.Clone()
	clone.OwnerCurrentInputs = v.OwnerCurrentInputs.Clone()
	return clone
}

func (v PendingActionApplicationBindingV2) DigestV2() (core.Digest, error) {
	copy := v.Clone()
	copy.Digest = ""
	return core.CanonicalJSONDigest(pendingActionApplicationBindingDomainV2, PendingActionApplicationBindingContractVersionV2, "PendingActionApplicationBindingV2", copy)
}

func (v PendingActionApplicationBindingV2) Validate() error {
	if v.ContractVersion != PendingActionApplicationBindingContractVersionV2 || v.Base.Validate() != nil || v.OwnerCurrentInputs.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "PendingAction Application binding V2 is incomplete")
	}
	operationDigest, err := v.OwnerCurrentInputs.ModelTurnOperation.DigestV3()
	if err != nil || operationDigest != v.Base.ModelTurnSettlementRef.Attempt.OperationDigest {
		return core.NewError(core.ErrorConflict, core.ReasonSettlementOwnerMismatch, "PendingAction Application binding V2 operation and Settlement attempt differ")
	}
	digest, err := v.DigestV2()
	if err != nil || digest != v.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "PendingAction Application binding V2 digest drifted")
	}
	return nil
}

func SealPendingActionApplicationBindingV2(v PendingActionApplicationBindingV2) (PendingActionApplicationBindingV2, error) {
	v = v.Clone()
	v.ContractVersion = PendingActionApplicationBindingContractVersionV2
	v.Digest = ""
	digest, err := v.DigestV2()
	if err != nil {
		return PendingActionApplicationBindingV2{}, err
	}
	v.Digest = digest
	return v.Clone(), v.Validate()
}

type CommittedPendingActionSubjectV3 struct {
	Base               CommittedPendingActionSubjectV2   `json:"base"`
	ApplicationBinding PendingActionApplicationBindingV2 `json:"application_binding"`
}

func (s CommittedPendingActionSubjectV3) Clone() CommittedPendingActionSubjectV3 {
	return CommittedPendingActionSubjectV3{Base: s.Base.Clone(), ApplicationBinding: s.ApplicationBinding.Clone()}
}

func (s CommittedPendingActionSubjectV3) Validate() error {
	if s.Base.Validate() != nil || s.ApplicationBinding.Validate() != nil || !samePendingActionBindingBaseExactV3(s.Base, s.ApplicationBinding.Base) {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "committed PendingAction V3 subject and binding differ")
	}
	op := s.ApplicationBinding.OwnerCurrentInputs.ModelTurnOperation
	if op.RunID != s.Base.Run.RunID || op.ExecutionScopeDigest != s.Base.ExecutionScopeDigest || !runtimeports.SameExecutionScopeV2(op.ExecutionScope, s.Base.Run.Scope) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectFenceStale, "committed PendingAction V3 operation and Session scope differ")
	}
	return nil
}

type CommittedPendingActionCurrentRequestV3 struct {
	Subject                   CommittedPendingActionSubjectV3 `json:"subject"`
	RequestedNotAfterUnixNano int64                           `json:"requested_not_after_unix_nano"`
}

func (r CommittedPendingActionCurrentRequestV3) Clone() CommittedPendingActionCurrentRequestV3 {
	return CommittedPendingActionCurrentRequestV3{Subject: r.Subject.Clone(), RequestedNotAfterUnixNano: r.RequestedNotAfterUnixNano}
}

func (r CommittedPendingActionCurrentRequestV3) Validate(now time.Time) error {
	if now.IsZero() || r.RequestedNotAfterUnixNano < 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "committed PendingAction V3 request time or upper bound is invalid")
	}
	if err := r.Subject.Validate(); err != nil {
		return err
	}
	if r.RequestedNotAfterUnixNano > 0 && r.RequestedNotAfterUnixNano <= now.UnixNano() {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonCapabilityExpired, "committed PendingAction V3 requested upper bound is not current")
	}
	return nil
}

type CommittedPendingActionCurrentV3 struct {
	ContractVersion      string                                                 `json:"contract_version"`
	Run                  RunRef                                                 `json:"run"`
	ExecutionScopeDigest core.Digest                                            `json:"execution_scope_digest"`
	SessionID            string                                                 `json:"session_id"`
	SessionRevision      core.Revision                                          `json:"session_revision"`
	SessionDigest        core.Digest                                            `json:"session_digest"`
	Phase                SessionPhaseV2                                         `json:"phase"`
	Turn                 uint32                                                 `json:"turn"`
	PendingAction        PendingActionV2                                        `json:"pending_action"`
	ApplicationBinding   PendingActionApplicationBindingV2                      `json:"application_binding"`
	SessionApplicability CommittedPendingActionSessionApplicabilityCoordinateV1 `json:"session_applicability"`
	TurnApplicability    CommittedPendingActionTurnApplicabilityCoordinateV1    `json:"turn_applicability"`
	CheckedUnixNano      int64                                                  `json:"checked_unix_nano"`
	ExpiresUnixNano      int64                                                  `json:"expires_unix_nano"`
	Digest               core.Digest                                            `json:"digest"`
}

func (p CommittedPendingActionCurrentV3) Clone() CommittedPendingActionCurrentV3 {
	clone := p
	clone.Run.Scope = cloneExecutionScopeV3(p.Run.Scope)
	clone.PendingAction.Payload.Inline = append([]byte(nil), p.PendingAction.Payload.Inline...)
	clone.ApplicationBinding = p.ApplicationBinding.Clone()
	return clone
}

func (p CommittedPendingActionCurrentV3) SubjectV3() CommittedPendingActionSubjectV3 {
	return CommittedPendingActionSubjectV3{Base: CommittedPendingActionSubjectV2{ExecutionScopeDigest: p.ExecutionScopeDigest, Run: p.Run, SessionID: p.SessionID, SessionRevision: p.SessionRevision, SessionDigest: p.SessionDigest, Turn: p.Turn, PendingActionRef: p.PendingAction.Ref, IdentityRef: p.ApplicationBinding.Base.IdentityRef, DomainResultFactRef: p.ApplicationBinding.Base.DomainResultFactRef, ModelTurnSettlement: p.ApplicationBinding.Base.ModelTurnSettlementRef}, ApplicationBinding: p.ApplicationBinding}.Clone()
}

func (p CommittedPendingActionCurrentV3) DigestV3() (core.Digest, error) {
	copy := p.Clone()
	copy.Digest = ""
	return core.CanonicalJSONDigest(committedPendingActionCurrentDomainV3, CommittedPendingActionCurrentContractVersionV3, "CommittedPendingActionCurrentV3", copy)
}

func (p CommittedPendingActionCurrentV3) Validate(now time.Time) error {
	if p.ContractVersion != CommittedPendingActionCurrentContractVersionV3 || p.Phase != SessionWaitingActionV2 || now.IsZero() || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano || now.UnixNano() < p.CheckedUnixNano || now.UnixNano() >= p.ExpiresUnixNano || p.SubjectV3().Validate() != nil || p.PendingAction.Validate() != nil || !samePendingActionV3(p.PendingAction, p.ApplicationBinding.Base.PendingAction) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectFenceStale, "committed PendingAction V3 projection is invalid or expired")
	}
	session, err := sessionApplicabilityCoordinateV3(p)
	if err != nil {
		return err
	}
	turn, err := turnApplicabilityCoordinateV3(p, session)
	if err != nil || p.SessionApplicability != session || p.TurnApplicability != turn {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidReference, "committed PendingAction V3 applicability coordinates drifted")
	}
	digest, err := p.DigestV3()
	if err != nil || digest != p.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "committed PendingAction V3 projection digest drifted")
	}
	return nil
}

func (p CommittedPendingActionCurrentV3) ValidateAgainst(expected CommittedPendingActionCurrentRequestV3, now time.Time) error {
	if err := expected.Validate(now); err != nil {
		return err
	}
	if err := p.Validate(now); err != nil {
		return err
	}
	if !sameCommittedPendingActionSubjectExactV3(p.SubjectV3(), expected.Subject) {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "committed PendingAction V3 projection belongs to another subject")
	}
	if expected.RequestedNotAfterUnixNano > 0 && p.ExpiresUnixNano > expected.RequestedNotAfterUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonCapabilityExpired, "committed PendingAction V3 projection exceeded requested upper bound")
	}
	return nil
}

func SealCommittedPendingActionCurrentV3(p CommittedPendingActionCurrentV3, expected CommittedPendingActionCurrentRequestV3, now time.Time) (CommittedPendingActionCurrentV3, error) {
	p = p.Clone()
	p.ContractVersion = CommittedPendingActionCurrentContractVersionV3
	var err error
	p.SessionApplicability, err = sessionApplicabilityCoordinateV3(p)
	if err != nil {
		return CommittedPendingActionCurrentV3{}, err
	}
	p.TurnApplicability, err = turnApplicabilityCoordinateV3(p, p.SessionApplicability)
	if err != nil {
		return CommittedPendingActionCurrentV3{}, err
	}
	p.Digest = ""
	p.Digest, err = p.DigestV3()
	if err != nil {
		return CommittedPendingActionCurrentV3{}, err
	}
	return p.Clone(), p.ValidateAgainst(expected, now)
}

func samePendingActionBindingBaseExactV3(subject CommittedPendingActionSubjectV2, base PendingActionApplicationBindingV1) bool {
	return subject.PendingActionRef == base.PendingAction.Ref && subject.IdentityRef == base.IdentityRef && subject.DomainResultFactRef == base.DomainResultFactRef && reflect.DeepEqual(subject.ModelTurnSettlement, base.ModelTurnSettlementRef)
}

func sameCommittedPendingActionSubjectExactV3(a, b CommittedPendingActionSubjectV3) bool {
	return sameCommittedPendingActionSubjectExactV2(a.Base, b.Base) && reflect.DeepEqual(a.ApplicationBinding, b.ApplicationBinding)
}

func sessionApplicabilityCoordinateV3(p CommittedPendingActionCurrentV3) (CommittedPendingActionSessionApplicabilityCoordinateV1, error) {
	digest, err := core.CanonicalJSONDigest(committedPendingActionSessionCoordinateDomainV3, CommittedPendingActionCurrentContractVersionV3, "CommittedPendingActionSessionApplicabilityCoordinateV3", struct {
		Run           RunRef                            `json:"run"`
		Scope         core.Digest                       `json:"execution_scope_digest"`
		SessionID     string                            `json:"session_id"`
		Revision      core.Revision                     `json:"revision"`
		SessionDigest core.Digest                       `json:"session_digest"`
		Phase         SessionPhaseV2                    `json:"phase"`
		Turn          uint32                            `json:"turn"`
		PendingAction PendingActionV2                   `json:"pending_action"`
		Binding       PendingActionApplicationBindingV2 `json:"application_binding"`
	}{p.Run, p.ExecutionScopeDigest, p.SessionID, p.SessionRevision, p.SessionDigest, p.Phase, p.Turn, p.PendingAction, p.ApplicationBinding})
	if err != nil {
		return CommittedPendingActionSessionApplicabilityCoordinateV1{}, err
	}
	v := CommittedPendingActionSessionApplicabilityCoordinateV1{Kind: CommittedPendingActionSessionKindV1, ID: "session:" + string(digest), Revision: p.SessionRevision, Digest: digest}
	return v, v.Validate()
}

func turnApplicabilityCoordinateV3(p CommittedPendingActionCurrentV3, session CommittedPendingActionSessionApplicabilityCoordinateV1) (CommittedPendingActionTurnApplicabilityCoordinateV1, error) {
	digest, err := core.CanonicalJSONDigest(committedPendingActionTurnCoordinateDomainV3, CommittedPendingActionCurrentContractVersionV3, "CommittedPendingActionTurnApplicabilityCoordinateV3", struct {
		Session       CommittedPendingActionSessionApplicabilityCoordinateV1 `json:"session"`
		Turn          uint32                                                 `json:"turn"`
		PendingAction PendingActionV2                                        `json:"pending_action"`
		IdentityRef   ModelToolCallPendingActionIdentityRefV1                `json:"identity_ref"`
	}{session, p.Turn, p.PendingAction, p.ApplicationBinding.Base.IdentityRef})
	if err != nil {
		return CommittedPendingActionTurnApplicabilityCoordinateV1{}, err
	}
	v := CommittedPendingActionTurnApplicabilityCoordinateV1{Kind: CommittedPendingActionTurnKindV1, ID: "turn:" + string(digest), Revision: core.Revision(p.Turn), Digest: digest}
	return v, v.Validate()
}
