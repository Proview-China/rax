package contract

import (
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const (
	TurnContinuationContractVersionV1 = "praxis.application/turn-continuation/v1"
	TurnContinuationPendingV1         = TurnContinuationStateV1("continuation_pending")
	TurnContinuationContextCurrentV1  = TurnContinuationStateV1("context_current")
)

type TurnContinuationStateV1 string

// HarnessActiveContextRefV1 is the exact Harness-owned value used as the CAS
// predicate for a Session's active immutable ContextFrame. Exact refs do not
// grant currentness; only the Harness owner may compare or publish this value.
type HarnessActiveContextRefV1 struct {
	ContractVersion          string                   `json:"contract_version"`
	ID                       string                   `json:"id"`
	Revision                 core.Revision            `json:"revision"`
	ExecutionScopeDigest     core.Digest              `json:"execution_scope_digest"`
	RunID                    core.AgentRunID          `json:"run_id"`
	SessionID                string                   `json:"session_id"`
	TurnOrdinal              uint32                   `json:"turn_ordinal"`
	ManifestRef              ContextRefreshExactRefV1 `json:"manifest_ref"`
	FrameRef                 ContextRefreshExactRefV1 `json:"frame_ref"`
	GenerationRef            ContextRefreshExactRefV1 `json:"generation_ref"`
	ContextCurrentPointerRef ContextRefreshExactRefV1 `json:"context_current_pointer_ref"`
	UpdatedUnixNano          int64                    `json:"updated_unix_nano"`
	Digest                   core.Digest              `json:"digest"`
}

func DeriveHarnessActiveContextIDV1(scope core.Digest, runID core.AgentRunID, sessionID string) (string, error) {
	if scope.Validate() != nil || !validSingleCallIDV1(string(runID)) || !validSingleCallIDV1(sessionID) {
		return "", core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Harness ActiveContext identity is incomplete")
	}
	digest, err := core.CanonicalJSONDigest("praxis.harness.active-context-id", TurnContinuationContractVersionV1, "HarnessActiveContextIDV1", struct {
		ExecutionScopeDigest core.Digest     `json:"execution_scope_digest"`
		RunID                core.AgentRunID `json:"run_id"`
		SessionID            string          `json:"session_id"`
	}{scope, runID, sessionID})
	if err != nil {
		return "", err
	}
	return "active-context:v1:" + strings.TrimPrefix(string(digest), "sha256:"), nil
}

func (r HarnessActiveContextRefV1) DigestV1() (core.Digest, error) {
	copy := r
	copy.Digest = ""
	return core.CanonicalJSONDigest("praxis.harness.active-context", TurnContinuationContractVersionV1, "HarnessActiveContextRefV1", copy)
}

func (r HarnessActiveContextRefV1) Validate() error {
	if r.ContractVersion != TurnContinuationContractVersionV1 || !validSingleCallIDV1(r.ID) || r.Revision == 0 || r.ExecutionScopeDigest.Validate() != nil || !validSingleCallIDV1(string(r.RunID)) || !validSingleCallIDV1(r.SessionID) || r.TurnOrdinal == 0 || r.ManifestRef.Validate() != nil || r.FrameRef.Validate() != nil || r.GenerationRef.Validate() != nil || r.ContextCurrentPointerRef.Validate() != nil || r.UpdatedUnixNano <= 0 || r.Digest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Harness ActiveContext ref is incomplete")
	}
	expectedID, err := DeriveHarnessActiveContextIDV1(r.ExecutionScopeDigest, r.RunID, r.SessionID)
	if err != nil || expectedID != r.ID {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidReference, "Harness ActiveContext ID drifted")
	}
	digest, err := r.DigestV1()
	if err != nil || digest != r.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Harness ActiveContext digest drifted")
	}
	return nil
}

