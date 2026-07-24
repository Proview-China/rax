package contract

import (
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const (
	ReviewWaitingContractVersionV1 = "praxis.application.review-waiting/v1"
	ReviewPhaseActionV1            = runtimeports.NamespacedNameV2("praxis.harness/action.review")
	ReviewPhaseSubagentV1          = runtimeports.NamespacedNameV2("praxis.harness/subagent.completion.validate")
	ReviewPhaseRunV1               = runtimeports.NamespacedNameV2("praxis.harness/run.completion.validate")
	MaxReviewWaitingIDBytesV1      = 512
)

type ReviewWaitingDeliveryV1 string

const (
	ReviewWaitingInlineV1   ReviewWaitingDeliveryV1 = "inline"
	ReviewWaitingDetachedV1 ReviewWaitingDeliveryV1 = "detached"
)

// ReviewPhasePointCoordinateV1 is an Application-neutral exact coordinate.
// It mirrors no Harness Fact and grants no right to mutate a Session.
type ReviewPhasePointCoordinateV1 struct {
	Kind            runtimeports.NamespacedNameV2 `json:"kind"`
	ID              string                        `json:"id"`
	Revision        core.Revision                 `json:"revision"`
	Digest          core.Digest                   `json:"digest"`
	CheckedUnixNano int64                         `json:"checked_unix_nano"`
	ExpiresUnixNano int64                         `json:"expires_unix_nano"`
}

func (c ReviewPhasePointCoordinateV1) Validate() error {
	if c.Kind != ReviewPhaseActionV1 && c.Kind != ReviewPhaseSubagentV1 && c.Kind != ReviewPhaseRunV1 {
		return reviewWaitingInvalidV1("Review waiting Phase kind is unsupported")
	}
	if !validReviewWaitingIDV1(c.ID) || c.Revision == 0 || c.Digest.Validate() != nil || c.CheckedUnixNano <= 0 || c.CheckedUnixNano >= c.ExpiresUnixNano {
		return reviewWaitingInvalidV1("Review waiting Phase coordinate is incomplete")
	}
	return nil
}

type ReviewWaitingTargetCoordinateV1 struct {
	TenantID        core.TenantID   `json:"tenant_id"`
	ID              string          `json:"id"`
	Revision        core.Revision   `json:"revision"`
	Digest          core.Digest     `json:"digest"`
	RunID           core.AgentRunID `json:"run_id"`
	CheckedUnixNano int64           `json:"checked_unix_nano"`
	ExpiresUnixNano int64           `json:"expires_unix_nano"`
}

func (c ReviewWaitingTargetCoordinateV1) Validate() error {
	if strings.TrimSpace(string(c.TenantID)) == "" || !validReviewWaitingIDV1(c.ID) || c.Revision == 0 || c.Digest.Validate() != nil || !validReviewWaitingIDV1(string(c.RunID)) || c.CheckedUnixNano <= 0 || c.CheckedUnixNano >= c.ExpiresUnixNano {
		return reviewWaitingInvalidV1("Review waiting Target coordinate is incomplete")
	}
	return nil
}

type ReviewRequestCoordinateV1 struct {
	TenantID core.TenantID `json:"tenant_id"`
	ID       string        `json:"id"`
	Revision core.Revision `json:"revision"`
	Digest   core.Digest   `json:"digest"`
	CaseID   string        `json:"case_id"`
}

func (c ReviewRequestCoordinateV1) Validate() error {
	if strings.TrimSpace(string(c.TenantID)) == "" || !validReviewWaitingIDV1(c.ID) || c.Revision == 0 || c.Digest.Validate() != nil || !validReviewWaitingIDV1(c.CaseID) {
		return reviewWaitingInvalidV1("Review Request coordinate is incomplete")
	}
	return nil
}

type ReviewWaitingRequestV1 struct {
	ContractVersion      string                          `json:"contract_version"`
	ID                   string                          `json:"id"`
	Revision             core.Revision                   `json:"revision"`
	Delivery             ReviewWaitingDeliveryV1         `json:"delivery"`
	ExecutionScope       core.ExecutionScope             `json:"execution_scope"`
	ExecutionScopeDigest core.Digest                     `json:"execution_scope_digest"`
	Phase                ReviewPhasePointCoordinateV1    `json:"phase"`
	Target               ReviewWaitingTargetCoordinateV1 `json:"target"`
	ReviewRequest        ReviewRequestCoordinateV1       `json:"review_request"`
	CreatedUnixNano      int64                           `json:"created_unix_nano"`
	ExpiresUnixNano      int64                           `json:"expires_unix_nano"`
	Digest               core.Digest                     `json:"digest"`
}

type reviewWaitingRequestIdentityV1 struct {
	Delivery             ReviewWaitingDeliveryV1         `json:"delivery"`
	ExecutionScopeDigest core.Digest                     `json:"execution_scope_digest"`
	Phase                ReviewPhasePointCoordinateV1    `json:"phase"`
	Target               ReviewWaitingTargetCoordinateV1 `json:"target"`
	ReviewRequest        ReviewRequestCoordinateV1       `json:"review_request"`
}

func (r ReviewWaitingRequestV1) identityV1() reviewWaitingRequestIdentityV1 {
	return reviewWaitingRequestIdentityV1{r.Delivery, r.ExecutionScopeDigest, r.Phase, r.Target, r.ReviewRequest}
}

func SealReviewWaitingRequestV1(r ReviewWaitingRequestV1) (ReviewWaitingRequestV1, error) {
	r.ContractVersion = ReviewWaitingContractVersionV1
	r.Revision = 1
	r.ID, r.Digest = "", ""
	idDigest, err := core.CanonicalJSONDigest("praxis.application.review-waiting", ReviewWaitingContractVersionV1, "ReviewWaitingRequestIdentityV1", r.identityV1())
	if err != nil {
		return ReviewWaitingRequestV1{}, err
	}
	r.ID = "review-waiting/" + strings.TrimPrefix(string(idDigest), "sha256:")
	r.Digest, err = r.DigestV1()
	if err != nil {
		return ReviewWaitingRequestV1{}, err
	}
	return r, r.Validate()
}

func (r ReviewWaitingRequestV1) DigestV1() (core.Digest, error) {
	copy := r
	copy.Digest = ""
	return core.CanonicalJSONDigest("praxis.application.review-waiting", ReviewWaitingContractVersionV1, "ReviewWaitingRequestV1", copy)
}

func (r ReviewWaitingRequestV1) Validate() error {
	if r.ContractVersion != ReviewWaitingContractVersionV1 || !validReviewWaitingIDV1(r.ID) || r.Revision != 1 || r.Digest.Validate() != nil || r.CreatedUnixNano <= 0 || r.CreatedUnixNano >= r.ExpiresUnixNano {
		return reviewWaitingInvalidV1("Review waiting Request identity or lifetime is incomplete")
	}
	if r.Delivery != ReviewWaitingInlineV1 && r.Delivery != ReviewWaitingDetachedV1 {
		return reviewWaitingInvalidV1("Review waiting Delivery is unsupported")
	}
	if err := r.ExecutionScope.Validate(); err != nil {
		return err
	}
	scopeDigest, err := runtimeports.ExecutionScopeDigestV2(r.ExecutionScope)
	if err != nil || scopeDigest != r.ExecutionScopeDigest {
		return core.NewError(core.ErrorConflict, core.ReasonEffectFenceStale, "Review waiting ExecutionScope digest drifted")
	}
	if err := r.Phase.Validate(); err != nil {
		return err
	}
	if err := r.Target.Validate(); err != nil {
		return err
	}
	if err := r.ReviewRequest.Validate(); err != nil {
		return err
	}
	if r.ExecutionScope.Identity.TenantID != r.Target.TenantID || r.Target.TenantID != r.ReviewRequest.TenantID || r.ExpiresUnixNano > r.Phase.ExpiresUnixNano || r.ExpiresUnixNano > r.Target.ExpiresUnixNano {
		return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "Review waiting Request applicability drifted")
	}
	idDigest, err := core.CanonicalJSONDigest("praxis.application.review-waiting", ReviewWaitingContractVersionV1, "ReviewWaitingRequestIdentityV1", r.identityV1())
	if err != nil || r.ID != "review-waiting/"+strings.TrimPrefix(string(idDigest), "sha256:") {
		return core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Review waiting Request ID drifted")
	}
	digest, err := r.DigestV1()
	if err != nil || digest != r.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Review waiting Request digest drifted")
	}
	return nil
}

