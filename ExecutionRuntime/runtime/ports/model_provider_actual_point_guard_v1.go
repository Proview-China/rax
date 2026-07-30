package ports

import (
	"context"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const (
	ModelProviderActualPointGuardContractVersionV1 = "1.0.0"
	ModelTurnEffectKindV1                          = EffectKindV2("praxis.harness/model-turn")
	ModelInvokeCapabilityV1                        = CapabilityNameV2("praxis.model/invoke")
	ModelProviderBoundaryCrossedV1                 = "provider_boundary_crossed"
)

// ModelProviderBoundaryCurrentRefV1 is Model Owner truth selected by Harness.
// Runtime validates it but never creates or advances the boundary.
type ModelProviderBoundaryCurrentRefV1 struct {
	ContractVersion        string                        `json:"contract_version"`
	Owner                  core.OwnerRef                 `json:"owner"`
	ID                     string                        `json:"id"`
	Revision               core.Revision                 `json:"revision"`
	OperationDigest        core.Digest                   `json:"operation_digest"`
	EffectID               core.EffectIntentID           `json:"effect_id"`
	RuntimeAttempt         OperationDispatchAttemptRefV3 `json:"runtime_attempt"`
	DispatchSequence       uint64                        `json:"dispatch_sequence"`
	ProviderAttemptOrdinal uint32                        `json:"provider_attempt_ordinal"`
	AttemptRequestDigest   core.Digest                   `json:"attempt_request_digest"`
	AcknowledgementDigest  core.Digest                   `json:"acknowledgement_digest"`
	ExpiresUnixNano        int64                         `json:"expires_unix_nano"`
	Digest                 core.Digest                   `json:"digest"`
}

func (r ModelProviderBoundaryCurrentRefV1) Validate() error {
	if r.ContractVersion != ModelProviderActualPointGuardContractVersionV1 || r.ID == "" || r.Revision == 0 || r.EffectID == "" || r.DispatchSequence == 0 || r.ProviderAttemptOrdinal == 0 || r.ExpiresUnixNano <= 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Model provider boundary identity is incomplete")
	}
	if err := r.Owner.Validate(); err != nil {
		return err
	}
	for _, digest := range []core.Digest{r.OperationDigest, r.AttemptRequestDigest, r.AcknowledgementDigest, r.Digest} {
		if err := digest.Validate(); err != nil {
			return err
		}
	}
	if err := r.RuntimeAttempt.Validate(); err != nil {
		return err
	}
	if r.RuntimeAttempt.OperationDigest != r.OperationDigest || r.RuntimeAttempt.EffectID != r.EffectID {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidReference, "Model boundary binds another Runtime attempt")
	}
	expected, err := r.DigestV1()
	if err != nil || expected != r.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Model boundary ref digest drifted")
	}
	return nil
}

func SameModelProviderOperationDispatchAttemptV1(left, right OperationDispatchAttemptRefV3) bool {
	if left.Validate() != nil || right.Validate() != nil {
		return false
	}
	leftDigest, leftErr := core.CanonicalJSONDigest("praxis.runtime.model-provider-actual-point", ModelProviderActualPointGuardContractVersionV1, "OperationDispatchAttemptRefV3", left)
	rightDigest, rightErr := core.CanonicalJSONDigest("praxis.runtime.model-provider-actual-point", ModelProviderActualPointGuardContractVersionV1, "OperationDispatchAttemptRefV3", right)
	return leftErr == nil && rightErr == nil && leftDigest == rightDigest
}

func SameModelProviderBoundaryCurrentRefV1(left, right ModelProviderBoundaryCurrentRefV1) bool {
	return left.Validate() == nil && right.Validate() == nil && left.Digest == right.Digest
}

func (r ModelProviderBoundaryCurrentRefV1) DigestV1() (core.Digest, error) {
	copy := r
	copy.Digest = ""
	return core.CanonicalJSONDigest("praxis.runtime.model-provider-actual-point", ModelProviderActualPointGuardContractVersionV1, "ModelProviderBoundaryCurrentRefV1", copy)
}

