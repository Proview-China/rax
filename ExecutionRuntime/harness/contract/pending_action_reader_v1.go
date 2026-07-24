package contract

import (
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const (
	CommittedPendingActionReaderContractVersionV1   = "praxis.harness.committed-pending-action-reader/v1"
	CommittedPendingActionSessionKindV1             = runtimeports.NamespacedNameV2("praxis.harness/session")
	CommittedPendingActionTurnKindV1                = runtimeports.NamespacedNameV2("praxis.harness/turn")
	committedPendingActionSessionCoordinateDomainV1 = "praxis.harness.committed-pending-action-session-coordinate"
	committedPendingActionTurnCoordinateDomainV1    = "praxis.harness.committed-pending-action-turn-coordinate"
)

// InspectCommittedPendingActionCurrentRequestV1 binds one exact Harness-owned
// Session/Turn/PendingAction observation. It grants no write or execution
// authority.
type InspectCommittedPendingActionCurrentRequestV1 struct {
	ContractVersion             string        `json:"contract_version"`
	Run                         RunRef        `json:"run"`
	ExecutionScopeDigest        core.Digest   `json:"execution_scope_digest"`
	SessionID                   string        `json:"session_id"`
	ExpectedSessionRevision     core.Revision `json:"expected_session_revision"`
	ExpectedTurn                uint32        `json:"expected_turn"`
	ExpectedPendingActionRef    string        `json:"expected_pending_action_ref"`
	ExpectedPendingActionDigest core.Digest   `json:"expected_pending_action_digest"`
	CheckedAtUnixNano           int64         `json:"checked_at_unix_nano"`
}

// CommittedPendingActionSubjectV1 contains only stable subject coordinates.
// Observation time is supplied afresh for every Inspect and is deliberately
// excluded from immutable Binding identity.
type CommittedPendingActionSubjectV1 struct {
	ContractVersion             string        `json:"contract_version"`
	Run                         RunRef        `json:"run"`
	ExecutionScopeDigest        core.Digest   `json:"execution_scope_digest"`
	SessionID                   string        `json:"session_id"`
	ExpectedSessionRevision     core.Revision `json:"expected_session_revision"`
	ExpectedTurn                uint32        `json:"expected_turn"`
	ExpectedPendingActionRef    string        `json:"expected_pending_action_ref"`
	ExpectedPendingActionDigest core.Digest   `json:"expected_pending_action_digest"`
}

func (r InspectCommittedPendingActionCurrentRequestV1) Clone() InspectCommittedPendingActionCurrentRequestV1 {
	clone := r
	if r.Run.Scope.SandboxLease != nil {
		lease := *r.Run.Scope.SandboxLease
		clone.Run.Scope.SandboxLease = &lease
	}
	return clone
}

func (r InspectCommittedPendingActionCurrentRequestV1) Validate() error {
	if r.CheckedAtUnixNano <= 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "committed PendingAction Inspect request is incomplete")
	}
	return r.SubjectV1().Validate()
}

func (r InspectCommittedPendingActionCurrentRequestV1) SubjectV1() CommittedPendingActionSubjectV1 {
	return CommittedPendingActionSubjectV1{
		ContractVersion:             r.ContractVersion,
		Run:                         r.Run,
		ExecutionScopeDigest:        r.ExecutionScopeDigest,
		SessionID:                   r.SessionID,
		ExpectedSessionRevision:     r.ExpectedSessionRevision,
		ExpectedTurn:                r.ExpectedTurn,
		ExpectedPendingActionRef:    r.ExpectedPendingActionRef,
		ExpectedPendingActionDigest: r.ExpectedPendingActionDigest,
	}.Clone()
}

func (s CommittedPendingActionSubjectV1) Clone() CommittedPendingActionSubjectV1 {
	clone := s
	if s.Run.Scope.SandboxLease != nil {
		lease := *s.Run.Scope.SandboxLease
		clone.Run.Scope.SandboxLease = &lease
	}
	return clone
}

func (s CommittedPendingActionSubjectV1) Validate() error {
	if s.ContractVersion != CommittedPendingActionReaderContractVersionV1 || strings.TrimSpace(s.SessionID) == "" || len(s.SessionID) > MaxReferenceBytes || s.ExpectedSessionRevision == 0 || s.ExpectedTurn == 0 || strings.TrimSpace(s.ExpectedPendingActionRef) == "" || len(s.ExpectedPendingActionRef) > MaxReferenceBytes {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "committed PendingAction subject is incomplete")
	}
	if err := s.Run.Validate(); err != nil {
		return err
	}
	if err := s.ExecutionScopeDigest.Validate(); err != nil {
		return err
	}
	if err := s.ExpectedPendingActionDigest.Validate(); err != nil {
		return err
	}
	scopeDigest, err := runtimeports.ExecutionScopeDigestV2(s.Run.Scope)
	if err != nil || scopeDigest != s.ExecutionScopeDigest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectFenceStale, "committed PendingAction request Execution Scope digest drifted")
	}
	return nil
}

