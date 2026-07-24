package lifecycle

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/composition"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/journal"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/ports"
)

type ConfigV1 struct {
	Decoder   ports.DefinitionDecoderV1
	Assembler ports.AgentAssemblerV1
	Compiler  ports.HarnessCompilerV1
	Bindings  ports.BindingPortV1
	Readiness ports.ReadinessPortV1
	Journal   *journal.CoordinatorV1
	Composer  *composition.ComposerV1
	Clock     func() time.Time
}

type HostV1 struct {
	config   ConfigV1
	locksMu  sync.Mutex
	locks    map[string]*startLockV1
	activeMu sync.Mutex
	active   map[string]*composition.CompositionV1
}

type startLockV1 struct {
	mu   sync.Mutex
	refs int
}

var _ ports.HostV1 = (*HostV1)(nil)

func NewHostV1(config ConfigV1) (*HostV1, error) {
	for name, value := range map[string]any{"decoder": config.Decoder, "assembler": config.Assembler, "compiler": config.Compiler, "bindings": config.Bindings, "readiness": config.Readiness, "journal": config.Journal, "composer": config.Composer} {
		if contract.IsTypedNilV1(value) {
			return nil, contract.NewError(contract.ErrorInvalidArgument, "host_dependency_missing", name+" dependency is required")
		}
	}
	if config.Clock == nil {
		config.Clock = time.Now
	}
	return &HostV1{config: config, locks: make(map[string]*startLockV1), active: make(map[string]*composition.CompositionV1)}, nil
}

func (h *HostV1) Validate(ctx context.Context, request contract.ValidateRequestV1) (contract.ValidateResultV1, error) {
	if err := request.Config.Validate(); err != nil {
		return contract.ValidateResultV1{}, err
	}
	digest, err := request.Config.DigestV1()
	if err != nil {
		return contract.ValidateResultV1{}, err
	}
	definition, err := safeDecodeV1(ctx, h.config.Decoder, request.Config.CanonicalV1())
	if err != nil {
		return contract.ValidateResultV1{}, err
	}
	if err := definition.Validate(); err != nil {
		return contract.ValidateResultV1{}, err
	}
	return contract.ValidateResultV1{Definition: definition, ConfigDigest: digest}, nil
}

func (h *HostV1) Assemble(ctx context.Context, request contract.AssembleRequestV1) (contract.AssembleResultV1, error) {
	validated, err := h.Validate(ctx, contract.ValidateRequestV1{Config: request.Config})
	if err != nil {
		return contract.AssembleResultV1{}, err
	}
	resolved, err := safeResolveV1(ctx, h.config.Assembler, request.Config.CanonicalV1(), validated.Definition)
	if err != nil {
		return contract.AssembleResultV1{}, err
	}
	if err := resolved.Validate(); err != nil {
		return contract.AssembleResultV1{}, err
	}
	compiled, err := safeCompileV1(ctx, h.config.Compiler, request.Config.CanonicalV1(), resolved)
	if err != nil {
		return contract.AssembleResultV1{}, err
	}
	if err := compiled.Validate(); err != nil {
		return contract.AssembleResultV1{}, err
	}
	return contract.AssembleResultV1{Definition: validated.Definition, Resolved: resolved, Compiled: compiled}, nil
}

