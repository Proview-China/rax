package ports

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const OperationReviewAuthorizationContractVersionV4 = "4.0.0"

type OperationReviewAuthorizationBasisV4 string

const (
	OperationReviewBasisAcceptedV4             OperationReviewAuthorizationBasisV4 = "accepted"
	OperationReviewBasisConditionalSatisfiedV4 OperationReviewAuthorizationBasisV4 = "conditional_satisfied"
	OperationReviewBasisNotRequiredV4          OperationReviewAuthorizationBasisV4 = "operation_not_required"
)

type OperationReviewTargetRefV4 struct {
	Ref      string        `json:"ref"`
	Revision core.Revision `json:"revision"`
	Digest   core.Digest   `json:"digest"`
}

func (r OperationReviewTargetRefV4) Validate() error {
	if err := validateOperationReviewTargetV4(r.Ref); err != nil || r.Revision == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonReviewCandidateConflict, "Review target identity and revision are required")
	}
	return r.Digest.Validate()
}

type OperationReviewConditionSatisfactionV4 struct {
	Fact             OperationGovernanceFactRefV3 `json:"fact"`
	ConditionsDigest core.Digest                  `json:"conditions_digest"`
	Evidence         []EvidenceRecordRefV2        `json:"evidence"`
	EvidenceDigest   core.Digest                  `json:"evidence_digest"`
}

func (s OperationReviewConditionSatisfactionV4) Validate(now time.Time) error {
	if err := s.Fact.Validate(now); err != nil {
		return err
	}
	if err := s.ConditionsDigest.Validate(); err != nil {
		return err
	}
	if len(s.Evidence) == 0 || len(s.Evidence) > MaxReviewEvidenceV2 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewConditionUnsatisfied, "conditional authorization requires bounded satisfaction evidence")
	}
	digest, err := DigestOperationReviewEvidenceV4(s.Evidence)
	if err != nil || digest != s.EvidenceDigest {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "condition satisfaction evidence drifted")
	}
	return nil
}

// OperationReviewCurrentProjectionV4 is a read-only projection over Review
// Owner facts. It is not an Authorization Fact and never grants dispatch.
type OperationReviewCurrentProjectionV4 struct {
	ContractVersion   string                                  `json:"contract_version"`
	Operation         OperationSubjectV3                      `json:"operation"`
	IntentID          core.EffectIntentID                     `json:"intent_id"`
	IntentRevision    core.Revision                           `json:"intent_revision"`
	IntentDigest      core.Digest                             `json:"intent_digest"`
	PayloadSchema     SchemaRefV2                             `json:"payload_schema"`
	PayloadDigest     core.Digest                             `json:"payload_digest"`
	PayloadRevision   core.Revision                           `json:"payload_revision"`
	Target            OperationReviewTargetRefV4              `json:"review_target"`
	Case              OperationGovernanceFactRefV3            `json:"case"`
	Verdict           OperationGovernanceFactRefV3            `json:"verdict"`
	Basis             OperationReviewAuthorizationBasisV4     `json:"basis"`
	Satisfaction      *OperationReviewConditionSatisfactionV4 `json:"satisfaction,omitempty"`
	Policy            OperationGovernanceFactRefV3            `json:"policy"`
	ReviewerAuthority OperationGovernanceFactRefV3            `json:"reviewer_authority"`
	Scope             OperationGovernanceFactRefV3            `json:"scope"`
	Binding           OperationGovernanceFactRefV3            `json:"binding"`
	DecisionEvidence  []EvidenceRecordRefV2                   `json:"decision_evidence"`
	EvidenceDigest    core.Digest                             `json:"evidence_digest"`
	Current           bool                                    `json:"current"`
	CurrentnessDigest core.Digest                             `json:"currentness_digest"`
	ProjectionDigest  core.Digest                             `json:"projection_digest"`
	ExpiresUnixNano   int64                                   `json:"expires_unix_nano"`
}

