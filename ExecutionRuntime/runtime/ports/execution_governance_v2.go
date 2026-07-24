package ports

import (
	"context"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const (
	OperationEffectContractVersionV3                  = "3.0.0"
	OperationEffectKindExecutionStartV3  EffectKindV2 = "runtime/execution-start"
	ExecutionGovernanceContractVersionV2              = "2.0.0"
	MaxExecutionDelegationHopsV2                      = 8
	MaxExecutionDelegationTTLV2                       = MaxDispatchPermitTTL
)

type OperationScopeKindV3 string

const (
	OperationScopeActivationV3  OperationScopeKindV3 = "activation_attempt"
	OperationScopeRunV3         OperationScopeKindV3 = "run"
	OperationScopeTerminationV3 OperationScopeKindV3 = "termination_attempt"
	OperationScopeAdminV3       OperationScopeKindV3 = "admin"
)

// OperationSubjectV3 is the additive non-synthetic replacement for the RunID-
// only subject in EffectIntentV2. Exactly one operation identity is present.
// The embedded ExecutionScope remains the authority/fence scope; activation
// may legitimately have no SandboxLease yet.
type OperationSubjectV3 struct {
	Kind                      OperationScopeKindV3 `json:"kind"`
	ExecutionScope            core.ExecutionScope  `json:"execution_scope"`
	ExecutionScopeDigest      core.Digest          `json:"execution_scope_digest"`
	ActivationAttemptID       string               `json:"activation_attempt_id,omitempty"`
	RunID                     core.AgentRunID      `json:"run_id,omitempty"`
	TerminationAttemptID      string               `json:"termination_attempt_id,omitempty"`
	AdminOperationID          string               `json:"admin_operation_id,omitempty"`
	CustomOperationID         string               `json:"custom_operation_id,omitempty"`
	SubjectRevision           core.Revision        `json:"subject_revision"`
	CurrentProjectionRef      string               `json:"current_projection_ref"`
	CurrentProjectionDigest   core.Digest          `json:"current_projection_digest"`
	CurrentProjectionRevision core.Revision        `json:"current_projection_revision"`
}

func (s OperationSubjectV3) Validate() error {
	if err := s.ExecutionScope.Validate(); err != nil {
		return err
	}
	scopeDigest, err := ExecutionScopeDigestV2(s.ExecutionScope)
	if err != nil || scopeDigest != s.ExecutionScopeDigest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectFenceStale, "operation subject execution scope digest drifted")
	}
	if s.SubjectRevision == 0 || validateEvidenceIDV2(s.CurrentProjectionRef) != nil || s.CurrentProjectionRevision == 0 || s.CurrentProjectionDigest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonEffectFenceStale, "operation subject current projection is incomplete")
	}
	activation := strings.TrimSpace(s.ActivationAttemptID) != ""
	run := strings.TrimSpace(string(s.RunID)) != ""
	termination := strings.TrimSpace(s.TerminationAttemptID) != ""
	admin := strings.TrimSpace(s.AdminOperationID) != ""
	custom := strings.TrimSpace(s.CustomOperationID) != ""
	if boolCountV3(activation, run, termination, admin, custom) != 1 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "operation subject requires exactly one scoped operation identity")
	}
	switch s.Kind {
	case OperationScopeActivationV3:
		if !activation || validateEvidenceIDV2(s.ActivationAttemptID) != nil {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonActivationAttemptConflict, "activation operation requires its exact attempt")
		}
	case OperationScopeRunV3:
		if !run || validateEvidenceIDV2(string(s.RunID)) != nil {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonRunConflict, "run operation requires its exact Run")
		}
	case OperationScopeTerminationV3:
		if !termination || validateEvidenceIDV2(s.TerminationAttemptID) != nil {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonCleanupEvidenceIncomplete, "termination operation requires its exact attempt")
		}
	case OperationScopeAdminV3:
		if !admin || validateEvidenceIDV2(s.AdminOperationID) != nil {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "admin operation requires its exact identity")
		}
	default:
		if ValidateNamespacedNameV2(NamespacedNameV2(s.Kind)) != nil || !custom || validateEvidenceIDV2(s.CustomOperationID) != nil {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "custom operation requires a namespaced kind and exact custom identity")
		}
	}
	return nil
}

func SameOperationSubjectV3(left, right OperationSubjectV3) bool {
	leftDigest, leftErr := left.DigestV3()
	rightDigest, rightErr := right.DigestV3()
	return leftErr == nil && rightErr == nil && leftDigest == rightDigest
}

func (s OperationSubjectV3) DigestV3() (core.Digest, error) {
	if err := s.Validate(); err != nil {
		return "", err
	}
	return core.CanonicalJSONDigest("praxis.runtime.operation-effect", OperationEffectContractVersionV3, "OperationSubjectV3", s)
}

func boolCountV3(values ...bool) int {
	count := 0
	for _, value := range values {
		if value {
			count++
		}
	}
	return count
}

type OperationReviewBindingRefV3 struct {
	CaseRef           string        `json:"case_ref"`
	CandidateDigest   core.Digest   `json:"candidate_digest"`
	CandidateRevision core.Revision `json:"candidate_revision"`
	PolicyDigest      core.Digest   `json:"policy_digest"`
}

func (r OperationReviewBindingRefV3) Validate() error {
	if validateEvidenceIDV2(r.CaseRef) != nil || r.CandidateRevision == 0 {
		return core.NewError(core.ErrorForbidden, core.ReasonReviewVerdictMissing, "operation review case and candidate are required")
	}
	for _, digest := range []core.Digest{r.CandidateDigest, r.PolicyDigest} {
		if err := digest.Validate(); err != nil {
			return err
		}
	}
	return nil
}

type OperationBudgetBindingRefV3 struct {
	Ref           string        `json:"ref"`
	Digest        core.Digest   `json:"digest"`
	Revision      core.Revision `json:"revision"`
	PolicyDigest  core.Digest   `json:"policy_digest"`
	SubjectDigest core.Digest   `json:"operation_subject_digest"`
}

func (r OperationBudgetBindingRefV3) Validate() error {
	if validateEvidenceIDV2(r.Ref) != nil || r.Revision == 0 {
		return core.NewError(core.ErrorForbidden, core.ReasonBudgetBindingMissing, "operation budget fact is required")
	}
	for _, digest := range []core.Digest{r.Digest, r.PolicyDigest, r.SubjectDigest} {
		if err := digest.Validate(); err != nil {
			return err
		}
	}
	return nil
}

type OperationPolicyBindingRefV3 struct {
	Ref           string        `json:"ref"`
	Digest        core.Digest   `json:"digest"`
	Revision      core.Revision `json:"revision"`
	SubjectDigest core.Digest   `json:"operation_subject_digest"`
}

func (r OperationPolicyBindingRefV3) Validate() error {
	if validateEvidenceIDV2(r.Ref) != nil || r.Revision == 0 {
		return core.NewError(core.ErrorForbidden, core.ReasonEffectAuthorizationMissing, "operation dispatch policy is required")
	}
	for _, digest := range []core.Digest{r.Digest, r.SubjectDigest} {
		if err := digest.Validate(); err != nil {
			return err
		}
	}
	return nil
}

type OperationEffectRelationV3 struct {
	Purpose                   NamespacedNameV2    `json:"purpose,omitempty"`
	OriginalOperationEffectID core.EffectIntentID `json:"original_operation_effect_id,omitempty"`
	OriginalRevision          core.Revision       `json:"original_revision,omitempty"`
}

func (r OperationEffectRelationV3) Validate(self core.EffectIntentID, revision core.Revision) error {
	present := r.Purpose != "" || r.OriginalOperationEffectID != "" || r.OriginalRevision != 0
	if !present {
		return nil
	}
	if ValidateNamespacedNameV2(r.Purpose) != nil || r.OriginalOperationEffectID == "" || r.OriginalRevision == 0 || r.OriginalOperationEffectID == self && r.OriginalRevision == revision {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "operation effect relation requires one different exact original revision")
	}
	return nil
}

// OperationEffectIntentV3 carries OperationSubject through the complete
// governance chain. It does not reinterpret activation/termination/admin as a
// synthetic Run and does not alter the stable EffectIntentV2 contract.
type OperationEffectIntentV3 struct {
	ContractVersion        string                      `json:"contract_version"`
	ID                     core.EffectIntentID         `json:"effect_intent_id"`
	Revision               core.Revision               `json:"effect_intent_revision"`
	Operation              OperationSubjectV3          `json:"operation_subject"`
	Kind                   EffectKindV2                `json:"effect_kind"`
	RiskClass              NamespacedNameV2            `json:"risk_class"`
	ActionScopeDigest      core.Digest                 `json:"action_scope_digest"`
	Payload                OpaquePayloadV2             `json:"payload"`
	PayloadRevision        core.Revision               `json:"payload_revision"`
	Target                 string                      `json:"target"`
	ConflictDomain         ConflictDomainBindingV2     `json:"conflict_domain"`
	Owners                 []EffectOwnerRefV2          `json:"owners"`
	Provider               ProviderBindingRefV2        `json:"provider_binding"`
	Authority              AuthorityBindingRefV2       `json:"authority_binding"`
	Review                 OperationReviewBindingRefV3 `json:"review_binding"`
	Budget                 OperationBudgetBindingRefV3 `json:"budget_binding"`
	Policy                 OperationPolicyBindingRefV3 `json:"dispatch_policy_binding"`
	Idempotency            IdempotencyBindingV2        `json:"idempotency"`
	CredentialLeases       []CredentialLeaseRefV2      `json:"credential_leases"`
	Relation               OperationEffectRelationV3   `json:"relation"`
	MayLeaveRemoteResidual bool                        `json:"may_leave_remote_residual"`
	RequiresCleanup        bool                        `json:"requires_cleanup"`
	ExpiresUnixNano        int64                       `json:"expires_unix_nano"`
}

func (i OperationEffectIntentV3) Validate() error {
	if i.ContractVersion != OperationEffectContractVersionV3 || validateEvidenceIDV2(string(i.ID)) != nil || i.Revision == 0 || i.PayloadRevision == 0 || i.ExpiresUnixNano <= 0 || strings.TrimSpace(i.Target) == "" || len(i.Target) > 512 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonEffectIntentMissing, "operation effect identity, revision, target and lifetime are required")
	}
	if err := i.Operation.Validate(); err != nil {
		return err
	}
	if ValidateNamespacedNameV2(NamespacedNameV2(i.Kind)) != nil || ValidateNamespacedNameV2(i.RiskClass) != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidNamespace, "operation effect kind and risk must be namespaced")
	}
	if err := i.ActionScopeDigest.Validate(); err != nil {
		return err
	}
	if err := i.Payload.Validate(); err != nil {
		return err
	}
	if err := i.ConflictDomain.ValidateForScope(i.Operation.ExecutionScope); err != nil {
		return err
	}
	if err := validateEffectOwnersV2(i.Owners); err != nil {
		return err
	}
	if err := i.Provider.Validate(); err != nil {
		return err
	}
	if err := i.Authority.Validate(); err != nil {
		return err
	}
	if err := i.Review.Validate(); err != nil {
		return err
	}
	operationDigest, err := i.Operation.DigestV3()
	if err != nil || i.Budget.SubjectDigest != operationDigest || i.Policy.SubjectDigest != operationDigest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectAuthorizationMissing, "operation budget or policy binds another subject")
	}
	if err := i.Budget.Validate(); err != nil {
		return err
	}
	if err := i.Policy.Validate(); err != nil {
		return err
	}
	if err := i.Idempotency.Validate(); err != nil {
		return err
	}
	if i.Idempotency.ScopeDigest != StableTenantScopeDigestV2(i.Operation.ExecutionScope.Identity.TenantID) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidReference, "operation idempotency must use the tenant-stable scope")
	}
	if len(i.CredentialLeases) > 64 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "operation credential set exceeds its bound")
	}
	previous := ""
	for index, credential := range i.CredentialLeases {
		if err := credential.Validate(); err != nil {
			return err
		}
		key := credential.Class + "\x00" + credential.Ref
		if index > 0 && key <= previous {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "operation credentials must be sorted and unique")
		}
		previous = key
	}
	return i.Relation.Validate(i.ID, i.Revision)
}

