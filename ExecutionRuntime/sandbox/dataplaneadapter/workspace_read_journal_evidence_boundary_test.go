package dataplaneadapter

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestWorkspaceReadPhysicalJournalEvidenceV2HasOneProductionAuthorizer(t *testing.T) {
	t.Parallel()
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("test source path is unavailable")
	}
	module := filepath.Dir(filepath.Dir(current))
	definition := filepath.Join(module, "kernel", "workspace_read_actual_point_contract_v2.go")
	consumer := filepath.Join(module, "kernel", "workspace_read_actual_point_v2.go")
	const needle = "authorizeWorkspaceReadPhysicalJournalEvidenceV2("
	found := map[string]int{}
	for _, directory := range []string{filepath.Join(module, "kernel"), filepath.Join(module, "dataplaneadapter")} {
		err := filepath.WalkDir(directory, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			body, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if count := strings.Count(string(body), needle); count > 0 {
				found[path] = count
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(found) != 2 || found[definition] != 1 || found[consumer] != 2 {
		t.Fatalf("physical journal evidence boundary=%v, want one private Kernel definition and two private IPC consumers", found)
	}
}
