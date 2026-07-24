package ports

import (
	"context"
	"encoding/json"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const AutoReviewTerminationCurrentContractVersionV1 = "praxis.review.auto-review-termination-current/v1"

// AutoReviewTerminationCurrentRequestV1 binds one current Review loop cut.
// Target.ID is the stable loop family; Target revision/digest and Case remain
// exact so a new Candidate revision cannot borrow an old current proof.
type AutoReviewTerminationCurrentRequestV1 struct {
	TenantID        core.TenantID               `json:"tenant_id"`
	Target          contract.ExactResourceRefV1 `json:"target"`
	Case            contract.ExactResourceRefV1 `json:"case"`
	Rubric          contract.ExactResourceRefV1 `json:"rubric"`
	ExpectedRound   contract.ExactResourceRefV1 `json:"expected_round"`
	CheckedUnixNano int64                       `json:"checked_unix_nano"`
}

func (r AutoReviewTerminationCurrentRequestV1) Validate() error {
	if r.TenantID == "" || r.CheckedUnixNano <= 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "auto review termination current request is incomplete")
	}
	for _, ref := range []contract.ExactResourceRefV1{r.Target, r.Case, r.Rubric, r.ExpectedRound} {
		if err := ref.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// AutoReviewTerminationCurrentProjectionV1 is a Review-owned linearizable
// history cut. Counts cover every Auto attempt for the stable Target.ID across
// Candidate revisions and Cases. RepeatedFindingCount is the maximum number of
// rounds carrying the same Category+Anchor+Claim signature; duplicate drafts
// inside one round count once. Historical expiry never erases loop history.
type AutoReviewTerminationCurrentProjectionV1 struct {
	ContractVersion        string                      `json:"contract_version"`
	TenantID               core.TenantID               `json:"tenant_id"`
	Target                 contract.ExactResourceRefV1 `json:"target"`
	Case                   contract.ExactResourceRefV1 `json:"case"`
	Rubric                 contract.ExactResourceRefV1 `json:"rubric"`
	ExpectedRound          contract.ExactResourceRefV1 `json:"expected_round"`
	RoundCount             uint32                      `json:"round_count"`
	HighestRoundOrdinal    uint32                      `json:"highest_round_ordinal"`
	RepeatedFindingCount   uint32                      `json:"repeated_finding_count"`
	RepeatedRejectionCount uint32                      `json:"repeated_rejection_count"`
	CheckedUnixNano        int64                       `json:"checked_unix_nano"`
	ExpiresUnixNano        int64                       `json:"expires_unix_nano"`
	ClosureDigest          core.Digest                 `json:"closure_digest"`
	ProjectionDigest       core.Digest                 `json:"projection_digest"`
}

func (p AutoReviewTerminationCurrentProjectionV1) closureValue() any {
	return struct {
		ContractVersion        string                      `json:"contract_version"`
		TenantID               core.TenantID               `json:"tenant_id"`
		Target                 contract.ExactResourceRefV1 `json:"target"`
		Case                   contract.ExactResourceRefV1 `json:"case"`
		Rubric                 contract.ExactResourceRefV1 `json:"rubric"`
		ExpectedRound          contract.ExactResourceRefV1 `json:"expected_round"`
		RoundCount             uint32                      `json:"round_count"`
		HighestRoundOrdinal    uint32                      `json:"highest_round_ordinal"`
		RepeatedFindingCount   uint32                      `json:"repeated_finding_count"`
		RepeatedRejectionCount uint32                      `json:"repeated_rejection_count"`
		ExpiresUnixNano        int64                       `json:"expires_unix_nano"`
	}{p.ContractVersion, p.TenantID, p.Target, p.Case, p.Rubric, p.ExpectedRound, p.RoundCount, p.HighestRoundOrdinal, p.RepeatedFindingCount, p.RepeatedRejectionCount, p.ExpiresUnixNano}
}

func (p AutoReviewTerminationCurrentProjectionV1) DigestClosureV1() (core.Digest, error) {
	return core.CanonicalJSONDigest("praxis.review.auto-review-termination-current", AutoReviewTerminationCurrentContractVersionV1, "AutoReviewTerminationCurrentClosureV1", p.closureValue())
}

func (p AutoReviewTerminationCurrentProjectionV1) DigestProjectionV1() (core.Digest, error) {
	p.ProjectionDigest = ""
	return core.CanonicalJSONDigest("praxis.review.auto-review-termination-current", AutoReviewTerminationCurrentContractVersionV1, "AutoReviewTerminationCurrentProjectionV1", p)
}

func SealAutoReviewTerminationCurrentProjectionV1(p AutoReviewTerminationCurrentProjectionV1) (AutoReviewTerminationCurrentProjectionV1, error) {
	p.ContractVersion = AutoReviewTerminationCurrentContractVersionV1
	p.ClosureDigest = ""
	p.ProjectionDigest = ""
	closure, err := p.DigestClosureV1()
	if err != nil {
		return AutoReviewTerminationCurrentProjectionV1{}, err
	}
	p.ClosureDigest = closure
	digest, err := p.DigestProjectionV1()
	if err != nil {
		return AutoReviewTerminationCurrentProjectionV1{}, err
	}
	p.ProjectionDigest = digest
	return p, p.Validate()
}

func (p AutoReviewTerminationCurrentProjectionV1) Validate() error {
	request := AutoReviewTerminationCurrentRequestV1{TenantID: p.TenantID, Target: p.Target, Case: p.Case, Rubric: p.Rubric, ExpectedRound: p.ExpectedRound, CheckedUnixNano: p.CheckedUnixNano}
	if p.ContractVersion != AutoReviewTerminationCurrentContractVersionV1 || request.Validate() != nil || p.RoundCount == 0 || p.HighestRoundOrdinal != p.RoundCount || p.RepeatedFindingCount > p.RoundCount || p.RepeatedRejectionCount > p.RoundCount || p.CheckedUnixNano >= p.ExpiresUnixNano || p.ClosureDigest.Validate() != nil || p.ProjectionDigest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "auto review termination current projection is incomplete")
	}
	closure, err := p.DigestClosureV1()
	if err != nil || closure != p.ClosureDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "auto review termination closure digest drifted")
	}
	digest, err := p.DigestProjectionV1()
	if err != nil || digest != p.ProjectionDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "auto review termination projection digest drifted")
	}
	return nil
}

