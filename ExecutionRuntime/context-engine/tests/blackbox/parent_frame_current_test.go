package blackbox_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testfixture"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testkit"
	contextports "github.com/Proview-China/rax/ExecutionRuntime/context-engine/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/runtimeadapter"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestRuntimeAdapterProjectsExactParentFrameCurrentness(t *testing.T) {
	now := time.Unix(0, testkit.Now)
	fixture, err := testfixture.NewParentFrameFixtureV1(func() time.Time { return now }, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	fact := runtimeParentFrameFactV3(fixture.Source)
	adapter := runtimeadapter.ParentFrameApplicabilityCurrentAdapterV3{Reader: fixture.Reader, Clock: func() time.Time { return now }}
	projection, err := adapter.InspectOperationScopeEvidenceApplicabilityCurrentV3(context.Background(), fact)
	if err != nil {
		t.Fatal(err)
	}
	if projection.Fact != fact || projection.ExecutionScopeDigest != core.Digest(fixture.Frame.Execution.ScopeDigest) || projection.ExpiresUnixNano != fixture.Pointer.ExpiresUnixNano || !projection.Current {
		t.Fatalf("runtime projection drifted: %+v", projection)
	}
	if err := projection.Validate(fact, core.Digest(fixture.Frame.Execution.ScopeDigest), now); err != nil {
		t.Fatal(err)
	}
}

func TestRuntimeAdapterRejectsOtherKindBeforeOwnerRead(t *testing.T) {
	now := time.Unix(0, testkit.Now)
	reader := &countingParentFrameReaderV1{}
	adapter := runtimeadapter.ParentFrameApplicabilityCurrentAdapterV3{Reader: reader, Clock: func() time.Time { return now }}
	fact := runtimeports.OperationScopeEvidenceApplicabilityFactRefV3{
		Kind: "praxis.other/not-context", ID: "frame-1", Revision: 1, Digest: core.Digest(testkit.D("source")),
	}
	if _, err := adapter.InspectOperationScopeEvidenceApplicabilityCurrentV3(context.Background(), fact); !core.HasReason(err, core.ReasonUnknownGovernanceCategory) {
		t.Fatalf("unexpected error: %v", err)
	}
	if reader.calls.Load() != 0 {
		t.Fatal("wrong kind reached Context Owner reader")
	}
}

func runtimeParentFrameFactV3(source contract.ContextParentFrameApplicabilitySourceCoordinateV1) runtimeports.OperationScopeEvidenceApplicabilityFactRefV3 {
	return runtimeports.OperationScopeEvidenceApplicabilityFactRefV3{
		Kind: runtimeports.NamespacedNameV2(source.Kind), ID: source.ID,
		Revision: core.Revision(source.Revision), Digest: core.Digest(source.Digest),
	}
}

type countingParentFrameReaderV1 struct {
	calls atomic.Int32
}

var _ contextports.ContextParentFrameCurrentReaderV1 = (*countingParentFrameReaderV1)(nil)

func (r *countingParentFrameReaderV1) InspectContextParentFrameCurrentV1(context.Context, contract.ContextParentFrameApplicabilitySourceCoordinateV1) (contract.ContextParentFrameCurrentProjectionV1, error) {
	r.calls.Add(1)
	return contract.ContextParentFrameCurrentProjectionV1{}, contract.ErrUnavailable
}
