package ports

import (
	"context"
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const (
	ReviewContractVersionV2 = "2.0.0"
	MaxReviewEvidenceV2     = 128
	MaxReviewConditionsV2   = 64
)

type ReviewInvocationModeV2 string

const (
	ReviewInvocationHuman           ReviewInvocationModeV2 = "human"
	ReviewInvocationAutomaticLocal  ReviewInvocationModeV2 = "automatic_local"
	ReviewInvocationAutomaticRemote ReviewInvocationModeV2 = "automatic_remote"
)

type ReviewDecisionBasisV2 string

const (
	ReviewBasisHuman             ReviewDecisionBasisV2 = "human"
	ReviewBasisAutomatic         ReviewDecisionBasisV2 = "automatic"
	ReviewBasisPolicyNotRequired ReviewDecisionBasisV2 = "policy_not_required"
)

type ReviewCaseStateV2 string

const (
	ReviewCasePending ReviewCaseStateV2 = "pending"
	ReviewCaseDecided ReviewCaseStateV2 = "decided"
	ReviewCaseExpired ReviewCaseStateV2 = "expired"
	ReviewCaseRevoked ReviewCaseStateV2 = "revoked"
)

type ReviewVerdictStateV2 string

const (
	ReviewVerdictAccepted    ReviewVerdictStateV2 = "accepted"
	ReviewVerdictRejected    ReviewVerdictStateV2 = "rejected"
	ReviewVerdictConditional ReviewVerdictStateV2 = "conditional"
	ReviewVerdictExpired     ReviewVerdictStateV2 = "expired"
	ReviewVerdictRevoked     ReviewVerdictStateV2 = "revoked"
)

type ConditionSatisfactionStateV2 string

const (
	ConditionSatisfactionPending ConditionSatisfactionStateV2 = "pending"
	ConditionSatisfied           ConditionSatisfactionStateV2 = "satisfied"
	ConditionSatisfactionExpired ConditionSatisfactionStateV2 = "expired"
	ConditionSatisfactionRevoked ConditionSatisfactionStateV2 = "revoked"
)

type ReviewPolicyBindingRefV2 struct {
	Ref      string        `json:"ref"`
	Digest   core.Digest   `json:"digest"`
	Revision core.Revision `json:"revision"`
}

type ReviewPolicyFactV2 struct {
	Ref                  string                     `json:"ref"`
	Digest               core.Digest                `json:"digest"`
	Revision             core.Revision              `json:"revision"`
	SubjectDigest        core.Digest                `json:"subject_digest"`
	Scope                core.ExecutionScope        `json:"scope"`
	RunID                core.AgentRunID            `json:"run_id"`
	CurrentScope         ExecutionScopeBindingRefV2 `json:"current_scope"`
	RiskClass            NamespacedNameV2           `json:"risk_class"`
	ActorAuthorityRef    string                     `json:"actor_authority_ref"`
	ReviewerAuthorityRef string                     `json:"reviewer_authority_ref"`
	AllowSelfReview      bool                       `json:"allow_self_review"`
	OperationNotRequired bool                       `json:"operation_not_required"`
	PolicyDecisionRef    string                     `json:"policy_decision_ref"`
	Active               bool                       `json:"active"`
	ExpiresUnixNano      int64                      `json:"expires_unix_nano"`
}

type ReviewEvidenceRefV2 struct {
	Ref            string           `json:"ref"`
	Classification NamespacedNameV2 `json:"classification"`
	Digest         core.Digest      `json:"digest"`
}

type ReviewInvocationEffectRefV2 struct {
	EffectID       core.EffectIntentID  `json:"effect_id"`
	EffectRevision core.Revision        `json:"effect_revision"`
	EffectKind     EffectKindV2         `json:"effect_kind"`
	PayloadDigest  core.Digest          `json:"payload_digest"`
	Provider       ProviderBindingRefV2 `json:"provider"`
}

type ReviewComponentBindingRefV2 struct {
	BindingSetID       string           `json:"binding_set_id"`
	BindingSetRevision core.Revision    `json:"binding_set_revision"`
	ComponentID        ComponentIDV2    `json:"component_id"`
	ManifestDigest     core.Digest      `json:"manifest_digest"`
	ArtifactDigest     core.Digest      `json:"artifact_digest"`
	Capability         CapabilityNameV2 `json:"capability"`
}

type ReviewConditionV2 struct {
	ID                NamespacedNameV2            `json:"id"`
	Revision          core.Revision               `json:"revision"`
	Schema            SchemaRefV2                 `json:"schema"`
	ConstraintDigest  core.Digest                 `json:"constraint_digest"`
	SatisfactionOwner ReviewComponentBindingRefV2 `json:"satisfaction_owner"`
	ScopeDigest       core.Digest                 `json:"scope_digest"`
	Authority         AuthorityBindingRefV2       `json:"authority"`
	ExpiresUnixNano   int64                       `json:"expires_unix_nano"`
}

type ReviewConditionProofV2 struct {
	ConditionID       NamespacedNameV2            `json:"condition_id"`
	ConditionRevision core.Revision               `json:"condition_revision"`
	ConstraintDigest  core.Digest                 `json:"constraint_digest"`
	Owner             ReviewComponentBindingRefV2 `json:"owner"`
	ScopeDigest       core.Digest                 `json:"scope_digest"`
	Authority         AuthorityBindingRefV2       `json:"authority"`
	Evidence          ReviewEvidenceRefV2         `json:"evidence"`
	ExpiresUnixNano   int64                       `json:"expires_unix_nano"`
}

// ReviewAttestationObservationV2 is reviewer evidence only. It cannot be
// passed to the dispatch gateway and becomes a verdict only through the
// Review Owner's governed DecideReview CAS.
type ReviewAttestationObservationV2 struct {
	CaseID            string                      `json:"case_id"`
	CandidateDigest   core.Digest                 `json:"candidate_digest"`
	ReviewerBinding   ReviewComponentBindingRefV2 `json:"reviewer_binding"`
	ReviewerAuthority AuthorityBindingRefV2       `json:"reviewer_authority"`
	ProposedState     ReviewVerdictStateV2        `json:"proposed_state"`
	Basis             ReviewDecisionBasisV2       `json:"basis"`
	Evidence          []ReviewEvidenceRefV2       `json:"evidence"`
	Conditions        []ReviewConditionV2         `json:"conditions"`
	ObservedUnixNano  int64                       `json:"observed_unix_nano"`
}

// ReviewCandidateV2 is a governed case subject. SubjectDigest is computed
// with EffectIntent.Review.Digest blank, so the intent can bind this case
// without a circular digest dependency.
type ReviewCandidateV2 struct {
	ContractVersion    string                       `json:"contract_version"`
	ID                 string                       `json:"review_id"`
	Revision           core.Revision                `json:"candidate_revision"`
	IntentID           core.EffectIntentID          `json:"intent_id"`
	IntentRevision     core.Revision                `json:"intent_revision"`
	SubjectDigest      core.Digest                  `json:"subject_digest"`
	CandidateKind      NamespacedNameV2             `json:"candidate_kind"`
	RiskClass          NamespacedNameV2             `json:"risk_class"`
	PayloadSchema      SchemaRefV2                  `json:"payload_schema"`
	PayloadDigest      core.Digest                  `json:"payload_digest"`
	PayloadRevision    core.Revision                `json:"payload_revision"`
	Scope              core.ExecutionScope          `json:"scope"`
	RunID              core.AgentRunID              `json:"run_id"`
	ActionScopeDigest  core.Digest                  `json:"action_scope_digest"`
	SubjectProvider    ProviderBindingRefV2         `json:"subject_provider_binding"`
	ReviewOwnerBinding ReviewComponentBindingRefV2  `json:"review_owner_binding"`
	ReviewerBinding    ReviewComponentBindingRefV2  `json:"reviewer_binding"`
	CurrentScope       ExecutionScopeBindingRefV2   `json:"execution_scope_watermark"`
	ActorAuthority     AuthorityBindingRefV2        `json:"actor_authority"`
	ReviewerAuthority  AuthorityBindingRefV2        `json:"reviewer_authority"`
	Policy             ReviewPolicyBindingRefV2     `json:"policy"`
	Evidence           []ReviewEvidenceRefV2        `json:"evidence"`
	InvocationMode     ReviewInvocationModeV2       `json:"invocation_mode"`
	InvocationEffect   *ReviewInvocationEffectRefV2 `json:"invocation_effect,omitempty"`
	RequestedUnixNano  int64                        `json:"requested_unix_nano"`
	ExpiresUnixNano    int64                        `json:"expires_unix_nano"`
}

type ReviewCaseFactV2 struct {
	Candidate          ReviewCandidateV2 `json:"candidate"`
	CandidateDigest    core.Digest       `json:"candidate_digest"`
	State              ReviewCaseStateV2 `json:"state"`
	Revision           core.Revision     `json:"revision"`
	VerdictID          string            `json:"verdict_id,omitempty"`
	VerdictRevision    core.Revision     `json:"verdict_revision,omitempty"`
	VerdictDigest      core.Digest       `json:"verdict_digest,omitempty"`
	UpdatedUnixNano    int64             `json:"updated_unix_nano"`
	InvalidationReason core.ReasonCode   `json:"invalidation_reason,omitempty"`
}

type ReviewVerdictFactV2 struct {
	ID                         string                       `json:"verdict_id"`
	CaseID                     string                       `json:"case_id"`
	CaseRevision               core.Revision                `json:"case_revision"`
	CandidateDigest            core.Digest                  `json:"candidate_digest"`
	IntentID                   core.EffectIntentID          `json:"intent_id"`
	IntentRevision             core.Revision                `json:"intent_revision"`
	SubjectDigest              core.Digest                  `json:"subject_digest"`
	Policy                     ReviewPolicyBindingRefV2     `json:"policy"`
	ActorAuthority             AuthorityBindingRefV2        `json:"actor_authority"`
	ReviewerAuthority          AuthorityBindingRefV2        `json:"reviewer_authority"`
	State                      ReviewVerdictStateV2         `json:"state"`
	Basis                      ReviewDecisionBasisV2        `json:"decision_basis"`
	PolicyDecisionRef          string                       `json:"policy_decision_ref"`
	DecisionEvidence           []ReviewEvidenceRefV2        `json:"decision_evidence"`
	DecisionEvidenceDigest     core.Digest                  `json:"decision_evidence_digest"`
	Conditions                 []ReviewConditionV2          `json:"conditions"`
	ConditionsDigest           core.Digest                  `json:"conditions_digest,omitempty"`
	InvocationEffect           *ReviewInvocationEffectRefV2 `json:"invocation_effect,omitempty"`
	InvocationSettlementDigest core.Digest                  `json:"invocation_settlement_digest,omitempty"`
	Revision                   core.Revision                `json:"revision"`
	DecidedUnixNano            int64                        `json:"decided_unix_nano"`
	UpdatedUnixNano            int64                        `json:"updated_unix_nano"`
	ExpiresUnixNano            int64                        `json:"expires_unix_nano"`
	InvalidationReason         core.ReasonCode              `json:"invalidation_reason,omitempty"`
}

type ConditionSatisfactionFactV2 struct {
	ID                 string                       `json:"satisfaction_id"`
	VerdictID          string                       `json:"verdict_id"`
	VerdictRevision    core.Revision                `json:"verdict_revision"`
	VerdictDigest      core.Digest                  `json:"verdict_digest"`
	CandidateDigest    core.Digest                  `json:"candidate_digest"`
	IntentID           core.EffectIntentID          `json:"intent_id"`
	IntentRevision     core.Revision                `json:"intent_revision"`
	SubjectDigest      core.Digest                  `json:"subject_digest"`
	ConditionsDigest   core.Digest                  `json:"conditions_digest"`
	Policy             ReviewPolicyBindingRefV2     `json:"policy"`
	Scope              core.ExecutionScope          `json:"scope"`
	RunID              core.AgentRunID              `json:"run_id"`
	ActionScopeDigest  core.Digest                  `json:"action_scope_digest"`
	CurrentScope       ExecutionScopeBindingRefV2   `json:"current_scope"`
	Proofs             []ReviewConditionProofV2     `json:"proofs"`
	ProofsDigest       core.Digest                  `json:"proofs_digest,omitempty"`
	State              ConditionSatisfactionStateV2 `json:"state"`
	Revision           core.Revision                `json:"revision"`
	SatisfiedUnixNano  int64                        `json:"satisfied_unix_nano,omitempty"`
	UpdatedUnixNano    int64                        `json:"updated_unix_nano"`
	ExpiresUnixNano    int64                        `json:"expires_unix_nano"`
	InvalidationReason core.ReasonCode              `json:"invalidation_reason,omitempty"`
}

type ReviewCaseCASRequestV2 struct {
	CaseID           string           `json:"case_id"`
	ExpectedRevision core.Revision    `json:"expected_revision"`
	Next             ReviewCaseFactV2 `json:"next"`
}
type DecideReviewRequestV2 struct {
	CaseID               string              `json:"case_id"`
	ExpectedCaseRevision core.Revision       `json:"expected_case_revision"`
	Verdict              ReviewVerdictFactV2 `json:"verdict"`
}
type DecideReviewResultV2 struct {
	Case    ReviewCaseFactV2    `json:"case"`
	Verdict ReviewVerdictFactV2 `json:"verdict"`
}
type ReviewVerdictCASRequestV2 struct {
	VerdictID        string              `json:"verdict_id"`
	ExpectedRevision core.Revision       `json:"expected_revision"`
	Next             ReviewVerdictFactV2 `json:"next"`
}
type ConditionSatisfactionCASRequestV2 struct {
	SatisfactionID   string                      `json:"satisfaction_id"`
	ExpectedRevision core.Revision               `json:"expected_revision"`
	Next             ConditionSatisfactionFactV2 `json:"next"`
}

type ReviewVerdictFactPortV2 interface {
	CreateReviewCase(context.Context, ReviewCaseFactV2) (ReviewCaseFactV2, error)
	InspectReviewCase(context.Context, string) (ReviewCaseFactV2, error)
	CompareAndSwapReviewCase(context.Context, ReviewCaseCASRequestV2) (ReviewCaseFactV2, error)
	DecideReview(context.Context, DecideReviewRequestV2) (DecideReviewResultV2, error)
	InspectReviewVerdict(context.Context, string) (ReviewVerdictFactV2, error)
	CompareAndSwapReviewVerdict(context.Context, ReviewVerdictCASRequestV2) (ReviewVerdictFactV2, error)
	CreateConditionSatisfaction(context.Context, ConditionSatisfactionFactV2) (ConditionSatisfactionFactV2, error)
	InspectConditionSatisfaction(context.Context, string) (ConditionSatisfactionFactV2, error)
	InspectConditionSatisfactionByVerdict(context.Context, string) (ConditionSatisfactionFactV2, error)
	CompareAndSwapConditionSatisfaction(context.Context, ConditionSatisfactionCASRequestV2) (ConditionSatisfactionFactV2, error)
}

type ReviewPolicyFactReaderV2 interface {
	InspectReviewPolicy(context.Context, string) (ReviewPolicyFactV2, error)
}

func (i EffectIntentV2) ReviewSubjectDigestV2() (core.Digest, error) {
	copy := i
	copy.Review.Digest = ""
	// The authoritative Review Policy fact itself binds SubjectDigest, so its
	// computed digest must also be excluded. Ref and revision remain part of
	// the subject and therefore still detect policy-locator drift.
	copy.Review.PolicyDigest = ""
	// Dispatch policy is independently bound by its ref/revision. Its computed
	// fact digest is filled after the Review case identity and is excluded here
	// to prevent a Review<->DispatchPolicy digest cycle.
	copy.Policy.Digest = ""
	if copy.Owners == nil {
		copy.Owners = []EffectOwnerRefV2{}
	}
	if copy.CredentialLeases == nil {
		copy.CredentialLeases = []CredentialLeaseRefV2{}
	}
	return core.CanonicalJSONDigest("praxis.runtime.review", ReviewContractVersionV2, "EffectReviewSubjectV2", copy)
}
func (f ReviewPolicyFactV2) DigestV2() (core.Digest, error) {
	copy := f
	copy.Digest = ""
	return core.CanonicalJSONDigest("praxis.runtime.review", ReviewContractVersionV2, "ReviewPolicyFactV2", copy)
}
func (c ReviewCandidateV2) DigestV2() (core.Digest, error) {
	if err := c.Validate(); err != nil {
		return "", err
	}
	if c.Evidence == nil {
		c.Evidence = []ReviewEvidenceRefV2{}
	}
	return core.CanonicalJSONDigest("praxis.runtime.review", ReviewContractVersionV2, "ReviewCandidateV2", c)
}
func (f ReviewCaseFactV2) DigestV2() (core.Digest, error) {
	if err := f.Validate(); err != nil {
		return "", err
	}
	if f.Candidate.Evidence == nil {
		f.Candidate.Evidence = []ReviewEvidenceRefV2{}
	}
	return core.CanonicalJSONDigest("praxis.runtime.review", ReviewContractVersionV2, "ReviewCaseFactV2", f)
}
func (f ReviewVerdictFactV2) DigestV2() (core.Digest, error) {
	if err := f.Validate(); err != nil {
		return "", err
	}
	if f.DecisionEvidence == nil {
		f.DecisionEvidence = []ReviewEvidenceRefV2{}
	}
	if f.Conditions == nil {
		f.Conditions = []ReviewConditionV2{}
	}
	return core.CanonicalJSONDigest("praxis.runtime.review", ReviewContractVersionV2, "ReviewVerdictFactV2", f)
}
func (f ConditionSatisfactionFactV2) DigestV2() (core.Digest, error) {
	if err := f.Validate(); err != nil {
		return "", err
	}
	if f.Proofs == nil {
		f.Proofs = []ReviewConditionProofV2{}
	}
	return core.CanonicalJSONDigest("praxis.runtime.review", ReviewContractVersionV2, "ConditionSatisfactionFactV2", f)
}

func (c ReviewCandidateV2) Validate() error {
	if c.ContractVersion != ReviewContractVersionV2 || strings.TrimSpace(c.ID) == "" || c.Revision == 0 || strings.TrimSpace(string(c.IntentID)) == "" || c.IntentRevision == 0 || strings.TrimSpace(string(c.RunID)) == "" || c.PayloadRevision == 0 || c.RequestedUnixNano <= 0 || c.ExpiresUnixNano <= c.RequestedUnixNano {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonReviewVerdictMissing, "review candidate identity, revisions, run and bounded time are required")
	}
	if err := ValidateNamespacedNameV2(c.CandidateKind); err != nil {
		return err
	}
	if err := c.Scope.Validate(); err != nil {
		return err
	}
	if err := c.PayloadSchema.Validate(); err != nil {
		return err
	}
	if err := ValidateNamespacedNameV2(c.RiskClass); err != nil {
		return err
	}
	for _, digest := range []core.Digest{c.SubjectDigest, c.PayloadDigest, c.ActionScopeDigest} {
		if err := digest.Validate(); err != nil {
			return err
		}
	}
	if err := c.SubjectProvider.Validate(); err != nil {
		return err
	}
	if err := c.ReviewOwnerBinding.Validate(); err != nil {
		return err
	}
	if err := c.ReviewerBinding.Validate(); err != nil {
		return err
	}
	if err := c.CurrentScope.Validate(); err != nil {
		return err
	}
	if err := c.ActorAuthority.Validate(); err != nil {
		return err
	}
	if err := c.ReviewerAuthority.Validate(); err != nil {
		return err
	}
	if err := c.Policy.Validate(); err != nil {
		return err
	}
	if err := validateReviewEvidenceV2(c.Evidence); err != nil {
		return err
	}
	automatic := c.InvocationMode == ReviewInvocationAutomaticLocal || c.InvocationMode == ReviewInvocationAutomaticRemote
	if c.InvocationMode != ReviewInvocationHuman && !automatic {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonReviewVerdictMissing, "known review invocation mode is required")
	}
	if automatic != (c.InvocationEffect != nil) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewRemoteEffectRequired, "automatic reviewer must bind its independent Effect")
	}
	if c.InvocationEffect != nil {
		if err := c.InvocationEffect.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (f ReviewCaseFactV2) Validate() error {
	if err := f.Candidate.Validate(); err != nil {
		return err
	}
	digest, err := f.Candidate.DigestV2()
	if err != nil || digest != f.CandidateDigest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewCandidateConflict, "case candidate digest drifted")
	}
	if f.Revision == 0 || f.UpdatedUnixNano < f.Candidate.RequestedUnixNano || !validReviewCaseStateV2(f.State) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonReviewVerdictMissing, "case state, revision and time are required")
	}
	decided := f.State == ReviewCaseDecided
	if decided {
		if strings.TrimSpace(f.VerdictID) == "" || f.VerdictRevision == 0 || f.VerdictDigest.Validate() != nil {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictMissing, "decided case requires exact verdict fact")
		}
	} else if f.VerdictID != "" || f.VerdictRevision != 0 || f.VerdictDigest != "" {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewCandidateConflict, "non-decided case cannot bind verdict")
	}
	if (f.State == ReviewCaseExpired || f.State == ReviewCaseRevoked) != (f.InvalidationReason != "") {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "case invalidation state and reason must agree")
	}
	return nil
}

