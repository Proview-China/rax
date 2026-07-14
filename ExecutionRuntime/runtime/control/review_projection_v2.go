package control

import (
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// BuildDispatchReviewProjectionV2 maps authoritative Review facts into the
// narrow read-only view consumed by the dispatch gateway. It creates no second
// Review owner and cannot dispatch or commit anything.
func BuildDispatchReviewProjectionV2(caseFact ports.ReviewCaseFactV2, verdict ports.ReviewVerdictFactV2, satisfaction *ports.ConditionSatisfactionFactV2, intent ports.EffectIntentV2, now time.Time) (ports.DispatchReviewFactV2, error) {
	if err := caseFact.Validate(); err != nil {
		return ports.DispatchReviewFactV2{}, err
	}
	if err := verdict.Validate(); err != nil {
		return ports.DispatchReviewFactV2{}, err
	}
	if now.IsZero() || caseFact.State != ports.ReviewCaseDecided || verdict.State == ports.ReviewVerdictExpired || verdict.State == ports.ReviewVerdictRevoked || !now.Before(time.Unix(0, verdict.ExpiresUnixNano)) {
		return ports.DispatchReviewFactV2{}, core.NewError(core.ErrorForbidden, core.ReasonReviewVerdictStale, "current decided non-expired review verdict is required")
	}
	verdictDigest, err := verdict.DigestV2()
	if err != nil {
		return ports.DispatchReviewFactV2{}, err
	}
	subjectDigest, err := intent.ReviewSubjectDigestV2()
	if err != nil {
		return ports.DispatchReviewFactV2{}, err
	}
	if caseFact.VerdictID != verdict.ID || caseFact.VerdictRevision != verdict.Revision || caseFact.VerdictDigest != verdictDigest || verdict.CaseID != caseFact.Candidate.ID || verdict.CandidateDigest != caseFact.CandidateDigest || verdict.SubjectDigest != subjectDigest || intent.Review.Ref != caseFact.Candidate.ID || intent.Review.Digest != caseFact.CandidateDigest || intent.Review.Revision != caseFact.Candidate.Revision || intent.Review.PolicyDigest != verdict.Policy.Digest || verdict.IntentID != intent.ID || verdict.IntentRevision != intent.Revision || caseFact.Candidate.SubjectDigest != subjectDigest || caseFact.Candidate.PayloadSchema != intent.Payload.Schema || caseFact.Candidate.PayloadDigest != intent.Payload.ContentDigest || caseFact.Candidate.PayloadRevision != intent.PayloadRevision || !ports.SameExecutionScopeV2(caseFact.Candidate.Scope, intent.Scope) || caseFact.Candidate.RunID != intent.RunID || caseFact.Candidate.ActionScopeDigest != intent.ActionScopeDigest || caseFact.Candidate.SubjectProvider != intent.Provider || caseFact.Candidate.CurrentScope != intent.CurrentScope || caseFact.Candidate.RiskClass != intent.RiskClass {
		return ports.DispatchReviewFactV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewCandidateConflict, "review projection drifted from exact effect subject or verdict")
	}
	decision := ports.ReviewDecisionAccepted
	if verdict.Basis == ports.ReviewBasisPolicyNotRequired {
		decision = ports.ReviewDecisionOperationNotRequired
	}
	if verdict.State == ports.ReviewVerdictRejected {
		decision = ports.ReviewDecisionRejected
	}
	if verdict.State == ports.ReviewVerdictConditional {
		decision = ports.ReviewDecisionConditional
	}
	expires := verdict.ExpiresUnixNano
	projection := ports.DispatchReviewFactV2{Ref: caseFact.Candidate.ID, Digest: caseFact.CandidateDigest, Revision: caseFact.Candidate.Revision, IntentID: intent.ID, IntentRevision: intent.Revision, SubjectDigest: subjectDigest, CandidateDigest: caseFact.CandidateDigest, VerdictDigest: verdictDigest, VerdictRevision: verdict.Revision, PayloadDigest: intent.Payload.ContentDigest, PayloadRevision: intent.PayloadRevision, ScopeDigest: intent.ActionScopeDigest, PolicyDigest: verdict.Policy.Digest, PolicyDecisionRef: verdict.PolicyDecisionRef, ActorAuthorityDigest: verdict.ActorAuthority.Digest, ReviewerAuthorityDigest: verdict.ReviewerAuthority.Digest, ConditionsDigest: verdict.ConditionsDigest, EvidenceDigest: verdict.DecisionEvidenceDigest, Decision: decision, ExpiresUnixNano: expires}
	if verdict.State == ports.ReviewVerdictConditional {
		if satisfaction == nil {
			return ports.DispatchReviewFactV2{}, core.NewError(core.ErrorForbidden, core.ReasonReviewConditionUnsatisfied, "conditional verdict lacks satisfaction fact")
		}
		if err := satisfaction.Validate(); err != nil {
			return ports.DispatchReviewFactV2{}, err
		}
		satisfactionDigest, err := satisfaction.DigestV2()
		if err != nil {
			return ports.DispatchReviewFactV2{}, err
		}
		if satisfaction.State != ports.ConditionSatisfied || satisfaction.VerdictID != verdict.ID || satisfaction.VerdictRevision != verdict.Revision || satisfaction.VerdictDigest != verdictDigest || satisfaction.CandidateDigest != caseFact.CandidateDigest || satisfaction.SubjectDigest != subjectDigest || satisfaction.IntentID != intent.ID || satisfaction.IntentRevision != intent.Revision || satisfaction.ConditionsDigest != verdict.ConditionsDigest || satisfaction.Policy != verdict.Policy || !ports.SameExecutionScopeV2(satisfaction.Scope, intent.Scope) || satisfaction.RunID != intent.RunID || satisfaction.ActionScopeDigest != intent.ActionScopeDigest || satisfaction.CurrentScope != intent.CurrentScope || !now.Before(time.Unix(0, satisfaction.ExpiresUnixNano)) {
			return ports.DispatchReviewFactV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewConditionUnsatisfied, "satisfaction fact drifted from exact conditional verdict")
		}
		if err := MatchConditionProofsV2(verdict.Conditions, satisfaction.Proofs); err != nil {
			return ports.DispatchReviewFactV2{}, err
		}
		if satisfaction.ExpiresUnixNano < expires {
			expires = satisfaction.ExpiresUnixNano
		}
		projection.SatisfactionRef, projection.SatisfactionRevision, projection.SatisfactionDigest = satisfaction.ID, satisfaction.Revision, satisfactionDigest
		projection.ExpiresUnixNano = expires
	}
	if err := projection.ValidateCurrent(intent.Review, intent, now); err != nil {
		return ports.DispatchReviewFactV2{}, err
	}
	return projection, nil
}