func SealHarnessActiveContextRefV1(r HarnessActiveContextRefV1) (HarnessActiveContextRefV1, error) {
	r.ContractVersion = TurnContinuationContractVersionV1
	expectedID, err := DeriveHarnessActiveContextIDV1(r.ExecutionScopeDigest, r.RunID, r.SessionID)
	if err != nil {
		return HarnessActiveContextRefV1{}, err
	}
	if r.ID != "" && r.ID != expectedID {
		return HarnessActiveContextRefV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidReference, "Harness ActiveContext supplied another ID")
	}
	r.ID = expectedID
	provided := r.Digest
	r.Digest = ""
	digest, err := r.DigestV1()
	if err != nil {
		return HarnessActiveContextRefV1{}, err
	}
	if provided != "" && provided != digest {
		return HarnessActiveContextRefV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Harness ActiveContext supplied another digest")
	}
	r.Digest = digest
	return r, r.Validate()
}

type TurnContinuationAttemptRefV1 struct {
	ID                   string          `json:"id"`
	Revision             core.Revision   `json:"revision"`
	Digest               core.Digest     `json:"digest"`
	ExecutionScopeDigest core.Digest     `json:"execution_scope_digest"`
	RunID                core.AgentRunID `json:"run_id"`
	SessionID            string          `json:"session_id"`
	SourceTurn           uint32          `json:"source_turn"`
	TargetTurn           uint32          `json:"target_turn"`
}

func (r TurnContinuationAttemptRefV1) Validate() error {
	if !validSingleCallIDV1(r.ID) || r.Revision != 1 || r.Digest.Validate() != nil || r.ExecutionScopeDigest.Validate() != nil || !validSingleCallIDV1(string(r.RunID)) || !validSingleCallIDV1(r.SessionID) || r.SourceTurn == 0 || r.SourceTurn == ^uint32(0) || r.TargetTurn != r.SourceTurn+1 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "TurnContinuation Attempt ref is incomplete")
	}
	return nil
}

// TurnContinuationStartRequestV1 co-seals exact Tool and Context-attempt
// coordinates against later splicing. It does not prove that the Context
// prepare content was derived from the ToolResult payload.
type TurnContinuationStartRequestV1 struct {
	ContractVersion               string                          `json:"contract_version"`
	AttemptID                     string                          `json:"attempt_id"`
	Revision                      core.Revision                   `json:"revision"`
	ExecutionScopeDigest          core.Digest                     `json:"execution_scope_digest"`
	RunID                         core.AgentRunID                 `json:"run_id"`
	Source                        ContextTurnSourceCurrentV1      `json:"source"`
	SettledToolResult             SingleCallToolActionResultRefV2 `json:"settled_tool_result"`
	ExpectedActiveContext         HarnessActiveContextRefV1       `json:"expected_active_context"`
	ExpectedContextRefreshAttempt ContextRefreshExactRefV1        `json:"expected_context_refresh_attempt"`
	TargetTurn                    uint32                          `json:"target_turn"`
	RequestedNotAfterUnixNano     int64                           `json:"requested_not_after_unix_nano"`
	Digest                        core.Digest                     `json:"digest"`
}

