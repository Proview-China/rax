package action

import (
	"context"
	"reflect"
	"strconv"
	"sync"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

type CoordinationStoreV1 struct {
	mu          sync.RWMutex
	current     map[string]contract.SingleCallToolActionCoordinationWatermarkV1
	history     map[boundaryHistoryKeyV1]contract.SingleCallToolActionCoordinationWatermarkV1
	commands    map[string]contract.SingleCallCanonicalCommandV1
	model       modelinvoker.ToolCallCandidateObservationProjectionReaderV1
	enforcement runtimeports.OperationProviderExecuteEnforcementCurrentReaderV1
	handoff     runtimeports.OperationProviderEvidenceHandoffCurrentReaderV1
	owner       runtimeports.EffectOwnerRefV2
}

type boundaryHistoryKeyV1 struct {
	id       string
	revision core.Revision
	digest   core.Digest
}

func NewCoordinationStoreV1(model modelinvoker.ToolCallCandidateObservationProjectionReaderV1, enforcement runtimeports.OperationProviderExecuteEnforcementCurrentReaderV1, handoff runtimeports.OperationProviderEvidenceHandoffCurrentReaderV1, owner runtimeports.EffectOwnerRefV2) *CoordinationStoreV1 {
	return &CoordinationStoreV1{current: make(map[string]contract.SingleCallToolActionCoordinationWatermarkV1), history: make(map[boundaryHistoryKeyV1]contract.SingleCallToolActionCoordinationWatermarkV1), commands: make(map[string]contract.SingleCallCanonicalCommandV1), model: model, enforcement: enforcement, handoff: handoff, owner: owner}
}

func (s *CoordinationStoreV1) StartOrInspectV1(ctx context.Context, command contract.SingleCallCanonicalCommandV1, now, expires time.Time) (contract.SingleCallToolActionCoordinationWatermarkV1, error) {
	if err := command.Validate(); err != nil {
		return contract.SingleCallToolActionCoordinationWatermarkV1{}, err
	}
	if s.model == nil {
		return contract.SingleCallToolActionCoordinationWatermarkV1{}, unavailableV2("Model exact Projection reader is unavailable")
	}
	projection, err := s.model.InspectExactProjectionV1(ctx, command.ModelProjection)
	if err != nil {
		return contract.SingleCallToolActionCoordinationWatermarkV1{}, err
	}
	if err = projection.Validate(); err != nil {
		return contract.SingleCallToolActionCoordinationWatermarkV1{}, err
	}
	if !reflect.DeepEqual(projection.Ref, command.ModelProjection) || projection.Observation.Digest != command.ObservationDigest || len(projection.Observation.Calls) != 1 {
		return contract.SingleCallToolActionCoordinationWatermarkV1{}, conflictV2("Model Projection exact ref, digest or N=1 cardinality drifted")
	}
	call := projection.Observation.Calls[0]
	if call.CallID != command.CallID || call.Name != command.CallName || core.DigestBytes(call.CanonicalArguments) != command.CanonicalArgumentsDigest {
		return contract.SingleCallToolActionCoordinationWatermarkV1{}, conflictV2("canonical command differs from the exact Model call")
	}
	if now.IsZero() || expires.IsZero() || !expires.After(now) {
		return contract.SingleCallToolActionCoordinationWatermarkV1{}, invalidV2("watermark current window is invalid")
	}
	commandDigest, err := command.DigestV1()
	if err != nil {
		return contract.SingleCallToolActionCoordinationWatermarkV1{}, err
	}
	id, err := contract.StableID("tool-watermark", string(command.TenantID), command.ApplicationRequestID, strconv.FormatUint(uint64(command.ApplicationRequestRevision), 10), string(command.OperationScopeDigest))
	if err != nil {
		return contract.SingleCallToolActionCoordinationWatermarkV1{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.current[id]; ok {
		stored, commandOK := s.commands[id]
		if commandOK && existing.ApplicationRequestDigest == command.ApplicationRequestDigest && existing.OperationScopeDigest == command.OperationScopeDigest && existing.CanonicalCommandDigest == commandDigest && reflect.DeepEqual(existing.ModelProjection, command.ModelProjection) && reflect.DeepEqual(stored, command) {
			return cloneWatermarkV1(existing), nil
		}
		return contract.SingleCallToolActionCoordinationWatermarkV1{}, conflictV2("Application request stable key binds another canonical command")
	}
	w, err := contract.SealCoordinationWatermarkV1(contract.SingleCallToolActionCoordinationWatermarkV1{ID: id, Revision: 1, TenantID: command.TenantID, ApplicationRequestID: command.ApplicationRequestID, ApplicationRequestRevision: command.ApplicationRequestRevision, ApplicationRequestDigest: command.ApplicationRequestDigest, OperationScopeDigest: command.OperationScopeDigest, ModelProjection: command.ModelProjection, ObservationDigest: command.ObservationDigest, CanonicalCommandDigest: commandDigest, Stage: contract.CoordinationRequestRecordedV1, Owner: s.owner, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano(), ExpiresUnixNano: expires.UnixNano()})
	if err != nil {
		return contract.SingleCallToolActionCoordinationWatermarkV1{}, err
	}
	s.current[id] = cloneWatermarkV1(w)
	s.commands[id] = command
	s.remember(w)
	return cloneWatermarkV1(w), nil
}

func (s *CoordinationStoreV1) InspectCanonicalCommandV1(id string, exactDigest core.Digest) (contract.SingleCallCanonicalCommandV1, error) {
	if contract.ValidateStableID(id) != nil || exactDigest.Validate() != nil {
		return contract.SingleCallCanonicalCommandV1{}, invalidV2("canonical command exact key is required")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	command, ok := s.commands[id]
	if !ok {
		return contract.SingleCallCanonicalCommandV1{}, notFoundV2("canonical command not found")
	}
	digest, err := command.DigestV1()
	if err != nil {
		return contract.SingleCallCanonicalCommandV1{}, err
	}
	if digest != exactDigest {
		return contract.SingleCallCanonicalCommandV1{}, conflictV2("canonical command exact digest drifted")
	}
	return command, nil
}

func (s *CoordinationStoreV1) InspectWatermarkV1(id string, revision core.Revision, digest core.Digest) (contract.SingleCallToolActionCoordinationWatermarkV1, error) {
	if contract.ValidateStableID(id) != nil || revision == 0 || digest.Validate() != nil {
		return contract.SingleCallToolActionCoordinationWatermarkV1{}, invalidV2("exact watermark key is required")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	w, ok := s.history[boundaryHistoryKeyV1{id, revision, digest}]
	if !ok {
		return contract.SingleCallToolActionCoordinationWatermarkV1{}, notFoundV2("watermark exact revision not found")
	}
	return cloneWatermarkV1(w), nil
}

// InspectCanonicalCurrentV1 returns only the current watermark for the exact
// canonical command. Unavailable/expired/drift never becomes NotFound.
func (s *CoordinationStoreV1) InspectCanonicalCurrentV1(id string, canonicalDigest core.Digest, now time.Time) (contract.SingleCallToolActionCoordinationWatermarkV1, error) {
	if contract.ValidateStableID(id) != nil || canonicalDigest.Validate() != nil || now.IsZero() {
		return contract.SingleCallToolActionCoordinationWatermarkV1{}, invalidV2("watermark ID, canonical digest and current time are required")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	w, ok := s.current[id]
	if !ok {
		return contract.SingleCallToolActionCoordinationWatermarkV1{}, notFoundV2("current watermark not found")
	}
	if w.CanonicalCommandDigest != canonicalDigest {
		return contract.SingleCallToolActionCoordinationWatermarkV1{}, conflictV2("canonical command digest drifted")
	}
	if !contract.IsCoordinationCurrentV1(w, now) {
		return contract.SingleCallToolActionCoordinationWatermarkV1{}, expiredV2("watermark is not current")
	}
	return cloneWatermarkV1(w), nil
}

func (s *CoordinationStoreV1) BindCandidateV1(exact contract.ToolProviderBoundarySourceRefV1, candidate contract.ObjectRef, now time.Time) (contract.SingleCallToolActionCoordinationWatermarkV1, error) {
	return s.advanceToolRef(exact, candidate, contract.CoordinationRequestRecordedV1, contract.CoordinationCandidateRecordedV1, now, func(w *contract.SingleCallToolActionCoordinationWatermarkV1, r contract.ObjectRef) {
		w.ActionCandidate = &r
	})
}
func (s *CoordinationStoreV1) BindReservationV1(exact contract.ToolProviderBoundarySourceRefV1, reservation contract.ObjectRef, now time.Time) (contract.SingleCallToolActionCoordinationWatermarkV1, error) {
	return s.advanceToolRef(exact, reservation, contract.CoordinationCandidateRecordedV1, contract.CoordinationReservationRecordedV1, now, func(w *contract.SingleCallToolActionCoordinationWatermarkV1, r contract.ObjectRef) {
		w.Reservation = &r
	})
}

func (s *CoordinationStoreV1) BindRuntimeAttemptV1(exact contract.ToolProviderBoundarySourceRefV1, operation runtimeports.OperationSubjectV3, attempt runtimeports.OperationDispatchAttemptRefV3, now time.Time) (contract.SingleCallToolActionCoordinationWatermarkV1, error) {
	if operation.Validate() != nil || attempt.Validate() != nil {
		return contract.SingleCallToolActionCoordinationWatermarkV1{}, invalidV2("exact Runtime operation and attempt are required")
	}
	od, err := operation.DigestV3()
	if err != nil || attempt.OperationDigest != od {
		return contract.SingleCallToolActionCoordinationWatermarkV1{}, conflictV2("Runtime Attempt belongs to another operation")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	w, err := s.currentForCAS(exact, contract.CoordinationReservationRecordedV1, now)
	if err != nil {
		return contract.SingleCallToolActionCoordinationWatermarkV1{}, err
	}
	w.RuntimeAttempt = &attempt
	w.Operation = &operation
	w.OperationDigest = od
	return s.commitAdvance(w, contract.CoordinationRuntimeAttemptBoundV1, now)
}

func (s *CoordinationStoreV1) CrossProviderBoundaryV1(ctx context.Context, exact contract.ToolProviderBoundarySourceRefV1, enforcement runtimeports.OperationDispatchEnforcementPhaseRefV4, handoff runtimeports.OperationScopeEvidenceProviderHandoffRefV3, now time.Time) (contract.ToolProviderBoundarySourceRefV1, error) {
	if s.enforcement == nil || s.handoff == nil {
		return contract.ToolProviderBoundarySourceRefV1{}, unavailableV2("Runtime execute Enforcement/Handoff current readers are unavailable")
	}
	s.mu.RLock()
	snapshot, err := s.currentForRead(exact, contract.CoordinationRuntimeAttemptBoundV1, now)
	s.mu.RUnlock()
	if err != nil {
		return contract.ToolProviderBoundarySourceRefV1{}, err
	}
	actualEnforcement, err := s.enforcement.InspectCurrentOperationProviderExecuteEnforcementV1(ctx, *snapshot.Operation, enforcement)
	if err != nil {
		return contract.ToolProviderBoundarySourceRefV1{}, err
	}
	if !reflect.DeepEqual(actualEnforcement, enforcement) || enforcement.Phase != runtimeports.OperationDispatchEnforcementExecuteV4 || enforcement.AttemptID != snapshot.RuntimeAttempt.AttemptID || enforcement.OperationDigest != snapshot.RuntimeAttempt.OperationDigest || enforcement.EffectID != snapshot.RuntimeAttempt.EffectID {
		return contract.ToolProviderBoundarySourceRefV1{}, conflictV2("execute Enforcement is not current for the exact Runtime Attempt")
	}
	handoffFact, err := s.handoff.InspectCurrentOperationProviderEvidenceHandoffV1(ctx, handoff)
	if err != nil {
		return contract.ToolProviderBoundarySourceRefV1{}, err
	}
	if !reflect.DeepEqual(handoffFact.RefV3(), handoff) || !reflect.DeepEqual(handoffFact.Phase, enforcement) || now.IsZero() || !now.Before(time.Unix(0, handoffFact.NotAfterUnixNano)) {
		return contract.ToolProviderBoundarySourceRefV1{}, conflictV2("execute Evidence Handoff is not current for the exact Enforcement phase")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	w, err := s.currentForCAS(exact, contract.CoordinationRuntimeAttemptBoundV1, now)
	if err != nil {
		return contract.ToolProviderBoundarySourceRefV1{}, err
	}
	w.ExecuteEnforcement = &enforcement
	w.ExecuteHandoff = &handoff
	w.ExpiresUnixNano = contract.MinUnixNanoV1(w.ExpiresUnixNano, enforcement.ExpiresUnixNano, handoffFact.NotAfterUnixNano)
	w, err = s.commitAdvance(w, contract.CoordinationProviderBoundaryV1, now)
	if err != nil {
		return contract.ToolProviderBoundarySourceRefV1{}, err
	}
	return w.BoundarySourceRefV1()
}

func (s *CoordinationStoreV1) RecordProviderObservationV1(exact contract.ToolProviderBoundarySourceRefV1, observation runtimeports.ProviderAttemptObservationRefV2, now time.Time) (contract.SingleCallToolActionCoordinationWatermarkV1, error) {
	if observation.Validate() != nil {
		return contract.SingleCallToolActionCoordinationWatermarkV1{}, invalidV2("exact Provider Observation is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	w, err := s.currentForCAS(exact, contract.CoordinationProviderBoundaryV1, now)
	if err != nil {
		return contract.SingleCallToolActionCoordinationWatermarkV1{}, err
	}
	w.ProviderObservation = &observation
	return s.commitAdvance(w, contract.CoordinationProviderObservedV1, now)
}
func (s *CoordinationStoreV1) BindDomainResultV1(exact contract.ToolProviderBoundarySourceRefV1, result contract.ObjectRef, now time.Time) (contract.SingleCallToolActionCoordinationWatermarkV1, error) {
	return s.advanceToolRef(exact, result, contract.CoordinationProviderObservedV1, contract.CoordinationDomainResultV1, now, func(w *contract.SingleCallToolActionCoordinationWatermarkV1, r contract.ObjectRef) {
		w.DomainResult = &r
	})
}
func (s *CoordinationStoreV1) BindApplyV1(exact contract.ToolProviderBoundarySourceRefV1, apply contract.ObjectRef, now time.Time) (contract.SingleCallToolActionCoordinationWatermarkV1, error) {
	return s.advanceToolRef(exact, apply, contract.CoordinationDomainResultV1, contract.CoordinationSettlementAppliedV1, now, func(w *contract.SingleCallToolActionCoordinationWatermarkV1, r contract.ObjectRef) { w.Apply = &r })
}
func (s *CoordinationStoreV1) BindResultV1(exact contract.ToolProviderBoundarySourceRefV1, result contract.ObjectRef, now time.Time) (contract.SingleCallToolActionCoordinationWatermarkV1, error) {
	return s.advanceToolRef(exact, result, contract.CoordinationSettlementAppliedV1, contract.CoordinationResultSettledV1, now, func(w *contract.SingleCallToolActionCoordinationWatermarkV1, r contract.ObjectRef) { w.Result = &r })
}

func (s *CoordinationStoreV1) InspectBoundarySourceCurrentV1(_ context.Context, exact contract.ToolProviderBoundarySourceRefV1, now time.Time) (contract.SingleCallToolActionCoordinationWatermarkV1, error) {
	if exact.Validate() != nil || now.IsZero() {
		return contract.SingleCallToolActionCoordinationWatermarkV1{}, invalidV2("exact boundary source and current time are required")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	w, ok := s.history[boundaryHistoryKeyV1{exact.WatermarkID, exact.WatermarkRevision, exact.WatermarkDigest}]
	if !ok {
		return contract.SingleCallToolActionCoordinationWatermarkV1{}, notFoundV2("provider boundary source not found")
	}
	if w.Stage != contract.CoordinationProviderBoundaryV1 || !contract.IsCoordinationCurrentV1(w, now) {
		return contract.SingleCallToolActionCoordinationWatermarkV1{}, expiredV2("provider boundary source is not current")
	}
	return cloneWatermarkV1(w), nil
}

func (s *CoordinationStoreV1) advanceToolRef(exact contract.ToolProviderBoundarySourceRefV1, ref contract.ObjectRef, from, to contract.SingleCallCoordinationStageV1, now time.Time, set func(*contract.SingleCallToolActionCoordinationWatermarkV1, contract.ObjectRef)) (contract.SingleCallToolActionCoordinationWatermarkV1, error) {
	if ref.Validate() != nil {
		return contract.SingleCallToolActionCoordinationWatermarkV1{}, invalidV2("exact Tool fact ref is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	w, err := s.currentForCAS(exact, from, now)
	if err != nil {
		return contract.SingleCallToolActionCoordinationWatermarkV1{}, err
	}
	set(&w, ref)
	return s.commitAdvance(w, to, now)
}
func (s *CoordinationStoreV1) currentForRead(exact contract.ToolProviderBoundarySourceRefV1, stage contract.SingleCallCoordinationStageV1, now time.Time) (contract.SingleCallToolActionCoordinationWatermarkV1, error) {
	if exact.Validate() != nil || now.IsZero() {
		return contract.SingleCallToolActionCoordinationWatermarkV1{}, invalidV2("exact watermark CAS key and current time are required")
	}
	w, ok := s.current[exact.WatermarkID]
	if !ok {
		return contract.SingleCallToolActionCoordinationWatermarkV1{}, notFoundV2("watermark not found")
	}
	if w.Revision != exact.WatermarkRevision || w.Digest != exact.WatermarkDigest {
		return contract.SingleCallToolActionCoordinationWatermarkV1{}, conflictV2("watermark CAS key drifted")
	}
	if w.Stage != stage {
		return contract.SingleCallToolActionCoordinationWatermarkV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidTransition, "watermark stage does not permit this transition")
	}
	if !contract.IsCoordinationCurrentV1(w, now) {
		return contract.SingleCallToolActionCoordinationWatermarkV1{}, expiredV2("watermark is expired")
	}
	return cloneWatermarkV1(w), nil
}
func (s *CoordinationStoreV1) currentForCAS(exact contract.ToolProviderBoundarySourceRefV1, stage contract.SingleCallCoordinationStageV1, now time.Time) (contract.SingleCallToolActionCoordinationWatermarkV1, error) {
	return s.currentForRead(exact, stage, now)
}
func (s *CoordinationStoreV1) commitAdvance(w contract.SingleCallToolActionCoordinationWatermarkV1, stage contract.SingleCallCoordinationStageV1, now time.Time) (contract.SingleCallToolActionCoordinationWatermarkV1, error) {
	w.Stage = stage
	w.Revision++
	w.UpdatedUnixNano = now.UnixNano()
	sealed, err := contract.SealCoordinationWatermarkV1(w)
	if err != nil {
		return contract.SingleCallToolActionCoordinationWatermarkV1{}, err
	}
	s.current[w.ID] = cloneWatermarkV1(sealed)
	s.remember(sealed)
	return cloneWatermarkV1(sealed), nil
}
func (s *CoordinationStoreV1) remember(w contract.SingleCallToolActionCoordinationWatermarkV1) {
	s.history[boundaryHistoryKeyV1{w.ID, w.Revision, w.Digest}] = cloneWatermarkV1(w)
}
func cloneWatermarkV1(w contract.SingleCallToolActionCoordinationWatermarkV1) contract.SingleCallToolActionCoordinationWatermarkV1 {
	if w.ActionCandidate != nil {
		x := *w.ActionCandidate
		w.ActionCandidate = &x
	}
	if w.Reservation != nil {
		x := *w.Reservation
		w.Reservation = &x
	}
	if w.RuntimeAttempt != nil {
		x := *w.RuntimeAttempt
		if x.Delegation != nil {
			d := *x.Delegation
			x.Delegation = &d
		}
		w.RuntimeAttempt = &x
	}
	if w.Operation != nil {
		x := *w.Operation
		w.Operation = &x
	}
	if w.ExecuteEnforcement != nil {
		x := *w.ExecuteEnforcement
		w.ExecuteEnforcement = &x
	}
	if w.ExecuteHandoff != nil {
		x := *w.ExecuteHandoff
		w.ExecuteHandoff = &x
	}
	if w.ProviderObservation != nil {
		x := *w.ProviderObservation
		w.ProviderObservation = &x
	}
	if w.DomainResult != nil {
		x := *w.DomainResult
		w.DomainResult = &x
	}
	if w.Apply != nil {
		x := *w.Apply
		w.Apply = &x
	}
	if w.Result != nil {
		x := *w.Result
		w.Result = &x
	}
	return w
}