func (h *HostV1) Start(ctx context.Context, request contract.StartRequestV1) (contract.StartResultV1, error) {
	unlock := h.acquireStartV1(activeKeyV1(request.Config.HostID, request.StartID))
	defer unlock()
	if err := request.Config.Validate(); err != nil {
		return contract.StartResultV1{}, err
	}
	if err := contract.ValidateIdentifierV1("start id", request.StartID); err != nil {
		return contract.StartResultV1{}, err
	}
	config := request.Config.CanonicalV1()
	configDigest, err := config.DigestV1()
	if err != nil {
		return contract.StartResultV1{}, err
	}
	current, err := h.config.Journal.EnsureAcceptedV1(ctx, config.HostID, request.StartID, configDigest)
	if err != nil {
		return contract.StartResultV1{}, err
	}
	if current.Phase == contract.HostClosedV1 {
		return contract.StartResultV1{}, contract.NewError(contract.ErrorPrecondition, "start_closed", "start lifecycle is already closed")
	}
	if current.Phase == contract.HostIndeterminateV1 || current.Phase == contract.HostDrainingV1 || current.Phase == contract.HostReconcilingV1 {
		return contract.StartResultV1{}, contract.NewError(contract.ErrorPrecondition, "start_not_resumable", "start lifecycle requires stop/reconciliation")
	}

	validated, err := h.Validate(ctx, contract.ValidateRequestV1{Config: config})
	if err != nil {
		return contract.StartResultV1{}, err
	}
	if current.DefinitionRef != nil && !contract.SameExactRefV1(*current.DefinitionRef, validated.Definition.Ref) {
		return contract.StartResultV1{}, contract.NewError(contract.ErrorConflict, "definition_drift", "recovered definition ref drifted")
	}
	if current.Phase == contract.HostAcceptedV1 {
		current, err = h.advanceV1(ctx, current, contract.HostValidatingV1, func(next *contract.HostJournalV1) { next.DefinitionRef = refPtrV1(validated.Definition.Ref) })
		if err != nil {
			return contract.StartResultV1{}, err
		}
	}

	resolved, err := safeResolveV1(ctx, h.config.Assembler, config, validated.Definition)
	if err != nil {
		return contract.StartResultV1{}, err
	}
	if err := resolved.Validate(); err != nil {
		return contract.StartResultV1{}, err
	}
	if current.PlanRef != nil && !contract.SameExactRefV1(*current.PlanRef, resolved.PlanRef) {
		return contract.StartResultV1{}, contract.NewError(contract.ErrorConflict, "plan_drift", "recovered plan ref drifted")
	}
	if current.Phase == contract.HostValidatingV1 {
		current, err = h.advanceV1(ctx, current, contract.HostResolvingV1, func(next *contract.HostJournalV1) { next.PlanRef = refPtrV1(resolved.PlanRef) })
		if err != nil {
			return contract.StartResultV1{}, err
		}
	}

	compiled, err := safeCompileV1(ctx, h.config.Compiler, config, resolved)
	if err != nil {
		return contract.StartResultV1{}, err
	}
	if err := compiled.Validate(); err != nil {
		return contract.StartResultV1{}, err
	}
	for _, pair := range [][2]contract.ExactRefV1{{valueRefV1(current.GenerationRef), compiled.GenerationRef}, {valueRefV1(current.HandoffRef), compiled.HandoffRef}, {valueRefV1(current.GraphRef), compiled.Graph.GraphRef}} {
		if pair[0] != (contract.ExactRefV1{}) && !contract.SameExactRefV1(pair[0], pair[1]) {
			return contract.StartResultV1{}, contract.NewError(contract.ErrorConflict, "assembly_drift", "recovered assembly ref drifted")
		}
	}
	if current.Phase == contract.HostResolvingV1 {
		current, err = h.advanceV1(ctx, current, contract.HostCompilingV1, func(next *contract.HostJournalV1) {
			next.GenerationRef = refPtrV1(compiled.GenerationRef)
			next.HandoffRef = refPtrV1(compiled.HandoffRef)
			next.GraphRef = refPtrV1(compiled.Graph.GraphRef)
		})
		if err != nil {
			return contract.StartResultV1{}, err
		}
	}

	desiredBindingAttempt, err := contract.NewBindingAttemptV1(config.HostID, request.StartID, configDigest, validated.Definition, resolved, compiled)
	if err != nil {
		return contract.StartResultV1{}, err
	}
	if current.Phase == contract.HostCompilingV1 {
		current, err = h.advanceV1(ctx, current, contract.HostBindingV1, func(next *contract.HostJournalV1) { next.BindingAttempt = bindingAttemptPtrV1(desiredBindingAttempt) })
		if err != nil {
			return contract.StartResultV1{}, err
		}
	}
	if current.BindingAttempt == nil || current.BindingAttempt.AttemptID != desiredBindingAttempt.AttemptID || current.BindingAttempt.RequestDigest != desiredBindingAttempt.RequestDigest {
		return contract.StartResultV1{}, contract.NewError(contract.ErrorConflict, "binding_attempt_drift", "recovered binding attempt differs from exact request")
	}
	bindingRequest := ports.BindingRequestV1{HostID: config.HostID, StartID: request.StartID, ConfigDigest: configDigest, Attempt: *current.BindingAttempt, Config: config, Definition: validated.Definition, Resolved: resolved, Compiled: compiled}
	var binding contract.ExactRefV1
	if current.BindingAttempt.State == contract.AttemptBoundV1 {
		binding, err = safeInspectBindingV1(ctx, h.config.Bindings, bindingRequest)
	} else if current.BindingAttempt.State == contract.AttemptUnknownV1 {
		binding, err = safeInspectBindingV1(ctx, h.config.Bindings, bindingRequest)
	} else {
		binding, err = safeStartBindingV1(ctx, h.config.Bindings, bindingRequest)
		if err != nil {
			binding, err = safeInspectBindingV1(context.WithoutCancel(ctx), h.config.Bindings, bindingRequest)
		}
	}
	if err != nil {
		if current.BindingAttempt.State == contract.AttemptPlannedV1 {
			unknown := *current.BindingAttempt
			unknown.State = contract.AttemptUnknownV1
			unknown.Reason = "binding outcome unavailable"
			unknown.BindingRef = nil
			unknown, _ = contract.SealBindingAttemptV1(unknown)
			previous := current
			advanced, persistErr := h.advanceV1(context.WithoutCancel(ctx), previous, contract.HostBindingV1, func(next *contract.HostJournalV1) { next.BindingAttempt = bindingAttemptPtrV1(unknown) })
			if persistErr == nil {
				current = advanced
			} else {
				recovered, inspectErr := h.config.Journal.InspectV1(context.WithoutCancel(ctx), previous.HostID, previous.StartID)
				if inspectErr == nil && recovered.Revision == previous.Revision+1 && recovered.Phase == contract.HostBindingV1 && recovered.BindingAttempt != nil && recovered.BindingAttempt.Digest == unknown.Digest {
					current = recovered
					persistErr = nil
				} else {
					persistErr = errors.Join(persistErr, inspectErr, contract.NewError(contract.ErrorUnknownOutcome, "unknown_progress_not_persisted", "binding unknown state was not proven durable"))
				}
			}
			if persistErr != nil {
				return contract.StartResultV1{}, errors.Join(err, persistErr)
			}
		}
		return contract.StartResultV1{}, err
	}
	if err := binding.Validate(); err != nil {
		return contract.StartResultV1{}, err
	}
	if current.BindingRef != nil && !contract.SameExactRefV1(*current.BindingRef, binding) {
		return contract.StartResultV1{}, contract.NewError(contract.ErrorConflict, "binding_drift", "binding inspection returned another ref")
	}
	if current.BindingAttempt.State != contract.AttemptBoundV1 {
		bound := *current.BindingAttempt
		bound.State = contract.AttemptBoundV1
		bound.BindingRef = refPtrV1(binding)
		bound.Reason = ""
		bound, _ = contract.SealBindingAttemptV1(bound)
		current, err = h.advanceV1(ctx, current, contract.HostBindingV1, func(next *contract.HostJournalV1) {
			next.BindingAttempt = bindingAttemptPtrV1(bound)
			next.BindingRef = refPtrV1(binding)
		})
		if err != nil {
			return contract.StartResultV1{}, err
		}
	}
	if current.Phase == contract.HostBindingV1 {
		current, err = h.advanceV1(ctx, current, contract.HostConstructingV1, nil)
		if err != nil {
			return contract.StartResultV1{}, err
		}
	}

	key := activeKeyV1(config.HostID, request.StartID)
	if current.Phase == contract.HostConstructingV1 {
		var composed *composition.CompositionV1
		composed, cleanup, constructErr := h.config.Composer.ConstructV1(ctx, config.HostID, request.StartID, compiled.Graph, current.ConstructionAttempts, current.Constructed, func(attempts []contract.ConstructionAttemptV1, values []contract.ConstructedComponentV1) error {
			previous := current
			advanced, advanceErr := h.advanceV1(ctx, previous, contract.HostConstructingV1, func(next *contract.HostJournalV1) {
				next.ConstructionAttempts = append([]contract.ConstructionAttemptV1(nil), attempts...)
				next.Constructed = append([]contract.ConstructedComponentV1(nil), values...)
			})
			if advanceErr == nil {
				current = advanced
				return nil
			}
			recovered, inspectErr := h.config.Journal.InspectV1(context.WithoutCancel(ctx), previous.HostID, previous.StartID)
			if inspectErr == nil && sameConstructionProgressV1(previous, recovered, attempts, values) {
				current = recovered
				if containsUnknownAttemptV1(attempts) {
					return nil
				}
			} else if inspectErr == nil {
				inspectErr = contract.NewError(contract.ErrorConflict, "construction_progress_recovery_drift", "journal recovery returned another construction progress")
			}
			if inspectErr != nil {
				return errors.Join(advanceErr, inspectErr)
			}
			return advanceErr
		})
		if constructErr != nil {
			finishErr := h.finishFailedConstructionV1(context.WithoutCancel(ctx), current, cleanup)
			return contract.StartResultV1{}, errors.Join(constructErr, finishErr)
		}
		h.setActiveV1(key, composed)
		current, err = h.advanceV1(ctx, current, contract.HostVerifyingV1, nil)
		if err != nil {
			return contract.StartResultV1{}, err
		}
	}
	if h.getActiveV1(key) == nil {
		composed, attachErr := h.config.Composer.ReattachV1(ctx, current.ConstructionAttempts, current.Constructed)
		if attachErr != nil {
			return contract.StartResultV1{}, attachErr
		}
		h.setActiveV1(key, composed)
	}

	if current.Phase == contract.HostVerifyingV1 {
		ready, verifyErr := safeVerifyReadyV1(ctx, h.config.Readiness, ports.ReadinessRequestV1{HostID: config.HostID, StartID: request.StartID, Definition: validated.Definition, Resolved: resolved, Compiled: compiled, BindingRef: binding, Components: append([]contract.ConstructedComponentV1(nil), current.Constructed...)})
		if verifyErr != nil {
			return contract.StartResultV1{}, verifyErr
		}
		if verifyErr = h.validateReadyV1(ready, current, validated, resolved, compiled, binding); verifyErr != nil {
			return contract.StartResultV1{}, verifyErr
		}
		readyRef, refErr := ready.RefV1()
		if refErr != nil {
			return contract.StartResultV1{}, refErr
		}
		current, err = h.advanceV1(ctx, current, contract.HostReadyV1, func(next *contract.HostJournalV1) { next.ReadyRef = refPtrV1(readyRef) })
		if err != nil {
			return contract.StartResultV1{}, err
		}
		journalRef, _ := current.RefV1()
		return contract.StartResultV1{Journal: journalRef, Ready: ready}, nil
	}
	if current.Phase != contract.HostReadyV1 || current.ReadyRef == nil {
		return contract.StartResultV1{}, contract.NewError(contract.ErrorPrecondition, "start_phase_invalid", "start did not converge to ready")
	}
	ready, err := safeInspectReadyV1(ctx, h.config.Readiness, *current.ReadyRef)
	if err != nil {
		return contract.StartResultV1{}, err
	}
	if err := h.validateReadyV1(ready, current, validated, resolved, compiled, binding); err != nil {
		return contract.StartResultV1{}, err
	}
	journalRef, _ := current.RefV1()
	return contract.StartResultV1{Journal: journalRef, Ready: ready}, nil
}

