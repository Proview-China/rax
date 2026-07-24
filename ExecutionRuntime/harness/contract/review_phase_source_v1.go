package contract

import (
	"reflect"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const (
	ReviewPhaseSourceContractVersionV1 = "praxis.harness.review-phase-source/v1"
	ReviewPhaseActionSourceV1          = runtimeports.NamespacedNameV2("praxis.harness/action.review")
	ReviewPhaseRunSourceV1             = runtimeports.NamespacedNameV2("praxis.harness/run.completion.validate")
	ReviewPhaseSubagentSourceV1        = runtimeports.NamespacedNameV2("praxis.harness/subagent.completion.validate")

	reviewPhaseSourceCanonicalDomainV1  = "praxis.harness.review-phase-source"
	MaxReviewPhaseSourceProjectionTTLV1 = 15 * time.Second
)

// ReviewActionPhaseSourceRefV1 binds the complete immutable PendingAction
// subject. Currentness is established only by CommittedPendingActionReaderV3.
type ReviewActionPhaseSourceRefV1 struct {
	Subject CommittedPendingActionSubjectV3 `json:"subject"`
}

func (r ReviewActionPhaseSourceRefV1) Clone() ReviewActionPhaseSourceRefV1 {
	return ReviewActionPhaseSourceRefV1{Subject: r.Subject.Clone()}
}

func (r ReviewActionPhaseSourceRefV1) Validate() error { return r.Subject.Validate() }

// ReviewRunPhaseSourceRefV1 is the exact Harness Session fact selected for a
// run completion phase. SessionDigest binds the complete Session payload; the
// duplicated fields make kind/type splicing fail before any Owner read.
type ReviewRunPhaseSourceRefV1 struct {
	Run                  RunRef          `json:"run"`
	ExecutionScopeDigest core.Digest     `json:"execution_scope_digest"`
	SessionID            string          `json:"session_id"`
	SessionRevision      core.Revision   `json:"session_revision"`
	SessionDigest        core.Digest     `json:"session_digest"`
	Phase                SessionPhaseV2  `json:"phase"`
	Turn                 uint32          `json:"turn"`
	CompletionClaim      CompletionClaim `json:"completion_claim"`
}

func (r ReviewRunPhaseSourceRefV1) Clone() ReviewRunPhaseSourceRefV1 {
	clone := r
	clone.Run.Scope = cloneExecutionScopeV3(r.Run.Scope)
	return clone
}

func (r ReviewRunPhaseSourceRefV1) Validate() error {
	if err := r.Run.Validate(); err != nil {
		return err
	}
	scopeDigest, err := runtimeports.ExecutionScopeDigestV2(r.Run.Scope)
	if err != nil || scopeDigest != r.ExecutionScopeDigest || strings.TrimSpace(r.SessionID) == "" || len(r.SessionID) > MaxReferenceBytes || r.SessionRevision == 0 || r.SessionDigest.Validate() != nil || r.Phase != SessionTerminalV2 || !reviewPhaseCompletionClaimV1(r.CompletionClaim) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "run Review phase source is not an exact terminal Session")
	}
	return nil
}

// ReviewSubagentPhaseSourceRefV1 is only an exact caller coordinate. Harness
// has no public Subagent current Owner reader in this version, so a valid shape
// is always rejected as unsupported by the Reader and never becomes current.
type ReviewSubagentPhaseSourceRefV1 struct {
	ParentRun      RunRef        `json:"parent_run"`
	SourceID       string        `json:"source_id"`
	SourceRevision core.Revision `json:"source_revision"`
	SourceDigest   core.Digest   `json:"source_digest"`
}

func (r ReviewSubagentPhaseSourceRefV1) Clone() ReviewSubagentPhaseSourceRefV1 {
	clone := r
	clone.ParentRun.Scope = cloneExecutionScopeV3(r.ParentRun.Scope)
	return clone
}

func (r ReviewSubagentPhaseSourceRefV1) Validate() error {
	if r.ParentRun.Validate() != nil || strings.TrimSpace(r.SourceID) == "" || len(r.SourceID) > MaxReferenceBytes || r.SourceRevision == 0 || r.SourceDigest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "subagent Review phase coordinate is incomplete")
	}
	return nil
}

