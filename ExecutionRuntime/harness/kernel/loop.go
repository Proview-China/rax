// Package kernel implements the provider-neutral Harness interaction loop.
package kernel

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/contract"
	harnessports "github.com/Proview-China/rax/ExecutionRuntime/harness/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const eventPayloadSchema = "praxis.harness.event/v1alpha1"

type Config struct {
	Manifest  contract.Manifest
	Context   harnessports.ContextPort
	Model     harnessports.ModelTurnPort
	Events    harnessports.EventCandidateJournalPort
	Clock     func() time.Time
	MaxEvents uint64
	MaxTurns  uint32
}

type StartRequest struct {
	Run    contract.RunRef            `json:"run"`
	Input  runtimeports.OpaquePayload `json:"input"`
	Intent core.EffectIntent          `json:"intent"`
	Fence  core.ExecutionFence        `json:"fence"`
}

type ProvideActionResultRequest struct {
	Run    contract.RunRef       `json:"run"`
	Result contract.ActionResult `json:"result"`
	Intent core.EffectIntent     `json:"intent"`
	Fence  core.ExecutionFence   `json:"fence"`
}

type ProvideInputRequest struct {
	Run    contract.RunRef            `json:"run"`
	Input  runtimeports.OpaquePayload `json:"input"`
	Intent core.EffectIntent          `json:"intent"`
	Fence  core.ExecutionFence        `json:"fence"`
}

type CancelRequest struct {
	Run    contract.RunRef     `json:"run"`
	Intent core.EffectIntent   `json:"intent"`
	Fence  core.ExecutionFence `json:"fence"`
}

type Loop struct {
	mu       sync.Mutex
	config   Config
	sessions map[string]*session
	active   map[string]string
}

type session struct {
	state      contract.RunState
	input      runtimeports.OpaquePayload
	context    harnessports.ContextSnapshot
	events     []contract.Event
	runContext context.Context
	cancel     context.CancelFunc
	inFlight   bool
	turns      uint32
}

type modelTurnObservation struct {
	Turn                   uint32                 `json:"turn"`
	State                  harnessports.TurnState `json:"state"`
	NativeSessionRef       string                 `json:"native_session_ref"`
	ResultDigest           core.Digest            `json:"result_digest"`
	ProviderEvidenceDigest core.Digest            `json:"provider_evidence_digest"`
}

func New(config Config) (*Loop, error) {
	if config.Clock == nil {
		config.Clock = time.Now
	}
	if config.Context == nil || config.Model == nil || config.Events == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "harness context, model and event candidate ports are required")
	}
	if config.MaxEvents == 0 || config.MaxTurns == 0 {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonPlanInvalid, "harness event and turn limits must be non-zero")
	}
	if err := config.Manifest.Validate(config.Clock()); err != nil {
		return nil, err
	}
	config.Manifest = contract.CloneManifest(config.Manifest)
	return &Loop{config: config, sessions: make(map[string]*session), active: make(map[string]string)}, nil
}

func (l *Loop) Manifest() contract.Manifest { return contract.CloneManifest(l.config.Manifest) }

