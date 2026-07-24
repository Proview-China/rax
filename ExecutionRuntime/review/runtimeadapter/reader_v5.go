package runtimeadapter

import (
	"context"
	"reflect"
	"sort"
	"strconv"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const currentSnapshotContractV5 = "praxis.review.runtime-current/v5"

type ExactCurrentRequestV5 struct {
	Intent runtimeports.OperationEffectIntentV3             `json:"intent"`
	Basis  runtimeports.OperationReviewAuthorizationBasisV5 `json:"basis"`
}

func (r ExactCurrentRequestV5) Validate() error {
	return (runtimeports.OperationReviewCurrentRequestV5{Intent: r.Intent, Basis: r.Basis}).Validate()
}

// OwnerCurrentReceiptV5 is an assembler receipt over one external Owner's
// exact immutable source. It is not the Owner fact and grants no authority.
type OwnerCurrentReceiptV5 struct {
	Kind                        string                                                `json:"kind"`
	Target                      contract.HumanTargetExactRefV2                        `json:"target"`
	Assignment                  *contract.HumanPanelAssignmentExactRefV2              `json:"assignment,omitempty"`
	HumanQuorumPolicySource     *contract.HumanQuorumPolicyBindingV2                  `json:"human_quorum_policy_source,omitempty"`
	HumanQuorumPolicyProjection *runtimeports.HumanQuorumPolicyCurrentProjectionRefV2 `json:"human_quorum_policy_projection,omitempty"`
	ReviewBindingSource         *runtimeports.ReviewComponentBindingRefV2             `json:"review_binding_source,omitempty"`
	ReviewBindingProjection     *runtimeports.ReviewBindingProjectionRefV1            `json:"review_binding_projection,omitempty"`
	SourceRef                   string                                                `json:"source_ref"`
	SourceRevision              core.Revision                                         `json:"source_revision"`
	SourceDigest                core.Digest                                           `json:"source_digest"`
	PolicyDecisionRef           string                                                `json:"policy_decision_ref,omitempty"`
	PolicyOperationNotRequired  bool                                                  `json:"policy_operation_not_required,omitempty"`
	Projection                  runtimeports.OperationGovernanceFactRefV3             `json:"projection"`
	SubjectDigest               core.Digest                                           `json:"subject_digest"`
	Current                     bool                                                  `json:"current"`
	CheckedUnixNano             int64                                                 `json:"checked_unix_nano"`
	SourceExpiresUnixNano       int64                                                 `json:"source_expires_unix_nano"`
	ExpiresUnixNano             int64                                                 `json:"expires_unix_nano"`
	ProjectionDigest            core.Digest                                           `json:"projection_digest"`
}

func (r OwnerCurrentReceiptV5) digestValue() OwnerCurrentReceiptV5 { r.ProjectionDigest = ""; return r }
func (r OwnerCurrentReceiptV5) DigestV5() (core.Digest, error) {
	return core.CanonicalJSONDigest("praxis.review.runtime-current", currentSnapshotContractV5, "OwnerCurrentReceiptV5", r.digestValue())
}
func SealOwnerCurrentReceiptV5(r OwnerCurrentReceiptV5) (OwnerCurrentReceiptV5, error) {
	if r.Assignment != nil {
		x := *r.Assignment
		r.Assignment = &x
	}
	if r.HumanQuorumPolicySource != nil {
		x := *r.HumanQuorumPolicySource
		r.HumanQuorumPolicySource = &x
	}
	if r.HumanQuorumPolicyProjection != nil {
		x := *r.HumanQuorumPolicyProjection
		r.HumanQuorumPolicyProjection = &x
	}
	if r.ReviewBindingSource != nil {
		x := *r.ReviewBindingSource
		r.ReviewBindingSource = &x
	}
	if r.ReviewBindingProjection != nil {
		x := *r.ReviewBindingProjection
		r.ReviewBindingProjection = &x
	}
	if r.Kind == "policy" {
		human := r.HumanQuorumPolicySource != nil || r.HumanQuorumPolicyProjection != nil
		legacy := r.PolicyDecisionRef != ""
		if human == legacy || (human && (r.HumanQuorumPolicySource == nil || r.HumanQuorumPolicyProjection == nil || r.PolicyOperationNotRequired)) {
			return OwnerCurrentReceiptV5{}, stale("Policy receipt must carry exactly one complete legacy decision or Human quorum projection")
		}
	} else if r.PolicyDecisionRef != "" || r.PolicyOperationNotRequired || r.HumanQuorumPolicySource != nil || r.HumanQuorumPolicyProjection != nil {
		return OwnerCurrentReceiptV5{}, stale("non-Policy receipt carries Policy semantics")
	}
	if r.Kind == "binding" {
		if r.Assignment != nil && (r.ReviewBindingSource == nil || r.ReviewBindingProjection == nil) {
			return OwnerCurrentReceiptV5{}, stale("Binding receipt lacks its exact Assignment, nominal source or Owner projection")
		}
		if r.Assignment == nil && (r.ReviewBindingSource != nil || r.ReviewBindingProjection != nil) {
			return OwnerCurrentReceiptV5{}, stale("Provider Binding receipt cannot type-pun a Review Binding projection")
		}
	} else if r.ReviewBindingSource != nil || r.ReviewBindingProjection != nil {
		return OwnerCurrentReceiptV5{}, stale("non-Binding receipt carries Binding semantics")
	}
	r.SubjectDigest = ""
	subject, err := ownerCurrentSubjectDigestV5(r)
	if err != nil {
		return OwnerCurrentReceiptV5{}, err
	}
	r.SubjectDigest = subject
	r.ProjectionDigest = ""
	d, err := r.DigestV5()
	if err != nil {
		return OwnerCurrentReceiptV5{}, err
	}
	r.ProjectionDigest = d
	return r, r.Validate(time.Unix(0, r.CheckedUnixNano))
}
func (r OwnerCurrentReceiptV5) Validate(now time.Time) error {
	if r.Kind == "" || r.SourceRef == "" || r.SourceRevision == 0 || !r.Current || r.CheckedUnixNano <= 0 || r.CheckedUnixNano >= r.ExpiresUnixNano || r.SourceExpiresUnixNano < r.ExpiresUnixNano || now.IsZero() || now.UnixNano() < r.CheckedUnixNano || now.UnixNano() >= r.ExpiresUnixNano {
		return stale("external Owner current receipt is incomplete or stale")
	}
	for _, validate := range []func() error{r.SourceDigest.Validate, r.SubjectDigest.Validate, r.ProjectionDigest.Validate} {
		if err := validate(); err != nil {
			return err
		}
	}
	if err := r.Target.Validate(); err != nil {
		return err
	}
	if r.Assignment != nil {
		if err := r.Assignment.Validate(); err != nil {
			return err
		}
		if r.Assignment.TenantID != r.Target.TenantID {
			return stale("external Owner receipt Assignment crosses Target tenant")
		}
	}
	if r.Kind == "binding" && r.Assignment != nil {
		if err := r.ReviewBindingSource.Validate(); err != nil {
			return err
		}
		if err := r.ReviewBindingProjection.Validate(); err != nil {
			return err
		}
		if r.Projection.Ref != r.ReviewBindingProjection.ID || r.Projection.Revision != r.ReviewBindingProjection.Revision || r.Projection.Digest != r.ReviewBindingProjection.Digest {
			return stale("Binding receipt output drifted from the exact Binding Owner projection")
		}
	}
	if r.Kind == "policy" && r.HumanQuorumPolicySource != nil {
		if err := r.HumanQuorumPolicySource.Validate(); err != nil {
			return err
		}
		if err := r.HumanQuorumPolicyProjection.Validate(); err != nil {
			return err
		}
		if r.SourceRef != r.HumanQuorumPolicySource.Ref || r.SourceRevision != r.HumanQuorumPolicySource.Revision || r.SourceDigest != r.HumanQuorumPolicySource.Digest || r.Projection.Ref != r.HumanQuorumPolicyProjection.ID || r.Projection.Revision != r.HumanQuorumPolicyProjection.Revision || r.Projection.Digest != r.HumanQuorumPolicyProjection.Digest {
			return stale("Human quorum Policy receipt drifted from its nominal source or Owner projection")
		}
	}
	wantSubject, subjectErr := ownerCurrentSubjectDigestV5(r)
	if subjectErr != nil || wantSubject != r.SubjectDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "external Owner current receipt subject drifted")
	}
	if err := r.Projection.Validate(now); err != nil {
		return err
	}
	if r.Projection.ExpiresUnixNano != r.ExpiresUnixNano {
		return stale("external Owner receipt projection TTL drifted from its completed cut")
	}
	d, err := r.DigestV5()
	if err != nil || d != r.ProjectionDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "external Owner current receipt digest drifted")
	}
	return nil
}

