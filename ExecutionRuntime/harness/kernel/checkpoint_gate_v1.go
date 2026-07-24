package kernel

import (
	"context"
	"reflect"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/contract"
	harnessports "github.com/Proview-China/rax/ExecutionRuntime/harness/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type CheckpointGateControllerV1 struct {
	sessions  harnessports.SessionCurrentReaderV4
	store     harnessports.CheckpointGateStoreV1
	terminals harnessports.CheckpointTerminalCurrentReaderV1
	clock     func() time.Time
}

func NewCheckpointGateControllerV1(sessions harnessports.SessionCurrentReaderV4, store harnessports.CheckpointGateStoreV1, terminals harnessports.CheckpointTerminalCurrentReaderV1, clock func() time.Time) (*CheckpointGateControllerV1, error) {
	if checkpointNilV1(sessions) || checkpointNilV1(store) || checkpointNilV1(terminals) || clock == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Harness checkpoint gate dependencies are required")
	}
	return &CheckpointGateControllerV1{sessions: sessions, store: store, terminals: terminals, clock: clock}, nil
}

func (c *CheckpointGateControllerV1) AcquireCheckpointGateV1(ctx context.Context, request contract.AcquireCheckpointGateRequestV1) (contract.CheckpointGateFactV1, contract.HarnessCheckpointSnapshotFactV1, error) {
	if err := request.Validate(); err != nil {
		return contract.CheckpointGateFactV1{}, contract.HarnessCheckpointSnapshotFactV1{}, err
	}
	now, err := c.nowV1(time.Time{})
	if err != nil {
		return contract.CheckpointGateFactV1{}, contract.HarnessCheckpointSnapshotFactV1{}, err
	}
	if now.UnixNano() >= request.RequestedNotAfter {
		return contract.CheckpointGateFactV1{}, contract.HarnessCheckpointSnapshotFactV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "Harness checkpoint gate TTL is not current")
	}
	s1, err := c.inspectExpectedSessionV1(ctx, request)
	if err != nil {
		return contract.CheckpointGateFactV1{}, contract.HarnessCheckpointSnapshotFactV1{}, err
	}
	snapshot, err := contract.SealHarnessCheckpointSnapshotFactV1(contract.HarnessCheckpointSnapshotFactV1{
		Ref: contract.HarnessCheckpointSnapshotRefV1{ID: request.StableID + ":snapshot"}, IntentDigest: request.IntentDigest,
		Run: request.Run, Session: s1, CapturedUnixNano: now.UnixNano(),
	})
	if err != nil {
		return contract.CheckpointGateFactV1{}, contract.HarnessCheckpointSnapshotFactV1{}, err
	}
	gate, err := contract.SealCheckpointGateFactV1(contract.CheckpointGateFactV1{
		Ref: contract.CheckpointGateRefV1{ID: request.StableID, Revision: 1}, State: contract.CheckpointGateAcquiredV1,
		Request: request, Snapshot: snapshot.Ref, AcquiredUnixNano: now.UnixNano(), ExpiresUnixNano: request.RequestedNotAfter,
	})
	if err != nil {
		return contract.CheckpointGateFactV1{}, contract.HarnessCheckpointSnapshotFactV1{}, err
	}
	storedGate, storedSnapshot, err := c.store.CreateCheckpointGateAndSnapshotV1(ctx, gate, snapshot)
	if err != nil {
		storedGate, err = c.store.InspectCheckpointGateV1(context.WithoutCancel(ctx), gate.Ref)
		if err != nil {
			return contract.CheckpointGateFactV1{}, contract.HarnessCheckpointSnapshotFactV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonCheckpointInconsistent, "Harness checkpoint gate create outcome cannot be inspected")
		}
		storedSnapshot, err = c.store.InspectHarnessCheckpointSnapshotV1(context.WithoutCancel(ctx), gate.Snapshot)
		if err != nil {
			return contract.CheckpointGateFactV1{}, contract.HarnessCheckpointSnapshotFactV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonCheckpointInconsistent, "Harness checkpoint snapshot create outcome cannot be inspected")
		}
	}
	if !reflect.DeepEqual(storedGate, gate) || !reflect.DeepEqual(storedSnapshot, snapshot) {
		return contract.CheckpointGateFactV1{}, contract.HarnessCheckpointSnapshotFactV1{}, core.NewError(core.ErrorConflict, core.ReasonCheckpointInconsistent, "Harness checkpoint gate replay changed immutable content")
	}
	if _, err = c.nowV1(now); err != nil {
		return contract.CheckpointGateFactV1{}, contract.HarnessCheckpointSnapshotFactV1{}, err
	}
	s2, err := c.inspectExpectedSessionV1(ctx, request)
	if err != nil || !reflect.DeepEqual(s1, s2) {
		c.invalidateAcquireV1(context.WithoutCancel(ctx), gate)
		if err != nil {
			return contract.CheckpointGateFactV1{}, contract.HarnessCheckpointSnapshotFactV1{}, err
		}
		return contract.CheckpointGateFactV1{}, contract.HarnessCheckpointSnapshotFactV1{}, core.NewError(core.ErrorConflict, core.ReasonCheckpointInconsistent, "Harness Session changed between checkpoint gate S1 and S2")
	}
	// A retry may arrive after the exact Gate has already been bound or
	// released. Return only a proven monotonic successor; never resurrect the
	// historical acquired revision as current.
	if current, currentErr := c.store.InspectCheckpointGateCurrentV1(context.WithoutCancel(ctx), request.Run); currentErr == nil && current.Ref != storedGate.Ref {
		if current.Ref.ID != storedGate.Ref.ID || current.Ref.Revision < storedGate.Ref.Revision || !sameCheckpointGateImmutableV1(storedGate, current) || current.Snapshot != storedSnapshot.Ref {
			return contract.CheckpointGateFactV1{}, contract.HarnessCheckpointSnapshotFactV1{}, core.NewError(core.ErrorConflict, core.ReasonCheckpointInconsistent, "Harness checkpoint gate retry found an unrelated or ABA current successor")
		}
		return current.Clone(), storedSnapshot.Clone(), nil
	}
	return storedGate.Clone(), storedSnapshot.Clone(), nil
}

