package contract

import (
	"math"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const (
	ReviewWaitingInputCurrentContractVersionV2  = "praxis.application.review-waiting-input-current/v2"
	ReviewWaitingInputSourceContractVersionV1   = "praxis.harness.review-phase-source/v1"
	MaxReviewWaitingInputCurrentProjectionTTLV2 = 15 * time.Second

	reviewWaitingInputCurrentCanonicalDomainV2 = "praxis.application.review-waiting-input-current"
)

// ReviewWaitingInputSourceRefV2 is an Application-neutral mirror of the exact
// Harness source selected before Application coordination. It is not a Harness
// Fact, Review Verdict, Runtime Evidence, or authority grant. Kind and
// ContractVersion form a closed nominal pair; ID/Revision/Digest are copied
// losslessly from the Harness Owner ref, and NotAfter is copied from the
// current Owner projection after its own S1/S2 closure.
type ReviewWaitingInputSourceRefV2 struct {
	ContractVersion  string                        `json:"contract_version"`
	Kind             runtimeports.NamespacedNameV2 `json:"kind"`
	ID               string                        `json:"id"`
	Revision         core.Revision                 `json:"revision"`
	Digest           core.Digest                   `json:"digest"`
	NotAfterUnixNano int64                         `json:"not_after_unix_nano"`
}

func (r ReviewWaitingInputSourceRefV2) Validate() error {
	if r.ContractVersion != ReviewWaitingInputSourceContractVersionV1 || !validReviewWaitingInputSourceIDV2(r.ID) || r.Revision == 0 || r.Digest.Validate() != nil || r.NotAfterUnixNano <= 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Review waiting input source ref is incomplete")
	}
	switch r.Kind {
	case ReviewPhaseActionV1, ReviewPhaseSubagentV1, ReviewPhaseRunV1:
		return nil
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonUnknownGovernanceCategory, "Review waiting input source kind is outside the closed union")
	}
}

// ReviewWaitingInputCurrentRequestV2 is the only exact-current request for the
// V2 Reader. Subject provides the closed tenant/run/phase lookup coordinate;
// Source, ExpectedPhase, ExpectedTarget, and ExecutionScopeDigest prevent a
// weak by-name/latest read or cross-owner type pun.
type ReviewWaitingInputCurrentRequestV2 struct {
	ContractVersion             string                          `json:"contract_version"`
	Subject                     ReviewWaitingInputSubjectV1     `json:"subject"`
	Source                      ReviewWaitingInputSourceRefV2   `json:"source"`
	ExpectedSourceClosureDigest core.Digest                     `json:"expected_source_closure_digest"`
	ExpectedPhase               ReviewPhasePointCoordinateV1    `json:"expected_phase"`
	ExpectedTarget              ReviewWaitingTargetCoordinateV1 `json:"expected_target"`
	ExecutionScopeDigest        core.Digest                     `json:"execution_scope_digest"`
	Digest                      core.Digest                     `json:"digest"`
}

func (r ReviewWaitingInputCurrentRequestV2) Clone() ReviewWaitingInputCurrentRequestV2 { return r }

func (r ReviewWaitingInputCurrentRequestV2) DigestV2() (core.Digest, error) {
	copy := r.Clone()
	copy.Digest = ""
	return core.CanonicalJSONDigest(reviewWaitingInputCurrentCanonicalDomainV2, ReviewWaitingInputCurrentContractVersionV2, "ReviewWaitingInputCurrentRequestV2", copy)
}

func SealReviewWaitingInputCurrentRequestV2(r ReviewWaitingInputCurrentRequestV2) (ReviewWaitingInputCurrentRequestV2, error) {
	r.ContractVersion = ReviewWaitingInputCurrentContractVersionV2
	r.Digest = ""
	digest, err := r.DigestV2()
	if err != nil {
		return ReviewWaitingInputCurrentRequestV2{}, err
	}
	r.Digest = digest
	return r.Clone(), r.Validate()
}