func (i OperationEffectIntentV3) DigestV3() (core.Digest, error) {
	if err := i.Validate(); err != nil {
		return "", err
	}
	if i.Owners == nil {
		i.Owners = []EffectOwnerRefV2{}
	}
	if i.CredentialLeases == nil {
		i.CredentialLeases = []CredentialLeaseRefV2{}
	}
	return core.CanonicalJSONDigest("praxis.runtime.operation-effect", OperationEffectContractVersionV3, "OperationEffectIntentV3", i)
}

type OperationDispatchPermitV3 struct {
	ContractVersion          string                         `json:"contract_version"`
	ID                       string                         `json:"permit_id"`
	Revision                 core.Revision                  `json:"permit_revision"`
	AttemptID                string                         `json:"attempt_id"`
	IntentID                 core.EffectIntentID            `json:"intent_id"`
	IntentRevision           core.Revision                  `json:"intent_revision"`
	IntentDigest             core.Digest                    `json:"intent_digest"`
	Operation                OperationSubjectV3             `json:"operation_subject"`
	PayloadSchema            SchemaRefV2                    `json:"payload_schema"`
	PayloadDigest            core.Digest                    `json:"payload_digest"`
	PayloadRevision          core.Revision                  `json:"payload_revision"`
	ConflictDomain           ConflictDomainBindingV2        `json:"conflict_domain"`
	Provider                 ProviderBindingRefV2           `json:"provider_binding"`
	EnforcementPoint         ProviderBindingRefV2           `json:"enforcement_point_binding"`
	Authority                AuthorityBindingRefV2          `json:"authority_binding"`
	Review                   OperationReviewBindingRefV3    `json:"review_binding"`
	ReviewAuthorization      OperationReviewAuthorizationV3 `json:"review_authorization"`
	Budget                   OperationBudgetBindingRefV3    `json:"budget_binding"`
	Policy                   OperationPolicyBindingRefV3    `json:"dispatch_policy_binding"`
	CapabilityGrantDigest    core.Digest                    `json:"capability_grant_digest"`
	CredentialGrantDigest    core.Digest                    `json:"credential_grant_digest"`
	GovernanceSnapshotDigest core.Digest                    `json:"governance_snapshot_digest"`
	FenceDigest              core.Digest                    `json:"fence_digest"`
	Idempotency              IdempotencyBindingV2           `json:"idempotency"`
	IssuedUnixNano           int64                          `json:"issued_unix_nano"`
	ExpiresUnixNano          int64                          `json:"expires_unix_nano"`
}

func (p OperationDispatchPermitV3) Validate() error {
	if p.ContractVersion != OperationEffectContractVersionV3 || validateEvidenceIDV2(p.ID) != nil || p.Revision == 0 || validateEvidenceIDV2(p.AttemptID) != nil || validateEvidenceIDV2(string(p.IntentID)) != nil || p.IntentRevision == 0 || p.PayloadRevision == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonDispatchPermitInvalid, "operation permit identity and one exact attempt are required")
	}
	if err := p.Operation.Validate(); err != nil {
		return err
	}
	if err := p.PayloadSchema.Validate(); err != nil {
		return err
	}
	for _, digest := range []core.Digest{p.IntentDigest, p.PayloadDigest, p.CapabilityGrantDigest, p.CredentialGrantDigest, p.GovernanceSnapshotDigest, p.FenceDigest} {
		if err := digest.Validate(); err != nil {
			return err
		}
	}
	if err := p.ConflictDomain.ValidateForScope(p.Operation.ExecutionScope); err != nil {
		return err
	}
	if err := p.Provider.Validate(); err != nil {
		return err
	}
	if err := p.EnforcementPoint.Validate(); err != nil {
		return err
	}
	// Isolation is established by exact provider capability/binding, not by
	// requiring a different ComponentID from a host relay.
	if p.EnforcementPoint != p.Provider {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonProviderBindingStale, "operation enforcement receipt must come from the exact provider binding and capability")
	}
	if err := p.Authority.Validate(); err != nil {
		return err
	}
	if err := p.Review.Validate(); err != nil {
		return err
	}
	if err := p.ReviewAuthorization.ValidateCurrent(p.Review, time.Unix(0, p.IssuedUnixNano)); err != nil {
		return err
	}
	if err := p.Budget.Validate(); err != nil {
		return err
	}
	if err := p.Policy.Validate(); err != nil {
		return err
	}
	if err := p.Idempotency.Validate(); err != nil {
		return err
	}
	if p.IssuedUnixNano <= 0 || p.ExpiresUnixNano <= p.IssuedUnixNano || time.Duration(p.ExpiresUnixNano-p.IssuedUnixNano) > MaxDispatchPermitTTL {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonDispatchPermitInvalid, "operation permit TTL is invalid")
	}
	return nil
}

type OperationGovernanceFactRefV3 struct {
	Ref             string        `json:"ref"`
	Revision        core.Revision `json:"revision"`
	Digest          core.Digest   `json:"digest"`
	ExpiresUnixNano int64         `json:"expires_unix_nano"`
}

func (r OperationGovernanceFactRefV3) Validate(now time.Time) error {
	if validateEvidenceIDV2(r.Ref) != nil || r.Revision == 0 || r.ExpiresUnixNano <= 0 || now.IsZero() || !now.Before(time.Unix(0, r.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectAuthorizationMissing, "operation governance fact ref is incomplete, expired or unavailable")
	}
	return r.Digest.Validate()
}

// OperationReviewAuthorizationV3 is the current, revocable Review projection.
// The immutable Effect binds only Case/Candidate/Policy; Verdict and optional
// Satisfaction are read at Issue, Begin and actual provider Prepare.
type OperationReviewAuthorizationV3 struct {
	Case              OperationGovernanceFactRefV3  `json:"case_fact"`
	CandidateDigest   core.Digest                   `json:"candidate_digest"`
	CandidateRevision core.Revision                 `json:"candidate_revision"`
	Verdict           OperationGovernanceFactRefV3  `json:"verdict_fact"`
	Satisfaction      *OperationGovernanceFactRefV3 `json:"satisfaction_fact,omitempty"`
	ReviewerAuthority OperationGovernanceFactRefV3  `json:"reviewer_authority_fact"`
	PolicyDigest      core.Digest                   `json:"policy_digest"`
	ExpiresUnixNano   int64                         `json:"expires_unix_nano"`
}

func (a OperationReviewAuthorizationV3) ValidateCurrent(expected OperationReviewBindingRefV3, now time.Time) error {
	if err := expected.Validate(); err != nil {
		return err
	}
	for _, fact := range []OperationGovernanceFactRefV3{a.Case, a.Verdict, a.ReviewerAuthority} {
		if err := fact.Validate(now); err != nil {
			return err
		}
	}
	if a.Satisfaction != nil {
		if err := a.Satisfaction.Validate(now); err != nil {
			return err
		}
	}
	if a.CandidateDigest.Validate() != nil || a.PolicyDigest.Validate() != nil || a.ExpiresUnixNano <= 0 || now.IsZero() || !now.Before(time.Unix(0, a.ExpiresUnixNano)) || a.Case.Ref != expected.CaseRef || a.CandidateDigest != expected.CandidateDigest || a.CandidateRevision != expected.CandidateRevision || a.PolicyDigest != expected.PolicyDigest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "operation Review case, candidate, verdict or policy drifted")
	}
	return nil
}

func sameOperationReviewAuthorizationV3(left, right OperationReviewAuthorizationV3) bool {
	leftDigest, leftErr := core.CanonicalJSONDigest("praxis.runtime.operation-effect", OperationEffectContractVersionV3, "OperationReviewAuthorizationV3", left)
	rightDigest, rightErr := core.CanonicalJSONDigest("praxis.runtime.operation-effect", OperationEffectContractVersionV3, "OperationReviewAuthorizationV3", right)
	return leftErr == nil && rightErr == nil && leftDigest == rightDigest
}

type OperationCredentialCurrentFactV3 struct {
	Lease           CredentialLeaseRefV2 `json:"lease"`
	FactDigest      core.Digest          `json:"fact_digest"`
	FactRevision    core.Revision        `json:"fact_revision"`
	ExpiresUnixNano int64                `json:"expires_unix_nano"`
}

func (f OperationCredentialCurrentFactV3) Validate(now time.Time) error {
	if err := f.Lease.Validate(); err != nil {
		return err
	}
	if f.FactRevision == 0 || f.FactDigest.Validate() != nil || now.IsZero() || !now.Before(time.Unix(0, f.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonCredentialLeaseMissing, "operation credential current fact is incomplete or expired")
	}
	return nil
}

// OperationGovernanceSnapshotV3 is a read-only consistent projection over
// existing Identity, Binding, Authority, Review, Budget, Policy, Credential
// and operation-scope Fact Owners. It is not a second authority store.
type OperationGovernanceSnapshotV3 struct {
	Operation             OperationSubjectV3                 `json:"operation_subject"`
	Active                bool                               `json:"active"`
	ProjectionWatermark   uint64                             `json:"projection_watermark"`
	Identity              OperationGovernanceFactRefV3       `json:"identity_fact"`
	Binding               OperationGovernanceFactRefV3       `json:"binding_fact"`
	CurrentScope          OperationGovernanceFactRefV3       `json:"current_scope_fact"`
	Authority             OperationGovernanceFactRefV3       `json:"authority_fact"`
	Review                OperationReviewAuthorizationV3     `json:"review_authorization"`
	Budget                OperationGovernanceFactRefV3       `json:"budget_fact"`
	Policy                OperationGovernanceFactRefV3       `json:"policy_fact"`
	Provider              ProviderBindingRefV2               `json:"provider_binding"`
	EnforcementPoint      ProviderBindingRefV2               `json:"enforcement_point_binding"`
	CapabilityGrantDigest core.Digest                        `json:"capability_grant_digest"`
	Credentials           []OperationCredentialCurrentFactV3 `json:"credentials"`
	ExpiresUnixNano       int64                              `json:"expires_unix_nano"`
}

func (s OperationGovernanceSnapshotV3) ValidateCurrent(intent OperationEffectIntentV3, now time.Time) error {
	if !s.Active || s.ProjectionWatermark == 0 || !SameOperationSubjectV3(s.Operation, intent.Operation) || now.IsZero() || !now.Before(time.Unix(0, s.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectFenceStale, "operation current projection is inactive, expired or drifted")
	}
	for _, ref := range []OperationGovernanceFactRefV3{s.Identity, s.Binding, s.CurrentScope, s.Authority, s.Budget, s.Policy} {
		if err := ref.Validate(now); err != nil {
			return err
		}
	}
	if err := s.Review.ValidateCurrent(intent.Review, now); err != nil {
		return err
	}
	if s.CurrentScope.Ref != intent.Operation.CurrentProjectionRef || s.CurrentScope.Revision != intent.Operation.CurrentProjectionRevision || s.CurrentScope.Digest != intent.Operation.CurrentProjectionDigest || s.Provider != intent.Provider || s.EnforcementPoint != intent.Provider || s.Binding.Ref != intent.Provider.BindingSetID || s.Binding.Revision != intent.Provider.BindingSetRevision || s.Authority.Ref != intent.Authority.Ref || s.Authority.Revision != intent.Authority.Revision || s.Authority.Digest != intent.Authority.Digest || s.Budget.Ref != intent.Budget.Ref || s.Budget.Revision != intent.Budget.Revision || s.Budget.Digest != intent.Budget.Digest || s.Policy.Ref != intent.Policy.Ref || s.Policy.Revision != intent.Policy.Revision || s.Policy.Digest != intent.Policy.Digest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectAuthorizationMissing, "operation current governance facts drifted from exact intent bindings")
	}
	if err := s.CapabilityGrantDigest.Validate(); err != nil {
		return err
	}
	if len(s.Credentials) != len(intent.CredentialLeases) || len(s.Credentials) > 64 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonCredentialLeaseMissing, "operation credential set cardinality drifted")
	}
	for index, credential := range s.Credentials {
		if err := credential.Validate(now); err != nil {
			return err
		}
		if credential.Lease != intent.CredentialLeases[index] {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonCredentialLeaseMissing, "operation credential lease set drifted")
		}
	}
	return nil
}

