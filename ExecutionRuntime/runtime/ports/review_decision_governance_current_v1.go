package ports

import (
	"context"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const ReviewDecisionGovernanceCurrentContractVersionV1 = "praxis.runtime.review-decision-governance-current/v1"

const reviewDecisionGovernanceCurrentCanonicalDomainV1 = "praxis.runtime.review-decision-governance-current"

// ReviewDecisionTargetRefV1 is the nominal, transport-neutral identity of the
// exact Review target used by governance Owners. It is not a Review fact and
// grants no authority.
type ReviewDecisionTargetRefV1 struct {
	TenantID core.TenantID   `json:"tenant_id"`
	ID       string          `json:"target_id"`
	Revision core.Revision   `json:"target_revision"`
	Digest   core.Digest     `json:"target_digest"`
	RunID    core.AgentRunID `json:"run_id"`
}

func (r ReviewDecisionTargetRefV1) Validate() error {
	if strings.TrimSpace(string(r.TenantID)) == "" || strings.TrimSpace(r.ID) == "" || r.Revision == 0 || strings.TrimSpace(string(r.RunID)) == "" {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "exact Review target identity is required")
	}
	return r.Digest.Validate()
}

// ReviewDecisionAssignmentRefV1 is the exact Review assignment identity used
// for authority applicability. Runtime owns no assignment state.
type ReviewDecisionAssignmentRefV1 struct {
	TenantID   core.TenantID `json:"tenant_id"`
	ID         string        `json:"assignment_id"`
	Revision   core.Revision `json:"assignment_revision"`
	Digest     core.Digest   `json:"assignment_digest"`
	ReviewerID string        `json:"reviewer_id"`
}

func (r ReviewDecisionAssignmentRefV1) Validate() error {
	if strings.TrimSpace(string(r.TenantID)) == "" || strings.TrimSpace(r.ID) == "" || r.Revision == 0 || strings.TrimSpace(r.ReviewerID) == "" {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "tenant-qualified exact Review assignment and reviewer are required")
	}
	return r.Digest.Validate()
}

type ReviewDecisionGovernanceProjectionStateV1 string

const (
	ReviewDecisionGovernanceProjectionActiveV1     ReviewDecisionGovernanceProjectionStateV1 = "active"
	ReviewDecisionGovernanceProjectionRevokedV1    ReviewDecisionGovernanceProjectionStateV1 = "revoked"
	ReviewDecisionGovernanceProjectionExpiredV1    ReviewDecisionGovernanceProjectionStateV1 = "expired"
	ReviewDecisionGovernanceProjectionSupersededV1 ReviewDecisionGovernanceProjectionStateV1 = "superseded"
)

func validateReviewDecisionGovernanceStateV1(state ReviewDecisionGovernanceProjectionStateV1) error {
	switch state {
	case ReviewDecisionGovernanceProjectionActiveV1, ReviewDecisionGovernanceProjectionRevokedV1, ReviewDecisionGovernanceProjectionExpiredV1, ReviewDecisionGovernanceProjectionSupersededV1:
		return nil
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "Review decision governance projection state is invalid")
	}
}

type ReviewDecisionPolicyCurrentSubjectV1 struct {
	Target ReviewDecisionTargetRefV1 `json:"target"`
	Policy ReviewPolicyBindingRefV2  `json:"policy"`
}

func (s ReviewDecisionPolicyCurrentSubjectV1) Validate() error {
	if err := s.Target.Validate(); err != nil {
		return err
	}
	return s.Policy.Validate()
}

type ReviewDecisionPolicyCurrentProjectionRefV1 struct {
	ID       string        `json:"id"`
	Revision core.Revision `json:"revision"`
	Digest   core.Digest   `json:"digest"`
}

func (r ReviewDecisionPolicyCurrentProjectionRefV1) Validate() error {
	if strings.TrimSpace(r.ID) == "" || r.Revision == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Review policy projection ref is incomplete")
	}
	return r.Digest.Validate()
}

