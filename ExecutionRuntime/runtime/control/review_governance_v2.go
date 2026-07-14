package control

import (
	"context"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type CreateGovernedReviewCaseRequestV2 struct {
	EffectID               core.EffectIntentID     `json:"effect_id"`
	ExpectedEffectRevision core.Revision           `json:"expected_effect_revision"`
	Candidate              ports.ReviewCandidateV2 `json:"candidate"`
}

type DecideGovernedReviewRequestV2 struct {
	CaseID               string                               `json:"case_id"`
	ExpectedCaseRevision core.Revision                        `json:"expected_case_revision"`
	Observation          ports.ReviewAttestationObservationV2 `json:"reviewer_observation"`
	RequestedTTL         time.Duration                        `json:"requested_ttl"`
}

type SatisfyGovernedConditionsRequestV2 struct {
	SatisfactionID   string                         `json:"satisfaction_id"`
	ExpectedRevision core.Revision                  `json:"expected_revision"`
	Proofs           []ports.ReviewConditionProofV2 `json:"proofs"`
	RequestedTTL     time.Duration                  `json:"requested_ttl"`
}

// ReviewGovernanceGatewayV2 is the only Application-facing Review authority
// entry. It re-reads current facts and calls the Review Owner's raw atomic
// journal; it never dispatches a provider or commits a domain result.
type ReviewGovernanceGatewayV2 struct {
	Facts         ports.ReviewVerdictFactPortV2
	Policies      ports.ReviewPolicyFactReaderV2
	Authority     ports.AuthorityFactReaderV2
	CurrentScopes ports.ExecutionScopeFactReaderV2
	Bindings      BindingFactPortV2
	Effects       EffectFactPortV2
	Clock         func() time.Time
}

func (g ReviewGovernanceGatewayV2) CreateCase(ctx context.Context, request CreateGovernedReviewCaseRequestV2) (ports.ReviewCaseFactV2, error) {
	if err := g.validate(); err != nil {
		return ports.ReviewCaseFactV2{}, err
	}
	now := g.Clock()
	if now.IsZero() {
		return ports.ReviewCaseFactV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "review gateway clock returned zero")
	}
	if err := request.Candidate.Validate(); err != nil {
		return ports.ReviewCaseFactV2{}, err
	}
	effect, err := g.Effects.InspectEffect(ctx, request.EffectID)
	if err != nil {
		return ports.ReviewCaseFactV2{}, err
	}
	if effect.Revision != request.ExpectedEffectRevision || effect.Intent.ID != request.EffectID {
		return ports.ReviewCaseFactV2{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "persisted review target Effect revision drifted")
	}
	if err := requireReviewableEffectV2(effect); err != nil {
		return ports.ReviewCaseFactV2{}, err
	}
	intent := effect.Intent
	subjectDigest, err := intent.ReviewSubjectDigestV2()
	if err != nil {
		return ports.ReviewCaseFactV2{}, err
	}
	candidateDigest, err := request.Candidate.DigestV2()
	if err != nil {
		return ports.ReviewCaseFactV2{}, err
	}
	if err := validateCandidateAgainstPersistedIntentV2(request.Candidate, candidateDigest, subjectDigest, intent); err != nil {
		return ports.ReviewCaseFactV2{}, err
	}
	if request.Candidate.RequestedUnixNano > effect.UpdatedUnixNano || effect.UpdatedUnixNano > now.UnixNano() || !now.Before(time.Unix(0, request.Candidate.ExpiresUnixNano)) {
		return ports.ReviewCaseFactV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "review candidate must predate the persisted Effect and remain current")
	}
	current, err := g.inspectCurrent(ctx, request.Candidate, intent, now)
	if err != nil {
		return ports.ReviewCaseFactV2{}, err
	}
	if request.Candidate.ExpiresUnixNano > current.maximumExpiry {
		return ports.ReviewCaseFactV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "candidate TTL exceeds a current governing fact")
	}
	caseFact, err := NewPendingReviewCaseV2(request.Candidate, now)
	if err != nil {
		return ports.ReviewCaseFactV2{}, err
	}
	return g.Facts.CreateReviewCase(ctx, caseFact)
}

