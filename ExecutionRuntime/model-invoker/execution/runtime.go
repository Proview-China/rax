package execution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/effect"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/union"
)

type EventIDGenerator func(union.ExecutionID, uint64) union.EventID

type RuntimeConfig struct {
	Registry   *Registry
	Reconciler Reconciler
	Verifier   Verifier
	Sanitizer  EventSanitizer
	Clock      func() time.Time
	EventID    EventIDGenerator
}

type Runtime struct {
	registry   *Registry
	reconciler Reconciler
	verifier   Verifier
	sanitizer  EventSanitizer
	clock      func() time.Time
	eventID    EventIDGenerator
	counter    atomic.Uint64
}

// Orchestrator is the public orchestration role of Runtime. The alias keeps a
// single authority for event commitment and terminal projection.
type Orchestrator = Runtime

func NewOrchestrator(config RuntimeConfig) (*Orchestrator, error) {
	return NewRuntime(config)
}

func NewRuntime(config RuntimeConfig) (*Runtime, error) {
	if config.Registry == nil {
		return nil, fmt.Errorf("%w: registry is required", ErrInvalidInvocation)
	}
	runtime := &Runtime{
		registry: config.Registry, reconciler: config.Reconciler, verifier: config.Verifier, sanitizer: config.Sanitizer,
		clock: config.Clock, eventID: config.EventID,
	}
	if runtime.clock == nil {
		runtime.clock = time.Now
	}
	if runtime.reconciler == nil {
		runtime.reconciler = defaultReconciler{}
	}
	if runtime.verifier == nil {
		runtime.verifier = defaultVerifier{}
	}
	if runtime.sanitizer == nil {
		runtime.sanitizer, _ = NewEventRedactor()
	}
	if runtime.eventID == nil {
		runtime.eventID = func(executionID union.ExecutionID, sequence uint64) union.EventID {
			serial := runtime.counter.Add(1)
			return union.EventID(fmt.Sprintf("%s:event:%d:%d", executionID, sequence, serial))
		}
	}
	return runtime, nil
}

type defaultReconciler struct{}

func (defaultReconciler) Reconcile(_ context.Context, input ReconcileInput) (ReconcileReport, error) {
	state := union.SideEffectNone
	if input.State.HasUncertainSideEffects() {
		state = union.SideEffectUnknown
	}
	quiesced := input.State.Cancellation == CancellationNone ||
		input.State.Cancellation == CancellationQuiesced || input.State.Cancellation == CancellationReconciling ||
		input.State.Cancellation == CancellationReconciled
	quiesced = quiesced && input.State.PendingBackgroundWork == 0
	return ReconcileReport{SideEffectState: state, Quiesced: quiesced}, nil
}

type defaultVerifier struct{}

func (defaultVerifier) Verify(context.Context, VerifyInput) (VerificationReport, error) {
	return VerificationReport{}, nil
}

type Execution struct {
	runtime    *Runtime
	descriptor AdapterDescriptor
	invocation Invocation
	preflight  PreflightReport
	session    Session
	ledger     *EventLedger
	ctx        context.Context
	cancel     context.CancelFunc

	emitMu     sync.Mutex
	commandMu  sync.Mutex
	closeOnce  sync.Once
	closeErr   error
	done       chan struct{}
	resultMu   sync.RWMutex
	result     union.UnifiedExecutionResult
	err        error
	residualMu sync.Mutex
	residuals  []union.Residual
}

