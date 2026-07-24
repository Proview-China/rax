package contract

import (
	"reflect"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const (
	ReviewGateContractVersionV1 = "praxis.harness.review-gate/v1"
	ReviewGatePhaseIDV1         = "action.review"
	reviewGateCanonicalDomainV1 = "praxis.harness.review-gate"
)

// ReviewGatePhaseDecisionV1 is a Harness flow decision. It is never a Review
// Verdict or Runtime Authorization.
type ReviewGatePhaseDecisionV1 string

const (
	ReviewGateAllowV1 ReviewGatePhaseDecisionV1 = "allow"
	ReviewGateDenyV1  ReviewGatePhaseDecisionV1 = "deny"
	ReviewGateAskV1   ReviewGatePhaseDecisionV1 = "ask"
	ReviewGateDeferV1 ReviewGatePhaseDecisionV1 = "defer"
)

type ReviewGateRequestV1 struct {
	ContractVersion           string                                           `json:"contract_version"`
	Action                    CommittedPendingActionCurrentRequestV3           `json:"action"`
	Target                    runtimeports.OperationReviewTargetRefV4          `json:"target"`
	Intent                    runtimeports.OperationEffectIntentV3             `json:"intent"`
	Authorization             *runtimeports.OperationReviewAuthorizationRefV5  `json:"authorization,omitempty"`
	Basis                     runtimeports.OperationReviewAuthorizationBasisV5 `json:"basis"`
	RequestedNotAfterUnixNano int64                                            `json:"requested_not_after_unix_nano"`
}

func (r ReviewGateRequestV1) Validate(now time.Time) error {
	if r.ContractVersion != ReviewGateContractVersionV1 || now.IsZero() || r.RequestedNotAfterUnixNano <= now.UnixNano() {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Review Gate request identity or TTL is incomplete")
	}
	if r.Action.RequestedNotAfterUnixNano != r.RequestedNotAfterUnixNano {
		return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "Review Gate Action and request TTL differ")
	}
	if err := r.Action.Validate(now); err != nil {
		return err
	}
	if err := r.Target.Validate(); err != nil {
		return err
	}
	if err := r.Intent.Validate(); err != nil {
		return err
	}
	if r.Authorization != nil {
		if err := r.Authorization.Validate(); err != nil {
			return err
		}
	}
	switch r.Basis {
	case runtimeports.OperationReviewBasisAcceptedQuorumV5, runtimeports.OperationReviewBasisConditionalQuorumSatisfiedV5, runtimeports.OperationReviewBasisPolicyNotRequiredV5:
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonReviewVerdictMissing, "Review Gate basis is unsupported")
	}
	pending := r.Action.Subject.ApplicationBinding.Base.PendingAction
	if r.Intent.Operation.Kind != runtimeports.OperationScopeRunV3 ||
		r.Intent.Operation.RunID != r.Action.Subject.Base.Run.RunID ||
		r.Intent.Operation.ExecutionScopeDigest != r.Action.Subject.Base.ExecutionScopeDigest ||
		!runtimeports.SameExecutionScopeV2(r.Intent.Operation.ExecutionScope, r.Action.Subject.Base.Run.Scope) {
		return core.NewError(core.ErrorConflict, core.ReasonEffectFenceStale, "Review Gate Intent and frozen Action Run scope differ")
	}
	if r.Intent.Target != r.Target.Ref || r.Intent.Review.CandidateRevision != r.Target.Revision || r.Intent.Review.CandidateDigest != r.Target.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "Review Gate Intent and target differ")
	}
	if r.Intent.Payload.Schema != pending.Payload.Schema || r.Intent.Payload.ContentDigest != pending.Payload.ContentDigest || !reflect.DeepEqual(r.Intent.Payload.Inline, pending.Payload.Inline) {
		return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "Review Gate Intent and frozen Action payload differ")
	}
	return nil
}

func ReviewGateRequestSchemaV1() runtimeports.SchemaRefV2 {
	return runtimeports.SchemaRefV2{Namespace: "praxis.harness", Name: "review-gate-request", Version: "1.0.0", MediaType: "application/json", ContentDigest: core.DigestBytes([]byte("praxis.harness/schema/review-gate-request@1.0.0"))}
}

func ReviewGateResultSchemaV1() runtimeports.SchemaRefV2 {
	return runtimeports.SchemaRefV2{Namespace: "praxis.harness", Name: "review-gate-result", Version: "1.0.0", MediaType: "application/json", ContentDigest: core.DigestBytes([]byte("praxis.harness/schema/review-gate-result@1.0.0"))}
}

func DecodeReviewGateRequestV1(payload []byte, now time.Time) (ReviewGateRequestV1, error) {
	var request ReviewGateRequestV1
	if err := core.DecodeStrictJSON(payload, &request); err != nil {
		return ReviewGateRequestV1{}, err
	}
	if err := request.Validate(now); err != nil {
		return ReviewGateRequestV1{}, err
	}
	return request, nil
}