func (g ReviewGovernanceGatewayV2) Decide(ctx context.Context, request DecideGovernedReviewRequestV2) (ports.DecideReviewResultV2, error) {
	if err := g.validate(); err != nil {
		return ports.DecideReviewResultV2{}, err
	}
	if request.RequestedTTL <= 0 || strings.TrimSpace(request.CaseID) == "" {
		return ports.DecideReviewResultV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonReviewVerdictMissing, "review decision requires case and bounded TTL")
	}
	now := g.Clock()
	caseFact, err := g.Facts.InspectReviewCase(ctx, request.CaseID)
	if err != nil {
		return ports.DecideReviewResultV2{}, err
	}
	effect, err := g.Effects.InspectEffect(ctx, caseFact.Candidate.IntentID)
	if err != nil {
		return ports.DecideReviewResultV2{}, err
	}
	if err := requireReviewableEffectV2(effect); err != nil {
		return ports.DecideReviewResultV2{}, err
	}
	if effect.Intent.Revision != caseFact.Candidate.IntentRevision {
		return ports.DecideReviewResultV2{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "review target Effect intent revision drifted")
	}
	current, err := g.inspectCurrent(ctx, caseFact.Candidate, effect.Intent, now)
	if err != nil {
		return ports.DecideReviewResultV2{}, err
	}
	if err := request.Observation.Validate(); err != nil {
		return ports.DecideReviewResultV2{}, err
	}
	if request.Observation.CaseID != caseFact.Candidate.ID || request.Observation.CandidateDigest != caseFact.CandidateDigest || request.Observation.ReviewerBinding != caseFact.Candidate.ReviewerBinding || request.Observation.ReviewerAuthority != caseFact.Candidate.ReviewerAuthority || request.Observation.ObservedUnixNano > now.UnixNano() || request.Observation.ObservedUnixNano < caseFact.Candidate.RequestedUnixNano {
		return ports.DecideReviewResultV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewCandidateConflict, "reviewer observation drifted from current case")
	}
	evidenceDigest, err := ports.DigestReviewEvidenceV2(request.Observation.Evidence)
	if err != nil {
		return ports.DecideReviewResultV2{}, err
	}
	conditionsDigest := core.Digest("")
	if len(request.Observation.Conditions) != 0 {
		conditionsDigest, err = ports.DigestReviewConditionsV2(request.Observation.Conditions)
		if err != nil {
			return ports.DecideReviewResultV2{}, err
		}
	}
	expires := now.Add(request.RequestedTTL).UnixNano()
	for _, limit := range []int64{caseFact.Candidate.ExpiresUnixNano, current.maximumExpiry} {
		if limit < expires {
			expires = limit
		}
	}
	for _, condition := range request.Observation.Conditions {
		if condition.ExpiresUnixNano < expires {
			expires = condition.ExpiresUnixNano
		}
	}
	verdict := ports.ReviewVerdictFactV2{ID: caseFact.Candidate.ID, CaseID: caseFact.Candidate.ID, CaseRevision: caseFact.Revision, CandidateDigest: caseFact.CandidateDigest, IntentID: caseFact.Candidate.IntentID, IntentRevision: caseFact.Candidate.IntentRevision, SubjectDigest: caseFact.Candidate.SubjectDigest, Policy: caseFact.Candidate.Policy, ActorAuthority: caseFact.Candidate.ActorAuthority, ReviewerAuthority: caseFact.Candidate.ReviewerAuthority, State: request.Observation.ProposedState, Basis: request.Observation.Basis, PolicyDecisionRef: current.policy.PolicyDecisionRef, DecisionEvidence: append([]ports.ReviewEvidenceRefV2{}, request.Observation.Evidence...), DecisionEvidenceDigest: evidenceDigest, Conditions: append([]ports.ReviewConditionV2{}, request.Observation.Conditions...), ConditionsDigest: conditionsDigest, Revision: 1, DecidedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano(), ExpiresUnixNano: expires}
	if caseFact.Candidate.InvocationEffect != nil {
		settlementDigest, settled := g.inspectInvocationEffect(ctx, caseFact.Candidate)
		if !settled {
			return ports.DecideReviewResultV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewRemoteEffectRequired, "automatic reviewer Effect is not exactly settled")
		}
		copy := *caseFact.Candidate.InvocationEffect
		verdict.InvocationEffect, verdict.InvocationSettlementDigest = &copy, settlementDigest
		current.context.InvocationEffectSettled = true
	}
	current.context.OperationNotRequiredByPolicy = current.policy.OperationNotRequired
	current.context.SelfReviewAllowedByPolicy = current.policy.AllowSelfReview
	current.context.MaximumExpiresUnixNano = current.maximumExpiry
	if _, err := validateReviewDecisionGovernanceV2(caseFact, verdict, current.context, now); err != nil {
		return ports.DecideReviewResultV2{}, err
	}
	return g.Facts.DecideReview(ctx, ports.DecideReviewRequestV2{CaseID: request.CaseID, ExpectedCaseRevision: request.ExpectedCaseRevision, Verdict: verdict})
}