func (p OperationReviewCurrentProjectionV4) Validate(now time.Time) error {
	if p.ContractVersion != OperationReviewAuthorizationContractVersionV4 || p.Operation.Validate() != nil || p.IntentID == "" || p.IntentRevision == 0 || p.PayloadRevision == 0 || p.ExpiresUnixNano <= 0 || now.IsZero() {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonReviewVerdictMissing, "Operation Review projection identity and TTL are incomplete")
	}
	for _, digest := range []core.Digest{p.IntentDigest, p.PayloadDigest, p.EvidenceDigest, p.CurrentnessDigest, p.ProjectionDigest} {
		if err := digest.Validate(); err != nil {
			return err
		}
	}
	if err := p.PayloadSchema.Validate(); err != nil {
		return err
	}
	if err := p.Target.Validate(); err != nil {
		return err
	}
	for _, ref := range []OperationGovernanceFactRefV3{p.Case, p.Verdict, p.Policy, p.ReviewerAuthority, p.Scope, p.Binding} {
		if err := ref.Validate(now); err != nil {
			return err
		}
	}
	if len(p.DecisionEvidence) == 0 || len(p.DecisionEvidence) > MaxReviewEvidenceV2 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceUnavailable, "Review authorization requires bounded decision evidence")
	}
	evidenceDigest, err := DigestOperationReviewEvidenceV4(p.DecisionEvidence)
	if err != nil || evidenceDigest != p.EvidenceDigest {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Review decision evidence drifted")
	}
	switch p.Basis {
	case OperationReviewBasisAcceptedV4, OperationReviewBasisNotRequiredV4:
		if p.Satisfaction != nil {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewCandidateConflict, "non-conditional authorization cannot carry condition satisfaction")
		}
	case OperationReviewBasisConditionalSatisfiedV4:
		if p.Satisfaction == nil {
			return core.NewError(core.ErrorForbidden, core.ReasonReviewConditionUnsatisfied, "conditional verdict is not satisfied")
		}
		if err := p.Satisfaction.Validate(now); err != nil {
			return err
		}
	default:
		return core.NewError(core.ErrorForbidden, core.ReasonReviewVerdictStale, "Review basis cannot authorize an operation")
	}
	if !p.Current || !now.Before(time.Unix(0, p.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "Review projection is inactive or expired")
	}
	digest, err := p.DigestV4()
	if err != nil || digest != p.ProjectionDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Review current projection digest drifted")
	}
	return nil
}

func (p OperationReviewCurrentProjectionV4) ValidateAgainstIntent(intent OperationEffectIntentV3, current OperationGovernanceSnapshotV3, now time.Time) error {
	if err := p.Validate(now); err != nil {
		return err
	}
	intentDigest, err := intent.DigestV3()
	if err != nil {
		return err
	}
	if !SameOperationSubjectV3(p.Operation, intent.Operation) || p.IntentID != intent.ID || p.IntentRevision != intent.Revision || p.IntentDigest != intentDigest || p.PayloadSchema != intent.Payload.Schema || p.PayloadDigest != intent.Payload.ContentDigest || p.PayloadRevision != intent.PayloadRevision || p.Target.Ref != intent.Target || p.Case.Ref != intent.Review.CaseRef || p.Target.Revision != intent.Review.CandidateRevision || p.Target.Digest != intent.Review.CandidateDigest || p.Policy.Digest != intent.Review.PolicyDigest || p.ReviewerAuthority.Digest != current.Review.ReviewerAuthority.Digest || p.ReviewerAuthority.Revision != current.Review.ReviewerAuthority.Revision || p.Scope != current.CurrentScope || p.Binding != current.Binding {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "Review target, intent, payload or governing facts drifted")
	}
	return nil
}

func (p OperationReviewCurrentProjectionV4) DigestV4() (core.Digest, error) {
	copy := p
	copy.ProjectionDigest = ""
	copy.DecisionEvidence = normalizedOperationReviewEvidenceV4(copy.DecisionEvidence)
	if copy.Satisfaction != nil {
		satisfaction := *copy.Satisfaction
		satisfaction.Evidence = normalizedOperationReviewEvidenceV4(satisfaction.Evidence)
		copy.Satisfaction = &satisfaction
	}
	return core.CanonicalJSONDigest("praxis.runtime.operation-review-authorization", OperationReviewAuthorizationContractVersionV4, "OperationReviewCurrentProjectionV4", copy)
}