type ReviewDecisionPolicyCurrentProjectionV1 struct {
	ContractVersion  string                                     `json:"contract_version"`
	Ref              ReviewDecisionPolicyCurrentProjectionRefV1 `json:"ref"`
	Subject          ReviewDecisionPolicyCurrentSubjectV1       `json:"subject"`
	Fact             ReviewPolicyFactV2                         `json:"fact"`
	State            ReviewDecisionGovernanceProjectionStateV1  `json:"state"`
	Current          bool                                       `json:"current"`
	CheckedUnixNano  int64                                      `json:"checked_unix_nano"`
	ExpiresUnixNano  int64                                      `json:"expires_unix_nano"`
	ProjectionDigest core.Digest                                `json:"projection_digest"`
}

func (p ReviewDecisionPolicyCurrentProjectionV1) Validate() error {
	if p.ContractVersion != ReviewDecisionGovernanceCurrentContractVersionV1 || p.Ref.Validate() != nil || p.Subject.Validate() != nil || validateReviewDecisionGovernanceStateV1(p.State) != nil || p.CheckedUnixNano <= 0 || p.CheckedUnixNano >= p.ExpiresUnixNano || p.ExpiresUnixNano > p.Fact.ExpiresUnixNano || p.Ref.Digest != p.ProjectionDigest {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Review policy current projection is incomplete")
	}
	if p.Fact.Ref != p.Subject.Policy.Ref || p.Fact.Revision != p.Subject.Policy.Revision || p.Fact.Digest != p.Subject.Policy.Digest || p.Fact.SubjectDigest != p.Subject.Target.Digest || p.Fact.RunID != p.Subject.Target.RunID || p.Fact.Scope.Identity.TenantID != p.Subject.Target.TenantID {
		return core.NewError(core.ErrorConflict, core.ReasonReviewVerdictStale, "Review policy fact drifted from the exact target and binding")
	}
	factDigest, err := p.Fact.DigestV2()
	if err != nil || factDigest != p.Fact.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Review policy source fact digest drifted")
	}
	if (p.State == ReviewDecisionGovernanceProjectionActiveV1) != p.Current || p.Current != p.Fact.Active {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "Review policy projection state/current truth table drifted from its source fact")
	}
	expectedID, err := DeriveReviewDecisionPolicyCurrentProjectionIDV1(p.Subject, p.Fact.Ref)
	if err != nil || expectedID != p.Ref.ID {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Review policy projection ID drifted")
	}
	digest, err := DigestReviewDecisionPolicyCurrentProjectionV1(p)
	if err != nil || digest != p.ProjectionDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Review policy projection digest drifted")
	}
	return nil
}

func (p ReviewDecisionPolicyCurrentProjectionV1) ValidateCurrent(expected ReviewDecisionPolicyCurrentProjectionRefV1, subject ReviewDecisionPolicyCurrentSubjectV1, now time.Time) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if expected != p.Ref || subject != p.Subject {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Review policy current projection ref or subject drifted")
	}
	return validateReviewDecisionProjectionCurrentTimeV1(p.State, p.Current, p.CheckedUnixNano, p.ExpiresUnixNano, now, core.ReasonReviewVerdictStale)
}

func DeriveReviewDecisionPolicyCurrentProjectionIDV1(subject ReviewDecisionPolicyCurrentSubjectV1, sourceRef string) (string, error) {
	if subject.Validate() != nil || strings.TrimSpace(sourceRef) == "" {
		return "", core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Review policy projection identity inputs are incomplete")
	}
	digest, err := core.CanonicalJSONDigest(reviewDecisionGovernanceCurrentCanonicalDomainV1, ReviewDecisionGovernanceCurrentContractVersionV1, "ReviewDecisionPolicyCurrentProjectionIdentityV1", struct {
		TenantID  core.TenantID `json:"tenant_id"`
		TargetID  string        `json:"target_id"`
		SourceRef string        `json:"source_ref"`
	}{subject.Target.TenantID, subject.Target.ID, sourceRef})
	if err != nil {
		return "", err
	}
	return "review-policy-current-" + strings.TrimPrefix(string(digest), "sha256:"), nil
}

