package api_test

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/api"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
)

type packageVerificationReadFixtureV1 struct {
	fixture testkit.PackageVerificationFixtureV1
	drift   bool
	calls   atomic.Int64
}

func (r *packageVerificationReadFixtureV1) InspectExactToolPackageVerificationObservationV1(ctx context.Context, _ toolcontract.ToolPackageVerificationObservationRefV1) (toolcontract.ToolPackageVerificationObservationV1, error) {
	r.calls.Add(1)
	value := r.fixture.Observation
	if r.drift && r.calls.Load()%2 == 0 {
		value.Ref.Digest = testkit.Digest("drift")
	}
	return value, ctx.Err()
}

func (r *packageVerificationReadFixtureV1) InspectExactToolPackageVerificationFactV1(ctx context.Context, _ toolcontract.ToolPackageVerificationFactRefV1) (toolcontract.ToolPackageVerificationFactV1, error) {
	r.calls.Add(1)
	value := r.fixture.Fact
	if r.drift && r.calls.Load()%2 == 0 {
		value.Ref.Digest = testkit.Digest("drift")
	}
	return value, ctx.Err()
}

func (r *packageVerificationReadFixtureV1) InspectCurrentToolPackageVerificationV1(ctx context.Context, _ toolcontract.ToolPackageVerificationCurrentRefV1) (toolcontract.ToolPackageVerificationCurrentProjectionV1, error) {
	r.calls.Add(1)
	value := r.fixture.Current
	if r.drift && r.calls.Load()%2 == 0 {
		value.ProjectionDigest = testkit.Digest("drift")
	}
	return value, ctx.Err()
}

func TestPackageVerificationReadExactDoubleInspectV1(t *testing.T) {
	fixture := &packageVerificationReadFixtureV1{fixture: testkit.PackageVerificationV1()}
	reader, err := api.NewPackageVerificationReadV1(fixture, fixture)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if got, err := reader.InspectPackageVerificationObservationV1(ctx, fixture.fixture.Observation.Ref); err != nil || got.Ref != fixture.fixture.Observation.Ref {
		t.Fatalf("Observation=%+v err=%v", got, err)
	}
	if got, err := reader.InspectPackageVerificationFactV1(ctx, fixture.fixture.Fact.Ref); err != nil || got.Ref != fixture.fixture.Fact.Ref {
		t.Fatalf("Fact=%+v err=%v", got, err)
	}
	if got, err := reader.InspectPackageVerificationCurrentV1(ctx, fixture.fixture.Current.Ref); err != nil || got.Ref != fixture.fixture.Current.Ref {
		t.Fatalf("Current=%+v err=%v", got, err)
	}
}

func TestPackageVerificationReadFailsClosedOnDriftAndTypedNilV1(t *testing.T) {
	var typedNil *packageVerificationReadFixtureV1
	if _, err := api.NewPackageVerificationReadV1(typedNil, typedNil); err == nil {
		t.Fatal("typed-nil dependencies were accepted")
	}
	fixture := &packageVerificationReadFixtureV1{fixture: testkit.PackageVerificationV1(), drift: true}
	reader, err := api.NewPackageVerificationReadV1(fixture, fixture)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = reader.InspectPackageVerificationObservationV1(context.Background(), fixture.fixture.Observation.Ref); err == nil {
		t.Fatal("S1/S2 Observation drift was accepted")
	}
}
