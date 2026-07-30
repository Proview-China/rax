package contract

import (
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const NextModelTurnEligibilityContractVersionV1 = "praxis.application/next-model-turn-eligibility/v1"

// NextModelTurnEligibilityRequestV1 is a read-only, cross-owner inspection
// coordinate. It carries exact references only: no Harness envelope, Provider
// payload, execution authority, or Application-owned durable fact.
type NextModelTurnEligibilityRequestV1 struct {
	ContractVersion                 string                                                       `json:"contract_version"`
	ContinuationAttempt             TurnContinuationAttemptRefV1                                 `json:"continuation_attempt"`
	ContinuationCurrentDigest       core.Digest                                                  `json:"continuation_current_digest"`
	ActiveContext                   HarnessActiveContextRefV1                                    `json:"active_context"`
	Run                             SingleCallRunCoordinateV1                                    `json:"run"`
	Session                         SingleCallSessionCoordinateV1                                `json:"session"`
	TargetTurn                      uint32                                                       `json:"target_turn"`
	RuntimeActualPoint              runtimeports.InspectCurrentModelProviderActualPointRequestV1 `json:"runtime_actual_point"`
	RuntimeActualPointRequestDigest core.Digest                                                  `json:"runtime_actual_point_request_digest"`
	RequestedNotAfterUnixNano       int64                                                        `json:"requested_not_after_unix_nano"`
	Digest                          core.Digest                                                  `json:"digest"`
}

func (r NextModelTurnEligibilityRequestV1) DigestV1() (core.Digest, error) {
	copy := r
	copy.Digest = ""
	return core.CanonicalJSONDigest(
		"praxis.application.next-model-turn-eligibility-request",
		NextModelTurnEligibilityContractVersionV1,
		"NextModelTurnEligibilityRequestV1",
		copy,
	)
}

func (r NextModelTurnEligibilityRequestV1) Validate() error {
	if r.ContractVersion != NextModelTurnEligibilityContractVersionV1 ||
		r.ContinuationAttempt.Validate() != nil ||
		r.ContinuationCurrentDigest.Validate() != nil ||
		r.ActiveContext.Validate() != nil ||
		r.Run.Validate() != nil ||
		r.Session.Validate() != nil ||
		r.TargetTurn == 0 ||
		r.RuntimeActualPoint.Validate() != nil ||
		r.RuntimeActualPointRequestDigest.Validate() != nil ||
		r.RequestedNotAfterUnixNano <= 0 ||
		r.Digest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "next Model Turn eligibility request is incomplete")
	}
	actualPointDigest, err := r.RuntimeActualPoint.DigestV1()
	if err != nil || actualPointDigest != r.RuntimeActualPointRequestDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "next Model Turn actual-point request digest drifted")
	}
	operation := r.RuntimeActualPoint.Operation
	if r.ContinuationAttempt.ExecutionScopeDigest != r.ActiveContext.ExecutionScopeDigest ||
		r.ContinuationAttempt.ExecutionScopeDigest != operation.ExecutionScopeDigest ||
		r.ContinuationAttempt.RunID != r.Run.RunID ||
		r.ContinuationAttempt.RunID != r.ActiveContext.RunID ||
		r.ContinuationAttempt.RunID != operation.RunID ||
		r.ContinuationAttempt.SessionID != r.Session.ID ||
		r.ContinuationAttempt.SessionID != r.ActiveContext.SessionID ||
		r.ContinuationAttempt.TargetTurn != r.TargetTurn ||
		r.ActiveContext.TurnOrdinal != r.TargetTurn ||
		r.Run.Revision != operation.CurrentProjectionRevision ||
		r.Run.Digest != operation.CurrentProjectionDigest ||
		r.RuntimeActualPoint.RequestedNotAfterUnixNano != r.RequestedNotAfterUnixNano {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "next Model Turn exact coordinates belong to different subjects")
	}
	digest, err := r.DigestV1()
	if err != nil || digest != r.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "next Model Turn eligibility request digest drifted")
	}
	return nil
}

