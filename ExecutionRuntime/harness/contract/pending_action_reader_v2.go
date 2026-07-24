package contract

import (
	"reflect"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const (
	CommittedPendingActionCurrentContractVersionV2  = "praxis.harness.committed-pending-action-current/v2"
	committedPendingActionCurrentCanonicalDomainV2  = "praxis.harness.committed-pending-action-current"
	committedPendingActionSessionCoordinateDomainV2 = "praxis.harness.committed-pending-action-session-coordinate-v2"
	committedPendingActionTurnCoordinateDomainV2    = "praxis.harness.committed-pending-action-turn-coordinate-v2"
)

// CommittedPendingActionSubjectV2 deliberately has no ContractVersion. It is
// one exact Harness-owned subject, not a versioned Application DTO.
type CommittedPendingActionSubjectV2 struct {
	ExecutionScopeDigest core.Digest                             `json:"execution_scope_digest"`
	Run                  RunRef                                  `json:"run"`
	SessionID            string                                  `json:"session_id"`
	SessionRevision      core.Revision                           `json:"session_revision"`
	SessionDigest        core.Digest                             `json:"session_digest"`
	Turn                 uint32                                  `json:"turn"`
	PendingActionRef     string                                  `json:"pending_action_ref"`
	IdentityRef          ModelToolCallPendingActionIdentityRefV1 `json:"identity_ref"`
	DomainResultFactRef  SettledTurnDomainResultFactRefV3        `json:"domain_result_fact_ref"`
	ModelTurnSettlement  runtimeports.OperationSettlementRefV3   `json:"model_turn_settlement"`
}

func (s CommittedPendingActionSubjectV2) Clone() CommittedPendingActionSubjectV2 {
	clone := s
	clone.Run.Scope = cloneExecutionScopeV3(s.Run.Scope)
	clone.ModelTurnSettlement = cloneOperationSettlementRefV3(s.ModelTurnSettlement)
	return clone
}

func (s CommittedPendingActionSubjectV2) Validate() error {
	if strings.TrimSpace(s.SessionID) == "" || len(s.SessionID) > MaxReferenceBytes || s.SessionRevision == 0 || s.Turn == 0 || strings.TrimSpace(s.PendingActionRef) == "" || len(s.PendingActionRef) > MaxReferenceBytes {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "committed PendingAction V2 subject is incomplete")
	}
	if err := s.Run.Validate(); err != nil {
		return err
	}
	if err := s.ExecutionScopeDigest.Validate(); err != nil {
		return err
	}
	scopeDigest, err := runtimeports.ExecutionScopeDigestV2(s.Run.Scope)
	if err != nil || scopeDigest != s.ExecutionScopeDigest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectFenceStale, "committed PendingAction V2 Execution Scope digest drifted")
	}
	if err := s.SessionDigest.Validate(); err != nil {
		return err
	}
	if err := s.IdentityRef.Validate(); err != nil {
		return err
	}
	if err := s.DomainResultFactRef.Validate(); err != nil {
		return err
	}
	if err := s.ModelTurnSettlement.Validate(); err != nil {
		return err
	}
	if s.PendingActionRef != s.IdentityRef.PendingActionRef || s.DomainResultFactRef.IdentityRef != s.IdentityRef {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "committed PendingAction V2 subject refs were spliced")
	}
	if s.ModelTurnSettlement.DomainResultSchema == nil || *s.ModelTurnSettlement.DomainResultSchema != s.DomainResultFactRef.Schema || s.ModelTurnSettlement.DomainResultDigest != s.DomainResultFactRef.ContentDigest {
		return core.NewError(core.ErrorConflict, core.ReasonSettlementOwnerMismatch, "committed PendingAction V2 Settlement differs from DomainResult")
	}
	return nil
}

type CommittedPendingActionCurrentRequestV2 struct {
	Subject                   CommittedPendingActionSubjectV2 `json:"subject"`
	RequestedNotAfterUnixNano int64                           `json:"requested_not_after_unix_nano"`
}

func (r CommittedPendingActionCurrentRequestV2) Clone() CommittedPendingActionCurrentRequestV2 {
	clone := r
	clone.Subject = r.Subject.Clone()
	return clone
}

func (r CommittedPendingActionCurrentRequestV2) Validate(now time.Time) error {
	if now.IsZero() {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "committed PendingAction V2 validation time is required")
	}
	if err := r.Subject.Validate(); err != nil {
		return err
	}
	if r.RequestedNotAfterUnixNano < 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonCapabilityExpired, "requested observation upper bound cannot be negative")
	}
	if r.RequestedNotAfterUnixNano > 0 && r.RequestedNotAfterUnixNano <= now.UnixNano() {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonCapabilityExpired, "requested observation upper bound is not current")
	}
	return nil
}