func SealModelProviderBoundaryCurrentRefV1(r ModelProviderBoundaryCurrentRefV1) (ModelProviderBoundaryCurrentRefV1, error) {
	r.ContractVersion = ModelProviderActualPointGuardContractVersionV1
	r.Digest = ""
	digest, err := r.DigestV1()
	if err != nil {
		return ModelProviderBoundaryCurrentRefV1{}, err
	}
	r.Digest = digest
	return r, r.Validate()
}

type ModelProviderBoundaryCurrentProjectionV1 struct {
	ContractVersion  string                            `json:"contract_version"`
	Ref              ModelProviderBoundaryCurrentRefV1 `json:"ref"`
	State            string                            `json:"state"`
	Provider         ProviderBindingRefV2              `json:"provider"`
	CheckedUnixNano  int64                             `json:"checked_unix_nano"`
	ExpiresUnixNano  int64                             `json:"expires_unix_nano"`
	ProjectionDigest core.Digest                       `json:"projection_digest"`
}

func (p ModelProviderBoundaryCurrentProjectionV1) Validate() error {
	if p.ContractVersion != ModelProviderActualPointGuardContractVersionV1 || p.State != ModelProviderBoundaryCrossedV1 || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano || p.ExpiresUnixNano > p.Ref.ExpiresUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidReference, "Model provider boundary projection is not current")
	}
	if err := p.Ref.Validate(); err != nil {
		return err
	}
	if err := p.Provider.Validate(); err != nil {
		return err
	}
	expected, err := p.DigestV1()
	if err != nil || expected != p.ProjectionDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Model boundary projection digest drifted")
	}
	return nil
}

func (p ModelProviderBoundaryCurrentProjectionV1) DigestV1() (core.Digest, error) {
	copy := p
	copy.ProjectionDigest = ""
	return core.CanonicalJSONDigest("praxis.runtime.model-provider-actual-point", ModelProviderActualPointGuardContractVersionV1, "ModelProviderBoundaryCurrentProjectionV1", copy)
}

func SealModelProviderBoundaryCurrentProjectionV1(p ModelProviderBoundaryCurrentProjectionV1) (ModelProviderBoundaryCurrentProjectionV1, error) {
	p.ContractVersion = ModelProviderActualPointGuardContractVersionV1
	p.ProjectionDigest = ""
	digest, err := p.DigestV1()
	if err != nil {
		return ModelProviderBoundaryCurrentProjectionV1{}, err
	}
	p.ProjectionDigest = digest
	return p, p.Validate()
}

type ModelProviderBoundaryCurrentReaderV1 interface {
	InspectCurrentModelProviderBoundaryV1(context.Context, ModelProviderBoundaryCurrentRefV1) (ModelProviderBoundaryCurrentProjectionV1, error)
}

type InspectCurrentModelProviderActualPointRequestV1 struct {
	Operation                  OperationSubjectV3                `json:"operation"`
	EffectID                   core.EffectIntentID               `json:"effect_id"`
	ExpectedEffectRevision     core.Revision                     `json:"expected_effect_revision"`
	PermitID                   string                            `json:"permit_id"`
	ExpectedPermitFactRevision core.Revision                     `json:"expected_permit_fact_revision"`
	PermitDigest               core.Digest                       `json:"permit_digest"`
	AdmissionDigest            core.Digest                       `json:"admission_digest"`
	ReviewAuthorization        OperationReviewAuthorizationRefV4 `json:"review_authorization"`
	Attempt                    OperationDispatchAttemptRefV3     `json:"attempt"`
	Verifier                   ProviderBindingRefV2              `json:"verifier"`
	FenceDigest                core.Digest                       `json:"fence_digest"`
	ModelBoundary              ModelProviderBoundaryCurrentRefV1 `json:"model_boundary"`
	RequestedNotAfterUnixNano  int64                             `json:"requested_not_after_unix_nano"`
}