func (r NextModelTurnEligibilityRequestV1) ValidateCurrent(now time.Time) error {
	if err := r.Validate(); err != nil {
		return err
	}
	if now.IsZero() || now.UnixNano() < r.Session.CheckedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "next Model Turn eligibility clock regressed")
	}
	if !now.Before(time.Unix(0, r.RequestedNotAfterUnixNano)) ||
		!now.Before(time.Unix(0, r.Session.ExpiresUnixNano)) ||
		!now.Before(time.Unix(0, r.RuntimeActualPoint.ModelBoundary.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "next Model Turn eligibility request expired")
	}
	return nil
}

func SealNextModelTurnEligibilityRequestV1(r NextModelTurnEligibilityRequestV1) (NextModelTurnEligibilityRequestV1, error) {
	r.ContractVersion = NextModelTurnEligibilityContractVersionV1
	actualPointDigest, err := r.RuntimeActualPoint.DigestV1()
	if err != nil {
		return NextModelTurnEligibilityRequestV1{}, err
	}
	if r.RuntimeActualPointRequestDigest != "" && r.RuntimeActualPointRequestDigest != actualPointDigest {
		return NextModelTurnEligibilityRequestV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "next Model Turn supplied another actual-point request digest")
	}
	r.RuntimeActualPointRequestDigest = actualPointDigest
	provided := r.Digest
	r.Digest = ""
	digest, err := r.DigestV1()
	if err != nil {
		return NextModelTurnEligibilityRequestV1{}, err
	}
	if provided != "" && provided != digest {
		return NextModelTurnEligibilityRequestV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "next Model Turn supplied another request digest")
	}
	r.Digest = digest
	return r, r.Validate()
}

// NextModelTurnDerivedDispatchRefV1 is a deterministic recovery coordinate,
// not a dispatch permit or an Application-owned fact. Re-inspecting the same
// exact request always derives the same ref.
type NextModelTurnDerivedDispatchRefV1 struct {
	ContractVersion                 string                       `json:"contract_version"`
	ID                              string                       `json:"id"`
	Revision                        core.Revision                `json:"revision"`
	RequestDigest                   core.Digest                  `json:"request_digest"`
	ContinuationAttempt             TurnContinuationAttemptRefV1 `json:"continuation_attempt"`
	ContinuationCurrentDigest       core.Digest                  `json:"continuation_current_digest"`
	ActiveContextDigest             core.Digest                  `json:"active_context_digest"`
	RuntimeActualPointRequestDigest core.Digest                  `json:"runtime_actual_point_request_digest"`
	Digest                          core.Digest                  `json:"digest"`
}

func DeriveNextModelTurnDispatchIDV1(request NextModelTurnEligibilityRequestV1) (string, error) {
	if err := request.Validate(); err != nil {
		return "", err
	}
	return DeriveNextModelTurnDispatchIDFromRequestDigestV1(request.Digest)
}

func DeriveNextModelTurnDispatchIDFromRequestDigestV1(
	requestDigest core.Digest,
) (string, error) {
	if requestDigest.Validate() != nil {
		return "", core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidDigest, "next Model Turn derived dispatch request digest is invalid")
	}
	digest, err := core.CanonicalJSONDigest(
		"praxis.application.next-model-turn-derived-dispatch-id",
		NextModelTurnEligibilityContractVersionV1,
		"NextModelTurnDerivedDispatchIdentityV1",
		struct {
			RequestDigest core.Digest `json:"request_digest"`
		}{RequestDigest: requestDigest},
	)
	if err != nil {
		return "", err
	}
	return "next-model-turn-dispatch:v1:" + strings.TrimPrefix(string(digest), "sha256:"), nil
}