func (l *Loop) Start(ctx context.Context, request StartRequest) (contract.Snapshot, error) {
	if err := request.Run.Validate(); err != nil {
		return contract.Snapshot{}, err
	}
	if err := contract.ValidateOpaque(request.Input); err != nil {
		return contract.Snapshot{}, err
	}
	if err := validateDispatch(request.Run, request.Intent, request.Fence, l.now()); err != nil {
		return contract.Snapshot{}, err
	}

	l.mu.Lock()
	scopeKey := executionKey(request.Run.Scope)
	runKey := executionRunKey(request.Run)
	if l.active[scopeKey] != "" {
		l.mu.Unlock()
		return contract.Snapshot{}, core.NewError(core.ErrorConflict, core.ReasonInvalidState, "harness already has an active run")
	}
	if _, exists := l.sessions[runKey]; exists {
		l.mu.Unlock()
		return contract.Snapshot{}, core.NewError(core.ErrorConflict, core.ReasonInvalidReference, "harness run id already exists")
	}
	l.mu.Unlock()

	prepared, err := l.config.Context.Prepare(ctx, harnessports.ContextRequest{
		Run: request.Run, ContextPlanDigest: l.config.Manifest.Bootstrap.ContextPlanDigest, Input: contract.CloneOpaque(request.Input),
	})
	if err != nil {
		return contract.Snapshot{}, err
	}
	if err := prepared.Validate(); err != nil {
		return contract.Snapshot{}, err
	}

	runContext, cancel := context.WithCancel(context.Background())
	now := l.now()
	s := &session{
		state: contract.RunState{
			Ref: request.Run, Phase: contract.RunRunning, Revision: 1,
			SessionRef: harnessSessionRef(request.Run), StartedAt: now,
		},
		input: contract.CloneOpaque(request.Input), context: prepared, runContext: runContext, cancel: cancel,
	}
	l.mu.Lock()
	if l.active[scopeKey] != "" {
		l.mu.Unlock()
		cancel()
		return contract.Snapshot{}, core.NewError(core.ErrorConflict, core.ReasonInvalidState, "harness already has an active run")
	}
	l.sessions[runKey] = s
	l.active[scopeKey] = runKey
	if err := l.appendLocked(ctx, s, contract.EventRunStarted, s.state); err != nil {
		delete(l.sessions, runKey)
		delete(l.active, scopeKey)
		l.mu.Unlock()
		cancel()
		return contract.Snapshot{}, err
	}
	l.mu.Unlock()

	return l.invoke(request.Run, nil, request.Intent, request.Fence)
}

func (l *Loop) ProvideActionResult(ctx context.Context, request ProvideActionResultRequest) (contract.Snapshot, error) {
	if err := request.Run.Validate(); err != nil {
		return contract.Snapshot{}, err
	}
	if err := request.Result.Validate(); err != nil {
		return contract.Snapshot{}, err
	}
	l.mu.Lock()
	s, err := l.activeSessionLocked(request.Run)
	if err != nil {
		l.mu.Unlock()
		return contract.Snapshot{}, err
	}
	if s.state.Phase != contract.RunWaitingAction || s.state.PendingAction == nil || s.state.PendingAction.Ref != request.Result.Ref {
		l.mu.Unlock()
		return contract.Snapshot{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "action result does not match the pending action")
	}
	if err := validateDispatch(s.state.Ref, request.Intent, request.Fence, l.now()); err != nil {
		l.mu.Unlock()
		return contract.Snapshot{}, err
	}
	if err := l.appendLocked(ctx, s, contract.EventActionResultReceived, request.Result); err != nil {
		l.mu.Unlock()
		return contract.Snapshot{}, err
	}
	s.state.Phase = contract.RunRunning
	s.state.PendingAction = nil
	s.state.Revision++
	l.mu.Unlock()
	result := request.Result
	return l.invoke(request.Run, &result, request.Intent, request.Fence)
}

func (l *Loop) ProvideInput(ctx context.Context, request ProvideInputRequest) (contract.Snapshot, error) {
	if err := request.Run.Validate(); err != nil {
		return contract.Snapshot{}, err
	}
	if err := contract.ValidateOpaque(request.Input); err != nil {
		return contract.Snapshot{}, err
	}
	l.mu.Lock()
	s, err := l.activeSessionLocked(request.Run)
	if err != nil {
		l.mu.Unlock()
		return contract.Snapshot{}, err
	}
	if s.state.Phase != contract.RunWaitingInput {
		l.mu.Unlock()
		return contract.Snapshot{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "harness run is not waiting for input")
	}
	if err := validateDispatch(s.state.Ref, request.Intent, request.Fence, l.now()); err != nil {
		l.mu.Unlock()
		return contract.Snapshot{}, err
	}
	if err := l.appendLocked(ctx, s, contract.EventInputReceived, request.Input); err != nil {
		l.mu.Unlock()
		return contract.Snapshot{}, err
	}
	s.input = contract.CloneOpaque(request.Input)
	s.state.Phase = contract.RunRunning
	s.state.Revision++
	l.mu.Unlock()
	return l.invoke(request.Run, nil, request.Intent, request.Fence)
}

