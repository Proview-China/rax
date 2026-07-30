package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-run-service/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-run-service/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-run-service/service"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-run-service/storage/memory"
)

type errorAdapterV1 struct{ calls int }

func (a *errorAdapterV1) ExecuteCommandV1(context.Context, contract.AgentRunCommandEnvelopeV1) (contract.AgentRunCommandReceiptV1, error) {
	a.calls++
	return contract.AgentRunCommandReceiptV1{}, errors.New("owner reply unavailable")
}

type panicAdapterV1 struct{ calls int }

func (a *panicAdapterV1) ExecuteCommandV1(context.Context, contract.AgentRunCommandEnvelopeV1) (contract.AgentRunCommandReceiptV1, error) {
	a.calls++
	panic("owner panic after admission")
}

type invalidAdapterV1 struct{ calls int }

func (a *invalidAdapterV1) ExecuteCommandV1(context.Context, contract.AgentRunCommandEnvelopeV1) (contract.AgentRunCommandReceiptV1, error) {
	a.calls++
	return contract.AgentRunCommandReceiptV1{}, nil
}

type rejectedAdapterV1 struct {
	now   time.Time
	calls int
}

func (a *rejectedAdapterV1) ExecuteCommandV1(_ context.Context, command contract.AgentRunCommandEnvelopeV1) (contract.AgentRunCommandReceiptV1, error) {
	a.calls++
	commandRef, err := command.CommandRefV1()
	if err != nil {
		return contract.AgentRunCommandReceiptV1{}, err
	}
	return contract.SealAgentRunCommandReceiptV1(contract.AgentRunCommandReceiptV1{
		CommandRef: commandRef, IdempotencyKey: command.IdempotencyKey,
		CanonicalPayloadDigest: command.CanonicalPayloadDigest,
		OriginalRequestDigest:  command.RequestDigest,
		Status:                 contract.AgentRunCommandRejectedV1,
		Fault: &contract.FaultV1{
			Code: contract.FaultForbiddenV1, Reason: "owner_policy_denied",
			Message: "owner policy denied the command", CommandRef: &commandRef,
			TraceID: command.TraceID, RetryDirective: contract.RetryNoneV1,
		},
		TraceID: command.TraceID, RecordedUnixNano: contract.NewWireUnixNanoV1(a.now.UnixNano()),
	})
}

type recordReplyLostJournalV1 struct {
	inner ports.CommandJournalV1
	lost  bool
}

func (j *recordReplyLostJournalV1) ReserveCommandV1(ctx context.Context, command contract.AgentRunCommandEnvelopeV1) (ports.CommandJournalDispositionV1, ports.CommandJournalEntryV1, error) {
	return j.inner.ReserveCommandV1(ctx, command)
}
func (j *recordReplyLostJournalV1) RecordReceiptV1(ctx context.Context, command contract.AgentRunCommandEnvelopeV1, receipt contract.AgentRunCommandReceiptV1) error {
	if err := j.inner.RecordReceiptV1(ctx, command, receipt); err != nil {
		return err
	}
	if !j.lost {
		j.lost = true
		return contract.NewError(contract.FaultUnknownOutcomeV1, "injected_lost_reply", "receipt commit reply was lost")
	}
	return nil
}
func (j *recordReplyLostJournalV1) InspectReservedCommandV1(ctx context.Context, command contract.AgentRunCommandEnvelopeV1) (ports.CommandJournalEntryV1, error) {
	return j.inner.InspectReservedCommandV1(ctx, command)
}
func (j *recordReplyLostJournalV1) InspectCommandV1(ctx context.Context, request contract.InspectOriginalRequestV1) (ports.CommandJournalEntryV1, error) {
	return j.inner.InspectCommandV1(ctx, request)
}