func (r InspectCurrentModelProviderActualPointRequestV1) Validate() error {
	if err := r.Operation.Validate(); err != nil {
		return err
	}
	if r.Operation.Kind != OperationScopeRunV3 || r.Operation.RunID == "" || r.EffectID == "" || r.ExpectedEffectRevision == 0 || r.PermitID == "" || r.ExpectedPermitFactRevision == 0 || r.RequestedNotAfterUnixNano <= 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonDispatchPermitInvalid, "Model actual-point request identity is incomplete")
	}
	for _, digest := range []core.Digest{r.PermitDigest, r.AdmissionDigest, r.FenceDigest} {
		if err := digest.Validate(); err != nil {
			return err
		}
	}
	if err := r.ReviewAuthorization.Validate(); err != nil {
		return err
	}
	if err := r.Attempt.Validate(); err != nil {
		return err
	}
	if err := r.Verifier.Validate(); err != nil {
		return err
	}
	if err := r.ModelBoundary.Validate(); err != nil {
		return err
	}
	operationDigest, err := r.Operation.DigestV3()
	if err != nil || r.Attempt.OperationDigest != operationDigest || r.ModelBoundary.OperationDigest != operationDigest || r.Attempt.EffectID != r.EffectID || r.ModelBoundary.EffectID != r.EffectID || !SameModelProviderOperationDispatchAttemptV1(r.ModelBoundary.RuntimeAttempt, r.Attempt) || r.Attempt.PermitID != r.PermitID {
		return core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "Model actual-point request cross-binding drifted")
	}
	return nil
}

func (r InspectCurrentModelProviderActualPointRequestV1) DigestV1() (core.Digest, error) {
	if err := r.Validate(); err != nil {
		return "", err
	}
	return core.CanonicalJSONDigest("praxis.runtime.model-provider-actual-point", ModelProviderActualPointGuardContractVersionV1, "InspectCurrentModelProviderActualPointRequestV1", r)
}

type ModelProviderActualPointCurrentProjectionV1 struct {
	ContractVersion      string                            `json:"contract_version"`
	RequestDigest        core.Digest                       `json:"request_digest"`
	OperationDigest      core.Digest                       `json:"operation_digest"`
	EffectID             core.EffectIntentID               `json:"effect_id"`
	EffectFactRevision   core.Revision                     `json:"effect_fact_revision"`
	PermitID             string                            `json:"permit_id"`
	PermitFactRevision   core.Revision                     `json:"permit_fact_revision"`
	PermitDigest         core.Digest                       `json:"permit_digest"`
	AdmissionDigest      core.Digest                       `json:"admission_digest"`
	ReviewAuthorization  OperationReviewAuthorizationRefV4 `json:"review_authorization"`
	Attempt              OperationDispatchAttemptRefV3     `json:"attempt"`
	FenceDigest          core.Digest                       `json:"fence_digest"`
	RuntimeControlDigest core.Digest                       `json:"runtime_control_digest"`
	ModelBoundary        ModelProviderBoundaryCurrentRefV1 `json:"model_boundary"`
	Provider             ProviderBindingRefV2              `json:"provider"`
	Verifier             ProviderBindingRefV2              `json:"verifier"`
	CheckedUnixNano      int64                             `json:"checked_unix_nano"`
	NotAfterUnixNano     int64                             `json:"not_after_unix_nano"`
	ProjectionDigest     core.Digest                       `json:"projection_digest"`
}

func (p ModelProviderActualPointCurrentProjectionV1) Validate() error {
	if p.ContractVersion != ModelProviderActualPointGuardContractVersionV1 || p.EffectID == "" || p.EffectFactRevision == 0 || p.PermitID == "" || p.PermitFactRevision == 0 || p.CheckedUnixNano <= 0 || p.NotAfterUnixNano <= p.CheckedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonDispatchPermitInvalid, "Model actual-point projection identity or TTL is invalid")
	}
	for _, digest := range []core.Digest{p.RequestDigest, p.OperationDigest, p.PermitDigest, p.AdmissionDigest, p.FenceDigest, p.RuntimeControlDigest, p.ProjectionDigest} {
		if err := digest.Validate(); err != nil {
			return err
		}
	}
	if err := p.ReviewAuthorization.Validate(); err != nil {
		return err
	}
	if err := p.Attempt.Validate(); err != nil {
		return err
	}
	if err := p.ModelBoundary.Validate(); err != nil {
		return err
	}
	if err := p.Provider.Validate(); err != nil {
		return err
	}
	if err := p.Verifier.Validate(); err != nil {
		return err
	}
	if p.Attempt.OperationDigest != p.OperationDigest || p.Attempt.EffectID != p.EffectID || p.Attempt.PermitID != p.PermitID || !SameModelProviderOperationDispatchAttemptV1(p.ModelBoundary.RuntimeAttempt, p.Attempt) || p.NotAfterUnixNano > p.ModelBoundary.ExpiresUnixNano {
		return core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "Model actual-point projection cross-binding drifted")
	}
	expected, err := p.DigestV1()
	if err != nil || expected != p.ProjectionDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Model actual-point projection digest drifted")
	}
	return nil
}

