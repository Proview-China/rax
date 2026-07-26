package conformance_test

import (
	"context"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/storage/memory"
)

func TestBackendSuiteV1PassesReferenceMemoryBackend(t *testing.T) {
	backend := memory.New()
	report, err := conformance.RunBackendSuiteV1(context.Background(), conformance.BackendSuiteRequestV1{
		Namespace:   "memory-reference",
		NowUnixNano: 1_900_000_000_000_000_000,
		Metadata:    backend,
		Content:     backend,
		Retention:   backend,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := report.Validate(); err != nil {
		t.Fatal(err)
	}
	if !report.ReferenceOnly || report.ProductionEligible || len(report.Checks) == 0 {
		t.Fatalf("backend report crossed reference boundary: %#v", report)
	}
}

func TestBackendSuiteV1RejectsTypedNilAndCanceledContextBeforeMutation(t *testing.T) {
	var typedNil *memory.Backend
	if _, err := conformance.RunBackendSuiteV1(context.Background(), conformance.BackendSuiteRequestV1{
		Namespace: "typed-nil", NowUnixNano: 1,
		Metadata: typedNil, Content: typedNil, Retention: typedNil,
	}); err == nil {
		t.Fatal("typed-nil backend was accepted")
	}

	backend := memory.New()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := conformance.RunBackendSuiteV1(ctx, conformance.BackendSuiteRequestV1{
		Namespace: "canceled", NowUnixNano: 1,
		Metadata: backend, Content: backend, Retention: backend,
	}); err != context.Canceled {
		t.Fatalf("canceled suite error=%v", err)
	}
	if _, err := backend.InspectJournal(context.Background(), "canceled-journal"); err == nil {
		t.Fatal("canceled suite mutated backend")
	}
}

func TestBackendSuiteReportV1IsExactCloneSafeAndCannotSelfPromote(t *testing.T) {
	backend := memory.New()
	report, err := conformance.RunBackendSuiteV1(context.Background(), conformance.BackendSuiteRequestV1{
		Namespace: "report-boundary", NowUnixNano: 1,
		Metadata: backend, Content: backend, Retention: backend,
	})
	if err != nil {
		t.Fatal(err)
	}
	clone := report.Clone()
	clone.Checks[0] = "caller-drift"
	if report.Checks[0] == clone.Checks[0] {
		t.Fatal("report clone aliases checks")
	}
	for _, mutate := range []func(*conformance.BackendSuiteReportV1){
		func(value *conformance.BackendSuiteReportV1) { value.ProductionEligible = true },
		func(value *conformance.BackendSuiteReportV1) { value.ReferenceOnly = false },
		func(value *conformance.BackendSuiteReportV1) { value.Checks = value.Checks[1:] },
		func(value *conformance.BackendSuiteReportV1) { value.Checks = append(value.Checks, value.Checks[0]) },
	} {
		changed := report.Clone()
		mutate(&changed)
		if err := changed.Validate(); err == nil {
			t.Fatalf("drifted report was accepted: %#v", changed)
		}
	}
}