// ReviewPhaseSourceRefV1 is a closed, nominal union. Exactly one arm is
// present; it is a coordinate only and grants no Review or Runtime authority.
type ReviewPhaseSourceRefV1 struct {
	ContractVersion string                          `json:"contract_version"`
	Kind            runtimeports.NamespacedNameV2   `json:"kind"`
	ID              string                          `json:"id"`
	Revision        core.Revision                   `json:"revision"`
	Action          *ReviewActionPhaseSourceRefV1   `json:"action,omitempty"`
	Run             *ReviewRunPhaseSourceRefV1      `json:"run,omitempty"`
	Subagent        *ReviewSubagentPhaseSourceRefV1 `json:"subagent,omitempty"`
	Digest          core.Digest                     `json:"digest"`
}

func (r ReviewPhaseSourceRefV1) Clone() ReviewPhaseSourceRefV1 {
	clone := r
	if r.Action != nil {
		value := r.Action.Clone()
		clone.Action = &value
	}
	if r.Run != nil {
		value := r.Run.Clone()
		clone.Run = &value
	}
	if r.Subagent != nil {
		value := r.Subagent.Clone()
		clone.Subagent = &value
	}
	return clone
}

func (r ReviewPhaseSourceRefV1) DigestV1() (core.Digest, error) {
	copy := r.Clone()
	copy.Digest = ""
	return core.CanonicalJSONDigest(reviewPhaseSourceCanonicalDomainV1, ReviewPhaseSourceContractVersionV1, "ReviewPhaseSourceRefV1", copy)
}

func (r ReviewPhaseSourceRefV1) Validate() error {
	if r.ContractVersion != ReviewPhaseSourceContractVersionV1 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Review phase source contract is unsupported")
	}
	arms := 0
	if r.Action != nil {
		arms++
	}
	if r.Run != nil {
		arms++
	}
	if r.Subagent != nil {
		arms++
	}
	if arms != 1 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonUnknownGovernanceCategory, "Review phase source must select exactly one closed union arm")
	}
	switch r.Kind {
	case ReviewPhaseActionSourceV1:
		if r.Action == nil || r.Run != nil || r.Subagent != nil || r.Action.Validate() != nil {
			return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "action Review phase source arm drifted")
		}
	case ReviewPhaseRunSourceV1:
		if r.Run == nil || r.Action != nil || r.Subagent != nil || r.Run.Validate() != nil {
			return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "run Review phase source arm drifted")
		}
	case ReviewPhaseSubagentSourceV1:
		if r.Subagent == nil || r.Action != nil || r.Run != nil || r.Subagent.Validate() != nil {
			return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "subagent Review phase source arm drifted")
		}
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonUnknownGovernanceCategory, "Review phase source kind is outside the closed union")
	}
	id, revision, err := r.identityV1()
	if err != nil || r.ID != id || r.Revision != revision {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidReference, "Review phase source identity drifted")
	}
	digest, err := r.DigestV1()
	if err != nil || digest != r.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Review phase source ref digest drifted")
	}
	return nil
}

func SealReviewPhaseSourceRefV1(r ReviewPhaseSourceRefV1) (ReviewPhaseSourceRefV1, error) {
	r = r.Clone()
	r.ContractVersion = ReviewPhaseSourceContractVersionV1
	var err error
	r.ID, r.Revision, err = r.identityV1()
	if err != nil {
		return ReviewPhaseSourceRefV1{}, err
	}
	r.Digest = ""
	digest, err := r.DigestV1()
	if err != nil {
		return ReviewPhaseSourceRefV1{}, err
	}
	r.Digest = digest
	return r.Clone(), r.Validate()
}

