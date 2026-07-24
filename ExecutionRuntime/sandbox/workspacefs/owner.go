package workspacefs

import (
	"context"
	"errors"
	"reflect"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

// OwnerV1 composes real capture with the durable Sandbox Owner current store.
// It does not govern or execute the later workspace-commit Effect.
type OwnerV1 struct {
	capture *DriverV1
	store   ports.WorkspaceOwnerStoreV1
}

func NewOwnerV1(capture *DriverV1, store ports.WorkspaceOwnerStoreV1) (*OwnerV1, error) {
	if capture == nil || nilLikeOwnerV1(store) {
		return nil, errors.New("workspace Owner requires capture driver and current store")
	}
	return &OwnerV1{capture: capture, store: store}, nil
}

func (o *OwnerV1) CaptureWorkspaceChangeSetV1(ctx context.Context, request ports.CaptureWorkspaceChangeSetRequest) (contract.WorkspaceChangeSet, error) {
	view, err := o.store.CreateWorkspaceViewV1(ctx, request.View)
	if err != nil {
		return contract.WorkspaceChangeSet{}, err
	}
	request.View = view
	set, err := o.capture.CaptureWorkspaceChangeSetV1(ctx, request)
	if err != nil {
		return contract.WorkspaceChangeSet{}, err
	}
	stored, err := o.store.CreateWorkspaceChangeSetV1(ctx, set)
	if err == nil {
		return stored, nil
	}
	// A dropped create reply is recovered only through the exact Owner reader.
	recovered, inspectErr := o.store.InspectWorkspaceChangeSetCurrentV1(context.WithoutCancel(ctx), set.Meta.Ref())
	if inspectErr != nil || !contract.SameRef(recovered.Meta.Ref(), set.Meta.Ref()) {
		return contract.WorkspaceChangeSet{}, err
	}
	return recovered, nil
}

var _ ports.WorkspaceChangeSetCapturePortV1 = (*OwnerV1)(nil)

func nilLikeOwnerV1(value any) bool {
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
