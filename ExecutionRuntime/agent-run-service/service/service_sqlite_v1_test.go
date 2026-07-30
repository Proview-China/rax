package service_test

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-run-service/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-run-service/service"
	storesqlite "github.com/Proview-China/rax/ExecutionRuntime/agent-run-service/storage/sqlite"
)

func TestSQLiteJournalCrossServiceLinearizationAndRestartV1(t *testing.T) {
	now := time.Unix(1_900_000_000, 0).UTC()
	path := filepath.Join(t.TempDir(), "agent-run-service.sqlite")
	firstJournal, err := storesqlite.OpenJournalV1(context.Background(), storesqlite.ConfigV1{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	secondJournal, err := storesqlite.OpenJournalV1(context.Background(), storesqlite.ConfigV1{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	adapter := &commandAdapterV1{now: now}
	first, err := service.NewV1(service.ConfigV1{Clock: func() time.Time { return now }, Journal: firstJournal, Cancel: adapter})
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.NewV1(service.ConfigV1{Clock: func() time.Time { return now }, Journal: secondJournal, Cancel: adapter})
	if err != nil {
		t.Fatal(err)
	}
	command := cancelCommandV1(t, now, "cross-service cancel")
	services := []*service.ServiceV1{first, second}
	const workers = 64
	var group sync.WaitGroup
	errors := make(chan error, workers)
	for index := 0; index < workers; index++ {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			result, err := services[index%len(services)].CancelAgentRunV1(context.Background(), contract.CancelAgentRunRequestV1{Command: command})
			if err != nil {
				errors <- err
				return
			}
			if result.Receipt.Status != contract.AgentRunCommandCompletedV1 && result.Receipt.Status != contract.AgentRunCommandIndeterminateV1 {
				errors <- contract.NewError(contract.FaultInternalV1, "unexpected_test_receipt", "concurrent receipt was neither completed nor indeterminate")
			}
		}(index)
	}
	group.Wait()
	close(errors)
	for err := range errors {
		t.Error(err)
	}
	if got := adapter.calls.Load(); got != 1 {
		t.Fatalf("owner calls=%d want=1", got)
	}
	completed, err := second.CancelAgentRunV1(context.Background(), contract.CancelAgentRunRequestV1{Command: command})
	if err != nil {
		t.Fatal(err)
	}
	if completed.Receipt.Status != contract.AgentRunCommandCompletedV1 {
		t.Fatalf("final receipt status=%s want COMPLETED", completed.Receipt.Status)
	}
	if err := firstJournal.Close(); err != nil {
		t.Fatal(err)
	}
	if err := secondJournal.Close(); err != nil {
		t.Fatal(err)
	}

	restartedJournal, err := storesqlite.OpenJournalV1(context.Background(), storesqlite.ConfigV1{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	defer restartedJournal.Close()
	restartedAdapter := &commandAdapterV1{now: now}
	restarted, err := service.NewV1(service.ConfigV1{Clock: func() time.Time { return now }, Journal: restartedJournal, Cancel: restartedAdapter})
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := restarted.CancelAgentRunV1(context.Background(), contract.CancelAgentRunRequestV1{Command: command})
	if err != nil {
		t.Fatal(err)
	}
	if replayed.Receipt.ReceiptDigest != completed.Receipt.ReceiptDigest {
		t.Fatalf("restart receipt drifted: %s != %s", replayed.Receipt.ReceiptDigest, completed.Receipt.ReceiptDigest)
	}
	if got := restartedAdapter.calls.Load(); got != 0 {
		t.Fatalf("restart owner calls=%d want=0", got)
	}
}

func TestSQLiteInspectOriginalReturnsDurableReceiptV1(t *testing.T) {
	now := time.Unix(1_900_000_000, 0).UTC()
	journal, err := storesqlite.OpenJournalV1(context.Background(), storesqlite.ConfigV1{Path: filepath.Join(t.TempDir(), "inspect.sqlite")})
	if err != nil {
		t.Fatal(err)
	}
	defer journal.Close()
	adapter := &commandAdapterV1{now: now}
	runtime, err := service.NewV1(service.ConfigV1{Clock: func() time.Time { return now }, Journal: journal, Cancel: adapter})
	if err != nil {
		t.Fatal(err)
	}
	command := cancelCommandV1(t, now, "inspect cancel")
	completed, err := runtime.CancelAgentRunV1(context.Background(), contract.CancelAgentRunRequestV1{Command: command})
	if err != nil {
		t.Fatal(err)
	}
	commandRef, _ := command.CommandRefV1()
	request, err := contract.SealInspectOriginalRequestV1(contract.InspectOriginalRequestV1{
		RequestID: "inspect-request-1", TraceID: "inspect-trace-1",
		OriginalCommandRef: &commandRef, OriginalIdempotencyKey: command.IdempotencyKey,
		OriginalRequestDigest: command.RequestDigest,
		RequestedUnixNano:     contract.NewWireUnixNanoV1(now.UnixNano()),
		NotAfterUnixNano:      contract.NewWireUnixNanoV1(now.Add(time.Minute).UnixNano()),
	})
	if err != nil {
		t.Fatal(err)
	}
	observed, err := runtime.InspectOriginalV1(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if observed.Disposition != contract.InspectOriginalObservedV1 || observed.CommandReceipt == nil {
		t.Fatalf("inspect result=%+v", observed)
	}
	if observed.CommandReceipt.ReceiptDigest != completed.Receipt.ReceiptDigest {
		t.Fatalf("inspect receipt drifted")
	}
}
