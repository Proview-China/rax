package applicationadapter

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolaction "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/action"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	toolsqlite "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/storage/sqlite"
	_ "modernc.org/sqlite"
)

type ownerStartOrInspectAdapterV2 struct {
	base *ownerExecutionV2
}

func (e ownerStartOrInspectAdapterV2) StartOrInspectBoundSingleCallToolActionV2(ctx context.Context, input ToolOwnerSingleCallStartOrInspectInputV2) (toolcontract.ToolResultV2, error) {
	return e.base.ExecuteBoundSingleCallToolActionV2(ctx, input.Execution)
}
func (e ownerStartOrInspectAdapterV2) InspectBoundSingleCallToolActionV2(ctx context.Context, input ToolOwnerSingleCallStartOrInspectInputV2) (toolcontract.ToolResultV2, error) {
	return e.base.InspectBoundSingleCallToolActionV2(ctx, input.Execution)
}

type exactDurableFactStoreV2 struct {
	result toolcontract.ToolResultV2
}

type barrierInspectExecutionV2 struct {
	result       toolcontract.ToolResultV2
	startCalls   int
	inspectCalls int
	mu           sync.Mutex
	release      chan struct{}
}

type capturingExecutionV2 struct {
	base       *ownerExecutionV2
	mu         sync.Mutex
	attemptIDs []string
}

type durableMutableClockV2 struct {
	mu  sync.RWMutex
	now time.Time
}

type failAfterEntryLeaseAcquireStateStoreV2 struct {
	base        ToolOwnerSingleCallExecutionStateStoreV2
	leases      ToolOwnerSingleCallEntryLeaseStoreV2
	acquired    chan struct{}
	failRelease chan struct{}
	signalOnce  sync.Once
	failNext    atomic.Bool
}

type indeterminateAcquireHandoffStateStoreV2 struct {
	base ToolOwnerSingleCallExecutionStateStoreV2
	once sync.Once
}

type actualEntryMutationModeV2 string

const (
	actualEntryExpireV2     actualEntryMutationModeV2 = "expire"
	actualEntryRegressV2    actualEntryMutationModeV2 = "regress"
	actualEntryCancelV2     actualEntryMutationModeV2 = "cancel"
	actualEntryStateDriftV2 actualEntryMutationModeV2 = "state_drift"
)

type actualEntryMutationStateStoreV2 struct {
	base                ToolOwnerSingleCallExecutionStateStoreV2
	leases              ToolOwnerSingleCallEntryLeaseStoreV2
	clock               *durableMutableClockV2
	cancel              context.CancelFunc
	mode                actualEntryMutationModeV2
	afterAcquire        atomic.Bool
	postAcquireInspects atomic.Int32
	leaseMu             sync.Mutex
	lease               ToolOwnerSingleCallEntryLeaseV2
}

func (c *durableMutableClockV2) Now() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.now
}

func (c *durableMutableClockV2) Set(now time.Time) {
	c.mu.Lock()
	c.now = now
	c.mu.Unlock()
}

func (s *failAfterEntryLeaseAcquireStateStoreV2) CreateExecutionStartV2(ctx context.Context, state ToolOwnerSingleCallExecutionStateV2) (ToolOwnerSingleCallExecutionStateV2, bool, error) {
	return s.base.CreateExecutionStartV2(ctx, state)
}

func (s *failAfterEntryLeaseAcquireStateStoreV2) InspectExecutionStateV2(ctx context.Context, key applicationcontract.SingleCallToolActionInspectKeyV2) (ToolOwnerSingleCallExecutionStateV2, error) {
	if s.failNext.CompareAndSwap(true, false) {
		select {
		case <-ctx.Done():
			return ToolOwnerSingleCallExecutionStateV2{}, ctx.Err()
		case <-s.failRelease:
		}
		return ToolOwnerSingleCallExecutionStateV2{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "simulated holder exit before external entry")
	}
	return s.base.InspectExecutionStateV2(ctx, key)
}

func (s *failAfterEntryLeaseAcquireStateStoreV2) AdvanceExecutionInspectOnlyV2(ctx context.Context, expected toolcontract.ObjectRef, unknown ToolOwnerSingleCallExecutionUnknownV2) (ToolOwnerSingleCallExecutionStateV2, error) {
	return s.base.AdvanceExecutionInspectOnlyV2(ctx, expected, unknown)
}

func (s *failAfterEntryLeaseAcquireStateStoreV2) AdvanceExecutionSettledV2(ctx context.Context, expected, result toolcontract.ObjectRef, now int64) (ToolOwnerSingleCallExecutionStateV2, error) {
	return s.base.AdvanceExecutionSettledV2(ctx, expected, result, now)
}

func (s *failAfterEntryLeaseAcquireStateStoreV2) TryAcquireExecutionEntryLeaseV2(ctx context.Context, request ToolOwnerSingleCallEntryLeaseAcquireV2) (ToolOwnerSingleCallEntryLeaseV2, bool, error) {
	lease, acquired, err := s.leases.TryAcquireExecutionEntryLeaseV2(ctx, request)
	if err == nil && acquired {
		s.failNext.Store(true)
		s.signalOnce.Do(func() { close(s.acquired) })
	}
	return lease, acquired, err
}

func (s *failAfterEntryLeaseAcquireStateStoreV2) InspectExecutionEntryLeaseV2(ctx context.Context, executionAttemptID string) (ToolOwnerSingleCallEntryLeaseV2, error) {
	return s.leases.InspectExecutionEntryLeaseV2(ctx, executionAttemptID)
}

func (s *failAfterEntryLeaseAcquireStateStoreV2) AdvanceExecutionEntryLeaseHandoffV2(ctx context.Context, expected ToolOwnerSingleCallEntryLeaseV2, nextPhase ToolOwnerSingleCallEntryLeasePhaseV2, now int64) (ToolOwnerSingleCallEntryLeaseV2, error) {
	return s.leases.AdvanceExecutionEntryLeaseHandoffV2(ctx, expected, nextPhase, now)
}

func (s *indeterminateAcquireHandoffStateStoreV2) CreateExecutionStartV2(ctx context.Context, state ToolOwnerSingleCallExecutionStateV2) (ToolOwnerSingleCallExecutionStateV2, bool, error) {
	return s.base.CreateExecutionStartV2(ctx, state)
}

func (s *indeterminateAcquireHandoffStateStoreV2) InspectExecutionStateV2(ctx context.Context, key applicationcontract.SingleCallToolActionInspectKeyV2) (ToolOwnerSingleCallExecutionStateV2, error) {
	return s.base.InspectExecutionStateV2(ctx, key)
}

func (s *indeterminateAcquireHandoffStateStoreV2) AdvanceExecutionInspectOnlyV2(ctx context.Context, expected toolcontract.ObjectRef, unknown ToolOwnerSingleCallExecutionUnknownV2) (ToolOwnerSingleCallExecutionStateV2, error) {
	return s.base.AdvanceExecutionInspectOnlyV2(ctx, expected, unknown)
}

func (s *indeterminateAcquireHandoffStateStoreV2) AdvanceExecutionSettledV2(ctx context.Context, expected, result toolcontract.ObjectRef, now int64) (ToolOwnerSingleCallExecutionStateV2, error) {
	return s.base.AdvanceExecutionSettledV2(ctx, expected, result, now)
}

func (s *indeterminateAcquireHandoffStateStoreV2) TryAcquireExecutionEntryLeaseV2(ctx context.Context, request ToolOwnerSingleCallEntryLeaseAcquireV2) (ToolOwnerSingleCallEntryLeaseV2, bool, error) {
	injected := false
	s.once.Do(func() { injected = true })
	if injected {
		return ToolOwnerSingleCallEntryLeaseV2{}, false, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "simulated acquisition reply unknown before exact Inspect")
	}
	return s.base.(ToolOwnerSingleCallEntryLeaseStoreV2).TryAcquireExecutionEntryLeaseV2(ctx, request)
}

func (s *indeterminateAcquireHandoffStateStoreV2) InspectExecutionEntryLeaseV2(ctx context.Context, executionAttemptID string) (ToolOwnerSingleCallEntryLeaseV2, error) {
	return s.base.(ToolOwnerSingleCallEntryLeaseStoreV2).InspectExecutionEntryLeaseV2(ctx, executionAttemptID)
}

func (s *indeterminateAcquireHandoffStateStoreV2) AdvanceExecutionEntryLeaseHandoffV2(ctx context.Context, expected ToolOwnerSingleCallEntryLeaseV2, nextPhase ToolOwnerSingleCallEntryLeasePhaseV2, now int64) (ToolOwnerSingleCallEntryLeaseV2, error) {
	return s.base.(ToolOwnerSingleCallEntryLeaseStoreV2).AdvanceExecutionEntryLeaseHandoffV2(ctx, expected, nextPhase, now)
}

func (s *actualEntryMutationStateStoreV2) CreateExecutionStartV2(ctx context.Context, state ToolOwnerSingleCallExecutionStateV2) (ToolOwnerSingleCallExecutionStateV2, bool, error) {
	return s.base.CreateExecutionStartV2(ctx, state)
}

func (s *actualEntryMutationStateStoreV2) InspectExecutionStateV2(ctx context.Context, key applicationcontract.SingleCallToolActionInspectKeyV2) (ToolOwnerSingleCallExecutionStateV2, error) {
	current, err := s.base.InspectExecutionStateV2(ctx, key)
	if err != nil || !s.afterAcquire.Load() {
		return current, err
	}
	inspection := s.postAcquireInspects.Add(1)
	if inspection == 1 {
		s.leaseMu.Lock()
		lease := s.lease
		s.leaseMu.Unlock()
		switch s.mode {
		case actualEntryExpireV2:
			s.clock.Set(time.Unix(0, lease.ExpiresUnixNano))
		case actualEntryRegressV2:
			s.clock.Set(time.Unix(0, lease.AcquiredUnixNano-1))
		case actualEntryCancelV2:
			s.cancel()
		}
	}
	if inspection == 2 && s.mode == actualEntryStateDriftV2 {
		drift := current
		drift.Revision++
		drift.State = ToolOwnerExecutionInspectOnlyV2
		drift.Unknown = &ToolOwnerSingleCallExecutionUnknownV2{
			Class:          ToolOwnerInspectionIndeterminateV2,
			ErrorDigest:    core.DigestBytes([]byte("actual-entry-state-drift")),
			MarkedUnixNano: current.UpdatedUnixNano + 1,
		}
		drift.UpdatedUnixNano = drift.Unknown.MarkedUnixNano
		drift.Digest, _ = drift.DigestV2()
		return drift, drift.Validate()
	}
	return current, nil
}

func (s *actualEntryMutationStateStoreV2) AdvanceExecutionInspectOnlyV2(ctx context.Context, expected toolcontract.ObjectRef, unknown ToolOwnerSingleCallExecutionUnknownV2) (ToolOwnerSingleCallExecutionStateV2, error) {
	return s.base.AdvanceExecutionInspectOnlyV2(ctx, expected, unknown)
}

func (s *actualEntryMutationStateStoreV2) AdvanceExecutionSettledV2(ctx context.Context, expected, result toolcontract.ObjectRef, now int64) (ToolOwnerSingleCallExecutionStateV2, error) {
	return s.base.AdvanceExecutionSettledV2(ctx, expected, result, now)
}

func (s *actualEntryMutationStateStoreV2) TryAcquireExecutionEntryLeaseV2(ctx context.Context, request ToolOwnerSingleCallEntryLeaseAcquireV2) (ToolOwnerSingleCallEntryLeaseV2, bool, error) {
	lease, acquired, err := s.leases.TryAcquireExecutionEntryLeaseV2(ctx, request)
	if err == nil && acquired {
		s.leaseMu.Lock()
		s.lease = lease
		s.leaseMu.Unlock()
		s.afterAcquire.Store(true)
	}
	return lease, acquired, err
}

func (s *actualEntryMutationStateStoreV2) InspectExecutionEntryLeaseV2(ctx context.Context, executionAttemptID string) (ToolOwnerSingleCallEntryLeaseV2, error) {
	return s.leases.InspectExecutionEntryLeaseV2(ctx, executionAttemptID)
}

func (s *actualEntryMutationStateStoreV2) AdvanceExecutionEntryLeaseHandoffV2(ctx context.Context, expected ToolOwnerSingleCallEntryLeaseV2, nextPhase ToolOwnerSingleCallEntryLeasePhaseV2, now int64) (ToolOwnerSingleCallEntryLeaseV2, error) {
	return s.leases.AdvanceExecutionEntryLeaseHandoffV2(ctx, expected, nextPhase, now)
}

type lostReplyClaimStoreV2 struct {
	base ToolOwnerSingleCallClaimStoreV2
	once sync.Once
}

func (s *lostReplyClaimStoreV2) CreateToolOwnerSingleCallClaimV2(ctx context.Context, record ToolOwnerSingleCallClaimRecordV2) (ToolOwnerSingleCallClaimRecordV2, bool, error) {
	winner, created, err := s.base.CreateToolOwnerSingleCallClaimV2(ctx, record)
	if err != nil {
		return winner, created, err
	}
	lost := false
	s.once.Do(func() { lost = true })
	if lost {
		return ToolOwnerSingleCallClaimRecordV2{}, false, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "lost claim reply")
	}
	return winner, created, nil
}
func (s *lostReplyClaimStoreV2) InspectToolOwnerSingleCallClaimV2(ctx context.Context, key applicationcontract.SingleCallToolActionInspectKeyV2) (ToolOwnerSingleCallClaimRecordV2, error) {
	return s.base.InspectToolOwnerSingleCallClaimV2(ctx, key)
}

type lostReplyExecutionStateStoreV2 struct {
	base        ToolOwnerSingleCallExecutionStateStoreV2
	createOnce  sync.Once
	advanceOnce sync.Once
}

type indeterminateStartOrInspectExecutionV2 struct {
	startCalls   atomic.Int32
	inspectCalls atomic.Int32
}

type postActualInspectHandoffExecutionV2 struct {
	result       toolcontract.ToolResultV2
	startCalls   atomic.Int32
	inspectCalls atomic.Int32
	attemptMu    sync.Mutex
	attemptIDs   []string
}

type inspectNotFoundThenStartExecutionV2 struct {
	result       toolcontract.ToolResultV2
	startCalls   atomic.Int32
	inspectCalls atomic.Int32
	attemptMu    sync.Mutex
	attemptIDs   []string
}

type blockingStartExecutionV2 struct {
	result       toolcontract.ToolResultV2
	startCalls   atomic.Int32
	inspectCalls atomic.Int32
	entered      chan struct{}
	release      chan struct{}
	enterOnce    sync.Once
}

type cancelThenBlockingInspectExecutionV2 struct {
	cancel       context.CancelFunc
	startCalls   atomic.Int32
	inspectCalls atomic.Int32
}

type liveDeadlineBlockingInspectExecutionV2 struct {
	startCalls   atomic.Int32
	inspectCalls atomic.Int32
	inspectStart chan time.Time
}