func (g ReviewGovernanceGatewayV2) CreateSatisfaction(ctx context.Context, verdictID, satisfactionID string) (ports.ConditionSatisfactionFactV2, error) {
	if err := g.validate(); err != nil {
		return ports.ConditionSatisfactionFactV2{}, err
	}
	now := g.Clock()
	verdict, err := g.Facts.InspectReviewVerdict(ctx, verdictID)
	if err != nil {
		return ports.ConditionSatisfactionFactV2{}, err
	}
	caseFact, err := g.Facts.InspectReviewCase(ctx, verdict.CaseID)
	if err != nil {
		return ports.ConditionSatisfactionFactV2{}, err
	}
	effect, err := g.Effects.InspectEffect(ctx, verdict.IntentID)
	if err != nil {
		return ports.ConditionSatisfactionFactV2{}, err
	}
	if _, err := g.inspectCurrent(ctx, caseFact.Candidate, effect.Intent, now); err != nil {
		return ports.ConditionSatisfactionFactV2{}, err
	}
	fact, err := NewPendingConditionSatisfactionV2(verdict, satisfactionID, caseFact.Candidate.CurrentScope, caseFact.Candidate.Scope, caseFact.Candidate.RunID, caseFact.Candidate.ActionScopeDigest, now)
	if err != nil {
		return ports.ConditionSatisfactionFactV2{}, err
	}
	return g.Facts.CreateConditionSatisfaction(ctx, fact)
}

func (g ReviewGovernanceGatewayV2) Satisfy(ctx context.Context, request SatisfyGovernedConditionsRequestV2) (ports.ConditionSatisfactionFactV2, error) {
	if err := g.validate(); err != nil {
		return ports.ConditionSatisfactionFactV2{}, err
	}
	if request.RequestedTTL <= 0 {
		return ports.ConditionSatisfactionFactV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonReviewConditionUnsatisfied, "condition satisfaction requires bounded TTL")
	}
	now := g.Clock()
	current, err := g.Facts.InspectConditionSatisfaction(ctx, request.SatisfactionID)
	if err != nil {
		return ports.ConditionSatisfactionFactV2{}, err
	}
	verdict, err := g.Facts.InspectReviewVerdict(ctx, current.VerdictID)
	if err != nil {
		return ports.ConditionSatisfactionFactV2{}, err
	}
	caseFact, err := g.Facts.InspectReviewCase(ctx, verdict.CaseID)
	if err != nil {
		return ports.ConditionSatisfactionFactV2{}, err
	}
	effect, err := g.Effects.InspectEffect(ctx, verdict.IntentID)
	if err != nil {
		return ports.ConditionSatisfactionFactV2{}, err
	}
	reviewCurrent, err := g.inspectCurrent(ctx, caseFact.Candidate, effect.Intent, now)
	if err != nil {
		return ports.ConditionSatisfactionFactV2{}, err
	}
	maximum := reviewCurrent.maximumExpiry
	for i, proof := range request.Proofs {
		if i >= len(verdict.Conditions) {
			return ports.ConditionSatisfactionFactV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewConditionUnsatisfied, "unexpected condition proof")
		}
		condition := verdict.Conditions[i]
		expiry, err := g.inspectConditionAuthority(ctx, condition, proof, caseFact.Candidate, now)
		if err != nil {
			return ports.ConditionSatisfactionFactV2{}, err
		}
		if expiry < maximum {
			maximum = expiry
		}
	}
	expires := now.Add(request.RequestedTTL).UnixNano()
	if maximum < expires {
		expires = maximum
	}
	if verdict.ExpiresUnixNano < expires {
		expires = verdict.ExpiresUnixNano
	}
	next := current
	next.State, next.Revision, next.Proofs, next.UpdatedUnixNano, next.SatisfiedUnixNano, next.ExpiresUnixNano = ports.ConditionSatisfied, current.Revision+1, append([]ports.ReviewConditionProofV2{}, request.Proofs...), now.UnixNano(), now.UnixNano(), expires
	next.ProofsDigest, err = ports.DigestConditionProofsV2(next.Proofs)
	if err != nil {
		return ports.ConditionSatisfactionFactV2{}, err
	}
	contextV2 := conditionSatisfactionContextV2{PolicyCurrent: true, ExecutionScopeCurrent: true, OwnersAndAuthorityCurrent: true, MaximumExpiresUnixNano: maximum}
	if err := validateConditionSatisfactionGovernanceV2(current, next, verdict, contextV2, now); err != nil {
		return ports.ConditionSatisfactionFactV2{}, err
	}
	return g.Facts.CompareAndSwapConditionSatisfaction(ctx, ports.ConditionSatisfactionCASRequestV2{SatisfactionID: current.ID, ExpectedRevision: request.ExpectedRevision, Next: next})
}

