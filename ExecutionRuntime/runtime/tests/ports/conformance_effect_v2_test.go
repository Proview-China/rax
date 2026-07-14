package ports_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestPermitVerifierConformanceReceiptNeverClaimsProviderOrDomainCommit(t *testing.T) {
	t.Parallel()
	now := time.Unix(30_000, 0)
	intent, permit, fence, current, set := effectPortFixtureV2(t, now)
	permitDigest, err := permit.DigestV2()
	if err != nil {
		t.Fatal(err)
	}
	fact := control.DispatchPermitFactV2{Permit: permit, PermitDigest: permitDigest, Fence: fence, State: control.DispatchPermitBegun, Revision: 2, EffectFactRevision: 3, BegunUnixNano: now.UnixNano()}
	verifier := staticPermitVerifierV2{receipt: ports.EnforcementReceiptV2{ContractVersion: ports.EffectContractVersionV2, PermitID: permit.ID, PermitRevision: permit.Revision, AttemptID: permit.AttemptID, PermitDigest: permitDigest, Verifier: permit.EnforcementPoint, ValidatedAt: now.UnixNano()}}
	scopeReader := staticScopeFactReaderV2{fact: portCurrentScopeFactV2(t, intent, permit.CapabilityGrantDigest, now)}
	bindingReader := staticConformanceBindingPortV2{set: set}
	caseV2 := governedPermitVerifierCaseV2(t, now, conformance.PermitVerifierCaseV2{Verifier: verifier, Permit: fact, Intent: intent, Current: current, Bindings: bindingReader, CurrentScopes: scopeReader, Credentials: staticCredentialFactReaderV2{}, Clock: func() time.Time { return now }})
	report, err := conformance.CheckPermitVerifierV2(context.Background(), caseV2)
	if err != nil {
		t.Fatal(err)
	}
	if !report.BegunFactVerified || !report.CurrentFactsVerified || report.ProviderOutcomeAuthoritative || report.DomainCommitEligible {
		t.Fatalf("verifier receipt must remain enforcement evidence only: %+v", report)
	}
	verifier.receipt.AttemptID = "another-attempt"
	caseV2.Verifier = verifier
	if _, err := conformance.CheckPermitVerifierV2(context.Background(), caseV2); !core.HasReason(err, core.ReasonEvidenceConflict) {
		t.Fatalf("verifier cannot substitute another attempt receipt: %v", err)
	}
	verifier.receipt.AttemptID = permit.AttemptID
	verifier.receipt.Verifier.ManifestDigest = portEffectDigestV2(t, "unbound-verifier")
	caseV2.Verifier = verifier
	if _, err := conformance.CheckPermitVerifierV2(context.Background(), caseV2); !core.HasReason(err, core.ReasonEvidenceConflict) {
		t.Fatalf("unbound verifier identity cannot mint enforcement evidence: %v", err)
	}
	for _, drift := range []struct {
		name   string
		mutate func(*control.BindingSetFactV2)
	}{
		{name: "manifest", mutate: func(set *control.BindingSetFactV2) {
			set.Members[0].ManifestDigest = portEffectDigestV2(t, "manifest-drift")
		}},
		{name: "artifact", mutate: func(set *control.BindingSetFactV2) {
			set.Members[0].ArtifactDigest = portEffectDigestV2(t, "artifact-drift")
		}},
		{name: "capability", mutate: func(set *control.BindingSetFactV2) { set.Members[0].Grants[0].Capability = "custom.vendor/other" }},
	} {
		t.Run("binding_"+drift.name+"_drift", func(t *testing.T) {
			drifted := set
			drifted.Members = append([]control.BindingMemberV2{}, set.Members...)
			drifted.Members[0].Grants = append([]ports.CapabilityGrantV2{}, set.Members[0].Grants...)
			drift.mutate(&drifted)
			caseV2 := governedPermitVerifierCaseV2(t, now, conformance.PermitVerifierCaseV2{Verifier: staticPermitVerifierV2{receipt: ports.EnforcementReceiptV2{ContractVersion: ports.EffectContractVersionV2, PermitID: permit.ID, PermitRevision: permit.Revision, AttemptID: permit.AttemptID, PermitDigest: permitDigest, Verifier: permit.EnforcementPoint, ValidatedAt: now.UnixNano()}}, Permit: fact, Intent: intent, Current: current, Bindings: staticConformanceBindingPortV2{set: drifted}, CurrentScopes: scopeReader, Credentials: staticCredentialFactReaderV2{}, Clock: func() time.Time { return now }})
			if _, err := conformance.CheckPermitVerifierV2(context.Background(), caseV2); err == nil {
				t.Fatal("binding drift must fail before verifier or provider reach")
			}
		})
	}
}

