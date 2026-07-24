package ports

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const OperationReviewAuthorizationContractVersionV5 = "5.0.0"

type OperationReviewAuthorizationBasisV5 string

const (
	OperationReviewBasisAcceptedQuorumV5             OperationReviewAuthorizationBasisV5 = "accepted_quorum"
	OperationReviewBasisConditionalQuorumSatisfiedV5 OperationReviewAuthorizationBasisV5 = "conditional_quorum_satisfied"
	OperationReviewBasisPolicyNotRequiredV5          OperationReviewAuthorizationBasisV5 = "operation_not_required"
)

// The five Review refs are deliberately nominally distinct. A ref is an exact
// coordinate, never authority and never evidence by itself.
type OperationReviewCaseRefV5 struct {
	TenantID        core.TenantID `json:"tenant_id"`
	ID              string        `json:"id"`
	Revision        core.Revision `json:"revision"`
	Digest          core.Digest   `json:"digest"`
	ExpiresUnixNano int64         `json:"expires_unix_nano"`
}

type OperationReviewPanelRefV5 struct {
	TenantID        core.TenantID `json:"tenant_id"`
	ID              string        `json:"id"`
	Revision        core.Revision `json:"revision"`
	Digest          core.Digest   `json:"digest"`
	ExpiresUnixNano int64         `json:"expires_unix_nano"`
}

type OperationReviewQuorumDecisionRefV5 struct {
	TenantID        core.TenantID `json:"tenant_id"`
	ID              string        `json:"id"`
	Revision        core.Revision `json:"revision"`
	Digest          core.Digest   `json:"digest"`
	ExpiresUnixNano int64         `json:"expires_unix_nano"`
}

type OperationReviewVerdictRefV5 struct {
	TenantID        core.TenantID `json:"tenant_id"`
	ID              string        `json:"id"`
	Revision        core.Revision `json:"revision"`
	Digest          core.Digest   `json:"digest"`
	ExpiresUnixNano int64         `json:"expires_unix_nano"`
}

type OperationReviewBypassDecisionRefV5 struct {
	TenantID        core.TenantID `json:"tenant_id"`
	ID              string        `json:"id"`
	Revision        core.Revision `json:"revision"`
	Digest          core.Digest   `json:"digest"`
	ExpiresUnixNano int64         `json:"expires_unix_nano"`
}

func validateOperationReviewExactRefV5(tenant core.TenantID, id string, revision core.Revision, digest core.Digest, expires int64, now time.Time) error {
	if strings.TrimSpace(string(tenant)) == "" || validateOperationReviewIDV4(id) != nil || revision == 0 || expires <= 0 || now.IsZero() {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Review V5 exact ref is incomplete")
	}
	if err := digest.Validate(); err != nil {
		return err
	}
	if !now.Before(time.Unix(0, expires)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "Review V5 exact ref expired")
	}
	return nil
}

func (r OperationReviewCaseRefV5) Validate(now time.Time) error {
	return validateOperationReviewExactRefV5(r.TenantID, r.ID, r.Revision, r.Digest, r.ExpiresUnixNano, now)
}
func (r OperationReviewPanelRefV5) Validate(now time.Time) error {
	return validateOperationReviewExactRefV5(r.TenantID, r.ID, r.Revision, r.Digest, r.ExpiresUnixNano, now)
}
func (r OperationReviewQuorumDecisionRefV5) Validate(now time.Time) error {
	return validateOperationReviewExactRefV5(r.TenantID, r.ID, r.Revision, r.Digest, r.ExpiresUnixNano, now)
}
func (r OperationReviewVerdictRefV5) Validate(now time.Time) error {
	return validateOperationReviewExactRefV5(r.TenantID, r.ID, r.Revision, r.Digest, r.ExpiresUnixNano, now)
}
func (r OperationReviewBypassDecisionRefV5) Validate(now time.Time) error {
	return validateOperationReviewExactRefV5(r.TenantID, r.ID, r.Revision, r.Digest, r.ExpiresUnixNano, now)
}

type OperationReviewRoleCountV5 struct {
	Role     string `json:"role"`
	Count    uint32 `json:"count"`
	Required uint32 `json:"required"`
}

func (r OperationReviewRoleCountV5) Validate() error {
	if validateOperationReviewIDV4(r.Role) != nil || r.Required == 0 || r.Count < r.Required {
		return core.NewError(core.ErrorForbidden, core.ReasonReviewConditionUnsatisfied, "Review quorum role requirement is not satisfied")
	}
	return nil
}

type OperationReviewQuorumCurrentProjectionV5 struct {
	ContractVersion       string                                  `json:"contract_version"`
	Operation             OperationSubjectV3                      `json:"operation"`
	IntentID              core.EffectIntentID                     `json:"intent_id"`
	IntentRevision        core.Revision                           `json:"intent_revision"`
	IntentDigest          core.Digest                             `json:"intent_digest"`
	PayloadSchema         SchemaRefV2                             `json:"payload_schema"`
	PayloadDigest         core.Digest                             `json:"payload_digest"`
	PayloadRevision       core.Revision                           `json:"payload_revision"`
	Target                OperationReviewTargetRefV4              `json:"target"`
	Case                  OperationReviewCaseRefV5                `json:"case"`
	Panel                 OperationReviewPanelRefV5               `json:"panel"`
	QuorumDecision        OperationReviewQuorumDecisionRefV5      `json:"quorum_decision"`
	Verdict               OperationReviewVerdictRefV5             `json:"verdict"`
	QuorumPolicy          OperationGovernanceFactRefV3            `json:"quorum_policy"`
	ReviewerSetDigest     core.Digest                             `json:"reviewer_set_digest"`
	AcceptCount           uint32                                  `json:"accept_count"`
	Threshold             uint32                                  `json:"threshold"`
	SatisfiedRoleCounts   []OperationReviewRoleCountV5            `json:"satisfied_role_counts"`
	ReviewerAuthorityRefs []OperationGovernanceFactRefV3          `json:"reviewer_authority_refs"`
	BindingRefs           []OperationGovernanceFactRefV3          `json:"binding_refs"`
	ScopeRef              OperationGovernanceFactRefV3            `json:"scope_ref"`
	DecisionEvidence      []EvidenceRecordRefV2                   `json:"decision_evidence"`
	EvidenceDigest        core.Digest                             `json:"evidence_digest"`
	Basis                 OperationReviewAuthorizationBasisV5     `json:"basis"`
	Satisfaction          *OperationReviewConditionSatisfactionV4 `json:"satisfaction,omitempty"`
	Current               bool                                    `json:"current"`
	CurrentnessDigest     core.Digest                             `json:"currentness_digest"`
	ProjectionDigest      core.Digest                             `json:"projection_digest"`
	CheckedUnixNano       int64                                   `json:"checked_unix_nano"`
	ExpiresUnixNano       int64                                   `json:"expires_unix_nano"`
}

