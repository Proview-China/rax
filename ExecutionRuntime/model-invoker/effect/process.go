package effect

import (
	"fmt"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/union"
)

type ProcessEvidence struct {
	Mechanism              string
	Origin                 union.CapabilityOrigin
	Argv                   []string
	RuntimeIdentity        string
	EnvironmentFingerprint string
	ExitCode               *int
	Stdout                 []byte
	Stderr                 []byte
	Duration               time.Duration
	NetworkEvidence        []union.EvidenceRef
	CompletedAt            time.Time
}

type ProcessExpectation struct {
	Argv                   []string
	RuntimeIdentity        string
	AllowedExitCodes       []int
	RequireNetworkEvidence bool
}

type ProcessValidation struct {
	Effect       union.EffectRecord
	Verification union.VerificationRecord
}

func ValidateCodeExecution(
	effectID union.EffectID,
	verificationID union.VerificationID,
	intentID union.IntentID,
	attemptID union.MechanismAttemptID,
	evidence ProcessEvidence,
	expectation ProcessExpectation,
) (ProcessValidation, error) {
	if effectID == "" || verificationID == "" || intentID == "" || attemptID == "" || evidence.CompletedAt.IsZero() {
		return ProcessValidation{}, fmt.Errorf("%w: process validation identity is incomplete", ErrInvalidPolicy)
	}
	if evidence.Mechanism == "" {
		evidence.Mechanism = "caller_hosted_shell"
	}
	if evidence.Origin == "" {
		evidence.Origin = union.CapabilityOriginCallerHosted
	}
	status := union.VerificationVerified
	failureCode := ""
	if evidence.ExitCode == nil {
		status, failureCode = union.VerificationUnverified, "exit_code_unavailable"
	} else if len(expectation.AllowedExitCodes) > 0 && !containsInt(expectation.AllowedExitCodes, *evidence.ExitCode) {
		status, failureCode = union.VerificationContradicted, "exit_code_rejected"
	}
	if expectation.RuntimeIdentity != "" && expectation.RuntimeIdentity != evidence.RuntimeIdentity {
		status, failureCode = union.VerificationContradicted, "runtime_identity_mismatch"
	}
	if len(expectation.Argv) > 0 && !equalStrings(expectation.Argv, evidence.Argv) {
		status, failureCode = union.VerificationContradicted, "argv_mismatch"
	}
	if expectation.RequireNetworkEvidence && len(evidence.NetworkEvidence) == 0 && status == union.VerificationVerified {
		status, failureCode = union.VerificationUnverified, "network_evidence_unavailable"
	}
	stdoutDigest := digestBytes(evidence.Stdout)
	stderrDigest := digestBytes(evidence.Stderr)
	effect := union.EffectRecord{
		ID: effectID, IntentIDs: []union.IntentID{intentID}, MechanismAttemptID: attemptID,
		Kind: "code_execution_completed", Target: evidence.RuntimeIdentity,
		Payload: union.EffectPayload{CodeExecution: &union.CodeExecutionEffect{
			Mechanism: evidence.Mechanism, Origin: evidence.Origin,
			Argv: append([]string(nil), evidence.Argv...), RuntimeIdentity: evidence.RuntimeIdentity,
			EnvironmentFingerprint: evidence.EnvironmentFingerprint, ExitCode: cloneInt(evidence.ExitCode),
			StdoutRef: stdoutDigest, StderrRef: stderrDigest, Duration: evidence.Duration,
			NetworkEvidence: append([]union.EvidenceRef(nil), evidence.NetworkEvidence...),
		}},
		EvidenceRefs: []union.EvidenceRef{
			{Kind: "process_stdout", Source: "praxis_process_supervisor", Digest: stdoutDigest, CapturedAt: evidence.CompletedAt.UTC(), Sensitivity: "internal"},
			{Kind: "process_stderr", Source: "praxis_process_supervisor", Digest: stderrDigest, CapturedAt: evidence.CompletedAt.UTC(), Sensitivity: "internal"},
		},
		ObservationSource: "praxis_process_supervisor", VerificationStatus: status,
		VerificationRefs: []union.VerificationID{verificationID}, Confidence: string(status), OccurredAt: evidence.CompletedAt.UTC(),
	}
	verification := union.VerificationRecord{
		ID: verificationID, EffectIDs: []union.EffectID{effectID}, IntentIDs: []union.IntentID{intentID},
		Kind: "process_postcondition", Status: status,
		Verifier:     union.VersionedIdentity{ID: "praxis.process-evidence", Version: "v1"},
		EvidenceRefs: append([]union.EvidenceRef(nil), effect.EvidenceRefs...), FailureCode: failureCode,
		CompletedAt: evidence.CompletedAt.UTC(),
	}
	if err := effect.Validate(); err != nil {
		return ProcessValidation{}, fmt.Errorf("%w: invalid code execution Effect: %v", ErrInvalidPolicy, err)
	}
	if err := verification.Validate(); err != nil {
		return ProcessValidation{}, fmt.Errorf("%w: invalid code execution verification: %v", ErrInvalidPolicy, err)
	}
	return ProcessValidation{Effect: effect, Verification: verification}, nil
}

func containsInt(values []int, target int) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}