func (f ReviewVerdictFactV2) Validate() error {
	if strings.TrimSpace(f.ID) == "" || strings.TrimSpace(f.CaseID) == "" || f.CaseRevision == 0 || strings.TrimSpace(string(f.IntentID)) == "" || f.IntentRevision == 0 || f.Revision == 0 || f.DecidedUnixNano <= 0 || f.UpdatedUnixNano < f.DecidedUnixNano || f.ExpiresUnixNano <= f.DecidedUnixNano || !validReviewVerdictStateV2(f.State) || !validReviewDecisionBasisV2(f.Basis) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonReviewVerdictMissing, "verdict identity, decision and bounded time are required")
	}
	for _, digest := range []core.Digest{f.CandidateDigest, f.SubjectDigest, f.DecisionEvidenceDigest} {
		if err := digest.Validate(); err != nil {
			return err
		}
	}
	if err := f.Policy.Validate(); err != nil {
		return err
	}
	if err := f.ActorAuthority.Validate(); err != nil {
		return err
	}
	if err := f.ReviewerAuthority.Validate(); err != nil {
		return err
	}
	if err := validateReviewEvidenceV2(f.DecisionEvidence); err != nil {
		return err
	}
	if len(f.DecisionEvidence) == 0 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceUnavailable, "verdict requires governed decision evidence")
	}
	decisionDigest, err := DigestReviewEvidenceV2(f.DecisionEvidence)
	if err != nil || decisionDigest != f.DecisionEvidenceDigest {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "decision evidence digest drifted")
	}
	if err := validateReviewConditionsV2(f.Conditions); err != nil {
		return err
	}
	if f.State == ReviewVerdictConditional {
		if len(f.Conditions) == 0 || f.ConditionsDigest.Validate() != nil {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonReviewConditionUnsatisfied, "conditional verdict requires canonical conditions")
		}
	} else if len(f.Conditions) != 0 || f.ConditionsDigest != "" {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewCandidateConflict, "non-conditional verdict cannot carry conditions")
	}
	if len(f.Conditions) != 0 {
		conditionsDigest, err := DigestReviewConditionsV2(f.Conditions)
		if err != nil || conditionsDigest != f.ConditionsDigest {
			return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "review condition digest drifted")
		}
	}
	if f.Basis == ReviewBasisPolicyNotRequired {
		if f.State != ReviewVerdictAccepted || strings.TrimSpace(f.PolicyDecisionRef) == "" {
			return core.NewError(core.ErrorForbidden, core.ReasonReviewVerdictStale, "policy-not-required basis requires accepted verdict and explicit policy decision")
		}
	} else if strings.TrimSpace(f.PolicyDecisionRef) == "" {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "every verdict must bind its explicit policy decision")
	}
	if (f.InvocationEffect == nil) != (f.InvocationSettlementDigest == "") {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewRemoteEffectRequired, "reviewer Effect and settlement digest must be paired")
	}
	if f.InvocationEffect != nil {
		if err := f.InvocationEffect.Validate(); err != nil {
			return err
		}
		if err := f.InvocationSettlementDigest.Validate(); err != nil {
			return err
		}
	}
	if (f.State == ReviewVerdictExpired || f.State == ReviewVerdictRevoked) != (f.InvalidationReason != "") {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "verdict invalidation state and reason must agree")
	}
	return nil
}