// DeriveTurnContinuationAttemptIDV1 fixes the stable Attempt before the
// Context prepare request is sealed. The prepare request must then use this ID;
// its exact digest is frozen in ExpectedContextRefreshAttempt before Begin.
func DeriveTurnContinuationAttemptIDV1(r TurnContinuationStartRequestV1) (string, error) {
	if r.ExecutionScopeDigest.Validate() != nil || !validSingleCallIDV1(string(r.RunID)) || r.Source.Session.Validate() != nil || r.Source.Turn.Validate() != nil || r.SettledToolResult.Validate() != nil || r.ExpectedActiveContext.Validate() != nil || r.TargetTurn == 0 {
		return "", core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "TurnContinuation Attempt identity is incomplete")
	}
	digest, err := core.CanonicalJSONDigest("praxis.application.turn-continuation-attempt-id", TurnContinuationContractVersionV1, "TurnContinuationAttemptIDV1", struct {
		ExecutionScopeDigest core.Digest                     `json:"execution_scope_digest"`
		RunID                core.AgentRunID                 `json:"run_id"`
		SessionID            string                          `json:"session_id"`
		SessionRevision      core.Revision                   `json:"session_revision"`
		SessionDigest        core.Digest                     `json:"session_digest"`
		SourceTurn           SingleCallTurnCoordinateV1      `json:"source_turn"`
		SettledToolResult    SingleCallToolActionResultRefV2 `json:"settled_tool_result"`
		ExpectedActive       HarnessActiveContextRefV1       `json:"expected_active_context"`
		TargetTurn           uint32                          `json:"target_turn"`
	}{r.ExecutionScopeDigest, r.RunID, r.Source.Session.ID, r.Source.Session.Revision, r.Source.Session.Digest, r.Source.Turn, r.SettledToolResult, r.ExpectedActiveContext, r.TargetTurn})
	if err != nil {
		return "", err
	}
	return "turn-continuation:v1:" + strings.TrimPrefix(string(digest), "sha256:"), nil
}

func (r TurnContinuationStartRequestV1) DigestV1() (core.Digest, error) {
	copy := r
	copy.Digest = ""
	return core.CanonicalJSONDigest("praxis.application.turn-continuation-start", TurnContinuationContractVersionV1, "TurnContinuationStartRequestV1", copy)
}

func (r TurnContinuationStartRequestV1) Validate() error {
	if r.ContractVersion != TurnContinuationContractVersionV1 || !validSingleCallIDV1(r.AttemptID) || r.Revision != 1 || r.ExecutionScopeDigest.Validate() != nil || !validSingleCallIDV1(string(r.RunID)) || r.Source.ValidateCurrent(time.Unix(0, r.Source.CheckedUnixNano)) != nil || r.SettledToolResult.Validate() != nil || r.ExpectedActiveContext.Validate() != nil || r.ExpectedContextRefreshAttempt.Validate() != nil || r.ExpectedContextRefreshAttempt.Kind != ContextTurnRefreshApplicationAttemptKindV1 || r.ExpectedContextRefreshAttempt.ID != r.AttemptID || r.ExpectedContextRefreshAttempt.Revision != 1 || r.Source.Turn.Ordinal == ^uint32(0) || r.TargetTurn != r.Source.Turn.Ordinal+1 || r.RequestedNotAfterUnixNano <= r.Source.CheckedUnixNano || r.RequestedNotAfterUnixNano > r.Source.ExpiresUnixNano || r.Digest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "TurnContinuation start request is incomplete")
	}
	if r.ExecutionScopeDigest != r.Source.ExecutionScopeDigest || r.RunID != r.Source.RunID || r.ExpectedActiveContext.ExecutionScopeDigest != r.ExecutionScopeDigest || r.ExpectedActiveContext.RunID != r.RunID || r.ExpectedActiveContext.SessionID != r.Source.Session.ID || r.ExpectedActiveContext.TurnOrdinal != r.Source.Turn.Ordinal || r.ExpectedActiveContext.UpdatedUnixNano > r.Source.CheckedUnixNano {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "TurnContinuation source and expected ActiveContext differ")
	}
	expectedID, err := DeriveTurnContinuationAttemptIDV1(r)
	if err != nil || expectedID != r.AttemptID {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidReference, "TurnContinuation Attempt ID drifted")
	}
	digest, err := r.DigestV1()
	if err != nil || digest != r.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "TurnContinuation start request digest drifted")
	}
	return nil
}

func (r TurnContinuationStartRequestV1) ValidateCurrent(now time.Time) error {
	if err := r.Validate(); err != nil {
		return err
	}
	if now.IsZero() || now.UnixNano() < r.Source.CheckedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "TurnContinuation start clock regressed")
	}
	if !now.Before(time.Unix(0, r.RequestedNotAfterUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "TurnContinuation start request expired")
	}
	return nil
}

