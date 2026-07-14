package conformance

import (
	"context"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type PermitVerifierCaseV2 struct {
	Verifier      ports.PermitVerifierPortV2
	Permit        control.DispatchPermitFactV2
	Intent        ports.EffectIntentV2
	Fence         core.ExecutionFence
	Current       ports.DispatchCurrentFactsV2
	Bindings      control.BindingFactPortV2
	CurrentScopes ports.ExecutionScopeFactReaderV2
	Credentials   ports.CredentialLeaseFactReaderV2
	Authority     ports.AuthorityFactReaderV2
	Review        ports.ReviewFactReaderV2
	Budgets       control.BudgetFactPortV2
	Policies      ports.DispatchPolicyFactReaderV2
	Clock         func() time.Time
}

type PermitVerifierReportV2 struct {
	BegunFactVerified            bool                       `json:"begun_fact_verified"`
	CurrentFactsVerified         bool                       `json:"current_facts_verified"`
	Receipt                      ports.EnforcementReceiptV2 `json:"receipt"`
	ProviderOutcomeAuthoritative bool                       `json:"provider_outcome_authoritative"`
	DomainCommitEligible         bool                       `json:"domain_commit_eligible"`
}

// CheckPermitVerifierV2 checks the actual execution-point adapter without
// upgrading its receipt into evidence that the provider was reached or that a
// domain operation committed. The caller must CAS the receipt onto the begun
// Permit Fact before touching the provider.
func CheckPermitVerifierV2(ctx context.Context, testCase PermitVerifierCaseV2) (PermitVerifierReportV2, error) {
	if testCase.Verifier == nil || testCase.Bindings == nil || testCase.CurrentScopes == nil || testCase.Credentials == nil || testCase.Authority == nil || testCase.Review == nil || testCase.Budgets == nil || testCase.Policies == nil || testCase.Clock == nil {
		return PermitVerifierReportV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "permit verifier, binding, current-scope, credential, authority, review, budget and policy readers, and injected clock are required")
	}
	if err := testCase.Permit.Validate(); err != nil {
		return PermitVerifierReportV2{}, err
	}
	now := testCase.Clock()
	set, err := testCase.Bindings.InspectBindingSet(ctx, testCase.Permit.Permit.EnforcementPoint.BindingSetID)
	if err != nil || set.State != control.BindingSetActive || set.Revision != testCase.Permit.Permit.EnforcementPoint.BindingSetRevision || !now.Before(time.Unix(0, set.ExpiresUnixNano)) {
		return PermitVerifierReportV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonProviderBindingStale, "execution-point binding set is not current")
	}
	if err := validateEnforcementBindingV2(set, testCase.Permit.Permit, now); err != nil {
		return PermitVerifierReportV2{}, err
	}
	capabilityDigest, err := set.CapabilityGrantDigestV2()
	if err != nil || capabilityDigest != testCase.Permit.Permit.CapabilityGrantDigest {
		return PermitVerifierReportV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonProviderBindingStale, "execution-point capability grant set drifted")
	}
	currentScope, err := testCase.CurrentScopes.InspectCurrentExecutionScope(ctx, testCase.Permit.Permit.CurrentScope.Ref)
	if err != nil {
		return PermitVerifierReportV2{}, err
	}
	if err := currentScope.ValidateCurrent(testCase.Permit.Permit.CurrentScope, testCase.Intent, testCase.Permit.Permit.CapabilityGrantDigest, now); err != nil {
		return PermitVerifierReportV2{}, err
	}
	credentials := make([]ports.CredentialLeaseFactV2, 0, len(testCase.Intent.CredentialLeases))
	for _, expected := range testCase.Intent.CredentialLeases {
		fact, err := testCase.Credentials.InspectCredentialLease(ctx, expected.Ref)
		if err != nil || fact.ValidateCurrent(expected, now) != nil {
			return PermitVerifierReportV2{}, core.NewError(core.ErrorForbidden, core.ReasonCredentialLeaseMissing, "execution-point credential fact is not current")
		}
		credentials = append(credentials, fact)
	}
	credentialDigest, err := ports.DigestCredentialLeaseFactsV2(credentials)
	if err != nil || credentialDigest != testCase.Permit.Permit.CredentialGrantDigest {
		return PermitVerifierReportV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonCredentialLeaseMissing, "execution-point credential fact set drifted")
	}
	authority, err := testCase.Authority.InspectDispatchAuthority(ctx, testCase.Intent.Authority.Ref)
	if err != nil || authority.ValidateCurrent(testCase.Intent.Authority, testCase.Intent.Scope, testCase.Intent.ActionScopeDigest, now) != nil {
		return PermitVerifierReportV2{}, core.NewError(core.ErrorForbidden, core.ReasonEffectAuthorizationMissing, "execution-point authority fact is not current")
	}
	review, err := testCase.Review.InspectDispatchReview(ctx, testCase.Intent.Review.Ref)
	if err != nil {
		return PermitVerifierReportV2{}, err
	}
	if err := review.ValidateCurrent(testCase.Intent.Review, testCase.Intent, now); err != nil {
		return PermitVerifierReportV2{}, err
	}
	budget, err := testCase.Budgets.InspectBudgetBinding(ctx, testCase.Intent.Budget.Ref)
	if err != nil || budget.ValidateCurrent(testCase.Intent.Budget, testCase.Intent, now) != nil {
		return PermitVerifierReportV2{}, core.NewError(core.ErrorForbidden, core.ReasonBudgetBindingStale, "execution-point budget fact is not current")
	}
	policy, err := testCase.Policies.InspectDispatchPolicy(ctx, testCase.Intent.Policy.Ref)
	remaining := time.Duration(testCase.Permit.Permit.ExpiresUnixNano - now.UnixNano())
	if err != nil || policy.ValidateCurrent(testCase.Intent.Policy, testCase.Intent, remaining, now) != nil {
		return PermitVerifierReportV2{}, core.NewError(core.ErrorForbidden, core.ReasonEffectAuthorizationMissing, "execution-point dispatch policy is not current")
	}
	current := testCase.Current
	current.Scope = currentScope.Scope
	current.Provider = testCase.Permit.Permit.Provider
	current.EnforcementPoint = testCase.Permit.Permit.EnforcementPoint
	current.CurrentScope = testCase.Permit.Permit.CurrentScope
	current.CapabilityGrantDigest = currentScope.CapabilityGrantDigest
	current.CredentialGrantDigest = credentialDigest
	current.Authority = testCase.Intent.Authority
	current.Review = testCase.Intent.Review
	current.ReviewVerdictDigest = review.VerdictDigest
	current.ReviewVerdictRevision = review.VerdictRevision
	current.ReviewSatisfactionRef = review.SatisfactionRef
	current.ReviewSatisfactionDigest = review.SatisfactionDigest
	current.ReviewSatisfactionRevision = review.SatisfactionRevision
	current.Budget = testCase.Intent.Budget
	current.Policy = testCase.Intent.Policy
	request := ports.PermitVerificationRequestV2{Permit: testCase.Permit.Permit, PermitFactRevision: testCase.Permit.Revision, PermitFactState: string(testCase.Permit.State), Intent: testCase.Intent, Fence: testCase.Permit.Fence, Current: current}
	if err := request.Validate(now); err != nil {
		return PermitVerifierReportV2{}, err
	}
	receipt, err := testCase.Verifier.VerifyDispatchPermit(ctx, request)
	if err != nil {
		return PermitVerifierReportV2{}, err
	}
	if err := receipt.Validate(); err != nil {
		return PermitVerifierReportV2{}, err
	}
	if receipt.PermitID != testCase.Permit.Permit.ID || receipt.PermitRevision != testCase.Permit.Permit.Revision || receipt.AttemptID != testCase.Permit.Permit.AttemptID || receipt.PermitDigest != testCase.Permit.PermitDigest || receipt.Verifier != testCase.Permit.Permit.EnforcementPoint || receipt.ValidatedAt < testCase.Permit.BegunUnixNano || receipt.ValidatedAt > now.UnixNano() {
		return PermitVerifierReportV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "verifier receipt drifted from the single begun attempt")
	}
	return PermitVerifierReportV2{BegunFactVerified: true, CurrentFactsVerified: true, Receipt: receipt, ProviderOutcomeAuthoritative: false, DomainCommitEligible: false}, nil
}

func validateEnforcementBindingV2(set control.BindingSetFactV2, permit ports.DispatchPermitV2, now time.Time) error {
	for _, member := range set.Members {
		if member.ComponentID != permit.EnforcementPoint.ComponentID {
			continue
		}
		if member.ManifestDigest != permit.EnforcementPoint.ManifestDigest || member.ArtifactDigest != permit.EnforcementPoint.ArtifactDigest {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonProviderBindingStale, "execution-point component artifact or manifest drifted")
		}
		for _, grant := range member.Grants {
			if grant.Capability == permit.EnforcementPoint.Capability && now.Before(time.Unix(0, grant.ExpiresUnixNano)) {
				return nil
			}
		}
		return core.NewError(core.ErrorCapabilityUnavailable, core.ReasonUnknownCapability, "execution-point capability grant is absent")
	}
	return core.NewError(core.ErrorCapabilityUnavailable, core.ReasonProviderBindingStale, "execution-point component is absent from the current binding set")
}
