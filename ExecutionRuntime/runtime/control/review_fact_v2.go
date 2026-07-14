package control

import (
	"reflect"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type reviewDecisionContextV2 struct {
	PolicyCurrent                bool
	ActorAuthorityCurrent        bool
	ReviewerAuthorityCurrent     bool
	ReviewerBindingCurrent       bool
	ExecutionScopeCurrent        bool
	InvocationEffectSettled      bool
	OperationNotRequiredByPolicy bool
	SelfReviewAllowedByPolicy    bool
	MaximumExpiresUnixNano       int64
}

type conditionSatisfactionContextV2 struct {
	PolicyCurrent             bool
	ExecutionScopeCurrent     bool
	OwnersAndAuthorityCurrent bool
	MaximumExpiresUnixNano    int64
}

func NewPendingReviewCaseV2(candidate ports.ReviewCandidateV2, now time.Time) (ports.ReviewCaseFactV2, error) {
	if now.IsZero() || candidate.RequestedUnixNano > now.UnixNano() || !now.Before(time.Unix(0, candidate.ExpiresUnixNano)) {
		return ports.ReviewCaseFactV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonReviewVerdictMissing, "pending review case requires the injected request time before expiry")
	}
	digest, err := candidate.DigestV2()
	if err != nil {
		return ports.ReviewCaseFactV2{}, err
	}
	fact := ports.ReviewCaseFactV2{Candidate: candidate, CandidateDigest: digest, State: ports.ReviewCasePending, Revision: 1, UpdatedUnixNano: now.UnixNano()}
	return fact, fact.Validate()
}

func ValidateReviewCaseTransitionV2(current, next ports.ReviewCaseFactV2, now time.Time) error {
	if err := current.Validate(); err != nil {
		return err
	}
	if err := next.Validate(); err != nil {
		return err
	}
	if now.IsZero() || now.UnixNano() < current.UpdatedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "review case transition clock regressed")
	}
	if current.CandidateDigest != next.CandidateDigest || next.Revision != current.Revision+1 || next.UpdatedUnixNano != now.UnixNano() {
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "review candidate is immutable and revision must advance once")
	}
	if current.State != ports.ReviewCasePending {
		return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "review case is already decided or invalidated")
	}
	if next.State == ports.ReviewCaseDecided {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewCandidateConflict, "case may become decided only through atomic DecideReview")
	}
	if next.State == ports.ReviewCaseExpired {
		if now.Before(time.Unix(0, current.Candidate.ExpiresUnixNano)) {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "review case cannot expire before exact boundary")
		}
		return nil
	}
	if next.State == ports.ReviewCaseRevoked {
		return nil
	}
	return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidTransition, "review case transition is not allowed")
}

