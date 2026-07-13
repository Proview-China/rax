package execution

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/union"
)

const (
	EventKindRouteTerminalCandidate = "route_terminal_candidate"

	ControlApprovalRequested      = "approval_requested"
	ControlApprovalResolved       = "approval_resolved"
	ControlApprovalExpired        = "approval_expired"
	ControlApprovalInvalidated    = "approval_invalidated"
	ControlCancelRequested        = "cancel_requested"
	ControlCancelDispatched       = "cancel_dispatched"
	ControlCancelAcknowledged     = "cancel_acknowledged"
	ControlCancellationQuiesced   = "cancellation_quiesced"
	ControlReconciliationStarted  = "effect_reconciliation_started"
	ControlReconciliationComplete = "effect_reconciliation_completed"
	ControlBackgroundWorkUpdated  = "background_work_updated"
	ControlBackgroundDrained      = "background_drained"
	ControlCommandDispatched      = "command_dispatched"
)

type ApprovalStatus string

const (
	ApprovalPending     ApprovalStatus = "pending"
	ApprovalResolved    ApprovalStatus = "resolved"
	ApprovalExpired     ApprovalStatus = "expired"
	ApprovalInvalidated ApprovalStatus = "invalidated"
)

type ApprovalState struct {
	ApprovalID         union.ApprovalID
	ActionID           union.ActionID
	MechanismAttemptID union.MechanismAttemptID
	InputDigest        string
	ActionRevision     uint64
	ExpiresAt          time.Time
	Status             ApprovalStatus
	Decision           string
}

type CancellationPhase string

const (
	CancellationNone         CancellationPhase = "none"
	CancellationRequested    CancellationPhase = "requested"
	CancellationDispatched   CancellationPhase = "dispatched"
	CancellationAcknowledged CancellationPhase = "acknowledged"
	CancellationQuiesced     CancellationPhase = "quiesced"
	CancellationReconciling  CancellationPhase = "reconciling"
	CancellationReconciled   CancellationPhase = "reconciled"
)

type ReconciliationSummary struct {
	SideEffectState union.SideEffectState `json:"side_effect_state"`
	Quiesced        bool                  `json:"quiesced"`
}

type ReconciliationPhase string

const (
	ReconciliationNone      ReconciliationPhase = "none"
	ReconciliationStarted   ReconciliationPhase = "started"
	ReconciliationCompleted ReconciliationPhase = "completed"
)

type commandReceipt struct {
	Digest string `json:"command_digest"`
}

type LedgerState struct {
	ExecutionID            union.ExecutionID
	LastSequence           uint64
	LastTimestamp          time.Time
	Terminal               bool
	TerminalEventID        union.EventID
	TerminalStatus         union.ExecutionStatus
	RouteTerminalObserved  bool
	RouteTerminalCandidate RouteTerminalCandidate
	PendingBackgroundWork  int
	Cancellation           CancellationPhase
	Reconciliation         ReconciliationPhase
	Approvals              map[union.ApprovalID]ApprovalState
	AcceptedIntents        map[union.IntentID]bool
	MechanismPlans         map[union.MechanismPlanID]union.IntentID
	SelectedMechanisms     map[union.MechanismPlanID]bool
	Attempts               map[union.MechanismAttemptID]union.AttemptStatus
	AttemptPlans           map[union.MechanismAttemptID]union.MechanismPlanID
	Items                  map[union.ItemID]union.ExecutionItem
	UncertainAttempts      map[union.MechanismAttemptID]bool
	SyntheticAttempts      map[union.MechanismAttemptID]bool
	SyntheticActions       map[union.ActionID]bool
	EffectIDs              map[union.EffectID]bool
	VerificationIDs        map[union.VerificationID]bool
	CommandDigests         map[string]string
}

func newLedgerState(executionID union.ExecutionID) LedgerState {
	return LedgerState{
		ExecutionID: executionID, Cancellation: CancellationNone, Reconciliation: ReconciliationNone,
		Approvals: make(map[union.ApprovalID]ApprovalState), AcceptedIntents: make(map[union.IntentID]bool),
		MechanismPlans: make(map[union.MechanismPlanID]union.IntentID), Attempts: make(map[union.MechanismAttemptID]union.AttemptStatus),
		SelectedMechanisms: make(map[union.MechanismPlanID]bool),
		AttemptPlans:       make(map[union.MechanismAttemptID]union.MechanismPlanID), Items: make(map[union.ItemID]union.ExecutionItem),
		UncertainAttempts: make(map[union.MechanismAttemptID]bool), SyntheticAttempts: make(map[union.MechanismAttemptID]bool),
		SyntheticActions: make(map[union.ActionID]bool), CommandDigests: make(map[string]string),
		EffectIDs: make(map[union.EffectID]bool), VerificationIDs: make(map[union.VerificationID]bool),
	}
}

