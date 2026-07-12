package catalogassets_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/catalog"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/anthropic"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/azureopenai"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/bedrockmantle"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/bedrockruntime"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/deepseek"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/gemini"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/kimi"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/mimo"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/minimax"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/openai"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/plancompat"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/qwen"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/vertex"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/xai"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/zai"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/upstream"
)

var assetRenderTime = time.Date(2026, 7, 11, 9, 0, 0, 0, time.UTC)

func TestEmbeddedSchemaIsStrictVersionedAndMatchesCheckedInAsset(t *testing.T) {
	if err := catalog.ValidateEmbeddedSchema(); err != nil {
		t.Fatalf("ValidateEmbeddedSchema() error = %v", err)
	}
	checkedIn, err := os.ReadFile(filepath.Join(moduleRoot(t), catalog.SchemaAssetPath))
	if err != nil {
		t.Fatalf("read checked-in schema: %v", err)
	}
	embedded := catalog.JSONSchema()
	if !bytes.Equal(embedded, checkedIn) {
		t.Fatal("embedded catalog schema differs from checked-in schema asset")
	}
	if len(embedded) == 0 {
		t.Fatal("embedded catalog schema is empty")
	}
	embedded[0] ^= 0xff
	if bytes.Equal(embedded, catalog.JSONSchema()) {
		t.Fatal("JSONSchema returned shared mutable storage")
	}
}

func TestEmbeddedSchemaCoversTheCompleteGoDocumentShape(t *testing.T) {
	var root map[string]any
	if err := json.Unmarshal(catalog.JSONSchema(), &root); err != nil {
		t.Fatalf("decode embedded schema: %v", err)
	}
	if err := compareGoJSONShape(reflect.TypeOf(catalog.Document{}), root, root, "$"); err != nil {
		t.Fatal(err)
	}
}

func TestDefaultCatalogEvidenceIsCurrentAtWallClock(t *testing.T) {
	if _, err := catalog.NewDefault(time.Now().UTC()); err != nil {
		t.Fatalf("default catalog evidence is no longer current: %v", err)
	}
}

func TestCurrentBindingsBlockMatchesProviderMatrix(t *testing.T) {
	matrix, err := os.ReadFile(providerMatrixPath(t))
	if err != nil {
		t.Fatalf("read provider matrix: %v", err)
	}
	if err := catalog.ValidateCurrentBindingsMarkdown(catalog.DefaultDocument(), assetRenderTime, matrix); err != nil {
		t.Fatal(err)
	}
	if bytes.Count(matrix, []byte(catalog.CurrentBindingsStartMarker)) != 1 ||
		bytes.Count(matrix, []byte(catalog.CurrentBindingsEndMarker)) != 1 {
		t.Fatal("provider matrix generated marker pair is not unique")
	}
	for _, entry := range catalog.DefaultDocument().Entries {
		if entry.Implementation.Callable && !bytes.Contains(matrix, []byte("`"+string(entry.ID)+"`")) {
			t.Fatalf("provider matrix generated block is missing route %q", entry.ID)
		}
	}
}

func TestDefaultBindingMetadataMatchesRuntimeAndEvidencePathsExist(t *testing.T) {
	wantAdapter := map[upstream.ProviderID]string{
		"openai": string(openai.ProviderID), "anthropic": string(anthropic.ProviderID),
		"google.gemini-developer": string(gemini.ProviderID),
		"aws.bedrock-mantle":      string(bedrockmantle.ProviderID),
		"aws.bedrock-runtime":     string(bedrockruntime.ProviderID),
		"google.vertex-ai":        string(vertex.ProviderID), "azure.openai": string(azureopenai.ProviderID),
		"deepseek": string(deepseek.ProviderID), "kimi": string(kimi.ProviderID), "minimax": string(minimax.ProviderID), "xiaomi.mimo": string(mimo.ProviderID), "zai": string(zai.ProviderID), "alibaba.model-studio": string(qwen.ProviderID), "xai.api": string(xai.ProviderID),
	}
	root := moduleRoot(t)
	for _, entry := range catalog.DefaultDocument().Entries {
		if !entry.Implementation.Callable {
			if entry.Implementation.AdapterID != "" && entry.Implementation.HostActivationRequirement != catalog.HostActivationTrustedSubscriptionAuthorizationResolver {
				t.Errorf("non-callable route %q has runtime adapter %q", entry.ID, entry.Implementation.AdapterID)
			}
			if entry.Implementation.HostActivationRequirement == catalog.HostActivationTrustedSubscriptionAuthorizationResolver &&
				(entry.Implementation.Status != catalog.ImplementationImplementedOffline || entry.Implementation.AdapterID == "") {
				t.Errorf("host-blocked route %q lost implemented adapter evidence", entry.ID)
			}
			continue
		}
		want, ok := wantAdapter[entry.Route.Provider]
		if !ok {
			t.Fatalf("default catalog contains unexpected current binding %q", entry.ID)
		}
		switch entry.Route.Offering.ID {
		case "kimi.code-membership":
			want = string(plancompat.KimiCodeProvider)
		case "minimax.token-plan":
			want = string(plancompat.MiniMaxTokenProvider)
		case "mimo.token-plan":
			want = string(plancompat.MiMoTokenProvider)
		case "alibaba.coding-plan", "alibaba.token-plan-team":
			want = string(plancompat.AlibabaPlanProvider)
		}
		if got := entry.Implementation.AdapterID; got != want {
			t.Errorf("route %q runtime adapter = %q, want %q", entry.ID, got, want)
		}
		for _, relative := range append(append([]string(nil), entry.Implementation.CodePaths...), entry.Implementation.TestEvidence...) {
			if filepath.IsAbs(relative) || strings.Contains(relative, "\\") {
				t.Errorf("route %q evidence path %q must be module-relative with slash separators", entry.ID, relative)
				continue
			}
			if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(relative))); err != nil {
				t.Errorf("route %q evidence path %q does not resolve: %v", entry.ID, relative, err)
			}
		}
	}
}

