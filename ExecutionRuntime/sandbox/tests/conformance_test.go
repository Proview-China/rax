package sandbox_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

func TestConformanceLocalTestkitIsNotProductionCertification(t *testing.T) {
	t.Parallel()
	report := testkit.ConformanceReport()
	port := testkit.NewLocalConformance(report)
	var _ ports.BackendConformancePort = port

	got, err := port.Assess(context.Background(), ports.BackendConformanceRequest{Backend: testkit.Backend(), Requirement: testkit.Requirement()})
	if err != nil {
		t.Fatal(err)
	}
	if got.ProductionProof {
		t.Fatal("local testkit claimed production proof")
	}
	if err := got.ValidateCurrent(testkit.FixedNow); err != nil {
		t.Fatal(err)
	}

	report.ProductionProof = true
	port = testkit.NewLocalConformance(report)
	if _, err := port.Assess(context.Background(), ports.BackendConformanceRequest{Backend: testkit.Backend(), Requirement: testkit.Requirement()}); !errors.Is(err, ports.ErrUnsupported) {
		t.Fatalf("production proof claim error = %v", err)
	}
}