func TestPermitVerifierConformanceRereadsAllGovernanceBeforeProviderReach(t *testing.T) {
	t.Parallel()
	now := time.Unix(30_900, 0)
	for _, testCase := range []struct {
		name   string
		reason core.ReasonCode
		mutate func(*conformance.PermitVerifierCaseV2)
	}{
		{name: "authority_revoked", reason: core.ReasonEffectAuthorizationMissing, mutate: func(c *conformance.PermitVerifierCaseV2) {
			reader := c.Authority.(staticAuthorityFactReaderV2)
			reader.fact.State = ports.AuthorityFactRevoked
			c.Authority = reader
		}},
		{name: "review_revoked", reason: core.ReasonReviewVerdictStale, mutate: func(c *conformance.PermitVerifierCaseV2) {
			reader := c.Review.(staticReviewFactReaderV2)
			reader.fact.Decision = ports.ReviewDecisionRejected
			c.Review = reader
		}},
		{name: "budget_consumed", reason: core.ReasonBudgetBindingStale, mutate: func(c *conformance.PermitVerifierCaseV2) {
			reader := c.Budgets.(staticBudgetFactReaderV2)
			reader.fact.State = control.BudgetFactConsumed
			c.Budgets = reader
		}},
		{name: "policy_revoked", reason: core.ReasonEffectAuthorizationMissing, mutate: func(c *conformance.PermitVerifierCaseV2) {
			reader := c.Policies.(staticPolicyFactReaderV2)
			reader.fact.Active = false
			c.Policies = reader
		}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			intent, permit, fence, current, set := effectPortFixtureV2(t, now)
			permitDigest, err := permit.DigestV2()
			if err != nil {
				t.Fatal(err)
			}
			fact := control.DispatchPermitFactV2{Permit: permit, PermitDigest: permitDigest, Fence: fence, State: control.DispatchPermitBegun, Revision: 2, EffectFactRevision: 3, BegunUnixNano: now.UnixNano()}
			calls := &atomic.Int32{}
			caseV2 := governedPermitVerifierCaseV2(t, now, conformance.PermitVerifierCaseV2{Verifier: countingPermitVerifierV2{calls: calls, receipt: ports.EnforcementReceiptV2{ContractVersion: ports.EffectContractVersionV2, PermitID: permit.ID, PermitRevision: permit.Revision, AttemptID: permit.AttemptID, PermitDigest: permitDigest, Verifier: permit.EnforcementPoint, ValidatedAt: now.UnixNano()}}, Permit: fact, Intent: intent, Current: current, Bindings: staticConformanceBindingPortV2{set: set}, CurrentScopes: staticScopeFactReaderV2{fact: portCurrentScopeFactV2(t, intent, permit.CapabilityGrantDigest, now)}, Credentials: staticCredentialFactReaderV2{}, Clock: func() time.Time { return now }})
			testCase.mutate(&caseV2)
			if _, err := conformance.CheckPermitVerifierV2(context.Background(), caseV2); !core.HasReason(err, testCase.reason) {
				t.Fatalf("execution point must fail closed on current fact drift: %v", err)
			}
			if calls.Load() != 0 {
				t.Fatalf("provider verifier was reached %d times", calls.Load())
			}
		})
	}
}

