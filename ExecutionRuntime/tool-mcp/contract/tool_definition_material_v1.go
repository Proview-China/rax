package contract

import (
	"context"
	"encoding/json"
	"strconv"
	"unicode/utf8"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const ToolDefinitionMaterialContractVersionV1 = "praxis.tool-mcp.tool-definition-material/v1"

const ModelToolDialectFunctionCallingV1 runtimeports.NamespacedNameV2 = "praxis.model/function-calling-v1"

const toolDefinitionMaterialCanonicalDomainV1 = "praxis.tool-mcp.tool-definition-material"

const (
	MaxToolDefinitionSchemaDepthV1 = 32
	MaxToolDefinitionSchemaNodesV1 = 4096
)

type ToolDefinitionMaterialRefV1 struct {
	ContractVersion   string                   `json:"contract_version"`
	ID                string                   `json:"id"`
	Revision          core.Revision            `json:"revision"`
	Digest            core.Digest              `json:"digest"`
	Tool              ObjectRef                `json:"tool"`
	InputSchema       runtimeports.SchemaRefV2 `json:"input_schema"`
	DescriptionDigest core.Digest              `json:"description_digest"`
}

func DeriveToolDefinitionMaterialRefV1(tool ObjectRef, schema runtimeports.SchemaRefV2, description core.Digest) (ToolDefinitionMaterialRefV1, error) {
	if tool.Validate() != nil || schema.Validate() != nil || description.Validate() != nil {
		return ToolDefinitionMaterialRefV1{}, invalid("Tool Definition Material source coordinates are invalid")
	}
	id, err := StableID("tool-material-v1", tool.ID, strconv.FormatUint(uint64(tool.Revision), 10), string(tool.Digest), schema.Key(), string(description))
	if err != nil {
		return ToolDefinitionMaterialRefV1{}, err
	}
	ref := ToolDefinitionMaterialRefV1{
		ContractVersion: ToolDefinitionMaterialContractVersionV1,
		ID:              id, Revision: 1, Tool: tool, InputSchema: schema, DescriptionDigest: description,
	}
	digest, err := digestToolDefinitionMaterialRefV1(ref)
	if err != nil {
		return ToolDefinitionMaterialRefV1{}, err
	}
	ref.Digest = digest
	return ref, nil
}

func (r ToolDefinitionMaterialRefV1) Validate() error {
	if r.ContractVersion != ToolDefinitionMaterialContractVersionV1 || ValidateStableID(r.ID) != nil || r.Revision != 1 || r.Tool.Validate() != nil || r.InputSchema.Validate() != nil || r.DescriptionDigest.Validate() != nil || r.Digest.Validate() != nil {
		return invalid("Tool Definition Material Ref is incomplete")
	}
	expected, err := DeriveToolDefinitionMaterialRefV1(r.Tool, r.InputSchema, r.DescriptionDigest)
	if err != nil {
		return err
	}
	if r != expected {
		return conflict("Tool Definition Material Ref does not bind exact source coordinates")
	}
	return nil
}

type ToolDefinitionMaterialV1 struct {
	Ref         ToolDefinitionMaterialRefV1 `json:"ref"`
	Description string                      `json:"description"`
	InputSchema json.RawMessage             `json:"input_schema"`
}

func (m ToolDefinitionMaterialV1) Validate() error {
	if err := m.Ref.Validate(); err != nil {
		return err
	}
	if len(m.Description) > MaxStringBytes || !utf8.ValidString(m.Description) || core.DigestBytes([]byte(m.Description)) != m.Ref.DescriptionDigest {
		return conflict("Tool Definition Material description differs from its exact digest")
	}
	if len(m.InputSchema) == 0 || len(m.InputSchema) > runtimeports.MaxOpaqueInlineBytes || core.DigestBytes(m.InputSchema) != m.Ref.InputSchema.ContentDigest {
		return conflict("Tool Definition Material schema differs from its exact Schema Ref")
	}
	var schema any
	if err := core.DecodeStrictJSON(m.InputSchema, &schema); err != nil {
		return err
	}
	root, ok := schema.(map[string]any)
	if !ok || root["type"] != "object" {
		return invalid("Tool Definition Material input schema must be one JSON object schema")
	}
	nodes := 0
	return validateToolDefinitionSchemaValueV1(schema, 0, &nodes)
}

func (m ToolDefinitionMaterialV1) Clone() ToolDefinitionMaterialV1 {
	m.InputSchema = append(json.RawMessage(nil), m.InputSchema...)
	return m
}

type ToolDefinitionMaterialReaderV1 interface {
	InspectExactToolDefinitionMaterialV1(context.Context, ToolDefinitionMaterialRefV1) (ToolDefinitionMaterialV1, error)
}

type ToolDefinitionMaterialRepositoryV1 interface {
	ToolDefinitionMaterialReaderV1
	EnsureExactToolDefinitionMaterialV1(context.Context, ToolDefinitionMaterialV1) (ToolDefinitionMaterialV1, error)
}

func digestToolDefinitionMaterialRefV1(ref ToolDefinitionMaterialRefV1) (core.Digest, error) {
	ref.Digest = ""
	return core.CanonicalJSONDigest(toolDefinitionMaterialCanonicalDomainV1, ToolDefinitionMaterialContractVersionV1, "ToolDefinitionMaterialRefV1", ref)
}

func validateToolDefinitionSchemaValueV1(value any, depth int, nodes *int) error {
	*nodes++
	if depth > MaxToolDefinitionSchemaDepthV1 || *nodes > MaxToolDefinitionSchemaNodesV1 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "Tool Definition Material schema depth or node count exceeds limit")
	}
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if len(key) > 256 {
				return core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "Tool Definition Material schema key exceeds limit")
			}
			if err := validateToolDefinitionSchemaValueV1(child, depth+1, nodes); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range typed {
			if err := validateToolDefinitionSchemaValueV1(child, depth+1, nodes); err != nil {
				return err
			}
		}
	case string:
		if len(typed) > 64<<10 {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "Tool Definition Material schema string exceeds limit")
		}
	}
	return nil
}
