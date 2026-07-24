package fakes

import (
	"context"
	"reflect"
	"sync"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
)

// ContentDeltaGovernanceV1 is a test-only lost-reply seam. It cannot create
// objects, execute patches or compaction, or delete content.
type ContentDeltaGovernanceV1 struct {
	Delegate ports.ContentDeltaGovernancePortV1

	mu            sync.Mutex
	nextCreateErr error
}

func NewContentDeltaGovernanceV1(delegate ports.ContentDeltaGovernancePortV1) (*ContentDeltaGovernanceV1, error) {
	if nilContentDeltaPortV1(delegate) {
		return nil, contract.NewError(contract.ErrInvalidArgument, "content_delta_governance_port", "delegate is required")
	}
	return &ContentDeltaGovernanceV1{Delegate: delegate}, nil
}

func (f *ContentDeltaGovernanceV1) LoseNextSuccessfulCreateReply(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextCreateErr = err
}

func (f *ContentDeltaGovernanceV1) CreateContentDeltaV1(ctx context.Context, request ports.CreateContentDeltaRequestV1) (contract.ContentDeltaFactV1, bool, error) {
	value, replay, err := f.Delegate.CreateContentDeltaV1(ctx, request)
	if err != nil {
		return value, replay, err
	}
	f.mu.Lock()
	lost := f.nextCreateErr
	f.nextCreateErr = nil
	f.mu.Unlock()
	if lost != nil {
		return contract.ContentDeltaFactV1{}, false, lost
	}
	return value, replay, nil
}

func (f *ContentDeltaGovernanceV1) InspectContentDeltaV1(ctx context.Context, request ports.InspectContentDeltaRequestV1) (contract.ContentDeltaFactV1, error) {
	return f.Delegate.InspectContentDeltaV1(ctx, request)
}

func (f *ContentDeltaGovernanceV1) InspectContentDeltaByIDV1(ctx context.Context, request ports.InspectContentDeltaByIDRequestV1) (contract.ContentDeltaFactV1, error) {
	return f.Delegate.InspectContentDeltaByIDV1(ctx, request)
}

func nilContentDeltaPortV1(value ports.ContentDeltaGovernancePortV1) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	return reflected.Kind() == reflect.Pointer && reflected.IsNil()
}

var _ ports.ContentDeltaGovernancePortV1 = (*ContentDeltaGovernanceV1)(nil)
