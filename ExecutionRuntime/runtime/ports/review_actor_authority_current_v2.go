package ports

import (
	"context"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"strings"
	"time"
)

const ReviewActorAuthorityCurrentContractVersionV2 = "praxis.runtime.review-actor-authority-current/v2"
const reviewActorAuthorityCurrentCanonicalDomainV2 = "praxis.runtime.review-actor-authority-current"

// ReviewActorAuthorityCurrentSubjectV2 is actor-only. It contains no Human
// Assignment and cannot be type-punned into reviewer authority.
type ReviewActorAuthorityCurrentSubjectV2 struct {
	Target            ReviewDecisionTargetRefV1 `json:"target"`
	ActorAuthority    AuthorityBindingRefV2     `json:"actor_authority"`
	ActionScopeDigest core.Digest               `json:"action_scope_digest"`
}

func (s ReviewActorAuthorityCurrentSubjectV2) Validate() error {
	if err := s.Target.Validate(); err != nil {
		return err
	}
	if err := s.ActorAuthority.Validate(); err != nil {
		return err
	}
	return s.ActionScopeDigest.Validate()
}

// Reuse the existing three-field exact projection Ref shape. V2 changes the
// actor-only subject and source semantics, not the nominal Ref.
type ReviewActorAuthorityCurrentProjectionRefV2 = ReviewDecisionAuthorityCurrentProjectionRefV1

type ReviewActorAuthorityCurrentProjectionV2 struct {
	ContractVersion  string                                     `json:"contract_version"`
	Ref              ReviewActorAuthorityCurrentProjectionRefV2 `json:"ref"`
	Subject          ReviewActorAuthorityCurrentSubjectV2       `json:"subject"`
	Fact             DispatchAuthorityFactV3                    `json:"fact"`
	State            ReviewDecisionGovernanceProjectionStateV1  `json:"state"`
	Current          bool                                       `json:"current"`
	CheckedUnixNano  int64                                      `json:"checked_unix_nano"`
	ExpiresUnixNano  int64                                      `json:"expires_unix_nano"`
	ProjectionDigest core.Digest                                `json:"projection_digest"`
}

func (p ReviewActorAuthorityCurrentProjectionV2) Clone() ReviewActorAuthorityCurrentProjectionV2 {
	p.Fact = p.Fact.Clone()
	return p
}
func (p ReviewActorAuthorityCurrentProjectionV2) Validate() error {
	if p.ContractVersion != ReviewActorAuthorityCurrentContractVersionV2 || p.Ref.Validate() != nil || p.Subject.Validate() != nil || validateReviewDecisionGovernanceStateV1(p.State) != nil || p.CheckedUnixNano <= 0 || p.CheckedUnixNano < p.Fact.CheckedUnixNano || p.CheckedUnixNano >= p.ExpiresUnixNano || p.ExpiresUnixNano > p.Fact.ExpiresUnixNano || p.Ref.Digest != p.ProjectionDigest {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Review actor authority V2 projection is incomplete")
	}
	if err := p.Fact.Validate(); err != nil {
		return err
	}
	s := p.Subject
	if p.Fact.Ref != s.ActorAuthority || p.Fact.Scope.Identity.TenantID != s.Target.TenantID || p.Fact.RunID != s.Target.RunID || p.Fact.Scope.AuthorityEpoch != s.ActorAuthority.Epoch || p.Fact.ActionScopeDigest != s.ActionScopeDigest {
		return core.NewError(core.ErrorConflict, core.ReasonStaleAuthorityEpoch, "Review actor authority V2 source drifted from exact Target applicability")
	}
	if (p.State == ReviewDecisionGovernanceProjectionActiveV1) != p.Current || p.Current != (p.Fact.State == AuthorityFactActive) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonStaleAuthorityEpoch, "Review actor authority V2 state/current truth table drifted")
	}
	id, err := DeriveReviewActorAuthorityCurrentProjectionIDV2(s)
	if err != nil || id != p.Ref.ID {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Review actor authority V2 stable projection ID drifted")
	}
	digest, err := DigestReviewActorAuthorityCurrentProjectionV2(p)
	if err != nil || digest != p.ProjectionDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Review actor authority V2 projection digest drifted")
	}
	return nil
}
func (p ReviewActorAuthorityCurrentProjectionV2) ValidateCurrent(expected ReviewActorAuthorityCurrentProjectionRefV2, subject ReviewActorAuthorityCurrentSubjectV2, now time.Time) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if expected != p.Ref || subject != p.Subject {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Review actor authority V2 current ref or subject drifted")
	}
	return validateReviewDecisionProjectionCurrentTimeV1(p.State, p.Current, p.CheckedUnixNano, p.ExpiresUnixNano, now, core.ReasonStaleAuthorityEpoch)
}

type ReviewActorAuthorityProjectionIdentityInputV2 struct {
	Target            ReviewDecisionTargetRefV1 `json:"target"`
	ActorAuthorityRef string                    `json:"actor_authority_ref"`
	ActionScopeDigest core.Digest               `json:"action_scope_digest"`
}

