package application_test

import (
	"context"
	"math"
	"testing"
	"time"

	application "github.com/Proview-China/rax/ExecutionRuntime/application"
	"github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func TestGovernedJournalCompletionV3RejectsInvalidPublicInputBeforeBackend(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	valid, _ := applicationFixtureV2(t, now, "user.prevalidation/completion", true)

	for _, testCase := range []struct {
		name      string
		plan      contract.WorkflowPlanV2
		attemptID string
	}{
		{name: "invalid-plan", plan: contract.WorkflowPlanV2{}, attemptID: "attempt"},
		{name: "empty-attempt", plan: valid.Plan, attemptID: ""},
		{name: "noncanonical-attempt", plan: valid.Plan, attemptID: " attempt"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			backendCalls := 0
			attempts := &completionAttemptBackendCounterV3{backendCalls: &backendCalls}
			journals := &completionJournalBackendCounterV3{backendCalls: &backendCalls}
			completion, err := application.NewGovernedJournalCompletionV3(application.GovernedJournalCompletionConfigV3{Attempts: attempts, Journals: journals, Clock: func() time.Time { return now }})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := completion.Complete(context.Background(), testCase.plan, testCase.attemptID); err == nil {
				t.Fatal("invalid completion request reached a backend")
			}
			if backendCalls != 0 {
				t.Fatalf("invalid completion request made %d backend calls", backendCalls)
			}
		})
	}
}

func TestRecoveryCoordinatorV2RejectsInvalidPublicRequestsBeforeBackend(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	bundle, _ := applicationFixtureV2(t, now, "user.prevalidation/recovery", true)
	validScope := bundle.Plan.Target
	policy := core.DigestBytes([]byte("recovery-policy"))

	listCases := []struct {
		name    string
		request applicationports.WorkflowJournalListRequestV2
	}{
		{name: "scope", request: applicationports.WorkflowJournalListRequestV2{Limit: 1}},
		{name: "cursor", request: applicationports.WorkflowJournalListRequestV2{Scope: validScope, AfterID: " cursor", Limit: 1}},
		{name: "zero-limit", request: applicationports.WorkflowJournalListRequestV2{Scope: validScope}},
		{name: "oversized-limit", request: applicationports.WorkflowJournalListRequestV2{Scope: validScope, Limit: 513}},
	}
	for _, testCase := range listCases {
		t.Run("list-"+testCase.name, func(t *testing.T) {
			coordinator, backendCalls := newRecoveryPrevalidationFixtureV2(t, now)
			if _, err := coordinator.ListRecoverableV2(context.Background(), testCase.request); err == nil {
				t.Fatal("invalid List request reached a backend")
			}
			if *backendCalls != 0 {
				t.Fatalf("invalid List request made %d backend calls", *backendCalls)
			}
		})
	}

	validAcquire := applicationports.WorkflowJournalClaimRequestV2{Scope: validScope, JournalID: "journal", OwnerID: "worker", PolicyDigest: policy, LeaseNanos: int64(time.Minute)}
	acquireCases := []struct {
		name   string
		mutate func(*applicationports.WorkflowJournalClaimRequestV2)
	}{
		{name: "scope", mutate: func(r *applicationports.WorkflowJournalClaimRequestV2) { r.Scope = core.ExecutionScope{} }},
		{name: "journal", mutate: func(r *applicationports.WorkflowJournalClaimRequestV2) { r.JournalID = "" }},
		{name: "owner", mutate: func(r *applicationports.WorkflowJournalClaimRequestV2) { r.OwnerID = " worker" }},
		{name: "policy", mutate: func(r *applicationports.WorkflowJournalClaimRequestV2) { r.PolicyDigest = "" }},
		{name: "ttl", mutate: func(r *applicationports.WorkflowJournalClaimRequestV2) { r.LeaseNanos = 0 }},
		{name: "ttl-overflow", mutate: func(r *applicationports.WorkflowJournalClaimRequestV2) { r.LeaseNanos = math.MaxInt64 }},
	}
	for _, testCase := range acquireCases {
		t.Run("acquire-"+testCase.name, func(t *testing.T) {
			request := validAcquire
			testCase.mutate(&request)
			coordinator, backendCalls := newRecoveryPrevalidationFixtureV2(t, now)
			if _, err := coordinator.AcquireV2(context.Background(), request); err == nil {
				t.Fatal("invalid Acquire request reached a backend")
			}
			if *backendCalls != 0 {
				t.Fatalf("invalid Acquire request made %d backend calls", *backendCalls)
			}
		})
	}

	validRelease := applicationports.WorkflowJournalReleaseRequestV2{Scope: validScope, JournalID: "journal", OwnerID: "worker", Epoch: 1, ExpectedRevision: 1}
	releaseCases := []struct {
		name   string
		mutate func(*applicationports.WorkflowJournalReleaseRequestV2)
	}{
		{name: "scope", mutate: func(r *applicationports.WorkflowJournalReleaseRequestV2) { r.Scope = core.ExecutionScope{} }},
		{name: "journal", mutate: func(r *applicationports.WorkflowJournalReleaseRequestV2) { r.JournalID = "" }},
		{name: "owner", mutate: func(r *applicationports.WorkflowJournalReleaseRequestV2) { r.OwnerID = "worker " }},
		{name: "epoch", mutate: func(r *applicationports.WorkflowJournalReleaseRequestV2) { r.Epoch = 0 }},
		{name: "revision", mutate: func(r *applicationports.WorkflowJournalReleaseRequestV2) { r.ExpectedRevision = 0 }},
	}
	for _, testCase := range releaseCases {
		t.Run("release-"+testCase.name, func(t *testing.T) {
			request := validRelease
			testCase.mutate(&request)
			coordinator, backendCalls := newRecoveryPrevalidationFixtureV2(t, now)
			if _, err := coordinator.ReleaseV2(context.Background(), request); err == nil {
				t.Fatal("invalid Release request reached a backend")
			}
			if *backendCalls != 0 {
				t.Fatalf("invalid Release request made %d backend calls", *backendCalls)
			}
		})
	}
}