func (g ReviewGovernanceGatewayV2) InspectDispatchReview(ctx context.Context, ref string) (ports.DispatchReviewFactV2, error) {
	if err := g.validate(); err != nil {
		return ports.DispatchReviewFactV2{}, err
	}
	caseFact, err := g.Facts.InspectReviewCase(ctx, ref)
	if err != nil {
		return ports.DispatchReviewFactV2{}, err
	}
	verdict, err := g.Facts.InspectReviewVerdict(ctx, caseFact.VerdictID)
	if err != nil {
		return ports.DispatchReviewFactV2{}, err
	}
	effect, err := g.Effects.InspectEffect(ctx, caseFact.Candidate.IntentID)
	if err != nil {
		return ports.DispatchReviewFactV2{}, err
	}
	if effect.State != EffectAccepted && effect.State != EffectDispatchIntent {
		return ports.DispatchReviewFactV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidTransition, "dispatch review projection requires an accepted or write-ahead dispatch-intent Effect before provider reach")
	}
	now := g.Clock()
	current, err := g.inspectCurrent(ctx, caseFact.Candidate, effect.Intent, now)
	if err != nil {
		return ports.DispatchReviewFactV2{}, err
	}
	var satisfaction *ports.ConditionSatisfactionFactV2
	if verdict.State == ports.ReviewVerdictConditional {
		fact, err := g.Facts.InspectConditionSatisfactionByVerdict(ctx, verdict.ID)
		if err != nil {
			return ports.DispatchReviewFactV2{}, err
		}
		satisfaction = &fact
	}
	projection, err := BuildDispatchReviewProjectionV2(caseFact, verdict, satisfaction, effect.Intent, now)
	if err != nil {
		return ports.DispatchReviewFactV2{}, err
	}
	if current.maximumExpiry < projection.ExpiresUnixNano {
		projection.ExpiresUnixNano = current.maximumExpiry
	}
	return projection, projection.ValidateCurrent(effect.Intent.Review, effect.Intent, now)
}

type reviewCurrentFactsV2 struct {
	policy        ports.ReviewPolicyFactV2
	context       reviewDecisionContextV2
	maximumExpiry int64
}