func validateReviewDecisionGovernanceV2(current ports.ReviewCaseFactV2, verdict ports.ReviewVerdictFactV2, context reviewDecisionContextV2, now time.Time) (ports.ReviewCaseFactV2, error) {
	if err := current.Validate(); err != nil {
		return ports.ReviewCaseFactV2{}, err
	}
	if err := verdict.Validate(); err != nil {
		return ports.ReviewCaseFactV2{}, err
	}
	if now.IsZero() || now.UnixNano() < current.UpdatedUnixNano {
		return ports.ReviewCaseFactV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "review decision clock regressed")
	}
	if current.State != ports.ReviewCasePending || verdict.CaseID != current.Candidate.ID || verdict.CaseRevision != current.Revision || verdict.CandidateDigest != current.CandidateDigest || verdict.IntentID != current.Candidate.IntentID || verdict.IntentRevision != current.Candidate.IntentRevision || verdict.SubjectDigest != current.Candidate.SubjectDigest || verdict.Policy != current.Candidate.Policy || verdict.ActorAuthority != current.Candidate.ActorAuthority || verdict.ReviewerAuthority != current.Candidate.ReviewerAuthority {
		return ports.ReviewCaseFactV2{}, core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "verdict drifted from exact pending candidate")
	}
	if verdict.DecidedUnixNano != now.UnixNano() || verdict.UpdatedUnixNano != now.UnixNano() || verdict.DecidedUnixNano < current.Candidate.RequestedUnixNano || !now.Before(time.Unix(0, current.Candidate.ExpiresUnixNano)) || verdict.ExpiresUnixNano > current.Candidate.ExpiresUnixNano || context.MaximumExpiresUnixNano <= 0 || verdict.ExpiresUnixNano > context.MaximumExpiresUnixNano {
		return ports.ReviewCaseFactV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "verdict time or TTL exceeds current governing facts")
	}
	if !context.PolicyCurrent || !context.ActorAuthorityCurrent || !context.ReviewerAuthorityCurrent || !context.ReviewerBindingCurrent || !context.ExecutionScopeCurrent {
		return ports.ReviewCaseFactV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "review policy, authority, binding and execution watermarks must be current")
	}
	if current.Candidate.ActorAuthority.Ref == current.Candidate.ReviewerAuthority.Ref && !context.SelfReviewAllowedByPolicy {
		return ports.ReviewCaseFactV2{}, core.NewError(core.ErrorForbidden, core.ReasonReviewVerdictStale, "self-review is not authorized")
	}
	if verdict.Basis == ports.ReviewBasisPolicyNotRequired {
		if !context.OperationNotRequiredByPolicy || strings.TrimSpace(verdict.PolicyDecisionRef) == "" || verdict.InvocationEffect != nil {
			return ports.ReviewCaseFactV2{}, core.NewError(core.ErrorForbidden, core.ReasonReviewVerdictStale, "operation_not_required requires explicit current policy and no reviewer invocation")
		}
	} else {
		switch current.Candidate.InvocationMode {
		case ports.ReviewInvocationHuman:
			if verdict.Basis != ports.ReviewBasisHuman || verdict.InvocationEffect != nil {
				return ports.ReviewCaseFactV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewCandidateConflict, "human review cannot use automatic evidence")
			}
		case ports.ReviewInvocationAutomaticLocal, ports.ReviewInvocationAutomaticRemote:
			if verdict.Basis != ports.ReviewBasisAutomatic || current.Candidate.InvocationEffect == nil || verdict.InvocationEffect == nil || *verdict.InvocationEffect != *current.Candidate.InvocationEffect || !context.InvocationEffectSettled {
				return ports.ReviewCaseFactV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewRemoteEffectRequired, "automatic review requires its exact independently settled Effect")
			}
		}
	}
	if verdict.ID != current.Candidate.ID {
		return ports.ReviewCaseFactV2{}, core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "verdict id must remain the preallocated review id")
	}
	if verdict.State == ports.ReviewVerdictConditional {
		for _, condition := range verdict.Conditions {
			if condition.ExpiresUnixNano > verdict.ExpiresUnixNano {
				return ports.ReviewCaseFactV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewConditionUnsatisfied, "condition TTL cannot exceed verdict TTL")
			}
		}
	}
	digest, err := verdict.DigestV2()
	if err != nil {
		return ports.ReviewCaseFactV2{}, err
	}
	next := current
	next.State, next.Revision, next.UpdatedUnixNano = ports.ReviewCaseDecided, current.Revision+1, now.UnixNano()
	next.VerdictID, next.VerdictRevision, next.VerdictDigest = verdict.ID, verdict.Revision, digest
	return next, next.Validate()
}

func ValidateReviewVerdictTransitionV2(current, next ports.ReviewVerdictFactV2, now time.Time) error {
	if err := current.Validate(); err != nil {
		return err
	}
	if err := next.Validate(); err != nil {
		return err
	}
	if now.IsZero() || now.UnixNano() < current.UpdatedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "verdict transition clock regressed")
	}
	immutableCurrent, immutableNext := current, next
	immutableCurrent.State, immutableNext.State = "", ""
	immutableCurrent.Revision, immutableNext.Revision = 0, 0
	immutableCurrent.UpdatedUnixNano, immutableNext.UpdatedUnixNano = 0, 0
	immutableCurrent.InvalidationReason, immutableNext.InvalidationReason = "", ""
	if immutableCurrent.DecisionEvidence == nil {
		immutableCurrent.DecisionEvidence = []ports.ReviewEvidenceRefV2{}
	}
	if immutableNext.DecisionEvidence == nil {
		immutableNext.DecisionEvidence = []ports.ReviewEvidenceRefV2{}
	}
	if immutableCurrent.Conditions == nil {
		immutableCurrent.Conditions = []ports.ReviewConditionV2{}
	}
	if immutableNext.Conditions == nil {
		immutableNext.Conditions = []ports.ReviewConditionV2{}
	}
	if !reflect.DeepEqual(immutableCurrent, immutableNext) || next.Revision != current.Revision+1 || next.UpdatedUnixNano != now.UnixNano() {
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "verdict decision is immutable and revision must advance once")
	}
	if current.State == ports.ReviewVerdictExpired || current.State == ports.ReviewVerdictRevoked {
		return core.NewError(core.ErrorConflict, core.ReasonReviewVerdictStale, "invalidated verdict is terminal")
	}
	if next.State == ports.ReviewVerdictExpired {
		if now.Before(time.Unix(0, current.ExpiresUnixNano)) {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "verdict cannot expire before exact boundary")
		}
		return nil
	}
	if next.State == ports.ReviewVerdictRevoked {
		return nil
	}
	return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidTransition, "verdict cannot flip its decision")
}