func (s OperationGovernanceSnapshotV3) DigestV3(now time.Time) (core.Digest, error) {
	// Digest validation uses a caller-supplied current time so expired facts can
	// never be re-signed as a current snapshot.
	if now.IsZero() || !s.Active {
		return "", core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectFenceStale, "current operation governance snapshot is required")
	}
	if s.Credentials == nil {
		s.Credentials = []OperationCredentialCurrentFactV3{}
	}
	return core.CanonicalJSONDigest("praxis.runtime.operation-effect", OperationEffectContractVersionV3, "OperationGovernanceSnapshotV3", s)
}

func DigestOperationCredentialFactsV3(facts []OperationCredentialCurrentFactV3, now time.Time) (core.Digest, error) {
	if facts == nil {
		facts = []OperationCredentialCurrentFactV3{}
	}
	previous := ""
	for index, fact := range facts {
		if err := fact.Validate(now); err != nil {
			return "", err
		}
		key := fact.Lease.Class + "\x00" + fact.Lease.Ref
		if index > 0 && key <= previous {
			return "", core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "operation credential facts must be sorted and unique")
		}
		previous = key
	}
	return core.CanonicalJSONDigest("praxis.runtime.operation-effect", OperationEffectContractVersionV3, "OperationCredentialCurrentFactsV3", facts)
}

type OperationGovernanceCurrentReaderV3 interface {
	// InspectOperationGovernance must reconstruct a consistent projection from
	// the independent authoritative Fact Owners. A caller-provided snapshot is
	// never sufficient authority.
	InspectOperationGovernance(context.Context, OperationSubjectV3) (OperationGovernanceSnapshotV3, error)
}

func ValidateOperationAtExecutionPointV3(permit OperationDispatchPermitV3, intent OperationEffectIntentV3, fence core.ExecutionFence, current OperationGovernanceSnapshotV3, now time.Time) error {
	if err := permit.Validate(); err != nil {
		return err
	}
	if err := intent.Validate(); err != nil {
		return err
	}
	if err := current.ValidateCurrent(intent, now); err != nil {
		return err
	}
	intentDigest, _ := intent.DigestV3()
	snapshotDigest, _ := current.DigestV3(now)
	credentialDigest, err := DigestOperationCredentialFactsV3(current.Credentials, now)
	if err != nil {
		return err
	}
	fenceDigest, err := DigestOperationExecutionFenceV3(fence, intent.Operation)
	if err != nil || permit.IntentID != intent.ID || permit.IntentRevision != intent.Revision || permit.IntentDigest != intentDigest || !SameOperationSubjectV3(permit.Operation, intent.Operation) || permit.PayloadSchema != intent.Payload.Schema || permit.PayloadDigest != intent.Payload.ContentDigest || permit.PayloadRevision != intent.PayloadRevision || permit.Provider != intent.Provider || permit.EnforcementPoint != current.EnforcementPoint || permit.Authority != intent.Authority || permit.Review != intent.Review || !sameOperationReviewAuthorizationV3(permit.ReviewAuthorization, current.Review) || permit.Budget != intent.Budget || permit.Policy != intent.Policy || permit.CapabilityGrantDigest != current.CapabilityGrantDigest || permit.CredentialGrantDigest != credentialDigest || permit.GovernanceSnapshotDigest != snapshotDigest || permit.FenceDigest != fenceDigest || now.IsZero() || !now.Before(time.Unix(0, permit.ExpiresUnixNano)) || !now.Before(time.Unix(0, intent.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonDispatchPermitInvalid, "operation Permit or current governance facts drifted before provider contact")
	}
	return core.CheckFence(fence, core.CurrentFenceFacts{Scope: current.Operation.ExecutionScope, CapabilityGrantDigest: current.CapabilityGrantDigest}, now)
}

func (p OperationDispatchPermitV3) DigestV3() (core.Digest, error) {
	if err := p.Validate(); err != nil {
		return "", err
	}
	return core.CanonicalJSONDigest("praxis.runtime.operation-effect", OperationEffectContractVersionV3, "OperationDispatchPermitV3", p)
}

func DigestOperationExecutionFenceV3(fence core.ExecutionFence, operation OperationSubjectV3) (core.Digest, error) {
	if err := fence.Validate(); err != nil {
		return "", err
	}
	if err := operation.Validate(); err != nil {
		return "", err
	}
	if !SameExecutionScopeV2(fence.Scope, operation.ExecutionScope) {
		return "", core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectFenceStale, "operation fence scope differs")
	}
	return core.CanonicalJSONDigest("praxis.runtime.operation-effect", OperationEffectContractVersionV3, "OperationExecutionFenceV3", struct {
		Fence     core.ExecutionFence `json:"fence"`
		Operation OperationSubjectV3  `json:"operation"`
	}{fence, operation})
}

type OperationEnforcementReceiptV3 struct {
	ContractVersion   string                 `json:"contract_version"`
	PermitID          string                 `json:"permit_id"`
	PermitRevision    core.Revision          `json:"permit_revision"`
	AttemptID         string                 `json:"attempt_id"`
	PermitDigest      core.Digest            `json:"permit_digest"`
	Operation         OperationSubjectV3     `json:"operation_subject"`
	Verifier          ProviderBindingRefV2   `json:"verifier_binding"`
	ValidatedUnixNano int64                  `json:"validated_unix_nano"`
	Attestation       *GovernanceExtensionV2 `json:"attestation,omitempty"`
}

func (r OperationEnforcementReceiptV3) Validate() error {
	if r.ContractVersion != OperationEffectContractVersionV3 || validateEvidenceIDV2(r.PermitID) != nil || r.PermitRevision == 0 || validateEvidenceIDV2(r.AttemptID) != nil || r.ValidatedUnixNano <= 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonDispatchPermitInvalid, "operation enforcement receipt identity is incomplete")
	}
	if err := r.PermitDigest.Validate(); err != nil {
		return err
	}
	if err := r.Operation.Validate(); err != nil {
		return err
	}
	if err := r.Verifier.Validate(); err != nil {
		return err
	}
	if r.Attestation != nil {
		return validateExtensionsV2([]GovernanceExtensionV2{*r.Attestation})
	}
	return nil
}

type ExecutionDelegationStateV2 string

const (
	ExecutionDelegationDeclaredV2 ExecutionDelegationStateV2 = "declared"
	ExecutionDelegationPreparedV2 ExecutionDelegationStateV2 = "prepared"
	ExecutionDelegationRevokedV2  ExecutionDelegationStateV2 = "revoked"
	ExecutionDelegationExpiredV2  ExecutionDelegationStateV2 = "expired"
)

type ExecutionRelayHopV2 struct {
	Sequence uint32               `json:"sequence"`
	Relay    ProviderBindingRefV2 `json:"relay_binding"`
}

func (h ExecutionRelayHopV2) Validate() error {
	if h.Sequence == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "relay hop sequence is required")
	}
	return h.Relay.Validate()
}

type ExecutionDelegationRefV2 struct {
	ID       string        `json:"id"`
	Revision core.Revision `json:"revision"`
	Digest   core.Digest   `json:"digest"`
}

func (r ExecutionDelegationRefV2) Validate() error {
	if validateEvidenceIDV2(r.ID) != nil || r.Revision == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "execution delegation ref is incomplete")
	}
	return r.Digest.Validate()
}

// ExecutionDelegationFactV2 binds a relay path to one already-governed
// provider attempt. It does not grant dispatch authority and may use the same
// ComponentID for host and provider only when their exact capability/binding
// refs remain distinct and the provider receipt matches DataProvider.
type ExecutionDelegationFactV2 struct {
	ContractVersion                string                            `json:"contract_version"`
	ID                             string                            `json:"id"`
	Revision                       core.Revision                     `json:"revision"`
	State                          ExecutionDelegationStateV2        `json:"state"`
	BindingSetID                   string                            `json:"binding_set_id"`
	BindingSetRevision             core.Revision                     `json:"binding_set_revision"`
	Operation                      OperationSubjectV3                `json:"operation_subject"`
	HostAdapter                    ProviderBindingRefV2              `json:"host_adapter_binding"`
	DataProvider                   ProviderBindingRefV2              `json:"data_provider_binding"`
	RelayHops                      []ExecutionRelayHopV2             `json:"relay_hops"`
	EndpointID                     string                            `json:"endpoint_id"`
	RuntimeSessionRef              string                            `json:"runtime_session_ref"`
	PayloadSchema                  SchemaRefV2                       `json:"payload_schema"`
	PayloadDigest                  core.Digest                       `json:"payload_digest"`
	PayloadRevision                core.Revision                     `json:"payload_revision"`
	IntentID                       core.EffectIntentID               `json:"intent_id"`
	IntentRevision                 core.Revision                     `json:"intent_revision"`
	IntentDigest                   core.Digest                       `json:"intent_digest"`
	ProviderPermitID               string                            `json:"provider_permit_id"`
	ProviderPermitRevision         core.Revision                     `json:"provider_permit_revision"`
	ProviderPermitDigest           core.Digest                       `json:"provider_permit_digest"`
	ProviderAttemptID              string                            `json:"provider_attempt_id"`
	PreparedAttemptID              string                            `json:"prepared_attempt_id"`
	Preparation                    *ProviderPreparationAttestationV2 `json:"preparation_attestation,omitempty"`
	OperationExpiresUnixNano       int64                             `json:"operation_expires_unix_nano"`
	PermitExpiresUnixNano          int64                             `json:"permit_expires_unix_nano"`
	HostBindingExpiresUnixNano     int64                             `json:"host_binding_expires_unix_nano"`
	ProviderBindingExpiresUnixNano int64                             `json:"provider_binding_expires_unix_nano"`
	CreatedUnixNano                int64                             `json:"created_unix_nano"`
	ExpiresUnixNano                int64                             `json:"expires_unix_nano"`
}