func (h *HostV1) Inspect(ctx context.Context, request contract.InspectRequestV1) (contract.InspectResultV1, error) {
	if err := contract.ValidateIdentifierV1("host id", request.HostID); err != nil {
		return contract.InspectResultV1{}, err
	}
	if err := contract.ValidateIdentifierV1("start id", request.StartID); err != nil {
		return contract.InspectResultV1{}, err
	}
	unlock := h.acquireStartV1(activeKeyV1(request.HostID, request.StartID))
	defer unlock()
	current, err := h.config.Journal.InspectV1(ctx, request.HostID, request.StartID)
	if err != nil {
		return contract.InspectResultV1{}, err
	}
	result := contract.InspectResultV1{Journal: current}
	if current.Phase == contract.HostReadyV1 && current.ReadyRef != nil {
		ready, inspectErr := safeInspectReadyV1(ctx, h.config.Readiness, *current.ReadyRef)
		if inspectErr != nil {
			return contract.InspectResultV1{}, inspectErr
		}
		now, clockErr := safeClockV1(h.config.Clock)
		if clockErr != nil {
			return contract.InspectResultV1{}, clockErr
		}
		if now.UnixNano() < current.UpdatedUnixNano {
			return contract.InspectResultV1{}, contract.NewError(contract.ErrorPrecondition, "clock_regression", "ready inspection clock is behind the journal watermark")
		}
		if inspectErr = ready.Validate(now); inspectErr != nil {
			return contract.InspectResultV1{}, inspectErr
		}
		actual, inspectErr := ready.RefV1()
		if inspectErr != nil {
			return contract.InspectResultV1{}, inspectErr
		}
		if !contract.SameExactRefV1(actual, *current.ReadyRef) || ready.HostID != current.HostID || ready.StartID != current.StartID {
			return contract.InspectResultV1{}, contract.NewError(contract.ErrorConflict, "ready_ref_drift", "ready inspection returned another exact projection")
		}
		result.Ready = &ready
	}
	return result, nil
}