func DigestReviewDecisionPolicyCurrentProjectionV1(p ReviewDecisionPolicyCurrentProjectionV1) (core.Digest, error) {
	p.Ref.Digest = ""
	p.ProjectionDigest = ""
	return core.CanonicalJSONDigest(reviewDecisionGovernanceCurrentCanonicalDomainV1, ReviewDecisionGovernanceCurrentContractVersionV1, "ReviewDecisionPolicyCurrentProjectionV1", p)
}

type ReviewDecisionPolicyCurrentResolveRequestV1 struct {
	Subject ReviewDecisionPolicyCurrentSubjectV1 `json:"subject"`
}

type ReviewDecisionPolicyCurrentPublishRequestV1 struct {
	Previous *ReviewDecisionPolicyCurrentProjectionRefV1 `json:"previous,omitempty"`
	Value    ReviewDecisionPolicyCurrentProjectionV1     `json:"value"`
}

func (r ReviewDecisionPolicyCurrentPublishRequestV1) Validate() error {
	if err := r.Value.Validate(); err != nil {
		return err
	}
	if r.Previous == nil {
		return validateReviewDecisionPublishRevisionV1(true, "", 0, r.Value.Ref.ID, r.Value.Ref.Revision)
	}
	if err := r.Previous.Validate(); err != nil {
		return err
	}
	return validateReviewDecisionPublishRevisionV1(false, r.Previous.ID, r.Previous.Revision, r.Value.Ref.ID, r.Value.Ref.Revision)
}

type ReviewDecisionPolicyCurrentPublishReceiptV1 struct {
	Ref     ReviewDecisionPolicyCurrentProjectionRefV1 `json:"ref"`
	Created bool                                       `json:"created"`
}

// ReviewDecisionPolicyCurrentReaderV1 is Policy-Owner-backed and read-only.
// Resolve returns NotFound only when the exact subject has no current index.
// Current Inspect atomically verifies that index equals expected; Historical
// Inspect depends only on the full Ref. Invalid shape is InvalidArgument,
// same-ID/index/digest drift is Conflict, stale/terminal/TTL is
// PreconditionFailed, canceled/deadline/unknown outcomes are Indeterminate,
// and a known backend outage is Unavailable. Every success is a deep clone.
type ReviewDecisionPolicyCurrentReaderV1 interface {
	ResolveCurrentReviewDecisionPolicyV1(context.Context, ReviewDecisionPolicyCurrentResolveRequestV1) (ReviewDecisionPolicyCurrentProjectionRefV1, error)
	InspectCurrentReviewDecisionPolicyV1(context.Context, ReviewDecisionPolicyCurrentSubjectV1, ReviewDecisionPolicyCurrentProjectionRefV1) (ReviewDecisionPolicyCurrentProjectionV1, error)
	InspectHistoricalReviewDecisionPolicyV1(context.Context, ReviewDecisionPolicyCurrentProjectionRefV1) (ReviewDecisionPolicyCurrentProjectionV1, error)
}

// ReviewDecisionPolicyCurrentPublisherV1 is Policy-Owner-only. Publication is
// create-once and atomically advances the subject index with a full-ref CAS.
// A lost reply is recovered only through exact Inspect; changed replay is
// Conflict. The Reader closed set is InvalidArgument, NotFound, Conflict,
// PreconditionFailed, Indeterminate and Unavailable. Implementations return
// immutable deep clones and never refresh sealed Checked/Expires/Digest.
type ReviewDecisionPolicyCurrentPublisherV1 interface {
	PublishReviewDecisionPolicyCurrentV1(context.Context, ReviewDecisionPolicyCurrentPublishRequestV1) (ReviewDecisionPolicyCurrentPublishReceiptV1, error)
}

