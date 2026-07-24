package contract

import (
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const BypassDecisionContractV1 = "praxis.review/bypass-decision-v1"

type BypassDecisionStateV1 string

const (
	BypassDecisionActiveV1     BypassDecisionStateV1 = "active"
	BypassDecisionRevokedV1    BypassDecisionStateV1 = "revoked"
	BypassDecisionExpiredV1    BypassDecisionStateV1 = "expired"
	BypassDecisionSupersededV1 BypassDecisionStateV1 = "superseded"
)

type BypassDecisionExactRefV1 struct {
	TenantID core.TenantID `json:"tenant_id"`
	ID       string        `json:"id"`
	Revision core.Revision `json:"revision"`
	Digest   core.Digest   `json:"digest"`
}

func (r BypassDecisionExactRefV1) Validate() error {
	if blank(string(r.TenantID)) || invalidID(r.ID) || r.Revision == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "bypass Decision exact ref is incomplete")
	}
	return r.Digest.Validate()
}

// BypassTargetExactRefV1 and BypassCaseExactRefV1 are deliberately nominal.
// A Target ref cannot be used as a Case ref even when their scalar values
// happen to match.
type BypassTargetExactRefV1 struct {
	TenantID core.TenantID `json:"tenant_id"`
	ID       string        `json:"id"`
	Revision core.Revision `json:"revision"`
	Digest   core.Digest   `json:"digest"`
}

func (r BypassTargetExactRefV1) Validate() error {
	if blank(string(r.TenantID)) || invalidID(r.ID) || r.Revision == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "bypass Target exact ref is incomplete")
	}
	return r.Digest.Validate()
}

type BypassCaseExactRefV1 struct {
	TenantID core.TenantID `json:"tenant_id"`
	ID       string        `json:"id"`
	Revision core.Revision `json:"revision"`
	Digest   core.Digest   `json:"digest"`
}

// BypassExternalCurrentProofV1 is a Review-owned receipt over the exact
// immutable Policy projection that explicitly says Review is not required.
// Authority, Scope, Binding and Evidence remain independent Runtime Gateway
// conditions and are not re-signed by Review.
type BypassExternalCurrentProofV1 struct {
	Policy          runtimeports.ReviewDecisionPolicyCurrentProjectionRefV1 `json:"policy"`
	CheckedUnixNano int64                                                   `json:"checked_unix_nano"`
	ExpiresUnixNano int64                                                   `json:"expires_unix_nano"`
	Digest          core.Digest                                             `json:"digest"`
}

func (p BypassExternalCurrentProofV1) Clone() BypassExternalCurrentProofV1 {
	return p
}

func (p BypassExternalCurrentProofV1) digestValue() BypassExternalCurrentProofV1 {
	p = p.Clone()
	p.Digest = ""
	return p
}

func (p BypassExternalCurrentProofV1) Validate() error {
	if err := p.Policy.Validate(); err != nil {
		return err
	}
	if p.CheckedUnixNano <= 0 || p.CheckedUnixNano >= p.ExpiresUnixNano {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "bypass external current proof is not canonical or bounded")
	}
	digest, err := core.CanonicalJSONDigest("praxis.review.bypass", BypassDecisionContractV1, "BypassExternalCurrentProofV1", p.digestValue())
	if err != nil || digest != p.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "bypass external current proof digest drifted")
	}
	return nil
}

func (p BypassExternalCurrentProofV1) ValidateCurrent(now time.Time) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if now.IsZero() || now.UnixNano() < p.CheckedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "bypass external current proof clock regressed")
	}
	if now.UnixNano() >= p.ExpiresUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "bypass external current proof expired")
	}
	return nil
}

func SealBypassExternalCurrentProofV1(p BypassExternalCurrentProofV1) (BypassExternalCurrentProofV1, error) {
	p = p.Clone()
	p.Digest = ""
	digest, err := core.CanonicalJSONDigest("praxis.review.bypass", BypassDecisionContractV1, "BypassExternalCurrentProofV1", p.digestValue())
	if err != nil {
		return BypassExternalCurrentProofV1{}, err
	}
	p.Digest = digest
	return p, p.Validate()
}

