package modelinvokeradapter

import (
	"context"
	"encoding/json"
	"reflect"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	toolsurface "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/surface"
)

// InvocationMaterialToolPairExactReaderV2 is request-scoped. Its constructor
// freezes the real RouteCall.Request.Tools bytes; the digest argument required
// by Model's public port is never accepted as an authority input.
type InvocationMaterialToolPairExactReaderV2 struct {
	pairs              toolsurface.ModelToolInjectionPairCurrentReaderV2
	actualRequestTools []modelinvoker.Tool
	requestToolsDigest core.Digest
	clock              func() time.Time
}

func NewInvocationMaterialToolPairExactReaderV2(
	pairs toolsurface.ModelToolInjectionPairCurrentReaderV2,
	actualRequestTools []modelinvoker.Tool,
	clock func() time.Time,
) (*InvocationMaterialToolPairExactReaderV2, error) {
	if nilLikeToolPairV2(pairs) || clock == nil {
		return nil, toolPairInvalidV2("invocation material Tool pair adapter dependencies are incomplete")
	}
	frozen := cloneModelToolsV2(actualRequestTools)
	digest, err := modelinvoker.DigestGovernedModelTurnRequestToolSetV2(frozen)
	if err != nil {
		return nil, err
	}
	return &InvocationMaterialToolPairExactReaderV2{
		pairs:              pairs,
		actualRequestTools: frozen,
		requestToolsDigest: digest,
		clock:              clock,
	}, nil
}

var _ modelinvoker.InvocationMaterialToolPairExactReaderV2 = (*InvocationMaterialToolPairExactReaderV2)(nil)

func (a *InvocationMaterialToolPairExactReaderV2) InspectExactInvocationToolPairV2(
	ctx context.Context,
	injection modelinvoker.InvocationMaterialExactSourceRefV1,
	surface modelinvoker.InvocationMaterialExactSourceRefV1,
	callerRequestToolsDigest core.Digest,
) (modelinvoker.InvocationMaterialToolPairProjectionV2, error) {
	if ctx == nil {
		return modelinvoker.InvocationMaterialToolPairProjectionV2{}, toolPairInvalidV2("invocation material Tool pair context is required")
	}
	if err := ctx.Err(); err != nil {
		return modelinvoker.InvocationMaterialToolPairProjectionV2{}, err
	}
	if a == nil || nilLikeToolPairV2(a.pairs) || a.clock == nil ||
		a.requestToolsDigest.Validate() != nil {
		return modelinvoker.InvocationMaterialToolPairProjectionV2{}, toolPairUnavailableV2("invocation material Tool pair adapter is unavailable")
	}
	if err := validateModelToolPairSourcesV2(injection, surface); err != nil {
		return modelinvoker.InvocationMaterialToolPairProjectionV2{}, err
	}

	// Recompute from the request-scoped frozen bytes. The caller digest can
	// only prove equality; it cannot choose or replace the authoritative value.
	actualRequestToolsDigest, err := modelinvoker.DigestGovernedModelTurnRequestToolSetV2(
		a.actualRequestTools,
	)
	if err != nil {
		return modelinvoker.InvocationMaterialToolPairProjectionV2{}, err
	}
	if actualRequestToolsDigest != a.requestToolsDigest ||
		callerRequestToolsDigest.Validate() != nil ||
		callerRequestToolsDigest != actualRequestToolsDigest {
		return modelinvoker.InvocationMaterialToolPairProjectionV2{}, toolPairConflictV2("caller Request Tools digest differs from actual RouteCall.Request.Tools")
	}

	materialRef := toolcontract.ModelToolInjectionMaterialRefV1{
		ContractVersion: toolcontract.ModelToolInjectionMaterialContractVersionV1,
		ID:              injection.ID,
		Revision:        injection.Revision,
		Digest:          injection.Digest,
	}
	surfaceRef := toolcontract.ToolSurfaceManifestCurrentRefV1{
		ContractVersion: toolcontract.ToolSurfaceManifestCurrentContractVersionV1,
		ID:              surface.ID,
		Revision:        surface.Revision,
		Digest:          surface.Digest,
	}
	if materialRef.Validate() != nil || surfaceRef.Validate() != nil {
		return modelinvoker.InvocationMaterialToolPairProjectionV2{}, toolPairConflictV2("Model exact sources cannot map losslessly to Tool refs")
	}
	toolProjection, err := a.pairs.InspectCurrentModelToolInjectionPairV2(
		ctx,
		toolsurface.ModelToolInjectionPairCurrentInspectRequestV2{
			Material:           materialRef,
			Surface:            surfaceRef,
			ActualRequestTools: cloneModelToolsV2(a.actualRequestTools),
		},
	)
	if err != nil {
		return modelinvoker.InvocationMaterialToolPairProjectionV2{}, err
	}
	if err := ctx.Err(); err != nil {
		return modelinvoker.InvocationMaterialToolPairProjectionV2{}, err
	}
	now := a.clock()
	if now.IsZero() {
		return modelinvoker.InvocationMaterialToolPairProjectionV2{}, toolPairInvalidV2("invocation material Tool pair adapter clock is required")
	}
	expectedMaterialSource, err := toolcontract.ModelToolInjectionMaterialSourceRefV1(
		injection.Owner,
		materialRef,
	)
	if err != nil {
		return modelinvoker.InvocationMaterialToolPairProjectionV2{}, err
	}
	expectedSurfaceSource := toolcontract.ModelToolInjectionLineageSourceRefV1{
		Owner:    surface.Owner,
		Kind:     toolcontract.ToolSurfaceManifestCurrentSourceKindV1,
		ID:       surfaceRef.ID,
		Revision: surfaceRef.Revision,
		Digest:   surfaceRef.Digest,
	}
	if err = expectedSurfaceSource.Validate(); err != nil {
		return modelinvoker.InvocationMaterialToolPairProjectionV2{}, err
	}
	if err = toolProjection.ValidateCurrent(
		expectedMaterialSource,
		expectedSurfaceSource,
		actualRequestToolsDigest,
		now,
	); err != nil {
		return modelinvoker.InvocationMaterialToolPairProjectionV2{}, err
	}
	mappedMaterial, err := mapToolSourceToModelV2(toolProjection.MaterialSource)
	if err != nil {
		return modelinvoker.InvocationMaterialToolPairProjectionV2{}, err
	}
	mappedSurface, err := mapToolSourceToModelV2(toolProjection.SurfaceSource)
	if err != nil {
		return modelinvoker.InvocationMaterialToolPairProjectionV2{}, err
	}
	if mappedMaterial != injection || mappedSurface != surface ||
		toolProjection.StoredCompiledToolsDigest != toolProjection.ActualCompiledToolsDigest {
		return modelinvoker.InvocationMaterialToolPairProjectionV2{}, toolPairConflictV2("authoritative Tool pair cannot map losslessly to Model sources")
	}

	projection, err := modelinvoker.SealInvocationMaterialToolPairProjectionV2(
		modelinvoker.InvocationMaterialToolPairProjectionV2{
			ToolInjectionMaterial:   mappedMaterial,
			ToolSurface:             mappedSurface,
			ExpectedInjectionDigest: toolProjection.ExpectedInjectionDigest,
			CompiledToolsDigest:     toolProjection.ActualCompiledToolsDigest,
			RequestToolsDigest:      toolProjection.RequestToolsDigest,
			CheckedUnixNano:         toolProjection.CheckedUnixNano,
			ExpiresUnixNano:         toolProjection.ExpiresUnixNano,
		},
	)
	if err != nil {
		return modelinvoker.InvocationMaterialToolPairProjectionV2{}, err
	}
	if err = projection.ValidateCurrentV2(
		injection,
		surface,
		toolProjection.ExpectedInjectionDigest,
		toolProjection.ActualCompiledToolsDigest,
		actualRequestToolsDigest,
		now,
	); err != nil {
		return modelinvoker.InvocationMaterialToolPairProjectionV2{}, err
	}
	return projection, nil
}

