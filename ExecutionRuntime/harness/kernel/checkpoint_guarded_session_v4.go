package kernel

import (
	"context"
	"errors"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/contract"
	harnessports "github.com/Proview-China/rax/ExecutionRuntime/harness/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

// CheckpointGuardedSessionFactPortV4 is the public Harness checkpoint seam.
// Production composition must place it at the actual Session write point; the
// Gate does not become effective merely because a capability was declared.
type CheckpointGuardedSessionFactPortV4 struct {
	sessions harnessports.SessionFactPortV4
	gates    harnessports.CheckpointGateStoreV1
	clock    func() time.Time
}

func NewCheckpointGuardedSessionFactPortV4(sessions harnessports.SessionFactPortV4, gates harnessports.CheckpointGateStoreV1, clock func() time.Time) (*CheckpointGuardedSessionFactPortV4, error) {
	if checkpointNilV1(sessions) || checkpointNilV1(gates) || clock == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "checkpoint guarded Session dependencies are required")
	}
	return &CheckpointGuardedSessionFactPortV4{sessions: sessions, gates: gates, clock: clock}, nil
}

func (p *CheckpointGuardedSessionFactPortV4) CreateSessionV4(ctx context.Context, session contract.GovernedSessionV4) (contract.GovernedSessionV4, error) {
	if err := p.rejectCurrentGateV1(ctx, session.Run); err != nil {
		return contract.GovernedSessionV4{}, err
	}
	return p.sessions.CreateSessionV4(ctx, session)
}

func (p *CheckpointGuardedSessionFactPortV4) InspectSessionV4(ctx context.Context, run contract.RunRef, id string) (contract.GovernedSessionV4, error) {
	return p.sessions.InspectSessionV4(ctx, run, id)
}

func (p *CheckpointGuardedSessionFactPortV4) CompareAndSwapSessionV4(ctx context.Context, request contract.SessionCASRequestV4) (contract.GovernedSessionV4, error) {
	if err := request.Validate(); err != nil {
		return contract.GovernedSessionV4{}, err
	}
	if err := p.rejectCurrentGateV1(ctx, request.Run); err != nil {
		return contract.GovernedSessionV4{}, err
	}
	return p.sessions.CompareAndSwapSessionV4(ctx, request)
}

func (p *CheckpointGuardedSessionFactPortV4) rejectCurrentGateV1(ctx context.Context, run contract.RunRef) error {
	gate, err := p.gates.InspectCheckpointGateCurrentV1(ctx, run)
	if err != nil {
		if core.HasCategory(err, core.ErrorNotFound) {
			return nil
		}
		return err
	}
	if gate.ValidateCurrent(p.clock()) == nil {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonCheckpointInconsistent, "Harness Session write is fenced by a current checkpoint gate")
	}
	if gate.State == contract.CheckpointGateReleasedV1 || gate.State == contract.CheckpointGateInvalidatedV1 || p.clock().UnixNano() >= gate.ExpiresUnixNano {
		return nil
	}
	return errors.New("invalid Harness checkpoint gate state")
}

var _ harnessports.SessionFactPortV4 = (*CheckpointGuardedSessionFactPortV4)(nil)