func (r NextModelTurnDerivedDispatchRefV1) Validate() error {
	if r.ContractVersion != NextModelTurnEligibilityContractVersionV1 ||
		!validSingleCallIDV1(r.ID) ||
		r.Revision != 1 ||
		r.RequestDigest.Validate() != nil ||
		r.ContinuationAttempt.Validate() != nil ||
		r.ContinuationCurrentDigest.Validate() != nil ||
		r.ActiveContextDigest.Validate() != nil ||
		r.RuntimeActualPointRequestDigest.Validate() != nil ||
		r.Digest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "next Model Turn derived dispatch ref is incomplete")
	}
	expectedID, err := DeriveNextModelTurnDispatchIDFromRequestDigestV1(r.RequestDigest)
	if err != nil || expectedID != r.ID {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidReference, "next Model Turn derived dispatch ID drifted")
	}
	digest, err := r.DigestV1()
	if err != nil || digest != r.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "next Model Turn derived dispatch ref digest drifted")
	}
	return nil
}

func (r NextModelTurnDerivedDispatchRefV1) DigestV1() (core.Digest, error) {
	copy := r
	copy.Digest = ""
	return core.CanonicalJSONDigest(
		"praxis.application.next-model-turn-derived-dispatch-ref",
		NextModelTurnEligibilityContractVersionV1,
		"NextModelTurnDerivedDispatchRefV1",
		copy,
	)
}

func (r NextModelTurnDerivedDispatchRefV1) ValidateFor(request NextModelTurnEligibilityRequestV1) error {
	if request.Validate() != nil ||
		r.Validate() != nil ||
		r.RequestDigest != request.Digest ||
		r.ContinuationAttempt != request.ContinuationAttempt ||
		r.ContinuationCurrentDigest != request.ContinuationCurrentDigest ||
		r.ActiveContextDigest != request.ActiveContext.Digest ||
		r.RuntimeActualPointRequestDigest != request.RuntimeActualPointRequestDigest {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "next Model Turn derived dispatch ref is incomplete")
	}
	return nil
}

func DeriveNextModelTurnDispatchRefV1(request NextModelTurnEligibilityRequestV1) (NextModelTurnDerivedDispatchRefV1, error) {
	id, err := DeriveNextModelTurnDispatchIDV1(request)
	if err != nil {
		return NextModelTurnDerivedDispatchRefV1{}, err
	}
	ref := NextModelTurnDerivedDispatchRefV1{
		ContractVersion:                 NextModelTurnEligibilityContractVersionV1,
		ID:                              id,
		Revision:                        1,
		RequestDigest:                   request.Digest,
		ContinuationAttempt:             request.ContinuationAttempt,
		ContinuationCurrentDigest:       request.ContinuationCurrentDigest,
		ActiveContextDigest:             request.ActiveContext.Digest,
		RuntimeActualPointRequestDigest: request.RuntimeActualPointRequestDigest,
	}
	ref.Digest, err = ref.DigestV1()
	if err != nil {
		return NextModelTurnDerivedDispatchRefV1{}, err
	}
	return ref, ref.ValidateFor(request)
}

// NextModelTurnEligibilityProjectionV1 is a short-lived read result. It is
// never persisted by Application and deliberately exposes no Allowed flag.
type NextModelTurnEligibilityProjectionV1 struct {
	ContractVersion                 string                            `json:"contract_version"`
	DerivedDispatch                 NextModelTurnDerivedDispatchRefV1 `json:"derived_dispatch"`
	RequestDigest                   core.Digest                       `json:"request_digest"`
	ContinuationCurrentDigest       core.Digest                       `json:"continuation_current_digest"`
	ActiveContextDigest             core.Digest                       `json:"active_context_digest"`
	RuntimeActualPointRequestDigest core.Digest                       `json:"runtime_actual_point_request_digest"`
	CheckedUnixNano                 int64                             `json:"checked_unix_nano"`
	NotAfterUnixNano                int64                             `json:"not_after_unix_nano"`
	Digest                          core.Digest                       `json:"digest"`
}