func (p OperationReviewQuorumCurrentProjectionV5) Validate(now time.Time) error {
	if p.ContractVersion != OperationReviewAuthorizationContractVersionV5 || p.Operation.Validate() != nil || p.IntentID == "" || p.IntentRevision == 0 || p.PayloadRevision == 0 || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano || now.IsZero() || now.UnixNano() < p.CheckedUnixNano {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonReviewVerdictMissing, "Review quorum projection identity or time is incomplete")
	}
	for _, digest := range []core.Digest{p.IntentDigest, p.PayloadDigest, p.ReviewerSetDigest, p.EvidenceDigest, p.CurrentnessDigest, p.ProjectionDigest} {
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
	if err := p.Case.Validate(now); err != nil {
		return err
	}
	if err := p.Panel.Validate(now); err != nil {
		return err
	}
	if err := p.QuorumDecision.Validate(now); err != nil {
		return err
	}
	if err := p.Verdict.Validate(now); err != nil {
		return err
	}
	tenant := p.Operation.ExecutionScope.Identity.TenantID
	if p.Case.TenantID != tenant || p.Panel.TenantID != tenant || p.QuorumDecision.TenantID != tenant || p.Verdict.TenantID != tenant {
		return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "Review quorum refs cross tenant boundaries")
	}
	if err := p.QuorumPolicy.Validate(now); err != nil {
		return err
	}
	if err := p.ScopeRef.Validate(now); err != nil {
		return err
	}
	if p.AcceptCount < p.Threshold || p.Threshold == 0 || len(p.ReviewerAuthorityRefs) < int(p.Threshold) || len(p.BindingRefs) == 0 || len(p.SatisfiedRoleCounts) == 0 {
		return core.NewError(core.ErrorForbidden, core.ReasonReviewConditionUnsatisfied, "Review quorum is not satisfied")
	}
	for i, role := range p.SatisfiedRoleCounts {
		if err := role.Validate(); err != nil {
			return err
		}
		if i > 0 && p.SatisfiedRoleCounts[i-1].Role >= role.Role {
			return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "Review role counts must be sorted and unique")
		}
	}
	for _, refs := range [][]OperationGovernanceFactRefV3{p.ReviewerAuthorityRefs, p.BindingRefs} {
		for i, ref := range refs {
			if err := ref.Validate(now); err != nil {
				return err
			}
			if i > 0 && refs[i-1].Ref >= ref.Ref {
				return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "Review exact ref set must be sorted and unique")
			}
		}
	}
	if len(p.DecisionEvidence) == 0 || len(p.DecisionEvidence) > MaxReviewEvidenceV2 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceUnavailable, "Review quorum requires bounded evidence")
	}
	evidenceDigest, err := DigestOperationReviewEvidenceV4(p.DecisionEvidence)
	if err != nil || evidenceDigest != p.EvidenceDigest {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Review quorum evidence drifted")
	}
	normalizedEvidence := normalizedOperationReviewEvidenceV4(p.DecisionEvidence)
	for index := 1; index < len(normalizedEvidence); index++ {
		previous, current := normalizedEvidence[index-1], normalizedEvidence[index]
		if previous.LedgerScopeDigest == current.LedgerScopeDigest && previous.Sequence == current.Sequence {
			return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Review quorum evidence coordinate carries multiple digests")
		}
	}
	switch p.Basis {
	case OperationReviewBasisAcceptedQuorumV5:
		if p.Satisfaction != nil {
			return core.NewError(core.ErrorConflict, core.ReasonReviewConditionUnsatisfied, "accepted quorum cannot carry satisfaction")
		}
	case OperationReviewBasisConditionalQuorumSatisfiedV5:
		if p.Satisfaction == nil {
			return core.NewError(core.ErrorForbidden, core.ReasonReviewConditionUnsatisfied, "conditional quorum requires satisfaction")
		}
		if err := p.Satisfaction.Validate(now); err != nil {
			return err
		}
	default:
		return core.NewError(core.ErrorForbidden, core.ReasonReviewVerdictStale, "Review quorum basis cannot authorize")
	}
	if !p.Current || !now.Before(time.Unix(0, p.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "Review quorum projection is inactive or expired")
	}
	if p.ExpiresUnixNano != minimumQuorumSourceExpiryV5(p) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "Review quorum projection TTL exceeds an exact source")
	}
	digest, err := p.DigestV5()
	if err != nil || digest != p.ProjectionDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Review quorum projection digest drifted")
	}
	return nil
}

