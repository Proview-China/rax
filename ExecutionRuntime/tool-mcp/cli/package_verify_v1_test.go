package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/cli"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
)

type packageVerificationCLIPortV1 struct {
	fixture testkit.PackageVerificationFixtureV1
	calls   atomic.Int64
}

func (p *packageVerificationCLIPortV1) VerifyPackageV1(ctx context.Context, request toolcontract.ToolPackageVerifyRequestV1) (toolcontract.ToolPackageVerificationFactV1, error) {
	p.calls.Add(1)
	if err := ctx.Err(); err != nil {
		return toolcontract.ToolPackageVerificationFactV1{}, err
	}
	if request.Subject != p.fixture.Request.Subject || request.TrustPolicyCurrent != p.fixture.Request.TrustPolicyCurrent {
		return toolcontract.ToolPackageVerificationFactV1{}, context.Canceled
	}
	return p.fixture.Fact, nil
}

func TestRunnerPackageVerifyExactJSONV1(t *testing.T) {
	base := newCLIFixtureV1(t)
	port := &packageVerificationCLIPortV1{fixture: testkit.PackageVerificationV1()}
	runner, err := cli.NewRunnerWithPackageVerificationV1(base.catalog, base.inspector, port)
	if err != nil {
		t.Fatal(err)
	}
	requestJSON, err := json.Marshal(port.fixture.Request)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err = runner.RunV1(context.Background(), []string{"package", "verify", "--request-json=" + string(requestJSON)}, &output); err != nil {
		t.Fatal(err)
	}
	var result cli.PackageVerificationOutputV1
	if err = json.Unmarshal(output.Bytes(), &result); err != nil || result.ContractVersion != cli.ContractVersionV1 || result.Fact.Ref != port.fixture.Fact.Ref || port.calls.Load() != 1 {
		t.Fatalf("Package Verify output=%+v calls=%d err=%v", result, port.calls.Load(), err)
	}
}

func TestRunnerPackageVerifyRejectsUnknownJSONWithoutCallingPortV1(t *testing.T) {
	base := newCLIFixtureV1(t)
	port := &packageVerificationCLIPortV1{fixture: testkit.PackageVerificationV1()}
	runner, err := cli.NewRunnerWithPackageVerificationV1(base.catalog, base.inspector, port)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err = runner.RunV1(context.Background(), []string{"package", "verify", `--request-json={"unknown":true}`}, &output); err == nil || output.Len() != 0 || port.calls.Load() != 0 {
		t.Fatalf("invalid Package Verify reached Port: err=%v output=%q calls=%d", err, output.String(), port.calls.Load())
	}
}

func TestRunnerPackageVerifyRejectsTypedNilV1(t *testing.T) {
	base := newCLIFixtureV1(t)
	var typedNil *packageVerificationCLIPortV1
	if _, err := cli.NewRunnerWithPackageVerificationV1(base.catalog, base.inspector, typedNil); err == nil {
		t.Fatal("typed-nil Package Verification Port was accepted")
	}
}
