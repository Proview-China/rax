package sqlite

import (
	"context"

	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	toolsurface "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/surface"
)

// InspectExactModelToolInjectionClosureV1 returns the exact compiled tools and
// Material from the same durable SQLite row. Neither half may be substituted by
// a separately supplied or reconstructed value.
func (s *StoreV1) InspectExactModelToolInjectionClosureV1(
	ctx context.Context,
	exact toolcontract.ModelToolInjectionMaterialRefV1,
) (toolsurface.CompiledModelToolsV1, toolcontract.ModelToolInjectionMaterialV1, error) {
	if err := s.readReadyV1(ctx); err != nil {
		return modelToolClosureZeroV1(err)
	}
	if err := exact.Validate(); err != nil {
		return modelToolClosureZeroV1(err)
	}
	compiled, material, err := inspectMaterialQueryV1(ctx, s.db, exact.ID)
	if err != nil {
		return modelToolClosureZeroV1(err)
	}
	if material.Ref != exact {
		return modelToolClosureZeroV1(conflictV1("Model Tool Injection closure exact Material Ref drifted"))
	}
	if err = material.ValidateCurrent(exact, s.clock()); err != nil {
		return modelToolClosureZeroV1(err)
	}
	if err = compiled.ValidateAgainstMaterialV1(material); err != nil {
		return modelToolClosureZeroV1(conflictV1("stored Model Tool Injection compiled closure drifted"))
	}
	return compiled.Clone(), material.Clone(), nil
}

var _ toolsurface.ModelToolInjectionExactClosureReaderV1 = (*StoreV1)(nil)
