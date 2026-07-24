// Package fakes contains test-only fault controls for Continuity public ports.
// It is not a production backend and provides no persistence or SLA.
package fakes

import (
	"context"
	"reflect"
	"sync"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
)

type CheckpointMutationV2 string

const (
	CheckpointCreateManifestV2 CheckpointMutationV2 = "create_manifest"
	CheckpointCASManifestV2    CheckpointMutationV2 = "cas_manifest"
	CheckpointCreateSealV2     CheckpointMutationV2 = "create_seal"
)

// CheckpointManifestGovernanceV2 delegates every operation and can hide the
// next successful mutation reply. The durable write still occurs, forcing the
// caller to recover by exact Inspect of the original identity.
type CheckpointManifestGovernanceV2 struct {
	Delegate ports.CheckpointManifestGovernancePortV2

	mu        sync.Mutex
	lostReply map[CheckpointMutationV2]error
}

func NewCheckpointManifestGovernanceV2(delegate ports.CheckpointManifestGovernancePortV2) (*CheckpointManifestGovernanceV2, error) {
	if nilGovernancePortV2(delegate) {
		return nil, contract.NewError(contract.ErrInvalidArgument, "checkpoint_manifest_governance_port", "delegate is required")
	}
	return &CheckpointManifestGovernanceV2{
		Delegate:  delegate,
		lostReply: make(map[CheckpointMutationV2]error),
	}, nil
}

func nilGovernancePortV2(value ports.CheckpointManifestGovernancePortV2) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	return reflected.Kind() == reflect.Pointer && reflected.IsNil()
}

func (f *CheckpointManifestGovernanceV2) LoseNextSuccessfulReply(operation CheckpointMutationV2, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lostReply[operation] = err
}

func (f *CheckpointManifestGovernanceV2) takeLostReply(operation CheckpointMutationV2) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	err := f.lostReply[operation]
	delete(f.lostReply, operation)
	return err
}

func (f *CheckpointManifestGovernanceV2) CreateCheckpointManifestV2(
	ctx context.Context,
	request ports.CreateCheckpointManifestRequestV2,
) (contract.CheckpointManifestFactV2, bool, error) {
	value, replay, err := f.Delegate.CreateCheckpointManifestV2(ctx, request)
	if err == nil {
		if lost := f.takeLostReply(CheckpointCreateManifestV2); lost != nil {
			return contract.CheckpointManifestFactV2{}, false, lost
		}
	}
	return value, replay, err
}

func (f *CheckpointManifestGovernanceV2) CompareAndSwapCheckpointManifestV2(
	ctx context.Context,
	request ports.CompareAndSwapCheckpointManifestRequestV2,
) (contract.CheckpointManifestFactV2, bool, error) {
	value, replay, err := f.Delegate.CompareAndSwapCheckpointManifestV2(ctx, request)
	if err == nil {
		if lost := f.takeLostReply(CheckpointCASManifestV2); lost != nil {
			return contract.CheckpointManifestFactV2{}, false, lost
		}
	}
	return value, replay, err
}

func (f *CheckpointManifestGovernanceV2) CreateCheckpointManifestSealV2(
	ctx context.Context,
	request ports.CreateCheckpointManifestSealRequestV2,
) (contract.CheckpointManifestSealFactV2, bool, error) {
	value, replay, err := f.Delegate.CreateCheckpointManifestSealV2(ctx, request)
	if err == nil {
		if lost := f.takeLostReply(CheckpointCreateSealV2); lost != nil {
			return contract.CheckpointManifestSealFactV2{}, false, lost
		}
	}
	return value, replay, err
}

func (f *CheckpointManifestGovernanceV2) InspectCheckpointManifestV2(
	ctx context.Context,
	request ports.InspectCheckpointManifestRequestV2,
) (contract.CheckpointManifestFactV2, error) {
	return f.Delegate.InspectCheckpointManifestV2(ctx, request)
}

func (f *CheckpointManifestGovernanceV2) InspectCurrentCheckpointManifestV2(
	ctx context.Context,
	request ports.InspectCurrentCheckpointManifestRequestV2,
) (contract.CheckpointManifestFactV2, error) {
	return f.Delegate.InspectCurrentCheckpointManifestV2(ctx, request)
}

func (f *CheckpointManifestGovernanceV2) InspectCheckpointManifestSealV2(
	ctx context.Context,
	request ports.InspectCheckpointManifestSealRequestV2,
) (contract.CheckpointManifestSealFactV2, error) {
	return f.Delegate.InspectCheckpointManifestSealV2(ctx, request)
}

var _ ports.CheckpointManifestGovernancePortV2 = (*CheckpointManifestGovernanceV2)(nil)
