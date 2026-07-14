package ports

import (
	"context"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type AuthorityFactStateV2 string

const (
	AuthorityFactActive  AuthorityFactStateV2 = "active"
	AuthorityFactRevoked AuthorityFactStateV2 = "revoked"
	AuthorityFactExpired AuthorityFactStateV2 = "expired"
)

type DispatchAuthorityFactV2 struct {
	Ref               string               `json:"ref"`
	Digest            core.Digest          `json:"digest"`
	Revision          core.Revision        `json:"revision"`
	Scope             core.ExecutionScope  `json:"scope"`
	ActionScopeDigest core.Digest          `json:"action_scope_digest"`
	State             AuthorityFactStateV2 `json:"state"`
	ExpiresUnixNano   int64                `json:"expires_unix_nano"`
}

type ReviewDecisionV2 string

const (
	ReviewDecisionAccepted             ReviewDecisionV2 = "accepted"
	ReviewDecisionConditional          ReviewDecisionV2 = "conditional"
	ReviewDecisionOperationNotRequired ReviewDecisionV2 = "operation_not_required"
	ReviewDecisionRejected             ReviewDecisionV2 = "rejected"
	ReviewDecisionRevoked              ReviewDecisionV2 = "revoked"
	ReviewDecisionExpired              ReviewDecisionV2 = "expired"
)

// DispatchReviewFactV2 is the narrow current-fact projection consumed by the
// gateway. P0.3 owns the richer Review state machine and maps it to this view.
type DispatchReviewFactV2 struct {
	Ref                     string              `json:"ref"`
	Digest                  core.Digest         `json:"digest"`
	Revision                core.Revision       `json:"revision"`
	IntentID                core.EffectIntentID `json:"intent_id"`
	IntentRevision          core.Revision       `json:"intent_revision"`
	SubjectDigest           core.Digest         `json:"subject_digest"`
	CandidateDigest         core.Digest         `json:"candidate_digest"`
	VerdictDigest           core.Digest         `json:"verdict_digest"`
	VerdictRevision         core.Revision       `json:"verdict_revision"`
	PayloadDigest           core.Digest         `json:"payload_digest"`
	PayloadRevision         core.Revision       `json:"payload_revision"`
	ScopeDigest             core.Digest         `json:"scope_digest"`
	PolicyDigest            core.Digest         `json:"policy_digest"`
	PolicyDecisionRef       string              `json:"policy_decision_ref"`
	ActorAuthorityDigest    core.Digest         `json:"actor_authority_digest"`
	ReviewerAuthorityDigest core.Digest         `json:"reviewer_authority_digest"`
	ConditionsDigest        core.Digest         `json:"conditions_digest,omitempty"`
	SatisfactionRef         string              `json:"satisfaction_ref,omitempty"`
	SatisfactionDigest      core.Digest         `json:"satisfaction_digest,omitempty"`
	SatisfactionRevision    core.Revision       `json:"satisfaction_revision,omitempty"`
	EvidenceDigest          core.Digest         `json:"evidence_digest"`
	Decision                ReviewDecisionV2    `json:"decision"`
	ExpiresUnixNano         int64               `json:"expires_unix_nano"`
}

type CredentialLeaseFactV2 struct {
	Ref             string      `json:"ref"`
	Class           string      `json:"class"`
	ScopeDigest     core.Digest `json:"scope_digest"`
	Epoch           core.Epoch  `json:"epoch"`
	Active          bool        `json:"active"`
	ExpiresUnixNano int64       `json:"expires_unix_nano"`
}

func DigestCredentialLeaseFactsV2(facts []CredentialLeaseFactV2) (core.Digest, error) {
	copy := append([]CredentialLeaseFactV2{}, facts...)
	for index, fact := range copy {
		if err := fact.ScopeDigest.Validate(); err != nil {
			return "", err
		}
		if index > 0 && (copy[index-1].Class > fact.Class || copy[index-1].Class == fact.Class && copy[index-1].Ref >= fact.Ref) {
			return "", core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "credential fact set must be sorted and unique")
		}
	}
	return core.CanonicalJSONDigest("praxis.runtime.effect", EffectContractVersionV2, "CredentialLeaseFactSetV2", copy)
}

