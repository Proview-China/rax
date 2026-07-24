package contract_test

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func TestHistoricalCaseAndRoundOmitAbsentExactRubricV1(t *testing.T) {
	now := time.Unix(1_900_999_900, 0)
	target := testkit.Target(now)
	caseFact, err := contract.SealReviewCaseV1(contract.ReviewCaseV1{FactIdentityV1: contract.FactIdentityV1{TenantID: target.TenantID, ID: "historical-case", Revision: 1, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()}, TargetID: target.ID, TargetRevision: target.Revision, TargetDigest: target.Digest, State: contract.CaseRequestedV1, ExpiresUnixNano: now.Add(time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	round := testkit.Round(now, caseFact, contract.RouteHumanV1)
	round.Rubric = nil
	round.Digest = ""
	round, err = contract.SealReviewRoundV1(round)
	if err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]any{"case": caseFact, "round": round} {
		payload, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(payload, []byte(`"rubric"`)) {
			t.Fatalf("historical %s serialization changed by an absent Rubric field: %s", name, payload)
		}
	}
}

func TestRubricDefinitionSupportsFrozenKindsV1(t *testing.T) {
	now := time.Unix(1_901_000_000, 0)
	kinds := []contract.RubricKindV1{
		contract.RubricActionSafetyV1, contract.RubricCodeChangeV1, contract.RubricWorkStateV1,
		contract.RubricArtifactQualityV1, contract.RubricOutcomeAcceptanceV1,
		contract.RubricLegalComplianceV1, contract.RubricFinanceControlV1,
	}
	for _, kind := range kinds {
		t.Run(string(kind), func(t *testing.T) {
			sealed, err := contract.NewBaselineRubricDefinitionV1(contract.FactIdentityV1{ContractVersion: contract.ContractVersionV1, TenantID: "tenant-a", ID: "rubric-" + string(kind), Revision: 1, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()}, kind, now.Add(time.Hour).UnixNano())
			if err != nil {
				t.Fatal(err)
			}
			if err := sealed.ValidateCurrent(sealed.ExactRef(), now); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRubricDefinitionHardNegativesV1(t *testing.T) {
	now := time.Unix(1_901_000_100, 0)
	tests := map[string]func(*contract.RubricDefinitionV1){
		"unknown-kind":     func(v *contract.RubricDefinitionV1) { v.Kind = "universal_prompt" },
		"missing-criteria": func(v *contract.RubricDefinitionV1) { v.Criteria = nil },
		"missing-rules":    func(v *contract.RubricDefinitionV1) { v.Rules = nil },
		"unknown-rule":     func(v *contract.RubricDefinitionV1) { v.Rules[0].Kind = "execute_tool" },
		"write-capability": func(v *contract.RubricDefinitionV1) {
			v.AllowedReadOnlyCapabilities = []contract.RubricReadOnlyCapabilityV1{"workspace.write"}
		},
		"unbounded-rounds":    func(v *contract.RubricDefinitionV1) { v.Termination.MaxRounds = 0 },
		"unbounded-tokens":    func(v *contract.RubricDefinitionV1) { v.Termination.MaxTokens = 0 },
		"accept-on-failure":   func(v *contract.RubricDefinitionV1) { v.Criteria[0].FailureResolution = contract.ResolutionAcceptV1 },
		"partial-output":      func(v *contract.RubricDefinitionV1) { v.OutputSchema.RequiredFindingFields = []string{"claim"} },
		"duplicate-criterion": func(v *contract.RubricDefinitionV1) { v.Criteria = append(v.Criteria, v.Criteria[0]) },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			value := testkit.Rubric(now, "tenant-a")
			mutate(&value)
			value.Digest = ""
			if _, err := contract.SealRubricDefinitionV1(value); err == nil {
				t.Fatal("invalid Rubric was sealed")
			}
		})
	}
	value := testkit.Rubric(now, "tenant-a")
	if err := value.ValidateCurrent(contract.ExactResourceRefV1{ID: value.ID, Revision: value.Revision, Digest: testkit.Digest("drift")}, now); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("drifted exact ref was accepted: %v", err)
	}
	if err := value.ValidateCurrent(value.ExactRef(), time.Unix(0, value.ExpiresUnixNano)); !core.HasCategory(err, core.ErrorPreconditionFailed) {
		t.Fatalf("expired Rubric was accepted: %v", err)
	}
	future := value
	future.UpdatedUnixNano = now.Add(time.Minute).UnixNano()
	future.Digest = ""
	future, _ = contract.SealRubricDefinitionV1(future)
	if err := future.ValidateCurrent(future.ExactRef(), now); !core.HasReason(err, core.ReasonClockRegression) {
		t.Fatalf("Rubric currentness accepted a clock before Updated: %v", err)
	}
}

func TestRubricDefinitionValidatesStructuredAttestationOutputV1(t *testing.T) {
	now := time.Unix(1_901_000_200, 0)
	rubric := testkit.Rubric(now, "tenant-a")
	target := testkit.Target(now)
	caseFact, err := contract.SealReviewCaseV1(contract.ReviewCaseV1{FactIdentityV1: contract.FactIdentityV1{ContractVersion: contract.ContractVersionV1, TenantID: target.TenantID, ID: "case-rubric-output", Revision: 1, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()}, TargetID: target.ID, TargetRevision: target.Revision, TargetDigest: target.Digest, State: contract.CaseRequestedV1, ExpiresUnixNano: now.Add(30 * time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	round := testkit.Round(now, caseFact, contract.RouteHumanV1)
	assignment := testkit.Assignment(now, caseFact, round, contract.RouteHumanV1)
	attestation := testkit.HumanAttestation(now, caseFact, round, assignment, contract.ResolutionAcceptV1, "rubric-output")
	authority := testkit.Evidence("rubric-authority")
	authority.Classification = "review.authority/current"
	scope := testkit.Evidence("rubric-scope")
	scope.Classification = "review.scope/current"
	attestation.Evidence = append(attestation.Evidence[:0], authority, scope)
	attestation.EvidenceDigest, _ = contract.ComputeReviewEvidenceDigestV1(attestation.Evidence)
	attestation.Digest = ""
	attestation, err = contract.SealAttestationV1(attestation)
	if err != nil {
		t.Fatal(err)
	}
	if err := rubric.ValidateAttestationOutputV1(attestation, nil); err != nil {
		t.Fatal(err)
	}
	drift := attestation
	drift.Evidence = drift.Evidence[:1]
	drift.EvidenceDigest, _ = contract.ComputeReviewEvidenceDigestV1(drift.Evidence)
	drift.Digest = ""
	drift, _ = contract.SealAttestationV1(drift)
	if err := rubric.ValidateAttestationOutputV1(drift, nil); !core.HasReason(err, core.ReasonEvidenceUnavailable) {
		t.Fatalf("Rubric-required Evidence kind was not enforced: %v", err)
	}
}