func ownerCurrentSubjectDigestV5(r OwnerCurrentReceiptV5) (core.Digest, error) {
	return core.CanonicalJSONDigest("praxis.review.runtime-current", currentSnapshotContractV5, "OwnerCurrentSubjectV5", struct {
		Kind                        string                                                `json:"kind"`
		Target                      contract.HumanTargetExactRefV2                        `json:"target"`
		Assignment                  *contract.HumanPanelAssignmentExactRefV2              `json:"assignment,omitempty"`
		HumanQuorumPolicySource     *contract.HumanQuorumPolicyBindingV2                  `json:"human_quorum_policy_source,omitempty"`
		HumanQuorumPolicyProjection *runtimeports.HumanQuorumPolicyCurrentProjectionRefV2 `json:"human_quorum_policy_projection,omitempty"`
		ReviewBindingSource         *runtimeports.ReviewComponentBindingRefV2             `json:"review_binding_source,omitempty"`
		ReviewBindingProjection     *runtimeports.ReviewBindingProjectionRefV1            `json:"review_binding_projection,omitempty"`
		SourceRef                   string                                                `json:"source_ref"`
		SourceRevision              core.Revision                                         `json:"source_revision"`
		SourceDigest                core.Digest                                           `json:"source_digest"`
		PolicyDecisionRef           string                                                `json:"policy_decision_ref,omitempty"`
		PolicyOperationNotRequired  bool                                                  `json:"policy_operation_not_required,omitempty"`
	}{r.Kind, r.Target, r.Assignment, r.HumanQuorumPolicySource, r.HumanQuorumPolicyProjection, r.ReviewBindingSource, r.ReviewBindingProjection, r.SourceRef, r.SourceRevision, r.SourceDigest, r.PolicyDecisionRef, r.PolicyOperationNotRequired})
}

type EvidenceCurrentReceiptV5 struct {
	Target                contract.HumanTargetExactRefV2                `json:"target"`
	Review                runtimeports.ReviewEvidenceRefV2              `json:"review"`
	Applicability         runtimeports.ReviewEvidenceApplicabilityRefV1 `json:"applicability"`
	Ledger                runtimeports.EvidenceRecordRefV2              `json:"ledger"`
	Current               bool                                          `json:"current"`
	CheckedUnixNano       int64                                         `json:"checked_unix_nano"`
	SourceExpiresUnixNano int64                                         `json:"source_expires_unix_nano"`
	ExpiresUnixNano       int64                                         `json:"expires_unix_nano"`
	ProjectionDigest      core.Digest                                   `json:"projection_digest"`
}

func (r EvidenceCurrentReceiptV5) digestValue() EvidenceCurrentReceiptV5 {
	r.ProjectionDigest = ""
	return r
}
func (r EvidenceCurrentReceiptV5) DigestV5() (core.Digest, error) {
	return core.CanonicalJSONDigest("praxis.review.runtime-current", currentSnapshotContractV5, "EvidenceCurrentReceiptV5", r.digestValue())
}
func SealEvidenceCurrentReceiptV5(r EvidenceCurrentReceiptV5) (EvidenceCurrentReceiptV5, error) {
	r.ProjectionDigest = ""
	d, err := r.DigestV5()
	if err != nil {
		return EvidenceCurrentReceiptV5{}, err
	}
	r.ProjectionDigest = d
	return r, r.Validate(time.Unix(0, r.CheckedUnixNano))
}
func (r EvidenceCurrentReceiptV5) Validate(now time.Time) error {
	if err := r.Target.Validate(); err != nil {
		return err
	}
	if err := r.Review.Validate(); err != nil {
		return err
	}
	if err := r.Applicability.Validate(); err != nil {
		return err
	}
	if err := r.Ledger.Validate(); err != nil {
		return err
	}
	if !r.Current || r.CheckedUnixNano <= 0 || r.CheckedUnixNano >= r.ExpiresUnixNano || r.SourceExpiresUnixNano < r.ExpiresUnixNano || now.IsZero() || now.UnixNano() < r.CheckedUnixNano || now.UnixNano() >= r.ExpiresUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceUnavailable, "Evidence receipt is not current")
	}
	d, err := r.DigestV5()
	if err != nil || d != r.ProjectionDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Evidence receipt digest drifted")
	}
	return nil
}

type QuorumCurrentSnapshotV5 struct {
	DecisionCase         contract.ReviewCaseV1                     `json:"decision_case"`
	CurrentCase          contract.ReviewCaseV1                     `json:"current_case"`
	CaseHistory          []contract.ReviewCaseV1                   `json:"case_history"`
	Round                contract.ReviewRoundV1                    `json:"round"`
	DecisionPanel        contract.HumanReviewPanelV2               `json:"decision_panel"`
	CurrentPanel         contract.HumanReviewPanelV2               `json:"current_panel"`
	PanelHistory         []contract.HumanReviewPanelV2             `json:"panel_history"`
	Quorum               contract.HumanQuorumDecisionV2            `json:"quorum"`
	Verdict              contract.HumanVerdictV2                   `json:"verdict"`
	Assignments          []contract.HumanPanelAssignmentV2         `json:"assignments"`
	Attestations         []contract.HumanAttestationV2             `json:"attestations"`
	OrganizationCut      reviewport.HumanOrganizationCurrentCutV2  `json:"organization_cut"`
	Policy               OwnerCurrentReceiptV5                     `json:"policy"`
	Scope                OwnerCurrentReceiptV5                     `json:"scope"`
	ActorAuthorities     []OwnerCurrentReceiptV5                   `json:"actor_authorities"`
	ReviewerAuthorities  []OwnerCurrentReceiptV5                   `json:"reviewer_authorities"`
	Bindings             []OwnerCurrentReceiptV5                   `json:"bindings"`
	Evidence             []EvidenceCurrentReceiptV5                `json:"evidence"`
	Satisfaction         *runtimeports.ConditionSatisfactionFactV2 `json:"satisfaction,omitempty"`
	SatisfactionEvidence []EvidenceCurrentReceiptV5                `json:"satisfaction_evidence,omitempty"`
}

type BypassCurrentSnapshotV5 struct {
	CurrentCase    contract.ReviewCaseV1     `json:"current_case"`
	Decision       contract.BypassDecisionV1 `json:"decision"`
	Policy         OwnerCurrentReceiptV5     `json:"policy"`
	PolicyDecision OwnerCurrentReceiptV5     `json:"policy_decision"`
	Authority      OwnerCurrentReceiptV5     `json:"authority"`
	Scope          OwnerCurrentReceiptV5     `json:"scope"`
	Binding        OwnerCurrentReceiptV5     `json:"binding"`
}

type CurrentFactSnapshotV5 struct {
	Revision          core.Revision                                    `json:"revision"`
	Basis             runtimeports.OperationReviewAuthorizationBasisV5 `json:"basis"`
	Target            contract.TargetSnapshotV1                        `json:"target"`
	Quorum            *QuorumCurrentSnapshotV5                         `json:"quorum,omitempty"`
	PolicyNotRequired *BypassCurrentSnapshotV5                         `json:"policy_not_required,omitempty"`
	Current           bool                                             `json:"current"`
	CheckedUnixNano   int64                                            `json:"checked_unix_nano"`
	ExpiresUnixNano   int64                                            `json:"expires_unix_nano"`
	Digest            core.Digest                                      `json:"digest"`
}

func (s CurrentFactSnapshotV5) DigestV5() (core.Digest, error) {
	c := cloneSnapshotV5(s)
	c.Digest = ""
	return core.CanonicalJSONDigest("praxis.review.runtime-current", currentSnapshotContractV5, "CurrentFactSnapshotV5", c)
}
func SealCurrentFactSnapshotV5(s CurrentFactSnapshotV5) (CurrentFactSnapshotV5, error) {
	s = cloneSnapshotV5(s)
	s.Digest = ""
	d, err := s.DigestV5()
	if err != nil {
		return CurrentFactSnapshotV5{}, err
	}
	s.Digest = d
	return s, nil
}

// CurrentFactSourceV5 is an injected production-shaped State Plane assembler.
// It must perform the public Owner Readers' S1/exact-S2 protocol, preserve each
// source expiry in SourceExpiresUnixNano, and may only shorten the emitted
// receipt expiry to the completed cut's true minimum. It has no mutation Port.
type CurrentFactSourceV5 interface {
	InspectReviewCurrentFactsV5(context.Context, ExactCurrentRequestV5) (CurrentFactSnapshotV5, error)
}

type ReaderV5 struct {
	source CurrentFactSourceV5
	clock  Clock
}

func NewReaderV5(source CurrentFactSourceV5, clock Clock) (*ReaderV5, error) {
	if nilInterfaceV5(source) || clock == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Review V5 current reader requires source and clock")
	}
	return &ReaderV5{source: source, clock: clock}, nil
}

