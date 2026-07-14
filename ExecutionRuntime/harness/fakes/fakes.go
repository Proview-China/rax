// Package fakes provides deterministic Harness contract doubles.
package fakes

import (
	"context"
	"sync"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type StaticContext struct {
	Snapshot ports.ContextSnapshot
	Err      error
}

func (f *StaticContext) Prepare(_ context.Context, request ports.ContextRequest) (ports.ContextSnapshot, error) {
	if err := request.Validate(); err != nil {
		return ports.ContextSnapshot{}, err
	}
	if f.Err != nil {
		return ports.ContextSnapshot{}, f.Err
	}
	result := f.Snapshot
	result.Payload = contract.CloneOpaque(result.Payload)
	return result, nil
}

type MemoryEvents struct {
	mu            sync.Mutex
	Events        []contract.Event
	FailAfter     int
	Err           error
	LoseNextReply bool
	LostReplyErr  error
}

func (f *MemoryEvents) AppendCandidate(_ context.Context, event contract.Event) error {
	if err := event.Validate(); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, existing := range f.Events {
		if existing.SourceComponentID == event.SourceComponentID && existing.SourceEpoch == event.SourceEpoch && existing.SourceSequence == event.SourceSequence {
			if sameEvent(existing, event) {
				return nil
			}
			return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "event source sequence already binds different content")
		}
	}
	if f.Err != nil && (f.FailAfter == 0 || len(f.Events) >= f.FailAfter) {
		return f.Err
	}
	clone := event
	clone.Payload = contract.CloneOpaque(event.Payload)
	f.Events = append(f.Events, clone)
	if f.LoseNextReply {
		f.LoseNextReply = false
		if f.LostReplyErr != nil {
			return f.LostReplyErr
		}
		return core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected event append reply loss")
	}
	return nil
}

func (f *MemoryEvents) InspectCandidate(ctx context.Context, sourceID string, epoch core.Epoch, sequence uint64) (contract.Event, error) {
	if err := ctx.Err(); err != nil {
		return contract.Event{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, event := range f.Events {
		if event.SourceComponentID == sourceID && event.SourceEpoch == epoch && event.SourceSequence == sequence {
			clone := event
			clone.Payload = contract.CloneOpaque(event.Payload)
			return clone, nil
		}
	}
	return contract.Event{}, core.NewError(core.ErrorNotFound, core.ReasonEvidenceUnavailable, "event candidate does not exist")
}

func (f *MemoryEvents) Snapshot() []contract.Event {
	f.mu.Lock()
	defer f.mu.Unlock()
	result := make([]contract.Event, len(f.Events))
	copy(result, f.Events)
	for index := range result {
		result[index].Payload = contract.CloneOpaque(result[index].Payload)
	}
	return result
}

func sameEvent(left, right contract.Event) bool {
	leftDigest, leftErr := core.DigestJSON(left)
	rightDigest, rightErr := core.DigestJSON(right)
	return leftErr == nil && rightErr == nil && leftDigest == rightDigest
}

type ScriptedModel struct {
	mu      sync.Mutex
	Results []ports.ModelTurnResult
	Err     error
	Calls   []ports.ModelTurnRequest
}

func (f *ScriptedModel) Invoke(_ context.Context, request ports.ModelTurnRequest) (ports.ModelTurnResult, error) {
	if err := request.Validate(request.Intent.PersistedAt.Add(1)); err != nil {
		return ports.ModelTurnResult{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, request)
	if f.Err != nil {
		return ports.ModelTurnResult{}, f.Err
	}
	if len(f.Results) == 0 {
		return ports.ModelTurnResult{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "scripted model has no result")
	}
	result := f.Results[0]
	f.Results = f.Results[1:]
	return result, nil
}

func (f *ScriptedModel) CallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.Calls)
}

type BlockingModel struct {
	Started chan struct{}
	once    sync.Once
}

func (f *BlockingModel) Invoke(ctx context.Context, request ports.ModelTurnRequest) (ports.ModelTurnResult, error) {
	if err := request.Validate(request.Intent.PersistedAt.Add(1)); err != nil {
		return ports.ModelTurnResult{}, err
	}
	f.once.Do(func() { close(f.Started) })
	<-ctx.Done()
	return ports.ModelTurnResult{}, ctx.Err()
}