func (e *cancelThenBlockingInspectExecutionV2) StartOrInspectBoundSingleCallToolActionV2(context.Context, ToolOwnerSingleCallStartOrInspectInputV2) (toolcontract.ToolResultV2, error) {
	e.startCalls.Add(1)
	e.cancel()
	return toolcontract.ToolResultV2{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "entry outcome unknown")
}

func (e *cancelThenBlockingInspectExecutionV2) InspectBoundSingleCallToolActionV2(ctx context.Context, _ ToolOwnerSingleCallStartOrInspectInputV2) (toolcontract.ToolResultV2, error) {
	e.inspectCalls.Add(1)
	<-ctx.Done()
	return toolcontract.ToolResultV2{}, ctx.Err()
}

func (e *liveDeadlineBlockingInspectExecutionV2) StartOrInspectBoundSingleCallToolActionV2(ctx context.Context, _ ToolOwnerSingleCallStartOrInspectInputV2) (toolcontract.ToolResultV2, error) {
	e.startCalls.Add(1)
	if deadline, ok := ctx.Deadline(); ok {
		wait := time.Until(deadline) - 250*time.Millisecond
		if wait > 0 {
			timer := time.NewTimer(wait)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return toolcontract.ToolResultV2{}, ctx.Err()
			case <-timer.C:
			}
		}
	}
	return toolcontract.ToolResultV2{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "entry outcome unknown")
}

func (e *liveDeadlineBlockingInspectExecutionV2) InspectBoundSingleCallToolActionV2(ctx context.Context, _ ToolOwnerSingleCallStartOrInspectInputV2) (toolcontract.ToolResultV2, error) {
	e.inspectCalls.Add(1)
	select {
	case e.inspectStart <- time.Now():
	default:
	}
	<-ctx.Done()
	return toolcontract.ToolResultV2{}, ctx.Err()
}

func (e *indeterminateStartOrInspectExecutionV2) StartOrInspectBoundSingleCallToolActionV2(context.Context, ToolOwnerSingleCallStartOrInspectInputV2) (toolcontract.ToolResultV2, error) {
	e.startCalls.Add(1)
	return toolcontract.ToolResultV2{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "entry outcome unknown")
}

func (e *indeterminateStartOrInspectExecutionV2) InspectBoundSingleCallToolActionV2(context.Context, ToolOwnerSingleCallStartOrInspectInputV2) (toolcontract.ToolResultV2, error) {
	e.inspectCalls.Add(1)
	return toolcontract.ToolResultV2{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "inspection indeterminate")
}

func (e *postActualInspectHandoffExecutionV2) StartOrInspectBoundSingleCallToolActionV2(_ context.Context, input ToolOwnerSingleCallStartOrInspectInputV2) (toolcontract.ToolResultV2, error) {
	e.startCalls.Add(1)
	e.attemptMu.Lock()
	e.attemptIDs = append(e.attemptIDs, input.ExecutionAttemptID)
	e.attemptMu.Unlock()
	return toolcontract.ToolResultV2{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "entry reply lost after possible actual point")
}

func (e *postActualInspectHandoffExecutionV2) InspectBoundSingleCallToolActionV2(_ context.Context, input ToolOwnerSingleCallStartOrInspectInputV2) (toolcontract.ToolResultV2, error) {
	call := e.inspectCalls.Add(1)
	e.attemptMu.Lock()
	e.attemptIDs = append(e.attemptIDs, input.ExecutionAttemptID)
	e.attemptMu.Unlock()
	if call == 1 {
		return toolcontract.ToolResultV2{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "first exact Inspect reply is indeterminate")
	}
	return cloneToolResultV2(e.result), nil
}

func (e *inspectNotFoundThenStartExecutionV2) StartOrInspectBoundSingleCallToolActionV2(_ context.Context, input ToolOwnerSingleCallStartOrInspectInputV2) (toolcontract.ToolResultV2, error) {
	e.startCalls.Add(1)
	e.attemptMu.Lock()
	e.attemptIDs = append(e.attemptIDs, input.ExecutionAttemptID)
	e.attemptMu.Unlock()
	return cloneToolResultV2(e.result), nil
}

func (e *inspectNotFoundThenStartExecutionV2) InspectBoundSingleCallToolActionV2(_ context.Context, input ToolOwnerSingleCallStartOrInspectInputV2) (toolcontract.ToolResultV2, error) {
	e.inspectCalls.Add(1)
	e.attemptMu.Lock()
	e.attemptIDs = append(e.attemptIDs, input.ExecutionAttemptID)
	e.attemptMu.Unlock()
	return toolcontract.ToolResultV2{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "exact downstream Attempt is absent")
}

func (e *blockingStartExecutionV2) StartOrInspectBoundSingleCallToolActionV2(ctx context.Context, _ ToolOwnerSingleCallStartOrInspectInputV2) (toolcontract.ToolResultV2, error) {
	e.startCalls.Add(1)
	e.enterOnce.Do(func() { close(e.entered) })
	select {
	case <-ctx.Done():
		return toolcontract.ToolResultV2{}, ctx.Err()
	case <-e.release:
		return cloneToolResultV2(e.result), nil
	}
}

func (e *blockingStartExecutionV2) InspectBoundSingleCallToolActionV2(context.Context, ToolOwnerSingleCallStartOrInspectInputV2) (toolcontract.ToolResultV2, error) {
	e.inspectCalls.Add(1)
	return toolcontract.ToolResultV2{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "exact downstream Attempt is absent")
}

type inspectOnlyCommitLostStateStoreV2 struct {
	base ToolOwnerSingleCallExecutionStateStoreV2
	once sync.Once
}

func (s *inspectOnlyCommitLostStateStoreV2) CreateExecutionStartV2(ctx context.Context, state ToolOwnerSingleCallExecutionStateV2) (ToolOwnerSingleCallExecutionStateV2, bool, error) {
	return s.base.CreateExecutionStartV2(ctx, state)
}

func (s *inspectOnlyCommitLostStateStoreV2) InspectExecutionStateV2(ctx context.Context, key applicationcontract.SingleCallToolActionInspectKeyV2) (ToolOwnerSingleCallExecutionStateV2, error) {
	return s.base.InspectExecutionStateV2(ctx, key)
}

func (s *inspectOnlyCommitLostStateStoreV2) AdvanceExecutionInspectOnlyV2(ctx context.Context, expected toolcontract.ObjectRef, unknown ToolOwnerSingleCallExecutionUnknownV2) (ToolOwnerSingleCallExecutionStateV2, error) {
	current, err := s.base.AdvanceExecutionInspectOnlyV2(ctx, expected, unknown)
	if err != nil {
		return current, err
	}
	lost := false
	s.once.Do(func() { lost = true })
	if lost {
		return ToolOwnerSingleCallExecutionStateV2{}, core.NewError(core.ErrorConflict, core.ReasonInvalidTransition, "simulated CAS reply conflict after inspect_only commit")
	}
	return current, nil
}

func (s *inspectOnlyCommitLostStateStoreV2) AdvanceExecutionSettledV2(ctx context.Context, expected, result toolcontract.ObjectRef, now int64) (ToolOwnerSingleCallExecutionStateV2, error) {
	return s.base.AdvanceExecutionSettledV2(ctx, expected, result, now)
}
func (s *inspectOnlyCommitLostStateStoreV2) TryAcquireExecutionEntryLeaseV2(ctx context.Context, request ToolOwnerSingleCallEntryLeaseAcquireV2) (ToolOwnerSingleCallEntryLeaseV2, bool, error) {
	return s.base.(ToolOwnerSingleCallEntryLeaseStoreV2).TryAcquireExecutionEntryLeaseV2(ctx, request)
}
func (s *inspectOnlyCommitLostStateStoreV2) InspectExecutionEntryLeaseV2(ctx context.Context, executionAttemptID string) (ToolOwnerSingleCallEntryLeaseV2, error) {
	return s.base.(ToolOwnerSingleCallEntryLeaseStoreV2).InspectExecutionEntryLeaseV2(ctx, executionAttemptID)
}
func (s *inspectOnlyCommitLostStateStoreV2) AdvanceExecutionEntryLeaseHandoffV2(ctx context.Context, expected ToolOwnerSingleCallEntryLeaseV2, nextPhase ToolOwnerSingleCallEntryLeasePhaseV2, now int64) (ToolOwnerSingleCallEntryLeaseV2, error) {
	return s.base.(ToolOwnerSingleCallEntryLeaseStoreV2).AdvanceExecutionEntryLeaseHandoffV2(ctx, expected, nextPhase, now)
}

func (s *lostReplyExecutionStateStoreV2) CreateExecutionStartV2(ctx context.Context, state ToolOwnerSingleCallExecutionStateV2) (ToolOwnerSingleCallExecutionStateV2, bool, error) {
	winner, created, err := s.base.CreateExecutionStartV2(ctx, state)
	if err != nil {
		return winner, created, err
	}
	lost := false
	s.createOnce.Do(func() { lost = true })
	if lost {
		return ToolOwnerSingleCallExecutionStateV2{}, false, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "lost marker create reply")
	}
	return winner, created, nil
}
func (s *lostReplyExecutionStateStoreV2) InspectExecutionStateV2(ctx context.Context, key applicationcontract.SingleCallToolActionInspectKeyV2) (ToolOwnerSingleCallExecutionStateV2, error) {
	return s.base.InspectExecutionStateV2(ctx, key)
}
func (s *lostReplyExecutionStateStoreV2) AdvanceExecutionInspectOnlyV2(ctx context.Context, expected toolcontract.ObjectRef, unknown ToolOwnerSingleCallExecutionUnknownV2) (ToolOwnerSingleCallExecutionStateV2, error) {
	return s.base.AdvanceExecutionInspectOnlyV2(ctx, expected, unknown)
}
func (s *lostReplyExecutionStateStoreV2) AdvanceExecutionSettledV2(ctx context.Context, expected, result toolcontract.ObjectRef, now int64) (ToolOwnerSingleCallExecutionStateV2, error) {
	state, err := s.base.AdvanceExecutionSettledV2(ctx, expected, result, now)
	if err != nil {
		return state, err
	}
	lost := false
	s.advanceOnce.Do(func() { lost = true })
	if lost {
		return ToolOwnerSingleCallExecutionStateV2{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "lost marker advance reply")
	}
	return state, nil
}
func (s *lostReplyExecutionStateStoreV2) TryAcquireExecutionEntryLeaseV2(ctx context.Context, request ToolOwnerSingleCallEntryLeaseAcquireV2) (ToolOwnerSingleCallEntryLeaseV2, bool, error) {
	return s.base.(ToolOwnerSingleCallEntryLeaseStoreV2).TryAcquireExecutionEntryLeaseV2(ctx, request)
}
func (s *lostReplyExecutionStateStoreV2) InspectExecutionEntryLeaseV2(ctx context.Context, executionAttemptID string) (ToolOwnerSingleCallEntryLeaseV2, error) {
	return s.base.(ToolOwnerSingleCallEntryLeaseStoreV2).InspectExecutionEntryLeaseV2(ctx, executionAttemptID)
}
func (s *lostReplyExecutionStateStoreV2) AdvanceExecutionEntryLeaseHandoffV2(ctx context.Context, expected ToolOwnerSingleCallEntryLeaseV2, nextPhase ToolOwnerSingleCallEntryLeasePhaseV2, now int64) (ToolOwnerSingleCallEntryLeaseV2, error) {
	return s.base.(ToolOwnerSingleCallEntryLeaseStoreV2).AdvanceExecutionEntryLeaseHandoffV2(ctx, expected, nextPhase, now)
}

func (e *barrierInspectExecutionV2) StartOrInspectBoundSingleCallToolActionV2(context.Context, ToolOwnerSingleCallStartOrInspectInputV2) (toolcontract.ToolResultV2, error) {
	e.mu.Lock()
	e.startCalls++
	e.mu.Unlock()
	return toolcontract.ToolResultV2{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "unexpected start")
}

func (e *barrierInspectExecutionV2) InspectBoundSingleCallToolActionV2(_ context.Context, input ToolOwnerSingleCallStartOrInspectInputV2) (toolcontract.ToolResultV2, error) {
	if err := input.Validate(); err != nil {
		return toolcontract.ToolResultV2{}, err
	}
	e.mu.Lock()
	e.inspectCalls++
	if e.inspectCalls == 1 {
		close(e.release)
	}
	e.mu.Unlock()
	<-e.release
	return cloneToolResultV2(e.result), nil
}

func (e *capturingExecutionV2) StartOrInspectBoundSingleCallToolActionV2(ctx context.Context, input ToolOwnerSingleCallStartOrInspectInputV2) (toolcontract.ToolResultV2, error) {
	if err := input.Validate(); err != nil {
		return toolcontract.ToolResultV2{}, err
	}
	e.mu.Lock()
	e.attemptIDs = append(e.attemptIDs, input.ExecutionAttemptID)
	e.mu.Unlock()
	return e.base.ExecuteBoundSingleCallToolActionV2(ctx, input.Execution)
}

func (e *capturingExecutionV2) InspectBoundSingleCallToolActionV2(ctx context.Context, input ToolOwnerSingleCallStartOrInspectInputV2) (toolcontract.ToolResultV2, error) {
	if err := input.Validate(); err != nil {
		return toolcontract.ToolResultV2{}, err
	}
	e.mu.Lock()
	e.attemptIDs = append(e.attemptIDs, input.ExecutionAttemptID)
	e.mu.Unlock()
	return e.base.InspectBoundSingleCallToolActionV2(ctx, input.Execution)
}

func (s exactDurableFactStoreV2) CreateCandidateFactV2(context.Context, toolcontract.ActionCandidateV2) (toolaction.RecordV2, error) {
	return toolaction.RecordV2{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "not used")
}
func (s exactDurableFactStoreV2) InspectCandidateCurrentV2(context.Context, toolcontract.ObjectRef, time.Time) (toolcontract.ActionCandidateV2, error) {
	return toolcontract.ActionCandidateV2{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "not used")
}
func (s exactDurableFactStoreV2) CreateReservationFactV2(context.Context, toolcontract.ObjectRef, toolcontract.ApplicationAttemptRefV1, core.Digest, string, core.Digest, time.Time, time.Time) (toolcontract.ActionReservationFactV2, error) {
	return toolcontract.ActionReservationFactV2{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "not used")
}
func (s exactDurableFactStoreV2) InspectReservationExactV2(context.Context, string, toolcontract.ObjectRef) (toolcontract.ActionReservationFactV2, error) {
	return toolcontract.ActionReservationFactV2{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "not used")
}
func (s exactDurableFactStoreV2) CreateDomainResultFactV2(context.Context, toolcontract.ToolDomainResultFactV2) (toolcontract.ToolDomainResultFactV2, error) {
	return toolcontract.ToolDomainResultFactV2{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "not used")
}
func (s exactDurableFactStoreV2) InspectDomainResultExactV2(context.Context, string, toolcontract.ObjectRef) (toolcontract.ToolDomainResultFactV2, error) {
	return toolcontract.ToolDomainResultFactV2{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "not used")
}
func (s exactDurableFactStoreV2) InspectDomainResultCurrentByExactV1(context.Context, toolcontract.ObjectRef, time.Time, time.Duration) (toolcontract.ToolDomainResultCurrentProjectionV1, error) {
	return toolcontract.ToolDomainResultCurrentProjectionV1{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "not used")
}
func (s exactDurableFactStoreV2) ApplySettlementAndCreateResultV2(context.Context, string, toolcontract.ObjectRef, runtimeports.OperationInspectionSettlementRefV4, toolcontract.ToolOutcomeV2, toolcontract.ToolDispositionV2, time.Time) (toolcontract.ToolResultV2, error) {
	return toolcontract.ToolResultV2{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "not used")
}
func (s exactDurableFactStoreV2) InspectResultExactV2(_ context.Context, actionID string, exact toolcontract.ObjectRef) (toolcontract.ToolResultV2, error) {
	if s.result.Action.ID != actionID || objectRefResultV1(s.result) != exact {
		return toolcontract.ToolResultV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "exact ToolResult drifted")
	}
	return cloneToolResultV2(s.result), nil
}
func (s exactDurableFactStoreV2) InspectSettledResultForApplyV2(_ context.Context, actionID string, apply toolcontract.ObjectRef) (toolcontract.ToolResultV2, error) {
	if s.result.Action.ID != actionID || s.result.Apply != apply {
		return toolcontract.ToolResultV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "exact ApplySettlement drifted")
	}
	return cloneToolResultV2(s.result), nil
}