func (r BypassCaseExactRefV1) Validate() error {
	if blank(string(r.TenantID)) || invalidID(r.ID) || r.Revision == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "bypass Case exact ref is incomplete")
	}
	return r.Digest.Validate()
}

// BypassDecisionV1 records that the current Policy explicitly made blocking
// Review unnecessary. It is neither a Verdict nor an Attestation and grants no
// dispatch, commit, permit, begin or provider authority.
type BypassDecisionV1 struct {
	FactIdentityV1
	Target                  BypassTargetExactRefV1                                  `json:"target"`
	Case                    BypassCaseExactRefV1                                    `json:"case"`
	IntentID                core.EffectIntentID                                     `json:"intent_id,omitempty"`
	IntentRevision          core.Revision                                           `json:"intent_revision,omitempty"`
	SubjectDigest           core.Digest                                             `json:"subject_digest,omitempty"`
	PayloadRevision         core.Revision                                           `json:"payload_revision"`
	PayloadDigest           core.Digest                                             `json:"payload_digest"`
	Scope                   core.ExecutionScope                                     `json:"scope"`
	RunID                   core.AgentRunID                                         `json:"run_id"`
	ActionScopeDigest       core.Digest                                             `json:"action_scope_digest"`
	Policy                  runtimeports.ReviewPolicyBindingRefV2                   `json:"policy"`
	PolicyCurrentProjection runtimeports.ReviewDecisionPolicyCurrentProjectionRefV1 `json:"policy_current_projection"`
	PolicyDecisionRef       string                                                  `json:"policy_decision_ref"`
	ActorAuthority          runtimeports.AuthorityBindingRefV2                      `json:"actor_authority"`
	CurrentScope            runtimeports.ExecutionScopeBindingRefV2                 `json:"current_scope"`
	TargetEvidenceSetDigest core.Digest                                             `json:"target_evidence_set_digest"`
	Profile                 ProfileV1                                               `json:"profile"`
	Risk                    RiskLevelV1                                             `json:"risk"`
	EffectClass             EffectClassV1                                           `json:"effect_class"`
	Environment             EnvironmentV1                                           `json:"environment"`
	RouteDecisionDigest     core.Digest                                             `json:"route_decision_digest"`
	ExternalProof           BypassExternalCurrentProofV1                            `json:"external_current_proof"`
	State                   BypassDecisionStateV1                                   `json:"state"`
	ExpiresUnixNano         int64                                                   `json:"expires_unix_nano"`
	InvalidationReason      core.ReasonCode                                         `json:"invalidation_reason,omitempty"`
}

func (v BypassDecisionV1) digestValue() BypassDecisionV1 {
	v.Digest = ""
	return v
}