type ReviewDecisionAuthorityRoleV1 string

const (
	ReviewDecisionAuthorityActorV1    ReviewDecisionAuthorityRoleV1 = "actor"
	ReviewDecisionAuthorityReviewerV1 ReviewDecisionAuthorityRoleV1 = "reviewer"
)

type ReviewDecisionAuthorityCurrentSubjectV1 struct {
	Role              ReviewDecisionAuthorityRoleV1 `json:"role"`
	Target            ReviewDecisionTargetRefV1     `json:"target"`
	Assignment        ReviewDecisionAssignmentRefV1 `json:"assignment"`
	Authority         AuthorityBindingRefV2         `json:"authority"`
	ActionScopeDigest core.Digest                   `json:"action_scope_digest"`
}

func (s ReviewDecisionAuthorityCurrentSubjectV1) Validate() error {
	if s.Role != ReviewDecisionAuthorityActorV1 && s.Role != ReviewDecisionAuthorityReviewerV1 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "Review authority role must be actor or reviewer")
	}
	if err := s.Target.Validate(); err != nil {
		return err
	}
	if err := s.Assignment.Validate(); err != nil {
		return err
	}
	if s.Assignment.TenantID != s.Target.TenantID {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Review assignment tenant drifted from target tenant")
	}
	if err := s.Authority.Validate(); err != nil {
		return err
	}
	return s.ActionScopeDigest.Validate()
}

type ReviewDecisionAuthorityCurrentProjectionRefV1 struct {
	ID       string        `json:"id"`
	Revision core.Revision `json:"revision"`
	Digest   core.Digest   `json:"digest"`
}

func (r ReviewDecisionAuthorityCurrentProjectionRefV1) Validate() error {
	if strings.TrimSpace(r.ID) == "" || r.Revision == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Review authority projection ref is incomplete")
	}
	return r.Digest.Validate()
}

type ReviewDecisionAuthorityCurrentProjectionV1 struct {
	ContractVersion  string                                        `json:"contract_version"`
	Ref              ReviewDecisionAuthorityCurrentProjectionRefV1 `json:"ref"`
	Subject          ReviewDecisionAuthorityCurrentSubjectV1       `json:"subject"`
	Fact             DispatchAuthorityFactV2                       `json:"fact"`
	State            ReviewDecisionGovernanceProjectionStateV1     `json:"state"`
	Current          bool                                          `json:"current"`
	CheckedUnixNano  int64                                         `json:"checked_unix_nano"`
	ExpiresUnixNano  int64                                         `json:"expires_unix_nano"`
	ProjectionDigest core.Digest                                   `json:"projection_digest"`
}

func (p ReviewDecisionAuthorityCurrentProjectionV1) Validate() error {
	if p.ContractVersion != ReviewDecisionGovernanceCurrentContractVersionV1 || p.Ref.Validate() != nil || p.Subject.Validate() != nil || validateReviewDecisionGovernanceStateV1(p.State) != nil || p.CheckedUnixNano <= 0 || p.CheckedUnixNano >= p.ExpiresUnixNano || p.ExpiresUnixNano > p.Fact.ExpiresUnixNano || p.Ref.Digest != p.ProjectionDigest {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Review authority current projection is incomplete")
	}
	if p.Fact.Ref != p.Subject.Authority.Ref || p.Fact.Revision != p.Subject.Authority.Revision || p.Fact.Digest != p.Subject.Authority.Digest || p.Fact.Scope.AuthorityEpoch != p.Subject.Authority.Epoch || p.Fact.Scope.Identity.TenantID != p.Subject.Target.TenantID || p.Fact.ActionScopeDigest != p.Subject.ActionScopeDigest {
		return core.NewError(core.ErrorConflict, core.ReasonStaleAuthorityEpoch, "Review authority fact drifted from exact target or assignment applicability")
	}
	if (p.State == ReviewDecisionGovernanceProjectionActiveV1) != p.Current || p.Current != (p.Fact.State == AuthorityFactActive) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonStaleAuthorityEpoch, "Review authority projection state/current truth table drifted from its source fact")
	}
	expectedID, err := DeriveReviewDecisionAuthorityCurrentProjectionIDV1(p.Subject, p.Fact.Ref)
	if err != nil || expectedID != p.Ref.ID {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Review authority projection ID drifted")
	}
	digest, err := DigestReviewDecisionAuthorityCurrentProjectionV1(p)
	if err != nil || digest != p.ProjectionDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Review authority projection digest drifted")
	}
	return nil
}

