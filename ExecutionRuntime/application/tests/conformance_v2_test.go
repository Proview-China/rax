package application_test

import (
	"context"
	"testing"
	"time"

	applicationconformance "github.com/Proview-China/rax/ExecutionRuntime/application/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/application/fakes"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func TestCustomStepCatalogConformanceCannotSelfGrantAuthority(t *testing.T) {
	now := time.Unix(1_900_000_000, 0)
	bundle, descriptor := applicationFixtureV2(t, now, "custom.ninth/process", true)
	store := fakes.NewFactStoreV2()
	if err := store.RegisterStepDescriptorV2(descriptor); err != nil {
		t.Fatal(err)
	}
	report, err := applicationconformance.CheckCustomStepCatalogV2(context.Background(), applicationconformance.CustomStepCatalogCaseV2{Catalog: store, Kind: descriptor.Kind, ExecutionClass: descriptor.ExecutionClass, RequiredCapability: descriptor.RequiredCapability, PayloadSchema: bundle.Plan.Steps[0].Payload.Schema, Clock: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	if !report.CertificationCandidate || !report.DescriptorValid || !report.DescriptorCurrent || !report.KindExact || !report.ExecutionClassExact || !report.CapabilityExact || !report.SchemaAccepted {
		t.Fatalf("custom module conformance report is incomplete: %#v", report)
	}
	if report.BindingEligible || report.DispatchEligible || report.CommitEligible {
		t.Fatalf("custom module conformance self-granted authority: %#v", report)
	}

	drifted := descriptor
	drifted.RequiredCapability = "custom.ninth/read-only"
	store2 := fakes.NewFactStoreV2()
	if err := store2.RegisterStepDescriptorV2(drifted); err != nil {
		t.Fatal(err)
	}
	_, err = applicationconformance.CheckCustomStepCatalogV2(context.Background(), applicationconformance.CustomStepCatalogCaseV2{Catalog: store2, Kind: descriptor.Kind, ExecutionClass: descriptor.ExecutionClass, RequiredCapability: descriptor.RequiredCapability, PayloadSchema: bundle.Plan.Steps[0].Payload.Schema, Clock: func() time.Time { return now }})
	if !core.HasReason(err, core.ReasonComponentMismatch) {
		t.Fatalf("capability drift passed custom module conformance: %v", err)
	}
}
