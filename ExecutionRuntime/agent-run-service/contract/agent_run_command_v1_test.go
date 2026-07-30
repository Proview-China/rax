package contract_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-run-service/contract"
)

func TestAgentRunCommandV1SameKeySamePayloadReplaysOriginalReceipt(t *testing.T) {
	original := cancelCommandV1(t, "command-1", "idem-1", "operator_cancel")
	retryInput := original
	retryInput.CommandID = "command-retry"
	retryInput.SubmittedAtUnixNano = contract.NewWireUnixNanoV1(110)
	retryInput.CanonicalPayloadDigest = ""
	retryInput.RequestDigest = ""
	retry, err := contract.SealAgentRunCommandEnvelopeV1(retryInput)
	if err != nil {
		t.Fatal(err)
	}
	if retry.RequestDigest == original.RequestDigest {
		t.Fatal("request digest should preserve distinct command envelope identity")
	}
	classification, err := contract.ClassifyAgentRunCommandReplayV1(original, retry)
	if err != nil || classification != contract.IdempotencyReplayOriginalV1 {
		t.Fatalf("classification=%s err=%v", classification, err)
	}
	receipt := acceptedReceiptV1(t, original)
	if err := receipt.ValidateForReplay(original, retry); err != nil {
		t.Fatal(err)
	}
}

func TestAgentRunCommandV1SameKeyDifferentPayloadConflicts(t *testing.T) {
	original := cancelCommandV1(t, "command-1", "idem-1", "operator_cancel")
	retry := cancelCommandV1(t, "command-2", "idem-1", "different_reason")
	if _, err := contract.ClassifyAgentRunCommandReplayV1(original, retry); err == nil || !contract.HasCode(err, contract.FaultIdempotencyConflictV1) {
		t.Fatalf("same key different payload err=%v", err)
	}

	drift := original
	drift.Target.RunCurrent = exactRefV1(t, contract.AgentRunCurrentKindV1, "run-1", 10)
	drift.Target.TargetDigest = ""
	sealedTarget, err := contract.SealAgentRunTargetV1(drift.Target)
	if err != nil {
		t.Fatal(err)
	}
	drift.Target = sealedTarget
	drift.ExpectedCurrent.Ref = drift.Target.RunCurrent
	drift.CanonicalPayloadDigest = ""
	drift.RequestDigest = ""
	drift, err = contract.SealAgentRunCommandEnvelopeV1(drift)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := contract.ClassifyAgentRunCommandReplayV1(original, drift); err == nil || !contract.HasCode(err, contract.FaultIdempotencyConflictV1) {
		t.Fatalf("expected-current payload drift err=%v", err)
	}
}

func TestAgentRunCommandV1RejectsCommandIDReboundToAnotherKey(t *testing.T) {
	original := cancelCommandV1(t, "command-1", "idem-1", "operator_cancel")
	rebound := cancelCommandV1(t, "command-1", "idem-2", "operator_cancel")
	if _, err := contract.ClassifyAgentRunCommandReplayV1(original, rebound); err == nil || !contract.HasCode(err, contract.FaultRevisionConflictV1) {
		t.Fatalf("command identity rebound err=%v", err)
	}
}

func TestAgentRunTargetV1RejectsCoordinateSplice(t *testing.T) {
	target := targetV1(t)
	target.HostStartClaim = exactRefV1(t, contract.AgentRunHostStartClaimKindV1, "claim-splice", 1)
	if err := target.Validate(); err == nil || !contract.HasCode(err, contract.FaultRevisionConflictV1) {
		t.Fatalf("target splice err=%v", err)
	}
}

func TestAgentRunCommandV1WindowBeforeEqualAfter(t *testing.T) {
	command := cancelCommandV1(t, "command-1", "idem-1", "operator_cancel")
	cases := []struct {
		name    string
		now     int64
		wantErr bool
	}{
		{"clock_regression", 99, true},
		{"at_submission", 100, false},
		{"before_not_after", 199, false},
		{"at_not_after", 200, true},
		{"after_not_after", 201, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := command.ValidateCurrentV1(contract.NewWireUnixNanoV1(tc.now))
			if (err != nil) != tc.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, tc.wantErr)
			}
		})
	}
}

