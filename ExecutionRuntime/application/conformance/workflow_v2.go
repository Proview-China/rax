// Package conformance exposes backend-neutral checks for custom Application
// step catalogs. Passing a check creates only a certification candidate; it
// never grants Binding, Review, Budget, Effect, dispatch or domain commit
// authority.
package conformance

import (
	"context"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type CustomStepCatalogCaseV2 struct {
	Catalog            applicationports.StepCatalogV2
	Kind               runtimeports.NamespacedNameV2
	ExecutionClass     contract.StepExecutionClassV2
	RequiredCapability runtimeports.CapabilityNameV2
	PayloadSchema      runtimeports.SchemaRefV2
	Clock              func() time.Time
}

type CustomStepCatalogReportV2 struct {
	DescriptorValid        bool `json:"descriptor_valid"`
	DescriptorCurrent      bool `json:"descriptor_current"`
	KindExact              bool `json:"kind_exact"`
	ExecutionClassExact    bool `json:"execution_class_exact"`
	CapabilityExact        bool `json:"capability_exact"`
	SchemaAccepted         bool `json:"schema_accepted"`
	CertificationCandidate bool `json:"certification_candidate"`
	BindingEligible        bool `json:"binding_eligible"`
	DispatchEligible       bool `json:"dispatch_eligible"`
	CommitEligible         bool `json:"commit_eligible"`
}

func CheckCustomStepCatalogV2(ctx context.Context, testCase CustomStepCatalogCaseV2) (CustomStepCatalogReportV2, error) {
	if testCase.Catalog == nil || testCase.Clock == nil {
		return CustomStepCatalogReportV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "custom step catalog is required")
	}
	if err := runtimeports.ValidateNamespacedNameV2(testCase.Kind); err != nil {
		return CustomStepCatalogReportV2{}, err
	}
	if err := testCase.PayloadSchema.Validate(); err != nil {
		return CustomStepCatalogReportV2{}, err
	}
	descriptor, err := testCase.Catalog.ResolveStepKindV2(ctx, testCase.Kind)
	if err != nil {
		return CustomStepCatalogReportV2{}, err
	}
	if err := descriptor.ValidateCurrent(testCase.Clock()); err != nil {
		return CustomStepCatalogReportV2{}, err
	}
	if descriptor.Kind != testCase.Kind || descriptor.ExecutionClass != testCase.ExecutionClass || descriptor.RequiredCapability != testCase.RequiredCapability || !containsSchemaV2(descriptor.Schemas, testCase.PayloadSchema) {
		return CustomStepCatalogReportV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonComponentMismatch, "custom step descriptor differs from its expected kind, execution class, capability or schema")
	}
	return CustomStepCatalogReportV2{
		DescriptorValid: true, DescriptorCurrent: true, KindExact: true, ExecutionClassExact: true,
		CapabilityExact: true, SchemaAccepted: true, CertificationCandidate: true,
		BindingEligible: false, DispatchEligible: false, CommitEligible: false,
	}, nil
}

func containsSchemaV2(values []runtimeports.SchemaRefV2, expected runtimeports.SchemaRefV2) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