func (f ExecutionDelegationFactV2) Validate() error {
	if f.ContractVersion != ExecutionGovernanceContractVersionV2 || validateEvidenceIDV2(f.ID) != nil || f.Revision == 0 || validateEvidenceIDV2(f.BindingSetID) != nil || f.BindingSetRevision == 0 || validateEvidenceIDV2(f.EndpointID) != nil || validateEvidenceIDV2(f.RuntimeSessionRef) != nil || f.PayloadRevision == 0 || validateEvidenceIDV2(string(f.IntentID)) != nil || f.IntentRevision == 0 || validateEvidenceIDV2(f.ProviderPermitID) != nil || f.ProviderPermitRevision == 0 || validateEvidenceIDV2(f.ProviderAttemptID) != nil || validateEvidenceIDV2(f.PreparedAttemptID) != nil || f.CreatedUnixNano <= 0 || f.ExpiresUnixNano <= f.CreatedUnixNano {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "execution delegation identity, provider attempt and TTL are incomplete")
	}
	expectedPreparedID, err := DerivePreparedProviderAttemptIDV2(f.ID, f.ProviderPermitID, f.ProviderAttemptID)
	if err != nil || expectedPreparedID != f.PreparedAttemptID {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonIdempotencyPayloadMismatch, "delegation prepared attempt identity is not the canonical preallocated key")
	}
	if f.State != ExecutionDelegationDeclaredV2 && f.State != ExecutionDelegationPreparedV2 && f.State != ExecutionDelegationRevokedV2 && f.State != ExecutionDelegationExpiredV2 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "execution delegation state is invalid")
	}
	if f.State == ExecutionDelegationDeclaredV2 && f.Preparation != nil {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "declared delegation cannot carry a preparation attestation")
	}
	if f.State == ExecutionDelegationPreparedV2 {
		if f.Preparation == nil {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonDispatchPermitInvalid, "prepared delegation requires the exact provider preparation attestation")
		}
		if err := f.Preparation.Validate(); err != nil {
			return err
		}
		if f.Preparation.Delegation.ID != f.ID || f.Preparation.Delegation.Revision+1 != f.Revision {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonRevisionConflict, "prepared delegation does not descend from its declared preparation watermark")
		}
	}
	if err := f.Operation.Validate(); err != nil {
		return err
	}
	if err := f.HostAdapter.Validate(); err != nil {
		return err
	}
	if err := f.DataProvider.Validate(); err != nil {
		return err
	}
	if f.HostAdapter == f.DataProvider {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonProviderBindingStale, "host relay and data provider require distinct exact binding/capability refs")
	}
	if f.HostAdapter.BindingSetID != f.BindingSetID || f.DataProvider.BindingSetID != f.BindingSetID || f.HostAdapter.BindingSetRevision != f.BindingSetRevision || f.DataProvider.BindingSetRevision != f.BindingSetRevision {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonProviderBindingStale, "delegation host and provider must bind the exact current BindingSet watermark")
	}
	if len(f.RelayHops) == 0 || len(f.RelayHops) > MaxExecutionDelegationHopsV2 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "execution delegation relay chain is empty or exceeds its bound")
	}
	seenRelays := make(map[ProviderBindingRefV2]struct{}, len(f.RelayHops))
	for index, hop := range f.RelayHops {
		if err := hop.Validate(); err != nil {
			return err
		}
		if hop.Sequence != uint32(index+1) {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "execution relay hops must form a bounded canonical sequence")
		}
		if index == 0 && hop.Relay != f.HostAdapter {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonProviderBindingStale, "first relay hop must be the bound host adapter")
		}
		if hop.Relay == f.DataProvider {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonProviderBindingStale, "data provider cannot appear in the relay-only hop chain")
		}
		if hop.Relay.BindingSetID != f.BindingSetID || hop.Relay.BindingSetRevision != f.BindingSetRevision {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonProviderBindingStale, "every relay hop must bind the frozen BindingSet watermark")
		}
		if _, exists := seenRelays[hop.Relay]; exists {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonDependencyCycle, "execution delegation relay chain repeats a binding")
		}
		seenRelays[hop.Relay] = struct{}{}
	}
	if err := f.PayloadSchema.Validate(); err != nil {
		return err
	}
	for _, digest := range []core.Digest{f.PayloadDigest, f.IntentDigest, f.ProviderPermitDigest} {
		if err := digest.Validate(); err != nil {
			return err
		}
	}
	if time.Duration(f.ExpiresUnixNano-f.CreatedUnixNano) > MaxExecutionDelegationTTLV2 || f.OperationExpiresUnixNano <= f.CreatedUnixNano || f.PermitExpiresUnixNano <= f.CreatedUnixNano || f.HostBindingExpiresUnixNano <= f.CreatedUnixNano || f.ProviderBindingExpiresUnixNano <= f.CreatedUnixNano || f.ExpiresUnixNano > f.OperationExpiresUnixNano || f.ExpiresUnixNano > f.PermitExpiresUnixNano || f.ExpiresUnixNano > f.HostBindingExpiresUnixNano || f.ExpiresUnixNano > f.ProviderBindingExpiresUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonDispatchPermitExpired, "delegation TTL exceeds its operation, Permit or binding lifetime")
	}
	return nil
}

func DerivePreparedProviderAttemptIDV2(delegationID, permitID, attemptID string) (string, error) {
	for _, value := range []string{delegationID, permitID, attemptID} {
		if err := validateEvidenceIDV2(value); err != nil {
			return "", err
		}
	}
	digest, err := core.CanonicalJSONDigest("praxis.runtime.execution-governance", ExecutionGovernanceContractVersionV2, "PreparedProviderAttemptIdentityV2", struct {
		DelegationID string `json:"delegation_id"`
		PermitID     string `json:"permit_id"`
		AttemptID    string `json:"attempt_id"`
	}{delegationID, permitID, attemptID})
	if err != nil {
		return "", err
	}
	return "prepared-" + strings.TrimPrefix(string(digest), "sha256:"), nil
}

func (f ExecutionDelegationFactV2) DigestV2() (core.Digest, error) {
	if err := f.Validate(); err != nil {
		return "", err
	}
	if f.RelayHops == nil {
		f.RelayHops = []ExecutionRelayHopV2{}
	}
	return core.CanonicalJSONDigest("praxis.runtime.execution-governance", ExecutionGovernanceContractVersionV2, "ExecutionDelegationFactV2", f)
}

func (f ExecutionDelegationFactV2) RefV2() (ExecutionDelegationRefV2, error) {
	digest, err := f.DigestV2()
	if err != nil {
		return ExecutionDelegationRefV2{}, err
	}
	return ExecutionDelegationRefV2{ID: f.ID, Revision: f.Revision, Digest: digest}, nil
}

type ExecutionDelegationCASRequestV2 struct {
	ExpectedRevision core.Revision             `json:"expected_revision"`
	Next             ExecutionDelegationFactV2 `json:"next"`
}

type ExecutionDelegationFactPortV2 interface {
	CreateExecutionDelegationV2(context.Context, ExecutionDelegationFactV2) (ExecutionDelegationFactV2, error)
	InspectExecutionDelegationV2(context.Context, string) (ExecutionDelegationFactV2, error)
	CompareAndSwapExecutionDelegationV2(context.Context, ExecutionDelegationCASRequestV2) (ExecutionDelegationFactV2, error)
}

type PreparedProviderAttemptRefV2 struct {
	ID                 string                   `json:"id"`
	Revision           core.Revision            `json:"revision"`
	Digest             core.Digest              `json:"digest"`
	DeclaredDelegation ExecutionDelegationRefV2 `json:"declared_delegation"`
	OperationDigest    core.Digest              `json:"operation_digest"`
	IntentID           core.EffectIntentID      `json:"intent_id"`
	IntentRevision     core.Revision            `json:"intent_revision"`
	IntentDigest       core.Digest              `json:"intent_digest"`
	PermitID           string                   `json:"permit_id"`
	PermitRevision     core.Revision            `json:"permit_revision"`
	PermitDigest       core.Digest              `json:"permit_digest"`
	AttemptID          string                   `json:"attempt_id"`
	Provider           ProviderBindingRefV2     `json:"provider_binding"`
	PayloadSchema      SchemaRefV2              `json:"payload_schema"`
	PayloadDigest      core.Digest              `json:"payload_digest"`
	PayloadRevision    core.Revision            `json:"payload_revision"`
	PreparedUnixNano   int64                    `json:"prepared_unix_nano"`
	ExpiresUnixNano    int64                    `json:"expires_unix_nano"`
}

func (r PreparedProviderAttemptRefV2) Validate() error {
	return r.validateV2(false)
}

func (r PreparedProviderAttemptRefV2) validateV2(allowPendingDigest bool) error {
	if validateEvidenceIDV2(r.ID) != nil || r.Revision == 0 || validateEvidenceIDV2(string(r.IntentID)) != nil || r.IntentRevision == 0 || validateEvidenceIDV2(r.PermitID) != nil || r.PermitRevision == 0 || validateEvidenceIDV2(r.AttemptID) != nil || r.PayloadRevision == 0 || r.PreparedUnixNano <= 0 || r.ExpiresUnixNano <= r.PreparedUnixNano {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "prepared provider attempt ref is incomplete")
	}
	if err := r.DeclaredDelegation.Validate(); err != nil {
		return err
	}
	expectedID, err := DerivePreparedProviderAttemptIDV2(r.DeclaredDelegation.ID, r.PermitID, r.AttemptID)
	if err != nil || expectedID != r.ID {
		return core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "prepared attempt ID is not the canonical preallocated identity")
	}
	digests := []core.Digest{r.OperationDigest, r.IntentDigest, r.PermitDigest, r.PayloadDigest}
	if !allowPendingDigest {
		digests = append(digests, r.Digest)
	}
	for _, digest := range digests {
		if err := digest.Validate(); err != nil {
			return err
		}
	}
	if err := r.PayloadSchema.Validate(); err != nil {
		return err
	}
	if err := r.Provider.Validate(); err != nil {
		return err
	}
	if !allowPendingDigest {
		digest, err := r.DigestV2()
		if err != nil || digest != r.Digest {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidDigest, "prepared attempt digest drifted")
		}
	}
	return nil
}

func (r PreparedProviderAttemptRefV2) DigestV2() (core.Digest, error) {
	r.Digest = ""
	if err := r.validateV2(true); err != nil {
		return "", err
	}
	return core.CanonicalJSONDigest("praxis.runtime.execution-governance", ExecutionGovernanceContractVersionV2, "PreparedProviderAttemptRefV2", r)

}

func SealPreparedProviderAttemptRefV2(r PreparedProviderAttemptRefV2) (PreparedProviderAttemptRefV2, error) {
	digest, err := r.DigestV2()
	if err != nil {
		return PreparedProviderAttemptRefV2{}, err
	}
	r.Digest = digest
	return r, r.Validate()
}

type PersistedOperationEnforcementRefV3 struct {
	PermitID         string               `json:"permit_id"`
	PermitRevision   core.Revision        `json:"permit_revision"`
	PermitDigest     core.Digest          `json:"permit_digest"`
	AttemptID        string               `json:"attempt_id"`
	OperationDigest  core.Digest          `json:"operation_digest"`
	Provider         ProviderBindingRefV2 `json:"provider_binding"`
	ReceiptDigest    core.Digest          `json:"receipt_digest"`
	RecordedRevision core.Revision        `json:"recorded_revision"`
}

func (r PersistedOperationEnforcementRefV3) Validate() error {
	if validateEvidenceIDV2(r.PermitID) != nil || r.PermitRevision == 0 || validateEvidenceIDV2(r.AttemptID) != nil || r.RecordedRevision == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonDispatchPermitInvalid, "persisted enforcement ref is incomplete")
	}
	for _, digest := range []core.Digest{r.PermitDigest, r.OperationDigest, r.ReceiptDigest} {
		if err := digest.Validate(); err != nil {
			return err
		}
	}
	return r.Provider.Validate()
}

type PrepareGovernedExecutionRequestV2 struct {
	Delegation ExecutionDelegationRefV2  `json:"delegation"`
	Intent     OperationEffectIntentV3   `json:"intent"`
	Permit     OperationDispatchPermitV3 `json:"permit"`
	Fence      core.ExecutionFence       `json:"fence"`
}

func (r PrepareGovernedExecutionRequestV2) Validate() error {
	if err := r.Delegation.Validate(); err != nil {
		return err
	}
	if err := r.Intent.Validate(); err != nil {
		return err
	}
	if err := r.Permit.Validate(); err != nil {
		return err
	}
	if err := r.Fence.Validate(); err != nil {
		return err
	}
	intentDigest, err := r.Intent.DigestV3()
	if err != nil {
		return err
	}
	fenceDigest, err := DigestOperationExecutionFenceV3(r.Fence, r.Intent.Operation)
	if err != nil || r.Permit.IntentID != r.Intent.ID || r.Permit.IntentRevision != r.Intent.Revision || r.Permit.IntentDigest != intentDigest || !SameOperationSubjectV3(r.Permit.Operation, r.Intent.Operation) || r.Permit.PayloadSchema != r.Intent.Payload.Schema || r.Permit.PayloadDigest != r.Intent.Payload.ContentDigest || r.Permit.PayloadRevision != r.Intent.PayloadRevision || r.Permit.Provider != r.Intent.Provider || r.Permit.AttemptID == "" || fenceDigest != r.Permit.FenceDigest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonDispatchPermitInvalid, "prepare request does not bind one exact intent, operation, payload, provider, Permit and Fence")
	}
	return nil
}