func TestDurableToolOwnerFlowClaimMarkerStartAndRestartInspect(t *testing.T) {
	fixture := newAdapterV2Fixture(t)
	input := ToolOwnerSingleCallExecutionV2{Request: fixture.binding.request.ApplicationRequest, Binding: fixture.projection}
	states := NewInMemoryToolOwnerSingleCallExecutionStateStoreV2()
	execution := &capturingExecutionV2{base: fixture.execution}
	flow, err := NewDurableToolOwnerSingleCallFlowV2(execution, fixture.execution, fixture.claims, states, exactDurableFactStoreV2{result: fixture.execution.result}, fixture.binding.clock)
	if err != nil {
		t.Fatal(err)
	}
	first, err := flow.StartOrInspectToolOwnerSingleCallV2(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if fixture.execution.executeCalls.Load() != 1 {
		t.Fatalf("physical StartOrInspect calls=%d, want 1", fixture.execution.executeCalls.Load())
	}
	key, err := applicationcontract.SealSingleCallToolActionInspectKeyV2(input.Request)
	if err != nil {
		t.Fatal(err)
	}
	state, err := states.InspectExecutionStateV2(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	execution.mu.Lock()
	if len(execution.attemptIDs) != 1 || execution.attemptIDs[0] != state.ExecutionAttemptID {
		t.Fatalf("downstream attempts=%v marker=%s", execution.attemptIDs, state.ExecutionAttemptID)
	}
	execution.mu.Unlock()
	restarted, err := NewDurableToolOwnerSingleCallFlowV2(execution, fixture.execution, fixture.claims, states, exactDurableFactStoreV2{result: fixture.execution.result}, fixture.binding.clock)
	if err != nil {
		t.Fatal(err)
	}
	second, err := restarted.StartOrInspectToolOwnerSingleCallV2(context.Background(), input)
	if err != nil || second.Digest != first.Digest {
		t.Fatalf("restart result=%#v err=%v", second, err)
	}
	if fixture.execution.executeCalls.Load() != 1 {
		t.Fatalf("settled restart re-entered StartOrInspect: %d", fixture.execution.executeCalls.Load())
	}
}

func TestToolOwnerClaimAndMarkerReplayUseDeterministicFullPayload(t *testing.T) {
	fixture := newAdapterV2Fixture(t)
	input := ToolOwnerSingleCallExecutionV2{Request: fixture.binding.request.ApplicationRequest, Binding: fixture.projection}
	first, err := newToolOwnerSingleCallClaimV2(input, fixture.binding.now.UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := newToolOwnerSingleCallClaimV2(input, fixture.binding.now.Add(time.Hour).UnixNano())
	if err != nil || !reflect.DeepEqual(first, replayed) {
		t.Fatalf("claim replay first=%#v replay=%#v err=%v", first, replayed, err)
	}
	claims := NewInMemoryToolOwnerSingleCallClaimStoreV2()
	record := ToolOwnerSingleCallClaimRecordV2{Claim: first, Input: input}
	if _, _, err = claims.CreateToolOwnerSingleCallClaimV2(context.Background(), record); err != nil {
		t.Fatal(err)
	}
	drift := record
	drift.Claim.CreatedUnixNano++
	drift.Claim.Digest, _ = drift.Claim.DigestV2()
	if _, _, err = claims.CreateToolOwnerSingleCallClaimV2(context.Background(), drift); err == nil || !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("claim time drift error=%v, want Conflict", err)
	}
	startA, err := NewToolOwnerSingleCallExecutionStartV2(record, fixture.binding.now.UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	startB, err := NewToolOwnerSingleCallExecutionStartV2(record, fixture.binding.now.Add(time.Hour).UnixNano())
	if err != nil || !reflect.DeepEqual(startA, startB) {
		t.Fatalf("marker replay A=%#v B=%#v err=%v", startA, startB, err)
	}
	states := NewInMemoryToolOwnerSingleCallExecutionStateStoreV2()
	if _, _, err = states.CreateExecutionStartV2(context.Background(), startA); err != nil {
		t.Fatal(err)
	}
	markerDrift := startA
	markerDrift.CreatedUnixNano++
	markerDrift.UpdatedUnixNano++
	markerDrift.Digest, _ = markerDrift.DigestV2()
	if _, _, err = states.CreateExecutionStartV2(context.Background(), markerDrift); err == nil || !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("marker time drift error=%v, want Conflict", err)
	}
}

func TestDurableToolOwnerFlowLostClaimAndMarkerRepliesDoNotRepeatStart(t *testing.T) {
	fixture := newAdapterV2Fixture(t)
	input := ToolOwnerSingleCallExecutionV2{Request: fixture.binding.request.ApplicationRequest, Binding: fixture.projection}
	claims := &lostReplyClaimStoreV2{base: NewInMemoryToolOwnerSingleCallClaimStoreV2()}
	states := &lostReplyExecutionStateStoreV2{base: NewInMemoryToolOwnerSingleCallExecutionStateStoreV2()}
	flow, err := NewDurableToolOwnerSingleCallFlowV2(ownerStartOrInspectAdapterV2{base: fixture.execution}, fixture.execution, claims, states, exactDurableFactStoreV2{result: fixture.execution.result}, fixture.binding.clock)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = flow.StartOrInspectToolOwnerSingleCallV2(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	if fixture.execution.executeCalls.Load() != 1 {
		t.Fatalf("lost replies repeated StartOrInspect %d times", fixture.execution.executeCalls.Load())
	}
	if _, err = flow.StartOrInspectToolOwnerSingleCallV2(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	if fixture.execution.executeCalls.Load() != 1 {
		t.Fatalf("settled replay repeated StartOrInspect %d times", fixture.execution.executeCalls.Load())
	}
}

func TestDurableToolOwnerFlowInspectOnly64NeverStartsAndSingleInspects(t *testing.T) {
	fixture := newAdapterV2Fixture(t)
	input := ToolOwnerSingleCallExecutionV2{Request: fixture.binding.request.ApplicationRequest, Binding: fixture.projection}
	claim, err := newToolOwnerSingleCallClaimV2(input, fixture.binding.now.UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	record := ToolOwnerSingleCallClaimRecordV2{Claim: claim, Input: input}
	if _, _, err = fixture.claims.CreateToolOwnerSingleCallClaimV2(context.Background(), record); err != nil {
		t.Fatal(err)
	}
	states := NewInMemoryToolOwnerSingleCallExecutionStateStoreV2()
	start, err := NewToolOwnerSingleCallExecutionStartV2(record, fixture.binding.now.UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	start, _, err = states.CreateExecutionStartV2(context.Background(), start)
	if err != nil {
		t.Fatal(err)
	}
	unknown := ToolOwnerSingleCallExecutionUnknownV2{Class: ToolOwnerEntryOutcomeUnknownV2, ErrorDigest: core.DigestBytes([]byte("unknown")), MarkedUnixNano: fixture.binding.now.UnixNano()}
	if _, err = states.AdvanceExecutionInspectOnlyV2(context.Background(), start.RefV2(), unknown); err != nil {
		t.Fatal(err)
	}
	fixture.execution.ready.Store(true)
	execution := &barrierInspectExecutionV2{result: fixture.execution.result, release: make(chan struct{})}
	flow, err := NewDurableToolOwnerSingleCallFlowV2(execution, fixture.execution, fixture.claims, states, exactDurableFactStoreV2{result: fixture.execution.result}, fixture.binding.clock)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 64)
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, callErr := flow.StartOrInspectToolOwnerSingleCallV2(context.Background(), input)
			errs <- callErr
		}()
	}
	wg.Wait()
	close(errs)
	for callErr := range errs {
		if callErr != nil {
			t.Fatal(callErr)
		}
	}
	execution.mu.Lock()
	startCalls, inspectCalls := execution.startCalls, execution.inspectCalls
	execution.mu.Unlock()
	if startCalls != 0 || inspectCalls != 1 {
		t.Fatalf("StartOrInspect=%d Inspect=%d, want 0/1", startCalls, inspectCalls)
	}
}

func TestDurableToolOwnerFlowStartCommitted64HasOneExternalEntry(t *testing.T) {
	fixture := newAdapterV2Fixture(t)
	input := ToolOwnerSingleCallExecutionV2{Request: fixture.binding.request.ApplicationRequest, Binding: fixture.projection}
	states := NewInMemoryToolOwnerSingleCallExecutionStateStoreV2()
	flow, err := NewDurableToolOwnerSingleCallFlowV2(
		ownerStartOrInspectAdapterV2{base: fixture.execution},
		fixture.execution,
		fixture.claims,
		states,
		exactDurableFactStoreV2{result: fixture.execution.result},
		fixture.binding.clock,
	)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan toolcontract.ToolResultV2, 64)
	errs := make(chan error, 64)
	for range 64 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, callErr := flow.StartOrInspectToolOwnerSingleCallV2(context.Background(), input)
			results <- result
			errs <- callErr
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	for callErr := range errs {
		if callErr != nil {
			t.Fatal(callErr)
		}
	}
	for result := range results {
		if result.Digest != fixture.execution.result.Digest {
			t.Fatalf("concurrent result digest=%s want=%s", result.Digest, fixture.execution.result.Digest)
		}
	}
	if fixture.execution.executeCalls.Load() != 1 {
		t.Fatalf("start_committed external entries=%d, want 1", fixture.execution.executeCalls.Load())
	}
}

func TestSQLiteDurableToolOwnerFlow64IndependentInstancesHaveOneExternalEntry(t *testing.T) {
	fixture := newAdapterV2Fixture(t)
	input := ToolOwnerSingleCallExecutionV2{Request: fixture.binding.request.ApplicationRequest, Binding: fixture.projection}
	config := toolsqlite.ConfigV1{
		Path: filepath.Join(t.TempDir(), "independent-flows.db"), Owner: core.OwnerRef{Domain: "tool-mcp", ID: "owner-v2"},
		Clock: func() time.Time { return fixture.binding.now },
	}
	rawStores := make([]*toolsqlite.OwnerClaimExecutionStoreV2, 8)
	var err error
	for index := range rawStores {
		rawStores[index], err = toolsqlite.OpenOwnerClaimExecutionStoreV2(context.Background(), config)
		if err != nil {
			t.Fatal(err)
		}
		defer rawStores[index].Close()
	}
	execution := ownerStartOrInspectAdapterV2{base: fixture.execution}
	flows := make([]*ToolOwnerSingleCallFlowImplV2, 64)
	for index := range flows {
		claims, claimErr := NewSQLiteToolOwnerSingleCallClaimStoreV2(rawStores[index%len(rawStores)])
		if claimErr != nil {
			t.Fatal(claimErr)
		}
		states, stateErr := NewSQLiteToolOwnerSingleCallExecutionStateStoreV2(rawStores[index%len(rawStores)])
		if stateErr != nil {
			t.Fatal(stateErr)
		}
		flows[index], err = NewDurableToolOwnerSingleCallFlowV2(execution, fixture.execution, claims, states, exactDurableFactStoreV2{result: fixture.execution.result}, fixture.binding.clock)
		if err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	errs := make(chan error, len(flows))
	for _, flow := range flows {
		wg.Add(1)
		go func(flow *ToolOwnerSingleCallFlowImplV2) {
			defer wg.Done()
			result, runErr := flow.StartOrInspectToolOwnerSingleCallV2(context.Background(), input)
			if runErr == nil && (result.Validate() != nil || objectRefResultV1(result) != objectRefResultV1(fixture.execution.result)) {
				runErr = errors.New("independent durable flow returned another exact result")
			}
			errs <- runErr
		}(flow)
	}
	wg.Wait()
	close(errs)
	for runErr := range errs {
		if runErr != nil {
			t.Fatal(runErr)
		}
	}
	if calls := fixture.execution.executeCalls.Load(); calls != 1 {
		t.Fatalf("independent flows StartOrInspect calls=%d, want 1", calls)
	}
	db, err := sql.Open("sqlite", config.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var leaseHistory, leaseHeads int
	if err = db.QueryRow(`SELECT COUNT(*) FROM tool_owner_entry_lease_history_v2`).Scan(&leaseHistory); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT COUNT(*) FROM tool_owner_entry_lease_head_v2`).Scan(&leaseHeads); err != nil {
		t.Fatal(err)
	}
	if leaseHistory != 1 || leaseHeads != 1 {
		t.Fatalf("entry lease history/head=%d/%d, want 1/1", leaseHistory, leaseHeads)
	}
}

func TestSQLiteDurableToolOwnerInspectOnly64IndependentInstancesHaveOneExternalInspect(t *testing.T) {
	fixture := newAdapterV2Fixture(t)
	input := ToolOwnerSingleCallExecutionV2{Request: fixture.binding.request.ApplicationRequest, Binding: fixture.projection}
	config := toolsqlite.ConfigV1{
		Path: filepath.Join(t.TempDir(), "independent-inspect-flows.db"), Owner: core.OwnerRef{Domain: "tool-mcp", ID: "owner-v2"},
		Clock: func() time.Time { return fixture.binding.now },
	}
	rawStores := make([]*toolsqlite.OwnerClaimExecutionStoreV2, 8)
	var err error
	for index := range rawStores {
		rawStores[index], err = toolsqlite.OpenOwnerClaimExecutionStoreV2(context.Background(), config)
		if err != nil {
			t.Fatal(err)
		}
		defer rawStores[index].Close()
	}
	claims, _ := NewSQLiteToolOwnerSingleCallClaimStoreV2(rawStores[0])
	states, _ := NewSQLiteToolOwnerSingleCallExecutionStateStoreV2(rawStores[0])
	claim, err := newToolOwnerSingleCallClaimV2(input, fixture.binding.now.UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	record, _, err := claims.CreateToolOwnerSingleCallClaimV2(context.Background(), ToolOwnerSingleCallClaimRecordV2{Claim: claim, Input: input})
	if err != nil {
		t.Fatal(err)
	}
	state, err := NewToolOwnerSingleCallExecutionStartV2(record, fixture.binding.now.UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	state, _, err = states.CreateExecutionStartV2(context.Background(), state)
	if err != nil {
		t.Fatal(err)
	}
	unknown := ToolOwnerSingleCallExecutionUnknownV2{
		Class: ToolOwnerEntryOutcomeUnknownV2, ErrorDigest: core.DigestBytes([]byte("inspect-only")),
		MarkedUnixNano: state.UpdatedUnixNano + 1,
	}
	if _, err = states.AdvanceExecutionInspectOnlyV2(context.Background(), state.RefV2(), unknown); err != nil {
		t.Fatal(err)
	}
	execution := &barrierInspectExecutionV2{result: fixture.execution.result, release: make(chan struct{})}
	flows := make([]*ToolOwnerSingleCallFlowImplV2, 64)
	for index := range flows {
		flowClaims, claimErr := NewSQLiteToolOwnerSingleCallClaimStoreV2(rawStores[index%len(rawStores)])
		if claimErr != nil {
			t.Fatal(claimErr)
		}
		flowStates, stateErr := NewSQLiteToolOwnerSingleCallExecutionStateStoreV2(rawStores[index%len(rawStores)])
		if stateErr != nil {
			t.Fatal(stateErr)
		}
		flows[index], err = NewDurableToolOwnerSingleCallFlowV2(execution, fixture.execution, flowClaims, flowStates, exactDurableFactStoreV2{result: fixture.execution.result}, fixture.binding.clock)
		if err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	errs := make(chan error, len(flows))
	for _, flow := range flows {
		wg.Add(1)
		go func(flow *ToolOwnerSingleCallFlowImplV2) {
			defer wg.Done()
			_, callErr := flow.StartOrInspectToolOwnerSingleCallV2(context.Background(), input)
			errs <- callErr
		}(flow)
	}
	wg.Wait()
	close(errs)
	for runErr := range errs {
		if runErr != nil {
			t.Fatal(runErr)
		}
	}
	execution.mu.Lock()
	startCalls, inspectCalls := execution.startCalls, execution.inspectCalls
	execution.mu.Unlock()
	if startCalls != 0 || inspectCalls != 1 {
		t.Fatalf("independent inspect-only flows StartOrInspect=%d Inspect=%d, want 0/1", startCalls, inspectCalls)
	}
}

func TestSQLiteDurableToolOwnerInspectOnlyHolderExitHandsOffWithFixedClockV2(t *testing.T) {
	fixture := newAdapterV2Fixture(t)
	input := ToolOwnerSingleCallExecutionV2{Request: fixture.binding.request.ApplicationRequest, Binding: fixture.projection}
	config := toolsqlite.ConfigV1{
		Path: filepath.Join(t.TempDir(), "inspect-holder-exit.db"), Owner: core.OwnerRef{Domain: "tool-mcp", ID: "owner-v2"},
		Clock: func() time.Time { return fixture.binding.now },
	}
	rawStores := make([]*toolsqlite.OwnerClaimExecutionStoreV2, 8)
	var err error
	for index := range rawStores {
		rawStores[index], err = toolsqlite.OpenOwnerClaimExecutionStoreV2(context.Background(), config)
		if err != nil {
			t.Fatal(err)
		}
		defer rawStores[index].Close()
	}
	claims, _ := NewSQLiteToolOwnerSingleCallClaimStoreV2(rawStores[0])
	states, _ := NewSQLiteToolOwnerSingleCallExecutionStateStoreV2(rawStores[0])
	claim, err := newToolOwnerSingleCallClaimV2(input, fixture.binding.now.UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	record, _, err := claims.CreateToolOwnerSingleCallClaimV2(context.Background(), ToolOwnerSingleCallClaimRecordV2{Claim: claim, Input: input})
	if err != nil {
		t.Fatal(err)
	}
	state, err := NewToolOwnerSingleCallExecutionStartV2(record, fixture.binding.now.UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	state, _, err = states.CreateExecutionStartV2(context.Background(), state)
	if err != nil {
		t.Fatal(err)
	}
	unknown := ToolOwnerSingleCallExecutionUnknownV2{
		Class: ToolOwnerEntryOutcomeUnknownV2, ErrorDigest: core.DigestBytes([]byte("inspect-only-holder-exit")),
		MarkedUnixNano: state.UpdatedUnixNano + 1,
	}
	state, err = states.AdvanceExecutionInspectOnlyV2(context.Background(), state.RefV2(), unknown)
	if err != nil {
		t.Fatal(err)
	}

	execution := &barrierInspectExecutionV2{result: fixture.execution.result, release: make(chan struct{})}
	holderClaims, _ := NewSQLiteToolOwnerSingleCallClaimStoreV2(rawStores[0])
	holderStates, _ := NewSQLiteToolOwnerSingleCallExecutionStateStoreV2(rawStores[0])
	failingStates := &failAfterEntryLeaseAcquireStateStoreV2{
		base: holderStates, leases: holderStates, acquired: make(chan struct{}), failRelease: make(chan struct{}),
	}
	holderID, _ := toolcontract.StableID("test-entry-holder", "fixed-clock-exit")
	holderFlow, err := NewDurableToolOwnerSingleCallFlowWithEntryLeaseV2(
		execution, fixture.execution, holderClaims, failingStates,
		exactDurableFactStoreV2{result: fixture.execution.result}, fixture.binding.clock,
		5*time.Second, 2*time.Second, holderID,
	)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	type callResult struct {
		index int
		err   error
	}
	results := make(chan callResult, 64)
	go func() {
		_, callErr := holderFlow.StartOrInspectToolOwnerSingleCallV2(ctx, input)
		results <- callResult{index: 0, err: callErr}
	}()
	select {
	case <-failingStates.acquired:
	case <-ctx.Done():
		t.Fatal("failing holder did not acquire the durable lease")
	}

	for index := 1; index < 64; index++ {
		flowClaims, claimErr := NewSQLiteToolOwnerSingleCallClaimStoreV2(rawStores[index%len(rawStores)])
		if claimErr != nil {
			t.Fatal(claimErr)
		}
		flowStates, stateErr := NewSQLiteToolOwnerSingleCallExecutionStateStoreV2(rawStores[index%len(rawStores)])
		if stateErr != nil {
			t.Fatal(stateErr)
		}
		flow, flowErr := NewDurableToolOwnerSingleCallFlowV2(
			execution, fixture.execution, flowClaims, flowStates,
			exactDurableFactStoreV2{result: fixture.execution.result}, fixture.binding.clock,
		)
		if flowErr != nil {
			t.Fatal(flowErr)
		}
		go func(index int, flow *ToolOwnerSingleCallFlowImplV2) {
			_, callErr := flow.StartOrInspectToolOwnerSingleCallV2(ctx, input)
			results <- callResult{index: index, err: callErr}
		}(index, flow)
	}
	close(failingStates.failRelease)

	holderErrors, waiterErrors := 0, 0
	for range 64 {
		select {
		case result := <-results:
			if result.err != nil {
				if result.index == 0 && core.HasCategory(result.err, core.ErrorUnavailable) {
					holderErrors++
				} else {
					waiterErrors++
				}
			}
		case <-ctx.Done():
			t.Fatalf("fixed-clock holder handoff did not converge: %v", ctx.Err())
		}
	}
	if holderErrors != 1 || waiterErrors != 0 {
		t.Fatalf("holder errors=%d waiter errors=%d, want 1/0", holderErrors, waiterErrors)
	}
	execution.mu.Lock()
	startCalls, inspectCalls := execution.startCalls, execution.inspectCalls
	execution.mu.Unlock()
	if startCalls != 0 || inspectCalls != 1 {
		t.Fatalf("fixed-clock holder handoff StartOrInspect=%d Inspect=%d, want 0/1", startCalls, inspectCalls)
	}
	current, err := states.InspectExecutionStateV2(context.Background(), mustToolOwnerInspectKeyV2(input))
	if err != nil || current.State != ToolOwnerExecutionSettledV2 || current.ExecutionAttemptID != state.ExecutionAttemptID {
		t.Fatalf("fixed-clock holder handoff current=%#v err=%v", current, err)
	}
	db, err := sql.Open("sqlite", config.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var handoffCount int
	if err = db.QueryRow(`SELECT COUNT(*) FROM tool_owner_entry_lease_history_v2 WHERE phase='handoff_inspect' AND execution_attempt_id=?`, state.ExecutionAttemptID).Scan(&handoffCount); err != nil {
		t.Fatal(err)
	}
	if handoffCount != 1 {
		t.Fatalf("durable inspect handoff facts=%d, want 1", handoffCount)
	}
}

func TestDurableToolOwnerPostActualFailureHandsOffToExactInspectOnlyV2(t *testing.T) {
	fixture := newAdapterV2Fixture(t)
	input := ToolOwnerSingleCallExecutionV2{Request: fixture.binding.request.ApplicationRequest, Binding: fixture.projection}
	states := NewInMemoryToolOwnerSingleCallExecutionStateStoreV2()
	execution := &postActualInspectHandoffExecutionV2{result: fixture.execution.result}
	first, err := NewDurableToolOwnerSingleCallFlowV2(
		execution, fixture.execution, fixture.claims, states,
		exactDurableFactStoreV2{result: fixture.execution.result}, fixture.binding.clock,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = first.StartOrInspectToolOwnerSingleCallV2(context.Background(), input); err == nil || !core.HasCategory(err, core.ErrorIndeterminate) {
		t.Fatalf("post-actual first holder error=%v, want Indeterminate", err)
	}
	state, err := states.InspectExecutionStateV2(context.Background(), mustToolOwnerInspectKeyV2(input))
	if err != nil || state.State != ToolOwnerExecutionInspectOnlyV2 {
		t.Fatalf("post-actual marker=%#v err=%v, want inspect_only", state, err)
	}
	lease, err := states.InspectExecutionEntryLeaseV2(context.Background(), state.ExecutionAttemptID)
	if err != nil || lease.Phase != ToolOwnerEntryHandoffInspectV2 {
		t.Fatalf("post-actual lease=%#v err=%v, want handoff_inspect", lease, err)
	}

	flows := make([]*ToolOwnerSingleCallFlowImplV2, 64)
	for index := range flows {
		flows[index], err = NewDurableToolOwnerSingleCallFlowV2(
			execution, fixture.execution, fixture.claims, states,
			exactDurableFactStoreV2{result: fixture.execution.result}, fixture.binding.clock,
		)
		if err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	errs := make(chan error, len(flows))
	for _, flow := range flows {
		go func(flow *ToolOwnerSingleCallFlowImplV2) {
			result, callErr := flow.StartOrInspectToolOwnerSingleCallV2(ctx, input)
			if callErr == nil && objectRefResultV1(result) != objectRefResultV1(fixture.execution.result) {
				callErr = errors.New("post-actual exact Inspect returned another result")
			}
			errs <- callErr
		}(flow)
	}
	for range flows {
		select {
		case callErr := <-errs:
			if callErr != nil {
				t.Fatal(callErr)
			}
		case <-ctx.Done():
			current, _ := states.InspectExecutionStateV2(context.Background(), mustToolOwnerInspectKeyV2(input))
			currentLease, _ := states.InspectExecutionEntryLeaseV2(context.Background(), state.ExecutionAttemptID)
			t.Fatalf("post-actual exact Inspect handoff did not converge: %v state=%s/%d lease=%s/%d start=%d inspect=%d", ctx.Err(), current.State, current.Revision, currentLease.Phase, currentLease.Revision, execution.startCalls.Load(), execution.inspectCalls.Load())
		}
	}
	if execution.startCalls.Load() != 1 || execution.inspectCalls.Load() != 2 {
		t.Fatalf("post-actual calls start=%d inspect=%d, want 1/2", execution.startCalls.Load(), execution.inspectCalls.Load())
	}
	execution.attemptMu.Lock()
	attempts := append([]string(nil), execution.attemptIDs...)
	execution.attemptMu.Unlock()
	for _, attemptID := range attempts {
		if attemptID != state.ExecutionAttemptID {
			t.Fatalf("post-actual handoff changed Attempt: got %q want %q", attemptID, state.ExecutionAttemptID)
		}
	}
}

func TestSQLiteDurableToolOwnerPureInspectNotFoundRestartAllowsExactStartV2(t *testing.T) {
	fixture := newAdapterV2Fixture(t)
	input := ToolOwnerSingleCallExecutionV2{Request: fixture.binding.request.ApplicationRequest, Binding: fixture.projection}
	config := toolsqlite.ConfigV1{
		Path: filepath.Join(t.TempDir(), "inspect-not-found-restart.db"), Owner: core.OwnerRef{Domain: "tool-mcp", ID: "owner-v2"},
		Clock: func() time.Time { return fixture.binding.now },
	}
	raw, err := toolsqlite.OpenOwnerClaimExecutionStoreV2(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	claims, _ := NewSQLiteToolOwnerSingleCallClaimStoreV2(raw)
	states, _ := NewSQLiteToolOwnerSingleCallExecutionStateStoreV2(raw)
	claim, err := newToolOwnerSingleCallClaimV2(input, fixture.binding.now.UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	record, _, err := claims.CreateToolOwnerSingleCallClaimV2(context.Background(), ToolOwnerSingleCallClaimRecordV2{Claim: claim, Input: input})
	if err != nil {
		t.Fatal(err)
	}
	state, err := NewToolOwnerSingleCallExecutionStartV2(record, fixture.binding.now.UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	state, _, err = states.CreateExecutionStartV2(context.Background(), state)
	if err != nil {
		t.Fatal(err)
	}
	execution := &inspectNotFoundThenStartExecutionV2{result: fixture.execution.result}
	first, err := NewDurableToolOwnerSingleCallFlowV2(
		execution, fixture.execution, claims, states,
		exactDurableFactStoreV2{result: fixture.execution.result}, fixture.binding.clock,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = first.InspectToolOwnerSingleCallV2(context.Background(), input); err == nil || !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("pure Inspect error=%v, want NotFound", err)
	}
	current, err := states.InspectExecutionStateV2(context.Background(), state.RequestKey)
	if err != nil || current.State != ToolOwnerExecutionStartCommittedV2 {
		t.Fatalf("pure Inspect marker=%#v err=%v, want start_committed", current, err)
	}
	lease, err := states.InspectExecutionEntryLeaseV2(context.Background(), state.ExecutionAttemptID)
	if err != nil || lease.Phase != ToolOwnerEntryHandoffStartOrInspectV2 {
		t.Fatalf("pure Inspect handoff=%#v err=%v, want handoff_start_or_inspect", lease, err)
	}
	if err = raw.Close(); err != nil {
		t.Fatal(err)
	}

	raw, err = toolsqlite.OpenOwnerClaimExecutionStoreV2(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	claims, _ = NewSQLiteToolOwnerSingleCallClaimStoreV2(raw)
	states, _ = NewSQLiteToolOwnerSingleCallExecutionStateStoreV2(raw)
	second, err := NewDurableToolOwnerSingleCallFlowV2(
		execution, fixture.execution, claims, states,
		exactDurableFactStoreV2{result: fixture.execution.result}, fixture.binding.clock,
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := second.StartOrInspectToolOwnerSingleCallV2(context.Background(), input)
	if err != nil || objectRefResultV1(result) != objectRefResultV1(fixture.execution.result) {
		t.Fatalf("restart StartOrInspect result=%#v err=%v", result, err)
	}
	if execution.startCalls.Load() != 1 || execution.inspectCalls.Load() != 1 {
		t.Fatalf("restart exact calls start=%d inspect=%d, want 1/1", execution.startCalls.Load(), execution.inspectCalls.Load())
	}
	execution.attemptMu.Lock()
	attempts := append([]string(nil), execution.attemptIDs...)
	execution.attemptMu.Unlock()
	if len(attempts) != 2 || attempts[0] != state.ExecutionAttemptID || attempts[1] != state.ExecutionAttemptID {
		t.Fatalf("restart changed exact Attempt: %v want %q twice", attempts, state.ExecutionAttemptID)
	}
}

func TestDurableToolOwnerAcquireUnknownConsumesCompatibleHandoffV2(t *testing.T) {
	fixture := newAdapterV2Fixture(t)
	input := ToolOwnerSingleCallExecutionV2{Request: fixture.binding.request.ApplicationRequest, Binding: fixture.projection}
	states := NewInMemoryToolOwnerSingleCallExecutionStateStoreV2()
	claim, err := newToolOwnerSingleCallClaimV2(input, fixture.binding.now.UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	record, _, err := fixture.claims.CreateToolOwnerSingleCallClaimV2(context.Background(), ToolOwnerSingleCallClaimRecordV2{Claim: claim, Input: input})
	if err != nil {
		t.Fatal(err)
	}
	state, err := NewToolOwnerSingleCallExecutionStartV2(record, fixture.binding.now.UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	state, _, err = states.CreateExecutionStartV2(context.Background(), state)
	if err != nil {
		t.Fatal(err)
	}
	priorHolder, _ := toolcontract.StableID("test-entry-holder", "acquire-unknown-prior")
	lease, acquired, err := states.TryAcquireExecutionEntryLeaseV2(context.Background(), ToolOwnerSingleCallEntryLeaseAcquireV2{
		RequestKey: state.RequestKey, RequestDigest: state.RequestDigest, ExecutionInputDigest: state.ExecutionInputDigest,
		ExecutionAttemptID: state.ExecutionAttemptID, HolderIncarnationID: priorHolder, Phase: ToolOwnerEntryStartOrInspectV2,
		AcquiredUnixNano: fixture.binding.now.UnixNano(), ExpiresUnixNano: fixture.binding.now.Add(2 * time.Second).UnixNano(),
	})
	if err != nil || !acquired {
		t.Fatalf("prior lease acquired=%v err=%v", acquired, err)
	}
	if _, err = states.AdvanceExecutionEntryLeaseHandoffV2(context.Background(), lease, ToolOwnerEntryStartOrInspectV2, fixture.binding.now.UnixNano()); err != nil {
		t.Fatal(err)
	}
	wrapped := &indeterminateAcquireHandoffStateStoreV2{base: states}
	execution := &capturingExecutionV2{base: fixture.execution}
	flow, err := NewDurableToolOwnerSingleCallFlowV2(
		execution, fixture.execution, fixture.claims, wrapped,
		exactDurableFactStoreV2{result: fixture.execution.result}, fixture.binding.clock,
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := flow.StartOrInspectToolOwnerSingleCallV2(context.Background(), input)
	if err != nil || objectRefResultV1(result) != objectRefResultV1(fixture.execution.result) {
		t.Fatalf("acquire unknown handoff result=%#v err=%v", result, err)
	}
	execution.mu.Lock()
	attempts := append([]string(nil), execution.attemptIDs...)
	execution.mu.Unlock()
	if len(attempts) != 1 || attempts[0] != state.ExecutionAttemptID {
		t.Fatalf("acquire unknown handoff attempts=%v, want exact %q", attempts, state.ExecutionAttemptID)
	}
	currentLease, err := states.InspectExecutionEntryLeaseV2(context.Background(), state.ExecutionAttemptID)
	if err != nil || currentLease.Revision != lease.Revision+2 || currentLease.Phase != ToolOwnerEntryStartOrInspectV2 {
		t.Fatalf("acquire unknown handoff lease=%#v err=%v, want active revision %d", currentLease, err, lease.Revision+2)
	}
	if fixture.execution.executeCalls.Load() != 1 || fixture.execution.inspectCalls.Load() != 0 {
		t.Fatalf("acquire unknown handoff external start=%d inspect=%d, want 1/0", fixture.execution.executeCalls.Load(), fixture.execution.inspectCalls.Load())
	}
}

func TestDurableToolOwnerAcquireUnknownConsumesInspectHandoffV2(t *testing.T) {
	fixture := newAdapterV2Fixture(t)
	fixture.execution.ready.Store(true)
	input := ToolOwnerSingleCallExecutionV2{Request: fixture.binding.request.ApplicationRequest, Binding: fixture.projection}
	states := NewInMemoryToolOwnerSingleCallExecutionStateStoreV2()
	claim, err := newToolOwnerSingleCallClaimV2(input, fixture.binding.now.UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	record, _, err := fixture.claims.CreateToolOwnerSingleCallClaimV2(context.Background(), ToolOwnerSingleCallClaimRecordV2{Claim: claim, Input: input})
	if err != nil {
		t.Fatal(err)
	}
	state, err := NewToolOwnerSingleCallExecutionStartV2(record, fixture.binding.now.UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	state, _, err = states.CreateExecutionStartV2(context.Background(), state)
	if err != nil {
		t.Fatal(err)
	}
	unknown := ToolOwnerSingleCallExecutionUnknownV2{
		Class: ToolOwnerEntryOutcomeUnknownV2, ErrorDigest: core.DigestBytes([]byte("inspect-handoff")),
		MarkedUnixNano: state.UpdatedUnixNano + 1,
	}
	state, err = states.AdvanceExecutionInspectOnlyV2(context.Background(), state.RefV2(), unknown)
	if err != nil {
		t.Fatal(err)
	}
	priorHolder, _ := toolcontract.StableID("test-entry-holder", "acquire-unknown-inspect")
	lease, acquired, err := states.TryAcquireExecutionEntryLeaseV2(context.Background(), ToolOwnerSingleCallEntryLeaseAcquireV2{
		RequestKey: state.RequestKey, RequestDigest: state.RequestDigest, ExecutionInputDigest: state.ExecutionInputDigest,
		ExecutionAttemptID: state.ExecutionAttemptID, HolderIncarnationID: priorHolder, Phase: ToolOwnerEntryInspectV2,
		AcquiredUnixNano: fixture.binding.now.UnixNano(), ExpiresUnixNano: fixture.binding.now.Add(2 * time.Second).UnixNano(),
	})
	if err != nil || !acquired {
		t.Fatalf("prior inspect lease acquired=%v err=%v", acquired, err)
	}
	if _, err = states.AdvanceExecutionEntryLeaseHandoffV2(context.Background(), lease, ToolOwnerEntryInspectV2, fixture.binding.now.UnixNano()); err != nil {
		t.Fatal(err)
	}
	wrapped := &indeterminateAcquireHandoffStateStoreV2{base: states}
	execution := &capturingExecutionV2{base: fixture.execution}
	flow, err := NewDurableToolOwnerSingleCallFlowV2(
		execution, fixture.execution, fixture.claims, wrapped,
		exactDurableFactStoreV2{result: fixture.execution.result}, fixture.binding.clock,
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := flow.StartOrInspectToolOwnerSingleCallV2(context.Background(), input)
	if err != nil || objectRefResultV1(result) != objectRefResultV1(fixture.execution.result) {
		t.Fatalf("acquire unknown inspect handoff result=%#v err=%v", result, err)
	}
	currentLease, err := states.InspectExecutionEntryLeaseV2(context.Background(), state.ExecutionAttemptID)
	if err != nil || currentLease.Revision != lease.Revision+2 || currentLease.Phase != ToolOwnerEntryInspectV2 {
		t.Fatalf("acquire unknown inspect lease=%#v err=%v, want active revision %d", currentLease, err, lease.Revision+2)
	}
	if fixture.execution.executeCalls.Load() != 0 || fixture.execution.inspectCalls.Load() != 1 {
		t.Fatalf("acquire unknown inspect external start=%d inspect=%d, want 0/1", fixture.execution.executeCalls.Load(), fixture.execution.inspectCalls.Load())
	}
}

func TestDurableToolOwnerAcquireUnknownConsumesInspectHandoffBeforeStartV2(t *testing.T) {
	fixture := newAdapterV2Fixture(t)
	input := ToolOwnerSingleCallExecutionV2{Request: fixture.binding.request.ApplicationRequest, Binding: fixture.projection}
	states := NewInMemoryToolOwnerSingleCallExecutionStateStoreV2()
	claim, err := newToolOwnerSingleCallClaimV2(input, fixture.binding.now.UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	record, _, err := fixture.claims.CreateToolOwnerSingleCallClaimV2(context.Background(), ToolOwnerSingleCallClaimRecordV2{Claim: claim, Input: input})
	if err != nil {
		t.Fatal(err)
	}
	state, err := NewToolOwnerSingleCallExecutionStartV2(record, fixture.binding.now.UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	state, _, err = states.CreateExecutionStartV2(context.Background(), state)
	if err != nil {
		t.Fatal(err)
	}
	priorHolder, _ := toolcontract.StableID("test-entry-holder", "opposite-handoff")
	lease, acquired, err := states.TryAcquireExecutionEntryLeaseV2(context.Background(), ToolOwnerSingleCallEntryLeaseAcquireV2{
		RequestKey: state.RequestKey, RequestDigest: state.RequestDigest, ExecutionInputDigest: state.ExecutionInputDigest,
		ExecutionAttemptID: state.ExecutionAttemptID, HolderIncarnationID: priorHolder, Phase: ToolOwnerEntryInspectV2,
		AcquiredUnixNano: fixture.binding.now.UnixNano(), ExpiresUnixNano: fixture.binding.now.Add(2 * time.Second).UnixNano(),
	})
	if err != nil || !acquired {
		t.Fatalf("prior opposite lease acquired=%v err=%v", acquired, err)
	}
	if _, err = states.AdvanceExecutionEntryLeaseHandoffV2(context.Background(), lease, ToolOwnerEntryInspectV2, fixture.binding.now.UnixNano()); err != nil {
		t.Fatal(err)
	}
	wrapped := &indeterminateAcquireHandoffStateStoreV2{base: states}
	execution := &indeterminateStartOrInspectExecutionV2{}
	flow, err := NewDurableToolOwnerSingleCallFlowV2(
		execution, fixture.execution, fixture.claims, wrapped,
		exactDurableFactStoreV2{result: fixture.execution.result}, fixture.binding.clock,
	)
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	if _, err = flow.StartOrInspectToolOwnerSingleCallV2(context.Background(), input); err == nil || !core.HasCategory(err, core.ErrorIndeterminate) {
		t.Fatalf("inspect handoff error=%v, want Indeterminate", err)
	}
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Fatalf("inspect handoff waited for fixed Owner clock: %s", elapsed)
	}
	if execution.startCalls.Load() != 0 || execution.inspectCalls.Load() != 1 {
		t.Fatalf("inspect handoff external start=%d inspect=%d, want 0/1", execution.startCalls.Load(), execution.inspectCalls.Load())
	}
	current, err := states.InspectExecutionStateV2(context.Background(), state.RequestKey)
	if err != nil || current.State != ToolOwnerExecutionInspectOnlyV2 {
		t.Fatalf("inspect handoff state=%#v err=%v, want inspect_only", current, err)
	}
	currentLease, err := states.InspectExecutionEntryLeaseV2(context.Background(), state.ExecutionAttemptID)
	if err != nil || currentLease.Phase != ToolOwnerEntryHandoffInspectV2 {
		t.Fatalf("inspect handoff lease=%#v err=%v, want handoff_inspect", currentLease, err)
	}
}

func TestDurableToolOwnerPureInspectCannotConsumeStartHandoffV2(t *testing.T) {
	fixture := newAdapterV2Fixture(t)
	input := ToolOwnerSingleCallExecutionV2{Request: fixture.binding.request.ApplicationRequest, Binding: fixture.projection}
	states := NewInMemoryToolOwnerSingleCallExecutionStateStoreV2()
	claim, err := newToolOwnerSingleCallClaimV2(input, fixture.binding.now.UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	record, _, err := fixture.claims.CreateToolOwnerSingleCallClaimV2(context.Background(), ToolOwnerSingleCallClaimRecordV2{Claim: claim, Input: input})
	if err != nil {
		t.Fatal(err)
	}
	state, err := NewToolOwnerSingleCallExecutionStartV2(record, fixture.binding.now.UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	state, _, err = states.CreateExecutionStartV2(context.Background(), state)
	if err != nil {
		t.Fatal(err)
	}
	holder, _ := toolcontract.StableID("test-entry-holder", "pure-inspect-start-handoff")
	lease, acquired, err := states.TryAcquireExecutionEntryLeaseV2(context.Background(), ToolOwnerSingleCallEntryLeaseAcquireV2{
		RequestKey: state.RequestKey, RequestDigest: state.RequestDigest, ExecutionInputDigest: state.ExecutionInputDigest,
		ExecutionAttemptID: state.ExecutionAttemptID, HolderIncarnationID: holder, Phase: ToolOwnerEntryStartOrInspectV2,
		AcquiredUnixNano: fixture.binding.now.UnixNano(), ExpiresUnixNano: fixture.binding.now.Add(2 * time.Second).UnixNano(),
	})
	if err != nil || !acquired {
		t.Fatalf("start lease acquired=%v err=%v", acquired, err)
	}
	if _, err = states.AdvanceExecutionEntryLeaseHandoffV2(context.Background(), lease, ToolOwnerEntryStartOrInspectV2, fixture.binding.now.UnixNano()); err != nil {
		t.Fatal(err)
	}
	execution := &indeterminateStartOrInspectExecutionV2{}
	flow, err := NewDurableToolOwnerSingleCallFlowV2(
		execution, fixture.execution, fixture.claims, states,
		exactDurableFactStoreV2{result: fixture.execution.result}, fixture.binding.clock,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = flow.InspectToolOwnerSingleCallV2(context.Background(), input); err == nil || !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("pure Inspect start-handoff error=%v, want Conflict", err)
	}
	if execution.startCalls.Load() != 0 || execution.inspectCalls.Load() != 0 {
		t.Fatalf("pure Inspect start-handoff external start=%d inspect=%d, want 0/0", execution.startCalls.Load(), execution.inspectCalls.Load())
	}
}

func TestDurableToolOwnerSameFlowEntryGateWaiterDeadlineV2(t *testing.T) {
	fixture := newAdapterV2Fixture(t)
	input := ToolOwnerSingleCallExecutionV2{Request: fixture.binding.request.ApplicationRequest, Binding: fixture.projection}
	execution := &blockingStartExecutionV2{
		result: fixture.execution.result, entered: make(chan struct{}), release: make(chan struct{}),
	}
	flow, err := NewDurableToolOwnerSingleCallFlowV2(
		execution, fixture.execution, fixture.claims, NewInMemoryToolOwnerSingleCallExecutionStateStoreV2(),
		exactDurableFactStoreV2{result: fixture.execution.result}, fixture.binding.clock,
	)
	if err != nil {
		t.Fatal(err)
	}
	holderResult := make(chan error, 1)
	go func() {
		_, callErr := flow.StartOrInspectToolOwnerSingleCallV2(context.Background(), input)
		holderResult <- callErr
	}()
	select {
	case <-execution.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("holder did not reach the external create-once seam")
	}
	waiterCtx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	started := time.Now()
	if _, err = flow.StartOrInspectToolOwnerSingleCallV2(waiterCtx, input); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("same-flow gate waiter error=%v, want DeadlineExceeded", err)
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("same-flow gate ignored waiter cancellation for %s", elapsed)
	}
	if execution.startCalls.Load() != 1 || execution.inspectCalls.Load() != 0 {
		t.Fatalf("cancelled gate waiter crossed external seam start=%d inspect=%d", execution.startCalls.Load(), execution.inspectCalls.Load())
	}
	close(execution.release)
	select {
	case err = <-holderResult:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("holder did not finish after release")
	}
	if execution.startCalls.Load() != 1 || execution.inspectCalls.Load() != 0 {
		t.Fatalf("same-flow holder final calls start=%d inspect=%d, want 1/0", execution.startCalls.Load(), execution.inspectCalls.Load())
	}
}

func TestSQLiteDurableToolOwnerEntryLeaseExpiredHolderTakeoverKeepsExactAttempt(t *testing.T) {
	fixture := newAdapterV2Fixture(t)
	input := ToolOwnerSingleCallExecutionV2{Request: fixture.binding.request.ApplicationRequest, Binding: fixture.projection}
	now := fixture.binding.now
	clock := &durableMutableClockV2{now: now}
	config := toolsqlite.ConfigV1{
		Path: filepath.Join(t.TempDir(), "lease-takeover.db"), Owner: core.OwnerRef{Domain: "tool-mcp", ID: "owner-v2"},
		Clock: clock.Now,
	}
	raw, err := toolsqlite.OpenOwnerClaimExecutionStoreV2(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	claims, _ := NewSQLiteToolOwnerSingleCallClaimStoreV2(raw)
	states, _ := NewSQLiteToolOwnerSingleCallExecutionStateStoreV2(raw)
	claim, err := newToolOwnerSingleCallClaimV2(input, now.UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	record, _, err := claims.CreateToolOwnerSingleCallClaimV2(context.Background(), ToolOwnerSingleCallClaimRecordV2{Claim: claim, Input: input})
	if err != nil {
		t.Fatal(err)
	}
	state, err := NewToolOwnerSingleCallExecutionStartV2(record, now.UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	state, _, err = states.CreateExecutionStartV2(context.Background(), state)
	if err != nil {
		t.Fatal(err)
	}
	holderOne, err := toolcontract.StableID("test-entry-holder", "one")
	if err != nil {
		t.Fatal(err)
	}
	first, acquired, err := states.TryAcquireExecutionEntryLeaseV2(context.Background(), ToolOwnerSingleCallEntryLeaseAcquireV2{
		RequestKey: state.RequestKey, RequestDigest: state.RequestDigest, ExecutionInputDigest: state.ExecutionInputDigest,
		ExecutionAttemptID: state.ExecutionAttemptID, HolderIncarnationID: holderOne, Phase: ToolOwnerEntryStartOrInspectV2,
		AcquiredUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Millisecond).UnixNano(),
	})
	if err != nil || !acquired {
		t.Fatalf("first lease=%#v acquired=%v err=%v", first, acquired, err)
	}
	clock.Set(now.Add(2 * time.Millisecond))
	holderTwo, _ := toolcontract.StableID("test-entry-holder", "two")
	capture := &capturingExecutionV2{base: fixture.execution}
	flow, err := NewDurableToolOwnerSingleCallFlowWithEntryLeaseV2(
		capture, fixture.execution, claims, states, exactDurableFactStoreV2{result: fixture.execution.result},
		clock, time.Second, 10*time.Millisecond, holderTwo,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = flow.StartOrInspectToolOwnerSingleCallV2(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	capture.mu.Lock()
	attempts := append([]string(nil), capture.attemptIDs...)
	capture.mu.Unlock()
	if len(attempts) != 1 || attempts[0] != state.ExecutionAttemptID {
		t.Fatalf("takeover attempts=%v, want exact %q", attempts, state.ExecutionAttemptID)
	}
	current, err := states.InspectExecutionEntryLeaseV2(context.Background(), state.ExecutionAttemptID)
	if err != nil || current.Revision != first.Revision+1 || current.HolderIncarnationID != holderTwo {
		t.Fatalf("takeover lease=%#v err=%v", current, err)
	}
}

func TestDurableToolOwnerFlowRevalidatesTTLImmediatelyBeforeExternalEntry(t *testing.T) {
	fixture := newAdapterV2Fixture(t)
	input := ToolOwnerSingleCallExecutionV2{Request: fixture.binding.request.ApplicationRequest, Binding: fixture.projection}
	expires := time.Unix(0, input.Binding.ExpiresUnixNano)
	clock := &scriptedClockV2{values: []time.Time{fixture.binding.now, expires}}
	flow, err := NewDurableToolOwnerSingleCallFlowV2(
		ownerStartOrInspectAdapterV2{base: fixture.execution},
		fixture.execution,
		fixture.claims,
		NewInMemoryToolOwnerSingleCallExecutionStateStoreV2(),
		exactDurableFactStoreV2{result: fixture.execution.result},
		clock,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = flow.StartOrInspectToolOwnerSingleCallV2(context.Background(), input); err == nil || !core.HasCategory(err, core.ErrorPreconditionFailed) {
		t.Fatalf("TTL crossing error=%v, want PreconditionFailed", err)
	}
	if fixture.execution.executeCalls.Load() != 0 || fixture.execution.inspectCalls.Load() != 0 {
		t.Fatalf("expired entry reached downstream start=%d inspect=%d", fixture.execution.executeCalls.Load(), fixture.execution.inspectCalls.Load())
	}
}

func TestDurableToolOwnerFlowInspectOnlyRevalidatesTTLBeforeExternalInspect(t *testing.T) {
	fixture := newAdapterV2Fixture(t)
	input := ToolOwnerSingleCallExecutionV2{Request: fixture.binding.request.ApplicationRequest, Binding: fixture.projection}
	claim, err := newToolOwnerSingleCallClaimV2(input, fixture.binding.now.UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	record := ToolOwnerSingleCallClaimRecordV2{Claim: claim, Input: input}
	if _, _, err = fixture.claims.CreateToolOwnerSingleCallClaimV2(context.Background(), record); err != nil {
		t.Fatal(err)
	}
	states := NewInMemoryToolOwnerSingleCallExecutionStateStoreV2()
	start, err := NewToolOwnerSingleCallExecutionStartV2(record, fixture.binding.now.UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	start, _, err = states.CreateExecutionStartV2(context.Background(), start)
	if err != nil {
		t.Fatal(err)
	}
	unknown := ToolOwnerSingleCallExecutionUnknownV2{Class: ToolOwnerEntryOutcomeUnknownV2, ErrorDigest: core.DigestBytes([]byte("unknown")), MarkedUnixNano: fixture.binding.now.UnixNano()}
	if _, err = states.AdvanceExecutionInspectOnlyV2(context.Background(), start.RefV2(), unknown); err != nil {
		t.Fatal(err)
	}
	execution := &indeterminateStartOrInspectExecutionV2{}
	expires := time.Unix(0, input.Binding.ExpiresUnixNano)
	flow, err := NewDurableToolOwnerSingleCallFlowV2(execution, fixture.execution, fixture.claims, states, exactDurableFactStoreV2{result: fixture.execution.result}, fixedClockV2{now: expires})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = flow.InspectToolOwnerSingleCallV2(context.Background(), input); err == nil || !core.HasCategory(err, core.ErrorPreconditionFailed) {
		t.Fatalf("inspect_only expiry error=%v, want PreconditionFailed", err)
	}
	if execution.startCalls.Load() != 0 || execution.inspectCalls.Load() != 0 {
		t.Fatalf("expired inspect_only reached external start=%d inspect=%d", execution.startCalls.Load(), execution.inspectCalls.Load())
	}
}

func TestDurableToolOwnerLiveCallerWaitUsesOwnerTTLNotWallClock(t *testing.T) {
	fixture := newAdapterV2Fixture(t)
	input := ToolOwnerSingleCallExecutionV2{Request: fixture.binding.request.ApplicationRequest, Binding: fixture.projection}
	now := fixture.binding.now
	expires := time.Unix(0, input.Binding.ExpiresUnixNano)
	clock := &durableMutableClockV2{now: now}
	claim, err := newToolOwnerSingleCallClaimV2(input, now.UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	record, _, err := fixture.claims.CreateToolOwnerSingleCallClaimV2(
		context.Background(),
		ToolOwnerSingleCallClaimRecordV2{Claim: claim, Input: input},
	)
	if err != nil {
		t.Fatal(err)
	}
	states := NewInMemoryToolOwnerSingleCallExecutionStateStoreV2()
	state, err := NewToolOwnerSingleCallExecutionStartV2(record, now.UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	state, _, err = states.CreateExecutionStartV2(context.Background(), state)
	if err != nil {
		t.Fatal(err)
	}
	holderOne, _ := toolcontract.StableID("test-entry-holder", "ttl-owner")
	if _, acquired, acquireErr := states.TryAcquireExecutionEntryLeaseV2(context.Background(), ToolOwnerSingleCallEntryLeaseAcquireV2{
		RequestKey: state.RequestKey, RequestDigest: state.RequestDigest, ExecutionInputDigest: state.ExecutionInputDigest,
		ExecutionAttemptID: state.ExecutionAttemptID, HolderIncarnationID: holderOne, Phase: ToolOwnerEntryStartOrInspectV2,
		AcquiredUnixNano: now.UnixNano(), ExpiresUnixNano: expires.UnixNano(),
	}); acquireErr != nil || !acquired {
		t.Fatalf("foreign lease acquired=%v err=%v", acquired, acquireErr)
	}
	holderTwo, _ := toolcontract.StableID("test-entry-holder", "ttl-waiter")
	execution := &indeterminateStartOrInspectExecutionV2{}
	flow, err := NewDurableToolOwnerSingleCallFlowWithEntryLeaseV2(
		execution,
		fixture.execution,
		fixture.claims,
		states,
		exactDurableFactStoreV2{result: fixture.execution.result},
		clock,
		10*time.Second,
		2*time.Second,
		holderTwo,
	)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	resultErr := make(chan error, 1)
	go func() {
		_, callErr := flow.StartOrInspectToolOwnerSingleCallV2(ctx, input)
		resultErr <- callErr
	}()
	time.Sleep(10 * time.Millisecond)
	crossedAt := time.Now()
	clock.Set(expires)
	err = <-resultErr
	if err == nil || !core.HasCategory(err, core.ErrorPreconditionFailed) {
		t.Fatalf("owner TTL error=%v, want PreconditionFailed", err)
	}
	if maxToolOwnerEntryPollDelayV2 != 100*time.Millisecond {
		t.Fatalf("maximum owner entry poll delay=%s, want 100ms", maxToolOwnerEntryPollDelayV2)
	}
	if elapsed := time.Since(crossedAt); elapsed > 5*time.Second {
		t.Fatalf("owner TTL crossing took %s beyond scheduler-tolerant polling bound", elapsed)
	}
	if ctx.Err() != nil {
		t.Fatalf("live caller expired before owner TTL result: %v", ctx.Err())
	}
	if execution.startCalls.Load() != 0 || execution.inspectCalls.Load() != 0 {
		t.Fatalf("expired owner current reached external start=%d inspect=%d", execution.startCalls.Load(), execution.inspectCalls.Load())
	}
}

func TestDurableToolOwnerLiveWaiterCallerCancelExitsWithoutExternalEntry(t *testing.T) {
	fixture := newAdapterV2Fixture(t)
	input := ToolOwnerSingleCallExecutionV2{Request: fixture.binding.request.ApplicationRequest, Binding: fixture.projection}
	now := fixture.binding.now
	claim, err := newToolOwnerSingleCallClaimV2(input, now.UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	record, _, err := fixture.claims.CreateToolOwnerSingleCallClaimV2(
		context.Background(),
		ToolOwnerSingleCallClaimRecordV2{Claim: claim, Input: input},
	)
	if err != nil {
		t.Fatal(err)
	}
	states := NewInMemoryToolOwnerSingleCallExecutionStateStoreV2()
	state, err := NewToolOwnerSingleCallExecutionStartV2(record, now.UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	state, _, err = states.CreateExecutionStartV2(context.Background(), state)
	if err != nil {
		t.Fatal(err)
	}
	holderOne, _ := toolcontract.StableID("test-entry-holder", "cancel-owner")
	if _, acquired, acquireErr := states.TryAcquireExecutionEntryLeaseV2(context.Background(), ToolOwnerSingleCallEntryLeaseAcquireV2{
		RequestKey: state.RequestKey, RequestDigest: state.RequestDigest, ExecutionInputDigest: state.ExecutionInputDigest,
		ExecutionAttemptID: state.ExecutionAttemptID, HolderIncarnationID: holderOne, Phase: ToolOwnerEntryStartOrInspectV2,
		AcquiredUnixNano: now.UnixNano(), ExpiresUnixNano: time.Unix(0, input.Binding.ExpiresUnixNano).UnixNano(),
	}); acquireErr != nil || !acquired {
		t.Fatalf("foreign lease acquired=%v err=%v", acquired, acquireErr)
	}
	holderTwo, _ := toolcontract.StableID("test-entry-holder", "cancel-waiter")
	execution := &indeterminateStartOrInspectExecutionV2{}
	flow, err := NewDurableToolOwnerSingleCallFlowWithEntryLeaseV2(
		execution,
		fixture.execution,
		fixture.claims,
		states,
		exactDurableFactStoreV2{result: fixture.execution.result},
		fixture.binding.clock,
		10*time.Second,
		2*time.Second,
		holderTwo,
	)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	resultErr := make(chan error, 1)
	go func() {
		_, callErr := flow.StartOrInspectToolOwnerSingleCallV2(ctx, input)
		resultErr <- callErr
	}()
	time.Sleep(10 * time.Millisecond)
	cancelledAt := time.Now()
	cancel()
	select {
	case err = <-resultErr:
	case <-time.After(5 * time.Second):
		t.Fatal("live waiter did not observe caller cancellation within polling tolerance")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("live waiter error=%v, want context.Canceled", err)
	}
	if elapsed := time.Since(cancelledAt); elapsed > 5*time.Second {
		t.Fatalf("live waiter cancellation took %s", elapsed)
	}
	if execution.startCalls.Load() != 0 || execution.inspectCalls.Load() != 0 {
		t.Fatalf("cancelled waiter reached external start=%d inspect=%d", execution.startCalls.Load(), execution.inspectCalls.Load())
	}
}

func TestSQLiteToolOwnerEntryLease64MixedPayloadsCreateOneWinner(t *testing.T) {
	fixture := newAdapterV2Fixture(t)
	input := ToolOwnerSingleCallExecutionV2{Request: fixture.binding.request.ApplicationRequest, Binding: fixture.projection}
	claim, err := newToolOwnerSingleCallClaimV2(input, fixture.binding.now.UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	record := ToolOwnerSingleCallClaimRecordV2{Claim: claim, Input: input}
	state, err := NewToolOwnerSingleCallExecutionStartV2(record, fixture.binding.now.UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	config := toolsqlite.ConfigV1{
		Path: filepath.Join(t.TempDir(), "mixed-entry-lease.db"), Owner: core.OwnerRef{Domain: "tool-mcp", ID: "owner-v2"},
		Clock: func() time.Time { return fixture.binding.now },
	}
	stores := make([]*toolsqlite.OwnerClaimExecutionStoreV2, 8)
	leases := make([]*SQLiteToolOwnerSingleCallExecutionStateStoreV2, len(stores))
	for index := range stores {
		stores[index], err = toolsqlite.OpenOwnerClaimExecutionStoreV2(context.Background(), config)
		if err != nil {
			t.Fatal(err)
		}
		defer stores[index].Close()
		leases[index], err = NewSQLiteToolOwnerSingleCallExecutionStateStoreV2(stores[index])
		if err != nil {
			t.Fatal(err)
		}
	}
	type leaseResult struct {
		request  ToolOwnerSingleCallEntryLeaseAcquireV2
		lease    ToolOwnerSingleCallEntryLeaseV2
		acquired bool
		err      error
	}
	results := make(chan leaseResult, 64)
	var wg sync.WaitGroup
	for index := range 64 {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			inputDigest := state.ExecutionInputDigest
			if index%2 == 1 {
				inputDigest = core.DigestBytes([]byte("mixed-entry-input"))
			}
			holder, _ := toolcontract.StableID("mixed-entry-holder", strconv.Itoa(index))
			request := ToolOwnerSingleCallEntryLeaseAcquireV2{
				RequestKey: state.RequestKey, RequestDigest: state.RequestDigest, ExecutionInputDigest: inputDigest,
				ExecutionAttemptID: state.ExecutionAttemptID, HolderIncarnationID: holder, Phase: ToolOwnerEntryStartOrInspectV2,
				AcquiredUnixNano: fixture.binding.now.UnixNano(), ExpiresUnixNano: fixture.binding.now.Add(time.Second).UnixNano(),
			}
			lease, acquired, callErr := leases[index%len(leases)].TryAcquireExecutionEntryLeaseV2(context.Background(), request)
			results <- leaseResult{request: request, lease: lease, acquired: acquired, err: callErr}
		}(index)
	}
	wg.Wait()
	close(results)
	winner, err := leases[0].InspectExecutionEntryLeaseV2(context.Background(), state.ExecutionAttemptID)
	if err != nil {
		t.Fatal(err)
	}
	acquiredCount, replayCount, conflictCount := 0, 0, 0
	for result := range results {
		if result.err != nil {
			if !core.HasCategory(result.err, core.ErrorConflict) ||
				result.request.ExecutionInputDigest == winner.ExecutionInputDigest {
				t.Fatalf("mixed lease error=%v requestInput=%s winnerInput=%s", result.err, result.request.ExecutionInputDigest, winner.ExecutionInputDigest)
			}
			conflictCount++
			continue
		}
		if result.lease.RefV2() != winner.RefV2() ||
			result.request.ExecutionInputDigest != winner.ExecutionInputDigest {
			t.Fatalf("mixed lease success drifted: result=%#v winner=%#v", result, winner)
		}
		if result.acquired {
			acquiredCount++
		} else {
			replayCount++
		}
	}
	if acquiredCount != 1 || replayCount != 31 || conflictCount != 32 {
		t.Fatalf("mixed lease acquired/replay/conflict=%d/%d/%d, want 1/31/32", acquiredCount, replayCount, conflictCount)
	}
}

func TestToolOwnerEntryLeaseSamePhaseABAReplayFailsClosedV2(t *testing.T) {
	fixture := newAdapterV2Fixture(t)
	input := ToolOwnerSingleCallExecutionV2{Request: fixture.binding.request.ApplicationRequest, Binding: fixture.projection}
	claim, err := newToolOwnerSingleCallClaimV2(input, fixture.binding.now.UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	record := ToolOwnerSingleCallClaimRecordV2{Claim: claim, Input: input}
	state, err := NewToolOwnerSingleCallExecutionStartV2(record, fixture.binding.now.UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	key := mustToolOwnerInspectKeyV2(input)
	inputDigest, err := ComputeToolOwnerSingleCallExecutionInputDigestV2(input)
	if err != nil {
		t.Fatal(err)
	}
	run := func(t *testing.T, store ToolOwnerSingleCallEntryLeaseStoreV2) {
		t.Helper()
		r1, acquired, err := store.TryAcquireExecutionEntryLeaseV2(context.Background(), ToolOwnerSingleCallEntryLeaseAcquireV2{
			RequestKey: key, RequestDigest: key.RequestDigest, ExecutionInputDigest: inputDigest,
			ExecutionAttemptID: state.ExecutionAttemptID, HolderIncarnationID: "entry-aba-holder-a",
			Phase: ToolOwnerEntryStartOrInspectV2, AcquiredUnixNano: fixture.binding.now.UnixNano(),
			ExpiresUnixNano: fixture.binding.now.Add(10 * time.Second).UnixNano(),
		})
		if err != nil || !acquired {
			t.Fatalf("r1 acquired=%v err=%v", acquired, err)
		}
		r2, err := store.AdvanceExecutionEntryLeaseHandoffV2(context.Background(), r1, ToolOwnerEntryInspectV2, fixture.binding.now.Add(time.Second).UnixNano())
		if err != nil {
			t.Fatal(err)
		}
		r3, acquired, err := store.TryAcquireExecutionEntryLeaseV2(context.Background(), ToolOwnerSingleCallEntryLeaseAcquireV2{
			RequestKey: key, RequestDigest: key.RequestDigest, ExecutionInputDigest: inputDigest,
			ExecutionAttemptID: state.ExecutionAttemptID, HolderIncarnationID: "entry-aba-holder-b",
			Phase: ToolOwnerEntryInspectV2, AcquiredUnixNano: fixture.binding.now.Add(2 * time.Second).UnixNano(),
			ExpiresUnixNano: fixture.binding.now.Add(12 * time.Second).UnixNano(),
		})
		if err != nil || !acquired || r3.Revision != r2.Revision+1 {
			t.Fatalf("r3=%#v acquired=%v err=%v", r3, acquired, err)
		}
		r4, err := store.AdvanceExecutionEntryLeaseHandoffV2(context.Background(), r3, ToolOwnerEntryInspectV2, fixture.binding.now.Add(3*time.Second).UnixNano())
		if err != nil || r4.Phase != ToolOwnerEntryHandoffInspectV2 {
			t.Fatalf("r4=%#v err=%v", r4, err)
		}
		if _, err = store.AdvanceExecutionEntryLeaseHandoffV2(context.Background(), r1, ToolOwnerEntryInspectV2, fixture.binding.now.Add(time.Second).UnixNano()); err == nil || !core.HasCategory(err, core.ErrorConflict) {
			t.Fatalf("same-phase ABA replay error=%v, want Conflict", err)
		}
	}
	t.Run("in_memory", func(t *testing.T) {
		run(t, NewInMemoryToolOwnerSingleCallExecutionStateStoreV2())
	})
	t.Run("sqlite", func(t *testing.T) {
		raw, openErr := toolsqlite.OpenOwnerClaimExecutionStoreV2(context.Background(), toolsqlite.ConfigV1{
			Path: filepath.Join(t.TempDir(), "entry-aba.db"), Owner: core.OwnerRef{Domain: "tool-mcp", ID: "owner-v2"},
			Clock: func() time.Time { return fixture.binding.now },
		})
		if openErr != nil {
			t.Fatal(openErr)
		}
		defer raw.Close()
		store, openErr := NewSQLiteToolOwnerSingleCallExecutionStateStoreV2(raw)
		if openErr != nil {
			t.Fatal(openErr)
		}
		run(t, store)
	})
}

func TestDurableToolOwnerFlowPreEntryFreshGateRevalidatesTTLAndClock(t *testing.T) {
	for _, test := range []struct {
		name   string
		values func(now, expires time.Time) []time.Time
		reason core.ReasonCode
	}{
		{name: "expiry", values: func(now, expires time.Time) []time.Time { return []time.Time{now, now.Add(time.Nanosecond), expires} }, reason: core.ReasonEffectFenceStale},
		{name: "rollback", values: func(now, _ time.Time) []time.Time { return []time.Time{now, now.Add(time.Nanosecond), now} }, reason: core.ReasonClockRegression},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newAdapterV2Fixture(t)
			input := ToolOwnerSingleCallExecutionV2{Request: fixture.binding.request.ApplicationRequest, Binding: fixture.projection}
			execution := &indeterminateStartOrInspectExecutionV2{}
			expires := time.Unix(0, input.Binding.ExpiresUnixNano)
			clock := &scriptedClockV2{values: test.values(fixture.binding.now, expires)}
			flow, err := NewDurableToolOwnerSingleCallFlowV2(execution, fixture.execution, fixture.claims, NewInMemoryToolOwnerSingleCallExecutionStateStoreV2(), exactDurableFactStoreV2{result: fixture.execution.result}, clock)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = flow.StartOrInspectToolOwnerSingleCallV2(context.Background(), input); err == nil || !core.HasReason(err, test.reason) {
				t.Fatalf("recovery error=%v, want reason %s", err, test.reason)
			}
			if execution.startCalls.Load() != 0 || execution.inspectCalls.Load() != 0 {
				t.Fatalf("fresh-gate calls start=%d inspect=%d, want 0/0", execution.startCalls.Load(), execution.inspectCalls.Load())
			}
		})
	}
}

func TestDurableToolOwnerFlowPostEntryRecoveryRevalidatesTTLAndClock(t *testing.T) {
	for _, test := range []struct {
		name   string
		values func(now, expires time.Time) []time.Time
		reason core.ReasonCode
	}{
		{name: "expiry", values: func(now, expires time.Time) []time.Time {
			return []time.Time{now, now.Add(time.Nanosecond), now.Add(2 * time.Nanosecond), expires}
		}, reason: core.ReasonEffectFenceStale},
		{name: "rollback", values: func(now, _ time.Time) []time.Time {
			return []time.Time{now, now.Add(time.Nanosecond), now.Add(2 * time.Nanosecond), now}
		}, reason: core.ReasonClockRegression},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newAdapterV2Fixture(t)
			input := ToolOwnerSingleCallExecutionV2{Request: fixture.binding.request.ApplicationRequest, Binding: fixture.projection}
			execution := &indeterminateStartOrInspectExecutionV2{}
			expires := time.Unix(0, input.Binding.ExpiresUnixNano)
			clock := &scriptedClockV2{values: test.values(fixture.binding.now, expires)}
			flow, err := NewDurableToolOwnerSingleCallFlowV2(execution, fixture.execution, fixture.claims, NewInMemoryToolOwnerSingleCallExecutionStateStoreV2(), exactDurableFactStoreV2{result: fixture.execution.result}, clock)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = flow.StartOrInspectToolOwnerSingleCallV2(context.Background(), input); err == nil || !core.HasReason(err, test.reason) {
				t.Fatalf("post-entry recovery error=%v, want reason %s", err, test.reason)
			}
			if execution.startCalls.Load() != 1 || execution.inspectCalls.Load() != 0 {
				t.Fatalf("post-entry recovery calls start=%d inspect=%d, want 1/0", execution.startCalls.Load(), execution.inspectCalls.Load())
			}
		})
	}
}

func TestDurableToolOwnerActualEntryFreshCurrentGateV2(t *testing.T) {
	for _, entry := range []struct {
		name       string
		inspect    bool
		wantReason core.ReasonCode
		wantCancel bool
	}{
		{name: "start_now_equals_lease_expiry", wantReason: core.ReasonCapabilityExpired},
		{name: "start_clock_regression", wantReason: core.ReasonClockRegression},
		{name: "start_caller_cancelled", wantCancel: true},
		{name: "start_execution_current_drift", wantReason: core.ReasonIdempotencyPayloadMismatch},
		{name: "inspect_now_equals_lease_expiry", inspect: true, wantReason: core.ReasonCapabilityExpired},
		{name: "inspect_clock_regression", inspect: true, wantReason: core.ReasonClockRegression},
		{name: "inspect_caller_cancelled", inspect: true, wantCancel: true},
		{name: "inspect_execution_current_drift", inspect: true, wantReason: core.ReasonIdempotencyPayloadMismatch},
	} {
		t.Run(entry.name, func(t *testing.T) {
			fixture := newAdapterV2Fixture(t)
			input := ToolOwnerSingleCallExecutionV2{Request: fixture.binding.request.ApplicationRequest, Binding: fixture.projection}
			clock := &durableMutableClockV2{now: fixture.binding.now}
			baseStates := NewInMemoryToolOwnerSingleCallExecutionStateStoreV2()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			mode := actualEntryExpireV2
			switch {
			case entry.wantCancel:
				mode = actualEntryCancelV2
			case entry.wantReason == core.ReasonClockRegression:
				mode = actualEntryRegressV2
			case entry.wantReason == core.ReasonIdempotencyPayloadMismatch:
				mode = actualEntryStateDriftV2
			}
			states := &actualEntryMutationStateStoreV2{
				base: baseStates, leases: baseStates, clock: clock, cancel: cancel, mode: mode,
			}
			if entry.inspect {
				claim, err := newToolOwnerSingleCallClaimV2(input, fixture.binding.now.UnixNano())
				if err != nil {
					t.Fatal(err)
				}
				record, _, err := fixture.claims.CreateToolOwnerSingleCallClaimV2(ctx, ToolOwnerSingleCallClaimRecordV2{Claim: claim, Input: input})
				if err != nil {
					t.Fatal(err)
				}
				start, err := NewToolOwnerSingleCallExecutionStartV2(record, fixture.binding.now.UnixNano())
				if err != nil {
					t.Fatal(err)
				}
				if _, _, err = baseStates.CreateExecutionStartV2(ctx, start); err != nil {
					t.Fatal(err)
				}
			}
			execution := &indeterminateStartOrInspectExecutionV2{}
			flow, err := NewDurableToolOwnerSingleCallFlowWithEntryLeaseV2(
				execution, fixture.execution, fixture.claims, states,
				exactDurableFactStoreV2{result: fixture.execution.result},
				clock, 5*time.Second, 2*time.Second, "actual-entry-fresh-"+entry.name,
			)
			if err != nil {
				t.Fatal(err)
			}
			var callErr error
			if entry.inspect {
				_, callErr = flow.InspectToolOwnerSingleCallV2(ctx, input)
			} else {
				_, callErr = flow.StartOrInspectToolOwnerSingleCallV2(ctx, input)
			}
			if entry.wantCancel {
				if !errors.Is(callErr, context.Canceled) {
					t.Fatalf("actual entry error=%v, want context.Canceled", callErr)
				}
			} else if callErr == nil || !core.HasReason(callErr, entry.wantReason) {
				t.Fatalf("actual entry error=%v, want reason %s", callErr, entry.wantReason)
			}
			if execution.startCalls.Load() != 0 || execution.inspectCalls.Load() != 0 {
				t.Fatalf("actual entry external calls start=%d inspect=%d, want 0/0", execution.startCalls.Load(), execution.inspectCalls.Load())
			}
			if states.postAcquireInspects.Load() == 0 {
				t.Fatal("actual entry mutation did not occur after successful lease acquisition")
			}
		})
	}
}

func TestDurableToolOwnerFlowCanceledTransportUsesBoundedRecovery(t *testing.T) {
	fixture := newAdapterV2Fixture(t)
	input := ToolOwnerSingleCallExecutionV2{Request: fixture.binding.request.ApplicationRequest, Binding: fixture.projection}
	ctx, cancel := context.WithCancel(context.Background())
	execution := &cancelThenBlockingInspectExecutionV2{cancel: cancel}
	const recoveryTimeout = 20 * time.Millisecond
	flow, err := NewDurableToolOwnerSingleCallFlowWithRecoveryTimeoutV2(
		execution,
		fixture.execution,
		fixture.claims,
		NewInMemoryToolOwnerSingleCallExecutionStateStoreV2(),
		exactDurableFactStoreV2{result: fixture.execution.result},
		fixture.binding.clock,
		recoveryTimeout,
	)
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	if _, err = flow.StartOrInspectToolOwnerSingleCallV2(ctx, input); err == nil || (!errors.Is(err, context.DeadlineExceeded) && !core.HasCategory(err, core.ErrorIndeterminate)) {
		t.Fatalf("bounded recovery error=%v, want DeadlineExceeded/Indeterminate", err)
	}
	elapsed := time.Since(started)
	if elapsed < recoveryTimeout || elapsed > 100*recoveryTimeout {
		t.Fatalf("bounded recovery elapsed=%s want [%s,%s]", elapsed, recoveryTimeout, 100*recoveryTimeout)
	}
	if execution.startCalls.Load() != 1 || execution.inspectCalls.Load() != 1 {
		t.Fatalf("bounded recovery calls start=%d inspect=%d, want 1/1", execution.startCalls.Load(), execution.inspectCalls.Load())
	}
}

func TestDurableToolOwnerFlowLiveDeadlineBoundsExactRecoveryV2(t *testing.T) {
	fixture := newAdapterV2Fixture(t)
	input := ToolOwnerSingleCallExecutionV2{Request: fixture.binding.request.ApplicationRequest, Binding: fixture.projection}
	execution := &liveDeadlineBlockingInspectExecutionV2{inspectStart: make(chan time.Time, 1)}
	states := NewInMemoryToolOwnerSingleCallExecutionStateStoreV2()
	claim, err := newToolOwnerSingleCallClaimV2(input, fixture.binding.now.UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	record, _, err := fixture.claims.CreateToolOwnerSingleCallClaimV2(context.Background(), ToolOwnerSingleCallClaimRecordV2{Claim: claim, Input: input})
	if err != nil {
		t.Fatal(err)
	}
	startState, err := NewToolOwnerSingleCallExecutionStartV2(record, fixture.binding.now.UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = states.CreateExecutionStartV2(context.Background(), startState); err != nil {
		t.Fatal(err)
	}
	flow, err := NewDurableToolOwnerSingleCallFlowWithRecoveryTimeoutV2(
		execution,
		fixture.execution,
		fixture.claims,
		states,
		exactDurableFactStoreV2{result: fixture.execution.result},
		fixture.binding.clock,
		time.Second,
	)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	resultErr := make(chan error, 1)
	go func() {
		_, callErr := flow.StartOrInspectToolOwnerSingleCallV2(ctx, input)
		resultErr <- callErr
	}()
	var inspectStarted time.Time
	select {
	case inspectStarted = <-execution.inspectStart:
	case <-ctx.Done():
		t.Fatalf("live recovery did not enter exact Inspect before deadline: %v", ctx.Err())
	}
	err = <-resultErr
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("live exact recovery error=%v, want DeadlineExceeded", err)
	}
	if elapsed := time.Since(inspectStarted); elapsed > time.Second {
		t.Fatalf("live exact recovery detached caller deadline for %s", elapsed)
	}
	if execution.startCalls.Load() != 1 || execution.inspectCalls.Load() != 1 {
		t.Fatalf("live exact recovery calls start=%d inspect=%d, want 1/1", execution.startCalls.Load(), execution.inspectCalls.Load())
	}
}

func TestDurableToolOwnerFlowInspectOnlyCASLoserConvergesToIndeterminate(t *testing.T) {
	fixture := newAdapterV2Fixture(t)
	input := ToolOwnerSingleCallExecutionV2{Request: fixture.binding.request.ApplicationRequest, Binding: fixture.projection}
	execution := &indeterminateStartOrInspectExecutionV2{}
	states := &inspectOnlyCommitLostStateStoreV2{base: NewInMemoryToolOwnerSingleCallExecutionStateStoreV2()}
	flow, err := NewDurableToolOwnerSingleCallFlowV2(
		execution,
		fixture.execution,
		fixture.claims,
		states,
		exactDurableFactStoreV2{result: fixture.execution.result},
		fixture.binding.clock,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = flow.StartOrInspectToolOwnerSingleCallV2(context.Background(), input); err == nil || !core.HasCategory(err, core.ErrorIndeterminate) {
		t.Fatalf("inspect_only CAS loser error=%v, want Indeterminate", err)
	}
	if execution.startCalls.Load() != 1 || execution.inspectCalls.Load() != 1 {
		t.Fatalf("start=%d inspect=%d, want 1/1", execution.startCalls.Load(), execution.inspectCalls.Load())
	}
}

func TestSQLiteToolOwnerClaimAndMarkerRestartExact(t *testing.T) {
	fixture := newAdapterV2Fixture(t)
	input := ToolOwnerSingleCallExecutionV2{Request: fixture.binding.request.ApplicationRequest, Binding: fixture.projection}
	path := filepath.Join(t.TempDir(), "tool-owner.db")
	config := toolsqlite.ConfigV1{Path: path, Owner: core.OwnerRef{Domain: "tool-mcp", ID: "owner-v2"}, Clock: func() time.Time { return fixture.binding.now }}
	raw, err := toolsqlite.OpenOwnerClaimExecutionStoreV2(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	claims, _ := NewSQLiteToolOwnerSingleCallClaimStoreV2(raw)
	states, _ := NewSQLiteToolOwnerSingleCallExecutionStateStoreV2(raw)
	claim, err := newToolOwnerSingleCallClaimV2(input, fixture.binding.now.UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	record, created, err := claims.CreateToolOwnerSingleCallClaimV2(context.Background(), ToolOwnerSingleCallClaimRecordV2{Claim: claim, Input: input})
	if err != nil || !created {
		t.Fatalf("claim created=%v err=%v", created, err)
	}
	state, err := NewToolOwnerSingleCallExecutionStartV2(record, fixture.binding.now.UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	state, created, err = states.CreateExecutionStartV2(context.Background(), state)
	if err != nil || !created {
		t.Fatalf("state created=%v err=%v", created, err)
	}
	if err = raw.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err = toolsqlite.OpenOwnerClaimExecutionStoreV2(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	claims, _ = NewSQLiteToolOwnerSingleCallClaimStoreV2(raw)
	states, _ = NewSQLiteToolOwnerSingleCallExecutionStateStoreV2(raw)
	key := mustToolOwnerInspectKeyV2(input)
	recoveredClaim, err := claims.InspectToolOwnerSingleCallClaimV2(context.Background(), key)
	if err != nil || recoveredClaim.Claim.Digest != record.Claim.Digest {
		t.Fatalf("recovered claim=%#v err=%v", recoveredClaim, err)
	}
	recoveredState, err := states.InspectExecutionStateV2(context.Background(), key)
	if err != nil || recoveredState.RefV2() != state.RefV2() {
		t.Fatalf("recovered state=%#v err=%v", recoveredState, err)
	}
}

func TestSQLiteToolOwnerClaimAndMarker64SingleWinnerInspectOnlyRestartAndABA(t *testing.T) {
	fixture := newAdapterV2Fixture(t)
	input := ToolOwnerSingleCallExecutionV2{Request: fixture.binding.request.ApplicationRequest, Binding: fixture.projection}
	path := filepath.Join(t.TempDir(), "owner-race.db")
	config := toolsqlite.ConfigV1{Path: path, Owner: core.OwnerRef{Domain: "tool-mcp", ID: "owner-v2"}, Clock: func() time.Time { return fixture.binding.now }}
	raw, err := toolsqlite.OpenOwnerClaimExecutionStoreV2(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	claims, _ := NewSQLiteToolOwnerSingleCallClaimStoreV2(raw)
	states, _ := NewSQLiteToolOwnerSingleCallExecutionStateStoreV2(raw)
	claim, err := newToolOwnerSingleCallClaimV2(input, fixture.binding.now.UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	requested := ToolOwnerSingleCallClaimRecordV2{Claim: claim, Input: input}
	var claimWinners atomic.Int32
	var stateWinners atomic.Int32
	var wg sync.WaitGroup
	errs := make(chan error, 128)
	for range 64 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			winner, created, createErr := claims.CreateToolOwnerSingleCallClaimV2(context.Background(), requested)
			if createErr != nil {
				errs <- createErr
				return
			}
			if created {
				claimWinners.Add(1)
			}
			start, createErr := NewToolOwnerSingleCallExecutionStartV2(winner, fixture.binding.now.UnixNano())
			if createErr != nil {
				errs <- createErr
				return
			}
			_, created, createErr = states.CreateExecutionStartV2(context.Background(), start)
			if createErr != nil {
				errs <- createErr
				return
			}
			if created {
				stateWinners.Add(1)
			}
		}()
	}
	wg.Wait()
	close(errs)
	for runErr := range errs {
		t.Fatal(runErr)
	}
	if claimWinners.Load() != 1 || stateWinners.Load() != 1 {
		t.Fatalf("claim winners=%d marker winners=%d", claimWinners.Load(), stateWinners.Load())
	}
	key := mustToolOwnerInspectKeyV2(input)
	start, err := states.InspectExecutionStateV2(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	unknown := ToolOwnerSingleCallExecutionUnknownV2{Class: ToolOwnerEntryOutcomeUnknownV2, ErrorDigest: core.DigestBytes([]byte("unknown")), MarkedUnixNano: start.UpdatedUnixNano + 1}
	inspectOnly, err := states.AdvanceExecutionInspectOnlyV2(context.Background(), start.RefV2(), unknown)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = states.AdvanceExecutionSettledV2(context.Background(), start.RefV2(), toolcontract.ObjectRef{ID: "result-v2", Revision: 1, Digest: core.DigestBytes([]byte("result"))}, inspectOnly.UpdatedUnixNano+1); err == nil || !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("stale start ABA error=%v, want Conflict", err)
	}
	if err = raw.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err = toolsqlite.OpenOwnerClaimExecutionStoreV2(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	states, _ = NewSQLiteToolOwnerSingleCallExecutionStateStoreV2(raw)
	restarted, err := states.InspectExecutionStateV2(context.Background(), key)
	if err != nil || restarted.RefV2() != inspectOnly.RefV2() || restarted.State != ToolOwnerExecutionInspectOnlyV2 {
		t.Fatalf("inspect-only restart=%#v err=%v", restarted, err)
	}
}

func TestSQLiteToolOwnerStrictJSONAndHistoryCorruptionFailClosed(t *testing.T) {
	fixture := newAdapterV2Fixture(t)
	input := ToolOwnerSingleCallExecutionV2{Request: fixture.binding.request.ApplicationRequest, Binding: fixture.projection}
	path := filepath.Join(t.TempDir(), "corrupt.db")
	config := toolsqlite.ConfigV1{Path: path, Owner: core.OwnerRef{Domain: "tool-mcp", ID: "owner-v2"}, Clock: func() time.Time { return fixture.binding.now }}
	raw, err := toolsqlite.OpenOwnerClaimExecutionStoreV2(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	claims, _ := NewSQLiteToolOwnerSingleCallClaimStoreV2(raw)
	states, _ := NewSQLiteToolOwnerSingleCallExecutionStateStoreV2(raw)
	claim, err := newToolOwnerSingleCallClaimV2(input, fixture.binding.now.UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	record, _, err := claims.CreateToolOwnerSingleCallClaimV2(context.Background(), ToolOwnerSingleCallClaimRecordV2{Claim: claim, Input: input})
	if err != nil {
		t.Fatal(err)
	}
	state, err := NewToolOwnerSingleCallExecutionStartV2(record, fixture.binding.now.UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	state, _, err = states.CreateExecutionStartV2(context.Background(), state)
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	key := mustToolOwnerInspectKeyV2(input)
	if _, err = db.Exec(`UPDATE tool_owner_single_call_claim_v2 SET input_json=? WHERE claim_id=?`, []byte(`{}`), claim.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = claims.InspectToolOwnerSingleCallClaimV2(context.Background(), key); err == nil || !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("empty claim input error=%v, want Conflict", err)
	}
	if _, _, err = claims.CreateToolOwnerSingleCallClaimV2(context.Background(), record); err == nil || !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("tampered create-once loser error=%v, want Conflict", err)
	}
	if _, err = db.Exec(`UPDATE tool_owner_execution_history_v2 SET state_json=? WHERE state_id=? AND state_revision=?`, []byte(`{"unknown_field":true}`), state.ID, state.Revision); err != nil {
		t.Fatal(err)
	}
	if _, err = states.InspectExecutionStateV2(context.Background(), key); err == nil || !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("history splice error=%v, want Conflict", err)
	}
	validState, _ := json.Marshal(state)
	if _, err = db.Exec(`UPDATE tool_owner_execution_history_v2 SET state_json=? WHERE state_id=? AND state_revision=?`, append(validState, []byte(`{}`)...), state.ID, state.Revision); err != nil {
		t.Fatal(err)
	}
	if _, err = states.InspectExecutionStateV2(context.Background(), key); err == nil || !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("trailing state JSON error=%v, want Conflict", err)
	}
}
