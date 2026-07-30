package surface

import (
	"context"
	"encoding/json"
	"reflect"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

const ModelToolInjectionLineageContractVersionV1 = "praxis.tool-mcp.model-tool-injection-lineage/v1"

const modelToolInjectionLineageCanonicalDomainV1 = "praxis.tool-mcp.model-tool-injection-lineage"

type ModelToolInjectionLineageInspectRequestV1 struct {
	Material  toolcontract.ModelToolInjectionMaterialRefV1 `json:"material"`
	RouteCall modelinvoker.RouteCall                       `json:"route_call"`
}

func (r ModelToolInjectionLineageInspectRequestV1) Validate() error {
	if err := r.Material.Validate(); err != nil {
		return err
	}
	_, err := modelinvoker.DigestGovernedModelTurnRouteCallV2(r.RouteCall)
	return err
}

type ModelToolInjectionLineageCurrentProjectionV1 struct {
	ContractVersion string `json:"contract_version"`

	Material       toolcontract.ModelToolInjectionMaterialV1         `json:"material"`
	MaterialSource toolcontract.ModelToolInjectionLineageSourceRefV1 `json:"material_source"`
	MaterialOwner  core.OwnerRef                                     `json:"material_owner"`

	Surface       toolcontract.ToolSurfaceManifestCurrentProjectionV1 `json:"surface"`
	SurfaceSource toolcontract.ModelToolInjectionLineageSourceRefV1   `json:"surface_source"`

	CompiledTools             CompiledModelToolsV1 `json:"compiled_tools"`
	ExpectedInjectionDigest   core.Digest          `json:"expected_injection_digest"`
	ActualCompiledToolsDigest core.Digest          `json:"actual_compiled_tools_digest"`
	RouteCallDigest           core.Digest          `json:"route_call_digest"`

	CheckedUnixNano  int64       `json:"checked_unix_nano"`
	ExpiresUnixNano  int64       `json:"expires_unix_nano"`
	ProjectionDigest core.Digest `json:"projection_digest"`
}

func (p ModelToolInjectionLineageCurrentProjectionV1) Validate() error {
	if p.ContractVersion != ModelToolInjectionLineageContractVersionV1 {
		return lineageInvalidV1("Model Tool Injection lineage contract version is invalid")
	}
	if err := p.Material.Validate(); err != nil {
		return err
	}
	if err := p.MaterialOwner.Validate(); err != nil {
		return err
	}
	if err := p.MaterialSource.Validate(); err != nil {
		return err
	}
	if err := p.Surface.Validate(); err != nil {
		return err
	}
	if err := p.SurfaceSource.Validate(); err != nil {
		return err
	}
	if err := p.CompiledTools.ValidateAgainstMaterialV1(p.Material); err != nil {
		return err
	}
	expectedMaterialSource, err := toolcontract.ModelToolInjectionMaterialSourceRefV1(p.MaterialOwner, p.Material.Ref)
	if err != nil || p.MaterialSource != expectedMaterialSource {
		return lineageConflictV1("Model Tool Injection Material source closure drifted")
	}
	expectedSurfaceSource, err := toolcontract.ToolSurfaceManifestSourceRefV1(p.Surface)
	if err != nil || p.SurfaceSource != expectedSurfaceSource {
		return lineageConflictV1("Tool Surface current source closure drifted")
	}
	if p.MaterialOwner != p.MaterialSource.Owner ||
		p.MaterialOwner != p.SurfaceSource.Owner ||
		p.MaterialOwner != p.Surface.Owner ||
		p.MaterialOwner != p.Surface.Manifest.Owner {
		return lineageConflictV1("Model Tool Injection Material and Tool Surface authoritative Owners drifted")
	}
	if p.Material.Surface != p.Surface.Ref ||
		p.CompiledTools.Surface != p.Surface.Ref {
		return lineageConflictV1("Material, Surface current, and Compiled Tools exact refs drifted")
	}
	expected, err := toolcontract.ComputeExpectedInjectionDigest(p.Surface.Manifest.Entries)
	if err != nil ||
		expected != p.Surface.Manifest.ExpectedInjectionDigest ||
		expected != p.Material.ExpectedInjectionDigest ||
		expected != p.ExpectedInjectionDigest {
		return lineageConflictV1("expected Tool Surface injection lineage drifted")
	}
	if p.Material.CompiledToolsDigest != p.CompiledTools.Digest ||
		p.Material.CompiledToolsDigest != p.ActualCompiledToolsDigest {
		return lineageConflictV1("actual RouteCall Tool compilation lineage drifted")
	}
	for _, digest := range []core.Digest{
		p.ExpectedInjectionDigest, p.ActualCompiledToolsDigest,
		p.RouteCallDigest, p.ProjectionDigest,
	} {
		if err := digest.Validate(); err != nil {
			return err
		}
	}
	if p.CheckedUnixNano <= 0 ||
		p.CheckedUnixNano < p.Material.CreatedUnixNano ||
		p.CheckedUnixNano < p.Surface.CheckedUnixNano ||
		p.ExpiresUnixNano <= p.CheckedUnixNano ||
		p.ExpiresUnixNano != minLineageExpiryV1(p.Material.ExpiresUnixNano, p.Surface.ExpiresUnixNano) {
		return lineageInvalidV1("Model Tool Injection lineage current window is invalid")
	}
	digest, err := p.ComputeDigest()
	if err != nil || digest != p.ProjectionDigest {
		return lineageConflictV1("Model Tool Injection lineage Projection digest drifted")
	}
	return nil
}

func (p ModelToolInjectionLineageCurrentProjectionV1) ValidateCurrent(
	expected toolcontract.ModelToolInjectionMaterialRefV1,
	now time.Time,
) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if expected.Validate() != nil || p.Material.Ref != expected {
		return lineageConflictV1("Model Tool Injection lineage exact Material Ref drifted")
	}
	if now.IsZero() || now.UnixNano() < p.CheckedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Model Tool Injection lineage clock regressed")
	}
	if !now.Before(time.Unix(0, p.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "Model Tool Injection lineage expired")
	}
	return nil
}

