package contract

import (
	"bytes"
	"encoding/json"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const (
	NextModelTurnDispatchContractVersionV1 = "praxis.application/next-model-turn-dispatch/v1"
	NextModelTurnDispatchAttemptBoundV1    = NextModelTurnDispatchStateV1("attempt_bound")
)

type NextModelTurnDispatchStateV1 string

// NextModelTurnDispatchRequestV1 binds only the existing Application
// eligibility coordinate. It contains no Model Prepared/current/material
// value, Context/Tool lineage, Harness envelope, Provider coordinate, or
// execution authority.
type NextModelTurnDispatchRequestV1 struct {
	ContractVersion           string                               `json:"contract_version"`
	EligibilityRequest        NextModelTurnEligibilityRequestV1    `json:"eligibility_request"`
	EligibilityProjection     NextModelTurnEligibilityProjectionV1 `json:"eligibility_projection"`
	RequestedNotAfterUnixNano int64                                `json:"requested_not_after_unix_nano"`
	RequestDigest             core.Digest                          `json:"request_digest"`
}

func (r NextModelTurnDispatchRequestV1) DigestV1() (core.Digest, error) {
	r.RequestDigest = ""
	return core.CanonicalJSONDigest(
		"praxis.application.next-model-turn-dispatch-request",
		NextModelTurnDispatchContractVersionV1,
		"NextModelTurnDispatchRequestV1",
		r,
	)
}

func (r NextModelTurnDispatchRequestV1) Validate() error {
	if r.ContractVersion != NextModelTurnDispatchContractVersionV1 ||
		r.EligibilityRequest.Validate() != nil ||
		r.EligibilityProjection.ValidateFor(
			r.EligibilityRequest,
			time.Unix(0, r.EligibilityProjection.CheckedUnixNano),
		) != nil ||
		r.RequestedNotAfterUnixNano <= r.EligibilityProjection.CheckedUnixNano ||
		r.RequestedNotAfterUnixNano > r.EligibilityProjection.NotAfterUnixNano ||
		r.RequestDigest.Validate() != nil {
		return nextModelTurnDispatchErrorV1(core.ErrorInvalidArgument, core.ReasonInvalidReference, "next Model Turn dispatch request is incomplete")
	}
	digest, err := r.DigestV1()
	if err != nil || digest != r.RequestDigest {
		return nextModelTurnDispatchErrorV1(core.ErrorConflict, core.ReasonInvalidDigest, "next Model Turn dispatch request digest drifted")
	}
	return nil
}

func (r NextModelTurnDispatchRequestV1) ValidateCurrent(now time.Time) error {
	if err := r.Validate(); err != nil {
		return err
	}
	if now.IsZero() || now.UnixNano() < r.EligibilityProjection.CheckedUnixNano {
		return nextModelTurnDispatchErrorV1(core.ErrorPreconditionFailed, core.ReasonClockRegression, "next Model Turn dispatch clock regressed")
	}
	if !now.Before(time.Unix(0, r.RequestedNotAfterUnixNano)) {
		return nextModelTurnDispatchErrorV1(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "next Model Turn dispatch request expired")
	}
	return nil
}

func SealNextModelTurnDispatchRequestV1(r NextModelTurnDispatchRequestV1) (NextModelTurnDispatchRequestV1, error) {
	if r.ContractVersion != "" && r.ContractVersion != NextModelTurnDispatchContractVersionV1 {
		return NextModelTurnDispatchRequestV1{}, nextModelTurnDispatchErrorV1(core.ErrorConflict, core.ReasonInvalidReference, "next Model Turn supplied another dispatch contract version")
	}
	r.ContractVersion = NextModelTurnDispatchContractVersionV1
	provided := r.RequestDigest
	r.RequestDigest = ""
	digest, err := r.DigestV1()
	if err != nil {
		return NextModelTurnDispatchRequestV1{}, err
	}
	if provided != "" && provided != digest {
		return NextModelTurnDispatchRequestV1{}, nextModelTurnDispatchErrorV1(core.ErrorConflict, core.ReasonInvalidDigest, "next Model Turn supplied another dispatch request digest")
	}
	r.RequestDigest = digest
	return r, r.Validate()
}

// NextModelTurnDispatchCurrentV1 is the Harness owner-local sidecar fact. It
// seals only the stable Application ref/digest and its exact lifetime.
type NextModelTurnDispatchCurrentV1 struct {
	ContractVersion  string                            `json:"contract_version"`
	DerivedDispatch  NextModelTurnDerivedDispatchRefV1 `json:"derived_dispatch"`
	Revision         core.Revision                     `json:"revision"`
	State            NextModelTurnDispatchStateV1      `json:"state"`
	RequestDigest    core.Digest                       `json:"request_digest"`
	CheckedUnixNano  int64                             `json:"checked_unix_nano"`
	NotAfterUnixNano int64                             `json:"not_after_unix_nano"`
	Digest           core.Digest                       `json:"digest"`
}

func (c NextModelTurnDispatchCurrentV1) DigestV1() (core.Digest, error) {
	c.Digest = ""
	return core.CanonicalJSONDigest(
		"praxis.application.next-model-turn-dispatch-current",
		NextModelTurnDispatchContractVersionV1,
		"NextModelTurnDispatchCurrentV1",
		c,
	)
}

func (c NextModelTurnDispatchCurrentV1) ValidateFor(request NextModelTurnDispatchRequestV1) error {
	if request.Validate() != nil ||
		c.ContractVersion != NextModelTurnDispatchContractVersionV1 ||
		c.DerivedDispatch != request.EligibilityProjection.DerivedDispatch ||
		c.Revision != 1 || c.State != NextModelTurnDispatchAttemptBoundV1 ||
		c.RequestDigest != request.RequestDigest ||
		c.CheckedUnixNano <= 0 ||
		c.NotAfterUnixNano <= c.CheckedUnixNano ||
		c.NotAfterUnixNano > request.RequestedNotAfterUnixNano ||
		c.Digest.Validate() != nil {
		return nextModelTurnDispatchErrorV1(core.ErrorInvalidArgument, core.ReasonInvalidReference, "next Model Turn durable binding is incomplete")
	}
	digest, err := c.DigestV1()
	if err != nil || digest != c.Digest {
		return nextModelTurnDispatchErrorV1(core.ErrorConflict, core.ReasonInvalidDigest, "next Model Turn durable binding digest drifted")
	}
	return nil
}

func (c NextModelTurnDispatchCurrentV1) ValidateCurrentFor(
	request NextModelTurnDispatchRequestV1,
	now time.Time,
) error {
	if err := c.ValidateFor(request); err != nil {
		return err
	}
	if now.IsZero() || now.UnixNano() < c.CheckedUnixNano {
		return nextModelTurnDispatchErrorV1(core.ErrorPreconditionFailed, core.ReasonClockRegression, "next Model Turn durable binding clock regressed")
	}
	if !now.Before(time.Unix(0, c.NotAfterUnixNano)) {
		return nextModelTurnDispatchErrorV1(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "next Model Turn durable binding expired")
	}
	return nil
}

func SealNextModelTurnDispatchAttemptBoundV1(
	request NextModelTurnDispatchRequestV1,
	checkedUnixNano int64,
	notAfterUnixNano int64,
) (NextModelTurnDispatchCurrentV1, error) {
	if err := request.Validate(); err != nil {
		return NextModelTurnDispatchCurrentV1{}, err
	}
	current := NextModelTurnDispatchCurrentV1{
		ContractVersion:  NextModelTurnDispatchContractVersionV1,
		DerivedDispatch:  request.EligibilityProjection.DerivedDispatch,
		Revision:         1,
		State:            NextModelTurnDispatchAttemptBoundV1,
		RequestDigest:    request.RequestDigest,
		CheckedUnixNano:  checkedUnixNano,
		NotAfterUnixNano: notAfterUnixNano,
	}
	digest, err := current.DigestV1()
	if err != nil {
		return NextModelTurnDispatchCurrentV1{}, err
	}
	current.Digest = digest
	return current, current.ValidateFor(request)
}

type NextModelTurnDispatchInspectRequestV1 struct {
	ContractVersion string                            `json:"contract_version"`
	DerivedDispatch NextModelTurnDerivedDispatchRefV1 `json:"derived_dispatch"`
	RequestDigest   core.Digest                       `json:"request_digest"`
	Digest          core.Digest                       `json:"digest"`
}

func (r NextModelTurnDispatchInspectRequestV1) DigestV1() (core.Digest, error) {
	r.Digest = ""
	return core.CanonicalJSONDigest(
		"praxis.application.next-model-turn-dispatch-inspect",
		NextModelTurnDispatchContractVersionV1,
		"NextModelTurnDispatchInspectRequestV1",
		r,
	)
}

func (r NextModelTurnDispatchInspectRequestV1) Validate() error {
	if r.ContractVersion != NextModelTurnDispatchContractVersionV1 ||
		r.DerivedDispatch.Validate() != nil ||
		r.RequestDigest.Validate() != nil ||
		r.Digest.Validate() != nil {
		return nextModelTurnDispatchErrorV1(core.ErrorInvalidArgument, core.ReasonInvalidReference, "next Model Turn Inspect request is incomplete")
	}
	digest, err := r.DigestV1()
	if err != nil || digest != r.Digest {
		return nextModelTurnDispatchErrorV1(core.ErrorConflict, core.ReasonInvalidDigest, "next Model Turn Inspect request digest drifted")
	}
	return nil
}

func NewNextModelTurnDispatchInspectRequestV1(
	request NextModelTurnDispatchRequestV1,
) (NextModelTurnDispatchInspectRequestV1, error) {
	if err := request.Validate(); err != nil {
		return NextModelTurnDispatchInspectRequestV1{}, err
	}
	inspect := NextModelTurnDispatchInspectRequestV1{
		ContractVersion: NextModelTurnDispatchContractVersionV1,
		DerivedDispatch: request.EligibilityProjection.DerivedDispatch,
		RequestDigest:   request.RequestDigest,
	}
	digest, err := inspect.DigestV1()
	if err != nil {
		return NextModelTurnDispatchInspectRequestV1{}, err
	}
	inspect.Digest = digest
	return inspect, inspect.Validate()
}

func EncodeNextModelTurnDispatchCurrentV1(current NextModelTurnDispatchCurrentV1) ([]byte, error) {
	if current.Digest.Validate() != nil {
		return nil, nextModelTurnDispatchErrorV1(core.ErrorInvalidArgument, core.ReasonInvalidDigest, "next Model Turn durable binding digest is invalid")
	}
	return json.Marshal(current)
}

func DecodeNextModelTurnDispatchCurrentV1(payload []byte) (NextModelTurnDispatchCurrentV1, error) {
	var current NextModelTurnDispatchCurrentV1
	if err := core.DecodeStrictJSON(payload, &current); err != nil {
		return NextModelTurnDispatchCurrentV1{}, nextModelTurnDispatchErrorV1(core.ErrorConflict, core.ReasonInvalidCanonicalForm, "next Model Turn durable binding is not strict JSON")
	}
	canonical, err := json.Marshal(current)
	if err != nil || !bytes.Equal(canonical, payload) {
		return NextModelTurnDispatchCurrentV1{}, nextModelTurnDispatchErrorV1(core.ErrorConflict, core.ReasonInvalidCanonicalForm, "next Model Turn durable binding is not canonical JSON")
	}
	digest, err := current.DigestV1()
	if err != nil || digest != current.Digest {
		return NextModelTurnDispatchCurrentV1{}, nextModelTurnDispatchErrorV1(core.ErrorConflict, core.ReasonInvalidDigest, "next Model Turn durable binding canonical digest drifted")
	}
	return current, nil
}

func NextModelTurnDispatchNotAfterV1(values ...int64) int64 {
	var minimum int64
	for _, value := range values {
		if value > 0 && (minimum == 0 || value < minimum) {
			minimum = value
		}
	}
	return minimum
}

func nextModelTurnDispatchErrorV1(
	category core.ErrorCategory,
	reason core.ReasonCode,
	message string,
) error {
	return core.NewError(category, reason, message)
}
