package core_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	. "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
)

func TestRootPackageDoesNotImportProviderSDKs(t *testing.T) {
	entries, err := os.ReadDir("../..")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Join("../..", entry.Name())
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, imported := range file.Imports {
			path, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(path, "openai-go") || strings.Contains(path, "anthropic-sdk-go") || path == "google.golang.org/genai" {
				t.Errorf("root production file %s imports provider SDK %q", entry.Name(), path)
			}
		}
	}
}

func TestAllCapabilitiesIsCompleteUniqueAndDefensive(t *testing.T) {
	capabilities := AllCapabilities()
	if len(capabilities) != 20 {
		t.Fatalf("AllCapabilities() length = %d, want 20", len(capabilities))
	}
	seen := make(map[Capability]struct{}, len(capabilities))
	for _, capability := range capabilities {
		if capability == "" {
			t.Fatal("AllCapabilities() contains an empty capability")
		}
		if _, duplicate := seen[capability]; duplicate {
			t.Fatalf("AllCapabilities() contains duplicate %q", capability)
		}
		seen[capability] = struct{}{}
	}
	capabilities[0] = "mutated"
	if AllCapabilities()[0] == "mutated" {
		t.Fatal("AllCapabilities() returned shared storage")
	}
}
