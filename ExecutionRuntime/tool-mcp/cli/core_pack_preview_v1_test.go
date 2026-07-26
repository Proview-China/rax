package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/api"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/cli"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/corepack"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/sdk"
)

func newCorePackPreviewRunnerV1(t *testing.T) (*cli.RunnerV1, api.CorePackPreviewConfigV1) {
	t.Helper()
	fixture := newCLIFixtureV1(t)
	now := time.Unix(20_000, 0).UTC()
	client, err := sdk.NewV1(fixture.registry, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	kit, err := corepack.NewCorePackAssemblyKitV1(fixture.registry, client, nil, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	factory, err := corepack.NewCorePackAssemblyFactoryV1(kit)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := api.NewCorePackPreviewV1(factory, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	runner, err := cli.NewRunnerWithCorePackPreviewV1(fixture.catalog, fixture.inspector, preview)
	if err != nil {
		t.Fatal(err)
	}
	config := api.CorePackPreviewConfigV1{ContractVersion: api.CorePackPreviewContractVersionV1, Owner: core.OwnerRef{Domain: "praxis.tool-mcp", ID: "core-pack-preview-cli"}, ArtifactDigest: core.DigestBytes([]byte("artifact")), SignatureDigest: core.DigestBytes([]byte("signature")), ProvenanceDigest: core.DigestBytes([]byte("provenance")), SurfaceID: "core-pack-preview-cli-surface-v1", ResolvedPlanDigest: core.DigestBytes([]byte("plan")), ProfileDigest: core.DigestBytes([]byte("profile")), CapabilityGrantDigest: core.DigestBytes([]byte("grant")), CreatedUnixNano: now.UnixNano(), SurfaceExpiresUnixNano: now.Add(time.Hour).UnixNano(), RequestedExpiresUnixNano: now.Add(2 * time.Hour).UnixNano()}
	return runner, config
}

func TestRunnerV1CorePackPreviewSingleJSON(t *testing.T) {
	runner, config := newCorePackPreviewRunnerV1(t)
	payload, _ := json.Marshal(config)
	var output bytes.Buffer
	if err := runner.RunV1(context.Background(), []string{"tool", "core-pack", "preview", "--config-json", string(payload)}, &output); err != nil {
		t.Fatal(err)
	}
	if bytes.Count(output.Bytes(), []byte{'\n'}) != 1 {
		t.Fatalf("output=%q", output.String())
	}
	var result struct {
		ContractVersion string            `json:"contract_version"`
		Declarations    []json.RawMessage `json:"declarations"`
		ReferenceOnly   bool              `json:"reference_only"`
		Admitted        bool              `json:"admitted"`
		Executable      bool              `json:"executable"`
	}
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.ContractVersion != api.CorePackPreviewContractVersionV1 || len(result.Declarations) != 5 || !result.ReferenceOnly || result.Admitted || result.Executable {
		t.Fatalf("result=%+v", result)
	}
}

func TestRunnerV1CorePackPreviewFailuresWriteNothing(t *testing.T) {
	runner, _ := newCorePackPreviewRunnerV1(t)
	commands := [][]string{{"tool", "core-pack", "preview", "--config-json", `{}`}, {"tool", "core-pack", "admit"}, {"tool", "core-pack", "enable"}, {"tool", "core-pack", "execute"}, {"tool", "core-pack", "publish"}}
	for _, command := range commands {
		var output bytes.Buffer
		if err := runner.RunV1(context.Background(), command, &output); err == nil {
			t.Fatalf("command accepted: %v", command)
		}
		if output.Len() != 0 {
			t.Fatalf("failure wrote stdout: %q", output.String())
		}
	}
}

func TestRunnerV1CorePackPreviewRejectsTypedNil(t *testing.T) {
	fixture := newCLIFixtureV1(t)
	var preview *api.CorePackPreviewV1
	if _, err := cli.NewRunnerWithCorePackPreviewV1(fixture.catalog, fixture.inspector, preview); err == nil {
		t.Fatal("typed-nil preview accepted")
	}
}
