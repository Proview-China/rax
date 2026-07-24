package ports

import (
	"context"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const ReviewDecisionPolicyCurrentContractVersionV2 = "praxis.runtime.review-decision-policy-current/v2"

const reviewDecisionPolicyCurrentCanonicalDomainV2 = "praxis.runtime.review-decision-policy-current"

// ReviewDecisionPolicyApplicabilitySubjectV2 is the non-circular, nominal
// applicability coordinate owned by Runtime Policy. It deliberately omits the
// sealed Review Target digest: that digest includes the policy binding and
// therefore cannot be an input to the policy projection that proves it.
type ReviewDecisionPolicyApplicabilitySubjectV2 struct {
	TenantID            core.TenantID              `json:"tenant_id"`
	TargetID            string                     `json:"target_id"`
	TargetRevision      core.Revision              `json:"target_revision"`
	IntentID            core.EffectIntentID        `json:"intent_id"`
	IntentRevision      core.Revision              `json:"intent_revision"`
	IntentSubjectDigest core.Digest                `json:"intent_subject_digest"`
	PayloadRevision     core.Revision              `json:"payload_revision"`
	PayloadDigest       core.Digest                `json:"payload_digest"`
	RunID               core.AgentRunID            `json:"run_id"`
	Scope               core.ExecutionScope        `json:"scope"`
	CurrentScope        ExecutionScopeBindingRefV2 `json:"current_scope"`
	ActionScopeDigest   core.Digest                `json:"action_scope_digest"`
	ActorAuthority      AuthorityBindingRefV2      `json:"actor_authority"`
	Policy              ReviewPolicyBindingRefV2   `json:"policy"`
}

func (s ReviewDecisionPolicyApplicabilitySubjectV2) Validate() error {
	if strings.TrimSpace(string(s.TenantID)) == "" || strings.TrimSpace(s.TargetID) == "" || s.TargetRevision == 0 || strings.TrimSpace(string(s.IntentID)) == "" || s.IntentRevision == 0 || s.PayloadRevision == 0 || strings.TrimSpace(string(s.RunID)) == "" {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Review decision policy applicability identity is incomplete")
	}
	for _, digest := range []core.Digest{s.IntentSubjectDigest, s.PayloadDigest, s.ActionScopeDigest} {
		if err := digest.Validate(); err != nil {
			return err
		}
	}
	if err := s.Scope.Validate(); err != nil {
		return err
	}
	if s.Scope.Identity.TenantID != s.TenantID {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Review decision policy tenant and execution scope drifted")
	}
	if err := s.CurrentScope.Validate(); err != nil {
		return err
	}
	if err := s.ActorAuthority.Validate(); err != nil {
		return err
	}
	if s.Scope.AuthorityEpoch != s.ActorAuthority.Epoch {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Review decision policy authority epoch drifted from execution scope")
	}
	return s.Policy.Validate()
}

// StableIdentityV2 excludes mutable source digests/revisions while retaining
// the exact target and intent revision boundary. Policy source revisions advance
// one append-only projection history under the same stable ID.
type ReviewDecisionPolicyProjectionIdentityInputV2 struct {
	TenantID            core.TenantID              `json:"tenant_id"`
	TargetID            string                     `json:"target_id"`
	TargetRevision      core.Revision              `json:"target_revision"`
	IntentID            core.EffectIntentID        `json:"intent_id"`
	IntentRevision      core.Revision              `json:"intent_revision"`
	IntentSubjectDigest core.Digest                `json:"intent_subject_digest"`
	PayloadRevision     core.Revision              `json:"payload_revision"`
	PayloadDigest       core.Digest                `json:"payload_digest"`
	RunID               core.AgentRunID            `json:"run_id"`
	Scope               core.ExecutionScope        `json:"scope"`
	CurrentScope        ExecutionScopeBindingRefV2 `json:"current_scope"`
	ActionScopeDigest   core.Digest                `json:"action_scope_digest"`
	ActorAuthority      AuthorityBindingRefV2      `json:"actor_authority"`
	PolicyRef           string                     `json:"policy_ref"`
}

func (s ReviewDecisionPolicyApplicabilitySubjectV2) IdentityInputV2() ReviewDecisionPolicyProjectionIdentityInputV2 {
	return ReviewDecisionPolicyProjectionIdentityInputV2{TenantID: s.TenantID, TargetID: s.TargetID, TargetRevision: s.TargetRevision, IntentID: s.IntentID, IntentRevision: s.IntentRevision, IntentSubjectDigest: s.IntentSubjectDigest, PayloadRevision: s.PayloadRevision, PayloadDigest: s.PayloadDigest, RunID: s.RunID, Scope: s.Scope, CurrentScope: s.CurrentScope, ActionScopeDigest: s.ActionScopeDigest, ActorAuthority: s.ActorAuthority, PolicyRef: s.Policy.Ref}
}

// SameReviewDecisionPolicyApplicabilitySubjectV2 compares the exact nominal
// subject semantically. ExecutionScope may contain a lease pointer, so plain
// struct equality would incorrectly compare allocation identity.
func SameReviewDecisionPolicyApplicabilitySubjectV2(left, right ReviewDecisionPolicyApplicabilitySubjectV2) bool {
	return left.TenantID == right.TenantID && left.TargetID == right.TargetID && left.TargetRevision == right.TargetRevision && left.IntentID == right.IntentID && left.IntentRevision == right.IntentRevision && left.IntentSubjectDigest == right.IntentSubjectDigest && left.PayloadRevision == right.PayloadRevision && left.PayloadDigest == right.PayloadDigest && left.RunID == right.RunID && SameExecutionScopeV2(left.Scope, right.Scope) && left.CurrentScope == right.CurrentScope && left.ActionScopeDigest == right.ActionScopeDigest && left.ActorAuthority == right.ActorAuthority && left.Policy == right.Policy
}

// SameReviewDecisionPolicyProjectionIdentityV2 checks the immutable identity
// without comparing pointer allocation identity and without treating a Policy
// fact revision/digest advance as a new applicability subject.
func SameReviewDecisionPolicyProjectionIdentityV2(left, right ReviewDecisionPolicyApplicabilitySubjectV2) bool {
	return left.TenantID == right.TenantID && left.TargetID == right.TargetID && left.TargetRevision == right.TargetRevision && left.IntentID == right.IntentID && left.IntentRevision == right.IntentRevision && left.IntentSubjectDigest == right.IntentSubjectDigest && left.PayloadRevision == right.PayloadRevision && left.PayloadDigest == right.PayloadDigest && left.RunID == right.RunID && SameExecutionScopeV2(left.Scope, right.Scope) && left.CurrentScope == right.CurrentScope && left.ActionScopeDigest == right.ActionScopeDigest && left.ActorAuthority == right.ActorAuthority && left.Policy.Ref == right.Policy.Ref
}

// ReviewDecisionPolicyCurrentProjectionRefV2 deliberately aliases the public
// V1 exact three-field nominal ref. BypassDecisionV1 already persists this
// shape, so V2 adds semantics without introducing a type-pun or migration.
type ReviewDecisionPolicyCurrentProjectionRefV2 = ReviewDecisionPolicyCurrentProjectionRefV1

type ReviewDecisionPolicyCurrentProjectionV2 struct {
	ContractVersion  string                                     `json:"contract_version"`
	Ref              ReviewDecisionPolicyCurrentProjectionRefV2 `json:"ref"`
	Subject          ReviewDecisionPolicyApplicabilitySubjectV2 `json:"subject"`
	Fact             ReviewPolicyFactV2                         `json:"fact"`
	State            ReviewDecisionGovernanceProjectionStateV1  `json:"state"`
	Current          bool                                       `json:"current"`
	CheckedUnixNano  int64                                      `json:"checked_unix_nano"`
	ExpiresUnixNano  int64                                      `json:"expires_unix_nano"`
	ProjectionDigest core.Digest                                `json:"projection_digest"`
}

func (p ReviewDecisionPolicyCurrentProjectionV2) Clone() ReviewDecisionPolicyCurrentProjectionV2 {
	if p.Subject.Scope.SandboxLease != nil {
		lease := *p.Subject.Scope.SandboxLease
		p.Subject.Scope.SandboxLease = &lease
	}
	if p.Fact.Scope.SandboxLease != nil {
		lease := *p.Fact.Scope.SandboxLease
		p.Fact.Scope.SandboxLease = &lease
	}
	return p
}

func (p ReviewDecisionPolicyCurrentProjectionV2) Validate() error {
	if p.ContractVersion != ReviewDecisionPolicyCurrentContractVersionV2 || p.Ref.Validate() != nil || p.Subject.Validate() != nil || validateReviewDecisionGovernanceStateV1(p.State) != nil || p.CheckedUnixNano <= 0 || p.CheckedUnixNano >= p.ExpiresUnixNano || p.ExpiresUnixNano > p.Fact.ExpiresUnixNano || p.Ref.Digest != p.ProjectionDigest {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Review decision policy V2 projection is incomplete")
	}
	s := p.Subject
	if p.Fact.Ref != s.Policy.Ref || p.Fact.Revision != s.Policy.Revision || p.Fact.Digest != s.Policy.Digest || p.Fact.SubjectDigest != s.IntentSubjectDigest || p.Fact.RunID != s.RunID || !SameExecutionScopeV2(p.Fact.Scope, s.Scope) || p.Fact.CurrentScope != s.CurrentScope || p.Fact.ActorAuthorityRef != s.ActorAuthority.Ref {
		return core.NewError(core.ErrorConflict, core.ReasonReviewVerdictStale, "Review decision policy source fact drifted from applicability subject")
	}
	factDigest, err := p.Fact.DigestV2()
	if err != nil || factDigest != p.Fact.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Review decision policy source fact digest drifted")
	}
	if (p.State == ReviewDecisionGovernanceProjectionActiveV1) != p.Current || p.Current != p.Fact.Active {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "Review decision policy state/current truth table drifted")
	}
	wantID, err := DeriveReviewDecisionPolicyCurrentProjectionIDV2(s)
	if err != nil || p.Ref.ID != wantID {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Review decision policy stable projection ID drifted")
	}
	wantDigest, err := DigestReviewDecisionPolicyCurrentProjectionV2(p)
	if err != nil || wantDigest != p.ProjectionDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Review decision policy projection digest drifted")
	}
	return nil
}

