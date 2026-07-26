// Package product provides owner-local composition roots for Tool/MCP product
// surfaces. These roots do not grant Runtime authority or Provider execution.
package product

import (
	"context"
	"reflect"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/api"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/cli"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/corepack"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/registry"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/sdk"
)

const ReferencePreviewProductContractVersionV1 = "praxis.tool-mcp.reference-preview-product/v1"

// ReferencePreviewProductV1 is a self-contained, in-process product surface.
// It exposes only the strict Core Pack Preview API and its read-only CLI.
type ReferencePreviewProductV1 struct {
	ContractVersion string
	Preview         *api.CorePackPreviewV1
	CLI             *cli.RunnerV1
}

func (p ReferencePreviewProductV1) Validate() error {
	if p.ContractVersion != ReferencePreviewProductContractVersionV1 || p.Preview == nil || p.CLI == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Tool/MCP reference preview product is incomplete")
	}
	return nil
}

type ReferencePreviewFactoryV1 struct{ clock func() time.Time }

func NewReferencePreviewFactoryV1(clock func() time.Time) (*ReferencePreviewFactoryV1, error) {
	if nilLikeReferencePreviewV1(clock) {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Tool/MCP reference preview clock is required")
	}
	return &ReferencePreviewFactoryV1{clock: clock}, nil
}

// BuildV1 creates a fresh owner-local Registry closure. It deliberately does
// not accept Provider, Sandbox, Credential, Surface Current, admission, or
// production-root dependencies.
func (f *ReferencePreviewFactoryV1) BuildV1(ctx context.Context) (ReferencePreviewProductV1, error) {
	if f == nil || nilLikeReferencePreviewV1(f.clock) {
		return ReferencePreviewProductV1{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "Tool/MCP reference preview factory is unavailable")
	}
	if ctx == nil {
		return ReferencePreviewProductV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Tool/MCP reference preview context is required")
	}
	if err := ctx.Err(); err != nil {
		return ReferencePreviewProductV1{}, err
	}
	now := f.clock().UTC()
	if now.IsZero() {
		return ReferencePreviewProductV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Tool/MCP reference preview clock is invalid")
	}
	store := registry.New()
	client, err := sdk.NewV1(store, f.clock)
	if err != nil {
		return ReferencePreviewProductV1{}, err
	}
	catalog, err := api.NewCatalogV1(client)
	if err != nil {
		return ReferencePreviewProductV1{}, err
	}
	kit, err := corepack.NewCorePackAssemblyKitV1(store, client, nil, f.clock)
	if err != nil {
		return ReferencePreviewProductV1{}, err
	}
	assembly, err := corepack.NewCorePackAssemblyFactoryV1(kit)
	if err != nil {
		return ReferencePreviewProductV1{}, err
	}
	preview, err := api.NewCorePackPreviewV1(assembly, f.clock)
	if err != nil {
		return ReferencePreviewProductV1{}, err
	}
	runner, err := cli.NewRunnerWithCorePackPreviewV1(catalog, client, preview)
	if err != nil {
		return ReferencePreviewProductV1{}, err
	}
	product := ReferencePreviewProductV1{ContractVersion: ReferencePreviewProductContractVersionV1, Preview: preview, CLI: runner}
	return product, product.Validate()
}

func nilLikeReferencePreviewV1(value any) bool {
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
