package contract_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestAutoReviewerOutputSchemaV1StrictDraftAndDeepClone(t *testing.T) {
	document, err := contract.BuiltinAutoReviewerOutputSchemaDocumentV1()
	if err != nil {
		t.Fatal(err)
	}
	payload := json.RawMessage(`{"resolution":"accept","reason_codes":["review.accepted"],"findings":[],"evidence":[{"ref":"evidence-1","classification":"review.test/result","digest":"` + string(core.DigestBytes([]byte("evidence"))) + `"}]}`)
	sealed, err := document.ValidateDraftV1(payload)
	if err != nil || sealed.Resolution != contract.ResolutionAcceptV1 || sealed.Digest.Validate() != nil {
		t.Fatalf("valid strict draft was not sealed: %+v err=%v", sealed, err)
	}

	clone := document.Clone()
	clone.Document[0] = '['
	if document.Document[0] == clone.Document[0] {
		t.Fatal("schema document clone leaked a mutable alias")
	}
	rubric, err := contract.NewBaselineRubricDefinitionV1(contract.FactIdentityV1{TenantID: "tenant-a", ID: "rubric-schema", Revision: 1, CreatedUnixNano: 1, UpdatedUnixNano: 1}, contract.RubricActionSafetyV1, 100)
	if err != nil || document.ValidateForRubricV1(rubric, document.Schema) != nil {
		t.Fatalf("builtin schema did not bind the exact baseline Rubric: %v", err)
	}
	drifted := document.Schema
	drifted.ContentDigest = core.DigestBytes([]byte("drift"))
	if err := document.ValidateForRubricV1(rubric, drifted); !core.HasReason(err, core.ReasonReviewCandidateConflict) {
		t.Fatalf("Attempt schema drift was accepted: %v", err)
	}
}

func TestAutoReviewerOutputSchemaV1HardNegatives(t *testing.T) {
	document, err := contract.BuiltinAutoReviewerOutputSchemaDocumentV1()
	if err != nil {
		t.Fatal(err)
	}
	evidence := `{"ref":"evidence-1","classification":"review.test/result","digest":"` + string(core.DigestBytes([]byte("evidence"))) + `"}`
	tests := []string{
		`{"resolution":"accept","resolution":"reject","reason_codes":["x"],"findings":[],"evidence":[` + evidence + `]}`,
		`{"resolution":"accept","reason_codes":["x"],"findings":[],"evidence":[` + evidence + `],"unknown":true}`,
		`{"resolution":"conditional_accept","reason_codes":["x"],"findings":[],"evidence":[` + evidence + `]}`,
		`{"resolution":"accept","reason_codes":[],"findings":[],"evidence":[` + evidence + `]}`,
		`{"resolution":"accept","reason_codes":["x"],"findings":[],"evidence":[]}`,
		`{"resolution":"accept","reason_codes":["x"],"findings":[],"evidence":[{"ref":"evidence-1","classification":"review.test/result","digest":"` + string(core.DigestBytes([]byte("evidence"))) + `","digest":"` + string(core.DigestBytes([]byte("other"))) + `"}]}`,
		`{"resolution":"accept","reason_codes":["x"],"findings":[],"evidence":[{"ref":"evidence-1","classification":"review.test/result","digest":"` + string(core.DigestBytes([]byte("evidence"))) + `","unknown":true}]}`,
		`{"resolution":"accept","reason_codes":["x"],"findings":[{"category":"safety","priority":"high","anchor":"a","claim":"c","impact":"i","evidence":[{"ref":"evidence-1","classification":"review.test/result","digest":"` + string(core.DigestBytes([]byte("evidence"))) + `","ref":"evidence-2"}]}],"evidence":[` + evidence + `]}`,
	}
	for _, payload := range tests {
		if got, err := document.ValidateDraftV1(json.RawMessage(payload)); err == nil || got.Digest != "" {
			t.Fatalf("invalid draft was accepted: %s => %+v", payload, got)
		}
	}

	drift := document.Clone()
	drift.Document = append(drift.Document, ' ')
	if err := drift.Validate(); !core.HasReason(err, core.ReasonInvalidReference) {
		t.Fatalf("schema byte drift was not rejected: %v", err)
	}
}