func (f ConditionSatisfactionFactV2) Validate() error {
	if strings.TrimSpace(f.ID) == "" || strings.TrimSpace(f.VerdictID) == "" || f.VerdictRevision == 0 || strings.TrimSpace(string(f.IntentID)) == "" || f.IntentRevision == 0 || f.Revision == 0 || f.UpdatedUnixNano <= 0 || f.ExpiresUnixNano <= 0 || !validConditionSatisfactionStateV2(f.State) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonReviewConditionUnsatisfied, "condition satisfaction identity, revisions, state and TTL are required")
	}
	if (f.State == ConditionSatisfactionPending || f.State == ConditionSatisfied) && f.ExpiresUnixNano <= f.UpdatedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewConditionUnsatisfied, "current condition satisfaction must remain before its TTL boundary")
	}
	if f.State == ConditionSatisfactionExpired && f.UpdatedUnixNano < f.ExpiresUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewConditionUnsatisfied, "expired condition satisfaction must be recorded at or after its TTL boundary")
	}
	if strings.TrimSpace(string(f.RunID)) == "" {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonReviewConditionUnsatisfied, "condition satisfaction requires active run")
	}
	if err := f.Scope.Validate(); err != nil {
		return err
	}
	if err := f.Policy.Validate(); err != nil {
		return err
	}
	if err := f.CurrentScope.Validate(); err != nil {
		return err
	}
	for _, digest := range []core.Digest{f.VerdictDigest, f.CandidateDigest, f.SubjectDigest, f.ConditionsDigest, f.ActionScopeDigest} {
		if err := digest.Validate(); err != nil {
			return err
		}
	}
	if err := validateConditionProofsV2(f.Proofs); err != nil {
		return err
	}
	if f.State == ConditionSatisfied {
		if f.SatisfiedUnixNano <= 0 || f.SatisfiedUnixNano < f.UpdatedUnixNano || f.ProofsDigest.Validate() != nil || f.InvalidationReason != "" {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewConditionUnsatisfied, "satisfied conditions require proof digest and time")
		}
		proofDigest, err := DigestConditionProofsV2(f.Proofs)
		if err != nil || proofDigest != f.ProofsDigest {
			return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "condition proof digest drifted")
		}
	} else if f.State == ConditionSatisfactionPending {
		if f.SatisfiedUnixNano != 0 || f.ProofsDigest != "" || f.InvalidationReason != "" {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewConditionUnsatisfied, "pending condition state cannot carry proof or invalidation")
		}
	} else {
		if f.InvalidationReason == "" {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewConditionUnsatisfied, "invalidated condition state requires a reason")
		}
		// Revocation/expiry preserves any proof that was previously linearized;
		// invalidation never rewrites historical evidence.
		if f.SatisfiedUnixNano != 0 || f.ProofsDigest != "" || len(f.Proofs) != 0 {
			if f.SatisfiedUnixNano <= 0 || f.ProofsDigest.Validate() != nil {
				return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewConditionUnsatisfied, "invalidated satisfied fact must preserve complete proof evidence")
			}
			proofDigest, err := DigestConditionProofsV2(f.Proofs)
			if err != nil || proofDigest != f.ProofsDigest {
				return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "invalidated condition proof digest drifted")
			}
		}
	}
	return nil
}