func SealTurnContinuationStartRequestV1(r TurnContinuationStartRequestV1) (TurnContinuationStartRequestV1, error) {
	r.ContractVersion = TurnContinuationContractVersionV1
	r.Revision = 1
	if r.Source.Turn.Ordinal == ^uint32(0) {
		return TurnContinuationStartRequestV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "TurnContinuation source Turn overflowed")
	}
	r.TargetTurn = r.Source.Turn.Ordinal + 1
	expectedID, err := DeriveTurnContinuationAttemptIDV1(r)
	if err != nil {
		return TurnContinuationStartRequestV1{}, err
	}
	if r.AttemptID != "" && r.AttemptID != expectedID {
		return TurnContinuationStartRequestV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidReference, "TurnContinuation start supplied another Attempt")
	}
	r.AttemptID = expectedID
	provided := r.Digest
	r.Digest = ""
	digest, err := r.DigestV1()
	if err != nil {
		return TurnContinuationStartRequestV1{}, err
	}
	if provided != "" && provided != digest {
		return TurnContinuationStartRequestV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "TurnContinuation start supplied another digest")
	}
	r.Digest = digest
	return r, r.Validate()
}

func (r TurnContinuationStartRequestV1) AttemptRefV1() TurnContinuationAttemptRefV1 {
	return TurnContinuationAttemptRefV1{ID: r.AttemptID, Revision: r.Revision, Digest: r.Digest, ExecutionScopeDigest: r.ExecutionScopeDigest, RunID: r.RunID, SessionID: r.Source.Session.ID, SourceTurn: r.Source.Turn.Ordinal, TargetTurn: r.TargetTurn}
}

type TurnContinuationCurrentV1 struct {
	ContractVersion                 string                         `json:"contract_version"`
	Revision                        core.Revision                  `json:"revision"`
	Start                           TurnContinuationStartRequestV1 `json:"start"`
	ActiveContext                   HarnessActiveContextRefV1      `json:"active_context"`
	AppliedContextRefresh           *ContextTurnRefreshResultV1    `json:"applied_context_refresh,omitempty"`
	CommitRequestDigest             core.Digest                    `json:"commit_request_digest,omitempty"`
	CommitRequestedNotAfterUnixNano int64                          `json:"commit_requested_not_after_unix_nano,omitempty"`
	PreviousDigest                  core.Digest                    `json:"previous_digest,omitempty"`
	CheckedUnixNano                 int64                          `json:"checked_unix_nano"`
	ExpiresUnixNano                 int64                          `json:"expires_unix_nano"`
	State                           TurnContinuationStateV1        `json:"state"`
	Digest                          core.Digest                    `json:"digest"`
}

func (c TurnContinuationCurrentV1) Clone() TurnContinuationCurrentV1 {
	clone := c
	if c.AppliedContextRefresh != nil {
		refresh := *c.AppliedContextRefresh
		if refresh.ApplySettlementRef != nil {
			value := *refresh.ApplySettlementRef
			refresh.ApplySettlementRef = &value
		}
		if refresh.CurrentPointerRef != nil {
			value := *refresh.CurrentPointerRef
			refresh.CurrentPointerRef = &value
		}
		clone.AppliedContextRefresh = &refresh
	}
	return clone
}

func (c TurnContinuationCurrentV1) DigestV1() (core.Digest, error) {
	copy := c.Clone()
	copy.Digest = ""
	return core.CanonicalJSONDigest("praxis.application.turn-continuation-current", TurnContinuationContractVersionV1, "TurnContinuationCurrentV1", copy)
}