func (r ReviewWaitingRequestV1) ValidateCurrent(now time.Time) error {
	if err := r.Validate(); err != nil {
		return err
	}
	if now.IsZero() || now.UnixNano() < r.CreatedUnixNano || now.UnixNano() < r.Phase.CheckedUnixNano || now.UnixNano() < r.Target.CheckedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Review waiting Request clock regressed")
	}
	if !now.Before(time.Unix(0, r.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "Review waiting Request expired")
	}
	return nil
}

type ReviewWaitingInputSubjectV1 struct {
	TenantID  core.TenantID                 `json:"tenant_id"`
	RunID     core.AgentRunID               `json:"run_id"`
	PhaseKind runtimeports.NamespacedNameV2 `json:"phase_kind"`
	PhaseID   string                        `json:"phase_id"`
}

func (r ReviewWaitingRequestV1) InputSubjectV1() ReviewWaitingInputSubjectV1 {
	return ReviewWaitingInputSubjectV1{TenantID: r.Target.TenantID, RunID: r.Target.RunID, PhaseKind: r.Phase.Kind, PhaseID: r.Phase.ID}
}

func (s ReviewWaitingInputSubjectV1) Validate() error {
	if strings.TrimSpace(string(s.TenantID)) == "" || !validReviewWaitingIDV1(string(s.RunID)) || (s.PhaseKind != ReviewPhaseActionV1 && s.PhaseKind != ReviewPhaseSubagentV1 && s.PhaseKind != ReviewPhaseRunV1) || !validReviewWaitingIDV1(s.PhaseID) {
		return reviewWaitingInvalidV1("Review waiting input subject is incomplete")
	}
	return nil
}

