package production_test

import (
	"context"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/autoattestation"
	"github.com/Proview-China/rax/ExecutionRuntime/review/autoreviewer"
	"github.com/Proview-China/rax/ExecutionRuntime/review/bypassowner"
	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/decisioncurrent"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/testkit"
	reviewmemory "github.com/Proview-China/rax/ExecutionRuntime/review/memory"
	"github.com/Proview-China/rax/ExecutionRuntime/review/multisigowner"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/review/production"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type autoInvocationV1 struct{}

func (*autoInvocationV1) StartOrInspectAutoReviewerInvocationV1(context.Context, reviewport.AutoReviewerInvocationCommandV1) (reviewport.AutoReviewerInvocationResultV1, error) {
	return reviewport.AutoReviewerInvocationResultV1{}, nil
}
func (*autoInvocationV1) InspectAutoReviewerInvocationV1(context.Context, contract.ExactResourceRefV1) (reviewport.AutoReviewerInvocationResultV1, error) {
	return reviewport.AutoReviewerInvocationResultV1{}, nil
}

type reviewerContextV1 struct{}

func (*reviewerContextV1) ResolveCurrentReviewerContextV1(context.Context, reviewport.ReviewerContextCurrentResolveRequestV1) (contract.ReviewerContextEnvelopeRefV1, error) {
	return contract.ReviewerContextEnvelopeRefV1{}, nil
}
func (*reviewerContextV1) InspectCurrentReviewerContextV1(context.Context, contract.ReviewerContextSubjectV1, contract.ReviewerContextEnvelopeRefV1) (contract.ReviewerContextEnvelopeV1, error) {
	return contract.ReviewerContextEnvelopeV1{}, nil
}
func (*reviewerContextV1) InspectHistoricalReviewerContextV1(context.Context, contract.ReviewerContextEnvelopeRefV1) (contract.ReviewerContextEnvelopeV1, error) {
	return contract.ReviewerContextEnvelopeV1{}, nil
}

type outputSchemaV1 struct{}

func (*outputSchemaV1) InspectAutoReviewerOutputSchemaV1(context.Context, runtimeports.SchemaRefV2) (contract.AutoReviewerOutputSchemaDocumentV1, error) {
	return contract.AutoReviewerOutputSchemaDocumentV1{}, nil
}

type humanExternalV2 struct{}

func (*humanExternalV2) ValidatePanelCurrentV2(context.Context, contract.HumanReviewPanelV2, []contract.HumanPanelAssignmentV2, contract.HumanReviewPanelV2, time.Time) (multisigowner.ExternalCurrentProofV2, error) {
	return multisigowner.ExternalCurrentProofV2{}, nil
}
func (*humanExternalV2) ValidateAttestationCurrentV2(context.Context, contract.HumanReviewPanelV2, contract.HumanPanelAssignmentV2, contract.HumanAttestationV2, time.Time) (multisigowner.ExternalCurrentProofV2, error) {
	return multisigowner.ExternalCurrentProofV2{}, nil
}
func (*humanExternalV2) ValidateDecisionCurrentV2(context.Context, contract.HumanReviewPanelV2, contract.HumanQuorumDecisionV2, contract.HumanVerdictV2, time.Time) (multisigowner.ExternalCurrentProofV2, error) {
	return multisigowner.ExternalCurrentProofV2{}, nil
}

type organizationCurrentV2 struct{}

func (*organizationCurrentV2) InspectHumanOrganizationCurrentV2(context.Context, []reviewport.HumanOrganizationCurrentRequestV2) (reviewport.HumanOrganizationCurrentCutV2, error) {
	return reviewport.HumanOrganizationCurrentCutV2{}, nil
}

type bypassExternalV1 struct{}

func (*bypassExternalV1) ReadBypassCurrentV1(context.Context, contract.BypassDecisionV1, time.Time) (contract.BypassExternalCurrentProofV1, error) {
	return contract.BypassExternalCurrentProofV1{}, nil
}

type humanPolicyV2 struct{}

func (*humanPolicyV2) ResolveCurrentHumanQuorumPolicyV2(context.Context, runtimeports.HumanQuorumPolicyCurrentResolveRequestV2) (runtimeports.HumanQuorumPolicyCurrentProjectionRefV2, error) {
	return runtimeports.HumanQuorumPolicyCurrentProjectionRefV2{}, nil
}
func (*humanPolicyV2) InspectCurrentHumanQuorumPolicyV2(context.Context, runtimeports.HumanQuorumPolicyCurrentSubjectV2, runtimeports.HumanQuorumPolicyCurrentProjectionRefV2) (runtimeports.HumanQuorumPolicyCurrentProjectionV2, error) {
	return runtimeports.HumanQuorumPolicyCurrentProjectionV2{}, nil
}
func (*humanPolicyV2) InspectHistoricalHumanQuorumPolicyV2(context.Context, runtimeports.HumanQuorumPolicyCurrentProjectionRefV2) (runtimeports.HumanQuorumPolicyCurrentProjectionV2, error) {
	return runtimeports.HumanQuorumPolicyCurrentProjectionV2{}, nil
}

type satisfactionV5 struct{}

func (*satisfactionV5) InspectConditionSatisfactionByVerdict(context.Context, string) (runtimeports.ConditionSatisfactionFactV2, error) {
	return runtimeports.ConditionSatisfactionFactV2{}, nil
}

type bypassPolicyV2 struct{}

func (*bypassPolicyV2) ResolveCurrentReviewDecisionPolicyV2(context.Context, runtimeports.ReviewDecisionPolicyCurrentResolveRequestV2) (runtimeports.ReviewDecisionPolicyCurrentProjectionRefV2, error) {
	return runtimeports.ReviewDecisionPolicyCurrentProjectionRefV2{}, nil
}
func (*bypassPolicyV2) InspectCurrentReviewDecisionPolicyV2(context.Context, runtimeports.ReviewDecisionPolicyApplicabilitySubjectV2, runtimeports.ReviewDecisionPolicyCurrentProjectionRefV2) (runtimeports.ReviewDecisionPolicyCurrentProjectionV2, error) {
	return runtimeports.ReviewDecisionPolicyCurrentProjectionV2{}, nil
}
func (*bypassPolicyV2) InspectHistoricalReviewDecisionPolicyV2(context.Context, runtimeports.ReviewDecisionPolicyCurrentProjectionRefV2) (runtimeports.ReviewDecisionPolicyCurrentProjectionV2, error) {
	return runtimeports.ReviewDecisionPolicyCurrentProjectionV2{}, nil
}

type actorAuthorityV2 struct{}

func (*actorAuthorityV2) ResolveCurrentReviewActorAuthorityV2(context.Context, runtimeports.ReviewActorAuthorityCurrentResolveRequestV2) (runtimeports.ReviewActorAuthorityCurrentProjectionRefV2, error) {
	return runtimeports.ReviewActorAuthorityCurrentProjectionRefV2{}, nil
}
func (*actorAuthorityV2) InspectCurrentReviewActorAuthorityV2(context.Context, runtimeports.ReviewActorAuthorityCurrentSubjectV2, runtimeports.ReviewActorAuthorityCurrentProjectionRefV2) (runtimeports.ReviewActorAuthorityCurrentProjectionV2, error) {
	return runtimeports.ReviewActorAuthorityCurrentProjectionV2{}, nil
}
func (*actorAuthorityV2) InspectHistoricalReviewActorAuthorityV2(context.Context, runtimeports.ReviewActorAuthorityCurrentProjectionRefV2) (runtimeports.ReviewActorAuthorityCurrentProjectionV2, error) {
	return runtimeports.ReviewActorAuthorityCurrentProjectionV2{}, nil
}

type providerBindingV2 struct{}

func (*providerBindingV2) InspectProviderBindingCurrentV2(context.Context, runtimeports.ProviderBindingRefV2) (runtimeports.ProviderBindingCurrentProjectionV2, error) {
	return runtimeports.ProviderBindingCurrentProjectionV2{}, nil
}

func validRootDependenciesV1() production.RootDependenciesV1 {
	store := reviewmemory.NewStore()
	clock := time.Now
	return production.RootDependenciesV1{
		Store: store, Clock: clock,
		Binding: &bindingReaderV1{}, Evidence: &evidenceReaderV1{}, Policy: &policyReaderV1{},
		Authority: &authorityReaderV1{}, Scope: &scopeReaderV1{},
		AutoInvocation: &autoInvocationV1{}, ReviewerContext: &reviewerContextV1{}, AutoOutputSchema: &outputSchemaV1{},
		HumanExternal: &humanExternalV2{}, HumanOrganization: &organizationCurrentV2{},
		BypassExternal: &bypassExternalV1{},
		RuntimeV5: decisioncurrent.HumanCurrentSourceDependenciesV5{
			Organization: &organizationCurrentV2{}, Binding: &bindingReaderV1{}, Evidence: &evidenceReaderV1{},
			Policy: &humanPolicyV2{}, Authority: &authorityReaderV1{}, Scope: &scopeReaderV1{},
			Satisfaction: &satisfactionV5{}, Clock: func() time.Time { panic("foreign V5 clock must never be retained") },
			Bypass: &decisioncurrent.BypassCurrentSourceDependenciesV5{
				Policy: &bypassPolicyV2{}, Authority: &actorAuthorityV2{}, Scope: &scopeReaderV1{}, Binding: &providerBindingV2{},
			},
		},
	}
}

func TestRootV1ConstructsCompleteReviewOwnedComposition(t *testing.T) {
	root, err := production.NewRootV1(validRootDependenciesV1())
	if err != nil {
		t.Fatal(err)
	}
	if root.Service == nil || root.Decision == nil || root.AutoReviewer == nil || root.AutoAttestation == nil ||
		root.HumanOwner == nil || root.HumanClaims == nil || root.HumanService == nil || root.BypassOwner == nil ||
		root.RuntimeV5Source == nil || root.RuntimeV5Reader == nil {
		t.Fatalf("complete Review root omitted a route: %+v", root)
	}
}

func TestRootV1RejectsTypedNilRouteDependency(t *testing.T) {
	deps := validRootDependenciesV1()
	var invocation *autoInvocationV1
	deps.AutoInvocation = invocation
	if _, err := production.NewRootV1(deps); !core.HasReason(err, core.ReasonComponentMissing) {
		t.Fatalf("typed-nil Auto invocation accepted: %v", err)
	}
}

func TestRootV1RoutesShareTheOwnerStoreAndFailClosedAtTheirPublicBoundaries(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	deps := validRootDependenciesV1()
	store, err := reviewmemory.NewStoreWithClockV1(func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	deps.Store = store
	deps.Clock = func() time.Time { return now }
	root, err := production.NewRootV1(deps)
	if err != nil {
		t.Fatal(err)
	}

	rubric := testkit.Rubric(now, "tenant-root")
	if _, err = root.Service.PublishRubricV1(context.Background(), reviewport.PublishRubricMutationV1{Next: rubric}); err != nil {
		t.Fatalf("base Service route could not mutate the shared Review Owner store: %v", err)
	}
	if _, err = root.Service.InspectRubricCurrentV1(context.Background(), rubric.TenantID, rubric.ExactRef()); err != nil {
		t.Fatalf("base Service route could not read its shared Review Owner fact: %v", err)
	}

	checks := []struct {
		name string
		call func() error
	}{
		{"auto-reviewer", func() error {
			_, err := root.AutoReviewer.RunV1(context.Background(), autoreviewer.RunCommandV1{})
			return err
		}},
		{"auto-attestation", func() error {
			_, err := root.AutoAttestation.RecordV1(context.Background(), autoattestation.RecordCommandV1{})
			return err
		}},
		{"human-open", func() error {
			_, err := root.HumanOwner.OpenPanelV2(context.Background(), reviewport.CreateHumanPanelMutationV2{})
			return err
		}},
		{"human-claim", func() error {
			_, err := root.HumanClaims.ClaimAssignmentV2(context.Background(), reviewport.ClaimHumanAssignmentMutationV2{}, reviewport.HumanOrganizationCurrentRequestV2{})
			return err
		}},
		{"human-service", func() error {
			_, err := root.HumanService.OpenPanelV2(context.Background(), reviewport.CreateHumanPanelMutationV2{})
			return err
		}},
		{"bypass", func() error {
			_, err := root.BypassOwner.CreateV1(context.Background(), reviewport.CreateBypassDecisionMutationV1{})
			return err
		}},
		{"decision-worker", func() error {
			_, _, err := root.Decision.Worker.RunOnceV1(context.Background(), "", "", 1)
			return err
		}},
		{"runtime-v5", func() error {
			_, err := root.RuntimeV5Reader.InspectOperationReviewCurrentV5(context.Background(), runtimeports.OperationReviewCurrentRequestV5{})
			return err
		}},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			if err := check.call(); err == nil {
				t.Fatal("zero/unknown input must fail closed through the wired route")
			}
		})
	}
}

var (
	_ reviewport.AutoReviewerInvocationPortV1     = (*autoInvocationV1)(nil)
	_ reviewport.ReviewerContextCurrentReaderV1   = (*reviewerContextV1)(nil)
	_ reviewport.AutoReviewerOutputSchemaReaderV1 = (*outputSchemaV1)(nil)
	_ multisigowner.ExternalCurrentCutV2          = (*humanExternalV2)(nil)
	_ reviewport.HumanOrganizationCurrentReaderV2 = (*organizationCurrentV2)(nil)
	_ bypassowner.ExternalCurrentCutV1            = (*bypassExternalV1)(nil)
)