func (state LedgerState) Clone() LedgerState {
	clone := state
	clone.Approvals = cloneMap(state.Approvals)
	clone.AcceptedIntents = cloneMap(state.AcceptedIntents)
	clone.MechanismPlans = cloneMap(state.MechanismPlans)
	clone.SelectedMechanisms = cloneMap(state.SelectedMechanisms)
	clone.Attempts = cloneMap(state.Attempts)
	clone.AttemptPlans = cloneMap(state.AttemptPlans)
	clone.Items = cloneMap(state.Items)
	clone.UncertainAttempts = cloneMap(state.UncertainAttempts)
	clone.SyntheticAttempts = cloneMap(state.SyntheticAttempts)
	clone.SyntheticActions = cloneMap(state.SyntheticActions)
	clone.EffectIDs = cloneMap(state.EffectIDs)
	clone.VerificationIDs = cloneMap(state.VerificationIDs)
	clone.CommandDigests = cloneMap(state.CommandDigests)
	return clone
}

func cloneMap[K comparable, V any](source map[K]V) map[K]V {
	clone := make(map[K]V, len(source))
	for key, value := range source {
		clone[key] = value
	}
	return clone
}

func (state LedgerState) HasUncertainSideEffects() bool {
	return len(state.UncertainAttempts) != 0
}

func (state LedgerState) ExecutionStatus() string {
	if state.Terminal {
		return string(state.TerminalStatus)
	}
	if state.Cancellation != CancellationNone {
		return "cancelling"
	}
	return "running"
}

type CommandDisposition struct {
	Duplicate       bool
	ApprovalExpired bool
	Digest          string
}

type EventLedger struct {
	mu        sync.RWMutex
	state     LedgerState
	events    []union.UnifiedExecutionEvent
	eventIDs  map[union.EventID]struct{}
	authority ledgerAuthority
}

type ledgerAuthority struct {
	bound      bool
	intents    map[union.IntentID]struct{}
	mechanisms map[union.MechanismPlanID]union.MechanismPlan
}

func NewEventLedger(executionID union.ExecutionID) (*EventLedger, error) {
	if strings.TrimSpace(string(executionID)) == "" {
		return nil, fmt.Errorf("%w: execution identity is required", ErrLedgerInvariant)
	}
	return &EventLedger{state: newLedgerState(executionID), eventIDs: make(map[union.EventID]struct{})}, nil
}

// NewEventLedgerForPlan binds the ledger to the sealed prepared semantic
// surface. Runtime and projection use this constructor so Adapter candidates
// cannot introduce accepted intents or mechanism plans outside that surface.
func NewEventLedgerForPlan(plan union.PreparedExecutionPlan) (*EventLedger, error) {
	if err := plan.Validate(); err != nil {
		return nil, fmt.Errorf("%w: prepared plan: %v", ErrLedgerInvariant, err)
	}
	clone, err := plan.Clone()
	if err != nil {
		return nil, fmt.Errorf("%w: clone prepared plan", ErrLedgerInvariant)
	}
	ledger, err := NewEventLedger(clone.ExecutionID)
	if err != nil {
		return nil, err
	}
	ledger.authority = ledgerAuthority{
		bound:      true,
		intents:    make(map[union.IntentID]struct{}, len(clone.IntentGraph.Nodes)),
		mechanisms: make(map[union.MechanismPlanID]union.MechanismPlan, len(clone.Mechanisms)),
	}
	for _, intent := range clone.IntentGraph.Nodes {
		ledger.authority.intents[intent.ID] = struct{}{}
	}
	for _, mechanism := range clone.Mechanisms {
		ledger.authority.mechanisms[mechanism.ID] = mechanism
	}
	return ledger, nil
}

func Replay(executionID union.ExecutionID, events []union.UnifiedExecutionEvent) (*EventLedger, error) {
	ledger, err := NewEventLedger(executionID)
	if err != nil {
		return nil, err
	}
	for _, event := range events {
		if err := ledger.Append(event); err != nil {
			return nil, err
		}
	}
	return ledger, nil
}

// ReplayForPlan replays a Runtime stream against the same sealed authority
// boundary used during live execution.
func ReplayForPlan(plan union.PreparedExecutionPlan, events []union.UnifiedExecutionEvent) (*EventLedger, error) {
	ledger, err := NewEventLedgerForPlan(plan)
	if err != nil {
		return nil, err
	}
	for _, event := range events {
		if err := ledger.Append(event); err != nil {
			return nil, err
		}
	}
	return ledger, nil
}

func (ledger *EventLedger) NextSequence() uint64 {
	if ledger == nil {
		return 0
	}
	ledger.mu.RLock()
	sequence := ledger.state.LastSequence + 1
	ledger.mu.RUnlock()
	return sequence
}