func (p ModelToolInjectionLineageCurrentProjectionV1) ValidateAgainst(
	request ModelToolInjectionLineageInspectRequestV1,
	now time.Time,
) error {
	request.RouteCall = freezeRouteCallV1(request.RouteCall)
	return p.validateAgainstFrozenV1(request, now)
}

func (p ModelToolInjectionLineageCurrentProjectionV1) validateAgainstFrozenV1(
	request ModelToolInjectionLineageInspectRequestV1,
	now time.Time,
) error {
	if err := request.Validate(); err != nil {
		return err
	}
	if err := p.ValidateCurrent(request.Material, now); err != nil {
		return err
	}
	routeCallDigest, err := modelinvoker.DigestGovernedModelTurnRouteCallV2(request.RouteCall)
	if err != nil {
		return err
	}
	actualCompiled, err := ComputeRouteCallCompiledToolsDigestV1(
		p.Surface.Ref, p.Surface.Manifest.Dialect, request.RouteCall.Request.Tools,
	)
	if err != nil {
		return err
	}
	if p.RouteCallDigest != routeCallDigest ||
		p.ActualCompiledToolsDigest != actualCompiled {
		return lineageConflictV1("Model Tool Injection lineage does not match the actual RouteCall")
	}
	return nil
}

func (p ModelToolInjectionLineageCurrentProjectionV1) ComputeDigest() (core.Digest, error) {
	p = p.Clone()
	p.ProjectionDigest = ""
	return core.CanonicalJSONDigest(
		modelToolInjectionLineageCanonicalDomainV1,
		ModelToolInjectionLineageContractVersionV1,
		"ModelToolInjectionLineageCurrentProjectionV1",
		p,
	)
}

func (p ModelToolInjectionLineageCurrentProjectionV1) Clone() ModelToolInjectionLineageCurrentProjectionV1 {
	p.Material = p.Material.Clone()
	p.Surface.Manifest.Entries = append([]toolcontract.ToolSurfaceEntry(nil), p.Surface.Manifest.Entries...)
	for index := range p.Surface.Manifest.Entries {
		p.Surface.Manifest.Entries[index].EffectKinds = append(
			[]runtimeports.NamespacedNameV2(nil),
			p.Surface.Manifest.Entries[index].EffectKinds...,
		)
	}
	p.Surface.Manifest.Residuals = append([]toolcontract.Residual(nil), p.Surface.Manifest.Residuals...)
	p.CompiledTools = p.CompiledTools.Clone()
	return p
}

type ModelToolInjectionExactClosureReaderV1 interface {
	InspectExactModelToolInjectionClosureV1(
		context.Context,
		toolcontract.ModelToolInjectionMaterialRefV1,
	) (CompiledModelToolsV1, toolcontract.ModelToolInjectionMaterialV1, error)
}

type ModelToolInjectionLineageCurrentReaderV1 interface {
	InspectCurrentModelToolInjectionLineageV1(
		context.Context,
		ModelToolInjectionLineageInspectRequestV1,
	) (ModelToolInjectionLineageCurrentProjectionV1, error)
}