func (r ReviewWaitingInputCurrentRequestV2) Validate() error {
	if r.ContractVersion != ReviewWaitingInputCurrentContractVersionV2 || r.Subject.Validate() != nil || r.Source.Validate() != nil || r.ExpectedSourceClosureDigest.Validate() != nil || r.ExpectedPhase.Validate() != nil || r.ExpectedTarget.Validate() != nil || r.ExecutionScopeDigest.Validate() != nil || r.Digest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Review waiting input current request is incomplete")
	}
	expectedSubject := ReviewWaitingInputSubjectV1{TenantID: r.ExpectedTarget.TenantID, RunID: r.ExpectedTarget.RunID, PhaseKind: r.ExpectedPhase.Kind, PhaseID: r.ExpectedPhase.ID}
	if r.Subject != expectedSubject || r.Source.Kind != r.Subject.PhaseKind {
		return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "Review waiting input request source, phase, target, or subject drifted")
	}
	digest, err := r.DigestV2()
	if err != nil || digest != r.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Review waiting input current request digest drifted")
	}
	return nil
}

func (r ReviewWaitingInputCurrentRequestV2) ValidateCurrent(now time.Time) error {
	if err := r.Validate(); err != nil {
		return err
	}
	if now.IsZero() || now.UnixNano() < r.ExpectedPhase.CheckedUnixNano || now.UnixNano() < r.ExpectedTarget.CheckedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Review waiting input current request clock regressed")
	}
	if !now.Before(time.Unix(0, reviewWaitingInputExpiryV2(r))) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "Review waiting input current request expired")
	}
	return nil
}

// ReviewWaitingInputCurrentProjectionV2 is a sealed neutral observation over
// one exact request. Current=true is meaningful only after ValidateCurrentFor
// rechecks the same request and fresh now. It grants no Evidence or authority.
type ReviewWaitingInputCurrentProjectionV2 struct {
	ContractVersion      string                          `json:"contract_version"`
	RequestDigest        core.Digest                     `json:"request_digest"`
	Source               ReviewWaitingInputSourceRefV2   `json:"source"`
	SourceClosureDigest  core.Digest                     `json:"source_closure_digest"`
	Phase                ReviewPhasePointCoordinateV1    `json:"phase"`
	Target               ReviewWaitingTargetCoordinateV1 `json:"target"`
	ExecutionScopeDigest core.Digest                     `json:"execution_scope_digest"`
	Current              bool                            `json:"current"`
	CheckedUnixNano      int64                           `json:"checked_unix_nano"`
	ExpiresUnixNano      int64                           `json:"expires_unix_nano"`
	Digest               core.Digest                     `json:"digest"`
}

func (p ReviewWaitingInputCurrentProjectionV2) Clone() ReviewWaitingInputCurrentProjectionV2 {
	return p
}

func (p ReviewWaitingInputCurrentProjectionV2) DigestV2() (core.Digest, error) {
	copy := p.Clone()
	copy.Digest = ""
	return core.CanonicalJSONDigest(reviewWaitingInputCurrentCanonicalDomainV2, ReviewWaitingInputCurrentContractVersionV2, "ReviewWaitingInputCurrentProjectionV2", copy)
}

func SealReviewWaitingInputCurrentProjectionV2(p ReviewWaitingInputCurrentProjectionV2, request ReviewWaitingInputCurrentRequestV2, now time.Time) (ReviewWaitingInputCurrentProjectionV2, error) {
	if err := request.ValidateCurrent(now); err != nil {
		return ReviewWaitingInputCurrentProjectionV2{}, err
	}
	p.ContractVersion = ReviewWaitingInputCurrentContractVersionV2
	p.RequestDigest = request.Digest
	p.Source = request.Source
	p.SourceClosureDigest = request.ExpectedSourceClosureDigest
	p.Phase = request.ExpectedPhase
	p.Target = request.ExpectedTarget
	p.ExecutionScopeDigest = request.ExecutionScopeDigest
	p.Current = true
	if p.CheckedUnixNano <= 0 || p.CheckedUnixNano > math.MaxInt64-int64(MaxReviewWaitingInputCurrentProjectionTTLV2) {
		return ReviewWaitingInputCurrentProjectionV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Review waiting input current projection checked time is outside its bound")
	}
	p.ExpiresUnixNano = reviewWaitingInputProjectionExpiryV2(request, p.CheckedUnixNano)
	p.Digest = ""
	digest, err := p.DigestV2()
	if err != nil {
		return ReviewWaitingInputCurrentProjectionV2{}, err
	}
	p.Digest = digest
	return p.Clone(), p.ValidateCurrentFor(request, now)
}