func sameCheckpointGateImmutableV1(left, right contract.CheckpointGateFactV1) bool {
	left.Ref, right.Ref = contract.CheckpointGateRefV1{}, contract.CheckpointGateRefV1{}
	left.State, right.State = "", ""
	left.Runtime, right.Runtime = nil, nil
	left.BoundUnixNano, right.BoundUnixNano = 0, 0
	left.ReleasedUnixNano, right.ReleasedUnixNano = 0, 0
	if left.ExpiresUnixNano > right.ExpiresUnixNano {
		left.ExpiresUnixNano = right.ExpiresUnixNano
	}
	return reflect.DeepEqual(left, right)
}

func (c *CheckpointGateControllerV1) BindCheckpointGateRuntimeV1(ctx context.Context, request contract.BindCheckpointGateRuntimeRequestV1) (contract.CheckpointGateFactV1, error) {
	if err := request.Validate(); err != nil {
		return contract.CheckpointGateFactV1{}, err
	}
	current, err := c.store.InspectCheckpointGateV1(ctx, request.Expected)
	if err != nil {
		return contract.CheckpointGateFactV1{}, err
	}
	if current.State != contract.CheckpointGateAcquiredV1 || request.Runtime.Validate(current.Request.Run) != nil {
		return contract.CheckpointGateFactV1{}, core.NewError(core.ErrorConflict, core.ReasonCheckpointInconsistent, "Harness checkpoint gate cannot bind these Runtime refs")
	}
	now, err := c.nowV1(time.Time{})
	if err != nil || now.UnixNano() >= current.ExpiresUnixNano || now.UnixNano() >= request.Runtime.Barrier.ExpiresUnixNano {
		if err != nil {
			return contract.CheckpointGateFactV1{}, err
		}
		return contract.CheckpointGateFactV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "Harness checkpoint gate or Runtime Barrier expired before bind")
	}
	if _, err := c.inspectExpectedSessionV1(ctx, current.Request); err != nil {
		return contract.CheckpointGateFactV1{}, err
	}
	next := current.Clone()
	next.Ref.Revision = 2
	next.State = contract.CheckpointGateBoundV1
	next.Runtime = &request.Runtime
	next.BoundUnixNano = now.UnixNano()
	next.ExpiresUnixNano = min(next.ExpiresUnixNano, request.Runtime.Barrier.ExpiresUnixNano)
	next, err = contract.SealCheckpointGateFactV1(next)
	if err != nil {
		return contract.CheckpointGateFactV1{}, err
	}
	stored, err := c.store.BindCheckpointGateRuntimeV1(ctx, current.Ref, next)
	if err != nil {
		stored, err = c.store.InspectCheckpointGateV1(context.WithoutCancel(ctx), next.Ref)
		if err != nil {
			return contract.CheckpointGateFactV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonCheckpointInconsistent, "Harness checkpoint Runtime bind outcome cannot be inspected")
		}
	}
	if !reflect.DeepEqual(stored, next) {
		return contract.CheckpointGateFactV1{}, core.NewError(core.ErrorConflict, core.ReasonCheckpointInconsistent, "Harness checkpoint Runtime bind recovery changed content")
	}
	return stored.Clone(), nil
}