func (p OperationReviewQuorumCurrentProjectionV5) DigestV5() (core.Digest, error) {
	copy := p
	copy.ProjectionDigest = ""
	copy.DecisionEvidence = normalizedOperationReviewEvidenceV4(copy.DecisionEvidence)
	copy.SatisfiedRoleCounts = append([]OperationReviewRoleCountV5{}, copy.SatisfiedRoleCounts...)
	sort.Slice(copy.SatisfiedRoleCounts, func(i, j int) bool { return copy.SatisfiedRoleCounts[i].Role < copy.SatisfiedRoleCounts[j].Role })
	copy.ReviewerAuthorityRefs = normalizedGovernanceRefsV5(copy.ReviewerAuthorityRefs)
	copy.BindingRefs = normalizedGovernanceRefsV5(copy.BindingRefs)
	if copy.Satisfaction != nil {
		satisfaction := *copy.Satisfaction
		satisfaction.Evidence = normalizedOperationReviewEvidenceV4(satisfaction.Evidence)
		copy.Satisfaction = &satisfaction
	}
	return core.CanonicalJSONDigest("praxis.runtime.operation-review-authorization", OperationReviewAuthorizationContractVersionV5, "OperationReviewQuorumCurrentProjectionV5", copy)
}

func SealOperationReviewQuorumCurrentProjectionV5(p OperationReviewQuorumCurrentProjectionV5, now time.Time) (OperationReviewQuorumCurrentProjectionV5, error) {
	p.ContractVersion = OperationReviewAuthorizationContractVersionV5
	p.DecisionEvidence = normalizedOperationReviewEvidenceV4(p.DecisionEvidence)
	if p.Satisfaction != nil {
		satisfaction := *p.Satisfaction
		satisfaction.Evidence = normalizedOperationReviewEvidenceV4(satisfaction.Evidence)
		digest, err := DigestOperationReviewEvidenceV4(satisfaction.Evidence)
		if err != nil {
			return OperationReviewQuorumCurrentProjectionV5{}, err
		}
		satisfaction.EvidenceDigest = digest
		p.Satisfaction = &satisfaction
	}
	p.SatisfiedRoleCounts = append([]OperationReviewRoleCountV5{}, p.SatisfiedRoleCounts...)
	sort.Slice(p.SatisfiedRoleCounts, func(i, j int) bool { return p.SatisfiedRoleCounts[i].Role < p.SatisfiedRoleCounts[j].Role })
	p.ReviewerAuthorityRefs = normalizedGovernanceRefsV5(p.ReviewerAuthorityRefs)
	p.BindingRefs = normalizedGovernanceRefsV5(p.BindingRefs)
	evidence, err := DigestOperationReviewEvidenceV4(p.DecisionEvidence)
	if err != nil {
		return OperationReviewQuorumCurrentProjectionV5{}, err
	}
	p.EvidenceDigest = evidence
	p.ProjectionDigest = ""
	digest, err := p.DigestV5()
	if err != nil {
		return OperationReviewQuorumCurrentProjectionV5{}, err
	}
	p.ProjectionDigest = digest
	return p, p.Validate(now)
}

type OperationReviewPolicyNotRequiredCurrentProjectionV5 struct {
	ContractVersion         string                             `json:"contract_version"`
	Operation               OperationSubjectV3                 `json:"operation"`
	IntentID                core.EffectIntentID                `json:"intent_id"`
	IntentRevision          core.Revision                      `json:"intent_revision"`
	IntentDigest            core.Digest                        `json:"intent_digest"`
	PayloadSchema           SchemaRefV2                        `json:"payload_schema"`
	PayloadDigest           core.Digest                        `json:"payload_digest"`
	PayloadRevision         core.Revision                      `json:"payload_revision"`
	Target                  OperationReviewTargetRefV4         `json:"target"`
	Case                    OperationReviewCaseRefV5           `json:"case"`
	BypassDecision          OperationReviewBypassDecisionRefV5 `json:"bypass_decision"`
	PolicyCurrentProjection OperationGovernanceFactRefV3       `json:"policy_current_projection"`
	PolicyDecisionRef       OperationGovernanceFactRefV3       `json:"policy_decision_ref"`
	ScopeRef                OperationGovernanceFactRefV3       `json:"scope_ref"`
	BindingRef              OperationGovernanceFactRefV3       `json:"binding_ref"`
	ActorAuthorityRef       OperationGovernanceFactRefV3       `json:"actor_authority_ref"`
	Current                 bool                               `json:"current"`
	CurrentnessDigest       core.Digest                        `json:"currentness_digest"`
	ProjectionDigest        core.Digest                        `json:"projection_digest"`
	CheckedUnixNano         int64                              `json:"checked_unix_nano"`
	ExpiresUnixNano         int64                              `json:"expires_unix_nano"`
}

func (p OperationReviewPolicyNotRequiredCurrentProjectionV5) Validate(now time.Time) error {
	if p.ContractVersion != OperationReviewAuthorizationContractVersionV5 || p.Operation.Validate() != nil || p.IntentID == "" || p.IntentRevision == 0 || p.PayloadRevision == 0 || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano || now.IsZero() || now.UnixNano() < p.CheckedUnixNano {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonReviewVerdictMissing, "Policy-not-required projection identity or time is incomplete")
	}
	for _, digest := range []core.Digest{p.IntentDigest, p.PayloadDigest, p.CurrentnessDigest, p.ProjectionDigest} {
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
	if err := p.Case.Validate(now); err != nil {
		return err
	}
	if err := p.BypassDecision.Validate(now); err != nil {
		return err
	}
	if p.Case.TenantID != p.Operation.ExecutionScope.Identity.TenantID || p.BypassDecision.TenantID != p.Case.TenantID {
		return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "Policy-not-required refs cross tenant boundaries")
	}
	for _, ref := range []OperationGovernanceFactRefV3{p.PolicyCurrentProjection, p.PolicyDecisionRef, p.ScopeRef, p.BindingRef, p.ActorAuthorityRef} {
		if err := ref.Validate(now); err != nil {
			return err
		}
	}
	if !p.Current || !now.Before(time.Unix(0, p.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "Policy-not-required projection is inactive or expired")
	}
	if p.ExpiresUnixNano != minimumPolicyNotRequiredSourceExpiryV5(p) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "Policy-not-required projection TTL exceeds an exact source")
	}
	digest, err := p.DigestV5()
	if err != nil || digest != p.ProjectionDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Policy-not-required projection digest drifted")
	}
	return nil
}