func (r *ReaderV5) InspectOperationReviewCurrentV5(ctx context.Context, request runtimeports.OperationReviewCurrentRequestV5) (runtimeports.OperationReviewCurrentProjectionV5, error) {
	if err := request.Validate(); err != nil {
		return runtimeports.OperationReviewCurrentProjectionV5{}, err
	}
	baseline := r.clock()
	if baseline.IsZero() {
		return runtimeports.OperationReviewCurrentProjectionV5{}, clockRegressionV5("Review V5 baseline clock is unavailable")
	}
	exact := ExactCurrentRequestV5{Intent: request.Intent, Basis: request.Basis}
	snapshot, err := r.source.InspectReviewCurrentFactsV5(ctx, exact)
	after := r.clock()
	if after.IsZero() || after.Before(baseline) {
		return runtimeports.OperationReviewCurrentProjectionV5{}, clockRegressionV5("Review V5 clock regressed across Inspect")
	}
	if err != nil && unknownReadV5(err) {
		originalUnknown := err
		recoveryCtx, cancel, ok := boundedDetachedExactRecoveryV4(ctx, after, request.Intent.ExpiresUnixNano, snapshot.ExpiresUnixNano)
		if !ok {
			return runtimeports.OperationReviewCurrentProjectionV5{}, originalUnknown
		}
		recovered, recoveryErr := r.source.InspectReviewCurrentFactsV5(recoveryCtx, exact)
		recoveryContextErr := recoveryCtx.Err()
		cancel()
		retried := r.clock()
		if retried.IsZero() || retried.Before(after) {
			return runtimeports.OperationReviewCurrentProjectionV5{}, clockRegressionV5("Review V5 clock regressed across exact Inspect recovery")
		}
		after = retried
		if recoveryErr != nil || recoveryContextErr != nil {
			return runtimeports.OperationReviewCurrentProjectionV5{}, originalUnknown
		}
		snapshot, err = recovered, nil
	}
	if err != nil {
		return runtimeports.OperationReviewCurrentProjectionV5{}, err
	}
	snapshot = cloneSnapshotV5(snapshot)
	return projectCurrentV5(request, snapshot, after)
}

func projectCurrentV5(request runtimeports.OperationReviewCurrentRequestV5, snapshot CurrentFactSnapshotV5, now time.Time) (runtimeports.OperationReviewCurrentProjectionV5, error) {
	if err := validateSnapshotEnvelopeV5(snapshot, request.Basis, now); err != nil {
		return runtimeports.OperationReviewCurrentProjectionV5{}, err
	}
	intent, target := request.Intent, snapshot.Target
	intentDigest, err := intent.DigestV3()
	if err != nil {
		return runtimeports.OperationReviewCurrentProjectionV5{}, err
	}
	if err := validateTargetIntentV5(target, intent, now); err != nil {
		return runtimeports.OperationReviewCurrentProjectionV5{}, err
	}
	var union runtimeports.OperationReviewCurrentProjectionV5
	switch request.Basis {
	case runtimeports.OperationReviewBasisAcceptedQuorumV5, runtimeports.OperationReviewBasisConditionalQuorumSatisfiedV5:
		q, err := projectQuorumV5(request.Basis, intent, intentDigest, target, *snapshot.Quorum, now, snapshot.Digest)
		if err != nil {
			return union, err
		}
		union = runtimeports.OperationReviewCurrentProjectionV5{Basis: request.Basis, Quorum: &q}
	case runtimeports.OperationReviewBasisPolicyNotRequiredV5:
		n, err := projectBypassV5(intent, intentDigest, target, *snapshot.PolicyNotRequired, now, snapshot.Digest)
		if err != nil {
			return union, err
		}
		union = runtimeports.OperationReviewCurrentProjectionV5{Basis: request.Basis, PolicyNotRequired: &n}
	default:
		return union, stale("Review V5 basis is unsupported")
	}
	sealed, err := runtimeports.SealOperationReviewCurrentProjectionV5(union, now)
	if err != nil {
		return union, err
	}
	return sealed, nil
}

func validateSnapshotEnvelopeV5(s CurrentFactSnapshotV5, basis runtimeports.OperationReviewAuthorizationBasisV5, now time.Time) error {
	if s.Revision == 0 || s.Basis != basis || !s.Current || s.CheckedUnixNano <= 0 || s.CheckedUnixNano >= s.ExpiresUnixNano || now.IsZero() || now.UnixNano() < s.CheckedUnixNano || now.UnixNano() >= s.ExpiresUnixNano {
		return stale("Review V5 snapshot is absent, inactive or stale")
	}
	if (s.Quorum == nil) == (s.PolicyNotRequired == nil) || (s.Quorum != nil) != (basis != runtimeports.OperationReviewBasisPolicyNotRequiredV5) {
		return stale("Review V5 snapshot branch drifted")
	}
	if err := s.Digest.Validate(); err != nil {
		return err
	}
	d, err := s.DigestV5()
	if err != nil || d != s.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Review V5 snapshot digest drifted")
	}
	if s.ExpiresUnixNano != minimumSnapshotExpiryV5(s) {
		return stale("Review V5 snapshot TTL is not the shortest exact input")
	}
	return nil
}

func validateTargetIntentV5(t contract.TargetSnapshotV1, i runtimeports.OperationEffectIntentV3, now time.Time) error {
	if err := t.Validate(); err != nil {
		return err
	}
	if t.Kind != contract.TargetEffectV1 || t.ID != i.Target || t.Revision != i.Review.CandidateRevision || t.Digest != i.Review.CandidateDigest || t.IntentID != i.ID || t.IntentRevision != i.Revision || t.PayloadSchema != i.Payload.Schema || t.PayloadDigest != i.Payload.ContentDigest || t.PayloadRevision != i.PayloadRevision || t.ActionScopeDigest != i.ActionScopeDigest || !runtimeports.SameExecutionScopeV2(t.Scope, i.Operation.ExecutionScope) || t.Policy.Digest != i.Review.PolicyDigest || t.ActorAuthority != i.Authority || t.ExpiresUnixNano <= now.UnixNano() {
		return stale("Review V5 Target or Intent drifted")
	}
	if i.Operation.Kind == runtimeports.OperationScopeRunV3 && t.RunID != i.Operation.RunID {
		return stale("Review V5 Target Run drifted")
	}
	return nil
}