func (h *HostV1) Stop(ctx context.Context, request contract.StopRequestV1) (contract.StopResultV1, error) {
	if err := contract.ValidateIdentifierV1("host id", request.HostID); err != nil {
		return contract.StopResultV1{}, err
	}
	if err := contract.ValidateIdentifierV1("start id", request.StartID); err != nil {
		return contract.StopResultV1{}, err
	}
	unlock := h.acquireStartV1(activeKeyV1(request.HostID, request.StartID))
	defer unlock()
	current, err := h.config.Journal.InspectV1(ctx, request.HostID, request.StartID)
	if err != nil {
		return contract.StopResultV1{}, err
	}
	if current.Phase == contract.HostClosedV1 {
		ref, _ := current.RefV1()
		return contract.StopResultV1{Journal: ref, Cleanup: contract.CleanupSummaryV1{State: contract.CleanupClosedV1}}, nil
	}
	if current.Phase == contract.HostIndeterminateV1 {
		current, err = h.advanceV1(ctx, current, contract.HostReconcilingV1, nil)
		if err != nil {
			return contract.StopResultV1{}, err
		}
	} else if current.Phase != contract.HostDrainingV1 && current.Phase != contract.HostReconcilingV1 {
		current, err = h.advanceV1(ctx, current, contract.HostDrainingV1, nil)
		if err != nil {
			return contract.StopResultV1{}, err
		}
	}
	key := activeKeyV1(request.HostID, request.StartID)
	composed := h.getActiveV1(key)
	if composed == nil {
		composed, err = h.config.Composer.ReattachV1(ctx, current.ConstructionAttempts, current.Constructed)
		if err != nil {
			_ = h.markIndeterminateV1(context.WithoutCancel(ctx), current)
			return contract.StopResultV1{}, err
		}
	}
	cleanup, cleanupErr := composed.CleanupV1(ctx)
	h.deleteActiveV1(key)
	if current.Phase == contract.HostDrainingV1 {
		current, err = h.advanceV1(context.WithoutCancel(ctx), current, contract.HostReconcilingV1, nil)
		if err != nil {
			return contract.StopResultV1{}, err
		}
	}
	final := contract.HostClosedV1
	if cleanup.State != contract.CleanupClosedV1 || cleanupErr != nil || current.HasUnknownAttemptsV1() {
		final = contract.HostIndeterminateV1
	}
	current, err = h.advanceV1(context.WithoutCancel(ctx), current, final, nil)
	if err != nil {
		return contract.StopResultV1{}, err
	}
	ref, _ := current.RefV1()
	if cleanupErr != nil {
		return contract.StopResultV1{Journal: ref, Cleanup: cleanup}, cleanupErr
	}
	return contract.StopResultV1{Journal: ref, Cleanup: cleanup}, nil
}