func (p OperationReviewPolicyNotRequiredCurrentProjectionV5) DigestV5() (core.Digest, error) {
	copy := p
	copy.ProjectionDigest = ""
	return core.CanonicalJSONDigest("praxis.runtime.operation-review-authorization", OperationReviewAuthorizationContractVersionV5, "OperationReviewPolicyNotRequiredCurrentProjectionV5", copy)
}

func SealOperationReviewPolicyNotRequiredCurrentProjectionV5(p OperationReviewPolicyNotRequiredCurrentProjectionV5, now time.Time) (OperationReviewPolicyNotRequiredCurrentProjectionV5, error) {
	p.ContractVersion = OperationReviewAuthorizationContractVersionV5
	p.ProjectionDigest = ""
	digest, err := p.DigestV5()
	if err != nil {
		return OperationReviewPolicyNotRequiredCurrentProjectionV5{}, err
	}
	p.ProjectionDigest = digest
	return p, p.Validate(now)
}

type OperationReviewCurrentRequestV5 struct {
	Intent OperationEffectIntentV3             `json:"intent"`
	Basis  OperationReviewAuthorizationBasisV5 `json:"basis"`
}

func (r OperationReviewCurrentRequestV5) Validate() error {
	if err := r.Intent.Validate(); err != nil {
		return err
	}
	switch r.Basis {
	case OperationReviewBasisAcceptedQuorumV5, OperationReviewBasisConditionalQuorumSatisfiedV5, OperationReviewBasisPolicyNotRequiredV5:
		return nil
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonReviewVerdictMissing, "Review V5 request basis is unsupported")
	}
}

type OperationReviewCurrentProjectionV5 struct {
	ContractVersion   string                                               `json:"contract_version"`
	Basis             OperationReviewAuthorizationBasisV5                  `json:"basis"`
	Quorum            *OperationReviewQuorumCurrentProjectionV5            `json:"quorum,omitempty"`
	PolicyNotRequired *OperationReviewPolicyNotRequiredCurrentProjectionV5 `json:"policy_not_required,omitempty"`
	ProjectionDigest  core.Digest                                          `json:"projection_digest"`
	ExpiresUnixNano   int64                                                `json:"expires_unix_nano"`
}

func (p OperationReviewCurrentProjectionV5) Validate(now time.Time) error {
	if p.ContractVersion != OperationReviewAuthorizationContractVersionV5 || now.IsZero() || p.ExpiresUnixNano <= 0 || !now.Before(time.Unix(0, p.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "Review V5 union is incomplete or expired")
	}
	if err := p.ProjectionDigest.Validate(); err != nil {
		return err
	}
	var branchExpiry int64
	switch p.Basis {
	case OperationReviewBasisAcceptedQuorumV5, OperationReviewBasisConditionalQuorumSatisfiedV5:
		if p.Quorum == nil || p.PolicyNotRequired != nil || p.Quorum.Basis != p.Basis {
			return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "Review V5 union quorum branch drifted")
		}
		if err := p.Quorum.Validate(now); err != nil {
			return err
		}
		branchExpiry = minimumQuorumExpiryV5(*p.Quorum)
	case OperationReviewBasisPolicyNotRequiredV5:
		if p.PolicyNotRequired == nil || p.Quorum != nil {
			return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "Review V5 union not-required branch drifted")
		}
		if err := p.PolicyNotRequired.Validate(now); err != nil {
			return err
		}
		branchExpiry = minimumPolicyNotRequiredExpiryV5(*p.PolicyNotRequired)
	default:
		return core.NewError(core.ErrorForbidden, core.ReasonReviewVerdictStale, "Review V5 union basis cannot authorize")
	}
	if p.ExpiresUnixNano != branchExpiry {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "Review V5 union TTL exceeds an exact source")
	}
	digest, err := p.DigestV5()
	if err != nil || digest != p.ProjectionDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Review V5 union digest drifted")
	}
	return nil
}

func (p OperationReviewCurrentProjectionV5) ValidateAgainstIntent(intent OperationEffectIntentV3, current OperationGovernanceSnapshotV3, now time.Time) error {
	if err := p.Validate(now); err != nil {
		return err
	}
	intentDigest, err := intent.DigestV3()
	if err != nil {
		return err
	}
	var operation OperationSubjectV3
	var intentID core.EffectIntentID
	var intentRevision, payloadRevision core.Revision
	var gotIntentDigest, payloadDigest core.Digest
	var payloadSchema SchemaRefV2
	var target OperationReviewTargetRefV4
	var caseID string
	switch p.Basis {
	case OperationReviewBasisAcceptedQuorumV5, OperationReviewBasisConditionalQuorumSatisfiedV5:
		q := p.Quorum
		operation, intentID, intentRevision, gotIntentDigest, payloadSchema, payloadDigest, payloadRevision, target, caseID = q.Operation, q.IntentID, q.IntentRevision, q.IntentDigest, q.PayloadSchema, q.PayloadDigest, q.PayloadRevision, q.Target, q.Case.ID
		if q.ScopeRef != current.CurrentScope || q.QuorumPolicy.Digest != intent.Review.PolicyDigest {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "Review quorum policy or scope drifted")
		}
	case OperationReviewBasisPolicyNotRequiredV5:
		n := p.PolicyNotRequired
		operation, intentID, intentRevision, gotIntentDigest, payloadSchema, payloadDigest, payloadRevision, target, caseID = n.Operation, n.IntentID, n.IntentRevision, n.IntentDigest, n.PayloadSchema, n.PayloadDigest, n.PayloadRevision, n.Target, n.Case.ID
		if n.ScopeRef != current.CurrentScope || n.BindingRef != current.Binding || n.ActorAuthorityRef != current.Authority || n.PolicyCurrentProjection.Digest != intent.Review.PolicyDigest {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "Policy-not-required governance drifted")
		}
	}
	if !SameOperationSubjectV3(operation, intent.Operation) || intentID != intent.ID || intentRevision != intent.Revision || gotIntentDigest != intentDigest || payloadSchema != intent.Payload.Schema || payloadDigest != intent.Payload.ContentDigest || payloadRevision != intent.PayloadRevision || target.Ref != intent.Target || target.Revision != intent.Review.CandidateRevision || target.Digest != intent.Review.CandidateDigest || caseID != intent.Review.CaseRef {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewCandidateConflict, "Review V5 target, intent or payload drifted")
	}
	return nil
}