func (f ReviewPolicyFactV2) ValidateCurrent(expected ReviewPolicyBindingRefV2, candidate ReviewCandidateV2, nowUnixNano int64) error {
	if strings.TrimSpace(f.Ref) == "" || f.Revision == 0 || !f.Active || f.ExpiresUnixNano <= nowUnixNano || strings.TrimSpace(string(f.RunID)) == "" || strings.TrimSpace(f.PolicyDecisionRef) == "" || strings.TrimSpace(f.ActorAuthorityRef) == "" || strings.TrimSpace(f.ReviewerAuthorityRef) == "" {
		return core.NewError(core.ErrorForbidden, core.ReasonReviewVerdictStale, "active bounded review policy fact is required")
	}
	if err := f.Scope.Validate(); err != nil {
		return err
	}
	if err := f.CurrentScope.Validate(); err != nil {
		return err
	}
	if err := ValidateNamespacedNameV2(f.RiskClass); err != nil {
		return err
	}
	digest, err := f.DigestV2()
	if err != nil {
		return err
	}
	if digest != f.Digest || f.Ref != expected.Ref || f.Digest != expected.Digest || f.Revision != expected.Revision || f.SubjectDigest != candidate.SubjectDigest || !SameExecutionScopeV2(f.Scope, candidate.Scope) || f.RunID != candidate.RunID || f.CurrentScope != candidate.CurrentScope || f.RiskClass != candidate.RiskClass || f.ActorAuthorityRef != candidate.ActorAuthority.Ref || f.ReviewerAuthorityRef != candidate.ReviewerAuthority.Ref {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "review policy drifted from exact candidate scope")
	}
	if candidate.ActorAuthority.Ref == candidate.ReviewerAuthority.Ref && !f.AllowSelfReview {
		return core.NewError(core.ErrorForbidden, core.ReasonReviewVerdictStale, "review policy does not authorize self-review")
	}
	return nil
}