func (p AutoReviewTerminationCurrentProjectionV1) ValidateCurrent(request AutoReviewTerminationCurrentRequestV1, now time.Time) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if p.TenantID != request.TenantID || p.Target != request.Target || p.Case != request.Case || p.Rubric != request.Rubric || p.ExpectedRound != request.ExpectedRound || p.CheckedUnixNano != request.CheckedUnixNano {
		return core.NewError(core.ErrorConflict, core.ReasonReviewVerdictStale, "auto review termination current subject drifted")
	}
	if now.IsZero() || now.UnixNano() < p.CheckedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "auto review termination current clock regressed")
	}
	if now.UnixNano() >= p.ExpiresUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "auto review termination current projection expired")
	}
	return nil
}

// AutoReviewerInvocationCommandV1 is a sealed Review command handed to the
// host-controlled operation gateway. The host owns dispatch/permit/begin and
// returns only an Observation. Review never receives Provider credentials.
type AutoReviewerInvocationCommandV1 struct {
	Attempt contract.AutoReviewerAttemptV1 `json:"attempt"`
}

// AutoReviewerInvocationResultV1 is an untrusted host/provider result
// envelope. RawOutput deliberately remains raw: only the Review Owner may
// validate it against the exact Review-owned schema and seal Review facts.
// Model-side schema_valid flags, typed decoding and Provider status are not
// accepted as proof.
type AutoReviewerInvocationResultV1 struct {
	ObservationID       string                                       `json:"observation_id"`
	Attempt             contract.ExactResourceRefV1                  `json:"attempt"`
	OperationDigest     core.Digest                                  `json:"operation_digest"`
	RuntimeAttempt      runtimeports.OperationDispatchAttemptRefV3   `json:"runtime_attempt"`
	ProviderObservation runtimeports.ProviderAttemptObservationRefV2 `json:"provider_observation"`
	ResultSchema        runtimeports.SchemaRefV2                     `json:"result_schema"`
	RawOutput           json.RawMessage                              `json:"raw_output"`
	Tokens              uint64                                       `json:"tokens"`
	CostMicros          uint64                                       `json:"cost_micros"`
	ObservedUnixNano    int64                                        `json:"observed_unix_nano"`
	ExpiresUnixNano     int64                                        `json:"expires_unix_nano"`
}

