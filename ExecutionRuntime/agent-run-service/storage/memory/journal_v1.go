package memory

import (
	"context"
	"sync"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-run-service/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-run-service/ports"
)

type JournalV1 struct {
	mu    sync.Mutex
	byKey map[string]ports.CommandJournalEntryV1
	byID  map[string]string
}

func NewJournalV1() *JournalV1 {
	return &JournalV1{byKey: map[string]ports.CommandJournalEntryV1{}, byID: map[string]string{}}
}

func (j *JournalV1) ReserveCommandV1(_ context.Context, command contract.AgentRunCommandEnvelopeV1) (ports.CommandJournalDispositionV1, ports.CommandJournalEntryV1, error) {
	if err := command.Validate(); err != nil {
		return "", ports.CommandJournalEntryV1{}, err
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if key, ok := j.byID[command.CommandID]; ok && key != command.IdempotencyKey {
		return "", ports.CommandJournalEntryV1{}, contract.NewError(contract.FaultRevisionConflictV1, "command_identity_conflict", "same command ID was rebound to another idempotency key")
	}
	if existing, ok := j.byKey[command.IdempotencyKey]; ok {
		if _, err := contract.ClassifyAgentRunCommandReplayV1(existing.Command, command); err != nil {
			return "", ports.CommandJournalEntryV1{}, err
		}
		return ports.CommandJournalReplayV1, copyEntryV1(existing), nil
	}
	entry := ports.CommandJournalEntryV1{Command: command}
	j.byKey[command.IdempotencyKey] = entry
	j.byID[command.CommandID] = command.IdempotencyKey
	return ports.CommandJournalReservedV1, copyEntryV1(entry), nil
}

func (j *JournalV1) RecordReceiptV1(_ context.Context, command contract.AgentRunCommandEnvelopeV1, receipt contract.AgentRunCommandReceiptV1) error {
	if err := receipt.ValidateFor(command); err != nil {
		return err
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	entry, ok := j.byKey[command.IdempotencyKey]
	if !ok {
		return contract.NewError(contract.FaultNotFoundV1, "command_reservation_missing", "command reservation was not found")
	}
	if _, err := contract.ClassifyAgentRunCommandReplayV1(entry.Command, command); err != nil {
		return err
	}
	if entry.Receipt != nil {
		if entry.Receipt.ReceiptDigest != receipt.ReceiptDigest {
			return contract.NewError(contract.FaultRevisionConflictV1, "command_receipt_conflict", "command receipt was already recorded with different content")
		}
		return nil
	}
	copy := receipt
	entry.Receipt = &copy
	j.byKey[command.IdempotencyKey] = entry
	return nil
}

func (j *JournalV1) InspectReservedCommandV1(_ context.Context, command contract.AgentRunCommandEnvelopeV1) (ports.CommandJournalEntryV1, error) {
	if err := command.Validate(); err != nil {
		return ports.CommandJournalEntryV1{}, err
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	entry, ok := j.byKey[command.IdempotencyKey]
	if !ok {
		return ports.CommandJournalEntryV1{}, contract.NewError(contract.FaultNotFoundV1, "original_command_not_found", "original command was not found")
	}
	if _, err := contract.ClassifyAgentRunCommandReplayV1(entry.Command, command); err != nil {
		return ports.CommandJournalEntryV1{}, err
	}
	return copyEntryV1(entry), nil
}

func (j *JournalV1) InspectCommandV1(_ context.Context, request contract.InspectOriginalRequestV1) (ports.CommandJournalEntryV1, error) {
	if err := request.Validate(); err != nil {
		return ports.CommandJournalEntryV1{}, err
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	entry, ok := j.byKey[request.OriginalIdempotencyKey]
	if !ok || request.OriginalCommandRef == nil {
		return ports.CommandJournalEntryV1{}, contract.NewError(contract.FaultNotFoundV1, "original_command_not_found", "original command was not found")
	}
	ref, _ := entry.Command.CommandRefV1()
	if ref != *request.OriginalCommandRef || entry.Command.RequestDigest != request.OriginalRequestDigest {
		return ports.CommandJournalEntryV1{}, contract.NewError(contract.FaultRevisionConflictV1, "original_command_splice", "Inspect Original coordinates do not bind the stored command")
	}
	return copyEntryV1(entry), nil
}

func copyEntryV1(entry ports.CommandJournalEntryV1) ports.CommandJournalEntryV1 {
	if entry.Receipt != nil {
		value := *entry.Receipt
		entry.Receipt = &value
	}
	return entry
}