func (r ReviewPolicyBindingRefV2) Validate() error {
	if strings.TrimSpace(r.Ref) == "" || r.Revision == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonReviewVerdictMissing, "review policy ref and revision are required")
	}
	return r.Digest.Validate()
}
func (r ReviewInvocationEffectRefV2) Validate() error {
	if strings.TrimSpace(string(r.EffectID)) == "" || r.EffectRevision == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonReviewRemoteEffectRequired, "review invocation Effect id and revision are required")
	}
	if err := ValidateNamespacedNameV2(NamespacedNameV2(r.EffectKind)); err != nil {
		return err
	}
	if err := r.PayloadDigest.Validate(); err != nil {
		return err
	}
	return r.Provider.Validate()
}
func (r ReviewComponentBindingRefV2) Validate() error {
	if strings.TrimSpace(r.BindingSetID) == "" || r.BindingSetRevision == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonProviderBindingStale, "review component binding set and revision are required")
	}
	if err := ValidateNamespacedNameV2(NamespacedNameV2(r.ComponentID)); err != nil {
		return err
	}
	if err := r.ManifestDigest.Validate(); err != nil {
		return err
	}
	if err := r.ArtifactDigest.Validate(); err != nil {
		return err
	}
	return ValidateNamespacedNameV2(NamespacedNameV2(r.Capability))
}
func (r ReviewEvidenceRefV2) Validate() error {
	if strings.TrimSpace(r.Ref) == "" {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonEvidenceUnavailable, "review evidence ref is required")
	}
	if err := ValidateNamespacedNameV2(r.Classification); err != nil {
		return err
	}
	return r.Digest.Validate()
}
func (o ReviewAttestationObservationV2) Validate() error {
	if strings.TrimSpace(o.CaseID) == "" || o.ObservedUnixNano <= 0 || o.CandidateDigest.Validate() != nil || !validReviewVerdictStateV2(o.ProposedState) || !validReviewDecisionBasisV2(o.Basis) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonReviewVerdictMissing, "review attestation identity, proposed decision and time are required")
	}
	if o.ProposedState == ReviewVerdictExpired || o.ProposedState == ReviewVerdictRevoked {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonReviewVerdictStale, "reviewer observation cannot invalidate authoritative facts")
	}
	if err := o.ReviewerBinding.Validate(); err != nil {
		return err
	}
	if err := o.ReviewerAuthority.Validate(); err != nil {
		return err
	}
	if err := validateReviewEvidenceV2(o.Evidence); err != nil {
		return err
	}
	return validateReviewConditionsV2(o.Conditions)
}
func (c ReviewConditionV2) Validate() error {
	if err := ValidateNamespacedNameV2(c.ID); err != nil {
		return err
	}
	if c.Revision == 0 || c.ExpiresUnixNano <= 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonReviewConditionUnsatisfied, "condition revision and TTL are required")
	}
	if err := c.Schema.Validate(); err != nil {
		return err
	}
	if err := c.ConstraintDigest.Validate(); err != nil {
		return err
	}
	if err := c.SatisfactionOwner.Validate(); err != nil {
		return err
	}
	if err := c.ScopeDigest.Validate(); err != nil {
		return err
	}
	return c.Authority.Validate()
}
func (p ReviewConditionProofV2) Validate() error {
	if err := ValidateNamespacedNameV2(p.ConditionID); err != nil {
		return err
	}
	if p.ConditionRevision == 0 || p.ExpiresUnixNano <= 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonReviewConditionUnsatisfied, "condition proof revision and TTL are required")
	}
	if err := p.ConstraintDigest.Validate(); err != nil {
		return err
	}
	if err := p.Owner.Validate(); err != nil {
		return err
	}
	if err := p.ScopeDigest.Validate(); err != nil {
		return err
	}
	if err := p.Authority.Validate(); err != nil {
		return err
	}
	return p.Evidence.Validate()
}

