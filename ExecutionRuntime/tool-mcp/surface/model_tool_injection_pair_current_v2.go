package surface

import (
	"context"
	"reflect"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

// ModelToolInjectionPairCurrentInspectRequestV2 carries the actual
// RouteCall.Request.Tools bytes. It deliberately has no digest or ToolChoice
// field: the Tool owner derives both Tool and Model digests from these bytes.
type ModelToolInjectionPairCurrentInspectRequestV2 struct {
	Material           toolcontract.ModelToolInjectionMaterialRefV1 `json:"material"`
	Surface            toolcontract.ToolSurfaceManifestCurrentRefV1 `json:"surface"`
	ActualRequestTools []modelinvoker.Tool                          `json:"actual_request_tools"`
}

func (r ModelToolInjectionPairCurrentInspectRequestV2) Validate() error {
	if r.Material.Validate() != nil || r.Surface.Validate() != nil {
		return lineageInvalidV1("Model Tool Injection pair current exact request is invalid")
	}
	if _, err := modelinvoker.DigestGovernedModelTurnRequestToolSetV2(r.ActualRequestTools); err != nil {
		return err
	}
	return nil
}

func (r ModelToolInjectionPairCurrentInspectRequestV2) clone() ModelToolInjectionPairCurrentInspectRequestV2 {
	r.ActualRequestTools = cloneRouteCallToolsV1(r.ActualRequestTools)
	return r
}

type ModelToolInjectionPairCurrentReaderV2 interface {
	InspectCurrentModelToolInjectionPairV2(
		context.Context,
		ModelToolInjectionPairCurrentInspectRequestV2,
	) (toolcontract.ModelToolInjectionPairCurrentProjectionV2, error)
}

type AuthoritativeModelToolInjectionPairCurrentReaderV2 struct {
	closures ModelToolInjectionExactClosureReaderV1
	surfaces toolcontract.ToolSurfaceManifestCurrentReaderV1
	owner    core.OwnerRef
	clock    func() time.Time
}

func NewAuthoritativeModelToolInjectionPairCurrentReaderV2(
	closures ModelToolInjectionExactClosureReaderV1,
	surfaces toolcontract.ToolSurfaceManifestCurrentReaderV1,
	owner core.OwnerRef,
	clock func() time.Time,
) (*AuthoritativeModelToolInjectionPairCurrentReaderV2, error) {
	if lineageNilV1(closures) || lineageNilV1(surfaces) ||
		lineageNilV1(clock) || owner.Validate() != nil {
		return nil, lineageInvalidV1("Model Tool Injection pair current Reader dependencies are incomplete")
	}
	return &AuthoritativeModelToolInjectionPairCurrentReaderV2{
		closures: closures,
		surfaces: surfaces,
		owner:    owner,
		clock:    clock,
	}, nil
}

var _ ModelToolInjectionPairCurrentReaderV2 = (*AuthoritativeModelToolInjectionPairCurrentReaderV2)(nil)

func (r *AuthoritativeModelToolInjectionPairCurrentReaderV2) InspectCurrentModelToolInjectionPairV2(
	ctx context.Context,
	request ModelToolInjectionPairCurrentInspectRequestV2,
) (toolcontract.ModelToolInjectionPairCurrentProjectionV2, error) {
	if ctx == nil {
		return toolcontract.ModelToolInjectionPairCurrentProjectionV2{}, lineageInvalidV1("Model Tool Injection pair current context is required")
	}
	if err := ctx.Err(); err != nil {
		return toolcontract.ModelToolInjectionPairCurrentProjectionV2{}, err
	}
	if r == nil || lineageNilV1(r.closures) || lineageNilV1(r.surfaces) ||
		lineageNilV1(r.clock) || r.owner.Validate() != nil {
		return toolcontract.ModelToolInjectionPairCurrentProjectionV2{}, lineageUnavailableV1("Model Tool Injection pair current Reader is unavailable")
	}
	request = request.clone()
	if err := request.Validate(); err != nil {
		return toolcontract.ModelToolInjectionPairCurrentProjectionV2{}, err
	}

	s1 := r.clock()
	if s1.IsZero() {
		return toolcontract.ModelToolInjectionPairCurrentProjectionV2{}, lineageInvalidV1("Model Tool Injection pair current S1 clock is required")
	}
	compiledS1, materialS1, surfaceS1, err := r.readExactPairV2(ctx, request, s1)
	if err != nil {
		return toolcontract.ModelToolInjectionPairCurrentProjectionV2{}, err
	}
	actualCompiledS1, requestToolsS1, err := validateActualRequestToolsV2(
		request.ActualRequestTools,
		compiledS1,
		materialS1,
		surfaceS1,
	)
	if err != nil {
		return toolcontract.ModelToolInjectionPairCurrentProjectionV2{}, err
	}
	if err := ctx.Err(); err != nil {
		return toolcontract.ModelToolInjectionPairCurrentProjectionV2{}, err
	}

	compiledS2, materialS2, surfaceS2, err := r.readExactPairV2(ctx, request, s1)
	if err != nil {
		return toolcontract.ModelToolInjectionPairCurrentProjectionV2{}, err
	}
	s2 := r.clock()
	if s2.IsZero() || s2.Before(s1) {
		return toolcontract.ModelToolInjectionPairCurrentProjectionV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Model Tool Injection pair current S2 clock regressed")
	}
	if !reflect.DeepEqual(compiledS1, compiledS2) ||
		!reflect.DeepEqual(materialS1, materialS2) ||
		!reflect.DeepEqual(surfaceS1, surfaceS2) {
		return toolcontract.ModelToolInjectionPairCurrentProjectionV2{}, lineageConflictV1("Model Tool Injection pair current drifted between S1 and S2")
	}
	if err = validatePairCurrentClosureV2(
		compiledS2, materialS2, surfaceS2, request, r.owner, s2,
	); err != nil {
		return toolcontract.ModelToolInjectionPairCurrentProjectionV2{}, err
	}
	actualCompiledS2, requestToolsS2, err := validateActualRequestToolsV2(
		request.ActualRequestTools,
		compiledS2,
		materialS2,
		surfaceS2,
	)
	if err != nil {
		return toolcontract.ModelToolInjectionPairCurrentProjectionV2{}, err
	}
	if actualCompiledS1 != actualCompiledS2 || requestToolsS1 != requestToolsS2 {
		return toolcontract.ModelToolInjectionPairCurrentProjectionV2{}, lineageConflictV1("actual Model request Tool digest drifted between S1 and S2")
	}
	if err := ctx.Err(); err != nil {
		return toolcontract.ModelToolInjectionPairCurrentProjectionV2{}, err
	}

	expires := minLineageExpiryV1(materialS2.ExpiresUnixNano, surfaceS2.ExpiresUnixNano)
	if !s2.Before(time.Unix(0, expires)) {
		return toolcontract.ModelToolInjectionPairCurrentProjectionV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "Model Tool Injection pair current expired before Seal")
	}
	materialSource, err := toolcontract.ModelToolInjectionMaterialSourceRefV1(r.owner, materialS2.Ref)
	if err != nil {
		return toolcontract.ModelToolInjectionPairCurrentProjectionV2{}, err
	}
	surfaceSource, err := toolcontract.ToolSurfaceManifestSourceRefV1(surfaceS2)
	if err != nil {
		return toolcontract.ModelToolInjectionPairCurrentProjectionV2{}, err
	}
	projection, err := toolcontract.SealModelToolInjectionPairCurrentV2(
		toolcontract.ModelToolInjectionPairCurrentProjectionV2{
			MaterialSource:            materialSource,
			SurfaceSource:             surfaceSource,
			ExpectedInjectionDigest:   materialS2.ExpectedInjectionDigest,
			StoredCompiledToolsDigest: compiledS2.Digest,
			ActualCompiledToolsDigest: actualCompiledS2,
			RequestToolsDigest:        requestToolsS2,
			CheckedUnixNano:           s2.UnixNano(),
			ExpiresUnixNano:           expires,
		},
	)
	if err != nil {
		return toolcontract.ModelToolInjectionPairCurrentProjectionV2{}, err
	}
	if err = projection.ValidateCurrent(materialSource, surfaceSource, requestToolsS2, s2); err != nil {
		return toolcontract.ModelToolInjectionPairCurrentProjectionV2{}, err
	}
	return projection, nil
}

