package contract

import (
	"bytes"
	"encoding/json"
	"sort"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func CloneSourceV1(source AgentDefinitionSourceV1) AgentDefinitionSourceV1 {
	clone := source
	clone.Components = append([]ComponentRequirementV1(nil), source.Components...)
	for i := range clone.Components {
		clone.Components[i].RequiredCapabilities = append([]string(nil), source.Components[i].RequiredCapabilities...)
		clone.Components[i].DependencyIDs = append([]string(nil), source.Components[i].DependencyIDs...)
	}
	clone.SecretRefs = append([]SecretRefV1(nil), source.SecretRefs...)
	clone.Extensions = append([]ExtensionV1(nil), source.Extensions...)
	for i := range clone.Extensions {
		clone.Extensions[i].Payload = append(json.RawMessage(nil), source.Extensions[i].Payload...)
	}
	return clone
}

func CloneDefinitionV1(definition AgentDefinitionV1) AgentDefinitionV1 {
	clone := definition
	clone.AgentDefinitionSourceV1 = CloneSourceV1(definition.AgentDefinitionSourceV1)
	return clone
}

func CloneValidationCatalogV1(catalog ValidationCatalogV1) ValidationCatalogV1 {
	return ValidationCatalogV1{
		Kinds:                   append([]string(nil), catalog.Kinds...),
		Capabilities:            append([]string(nil), catalog.Capabilities...),
		RegisteredExtensionKeys: append([]string(nil), catalog.RegisteredExtensionKeys...),
	}
}

func NormalizeSourceV1(source AgentDefinitionSourceV1) AgentDefinitionSourceV1 {
	result := CloneSourceV1(source)
	for i := range result.Components {
		result.Components[i].RequiredCapabilities = normalizedStrings(result.Components[i].RequiredCapabilities)
		result.Components[i].DependencyIDs = normalizedStrings(result.Components[i].DependencyIDs)
	}
	sort.Slice(result.Components, func(i, j int) bool { return result.Components[i].ComponentID < result.Components[j].ComponentID })
	if result.Components == nil {
		result.Components = []ComponentRequirementV1{}
	}
	sort.Slice(result.SecretRefs, func(i, j int) bool { return result.SecretRefs[i].SecretID < result.SecretRefs[j].SecretID })
	if result.SecretRefs == nil {
		result.SecretRefs = []SecretRefV1{}
	}
	sort.Slice(result.Extensions, func(i, j int) bool { return result.Extensions[i].Key < result.Extensions[j].Key })
	if result.Extensions == nil {
		result.Extensions = []ExtensionV1{}
	}
	return result
}

func normalizeSourceForDigestV1(source AgentDefinitionSourceV1) AgentDefinitionSourceV1 {
	result := NormalizeSourceV1(source)
	for i := range result.Extensions {
		result.Extensions[i].Payload = canonicalRawJSON(result.Extensions[i].Payload)
	}
	return result
}

func normalizedStrings(values []string) []string {
	result := append([]string(nil), values...)
	sort.Strings(result)
	if result == nil {
		result = []string{}
	}
	return result
}

func canonicalRawJSON(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return raw
	}
	var value any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&value) != nil {
		return raw
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return raw
	}
	return canonical
}

func SourceDigestV1(source AgentDefinitionSourceV1, catalog ValidationCatalogV1) (core.Digest, error) {
	if err := ValidateSourceV1(source, catalog); err != nil {
		return "", err
	}
	return core.CanonicalJSONDigest(DigestDomainV1, DigestVersionV1, "AgentDefinitionSourceV1", normalizeSourceForDigestV1(source))
}

func DefinitionDigestV1(definition AgentDefinitionV1, catalog ValidationCatalogV1) (core.Digest, error) {
	copy := CloneDefinitionV1(definition)
	copy.Digest = ""
	copy.AgentDefinitionSourceV1 = normalizeSourceForDigestV1(copy.AgentDefinitionSourceV1)
	if err := validateDefinitionWithoutDigestV1(copy, catalog); err != nil {
		return "", err
	}
	return core.CanonicalJSONDigest(DigestDomainV1, DigestVersionV1, "AgentDefinitionV1", copy)
}

func SealDefinitionV1(source AgentDefinitionSourceV1, catalog ValidationCatalogV1, createdUnixNano int64) (AgentDefinitionV1, error) {
	if err := ValidateSourceV1(source, catalog); err != nil {
		return AgentDefinitionV1{}, err
	}
	source = normalizeSourceForDigestV1(source)
	sourceDigest, err := SourceDigestV1(source, catalog)
	if err != nil {
		return AgentDefinitionV1{}, err
	}
	definition := AgentDefinitionV1{AgentDefinitionSourceV1: source, CreatedUnixNano: createdUnixNano, SourceDigest: sourceDigest}
	if err := validateDefinitionWithoutDigestV1(definition, catalog); err != nil {
		return AgentDefinitionV1{}, err
	}
	definition.Digest, err = DefinitionDigestV1(definition, catalog)
	if err != nil {
		return AgentDefinitionV1{}, err
	}
	return definition, nil
}