func (r PrepareGovernedExecutionRequestV2) ValidateAgainstDelegation(f ExecutionDelegationFactV2, now time.Time) error {
	if err := r.Validate(); err != nil {
		return err
	}
	ref, err := f.RefV2()
	if err != nil || ref != r.Delegation || f.State != ExecutionDelegationDeclaredV2 || f.IntentID != r.Intent.ID || f.IntentRevision != r.Intent.Revision || f.IntentDigest != r.Permit.IntentDigest || !SameOperationSubjectV3(f.Operation, r.Intent.Operation) || f.PayloadSchema != r.Intent.Payload.Schema || f.PayloadDigest != r.Intent.Payload.ContentDigest || f.PayloadRevision != r.Intent.PayloadRevision || f.ProviderPermitID != r.Permit.ID || f.ProviderPermitRevision != r.Permit.Revision || f.ProviderPermitDigest != mustOperationPermitDigestV3(r.Permit) || f.ProviderAttemptID != r.Permit.AttemptID || f.DataProvider != r.Permit.Provider || now.IsZero() || !now.Before(time.Unix(0, f.ExpiresUnixNano)) || !now.Before(time.Unix(0, r.Permit.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonProviderBindingStale, "prepare request does not match the current exact delegation/provider attempt")
	}
	return nil
}

type OperationDispatchCurrentProjectionV3 struct {
	Operation          OperationSubjectV3                  `json:"operation_subject"`
	Permit             OperationDispatchPermitV3           `json:"permit"`
	PermitDigest       core.Digest                         `json:"permit_digest"`
	PermitFactRevision core.Revision                       `json:"permit_fact_revision"`
	PermitFactState    string                              `json:"permit_fact_state"`
	Enforcement        *PersistedOperationEnforcementRefV3 `json:"persisted_enforcement,omitempty"`
	Delegation         ExecutionDelegationRefV2            `json:"delegation"`
	DelegationState    ExecutionDelegationStateV2          `json:"delegation_state"`
	PreparedAttemptID  string                              `json:"prepared_attempt_id"`
	PreparationDigest  core.Digest                         `json:"preparation_digest,omitempty"`
	ExpiresUnixNano    int64                               `json:"expires_unix_nano"`
}

func (p OperationDispatchCurrentProjectionV3) ValidateForPrepare(request PrepareGovernedExecutionRequestV2, current OperationGovernanceSnapshotV3, now time.Time) error {
	if err := request.Validate(); err != nil {
		return err
	}
	permitDigest, err := request.Permit.DigestV3()
	expectedPreparedID, preparedErr := DerivePreparedProviderAttemptIDV2(request.Delegation.ID, request.Permit.ID, request.Permit.AttemptID)
	if err != nil || preparedErr != nil || !SameOperationSubjectV3(p.Operation, request.Intent.Operation) || p.PermitDigest != permitDigest || p.Permit.ID != request.Permit.ID || p.Permit.Revision != request.Permit.Revision || p.PermitFactRevision < 2 || p.PermitFactState != "begun" || p.Delegation != request.Delegation || p.DelegationState != ExecutionDelegationDeclaredV2 || p.PreparedAttemptID != expectedPreparedID || p.Enforcement != nil || p.ExpiresUnixNano <= 0 || now.IsZero() || !now.Before(time.Unix(0, p.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonDispatchPermitConsumed, "provider Prepare current Permit/delegation projection is stale or forged")
	}
	return ValidateOperationAtExecutionPointV3(request.Permit, request.Intent, request.Fence, current, now)
}

func (p OperationDispatchCurrentProjectionV3) ValidateForExecute(request ExecutePreparedRequestV2, current OperationGovernanceSnapshotV3, now time.Time) error {
	if err := request.Validate(); err != nil {
		return err
	}
	if p.PermitDigest != request.Prepared.PermitDigest || p.Permit.ID != request.Prepared.PermitID || p.Permit.Revision != request.Prepared.PermitRevision || p.Permit.AttemptID != request.Prepared.AttemptID || p.PermitFactRevision < 3 || p.PermitFactState != "begun" || p.Enforcement == nil || *p.Enforcement != request.Enforcement || p.Delegation != request.Delegation || p.DelegationState != ExecutionDelegationPreparedV2 || p.PreparedAttemptID != request.Prepared.ID || p.PreparationDigest.Validate() != nil || p.ExpiresUnixNano <= 0 || now.IsZero() || !now.Before(time.Unix(0, p.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonDispatchPermitInvalid, "ExecutePrepared current Permit, Enforcement, Delegation or Prepared attempt drifted")
	}
	return ValidateOperationAtExecutionPointV3(request.Permit, request.Intent, request.Fence, current, now)
}

type OperationDispatchCurrentReaderV3 interface {
	// InspectOperationDispatch reconstructs current Permit, Enforcement and
	// Delegation watermarks from their Fact Owners. Caller strings/refs cannot
	// authorize provider contact.
	InspectOperationDispatch(context.Context, OperationSubjectV3, string, string) (OperationDispatchCurrentProjectionV3, error)
}

type InspectPreparedProviderRequestV2 struct {
	DeclaredDelegation ExecutionDelegationRefV2 `json:"declared_delegation"`
	PreparedAttemptID  string                   `json:"prepared_attempt_id"`
	PermitID           string                   `json:"permit_id"`
	AttemptID          string                   `json:"attempt_id"`
}

func (r InspectPreparedProviderRequestV2) Validate() error {
	if err := r.DeclaredDelegation.Validate(); err != nil {
		return err
	}
	if validateEvidenceIDV2(r.PreparedAttemptID) != nil || validateEvidenceIDV2(r.PermitID) != nil || validateEvidenceIDV2(r.AttemptID) != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "prepared inspection key is incomplete")
	}
	expected, err := DerivePreparedProviderAttemptIDV2(r.DeclaredDelegation.ID, r.PermitID, r.AttemptID)
	if err != nil || expected != r.PreparedAttemptID {
		return core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "prepared inspection key drifted")
	}
	return nil
}

func mustOperationPermitDigestV3(permit OperationDispatchPermitV3) core.Digest {
	digest, _ := permit.DigestV3()
	return digest
}

type ProviderPreparationAttestationV2 struct {
	ContractVersion  string                        `json:"contract_version"`
	Delegation       ExecutionDelegationRefV2      `json:"delegation"`
	Prepared         PreparedProviderAttemptRefV2  `json:"prepared_attempt"`
	Enforcement      OperationEnforcementReceiptV3 `json:"enforcement_receipt"`
	ObservedUnixNano int64                         `json:"observed_unix_nano"`
}

func (a ProviderPreparationAttestationV2) Validate() error {
	if a.ContractVersion != ExecutionGovernanceContractVersionV2 || a.ObservedUnixNano <= 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonDispatchPermitInvalid, "provider preparation attestation is incomplete")
	}
	if err := a.Delegation.Validate(); err != nil {
		return err
	}
	if err := a.Prepared.Validate(); err != nil {
		return err
	}
	if err := a.Enforcement.Validate(); err != nil {
		return err
	}
	if a.Prepared.DeclaredDelegation != a.Delegation || a.Prepared.PermitID != a.Enforcement.PermitID || a.Prepared.PermitRevision != a.Enforcement.PermitRevision || a.Prepared.PermitDigest != a.Enforcement.PermitDigest || a.Prepared.AttemptID != a.Enforcement.AttemptID || a.Prepared.Provider != a.Enforcement.Verifier || a.Prepared.OperationDigest != mustOperationSubjectDigestV3(a.Enforcement.Operation) || a.ObservedUnixNano < a.Prepared.PreparedUnixNano || a.ObservedUnixNano < a.Enforcement.ValidatedUnixNano || a.ObservedUnixNano >= a.Prepared.ExpiresUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonDispatchPermitInvalid, "preparation attestation does not bind exact delegation, prepared attempt and enforcement")
	}
	return nil
}

func (a ProviderPreparationAttestationV2) ValidateAgainstPrepare(request PrepareGovernedExecutionRequestV2, now time.Time) error {
	if err := request.Validate(); err != nil {
		return err
	}
	if err := a.Validate(); err != nil {
		return err
	}
	operationDigest, _ := request.Intent.Operation.DigestV3()
	permitDigest, _ := request.Permit.DigestV3()
	if a.Delegation != request.Delegation || a.Prepared.DeclaredDelegation != request.Delegation || a.Prepared.OperationDigest != operationDigest || a.Prepared.IntentID != request.Intent.ID || a.Prepared.IntentRevision != request.Intent.Revision || a.Prepared.IntentDigest != request.Permit.IntentDigest || a.Prepared.PermitID != request.Permit.ID || a.Prepared.PermitRevision != request.Permit.Revision || a.Prepared.PermitDigest != permitDigest || a.Prepared.AttemptID != request.Permit.AttemptID || a.Prepared.Provider != request.Permit.Provider || a.Prepared.PayloadSchema != request.Intent.Payload.Schema || a.Prepared.PayloadDigest != request.Intent.Payload.ContentDigest || a.Prepared.PayloadRevision != request.Intent.PayloadRevision || a.Enforcement.PermitID != request.Permit.ID || a.Enforcement.PermitRevision != request.Permit.Revision || a.Enforcement.PermitDigest != permitDigest || a.Enforcement.AttemptID != request.Permit.AttemptID || !SameOperationSubjectV3(a.Enforcement.Operation, request.Intent.Operation) || a.Enforcement.Verifier != request.Permit.EnforcementPoint || now.IsZero() || a.ObservedUnixNano > now.UnixNano() || !now.Before(time.Unix(0, request.Permit.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonDispatchPermitInvalid, "provider preparation changed exact delegation, operation, payload, Permit, attempt or provider")
	}
	return nil
}

func mustOperationSubjectDigestV3(subject OperationSubjectV3) core.Digest {
	digest, _ := subject.DigestV3()
	return digest
}

type ExecutePreparedRequestV2 struct {
	Delegation  ExecutionDelegationRefV2           `json:"delegation"`
	Prepared    PreparedProviderAttemptRefV2       `json:"prepared_attempt"`
	Enforcement PersistedOperationEnforcementRefV3 `json:"persisted_enforcement"`
	Intent      OperationEffectIntentV3            `json:"intent"`
	Permit      OperationDispatchPermitV3          `json:"permit"`
	Fence       core.ExecutionFence                `json:"fence"`
}

func (r ExecutePreparedRequestV2) Validate() error {
	if err := r.Delegation.Validate(); err != nil {
		return err
	}
	if err := r.Prepared.Validate(); err != nil {
		return err
	}
	if err := r.Enforcement.Validate(); err != nil {
		return err
	}
	if err := r.Intent.Validate(); err != nil {
		return err
	}
	if err := r.Permit.Validate(); err != nil {
		return err
	}
	if err := r.Fence.Validate(); err != nil {
		return err
	}
	intentDigest, _ := r.Intent.DigestV3()
	permitDigest, _ := r.Permit.DigestV3()
	operationDigest, _ := r.Intent.Operation.DigestV3()
	if r.Delegation.ID != r.Prepared.DeclaredDelegation.ID || r.Delegation.Revision <= r.Prepared.DeclaredDelegation.Revision || r.Enforcement.PermitID != r.Prepared.PermitID || r.Enforcement.PermitRevision != r.Prepared.PermitRevision || r.Enforcement.PermitDigest != r.Prepared.PermitDigest || r.Enforcement.AttemptID != r.Prepared.AttemptID || r.Enforcement.OperationDigest != r.Prepared.OperationDigest || r.Enforcement.Provider != r.Prepared.Provider || r.Intent.ID != r.Prepared.IntentID || r.Intent.Revision != r.Prepared.IntentRevision || intentDigest != r.Prepared.IntentDigest || operationDigest != r.Prepared.OperationDigest || r.Permit.ID != r.Prepared.PermitID || r.Permit.Revision != r.Prepared.PermitRevision || permitDigest != r.Prepared.PermitDigest || r.Permit.AttemptID != r.Prepared.AttemptID || r.Permit.Provider != r.Prepared.Provider {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonDispatchPermitInvalid, "execute request changed delegation, operation, provider, Permit, attempt or persisted enforcement")
	}
	return nil
}