type ModelToolInjectionLineageReaderV1 struct {
	closures ModelToolInjectionExactClosureReaderV1
	surfaces toolcontract.ToolSurfaceManifestCurrentReaderV1
	owner    core.OwnerRef
	clock    func() time.Time
}

func NewModelToolInjectionLineageReaderV1(
	closures ModelToolInjectionExactClosureReaderV1,
	surfaces toolcontract.ToolSurfaceManifestCurrentReaderV1,
	owner core.OwnerRef,
	clock func() time.Time,
) (*ModelToolInjectionLineageReaderV1, error) {
	if lineageNilV1(closures) || lineageNilV1(surfaces) || lineageNilV1(clock) || owner.Validate() != nil {
		return nil, lineageInvalidV1("Model Tool Injection lineage Reader dependencies are incomplete")
	}
	return &ModelToolInjectionLineageReaderV1{
		closures: closures, surfaces: surfaces, owner: owner, clock: clock,
	}, nil
}

func (r *ModelToolInjectionLineageReaderV1) InspectCurrentModelToolInjectionLineageV1(
	ctx context.Context,
	request ModelToolInjectionLineageInspectRequestV1,
) (ModelToolInjectionLineageCurrentProjectionV1, error) {
	if ctx == nil {
		return ModelToolInjectionLineageCurrentProjectionV1{}, lineageInvalidV1("Model Tool Injection lineage context is required")
	}
	if err := ctx.Err(); err != nil {
		return ModelToolInjectionLineageCurrentProjectionV1{}, err
	}
	if r == nil || lineageNilV1(r.closures) || lineageNilV1(r.surfaces) ||
		lineageNilV1(r.clock) || r.owner.Validate() != nil {
		return ModelToolInjectionLineageCurrentProjectionV1{}, lineageUnavailableV1("Model Tool Injection lineage Reader is unavailable")
	}
	request.RouteCall = freezeRouteCallV1(request.RouteCall)
	if err := request.Validate(); err != nil {
		return ModelToolInjectionLineageCurrentProjectionV1{}, err
	}
	s1 := r.clock()
	if s1.IsZero() {
		return ModelToolInjectionLineageCurrentProjectionV1{}, lineageInvalidV1("Model Tool Injection lineage S1 clock is required")
	}
	compiledS1, materialS1, err := r.closures.InspectExactModelToolInjectionClosureV1(ctx, request.Material)
	if err != nil {
		return ModelToolInjectionLineageCurrentProjectionV1{}, err
	}
	if err = validateLineageClosureV1(compiledS1, materialS1, request.Material, s1); err != nil {
		return ModelToolInjectionLineageCurrentProjectionV1{}, err
	}
	surfaceS1, err := r.surfaces.InspectExactToolSurfaceManifestCurrentV1(ctx, materialS1.Surface)
	if err != nil {
		return ModelToolInjectionLineageCurrentProjectionV1{}, err
	}
	if err = surfaceS1.ValidateCurrent(materialS1.Surface, s1); err != nil {
		return ModelToolInjectionLineageCurrentProjectionV1{}, err
	}
	if err = validateLineageRelationsV1(compiledS1, materialS1, surfaceS1, r.owner); err != nil {
		return ModelToolInjectionLineageCurrentProjectionV1{}, err
	}
	actualCompiled, err := ComputeRouteCallCompiledToolsDigestV1(
		surfaceS1.Ref, surfaceS1.Manifest.Dialect, request.RouteCall.Request.Tools,
	)
	if err != nil {
		return ModelToolInjectionLineageCurrentProjectionV1{}, err
	}
	if actualCompiled != materialS1.CompiledToolsDigest {
		return ModelToolInjectionLineageCurrentProjectionV1{}, lineageConflictV1("actual RouteCall Tools differ from the exact compiled Tool closure")
	}
	routeCallDigest, err := modelinvoker.DigestGovernedModelTurnRouteCallV2(request.RouteCall)
	if err != nil {
		return ModelToolInjectionLineageCurrentProjectionV1{}, err
	}

	compiledS2, materialS2, err := r.closures.InspectExactModelToolInjectionClosureV1(ctx, request.Material)
	if err != nil {
		return ModelToolInjectionLineageCurrentProjectionV1{}, err
	}
	surfaceS2, err := r.surfaces.InspectExactToolSurfaceManifestCurrentV1(ctx, materialS1.Surface)
	if err != nil {
		return ModelToolInjectionLineageCurrentProjectionV1{}, err
	}
	s2 := r.clock()
	if s2.IsZero() || s2.Before(s1) {
		return ModelToolInjectionLineageCurrentProjectionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Model Tool Injection lineage S2 clock regressed")
	}
	if !reflect.DeepEqual(compiledS1, compiledS2) ||
		!reflect.DeepEqual(materialS1, materialS2) ||
		!reflect.DeepEqual(surfaceS1, surfaceS2) {
		return ModelToolInjectionLineageCurrentProjectionV1{}, lineageConflictV1("Model Tool Injection lineage drifted between S1 and S2")
	}
	if err = validateLineageClosureV1(compiledS2, materialS2, request.Material, s2); err != nil {
		return ModelToolInjectionLineageCurrentProjectionV1{}, err
	}
	if err = surfaceS2.ValidateCurrent(materialS2.Surface, s2); err != nil {
		return ModelToolInjectionLineageCurrentProjectionV1{}, err
	}
	if err = validateLineageRelationsV1(compiledS2, materialS2, surfaceS2, r.owner); err != nil {
		return ModelToolInjectionLineageCurrentProjectionV1{}, err
	}
	if err = ctx.Err(); err != nil {
		return ModelToolInjectionLineageCurrentProjectionV1{}, err
	}
	final := r.clock()
	expires := minLineageExpiryV1(materialS2.ExpiresUnixNano, surfaceS2.ExpiresUnixNano)
	if final.IsZero() || final.Before(s2) {
		return ModelToolInjectionLineageCurrentProjectionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Model Tool Injection lineage final clock regressed")
	}
	if !final.Before(time.Unix(0, expires)) {
		return ModelToolInjectionLineageCurrentProjectionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "Model Tool Injection lineage expired before Seal")
	}
	materialSource, err := toolcontract.ModelToolInjectionMaterialSourceRefV1(r.owner, materialS2.Ref)
	if err != nil {
		return ModelToolInjectionLineageCurrentProjectionV1{}, err
	}
	surfaceSource, err := toolcontract.ToolSurfaceManifestSourceRefV1(surfaceS2)
	if err != nil {
		return ModelToolInjectionLineageCurrentProjectionV1{}, err
	}
	projection := ModelToolInjectionLineageCurrentProjectionV1{
		ContractVersion: ModelToolInjectionLineageContractVersionV1,
		Material:        materialS2.Clone(), MaterialSource: materialSource, MaterialOwner: r.owner,
		Surface: surfaceS2, SurfaceSource: surfaceSource,
		CompiledTools:             compiledS2.Clone(),
		ExpectedInjectionDigest:   materialS2.ExpectedInjectionDigest,
		ActualCompiledToolsDigest: actualCompiled,
		RouteCallDigest:           routeCallDigest,
		CheckedUnixNano:           final.UnixNano(), ExpiresUnixNano: expires,
	}
	projection.ProjectionDigest, err = projection.ComputeDigest()
	if err != nil {
		return ModelToolInjectionLineageCurrentProjectionV1{}, err
	}
	if err = projection.validateAgainstFrozenV1(request, final); err != nil {
		return ModelToolInjectionLineageCurrentProjectionV1{}, err
	}
	return projection.Clone(), nil
}