func (p OperationReviewCurrentProjectionV5) DigestV5() (core.Digest, error) {
	copy := p
	copy.ProjectionDigest = ""
	return core.CanonicalJSONDigest("praxis.runtime.operation-review-authorization", OperationReviewAuthorizationContractVersionV5, "OperationReviewCurrentProjectionV5", copy)
}

func SealOperationReviewCurrentProjectionV5(p OperationReviewCurrentProjectionV5, now time.Time) (OperationReviewCurrentProjectionV5, error) {
	p.ContractVersion = OperationReviewAuthorizationContractVersionV5
	if p.Quorum != nil {
		p.ExpiresUnixNano = minimumQuorumExpiryV5(*p.Quorum)
	}
	if p.PolicyNotRequired != nil {
		p.ExpiresUnixNano = minimumPolicyNotRequiredExpiryV5(*p.PolicyNotRequired)
	}
	p.ProjectionDigest = ""
	digest, err := p.DigestV5()
	if err != nil {
		return OperationReviewCurrentProjectionV5{}, err
	}
	p.ProjectionDigest = digest
	return p, p.Validate(now)
}

type OperationReviewAuthorizationStateV5 string

const (
	OperationReviewAuthorizationActiveV5     OperationReviewAuthorizationStateV5 = "active"
	OperationReviewAuthorizationRevokedV5    OperationReviewAuthorizationStateV5 = "revoked"
	OperationReviewAuthorizationExpiredV5    OperationReviewAuthorizationStateV5 = "expired"
	OperationReviewAuthorizationSupersededV5 OperationReviewAuthorizationStateV5 = "superseded"
)

type OperationReviewAuthorizationFactV5 struct {
	ContractVersion      string                              `json:"contract_version"`
	ID                   string                              `json:"id"`
	Revision             core.Revision                       `json:"revision"`
	State                OperationReviewAuthorizationStateV5 `json:"state"`
	Intent               OperationReviewIntentBindingV4      `json:"intent"`
	Review               OperationReviewCurrentProjectionV5  `json:"review"`
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

func (f OperationReviewAuthorizationFactV5) Validate() error {
	created := time.Unix(0, f.CreatedUnixNano)
	if f.ContractVersion != OperationReviewAuthorizationContractVersionV5 || validateOperationReviewIDV4(f.ID) != nil || f.Revision == 0 || f.RequestedTTLUnixNano <= 0 || f.CreatedUnixNano <= 0 || f.UpdatedUnixNano < f.CreatedUnixNano {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonReviewVerdictMissing, "Review Authorization V5 identity and time are incomplete")
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
	if err := validateAuthorizationFactBindingsV5(f); err != nil {
		return err
	}
	if err := f.Fence.Validate(); err != nil {
		return err
	}
	fenceDigest, err := DigestOperationExecutionFenceV3(f.Fence, f.Intent.Operation)
	if err != nil || fenceDigest != f.FenceDigest || f.Fence.EffectIntentID != f.Intent.IntentID || f.Fence.EffectIntentRevision != f.Intent.IntentRevision || f.Fence.CanonicalPayloadDigest != f.Intent.PayloadDigest || f.Fence.CapabilityGrantDigest != f.Governance.CapabilityGrantDigest {
		return core.NewError(core.ErrorConflict, core.ReasonEffectFenceStale, "Review Authorization V5 Fence drifted")
	}
	minimum := minimumOperationReviewAuthorizationExpiryV5(f)
	if f.ExpiresUnixNano != minimum || f.Fence.ExpiresAt.UnixNano() != minimum {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "Review Authorization V5 TTL exceeds a governing fact")
	}
	switch f.State {
	case OperationReviewAuthorizationActiveV5:
		if f.InvalidationReason != "" || f.UpdatedUnixNano >= f.ExpiresUnixNano {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "active Review Authorization V5 is invalid")
		}
	case OperationReviewAuthorizationRevokedV5, OperationReviewAuthorizationSupersededV5:
		if f.InvalidationReason == "" {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "invalidated Review Authorization V5 requires a reason")
		}
	case OperationReviewAuthorizationExpiredV5:
		if f.InvalidationReason != core.ReasonReviewVerdictStale || f.UpdatedUnixNano < f.ExpiresUnixNano {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "expired Review Authorization V5 has not crossed TTL")
		}
	default:
		return core.NewError(core.ErrorForbidden, core.ReasonReviewVerdictStale, "unknown Review Authorization V5 state")
	}
	digest, err := f.DigestV5()
	if err != nil || digest != f.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Review Authorization V5 digest drifted")
	}
	return nil
}