func projectQuorumV5(basis runtimeports.OperationReviewAuthorizationBasisV5, intent runtimeports.OperationEffectIntentV3, intentDigest core.Digest, target contract.TargetSnapshotV1, s QuorumCurrentSnapshotV5, now time.Time, currentness core.Digest) (runtimeports.OperationReviewQuorumCurrentProjectionV5, error) {
	for _, err := range []error{s.DecisionCase.Validate(), s.CurrentCase.Validate(), s.Round.Validate(), s.DecisionPanel.Validate(), s.CurrentPanel.Validate(), s.Quorum.Validate(), s.Verdict.Validate(), s.OrganizationCut.Validate(now)} {
		if err != nil {
			return runtimeports.OperationReviewQuorumCurrentProjectionV5{}, err
		}
	}
	if s.DecisionCase.ID != intent.Review.CaseRef || s.DecisionCase.TargetID != target.ID || s.DecisionCase.TargetRevision != target.Revision || s.DecisionCase.TargetDigest != target.Digest || s.CurrentCase.ID != s.DecisionCase.ID || s.CurrentCase.Revision != s.DecisionCase.Revision+1 || s.CurrentCase.State != contract.CaseResolvedV1 || s.CurrentCase.VerdictID != s.Verdict.ID || s.CurrentCase.VerdictRevision != s.Verdict.Revision || s.CurrentCase.VerdictDigest != s.Verdict.Digest || !sameHumanCaseV5(s.Verdict.Case, s.DecisionCase) {
		return runtimeports.OperationReviewQuorumCurrentProjectionV5{}, stale("current resolved Case is not the Verdict's atomic successor")
	}
	if s.CurrentPanel.ID != s.DecisionPanel.ID || s.CurrentPanel.Revision != s.DecisionPanel.Revision+1 || s.CurrentPanel.State != contract.HumanPanelDecidedV2 || !sameHumanPanelV5(s.Verdict.Panel, s.DecisionPanel) || s.Verdict.QuorumDecision != s.Quorum.ExactRef() {
		return runtimeports.OperationReviewQuorumCurrentProjectionV5{}, stale("current decided Panel is not the Verdict's atomic successor")
	}
	if !sameHumanTargetV5(s.Verdict.Target, target) || !sameHumanTargetV5(s.DecisionPanel.Target, target) || !sameHumanRoundV5(s.Verdict.Round, s.Round) || !sameHumanRoundV5(s.DecisionPanel.Round, s.Round) || s.Round.CaseID != s.DecisionCase.ID || s.Round.TargetID != target.ID || s.Round.TargetRevision != target.Revision || s.Round.TargetDigest != target.Digest || s.Round.ExpiresUnixNano <= now.UnixNano() {
		return runtimeports.OperationReviewQuorumCurrentProjectionV5{}, stale("quorum Target/Round chain drifted")
	}
	if s.Verdict.Policy != s.DecisionPanel.QuorumPolicy || s.Verdict.Policy != s.Quorum.Policy || s.Verdict.ReviewerSetDigest != s.Quorum.ReviewerSetDigest || s.Verdict.ExpiresUnixNano <= now.UnixNano() || s.Quorum.ExpiresUnixNano <= now.UnixNano() {
		return runtimeports.OperationReviewQuorumCurrentProjectionV5{}, stale("quorum Policy/reviewer set/TTL drifted")
	}
	if s.Verdict.ConditionsDigest != s.Quorum.ConditionsDigest || !reflect.DeepEqual(s.Verdict.Conditions, s.Quorum.Conditions) {
		return runtimeports.OperationReviewQuorumCurrentProjectionV5{}, stale("quorum Verdict condition set drifted")
	}
	if s.Verdict.State == contract.HumanVerdictRejectedV2 || s.Verdict.State == contract.HumanVerdictExpiredV2 || s.Verdict.State == contract.HumanVerdictRevokedV2 || s.Verdict.State == contract.HumanVerdictSupersededV2 || s.Quorum.Vetoed || s.Quorum.AcceptCount < s.Quorum.Threshold {
		return runtimeports.OperationReviewQuorumCurrentProjectionV5{}, stale("quorum Verdict cannot authorize")
	}
	if (basis == runtimeports.OperationReviewBasisAcceptedQuorumV5) != (s.Verdict.State == contract.HumanVerdictAcceptedV2 && s.Quorum.Resolution == contract.ResolutionAcceptV1) {
		if basis != runtimeports.OperationReviewBasisConditionalQuorumSatisfiedV5 || s.Verdict.State != contract.HumanVerdictConditionalV2 || s.Quorum.Resolution != contract.ResolutionConditionalV1 {
			return runtimeports.OperationReviewQuorumCurrentProjectionV5{}, stale("quorum basis and Verdict drifted")
		}
	}
	assignments, attestations, ledger, authorities, bindings, roles, err := validateQuorumChainV5(s, target, now)
	if err != nil {
		return runtimeports.OperationReviewQuorumCurrentProjectionV5{}, err
	}
	_ = assignments
	_ = attestations
	if err := validateOwnerReceiptV5(s.Policy, "policy", s.Verdict.Policy.Ref, s.Verdict.Policy.Revision, s.Verdict.Policy.Digest, target, nil, now); err != nil {
		return runtimeports.OperationReviewQuorumCurrentProjectionV5{}, err
	}
	if s.Policy.PolicyOperationNotRequired {
		return runtimeports.OperationReviewQuorumCurrentProjectionV5{}, stale("operation_not_required Policy cannot be combined with a quorum Verdict")
	}
	if err := validateOwnerReceiptV5(s.Scope, "scope", s.Verdict.CurrentScope.Ref, s.Verdict.CurrentScope.Revision, s.Verdict.CurrentScope.Digest, target, nil, now); err != nil {
		return runtimeports.OperationReviewQuorumCurrentProjectionV5{}, err
	}
	var satisfaction *runtimeports.OperationReviewConditionSatisfactionV4
	if basis == runtimeports.OperationReviewBasisConditionalQuorumSatisfiedV5 {
		x, e := projectHumanSatisfactionV5(s, intent, target, now)
		if e != nil {
			return runtimeports.OperationReviewQuorumCurrentProjectionV5{}, e
		}
		satisfaction = &x
	} else if s.Satisfaction != nil || len(s.SatisfactionEvidence) != 0 {
		return runtimeports.OperationReviewQuorumCurrentProjectionV5{}, stale("accepted quorum cannot carry Satisfaction")
	}
	p := runtimeports.OperationReviewQuorumCurrentProjectionV5{Operation: intent.Operation, IntentID: intent.ID, IntentRevision: intent.Revision, IntentDigest: intentDigest, PayloadSchema: intent.Payload.Schema, PayloadDigest: intent.Payload.ContentDigest, PayloadRevision: intent.PayloadRevision, Target: runtimeports.OperationReviewTargetRefV4{Ref: target.ID, Revision: target.Revision, Digest: target.Digest}, Case: reviewCaseRefV5(s.CurrentCase), Panel: reviewPanelRefV5(s.CurrentPanel), QuorumDecision: reviewQuorumRefV5(s.Quorum), Verdict: reviewVerdictRefV5(s.Verdict), QuorumPolicy: s.Policy.Projection, ReviewerSetDigest: s.Verdict.ReviewerSetDigest, AcceptCount: s.Quorum.AcceptCount, Threshold: s.Quorum.Threshold, SatisfiedRoleCounts: roles, ReviewerAuthorityRefs: authorities, BindingRefs: bindings, ScopeRef: s.Scope.Projection, DecisionEvidence: ledger, Basis: basis, Satisfaction: satisfaction, Current: true, CurrentnessDigest: currentness, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: minimumQuorumSnapshotExpiryV5(s)}
	return runtimeports.SealOperationReviewQuorumCurrentProjectionV5(p, now)
}

