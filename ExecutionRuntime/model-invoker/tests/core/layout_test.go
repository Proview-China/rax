package core_test

import (
	"io/fs"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

func TestGoTestsStayUnderDedicatedTestsTree(t *testing.T) {
	t.Parallel()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve current test file")
	}
	moduleRoot := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", ".."))

	var violations []string
	err := filepath.WalkDir(moduleRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		relative, err := filepath.Rel(moduleRoot, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if relative != "tests" && !strings.HasPrefix(relative, "tests/") {
			violations = append(violations, relative)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan module layout: %v", err)
	}
	if len(violations) == 0 {
		return
	}
	sort.Strings(violations)
	t.Fatalf("Go test files must live under tests/: %s", strings.Join(violations, ", "))
}