func (r ExecutePreparedRequestV2) ValidateAgainstDelegation(f ExecutionDelegationFactV2, now time.Time) error {
	if err := r.Validate(); err != nil {
		return err
	}
	ref, err := f.RefV2()
	if err != nil || ref != r.Delegation || f.State != ExecutionDelegationPreparedV2 || r.Prepared.IntentID != f.IntentID || r.Prepared.IntentRevision != f.IntentRevision || r.Prepared.IntentDigest != f.IntentDigest || r.Prepared.OperationDigest != mustOperationSubjectDigestV3(f.Operation) || r.Prepared.PayloadSchema != f.PayloadSchema || r.Prepared.PayloadDigest != f.PayloadDigest || r.Prepared.PayloadRevision != f.PayloadRevision || r.Prepared.PermitID != f.ProviderPermitID || r.Prepared.PermitRevision != f.ProviderPermitRevision || r.Prepared.PermitDigest != f.ProviderPermitDigest || r.Prepared.AttemptID != f.ProviderAttemptID || r.Prepared.Provider != f.DataProvider || now.IsZero() || !now.Before(time.Unix(0, f.ExpiresUnixNano)) || !now.Before(time.Unix(0, r.Prepared.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonProviderBindingStale, "execute request does not match current prepared delegation")
	}
	return nil
}

type InspectLocalProviderAttemptRequestV2 struct {
	Delegation ExecutionDelegationRefV2     `json:"delegation"`
	Prepared   PreparedProviderAttemptRefV2 `json:"prepared_attempt"`
}

func (r InspectLocalProviderAttemptRequestV2) Validate() error {
	if err := r.Delegation.Validate(); err != nil {
		return err
	}
	if err := r.Prepared.Validate(); err != nil {
		return err
	}
	if r.Delegation.ID != r.Prepared.DeclaredDelegation.ID || r.Delegation.Revision <= r.Prepared.DeclaredDelegation.Revision {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidReference, "inspect request binds another delegation")
	}
	return nil
}

type ProviderAttemptStateV2 string

const (
	ProviderAttemptPreparedV2  ProviderAttemptStateV2 = "prepared"
	ProviderAttemptExecutingV2 ProviderAttemptStateV2 = "executing"
	ProviderAttemptObservedV2  ProviderAttemptStateV2 = "observed"
	ProviderAttemptUnknownV2   ProviderAttemptStateV2 = "unknown"
)

type ProviderAttemptObservationV2 struct {
	ContractVersion      string                       `json:"contract_version"`
	Delegation           ExecutionDelegationRefV2     `json:"delegation"`
	Prepared             PreparedProviderAttemptRefV2 `json:"prepared_attempt"`
	Revision             core.Revision                `json:"revision"`
	State                ProviderAttemptStateV2       `json:"state"`
	Payload              OpaquePayloadV2              `json:"payload"`
	PayloadRevision      core.Revision                `json:"payload_revision"`
	ProviderOperationRef string                       `json:"provider_operation_ref"`
	SourceRegistrationID string                       `json:"source_registration_id"`
	SourceEpoch          core.Epoch                   `json:"source_epoch"`
	SourceSequence       uint64                       `json:"source_sequence"`
	Evidence             EvidenceRecordRefV2          `json:"evidence"`
	ObservedUnixNano     int64                        `json:"observed_unix_nano"`
}

type ProviderAttemptObservationRefV2 struct {
	Delegation           ExecutionDelegationRefV2 `json:"delegation"`
	PreparedAttemptID    string                   `json:"prepared_attempt_id"`
	ProviderOperationRef string                   `json:"provider_operation_ref"`
	Revision             core.Revision            `json:"revision"`
	State                ProviderAttemptStateV2   `json:"state"`
	Digest               core.Digest              `json:"digest"`
	PayloadDigest        core.Digest              `json:"payload_digest"`
	PayloadRevision      core.Revision            `json:"payload_revision"`
	SourceRegistrationID string                   `json:"source_registration_id"`
	SourceEpoch          core.Epoch               `json:"source_epoch"`
	SourceSequence       uint64                   `json:"source_sequence"`
	Evidence             EvidenceRecordRefV2      `json:"evidence"`
	ObservedUnixNano     int64                    `json:"observed_unix_nano"`
}

func (r ProviderAttemptObservationRefV2) Validate() error {
	if err := r.Delegation.Validate(); err != nil {
		return err
	}
	if validateEvidenceIDV2(r.PreparedAttemptID) != nil || validateEvidenceIDV2(r.ProviderOperationRef) != nil || r.Revision == 0 || r.Digest.Validate() != nil || r.PayloadDigest.Validate() != nil || r.PayloadRevision == 0 || validateEvidenceIDV2(r.SourceRegistrationID) != nil || r.SourceEpoch == 0 || r.SourceSequence == 0 || r.ObservedUnixNano <= 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonEvidenceConflict, "provider observation ref is incomplete")
	}
	if r.State != ProviderAttemptPreparedV2 && r.State != ProviderAttemptExecutingV2 && r.State != ProviderAttemptObservedV2 && r.State != ProviderAttemptUnknownV2 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "provider observation ref state is invalid")
	}
	return r.Evidence.Validate()
}

func (o ProviderAttemptObservationV2) RefV2() (ProviderAttemptObservationRefV2, error) {
	if err := o.Validate(); err != nil {
		return ProviderAttemptObservationRefV2{}, err
	}
	digest, err := core.CanonicalJSONDigest("praxis.runtime.execution-governance", ExecutionGovernanceContractVersionV2, "ProviderAttemptObservationV2", o)
	if err != nil {
		return ProviderAttemptObservationRefV2{}, err
	}
	return ProviderAttemptObservationRefV2{Delegation: o.Delegation, PreparedAttemptID: o.Prepared.ID, ProviderOperationRef: o.ProviderOperationRef, Revision: o.Revision, State: o.State, Digest: digest, PayloadDigest: o.Payload.ContentDigest, PayloadRevision: o.PayloadRevision, SourceRegistrationID: o.SourceRegistrationID, SourceEpoch: o.SourceEpoch, SourceSequence: o.SourceSequence, Evidence: o.Evidence, ObservedUnixNano: o.ObservedUnixNano}, nil
}

// GovernedExecutionAttemptRefsV2 is safe for Harness Session/Turn facts to
// embed. It contains only public immutable refs, never control/fake handles.
type GovernedExecutionAttemptRefsV2 struct {
	Admission      OperationEffectAdmissionReceiptV3  `json:"admission"`
	PermitID       string                             `json:"permit_id"`
	PermitRevision core.Revision                      `json:"permit_revision"`
	PermitDigest   core.Digest                        `json:"permit_digest"`
	AttemptID      string                             `json:"attempt_id"`
	Delegation     ExecutionDelegationRefV2           `json:"delegation"`
	Prepared       PreparedProviderAttemptRefV2       `json:"prepared"`
	Enforcement    PersistedOperationEnforcementRefV3 `json:"enforcement"`
	Observation    *ProviderAttemptObservationRefV2   `json:"observation,omitempty"`
	Settlement     *OperationSettlementRefV3          `json:"settlement,omitempty"`
}

func (r GovernedExecutionAttemptRefsV2) ValidatePrepared() error {
	if err := r.Admission.Validate(); err != nil {
		return err
	}
	if validateEvidenceIDV2(r.PermitID) != nil || r.PermitRevision == 0 || r.PermitDigest.Validate() != nil || validateEvidenceIDV2(r.AttemptID) != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonDispatchPermitInvalid, "governed execution Permit ref is incomplete")
	}
	if err := r.Delegation.Validate(); err != nil {
		return err
	}
	if err := r.Prepared.Validate(); err != nil {
		return err
	}
	if err := r.Enforcement.Validate(); err != nil {
		return err
	}
	if r.Admission.OperationDigest != r.Prepared.OperationDigest || r.Admission.EffectID != r.Prepared.IntentID || r.Admission.IntentRevision != r.Prepared.IntentRevision || r.Admission.IntentDigest != r.Prepared.IntentDigest || r.Prepared.PermitID != r.PermitID || r.Prepared.PermitRevision != r.PermitRevision || r.Prepared.PermitDigest != r.PermitDigest || r.Prepared.AttemptID != r.AttemptID || r.Enforcement.PermitID != r.PermitID || r.Enforcement.PermitRevision != r.PermitRevision || r.Enforcement.PermitDigest != r.PermitDigest || r.Enforcement.AttemptID != r.AttemptID || r.Enforcement.OperationDigest != r.Prepared.OperationDigest || r.Delegation.ID != r.Prepared.DeclaredDelegation.ID || r.Delegation.Revision <= r.Prepared.DeclaredDelegation.Revision {
		return core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "governed execution refs do not describe one exact attempt")
	}
	if r.Observation != nil {
		if r.Observation.Validate() != nil || r.Observation.Delegation != r.Delegation || r.Observation.PreparedAttemptID != r.Prepared.ID {
			return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "provider observation ref belongs to another governed execution attempt")
		}
	}
	if r.Settlement != nil {
		if err := r.Settlement.Validate(); err != nil {
			return err
		}
		attempt := r.Settlement.Attempt
		if attempt.OperationDigest != r.Admission.OperationDigest || attempt.EffectID != r.Admission.EffectID || attempt.IntentRevision != r.Admission.IntentRevision || attempt.IntentDigest != r.Admission.IntentDigest || attempt.PermitID != r.PermitID || attempt.PermitRevision != r.PermitRevision || attempt.PermitDigest != r.PermitDigest || attempt.AttemptID != r.AttemptID || attempt.Delegation == nil || *attempt.Delegation != r.Delegation {
			return core.NewError(core.ErrorConflict, core.ReasonSettlementOwnerMismatch, "operation settlement belongs to another governed execution attempt")
		}
		if (r.Observation == nil) != (r.Settlement.Observation == nil) || r.Observation != nil && *r.Observation != *r.Settlement.Observation {
			return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "operation settlement does not bind the exact provider observation")
		}
		if r.Observation == nil && (r.Settlement.InspectionEffect == nil || r.Settlement.InspectionSettlement == nil) {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectUnknownOutcome, "post-prepared unknown settlement requires exact Inspect provenance")
		}
	}
	return nil
}

type OperationDispatchAttemptRefV3 struct {
	OperationDigest core.Digest               `json:"operation_digest"`
	EffectID        core.EffectIntentID       `json:"effect_id"`
	IntentRevision  core.Revision             `json:"intent_revision"`
	IntentDigest    core.Digest               `json:"intent_digest"`
	PermitID        string                    `json:"permit_id"`
	PermitRevision  core.Revision             `json:"permit_revision"`
	PermitDigest    core.Digest               `json:"permit_digest"`
	AttemptID       string                    `json:"attempt_id"`
	Delegation      *ExecutionDelegationRefV2 `json:"delegation,omitempty"`
}

