package runtimeadapter_test

import (
	"context"
	"testing"
	"time"

	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/runtimeadapter"
)

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }

type boundarySource struct {
	value contract.SingleCallToolActionCoordinationWatermarkV1
}

func (s boundarySource) InspectBoundarySourceCurrentV1(_ context.Context, ref contract.ToolProviderBoundarySourceRefV1, now time.Time) (contract.SingleCallToolActionCoordinationWatermarkV1, error) {
	if s.value.ID != ref.WatermarkID || s.value.Revision != ref.WatermarkRevision || s.value.Digest != ref.WatermarkDigest || !contract.IsCoordinationCurrentV1(s.value, now) {
		return contract.SingleCallToolActionCoordinationWatermarkV1{}, context.Canceled
	}
	return s.value, nil
}

func TestProviderBoundaryAdapterExactLosslessMappingAndDrift(t *testing.T) {
	now := testkit.FixedTime
	fixture := testkit.BoundaryFixture(now)
	w, err := contract.SealCoordinationWatermarkV1(contract.SingleCallToolActionCoordinationWatermarkV1{ID: "tool-watermark-v2", Revision: 5, TenantID: "tenant-v2", ApplicationRequestID: "application-request-v2", ApplicationRequestRevision: 1, ApplicationRequestDigest: testkit.Digest("application-request"), OperationScopeDigest: testkit.Digest("scope"), ModelProjection: testkit.ModelProjection(1).Ref, ObservationDigest: testkit.ModelProjection(1).Observation.Digest, CanonicalCommandDigest: testkit.Digest("command"), Stage: contract.CoordinationProviderBoundaryV1, Owner: testkit.SettlementOwner(), ActionCandidate: &contract.ObjectRef{ID: "action-v2", Revision: 1, Digest: testkit.Digest("action")}, Reservation: &contract.ObjectRef{ID: "reservation-v2", Revision: 1, Digest: testkit.Digest("reservation")}, RuntimeAttempt: &fixture.Attempt, Operation: &fixture.Operation, OperationDigest: fixture.Attempt.OperationDigest, ExecuteEnforcement: &fixture.Enforcement, ExecuteHandoff: ptrHandoff(fixture.Handoff.RefV3()), CreatedUnixNano: now.Add(-time.Second).UnixNano(), UpdatedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(10 * time.Second).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	source, err := w.BoundarySourceRefV1()
	if err != nil {
		t.Fatal(err)
	}
	ref, _ := source.RuntimeRefV1()
	adapter := runtimeadapter.NewProviderBoundaryCurrentAdapterV1(boundarySource{w}, fixedClock{now.Add(time.Millisecond)})
	projection, err := adapter.InspectCurrentOperationProviderBoundaryV1(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	if err := projection.ValidateCurrent(ref, fixture.Operation, w.OperationScopeDigest, fixture.Attempt, fixture.Enforcement, fixture.Handoff.RefV3(), now.Add(time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	drift := ref
	drift.Digest = testkit.Digest("drift")
	if _, err := adapter.InspectCurrentOperationProviderBoundaryV1(context.Background(), drift); err == nil {
		t.Fatal("same ID/revision with another digest passed")
	}
	var _ runtimeports.OperationProviderBoundaryCurrentReaderV1 = adapter
}
func ptrHandoff(v runtimeports.OperationScopeEvidenceProviderHandoffRefV3) *runtimeports.OperationScopeEvidenceProviderHandoffRefV3 {
	return &v
}