func SealOperationReviewCurrentProjectionV4(p OperationReviewCurrentProjectionV4, now time.Time) (OperationReviewCurrentProjectionV4, error) {
	p.ContractVersion = OperationReviewAuthorizationContractVersionV4
	p.DecisionEvidence = normalizedOperationReviewEvidenceV4(p.DecisionEvidence)
	if p.Satisfaction != nil {
		satisfaction := *p.Satisfaction
		satisfaction.Evidence = normalizedOperationReviewEvidenceV4(satisfaction.Evidence)
		digest, err := DigestOperationReviewEvidenceV4(satisfaction.Evidence)
		if err != nil {
			return OperationReviewCurrentProjectionV4{}, err
		}
		satisfaction.EvidenceDigest = digest
		p.Satisfaction = &satisfaction
	}
	evidenceDigest, err := DigestOperationReviewEvidenceV4(p.DecisionEvidence)
	if err != nil {
		return OperationReviewCurrentProjectionV4{}, err
	}
	p.EvidenceDigest = evidenceDigest
	p.ProjectionDigest = ""
	digest, err := p.DigestV4()
	if err != nil {
		return OperationReviewCurrentProjectionV4{}, err
	}
	p.ProjectionDigest = digest
	return p, p.Validate(now)
}

type OperationReviewIntentBindingV4 struct {
	Operation          OperationSubjectV3          `json:"operation"`
	IntentID           core.EffectIntentID         `json:"intent_id"`
	IntentRevision     core.Revision               `json:"intent_revision"`
	IntentDigest       core.Digest                 `json:"intent_digest"`
	EffectFactRevision core.Revision               `json:"effect_fact_revision"`
	Target             string                      `json:"target"`
	PayloadSchema      SchemaRefV2                 `json:"payload_schema"`
	PayloadDigest      core.Digest                 `json:"payload_digest"`
	PayloadRevision    core.Revision               `json:"payload_revision"`
	Provider           ProviderBindingRefV2        `json:"provider"`
	Authority          AuthorityBindingRefV2       `json:"authority"`
	ReviewBinding      OperationReviewBindingRefV3 `json:"review_binding"`
	DispatchPolicy     OperationPolicyBindingRefV3 `json:"dispatch_policy"`
	IntentExpires      int64                       `json:"intent_expires_unix_nano"`
}

func (b OperationReviewIntentBindingV4) Validate() error {
	if b.Operation.Validate() != nil || b.IntentID == "" || b.IntentRevision == 0 || b.EffectFactRevision == 0 || strings.TrimSpace(b.Target) == "" || b.PayloadRevision == 0 || b.IntentExpires <= 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonEffectIntentMissing, "authorization Intent binding is incomplete")
	}
	if err := b.IntentDigest.Validate(); err != nil {
		return err
	}
	if err := b.PayloadSchema.Validate(); err != nil {
		return err
	}
	if err := b.PayloadDigest.Validate(); err != nil {
		return err
	}
	if err := b.Provider.Validate(); err != nil {
		return err
	}
	if err := b.Authority.Validate(); err != nil {
		return err
	}
	if err := b.ReviewBinding.Validate(); err != nil {
		return err
	}
	return b.DispatchPolicy.Validate()
}

type OperationReviewGovernanceBindingV4 struct {
	SnapshotDigest        core.Digest                  `json:"snapshot_digest"`
	ProjectionWatermark   uint64                       `json:"projection_watermark"`
	Identity              OperationGovernanceFactRefV3 `json:"identity"`
	Binding               OperationGovernanceFactRefV3 `json:"binding"`
	CurrentScope          OperationGovernanceFactRefV3 `json:"current_scope"`
	Authority             OperationGovernanceFactRefV3 `json:"authority"`
	Policy                OperationGovernanceFactRefV3 `json:"policy"`
	Budget                OperationGovernanceFactRefV3 `json:"budget"`
	CapabilityGrantDigest core.Digest                  `json:"capability_grant_digest"`
	CredentialGrantDigest core.Digest                  `json:"credential_grant_digest"`
	ExpiresUnixNano       int64                        `json:"expires_unix_nano"`
}