// IssueGovernedOperationDispatchRequestV3 and
// BeginGovernedOperationDispatchRequestV3 are Application-facing commands.
// They never expose the raw Effect/Permit Fact Owner transitions.
type IssueGovernedOperationDispatchRequestV3 struct {
	Operation              OperationSubjectV3  `json:"operation_subject"`
	EffectID               core.EffectIntentID `json:"effect_id"`
	ExpectedEffectRevision core.Revision       `json:"expected_effect_revision"`
	PermitID               string              `json:"permit_id"`
	AttemptID              string              `json:"attempt_id"`
	PermitTTL              time.Duration       `json:"permit_ttl"`
}

type BeginGovernedOperationDispatchRequestV3 struct {
	Operation              OperationSubjectV3  `json:"operation_subject"`
	EffectID               core.EffectIntentID `json:"effect_id"`
	ExpectedEffectRevision core.Revision       `json:"expected_effect_revision"`
	PermitID               string              `json:"permit_id"`
	ExpectedPermitRevision core.Revision       `json:"expected_permit_revision"`
}

func (r BeginGovernedOperationDispatchRequestV3) Validate() error {
	if err := r.Operation.Validate(); err != nil {
		return err
	}
	if validateEvidenceIDV2(string(r.EffectID)) != nil || r.ExpectedEffectRevision == 0 || validateEvidenceIDV2(r.PermitID) != nil || r.ExpectedPermitRevision == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonDispatchPermitInvalid, "operation Begin requires exact Effect and Permit revisions")
	}
	return nil
}

type MarkOperationDispatchUnknownRequestV3 struct {
	Operation              OperationSubjectV3            `json:"operation_subject"`
	EffectID               core.EffectIntentID           `json:"effect_id"`
	ExpectedEffectRevision core.Revision                 `json:"expected_effect_revision"`
	Permit                 OperationDispatchAttemptRefV3 `json:"permit_attempt"`
}

func (r MarkOperationDispatchUnknownRequestV3) Validate() error {
	if err := r.Operation.Validate(); err != nil {
		return err
	}
	if validateEvidenceIDV2(string(r.EffectID)) != nil || r.ExpectedEffectRevision == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonDispatchPermitInvalid, "unknown outcome requires an exact Effect revision")
	}
	if err := r.Permit.Validate(); err != nil {
		return err
	}
	operationDigest, err := r.Operation.DigestV3()
	if err != nil || r.Permit.OperationDigest != operationDigest || r.Permit.EffectID != r.EffectID {
		return core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "unknown outcome Permit attempt belongs to another operation Effect")
	}
	return nil
}

type OperationDispatchAuthorizationStateV3 string

const (
	OperationDispatchAuthorizationIssuedV3  OperationDispatchAuthorizationStateV3 = "issued"
	OperationDispatchAuthorizationBegunV3   OperationDispatchAuthorizationStateV3 = "begun"
	OperationDispatchAuthorizationUnknownV3 OperationDispatchAuthorizationStateV3 = "unknown_outcome"
)

type OperationDispatchAuthorizationV3 struct {
	Attempt            OperationDispatchAttemptRefV3         `json:"attempt"`
	Permit             OperationDispatchPermitV3             `json:"permit"`
	EffectFactRevision core.Revision                         `json:"effect_fact_revision"`
	PermitFactRevision core.Revision                         `json:"permit_fact_revision"`
	State              OperationDispatchAuthorizationStateV3 `json:"state"`
	Fence              core.ExecutionFence                   `json:"fence"`
	ExpiresUnixNano    int64                                 `json:"expires_unix_nano"`
}

func (a OperationDispatchAuthorizationV3) Validate() error {
	if err := a.Attempt.Validate(); err != nil {
		return err
	}
	if a.EffectFactRevision == 0 || a.PermitFactRevision == 0 || a.ExpiresUnixNano <= 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonDispatchPermitInvalid, "operation dispatch authorization watermarks are incomplete")
	}
	if err := a.Permit.Validate(); err != nil {
		return err
	}
	permitDigest, err := a.Permit.DigestV3()
	if err != nil || a.Attempt.PermitID != a.Permit.ID || a.Attempt.PermitRevision != a.Permit.Revision || a.Attempt.PermitDigest != permitDigest || a.Attempt.AttemptID != a.Permit.AttemptID || a.Attempt.EffectID != a.Permit.IntentID || a.Attempt.IntentRevision != a.Permit.IntentRevision || a.Attempt.IntentDigest != a.Permit.IntentDigest || a.ExpiresUnixNano != a.Permit.ExpiresUnixNano {
		return core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "operation dispatch authorization does not bind one exact immutable Permit")
	}
	operationDigest, err := a.Permit.Operation.DigestV3()
	if err != nil || operationDigest != a.Attempt.OperationDigest {
		return core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "operation dispatch authorization subject drifted")
	}
	fenceDigest, err := DigestOperationExecutionFenceV3(a.Fence, a.Permit.Operation)
	if err != nil || fenceDigest != a.Permit.FenceDigest || !a.Fence.ExpiresAt.Equal(time.Unix(0, a.ExpiresUnixNano)) {
		return core.NewError(core.ErrorConflict, core.ReasonEffectFenceStale, "operation dispatch authorization Fence drifted")
	}
	if a.State != OperationDispatchAuthorizationIssuedV3 && a.State != OperationDispatchAuthorizationBegunV3 && a.State != OperationDispatchAuthorizationUnknownV3 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonDispatchPermitInvalid, "operation dispatch authorization state is invalid")
	}
	return nil
}

// OperationGovernancePortV3 is the only Application-facing dispatch state
// machine. Raw Effect Fact Store methods are Fact Owner implementation detail.
type OperationGovernancePortV3 interface {
	IssueOperationDispatchV3(context.Context, IssueGovernedOperationDispatchRequestV3) (OperationDispatchAuthorizationV3, error)
	BeginOperationDispatchV3(context.Context, BeginGovernedOperationDispatchRequestV3) (OperationDispatchAuthorizationV3, error)
	MarkOperationDispatchUnknownV3(context.Context, MarkOperationDispatchUnknownRequestV3) (OperationDispatchAuthorizationV3, error)
	InspectOperationDispatchAuthorizationV3(context.Context, OperationSubjectV3, core.EffectIntentID, string) (OperationDispatchAuthorizationV3, error)
}

func (r OperationDispatchAttemptRefV3) Validate() error {
	if r.OperationDigest.Validate() != nil || r.IntentDigest.Validate() != nil || r.PermitDigest.Validate() != nil || validateEvidenceIDV2(string(r.EffectID)) != nil || r.IntentRevision == 0 || validateEvidenceIDV2(r.PermitID) != nil || r.PermitRevision == 0 || validateEvidenceIDV2(r.AttemptID) != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonDispatchPermitInvalid, "operation dispatch attempt ref is incomplete")
	}
	if r.Delegation != nil {
		return r.Delegation.Validate()
	}
	return nil
}

type OperationSettlementDispositionV3 string

const (
	OperationSettlementAppliedV3    OperationSettlementDispositionV3 = "confirmed_applied"
	OperationSettlementNotAppliedV3 OperationSettlementDispositionV3 = "confirmed_not_applied"
	OperationSettlementFailedV3     OperationSettlementDispositionV3 = "confirmed_failed"
)

type OperationSettlementRefV3 struct {
	ID                   string                              `json:"id"`
	Revision             core.Revision                       `json:"revision"`
	Digest               core.Digest                         `json:"digest"`
	Attempt              OperationDispatchAttemptRefV3       `json:"attempt"`
	Disposition          OperationSettlementDispositionV3    `json:"disposition"`
	Owner                EffectOwnerRefV2                    `json:"owner"`
	Observation          *ProviderAttemptObservationRefV2    `json:"observation,omitempty"`
	InspectionEffect     *OperationDispatchAttemptRefV3      `json:"inspection_effect,omitempty"`
	InspectionSettlement *OperationInspectionSettlementRefV3 `json:"inspection_settlement,omitempty"`
	Evidence             []EvidenceRecordRefV2               `json:"evidence"`
	DomainResultSchema   *SchemaRefV2                        `json:"domain_result_schema,omitempty"`
	DomainResultDigest   core.Digest                         `json:"domain_result_digest,omitempty"`
}

// OperationInspectionSettlementRefV3 is deliberately non-recursive. An
// Inspect Effect may settle an original unknown attempt, but that provenance
// cannot itself form an unbounded/self-referential settlement chain.
type OperationInspectionSettlementRefV3 struct {
	ID                 string                           `json:"id"`
	Revision           core.Revision                    `json:"revision"`
	Digest             core.Digest                      `json:"digest"`
	Attempt            OperationDispatchAttemptRefV3    `json:"attempt"`
	Disposition        OperationSettlementDispositionV3 `json:"disposition"`
	Owner              EffectOwnerRefV2                 `json:"owner"`
	Observation        *ProviderAttemptObservationRefV2 `json:"observation,omitempty"`
	Evidence           []EvidenceRecordRefV2            `json:"evidence"`
	DomainResultSchema *SchemaRefV2                     `json:"domain_result_schema,omitempty"`
	DomainResultDigest core.Digest                      `json:"domain_result_digest,omitempty"`
}

func (r OperationInspectionSettlementRefV3) Validate() error {
	flat := OperationSettlementRefV3{ID: r.ID, Revision: r.Revision, Digest: r.Digest, Attempt: r.Attempt, Disposition: r.Disposition, Owner: r.Owner, Observation: r.Observation, Evidence: r.Evidence, DomainResultSchema: r.DomainResultSchema, DomainResultDigest: r.DomainResultDigest}
	return flat.Validate()
}

func (r OperationSettlementRefV3) InspectionRefV3() (OperationInspectionSettlementRefV3, error) {
	if r.InspectionEffect != nil || r.InspectionSettlement != nil {
		return OperationInspectionSettlementRefV3{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectUnknownOutcome, "nested unknown-outcome inspection settlement is forbidden")
	}
	if err := r.Validate(); err != nil {
		return OperationInspectionSettlementRefV3{}, err
	}
	ref := OperationInspectionSettlementRefV3{ID: r.ID, Revision: r.Revision, Digest: r.Digest, Attempt: r.Attempt, Disposition: r.Disposition, Owner: r.Owner, Observation: r.Observation, Evidence: append([]EvidenceRecordRefV2{}, r.Evidence...), DomainResultSchema: r.DomainResultSchema, DomainResultDigest: r.DomainResultDigest}
	return ref, ref.Validate()
}

