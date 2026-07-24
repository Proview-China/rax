// Package refreshstore provides a process-local reference implementation of
// the Context Owner refresh store. It is not a production State Plane root,
// persistence backend or SLA.
package refreshstore

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	contextports "github.com/Proview-China/rax/ExecutionRuntime/context-engine/ports"
)

type lineageKeyV1 struct {
	Scope   contract.Digest
	RunID   string
	Session contract.FactRef
}

type scopedFactKeyV1 struct {
	Scope contract.Digest
	Ref   contract.FactRef
}
type pointerKeyV1 struct {
	Scope   contract.Digest
	RunID   string
	Session contract.FactRef
	Turn    uint32
}

type entryV1 struct {
	record contract.ContextTurnRefreshPendingRecordV1
	result contract.ContextTurnRefreshResultV1
}

type compactionEntryV1 struct {
	record contract.ContextCompactionPendingRecordV1
	result contract.ContextCompactionResultV1
}

// CurrentStateV1 is an exact Context Owner state snapshot used only to open
// this process-local backend on already authoritative metadata. It is not a
// request-derived seed and does not make the backend a production root.
type CurrentStateV1 struct {
	Binding    contract.ContextParentFrameSourceBindingV1
	Frame      contract.ContextFrame
	Manifest   contract.ContextManifest
	Generation contract.ContextGeneration
	Pointer    contract.ContextGenerationCurrentPointerV1
}

type Memory struct {
	mu                 sync.RWMutex
	entries            map[string]entryV1
	transitionRequests map[string]contract.ContextTurnTransitionRequestV1
	transitionProofs   map[string]contract.ContextTurnTransitionProofCurrentV1
	compactions        map[string]compactionEntryV1
	current            map[lineageKeyV1]contract.ContextGenerationCurrentPointerV1
	bindings           map[contract.ContextParentFrameApplicabilitySourceCoordinateV1]contract.ContextParentFrameSourceBindingV1
	frames             map[scopedFactKeyV1]contract.ContextFrame
	manifests          map[scopedFactKeyV1]contract.ContextManifest
	generations        map[scopedFactKeyV1]contract.ContextGeneration
	pointers           map[pointerKeyV1]contract.ContextGenerationCurrentPointerV1
}

func NewMemory() *Memory {
	return &Memory{
		entries:            make(map[string]entryV1),
		transitionRequests: make(map[string]contract.ContextTurnTransitionRequestV1),
		transitionProofs:   make(map[string]contract.ContextTurnTransitionProofCurrentV1),
		compactions:        make(map[string]compactionEntryV1),
		current:            make(map[lineageKeyV1]contract.ContextGenerationCurrentPointerV1),
		bindings:           make(map[contract.ContextParentFrameApplicabilitySourceCoordinateV1]contract.ContextParentFrameSourceBindingV1),
		frames:             make(map[scopedFactKeyV1]contract.ContextFrame), manifests: make(map[scopedFactKeyV1]contract.ContextManifest), generations: make(map[scopedFactKeyV1]contract.ContextGeneration), pointers: make(map[pointerKeyV1]contract.ContextGenerationCurrentPointerV1),
	}
}

func NewMemoryWithCurrentV1(state CurrentStateV1) (*Memory, error) {
	if err := validateCurrentStateV1(state); err != nil {
		return nil, err
	}
	state, err := clone(state)
	if err != nil {
		return nil, err
	}
	memory := NewMemory()
	scope := state.Frame.Execution.ScopeDigest
	memory.bindings[state.Binding.Source] = state.Binding
	memory.frames[scopedFactKeyV1{Scope: scope, Ref: state.Binding.Subject.FrameRef}] = state.Frame
	memory.manifests[scopedFactKeyV1{Scope: scope, Ref: state.Binding.Subject.ManifestRef}] = state.Manifest
	memory.generations[scopedFactKeyV1{Scope: scope, Ref: state.Binding.Subject.GenerationRef}] = state.Generation
	memory.pointers[pointerKeyV1{Scope: state.Pointer.ExecutionScopeDigest, RunID: state.Pointer.RunID, Session: state.Pointer.SessionRef, Turn: state.Pointer.Turn}] = state.Pointer
	memory.current[lineageKey(state.Pointer)] = state.Pointer
	return memory, nil
}

var _ contextports.ContextTurnRefreshStoreV1 = (*Memory)(nil)
var _ contextports.ContextTurnRefreshOwnerBackendV1 = (*Memory)(nil)
var _ contextports.ContextTurnTransitionProofStoreV1 = (*Memory)(nil)
var _ contextports.ContextParentFrameSourceBindingReaderV1 = (*Memory)(nil)
var _ contextports.ContextFrameMetadataReaderV1 = (*Memory)(nil)
var _ contextports.ContextManifestMetadataReaderV1 = (*Memory)(nil)
var _ contextports.ContextGenerationMetadataReaderV1 = (*Memory)(nil)
var _ contextports.ContextGenerationCurrentPointerReaderV1 = (*Memory)(nil)
var _ contextports.ContextCompactionOwnerBackendV1 = (*Memory)(nil)