func (b OperationReviewGovernanceBindingV4) Validate(now time.Time) error {
	if b.ProjectionWatermark == 0 || b.ExpiresUnixNano <= 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonEffectAuthorizationMissing, "authorization governance binding is incomplete")
	}
	for _, digest := range []core.Digest{b.SnapshotDigest, b.CapabilityGrantDigest, b.CredentialGrantDigest} {
		if err := digest.Validate(); err != nil {
			return err
		}
	}
	for _, ref := range []OperationGovernanceFactRefV3{b.Identity, b.Binding, b.CurrentScope, b.Authority, b.Policy, b.Budget} {
		if err := ref.Validate(now); err != nil {
			return err
		}
	}
	if now.IsZero() || !now.Before(time.Unix(0, b.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectAuthorizationMissing, "authorization governance binding expired")
	}
	return nil
}

type OperationReviewAuthorizationStateV4 string

const (
	OperationReviewAuthorizationActiveV4     OperationReviewAuthorizationStateV4 = "active"
	OperationReviewAuthorizationRevokedV4    OperationReviewAuthorizationStateV4 = "revoked"
	OperationReviewAuthorizationExpiredV4    OperationReviewAuthorizationStateV4 = "expired"
	OperationReviewAuthorizationSupersededV4 OperationReviewAuthorizationStateV4 = "superseded"
)

type OperationReviewAuthorizationFactV4 struct {
	ContractVersion      string                              `json:"contract_version"`
	ID                   string                              `json:"id"`
	Revision             core.Revision                       `json:"revision"`
	State                OperationReviewAuthorizationStateV4 `json:"state"`
	Intent               OperationReviewIntentBindingV4      `json:"intent"`
	Review               OperationReviewCurrentProjectionV4  `json:"review"`
	Governance           OperationReviewGovernanceBindingV4  `json:"governance"`
	Fence                core.ExecutionFence                 `json:"fence"`
	FenceDigest          core.Digest                         `json:"fence_digest"`
	RequestedTTLUnixNano int64                               `json:"requested_ttl_unix_nano"`
	CreatedUnixNano      int64                               `json:"created_unix_nano"`
	UpdatedUnixNano      int64                               `json:"updated_unix_nano"`
	ExpiresUnixNano      int64                               `json:"expires_unix_nano"`
	InvalidationReason   core.ReasonCode                     `json:"invalidation_reason,omitempty"`
	Digest               core.Digest                         `json:"digest"`
}

