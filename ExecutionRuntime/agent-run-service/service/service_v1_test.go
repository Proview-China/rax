package service_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-run-service/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-run-service/service"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-run-service/storage/memory"
)

type commandAdapterV1 struct {
	now   time.Time
	calls atomic.Int64
}

func (a *commandAdapterV1) ExecuteCommandV1(_ context.Context, command contract.AgentRunCommandEnvelopeV1) (contract.AgentRunCommandReceiptV1, error) {
	a.calls.Add(1)
	commandRef, err := command.CommandRefV1()
	if err != nil {
		return contract.AgentRunCommandReceiptV1{}, err
	}
	current := command.Target.RunCurrent
	return contract.SealAgentRunCommandReceiptV1(contract.AgentRunCommandReceiptV1{
		CommandRef: commandRef, CurrentRef: &current,
		IdempotencyKey:         command.IdempotencyKey,
		CanonicalPayloadDigest: command.CanonicalPayloadDigest,
		OriginalRequestDigest:  command.RequestDigest,
		Status:                 contract.AgentRunCommandCompletedV1,
		TraceID:                command.TraceID,
		RecordedUnixNano:       contract.NewWireUnixNanoV1(a.now.UnixNano()),
	})
}

func digestV1(t *testing.T, value string) contract.DigestV1 {
	t.Helper()
	digest, err := contract.DigestJSONV1(value)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

func refV1(t *testing.T, kind, id string) contract.ExactRefWireV1 {
	t.Helper()
	return contract.ExactRefWireV1{Kind: kind, ID: id, Revision: contract.NewWireUint64V1(1), Digest: digestV1(t, kind+"/"+id)}
}

func cancelCommandV1(t *testing.T, now time.Time, reason string) contract.AgentRunCommandEnvelopeV1 {
	t.Helper()
	target, err := contract.SealAgentRunTargetV1(contract.AgentRunTargetV1{
		HostID: "host-1", StartID: "start-1",
		HostStartClaim: refV1(t, contract.AgentRunHostStartClaimKindV1, "start-claim-1"),
		ExecutionScope: refV1(t, contract.AgentRunExecutionScopeKindV1, "scope-1"),
		RunCurrent:     refV1(t, contract.AgentRunCurrentKindV1, "run-1"),
		AuthorityEpoch: contract.NewWireUint64V1(1),
	})
	if err != nil {
		t.Fatal(err)
	}
	command, err := contract.SealAgentRunCommandEnvelopeV1(contract.AgentRunCommandEnvelopeV1{
		CommandID: "command-1", TraceID: "trace-1", Kind: contract.AgentRunCommandCancelRunV1,
		Target: target, Actor: "actor-1",
		AuthorityRef:        refV1(t, "praxis.identity/authority", "authority-1"),
		Payload:             contract.AgentRunCommandPayloadV1{Reason: reason},
		ExpectedCurrent:     contract.ExpectedCurrentV1{Ref: target.RunCurrent, Epoch: target.AuthorityEpoch},
		IdempotencyKey:      "idempotency-1",
		SubmittedAtUnixNano: contract.NewWireUnixNanoV1(now.Add(-time.Second).UnixNano()),
		NotAfterUnixNano:    contract.NewWireUnixNanoV1(now.Add(time.Minute).UnixNano()),
	})
	if err != nil {
		t.Fatal(err)
	}
	return command
}

func TestCancelSameKeyReplaysExactReceiptV1(t *testing.T) {
	now := time.Unix(1_900_000_000, 0).UTC()
	adapter := &commandAdapterV1{now: now}
	runtime, err := service.NewV1(service.ConfigV1{
		Clock: func() time.Time { return now }, ResultTTL: time.Second,
		Journal: memory.NewJournalV1(), Cancel: adapter,
	})
	if err != nil {
		t.Fatal(err)
	}
	command := cancelCommandV1(t, now, "cancel requested")
	first, err := runtime.CancelAgentRunV1(context.Background(), contract.CancelAgentRunRequestV1{Command: command})
	if err != nil {
		t.Fatal(err)
	}
	replay, err := runtime.CancelAgentRunV1(context.Background(), contract.CancelAgentRunRequestV1{Command: command})
	if err != nil {
		t.Fatal(err)
	}
	if first.Receipt.ReceiptDigest != replay.Receipt.ReceiptDigest || first.ResultDigest != replay.ResultDigest {
		t.Fatalf("replay drifted: first=%+v replay=%+v", first, replay)
	}
	if got := adapter.calls.Load(); got != 1 {
		t.Fatalf("owner calls=%d want=1", got)
	}
}

func TestCancelSameKeyDifferentPayloadConflictsV1(t *testing.T) {
	now := time.Unix(1_900_000_000, 0).UTC()
	adapter := &commandAdapterV1{now: now}
	runtime, err := service.NewV1(service.ConfigV1{
		Clock: func() time.Time { return now }, Journal: memory.NewJournalV1(), Cancel: adapter,
	})
	if err != nil {
		t.Fatal(err)
	}
	first := cancelCommandV1(t, now, "first reason")
	if _, err := runtime.CancelAgentRunV1(context.Background(), contract.CancelAgentRunRequestV1{Command: first}); err != nil {
		t.Fatal(err)
	}
	conflict := cancelCommandV1(t, now, "different reason")
	if _, err := runtime.CancelAgentRunV1(context.Background(), contract.CancelAgentRunRequestV1{Command: conflict}); !contract.HasCode(err, contract.FaultIdempotencyConflictV1) {
		t.Fatalf("error=%v want IDEMPOTENCY_CONFLICT", err)
	}
	if got := adapter.calls.Load(); got != 1 {
		t.Fatalf("owner calls=%d want=1", got)
	}
}

func TestCancelConcurrentReplayLinearizesV1(t *testing.T) {
	now := time.Unix(1_900_000_000, 0).UTC()
	adapter := &commandAdapterV1{now: now}
	runtime, err := service.NewV1(service.ConfigV1{
		Clock: func() time.Time { return now }, Journal: memory.NewJournalV1(), Cancel: adapter,
	})
	if err != nil {
		t.Fatal(err)
	}
	command := cancelCommandV1(t, now, "concurrent cancel")
	const workers = 64
	results := make(chan contract.DigestV1, workers)
	errors := make(chan error, workers)
	var group sync.WaitGroup
	for index := 0; index < workers; index++ {
		group.Add(1)
		go func() {
			defer group.Done()
			result, err := runtime.CancelAgentRunV1(context.Background(), contract.CancelAgentRunRequestV1{Command: command})
			if err != nil {
				errors <- err
				return
			}
			results <- result.Receipt.ReceiptDigest
		}()
	}
	group.Wait()
	close(results)
	close(errors)
	for err := range errors {
		t.Error(err)
	}
	var first contract.DigestV1
	for result := range results {
		if first == "" {
			first = result
		}
		if result != first {
			t.Fatalf("receipt digest drifted: %s != %s", result, first)
		}
	}
	if got := adapter.calls.Load(); got != 1 {
		t.Fatalf("owner calls=%d want=1", got)
	}
}