type DispatchPolicyFactV2 struct {
	Ref               string              `json:"ref"`
	Digest            core.Digest         `json:"digest"`
	Revision          core.Revision       `json:"revision"`
	IntentID          core.EffectIntentID `json:"intent_id"`
	IntentRevision    core.Revision       `json:"intent_revision"`
	IntentDigest      core.Digest         `json:"intent_digest"`
	Scope             core.ExecutionScope `json:"scope"`
	EffectKind        EffectKindV2        `json:"effect_kind"`
	RiskClass         NamespacedNameV2    `json:"risk_class"`
	ActionScopeDigest core.Digest         `json:"action_scope_digest"`
	MaximumPermitTTL  time.Duration       `json:"maximum_permit_ttl"`
	Active            bool                `json:"active"`
	ExpiresUnixNano   int64               `json:"expires_unix_nano"`
}

type ExecutionScopeFactStateV2 string

const (
	ExecutionScopeFactActive  ExecutionScopeFactStateV2 = "active"
	ExecutionScopeFactRevoked ExecutionScopeFactStateV2 = "revoked"
	ExecutionScopeFactExpired ExecutionScopeFactStateV2 = "expired"
)

type ExecutionScopeCurrentFactV2 struct {
	Ref                   string                     `json:"ref"`
	Digest                core.Digest                `json:"digest"`
	Revision              core.Revision              `json:"revision"`
	Scope                 core.ExecutionScope        `json:"scope"`
	CapabilityGrantDigest core.Digest                `json:"capability_grant_digest"`
	ActivationSource      GovernanceSourceFactRefV2  `json:"activation_source"`
	InstanceSource        GovernanceSourceFactRefV2  `json:"instance_source"`
	SandboxSource         *GovernanceSourceFactRefV2 `json:"sandbox_source,omitempty"`
	AuthoritySource       GovernanceSourceFactRefV2  `json:"authority_source"`
	BindingSource         GovernanceSourceFactRefV2  `json:"binding_source"`
	RunSource             GovernanceSourceFactRefV2  `json:"run_source"`
	ActiveRunID           core.AgentRunID            `json:"active_run_id"`
	RunState              string                     `json:"run_state"`
	ProjectionWatermark   core.Revision              `json:"projection_watermark"`
	State                 ExecutionScopeFactStateV2  `json:"state"`
	ExpiresUnixNano       int64                      `json:"expires_unix_nano"`
}

type GovernanceSourceFactRefV2 struct {
	Ref      string        `json:"ref"`
	Revision core.Revision `json:"revision"`
	Digest   core.Digest   `json:"digest"`
}

func (r GovernanceSourceFactRefV2) Validate() error {
	if strings.TrimSpace(r.Ref) == "" || r.Revision == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "projection source fact ref and revision are required")
	}
	return r.Digest.Validate()
}

func (f DispatchPolicyFactV2) DigestV2() (core.Digest, error) {
	copy := f
	copy.Digest = ""
	return core.CanonicalJSONDigest("praxis.runtime.effect", EffectContractVersionV2, "DispatchPolicyFactV2", copy)
}

type AuthorityFactReaderV2 interface {
	InspectDispatchAuthority(context.Context, string) (DispatchAuthorityFactV2, error)
}

type ReviewFactReaderV2 interface {
	InspectDispatchReview(context.Context, string) (DispatchReviewFactV2, error)
}

type CredentialLeaseFactReaderV2 interface {
	InspectCredentialLease(context.Context, string) (CredentialLeaseFactV2, error)
}

type DispatchPolicyFactReaderV2 interface {
	InspectDispatchPolicy(context.Context, string) (DispatchPolicyFactV2, error)
}

type ExecutionScopeFactReaderV2 interface {
	InspectCurrentExecutionScope(context.Context, string) (ExecutionScopeCurrentFactV2, error)
}

type PermitVerificationRequestV2 struct {
	Permit             DispatchPermitV2       `json:"permit"`
	PermitFactRevision core.Revision          `json:"permit_fact_revision"`
	PermitFactState    string                 `json:"permit_fact_state"`
	Intent             EffectIntentV2         `json:"intent"`
	Fence              core.ExecutionFence    `json:"fence"`
	Current            DispatchCurrentFactsV2 `json:"current"`
}

// PermitVerifierPortV2 is implemented at the actual local or remote execution
// point. Attestation/signature algorithms and transport remain adapter choices.
type PermitVerifierPortV2 interface {
	VerifyDispatchPermit(context.Context, PermitVerificationRequestV2) (EnforcementReceiptV2, error)
}

func (r PermitVerificationRequestV2) Validate(now time.Time) error {
	if r.PermitFactRevision < 2 || r.PermitFactState != "begun" {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonDispatchPermitConsumed, "execution point requires the single begun permit fact")
	}
	return ValidateDispatchAtExecutionPointV2(r.Permit, r.Intent, r.Fence, r.Current, now)
}