func (f OperationReviewAuthorizationFactV4) Validate() error {
	created := time.Unix(0, f.CreatedUnixNano)
	if f.ContractVersion != OperationReviewAuthorizationContractVersionV4 || validateOperationReviewIDV4(f.ID) != nil || f.Revision == 0 || f.RequestedTTLUnixNano <= 0 || f.CreatedUnixNano <= 0 || f.UpdatedUnixNano < f.CreatedUnixNano {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonReviewVerdictMissing, "Review Authorization Fact identity and time are incomplete")
	}
	if err := f.Intent.Validate(); err != nil {
		return err
	}
	if err := f.Review.Validate(created); err != nil {
		return err
	}
	if err := f.Governance.Validate(created); err != nil {
		return err
	}
	if !SameOperationSubjectV3(f.Intent.Operation, f.Review.Operation) || f.Intent.IntentID != f.Review.IntentID || f.Intent.IntentRevision != f.Review.IntentRevision || f.Intent.IntentDigest != f.Review.IntentDigest || f.Intent.PayloadSchema != f.Review.PayloadSchema || f.Intent.PayloadDigest != f.Review.PayloadDigest || f.Intent.PayloadRevision != f.Review.PayloadRevision || f.Intent.Target != f.Review.Target.Ref || f.Intent.ReviewBinding.CaseRef != f.Review.Case.Ref || f.Intent.ReviewBinding.CandidateRevision != f.Review.Target.Revision || f.Intent.ReviewBinding.CandidateDigest != f.Review.Target.Digest || f.Intent.ReviewBinding.PolicyDigest != f.Review.Policy.Digest || f.Review.Scope != f.Governance.CurrentScope || f.Review.Binding != f.Governance.Binding || f.Intent.Provider.BindingSetID != f.Governance.Binding.Ref || f.Intent.Provider.BindingSetRevision != f.Governance.Binding.Revision || f.Intent.Authority.Ref != f.Governance.Authority.Ref || f.Intent.Authority.Revision != f.Governance.Authority.Revision || f.Intent.Authority.Digest != f.Governance.Authority.Digest || f.Intent.DispatchPolicy.Ref != f.Governance.Policy.Ref || f.Intent.DispatchPolicy.Revision != f.Governance.Policy.Revision || f.Intent.DispatchPolicy.Digest != f.Governance.Policy.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "Review Authorization intent, Review and governance bindings do not form one exact fact")
	}
	if err := f.Fence.Validate(); err != nil {
		return err
	}
	fenceDigest, err := DigestOperationExecutionFenceV3(f.Fence, f.Intent.Operation)
	if err != nil || fenceDigest != f.FenceDigest || f.Fence.EffectIntentID != f.Intent.IntentID || f.Fence.EffectIntentRevision != f.Intent.IntentRevision || f.Fence.CanonicalPayloadDigest != f.Intent.PayloadDigest || f.Fence.CapabilityGrantDigest != f.Governance.CapabilityGrantDigest {
		return core.NewError(core.ErrorConflict, core.ReasonEffectFenceStale, "Review Authorization Fence drifted")
	}
	expires := minimumOperationReviewAuthorizationExpiryV4(f)
	if f.ExpiresUnixNano != expires || f.Fence.ExpiresAt.UnixNano() != expires {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "Review Authorization TTL exceeds a governing fact")
	}
	switch f.State {
	case OperationReviewAuthorizationActiveV4:
		if f.InvalidationReason != "" || f.UpdatedUnixNano >= f.ExpiresUnixNano {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "active Review Authorization is invalid or expired")
		}
	case OperationReviewAuthorizationRevokedV4, OperationReviewAuthorizationSupersededV4:
		if f.InvalidationReason == "" {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "invalidated Review Authorization requires a reason")
		}
	case OperationReviewAuthorizationExpiredV4:
		if f.InvalidationReason != core.ReasonReviewVerdictStale || f.UpdatedUnixNano < f.ExpiresUnixNano {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "expired Review Authorization has not crossed its TTL")
		}
	default:
		return core.NewError(core.ErrorForbidden, core.ReasonReviewVerdictStale, "unknown Review Authorization state")
	}
	digest, err := f.DigestV4()
	if err != nil || digest != f.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Review Authorization Fact digest drifted")
	}
	return nil
}

func (f OperationReviewAuthorizationFactV4) DigestV4() (core.Digest, error) {
	copy := f
	copy.Digest = ""
	copy.Review.DecisionEvidence = normalizedOperationReviewEvidenceV4(copy.Review.DecisionEvidence)
	return core.CanonicalJSONDigest("praxis.runtime.operation-review-authorization", OperationReviewAuthorizationContractVersionV4, "OperationReviewAuthorizationFactV4", copy)
}

func SealOperationReviewAuthorizationFactV4(f OperationReviewAuthorizationFactV4) (OperationReviewAuthorizationFactV4, error) {
	f.ContractVersion = OperationReviewAuthorizationContractVersionV4
	f.Digest = ""
	digest, err := f.DigestV4()
	if err != nil {
		return OperationReviewAuthorizationFactV4{}, err
	}
	f.Digest = digest
	return f, f.Validate()
}

type OperationReviewAuthorizationRefV4 struct {
	ID       string        `json:"id"`
	Revision core.Revision `json:"revision"`
	Digest   core.Digest   `json:"digest"`
}

func (r OperationReviewAuthorizationRefV4) Validate() error {
	if validateOperationReviewIDV4(r.ID) != nil || r.Revision == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Review Authorization ref is incomplete")
	}
	return r.Digest.Validate()
}

func (f OperationReviewAuthorizationFactV4) RefV4() OperationReviewAuthorizationRefV4 {
	return OperationReviewAuthorizationRefV4{ID: f.ID, Revision: f.Revision, Digest: f.Digest}
}

