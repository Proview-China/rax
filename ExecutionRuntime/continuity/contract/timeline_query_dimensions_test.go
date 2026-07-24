package contract_test

import (
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
)

func TestTimelineQueryV1TypedObjectDimensionsAreValidatedAndSealed(t *testing.T) {
	baseline := contract.TimelineQuery{
		LedgerScopeDigest: "ledger-scope-1", AuthorityWatermark: "authority-1",
		PolicyWatermark: "policy-1", PageLimit: 10,
	}
	baseDigest, err := baseline.Digest()
	if err != nil {
		t.Fatal(err)
	}
	fields := []struct {
		name string
		set  func(*contract.TimelineQuery, string)
	}{
		{"turn_ref", func(q *contract.TimelineQuery, v string) { q.TurnRef = v }},
		{"step_ref", func(q *contract.TimelineQuery, v string) { q.StepRef = v }},
		{"action_ref", func(q *contract.TimelineQuery, v string) { q.ActionRef = v }},
		{"artifact_ref", func(q *contract.TimelineQuery, v string) { q.ArtifactRef = v }},
		{"effect_ref", func(q *contract.TimelineQuery, v string) { q.EffectRef = v }},
		{"review_case_ref", func(q *contract.TimelineQuery, v string) { q.ReviewCaseRef = v }},
		{"checkpoint_ref", func(q *contract.TimelineQuery, v string) { q.CheckpointRef = v }},
	}
	for _, field := range fields {
		t.Run(field.name, func(t *testing.T) {
			query := baseline
			field.set(&query, field.name+"-1")
			digest, err := query.Digest()
			if err != nil || digest == baseDigest {
				t.Fatalf("typed dimension was not sealed: digest=%q err=%v", digest, err)
			}
			field.set(&query, "bad\nref")
			if err := query.Validate(); !contract.HasCode(err, contract.ErrInvalidArgument) {
				t.Fatalf("invalid typed dimension was accepted: %v", err)
			}
		})
	}
}