func ComputeRouteCallCompiledToolsDigestV1(
	surface toolcontract.ToolSurfaceManifestCurrentRefV1,
	dialect runtimeports.NamespacedNameV2,
	tools []modelinvoker.Tool,
) (core.Digest, error) {
	value := CompiledModelToolsV1{
		ContractVersion: CompiledModelToolsContractVersionV1,
		Surface:         surface, Dialect: string(dialect),
		Tools: cloneRouteCallToolsV1(tools),
	}
	digest, err := digestCompiledModelToolsV1(value)
	if err != nil {
		return "", err
	}
	value.Digest = digest
	if err = value.Validate(); err != nil {
		return "", err
	}
	return digest, nil
}

func validateLineageClosureV1(
	compiled CompiledModelToolsV1,
	material toolcontract.ModelToolInjectionMaterialV1,
	expected toolcontract.ModelToolInjectionMaterialRefV1,
	now time.Time,
) error {
	if err := material.ValidateCurrent(expected, now); err != nil {
		return err
	}
	return compiled.ValidateAgainstMaterialV1(material)
}

func validateLineageRelationsV1(
	compiled CompiledModelToolsV1,
	material toolcontract.ModelToolInjectionMaterialV1,
	surface toolcontract.ToolSurfaceManifestCurrentProjectionV1,
	owner core.OwnerRef,
) error {
	if owner.Validate() != nil ||
		owner != surface.Owner ||
		owner != surface.Manifest.Owner {
		return lineageConflictV1("configured Tool owner and Tool Surface authoritative Owners differ")
	}
	if material.Surface != surface.Ref || compiled.Surface != surface.Ref {
		return lineageConflictV1("Material, Surface, and Compiled Tools exact refs differ")
	}
	expected, err := toolcontract.ComputeExpectedInjectionDigest(surface.Manifest.Entries)
	if err != nil ||
		expected != surface.Manifest.ExpectedInjectionDigest ||
		expected != material.ExpectedInjectionDigest {
		return lineageConflictV1("Material and Surface expected injection digests differ")
	}
	if compiled.Digest != material.CompiledToolsDigest {
		return lineageConflictV1("Material and stored Compiled Tools digests differ")
	}
	return nil
}

