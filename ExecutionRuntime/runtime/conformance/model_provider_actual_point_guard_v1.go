package conformance

import (
	"context"
	"reflect"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

var requiredModelProviderPathsV1 = []string{
	"direct",
	"stream",
	"continuation",
	"realtime",
	"routegateway",
	"raw",
}

type ModelProviderActualPointWiringV1 struct {
	GuardedPaths []string `json:"guarded_paths"`
}

type ModelProviderActualPointConformanceFixtureV1 struct {
	Request                      ports.InspectCurrentModelProviderActualPointRequestV1
	CheckedUnixNano              int64
	ExpectedProvider             ports.ProviderBindingRefV2
	ExpectedRuntimeControlDigest core.Digest
	ExpectedNotAfterUnixNano     int64
	Wiring                       ModelProviderActualPointWiringV1
}

type ModelProviderActualPointConformanceReportV1 struct {
	CurrentProjectionValid       bool `json:"current_projection_valid"`
	TamperFailsClosed            bool `json:"tamper_fails_closed"`
	OwnerLocalPathInventoryExact bool `json:"owner_local_path_inventory_exact"`
	ProductionEligible           bool `json:"production_eligible"`
}

// CheckModelProviderActualPointGuardV1 uses only the public read-only guard.
// It never invokes a Provider and cannot claim production eligibility while a
// Runtime cancel/current implementation and production composition root are
// absent.
func CheckModelProviderActualPointGuardV1(ctx context.Context, guard ports.ModelProviderActualPointGuardV1, fixture ModelProviderActualPointConformanceFixtureV1) (ModelProviderActualPointConformanceReportV1, error) {
	if conformanceDependencyNilV1(guard) {
		return ModelProviderActualPointConformanceReportV1{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "Model actual-point guard is missing")
	}
	if err := fixture.Request.Validate(); err != nil {
		return ModelProviderActualPointConformanceReportV1{}, err
	}
	if fixture.CheckedUnixNano <= 0 || fixture.ExpectedNotAfterUnixNano <= fixture.CheckedUnixNano {
		return ModelProviderActualPointConformanceReportV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Model actual-point conformance time window is invalid")
	}
	if err := fixture.ExpectedProvider.Validate(); err != nil {
		return ModelProviderActualPointConformanceReportV1{}, err
	}
	if err := fixture.ExpectedRuntimeControlDigest.Validate(); err != nil {
		return ModelProviderActualPointConformanceReportV1{}, err
	}
	projection, err := guard.InspectCurrentModelProviderActualPointV1(ctx, fixture.Request)
	if err != nil {
		return ModelProviderActualPointConformanceReportV1{}, err
	}
	checkedAt := time.Unix(0, fixture.CheckedUnixNano)
	exactClosure := projection.ValidateAgainst(fixture.Request, checkedAt) == nil &&
		projection.Provider == fixture.ExpectedProvider &&
		projection.RuntimeControlDigest == fixture.ExpectedRuntimeControlDigest &&
		projection.NotAfterUnixNano == fixture.ExpectedNotAfterUnixNano
	report := ModelProviderActualPointConformanceReportV1{
		CurrentProjectionValid: exactClosure,
		ProductionEligible:     false,
	}
	tampered := fixture.Request
	tampered.ModelBoundary.Digest = fixture.Request.PermitDigest
	tamperedProjection, tamperErr := guard.InspectCurrentModelProviderActualPointV1(ctx, tampered)
	report.TamperFailsClosed = tamperErr != nil && tamperedProjection == (ports.ModelProviderActualPointCurrentProjectionV1{})
	// This checks owner-local metadata only. It cannot prove that an external
	// Model/Harness/Provider composition has no bypass.
	report.OwnerLocalPathInventoryExact = exactGuardedModelProviderPathsV1(fixture.Wiring.GuardedPaths)
	return report, nil
}

func conformanceDependencyNilV1(value any) bool {
	if value == nil {
		return true
	}
	ref := reflect.ValueOf(value)
	switch ref.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return ref.IsNil()
	default:
		return false
	}
}

func exactGuardedModelProviderPathsV1(paths []string) bool {
	if len(paths) != len(requiredModelProviderPathsV1) {
		return false
	}
	seen := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		if _, exists := seen[path]; exists {
			return false
		}
		seen[path] = struct{}{}
	}
	for _, required := range requiredModelProviderPathsV1 {
		if _, exists := seen[required]; !exists {
			return false
		}
	}
	return true
}