func validateConditionSatisfactionGovernanceV2(current, next ports.ConditionSatisfactionFactV2, verdict ports.ReviewVerdictFactV2, context conditionSatisfactionContextV2, now time.Time) error {
	if err := current.Validate(); err != nil {
		return err
	}
	if err := next.Validate(); err != nil {
		return err
	}
	if err := verdict.Validate(); err != nil {
		return err
	}
	if now.IsZero() || now.UnixNano() < current.UpdatedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "condition satisfaction clock regressed")
	}
	if current.ID != next.ID || current.VerdictID != next.VerdictID || current.VerdictDigest != next.VerdictDigest || current.CandidateDigest != next.CandidateDigest || current.SubjectDigest != next.SubjectDigest || current.ConditionsDigest != next.ConditionsDigest || current.IntentID != next.IntentID || current.IntentRevision != next.IntentRevision || current.Policy != next.Policy || !ports.SameExecutionScopeV2(current.Scope, next.Scope) || current.RunID != next.RunID || current.ActionScopeDigest != next.ActionScopeDigest || current.CurrentScope != next.CurrentScope || next.Revision != current.Revision+1 || next.UpdatedUnixNano != now.UnixNano() {
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "condition satisfaction subject is immutable")
	}
	if current.State == ports.ConditionSatisfactionExpired || current.State == ports.ConditionSatisfactionRevoked {
		return core.NewError(core.ErrorConflict, core.ReasonReviewConditionUnsatisfied, "invalidated satisfaction fact is terminal")
	}
	if next.State == ports.ConditionSatisfied {
		if current.State != ports.ConditionSatisfactionPending || verdict.State != ports.ReviewVerdictConditional || !context.PolicyCurrent || !context.ExecutionScopeCurrent || !context.OwnersAndAuthorityCurrent || context.MaximumExpiresUnixNano <= 0 || next.ExpiresUnixNano > context.MaximumExpiresUnixNano || next.ExpiresUnixNano > verdict.ExpiresUnixNano || next.SatisfiedUnixNano != now.UnixNano() {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewConditionUnsatisfied, "all current condition owners, authorities, scope and TTL are required")
		}
		if err := MatchConditionProofsV2(verdict.Conditions, next.Proofs); err != nil {
			return err
		}
		return nil
	}
	if next.State == ports.ConditionSatisfactionExpired {
		if current.State == ports.ConditionSatisfied && (!reflect.DeepEqual(current.Proofs, next.Proofs) || current.ProofsDigest != next.ProofsDigest || current.SatisfiedUnixNano != next.SatisfiedUnixNano) {
			return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "satisfaction invalidation cannot rewrite proof evidence")
		}
		if now.Before(time.Unix(0, current.ExpiresUnixNano)) {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewConditionUnsatisfied, "satisfaction cannot expire before exact boundary")
		}
		return nil
	}
	if next.State == ports.ConditionSatisfactionRevoked {
		if current.State == ports.ConditionSatisfied && (!reflect.DeepEqual(current.Proofs, next.Proofs) || current.ProofsDigest != next.ProofsDigest || current.SatisfiedUnixNano != next.SatisfiedUnixNano) {
			return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "satisfaction invalidation cannot rewrite proof evidence")
		}
		return nil
	}
	return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidTransition, "condition satisfaction transition is not allowed")
}

