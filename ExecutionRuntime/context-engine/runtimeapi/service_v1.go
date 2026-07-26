package runtimeapi

import (
	"context"
	"fmt"
	"reflect"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/kernel"
)

type AppendSettledToolResultRequestV2 = kernel.AppendSettledToolResultRequestV2
type AppendSettledToolResultResultV2 = kernel.AppendSettledToolResultResultV2

type ContextRuntimeAPIV1 interface {
	ConsumeFrame(context.Context, contract.ContextFrameConsumptionRequestV1) (contract.ContextFrameConsumptionDescriptorV1, error)
	AppendSettledToolResult(context.Context, AppendSettledToolResultRequestV2) (AppendSettledToolResultResultV2, error)
	MaterializeDescriptor(context.Context, MaterializeDescriptorRequestV1) (MaterializeDescriptorResultV1, error)
}

type ServiceV1 struct {
	frameReader kernel.FrameConsumptionCurrentReaderV1
	toolReader  kernel.SettledToolResultCurrentReaderV2
	store       kernel.ContextAwareReferenceStoreV1
}

func NewServiceV1(
	frameReader kernel.FrameConsumptionCurrentReaderV1,
	toolReader kernel.SettledToolResultCurrentReaderV2,
	store kernel.ContextAwareReferenceStoreV1,
) (*ServiceV1, error) {
	if nilInterfaceV1(frameReader) || nilInterfaceV1(toolReader) || nilInterfaceV1(store) {
		return nil, fmt.Errorf("%w: context runtime api dependencies", contract.ErrInvalid)
	}
	return &ServiceV1{frameReader: frameReader, toolReader: toolReader, store: store}, nil
}

func (s *ServiceV1) ConsumeFrame(ctx context.Context, request contract.ContextFrameConsumptionRequestV1) (contract.ContextFrameConsumptionDescriptorV1, error) {
	if s == nil {
		return contract.ContextFrameConsumptionDescriptorV1{}, fmt.Errorf("%w: context runtime api service", contract.ErrInvalid)
	}
	return kernel.BuildFrameConsumptionDescriptorV1(ctx, s.frameReader, s.store, request)
}

func (s *ServiceV1) AppendSettledToolResult(ctx context.Context, request AppendSettledToolResultRequestV2) (AppendSettledToolResultResultV2, error) {
	if s == nil {
		return AppendSettledToolResultResultV2{}, fmt.Errorf("%w: context runtime api service", contract.ErrInvalid)
	}
	return kernel.AppendSettledToolResultV2(ctx, s.toolReader, s.store, request)
}

func nilInterfaceV1(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}

var _ ContextRuntimeAPIV1 = (*ServiceV1)(nil)