func inspectOriginalRequestV1(t *testing.T, command contract.AgentRunCommandEnvelopeV1, now time.Time) contract.InspectOriginalRequestV1 {
	t.Helper()
	commandRef, err := command.CommandRefV1()
	if err != nil {
		t.Fatal(err)
	}
	request, err := contract.SealInspectOriginalRequestV1(contract.InspectOriginalRequestV1{
		RequestID: "inspect-command-1", TraceID: "inspect-command-trace-1",
		OriginalCommandRef: &commandRef, OriginalIdempotencyKey: command.IdempotencyKey,
		OriginalRequestDigest: command.RequestDigest,
		RequestedUnixNano:     contract.NewWireUnixNanoV1(now.UnixNano()),
		NotAfterUnixNano:      contract.NewWireUnixNanoV1(now.Add(time.Minute).UnixNano()),
	})
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func assertDurableIndeterminateV1(t *testing.T, adapter ports.CommandOwnerAdapterV1) {
	t.Helper()
	now := time.Unix(1_900_000_000, 0).UTC()
	journal := memory.NewJournalV1()
	runtime, err := service.NewV1(service.ConfigV1{Clock: func() time.Time { return now }, Journal: journal, Cancel: adapter})
	if err != nil {
		t.Fatal(err)
	}
	command := cancelCommandV1(t, now, "unknown owner outcome")
	result, err := runtime.CancelAgentRunV1(context.Background(), contract.CancelAgentRunRequestV1{Command: command})
	if err != nil {
		t.Fatal(err)
	}
	if result.Receipt.Status != contract.AgentRunCommandIndeterminateV1 || result.Receipt.Fault == nil {
		t.Fatalf("receipt=%+v want INDETERMINATE", result.Receipt)
	}
	commandRef, _ := command.CommandRefV1()
	if result.Receipt.Fault.Code != contract.FaultUnknownOutcomeV1 || result.Receipt.Fault.RetryDirective != contract.RetryInspectV1 || result.Receipt.Fault.CommandRef == nil || *result.Receipt.Fault.CommandRef != commandRef {
		t.Fatalf("fault lost exact inspect coordinates: %+v", result.Receipt.Fault)
	}
	inspected, err := runtime.InspectOriginalV1(context.Background(), inspectOriginalRequestV1(t, command, now))
	if err != nil {
		t.Fatal(err)
	}
	if inspected.Disposition != contract.InspectOriginalObservedV1 || inspected.CommandReceipt == nil || inspected.CommandReceipt.ReceiptDigest != result.Receipt.ReceiptDigest {
		t.Fatalf("inspect=%+v result=%+v", inspected, result)
	}
}

func TestOwnerErrorBecomesDurableExactUnknownOutcomeV1(t *testing.T) {
	assertDurableIndeterminateV1(t, &errorAdapterV1{})
}
func TestOwnerPanicBecomesDurableExactUnknownOutcomeV1(t *testing.T) {
	assertDurableIndeterminateV1(t, &panicAdapterV1{})
}
func TestOwnerInvalidReceiptBecomesDurableExactUnknownOutcomeV1(t *testing.T) {
	assertDurableIndeterminateV1(t, &invalidAdapterV1{})
}

func TestRejectedOwnerReceiptRemainsTypedResultV1(t *testing.T) {
	now := time.Unix(1_900_000_000, 0).UTC()
	adapter := &rejectedAdapterV1{now: now}
	runtime, err := service.NewV1(service.ConfigV1{Clock: func() time.Time { return now }, Journal: memory.NewJournalV1(), Cancel: adapter})
	if err != nil {
		t.Fatal(err)
	}
	result, err := runtime.CancelAgentRunV1(context.Background(), contract.CancelAgentRunRequestV1{Command: cancelCommandV1(t, now, "denied cancel")})
	if err != nil {
		t.Fatal(err)
	}
	if result.Receipt.Status != contract.AgentRunCommandRejectedV1 || result.Receipt.Fault == nil || result.Receipt.Fault.Code != contract.FaultForbiddenV1 {
		t.Fatalf("typed rejection was lost: %+v", result)
	}
}

func TestReceiptPersistenceLostReplyInspectsWithoutReexecutionV1(t *testing.T) {
	now := time.Unix(1_900_000_000, 0).UTC()
	adapter := &commandAdapterV1{now: now}
	journal := &recordReplyLostJournalV1{inner: memory.NewJournalV1()}
	runtime, err := service.NewV1(service.ConfigV1{Clock: func() time.Time { return now }, Journal: journal, Cancel: adapter})
	if err != nil {
		t.Fatal(err)
	}
	command := cancelCommandV1(t, now, "lost receipt reply")
	first, err := runtime.CancelAgentRunV1(context.Background(), contract.CancelAgentRunRequestV1{Command: command})
	if err != nil {
		t.Fatal(err)
	}
	replay, err := runtime.CancelAgentRunV1(context.Background(), contract.CancelAgentRunRequestV1{Command: command})
	if err != nil {
		t.Fatal(err)
	}
	if first.Receipt.ReceiptDigest != replay.Receipt.ReceiptDigest {
		t.Fatal("lost-reply recovery drifted")
	}
	if got := adapter.calls.Load(); got != 1 {
		t.Fatalf("owner calls=%d want=1", got)
	}
}

func TestInspectPendingCommandReturnsExactIndeterminateResultV1(t *testing.T) {
	now := time.Unix(1_900_000_000, 0).UTC()
	journal := memory.NewJournalV1()
	command := cancelCommandV1(t, now, "pending command")
	if disposition, _, err := journal.ReserveCommandV1(context.Background(), command); err != nil || disposition != ports.CommandJournalReservedV1 {
		t.Fatalf("reserve disposition=%s err=%v", disposition, err)
	}
	runtime, err := service.NewV1(service.ConfigV1{Clock: func() time.Time { return now }, Journal: journal})
	if err != nil {
		t.Fatal(err)
	}
	result, err := runtime.InspectOriginalV1(context.Background(), inspectOriginalRequestV1(t, command, now))
	if err != nil {
		t.Fatal(err)
	}
	commandRef, _ := command.CommandRefV1()
	if result.Disposition != contract.InspectOriginalIndeterminateV1 || result.Fault == nil || result.Fault.CommandRef == nil || *result.Fault.CommandRef != commandRef || result.Fault.RetryDirective != contract.RetryInspectV1 {
		t.Fatalf("pending inspect lost exact coordinates: %+v", result)
	}
}

func TestInspectMissingCommandReturnsTypedNotFoundResultV1(t *testing.T) {
	now := time.Unix(1_900_000_000, 0).UTC()
	command := cancelCommandV1(t, now, "missing command")
	runtime, err := service.NewV1(service.ConfigV1{Clock: func() time.Time { return now }, Journal: memory.NewJournalV1()})
	if err != nil {
		t.Fatal(err)
	}
	result, err := runtime.InspectOriginalV1(context.Background(), inspectOriginalRequestV1(t, command, now))
	if err != nil {
		t.Fatal(err)
	}
	if result.Disposition != contract.InspectOriginalNotFoundV1 || result.Fault == nil || result.Fault.Code != contract.FaultNotFoundV1 {
		t.Fatalf("missing inspect was not typed NOT_FOUND: %+v", result)
	}
}