func (ledger *EventLedger) State() LedgerState {
	if ledger == nil {
		return LedgerState{}
	}
	ledger.mu.RLock()
	state := ledger.state.Clone()
	ledger.mu.RUnlock()
	return state
}

func (ledger *EventLedger) Events() []union.UnifiedExecutionEvent {
	if ledger == nil {
		return nil
	}
	ledger.mu.RLock()
	events := make([]union.UnifiedExecutionEvent, 0, len(ledger.events))
	for _, event := range ledger.events {
		clone, err := event.Clone()
		if err != nil {
			ledger.mu.RUnlock()
			return nil
		}
		events = append(events, clone)
	}
	ledger.mu.RUnlock()
	return events
}

func (ledger *EventLedger) Append(event union.UnifiedExecutionEvent) error {
	if ledger == nil {
		return ErrLedgerInvariant
	}
	clone, err := event.Clone()
	if err != nil {
		return fmt.Errorf("%w: event clone failed", ErrLedgerInvariant)
	}
	if err := clone.Validate(); err != nil {
		return fmt.Errorf("%w: event validation failed: %v", ErrLedgerInvariant, err)
	}
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	if ledger.state.Terminal {
		return ErrTerminal
	}
	if clone.Header.ExecutionID != ledger.state.ExecutionID {
		return fmt.Errorf("%w: execution identity differs", ErrLedgerInvariant)
	}
	if clone.Header.Sequence != ledger.state.LastSequence+1 {
		return ErrSequence
	}
	if _, duplicate := ledger.eventIDs[clone.Header.EventID]; duplicate {
		return fmt.Errorf("%w: event identity is duplicated", ErrLedgerInvariant)
	}
	if !ledger.state.LastTimestamp.IsZero() && clone.Header.Timestamp.Before(ledger.state.LastTimestamp) {
		return fmt.Errorf("%w: event time moved backwards", ErrLedgerInvariant)
	}
	if err := ledger.authorize(clone); err != nil {
		return err
	}
	next := ledger.state.Clone()
	if err := applyEvent(&next, clone); err != nil {
		return err
	}
	next.LastSequence = clone.Header.Sequence
	next.LastTimestamp = clone.Header.Timestamp
	ledger.events = append(ledger.events, clone)
	ledger.eventIDs[clone.Header.EventID] = struct{}{}
	ledger.state = next
	return nil
}

func (ledger *EventLedger) authorize(event union.UnifiedExecutionEvent) error {
	if event.Header.CausationID != "" {
		if _, exists := ledger.eventIDs[event.Header.CausationID]; !exists {
			return fmt.Errorf("%w: event causation must reference an earlier committed event", ErrLedgerInvariant)
		}
	}
	authority := ledger.authority
	if !authority.bound {
		return nil
	}
	if event.Header.IntentID != "" {
		if _, exists := authority.intents[event.Header.IntentID]; !exists {
			return fmt.Errorf("%w: event intent is outside the prepared plan", ErrLedgerInvariant)
		}
	}
	if event.Header.MechanismPlanID != "" {
		mechanism, exists := authority.mechanisms[event.Header.MechanismPlanID]
		if !exists {
			return fmt.Errorf("%w: event mechanism is outside the prepared plan", ErrLedgerInvariant)
		}
		if event.Header.IntentID != "" && event.Header.IntentID != mechanism.IntentID {
			return fmt.Errorf("%w: event mechanism belongs to a different prepared intent", ErrLedgerInvariant)
		}
	}
	if event.Header.MechanismAttemptID != "" {
		attemptPlanID, observed := ledger.state.AttemptPlans[event.Header.MechanismAttemptID]
		if observed {
			if event.Header.MechanismPlanID != "" && event.Header.MechanismPlanID != attemptPlanID {
				return fmt.Errorf("%w: event attempt belongs to a different mechanism plan", ErrLedgerInvariant)
			}
			mechanism, exists := authority.mechanisms[attemptPlanID]
			if !exists {
				return fmt.Errorf("%w: event attempt refers to an unprepared mechanism plan", ErrLedgerInvariant)
			}
			if event.Header.IntentID != "" && event.Header.IntentID != mechanism.IntentID {
				return fmt.Errorf("%w: event attempt belongs to a different prepared intent", ErrLedgerInvariant)
			}
		} else if event.Mechanism == nil || event.Mechanism.Attempt == nil || event.Mechanism.Attempt.ID != event.Header.MechanismAttemptID {
			return fmt.Errorf("%w: event references an unobserved mechanism attempt", ErrLedgerInvariant)
		}
	}
	if event.Intent != nil && event.Intent.Kind == "accepted" && event.Header.Origin != union.EventOriginPraxis {
		return ErrAdapterAuthority
	}
	if event.Mechanism != nil && event.Mechanism.Plan != nil {
		expected, exists := authority.mechanisms[event.Mechanism.Plan.ID]
		if !exists || !reflect.DeepEqual(expected, *event.Mechanism.Plan) {
			return fmt.Errorf("%w: mechanism plan differs from the sealed prepared plan", ErrLedgerInvariant)
		}
		if event.Mechanism.Kind == "planned" && event.Header.Origin != union.EventOriginPraxis {
			return ErrAdapterAuthority
		}
	}
	return nil
}