func (r OperationSettlementRefV3) Validate() error {
	if validateEvidenceIDV2(r.ID) != nil || r.Revision == 0 || r.Digest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonSettlementOwnerMismatch, "operation settlement ref is incomplete")
	}
	if err := r.Attempt.Validate(); err != nil {
		return err
	}
	if r.Disposition != OperationSettlementAppliedV3 && r.Disposition != OperationSettlementNotAppliedV3 && r.Disposition != OperationSettlementFailedV3 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonSettlementOwnerMismatch, "operation settlement disposition is invalid")
	}
	if r.Owner.Role != OwnerSettlement || ValidateNamespacedNameV2(NamespacedNameV2(r.Owner.ComponentID)) != nil || r.Owner.ManifestDigest.Validate() != nil || len(r.Evidence) == 0 || len(r.Evidence) > 64 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonSettlementOwnerMismatch, "operation settlement owner/evidence is incomplete")
	}
	for _, evidence := range r.Evidence {
		if err := evidence.Validate(); err != nil {
			return err
		}
	}
	if r.Observation != nil {
		if err := r.Observation.Validate(); err != nil {
			return err
		}
		if r.Attempt.Delegation == nil || *r.Attempt.Delegation != r.Observation.Delegation {
			return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "operation settlement observation belongs to another attempt")
		}
	}
	if (r.InspectionEffect == nil) != (r.InspectionSettlement == nil) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectUnknownOutcome, "settlement ref requires both inspection Effect and inspection Settlement")
	}
	if r.Observation != nil && r.InspectionEffect != nil {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "observed settlement cannot also claim unknown-outcome inspection provenance")
	}
	if r.InspectionEffect != nil {
		if err := r.InspectionEffect.Validate(); err != nil {
			return err
		}
		if err := r.InspectionSettlement.Validate(); err != nil {
			return err
		}
		if !sameOperationDispatchAttemptRefPublicV3(*r.InspectionEffect, r.InspectionSettlement.Attempt) {
			return core.NewError(core.ErrorConflict, core.ReasonSettlementOwnerMismatch, "inspection Settlement belongs to another inspection attempt")
		}
	}
	if (r.DomainResultSchema == nil) != (r.DomainResultDigest == "") {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "operation settlement domain result schema and digest must be present together")
	}
	if r.DomainResultSchema != nil {
		if err := r.DomainResultSchema.Validate(); err != nil {
			return err
		}
		if err := r.DomainResultDigest.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func sameOperationDispatchAttemptRefPublicV3(left, right OperationDispatchAttemptRefV3) bool {
	leftDigest, leftErr := core.CanonicalJSONDigest("praxis.runtime.operation-effect", OperationEffectContractVersionV3, "OperationDispatchAttemptRefV3", left)
	rightDigest, rightErr := core.CanonicalJSONDigest("praxis.runtime.operation-effect", OperationEffectContractVersionV3, "OperationDispatchAttemptRefV3", right)
	return leftErr == nil && rightErr == nil && leftDigest == rightDigest
}

type OperationSettlementSubmissionV3 struct {
	ID                   string                              `json:"id"`
	Revision             core.Revision                       `json:"revision"`
	Attempt              OperationDispatchAttemptRefV3       `json:"attempt"`
	Owner                EffectOwnerRefV2                    `json:"owner"`
	Disposition          OperationSettlementDispositionV3    `json:"disposition"`
	Observation          *ProviderAttemptObservationRefV2    `json:"observation,omitempty"`
	InspectionEffect     *OperationDispatchAttemptRefV3      `json:"inspection_effect,omitempty"`
	InspectionSettlement *OperationInspectionSettlementRefV3 `json:"inspection_settlement,omitempty"`
	Evidence             []EvidenceRecordRefV2               `json:"evidence"`
	DomainResult         *OpaquePayloadV2                    `json:"domain_result,omitempty"`
	SettledUnixNano      int64                               `json:"settled_unix_nano"`
}

func (s OperationSettlementSubmissionV3) Validate() error {
	if validateEvidenceIDV2(s.ID) != nil || s.Revision != 1 || s.SettledUnixNano <= 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonSettlementOwnerMismatch, "new operation settlement identity, revision and time are required")
	}
	if err := s.Attempt.Validate(); err != nil {
		return err
	}
	if s.Owner.Role != OwnerSettlement || ValidateNamespacedNameV2(NamespacedNameV2(s.Owner.ComponentID)) != nil || s.Owner.ManifestDigest.Validate() != nil {
		return core.NewError(core.ErrorForbidden, core.ReasonSettlementOwnerMismatch, "operation settlement requires an exact Settlement Owner")
	}
	if s.Disposition != OperationSettlementAppliedV3 && s.Disposition != OperationSettlementNotAppliedV3 && s.Disposition != OperationSettlementFailedV3 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonSettlementOwnerMismatch, "operation settlement disposition is invalid")
	}
	if len(s.Evidence) == 0 || len(s.Evidence) > 64 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceUnavailable, "operation settlement requires bounded exact evidence")
	}
	for _, ref := range s.Evidence {
		if err := ref.Validate(); err != nil {
			return err
		}
	}
	if s.Observation != nil {
		if err := s.Observation.Validate(); err != nil {
			return err
		}
	}
	if (s.InspectionEffect == nil) != (s.InspectionSettlement == nil) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectUnknownOutcome, "unknown outcome settlement requires exact inspection Effect and Settlement")
	}
	if s.InspectionEffect != nil {
		if err := s.InspectionEffect.Validate(); err != nil {
			return err
		}
		if err := s.InspectionSettlement.Validate(); err != nil {
			return err
		}
	}
	if s.DomainResult != nil {
		if err := s.DomainResult.Validate(); err != nil {
			return err
		}
	}
	return nil
}

type OperationSettlementCurrentReaderV3 interface {
	InspectOperationSettlementV3(context.Context, OperationSubjectV3, core.EffectIntentID) (OperationSettlementRefV3, error)
}

type OperationSettlementGovernancePortV3 interface {
	OperationSettlementCurrentReaderV3
	SettleOperationEffectV3(context.Context, OperationEffectIntentV3, OperationSettlementSubmissionV3) (OperationSettlementRefV3, error)
}

// ProviderAttemptObservationFactPortV2 is the create-once Observation owner.
// It never grants Settlement or Runtime Outcome authority.
type ProviderAttemptObservationFactPortV2 interface {
	CreateProviderAttemptObservationV2(context.Context, ProviderAttemptObservationV2) (ProviderAttemptObservationV2, error)
	InspectProviderAttemptObservationV2(context.Context, ExecutionDelegationRefV2, string) (ProviderAttemptObservationV2, error)
}

type DeclareExecutionDelegationRequestV2 struct {
	Delegation ExecutionDelegationFactV2 `json:"delegation"`
	Intent     OperationEffectIntentV3   `json:"intent"`
	Permit     OperationDispatchPermitV3 `json:"permit"`
	Fence      core.ExecutionFence       `json:"fence"`
}

type CommitPreparedExecutionRequestV2 struct {
	Declared    ExecutionDelegationRefV2         `json:"declared_delegation"`
	Intent      OperationEffectIntentV3          `json:"intent"`
	Permit      OperationDispatchPermitV3        `json:"permit"`
	Fence       core.ExecutionFence              `json:"fence"`
	Preparation ProviderPreparationAttestationV2 `json:"preparation"`
}

type PreparedExecutionGovernanceResultV2 struct {
	Delegation  ExecutionDelegationRefV2           `json:"delegation"`
	Prepared    PreparedProviderAttemptRefV2       `json:"prepared_attempt"`
	Enforcement PersistedOperationEnforcementRefV3 `json:"persisted_enforcement"`
}

func (r PreparedExecutionGovernanceResultV2) Validate() error {
	if err := r.Delegation.Validate(); err != nil {
		return err
	}
	if err := r.Prepared.Validate(); err != nil {
		return err
	}
	if err := r.Enforcement.Validate(); err != nil {
		return err
	}
	if r.Delegation.ID != r.Prepared.DeclaredDelegation.ID || r.Delegation.Revision <= r.Prepared.DeclaredDelegation.Revision || r.Enforcement.PermitID != r.Prepared.PermitID || r.Enforcement.PermitRevision != r.Prepared.PermitRevision || r.Enforcement.PermitDigest != r.Prepared.PermitDigest || r.Enforcement.AttemptID != r.Prepared.AttemptID || r.Enforcement.OperationDigest != r.Prepared.OperationDigest || r.Enforcement.Provider != r.Prepared.Provider {
		return core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "prepared governance result combines different attempts")
	}
	return nil
}

// ExecutionDelegationGovernancePortV2 is the Application-facing declaration
// and preparation-commit seam. Raw delegation/enforcement Fact owners are not
// an Application API.
type ExecutionDelegationGovernancePortV2 interface {
	DeclareExecutionDelegationV2(context.Context, DeclareExecutionDelegationRequestV2) (ExecutionDelegationRefV2, error)
	InspectDeclaredExecutionV2(context.Context, OperationSubjectV3, string) (ExecutionDelegationRefV2, error)
	CommitPreparedExecutionV2(context.Context, CommitPreparedExecutionRequestV2) (PreparedExecutionGovernanceResultV2, error)
	InspectPreparedExecutionV2(context.Context, OperationSubjectV3, string, string) (PreparedExecutionGovernanceResultV2, error)
}

type RecordGovernedProviderObservationRequestV2 struct {
	Intent      OperationEffectIntentV3        `json:"intent"`
	Permit      OperationDispatchPermitV3      `json:"permit"`
	Fence       core.ExecutionFence            `json:"fence"`
	Attempt     GovernedExecutionAttemptRefsV2 `json:"attempt"`
	Observation ProviderAttemptObservationV2   `json:"observation"`
}

type OperationObservationGovernancePortV3 interface {
	RecordGovernedProviderObservationV3(context.Context, RecordGovernedProviderObservationRequestV2) (ProviderAttemptObservationRefV2, error)
	InspectGovernedProviderObservationV3(context.Context, ExecutionDelegationRefV2, string) (ProviderAttemptObservationRefV2, error)
}

func (o ProviderAttemptObservationV2) Validate() error {
	if o.ContractVersion != ExecutionGovernanceContractVersionV2 || o.Revision == 0 || o.PayloadRevision == 0 || o.State != ProviderAttemptPreparedV2 && o.State != ProviderAttemptExecutingV2 && o.State != ProviderAttemptObservedV2 && o.State != ProviderAttemptUnknownV2 || validateEvidenceIDV2(o.ProviderOperationRef) != nil || validateEvidenceIDV2(o.SourceRegistrationID) != nil || o.SourceEpoch == 0 || o.SourceSequence == 0 || o.ObservedUnixNano <= 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "provider attempt observation state, operation ref or time is invalid")
	}
	if err := o.Delegation.Validate(); err != nil {
		return err
	}
	if err := o.Prepared.Validate(); err != nil {
		return err
	}
	if err := o.Payload.Validate(); err != nil {
		return err
	}
	if err := o.Evidence.Validate(); err != nil {
		return err
	}
	if o.Delegation.ID != o.Prepared.DeclaredDelegation.ID || o.Delegation.Revision <= o.Prepared.DeclaredDelegation.Revision || o.ObservedUnixNano < o.Prepared.PreparedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidReference, "provider observation does not bind exact prepared delegation/attempt")
	}
	return nil
}

func (o ProviderAttemptObservationV2) ValidateAgainstPrepared(r PreparedProviderAttemptRefV2) error {
	if err := o.Validate(); err != nil {
		return err
	}
	if o.Prepared != r {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "provider observation changed prepared operation, payload, Permit, attempt or provider")
	}
	return nil
}

// GovernedExecutionProviderV2 is the actual data-plane seam. Prepare validates
// a begun provider Permit; Runtime persists Enforcement before ExecutePrepared.
// Any uncertain Execute result is recovered only through InspectAttempt.
type GovernedExecutionProviderV2 interface {
	Prepare(context.Context, PrepareGovernedExecutionRequestV2) (ProviderPreparationAttestationV2, error)
	// InspectPrepared is a local state-plane read used after a lost Prepare
	// reply. It must never contact a remote provider.
	InspectPrepared(context.Context, InspectPreparedProviderRequestV2) (ProviderPreparationAttestationV2, error)
	ExecutePrepared(context.Context, ExecutePreparedRequestV2) (ProviderAttemptObservationV2, error)
	// InspectLocalAttempt is also a local state-plane read. A remote Inspect is
	// an independent Operation Effect related to the original Effect.
	InspectLocalAttempt(context.Context, InspectLocalProviderAttemptRequestV2) (ProviderAttemptObservationV2, error)
}

// GovernedExecutionPortV2 is the host relay seam. It has the same bounded
// messages but gains no right to author a provider Enforcement receipt.
type GovernedExecutionPortV2 interface {
	RelayPrepare(context.Context, PrepareGovernedExecutionRequestV2) (ProviderPreparationAttestationV2, error)
	RelayInspectPrepared(context.Context, InspectPreparedProviderRequestV2) (ProviderPreparationAttestationV2, error)
	RelayExecutePrepared(context.Context, ExecutePreparedRequestV2) (ProviderAttemptObservationV2, error)
	RelayInspectLocalAttempt(context.Context, InspectLocalProviderAttemptRequestV2) (ProviderAttemptObservationV2, error)
}