func (h *HostV1) advanceV1(ctx context.Context, current contract.HostJournalV1, phase contract.HostPhaseV1, mutate func(*contract.HostJournalV1)) (contract.HostJournalV1, error) {
	next := current
	next.Revision++
	next.Phase = phase
	clock, clockErr := safeClockV1(h.config.Clock)
	if clockErr != nil {
		return contract.HostJournalV1{}, clockErr
	}
	now := clock.UnixNano()
	if now <= current.UpdatedUnixNano {
		return contract.HostJournalV1{}, contract.NewError(contract.ErrorPrecondition, "clock_regression", "host clock must advance strictly beyond the journal watermark")
	}
	next.UpdatedUnixNano = now
	if mutate != nil {
		mutate(&next)
	}
	sealed, err := contract.SealHostJournalV1(next)
	if err != nil {
		return contract.HostJournalV1{}, err
	}
	return h.config.Journal.AdvanceV1(ctx, current, sealed)
}

func (h *HostV1) validateReadyV1(ready contract.SystemReadyV1, current contract.HostJournalV1, definition contract.ValidateResultV1, resolved contract.ResolvedAssemblyV1, compiled contract.CompiledAssemblyV1, binding contract.ExactRefV1) error {
	now, err := safeClockV1(h.config.Clock)
	if err != nil {
		return err
	}
	if now.UnixNano() < current.UpdatedUnixNano {
		return contract.NewError(contract.ErrorPrecondition, "clock_regression", "ready validation clock is behind the journal watermark")
	}
	if err := ready.Validate(now); err != nil {
		return err
	}
	if ready.HostID != current.HostID || ready.StartID != current.StartID || !contract.SameExactRefV1(ready.DefinitionRef, definition.Definition.Ref) || !contract.SameExactRefV1(ready.PlanRef, resolved.PlanRef) || !contract.SameExactRefV1(ready.GenerationRef, compiled.GenerationRef) || !contract.SameExactRefV1(ready.HandoffRef, compiled.HandoffRef) || !contract.SameExactRefV1(ready.BindingRef, binding) || !sameComponentsV1(ready.Components, current.Constructed) {
		return contract.NewError(contract.ErrorConflict, "ready_chain_drift", "ready projection does not bind the exact host chain")
	}
	if current.ReadyRef != nil {
		actual, err := ready.RefV1()
		if err != nil {
			return err
		}
		if !contract.SameExactRefV1(actual, *current.ReadyRef) {
			return contract.NewError(contract.ErrorConflict, "ready_ref_drift", "ready projection does not match exact journal ref")
		}
	}
	return nil
}