// CompatibilityProjectionV3 may be used only after the V4 Gateway returned a
// current Fact. The projection is not independently current and is not a Permit.
func (f OperationReviewAuthorizationFactV4) CompatibilityProjectionV3(now time.Time) (OperationReviewAuthorizationV3, error) {
	if err := f.Validate(); err != nil {
		return OperationReviewAuthorizationV3{}, err
	}
	if f.State != OperationReviewAuthorizationActiveV4 || now.IsZero() || !now.Before(time.Unix(0, f.ExpiresUnixNano)) {
		return OperationReviewAuthorizationV3{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "only a current active V4 Authorization may project to V3")
	}
	projection := OperationReviewAuthorizationV3{Case: f.Review.Case, CandidateDigest: f.Review.Target.Digest, CandidateRevision: f.Review.Target.Revision, Verdict: f.Review.Verdict, ReviewerAuthority: f.Review.ReviewerAuthority, PolicyDigest: f.Review.Policy.Digest, ExpiresUnixNano: f.ExpiresUnixNano}
	if f.Review.Satisfaction != nil {
		fact := f.Review.Satisfaction.Fact
		projection.Satisfaction = &fact
	}
	return projection, projection.ValidateCurrent(OperationReviewBindingRefV3{CaseRef: f.Review.Case.Ref, CandidateDigest: f.Review.Target.Digest, CandidateRevision: f.Review.Target.Revision, PolicyDigest: f.Review.Policy.Digest}, now)
}

type CreateOperationReviewAuthorizationRequestV4 struct {
	AuthorizationID        string              `json:"authorization_id"`
	Operation              OperationSubjectV3  `json:"operation"`
	EffectID               core.EffectIntentID `json:"effect_id"`
	ExpectedEffectRevision core.Revision       `json:"expected_effect_revision"`
	RequestedTTL           time.Duration       `json:"requested_ttl"`
}

func (r CreateOperationReviewAuthorizationRequestV4) Validate() error {
	if validateOperationReviewIDV4(r.AuthorizationID) != nil || r.Operation.Validate() != nil || r.EffectID == "" || r.ExpectedEffectRevision == 0 || r.RequestedTTL <= 0 || r.RequestedTTL > MaxDispatchPermitTTL {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonReviewVerdictMissing, "Review Authorization request requires exact Effect and bounded TTL")
	}
	return nil
}

type OperationReviewAuthorizationCASRequestV4 struct {
	ExpectedRevision core.Revision                      `json:"expected_revision"`
	Next             OperationReviewAuthorizationFactV4 `json:"next"`
}

type OperationReviewAuthorizationFactPortV4 interface {
	CreateOperationReviewAuthorizationV4(context.Context, OperationReviewAuthorizationFactV4) (OperationReviewAuthorizationFactV4, error)
	InspectOperationReviewAuthorizationV4(context.Context, string) (OperationReviewAuthorizationFactV4, error)
	CompareAndSwapOperationReviewAuthorizationV4(context.Context, OperationReviewAuthorizationCASRequestV4) (OperationReviewAuthorizationFactV4, error)
}

type OperationReviewCurrentReaderV4 interface {
	InspectOperationReviewCurrentV4(context.Context, OperationEffectIntentV3) (OperationReviewCurrentProjectionV4, error)
}

type OperationReviewAuthorizationGovernancePortV4 interface {
	CreateOperationReviewAuthorizationV4(context.Context, CreateOperationReviewAuthorizationRequestV4) (OperationReviewAuthorizationFactV4, error)
	InspectCurrentOperationReviewAuthorizationV4(context.Context, OperationSubjectV3, core.EffectIntentID, string) (OperationReviewAuthorizationFactV4, error)
}