func (m *Memory) ReserveContextCompactionV1(ctx context.Context, record contract.ContextCompactionPendingRecordV1) (contract.ContextCompactionPreparedV1, error) {
	if err := checkContext(ctx); err != nil {
		return contract.ContextCompactionPreparedV1{}, err
	}
	if err := record.Validate(); err != nil {
		return contract.ContextCompactionPreparedV1{}, err
	}
	recordCopy, err := clone(record)
	if err != nil {
		return contract.ContextCompactionPreparedV1{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.compactions[record.Plan.AttemptID]; exists {
		return contract.ContextCompactionPreparedV1{}, inspectOnlyError("compaction attempt already exists")
	}
	key := lineageKey(record.Plan.ExpectedCurrent)
	if current, ok := m.current[key]; !ok {
		return contract.ContextCompactionPreparedV1{}, fmt.Errorf("%w: authoritative generation current", contract.ErrNotFound)
	} else if current != record.Plan.ExpectedCurrent {
		return contract.ContextCompactionPreparedV1{}, fmt.Errorf("%w: expected compaction generation current", contract.ErrConflict)
	}
	result, err := compactionResult(recordCopy, nil, contract.ContextCompactionPendingV1)
	if err != nil {
		return contract.ContextCompactionPreparedV1{}, err
	}
	m.compactions[record.Plan.AttemptID] = compactionEntryV1{record: recordCopy, result: result}
	return clone(record.Prepared)
}

func (m *Memory) LoadContextCompactionPendingV1(ctx context.Context, planRef contract.FactRef) (contract.ContextCompactionPendingRecordV1, error) {
	if err := checkContext(ctx); err != nil {
		return contract.ContextCompactionPendingRecordV1{}, err
	}
	if planRef.Validate() != nil {
		return contract.ContextCompactionPendingRecordV1{}, fmt.Errorf("%w: compaction plan ref", contract.ErrInvalid)
	}
	m.mu.RLock()
	entry, ok := m.compactions[planRef.ID]
	m.mu.RUnlock()
	if !ok {
		return contract.ContextCompactionPendingRecordV1{}, fmt.Errorf("%w: compaction attempt", contract.ErrNotFound)
	}
	if compactionPlanRef(entry.record.Plan) != planRef {
		return contract.ContextCompactionPendingRecordV1{}, fmt.Errorf("%w: exact compaction plan", contract.ErrConflict)
	}
	return clone(entry.record)
}

func (m *Memory) ApplyContextCompactionCurrentCASV1(ctx context.Context, apply contract.ApplyContextCompactionRequestV1) (contract.ContextCompactionResultV1, error) {
	if err := checkContext(ctx); err != nil {
		return contract.ContextCompactionResultV1{}, err
	}
	if err := apply.Validate(); err != nil {
		return contract.ContextCompactionResultV1{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	entry, ok := m.compactions[apply.PlanRef.ID]
	if !ok {
		return contract.ContextCompactionResultV1{}, fmt.Errorf("%w: compaction attempt", contract.ErrNotFound)
	}
	if entry.result.Status == contract.ContextCompactionAppliedV1 {
		return contract.ContextCompactionResultV1{}, inspectOnlyError("compaction already applied")
	}
	if compactionPlanRef(entry.record.Plan) != apply.PlanRef || apply.ExpectedCurrent != entry.record.Plan.ExpectedCurrent || apply.PreparedDigest != entry.record.Prepared.Digest {
		return contract.ContextCompactionResultV1{}, fmt.Errorf("%w: compaction apply exact binding", contract.ErrConflict)
	}
	if apply.CheckedUnixNano >= entry.record.Plan.ExpiresUnixNano || apply.CheckedUnixNano >= entry.record.Summary.ExpiresUnixNano || apply.CheckedUnixNano >= entry.record.Frame.ExpiresUnixNano || apply.CheckedUnixNano >= entry.record.NextCurrent.ExpiresUnixNano {
		return contract.ContextCompactionResultV1{}, fmt.Errorf("%w: compaction apply currentness", contract.ErrExpired)
	}
	key := lineageKey(apply.ExpectedCurrent)
	if current, exists := m.current[key]; !exists || current != apply.ExpectedCurrent {
		return contract.ContextCompactionResultV1{}, fmt.Errorf("%w: compaction generation current CAS", contract.ErrConflict)
	}
	next := entry.record.NextCurrent
	result, err := compactionResult(entry.record, &next, contract.ContextCompactionAppliedV1)
	if err != nil {
		return contract.ContextCompactionResultV1{}, err
	}
	manifestDigest, _ := entry.record.Manifest.DigestValue()
	frameDigest, _ := entry.record.Frame.DigestValue()
	scope := entry.record.Frame.Execution.ScopeDigest
	m.manifests[scopedFactKeyV1{Scope: scope, Ref: contract.FactRef{ID: entry.record.Manifest.ID, Revision: entry.record.Manifest.Revision, Digest: manifestDigest}}] = entry.record.Manifest
	m.frames[scopedFactKeyV1{Scope: scope, Ref: contract.FactRef{ID: entry.record.Frame.ID, Revision: entry.record.Frame.Revision, Digest: frameDigest}}] = entry.record.Frame
	m.generations[scopedFactKeyV1{Scope: scope, Ref: entry.record.Prepared.GenerationRef}] = entry.record.Prepared.Generation
	m.current[key] = next
	m.pointers[pointerKeyV1{Scope: next.ExecutionScopeDigest, RunID: next.RunID, Session: next.SessionRef, Turn: next.Turn}] = next
	entry.result = result
	m.compactions[apply.PlanRef.ID] = entry
	return clone(result)
}

func (m *Memory) InspectContextCompactionV1(ctx context.Context, request contract.InspectContextCompactionRequestV1) (contract.ContextCompactionResultV1, error) {
	if err := checkContext(ctx); err != nil {
		return contract.ContextCompactionResultV1{}, err
	}
	if request.Validate() != nil {
		return contract.ContextCompactionResultV1{}, fmt.Errorf("%w: inspect compaction", contract.ErrInvalid)
	}
	m.mu.RLock()
	entry, ok := m.compactions[request.PlanRef.ID]
	m.mu.RUnlock()
	if !ok {
		return contract.ContextCompactionResultV1{}, fmt.Errorf("%w: compaction attempt", contract.ErrNotFound)
	}
	if compactionPlanRef(entry.record.Plan) != request.PlanRef {
		return contract.ContextCompactionResultV1{}, fmt.Errorf("%w: exact compaction plan", contract.ErrConflict)
	}
	return clone(entry.result)
}

func compactionPlanRef(plan contract.ContextCompactionPlanV1) contract.FactRef {
	return contract.FactRef{ID: plan.AttemptID, Revision: plan.Revision, Digest: plan.Digest}
}

func compactionResult(record contract.ContextCompactionPendingRecordV1, current *contract.ContextGenerationCurrentPointerV1, status contract.ContextCompactionStatusV1) (contract.ContextCompactionResultV1, error) {
	manifestDigest, err := record.Manifest.DigestValue()
	if err != nil {
		return contract.ContextCompactionResultV1{}, err
	}
	frameDigest, err := record.Frame.DigestValue()
	if err != nil {
		return contract.ContextCompactionResultV1{}, err
	}
	return contract.SealContextCompactionResultV1(contract.ContextCompactionResultV1{
		PlanRef: compactionPlanRef(record.Plan), SummaryRef: record.Plan.SummaryRef,
		ManifestRef:   contract.FactRef{ID: record.Manifest.ID, Revision: record.Manifest.Revision, Digest: manifestDigest},
		FrameRef:      contract.FactRef{ID: record.Frame.ID, Revision: record.Frame.Revision, Digest: frameDigest},
		GenerationRef: record.Prepared.GenerationRef, Current: current, Status: status,
	})
}

func (m *Memory) ReserveContextTurnRefreshV1(ctx context.Context, record contract.ContextTurnRefreshPendingRecordV1) (contract.ContextTurnRefreshPreparedV1, error) {
	if err := checkContext(ctx); err != nil {
		return contract.ContextTurnRefreshPreparedV1{}, err
	}
	prepared, err := validateRecord(record)
	if err != nil {
		return contract.ContextTurnRefreshPreparedV1{}, err
	}
	recordCopy, err := clone(record)
	if err != nil {
		return contract.ContextTurnRefreshPreparedV1{}, err
	}
	key := lineageKey(record.Request.ExpectedCurrent)
	recordDigest, err := contract.DigestJSON(record)
	if err != nil {
		return contract.ContextTurnRefreshPreparedV1{}, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if prior, ok := m.entries[record.Request.RefreshAttemptID]; ok {
		priorDigest, digestErr := contract.DigestJSON(prior.record)
		if digestErr != nil || priorDigest != recordDigest {
			return contract.ContextTurnRefreshPreparedV1{}, fmt.Errorf("%w: refresh attempt identity collision", contract.ErrConflict)
		}
		return contract.ContextTurnRefreshPreparedV1{}, inspectOnlyError("refresh attempt already exists")
	}
	current, ok := m.current[key]
	if !ok {
		return contract.ContextTurnRefreshPreparedV1{}, fmt.Errorf("%w: authoritative generation current", contract.ErrNotFound)
	}
	if current != record.Request.ExpectedCurrent {
		return contract.ContextTurnRefreshPreparedV1{}, fmt.Errorf("%w: expected authoritative generation current drift", contract.ErrConflict)
	}
	m.entries[record.Request.RefreshAttemptID] = entryV1{record: recordCopy, result: pendingResult(recordCopy)}
	return prepared, nil
}

func (m *Memory) ReserveContextTurnTransitionRequestV1(ctx context.Context, request contract.ContextTurnTransitionRequestV1) error {
	if err := checkContext(ctx); err != nil {
		return err
	}
	if request.ValidateAt(request.CheckedUnixNano) != nil {
		return fmt.Errorf("%w: context transition request", contract.ErrInvalid)
	}
	copy, err := clone(request)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if prior, ok := m.transitionRequests[request.ApplicationAttemptRef.ID]; ok {
		if prior != request {
			return fmt.Errorf("%w: context transition application attempt collision", contract.ErrConflict)
		}
		return inspectOnlyError("context transition request already exists")
	}
	m.transitionRequests[request.ApplicationAttemptRef.ID] = copy
	return nil
}

func (m *Memory) InspectContextTurnTransitionRequestByApplicationAttemptV1(ctx context.Context, applicationAttempt contract.FactRef) (contract.ContextTurnTransitionRequestV1, error) {
	if err := checkContext(ctx); err != nil {
		return contract.ContextTurnTransitionRequestV1{}, err
	}
	if applicationAttempt.Validate() != nil {
		return contract.ContextTurnTransitionRequestV1{}, fmt.Errorf("%w: context transition application attempt", contract.ErrInvalid)
	}
	m.mu.RLock()
	request, ok := m.transitionRequests[applicationAttempt.ID]
	m.mu.RUnlock()
	if !ok {
		return contract.ContextTurnTransitionRequestV1{}, fmt.Errorf("%w: context transition request", contract.ErrNotFound)
	}
	if request.ApplicationAttemptRef != applicationAttempt {
		return contract.ContextTurnTransitionRequestV1{}, fmt.Errorf("%w: context transition application attempt drift", contract.ErrConflict)
	}
	return clone(request)
}

func (m *Memory) EnsureContextTurnTransitionProofV1(ctx context.Context, current contract.ContextTurnTransitionProofCurrentV1) (contract.ContextTurnTransitionProofCurrentV1, error) {
	if err := checkContext(ctx); err != nil {
		return contract.ContextTurnTransitionProofCurrentV1{}, err
	}
	if current.ValidateAt(current.CheckedUnixNano) != nil {
		return contract.ContextTurnTransitionProofCurrentV1{}, fmt.Errorf("%w: context transition proof", contract.ErrInvalid)
	}
	copy, err := clone(current)
	if err != nil {
		return contract.ContextTurnTransitionProofCurrentV1{}, err
	}
	proof := current.Proof
	m.mu.Lock()
	defer m.mu.Unlock()
	request, ok := m.transitionRequests[proof.ApplicationAttemptRef.ID]
	if !ok {
		return contract.ContextTurnTransitionProofCurrentV1{}, fmt.Errorf("%w: context transition request", contract.ErrNotFound)
	}
	requestRef, err := request.Ref()
	if err != nil {
		return contract.ContextTurnTransitionProofCurrentV1{}, err
	}
	entry, ok := m.entries[proof.RefreshAttemptRef.ID]
	if !ok {
		return contract.ContextTurnTransitionProofCurrentV1{}, fmt.Errorf("%w: context refresh pending attempt", contract.ErrNotFound)
	}
	pendingRef, err := pendingRef(entry.record.Pending)
	if err != nil {
		return contract.ContextTurnTransitionProofCurrentV1{}, err
	}
	if proof.TransitionRequestRef != requestRef || proof.ApplicationAttemptRef != request.ApplicationAttemptRef || proof.RefreshAttemptRef != request.RefreshAttemptRef || proof.SourceSessionRef != request.SourceSessionRef || proof.SourceTurnRef != request.SourceTurnRef || proof.SourceTurnOrdinal != request.SourceTurnOrdinal || proof.TargetTurnOrdinal != request.ExpectedTargetOrdinal || proof.ExpectedCurrent != request.ExpectedCurrent || proof.PendingDomainResultRef != pendingRef || proof.ManifestRef != entry.record.Pending.ManifestRef || proof.FrameRef != entry.record.Pending.FrameRef || proof.GenerationRef != entry.record.Pending.GenerationRef || proof.StableSourceSetDigest != request.StableSourceSetDigest || current.S1AssociationSetDigest != request.S1AssociationSetDigest || proof.ChildExecution != entry.record.Frame.Execution {
		return contract.ContextTurnTransitionProofCurrentV1{}, fmt.Errorf("%w: context transition proof exact binding", contract.ErrConflict)
	}
	if prior, exists := m.transitionProofs[proof.ID]; exists {
		if prior.Proof != proof || prior.S1AssociationSetDigest != current.S1AssociationSetDigest {
			return contract.ContextTurnTransitionProofCurrentV1{}, fmt.Errorf("%w: context transition proof collision", contract.ErrConflict)
		}
		return clone(prior)
	}
	m.transitionProofs[proof.ID] = copy
	return clone(copy)
}

func (m *Memory) InspectContextTurnTransitionProofV1(ctx context.Context, proofRef contract.FactRef) (contract.ContextTurnTransitionProofCurrentV1, error) {
	if err := checkContext(ctx); err != nil {
		return contract.ContextTurnTransitionProofCurrentV1{}, err
	}
	if proofRef.Validate() != nil {
		return contract.ContextTurnTransitionProofCurrentV1{}, fmt.Errorf("%w: context transition proof ref", contract.ErrInvalid)
	}
	m.mu.RLock()
	current, ok := m.transitionProofs[proofRef.ID]
	m.mu.RUnlock()
	if !ok {
		return contract.ContextTurnTransitionProofCurrentV1{}, fmt.Errorf("%w: context transition proof", contract.ErrNotFound)
	}
	exact, err := current.Proof.Ref()
	if err != nil {
		return contract.ContextTurnTransitionProofCurrentV1{}, err
	}
	if exact != proofRef {
		return contract.ContextTurnTransitionProofCurrentV1{}, fmt.Errorf("%w: context transition proof drift", contract.ErrConflict)
	}
	return clone(current)
}

func (m *Memory) LoadContextTurnRefreshPendingRecordV1(ctx context.Context, attempt contract.FactRef) (contract.ContextTurnRefreshPendingRecordV1, error) {
	if err := checkContext(ctx); err != nil {
		return contract.ContextTurnRefreshPendingRecordV1{}, err
	}
	if attempt.Validate() != nil {
		return contract.ContextTurnRefreshPendingRecordV1{}, fmt.Errorf("%w: load refresh pending record", contract.ErrInvalid)
	}
	m.mu.RLock()
	entry, ok := m.entries[attempt.ID]
	m.mu.RUnlock()
	if !ok {
		return contract.ContextTurnRefreshPendingRecordV1{}, fmt.Errorf("%w: refresh attempt", contract.ErrNotFound)
	}
	if attempt != attemptRef(entry.record.Request) {
		return contract.ContextTurnRefreshPendingRecordV1{}, fmt.Errorf("%w: exact refresh attempt", contract.ErrConflict)
	}
	return clone(entry.record)
}

func (m *Memory) ApplyContextTurnRefreshCurrentCASV1(ctx context.Context, commit contract.ContextTurnRefreshCommitV1) (contract.ContextTurnRefreshResultV1, error) {
	if err := checkContext(ctx); err != nil {
		return contract.ContextTurnRefreshResultV1{}, err
	}
	if commit.AppliedUnixNano <= 0 || commit.Apply.ValidateAt(commit.AppliedUnixNano) != nil || commit.Settlement.Validate() != nil || commit.Settlement.AppliedUnixNano != commit.AppliedUnixNano {
		return contract.ContextTurnRefreshResultV1{}, fmt.Errorf("%w: refresh commit", contract.ErrInvalid)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	entry, ok := m.entries[commit.Apply.AttemptRef.ID]
	if !ok {
		return contract.ContextTurnRefreshResultV1{}, fmt.Errorf("%w: refresh attempt", contract.ErrNotFound)
	}
	wantAttempt := attemptRef(entry.record.Request)
	pendingRef, err := pendingRef(entry.record.Pending)
	if err != nil {
		return contract.ContextTurnRefreshResultV1{}, err
	}
	if commit.Apply.AttemptRef != wantAttempt || commit.Apply.PendingDomainResultRef != pendingRef || commit.Apply.ExpectedCurrent != entry.record.Request.ExpectedCurrent || commit.Settlement.AttemptRef != wantAttempt || commit.Settlement.PendingDomainResultRef != pendingRef || commit.Settlement.PreviousCurrentDigest != entry.record.Request.ExpectedCurrent.Digest || commit.Settlement.CurrentGenerationRef != entry.record.Pending.GenerationRef {
		return contract.ContextTurnRefreshResultV1{}, fmt.Errorf("%w: refresh commit exact binding", contract.ErrConflict)
	}
	if !sameOptionalFactRef(commit.Apply.TransitionProofRef, commit.Settlement.TransitionProofRef) || commit.Apply.StableSourceSetDigest != commit.Settlement.StableSourceSetDigest || commit.Apply.S2AssociationSetDigest != commit.Settlement.S2AssociationSetDigest {
		return contract.ContextTurnRefreshResultV1{}, fmt.Errorf("%w: refresh settlement transition binding", contract.ErrConflict)
	}
	if commit.Apply.TransitionProofRef != nil {
		proofCurrent, exists := m.transitionProofs[commit.Apply.TransitionProofRef.ID]
		if !exists {
			return contract.ContextTurnRefreshResultV1{}, fmt.Errorf("%w: context transition proof", contract.ErrNotFound)
		}
		proofRef, proofErr := proofCurrent.Proof.Ref()
		if proofErr != nil {
			return contract.ContextTurnRefreshResultV1{}, proofErr
		}
		proof := proofCurrent.Proof
		if proofRef != *commit.Apply.TransitionProofRef || proof.RefreshAttemptRef != wantAttempt || proof.PendingDomainResultRef != pendingRef || proof.ExpectedCurrent != commit.Apply.ExpectedCurrent || proof.StableSourceSetDigest != commit.Apply.StableSourceSetDigest {
			return contract.ContextTurnRefreshResultV1{}, fmt.Errorf("%w: context transition proof apply binding", contract.ErrConflict)
		}
	}
	if entry.result.Status == contract.ContextTurnRefreshAppliedV1 {
		return contract.ContextTurnRefreshResultV1{}, inspectOnlyError("refresh attempt already applied")
	}
	key := lineageKey(entry.record.Request.ExpectedCurrent)
	current, ok := m.current[key]
	if !ok || current != entry.record.Request.ExpectedCurrent {
		return contract.ContextTurnRefreshResultV1{}, fmt.Errorf("%w: expected generation current CAS", contract.ErrConflict)
	}
	settlementDigest, err := commit.Settlement.DigestValue()
	if err != nil {
		return contract.ContextTurnRefreshResultV1{}, err
	}
	settlementRef := contract.FactRef{ID: commit.Settlement.ID, Revision: commit.Settlement.Revision, Digest: settlementDigest}
	next := entry.record.Pointer
	result, err := contract.SealContextTurnRefreshResultV1(contract.ContextTurnRefreshResultV1{
		AttemptRef: wantAttempt, PendingDomainResultRef: pendingRef,
		ManifestRef: entry.record.Pending.ManifestRef, FrameRef: entry.record.Pending.FrameRef, GenerationRef: entry.record.Pending.GenerationRef,
		TransitionProofRef: commit.Apply.TransitionProofRef, StableSourceSetDigest: commit.Apply.StableSourceSetDigest, S2AssociationSetDigest: commit.Apply.S2AssociationSetDigest,
		ApplySettlementRef: &settlementRef, Current: &next, Status: contract.ContextTurnRefreshAppliedV1,
	})
	if err != nil {
		return contract.ContextTurnRefreshResultV1{}, err
	}
	// Settlement and current pointer become observable in this single critical
	// section. There is no Runtime settlement or partially visible state.
	m.current[key] = next
	scope := entry.record.Frame.Execution.ScopeDigest
	m.bindings[entry.record.Binding.Source] = entry.record.Binding
	m.frames[scopedFactKeyV1{Scope: scope, Ref: entry.record.Pending.FrameRef}] = entry.record.Frame
	m.manifests[scopedFactKeyV1{Scope: scope, Ref: entry.record.Pending.ManifestRef}] = entry.record.Manifest
	m.generations[scopedFactKeyV1{Scope: scope, Ref: entry.record.Pending.GenerationRef}] = entry.record.Generation
	m.pointers[pointerKeyV1{Scope: next.ExecutionScopeDigest, RunID: next.RunID, Session: next.SessionRef, Turn: next.Turn}] = next
	entry.result = result
	m.entries[commit.Apply.AttemptRef.ID] = entry
	return clone(result)
}

func sameOptionalFactRef(left, right *contract.FactRef) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

// CompareAndSwapGenerationCurrentV1 models another legitimate Context Owner
// writer using the same authoritative lock domain as current reads and Refresh
// Apply. It publishes no Refresh settlement or candidate metadata.
func (m *Memory) CompareAndSwapGenerationCurrentV1(ctx context.Context, expected, next contract.ContextGenerationCurrentPointerV1) error {
	if err := checkContext(ctx); err != nil {
		return err
	}
	if expected.Validate() != nil || next.Validate() != nil || lineageKey(expected) != lineageKey(next) {
		return fmt.Errorf("%w: generation current CAS", contract.ErrInvalid)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	key := lineageKey(expected)
	current, ok := m.current[key]
	if !ok {
		return fmt.Errorf("%w: authoritative generation current", contract.ErrNotFound)
	}
	if current != expected {
		return fmt.Errorf("%w: generation current CAS", contract.ErrConflict)
	}
	m.current[key] = next
	m.pointers[pointerKeyV1{Scope: next.ExecutionScopeDigest, RunID: next.RunID, Session: next.SessionRef, Turn: next.Turn}] = next
	return nil
}

func (m *Memory) ResolveExactSourceBinding(ctx context.Context, source contract.ContextParentFrameApplicabilitySourceCoordinateV1) (contract.ContextParentFrameSourceBindingV1, error) {
	if err := checkContext(ctx); err != nil {
		return contract.ContextParentFrameSourceBindingV1{}, err
	}
	if source.Validate() != nil {
		return contract.ContextParentFrameSourceBindingV1{}, fmt.Errorf("%w: source binding", contract.ErrInvalid)
	}
	m.mu.RLock()
	value, ok := m.bindings[source]
	if !ok {
		for candidate := range m.bindings {
			if candidate.ID == source.ID {
				m.mu.RUnlock()
				return contract.ContextParentFrameSourceBindingV1{}, fmt.Errorf("%w: source coordinate drift", contract.ErrConflict)
			}
		}
	}
	m.mu.RUnlock()
	if !ok {
		return contract.ContextParentFrameSourceBindingV1{}, fmt.Errorf("%w: source binding", contract.ErrNotFound)
	}
	return clone(value)
}

func (m *Memory) FrameByExactRef(ctx context.Context, ref contract.FactRef, scope contract.Digest) (contract.ContextFrame, error) {
	if err := checkContext(ctx); err != nil {
		return contract.ContextFrame{}, err
	}
	if ref.Validate() != nil || scope.Validate() != nil {
		return contract.ContextFrame{}, fmt.Errorf("%w: frame exact read", contract.ErrInvalid)
	}
	m.mu.RLock()
	value, ok := m.frames[scopedFactKeyV1{scope, ref}]
	conflict := !ok && hasFactID(m.frames, ref.ID)
	m.mu.RUnlock()
	if conflict {
		return contract.ContextFrame{}, fmt.Errorf("%w: frame exact drift", contract.ErrConflict)
	}
	if !ok {
		return contract.ContextFrame{}, fmt.Errorf("%w: frame exact read", contract.ErrNotFound)
	}
	return clone(value)
}

func (m *Memory) ManifestByExactRef(ctx context.Context, ref contract.FactRef, scope contract.Digest) (contract.ContextManifest, error) {
	if err := checkContext(ctx); err != nil {
		return contract.ContextManifest{}, err
	}
	if ref.Validate() != nil || scope.Validate() != nil {
		return contract.ContextManifest{}, fmt.Errorf("%w: manifest exact read", contract.ErrInvalid)
	}
	m.mu.RLock()
	value, ok := m.manifests[scopedFactKeyV1{scope, ref}]
	conflict := !ok && hasFactID(m.manifests, ref.ID)
	m.mu.RUnlock()
	if conflict {
		return contract.ContextManifest{}, fmt.Errorf("%w: manifest exact drift", contract.ErrConflict)
	}
	if !ok {
		return contract.ContextManifest{}, fmt.Errorf("%w: manifest exact read", contract.ErrNotFound)
	}
	return clone(value)
}

func (m *Memory) GenerationByExactRef(ctx context.Context, ref contract.FactRef, scope contract.Digest) (contract.ContextGeneration, error) {
	if err := checkContext(ctx); err != nil {
		return contract.ContextGeneration{}, err
	}
	if ref.Validate() != nil || scope.Validate() != nil {
		return contract.ContextGeneration{}, fmt.Errorf("%w: generation exact read", contract.ErrInvalid)
	}
	m.mu.RLock()
	value, ok := m.generations[scopedFactKeyV1{scope, ref}]
	conflict := !ok && hasFactID(m.generations, ref.ID)
	m.mu.RUnlock()
	if conflict {
		return contract.ContextGeneration{}, fmt.Errorf("%w: generation exact drift", contract.ErrConflict)
	}
	if !ok {
		return contract.ContextGeneration{}, fmt.Errorf("%w: generation exact read", contract.ErrNotFound)
	}
	return clone(value)
}

func (m *Memory) InspectCurrentGenerationPointer(ctx context.Context, request contract.ContextGenerationCurrentPointerRequestV1) (contract.ContextGenerationCurrentPointerV1, error) {
	if err := checkContext(ctx); err != nil {
		return contract.ContextGenerationCurrentPointerV1{}, err
	}
	if request.Validate() != nil {
		return contract.ContextGenerationCurrentPointerV1{}, fmt.Errorf("%w: generation pointer request", contract.ErrInvalid)
	}
	key := pointerKeyV1{request.ExecutionScopeDigest, request.RunID, request.SessionRef, request.Turn}
	m.mu.RLock()
	value, ok := m.pointers[key]
	if ok {
		current, currentOK := m.current[lineageKey(value)]
		if !currentOK || current != value {
			m.mu.RUnlock()
			return contract.ContextGenerationCurrentPointerV1{}, fmt.Errorf("%w: generation pointer is no longer authoritative current", contract.ErrConflict)
		}
	}
	m.mu.RUnlock()
	if !ok {
		return contract.ContextGenerationCurrentPointerV1{}, fmt.Errorf("%w: generation pointer", contract.ErrNotFound)
	}
	return clone(value)
}

func validateCurrentStateV1(state CurrentStateV1) error {
	if state.Binding.Validate() != nil || state.Frame.Validate() != nil || state.Manifest.Validate() != nil || state.Generation.Validate() != nil || state.Pointer.Validate() != nil {
		return fmt.Errorf("%w: authoritative current state", contract.ErrInvalid)
	}
	frameDigest, err := state.Frame.DigestValue()
	if err != nil {
		return err
	}
	manifestDigest, err := state.Manifest.DigestValue()
	if err != nil {
		return err
	}
	generationDigest, err := state.Generation.DigestValue()
	if err != nil {
		return err
	}
	subject := state.Binding.Subject
	if subject.FrameRef != (contract.FactRef{ID: state.Frame.ID, Revision: state.Frame.Revision, Digest: frameDigest}) || subject.ManifestRef != (contract.FactRef{ID: state.Manifest.ID, Revision: state.Manifest.Revision, Digest: manifestDigest}) || subject.GenerationRef != (contract.FactRef{ID: state.Generation.ID, Revision: state.Generation.Revision, Digest: generationDigest}) || state.Pointer.ExecutionScopeDigest != subject.ExecutionScopeDigest || state.Pointer.RunID != subject.RunID || state.Pointer.SessionRef != subject.SessionRef || state.Pointer.Turn != subject.Turn || state.Pointer.GenerationRef != subject.GenerationRef || state.Pointer.GenerationOrdinal != subject.GenerationOrdinal || state.Pointer.ParentFrameGenerationBindingDigest != subject.ParentFrameGenerationBindingDigest {
		return fmt.Errorf("%w: authoritative current state exact binding", contract.ErrConflict)
	}
	return nil
}

func (m *Memory) InspectContextTurnRefreshV1(ctx context.Context, request contract.InspectContextTurnRefreshRequestV1) (contract.ContextTurnRefreshResultV1, error) {
	if err := checkContext(ctx); err != nil {
		return contract.ContextTurnRefreshResultV1{}, err
	}
	if request.Validate() != nil {
		return contract.ContextTurnRefreshResultV1{}, fmt.Errorf("%w: inspect refresh", contract.ErrInvalid)
	}
	m.mu.RLock()
	entry, ok := m.entries[request.AttemptRef.ID]
	m.mu.RUnlock()
	if !ok {
		return contract.ContextTurnRefreshResultV1{}, fmt.Errorf("%w: refresh attempt", contract.ErrNotFound)
	}
	if request.AttemptRef != attemptRef(entry.record.Request) {
		return contract.ContextTurnRefreshResultV1{}, fmt.Errorf("%w: exact refresh attempt", contract.ErrConflict)
	}
	return clone(entry.result)
}

func validateRecord(record contract.ContextTurnRefreshPendingRecordV1) (contract.ContextTurnRefreshPreparedV1, error) {
	r := record.Request
	if r.Validate() != nil || record.ParentProjection.ValidateAt(r.CheckedUnixNano) != nil || record.ToolProjection.ValidateAt(r.CheckedUnixNano) != nil || record.Manifest.Validate() != nil || record.Frame.Validate() != nil || record.Generation.Validate() != nil || record.Binding.Validate() != nil || record.Pointer.Validate() != nil || record.Pending.Validate() != nil {
		return contract.ContextTurnRefreshPreparedV1{}, fmt.Errorf("%w: refresh pending record", contract.ErrInvalid)
	}
	manifestDigest, _ := record.Manifest.DigestValue()
	frameDigest, _ := record.Frame.DigestValue()
	generationDigest, _ := record.Generation.DigestValue()
	manifestRef := contract.FactRef{ID: record.Manifest.ID, Revision: record.Manifest.Revision, Digest: manifestDigest}
	frameRef := contract.FactRef{ID: record.Frame.ID, Revision: record.Frame.Revision, Digest: frameDigest}
	generationRef := contract.FactRef{ID: record.Generation.ID, Revision: record.Generation.Revision, Digest: generationDigest}
	if record.Pending.RequestDigest != r.Digest || record.Pending.ParentProjectionDigest != record.ParentProjection.Digest || record.Pending.ToolSourceDigest != record.ToolProjection.SourceDigest || record.Pending.ManifestRef != manifestRef || record.Pending.FrameRef != frameRef || record.Pending.GenerationRef != generationRef || record.Pending.ExpectedCurrent != r.ExpectedCurrent || record.Pending.NextCurrent != record.Pointer || record.Pending.ChildSource != record.Binding.Source || record.Frame.ManifestRef != manifestRef || record.Generation.RootFrame != frameRef || record.Pointer.GenerationRef != generationRef {
		return contract.ContextTurnRefreshPreparedV1{}, fmt.Errorf("%w: refresh pending exact binding", contract.ErrConflict)
	}
	return preparedFromRecord(record)
}

func preparedFromRecord(record contract.ContextTurnRefreshPendingRecordV1) (contract.ContextTurnRefreshPreparedV1, error) {
	pendingRef, err := pendingRef(record.Pending)
	if err != nil {
		return contract.ContextTurnRefreshPreparedV1{}, err
	}
	return contract.SealContextTurnRefreshPreparedV1(contract.ContextTurnRefreshPreparedV1{
		AttemptRef: attemptRef(record.Request), PendingDomainResultRef: pendingRef,
		ManifestRef: record.Pending.ManifestRef, FrameRef: record.Pending.FrameRef, GenerationRef: record.Pending.GenerationRef,
		CheckedUnixNano: record.Pending.CreatedUnixNano, ExpiresUnixNano: record.Pending.ExpiresUnixNano, Status: contract.ContextTurnRefreshPendingV1,
	}, record.Pending.CreatedUnixNano)
}

func pendingResult(record contract.ContextTurnRefreshPendingRecordV1) contract.ContextTurnRefreshResultV1 {
	pending, _ := pendingRef(record.Pending)
	result, _ := contract.SealContextTurnRefreshResultV1(contract.ContextTurnRefreshResultV1{AttemptRef: attemptRef(record.Request), PendingDomainResultRef: pending, ManifestRef: record.Pending.ManifestRef, FrameRef: record.Pending.FrameRef, GenerationRef: record.Pending.GenerationRef, Status: contract.ContextTurnRefreshPendingV1})
	return result
}

func attemptRef(request contract.ContextTurnRefreshRequestV1) contract.FactRef {
	return contract.FactRef{ID: request.RefreshAttemptID, Revision: 1, Digest: request.Digest}
}
func pendingRef(pending contract.ContextTurnRefreshPendingDomainResultV1) (contract.FactRef, error) {
	digest, err := pending.DigestValue()
	return contract.FactRef{ID: pending.ID, Revision: pending.Revision, Digest: digest}, err
}
func lineageKey(pointer contract.ContextGenerationCurrentPointerV1) lineageKeyV1 {
	return lineageKeyV1{pointer.ExecutionScopeDigest, pointer.RunID, pointer.SessionRef}
}
func hasFactID[T any](values map[scopedFactKeyV1]T, id string) bool {
	for key := range values {
		if key.Ref.ID == id {
			return true
		}
	}
	return false
}
func checkContext(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("%w: nil context", contract.ErrInvalid)
	}
	return ctx.Err()
}
func inspectOnlyError(reason string) error {
	return fmt.Errorf("%w: %w: %s; inspect original attempt", contract.ErrInspectOnly, contract.ErrConflict, reason)
}
func clone[T any](value T) (T, error) {
	var out T
	payload, err := json.Marshal(value)
	if err != nil {
		return out, err
	}
	if err = json.Unmarshal(payload, &out); err != nil {
		return out, err
	}
	return out, nil
}
