package conformance

import (
	"context"
	"sync"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// GovernedExecutionProviderCaseV2 checks the public Prepare boundary only.
// Passing proves exact envelope handling; it never grants Binding, dispatch,
// commit, Outcome, production durability or an SLA.
type GovernedExecutionProviderCaseV2 struct {
	Provider ports.GovernedExecutionProviderV2
	Prepare  ports.PrepareGovernedExecutionRequestV2
}

// IsolatedGovernedExecutionIdempotencyCaseV2 is a destructive fixture-only
// profile. It must never be run by the default provider conformance path or
// against an external production Effect. The external counter is the only
// evidence of logical Effects; equal observations alone cannot prove physical
// exactly-once execution.
type IsolatedGovernedExecutionIdempotencyCaseV2 struct {
	Provider               ports.GovernedExecutionProviderV2
	Execute                ports.ExecutePreparedRequestV2
	Concurrency            int
	IsolatedFixtureOnly    bool
	ObservedLogicalEffects func() uint64
}

type IsolatedGovernedExecutionIdempotencyReportV2 struct {
	MethodEntries               int    `json:"method_entries"`
	DistinctAttemptObservations int    `json:"distinct_attempt_observations"`
	ObservedLogicalEffects      uint64 `json:"observed_logical_effects"`
	ProductionClaimEligible     bool   `json:"production_claim_eligible"`
}

func CheckIsolatedGovernedExecutionIdempotencyV2(ctx context.Context, testCase IsolatedGovernedExecutionIdempotencyCaseV2) (IsolatedGovernedExecutionIdempotencyReportV2, error) {
	if !testCase.IsolatedFixtureOnly || testCase.Provider == nil || testCase.ObservedLogicalEffects == nil || testCase.Concurrency < 2 || testCase.Concurrency > 64 {
		return IsolatedGovernedExecutionIdempotencyReportV2{}, core.NewError(core.ErrorForbidden, core.ReasonEffectAuthorizationMissing, "destructive provider idempotency profile requires an isolated fixture and bounded concurrency")
	}
	if err := testCase.Execute.Validate(); err != nil {
		return IsolatedGovernedExecutionIdempotencyReportV2{}, err
	}
	var wait sync.WaitGroup
	var mu sync.Mutex
	observations := map[core.Digest]struct{}{}
	errors := make(chan error, testCase.Concurrency)
	for range testCase.Concurrency {
		wait.Add(1)
		go func() {
			defer wait.Done()
			observation, err := testCase.Provider.ExecutePrepared(ctx, testCase.Execute)
			if err != nil {
				errors <- err
				return
			}
			digest, digestErr := core.CanonicalJSONDigest("praxis.runtime.execution-governance", ports.ExecutionGovernanceContractVersionV2, "ProviderAttemptObservationV2", observation)
			if digestErr != nil {
				errors <- digestErr
				return
			}
			mu.Lock()
			observations[digest] = struct{}{}
			mu.Unlock()
		}()
	}
	wait.Wait()
	close(errors)
	for err := range errors {
		return IsolatedGovernedExecutionIdempotencyReportV2{}, err
	}
	report := IsolatedGovernedExecutionIdempotencyReportV2{
		MethodEntries:               testCase.Concurrency,
		DistinctAttemptObservations: len(observations),
		ObservedLogicalEffects:      testCase.ObservedLogicalEffects(),
		ProductionClaimEligible:     false,
	}
	if report.DistinctAttemptObservations != 1 || report.ObservedLogicalEffects != 1 {
		return report, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "isolated provider did not linearize one logical attempt")
	}
	return report, nil
}

type GovernedExecutionProviderReportV2 struct {
	ExactPrepareBinding    bool `json:"exact_prepare_binding"`
	EnforcementAttested    bool `json:"enforcement_attested"`
	BindingEligible        bool `json:"binding_eligible"`
	DispatchCommitEligible bool `json:"dispatch_commit_eligible"`
	OutcomeCommitEligible  bool `json:"outcome_commit_eligible"`
	ProductionEligible     bool `json:"production_eligible"`
	SLAClaim               bool `json:"sla_claim"`
}