func applyEvent(state *LedgerState, event union.UnifiedExecutionEvent) error {
	if event.Effect != nil {
		if event.Header.Origin != union.EventOriginPraxis {
			return ErrAdapterAuthority
		}
		if event.Effect.Effect != nil {
			if state.EffectIDs[event.Effect.Effect.ID] {
				return fmt.Errorf("%w: Effect identity is duplicated", ErrLedgerInvariant)
			}
			attemptID := event.Effect.Effect.MechanismAttemptID
			if _, exists := state.Attempts[attemptID]; !exists {
				return fmt.Errorf("%w: Effect requires an observed mechanism attempt", ErrLedgerInvariant)
			}
			planID := state.AttemptPlans[attemptID]
			planIntentID := state.MechanismPlans[planID]
			if planIntentID == "" || !containsIntentID(event.Effect.Effect.IntentIDs, planIntentID) {
				return fmt.Errorf("%w: Effect must include the mechanism plan intent", ErrLedgerInvariant)
			}
			for _, intentID := range event.Effect.Effect.IntentIDs {
				if !state.AcceptedIntents[intentID] {
					return fmt.Errorf("%w: Effect references an unaccepted intent", ErrLedgerInvariant)
				}
			}
			for _, superseded := range event.Effect.Effect.SupersedesEffectIDs {
				if !state.EffectIDs[superseded] {
					return fmt.Errorf("%w: Effect supersession must reference an earlier Effect", ErrLedgerInvariant)
				}
			}
			if state.SyntheticAttempts[attemptID] || state.SyntheticActions[event.Header.ActionID] {
				return fmt.Errorf("%w: synthetic execution cannot produce an Effect", ErrLedgerInvariant)
			}
			state.EffectIDs[event.Effect.Effect.ID] = true
		}
		if event.Effect.Verification != nil {
			verification := event.Effect.Verification
			if state.VerificationIDs[verification.ID] {
				return fmt.Errorf("%w: verification identity is duplicated", ErrLedgerInvariant)
			}
			for _, effectID := range verification.EffectIDs {
				if !state.EffectIDs[effectID] {
					return fmt.Errorf("%w: verification references an unknown Effect", ErrLedgerInvariant)
				}
			}
			for _, intentID := range verification.IntentIDs {
				if !state.AcceptedIntents[intentID] {
					return fmt.Errorf("%w: verification references an unaccepted intent", ErrLedgerInvariant)
				}
			}
			state.VerificationIDs[verification.ID] = true
		}
	}
	if event.Intent != nil && event.Intent.Kind == "accepted" {
		intentID := event.Header.IntentID
		if intentID == "" {
			return fmt.Errorf("%w: accepted intent identity is required", ErrLedgerInvariant)
		}
		state.AcceptedIntents[intentID] = true
	}
	if event.Mechanism != nil {
		if event.Mechanism.Plan != nil {
			plan := event.Mechanism.Plan
			_, wasPlanned := state.MechanismPlans[plan.ID]
			if !state.AcceptedIntents[plan.IntentID] {
				return fmt.Errorf("%w: mechanism requires an accepted intent", ErrLedgerInvariant)
			}
			if intentID, exists := state.MechanismPlans[plan.ID]; exists && intentID != plan.IntentID {
				return fmt.Errorf("%w: mechanism identity changed intent", ErrLedgerInvariant)
			}
			state.MechanismPlans[plan.ID] = plan.IntentID
			if event.Mechanism.Kind == "selected" {
				if !wasPlanned {
					return fmt.Errorf("%w: mechanism selection requires a prior plan", ErrLedgerInvariant)
				}
				state.SelectedMechanisms[plan.ID] = true
			}
		}
		if event.Mechanism.Attempt != nil {
			attempt := event.Mechanism.Attempt
			if _, exists := state.MechanismPlans[attempt.MechanismPlanID]; !exists {
				return fmt.Errorf("%w: attempt requires a planned mechanism", ErrLedgerInvariant)
			}
			if !state.SelectedMechanisms[attempt.MechanismPlanID] {
				return fmt.Errorf("%w: attempt requires a selected mechanism", ErrLedgerInvariant)
			}
			if previousPlan, exists := state.AttemptPlans[attempt.ID]; exists && previousPlan != attempt.MechanismPlanID {
				return fmt.Errorf("%w: attempt identity changed mechanism plan", ErrLedgerInvariant)
			}
			if previous, exists := state.Attempts[attempt.ID]; exists && !validAttemptTransition(previous, attempt.Status) {
				return fmt.Errorf("%w: mechanism attempt status regressed from %s to %s", ErrLedgerInvariant, previous, attempt.Status)
			}
			if attempt.RetryOf != "" {
				previous, exists := state.Attempts[attempt.RetryOf]
				if !exists || !terminalAttemptStatus(previous) {
					return fmt.Errorf("%w: retry requires a terminal prior attempt", ErrLedgerInvariant)
				}
			}
			state.AttemptPlans[attempt.ID] = attempt.MechanismPlanID
			state.Attempts[attempt.ID] = attempt.Status
			switch attempt.SideEffectState {
			case union.SideEffectPossible, union.SideEffectUnknown:
				state.UncertainAttempts[attempt.ID] = true
			case union.SideEffectNone, union.SideEffectObserved, union.SideEffectReconciled:
				delete(state.UncertainAttempts, attempt.ID)
			}
		}
	}
	if event.Item != nil {
		item := event.Item.Item
		if item.AttemptID != "" {
			if _, exists := state.Attempts[item.AttemptID]; !exists {
				return fmt.Errorf("%w: execution item requires an observed mechanism attempt", ErrLedgerInvariant)
			}
		}
		if previous, exists := state.Items[item.ID]; exists {
			if previous.ActionID != "" && item.ActionID != previous.ActionID {
				return fmt.Errorf("%w: execution item changed action identity", ErrLedgerInvariant)
			}
			if previous.AttemptID != "" && item.AttemptID != previous.AttemptID {
				return fmt.Errorf("%w: execution item changed attempt identity", ErrLedgerInvariant)
			}
			if !validItemTransition(previous.Status, item.Status) {
				return fmt.Errorf("%w: execution item status regressed from %s to %s", ErrLedgerInvariant, previous.Status, item.Status)
			}
			if !validSideEffectTransition(previous.SideEffectState, item.SideEffectState) {
				return fmt.Errorf("%w: execution item side-effect state regressed", ErrLedgerInvariant)
			}
		}
		state.Items[item.ID] = item
	}
	if event.Model != nil && event.Model.Executed != nil && !*event.Model.Executed && event.Model.SyntheticReason != "" {
		if event.Header.MechanismAttemptID != "" {
			state.SyntheticAttempts[event.Header.MechanismAttemptID] = true
		}
		if event.Header.ActionID != "" {
			state.SyntheticActions[event.Header.ActionID] = true
		}
	}
	if event.Control != nil {
		if err := applyControl(state, event); err != nil {
			return err
		}
	}
	if event.Diagnostic != nil && event.Diagnostic.Kind == EventKindRouteTerminalCandidate {
		var candidate RouteTerminalCandidate
		if len(event.Diagnostic.Payload) == 0 || json.Unmarshal(event.Diagnostic.Payload, &candidate) != nil {
			return fmt.Errorf("%w: route terminal candidate is invalid", ErrLedgerInvariant)
		}
		if candidate.PendingBackgroundWork < 0 {
			return fmt.Errorf("%w: pending background work is negative", ErrLedgerInvariant)
		}
		if err := validateCandidateStatus(candidate.Status); err != nil {
			return err
		}
		if err := validateSideEffectState(candidate.SideEffectState); err != nil {
			return err
		}
		if state.RouteTerminalObserved && event.Header.Origin != union.EventOriginPraxis {
			return fmt.Errorf("%w: adapter emitted more than one route terminal candidate", ErrLedgerInvariant)
		}
		state.RouteTerminalObserved = true
		state.RouteTerminalCandidate = candidate
		state.PendingBackgroundWork = candidate.PendingBackgroundWork
		if candidate.SideEffectState == union.SideEffectPossible || candidate.SideEffectState == union.SideEffectUnknown {
			state.UncertainAttempts[""] = true
		}
	}
	if event.Lifecycle != nil && event.Lifecycle.Status != "" {
		if event.Header.Origin != union.EventOriginPraxis {
			return ErrAdapterAuthority
		}
		if !state.RouteTerminalObserved {
			return fmt.Errorf("%w: unified terminal requires a route terminal candidate", ErrLedgerInvariant)
		}
		if event.Lifecycle.PendingBackgroundWork != 0 || state.PendingBackgroundWork != 0 {
			return fmt.Errorf("%w: unified terminal requires background drain", ErrLedgerInvariant)
		}
		if state.Cancellation != CancellationNone {
			if event.Lifecycle.Status != union.ExecutionStatusCancelled && event.Lifecycle.Status != union.ExecutionStatusIndeterminate {
				return fmt.Errorf("%w: cancellation can only end cancelled or indeterminate", ErrLedgerInvariant)
			}
			if event.Lifecycle.Status == union.ExecutionStatusCancelled && state.Cancellation != CancellationReconciled {
				return fmt.Errorf("%w: cancelled terminal requires reconciliation", ErrLedgerInvariant)
			}
		}
		if state.HasUncertainSideEffects() && event.Lifecycle.Status != union.ExecutionStatusIndeterminate {
			return fmt.Errorf("%w: unknown side effects require an indeterminate terminal", ErrLedgerInvariant)
		}
		state.Terminal = true
		state.TerminalEventID = event.Header.EventID
		state.TerminalStatus = event.Lifecycle.Status
	}
	return nil
}

