package contract

import (
	"sort"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type VerdictStateV1 string

const (
	VerdictAcceptedV1    VerdictStateV1 = "accepted"
	VerdictRejectedV1    VerdictStateV1 = "rejected"
	VerdictConditionalV1 VerdictStateV1 = "conditional"
	VerdictExpiredV1     VerdictStateV1 = "expired"
	VerdictRevokedV1     VerdictStateV1 = "revoked"
	VerdictSupersededV1  VerdictStateV1 = "superseded"
)

type VerdictV1 struct {
	FactIdentityV1
	CaseID                  string                                   `json:"case_id"`
	CaseRevision            core.Revision                            `json:"case_revision"`
	CaseDigest              core.Digest                              `json:"case_digest"`
	TargetID                string                                   `json:"target_id"`
	TargetRevision          core.Revision                            `json:"target_revision"`
	TargetDigest            core.Digest                              `json:"target_digest"`
	PayloadRevision         core.Revision                            `json:"payload_revision"`
	PayloadDigest           core.Digest                              `json:"payload_digest"`
	Scope                   core.ExecutionScope                      `json:"scope"`
	ActionScopeDigest       core.Digest                              `json:"action_scope_digest"`
	TargetEvidenceSetDigest core.Digest                              `json:"target_evidence_set_digest"`
	ContextFrameDigest      core.Digest                              `json:"context_frame_digest"`
	IntentID                core.EffectIntentID                      `json:"intent_id,omitempty"`
	IntentRevision          core.Revision                            `json:"intent_revision,omitempty"`
	SubjectDigest           core.Digest                              `json:"subject_digest,omitempty"`
	Policy                  runtimeports.ReviewPolicyBindingRefV2    `json:"policy"`
	ActorAuthority          runtimeports.AuthorityBindingRefV2       `json:"actor_authority"`
	ReviewerAuthority       runtimeports.AuthorityBindingRefV2       `json:"reviewer_authority"`
	CurrentScope            runtimeports.ExecutionScopeBindingRefV2  `json:"current_scope"`
	RoundID                 string                                   `json:"round_id"`
	RoundRevision           core.Revision                            `json:"round_revision"`
	RoundDigest             core.Digest                              `json:"round_digest"`
	AssignmentID            string                                   `json:"assignment_id"`
	AssignmentRevision      core.Revision                            `json:"assignment_revision"`
	AssignmentDigest        core.Digest                              `json:"assignment_digest"`
	ReviewerID              string                                   `json:"reviewer_id"`
	ReviewerBinding         runtimeports.ReviewComponentBindingRefV2 `json:"reviewer_binding"`
	State                   VerdictStateV1                           `json:"state"`
	AttestationRefs         []string                                 `json:"attestation_refs"`
	ReasonCodes             []string                                 `json:"reason_codes"`
	FindingDigest           core.Digest                              `json:"finding_digest"`
	EvidenceDigest          core.Digest                              `json:"evidence_digest"`
	Conditions              []runtimeports.ReviewConditionV2         `json:"conditions,omitempty"`
	ConditionsDigest        core.Digest                              `json:"conditions_digest,omitempty"`
	ExpiresUnixNano         int64                                    `json:"expires_unix_nano"`
	InvalidationReason      core.ReasonCode                          `json:"invalidation_reason,omitempty"`
}

func (v VerdictV1) digestValue() VerdictV1 { v.Digest = ""; return v }
func (v VerdictV1) validateShape() error {
	if err := v.FactIdentityV1.ValidateShape(); err != nil {
		return err
	}
	if invalidID(v.CaseID) || v.CaseRevision == 0 || v.CaseDigest.Validate() != nil || invalidID(v.TargetID) || v.TargetRevision == 0 || v.TargetDigest.Validate() != nil || v.PayloadRevision == 0 || v.PayloadDigest.Validate() != nil || v.ActionScopeDigest.Validate() != nil || v.TargetEvidenceSetDigest.Validate() != nil || v.ContextFrameDigest.Validate() != nil || invalidID(v.RoundID) || v.RoundRevision == 0 || v.RoundDigest.Validate() != nil || invalidID(v.AssignmentID) || v.AssignmentRevision == 0 || v.AssignmentDigest.Validate() != nil || invalidID(v.ReviewerID) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonReviewVerdictMissing, "review verdict subject is incomplete")
	}
	if err := v.Scope.Validate(); err != nil {
		return err
	}
	if err := v.Policy.Validate(); err != nil {
		return err
	}
	if err := v.ActorAuthority.Validate(); err != nil {
		return err
	}
	if err := v.ReviewerAuthority.Validate(); err != nil {
		return err
	}
	if err := v.ReviewerBinding.Validate(); err != nil {
		return err
	}
	if err := v.CurrentScope.Validate(); err != nil {
		return err
	}
	switch v.State {
	case VerdictAcceptedV1, VerdictRejectedV1, VerdictConditionalV1, VerdictExpiredV1, VerdictRevokedV1, VerdictSupersededV1:
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "review verdict state is unsupported")
	}
	if len(v.AttestationRefs) == 0 || len(v.AttestationRefs) > MaxListItemsV1 || !sort.StringsAreSorted(v.AttestationRefs) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "verdict attestation refs must be bounded and sorted")
	}
	for i, x := range v.AttestationRefs {
		if invalidID(x) || (i > 0 && v.AttestationRefs[i-1] == x) {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "verdict attestation refs are invalid or duplicated")
		}
	}
	if len(v.ReasonCodes) == 0 || len(v.ReasonCodes) > MaxListItemsV1 || !sort.StringsAreSorted(v.ReasonCodes) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "verdict reason codes must be bounded and sorted")
	}
	for _, d := range []core.Digest{v.FindingDigest, v.EvidenceDigest} {
		if err := d.Validate(); err != nil {
			return err
		}
	}
	if err := validateConditionsSetV2Compat(v.Conditions, v.ConditionsDigest, v.State == VerdictConditionalV1); err != nil {
		return err
	}
	if (v.IntentID == "") != (v.IntentRevision == 0) || (v.IntentID == "") != (v.SubjectDigest == "") {
		return core.NewError(core.ErrorConflict, core.ReasonEffectIntentMissing, "verdict effect intent binding is partial")
	}
	if v.SubjectDigest != "" && v.SubjectDigest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidDigest, "verdict subject digest is invalid")
	}
	if activeVerdict(v.State) {
		if v.InvalidationReason != "" {
			return core.NewError(core.ErrorConflict, core.ReasonInvalidState, "active verdict cannot have invalidation reason")
		}
	} else if v.InvalidationReason == "" {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "inactive verdict requires invalidation reason")
	}
	return ValidateExpires(v.CreatedUnixNano, v.ExpiresUnixNano)
}
func SealVerdictV1(v VerdictV1) (VerdictV1, error) {
	v.AttestationRefs = append([]string(nil), v.AttestationRefs...)
	v.ReasonCodes = append([]string(nil), v.ReasonCodes...)
	v.Conditions = append([]runtimeports.ReviewConditionV2(nil), v.Conditions...)
	v.ContractVersion = ContractVersionV1
	v.Digest = ""
	sortConditionsV2(v.Conditions)
	if len(v.Conditions) > 0 && v.ConditionsDigest == "" {
		var err error
		v.ConditionsDigest, err = runtimeports.DigestReviewConditionsV2(v.Conditions)
		if err != nil {
			return VerdictV1{}, err
		}
	}
	if err := validateConditionsSetV2(v.Conditions, v.ConditionsDigest, v.State == VerdictConditionalV1); err != nil {
		return VerdictV1{}, err
	}
	if err := v.validateShape(); err != nil {
		return VerdictV1{}, err
	}
	d, err := seal("VerdictV1", v.digestValue())
	if err != nil {
		return VerdictV1{}, err
	}
	v.Digest = d
	return v, v.Validate()
}