func (p ReviewWaitingInputCurrentProjectionV2) Validate() error {
	if p.ContractVersion != ReviewWaitingInputCurrentContractVersionV2 || p.RequestDigest.Validate() != nil || p.Source.Validate() != nil || p.SourceClosureDigest.Validate() != nil || p.Phase.Validate() != nil || p.Target.Validate() != nil || p.ExecutionScopeDigest.Validate() != nil || !p.Current || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano || p.Digest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Review waiting input current projection is incomplete")
	}
	if p.CheckedUnixNano > math.MaxInt64-int64(MaxReviewWaitingInputCurrentProjectionTTLV2) || p.Source.Kind != p.Phase.Kind || p.CheckedUnixNano < p.Phase.CheckedUnixNano || p.CheckedUnixNano < p.Target.CheckedUnixNano || p.ExpiresUnixNano-p.CheckedUnixNano > int64(MaxReviewWaitingInputCurrentProjectionTTLV2) || p.ExpiresUnixNano > p.Source.NotAfterUnixNano || p.ExpiresUnixNano > p.Phase.ExpiresUnixNano || p.ExpiresUnixNano > p.Target.ExpiresUnixNano {
		return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "Review waiting input current projection owner closure drifted")
	}
	digest, err := p.DigestV2()
	if err != nil || digest != p.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Review waiting input current projection digest drifted")
	}
	return nil
}

func (p ReviewWaitingInputCurrentProjectionV2) ValidateCurrent(now time.Time) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if now.IsZero() || now.UnixNano() < p.CheckedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Review waiting input current projection clock regressed")
	}
	if !now.Before(time.Unix(0, p.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "Review waiting input current projection expired")
	}
	return nil
}

func (p ReviewWaitingInputCurrentProjectionV2) ValidateCurrentFor(request ReviewWaitingInputCurrentRequestV2, now time.Time) error {
	if err := request.ValidateCurrent(now); err != nil {
		return err
	}
	if err := p.ValidateCurrent(now); err != nil {
		return err
	}
	if p.RequestDigest != request.Digest || p.Source != request.Source || p.SourceClosureDigest != request.ExpectedSourceClosureDigest || p.Phase != request.ExpectedPhase || p.Target != request.ExpectedTarget || p.ExecutionScopeDigest != request.ExecutionScopeDigest || p.ExpiresUnixNano != reviewWaitingInputProjectionExpiryV2(request, p.CheckedUnixNano) {
		return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "Review waiting input current projection does not bind the exact request or minimum TTL")
	}
	return nil
}

func reviewWaitingInputExpiryV2(request ReviewWaitingInputCurrentRequestV2) int64 {
	minimum := request.Source.NotAfterUnixNano
	for _, value := range []int64{request.ExpectedPhase.ExpiresUnixNano, request.ExpectedTarget.ExpiresUnixNano} {
		if value > 0 && (minimum == 0 || value < minimum) {
			minimum = value
		}
	}
	return minimum
}

func reviewWaitingInputProjectionExpiryV2(request ReviewWaitingInputCurrentRequestV2, checkedUnixNano int64) int64 {
	minimum := reviewWaitingInputExpiryV2(request)
	capExpiry := checkedUnixNano + int64(MaxReviewWaitingInputCurrentProjectionTTLV2)
	if capExpiry < minimum {
		minimum = capExpiry
	}
	return minimum
}

func validReviewWaitingInputSourceIDV2(value string) bool {
	return strings.TrimSpace(value) != "" && len(value) <= MaxReviewWaitingIDBytesV1
}