func TestPermitVerifierConformanceRejectsRevokedConditionalSatisfactionBeforeProvider(t *testing.T) {
	t.Parallel()
	now := time.Unix(31_000, 0)
	intent, permit, fence, current, set := effectPortFixtureV2(t, now)
	permit.ReviewSatisfactionRef = "satisfaction-1"
	permit.ReviewSatisfactionDigest = portEffectDigestV2(t, "satisfaction-1")
	permit.ReviewSatisfactionRevision = 2
	current.ReviewSatisfactionRef = permit.ReviewSatisfactionRef
	current.ReviewSatisfactionDigest = permit.ReviewSatisfactionDigest
	current.ReviewSatisfactionRevision = permit.ReviewSatisfactionRevision
	permitDigest, err := permit.DigestV2()
	if err != nil {
		t.Fatal(err)
	}
	fact := control.DispatchPermitFactV2{Permit: permit, PermitDigest: permitDigest, Fence: fence, State: control.DispatchPermitBegun, Revision: 2, EffectFactRevision: 3, BegunUnixNano: now.UnixNano()}
	calls := &atomic.Int32{}
	caseV2 := governedPermitVerifierCaseV2(t, now, conformance.PermitVerifierCaseV2{Verifier: countingPermitVerifierV2{calls: calls}, Permit: fact, Intent: intent, Current: current, Bindings: staticConformanceBindingPortV2{set: set}, CurrentScopes: staticScopeFactReaderV2{fact: portCurrentScopeFactV2(t, intent, permit.CapabilityGrantDigest, now)}, Credentials: staticCredentialFactReaderV2{}, Clock: func() time.Time { return now }})
	caseV2.Review = failingReviewFactReaderV2{err: core.NewError(core.ErrorForbidden, core.ReasonReviewConditionUnsatisfied, "condition satisfaction revoked after Begin")}
	if _, err := conformance.CheckPermitVerifierV2(context.Background(), caseV2); !core.HasReason(err, core.ReasonReviewConditionUnsatisfied) {
		t.Fatalf("revoked satisfaction must fail at the actual execution point: %v", err)
	}
	if calls.Load() != 0 {
		t.Fatalf("provider verifier was reached %d times", calls.Load())
	}
}

func TestPermitVerifierConformanceRereadsCredentialAtExecutionPoint(t *testing.T) {
	t.Parallel()
	now := time.Unix(30_500, 0)
	intent, permit, fence, current, set := effectPortFixtureV2(t, now)
	credential := ports.CredentialLeaseRefV2{Ref: "credential-1", Class: "custom/model-provider", ScopeDigest: portEffectDigestV2(t, "credential-scope"), Epoch: 1}
	credentialFact := ports.CredentialLeaseFactV2{Ref: credential.Ref, Class: credential.Class, ScopeDigest: credential.ScopeDigest, Epoch: credential.Epoch, Active: true, ExpiresUnixNano: now.Add(5 * time.Second).UnixNano()}
	intent.CredentialLeases = []ports.CredentialLeaseRefV2{credential}
	policy := portPolicyFactV2(t, intent, now)
	intent.Policy = ports.DispatchPolicyBindingRefV2{Ref: policy.Ref, Digest: policy.Digest, Revision: policy.Revision}
	intentDigest, err := intent.DigestV2()
	if err != nil {
		t.Fatal(err)
	}
	credentialDigest, err := ports.DigestCredentialLeaseFactsV2([]ports.CredentialLeaseFactV2{credentialFact})
	if err != nil {
		t.Fatal(err)
	}
	permit.IntentDigest = intentDigest
	permit.Policy = intent.Policy
	permit.CredentialGrantDigest = credentialDigest
	permit.ExpiresUnixNano = credentialFact.ExpiresUnixNano
	permitDigest, err := permit.DigestV2()
	if err != nil {
		t.Fatal(err)
	}
	fact := control.DispatchPermitFactV2{Permit: permit, PermitDigest: permitDigest, Fence: fence, State: control.DispatchPermitBegun, Revision: 2, EffectFactRevision: 3, BegunUnixNano: now.UnixNano()}
	current.CredentialGrantDigest = credentialDigest
	verifier := staticPermitVerifierV2{receipt: ports.EnforcementReceiptV2{ContractVersion: ports.EffectContractVersionV2, PermitID: permit.ID, PermitRevision: permit.Revision, AttemptID: permit.AttemptID, PermitDigest: permitDigest, Verifier: permit.EnforcementPoint, ValidatedAt: now.UnixNano()}}
	scopeReader := staticScopeFactReaderV2{fact: portCurrentScopeFactV2(t, intent, permit.CapabilityGrantDigest, now)}
	credentials := staticCredentialFactReaderV2{fact: credentialFact}
	check := func(clock time.Time, reader staticCredentialFactReaderV2) error {
		caseV2 := governedPermitVerifierCaseV2(t, now, conformance.PermitVerifierCaseV2{Verifier: verifier, Permit: fact, Intent: intent, Current: current, Bindings: staticConformanceBindingPortV2{set: set}, CurrentScopes: scopeReader, Credentials: reader, Clock: func() time.Time { return clock }})
		caseV2.Clock = func() time.Time { return clock }
		_, err := conformance.CheckPermitVerifierV2(context.Background(), caseV2)
		return err
	}
	if err := check(now, credentials); err != nil {
		t.Fatalf("current credential must pass execution-point verification: %v", err)
	}
	revoked := credentials
	revoked.fact.Active = false
	if err := check(now, revoked); !core.HasReason(err, core.ReasonCredentialLeaseMissing) {
		t.Fatalf("credential revoked after Begin must block before provider reach: %v", err)
	}
	if err := check(time.Unix(0, credentialFact.ExpiresUnixNano), credentials); !core.HasReason(err, core.ReasonCredentialLeaseMissing) {
		t.Fatalf("credential exact expiry after Begin must block before provider reach: %v", err)
	}
}