type ReviewWaitingInputCurrentProjectionV1 struct {
	ContractVersion      string                          `json:"contract_version"`
	Subject              ReviewWaitingInputSubjectV1     `json:"subject"`
	Phase                ReviewPhasePointCoordinateV1    `json:"phase"`
	Target               ReviewWaitingTargetCoordinateV1 `json:"target"`
	ExecutionScopeDigest core.Digest                     `json:"execution_scope_digest"`
	CheckedUnixNano      int64                           `json:"checked_unix_nano"`
	ExpiresUnixNano      int64                           `json:"expires_unix_nano"`
	Digest               core.Digest                     `json:"digest"`
}

func SealReviewWaitingInputCurrentProjectionV1(p ReviewWaitingInputCurrentProjectionV1) (ReviewWaitingInputCurrentProjectionV1, error) {
	p.ContractVersion = ReviewWaitingContractVersionV1
	p.Digest = ""
	digest, err := p.DigestV1()
	if err != nil {
		return ReviewWaitingInputCurrentProjectionV1{}, err
	}
	p.Digest = digest
	return p, p.Validate()
}

func (p ReviewWaitingInputCurrentProjectionV1) DigestV1() (core.Digest, error) {
	copy := p
	copy.Digest = ""
	return core.CanonicalJSONDigest("praxis.application.review-waiting", ReviewWaitingContractVersionV1, "ReviewWaitingInputCurrentProjectionV1", copy)
}

func (p ReviewWaitingInputCurrentProjectionV1) Validate() error {
	if p.ContractVersion != ReviewWaitingContractVersionV1 || p.Subject.Validate() != nil || p.Phase.Validate() != nil || p.Target.Validate() != nil || p.ExecutionScopeDigest.Validate() != nil || p.CheckedUnixNano <= 0 || p.CheckedUnixNano < p.Phase.CheckedUnixNano || p.CheckedUnixNano < p.Target.CheckedUnixNano || p.CheckedUnixNano >= p.ExpiresUnixNano || p.ExpiresUnixNano > p.Phase.ExpiresUnixNano || p.ExpiresUnixNano > p.Target.ExpiresUnixNano {
		return reviewWaitingInvalidV1("Review waiting input current projection is incomplete")
	}
	if p.Subject != (ReviewWaitingInputSubjectV1{TenantID: p.Target.TenantID, RunID: p.Target.RunID, PhaseKind: p.Phase.Kind, PhaseID: p.Phase.ID}) {
		return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "Review waiting input source subject drifted")
	}
	digest, err := p.DigestV1()
	if err != nil || digest != p.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Review waiting input current digest drifted")
	}
	return nil
}

func (p ReviewWaitingInputCurrentProjectionV1) ValidateFor(request ReviewWaitingRequestV1, now time.Time) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if p.Subject != request.InputSubjectV1() || p.Phase != request.Phase || p.Target != request.Target || p.ExecutionScopeDigest != request.ExecutionScopeDigest {
		return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "Review waiting input current projection drifted")
	}
	if now.IsZero() || now.UnixNano() < p.CheckedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Review waiting input current clock regressed")
	}
	if !now.Before(time.Unix(0, p.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "Review waiting input current projection expired")
	}
	return nil
}

type ReviewWaitingCaseCoordinateV1 struct {
	TenantID        core.TenantID                   `json:"tenant_id"`
	ID              string                          `json:"id"`
	Revision        core.Revision                   `json:"revision"`
	Digest          core.Digest                     `json:"digest"`
	Target          ReviewWaitingTargetCoordinateV1 `json:"target"`
	ExpiresUnixNano int64                           `json:"expires_unix_nano"`
}

func (c ReviewWaitingCaseCoordinateV1) Validate() error {
	if strings.TrimSpace(string(c.TenantID)) == "" || !validReviewWaitingIDV1(c.ID) || c.Revision == 0 || c.Digest.Validate() != nil || c.Target.Validate() != nil || c.TenantID != c.Target.TenantID || c.ExpiresUnixNano <= 0 || c.ExpiresUnixNano > c.Target.ExpiresUnixNano {
		return reviewWaitingInvalidV1("Review waiting Case coordinate is incomplete")
	}
	return nil
}

type ReviewWaitingVerdictCoordinateV1 struct {
	TenantID        core.TenantID                   `json:"tenant_id"`
	ID              string                          `json:"id"`
	Revision        core.Revision                   `json:"revision"`
	Digest          core.Digest                     `json:"digest"`
	CaseID          string                          `json:"case_id"`
	CaseRevision    core.Revision                   `json:"case_revision"`
	CaseDigest      core.Digest                     `json:"case_digest"`
	Target          ReviewWaitingTargetCoordinateV1 `json:"target"`
	ExpiresUnixNano int64                           `json:"expires_unix_nano"`
}

func (c ReviewWaitingVerdictCoordinateV1) Validate() error {
	if strings.TrimSpace(string(c.TenantID)) == "" || !validReviewWaitingIDV1(c.ID) || c.Revision == 0 || c.Digest.Validate() != nil || !validReviewWaitingIDV1(c.CaseID) || c.CaseRevision == 0 || c.CaseDigest.Validate() != nil || c.Target.Validate() != nil || c.TenantID != c.Target.TenantID || c.ExpiresUnixNano <= 0 || c.ExpiresUnixNano > c.Target.ExpiresUnixNano {
		return reviewWaitingInvalidV1("Review waiting Verdict coordinate is incomplete")
	}
	return nil
}