func (s ReviewActorAuthorityCurrentSubjectV2) IdentityInputV2() ReviewActorAuthorityProjectionIdentityInputV2 {
	return ReviewActorAuthorityProjectionIdentityInputV2{Target: s.Target, ActorAuthorityRef: s.ActorAuthority.Ref, ActionScopeDigest: s.ActionScopeDigest}
}
func DeriveReviewActorAuthorityCurrentProjectionIDV2(subject ReviewActorAuthorityCurrentSubjectV2) (string, error) {
	if err := subject.Validate(); err != nil {
		return "", err
	}
	digest, err := core.CanonicalJSONDigest(reviewActorAuthorityCurrentCanonicalDomainV2, ReviewActorAuthorityCurrentContractVersionV2, "ReviewActorAuthorityProjectionIdentityInputV2", subject.IdentityInputV2())
	if err != nil {
		return "", err
	}
	return "review-actor-authority-current-" + strings.TrimPrefix(string(digest), "sha256:"), nil
}
func DigestReviewActorAuthorityCurrentProjectionV2(p ReviewActorAuthorityCurrentProjectionV2) (core.Digest, error) {
	p = p.Clone()
	p.Ref.Digest = ""
	p.ProjectionDigest = ""
	return core.CanonicalJSONDigest(reviewActorAuthorityCurrentCanonicalDomainV2, ReviewActorAuthorityCurrentContractVersionV2, "ReviewActorAuthorityCurrentProjectionV2", p)
}
func SealReviewActorAuthorityCurrentProjectionV2(p ReviewActorAuthorityCurrentProjectionV2) (ReviewActorAuthorityCurrentProjectionV2, error) {
	p = p.Clone()
	p.ContractVersion = ReviewActorAuthorityCurrentContractVersionV2
	id, err := DeriveReviewActorAuthorityCurrentProjectionIDV2(p.Subject)
	if err != nil {
		return ReviewActorAuthorityCurrentProjectionV2{}, err
	}
	if p.Ref.ID == "" {
		p.Ref.ID = id
	} else if p.Ref.ID != id {
		return ReviewActorAuthorityCurrentProjectionV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Review actor authority V2 stable projection ID drifted")
	}
	p.Ref.Digest = ""
	p.ProjectionDigest = ""
	digest, err := DigestReviewActorAuthorityCurrentProjectionV2(p)
	if err != nil {
		return ReviewActorAuthorityCurrentProjectionV2{}, err
	}
	p.Ref.Digest, p.ProjectionDigest = digest, digest
	return p, p.Validate()
}
func SameReviewActorAuthorityStableIdentityV2(left, right ReviewActorAuthorityCurrentSubjectV2) bool {
	return left.IdentityInputV2() == right.IdentityInputV2()
}

type ReviewActorAuthorityCurrentResolveRequestV2 struct {
	Subject ReviewActorAuthorityCurrentSubjectV2 `json:"subject"`
}

func (r ReviewActorAuthorityCurrentResolveRequestV2) Validate() error { return r.Subject.Validate() }

type ReviewActorAuthorityCurrentPublishRequestV2 struct {
	Previous *ReviewActorAuthorityCurrentProjectionRefV2 `json:"previous,omitempty"`
	Value    ReviewActorAuthorityCurrentProjectionV2     `json:"value"`
}

func (r ReviewActorAuthorityCurrentPublishRequestV2) Validate() error {
	if err := r.Value.Validate(); err != nil {
		return err
	}
	if r.Previous == nil {
		if r.Value.Ref.Revision != 1 {
			return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "initial Review actor authority V2 revision must be one")
		}
		return nil
	}
	if err := r.Previous.Validate(); err != nil {
		return err
	}
	if r.Value.Ref.ID != r.Previous.ID || r.Value.Ref.Revision != r.Previous.Revision+1 {
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Review actor authority V2 full-ref CAS revision drifted")
	}
	return nil
}

type ReviewActorAuthorityCurrentPublishReceiptV2 struct {
	Ref     ReviewActorAuthorityCurrentProjectionRefV2 `json:"ref"`
	Created bool                                       `json:"created"`
}
type ReviewActorAuthorityCurrentReaderV2 interface {
	ResolveCurrentReviewActorAuthorityV2(context.Context, ReviewActorAuthorityCurrentResolveRequestV2) (ReviewActorAuthorityCurrentProjectionRefV2, error)
	InspectCurrentReviewActorAuthorityV2(context.Context, ReviewActorAuthorityCurrentSubjectV2, ReviewActorAuthorityCurrentProjectionRefV2) (ReviewActorAuthorityCurrentProjectionV2, error)
	InspectHistoricalReviewActorAuthorityV2(context.Context, ReviewActorAuthorityCurrentProjectionRefV2) (ReviewActorAuthorityCurrentProjectionV2, error)
}
type ReviewActorAuthorityCurrentPublisherV2 interface {
	PublishReviewActorAuthorityCurrentV2(context.Context, ReviewActorAuthorityCurrentPublishRequestV2) (ReviewActorAuthorityCurrentPublishReceiptV2, error)
}