type staticScopeFactReaderV2 struct {
	fact ports.ExecutionScopeCurrentFactV2
}

type staticConformanceBindingPortV2 struct{ set control.BindingSetFactV2 }

func (s staticConformanceBindingPortV2) InspectBindingSet(context.Context, string) (control.BindingSetFactV2, error) {
	return s.set, nil
}
func (staticConformanceBindingPortV2) CreateBinding(context.Context, control.BindingFactV2) (control.BindingFactV2, error) {
	return control.BindingFactV2{}, core.NewError(core.ErrorInternal, core.ReasonComponentMissing, "unused")
}
func (staticConformanceBindingPortV2) InspectBinding(context.Context, string) (control.BindingFactV2, error) {
	return control.BindingFactV2{}, core.NewError(core.ErrorInternal, core.ReasonComponentMissing, "unused")
}
func (staticConformanceBindingPortV2) CompareAndSwapBinding(context.Context, control.BindingFactCASRequestV2) (control.BindingFactV2, error) {
	return control.BindingFactV2{}, core.NewError(core.ErrorInternal, core.ReasonComponentMissing, "unused")
}
func (staticConformanceBindingPortV2) CommitBindingSet(context.Context, control.CommitBindingSetRequestV2) (control.BindingSetFactV2, error) {
	return control.BindingSetFactV2{}, core.NewError(core.ErrorInternal, core.ReasonComponentMissing, "unused")
}
func (staticConformanceBindingPortV2) CompareAndSwapBindingSet(context.Context, control.BindingSetCASRequestV2) (control.BindingSetFactV2, error) {
	return control.BindingSetFactV2{}, core.NewError(core.ErrorInternal, core.ReasonComponentMissing, "unused")
}

func (s staticScopeFactReaderV2) InspectCurrentExecutionScope(context.Context, string) (ports.ExecutionScopeCurrentFactV2, error) {
	return s.fact, nil
}

type staticCredentialFactReaderV2 struct{ fact ports.CredentialLeaseFactV2 }

type staticReviewFactReaderV2 struct{ fact ports.DispatchReviewFactV2 }
type staticAuthorityFactReaderV2 struct{ fact ports.DispatchAuthorityFactV2 }
type staticPolicyFactReaderV2 struct{ fact ports.DispatchPolicyFactV2 }
type staticBudgetFactReaderV2 struct{ fact control.BudgetBindingFactV2 }
type failingReviewFactReaderV2 struct{ err error }
type countingPermitVerifierV2 struct {
	calls   *atomic.Int32
	receipt ports.EnforcementReceiptV2
}