type ReviewPhaseDecisionV1 string

const (
	ReviewPhaseAllowV1 ReviewPhaseDecisionV1 = "allow"
	ReviewPhaseDenyV1  ReviewPhaseDecisionV1 = "deny"
	ReviewPhaseAskV1   ReviewPhaseDecisionV1 = "ask"
	ReviewPhaseDeferV1 ReviewPhaseDecisionV1 = "defer"
)

type ReviewWaitingCurrentProjectionV1 struct {
	ContractVersion string                            `json:"contract_version"`
	RequestID       string                            `json:"request_id"`
	RequestDigest   core.Digest                       `json:"request_digest"`
	Case            ReviewWaitingCaseCoordinateV1     `json:"case"`
	Verdict         *ReviewWaitingVerdictCoordinateV1 `json:"verdict,omitempty"`
	Decision        ReviewPhaseDecisionV1             `json:"decision"`
	Current         bool                              `json:"current"`
	CheckedUnixNano int64                             `json:"checked_unix_nano"`
	ExpiresUnixNano int64                             `json:"expires_unix_nano"`
	Digest          core.Digest                       `json:"digest"`
}

func (p ReviewWaitingCurrentProjectionV1) Clone() ReviewWaitingCurrentProjectionV1 {
	if p.Verdict != nil {
		verdict := *p.Verdict
		p.Verdict = &verdict
	}
	return p
}

func SealReviewWaitingCurrentProjectionV1(p ReviewWaitingCurrentProjectionV1) (ReviewWaitingCurrentProjectionV1, error) {
	p = p.Clone()
	p.ContractVersion = ReviewWaitingContractVersionV1
	p.Digest = ""
	digest, err := p.DigestV1()
	if err != nil {
		return ReviewWaitingCurrentProjectionV1{}, err
	}
	p.Digest = digest
	return p, p.Validate()
}

func (p ReviewWaitingCurrentProjectionV1) DigestV1() (core.Digest, error) {
	copy := p.Clone()
	copy.Digest = ""
	return core.CanonicalJSONDigest("praxis.application.review-waiting", ReviewWaitingContractVersionV1, "ReviewWaitingCurrentProjectionV1", copy)
}

func (p ReviewWaitingCurrentProjectionV1) Validate() error {
	if p.ContractVersion != ReviewWaitingContractVersionV1 || !validReviewWaitingIDV1(p.RequestID) || p.RequestDigest.Validate() != nil || p.Case.Validate() != nil || !p.Current || p.CheckedUnixNano <= 0 || p.CheckedUnixNano >= p.ExpiresUnixNano || p.ExpiresUnixNano > p.Case.ExpiresUnixNano {
		return reviewWaitingInvalidV1("Review waiting current projection is incomplete")
	}
	switch p.Decision {
	case ReviewPhaseAllowV1:
		if p.Verdict == nil {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictMissing, "Review allow requires an exact Verdict")
		}
	case ReviewPhaseDenyV1, ReviewPhaseAskV1, ReviewPhaseDeferV1:
	default:
		return reviewWaitingInvalidV1("Review phase decision is unsupported")
	}
	if p.Decision == ReviewPhaseDeferV1 && p.Verdict != nil {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidState, "deferred Review cannot claim a Verdict")
	}
	if p.Verdict != nil {
		if err := p.Verdict.Validate(); err != nil {
			return err
		}
		if p.Verdict.TenantID != p.Case.TenantID || p.Verdict.CaseID != p.Case.ID || p.Verdict.CaseRevision+1 != p.Case.Revision || p.Verdict.Target != p.Case.Target || p.ExpiresUnixNano > p.Verdict.ExpiresUnixNano {
			return core.NewError(core.ErrorConflict, core.ReasonReviewVerdictStale, "Review Verdict does not atomically resolve the exact Case")
		}
	}
	digest, err := p.DigestV1()
	if err != nil || digest != p.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Review waiting current projection digest drifted")
	}
	return nil
}

func (p ReviewWaitingCurrentProjectionV1) ValidateFor(request ReviewWaitingRequestV1, now time.Time) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if p.RequestID != request.ReviewRequest.ID || p.RequestDigest != request.ReviewRequest.Digest || p.Case.TenantID != request.Target.TenantID || p.Case.ID != request.ReviewRequest.CaseID || p.Case.Target != request.Target || p.ExpiresUnixNano > request.ExpiresUnixNano {
		return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "Review waiting current projection belongs to another request or Target")
	}
	if now.IsZero() || now.UnixNano() < p.CheckedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Review waiting current projection clock regressed")
	}
	if !now.Before(time.Unix(0, p.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "Review waiting current projection expired")
	}
	return nil
}

type ReviewWaitingInspectRequestV1 struct {
	Request ReviewRequestCoordinateV1       `json:"request"`
	Target  ReviewWaitingTargetCoordinateV1 `json:"target"`
}