func (l *Loop) Cancel(ctx context.Context, request CancelRequest) (contract.Snapshot, error) {
	if err := request.Run.Validate(); err != nil {
		return contract.Snapshot{}, err
	}
	if err := validateDispatch(request.Run, request.Intent, request.Fence, l.now()); err != nil {
		return contract.Snapshot{}, err
	}
	l.mu.Lock()
	s, exists := l.sessions[executionRunKey(request.Run)]
	if !exists {
		l.mu.Unlock()
		return contract.Snapshot{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "harness run does not exist")
	}
	if s.state.Phase == contract.RunTerminal {
		snapshot, err := l.snapshotLocked(s)
		l.mu.Unlock()
		return snapshot, err
	}
	if s.state.Phase != contract.RunCancelling {
		if err := l.appendLocked(ctx, s, contract.EventCancelRequested, map[string]string{"reason": "runtime_control"}); err != nil {
			l.mu.Unlock()
			return contract.Snapshot{}, err
		}
		s.state.Phase = contract.RunCancelling
		s.state.PendingAction = nil
		s.state.Revision++
		s.cancel()
	}
	if !s.inFlight {
		if err := l.finishLocked(ctx, s, contract.ClaimCancelled, contract.EventRunCancelled, map[string]string{"result": "cancelled"}); err != nil {
			l.mu.Unlock()
			return contract.Snapshot{}, err
		}
	}
	snapshot, err := l.snapshotLocked(s)
	l.mu.Unlock()
	return snapshot, err
}