func validateQuorumChainV5(s QuorumCurrentSnapshotV5, target contract.TargetSnapshotV1, now time.Time) ([]contract.HumanPanelAssignmentV2, []contract.HumanAttestationV2, []runtimeports.EvidenceRecordRefV2, []runtimeports.OperationGovernanceFactRefV3, []runtimeports.OperationGovernanceFactRefV3, []runtimeports.OperationReviewRoleCountV5, error) {
	if len(s.Assignments) != len(s.DecisionPanel.AssignmentRefs) || len(s.Assignments) == 0 || len(s.Attestations) != len(s.Quorum.AcceptedAttestationRefs)+len(s.Quorum.OtherAttestationRefs) || len(s.OrganizationCut.Items) != len(s.Assignments) || len(s.ActorAuthorities) != len(s.Assignments) || len(s.ReviewerAuthorities) != len(s.Assignments) || len(s.Bindings) != len(s.Assignments) {
		return nil, nil, nil, nil, nil, nil, stale("quorum Assignment/Attestation/Organization cut is incomplete")
	}
	caseRefs, panelRefs, err := validateHumanHistoryV5(s, target, now)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, err
	}
	if _, ok := panelRefs[humanPanelRefKeyV5(s.Quorum.Panel)]; !ok {
		return nil, nil, nil, nil, nil, nil, stale("Quorum historical Panel is absent")
	}
	byAssignment := map[string]contract.HumanPanelAssignmentV2{}
	byAttestation := map[string]contract.HumanAttestationV2{}
	actorAuthorityByAssignment := receiptMapByAssignmentV5(s.ActorAuthorities)
	authorityBySource := receiptMapV5(s.ReviewerAuthorities)
	bindingByAssignment := receiptMapByAssignmentV5(s.Bindings)
	orgByAssignment := map[string]reviewport.HumanOrganizationAssignmentCurrentV2{}
	panelAssignments := map[string]contract.HumanPanelAssignmentExactRefV2{}
	for _, ref := range s.DecisionPanel.AssignmentRefs {
		panelAssignments[ref.ID] = ref
	}
	for _, item := range s.OrganizationCut.Items {
		orgByAssignment[item.Assignment.ID] = item
	}
	for _, a := range s.Assignments {
		if err := a.ValidateCurrent(a.ExactRef(), now); err != nil {
			return nil, nil, nil, nil, nil, nil, err
		}
		_, panelOK := panelRefs[humanPanelRefKeyV5(a.Panel)]
		_, caseOK := caseRefs[humanCaseRefKeyV5(a.Case)]
		if !panelOK || !caseOK || !sameHumanRoundRefV5(a.Round, s.Round) || !sameHumanTargetV5(a.Target, target) {
			return nil, nil, nil, nil, nil, nil, stale("Assignment provenance drifted")
		}
		if _, ok := byAssignment[a.ID]; ok {
			return nil, nil, nil, nil, nil, nil, core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "duplicate Assignment")
		}
		if ref, ok := panelAssignments[a.ID]; !ok || ref != a.ExactRef() {
			return nil, nil, nil, nil, nil, nil, stale("Panel Assignment exact set drifted")
		}
		byAssignment[a.ID] = a
		item, ok := orgByAssignment[a.ID]
		owner := item.OwnerProjectionRef
		if !ok || item.Assignment != a.ExactRef() || item.ReviewerIdentity != a.ReviewerIdentity || item.ExpiresUnixNano <= now.UnixNano() || owner.Identity.TenantID != a.ReviewerIdentity.TenantID || owner.Identity.ID != a.ReviewerIdentity.Ref || owner.Identity.Revision != a.ReviewerIdentity.Revision || owner.Identity.Digest != a.ReviewerIdentity.Digest || !reflect.DeepEqual(owner.Source.RequiredRoles, a.Roles) || owner.Source.ScopeDigest != a.DelegationScopeDigest || owner.Source.ResponsibilitySubjectID != target.ID || owner.Source.ResponsibilitySubjectDigest != target.Digest || owner.Responsibility.TenantID != s.DecisionPanel.ResponsibilitySubject.TenantID || owner.Responsibility.ID != s.DecisionPanel.ResponsibilitySubject.Ref || owner.Responsibility.Revision != s.DecisionPanel.ResponsibilitySubject.Revision || owner.Responsibility.Digest != s.DecisionPanel.ResponsibilitySubject.Digest {
			return nil, nil, nil, nil, nil, nil, stale("Organization current cut drifted")
		}
		if a.Delegated {
			if owner.Delegation == nil || owner.Delegation.TenantID != a.DelegationFact.TenantID || owner.Delegation.ID != a.DelegationFact.Ref || owner.Delegation.Revision != a.DelegationFact.Revision || owner.Delegation.Digest != a.DelegationFact.Digest {
				return nil, nil, nil, nil, nil, nil, stale("Organization delegation current cut drifted")
			}
		} else if owner.Delegation != nil {
			return nil, nil, nil, nil, nil, nil, stale("direct reviewer received Organization delegation")
		}
		assignmentRef := a.ExactRef()
		if err := validateOwnerReceiptV5(actorAuthorityByAssignment[a.ID], "actor_authority", target.ActorAuthority.Ref, target.ActorAuthority.Revision, target.ActorAuthority.Digest, target, &assignmentRef, now); err != nil {
			return nil, nil, nil, nil, nil, nil, err
		}
		if err := validateOwnerReceiptV5(authorityBySource[sourceKeyV5(a.ReviewerAuthority.Ref, a.ReviewerAuthority.Revision, a.ReviewerAuthority.Digest)], "reviewer_authority", a.ReviewerAuthority.Ref, a.ReviewerAuthority.Revision, a.ReviewerAuthority.Digest, target, &assignmentRef, now); err != nil {
			return nil, nil, nil, nil, nil, nil, err
		}
		if err := validateBindingReceiptV5(bindingByAssignment[a.ID], target, assignmentRef, a.ReviewerBinding, now); err != nil {
			return nil, nil, nil, nil, nil, nil, err
		}
	}
	allRefs := append(append([]contract.HumanAttestationExactRefV2{}, s.Quorum.AcceptedAttestationRefs...), s.Quorum.OtherAttestationRefs...)
	expected := map[string]contract.HumanAttestationExactRefV2{}
	for _, r := range allRefs {
		expected[r.ID] = r
	}
	if len(s.Verdict.AttestationRefs) != len(s.Quorum.AcceptedAttestationRefs) {
		return nil, nil, nil, nil, nil, nil, stale("Verdict and Quorum accepted Attestation sets differ")
	}
	for i := range s.Verdict.AttestationRefs {
		if s.Verdict.AttestationRefs[i] != s.Quorum.AcceptedAttestationRefs[i] {
			return nil, nil, nil, nil, nil, nil, stale("Verdict accepted Attestation ref drifted")
		}
	}
	var reviewEvidence []runtimeports.ReviewEvidenceRefV2
	reviewerIDs := map[string]struct{}{}
	roleCounts := map[string]uint32{}
	for _, a := range s.Attestations {
		if err := a.ValidateCurrent(a.ExactRef(), now); err != nil {
			return nil, nil, nil, nil, nil, nil, err
		}
		ref, ok := expected[a.ID]
		as, asOK := byAssignment[a.Assignment.ID]
		_, panelOK := panelRefs[humanPanelRefKeyV5(a.Panel)]
		_, caseOK := caseRefs[humanCaseRefKeyV5(a.Case)]
		if !ok || ref != a.ExactRef() || !asOK || a.Assignment != as.ExactRef() || !panelOK || !caseOK || !sameHumanRoundRefV5(a.Round, s.Round) || !sameHumanTargetV5(a.Target, target) || a.ReviewerIdentity != as.ReviewerIdentity || a.ReviewerAuthority != as.ReviewerAuthority || a.ReviewerBinding != as.ReviewerBinding {
			return nil, nil, nil, nil, nil, nil, stale("Attestation chain drifted")
		}
		key := string(a.ReviewerIdentity.TenantID) + "\x00" + a.ReviewerIdentity.Ref
		if _, ok := reviewerIDs[key]; ok {
			return nil, nil, nil, nil, nil, nil, core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "reviewer voted twice")
		}
		reviewerIDs[key] = struct{}{}
		byAttestation[a.ID] = a
		if containsHumanAttestationRefV5(s.Quorum.AcceptedAttestationRefs, a.ExactRef()) && a.Resolution != contract.ResolutionAcceptV1 && a.Resolution != contract.ResolutionConditionalV1 {
			return nil, nil, nil, nil, nil, nil, stale("accepted quorum counted a non-accepting Attestation")
		}
		reviewEvidence = append(reviewEvidence, a.Evidence...)
		if a.Resolution == contract.ResolutionAcceptV1 || a.Resolution == contract.ResolutionConditionalV1 {
			for _, role := range as.Roles {
				roleCounts[role]++
			}
		}
	}
	if len(byAttestation) != len(expected) {
		return nil, nil, nil, nil, nil, nil, stale("Attestation exact set is incomplete")
	}
	conditions, conditionsDigest, err := contract.CanonicalAcceptedConditionsV2(s.Attestations, s.Quorum.AcceptedAttestationRefs)
	if err != nil || conditionsDigest != s.Quorum.ConditionsDigest || !reflect.DeepEqual(conditions, s.Quorum.Conditions) || !reflect.DeepEqual(conditions, s.Verdict.Conditions) {
		return nil, nil, nil, nil, nil, nil, core.NewError(core.ErrorConflict, core.ReasonReviewConditionUnsatisfied, "quorum condition union drifted")
	}
	for _, condition := range conditions {
		if condition.ScopeDigest != target.ActionScopeDigest || condition.ExpiresUnixNano <= now.UnixNano() {
			return nil, nil, nil, nil, nil, nil, core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewConditionUnsatisfied, "quorum condition Scope or TTL drifted")
		}
	}
	computedEvidence, err := contract.ComputeReviewEvidenceDigestV1(reviewEvidence)
	if err != nil || computedEvidence != s.Quorum.EvidenceSetDigest || computedEvidence != s.Verdict.EvidenceSetDigest {
		return nil, nil, nil, nil, nil, nil, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "quorum Evidence digest drifted")
	}
	verdictAuthorities := append([]runtimeports.AuthorityBindingRefV2(nil), s.Verdict.ReviewerAuthorityRefs...)
	verdictBindings := append([]runtimeports.ReviewComponentBindingRefV2(nil), s.Verdict.BindingClosures...)
	if len(verdictAuthorities) != len(s.Assignments) || len(verdictBindings) != len(s.Assignments) {
		return nil, nil, nil, nil, nil, nil, stale("Verdict authority or binding closure is incomplete")
	}
	for _, a := range s.Assignments {
		if !containsAuthorityBindingV5(verdictAuthorities, a.ReviewerAuthority) || !containsReviewBindingV5(verdictBindings, a.ReviewerBinding) {
			return nil, nil, nil, nil, nil, nil, stale("Verdict authority or binding exact set drifted")
		}
	}
	ledger, err := ledgerEvidenceV5(target, reviewEvidence, s.Evidence, now)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, err
	}
	authorities := make([]runtimeports.OperationGovernanceFactRefV3, 0, len(s.ReviewerAuthorities))
	for _, r := range s.ReviewerAuthorities {
		authorities = append(authorities, r.Projection)
	}
	sort.Slice(authorities, func(i, j int) bool { return authorities[i].Ref < authorities[j].Ref })
	bindings := make([]runtimeports.OperationGovernanceFactRefV3, 0, len(s.Bindings))
	for _, r := range s.Bindings {
		bindings = append(bindings, r.Projection)
	}
	sort.Slice(bindings, func(i, j int) bool { return bindings[i].Ref < bindings[j].Ref })
	roles := make([]runtimeports.OperationReviewRoleCountV5, 0, len(s.DecisionPanel.RoleRequirements))
	for _, req := range s.DecisionPanel.RoleRequirements {
		count := roleCounts[req.Role]
		if count < req.Minimum {
			return nil, nil, nil, nil, nil, nil, stale("quorum role requirement is unsatisfied")
		}
		roles = append(roles, runtimeports.OperationReviewRoleCountV5{Role: req.Role, Count: count, Required: req.Minimum})
	}
	return s.Assignments, s.Attestations, ledger, authorities, bindings, roles, nil
}

