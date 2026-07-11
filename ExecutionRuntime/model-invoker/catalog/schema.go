package catalog

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
)

const (
	// SchemaAssetPath is the module-relative path of the checked-in catalog
	// schema. It is stable so non-Go tooling can consume the same contract.
	SchemaAssetPath = "catalog/schema/catalog-v1.schema.json"
	schemaDraft     = "https://json-schema.org/draft/2020-12/schema"
)

//go:embed schema/catalog-v1.schema.json
var embeddedSchema []byte

// JSONSchema returns a defensive copy of the versioned catalog JSON Schema.
func JSONSchema() []byte {
	return append([]byte(nil), embeddedSchema...)
}

// ValidateEmbeddedSchema performs the offline structural checks that protect
// the embedded cross-language contract. Runtime catalog documents are still
// decoded with Decode and validated with Validate.
func ValidateEmbeddedSchema() error {
	decoder := json.NewDecoder(bytes.NewReader(embeddedSchema))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return fmt.Errorf("decode embedded catalog schema: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("embedded catalog schema contains multiple JSON values")
		}
		return fmt.Errorf("decode embedded catalog schema trailing data: %w", err)
	}

	root, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("embedded catalog schema root must be an object")
	}
	if root["$schema"] != schemaDraft {
		return fmt.Errorf("embedded catalog schema draft is %q, want %q", root["$schema"], schemaDraft)
	}
	properties, ok := root["properties"].(map[string]any)
	if !ok {
		return fmt.Errorf("embedded catalog schema has no properties object")
	}
	version, ok := properties["schema_version"].(map[string]any)
	if !ok || version["const"] != SchemaVersion {
		return fmt.Errorf("embedded catalog schema version does not match %q", SchemaVersion)
	}
	definitions, ok := root["$defs"].(map[string]any)
	if !ok || len(definitions) == 0 {
		return fmt.Errorf("embedded catalog schema has no definitions")
	}
	if err := validateStrictSchemaObjects(root, "$"); err != nil {
		return err
	}
	return nil
}

func validateStrictSchemaObjects(value any, path string) error {
	switch node := value.(type) {
	case map[string]any:
		if node["type"] == "object" {
			additional, ok := node["additionalProperties"].(bool)
			if !ok || additional {
				return fmt.Errorf("embedded catalog schema object %s must set additionalProperties to false", path)
			}
			if _, ok := node["properties"].(map[string]any); !ok {
				return fmt.Errorf("embedded catalog schema object %s has no properties", path)
			}
		}
		for key, child := range node {
			if err := validateStrictSchemaObjects(child, path+"."+key); err != nil {
				return err
			}
		}
	case []any:
		for index, child := range node {
			if err := validateStrictSchemaObjects(child, fmt.Sprintf("%s[%d]", path, index)); err != nil {
				return err
			}
		}
	}
	return nil
}
