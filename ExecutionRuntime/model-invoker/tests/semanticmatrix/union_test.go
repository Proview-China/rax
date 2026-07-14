package semanticmatrix_test

import (
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/catalog"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/operation"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/semanticmatrix"
)

func TestUnionMatrixClosesEveryPrimitivePlaneAndUpstreamSurface(t *testing.T) {
	routeCatalog, err := catalog.NewDefault(matrixNow)
	if err != nil {
		t.Fatal(err)
	}
	matrix, err := semanticmatrix.BuildUnion(routeCatalog, matrixNow)
	if err != nil {
		t.Fatal(err)
	}
	planes := map[semanticmatrix.PrimitivePlane]int{}
	boundaries := map[string]int{}
	for _, row := range matrix.Surfaces {
		planes[row.Plane]++
		boundaries[row.InvocationBoundary]++
	}
	for _, plane := range []semanticmatrix.PrimitivePlane{
		semanticmatrix.PlaneExecution, semanticmatrix.PlaneLLMLocal, semanticmatrix.PlaneLLMRelay,
		semanticmatrix.PlaneOperation, semanticmatrix.PlaneRealtime,
	} {
		if planes[plane] == 0 {
			t.Fatalf("primitive plane %s is empty", plane)
		}
	}
	for _, boundary := range []string{
		"execution.Runtime", "modelinvoker.Invoker", "routegateway.Gateway",
		"operation.Invoker", "realtime.Invoker",
	} {
		if boundaries[boundary] == 0 {
			t.Fatalf("canonical invocation boundary %s is missing", boundary)
		}
	}
	if len(matrix.LLM.Rows) == 0 {
		t.Fatal("LLM route matrix is empty")
	}
	t.Logf("llm_rows=%d surface_rows=%d planes=%v", len(matrix.LLM.Rows), len(matrix.Surfaces), planes)
}

func TestUnionMatrixRejectsPrimitiveCoverageDriftAndDuplicateRows(t *testing.T) {
	routeCatalog, _ := catalog.NewDefault(matrixNow)
	matrix, err := semanticmatrix.BuildUnion(routeCatalog, matrixNow)
	if err != nil {
		t.Fatal(err)
	}

	missing := matrix
	missing.Surfaces = append([]semanticmatrix.SurfaceRow(nil), matrix.Surfaces...)
	filtered := missing.Surfaces[:0]
	for _, row := range missing.Surfaces {
		if row.Plane != semanticmatrix.PlaneOperation || row.Primitive != string(operation.MusicGenerate) {
			filtered = append(filtered, row)
		}
	}
	missing.Surfaces = filtered
	if err := missing.Validate(matrixNow); err == nil {
		t.Fatal("matrix accepted a missing operation primitive")
	}

	duplicate := matrix
	duplicate.Surfaces = append(append([]semanticmatrix.SurfaceRow(nil), matrix.Surfaces...), matrix.Surfaces[0])
	if err := duplicate.Validate(matrixNow); err == nil {
		t.Fatal("matrix accepted a duplicate primitive row")
	}
}