func (r AutoReviewerInvocationResultV1) Clone() AutoReviewerInvocationResultV1 {
	r.RawOutput = append(json.RawMessage(nil), r.RawOutput...)
	return r
}

type AutoReviewerInvocationPortV1 interface {
	StartOrInspectAutoReviewerInvocationV1(context.Context, AutoReviewerInvocationCommandV1) (AutoReviewerInvocationResultV1, error)
	InspectAutoReviewerInvocationV1(context.Context, contract.ExactResourceRefV1) (AutoReviewerInvocationResultV1, error)
}

type BeginAutoReviewerAttemptMutationV1 struct {
	Attempt contract.AutoReviewerAttemptV1 `json:"attempt"`
}

type MarkAutoReviewerWaitingInspectMutationV1 struct {
	Expected contract.ExactResourceRefV1    `json:"expected"`
	Next     contract.AutoReviewerAttemptV1 `json:"next"`
}

// AutoReviewerInvocationStartClaimReceiptV1 grants the external Start right
// only to the caller that linearized prepared -> waiting_inspect. Replays and
// lost-reply recovery return Applied=false and may only Inspect the original
// invocation coordinate.
type AutoReviewerInvocationStartClaimReceiptV1 struct {
	Attempt contract.AutoReviewerAttemptV1 `json:"attempt"`
	Applied bool                           `json:"applied"`
}

type RecordAutoReviewerObservationMutationV1 struct {
	Expected     contract.ExactResourceRefV1                  `json:"expected"`
	Next         contract.AutoReviewerAttemptV1               `json:"next"`
	Observation  contract.AutoReviewerInvocationObservationV1 `json:"observation"`
	DomainResult contract.ReviewerInvocationResultFactV1      `json:"domain_result"`
}

type TerminateAutoReviewerAttemptMutationV1 struct {
	Expected contract.ExactResourceRefV1    `json:"expected"`
	Next     contract.AutoReviewerAttemptV1 `json:"next"`
}

// AutoReviewerStoreV1 is Review's append-only attempt boundary. Observation
// and DomainResult publication are one atomic mutation; no Runtime fact is
// created here.
type AutoReviewerStoreV1 interface {
	BeginAutoReviewerAttemptV1(context.Context, BeginAutoReviewerAttemptMutationV1) (contract.AutoReviewerAttemptV1, error)
	MarkAutoReviewerWaitingInspectV1(context.Context, MarkAutoReviewerWaitingInspectMutationV1) (AutoReviewerInvocationStartClaimReceiptV1, error)
	RecordAutoReviewerObservationV1(context.Context, RecordAutoReviewerObservationMutationV1) (contract.AutoReviewerAttemptV1, contract.ReviewerInvocationResultFactV1, error)
	TerminateAutoReviewerAttemptV1(context.Context, TerminateAutoReviewerAttemptMutationV1) (contract.AutoReviewerAttemptV1, error)
	InspectAutoReviewerAttemptExactV1(context.Context, core.TenantID, contract.ExactResourceRefV1) (contract.AutoReviewerAttemptV1, error)
	InspectAutoReviewerAttemptCurrentV1(context.Context, core.TenantID, string) (contract.AutoReviewerAttemptV1, error)
	InspectAutoReviewerAttemptByIdempotencyV1(context.Context, core.TenantID, string) (contract.AutoReviewerAttemptV1, error)
	InspectAutoReviewerObservationExactV1(context.Context, core.TenantID, contract.AutoReviewerInvocationObservationRefV1) (contract.AutoReviewerInvocationObservationV1, error)
	InspectAutoReviewTerminationCurrentV1(context.Context, AutoReviewTerminationCurrentRequestV1) (AutoReviewTerminationCurrentProjectionV1, error)
}