func (c TurnContinuationCurrentV1) ValidateCurrent(now time.Time) error {
	if c.ContractVersion != TurnContinuationContractVersionV1 || c.Start.Validate() != nil || c.ActiveContext.Validate() != nil || now.IsZero() || c.CheckedUnixNano <= 0 || c.ExpiresUnixNano <= c.CheckedUnixNano || c.ExpiresUnixNano > c.Start.RequestedNotAfterUnixNano || now.UnixNano() < c.CheckedUnixNano || !now.Before(time.Unix(0, c.ExpiresUnixNano)) || c.Digest.Validate() != nil {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "TurnContinuation current is incomplete or expired")
	}
	if c.ActiveContext.ExecutionScopeDigest != c.Start.ExecutionScopeDigest || c.ActiveContext.RunID != c.Start.RunID || c.ActiveContext.SessionID != c.Start.Source.Session.ID {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "TurnContinuation ActiveContext belongs to another Session")
	}
	switch c.State {
	case TurnContinuationPendingV1:
		if c.Revision != 1 || c.PreviousDigest != "" || c.AppliedContextRefresh != nil || c.CommitRequestDigest != "" || c.CommitRequestedNotAfterUnixNano != 0 || c.ActiveContext != c.Start.ExpectedActiveContext {
			return core.NewError(core.ErrorConflict, core.ReasonInvalidState, "continuation_pending claimed a Context switch")
		}
	case TurnContinuationContextCurrentV1:
		if c.Revision != 2 || c.PreviousDigest.Validate() != nil || c.AppliedContextRefresh == nil || c.AppliedContextRefresh.Validate() != nil || c.AppliedContextRefresh.State != ContextTurnRefreshAppliedStateV1 || c.AppliedContextRefresh.AttemptRef != c.Start.ExpectedContextRefreshAttempt || c.CommitRequestDigest.Validate() != nil || c.CommitRequestedNotAfterUnixNano <= c.CheckedUnixNano || c.CommitRequestedNotAfterUnixNano > c.Start.RequestedNotAfterUnixNano || c.CommitRequestedNotAfterUnixNano > c.AppliedContextRefresh.ExpiresUnixNano || c.CheckedUnixNano < c.AppliedContextRefresh.CheckedUnixNano || c.ExpiresUnixNano > c.CommitRequestedNotAfterUnixNano {
			return core.NewError(core.ErrorConflict, core.ReasonInvalidState, "context_current lacks an exact applied Context refresh")
		}
		expected, err := NextHarnessActiveContextRefV1(c.Start.ExpectedActiveContext, c.Start.TargetTurn, *c.AppliedContextRefresh, c.ActiveContext.UpdatedUnixNano)
		if err != nil || expected != c.ActiveContext {
			return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "context_current did not CAS the exact immutable ContextFrame")
		}
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "TurnContinuation state is invalid")
	}
	digest, err := c.DigestV1()
	if err != nil || digest != c.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "TurnContinuation current digest drifted")
	}
	return nil
}

func (c TurnContinuationCurrentV1) ModelTurnAllowedV1(now time.Time) error {
	if c.State != TurnContinuationContextCurrentV1 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "next Model Turn is forbidden until ActiveContextRef CAS completes")
	}
	return c.ValidateCurrent(now)
}

func SealTurnContinuationPendingV1(start TurnContinuationStartRequestV1, checkedUnixNano, expiresUnixNano int64) (TurnContinuationCurrentV1, error) {
	now := time.Unix(0, checkedUnixNano)
	if err := start.ValidateCurrent(now); err != nil {
		return TurnContinuationCurrentV1{}, err
	}
	current := TurnContinuationCurrentV1{ContractVersion: TurnContinuationContractVersionV1, Revision: 1, Start: start, ActiveContext: start.ExpectedActiveContext, CheckedUnixNano: checkedUnixNano, ExpiresUnixNano: expiresUnixNano, State: TurnContinuationPendingV1}
	digest, err := current.DigestV1()
	if err != nil {
		return TurnContinuationCurrentV1{}, err
	}
	current.Digest = digest
	return current.Clone(), current.ValidateCurrent(now)
}

