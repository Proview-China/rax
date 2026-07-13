package effect

import (
	"fmt"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/union"
)

type ToolCallEvidence struct {
	ToolID          string
	ActionID        union.ActionID
	Mechanism       string
	Origin          union.CapabilityOrigin
	Owner           union.ExecutionOwner
	Input           []byte
	Output          []byte
	ResultOrigin    union.EventOrigin
	SideEffectState union.SideEffectState
	CompletedAt     time.Time
}

type ToolCallExpectation struct {
	ToolID        string
	ActionID      union.ActionID
	Owner         union.ExecutionOwner
	RequireOutput bool
}

type ToolCallValidation struct {
	Effect       union.EffectRecord
	Verification union.VerificationRecord
}

// ValidateToolCall records only digests of tool input/output. A synthetic or
// merely proposed tool result must never call this function as an Effect.
func ValidateToolCall(
	effectID union.EffectID,
	verificationID union.VerificationID,
	intentID union.IntentID,
	attemptID union.MechanismAttemptID,
	evidence ToolCallEvidence,
	expectation ToolCallExpectation,
) (ToolCallValidation, error) {
	if effectID == "" || verificationID == "" || intentID == "" || attemptID == "" || evidence.CompletedAt.IsZero() ||
		evidence.ToolID == "" || evidence.ActionID == "" || evidence.Mechanism == "" || evidence.Origin == "" ||
		evidence.Owner == "" || evidence.ResultOrigin == "" || evidence.SideEffectState == "" || len(evidence.Input) == 0 {
		return ToolCallValidation{}, fmt.Errorf("%w: tool call evidence is incomplete", ErrInvalidPolicy)
	}
	if evidence.ResultOrigin == union.EventOriginModel {
		return ToolCallValidation{}, fmt.Errorf("%w: model prose is not tool execution evidence", ErrInvalidPolicy)
	}
	status := union.VerificationVerified
	failureCode := ""
	if expectation.ToolID != "" && expectation.ToolID != evidence.ToolID {
		status, failureCode = union.VerificationContradicted, "tool_identity_mismatch"
	}
	if expectation.ActionID != "" && expectation.ActionID != evidence.ActionID {
		status, failureCode = union.VerificationContradicted, "action_identity_mismatch"
	}
	if expectation.Owner != "" && expectation.Owner != evidence.Owner {
		status, failureCode = union.VerificationContradicted, "execution_owner_mismatch"
	}
	if expectation.RequireOutput && len(evidence.Output) == 0 && status == union.VerificationVerified {
		status, failureCode = union.VerificationUnverified, "tool_output_unavailable"
	}
	inputDigest := digestBytes(evidence.Input)
	outputDigest := ""
	if len(evidence.Output) != 0 {
		outputDigest = digestBytes(evidence.Output)
	}
	ioDigest, err := digestValue(struct {
		InputDigest  string `json:"input_digest"`
		OutputDigest string `json:"output_digest,omitempty"`
	}{InputDigest: inputDigest, OutputDigest: outputDigest})
	if err != nil {
		return ToolCallValidation{}, err
	}
	observed := union.EffectRecord{
		ID: effectID, IntentIDs: []union.IntentID{intentID}, MechanismAttemptID: attemptID,
		Kind: "tool_call_completed", Target: evidence.ToolID,
		Payload: union.EffectPayload{ToolCall: &union.ToolCallEffect{
			ToolID: evidence.ToolID, ActionID: evidence.ActionID, Mechanism: evidence.Mechanism,
			Origin: evidence.Origin, Owner: evidence.Owner, Executed: true,
			InputDigest: inputDigest, OutputDigest: outputDigest, ResultOrigin: evidence.ResultOrigin,
			SideEffectState: evidence.SideEffectState,
		}},
		EvidenceRefs: []union.EvidenceRef{{
			Kind: "tool_call_io", Source: "praxis_tool_observer", Digest: ioDigest,
			CapturedAt: evidence.CompletedAt.UTC(), Sensitivity: "internal",
		}},
		ObservationSource: "praxis_tool_observer", VerificationStatus: status,
		VerificationRefs: []union.VerificationID{verificationID}, Confidence: string(status), OccurredAt: evidence.CompletedAt.UTC(),
	}
	verification := union.VerificationRecord{
		ID: verificationID, EffectIDs: []union.EffectID{effectID}, IntentIDs: []union.IntentID{intentID},
		Kind: "tool_call_postcondition", Status: status,
		Verifier:     union.VersionedIdentity{ID: "praxis.tool-observer", Version: "v1"},
		EvidenceRefs: append([]union.EvidenceRef(nil), observed.EvidenceRefs...), FailureCode: failureCode,
		CompletedAt: evidence.CompletedAt.UTC(),
	}
	if err := observed.Validate(); err != nil {
		return ToolCallValidation{}, fmt.Errorf("%w: invalid tool Effect: %v", ErrInvalidPolicy, err)
	}
	if err := verification.Validate(); err != nil {
		return ToolCallValidation{}, fmt.Errorf("%w: invalid tool verification: %v", ErrInvalidPolicy, err)
	}
	return ToolCallValidation{Effect: observed, Verification: verification}, nil
}