func validateModelToolPairSourcesV2(
	injection modelinvoker.InvocationMaterialExactSourceRefV1,
	surface modelinvoker.InvocationMaterialExactSourceRefV1,
) error {
	if injection.Validate() != nil || surface.Validate() != nil {
		return toolPairInvalidV2("invocation material Tool pair exact source is invalid")
	}
	if injection.Kind != string(toolcontract.ModelToolInjectionMaterialSourceKindV1) ||
		surface.Kind != string(toolcontract.ToolSurfaceManifestCurrentSourceKindV1) {
		return toolPairConflictV2("invocation material Tool pair source Kind drifted")
	}
	if injection.Owner != surface.Owner || injection == surface {
		return toolPairConflictV2("invocation material Tool pair source Owner or role collapsed")
	}
	return nil
}

func mapToolSourceToModelV2(
	source toolcontract.ModelToolInjectionLineageSourceRefV1,
) (modelinvoker.InvocationMaterialExactSourceRefV1, error) {
	mapped := modelinvoker.InvocationMaterialExactSourceRefV1{
		Owner:    source.Owner,
		Kind:     string(source.Kind),
		ID:       source.ID,
		Revision: source.Revision,
		Digest:   source.Digest,
	}
	if source.Validate() != nil || mapped.Validate() != nil ||
		mapped.Owner != source.Owner ||
		mapped.Kind != string(source.Kind) ||
		mapped.ID != source.ID ||
		mapped.Revision != source.Revision ||
		mapped.Digest != source.Digest {
		return modelinvoker.InvocationMaterialExactSourceRefV1{}, toolPairConflictV2("Tool exact source cannot map losslessly to Model")
	}
	return mapped, nil
}

func cloneModelToolsV2(tools []modelinvoker.Tool) []modelinvoker.Tool {
	cloned := append([]modelinvoker.Tool(nil), tools...)
	for index := range cloned {
		cloned[index].Parameters = append(json.RawMessage(nil), cloned[index].Parameters...)
		if cloned[index].Strict != nil {
			strict := *cloned[index].Strict
			cloned[index].Strict = &strict
		}
	}
	return cloned
}

func nilLikeToolPairV2(value any) bool {
	if value == nil {
		return true
	}
	ref := reflect.ValueOf(value)
	switch ref.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return ref.IsNil()
	default:
		return false
	}
}

func toolPairInvalidV2(message string) error {
	return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, message)
}

func toolPairConflictV2(message string) error {
	return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, message)
}

func toolPairUnavailableV2(message string) error {
	return core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, message)
}