type completionAttemptBackendCounterV3 struct {
	applicationports.GovernedOperationAttemptFactPortV3
	backendCalls *int
}

func (b *completionAttemptBackendCounterV3) InspectGovernedOperationAttemptV3(context.Context, core.ExecutionScope, string) (contract.GovernedOperationAttemptFactV3, error) {
	*b.backendCalls++
	return contract.GovernedOperationAttemptFactV3{}, core.NewError(core.ErrorInternal, core.ReasonEvidenceUnavailable, "unexpected completion backend call")
}

type completionJournalBackendCounterV3 struct {
	applicationports.WorkflowJournalFactPortV2
	backendCalls *int
}

func (b *completionJournalBackendCounterV3) InspectWorkflowJournalV2(context.Context, core.ExecutionScope, string) (contract.WorkflowJournalV2, error) {
	*b.backendCalls++
	return contract.WorkflowJournalV2{}, core.NewError(core.ErrorInternal, core.ReasonEvidenceUnavailable, "unexpected completion backend call")
}

type recoveryBackendCounterV2 struct {
	applicationports.WorkflowJournalRecoveryPortV2
	backendCalls *int
}

func (b *recoveryBackendCounterV2) ListWorkflowJournalsV2(context.Context, core.ExecutionScope, string, uint16) ([]contract.WorkflowJournalV2, error) {
	*b.backendCalls++
	return nil, core.NewError(core.ErrorInternal, core.ReasonEvidenceUnavailable, "unexpected recovery backend call")
}

func (b *recoveryBackendCounterV2) ClaimWorkflowJournalV2(context.Context, applicationports.WorkflowJournalClaimRequestV2) (applicationports.WorkflowJournalClaimV2, error) {
	*b.backendCalls++
	return applicationports.WorkflowJournalClaimV2{}, core.NewError(core.ErrorInternal, core.ReasonEvidenceUnavailable, "unexpected recovery backend call")
}

func (b *recoveryBackendCounterV2) InspectWorkflowJournalClaimV2(context.Context, core.ExecutionScope, string) (applicationports.WorkflowJournalClaimV2, error) {
	*b.backendCalls++
	return applicationports.WorkflowJournalClaimV2{}, core.NewError(core.ErrorInternal, core.ReasonEvidenceUnavailable, "unexpected recovery backend call")
}

func (b *recoveryBackendCounterV2) ReleaseWorkflowJournalClaimV2(context.Context, applicationports.WorkflowJournalReleaseRequestV2) (applicationports.WorkflowJournalClaimV2, error) {
	*b.backendCalls++
	return applicationports.WorkflowJournalClaimV2{}, core.NewError(core.ErrorInternal, core.ReasonEvidenceUnavailable, "unexpected recovery backend call")
}

type recoveryJournalBackendCounterV2 struct {
	applicationports.WorkflowJournalFactPortV2
	backendCalls *int
}

func (b *recoveryJournalBackendCounterV2) InspectWorkflowJournalV2(context.Context, core.ExecutionScope, string) (contract.WorkflowJournalV2, error) {
	*b.backendCalls++
	return contract.WorkflowJournalV2{}, core.NewError(core.ErrorInternal, core.ReasonEvidenceUnavailable, "unexpected recovery journal backend call")
}

func newRecoveryPrevalidationFixtureV2(t *testing.T, now time.Time) (*application.RecoveryCoordinatorV2, *int) {
	t.Helper()
	backendCalls := 0
	recovery := &recoveryBackendCounterV2{backendCalls: &backendCalls}
	facts := &recoveryJournalBackendCounterV2{backendCalls: &backendCalls}
	coordinator, err := application.NewRecoveryCoordinatorV2(application.RecoveryCoordinatorConfigV2{Recovery: recovery, Facts: facts, Clock: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	return coordinator, &backendCalls
}
