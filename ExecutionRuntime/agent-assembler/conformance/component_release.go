package conformance

import (
	"context"
	"reflect"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/contract"
	assemblerports "github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type ComponentReleaseReportV1 struct {
	Exact                  bool `json:"exact"`
	Stable                 bool `json:"stable"`
	Production             bool `json:"production"`
	CertificationPresent   bool `json:"certification_present"`
	CertificationCandidate bool `json:"certification_candidate"`
	GrantsAuthority        bool `json:"grants_authority"`
	GrantsDispatch         bool `json:"grants_dispatch"`
}

func CheckComponentReleaseV1(ctx context.Context, reader assemblerports.ComponentReleaseReaderV1, ref contract.ComponentReleaseRefV1, nowUnixNano int64) (ComponentReleaseReportV1, error) {
	if reader == nil {
		return ComponentReleaseReportV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "component release reader is required")
	}
	first, err := reader.InspectExactComponentReleaseV1(ctx, ref)
	if err != nil {
		return ComponentReleaseReportV1{}, err
	}
	if err := first.Validate(); err != nil || first.RefV1() != ref {
		return ComponentReleaseReportV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "component release reader returned invalid exact content")
	}
	second, err := reader.InspectExactComponentReleaseV1(ctx, ref)
	if err != nil {
		return ComponentReleaseReportV1{}, err
	}
	if err := second.Validate(); err != nil || second.RefV1() != first.RefV1() || !reflect.DeepEqual(first, second) {
		return ComponentReleaseReportV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "component release drifted during conformance")
	}
	if nowUnixNano < first.CreatedUnixNano || nowUnixNano >= first.ExpiresUnixNano {
		return ComponentReleaseReportV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "component release is not current during conformance")
	}
	production := first.SupportMode == contract.SupportProductionV1
	report := ComponentReleaseReportV1{Exact: true, Stable: true, Production: production, CertificationPresent: first.CertificationRef.Validate() == nil, CertificationCandidate: production, GrantsAuthority: false, GrantsDispatch: false}
	return report, nil
}

type ResolverReportV1 struct {
	PlanValid                bool `json:"plan_valid"`
	BindingPlanValid         bool `json:"binding_plan_valid"`
	AssemblyInputValid       bool `json:"assembly_input_valid"`
	ProductionReadyCandidate bool `json:"production_ready_candidate"`
	GrantsActivation         bool `json:"grants_activation"`
}

func CheckResolveResultV1(result contract.ResolveResultV1) (ResolverReportV1, error) {
	if err := result.Plan.Validate(); err != nil {
		return ResolverReportV1{}, err
	}
	if err := result.BindingPlan.Validate(); err != nil {
		return ResolverReportV1{}, err
	}
	if err := result.AssemblyInput.Validate(); err != nil {
		return ResolverReportV1{}, err
	}
	if !reflect.DeepEqual(result.Plan.BindingPlan, result.BindingPlan) || !reflect.DeepEqual(result.AssemblyInput.Plan, result.Plan.AssemblyPlanRefs) {
		return ResolverReportV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "resolver outputs are not exact-cross-bound")
	}
	return ResolverReportV1{PlanValid: true, BindingPlanValid: true, AssemblyInputValid: true, ProductionReadyCandidate: true, GrantsActivation: false}, nil
}