func (c *CheckpointGateControllerV1) InspectCheckpointGateCurrentV1(ctx context.Context, run contract.RunRef) (contract.CheckpointGateFactV1, error) {
	if err := run.Validate(); err != nil {
		return contract.CheckpointGateFactV1{}, err
	}
	fact, err := c.store.InspectCheckpointGateCurrentV1(ctx, run)
	if err != nil {
		return contract.CheckpointGateFactV1{}, err
	}
	if err := fact.ValidateCurrent(c.clock()); err != nil {
		return contract.CheckpointGateFactV1{}, err
	}
	return fact.Clone(), nil
}

func (c *CheckpointGateControllerV1) InvalidateCheckpointGateV1(ctx context.Context, request contract.InvalidateCheckpointGateRequestV1) (contract.CheckpointGateFactV1, error) {
	if err := request.Validate(); err != nil {
		return contract.CheckpointGateFactV1{}, err
	}
	current, err := c.store.InspectCheckpointGateV1(ctx, request.Expected)
	if err != nil {
		return contract.CheckpointGateFactV1{}, err
	}
	if current.State != contract.CheckpointGateAcquiredV1 {
		return contract.CheckpointGateFactV1{}, core.NewError(core.ErrorConflict, core.ReasonCheckpointInconsistent, "only an unbound Harness checkpoint gate may be invalidated")
	}
	next, err := transitionCheckpointGateV1(current, contract.CheckpointGateInvalidatedV1, c.clock())
	if err != nil {
		return contract.CheckpointGateFactV1{}, err
	}
	stored, err := c.store.InvalidateCheckpointGateV1(ctx, current.Ref, next)
	if err != nil {
		stored, err = c.store.InspectCheckpointGateV1(context.WithoutCancel(ctx), next.Ref)
		if err != nil {
			return contract.CheckpointGateFactV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonCheckpointInconsistent, "Harness checkpoint gate invalidation outcome cannot be inspected")
		}
	}
	if !reflect.DeepEqual(stored, next) {
		return contract.CheckpointGateFactV1{}, core.NewError(core.ErrorConflict, core.ReasonCheckpointInconsistent, "Harness checkpoint gate invalidation recovery changed content")
	}
	return stored.Clone(), nil
}

func (c *CheckpointGateControllerV1) InspectCheckpointGateV1(ctx context.Context, ref contract.CheckpointGateRefV1) (contract.CheckpointGateFactV1, error) {
	if err := ref.Validate(); err != nil {
		return contract.CheckpointGateFactV1{}, err
	}
	fact, err := c.store.InspectCheckpointGateV1(ctx, ref)
	if err != nil {
		return contract.CheckpointGateFactV1{}, err
	}
	return fact.Clone(), fact.Validate()
}

func (c *CheckpointGateControllerV1) InspectHarnessCheckpointSnapshotV1(ctx context.Context, ref contract.HarnessCheckpointSnapshotRefV1) (contract.HarnessCheckpointSnapshotFactV1, error) {
	if err := ref.Validate(); err != nil {
		return contract.HarnessCheckpointSnapshotFactV1{}, err
	}
	fact, err := c.store.InspectHarnessCheckpointSnapshotV1(ctx, ref)
	if err != nil {
		return contract.HarnessCheckpointSnapshotFactV1{}, err
	}
	return fact.Clone(), fact.Validate()
}

