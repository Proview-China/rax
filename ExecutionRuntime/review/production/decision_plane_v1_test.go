package production_test

import (
	"context"
	"testing"
	"time"

	reviewmemory "github.com/Proview-China/rax/ExecutionRuntime/review/memory"
	"github.com/Proview-China/rax/ExecutionRuntime/review/production"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type bindingReaderV1 struct{}

func (*bindingReaderV1) ResolveCurrentReviewBindingV1(context.Context, runtimeports.ResolveReviewBindingCurrentRequestV1) (runtimeports.ReviewBindingProjectionRefV1, error) {
	return runtimeports.ReviewBindingProjectionRefV1{}, nil
}
func (*bindingReaderV1) InspectReviewBindingProjectionV1(context.Context, runtimeports.InspectReviewBindingProjectionRequestV1) (runtimeports.ReviewBindingCurrentProjectionV1, error) {
	return runtimeports.ReviewBindingCurrentProjectionV1{}, nil
}
func (*bindingReaderV1) InspectCurrentReviewBindingV1(context.Context, runtimeports.InspectCurrentReviewBindingRequestV1) (runtimeports.ReviewBindingCurrentProjectionV1, error) {
	return runtimeports.ReviewBindingCurrentProjectionV1{}, nil
}

type evidenceReaderV1 struct{}

func (*evidenceReaderV1) ResolveReviewEvidenceApplicabilityCurrentV1(context.Context, runtimeports.ResolveReviewEvidenceApplicabilityCurrentRequestV1) (runtimeports.ReviewEvidenceApplicabilityCurrentSnapshotV1, error) {
	return runtimeports.ReviewEvidenceApplicabilityCurrentSnapshotV1{}, nil
}
func (*evidenceReaderV1) InspectCurrentReviewEvidenceApplicabilityV1(context.Context, runtimeports.ReviewEvidenceApplicabilityRefV1) (runtimeports.ReviewEvidenceApplicabilityCurrentSnapshotV1, error) {
	return runtimeports.ReviewEvidenceApplicabilityCurrentSnapshotV1{}, nil
}
func (*evidenceReaderV1) InspectHistoricalReviewEvidenceApplicabilityV1(context.Context, runtimeports.ReviewEvidenceApplicabilityRefV1) (runtimeports.ReviewEvidenceApplicabilityProjectionV1, error) {
	return runtimeports.ReviewEvidenceApplicabilityProjectionV1{}, nil
}

type policyReaderV1 struct{}

func (*policyReaderV1) ResolveCurrentReviewDecisionPolicyV1(context.Context, runtimeports.ReviewDecisionPolicyCurrentResolveRequestV1) (runtimeports.ReviewDecisionPolicyCurrentProjectionRefV1, error) {
	return runtimeports.ReviewDecisionPolicyCurrentProjectionRefV1{}, nil
}
func (*policyReaderV1) InspectCurrentReviewDecisionPolicyV1(context.Context, runtimeports.ReviewDecisionPolicyCurrentSubjectV1, runtimeports.ReviewDecisionPolicyCurrentProjectionRefV1) (runtimeports.ReviewDecisionPolicyCurrentProjectionV1, error) {
	return runtimeports.ReviewDecisionPolicyCurrentProjectionV1{}, nil
}
func (*policyReaderV1) InspectHistoricalReviewDecisionPolicyV1(context.Context, runtimeports.ReviewDecisionPolicyCurrentProjectionRefV1) (runtimeports.ReviewDecisionPolicyCurrentProjectionV1, error) {
	return runtimeports.ReviewDecisionPolicyCurrentProjectionV1{}, nil
}

type authorityReaderV1 struct{}

func (*authorityReaderV1) ResolveCurrentReviewDecisionAuthorityV1(context.Context, runtimeports.ReviewDecisionAuthorityCurrentResolveRequestV1) (runtimeports.ReviewDecisionAuthorityCurrentProjectionRefV1, error) {
	return runtimeports.ReviewDecisionAuthorityCurrentProjectionRefV1{}, nil
}
func (*authorityReaderV1) InspectCurrentReviewDecisionAuthorityV1(context.Context, runtimeports.ReviewDecisionAuthorityCurrentSubjectV1, runtimeports.ReviewDecisionAuthorityCurrentProjectionRefV1) (runtimeports.ReviewDecisionAuthorityCurrentProjectionV1, error) {
	return runtimeports.ReviewDecisionAuthorityCurrentProjectionV1{}, nil
}
func (*authorityReaderV1) InspectHistoricalReviewDecisionAuthorityV1(context.Context, runtimeports.ReviewDecisionAuthorityCurrentProjectionRefV1) (runtimeports.ReviewDecisionAuthorityCurrentProjectionV1, error) {
	return runtimeports.ReviewDecisionAuthorityCurrentProjectionV1{}, nil
}

type scopeReaderV1 struct{}

func (*scopeReaderV1) ResolveCurrentReviewDecisionScopeV1(context.Context, runtimeports.ReviewDecisionScopeCurrentResolveRequestV1) (runtimeports.ReviewDecisionScopeCurrentProjectionRefV1, error) {
	return runtimeports.ReviewDecisionScopeCurrentProjectionRefV1{}, nil
}
func (*scopeReaderV1) InspectCurrentReviewDecisionScopeV1(context.Context, runtimeports.ReviewDecisionScopeCurrentSubjectV1, runtimeports.ReviewDecisionScopeCurrentProjectionRefV1) (runtimeports.ReviewDecisionScopeCurrentProjectionV1, error) {
	return runtimeports.ReviewDecisionScopeCurrentProjectionV1{}, nil
}
func (*scopeReaderV1) InspectHistoricalReviewDecisionScopeV1(context.Context, runtimeports.ReviewDecisionScopeCurrentProjectionRefV1) (runtimeports.ReviewDecisionScopeCurrentProjectionV1, error) {
	return runtimeports.ReviewDecisionScopeCurrentProjectionV1{}, nil
}

func TestDecisionPlaneV1ConstructsOnlyWithAllOwnerReaders(t *testing.T) {
	store := reviewmemory.NewStore()
	dependencies := production.DecisionPlaneDependenciesV1{
		Store: store, Binding: &bindingReaderV1{}, Evidence: &evidenceReaderV1{},
		Policy: &policyReaderV1{}, Authority: &authorityReaderV1{}, Scope: &scopeReaderV1{},
		Clock: time.Now,
	}
	plane, err := production.NewDecisionPlaneV1(dependencies)
	if err != nil || plane.External == nil || plane.Current == nil || plane.Verdicts == nil || plane.Worker == nil {
		t.Fatalf("plane=%+v err=%v", plane, err)
	}

	var typedNil *bindingReaderV1
	dependencies.Binding = typedNil
	if _, err = production.NewDecisionPlaneV1(dependencies); !core.HasReason(err, core.ReasonComponentMissing) {
		t.Fatalf("typed-nil Binding Reader accepted: %v", err)
	}
}

func TestRootV1RejectsPartialProductionComposition(t *testing.T) {
	if _, err := production.NewRootV1(production.RootDependenciesV1{}); !core.HasReason(err, core.ReasonComponentMissing) {
		t.Fatalf("partial production root accepted: %v", err)
	}
}