func (p ReviewDecisionAuthorityCurrentProjectionV1) ValidateCurrent(expected ReviewDecisionAuthorityCurrentProjectionRefV1, subject ReviewDecisionAuthorityCurrentSubjectV1, now time.Time) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if expected != p.Ref || subject != p.Subject {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Review authority current projection ref or subject drifted")
	}
	return validateReviewDecisionProjectionCurrentTimeV1(p.State, p.Current, p.CheckedUnixNano, p.ExpiresUnixNano, now, core.ReasonStaleAuthorityEpoch)
}

func DeriveReviewDecisionAuthorityCurrentProjectionIDV1(subject ReviewDecisionAuthorityCurrentSubjectV1, sourceRef string) (string, error) {
	if subject.Validate() != nil || strings.TrimSpace(sourceRef) == "" {
		return "", core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Review authority projection identity inputs are incomplete")
	}
	digest, err := core.CanonicalJSONDigest(reviewDecisionGovernanceCurrentCanonicalDomainV1, ReviewDecisionGovernanceCurrentContractVersionV1, "ReviewDecisionAuthorityCurrentProjectionIdentityV1", struct {
		Role         ReviewDecisionAuthorityRoleV1 `json:"role"`
		TenantID     core.TenantID                 `json:"tenant_id"`
		TargetID     string                        `json:"target_id"`
		AssignmentID string                        `json:"assignment_id"`
		ReviewerID   string                        `json:"reviewer_id"`
		SourceRef    string                        `json:"source_ref"`
	}{subject.Role, subject.Target.TenantID, subject.Target.ID, subject.Assignment.ID, subject.Assignment.ReviewerID, sourceRef})
	if err != nil {
		return "", err
	}
	return "review-authority-current-" + strings.TrimPrefix(string(digest), "sha256:"), nil
}

func DigestReviewDecisionAuthorityCurrentProjectionV1(p ReviewDecisionAuthorityCurrentProjectionV1) (core.Digest, error) {
	p.Ref.Digest = ""
	p.ProjectionDigest = ""
	return core.CanonicalJSONDigest(reviewDecisionGovernanceCurrentCanonicalDomainV1, ReviewDecisionGovernanceCurrentContractVersionV1, "ReviewDecisionAuthorityCurrentProjectionV1", p)
}

type ReviewDecisionAuthorityCurrentResolveRequestV1 struct {
	Subject ReviewDecisionAuthorityCurrentSubjectV1 `json:"subject"`
}
type ReviewDecisionAuthorityCurrentPublishRequestV1 struct {
	Previous *ReviewDecisionAuthorityCurrentProjectionRefV1 `json:"previous,omitempty"`
	Value    ReviewDecisionAuthorityCurrentProjectionV1     `json:"value"`
}

func (r ReviewDecisionAuthorityCurrentPublishRequestV1) Validate() error {
	if err := r.Value.Validate(); err != nil {
		return err
	}
	if r.Previous == nil {
		return validateReviewDecisionPublishRevisionV1(true, "", 0, r.Value.Ref.ID, r.Value.Ref.Revision)
	}
	if err := r.Previous.Validate(); err != nil {
		return err
	}
	return validateReviewDecisionPublishRevisionV1(false, r.Previous.ID, r.Previous.Revision, r.Value.Ref.ID, r.Value.Ref.Revision)
}

