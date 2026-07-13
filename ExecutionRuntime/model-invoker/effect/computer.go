package effect

import (
	"fmt"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/union"
)

type ComputerEvidence struct {
	Mechanism        string
	Origin           union.CapabilityOrigin
	Action           string
	Target           string
	BeforeRefs       []union.EvidenceRef
	AfterRefs        []union.EvidenceRef
	ExternalReadback string
	CompletedAt      time.Time
}

type ComputerExpectation struct {
	RequireBeforeAfter      bool
	RequireExternalReadback bool
	Irreversible            bool
	Approved                bool
}

type ComputerValidation struct {
	Effect       union.EffectRecord
	Verification union.VerificationRecord
}

func ValidateComputerUse(
	effectID union.EffectID,
	verificationID union.VerificationID,
	intentID union.IntentID,
	attemptID union.MechanismAttemptID,
	evidence ComputerEvidence,
	expectation ComputerExpectation,
) (ComputerValidation, error) {
	if effectID == "" || verificationID == "" || intentID == "" || attemptID == "" || evidence.CompletedAt.IsZero() ||
		evidence.Action == "" || evidence.Target == "" {
		return ComputerValidation{}, fmt.Errorf("%w: computer validation identity is incomplete", ErrInvalidPolicy)
	}
	if evidence.Mechanism == "" {
		evidence.Mechanism = "caller_computer_use"
	}
	if evidence.Origin == "" {
		evidence.Origin = union.CapabilityOriginCallerHosted
	}
	status := union.VerificationVerified
	failureCode := ""
	if expectation.Irreversible && !expectation.Approved {
		status, failureCode = union.VerificationContradicted, "irreversible_action_not_approved"
	} else if expectation.RequireBeforeAfter && (len(evidence.BeforeRefs) == 0 || len(evidence.AfterRefs) == 0) {
		status, failureCode = union.VerificationUnverified, "state_evidence_unavailable"
	} else if expectation.RequireExternalReadback && evidence.ExternalReadback == "" {
		status, failureCode = union.VerificationUnverified, "external_readback_unavailable"
	} else if len(evidence.AfterRefs) == 0 && evidence.ExternalReadback == "" {
		// An action command is a Mechanism observation, not proof that external
		// state changed. Without after-state or independent readback, retain the
		// Effect but fail closed as unverified.
		status, failureCode = union.VerificationUnverified, "effect_evidence_unavailable"
	}
	allEvidence := append(append([]union.EvidenceRef(nil), evidence.BeforeRefs...), evidence.AfterRefs...)
	effect := union.EffectRecord{
		ID: effectID, IntentIDs: []union.IntentID{intentID}, MechanismAttemptID: attemptID,
		Kind: "computer_action_observed", Target: evidence.Target,
		Payload: union.EffectPayload{ComputerUse: &union.ComputerUseEffect{
			Mechanism: evidence.Mechanism, Origin: evidence.Origin, Action: evidence.Action, Target: evidence.Target,
			BeforeRefs: append([]union.EvidenceRef(nil), evidence.BeforeRefs...), AfterRefs: append([]union.EvidenceRef(nil), evidence.AfterRefs...),
			ExternalReadback: evidence.ExternalReadback,
		}},
		EvidenceRefs: allEvidence, ObservationSource: "praxis_computer_observer", VerificationStatus: status,
		VerificationRefs: []union.VerificationID{verificationID}, Confidence: string(status), OccurredAt: evidence.CompletedAt.UTC(),
	}
	verification := union.VerificationRecord{
		ID: verificationID, EffectIDs: []union.EffectID{effectID}, IntentIDs: []union.IntentID{intentID},
		Kind: "computer_postcondition", Status: status,
		Verifier:     union.VersionedIdentity{ID: "praxis.computer-evidence", Version: "v1"},
		EvidenceRefs: append([]union.EvidenceRef(nil), allEvidence...), FailureCode: failureCode, CompletedAt: evidence.CompletedAt.UTC(),
	}
	if err := effect.Validate(); err != nil {
		return ComputerValidation{}, fmt.Errorf("%w: invalid computer Effect: %v", ErrInvalidPolicy, err)
	}
	if err := verification.Validate(); err != nil {
		return ComputerValidation{}, fmt.Errorf("%w: invalid computer verification: %v", ErrInvalidPolicy, err)
	}
	return ComputerValidation{Effect: effect, Verification: verification}, nil
}