func (s CommittedPendingActionSubjectV1) InspectRequestAtV1(checkedAt time.Time) (InspectCommittedPendingActionCurrentRequestV1, error) {
	if err := s.Validate(); err != nil {
		return InspectCommittedPendingActionCurrentRequestV1{}, err
	}
	if checkedAt.IsZero() {
		return InspectCommittedPendingActionCurrentRequestV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "committed PendingAction Inspect time is missing")
	}
	request := InspectCommittedPendingActionCurrentRequestV1{
		ContractVersion:             s.ContractVersion,
		Run:                         s.Run,
		ExecutionScopeDigest:        s.ExecutionScopeDigest,
		SessionID:                   s.SessionID,
		ExpectedSessionRevision:     s.ExpectedSessionRevision,
		ExpectedTurn:                s.ExpectedTurn,
		ExpectedPendingActionRef:    s.ExpectedPendingActionRef,
		ExpectedPendingActionDigest: s.ExpectedPendingActionDigest,
		CheckedAtUnixNano:           checkedAt.UnixNano(),
	}
	return request.Clone(), request.Validate()
}

// CommittedPendingActionSessionApplicabilityCoordinateV1 is a Harness-owned
// source coordinate. It is not Runtime Evidence, authority, or a public
// applicability Fact reference.
type CommittedPendingActionSessionApplicabilityCoordinateV1 struct {
	Kind     runtimeports.NamespacedNameV2 `json:"kind"`
	ID       string                        `json:"id"`
	Revision core.Revision                 `json:"revision"`
	Digest   core.Digest                   `json:"digest"`
}

func (c CommittedPendingActionSessionApplicabilityCoordinateV1) Validate() error {
	if c.Kind != CommittedPendingActionSessionKindV1 || strings.TrimSpace(c.ID) == "" || len(c.ID) > MaxReferenceBytes || c.Revision == 0 || c.ID != "session:"+string(c.Digest) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "committed PendingAction Session source coordinate is invalid")
	}
	return c.Digest.Validate()
}

// CommittedPendingActionTurnApplicabilityCoordinateV1 is deliberately a
// distinct nominal type and canonical domain from the Session coordinate.
type CommittedPendingActionTurnApplicabilityCoordinateV1 struct {
	Kind     runtimeports.NamespacedNameV2 `json:"kind"`
	ID       string                        `json:"id"`
	Revision core.Revision                 `json:"revision"`
	Digest   core.Digest                   `json:"digest"`
}

func (c CommittedPendingActionTurnApplicabilityCoordinateV1) Validate() error {
	if c.Kind != CommittedPendingActionTurnKindV1 || strings.TrimSpace(c.ID) == "" || len(c.ID) > MaxReferenceBytes || c.Revision == 0 || c.ID != "turn:"+string(c.Digest) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "committed PendingAction Turn source coordinate is invalid")
	}
	return c.Digest.Validate()
}

// CommittedPendingActionCurrentV1 is a short-lived, read-only projection. The
// complete PendingAction remains owned by the Harness Session fact.
type CommittedPendingActionCurrentV1 struct {
	ContractVersion      string                                                 `json:"contract_version"`
	Run                  RunRef                                                 `json:"run"`
	ExecutionScopeDigest core.Digest                                            `json:"execution_scope_digest"`
	SessionID            string                                                 `json:"session_id"`
	SessionRevision      core.Revision                                          `json:"session_revision"`
	SessionDigest        core.Digest                                            `json:"session_digest"`
	Phase                SessionPhaseV2                                         `json:"phase"`
	Turn                 uint32                                                 `json:"turn"`
	PendingAction        PendingActionV2                                        `json:"pending_action"`
	SessionApplicability CommittedPendingActionSessionApplicabilityCoordinateV1 `json:"session_applicability"`
	TurnApplicability    CommittedPendingActionTurnApplicabilityCoordinateV1    `json:"turn_applicability"`
	CheckedUnixNano      int64                                                  `json:"checked_unix_nano"`
	ExpiresUnixNano      int64                                                  `json:"expires_unix_nano"`
	Digest               core.Digest                                            `json:"digest"`
}

func (p CommittedPendingActionCurrentV1) Validate(expected InspectCommittedPendingActionCurrentRequestV1, now time.Time) error {
	if err := expected.Validate(); err != nil {
		return err
	}
	if p.ContractVersion != CommittedPendingActionReaderContractVersionV1 || p.Run.RunID != expected.Run.RunID || !runtimeports.SameExecutionScopeV2(p.Run.Scope, expected.Run.Scope) || p.ExecutionScopeDigest != expected.ExecutionScopeDigest || p.SessionID != expected.SessionID || p.SessionRevision != expected.ExpectedSessionRevision || p.Phase != SessionWaitingActionV2 || p.Turn != expected.ExpectedTurn || p.CheckedUnixNano < expected.CheckedAtUnixNano || p.ExpiresUnixNano <= p.CheckedUnixNano || now.IsZero() || now.UnixNano() < p.CheckedUnixNano || !now.Before(time.Unix(0, p.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectFenceStale, "committed PendingAction projection is stale or mismatched")
	}
	if err := p.SessionDigest.Validate(); err != nil {
		return err
	}
	if err := p.PendingAction.Validate(); err != nil {
		return err
	}
	if p.PendingAction.Ref != expected.ExpectedPendingActionRef || p.PendingAction.RequestDigest != expected.ExpectedPendingActionDigest {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "committed PendingAction projection binds another action")
	}
	expectedSession, err := sessionApplicabilityCoordinateV1(p)
	if err != nil {
		return err
	}
	expectedTurn, err := turnApplicabilityCoordinateV1(p, expectedSession)
	if err != nil {
		return err
	}
	if p.SessionApplicability != expectedSession || p.TurnApplicability != expectedTurn || p.SessionApplicability.Validate() != nil || p.TurnApplicability.Validate() != nil {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidReference, "committed PendingAction applicability refs drifted or type-punned")
	}
	digest, err := p.DigestV1()
	if err != nil || digest != p.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "committed PendingAction projection digest drifted")
	}
	return nil
}