func (p NextModelTurnEligibilityProjectionV1) DigestV1() (core.Digest, error) {
	copy := p
	copy.Digest = ""
	return core.CanonicalJSONDigest(
		"praxis.application.next-model-turn-eligibility-projection",
		NextModelTurnEligibilityContractVersionV1,
		"NextModelTurnEligibilityProjectionV1",
		copy,
	)
}

func (p NextModelTurnEligibilityProjectionV1) ValidateFor(request NextModelTurnEligibilityRequestV1, now time.Time) error {
	if err := request.Validate(); err != nil {
		return err
	}
	if p.ContractVersion != NextModelTurnEligibilityContractVersionV1 ||
		p.DerivedDispatch.ValidateFor(request) != nil ||
		p.RequestDigest != request.Digest ||
		p.ContinuationCurrentDigest != request.ContinuationCurrentDigest ||
		p.ActiveContextDigest != request.ActiveContext.Digest ||
		p.RuntimeActualPointRequestDigest != request.RuntimeActualPointRequestDigest ||
		p.CheckedUnixNano <= 0 ||
		p.NotAfterUnixNano <= p.CheckedUnixNano ||
		p.NotAfterUnixNano > request.RequestedNotAfterUnixNano ||
		p.NotAfterUnixNano > request.RuntimeActualPoint.ModelBoundary.ExpiresUnixNano ||
		p.Digest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "next Model Turn eligibility projection is incomplete")
	}
	if now.IsZero() || now.UnixNano() < p.CheckedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "next Model Turn eligibility projection clock regressed")
	}
	if !now.Before(time.Unix(0, p.NotAfterUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "next Model Turn eligibility projection expired")
	}
	digest, err := p.DigestV1()
	if err != nil || digest != p.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "next Model Turn eligibility projection digest drifted")
	}
	return nil
}

func SealNextModelTurnEligibilityProjectionV1(
	request NextModelTurnEligibilityRequestV1,
	continuationNotAfterUnixNano int64,
	now time.Time,
) (NextModelTurnEligibilityProjectionV1, error) {
	if err := request.ValidateCurrent(now); err != nil {
		return NextModelTurnEligibilityProjectionV1{}, err
	}
	if continuationNotAfterUnixNano <= 0 {
		return NextModelTurnEligibilityProjectionV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "next Model Turn continuation expiry is missing")
	}
	notAfter := nextModelTurnNotAfterV1(
		request.RequestedNotAfterUnixNano,
		request.Session.ExpiresUnixNano,
		continuationNotAfterUnixNano,
		request.RuntimeActualPoint.ModelBoundary.ExpiresUnixNano,
	)
	if notAfter <= now.UnixNano() {
		return NextModelTurnEligibilityProjectionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "next Model Turn eligibility TTL crossed before seal")
	}
	derived, err := DeriveNextModelTurnDispatchRefV1(request)
	if err != nil {
		return NextModelTurnEligibilityProjectionV1{}, err
	}
	projection := NextModelTurnEligibilityProjectionV1{
		ContractVersion:                 NextModelTurnEligibilityContractVersionV1,
		DerivedDispatch:                 derived,
		RequestDigest:                   request.Digest,
		ContinuationCurrentDigest:       request.ContinuationCurrentDigest,
		ActiveContextDigest:             request.ActiveContext.Digest,
		RuntimeActualPointRequestDigest: request.RuntimeActualPointRequestDigest,
		CheckedUnixNano:                 now.UnixNano(),
		NotAfterUnixNano:                notAfter,
	}
	projection.Digest, err = projection.DigestV1()
	if err != nil {
		return NextModelTurnEligibilityProjectionV1{}, err
	}
	return projection, projection.ValidateFor(request, now)
}

func nextModelTurnNotAfterV1(values ...int64) int64 {
	var result int64
	for _, value := range values {
		if value > 0 && (result == 0 || value < result) {
			result = value
		}
	}
	return result
}
