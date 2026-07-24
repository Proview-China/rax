package sdk_test

import (
	"context"
	"sync/atomic"
	"testing"

	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/registry"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/sdk"
)

type packageVerificationSDKPortV1 struct {
	fixture testkit.PackageVerificationFixtureV1
	calls   atomic.Int64
}

func (p *packageVerificationSDKPortV1) VerifyV1(ctx context.Context, request toolcontract.ToolPackageVerifyRequestV1) (toolcontract.ToolPackageVerificationFactV1, error) {
	p.calls.Add(1)
	return p.fixture.Fact, ctx.Err()
}

func (p *packageVerificationSDKPortV1) ResolveCurrentToolPackageVerificationV1(ctx context.Context, _ toolcontract.ToolPackageVerificationCurrentIssuanceV1) (toolcontract.ToolPackageVerificationCurrentProjectionV1, error) {
	p.calls.Add(1)
	return p.fixture.Current, ctx.Err()
}

func (p *packageVerificationSDKPortV1) InspectCurrentToolPackageVerificationV1(ctx context.Context, _ toolcontract.ToolPackageVerificationCurrentRefV1) (toolcontract.ToolPackageVerificationCurrentProjectionV1, error) {
	p.calls.Add(1)
	return p.fixture.Current, ctx.Err()
}

func (p *packageVerificationSDKPortV1) InspectExactToolPackageVerificationObservationV1(ctx context.Context, _ toolcontract.ToolPackageVerificationObservationRefV1) (toolcontract.ToolPackageVerificationObservationV1, error) {
	p.calls.Add(1)
	return p.fixture.Observation, ctx.Err()
}

func (p *packageVerificationSDKPortV1) InspectExactToolPackageVerificationFactV1(ctx context.Context, _ toolcontract.ToolPackageVerificationFactRefV1) (toolcontract.ToolPackageVerificationFactV1, error) {
	p.calls.Add(1)
	return p.fixture.Fact, ctx.Err()
}

func (p *packageVerificationSDKPortV1) AdmitPackageV1(ctx context.Context, _ toolcontract.ToolPackageAdmissionCommandV1) (registry.Record, error) {
	p.calls.Add(1)
	return registry.Record{Kind: "package", ID: p.fixture.Fact.Package.ID, ObjectRevision: p.fixture.Fact.Package.Revision, ObjectDigest: p.fixture.Fact.Package.Digest, State: registry.StateAdmitted, RegistryRevision: 8, UpdatedUnixNano: testkit.FixedTime.UnixNano()}, ctx.Err()
}

func TestPackageVerificationSDKExactSurfaceV1(t *testing.T) {
	fixture := testkit.PackageVerificationV1()
	port := &packageVerificationSDKPortV1{fixture: fixture}
	client, err := sdk.NewPackageVerificationV1(port, port, port)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if fact, err := client.VerifyPackageV1(ctx, fixture.Request); err != nil || fact.Ref != fixture.Fact.Ref {
		t.Fatalf("Verify=%+v err=%v", fact, err)
	}
	if current, err := client.ResolvePackageVerificationCurrentV1(ctx, fixture.Current.Issuance); err != nil || current.Ref != fixture.Current.Ref {
		t.Fatalf("Resolve=%+v err=%v", current, err)
	}
	if observation, err := client.InspectPackageVerificationObservationV1(ctx, fixture.Observation.Ref); err != nil || observation.Ref != fixture.Observation.Ref {
		t.Fatalf("Observation=%+v err=%v", observation, err)
	}
	if fact, err := client.InspectPackageVerificationFactV1(ctx, fixture.Fact.Ref); err != nil || fact.Ref != fixture.Fact.Ref {
		t.Fatalf("Fact=%+v err=%v", fact, err)
	}
	if current, err := client.InspectPackageVerificationCurrentV1(ctx, fixture.Current.Ref); err != nil || current.Ref != fixture.Current.Ref {
		t.Fatalf("Current=%+v err=%v", current, err)
	}
	command := toolcontract.ToolPackageAdmissionCommandV1{ContractVersion: toolcontract.PackageVerificationContractVersionV1, VerificationCurrent: fixture.Current.Ref, ExpectedRegistryRevision: fixture.Current.CurrentPackageRegistry.RegistryRevision}
	if admitted, err := client.AdmitVerifiedPackageV1(ctx, command); err != nil || admitted.State != registry.StateAdmitted {
		t.Fatalf("Admission=%+v err=%v", admitted, err)
	}
}

func TestPackageVerificationSDKRejectsTypedNilNilAndCanceledContextV1(t *testing.T) {
	var typedNil *packageVerificationSDKPortV1
	if _, err := sdk.NewPackageVerificationV1(typedNil, typedNil, typedNil); err == nil {
		t.Fatal("typed-nil dependencies were accepted")
	}
	port := &packageVerificationSDKPortV1{fixture: testkit.PackageVerificationV1()}
	client, err := sdk.NewPackageVerificationV1(port, port, port)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = client.VerifyPackageV1(nil, port.fixture.Request); err == nil || port.calls.Load() != 0 {
		t.Fatalf("nil context reached port: err=%v calls=%d", err, port.calls.Load())
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = client.VerifyPackageV1(ctx, port.fixture.Request); err != context.Canceled || port.calls.Load() != 0 {
		t.Fatalf("canceled context reached port: err=%v calls=%d", err, port.calls.Load())
	}
}