func containsIntentID(values []union.IntentID, target union.IntentID) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func terminalAttemptStatus(status union.AttemptStatus) bool {
	switch status {
	case union.AttemptStatusCompleted, union.AttemptStatusFailed, union.AttemptStatusDeclined,
		union.AttemptStatusCancelled, union.AttemptStatusIndeterminate:
		return true
	default:
		return false
	}
}

func validAttemptTransition(previous, next union.AttemptStatus) bool {
	if previous == next {
		return true
	}
	if terminalAttemptStatus(previous) {
		return false
	}
	switch previous {
	case union.AttemptStatusPlanned:
		return next == union.AttemptStatusSelected || next == union.AttemptStatusAwaitingApproval || next == union.AttemptStatusRunning || terminalAttemptStatus(next)
	case union.AttemptStatusSelected:
		return next == union.AttemptStatusAwaitingApproval || next == union.AttemptStatusRunning || terminalAttemptStatus(next)
	case union.AttemptStatusAwaitingApproval:
		return next == union.AttemptStatusRunning || terminalAttemptStatus(next)
	case union.AttemptStatusRunning:
		return next == union.AttemptStatusAwaitingApproval || terminalAttemptStatus(next)
	default:
		return false
	}
}

func terminalItemStatus(status union.ItemStatus) bool {
	switch status {
	case union.ItemStatusCompleted, union.ItemStatusIncomplete, union.ItemStatusFailed,
		union.ItemStatusCancelled, union.ItemStatusIndeterminate:
		return true
	default:
		return false
	}
}