func TestAutoReviewerOutputSchemaV1RejectsExternalRef(t *testing.T) {
	for _, keyword := range []string{"$ref", "$dynamicRef", "$recursiveRef"} {
		_, err := contract.SealAutoReviewerOutputSchemaDocumentV1(contract.AutoReviewerOutputSchemaDocumentV1{Document: json.RawMessage(`{"` + keyword + `":"https://example.com/schema.json"}`)})
		if !core.HasReason(err, core.ReasonInvalidReference) {
			t.Fatalf("external schema %s was accepted: %v", keyword, err)
		}
	}
}

func TestConditionV2AutoReviewerSchemaExactSetAndHardNegatives(t *testing.T) {
	document, err := contract.BuiltinAutoReviewerOutputSchemaDocumentV1()
	if err != nil {
		t.Fatal(err)
	}
	condition := runtimeports.ReviewConditionV2{ID: "review/followup", Revision: 1, Schema: runtimeports.SchemaRefV2{Namespace: "review", Name: "condition", Version: "1.0.0", MediaType: "application/json", ContentDigest: core.DigestBytes([]byte("schema"))}, ConstraintDigest: core.DigestBytes([]byte("constraint")), SatisfactionOwner: runtimeports.ReviewComponentBindingRefV2{BindingSetID: "set", BindingSetRevision: 1, ComponentID: "review/condition-owner", ManifestDigest: core.DigestBytes([]byte("manifest")), ArtifactDigest: core.DigestBytes([]byte("artifact")), Capability: "review/satisfy"}, ScopeDigest: core.DigestBytes([]byte("scope")), Authority: runtimeports.AuthorityBindingRefV2{Ref: "authority", Revision: 1, Digest: core.DigestBytes([]byte("authority")), Epoch: 1}, ExpiresUnixNano: 100}
	evidence := runtimeports.ReviewEvidenceRefV2{Ref: "evidence", Classification: "review/evidence", Digest: core.DigestBytes([]byte("evidence"))}
	payload, _ := json.Marshal(map[string]any{"resolution": "conditional_acceptance", "reason_codes": []string{"review/conditional"}, "findings": []any{}, "evidence": []runtimeports.ReviewEvidenceRefV2{evidence}, "conditions": []runtimeports.ReviewConditionV2{condition}})
	sealed, err := document.ValidateDraftV1(payload)
	if err != nil || len(sealed.Conditions) != 1 || sealed.Conditions[0] != condition {
		t.Fatalf("exact conditional draft was not preserved: %+v err=%v", sealed, err)
	}
	want, _ := runtimeports.DigestReviewConditionsV2([]runtimeports.ReviewConditionV2{condition})
	if sealed.ConditionsDigest != want {
		t.Fatalf("host did not compute exact condition digest: got=%s want=%s", sealed.ConditionsDigest, want)
	}
	conditionJSON, _ := json.Marshal(condition)
	evidenceJSON, _ := json.Marshal(evidence)
	invalid := [][]byte{
		[]byte(`{"resolution":"conditional_acceptance","reason_codes":["x"],"findings":[],"evidence":[` + string(evidenceJSON) + `]}`),
		[]byte(`{"resolution":"accept","reason_codes":["x"],"findings":[],"evidence":[` + string(evidenceJSON) + `],"conditions":[` + string(conditionJSON) + `]}`),
		[]byte(`{"resolution":"conditional_acceptance","reason_codes":["x"],"findings":[],"evidence":[` + string(evidenceJSON) + `],"conditions_digest":"` + string(want) + `","conditions":[` + string(conditionJSON) + `]}`),
		[]byte(`{"resolution":"conditional_acceptance","reason_codes":["x"],"findings":[],"evidence":[` + string(evidenceJSON) + `],"conditions":[` + strings.Replace(string(conditionJSON), `"revision":1`, `"revision":1,"unknown":true`, 1) + `]}`),
		[]byte(`{"resolution":"conditional_acceptance","reason_codes":["x"],"findings":[],"evidence":[` + string(evidenceJSON) + `],"conditions":[` + strings.Replace(string(conditionJSON), `"revision":1`, `"revision":1,"revision":2`, 1) + `]}`),
	}
	for _, raw := range invalid {
		if got, e := document.ValidateDraftV1(raw); e == nil || got.Digest != "" {
			t.Fatalf("invalid exact condition payload was accepted: %s", raw)
		}
	}
}