type ReviewDecisionAuthorityCurrentPublishReceiptV1 struct {
	Ref     ReviewDecisionAuthorityCurrentProjectionRefV1 `json:"ref"`
	Created bool                                          `json:"created"`
}

// ReviewDecisionAuthorityCurrentReaderV1 has the same exact-index,
// historical-read, deep-clone and closed-error contract as the Policy Reader.
type ReviewDecisionAuthorityCurrentReaderV1 interface {
	ResolveCurrentReviewDecisionAuthorityV1(context.Context, ReviewDecisionAuthorityCurrentResolveRequestV1) (ReviewDecisionAuthorityCurrentProjectionRefV1, error)
	InspectCurrentReviewDecisionAuthorityV1(context.Context, ReviewDecisionAuthorityCurrentSubjectV1, ReviewDecisionAuthorityCurrentProjectionRefV1) (ReviewDecisionAuthorityCurrentProjectionV1, error)
	InspectHistoricalReviewDecisionAuthorityV1(context.Context, ReviewDecisionAuthorityCurrentProjectionRefV1) (ReviewDecisionAuthorityCurrentProjectionV1, error)
}

// ReviewDecisionAuthorityCurrentPublisherV1 is Authority-Owner-only and has
// the same atomic create-once, full-ref CAS, clone, error and lost-reply
// contract documented on ReviewDecisionPolicyCurrentPublisherV1.
type ReviewDecisionAuthorityCurrentPublisherV1 interface {
	PublishReviewDecisionAuthorityCurrentV1(context.Context, ReviewDecisionAuthorityCurrentPublishRequestV1) (ReviewDecisionAuthorityCurrentPublishReceiptV1, error)
}

type ReviewDecisionScopeCurrentSubjectV1 struct {
	TenantID          core.TenantID              `json:"tenant_id"`
	Target            ReviewDecisionTargetRefV1  `json:"target"`
	RunID             core.AgentRunID            `json:"run_id"`
	Scope             core.ExecutionScope        `json:"scope"`
	CurrentScope      ExecutionScopeBindingRefV2 `json:"current_scope"`
	ActionScopeDigest core.Digest                `json:"action_scope_digest"`
}

func (s ReviewDecisionScopeCurrentSubjectV1) Validate() error {
	if strings.TrimSpace(string(s.TenantID)) == "" || s.Target.Validate() != nil || strings.TrimSpace(string(s.RunID)) == "" || s.Scope.Validate() != nil || s.CurrentScope.Validate() != nil || s.ActionScopeDigest.Validate() != nil || s.TenantID != s.Target.TenantID || s.RunID != s.Target.RunID || s.Scope.Identity.TenantID != s.TenantID {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Review scope exact tenant, target, run, scope and action digest are required")
	}
	return nil
}

type ReviewDecisionScopeCurrentProjectionRefV1 struct {
	ID       string        `json:"id"`
	Revision core.Revision `json:"revision"`
	Digest   core.Digest   `json:"digest"`
}

func (r ReviewDecisionScopeCurrentProjectionRefV1) Validate() error {
	if strings.TrimSpace(r.ID) == "" || r.Revision == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Review scope projection ref is incomplete")
	}
	return r.Digest.Validate()
}

type ReviewDecisionScopeCurrentProjectionV1 struct {
	ContractVersion  string                                    `json:"contract_version"`
	Ref              ReviewDecisionScopeCurrentProjectionRefV1 `json:"ref"`
	Subject          ReviewDecisionScopeCurrentSubjectV1       `json:"subject"`
	Fact             ExecutionScopeCurrentFactV2               `json:"fact"`
	State            ReviewDecisionGovernanceProjectionStateV1 `json:"state"`
	Current          bool                                      `json:"current"`
	CheckedUnixNano  int64                                     `json:"checked_unix_nano"`
	ExpiresUnixNano  int64                                     `json:"expires_unix_nano"`
	ProjectionDigest core.Digest                               `json:"projection_digest"`
}