// ValidateProductionConditionsV2 rejects the legacy digest-only conditional
// shape while keeping immutable historical V1 facts readable.
func (v VerdictV1) ValidateProductionConditionsV2() error {
	if err := v.Validate(); err != nil {
		return err
	}
	if err := validateConditionsSetV2(v.Conditions, v.ConditionsDigest, v.State == VerdictConditionalV1); err != nil {
		return err
	}
	for _, condition := range v.Conditions {
		if condition.ExpiresUnixNano <= v.CreatedUnixNano || v.ExpiresUnixNano > condition.ExpiresUnixNano {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewConditionUnsatisfied, "verdict exceeds an exact condition TTL")
		}
	}
	return nil
}
func (v VerdictV1) Validate() error {
	if err := v.validateShape(); err != nil {
		return err
	}
	return validateSealed("VerdictV1", v.digestValue(), v.Digest)
}
func activeVerdict(s VerdictStateV1) bool {
	return s == VerdictAcceptedV1 || s == VerdictRejectedV1 || s == VerdictConditionalV1
}

type VerdictCurrentnessV1 struct {
	Target            TargetCurrentnessV1
	ReviewerAuthority runtimeports.AuthorityBindingRefV2
	Now               time.Time
}

func (v VerdictV1) ValidateCurrent(c VerdictCurrentnessV1) error {
	if err := v.ValidateProductionConditionsV2(); err != nil {
		return err
	}
	if !activeVerdict(v.State) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "review verdict is inactive")
	}
	if c.Target.TargetID != v.TargetID || c.Target.TargetRevision != v.TargetRevision || c.Target.TargetDigest != v.TargetDigest || c.Target.PayloadRevision != v.PayloadRevision || c.Target.PayloadDigest != v.PayloadDigest || c.Target.Scope != v.Scope || c.Target.ActionScopeDigest != v.ActionScopeDigest || c.Target.EvidenceSetDigest != v.TargetEvidenceSetDigest || c.Target.ContextFrameDigest != v.ContextFrameDigest || c.Target.Policy != v.Policy || c.Target.ActorAuthority != v.ActorAuthority || c.Target.CurrentScope != v.CurrentScope || c.ReviewerAuthority != v.ReviewerAuthority {
		return core.NewError(core.ErrorConflict, core.ReasonReviewVerdictStale, "review verdict currentness drifted")
	}
	return ValidateNow(c.Now, v.CreatedUnixNano, v.ExpiresUnixNano)
}
