package fault_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/runtimeadapter"
)

type faultClock struct{ now time.Time }

func (c faultClock) Now() time.Time { return c.now }

type unavailableBoundarySource struct{}

func (unavailableBoundarySource) InspectBoundarySourceCurrentV1(context.Context, contract.ToolProviderBoundarySourceRefV1, time.Time) (contract.SingleCallToolActionCoordinationWatermarkV1, error) {
	return contract.SingleCallToolActionCoordinationWatermarkV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEvidenceUnavailable, "lost boundary read reply")
}

func TestBoundaryUnknownIsInspectOnlyAndReturnsZeroProjection(t *testing.T) {
	adapter := runtimeadapter.NewProviderBoundaryCurrentAdapterV1(unavailableBoundarySource{}, faultClock{time.Unix(1_800_000_000, 0)})
	ref := runtimeports.OperationProviderBoundaryRefV1{ID: "tool-watermark-fault", Revision: 5, Digest: core.DigestBytes([]byte("boundary"))}
	projection, err := adapter.InspectCurrentOperationProviderBoundaryV1(context.Background(), ref)
	if err == nil || !reflect.DeepEqual(projection, runtimeports.OperationProviderBoundaryCurrentProjectionV1{}) {
		t.Fatalf("unknown boundary=%#v err=%v", projection, err)
	}
	if !core.HasCategory(err, core.ErrorIndeterminate) {
		t.Fatalf("error classification=%T %v", err, err)
	}
}