func (p ReviewDecisionScopeCurrentProjectionV1) Validate() error {
	if p.ContractVersion != ReviewDecisionGovernanceCurrentContractVersionV1 || p.Ref.Validate() != nil || p.Subject.Validate() != nil || validateReviewDecisionGovernanceStateV1(p.State) != nil || p.CheckedUnixNano <= 0 || p.CheckedUnixNano >= p.ExpiresUnixNano || p.ExpiresUnixNano > p.Fact.ExpiresUnixNano || p.Ref.Digest != p.ProjectionDigest {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Review scope current projection is incomplete")
	}
	factRef, err := p.Fact.BindingRefV2()
	if err != nil || factRef != p.Subject.CurrentScope || !SameExecutionScopeV2(p.Fact.Scope, p.Subject.Scope) || p.Fact.Scope.Identity.TenantID != p.Subject.TenantID || p.Fact.ActiveRunID != p.Subject.RunID {
		return core.NewError(core.ErrorConflict, core.ReasonEffectFenceStale, "Review scope fact drifted from exact tenant, target, run or scope")
	}
	if (p.State == ReviewDecisionGovernanceProjectionActiveV1) != p.Current || p.Current != (p.Fact.State == ExecutionScopeFactActive) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectFenceStale, "Review scope projection state/current truth table drifted from its source fact")
	}
	expectedID, err := DeriveReviewDecisionScopeCurrentProjectionIDV1(p.Subject, p.Fact.Ref)
	if err != nil || expectedID != p.Ref.ID {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Review scope projection ID drifted")
	}
	digest, err := DigestReviewDecisionScopeCurrentProjectionV1(p)
	if err != nil || digest != p.ProjectionDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Review scope projection digest drifted")
	}
	return nil
}

func (p ReviewDecisionScopeCurrentProjectionV1) ValidateCurrent(expected ReviewDecisionScopeCurrentProjectionRefV1, subject ReviewDecisionScopeCurrentSubjectV1, now time.Time) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if expected != p.Ref || subject != p.Subject {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Review scope current projection ref or subject drifted")
	}
	return validateReviewDecisionProjectionCurrentTimeV1(p.State, p.Current, p.CheckedUnixNano, p.ExpiresUnixNano, now, core.ReasonEffectFenceStale)
}

func DeriveReviewDecisionScopeCurrentProjectionIDV1(subject ReviewDecisionScopeCurrentSubjectV1, sourceRef string) (string, error) {
	if subject.Validate() != nil || strings.TrimSpace(sourceRef) == "" {
		return "", core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Review scope projection identity inputs are incomplete")
	}
	digest, err := core.CanonicalJSONDigest(reviewDecisionGovernanceCurrentCanonicalDomainV1, ReviewDecisionGovernanceCurrentContractVersionV1, "ReviewDecisionScopeCurrentProjectionIdentityV1", struct {
		TenantID  core.TenantID   `json:"tenant_id"`
		TargetID  string          `json:"target_id"`
		RunID     core.AgentRunID `json:"run_id"`
		SourceRef string          `json:"source_ref"`
	}{subject.TenantID, subject.Target.ID, subject.RunID, sourceRef})
	if err != nil {
		return "", err
	}
	return "review-scope-current-" + strings.TrimPrefix(string(digest), "sha256:"), nil
}

func DigestReviewDecisionScopeCurrentProjectionV1(p ReviewDecisionScopeCurrentProjectionV1) (core.Digest, error) {
	p.Ref.Digest = ""
	p.ProjectionDigest = ""
	return core.CanonicalJSONDigest(reviewDecisionGovernanceCurrentCanonicalDomainV1, ReviewDecisionGovernanceCurrentContractVersionV1, "ReviewDecisionScopeCurrentProjectionV1", p)
}