func (p ReviewDecisionPolicyCurrentProjectionV2) ValidateCurrent(expected ReviewDecisionPolicyCurrentProjectionRefV2, subject ReviewDecisionPolicyApplicabilitySubjectV2, now time.Time) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if expected != p.Ref || !SameReviewDecisionPolicyApplicabilitySubjectV2(subject, p.Subject) {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Review decision policy current ref or subject drifted")
	}
	return validateReviewDecisionProjectionCurrentTimeV1(p.State, p.Current, p.CheckedUnixNano, p.ExpiresUnixNano, now, core.ReasonReviewVerdictStale)
}

func DeriveReviewDecisionPolicyCurrentProjectionIDV2(subject ReviewDecisionPolicyApplicabilitySubjectV2) (string, error) {
	if err := subject.Validate(); err != nil {
		return "", err
	}
	digest, err := core.CanonicalJSONDigest(reviewDecisionPolicyCurrentCanonicalDomainV2, ReviewDecisionPolicyCurrentContractVersionV2, "ReviewDecisionPolicyProjectionIdentityInputV2", subject.IdentityInputV2())
	if err != nil {
		return "", err
	}
	return "review-policy-current-v2-" + strings.TrimPrefix(string(digest), "sha256:"), nil
}

func DigestReviewDecisionPolicyCurrentProjectionV2(p ReviewDecisionPolicyCurrentProjectionV2) (core.Digest, error) {
	p.Ref.Digest = ""
	p.ProjectionDigest = ""
	return core.CanonicalJSONDigest(reviewDecisionPolicyCurrentCanonicalDomainV2, ReviewDecisionPolicyCurrentContractVersionV2, "ReviewDecisionPolicyCurrentProjectionV2", p)
}