func (g ReviewGovernanceGatewayV2) inspectCurrent(ctx context.Context, candidate ports.ReviewCandidateV2, intent ports.EffectIntentV2, now time.Time) (reviewCurrentFactsV2, error) {
	policy, err := g.Policies.InspectReviewPolicy(ctx, candidate.Policy.Ref)
	if err != nil {
		return reviewCurrentFactsV2{}, err
	}
	if err := policy.ValidateCurrent(candidate.Policy, candidate, now.UnixNano()); err != nil {
		return reviewCurrentFactsV2{}, err
	}
	actor, err := g.Authority.InspectDispatchAuthority(ctx, candidate.ActorAuthority.Ref)
	if err != nil || actor.ValidateCurrent(candidate.ActorAuthority, candidate.Scope, candidate.ActionScopeDigest, now) != nil {
		return reviewCurrentFactsV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "actor authority is not current")
	}
	reviewer, err := g.Authority.InspectDispatchAuthority(ctx, candidate.ReviewerAuthority.Ref)
	if err != nil || reviewer.ValidateCurrent(candidate.ReviewerAuthority, candidate.Scope, candidate.ActionScopeDigest, now) != nil {
		return reviewCurrentFactsV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "reviewer authority is not current")
	}
	subjectSet, err := g.Bindings.InspectBindingSet(ctx, candidate.SubjectProvider.BindingSetID)
	if err != nil {
		return reviewCurrentFactsV2{}, err
	}
	subjectExpiry, err := validateProviderBindingForReviewV2(subjectSet, candidate.SubjectProvider, now)
	if err != nil {
		return reviewCurrentFactsV2{}, err
	}
	capabilityDigest, err := subjectSet.CapabilityGrantDigestV2()
	if err != nil {
		return reviewCurrentFactsV2{}, err
	}
	currentScope, err := g.CurrentScopes.InspectCurrentExecutionScope(ctx, candidate.CurrentScope.Ref)
	if err != nil || currentScope.ValidateCurrent(candidate.CurrentScope, intent, capabilityDigest, now) != nil {
		return reviewCurrentFactsV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectFenceStale, "review execution scope/run watermark is not current")
	}
	ownerExpiry, err := g.inspectReviewBinding(ctx, candidate.ReviewOwnerBinding, now)
	if err != nil {
		return reviewCurrentFactsV2{}, err
	}
	reviewerExpiry, err := g.inspectReviewBinding(ctx, candidate.ReviewerBinding, now)
	if err != nil {
		return reviewCurrentFactsV2{}, err
	}
	maximum := policy.ExpiresUnixNano
	for _, expiry := range []int64{actor.ExpiresUnixNano, reviewer.ExpiresUnixNano, subjectExpiry, ownerExpiry, reviewerExpiry, currentScope.ExpiresUnixNano, candidate.ExpiresUnixNano, intent.ExpiresUnixNano} {
		if expiry < maximum {
			maximum = expiry
		}
	}
	return reviewCurrentFactsV2{policy: policy, context: reviewDecisionContextV2{PolicyCurrent: true, ActorAuthorityCurrent: true, ReviewerAuthorityCurrent: true, ReviewerBindingCurrent: true, ExecutionScopeCurrent: true}, maximumExpiry: maximum}, nil
}
func (g ReviewGovernanceGatewayV2) inspectReviewBinding(ctx context.Context, ref ports.ReviewComponentBindingRefV2, now time.Time) (int64, error) {
	set, err := g.Bindings.InspectBindingSet(ctx, ref.BindingSetID)
	if err != nil {
		return 0, err
	}
	if set.ID != ref.BindingSetID || set.State != BindingSetActive || set.Revision != ref.BindingSetRevision || !now.Before(time.Unix(0, set.ExpiresUnixNano)) {
		return 0, core.NewError(core.ErrorPreconditionFailed, core.ReasonProviderBindingStale, "review component binding set is not current")
	}
	for _, member := range set.Members {
		if member.ComponentID != ref.ComponentID {
			continue
		}
		if member.ManifestDigest != ref.ManifestDigest || member.ArtifactDigest != ref.ArtifactDigest {
			return 0, core.NewError(core.ErrorPreconditionFailed, core.ReasonProviderBindingStale, "review component artifact drifted")
		}
		for _, grant := range member.Grants {
			if grant.Capability == ref.Capability && now.Before(time.Unix(0, grant.ExpiresUnixNano)) {
				expiry := set.ExpiresUnixNano
				if grant.ExpiresUnixNano < expiry {
					expiry = grant.ExpiresUnixNano
				}
				return expiry, nil
			}
		}
		return 0, core.NewError(core.ErrorCapabilityUnavailable, core.ReasonUnknownCapability, "review component capability is not granted")
	}
	return 0, core.NewError(core.ErrorNotFound, core.ReasonComponentMissing, "review component is absent from binding set")
}
func validateProviderBindingForReviewV2(set BindingSetFactV2, ref ports.ProviderBindingRefV2, now time.Time) (int64, error) {
	if set.ID != ref.BindingSetID || set.State != BindingSetActive || set.Revision != ref.BindingSetRevision || !now.Before(time.Unix(0, set.ExpiresUnixNano)) {
		return 0, core.NewError(core.ErrorPreconditionFailed, core.ReasonProviderBindingStale, "subject provider binding is not current")
	}
	for _, member := range set.Members {
		if member.ComponentID == ref.ComponentID && member.ManifestDigest == ref.ManifestDigest && member.ArtifactDigest == ref.ArtifactDigest {
			for _, grant := range member.Grants {
				if grant.Capability == ref.Capability && now.Before(time.Unix(0, grant.ExpiresUnixNano)) {
					expiry := set.ExpiresUnixNano
					if grant.ExpiresUnixNano < expiry {
						expiry = grant.ExpiresUnixNano
					}
					return expiry, nil
				}
			}
		}
	}
	return 0, core.NewError(core.ErrorCapabilityUnavailable, core.ReasonUnknownCapability, "subject provider capability is not current")
}
func (g ReviewGovernanceGatewayV2) inspectInvocationEffect(ctx context.Context, candidate ports.ReviewCandidateV2) (core.Digest, bool) {
	expected := candidate.InvocationEffect
	if expected == nil {
		return "", false
	}
	fact, err := g.Effects.InspectEffect(ctx, expected.EffectID)
	if err != nil || fact.Intent.Revision != expected.EffectRevision || fact.Intent.Kind != expected.EffectKind || fact.Intent.Payload.ContentDigest != expected.PayloadDigest || fact.Intent.Provider != expected.Provider || fact.Intent.Relation.ReviewsCaseID != candidate.ID || fact.Intent.Relation.ReviewsCandidateRevision != candidate.Revision || fact.State != EffectSettled || fact.Settlement == nil || fact.Settlement.Disposition != SettlementConfirmedApplied {
		return "", false
	}
	digest, err := core.CanonicalJSONDigest("praxis.runtime.effect", ports.EffectContractVersionV2, "EffectSettlementFactV2", *fact.Settlement)
	return digest, err == nil
}
func (g ReviewGovernanceGatewayV2) inspectConditionAuthority(ctx context.Context, condition ports.ReviewConditionV2, proof ports.ReviewConditionProofV2, candidate ports.ReviewCandidateV2, now time.Time) (int64, error) {
	if err := MatchConditionProofsV2([]ports.ReviewConditionV2{condition}, []ports.ReviewConditionProofV2{proof}); err != nil {
		return 0, err
	}
	bindingExpiry, err := g.inspectReviewBinding(ctx, condition.SatisfactionOwner, now)
	if err != nil {
		return 0, err
	}
	authority, err := g.Authority.InspectDispatchAuthority(ctx, condition.Authority.Ref)
	if err != nil || authority.ValidateCurrent(condition.Authority, candidate.Scope, candidate.ActionScopeDigest, now) != nil {
		return 0, core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewConditionUnsatisfied, "condition satisfier authority is not current")
	}
	expiry := condition.ExpiresUnixNano
	for _, limit := range []int64{bindingExpiry, authority.ExpiresUnixNano, proof.ExpiresUnixNano} {
		if limit < expiry {
			expiry = limit
		}
	}
	return expiry, nil
}
func (g ReviewGovernanceGatewayV2) validate() error {
	if g.Facts == nil || g.Policies == nil || g.Authority == nil || g.CurrentScopes == nil || g.Bindings == nil || g.Effects == nil || g.Clock == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "review gateway requires fact, policy, authority, current-scope, binding, effect and clock ports")
	}
	return nil
}