func projectHumanSatisfactionV5(s QuorumCurrentSnapshotV5, intent runtimeports.OperationEffectIntentV3, target contract.TargetSnapshotV1, now time.Time) (runtimeports.OperationReviewConditionSatisfactionV4, error) {
	if s.Satisfaction == nil {
		return runtimeports.OperationReviewConditionSatisfactionV4{}, core.NewError(core.ErrorForbidden, core.ReasonReviewConditionUnsatisfied, "conditional quorum lacks Satisfaction")
	}
	f := *s.Satisfaction
	if err := f.Validate(); err != nil {
		return runtimeports.OperationReviewConditionSatisfactionV4{}, err
	}
	if f.State != runtimeports.ConditionSatisfied || f.VerdictID != s.Verdict.ID || f.VerdictRevision != s.Verdict.Revision || f.VerdictDigest != s.Verdict.Digest || f.CandidateDigest != target.Digest || f.IntentID != intent.ID || f.IntentRevision != intent.Revision || f.SubjectDigest != target.SubjectDigest || f.ConditionsDigest != s.Verdict.ConditionsDigest || f.Policy != target.Policy || !runtimeports.SameExecutionScopeV2(f.Scope, target.Scope) || f.RunID != target.RunID || f.ActionScopeDigest != target.ActionScopeDigest || f.CurrentScope != target.CurrentScope || f.ExpiresUnixNano <= now.UnixNano() {
		return runtimeports.OperationReviewConditionSatisfactionV4{}, core.NewError(core.ErrorForbidden, core.ReasonReviewConditionUnsatisfied, "Satisfaction drifted")
	}
	conditionByID := make(map[runtimeports.NamespacedNameV2]runtimeports.ReviewConditionV2, len(s.Verdict.Conditions))
	for _, condition := range s.Verdict.Conditions {
		conditionByID[condition.ID] = condition
	}
	if len(f.Proofs) != len(conditionByID) {
		return runtimeports.OperationReviewConditionSatisfactionV4{}, core.NewError(core.ErrorForbidden, core.ReasonReviewConditionUnsatisfied, "Satisfaction proof set is incomplete")
	}
	expected := make([]runtimeports.ReviewEvidenceRefV2, 0, len(f.Proofs))
	for _, p := range f.Proofs {
		condition, ok := conditionByID[p.ConditionID]
		if !ok || p.ConditionRevision != condition.Revision || p.ConstraintDigest != condition.ConstraintDigest || p.Owner != condition.SatisfactionOwner || p.ScopeDigest != condition.ScopeDigest || p.Authority != condition.Authority || p.ExpiresUnixNano > condition.ExpiresUnixNano || p.ExpiresUnixNano <= now.UnixNano() || p.ScopeDigest != target.ActionScopeDigest {
			return runtimeports.OperationReviewConditionSatisfactionV4{}, core.NewError(core.ErrorForbidden, core.ReasonReviewConditionUnsatisfied, "Satisfaction proof Scope, Authority or TTL drifted")
		}
		expected = append(expected, p.Evidence)
	}
	ledger, err := ledgerEvidenceV5(target, expected, s.SatisfactionEvidence, now)
	if err != nil {
		return runtimeports.OperationReviewConditionSatisfactionV4{}, err
	}
	d, err := f.DigestV2()
	if err != nil {
		return runtimeports.OperationReviewConditionSatisfactionV4{}, err
	}
	return runtimeports.OperationReviewConditionSatisfactionV4{Fact: ownerRef(f.ID, f.Revision, d, f.ExpiresUnixNano), ConditionsDigest: f.ConditionsDigest, Evidence: ledger}, nil
}

func projectBypassV5(intent runtimeports.OperationEffectIntentV3, intentDigest core.Digest, target contract.TargetSnapshotV1, s BypassCurrentSnapshotV5, now time.Time, currentness core.Digest) (runtimeports.OperationReviewPolicyNotRequiredCurrentProjectionV5, error) {
	if err := s.CurrentCase.Validate(); err != nil {
		return runtimeports.OperationReviewPolicyNotRequiredCurrentProjectionV5{}, err
	}
	if err := s.Decision.ValidateCurrent(target.BypassExactRefV1(), s.CurrentCase.BypassExactRefV1(), s.Decision.PolicyCurrentProjection, now); err != nil {
		return runtimeports.OperationReviewPolicyNotRequiredCurrentProjectionV5{}, err
	}
	if s.CurrentCase.State != contract.CaseRoutedV1 || s.CurrentCase.ID != intent.Review.CaseRef || s.CurrentCase.TargetID != target.ID || s.CurrentCase.TargetRevision != target.Revision || s.CurrentCase.TargetDigest != target.Digest {
		return runtimeports.OperationReviewPolicyNotRequiredCurrentProjectionV5{}, stale("Bypass requires the exact routed current Case")
	}
	if s.Decision.IntentID != intent.ID || s.Decision.IntentRevision != intent.Revision || s.Decision.SubjectDigest != target.SubjectDigest || s.Decision.PayloadRevision != intent.PayloadRevision || s.Decision.PayloadDigest != intent.Payload.ContentDigest || !runtimeports.SameExecutionScopeV2(s.Decision.Scope, intent.Operation.ExecutionScope) || s.Decision.ActionScopeDigest != intent.ActionScopeDigest || s.Decision.ActorAuthority != intent.Authority || s.Decision.Policy.Digest != intent.Review.PolicyDigest || s.Decision.CurrentScope.Ref != intent.Operation.CurrentProjectionRef || s.Decision.CurrentScope.Revision != intent.Operation.CurrentProjectionRevision || s.Decision.CurrentScope.Digest != intent.Operation.CurrentProjectionDigest {
		return runtimeports.OperationReviewPolicyNotRequiredCurrentProjectionV5{}, stale("Bypass Target/Intent/Authority/Scope drifted")
	}
	checks := []struct {
		r     OwnerCurrentReceiptV5
		k, id string
		rev   core.Revision
		d     core.Digest
	}{{s.Policy, "policy", s.Decision.PolicyCurrentProjection.ID, s.Decision.PolicyCurrentProjection.Revision, s.Decision.PolicyCurrentProjection.Digest}, {s.PolicyDecision, "policy_decision", s.Decision.PolicyDecisionRef, s.Decision.Policy.Revision, s.Decision.Policy.Digest}, {s.Authority, "actor_authority", s.Decision.ActorAuthority.Ref, s.Decision.ActorAuthority.Revision, s.Decision.ActorAuthority.Digest}, {s.Scope, "scope", s.Decision.CurrentScope.Ref, s.Decision.CurrentScope.Revision, s.Decision.CurrentScope.Digest}, {s.Binding, "binding", intent.Provider.BindingSetID, intent.Provider.BindingSetRevision, bindingDigestProviderV5(intent.Provider)}}
	for _, c := range checks {
		if err := validateOwnerReceiptV5(c.r, c.k, c.id, c.rev, c.d, target, nil, now); err != nil {
			return runtimeports.OperationReviewPolicyNotRequiredCurrentProjectionV5{}, err
		}
	}
	if !s.Policy.PolicyOperationNotRequired || s.Policy.PolicyDecisionRef != s.Decision.PolicyDecisionRef {
		return runtimeports.OperationReviewPolicyNotRequiredCurrentProjectionV5{}, stale("Bypass Policy does not currently say operation_not_required")
	}
	p := runtimeports.OperationReviewPolicyNotRequiredCurrentProjectionV5{Operation: intent.Operation, IntentID: intent.ID, IntentRevision: intent.Revision, IntentDigest: intentDigest, PayloadSchema: intent.Payload.Schema, PayloadDigest: intent.Payload.ContentDigest, PayloadRevision: intent.PayloadRevision, Target: runtimeports.OperationReviewTargetRefV4{Ref: target.ID, Revision: target.Revision, Digest: target.Digest}, Case: reviewCaseRefV5(s.CurrentCase), BypassDecision: runtimeports.OperationReviewBypassDecisionRefV5{TenantID: s.Decision.TenantID, ID: s.Decision.ID, Revision: s.Decision.Revision, Digest: s.Decision.Digest, ExpiresUnixNano: s.Decision.ExpiresUnixNano}, PolicyCurrentProjection: s.Policy.Projection, PolicyDecisionRef: s.PolicyDecision.Projection, ScopeRef: s.Scope.Projection, BindingRef: s.Binding.Projection, ActorAuthorityRef: s.Authority.Projection, Current: true, CurrentnessDigest: currentness, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: minimumBypassSnapshotExpiryV5(s)}
	return runtimeports.SealOperationReviewPolicyNotRequiredCurrentProjectionV5(p, now)
}