func (f DispatchAuthorityFactV2) ValidateCurrent(expected AuthorityBindingRefV2, scope core.ExecutionScope, actionScopeDigest core.Digest, now time.Time) error {
	if strings.TrimSpace(f.Ref) == "" || f.Revision == 0 || f.ExpiresUnixNano <= 0 || f.State != AuthorityFactActive {
		return core.NewError(core.ErrorForbidden, core.ReasonEffectAuthorizationMissing, "current active authority fact is required")
	}
	if err := f.Scope.Validate(); err != nil {
		return err
	}
	for _, digest := range []core.Digest{f.Digest, f.ActionScopeDigest} {
		if err := digest.Validate(); err != nil {
			return err
		}
	}
	if now.IsZero() || !now.Before(time.Unix(0, f.ExpiresUnixNano)) || f.Ref != expected.Ref || f.Digest != expected.Digest || f.Revision != expected.Revision || f.Scope.AuthorityEpoch != expected.Epoch || f.ActionScopeDigest != actionScopeDigest || !SameExecutionScopeV2(f.Scope, scope) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonStaleAuthorityEpoch, "authority fact expired or drifted from effect scope")
	}
	return nil
}

func (f DispatchReviewFactV2) ValidateCurrent(expected ReviewBindingRefV2, intent EffectIntentV2, now time.Time) error {
	if strings.TrimSpace(f.Ref) == "" || strings.TrimSpace(f.PolicyDecisionRef) == "" || f.Revision == 0 || f.IntentRevision == 0 || f.PayloadRevision == 0 || f.ExpiresUnixNano <= 0 {
		return core.NewError(core.ErrorForbidden, core.ReasonReviewVerdictMissing, "current review fact is incomplete")
	}
	for _, digest := range []core.Digest{f.Digest, f.SubjectDigest, f.CandidateDigest, f.VerdictDigest, f.PayloadDigest, f.ScopeDigest, f.PolicyDigest, f.ActorAuthorityDigest, f.ReviewerAuthorityDigest, f.EvidenceDigest} {
		if err := digest.Validate(); err != nil {
			return err
		}
	}
	if f.Decision == ReviewDecisionConditional {
		if f.ConditionsDigest.Validate() != nil || strings.TrimSpace(f.SatisfactionRef) == "" || f.SatisfactionRevision == 0 || f.SatisfactionDigest.Validate() != nil {
			return core.NewError(core.ErrorForbidden, core.ReasonReviewConditionUnsatisfied, "conditional review requires an exact current satisfaction fact")
		}
	} else if f.ConditionsDigest != "" || f.SatisfactionRef != "" || f.SatisfactionRevision != 0 || f.SatisfactionDigest != "" {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewCandidateConflict, "unconditional review cannot carry condition satisfaction")
	}
	if f.Decision != ReviewDecisionAccepted && f.Decision != ReviewDecisionOperationNotRequired && f.Decision != ReviewDecisionConditional {
		return core.NewError(core.ErrorForbidden, core.ReasonReviewVerdictStale, "review decision does not allow dispatch")
	}
	subjectDigest, err := intent.ReviewSubjectDigestV2()
	if err != nil {
		return err
	}
	if f.VerdictRevision == 0 || now.IsZero() || !now.Before(time.Unix(0, f.ExpiresUnixNano)) || f.Ref != expected.Ref || f.Digest != expected.Digest || f.Revision != expected.Revision || f.CandidateDigest != expected.Digest || f.PolicyDigest != expected.PolicyDigest || f.IntentID != intent.ID || f.IntentRevision != intent.Revision || f.SubjectDigest != subjectDigest || f.PayloadDigest != intent.Payload.ContentDigest || f.PayloadRevision != intent.PayloadRevision || f.ScopeDigest != intent.ActionScopeDigest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "review fact expired or drifted from exact effect revision")
	}
	return nil
}

func (f CredentialLeaseFactV2) ValidateCurrent(expected CredentialLeaseRefV2, now time.Time) error {
	if strings.TrimSpace(f.Ref) == "" || strings.TrimSpace(f.Class) == "" || f.Epoch == 0 || f.ExpiresUnixNano <= 0 || !f.Active {
		return core.NewError(core.ErrorForbidden, core.ReasonCredentialLeaseMissing, "active credential lease fact is required")
	}
	if err := f.ScopeDigest.Validate(); err != nil {
		return err
	}
	if now.IsZero() || !now.Before(time.Unix(0, f.ExpiresUnixNano)) || f.Ref != expected.Ref || f.Class != expected.Class || f.ScopeDigest != expected.ScopeDigest || f.Epoch != expected.Epoch {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonCredentialLeaseMissing, "credential lease expired or drifted")
	}
	return nil
}