func (h *HostV1) finishFailedConstructionV1(ctx context.Context, current contract.HostJournalV1, cleanup contract.CleanupSummaryV1) error {
	var err error
	current, err = h.advanceV1(ctx, current, contract.HostDrainingV1, nil)
	if err != nil {
		return err
	}
	current, err = h.advanceV1(ctx, current, contract.HostReconcilingV1, nil)
	if err != nil {
		return err
	}
	final := contract.HostClosedV1
	if cleanup.State != contract.CleanupClosedV1 || current.HasUnknownAttemptsV1() {
		final = contract.HostIndeterminateV1
	}
	_, err = h.advanceV1(ctx, current, final, nil)
	return err
}
func (h *HostV1) markIndeterminateV1(ctx context.Context, current contract.HostJournalV1) error {
	if current.Phase == contract.HostIndeterminateV1 {
		return nil
	}
	_, err := h.advanceV1(ctx, current, contract.HostIndeterminateV1, nil)
	return err
}
func sameComponentsV1(a, b []contract.ConstructedComponentV1) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
func refPtrV1(value contract.ExactRefV1) *contract.ExactRefV1 { clone := value; return &clone }
func valueRefV1(value *contract.ExactRefV1) contract.ExactRefV1 {
	if value == nil {
		return contract.ExactRefV1{}
	}
	return *value
}
func activeKeyV1(hostID, startID string) string { return hostID + "\x00" + startID }

func sameConstructionProgressV1(previous, recovered contract.HostJournalV1, attempts []contract.ConstructionAttemptV1, components []contract.ConstructedComponentV1) bool {
	if recovered.HostID != previous.HostID || recovered.StartID != previous.StartID || recovered.Revision != previous.Revision+1 || recovered.Phase != contract.HostConstructingV1 {
		return false
	}
	wantAttempts, err := contract.DigestJSONV1(attempts)
	if err != nil {
		return false
	}
	gotAttempts, err := contract.DigestJSONV1(recovered.ConstructionAttempts)
	if err != nil || wantAttempts != gotAttempts {
		return false
	}
	wantComponents, err := contract.DigestJSONV1(components)
	if err != nil {
		return false
	}
	gotComponents, err := contract.DigestJSONV1(recovered.Constructed)
	return err == nil && wantComponents == gotComponents
}
func containsUnknownAttemptV1(attempts []contract.ConstructionAttemptV1) bool {
	for _, attempt := range attempts {
		if attempt.State == contract.AttemptUnknownV1 {
			return true
		}
	}
	return false
}