type TurnContinuationCommitRequestV1 struct {
	ContractVersion           string                     `json:"contract_version"`
	Pending                   TurnContinuationCurrentV1  `json:"pending"`
	ExpectedActiveContext     HarnessActiveContextRefV1  `json:"expected_active_context"`
	AppliedContextRefresh     ContextTurnRefreshResultV1 `json:"applied_context_refresh"`
	RequestedNotAfterUnixNano int64                      `json:"requested_not_after_unix_nano"`
	Digest                    core.Digest                `json:"digest"`
}

func (r TurnContinuationCommitRequestV1) DigestV1() (core.Digest, error) {
	copy := r
	copy.Pending = r.Pending.Clone()
	copy.Digest = ""
	return core.CanonicalJSONDigest("praxis.application.turn-continuation-commit", TurnContinuationContractVersionV1, "TurnContinuationCommitRequestV1", copy)
}

func (r TurnContinuationCommitRequestV1) ValidateCurrent(now time.Time) error {
	if r.ContractVersion != TurnContinuationContractVersionV1 || r.Pending.ValidateCurrent(now) != nil || r.Pending.State != TurnContinuationPendingV1 || r.ExpectedActiveContext.Validate() != nil || r.ExpectedActiveContext != r.Pending.ActiveContext || r.AppliedContextRefresh.Validate() != nil || r.AppliedContextRefresh.State != ContextTurnRefreshAppliedStateV1 || r.AppliedContextRefresh.AttemptRef != r.Pending.Start.ExpectedContextRefreshAttempt || r.RequestedNotAfterUnixNano <= 0 || r.RequestedNotAfterUnixNano > r.Pending.ExpiresUnixNano || r.RequestedNotAfterUnixNano > r.AppliedContextRefresh.ExpiresUnixNano || now.UnixNano() < r.AppliedContextRefresh.CheckedUnixNano || !now.Before(time.Unix(0, r.RequestedNotAfterUnixNano)) || r.Digest.Validate() != nil {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "TurnContinuation commit request is incomplete or expired")
	}
	digest, err := r.DigestV1()
	if err != nil || digest != r.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "TurnContinuation commit request digest drifted")
	}
	return nil
}

func SealTurnContinuationCommitRequestV1(r TurnContinuationCommitRequestV1, now time.Time) (TurnContinuationCommitRequestV1, error) {
	r.ContractVersion = TurnContinuationContractVersionV1
	r.Pending = r.Pending.Clone()
	r.ExpectedActiveContext = r.Pending.ActiveContext
	provided := r.Digest
	r.Digest = ""
	digest, err := r.DigestV1()
	if err != nil {
		return TurnContinuationCommitRequestV1{}, err
	}
	if provided != "" && provided != digest {
		return TurnContinuationCommitRequestV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "TurnContinuation commit supplied another digest")
	}
	r.Digest = digest
	return r, r.ValidateCurrent(now)
}

func NextHarnessActiveContextRefV1(current HarnessActiveContextRefV1, targetTurn uint32, refresh ContextTurnRefreshResultV1, updatedUnixNano int64) (HarnessActiveContextRefV1, error) {
	if current.Validate() != nil || current.Revision == ^core.Revision(0) || current.TurnOrdinal == ^uint32(0) || targetTurn != current.TurnOrdinal+1 || refresh.Validate() != nil || refresh.State != ContextTurnRefreshAppliedStateV1 || refresh.CurrentPointerRef == nil || updatedUnixNano < refresh.CheckedUnixNano {
		return HarnessActiveContextRefV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidState, "next Harness ActiveContext inputs are incomplete")
	}
	next := current
	next.Revision++
	next.TurnOrdinal = targetTurn
	next.ManifestRef = refresh.ManifestRef
	next.FrameRef = refresh.FrameRef
	next.GenerationRef = refresh.GenerationRef
	next.ContextCurrentPointerRef = *refresh.CurrentPointerRef
	next.UpdatedUnixNano = updatedUnixNano
	next.Digest = ""
	return SealHarnessActiveContextRefV1(next)
}