func (l *Loop) Inspect(run contract.RunRef) (contract.Snapshot, error) {
	if err := run.Validate(); err != nil {
		return contract.Snapshot{}, err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	s, exists := l.sessions[executionRunKey(run)]
	if !exists {
		return contract.Snapshot{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "harness run does not exist")
	}
	return l.snapshotLocked(s)
}

func (l *Loop) Events(run contract.RunRef) ([]contract.Event, error) {
	if err := run.Validate(); err != nil {
		return nil, err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	s, exists := l.sessions[executionRunKey(run)]
	if !exists {
		return nil, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "harness run does not exist")
	}
	events := make([]contract.Event, len(s.events))
	for index := range s.events {
		events[index] = cloneEvent(s.events[index])
	}
	return events, nil
}

func (l *Loop) ActiveRun(scope core.ExecutionScope) core.AgentRunID {
	l.mu.Lock()
	defer l.mu.Unlock()
	key := l.active[executionKey(scope)]
	if key == "" || l.sessions[key] == nil {
		return ""
	}
	return l.sessions[key].state.Ref.RunID
}

func (l *Loop) invoke(run contract.RunRef, actionResult *contract.ActionResult, intent core.EffectIntent, fence core.ExecutionFence) (contract.Snapshot, error) {
	l.mu.Lock()
	s, err := l.activeSessionLocked(run)
	if err != nil {
		l.mu.Unlock()
		return contract.Snapshot{}, err
	}
	if s.state.Phase != contract.RunRunning || s.inFlight {
		l.mu.Unlock()
		return contract.Snapshot{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "harness run cannot start another model turn")
	}
	if s.turns >= l.config.MaxTurns {
		err := l.finishLocked(context.Background(), s, contract.ClaimFailed, contract.EventRunFailed, map[string]string{"reason": "turn_limit"})
		snapshot, snapshotErr := l.snapshotLocked(s)
		l.mu.Unlock()
		if err != nil {
			if snapshotErr != nil {
				return contract.Snapshot{}, snapshotErr
			}
			return snapshot, err
		}
		return snapshot, snapshotErr
	}
	request := harnessports.ModelTurnRequest{
		Run: s.state.Ref, Input: contract.CloneOpaque(s.input), Context: s.context,
		ActionResult: actionResult, Intent: intent, Fence: fence,
	}
	if err := request.Validate(l.now()); err != nil {
		l.mu.Unlock()
		return contract.Snapshot{}, err
	}
	if err := l.appendLocked(context.Background(), s, contract.EventModelTurnStarted, map[string]uint32{"turn": s.turns + 1}); err != nil {
		l.mu.Unlock()
		return contract.Snapshot{}, err
	}
	s.turns++
	s.inFlight = true
	runContext := s.runContext
	l.mu.Unlock()

	result, invokeErr := l.config.Model.Invoke(runContext, request)

	l.mu.Lock()
	s.inFlight = false
	if s.state.Phase == contract.RunCancelling {
		err = l.finishLocked(context.Background(), s, contract.ClaimCancelled, contract.EventRunCancelled, map[string]string{"result": "cancelled"})
		snapshot, snapshotErr := l.snapshotLocked(s)
		l.mu.Unlock()
		if err != nil {
			if snapshotErr != nil {
				return contract.Snapshot{}, snapshotErr
			}
			return snapshot, err
		}
		return snapshot, snapshotErr
	}
	if invokeErr != nil {
		// Once the model execution surface has been entered, an error may be a
		// lost reply. Harness cannot upgrade it to an authoritative failure or
		// dispatch the same Effect again; it stays reconcilable until an
		// independent Effect/Execution inspector settles the attempt.
		s.state.Phase = contract.RunReconciling
		s.state.PendingAction = nil
		s.state.Revision++
		err = l.appendLocked(context.Background(), s, contract.EventModelTurnUncertain, map[string]string{"reason": "model_turn_outcome_unknown"})
		snapshot, snapshotErr := l.snapshotLocked(s)
		l.mu.Unlock()
		if err != nil {
			if snapshotErr != nil {
				return contract.Snapshot{}, snapshotErr
			}
			return snapshot, err
		}
		if snapshotErr != nil {
			return contract.Snapshot{}, snapshotErr
		}
		return snapshot, invokeErr
	}
	if err := result.Validate(); err != nil {
		finishErr := l.finishLocked(context.Background(), s, contract.ClaimFailed, contract.EventRunFailed, map[string]string{"reason": "invalid_model_result"})
		if finishErr != nil {
			l.markReconcilingLocked(s)
		}
		snapshot, snapshotErr := l.snapshotLocked(s)
		l.mu.Unlock()
		if finishErr != nil {
			if snapshotErr != nil {
				return contract.Snapshot{}, snapshotErr
			}
			return snapshot, finishErr
		}
		return contract.Snapshot{}, err
	}
	resultDigest, err := core.DigestJSON(result)
	if err != nil {
		l.mu.Unlock()
		return contract.Snapshot{}, err
	}
	if err := l.appendLocked(context.Background(), s, contract.EventModelTurnObserved, modelTurnObservation{
		Turn: s.turns, State: result.State, NativeSessionRef: result.NativeSessionRef,
		ResultDigest: resultDigest, ProviderEvidenceDigest: result.EvidenceDigest,
	}); err != nil {
		l.markReconcilingLocked(s)
		snapshot, snapshotErr := l.snapshotLocked(s)
		l.mu.Unlock()
		if snapshotErr != nil {
			return contract.Snapshot{}, snapshotErr
		}
		return snapshot, err
	}
	s.state.Revision++
	switch result.State {
	case harnessports.TurnCompleted:
		if err := l.appendLocked(context.Background(), s, contract.EventModelOutput, *result.Output); err != nil {
			l.markReconcilingLocked(s)
			snapshot, snapshotErr := l.snapshotLocked(s)
			l.mu.Unlock()
			if snapshotErr != nil {
				return contract.Snapshot{}, snapshotErr
			}
			return snapshot, err
		}
		err = l.finishLocked(context.Background(), s, contract.ClaimCompleted, contract.EventRunCompleted, map[string]string{"result": "completed"})
	case harnessports.TurnActionRequired:
		action := *result.Action
		action.Payload = contract.CloneOpaque(action.Payload)
		if err := l.appendLocked(context.Background(), s, contract.EventActionRequested, action); err != nil {
			l.markReconcilingLocked(s)
			snapshot, snapshotErr := l.snapshotLocked(s)
			l.mu.Unlock()
			if snapshotErr != nil {
				return contract.Snapshot{}, snapshotErr
			}
			return snapshot, err
		}
		s.state.Phase = contract.RunWaitingAction
		s.state.PendingAction = &action
	case harnessports.TurnInputRequired:
		if err := l.appendLocked(context.Background(), s, contract.EventInputRequested, map[string]string{"result": "input_required"}); err != nil {
			l.markReconcilingLocked(s)
			snapshot, snapshotErr := l.snapshotLocked(s)
			l.mu.Unlock()
			if snapshotErr != nil {
				return contract.Snapshot{}, snapshotErr
			}
			return snapshot, err
		}
		s.state.Phase = contract.RunWaitingInput
	}
	if err != nil {
		l.markReconcilingLocked(s)
		snapshot, snapshotErr := l.snapshotLocked(s)
		l.mu.Unlock()
		if snapshotErr != nil {
			return contract.Snapshot{}, snapshotErr
		}
		return snapshot, err
	}
	snapshot, err := l.snapshotLocked(s)
	l.mu.Unlock()
	return snapshot, err
}

func (l *Loop) markReconcilingLocked(s *session) {
	if s.state.Phase == contract.RunTerminal || s.state.Phase == contract.RunReconciling {
		return
	}
	s.state.Phase = contract.RunReconciling
	s.state.PendingAction = nil
	s.state.Revision++
}

func (l *Loop) finishLocked(ctx context.Context, s *session, claim contract.CompletionClaim, kind contract.EventKind, payload any) error {
	if s.state.Phase == contract.RunTerminal {
		return nil
	}
	// Persist the source observation before committing the terminal state. A
	// rejected event candidate must leave the run recoverable and non-terminal.
	if err := l.appendLocked(ctx, s, kind, payload); err != nil {
		return err
	}
	s.state.Phase = contract.RunTerminal
	s.state.PendingAction = nil
	s.state.CompletionClaim = claim
	s.state.EndedAt = l.now()
	s.state.Revision++
	key := executionKey(s.state.Ref.Scope)
	if l.active[key] == executionRunKey(s.state.Ref) {
		delete(l.active, key)
	}
	s.cancel()
	return nil
}

func (l *Loop) appendLocked(ctx context.Context, s *session, kind contract.EventKind, value any) error {
	if s.state.SourceSequence >= l.config.MaxEvents {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonPlanInvalid, "harness event limit reached")
	}
	payloadBytes, err := json.Marshal(value)
	if err != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidDigest, "harness event payload cannot be serialized")
	}
	digest, err := core.DigestJSON(value)
	if err != nil {
		return err
	}
	event := contract.Event{
		SourceComponentID: eventSourceID(l.config.Manifest.ID, s.state.Ref), SourceEpoch: s.state.Ref.Scope.Instance.Epoch,
		SourceSequence: s.state.SourceSequence + 1, RunID: s.state.Ref.RunID, Kind: kind,
		Payload: runtimeports.OpaquePayload{Schema: eventPayloadSchema, Digest: digest, Payload: payloadBytes}, ObservedAt: l.now(),
	}
	if err := event.Validate(); err != nil {
		return err
	}
	if err := l.config.Events.AppendCandidate(ctx, event); err != nil {
		// Transport libraries do not necessarily translate a lost reply into a
		// Runtime DomainError. Inspect every failed append by its exact source
		// identity before deciding it was not committed. A mismatched record is
		// never accepted, including conflicts and native timeouts.
		// Recovery inspection is a new read operation. It must not inherit a
		// caller cancellation that may itself be the reason the append reply was
		// lost. No timeout is invented here; the injected journal/provider owns
		// its explicit read policy.
		inspectCtx := context.WithoutCancel(ctx)
		persisted, inspectErr := l.config.Events.InspectCandidate(inspectCtx, event.SourceComponentID, event.SourceEpoch, event.SourceSequence)
		if inspectErr != nil || !sameEventCandidate(event, persisted) {
			return err
		}
	}
	s.state.SourceSequence = event.SourceSequence
	s.events = append(s.events, cloneEvent(event))
	return nil
}