func SealReviewDecisionPolicyCurrentProjectionV2(p ReviewDecisionPolicyCurrentProjectionV2) (ReviewDecisionPolicyCurrentProjectionV2, error) {
	p.ContractVersion = ReviewDecisionPolicyCurrentContractVersionV2
	id, err := DeriveReviewDecisionPolicyCurrentProjectionIDV2(p.Subject)
	if err != nil {
		return ReviewDecisionPolicyCurrentProjectionV2{}, err
	}
	if p.Ref.ID == "" {
		p.Ref.ID = id
	} else if p.Ref.ID != id {
		return ReviewDecisionPolicyCurrentProjectionV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Review decision policy stable projection ID drifted")
	}
	p.Ref.Digest = ""
	p.ProjectionDigest = ""
	digest, err := DigestReviewDecisionPolicyCurrentProjectionV2(p)
	if err != nil {
		return ReviewDecisionPolicyCurrentProjectionV2{}, err
	}
	p.Ref.Digest, p.ProjectionDigest = digest, digest
	return p, p.Validate()
}

type ReviewDecisionPolicyCurrentResolveRequestV2 struct {
	Subject ReviewDecisionPolicyApplicabilitySubjectV2 `json:"subject"`
}

func (r ReviewDecisionPolicyCurrentResolveRequestV2) Validate() error { return r.Subject.Validate() }

