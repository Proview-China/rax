package reviewer

import (
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type TerminationPolicyV1 struct {
	MaxRounds            uint32
	MaxTokens            uint64
	MaxDuration          time.Duration
	MaxCostMicros        uint64
	MaxRepeatedFinding   uint32
	MaxRepeatedRejection uint32
	// AllowObservationDegrade applies only to a non-authorizing, observation-
	// only result review. It never converts a reviewer failure into Accept.
	AllowObservationDegrade bool
}

func (p TerminationPolicyV1) Validate() error {
	if p.MaxRounds == 0 || p.MaxTokens == 0 || p.MaxDuration <= 0 || p.MaxCostMicros == 0 || p.MaxRepeatedFinding == 0 || p.MaxRepeatedRejection == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "reviewer termination policy requires explicit positive bounds")
	}
	return nil
}

type TerminationProgressV1 struct {
	Rounds                        uint32
	Tokens                        uint64
	Elapsed                       time.Duration
	CostMicros                    uint64
	RepeatedFinding               uint32
	RepeatedRejection             uint32
	ParseFailed                   bool
	Unavailable                   bool
	UnknownOutcome                bool
	ObservationOnly               bool
	StructuredAttestationProduced bool
	// OutputResolution is the exact structured resolution when an output was
	// produced. Empty preserves the legacy one-shot completion signal.
	OutputResolution contract.ResolutionV1
}

type TerminationDecisionV1 string

const (
	TerminationContinueV1         TerminationDecisionV1 = "continue"
	TerminationCompleteV1         TerminationDecisionV1 = "complete"
	TerminationEscalateHumanV1    TerminationDecisionV1 = "escalate_human"
	TerminationFailClosedV1       TerminationDecisionV1 = "fail_closed"
	TerminationDeferObservationV1 TerminationDecisionV1 = "defer_observation"
	TerminationInspectOriginalV1  TerminationDecisionV1 = "inspect_original_attempt"
)

type TerminationResultV1 struct {
	Decision TerminationDecisionV1
	Reason   string
}

func EvaluateTerminationV1(policy TerminationPolicyV1, progress TerminationProgressV1) (TerminationResultV1, error) {
	if err := policy.Validate(); err != nil {
		return TerminationResultV1{}, err
	}
	if progress.UnknownOutcome {
		return TerminationResultV1{Decision: TerminationInspectOriginalV1, Reason: "unknown_outcome"}, nil
	}
	if progress.ParseFailed || progress.Unavailable {
		if progress.ObservationOnly && policy.AllowObservationDegrade {
			return TerminationResultV1{Decision: TerminationDeferObservationV1, Reason: "observation_only_policy_degrade"}, nil
		}
		return TerminationResultV1{Decision: TerminationFailClosedV1, Reason: "reviewer_unavailable_or_invalid"}, nil
	}
	// Resource and repetition ceilings are Owner-enforced hard bounds. A
	// structured model output is still only an Observation and must not bypass
	// a ceiling reached by that same round.
	if progress.Tokens >= policy.MaxTokens || progress.Elapsed >= policy.MaxDuration || progress.CostMicros >= policy.MaxCostMicros || progress.RepeatedFinding >= policy.MaxRepeatedFinding || progress.RepeatedRejection >= policy.MaxRepeatedRejection {
		return TerminationResultV1{Decision: TerminationEscalateHumanV1, Reason: "review_budget_or_loop_bound_reached"}, nil
	}
	if progress.StructuredAttestationProduced {
		switch progress.OutputResolution {
		case "", contract.ResolutionAcceptV1, contract.ResolutionConditionalV1, contract.ResolutionRejectV1:
			return TerminationResultV1{Decision: TerminationCompleteV1, Reason: "single_attestation_produced"}, nil
		case contract.ResolutionEscalateHumanV1:
			return TerminationResultV1{Decision: TerminationEscalateHumanV1, Reason: "reviewer_escalated_human"}, nil
		case contract.ResolutionRequestChangesV1, contract.ResolutionInsufficientEvidenceV1:
			if progress.Rounds >= policy.MaxRounds {
				return TerminationResultV1{Decision: TerminationEscalateHumanV1, Reason: "review_round_limit_reached"}, nil
			}
			return TerminationResultV1{Decision: TerminationCompleteV1, Reason: "single_attestation_produced"}, nil
		default:
			return TerminationResultV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "reviewer output resolution is invalid")
		}
	}
	if progress.Rounds >= policy.MaxRounds {
		return TerminationResultV1{Decision: TerminationEscalateHumanV1, Reason: "review_round_limit_reached"}, nil
	}
	return TerminationResultV1{Decision: TerminationContinueV1, Reason: "within_bounds"}, nil
}

// ProductionBaselineTerminationPolicyV1 freezes the user-approved Auto Review
// loop bounds. Cost remains a tenant Policy input and therefore has no guessed
// code default. Observation degradation is explicit and non-authorizing.
func ProductionBaselineTerminationPolicyV1(maxCostMicros uint64, allowObservationDegrade bool) (TerminationPolicyV1, error) {
	policy := TerminationPolicyV1{
		MaxRounds:               3,
		MaxTokens:               64_000,
		MaxDuration:             10 * time.Minute,
		MaxCostMicros:           maxCostMicros,
		MaxRepeatedFinding:      2,
		MaxRepeatedRejection:    2,
		AllowObservationDegrade: allowObservationDegrade,
	}
	return policy, policy.Validate()
}