func (r ReviewPhaseSourceRefV1) identityV1() (string, core.Revision, error) {
	type identityInputV1 struct {
		Kind                 runtimeports.NamespacedNameV2 `json:"kind"`
		ExecutionScopeDigest core.Digest                   `json:"execution_scope_digest"`
		RunID                core.AgentRunID               `json:"run_id"`
		SessionID            string                        `json:"session_id,omitempty"`
		Turn                 uint32                        `json:"turn,omitempty"`
		SourceID             string                        `json:"source_id"`
	}
	var input identityInputV1
	var revision core.Revision
	switch r.Kind {
	case ReviewPhaseActionSourceV1:
		if r.Action == nil {
			return "", 0, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "action Review phase identity is absent")
		}
		base := r.Action.Subject.Base
		input = identityInputV1{Kind: r.Kind, ExecutionScopeDigest: base.ExecutionScopeDigest, RunID: base.Run.RunID, SessionID: base.SessionID, Turn: base.Turn, SourceID: base.PendingActionRef}
		revision = base.SessionRevision
	case ReviewPhaseRunSourceV1:
		if r.Run == nil {
			return "", 0, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "run Review phase identity is absent")
		}
		input = identityInputV1{Kind: r.Kind, ExecutionScopeDigest: r.Run.ExecutionScopeDigest, RunID: r.Run.Run.RunID, SessionID: r.Run.SessionID, SourceID: r.Run.SessionID}
		revision = r.Run.SessionRevision
	case ReviewPhaseSubagentSourceV1:
		if r.Subagent == nil {
			return "", 0, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "subagent Review phase identity is absent")
		}
		scopeDigest, err := runtimeports.ExecutionScopeDigestV2(r.Subagent.ParentRun.Scope)
		if err != nil {
			return "", 0, err
		}
		input = identityInputV1{Kind: r.Kind, ExecutionScopeDigest: scopeDigest, RunID: r.Subagent.ParentRun.RunID, SourceID: r.Subagent.SourceID}
		revision = r.Subagent.SourceRevision
	default:
		return "", 0, core.NewError(core.ErrorInvalidArgument, core.ReasonUnknownGovernanceCategory, "Review phase identity kind is unsupported")
	}
	digest, err := core.CanonicalJSONDigest(reviewPhaseSourceCanonicalDomainV1, ReviewPhaseSourceContractVersionV1, "ReviewPhaseSourceIdentityInputV1", input)
	if err != nil {
		return "", 0, err
	}
	return "review-phase-source:" + strings.TrimPrefix(string(digest), "sha256:"), revision, nil
}

type ReviewPhaseSourceCurrentRequestV1 struct {
	Source                    ReviewPhaseSourceRefV1 `json:"source"`
	RequestedNotAfterUnixNano int64                  `json:"requested_not_after_unix_nano"`
}

func (r ReviewPhaseSourceCurrentRequestV1) Clone() ReviewPhaseSourceCurrentRequestV1 {
	return ReviewPhaseSourceCurrentRequestV1{Source: r.Source.Clone(), RequestedNotAfterUnixNano: r.RequestedNotAfterUnixNano}
}

func (r ReviewPhaseSourceCurrentRequestV1) Validate(now time.Time) error {
	if now.IsZero() || r.RequestedNotAfterUnixNano < 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Review phase current request clock or TTL is invalid")
	}
	if err := r.Source.Validate(); err != nil {
		return err
	}
	if r.RequestedNotAfterUnixNano > 0 && r.RequestedNotAfterUnixNano <= now.UnixNano() {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonCapabilityExpired, "Review phase current request expired")
	}
	return nil
}

// ReviewPhaseSourceCurrentProjectionV1 is a read-only Harness observation. It
// carries either the exact Action current projection or the exact Run Session
// snapshot; it cannot carry both and cannot represent Subagent currentness.
type ReviewPhaseSourceCurrentProjectionV1 struct {
	ContractVersion      string                           `json:"contract_version"`
	Source               ReviewPhaseSourceRefV1           `json:"source"`
	Run                  RunRef                           `json:"run"`
	ExecutionScopeDigest core.Digest                      `json:"execution_scope_digest"`
	SessionID            string                           `json:"session_id"`
	SessionRevision      core.Revision                    `json:"session_revision"`
	SessionDigest        core.Digest                      `json:"session_digest"`
	Phase                SessionPhaseV2                   `json:"phase"`
	Turn                 uint32                           `json:"turn"`
	CompletionClaim      CompletionClaim                  `json:"completion_claim,omitempty"`
	Action               *CommittedPendingActionCurrentV3 `json:"action,omitempty"`
	RunSession           *GovernedSessionV4               `json:"run_session,omitempty"`
	ClosureDigest        core.Digest                      `json:"closure_digest"`
	CheckedUnixNano      int64                            `json:"checked_unix_nano"`
	ExpiresUnixNano      int64                            `json:"expires_unix_nano"`
	Digest               core.Digest                      `json:"digest"`
}