type ReviewDecisionPolicyCurrentPublishRequestV2 struct {
	Previous *ReviewDecisionPolicyCurrentProjectionRefV2 `json:"previous,omitempty"`
	Value    ReviewDecisionPolicyCurrentProjectionV2     `json:"value"`
}

func (r ReviewDecisionPolicyCurrentPublishRequestV2) Validate() error {
	if err := r.Value.Validate(); err != nil {
		return err
	}
	if r.Previous == nil {
		if r.Value.Ref.Revision != 1 {
			return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "initial Review decision policy V2 revision must be one")
		}
		return nil
	}
	if err := r.Previous.Validate(); err != nil {
		return err
	}
	if r.Value.Ref.ID != r.Previous.ID || r.Value.Ref.Revision != r.Previous.Revision+1 {
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Review decision policy V2 full-ref CAS revision drifted")
	}
	return nil
}

type ReviewDecisionPolicyCurrentPublishReceiptV2 struct {
	Ref     ReviewDecisionPolicyCurrentProjectionRefV2 `json:"ref"`
	Created bool                                       `json:"created"`
}

type ReviewDecisionPolicyCurrentReaderV2 interface {
	ResolveCurrentReviewDecisionPolicyV2(context.Context, ReviewDecisionPolicyCurrentResolveRequestV2) (ReviewDecisionPolicyCurrentProjectionRefV2, error)
	InspectCurrentReviewDecisionPolicyV2(context.Context, ReviewDecisionPolicyApplicabilitySubjectV2, ReviewDecisionPolicyCurrentProjectionRefV2) (ReviewDecisionPolicyCurrentProjectionV2, error)
	InspectHistoricalReviewDecisionPolicyV2(context.Context, ReviewDecisionPolicyCurrentProjectionRefV2) (ReviewDecisionPolicyCurrentProjectionV2, error)
}
type ReviewDecisionPolicyCurrentPublisherV2 interface {
	PublishReviewDecisionPolicyCurrentV2(context.Context, ReviewDecisionPolicyCurrentPublishRequestV2) (ReviewDecisionPolicyCurrentPublishReceiptV2, error)
}