func TestRepositoryAssetsResolveWithoutWorkingDirectory(t *testing.T) {
	t.Chdir(t.TempDir())
	if _, err := os.Stat(filepath.Join(moduleRoot(t), "go.mod")); err != nil {
		t.Fatalf("module root does not resolve after chdir: %v", err)
	}
	if _, err := os.Stat(providerMatrixPath(t)); err != nil {
		t.Fatalf("provider matrix does not resolve after chdir: %v", err)
	}
}

func moduleRoot(t testing.TB) string {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve catalogassets test file")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", ".."))
}

func repositoryRoot(t testing.TB) string {
	t.Helper()
	return filepath.Clean(filepath.Join(moduleRoot(t), "..", ".."))
}

func providerMatrixPath(t testing.TB) string {
	t.Helper()
	return filepath.Join(repositoryRoot(t), ".properties.rax", "design", "model-invoker", "provider-matrix.md")
}

func compareGoJSONShape(goType reflect.Type, schemaRoot, schemaNode map[string]any, path string) error {
	for goType.Kind() == reflect.Pointer {
		goType = goType.Elem()
	}
	schemaNode, err := resolveSchemaNode(schemaRoot, schemaNode)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if goType.PkgPath() == "time" && goType.Name() == "Time" {
		if schemaNode["type"] != "string" {
			return fmt.Errorf("%s: time.Time schema type = %v, want string", path, schemaNode["type"])
		}
		return nil
	}

	switch goType.Kind() {
	case reflect.Struct:
		properties, ok := schemaNode["properties"].(map[string]any)
		if !ok {
			return fmt.Errorf("%s: struct %s has no schema properties", path, goType)
		}
		required := make(map[string]struct{})
		if values, ok := schemaNode["required"].([]any); ok {
			for _, value := range values {
				if name, ok := value.(string); ok {
					required[name] = struct{}{}
				}
			}
		}
		seen := make(map[string]struct{})
		for index := 0; index < goType.NumField(); index++ {
			field := goType.Field(index)
			if !field.IsExported() {
				continue
			}
			tag := field.Tag.Get("json")
			parts := strings.Split(tag, ",")
			name := parts[0]
			if name == "-" {
				continue
			}
			if name == "" {
				name = field.Name
			}
			optional := false
			for _, option := range parts[1:] {
				optional = optional || option == "omitempty"
			}
			rawChild, exists := properties[name]
			if !exists {
				return fmt.Errorf("%s.%s: Go JSON field is absent from strict schema", path, name)
			}
			seen[name] = struct{}{}
			if !optional {
				if _, exists := required[name]; !exists {
					return fmt.Errorf("%s.%s: non-optional Go JSON field is not required by schema", path, name)
				}
			}
			child, ok := rawChild.(map[string]any)
			if !ok {
				return fmt.Errorf("%s.%s: schema property is not an object", path, name)
			}
			if err := compareGoJSONShape(field.Type, schemaRoot, child, path+"."+name); err != nil {
				return err
			}
		}
		for name := range properties {
			if _, exists := seen[name]; !exists {
				return fmt.Errorf("%s.%s: strict schema property has no Go JSON field", path, name)
			}
		}
	case reflect.Slice, reflect.Array:
		if schemaNode["type"] != "array" {
			return fmt.Errorf("%s: slice schema type = %v, want array", path, schemaNode["type"])
		}
		rawItems, ok := schemaNode["items"].(map[string]any)
		if !ok {
			return fmt.Errorf("%s: slice schema has no items object", path)
		}
		return compareGoJSONShape(goType.Elem(), schemaRoot, rawItems, path+"[]")
	}
	return nil
}

func resolveSchemaNode(root, node map[string]any) (map[string]any, error) {
	if rawAll, ok := node["allOf"].([]any); ok {
		for _, candidate := range rawAll {
			object, ok := candidate.(map[string]any)
			if ok && object["$ref"] != nil {
				return resolveSchemaNode(root, object)
			}
		}
	}
	reference, _ := node["$ref"].(string)
	if reference == "" {
		return node, nil
	}
	const prefix = "#/$defs/"
	if !strings.HasPrefix(reference, prefix) {
		return nil, fmt.Errorf("unsupported schema reference %q", reference)
	}
	definitions, ok := root["$defs"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("schema definitions are absent")
	}
	target, ok := definitions[strings.TrimPrefix(reference, prefix)].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("schema reference %q is unresolved", reference)
	}
	return resolveSchemaNode(root, target)
}
