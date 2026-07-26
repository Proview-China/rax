package conformance_test

import (
	"context"
	"fmt"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/storage/memory"
)

func ExampleRunBackendSuiteV1() {
	isolated := memory.New()
	report, err := conformance.RunBackendSuiteV1(context.Background(), conformance.BackendSuiteRequestV1{
		Namespace:   "example-backend",
		NowUnixNano: 1_900_000_000_000_000_000,
		Metadata:    isolated,
		Content:     isolated,
		Retention:   isolated,
	})
	fmt.Println(err == nil, report.ReferenceOnly, report.ProductionEligible, len(report.Checks))
	// Output: true true false 11
}
