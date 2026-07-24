package contract

import (
	"encoding/json"
	"strings"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const ReviewModelInvocationAssociationContractVersionV1 = "praxis.agent-host.review-model-invocation-association/v1"

type ReviewAttemptExactCoordinateV1 struct {
	ID       string        `json:"id"`
	Revision core.Revision `json:"revision"`
	Digest   core.Digest   `json:"digest"`
}

func (r ReviewAttemptExactCoordinateV1) Validate() error {
	if err := ValidateIdentifierV1("review attempt id", r.ID); err != nil {
		return err
	}
	if r.Revision == 0 || r.Digest.Validate() != nil {
		return NewError(ErrorInvalidArgument, "review_attempt_ref_invalid", "Review Attempt exact coordinate is incomplete")
	}
	return nil
}

type ReviewModelInvocationAssociationSubjectV1 struct {
	TenantID      core.TenantID                  `json:"tenant_id"`
	ReviewAttempt ReviewAttemptExactCoordinateV1 `json:"review_attempt"`
}

func (s ReviewModelInvocationAssociationSubjectV1) Validate() error {
	if strings.TrimSpace(string(s.TenantID)) == "" {
		return NewError(ErrorInvalidArgument, "tenant_missing", "association TenantID is required")
	}
	return s.ReviewAttempt.Validate()
}

func (s ReviewModelInvocationAssociationSubjectV1) StableIDV1() (string, error) {
	if err := s.Validate(); err != nil {
		return "", err
	}
	digest, err := core.CanonicalJSONDigest("praxis.agent-host.review-model-invocation-association", "v1", "SubjectV1", s)
	if err != nil {
		return "", NewError(ErrorInvalidArgument, "canonical_subject_invalid", "association subject cannot be sealed")
	}
	return "review-model-association/" + strings.TrimPrefix(string(digest), "sha256:"), nil
}

type ReviewModelInvocationAssociationStateV1 string

const (
	ReviewModelInvocationAssociationActiveV1     ReviewModelInvocationAssociationStateV1 = "active"
	ReviewModelInvocationAssociationRevokedV1    ReviewModelInvocationAssociationStateV1 = "revoked"
	ReviewModelInvocationAssociationSupersededV1 ReviewModelInvocationAssociationStateV1 = "superseded"
)

type ReviewModelInvocationAssociationRefV1 struct {
	ContractVersion string                                    `json:"contract_version"`
	ID              string                                    `json:"id"`
	Revision        core.Revision                             `json:"revision"`
	Digest          core.Digest                               `json:"digest"`
	Subject         ReviewModelInvocationAssociationSubjectV1 `json:"subject"`
}

func (r ReviewModelInvocationAssociationRefV1) Validate() error {
	if r.ContractVersion != ReviewModelInvocationAssociationContractVersionV1 || r.Revision == 0 || r.Digest.Validate() != nil {
		return NewError(ErrorInvalidArgument, "association_ref_invalid", "association Ref is incomplete")
	}
	id, err := r.Subject.StableIDV1()
	if err != nil {
		return err
	}
	if id != r.ID {
		return NewError(ErrorConflict, "association_identity_drift", "association Ref identity drifted")
	}
	return nil
}

type ReviewModelInvocationAssociationFactV1 struct {
	ContractVersion string                                        `json:"contract_version"`
	ID              string                                        `json:"id"`
	Revision        core.Revision                                 `json:"revision"`
	Digest          core.Digest                                   `json:"digest"`
	PreviousDigest  core.Digest                                   `json:"previous_digest,omitempty"`
	Subject         ReviewModelInvocationAssociationSubjectV1     `json:"subject"`
	Command         modelinvoker.GovernedModelInvocationCommandV1 `json:"command"`
	CommandDigest   core.Digest                                   `json:"command_digest"`
	State           ReviewModelInvocationAssociationStateV1       `json:"state"`
	CheckedUnixNano int64                                         `json:"checked_unix_nano"`
	ExpiresUnixNano int64                                         `json:"expires_unix_nano"`
}

func (f ReviewModelInvocationAssociationFactV1) RefV1() ReviewModelInvocationAssociationRefV1 {
	return ReviewModelInvocationAssociationRefV1{f.ContractVersion, f.ID, f.Revision, f.Digest, f.Subject}
}
func (f ReviewModelInvocationAssociationFactV1) digestV1() (core.Digest, error) {
	copy := f
	copy.Digest = ""
	return core.CanonicalJSONDigest("praxis.agent-host.review-model-invocation-association", "v1", "FactV1", copy)
}
func (f ReviewModelInvocationAssociationFactV1) ValidateHistoricalV1() error {
	if f.ContractVersion != ReviewModelInvocationAssociationContractVersionV1 || f.Revision == 0 || f.Digest.Validate() != nil || f.CheckedUnixNano <= 0 || f.CheckedUnixNano >= f.ExpiresUnixNano {
		return NewError(ErrorInvalidArgument, "association_fact_invalid", "association Fact is incomplete")
	}
	if err := f.Subject.Validate(); err != nil {
		return err
	}
	if err := validateGovernedCommandV1(f.Command); err != nil {
		return err
	}
	if f.CheckedUnixNano < f.Command.CurrentRef.CheckedUnixNano {
		return NewError(ErrorConflict, "association_checked_before_model_current", "association Checked precedes the sealed Model current projection")
	}
	commandDigest, err := core.CanonicalJSONDigest("praxis.agent-host.review-model-invocation-association", "v1", "GovernedCommandV1", f.Command)
	if err != nil || commandDigest != f.CommandDigest {
		return NewError(ErrorConflict, "association_command_drift", "association command digest drifted")
	}
	if f.ExpiresUnixNano > minAssociationTimeV1(f.Command.CurrentRef.ExpiresUnixNano, f.Command.CurrentRef.NotAfterUnixNano) {
		return NewError(ErrorConflict, "association_ttl_drift", "association outlives Model currentness")
	}
	switch f.State {
	case ReviewModelInvocationAssociationActiveV1:
		if f.Revision != 1 || f.PreviousDigest != "" {
			return NewError(ErrorConflict, "association_active_revision_invalid", "active association must be revision one")
		}
	case ReviewModelInvocationAssociationRevokedV1, ReviewModelInvocationAssociationSupersededV1:
		if f.Revision < 2 || f.PreviousDigest.Validate() != nil {
			return NewError(ErrorConflict, "association_terminal_revision_invalid", "terminal association lacks predecessor")
		}
	default:
		return NewError(ErrorInvalidArgument, "association_state_invalid", "association state is unsupported")
	}
	if err := f.RefV1().Validate(); err != nil {
		return err
	}
	digest, err := f.digestV1()
	if err != nil || digest != f.Digest {
		return NewError(ErrorConflict, "association_digest_drift", "association Fact digest drifted")
	}
	return nil
}
func (f ReviewModelInvocationAssociationFactV1) ValidateCurrentV1(now time.Time) error {
	if err := f.ValidateHistoricalV1(); err != nil {
		return err
	}
	if now.IsZero() || now.UnixNano() < f.CheckedUnixNano {
		return NewError(ErrorPrecondition, "clock_regression", "association clock regressed")
	}
	if !now.Before(time.Unix(0, f.ExpiresUnixNano)) {
		return NewError(ErrorPrecondition, "association_expired", "association expired")
	}
	if f.State != ReviewModelInvocationAssociationActiveV1 {
		return NewError(ErrorPrecondition, "association_terminal", "association is terminal")
	}
	return nil
}

func SealReviewModelInvocationAssociationV1(f ReviewModelInvocationAssociationFactV1) (ReviewModelInvocationAssociationFactV1, error) {
	f.ContractVersion = ReviewModelInvocationAssociationContractVersionV1
	f.Revision = 1
	f.PreviousDigest = ""
	f.State = ReviewModelInvocationAssociationActiveV1
	id, err := f.Subject.StableIDV1()
	if err != nil {
		return ReviewModelInvocationAssociationFactV1{}, err
	}
	if f.ID != "" && f.ID != id {
		return ReviewModelInvocationAssociationFactV1{}, NewError(ErrorConflict, "association_identity_drift", "supplied association ID drifted")
	}
	f.ID = id
	if err = validateGovernedCommandV1(f.Command); err != nil {
		return ReviewModelInvocationAssociationFactV1{}, err
	}
	f.CommandDigest, err = core.CanonicalJSONDigest("praxis.agent-host.review-model-invocation-association", "v1", "GovernedCommandV1", f.Command)
	if err != nil {
		return ReviewModelInvocationAssociationFactV1{}, err
	}
	provided := f.Digest
	f.Digest = ""
	f.Digest, err = f.digestV1()
	if err != nil {
		return ReviewModelInvocationAssociationFactV1{}, err
	}
	if provided != "" && provided != f.Digest {
		return ReviewModelInvocationAssociationFactV1{}, NewError(ErrorConflict, "association_digest_drift", "supplied association digest drifted")
	}
	return f, f.ValidateHistoricalV1()
}
func SealReviewModelInvocationAssociationSuccessorV1(previous, next ReviewModelInvocationAssociationFactV1) (ReviewModelInvocationAssociationFactV1, error) {
	if err := previous.ValidateHistoricalV1(); err != nil {
		return ReviewModelInvocationAssociationFactV1{}, err
	}
	if previous.State != ReviewModelInvocationAssociationActiveV1 || (next.State != ReviewModelInvocationAssociationRevokedV1 && next.State != ReviewModelInvocationAssociationSupersededV1) {
		return ReviewModelInvocationAssociationFactV1{}, NewError(ErrorConflict, "association_transition_invalid", "association transition is unsupported")
	}
	next.ContractVersion, next.ID, next.Subject, next.Command, next.CommandDigest = previous.ContractVersion, previous.ID, previous.Subject, previous.Command, previous.CommandDigest
	next.Revision, next.PreviousDigest = previous.Revision+1, previous.Digest
	if next.CheckedUnixNano < previous.CheckedUnixNano || next.ExpiresUnixNano > previous.ExpiresUnixNano {
		return ReviewModelInvocationAssociationFactV1{}, NewError(ErrorConflict, "association_time_drift", "association successor time drifted")
	}
	provided := next.Digest
	next.Digest = ""
	var err error
	next.Digest, err = next.digestV1()
	if err != nil {
		return ReviewModelInvocationAssociationFactV1{}, err
	}
	if provided != "" && provided != next.Digest {
		return ReviewModelInvocationAssociationFactV1{}, NewError(ErrorConflict, "association_digest_drift", "successor digest drifted")
	}
	return next, next.ValidateHistoricalV1()
}
func ValidateReviewModelInvocationAssociationTransitionV1(previous, next ReviewModelInvocationAssociationFactV1) error {
	if err := previous.ValidateHistoricalV1(); err != nil {
		return err
	}
	if err := next.ValidateHistoricalV1(); err != nil {
		return err
	}
	if previous.State != ReviewModelInvocationAssociationActiveV1 || (next.State != ReviewModelInvocationAssociationRevokedV1 && next.State != ReviewModelInvocationAssociationSupersededV1) || next.ID != previous.ID || next.Subject != previous.Subject || next.Revision != previous.Revision+1 || next.PreviousDigest != previous.Digest || next.CommandDigest != previous.CommandDigest || next.CheckedUnixNano < previous.CheckedUnixNano || next.ExpiresUnixNano > previous.ExpiresUnixNano {
		return NewError(ErrorConflict, "association_transition_drift", "association transition drifted")
	}
	a, _ := json.Marshal(previous.Command)
	b, _ := json.Marshal(next.Command)
	if string(a) != string(b) {
		return NewError(ErrorConflict, "association_command_drift", "association command changed")
	}
	return nil
}
func validateGovernedCommandV1(command modelinvoker.GovernedModelInvocationCommandV1) error {
	if err := command.PreparedRef.Validate(); err != nil {
		return NewError(ErrorInvalidArgument, "model_prepared_ref_invalid", err.Error())
	}
	if err := command.CurrentRef.Validate(); err != nil {
		return NewError(ErrorInvalidArgument, "model_current_ref_invalid", err.Error())
	}
	if command.CurrentRef.Prepared != command.PreparedRef || command.AttemptRequestDigest != command.PreparedRef.UnifiedRequestDigest || command.DispatchSequence == 0 || command.ProviderAttemptOrdinal == 0 {
		return NewError(ErrorConflict, "model_command_lineage_drift", "governed Model command lineage drifted")
	}
	if _, err := modelinvoker.DigestGovernedRouteCallV1(command.Call); err != nil {
		return NewError(ErrorInvalidArgument, "model_route_call_invalid", err.Error())
	}
	return nil
}
func minAssociationTimeV1(values ...int64) int64 {
	var result int64
	for _, value := range values {
		if value > 0 && (result == 0 || value < result) {
			result = value
		}
	}
	return result
}