func (f OperationReviewAuthorizationFactV5) DigestV5() (core.Digest, error) {
	copy := f
	copy.Digest = ""
	return core.CanonicalJSONDigest("praxis.runtime.operation-review-authorization", OperationReviewAuthorizationContractVersionV5, "OperationReviewAuthorizationFactV5", copy)
}

func SealOperationReviewAuthorizationFactV5(f OperationReviewAuthorizationFactV5) (OperationReviewAuthorizationFactV5, error) {
	f.ContractVersion = OperationReviewAuthorizationContractVersionV5
	f.Digest = ""
	digest, err := f.DigestV5()
	if err != nil {
		return OperationReviewAuthorizationFactV5{}, err
	}
	f.Digest = digest
	return f, f.Validate()
}

type OperationReviewAuthorizationRefV5 struct {
	ID       string        `json:"id"`
	Revision core.Revision `json:"revision"`
	Digest   core.Digest   `json:"digest"`
}

func (r OperationReviewAuthorizationRefV5) Validate() error {
	if validateOperationReviewIDV4(r.ID) != nil || r.Revision == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Review Authorization V5 ref is incomplete")
	}
	return r.Digest.Validate()
}
func (f OperationReviewAuthorizationFactV5) RefV5() OperationReviewAuthorizationRefV5 {
	return OperationReviewAuthorizationRefV5{ID: f.ID, Revision: f.Revision, Digest: f.Digest}
}

type CreateOperationReviewAuthorizationRequestV5 struct {
	AuthorizationID        string                              `json:"authorization_id"`
	Operation              OperationSubjectV3                  `json:"operation"`
	EffectID               core.EffectIntentID                 `json:"effect_id"`
	ExpectedEffectRevision core.Revision                       `json:"expected_effect_revision"`
	Basis                  OperationReviewAuthorizationBasisV5 `json:"basis"`
	RequestedTTL           time.Duration                       `json:"requested_ttl"`
}

func (r CreateOperationReviewAuthorizationRequestV5) Validate() error {
	if validateOperationReviewIDV4(r.AuthorizationID) != nil || r.Operation.Validate() != nil || r.EffectID == "" || r.ExpectedEffectRevision == 0 || r.RequestedTTL <= 0 || r.RequestedTTL > MaxDispatchPermitTTL {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonReviewVerdictMissing, "Review Authorization V5 request requires exact Effect and bounded TTL")
	}
	switch r.Basis {
	case OperationReviewBasisAcceptedQuorumV5, OperationReviewBasisConditionalQuorumSatisfiedV5, OperationReviewBasisPolicyNotRequiredV5:
		return nil
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonReviewVerdictMissing, "Review Authorization V5 request basis is unsupported")
	}
}

type OperationReviewAuthorizationCASRequestV5 struct {
	ExpectedRevision core.Revision                      `json:"expected_revision"`
	Next             OperationReviewAuthorizationFactV5 `json:"next"`
}

type OperationReviewAuthorizationFactPortV5 interface {
	CreateOperationReviewAuthorizationV5(context.Context, OperationReviewAuthorizationFactV5) (OperationReviewAuthorizationFactV5, error)
	InspectOperationReviewAuthorizationV5(context.Context, string) (OperationReviewAuthorizationFactV5, error)
	InspectOperationReviewAuthorizationExactV5(context.Context, OperationReviewAuthorizationRefV5) (OperationReviewAuthorizationFactV5, error)
	CompareAndSwapOperationReviewAuthorizationV5(context.Context, OperationReviewAuthorizationCASRequestV5) (OperationReviewAuthorizationFactV5, error)
}

type OperationReviewCurrentReaderV5 interface {
	InspectOperationReviewCurrentV5(context.Context, OperationReviewCurrentRequestV5) (OperationReviewCurrentProjectionV5, error)
}

type OperationReviewAuthorizationGovernancePortV5 interface {
	CreateOperationReviewAuthorizationV5(context.Context, CreateOperationReviewAuthorizationRequestV5) (OperationReviewAuthorizationFactV5, error)
	InspectCurrentOperationReviewAuthorizationV5(context.Context, OperationSubjectV3, core.EffectIntentID, string) (OperationReviewAuthorizationFactV5, error)
	CompareAndSwapOperationReviewAuthorizationV5(context.Context, OperationReviewAuthorizationCASRequestV5) (OperationReviewAuthorizationFactV5, error)
}

func ValidateOperationReviewAuthorizationTransitionV5(current, next OperationReviewAuthorizationFactV5, now time.Time) error {
	if err := current.Validate(); err != nil {
		return err
	}
	if err := next.Validate(); err != nil {
		return err
	}
	if now.IsZero() || now.UnixNano() < current.UpdatedUnixNano || next.ID != current.ID || next.Revision != current.Revision+1 || next.Intent.IntentDigest != current.Intent.IntentDigest || next.Review.ProjectionDigest != current.Review.ProjectionDigest || next.Governance.SnapshotDigest != current.Governance.SnapshotDigest || next.FenceDigest != current.FenceDigest || next.CreatedUnixNano != current.CreatedUnixNano || next.ExpiresUnixNano != current.ExpiresUnixNano || next.UpdatedUnixNano != now.UnixNano() {
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Review Authorization V5 immutable content or revision drifted")
	}
	if current.State != OperationReviewAuthorizationActiveV5 || next.State != OperationReviewAuthorizationRevokedV5 && next.State != OperationReviewAuthorizationExpiredV5 && next.State != OperationReviewAuthorizationSupersededV5 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidTransition, "Review Authorization V5 may only leave active")
	}
	if next.State == OperationReviewAuthorizationExpiredV5 && now.Before(time.Unix(0, current.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "Review Authorization V5 cannot expire before TTL")
	}
	return nil
}

