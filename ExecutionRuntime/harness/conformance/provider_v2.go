package conformance

import (
	"context"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const (
	defaultProviderPrepareParallelismV2 = 16
	maxProviderPrepareParallelismV2     = 128
)

// GovernedProviderCaseV2 is a backend-neutral contract case for a custom
// data-plane provider. Execute must already contain the Runtime-persisted
// Enforcement and prepared Delegation that descend from Prepare.
type GovernedProviderCaseV2 struct {
	Provider           runtimeports.GovernedExecutionProviderV2
	Prepare            runtimeports.PrepareGovernedExecutionRequestV2
	Execute            runtimeports.ExecutePreparedRequestV2
	Now                time.Time
	PrepareParallelism int
}

// GovernedProviderReportV2 is only a certification candidate. In particular,
// passing it never grants Binding, dispatch, settlement or Runtime Outcome
// authority to the provider.
type GovernedProviderReportV2 struct {
	PrepareCreateOnce          bool `json:"prepare_create_once"`
	PrepareConcurrentExact     bool `json:"prepare_concurrent_exact"`
	PreparedInspectExact       bool `json:"prepared_inspect_exact"`
	ExecuteSingleCallExact     bool `json:"execute_single_call_exact"`
	LocalAttemptInspectExact   bool `json:"local_attempt_inspect_exact"`
	CertificationCandidate     bool `json:"certification_candidate"`
	ProductionClaimEligible    bool `json:"production_claim_eligible"`
	BindingEligible            bool `json:"binding_eligible"`
	DispatchEligible           bool `json:"dispatch_eligible"`
	SettlementEligible         bool `json:"settlement_eligible"`
	CompletionEligible         bool `json:"completion_eligible"`
	LocalityAttested           bool `json:"locality_attested"`
	ProductionDurabilityProven bool `json:"production_durability_proven"`
}

// CheckGovernedProviderV2 verifies the provider-owned invariants that a host
// relay cannot manufacture: concurrent Prepare is content-idempotent and
// create-once, and both recovery methods are exact local reads. Execute is
// intentionally invoked exactly once; uncertain execution must be inspected,
// never retried by a conformance harness.
func CheckGovernedProviderV2(ctx context.Context, testCase GovernedProviderCaseV2) (GovernedProviderReportV2, error) {
	if testCase.Provider == nil {
		return GovernedProviderReportV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "governed execution provider is required")
	}
	if err := testCase.Prepare.Validate(); err != nil {
		return GovernedProviderReportV2{}, err
	}
	if err := testCase.Execute.Validate(); err != nil {
		return GovernedProviderReportV2{}, err
	}
	if testCase.Now.IsZero() || !testCase.Now.Before(time.Unix(0, testCase.Prepare.Permit.ExpiresUnixNano)) {
		return GovernedProviderReportV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonDispatchPermitExpired, "conformance clock must be inside the Permit lifetime")
	}
	parallelism := testCase.PrepareParallelism
	if parallelism == 0 {
		parallelism = defaultProviderPrepareParallelismV2
	}
	if parallelism < 2 || parallelism > maxProviderPrepareParallelismV2 {
		return GovernedProviderReportV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "Prepare parallelism must be between 2 and 128")
	}

	results := make([]runtimeports.ProviderPreparationAttestationV2, parallelism)
	errs := make([]error, parallelism)
	var ready sync.WaitGroup
	var start sync.WaitGroup
	ready.Add(parallelism)
	start.Add(1)
	var workers sync.WaitGroup
	workers.Add(parallelism)
	for index := range results {
		go func(index int) {
			defer workers.Done()
			ready.Done()
			start.Wait()
			results[index], errs[index] = testCase.Provider.Prepare(ctx, testCase.Prepare)
		}(index)
	}
	ready.Wait()
	start.Done()
	workers.Wait()

	var expectedDigest core.Digest
	for index, result := range results {
		if errs[index] != nil {
			return GovernedProviderReportV2{}, errs[index]
		}
		if err := result.ValidateAgainstPrepare(testCase.Prepare, testCase.Now); err != nil {
			return GovernedProviderReportV2{}, err
		}
		digest, err := providerConformanceDigestV2("ProviderPreparationAttestationV2", result)
		if err != nil {
			return GovernedProviderReportV2{}, err
		}
		if index == 0 {
			expectedDigest = digest
		} else if digest != expectedDigest {
			return GovernedProviderReportV2{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "concurrent Prepare did not resolve to one exact create-once result")
		}
	}
	expectedPrepared := results[0]
	if testCase.Execute.Prepared != expectedPrepared.Prepared {
		return GovernedProviderReportV2{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Execute does not descend from the exact create-once Prepare result")
	}
	prepared, err := testCase.Provider.InspectPrepared(ctx, runtimeports.InspectPreparedProviderRequestV2{
		DeclaredDelegation: testCase.Prepare.Delegation,
		PreparedAttemptID:  expectedPrepared.Prepared.ID,
		PermitID:           testCase.Prepare.Permit.ID,
		AttemptID:          testCase.Prepare.Permit.AttemptID,
	})
	if err != nil {
		return GovernedProviderReportV2{}, err
	}
	preparedDigest, err := providerConformanceDigestV2("ProviderPreparationAttestationV2", prepared)
	if err != nil || preparedDigest != expectedDigest {
		return GovernedProviderReportV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "InspectPrepared did not return the exact create-once result")
	}

	observation, err := testCase.Provider.ExecutePrepared(ctx, testCase.Execute)
	if err != nil {
		return GovernedProviderReportV2{}, err
	}
	if err := observation.ValidateAgainstPrepared(expectedPrepared.Prepared); err != nil {
		return GovernedProviderReportV2{}, err
	}
	if observation.Delegation != testCase.Execute.Delegation {
		return GovernedProviderReportV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "ExecutePrepared returned another delegation")
	}
	observationDigest, err := providerConformanceDigestV2("ProviderAttemptObservationV2", observation)
	if err != nil {
		return GovernedProviderReportV2{}, err
	}
	inspected, err := testCase.Provider.InspectLocalAttempt(ctx, runtimeports.InspectLocalProviderAttemptRequestV2{Delegation: testCase.Execute.Delegation, Prepared: expectedPrepared.Prepared})
	if err != nil {
		return GovernedProviderReportV2{}, err
	}
	inspectedDigest, err := providerConformanceDigestV2("ProviderAttemptObservationV2", inspected)
	if err != nil || inspectedDigest != observationDigest {
		return GovernedProviderReportV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "InspectLocalAttempt did not return the exact single Execute result")
	}

	return GovernedProviderReportV2{
		PrepareCreateOnce:          true,
		PrepareConcurrentExact:     true,
		PreparedInspectExact:       true,
		ExecuteSingleCallExact:     true,
		LocalAttemptInspectExact:   true,
		CertificationCandidate:     true,
		ProductionClaimEligible:    false,
		BindingEligible:            false,
		DispatchEligible:           false,
		SettlementEligible:         false,
		CompletionEligible:         false,
		LocalityAttested:           false,
		ProductionDurabilityProven: false,
	}, nil
}

func providerConformanceDigestV2(kind string, value any) (core.Digest, error) {
	return core.CanonicalJSONDigest("praxis.harness.governed-provider", runtimeports.ExecutionGovernanceContractVersionV2, kind, value)
}