type ReviewDecisionScopeCurrentResolveRequestV1 struct {
	Subject ReviewDecisionScopeCurrentSubjectV1 `json:"subject"`
}
type ReviewDecisionScopeCurrentPublishRequestV1 struct {
	Previous *ReviewDecisionScopeCurrentProjectionRefV1 `json:"previous,omitempty"`
	Value    ReviewDecisionScopeCurrentProjectionV1     `json:"value"`
}

func (r ReviewDecisionScopeCurrentPublishRequestV1) Validate() error {
	if err := r.Value.Validate(); err != nil {
		return err
	}
	if r.Previous == nil {
		return validateReviewDecisionPublishRevisionV1(true, "", 0, r.Value.Ref.ID, r.Value.Ref.Revision)
	}
	if err := r.Previous.Validate(); err != nil {
		return err
	}
	return validateReviewDecisionPublishRevisionV1(false, r.Previous.ID, r.Previous.Revision, r.Value.Ref.ID, r.Value.Ref.Revision)
}

type ReviewDecisionScopeCurrentPublishReceiptV1 struct {
	Ref     ReviewDecisionScopeCurrentProjectionRefV1 `json:"ref"`
	Created bool                                      `json:"created"`
}

// ReviewDecisionScopeCurrentReaderV1 has the same exact-index,
// historical-read, deep-clone and closed-error contract as the Policy Reader.
type ReviewDecisionScopeCurrentReaderV1 interface {
	ResolveCurrentReviewDecisionScopeV1(context.Context, ReviewDecisionScopeCurrentResolveRequestV1) (ReviewDecisionScopeCurrentProjectionRefV1, error)
	InspectCurrentReviewDecisionScopeV1(context.Context, ReviewDecisionScopeCurrentSubjectV1, ReviewDecisionScopeCurrentProjectionRefV1) (ReviewDecisionScopeCurrentProjectionV1, error)
	InspectHistoricalReviewDecisionScopeV1(context.Context, ReviewDecisionScopeCurrentProjectionRefV1) (ReviewDecisionScopeCurrentProjectionV1, error)
}

// ReviewDecisionScopeCurrentPublisherV1 is Scope-Owner-only and has the same
// atomic create-once, full-ref CAS, clone, error and lost-reply contract as the
// Policy publisher. No projection grants Evidence, Authority or execution.
type ReviewDecisionScopeCurrentPublisherV1 interface {
	PublishReviewDecisionScopeCurrentV1(context.Context, ReviewDecisionScopeCurrentPublishRequestV1) (ReviewDecisionScopeCurrentPublishReceiptV1, error)
}

func validateReviewDecisionProjectionCurrentTimeV1(state ReviewDecisionGovernanceProjectionStateV1, current bool, checked, expires int64, now time.Time, reason core.ReasonCode) error {
	if now.IsZero() {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Review decision current validation requires time")
	}
	if now.UnixNano() < checked {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Review decision governance clock regressed")
	}
	if state != ReviewDecisionGovernanceProjectionActiveV1 || !current || !now.Before(time.Unix(0, expires)) {
		return core.NewError(core.ErrorPreconditionFailed, reason, "Review decision governance projection is terminal, stale or expired")
	}
	return nil
}

func validateReviewDecisionPublishRevisionV1(initial bool, previousID string, previousRevision core.Revision, valueID string, valueRevision core.Revision) error {
	if initial {
		if valueRevision != 1 {
			return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "initial Review decision governance projection revision must be one")
		}
		return nil
	}
	if previousID != valueID {
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Review decision governance projection stable ID drifted")
	}
	if valueRevision != previousRevision+1 {
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Review decision governance projection revision must advance exactly once")
	}
	return nil
}