func cloneRouteCallToolsV1(tools []modelinvoker.Tool) []modelinvoker.Tool {
	result := append([]modelinvoker.Tool(nil), tools...)
	for index := range result {
		result[index].Parameters = append(json.RawMessage(nil), result[index].Parameters...)
		if result[index].Strict != nil {
			strict := *result[index].Strict
			result[index].Strict = &strict
		}
	}
	return result
}

func freezeRouteCallV1(call modelinvoker.RouteCall) modelinvoker.RouteCall {
	frozen := call
	if call.EntitlementState != nil {
		entitlement := *call.EntitlementState
		if call.EntitlementState.RemainingQuota != nil {
			remaining := *call.EntitlementState.RemainingQuota
			entitlement.RemainingQuota = &remaining
		}
		frozen.EntitlementState = &entitlement
	}
	frozen.Request.Input = make([]modelinvoker.InputItem, len(call.Request.Input))
	for index, item := range call.Request.Input {
		frozen.Request.Input[index] = item
		if item.Message != nil {
			message := *item.Message
			frozen.Request.Input[index].Message = &message
		}
		if item.FunctionCall != nil {
			functionCall := *item.FunctionCall
			functionCall.Arguments = append(json.RawMessage(nil), item.FunctionCall.Arguments...)
			frozen.Request.Input[index].FunctionCall = &functionCall
		}
		if item.FunctionResult != nil {
			functionResult := *item.FunctionResult
			frozen.Request.Input[index].FunctionResult = &functionResult
		}
	}
	frozen.Request.Instructions = append([]modelinvoker.Instruction(nil), call.Request.Instructions...)
	frozen.Request.Tools = cloneRouteCallToolsV1(call.Request.Tools)
	if call.Request.ParallelToolCalls != nil {
		parallel := *call.Request.ParallelToolCalls
		frozen.Request.ParallelToolCalls = &parallel
	}
	frozen.Request.Output.Schema = append(json.RawMessage(nil), call.Request.Output.Schema...)
	if call.Request.Output.Strict != nil {
		strict := *call.Request.Output.Strict
		frozen.Request.Output.Strict = &strict
	}
	if call.Request.Reasoning != nil {
		reasoning := *call.Request.Reasoning
		if call.Request.Reasoning.BudgetTokens != nil {
			budget := *call.Request.Reasoning.BudgetTokens
			reasoning.BudgetTokens = &budget
		}
		frozen.Request.Reasoning = &reasoning
	}
	if call.Request.State != nil {
		state := *call.Request.State
		state.Payload = modelinvoker.NewRawPayload(call.Request.State.Payload.Bytes())
		frozen.Request.State = &state
	}
	if call.Request.Metadata != nil {
		frozen.Request.Metadata = make(modelinvoker.Metadata, len(call.Request.Metadata))
		for key, value := range call.Request.Metadata {
			frozen.Request.Metadata[key] = value
		}
	}
	if call.Request.ProviderOptions != nil {
		frozen.Request.ProviderOptions = make(modelinvoker.ProviderOptions, len(call.Request.ProviderOptions))
		for provider, value := range call.Request.ProviderOptions {
			frozen.Request.ProviderOptions[provider] = append(json.RawMessage(nil), value...)
		}
	}
	return frozen
}

func minLineageExpiryV1(left, right int64) int64 {
	if left < right {
		return left
	}
	return right
}

func lineageNilV1(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

func lineageInvalidV1(message string) error {
	return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, message)
}

func lineageConflictV1(message string) error {
	return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, message)
}

func lineageUnavailableV1(message string) error {
	return core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, message)
}

var _ ModelToolInjectionLineageCurrentReaderV1 = (*ModelToolInjectionLineageReaderV1)(nil)