// ValidateReviewDecisionTransitionV2 validates only the atomic Case->Verdict
// linkage. It is a raw Fact Owner primitive and does not authorize a review;
// Application callers must use ReviewGovernanceGatewayV2.
func ValidateReviewDecisionTransitionV2(current ports.ReviewCaseFactV2, verdict ports.ReviewVerdictFactV2, now time.Time) (ports.ReviewCaseFactV2, error) {
	return validateReviewDecisionGovernanceV2(current, verdict, reviewDecisionContextV2{
		PolicyCurrent:                true,
		ActorAuthorityCurrent:        true,
		ReviewerAuthorityCurrent:     true,
		ReviewerBindingCurrent:       true,
		ExecutionScopeCurrent:        true,
		InvocationEffectSettled:      verdict.InvocationEffect != nil,
		OperationNotRequiredByPolicy: verdict.Basis == ports.ReviewBasisPolicyNotRequired,
		SelfReviewAllowedByPolicy:    true,
		MaximumExpiresUnixNano:       verdict.ExpiresUnixNano,
	}, now)
}

// ValidateConditionSatisfactionTransitionV2 validates only the atomic
// satisfaction journal transition. It does not attest that policy, owners,
// authority or execution scope are current; the governed gateway does that.
func ValidateConditionSatisfactionTransitionV2(current, next ports.ConditionSatisfactionFactV2, verdict ports.ReviewVerdictFactV2, now time.Time) error {
	return validateConditionSatisfactionGovernanceV2(current, next, verdict, conditionSatisfactionContextV2{
		PolicyCurrent:             true,
		ExecutionScopeCurrent:     true,
		OwnersAndAuthorityCurrent: true,
		MaximumExpiresUnixNano:    next.ExpiresUnixNano,
	}, now)
}

func NewPendingConditionSatisfactionV2(verdict ports.ReviewVerdictFactV2, id string, scope ports.ExecutionScopeBindingRefV2, executionScope core.ExecutionScope, runID core.AgentRunID, actionScopeDigest core.Digest, now time.Time) (ports.ConditionSatisfactionFactV2, error) {
	if err := verdict.Validate(); err != nil {
		return ports.ConditionSatisfactionFactV2{}, err
	}
	if verdict.State != ports.ReviewVerdictConditional || strings.TrimSpace(id) == "" || now.IsZero() || !now.Before(time.Unix(0, verdict.ExpiresUnixNano)) {
		return ports.ConditionSatisfactionFactV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonReviewConditionUnsatisfied, "pending satisfaction requires a current conditional verdict")
	}
	verdictDigest, err := verdict.DigestV2()
	if err != nil {
		return ports.ConditionSatisfactionFactV2{}, err
	}
	fact := ports.ConditionSatisfactionFactV2{ID: id, VerdictID: verdict.ID, VerdictRevision: verdict.Revision, VerdictDigest: verdictDigest, CandidateDigest: verdict.CandidateDigest, IntentID: verdict.IntentID, IntentRevision: verdict.IntentRevision, SubjectDigest: verdict.SubjectDigest, ConditionsDigest: verdict.ConditionsDigest, Policy: verdict.Policy, Scope: executionScope, RunID: runID, ActionScopeDigest: actionScopeDigest, CurrentScope: scope, Proofs: []ports.ReviewConditionProofV2{}, State: ports.ConditionSatisfactionPending, Revision: 1, UpdatedUnixNano: now.UnixNano(), ExpiresUnixNano: verdict.ExpiresUnixNano}
	return fact, fact.Validate()
}

func MatchConditionProofsV2(conditions []ports.ReviewConditionV2, proofs []ports.ReviewConditionProofV2) error {
	if len(conditions) == 0 || len(conditions) != len(proofs) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewConditionUnsatisfied, "every required condition needs exactly one proof")
	}
	for i := range conditions {
		condition, proof := conditions[i], proofs[i]
		if proof.ConditionID != condition.ID || proof.ConditionRevision != condition.Revision || proof.ConstraintDigest != condition.ConstraintDigest || proof.Owner != condition.SatisfactionOwner || proof.ScopeDigest != condition.ScopeDigest || proof.Authority != condition.Authority || proof.ExpiresUnixNano > condition.ExpiresUnixNano {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewConditionUnsatisfied, "condition proof drifted from owner, authority, scope or revision")
		}
	}
	return nil
}