func CheckGovernedExecutionProviderV2(ctx context.Context, testCase GovernedExecutionProviderCaseV2) (GovernedExecutionProviderReportV2, error) {
	if testCase.Provider == nil {
		return GovernedExecutionProviderReportV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "governed execution provider public Port is required")
	}
	if err := testCase.Prepare.Validate(); err != nil {
		return GovernedExecutionProviderReportV2{}, err
	}
	attestation, err := testCase.Provider.Prepare(ctx, testCase.Prepare)
	if err != nil {
		return GovernedExecutionProviderReportV2{}, err
	}
	if err := attestation.Validate(); err != nil {
		return GovernedExecutionProviderReportV2{}, err
	}
	if attestation.Delegation != testCase.Prepare.Delegation || attestation.Prepared.DeclaredDelegation != testCase.Prepare.Delegation || attestation.Prepared.IntentID != testCase.Prepare.Intent.ID || attestation.Prepared.IntentRevision != testCase.Prepare.Intent.Revision || attestation.Prepared.IntentDigest != testCase.Prepare.Permit.IntentDigest || attestation.Prepared.PermitID != testCase.Prepare.Permit.ID || attestation.Prepared.PermitRevision != testCase.Prepare.Permit.Revision || attestation.Prepared.PermitDigest != mustPermitDigestV3(testCase.Prepare.Permit) || attestation.Prepared.AttemptID != testCase.Prepare.Permit.AttemptID || attestation.Prepared.Provider != testCase.Prepare.Permit.Provider || attestation.Enforcement.Verifier != testCase.Prepare.Permit.EnforcementPoint {
		return GovernedExecutionProviderReportV2{}, core.NewError(core.ErrorConflict, core.ReasonProviderBindingStale, "provider Prepare changed operation, Permit, attempt or provider binding")
	}
	return GovernedExecutionProviderReportV2{
		ExactPrepareBinding:    true,
		EnforcementAttested:    true,
		BindingEligible:        false,
		DispatchCommitEligible: false,
		OutcomeCommitEligible:  false,
		ProductionEligible:     false,
		SLAClaim:               false,
	}, nil
}

func mustPermitDigestV3(permit ports.OperationDispatchPermitV3) core.Digest {
	digest, _ := permit.DigestV3()
	return digest
}

type ExecutionDelegationBackendCaseV2 struct {
	Facts      ports.ExecutionDelegationFactPortV2
	Delegation ports.ExecutionDelegationFactV2
}

type ExecutionDelegationBackendReportV2 struct {
	CreateOnceVisible      bool `json:"create_once_visible"`
	IdempotentReplay       bool `json:"idempotent_replay"`
	DispatchAuthorityClaim bool `json:"dispatch_authority_claim"`
	ProductionDurability   bool `json:"production_durability_claim"`
	SLAClaim               bool `json:"sla_claim"`
}

func CheckExecutionDelegationBackendV2(ctx context.Context, testCase ExecutionDelegationBackendCaseV2) (ExecutionDelegationBackendReportV2, error) {
	if testCase.Facts == nil {
		return ExecutionDelegationBackendReportV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "execution delegation Fact Port is required")
	}
	created, err := testCase.Facts.CreateExecutionDelegationV2(ctx, testCase.Delegation)
	if err != nil {
		return ExecutionDelegationBackendReportV2{}, err
	}
	inspected, err := testCase.Facts.InspectExecutionDelegationV2(ctx, testCase.Delegation.ID)
	if err != nil {
		return ExecutionDelegationBackendReportV2{}, err
	}
	replayed, err := testCase.Facts.CreateExecutionDelegationV2(ctx, testCase.Delegation)
	if err != nil {
		return ExecutionDelegationBackendReportV2{}, err
	}
	createdDigest, _ := created.DigestV2()
	inspectedDigest, _ := inspected.DigestV2()
	replayedDigest, _ := replayed.DigestV2()
	if createdDigest != inspectedDigest || createdDigest != replayedDigest {
		return ExecutionDelegationBackendReportV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "delegation create/inspect/replay changed canonical content")
	}
	return ExecutionDelegationBackendReportV2{CreateOnceVisible: true, IdempotentReplay: true}, nil
}