func (r ReviewWaitingInspectRequestV1) Validate() error {
	if r.Request.Validate() != nil || r.Target.Validate() != nil || r.Request.TenantID != r.Target.TenantID {
		return reviewWaitingInvalidV1("Review waiting Inspect request is incomplete")
	}
	return nil
}

type ReviewWaitingCoordinationRefV1 struct {
	ID       string        `json:"id"`
	Revision core.Revision `json:"revision"`
	Digest   core.Digest   `json:"digest"`
}

func (r ReviewWaitingCoordinationRefV1) Validate() error {
	if !validReviewWaitingIDV1(r.ID) || r.Revision == 0 || r.Digest.Validate() != nil {
		return reviewWaitingInvalidV1("Review waiting coordination Ref is incomplete")
	}
	return nil
}

type ReviewPhaseReceiptV1 struct {
	ContractVersion string                            `json:"contract_version"`
	ID              string                            `json:"id"`
	Revision        core.Revision                     `json:"revision"`
	Coordination    ReviewWaitingCoordinationRefV1    `json:"coordination"`
	RequestID       string                            `json:"request_id"`
	RequestDigest   core.Digest                       `json:"request_digest"`
	Phase           ReviewPhasePointCoordinateV1      `json:"phase"`
	Target          ReviewWaitingTargetCoordinateV1   `json:"target"`
	Case            ReviewWaitingCaseCoordinateV1     `json:"case"`
	Verdict         *ReviewWaitingVerdictCoordinateV1 `json:"verdict,omitempty"`
	Decision        ReviewPhaseDecisionV1             `json:"decision"`
	CheckedUnixNano int64                             `json:"checked_unix_nano"`
	ExpiresUnixNano int64                             `json:"expires_unix_nano"`
	Digest          core.Digest                       `json:"digest"`
}

// ReviewWaitingOutcomeV1 is the Application return view. A deferred outcome
// carries no Phase receipt; terminal outcomes carry the exact persisted receipt.
// It is not a Review Verdict, Harness Session mutation or Runtime authorization.
type ReviewWaitingOutcomeV1 struct {
	Coordination ReviewWaitingCoordinationRefV1   `json:"coordination"`
	Review       ReviewWaitingCurrentProjectionV1 `json:"review"`
	Receipt      *ReviewPhaseReceiptV1            `json:"receipt,omitempty"`
}

func (o ReviewWaitingOutcomeV1) Clone() ReviewWaitingOutcomeV1 {
	o.Review = o.Review.Clone()
	if o.Receipt != nil {
		value := o.Receipt.Clone()
		o.Receipt = &value
	}
	return o
}

func (o ReviewWaitingOutcomeV1) ValidateFor(request ReviewWaitingRequestV1, now time.Time) error {
	if err := o.Coordination.Validate(); err != nil {
		return err
	}
	if err := o.Review.ValidateFor(request, now); err != nil {
		return err
	}
	if o.Review.Decision == ReviewPhaseDeferV1 {
		if o.Receipt != nil {
			return core.NewError(core.ErrorConflict, core.ReasonInvalidState, "deferred Review outcome cannot contain a Phase receipt")
		}
		return nil
	}
	if o.Receipt == nil {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictMissing, "terminal Review outcome lacks its Phase receipt")
	}
	if err := o.Receipt.ValidateCurrentFor(request, now); err != nil {
		return err
	}
	if o.Receipt.Case != o.Review.Case || o.Receipt.Decision != o.Review.Decision {
		return core.NewError(core.ErrorConflict, core.ReasonReviewVerdictStale, "Review outcome receipt drifted from current Review projection")
	}
	if (o.Receipt.Verdict == nil) != (o.Review.Verdict == nil) || o.Receipt.Verdict != nil && *o.Receipt.Verdict != *o.Review.Verdict {
		return core.NewError(core.ErrorConflict, core.ReasonReviewVerdictStale, "Review outcome receipt Verdict drifted from current Review projection")
	}
	return nil
}

func (r ReviewPhaseReceiptV1) Clone() ReviewPhaseReceiptV1 {
	if r.Verdict != nil {
		verdict := *r.Verdict
		r.Verdict = &verdict
	}
	return r
}

func SealReviewPhaseReceiptV1(r ReviewPhaseReceiptV1, request ReviewWaitingRequestV1, current ReviewWaitingCurrentProjectionV1, input ReviewWaitingInputCurrentProjectionV1, now time.Time) (ReviewPhaseReceiptV1, error) {
	if current.Decision == ReviewPhaseDeferV1 {
		return ReviewPhaseReceiptV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "deferred Review is not a terminal Phase receipt")
	}
	r = r.Clone()
	r.ContractVersion = ReviewWaitingContractVersionV1
	r.Revision = 1
	r.RequestID, r.RequestDigest = request.ID, request.Digest
	r.Phase, r.Target, r.Case, r.Verdict, r.Decision = request.Phase, request.Target, current.Case, current.Verdict, current.Decision
	r.CheckedUnixNano = now.UnixNano()
	r.ExpiresUnixNano = minReviewWaitingExpiryV1(request.ExpiresUnixNano, input.ExpiresUnixNano, current.ExpiresUnixNano)
	r.ID, r.Digest = "", ""
	idDigest, err := core.CanonicalJSONDigest("praxis.application.review-waiting", ReviewWaitingContractVersionV1, "ReviewPhaseReceiptIdentityV1", struct {
		Coordination  ReviewWaitingCoordinationRefV1 `json:"coordination"`
		RequestID     string                         `json:"request_id"`
		RequestDigest core.Digest                    `json:"request_digest"`
		Case          ReviewWaitingCaseCoordinateV1  `json:"case"`
		Decision      ReviewPhaseDecisionV1          `json:"decision"`
	}{r.Coordination, r.RequestID, r.RequestDigest, r.Case, r.Decision})
	if err != nil {
		return ReviewPhaseReceiptV1{}, err
	}
	r.ID = "review-phase-receipt/" + strings.TrimPrefix(string(idDigest), "sha256:")
	r.Digest, err = r.DigestV1()
	if err != nil {
		return ReviewPhaseReceiptV1{}, err
	}
	return r, r.ValidateCurrentFor(request, now)
}