func normalizedGovernanceRefsV5(values []OperationGovernanceFactRefV3) []OperationGovernanceFactRefV3 {
	result := append([]OperationGovernanceFactRefV3{}, values...)
	sort.Slice(result, func(i, j int) bool {
		if result[i].Ref != result[j].Ref {
			return result[i].Ref < result[j].Ref
		}
		if result[i].Revision != result[j].Revision {
			return result[i].Revision < result[j].Revision
		}
		return result[i].Digest < result[j].Digest
	})
	return result
}

func minimumQuorumExpiryV5(p OperationReviewQuorumCurrentProjectionV5) int64 {
	limits := []int64{p.ExpiresUnixNano, p.Case.ExpiresUnixNano, p.Panel.ExpiresUnixNano, p.QuorumDecision.ExpiresUnixNano, p.Verdict.ExpiresUnixNano, p.QuorumPolicy.ExpiresUnixNano, p.ScopeRef.ExpiresUnixNano}
	for _, refs := range [][]OperationGovernanceFactRefV3{p.ReviewerAuthorityRefs, p.BindingRefs} {
		for _, ref := range refs {
			limits = append(limits, ref.ExpiresUnixNano)
		}
	}
	if p.Satisfaction != nil {
		limits = append(limits, p.Satisfaction.Fact.ExpiresUnixNano)
	}
	return minimumPositiveV5(limits...)
}
func minimumQuorumSourceExpiryV5(p OperationReviewQuorumCurrentProjectionV5) int64 {
	limits := []int64{p.Case.ExpiresUnixNano, p.Panel.ExpiresUnixNano, p.QuorumDecision.ExpiresUnixNano, p.Verdict.ExpiresUnixNano, p.QuorumPolicy.ExpiresUnixNano, p.ScopeRef.ExpiresUnixNano}
	for _, refs := range [][]OperationGovernanceFactRefV3{p.ReviewerAuthorityRefs, p.BindingRefs} {
		for _, ref := range refs {
			limits = append(limits, ref.ExpiresUnixNano)
		}
	}
	if p.Satisfaction != nil {
		limits = append(limits, p.Satisfaction.Fact.ExpiresUnixNano)
	}
	return minimumPositiveV5(limits...)
}
func minimumPolicyNotRequiredExpiryV5(p OperationReviewPolicyNotRequiredCurrentProjectionV5) int64 {
	return minimumPositiveV5(p.ExpiresUnixNano, p.Case.ExpiresUnixNano, p.BypassDecision.ExpiresUnixNano, p.PolicyCurrentProjection.ExpiresUnixNano, p.PolicyDecisionRef.ExpiresUnixNano, p.ScopeRef.ExpiresUnixNano, p.BindingRef.ExpiresUnixNano, p.ActorAuthorityRef.ExpiresUnixNano)
}
func minimumPolicyNotRequiredSourceExpiryV5(p OperationReviewPolicyNotRequiredCurrentProjectionV5) int64 {
	return minimumPositiveV5(p.Case.ExpiresUnixNano, p.BypassDecision.ExpiresUnixNano, p.PolicyCurrentProjection.ExpiresUnixNano, p.PolicyDecisionRef.ExpiresUnixNano, p.ScopeRef.ExpiresUnixNano, p.BindingRef.ExpiresUnixNano, p.ActorAuthorityRef.ExpiresUnixNano)
}
func minimumPositiveV5(values ...int64) int64 {
	var minimum int64
	for _, value := range values {
		if value > 0 && (minimum == 0 || value < minimum) {
			minimum = value
		}
	}
	return minimum
}

func validateAuthorizationFactBindingsV5(f OperationReviewAuthorizationFactV5) error {
	if f.Intent.IntentID == "" || !SameOperationSubjectV3(f.Intent.Operation, reviewOperationV5(f.Review)) || f.Intent.IntentID != reviewIntentIDV5(f.Review) || f.Intent.IntentRevision != reviewIntentRevisionV5(f.Review) || f.Intent.IntentDigest != reviewIntentDigestV5(f.Review) || f.Intent.PayloadSchema != reviewPayloadSchemaV5(f.Review) || f.Intent.PayloadDigest != reviewPayloadDigestV5(f.Review) || f.Intent.PayloadRevision != reviewPayloadRevisionV5(f.Review) || f.Intent.Target != reviewTargetV5(f.Review).Ref || f.Intent.ReviewBinding.CaseRef != reviewCaseIDV5(f.Review) || f.Intent.ReviewBinding.CandidateRevision != reviewTargetV5(f.Review).Revision || f.Intent.ReviewBinding.CandidateDigest != reviewTargetV5(f.Review).Digest || f.Intent.Provider.BindingSetID != f.Governance.Binding.Ref || f.Intent.Provider.BindingSetRevision != f.Governance.Binding.Revision || f.Intent.Authority.Ref != f.Governance.Authority.Ref || f.Intent.Authority.Revision != f.Governance.Authority.Revision || f.Intent.Authority.Digest != f.Governance.Authority.Digest || f.Intent.DispatchPolicy.Ref != f.Governance.Policy.Ref || f.Intent.DispatchPolicy.Revision != f.Governance.Policy.Revision || f.Intent.DispatchPolicy.Digest != f.Governance.Policy.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "Review Authorization V5 bindings do not form one exact fact")
	}
	if f.Review.Quorum != nil && (f.Review.Quorum.ScopeRef != f.Governance.CurrentScope || f.Review.Quorum.QuorumPolicy.Digest != f.Intent.ReviewBinding.PolicyDigest) {
		return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "Review quorum governance bindings drifted")
	}
	if f.Review.PolicyNotRequired != nil {
		n := f.Review.PolicyNotRequired
		if n.ScopeRef != f.Governance.CurrentScope || n.BindingRef != f.Governance.Binding || n.ActorAuthorityRef != f.Governance.Authority || n.PolicyCurrentProjection.Digest != f.Intent.ReviewBinding.PolicyDigest {
			return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "Policy-not-required governance bindings drifted")
		}
	}
	return nil
}