func DigestReviewEvidenceV2(v []ReviewEvidenceRefV2) (core.Digest, error) {
	if err := validateReviewEvidenceV2(v); err != nil {
		return "", err
	}
	if v == nil {
		v = []ReviewEvidenceRefV2{}
	}
	return core.CanonicalJSONDigest("praxis.runtime.review", ReviewContractVersionV2, "ReviewEvidenceSetV2", v)
}
func DigestReviewConditionsV2(v []ReviewConditionV2) (core.Digest, error) {
	if err := validateReviewConditionsV2(v); err != nil {
		return "", err
	}
	if v == nil {
		v = []ReviewConditionV2{}
	}
	return core.CanonicalJSONDigest("praxis.runtime.review", ReviewContractVersionV2, "ReviewConditionSetV2", v)
}
func DigestConditionProofsV2(v []ReviewConditionProofV2) (core.Digest, error) {
	if err := validateConditionProofsV2(v); err != nil {
		return "", err
	}
	if v == nil {
		v = []ReviewConditionProofV2{}
	}
	return core.CanonicalJSONDigest("praxis.runtime.review", ReviewContractVersionV2, "ReviewConditionProofSetV2", v)
}
func validateReviewEvidenceV2(v []ReviewEvidenceRefV2) error {
	if len(v) > MaxReviewEvidenceV2 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "review evidence exceeds bound")
	}
	last := ""
	for _, x := range v {
		if err := x.Validate(); err != nil {
			return err
		}
		k := string(x.Classification) + "\x00" + x.Ref
		if last != "" && k <= last {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "review evidence must be sorted and unique")
		}
		last = k
	}
	return nil
}
func validateReviewConditionsV2(v []ReviewConditionV2) error {
	if len(v) > MaxReviewConditionsV2 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "review conditions exceed bound")
	}
	last := ""
	for _, x := range v {
		if err := x.Validate(); err != nil {
			return err
		}
		k := string(x.ID)
		if last != "" && k <= last {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "review conditions must be sorted and unique")
		}
		last = k
	}
	return nil
}
func validateConditionProofsV2(v []ReviewConditionProofV2) error {
	if len(v) > MaxReviewConditionsV2 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "condition proofs exceed bound")
	}
	last := ""
	for _, x := range v {
		if err := x.Validate(); err != nil {
			return err
		}
		k := string(x.ConditionID)
		if last != "" && k <= last {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "condition proofs must be sorted and unique")
		}
		last = k
	}
	return nil
}
func validReviewCaseStateV2(v ReviewCaseStateV2) bool {
	return v == ReviewCasePending || v == ReviewCaseDecided || v == ReviewCaseExpired || v == ReviewCaseRevoked
}
func validReviewVerdictStateV2(v ReviewVerdictStateV2) bool {
	return v == ReviewVerdictAccepted || v == ReviewVerdictRejected || v == ReviewVerdictConditional || v == ReviewVerdictExpired || v == ReviewVerdictRevoked
}
func validReviewDecisionBasisV2(v ReviewDecisionBasisV2) bool {
	return v == ReviewBasisHuman || v == ReviewBasisAutomatic || v == ReviewBasisPolicyNotRequired
}
func validConditionSatisfactionStateV2(v ConditionSatisfactionStateV2) bool {
	return v == ConditionSatisfactionPending || v == ConditionSatisfied || v == ConditionSatisfactionExpired || v == ConditionSatisfactionRevoked
}
