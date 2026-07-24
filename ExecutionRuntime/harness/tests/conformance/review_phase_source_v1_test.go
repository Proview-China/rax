package conformance_test

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/kernel"
	harnessports "github.com/Proview-China/rax/ExecutionRuntime/harness/ports"
)

func TestReviewPhaseSourceV1PortIsNarrowAndConstructorHasReadOnlyOwners(t *testing.T) {
	reader := reflect.TypeOf((*harnessports.ReviewPhaseSourceCurrentReaderV1)(nil)).Elem()
	if reader.NumMethod() != 1 || reader.Method(0).Name != "InspectReviewPhaseSourceCurrentV1" {
		t.Fatalf("Review phase source Reader method set=%v", reader)
	}
	constructor := reflect.TypeOf(kernel.NewReviewPhaseSourceCurrentReaderV1)
	want := []reflect.Type{reflect.TypeOf((*harnessports.CommittedPendingActionReaderV3)(nil)).Elem(), reflect.TypeOf((*harnessports.SessionCurrentReaderV4)(nil)).Elem(), reflect.TypeOf((func() time.Time)(nil))}
	if constructor.NumIn() != len(want) {
		t.Fatalf("constructor inputs=%d", constructor.NumIn())
	}
	for index, expected := range want {
		if constructor.In(index) != expected {
			t.Fatalf("constructor input %d=%v want=%v", index, constructor.In(index), expected)
		}
	}
	for _, forbidden := range []reflect.Type{reflect.TypeOf((*harnessports.SessionFactPortV4)(nil)).Elem()} {
		for index := 0; index < constructor.NumIn(); index++ {
			if constructor.In(index) == forbidden {
				t.Fatalf("constructor exposes write port %v", forbidden)
			}
		}
	}
}

func TestReviewPhaseSourceV1ProductionFilesDoNotImportApplicationOrReview(t *testing.T) {
	_, current, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(current), "..", ".."))
	files := []string{filepath.Join(root, "contract", "review_phase_source_v1.go"), filepath.Join(root, "ports", "review_phase_source_v1.go"), filepath.Join(root, "kernel", "review_phase_source_v1.go")}
	for _, path := range files {
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imported := range parsed.Imports {
			value := strings.Trim(imported.Path.Value, `"`)
			if strings.Contains(value, "/ExecutionRuntime/application") || strings.Contains(value, "/ExecutionRuntime/review") {
				t.Fatalf("%s imports forbidden owner %s", path, value)
			}
		}
	}
}