func validateOwnerReceiptV5(r OwnerCurrentReceiptV5, kind, id string, revision core.Revision, digest core.Digest, target contract.TargetSnapshotV1, assignment *contract.HumanPanelAssignmentExactRefV2, now time.Time) error {
	if err := r.Validate(now); err != nil {
		return err
	}
	if r.Kind != kind || r.Target != (contract.HumanTargetExactRefV2{TenantID: target.TenantID, ID: target.ID, Revision: target.Revision, Digest: target.Digest}) || !reflect.DeepEqual(r.Assignment, assignment) || r.SourceRef != id || r.SourceRevision != revision || r.SourceDigest != digest {
		return stale("external Owner exact source drifted")
	}
	return nil
}
func validateBindingReceiptV5(r OwnerCurrentReceiptV5, target contract.TargetSnapshotV1, assignment contract.HumanPanelAssignmentExactRefV2, source runtimeports.ReviewComponentBindingRefV2, now time.Time) error {
	if err := r.Validate(now); err != nil {
		return err
	}
	wantTarget := contract.HumanTargetExactRefV2{TenantID: target.TenantID, ID: target.ID, Revision: target.Revision, Digest: target.Digest}
	if r.Kind != "binding" || r.Target != wantTarget || r.Assignment == nil || *r.Assignment != assignment || r.ReviewBindingSource == nil || *r.ReviewBindingSource != source || r.ReviewBindingProjection == nil {
		return stale("Binding Owner nominal source or exact projection drifted")
	}
	return nil
}
func ledgerEvidenceV5(target contract.TargetSnapshotV1, expected []runtimeports.ReviewEvidenceRefV2, receipts []EvidenceCurrentReceiptV5, now time.Time) ([]runtimeports.EvidenceRecordRefV2, error) {
	if len(expected) == 0 || len(expected) != len(receipts) {
		return nil, core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceUnavailable, "Evidence current closure is incomplete")
	}
	by := map[string]runtimeports.ReviewEvidenceRefV2{}
	for _, e := range expected {
		if err := e.Validate(); err != nil {
			return nil, err
		}
		if _, ok := by[e.Ref]; ok {
			return nil, core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "duplicate Review Evidence")
		}
		by[e.Ref] = e
	}
	out := make([]runtimeports.EvidenceRecordRefV2, 0, len(receipts))
	seen := map[string]bool{}
	for _, r := range receipts {
		if err := r.Validate(now); err != nil {
			return nil, err
		}
		if !sameHumanTargetV5(r.Target, target) {
			return nil, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Evidence receipt Target drifted")
		}
		e, ok := by[r.Review.Ref]
		if !ok || e != r.Review {
			return nil, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Evidence receipt exact ref drifted")
		}
		delete(by, r.Review.Ref)
		k := string(r.Ledger.LedgerScopeDigest) + "\x00" + strconv.FormatUint(r.Ledger.Sequence, 10) + "\x00" + string(r.Ledger.RecordDigest)
		if seen[k] {
			return nil, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "duplicate Ledger Evidence")
		}
		seen[k] = true
		out = append(out, r.Ledger)
	}
	if len(by) != 0 {
		return nil, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Evidence receipt set is incomplete")
	}
	return out, nil
}

func minimumSnapshotExpiryV5(s CurrentFactSnapshotV5) int64 {
	v := []int64{s.Target.ExpiresUnixNano}
	if s.Quorum != nil {
		v = append(v, minimumQuorumSnapshotExpiryV5(*s.Quorum))
	}
	if s.PolicyNotRequired != nil {
		v = append(v, minimumBypassSnapshotExpiryV5(*s.PolicyNotRequired))
	}
	return minimumPositiveV5Local(v...)
}
func minimumQuorumSnapshotExpiryV5(s QuorumCurrentSnapshotV5) int64 {
	v := []int64{s.DecisionCase.ExpiresUnixNano, s.CurrentCase.ExpiresUnixNano, s.Round.ExpiresUnixNano, s.DecisionPanel.ExpiresUnixNano, s.CurrentPanel.ExpiresUnixNano, s.Quorum.ExpiresUnixNano, s.Verdict.ExpiresUnixNano, s.OrganizationCut.ExpiresUnixNano, s.Policy.ExpiresUnixNano, s.Scope.ExpiresUnixNano}
	for _, a := range s.Assignments {
		v = append(v, a.ExpiresUnixNano, a.LeaseExpiresUnixNano)
	}
	for _, a := range s.Attestations {
		v = append(v, a.ExpiresUnixNano)
		for _, condition := range a.Conditions {
			v = append(v, condition.ExpiresUnixNano)
		}
	}
	for _, condition := range s.Quorum.Conditions {
		v = append(v, condition.ExpiresUnixNano)
	}
	for _, condition := range s.Verdict.Conditions {
		v = append(v, condition.ExpiresUnixNano)
	}
	for _, c := range s.CaseHistory {
		v = append(v, c.ExpiresUnixNano)
	}
	for _, p := range s.PanelHistory {
		v = append(v, p.ExpiresUnixNano)
	}
	for _, r := range s.ActorAuthorities {
		v = append(v, r.ExpiresUnixNano)
	}
	for _, r := range s.ReviewerAuthorities {
		v = append(v, r.ExpiresUnixNano)
	}
	for _, r := range s.Bindings {
		v = append(v, r.ExpiresUnixNano)
	}
	for _, r := range s.Evidence {
		v = append(v, r.ExpiresUnixNano)
	}
	if s.Satisfaction != nil {
		v = append(v, s.Satisfaction.ExpiresUnixNano)
		for _, p := range s.Satisfaction.Proofs {
			v = append(v, p.ExpiresUnixNano)
		}
	}
	for _, r := range s.SatisfactionEvidence {
		v = append(v, r.ExpiresUnixNano)
	}
	return minimumPositiveV5Local(v...)
}
func minimumBypassSnapshotExpiryV5(s BypassCurrentSnapshotV5) int64 {
	return minimumPositiveV5Local(s.CurrentCase.ExpiresUnixNano, s.Decision.ExpiresUnixNano, s.Policy.ExpiresUnixNano, s.PolicyDecision.ExpiresUnixNano, s.Authority.ExpiresUnixNano, s.Scope.ExpiresUnixNano, s.Binding.ExpiresUnixNano)
}
func minimumPositiveV5Local(v ...int64) int64 {
	m := int64(0)
	for _, x := range v {
		if x > 0 && (m == 0 || x < m) {
			m = x
		}
	}
	return m
}