func TestAgentRunCommandV1SeparatesCancelAndStop(t *testing.T) {
	target := targetV1(t)
	hostJournal := exactRefV1(t, "praxis.agent-host/journal", "journal-1", 4)
	cleanup := exactRefV1(t, "praxis.agent-host/cleanup-closure", "closure-1", 2)
	stop, err := contract.SealAgentRunCommandEnvelopeV1(contract.AgentRunCommandEnvelopeV1{
		CommandID:    "stop-1",
		TraceID:      "trace-1",
		Kind:         contract.AgentRunCommandStopHostV1,
		Target:       target,
		Actor:        "actor-1",
		AuthorityRef: exactRefV1(t, "praxis.agent-host/authority-current", "host-authority-1", 7),
		Payload: contract.AgentRunCommandPayloadV1{
			Reason:         "operator_stop",
			HostJournal:    &hostJournal,
			CleanupClosure: &cleanup,
		},
		ExpectedCurrent:     contract.ExpectedCurrentV1{Ref: hostJournal, Epoch: target.AuthorityEpoch},
		IdempotencyKey:      "stop-idem-1",
		SubmittedAtUnixNano: contract.NewWireUnixNanoV1(100),
		NotAfterUnixNano:    contract.NewWireUnixNanoV1(200),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := (contract.StopAgentHostRequestV1{Command: stop}).Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (contract.CancelAgentRunRequestV1{Command: stop}).Validate(); err == nil {
		t.Fatal("Host Stop was accepted as Run Cancel")
	}
}

func TestAgentRunCommandV1IndeterminateReceiptPreservesUnknownOutcome(t *testing.T) {
	command := cancelCommandV1(t, "command-1", "idem-1", "operator_cancel")
	commandRef, _ := command.CommandRefV1()
	fault := contract.FaultV1{
		Code:           contract.FaultUnknownOutcomeV1,
		Reason:         "lost_reply",
		Message:        "owner reply was lost",
		CommandRef:     &commandRef,
		TraceID:        command.TraceID,
		RetryDirective: contract.RetryInspectV1,
	}
	receipt, err := contract.SealAgentRunCommandReceiptV1(contract.AgentRunCommandReceiptV1{
		CommandRef:             commandRef,
		IdempotencyKey:         command.IdempotencyKey,
		CanonicalPayloadDigest: command.CanonicalPayloadDigest,
		OriginalRequestDigest:  command.RequestDigest,
		Status:                 contract.AgentRunCommandIndeterminateV1,
		Fault:                  &fault,
		TraceID:                command.TraceID,
		RecordedUnixNano:       contract.NewWireUnixNanoV1(120),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := receipt.ValidateFor(command); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"code":"UNKNOWN_OUTCOME"`) || !strings.Contains(string(encoded), `"retry_directive":"INSPECT"`) {
		t.Fatalf("unknown outcome semantics lost: %s", encoded)
	}
}

func TestAgentRunCommandResultV1BindsTraceAndExactRequestWindow(t *testing.T) {
	command := cancelCommandV1(t, "command-1", "idem-1", "operator_cancel")
	receipt := acceptedReceiptV1(t, command)
	result, err := contract.SealCommandResultV1(contract.CommandResultV1{
		RequestDigest: command.RequestDigest,
		TraceID:       command.TraceID,
		Receipt:       receipt,
		Window: contract.WireValidityWindowV1{
			CheckedUnixNano: contract.NewWireUnixNanoV1(120),
			ExpiresUnixNano: contract.NewWireUnixNanoV1(190),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := result.ValidateFor(command); err != nil {
		t.Fatal(err)
	}

	expired := result
	expired.Window = contract.WireValidityWindowV1{
		CheckedUnixNano: contract.NewWireUnixNanoV1(120),
		ExpiresUnixNano: contract.NewWireUnixNanoV1(201),
	}
	expired.ResultDigest = ""
	expired, err = contract.SealCommandResultV1(expired)
	if err != nil {
		t.Fatal(err)
	}
	if err := expired.ValidateFor(command); err == nil || !contract.HasCode(err, contract.FaultPreconditionFailedV1) {
		t.Fatalf("command result outside request window err=%v", err)
	}

	spliced := result
	spliced.TraceID = "trace-splice"
	spliced.ResultDigest = ""
	if _, err := contract.SealCommandResultV1(spliced); err == nil || !contract.HasCode(err, contract.FaultRevisionConflictV1) {
		t.Fatalf("command result trace splice err=%v", err)
	}
}

func TestAgentRunCommandV1UnknownStatusIsRejected(t *testing.T) {
	command := cancelCommandV1(t, "command-1", "idem-1", "operator_cancel")
	receipt := acceptedReceiptV1(t, command)
	receipt.Status = contract.AgentRunCommandStatusV1("FUTURE_STATUS")
	receipt.ReceiptDigest = ""
	if _, err := contract.SealAgentRunCommandReceiptV1(receipt); err == nil || !contract.HasCode(err, contract.FaultInvalidArgumentV1) {
		t.Fatalf("unknown status err=%v", err)
	}
}