func (p ReviewPhaseSourceCurrentProjectionV1) Clone() ReviewPhaseSourceCurrentProjectionV1 {
	clone := p
	clone.Source = p.Source.Clone()
	clone.Run.Scope = cloneExecutionScopeV3(p.Run.Scope)
	if p.Action != nil {
		value := p.Action.Clone()
		clone.Action = &value
	}
	if p.RunSession != nil {
		value := p.RunSession.Clone()
		clone.RunSession = &value
	}
	return clone
}

func (p ReviewPhaseSourceCurrentProjectionV1) DigestV1() (core.Digest, error) {
	copy := p.Clone()
	copy.Digest = ""
	return core.CanonicalJSONDigest(reviewPhaseSourceCanonicalDomainV1, ReviewPhaseSourceContractVersionV1, "ReviewPhaseSourceCurrentProjectionV1", copy)
}

// ClosureDigestV1 seals only Owner semantics. Fresh observation time, natural
// expiry and the outer projection digest are deliberately excluded so S1/S2
// can compare unchanged current facts without treating time passage as drift.
func (p ReviewPhaseSourceCurrentProjectionV1) ClosureDigestV1() (core.Digest, error) {
	type closureInputV1 struct {
		Source               ReviewPhaseSourceRefV1           `json:"source"`
		Run                  RunRef                           `json:"run"`
		ExecutionScopeDigest core.Digest                      `json:"execution_scope_digest"`
		SessionID            string                           `json:"session_id"`
		SessionRevision      core.Revision                    `json:"session_revision"`
		SessionDigest        core.Digest                      `json:"session_digest"`
		Phase                SessionPhaseV2                   `json:"phase"`
		Turn                 uint32                           `json:"turn"`
		CompletionClaim      CompletionClaim                  `json:"completion_claim,omitempty"`
		Action               *CommittedPendingActionCurrentV3 `json:"action,omitempty"`
		RunSession           *GovernedSessionV4               `json:"run_session,omitempty"`
	}
	var action *CommittedPendingActionCurrentV3
	if p.Action != nil {
		value := p.Action.Clone()
		value.CheckedUnixNano, value.ExpiresUnixNano, value.Digest = 0, 0, ""
		action = &value
	}
	var session *GovernedSessionV4
	if p.RunSession != nil {
		value := p.RunSession.Clone()
		session = &value
	}
	return core.CanonicalJSONDigest(reviewPhaseSourceCanonicalDomainV1, ReviewPhaseSourceContractVersionV1, "ReviewPhaseSourceClosureV1", closureInputV1{Source: p.Source.Clone(), Run: RunRef{Scope: cloneExecutionScopeV3(p.Run.Scope), RunID: p.Run.RunID}, ExecutionScopeDigest: p.ExecutionScopeDigest, SessionID: p.SessionID, SessionRevision: p.SessionRevision, SessionDigest: p.SessionDigest, Phase: p.Phase, Turn: p.Turn, CompletionClaim: p.CompletionClaim, Action: action, RunSession: session})
}