func (r ReviewPhaseReceiptV1) DigestV1() (core.Digest, error) {
	copy := r.Clone()
	copy.Digest = ""
	return core.CanonicalJSONDigest("praxis.application.review-waiting", ReviewWaitingContractVersionV1, "ReviewPhaseReceiptV1", copy)
}

func (r ReviewPhaseReceiptV1) ValidateCurrentFor(request ReviewWaitingRequestV1, now time.Time) error {
	if err := request.ValidateCurrent(now); err != nil {
		return err
	}
	if r.ContractVersion != ReviewWaitingContractVersionV1 || !validReviewWaitingIDV1(r.ID) || r.Revision != 1 || r.Coordination.Validate() != nil || r.RequestID != request.ID || r.RequestDigest != request.Digest || r.Phase != request.Phase || r.Target != request.Target || r.Case.Validate() != nil || r.Case.Target != request.Target || r.Case.ID != request.ReviewRequest.CaseID || r.Decision == ReviewPhaseDeferV1 || r.CheckedUnixNano <= 0 || r.CheckedUnixNano >= r.ExpiresUnixNano || r.ExpiresUnixNano > request.ExpiresUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "Review Phase receipt is incomplete or stale")
	}
	if now.IsZero() || now.UnixNano() < r.CheckedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Review Phase receipt clock regressed")
	}
	if !now.Before(time.Unix(0, r.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "Review Phase receipt expired")
	}
	if r.Decision == ReviewPhaseAllowV1 && r.Verdict == nil {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictMissing, "allow Phase receipt lacks exact Verdict")
	}
	if r.Verdict != nil {
		if err := r.Verdict.Validate(); err != nil {
			return err
		}
		if r.Verdict.CaseID != r.Case.ID || r.Verdict.CaseRevision+1 != r.Case.Revision || r.Verdict.Target != r.Target || r.ExpiresUnixNano > r.Verdict.ExpiresUnixNano {
			return core.NewError(core.ErrorConflict, core.ReasonReviewVerdictStale, "Review Phase receipt Verdict drifted")
		}
	}
	digest, err := r.DigestV1()
	if err != nil || digest != r.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Review Phase receipt digest drifted")
	}
	return nil
}

type ReviewWaitingCoordinationStateV1 string

const (
	ReviewWaitingStateV1    ReviewWaitingCoordinationStateV1 = "waiting_review"
	ReviewInspectStateV1    ReviewWaitingCoordinationStateV1 = "waiting_inspect"
	ReviewCompletedStateV1  ReviewWaitingCoordinationStateV1 = "completed"
	ReviewSupersededStateV1 ReviewWaitingCoordinationStateV1 = "superseded"
)

type ReviewWaitingCoordinationFactV1 struct {
	ContractVersion    string                           `json:"contract_version"`
	ID                 string                           `json:"id"`
	Revision           core.Revision                    `json:"revision"`
	PreviousDigest     core.Digest                      `json:"previous_digest,omitempty"`
	State              ReviewWaitingCoordinationStateV1 `json:"state"`
	Request            ReviewWaitingRequestV1           `json:"request"`
	StartClaimID       string                           `json:"start_claim_id,omitempty"`
	Case               *ReviewWaitingCaseCoordinateV1   `json:"case,omitempty"`
	Receipt            *ReviewPhaseReceiptV1            `json:"receipt,omitempty"`
	InvalidationReason core.ReasonCode                  `json:"invalidation_reason,omitempty"`
	CreatedUnixNano    int64                            `json:"created_unix_nano"`
	UpdatedUnixNano    int64                            `json:"updated_unix_nano"`
	ExpiresUnixNano    int64                            `json:"expires_unix_nano"`
	Digest             core.Digest                      `json:"digest"`
}

func (f ReviewWaitingCoordinationFactV1) Clone() ReviewWaitingCoordinationFactV1 {
	f.Request.ExecutionScope = cloneReviewWaitingScopeV1(f.Request.ExecutionScope)
	if f.Case != nil {
		value := *f.Case
		f.Case = &value
	}
	if f.Receipt != nil {
		value := f.Receipt.Clone()
		f.Receipt = &value
	}
	return f
}