type CommittedPendingActionCurrentV2 struct {
	ContractVersion      string                                                 `json:"contract_version"`
	Run                  RunRef                                                 `json:"run"`
	ExecutionScopeDigest core.Digest                                            `json:"execution_scope_digest"`
	SessionID            string                                                 `json:"session_id"`
	SessionRevision      core.Revision                                          `json:"session_revision"`
	SessionDigest        core.Digest                                            `json:"session_digest"`
	Phase                SessionPhaseV2                                         `json:"phase"`
	Turn                 uint32                                                 `json:"turn"`
	PendingAction        PendingActionV2                                        `json:"pending_action"`
	ApplicationBinding   PendingActionApplicationBindingV1                      `json:"application_binding"`
	SessionApplicability CommittedPendingActionSessionApplicabilityCoordinateV1 `json:"session_applicability"`
	TurnApplicability    CommittedPendingActionTurnApplicabilityCoordinateV1    `json:"turn_applicability"`
	CheckedUnixNano      int64                                                  `json:"checked_unix_nano"`
	ExpiresUnixNano      int64                                                  `json:"expires_unix_nano"`
	Digest               core.Digest                                            `json:"digest"`
}

func (p CommittedPendingActionCurrentV2) Clone() CommittedPendingActionCurrentV2 {
	clone := p
	clone.Run.Scope = cloneExecutionScopeV3(p.Run.Scope)
	clone.PendingAction.Payload.Inline = append([]byte(nil), p.PendingAction.Payload.Inline...)
	clone.ApplicationBinding = p.ApplicationBinding.Clone()
	return clone
}

func (p CommittedPendingActionCurrentV2) SubjectV2() CommittedPendingActionSubjectV2 {
	return CommittedPendingActionSubjectV2{
		ExecutionScopeDigest: p.ExecutionScopeDigest,
		Run:                  p.Run,
		SessionID:            p.SessionID,
		SessionRevision:      p.SessionRevision,
		SessionDigest:        p.SessionDigest,
		Turn:                 p.Turn,
		PendingActionRef:     p.PendingAction.Ref,
		IdentityRef:          p.ApplicationBinding.IdentityRef,
		DomainResultFactRef:  p.ApplicationBinding.DomainResultFactRef,
		ModelTurnSettlement:  p.ApplicationBinding.ModelTurnSettlementRef,
	}.Clone()
}

func (p CommittedPendingActionCurrentV2) DigestV2() (core.Digest, error) {
	copy := p.Clone()
	copy.Digest = ""
	return core.CanonicalJSONDigest(committedPendingActionCurrentCanonicalDomainV2, CommittedPendingActionCurrentContractVersionV2, "CommittedPendingActionCurrentV2", copy)
}

func (p CommittedPendingActionCurrentV2) Validate(now time.Time) error {
	if p.ContractVersion != CommittedPendingActionCurrentContractVersionV2 || p.Phase != SessionWaitingActionV2 || now.IsZero() || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano || now.UnixNano() < p.CheckedUnixNano || !now.Before(time.Unix(0, p.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectFenceStale, "committed PendingAction V2 projection is invalid, rolled back, or expired")
	}
	if err := p.SubjectV2().Validate(); err != nil {
		return err
	}
	if err := p.PendingAction.Validate(); err != nil {
		return err
	}
	if err := p.ApplicationBinding.Validate(); err != nil {
		return err
	}
	if !samePendingActionV3(p.PendingAction, p.ApplicationBinding.PendingAction) {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "committed PendingAction V2 projection and binding differ")
	}
	expectedSession, err := sessionApplicabilityCoordinateV2(p)
	if err != nil {
		return err
	}
	expectedTurn, err := turnApplicabilityCoordinateV2(p, expectedSession)
	if err != nil {
		return err
	}
	if p.SessionApplicability != expectedSession || p.TurnApplicability != expectedTurn || p.SessionApplicability.Validate() != nil || p.TurnApplicability.Validate() != nil {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidReference, "committed PendingAction V2 source coordinates drifted or were type-punned")
	}
	digest, err := p.DigestV2()
	if err != nil || digest != p.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "committed PendingAction V2 projection digest drifted")
	}
	return nil
}

func (p CommittedPendingActionCurrentV2) ValidateAgainst(expected CommittedPendingActionCurrentRequestV2, now time.Time) error {
	if err := expected.Validate(now); err != nil {
		return err
	}
	if err := p.Validate(now); err != nil {
		return err
	}
	if !sameCommittedPendingActionSubjectExactV2(p.SubjectV2(), expected.Subject) {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "committed PendingAction V2 projection belongs to another subject")
	}
	if expected.RequestedNotAfterUnixNano > 0 && p.ExpiresUnixNano > expected.RequestedNotAfterUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonCapabilityExpired, "committed PendingAction V2 projection exceeded the requested upper bound")
	}
	return nil
}