func (l *Loop) activeSessionLocked(run contract.RunRef) (*session, error) {
	key := executionRunKey(run)
	s, exists := l.sessions[key]
	if !exists {
		return nil, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "harness run does not exist")
	}
	if l.active[executionKey(s.state.Ref.Scope)] != key || s.state.Phase == contract.RunTerminal {
		return nil, core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "harness run is not active")
	}
	return s, nil
}

func (l *Loop) snapshotLocked(s *session) (contract.Snapshot, error) {
	eventsDigest, err := core.DigestJSON(s.events)
	if err != nil {
		return contract.Snapshot{}, err
	}
	snapshot := contract.Snapshot{State: cloneState(s.state), EventsDigest: eventsDigest, CapturedAt: l.now()}
	if err := snapshot.Validate(); err != nil {
		return contract.Snapshot{}, err
	}
	return snapshot, nil
}

func (l *Loop) now() time.Time { return l.config.Clock().UTC() }

func validateDispatch(run contract.RunRef, intent core.EffectIntent, fence core.ExecutionFence, now time.Time) error {
	return core.ValidateEffectDispatch(intent, fence, core.CurrentFenceFacts{
		Scope: run.Scope, CapabilityGrantDigest: fence.CapabilityGrantDigest,
	}, now)
}

func cloneState(state contract.RunState) contract.RunState {
	clone := state
	if state.PendingAction != nil {
		action := *state.PendingAction
		action.Payload = contract.CloneOpaque(action.Payload)
		clone.PendingAction = &action
	}
	return clone
}