func (c *CheckpointGateControllerV1) ReleaseCheckpointGateV1(ctx context.Context, request contract.ReleaseCheckpointGateRequestV1) (contract.CheckpointGateFactV1, error) {
	if err := request.Validate(); err != nil {
		return contract.CheckpointGateFactV1{}, err
	}
	current, err := c.store.InspectCheckpointGateV1(ctx, request.Expected)
	if err != nil {
		return contract.CheckpointGateFactV1{}, err
	}
	if current.State != contract.CheckpointGateBoundV1 || current.Runtime == nil || current.Runtime.Attempt.TenantID != request.TerminalAttempt.TenantID || current.Runtime.Attempt.ID != request.TerminalAttempt.ID || request.TerminalAttempt.Revision < current.Runtime.Attempt.Revision {
		return contract.CheckpointGateFactV1{}, core.NewError(core.ErrorConflict, core.ReasonCheckpointInconsistent, "checkpoint gate release binds another or unbound Attempt")
	}
	terminal, err := c.terminals.InspectCheckpointAttemptTerminalCurrentV2(ctx, request.TerminalAttempt)
	if err != nil {
		return contract.CheckpointGateFactV1{}, err
	}
	if terminal.Attempt != request.TerminalAttempt || terminal.Attempt.ID != current.Runtime.Attempt.ID || terminal.Attempt.TenantID != current.Runtime.Attempt.TenantID || terminal.Barrier.ID != current.Runtime.Barrier.ID || terminal.Barrier.AttemptID != current.Runtime.Barrier.AttemptID {
		return contract.CheckpointGateFactV1{}, core.NewError(core.ErrorConflict, core.ReasonCheckpointInconsistent, "Runtime terminal projection does not close this Harness gate")
	}
	now, err := c.nowV1(time.Time{})
	if err != nil {
		return contract.CheckpointGateFactV1{}, err
	}
	next, err := transitionCheckpointGateV1(current, contract.CheckpointGateReleasedV1, now)
	if err != nil {
		return contract.CheckpointGateFactV1{}, err
	}
	stored, err := c.store.ReleaseCheckpointGateV1(ctx, current.Ref, next)
	if err != nil {
		stored, err = c.store.InspectCheckpointGateV1(context.WithoutCancel(ctx), next.Ref)
		if err != nil {
			return contract.CheckpointGateFactV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonCheckpointInconsistent, "Harness checkpoint gate release outcome cannot be inspected")
		}
	}
	if !reflect.DeepEqual(stored, next) {
		return contract.CheckpointGateFactV1{}, core.NewError(core.ErrorConflict, core.ReasonCheckpointInconsistent, "Harness checkpoint gate release recovery changed content")
	}
	return stored.Clone(), nil
}

func (c *CheckpointGateControllerV1) inspectExpectedSessionV1(ctx context.Context, request contract.AcquireCheckpointGateRequestV1) (contract.GovernedSessionV4, error) {
	session, err := c.sessions.InspectSessionV4(ctx, request.Run, request.SessionID)
	if err != nil {
		return contract.GovernedSessionV4{}, err
	}
	if session.Validate() != nil || session.ID != request.SessionID || session.Revision != request.ExpectedSessionRevision || session.Digest != request.ExpectedSessionDigest || session.Run.RunID != request.Run.RunID || !contract.CheckpointSafeSessionPhaseV1(session.Phase) {
		return contract.GovernedSessionV4{}, core.NewError(core.ErrorConflict, core.ReasonCheckpointInconsistent, "Harness checkpoint request does not match a safe exact Session")
	}
	return session.Clone(), nil
}

func (c *CheckpointGateControllerV1) invalidateAcquireV1(ctx context.Context, current contract.CheckpointGateFactV1) {
	now := c.clock()
	next, err := transitionCheckpointGateV1(current, contract.CheckpointGateInvalidatedV1, now)
	if err == nil {
		_, _ = c.store.InvalidateCheckpointGateV1(ctx, current.Ref, next)
	}
}

func (c *CheckpointGateControllerV1) nowV1(previous time.Time) (time.Time, error) {
	now := c.clock()
	if now.IsZero() || (!previous.IsZero() && now.Before(previous)) {
		return time.Time{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "Harness checkpoint clock is zero or moved backwards")
	}
	return now, nil
}

func transitionCheckpointGateV1(current contract.CheckpointGateFactV1, state contract.CheckpointGateStateV1, now time.Time) (contract.CheckpointGateFactV1, error) {
	next := current.Clone()
	next.Ref.Revision = current.Ref.Revision + 1
	next.State = state
	next.ReleasedUnixNano = now.UnixNano()
	return contract.SealCheckpointGateFactV1(next)
}

func checkpointNilV1(value any) bool {
	if value == nil {
		return true
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return rv.IsNil()
	default:
		return false
	}
}

var _ harnessports.CheckpointGateGovernancePortV1 = (*CheckpointGateControllerV1)(nil)