func validItemTransition(previous, next union.ItemStatus) bool {
	if previous == next {
		return true
	}
	if terminalItemStatus(previous) {
		return false
	}
	switch previous {
	case union.ItemStatusPending:
		return next == union.ItemStatusInProgress || terminalItemStatus(next)
	case union.ItemStatusInProgress:
		return terminalItemStatus(next)
	default:
		return false
	}
}

func validSideEffectTransition(previous, next union.SideEffectState) bool {
	if previous == next {
		return true
	}
	switch previous {
	case union.SideEffectNone:
		return true
	case union.SideEffectPossible:
		return next == union.SideEffectObserved || next == union.SideEffectReconciled || next == union.SideEffectUnknown
	case union.SideEffectObserved:
		return next == union.SideEffectReconciled || next == union.SideEffectUnknown
	case union.SideEffectReconciled:
		return false
	case union.SideEffectUnknown:
		return next == union.SideEffectReconciled
	default:
		return false
	}
}

func validateCandidateStatus(status union.ExecutionStatus) error {
	switch status {
	case union.ExecutionStatusSucceeded, union.ExecutionStatusPartial, union.ExecutionStatusFailed,
		union.ExecutionStatusCancelled, union.ExecutionStatusIndeterminate:
		return nil
	default:
		return fmt.Errorf("%w: route terminal status is invalid", ErrLedgerInvariant)
	}
}

func validateSideEffectState(state union.SideEffectState) error {
	switch state {
	case union.SideEffectNone, union.SideEffectPossible, union.SideEffectObserved, union.SideEffectReconciled, union.SideEffectUnknown:
		return nil
	default:
		return fmt.Errorf("%w: side effect state is invalid", ErrLedgerInvariant)
	}
}