func cloneEvent(event contract.Event) contract.Event {
	clone := event
	clone.Payload = contract.CloneOpaque(event.Payload)
	return clone
}

func executionKey(scope core.ExecutionScope) string {
	digest, err := core.DigestJSON(scope)
	if err != nil {
		// ExecutionScope contains only JSON-safe value fields. Keep a unique
		// fallback for defensive completeness without collapsing partitions.
		return fmt.Sprintf("invalid-scope:%#v", scope)
	}
	return string(digest)
}

func executionRunKey(run contract.RunRef) string {
	return executionKey(run.Scope) + "/" + string(run.RunID)
}

func eventSourceID(componentID string, run contract.RunRef) string {
	digest, err := core.DigestJSON(struct {
		ComponentID string          `json:"component_id"`
		Run         contract.RunRef `json:"run"`
	}{ComponentID: componentID, Run: run})
	if err != nil {
		return fmt.Sprintf("%s:%#v", componentID, run)
	}
	return "harness:" + string(digest)
}

func harnessSessionRef(run contract.RunRef) string {
	digest, err := core.DigestJSON(struct {
		Domain string          `json:"domain"`
		Run    contract.RunRef `json:"run"`
	}{Domain: "praxis.harness.session/v1", Run: run})
	if err != nil {
		return fmt.Sprintf("harness-session:%#v", run)
	}
	return "harness-session:" + string(digest)
}

func sameEventCandidate(left, right contract.Event) bool {
	leftDigest, leftErr := core.DigestJSON(left)
	rightDigest, rightErr := core.DigestJSON(right)
	return leftErr == nil && rightErr == nil && leftDigest == rightDigest
}