func SealTurnContinuationContextCurrentV1(request TurnContinuationCommitRequestV1, checkedUnixNano, expiresUnixNano int64) (TurnContinuationCurrentV1, error) {
	now := time.Unix(0, checkedUnixNano)
	if err := request.ValidateCurrent(now); err != nil {
		return TurnContinuationCurrentV1{}, err
	}
	next, err := NextHarnessActiveContextRefV1(request.ExpectedActiveContext, request.Pending.Start.TargetTurn, request.AppliedContextRefresh, checkedUnixNano)
	if err != nil {
		return TurnContinuationCurrentV1{}, err
	}
	refresh := request.AppliedContextRefresh
	current := TurnContinuationCurrentV1{ContractVersion: TurnContinuationContractVersionV1, Revision: 2, Start: request.Pending.Start, ActiveContext: next, AppliedContextRefresh: &refresh, CommitRequestDigest: request.Digest, CommitRequestedNotAfterUnixNano: request.RequestedNotAfterUnixNano, PreviousDigest: request.Pending.Digest, CheckedUnixNano: checkedUnixNano, ExpiresUnixNano: expiresUnixNano, State: TurnContinuationContextCurrentV1}
	digest, err := current.DigestV1()
	if err != nil {
		return TurnContinuationCurrentV1{}, err
	}
	current.Digest = digest
	if err := ValidateTurnContinuationTransitionV1(request.Pending, current, now); err != nil {
		return TurnContinuationCurrentV1{}, err
	}
	return current.Clone(), nil
}

func ValidateTurnContinuationTransitionV1(pending, next TurnContinuationCurrentV1, now time.Time) error {
	if pending.ValidateCurrent(now) != nil || pending.State != TurnContinuationPendingV1 || next.ValidateCurrent(now) != nil || next.State != TurnContinuationContextCurrentV1 || next.Revision != pending.Revision+1 || next.PreviousDigest != pending.Digest || next.Start.Digest != pending.Start.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "TurnContinuation successor does not bind the pending CAS fact")
	}
	return nil
}

type TurnContinuationInspectRequestV1 struct {
	ContractVersion string                       `json:"contract_version"`
	AttemptRef      TurnContinuationAttemptRefV1 `json:"attempt_ref"`
	Digest          core.Digest                  `json:"digest"`
}

func (r TurnContinuationInspectRequestV1) DigestV1() (core.Digest, error) {
	copy := r
	copy.Digest = ""
	return core.CanonicalJSONDigest("praxis.application.turn-continuation-inspect", TurnContinuationContractVersionV1, "TurnContinuationInspectRequestV1", copy)
}

func (r TurnContinuationInspectRequestV1) Validate() error {
	if r.ContractVersion != TurnContinuationContractVersionV1 || r.AttemptRef.Validate() != nil || r.Digest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "TurnContinuation Inspect request is incomplete")
	}
	digest, err := r.DigestV1()
	if err != nil || digest != r.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "TurnContinuation Inspect request digest drifted")
	}
	return nil
}

func SealTurnContinuationInspectRequestV1(r TurnContinuationInspectRequestV1) (TurnContinuationInspectRequestV1, error) {
	r.ContractVersion = TurnContinuationContractVersionV1
	provided := r.Digest
	r.Digest = ""
	digest, err := r.DigestV1()
	if err != nil {
		return TurnContinuationInspectRequestV1{}, err
	}
	if provided != "" && provided != digest {
		return TurnContinuationInspectRequestV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "TurnContinuation Inspect supplied another digest")
	}
	r.Digest = digest
	return r, r.Validate()
}