func (f DispatchPolicyFactV2) ValidateCurrent(expected DispatchPolicyBindingRefV2, intent EffectIntentV2, requestedTTL time.Duration, now time.Time) error {
	if strings.TrimSpace(f.Ref) == "" || f.Revision == 0 || strings.TrimSpace(string(f.IntentID)) == "" || f.IntentRevision == 0 || f.ExpiresUnixNano <= 0 || f.MaximumPermitTTL <= 0 || f.MaximumPermitTTL > MaxDispatchPermitTTL || !f.Active {
		return core.NewError(core.ErrorForbidden, core.ReasonEffectAuthorizationMissing, "active bounded dispatch policy fact is required")
	}
	if err := f.Scope.Validate(); err != nil {
		return err
	}
	if err := ValidateNamespacedNameV2(NamespacedNameV2(f.EffectKind)); err != nil {
		return err
	}
	if err := ValidateNamespacedNameV2(f.RiskClass); err != nil {
		return err
	}
	for _, digest := range []core.Digest{f.Digest, f.IntentDigest, f.ActionScopeDigest} {
		if err := digest.Validate(); err != nil {
			return err
		}
	}
	policyDigest, err := f.DigestV2()
	if err != nil {
		return err
	}
	candidateDigest, err := intent.PolicyCandidateDigestV2()
	if err != nil {
		return err
	}
	if now.IsZero() || requestedTTL <= 0 || requestedTTL > f.MaximumPermitTTL || !now.Before(time.Unix(0, f.ExpiresUnixNano)) || f.Ref != expected.Ref || f.Digest != policyDigest || f.Digest != expected.Digest || f.Revision != expected.Revision || f.IntentID != intent.ID || f.IntentRevision != intent.Revision || f.IntentDigest != candidateDigest || !SameExecutionScopeV2(f.Scope, intent.Scope) || f.EffectKind != intent.Kind || f.RiskClass != intent.RiskClass || f.ActionScopeDigest != intent.ActionScopeDigest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectAuthorizationMissing, "dispatch policy fact expired or drifted from exact effect and requested permit TTL")
	}
	return nil
}

func (f ExecutionScopeCurrentFactV2) DigestV2() (core.Digest, error) {
	copy := f
	copy.Digest = ""
	return core.CanonicalJSONDigest("praxis.runtime.effect", EffectContractVersionV2, "ExecutionScopeCurrentFactV2", copy)
}

func (f ExecutionScopeCurrentFactV2) BindingRefV2() (ExecutionScopeBindingRefV2, error) {
	digest, err := f.DigestV2()
	if err != nil {
		return ExecutionScopeBindingRefV2{}, err
	}
	return ExecutionScopeBindingRefV2{Ref: f.Ref, Digest: digest, Revision: f.Revision}, nil
}

func (f ExecutionScopeCurrentFactV2) ValidateCurrent(expected ExecutionScopeBindingRefV2, intent EffectIntentV2, capabilityDigest core.Digest, now time.Time) error {
	if strings.TrimSpace(f.Ref) == "" || f.Revision == 0 || f.ExpiresUnixNano <= 0 || f.State != ExecutionScopeFactActive || f.ProjectionWatermark == 0 || f.ActiveRunID != intent.RunID || f.RunState != "running" {
		return core.NewError(core.ErrorForbidden, core.ReasonEffectFenceStale, "active current execution scope fact is required")
	}
	if err := f.Scope.Validate(); err != nil {
		return err
	}
	if err := f.CapabilityGrantDigest.Validate(); err != nil {
		return err
	}
	for _, source := range []GovernanceSourceFactRefV2{f.ActivationSource, f.InstanceSource, f.AuthoritySource, f.BindingSource, f.RunSource} {
		if err := source.Validate(); err != nil {
			return err
		}
	}
	if f.Scope.SandboxLease == nil {
		if f.SandboxSource != nil {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectFenceStale, "sandbox source cannot exist without a current sandbox lease")
		}
	} else if f.SandboxSource == nil {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectFenceStale, "sandbox lease requires its authoritative source fact")
	} else if err := f.SandboxSource.Validate(); err != nil {
		return err
	}
	ref, err := f.BindingRefV2()
	if err != nil {
		return err
	}
	if f.Digest != ref.Digest || ref != expected || now.IsZero() || !now.Before(time.Unix(0, f.ExpiresUnixNano)) || capabilityDigest != f.CapabilityGrantDigest || !SameExecutionScopeV2(f.Scope, intent.Scope) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectFenceStale, "current instance, sandbox, authority or capability scope drifted")
	}
	return nil
}