func (runtime *Runtime) Start(ctx context.Context, adapterID string, invocation Invocation) (*Execution, error) {
	if runtime == nil || ctx == nil {
		return nil, ErrInvalidInvocation
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	normalized, err := invocation.Clone()
	if err != nil {
		return nil, fmt.Errorf("%w: clone invocation", ErrInvalidInvocation)
	}
	if err := normalized.Validate(); err != nil {
		return nil, err
	}
	registered, err := runtime.registry.Resolve(adapterID)
	if err != nil {
		return nil, err
	}
	if !registered.Descriptor.Supports(normalized.Plan.ExecutionKind) {
		return nil, fmt.Errorf("%w: adapter does not support the planned execution kind", ErrInvalidInvocation)
	}
	adapterInvocation, err := normalized.Clone()
	if err != nil {
		return nil, err
	}
	preflight, err := registered.Adapter.Preflight(ctx, adapterInvocation)
	if err != nil {
		cleanupPrepared(registered.Adapter, normalized.Request.ExecutionID)
		return nil, fmt.Errorf("execution preflight: %w", err)
	}
	if err := preflight.Validate(); err != nil {
		cleanupPrepared(registered.Adapter, normalized.Request.ExecutionID)
		return nil, err
	}
	if !preflight.Accepted {
		cleanupPrepared(registered.Adapter, normalized.Request.ExecutionID)
		return nil, ErrPreflightRejected
	}
	actualManifest, err := preflight.ActualManifest.Clone()
	if err != nil {
		cleanupPrepared(registered.Adapter, normalized.Request.ExecutionID)
		return nil, err
	}
	preflight.ActualManifest = actualManifest
	preflight.Residuals = append([]union.Residual(nil), preflight.Residuals...)
	if err := CompareContextManifests(normalized.Plan.ExpectedManifest, preflight.ActualManifest); err != nil {
		cleanupPrepared(registered.Adapter, normalized.Request.ExecutionID)
		return nil, err
	}
	ledger, err := NewEventLedgerForPlan(normalized.Plan)
	if err != nil {
		return nil, err
	}
	runCtx, cancel := context.WithCancel(ctx)
	execution := &Execution{
		runtime: runtime, descriptor: registered.Descriptor, invocation: normalized, preflight: preflight,
		ledger: ledger, ctx: runCtx, cancel: cancel, done: make(chan struct{}),
		residuals: append([]union.Residual(nil), preflight.Residuals...),
	}
	if err := execution.emitInitialEvents(); err != nil {
		cancel()
		cleanupPrepared(registered.Adapter, normalized.Request.ExecutionID)
		return nil, err
	}
	adapterInvocation, err = normalized.Clone()
	if err != nil {
		cancel()
		cleanupPrepared(registered.Adapter, normalized.Request.ExecutionID)
		return nil, err
	}
	session, err := registered.Adapter.Open(runCtx, adapterInvocation)
	if err != nil {
		cancel()
		cleanupPrepared(registered.Adapter, normalized.Request.ExecutionID)
		return nil, fmt.Errorf("open execution session: %w", err)
	}
	if session == nil {
		cancel()
		cleanupPrepared(registered.Adapter, normalized.Request.ExecutionID)
		return nil, fmt.Errorf("%w: adapter returned a nil session", ErrInvalidAdapter)
	}
	execution.session = session
	go execution.receiveLoop()
	return execution, nil
}

func cleanupPrepared(adapter Adapter, executionID union.ExecutionID) {
	if cleaner, ok := adapter.(PreflightCleaner); ok {
		_ = cleaner.ClosePrepared(executionID)
	}
}

func (runtime *Runtime) Execute(ctx context.Context, adapterID string, invocation Invocation) (union.UnifiedExecutionResult, error) {
	execution, err := runtime.Start(ctx, adapterID, invocation)
	if err != nil {
		return union.UnifiedExecutionResult{}, err
	}
	return execution.Wait(ctx)
}

func (execution *Execution) Wait(ctx context.Context) (union.UnifiedExecutionResult, error) {
	if execution == nil || ctx == nil {
		return union.UnifiedExecutionResult{}, ErrInvalidInvocation
	}
	select {
	case <-execution.done:
		execution.resultMu.RLock()
		result, err := execution.result, execution.err
		execution.resultMu.RUnlock()
		clone, cloneErr := result.Clone()
		if cloneErr != nil && err == nil {
			err = cloneErr
		}
		return clone, err
	case <-ctx.Done():
		return union.UnifiedExecutionResult{}, ctx.Err()
	}
}

func (execution *Execution) Events() []union.UnifiedExecutionEvent {
	if execution == nil {
		return nil
	}
	return execution.ledger.Events()
}

func (execution *Execution) State() LedgerState {
	if execution == nil {
		return LedgerState{}
	}
	return execution.ledger.State()
}

func (execution *Execution) emitInitialEvents() error {
	if err := execution.commitRuntime(union.UnifiedExecutionEvent{
		Lifecycle: &union.LifecycleEvent{Kind: "execution_started"},
	}); err != nil {
		return err
	}
	for _, intent := range execution.invocation.Plan.IntentGraph.Nodes {
		if err := execution.commitRuntime(union.UnifiedExecutionEvent{
			Header: union.EventHeader{IntentID: intent.ID},
			Intent: &union.IntentEvent{Kind: "accepted"},
		}); err != nil {
			return err
		}
	}
	for index := range execution.invocation.Plan.Mechanisms {
		plan := execution.invocation.Plan.Mechanisms[index]
		if err := execution.commitRuntime(union.UnifiedExecutionEvent{
			Header:    union.EventHeader{IntentID: plan.IntentID, MechanismPlanID: plan.ID},
			Mechanism: &union.MechanismEvent{Kind: "planned", Plan: &plan},
		}); err != nil {
			return err
		}
	}
	selected := make(map[union.IntentID]union.MechanismPlan)
	for _, mechanism := range execution.invocation.Plan.Mechanisms {
		current, exists := selected[mechanism.IntentID]
		if !exists || mechanism.PreferredRank < current.PreferredRank ||
			(mechanism.PreferredRank == current.PreferredRank && mechanism.ID < current.ID) {
			selected[mechanism.IntentID] = mechanism
		}
	}
	for _, intent := range execution.invocation.Plan.IntentGraph.Nodes {
		plan := selected[intent.ID]
		if err := execution.commitRuntime(union.UnifiedExecutionEvent{
			Header:    union.EventHeader{IntentID: plan.IntentID, MechanismPlanID: plan.ID},
			Mechanism: &union.MechanismEvent{Kind: "selected", Plan: &plan},
		}); err != nil {
			return err
		}
	}
	return nil
}

func (execution *Execution) commitRuntime(event union.UnifiedExecutionEvent) error {
	event.Header.Origin = union.EventOriginPraxis
	return execution.commit(event, true)
}

func (execution *Execution) commitCandidate(event union.UnifiedExecutionEvent) error {
	return execution.commit(event, false)
}

func (execution *Execution) commit(event union.UnifiedExecutionEvent, runtimeOwned bool) error {
	execution.emitMu.Lock()
	defer execution.emitMu.Unlock()
	return execution.commitLocked(event, runtimeOwned)
}

// commitLocked appends an event while emitMu is already held. Command uses it
// to make Adapter dispatch and its Praxis receipt an ordering barrier: a fast
// Adapter cannot race an acknowledgement or terminal ahead of the receipt.
func (execution *Execution) commitLocked(event union.UnifiedExecutionEvent, runtimeOwned bool) error {
	clone, err := event.Clone()
	if err != nil {
		return fmt.Errorf("%w: candidate clone failed", ErrLedgerInvariant)
	}
	event = clone
	if !runtimeOwned {
		event, err = execution.runtime.sanitizer.Sanitize(event)
		if err != nil {
			return fmt.Errorf("%w: candidate sanitization failed", ErrLedgerInvariant)
		}
	}
	family, err := taggedFamily(event)
	if err != nil {
		return err
	}
	header := &event.Header
	if !runtimeOwned {
		if header.Origin == union.EventOriginPraxis {
			return ErrAdapterAuthority
		}
		if header.Origin == "" {
			header.Origin = execution.descriptor.Origin
		}
		if header.SemanticVersion != "" && header.SemanticVersion != union.SemanticVersionV1 {
			return fmt.Errorf("%w: candidate semantic version differs", ErrLedgerInvariant)
		}
		if header.ExecutionID != "" && header.ExecutionID != execution.invocation.Request.ExecutionID {
			return fmt.Errorf("%w: candidate execution identity differs", ErrLedgerInvariant)
		}
		if header.ExecutionKind != "" && header.ExecutionKind != execution.invocation.Plan.ExecutionKind {
			return fmt.Errorf("%w: candidate execution kind differs", ErrLedgerInvariant)
		}
		if expected := execution.invocation.Request.SessionIntent.SessionID; expected != "" && header.SessionID != "" && header.SessionID != expected {
			return fmt.Errorf("%w: candidate session identity differs", ErrLedgerInvariant)
		}
		if expected := execution.invocation.Request.SessionIntent.TurnID; expected != "" && header.TurnID != "" && header.TurnID != expected {
			return fmt.Errorf("%w: candidate turn identity differs", ErrLedgerInvariant)
		}
		if !zeroIdentity(header.Profile) && header.Profile != execution.invocation.Plan.Profile {
			return fmt.Errorf("%w: candidate profile identity differs", ErrLedgerInvariant)
		}
		if !zeroIdentity(header.Route) && header.Route != execution.invocation.Plan.Route {
			return fmt.Errorf("%w: candidate route identity differs", ErrLedgerInvariant)
		}
		if header.Family != "" && header.Family != family {
			return fmt.Errorf("%w: candidate family differs from payload", ErrLedgerInvariant)
		}
		if header.SourceSequence == 0 {
			header.SourceSequence = header.Sequence
		}
		if header.SourceTimestamp.IsZero() {
			header.SourceTimestamp = header.Timestamp
		}
		if header.NativeIdentity == nil && header.EventID != "" {
			header.NativeIdentity = &union.NativeIdentity{
				Namespace: execution.descriptor.Identity.ID, Kind: "event", Value: string(header.EventID),
			}
		}
	} else if header.Origin != union.EventOriginPraxis {
		return ErrAdapterAuthority
	}
	state := execution.ledger.State()
	sequence := state.LastSequence + 1
	now := execution.runtime.clock().UTC()
	if now.IsZero() {
		return fmt.Errorf("%w: runtime clock returned zero", ErrLedgerInvariant)
	}
	if !state.LastTimestamp.IsZero() && now.Before(state.LastTimestamp) {
		now = state.LastTimestamp
	}
	header.EventID = execution.runtime.eventID(execution.invocation.Request.ExecutionID, sequence)
	header.Sequence = sequence
	header.Timestamp = now
	header.IngestedAt = now
	header.SemanticVersion = union.SemanticVersionV1
	header.ExecutionID = execution.invocation.Request.ExecutionID
	header.ExecutionKind = execution.invocation.Plan.ExecutionKind
	header.Profile = execution.invocation.Plan.Profile
	header.Route = execution.invocation.Plan.Route
	header.Family = family
	if header.SessionID == "" {
		header.SessionID = execution.invocation.Request.SessionIntent.SessionID
	}
	if header.TurnID == "" {
		header.TurnID = execution.invocation.Request.SessionIntent.TurnID
	}
	if header.Visibility == "" {
		header.Visibility = union.VisibilityAuditOnly
	}
	if header.SecurityClassification == "" {
		header.SecurityClassification = union.SecurityInternal
	}
	return execution.ledger.Append(event)
}

func zeroIdentity(identity union.VersionedIdentity) bool {
	return identity.ID == "" && identity.Version == ""
}

func taggedFamily(event union.UnifiedExecutionEvent) (union.EventFamily, error) {
	family := union.EventFamily("")
	count := 0
	for candidate, present := range map[union.EventFamily]bool{
		union.EventFamilyLifecycle:  event.Lifecycle != nil,
		union.EventFamilyIntent:     event.Intent != nil,
		union.EventFamilyMechanism:  event.Mechanism != nil,
		union.EventFamilyModel:      event.Model != nil,
		union.EventFamilyItem:       event.Item != nil,
		union.EventFamilyEffect:     event.Effect != nil,
		union.EventFamilyControl:    event.Control != nil,
		union.EventFamilyDiagnostic: event.Diagnostic != nil,
	} {
		if present {
			family = candidate
			count++
		}
	}
	if count != 1 {
		return "", fmt.Errorf("%w: candidate must contain exactly one event payload", ErrLedgerInvariant)
	}
	return family, nil
}

func (execution *Execution) receiveLoop() {
	defer close(execution.done)
	defer execution.cancel()
	var receiveErr error
	for {
		candidate, err := execution.session.Receive(execution.ctx)
		if err != nil {
			receiveErr = err
			break
		}
		if err := execution.ingestCandidate(candidate); err != nil {
			receiveErr = err
			break
		}
		if execution.readyToFinalize() {
			break
		}
	}
	closeErr := execution.closeSession()
	if closeErr != nil && receiveErr == nil {
		receiveErr = closeErr
	}
	if receiveErr != nil && !errors.Is(receiveErr, io.EOF) && !errors.Is(receiveErr, context.Canceled) {
		execution.addResidual("session", "transport_error", "P1", "adapter session ended before a trustworthy unified terminal")
	}
	if !execution.ledger.State().RouteTerminalObserved {
		status := union.ExecutionStatusIndeterminate
		sideEffects := execution.currentSideEffectState()
		if errors.Is(receiveErr, io.EOF) && !execution.ledger.State().HasUncertainSideEffects() {
			status = union.ExecutionStatusFailed
		} else if receiveErr != nil {
			sideEffects = union.SideEffectUnknown
		}
		if err := execution.emitRouteTerminal(RouteTerminalCandidate{
			Status: status, StopReason: "session_ended_without_route_terminal", SideEffectState: sideEffects,
		}); err != nil && receiveErr == nil {
			receiveErr = err
		}
	}
	result, err := execution.finalize(receiveErr)
	execution.resultMu.Lock()
	execution.result, execution.err = result, err
	execution.resultMu.Unlock()
}

func (execution *Execution) closeSession() error {
	execution.closeOnce.Do(func() { execution.closeErr = execution.session.Close() })
	return execution.closeErr
}

func (execution *Execution) ingestCandidate(candidate union.UnifiedExecutionEvent) error {
	if candidate.Effect != nil {
		return ErrAdapterAuthority
	}
	if candidate.Lifecycle != nil && candidate.Lifecycle.Status != "" {
		terminal := RouteTerminalCandidate{
			Status: candidate.Lifecycle.Status, StopReason: candidate.Lifecycle.StopReason,
			PendingBackgroundWork: candidate.Lifecycle.PendingBackgroundWork, SideEffectState: execution.currentSideEffectState(),
		}
		payload, err := json.Marshal(terminal)
		if err != nil {
			return err
		}
		header := candidate.Header
		header.Family = ""
		candidate = union.UnifiedExecutionEvent{
			Header:     header,
			Diagnostic: &union.DiagnosticEvent{Kind: EventKindRouteTerminalCandidate, Payload: payload},
		}
	}
	return execution.commitCandidate(candidate)
}

func (execution *Execution) readyToFinalize() bool {
	state := execution.ledger.State()
	if !state.RouteTerminalObserved || state.PendingBackgroundWork != 0 {
		return false
	}
	if state.Cancellation == CancellationNone {
		return true
	}
	return state.Cancellation == CancellationQuiesced || state.Cancellation == CancellationReconciled
}

func (execution *Execution) currentSideEffectState() union.SideEffectState {
	if execution.ledger.State().HasUncertainSideEffects() {
		return union.SideEffectUnknown
	}
	return union.SideEffectNone
}

func (execution *Execution) emitRouteTerminal(candidate RouteTerminalCandidate) error {
	payload, err := json.Marshal(candidate)
	if err != nil {
		return err
	}
	return execution.commitRuntime(union.UnifiedExecutionEvent{
		Diagnostic: &union.DiagnosticEvent{Kind: EventKindRouteTerminalCandidate, Payload: payload},
	})
}

func (execution *Execution) addResidual(path, kind, severity, impact string) {
	execution.residualMu.Lock()
	execution.residuals = append(execution.residuals, union.Residual{
		Path: path, Kind: kind, Severity: severity, Impact: impact,
	})
	execution.residualMu.Unlock()
}

func (execution *Execution) appendResiduals(residuals []union.Residual) {
	execution.residualMu.Lock()
	execution.residuals = append(execution.residuals, residuals...)
	execution.residualMu.Unlock()
}

func (execution *Execution) residualSnapshot() []union.Residual {
	execution.residualMu.Lock()
	residuals := append([]union.Residual(nil), execution.residuals...)
	execution.residualMu.Unlock()
	return residuals
}

func (execution *Execution) finalize(receiveErr error) (union.UnifiedExecutionResult, error) {
	state := execution.ledger.State()
	canReconcile := state.Cancellation == CancellationNone || state.Cancellation == CancellationQuiesced
	if canReconcile {
		if err := execution.commitRuntime(union.UnifiedExecutionEvent{
			Control: &union.ControlEvent{Kind: ControlReconciliationStarted},
		}); err != nil {
			return union.UnifiedExecutionResult{}, err
		}
	}
	reconcileInvocation, err := execution.invocation.Clone()
	if err != nil {
		return union.UnifiedExecutionResult{}, err
	}
	reconcileInput := ReconcileInput{
		Invocation: reconcileInvocation, Events: execution.ledger.Events(), State: execution.ledger.State(),
		Candidate: execution.ledger.State().RouteTerminalCandidate,
	}
	reconcileReport, reconcileErr := execution.runtime.reconciler.Reconcile(execution.ctx, reconcileInput)
	if reconcileErr != nil {
		execution.addResidual("reconciliation", "reconcile_error", "P0", "side effects could not be reconciled")
		reconcileReport.SideEffectState = union.SideEffectUnknown
		reconcileReport.Quiesced = false
	}
	if err := validateSideEffectState(reconcileReport.SideEffectState); err != nil {
		execution.addResidual("reconciliation.side_effect_state", "invalid_reconcile_report", "P0", "reconciler returned an invalid side-effect state")
		reconcileReport.SideEffectState = union.SideEffectUnknown
		reconcileErr = err
	}
	if (len(reconcileReport.Effects) != 0 && reconcileReport.SideEffectState == union.SideEffectNone) ||
		(len(reconcileReport.Effects) == 0 && reconcileReport.SideEffectState == union.SideEffectObserved) {
		execution.addResidual("reconciliation", "inconsistent_reconcile_report", "P0", "reconciler Effect records and side-effect state disagree")
		reconcileReport.SideEffectState = union.SideEffectUnknown
		reconcileErr = ErrLedgerInvariant
	}
	if state.Cancellation != CancellationNone && !reconcileReport.Quiesced {
		reconcileReport.SideEffectState = union.SideEffectUnknown
	}
	if state.PendingBackgroundWork != 0 {
		if reconcileErr != nil || !reconcileReport.Quiesced {
			return union.UnifiedExecutionResult{}, fmt.Errorf("%w: background work did not quiesce", ErrProjectionInvariant)
		}
		if err := execution.commitRuntime(union.UnifiedExecutionEvent{
			Control: &union.ControlEvent{Kind: ControlBackgroundDrained, PendingBackgroundWork: 0},
		}); err != nil {
			return union.UnifiedExecutionResult{}, err
		}
	}
	committedEffects := make([]union.EffectRecord, 0, len(reconcileReport.Effects))
	for index := range reconcileReport.Effects {
		observed := reconcileReport.Effects[index]
		header := union.EventHeader{EffectID: observed.ID, MechanismAttemptID: observed.MechanismAttemptID}
		if len(observed.IntentIDs) != 0 {
			header.IntentID = observed.IntentIDs[0]
		}
		if observed.Payload.ToolCall != nil {
			header.ActionID = observed.Payload.ToolCall.ActionID
		}
		if err := execution.commitRuntime(union.UnifiedExecutionEvent{
			Header: header, Effect: &union.EffectEvent{Kind: "observed", Effect: &observed},
		}); err != nil {
			execution.addResidual("reconciliation.effects", "invalid_observed_effect", "P0", "reconciler Effect was rejected by the execution ledger")
			reconcileReport.SideEffectState = union.SideEffectUnknown
			reconcileErr = err
			continue
		}
		committedEffects = append(committedEffects, observed)
	}
	if canReconcile && reconcileErr == nil && reconcileReport.Quiesced {
		summary, err := json.Marshal(ReconciliationSummary{
			SideEffectState: reconcileReport.SideEffectState, Quiesced: true,
		})
		if err != nil {
			return union.UnifiedExecutionResult{}, err
		}
		if err := execution.commitRuntime(union.UnifiedExecutionEvent{
			Control: &union.ControlEvent{Kind: ControlReconciliationComplete, Payload: summary},
		}); err != nil {
			return union.UnifiedExecutionResult{}, err
		}
	}
	execution.appendResiduals(reconcileReport.Residuals)

	verifyInvocation, err := execution.invocation.Clone()
	if err != nil {
		return union.UnifiedExecutionResult{}, err
	}
	verificationReport, verifyErr := execution.runtime.verifier.Verify(execution.ctx, VerifyInput{
		Invocation: verifyInvocation, Events: execution.ledger.Events(), Effects: cloneEffects(committedEffects),
	})
	if verifyErr != nil {
		execution.addResidual("verification", "verification_error", "P1", "required verification could not be completed")
	}
	for index := range verificationReport.Verifications {
		verification := verificationReport.Verifications[index]
		header := union.EventHeader{VerificationID: verification.ID}
		if len(verification.IntentIDs) != 0 {
			header.IntentID = verification.IntentIDs[0]
		}
		if len(verification.EffectIDs) != 0 {
			header.EffectID = verification.EffectIDs[0]
		}
		if err := execution.commitRuntime(union.UnifiedExecutionEvent{
			Header: header, Effect: &union.EffectEvent{Kind: "verification_completed", Verification: &verification},
		}); err != nil {
			execution.addResidual("verification.records", "invalid_verification", "P1", "verification record was rejected by the execution ledger")
			verifyErr = err
		}
	}
	execution.appendResiduals(verificationReport.Residuals)

	status := execution.decideTerminal(reconcileReport.SideEffectState, reconcileErr, verifyErr)
	stopReason := execution.ledger.State().RouteTerminalCandidate.StopReason
	if receiveErr != nil && stopReason == "" {
		stopReason = "adapter_session_ended"
	}
	if err := execution.commitRuntime(union.UnifiedExecutionEvent{
		Lifecycle: &union.LifecycleEvent{
			Kind: "execution_" + string(status), Status: status, StopReason: stopReason, PendingBackgroundWork: 0,
		},
	}); err != nil {
		return union.UnifiedExecutionResult{}, err
	}
	return (Projector{}).Project(ProjectionInput{
		Invocation: execution.invocation, Events: execution.ledger.Events(),
		ContextManifest: execution.preflight.ActualManifest, Residuals: execution.residualSnapshot(),
	})
}

func cloneEffects(effects []union.EffectRecord) []union.EffectRecord {
	clones := make([]union.EffectRecord, 0, len(effects))
	for _, observed := range effects {
		clone, err := observed.Clone()
		if err != nil {
			return nil
		}
		clones = append(clones, clone)
	}
	return clones
}

func (execution *Execution) decideTerminal(sideEffects union.SideEffectState, reconcileErr, verifyErr error) union.ExecutionStatus {
	state := execution.ledger.State()
	if state.Cancellation != CancellationNone {
		if state.Cancellation == CancellationReconciled && sideEffects != union.SideEffectUnknown &&
			sideEffects != union.SideEffectPossible && reconcileErr == nil {
			return union.ExecutionStatusCancelled
		}
		return union.ExecutionStatusIndeterminate
	}
	if sideEffects == union.SideEffectUnknown || sideEffects == union.SideEffectPossible || reconcileErr != nil || state.HasUncertainSideEffects() {
		return union.ExecutionStatusIndeterminate
	}
	effects, verifications := collectEvidence(execution.ledger.Events())
	allRequiredSatisfied := true
	anyObserved := false
	contradicted := false
	for _, intent := range execution.invocation.Plan.IntentGraph.Nodes {
		satisfaction := effect.EvaluateIntent(intent, effects, verifications)
		if satisfaction.Status == union.IntentContradicted {
			contradicted = true
		}
		if satisfaction.Status == union.IntentSatisfied || satisfaction.Status == union.IntentPartiallySatisfied {
			anyObserved = true
		}
		if intent.Required && satisfaction.Status != union.IntentSatisfied {
			allRequiredSatisfied = false
		}
	}
	if contradicted {
		return union.ExecutionStatusFailed
	}
	if allRequiredSatisfied && verifyErr == nil {
		return union.ExecutionStatusSucceeded
	}
	if anyObserved {
		return union.ExecutionStatusPartial
	}
	if state.RouteTerminalCandidate.Status == union.ExecutionStatusIndeterminate {
		return union.ExecutionStatusIndeterminate
	}
	return union.ExecutionStatusFailed
}

func collectEvidence(events []union.UnifiedExecutionEvent) ([]union.EffectRecord, []union.VerificationRecord) {
	var effects []union.EffectRecord
	var verifications []union.VerificationRecord
	for _, event := range events {
		if event.Effect == nil {
			continue
		}
		if event.Effect.Effect != nil {
			effects = append(effects, *event.Effect.Effect)
		}
		if event.Effect.Verification != nil {
			verifications = append(verifications, *event.Effect.Verification)
		}
	}
	effectIndex := make(map[union.EffectID]int, len(effects))
	for index := range effects {
		effectIndex[effects[index].ID] = index
	}
	for _, verification := range verifications {
		for _, effectID := range verification.EffectIDs {
			index, exists := effectIndex[effectID]
			if !exists {
				continue
			}
			observed := &effects[index]
			if !containsVerificationID(observed.VerificationRefs, verification.ID) {
				observed.VerificationRefs = append(observed.VerificationRefs, verification.ID)
			}
			observed.VerificationStatus = mergeVerificationStatus(observed.VerificationStatus, verification.Status)
		}
	}
	return effects, verifications
}

func (execution *Execution) Command(ctx context.Context, command union.ExecutionCommand) error {
	if execution == nil || ctx == nil {
		return ErrInvalidInvocation
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if command.SessionID != "" && command.SessionID != execution.invocation.Request.SessionIntent.SessionID {
		return fmt.Errorf("%w: command session identity differs", ErrInvalidInvocation)
	}
	if command.TurnID != "" && command.TurnID != execution.invocation.Request.SessionIntent.TurnID {
		return fmt.Errorf("%w: command turn identity differs", ErrInvalidInvocation)
	}
	execution.commandMu.Lock()
	defer execution.commandMu.Unlock()
	now := execution.runtime.clock().UTC()
	disposition, err := execution.ledger.CheckCommand(command, now)
	if err != nil {
		if errors.Is(err, ErrApprovalExpired) && disposition.ApprovalExpired {
			_ = execution.commitRuntime(union.UnifiedExecutionEvent{
				Header:  union.EventHeader{ApprovalID: command.ApprovalID, ActionID: command.ActionID, MechanismAttemptID: command.MechanismAttemptID},
				Control: &union.ControlEvent{Kind: ControlApprovalExpired, ApprovalID: command.ApprovalID},
			})
		}
		return err
	}
	if disposition.Duplicate {
		return nil
	}
	receipt, err := json.Marshal(commandReceipt{Digest: disposition.Digest})
	if err != nil {
		return err
	}
	if command.Kind == union.CommandCancelExecution {
		if err := execution.commitRuntime(union.UnifiedExecutionEvent{
			Control: &union.ControlEvent{Kind: ControlCancelRequested, IdempotencyKey: command.IdempotencyKey, Payload: receipt},
		}); err != nil {
			return err
		}
		execution.emitMu.Lock()
		if err := execution.session.Command(ctx, command); err != nil {
			execution.emitMu.Unlock()
			execution.failCommandDispatch("cancellation.dispatch", "cancel_dispatch_error", "cancel request could not be confirmed as dispatched")
			return err
		}
		err := execution.commitLocked(union.UnifiedExecutionEvent{
			Header:  union.EventHeader{Origin: union.EventOriginPraxis},
			Control: &union.ControlEvent{Kind: ControlCancelDispatched},
		}, true)
		execution.emitMu.Unlock()
		return err
	}
	execution.emitMu.Lock()
	if err := execution.session.Command(ctx, command); err != nil {
		execution.emitMu.Unlock()
		execution.failCommandDispatch("command.dispatch", "command_dispatch_error", "command delivery could not be confirmed")
		return err
	}
	if command.Kind == union.CommandApproveAction || command.Kind == union.CommandDenyAction {
		decision := "approve"
		if command.Kind == union.CommandDenyAction {
			decision = "deny"
		}
		err := execution.commitLocked(union.UnifiedExecutionEvent{
			Header: union.EventHeader{Origin: union.EventOriginPraxis, ApprovalID: command.ApprovalID, ActionID: command.ActionID, MechanismAttemptID: command.MechanismAttemptID},
			Control: &union.ControlEvent{
				Kind: ControlApprovalResolved, ApprovalID: command.ApprovalID, ActionID: command.ActionID,
				MechanismAttemptID: command.MechanismAttemptID, InputDigest: command.InputDigest,
				ActionRevision: command.ActionRevision, IdempotencyKey: command.IdempotencyKey, Decision: decision, Payload: receipt,
			},
		}, true)
		execution.emitMu.Unlock()
		return err
	}
	err = execution.commitLocked(union.UnifiedExecutionEvent{
		Header: union.EventHeader{Origin: union.EventOriginPraxis, ActionID: command.ActionID, MechanismAttemptID: command.MechanismAttemptID},
		Control: &union.ControlEvent{
			Kind: ControlCommandDispatched, Scope: string(command.Kind), IdempotencyKey: command.IdempotencyKey, Payload: receipt,
		},
	}, true)
	execution.emitMu.Unlock()
	return err
}

func (execution *Execution) failCommandDispatch(path, kind, impact string) {
	execution.addResidual(path, kind, "P0", impact)
	_ = execution.emitRouteTerminal(RouteTerminalCandidate{
		Status: union.ExecutionStatusIndeterminate, StopReason: "command_dispatch_unconfirmed", SideEffectState: union.SideEffectUnknown,
	})
	execution.cancel()
}