func (p CommittedPendingActionCurrentV1) DigestV1() (core.Digest, error) {
	copy := p
	copy.Digest = ""
	return core.CanonicalJSONDigest("praxis.harness.committed-pending-action", CommittedPendingActionReaderContractVersionV1, "CommittedPendingActionCurrentV1", copy)
}

func SealCommittedPendingActionCurrentV1(p CommittedPendingActionCurrentV1, expected InspectCommittedPendingActionCurrentRequestV1, now time.Time) (CommittedPendingActionCurrentV1, error) {
	p.ContractVersion = CommittedPendingActionReaderContractVersionV1
	sessionCoordinate, err := sessionApplicabilityCoordinateV1(p)
	if err != nil {
		return CommittedPendingActionCurrentV1{}, err
	}
	p.SessionApplicability = sessionCoordinate
	turnCoordinate, err := turnApplicabilityCoordinateV1(p, sessionCoordinate)
	if err != nil {
		return CommittedPendingActionCurrentV1{}, err
	}
	p.TurnApplicability = turnCoordinate
	p.Digest = ""
	p.Digest, err = p.DigestV1()
	if err != nil {
		return CommittedPendingActionCurrentV1{}, err
	}
	return p, p.Validate(expected, now)
}

func (s GovernedSessionV2) DigestV2() (core.Digest, error) {
	if err := s.Validate(); err != nil {
		return "", err
	}
	return core.CanonicalJSONDigest("praxis.harness.governed", GovernedContractVersionV2, "GovernedSessionV2", s)
}

func sessionApplicabilityCoordinateV1(p CommittedPendingActionCurrentV1) (CommittedPendingActionSessionApplicabilityCoordinateV1, error) {
	digest, err := core.CanonicalJSONDigest(committedPendingActionSessionCoordinateDomainV1, CommittedPendingActionReaderContractVersionV1, "CommittedPendingActionSessionApplicabilityCoordinateV1", struct {
		Run                  RunRef         `json:"run"`
		ExecutionScopeDigest core.Digest    `json:"execution_scope_digest"`
		SessionID            string         `json:"session_id"`
		SessionRevision      core.Revision  `json:"session_revision"`
		SessionDigest        core.Digest    `json:"session_digest"`
		Phase                SessionPhaseV2 `json:"phase"`
		PendingActionRef     string         `json:"pending_action_ref"`
		PendingActionDigest  core.Digest    `json:"pending_action_digest"`
	}{p.Run, p.ExecutionScopeDigest, p.SessionID, p.SessionRevision, p.SessionDigest, p.Phase, p.PendingAction.Ref, p.PendingAction.RequestDigest})
	if err != nil {
		return CommittedPendingActionSessionApplicabilityCoordinateV1{}, err
	}
	coordinate := CommittedPendingActionSessionApplicabilityCoordinateV1{Kind: CommittedPendingActionSessionKindV1, ID: "session:" + string(digest), Revision: p.SessionRevision, Digest: digest}
	return coordinate, coordinate.Validate()
}

func turnApplicabilityCoordinateV1(p CommittedPendingActionCurrentV1, session CommittedPendingActionSessionApplicabilityCoordinateV1) (CommittedPendingActionTurnApplicabilityCoordinateV1, error) {
	digest, err := core.CanonicalJSONDigest(committedPendingActionTurnCoordinateDomainV1, CommittedPendingActionReaderContractVersionV1, "CommittedPendingActionTurnApplicabilityCoordinateV1", struct {
		Session             CommittedPendingActionSessionApplicabilityCoordinateV1 `json:"session"`
		Turn                uint32                                                 `json:"turn"`
		PendingActionRef    string                                                 `json:"pending_action_ref"`
		PendingActionDigest core.Digest                                            `json:"pending_action_digest"`
	}{session, p.Turn, p.PendingAction.Ref, p.PendingAction.RequestDigest})
	if err != nil {
		return CommittedPendingActionTurnApplicabilityCoordinateV1{}, err
	}
	coordinate := CommittedPendingActionTurnApplicabilityCoordinateV1{Kind: CommittedPendingActionTurnKindV1, ID: "turn:" + string(digest), Revision: p.SessionRevision, Digest: digest}
	return coordinate, coordinate.Validate()
}