func TestAutoReviewerOutputAndObservationSealsOwnDeepCopiesV1(t *testing.T) {
	digest := core.DigestBytes([]byte("evidence"))
	reasons := []string{"reason-b", "reason-a"}
	findingEvidence := []runtimeports.ReviewEvidenceRefV2{{Ref: "finding-evidence", Classification: "review.test/finding", Digest: digest}}
	findings := []contract.AutoFindingDraftV1{{Category: "safety", Priority: "high", Anchor: "anchor", Claim: "claim", Impact: "impact", Evidence: findingEvidence}}
	outputEvidence := []runtimeports.ReviewEvidenceRefV2{{Ref: "output-evidence", Classification: "review.test/output", Digest: digest}}
	sealed, err := contract.SealAutoReviewerStructuredOutputV1(contract.AutoReviewerStructuredOutputV1{Resolution: contract.ResolutionRejectV1, ReasonCodes: reasons, Findings: findings, Evidence: outputEvidence})
	if err != nil {
		t.Fatal(err)
	}
	reasons[0] = "mutated"
	findings[0].Claim = "mutated"
	findingEvidence[0].Ref = "mutated"
	outputEvidence[0].Ref = "mutated"
	if err = sealed.Validate(); err != nil || sealed.ReasonCodes[0] != "reason-a" || sealed.Findings[0].Claim != "claim" || sealed.Findings[0].Evidence[0].Ref != "finding-evidence" || sealed.Evidence[0].Ref != "output-evidence" {
		t.Fatalf("structured output seal retained a mutable alias: %+v err=%v", sealed, err)
	}

	input := sealed.Clone()
	delegation := runtimeports.ExecutionDelegationRefV2{ID: "delegation-a", Revision: 1, Digest: core.DigestBytes([]byte("delegation"))}
	observation, err := contract.SealAutoReviewerInvocationObservationV1(contract.AutoReviewerInvocationObservationV1{
		FactIdentityV1: contract.FactIdentityV1{TenantID: "tenant-a", ID: "observation-alias", Revision: 1, CreatedUnixNano: 1, UpdatedUnixNano: 1},
		AttemptID:      "attempt-a", AttemptRevision: 1, AttemptDigest: core.DigestBytes([]byte("attempt")), OperationDigest: core.DigestBytes([]byte("operation")),
		RuntimeAttempt:      runtimeports.OperationDispatchAttemptRefV3{OperationDigest: core.DigestBytes([]byte("runtime-operation")), EffectID: "effect-a", IntentRevision: 1, IntentDigest: core.DigestBytes([]byte("intent")), PermitID: "permit-a", PermitRevision: 1, PermitDigest: core.DigestBytes([]byte("permit")), AttemptID: "runtime-attempt-a", Delegation: &delegation},
		ProviderObservation: runtimeports.ProviderAttemptObservationRefV2{Delegation: delegation, PreparedAttemptID: "prepared-a", ProviderOperationRef: "provider-operation-a", Revision: 1, State: runtimeports.ProviderAttemptObservedV2, Digest: core.DigestBytes([]byte("provider-observation")), PayloadDigest: core.DigestBytes([]byte("provider-payload")), PayloadRevision: 1, SourceRegistrationID: "source-a", SourceEpoch: 1, SourceSequence: 1, Evidence: runtimeports.EvidenceRecordRefV2{LedgerScopeDigest: core.DigestBytes([]byte("ledger")), Sequence: 1, RecordDigest: core.DigestBytes([]byte("record"))}, ObservedUnixNano: 1}, Output: input,
		ResultSchema: runtimeports.SchemaRefV2{Namespace: "praxis.review", Name: "schema", Version: "1.0.0", MediaType: "application/schema+json", ContentDigest: core.DigestBytes([]byte("schema"))},
		Tokens:       1, CostMicros: 1, ObservedUnixNano: 1, ExpiresUnixNano: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	input.ReasonCodes[0] = "mutated"
	if observation.Output.ReasonCodes[0] == "mutated" || observation.Validate() != nil {
		t.Fatal("Observation seal retained the caller Output alias")
	}
}