func (p ReviewPhaseSourceCurrentProjectionV1) ValidateCurrentFor(request ReviewPhaseSourceCurrentRequestV1, now time.Time) error {
	if err := request.Validate(now); err != nil {
		return err
	}
	if p.ContractVersion != ReviewPhaseSourceContractVersionV1 || !reflect.DeepEqual(p.Source, request.Source) || p.Run.Validate() != nil || p.ExecutionScopeDigest.Validate() != nil || strings.TrimSpace(p.SessionID) == "" || p.SessionRevision == 0 || p.SessionDigest.Validate() != nil || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano || p.ExpiresUnixNano-p.CheckedUnixNano > int64(MaxReviewPhaseSourceProjectionTTLV1) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidReference, "Review phase current projection is incomplete")
	}
	scopeDigest, err := runtimeports.ExecutionScopeDigestV2(p.Run.Scope)
	if err != nil || scopeDigest != p.ExecutionScopeDigest || now.IsZero() || now.UnixNano() < p.CheckedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Review phase current projection clock or scope regressed")
	}
	if !now.Before(time.Unix(0, p.ExpiresUnixNano)) || request.RequestedNotAfterUnixNano > 0 && p.ExpiresUnixNano > request.RequestedNotAfterUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonCapabilityExpired, "Review phase current projection expired or exceeded its caller bound")
	}
	switch p.Source.Kind {
	case ReviewPhaseActionSourceV1:
		if p.Action == nil || p.RunSession != nil || p.CompletionClaim != "" || p.Phase != SessionWaitingActionV2 || p.Action.ValidateAgainst(CommittedPendingActionCurrentRequestV3{Subject: p.Source.Action.Subject.Clone(), RequestedNotAfterUnixNano: request.RequestedNotAfterUnixNano}, now) != nil || p.Run.RunID != p.Action.Run.RunID || !runtimeports.SameExecutionScopeV2(p.Run.Scope, p.Action.Run.Scope) || p.ExecutionScopeDigest != p.Action.ExecutionScopeDigest || p.SessionID != p.Action.SessionID || p.SessionRevision != p.Action.SessionRevision || p.SessionDigest != p.Action.SessionDigest || p.Phase != p.Action.Phase || p.Turn != p.Action.Turn || p.CheckedUnixNano < p.Action.CheckedUnixNano || p.ExpiresUnixNano > p.Action.ExpiresUnixNano {
			return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "action Review phase current projection drifted")
		}
	case ReviewPhaseRunSourceV1:
		if p.Action != nil || p.RunSession == nil || p.Source.Run == nil || p.Phase != SessionTerminalV2 || !reviewPhaseCompletionClaimV1(p.CompletionClaim) || p.RunSession.Validate() != nil || !sameRunReviewPhaseSourceV1(*p.RunSession, *p.Source.Run) || p.Run.RunID != p.RunSession.Run.RunID || !runtimeports.SameExecutionScopeV2(p.Run.Scope, p.RunSession.Run.Scope) || p.SessionID != p.RunSession.ID || p.SessionRevision != p.RunSession.Revision || p.SessionDigest != p.RunSession.Digest || p.Phase != p.RunSession.Phase || p.Turn != p.RunSession.Turn || p.CompletionClaim != p.RunSession.CompletionClaim {
			return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "run Review phase current projection drifted")
		}
	case ReviewPhaseSubagentSourceV1:
		return core.NewError(core.ErrorCapabilityUnavailable, core.ReasonUnknownGovernanceCategory, "subagent Review phase current source is unsupported")
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonUnknownGovernanceCategory, "Review phase current projection kind is unsupported")
	}
	digest, err := p.DigestV1()
	closure, closureErr := p.ClosureDigestV1()
	if closureErr != nil || closure != p.ClosureDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Review phase current closure digest drifted")
	}
	if err != nil || digest != p.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Review phase current projection digest drifted")
	}
	return nil
}

func SealReviewPhaseSourceCurrentProjectionV1(p ReviewPhaseSourceCurrentProjectionV1, request ReviewPhaseSourceCurrentRequestV1, now time.Time) (ReviewPhaseSourceCurrentProjectionV1, error) {
	p = p.Clone()
	p.ContractVersion = ReviewPhaseSourceContractVersionV1
	var err error
	p.ClosureDigest, err = p.ClosureDigestV1()
	if err != nil {
		return ReviewPhaseSourceCurrentProjectionV1{}, err
	}
	p.Digest = ""
	digest, err := p.DigestV1()
	if err != nil {
		return ReviewPhaseSourceCurrentProjectionV1{}, err
	}
	p.Digest = digest
	return p.Clone(), p.ValidateCurrentFor(request, now)
}

func sameRunReviewPhaseSourceV1(session GovernedSessionV4, source ReviewRunPhaseSourceRefV1) bool {
	return session.ID == source.SessionID && session.Revision == source.SessionRevision && session.Digest == source.SessionDigest && session.Run.RunID == source.Run.RunID && runtimeports.SameExecutionScopeV2(session.Run.Scope, source.Run.Scope) && session.Phase == source.Phase && session.Turn == source.Turn && session.CompletionClaim == source.CompletionClaim
}

func reviewPhaseCompletionClaimV1(claim CompletionClaim) bool {
	switch claim {
	case ClaimCompleted, ClaimCancelled, ClaimFailed, ClaimIndeterminate:
		return true
	default:
		return false
	}
}