func ValidateOperationReviewAuthorizationTransitionV4(current, next OperationReviewAuthorizationFactV4, now time.Time) error {
	if err := current.Validate(); err != nil {
		return err
	}
	if err := next.Validate(); err != nil {
		return err
	}
	if now.IsZero() || now.UnixNano() < current.UpdatedUnixNano || next.ID != current.ID || next.Revision != current.Revision+1 || next.Intent.IntentDigest != current.Intent.IntentDigest || next.Review.ProjectionDigest != current.Review.ProjectionDigest || next.Governance.SnapshotDigest != current.Governance.SnapshotDigest || next.FenceDigest != current.FenceDigest || next.CreatedUnixNano != current.CreatedUnixNano || next.ExpiresUnixNano != current.ExpiresUnixNano || next.UpdatedUnixNano != now.UnixNano() {
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Review Authorization immutable content or revision drifted")
	}
	if current.State != OperationReviewAuthorizationActiveV4 || next.State != OperationReviewAuthorizationRevokedV4 && next.State != OperationReviewAuthorizationExpiredV4 && next.State != OperationReviewAuthorizationSupersededV4 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidTransition, "Review Authorization may only leave active")
	}
	if next.State == OperationReviewAuthorizationExpiredV4 && now.Before(time.Unix(0, current.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "Review Authorization cannot expire before its TTL")
	}
	return nil
}

func DigestOperationReviewEvidenceV4(values []EvidenceRecordRefV2) (core.Digest, error) {
	values = normalizedOperationReviewEvidenceV4(values)
	if len(values) == 0 || len(values) > MaxReviewEvidenceV2 {
		return "", core.NewError(core.ErrorInvalidArgument, core.ReasonEvidenceUnavailable, "Review evidence set must be bounded and non-empty")
	}
	for index, value := range values {
		if err := value.Validate(); err != nil {
			return "", err
		}
		if index > 0 && values[index-1] == value {
			return "", core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Review evidence contains a duplicate")
		}
	}
	return core.CanonicalJSONDigest("praxis.runtime.operation-review-authorization", OperationReviewAuthorizationContractVersionV4, "OperationReviewEvidenceV4", values)
}

func normalizedOperationReviewEvidenceV4(values []EvidenceRecordRefV2) []EvidenceRecordRefV2 {
	result := append([]EvidenceRecordRefV2{}, values...)
	sort.Slice(result, func(i, j int) bool {
		if result[i].LedgerScopeDigest != result[j].LedgerScopeDigest {
			return result[i].LedgerScopeDigest < result[j].LedgerScopeDigest
		}
		if result[i].Sequence != result[j].Sequence {
			return result[i].Sequence < result[j].Sequence
		}
		return result[i].RecordDigest < result[j].RecordDigest
	})
	return result
}

func minimumOperationReviewAuthorizationExpiryV4(f OperationReviewAuthorizationFactV4) int64 {
	minimum := f.CreatedUnixNano + f.RequestedTTLUnixNano
	limits := []int64{f.Intent.IntentExpires, f.Review.ExpiresUnixNano, f.Governance.ExpiresUnixNano}
	for _, ref := range []OperationGovernanceFactRefV3{f.Review.Case, f.Review.Verdict, f.Review.Policy, f.Review.ReviewerAuthority, f.Review.Scope, f.Review.Binding, f.Governance.Identity, f.Governance.Binding, f.Governance.CurrentScope, f.Governance.Authority, f.Governance.Policy, f.Governance.Budget} {
		limits = append(limits, ref.ExpiresUnixNano)
	}
	if f.Review.Satisfaction != nil {
		limits = append(limits, f.Review.Satisfaction.Fact.ExpiresUnixNano)
	}
	for _, limit := range limits {
		if limit < minimum {
			minimum = limit
		}
	}
	return minimum
}

func validateOperationReviewIDV4(value string) error {
	if value == "" || len(value) > 256 || strings.TrimSpace(value) != value {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Review Authorization identity must be bounded and canonical")
	}
	for _, char := range []byte(value) {
		if char < 0x21 || char > 0x7e {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Review Authorization identity contains unstable characters")
		}
	}
	return nil
}

func validateOperationReviewTargetV4(value string) error {
	if value == "" || len(value) > 512 || strings.TrimSpace(value) != value {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Review target must be bounded and canonical")
	}
	for _, char := range []byte(value) {
		if char < 0x20 || char > 0x7e {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Review target contains unstable characters")
		}
	}
	return nil
}