func (v BypassDecisionV1) validateShape() error {
	if v.ContractVersion != BypassDecisionContractV1 || blank(string(v.TenantID)) || invalidID(v.ID) || v.Revision == 0 || v.CreatedUnixNano <= 0 || v.UpdatedUnixNano < v.CreatedUnixNano {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "bypass contract version is unsupported")
	}
	for _, err := range []error{
		v.Target.Validate(), v.Case.Validate(), v.PayloadDigest.Validate(), v.Scope.Validate(),
		v.ActionScopeDigest.Validate(), v.Policy.Validate(), v.PolicyCurrentProjection.Validate(),
		v.ActorAuthority.Validate(), v.CurrentScope.Validate(),
		v.TargetEvidenceSetDigest.Validate(), v.RouteDecisionDigest.Validate(), v.ExternalProof.Validate(), ValidateProfileV1(v.Profile),
	} {
		if err != nil {
			return err
		}
	}
	if v.TenantID != v.Target.TenantID || v.TenantID != v.Case.TenantID || v.PayloadRevision == 0 || blank(string(v.RunID)) || invalidID(v.PolicyDecisionRef) {
		return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "bypass Target, Case, payload or Policy decision binding drifted")
	}
	if v.PolicyCurrentProjection != v.ExternalProof.Policy || v.ExpiresUnixNano > v.ExternalProof.ExpiresUnixNano {
		return core.NewError(core.ErrorConflict, core.ReasonReviewVerdictStale, "bypass external current proof or minimum TTL drifted")
	}
	if (v.IntentID == "") != (v.IntentRevision == 0) || (v.IntentID == "") != (v.SubjectDigest == "") {
		return core.NewError(core.ErrorConflict, core.ReasonEffectIntentMissing, "bypass Effect Intent binding is partial")
	}
	if v.SubjectDigest != "" && v.SubjectDigest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidDigest, "bypass subject digest is invalid")
	}
	switch v.Risk {
	case RiskLowV1, RiskMediumV1, RiskHighV1, RiskCriticalV1:
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonUnknownGovernanceCategory, "bypass risk is unsupported")
	}
	switch v.EffectClass {
	case EffectObserveOnlyV1, EffectReversibleV1, EffectPersistentV1, EffectIrreversibleV1:
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonUnknownGovernanceCategory, "bypass effect class is unsupported")
	}
	switch v.Environment {
	case EnvironmentDevelopmentV1, EnvironmentTestV1, EnvironmentStagingV1, EnvironmentProductionV1:
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonUnknownGovernanceCategory, "bypass environment is unsupported")
	}
	if err := ValidateExpires(v.CreatedUnixNano, v.ExpiresUnixNano); err != nil {
		return err
	}
	switch v.State {
	case BypassDecisionActiveV1:
		if v.InvalidationReason != "" || v.UpdatedUnixNano >= v.ExpiresUnixNano {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "active bypass decision is invalid or expired")
		}
	case BypassDecisionRevokedV1, BypassDecisionSupersededV1:
		if v.InvalidationReason == "" {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "invalidated bypass decision requires a reason")
		}
	case BypassDecisionExpiredV1:
		if v.InvalidationReason != core.ReasonReviewVerdictStale || v.UpdatedUnixNano < v.ExpiresUnixNano {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "expired bypass decision has not crossed its TTL")
		}
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "bypass decision state is unsupported")
	}
	return nil
}

func (v BypassDecisionV1) DigestV1() (core.Digest, error) {
	return core.CanonicalJSONDigest("praxis.review.bypass", BypassDecisionContractV1, "BypassDecisionV1", v.digestValue())
}

func SealBypassDecisionV1(v BypassDecisionV1) (BypassDecisionV1, error) {
	v.ContractVersion = BypassDecisionContractV1
	v.Digest = ""
	if err := v.validateShape(); err != nil {
		return BypassDecisionV1{}, err
	}
	digest, err := v.DigestV1()
	if err != nil {
		return BypassDecisionV1{}, err
	}
	v.Digest = digest
	return v, v.Validate()
}

func (v BypassDecisionV1) Validate() error {
	if err := v.validateShape(); err != nil {
		return err
	}
	digest, err := v.DigestV1()
	if err != nil || digest != v.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "bypass decision digest drifted")
	}
	return nil
}

func (v BypassDecisionV1) ValidateCurrent(expectedTarget BypassTargetExactRefV1, expectedCase BypassCaseExactRefV1, expectedPolicy runtimeports.ReviewDecisionPolicyCurrentProjectionRefV1, now time.Time) error {
	if err := v.Validate(); err != nil {
		return err
	}
	if v.State != BypassDecisionActiveV1 || v.Target != expectedTarget || v.Case != expectedCase || v.PolicyCurrentProjection != expectedPolicy {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "bypass decision is inactive or its exact current inputs drifted")
	}
	return ValidateNow(now, v.CreatedUnixNano, v.ExpiresUnixNano)
}

func (v BypassDecisionV1) ExactRef() BypassDecisionExactRefV1 {
	return BypassDecisionExactRefV1{TenantID: v.TenantID, ID: v.ID, Revision: v.Revision, Digest: v.Digest}
}

func (t TargetSnapshotV1) BypassExactRefV1() BypassTargetExactRefV1 {
	return BypassTargetExactRefV1{TenantID: t.TenantID, ID: t.ID, Revision: t.Revision, Digest: t.Digest}
}

func (c ReviewCaseV1) BypassExactRefV1() BypassCaseExactRefV1 {
	return BypassCaseExactRefV1{TenantID: c.TenantID, ID: c.ID, Revision: c.Revision, Digest: c.Digest}
}
