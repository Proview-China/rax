package builder

import (
	"strings"

	definitioncontract "github.com/Proview-China/rax/ExecutionRuntime/agent-definition/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-definition/decoder"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type DefinitionFormatV1 string

const (
	DefinitionFormatYAMLV1 DefinitionFormatV1 = "yaml"
	DefinitionFormatJSONV1 DefinitionFormatV1 = "json"
)

func DecodeDefinitionSourceV1(format DefinitionFormatV1, payload []byte, catalog definitioncontract.ValidationCatalogV1) (definitioncontract.AgentDefinitionSourceV1, error) {
	switch format {
	case DefinitionFormatYAMLV1:
		return decoder.DecodeYAMLV1(payload, definitioncontract.CloneValidationCatalogV1(catalog))
	case DefinitionFormatJSONV1:
		return decoder.DecodeJSONV1(payload, definitioncontract.CloneValidationCatalogV1(catalog))
	default:
		return definitioncontract.AgentDefinitionSourceV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "definition input format must be yaml or json")
	}
}

type DefinitionSourceBuilderV1 struct {
	source definitioncontract.AgentDefinitionSourceV1
}

func NewDefinitionSourceBuilderV1(source definitioncontract.AgentDefinitionSourceV1) *DefinitionSourceBuilderV1 {
	return &DefinitionSourceBuilderV1{source: definitioncontract.CloneSourceV1(source)}
}

func (b *DefinitionSourceBuilderV1) AddComponent(value definitioncontract.ComponentRequirementV1) *DefinitionSourceBuilderV1 {
	if b != nil {
		b.source.Components = append(b.source.Components, value)
	}
	return b
}

func (b *DefinitionSourceBuilderV1) AddSecretRef(value definitioncontract.SecretRefV1) *DefinitionSourceBuilderV1 {
	if b != nil {
		b.source.SecretRefs = append(b.source.SecretRefs, value)
	}
	return b
}

func (b *DefinitionSourceBuilderV1) AddExtension(value definitioncontract.ExtensionV1) *DefinitionSourceBuilderV1 {
	if b != nil {
		b.source.Extensions = append(b.source.Extensions, value)
	}
	return b
}

func (b *DefinitionSourceBuilderV1) Build(catalog definitioncontract.ValidationCatalogV1) (definitioncontract.AgentDefinitionSourceV1, error) {
	if b == nil {
		return definitioncontract.AgentDefinitionSourceV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "definition source builder is nil")
	}
	value := definitioncontract.NormalizeSourceV1(definitioncontract.CloneSourceV1(b.source))
	if strings.TrimSpace(value.ContractVersion) == "" {
		value.ContractVersion = definitioncontract.ContractVersionV1
	}
	if err := definitioncontract.ValidateSourceV1(value, definitioncontract.CloneValidationCatalogV1(catalog)); err != nil {
		return definitioncontract.AgentDefinitionSourceV1{}, err
	}
	return definitioncontract.CloneSourceV1(value), nil
}