func applyControl(state *LedgerState, event union.UnifiedExecutionEvent) error {
	control := event.Control
	if runtimeOwnedControl(control.Kind) && event.Header.Origin != union.EventOriginPraxis {
		return ErrAdapterAuthority
	}
	switch control.Kind {
	case ControlApprovalRequested:
		if control.ApprovalID == "" || control.ActionID == "" || control.MechanismAttemptID == "" ||
			strings.TrimSpace(control.InputDigest) == "" || control.ActionRevision == 0 {
			return fmt.Errorf("%w: approval request identity is incomplete", ErrLedgerInvariant)
		}
		if control.ExpiresAt.IsZero() || !control.ExpiresAt.After(event.Header.Timestamp) {
			return fmt.Errorf("%w: approval request expiry is invalid", ErrLedgerInvariant)
		}
		if _, exists := state.Attempts[control.MechanismAttemptID]; !exists {
			return fmt.Errorf("%w: approval request requires an observed attempt", ErrLedgerInvariant)
		}
		if previous, exists := state.Approvals[control.ApprovalID]; exists {
			if previous.Status != ApprovalPending || previous.ActionID != control.ActionID || previous.MechanismAttemptID != control.MechanismAttemptID {
				return fmt.Errorf("%w: approval identity cannot be reused", ErrLedgerInvariant)
			}
			if control.ActionRevision <= previous.ActionRevision || control.InputDigest == previous.InputDigest {
				return fmt.Errorf("%w: approval revision must advance with changed input", ErrLedgerInvariant)
			}
		}
		state.Approvals[control.ApprovalID] = ApprovalState{
			ApprovalID: control.ApprovalID, ActionID: control.ActionID, MechanismAttemptID: control.MechanismAttemptID,
			InputDigest: control.InputDigest, ActionRevision: control.ActionRevision, ExpiresAt: control.ExpiresAt,
			Status: ApprovalPending,
		}
	case ControlApprovalResolved:
		approval, exists := state.Approvals[control.ApprovalID]
		if !exists || approval.Status != ApprovalPending {
			return ErrApprovalNotPending
		}
		if !event.Header.Timestamp.Before(approval.ExpiresAt) {
			return ErrApprovalExpired
		}
		if approval.ActionID != control.ActionID || approval.MechanismAttemptID != control.MechanismAttemptID ||
			approval.InputDigest != control.InputDigest || approval.ActionRevision != control.ActionRevision {
			return ErrApprovalRevision
		}
		if control.Decision != "approve" && control.Decision != "deny" {
			return fmt.Errorf("%w: approval decision is invalid", ErrLedgerInvariant)
		}
		if err := recordCommandReceipt(state, control); err != nil {
			return err
		}
		approval.Status, approval.Decision = ApprovalResolved, control.Decision
		state.Approvals[control.ApprovalID] = approval
	case ControlApprovalExpired:
		approval, exists := state.Approvals[control.ApprovalID]
		if !exists || approval.Status != ApprovalPending {
			return ErrApprovalNotPending
		}
		if event.Header.Timestamp.Before(approval.ExpiresAt) {
			return fmt.Errorf("%w: approval has not expired", ErrLedgerInvariant)
		}
		if (event.Header.ActionID != "" && event.Header.ActionID != approval.ActionID) ||
			(event.Header.MechanismAttemptID != "" && event.Header.MechanismAttemptID != approval.MechanismAttemptID) {
			return ErrApprovalRevision
		}
		approval.Status = ApprovalExpired
		state.Approvals[control.ApprovalID] = approval
	case ControlApprovalInvalidated:
		approval, exists := state.Approvals[control.ApprovalID]
		if !exists || approval.Status != ApprovalPending {
			return ErrApprovalNotPending
		}
		if (event.Header.ActionID != "" && event.Header.ActionID != approval.ActionID) ||
			(event.Header.MechanismAttemptID != "" && event.Header.MechanismAttemptID != approval.MechanismAttemptID) {
			return ErrApprovalRevision
		}
		approval.Status = ApprovalInvalidated
		state.Approvals[control.ApprovalID] = approval
	case ControlCancelRequested:
		if state.Cancellation != CancellationNone {
			return ErrCancelState
		}
		if err := recordCommandReceipt(state, control); err != nil {
			return err
		}
		state.Cancellation = CancellationRequested
		invalidatePendingApprovals(state)
	case ControlCancelDispatched:
		if state.Cancellation != CancellationRequested {
			return ErrCancelState
		}
		state.Cancellation = CancellationDispatched
	case ControlCancelAcknowledged:
		if state.Cancellation != CancellationDispatched {
			return ErrCancelState
		}
		state.Cancellation = CancellationAcknowledged
	case ControlCancellationQuiesced:
		if state.Cancellation != CancellationAcknowledged {
			return ErrCancelState
		}
		state.Cancellation = CancellationQuiesced
	case ControlReconciliationStarted:
		if state.Reconciliation != ReconciliationNone {
			return ErrCancelState
		}
		if state.Cancellation != CancellationNone {
			if state.Cancellation != CancellationQuiesced {
				return ErrCancelState
			}
			state.Cancellation = CancellationReconciling
		}
		state.Reconciliation = ReconciliationStarted
	case ControlReconciliationComplete:
		if state.Reconciliation != ReconciliationStarted {
			return ErrCancelState
		}
		var summary ReconciliationSummary
		if len(control.Payload) == 0 || json.Unmarshal(control.Payload, &summary) != nil || !summary.Quiesced {
			return fmt.Errorf("%w: reconciliation summary is invalid", ErrLedgerInvariant)
		}
		if err := validateSideEffectState(summary.SideEffectState); err != nil {
			return err
		}
		if summary.SideEffectState == union.SideEffectPossible || summary.SideEffectState == union.SideEffectUnknown {
			state.UncertainAttempts[""] = true
		} else {
			clear(state.UncertainAttempts)
		}
		if state.Cancellation == CancellationReconciling {
			state.Cancellation = CancellationReconciled
		}
		state.Reconciliation = ReconciliationCompleted
	case ControlBackgroundWorkUpdated, ControlBackgroundDrained:
		if control.PendingBackgroundWork < 0 {
			return fmt.Errorf("%w: pending background work is negative", ErrLedgerInvariant)
		}
		if control.Kind == ControlBackgroundDrained && control.PendingBackgroundWork != 0 {
			return fmt.Errorf("%w: background drain must report zero", ErrLedgerInvariant)
		}
		state.PendingBackgroundWork = control.PendingBackgroundWork
	case ControlCommandDispatched:
		if err := recordCommandReceipt(state, control); err != nil {
			return err
		}
	}
	return nil
}

