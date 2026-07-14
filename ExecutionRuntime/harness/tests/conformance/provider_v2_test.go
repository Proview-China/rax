package conformance_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type customProviderV2 struct {
	mu                  sync.Mutex
	prepared            runtimeports.ProviderPreparationAttestationV2
	observation         runtimeports.ProviderAttemptObservationV2
	prepareCalls        int
	executeCalls        int
	driftConcurrentTime bool
}

func (p *customProviderV2) Prepare(_ context.Context, request runtimeports.PrepareGovernedExecutionRequestV2) (runtimeports.ProviderPreparationAttestationV2, error) {
	if err := request.Validate(); err != nil {
		return runtimeports.ProviderPreparationAttestationV2{}, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.prepareCalls++
	result := p.prepared
	if p.driftConcurrentTime && p.prepareCalls > 1 {
		result.ObservedUnixNano++
	}
	return result, nil
}

func (p *customProviderV2) InspectPrepared(_ context.Context, request runtimeports.InspectPreparedProviderRequestV2) (runtimeports.ProviderPreparationAttestationV2, error) {
	if err := request.Validate(); err != nil {
		return runtimeports.ProviderPreparationAttestationV2{}, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.prepared.Prepared.ID != request.PreparedAttemptID || p.prepared.Prepared.PermitID != request.PermitID || p.prepared.Prepared.AttemptID != request.AttemptID {
		return runtimeports.ProviderPreparationAttestationV2{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "prepared attempt not found")
	}
	return p.prepared, nil
}

func (p *customProviderV2) ExecutePrepared(_ context.Context, request runtimeports.ExecutePreparedRequestV2) (runtimeports.ProviderAttemptObservationV2, error) {
	if err := request.Validate(); err != nil {
		return runtimeports.ProviderAttemptObservationV2{}, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.executeCalls++
	return p.observation, nil
}

func (p *customProviderV2) InspectLocalAttempt(_ context.Context, request runtimeports.InspectLocalProviderAttemptRequestV2) (runtimeports.ProviderAttemptObservationV2, error) {
	if err := request.Validate(); err != nil {
		return runtimeports.ProviderAttemptObservationV2{}, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.observation.Prepared != request.Prepared || p.observation.Delegation != request.Delegation {
		return runtimeports.ProviderAttemptObservationV2{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "provider attempt not found")
	}
	return p.observation, nil
}

func TestGovernedProviderV2ConformanceAcceptsNamespacedCustomProviderWithoutGrantingAuthority(t *testing.T) {
	now := time.Unix(2_000_000_000, 0)
	prepare, prepared, execute, observation := testkit.GovernedProviderFixtureV2(now)
	provider := &customProviderV2{prepared: prepared, observation: observation}
	report, err := conformance.CheckGovernedProviderV2(context.Background(), conformance.GovernedProviderCaseV2{
		Provider: provider, Prepare: prepare, Execute: execute, Now: now, PrepareParallelism: 64,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !report.PrepareCreateOnce || !report.PrepareConcurrentExact || !report.PreparedInspectExact || !report.ExecuteSingleCallExact || !report.LocalAttemptInspectExact || !report.CertificationCandidate {
		t.Fatalf("conformance evidence is incomplete: %#v", report)
	}
	if report.ProductionClaimEligible || report.BindingEligible || report.DispatchEligible || report.SettlementEligible || report.CompletionEligible || report.LocalityAttested || report.ProductionDurabilityProven {
		t.Fatalf("provider conformance self-granted authority: %#v", report)
	}
	if provider.prepareCalls != 64 || provider.executeCalls != 1 {
		t.Fatalf("conformance changed call cardinality: prepare=%d execute=%d", provider.prepareCalls, provider.executeCalls)
	}
}

func TestGovernedProviderV2ConformanceRejectsConcurrentPrepareIdentityDrift(t *testing.T) {
	now := time.Unix(2_000_000_000, 0)
	prepare, prepared, execute, observation := testkit.GovernedProviderFixtureV2(now)
	provider := &customProviderV2{prepared: prepared, observation: observation, driftConcurrentTime: true}
	if report, err := conformance.CheckGovernedProviderV2(context.Background(), conformance.GovernedProviderCaseV2{
		Provider: provider, Prepare: prepare, Execute: execute, Now: now.Add(time.Second), PrepareParallelism: 32,
	}); err == nil || report.CertificationCandidate {
		t.Fatalf("concurrent create-once drift was certified: report=%#v err=%v", report, err)
	}
	if provider.executeCalls != 0 {
		t.Fatal("Execute was reached after Prepare conformance failed")
	}
}

func TestGovernedProviderV2ConformanceRejectsInspectSubstitution(t *testing.T) {
	now := time.Unix(2_000_000_000, 0)
	prepare, prepared, execute, observation := testkit.GovernedProviderFixtureV2(now)
	forged := prepared
	forged.ObservedUnixNano++
	provider := &customProviderV2{prepared: prepared, observation: observation}
	providerForInspect := &inspectSubstitutionProviderV2{customProviderV2: provider, forgedPrepared: forged}
	if report, err := conformance.CheckGovernedProviderV2(context.Background(), conformance.GovernedProviderCaseV2{
		Provider: providerForInspect, Prepare: prepare, Execute: execute, Now: now, PrepareParallelism: 8,
	}); err == nil || report.CertificationCandidate {
		t.Fatalf("InspectPrepared substitution was certified: report=%#v err=%v", report, err)
	}
}

func TestGovernedProviderV2ConformanceRejectsLocalAttemptInspectSubstitution(t *testing.T) {
	now := time.Unix(2_000_000_000, 0)
	prepare, prepared, execute, observation := testkit.GovernedProviderFixtureV2(now)
	forged := observation
	forged.SourceSequence++
	provider := &localInspectSubstitutionProviderV2{
		customProviderV2:  &customProviderV2{prepared: prepared, observation: observation},
		forgedObservation: forged,
	}
	if report, err := conformance.CheckGovernedProviderV2(context.Background(), conformance.GovernedProviderCaseV2{
		Provider: provider, Prepare: prepare, Execute: execute, Now: now, PrepareParallelism: 8,
	}); err == nil || report.CertificationCandidate {
		t.Fatalf("InspectLocalAttempt substitution was certified: report=%#v err=%v", report, err)
	}
	if provider.executeCalls != 1 {
		t.Fatalf("conformance redispatched Execute while checking local Inspect: %d", provider.executeCalls)
	}
}

type inspectSubstitutionProviderV2 struct {
	*customProviderV2
	forgedPrepared runtimeports.ProviderPreparationAttestationV2
}

func (p *inspectSubstitutionProviderV2) InspectPrepared(context.Context, runtimeports.InspectPreparedProviderRequestV2) (runtimeports.ProviderPreparationAttestationV2, error) {
	return p.forgedPrepared, nil
}

type localInspectSubstitutionProviderV2 struct {
	*customProviderV2
	forgedObservation runtimeports.ProviderAttemptObservationV2
}

func (p *localInspectSubstitutionProviderV2) InspectLocalAttempt(context.Context, runtimeports.InspectLocalProviderAttemptRequestV2) (runtimeports.ProviderAttemptObservationV2, error) {
	return p.forgedObservation, nil
}