func reviewOperationV5(p OperationReviewCurrentProjectionV5) OperationSubjectV3 {
	if p.Quorum != nil {
		return p.Quorum.Operation
	}
	if p.PolicyNotRequired != nil {
		return p.PolicyNotRequired.Operation
	}
	return OperationSubjectV3{}
}

func reviewIntentIDV5(p OperationReviewCurrentProjectionV5) core.EffectIntentID {
	if p.Quorum != nil {
		return p.Quorum.IntentID
	}
	if p.PolicyNotRequired != nil {
		return p.PolicyNotRequired.IntentID
	}
	return ""
}
func reviewIntentRevisionV5(p OperationReviewCurrentProjectionV5) core.Revision {
	if p.Quorum != nil {
		return p.Quorum.IntentRevision
	}
	if p.PolicyNotRequired != nil {
		return p.PolicyNotRequired.IntentRevision
	}
	return 0
}
func reviewIntentDigestV5(p OperationReviewCurrentProjectionV5) core.Digest {
	if p.Quorum != nil {
		return p.Quorum.IntentDigest
	}
	if p.PolicyNotRequired != nil {
		return p.PolicyNotRequired.IntentDigest
	}
	return ""
}
func reviewPayloadSchemaV5(p OperationReviewCurrentProjectionV5) SchemaRefV2 {
	if p.Quorum != nil {
		return p.Quorum.PayloadSchema
	}
	if p.PolicyNotRequired != nil {
		return p.PolicyNotRequired.PayloadSchema
	}
	return SchemaRefV2{}
}
func reviewPayloadDigestV5(p OperationReviewCurrentProjectionV5) core.Digest {
	if p.Quorum != nil {
		return p.Quorum.PayloadDigest
	}
	if p.PolicyNotRequired != nil {
		return p.PolicyNotRequired.PayloadDigest
	}
	return ""
}
func reviewPayloadRevisionV5(p OperationReviewCurrentProjectionV5) core.Revision {
	if p.Quorum != nil {
		return p.Quorum.PayloadRevision
	}
	if p.PolicyNotRequired != nil {
		return p.PolicyNotRequired.PayloadRevision
	}
	return 0
}
func reviewTargetV5(p OperationReviewCurrentProjectionV5) OperationReviewTargetRefV4 {
	if p.Quorum != nil {
		return p.Quorum.Target
	}
	if p.PolicyNotRequired != nil {
		return p.PolicyNotRequired.Target
	}
	return OperationReviewTargetRefV4{}
}
func reviewCaseIDV5(p OperationReviewCurrentProjectionV5) string {
	if p.Quorum != nil {
		return p.Quorum.Case.ID
	}
	if p.PolicyNotRequired != nil {
		return p.PolicyNotRequired.Case.ID
	}
	return ""
}

func minimumOperationReviewAuthorizationExpiryV5(f OperationReviewAuthorizationFactV5) int64 {
	minimum := minimumPositiveV5(f.CreatedUnixNano+f.RequestedTTLUnixNano, f.Intent.IntentExpires, f.Review.ExpiresUnixNano, f.Governance.ExpiresUnixNano)
	for _, ref := range []OperationGovernanceFactRefV3{f.Governance.Identity, f.Governance.Binding, f.Governance.CurrentScope, f.Governance.Authority, f.Governance.Policy, f.Governance.Budget} {
		minimum = minimumPositiveV5(minimum, ref.ExpiresUnixNano)
	}
	return minimum
}

// DigestOperationGovernanceForReviewAuthorizationV5 deliberately excludes the
// legacy V3 Review field. V5 obtains Review currentness only from its V5 Reader.
func DigestOperationGovernanceForReviewAuthorizationV5(s OperationGovernanceSnapshotV3) (core.Digest, error) {
	neutral := struct {
		Operation             OperationSubjectV3                 `json:"operation"`
		Active                bool                               `json:"active"`
		ProjectionWatermark   uint64                             `json:"projection_watermark"`
		Identity              OperationGovernanceFactRefV3       `json:"identity"`
		Binding               OperationGovernanceFactRefV3       `json:"binding"`
		CurrentScope          OperationGovernanceFactRefV3       `json:"current_scope"`
		Authority             OperationGovernanceFactRefV3       `json:"authority"`
		Budget                OperationGovernanceFactRefV3       `json:"budget"`
		Policy                OperationGovernanceFactRefV3       `json:"policy"`
		Provider              ProviderBindingRefV2               `json:"provider"`
		EnforcementPoint      ProviderBindingRefV2               `json:"enforcement_point"`
		CapabilityGrantDigest core.Digest                        `json:"capability_grant_digest"`
		Credentials           []OperationCredentialCurrentFactV3 `json:"credentials"`
		ExpiresUnixNano       int64                              `json:"expires_unix_nano"`
	}{s.Operation, s.Active, s.ProjectionWatermark, s.Identity, s.Binding, s.CurrentScope, s.Authority, s.Budget, s.Policy, s.Provider, s.EnforcementPoint, s.CapabilityGrantDigest, append([]OperationCredentialCurrentFactV3{}, s.Credentials...), s.ExpiresUnixNano}
	if neutral.Credentials == nil {
		neutral.Credentials = []OperationCredentialCurrentFactV3{}
	}
	return core.CanonicalJSONDigest("praxis.runtime.operation-review-authorization", OperationReviewAuthorizationContractVersionV5, "OperationGovernanceSnapshotForReviewV5", neutral)
}