func safeDecodeV1(ctx context.Context, port ports.DefinitionDecoderV1, config contract.HostConfigV1) (result contract.DecodedDefinitionV1, err error) {
	defer func() {
		if recover() != nil {
			result = contract.DecodedDefinitionV1{}
			err = contract.NewError(contract.ErrorUnavailable, "decoder_panic", "definition decoder panicked")
		}
	}()
	return port.DecodeDefinitionV1(ctx, config)
}
func safeResolveV1(ctx context.Context, port ports.AgentAssemblerV1, config contract.HostConfigV1, definition contract.DecodedDefinitionV1) (result contract.ResolvedAssemblyV1, err error) {
	defer func() {
		if recover() != nil {
			result = contract.ResolvedAssemblyV1{}
			err = contract.NewError(contract.ErrorUnavailable, "assembler_panic", "agent assembler panicked")
		}
	}()
	return port.ResolveAgentV1(ctx, config, definition)
}
func safeCompileV1(ctx context.Context, port ports.HarnessCompilerV1, config contract.HostConfigV1, resolved contract.ResolvedAssemblyV1) (result contract.CompiledAssemblyV1, err error) {
	defer func() {
		if recover() != nil {
			result = contract.CompiledAssemblyV1{}
			err = contract.NewError(contract.ErrorUnavailable, "compiler_panic", "harness compiler panicked")
		}
	}()
	return port.CompileHarnessV1(ctx, config, resolved)
}
func safeStartBindingV1(ctx context.Context, port ports.BindingPortV1, request ports.BindingRequestV1) (result contract.ExactRefV1, err error) {
	defer func() {
		if recover() != nil {
			result = contract.ExactRefV1{}
			err = contract.NewError(contract.ErrorUnknownOutcome, "binding_start_panic", "binding start-or-inspect panicked")
		}
	}()
	return port.StartOrInspectBindingV1(ctx, request)
}
func safeInspectBindingV1(ctx context.Context, port ports.BindingPortV1, request ports.BindingRequestV1) (result contract.ExactRefV1, err error) {
	defer func() {
		if recover() != nil {
			result = contract.ExactRefV1{}
			err = contract.NewError(contract.ErrorUnavailable, "binding_inspect_panic", "binding inspection panicked")
		}
	}()
	return port.InspectBindingV1(ctx, request)
}
func safeVerifyReadyV1(ctx context.Context, port ports.ReadinessPortV1, request ports.ReadinessRequestV1) (result contract.SystemReadyV1, err error) {
	defer func() {
		if recover() != nil {
			result = contract.SystemReadyV1{}
			err = contract.NewError(contract.ErrorUnavailable, "readiness_verify_panic", "readiness verification panicked")
		}
	}()
	return port.VerifySystemReadyV1(ctx, request)
}
func safeInspectReadyV1(ctx context.Context, port ports.ReadinessPortV1, ref contract.ExactRefV1) (result contract.SystemReadyV1, err error) {
	defer func() {
		if recover() != nil {
			result = contract.SystemReadyV1{}
			err = contract.NewError(contract.ErrorUnavailable, "readiness_inspect_panic", "readiness inspection panicked")
		}
	}()
	return port.InspectSystemReadyV1(ctx, ref)
}
func safeClockV1(clock func() time.Time) (result time.Time, err error) {
	defer func() {
		if recover() != nil {
			result = time.Time{}
			err = contract.NewError(contract.ErrorUnavailable, "clock_panic", "host clock panicked")
		}
	}()
	result = clock()
	if result.IsZero() {
		return time.Time{}, contract.NewError(contract.ErrorUnavailable, "clock_unavailable", "host clock returned zero")
	}
	return result, nil
}

func (h *HostV1) acquireStartV1(key string) func() {
	h.locksMu.Lock()
	item := h.locks[key]
	if item == nil {
		item = &startLockV1{}
		h.locks[key] = item
	}
	item.refs++
	h.locksMu.Unlock()
	item.mu.Lock()
	return func() {
		item.mu.Unlock()
		h.locksMu.Lock()
		item.refs--
		if item.refs == 0 {
			delete(h.locks, key)
		}
		h.locksMu.Unlock()
	}
}
func (h *HostV1) getActiveV1(key string) *composition.CompositionV1 {
	h.activeMu.Lock()
	defer h.activeMu.Unlock()
	return h.active[key]
}
func (h *HostV1) setActiveV1(key string, value *composition.CompositionV1) {
	h.activeMu.Lock()
	h.active[key] = value
	h.activeMu.Unlock()
}
func (h *HostV1) deleteActiveV1(key string) {
	h.activeMu.Lock()
	delete(h.active, key)
	h.activeMu.Unlock()
}
func bindingAttemptPtrV1(value contract.BindingAttemptV1) *contract.BindingAttemptV1 {
	clone := value
	return &clone
}