type ReviewGateReceiptV1 struct {
	ContractVersion        string                                           `json:"contract_version"`
	PhaseID                string                                           `json:"phase_id"`
	RunID                  core.AgentRunID                                  `json:"run_id"`
	SessionID              string                                           `json:"session_id"`
	SessionRevision        core.Revision                                    `json:"session_revision"`
	SessionDigest          core.Digest                                      `json:"session_digest"`
	Turn                   uint32                                           `json:"turn"`
	ActionRef              string                                           `json:"action_ref"`
	ActionRequestDigest    core.Digest                                      `json:"action_request_digest"`
	Target                 runtimeports.OperationReviewTargetRefV4          `json:"target"`
	Authorization          *runtimeports.OperationReviewAuthorizationRefV5  `json:"authorization,omitempty"`
	Basis                  runtimeports.OperationReviewAuthorizationBasisV5 `json:"basis"`
	ReviewProjectionDigest core.Digest                                      `json:"review_projection_digest,omitempty"`
	Decision               ReviewGatePhaseDecisionV1                        `json:"decision"`
	ErrorCategory          core.ErrorCategory                               `json:"error_category,omitempty"`
	Reason                 core.ReasonCode                                  `json:"reason,omitempty"`
	CheckedUnixNano        int64                                            `json:"checked_unix_nano"`
	ExpiresUnixNano        int64                                            `json:"expires_unix_nano"`
	Digest                 core.Digest                                      `json:"digest"`
}

func (r ReviewGateReceiptV1) DigestV1() (core.Digest, error) {
	copy := r
	copy.Digest = ""
	return core.CanonicalJSONDigest(reviewGateCanonicalDomainV1, ReviewGateContractVersionV1, "ReviewGateReceiptV1", copy)
}

func (r ReviewGateReceiptV1) Validate() error {
	if r.ContractVersion != ReviewGateContractVersionV1 || r.PhaseID != ReviewGatePhaseIDV1 || r.RunID == "" || strings.TrimSpace(r.SessionID) == "" || r.SessionRevision == 0 || r.Turn == 0 || strings.TrimSpace(r.ActionRef) == "" || r.CheckedUnixNano <= 0 || r.ExpiresUnixNano <= r.CheckedUnixNano {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Review Gate receipt identity or time is incomplete")
	}
	for _, digest := range []core.Digest{r.SessionDigest, r.ActionRequestDigest} {
		if err := digest.Validate(); err != nil {
			return err
		}
	}
	if err := r.Target.Validate(); err != nil {
		return err
	}
	switch r.Decision {
	case ReviewGateAllowV1:
		if r.Authorization == nil || r.Authorization.Validate() != nil || r.ErrorCategory != "" || r.Reason != "" || r.ReviewProjectionDigest.Validate() != nil {
			return core.NewError(core.ErrorConflict, core.ReasonReviewVerdictStale, "allow receipt must carry current Review material and no error")
		}
	case ReviewGateAskV1:
		if r.ErrorCategory == "" || r.Reason == "" {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "fail-closed Review Gate receipt requires a typed cause")
		}
		if r.Authorization == nil && (r.ErrorCategory != core.ErrorNotFound || r.Reason != core.ReasonReviewVerdictMissing) {
			return core.NewError(core.ErrorConflict, core.ReasonReviewVerdictMissing, "nil Authorization is only a typed first-review ask")
		}
		if r.Authorization != nil && r.Authorization.Validate() != nil {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "ask receipt Authorization ref is invalid")
		}
	case ReviewGateDenyV1, ReviewGateDeferV1:
		if r.Authorization == nil || r.Authorization.Validate() != nil || r.ErrorCategory == "" || r.Reason == "" {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "deny/defer requires an exact Authorization and typed cause")
		}
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "Review Gate decision is unsupported")
	}
	digest, err := r.DigestV1()
	if err != nil || digest != r.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Review Gate receipt digest drifted")
	}
	return nil
}

func SealReviewGateReceiptV1(r ReviewGateReceiptV1) (ReviewGateReceiptV1, error) {
	r.ContractVersion = ReviewGateContractVersionV1
	r.PhaseID = ReviewGatePhaseIDV1
	r.Digest = ""
	digest, err := r.DigestV1()
	if err != nil {
		return ReviewGateReceiptV1{}, err
	}
	r.Digest = digest
	return r, r.Validate()
}

type ReviewGateResultV1 struct {
	ContractVersion string                    `json:"contract_version"`
	Decision        ReviewGatePhaseDecisionV1 `json:"decision"`
	Receipt         ReviewGateReceiptV1       `json:"receipt"`
}

func (r ReviewGateResultV1) Validate() error {
	if r.ContractVersion != ReviewGateContractVersionV1 || r.Decision != r.Receipt.Decision {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidState, "Review Gate result and receipt differ")
	}
	return r.Receipt.Validate()
}

func DecodeReviewGateResultV1(payload []byte) (ReviewGateResultV1, error) {
	var result ReviewGateResultV1
	if err := core.DecodeStrictJSON(payload, &result); err != nil {
		return ReviewGateResultV1{}, err
	}
	if err := result.Validate(); err != nil {
		return ReviewGateResultV1{}, err
	}
	return result, nil
}