// ValidateAgainst proves that a sealed projection is the exact result of one
// public request at the consumer's current time. It does not authorize a
// Provider call after NotAfter.
func (p ModelProviderActualPointCurrentProjectionV1) ValidateAgainst(request InspectCurrentModelProviderActualPointRequestV1, now time.Time) error {
	if err := request.Validate(); err != nil {
		return err
	}
	if err := p.ValidateCurrent(now); err != nil {
		return err
	}
	requestDigest, err := request.DigestV1()
	if err != nil {
		return err
	}
	operationDigest, err := request.Operation.DigestV3()
	if err != nil {
		return err
	}
	if p.RequestDigest != requestDigest ||
		p.OperationDigest != operationDigest ||
		p.EffectID != request.EffectID ||
		p.EffectFactRevision != request.ExpectedEffectRevision ||
		p.PermitID != request.PermitID ||
		p.PermitFactRevision != request.ExpectedPermitFactRevision ||
		p.PermitDigest != request.PermitDigest ||
		p.AdmissionDigest != request.AdmissionDigest ||
		p.ReviewAuthorization != request.ReviewAuthorization ||
		!SameModelProviderOperationDispatchAttemptV1(p.Attempt, request.Attempt) ||
		p.Verifier != request.Verifier ||
		p.FenceDigest != request.FenceDigest ||
		!SameModelProviderBoundaryCurrentRefV1(p.ModelBoundary, request.ModelBoundary) ||
		p.NotAfterUnixNano > request.RequestedNotAfterUnixNano {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Model actual-point projection belongs to another request")
	}
	return nil
}

func (p ModelProviderActualPointCurrentProjectionV1) DigestV1() (core.Digest, error) {
	copy := p
	copy.ProjectionDigest = ""
	return core.CanonicalJSONDigest("praxis.runtime.model-provider-actual-point", ModelProviderActualPointGuardContractVersionV1, "ModelProviderActualPointCurrentProjectionV1", copy)
}

func SealModelProviderActualPointCurrentProjectionV1(p ModelProviderActualPointCurrentProjectionV1) (ModelProviderActualPointCurrentProjectionV1, error) {
	p.ContractVersion = ModelProviderActualPointGuardContractVersionV1
	p.ProjectionDigest = ""
	digest, err := p.DigestV1()
	if err != nil {
		return ModelProviderActualPointCurrentProjectionV1{}, err
	}
	p.ProjectionDigest = digest
	return p, p.Validate()
}

type ModelProviderActualPointGuardV1 interface {
	InspectCurrentModelProviderActualPointV1(context.Context, InspectCurrentModelProviderActualPointRequestV1) (ModelProviderActualPointCurrentProjectionV1, error)
}

func ModelProviderActualPointNotAfterV1(values ...int64) int64 {
	var result int64
	for _, value := range values {
		if value > 0 && (result == 0 || value < result) {
			result = value
		}
	}
	return result
}

func (p ModelProviderActualPointCurrentProjectionV1) ValidateCurrent(now time.Time) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if now.IsZero() || now.UnixNano() < p.CheckedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Model actual-point consumer clock regressed")
	}
	if !now.Before(time.Unix(0, p.NotAfterUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "Model actual-point projection expired")
	}
	return nil
}