func reviewCaseRefV5(c contract.ReviewCaseV1) runtimeports.OperationReviewCaseRefV5 {
	return runtimeports.OperationReviewCaseRefV5{TenantID: c.TenantID, ID: c.ID, Revision: c.Revision, Digest: c.Digest, ExpiresUnixNano: c.ExpiresUnixNano}
}
func reviewPanelRefV5(p contract.HumanReviewPanelV2) runtimeports.OperationReviewPanelRefV5 {
	return runtimeports.OperationReviewPanelRefV5{TenantID: p.TenantID, ID: p.ID, Revision: p.Revision, Digest: p.Digest, ExpiresUnixNano: p.ExpiresUnixNano}
}
func reviewQuorumRefV5(q contract.HumanQuorumDecisionV2) runtimeports.OperationReviewQuorumDecisionRefV5 {
	return runtimeports.OperationReviewQuorumDecisionRefV5{TenantID: q.TenantID, ID: q.ID, Revision: q.Revision, Digest: q.Digest, ExpiresUnixNano: q.ExpiresUnixNano}
}
func reviewVerdictRefV5(v contract.HumanVerdictV2) runtimeports.OperationReviewVerdictRefV5 {
	return runtimeports.OperationReviewVerdictRefV5{TenantID: v.TenantID, ID: v.ID, Revision: v.Revision, Digest: v.Digest, ExpiresUnixNano: v.ExpiresUnixNano}
}
func sameHumanCaseV5(r contract.HumanCaseExactRefV2, c contract.ReviewCaseV1) bool {
	return r.TenantID == c.TenantID && r.ID == c.ID && r.Revision == c.Revision && r.Digest == c.Digest
}
func sameHumanCaseRefV5(r contract.HumanCaseExactRefV2, c contract.ReviewCaseV1) bool {
	return sameHumanCaseV5(r, c)
}
func sameHumanPanelV5(r contract.HumanPanelExactRefV2, p contract.HumanReviewPanelV2) bool {
	return r == p.ExactRef()
}
func sameHumanRoundV5(r contract.HumanRoundExactRefV2, x contract.ReviewRoundV1) bool {
	return r.TenantID == x.TenantID && r.ID == x.ID && r.Revision == x.Revision && r.Digest == x.Digest
}
func sameHumanRoundRefV5(r contract.HumanRoundExactRefV2, x contract.ReviewRoundV1) bool {
	return sameHumanRoundV5(r, x)
}
func sameHumanTargetV5(r contract.HumanTargetExactRefV2, t contract.TargetSnapshotV1) bool {
	return r.TenantID == t.TenantID && r.ID == t.ID && r.Revision == t.Revision && r.Digest == t.Digest
}
func containsHumanAttestationRefV5(values []contract.HumanAttestationExactRefV2, wanted contract.HumanAttestationExactRefV2) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
func containsAuthorityBindingV5(values []runtimeports.AuthorityBindingRefV2, wanted runtimeports.AuthorityBindingRefV2) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
func containsReviewBindingV5(values []runtimeports.ReviewComponentBindingRefV2, wanted runtimeports.ReviewComponentBindingRefV2) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
func humanCaseRefKeyV5(r contract.HumanCaseExactRefV2) string {
	return string(r.TenantID) + "\x00" + r.ID + "\x00" + strconv.FormatUint(uint64(r.Revision), 10) + "\x00" + string(r.Digest)
}
func humanPanelRefKeyV5(r contract.HumanPanelExactRefV2) string {
	return string(r.TenantID) + "\x00" + r.ID + "\x00" + strconv.FormatUint(uint64(r.Revision), 10) + "\x00" + string(r.Digest)
}
func validateHumanHistoryV5(s QuorumCurrentSnapshotV5, target contract.TargetSnapshotV1, now time.Time) (map[string]contract.ReviewCaseV1, map[string]contract.HumanReviewPanelV2, error) {
	cases := map[string]contract.ReviewCaseV1{}
	lastCase := core.Revision(0)
	for _, c := range s.CaseHistory {
		if err := c.Validate(); err != nil {
			return nil, nil, err
		}
		if c.ID != s.DecisionCase.ID || c.TargetID != target.ID || c.TargetRevision != target.Revision || c.TargetDigest != target.Digest || c.Revision <= lastCase || c.ExpiresUnixNano <= now.UnixNano() {
			return nil, nil, stale("Case history is non-monotonic or drifted")
		}
		lastCase = c.Revision
		ref := contract.HumanCaseExactRefV2{TenantID: c.TenantID, ID: c.ID, Revision: c.Revision, Digest: c.Digest}
		cases[humanCaseRefKeyV5(ref)] = c
	}
	if c, ok := cases[humanCaseRefKeyV5(s.Verdict.Case)]; !ok || c.Digest != s.DecisionCase.Digest {
		return nil, nil, stale("Verdict historical Case is absent")
	}
	panels := map[string]contract.HumanReviewPanelV2{}
	lastPanel := core.Revision(0)
	for _, p := range s.PanelHistory {
		if err := p.Validate(); err != nil {
			return nil, nil, err
		}
		if p.ID != s.DecisionPanel.ID || !sameHumanTargetV5(p.Target, target) || p.Revision <= lastPanel || p.ExpiresUnixNano <= now.UnixNano() {
			return nil, nil, stale("Panel history is non-monotonic or drifted")
		}
		lastPanel = p.Revision
		panels[humanPanelRefKeyV5(p.ExactRef())] = p
	}
	if p, ok := panels[humanPanelRefKeyV5(s.Verdict.Panel)]; !ok || p.Digest != s.DecisionPanel.Digest {
		return nil, nil, stale("Verdict historical Panel is absent")
	}
	return cases, panels, nil
}
func sourceKeyV5(id string, r core.Revision, d core.Digest) string {
	return id + "\x00" + strconv.FormatUint(uint64(r), 10) + "\x00" + string(d)
}
func receiptMapV5(v []OwnerCurrentReceiptV5) map[string]OwnerCurrentReceiptV5 {
	m := map[string]OwnerCurrentReceiptV5{}
	for _, r := range v {
		m[sourceKeyV5(r.SourceRef, r.SourceRevision, r.SourceDigest)] = r
	}
	return m
}
func receiptMapByAssignmentV5(v []OwnerCurrentReceiptV5) map[string]OwnerCurrentReceiptV5 {
	m := map[string]OwnerCurrentReceiptV5{}
	for _, r := range v {
		if r.Assignment != nil {
			m[r.Assignment.ID] = r
		}
	}
	return m
}
func bindingSourceIDV5(b runtimeports.ReviewComponentBindingRefV2) string {
	return b.BindingSetID + "/" + string(b.ComponentID) + "/" + string(b.Capability)
}
func bindingDigestV5(b runtimeports.ReviewComponentBindingRefV2) core.Digest {
	d, _ := core.CanonicalJSONDigest("praxis.review.runtime-current", currentSnapshotContractV5, "ReviewComponentBindingRefV2", b)
	return d
}
func bindingDigestProviderV5(b runtimeports.ProviderBindingRefV2) core.Digest {
	d, _ := core.CanonicalJSONDigest("praxis.review.runtime-current", currentSnapshotContractV5, "ProviderBindingRefV2", b)
	return d
}
func clockRegressionV5(message string) error {
	return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, message)
}
func unknownReadV5(err error) bool {
	return core.HasCategory(err, core.ErrorUnavailable) || core.HasCategory(err, core.ErrorIndeterminate)
}
func nilInterfaceV5(v any) bool {
	if v == nil {
		return true
	}
	x := reflect.ValueOf(v)
	switch x.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return x.IsNil()
	}
	return false
}

func cloneSnapshotV5(s CurrentFactSnapshotV5) CurrentFactSnapshotV5 {
	s.Target.Evidence = append([]runtimeports.ReviewEvidenceRefV2(nil), s.Target.Evidence...)
	if s.Quorum != nil {
		q := *s.Quorum
		q.DecisionPanel = q.DecisionPanel.Clone()
		q.CurrentPanel = q.CurrentPanel.Clone()
		q.CaseHistory = append([]contract.ReviewCaseV1(nil), q.CaseHistory...)
		q.PanelHistory = append([]contract.HumanReviewPanelV2(nil), q.PanelHistory...)
		for i := range q.PanelHistory {
			q.PanelHistory[i] = q.PanelHistory[i].Clone()
		}
		q.Quorum = q.Quorum.Clone()
		q.Verdict = q.Verdict.Clone()
		q.Assignments = append([]contract.HumanPanelAssignmentV2(nil), q.Assignments...)
		for i := range q.Assignments {
			q.Assignments[i] = q.Assignments[i].Clone()
		}
		q.Attestations = append([]contract.HumanAttestationV2(nil), q.Attestations...)
		for i := range q.Attestations {
			q.Attestations[i] = q.Attestations[i].Clone()
		}
		q.OrganizationCut = q.OrganizationCut.Clone()
		q.Policy = cloneOwnerCurrentReceiptV5(q.Policy)
		q.Scope = cloneOwnerCurrentReceiptV5(q.Scope)
		q.ActorAuthorities = cloneOwnerCurrentReceiptsV5(q.ActorAuthorities)
		q.ReviewerAuthorities = cloneOwnerCurrentReceiptsV5(q.ReviewerAuthorities)
		q.Bindings = cloneOwnerCurrentReceiptsV5(q.Bindings)
		q.Evidence = append([]EvidenceCurrentReceiptV5(nil), q.Evidence...)
		q.SatisfactionEvidence = append([]EvidenceCurrentReceiptV5(nil), q.SatisfactionEvidence...)
		if q.Satisfaction != nil {
			f := *q.Satisfaction
			f.Proofs = append([]runtimeports.ReviewConditionProofV2(nil), f.Proofs...)
			q.Satisfaction = &f
		}
		s.Quorum = &q
	}
	if s.PolicyNotRequired != nil {
		b := *s.PolicyNotRequired
		b.Policy = cloneOwnerCurrentReceiptV5(b.Policy)
		b.PolicyDecision = cloneOwnerCurrentReceiptV5(b.PolicyDecision)
		b.Authority = cloneOwnerCurrentReceiptV5(b.Authority)
		b.Scope = cloneOwnerCurrentReceiptV5(b.Scope)
		b.Binding = cloneOwnerCurrentReceiptV5(b.Binding)
		s.PolicyNotRequired = &b
	}
	return s
}

func cloneOwnerCurrentReceiptV5(value OwnerCurrentReceiptV5) OwnerCurrentReceiptV5 {
	if value.Assignment != nil {
		cloned := *value.Assignment
		value.Assignment = &cloned
	}
	if value.HumanQuorumPolicySource != nil {
		cloned := *value.HumanQuorumPolicySource
		value.HumanQuorumPolicySource = &cloned
	}
	if value.HumanQuorumPolicyProjection != nil {
		cloned := *value.HumanQuorumPolicyProjection
		value.HumanQuorumPolicyProjection = &cloned
	}
	if value.ReviewBindingSource != nil {
		cloned := *value.ReviewBindingSource
		value.ReviewBindingSource = &cloned
	}
	if value.ReviewBindingProjection != nil {
		cloned := *value.ReviewBindingProjection
		value.ReviewBindingProjection = &cloned
	}
	return value
}

func cloneOwnerCurrentReceiptsV5(values []OwnerCurrentReceiptV5) []OwnerCurrentReceiptV5 {
	out := append([]OwnerCurrentReceiptV5(nil), values...)
	for index := range out {
		out[index] = cloneOwnerCurrentReceiptV5(out[index])
	}
	return out
}

var _ runtimeports.OperationReviewCurrentReaderV5 = (*ReaderV5)(nil)