func NewReviewWaitingCoordinationFactV1(request ReviewWaitingRequestV1, now time.Time) (ReviewWaitingCoordinationFactV1, error) {
	if err := request.ValidateCurrent(now); err != nil {
		return ReviewWaitingCoordinationFactV1{}, err
	}
	fact := ReviewWaitingCoordinationFactV1{ContractVersion: ReviewWaitingContractVersionV1, ID: request.ID, Revision: 1, State: ReviewWaitingStateV1, Request: request, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano(), ExpiresUnixNano: request.ExpiresUnixNano}
	return SealReviewWaitingCoordinationFactV1(fact)
}

func (f ReviewWaitingCoordinationFactV1) RefV1() ReviewWaitingCoordinationRefV1 {
	return ReviewWaitingCoordinationRefV1{ID: f.ID, Revision: f.Revision, Digest: f.Digest}
}

func SealReviewWaitingCoordinationFactV1(f ReviewWaitingCoordinationFactV1) (ReviewWaitingCoordinationFactV1, error) {
	f = f.Clone()
	f.ContractVersion = ReviewWaitingContractVersionV1
	f.Digest = ""
	digest, err := f.DigestV1()
	if err != nil {
		return ReviewWaitingCoordinationFactV1{}, err
	}
	f.Digest = digest
	return f, f.Validate()
}

func (f ReviewWaitingCoordinationFactV1) DigestV1() (core.Digest, error) {
	copy := f.Clone()
	copy.Digest = ""
	return core.CanonicalJSONDigest("praxis.application.review-waiting", ReviewWaitingContractVersionV1, "ReviewWaitingCoordinationFactV1", copy)
}

func (f ReviewWaitingCoordinationFactV1) Validate() error {
	if f.ContractVersion != ReviewWaitingContractVersionV1 || f.ID != f.Request.ID || f.Revision == 0 || f.Request.Validate() != nil || f.CreatedUnixNano <= 0 || f.UpdatedUnixNano < f.CreatedUnixNano || f.ExpiresUnixNano != f.Request.ExpiresUnixNano {
		return reviewWaitingInvalidV1("Review waiting coordination Fact is incomplete")
	}
	if f.Revision == 1 && f.PreviousDigest != "" || f.Revision > 1 && f.PreviousDigest.Validate() != nil {
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Review waiting coordination history link is incomplete")
	}
	switch f.State {
	case ReviewWaitingStateV1:
		if f.StartClaimID != "" || f.Case != nil || f.Receipt != nil || f.InvalidationReason != "" {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "waiting_review Fact contains post-start data")
		}
	case ReviewInspectStateV1:
		if !validReviewWaitingIDV1(f.StartClaimID) || f.Receipt != nil || f.InvalidationReason != "" {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "waiting_inspect Fact lacks its unique start claim")
		}
		if f.Case != nil && (f.Case.Validate() != nil || f.Case.ID != f.Request.ReviewRequest.CaseID || f.Case.Target != f.Request.Target) {
			return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "waiting_inspect Fact Case drifted")
		}
	case ReviewCompletedStateV1:
		if !validReviewWaitingIDV1(f.StartClaimID) || f.Case == nil || f.Receipt == nil || f.InvalidationReason != "" || f.Case.ID != f.Request.ReviewRequest.CaseID || f.Case.Target != f.Request.Target || f.Receipt.Case != *f.Case || f.Receipt.RequestID != f.Request.ID || f.Receipt.RequestDigest != f.Request.Digest {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictMissing, "completed Review waiting Fact lacks its exact receipt closure")
		}
	case ReviewSupersededStateV1:
		if f.Receipt != nil || f.InvalidationReason == "" {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewCandidateConflict, "superseded Review waiting Fact lacks its reason")
		}
	default:
		return reviewWaitingInvalidV1("Review waiting coordination state is unsupported")
	}
	digest, err := f.DigestV1()
	if err != nil || digest != f.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Review waiting coordination Fact digest drifted")
	}
	return nil
}

func ClaimReviewWaitingStartV1(current ReviewWaitingCoordinationFactV1, claimID string, now time.Time) (ReviewWaitingCoordinationFactV1, error) {
	if current.State != ReviewWaitingStateV1 || !validReviewWaitingIDV1(claimID) {
		return ReviewWaitingCoordinationFactV1{}, reviewWaitingInvalidV1("Review waiting start claim requires waiting_review")
	}
	next := current.Clone()
	next.Revision++
	next.PreviousDigest = current.Digest
	next.State = ReviewInspectStateV1
	next.StartClaimID = claimID
	next.UpdatedUnixNano = now.UnixNano()
	next.Digest = ""
	sealed, err := SealReviewWaitingCoordinationFactV1(next)
	if err != nil {
		return ReviewWaitingCoordinationFactV1{}, err
	}
	return sealed, ValidateReviewWaitingCoordinationTransitionV1(current, sealed)
}

