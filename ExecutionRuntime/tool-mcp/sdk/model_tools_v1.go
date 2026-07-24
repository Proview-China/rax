package sdk

import (
	"context"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/surface"
)

type ModelToolAssemblyV1 struct {
	surfaces  toolcontract.ToolSurfaceManifestCurrentReaderV1
	materials toolcontract.ToolDefinitionMaterialRepositoryV1
	clock     ClockV1
}

func NewModelToolAssemblyV1(surfaces toolcontract.ToolSurfaceManifestCurrentReaderV1, materials toolcontract.ToolDefinitionMaterialRepositoryV1, clock ClockV1) (*ModelToolAssemblyV1, error) {
	if nilLikeV1(surfaces) || nilLikeV1(materials) || clock == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Model Tool Assembly SDK dependencies are required")
	}
	return &ModelToolAssemblyV1{surfaces: surfaces, materials: materials, clock: clock}, nil
}

func (s *ModelToolAssemblyV1) EnsureToolDefinitionMaterialV1(ctx context.Context, material toolcontract.ToolDefinitionMaterialV1) (toolcontract.ToolDefinitionMaterialV1, error) {
	if err := s.readyV1(ctx); err != nil {
		return toolcontract.ToolDefinitionMaterialV1{}, err
	}
	return s.materials.EnsureExactToolDefinitionMaterialV1(ctx, material)
}

func (s *ModelToolAssemblyV1) InspectToolDefinitionMaterialV1(ctx context.Context, exact toolcontract.ToolDefinitionMaterialRefV1) (toolcontract.ToolDefinitionMaterialV1, error) {
	if err := s.readyV1(ctx); err != nil {
		return toolcontract.ToolDefinitionMaterialV1{}, err
	}
	return s.materials.InspectExactToolDefinitionMaterialV1(ctx, exact)
}

func (s *ModelToolAssemblyV1) CompileModelToolsV1(ctx context.Context, exact toolcontract.ToolSurfaceManifestCurrentRefV1) (surface.CompiledModelToolsV1, error) {
	first, err := s.nowV1(ctx)
	if err != nil {
		return surface.CompiledModelToolsV1{}, err
	}
	if err := exact.Validate(); err != nil {
		return surface.CompiledModelToolsV1{}, err
	}
	current, err := s.surfaces.InspectExactToolSurfaceManifestCurrentV1(ctx, exact)
	if err != nil {
		return surface.CompiledModelToolsV1{}, err
	}
	second, err := s.nowV1(ctx)
	if err != nil {
		return surface.CompiledModelToolsV1{}, err
	}
	if second.Before(first) {
		return surface.CompiledModelToolsV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Model Tool Assembly SDK clock regressed across Surface inspection")
	}
	if err := current.ValidateCurrent(exact, second); err != nil {
		return surface.CompiledModelToolsV1{}, err
	}
	return surface.CompileModelToolsV1(ctx, current, s.materials, func() time.Time { return s.clock().UTC() })
}

func (s *ModelToolAssemblyV1) readyV1(ctx context.Context) error {
	_, err := s.nowV1(ctx)
	return err
}

func (s *ModelToolAssemblyV1) nowV1(ctx context.Context) (time.Time, error) {
	if s == nil || nilLikeV1(s.surfaces) || nilLikeV1(s.materials) || s.clock == nil {
		return time.Time{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "Model Tool Assembly SDK is unavailable")
	}
	if ctx == nil {
		return time.Time{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Model Tool Assembly SDK context is required")
	}
	if err := ctx.Err(); err != nil {
		return time.Time{}, err
	}
	now := s.clock()
	if now.IsZero() {
		return time.Time{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Model Tool Assembly SDK clock is unavailable")
	}
	return now.UTC(), nil
}
