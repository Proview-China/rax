package surface

import (
	"context"
	"encoding/json"
	"reflect"
	"time"
	"unicode/utf8"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

const CompiledModelToolsContractVersionV1 = "praxis.tool-mcp.compiled-model-tools/v1"

type CompiledModelToolsV1 struct {
	ContractVersion string                                       `json:"contract_version"`
	Surface         toolcontract.ToolSurfaceManifestCurrentRefV1 `json:"surface"`
	Dialect         string                                       `json:"dialect"`
	Tools           []modelinvoker.Tool                          `json:"tools"`
	Digest          core.Digest                                  `json:"digest"`
}

func (v CompiledModelToolsV1) Validate() error {
	if v.ContractVersion != CompiledModelToolsContractVersionV1 || v.Surface.Validate() != nil ||
		v.Dialect != string(toolcontract.ModelToolDialectFunctionCallingV1) ||
		len(v.Tools) == 0 || len(v.Tools) > toolcontract.MaxSurfaceEntries {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "Compiled Model Tools identity, dialect, or tool count is invalid")
	}
	seen := make(map[string]struct{}, len(v.Tools))
	for index, tool := range v.Tools {
		if err := toolcontract.ValidatePortableFunctionToolNameV1(tool.Name); err != nil {
			return err
		}
		if _, exists := seen[tool.Name]; exists {
			return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Compiled Model Tools contain a duplicate Model name")
		}
		seen[tool.Name] = struct{}{}
		if index > 0 && v.Tools[index-1].Name > tool.Name {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "Compiled Model Tools are not canonically ordered")
		}
		if !utf8.ValidString(tool.Description) || len(tool.Description) > toolcontract.MaxStringBytes ||
			tool.Strict == nil || !*tool.Strict {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "Compiled Model Tool description or strict flag is invalid")
		}
		if err := toolcontract.ValidatePortableFunctionToolSchemaV1(tool.Parameters); err != nil {
			return err
		}
	}
	if err := v.Digest.Validate(); err != nil {
		return err
	}
	digest, err := digestCompiledModelToolsV1(v)
	if err != nil || digest != v.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Compiled Model Tools digest drifted")
	}
	return nil
}

func (v CompiledModelToolsV1) ValidateAgainstMaterialV1(material toolcontract.ModelToolInjectionMaterialV1) error {
	if err := v.Validate(); err != nil {
		return err
	}
	if err := material.Validate(); err != nil {
		return err
	}
	if v.Surface != material.Surface || v.Digest != material.CompiledToolsDigest || len(v.Tools) != len(material.Entries) {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Compiled Model Tools do not close their exact Tool Injection Material")
	}
	for index, tool := range v.Tools {
		entry := material.Entries[index]
		if tool.Name != entry.ModelName || tool.Strict == nil || !*tool.Strict ||
			core.DigestBytes([]byte(tool.Description)) != entry.DescriptionDigest ||
			core.DigestBytes(tool.Parameters) != entry.InputSchemaRef.ContentDigest {
			return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Compiled Model Tool bytes drifted from their exact Tool Injection entry")
		}
	}
	return nil
}

func (v CompiledModelToolsV1) Clone() CompiledModelToolsV1 {
	return cloneCompiledModelToolsV1(v)
}

