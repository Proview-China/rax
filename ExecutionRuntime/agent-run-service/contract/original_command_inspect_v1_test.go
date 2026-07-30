package contract_test

import (
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-run-service/contract"
)

func TestInspectOriginalV1RecoversLostCommandReplyWithoutSecondCommand(t *testing.T) {
	command := cancelCommandV1(t, "command-1", "idem-1", "operator_cancel")
	commandRef, _ := command.CommandRefV1()
	request, err := contract.SealInspectOriginalRequestV1(contract.InspectOriginalRequestV1{
		RequestID:              "inspect-original-1",
		TraceID:                command.TraceID,
		OriginalCommandRef:     &commandRef,
		OriginalIdempotencyKey: command.IdempotencyKey,
		OriginalRequestDigest:  command.RequestDigest,
		RequestedUnixNano:      contract.NewWireUnixNanoV1(130),
		NotAfterUnixNano:       contract.NewWireUnixNanoV1(230),
	})
	if err != nil {
		t.Fatal(err)
	}
	fault := contract.FaultV1{
		Code:           contract.FaultUnknownOutcomeV1,
		Reason:         "lost_reply",
		Message:        "owner still cannot prove the command outcome",
		CommandRef:     &commandRef,
		TraceID:        command.TraceID,
		RetryDirective: contract.RetryInspectV1,
	}
	result, err := contract.SealInspectOriginalResultV1(contract.InspectOriginalResultV1{
		RequestDigest: request.RequestDigest,
		TraceID:       request.TraceID,
		Disposition:   contract.InspectOriginalIndeterminateV1,
		Fault:         &fault,
		Window:        contract.WireValidityWindowV1{CheckedUnixNano: contract.NewWireUnixNanoV1(140), ExpiresUnixNano: contract.NewWireUnixNanoV1(220)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := result.ValidateFor(request); err != nil {
		t.Fatal(err)
	}
	old := result
	old.Window = contract.WireValidityWindowV1{CheckedUnixNano: contract.NewWireUnixNanoV1(129), ExpiresUnixNano: contract.NewWireUnixNanoV1(220)}
	old.ResultDigest = ""
	old, err = contract.SealInspectOriginalResultV1(old)
	if err != nil {
		t.Fatal(err)
	}
	if err := old.ValidateFor(request); err == nil || !contract.HasCode(err, contract.FaultPreconditionFailedV1) {
		t.Fatalf("old Inspect Original snapshot err=%v", err)
	}
}

func TestInspectOriginalV1RejectsFaultSubjectTypeSplice(t *testing.T) {
	command := cancelCommandV1(t, "command-1", "idem-1", "operator_cancel")
	commandRef, _ := command.CommandRefV1()
	request, err := contract.SealInspectOriginalRequestV1(contract.InspectOriginalRequestV1{
		RequestID:              "inspect-original-splice",
		TraceID:                command.TraceID,
		OriginalCommandRef:     &commandRef,
		OriginalIdempotencyKey: command.IdempotencyKey,
		OriginalRequestDigest:  command.RequestDigest,
		RequestedUnixNano:      contract.NewWireUnixNanoV1(130),
		NotAfterUnixNano:       contract.NewWireUnixNanoV1(230),
	})
	if err != nil {
		t.Fatal(err)
	}
	otherAttempt := exactRefV1(t, "praxis.runtime/attempt", "attempt-other", 1)
	fault := contract.FaultV1{
		Code:           contract.FaultNotFoundV1,
		Reason:         "command_not_found",
		Message:        "the original command was not found",
		AttemptRef:     &otherAttempt,
		TraceID:        request.TraceID,
		RetryDirective: contract.RetryNoneV1,
	}
	result, err := contract.SealInspectOriginalResultV1(contract.InspectOriginalResultV1{
		RequestDigest: request.RequestDigest,
		TraceID:       request.TraceID,
		Disposition:   contract.InspectOriginalNotFoundV1,
		Fault:         &fault,
		Window:        contract.WireValidityWindowV1{CheckedUnixNano: contract.NewWireUnixNanoV1(140), ExpiresUnixNano: contract.NewWireUnixNanoV1(220)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := result.ValidateFor(request); err == nil || !contract.HasCode(err, contract.FaultRevisionConflictV1) {
		t.Fatalf("fault subject type splice err=%v", err)
	}
}

func TestInspectOriginalV1SupportsAttemptOnlyAndRejectsAmbiguity(t *testing.T) {
	attemptRef := exactRefV1(t, "praxis.runtime/attempt", "attempt-1", 3)
	request, err := contract.SealInspectOriginalRequestV1(contract.InspectOriginalRequestV1{
		RequestID:          "inspect-attempt-1",
		TraceID:            "trace-1",
		OriginalAttemptRef: &attemptRef,
		RequestedUnixNano:  contract.NewWireUnixNanoV1(130),
		NotAfterUnixNano:   contract.NewWireUnixNanoV1(230),
	})
	if err != nil {
		t.Fatal(err)
	}
	fault := contract.FaultV1{Code: contract.FaultIndeterminateV1, Reason: "attempt_unknown", Message: "attempt outcome is indeterminate", AttemptRef: &attemptRef, TraceID: request.TraceID, RetryDirective: contract.RetryInspectV1}
	result, err := contract.SealInspectOriginalResultV1(contract.InspectOriginalResultV1{
		RequestDigest: request.RequestDigest,
		TraceID:       request.TraceID,
		Disposition:   contract.InspectOriginalIndeterminateV1,
		Fault:         &fault,
		Window:        contract.WireValidityWindowV1{CheckedUnixNano: contract.NewWireUnixNanoV1(140), ExpiresUnixNano: contract.NewWireUnixNanoV1(220)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := result.ValidateFor(request); err != nil {
		t.Fatal(err)
	}

	commandRef := exactRefV1(t, contract.AgentRunCommandRefKindV1, "command-1", 1)
	_, err = contract.SealInspectOriginalRequestV1(contract.InspectOriginalRequestV1{
		RequestID:          "ambiguous-1",
		TraceID:            "trace-1",
		OriginalCommandRef: &commandRef,
		OriginalAttemptRef: &attemptRef,
		RequestedUnixNano:  contract.NewWireUnixNanoV1(130),
		NotAfterUnixNano:   contract.NewWireUnixNanoV1(230),
	})
	if err == nil {
		t.Fatal("accepted ambiguous command and attempt originals")
	}
}