func runtimeOwnedControl(kind string) bool {
	switch kind {
	case ControlApprovalResolved, ControlApprovalExpired, ControlApprovalInvalidated,
		ControlCancelRequested, ControlCancelDispatched, ControlReconciliationStarted,
		ControlReconciliationComplete, ControlCommandDispatched:
		return true
	default:
		return false
	}
}

func invalidatePendingApprovals(state *LedgerState) {
	for id, approval := range state.Approvals {
		if approval.Status == ApprovalPending {
			approval.Status = ApprovalInvalidated
			state.Approvals[id] = approval
		}
	}
}

func recordCommandReceipt(state *LedgerState, control *union.ControlEvent) error {
	if strings.TrimSpace(control.IdempotencyKey) == "" {
		return fmt.Errorf("%w: command idempotency key is required", ErrLedgerInvariant)
	}
	var receipt commandReceipt
	if len(control.Payload) == 0 || json.Unmarshal(control.Payload, &receipt) != nil || strings.TrimSpace(receipt.Digest) == "" {
		return fmt.Errorf("%w: command receipt is invalid", ErrLedgerInvariant)
	}
	if previous, exists := state.CommandDigests[control.IdempotencyKey]; exists && previous != receipt.Digest {
		return ErrIdempotencyConflict
	}
	state.CommandDigests[control.IdempotencyKey] = receipt.Digest
	return nil
}

func (ledger *EventLedger) CheckCommand(command union.ExecutionCommand, now time.Time) (CommandDisposition, error) {
	if ledger == nil {
		return CommandDisposition{}, ErrLedgerInvariant
	}
	if err := command.Validate(); err != nil {
		return CommandDisposition{}, fmt.Errorf("%w: %v", ErrInvalidInvocation, err)
	}
	if now.IsZero() {
		return CommandDisposition{}, fmt.Errorf("%w: command time is required", ErrInvalidInvocation)
	}
	digest, err := command.Digest()
	if err != nil {
		return CommandDisposition{}, err
	}
	ledger.mu.RLock()
	defer ledger.mu.RUnlock()
	if command.ExecutionID != ledger.state.ExecutionID {
		return CommandDisposition{}, fmt.Errorf("%w: execution identity differs", ErrInvalidInvocation)
	}
	if previous, exists := ledger.state.CommandDigests[command.IdempotencyKey]; exists {
		if previous != digest {
			return CommandDisposition{}, ErrIdempotencyConflict
		}
		return CommandDisposition{Duplicate: true, Digest: digest}, nil
	}
	if ledger.state.Terminal {
		return CommandDisposition{}, ErrTerminal
	}
	if command.ExpectedExecutionStatus != ledger.state.ExecutionStatus() {
		return CommandDisposition{}, ErrOptimisticConcurrency
	}
	disposition := CommandDisposition{Digest: digest}
	if command.Kind == union.CommandApproveAction || command.Kind == union.CommandDenyAction {
		approval, exists := ledger.state.Approvals[command.ApprovalID]
		if !exists || approval.Status != ApprovalPending {
			return CommandDisposition{}, ErrApprovalNotPending
		}
		if !now.Before(approval.ExpiresAt) {
			disposition.ApprovalExpired = true
			return disposition, ErrApprovalExpired
		}
		if approval.ActionID != command.ActionID || approval.MechanismAttemptID != command.MechanismAttemptID ||
			approval.InputDigest != command.InputDigest || approval.ActionRevision != command.ActionRevision {
			return CommandDisposition{}, ErrApprovalRevision
		}
	}
	if command.Kind == union.CommandCancelExecution && ledger.state.Cancellation != CancellationNone {
		return CommandDisposition{}, ErrCancelState
	}
	return disposition, nil
}