func RecordReviewWaitingCurrentV1(current ReviewWaitingCoordinationFactV1, projection ReviewWaitingCurrentProjectionV1, now time.Time) (ReviewWaitingCoordinationFactV1, error) {
	if current.State != ReviewInspectStateV1 || projection.Decision != ReviewPhaseDeferV1 {
		return ReviewWaitingCoordinationFactV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidTransition, "only deferred Review can remain waiting_inspect")
	}
	next := current.Clone()
	next.Revision++
	next.PreviousDigest = current.Digest
	caseValue := projection.Case
	next.Case = &caseValue
	next.UpdatedUnixNano = now.UnixNano()
	next.Digest = ""
	sealed, err := SealReviewWaitingCoordinationFactV1(next)
	if err != nil {
		return ReviewWaitingCoordinationFactV1{}, err
	}
	return sealed, ValidateReviewWaitingCoordinationTransitionV1(current, sealed)
}

func CompleteReviewWaitingV1(current ReviewWaitingCoordinationFactV1, receipt ReviewPhaseReceiptV1, now time.Time) (ReviewWaitingCoordinationFactV1, error) {
	if current.State != ReviewInspectStateV1 || receipt.Coordination != current.RefV1() {
		return ReviewWaitingCoordinationFactV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidTransition, "Review completion does not bind the waiting_inspect predecessor")
	}
	next := current.Clone()
	next.Revision++
	next.PreviousDigest = current.Digest
	next.State = ReviewCompletedStateV1
	caseValue, receiptValue := receipt.Case, receipt.Clone()
	next.Case, next.Receipt = &caseValue, &receiptValue
	next.UpdatedUnixNano = now.UnixNano()
	next.Digest = ""
	sealed, err := SealReviewWaitingCoordinationFactV1(next)
	if err != nil {
		return ReviewWaitingCoordinationFactV1{}, err
	}
	return sealed, ValidateReviewWaitingCoordinationTransitionV1(current, sealed)
}

func SupersedeReviewWaitingV1(current ReviewWaitingCoordinationFactV1, reason core.ReasonCode, now time.Time) (ReviewWaitingCoordinationFactV1, error) {
	if current.State != ReviewWaitingStateV1 && current.State != ReviewInspectStateV1 && current.State != ReviewCompletedStateV1 || reason == "" {
		return ReviewWaitingCoordinationFactV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidTransition, "Review supersede requires a live wait and reason")
	}
	next := current.Clone()
	next.Revision++
	next.PreviousDigest = current.Digest
	next.State = ReviewSupersededStateV1
	next.Receipt = nil
	next.InvalidationReason = reason
	next.UpdatedUnixNano = now.UnixNano()
	next.Digest = ""
	sealed, err := SealReviewWaitingCoordinationFactV1(next)
	if err != nil {
		return ReviewWaitingCoordinationFactV1{}, err
	}
	return sealed, ValidateReviewWaitingCoordinationTransitionV1(current, sealed)
}

func ValidateReviewWaitingCoordinationTransitionV1(current, next ReviewWaitingCoordinationFactV1) error {
	if current.Validate() != nil || next.Validate() != nil || current.ID != next.ID || current.Request.Digest != next.Request.Digest || next.Revision != current.Revision+1 || next.PreviousDigest != current.Digest || next.CreatedUnixNano != current.CreatedUnixNano || next.ExpiresUnixNano != current.ExpiresUnixNano || next.UpdatedUnixNano < current.UpdatedUnixNano {
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Review waiting coordination immutable content or history drifted")
	}
	if current.State == ReviewSupersededStateV1 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidTransition, "terminal Review waiting coordination cannot advance")
	}
	if current.StartClaimID != "" && current.StartClaimID != next.StartClaimID {
		return core.NewError(core.ErrorConflict, core.ReasonOwnerConflict, "Review waiting start claim changed")
	}
	allowed := current.State == ReviewWaitingStateV1 && (next.State == ReviewInspectStateV1 || next.State == ReviewSupersededStateV1) || current.State == ReviewInspectStateV1 && (next.State == ReviewInspectStateV1 || next.State == ReviewCompletedStateV1 || next.State == ReviewSupersededStateV1) || current.State == ReviewCompletedStateV1 && next.State == ReviewSupersededStateV1
	if !allowed {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidTransition, "Review waiting coordination transition is unsupported")
	}
	if current.Case != nil && next.Case != nil {
		if current.Case.ID != next.Case.ID || current.Case.Target != next.Case.Target || next.Case.Revision < current.Case.Revision || next.Case.Revision == current.Case.Revision && next.Case.Digest != current.Case.Digest {
			return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "Review waiting Case history regressed or drifted")
		}
	}
	return nil
}

func cloneReviewWaitingScopeV1(scope core.ExecutionScope) core.ExecutionScope {
	if scope.SandboxLease != nil {
		lease := *scope.SandboxLease
		scope.SandboxLease = &lease
	}
	return scope
}

func minReviewWaitingExpiryV1(values ...int64) int64 {
	minimum := int64(0)
	for _, value := range values {
		if value > 0 && (minimum == 0 || value < minimum) {
			minimum = value
		}
	}
	return minimum
}

func validReviewWaitingIDV1(value string) bool {
	return value != "" && len(value) <= MaxReviewWaitingIDBytesV1 && strings.TrimSpace(value) == value
}

func reviewWaitingInvalidV1(message string) error {
	return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, message)
}