func sameCommittedPendingActionSubjectExactV2(actual, expected CommittedPendingActionSubjectV2) bool {
	return actual.ExecutionScopeDigest == expected.ExecutionScopeDigest &&
		actual.Run.RunID == expected.Run.RunID &&
		runtimeports.SameExecutionScopeV2(actual.Run.Scope, expected.Run.Scope) &&
		actual.SessionID == expected.SessionID &&
		actual.SessionRevision == expected.SessionRevision &&
		actual.SessionDigest == expected.SessionDigest &&
		actual.Turn == expected.Turn &&
		actual.PendingActionRef == expected.PendingActionRef &&
		actual.IdentityRef == expected.IdentityRef &&
		actual.DomainResultFactRef == expected.DomainResultFactRef &&
		reflect.DeepEqual(actual.ModelTurnSettlement, expected.ModelTurnSettlement)
}

func SealCommittedPendingActionCurrentV2(p CommittedPendingActionCurrentV2, expected CommittedPendingActionCurrentRequestV2, now time.Time) (CommittedPendingActionCurrentV2, error) {
	p.ContractVersion = CommittedPendingActionCurrentContractVersionV2
	sessionCoordinate, err := sessionApplicabilityCoordinateV2(p)
	if err != nil {
		return CommittedPendingActionCurrentV2{}, err
	}
	p.SessionApplicability = sessionCoordinate
	p.TurnApplicability, err = turnApplicabilityCoordinateV2(p, sessionCoordinate)
	if err != nil {
		return CommittedPendingActionCurrentV2{}, err
	}
	p.Digest = ""
	p.Digest, err = p.DigestV2()
	if err != nil {
		return CommittedPendingActionCurrentV2{}, err
	}
	return p.Clone(), p.ValidateAgainst(expected, now)
}

func sessionApplicabilityCoordinateV2(p CommittedPendingActionCurrentV2) (CommittedPendingActionSessionApplicabilityCoordinateV1, error) {
	digest, err := core.CanonicalJSONDigest(committedPendingActionSessionCoordinateDomainV2, CommittedPendingActionCurrentContractVersionV2, "CommittedPendingActionSessionApplicabilityCoordinateV2", struct {
		Run           RunRef                            `json:"run"`
		Scope         core.Digest                       `json:"execution_scope_digest"`
		SessionID     string                            `json:"session_id"`
		Revision      core.Revision                     `json:"revision"`
		SessionDigest core.Digest                       `json:"session_digest"`
		Phase         SessionPhaseV2                    `json:"phase"`
		Turn          uint32                            `json:"turn"`
		PendingAction PendingActionV2                   `json:"pending_action"`
		Binding       PendingActionApplicationBindingV1 `json:"application_binding"`
	}{p.Run, p.ExecutionScopeDigest, p.SessionID, p.SessionRevision, p.SessionDigest, p.Phase, p.Turn, p.PendingAction, p.ApplicationBinding})
	if err != nil {
		return CommittedPendingActionSessionApplicabilityCoordinateV1{}, err
	}
	coordinate := CommittedPendingActionSessionApplicabilityCoordinateV1{Kind: CommittedPendingActionSessionKindV1, ID: "session:" + string(digest), Revision: p.SessionRevision, Digest: digest}
	return coordinate, coordinate.Validate()
}

func turnApplicabilityCoordinateV2(p CommittedPendingActionCurrentV2, session CommittedPendingActionSessionApplicabilityCoordinateV1) (CommittedPendingActionTurnApplicabilityCoordinateV1, error) {
	digest, err := core.CanonicalJSONDigest(committedPendingActionTurnCoordinateDomainV2, CommittedPendingActionCurrentContractVersionV2, "CommittedPendingActionTurnApplicabilityCoordinateV2", struct {
		Session       CommittedPendingActionSessionApplicabilityCoordinateV1 `json:"session"`
		Turn          uint32                                                 `json:"turn"`
		PendingAction PendingActionV2                                        `json:"pending_action"`
		IdentityRef   ModelToolCallPendingActionIdentityRefV1                `json:"identity_ref"`
	}{session, p.Turn, p.PendingAction, p.ApplicationBinding.IdentityRef})
	if err != nil {
		return CommittedPendingActionTurnApplicabilityCoordinateV1{}, err
	}
	coordinate := CommittedPendingActionTurnApplicabilityCoordinateV1{Kind: CommittedPendingActionTurnKindV1, ID: "turn:" + string(digest), Revision: p.SessionRevision, Digest: digest}
	return coordinate, coordinate.Validate()
}