func requireReviewableEffectV2(effect EffectFactV2) error {
	if effect.State != EffectProposed && effect.State != EffectAccepted {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidTransition, "review target Effect must remain proposed or accepted and pre-dispatch")
	}
	return nil
}

func validateCandidateAgainstPersistedIntentV2(candidate ports.ReviewCandidateV2, candidateDigest, subjectDigest core.Digest, intent ports.EffectIntentV2) error {
	conflict := func(message string) error {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewCandidateConflict, message)
	}
	if candidate.ID != intent.Review.Ref || candidate.Revision != intent.Review.Revision || candidateDigest != intent.Review.Digest {
		return conflict("review candidate identity or digest drifted from immutable Effect binding")
	}
	if candidate.SubjectDigest != subjectDigest {
		return conflict("review subject digest drifted")
	}
	if candidate.IntentID != intent.ID {
		return conflict("review Effect intent id drifted")
	}
	if candidate.IntentRevision != intent.Revision {
		return conflict("review Effect intent revision drifted")
	}
	if candidate.PayloadSchema != intent.Payload.Schema || candidate.PayloadDigest != intent.Payload.ContentDigest || candidate.PayloadRevision != intent.PayloadRevision {
		return conflict("review payload schema, digest or revision drifted")
	}
	if !ports.SameExecutionScopeV2(candidate.Scope, intent.Scope) || candidate.RunID != intent.RunID || candidate.ActionScopeDigest != intent.ActionScopeDigest {
		return conflict("review execution, run or action scope drifted")
	}
	if candidate.SubjectProvider != intent.Provider {
		return conflict("review subject provider binding drifted")
	}
	if candidate.CurrentScope != intent.CurrentScope {
		return conflict("review current-scope watermark drifted")
	}
	if candidate.RiskClass != intent.RiskClass {
		return conflict("review risk classification drifted")
	}
	if candidate.Policy.Digest != intent.Review.PolicyDigest {
		return conflict("review policy digest drifted from immutable Effect binding")
	}
	return nil
}

var _ ports.ReviewFactReaderV2 = ReviewGovernanceGatewayV2{}
