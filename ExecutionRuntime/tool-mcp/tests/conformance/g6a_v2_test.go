package conformance_test

import (
	"context"
	"reflect"
	"testing"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/action"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/runtimeadapter"
)

func TestG6APublicTypeAndReaderConformance(t *testing.T) {
	var _ modelinvoker.ToolCallCandidateObservationProjectionReaderV1 = (*modelProjectionReaderShape)(nil)
	var _ runtimeports.OperationProviderBoundaryCurrentReaderV1 = (*runtimeadapter.ProviderBoundaryCurrentAdapterV1)(nil)
	var _ runtimeadapter.ToolBoundarySourceCurrentReaderV1 = (*action.CoordinationStoreV1)(nil)
	assertFields(t, reflect.TypeOf(runtimeports.OperationProviderBoundaryRefV1{}), []string{"ID", "Revision", "Digest"})
	assertFields(t, reflect.TypeOf(contract.ToolProviderBoundarySourceRefV1{}), []string{"WatermarkID", "WatermarkRevision", "WatermarkDigest"})
	domain := reflect.TypeOf(contract.ToolDomainResultFactV2{})
	for _, name := range []string{"Observation", "PrepareEnforcement", "ExecuteEnforcement", "PrepareConsumption", "ExecuteConsumption"} {
		field, ok := domain.FieldByName(name)
		if !ok || field.Type.Kind() == reflect.String {
			t.Fatalf("ToolDomainResultFactV2.%s is missing or weakly typed", name)
		}
	}
}

type modelProjectionReaderShape struct{}

func (*modelProjectionReaderShape) InspectExactProjectionV1(context.Context, modelinvoker.ToolCallCandidateObservationRefV1) (modelinvoker.ToolCallCandidateObservationProjectionV1, error) {
	panic("shape only")
}

func assertFields(t *testing.T, typ reflect.Type, want []string) {
	t.Helper()
	if typ.NumField() != len(want) {
		t.Fatalf("%s fields=%d want=%d", typ, typ.NumField(), len(want))
	}
	for i, name := range want {
		if typ.Field(i).Name != name {
			t.Fatalf("%s field[%d]=%s want=%s", typ, i, typ.Field(i).Name, name)
		}
	}
}