func (r *AuthoritativeModelToolInjectionPairCurrentReaderV2) readExactPairV2(
	ctx context.Context,
	request ModelToolInjectionPairCurrentInspectRequestV2,
	now time.Time,
) (
	CompiledModelToolsV1,
	toolcontract.ModelToolInjectionMaterialV1,
	toolcontract.ToolSurfaceManifestCurrentProjectionV1,
	error,
) {
	compiled, material, err := r.closures.InspectExactModelToolInjectionClosureV1(ctx, request.Material)
	if err != nil {
		return CompiledModelToolsV1{}, toolcontract.ModelToolInjectionMaterialV1{}, toolcontract.ToolSurfaceManifestCurrentProjectionV1{}, err
	}
	if material.Surface != request.Surface {
		return CompiledModelToolsV1{}, toolcontract.ModelToolInjectionMaterialV1{}, toolcontract.ToolSurfaceManifestCurrentProjectionV1{}, lineageConflictV1("Model Tool Injection Material does not bind the requested current Surface")
	}
	surface, err := r.surfaces.InspectExactToolSurfaceManifestCurrentV1(ctx, request.Surface)
	if err != nil {
		return CompiledModelToolsV1{}, toolcontract.ModelToolInjectionMaterialV1{}, toolcontract.ToolSurfaceManifestCurrentProjectionV1{}, err
	}
	if err = validatePairCurrentClosureV2(compiled, material, surface, request, r.owner, now); err != nil {
		return CompiledModelToolsV1{}, toolcontract.ModelToolInjectionMaterialV1{}, toolcontract.ToolSurfaceManifestCurrentProjectionV1{}, err
	}
	return compiled.Clone(), material.Clone(), clonePairSurfaceProjectionV2(surface), nil
}