func CompileModelToolsV1(ctx context.Context, current toolcontract.ToolSurfaceManifestCurrentProjectionV1, materials toolcontract.ToolDefinitionMaterialReaderV1, clock func() time.Time) (CompiledModelToolsV1, error) {
	if ctx == nil {
		return CompiledModelToolsV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "Model Tool compile context is required")
	}
	if err := ctx.Err(); err != nil {
		return CompiledModelToolsV1{}, err
	}
	if readerUnavailableV1(materials) {
		return CompiledModelToolsV1{}, toolDefinitionMaterialUnavailableV1("Tool Definition Material Reader is nil or typed-nil")
	}
	if clock == nil {
		return CompiledModelToolsV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonClockRegression, "Model Tool compile clock is required")
	}
	now := clock()
	if now.IsZero() {
		return CompiledModelToolsV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonClockRegression, "Model Tool compile time is required")
	}
	if err := current.ValidateCurrent(current.Ref, now); err != nil {
		return CompiledModelToolsV1{}, err
	}
	if current.Manifest.Dialect != toolcontract.ModelToolDialectFunctionCallingV1 {
		return CompiledModelToolsV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonUnknownCapability, "Tool Surface Dialect is not supported by neutral function-tool assembly")
	}
	result := CompiledModelToolsV1{
		ContractVersion: CompiledModelToolsContractVersionV1,
		Surface:         current.Ref, Dialect: string(current.Manifest.Dialect),
		Tools: make([]modelinvoker.Tool, 0, len(current.Manifest.Entries)),
	}
	for _, entry := range current.Manifest.Entries {
		if entry.Visibility != toolcontract.SurfaceVisible || !entry.Allowed {
			return CompiledModelToolsV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonPlanInvalid, "neutral Model Tool assembly requires every Surface entry to be visible and allowed")
		}
		if err := toolcontract.ValidatePortableFunctionToolNameV1(entry.ModelName); err != nil {
			return CompiledModelToolsV1{}, err
		}
		ref, err := toolcontract.DeriveToolDefinitionMaterialRefV1(entry.Tool, entry.InputSchema, entry.DescriptionDigest)
		if err != nil {
			return CompiledModelToolsV1{}, err
		}
		material, err := materials.InspectExactToolDefinitionMaterialV1(ctx, ref)
		if err != nil {
			return CompiledModelToolsV1{}, err
		}
		if err := material.Validate(); err != nil || material.Ref != ref {
			return CompiledModelToolsV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Tool Definition Material does not bind its Surface entry")
		}
		if err := toolcontract.ValidatePortableFunctionToolSchemaV1(material.InputSchema); err != nil {
			return CompiledModelToolsV1{}, err
		}
		strict := true
		result.Tools = append(result.Tools, modelinvoker.Tool{
			Name: entry.ModelName, Description: material.Description,
			Parameters: append(json.RawMessage(nil), material.InputSchema...), Strict: &strict,
		})
	}
	if err := ctx.Err(); err != nil {
		return CompiledModelToolsV1{}, err
	}
	final := clock()
	if final.IsZero() || final.Before(now) {
		return CompiledModelToolsV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Model Tool compile clock regressed")
	}
	if err := current.ValidateCurrent(current.Ref, final); err != nil {
		return CompiledModelToolsV1{}, err
	}
	digest, err := digestCompiledModelToolsV1(result)
	if err != nil {
		return CompiledModelToolsV1{}, err
	}
	result.Digest = digest
	return cloneCompiledModelToolsV1(result), nil
}

func digestCompiledModelToolsV1(value CompiledModelToolsV1) (core.Digest, error) {
	type canonicalToolV1 struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Parameters  json.RawMessage `json:"parameters"`
		Strict      bool            `json:"strict"`
	}
	body := struct {
		ContractVersion string                                       `json:"contract_version"`
		Surface         toolcontract.ToolSurfaceManifestCurrentRefV1 `json:"surface"`
		Dialect         string                                       `json:"dialect"`
		Tools           []canonicalToolV1                            `json:"tools"`
	}{ContractVersion: value.ContractVersion, Surface: value.Surface, Dialect: value.Dialect}
	body.Tools = make([]canonicalToolV1, 0, len(value.Tools))
	for _, tool := range value.Tools {
		strict := tool.Strict != nil && *tool.Strict
		body.Tools = append(body.Tools, canonicalToolV1{Name: tool.Name, Description: tool.Description, Parameters: tool.Parameters, Strict: strict})
	}
	return core.CanonicalJSONDigest("praxis.tool-mcp.compiled-model-tools", CompiledModelToolsContractVersionV1, "CompiledModelToolsV1", body)
}

func cloneCompiledModelToolsV1(value CompiledModelToolsV1) CompiledModelToolsV1 {
	value.Tools = append([]modelinvoker.Tool(nil), value.Tools...)
	for i := range value.Tools {
		value.Tools[i].Parameters = append(json.RawMessage(nil), value.Tools[i].Parameters...)
		if value.Tools[i].Strict != nil {
			strict := *value.Tools[i].Strict
			value.Tools[i].Strict = &strict
		}
	}
	return value
}

func readerUnavailableV1(reader toolcontract.ToolDefinitionMaterialReaderV1) bool {
	if reader == nil {
		return true
	}
	value := reflect.ValueOf(reader)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