func governedPermitVerifierCaseV2(t *testing.T, now time.Time, testCase conformance.PermitVerifierCaseV2) conformance.PermitVerifierCaseV2 {
	t.Helper()
	intent, permit, current := testCase.Intent, testCase.Permit.Permit, testCase.Current
	testCase.Authority = staticAuthorityFactReaderV2{fact: ports.DispatchAuthorityFactV2{Ref: intent.Authority.Ref, Digest: intent.Authority.Digest, Revision: intent.Authority.Revision, Scope: intent.Scope, ActionScopeDigest: intent.ActionScopeDigest, State: ports.AuthorityFactActive, ExpiresUnixNano: now.Add(time.Minute).UnixNano()}}
	subjectDigest, err := intent.ReviewSubjectDigestV2()
	if err != nil {
		t.Fatal(err)
	}
	testCase.Review = staticReviewFactReaderV2{fact: ports.DispatchReviewFactV2{Ref: intent.Review.Ref, Digest: intent.Review.Digest, Revision: intent.Review.Revision, IntentID: intent.ID, IntentRevision: intent.Revision, SubjectDigest: subjectDigest, CandidateDigest: intent.Review.Digest, VerdictDigest: permit.ReviewVerdictDigest, VerdictRevision: permit.ReviewVerdictRevision, PayloadDigest: intent.Payload.ContentDigest, PayloadRevision: intent.PayloadRevision, ScopeDigest: intent.ActionScopeDigest, PolicyDigest: intent.Review.PolicyDigest, PolicyDecisionRef: "review-policy-decision-1", ActorAuthorityDigest: intent.Authority.Digest, ReviewerAuthorityDigest: portEffectDigestV2(t, "reviewer-authority"), EvidenceDigest: portEffectDigestV2(t, "review-evidence"), Decision: ports.ReviewDecisionAccepted, ExpiresUnixNano: now.Add(time.Minute).UnixNano(), SatisfactionRef: current.ReviewSatisfactionRef, SatisfactionDigest: current.ReviewSatisfactionDigest, SatisfactionRevision: current.ReviewSatisfactionRevision}}
	testCase.Budgets = staticBudgetFactReaderV2{fact: portBudgetFactV2(t, intent, now)}
	testCase.Policies = staticPolicyFactReaderV2{fact: portPolicyFactV2(t, intent, now)}
	return testCase
}

func (s staticReviewFactReaderV2) InspectDispatchReview(context.Context, string) (ports.DispatchReviewFactV2, error) {
	return s.fact, nil
}
func (s failingReviewFactReaderV2) InspectDispatchReview(context.Context, string) (ports.DispatchReviewFactV2, error) {
	return ports.DispatchReviewFactV2{}, s.err
}
func (s countingPermitVerifierV2) VerifyDispatchPermit(context.Context, ports.PermitVerificationRequestV2) (ports.EnforcementReceiptV2, error) {
	s.calls.Add(1)
	return s.receipt, nil
}
func (s staticAuthorityFactReaderV2) InspectDispatchAuthority(context.Context, string) (ports.DispatchAuthorityFactV2, error) {
	return s.fact, nil
}
func (s staticPolicyFactReaderV2) InspectDispatchPolicy(context.Context, string) (ports.DispatchPolicyFactV2, error) {
	return s.fact, nil
}
func (s staticBudgetFactReaderV2) InspectBudgetBinding(context.Context, string) (control.BudgetBindingFactV2, error) {
	return s.fact, nil
}
func (s staticBudgetFactReaderV2) CreateBudgetBinding(context.Context, control.BudgetBindingFactV2) (control.BudgetBindingFactV2, error) {
	return control.BudgetBindingFactV2{}, core.NewError(core.ErrorInternal, core.ReasonComponentMissing, "unused")
}
func (s staticBudgetFactReaderV2) CompareAndSwapBudgetBinding(context.Context, control.BudgetFactCASRequestV2) (control.BudgetBindingFactV2, error) {
	return control.BudgetBindingFactV2{}, core.NewError(core.ErrorInternal, core.ReasonComponentMissing, "unused")
}

func (s staticCredentialFactReaderV2) InspectCredentialLease(_ context.Context, ref string) (ports.CredentialLeaseFactV2, error) {
	if s.fact.Ref != ref {
		return ports.CredentialLeaseFactV2{}, core.NewError(core.ErrorNotFound, core.ReasonCredentialLeaseMissing, "credential not found")
	}
	return s.fact, nil
}

type staticPermitVerifierV2 struct{ receipt ports.EnforcementReceiptV2 }

func (s staticPermitVerifierV2) VerifyDispatchPermit(context.Context, ports.PermitVerificationRequestV2) (ports.EnforcementReceiptV2, error) {
	return s.receipt, nil
}