func validatePairCurrentClosureV2(
	compiled CompiledModelToolsV1,
	material toolcontract.ModelToolInjectionMaterialV1,
	surface toolcontract.ToolSurfaceManifestCurrentProjectionV1,
	request ModelToolInjectionPairCurrentInspectRequestV2,
	owner core.OwnerRef,
	now time.Time,
) error {
	if err := material.ValidateCurrent(request.Material, now); err != nil {
		return err
	}
	if err := surface.ValidateCurrent(request.Surface, now); err != nil {
		return err
	}
	if err := compiled.ValidateAgainstMaterialV1(material); err != nil {
		return err
	}
	if owner.Validate() != nil ||
		owner != surface.Owner ||
		owner != surface.Manifest.Owner {
		return lineageConflictV1("configured Tool owner and authoritative Surface Owners differ")
	}
	if material.Surface != request.Surface ||
		compiled.Surface != request.Surface ||
		surface.Ref != request.Surface {
		return lineageConflictV1("Material, Compiled Tools, and Surface exact refs differ")
	}
	expected, err := toolcontract.ComputeExpectedInjectionDigest(surface.Manifest.Entries)
	if err != nil ||
		expected != surface.Manifest.ExpectedInjectionDigest ||
		expected != material.ExpectedInjectionDigest {
		return lineageConflictV1("Material and Surface expected injection digests differ")
	}
	if compiled.Digest != material.CompiledToolsDigest {
		return lineageConflictV1("stored Compiled Tools and Material digests differ")
	}
	return nil
}

func validateActualRequestToolsV2(
	tools []modelinvoker.Tool,
	compiled CompiledModelToolsV1,
	material toolcontract.ModelToolInjectionMaterialV1,
	surface toolcontract.ToolSurfaceManifestCurrentProjectionV1,
) (core.Digest, core.Digest, error) {
	actualCompiled, err := ComputeRouteCallCompiledToolsDigestV1(
		surface.Ref,
		surface.Manifest.Dialect,
		tools,
	)
	if err != nil {
		return "", "", err
	}
	if actualCompiled != compiled.Digest ||
		actualCompiled != material.CompiledToolsDigest {
		return "", "", lineageConflictV1("actual Model request Tools differ from stored Compiled Tools and Material")
	}
	requestTools, err := modelinvoker.DigestGovernedModelTurnRequestToolSetV2(tools)
	if err != nil {
		return "", "", err
	}
	return actualCompiled, requestTools, nil
}

func clonePairSurfaceProjectionV2(
	p toolcontract.ToolSurfaceManifestCurrentProjectionV1,
) toolcontract.ToolSurfaceManifestCurrentProjectionV1 {
	p.Manifest.Entries = append([]toolcontract.ToolSurfaceEntry(nil), p.Manifest.Entries...)
	for index := range p.Manifest.Entries {
		p.Manifest.Entries[index].EffectKinds = append(
			[]runtimeports.NamespacedNameV2(nil),
			p.Manifest.Entries[index].EffectKinds...,
		)
	}
	p.Manifest.Residuals = append([]toolcontract.Residual(nil), p.Manifest.Residuals...)
	return p
}
